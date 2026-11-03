package rumable

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Real Nginx is required: map escaping, internal redirects and sub_filter's
// case-insensitive matching cannot be verified by a Go string replacement.
func TestNginxHeadAnchors(t *testing.T) {
	binary := os.Getenv("RUMABLE_TEST_NGINX")
	if binary == "" {
		t.Skip("set RUMABLE_TEST_NGINX for native head injection verification")
	}
	root, err := os.MkdirTemp("", "head-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", root+"/site.sock")
	}}
	oldClient := httpClient
	httpClient = &http.Client{Transport: transport, Timeout: 5 * time.Second}
	defer func() { httpClient = oldClient; transport.CloseIdleConnections() }()
	s := testSite()
	s.PublicURL = "http://site.invalid/"
	s.SDK.Loading = "sync"
	s.ProbePaths = []string{"/", "/en", "/space", "/quoted", "/multiline"}
	s.Exclude = append(s.Exclude, "/excluded")
	conf := filepath.Join(root, "nginx.conf")
	s.Nginx = &Nginx{Binary: binary, Args: []string{"-p", root + "/", "-c", conf}, Reviewed: true}
	heads := map[string]string{
		"/":          `<head lang="en">`,
		"/en":        `<HeAd data-fixture="CaseSensitive">`,
		"/space":     `<head >`,
		"/quoted":    `<head title='a>b' data-fixture="a\b &amp; c">`,
		"/multiline": "<head\n data-fixture=\"yes\"\t>",
		"/unprobed":  "<head>",
		"/excluded":  "<head>",
	}
	bodies := map[string]string{}
	writePage := func(path, head string) {
		t.Helper()
		// Other anchor variants in inert text must never trigger a second injection.
		body := "<!doctype html><html>" + head + `<script>window.fixture='<head ><head lang="en">';</script><title>fixture</title></head><body><header>fixture</header></body></html>`
		bodies[path] = body
		name := strings.TrimPrefix(path, "/")
		if name == "" {
			name = "index"
		}
		if err := os.WriteFile(filepath.Join(root, name+".html"), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for path, head := range heads {
		writePage(path, head)
	}
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	dir := siteDir(root, s)
	config := fmt.Sprintf("pid %s/nginx.pid; error_log %s/error.log; events {} http { access_log off; default_type text/html; include %s/http.conf; server { listen unix:%s/site.sock; include %s/server.conf; location / { root %s; index index.html; try_files $uri $uri.html /index.html; include %s/location.conf; } } }", root, root, dir, root, dir, root, dir)
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
		got, err := loadState(root, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if err := apply(root, st, s, true, "all", io.Discard); err != nil {
		t.Fatal("enable:", err)
	}
	st = read()
	verify := func(path string) {
		t.Helper()
		p, err := getPage(s.PublicURL+strings.TrimPrefix(path, "/"), nil)
		if err != nil {
			t.Fatal(err)
		}
		want := bodies[path]
		if path != "/excluded" {
			want = strings.Replace(want, heads[path], heads[path]+injectionTags(st.Current), 1)
		}
		if string(p.Body) != want {
			t.Fatalf("%s: attributes, insertion position or original bytes changed", path)
		}
	}
	for path := range heads {
		verify(path)
	}
	p, err := getPage(s.PublicURL+"en?lang=fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(p.Body), heads["/en"]+injectionTags(st.Current)) {
		t.Fatal("query changed anchor selection")
	}
	var output strings.Builder
	if err := apply(root, st, s, true, "all", &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Already in requested state") {
		t.Fatal("not idempotent")
	}
	// A new fixed template must fail probe, then be learned on explicit enable.
	oldHead := heads["/en"]
	heads["/en"] = `<head data-fixture="new-template">`
	writePage("/en", heads["/en"])
	if _, err := probePages(s, []string{"/en"}, &st.Current); err == nil || !strings.Contains(err.Error(), "anchor changed") {
		t.Fatalf("missing drift diagnostic: %v", err)
	}
	if err := apply(root, st, s, true, "all", io.Discard); err != nil {
		t.Fatal("refresh:", err)
	}
	st = read()
	verify("/en")
	// Restore the original template before rolling back its matching snapshot.
	heads["/en"] = oldHead
	writePage("/en", oldHead)
	if err := rollback(root, st, io.Discard); err != nil {
		t.Fatal("rollback:", err)
	}
	st = read()
	verify("/en")
	// Earlier tag-shaped text must fail before any state/configuration mutation.
	before := st.Current
	if err := os.WriteFile(filepath.Join(root, "en.html"), []byte("<!-- "+oldHead+" -->"+bodies["/en"]), 0644); err != nil {
		t.Fatal(err)
	}
	if err := apply(root, st, s, true, "all", io.Discard); err == nil {
		t.Fatal("unsafe earlier anchor accepted")
	}
	if !snapshotsEqual(read().Current, before) {
		t.Fatal("failed preflight changed state")
	}
	if err := verifyFiles(root, before); err != nil {
		t.Fatal(err)
	}
	writePage("/en", oldHead)
	if err := apply(root, read(), s, false, "all", io.Discard); err != nil {
		t.Fatal("disable:", err)
	}
	st = read()
	if st.Current.Enabled || st.Current.HeadAnchors["/en"] != oldHead {
		t.Fatal("disable lost snapshot anchor")
	}
	for path, body := range bodies {
		p, err := getPage(s.PublicURL+strings.TrimPrefix(path, "/"), nil)
		if err != nil || string(p.Body) != body {
			t.Fatalf("disable changed %s: %v", path, err)
		}
	}
	t.Log("native Nginx: attributes, multiline/quotes, per-path anchors, internal redirects, query, single injection, exclusion, drift, refresh, idempotence, rollback and disable passed")
}
