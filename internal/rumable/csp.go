package rumable

import (
	"fmt"
	"net/url"
	"strings"
)

func checkCSP(s Site, p Page, scripts ...string) error {
	doc, err := inspectHTML(p.Body)
	if err != nil {
		return err
	}
	// Root-relative script/worker URLs follow the document base, not always its origin.
	for _, tag := range doc.elements {
		if tag.name != "base" {
			continue
		}
		href, exists := tag.attrs["href"]
		if !exists {
			continue
		}
		b, _ := url.Parse(s.PublicURL)
		documentURL := b
		if p.URL != "" {
			documentURL, _ = url.Parse(p.URL)
		}
		u, err := url.Parse(href)
		if err != nil {
			return fmt.Errorf("invalid HTML base URL")
		}
		u = documentURL.ResolveReference(u)
		if u.Scheme != b.Scheme || !strings.EqualFold(u.Hostname(), b.Hostname()) || urlPort(u) != urlPort(b) {
			return fmt.Errorf("cross-origin HTML base URL redirects root-relative SDK assets; use a same-origin base")
		}
		break
	}
	if len(scripts) == 0 {
		_, info, err := sdk()
		if err != nil {
			return err
		}
		sdkPath := sdkLocation(s, info)
		scripts = []string{s.assetPrefix() + "init." + hash(bootstrap(s, sdkPath, info)) + ".js", sdkPath}
	}
	type requirement struct {
		keys   []string
		target string
	}
	var checks []requirement
	for _, path := range scripts {
		checks = append(checks, requirement{[]string{"script-src-elem", "script-src", "default-src"}, scriptURL(s, path)})
	}
	paths := []string{"/v1/write/rum"}
	if s.RUM.startsReplay() {
		paths = append(paths, "/v1/write/rum/replay")
	}
	if enabled(s.RUM.RemoteConfiguration) {
		paths = append(paths, "/v1/env_variable")
	}
	for _, path := range paths {
		checks = append(checks, requirement{[]string{"connect-src", "default-src"}, strings.TrimRight(s.reportURL(), "/") + path})
	}
	policies := append([]string{}, p.Header.Values("Content-Security-Policy")...)
	for _, tag := range doc.elements {
		if tag.name != "meta" {
			continue
		}
		a := tag.attrs
		if strings.EqualFold(a["http-equiv"], "Content-Security-Policy") {
			policies = append(policies, a["content"])
		}
	}
	for _, header := range policies {
		for _, policy := range strings.Split(header, ",") {
			d := map[string][]string{}
			for _, raw := range strings.Split(policy, ";") {
				f := strings.Fields(raw)
				if len(f) > 0 {
					key := strings.ToLower(f[0])
					if _, exists := d[key]; !exists {
						d[key] = f[1:]
					}
				}
			}
			if sandbox, ok := d["sandbox"]; ok && (!contains(sandbox, "allow-scripts") || !contains(sandbox, "allow-same-origin")) {
				return fmt.Errorf("CSP sandbox does not permit scripts with a normal origin")
			}
			if types, ok := d["require-trusted-types-for"]; ok && contains(types, "'script'") {
				return fmt.Errorf("Trusted Types requires site-specific integration")
			}
			for _, worker := range s.RUM.workerURLs(s.PublicURL) {
				for _, key := range []string{"worker-src", "child-src", "script-src", "default-src"} {
					if sources, exists := d[key]; exists {
						// Explicit blob: avoids differences in browser 'self' handling for inline Workers.
						if (worker == "blob:" && !contains(sources, "blob:")) || (worker != "blob:" && !cspAllows(sources, s.PublicURL, worker)) {
							return fmt.Errorf("CSP %s blocks required SDK worker %s; update the site's policy explicitly", key, worker)
						}
						break
					}
				}
			}
			for _, check := range checks {
				for _, key := range check.keys {
					sources, exists := d[key]
					if !exists {
						continue
					}
					if !cspAllows(sources, s.PublicURL, check.target) {
						return fmt.Errorf("CSP %s blocks required %s; update the site's policy explicitly", key, check.target)
					}
					break
				}
			}
		}
	}
	return nil
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func cspAllows(sources []string, base, target string) bool {
	b, err := url.Parse(base)
	if err != nil {
		return false
	}
	t, err := url.Parse(target)
	if err != nil || t.Hostname() == "" || (t.Scheme != "http" && t.Scheme != "https") {
		return false
	}
	// With strict-dynamic, parser-inserted external scripts need a nonce/hash.
	if contains(sources, "'strict-dynamic'") {
		return false
	}
	for _, src := range sources {
		if src == "*" {
			return true
		}
		if src == "'self'" && b.Scheme == t.Scheme && strings.EqualFold(b.Hostname(), t.Hostname()) && urlPort(b) == urlPort(t) {
			return true
		}
		if src == t.Scheme+":" {
			return true
		}
		if strings.HasPrefix(src, "'") {
			continue
		}
		raw := src
		if !strings.Contains(raw, "://") {
			raw = b.Scheme + "://" + raw
		}
		authority := strings.SplitN(strings.SplitN(raw, "://", 2)[1], "/", 2)[0]
		wildcardPort := strings.HasSuffix(authority, ":*")
		if wildcardPort {
			raw = strings.Replace(raw, authority, strings.TrimSuffix(authority, ":*"), 1)
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != t.Scheme {
			continue
		}
		host := u.Hostname() == "*" || strings.EqualFold(u.Hostname(), t.Hostname())
		if strings.HasPrefix(u.Hostname(), "*.") {
			host = strings.HasSuffix(strings.ToLower(t.Hostname()), strings.ToLower(u.Hostname()[1:]))
		}
		if host && (wildcardPort || urlPort(u) == urlPort(t)) && cspPathMatches(u.EscapedPath(), t.EscapedPath()) {
			return true
		}
	}
	return false
}

func cspPathMatches(pattern, target string) bool {
	if pattern == "" || (pattern == "/" && target == "") {
		return true
	}
	a, b := strings.Split(pattern, "/"), strings.Split(target, "/")
	exact := !strings.HasSuffix(pattern, "/")
	if len(a) > len(b) || (exact && len(a) != len(b)) {
		return false
	}
	if !exact {
		a = a[:len(a)-1]
	}
	// Decode individual segments so an encoded slash cannot become a directory boundary.
	for i, segment := range a {
		x, err := url.PathUnescape(segment)
		y, otherErr := url.PathUnescape(b[i])
		if err != nil || otherErr != nil || x != y {
			return false
		}
	}
	return true
}
