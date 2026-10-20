package rumable

import (
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestAttachmentRecoveryRefusesNewSharedInclude(t *testing.T) {
	dir := t.TempDir()
	main, child, extra := filepath.Join(dir, "nginx.conf"), filepath.Join(dir, "handler.conf"), filepath.Join(dir, "extra.conf")
	childBefore, childAfter := []byte("root /www;\n"), []byte("root /www;\n# managed hook\n")
	source := fmt.Sprintf("events {} http { server { listen 443; server_name example.com; location / { include %s; } } include %s; }", nginxQuote(child), nginxQuote(extra))
	for name, b := range map[string][]byte{main: []byte(source), child: childAfter, extra: []byte(fmt.Sprintf("server { listen 81; location / { include %s; } }", nginxQuote(child)))} {
		if err := os.WriteFile(name, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(dir, "nginx")
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n-V) echo 'nginx version: nginx/1.28.3 --with-http_sub_module';;\n-T) printf '# configuration file %%s:\\n' '%s' '%s' '%s';;\n*) exit 91;;\nesac\n", main, child, extra)
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	s := testSite()
	s.Nginx.Binary = binary
	a := attachmentRecord{SchemaVersion: 1, Site: s, Phase: "attached", Files: []attachmentFile{{Path: child, Before: childBefore, After: childAfter, Mode: 0644}}}
	if err := restoreAttachment(dir, a, Snapshot{Site: s}); err == nil || !strings.Contains(err.Error(), "included 2 times") {
		t.Fatal("undo did not refuse the now-shared handler", err)
	}
	live, _ := os.ReadFile(child)
	if string(live) != string(childAfter) {
		t.Fatal("shared configuration changed before ownership rejection")
	}
}

func TestAttachmentStopsReloadWhenConfigChangesDuringConfigtest(t *testing.T) {
	dir := t.TempDir()
	name, binary, signal := filepath.Join(dir, "native.conf"), filepath.Join(dir, "nginx"), filepath.Join(dir, "reload-sent")
	if err := os.WriteFile(name, []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$*\" in\n*-t*) " + shellCommand("sh", "-c", `printf external > "$1"`, "sh", name) + ";;\n*) " + shellCommand("touch", signal) + ";;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	s := testSite()
	s.Nginx.Binary = binary
	a := attachmentRecord{Site: s, Files: []attachmentFile{{Path: name, Before: []byte("before"), After: []byte("after"), Mode: 0644}}}
	if err := reloadAttachment(t.TempDir(), a, true); err == nil {
		t.Fatal("changed candidate was reloaded")
	}
	if _, err := os.Stat(signal); !os.IsNotExist(err) {
		t.Fatal("reload signal sent after concurrent config change", err)
	}
	live, _ := os.ReadFile(name)
	if string(live) != "external" {
		t.Fatal("concurrent edit was overwritten")
	}
}

func TestAttachmentAtomicReplacementPreservesMetadataAndRefusesConflicts(t *testing.T) {
	name := filepath.Join(t.TempDir(), "server.conf")
	if err := os.WriteFile(name, []byte("before"), 0640); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(name)
	attrErr := syscall.Setxattr(name, "user.rumable", []byte("preserved"), 0)
	if attrErr != nil && attrErr != syscall.ENOTSUP {
		t.Fatal(attrErr)
	}
	// A default ACL added after the original file was created must not grant
	// additional readers to the replacement configuration.
	acl := make([]byte, 4)
	binary.LittleEndian.PutUint32(acl, 2)
	for _, entry := range []struct {
		tag, perm uint16
		id        uint32
	}{{1, 7, 0xffffffff}, {2, 4, 12345}, {4, 0, 0xffffffff}, {16, 4, 0xffffffff}, {32, 0, 0xffffffff}} {
		b := make([]byte, 8)
		binary.LittleEndian.PutUint16(b, entry.tag)
		binary.LittleEndian.PutUint16(b[2:], entry.perm)
		binary.LittleEndian.PutUint32(b[4:], entry.id)
		acl = append(acl, b...)
	}
	aclErr := syscall.Setxattr(filepath.Dir(name), "system.posix_acl_default", acl, 0)
	if aclErr != nil && aclErr != syscall.ENOTSUP {
		t.Fatal(aclErr)
	}
	if err := replaceAttachmentFile(name, []byte("before"), []byte("after"), 0640); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(name)
	originalStat, newStat := info.Sys().(*syscall.Stat_t), after.Sys().(*syscall.Stat_t)
	if after.Mode().Perm() != 0640 || originalStat.Uid != newStat.Uid || originalStat.Gid != newStat.Gid {
		t.Fatal("configuration metadata changed")
	}
	if aclErr == nil {
		t.Log("default ACL inheritance regression exercised")
		if _, err := syscall.Getxattr(name, "system.posix_acl_access", nil); err != syscall.ENODATA {
			t.Fatal("replacement inherited extra ACL access", err)
		}
	}
	if attrErr == nil {
		value := make([]byte, 64)
		n, err := syscall.Getxattr(name, "user.rumable", value)
		if err != nil || string(value[:n]) != "preserved" {
			t.Fatal("configuration xattr lost", err)
		}
	}
	if err := replaceAttachmentFile(name, []byte("before"), []byte("overwrite"), 0640); err == nil {
		t.Fatal("external bytes overwritten")
	}
	link := filepath.Join(filepath.Dir(name), "symlink")
	if err := os.Symlink(name, link); err != nil {
		t.Fatal(err)
	}
	if err := replaceAttachmentFile(link, []byte("after"), []byte("overwrite"), 0640); err == nil {
		t.Fatal("symlink swapped")
	}
	hard := filepath.Join(filepath.Dir(name), "hardlink")
	if err := os.Link(name, hard); err != nil {
		t.Fatal(err)
	}
	if err := replaceAttachmentFile(name, []byte("after"), []byte("overwrite"), 0640); err == nil {
		t.Fatal("hard-linked file replaced")
	}
}

func TestAttachmentRecoversPartiallyWrittenFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head></head></html>"))
	}))
	defer server.Close()
	root := t.TempDir()
	s := testSite()
	s.PublicURL = server.URL + "/"
	s.Nginx.Binary = "/bin/true"
	a := attachmentRecord{SchemaVersion: 1, Site: s, Phase: "applying"}
	for _, name := range []string{"first.conf", "second.conf"} {
		a.Files = append(a.Files, attachmentFile{Path: filepath.Join(root, name), Before: []byte("before\n"), After: []byte("after\n"), Mode: 0644})
	}
	for i, f := range a.Files {
		body := f.Before
		if i == 0 {
			body = f.After
		}
		if err := os.WriteFile(f.Path, body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAttachment(attachmentPath(root, s), a); err != nil {
		t.Fatal(err)
	}
	if attachmentPending(root, s) == nil {
		t.Fatal("unfinished hook did not block deployment")
	}
	if err := restoreAttachment(root, a, Snapshot{Site: s}); err != nil {
		t.Fatal(err)
	}
	for _, f := range a.Files {
		b, _ := os.ReadFile(f.Path)
		if string(b) != string(f.Before) {
			t.Fatal("partial write not recovered")
		}
	}
	saved, err := loadAttachment(root, s)
	if err != nil || saved.Phase != "detached" {
		t.Fatal("recovery record lost", err)
	}
}

func TestAttachmentRecoversCandidateRejectedByNativeParser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head></head></html>"))
	}))
	defer server.Close()
	root := t.TempDir()
	name, binary := filepath.Join(root, "nginx.conf"), filepath.Join(root, "nginx")
	before, after := []byte("# original\n"), []byte("# incompatible native module\n")
	if err := os.WriteFile(name, after, 0644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$1\" in\n-V) echo 'nginx version: nginx/1.28.3 --with-http_sub_module';;\n-T|-t) ! " + shellCommand("grep", "-q", "incompatible", name) + ";;\n*) exit 0;;\nesac\n"
	if err := os.WriteFile(binary, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	s := testSite()
	s.Nginx.Binary, s.PublicURL = binary, server.URL+"/"
	a := attachmentRecord{SchemaVersion: 1, Site: s, Phase: "applying", Files: []attachmentFile{{Path: name, Before: before, After: after, Mode: 0644}}, Inputs: map[string]string{name: hash(before)}}
	if err := restoreAttachment(root, a, Snapshot{Site: s}); err != nil {
		t.Fatal("native parser rejection stranded the candidate configuration", err)
	}
	live, _ := os.ReadFile(name)
	if string(live) != string(before) {
		t.Fatal("original configuration was not restored")
	}
}
