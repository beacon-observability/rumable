package rumable

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func Run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(out, "rumable init|render|attach|check|enable|disable|status|rollback|probe|version\nStart: init --server nginx|apache|iis --reporting openway|datakit; edit TOML; render; attach; enable\nFlags: --config rumable.toml --state-dir .rumable --site ID (optional for one site)\nattach: --dry-run previews; --target FILE:LINE selects; --yes confirms without a prompt\nrollback --attachment: undo first attachment after disable\ninit: --full lists advanced SDK options; probe: --path / (optional)")
		return nil
	}
	if args[0] == "version" {
		_, info, err := sdk()
		if err != nil {
			return err
		}
		return output(out, map[string]any{"version": Version, "sdk": info, "host": hostInfo()})
	}
	cmd := args[0]
	switch cmd {
	case "init", "render", "attach", "check", "enable", "disable", "status", "rollback", "probe":
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	f.SetOutput(out)
	config := f.String("config", "rumable.toml", "TOML configuration file")
	id := f.String("site", "", "site id")
	dir := f.String("state-dir", ".rumable", "managed deployment directory (use a stable absolute path)")
	path := f.String("path", "", "probe path (defaults to configured probePaths)")
	var server, reporting *string
	var full *bool
	var target *string
	var yes, dry, attachmentUndo *bool
	if cmd == "attach" || cmd == "rollback" {
		yes = f.Bool("yes", false, "confirm the displayed attachment changes (noninteractive)")
		dry = f.Bool("dry-run", false, "preview attachment changes without applying")
	}
	if cmd == "attach" {
		target = f.String("target", "", "candidate FILE:LINE when automatic site selection is ambiguous")
	}
	if cmd == "rollback" {
		attachmentUndo = f.Bool("attachment", false, "undo/recover first attachment (disable first)")
	}
	if cmd == "init" {
		server = f.String("server", "nginx", "nginx, apache or iis")
		reporting = f.String("reporting", "openway", "openway or datakit")
		full = f.Bool("full", false, "include advanced SDK settings")
	}
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if cmd == "init" {
		text, err := initialConfig(*server, *reporting, *id, *full)
		if err != nil {
			return err
		}
		file, err := os.OpenFile(*config, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = file.WriteString(text)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		_, err = fmt.Fprintf(out, "Created %s (%s / %s). Fill in the site URL, server target and RUM values, then run render.\n", *config, *server, *reporting)
		return err
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	if *id == "" {
		*id, err = selectSite(root, *config, cmd == "status" || cmd == "disable" || cmd == "rollback" || cmd == "probe" || cmd == "attach")
		if err != nil {
			return err
		}
	}
	if !idPattern.MatchString(*id) {
		return errors.New("a valid --site ID is required")
	}
	if cmd == "status" {
		st, err := loadState(root, *id)
		if err != nil {
			return err
		}
		drift := ""
		if e := verifyFiles(root, st.Current); e != nil {
			drift = e.Error()
		}
		if drift == "" {
			if e := verifyNative(root, st.Current); e != nil {
				drift = e.Error()
			}
		}
		var attachmentStatus any
		if a, e := loadAttachment(root, st.Current.Site); e == nil {
			attachmentStatus = map[string]any{"phase": a.Phase, "target": a.Target, "backup": attachmentPath(root, st.Current.Site)}
		} else if !errors.Is(e, os.ErrNotExist) {
			attachmentStatus = map[string]string{"error": e.Error()}
		}
		_, pending := os.Stat(journalPath(root, st.Current.Site))
		return output(out, map[string]any{"site": *id, "enabled": st.Current.Enabled, "platform": st.Current.Site.platform(), "sdk": st.Current.SDK, "sdkURL": st.Current.SDKURL, "lastCheck": st.LastCheck, "drift": drift, "pendingTransaction": pending == nil, "attachment": attachmentStatus, "host": hostInfo()})
	}
	if cmd == "probe" {
		st, stateErr := loadState(root, *id)
		if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
			return stateErr
		}
		s := st.Current.Site
		var expected *Snapshot
		if stateErr == nil {
			expected = &st.Current
		} else {
			s, err = loadConfig(*config, *id)
			if err != nil {
				return err
			}
		}
		paths := s.ProbePaths
		if *path != "" {
			paths = []string{*path}
		}
		for _, p := range paths {
			if !s.matches(p) {
				return fmt.Errorf("path %q is outside selected paths", p)
			}
		}
		results, err := probePages(s, paths, expected)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err = output(out, r); err != nil {
				return err
			}
		}
		return nil
	}
	unlock, err := lock(root)
	if err != nil {
		return err
	}
	defer unlock()
	if cmd == "rollback" {
		st, err := loadState(root, *id)
		if err != nil {
			return err
		}
		if *attachmentUndo {
			return undoAttachment(root, st, *yes, *dry, os.Stdin, out)
		}
		if *yes || *dry {
			return errors.New("--yes/--dry-run require rollback --attachment")
		}
		if err = attachmentPending(root, st.Current.Site); err != nil {
			return err
		}
		return rollback(root, st, out)
	}
	if cmd == "disable" {
		st, e := loadState(root, *id)
		if e != nil {
			return e
		}
		return apply(root, st, st.Current.Site, false, out)
	}
	if cmd == "attach" {
		st, err := loadState(root, *id)
		if err != nil {
			return fmt.Errorf("render the site first: %w", err)
		}
		return attachSite(root, st, *target, *yes, *dry, os.Stdin, out)
	}
	s, err := loadConfig(*config, *id)
	if err != nil {
		return err
	}
	if cmd == "render" {
		_, err = prepare(root, s)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Rendered disabled deployment. Attachment details:", filepath.Join(siteDir(root, s), "ATTACH.md"))
		fmt.Fprint(out, setupInstructions(root, s, *config))
		return nil
	}
	st, err := loadState(root, *id)
	if err != nil {
		return fmt.Errorf("render the site first: %w", err)
	}
	if cmd == "check" {
		if !compatibleTarget(st.Current.Site, s) {
			return errors.New("site target differs from the rendered attachment")
		}
		if _, e := os.Stat(journalPath(root, s)); e == nil {
			return errors.New("unfinished transaction; use rollback before check")
		}
		e := verifyFiles(root, st.Current)
		if e == nil {
			e = verifyNative(root, st.Current)
		}
		if e == nil {
			e = preflight(root, s)
		}
		detail := "server prerequisites and sampled HTML/CSP passed; endpoint reachability/CORS/browser execution/ingestion need browser verification"
		if e != nil {
			detail = e.Error()
		}
		st.LastCheck = &CheckRecord{time.Now().UTC(), e == nil, detail}
		if err = saveState(root, st); err != nil {
			return err
		}
		if e != nil {
			return e
		}
		return output(out, st.LastCheck)
	}
	return apply(root, st, s, cmd == "enable", out)
}

func output(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
