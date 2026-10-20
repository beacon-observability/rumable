package rumable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

// OTelInstall selects a verified offline or HTTPS module distribution. With no
// override, installers use the pinned upstream Agent or the local APT index.
type OTelInstall struct {
	Archive string `json:"archive,omitempty" toml:"archive"`
	URL     string `json:"url,omitempty" toml:"url"`
	SHA256  string `json:"sha256,omitempty" toml:"sha256"`
}

func (i *OTelInstall) validate() error {
	if i == nil {
		return nil
	}
	if i.Archive != "" && i.URL != "" {
		return errors.New("otel.install: choose archive or url, not both")
	}
	if (i.Archive != "" || i.URL != "") != (i.SHA256 != "") || (i.SHA256 != "" && !digestPattern.MatchString(i.SHA256)) {
		return errors.New("otel.install archive/url requires a lowercase SHA-256 digest")
	}
	if i.Archive != "" && !filepath.IsAbs(i.Archive) {
		return errors.New("otel.install.archive must be an absolute local path")
	}
	if i.URL != "" {
		u, err := url.Parse(i.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery {
			return errors.New("otel.install.url must use HTTPS without credentials, query or fragment")
		}
	}
	if strings.ContainsAny(i.Archive, "\x00\r\n") {
		return errors.New("OTel installation paths cannot contain control characters")
	}
	return nil
}

func dependencyDir(root string, s Site) string {
	key := s.platform()
	if s.IIS != nil {
		key += "/dotnet-1.17.0"
	} else {
		target := append([]string{s.Nginx.Binary}, s.Nginx.Args...)
		b, _ := json.Marshal(target)
		key += "/" + string(b)
	}
	if s.OTel != nil && s.OTel.Install != nil {
		b, _ := json.Marshal(s.OTel.Install)
		key += string(b)
	}
	// The public, read-only Agent tree is outside the private state directory.
	return filepath.Join(root+"-otel", s.platform()+"-"+hash([]byte(key))[:20])
}

func resolveOTelPaths(root string, s Site) Site {
	if s.OTel == nil {
		return s
	}
	o := *s.OTel
	s.OTel = &o
	d := dependencyDir(root, s)
	if s.IIS != nil {
		if o.AgentPath == "" {
			o.AgentPath = filepath.Join(d, "agent")
		}
	} else if o.ModulePath == "" {
		o.ModulePath = filepath.Join(d, "ngx_otel_module.so")
	}
	return s
}

type dependencyRecord struct {
	SchemaVersion int               `json:"schemaVersion"`
	Root          string            `json:"root"`
	Kind          string            `json:"kind"`
	Platform      string            `json:"platform"`
	Source        []string          `json:"source"`
	Files         map[string]string `json:"files"`
	PendingACL    bool              `json:"pendingACL,omitempty"`
}

func installOTel(root string, s Site, yes, dry bool, in io.Reader, out io.Writer) error {
	if s.OTel == nil {
		return errors.New("configure [sites.otel] first")
	}
	if err := s.OTel.validate(s); err != nil {
		return err
	}
	if s.IIS != nil {
		if runtime.GOOS != "windows" {
			return errors.New("install the IIS Agent on the target Windows host")
		}
		if _, err := iisCommand(root, s, "install-check", nil); err != nil {
			return err
		}
	} else if runtime.GOOS != "linux" {
		return errors.New("install native modules on the target Linux host")
	}
	if s.Nginx != nil {
		if _, err := nginxInstallVersion(s); err != nil {
			return err
		}
	}
	s = resolveOTelPaths(root, s)
	d := dependencyDir(root, s)
	managed := s.OTel.ModulePath
	if s.IIS != nil {
		managed = s.OTel.AgentPath
	}
	if !strings.HasPrefix(managed, d+string(filepath.Separator)) {
		return fmt.Errorf("explicit modulePath/agentPath is externally managed; omit it to install in %s (existing files are never replaced)", d)
	}
	if _, err := os.Lstat(d); err == nil {
		r, err := verifyDependency(root, s)
		if err != nil {
			return err
		}
		if r.PendingACL {
			fmt.Fprintln(out, "Complete the interrupted IIS Agent read-permission setup:", d)
			if err = confirmAttachment(out, in, yes, dry); err != nil || dry {
				return err
			}
			live, e := verifyDependency(root, s)
			if e != nil {
				return e
			}
			if !reflect.DeepEqual(live, r) {
				return errors.New("dependency record changed after preview; rerun install-otel")
			}
			if err = finishDependencyACL(d, r); err != nil {
				return err
			}
		}
		fmt.Fprintln(out, "OTel dependency already installed and verified:", managed)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	fmt.Fprintf(out, "Install %s OTel dependency into %s\n", s.platform(), d)
	if s.OTel.Install != nil && (s.OTel.Install.Archive != "" || s.OTel.Install.URL != "") {
		fmt.Fprintf(out, "Verified custom distribution: %s%s (SHA-256 %s)\n", s.OTel.Install.Archive, s.OTel.Install.URL, s.OTel.Install.SHA256)
	} else if s.Nginx != nil {
		fmt.Fprintln(out, "Download an exact-version nginx-module-otel from configured APT repositories; extract privately without running package scripts.")
	} else {
		fmt.Fprintln(out, "Download the pinned official .NET Agent v1.17.0; grant IIS_IUSRS read/execute on its tree. No global IIS registration or restart.")
	}
	if err := confirmAttachment(out, in, yes, dry); err != nil || dry {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(d), 0755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(filepath.Dir(d), ".rumable-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	stage := filepath.Join(work, "package")
	if err = os.Mkdir(stage, 0755); err != nil {
		return err
	}
	var sources []string
	if s.OTel.Install != nil && (s.OTel.Install.Archive != "" || s.OTel.Install.URL != "") {
		i := s.OTel.Install
		archive := filepath.Join(work, "download")
		if err = fetchDependency(archive, i.Archive, i.URL, i.SHA256); err == nil {
			err = stageDependencyArchive(s, archive, stage, work)
		}
		sources = []string{i.Archive + i.URL + " sha256=" + i.SHA256}
	} else if s.Nginx != nil {
		sources, err = installNginx(s, work, stage)
	} else {
		sources, err = installDotnet(work, stage)
	}
	if err != nil {
		return err
	}
	staged := s
	o := *s.OTel
	staged.OTel = &o
	if s.IIS != nil {
		o.AgentPath = filepath.Join(stage, "agent")
	} else {
		o.ModulePath = filepath.Join(stage, strings.TrimPrefix(s.OTel.ModulePath, d+string(filepath.Separator)))
	}
	if err = validateInstalledDependency(staged); err != nil {
		return err
	}
	files, err := dependencyFiles(stage)
	if err != nil {
		return err
	}
	record := dependencyRecord{1, root, s.platform(), runtime.GOOS + "/" + runtime.GOARCH, sources, files, s.IIS != nil}
	if err = writeJSON(filepath.Join(stage, "rumable-install.json"), record); err != nil {
		return err
	}
	if _, err = os.Lstat(d); !os.IsNotExist(err) {
		return errors.New("dependency destination changed during install; refusing replacement")
	}
	if err = os.Rename(stage, d); err != nil {
		return err
	}
	if s.IIS != nil {
		if err = finishDependencyACL(d, record); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "Installed and verified:", managed, "\nNext: render, attach (Linux), then enable. Existing rendered sites with explicit paths keep their original dependency.")
	return nil
}

func finishDependencyACL(dir string, r dependencyRecord) error {
	if _, err := command("icacls.exe", dir, "/grant", "*S-1-5-32-568:(OI)(CI)(RX)", "/T", "/Q"); err != nil {
		return fmt.Errorf("IIS Agent files retained, read-permission setup incomplete; rerun install-otel or uninstall --remove-dependencies: %w", err)
	}
	r.PendingACL = false
	return writeJSON(filepath.Join(dir, "rumable-install.json"), r)
}

const maxDependencyBytes = 256 << 20

func fetchDependency(dest, local, address, digest string) error {
	var reader io.ReadCloser
	var err error
	if local != "" {
		reader, err = os.Open(local)
	} else {
		client := &http.Client{Timeout: 3 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Scheme != "https" || len(via) > 10 {
				return errors.New("invalid dependency download redirect")
			}
			return nil
		}}
		var res *http.Response
		res, err = client.Get(address)
		if err == nil {
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				return fmt.Errorf("dependency download: HTTP %d", res.StatusCode)
			}
			reader = res.Body
		}
	}
	if err != nil {
		return err
	}
	defer reader.Close()
	b, err := io.ReadAll(io.LimitReader(reader, maxDependencyBytes+1))
	if err != nil {
		return err
	}
	if len(b) > maxDependencyBytes || hash(b) != digest {
		return errors.New("dependency size limit or SHA-256 mismatch; nothing installed")
	}
	return os.WriteFile(dest, b, 0600)
}

func installCommand(dir, binary string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	c := exec.CommandContext(ctx, binary, args...)
	c.Dir = dir
	b, err := c.CombinedOutput()
	if len(b) > 8192 {
		b = b[len(b)-8192:]
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", binary, err, b)
	}
	return b, nil
}

func dependencyFiles(dir string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("dependency symlink is not supported: %s", p)
		}
		if p == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		if e.IsDir() {
			files[filepath.ToSlash(rel)+"/"] = "directory"
			return nil
		}
		if !e.Type().IsRegular() {
			return fmt.Errorf("unsupported dependency file: %s", p)
		}
		if rel == "rumable-install.json" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err == nil {
			files[filepath.ToSlash(rel)] = hash(b)
		}
		return err
	})
	return files, err
}

func checkManagedDependency(root string, s Site) error {
	if s.OTel == nil {
		return nil
	}
	d := dependencyDir(root, s) + string(filepath.Separator)
	if !strings.HasPrefix(s.OTel.ModulePath, d) && !strings.HasPrefix(s.OTel.AgentPath, d) {
		return nil
	}
	r, err := verifyDependency(root, s)
	if err != nil {
		return fmt.Errorf("managed OTel dependency is unavailable or changed; run install-otel before attachment/enable: %w", err)
	}
	if r.PendingACL {
		return errors.New("IIS Agent read-permission setup is incomplete; rerun install-otel before enable")
	}
	return nil
}

func verifyDependency(root string, s Site) (dependencyRecord, error) {
	d := dependencyDir(root, s)
	var r dependencyRecord
	info, err := os.Lstat(d)
	if err != nil {
		return r, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return r, errors.New("dependency directory has been replaced")
	}
	b, err := os.ReadFile(filepath.Join(d, "rumable-install.json"))
	if err != nil {
		return r, fmt.Errorf("unowned dependency directory; refusing modification: %w", err)
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, err
	}
	if r.SchemaVersion != 1 || r.Root != root || r.Kind != s.platform() || len(r.Files) == 0 {
		return r, errors.New("dependency ownership mismatch")
	}
	live, err := dependencyFiles(d)
	if err != nil {
		return r, err
	}
	if len(live) != len(r.Files) {
		return r, errors.New("dependency files changed; refusing removal/reuse")
	}
	for p, v := range r.Files {
		if live[p] != v {
			return r, fmt.Errorf("dependency file changed: %s", p)
		}
	}
	return r, nil
}
