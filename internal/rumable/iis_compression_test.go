package rumable

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIISDynamicCompressionOwnership(t *testing.T) {
	legacy := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":null,"compressionPresent":true,"compressionValue":"false"}`)
	baseline := json.RawMessage(`{"dynamicCompressionPresent":true,"dynamicCompressionValue":"true"}`)
	live, err := copyDynamicCompression(legacy, baseline)
	if err != nil || !sameIISSnapshot(live, legacy) {
		t.Fatal("old snapshot compatibility lost", err)
	}
	changed := dynamicCompressionSnapshot(live)
	changed["dynamicCompressionValue"] = json.RawMessage(`"false"`)
	other, _ := json.Marshal(changed)
	if sameIISSnapshot(other, live) {
		t.Fatal("managed dynamic drift ignored")
	}
	changed["compressionValue"] = json.RawMessage(`"true"`)
	other, _ = json.Marshal(changed)
	if sameIISSnapshot(other, legacy) {
		t.Fatal("legacy static drift ignored")
	}
	changed = dynamicCompressionSnapshot(live)
	changed["unexpected"] = json.RawMessage(`true`)
	other, _ = json.Marshal(changed)
	if sameIISSnapshot(other, legacy) {
		t.Fatal("unknown field silently ignored")
	}
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Website", Application: "/"}
	st := State{Current: Snapshot{Site: s, Native: legacy}, OriginalIIS: legacy, Previous: &Snapshot{Native: legacy}}
	bridge := iisCommand
	t.Cleanup(func() { iisCommand = bridge })
	iisCommand = func(_ string, _ Site, action string, _ map[string]any) (json.RawMessage, error) {
		if action != "inspect" {
			t.Fatal(action)
		}
		return live, nil
	}
	if err = adoptIISDynamicCompression(t.TempDir(), &st); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []json.RawMessage{st.Current.Native, st.OriginalIIS, st.Previous.Native} {
		if !sameJSON(raw, live) {
			t.Fatal("upgrade lost original local setting", string(raw))
		}
	}
	target := st.Current
	target.Enabled = true
	if err = protectIISRollback(t.TempDir(), &target); err != nil || !iisDynamicCompressionDisabled(target.Native) {
		t.Fatal("history rollback revived dynamic compression", err)
	}
	if !sameJSON(st.Current.Native, live) {
		t.Fatal("rollback changed exact recovery baseline")
	}
}

func TestIISLegacyCompressionUpgradeLifecycle(t *testing.T) {
	for _, failUpgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "lifecycle", true: "failed-upgrade"}[failUpgrade], func(t *testing.T) {
			root := t.TempDir()
			s := testSite()
			s.Nginx = nil
			s.IIS = &IIS{Site: "Website", Application: "/", AllowCompressionChanges: true}
			var live map[string]any
			broken := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, s.assetPrefix()) {
					w.Header().Set("Content-Type", "application/javascript")
					b, err := os.ReadFile(filepath.Join(siteDir(root, s), "public", strings.TrimPrefix(r.URL.Path, "/__rumable__/")))
					if err != nil {
						w.WriteHeader(404)
						return
					}
					w.Write(b)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				raw, _ := json.Marshal(live)
				if (r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "") && !iisCacheGuardEnabled(raw) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
				io.WriteString(w, "<html><head>")
				if !broken {
					for _, rule := range live["rules"].([]any) {
						io.WriteString(w, rule.(string))
					}
				}
				io.WriteString(w, "</head><body>Business</body></html>")
			}))
			defer server.Close()
			s.PublicURL = server.URL + "/"
			st, err := prepare(root, s)
			if err != nil {
				t.Fatal(err)
			}
			n, _, err := render(root, s, true)
			if err != nil {
				t.Fatal(err)
			}
			original := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":null,"compressionPresent":false,"compressionValue":null}`)
			live = map[string]any{"rules": []any{injectionTags(n)}, "preconditions": []any{}, "virtualDirectory": "managed", "compressionPresent": true, "compressionValue": "false"}
			n.Native, _ = json.Marshal(live)
			st.Current = n
			st.OriginalIIS = original
			if err = saveState(root, st); err != nil {
				t.Fatal(err)
			}
			live["dynamicCompressionPresent"] = true
			live["dynamicCompressionValue"] = "true"
			live["cacheGuard"] = map[string]any{"rules": []string{}, "permissions": []string{}, "effectivePermissions": []string{}, "ruleIndex": -1}
			applies := 0
			bridge := iisCommand
			t.Cleanup(func() { iisCommand = bridge })
			iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
				switch action {
				case "inspect", "check":
				case "apply":
					applies++
					// The upgrade baseline must be durable before the bridge changes IIS.
					b, e := os.ReadFile(journalPath(root, s))
					if e != nil {
						t.Fatal(e)
					}
					var tx transaction
					if json.Unmarshal(b, &tx) != nil {
						t.Fatal("invalid journal")
					}
					if string(dynamicCompressionSnapshot(tx.Before.OriginalIIS)["dynamicCompressionValue"]) != `"true"` {
						t.Fatal("original dynamic setting not durable")
					}
					if extra["enabled"].(bool) {
						live["rules"] = []any{extra["injection"].(string)}
						live["compressionPresent"] = true
						live["compressionValue"] = "false"
						live["dynamicCompressionPresent"] = true
						live["dynamicCompressionValue"] = "false"
						live["cacheGuard"] = map[string]any{"rules": []string{"guard"}, "permissions": []string{"HTTP_IF_NONE_MATCH", "HTTP_IF_MODIFIED_SINCE"}, "effectivePermissions": []string{"HTTP_IF_NONE_MATCH", "HTTP_IF_MODIFIED_SINCE"}, "ruleIndex": 0}
						broken = failUpgrade
					} else {
						if e := json.Unmarshal(extra["original"].(json.RawMessage), &live); e != nil {
							t.Fatal(e)
						}
						live["virtualDirectory"] = "managed"
					}
				case "prepare-rum-rollback":
					var target map[string]any
					if e := json.Unmarshal(extra["snapshot"].(json.RawMessage), &target); e != nil {
						t.Fatal(e)
					}
					target["cacheGuard"] = map[string]any{"rules": []string{"guard"}, "permissions": []string{"HTTP_IF_NONE_MATCH", "HTTP_IF_MODIFIED_SINCE"}, "effectivePermissions": []string{"HTTP_IF_NONE_MATCH", "HTTP_IF_MODIFIED_SINCE"}, "ruleIndex": 0}
					return json.Marshal(target)
				case "restore":
					if e := json.Unmarshal(extra["snapshot"].(json.RawMessage), &live); e != nil {
						t.Fatal(e)
					}
					broken = false
				default:
					t.Fatal("unexpected bridge action", action)
				}
				return json.Marshal(live)
			}
			var output bytes.Buffer
			err = setupSite(root, s, "", true, strings.NewReader(""), &output)
			if failUpgrade {
				if err == nil || !strings.Contains(err.Error(), "was rolled back") {
					t.Fatal("failure did not recover", err)
				}
				got, e := loadState(root, s.ID)
				if e != nil {
					t.Fatal(e)
				}
				if live["dynamicCompressionValue"] != "true" || string(dynamicCompressionSnapshot(got.Current.Native)["dynamicCompressionValue"]) != `"true"` {
					t.Fatal("failed upgrade lost original compression")
				}
			} else {
				if err != nil || applies != 1 {
					t.Fatal("unchanged 0.1.6 config skipped compression upgrade", applies, err)
				}
				if !strings.Contains(output.String(), "these URLs only:") || !strings.Contains(output.String(), server.URL+"/") || !strings.Contains(output.String(), "previously cached browser responses are not verified") {
					t.Fatal("setup overstated verification scope", output.String())
				}
				for _, operation := range []string{"repeat", "disable", "rollback", "disable"} {
					current, e := loadState(root, s.ID)
					if e != nil {
						t.Fatal(e)
					}
					switch operation {
					case "repeat":
						err = apply(root, current, s, true, "all", io.Discard)
						if applies != 1 {
							t.Fatal("repeat was not idempotent")
						}
					case "disable":
						err = apply(root, current, s, false, "all", io.Discard)
					case "rollback":
						err = rollback(root, current, io.Discard)
					}
					if err != nil {
						t.Fatal(operation, err)
					}
					want := "false"
					if operation == "disable" {
						want = "true"
					}
					if live["dynamicCompressionValue"] != want {
						t.Fatal(operation, live)
					}
				}
			}
			if _, e := os.Stat(journalPath(root, s)); !os.IsNotExist(e) {
				t.Fatal("completed operation retained transaction", e)
			}
		})
	}
}

func TestIISCacheGuardSnapshotMigration(t *testing.T) {
	legacy := json.RawMessage(`{"rules":[],"compressionPresent":false}`)
	baseline := json.RawMessage(`{"cacheGuard":{"rules":[],"ruleIndex":-1,"permissions":[],"effectivePermissions":["HTTP_IF_NONE_MATCH"]}}`)
	live, err := copyDynamicCompression(legacy, baseline)
	if err != nil || !sameIISSnapshot(live, legacy) {
		t.Fatal("legacy empty guard not adopted", err)
	}
	fields := dynamicCompressionSnapshot(live)
	fields["cacheGuard"] = json.RawMessage(`{"rules":["guard"],"ruleIndex":0,"permissions":["HTTP_IF_MODIFIED_SINCE"],"effectivePermissions":["HTTP_IF_MODIFIED_SINCE","HTTP_IF_NONE_MATCH"]}`)
	active, _ := json.Marshal(fields)
	if sameIISSnapshot(active, legacy) || sameIISSnapshot(active, live) {
		t.Fatal("unexpected new owned rule ignored")
	}
	if !iisCacheGuardEnabled(active) || iisCacheGuardEnabled(live) {
		t.Fatal("legacy configuration can skip upgrade")
	}
	changed := dynamicCompressionSnapshot(live)
	changed["cacheGuard"] = json.RawMessage(`{"rules":[],"ruleIndex":-1,"permissions":[],"effectivePermissions":[]}`)
	drift, _ := json.Marshal(changed)
	if sameIISSnapshot(drift, live) {
		t.Fatal("managed inherited permission drift ignored")
	}
}
