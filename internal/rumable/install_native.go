package rumable

import (
	"debug/pe"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const dotnetURL = "https://github.com/open-telemetry/opentelemetry-dotnet-instrumentation/releases/download/v1.17.0/opentelemetry-dotnet-instrumentation-windows.zip"
const dotnetDigest = "764ae7994c77a7fdacc7c4752cd91211a44cab0a1e530b6b7761fc070d137463"

func installDotnet(work, stage string) ([]string, error) {
	archive := filepath.Join(work, "dotnet.zip")
	if err := fetchDependency(archive, "", dotnetURL, dotnetDigest); err != nil {
		return nil, err
	}
	return []string{dotnetURL + " sha256=" + dotnetDigest}, extractDependency(archive, filepath.Join(stage, "agent"))
}

func nginxInstallVersion(s Site) (string, error) {
	v, err := command(s.Nginx.Binary, "-v")
	if err != nil {
		return "", err
	}
	m := nginxVersion.FindStringSubmatch(string(v))
	if len(m) != 4 || !nginxOTelVersion(m[1], m[2], m[3]) {
		return "", errors.New("Nginx OTel requires >=1.25.3; older servers remain RUM-only")
	}
	return strings.Join(m[1:], "."), nil
}

func installNginx(s Site, work, stage string) ([]string, error) {
	nginx, err := nginxInstallVersion(s)
	if err != nil {
		return nil, err
	}
	info, err := command("apt-cache", "show", "nginx-module-otel")
	if err != nil {
		return nil, errors.New("no compatible APT module available; configure the official nginx.org APT repository or supply otel.install.archive/url + sha256; no repository/server changes are made automatically")
	}
	version, digest, err := nginxPackage(string(info), nginx, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	if _, err = installCommand(work, "apt-get", "download", "nginx-module-otel="+version); err != nil {
		return nil, err
	}
	names, _ := filepath.Glob(filepath.Join(work, "*.deb"))
	if len(names) != 1 {
		return nil, errors.New("expected exactly one downloaded module package")
	}
	checked := filepath.Join(work, "checked.deb")
	if err = fetchDependency(checked, names[0], "", digest); err != nil {
		return nil, err
	}
	return []string{"APT nginx-module-otel=" + version + " sha256=" + digest}, stageDependencyArchive(s, checked, stage, work)
}

func nginxPackage(index, nginx, arch string) (string, string, error) {
	if arch != "amd64" && arch != "arm64" {
		return "", "", errors.New("Nginx OTel automatic installation supports Linux x64/ARM64")
	}
	for _, stanza := range strings.Split(strings.ReplaceAll(index, "\r\n", "\n"), "\n\n") {
		fields := map[string]string{}
		for _, line := range strings.Split(stanza, "\n") {
			if k, v, ok := strings.Cut(line, ": "); ok {
				fields[k] = v
			}
		}
		compatible := false
		for _, dependency := range strings.Split(fields["Depends"], ",") {
			dependency = strings.TrimSpace(dependency)
			if dependency == "nginx-r"+nginx || strings.HasPrefix(dependency, "nginx (= "+nginx+"-") {
				compatible = true
			}
		}
		if fields["Package"] == "nginx-module-otel" && fields["Architecture"] == arch && compatible && digestPattern.MatchString(fields["SHA256"]) {
			return fields["Version"], fields["SHA256"], nil
		}
	}
	return "", "", fmt.Errorf("configured APT indexes have no nginx-module-otel for exact Nginx %s / %s; update indexes or supply a verified compatible archive; server upgrades are never automatic", nginx, arch)
}

func stageDependencyArchive(s Site, archive, stage, work string) error {
	unpack := filepath.Join(work, "unpack")
	if err := extractDependency(archive, unpack); err != nil {
		return err
	}
	if s.IIS != nil {
		return os.Rename(unpack, filepath.Join(stage, "agent"))
	}

	module, err := findDependencyFile(unpack, "ngx_otel_module.so")
	if err != nil {
		return err
	}
	b, err := os.ReadFile(module)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(stage, "ngx_otel_module.so"), b, 0644); err != nil {
		return err
	}
	// Retain the official package's copyright/license when present.
	for _, name := range []string{"copyright", "LICENSE"} {
		if p, e := findDependencyFile(unpack, name); e == nil {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(stage, name), b, 0644); e != nil {
				return e
			}
		}
	}
	return nil
}

func findDependencyFile(root, name string) (string, error) {
	var found []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == name {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) != 1 {
		return "", fmt.Errorf("distribution must contain exactly one %s (found %d)", name, len(found))
	}
	return found[0], nil
}

func validateInstalledDependency(s Site) error {
	if s.IIS != nil {
		for _, name := range []string{"win-x64/OpenTelemetry.AutoInstrumentation.Native.dll", "win-x86/OpenTelemetry.AutoInstrumentation.Native.dll", "net/OpenTelemetry.AutoInstrumentation.StartupHook.dll", "net/OpenTelemetry.AutoInstrumentation.dll", "netfx/OpenTelemetry.AutoInstrumentation.dll"} {
			p := filepath.Join(s.OTel.AgentPath, filepath.FromSlash(name))
			if st, err := os.Stat(p); err != nil || !st.Mode().IsRegular() {
				return fmt.Errorf("Agent distribution is incomplete: %s", name)
			}
			if strings.Contains(name, "Native.dll") {
				f, err := pe.Open(p)
				if err != nil {
					return fmt.Errorf("invalid Agent native DLL: %w", err)
				}
				machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
				if strings.HasPrefix(name, "win-x86/") {
					machine = pe.IMAGE_FILE_MACHINE_I386
				}
				valid := f.Machine == machine
				f.Close()
				if !valid {
					return fmt.Errorf("wrong architecture for Agent %s", name)
				}
			}
		}
		return nil
	}
	if err := checkOTelModule(s); err != nil {
		return err
	}

	_, err := command(s.Nginx.Binary, append(append([]string{}, s.Nginx.Args...), "-t", "-g", "load_module "+nginxQuote(s.OTel.ModulePath)+";")...)
	return err
}
