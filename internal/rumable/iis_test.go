package rumable

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

// This boundary runs the real transaction/journal code, with only the
// Windows-only configuration API and process recycle replaced.
func TestIISRecycleRecoveryJournal(t *testing.T) {
	for _, failures := range []int{0, 1, 2, 3} {
		name := fmt.Sprintf("%d recycle failures", failures)
		if failures == 3 {
			name = "HTTP failure after recycle"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			s := otelSite()
			s.Nginx = nil
			s.IIS = &IIS{Site: "Orders", Application: "/"}
			s.OTel.ModulePath, s.OTel.AgentPath, s.OTel.Runtime = "", `C:\otel`, "netframework"
			off := false
			s.RUM.Enabled = &off
			var unhealthy int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.LoadInt32(&unhealthy) != 0 {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			s.PublicURL = server.URL + "/"
			st, err := prepare(root, s)
			if err != nil {
				t.Fatal(err)
			}
			original := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":null,"compressionPresent":false,"compressionValue":null,"otel":{"pool":"Dedicated","environment":{}}}`)
			after := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":null,"compressionPresent":false,"compressionValue":null,"otel":{"pool":"Dedicated","environment":{"OTEL_SERVICE_NAME":"web"}}}`)
			st.Current.Native = original
			st.OriginalIIS = original
			if err = saveState(root, st); err != nil {
				t.Fatal(err)
			}
			live := original
			recycles, remaining := 0, failures
			if failures == 3 {
				remaining = 0 // HTTP failure after a successful recycle.
			}
			committed := after
			savedBridge := iisCommand
			t.Cleanup(func() { iisCommand = savedBridge })
			iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
				switch action {
				case "inspect":
					return live, nil
				case "check":
					if extra["restoringOTel"] != true {
						t.Fatal("rollback re-read credentials instead of validating saved configuration")
					}
					return live, nil
				case "apply":
					if !sameJSON(extra["expected"].(json.RawMessage), live) {
						t.Fatal("apply lost expected native state")
					}
					live = after
				case "restore":
					if !sameJSON(extra["expected"].(json.RawMessage), live) {
						t.Fatal("restore lost external-change protection")
					}
					live = extra["snapshot"].(json.RawMessage)
				case "activate":
					var journal transaction
					data, err := os.ReadFile(journalPath(root, s))
					if err == nil {
						err = json.Unmarshal(data, &journal)
					}
					if err != nil || !journal.NativeCommitted || !sameJSON(journal.After.Native, committed) {
						t.Fatal("pool recycled before the committed snapshot was durable", err)
					}
					recycles++
					if failures == 3 {
						if sameJSON(live, after) {
							atomic.StoreInt32(&unhealthy, 1)
						} else {
							atomic.StoreInt32(&unhealthy, 0)
						}
					}
					if remaining > 0 {
						remaining--
						return nil, errors.New("recycle failed")
					}
				default:
					t.Fatal("unexpected IIS action", action)
				}
				return live, nil
			}
			target, public, err := render(root, s, true)
			if err != nil {
				t.Fatal(err)
			}
			err = transact(root, st, target, public, false)
			if failures == 0 {
				if err != nil || recycles != 1 || !sameJSON(live, after) {
					t.Fatal("successful activation was not committed", err, recycles)
				}
				active, err := loadState(root, s.ID)
				if err != nil {
					t.Fatal(err)
				}
				committed = original
				if err = rollback(root, active, io.Discard); err != nil || !sameJSON(live, original) || recycles != 2 {
					t.Fatal("ordinary rollback did not restore and recycle the saved environment", err, recycles)
				}
				disabled, err := loadState(root, s.ID)
				if err != nil {
					t.Fatal(err)
				}
				committed = after
				if err = rollback(root, disabled, io.Discard); err != nil || !sameJSON(live, after) || recycles != 3 {
					t.Fatal("rollback did not re-enable the saved environment", err, recycles)
				}
				return
			}
			if err == nil || !sameJSON(live, original) {
				t.Fatal("failed activation did not restore the owned configuration", err)
			}
			if failures == 2 {
				if _, err = os.Stat(journalPath(root, s)); err != nil {
					t.Fatal("failed recovery discarded its journal", err)
				}
				external := json.RawMessage(`{"otel":{"pool":"ForeignPool","environment":{}}}`)
				live = external
				if err = rollback(root, st, io.Discard); err == nil || !sameJSON(live, external) || recycles != 2 {
					t.Fatal("recovery overwrote external pool changes", err)
				}
				live = original
				// Configuration is already back, but the recovery recycle must be
				// retried rather than inferred from equal environment values.
				if err = rollback(root, st, io.Discard); err != nil || recycles != 3 {
					t.Fatal("retry did not recover the pool generation", err, recycles)
				}
			}
			if _, err = os.Stat(journalPath(root, s)); !os.IsNotExist(err) {
				t.Fatal("completed recovery retained a journal", err)
			}
			restored, err := loadState(root, s.ID)
			if err != nil || !snapshotsEqual(restored.Current, st.Current) || restored.Previous != nil {
				t.Fatal("failed activation changed successful deployment history", err)
			}
		})
	}
}

func TestIISRUMComponentPreservesDeployedToken(t *testing.T) {
	for _, token := range []string{"", "rotated-token"} {
		t.Run(token, func(t *testing.T) {
			root := t.TempDir()
			s := otelSite()
			s.Nginx = nil
			s.IIS = &IIS{Site: "Orders", Application: "/"}
			s.OTel.ModulePath, s.OTel.AgentPath, s.OTel.Runtime = "", `C:\otel`, "netframework"
			s.OTel.Backend, s.OTel.Endpoint, s.OTel.TokenEnv = "dataway", "https://127.0.0.1:1/otel", "RUMABLE_IIS_COMPONENT_TOKEN"
			off := false
			s.RUM.Enabled = &off
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			s.PublicURL = server.URL + "/"
			t.Setenv(s.OTel.TokenEnv, "deployed-token")
			n, public, err := render(root, s, true)
			if err != nil {
				t.Fatal(err)
			}
			if err = deployAssets(root, s, public); err != nil {
				t.Fatal(err)
			}
			n.Native = json.RawMessage(`{"otel":{"pool":"Dedicated","environment":{"OTEL_EXPORTER_OTLP_HEADERS":"X-Token=deployed-token"}}}`)
			st := State{SchemaVersion: 1, Current: n, OriginalIIS: n.Native}
			if err = saveState(root, st); err != nil {
				t.Fatal(err)
			}
			t.Setenv(s.OTel.TokenEnv, token)
			savedBridge := iisCommand
			t.Cleanup(func() { iisCommand = savedBridge })
			iisCommand = func(_ string, checking Site, action string, _ map[string]any) (json.RawMessage, error) {
				if action != "check" && action != "inspect" {
					t.Fatal("idempotent RUM operation changed native state", action)
				}
				if action == "check" && checking.OTel.Enabled {
					t.Fatal("RUM operation revalidated unchanged OTel credentials/dependencies")
				}
				return n.Native, nil
			}
			if err = apply(root, st, s, true, "rum", io.Discard); err != nil {
				t.Fatal("RUM-only operation depends on current OTel credentials", err)
			}
			restored, err := loadState(root, s.ID)
			if err != nil || !snapshotsEqual(n, restored.Current) || restored.Previous != nil {
				t.Fatal("unchanged RUM operation rotated OTel credentials/history", err)
			}
		})
	}
}
