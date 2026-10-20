package rumable

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// OTel describes server instrumentation. Credentials never enter browser assets.
type OTel struct {
	Enabled           bool         `json:"enabled" toml:"enabled"`
	Backend           string       `json:"backend" toml:"backend"`
	Endpoint          string       `json:"endpoint" toml:"endpoint"`
	ServiceName       string       `json:"serviceName" toml:"serviceName"`
	SampleRate        *float64     `json:"sampleRate,omitempty" toml:"sampleRate"`
	Include           []string     `json:"includePaths,omitempty" toml:"includePaths"`
	Exclude           []string     `json:"excludePaths,omitempty" toml:"excludePaths"`
	ModulePath        string       `json:"modulePath,omitempty" toml:"modulePath"`
	AgentPath         string       `json:"agentPath,omitempty" toml:"agentPath"`
	Runtime           string       `json:"runtime,omitempty" toml:"runtime"`
	CollectorEndpoint string       `json:"collectorEndpoint,omitempty" toml:"collectorEndpoint"`
	TokenEnv          string       `json:"tokenEnv,omitempty" toml:"tokenEnv"`
	TrustedCA         string       `json:"trustedCA,omitempty" toml:"trustedCA"`
	Install           *OTelInstall `json:"install,omitempty" toml:"install"`
}

func (o *OTel) rate() float64 {
	if o.SampleRate == nil {
		return 100
	}
	return *o.SampleRate
}

func (o *OTel) nativeEndpoint(s Site) string {
	if o.Backend == "dataway" && s.IIS == nil {
		return strings.TrimSuffix(o.CollectorEndpoint, "/")
	}
	return strings.TrimSuffix(o.Endpoint, "/")
}

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (o *OTel) validate(s Site) error {
	if o == nil {
		return nil
	}
	if err := o.Install.validate(); err != nil {
		return err
	}

	if o.Backend != "datakit" && o.Backend != "dataway" {
		return errors.New("otel.backend must be datakit or dataway")
	}
	if o.ServiceName == "" || strings.ContainsAny(o.ServiceName, "\x00\r\n${}") {
		return errors.New("otel.serviceName is required and cannot contain control characters or expansions")
	}
	if v := o.rate(); math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 100 {
		return errors.New("otel.sampleRate must be finite and between 0 and 100")
	}
	if s.Nginx != nil && math.Abs(o.rate()*100-math.Round(o.rate()*100)) > 1e-7 {
		return errors.New("Nginx otel.sampleRate supports at most two decimal places")
	}
	for _, raw := range []string{o.Endpoint, o.CollectorEndpoint} {
		if raw == "" {
			continue
		}
		u, err := endpoint(raw)
		if err != nil || u.RawQuery != "" || u.ForceQuery || u.RawPath != "" || strings.ContainsAny(raw, "${} \"'") {
			return errors.New("OTel endpoints must be http(s) URLs without credentials, query, expansions or fragment")
		}
	}
	u, err := endpoint(o.Endpoint)
	if err != nil {
		return errors.New("otel.endpoint is required")
	}
	if o.Backend == "datakit" {
		if u.Port() == "" {
			return errors.New("DataKit OTel gRPC endpoint must include its port")
		}
		if u.Path != "" && u.Path != "/" {
			return errors.New("DataKit OTel endpoint must be an OTLP/gRPC origin, normally http://127.0.0.1:4317")
		}
		if o.TokenEnv != "" || o.CollectorEndpoint != "" {
			return errors.New("tokenEnv and collectorEndpoint are only used with Dataway")
		}
	} else {
		if u.Path != "/otel" && u.Path != "/otel/" {
			return errors.New("Dataway endpoint must end in /otel (Dataway 1.17.0+)")
		}
		if !envName.MatchString(o.TokenEnv) {
			return errors.New("Dataway requires otel.tokenEnv: the environment variable containing the workspace token")
		}
		if s.IIS == nil {
			c, err := endpoint(o.CollectorEndpoint)
			if err != nil || c.Port() == "" || (c.Path != "" && c.Path != "/") {
				return errors.New("Nginx Dataway requires collectorEndpoint: an existing OTLP/gRPC Collector origin")
			}
		} else if o.CollectorEndpoint != "" {
			return errors.New("IIS sends OTLP/HTTP directly to Dataway; omit collectorEndpoint")
		}
	}
	if s.IIS != nil {
		if o.ModulePath != "" || o.TrustedCA != "" {
			return errors.New("IIS OTel accepts agentPath, with no modulePath or trustedCA (use Windows trust store)")
		}
		if o.Runtime != "netframework" && o.Runtime != "dotnet" {
			return errors.New("IIS otel.runtime must be netframework or dotnet")
		}
		if len(o.Include)+len(o.Exclude) != 0 {
			return errors.New("IIS OTel instruments the dedicated application pool; path filters are not supported")
		}
	} else {
		if (o.ModulePath != "" && !path.IsAbs(o.ModulePath)) || o.AgentPath != "" || o.Runtime != "" {
			return errors.New("Linux OTel modulePath must be absolute when set, with no agentPath or runtime")
		}

		if o.TrustedCA != "" && (!path.IsAbs(o.TrustedCA) || !strings.HasPrefix(o.nativeEndpoint(s), "https://")) {
			return errors.New("otel.trustedCA requires an absolute Linux CA path and an HTTPS native exporter endpoint")
		}

		if o.Backend == "dataway" {
			c, _ := url.Parse(o.CollectorEndpoint)
			if c.Scheme != "http" || (c.Hostname() != "127.0.0.1" && c.Hostname() != "localhost") {
				return errors.New("Dataway collectorEndpoint must use a local plaintext gRPC receiver (http://127.0.0.1:PORT); the generated receiver binds loopback")
			}
		}
		for _, p := range append(append([]string{}, o.Include...), o.Exclude...) {
			if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\x00\r\n?#\\") || strings.Contains(p, "..") {
				return errors.New("OTel paths must be absolute URL path prefixes")
			}
		}
	}
	for _, p := range []string{o.ModulePath, o.AgentPath, o.TrustedCA} {
		if strings.ContainsAny(p, "\x00\r\n${}") {
			return errors.New("OTel file paths cannot contain control characters or expansions")
		}
	}
	return nil
}

func (n Snapshot) active() bool { return n.Enabled || n.OTelEnabled }

func (n Snapshot) otelGeneration() string {
	b, _ := json.Marshal(struct {
		Enabled bool
		Config  *OTel
	}{n.OTelEnabled, n.Site.OTel})
	return hash(b)
}

func probeOTelState(n Snapshot) error {
	if n.Site.OTel == nil || n.Site.IIS != nil {
		return nil
	}
	p, err := getPage(pageURL(n.Site, n.Site.assetPrefix()+"otel-status.json"), nil)
	if err != nil {
		return err
	}
	if string(p.Body) != n.otelGeneration() {
		return errors.New("Nginx OTel configuration has not become active; waiting for graceful reload")
	}
	return nil
}

// Native Linux exporters have one process-wide SDK/exporter. Sites may toggle
// independently, but conflicting destinations/resources must never win by order.
func otelOwner(root string, s Site) (string, error) {
	if s.OTel == nil || s.IIS != nil {
		return s.ID, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "sites"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	owner := s.ID
	for _, e := range entries {
		if e.Name() == s.ID || !e.IsDir() {
			continue
		}
		if !idPattern.MatchString(e.Name()) {
			continue
		}
		st, err := loadState(root, e.Name())
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		x := st.Current.Site
		if x.OTel == nil || !sameServer(s, x) {
			continue
		}
		a, attachmentErr := loadAttachment(root, x)
		if attachmentErr == nil {
			if a.Phase == "detached" {
				continue
			}
			if a.Phase != "attached" {
				return "", fmt.Errorf("shared OTel site %s has an unfinished attachment; recover it first", x.ID)
			}
		} else if !os.IsNotExist(attachmentErr) {
			return "", fmt.Errorf("shared OTel site %s: %w", x.ID, attachmentErr)
		}
		if err := verifyFiles(root, st.Current); err != nil {
			return "", fmt.Errorf("shared OTel site %s: %w", x.ID, err)
		}
		if !sameExporter(s, x) {
			return "", fmt.Errorf("OTel exporter conflicts with site %s: this server instance shares serviceName, endpoint, backend, tokenEnv, modulePath and trustedCA", x.ID)
		}
		if st.Current.OTelOwner != "" {
			owner = st.Current.OTelOwner
		} else {
			owner = x.ID
		}
	}
	return owner, nil
}

func sameServer(a, b Site) bool {
	x, y := a, b
	x.ID, y.ID = "instance", "instance"
	x.PublicURL, y.PublicURL = "http://instance/", "http://instance/"
	return x.platform() == y.platform() && compatibleTarget(x, y)
}

func sameExporter(a, b Site) bool {
	x, y := a.OTel, b.OTel
	return x.Backend == y.Backend && strings.TrimSuffix(x.Endpoint, "/") == strings.TrimSuffix(y.Endpoint, "/") && strings.TrimSuffix(x.CollectorEndpoint, "/") == strings.TrimSuffix(y.CollectorEndpoint, "/") && x.ServiceName == y.ServiceName && x.ModulePath == y.ModulePath && x.TrustedCA == y.TrustedCA && x.TokenEnv == y.TokenEnv
}

func otelPreflight(root string, s Site) error {
	if s.OTel == nil || !s.OTel.Enabled {
		return nil
	}
	if err := s.OTel.validate(s); err != nil {
		return err
	}
	if _, err := otelOwner(root, s); err != nil {
		return err
	}
	if s.IIS == nil {
		if runtime.GOOS != "linux" {
			return errors.New("native Nginx OTel activation requires Linux")
		}
		f, err := os.Stat(s.OTel.ModulePath)
		if err != nil || !f.Mode().IsRegular() {
			return errors.New("OTel modulePath must point to an installed, version-compatible native module")
		}
		if err := checkOTelModule(s); err != nil {
			return err
		}
	}
	if s.IIS != nil && s.OTel.Backend == "dataway" && os.Getenv(s.OTel.TokenEnv) == "" {
		return errors.New("the environment variable named by otel.tokenEnv is missing")
	}
	u, _ := url.Parse(s.OTel.nativeEndpoint(s))
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(u.Hostname(), port), 3*time.Second)
	if err != nil {
		return errors.New("OTel receiver TCP connection failed; start DataKit or the generated Dataway Collector first (this check does not prove OTLP ingestion)")
	}
	return c.Close()
}

func checkOTelModule(s Site) error {
	if s.OTel == nil || s.IIS != nil {
		return nil
	}
	b, err := os.ReadFile(s.OTel.ModulePath)
	if err != nil {
		return fmt.Errorf("read installed OTel module: %w", err)
	}
	module, err := elf.NewFile(bytes.NewReader(b))
	if err != nil || module.Type != elf.ET_DYN {
		return errors.New("otel.modulePath must be a Linux ELF shared module")
	}

	if s.Nginx != nil && strings.HasPrefix(s.OTel.nativeEndpoint(s), "https://") && !bytes.Contains(b, []byte("trusted_certificate")) {
		return errors.New("HTTPS OTel export requires ngx_otel_module 0.1.2+ with TLS support; use a compatible module or a local plaintext Collector")
	}
	return nil
}

func renderOTelCollector(s Site, n *Snapshot, files map[string][]byte) {
	o := s.OTel
	if o == nil || o.Backend != "dataway" || s.IIS != nil {
		return
	}
	u, _ := url.Parse(o.CollectorEndpoint)
	// Conversion is local-only; never expose the plaintext receiver publicly.
	port := u.Port()
	if port == "" {
		port = "4317"
	}
	config := map[string]any{
		"receivers":  map[string]any{"otlp": map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "127.0.0.1:" + port}}}},
		"exporters":  map[string]any{"otlphttp/dataway": map[string]any{"endpoint": strings.TrimSuffix(o.Endpoint, "/"), "headers": map[string]string{"X-Token": "${env:" + o.TokenEnv + "}"}}},
		"processors": map[string]any{"batch": map[string]any{}},
		"service":    map[string]any{"pipelines": map[string]any{"traces": map[string]any{"receivers": []string{"otlp"}, "processors": []string{"batch"}, "exporters": []string{"otlphttp/dataway"}}}},
	}
	b, _ := json.MarshalIndent(config, "", "  ")
	name := "collector/" + s.ID + "/otel." + hash(b) + ".json"
	files[name] = b
	n.OTelCollector = name
}
