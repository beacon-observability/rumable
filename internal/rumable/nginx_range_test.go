package rumable

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Optional native integration test: uses the actual rendered configuration and
// Nginx header/body filter chain, which a Go string/configuration test cannot model.
func TestNginxRangeResponses(t *testing.T) {
	binary := os.Getenv("RUMABLE_TEST_NGINX")
	if binary == "" {
		t.Skip("set RUMABLE_TEST_NGINX to a native Nginx with sub_filter and PCRE")
	}
	root, err := os.MkdirTemp("", "rng-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	body := []byte("<html><head><title>Range fixture</title></head><body>" + strings.Repeat("range-fixture ", 30) + "</body></html>")
	if err := os.WriteFile(filepath.Join(root, "static.html"), body, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plain.txt"), body, 0644); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("ETag", `"range-fixture"`)
		http.ServeContent(w, r, "fixture.html", time.Unix(1700000000, 0), bytes.NewReader(body))
	}))
	defer upstream.Close()
	var config strings.Builder
	fmt.Fprintf(&config, "pid %s/nginx.pid; error_log %s/error.log; events {} http { access_log off; default_type text/html; sub_filter_types *;\n", root, root)
	for _, enabled := range []bool{false, true} {
		s := testSite()
		s.ID = fmt.Sprintf("range-%t", enabled)
		n, _, err := render(root, s, enabled)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, s.ID)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for name, data := range n.Files {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
				t.Fatal(err)
			}
		}
		fmt.Fprintf(&config, "include %s/http.conf; server { listen unix:%s/%t.sock; root %s; location / { include %s/location.conf; } location = /plain.txt { default_type text/plain; include %s/location.conf; } location = /proxy { proxy_pass %s; include %s/location.conf; } }\n", dir, root, enabled, root, dir, dir, upstream.URL, dir)
	}
	config.WriteString("}\n")
	conf := filepath.Join(root, "nginx.conf")
	if err := os.WriteFile(conf, []byte(config.String()), 0644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(binary, "-p", root+"/", "-c", conf).CombinedOutput(); err != nil {
		t.Fatalf("Nginx start: %v %s", err, output)
	}
	defer func() {
		if output, err := exec.Command(binary, "-p", root+"/", "-c", conf, "-s", "quit").CombinedOutput(); err != nil {
			t.Errorf("Nginx cleanup: %v %s", err, output)
		}
	}()
	type response struct {
		status int
		header http.Header
		body   []byte
	}
	fetch := func(enabled bool, path, method string, headers http.Header) response {
		t.Helper()
		transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", fmt.Sprintf("%s/%t.sock", root, enabled))
		}, DisableCompression: true}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		request, err := http.NewRequest(method, "http://fixture.invalid"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header = headers
		r, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response{r.StatusCode, r.Header, data}
	}
	for _, path := range []string{"/static.html", "/proxy"} {
		original := fetch(false, path, "GET", nil)
		for _, tc := range []struct {
			name, span, ifRange string
			status              int
		}{
			{"single", "bytes=0-31", "", 206}, {"suffix", "bytes=-16", "", 206},
			{"multiple", "bytes=0-7,20-27", "", 206}, {"unsatisfiable", "bytes=999999-", "", 416},
			{"invalid", "invalid", "", 0}, {"if-range-match", "bytes=0-31", original.header.Get("ETag"), 206},
			{"if-range-miss", "bytes=0-31", `"different"`, 200},
			{"if-range-date", "bytes=0-31", original.header.Get("Last-Modified"), 206},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				// Existing cache protection disables static ETags. A strong If-Range
				// therefore falls back to a full, uninjected response, as HTTP requires.
				if path == "/static.html" && tc.name == "if-range-match" {
					tc.status = 200
				}
				h := http.Header{"Range": {tc.span}}
				if tc.ifRange != "" {
					h.Set("If-Range", tc.ifRange)
				}
				before := fetch(false, path, "GET", h)
				if path == "/static.html" && tc.name == "if-range-match" {
					before = fetch(false, path, "GET", nil)
				}
				after := fetch(true, path, "GET", h)
				if before.status != after.status || tc.status != 0 && after.status != tc.status {
					t.Fatalf("status changed: %d -> %d, expected %d", before.status, after.status, tc.status)
				}
				if bytes.Contains(after.body, []byte("data-rumable=")) {
					t.Fatal("Range response was injected")
				}
				for _, key := range []string{"Content-Length", "Content-Range", "ETag", "Last-Modified"} {
					if path == "/static.html" && key == "ETag" {
						continue
					}
					if before.header.Get(key) != after.header.Get(key) {
						t.Fatalf("%s changed: %q -> %q", key, before.header.Get(key), after.header.Get(key))
					}
				}
				// Multipart boundaries are generated per request; compare payload separately.
				if tc.name == "multiple" {
					for _, fragment := range [][]byte{body[:8], body[20:28]} {
						if !bytes.Contains(after.body, fragment) {
							t.Fatal("multipart payload changed")
						}
					}
				} else if !bytes.Equal(before.body, after.body) {
					t.Fatal("range response bytes changed")
				}
			})
		}
		ordinary := fetch(true, path, "GET", nil)
		if ordinary.status != 200 || bytes.Count(ordinary.body, []byte("data-rumable=")) != 1 {
			t.Fatal("ordinary GET injection regressed")
		}
		for _, method := range []string{"GET", "HEAD"} {
			for _, h := range []http.Header{
				{"If-None-Match": {original.header.Get("ETag")}},
				{"If-Modified-Since": {original.header.Get("Last-Modified")}},
			} {
				got := fetch(true, path, method, h)
				// Proxy conditional headers are intentionally forwarded in this minimal
				// fixture. Production attachment clears them; static handling is under test.
				if path == "/static.html" && (got.status != 200 || got.header.Get("ETag") != "" || got.header.Get("Content-Length") != "") {
					t.Fatalf("static %s stale metadata: %+v", method, got)
				}
			}
		}
	}
	plain := fetch(true, "/plain.txt", "GET", nil)
	if !bytes.Equal(plain.body, body) {
		t.Fatal("non-HTML response changed")
	}
	t.Log("actual Nginx: static/proxy range matrix, ordinary GET/HEAD and non-HTML passed")
}
