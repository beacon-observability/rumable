package rumable

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSDKOptionsSurviveTOMLAndDeploymentState(t *testing.T) {
	config := strings.NewReplacer(
		"trackUserInteractions = true", "trackUserInteractions = false",
		`defaultPrivacyLevel = "mask"`, `defaultPrivacyLevel = "mask-user-input"`,
		"sessionOnErrorSampleRate = 0", "sessionOnErrorSampleRate = 12.5",
		"sessionReplayOnErrorSampleRate = 0", "sessionReplayOnErrorSampleRate = 75",
		"allowedTracingUrls = []", `allowedTracingUrls = ["https://api.example.com/", {match = "https://api.example.com/v2/", traceType = "w3c_traceparent"}]`,
		"replayCanvasQuality = 0.4", `replayCanvasQuality = "high"`,
		"replayCanvasSampling = 2", `replayCanvasSampling = "all"`,
	).Replace(exampleConfig) + `
[sites.rum.visitorId]
enabled = true
header = "x-rum-visitor-id"
match = ["https://api.example.com/"]
ttlHours = 12.5
`
	name := filepath.Join(t.TempDir(), "options.toml")
	if err := os.WriteFile(name, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := loadConfig(name, "example")
	if err != nil {
		t.Fatal(err)
	}
	got := s.RUM.sdkConfig()
	if got["trackUserInteractions"] != false || got["defaultPrivacyLevel"] != "mask-user-input" || got["sessionOnErrorSampleRate"] != 12.5 {
		t.Fatalf("explicit options were lost: %v", got)
	}
	if got["sessionReplayOnErrorSampleRate"] != float64(0) || s.RUM.startsReplay() {
		t.Fatal("error replay bypassed the disabled replay switch")
	}
	if s.RUM.VisitorID == nil || !s.RUM.VisitorID.Enabled || len(s.RUM.AllowedTracingURLs) != 2 || got["replayCanvasQuality"] != "high" || got["replayCanvasSampling"] != "all" {
		t.Fatal("nested or union SDK options were lost")
	}
	root := t.TempDir()
	if _, err := prepare(root, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadState(root, s.ID)
	if err != nil || !reflect.DeepEqual(got, loaded.Current.Site.RUM.sdkConfig()) {
		t.Fatalf("SDK options changed during persistence: %v", err)
	}
	s.RUM.SessionReplayEnabled = true
	zero := 0.0
	s.RUM.SessionReplaySampleRate = &zero
	if !s.RUM.startsReplay() || s.RUM.replayRate() != 0 || s.RUM.replayOnErrorRate() != 75 {
		t.Fatal("error-only replay did not start")
	}
	for _, change := range []struct{ old, next string }{
		{"sessionOnErrorSampleRate = 12.5", "sessionOnErrorSampleRate = nan"},
		{"compressIntakeRequests = false\nsendContentTypeByJson = false", "compressIntakeRequests = true\nsendContentTypeByJson = true"},
		{"tracingSampleRate = 100", "tracingSampleRate = -1"},
		{`defaultPrivacyLevel = "mask-user-input"`, `defaultPrivacyLevel = "typo"`},
		{`sessionPersistence = "cookie"`, `sessionPersistence = "session-storage"`},
		{`traceType = "ddtrace"`, `traceType = "unknown"`},
		{"enableExperimentalFeatures = []", `enableExperimentalFeatures = ["unknown"]`},
		{"replayCanvasMaxConcurrentEncodes = 1", "replayCanvasMaxConcurrentEncodes = 0"},
		{`replayCanvasQuality = "high"`, "replayCanvasQuality = inf"},
		{`replayCanvasSampling = "all"`, "replayCanvasSampling = -1"},
		{`header = "x-rum-visitor-id"`, `header = "authorization"`},
		{`header = "x-rum-visitor-id"`, `header = "traceparent"`},
		{"ttlHours = 12.5", "ttlHours = 0"},
		{`match = "https://api.example.com/v2/", traceType`, `match = "https://api.example.com/v2/", typo = true, traceType`},
		{`"https://api.example.com/", {match`, `42, {match`},
	} {
		if err := os.WriteFile(name, []byte(strings.ReplaceAll(config, change.old, change.next)), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(name, "example"); err == nil {
			t.Fatalf("invalid SDK option accepted: %s", change.next)
		}
	}
}

func TestWorkerRequirementsFollowSDKFeatures(t *testing.T) {
	sameOrigin := testSite()
	yes := true
	sameOrigin.RUM.CompressIntakeRequests = &yes
	sameOrigin.RUM.WorkerURL = "https://EXAMPLE.com:443/worker.js"
	selfPolicy := Page{Header: http.Header{"Content-Security-Policy": []string{"worker-src 'self'"}}}
	if err := sameOrigin.validate(); err != nil {
		t.Fatal(err)
	}
	if err := checkCSP(sameOrigin, selfPolicy); err != nil {
		t.Fatal("explicit default port changed worker origin", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/invalid.js" {
			w.Header().Set("Content-Type", "text/html")
		} else {
			w.Header().Set("Content-Type", "application/javascript")
		}
		w.Write([]byte("/* hosted worker */"))
	}))
	defer server.Close()
	s := testSite()
	s.PublicURL = server.URL + "/"
	s.RUM.CompressIntakeRequests = &yes
	s.RUM.WorkerURL = "/worker.js"
	page := Page{Header: http.Header{"Content-Security-Policy": []string{"script-src 'self'; worker-src 'self'; connect-src https://rum.example.com"}}}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	if err := checkCSP(s, page); err != nil {
		t.Fatal("hosted worker incorrectly requires blob:", err)
	}
	if err := probeWorkers(s); err != nil {
		t.Fatal(err)
	}
	s.RUM.WorkerURL = "/invalid.js"
	if probeWorkers(s) == nil {
		t.Fatal("HTML accepted as a worker")
	}
	s.RUM.WorkerURL = "https://other.example.com/worker.js"
	if s.validate() == nil {
		t.Fatal("cross-origin worker accepted")
	}
	s.RUM.WorkerURL = "/worker.js"
	s.RUM.SessionReplayEnabled = true
	s.RUM.ReplayCanvasEnabled = &yes
	if checkCSP(s, page) == nil {
		t.Fatal("inline canvas worker escaped its CSP requirement")
	}
	s.RUM.ReplayCanvasWorkerURL = "/canvas-worker.js"
	if err := checkCSP(s, page); err != nil {
		t.Fatal(err)
	}
	if err := probeWorkers(s); err != nil {
		t.Fatal(err)
	}
	s.RUM.CompressIntakeRequests = nil
	s.RUM.SessionReplayEnabled = false
	s.RUM.RemoteConfiguration = &yes
	s.RUM.WorkerURL = ""
	if checkCSP(s, page) == nil {
		t.Fatal("remote configuration did not check potential worker use")
	}
	zero, errorRate := 0.0, 100.0
	s.RUM.RemoteConfiguration = nil
	s.RUM.SessionReplayEnabled = true
	s.RUM.SessionReplaySampleRate = &zero
	s.RUM.SessionReplayOnErrorSampleRate = &errorRate
	if checkCSP(s, page) == nil {
		t.Fatal("error-only replay did not check its worker")
	}
}
