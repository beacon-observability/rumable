package rumable

import (
	"compress/gzip"
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
				browser := r.Header.Get("Accept-Encoding") == "gzip, deflate, br, zstd"
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
