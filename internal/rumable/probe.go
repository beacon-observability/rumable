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
		return Page{}, fmt.Errorf("%s: expected HTTP 200, got %d", raw, resp.StatusCode)
	}
	var body io.Reader = resp.Body
	if e := resp.Header.Get("Content-Encoding"); e != "" && e != "identity" {
		if e != "gzip" || r.Header.Get("Accept-Encoding") != "gzip" {
			return Page{}, fmt.Errorf("%s: encoded response %q for Accept-Encoding %q; fix source compression", raw, e, r.Header.Get("Accept-Encoding"))
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

func probePage(s Site, path string, expected *Snapshot) (ProbeResult, error) {
	p, err := getPage(pageURL(s, path), nil)
	if err != nil {
		return ProbeResult{}, err
	}
	r, err := inspectInjection(s, p, expected)
	if err != nil {
		return r, err
	}
	// Identity alone misses gzip_static and cached compressed HTML used by browsers.
	compressed, err := getPage(pageURL(s, path), http.Header{"Accept-Encoding": []string{"gzip"}})
	if err != nil {
		return r, err
	}
	other, err := inspectInjection(s, compressed, expected)
	if err != nil {
		return r, fmt.Errorf("gzip variant: %w", err)
	}
	if other.Script != r.Script {
		return r, errors.New("gzip variant uses a different bootstrap URL; clear HTML caches")
	}
	return r, nil
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
