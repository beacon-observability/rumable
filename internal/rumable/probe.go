package rumable

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

// Match the browser negotiation that exposed the IIS 0.1.5 cached HTML gap.
// An unsupported response encoding fails closed; never inspect compressed bytes
// as HTML or claim that a variant we could not decode passed.
const browserAcceptEncoding = "gzip, deflate, br, zstd"

type Page struct {
	URL    string
	Header http.Header
	Body   []byte
}
type ProbeResult struct {
	URL      string `json:"url"`
	Injected bool   `json:"injected"`
	Script   string `json:"script,omitempty"`
	Message  string `json:"message"`
}

func getPage(raw string, headers http.Header) (Page, error) {
	r, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return Page{}, err
	}
	r.Header.Set("Accept-Encoding", "identity")
	r.Header.Set("Cache-Control", "no-cache")
	// A graceful reload can leave established connections on an old worker.
	// Deployment probes must reconnect to observe the current listener.
	r.Close = true
	for k, v := range headers {
		r.Header[k] = v
	}
	resp, err := httpClient.Do(r)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		err := fmt.Errorf("%s: expected HTTP 200, got %d", raw, resp.StatusCode)
		if resp.StatusCode == http.StatusNotModified {
			return Page{}, &probeContentError{fmt.Errorf("%w; conditional request can reuse stale browser HTML", err)}
		}
		return Page{}, err
	}
	var body io.Reader = resp.Body
	if e := resp.Header.Get("Content-Encoding"); e != "" && e != "identity" {
		if e != "gzip" || (r.Header.Get("Accept-Encoding") != "gzip" && r.Header.Get("Accept-Encoding") != browserAcceptEncoding) {
			return Page{}, fmt.Errorf("%s: cannot verify encoded response %q for Accept-Encoding %q; review source compression (%s)", raw, e, r.Header.Get("Accept-Encoding"), responseEvidence(resp.Header))
		}
		decoded, err := gzip.NewReader(resp.Body)
		if err != nil {
			return Page{}, fmt.Errorf("%s: invalid gzip response: %w", raw, err)
		}
		defer decoded.Close()
		body = decoded
	}
	b, err := io.ReadAll(io.LimitReader(body, 8*1024*1024+1))
	if err != nil {
		return Page{}, err
	}
	if len(b) > 8*1024*1024 {
		return Page{}, errors.New("probe response exceeds 8 MiB")
	}
	return Page{raw, resp.Header, b}, nil
}

func pageURL(s Site, p string) string {
	u, _ := url.Parse(s.PublicURL)
	u.Path = p
	u.RawPath = ""
	return u.String()
}

func scriptURL(s Site, raw string) string {
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return raw
	}
	return pageURL(s, raw)
}

func snapshotSDKURL(n Snapshot) string {
	if n.SDKURL != "" {
		return n.SDKURL
	}
	return n.Site.assetPrefix() + "sdk." + n.SDK.SHA256 + ".js"
}

func probeCDNSDK(s Site, info SDKInfo, raw string) error {
	origin := ""
	if s.SDK.Integrity {
		u, _ := url.Parse(s.PublicURL)
		remote, _ := url.Parse(raw)
		if u.Scheme != remote.Scheme || !strings.EqualFold(u.Hostname(), remote.Hostname()) || urlPort(u) != urlPort(remote) {
			origin = u.Scheme + "://" + u.Host
		}
	}
	if err := probeScript(raw, info.SHA256, origin); err != nil {
		return fmt.Errorf("CDN SDK: %w", err)
	}
	return nil
}

func checkPage(s Site, p Page, managed ...Snapshot) error {
	t, params, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if err != nil || t != "text/html" {
		return fmt.Errorf("%s: expected text/html", p.URL)
	}
	if strings.HasPrefix(strings.ToLower(params["charset"]), "utf-16") || strings.HasPrefix(strings.ToLower(params["charset"]), "utf-32") {
		return errors.New("UTF-16/32 HTML is unsupported")
	}
	if strings.Contains(strings.ToLower(p.Header.Get("Content-Disposition")), "attachment") {
		return errors.New("probe page is a download")
	}
	doc, err := inspectHTML(p.Body)
	if err != nil {
		return err
	}
	if err = doc.checkAnchor(p.Body); err != nil {
		return fmt.Errorf("%s: %w", p.URL, err)
	}
	if err := checkCSP(s, p); err != nil {
		return fmt.Errorf("%s: %w", p.URL, err)
	}
	var managedSync *Snapshot
	for i := range managed {
		n := &managed[i]
		if n.Enabled && n.Site.SDK.Loading == "sync" && n.Site.ID == s.ID {
			managedSync = n
			break
		}
	}
	for _, tag := range doc.elements {
		if tag.name != "script" {
			continue
		}
		a := tag.attrs
		if _, ok := a["data-rumable-sdk"]; ok {
			if managedSync == nil || a["data-rumable-sdk"] != s.ID {
				return errors.New("unrecognized synchronous SDK injection; verify managed state before changing integration")
			}
			// Only verify an existing tag: newly included paths may not have
			// been instrumented by the currently deployed snapshot.
			if _, err := inspectInjection(managedSync.Site, p, managedSync); err != nil {
				return fmt.Errorf("existing synchronous injection: %w", err)
			}
			continue
		}
		if owner, ok := a["data-rumable"]; ok && owner != s.ID {
			return errors.New("another Rumable deployment already instruments this page")
		}
		src := strings.ToLower(a["src"])
		if strings.Contains(src, "dataflux-rum") || strings.Contains(src, "browser-sdk/") {
			return errors.New("existing RUM integration found; do not take over a business-owned SDK")
		}
		if strings.Contains(tag.text, "DATAFLUX_RUM") {
			return errors.New("existing RUM integration found; do not take over a business-owned SDK")
		}
	}
	return nil
}

func probePages(s Site, paths []string, expected *Snapshot) ([]ProbeResult, error) {
	var results []ProbeResult
	checked := map[string]bool{}
	for _, path := range paths {
		r, err := probePage(s, path, expected)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pageURL(s, path), err)
		}
		if (expected == nil && s.RUM.isEnabled() || expected != nil && expected.Enabled) && !checked[r.Script] {
			if err := probeScripts(s, r.Script, expected); err != nil {
				return nil, err
			}
			checked[r.Script] = true
		}
		results = append(results, r)
	}
	if expected == nil && s.RUM.isEnabled() || expected != nil && expected.Enabled {
		if err := probeWorkers(s); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// An observed body mismatch on IIS must not be erased by a later warm-cache
// success. Transport/readiness failures may still use the bounded retry loop.
type probeContentError struct{ error }

func browserProbeHeaders() http.Header {
	return http.Header{
		"Accept-Encoding": {browserAcceptEncoding},
		"Accept":          {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"Cache-Control":   nil,
	}
}

func preflightPage(s Site, path string) (Page, error) {
	if s.IIS != nil && s.RUM.isEnabled() {
		// Identity before the commit can also warm the IIS response cache and
		// mask the first compressed response after installing the rules.
		return getPage(pageURL(s, path), browserProbeHeaders())
	}
	return getPage(pageURL(s, path), nil)
}

func probePage(s Site, path string, expected *Snapshot) (ProbeResult, error) {
	var result ProbeResult
	check := func(label string, headers http.Header) error {
		page, err := getPage(pageURL(s, path), headers)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		got, err := inspectInjection(s, page, expected)
		if err == nil && result.URL != "" && got.Script != result.Script {
			err = errors.New("uses a different bootstrap URL")
		}
		if err != nil {
			return &probeContentError{fmt.Errorf("%s (%s): %w; inspect effective per-URL compression and HTML caches", label, responseEvidence(page.Header), err)}
		}
		result = got
		return nil
	}
	// Exact transaction recovery can restore a pre-0.1.7 deployment without
	// this guard. Do not impose a newly introduced prerequisite on that old
	// snapshot; normal enable/history rollback installs it before verification.
	conditional := expected == nil || !expected.legacyIISRecovery
	if s.IIS != nil && conditional && (expected == nil && s.RUM.isEnabled() || expected != nil && expected.Enabled) {
		// A wildcard ETag matches any pre-existing representation; a future
		// modification date covers old Last-Modified-only browser caches.
		// Run these BEFORE unconditional requests can warm an IIS cache.
		for _, variant := range []struct{ label, etag, modified string }{
			{"conditional ETag reload", "*", ""},
			{"conditional modified reload", "", "Thu, 01 Jan 2099 00:00:00 GMT"},
			{"conditional combined reload", "*", "Thu, 01 Jan 2099 00:00:00 GMT"},
		} {
			headers := browserProbeHeaders()
			headers.Set("Cache-Control", "max-age=0")
			if variant.etag != "" {
				headers.Set("If-None-Match", variant.etag)
			}
			if variant.modified != "" {
				headers.Set("If-Modified-Since", variant.modified)
			}
			if err := check(variant.label, headers); err != nil {
				return result, err
			}
		}
	}
	// IIS 0.1.6 native evidence: a single identity request can make subsequent
	// gzip responses look healthy. Inspect cold browser variants FIRST, including
	// the normal-refresh header; never continue to identity after a mismatch.
	for attempt := 1; attempt <= 3; attempt++ {
		if err := check(fmt.Sprintf("browser cache variant %d", attempt), browserProbeHeaders()); err != nil {
			return result, err
		}
	}
	reload := browserProbeHeaders()
	reload.Set("Cache-Control", "max-age=0")
	if err := check("browser reload variant", reload); err != nil {
		return result, err
	}
	if err := check("gzip variant", http.Header{"Accept-Encoding": {"gzip"}}); err != nil {
		return result, err
	}
	if err := check("identity variant", nil); err != nil {
		return result, err
	}
	return result, nil
}

// Record only headers needed to diagnose compression/cache variants. Avoid
// response bodies, cookies and other potentially secret response headers.
func responseEvidence(h http.Header) string {
	var fields []string
	for _, name := range []string{"Content-Type", "Content-Encoding", "Vary", "Age", "ETag", "Last-Modified", "Server"} {
		value := h.Get(name)
		if len(value) > 256 {
			value = value[:256]
		}
		fields = append(fields, fmt.Sprintf("%s=%q", name, value))
	}
	return strings.Join(fields, ", ")
}

func inspectInjection(s Site, p Page, expected *Snapshot) (ProbeResult, error) {
	if expected == nil && !s.RUM.isEnabled() {
		expected = &Snapshot{}
	}
	r := ProbeResult{URL: p.URL, Message: "injection and script availability only; browser execution and ingestion are not verified"}
	contentType, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if contentType != "text/html" && (expected == nil && s.RUM.isEnabled() || expected != nil && expected.Enabled) {
		return r, errors.New("probe target no longer serves HTML")
	}
	if contentType != "text/html" {
		r.Message = "non-HTML HTTP response verified; no browser SDK expected, OTLP ingestion unverified"
		return r, nil
	}
	count := 0
	total := 0
	sdkCount, sdkIndex, initIndex := 0, -1, -1
	doc, err := inspectHTML(p.Body)
	if err != nil {
		return r, err
	}
	for index, tag := range doc.elements {
		if tag.name != "script" {
			continue
		}
		a := tag.attrs
		if owner, exists := a["data-rumable-sdk"]; exists {
			if owner != s.ID {
				return r, errors.New("another deployment's SDK tag is present")
			}
			sdkCount++
			sdkIndex = index
		}
		if _, exists := a["data-rumable"]; exists {
			total++
		}
		if a["data-rumable"] == s.ID {
			count++
			initIndex = index
			r.Script = a["src"]
		}
	}
	r.Injected = count > 0
	if expected != nil && !expected.Enabled {
		r.Message = "RUM injection absent; HTTP/component state only, SDK execution and OTLP ingestion unverified"
		if count != 0 || sdkCount != 0 {
			return r, errors.New("injection still present after disable; check HTML caches")
		}
		return r, nil
	}
	if count != 1 || total != 1 {
		return r, fmt.Errorf("expected one injection tag for this deployment and no competing tags, found %d own / %d total", count, total)
	}
	if expected != nil && r.Script != expected.Script {
		return r, errors.New("stale bootstrap URL; clear HTML caches")
	}
	if !strings.HasPrefix(r.Script, s.assetPrefix()) || strings.ContainsAny(r.Script, "?#") {
		return r, errors.New("unexpected bootstrap path")
	}
	n := Snapshot{Site: s, Script: r.Script}
	if expected != nil {
		n = *expected
	} else {
		_, n.SDK, err = sdk()
		if err != nil {
			return r, err
		}
		if s.SDK.Source == "cdn" {
			n.SDKURL = sdkLocation(s, n.SDK)
		}
	}
	if n.Site.SDK.Loading == "sync" {
		if sdkCount != 1 || sdkIndex+1 != initIndex {
			return r, errors.New("expected synchronous SDK immediately before initialization script")
		}
		if doc.elements[sdkIndex].attrs["src"] != snapshotSDKURL(n) {
			return r, errors.New("stale synchronous SDK URL; clear HTML caches")
		}
		for _, index := range []int{sdkIndex, initIndex} {
			a := doc.elements[index].attrs
			_, async := a["async"]
			_, deferScript := a["defer"]
			if async || deferScript || a["type"] != "" {
				return r, errors.New("synchronous scripts must be classic scripts without async or defer")
			}
		}
		a := doc.elements[sdkIndex].attrs
		if n.Site.SDK.Source == "cdn" && n.Site.SDK.Integrity {
			if a["integrity"] != sdkIntegrity(n.SDK) || a["crossorigin"] != "anonymous" {
				return r, errors.New("synchronous SDK integrity attributes do not match deployment")
			}
		} else if a["integrity"] != "" || a["crossorigin"] != "" {
			return r, errors.New("unexpected synchronous SDK integrity attributes")
		}
	} else if sdkCount != 0 {
		return r, errors.New("synchronous SDK tag remains in an asynchronous deployment")
	}
	scripts := []string{r.Script, snapshotSDKURL(n)}
	if err := checkCSP(s, p, scripts...); err != nil {
		return r, err
	}
	if etag := p.Header.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		return r, errors.New("rewritten HTML retains a strong ETag")
	}
	return r, nil
}

func probeScripts(s Site, src string, expected *Snapshot) error {
	digest := ""
	if expected != nil {
		rel := "public/" + s.ID + "/" + strings.TrimPrefix(src, s.assetPrefix())
		digest = expected.Assets[rel]
		if digest == "" {
			return errors.New("missing bootstrap checksum in snapshot")
		}
	}
	if err := probeJavaScript(pageURL(s, src), digest); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	if expected != nil {
		if expected.SDKURL != "" {
			return probeCDNSDK(s, expected.SDK, expected.SDKURL)
		}
		if err := probeJavaScript(scriptURL(s, snapshotSDKURL(*expected)), expected.SDK.SHA256); err != nil {
			return fmt.Errorf("SDK: %w", err)
		}
	} else if s.SDK.Source == "cdn" {
		_, info, err := sdk()
		if err != nil {
			return err
		}
		return probeCDNSDK(s, info, sdkLocation(s, info))
	}
	return nil
}

func probeWorkers(s Site) error {
	seen := map[string]bool{}
	for _, raw := range s.RUM.workerURLs(s.PublicURL) {
		if raw == "blob:" || seen[raw] {
			continue
		}
		seen[raw] = true
		if err := probeJavaScript(raw, ""); err != nil {
			return fmt.Errorf("SDK worker: %w", err)
		}
	}
	return nil
}

func probeJavaScript(raw, digest string) error {
	return probeScript(raw, digest, "")
}

func probeScript(raw, digest, origin string) error {
	for _, encoding := range []string{"identity", "gzip"} {
		headers := http.Header{"Accept-Encoding": []string{encoding}}
		if origin != "" {
			headers.Set("Origin", origin)
		}
		p, err := getPage(raw, headers)
		if err != nil {
			return err
		}
		kind, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if !javascriptMIME(kind) {
			return fmt.Errorf("%s (%s) is not served as JavaScript", raw, encoding)
		}
		if origin != "" {
			allowed := p.Header.Get("Access-Control-Allow-Origin")
			if len(p.Header.Values("Access-Control-Allow-Origin")) != 1 || (allowed != "*" && allowed != origin) {
				return fmt.Errorf("%s: CDN integrity requires Access-Control-Allow-Origin for %s", raw, origin)
			}
		}
		if digest == "" {
			digest = hash(p.Body)
		} else if hash(p.Body) != digest {
			return fmt.Errorf("%s (%s): script checksum mismatch", raw, encoding)
		}
	}
	return nil
}

func javascriptMIME(t string) bool {
	// Include historical JavaScript types defined by the MIME Sniffing Standard.
	switch t {
	case "application/ecmascript", "application/javascript", "application/x-ecmascript", "application/x-javascript", "text/ecmascript", "text/javascript", "text/javascript1.0", "text/javascript1.1", "text/javascript1.2", "text/javascript1.3", "text/javascript1.4", "text/javascript1.5", "text/jscript", "text/livescript", "text/x-ecmascript", "text/x-javascript":
		return true
	}
	return false
}
