package rumable

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func command(binary string, args ...string) ([]byte, error) {
	stdout, stderr, err := commandOutput(binary, args...)
	return append(stdout, stderr...), err
}

func commandOutput(binary string, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		message := stderr.String()
		if len(message) > 2048 {
			message = message[len(message)-2048:]
		}
		return nil, nil, fmt.Errorf("%s: %w: %s", binary, err, strings.TrimSpace(message))
	}
	return stdout.Bytes(), stderr.Bytes(), nil
}

var nginxVersion = regexp.MustCompile(`nginx/(\d+)\.(\d+)\.(\d+)`)

func nativeCheck(root string, s Site, enabling bool) error {
	if s.Apache != nil {
		return apacheCheck(root, s, enabling)
	}
	if s.Nginx == nil {
		_, err := iisCommand(root, s, "check", map[string]any{"checkingEnabled": enabling})
		return err
	}
	if enabling {
		if err := attachmentReview(root, s, s.Nginx.Reviewed); err != nil {
			return err
		}
	}
	v, dump, err := nginxInspect(s)
	if err != nil {
		return err
	}
	for _, name := range []string{"http.conf", "server.conf", "location.conf"} {
		p := filepath.ToSlash(filepath.Join(siteDir(root, s), name))
		if !strings.Contains(string(dump), "# configuration file "+p+":") {
			return fmt.Errorf("missing effective Nginx include %s; complete ATTACH.md", p)
		}
	}
	if s.OTel != nil {
		return checkNginxOTel(root, s, v, dump)
	}
	if enabling && nginxVersion.FindString(string(v)) == "nginx/1.9.4" {
		return checkNginx194(dump)
	}
	return checkNginxOTel(root, s, v, dump)
}

func nginxInspect(s Site) ([]byte, []byte, error) {
	v, err := command(s.Nginx.Binary, "-V")
	if err != nil {
		return nil, nil, err
	}
	m := nginxVersion.FindStringSubmatch(string(v))
	if len(m) != 4 {
		return nil, nil, fmt.Errorf("unable to identify Nginx version")
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	if major < 1 || (major == 1 && (minor < 9 || (minor == 9 && patch < 4))) {
		return nil, nil, fmt.Errorf("Nginx >=1.9.4 is required for variable substitution")
	}
	if s.RUM.isEnabled() && !strings.Contains(string(v), "--with-http_sub_module") {
		return nil, nil, fmt.Errorf("Nginx lacks --with-http_sub_module")
	}
	dump, _, err := commandOutput(s.Nginx.Binary, append(append([]string{}, s.Nginx.Args...), "-T")...)
	if err != nil {
		return nil, nil, err
	}
	return v, dump, nil
}

func nativeApply(root string, n *Snapshot, original, expected json.RawMessage) error {
	if n.Site.Apache != nil {
		if err := apacheCheck(root, n.Site, n.active()); err != nil {
			return err
		}
		x := n.Site.Apache
		_, err := command(x.Binary, append(append([]string{}, x.Args...), "-k", "graceful")...)
		return err
	}
	if n.Site.Nginx != nil {
		x := n.Site.Nginx
		if n.active() {
			if err := nativeCheck(root, n.Site, true); err != nil {
				return err
			}
		}
		if _, err := command(x.Binary, append(append([]string{}, x.Args...), "-t")...); err != nil {
			return err
		}
		_, err := command(x.Binary, append(append([]string{}, x.Args...), "-s", "reload")...)
		return err
	}
	data, err := iisCommand(root, n.Site, "apply", map[string]any{"enabled": n.Enabled, "otelEnabled": n.OTelEnabled, "script": n.Script, "original": original, "expected": expected})
	if err == nil {
		n.Native = data
	}
	return err
}

func nativeCapture(root string, s Site) (json.RawMessage, error) {
	if s.IIS == nil {
		return nil, nil
	}
	return iisCommand(root, s, "inspect", nil)
}

func nativeRestore(root string, n *Snapshot, expected json.RawMessage) error {
	if n.Site.IIS == nil {
		return nativeApply(root, n, nil, nil)
	}
	data, err := iisCommand(root, n.Site, "restore", map[string]any{"snapshot": n.Native, "expected": expected})
	if err == nil {
		n.Native = data
	}
	return err
}

func iisCommand(root string, s Site, action string, extra map[string]any) (json.RawMessage, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("IIS commands require Windows; render can run on any OS")
	}
	ps, err := iisPowerShell(os.Getenv("SystemRoot"), runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	script, _ := assets.ReadFile("assets/iis.ps1")
	dir := siteDir(root, s)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	if err := atomicWrite(filepath.Join(dir, "iis.ps1"), script, 0600); err != nil {
		return nil, err
	}
	otelScript, _ := assets.ReadFile("assets/otel-iis.ps1")
	if err := atomicWrite(filepath.Join(dir, "otel-iis.ps1"), otelScript, 0600); err != nil {
		return nil, err
	}
	payload := map[string]any{"site": s, "root": dir, "action": action}
	for k, v := range extra {
		payload[k] = v
	}
	b, _ := json.Marshal(payload)
	f, err := os.CreateTemp(dir, "iis-request-*.json")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	out, err := command(ps, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(dir, "iis.ps1"), "-RequestPath", name)
	if err != nil {
		return nil, err
	}
	var result any
	if err = json.Unmarshal([]byte(strings.TrimPrefix(strings.TrimSpace(string(out)), "\ufeff")), &result); err != nil {
		return nil, fmt.Errorf("invalid IIS bridge response: %w", err)
	}
	return json.Marshal(result)
}

func iisPowerShell(systemRoot, arch string) (string, error) {
	if systemRoot == "" {
		return "", fmt.Errorf("SystemRoot is missing; run on the target Windows host")
	}
	directory := "System32"
	if arch == "386" {
		// WOW64 redirects System32 to the 32-bit PowerShell. Sysnative selects the
		// real 64-bit host needed by the existing IIS configuration bridge.
		directory = "Sysnative"
		p := filepath.Join(systemRoot, directory, "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("IIS management requires 64-bit Windows and PowerShell 3+; x86 CLI on 32-bit Windows supports init/render/status/probe only: %w", err)
		}
	}
	return filepath.Join(systemRoot, directory, "WindowsPowerShell", "v1.0", "powershell.exe"), nil
}
