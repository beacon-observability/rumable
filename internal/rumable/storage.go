package rumable

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type CheckRecord struct {
	At     time.Time `json:"at"`
	OK     bool      `json:"ok"`
	Detail string    `json:"detail"`
}
type State struct {
	SchemaVersion int             `json:"schemaVersion"`
	Current       Snapshot        `json:"current"`
	Previous      *Snapshot       `json:"previous,omitempty"`
	OriginalIIS   json.RawMessage `json:"originalIIS,omitempty"`
	LastCheck     *CheckRecord    `json:"lastCheck,omitempty"`
}
type transaction struct {
	Before          State    `json:"before"`
	After           Snapshot `json:"after"`
	NativeCommitted bool     `json:"nativeCommitted"`
}

func atomicWrite(path string, b []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".rumable-tmp-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if mode.Perm()&0077 == 0 {
		if err = protectPrivateFile(name); err != nil {
			f.Close()
			return err
		}
	}
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	// Windows Rename replaces a regular file, but not an open configuration file.
	if err = os.Rename(name, path); err != nil {
		return err
	}
	return nil
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(b, '\n'), 0600)
}

func lock(root string) (func(), error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(root, "operation.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("operation lock: %w (after a crash, verify no process is running before removing operation.lock)", err)
	}
	fmt.Fprintf(f, "pid=%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(filepath.Join(root, "operation.lock")) }, nil
}

func loadState(root string, id string) (State, error) {
	var s State
	b, err := os.ReadFile(filepath.Join(root, "sites", id, "state.json"))
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.SchemaVersion != 1 || s.Current.Site.ID != id {
		return s, errors.New("unsupported or mismatched state")
	}
	if err = validateSnapshot(s.Current, id); err != nil {
		return s, err
	}
	if s.Previous != nil {
		if err = validateSnapshot(*s.Previous, id); err != nil {
			return s, err
		}
		if !compatibleTarget(s.Current.Site, s.Previous.Site) {
			return s, errors.New("snapshot history target mismatch")
		}
	}
	return s, nil
}

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var initPattern = regexp.MustCompile(`^init\.[a-f0-9]{64}\.js$`)

func validateSnapshot(n Snapshot, id string) error {
	if n.Uninstall && (n.active() || n.Site.IIS == nil) {
		return errors.New("invalid uninstall snapshot")
	}
	if n.Site.ID != id {
		return errors.New("snapshot site mismatch")
	}
	// Historical assets are restored verbatim. New SDK option restrictions must
	// not prevent disabling/recovering a deployment made by an older release.
	if err := n.Site.validateTarget(); err != nil {
		return fmt.Errorf("invalid snapshot: %w", err)
	}
	if !digestPattern.MatchString(n.SDK.SHA256) {
		return errors.New("invalid SDK digest in snapshot")
	}
	if err := n.Site.SDK.validate(); err != nil {
		return fmt.Errorf("invalid snapshot: %w", err)
	}
	if n.Site.SDK.Source == "cdn" {
		if n.SDKURL == "" || n.SDKURL != sdkLocation(n.Site, n.SDK) {
			return errors.New("invalid CDN SDK reference in snapshot")
		}
		if err := (SDKConfig{Source: "cdn", URL: n.SDKURL}).validate(); err != nil {
			return err
		}
	} else if n.SDKURL != "" {
		return errors.New("unexpected CDN SDK reference in bundled snapshot")
	}
	if !strings.HasPrefix(n.Script, n.Site.assetPrefix()) || !initPattern.MatchString(strings.TrimPrefix(n.Script, n.Site.assetPrefix())) {
		return errors.New("invalid bootstrap URL in snapshot")
	}
	if n.Site.Nginx != nil {
		if len(n.Files) != 3 {
			return errors.New("invalid Nginx snapshot files")
		}
		for _, name := range []string{"http.conf", "server.conf", "location.conf"} {
			if _, ok := n.Files[name]; !ok {
				return errors.New("incomplete Nginx snapshot")
			}
		}
	} else if len(n.Files) != 0 {
		return errors.New("unexpected IIS snapshot files")
	}
	for name, digest := range n.Assets {
		allowed := strings.HasPrefix(name, "public/"+id+"/")
		if n.Site.OTel != nil && name == "collector/"+id+"/otel."+digest+".json" && n.OTelCollector == name {
			allowed = true
		}

		if path.Clean(name) != name || !allowed || strings.ContainsAny(name, "\\\x00") || !digestPattern.MatchString(digest) {
			return errors.New("invalid asset path or digest in snapshot")
		}
	}

	if len(n.Assets) < 2 {
		return errors.New("incomplete assets in snapshot")
	}
	if n.OTelEnabled && n.Site.OTel == nil {
		return errors.New("OTel snapshot is missing its configuration")
	}
	if n.OTelOwner != "" && !idPattern.MatchString(n.OTelOwner) {
		return errors.New("invalid OTel exporter owner")
	}
	if n.OTelTokenHash != "" && (!digestPattern.MatchString(n.OTelTokenHash) || n.Site.IIS == nil || n.Site.OTel == nil || n.Site.OTel.Backend != "dataway") {
		return errors.New("invalid IIS OTel credential fingerprint")
	}
	if n.OTelCollector != "" {
		digest := n.Assets[n.OTelCollector]
		if !digestPattern.MatchString(digest) || n.OTelCollector != "collector/"+id+"/otel."+digest+".json" {
			return errors.New("missing OTel Collector asset digest")
		}
	}
	sdkName := "public/" + id + "/sdk." + n.SDK.SHA256 + ".js"
	initName := strings.TrimPrefix(n.Script, n.Site.assetPrefix())
	if n.Assets[sdkName] != n.SDK.SHA256 || n.Assets["public/"+id+"/"+initName] != strings.TrimSuffix(strings.TrimPrefix(initName, "init."), ".js") {
		return errors.New("snapshot assets do not match SDK and bootstrap references")
	}
	return nil
}
func saveState(root string, s State) error {
	return writeJSON(filepath.Join(siteDir(root, s.Current.Site), "state.json"), s)
}
func journalPath(root string, s Site) string {
	return filepath.Join(siteDir(root, s), "transaction.json")
}

func verifyFiles(root string, n Snapshot) error {
	d := siteDir(root, n.Site)
	for name, want := range n.Files {
		got, err := os.ReadFile(filepath.Join(d, name))
		if err != nil || !bytes.Equal(got, want) {
			return fmt.Errorf("external change to managed file %s; refusing overwrite", filepath.Join(d, name))
		}
	}
	for name, digest := range n.Assets {
		got, err := os.ReadFile(filepath.Join(d, filepath.FromSlash(name)))
		if err != nil || hash(got) != digest {
			return fmt.Errorf("asset drift at %s", name)
		}
	}
	return nil
}
func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return len(a) == 0 && len(b) == 0
	}
	u, _ := json.Marshal(x)
	v, _ := json.Marshal(y)
	return bytes.Equal(u, v)
}
