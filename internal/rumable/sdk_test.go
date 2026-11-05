package rumable

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Synthetic SDKs exercise metadata/byte identity without depending on a live CDN.
func syntheticCDNSDK(version string) []byte {
	return []byte(`window.DATAFLUX_RUM={init:function(){},startSessionReplayRecording:function(){},sdkVersion:"` + version + `",sdkName:"df_web_rum_sdk"};`)
}

func TestCDNResolveAndRetainActualVersion(t *testing.T) {
	var content atomic.Value
	content.Store(syntheticCDNSDK("3.99.1"))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		body := content.Load().([]byte)
		if r.Header.Get("Accept-Encoding") == "gzip" {
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			defer z.Close()
			z.Write(body)
		} else {
			w.Write(body)
		}
	}))
	defer server.Close()
	previousClient := httpClient
	httpClient = server.Client()
	t.Cleanup(func() { httpClient = previousClient })
	s := testSite()
	s.SDK = SDKConfig{Source: "cdn", URL: server.URL + "/sdk.js", Integrity: true}
	root := t.TempDir()
	resolved, err := resolveCDNSDK(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := probeCDNSDK(s, resolved.info, resolved.info.Source); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"async", "sync"} {
		s.SDK.Loading = mode
		n, files, err := renderDeploymentSDK(root, s, true, nil, false, resolved)
		if err != nil {
			t.Fatal(err)
		}
		if n.SDK.Version != "3.99.1" || n.SDK.SHA256 != hash(content.Load().([]byte)) || n.SDKURL != s.SDK.URL {
			t.Fatal("deployment substituted the bundled SDK for the resolved CDN")
		}
		if err := validateSnapshot(n, s.ID); err != nil {
			t.Fatal(err)
		}
		page := Page{Header: http.Header{"Content-Type": {"text/html"}}, Body: []byte("<html><head>" + injectionTags(n) + "</head></html>")}
		if _, err := inspectInjection(s, page, nil); err != nil {
			t.Fatal("probe without saved state still compared the bundled SDK", err)
		}
		if !bytes.Equal(files["public/"+s.ID+"/sdk."+n.SDK.SHA256+".js"], resolved.body) {
			t.Fatal("retained SDK differs from the deployment digest")
		}
		boot := files["public/"+s.ID+"/"+strings.TrimPrefix(n.Script, s.assetPrefix())]
		if !strings.Contains(string(boot)+injectionTags(n), sdkIntegrity(resolved.info)) {
			t.Fatal("SRI does not use resolved bytes")
		}
		if err := deployAssets(root, s, files); err != nil {
			t.Fatal(err)
		}
		// Serialization must retain the selected version independently of this CLI.
		encoded, err := json.Marshal(n)
		if err != nil {
			t.Fatal(err)
		}
		var saved Snapshot
		if err := json.Unmarshal(encoded, &saved); err != nil {
			t.Fatal(err)
		}
		if err := verifyFilesAssets(root, saved); err != nil {
			t.Fatal(err)
		}
		content.Store(syntheticCDNSDK("3.99.2"))
		if probeCDNSDK(s, saved.SDK, saved.SDKURL) == nil {
			t.Fatal("saved deployment silently adopted changed CDN")
		}
		next, err := resolveCDNSDK(s)
		if err != nil || next.info.Version != "3.99.2" {
			t.Fatal("re-enable cannot resolve updated CDN", err)
		}
		content.Store(resolved.body)
		if err := probeCDNSDK(s, saved.SDK, saved.SDKURL); err != nil {
			t.Fatal("historical CDN cannot be verified", err)
		}
	}
}

func TestCDNResolveRejectsUnsupportedContent(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		body       []byte
		status     int
	}{
		{"html", "text/html", syntheticCDNSDK("3.99.1"), 200},
		{"wrong SDK", "application/javascript", []byte("window.otherSDK={};"), 200},
		{"new major", "application/javascript", syntheticCDNSDK("4.0.0"), 200},
		{"prerelease", "application/javascript", syntheticCDNSDK("3.99.1-beta"), 200},
		{"ambiguous", "application/javascript", append(syntheticCDNSDK("3.99.1"), syntheticCDNSDK("3.99.2")...), 200},
		{"redirect", "application/javascript", syntheticCDNSDK("3.99.1"), 302},
		{"empty", "application/javascript", nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.kind)
				w.WriteHeader(tc.status)
				w.Write(tc.body)
			}))
			defer server.Close()
			previous := httpClient
			httpClient = server.Client()
			defer func() { httpClient = previous }()
			s := testSite()
			s.SDK = SDKConfig{Source: "cdn", URL: server.URL + "/sdk.js"}
			if _, err := resolveCDNSDK(s); err == nil {
				t.Fatal("unsupported CDN accepted")
			}
		})
	}
}

func TestCDNConfigurationAndSavedReferences(t *testing.T) {
	name := filepath.Join(t.TempDir(), "rumable.toml")
	config := strings.Replace(exampleConfig, `source = "bundled"`, `source = "cdn"`, 1)
	if err := os.WriteFile(name, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := loadConfig(name, "")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(root, s.ID)
	if err != nil || saved.Current.SDKURL != st.Current.SDK.Source {
		t.Fatal("CDN source did not survive deployment", err)
	}
	s.SDK.URL = "https://cdn.example.com/rum/3.3.21.js"
	s.SDK.Integrity = true
	n, files, err := render(root, s, true)
	if err != nil || n.SDKURL != s.SDK.URL {
		t.Fatal("custom CDN reference lost", err)
	}
	boot := string(files["public/"+s.ID+"/"+strings.TrimPrefix(n.Script, s.assetPrefix())])
	if !strings.Contains(boot, `script.crossOrigin="anonymous"`) || !strings.Contains(boot, `script.integrity="sha256-`) || !strings.Contains(boot, s.SDK.URL) {
		t.Fatal("CDN integrity did not reach executable bootstrap")
	}
	if err = validateSnapshot(n, s.ID); err != nil {
		t.Fatal(err)
	}
	n.SDKURL = "https://different.example.com/sdk.js"
	if validateSnapshot(n, s.ID) == nil {
		t.Fatal("saved CDN source can be substituted")
	}
	for _, c := range []SDKConfig{{Source: "unknown"}, {URL: s.SDK.URL}, {Integrity: true}, {Source: "cdn", URL: "http://cdn.example.com/sdk.js"}, {Source: "cdn", URL: "https://a:b@cdn.example.com/sdk.js"}, {Source: "cdn", URL: "https://cdn.example.com/sdk.js?"}} {
		s.SDK = c
		if s.validate() == nil {
			t.Fatal("invalid SDK configuration accepted", c)
		}
	}
}

func TestCDNProbeChecksCSPContentAndOptionalCORS(t *testing.T) {
	b, info, err := sdk()
	if err != nil {
		t.Fatal(err)
	}
	cors, corrupt, badGzip := false, false, false
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		if cors {
			w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
		}
		body := b
		if corrupt || (badGzip && r.Header.Get("Accept-Encoding") == "gzip") {
			body = []byte("/* stale CDN object */")
		}
		if r.Header.Get("Accept-Encoding") == "gzip" {
			w.Header().Set("Content-Encoding", "gzip")
			z := gzip.NewWriter(w)
			z.Write(body)
			z.Close()
		} else {
			w.Write(body)
		}
	}))
	defer server.Close()
	oldClient := httpClient
	httpClient = server.Client()
	defer func() { httpClient = oldClient }()
	s := testSite()
	s.SDK = SDKConfig{Source: "cdn", URL: server.URL + "/sdk.js"}
	p := Page{Header: http.Header{"Content-Security-Policy": []string{"script-src 'self'; connect-src https:"}}}
	if checkCSP(s, p) == nil {
		t.Fatal("CSP allowed CDN through self")
	}
	p.Header.Set("Content-Security-Policy", "script-src 'self' "+server.URL+"; connect-src https:")
	if err := checkCSP(s, p); err != nil {
		t.Fatal(err)
	}
	if err := probeCDNSDK(s, info, s.SDK.URL); err != nil {
		t.Fatal("ordinary CDN incorrectly required CORS", err)
	}
	s.SDK.Integrity = true
	if probeCDNSDK(s, info, s.SDK.URL) == nil {
		t.Fatal("SRI without CORS accepted")
	}
	local := s
	local.PublicURL = server.URL + "/"
	if err := probeCDNSDK(local, info, local.SDK.URL); err != nil {
		t.Fatal("same-origin SRI incorrectly required CORS", err)
	}
	cors = true
	if err := probeCDNSDK(s, info, s.SDK.URL); err != nil {
		t.Fatal(err)
	}
	badGzip = true
	if probeCDNSDK(s, info, s.SDK.URL) == nil {
		t.Fatal("stale compressed CDN accepted")
	}
	badGzip, corrupt = false, true
	if probeCDNSDK(s, info, s.SDK.URL) == nil {
		t.Fatal("different SDK accepted")
	}
	// Historical deployments must probe their saved URL and digest after a package upgrade.
	historical := Snapshot{Site: s, SDK: SDKInfo{SHA256: hash([]byte("/* stale CDN object */"))}, SDKURL: s.SDK.URL}
	if err := probeCDNSDK(s, historical.SDK, snapshotSDKURL(historical)); err != nil {
		t.Fatal("historical CDN digest replaced by current package", err)
	}
}
