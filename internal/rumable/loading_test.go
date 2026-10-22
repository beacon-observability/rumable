package rumable

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSDKLoadingModesAndProbes(t *testing.T) {
	for _, platform := range []string{"nginx", "iis"} {
		for _, source := range []string{"bundled", "cdn"} {
			for _, loading := range []string{"async", "sync"} {
				t.Run(platform+"/"+source+"/"+loading, func(t *testing.T) {
					s := testSite()
					s.SDK = SDKConfig{Source: source, Loading: loading, Integrity: source == "cdn"}
					if platform == "iis" {
						s.Nginx, s.IIS = nil, &IIS{Site: "Fixture", Application: "/"}
					}
					n, _, err := render(t.TempDir(), s, true)
					if err != nil {
						t.Fatal(err)
					}
					if err := validateSnapshot(n, s.ID); err != nil {
						t.Fatal(err)
					}
					tags := injectionTags(n)
					page := Page{Header: http.Header{"Content-Type": []string{"text/html"}}, Body: []byte("<html><head>" + tags + "</head><body>business</body></html>")}
					if _, err := inspectInjection(s, page, &n); err != nil {
						t.Fatal(err)
					}
					if err := checkPage(s, page, n); err != nil {
						t.Fatal("managed integration prevents re-enable", err)
					}
					if s.Nginx != nil && !strings.Contains(string(n.Files["http.conf"]), nginxQuote(tags)) {
						t.Fatal("Nginx does not inject the selected scripts")
					}
					if loading == "sync" {
						if strings.Count(tags, "<script ") != 2 || strings.Contains(tags, " async") || strings.Contains(tags, " defer") {
							t.Fatal("sync is not parser-blocking", tags)
						}
						if checkPage(s, page) == nil {
							t.Fatal("unowned SDK tag was taken over")
						}
						async := s
						async.SDK.Loading = "async"
						if err := checkPage(async, page, n); err != nil {
							t.Fatal("cannot switch from sync to async", err)
						}
						broken := []string{
							strings.Replace(tags, "<script ", "<script async ", 1),
							strings.Replace(tags, "<script ", "<script defer ", 1),
							strings.Replace(tags, "<script ", `<script type="module" `, 1),
							strings.Replace(tags, snapshotSDKURL(n), "/wrong.js", 1),
							strings.Replace(tags, "</script><script", "</script><script src='/business.js'></script><script", 1),
						}
						if source == "cdn" {
							broken = append(broken, strings.Replace(tags, sdkIntegrity(n.SDK), "sha256-wrong", 1))
						}
						for _, bad := range broken {
							page.Body = []byte("<head>" + bad + "</head>")
							if _, err := inspectInjection(s, page, &n); err == nil {
								t.Fatal("invalid sync sequence accepted", bad)
							}
						}
						page.Body = []byte("<head>" + tags[:strings.Index(tags, "</script>")+9] + "</head>")
						off := n
						off.Enabled = false
						if _, err := inspectInjection(s, page, &off); err == nil {
							t.Fatal("orphan SDK tag survived disable")
						}
					}
				})
			}
		}
	}
	if (SDKConfig{Loading: "defer"}).validate() == nil {
		t.Fatal("unknown loading mode accepted")
	}
}

func TestSynchronousBootstrapExecution(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is needed to execute bootstrap")
	}
	s := testSite()
	s.SDK.Loading = "sync"
	s.RUM.SessionReplayEnabled = true
	_, info, err := sdk()
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "init.js")
	if err := os.WriteFile(name, bootstrap(s, sdkLocation(s, info), info), 0600); err != nil {
		t.Fatal(err)
	}
	script := `const assert=require('node:assert/strict'),vm=require('node:vm'),fs=require('node:fs');
const source=fs.readFileSync(process.argv[1],'utf8');
for(const failure of ['', 'load', 'init', 'replay']){
 const calls=[]; const window={};
 if(failure!=='load') window.DATAFLUX_RUM={init(c){calls.push('init');assert.equal(c.applicationId,'app');if(failure==='init')throw Error('init');},startSessionReplayRecording(){calls.push('replay');if(failure==='replay')throw Error('replay');}};
 const context=vm.createContext({window});
 vm.runInContext(source,context);vm.runInContext(source,context);
 assert.deepEqual(calls,failure==='load'?[]:failure==='init'?['init']:['init','replay']);
 assert.equal(window.__RUMABLE__.status,failure==='load'?'load-error':failure==='init'?'init-error':'initialized');
 if(failure==='replay')assert.equal(window.__RUMABLE__.replayStatus,'start-error');
}

const window={__RUMABLE__:{status:'owned'},DATAFLUX_RUM:{init(){throw Error('duplicate');}}};
vm.runInNewContext(source,{window});assert.equal(window.__RUMABLE__.status,'owned');`
	if b, err := exec.Command("node", "-e", script, name).CombinedOutput(); err != nil {
		t.Fatalf("sync bootstrap: %v: %s", err, b)
	}
}

func TestSynchronousPreflightAllowsNewPaths(t *testing.T) {
	for _, source := range []string{"bundled", "cdn"} {
		t.Run(source, func(t *testing.T) {
			s := testSite()
			s.SDK = SDKConfig{Source: source, Loading: "sync"}
			s.Include, s.ProbePaths = []string{"/old/"}, []string{"/old/"}
			n, _, err := render(t.TempDir(), s, true)
			if err != nil {
				t.Fatal(err)
			}
			s.Include, s.ProbePaths = []string{"/"}, []string{"/new/"}
			p := Page{URL: pageURL(s, "/new/"), Header: http.Header{"Content-Type": []string{"text/html"}}, Body: []byte("<html><head><title>New page</title></head><body></body></html>")}
			if err := checkPage(s, p, n); err != nil {
				t.Fatal("newly included page must not require an existing injection", err)
			}
			p.Body = []byte(`<html><head><script src="https://static.guance.com/browser-sdk/v3/dataflux-rum.js"></script></head></html>`)
			if err := checkPage(s, p, n); err == nil {
				t.Fatal("business-owned SDK on new path was accepted")
			}
		})
	}
}

func TestIISSynchronousApplyUsesSavedSDK(t *testing.T) {
	s := testSite()
	s.Nginx, s.IIS = nil, &IIS{Site: "Fixture", Application: "/"}
	s.SDK = SDKConfig{Source: "cdn", Loading: "sync", Integrity: true}
	n, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a historical release: native apply must use its saved digest,
	// never the SDK embedded in the currently running CLI.
	n.SDK.SHA256 = strings.Repeat("a", 64)
	old := iisCommand
	t.Cleanup(func() { iisCommand = old })
	iisCommand = func(root string, site Site, action string, extra map[string]any) (json.RawMessage, error) {
		if action != "apply" || extra["injection"] != injectionTags(n) || !strings.Contains(fmt.Sprint(extra["injection"]), sdkIntegrity(n.SDK)) {
			t.Fatal("IIS received wrong sync injection", extra)
		}
		return json.RawMessage(`{}`), nil
	}
	if err := nativeApply(t.TempDir(), &n, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSynchronousSDKURLRemainsLiteral(t *testing.T) {
	s := testSite()
	s.SDK = SDKConfig{Source: "cdn", Loading: "sync", URL: "https://cdn.example.com/sdk$version{R:0}&part.js"}
	if err := s.SDK.validate(); err != nil {
		t.Fatal(err)
	}
	n, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	tags := injectionTags(n)
	if strings.ContainsAny(tags, "${}") {
		t.Fatal("script URL contains Nginx variables or IIS rewrite tokens", tags)
	}
	p := Page{Header: http.Header{"Content-Type": []string{"text/html"}}, Body: []byte("<html><head>" + tags + "</head></html>")}
	if _, err := inspectInjection(s, p, &n); err != nil {
		t.Fatal("escaped script URL did not round trip through HTML", err)
	}
}
