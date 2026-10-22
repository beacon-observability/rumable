//go:build ignore

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	modern := flag.String("go", "go", "maintained Go compiler")
	legacy := flag.String("legacy-go", "", "Go 1.20.14 compiler (required)")
	versionName := flag.String("version", "0.1.6", "release version")
	revision := flag.String("revision", "", "source commit; omit to discover local HEAD")
	out := flag.String("out", "", "output directory (default dist/<version>)")
	skipSync := flag.Bool("skip-sdk-sync", false, "offline rebuild using the already verified SDK; release builds sync by default")
	flag.Parse()
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-z0-9]+(?:\.[a-z0-9]+)*)?$`).MatchString(*versionName) {
		return fmt.Errorf("invalid release version")
	}
	if *out == "" {
		*out = filepath.Join("dist", *versionName)
	}
	if *revision == "" {
		if b, e := exec.Command("git", "rev-parse", "HEAD").Output(); e == nil {
			*revision = strings.TrimSpace(string(b))
		} else {
			*revision = "uncommitted"
		}
	}
	if *revision != "uncommitted" && !regexp.MustCompile(`^[a-f0-9]{40,64}$`).MatchString(*revision) {
		return fmt.Errorf("invalid source revision")
	}
	if *legacy == "" {
		return fmt.Errorf("--legacy-go must point to an isolated Go 1.20.14 compiler")
	}
	version := func(binary string) (string, error) {
		b, err := exec.Command(binary, "version").Output()
		if err != nil {
			return "", err
		}
		f := strings.Fields(string(b))
		if len(f) < 3 {
			return "", fmt.Errorf("bad compiler version")
		}
		return f[2], nil
	}
	mv, err := version(*modern)
	if err != nil {
		return err
	}
	lv, err := version(*legacy)
	if err != nil {
		return err
	}
	if lv != "go1.20.14" {
		return fmt.Errorf("legacy compiler must be go1.20.14, got %s", lv)
	}
	if !strings.HasPrefix(mv, "go1.26.") {
		return fmt.Errorf("modern release compiler must be Go 1.26.x; got %s", mv)
	}
	if !*skipSync {
		cmd := exec.Command(*modern, "run", "scripts/sync-sdk.go")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return fmt.Errorf("SDK sync: %w", err)
		}
	}
	if *revision != "uncommitted" {
		if err = verifySourceRevision(".", *revision); err != nil {
			return err
		}
	}
	for _, compiler := range []string{*modern, *legacy} {
		for _, args := range [][]string{{"test", "./..."}, {"test", "scripts/sync-sdk.go", "scripts/sync-sdk_test.go"}, {"test", "scripts/release.go", "scripts/release_test.go"}, {"vet", "./..."}} {
			cmd := exec.Command(compiler, args...)
			cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err = cmd.Run(); err != nil {
				return err
			}
		}
	}
	if err = os.MkdirAll(filepath.Dir(*out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(*out, 0755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "rumable-release-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	shared, err := releaseSupportFiles(".")
	if err != nil {
		return err
	}
	var sums []string
	var packages []map[string]string
	for _, target := range []struct{ os, arch, compiler, label string }{
		{"linux", "386", *modern, mv}, {"linux", "amd64", *modern, mv}, {"linux", "arm64", *modern, mv},
		{"windows", "386", *modern, mv}, {"windows", "amd64", *modern, mv}, {"windows", "arm64", *modern, mv},
		{"windows", "386", *legacy, "legacy-" + lv}, {"windows", "amd64", *legacy, "legacy-" + lv},
		{"linux", "386", *legacy, "legacy-" + lv}, {"linux", "amd64", *legacy, "legacy-" + lv},
	} {
		name := "rumable"
		if target.os == "windows" {
			name += ".exe"
		}
		binary := filepath.Join(tmp, name)
		cmd := exec.Command(target.compiler, "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w -X rumable/internal/rumable.Version="+*versionName, "-o", binary, "./cmd/rumable")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.os, "GOARCH="+target.arch, "GOAMD64=v1", "GO386=sse2", "GOARM64=v8.0", "GOTOOLCHAIN=local")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err = cmd.Run(); err != nil {
			return err
		}
		info, e := buildinfo.ReadFile(binary)
		if e != nil {
			return e
		}
		settings := map[string]string{}
		for _, v := range info.Settings {
			settings[v.Key] = v.Value
		}
		if settings["GOOS"] != target.os || settings["GOARCH"] != target.arch || settings["CGO_ENABLED"] != "0" || info.GoVersion != strings.TrimPrefix(target.label, "legacy-") {
			return fmt.Errorf("binary build metadata differs from requested target %s/%s", target.os, target.arch)
		}
		if target.os == runtime.GOOS && target.arch == runtime.GOARCH {
			b, e := exec.Command(binary, "version").Output()
			if e != nil {
				return e
			}
			var got struct {
				Version string `json:"version"`
			}
			if e = json.Unmarshal(b, &got); e != nil || got.Version != *versionName {
				return fmt.Errorf("packaged executable did not report version %s", *versionName)
			}
		}
		contents := map[string][]byte{}
		for name, b := range shared {
			contents[name] = b
		}
		contents[name], err = os.ReadFile(binary)
		if err != nil {
			return err
		}
		flavor := "modern"
		if strings.HasPrefix(target.label, "legacy-") {
			flavor = "legacy"
		}
		channel := "stable"
		if strings.Contains(*versionName, "-") {
			channel = "preview"
		}
		binaryHash := sha256.Sum256(contents[name])
		contents["build.json"], _ = json.MarshalIndent(map[string]string{"version": *versionName, "revision": *revision, "channel": channel, "go": strings.TrimPrefix(target.label, "legacy-"), "flavor": flavor, "os": target.os, "arch": target.arch, "binarySHA256": hex.EncodeToString(binaryHash[:])}, "", "  ")
		contents["README.md"] = []byte(fmt.Sprintf("# Rumable %s\n\n平台：%s/%s；编译器：%s；类型：%s。\n\n先阅读 [面向最终用户的使用指南](USER-GUIDE.md)。本包含可执行文件，无需安装 Go 或 Node.js。\n\n- [版本说明](CHANGELOG.md)\n- [平台兼容范围](docs/compatibility.md)\n- [完整 TOML 参数](docs/configuration.md)\n- [启停与恢复](docs/operations.md)\n- [服务端 OTel 安装与使用](docs/otel.md)\n\n在终端执行 `%s version` 核对版本；Windows PowerShell 使用 `.\\rumable.exe version`。构建来源与二进制摘要见 build.json，SDK 元数据见 sdk.json。\n", *versionName, target.os, target.arch, info.GoVersion, flavor, name))
		filename := fmt.Sprintf("rumable_%s_%s_%s", *versionName, target.os, target.arch)
		if flavor == "legacy" {
			filename += "_legacy"
		}
		var payload []byte
		if target.os == "windows" {
			filename += ".zip"
			payload, err = zipFiles(contents, name)
		} else {
			filename += ".tar.gz"
			payload, err = tarFiles(contents, name)
		}
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(*out, filename), payload, 0644); err != nil {
			return err
		}
		h := sha256.Sum256(payload)
		packages = append(packages, map[string]string{"os": target.os, "arch": target.arch, "flavor": flavor, "file": filename, "sha256": hex.EncodeToString(h[:])})
		sums = append(sums, hex.EncodeToString(h[:])+"  "+filename)
		fmt.Println(filename)
	}
	// Only installation artifacts are exposed as standalone OSS objects.
	// User documents, examples and licenses stay inside the platform archives.
	public := map[string][]byte{"sdk.json": shared["sdk.json"]}
	// Publish the matching configuration page alongside this immutable version.
	for _, name := range []string{"index.html", "app.mjs", "generator.mjs", "i18n.mjs", "styles.css"} {
		public["setup/"+name], err = os.ReadFile(filepath.Join("web", name))
		if err != nil {
			return err
		}
	}
	installerFiles, installers, err := releaseInstallers("scripts", *versionName, packages)
	if err != nil {
		return err
	}
	for name, body := range installerFiles {
		public[name] = body
	}
	public["manifest.json"], err = json.MarshalIndent(map[string]any{
		"schemaVersion": 1, "version": *versionName, "revision": *revision,
		"artifactBase": "../" + *versionName + "/",
		"capabilities": []string{"rum-setup-v1", "rum-install-v1", "rum-discovery-v1", "rum-discovery-sdk-v1", "rum-sdk-loading-v1"}, "sdk": json.RawMessage(public["sdk.json"]), "packages": packages, "installers": installers,
	}, "", "  ")
	if err != nil {
		return err
	}
	for _, name := range keys(public) {
		if err = os.MkdirAll(filepath.Dir(filepath.Join(*out, name)), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(*out, name), public[name], 0644); err != nil {
			return err
		}
		h := sha256.Sum256(public[name])
		sums = append(sums, hex.EncodeToString(h[:])+"  "+name)
	}
	sort.Strings(sums)
	if *revision != "uncommitted" {
		if err = verifySourceRevision(".", *revision); err != nil {
			return fmt.Errorf("source changed during release; discard incomplete output %s: %w", *out, err)
		}
	}
	return os.WriteFile(filepath.Join(*out, "SHA256SUMS"), []byte(strings.Join(sums, "\n")+"\n"), 0644)
}

// Explicit archive allowlist: never distribute repository source or test fixtures.
func releaseSupportFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for _, name := range []string{
		"USER-GUIDE.md", "CHANGELOG.md",
		"docs/compatibility.md", "docs/operations.md", "docs/configuration.md",
		"docs/one-click-onboarding.md", "docs/gitlab-ci-release.md", "docs/verification.md", "docs/development.md",
		"docs/otel.md", "docs/otel-installation.md", "docs/windows-acceptance.md",
		"docs/windows-0.1.4-recovery.md", "docs/review-2026-10-22.md",
		"examples/nginx.toml", "examples/iis.toml", "examples/full.toml",
		"examples/otel-nginx.toml", "examples/otel-iis.toml",
		"third_party/BurntSushi-toml-LICENSE", "third_party/tdewolff-parse-LICENSE",
		"internal/rumable/assets/SDK-LICENSE", "internal/rumable/assets/sdk.json",
	} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		files[strings.TrimPrefix(name, "internal/rumable/assets/")] = body
	}
	return files, nil
}

// The installer is bound to the archives from this build, but not to an OSS
// domain. The page passes the directory of the manifest selected by the user.
func releaseInstallers(templateDir, version string, packages []map[string]string) (map[string][]byte, map[string]map[string]string, error) {
	values := []string{"@@VERSION@@", version}
	for _, target := range []struct{ os, arch, flavor, marker string }{
		{"linux", "386", "modern", "@@LINUX_386_SHA256@@"},
		{"linux", "amd64", "modern", "@@LINUX_AMD64_SHA256@@"},
		{"linux", "arm64", "modern", "@@LINUX_ARM64_SHA256@@"},
		{"windows", "386", "modern", "@@WINDOWS_386_SHA256@@"},
		{"windows", "amd64", "modern", "@@WINDOWS_AMD64_SHA256@@"},
		{"windows", "arm64", "modern", "@@WINDOWS_ARM64_SHA256@@"},
		{"windows", "386", "legacy", "@@WINDOWS_386_LEGACY_SHA256@@"},
		{"windows", "amd64", "legacy", "@@WINDOWS_AMD64_LEGACY_SHA256@@"},
	} {
		digest := ""
		for _, p := range packages {
			if p["os"] == target.os && p["arch"] == target.arch && p["flavor"] == target.flavor {
				digest = p["sha256"]
			}
		}
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(digest) {
			return nil, nil, fmt.Errorf("installer requires %s/%s %s archive", target.os, target.arch, target.flavor)
		}
		values = append(values, target.marker, digest)
	}
	files := map[string][]byte{}
	metadata := map[string]map[string]string{}
	for platform, name := range map[string]string{"linux": "install.sh", "windows": "install.ps1"} {
		b, err := os.ReadFile(filepath.Join(templateDir, name))
		if err != nil {
			return nil, nil, err
		}
		b = []byte(strings.NewReplacer(values...).Replace(string(b)))
		if regexp.MustCompile(`@@[A-Z0-9_]+@@`).Match(b) {
			return nil, nil, fmt.Errorf("unresolved installer placeholder in %s", name)
		}
		files[name] = b
		h := sha256.Sum256(b)
		metadata[platform] = map[string]string{"file": name, "sha256": hex.EncodeToString(h[:])}
	}
	return files, metadata, nil
}

func verifySourceRevision(dir, revision string) error {
	head, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(head)) != revision {
		return fmt.Errorf("release revision must equal the current local Git HEAD")
	}
	status, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--untracked-files=normal").Output()
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(status)) != 0 {
		return fmt.Errorf("source is not clean; review and commit SDK/source changes before release, or use --revision uncommitted for a candidate build")
	}
	return nil
}
func keys(files map[string][]byte) []string {
	a := []string{}
	for k := range files {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func zipFiles(files map[string][]byte, binary string) ([]byte, error) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range keys(files) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetModTime(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
		h.SetMode(0644)
		if name == binary {
			h.SetMode(0755)
		}
		f, e := w.CreateHeader(h)
		if e != nil {
			return nil, e
		}
		if _, e = f.Write(files[name]); e != nil {
			return nil, e
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func tarFiles(files map[string][]byte, binary string) ([]byte, error) {
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	w := tar.NewWriter(g)
	for _, name := range keys(files) {
		mode := int64(0644)
		if name == binary {
			mode = 0755
		}
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(files[name])), ModTime: time.Unix(0, 0)}); err != nil {
			return nil, err
		}
		if _, err := io.Copy(w, bytes.NewReader(files[name])); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	if err := g.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
