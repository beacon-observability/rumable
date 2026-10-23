# Recover retained Windows 0.1.4/0.1.5 failures

Historical procedure dated 2026-10-22. Version 0.1.6 introduced these recovery fixes; later releases add IIS lifecycle changes. Old artifacts remain immutable. Use this procedure for preserved test deployments with a matching fixed binary. Recover a pending transaction before retrying setup.

The 0.1.4 native report passed download/hash/architecture/discovery but failed injection and recovery. The 0.1.5 report passed initial injection but still failed retained-state recovery, lifecycle changes, and some compressed browser pages.

## Original failures and fixes

1. ConfigurationElementCollection.Add emitted objects before bridge JSON although IIS had already committed. Return values are now discarded; stderr remains separate.
2. Null exclusion lists produced an empty regex alternative excluding all HTML. Only nonempty paths are used; reserved assets remain excluded.
3. Legacy nativeCommitted=false journals without after.native can be verified against exact journal-derived targets, script URL, paths, original compression, historical faulty rules, and retained mappings. The verified snapshot is saved before restoration. Extra edits, unknown OTel, or uninstall transactions are not adopted.
4. Actual Windows stdin console checks and installer noninteractive forwarding prevent hidden CONIN$ waits. Explicit --non-interactive/-NonInteractive is supported.
5. Private state, journals, bridge requests, and temporary writes use protected Administrators/SYSTEM DACLs. New directories avoid broad inheritance; existing directories are not reset wholesale. Public scripts grant only required anonymous/pool reads and parent traversal, reject reparse points, and retain permissions for cached HTML.

Opening CONIN$ does not prove standard input is interactive. See [Microsoft console handles](https://learn.microsoft.com/en-us/windows/console/console-handles).

## Additional 0.1.6 fixes and limits

XML DOM handling reads empty urlCompression elements and actual rule/preCondition node types when name attributes shadow PowerShell's adapted Name. Default attributes are normalized. Nonempty snapshots restore while wrong tags and external rules remain rejected.

Each URL adds three browser-style requests accepting gzip, deflate, br, zstd without random queries or cache-bypass headers. Missing/duplicate injection, old scripts, or disabled-state residue fails and attempts restoration. Only identity/gzip is decoded; other returned encodings explicitly fail acceptance.

Bounded probes do not prove every cache or future request. Historical evidence did not identify which layer supplied unmodified gzip despite application doStaticCompression=false. The probe correction did not clear global caches, change whole-server compression, restart IIS, or establish the gzip root cause.

## Recover the retained state first

Use administrator 64-bit Windows PowerShell 5.1. Obtain the fixed archive and SHA256SUMS from the actual release base, verify, and extract into a new directory. Keep old binaries. See [manual download](../USER-GUIDE.md#5-download-and-verify-the-cli).

Do not overwrite the original version, render again, or delete state.json, transaction.json, or operation.lock. Copy state/journal/report into a private evidence directory. Replace the example paths and ID with the original deployment details:

~~~powershell
$cli = 'C:\Rumable\0.1.6\rumable.exe'
$state = 'C:\ProgramData\Rumable'
$id = 'REPLACE_ORIGINAL_SITE_ID'
& $cli version
if ($LASTEXITCODE -ne 0) { throw 'Fixed CLI cannot run' }
& $cli status --state-dir $state --site $id
& $cli rollback --state-dir $state --site $id
if ($LASTEXITCODE -ne 0) { throw 'Recovery incomplete; retain state and original error' }
& $cli status --state-dir $state --site $id
& $cli probe --state-dir $state --site $id
if ($LASTEXITCODE -ne 0) { throw 'Recovered HTTP state did not verify' }
~~~

Require no pending transaction and empty drift. Recovery returns to the state before the failed operation, not always disabled:

| Retained case | First recovery result |
| --- | --- |
| 0.1.4 A failed first setup | Disabled, original static-compression attribute, retained public mapping |
| 0.1.5 C failed v2-to-v1 rollback | Clear failed transaction and restore v2; verify before a separate history rollback |
| 0.1.5 D disable committed but snapshot read failed | Restore the previously enabled baseline; verify before disabling again |

Do not repeatedly run rollback without checking whether it is transaction recovery or history rollback. If ownership differs, retain the journal and compare actual configuration.

## Retest without losing evidence

1. Preserve old sites, state, and browser caches; use a new isolated copy.
2. Require one current tag and intact HTML, including JavaScript containing a closing-head literal.
3. Record real browser Accept-Encoding, Content-Encoding, SDK execution, and local POST receipt. Mock 204 is not actual application ingestion.
4. If unmodified gzip recurs, capture raw headers/body and effective per-URL configuration before intervention. Inspect application/file compression, caching/modules, server httpCompression, web.config inheritance, and external proxies/CDNs. Root settings do not represent every URL.
5. Do not clear caches, delete rules/logs, globally disable compression, or restart IIS to turn failed evidence into a pass. Correct refusal and recovery can pass failure handling while injection remains unverified.

## Isolated automated checks

Users do not need Git. Developers obtain matching release-oss-v0.1.6 source scripts and select the separately verified executable:

~~~powershell
.\scripts\windows-smoke.ps1 -Binary 'C:\Rumable\0.1.6\rumable.exe' -StaticOnly -Keep
~~~

Random loopback-only resources are separate from retained sites. StaticOnly returns 2 for partial; remove it only with ASP.NET fixture prerequisites. Use report-owned windows-cleanup.ps1 afterward. The suite does not install IIS/URL Rewrite or run iisreset.

Native file-permission/input checks require matching source, Go 1.26.x, and administrator Windows:

~~~powershell
go test ./internal/rumable -run 'TestWindowsPrivateFilesAndLegacyPublicACL|TestDiscoveryRejectsRedirectedWindowsInput' -v
if ($LASTEXITCODE -ne 0) { throw 'Native Windows checks failed' }
~~~

Linux Go/Node, portable PowerShell mocks, and cross-compilation do not establish NTFS, Windows PowerShell 5.1, IIS, or Agent execution. See [current acceptance](windows-acceptance.md) and [dated evidence](verification.md).
