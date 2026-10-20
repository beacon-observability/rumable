package rumable

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

type RUM struct {
	ApplicationID                        string     `json:"applicationId" toml:"applicationId"`
	Site                                 string     `json:"site,omitempty" toml:"site"`
	ClientToken                          string     `json:"clientToken,omitempty" toml:"clientToken"`
	DatakitOrigin                        string     `json:"datakitOrigin,omitempty" toml:"datakitOrigin"`
	Env                                  string     `json:"env,omitempty" toml:"env"`
	Service                              string     `json:"service,omitempty" toml:"service"`
	Version                              string     `json:"version,omitempty" toml:"version"`
	SessionSampleRate                    *float64   `json:"sessionSampleRate,omitempty" toml:"sessionSampleRate"`
	SessionReplayEnabled                 bool       `json:"sessionReplayEnabled,omitempty" toml:"sessionReplayEnabled"`
	SessionReplaySampleRate              *float64   `json:"sessionReplaySampleRate,omitempty" toml:"sessionReplaySampleRate"`
	SessionOnErrorSampleRate             *float64   `json:"sessionOnErrorSampleRate,omitempty" toml:"sessionOnErrorSampleRate"`
	SessionReplayOnErrorSampleRate       *float64   `json:"sessionReplayOnErrorSampleRate,omitempty" toml:"sessionReplayOnErrorSampleRate"`
	TelemetrySampleRate                  *float64   `json:"telemetrySampleRate,omitempty" toml:"telemetrySampleRate"`
	TrackUserInteractions                *bool      `json:"trackUserInteractions,omitempty" toml:"trackUserInteractions"`
	ActionNameAttribute                  string     `json:"actionNameAttribute,omitempty" toml:"actionNameAttribute"`
	EnablePrivacyForActionName           *bool      `json:"enablePrivacyForActionName,omitempty" toml:"enablePrivacyForActionName"`
	TrackViewsManually                   *bool      `json:"trackViewsManually,omitempty" toml:"trackViewsManually"`
	EnableLongAnimationFrame             *bool      `json:"enableLongAnimationFrame,omitempty" toml:"enableLongAnimationFrame"`
	EnableExperimentalFeatures           []string   `json:"enableExperimentalFeatures,omitempty" toml:"enableExperimentalFeatures"`
	DefaultPrivacyLevel                  string     `json:"defaultPrivacyLevel,omitempty" toml:"defaultPrivacyLevel"`
	SessionPersistence                   string     `json:"sessionPersistence,omitempty" toml:"sessionPersistence"`
	UseSecureSessionCookie               *bool      `json:"useSecureSessionCookie,omitempty" toml:"useSecureSessionCookie"`
	UsePartitionedCrossSiteSessionCookie *bool      `json:"usePartitionedCrossSiteSessionCookie,omitempty" toml:"usePartitionedCrossSiteSessionCookie"`
	TrackSessionAcrossSubdomains         *bool      `json:"trackSessionAcrossSubdomains,omitempty" toml:"trackSessionAcrossSubdomains"`
	AllowFallbackToLocalStorage          *bool      `json:"allowFallbackToLocalStorage,omitempty" toml:"allowFallbackToLocalStorage"`
	StoreContextsToLocal                 *bool      `json:"storeContextsToLocal,omitempty" toml:"storeContextsToLocal"`
	StoreContextsKey                     string     `json:"storeContextsKey,omitempty" toml:"storeContextsKey"`
	CompressIntakeRequests               *bool      `json:"compressIntakeRequests,omitempty" toml:"compressIntakeRequests"`
	SendContentTypeByJSON                *bool      `json:"sendContentTypeByJson,omitempty" toml:"sendContentTypeByJson"`
	WorkerURL                            string     `json:"workerUrl,omitempty" toml:"workerUrl"`
	ReplayCanvasWorkerURL                string     `json:"replayCanvasWorkerUrl,omitempty" toml:"replayCanvasWorkerUrl"`
	RemoteConfiguration                  *bool      `json:"remoteConfiguration,omitempty" toml:"remoteConfiguration"`
	SilentMultipleInit                   *bool      `json:"silentMultipleInit,omitempty" toml:"silentMultipleInit"`
	AllowedTracingURLs                   []any      `json:"allowedTracingUrls,omitempty" toml:"allowedTracingUrls"`
	TraceType                            string     `json:"traceType,omitempty" toml:"traceType"`
	TracingSampleRate                    *float64   `json:"tracingSampleRate,omitempty" toml:"tracingSampleRate"`
	TraceID128Bit                        *bool      `json:"traceId128Bit,omitempty" toml:"traceId128Bit"`
	AllowTraceHeaderWithoutSession       *bool      `json:"allowTraceHeaderWithoutSession,omitempty" toml:"allowTraceHeaderWithoutSession"`
	ExcludedActivityURLs                 []string   `json:"excludedActivityUrls,omitempty" toml:"excludedActivityUrls"`
	VisitorID                            *VisitorID `json:"visitorId,omitempty" toml:"visitorId"`
	ReplayCanvasEnabled                  *bool      `json:"replayCanvasEnabled,omitempty" toml:"replayCanvasEnabled"`
	ReplayCanvasMode                     string     `json:"replayCanvasMode,omitempty" toml:"replayCanvasMode"`
	ReplayCanvasSampling                 any        `json:"replayCanvasSampling,omitempty" toml:"replayCanvasSampling"`
	ReplayCanvasQuality                  any        `json:"replayCanvasQuality,omitempty" toml:"replayCanvasQuality"`
	ReplayCanvasMimeType                 string     `json:"replayCanvasMimeType,omitempty" toml:"replayCanvasMimeType"`
	ReplayCanvasMaxCanvasSize            *float64   `json:"replayCanvasMaxCanvasSize,omitempty" toml:"replayCanvasMaxCanvasSize"`
	ReplayCanvasMaxEncodedBytes          *float64   `json:"replayCanvasMaxEncodedBytes,omitempty" toml:"replayCanvasMaxEncodedBytes"`
	ReplayCanvasMaxConcurrentEncodes     *float64   `json:"replayCanvasMaxConcurrentEncodes,omitempty" toml:"replayCanvasMaxConcurrentEncodes"`
	ReplayCanvasFlushImmediately         *bool      `json:"replayCanvasFlushImmediately,omitempty" toml:"replayCanvasFlushImmediately"`
	ReplayCanvasAutoInterval             *float64   `json:"replayCanvasAutoInterval,omitempty" toml:"replayCanvasAutoInterval"`
	ReplayCanvasAutoCooldown             *float64   `json:"replayCanvasAutoCooldown,omitempty" toml:"replayCanvasAutoCooldown"`
	ReplayCanvasAutoUnchangedBackoff     *float64   `json:"replayCanvasAutoUnchangedBackoff,omitempty" toml:"replayCanvasAutoUnchangedBackoff"`
	ReplayCanvasAutoFailureBackoff       *float64   `json:"replayCanvasAutoFailureBackoff,omitempty" toml:"replayCanvasAutoFailureBackoff"`
	ReplayCanvasAutoMaxPerRun            *float64   `json:"replayCanvasAutoMaxPerRun,omitempty" toml:"replayCanvasAutoMaxPerRun"`
}

type VisitorID struct {
	Enabled  bool     `json:"enabled" toml:"enabled"`
	Header   string   `json:"header,omitempty" toml:"header"`
	Match    []string `json:"match,omitempty" toml:"match"`
	TTLHours *float64 `json:"ttlHours,omitempty" toml:"ttlHours"`
}

func (r RUM) replayRate() float64 {
	if !r.SessionReplayEnabled {
		return 0
	}
	if r.SessionReplaySampleRate == nil {
		return 100
	}
	return *r.SessionReplaySampleRate
}

func enabled(v *bool) bool { return v != nil && *v }

func (r RUM) replayOnErrorRate() float64 {
	if !r.SessionReplayEnabled || r.SessionReplayOnErrorSampleRate == nil {
		return 0
	}
	return *r.SessionReplayOnErrorSampleRate
}

func (r RUM) startsReplay() bool {
	return r.SessionReplayEnabled && (r.replayRate() > 0 || r.replayOnErrorRate() > 0 || enabled(r.RemoteConfiguration))
}

func (r RUM) sdkConfig() map[string]any {
	config := map[string]any{
		"env": "", "service": "", "version": "", "sessionSampleRate": 100,
		"sessionOnErrorSampleRate": 0, "trackUserInteractions": true,
		"compressIntakeRequests": false, "allowedTracingUrls": []any{}, "defaultPrivacyLevel": "mask",
	}
	b, _ := json.Marshal(r)
	_ = json.Unmarshal(b, &config)
	delete(config, "sessionReplayEnabled")
	config["sessionReplaySampleRate"] = r.replayRate()
	config["sessionReplayOnErrorSampleRate"] = r.replayOnErrorRate()
	return config
}

func (r RUM) validate(publicURL string) error {
	if r.ApplicationID == "" {
		return errors.New("rum.applicationId is required")
	}
	if r.DatakitOrigin != "" {
		if r.Site != "" || r.ClientToken != "" {
			return errors.New("DataKit and OpenWay settings cannot be mixed")
		}
	} else if r.Site == "" || r.ClientToken == "" {
		return errors.New("OpenWay requires site and clientToken; DataKit requires datakitOrigin")
	}
	// The pinned SDK concatenates these values into query strings without encoding.
	for _, value := range []struct{ name, text string }{{"applicationId", r.ApplicationID}, {"clientToken", r.ClientToken}} {
		if strings.ContainsAny(value.text, "&#+%;") || strings.IndexFunc(value.text, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }) >= 0 {
			return fmt.Errorf("%s contains characters that SDK 3.3.21 cannot safely include in query parameters", value.name)
		}
	}
	report := r.DatakitOrigin
	if report == "" {
		report = r.Site
	}
	e, err := endpoint(report)
	if err != nil {
		return fmt.Errorf("report endpoint: %w", err)
	}
	if e.RawQuery != "" || e.ForceQuery || e.RawPath != "" || (e.Path != "" && e.Path != "/") {
		return errors.New("report endpoint must be an origin without path or query")
	}
	base, _ := url.Parse(publicURL)
	if base.Scheme == "https" && e.Scheme != "https" {
		return errors.New("HTTPS pages require an HTTPS reporting endpoint")
	}
	for _, sample := range []struct {
		name string
		rate *float64
	}{
		{"sessionSampleRate", r.SessionSampleRate}, {"sessionOnErrorSampleRate", r.SessionOnErrorSampleRate},
		{"sessionReplaySampleRate", r.SessionReplaySampleRate}, {"sessionReplayOnErrorSampleRate", r.SessionReplayOnErrorSampleRate},
		{"telemetrySampleRate", r.TelemetrySampleRate}, {"tracingSampleRate", r.TracingSampleRate},
	} {
		if v := sample.rate; v != nil && (!finite(*v) || *v < 0 || *v > 100) {
			return fmt.Errorf("%s must be between 0 and 100", sample.name)
		}
	}
	for _, option := range []struct {
		name, value string
		values      []string
	}{
		{"defaultPrivacyLevel", r.DefaultPrivacyLevel, []string{"allow", "mask", "mask-user-input"}},
		{"sessionPersistence", r.SessionPersistence, []string{"cookie", "local-storage"}},
		{"traceType", r.TraceType, traceTypes}, {"replayCanvasMode", r.ReplayCanvasMode, []string{"manual", "auto"}},
	} {
		if option.value != "" && !contains(option.values, option.value) {
			return fmt.Errorf("%s must be one of %v", option.name, option.values)
		}
	}
	for _, feature := range r.EnableExperimentalFeatures {
		if feature != "track_websockets" {
			return fmt.Errorf("unsupported SDK experimental feature %q", feature)
		}
	}
	if base.Scheme != "https" && (enabled(r.UseSecureSessionCookie) || enabled(r.UsePartitionedCrossSiteSessionCookie)) {
		return errors.New("secure or partitioned session cookies require HTTPS")
	}
	if enabled(r.CompressIntakeRequests) && enabled(r.SendContentTypeByJSON) {
		return errors.New("compressIntakeRequests and sendContentTypeByJson cannot both be true: SDK 3.3.21 bypasses compression in JSON mode")
	}
	for _, raw := range []string{r.WorkerURL, r.ReplayCanvasWorkerURL} {
		if raw != "" {
			if _, err := resolveWorkerURL(publicURL, raw); err != nil {
				return err
			}
		}
	}
	for _, raw := range r.ExcludedActivityURLs {
		if err := validateMatchURL(raw); err != nil {
			return fmt.Errorf("excludedActivityUrls: %w", err)
		}
	}
	for _, match := range r.AllowedTracingURLs {
		switch v := match.(type) {
		case string:
			if err := validateMatchURL(v); err != nil {
				return fmt.Errorf("allowedTracingUrls: %w", err)
			}
		case map[string]any:
			raw, ok := v["match"].(string)
			trace, traceOK := v["traceType"].(string)
			if len(v) != 2 || !ok || !traceOK || !contains(traceTypes, trace) {
				return errors.New("allowedTracingUrls objects require only match (URL string) and traceType")
			}
			if err := validateMatchURL(raw); err != nil {
				return fmt.Errorf("allowedTracingUrls.match: %w", err)
			}
		default:
			return errors.New("allowedTracingUrls accepts URL strings or {match, traceType} tables")
		}
	}
	if r.VisitorID != nil {
		v := r.VisitorID
		if v.Enabled && (v.Header == "" || len(v.Match) == 0) {
			return errors.New("visitorId.enabled requires header and non-empty match URLs")
		}
		if v.Header != "" && !validVisitorHeader(v.Header) {
			return errors.New("visitorId.header must be a lowercase writable HTTP header and cannot overwrite authentication, security or tracing headers")
		}
		for _, raw := range v.Match {
			if err := validateMatchURL(raw); err != nil {
				return fmt.Errorf("visitorId.match: %w", err)
			}
		}
		if v.TTLHours != nil && (!finite(*v.TTLHours) || *v.TTLHours <= 0) {
			return errors.New("visitorId.ttlHours must be a positive finite number")
		}
	}
	if r.ReplayCanvasQuality != nil {
		if name, ok := r.ReplayCanvasQuality.(string); ok {
			if !contains([]string{"low", "medium", "high"}, name) {
				return errors.New("replayCanvasQuality must be low, medium, high or a number from 0 to 1")
			}
		} else if n, ok := number(r.ReplayCanvasQuality); !ok || !finite(n) || n < 0 || n > 1 {
			return errors.New("replayCanvasQuality must be low, medium, high or a number from 0 to 1")
		}
	}
	if r.ReplayCanvasSampling != nil {
		if name, ok := r.ReplayCanvasSampling.(string); ok {
			if name != "all" {
				return errors.New("replayCanvasSampling must be all or a positive number")
			}
		} else if n, ok := number(r.ReplayCanvasSampling); !ok || !finite(n) || n <= 0 {
			return errors.New("replayCanvasSampling must be all or a positive finite number")
		}
	}
	for _, limit := range []struct {
		name  string
		value *float64
	}{
		{"replayCanvasMaxCanvasSize", r.ReplayCanvasMaxCanvasSize}, {"replayCanvasMaxEncodedBytes", r.ReplayCanvasMaxEncodedBytes},
		{"replayCanvasMaxConcurrentEncodes", r.ReplayCanvasMaxConcurrentEncodes}, {"replayCanvasAutoInterval", r.ReplayCanvasAutoInterval},
		{"replayCanvasAutoCooldown", r.ReplayCanvasAutoCooldown}, {"replayCanvasAutoUnchangedBackoff", r.ReplayCanvasAutoUnchangedBackoff},
		{"replayCanvasAutoFailureBackoff", r.ReplayCanvasAutoFailureBackoff}, {"replayCanvasAutoMaxPerRun", r.ReplayCanvasAutoMaxPerRun},
	} {
		if v := limit.value; v != nil && (!finite(*v) || *v <= 0) {
			return fmt.Errorf("%s must be a positive finite number", limit.name)
		}
	}
	if r.ReplayCanvasMimeType != "" && !contains([]string{"image/png", "image/jpeg", "image/webp"}, r.ReplayCanvasMimeType) {
		return errors.New("replayCanvasMimeType must be image/png, image/jpeg or image/webp")
	}
	return nil
}

var traceTypes = []string{"ddtrace", "zipkin", "zipkin_single_header", "w3c_traceparent", "w3c_traceparent_64bit", "skywalking_v3", "jaeger"}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return 0, false
}
func validateMatchURL(raw string) error {
	_, err := endpoint(raw)
	if err != nil {
		return errors.New("expected an absolute HTTP(S) URL without credentials or fragment")
	}
	return nil
}
func resolveWorkerURL(base, raw string) (string, error) {
	b, _ := url.Parse(base)
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" || (u.Scheme == "" && (!strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//"))) {
		return "", errors.New("worker URLs must be root-relative or absolute same-origin HTTP(S) URLs")
	}
	u = b.ResolveReference(u)
	if _, err := endpoint(u.String()); err != nil {
		return "", fmt.Errorf("worker URL: %w", err)
	}
	if u.Scheme != b.Scheme || !strings.EqualFold(u.Hostname(), b.Hostname()) || urlPort(u) != urlPort(b) {
		return "", errors.New("worker URLs must use the page origin")
	}
	return u.String(), nil
}
func urlPort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

var headerNamePattern = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9a-z-]+$")

func validVisitorHeader(header string) bool {
	if !headerNamePattern.MatchString(header) {
		return false
	}
	for _, prefix := range []string{"proxy-", "sec-", "x-datadog-", "x-b3-"} {
		if strings.HasPrefix(header, prefix) {
			return false
		}
	}
	for _, word := range strings.Fields("authorization authentication api-key api_key apikey auth-token auth_token access-token access_token session-token session_token refresh-token refresh_token id-token id_token security-token security_token csrf xsrf jwt signature") {
		if strings.Contains(header, word) {
			return false
		}
	}
	return !contains(strings.Fields("b3 traceparent tracestate uber-trace-id sw8 x-gc-trace-id x-gc-span-id x-rum-resource-id accept-charset accept-encoding access-control-request-headers access-control-request-method connection content-length cookie cookie2 date dnt expect host keep-alive origin permissions-policy referer te trailer transfer-encoding upgrade via"), header)
}

// A remote configuration may enable compression, so its worker requirements are checked conservatively.
func (r RUM) workerURLs(base string) []string {
	var workers []string
	if r.startsReplay() || enabled(r.CompressIntakeRequests) || enabled(r.RemoteConfiguration) {
		workers = append(workers, r.WorkerURL)
	}
	if r.startsReplay() && enabled(r.ReplayCanvasEnabled) {
		workers = append(workers, r.ReplayCanvasWorkerURL)
	}
	for i, raw := range workers {
		if raw == "" {
			workers[i] = "blob:"
		} else {
			workers[i], _ = resolveWorkerURL(base, raw)
		}
	}
	return workers
}
