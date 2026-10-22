package rumable

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

var Version = "0.1.6"

type Config struct {
	SchemaVersion int    `json:"schemaVersion" toml:"schemaVersion"`
	Sites         []Site `json:"sites" toml:"sites"`
}

type Site struct {
	ID         string    `json:"id" toml:"id"`
	PublicURL  string    `json:"publicURL" toml:"publicURL"`
	ProbePaths []string  `json:"probePaths" toml:"probePaths"`
	Include    []string  `json:"includePaths" toml:"includePaths"`
	Exclude    []string  `json:"excludePaths,omitempty" toml:"excludePaths"`
	Nginx      *Nginx    `json:"nginx,omitempty" toml:"nginx"`
	IIS        *IIS      `json:"iis,omitempty" toml:"iis"`
	RUM        RUM       `json:"rum" toml:"rum"`
	SDK        SDKConfig `json:"sdk,omitempty" toml:"sdk"`
	OTel       *OTel     `json:"otel,omitempty" toml:"otel"`
}

type SDKConfig struct {
	Source    string `json:"source,omitempty" toml:"source"`
	Loading   string `json:"loading,omitempty" toml:"loading"`
	URL       string `json:"url,omitempty" toml:"url"`
	Integrity bool   `json:"integrity,omitempty" toml:"integrity"`
}

func (c SDKConfig) validate() error {
	if c.Loading != "" && c.Loading != "async" && c.Loading != "sync" {
		return errors.New("sdk.loading must be async or sync")
	}
	if c.Source == "" || c.Source == "bundled" {
		if c.URL != "" || c.Integrity {
			return errors.New("sdk.url and sdk.integrity require sdk.source=cdn")
		}
		return nil
	}
	if c.Source != "cdn" {
		return errors.New("sdk.source must be bundled or cdn")
	}
	if c.URL != "" {
		u, err := endpoint(c.URL)
		if err != nil || u.Scheme != "https" || u.RawQuery != "" || u.ForceQuery || u.Path == "" || u.Path == "/" {
			return errors.New("sdk.url must be an HTTPS script URL without credentials, query or fragment")
		}
	}
	return nil
}

type Nginx struct {
	Binary   string   `json:"binary" toml:"binary"`
	Args     []string `json:"args,omitempty" toml:"args"`
	Reviewed bool     `json:"attachmentReviewed" toml:"attachmentReviewed"`
}

type IIS struct {
	Site                    string `json:"site" toml:"site"`
	Application             string `json:"application" toml:"application"`
	AllowCompressionChanges bool   `json:"allowCompressionChanges" toml:"allowCompressionChanges"`
}

func loadConfig(name, id string) (Site, error) {
	b, err := os.ReadFile(name)
	if err != nil {
		return Site{}, err
	}
	var c Config
	metadata, err := toml.Decode(string(b), &c)
	if err != nil {
		return Site{}, fmt.Errorf("configuration %s: %w", name, err)
	}
	var unknown []toml.Key
	for _, key := range metadata.Undecoded() {
		// Inline tables in interface slices keep child keys undecoded; RUM.validate checks their values.
		if len(key) == 4 && strings.EqualFold(key[0], "sites") && strings.EqualFold(key[1], "rum") && strings.EqualFold(key[2], "allowedTracingUrls") && contains([]string{"match", "traceType"}, key[3]) {
			continue
		}
		unknown = append(unknown, key)
	}
	if len(unknown) != 0 {
		return Site{}, fmt.Errorf("configuration %s: unknown keys %v", name, unknown)
	}
	if c.SchemaVersion != 1 {
		return Site{}, errors.New("schemaVersion must be 1")
	}
	seen := map[string]bool{}
	for _, s := range c.Sites {
		if seen[s.ID] {
			return Site{}, fmt.Errorf("duplicate site id %q", s.ID)
		}
		seen[s.ID] = true
		if err := s.validate(); err != nil {
			return Site{}, fmt.Errorf("site %q: %w", s.ID, err)
		}
	}
	for _, s := range c.Sites {
		if s.ID == id {
			return s, nil
		}
	}
	if id == "" {
		if len(c.Sites) == 1 {
			return c.Sites[0], nil
		}
		return Site{}, errors.New("configuration must contain one site, or select one with --site ID")
	}
	return Site{}, fmt.Errorf("site %q not found; use --site", id)
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,47}$`)

func (s Site) validate() error {
	if err := s.validateTarget(); err != nil {
		return err
	}
	u, _ := url.Parse(s.PublicURL)
	if u.ForceQuery {
		return errors.New("publicURL must have no query marker")
	}
	if err := s.SDK.validate(); err != nil {
		return err
	}
	if err := s.OTel.validate(s); err != nil {
		return err
	}
	if !s.RUM.isEnabled() {
		return nil
	}
	return s.RUM.validate(s.PublicURL)
}

func (s Site) validateTarget() error {
	if !idPattern.MatchString(s.ID) {
		return errors.New("id must start with a lowercase letter and contain at most 48 lowercase letters, digits or hyphens")
	}
	servers := 0
	for _, present := range []bool{s.Nginx != nil, s.IIS != nil} {
		if present {
			servers++
		}
	}
	if servers != 1 {
		return errors.New("configure exactly one of nginx or iis")
	}
	u, err := endpoint(s.PublicURL)
	if err != nil {
		return fmt.Errorf("publicURL: %w", err)
	}
	if u.RawQuery != "" || (u.Path != "" && !strings.HasSuffix(u.Path, "/")) {
		return errors.New("publicURL must end in / and have no query")
	}
	if strings.ContainsAny(u.Path, "\"'<>&\\%?#{} \t\r\n\x00") || strings.Contains(u.Path, "..") {
		return errors.New("publicURL path contains unsupported characters")
	}
	if len(s.Include) == 0 || len(s.ProbePaths) == 0 {
		return errors.New("includePaths and probePaths must not be empty")
	}
	for _, p := range append(append(append([]string{}, s.Include...), s.Exclude...), s.ProbePaths...) {
		if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\r\n?#\\\x00") || strings.Contains(p, "..") {
			return fmt.Errorf("invalid absolute path %q", p)
		}
	}
	for _, p := range s.ProbePaths {
		if !s.matches(p) {
			return fmt.Errorf("probe path %q is outside the selected paths", p)
		}
	}
	if s.Nginx != nil {
		if s.Nginx.Binary == "" {
			return errors.New("nginx.binary is required")
		}
		for _, a := range s.Nginx.Args {
			if a == "-s" || strings.HasPrefix(a, "-s") || a == "-g" || strings.HasPrefix(a, "-g") {
				return errors.New("nginx.args may only supply configuration/prefix options (-c, -p, -e)")
			}
		}
		for i := 0; i < len(s.Nginx.Args); i += 2 {
			if i+1 >= len(s.Nginx.Args) || (s.Nginx.Args[i] != "-c" && s.Nginx.Args[i] != "-p" && s.Nginx.Args[i] != "-e") {
				return errors.New("nginx.args must be option/value pairs using -c, -p, -e")
			}
		}
	}
	if s.IIS != nil {
		if s.IIS.Site == "" || !strings.HasPrefix(s.IIS.Application, "/") || strings.ContainsAny(s.IIS.Site+s.IIS.Application, "\r\n\x00") {
			return errors.New("iis.site and an absolute iis.application are required")
		}
		app := strings.TrimSuffix(s.IIS.Application, "/") + "/"
		if u.Path != app {
			return errors.New("publicURL path must equal iis.application followed by /")
		}
	}

	return nil
}

func endpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || strings.Contains(raw, "#") || strings.ContainsAny(raw, "\\\t\r\n\x00") {
		return nil, errors.New("expected an http(s) URL without credentials or fragment")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, errors.New("URL port must be between 1 and 65535")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, errors.New("URL port must not be empty")
	}
	return u, nil
}

func (s Site) reportURL() string {
	if s.RUM.DatakitOrigin != "" {
		return s.RUM.DatakitOrigin
	}
	return s.RUM.Site
}
func (s Site) platform() string {
	if s.Nginx != nil {
		return "nginx"
	}
	return "iis"
}
func (s Site) assetPrefix() string {
	u, _ := url.Parse(s.PublicURL)
	return strings.TrimRight(u.Path, "/") + "/__rumable__/" + s.ID + "/"
}
func (s Site) matches(p string) bool {
	if strings.HasPrefix(p, strings.TrimSuffix(s.assetPrefix(), s.ID+"/")) {
		return false
	}
	for _, x := range s.Exclude {
		if strings.HasPrefix(p, x) {
			return false
		}
	}
	for _, x := range s.Include {
		if strings.HasPrefix(p, x) {
			return true
		}
	}
	return false
}
func siteDir(root string, s Site) string { return filepath.Join(root, "sites", s.ID) }

const sdkExample = `[sites.sdk]
source = "bundled" # bundled (same origin) or cdn
loading = "async" # async (default) or sync (blocks HTML parsing)
# url = "https://static.guance.com/browser-sdk/v3/dataflux-rum.js"
integrity = false # CDN SRI requires Access-Control-Allow-Origin.

`

const exampleConfig = `schemaVersion = 1

[[sites]]
id = "example"
publicURL = "https://www.example.com/"
probePaths = ["/"]
includePaths = ["/"]
excludePaths = ["/api/", "/downloads/"]

[sites.nginx]
binary = "/usr/sbin/nginx"
attachmentReviewed = false

` + sdkExample + `[sites.rum]
applicationId = "REPLACE_APPLICATION_ID"
site = "https://REPLACE_OPENWAY_HOST"
clientToken = "REPLACE_CLIENT_TOKEN"
env = "production"
service = "browser"
version = "1.0.0"
sessionSampleRate = 100
# Session Replay is opt-in. Error replay also requires this switch.
sessionReplayEnabled = false
sessionReplaySampleRate = 100
sessionOnErrorSampleRate = 0
sessionReplayOnErrorSampleRate = 0
telemetrySampleRate = 100

# Collection and privacy.
trackUserInteractions = true
enablePrivacyForActionName = false
defaultPrivacyLevel = "mask"
# actionNameAttribute = "data-rum-action-name"
trackViewsManually = false
enableLongAnimationFrame = false
enableExperimentalFeatures = [] # Supported: "track_websockets"
excludedActivityUrls = [] # Absolute URLs; exact string matching by the SDK.

# Session cookies and context storage.
sessionPersistence = "cookie" # "cookie" or "local-storage"
useSecureSessionCookie = false # Requires HTTPS when enabled.
usePartitionedCrossSiteSessionCookie = false # Requires HTTPS when enabled.
trackSessionAcrossSubdomains = false
allowFallbackToLocalStorage = false # Remove sessionPersistence to permit automatic fallback.
storeContextsToLocal = false
# storeContextsKey = "rumable-example"

# Transport. Workers need CSP permission when the feature is enabled.
compressIntakeRequests = false
sendContentTypeByJson = false
# workerUrl = "/rum-worker.js" # Host the matching official 3.3.21 worker yourself.
# replayCanvasWorkerUrl = "/canvas-worker.js"
remoteConfiguration = false # Remote values may override static SDK settings.
silentMultipleInit = false # Rumable still never takes over an existing SDK.

# Trace headers are disabled while the allow list is empty.
allowedTracingUrls = []
# Example: ["https://api.example.com/", {match = "https://api.example.com/v2/", traceType = "w3c_traceparent"}]
traceType = "ddtrace"
tracingSampleRate = 100
traceId128Bit = false
allowTraceHeaderWithoutSession = false

# Canvas 2D recording. Requires Session Replay and a sampled session.
replayCanvasEnabled = false
replayCanvasMode = "auto" # "manual" requires a business snapshotCanvas() call.
replayCanvasSampling = 2 # Positive number or "all".
replayCanvasQuality = 0.4 # 0-1, or "low" / "medium" / "high" presets.
replayCanvasMimeType = "image/webp"
replayCanvasMaxCanvasSize = 1280
replayCanvasMaxEncodedBytes = 40000
replayCanvasMaxConcurrentEncodes = 1
replayCanvasFlushImmediately = false
replayCanvasAutoInterval = 250
replayCanvasAutoCooldown = 250
replayCanvasAutoUnchangedBackoff = 3000
replayCanvasAutoFailureBackoff = 5000
replayCanvasAutoMaxPerRun = 2

# Optional Visitor ID headers; keep this table after the other rum options.
# [sites.rum.visitorId]
# enabled = false
# header = "x-rum-visitor-id"
# match = ["https://api.example.com/"]
# ttlHours = 24
`
