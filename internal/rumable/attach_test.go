package rumable

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nginxAttachFixture(t *testing.T, source string) *attachConfig {
	t.Helper()
	name := filepath.Join(t.TempDir(), "nginx.conf")
	if err := os.WriteFile(name, []byte(source), 0640); err != nil {
		t.Fatal(err)
	}
	c, err := nginxAttachmentConfig([]byte("# configuration file " + name + ":\n" + source + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestAttachmentNginxPreservesBusinessHeadersAndOtherSites(t *testing.T) {
	root := t.TempDir()
	c := nginxAttachFixture(t, `events {} http {
 proxy_set_header Host business.test;
 proxy_set_header Authorization $http_authorization;
 proxy_set_header X-Tenant "tenant with spaces";
 server { listen 443; server_name example.com;
  location / { proxy_pass http://backend; }
  location /static-a { root /www; }
  location /static-b { root /www; }
  location /local { proxy_set_header X-Local local; proxy_set_header Accept-Encoding gzip; proxy_pass http://backend; proxy_cache pages; }
 }
 server { listen 81; server_name other.test; location / { root /other; } }
}`)
	s := testSite()
	files, _, err := c.planNginx(root, s, "", "--with-http_gzip_static_module")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatal("wrong changed file count")
	}
	changed := string(files[0].After)
	if !strings.Contains(changed, `proxy_set_header Authorization $http_authorization;`) || strings.Count(changed, `proxy_set_header X-Tenant "tenant with spaces";`) != 2 {
		t.Fatal("inherited business headers were lost")
	}
	local := strings.Split(strings.Split(changed, "location /local")[1], "server { listen 81")[0]
	if strings.Contains(local, "Accept-Encoding gzip") || !strings.Contains(local, "Accept-Encoding \"\"") || !strings.Contains(local, "proxy_cache off;") || strings.Contains(local, "X-Tenant") {
		t.Fatal("local header inheritance or compression/cache merge changed", local)
	}
	if !strings.Contains(changed, "server { listen 81; server_name other.test; location / { root /other; } }") {
		t.Fatal("unselected site changed")
	}
	if err := os.WriteFile(files[0].Path, files[0].After, 0640); err != nil {
		t.Fatal(err)
	}
	a := attachmentRecord{SchemaVersion: 1, Site: s, Phase: "attached", Files: files}
	if err := writeJSON(attachmentPath(root, s), a); err != nil {
		t.Fatal(err)
	}
	if err := attachmentReview(root, s, false); err != nil {
		t.Fatal("identical handlers were mistaken for duplicate hooks", err)
	}
	if err := os.WriteFile(files[0].Path, files[0].Before, 0640); err != nil {
		t.Fatal(err)
	}
	if files[0].Mode != 0640 {
		t.Fatal("mode not backed up")
	}
	// Preview must never mutate the serving config.
	live, _ := os.ReadFile(files[0].Path)
	if !bytes.Equal(live, files[0].Before) {
		t.Fatal("preview modified server")
	}
}
func TestAttachmentNginxRejectsSharedIncludesAndUnsafeMerges(t *testing.T) {
	for _, directive := range []string{"sub_filter old new;", "rewrite ^ /other;", "try_files $uri.gz $uri;", "fastcgi_pass backend;"} {
		c := nginxAttachFixture(t, "events {} http { server { listen 443; server_name example.com; location / { "+directive+" } } }")
		if _, _, err := c.planNginx("/state", testSite(), "", ""); err == nil {
			t.Fatal("unsafe merge accepted", directive)
		}
	}
	c := nginxAttachFixture(t, "events {} http { server { listen 443; server_name example.com; location / {} } }")
	for name := range c.uses {
		c.uses[name] = 2
	}
	if _, _, err := c.planNginx("/state", testSite(), "", ""); err == nil {
		t.Fatal("shared source file edited")
	}
}
func TestAttachmentSiteSelectionRequiresUnambiguousName(t *testing.T) {
	c := nginxAttachFixture(t, "events {} http {\n server { listen 443; server_name example.com; location / {} }\n server { listen 443; server_name example.com; location / {} } }")
	_, _, err := c.planNginx("/state", testSite(), "", "")
	if err == nil || !strings.Contains(err.Error(), "--target FILE:LINE") {
		t.Fatal("ambiguous host was guessed", err)
	}
	candidate := c.root.children[1].children[0]
	if _, target, err := c.planNginx("/state", testSite(), candidate.key(), ""); err != nil || target != candidate.key() {
		t.Fatal("explicit selection failed", err)
	}
	c = nginxAttachFixture(t, "events {} http { server { listen 443; server_name example.com; } server { listen 443; server_name other.test; } }")
	candidate = c.root.children[1].children[0]
	if _, _, err = c.planNginx("/state", testSite(), candidate.key(), ""); err == nil {
		t.Fatal("same-line target selected another site")
	}
}

func TestAttachmentRejectsNamespaceOwnedByAnotherDeployment(t *testing.T) {
	for _, conflict := range []string{`map $uri $rumable_test_path { default 0; }`, `server { listen 81; location /__rumable__/test/ {} }`, "# BEGIN rumable-test http\n"} {
		c := nginxAttachFixture(t, "events {} http {\n"+conflict+"\nserver { listen 443; server_name example.com; } }")
		if _, _, err := c.planNginx("/state", testSite(), "", ""); err == nil {
			t.Fatal("other deployment namespace was reused", conflict)
		}
	}
	c := nginxAttachFixture(t, `events {} http { map $uri $rumable_test_other_path { default 0; } server { listen 443; server_name example.com; } }`)
	if _, _, err := c.planNginx("/state", testSite(), "", ""); err != nil {
		t.Fatal("different site sharing an id prefix was rejected", err)
	}
}

func TestAttachmentSourceAndBackupConflicts(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "parent.conf")
	if err := os.WriteFile(name, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "enabled.conf")
	if err := os.Symlink(name, alias); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(name)
	if err != nil {
		t.Fatal(err)
	}
	a := attachmentRecord{Inputs: map[string]string{name: hash([]byte("original"))}, Aliases: map[string]string{alias: canonical}}
	if err := verifyAttachmentSources(t.TempDir(), a); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("external edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifyAttachmentSources(t.TempDir(), a) == nil {
		t.Fatal("changed parent configuration accepted during recovery")
	}
	if err := os.WriteFile(name, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("another.conf", alias); err != nil {
		t.Fatal(err)
	}
	if verifyAttachmentSources(t.TempDir(), a) == nil {
		t.Fatal("retargeted include symlink accepted")
	}
	archive := filepath.Join(dir, "attachment.saved.json")
	if err := archiveAttachment(archive, a); err != nil {
		t.Fatal(err)
	}
	if err := archiveAttachment(archive, a); err != nil {
		t.Fatal("identical archive cannot be reused", err)
	}
	if err := os.WriteFile(archive, []byte("external backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if archiveAttachment(archive, a) == nil {
		t.Fatal("existing backup overwritten")
	}
	live, _ := os.ReadFile(archive)
	if string(live) != "external backup" {
		t.Fatal("archive conflict lost bytes")
	}
}

func TestAttachmentConfirmationAndRecoveryConflict(t *testing.T) {
	var out bytes.Buffer
	if confirmAttachment(&out, strings.NewReader("no\n"), false, false) == nil {
		t.Fatal("missing consent accepted")
	}
	if err := confirmAttachment(&out, strings.NewReader("yes\n"), false, false); err != nil {
		t.Fatal(err)
	}
	if err := confirmAttachment(io.Discard, strings.NewReader(""), false, true); err != nil {
		t.Fatal("preview tried to prompt")
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "server.conf")
	before, after := []byte("original\n"), []byte("hook\n")
	a := attachmentRecord{Files: []attachmentFile{{Path: name, Before: before, After: after, Mode: 0644}}}
	if err := os.WriteFile(name, after, 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyAttachmentFiles(a, true, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("business edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifyAttachmentFiles(a, true, true) == nil {
		t.Fatal("recovery would overwrite external changes")
	}
	live, _ := os.ReadFile(name)
	if string(live) != "business edit\n" {
		t.Fatal("conflict changed server file")
	}
}
