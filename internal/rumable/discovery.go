package rumable

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// DiscoveryConfig deliberately contains no guessed deployment target. The
// ordinary Site validator and setup transaction still run after selection.
type DiscoveryConfig struct {
	SchemaVersion           int       `toml:"schemaVersion"`
	Server                  string    `toml:"server"`
	AllowCompressionChanges bool      `toml:"allowCompressionChanges"`
	RUM                     RUM       `toml:"rum"`
	SDK                     SDKConfig `toml:"sdk"`
}

type discoveredSite struct {
	Site               Site
	Label, Target      string
	URLs               []string
	StaticCompression  bool
	DynamicCompression bool
}

func loadDiscoveryConfig(name string) (DiscoveryConfig, error) {
	var c DiscoveryConfig
	b, err := os.ReadFile(name)
	if err != nil {
		return c, err
	}
	meta, err := toml.Decode(string(b), &c)
	if err != nil {
		return c, err
	}
	for _, key := range meta.Undecoded() {
		if len(key) == 3 && strings.EqualFold(key[0], "rum") && strings.EqualFold(key[1], "allowedTracingUrls") && contains([]string{"match", "traceType"}, key[2]) {
			continue
		}
		return c, fmt.Errorf("unknown discovery configuration key: %s", key)
	}
	if c.SchemaVersion != 1 || (c.Server != "nginx" && c.Server != "iis") {
		return c, errors.New("discovery requires schemaVersion=1 and server=nginx|iis")
	}
	if !c.RUM.isEnabled() {
		return c, errors.New("discovery requires RUM enabled")
	}
	if err := c.SDK.validate(); err != nil {
		return c, fmt.Errorf("sdk: %w", err)
	}
	return c, nil
}

func discoveredID(parts ...string) string {
	b, _ := json.Marshal(parts)
	return "auto-" + hash(b)[:20]
}

type discoveryPrompt struct {
	in  *bufio.Reader
	out io.Writer
}

func (p discoveryPrompt) line(message string) (string, error) {
	fmt.Fprint(p.out, message)
	if p.in == nil {
		return "", errors.New("selection requires an interactive terminal; rerun with a terminal or use the page's manual site settings")
	}
	v, err := p.in.ReadString('\n')
	if err != nil {
		return "", errors.New("selection cancelled; no site was changed")
	}
	return strings.TrimSpace(v), nil
}
func (p discoveryPrompt) choose(label string, values []string) (int, error) {
	if len(values) == 0 {
		return 0, fmt.Errorf("no %s found; use manual site settings", label)
	}
	if len(values) == 1 {
		fmt.Fprintf(p.out, "Selected %s: %s\n", label, values[0])
		return 0, nil
	}
	fmt.Fprintf(p.out, "Available %s:\n", label)
	for i, v := range values {
		fmt.Fprintf(p.out, "  %d) %s\n", i+1, v)
	}
	v, err := p.line("Select a number (empty cancels): ")
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > len(values) {
		return 0, errors.New("invalid selection; no site was changed")
	}
	return n - 1, nil
}

func resolveDiscoveredSite(c DiscoveryConfig, candidates []discoveredSite, in io.Reader, out io.Writer) (Site, string, error) {
	p := discoveryPrompt{out: out}
	if in != nil {
		p.in = bufio.NewReader(in)
	}
	labels := make([]string, len(candidates))
	for i, candidate := range candidates {
		labels[i] = candidate.Label
	}
	i, err := p.choose("sites", labels)
	if err != nil {
		return Site{}, "", err
	}
	selected := candidates[i]
	s := selected.Site
	if len(selected.URLs) == 0 {
		s.PublicURL, err = p.line("Enter the site's reachable HTTP(S) URL, ending in /: ")
	} else {
		var j int
		j, err = p.choose("site URLs", selected.URLs)
		if err == nil {
			s.PublicURL = selected.URLs[j]
		}
	}
	if err != nil {
		return Site{}, "", err
	}
	u, err := endpoint(s.PublicURL)
	if err != nil {
		return Site{}, "", fmt.Errorf("site URL: %w", err)
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	if len(s.Include) == 0 {
		s.Include = []string{path}
	}
	if len(s.ProbePaths) == 0 {
		s.ProbePaths = []string{path}
	}
	s.RUM = c.RUM
	s.SDK = c.SDK
	if s.IIS != nil {
		s.IIS.AllowCompressionChanges = c.AllowCompressionChanges || s.IIS.AllowCompressionChanges
		if (selected.StaticCompression || selected.DynamicCompression) && !s.IIS.AllowCompressionChanges {
			answer, err := p.line("IIS static and dynamic compression must be disabled for this application (restored on disable/uninstall). Allow? [y/N]: ")
			if err != nil {
				return Site{}, "", err
			}
			if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
				return Site{}, "", errors.New("compression change declined; no site was changed")
			}
			s.IIS.AllowCompressionChanges = true
		}
	}
	if err := s.validate(); err != nil {
		return Site{}, "", err
	}
	fmt.Fprintf(out, "Site ID: %s; URL: %s\n", s.ID, s.PublicURL)
	return s, selected.Target, nil
}

func setupDiscovered(root, name string, yes, nonInteractive bool, out io.Writer) error {
	c, err := loadDiscoveryConfig(name)
	if err != nil {
		return err
	}
	var candidates []discoveredSite
	if c.Server == "iis" {
		candidates, err = discoverIIS()
	} else {
		candidates, err = discoverNginx(root)
	}
	if err != nil {
		return err
	}
	// Reuse the recorded URL and ID for an existing IIS target. Nginx IDs are
	// recovered from its managed server include during discovery.
	if err := reuseDiscoveredState(root, candidates); err != nil {
		return err
	}
	var input io.Reader
	if !nonInteractive {
		terminal, err := discoveryInput()
		if err == nil {
			defer terminal.Close()
			input = bufio.NewReader(terminal)
		}
	}
	s, target, err := resolveDiscoveredSite(c, candidates, input, out)
	if err != nil {
		return err
	}
	if input == nil && !yes {
		return errors.New("setup confirmation requires a terminal or --yes")
	}
	unlock, err := lock(root)
	if err != nil {
		return err
	}
	defer unlock()
	// Resolve again after the prompt and lock: configuration can change while
	// a human is choosing. Never apply an old line number to a different site.
	if c.Server == "iis" {
		candidates, err = discoverIIS()
	} else {
		candidates, err = discoverNginx(root)
	}
	if err != nil {
		return err
	}
	if err = reuseDiscoveredState(root, candidates); err != nil {
		return err
	}
	found := false
	for _, candidate := range candidates {
		if candidate.Site.ID != s.ID {
			continue
		}
		if len(candidate.URLs) != 0 && !contains(candidate.URLs, s.PublicURL) {
			return errors.New("site bindings changed during selection; rerun setup")
		}
		target, found = candidate.Target, true
	}
	if !found {
		return errors.New("selected site changed during selection; rerun setup")
	}
	if s.Nginx != nil {
		// Line numbers can move when attachment adds includes before a server.
		// The existing ownership checks verify the recorded attachment instead.
		if a, e := loadAttachment(root, s); e == nil && a.Phase == "attached" {
			target = ""
		}
	}
	return setupSite(root, s, target, yes, input, out)
}

func reuseDiscoveredState(root string, candidates []discoveredSite) error {
	files, err := filepath.Glob(filepath.Join(root, "sites", "*", "state.json"))
	if err != nil {
		return err
	}
	for i := range candidates {
		c := &candidates[i]
		var matches []Site
		for _, file := range files {
			st, err := loadState(root, filepath.Base(filepath.Dir(file)))
			if err != nil {
				return err
			}
			s := st.Current.Site
			if s.ID == c.Site.ID || (s.IIS != nil && c.Site.IIS != nil && strings.EqualFold(s.IIS.Site, c.Site.IIS.Site) && strings.EqualFold(s.IIS.Application, c.Site.IIS.Application)) {
				matches = append(matches, s)
			}
		}
		if len(matches) > 1 {
			return fmt.Errorf("multiple managed deployments for %s; use manual settings", c.Label)
		}
		if len(matches) == 1 {
			s := matches[0]
			if !discoveredURLMatches(*c, s.PublicURL) {
				return fmt.Errorf("bindings changed for managed site %s; review its saved configuration", s.ID)
			}
			c.Site, c.URLs = s, []string{s.PublicURL}
		}
	}
	return nil
}

func discoveredURLMatches(c discoveredSite, raw string) bool {
	if len(c.URLs) == 0 {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for _, v := range c.URLs {
		candidate, err := url.Parse(v)
		if err == nil && candidate.Scheme == u.Scheme && strings.EqualFold(candidate.Host, u.Host) && (c.Site.Nginx != nil || candidate.Path == u.Path) {
			return true
		}
	}
	return false
}

func discoverNginx(root string) ([]discoveredSite, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("Nginx discovery requires Linux")
	}
	binary, err := exec.LookPath("nginx")
	if err != nil {
		for _, name := range []string{"/usr/sbin/nginx", "/usr/local/nginx/sbin/nginx"} {
			if info, e := os.Stat(name); e == nil && !info.IsDir() {
				binary, err = name, nil
				break
			}
		}
	}
	if err != nil {
		return nil, errors.New("Nginx not found; use manual site settings to specify its binary and configuration")
	}
	binary, err = filepath.Abs(binary)
	if err != nil {
		return nil, err
	}
	n := Nginx{Binary: binary}
	_, dump, err := nginxInspect(Site{Nginx: &n})
	if err != nil {
		return nil, err
	}
	return nginxDiscoveredSites(root, n, dump)
}

func nginxDiscoveredSites(root string, nginx Nginx, dump []byte) ([]discoveredSite, error) {
	if canonical, err := filepath.EvalSymlinks(root); err == nil {
		root = canonical
	}
	c, err := nginxAttachmentConfig(dump)
	if err != nil {
		return nil, err
	}
	var sites []discoveredSite
	seen := map[string]bool{}
	err = visitNodes(c.root, func(n *attachNode) error {
		if n.words[0] != "server" || n.parent.words == nil || n.parent.words[0] != "http" {
			return nil
		}
		var names, listeners []string
		for _, d := range nodeDirect(n, "server_name") {
			names = append(names, d.words[1:]...)
		}
		for _, d := range nodeDirect(n, "listen") {
			listeners = append(listeners, strings.Join(d.words[1:], " "))
		}
		sort.Strings(names)
		sort.Strings(listeners)
		args, _ := json.Marshal(nginx.Args)
		id := discoveredID("nginx", nginx.Binary, string(args), n.file, strings.Join(names, " "), strings.Join(listeners, "\n"))
		for _, child := range n.children {
			if filepath.Base(child.file) != "server.conf" {
				continue
			}
			rel, e := filepath.Rel(filepath.Join(root, "sites"), child.file)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if e == nil && len(parts) == 2 && idPattern.MatchString(parts[0]) {
				id = parts[0]
			}
		}
		if seen[id] {
			return errors.New("indistinguishable Nginx server blocks; use manual site settings with --target FILE:LINE")
		}
		seen[id] = true
		urls := nginxDiscoveredURLs(n, names)
		sites = append(sites, discoveredSite{Site: Site{ID: id, Nginx: &Nginx{Binary: nginx.Binary, Args: nginx.Args}}, Label: n.key() + " (" + strings.Join(names, ", ") + ")", Target: n.key(), URLs: urls})
		return nil
	})
	return sites, err
}

func discoveryURL(scheme, host, port, path string) string {
	if host == "" || strings.ContainsAny(host, "*~$ /\\\t\r\n") || strings.HasPrefix(host, ".") || host == "_" {
		return ""
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return ""
	}
	authority := net.JoinHostPort(strings.Trim(host, "[]"), port)
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		authority = host
		if strings.Contains(host, ":") {
			authority = "[" + strings.Trim(host, "[]") + "]"
		}
	}
	u := (&url.URL{Scheme: scheme, Host: authority, Path: path}).String()
	if _, err := endpoint(u); err != nil {
		return ""
	}
	return u
}

func nginxDiscoveredURLs(n *attachNode, names []string) []string {
	listeners := nodeDirect(n, "listen")
	if len(listeners) == 0 {
		listeners = []*attachNode{{words: []string{"listen", "80"}}}
	}
	set := map[string]bool{}
	for _, listener := range listeners {
		if len(listener.words) < 2 {
			continue
		}
		value := listener.words[1]
		if strings.HasPrefix(value, "unix:") || contains(listener.words, "quic") {
			continue
		}
		port := configPort(value)
		if port == "" && (net.ParseIP(strings.Trim(value, "[]")) != nil || value == "*") {
			port = "80"
		}
		scheme := "http"
		if contains(listener.words, "ssl") {
			scheme = "https"
		}
		for _, directive := range nodeDirect(n, "ssl") {
			if len(directive.words) == 2 && directive.words[1] == "on" {
				scheme = "https"
			}
		}
		// Wildcard/default bindings do not prove a public hostname. Ask for it
		// instead of silently probing localhost or an unrelated virtual host.
		for _, name := range names {
			if u := discoveryURL(scheme, name, port, "/"); u != "" {
				set[u] = true
			}
		}
	}
	var result []string
	for u := range set {
		result = append(result, u)
	}
	sort.Strings(result)
	return result
}

type iisDiscoveryResult struct {
	Sites []struct {
		Name, Application  string
		StaticCompression  bool
		DynamicCompression bool
		Bindings           []struct {
			Protocol, Host, Address string
			Port                    int
		}
	}
}

func iisDiscoveredSites(data []byte) ([]discoveredSite, error) {
	var result iisDiscoveryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	var sites []discoveredSite
	seen := map[string]bool{}
	for _, candidate := range result.Sites {
		id := discoveredID("iis", strings.ToLower(candidate.Name), strings.ToLower(candidate.Application))
		if candidate.Name == "" || !strings.HasPrefix(candidate.Application, "/") || seen[id] {
			return nil, errors.New("invalid or duplicate IIS discovery target")
		}
		seen[id] = true
		path := candidate.Application
		if path != "/" {
			path += "/"
		}
		set := map[string]bool{}
		for _, b := range candidate.Bindings {
			if b.Protocol != "http" && b.Protocol != "https" {
				continue
			}
			host := b.Host
			if host == "" && b.Address != "0.0.0.0" && b.Address != "::" {
				host = b.Address
			}
			if u := discoveryURL(b.Protocol, host, strconv.Itoa(b.Port), path); u != "" {
				set[u] = true
			}
		}
		var urls []string
		for u := range set {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		sites = append(sites, discoveredSite{Site: Site{ID: id, IIS: &IIS{Site: candidate.Name, Application: candidate.Application}}, Label: candidate.Name + " " + candidate.Application, URLs: urls, StaticCompression: candidate.StaticCompression, DynamicCompression: candidate.DynamicCompression})
	}
	return sites, nil
}

func discoverIIS() ([]discoveredSite, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("IIS discovery requires Windows")
	}
	ps, err := iisPowerShell(os.Getenv("SystemRoot"), runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	work, err := os.MkdirTemp("", "rumable-discover-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	script, err := assets.ReadFile("assets/iis-discover.ps1")
	if err != nil {
		return nil, err
	}
	name := filepath.Join(work, "discover.ps1")
	if err = os.WriteFile(name, script, 0600); err != nil {
		return nil, err
	}
	data, _, err := commandOutput(ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", name)
	if err != nil {
		return nil, err
	}
	return iisDiscoveredSites(data)
}
