package rumable

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitialConfigurations(t *testing.T) {
	for _, server := range []string{"nginx", "iis"} {
		for _, reporting := range []string{"openway", "datakit"} {
			for _, full := range []bool{false, true} {
				name := filepath.Join(t.TempDir(), "site.toml")
				args := []string{"init", "--config", name, "--server", server, "--reporting", reporting, "--site", "starter"}
				if full {
					args = append(args, "--full")
				}
				if err := Run(args, io.Discard); err != nil {
					t.Fatal(err)
				}
				s, err := loadConfig(name, "")
				if err != nil || s.ID != "starter" || s.platform() != server || (s.RUM.DatakitOrigin != "") != (reporting == "datakit") || s.RUM.startsReplay() {
					t.Fatalf("invalid %s/%s full=%v starter: %v", server, reporting, full, err)
				}
				if full && s.RUM.SessionPersistence != "cookie" {
					t.Fatal("full template lost advanced settings")
				}
				if s.IIS != nil && s.IIS.AllowCompressionChanges {
					t.Fatal("starter pre-approved IIS compression changes")
				}
			}
		}
	}
	for _, args := range [][]string{{"--server", "other"}, {"--reporting", "other"}, {"--site", "../other"}} {
		name := filepath.Join(t.TempDir(), "invalid.toml")
		if Run(append([]string{"init", "--config", name}, args...), io.Discard) == nil {
			t.Fatal("invalid starter option accepted")
		}
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatal("invalid option left a configuration file")
		}
	}
}

func TestIISPowerShellUses64BitHostForX86CLI(t *testing.T) {
	root := t.TempDir()
	standard := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	for _, arch := range []string{"amd64", "arm64"} {
		got, err := iisPowerShell(root, arch)
		if err != nil || got != standard {
			t.Fatal("native Windows host path changed", got, err)
		}
	}
	if _, err := iisPowerShell(root, "386"); err == nil {
		t.Fatal("32-bit Windows was accepted for 64-bit IIS management")
	}
	native := filepath.Join(root, "Sysnative", "WindowsPowerShell", "v1.0", "powershell.exe")
	if err := os.MkdirAll(filepath.Dir(native), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native, []byte("64-bit host"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := iisPowerShell(root, "386")
	if err != nil || got != native {
		t.Fatal("x86 CLI can launch the redirected 32-bit PowerShell", got, err)
	}
}

func TestSingleSiteSelectionPreservesRecoveryAndRejectsAmbiguity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state with spaces")
	name := filepath.Join(t.TempDir(), "site.toml")
	if err := Run([]string{"init", "--config", name}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := Run([]string{"render", "--config", name, "--state-dir", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	// Recovery selection must work without reading the original configuration.
	missing := filepath.Join(t.TempDir(), "missing.toml")
	if err := Run([]string{"status", "--config", missing, "--state-dir", root}, io.Discard); err != nil {
		t.Fatal("single-site status required the source TOML", err)
	}
	config, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	second := strings.Replace(string(config), "schemaVersion = 1", "", 1)
	second = strings.Replace(second, `id = "example"`, `id = "other"`, 1)
	if err = os.WriteFile(name, append(config, []byte(second)...), 0600); err != nil {
		t.Fatal(err)
	}
	if Run([]string{"render", "--config", name, "--state-dir", root}, io.Discard) == nil {
		t.Fatal("multi-site TOML was selected implicitly")
	}
	if err = Run([]string{"render", "--config", name, "--site", "other", "--state-dir", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"status", "disable", "rollback", "probe"} {
		err = Run([]string{cmd, "--config", missing, "--state-dir", root}, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "multiple managed sites") {
			t.Fatalf("%s did not reject ambiguous stored targets: %v", cmd, err)
		}
	}
	for _, id := range []string{"example", "other"} {
		st, err := loadState(root, id)
		if err != nil || st.Current.Enabled || st.Previous != nil {
			t.Fatal("ambiguous operation changed deployment history", err)
		}
		if err := verifyFiles(root, st.Current); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(journalPath(root, st.Current.Site)); !os.IsNotExist(err) {
			t.Fatal("ambiguous operation created a transaction")
		}
	}
}

func TestSetupCommandsCanBePastedIntoShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command execution; Windows PowerShell needs Windows acceptance")
	}
	for _, value := range []string{"path with spaces/site.toml", "quote'and\"double", "$(printf wrong); `printf wrong` $PATH", "line\nbreak"} {
		out, err := exec.Command("sh", "-c", shellCommand("printf", "%s", value)).Output()
		if err != nil || string(out) != value {
			t.Fatal("copyable command changed its argument", err)
		}
	}
}
