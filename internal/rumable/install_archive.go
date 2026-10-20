package rumable

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// Only regular files/directories are extracted. Links, special files, duplicate
// entries and Windows path aliases must never let an archive escape staging.
func extractDependency(name, dest string) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	header, _ := r.Peek(8)
	var total int64
	seen := map[string]bool{}
	write := func(name string, size int64, dir bool, mode os.FileMode, body io.Reader) error {
		name = strings.TrimSuffix(name, "/")
		if name == "." {
			return nil
		}
		if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe archive path %q", name)
		}
		for _, part := range strings.Split(name, "/") {
			upper := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
			device := len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9'
			if strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" || device {
				return fmt.Errorf("unsafe archive filename %q", name)
			}
		}
		key := strings.ToLower(name)
		if seen[key] || len(seen) > 20000 {
			return errors.New("duplicate archive entry or entry limit exceeded")
		}
		seen[key] = true
		total += size
		if size < 0 || size > maxDependencyBytes || total > 768<<20 {
			return errors.New("expanded dependency exceeds size limit")
		}
		p := filepath.Join(dest, filepath.FromSlash(name))
		if dir {
			return os.MkdirAll(p, 0755)
		}
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		permissions := os.FileMode(0644)
		if mode&0111 != 0 {
			permissions = 0755
		}
		file, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permissions)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(file, io.LimitReader(body, size+1))
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if n != size {
			return errors.New("archive entry size mismatch")
		}
		return closeErr
	}
	if strings.HasPrefix(string(header), "PK\x03\x04") {
		z, err := zip.OpenReader(name)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, e := range z.File {
			if e.Mode()&os.ModeType != 0 && !e.FileInfo().IsDir() {
				return errors.New("links/special files are not allowed in dependency archives")
			}
			if e.UncompressedSize64 > maxDependencyBytes {
				return errors.New("oversized ZIP entry")
			}
			b, err := e.Open()
			if err != nil {
				return err
			}
			err = write(e.Name, int64(e.UncompressedSize64), e.FileInfo().IsDir(), e.Mode(), b)
			b.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	var stream io.Reader = r
	var process *exec.Cmd
	if strings.HasPrefix(string(header), "!<arch>\n") {
		// dpkg-deb only decompresses the data archive; maintainer scripts are
		// never executed and tar entry validation remains in Go.
		process = exec.Command("dpkg-deb", "--fsys-tarfile", name)
		pipe, err := process.StdoutPipe()
		if err != nil {
			return err
		}
		if err = process.Start(); err != nil {
			return err
		}
		defer func() { process.Process.Kill(); process.Wait() }()
		stream = pipe
	} else if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		g, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		defer g.Close()
		stream = g
	}
	t := tar.NewReader(stream)
	for {
		e, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if e.Typeflag != tar.TypeDir && e.Typeflag != tar.TypeReg {
			if e.Typeflag == tar.TypeXGlobalHeader {
				continue
			}
			return errors.New("links/special files are not allowed in dependency archives")
		}
		if e.Typeflag == tar.TypeDir && (e.Name == "./" || e.Name == ".") {
			continue
		}
		name := strings.TrimPrefix(e.Name, "./")
		if err = write(name, e.Size, e.Typeflag == tar.TypeDir, os.FileMode(e.Mode), t); err != nil {
			return err
		}
	}
	if process != nil {
		return process.Wait()
	}
	return nil
}
