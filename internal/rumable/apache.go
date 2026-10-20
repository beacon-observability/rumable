package rumable

import (
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var apacheVersion = regexp.MustCompile(`Apache/(\d+)\.(\d+)\.(\d+)`)
var apacheInclude = regexp.MustCompile(`^\s*\((?:\*|[0-9]+)\) (.+)$`)

func apacheCheck(root string, s Site, enabling bool) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("Apache commands require Linux; render can run on any OS")
	}
	x := s.Apache
	if enabling {
		if err := attachmentReview(root, s, x.Reviewed); err != nil {
			return err
		}
	}
	dump, err := apacheInspect(s)
	if err != nil {
		return err
	}
	want := filepath.Clean(filepath.Join(siteDir(root, s), "apache.conf"))
	count := 0
	for _, line := range strings.Split(string(dump), "\n") {
		if match := apacheInclude.FindStringSubmatch(line); len(match) == 2 && filepath.Clean(match[1]) == want {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("expected exactly one effective Apache include %s, found %d; complete ATTACH.md", want, count)
	}
	return nil
}

func apacheInspect(s Site) ([]byte, error) {
	x := s.Apache
	v, err := command(x.Binary, append(append([]string{}, x.Args...), "-v")...)
	if err != nil {
		return nil, err
	}
	m := apacheVersion.FindStringSubmatch(string(v))
	if len(m) != 4 {
		return nil, fmt.Errorf("unable to identify Apache version")
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	if major != 2 || minor != 4 || patch < 23 {
		return nil, fmt.Errorf("Apache 2.4.23+ is required (Lua output filters and DUMP_INCLUDES)")
	}
	modules, err := command(x.Binary, append(append([]string{}, x.Args...), "-M")...)
	if err != nil {
		return nil, err
	}
	for _, module := range []string{"lua_module", "filter_module", "alias_module", "authz_core_module", "headers_module"} {
		if !regexp.MustCompile(`(?m)^\s*` + module + `\s+\((?:shared|static)\)\s*$`).Match(modules) {
			return nil, fmt.Errorf("Apache lacks loaded %s", module)
		}
	}
	// This configtest also validates the candidate before every graceful reload.
	dump, err := command(x.Binary, append(append([]string{}, x.Args...), "-t", "-D", "DUMP_INCLUDES")...)
	if err != nil {
		return nil, err
	}
	return dump, nil
}

func renderApache(root string, n *Snapshot, files map[string][]byte) error {
	s := n.Site
	d := filepath.ToSlash(siteDir(root, s))
	// Apache expands ${...} even inside quoted directive arguments.
	for _, value := range append(append([]string{d}, s.Include...), s.Exclude...) {
		if strings.ContainsAny(value, "\r\n\x00") || strings.Contains(value, "${") {
			return fmt.Errorf("Apache paths cannot contain control characters or ${...} expansions")
		}
	}
	pub := d + "/public/" + s.ID + "/"
	v := "RUMABLE_" + strings.ReplaceAll(s.ID, "-", "_")
	var b strings.Builder
	fmt.Fprintf(&b, "# Managed by Rumable. Versioned assets survive disable/rollback.\nAlias %s %s\n<Directory %s>\n    AllowOverride None\n    Options -Indexes -ExecCGI\n    Require all granted\n    SetHandler default-handler\n    <FilesMatch \"[.]js$\">\n        ForceType application/javascript\n    </FilesMatch>\n</Directory>\n", apacheQuote(s.assetPrefix()), apacheQuote(pub), apacheQuote(pub))
	if n.Enabled {
		template, _ := assets.ReadFile("assets/apache.lua")
		luaArray := func(values []string) string {
			var quoted []string
			for _, value := range values {
				quoted = append(quoted, luaQuote(value))
			}
			return "{" + strings.Join(quoted, ",") + "}"
		}
		filter := []byte(strings.NewReplacer("RUMABLE_INCLUDES", luaArray(s.Include), "RUMABLE_EXCLUDES", luaArray(append([]string{strings.TrimSuffix(s.assetPrefix(), s.ID+"/")}, s.Exclude...)), "RUMABLE_TAG", luaQuote(scriptTag(s, n.Script)), "RUMABLE_ENV", luaQuote(v)).Replace(string(template)))
		name := "filters/" + s.ID + "/filter." + hash(filter) + ".lua"
		files[name] = filter
		fmt.Fprintf(&b, "LuaOutputFilter %s %s inject\n", v, apacheQuote(d+"/"+name))
		var inc, exc []string
		for _, p := range s.Include {
			inc = append(inc, regexp.QuoteMeta(p))
		}
		for _, p := range append([]string{strings.TrimSuffix(s.assetPrefix(), s.ID+"/")}, s.Exclude...) {
			exc = append(exc, regexp.QuoteMeta(p))
		}
		// Suppress validators before the handler can turn a cached HTML request
		// into 304/206. These headers affect every resource in the selected paths.
		selected := "^(?!(?:" + strings.Join(exc, "|") + "))(?:" + strings.Join(inc, "|") + ")"
		fmt.Fprintf(&b, "<LocationMatch %s>\n", apacheQuote(selected))
		fmt.Fprintf(&b, "    FilterDeclare %s\n    FilterProvider %s %s \"(%%{REQUEST_METHOD} == 'HEAD' || (%%{REQUEST_METHOD} == 'GET' && resp('Content-Encoding') == '')) && %%{REQUEST_STATUS} == 200 && %%{CONTENT_TYPE} =~ m#(?i)^text/html(?:;|$)# && resp('Content-Disposition') !~ m#(?i)attachment#\"\n    FilterChain +%s\n", v, v, v, v)
		// HEAD metadata cleanup cannot depend on receiving a response body;
		// some handlers emit only EOS and never set the Lua request marker.
		clearHeaders := "expr=reqenv('" + v + "') == '1' || (%{REQUEST_METHOD} == 'HEAD' && %{REQUEST_STATUS} == 200 && %{CONTENT_TYPE} =~ m#(?i)^text/html(?:;|$)# && resp('Content-Disposition') !~ m#(?i)attachment#)"
		for _, header := range []string{"Content-Length", "ETag", "Last-Modified", "Content-MD5", "Digest", "Accept-Ranges"} {
			fmt.Fprintf(&b, "    Header unset %s %s\n    Header always unset %s %s\n", header, apacheQuote(clearHeaders), header, apacheQuote(clearHeaders))
		}
		for _, header := range []string{"If-None-Match", "If-Modified-Since", "Range", "If-Range"} {
			fmt.Fprintf(&b, "    RequestHeader unset %s \"expr=%%{REQUEST_METHOD} == 'GET' || %%{REQUEST_METHOD} == 'HEAD'\"\n", header)
		}
		b.WriteString("</LocationMatch>\n")
		b.WriteString("<IfModule cache_module>\n    CacheQuickHandler off\n")
		for _, prefix := range s.Include {
			fmt.Fprintf(&b, "    CacheDisable %s\n", apacheQuote(prefix))
		}
		b.WriteString("</IfModule>\n")
	}
	fmt.Fprintf(&b, "<Location %s>\n    FilterChain !\n    Header set Cache-Control \"public, max-age=31536000, immutable\"\n</Location>\n", apacheQuote(s.assetPrefix()))
	n.Files["apache.conf"] = []byte(b.String())
	return nil
}

func apacheQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range []byte(s) {
		if c < 32 || c == '\\' || c == '"' || c == 127 {
			fmt.Fprintf(&b, `\%03d`, c)
		} else {
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func apacheAttachment(root string, s Site) string {
	return fmt.Sprintf(`# Apache first attachment (Linux, Apache 2.4.23+)

Load mod_lua, mod_filter, mod_alias, mod_authz_core and mod_headers.
Debian/Ubuntu: apache.binary can be /usr/sbin/apache2ctl to retain envvars.
RHEL/container installations can use the native httpd binary with -f/-d/-D args.
Use the SAME binary, arguments, environment and privileged account as the service.

Inside ONLY the target VirtualHost, after existing Location/LocationMatch blocks:

    Include %s

Do not attach at global server scope or nest this include in Location/Directory.
It adds a named RESOURCE smart filter without replacing business chains.
Output compression must run at CONTENT_SET, after RESOURCE. On old Apache,
inherited AddOutputFilterByType DEFLATE can run too early. Use an explicit
typed chain in the existing compression config (do not duplicate it):

    FilterDeclare OUTPUT_GZIP CONTENT_SET
    FilterProvider OUTPUT_GZIP DEFLATE "%%{CONTENT_TYPE} =~ m|^text/html|"
    FilterChain +OUTPUT_GZIP

Review later Location,
.htaccess, FilterChain/SetOutputFilter,
LuaInherit and existing Lua filters for overrides or duplicate execution.
Add every HTML handler to probePaths. Only one Rumable deployment per VirtualHost
is recommended; use includePaths/excludePaths for scope.

Keep the reserved %s path anonymous and outside proxy/rewrite routes.
Its smart FilterChain is cleared. Existing SetOutputFilter/AddOutputFilter,
rewrite and handler rules must also exclude it; probes verify SDK byte hashes.
If ProxyPass / is used, add its exclusion BEFORE that catch-all rule:

    ProxyPass %s !

The Apache worker must read sites/%s/public and sites/%s/filters and traverse
their parents; only administrators may write them. Never expose the state root.
The content-hashed Lua files are private server assets, outside the public Alias.

Require uncompressed source HTML. Disable backend compression or manually arrange
Apache INFLATE before the Rumable filter, retaining existing business filters and
output DEFLATE. Do not blindly unset client Accept-Encoding: output compression
uses it too. Static .gz/.br rewrite routes must not bypass the HTML filter.
Encoded source responses are skipped and rejected by probes, never edited as text.

When enabled, selected GET/HEAD paths lose request If-None-Match, If-Modified-Since, Range
and If-Range before their handlers. This also affects non-HTML in those paths;
exclude static asset directories where conditional/range serving matters.
Eligible HTML loses Content-Length, ETag, Last-Modified, Content-MD5 and Digest;
HEAD clears HTML metadata too, without injecting a body.
Disabled mode restores normal header/conditional behavior and retains public assets.
Clear browser/CDN/Service Worker HTML caches on enable, disable, upgrade and rollback.
When mod_cache is loaded, enabled mode sets CacheQuickHandler off for this vhost
and CacheDisable for every includePaths prefix, including non-HTML/excluded URLs
within those prefixes. This prevents warm cache entries bypassing the filter or
storing deployment-specific HTML. Disable removes those managed cache directives.
Review this cache impact before attachment; later overrides are incompatible.

Set apache.attachmentReviewed=true only after this review. Run configtest and a
first manual graceful reload, then rumable enable (includes preflight). Later operations only
write managed files, configtest and use -k graceful; they do not stop the service.
`, apacheQuote(filepath.ToSlash(filepath.Join(siteDir(root, s), "apache.conf"))), s.assetPrefix(), apacheQuote(s.assetPrefix()), s.ID, s.ID)
}
