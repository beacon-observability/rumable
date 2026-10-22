package rumable

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// setupSite composes the existing ownership checks and recovery mechanisms.
// It runs under the same operation lock as individual deployment commands.
func setupSite(root string, s Site, target string, yes bool, in io.Reader, out io.Writer) error {
	if s.OTel != nil || !s.RUM.isEnabled() {
		return errors.New("setup supports RUM-only onboarding; use the individual commands for OTel")
	}
	if s.IIS != nil {
		if target != "" {
			return errors.New("--target is only used for Nginx")
		}
		fmt.Fprintf(out, "Set up RUM on IIS %s%s; manage injection, resource mapping and application static/dynamic compression.\n", s.IIS.Site, s.IIS.Application)
		if err := confirmAttachment(out, in, yes, false); err != nil {
			return err
		}
	}
	st, err := loadState(root, s.ID)
	if errors.Is(err, os.ErrNotExist) {
		st, err = prepare(root, s)
		if err == nil {
			// Use the persisted representation, just like separate render/attach
			// invocations (omitempty slices and numeric interface values normalize).
			st, err = loadState(root, s.ID)
		}
	}
	if err != nil {
		return err
	}
	if !compatibleTarget(st.Current.Site, s) || st.Current.Site.OTel != nil {
		return errors.New("setup target differs from existing deployment; use its original target or a new site ID")
	}
	if _, err = os.Stat(journalPath(root, s)); err == nil {
		return errors.New("unfinished transaction; use rollback before setup")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if s.Nginx != nil {
		if err = attachSite(root, st, target, yes, false, in, out); err != nil {
			return err
		}
	}
	if err = apply(root, st, s, true, "all", out); err != nil {
		// Failed activation can already have served HTML. Keep the attachment
		// and its assets reachable; apply restores injection or leaves a journal.
		return fmt.Errorf("setup failed (state, assets and attachment backups retained): %w", err)
	}
	fmt.Fprintln(out, "RUM setup complete; HTTP injection and assets verified for these URLs only:")
	for _, path := range s.ProbePaths {
		fmt.Fprintln(out, "  "+pageURL(s, path))
	}
	fmt.Fprintln(out, "Other pages and previously cached browser responses are not verified. Configure probePaths for every HTML handler; verify normal browser refresh and received events separately.")
	return nil
}
