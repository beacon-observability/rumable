package rumable

import (
	"fmt"
	"strings"
	"testing"
)

func TestNginx194EffectiveHTTPFilters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		main    string
		files   string
		blocked bool
	}{
		{"direct", `http { sub_filter a b; }`, "", true},
		{"multiline", "http {\n# sub_filter is unsafe here\nsub_filter\n'a'\n'b';\n}", "", true},
		{"quoted-directive", `http { "sub_filter" a b; }`, "", true},
		{"server-and-location", `http { server { sub_filter a b; location / { sub_filter a b; } } }`, "", false},
		{"comments-quotes-variables", `http {
 # sub_filter a b; } fake braces do not change scope
 map $uri $value { default "}; sub_filter a b; {"; sub_filter literal; }
 log_format sample '${request_uri}';
 server { location / { sub_filter "escaped\"}#;" 'keep'; } }
}`, "", false},
		{"relative-nested-glob", `http { include conf.d/*.conf; }`, `
# configuration file /etc/nginx/conf.d/site.conf:
include rules/rum.conf;
# configuration file /etc/nginx/rules/rum.conf:
sub_filter a b;
`, true},
		{"http-block-in-include", `include conf.d/http.conf;`, `
# configuration file /etc/nginx/conf.d/http.conf:
http { include rules.conf; }
# configuration file /etc/nginx/rules.conf:
sub_filter a b;
`, true},
		{"server-nested-glob", `http { include conf.d/*.conf; }`, `
# configuration file /etc/nginx/conf.d/site.conf:
server { include rules/rum.conf; }
# configuration file /etc/nginx/rules/rum.conf:
sub_filter a b;
`, false},
		{"absolute-quoted-path", `http { include "/custom/rules with spaces.conf"; }`, `
# configuration file /custom/rules with spaces.conf:
sub_filter a b;
`, true},
		{"same-file-two-scopes", `http { server { include shared.conf; } include shared.conf; }`, `
# configuration file /etc/nginx/shared.conf:
sub_filter a b;
# configuration file /etc/nginx/shared.conf:
sub_filter a b;
`, true},
		{"empty-glob", `http { include missing/*.conf; server {} }`, "", false},
		{"dot-file-not-selected-by-glob", `http { server { include conf.d/.private.conf; } include conf.d/*.conf; }`, `
# configuration file /etc/nginx/conf.d/.private.conf:
sub_filter a b;
`, false},
		{"question-mask", `http { include conf.d/?.conf; }`, `
# configuration file /etc/nginx/conf.d/a.conf:
sub_filter a b;
`, true},
		{"explicit-escaped-dot-mask", `http { server { include conf.d/.private.conf; } include conf.d/\.private*; }`, `
# configuration file /etc/nginx/conf.d/.private.conf:
sub_filter a b;
`, true},
		{"forged-dump-comment", `http { include shared.conf; }`, `
# configuration file /etc/nginx/shared.conf:
# configuration file /misleading-comment.conf:
sub_filter a b;
`, true},
		{"missing-include", `http { include lost.conf; }`, "", true},
		{"unmatched-block", `http { server {}`, "", true},
		{"unresolved-dump", `http { server {} }`, `
# configuration file /unresolved.conf:
sub_filter a b;
`, true},
		{"unsupported-posix-mask", `http { include conf.d/[[:alpha:]]*.conf; }`, "", true},
		{"unsupported-parent-mask", `http { include conf.d/*/../*.conf; }`, "", true},
		{"include-cycle", `http { include cycle.conf; }`, `
# configuration file /etc/nginx/cycle.conf:
include cycle.conf;
`, true},
		{"conflicting-duplicate", `http { include shared.conf; }`, `
# configuration file /etc/nginx/shared.conf:
# originally safe
# configuration file /etc/nginx/shared.conf:
sub_filter a b;
`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dump := "# configuration file /etc/nginx/nginx.conf:\n" + tc.main + "\n" + tc.files
			err := checkNginx194([]byte(dump))
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked=%v, want %v: %v", err != nil, tc.blocked, err)
			}
		})
	}
	if checkNginx194(nil) == nil {
		t.Fatal("missing effective configuration was accepted")
	}
	// The relative include is resolved against nginx.conf, not conf.d; the
	// error must point to the source directive rather than the including file.
	dump := `# configuration file /etc/nginx/nginx.conf:
http { include conf.d/entry.conf; }
# configuration file /etc/nginx/conf.d/entry.conf:
include rules.conf;
# configuration file /etc/nginx/rules.conf:
# first line
sub_filter a b;
`
	if err := checkNginx194([]byte(dump)); err == nil || !strings.Contains(err.Error(), "/etc/nginx/rules.conf:2") {
		t.Fatalf("wrong source location: %v", err)
	}
}

func TestNginx194RejectsExcessiveIncludeExpansion(t *testing.T) {
	var dump strings.Builder
	dump.WriteString("# configuration file /etc/nginx/nginx.conf:\nhttp { include 0.conf; }\n")
	for i := 0; i < 140; i++ {
		fmt.Fprintf(&dump, "# configuration file /etc/nginx/%d.conf:\ninclude %d.conf;\n", i, i+1)
	}
	dump.WriteString("# configuration file /etc/nginx/140.conf:\n# end of a complete include chain\n")
	if checkNginx194([]byte(dump.String())) == nil {
		t.Fatal("unbounded include expansion accepted")
	}
}
