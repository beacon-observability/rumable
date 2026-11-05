package rumable

import (
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// The first attachment is disabled and carries bundled placeholder metadata.
// Its local assets must be reachable without treating that digest as CDN truth.
func TestAttachmentCDNResourceVerification(t *testing.T) {
	for _, loading := range []string{"async", "sync"} {
		t.Run(loading, func(t *testing.T) {
			var snapshot atomic.Value
			var public atomic.Value
			var cdnBody atomic.Value
			var requests atomic.Int64
			var offline atomic.Bool
			cdnBody.Store(syntheticCDNSDK("3.99.1"))
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := snapshot.Load().(Snapshot)
				var body []byte
				switch r.URL.Path {
				case "/cdn.js":
					requests.Add(1)
					if offline.Load() {
						http.Error(w, "offline", http.StatusServiceUnavailable)
						return
					}
					w.Header().Set("Content-Type", "application/javascript")
					w.Header().Set("Access-Control-Allow-Origin", "*")
					body = cdnBody.Load().([]byte)
				case "/":
					w.Header().Set("Content-Type", "text/html")
					tags := ""
					if n.Enabled {
						tags = injectionTags(n)
					}
					body = []byte("<html><head>" + tags + "</head><body>fixture</body></html>")
				default:
					w.Header().Set("Content-Type", "application/javascript")
					key := "public/" + n.Site.ID + "/" + strings.TrimPrefix(r.URL.Path, n.Site.assetPrefix())
					body = public.Load().(map[string][]byte)[key]
					if body == nil {
						http.NotFound(w, r)
						return
					}
				}
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
			old := httpClient
			httpClient = server.Client()
			defer func() { httpClient = old }()
			s := testSite()
			s.PublicURL = server.URL + "/"
			s.SDK = SDKConfig{Source: "cdn", URL: server.URL + "/cdn.js", Integrity: true, Loading: loading}
			root := t.TempDir()
			st, err := prepare(root, s)
			if err != nil {
				t.Fatal(err)
			}
			_, files, err := render(root, s, false)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Store(st.Current)
			public.Store(files)
			if st.Current.SDK.SHA256 == hash(cdnBody.Load().([]byte)) {
				t.Fatal("fixture must differ from bundled SDK")
			}
			if err = waitAttachment(st.Current); err != nil {
				t.Fatal("first attachment", err)
			}
			if requests.Load() != 0 {
				t.Fatal("disabled attachment contacted CDN")
			}
			// A disconnected CDN must not prevent verifying disabled local mappings.
			offline.Store(true)
			if err = waitAttachment(st.Current); err != nil {
				t.Fatal("offline attachment", err)
			}
			offline.Store(false)
			// Keep local integrity checks: neither SDK nor bootstrap corruption is ignored.
			for _, name := range []string{"public/" + s.ID + "/sdk." + st.Current.SDK.SHA256 + ".js", "public/" + s.ID + "/" + strings.TrimPrefix(st.Current.Script, s.assetPrefix())} {
				corrupt := map[string][]byte{}
				for k, v := range files {
					corrupt[k] = v
				}
				corrupt[name] = []byte("corrupted")
				public.Store(corrupt)
				if err = waitAttachment(st.Current); err == nil || !strings.Contains(err.Error(), "script checksum mismatch") {
					t.Fatal("corrupt retained asset accepted", name, err)
				}
				public.Store(files)
			}
			resolved, err := resolveCDNSDK(s)
			if err != nil {
				t.Fatal(err)
			}
			active, activeFiles, err := renderDeploymentSDK(root, s, true, nil, false, resolved)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Store(active)
			public.Store(activeFiles)
			before := requests.Load()
			if err = waitAttachment(active); err != nil {
				t.Fatal("active attachment", err)
			}
			if requests.Load() <= before {
				t.Fatal("active attachment skipped CDN verification")
			}
			cdnBody.Store(syntheticCDNSDK("3.99.2"))
			if err = waitAttachment(active); err == nil || !strings.Contains(err.Error(), "script checksum mismatch") {
				t.Fatal("active CDN drift accepted", err)
			}
			// Existing disabled CDN snapshots retain the actually deployed SDK locally.
			active.Enabled = false
			snapshot.Store(active)
			offline.Store(true)
			if err = waitAttachment(active); err != nil {
				t.Fatal("historical disabled attachment", err)
			}
		})
	}
}
