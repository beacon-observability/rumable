package rumable

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Snapshot struct {
	Uninstall     bool              `json:"uninstall,omitempty"`
	Enabled       bool              `json:"enabled"`
	OTelEnabled   bool              `json:"otelEnabled,omitempty"`
	OTelOwner     string            `json:"otelOwner,omitempty"`
	OTelCollector string            `json:"otelCollector,omitempty"`
	OTelTokenHash string            `json:"otelTokenHash,omitempty"`
	Site          Site              `json:"site"`
	Files         map[string][]byte `json:"files"`
	Assets        map[string]string `json:"assets"`
	Script        string            `json:"script"`
	SDK           SDKInfo           `json:"sdk"`
	SDKURL        string            `json:"sdkURL,omitempty"`
	Native        json.RawMessage   `json:"native,omitempty"`
}

func render(root string, s Site, enabled bool) (Snapshot, map[string][]byte, error) {
	return renderDeployment(root, s, enabled, nil, false)
}

func renderDeployment(root string, s Site, enabled bool, previous *Snapshot, preserveBrowser bool) (Snapshot, map[string][]byte, error) {
	s = resolveOTelPaths(root, s)
	active := enabled
	enabled = enabled && s.RUM.isEnabled()
	n := Snapshot{Enabled: enabled, Site: s, Files: map[string][]byte{}, Assets: map[string]string{}}
	public := map[string][]byte{}
	if preserveBrowser {
		// OTel-only operations keep the exact browser release, including a saved
		// CDN URL or bootstrap produced by an older CLI. Its assets already exist.
		n.Script, n.SDK, n.SDKURL = previous.Script, previous.SDK, previous.SDKURL
		for name, digest := range previous.Assets {
			if strings.HasPrefix(name, "public/"+s.ID+"/") {
				n.Assets[name] = digest
			}
		}
	} else {
		b, info, err := sdk()
		if err != nil {
			return Snapshot{}, nil, err
		}
		sdkName := "sdk." + info.SHA256 + ".js"
		sdkURL := sdkLocation(s, info)
		boot := bootstrap(s, sdkURL, info)
		bootName := "init." + hash(boot) + ".js"
		public[filepath.Join("public", s.ID, sdkName)] = b
		public[filepath.Join("public", s.ID, bootName)] = boot
		lic, _ := assets.ReadFile("assets/SDK-LICENSE")
		public[filepath.Join("public", s.ID, "SDK-LICENSE."+hash(lic)+".txt")] = lic
		manifest, _ := json.MarshalIndent(info, "", "  ")
		public[filepath.Join("public", s.ID, "sdk."+info.SHA256+".json")] = manifest
		n.Script, n.SDK = s.assetPrefix()+bootName, info
		if s.SDK.Source == "cdn" {
			n.SDKURL = sdkURL
		}
	}
	if s.OTel != nil {
		n.OTelEnabled = active && s.OTel.Enabled
		if n.OTelEnabled && s.IIS != nil && s.OTel.Backend == "dataway" {
			n.OTelTokenHash = hash([]byte(os.Getenv(s.OTel.TokenEnv)))
		}
		owner, err := otelOwner(root, s)
		if err != nil {
			return Snapshot{}, nil, err
		}
		n.OTelOwner = owner
		renderOTelCollector(s, &n, public)
	}
	for name, data := range public {
		n.Assets[filepath.ToSlash(name)] = hash(data)
	}
	if s.Nginx != nil {
		// Once RUM handlers were attached, keep their inactive filter on disable.
		// Removing it would revive parent filters on cached-page SDK requests.
		rumPrepared := s.RUM.isEnabled() || previous != nil && bytes.Contains(previous.Files["location.conf"], []byte("sub_filter "))
		v := "rumable_" + strings.ReplaceAll(s.ID, "-", "_")
		var h bytes.Buffer
		fmt.Fprintf(&h, "# Managed by Rumable. Do not edit.\nmap $uri $%s_path {\n    default 0;\n", v)
		fmt.Fprintf(&h, "    %s 0;\n", nginxQuote("~^"+regexp.QuoteMeta(strings.TrimSuffix(s.assetPrefix(), s.ID+"/"))))
		for _, p := range s.Exclude {
			fmt.Fprintf(&h, "    %s 0;\n", nginxQuote("~^"+regexp.QuoteMeta(p)))
		}
		for _, p := range s.Include {
			fmt.Fprintf(&h, "    %s 1;\n", nginxQuote("~^"+regexp.QuoteMeta(p)))
		}
		fmt.Fprintf(&h, "}\nmap $sent_http_content_type $%s_html {\n    default 0;\n    text/html 1;\n    \"~*^text/html;\" 1;\n}\n", v)
		fmt.Fprintf(&h, "map $sent_http_content_disposition $%s_download {\n    default 0;\n    ~*attachment 1;\n}\n", v)
		fmt.Fprintf(&h, "map \"$%s_path:$%s_html:$request_method:$status:$sent_http_content_encoding:$%s_download\" $%s_tag {\n    default \"\";\n", v, v, v, v)
		if enabled {
			// HEAD needs the GET metadata cleanup. A regex avoids longer hash keys
			// exceeding the default 32-byte map bucket on legacy Nginx builds.
			fmt.Fprintf(&h, "    \"~^1:1:(GET|HEAD):200::0$\" %s;\n", nginxQuote(scriptTag(s, n.Script)))
		}
		h.WriteString("}\n")
		// Empty dynamic matches bypass the filter; excluded HTML must keep its bytes.
		fmt.Fprintf(&h, "map $%s_tag $%s_anchor {\n    default '<head>';\n    \"\" \"\";\n}\n", v, v)
		n.Files["http.conf"] = h.Bytes()
		assetDir := filepath.ToSlash(filepath.Join(siteDir(root, s), "public", s.ID)) + "/"
		// The reserved path always has an empty dynamic anchor. Override inherited
		// body filters: they can corrupt SDK bytes, and http-level sub_filter
		// inheritance crashes unpatched Nginx 1.9.4. expires preserves add_header inheritance.
		n.Files["server.conf"] = []byte(fmt.Sprintf("# Managed by Rumable. Assets survive disable/rollback.\nlocation ^~ %s {\n    alias %s;\n    types { application/javascript js; text/plain txt; application/json json; }\n    default_type application/octet-stream;\n    sub_filter '$%s_anchor' '';\n    expires 1y;\n}\n", nginxQuote(s.assetPrefix()), nginxQuote(assetDir), v))
		// Keep an explicit, inactive rule when disabled, so toggling never revives
		// inherited http-level filters on Nginx 1.9.4. Retained business rules are
		// attached explicitly alongside this include during first attachment.
		filter := fmt.Sprintf("# Managed by Rumable. Injection disabled; empty dynamic anchor.\nsub_filter '$%s_anchor' '';\n", v)
		if !rumPrepared {
			filter = "# RUM disabled in TOML.\n"
			n.Files["server.conf"] = []byte(strings.ReplaceAll(string(n.Files["server.conf"]), fmt.Sprintf("    sub_filter '$%s_anchor' '';\n", v), ""))
		}
		if enabled {
			filter = fmt.Sprintf("# Managed by Rumable. HTML is gated independently of inherited sub_filter_types.\nsub_filter_once on;\nsub_filter_last_modified off;\nsub_filter '$%s_anchor' '<head>$%s_tag';\netag off;\nif_modified_since off;\n", v, v)
		}
		n.Files["location.conf"] = []byte(filter)
		renderNginxOTel(&n)
	}
	return n, public, nil
}

func scriptTag(s Site, src string) string {
	return `<script async data-rumable="` + s.ID + `" src="` + src + `"></script>`
}
func nginxQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`).Replace(s) + `"`
}

func attachment(root string, s Site) string {
	if s.OTel != nil {
		guide := "# Server OTel attachment\n\nUse install-otel to prepare managed dependencies when modulePath/agentPath is omitted. See docs/otel-installation.md and docs/otel.md for native requirements, DataKit/Dataway and Collector lifecycle. Full site removal: uninstall; optionally --remove-dependencies for unchanged, unshared managed packages.\n"
		if s.IIS != nil {
			guide += "IIS OTel requires IIS 10 / Windows 10 or Server 2016+, Windows PowerShell 5.1, an installed .NET Agent and a dedicated application pool. Run enable as Administrator; only that pool is configured/recycled.\n"
		} else {
			guide += "Run attach --dry-run, review the exact native module/shared exporter/target-site changes, then run attach to confirm. OTel requires the managed attachment record; do not create unmanaged OTel directives from the RUM manual fallback.\n"
		}
		if !s.RUM.isEnabled() {
			return guide + "\nRUM is disabled: preserve existing compression, cache, business request headers and CSP. No HTML rewrite module or browser credentials are required. Run enable after attachment/dependency setup; use --component otel for later toggles and rollback for the previous successful deployment.\n"
		}
		return guide + "\nRUM is configured too. Its additional HTML requirements follow.\n\n" + rumAttachment(root, s)
	}
	return rumAttachment(root, s)
}

func rumAttachment(root string, s Site) string {
	d := filepath.ToSlash(siteDir(root, s))
	if s.Nginx == nil {
		return fmt.Sprintf("# IIS attachment\n\nTarget: %s%s\n\nRun enable (includes preflight) as Administrator using 64-bit PowerShell 3+ and URL Rewrite 2.0.\nThe tool manages only rumable-%s-* rules/preconditions, the application /__rumable__ virtual directory, and the local doStaticCompression attribute. Static assets must be anonymously readable with JavaScript MIME types; configure that in IIS before enable.\nReview compression impact before enabling: all static resources in the selected application may lose static compression.\n", s.IIS.Site, s.IIS.Application, s.ID)
	}
	return fmt.Sprintf(`# Nginx first attachment

Add the following includes once, in the actual serving configuration:

    http { include %s; ... }
    server { include %s; ... }
    location <existing HTML handler> { include %s; ... }

Do not create a competing location or replace existing proxy_pass/root/try_files.
Review every HTML handler selected by includePaths and every probePaths entry.

For proxy handlers, merge these into the existing proxy_set_header list at the
same effective level; keep ALL existing Host/auth/custom headers. Adding a new
proxy_set_header at a child level disables inheritance of its parent's list:

    proxy_set_header Accept-Encoding "";
    proxy_set_header If-None-Match "";
    proxy_set_header If-Modified-Since "";

For static handlers disable gzip_static/brotli_static if those modules are
installed; remove precompressed try_files routes. Output gzip can remain on.
Existing sub_filter rules must be repeated alongside the location include if
they should still apply; child directives replace inherited rules. The disabled
include also defines an inactive rule, keeping inheritance stable across toggles.
Rumable gates its injection to text/html even when inherited sub_filter_types is broader.
Nginx 1.9.4 check/enable automatically reject http-level sub_filter through
effective include scopes, regardless of attachmentReviewed. Move business rules
to server/location or upgrade to >=1.9.5. Missing or ambiguous dump/include
contexts also block activation; use explicit paths or simple * / ASCII ? masks.
Rumable assets explicitly bypass inherited body filters.

The enabled handler disables local validators; sub_filter removes ETag and
Last-Modified and the original Content-Length for eligible GET and HEAD HTML
without changing proxy_hide_header inheritance. HEAD emits no response body.
Local validator directives affect the entire attached handler, so keep
static asset handlers separate where caching matters. The tool cannot infer
all semantics of arbitrary Nginx configurations. Set attachmentReviewed=true
only after reviewing these conditions. Run nginx -t before the first reload.

The /__rumable__/ resource location uses expires for one-year caching and keeps
parent add_header security/CORS headers inherited, including on old Nginx.

Clear HTML caches in CDN/proxy/service workers on enable, disable and upgrade.
Old versioned SDK files remain available for cached pages. Do not delete them
until all HTML cache retention periods have expired.
`, nginxQuote(d+"/http.conf"), nginxQuote(d+"/server.conf"), nginxQuote(d+"/location.conf"))
}

func fileNames(m map[string][]byte) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
