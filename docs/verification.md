# Verification records

These are dated historical results, rendered in English with repeated procedural detail consolidated. They are not a fresh execution report for the documentation update. Exact original records remain in Git history; referenced temporary/Windows evidence belongs to the historical execution hosts and is not a public download. Reported passes, known failures, unverified environments, immutable artifact provenance, and mock-versus-native boundaries remain distinct.


## 0.1.8 native Windows acceptance

### Result and provenance

On 2026-10-23, the maintainer supplied results from an actual Windows execution: **41 passed, 0 product failures, and 9 unverified out of the original 50 checks**. The result covers the reported static-site scenarios, not every Windows platform, document type, or real ingestion path.

This is an English rendering of maintainer-supplied evidence. The authoring host did not access the remote D: files or rerun Windows. Archive sizes/digests and before/after comparisons below come from that report. Tests used published artifacts, dummy application values, and loopback intake.

Release revision: `802544ef882ba12b46376ce0704e10cec437790d`; tag: `release-oss-v0.1.8`. Installer/ZIP/executable digests and revision matched. Native and 32-bit PowerShell both selected amd64 and native Program Files.

### Covered behavior

| Topic | Reported observation |
| --- | --- |
| One-command entry | Operated the actual page, saved TOML, and ran its generated installer |
| First refresh after gzip cache | A/B had 200 gzip on home/second page before setup; first ordinary second.html refresh afterward returned real 200, one current script, intact business content |
| Pool activation | One recycle for each effective change; old workers exited before verification; identical repeat install/disable did not recycle again |
| Two lifecycle rounds | A/B each completed env update, history rollback, disable/repeat-disable, restore, and disable |
| Upgrade | A new isolated copy upgraded 0.1.7 to 0.1.8 with unchanged config/ID; first activation then idempotency |
| Shared/stopped pools | Correct refusal without managed writes, shared-pool recycle, or pool startup |
| Child/bypass | Dedicated child succeeded; JSON/download/excluded paths bypassed injection; parent was not integrated |
| Permissions/inheritance | Anonymous HTTP could not read private state; INHERIT restored compression inheritance |
| Controlled failure | File-level compression caused missing injection; installer failed and restored, no pending transaction; business override retained |
| Browser/local intake | SDK loaded and executed; 67 RUM POSTs returned mock 204, including view/resource and updated env |

First-refresh 200 is distinct from historical unmodified gzip and conditional reuse failures. Subsequent responses do not replace initial evidence. The original detailed assertions remain authoritative; this summary does not prove all cache layers.

### Controlled fault and limits

At 11:51:50.717 Beijing time, FAULT second.html returned 200 text/html, gzip, length 411, and 0 own / 0 total tags. The installer failed and restored to enabled=false with no transaction, retaining the file-level override. Two recycles represented attempted activation and restoration, so failure handling passed.

FAULTCAP captured actual gzip bytes, supplementary request/response headers, and decoded digest on an independent copy. That supplementary exchange is not the installer's full internal failed-request capture.

A driver handoff error and a replay at the wrong matrix stage were retained as test-process issues, not product failures. CTRL exited after the default 20-minute idle timeout with WAS 5186 evidence; this was not product recycle. No FREB was captured, so internal cache/module ordering remains unknown.

### Nine unverified items

1. Real application ingestion; dummy credentials and mock 204 cannot prove it.
2. FREB and internal cache attribution.
3. Long-request recycle timeout and recovery.
4. Dynamic HTML.
5. Native ARM64.
6. Old Windows.
7. Native 32-bit Windows.
8. Independent br/zstd responses.
9. Complete original network data for the installer's internal failed request.

Server-side OTel Agent acceptance was outside this run. Earlier Linux/PowerShell mocks do not establish these native cases.

### Retained scene

Before/after comparisons found 26 old sites, 158 business files, 235 state files, and 53 relevant configuration nodes unchanged. Old CT was not accessed; no lifecycle commands targeted old IDs.

New resource prefix: Rumable018-Accept-20261023-113012-. Normal pool names append -Pool; SUB has -Sub-Pool.

| Group | Loopback port | Final state |
| --- | --- | --- |
| A | 58809 | Disabled, no transaction |
| B | 58810 | Disabled, no transaction |
| CTRL | 58811 | Never integrated |
| UP | 58881 | Upgrade/lifecycle completed, disabled |
| SUB | 58829 | Child disabled, parent not integrated |
| SHARED | 58812 | Correct refusal, not enabled |
| STOP | 58815 | Correct refusal, pool still stopped |
| FAULT | 58898 | Restored, disabled |
| INHERIT | 63721 | Disabled, original compression inheritance |
| FAULTCAP | 52892 | Restored, disabled |

A used auto-336e87d5f014ef8e0184; other attempts used accept-20261023-113012- plus lowercase group. State root: C:\ProgramData\Rumable. New/old sites, pools, state, and browser caches were retained; helper/browser/intake processes exited.

Original evidence is on the Windows execution host, not at public download URLs:

- Report: `D:\www\CODEX\2026-10-22\windows-rumable-0-1-4-iis\outputs\rumable-0.1.8-20261023-113012\REPORT.md`
- Sanitized ZIP: `D:\www\CODEX\2026-10-22\windows-rumable-0-1-4-iis\outputs\rumable-0.1.8-20261023-113012-evidence.zip`
- Size: 19,902,513 bytes; archive checks reportedly passed without private backups/full browser profiles.
- SHA-256: `bd506748c511b371a4a2bc8d706302e09ba7d014b7c761c401f8b3b6a7aeb29c`

The following sections retain earlier stages and do not retroactively turn historical pending tests into passes.


## 0.1.8 IIS runtime activation and recovery — 2026-10-23, pre-release

The supplied 0.1.7 report had 29 passed, 5 failed, and 5 unverified out of 39. S3 first setup received conditional 304 and restored. CT update/disable returned stale HTML. FAULT restored stored configuration but HTTP still had injection, leaving a journal; a later rollback succeeded with matching location digests. These observations did not identify a specific cache layer or expiry.

RUM gained the dedicated-pool activation previously used by OTel: commit, persist snapshot, verify ownership, recycle, wait for old workers, then verify HTTP. Restoration follows the same runtime sequence even when configuration is already correct. Pool/membership migration and drift checks reject shared/stopped/changed pools without reallocating them. First upgrade activates once; identical later operations are idempotent; combined components recycle once without rereading unchanged credentials.

First 304, stale URL, and residual injection remain immediate failures. Separate private operation/recovery evidence records timestamps/status, missing versus empty headers, decoded wrong-body hashes, and expected/observed managed paths. Native test children now use dedicated pools.

Local Go 1.26.1 test/vet, Linux-container Go 1.20.14 tests, modeled IIS lifecycle/race, eight PowerShell 7.4 mock suites, 36 then-current Node checks, Nginx 1.28.3 integration, and Windows amd64 cross-build passed. Actual Windows cache/recycle/module behavior was pending at this stage; later static coverage is recorded above. Real ingestion and long-request timeout still need separate evidence.


## 0.1.6 XML recovery and browser-cache probes — 2026-10-22

The supplied 0.1.5 report had 36 passed, 9 failed, and 6 unverified out of 51. PowerShell 5.1 evidence showed empty urlCompression adapted as a string, name attributes shadowing DOM Name, and unmodified gzip second.html despite injected identity HTML.

Fixes used XML DOM/node types and normalized defaults while preserving exact-journal/ownership protection. Each URL gained three ordinary browser compressed requests without query/header cache bypass. Wrong content fails nonzero and restores; identity/gzip decode is supported, other encodings explicitly fail. There was no proof of gzip root cause and no global cache/compression intervention.

Fail-before/fix-after regressions, Go 1.26.1/1.20.14 test/vet, five portable PowerShell suites, 32 Node checks, and ARM64 Nginx 1.28.3 RUM/discovery lifecycle passed. Downloads in pre-release discovery used local archive substitutes. Shared assets remained deduplicated per round. Native Windows/old OS/ARM64 and real ingestion were still separate.


## 0.1.5 fixes following native Windows feedback — 2026-10-22

A supplied 42-item 0.1.4 report showed installation/discovery passed but zero IIS injection, ConfigurationElement output before JSON, native commit with nativeCommitted=false, and failed rollback/disable. The exclusion regex contained an empty alternative.

The fix covered bridge stdout, nonempty exclusions, exact RUM-only legacy commit reconstruction, redirected/noninteractive Windows input, private/public ACLs, and Linux pipefail. Unmatched extra configuration stayed rejected. See [recovery](windows-0.1.4-recovery.md).

Go test/vet and archive contracts, 29 Node checks, four PowerShell 7.4/Linux mock suites, and Linux onboarding/discovery smoke passed, including lost commit/recovery responses and drift refusal. Shell download errors 22/60 and installer error 17 propagated nonzero. Native permission/input tests were added and cross-built. Go 1.20.14 test/vet/legacy Windows x86/x64 and modern Windows x86/x64/ARM64 builds passed. Local download substitutes were not public-channel acceptance; native Windows remained separate.


## User-flow review and local Linux demo — 2026-10-22

Baseline 0.1.4/c76054d70d6b48c9aaccfb5ac370f8fe7cdf442e matched the then-read latest manifest. The review identified Linux pipeline exit and Windows NTFS gaps; that stage only changed documentation/demo, not official binaries. See [historical review](review-2026-10-22.md).

Go test/vet, 28 Node checks, 12 shell examples, two actual TOML renders, 11 PowerShell syntax examples, and local links passed. The ARM64 Docker demo actually downloaded published 0.1.4 installer/archive without substitutes, then discovered, verified, installed, attached, and enabled Nginx. Home/checkout each had one tag, no drift/pending transaction, and successful probes.

Chrome actions, requests, errors, navigation, and simulated checkout produced actual local view/resource/action/error/long_task POSTs returning 204. Dummy credentials/local intake did not establish real ingestion. Evidence stayed under ignored .local-demo/. No new version was pushed or published in that review.


Historical evidence references: `status.json`, `probe.log`, `intake-evidence.json`.


## 0.1.4 discovery and native Windows detection — 2026-10-21

Go test/vet/archive checks and 28 Node checks passed. Discovery covered strict formats, actual page-to-Go TOML, unique/ambiguous sites/bindings, no-terminal refusal, stable IDs/paths, HTTPS intake, and IIS compression consent.

Linux discovery and manual onboarding executed generated commands with local download substitutes, exercising first/repeat setup, env updates, rollback, disable, detach, and peer isolation. Chrome verified automatic/manual fields and old-manifest refusal.

Portable Windows tests covered modern/legacy x86/x64/ARM64, WOW64 directories, bridge compilation, discovery forwarding, ZIP/hash/retry/cleanup. The historical 0.1.3 pipeline 319770 stalled at apt dependencies before upload and was cancelled; 0.1.4 moved those contracts to a standalone PowerShell container. Native APIs remained unverified.

Publisher checks covered inline HTML/MIME and the then-current 19 immutable objects plus four latest aliases. Actual publication remained defined by tag pipeline/report/public-download evidence.


## 0.1.2 distribution scope — 2026-10-21

Go archive tests and 27 Node checks rejected extra Go/PowerShell/ASPX/page-test/document/hidden files before any upload, even with valid checksums. Archive local links passed. Acceptance scripts moved to matching source tags.

Injection/lifecycle logic was unchanged. CI was responsible for both compiler matrices and ten archives; this local result was not native Windows or real ingestion acceptance.


## 0.1.1 latest and CI downloads — 2026-10-21

Twenty Node checks covered version-pinned latest commands/installers, immutable-first promotion, old-version refusal, prerelease skipping, failed-promotion retry, and digest-readback refusal. Go/archive/installer contracts and shell/diff checks passed. Nginx onboarding from a latest manifest passed using local download substitutes.

Initial pipeline 319659 timed out at the Go module proxy before packaging/upload. CI then forwarded GOPROXY/GOSUMDB with a reachable mirror and checksum verification. Old tags were retained. Native Windows, real publication, and actual stored telemetry remained independent.


Historical evidence references: `/tmp/rumable-latest-onboarding-smoke.log`.


## 0.1.0 short installers and local candidate — 2026-10-21

Installation moved into versioned install.sh/install.ps1 and page commands resolved the manifest's base. Ten initial Node checks plus real-shell architecture/input/refusal and Go installer contracts passed. Linux onboarding used local script/archive substitutes for setup, preflight retry, idempotency, update/rollback/disable/detach.

Go 1.26.1/1.20.14 built ten local candidate archives marked 0.1.0/uncommitted. At that historical stage, 46 release files passed checksums/manifest and in-memory upload/readback ordering. This was not a pinned tag or real publication.

After the tag helper, 15 Node checks covered five temporary Git/bare-remote groups: dry run, one annotated tag, dirty/version/conflict refusal, rejected-push retry, and push-destination validation. No project remote was pushed. Chrome verified then-current page copy with no Console errors. Native Windows/real storage/Runner/ingestion remained pending.


Historical evidence references: `/tmp/rumable-short-installer-smoke-final.log`, `/tmp/rumable-short-installer-release.log`.


## HTML, IIS recovery, and page integration — 2026-10-21, unpublished source

Go 1.26.1/1.20.14 test/vet passed with HTML token/anchor and retained-IIS-mapping recovery regressions. Generated page TOML passed real Go validation. ARM64 Nginx 1.28.3 smoke covered split opening tags, static/proxy/chunked, encodings/cache/HEAD, lifecycle, and failures.

Chrome /script-head retained the business closing-head string, showed Business script passed, one managed tag, initialized SDK, and no JS errors. Page interactions covered exclusive reporting modes, replay/fractional sampling, invalid rates, IIS fields, fallback storage, and TOML.

Nine then-current Node checks covered protected tag/version/archive/conflict/retry/readback/report secrecy. Ten local archives marked 0.1.0-preview.4/uncommitted and 44 historical release objects passed an in-memory storage check. Neither candidate name nor in-memory upload was a release. A slow image download stopped the local full CI-container attempt. Native Windows/old kernels/real channel and ingestion remained unverified.


Historical evidence references: `/tmp/rumable-onboarding-release-check.log`.


## 0.1.0-preview.3 candidate revalidation — 2026-10-20

The preview3-rc1 archive executables, not separate debug builds, were exercised. Both compilers passed test/vet/SDK/archive checks. Ten archives passed digests/CRC/machine metadata/modes/local links; five Linux packages actually ran init/render/status combinations.

Modern Linux x86 passed Nginx 1.9.4 RUM; legacy x64 passed exact attachment/undo. ARM64 passed Nginx 1.25.3/module 0.1.0 and 1.28.3/module 0.1.2 through Collector 0.163.1/private intake, including shared exporters, components, offline receiver RUM actions, split-file attachment/recovery, and uninstall. A local APT fixture exercised real package tooling without validating remote repositories/other ABIs.

PowerShell 7.4/Linux mock/parse/portable checks passed. Windows artifacts were not run natively. Old kernels/TLS/real ingestion remained unverified. SDK/init did not change; Chrome was not rerun. See [final delivery record](preview-0.1.0-preview.3.md) for immutable provenance/digests.


Historical evidence references: `/tmp/rumable-otel-research/`, `preview3-candidate-build.log`, `preview3-matrix.log`, `preview3-rum194.log`, `preview3-attach194.log`, `preview3-otel125.log`, `preview3-otel128.log`, `preview3-powershell.log`.


## Functional review round two — 2026-10-20, unpublished source

Two deployment/recovery failures were reproduced and fixed: placing load_module too late when http lived in an include, and permanently blocking partial two-file recovery because temporary configuration no longer parsed.

The main module entry now precedes events/http. Applying/restoring attachment states can continue only after input/Before/After byte checks. The real regression restored main first, left otel directives in http, then successfully retried attachment rollback and uninstall.

Go 1.26.1/1.20.14 test/vet/archive, actual Nginx 1.25.3/module 0.1.0/Collector flow with local APT/private intake, Nginx 1.9.4 attachment, and six-platform compilation passed. No Windows runtime, real receiver account, new dist, or repeated Chrome evidence was claimed.


Historical evidence references: `/tmp/rumable-otel-research/review2-before-split-http.log`, `review2-before-restore-retry.log`, `review2-fixed-otel125.log`, `review2-fixed-attach194.log`.


## Functional review — 2026-10-20, unpublished source

Three failures were reproduced before fixing: RUM-only operations depended on an unchanged stopped OTel receiver; disabling RUM reintroduced inherited filters that changed cached initialization bytes; OTel-only enable unexpectedly upgraded the browser SDK.

Fixes preserved independent component checks, an empty dynamic RUM filter on prepared deployments, and saved browser SDK/script/digest/CDN references for OTel-only changes. OTel-only first attachment still did not introduce RUM filters.

Both compiler test/vet/archive, modern race, Nginx 1.9.4/1.28.3 RUM, native 1.25.3/module 0.1.0/Collector/private intake with stopped-Collector RUM operations, and six-platform compilation passed. IIS used actual Go transaction/HTTP code with Windows API mocks. Queued/native spans were not guaranteed lossless while intake was down. No new dist or repeated Chrome run.


Historical evidence references: `/tmp/rumable-otel-research/review-before-rum.log`, `review-before-otel.log`, `review-fixed-rum194.log`, `review-fixed-rum128.log`, `review-fixed-otel125.log`.


## Apache removal — 2026-10-20, unpublished source

Current source was reduced to Nginx/IIS. Apache configuration, rendering, module build/install, attachment, checks, Lua assets, tests, examples, scripts, shared VirtualHost selection, and Lua-name checks were removed without a legacy state branch. Existing preview archives/tags and historical evidence were retained.

Both compiler test/vet/archive, ARM64 Nginx 1.9.4 RUM and 1.9.4/1.28.3 attachment, exact-version offline/local-APT Nginx 1.25.3 module installation, actual private OTLP flow/lifecycle/uninstall, portable IIS mocks, and six-platform compilation passed. Documents, affected shell syntax, and diff checks passed.

Remote repository trust, other ABIs, Windows native execution, old kernels, native TLS, and real ingestion were not established. SDK/init did not change; no repeated browser acceptance or regenerated dist.


Historical evidence references: `/tmp/rumable-otel-research/remove-apache-rum194.log`, `remove-apache-nginx194.log`, `remove-apache-nginx128.log`, `remove-apache-otel.log`, `remove-apache-otel-offline.log`.


## OTel install and uninstall — 2026-10-20, unpublished source

Added install-otel/uninstall without rewriting old preview.2 files. Both compiler tests/vet and release contracts passed.

Archive/ownership checks rejected traversal, Windows aliases, links, duplicates, bad hashes, unknown/modified files, and new directories. Shared/current/rollback/live native references blocked dependency deletion even if state was missing. Incomplete IIS ACL setup blocked enable; confirmation-time state changes were rejected.

ARM64 Nginx 1.25.3/module 0.1.0 passed offline .deb and real apt-cache/apt-get/dpkg-deb through a temporary local repository, native loading, reuse/dry-run, spans, components/shared exporters/recovery/rollback/archive/cleanup. Nginx 1.9.4 OTel was refused before download.

IIS transaction tests used mocked APIs with actual journals/HTTP recovery; preexisting mappings/assets were retained and wrong profiler bitness rejected. Portable PowerShell Restore-Nodes/mock contracts and six-platform builds passed. Windows download/ACL/locked-DLL cleanup, other distributions, native IIS/.NET, and real ingestion remained separate.

Module .deb SHA-256: 7a39abec8ac3b0f3f00f03d26c2264915bb9c48ce1e1a638f41a8407dd8bcf74. Reproduce with INSTALL_OTEL=1, NGINX_OTEL_ARCHIVE, OTELCOL_BINARY, and optional NGINX_APT_FIXTURE=1 using scripts/smoke-otel.sh on matching isolated infrastructure.


## Automatic first attachment — 2026-10-20

Added attach and independent rollback --attachment, retaining manual attachment and ordinary deployment rollback.

Both compiler test/vet/race and file-boundary checks passed, preserving modes/owner/group/xattrs and clearing inherited temporary default ACLs. Symlink/hardlink replacement and external changes were rejected; partial two-file writes could restore.

Nginx 1.9.4/1.28.3 attachment passed static precompression, proxy headers, cache bypass/restoration, cancellation/dry-run, confirmation-time mutation, config/reload/post-commit HTTP failure, repeat no-reload, site isolation, lifecycle, exact undo, modes/backups, and reattach. The 1.9.4 unsafe http filter was refused before backup/write.

Legacy amd64 was exercised under emulation, not an old kernel. Chrome had one initialization/SDK and local counts View 2/Resource 6/Action 1/Error 1, no replay. Native Windows, Brotli, arbitrary modules/layouts, and real ingestion were not implied. See [attachment operations](operations.md#automatic-first-attachment-on-linux).


## Initial passed checks

Historical Go 1.20.14/1.26.8 test/vet and modern race passed on ARM64 Linux. Nginx 1.9.4, 1.14.2, and 1.28.3 actual flows and Node VM initialization/idempotency/failure isolation passed. Chrome 155 loaded SDK 3.3.21 and sent local events. PowerShell Linux parsing was not IIS execution.

An earlier six-package artifact/runtime stage included a local macOS version check; later public matrices explicitly removed macOS. That historical check is not current macOS distribution support.

Actual HTTP covered static/proxy/chunked, output gzip, validators, bypass paths, lifecycle/old resources, forced reload restoration, and drift refusal. Historical image digests:

- nginx:1.28-alpine: sha256:a8b39bd9cf0f83869a2162827a0caf6137ddf759d50a171451b335cecc87d236
- nginx:1.14-alpine: sha256:485b610fefec7ff6c463ced9623314a04ed67e3945b9c08d7e53a47f6d108dc7


## Minimum Nginx and boundary tests

Actual ARM64 Linux runs used Nginx 1.9.4/1.14.2/1.28.3, not old kernels. Schema-v1 images were replaced by an unchanged-source 1.9.4 build using Debian 12/GCC 12 with http_sub/gzip_static. Disabling unused auth_basic avoided obsolete glibc fields; -Wno-error changed warning handling, not server behavior. Old TLS/OpenSSL was untested.

Source: https://nginx.org/download/nginx-1.9.4.tar.gz; SHA-256 479b0c03747ee6b2d4a21046f89b06d178a2881ea80cfef160451325788f2ba8. Local image ID: sha256:0f1d8b0d14d3722471d7b290589d7a1800ec653003f4f509d8f1a35a3979ecb6.

Reproduced inherited http-level sub_filter crashes on SDK/early-503 requests; Nginx fixed that upstream in 1.9.5. Final passing 1.9.4 configuration moved business rules to server/location, used empty dynamic asset/disabled filters, and later gained a hard preflight guard.

Other fixes added independent MIME/Content-Encoding gating, preserved hidden/security-header inheritance without replacement lists, one-year expires for assets, identity/gzip probing, invalid gzip refusal, and an 8 MiB decoded bound. Source precompression failure restored state; Brotli was not tested.

All three versions covered templates/chunks, conditional validators, warm proxy cache, effective request headers/filters, lifecycle/idempotency, resources, three native/HTTP failure stages, drift, and absence of worker crash/critical logs. Chrome minimum-version counts were View 5/Resource 6/Action 3/Error 1/Long Task 8 across five requests. Both compilers/race and legacy amd64 emulation passed. These were local receipts, not real ingestion.


Historical evidence references: `/tmp/rumable-nginx-1.9.4-chrome.png`, `/tmp/rumable-nginx-1.9.4-stats.json`, `/tmp/rumable-nginx-minimum-final.log`, `/tmp/rumable-nginx-1.14-hardened.log`, `/tmp/rumable-nginx-1.28-hardened.log`, `/tmp/rumable-nginx-minimum-legacy-go.log`, `/tmp/rumable-nginx-hardened-go.log`.


## Nginx 1.9.4 automatic guard

The inherited-filter risk became an automatic hard check from one nginx -T stdout dump, respecting include contexts instead of treating included files as roots. Direct/multiline/quoted rules, nested/glob/relative paths, spaces, repeated cross-context references, and false-positive text cases gained regressions.

Unknown/missing/inconsistent/cyclic/deep contexts and unsupported masks fail closed. attachmentReviewed cannot bypass it. Rejected preflight does not write a deployment transaction/history or reload. Rollback-to-enabled and candidate reload also check; disable remains available.

Actual 1.9.4 tests proved refusal without writes/reload, allowed disable after an external unsafe edit, refused enabled rollback, then succeeded after correction. Nginx 1.14.2/1.28.3 did not receive the version-specific restriction. Both compiler test/vet, modern race, and legacy amd64 emulation passed. Chrome was not repeated because browser code did not change.


Historical evidence references: `/tmp/rumable-nginx194-preflight-final.log`, `/tmp/rumable-nginx114-preflight-final.log`, `/tmp/rumable-nginx128-preflight-final.log`, `/tmp/rumable-nginx194-preflight-legacy.log`, `/tmp/rumable-preflight-legacy-go.log`, `/tmp/rumable-preflight-modern-go.log`, `/tmp/rumable-preflight-final-build.log`.


## TOML migration checks

Go 1.20.14/1.26.8 test/vet and modern race passed without raising legacy requirements. Tests covered generated files/refused overwrite, multisite reporting isolation, literal Windows paths, comments/escapes, explicit zero/false, and refusal of unknown/duplicate/mixed-mode/wrong-type/nonfinite values.

Both Nginx versions reran lifecycle/update/reload recovery/drift with TOML. Windows scripts generated TOML and passed parsing only. Browser code was unchanged; prior RUM evidence was retained, with replay separately tested below.


## Chrome and local reporting

Existing Chrome controls opened the loopback fixture, fetched resources, and triggered an expected test error. The DOM had one managed initialization tag and one SDK. Console had only the intentional regression error after deprecated options were corrected.

Direct-collector configuration produced View 4/Resource 6/Action 1/Error 1; gateway configuration produced View 4/Resource 6/Action 2/Error 1. These cumulative fixture counters prove generated event types, not business metrics or real storage. Intake retained type counts rather than raw telemetry bodies.


## Session Replay revalidation

Actual browser checks observed recording=false by default, true after enabling 100% replay, false at zero sampling, and restored recording after rollback. There was one SDK/init, normal fetch interaction, and no unexpected Console warnings/errors.

Local intake received 13 nonempty replay fragments totaling 10,229 compressed bytes across configuration changes/visits. It retained only counts/size, not replay bodies, and did not validate backend decode/playback. Worker CSP allow/refuse/fallback/intersection boundaries passed. Refresh/new-session testing was required; server disable did not terminate already open recording.


## Expanded static SDK configuration

The SDK 3.3.21 public types/bundle were checked for 54 RUM fields plus Visitor ID, excluding deprecated/private/function/RegExp/plugin options. Tests preserved explicit false, mixed trace match tables, Canvas unions, site isolation, numeric/enums/reserved headers, transport exclusivity, and persisted fields.

Generated scripts were run in Node VM for config forwarding, error-only replay, disabled replay, JSON/remote configuration, quoting, duplicate protection, existing-SDK refusal, and failure isolation. Worker checks covered embedded/same-origin/Canvas/remote requirements, fallback policy, ports/host casing, and 200/JS MIME, not implementation correctness.

Chrome confirmed privacy, localStorage, action settings, compression, Canvas options, trace/Visitor ID request headers, and no SDK/CSP/CORS warnings. Local intake decoded ordinary requests and error-triggered replay; final counts were five decoded requests, Error 1, and 14 replay fragments/10,918 bytes. Remote service, cross-origin API CORS, partitioned cookies, browsers/WebSockets/manual APIs/real Canvas playback and Windows remained separate.


## Review-fix revalidation — 2026-10-19

Tightened URL/port/credential characters, actual CSP script/intake paths and cross-origin base rejection, exact recovery target/asset hashes, and case-insensitive IIS snapshot target lookup. Old SDK snapshots were validated for safe identity/assets without rewriting historical bytes or applying new configuration restrictions to disable/recovery.

Both compiler test/vet/race and Nginx 1.14.2/1.28.3 lifecycle passed. PowerShell snapshot tests were Linux helper checks, not native commits. Chrome verified default/replay, privacy/storage/compression/Canvas, trace/Visitor ID, disable, and rollback. Cumulative local counts were View 8/Resource 18/Action 2/Error 1, three decoded ordinary requests, and four replay fragments/4,165 bytes; not real backend acceptance.


## Optimization checks — 2026-10-20

Fixed repeated disable against an initially unreviewed disabled Nginx snapshot: human attachment consent is required for enabling, while disabled operations still check module/includes/native config/ownership. An old binary reproduced the failure before the expanded 1.9.4 regression passed.

Shared script/Worker checks were deduplicated within each probe round without caching HTML/CSP across pages or results across rounds. A then-current two-page/shared-Worker case dropped from 10 to 7 requests; that historical count predates the later gzip-asset expansion and is not a latency benchmark.

Both compiler test/vet/race, modern ARM64 Nginx 1.9.4/1.28.3, and legacy amd64 emulation passed. Generated HTML/SDK/server rules were unchanged; Chrome/native Windows/real ingestion were not repeated.


Historical evidence references: `/tmp/rumable-optimize-before.log`, `/tmp/rumable-optimize-go.log`, `/tmp/rumable-optimize-nginx194.log`, `/tmp/rumable-optimize-nginx128.log`, `/tmp/rumable-optimize-legacy.log`.


## Linux review round two — 2026-10-20

Reproduced HEAD retaining original validators/length on 1.9.4 and stale gzip assets passing identity-only checks. HEAD now activates filter header cleanup without a body, using regex map keys compatible with old bucket defaults. Scripts/SDKs require matching hashes in identity/gzip; Workers require matching decoded content without claiming their implementation/version.

Actual static/proxy/warm-cache HEAD, disabled conditional restoration, and stale gzip SDK/init failure recovery passed, retaining page/bypass/drift/historical SDK checks. Shared assets remain once per encoding per round; the expanded two-page/one-Worker case is ten requests.

Both compilers/test/vet/race, ARM64 1.9.4/1.28.3, and legacy amd64 emulation passed. Chrome local final counts were View 3/Resource 6/Action 3/Error 1 across three requests, no replay. Old kernels, actual TLS/Brotli, arbitrary filters, native Windows, and real ingestion remained untested.


Historical evidence references: `/tmp/rumable-linux-review-chrome.png`, `/tmp/rumable-linux-review-chrome-stats.json`, `/tmp/rumable-linux-review-head-before.log`, `/tmp/rumable-linux-review-gzip-before.log`, `/tmp/rumable-linux-review-go.log`, `/tmp/rumable-linux-review-nginx194.log`, `/tmp/rumable-linux-review-nginx128.log`, `/tmp/rumable-linux-review-legacy.log`.


## Simplified onboarding — 2026-10-20

The flow became init, edit minimal TOML, render/attach, enable. Enable includes preflight/native/activation/HTTP; check is optional. init gained server/reporting/site/full choices with replay and IIS compression consent off.

Single configured/state sites can omit --site; ambiguity fails without mutation. Render prints shell-escaped real paths/commands; ATTACH.md retains compression/inheritance/ACL/cache boundaries.

Both compilers/test/vet/race, 12 real CLI template combinations, strict TOML, single/multisite state behavior, and actual POSIX quoting of spaces/quotes/dollars/command-substitution literals passed. Nginx 1.9.4 modern ARM64 and 1.28.3 legacy amd64 flows passed. Container-side atomic fixture replacement avoided shared-mount partial TOML. This was not new Chrome/Windows/real ingestion evidence.


Historical evidence references: `/tmp/rumable-onboarding-go-final.log`, `/tmp/rumable-onboarding-final-nginx194.log`, `/tmp/rumable-onboarding-final-legacy.log`.


## Attachment, SDK sync, and CDN review — 2026-10-20

Attachment now rejects ambiguous same-line blocks and conflicting global markers/resources/variables, rechecks state/backups/inputs/symlink/file identity after confirmation, exclusively creates backups, and reassesses include ownership before undo. Shared/new references or unmanaged drift stop restoration.

SDK preparation checks a stable v3 package/license/full-bundle identity and records version/source/hash. Offline skip only reuses reviewed bytes. Site-level bundled/CDN delivery validates HTTPS URL, encodings/MIME/hash/CSP and optional SRI/CORS; snapshots retain historical references.

Both compiler tests/vet/race, Nginx 1.9.4/1.28.3 attachment and concurrent-change/refusal/recovery, legacy amd64, manual attachment, and actual CDN switching passed. SDK remained 3.3.21 with SHA-256 67c7d58349e376864f3297e9e6d4a60453ede6ff0c6f6b1900a74c2a44d4d3b1. The CDN lacked CORS then; default SRI was off. Custom SRI had HTTP/VM coverage, not real-browser hosted-CDN acceptance.

Chrome observed one init/SDK, correct default/replay/new-session/disable/rollback behavior, and only expected errors. Final local counts: View 12/Resource 22/Action 7/Error 1/Long Task 1, five replay fragments/3,696 bytes, 17 requests. Windows/old kernels/real ingestion and external CDN file restoration were not established.


Historical evidence references: `/tmp/rumable-cdn-final-go.log`, `/tmp/rumable-cdn-final-nginx194.log`, `/tmp/rumable-cdn-final-legacy.log`, `/tmp/rumable-cdn-final-nginx128-cdn.log`, `/tmp/rumable-cdn-manual-nginx.log`, `/tmp/rumable-cdn-chrome.png`, `/tmp/rumable-cdn-replay-chrome.png`, `/tmp/rumable-cdn-rollback-chrome.png`, `/tmp/rumable-cdn-chrome-stats.json`.


## 0.1.0-preview.1 packaging — 2026-10-20

The public candidate matrix became ten Linux/Windows packages, no macOS: modern 386/amd64/arm64 and legacy 386/amd64 per OS. Modern Go 1.26.8, legacy 1.20.14, CGO=0, GO386=sse2, GOAMD64=v1, GOARM64=v8.0.

Both compiler test/vet/SDK/archive and clean-revision refusal tests passed, plus modern race. SDK stayed 3.3.21. Machine types, binary digests, preview labels, CRC/modes passed. Five Linux binaries ran init/render/status; 386 test suites and x86/x64 binaries used emulation on a modern kernel.

Actual candidate x86/1.9.4 and ARM64/1.28.3-CDN attachment flows passed. Windows Sysnative selection was boundary-tested; PowerShell 7.4/Linux portable behavior was not PS3/5.1/IIS. Final release binaries must match tested candidates byte-for-byte. Native Windows/all architectures/old kernels/real ingestion stayed pending.


Historical evidence references: `/tmp/rumable-preview-candidate-build.log`, `/tmp/rumable-preview-go-preflight.log`, `/tmp/rumable-preview-x86-tests.log`, `/tmp/rumable-preview-race.log`, `/tmp/rumable-preview-nginx-x86.log`, `/tmp/rumable-preview-nginx-arm64-cdn.log`, `/tmp/rumable-preview-powershell-portable.log`.


## Server OTel source acceptance — 2026-10-20

Existing preview.1 dist was not rebuilt. Both compiler test/vet/archive, modern race, Windows cross-build, minimum-Nginx RUM, portable IIS contracts, and ARM64 native Nginx 1.25.3/module 0.1.0 plus 1.28.3/module 0.1.2 passed.

Collector 0.163.1 forwarded actual gRPC spans to a private HTTP/protobuf fixture. Assertions covered known W3C ID/parent/service.name, both transport/token branches, independent controls, 0/100 sampling, paths/peer isolation/shared exporter ownership, reload failure, and exact reverse-order detach.

OTel-only attachment preserved source gzip/strong ETag/Host/Authorization, injected no browser script, and traced API requests. Native load failure restored files; old-module HTTPS was refused before writes. Old-module host:port normalization and fresh-connection/active-generation checks gained regressions.

IIS mocks covered protocol/token escaping/sampling/runtime/bitness/business env/ownership and saved-token boundaries. Native Windows/Agent, old OS, native TLS, real receiver services/accounts/ingestion remained unverified. Chrome combined/default/disabled interaction was local-only.

Module package: nginx-module-otel_1.25.3+0.1.0-1~bookworm_arm64.deb; SHA-256 7a39abec8ac3b0f3f00f03d26c2264915bb9c48ce1e1a638f41a8407dd8bcf74. Local test images were not published product images.


Historical evidence references: `/tmp/rumable-otel-nginx-final.log`, `/tmp/rumable-otel-nginx125-final.log`, `/tmp/rumable-otel-builder.log`, `/tmp/rumable-otel-nginx-native-failure.log`, `/tmp/rumable-otel-nginx-only.log`, `/tmp/rumable-otel-nginx-origin-slash.log`, `/tmp/rumable-otel-go120.log`, `/tmp/rumable-otel-go126.log`, `/tmp/rumable-otel-powershell.log`, `/tmp/rumable-otel-legacy-nginx.log`.


## 0.1.0-preview.2 candidate review — 2026-10-20

Kept preview.1 immutable and rebuilt the ten-package preview with Go 1.26.9/1.20.14.

Fixed detached sites still reserving an exporter, environment-order-only IIS recycles, and early return during Nginx graceful reload. The real old/new worker race led to two successful generation/HTTP rounds 200 ms apart. Existing requests/queued spans still were not instantly cancelled. Existing output directories and dirty final revisions were refused.

preview2-rc3 actual binaries passed both compiler tests/vet/SDK/archive/race, ARM64 native OTel flows, x86 minimum-Nginx RUM, five Linux init/render/status variants, Windows cross-build/machine/hash/CRC/modes, and portable PowerShell checks. SDK stayed 3.3.21 with its pinned digest. x86/x64 used modern-kernel emulation.

SDK/init unchanged meant no repeated Chrome run. Windows native/old kernels/TLS/real ingestion remained unverified. [Final artifact provenance](preview-0.1.0-preview.2.md) separately pins the immutable build.


Historical evidence references: `/tmp/rumable-preview2-rc3-build.log`, `/tmp/rumable-preview2-race.log`, `/tmp/rumable-preview2-otel-nginx125.log`, `/tmp/rumable-preview2-otel-nginx128.log`, `/tmp/rumable-preview2-rum-nginx194.log`, `/tmp/rumable-preview2-linux-matrix.log`, `/tmp/rumable-preview2-review-ps.log`, `/tmp/rumable-preview2-existing-output.log`.


## IIS OTel hardening — 2026-10-20, unpublished source

This stage followed preview.2 without rewriting its tag/files. The historical host had Linux/OrbStack, not Windows.

Fixes explicitly discarded Recycle's ObjectState return, preserved unchanged server credentials during RUM actions, separated configuration commit from snapshot-before-activation, waited up to 15 seconds for original workers without killing them, and rejected machine/service/pool/application instrumentation overrides including unknown OTEL signal exporters.

Rollback rechecks Agent/pool compatibility while restoring saved credentials. ASP.NET Core requires No Managed Code; Windows ARM64 emulated hosts remain rejected. Test scripts gained report-owned cleanup, separate result directories, peer pool/PID assertions, WithRUM/missing-token checks, and actual served-environment generation assertions.

Both compiler tests/vet/archive, modern race, actual Go journals/HTTP with API mocks, PowerShell 7.4/Linux snapshot/contract/parse checks using real Linux subprocess waits, Windows machine/build metadata, and shared Nginx OTel regression passed. Native IIS commit/recycle/worker enumeration/ACL/cleanup, PS5.1, real Agents/TLS/ingestion remained unverified. The non-atomic commit/journal window still stops ambiguous recovery for review.


Historical evidence references: `/tmp/rumable-iis-otel-hardening-go120.log`, `/tmp/rumable-iis-otel-hardening-go126.log`, `/tmp/rumable-iis-otel-hardening-race.log`, `/tmp/rumable-iis-otel-hardening-powershell.log`, `/tmp/rumable-iis-otel-hardening-build.log`, `/tmp/rumable-iis-otel-hardening-pe.log`, `/tmp/rumable-iis-otel-hardening-nginx.log`.


## Remaining environment acceptance

The reported 0.1.8 static Windows scope is recorded first and is no longer described as entirely unexecuted. Remaining independent gaps include old Windows/Server, native ARM64/32-bit, old IIS 7.5/8/8.5, dynamic HTML, long-request recycle timeout, independent br/zstd, internal module/cache attribution, and server-side .NET Agent acceptance.

Old Linux kernels are not validated by modern-kernel containers. Real browser/trace receiver authentication and stored events remain unverified by dummy/local intake. Old browsers, streaming SSR, nonce/Trusted Types, and site-specific policies need explicit acceptance.

Use matching Windows scripts in an isolated loopback test environment; [Windows acceptance](windows-acceptance.md) defines the matrix, ownership cleanup, evidence, and exit codes.


## Expanded Windows acceptance-script checks

Scripts gained three compression baselines, parent/child rule preservation/conflicts, reserved mapping/cache conflicts, idempotency/versioned assets, update/rollback/drift/encoded-source refusal, post-commit HTTP restoration, peer isolation, PreviousBinary, and explicit StaticOnly partial results.

Reports preserve environment/digests/steps/CLI/state/local test configuration. Default cleanup only removes owned sites/pools/location nodes/temp files; evidence remains. Keep cleanup rechecks ownership.

PowerShell 7.4/Linux actually invoked a Go 1.20.14 Linux CLI for UTF-8 TOML, explicit false/zero/replay, state/history, expected failure versus unexpected success, gzip/chunked/503, redirect refusal, and loopback limits. These are helper/portable checks, not Windows PS3/5.1/ASP.NET/MWA/old-IIS native execution.
