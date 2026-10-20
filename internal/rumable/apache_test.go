package rumable

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApacheConfigurationAndTargetIdentity(t *testing.T) {
	name := filepath.Join(t.TempDir(), "apache.toml")
	config, err := os.ReadFile("../../examples/apache.toml")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(name, config, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := loadConfig(name, "apache-example")
	if err != nil || s.platform() != "apache" {
		t.Fatal("Apache TOML example cannot be loaded", err)
	}
	for _, tc := range []struct {
		name  string
		args  []string
		valid bool
	}{
		{"default", nil, true},
		{"config-and-define", []string{"-f", "/etc/httpd/httpd.conf", "-D", "MY_SITE"}, true},
		{"reload", []string{"-k", "restart"}, false},
		{"inline-config", []string{"-C", "Include /other.conf"}, false},
		{"incomplete", []string{"-f"}, false},
		{"option-as-value", []string{"-f", "-k"}, false},
		{"process-mode", []string{"-D", "FOREGROUND"}, false},
		{"dump-mode", []string{"-D", "DUMP_INCLUDES"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := s
			apache := *s.Apache
			apache.Args = tc.args
			x.Apache = &apache
			if (x.validate() == nil) != tc.valid {
				t.Fatalf("argument validation mismatch: %v", x.validate())
			}
			if tc.valid && compatibleTarget(s, x) != (len(tc.args) == 0) {
				t.Fatal("server arguments were not bound to deployment identity")
			}
		})
	}
	s.Nginx = &Nginx{Binary: "nginx"}
	if s.validate() == nil {
		t.Fatal("mixed server targets were accepted")
	}
}

func TestApacheImmutableDeploymentAndRecoveryState(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.Nginx = nil
	s.Apache = &Apache{Binary: "httpd", Reviewed: true}
	s.Include = []string{"/shop/", "/quote\"'\t中文/"}
	s.ProbePaths = []string{"/shop/"}
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	n, files, err := render(root, s, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = deployAssets(root, s, files); err != nil {
		t.Fatal(err)
	}
	previous := st.Current
	st.Previous, st.Current = &previous, n
	if err = saveState(root, st); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadState(root, s.ID)
	if err != nil || !snapshotsEqual(loaded.Current, n) || loaded.Previous == nil || loaded.Previous.Enabled {
		t.Fatal("Apache history did not round trip", err)
	}
	var private string
	for name, digest := range n.Assets {
		if strings.HasPrefix(name, "filters/") {
			private = name
			if hash(files[name]) != digest || !strings.Contains(string(files[name]), `\034`) {
				t.Fatal("private filter digest or Lua string escaping invalid")
			}
		}
	}
	if private == "" {
		t.Fatal("enabled Apache deployment has no immutable filter")
	}
	filterDigest := n.Assets[private]
	delete(n.Assets, private)
	if validateSnapshot(n, s.ID) == nil {
		t.Fatal("snapshot accepted a dangling Lua filter reference")
	}
	n.Assets[private] = filterDigest
	s.RUM.Env = "upgrade"
	newer, newerFiles, err := render(root, s, true)
	if err != nil || newer.Script == n.Script {
		t.Fatal("SDK configuration upgrade did not change bootstrap", err)
	}
	if _, ok := newerFiles[private]; ok {
		t.Fatal("upgrade would reuse a changed live Lua file")
	}
	if err = deployAssets(root, s, newerFiles); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(siteDir(root, s), private)); err != nil || hash(data) != n.Assets[private] {
		t.Fatal("upgrade replaced the previous filter", err)
	}
	n.Assets["filters/other/filter."+n.Assets[private]+".lua"] = n.Assets[private]
	if validateSnapshot(n, s.ID) == nil {
		t.Fatal("snapshot escaped private site asset boundary")
	}
}
