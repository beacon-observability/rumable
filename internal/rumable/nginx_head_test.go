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

// Seed the 0.1.9 snapshot format using its historical rendering branch. New
// enables cannot supply anchors; only snapshot preservation can render them.
func legacyHeadSnapshot(t *testing.T, root string, s Site, anchors map[string]string) (Snapshot, map[string][]byte) {
	t.Helper()
	n, assets, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	n.HeadAnchors = anchors
	n, _, err = renderDeploymentSDK(root, s, true, &n, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	return n, assets
}

type headFixture struct {
	root string
	s    Site
	st   State
}

func newHeadFixture(t *testing.T, loading string, legacy bool) *headFixture {
	t.Helper()
	binary := os.Getenv("RUMABLE_TEST_NGINX")
	if binary == "" {
		t.Skip("set RUMABLE_TEST_NGINX for native verification")
	}
	root, err := os.MkdirTemp("", "head-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", root+"/site.sock")
	}}
	old := httpClient
	httpClient = &http.Client{Transport: tr, Timeout: 5 * time.Second}
	t.Cleanup(func() { httpClient = old; tr.CloseIdleConnections() })
	s := testSite()
	s.SDK.Loading = loading
	s.PublicURL = "http://site.invalid/"
	s.ProbePaths = []string{"/"}
	s.Exclude = []string{"/excluded"}
	s.Nginx = &Nginx{Binary: binary, Args: []string{"-p", root + "/", "-c", root + "/nginx.conf"}, Reviewed: true}
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	f := &headFixture{root, s, st}
	f.write(t, "index", "<head>")
	f.write(t, "second", "<HEAD>")
	f.write(t, "attr", `<head data-check="yes">`)
	if legacy {
		legacySite := s
		legacySite.ProbePaths = []string{"/", "/attr"}
		n, assets := legacyHeadSnapshot(t, root, legacySite, map[string]string{"/": "<head>", "/attr": `<head data-check="yes">`})
		if err = deployAssets(root, s, assets); err != nil {
			t.Fatal(err)
		}
		for name, b := range n.Files {
			if err = atomicWrite(filepath.Join(siteDir(root, s), name), b, 0644); err != nil {
				t.Fatal(err)
			}
		}
		f.st.Current = n
		if err = saveState(root, f.st); err != nil {
			t.Fatal(err)
		}
	}
	dir := siteDir(root, s)
	config := fmt.Sprintf("pid %s/nginx.pid; error_log %s/error.log; events {} http { access_log off; default_type text/html; include %s/http.conf; server { listen unix:%s/site.sock; include %s/server.conf; location / { root %s; index index.html; try_files $uri $uri.html /index.html; include %s/location.conf; } } }", root, root, dir, root, dir, root, dir)
	if err = os.WriteFile(root+"/nginx.conf", []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(binary, s.Nginx.Args...).CombinedOutput(); err != nil {
		t.Fatalf("start: %v %s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command(binary, append(s.Nginx.Args, "-s", "quit")...).CombinedOutput(); err != nil {
			t.Errorf("quit: %v %s", err, out)
		}
	})
	return f
}
func (f *headFixture) write(t *testing.T, name, head string) string {
	t.Helper()
	body := "<!doctype html><html>" + head + `<title>fixture</title></head><body>fixture</body></html>`
	if err := os.WriteFile(f.root+"/"+name+".html", []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return body
}
func (f *headFixture) read(t *testing.T) {
	t.Helper()
	var err error
	f.st, err = loadState(f.root, f.s.ID)
	if err != nil {
		t.Fatal(err)
	}
}
func (f *headFixture) get(t *testing.T, path string) Page {
	t.Helper()
	p, err := getPage(f.s.PublicURL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func (f *headFixture) apply(t *testing.T, enabled bool) {
	t.Helper()
	if err := apply(f.root, f.st, f.s, enabled, "all", io.Discard); err != nil {
		t.Fatal(err)
	}
	f.read(t)
}

func TestNginxPlainHead(t *testing.T) {
	for _, loading := range []string{"async", "sync"} {
		t.Run(loading, func(t *testing.T) {
			f := newHeadFixture(t, loading, false)
			f.apply(t, true)
			for _, head := range []string{"<head>", "<HEAD>", "<HeAd>"} {
				f.write(t, "second", head)
				p := f.get(t, "second?fixture=yes")
				if strings.Count(string(p.Body), "data-rumable=") != 1 || !strings.Contains(string(p.Body), "<head>"+injectionTags(f.st.Current)) {
					t.Fatalf("unprobed %s was not injected", head)
				}
			}
			f.write(t, "excluded", "<head>")
			if strings.Contains(string(f.get(t, "excluded").Body), "data-rumable=") {
				t.Fatal("excluded page injected")
			}
			for _, head := range []string{`<head data-check="yes">`, `<head lang="en">`, "<head >", "<head\n>", "<head\t>", "<head/>"} {
				f.write(t, "attr", head)
				if strings.Contains(string(f.get(t, "attr").Body), "data-rumable=") {
					t.Fatal("unsupported head injected")
				}
				before := f.st.Current
				s := f.s
				s.ProbePaths = []string{"/", "/attr"}
				if err := apply(f.root, f.st, s, true, "all", io.Discard); err == nil {
					t.Fatalf("accepted unsupported %s", head)
				}
				f.read(t)
				if !snapshotsEqual(before, f.st.Current) {
					t.Fatal("failed preflight changed deployment")
				}
			}
			before := f.st.Current
			f.s.ProbePaths = []string{"/second"}
			f.apply(t, true)
			for name, b := range before.Files {
				if string(b) != string(f.st.Current.Files[name]) {
					t.Fatal("probePaths altered injection rules", name)
				}
			}
			if len(f.st.Current.HeadAnchors) != 0 {
				t.Fatal("new deployment saved path anchors")
			}
			f.apply(t, false)
			if strings.Contains(string(f.get(t, "second").Body), "data-rumable=") {
				t.Fatal("disable failed")
			}
			if err := rollback(f.root, f.st, io.Discard); err != nil {
				t.Fatal(err)
			}
			f.read(t)
			if strings.Count(string(f.get(t, "second").Body), "data-rumable=") != 1 {
				t.Fatal("rollback failed")
			}
		})
	}
}

func TestNginxLegacyHeadMigration(t *testing.T) {
	f := newHeadFixture(t, "sync", true)
	legacy := f.st.Current
	// Even removing the attributed path from probePaths cannot silently lose it.
	if err := apply(f.root, f.st, f.s, true, "all", io.Discard); err == nil || !strings.Contains(err.Error(), "plain opening <head>") {
		t.Fatalf("unsafe legacy migration accepted: %v", err)
	}
	f.read(t)
	if !snapshotsEqual(legacy, f.st.Current) {
		t.Fatal("failed migration changed saved state")
	}
	if err := verifyFiles(f.root, legacy); err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(f.get(t, "attr").Body), "data-rumable=") != 1 {
		t.Fatal("legacy injection stopped")
	}
	f.apply(t, false)
	if len(f.st.Current.HeadAnchors) == 0 {
		t.Fatal("disable lost legacy state")
	}
	if err := rollback(f.root, f.st, io.Discard); err != nil {
		t.Fatal(err)
	}
	f.read(t)
	if !snapshotsEqual(legacy, f.st.Current) {
		t.Fatal("rollback did not restore exact legacy snapshot")
	}
	f.write(t, "attr", "<head>")
	f.apply(t, true)
	if len(f.st.Current.HeadAnchors) != 0 {
		t.Fatal("migration retained path dependency")
	}
	if strings.Count(string(f.get(t, "attr").Body), "data-rumable=") != 1 {
		t.Fatal("migrated unprobed page missing injection")
	}
	// Historical rollback is exact; restore that historical page template too.
	f.write(t, "attr", `<head data-check="yes">`)
	if err := rollback(f.root, f.st, io.Discard); err != nil {
		t.Fatal(err)
	}
	f.read(t)
	if !snapshotsEqual(legacy, f.st.Current) {
		t.Fatal("migration history changed")
	}
}
