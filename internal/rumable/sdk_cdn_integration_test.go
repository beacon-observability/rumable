package rumable

import (
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
	"sync/atomic"
	"testing"
)

func TestCDNEnableUpgradeRollback(t *testing.T) {
	binary := os.Getenv("RUMABLE_TEST_NGINX")
	if binary == "" {
		t.Skip("set RUMABLE_TEST_NGINX for native CDN deployment lifecycle verification")
	}
	root, err := os.MkdirTemp("", "cdn-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	var cdnBody atomic.Value
	cdnBody.Store(syntheticCDNSDK("3.99.1"))
	cdn := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(cdnBody.Load().([]byte))
	}))
	defer cdn.Close()
	oldClient := httpClient
	client := cdn.Client()
	transport := client.Transport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "site.invalid:80" {
			return (&net.Dialer{}).DialContext(ctx, "unix", root+"/site.sock")
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	client.Transport = transport
	httpClient = client
	defer func() { httpClient = oldClient; transport.CloseIdleConnections() }()
	s := testSite()
	s.PublicURL = "http://site.invalid/"
	s.ProbePaths = []string{"/index.html"}
	s.SDK = SDKConfig{Source: "cdn", URL: cdn.URL + "/sdk.js", Integrity: true}
	conf := filepath.Join(root, "nginx.conf")
	s.Nginx = &Nginx{Binary: binary, Args: []string{"-p", root + "/", "-c", conf}, Reviewed: true}
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	page := []byte("<html><head><title>Synthetic CDN lifecycle</title></head><body>fixture</body></html>")
	if err := os.WriteFile(filepath.Join(root, "index.html"), page, 0644); err != nil {
		t.Fatal(err)
	}
	dir := siteDir(root, s)
	config := fmt.Sprintf("pid %s/nginx.pid; error_log %s/error.log; events {} http { access_log off; default_type text/html; include %s/http.conf; server { listen unix:%s/site.sock; include %s/server.conf; location / { root %s; include %s/location.conf; } } }", root, root, dir, root, dir, root, dir)
	if err := os.WriteFile(conf, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, s.Nginx.Args...).CombinedOutput(); err != nil {
		t.Fatalf("start: %v %s", err, out)
	}
	defer func() {
		if out, err := exec.Command(binary, append(s.Nginx.Args, "-s", "quit")...).CombinedOutput(); err != nil {
			t.Errorf("cleanup: %v %s", err, out)
		}
	}()
	read := func() State {
		t.Helper()
		st, err := loadState(root, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		return st
	}
	check := func(version string) {
		t.Helper()
		st = read()
		if st.Current.SDK.Version != version || st.Current.SDK.SHA256 != hash(cdnBody.Load().([]byte)) {
			t.Fatalf("wrong CDN snapshot: %+v", st.Current.SDK)
		}
		if _, err := probePages(st.Current.Site, st.Current.Site.ProbePaths, &st.Current); err != nil {
			t.Fatal(err)
		}
		if err := verifyFilesAssets(root, st.Current); err != nil {
			t.Fatal(err)
		}
	}
	if err := waitAttachment(st.Current); err != nil {
		t.Fatal("prepared CDN attachment:", err)
	}
	if err := apply(root, st, s, true, "all", io.Discard); err != nil {
		t.Fatal("first enable:", err)
	}
	check("3.99.1")
	oldSnapshot := st.Current
	// Same settings and bytes are idempotent; upgrading the remote bundle is not.
	var output strings.Builder
	if err := apply(root, st, s, true, "all", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Already in requested state") {
		t.Fatal("repeat enable was not idempotent", output.String())
	}
	cdnBody.Store(syntheticCDNSDK("3.99.2"))
	if err := apply(root, read(), s, true, "all", io.Discard); err != nil {
		t.Fatal("upgrade:", err)
	}
	check("3.99.2")
	cdnBody.Store([]byte("not a supported SDK"))
	if err := apply(root, read(), s, true, "all", io.Discard); err == nil {
		t.Fatal("invalid CDN content was deployed")
	}
	if read().Current.SDK.Version != "3.99.2" {
		t.Fatal("failed preflight replaced the previous deployment")
	}
	cdnBody.Store(syntheticCDNSDK("3.99.2"))
	// Rollback must not silently adopt the new bytes at a historical URL.
	if err := probeCDNSDK(s, oldSnapshot.SDK, oldSnapshot.SDKURL); err == nil {
		t.Fatal("old snapshot accepted new CDN bytes")
	}
	cdnBody.Store(syntheticCDNSDK("3.99.1"))
	if err := rollback(root, read(), io.Discard); err != nil {
		t.Fatal("rollback:", err)
	}
	check("3.99.1")
	s.SDK.Loading = "sync"
	if err := apply(root, read(), s, true, "all", io.Discard); err != nil {
		t.Fatal("sync enable:", err)
	}
	check("3.99.1")
	// Offline disable retains the actual deployed SDK metadata, not the bundled version.
	cdn.Close()
	if err := apply(root, read(), s, false, "all", io.Discard); err != nil {
		t.Fatal("offline disable:", err)
	}
	st = read()
	if st.Current.Enabled || st.Current.SDK.Version != "3.99.1" {
		t.Fatal("disable lost selected CDN version")
	}
	if err := waitAttachment(st.Current); err != nil {
		t.Fatal("offline disabled attachment:", err)
	}
	if _, err := probePages(st.Current.Site, st.Current.Site.ProbePaths, &st.Current); err != nil {
		t.Fatal(err)
	}
	t.Log("native Nginx + local TLS CDN: enable, idempotence, upgrade, saved digest, rollback, sync/SRI and offline disable passed")
}
