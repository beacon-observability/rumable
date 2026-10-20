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
	s := testSite()
	s.Nginx = nil
	s.Apache = &Apache{Binary: "httpd"}
	name := filepath.Join(t.TempDir(), "httpd.conf")
	for _, filter := range []string{"RUMABLE_test", "RUMABLE_other"} {
		b := []byte("<VirtualHost *:443>\nServerName example.com\n</VirtualHost>\n<VirtualHost *:81>\nLuaOutputFilter " + filter + " /other.lua inject\n</VirtualHost>\n")
		if err := os.WriteFile(name, b, 0644); err != nil {
			t.Fatal(err)
		}
		tree, err := apacheAttachmentTree(name, b)
		if err != nil {
			t.Fatal(err)
		}
		c := &attachConfig{root: tree, files: map[string][]byte{name: b}, uses: map[string]int{name: 1}}
		_, _, err = c.planApache("/state", s, "", map[string]bool{name + ":1": true})
		if (err == nil) != (filter == "RUMABLE_other") {
			t.Fatalf("namespace isolation %s: %v", filter, err)
		}
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
	if err := verifyAttachmentSources(a); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("external edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if verifyAttachmentSources(a) == nil {
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
	if verifyAttachmentSources(a) == nil {
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
func TestAttachmentApacheActiveVhostAndBoundedEdits(t *testing.T) {
	name := filepath.Join(t.TempDir(), "apache.conf")
	source := `FilterDeclare GZIP CONTENT_SET
FilterProvider GZIP DEFLATE "%{CONTENT_TYPE} =~ m|^text/html|"
FilterChain +GZIP
<VirtualHost *:443>
 ServerName example.com
 ProxyPass / http://backend/
 <Location "/existing">
  Header set X-Business "preserved"
 </Location>
</VirtualHost>
<VirtualHost *:81>
 ServerName other.test
 FilterDeclare GZIP CONTENT_SET
 SetOutputFilter DEFLATE
</VirtualHost>
`
	if err := os.WriteFile(name, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	tree, err := apacheAttachmentTree(name, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	c := &attachConfig{root: tree, files: map[string][]byte{name: []byte(source)}, uses: map[string]int{name: 1}}
	s := testSite()
	s.Nginx = nil
	s.Apache = &Apache{Binary: "httpd"}
	active := map[string]bool{}
	for _, n := range nodeDirect(tree, "VirtualHost") {
		active[n.key()] = true
	}
	files, _, err := c.planApache("/state", s, "", active)
	if err != nil {
		t.Fatal(err)
	}
	result := string(files[0].After)
	if strings.Index(result, `ProxyPass "/__rumable__/test/" !`) > strings.Index(result, "ProxyPass / http:") {
		t.Fatal("proxy exclusion came too late")
	}
	if strings.Index(result, "/apache.conf\"") < strings.Index(result, `Header set X-Business`) {
		t.Fatal("managed Location rules came before business rules")
	}
	if !strings.HasSuffix(result, source[strings.Index(source, "<VirtualHost *:81>"):]) {
		t.Fatal("other vhost changed")
	}
	// Without an explicit stage this DEFLATE provider defaults to RESOURCE on
	// old Apache, compressing before injection and defeating the body filter.
	tree.children = tree.children[1:]
	if _, _, err = c.planApache("/state", s, "", active); err == nil {
		t.Fatal("untyped compression chain was accepted")
	}
	if _, _, err = c.planApache("/state", s, "", map[string]bool{}); err == nil {
		t.Fatal("inactive vhost selected")
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

func TestApacheAttachmentKeepsNativeIncludeContexts(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "httpd.conf")
	target := filepath.Join(dir, "target.conf")
	other := filepath.Join(dir, "other.conf")
	sources := map[string]string{
		main:   "<VirtualHost *:443>\n Include " + apacheQuote(target) + "\n</VirtualHost>\n<VirtualHost *:444>\n ServerName other.test\n Include " + apacheQuote(other) + "\n</VirtualHost>\n",
		target: "ServerName example.com\n",
		other:  "SetOutputFilter DEFLATE\n",
	}
	for name, text := range sources {
		if err := os.WriteFile(name, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	dump := []byte("Included configuration files:\n  (*) " + main + "\n    (2) " + target + "\n    (6) " + other + "\n")
	c, aliases, err := apacheAttachmentConfig(dump)
	if err != nil {
		t.Fatal(err)
	}
	s := testSite()
	s.Nginx = nil
	s.Apache = &Apache{Binary: "httpd"}
	files, selected, err := c.planApache("/state", s, "", map[string]bool{aliases[main] + ":1": true, aliases[main] + ":4": true})
	if err != nil {
		t.Fatal("other vhost's included filter was treated as global", err)
	}
	if selected != aliases[main]+":1" || len(files) != 1 || files[0].Path != aliases[main] {
		t.Fatal("included ServerName did not select its owning vhost")
	}
	// A future native dump format must not silently lose inheritance context.
	if _, _, err := apacheAttachmentConfig([]byte(strings.Replace(string(dump), "    (6)", "    (99)", 1))); err == nil {
		t.Fatal("unresolved include context accepted")
	}
}
