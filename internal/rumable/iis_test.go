package rumable

import (
	"encoding/json"
	"errors"
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

func TestOTelComponentKeepsDeployedBrowserVersion(t *testing.T) {
	for _, cdn := range []bool{false, true} {
		t.Run(fmt.Sprintf("cdn=%t", cdn), func(t *testing.T) {
			root := t.TempDir()
			s := otelSite()
			s.Nginx = nil
			s.IIS = &IIS{Site: "Orders", Application: "/"}
			s.OTel.ModulePath, s.OTel.AgentPath, s.OTel.Runtime = "", `C:\otel`, "netframework"
			s.OTel.Enabled = false
			oldSDK, oldBoot := []byte("/* previous SDK */"), []byte("/* previous bootstrap */")
			var liveScript atomic.Value
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/old-sdk.js" {
					w.Header().Set("Content-Type", "application/javascript")
					_, _ = w.Write(oldSDK)
					return
				}
				if strings.HasPrefix(r.URL.Path, s.assetPrefix()) {
					w.Header().Set("Content-Type", "application/javascript")
					data, err := os.ReadFile(filepath.Join(siteDir(root, s), "public", s.ID, filepath.Base(r.URL.Path)))
					if err != nil {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write(data)
					return
				}
				w.Header().Set("Content-Type", "text/html")
				_, _ = fmt.Fprint(w, "<html><head>"+scriptTag(s, liveScript.Load().(string))+"</head></html>")
			}))
			defer server.Close()
			savedClient := httpClient
			httpClient = server.Client()
			t.Cleanup(func() { httpClient = savedClient })
			if cdn {
				s.SDK = SDKConfig{Source: "cdn", URL: server.URL + "/old-sdk.js"}
			}
			s.PublicURL, s.OTel.Endpoint = server.URL+"/", server.URL
			st, err := prepare(root, s)
			if err != nil {
				t.Fatal(err)
			}
			// Model a deployment made by a previous CLI/SDK release, with different bytes.
			st.Current.SDK = SDKInfo{Version: "3.0.0", Source: "https://cdn.example.com/3.0.0.js", SHA256: hash(oldSDK)}
			st.Current.Script = s.assetPrefix() + "init." + hash(oldBoot) + ".js"
			if cdn {
				st.Current.SDKURL = s.SDK.URL
			}
			files := map[string][]byte{
				"public/" + s.ID + "/sdk." + hash(oldSDK) + ".js":   oldSDK,
				"public/" + s.ID + "/init." + hash(oldBoot) + ".js": oldBoot,
			}
			st.Current.Assets = map[string]string{}
			for name, data := range files {
				st.Current.Assets[name] = hash(data)
			}
			st.Current.Enabled = true
			st.Current.Native = json.RawMessage(`{"otel":{"pool":"Dedicated","environment":{}}}`)
			st.OriginalIIS = st.Current.Native
			liveScript.Store(st.Current.Script)
			if err = deployAssets(root, s, files); err != nil {
				t.Fatal(err)
			}
			if err = saveState(root, st); err != nil {
				t.Fatal(err)
			}
			live := st.Current.Native
			saved := iisCommand
			t.Cleanup(func() { iisCommand = saved })
			iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
				if action == "apply" {
					liveScript.Store(extra["script"].(string))
					live = json.RawMessage(`{"otel":{"pool":"Dedicated","environment":{"OTEL_SERVICE_NAME":"web"}}}`)
				}
				return live, nil
			}
			desired, active, err := componentSite(s, st.Current, "otel", true)
			if err != nil {
				t.Fatal(err)
			}
			if err = apply(root, st, desired, active, "otel", io.Discard); err != nil {
				t.Fatal(err)
			}
			next, err := loadState(root, s.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !next.Current.OTelEnabled || !next.Current.Enabled || next.Current.Script != st.Current.Script || next.Current.SDK != st.Current.SDK || next.Current.SDKURL != st.Current.SDKURL {
				t.Fatal("OTel-only enable silently upgraded the browser deployment")
			}
			for name, digest := range st.Current.Assets {
				if next.Current.Assets[name] != digest {
					t.Fatal("OTel-only deployment lost browser rollback assets", name)
				}
			}
		})
	}
}

func TestIISUninstallRecoveryAndArchive(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		t.Run(fmt.Sprint(failCleanup), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "state")
			s := otelSite()
			s.Nginx = nil
			s.IIS = &IIS{Site: "Orders", Application: "/"}
			s.OTel.ModulePath, s.OTel.AgentPath, s.OTel.Runtime = "", `C:\external-agent`, "netframework"
			off := false
			s.RUM.Enabled = &off
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			disabled := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":"managed-mapping","compressionPresent":false,"compressionValue":null,"otel":{"pool":"Dedicated","environment":{}}}`)
			active := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":"managed-mapping","compressionPresent":false,"compressionValue":null,"otel":{"pool":"Dedicated","environment":{"OTEL_SERVICE_NAME":"web"}}}`)
			st.Current.OTelEnabled = true
			st.Current.Native, st.OriginalIIS = active, original
			if err = saveState(root, st); err != nil {
				t.Fatal(err)
			}
			live := active
			removed := false
			saved := iisCommand
			t.Cleanup(func() { iisCommand = saved })
			iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
				switch action {
				case "inspect", "check":
					return live, nil
				case "apply":
					live = disabled
				case "restore":
					if !sameJSON(extra["expected"].(json.RawMessage), live) {
						t.Fatal("uninstall lost compare-before-write")
					}
					live = extra["snapshot"].(json.RawMessage)
					removed = extra["removeMapping"] == true
				case "activate":
					if removed && failCleanup {
						failCleanup = false
						return nil, errors.New("cleanup activation failure")
					}
				default:
					t.Fatal("unexpected action", action)
				}
				return live, nil
			}
			failure := failCleanup
			err = uninstallSite(root, s, false, true, false, nil, io.Discard)
			if failure {
				if err == nil {
					t.Fatal("cleanup failure was ignored")
				}
				if !sameJSON(live, disabled) {
					t.Fatal("failed cleanup did not restore retained mapping")
				}
				if _, err = loadState(root, s.ID); err != nil {
					t.Fatal("failed uninstall lost state", err)
				}
				err = uninstallSite(root, s, false, true, false, nil, io.Discard)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !sameJSON(live, original) {
				t.Fatal("original IIS configuration not restored")
			}
			if _, err = loadState(root, s.ID); !os.IsNotExist(err) {
				t.Fatal("live state not archived", err)
			}
			archives, _ := filepath.Glob(filepath.Join(root, "removed", s.ID+"-*", "state.json"))
			if len(archives) != 1 {
				t.Fatal("backup not preserved")
			}
			if err = uninstallSite(root, s, false, true, false, nil, io.Discard); err != nil {
				t.Fatal("uninstall not idempotent", err)
			}
		})
	}
}

func TestIISUninstallRetainsPreexistingAssetMapping(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Existing", Application: "/"}
	off := false
	s.RUM.Enabled = &off
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	s.PublicURL = server.URL + "/"
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	original := json.RawMessage(`{"rules":[],"preconditions":[],"virtualDirectory":"pre-existing mapping","compressionPresent":false,"compressionValue":null}`)
	st.Current.Native, st.OriginalIIS = original, original
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	saved := iisCommand
	t.Cleanup(func() { iisCommand = saved })
	iisCommand = func(_ string, _ Site, _ string, _ map[string]any) (json.RawMessage, error) { return original, nil }
	if err = uninstallSite(root, s, false, true, false, nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err = loadState(root, s.ID); !os.IsNotExist(err) {
		t.Fatal("deployment state not retired", err)
	}
	for name := range st.Current.Assets {
		if _, err = os.Stat(filepath.Join(siteDir(root, s), filepath.FromSlash(name))); err != nil {
			t.Fatal("pre-existing mapping's assets moved", err)
		}
	}
}

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
