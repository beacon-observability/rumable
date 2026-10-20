package rumable

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentArchitectureValidation(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"net/OpenTelemetry.AutoInstrumentation.StartupHook.dll", "net/OpenTelemetry.AutoInstrumentation.dll", "netfx/OpenTelemetry.AutoInstrumentation.dll", "win-x64/OpenTelemetry.AutoInstrumentation.Native.dll", "win-x86/OpenTelemetry.AutoInstrumentation.Native.dll"} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, 128)
		copy(b, []byte{'M', 'Z'})
		binary.LittleEndian.PutUint32(b[0x3c:], 96)
		copy(b[96:], []byte{'P', 'E', 0, 0})
		machine := uint16(0x8664)
		if strings.HasPrefix(name, "win-x86/") {
			machine = 0x14c
		}
		binary.LittleEndian.PutUint16(b[100:], machine)
		if err := os.WriteFile(p, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	s := otelSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Test", Application: "/"}
	s.OTel.AgentPath = dir
	if err := validateInstalledDependency(s); err != nil {
		t.Fatal(err)
	}
	x64 := filepath.Join(dir, "win-x64", "OpenTelemetry.AutoInstrumentation.Native.dll")
	x86 := filepath.Join(dir, "win-x86", "OpenTelemetry.AutoInstrumentation.Native.dll")
	b, err := os.ReadFile(x64)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(x86, b, 0644); err != nil {
		t.Fatal(err)
	}
	if err = validateInstalledDependency(s); err == nil {
		t.Fatal("wrong profiler architecture accepted")
	}
}

func TestDependencyArchiveBoundary(t *testing.T) {
	for _, kind := range []string{"tar", "zip"} {
		for _, name := range []string{"lib/module.so", "../outside", "/outside", `C:/outside`, `lib\outside`, "lib/CON.dll", "lib/alias.", "duplicate", "link"} {
			t.Run(kind+"/"+name, func(t *testing.T) {
				var data bytes.Buffer
				if kind == "tar" {
					w := tar.NewWriter(&data)
					if err := w.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
						t.Fatal(err)
					}
					h := &tar.Header{Name: name, Mode: 0644, Size: 2}
					if name == "link" {
						h.Typeflag, h.Size, h.Linkname = tar.TypeSymlink, 0, "../outside"
					}
					if err := w.WriteHeader(h); err != nil {
						t.Fatal(err)
					}
					if name != "link" {
						_, _ = w.Write([]byte("ok"))
					}
					if name == "duplicate" {
						_ = w.WriteHeader(h)
						_, _ = w.Write([]byte("ok"))
					}
					_ = w.Close()
				} else {
					w := zip.NewWriter(&data)
					h := &zip.FileHeader{Name: name}
					if name == "link" {
						h.SetMode(os.ModeSymlink | 0777)
					}
					f, _ := w.CreateHeader(h)
					_, _ = f.Write([]byte("ok"))
					if name == "duplicate" {
						f, _ = w.CreateHeader(&zip.FileHeader{Name: name})
						_, _ = f.Write([]byte("ok"))
					}
					_ = w.Close()
				}
				work := t.TempDir()
				archive := filepath.Join(work, "archive")
				if err := os.WriteFile(archive, data.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				err := extractDependency(archive, filepath.Join(work, "out"))
				if name != "lib/module.so" {
					if err == nil {
						t.Fatal("unsafe archive accepted")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					b, err := os.ReadFile(filepath.Join(work, "out", "lib", "module.so"))
					if err != nil || string(b) != "ok" {
						t.Fatal("archive contents lost", err)
					}
				}
			})
		}
	}
}

func TestDependencyOwnershipAndSharedReferences(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	s := otelSite()
	s.OTel.ModulePath = ""
	s = resolveOTelPaths(root, s)
	peer := s
	peer.ID = "peer"
	x := *peer.Nginx
	x.Reviewed = !x.Reviewed
	peer.Nginx = &x
	if dependencyDir(root, s) != dependencyDir(root, peer) {
		t.Fatal("review/site settings split a shared server dependency")
	}
	d := dependencyDir(root, s)
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.OTel.ModulePath, []byte("module"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyDependency(root, s); err == nil {
		t.Fatal("adopted unowned installation")
	}
	files, err := dependencyFiles(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeJSON(filepath.Join(d, "rumable-install.json"), dependencyRecord{SchemaVersion: 1, Root: root, Kind: s.platform(), Files: files}); err != nil {
		t.Fatal(err)
	}
	if err = checkDependencyRemoval(root, s); err != nil {
		t.Fatal(err)
	}
	if _, err = prepare(root, peer); err != nil {
		t.Fatal(err)
	}
	if err = checkDependencyRemoval(root, s); err == nil {
		t.Fatal("removed another site's dependency")
	}
	if err = os.RemoveAll(siteDir(root, peer)); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(d, "user-file"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = checkDependencyRemoval(root, s); err == nil {
		t.Fatal("deleted a newly added user file")
	}
	if err = os.Remove(filepath.Join(d, "user-file")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(d, "user-directory"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = checkDependencyRemoval(root, s); err == nil {
		t.Fatal("deleted a newly added directory")
	}
	if err = os.Remove(filepath.Join(d, "user-directory")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(s.OTel.ModulePath, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = checkDependencyRemoval(root, s); err == nil {
		t.Fatal("deleted modified module")
	}
}

func TestUninstallWithoutStatePreservesLiveAgent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	s := otelSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Live", Application: "/"}
	s.OTel.ModulePath, s.OTel.AgentPath, s.OTel.Runtime = "", "", "netframework"
	s = resolveOTelPaths(root, s)
	if err := os.MkdirAll(s.OTel.AgentPath, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.OTel.AgentPath, "profiler.dll")
	if err := os.WriteFile(file, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	d := dependencyDir(root, s)
	files, err := dependencyFiles(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeJSON(filepath.Join(d, "rumable-install.json"), dependencyRecord{SchemaVersion: 1, Root: root, Kind: "iis", Files: files, PendingACL: true}); err != nil {
		t.Fatal(err)
	}
	if err = checkManagedDependency(root, s); err == nil {
		t.Fatal("incomplete Agent ACL setup accepted by enable preflight")
	}
	saved := iisCommand
	t.Cleanup(func() { iisCommand = saved })
	checked := false
	iisCommand = func(_ string, _ Site, action string, extra map[string]any) (json.RawMessage, error) {
		if action != "dependency-check" || extra["dependencyPath"] != d {
			t.Fatal("unexpected cleanup check", action)
		}
		checked = true
		return nil, errors.New("live application pool still references this Agent")
	}
	if err = uninstallSite(root, s, true, true, false, nil, io.Discard); err == nil || !checked {
		t.Fatal("orphan live Agent was removed", err)
	}
	b, err := os.ReadFile(file)
	if err != nil || string(b) != "keep" {
		t.Fatal("live Agent changed", err)
	}
}

func TestDependencyInputAndDigest(t *testing.T) {
	for _, i := range []*OTelInstall{
		{Archive: "relative", SHA256: strings.Repeat("a", 64)}, {URL: "http://unsafe", SHA256: strings.Repeat("a", 64)}, {URL: "https://example.com/sdk.zip"}, {SHA256: strings.Repeat("a", 64)}, {Archive: "/offline", URL: "https://example.com", SHA256: strings.Repeat("a", 64)},
	} {
		if err := i.validate(); err == nil {
			t.Fatalf("accepted invalid source: %+v", i)
		}
	}
	work := t.TempDir()
	source, target := filepath.Join(work, "archive"), filepath.Join(work, "checked")
	if err := os.WriteFile(source, []byte("verified"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := fetchDependency(target, source, "", strings.Repeat("0", 64)); err == nil {
		t.Fatal("bad hash accepted")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("failed download left an installed file")
	}
	if err := fetchDependency(target, source, "", hash([]byte("verified"))); err != nil {
		t.Fatal(err)
	}
	index := "Package: nginx-module-otel\nVersion: 1.25.3+0.1.0-1~bookworm\nArchitecture: arm64\nDepends: nginx (= 1.25.3-1~bookworm)\nSHA256: " + strings.Repeat("a", 64)
	if _, _, err := nginxPackage(index, "1.25.3", "arm64"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := nginxPackage(index, "1.25.4", "arm64"); err == nil {
		t.Fatal("installed module for another server version")
	}
	if _, _, err := nginxPackage(index, "1.25.3", "amd64"); err == nil {
		t.Fatal("installed module for another architecture")
	}
	virtual := strings.Replace(index, "nginx (= 1.25.3-1~bookworm)", "libc6 (>= 2.34), nginx-r1.25.3", 1)
	if _, _, err := nginxPackage(virtual, "1.25.3", "arm64"); err != nil {
		t.Fatal("official Nginx ABI dependency not recognized", err)
	}
	if _, _, err := nginxPackage(strings.Replace(virtual, "nginx-r1.25.3", "nginx-r1.25.30", 1), "1.25.3", "arm64"); err == nil {
		t.Fatal("accepted a different virtual ABI version")
	}
}

type confirmationEdit struct {
	edit  func()
	input io.Reader
}

func (r *confirmationEdit) Read(p []byte) (int, error) {
	if r.edit != nil {
		r.edit()
		r.edit = nil
	}
	return r.input.Read(p)
}

func TestUninstallRechecksAfterConfirmation(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Test", Application: "/"}
	st, err := prepare(root, s)
	if err != nil {
		t.Fatal(err)
	}
	reader := &confirmationEdit{input: strings.NewReader("yes\n"), edit: func() {
		st.LastCheck = &CheckRecord{Detail: "external writer"}
		if err := saveState(root, st); err != nil {
			t.Fatal(err)
		}
	}}
	if err = uninstallSite(root, s, false, false, false, reader, io.Discard); err == nil {
		t.Fatal("concurrent state edit overwritten")
	}
	live, err := loadState(root, s.ID)
	if err != nil || live.LastCheck.Detail != "external writer" {
		t.Fatal("external state lost", err)
	}
}
