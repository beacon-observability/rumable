package rumable

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

func checkUninstallAttachment(root string, s Site, a attachmentRecord) error {
	if a.Phase == "attached" {
		return verifyAttachmentFiles(a, true, false)
	}
	// A detached peer's old Before bytes may contain an owner attachment that
	// has since been removed. Inspect current includes rather than requiring
	// stale backups to equal an otherwise unrelated live server file.
	_, dump, err := nginxInspect(s)
	if err != nil {
		return err
	}
	c, err := nginxAttachmentConfig(dump)
	if err != nil {
		return err
	}
	d := filepath.ToSlash(siteDir(root, s))
	for name, data := range c.files {
		if strings.HasPrefix(filepath.ToSlash(name), d+"/") || bytes.Contains(data, []byte(d+"/")) || bytes.Contains(data, []byte("# BEGIN rumable-"+s.ID+" ")) {
			return errors.New("detached site is referenced by live server configuration; refusing to archive/delete it")
		}
	}
	return nil
}

func checkLiveDependencyReferences(root string, s Site) error {
	d := dependencyDir(root, s)
	if s.IIS != nil {
		_, err := iisCommand(root, s, "dependency-check", map[string]any{"dependencyPath": d})
		return err
	}
	_, dump, err := nginxInspect(s)
	if err != nil {
		return err
	}
	c, err := nginxAttachmentConfig(dump)
	if err != nil {
		return err
	}
	inside := func(p string) bool {
		p = filepath.Clean(p)
		return p == d || strings.HasPrefix(p, d+string(filepath.Separator))
	}
	for name := range c.files {
		if inside(name) {
			return errors.New("OTel dependency is still included by live server configuration; refusing removal")
		}
	}
	return visitNodes(c.root, func(n *attachNode) error {
		for _, word := range n.words {
			if inside(word) {
				return fmt.Errorf("OTel dependency is still referenced at %s; refusing removal", n.key())
			}
		}
		return nil
	})
}

// Check before disabling: removing the exporter owner must not interrupt a
// different site's module/configuration, even when that site is disabled.
func checkDetachDependents(root string, st State) error {
	if st.Current.OTelOwner != st.Current.Site.ID || st.Current.Site.OTel == nil {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "sites"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == st.Current.Site.ID || !idPattern.MatchString(entry.Name()) {
			continue
		}
		x, err := loadState(root, entry.Name())
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if x.Current.OTelOwner == st.Current.Site.ID && sameServer(st.Current.Site, x.Current.Site) {
			a, err := loadAttachment(root, x.Current.Site)
			if err != nil || a.Phase != "detached" {
				return fmt.Errorf("site %s still depends on this OTel module/exporter attachment; uninstall/detach it first", entry.Name())
			}
		}
	}
	return nil
}

func checkDependencyRemoval(root string, s Site) error {
	d := dependencyDir(root, s)
	if s.OTel == nil || (!strings.HasPrefix(s.OTel.ModulePath, d+string(filepath.Separator)) && !strings.HasPrefix(s.OTel.AgentPath, d+string(filepath.Separator))) {
		return errors.New("dependency was not installed for this site; externally managed paths are never removed")
	}
	if _, err := verifyDependency(root, s); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(root, "sites"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == s.ID || !idPattern.MatchString(entry.Name()) {
			continue
		}
		x, err := loadState(root, entry.Name())
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		snapshots := []Snapshot{x.Current}
		if x.Previous != nil {
			snapshots = append(snapshots, *x.Previous)
		}
		for _, n := range snapshots {
			if n.Site.OTel == nil {
				continue
			}
			for _, p := range []string{n.Site.OTel.ModulePath, n.Site.OTel.AgentPath} {
				if p == d || strings.HasPrefix(p, d+string(filepath.Separator)) {
					return fmt.Errorf("OTel dependency is referenced by site %s or its rollback snapshot; uninstall it first", entry.Name())
				}
			}
		}
	}
	return nil
}

func uninstallSite(root string, s Site, remove, yes, dry bool, in io.Reader, out io.Writer) error {
	st, err := loadState(root, s.ID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	hasState := err == nil
	var attachment attachmentRecord
	if hasState {
		s = st.Current.Site
		if _, err = os.Stat(journalPath(root, s)); !os.IsNotExist(err) {
			return errors.New("unfinished deployment transaction; recover with rollback before uninstall")
		}
		if err = verifyFiles(root, st.Current); err != nil {
			return err
		}
		if err = verifyNative(root, st.Current); err != nil {
			return err
		}
		if err = checkDetachDependents(root, st); err != nil {
			return err
		}
		if s.IIS == nil {
			attachment, err = loadAttachment(root, s)
			if os.IsNotExist(err) {
				attachment.Phase = "detached"
			} else if err != nil {
				return err
			}
			if attachment.Phase != "attached" && attachment.Phase != "detached" {
				return errors.New("unfinished attachment; recover using rollback --attachment before uninstall")
			}
			if err = checkUninstallAttachment(root, s, attachment); err != nil {
				return err
			}
		}
	} else {
		s = resolveOTelPaths(root, s)
	}
	dependencyExists := false
	if s.OTel != nil {
		_, e := os.Lstat(dependencyDir(root, s))
		dependencyExists = e == nil
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if remove && dependencyExists {
			if err = checkDependencyRemoval(root, s); err != nil {
				return err
			}
		}
	}
	if !hasState && (!remove || !dependencyExists) {
		fmt.Fprintln(out, "Site already uninstalled (or never rendered); no changes.")
		return nil
	}
	fmt.Fprintf(out, "Uninstall site %s: stop RUM and OTel, restore owned server attachment/settings, archive deployment backups and assets.\n", s.ID)
	if hasState && s.IIS != nil {
		fmt.Fprintf(out, "IIS target: site %q, application %q. Restore the original owned rules, compression and pool environment; only this dedicated pool may recycle.\n", s.IIS.Site, s.IIS.Application)
	}
	fmt.Fprintln(out, "Resource mappings created by this deployment will be removed. Clear cached HTML in browsers/proxies/CDNs first; cached pages can otherwise request removed scripts.")
	if hasState && s.IIS == nil && attachment.Phase == "attached" {
		for _, f := range attachment.Files {
			fmt.Fprintf(out, "Restore %s from %s\n%s\n", f.Path, attachmentPath(root, s), inverseAttachmentDiff(f.Diff))
		}
	}
	if remove && dependencyExists {
		fmt.Fprintln(out, "Remove verified owned dependency:", dependencyDir(root, s))
	} else {
		fmt.Fprintln(out, "Dependency files retained; user-installed modules/Agents and external Collectors are never removed.")
	}
	if err = confirmAttachment(out, in, yes, dry); err != nil || dry {
		return err
	}
	if hasState {
		current, err := loadState(root, s.ID)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(current, st) {
			return errors.New("deployment changed after uninstall preview; rerun uninstall")
		}
		if s.IIS == nil {
			if err = attachmentRecordUnchanged(root, s, attachment, st); err != nil {
				return err
			}
			if err = checkUninstallAttachment(root, s, attachment); err != nil {
				return err
			}
		}
		if err = verifyNative(root, st.Current); err != nil {
			return err
		}
		if err = checkDetachDependents(root, st); err != nil {
			return err
		}
	}
	if remove && dependencyExists {
		if err = checkDependencyRemoval(root, s); err != nil {
			return err
		}
	}
	if hasState {
		if st.Current.active() {
			if err = apply(root, st, s, false, "all", out); err != nil {
				return err
			}
			st, err = loadState(root, s.ID)
			if err != nil {
				return err
			}
		}
		if s.IIS == nil {
			if attachment.SchemaVersion != 0 {
				err = undoAttachment(root, st, true, false, in, out)
			}
			if err != nil {
				return err
			}
		} else if len(st.Current.Native) != 0 {
			n := st.Current
			n.Uninstall = true
			if len(st.OriginalIIS) != 0 {
				n.Native = st.OriginalIIS
			}
			if err = transact(root, st, n, nil, true); err != nil {
				return err
			}
		}
	}
	// Keep the live state until dependency deletion has succeeded. A failure
	// (for example a Windows DLL still in use) can be retried safely.
	if remove && dependencyExists {
		if err = checkDependencyRemoval(root, s); err != nil {
			return err
		}
		if err = checkLiveDependencyReferences(root, s); err != nil {
			return err
		}
		d := dependencyDir(root, s)
		trash := d + ".removed-" + time.Now().UTC().Format("20060102T150405.000000000")
		if err = os.Rename(d, trash); err != nil {
			return fmt.Errorf("site detached but dependency is still in use; retry uninstall: %w", err)
		}
		if err = os.RemoveAll(trash); err != nil {
			fmt.Fprintf(out, "Dependency detached; residual files could not be deleted: %s (%v). Remove after processes release them.\n", trash, err)
		}
	}
	if hasState {
		archive := filepath.Join(root, "removed", s.ID+"-"+time.Now().UTC().Format("20060102T150405.000000000"))
		if err = os.MkdirAll(filepath.Dir(archive), 0700); err != nil {
			return err
		}
		// Preserve the exact resolved paths for a later --remove-dependencies
		// retry without needing the original configuration file.
		if err = writeJSON(filepath.Join(root, "removed", s.ID+".json"), s); err != nil {
			return err
		}
		var original struct {
			VirtualDirectory *string `json:"virtualDirectory"`
		}
		if s.IIS != nil && len(st.OriginalIIS) != 0 {
			if err = json.Unmarshal(st.OriginalIIS, &original); err != nil {
				return err
			}
		}
		if original.VirtualDirectory != nil {
			// A mapping that predates us is restored, not removed. Its existing
			// physical directory must remain available after retiring live state.
			if err = os.Mkdir(archive, 0700); err != nil {
				return err
			}
			latest, err := loadState(root, s.ID)
			if err != nil {
				return err
			}
			if err = writeJSON(filepath.Join(archive, "state.json"), latest); err != nil {
				return err
			}
			if err = os.Remove(filepath.Join(siteDir(root, s), "state.json")); err != nil {
				return err
			}
			fmt.Fprintln(out, "Pre-existing resource mapping and its asset directory retained:", filepath.Join(siteDir(root, s), "public"))
		} else {
			if err = os.Rename(siteDir(root, s), archive); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "Uninstalled. Private backup/archive:", archive)
	} else {
		fmt.Fprintln(out, "Owned OTel dependency removed.")
	}
	return nil
}

func inverseAttachmentDiff(diff string) string {
	lines := strings.Split(diff, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "+") {
			lines[i] = "-" + line[1:]
		} else if strings.HasPrefix(line, "-") {
			lines[i] = "+" + line[1:]
		}
	}
	return strings.Join(lines, "\n")
}

func uninstalledSite(root, id string) (Site, error) {
	var s Site
	b, err := os.ReadFile(filepath.Join(root, "removed", id+".json"))
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.ID != id {
		return s, errors.New("uninstall receipt site mismatch")
	}
	return s, s.validateTarget()
}
