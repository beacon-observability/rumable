package rumable

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func attachmentIdentity(name string) (string, error) {
	i, err := os.Stat(name)
	if err != nil {
		return "", err
	}
	s := i.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%d:%d:%d:%d:%d:%v", s.Dev, s.Ino, s.Uid, s.Gid, s.Mode, s.Ctim), nil
}

// Configuration files may carry SELinux labels or ACLs. Renaming a freshly
// created file without preserving those attributes can make a valid hook unusable.
func replaceAttachmentFile(name string, before, after []byte, mode os.FileMode) error {
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || stat.Nlink != 1 {
		return fmt.Errorf("refusing non-regular or hard-linked configuration %s", name)
	}
	check := func() error {
		current, err := os.Lstat(name)
		if err != nil {
			return err
		}
		live, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		currentStat, ok := current.Sys().(*syscall.Stat_t)
		if !ok || !os.SameFile(info, current) || currentStat.Ctim != stat.Ctim || currentStat.Uid != stat.Uid || currentStat.Gid != stat.Gid || current.Mode() != info.Mode() || current.Mode().Perm() != mode || !bytes.Equal(live, before) {
			return fmt.Errorf("external change to %s; refusing attachment overwrite", name)
		}
		return nil
	}
	if err = check(); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(name), ".rumable-attach-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	fail := func(e error) error { temp.Close(); return e }
	if _, err = temp.Write(after); err != nil {
		return fail(err)
	}
	if err = temp.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
		return fail(err)
	}
	// CreateTemp inherits a parent's default ACL. Clear it while the file is
	// still 0600; chmod could otherwise enable named users absent from the source.
	if err = syscall.Removexattr(temp.Name(), "system.posix_acl_access"); err != nil && err != syscall.ENODATA && err != syscall.ENOTSUP {
		return fail(err)
	}
	if err = temp.Chmod(mode); err != nil {
		return fail(err)
	}
	size, err := syscall.Listxattr(name, nil)
	if err != nil && err != syscall.ENOTSUP {
		return fail(err)
	}
	if size > 0 {
		attrs := make([]byte, size)
		count, err := syscall.Listxattr(name, attrs)
		if err != nil {
			return fail(err)
		}
		for _, key := range strings.Split(string(attrs[:count]), "\x00") {
			if key == "" {
				continue
			}
			length, err := syscall.Getxattr(name, key, nil)
			if err != nil {
				return fail(err)
			}
			value := make([]byte, length)
			length, err = syscall.Getxattr(name, key, value)
			if err != nil {
				return fail(err)
			}
			if err = syscall.Setxattr(temp.Name(), key, value[:length], 0); err != nil {
				return fail(err)
			}
		}
	}
	if err = temp.Sync(); err != nil {
		return fail(err)
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = check(); err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), name); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
