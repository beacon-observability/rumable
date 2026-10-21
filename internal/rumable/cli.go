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
		fmt.Fprintln(out, "rumable init|setup|install-otel|render|attach|check|enable|disable|uninstall|status|rollback|probe|version\nOne command: setup --config rumable.toml --state-dir /var/lib/rumable --yes (RUM only)\nStart: init --server nginx|iis --reporting openway|datakit; edit TOML; install-otel (optional); render; attach; enable\nFlags: --config rumable.toml --state-dir .rumable --site ID (optional for one site)\nattach/install-otel/uninstall: --dry-run previews; --yes confirms without a prompt\nattach: --target FILE:LINE selects a candidate\nuninstall: detach the whole site; --remove-dependencies also removes unchanged, unshared managed dependencies\nrollback --attachment: undo first attachment after disable\nenable/disable: --component all|rum|otel (default all)\nrender --collector-config: print Dataway Collector JSON without deployment changes\ninit: --full lists advanced SDK options; probe: --path / (optional)")
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
	case "init", "setup", "install-otel", "uninstall", "render", "attach", "check", "enable", "disable", "status", "rollback", "probe":
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
	var collectorConfig *bool
	if cmd == "render" {
		collectorConfig = f.Bool("collector-config", false, "print Dataway Collector JSON only; does not modify deployment state")
	}
	component := "all"
	if cmd == "enable" || cmd == "disable" {
		f.StringVar(&component, "component", "all", "all, rum or otel; independently change one component")
	}
	var target *string
	var yes, dry, attachmentUndo *bool
	if cmd == "attach" || cmd == "rollback" || cmd == "install-otel" || cmd == "uninstall" || cmd == "setup" {
		yes = f.Bool("yes", false, "confirm the displayed changes (noninteractive)")
	}
	if cmd == "attach" || cmd == "rollback" || cmd == "install-otel" || cmd == "uninstall" {
		dry = f.Bool("dry-run", false, "preview changes without applying")
	}
	var removeDependencies *bool
	if cmd == "uninstall" {
		removeDependencies = f.Bool("remove-dependencies", false, "also remove unchanged, unshared dependencies installed by Rumable")
	}
	if cmd == "attach" || cmd == "setup" {
		target = f.String("target", "", "candidate FILE:LINE when automatic site selection is ambiguous")
	}
	if cmd == "rollback" {
		attachmentUndo = f.Bool("attachment", false, "undo/recover first attachment (disable first)")
	}
	if cmd == "init" {
		server = f.String("server", "nginx", "nginx or iis")
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
	if component != "all" && component != "rum" && component != "otel" {
		return errors.New("--component must be all, rum or otel")
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
		*id, err = selectSite(root, *config, cmd == "status" || cmd == "disable" || cmd == "rollback" || cmd == "probe" || cmd == "attach" || cmd == "uninstall")
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
		collector := ""
		if st.Current.OTelCollector != "" {
			collector = filepath.Join(siteDir(root, st.Current.Site), filepath.FromSlash(st.Current.OTelCollector))
		}
		return output(out, map[string]any{"site": *id, "enabled": st.Current.active(), "rumEnabled": st.Current.Enabled, "otelEnabled": st.Current.OTelEnabled, "otelExporterOwner": st.Current.OTelOwner, "otelCollectorConfig": collector, "platform": st.Current.Site.platform(), "sdk": st.Current.SDK, "sdkURL": st.Current.SDKURL, "lastCheck": st.LastCheck, "drift": drift, "pendingTransaction": pending == nil, "attachment": attachmentStatus, "host": hostInfo()})
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
		if expected != nil {
			if err := probeOTelState(*expected); err != nil {
				return err
			}
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
	if cmd == "uninstall" {
		st, e := loadState(root, *id)
		s := st.Current.Site
		if os.IsNotExist(e) {
			s, e = uninstalledSite(root, *id)
			if os.IsNotExist(e) {
				s, e = loadConfig(*config, *id)
			}
		}
		if e != nil {
			return e
		}
		return uninstallSite(root, s, *removeDependencies, *yes, *dry, os.Stdin, out)
	}
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
		s, activate, err := componentSite(st.Current.Site, st.Current, component, false)
		if err != nil {
			return err
		}
		return apply(root, st, s, activate, component, out)
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
	if cmd == "setup" {
		return setupSite(root, s, *target, *yes, os.Stdin, out)
	}
	if cmd == "install-otel" {
		return installOTel(root, s, *yes, *dry, os.Stdin, out)
	}
	s = resolveOTelPaths(root, s)
	if cmd == "render" {
		if *collectorConfig {
			n := Snapshot{}
			files := map[string][]byte{}
			renderOTelCollector(s, &n, files)
			if n.OTelCollector == "" {
				return errors.New("--collector-config requires Linux Nginx otel.backend=dataway")
			}
			_, err := fmt.Fprintln(out, string(files[n.OTelCollector]))
			return err
		}
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
		detail := "server prerequisites and HTTP checks passed; browser execution and backend ingestion are not verified"
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
	s, activate, err := componentSite(s, st.Current, component, true)
	if err != nil {
		return err
	}
	if err = s.validate(); err != nil {
		return err
	}
	return apply(root, st, s, activate, component, out)
}

func componentSite(s Site, current Snapshot, component string, on bool) (Site, bool, error) {
	if component == "all" {
		return s, on, nil
	}
	if component == "rum" {
		s.OTel = current.Site.OTel
	} else {
		s.RUM = current.Site.RUM
		s.SDK = current.Site.SDK
		s.Include, s.Exclude = current.Site.Include, current.Site.Exclude
	}
	// Clone pointer fields: changing the desired state must not mutate history.
	if s.OTel != nil {
		copy := *s.OTel
		s.OTel = &copy
	}
	rumOn := current.Enabled
	if component == "rum" {
		rumOn = on
	} else {
		if s.OTel == nil {
			return s, false, errors.New("configure OTel before using --component otel")
		}
		s.OTel.Enabled = on
	}
	if component == "rum" && s.OTel != nil {
		s.OTel.Enabled = current.OTelEnabled
	}
	s.RUM.Enabled = &rumOn
	return s, rumOn || s.OTel != nil && s.OTel.Enabled, nil
}

func output(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
