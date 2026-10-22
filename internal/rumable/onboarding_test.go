package rumable

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebSDKConfigurationContract(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node is needed to exercise the actual web generator")
	}
	data, err := exec.Command("node", "../../web/generator_test.mjs", "--fixtures").CombinedOutput()
	if err != nil {
		t.Fatalf("web generator: %v: %s", err, data)
	}
	var configs []string
	if err = json.Unmarshal(data, &configs); err != nil {
		t.Fatal(err)
	}
	for i, config := range configs {
		name := filepath.Join(t.TempDir(), "site.toml")
		if err = os.WriteFile(name, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		s, err := loadConfig(name, "website")
		if err != nil {
			t.Fatalf("fixture %d: %v", i, err)
		}
		if s.RUM.Service != "网站 \"quoted\" \\ test" {
			t.Fatal("TOML escaping changed SDK service", s.RUM.Service)
		}
		actual := s.RUM.sdkConfig()
		if actual["sessionSampleRate"] != 35.5 || actual["applicationId"] != "app-test" {
			t.Fatal("Web SDK config mismatch", actual)
		}
		if strings.Contains(config, "[sites.sdk]") && s.SDK.Source == "cdn" {
			_, info, err := sdk()
			if err != nil {
				t.Fatal(err)
			}
			boot := string(bootstrap(s, sdkLocation(s, info), info))
			integrityMarker := "script.integrity="
			if s.SDK.Loading == "sync" {
				boot = injectionTags(Snapshot{Site: s, SDK: info, SDKURL: sdkLocation(s, info)})
				integrityMarker = " integrity="
			}
			if !strings.Contains(boot, s.SDK.URL) || strings.Contains(boot, integrityMarker) != s.SDK.Integrity {
				t.Fatal("page CDN selection did not reach bootstrap", s.SDK)
			}
		}
	}
}

func TestSetupRejectsTargetChangesAndPendingRecovery(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	other := s
	other.PublicURL = "https://another.example/"
	if err = setupSite(root, other, "", true, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "target differs") {
		t.Fatal("setup changed target", err)
	}
	if err = writeJSON(journalPath(root, s), transaction{Before: st, After: st.Current}); err != nil {
		t.Fatal(err)
	}
	if err = setupSite(root, s, "", true, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "unfinished transaction") {
		t.Fatal("setup ignored pending recovery", err)
	}
	if err = setupSite(t.TempDir(), otelSite(), "", true, strings.NewReader(""), io.Discard); err == nil {
		t.Fatal("setup silently took over OTel")
	}
}
