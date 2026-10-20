//go:build ignore

// Run from the repository root before release: go run scripts/sync-sdk.go.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const sdkSource = "https://static.guance.com/browser-sdk/v3/dataflux-rum.js"
const stablePackage = "https://registry.npmjs.org/@cloudcare/browser-rum/latest"

var sdkClient = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}

func main() {
	if err := syncSDK(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func download(raw, kind string) ([]byte, error) {
	r, err := http.NewRequest("GET", raw, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Cache-Control", "no-cache")
	resp, err := sdkClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	t, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != 200 || t != kind {
		return nil, fmt.Errorf("%s: expected HTTP 200 %s; got %d %s", raw, kind, resp.StatusCode, t)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b) > 8*1024*1024 {
		return nil, fmt.Errorf("%s: empty or oversized response", raw)
	}
	return b, nil
}

func sdkMetadata(b, pkg []byte) ([]byte, error) {
	var p struct{ Version, License string }
	if err := json.Unmarshal(pkg, &p); err != nil {
		return nil, err
	}
	// A changed major, license or bundle layout requires an explicit compatibility review.
	if !regexp.MustCompile(`^3\.[0-9]+\.[0-9]+$`).MatchString(p.Version) || p.License != "MIT" {
		return nil, fmt.Errorf("unsupported stable SDK version/license: %s / %s", p.Version, p.License)
	}
	m := regexp.MustCompile(`sdkVersion:"(3\.[0-9]+\.[0-9]+)",sdkName:"df_web_rum_sdk"`).FindSubmatch(b)
	if len(m) != 2 || string(m[1]) != p.Version || !bytes.Contains(b, []byte("startSessionReplayRecording")) || !bytes.Contains(b, []byte("DATAFLUX_RUM")) {
		return nil, fmt.Errorf("official v3 bundle does not match latest stable package %s; retry after CDN publication or review compatibility", p.Version)
	}
	h := sha256.Sum256(b)
	return json.MarshalIndent(struct {
		Version string `json:"version"`
		Source  string `json:"source"`
		SHA256  string `json:"sha256"`
	}{p.Version, sdkSource, hex.EncodeToString(h[:])}, "", "  ")
}

func syncSDK() error {
	pkg, err := download(stablePackage, "application/json")
	if err != nil {
		return err
	}
	b, err := download(sdkSource, "application/javascript")
	if err != nil {
		return err
	}
	metadata, err := sdkMetadata(b, pkg)
	if err != nil {
		return err
	}
	metadata = append(metadata, '\n')
	dir := "internal/rumable/assets"
	js, manifest := filepath.Join(dir, "dataflux-rum.js"), filepath.Join(dir, "sdk.json")
	original, err := os.ReadFile(js)
	if err != nil {
		return err
	}
	oldMetadata, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	if bytes.Equal(original, b) && bytes.Equal(bytes.TrimSpace(oldMetadata), bytes.TrimSpace(metadata)) {
		fmt.Println("SDK already matches latest stable official v3 bundle:", string(metadata))
		return nil
	}
	if err = replaceSDK(js, b); err != nil {
		return err
	}
	if err = replaceSDK(manifest, metadata); err != nil {
		if restoreErr := replaceSDK(js, original); restoreErr != nil {
			return fmt.Errorf("manifest update failed: %v; restoring SDK failed: %w", err, restoreErr)
		}
		return err
	}
	fmt.Println("Synchronized official stable SDK:", string(metadata))
	return nil
}

func replaceSDK(name string, b []byte) error {
	i, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if !i.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular SDK file %s", name)
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".sdk-sync-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Chmod(i.Mode().Perm())
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), name)
}
