package rumable

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
