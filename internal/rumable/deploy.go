package rumable

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func verifyNative(root string, n Snapshot) error {
	if err := attachmentPending(root, n.Site); err != nil {
		return err
	}
	if n.Site.IIS == nil || len(n.Native) == 0 {
		return nil
	}
	live, err := nativeCapture(root, n.Site)
	if err != nil {
		return err
	}
	if !sameJSON(live, n.Native) {
		return errors.New("external change to owned IIS rules, resources, compression or OTel pool settings; refusing overwrite")
	}
	return nil
}

func prepare(root string, s Site) (State, error) {
	s = resolveOTelPaths(root, s)
	if st, err := loadState(root, s.ID); err == nil {
		return st, fmt.Errorf("already rendered; use enable to apply config changes")
	} else if !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	n, public, err := render(root, s, false)
	if err != nil {
		return State{}, err
	}
	d := siteDir(root, s)
	for name, data := range n.Files {
		if old, e := os.ReadFile(filepath.Join(d, name)); e == nil && !bytes.Equal(old, data) {
			return State{}, fmt.Errorf("refusing to overwrite existing file %s", name)
		}
	}
	if err = deployAssets(root, s, public); err != nil {
		return State{}, err
	}
	for _, name := range fileNames(n.Files) {
		if err = atomicWrite(filepath.Join(d, name), n.Files[name], 0644); err != nil {
			return State{}, err
		}
	}
	if err = atomicWrite(filepath.Join(d, "ATTACH.md"), []byte(attachment(root, s)), 0644); err != nil {
		return State{}, err
	}
	st := State{SchemaVersion: 1, Current: n}
	return st, saveState(root, st)
}

func deployAssets(root string, s Site, files map[string][]byte) error {
	for name, data := range files {
		p := filepath.Join(siteDir(root, s), filepath.FromSlash(name))
		if old, err := os.ReadFile(p); err == nil {
			if !bytes.Equal(old, data) {
				return fmt.Errorf("immutable asset conflict at %s", p)
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := atomicWrite(p, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

func compatibleTarget(a, b Site) bool {
	if a.ID != b.ID || a.platform() != b.platform() || a.PublicURL != b.PublicURL {
		return false
	}
	if a.IIS != nil {
		return a.IIS.Site == b.IIS.Site && a.IIS.Application == b.IIS.Application
	}
	binaryA, binaryB, argsA, argsB := a.Nginx.Binary, b.Nginx.Binary, a.Nginx.Args, b.Nginx.Args
	if binaryA != binaryB || len(argsA) != len(argsB) {
		return false
	}
	for i, arg := range argsA {
		if arg != argsB[i] {
			return false
		}
	}
	return true
}

func preflight(root string, s Site) error {
	if s.OTel != nil && s.OTel.Enabled {
		if err := checkManagedDependency(root, s); err != nil {
			return err
		}
	}
	if s.RUM.isEnabled() && s.IIS == nil {
		if a, err := loadAttachment(root, s); err == nil && a.Phase == "attached" && !a.Site.RUM.isEnabled() {
			return errors.New("RUM was not prepared in the original OTel-only attachment; use a new attachment with RUM prerequisites reviewed before enabling browser injection")
		}
	}
	if err := nativeCheck(root, s, true); err != nil {
		return err
	}
	if err := otelPreflight(root, s); err != nil {
		return err
	}
	if !s.RUM.isEnabled() {
		for _, path := range s.ProbePaths {
			if _, err := getPage(pageURL(s, path), nil); err != nil {
				return err
			}
		}
		return nil
	}
	if s.SDK.Source == "cdn" {
		_, info, err := sdk()
		if err != nil {
			return err
		}
		if err = probeCDNSDK(s, info, sdkLocation(s, info)); err != nil {
			return err
		}
	}
	for _, path := range s.ProbePaths {
		p, err := getPage(pageURL(s, path), nil)
		if err != nil {
			return err
		}
		if err = checkPage(s, p); err != nil {
			return err
		}
	}
	return probeWorkers(s)
}

func apply(root string, st State, s Site, enabled bool, component string, w io.Writer) error {
	if !compatibleTarget(st.Current.Site, s) {
		return errors.New("site identity/target changed; use a new site ID and separate attachment")
	}
	if (st.Current.Site.OTel == nil) != (s.OTel == nil) {
		return errors.New("adding/removing OTel changes the first attachment; disable and undo the existing attachment, then render a new site ID with OTel configured")
	}
	if s.OTel != nil && st.Current.Site.OTel.ModulePath != s.OTel.ModulePath {
		return errors.New("modulePath is fixed by first attachment; use the original module path or detach before changing it")
	}
	if err := verifyFiles(root, st.Current); err != nil {
		return err
	}
	if err := verifyNative(root, st.Current); err != nil {
		return err
	}
	if _, err := os.Stat(journalPath(root, s)); err == nil {
		return errors.New("unfinished transaction; use rollback first")
	}
	checking := s
	if component == "rum" && s.OTel != nil {
		otel := *s.OTel
		otel.Enabled = false
		checking.OTel = &otel
	}
	if component == "otel" {
		// Validate the changed component; saved browser bytes and URLs are
		// verified after activation, without comparing them to this CLI's SDK.
		off := false
		checking.RUM.Enabled = &off
	}
	if enabled {
		if err := preflight(root, checking); err != nil {
			return err
		}
	} else if err := nativeCheck(root, s, false); err != nil {
		return err
	}
	if s.IIS != nil && len(st.Current.Native) == 0 {
		native, err := nativeCapture(root, s)
		if err != nil {
			return err
		}
		var owned struct {
			Rules         []string `json:"rules"`
			Preconditions []string `json:"preconditions"`
		}
		if err = json.Unmarshal(native, &owned); err != nil {
			return err
		}
		if len(owned.Rules) > 0 || len(owned.Preconditions) > 0 {
			return errors.New("owned IIS rule names already exist without managed state")
		}
		st.Current.Native = native
		st.OriginalIIS = native
	}
	n, public, err := renderDeployment(root, s, enabled, &st.Current, component == "otel")
	if err != nil {
		return err
	}
	if component == "rum" && s.IIS != nil {
		// Keep the deployed credentials as well as the OTel TOML settings.
		n.Native = st.Current.Native
		n.OTelTokenHash = st.Current.OTelTokenHash
	}
	if snapshotsEqual(st.Current, n) {
		fmt.Fprintln(w, "Already in requested state; verifying HTTP probes.")
		return waitProbes(st.Current)
	}
	fmt.Fprintf(w, "%s site %s (%s): SDK %s; managed files %s\n", map[bool]string{true: "Enable", false: "Disable"}[enabled], s.ID, s.platform(), n.SDK.Version, strings.Join(fileNames(n.Files), ", "))
	if s.IIS != nil && n.Enabled {
		fmt.Fprintln(w, "IIS: static compression will be disabled for this application; original local setting is saved.")
	}

	return transact(root, st, n, public, false)
}
func snapshotsEqual(a, b Snapshot) bool {
	x, y := a, b
	x.Native = nil
	y.Native = nil
	u, _ := json.Marshal(x)
	v, _ := json.Marshal(y)
	return bytes.Equal(u, v)
}

func transact(root string, st State, n Snapshot, public map[string][]byte, restore bool) error {
	t := transaction{Before: st, After: n}
	jp := journalPath(root, n.Site)
	if err := writeJSON(jp, t); err != nil {
		return err
	}
	fail := func(cause error) error {
		if err := restoreTransaction(root, t); err != nil {
			return fmt.Errorf("operation failed: %v; recovery incomplete: %w; keep transaction.json", cause, err)
		}
		if err := os.Remove(jp); err != nil {
			return fmt.Errorf("recovered, but cannot remove journal: %w", err)
		}
		return fmt.Errorf("operation failed and was rolled back: %w", cause)
	}
	if err := deployAssets(root, n.Site, public); err != nil {
		return fail(err)
	}
	for _, name := range fileNames(n.Files) {
		if err := atomicWrite(filepath.Join(siteDir(root, n.Site), name), n.Files[name], 0644); err != nil {
			return fail(err)
		}
	}
	var err error
	if restore {
		err = nativeRestore(root, &n, st.Current.Native)
	} else {
		err = nativeApply(root, &n, st.OriginalIIS, st.Current.Native)
	}
	if err != nil {
		return fail(err)
	}
	t.After = n
	t.NativeCommitted = true
	if err = writeJSON(jp, t); err != nil {
		return fail(err)
	}
	if err = nativeActivate(root, n, st.Current.Native); err != nil {
		return fail(err)
	}
	if err = waitProbes(n); err != nil {
		return fail(err)
	}
	old := st.Current
	st.Previous = &old
	st.Current = n
	st.LastCheck = &CheckRecord{time.Now().UTC(), true, "configuration committed; HTTP/component state verified; browser execution and OTLP ingestion unverified"}
	if err = saveState(root, st); err != nil {
		return fail(err)
	}
	return os.Remove(jp)
}

func restoreTransaction(root string, t transaction) error {
	if err := verifyFilesAssets(root, t.Before.Current); err != nil {
		return err
	}
	d := siteDir(root, t.Before.Current.Site)
	for name, before := range t.Before.Current.Files {
		live, err := os.ReadFile(filepath.Join(d, name))
		if err != nil {
			return err
		}
		if !bytes.Equal(live, before) && !bytes.Equal(live, t.After.Files[name]) {
			return fmt.Errorf("recovery blocked by external file change: %s", name)
		}
	}
	var nativeExpected json.RawMessage
	if t.Before.Current.Site.IIS != nil {
		live, err := nativeCapture(root, t.Before.Current.Site)
		if err != nil {
			return err
		}
		if !sameJSON(live, t.Before.Current.Native) && (!t.NativeCommitted || !sameJSON(live, t.After.Native)) {
			return errors.New("IIS state differs from journal; manual review required before recovery")
		}
		nativeExpected = live
	}
	for name, before := range t.Before.Current.Files {
		if err := atomicWrite(filepath.Join(d, name), before, 0644); err != nil {
			return err
		}
	}
	if err := nativeRestore(root, &t.Before.Current, nativeExpected); err != nil {
		return err
	}
	if t.NativeCommitted {
		// Retry the old pool generation even if an earlier recovery already
		// restored its configuration but failed while recycling the process.
		if err := nativeActivate(root, t.Before.Current, t.After.Native); err != nil {
			return err
		}
	}
	if err := waitProbes(t.Before.Current); err != nil {
		return fmt.Errorf("restored configuration but HTTP recovery verification failed: %w", err)
	}
	return saveState(root, t.Before)
}

func rollback(root string, st State, w io.Writer) error {
	b, err := os.ReadFile(journalPath(root, st.Current.Site))
	if err == nil {
		var t transaction
		if err = json.Unmarshal(b, &t); err != nil {
			return err
		}
		if t.Before.SchemaVersion != 1 {
			return errors.New("unsupported transaction state")
		}
		if err = validateSnapshot(t.Before.Current, st.Current.Site.ID); err != nil {
			return err
		}
		if err = validateSnapshot(t.After, st.Current.Site.ID); err != nil {
			return err
		}
		if !compatibleTarget(st.Current.Site, t.Before.Current.Site) || !compatibleTarget(t.Before.Current.Site, t.After.Site) {
			return errors.New("transaction target differs from managed site; refusing recovery")
		}
		if t.Before.Previous != nil {
			if err = validateSnapshot(*t.Before.Previous, st.Current.Site.ID); err != nil {
				return err
			}
			if !compatibleTarget(t.Before.Current.Site, t.Before.Previous.Site) {
				return errors.New("transaction history target mismatch")
			}
		}
		if err = restoreTransaction(root, t); err != nil {
			return err
		}
		return os.Remove(journalPath(root, st.Current.Site))
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if st.Previous == nil {
		return errors.New("no previous successful deployment")
	}
	if err = verifyFiles(root, st.Current); err != nil {
		return err
	}
	if err = verifyNative(root, st.Current); err != nil {
		return err
	}
	if err = verifyFilesAssets(root, *st.Previous); err != nil {
		return err
	}
	if st.Previous.active() && st.Previous.Site.IIS == nil {
		if err = nativeCheck(root, st.Previous.Site, true); err != nil {
			return err
		}
	}
	if st.Previous.OTelEnabled && st.Previous.Site.IIS != nil {
		if _, err = iisCommand(root, st.Previous.Site, "check", map[string]any{"checkingEnabled": true, "restoringOTel": true}); err != nil {
			return err
		}
	}
	fmt.Fprintln(w, "Restoring previous successful deployment for", st.Current.Site.ID)
	return transact(root, st, *st.Previous, nil, true)
}
func verifyFilesAssets(root string, n Snapshot) error { n.Files = nil; return verifyFiles(root, n) }

func waitProbes(n Snapshot) error {
	var err error
	stable := 0
	for attempt := 0; attempt < 12; attempt++ {
		_, err = probePages(n.Site, n.Site.ProbePaths, &n)
		if err == nil {
			err = probeOTelState(n)
		}
		if err == nil {
			stable++
			// Nginx starts new workers before signalling the old ones to quit
			// after its 100ms grace interval. One matching response is too early.
			if stable == 2 {
				return nil
			}
		} else {
			stable = 0
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err == nil {
		err = errors.New("deployment probes did not remain stable across successive checks")
	}
	return err
}

func hostInfo() map[string]string {
	return map[string]string{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH}
}
