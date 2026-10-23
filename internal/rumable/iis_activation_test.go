package rumable

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Persisted configuration and the HTTP worker deliberately have separate state.
// Commit alone never switches a response. This models the Windows S3/CT/FAULT
// boundary missing from the old instant-commit mocks; it is not a native IIS test.
func TestIISActivationBeforeProbesAndRecovery(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.Nginx, s.IIS = nil, &IIS{Site: "Website", Application: "/", AllowCompressionChanges: true}
	s.ProbePaths = []string{"/", "/second.html"}
	var persisted, worker atomic.Value
	original := `{"rules":[],"preconditions":[],"virtualDirectory":"managed","compressionPresent":true,"compressionValue":"true","dynamicCompressionPresent":true,"dynamicCompressionValue":"true","cacheGuard":{"rules":[],"ruleIndex":-1},"rumRuntime":{"pool":"Dedicated","applications":["Website/"]}}`
	persisted.Store(original)
	worker.Store(original)
	var unactivatedRequests atomic.Int32
	var brokenScript atomic.Value
	brokenScript.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, s.assetPrefix()) {
			w.Header().Set("Content-Type", "application/javascript")
			http.ServeFile(w, r, filepath.Join(siteDir(root, s), "public", s.ID, filepath.Base(r.URL.Path)))
			return
		}
		if !sameJSON(json.RawMessage(worker.Load().(string)), json.RawMessage(persisted.Load().(string))) {
			unactivatedRequests.Add(1)
		}
		var live struct {
			Rules []string `json:"rules"`
		}
		json.Unmarshal([]byte(worker.Load().(string)), &live)
		w.Header().Set("Content-Type", "text/html")
		if len(live.Rules) == 0 && r.Header.Get("If-None-Match") != "" {
			w.WriteHeader(304)
			return
		}
		tag := strings.Join(live.Rules, "")
		if bad := brokenScript.Load().(string); bad != "" && strings.Contains(tag, bad) && r.URL.Path == "/second.html" {
			tag = ""
		}
		fmt.Fprint(w, "<html><head>"+tag+"</head><body>Business</body></html>")
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	st.Current.Native = json.RawMessage(original)
	st.OriginalIIS = st.Current.Native
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	bridge := iisCommand
	t.Cleanup(func() { iisCommand = bridge })
	activations := 0
	restoring, omitRecoveryActivation := false, false
	iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
		switch action {
		case "inspect", "check":
		case "prepare-rum-rollback":
			return extra["snapshot"].(json.RawMessage), nil
		case "apply":
			restoring = false
			fields := dynamicCompressionSnapshot(json.RawMessage(original))
			if extra["enabled"].(bool) {
				fields["rules"], _ = json.Marshal([]string{extra["injection"].(string)})
				fields["cacheGuard"] = json.RawMessage(`{"rules":["guard"],"ruleIndex":0}`)
				fields["compressionValue"] = json.RawMessage(`"false"`)
				fields["dynamicCompressionValue"] = json.RawMessage(`"false"`)
			}
			data, _ := json.Marshal(fields)
			persisted.Store(string(data))
		case "restore":
			restoring = true
			persisted.Store(string(extra["snapshot"].(json.RawMessage)))
		case "activate":
			data, e := os.ReadFile(journalPath(root, s))
			if e != nil {
				t.Fatal(e)
			}
			var receipt transaction
			if json.Unmarshal(data, &receipt) != nil || !receipt.NativeCommitted {
				t.Fatal("activation ran before durable commit receipt")
			}
			if extra["rumChanged"] != true || !sameJSON(extra["expected"].(json.RawMessage), json.RawMessage(persisted.Load().(string))) {
				t.Fatal("activation lost target snapshot")
			}
			activations++
			if !(restoring && omitRecoveryActivation) {
				worker.Store(persisted.Load().(string))
			}
		default:
			t.Fatalf("unexpected action %s", action)
		}
		return json.RawMessage(persisted.Load().(string)), nil
	}
	load := func() State {
		t.Helper()
		state, e := loadState(root, s.ID)
		if e != nil {
			t.Fatal(e)
		}
		return state
	}
	applyOK := func(enabled bool) {
		t.Helper()
		if e := apply(root, load(), s, enabled, "all", io.Discard); e != nil {
			t.Fatal(e)
		}
	}
	for round := 0; round < 2; round++ {
		s.RUM.Env = fmt.Sprintf("v%d", round)
		applyOK(true)
		if round == 0 {
			// Upgrade an otherwise current 0.1.7 state: existing rule bytes
			// must not bypass the newly introduced runtime activation.
			legacy := load()
			for _, raw := range []*json.RawMessage{&legacy.Current.Native, &legacy.OriginalIIS} {
				fields := dynamicCompressionSnapshot(*raw)
				delete(fields, "rumRuntime")
				*raw, _ = json.Marshal(fields)
			}
			if e := saveState(root, legacy); e != nil {
				t.Fatal(e)
			}
			count := activations
			applyOK(true)
			if activations != count+1 {
				t.Fatal("0.1.7 migration skipped activation")
			}
		}
		count := activations
		applyOK(true)
		if activations != count {
			t.Fatal("idempotent enable recycled")
		}
		first := load().Current.Script
		s.RUM.Env += "-updated"
		applyOK(true)
		if load().Current.Script == first {
			t.Fatal("update did not replace bootstrap")
		}
		if e := rollback(root, load(), io.Discard); e != nil {
			t.Fatal(e)
		}
		if load().Current.Script != first {
			t.Fatal("history rollback lost bootstrap")
		}
		applyOK(false)
		count = activations
		applyOK(false)
		if activations != count {
			t.Fatal("idempotent disable recycled")
		}
	}
	if unactivatedRequests.Load() != 0 {
		t.Fatal("HTTP validation ran before activation", unactivatedRequests.Load())
	}

	// A persistent second-page fault is still rejected on its first response,
	// but recovery must activate the restored disabled configuration too.
	for _, leaveOldWorker := range []bool{false, true} {
		s.RUM.Env = fmt.Sprintf("fault-%t", leaveOldWorker)
		bad, _, e := render(root, s, true)
		if e != nil {
			t.Fatal(e)
		}
		brokenScript.Store(bad.Script)
		omitRecoveryActivation = leaveOldWorker
		e = apply(root, load(), s, true, "all", io.Discard)
		if e == nil {
			t.Fatal("persistent bad response accepted")
		}
		evidence, readErr := os.ReadFile(filepath.Join(siteDir(root, s), "last-operation-probe-failure.json"))
		if readErr != nil || !strings.Contains(string(evidence), "HTTP 200") || !strings.Contains(string(evidence), "decodedBodySHA256") || !strings.Contains(string(evidence), "/second.html") {
			t.Fatal("first failure evidence missing", readErr, string(evidence))
		}
		if leaveOldWorker {
			if !strings.Contains(e.Error(), "restored configuration but HTTP recovery verification failed") {
				t.Fatal(e)
			}
			if _, e = os.Stat(journalPath(root, s)); e != nil {
				t.Fatal("lost failed-recovery journal", e)
			}
			if _, e = os.Stat(filepath.Join(siteDir(root, s), "last-recovery-probe-failure.json")); e != nil {
				t.Fatal(e)
			}
			// Configuration already equals before. Official rollback must
			// nevertheless activate it again, not just compare config files.
			before := persisted.Load().(string)
			omitRecoveryActivation = false
			if e = rollback(root, load(), io.Discard); e != nil {
				t.Fatal(e)
			}
			if !sameJSON(json.RawMessage(persisted.Load().(string)), json.RawMessage(before)) {
				t.Fatal("retry changed the restored configuration")
			}
		} else if !strings.Contains(e.Error(), "was rolled back") {
			t.Fatal(e)
		}
		if load().Current.Enabled {
			t.Fatal("failed first enable did not recover disabled state")
		}
		if _, e = os.Stat(journalPath(root, s)); !os.IsNotExist(e) {
			t.Fatal("completed recovery retained journal", e)
		}
	}
}

func TestIISRuntimeMigrationAndDrift(t *testing.T) {
	legacy := json.RawMessage(`{"rules":[]}`)
	live := json.RawMessage(`{"rules":[],"rumRuntime":{"pool":"Dedicated","applications":["Website/"]}}`)
	if !sameIISSnapshot(live, legacy) {
		t.Fatal("legacy read rejected")
	}
	adopted, err := copyDynamicCompression(legacy, live)
	if err != nil || !sameIISSnapshot(live, adopted) {
		t.Fatal("runtime not adopted", err)
	}
	for _, raw := range []string{
		`{"rules":[],"rumRuntime":{"pool":"Other","applications":["Website/"]}}`,
		`{"rules":[],"rumRuntime":{"pool":"Dedicated","applications":["Website/","Other/"]}}`,
	} {
		if sameIISSnapshot(json.RawMessage(raw), adopted) {
			t.Fatal("pool drift ignored")
		}
	}
	if iisRUMChanged(legacy, live) {
		t.Fatal("metadata alone is not a RUM rule change")
	}
}

func TestIISProbeFailureEvidence(t *testing.T) {
	if got := responseEvidence(http.Header{"Vary": {""}}); !strings.Contains(got, `Vary=""`) || !strings.Contains(got, "Age=<absent>") {
		t.Fatal("absent and empty response headers were conflated", got)
	}
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Website", Application: "/"}
	n := Snapshot{Site: s, Enabled: true, Script: s.assetPrefix() + "init." + hash([]byte("expected")) + ".js"}
	oldScript := s.assetPrefix() + "init." + hash([]byte("old")) + ".js"
	for _, status := range []int{200, 304} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Cache-Control", "public, max-age=60")
			w.Header().Set("Set-Cookie", "secret-cookie")
			w.WriteHeader(status)
			fmt.Fprint(w, "<html><head>"+scriptTag(s, oldScript)+"</head></html>")
		}))
		s.PublicURL = server.URL + "/"
		n.Site = s
		_, err := probePage(s, "/", &n)
		server.Close()
		if err == nil {
			t.Fatal("bad response accepted")
		}
		message := err.Error()
		if !strings.Contains(message, " at ") || !strings.Contains(message, "Cache-Control") || strings.Contains(message, "secret-cookie") {
			t.Fatal(message)
		}
		if status == 200 && (!strings.Contains(message, "HTTP 200") || !strings.Contains(message, oldScript) || !strings.Contains(message, n.Script)) {
			t.Fatal(message)
		}
		if status == 304 && !strings.Contains(message, "got 304") {
			t.Fatal(message)
		}
	}
	for _, raw := range []string{"https://user:secret@example.com/init.js?token=secret", s.assetPrefix() + "init.js?token=secret", s.assetPrefix() + "secret.js"} {
		if strings.Contains(bootstrapEvidence(s, raw), "secret") {
			t.Fatal("source attribute leaked")
		}
	}
}
