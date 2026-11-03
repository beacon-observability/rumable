package rumable

import (
	"bytes"
	"fmt"
	"mime"
	"regexp"
)

// Resolved once per enable transaction. Rendering, local retained assets and
// SRI must all describe the same bytes; historical probes never resolve anew.
type sdkBundle struct {
	body []byte
	info SDKInfo
}

var cdnSDKVersion = regexp.MustCompile(`sdkVersion:"(3\.[0-9]+\.[0-9]+)",sdkName:"df_web_rum_sdk"`)

func resolveCDNSDK(s Site) (*sdkBundle, error) {
	if err := s.SDK.validate(); err != nil {
		return nil, err
	}
	_, embedded, err := sdk()
	if err != nil {
		return nil, err
	}
	raw := sdkLocation(s, embedded)
	p, err := getPage(raw, nil)
	if err != nil {
		return nil, fmt.Errorf("CDN SDK: %w", err)
	}
	kind, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if !javascriptMIME(kind) {
		return nil, fmt.Errorf("CDN SDK is not served as JavaScript")
	}
	versions := cdnSDKVersion.FindAllSubmatch(p.Body, -1)
	if len(versions) != 1 || !bytes.Contains(p.Body, []byte("DATAFLUX_RUM")) || !bytes.Contains(p.Body, []byte("startSessionReplayRecording")) {
		return nil, fmt.Errorf("CDN SDK does not match the expected stable v3 version and SDK markers; verify the configured HTTPS source and SDK compatibility")
	}
	info := SDKInfo{Version: string(versions[0][1]), Source: raw, SHA256: hash(p.Body)}
	return &sdkBundle{body: p.Body, info: info}, nil
}
