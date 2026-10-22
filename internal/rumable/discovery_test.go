package rumable

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverySelectionAndValidation(t *testing.T) {
	c := DiscoveryConfig{SchemaVersion: 1, Server: "nginx", RUM: testSite().RUM}
	if _, _, err := resolveDiscoveredSite(c, nil, nil, io.Discard); err == nil {
		t.Fatal("empty inventory accepted")
	}
	one := discoveredSite{Site: Site{ID: "auto-test", Nginx: &Nginx{Binary: "/usr/sbin/nginx"}}, Label: "example", Target: "/etc/nginx.conf:3", URLs: []string{"https://www.example.com/"}}
	s, target, err := resolveDiscoveredSite(c, []discoveredSite{one}, nil, io.Discard)
	if err != nil || target != one.Target || s.PublicURL != one.URLs[0] || s.ProbePaths[0] != "/" {
		t.Fatal(s, target, err)
	}
	second := one
	second.Site.ID = "second"
	second.Label = "second"
	second.URLs = []string{"https://other.example/"}
	for _, in := range []io.Reader{nil, strings.NewReader(""), strings.NewReader("0\n"), strings.NewReader("3\n"), strings.NewReader("y\n")} {
		if _, _, err := resolveDiscoveredSite(c, []discoveredSite{one, second}, in, io.Discard); err == nil {
			t.Fatal("ambiguous site silently selected")
		}
	}
	s, _, err = resolveDiscoveredSite(c, []discoveredSite{one, second}, strings.NewReader("2\n"), io.Discard)
	if err != nil || s.ID != "second" {
		t.Fatal(s, err)
	}
	one.URLs = append(one.URLs, "https://alias.example/")
	if _, _, err := resolveDiscoveredSite(c, []discoveredSite{one}, nil, io.Discard); err == nil {
		t.Fatal("ambiguous binding silently selected")
	}
	s, _, err = resolveDiscoveredSite(c, []discoveredSite{one}, strings.NewReader("2\n"), io.Discard)
	if err != nil || s.PublicURL != one.URLs[1] {
		t.Fatal(s, err)
	}
	one.URLs = nil
	if _, _, err := resolveDiscoveredSite(c, []discoveredSite{one}, nil, io.Discard); err == nil {
		t.Fatal("wildcard URL guessed")
	}
	s, _, err = resolveDiscoveredSite(c, []discoveredSite{one}, strings.NewReader("https://manual.example/app/\n"), io.Discard)
	if err != nil || s.Include[0] != "/app/" {
		t.Fatal(s, err)
	}
	for _, raw := range []string{"file:///etc/", "https://example.com/path", "https://example.com/?x=1"} {
		if _, _, err := resolveDiscoveredSite(c, []discoveredSite{one}, strings.NewReader(raw+"\n"), io.Discard); err == nil {
			t.Fatal("accepted invalid URL", raw)
		}
	}
	c.RUM.DatakitOrigin = "http://intake.example.com"
	if _, _, err := resolveDiscoveredSite(c, []discoveredSite{second}, nil, io.Discard); err == nil {
		t.Fatal("HTTPS target accepted HTTP intake after discovery")
	}
}

func TestIISDiscoveryAndCompressionConsent(t *testing.T) {
	data := []byte(`{"Sites":[{"Name":"示例","Application":"/app","StaticCompression":true,"Bindings":[{"Protocol":"https","Host":"site.example","Address":"0.0.0.0","Port":443},{"Protocol":"ftp","Host":"bad.example","Port":21}]},{"Name":"Default","Application":"/","Bindings":[{"Protocol":"http","Address":"0.0.0.0","Port":80}]}]}`)
	sites, err := iisDiscoveredSites(data)
	if err != nil || len(sites) != 2 || len(sites[1].URLs) != 0 || sites[0].URLs[0] != "https://site.example/app/" {
		t.Fatal(sites, err)
	}
	c := DiscoveryConfig{SchemaVersion: 1, Server: "iis", RUM: testSite().RUM}
	for _, in := range []io.Reader{nil, strings.NewReader("n\n")} {
		if _, _, err := resolveDiscoveredSite(c, sites[:1], in, io.Discard); err == nil {
			t.Fatal("compression silently approved")
		}
	}
	s, _, err := resolveDiscoveredSite(c, sites[:1], strings.NewReader("y\n"), io.Discard)
	if err != nil || !s.IIS.AllowCompressionChanges || s.Include[0] != "/app/" {
		t.Fatal(s, err)
	}
	c.AllowCompressionChanges = true
	if _, _, err := resolveDiscoveredSite(c, sites[:1], nil, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := iisDiscoveredSites([]byte(`{"Sites":[{"Name":"x","Application":"/"},{"Name":"x","Application":"/"}]}`)); err == nil {
		t.Fatal("duplicate IIS site accepted")
	}
}

func TestNginxDiscoveryIdentityAndBindings(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "nginx.conf")
	nginx := Nginx{Binary: "/usr/sbin/nginx"}
	parse := func(text string) ([]discoveredSite, error) {
		t.Helper()
		if err := os.WriteFile(name, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		return nginxDiscoveredSites(dir, nginx, []byte("# configuration file "+name+":\n"))
	}
	conf := "events {}\nhttp {\nserver { listen 80; listen [::]:443 ssl; server_name www.example.com alias.example.com; }\nserver { listen 127.0.0.1:8080; server_name _ *.example.com; }\n}\nstream { server { listen 9000; } }"
	sites, err := parse(conf)
	if err != nil || len(sites) != 2 || len(sites[0].URLs) != 4 || len(sites[1].URLs) != 0 {
		t.Fatal(sites, err)
	}
	again, err := parse("\n# line numbers moved\n" + conf)
	if err != nil || sites[0].Site.ID != again[0].Site.ID || sites[0].Target == again[0].Target {
		t.Fatal("unstable ID", again, err)
	}
	if _, err := parse("http {server { listen 80; server_name x; } server { listen 80; server_name x; }}"); err == nil {
		t.Fatal("duplicate target identity accepted")
	}
	// A manual deployment must retain its original ID rather than get a
	// second auto deployment in the same server block.
	managed := filepath.Join(dir, "sites", "website", "server.conf")
	if err := os.MkdirAll(filepath.Dir(managed), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managed, []byte("location /__rumable__/ { alias /tmp/assets/; }"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("http { server { listen 80; server_name example.com; include "+managed+"; }}"), 0600); err != nil {
		t.Fatal(err)
	}
	sites, err = nginxDiscoveredSites(dir, nginx, []byte("# configuration file "+name+":\n# configuration file "+managed+":\n"))
	if err != nil || sites[0].Site.ID != "website" {
		t.Fatal(sites, err)
	}
}

func TestDiscoveryReusesSavedTargetAndRejectsDrift(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Default Web Site", Application: "/"}
	s.Include, s.Exclude, s.ProbePaths = []string{"/app/"}, []string{"/app/private/"}, []string{"/app/health"}
	if _, err := prepare(root, s); err != nil {
		t.Fatal(err)
	}
	candidates := []discoveredSite{{Site: Site{ID: "auto-other", IIS: &IIS{Site: s.IIS.Site, Application: "/"}}, URLs: []string{s.PublicURL, "https://alias.example/"}}}
	if err := reuseDiscoveredState(root, candidates); err != nil {
		t.Fatal(err)
	}
	if candidates[0].Site.ID != s.ID || len(candidates[0].URLs) != 1 {
		t.Fatal(candidates)
	}
	selected, _, err := resolveDiscoveredSite(DiscoveryConfig{RUM: s.RUM}, candidates, nil, io.Discard)
	if err != nil || selected.Include[0] != "/app/" || selected.Exclude[0] != "/app/private/" || selected.ProbePaths[0] != "/app/health" {
		t.Fatal("saved scope was widened", selected, err)
	}
	candidates[0].URLs = []string{"https://changed.example/"}
	if err := reuseDiscoveredState(root, candidates); err == nil {
		t.Fatal("changed binding ignored")
	}
}

func TestDiscoveryConfigWebContractAndStrictKeys(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node needed")
	}
	data, err := exec.Command("node", "--input-type=module", "-e", `import {makeConfig} from '../../web/generator.mjs'; console.log(JSON.stringify(['nginx','iis'].flatMap(server=>[{}, {sdkSource:'cdn'}, {sdkSource:'cdn',sdkURL:'https://cdn.example.com/pinned.js',sdkIntegrity:true}, {sdkLoading:'sync'}, {sdkSource:'cdn',sdkLoading:'sync'}].map(sdk=>makeConfig({...sdk,server, applicationId:'test',reporting:'datakit',endpoint:'https://intake.example.com',defaultPrivacyLevel:'mask',sessionPersistence:'cookie'})))));`).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	var configs []string
	if err := json.Unmarshal(bytes.TrimSpace(data), &configs); err != nil {
		t.Fatal(err)
	}
	for _, config := range configs {
		name := filepath.Join(t.TempDir(), "discovery.toml")
		if err := os.WriteFile(name, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		c, err := loadDiscoveryConfig(name)
		if err != nil || c.RUM.ApplicationID != "test" {
			t.Fatal(c, err)
		}
		candidate := discoveredSite{Site: testSite(), URLs: []string{"https://example.com/"}}
		if c.Server == "iis" {
			candidate.Site.Nginx = nil
			candidate.Site.IIS = &IIS{Site: "Fixture", Application: "/"}
		}
		selected, _, err := resolveDiscoveredSite(c, []discoveredSite{candidate}, nil, io.Discard)
		if err != nil || selected.SDK != c.SDK {
			t.Fatal("discovery lost SDK selection", selected.SDK, c.SDK, err)
		}
		if strings.Contains(config, `source = "cdn"`) && selected.SDK.Source != "cdn" || strings.Contains(config, `loading = "sync"`) && selected.SDK.Loading != "sync" {
			t.Fatal("SDK options did not survive TOML decoding", selected.SDK)
		}
		if err := os.WriteFile(name, []byte("unexpected = true\n"+config), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadDiscoveryConfig(name); err == nil {
			t.Fatal("unknown key accepted")
		}
	}
}
