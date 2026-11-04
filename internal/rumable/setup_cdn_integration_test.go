package rumable

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// Run on Linux with a real Nginx: exercise the complete first installation,
// including attachment and its rollback, instead of starting at enable.
func TestNginxFirstSetupCDN(t *testing.T) {
	binary := os.Getenv("RUMABLE_TEST_NGINX")
	if runtime.GOOS != "linux" || binary == "" {
		t.Skip("requires Linux and RUMABLE_TEST_NGINX")
	}
	for _, tc := range []struct {
		name, loading          string
		integrity, unavailable bool
	}{
		{"async", "async", false, false}, {"async-sri", "async", true, false},
		{"sync", "sync", false, false}, {"sync-sri", "sync", true, false},
		{"unavailable", "async", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := syntheticCDNSDK("3.99.1")
			var cdnRequests atomic.Int64
			cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				cdnRequests.Add(1)
				if tc.unavailable {
					http.Error(w, "unavailable", 503)
					return
				}
				w.Header().Set("Content-Type", "application/javascript")
				w.Header().Set("Access-Control-Allow-Origin", "*")
				if r.Header.Get("Accept-Encoding") == "gzip" {
					w.Header().Set("Content-Encoding", "gzip")
					z := gzip.NewWriter(w)
					defer z.Close()
					z.Write(body)
				} else {
					w.Write(body)
				}
			}))
			defer cdn.Close()
			oldClient := httpClient
			httpClient = cdn.Client()
			defer func() { httpClient = oldClient }()
			root, err := os.MkdirTemp("", "first-cdn-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(root)
			if err = os.Chmod(root, 0755); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			addr := listener.Addr().String()
			listener.Close()
			conf := filepath.Join(root, "nginx.conf")
			original := []byte(fmt.Sprintf("pid %s/nginx.pid; error_log %s/error.log; events {} http { access_log off; default_type text/html; server { listen %s; server_name 127.0.0.1; location / { root %s; index index.html; } } }", root, root, addr, root))
			if err = os.WriteFile(conf, original, 0644); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, "index.html"), []byte("<html><head><title>fixture</title></head><body>fixture</body></html>"), 0644); err != nil {
				t.Fatal(err)
			}
			s := testSite()
			s.PublicURL = "http://" + addr + "/"
			s.SDK = SDKConfig{Source: "cdn", URL: cdn.URL + "/sdk.js", Loading: tc.loading, Integrity: tc.integrity}
			s.Nginx = &Nginx{Binary: binary, Args: []string{"-p", root + "/", "-c", conf}}
			if output, err := exec.Command(binary, s.Nginx.Args...).CombinedOutput(); err != nil {
				t.Fatalf("start: %v %s", err, output)
			}
			defer exec.Command(binary, append(s.Nginx.Args, "-s", "quit")...).Run()
			// Match the snapshot left by a failed 0.1.10 first attachment as well as
			// a fresh installation; both enter setup with disabled bundled metadata.
			if tc.name == "sync-sri" {
				if _, err = prepare(root, s); err != nil {
					t.Fatal(err)
				}
			}
			err = setupSite(root, s, "", true, strings.NewReader(""), io.Discard)
			st, readErr := loadState(root, s.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if tc.unavailable {
				if err == nil || !strings.Contains(err.Error(), "CDN SDK:") {
					t.Fatal("unavailable CDN accepted", err)
				}
				if st.Current.Enabled {
					t.Fatal("failed preflight enabled RUM")
				}
				a, e := loadAttachment(root, s)
				if e != nil || a.Phase != "attached" {
					t.Fatal("failure occurred before attachment completed", e)
				}
			} else {
				if err != nil {
					t.Fatal("first setup", err)
				}
				if !st.Current.Enabled || st.Current.SDK.Version != "3.99.1" || st.Current.SDK.SHA256 != hash(body) {
					t.Fatal("wrong active CDN metadata")
				}
				if cdnRequests.Load() == 0 {
					t.Fatal("setup skipped real CDN verification")
				}
				if err = waitAttachment(st.Current); err != nil {
					t.Fatal("active attachment", err)
				}
				if err = setupSite(root, s, "", true, strings.NewReader(""), io.Discard); err != nil {
					t.Fatal("repeat setup", err)
				}
				cdn.Close()
				st, err = loadState(root, s.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err = apply(root, st, s, false, "all", io.Discard); err != nil {
					t.Fatal("offline disable", err)
				}
				st, err = loadState(root, s.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err = undoAttachment(root, st, true, false, strings.NewReader(""), io.Discard); err != nil {
				t.Fatal("undo", err)
			}
			restored, err := os.ReadFile(conf)
			if err != nil || !bytes.Equal(restored, original) {
				t.Fatal("original server config not restored", err)
			}
		})
	}
}
