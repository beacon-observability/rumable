package rumable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func initialConfig(server, reporting, id string, full bool) (string, error) {
	if id == "" {
		id = "example"
	}
	if !idPattern.MatchString(id) {
		return "", errors.New("a valid --site ID is required")
	}
	servers := map[string]string{
		"nginx": "[sites.nginx]\nbinary = \"/usr/sbin/nginx\"\nattachmentReviewed = false",
		"iis":   "[sites.iis]\nsite = \"Default Web Site\"\napplication = \"/\"\nallowCompressionChanges = false",
	}
	target, ok := servers[server]
	if !ok {
		return "", errors.New("--server must be nginx or iis")
	}
	if reporting != "openway" && reporting != "datakit" {
		return "", errors.New("--reporting must be openway or datakit")
	}
	config := exampleConfig
	if !full {
		config = strings.Replace(config, sdkExample, "", 1)
		config = strings.SplitN(config, "# Session Replay", 2)[0] + "sessionReplayEnabled = false\nsessionReplaySampleRate = 100\n"
		config = strings.Replace(config, "env = \"production\"\nservice = \"browser\"\nversion = \"1.0.0\"\n", "", 1)
	}
	config = strings.Replace(config, servers["nginx"], target, 1)
	config = strings.Replace(config, `id = "example"`, fmt.Sprintf("id = %q", id), 1)
	if reporting == "datakit" {
		config = strings.Replace(config, "site = \"https://REPLACE_OPENWAY_HOST\"\nclientToken = \"REPLACE_CLIENT_TOKEN\"", "datakitOrigin = \"https://REPLACE_DATAKIT_HOST\"", 1)
	}
	return config, nil
}

func selectSite(root, config string, stored bool) (string, error) {
	if !stored {
		s, err := loadConfig(config, "")
		return s.ID, err
	}
	entries, err := os.ReadDir(filepath.Join(root, "sites"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && idPattern.MatchString(entry.Name()) {
			_, err := os.Stat(filepath.Join(root, "sites", entry.Name(), "state.json"))
			if err == nil {
				ids = append(ids, entry.Name())
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("multiple managed sites (%s); use --site ID", strings.Join(ids, ", "))
	}
	s, err := loadConfig(config, "")
	return s.ID, err
}

// Commands are pasted into a POSIX shell or Windows PowerShell, not exec.Command.
func shellCommand(binary string, args ...string) string {
	parts := append([]string{binary}, args...)
	for i, part := range parts {
		if runtime.GOOS == "windows" {
			parts[i] = "'" + strings.ReplaceAll(part, "'", "''") + "'"
		} else {
			parts[i] = "'" + strings.ReplaceAll(part, "'", "'\"'\"'") + "'"
		}
	}
	line := strings.Join(parts, " ")
	if runtime.GOOS == "windows" {
		line = "& " + line
	}
	return line
}

func setupInstructions(root string, s Site, config string) string {
	var b strings.Builder
	d := siteDir(root, s)
	if absolute, err := filepath.Abs(config); err == nil {
		config = absolute
	}
	executable, err := os.Executable()
	if err != nil {
		executable = "rumable"
	}
	if s.IIS == nil {
		fmt.Fprintf(&b, "\n1. Automatically detect and preview the first attachment, then confirm:\n   %s\n   Preview only: add --dry-run. Multiple candidates: use --target FILE:LINE.\n", shellCommand(executable, "attach", "--config", config, "--site", s.ID, "--state-dir", root))
		if s.OTel == nil {
			fmt.Fprintf(&b, "   Manual fallback: %s (then set %s.attachmentReviewed=true).\n", filepath.Join(d, "ATTACH.md"), s.platform())
		} else {
			fmt.Fprintf(&b, "   Prepare managed native dependencies first: %s\n   OTel requires managed attachment ownership; see %s and docs/otel-installation.md.\n", shellCommand(executable, "install-otel", "--config", config, "--site", s.ID, "--state-dir", root), filepath.Join(d, "ATTACH.md"))
		}
	} else {
		if s.RUM.isEnabled() {
			b.WriteString("\n1. Review ATTACH.md; prepare URL Rewrite, asset access and compression permission as Administrator.\n")
		} else {
			b.WriteString("\n1. Review ATTACH.md; run install-otel and prepare a dedicated IIS 10 application pool as Administrator.\n")
		}
	}
	fmt.Fprintf(&b, "2. Enable (includes preflight, server validation, activation and HTTP probes):\n   %s\n", shellCommand(executable, "enable", "--config", config, "--site", s.ID, "--state-dir", root))
	if s.OTel != nil && s.OTel.Backend == "dataway" && s.IIS == nil {
		fmt.Fprintln(&b, "Dataway requires an existing OTLP/gRPC Collector. Its generated private JSON configuration is under", filepath.Join(d, "collector", s.ID), "(JSON is accepted as YAML). Start it with the tokenEnv environment variable before enable; status shows the exact configuration path. Shared exporters use ONE Collector process.")
	}
	return b.String()
}
