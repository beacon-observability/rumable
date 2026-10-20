package rumable

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

type attachmentFile struct {
	Path   string      `json:"path"`
	Before []byte      `json:"before"`
	After  []byte      `json:"after"`
	Mode   os.FileMode `json:"mode"`
	Diff   string      `json:"diff"`
}
type attachmentRecord struct {
	SchemaVersion int               `json:"schemaVersion"`
	Site          Site              `json:"site"`
	Target        string            `json:"target"`
	Phase         string            `json:"phase"`
	At            time.Time         `json:"at"`
	Files         []attachmentFile  `json:"files"`
	Inputs        map[string]string `json:"inputs,omitempty"`
	Aliases       map[string]string `json:"aliases,omitempty"`
	Identities    map[string]string `json:"identities,omitempty"`
}

func attachmentPath(root string, s Site) string {
	return filepath.Join(siteDir(root, s), "attachment.json")
}
func writeAttachment(name string, a attachmentRecord) error {
	if err := writeJSON(name, a); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func archiveAttachment(name string, a attachmentRecord) error {
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		live, e := os.ReadFile(name)
		if e == nil && bytes.Equal(bytes.TrimSpace(live), bytes.TrimSpace(b)) {
			return nil
		}
		return fmt.Errorf("attachment archive already exists with different contents: %s", name)
	}
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		os.Remove(name)
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	dir, err := os.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func attachmentRecordUnchanged(root string, s Site, expected attachmentRecord, st State) error {
	live, err := loadAttachment(root, s)
	if expected.SchemaVersion == 0 && errors.Is(err, os.ErrNotExist) {
		err = nil
	} else if err == nil && !reflect.DeepEqual(live, expected) {
		return errors.New("attachment backup changed after preview; rerun attach/rollback")
	}
	if err != nil {
		return err
	}
	current, err := loadState(root, s.ID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, st) {
		return errors.New("deployment state changed after preview; rerun attach/rollback")
	}
	return verifyFiles(root, st.Current)
}

func verifyAttachmentSources(root string, a attachmentRecord) error {
	owned := map[string]bool{}
	for _, f := range a.Files {
		owned[f.Path] = true
	}
	for alias, canonical := range a.Aliases {
		live, err := filepath.EvalSymlinks(alias)
		if err != nil || live != canonical {
			return fmt.Errorf("configuration path changed: %s; refusing attachment overwrite", alias)
		}
	}
	for name, digest := range a.Inputs {
		if owned[name] {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		identity, err := attachmentIdentity(name)
		if err != nil {
			return err
		}
		if hash(b) != digest || (a.Identities[name] != "" && identity != a.Identities[name]) {
			// Another managed site can change its switch after this attachment.
			// Accept only its currently committed bytes, never arbitrary edits.
			if a.Site.OTel != nil && managedOTelFile(root, name) {
				rel, _ := filepath.Rel(filepath.Join(root, "sites"), name)
				id := strings.Split(filepath.ToSlash(rel), "/")[0]
				st, err := loadState(root, id)
				if err == nil && sameServer(a.Site, st.Current.Site) && sameExporter(a.Site, st.Current.Site) && verifyFiles(root, st.Current) == nil {
					continue
				}
			}
			return fmt.Errorf("external configuration change to %s; refusing attachment overwrite", name)
		}
	}
	return nil
}

var errAttachmentInspection = errors.New("native attachment inspection failed")

func verifyAttachmentOwnership(a attachmentRecord) error {
	_, dump, err := nginxInspect(a.Site)
	if err != nil {
		return fmt.Errorf("%w: %v", errAttachmentInspection, err)
	}
	c, err := nginxAttachmentConfig(dump)
	if err != nil {
		return err
	}
	for _, f := range a.Files {
		if c.uses[f.Path] != 1 {
			return fmt.Errorf("configuration %s is now included %d times; refusing to change another site", f.Path, c.uses[f.Path])
		}
	}
	return nil
}

func loadAttachment(root string, s Site) (attachmentRecord, error) {
	var a attachmentRecord
	b, err := os.ReadFile(attachmentPath(root, s))
	if err != nil {
		return a, err
	}
	if err = json.Unmarshal(b, &a); err != nil {
		return a, err
	}
	if a.SchemaVersion != 1 || !compatibleTarget(a.Site, s) || len(a.Files) == 0 {
		return a, errors.New("invalid or mismatched attachment backup")
	}
	if a.Phase != "applying" && a.Phase != "attached" && a.Phase != "restoring" && a.Phase != "detached" {
		return a, errors.New("invalid attachment phase")
	}
	seen := map[string]bool{}
	for _, f := range a.Files {
		if !filepath.IsAbs(f.Path) || filepath.Clean(f.Path) != f.Path || seen[f.Path] || bytes.Equal(f.Before, f.After) || f.Mode&^os.FileMode(0777) != 0 {
			return a, errors.New("invalid attachment file backup")
		}
		seen[f.Path] = true
	}
	return a, nil
}
func attachmentPending(root string, s Site) error {
	a, err := loadAttachment(root, s)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.Phase == "applying" || a.Phase == "restoring" {
		return errors.New("unfinished attachment; use rollback --attachment")
	}
	if a.Phase == "attached" {
		return attachmentReview(root, s, false)
	}
	return nil
}
func attachmentReview(root string, s Site, manual bool) error {
	a, err := loadAttachment(root, s)
	if errors.Is(err, os.ErrNotExist) {
		if manual {
			return nil
		}
		return fmt.Errorf("run attach, or complete ATTACH.md and set %s.attachmentReviewed=true", s.platform())
	}
	if err != nil {
		return err
	}
	if a.Phase != "attached" {
		return fmt.Errorf("attachment is %s; run attach before enabling", a.Phase)
	}
	// Ordinary deployment changes do not own the rest of these business files.
	// Their initial full-file backups are used only by explicit attachment undo.
	for _, f := range a.Files {
		live, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		prefix := "# BEGIN rumable-" + s.ID + " "
		remaining := string(f.After)
		for {
			i := strings.Index(remaining, prefix)
			if i < 0 {
				break
			}
			part := remaining[i:]
			end := strings.Index(part, "# END rumable-"+s.ID+" ")
			if end < 0 {
				return errors.New("invalid attachment marker backup")
			}
			stop := strings.IndexByte(part[end:], '\n')
			if stop < 0 {
				return errors.New("invalid attachment marker end")
			}
			stop += end + 1
			block := part[:stop]
			if bytes.Count(live, []byte(block)) != 1 {
				return fmt.Errorf("external change to attachment block in %s", f.Path)
			}
			remaining = part[stop:]
		}
	}
	return nil
}
func confirmAttachment(out io.Writer, in io.Reader, yes, dry bool) error {
	if dry {
		return nil
	}
	if yes {
		return nil
	}
	if _, err := fmt.Fprint(out, "Apply these changes? Type yes: "); err != nil {
		return err
	}
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return err
	}
	if strings.TrimSpace(answer) != "yes" {
		return errors.New("attachment cancelled; no server configuration changed")
	}
	return nil
}
func attachSite(root string, st State, target string, yes, dry bool, in io.Reader, out io.Writer) error {
	if err := checkManagedDependency(root, st.Current.Site); err != nil {
		return err
	}
	if runtime.GOOS != "linux" {
		return errors.New("automatic Nginx attachment requires Linux; use render and ATTACH.md on other OSes")
	}
	s := st.Current.Site
	if s.IIS != nil {
		return errors.New("IIS uses enable; attach supports Nginx")
	}
	if err := verifyFiles(root, st.Current); err != nil {
		return err
	}
	if _, err := os.Stat(journalPath(root, s)); err == nil {
		return errors.New("unfinished deployment; use rollback first")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	old, err := loadAttachment(root, s)
	if err == nil {
		if old.Phase == "attached" {
			if target != "" && target != old.Target {
				return errors.New("site is already attached to a different target; disable and undo attachment first")
			}
			if err = attachmentReview(root, s, false); err != nil {
				return err
			}
			if err = nativeCheck(root, s, st.Current.Enabled); err != nil {
				return err
			}
			if err = waitAttachment(st.Current); err != nil {
				return err
			}
			fmt.Fprintln(out, "Already attached:", old.Target)
			return nil
		}
		if old.Phase != "detached" {
			return errors.New("unfinished attachment; use rollback --attachment")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if st.Current.active() {
		return errors.New("disable the site before changing its attachment")
	}
	plan, err := planNginxAttachment(root, s, target)
	if err != nil {
		return err
	}
	files, selected := plan.Files, plan.Target
	a := attachmentRecord{SchemaVersion: 1, Site: s, Target: selected, Phase: "applying", At: time.Now().UTC(), Files: files, Inputs: plan.Inputs, Aliases: plan.Aliases, Identities: plan.Identities}
	var preview strings.Builder
	fmt.Fprintln(&preview, "Selected:", selected)
	if !s.RUM.isEnabled() {
		fmt.Fprintln(&preview, "OTel-only attachment: native instrumentation and reserved resources; existing response compression, cache and business headers are preserved.")
	} else {
		fmt.Fprintln(&preview, "Attached handlers bypass upstream compression, conditional headers and proxy cache; static precompression is disabled until attachment undo. Output gzip is retained.")
	}
	fmt.Fprintln(&preview, "Review ATTACH.md and clear external HTML caches when enabling/disabling.")
	for _, f := range files {
		fmt.Fprint(&preview, f.Diff)
	}
	fmt.Fprintln(&preview, "Backup:", attachmentPath(root, s), "(original bytes, mode, diff; retained after undo)")
	if _, err = io.WriteString(out, preview.String()); err != nil {
		return err
	}
	if err = confirmAttachment(out, in, yes, dry); err != nil || dry {
		return err
	}
	// Native discovery also catches new wildcard includes added during confirmation.
	current, err := planNginxAttachment(root, s, target)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, current) {
		return errors.New("configuration changed after preview; rerun attach to review the new changes")
	}
	if err = attachmentRecordUnchanged(root, s, old, st); err != nil {
		return err
	}
	// Recheck every changed file after confirmation, before writing any backup or
	// server file. Each atomic replacement checks again to catch concurrent edits.
	for name, digest := range plan.Inputs {
		b, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		if hash(b) != digest {
			return fmt.Errorf("configuration changed after preview: %s; rerun attach", name)
		}
	}
	if err = verifyAttachmentFiles(a, false, false); err != nil {
		return err
	}
	if old.SchemaVersion != 0 {
		if err = archiveAttachment(filepath.Join(siteDir(root, s), "attachment."+old.At.Format("20060102T150405.000000000")+".json"), old); err != nil {
			return err
		}
	}
	if err = writeAttachment(attachmentPath(root, s), a); err != nil {
		return err
	}
	for _, f := range files {
		if err = verifyAttachmentSources(root, a); err != nil {
			break
		}
		identity, e := attachmentIdentity(f.Path)
		if e != nil {
			err = e
			break
		}
		if a.Identities[f.Path] != "" && identity != a.Identities[f.Path] {
			err = fmt.Errorf("configuration metadata changed after preview: %s", f.Path)
			break
		}
		if err = replaceAttachmentFile(f.Path, f.Before, f.After, f.Mode); err != nil {
			break
		}
	}
	if err == nil {
		err = verifyAttachmentSources(root, a)
	}
	if err == nil {
		err = verifyAttachmentOwnership(a)
	}
	if err == nil {
		err = nativeCheck(root, s, false)
	}
	if err == nil {
		err = reloadAttachment(root, a, true)
	}
	if err == nil {
		err = waitAttachment(st.Current)
	}
	if err == nil {
		a.Phase = "attached"
		err = writeAttachment(attachmentPath(root, s), a)
	}
	if err != nil {
		failure := err
		if restoreErr := restoreAttachment(root, a, st.Current); restoreErr != nil {
			return fmt.Errorf("attachment failed: %v; recovery incomplete: %w; backup retained, use rollback --attachment", failure, restoreErr)
		}
		return fmt.Errorf("attachment failed and was restored: %w", failure)
	}
	fmt.Fprintln(out, "Attached with collection disabled; run enable. Undo after disable: rollback --attachment")
	return nil
}
func verifyAttachmentFiles(a attachmentRecord, after, either bool) error {
	for _, f := range a.Files {
		live, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		info, err := os.Stat(f.Path)
		if err != nil {
			return err
		}
		want := f.Before
		if after {
			want = f.After
		}
		if info.Mode().Perm() != f.Mode || (!bytes.Equal(live, want) && !(either && (bytes.Equal(live, f.Before) || bytes.Equal(live, f.After)))) {
			return fmt.Errorf("external change to %s; refusing attachment overwrite", f.Path)
		}
	}
	return nil
}
func reloadAttachment(root string, a attachmentRecord, after bool) error {
	s := a.Site
	binary, args := s.Nginx.Binary, s.Nginx.Args
	if _, err := command(binary, append(append([]string{}, args...), "-t")...); err != nil {
		return err
	}
	if err := verifyAttachmentSources(root, a); err != nil {
		return err
	}
	if err := verifyAttachmentFiles(a, after, false); err != nil {
		return err
	}
	action := []string{"-s", "reload"}
	_, err := command(binary, append(append([]string{}, args...), action...)...)
	return err
}
func restoreAttachment(root string, a attachmentRecord, n Snapshot) error {
	if err := verifyAttachmentSources(root, a); err != nil {
		return err
	}
	if a.Phase == "attached" || len(a.Inputs) > 0 {
		if err := verifyAttachmentOwnership(a); err != nil {
			// Initial candidates and interrupted multi-file restores can make -T
			// unusable. Unchanged inputs and exact Before/After bytes still let us
			// finish restoring those files after a failed or interrupted attempt.
			if (a.Phase != "applying" && a.Phase != "restoring") || !errors.Is(err, errAttachmentInspection) {
				return fmt.Errorf("cannot verify attachment ownership before recovery: %w", err)
			}
		}
	}
	if err := verifyAttachmentFiles(a, true, a.Phase != "attached"); err != nil {
		return err
	}
	a.Phase = "restoring"
	if err := writeAttachment(attachmentPath(root, a.Site), a); err != nil {
		return err
	}
	for _, f := range a.Files {
		if err := verifyAttachmentSources(root, a); err != nil {
			return err
		}
		live, err := os.ReadFile(f.Path)
		if err != nil {
			return err
		}
		if bytes.Equal(live, f.Before) {
			continue
		}
		if err = replaceAttachmentFile(f.Path, f.After, f.Before, f.Mode); err != nil {
			return err
		}
	}
	if err := reloadAttachment(root, a, false); err != nil {
		return err
	}
	// The reserved resource mapping is gone, so only verify the original pages.
	if err := probeOriginalPages(n.Site); err != nil {
		return fmt.Errorf("configuration restored but page verification failed: %w", err)
	}
	a.Phase = "detached"
	return writeAttachment(attachmentPath(root, a.Site), a)
}
func probeOriginalPages(s Site) error {
	for _, p := range s.ProbePaths {
		page, err := getPage(pageURL(s, p), http.Header{"Accept-Encoding": []string{"gzip"}})
		if err != nil {
			return err
		}
		if _, err = inspectInjection(s, page, &Snapshot{Enabled: false}); err != nil {
			return err
		}
	}
	return nil
}
func waitAttachment(n Snapshot) error {
	var err error
	for i := 0; i < 12; i++ {
		_, err = probePages(n.Site, n.Site.ProbePaths, &n)
		if err == nil {
			err = probeScripts(n.Site, n.Script, &n)
		}
		if err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}

func undoAttachment(root string, st State, yes, dry bool, in io.Reader, out io.Writer) error {
	if st.Current.active() {
		return errors.New("run disable before rollback --attachment; cached pages may still require retained asset mappings")
	}
	if _, err := os.Stat(journalPath(root, st.Current.Site)); err == nil {
		return errors.New("recover the deployment with rollback before undoing attachment")
	}
	a, err := loadAttachment(root, st.Current.Site)
	if err != nil {
		return err
	}
	if a.Phase == "detached" {
		fmt.Fprintln(out, "Attachment already restored; backup retained.")
		return nil
	}
	if err = checkDetachDependents(root, st); err != nil {
		return err
	}
	if err = verifyAttachmentFiles(a, true, a.Phase != "attached"); err != nil {
		return err
	}
	var preview strings.Builder
	fmt.Fprintln(&preview, "Restore original server configuration; reserved SDK mappings will be removed. Clear cached HTML first.")
	for _, f := range a.Files {
		fmt.Fprintf(&preview, "Restore %s from %s\n", f.Path, attachmentPath(root, a.Site))
		for _, line := range strings.Split(f.Diff, "\n") {
			if strings.HasPrefix(line, "+") {
				line = "-" + line[1:]
			} else if strings.HasPrefix(line, "-") {
				line = "+" + line[1:]
			}
			if line != "" {
				fmt.Fprintln(&preview, line)
			}
		}
	}
	if _, err = io.WriteString(out, preview.String()); err != nil {
		return err
	}
	if err = confirmAttachment(out, in, yes, dry); err != nil || dry {
		return err
	}
	if err = attachmentRecordUnchanged(root, a.Site, a, st); err != nil {
		return err
	}
	if err = restoreAttachment(root, a, st.Current); err != nil {
		return err
	}
	fmt.Fprintln(out, "Original attachment configuration restored; backup and versioned assets retained.")
	return nil
}
