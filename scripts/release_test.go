//go:build ignore

package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseInstallersPinArchivesWithoutBindingADomain(t *testing.T) {
	var packages []map[string]string
	for i, target := range []struct{ os, arch string }{{"linux", "386"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}} {
		packages = append(packages, map[string]string{"os": target.os, "arch": target.arch, "flavor": "modern", "sha256": strings.Repeat(string(rune('a'+i)), 64)})
	}
	files, metadata, err := releaseInstallers(".", "0.1.0", packages)
	if err != nil {
		t.Fatal(err)
	}
	for platform, m := range metadata {
		body := files[m["file"]]
		hash := sha256.Sum256(body)
		if hex.EncodeToString(hash[:]) != m["sha256"] || !bytes.Contains(body, []byte("'0.1.0'")) || bytes.Contains(body, []byte("@@VERSION@@")) {
			t.Fatal("installer metadata/version mismatch", platform)
		}
	}
	for _, p := range packages {
		file := "install.sh"
		if p["os"] == "windows" {
			file = "install.ps1"
		}
		if !bytes.Contains(files[file], []byte(p["sha256"])) {
			t.Fatal("installer lost archive integrity pin", p)
		}
	}
	if _, _, err = releaseInstallers(".", "0.1.0", packages[:3]); err == nil {
		t.Fatal("missing Windows package was accepted")
	}
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = bytes.NewReader(files["install.sh"])
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("installer syntax: %v: %s", err, b)
	}
}

func TestReleaseRevisionCannotDescribeDifferentOrDirtySource(t *testing.T) {
	if err := exec.Command("git", "--version").Run(); err != nil {
		t.Skip("Git is unavailable; release provenance needs a Git-enabled builder")
	}
	dir := t.TempDir()
	git := func(args ...string) string {
		b, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatal(string(b), err)
		}
		return strings.TrimSpace(string(b))
	}
	git("init")
	name := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(name, []byte("approved source"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", "source.txt")
	git("-c", "user.name=Release test", "-c", "user.email=release-test@localhost", "-c", "commit.gpgsign=false", "commit", "-m", "source")
	head := git("rev-parse", "HEAD")
	if err := verifySourceRevision(dir, head); err != nil {
		t.Fatal(err)
	}
	if verifySourceRevision(dir, strings.Repeat("0", 40)) == nil {
		t.Fatal("unrelated revision accepted")
	}
	if err := os.WriteFile(name, []byte("unreviewed source"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifySourceRevision(dir, head) == nil {
		t.Fatal("modified source stamped with the old revision")
	}
	git("add", "source.txt")
	if verifySourceRevision(dir, head) == nil {
		t.Fatal("staged source stamped with the old revision")
	}
	git("reset", "--hard", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifySourceRevision(dir, head) == nil {
		t.Fatal("untracked build source stamped with the old revision")
	}
}

func TestReleaseArchivesPreserveExecutableAndUserDocuments(t *testing.T) {
	files := map[string][]byte{"rumable": []byte("executable"), "USER-GUIDE.md": []byte("用户指南"), "docs/configuration.md": []byte("TOML configuration")}
	b, err := tarFiles(files, "rumable")
	if err != nil {
		t.Fatal(err)
	}
	z, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	r := tar.NewReader(z)
	seen := map[string]bool{}
	for {
		h, e := r.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(r)
		if e != nil || !bytes.Equal(body, files[h.Name]) {
			t.Fatal("tar contents changed", h.Name, e)
		}
		seen[h.Name] = true
		mode := int64(0644)
		if h.Name == "rumable" {
			mode = 0755
		}
		if h.Mode != mode {
			t.Fatal("Linux executable or document mode changed", h.Name, h.Mode)
		}
	}
	if len(seen) != len(files) {
		t.Fatal("tar omitted user documents")
	}
	b, err = zipFiles(files, "rumable")
	if err != nil {
		t.Fatal(err)
	}
	rz, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rz.File) != len(files) {
		t.Fatal("ZIP omitted files")
	}
	for _, f := range rz.File {
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(r)
		r.Close()
		if e != nil || !bytes.Equal(body, files[f.Name]) {
			t.Fatal("ZIP/CRC contents changed", f.Name, e)
		}
	}
}
