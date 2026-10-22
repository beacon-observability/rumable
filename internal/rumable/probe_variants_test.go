package rumable

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeRejectsBrowserCacheVariants(t *testing.T) {
	for _, failure := range []string{"none", "encoding-list", "warm-cache", "stale-bootstrap", "duplicate", "disabled"} {
		t.Run(failure, func(t *testing.T) {
			s := testSite()
			n := Snapshot{Site: s, Enabled: failure != "disabled", Script: s.assetPrefix() + "init.current.js"}
			browserHits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.Header().Set("Vary", "Accept-Encoding")
				browser := r.Header.Get("Accept-Encoding") == "gzip, deflate, br, zstd" && r.Header.Get("Cache-Control") != "max-age=0"
				if browser {
					if r.Header.Get("Cache-Control") != "" || r.URL.RawQuery != "" {
						t.Error("browser probe bypassed the actual cached URL")
					}
					browserHits++
				}
				tag := scriptTag(s, n.Script)
				if !n.Enabled {
					tag = ""
				}
				if browser {
					switch failure {
					case "encoding-list":
						tag = ""
					case "warm-cache":
						if browserHits >= 3 {
							tag = ""
						}
					case "stale-bootstrap":
						tag = scriptTag(s, s.assetPrefix()+"init.old.js")
					case "duplicate":
						tag += tag
					case "disabled":
						tag = scriptTag(s, n.Script)
					}
				}
				if n.Enabled && tag == "" {
					w.Header().Set("ETag", `"cached-html"`)
					w.Header().Set("Last-Modified", "Thu, 22 Oct 2026 06:44:37 GMT")
				}
				var body io.Writer = w
				if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
					w.Header().Set("Content-Encoding", "gzip")
					compressed := gzip.NewWriter(w)
					defer compressed.Close()
					body = compressed
				}
				fmt.Fprint(body, `<html><head>`+tag+`<script>window.businessLiteral="</head>";</script></head><body>Business content intact</body></html>`)
			}))
			defer server.Close()
			s.PublicURL = server.URL + "/"
			n.Site = s
			_, err := probePage(s, "/second.html", &n)
			if failure == "none" {
				if err != nil || browserHits != 3 {
					t.Fatalf("browser warm probes: hits=%d, error=%v", browserHits, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "browser cache variant") || !strings.Contains(err.Error(), `Content-Encoding="gzip"`) {
				t.Fatalf("cached browser failure was accepted or lacks evidence: %v", err)
			}
		})
	}
}

func TestBrowserProbeRejectsUnsupportedEncodingWithoutLeakingHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		w.Header().Set("Set-Cookie", "session=secret-cookie")
		io.WriteString(w, "not decoded HTML")
	}))
	defer server.Close()
	_, err := getPage(server.URL, http.Header{"Accept-Encoding": {browserAcceptEncoding}})
	if err == nil || !strings.Contains(err.Error(), "cannot verify encoded response") || strings.Contains(err.Error(), "secret-cookie") {
		t.Fatalf("unsupported response accepted or private header exposed: %v", err)
	}
}

// Models the Windows N1/R3 evidence: identity warms away a compressed failure.
// This verifies request ordering, not native IIS module execution.
func TestIISProbeDoesNotWarmAwayFirstCompressedFailure(t *testing.T) {
	s := testSite()
	s.Nginx, s.IIS = nil, &IIS{Site: "Website", Application: "/"}
	n := Snapshot{Site: s, Enabled: true, Script: s.assetPrefix() + "init.current.js"}
	warm, hits := false, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "text/html")
		if r.Header.Get("Accept-Encoding") == "identity" {
			warm = true
		}
		tag := ""
		if warm {
			tag = scriptTag(s, n.Script)
		}
		var body io.Writer = w
		if !warm {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			body = gz
		}
		fmt.Fprint(body, "<html><head>"+tag+"</head><body>Business</body></html>")
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	n.Site = s
	if _, err := preflightPage(s, "/"); err != nil {
		t.Fatal(err)
	}
	if warm {
		t.Fatal("preflight warmed the identity cache")
	}
	hits = 0
	if err := waitProbes(n); err == nil {
		t.Fatal("cold compressed failure accepted")
	}
	if warm || hits != 1 {
		t.Fatalf("first failure was warmed/retried: warm=%v hits=%d", warm, hits)
	}
}

func TestProbeNormalRefreshPrecedesIdentity(t *testing.T) {
	s := testSite()
	n := Snapshot{Site: s, Enabled: true, Script: s.assetPrefix() + "init.current.js"}
	identity := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.Header.Get("Accept-Encoding") == "identity" {
			identity = true
		}
		tag := scriptTag(s, n.Script)
		if r.Header.Get("Cache-Control") == "max-age=0" {
			tag = ""
		}
		fmt.Fprint(w, "<html><head>"+tag+"</head></html>")
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	n.Site = s
	_, err := probePage(s, "/", &n)
	if err == nil || !strings.Contains(err.Error(), "browser reload variant") || identity {
		t.Fatalf("reload failure missed/warmed: %v identity=%v", err, identity)
	}
}

// A normal refresh must not be accepted just because an unconditional GET is
// healthy. Model each validator independently and the combined browser request.
func TestIISConditionalReloadCannotReuseOldHTML(t *testing.T) {
	for _, failure := range []string{"none", "etag", "modified", "both", "stale-200"} {
		t.Run(failure, func(t *testing.T) {
			s := testSite()
			s.Nginx, s.IIS = nil, &IIS{Site: "Website", Application: "/"}
			n := Snapshot{Site: s, Enabled: true, Script: s.assetPrefix() + "init.current.js"}
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				e, m := r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
				key := "unconditional"
				if e != "" {
					key = "etag"
				}
				if m != "" {
					key = "modified"
				}
				if e != "" && m != "" {
					key = "both"
				}
				requests = append(requests, key)
				if key != "unconditional" && (r.Header.Get("Cache-Control") != "max-age=0" || !strings.Contains(r.Header.Get("Accept"), "text/html") || r.Header.Get("Accept-Encoding") != browserAcceptEncoding) {
					t.Error("not a browser conditional refresh")
				}
				if key == failure {
					w.WriteHeader(304)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				tag := scriptTag(s, n.Script)
				if failure == "stale-200" && key == "etag" {
					tag = ""
				}
				fmt.Fprint(w, "<html><head>"+tag+"</head><body>business</body></html>")
			}))
			defer server.Close()
			s.PublicURL = server.URL + "/"
			n.Site = s
			_, err := probePage(s, "/second.html", &n)
			if failure == "none" {
				if err != nil {
					t.Fatal(err)
				}
				if len(requests) < 4 || strings.Join(requests[:3], ",") != "etag,modified,both" {
					t.Fatal("validators did not precede unconditional warming", requests)
				}
			} else {
				var content *probeContentError
				if err == nil || !errors.As(err, &content) || strings.Contains(strings.Join(requests, ","), "unconditional") {
					t.Fatal("304/stale body accepted or warmed away", requests, err)
				}
			}
		})
	}
}

func TestIIS304IsNotRetriedIntoSuccess(t *testing.T) {
	s := testSite()
	s.Nginx, s.IIS = nil, &IIS{Site: "Website", Application: "/"}
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(304) }))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	err := waitProbes(Snapshot{Site: s, Enabled: true})
	if err == nil || hits != 1 {
		t.Fatal("304 must stop before a warm retry", hits, err)
	}
}

func TestIISExactLegacyRecoveryDoesNotRequireNewGuard(t *testing.T) {
	s := testSite()
	s.Nginx, s.IIS = nil, &IIS{Site: "Website", Application: "/"}
	n := Snapshot{Site: s, Enabled: true, Script: s.assetPrefix() + "init.legacy.js", Native: json.RawMessage(`{"rules":[],"cacheGuard":{"rules":[]}}`), legacyIISRecovery: true}
	conditionals := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			conditionals++
			w.WriteHeader(304)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><head>"+scriptTag(s, n.Script)+"</head></html>")
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	n.Site = s
	if _, err := probePage(s, "/", &n); err != nil || conditionals != 0 {
		t.Fatal("old exact recovery acquired a new prerequisite", err, conditionals)
	}
	if _, err := probePage(s, "/", nil); err == nil || conditionals != 1 {
		t.Fatal("explicit probe hid legacy 304", err, conditionals)
	}
}
