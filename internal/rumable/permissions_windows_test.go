package rumable

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsPrivateFilesAndLegacyPublicACL(t *testing.T) {
	root := t.TempDir()
	s := testSite()
	s.Nginx = nil
	s.IIS = &IIS{Site: "Test", Application: "/"}
	dir := siteDir(root, s)
	if err := os.MkdirAll(filepath.Join(dir, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	// Model the broad inheritance present before the new CLI, then harden
	// private files without rewriting the existing public directory's DACL.
	if err := setProtectedDACL(dir, "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;WD)"); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(dir, "state.json")
	if err := os.WriteFile(state, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := protectSiteDirectory(root, s); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(state, []byte(`{"replacement":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	ps, err := iisPowerShell(os.Getenv("SystemRoot"), "amd64")
	if err != nil {
		t.Fatal(err)
	}
	// Resolve SIDs, not localized group names. This is a native Windows test,
	// intentionally not described as validated by cross-compilation.
	script := `$ErrorActionPreference='Stop'; $acl=Get-Acl -LiteralPath $env:RUMABLE_ACL_TEST_FILE; if (-not $acl.AreAccessRulesProtected) { throw 'private ACL inherits' }; $ids=@($acl.Access | ForEach-Object { $_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value } | Sort-Object); if (($ids -join ',') -ne 'S-1-5-18,S-1-5-32-544') { throw 'private ACL exposed' }; $public=Get-Acl -LiteralPath $env:RUMABLE_ACL_TEST_PUBLIC; if (-not @($public.Access | Where-Object { $_.IdentityReference.Translate([Security.Principal.SecurityIdentifier]).Value -eq 'S-1-1-0' }).Count) { throw 'legacy public ACL removed' }`
	t.Setenv("RUMABLE_ACL_TEST_FILE", state)
	t.Setenv("RUMABLE_ACL_TEST_PUBLIC", filepath.Join(dir, "public"))
	if _, err = command(ps, "-NoProfile", "-NonInteractive", "-Command", script); err != nil {
		t.Fatal(err)
	}
}
