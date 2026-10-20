package rumable

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testSite() Site {
	return Site{ID: "test", PublicURL: "https://example.com/", ProbePaths: []string{"/"}, Include: []string{"/"}, Exclude: []string{"/api/"}, Nginx: &Nginx{Binary: "nginx", Reviewed: true}, RUM: RUM{ApplicationID: "app", DatakitOrigin: "https://rum.example.com"}}
}

func TestConfigurationBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Site)
		valid  bool
	}{
		{"datakit", func(s *Site) {}, true},
		{"openway", func(s *Site) {
			s.RUM.DatakitOrigin = ""
			s.RUM.Site = "https://openway.example.com"
			s.RUM.ClientToken = "browser-token"
		}, true},
		{"mixed", func(s *Site) { s.RUM.Site = "https://openway.example.com" }, false},
		{"credential-url", func(s *Site) { s.RUM.DatakitOrigin = "https://user:secret@rum.example.com" }, false},
		{"mixed-content", func(s *Site) { s.RUM.DatakitOrigin = "http://rum.example.com" }, false},
		{"traversal", func(s *Site) { s.ID = "../x" }, false},
		{"sample-rate", func(s *Site) { v := 101.0; s.RUM.SessionSampleRate = &v }, false},
		{"command-injection", func(s *Site) { s.Nginx.Args = []string{"-s", "stop"} }, false},
		{"out-of-scope-probe", func(s *Site) { s.ProbePaths = []string{"/api/"} }, false},
		{"empty-query-origin", func(s *Site) { s.RUM.DatakitOrigin += "?" }, false},
		{"encoded-origin", func(s *Site) { s.RUM.DatakitOrigin += "/%2f" }, false},
		{"invalid-port", func(s *Site) { s.RUM.DatakitOrigin += ":65536" }, false},
		{"empty-port", func(s *Site) { s.PublicURL = "https://example.com:/" }, false},
		{"empty-query-page", func(s *Site) { s.PublicURL += "?" }, false},
		{"fragment-origin", func(s *Site) { s.RUM.DatakitOrigin += "#" }, false},
		{"query-app-id", func(s *Site) { s.RUM.ApplicationID = "app&other=true" }, false},
		{"query-token", func(s *Site) {
			s.RUM.DatakitOrigin = ""
			s.RUM.Site = "https://open.example.com"
			s.RUM.ClientToken = "token+suffix"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testSite()
			tc.change(&s)
			if (s.validate() == nil) != tc.valid {
				t.Fatalf("validation mismatch: %v", s.validate())
			}
		})
	}
}

func TestTOMLConfiguration(t *testing.T) {
	name := filepath.Join(t.TempDir(), "rumable.toml")
	var out strings.Builder
	if err := Run([]string{"init", "--config", name}, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(name, "example"); err != nil {
		t.Fatal("generated configuration cannot be loaded", err)
	}
	if err := Run([]string{"init", "--config", name}, &out); err == nil {
		t.Fatal("init overwrote existing configuration")
	}
	config := strings.Replace(exampleConfig, `binary = "/usr/sbin/nginx"`, `binary = 'C:\nginx\nginx.exe'`, 1) + `
# A second site must keep its own reporting mode and sample rate.
[[sites]]
id = "intranet"
publicURL = "https://intranet.example.com/"
probePaths = [
  "/", # arrays may span multiple lines
]
includePaths = ["/"]
[sites.iis]
site = "Default Web Site"
application = "/"
allowCompressionChanges = true
[sites.rum]
applicationId = "intranet-app"
datakitOrigin = "https://datakit.example.com"
service = "browser \"quoted\" \u4e2d\u6587"
sessionSampleRate = 0
sessionReplayEnabled = true
sessionReplaySampleRate = 12.5
`
	if err := os.WriteFile(name, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	nginx, err := loadConfig(name, "example")
	if err != nil {
		t.Fatal(err)
	}
	iis, err := loadConfig(name, "intranet")
	if err != nil {
		t.Fatal(err)
	}
	if nginx.Nginx == nil || nginx.Nginx.Binary != `C:\nginx\nginx.exe` || nginx.RUM.ClientToken == "" || nginx.RUM.SessionSampleRate == nil || *nginx.RUM.SessionSampleRate != 100 || nginx.RUM.replayRate() != 0 {
		t.Fatalf("OpenWay or literal Windows path did not round trip: %+v", nginx)
	}
	if iis.IIS == nil || !iis.IIS.AllowCompressionChanges || iis.RUM.DatakitOrigin != "https://datakit.example.com" || iis.RUM.ClientToken != "" || iis.RUM.Service != "browser \"quoted\" 中文" || iis.RUM.SessionSampleRate == nil || *iis.RUM.SessionSampleRate != 0 || iis.RUM.replayRate() != 12.5 {
		t.Fatalf("DataKit or explicit zero sample rate did not round trip: %+v", iis)
	}
	for _, tc := range []struct{ name, config string }{
		{"unknown-root", "unexpected = true\n" + exampleConfig},
		{"unknown-nested", exampleConfig + "sampleRate = 50\n"},
		{"duplicate-key", exampleConfig + "applicationId = 'other'\n"},
		{"duplicate-site", exampleConfig + strings.SplitN(exampleConfig, "\n", 2)[1]},
		{"mixed-reporting", exampleConfig + "datakitOrigin = 'https://datakit.example.com'\n"},
		{"wrong-type", strings.Replace(exampleConfig, "attachmentReviewed = false", `attachmentReviewed = "false"`, 1)},
		{"nan", strings.Replace(exampleConfig, "sessionSampleRate = 100", "sessionSampleRate = nan", 1)},
		{"infinity", strings.Replace(exampleConfig, "sessionSampleRate = 100", "sessionSampleRate = inf", 1)},
		{"replay-negative", strings.Replace(exampleConfig, "sessionReplaySampleRate = 100", "sessionReplaySampleRate = -1", 1)},
		{"replay-too-large", strings.Replace(exampleConfig, "sessionReplaySampleRate = 100", "sessionReplaySampleRate = 101", 1)},
		{"replay-nan", strings.Replace(exampleConfig, "sessionReplaySampleRate = 100", "sessionReplaySampleRate = nan", 1)},
		{"replay-infinity", strings.Replace(exampleConfig, "sessionReplaySampleRate = 100", "sessionReplaySampleRate = inf", 1)},
		{"replay-wrong-type", strings.Replace(exampleConfig, "sessionReplayEnabled = false", "sessionReplayEnabled = 'true'", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(name, []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadConfig(name, "example"); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	for _, tc := range []struct{ file, id string }{{"nginx", "website"}, {"iis", "intranet"}} {
		if _, err := loadConfig(filepath.Join("..", "..", "examples", tc.file+".toml"), tc.id); err != nil {
			t.Fatal("example configuration cannot be loaded", err)
		}
	}
}

func TestSessionReplayDefaultsAndState(t *testing.T) {
	s := testSite()
	if s.RUM.replayRate() != 0 {
		t.Fatal("existing configuration opted into recording")
	}
	s.RUM.SessionReplayEnabled = true
	if s.RUM.replayRate() != 100 {
		t.Fatal("enabled replay did not default to 100 percent")
	}
	for _, rate := range []float64{0, 12.5, 100} {
		s.RUM.SessionReplaySampleRate = &rate
		root := t.TempDir()
		if _, err := prepare(root, s); err != nil {
			t.Fatal(err)
		}
		st, err := loadState(root, s.ID)
		if err != nil || st.Current.Site.RUM.replayRate() != rate {
			t.Fatalf("replay setting lost during state round trip: %v", err)
		}
	}
}

func TestHistoricalSDKConfigurationRemainsRecoverable(t *testing.T) {
	s := testSite()
	// This identifier passed earlier validation, but the current SDK query
	// parameter guard rejects it for a new deployment.
	s.RUM.ApplicationID = "legacy+app"
	if s.validate() == nil {
		t.Fatal("unsafe identifier accepted for a new deployment")
	}
	root := t.TempDir()
	old, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	st, err := loadState(root, s.ID)
	if err != nil {
		t.Fatal("historical SDK settings blocked recovery", err)
	}
	if !snapshotsEqual(old.Current, st.Current) {
		t.Fatal("historical snapshot was rewritten")
	}
	if err := verifyFiles(root, st.Current); err != nil {
		t.Fatal(err)
	}
	s.RUM.ApplicationID = "fixed-app"
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	if !compatibleTarget(st.Current.Site, s) {
		t.Fatal("correcting SDK settings changed deployment identity")
	}
}

func TestCSPPolicies(t *testing.T) {
	s := testSite()
	for _, tc := range []struct {
		policy string
		allow  bool
	}{
		{"", true},
		{"default-src 'self'; connect-src https://rum.example.com", true},
		{"script-src 'nonce-abc'", false},
		{"script-src 'self' 'nonce-abc' 'strict-dynamic'", false},
		{"script-src 'self'; connect-src 'none'", false},
		{"script-src 'self'; connect-src https://*.example.com", true},
		{"script-src 'self'; connect-src https://rum.example.com.evil", false},
		{"script-src 'self'; connect-src https://rum.example.com, script-src 'none'", false},
		{"sandbox allow-scripts; default-src *", false},
		{"require-trusted-types-for 'script'", false},
		{"script-src https://example.com/__rumable__/test/init.js; connect-src *", false},
		{"script-src https://example.com/__rumable__/test/; connect-src https://rum.example.com/v1/write/rum", true},
		{"script-src https://example.com/__rumable__/tes; connect-src *", false},
		{"script-src https://example.com:*/__rumable__/test/; connect-src https://rum.example.com:*/v1/write/", true},
		{"script-src https://*/__rumable__/test/; connect-src *", true},
		{"script-src https://example.com/__rumable__%2ftest/; connect-src *", false},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			p := Page{Header: http.Header{"Content-Security-Policy": []string{tc.policy}}}
			if (checkCSP(s, p) == nil) != tc.allow {
				t.Fatalf("unexpected CSP decision: %v", checkCSP(s, p))
			}
		})
	}
	p := Page{Header: http.Header{}, Body: []byte(`<meta http-equiv="Content-Security-Policy" content="script-src 'none'">`)}
	if checkCSP(s, p) == nil {
		t.Fatal("meta CSP ignored")
	}
	for _, tc := range []struct {
		body  string
		allow bool
	}{
		{`<base href="/app/">`, true},
		{`<base href="https://EXAMPLE.com:443/app/">`, true},
		{`<base href="//cdn.example.com/">`, false},
		{`<base href="data:text/html,hello">`, false},
		{`<base target="_blank"><base href="https://cdn.example.com/">`, false},
		{`<base href="/"><base href="https://cdn.example.com/">`, true},
	} {
		if (checkCSP(s, Page{Header: http.Header{}, Body: []byte(tc.body)}) == nil) != tc.allow {
			t.Fatalf("incorrect HTML base decision for %s", tc.body)
		}
	}
	s.RUM.SessionReplayEnabled = true
	for _, tc := range []struct {
		policy string
		allow  bool
	}{
		{"", true},
		{"script-src 'self'; connect-src https://rum.example.com; worker-src blob:", true},
		{"default-src 'self' blob:; connect-src https://rum.example.com", true},
		{"script-src 'self'; child-src blob:; connect-src https://rum.example.com", true},
		{"script-src 'self'; connect-src https://rum.example.com", false},
		{"script-src 'self' blob:; worker-src 'none'; connect-src https://rum.example.com", false},
		{"script-src 'self'; child-src 'none'; worker-src blob:; connect-src https://rum.example.com", true},
		{"worker-src blob:, worker-src 'none'", false},
	} {
		p := Page{Header: http.Header{"Content-Security-Policy": []string{tc.policy}}}
		if (checkCSP(s, p) == nil) != tc.allow {
			t.Fatalf("unexpected replay Worker CSP decision for %q: %v", tc.policy, checkCSP(s, p))
		}
	}
	p = Page{Header: http.Header{}, Body: []byte(`<meta http-equiv="Content-Security-Policy" content="worker-src 'none'">`)}
	if checkCSP(s, p) == nil {
		t.Fatal("meta CSP allowed blocked replay worker")
	}
	zero := 0.0
	s.RUM.SessionReplaySampleRate = &zero
	if err := checkCSP(s, p); err != nil {
		t.Fatal("zero replay rate unnecessarily requires a worker", err)
	}
	n, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	p = Page{Header: http.Header{"Content-Security-Policy": []string{"script-src https://example.com" + n.Script + "; connect-src *"}}}
	if checkCSP(s, p) == nil {
		t.Fatal("allowed bootstrap hid a blocked SDK")
	}
	policy := "script-src https://example.com" + n.Script + " https://example.com" + s.assetPrefix() + "sdk." + n.SDK.SHA256 + ".js; connect-src https://rum.example.com/v1/write/rum"
	p.Header.Set("Content-Security-Policy", policy)
	if err := checkCSP(s, p); err != nil {
		t.Fatal("actual hashed scripts were rejected", err)
	}
	s.RUM.SessionReplaySampleRate = nil
	p.Header.Set("Content-Security-Policy", "script-src 'self'; worker-src blob:; connect-src https://rum.example.com/v1/write/rum")
	if checkCSP(s, p) == nil {
		t.Fatal("replay escaped its intake path restriction")
	}
	s.RUM.SessionReplayEnabled = false
	yes := true
	s.RUM.RemoteConfiguration = &yes
	p.Header.Set("Content-Security-Policy", "script-src 'self'; worker-src blob:; connect-src https://rum.example.com/v1/write/")
	if checkCSP(s, p) == nil {
		t.Fatal("remote configuration escaped its endpoint restriction")
	}
}

func TestExistingSDKDetectionUsesScriptContext(t *testing.T) {
	s := testSite()
	for _, tc := range []struct {
		body  string
		allow bool
	}{
		{`<html><head></head><body>DATAFLUX_RUM documentation</body></html>`, true},
		{`<html><head><script>window.DATAFLUX_RUM.init({});</script></head></html>`, false},
		{`<html><head><script src="https://static.guance.com/browser-sdk/v3/dataflux-rum.js"></script></head></html>`, false},
		{`<html><head><script data-rumable="other" src="/other.js"></script></head></html>`, false},
	} {
		p := Page{Header: http.Header{"Content-Type": []string{"text/html"}}, Body: []byte(tc.body)}
		if (checkPage(s, p) == nil) != tc.allow {
			t.Fatalf("unexpected SDK ownership decision for %s", tc.body)
		}
	}
}

func TestImmutableAssetsAndSiteIsolation(t *testing.T) {
	root := t.TempDir()
	a := testSite()
	b := testSite()
	b.ID = "second"
	na, pa, err := render(root, a, true)
	if err != nil {
		t.Fatal(err)
	}
	nb, pb, _ := render(root, b, true)
	if na.Script == nb.Script {
		t.Fatal("site namespaces collide")
	}
	if err = deployAssets(root, a, pa); err != nil {
		t.Fatal(err)
	}
	if err = deployAssets(root, b, pb); err != nil {
		t.Fatal(err)
	}
	for name := range pa {
		if strings.HasSuffix(name, ".js") {
			if err = os.WriteFile(filepath.Join(siteDir(root, a), name), []byte("external modification"), 0644); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if deployAssets(root, a, pa) == nil {
		t.Fatal("immutable asset overwritten")
	}
	if err = verifyFilesAssets(root, nb); err != nil {
		t.Fatal("other site was affected", err)
	}
}

func TestUpgradeKeepsCachedPageAssets(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	old, oldFiles, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = deployAssets(root, s, oldFiles); err != nil {
		t.Fatal(err)
	}
	s.RUM.SessionReplayEnabled = true
	next, nextFiles, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	if next.Script == old.Script {
		t.Fatal("changed initialization reused cached URL")
	}
	if err = deployAssets(root, s, nextFiles); err != nil {
		t.Fatal(err)
	}
	if err = verifyFilesAssets(root, old); err != nil {
		t.Fatal("cached page lost its versioned assets", err)
	}
	if err = verifyFilesAssets(root, next); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryDoesNotOverwriteExternalChanges(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	before, public, _ := render(root, s, false)
	after, _, _ := render(root, s, true)
	if err := deployAssets(root, s, public); err != nil {
		t.Fatal(err)
	}
	d := siteDir(root, s)
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	for name, b := range after.Files {
		if err := os.WriteFile(filepath.Join(d, name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(d, "location.conf")
	external := []byte("# operator change\n")
	os.WriteFile(file, external, 0644)
	err := restoreTransaction(root, transaction{Before: State{Current: before}, After: after})
	if err == nil {
		t.Fatal("external change accepted")
	}
	got, _ := os.ReadFile(file)
	if string(got) != string(external) {
		t.Fatal("external change lost")
	}
	os.WriteFile(file, after.Files["location.conf"], 0644)
	for name := range before.Assets {
		if err := os.WriteFile(filepath.Join(d, filepath.FromSlash(name)), []byte("changed asset"), 0644); err != nil {
			t.Fatal(err)
		}
		break
	}
	if restoreTransaction(root, transaction{Before: State{Current: before}, After: after}) == nil {
		t.Fatal("corrupt recovery assets accepted")
	}
	got, _ = os.ReadFile(file)
	if string(got) != string(after.Files["location.conf"]) {
		t.Fatal("configuration changed before recovery assets were verified")
	}
}

func TestRollbackRejectsJournalForDifferentTarget(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	s.PublicURL = "https://other.example.com/"
	foreign, _, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	foreignAfter, _, err := render(root, s, false)
	if err != nil {
		t.Fatal(err)
	}
	journal := transaction{Before: State{SchemaVersion: 1, Current: foreign}, After: foreignAfter}
	if err := writeJSON(journalPath(root, st.Current.Site), journal); err != nil {
		t.Fatal(err)
	}
	if rollback(root, st, io.Discard) == nil {
		t.Fatal("journal for another server target accepted")
	}
	if err := verifyFiles(root, st.Current); err != nil {
		t.Fatal("foreign journal modified this deployment", err)
	}
	if _, err := os.Stat(journalPath(root, st.Current.Site)); err != nil {
		t.Fatal("conflicting journal was removed", err)
	}
}

func TestProbeUsesExpectedAssetsAndRejectsDuplicateInjection(t *testing.T) {
	s := testSite()
	paths := []string{"/", "/second"}
	var requestsMu sync.Mutex
	requests := map[string]int{}
	compression := true
	s.RUM.CompressIntakeRequests = &compression
	s.RUM.WorkerURL = "/worker.js"
	var snapshot Snapshot
	var files map[string][]byte
	duplicate := false
	missingCompressedTag := false
	badGzipScript := ""
	jsMIME := "application/javascript"
	policy := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsMu.Lock()
		requests[r.URL.Path+" "+r.Header.Get("Accept-Encoding")]++
		requestsMu.Unlock()
		var body io.Writer = w
		if r.Header.Get("Accept-Encoding") == "gzip" {
			w.Header().Set("Content-Encoding", "gzip")
			compressed := gzip.NewWriter(w)
			defer compressed.Close()
			body = compressed
		}
		if r.URL.Path == s.RUM.WorkerURL {
			w.Header().Set("Content-Type", "application/javascript")
			if r.URL.Path == badGzipScript && r.Header.Get("Accept-Encoding") == "gzip" {
				io.WriteString(body, "/* stale worker */")
			} else {
				io.WriteString(body, "/* matching worker */")
			}
			return
		}
		if strings.HasPrefix(r.URL.Path, s.assetPrefix()) {
			name := "public/" + s.ID + "/" + strings.TrimPrefix(r.URL.Path, s.assetPrefix())
			w.Header().Set("Content-Type", jsMIME)
			if r.URL.Path == badGzipScript && r.Header.Get("Accept-Encoding") == "gzip" {
				io.WriteString(body, "/* stale compressed script */")
			} else {
				body.Write(files[name])
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if policy != "" {
			w.Header().Set("Content-Security-Policy", policy)
		}
		io.WriteString(body, "<html><head>")
		if !missingCompressedTag || r.URL.Path != "/second" || r.Header.Get("Accept-Encoding") != "gzip" {
			io.WriteString(body, scriptTag(s, snapshot.Script))
		}
		if duplicate {
			io.WriteString(body, scriptTag(s, snapshot.Script))
		}
		io.WriteString(body, "</head><body>OK</body></html>")
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	s.RUM.DatakitOrigin = server.URL
	var err error
	snapshot, files, err = render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = probePages(s, paths, &snapshot); err != nil {
		t.Fatal(err)
	}
	requestsMu.Lock()
	for _, path := range append(paths, snapshot.Script, s.assetPrefix()+"sdk."+snapshot.SDK.SHA256+".js", s.RUM.WorkerURL) {
		for _, encoding := range []string{"identity", "gzip"} {
			if count := requests[path+" "+encoding]; count != 1 {
				t.Errorf("%s (%s) should be checked once per round, got %d requests", path, encoding, count)
			}
		}
	}
	requestsMu.Unlock()
	for _, path := range []string{snapshot.Script, s.assetPrefix() + "sdk." + snapshot.SDK.SHA256 + ".js", s.RUM.WorkerURL} {
		badGzipScript = path
		if _, err = probePages(s, paths, &snapshot); err == nil {
			t.Errorf("stale compressed script was accepted: %s", path)
		}
	}
	badGzipScript = ""
	missingCompressedTag = true
	if _, err = probePages(s, paths, &snapshot); err == nil {
		t.Fatal("precompressed HTML without injection was accepted")
	}
	missingCompressedTag = false
	jsMIME = "application/x-javascript; charset=utf-8"
	if _, err = probePages(s, paths, &snapshot); err != nil {
		t.Fatal("legacy JavaScript MIME rejected", err)
	}
	jsMIME = "text/plain"
	if _, err = probePages(s, paths, &snapshot); err == nil {
		t.Fatal("plain text accepted as SDK")
	}
	jsMIME = "application/javascript"
	duplicate = true
	if _, err = probePages(s, paths, &snapshot); err == nil {
		t.Fatal("duplicate injection accepted")
	}
	duplicate = false
	// Archived releases can contain another SDK digest; probe must use the saved
	// references instead of regenerating URLs from the currently embedded SDK.
	oldSDK := []byte("/* archived SDK */")
	snapshot.SDK.SHA256 = hash(oldSDK)
	oldSDKPath := s.assetPrefix() + "sdk." + snapshot.SDK.SHA256 + ".js"
	oldBoot := bootstrap(s, oldSDKPath, snapshot.SDK)
	snapshot.Script = s.assetPrefix() + "init." + hash(oldBoot) + ".js"
	files["public/"+s.ID+"/sdk."+snapshot.SDK.SHA256+".js"] = oldSDK
	files["public/"+s.ID+"/init."+hash(oldBoot)+".js"] = oldBoot
	snapshot.Assets["public/"+s.ID+"/init."+hash(oldBoot)+".js"] = hash(oldBoot)
	snapshot.Assets["public/"+s.ID+"/sdk."+snapshot.SDK.SHA256+".js"] = snapshot.SDK.SHA256
	policy = "script-src " + strings.TrimRight(s.PublicURL, "/") + snapshot.Script + " " + strings.TrimRight(s.PublicURL, "/") + oldSDKPath + "; connect-src " + s.reportURL() + "; worker-src 'self'"
	if _, err = probePages(s, paths, &snapshot); err != nil {
		t.Fatal("archived SDK probe failed", err)
	}
}

func TestGzipProbeKeepsSourceAndSizeChecks(t *testing.T) {
	body := "<html><head></head></html>"
	corrupt := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		if corrupt {
			io.WriteString(w, "not gzip")
			return
		}
		compressed := gzip.NewWriter(w)
		io.WriteString(compressed, body)
		compressed.Close()
	}))
	defer server.Close()
	if _, err := getPage(server.URL, nil); err == nil {
		t.Fatal("source compression despite identity request was accepted")
	}
	headers := http.Header{"Accept-Encoding": []string{"gzip"}}
	if page, err := getPage(server.URL, headers); err != nil || string(page.Body) != body {
		t.Fatal("negotiated gzip was not decoded", err)
	}
	corrupt = true
	if _, err := getPage(server.URL, headers); err == nil {
		t.Fatal("corrupt gzip was accepted")
	}
	corrupt = false
	body = strings.Repeat("x", 8*1024*1024+1)
	if _, err := getPage(server.URL, headers); err == nil {
		t.Fatal("decompressed response exceeded the probe limit")
	}
}

func TestRenderAndLoadDoesNotExposeConfigurationInStatus(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.RUM.DatakitOrigin = ""
	s.RUM.Site = "https://open.example.com"
	s.RUM.ClientToken = "sensitive-browser-token"
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadState(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotsEqual(st.Current, loaded.Current) {
		t.Fatal("state did not round trip")
	}
	withEmptyArgs := s
	n := *s.Nginx
	withEmptyArgs.Nginx = &n
	withEmptyArgs.Nginx.Args = []string{}
	if !compatibleTarget(loaded.Current.Site, withEmptyArgs) {
		t.Fatal("omitted and empty Nginx args changed target identity")
	}
	if _, err = prepare(root, s); err == nil {
		t.Fatal("render overwrote existing deployment")
	}
	var out strings.Builder
	if err = Run([]string{"status", "--site", s.ID, "--state-dir", root}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), s.RUM.ClientToken) {
		t.Fatal("status leaked configuration token")
	}
	if !json.Valid([]byte(out.String())) {
		t.Fatal("invalid status JSON")
	}
}

func TestCorruptStateCannotEscapeManagedDirectory(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	delete(st.Current.Files, "location.conf")
	st.Current.Files["../../outside.conf"] = []byte("outside")
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	if _, err = loadState(root, s.ID); err == nil {
		t.Fatal("unsafe snapshot accepted")
	}
	st.Current, _, _ = render(root, s, false)
	st.Current.Assets["public/"+s.ID+"/sdk."+st.Current.SDK.SHA256+".js"] = strings.Repeat("0", 64)
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	if _, err = loadState(root, s.ID); err == nil {
		t.Fatal("mismatched SDK reference accepted")
	}
}
