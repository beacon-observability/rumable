package rumable

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeploymentProbeReconnectsAfterWorkerGenerationChanges(t *testing.T) {
	type workerGeneration struct{}
	var generation int32 = 1
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(workerGeneration{}).(int32) == 1 {
			_, _ = w.Write([]byte("old worker"))
		} else {
			_, _ = w.Write([]byte("new worker"))
		}
	}))
	server.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, workerGeneration{}, atomic.LoadInt32(&generation))
	}
	server.Start()
	defer server.Close()
	if _, err := getPage(server.URL, nil); err != nil {
		t.Fatal(err)
	}
	atomic.StoreInt32(&generation, 2)
	page, err := getPage(server.URL, nil)
	if err != nil || string(page.Body) != "new worker" {
		t.Fatal("deployment verification stayed pinned to the pre-reload connection", err)
	}
}

func otelSite() Site {
	s := testSite()
	s.OTel = &OTel{Enabled: true, Backend: "datakit", Endpoint: "http://127.0.0.1:4317", ServiceName: "web", ModulePath: "/opt/otel/ngx_otel_module.so"}
	return s
}

func TestOTelOnlyProbeDoesNotTreatJSONDocumentationAsInjection(t *testing.T) {
	s := otelSite()
	off := false
	s.RUM.Enabled = &off
	page := Page{Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`"<script data-rumable='test' src='/example.js'></script>"`)}
	result, err := inspectInjection(s, page, &Snapshot{Site: s, OTelEnabled: true})
	if err != nil || result.Injected {
		t.Fatal("JSON content was interpreted as an executable HTML script", err)
	}
}

func TestOTelReloadProbeRequiresNewWorkerConfiguration(t *testing.T) {
	generation := "old-worker-generation"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(generation))
	}))
	defer server.Close()
	s := otelSite()
	s.PublicURL = server.URL + "/"

	n := Snapshot{Site: s, OTelEnabled: true}
	if probeOTelState(n) == nil {
		t.Fatal("old worker accepted after OTel-only change")
	}
	generation = n.otelGeneration()
	if err := probeOTelState(n); err != nil {
		t.Fatal("active worker not recognized", err)
	}
	n.OTelEnabled = false
	if probeOTelState(n) == nil {
		t.Fatal("enable and disable share a deployment marker")
	}
}

func TestOTelActivationRequiresStableWorkerGeneration(t *testing.T) {
	s := otelSite()
	off := false
	s.RUM.Enabled = &off
	n := Snapshot{Site: s, OTelEnabled: true}
	var checks int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "otel-status.json") {
			if atomic.AddInt32(&checks, 1) == 2 {
				_, _ = w.Write([]byte("old worker"))
			} else {
				_, _ = w.Write([]byte(n.otelGeneration()))
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	n.Site.PublicURL = server.URL + "/"
	if err := waitProbes(n); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&checks) < 4 {
		t.Fatal("activation accepted a transient new worker before the old generation drained")
	}
}

func TestOTelExamplesLoadAndCollectorPreviewIsReadOnly(t *testing.T) {
	for _, kind := range []string{"nginx", "iis"} {
		s, err := loadConfig(filepath.Join("..", "..", "examples", "otel-"+kind+".toml"), "")
		if err != nil || s.RUM.isEnabled() || !s.OTel.Enabled {
			t.Fatalf("%s example: %v", kind, err)
		}
	}
	root := t.TempDir()
	file := filepath.Join(root, "site.toml")
	config := `schemaVersion=1
[[sites]]
id="test"
publicURL="http://example.com/"
probePaths=["/"]
includePaths=["/"]
[sites.nginx]
binary="nginx"
[sites.rum]
enabled=false
[sites.otel]
enabled=true
backend="dataway"
endpoint="https://dataway.example.com/otel"
collectorEndpoint="http://127.0.0.1:4317"
tokenEnv="WORKSPACE_TOKEN"
serviceName="nginx"
modulePath="/opt/otel/module.so"
`
	if err := os.WriteFile(file, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(root, "uncreated-state")
	var out strings.Builder
	if err := Run([]string{"render", "--config", file, "--state-dir", stateDir, "--collector-config"}, &out); err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if json.Unmarshal([]byte(out.String()), &c) != nil || c["exporters"] == nil {
		t.Fatal("Collector preview is not usable JSON")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "sites", "test", "state.json")); !os.IsNotExist(err) {
		t.Fatal("Collector preview registered a deployment")
	}
}

func TestOTelConfigurationBoundaries(t *testing.T) {
	for _, platform := range []string{"nginx", "iis"} {
		for _, backend := range []string{"datakit", "dataway"} {
			t.Run(platform+"/"+backend, func(t *testing.T) {
				s := otelSite()

				if platform == "iis" {
					s.Nginx = nil
					s.IIS = &IIS{Site: "Default Web Site", Application: "/"}
					s.OTel.ModulePath = ""
					s.OTel.AgentPath = `C:\otel`
					s.OTel.Runtime = "netframework"
				}
				s.OTel.Backend = backend
				if backend == "dataway" {
					s.OTel.Endpoint = "https://dataway.example.com/otel"
					s.OTel.TokenEnv = "TEST_DATAWAY_TOKEN"
					if platform != "iis" {
						s.OTel.CollectorEndpoint = "http://127.0.0.1:4317"
					}
				}
				// OTel-only operation must not require browser credentials or CSP changes.
				off := false
				s.RUM = RUM{Enabled: &off}
				if err := s.validate(); err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				var restored Site
				if err = json.Unmarshal(b, &restored); err != nil || restored.RUM.isEnabled() || restored.OTel.Backend != backend {
					t.Fatal("independent switches lost in persisted state", err)
				}
			})
		}
	}
	for _, change := range []func(*Site){
		func(s *Site) { s.OTel.Backend = "openway" },
		func(s *Site) { s.OTel.Endpoint = "https://token@host:4317" },
		func(s *Site) { s.OTel.Endpoint = "http://host:4317/otel/v1/traces" },
		func(s *Site) { s.OTel.Endpoint = "http://host" },
		func(s *Site) { s.OTel.ServiceName = "${SECRET}" },
		func(s *Site) { s.OTel.TokenEnv = "UNUSED_SECRET" },
		func(s *Site) { v := math.NaN(); s.OTel.SampleRate = &v },
		func(s *Site) { v := 0.001; s.OTel.SampleRate = &v },
		func(s *Site) { s.OTel.ModulePath = "/opt/${SDK}/module.so" },
		func(s *Site) { s.OTel.Exclude = []string{"/../private"} },
		func(s *Site) { s.OTel.TrustedCA = "/etc/ssl/certs/ca-certificates.crt" },
		func(s *Site) {
			s.OTel.Backend = "dataway"
			s.OTel.Endpoint = "https://host/otel"
			s.OTel.TokenEnv = "TOKEN"
		},
	} {
		s := otelSite()
		change(&s)
		if s.validate() == nil {
			t.Fatalf("invalid OTel configuration accepted: %+v", s.OTel)
		}
	}
}

func TestOTelAndRUMRenderIndependently(t *testing.T) {
	for _, rumOn := range []bool{false, true} {
		for _, otelOn := range []bool{false, true} {
			s := otelSite()
			s.OTel.Endpoint += "/"
			s.RUM.Enabled = &rumOn
			s.OTel.Enabled = otelOn
			n, assets, err := render(t.TempDir(), s, true)
			if err != nil {
				t.Fatal(err)
			}
			if n.Enabled != rumOn || n.OTelEnabled != otelOn || n.active() != (rumOn || otelOn) {
				t.Fatal("render conflated RUM and server instrumentation")
			}
			if strings.Contains(string(n.Files["http.conf"]), "data-rumable=") != rumOn {
				t.Fatal("RUM injection did not follow its switch")
			}
			if !strings.Contains(string(n.Files["http.conf"]), `endpoint "127.0.0.1:4317";`) {
				t.Fatal("plaintext endpoint is incompatible with the 0.1.0 gRPC target resolver")
			}
			if strings.Contains(string(n.Files["location.conf"]), "otel_trace $rumable_test_otel_selected") != otelOn {
				t.Fatal("native tracing did not follow its switch")
			}
			if err = validateSnapshot(n, s.ID); err != nil {
				t.Fatal(err)
			}
			for name, b := range assets {
				if strings.HasSuffix(name, ".js") && strings.HasPrefix(filepath.Base(name), "init.") && strings.Contains(string(b), `"enabled"`) {
					t.Fatal("CLI enabled flag leaked into SDK init")
				}
			}
		}
	}
	s := otelSite()
	s.OTel.Enabled = true
	n, _, _ := render(t.TempDir(), s, true)
	for _, c := range []string{"rum", "otel"} {
		desired := s
		changedOTel := *s.OTel
		desired.OTel = &changedOTel
		desired.OTel.ServiceName = "unrelated-exporter-edit"
		desired.RUM.ApplicationID = "unrelated-browser-edit"
		selected, on, err := componentSite(desired, n, c, false)
		if err != nil || !on {
			t.Fatal(err)
		}
		next, _, err := render(t.TempDir(), selected, on)
		if err != nil {
			t.Fatal(err)
		}
		if c == "rum" && (!next.OTelEnabled || next.Enabled) || c == "otel" && (next.OTelEnabled || !next.Enabled) {
			t.Fatal("component disable changed its peer")
		}
		if !s.RUM.isEnabled() || !s.OTel.Enabled {
			t.Fatal("component selection mutated deployment history")
		}
		if c == "rum" && selected.OTel.ServiceName != s.OTel.ServiceName || c == "otel" && selected.RUM.ApplicationID != s.RUM.ApplicationID {
			t.Fatal("component operation applied unrelated component parameters")
		}
	}
}

func TestOTelCollectorCredentialsStayPrivate(t *testing.T) {
	s := otelSite()
	s.OTel.Backend = "dataway"
	s.OTel.Endpoint = "https://dataway.example.com/otel"
	s.OTel.CollectorEndpoint = "http://127.0.0.1:4317"
	s.OTel.TokenEnv = "TEST_OTEL_WORKSPACE_TOKEN"
	t.Setenv(s.OTel.TokenEnv, "secret-value-must-never-be-rendered")
	n, files, err := render(t.TempDir(), s, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateSnapshot(n, s.ID); err != nil {
		t.Fatal(err)
	}
	for name, b := range files {
		if strings.Contains(string(b), os.Getenv(s.OTel.TokenEnv)) {
			t.Fatal("workspace secret embedded in an asset")
		}
		if strings.HasPrefix(name, "public/") && (strings.Contains(string(b), s.OTel.TokenEnv) || strings.Contains(string(b), "dataway.example.com")) {
			t.Fatal("server exporter configuration leaked into public SDK assets")
		}
	}
	var config map[string]any
	if json.Unmarshal(files[n.OTelCollector], &config) != nil || config["exporters"] == nil {
		t.Fatal("Collector configuration is not valid JSON/YAML")
	}
	if !strings.Contains(string(files[n.OTelCollector]), "${env:"+s.OTel.TokenEnv+"}") {
		t.Fatal("Collector lost its environment credential reference")
	}
	n.OTelCollector = "public/test/not-a-collector.json"
	if validateSnapshot(n, s.ID) == nil {
		t.Fatal("Collector reference accepted outside private assets")
	}
}

func TestIISDatawayTokenRotationChangesDeployment(t *testing.T) {
	s := otelSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Orders", Application: "/"}
	s.OTel.ModulePath = ""
	s.OTel.AgentPath = `C:\otel`
	s.OTel.Runtime = "netframework"
	s.OTel.Backend = "dataway"
	s.OTel.Endpoint = "https://dataway.example.com/otel"
	s.OTel.TokenEnv = "RUMABLE_ROTATION_TEST"
	t.Setenv(s.OTel.TokenEnv, "first-token")
	first, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(s.OTel.TokenEnv, "rotated-token")
	next, _, err := render(t.TempDir(), s, true)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotsEqual(first, next) {
		t.Fatal("rotated token would be skipped as an idempotent enable")
	}
	if first.OTelTokenHash == "first-token" || next.OTelTokenHash != hash([]byte("rotated-token")) {
		t.Fatal("credential fingerprint is not an irreversible deployment marker")
	}
}

func TestOTelSharedExporterIsolationAndRecovery(t *testing.T) {
	root := t.TempDir()
	first := otelSite()
	st, err := prepare(root, first)
	if err != nil {
		t.Fatal(err)
	}
	second := otelSite()
	second.ID = "second"
	second.PublicURL = "https://second.example.com/"
	second.OTel.Endpoint += "/"
	n, _, err := render(root, second, true)
	if err != nil {
		t.Fatal(err)
	}
	if n.OTelOwner != first.ID || strings.Contains(string(n.Files["http.conf"]), "otel_exporter {") {
		t.Fatal("second site duplicated the process-wide exporter")
	}
	second.OTel.Endpoint = "http://127.0.0.1:9999"
	if _, _, err = render(root, second, true); err == nil {
		t.Fatal("conflicting exporter silently took over another site")
	}
	// Recovery must stop before touching files when an operator modifies an OTel rule.
	after, _, err := render(root, first, true)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range after.Files {
		if err = os.WriteFile(filepath.Join(siteDir(root, first), name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(siteDir(root, first), "location.conf")
	foreign := []byte("otel_trace on; # external change\n")
	if err = os.WriteFile(path, foreign, 0644); err != nil {
		t.Fatal(err)
	}
	if err = restoreTransaction(root, transaction{Before: st, After: after}); err == nil {
		t.Fatal("rollback overwrote external OTel configuration")
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(foreign) {
		t.Fatal("external configuration lost")
	}
}

func TestOTelDetachedSiteReleasesExporterReservation(t *testing.T) {
	root := t.TempDir()
	first := otelSite()
	if _, err := prepare(root, first); err != nil {
		t.Fatal(err)
	}
	next := otelSite()
	next.ID = "replacement"
	next.PublicURL = "https://replacement.example.com/"
	next.OTel.Endpoint = "http://127.0.0.1:5317"
	if _, _, err := render(root, next, false); err == nil {
		t.Fatal("reserved exporter was replaced before detachment")
	}
	a := attachmentRecord{SchemaVersion: 1, Site: first, Phase: "applying", Files: []attachmentFile{{Path: filepath.Join(root, "native.conf"), Before: []byte("original"), After: []byte("attached"), Mode: 0644}}}
	if err := writeAttachment(attachmentPath(root, first), a); err != nil {
		t.Fatal(err)
	}
	if _, _, err := render(root, next, false); err == nil {
		t.Fatal("unfinished attachment did not block shared exporter changes")
	}
	a.Phase = "detached"
	if err := writeAttachment(attachmentPath(root, first), a); err != nil {
		t.Fatal(err)
	}
	n, _, err := render(root, next, false)
	if err != nil || n.OTelOwner != next.ID || !strings.Contains(string(n.Files["http.conf"]), "otel_exporter {") {
		t.Fatal("detached site stranded the previous exporter reservation", err)
	}
}

func TestOTelAttachmentRecoveryAcceptsCommittedPeerSwitchOnly(t *testing.T) {
	root := t.TempDir()
	s := otelSite()
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(siteDir(root, s), "location.conf")
	peer := s
	peer.ID = "peer"
	peer.PublicURL = "https://peer.example.com/"
	identity, err := attachmentIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	a := attachmentRecord{Site: peer, Inputs: map[string]string{name: hash(st.Current.Files["location.conf"])}, Identities: map[string]string{name: identity}}
	n, files, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = deployAssets(root, s, files); err != nil {
		t.Fatal(err)
	}
	for name, b := range n.Files {
		if err = atomicWrite(filepath.Join(siteDir(root, s), name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	st.Current = n
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	if err = verifyAttachmentSources(root, a); err != nil {
		t.Fatal("committed peer enable blocked attachment recovery", err)
	}
	if err = os.WriteFile(name, []byte("otel_exporter { endpoint external:4317; }"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifyAttachmentSources(root, a) == nil {
		t.Fatal("attachment recovery trusted an uncommitted peer edit")
	}
}

func TestOTelOnlyNginxAttachmentPreservesResponsePipeline(t *testing.T) {
	s := otelSite()
	off := false
	s.RUM.Enabled = &off
	c := nginxAttachFixture(t, `events {} http {
 gzip on; gzip_static on;
 proxy_set_header Host business.test;
 proxy_set_header Accept-Encoding gzip;
 proxy_cache_path /cache keys_zone=pages:1m;
 server {listen 443;server_name example.com;
  location / {proxy_pass http://backend;proxy_cache pages;}
 }
}`)
	files, _, err := c.planNginx(t.TempDir(), s, "", "--with-http_gzip_static_module")
	if err != nil {
		t.Fatal(err)
	}
	changed := string(files[0].After)
	for _, directive := range []string{"gzip on;", "gzip_static on;", "proxy_set_header Host business.test;", "proxy_set_header Accept-Encoding gzip;", "proxy_cache pages;"} {
		if strings.Count(changed, directive) != 1 {
			t.Fatal("OTel-only attachment changed the response pipeline", changed)
		}
	}
	if strings.Contains(changed, "proxy_cache off") || strings.Contains(changed, "If-None-Match") {
		t.Fatal("OTel-only attachment introduced RUM cache overrides")
	}
}
