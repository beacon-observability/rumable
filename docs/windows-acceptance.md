# Windows and IIS acceptance

The maintainer supplied a native 0.1.8 report with **41 passed, 0 product failures, and 9 unverified out of 50**. It covered selected static-site scenarios, dedicated-pool activation, first ordinary refresh after gzip caching, repeated lifecycle changes, and failure recovery. This host did not rerun that Windows environment. See [verification records](verification.md) for provenance and limits.

Keep old sites/state/browser caches intact and test new copies. First-refresh evidence precedes extra identity/probe requests or cache clearing. Correct rejection with successful restoration can pass failure handling; a later successful response cannot replace the first failure. Real ingestion is separate.

Historical 0.1.7 reported 29 passed, 5 failed, and 5 unverified, including conditional 304, stale update/disable HTML, and failed recovery HTTP confirmation. The later result does not prove a particular cache layer or module order. Retained 0.1.4/0.1.5 failures use the [recovery procedure](windows-0.1.4-recovery.md).

## Test boundaries

RUM tests require IIS 7.5+ and 64-bit Windows PowerShell. Server OTel has a stricter [IIS 10 flow](#7-iis-10-otel-acceptance). x86 CLI on x64 uses Sysnative; native 32-bit Windows cannot manage IIS. ARM64 needs its own native acceptance.

Linux/OrbStack can cross-compile and run PowerShell 7 syntax/serialization/mock contracts. It cannot prove Windows PowerShell 5.1, IIS API commits, NTFS ACLs, pool behavior, or Agent loading. See [OrbStack architecture](https://docs.orbstack.dev/architecture).

The suite creates a random loopback-only site, root pool, and three child applications with dedicated pools; default port is 19182. It does not restart IIS, install components, change existing sites/global compression/delegation, or restore a whole-server backup.

## One-command installer acceptance

Use isolated administrator Windows PowerShell 5.1+, 64-bit Windows, IIS, URL Rewrite 2.x, and a dedicated started pool.

1. Open the actual release setup page. Record version, revision, and SDK. Select test credentials/site/binding, not production, and record compression consent.
2. Save TOML and execute the generated command. It verifies ZIP, installs native CLI, and invokes discovery/setup. Record exit status without exposing tokens.
3. For the first-refresh regression, capture the real browser refresh before any extra probe warms caches. Then check the actual installed CLI/state/ID:

~~~powershell
$nativePrograms = $env:ProgramFiles
if ([Environment]::Is64BitOperatingSystem -and -not [Environment]::Is64BitProcess) {
  $nativePrograms = $env:ProgramW6432
}
$rumable = Join-Path $nativePrograms 'Rumable\0.1.8\rumable.exe'
$state = Join-Path $env:ProgramData 'Rumable'
$site = 'REPLACE_ACTUAL_SITE_ID'
& $rumable version
& $rumable status --state-dir $state --site $site
& $rumable probe --state-dir $state --site $site
~~~

Require rumEnabled=true, empty drift, no pending transaction, one current script[data-rumable], valid script/SDK responses, and intact business content including a closing-head JavaScript string.

4. Repeat setup without duplicate injection. Change env, redeploy, then rollback and verify the original script URL.
5. Disable, check newly requested HTML has no injection, and confirm old assets remain. Rollback restores the preceding enabled state; disable again.
6. Retain unfinished transactions and recover with the same state/site. Never delete state and reinstall blindly.

Independently test native x64 PowerShell, x86 PowerShell on x64, native/emulated ARM64, old-system legacy selection, non-administrator/old-PowerShell/native-32-bit refusal, single/multiple sites, multiple/wildcard bindings, compression refusal/approval, noninteractive ambiguity, and stable ID/path reuse.

Installers/archives do not contain test scripts. Obtain matching source separately. Portable scripts/test-windows-installer.ps1 uses API substitutes for archive/hash/config/retry/cleanup, not native matrix evidence.

Record OS/IIS/PowerShell/CLI version, archive digest, ID, initial/repeat installation, updates/rollback/disable, browser behavior, and separate stored-event evidence. Full suites retain report.json and sanitized cli.log.

## 1. Select environment and archive

| x64 environment | IIS | Flavor | PowerShell acceptance |
| --- | --- | --- | --- |
| Windows 7 SP1 / Server 2008 R2 SP1 | 7.5 | Legacy, actual Go 1.20.14 | Native 64-bit PS 3.0 and separate 5.1 |
| Server 2012 / 2012 R2 | 8.0 / 8.5 | Legacy | System PS and separate 5.1 |
| Windows 10 / Server 2016+ | 10.0 | Modern, Go 1.26.x | Native 64-bit PS 5.1 |
| Legacy binary on modern Windows | 10.0 | Legacy | Supplementary, not old-OS evidence |

Check build.json, executable version, and SHA-256. Old systems reject modern archive metadata before launching when available. Keep complete package layout.

Default PS 2.0 is insufficient. Deploy compatible WMF/.NET beforehand; PowerShell 7 is not a substitute. See [WMF guidance](https://learn.microsoft.com/en-us/powershell/scripting/windows-powershell/wmf-overview).

## 2. Prepare an isolated machine

Full tests require Static Content, static/dynamic compression, anonymous access, management APIs, and URL Rewrite outbound rules. Registered 64-bit ASP.NET 4.x Integrated is needed by fixtures for dynamic/chunked/source-gzip/post-commit failure scenarios, not every product installation.

On a disposable Server 2016+ VM, prepare roles and install URL Rewrite before acceptance:

~~~powershell
Install-WindowsFeature Web-Server,Web-Static-Content,Web-Stat-Compression,Web-Dyn-Compression,Web-Asp-Net45,Web-Mgmt-Console
~~~

Use corresponding Windows Features/Server Manager on other systems. Complete prerequisites and necessary restarts beforehand; do not experiment with role installation on production.

Static gzip tests warm actual files without changing global httpCompression. MIME/min-size/CPU/frequency conditions preventing real gzip must remain failed/partial evidence; a true configuration flag is insufficient. See [compression testing](https://learn.microsoft.com/en-us/iis/extensions/iis-compression/using-iis-compression#testing-iis-compression).

## 3. Run full acceptance

Verify the archive and extract separately from matching source, for example source under C:\Rumable\source. Obtain release-oss-v<version> using maintainer-supplied repository access. Do not guess a clone URL.

Administrator native Windows PowerShell:

~~~powershell
$ps = "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
& $ps -NoLogo -NoProfile -ExecutionPolicy Bypass -File C:\Rumable\source\scripts\windows-smoke.ps1 -Binary C:\Rumable\modern\rumable.exe -ExpectedIISVersion 10.0 -ResultDir C:\RumableResults\iis10-modern-01
$LASTEXITCODE
~~~

For IIS 7.5/8/8.5, use a real legacy binary and ExpectedIISVersion 7.5/8.0/8.5. ResultDir must not exist. Reopen native PowerShell or use Sysnative from a 32-bit process; tests reject x86 hosts and PS 7.

Add -PreviousBinary with a schema/CLI-compatible old binary for actual binary upgrade/rollback. Without it, current-binary configuration updates are still tested. Do not launch modern previous binaries on old Windows.

-StaticOnly skips dynamic/chunked/source-gzip/post-commit HTTP recovery and always yields partial, exit 2. It cannot replace full acceptance.

## 4. Assertions and limits

| Scenario | Assertion |
| --- | --- |
| Compression | Inherited/local true/local false, actual gzip, disabled attributes while active, original attributes/inheritance afterward |
| Consent | Reject required compression changes without permission |
| Inheritance | Preserve parent/child business header rules and unrelated parent edits; reject owned namespace conflicts |
| Native conflicts | Refuse rewriteBeforeCache and reserved mapping conflicts without replacing originals |
| HTML/bypass | One tag for static/dynamic HTML; real chunked transfer; unchanged excluded/no-anchor/JSON; downloads bypass |
| Assets/headers | Same-origin scripts/metadata/notices reachable and hashed; no original strong ETag; old assets survive rollback/disable |
| Idempotency/update | Repeated enable leaves history stable; env/replay update changes script URL; rollback restores it |
| Drift | Refuse disable/rollback after native/resource edits while preserving the changes |
| Encoded source | Reject app-generated gzip without changing middleware |
| Post-commit failure | Fixture 200 before commit/503 afterward, actual second request, restored baseline without drift/pending journal |
| Isolation/cleanup | Each application baseline and parent unchanged; only report-owned resources removed |

Post-commit HTTP failure is not every possible API exception, power loss, or concurrent administrator mutation. ARR/proxies, nonce CSP, browser variants, replay playback, and real ingestion need separate validation.

Origin configuration changes do not invalidate all browser/CDN/Service Worker HTML. Keep diagnostic caches before intervention. A conditional 304 reusing old HTML is not current-injection proof.

## 5. Reports and cleanup

~~~powershell
$report = Get-Content -LiteralPath C:\RumableResults\iis10-modern-01\report.json -Raw | ConvertFrom-Json
$report.result
$report.environment
$report.steps | Format-Table name,result,elapsedMs -AutoSize
$report.cleanup
~~~

Exit 0 means suite assertions/cleanup passed or explicit Keep; 1 means environment/assertion/evidence/cleanup failure; 2 means partial. Expected CLI refusals appear in logs without becoming suite failures; unexpected success fails.

Reports contain OS/service-pack, IIS/Rewrite, PS/CLR, binary versions/digests, and steps. deployment preserves private state/resources; test-site-locations.xml only contains this test's configuration.

-Keep retains the scene. Successful applications end disabled; failures can leave intermediate state. From the original administrator account:

~~~powershell
& $ps -NoLogo -NoProfile -ExecutionPolicy Bypass -File C:\Rumable\source\scripts\windows-cleanup.ps1 -Report C:\RumableResults\iis10-modern-01\report.json
~~~

Cleanup verifies random names, account TEMP paths, physical mappings, and other pool references. It refuses ambiguous ownership and can be repeated. Confirm no test/CLI process remains after termination/restart. Do not restore whole-server backups or run iisreset.

## 6. Browser and real receiver acceptance

Automated RUM fixtures use 0% session/telemetry sampling and a nonfunctional local receiver. Replay settings test lifecycle/hash changes, not recording/upload.

Use a separate application with real compatible receiver details, 100% session sampling, and desired replay. Check Console/Network, one initialization, actions/errors, Workers/uploads, and stored events/playback. Passing an old Windows server test does not prove old-browser SDK support.

## 7. IIS 10 OTel acceptance

Use preview.3 or later CLI and matching source scripts. RUM tests do not establish server tracing.

Prepare Windows 10/Server 2016+, IIS 10, native administrator PS 5.1, ASP.NET 4.x Integrated, .NET Framework 4.6.2+, and a verified Agent matching pool bitness. Reject conflicting machine/WAS/W3SVC instrumentation. Prepare actual gRPC intake or securely inject the gateway token into the administrator process. WithRUM additionally needs URL Rewrite/static resources.

Source-built test binary:

~~~powershell
$env:CGO_ENABLED='0'
go build -buildvcs=false -o C:\RumableTest\rumable.exe .\cmd\rumable
~~~

Prefer the verified release binary for release acceptance. Each ResultDir is new; default loopback ports are 19780/19781.

~~~powershell
$ps = "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
$agent = 'C:\Program Files\OpenTelemetry .NET AutoInstrumentation'
& $ps -NoProfile -ExecutionPolicy Bypass -File C:\Rumable\source\scripts\windows-otel-smoke.ps1 -Binary C:\RumableTest\rumable.exe -AgentPath $agent -Backend datakit -Endpoint http://127.0.0.1:4317 -ResultDir C:\RumableResults\otel-x64-01

# x86 pool and combined browser/server lifecycle.
& $ps -NoProfile -ExecutionPolicy Bypass -File C:\Rumable\source\scripts\windows-otel-smoke.ps1 -Binary C:\RumableTest\rumable.exe -AgentPath $agent -X86 -WithRUM -Backend datakit -Endpoint http://127.0.0.1:4317 -ResultDir C:\RumableResults\otel-x86-rum-01

# Set RUMABLE_DATAWAY_TOKEN securely in this process first.
& $ps -NoProfile -ExecutionPolicy Bypass -File C:\Rumable\source\scripts\windows-otel-smoke.ps1 -Binary C:\RumableTest\rumable.exe -AgentPath $agent -WithRUM -Backend dataway -Endpoint https://gateway.example.com/otel -ResultDir C:\RumableResults\otel-gateway-01
~~~

Assertions cover dedicated/peer pool isolation, unchanged peer service/PID, shared-pool refusal, idempotency, independent controls, preserved credentials, sampler 0/100 conversion, actual worker recovery after post-commit 503, signal-specific drift refusal, saved-token rollback, and report-owned cleanup/ACLs. Sampler settings are not statistical or ingestion evidence.

~~~powershell
$report = Get-Content C:\RumableResults\otel-gateway-01\report.json -Raw | ConvertFrom-Json
$report.result
$report.steps | Format-Table name,result,time -AutoSize
$report.cleanup
$report.traceId # Find at the receiver and verify parentage and service.name.
~~~

This fixture is .NET Framework; ASP.NET Core No Managed Code/hosting/Agent loading needs separate tests. Default OTel-only runs do not cover WithRUM. Reports do not retain raw tokens.

Keep leaves two test resource groups. Load scripts/windows-iis-helpers.ps1 and use Remove-TestDeployment for report site/peer under the original administrator identity after verifying ownership.

## Compression, cold-page, and conditional-GET regression

Test inherited/local true/false for both compression attributes. Confirm both are disabled while active and independently restored.

second.html intentionally stays outside probePaths. Warm gzip and preserve actual ETag/Last-Modified before installation. After enable, send separate/combined old validators before unconditional/repeat-enable/identity requests can mask the first result.

Keep a browser with pre-installation session/cache. Capture first ordinary refresh of / and /second.html using CDP/NetLog, status, and body. Distinguish unmodified 200 gzip from later 304 reuse. Require first-refresh 200 with one current tag; do not clear cache to avoid the regression.

Check inbound-rule/allowedServerVariables snapshot/update/rollback/disable/drift, dynamic/chunked pages, JSON/download bypass, and retained old failures independently. The 0.1.8 report establishes only the static coverage and limits recorded in [verification](verification.md).
