# Changelog

## Unreleased

- Keep Rumable as the public product name and retain the rumable executable, configuration schema, state/resource paths, and deployment markers.
- Publish English documentation, example comments, setup-page copy, generated archive README files, and latest redirect pages.
- Use neutral application/receiver wording and release-relative installation guidance in public documents. No new hosted download channel is announced.
- Keep package-default CDN configuration implicit so the CLI resolves the package-recorded SDK source; custom CDN URLs remain supported.
- Add a public-documentation wording/language/link check to release validation. Runtime SDK bytes, required third-party notices, native dependencies, and collection protocols are unchanged.

## 0.1.5 · 2026-10-22

- Discard IIS collection Add return values that polluted bridge JSON; parse stdout separately from stderr.
- Remove empty excluded-path entries, fixing null discovery lists that excluded all HTML.
- Strictly reconstruct and restore RUM-only legacy transactions committed without a recorded native snapshot. Retain retry support after interrupted recovery and reject mismatched rules, mappings, compression, and OTel.
- Detect redirected Windows input and noninteractive PowerShell; add setup --non-interactive and installer -NonInteractive.
- Protect private Windows files with native DACLs. Grant only public-script reads to actual anonymous/pool identities without recursively exposing state.
- Generate Linux commands with Bash pipefail so download and installer failures return nonzero.
- Extend portable bridge/legacy recovery contracts and provide native Windows permission/input tests; native retest was pending at this historical stage.

## 0.1.4 · 2026-10-21

- Move Windows installer contracts to a standalone official PowerShell container. The 0.1.3 tag pipeline stalled during apt downloads before OSS publication; it was cancelled and its tag retained.
- Simplify the page to RUM details and server selection. Add setup --discover for Nginx/IIS, selecting a unique target or asking through the terminal. Keep manual targeting in advanced settings.
- Repeat discovery reuses site ID/URL/paths and existing locks, preflight, backups, lifecycle, and recovery. Ambiguity without a terminal fails.
- Detect native Windows architecture/version, select modern/legacy packages, and handle native installation directories from 32-bit PowerShell. Reject unsupported OS before downloading.
- Load the page's matching manifest automatically, reject unsupported discovery commands, enable TLS 1.2 for Windows downloads, and publish HTML with correct MIME/inline headers.
- Add discovery and Windows installer contracts. Native Windows/IIS still required separate acceptance.

## 0.1.2 · 2026-10-21

- Remove test source, acceptance scripts, and fixtures from platform archives; retain executable, manuals, examples, and notices.
- Enforce the then-current 19-file OSS allowlist. Even checksum-valid unexpected files block publication.
- Obtain Windows acceptance scripts from the matching source tag and select the release CLI separately. Keep historical 0.1.1 artifacts immutable.

## 0.1.1 · 2026-10-21

- Add stable latest page, manifest, and Linux/Windows installer entries after complete stable publication; old retries cannot move latest backward.
- Add artifactBase so latest discovery resolves immutable selected-version artifacts.
- Use a reachable Go module mirror while keeping checksum-database validation after initial network failures.
- Document main/tag release flow and separate Windows installation acceptance.

## 0.1.0 · 2026-10-21

- Add a static SDK setup page generating TOML for existing reporting modes, sampling, replay, privacy, and tracing, plus checksum-pinned platform installation commands.
- Add RUM-only setup composing initial preparation, Nginx attachment, enable, and recovery. Publish manifest and separate installers.
- Add protected-tag GitLab/OSS publication with Docker, serial immutable uploads, checksum readback, manifest-last publication, and equal-byte retries.
- Add release-oss.sh for clean-source/version/remote checks, dry run, annotated tag creation/push, commit readback, conflict refusal, and retry after rejected push.
- Document public release layout and separate Windows acceptance; successful packaging is not native Windows execution.
- Inject after a standard opening head instead of matching closing-head text that can occur in JavaScript. HTML tokenization ignores fake tags in script/comment/inert-template content.
- Fix IIS retained mapping and repeated interrupted-restore recovery with exact journal-derived ownership checks.

## 0.1.0-preview.3 · 2026-10-20

Local preview with modern Linux/Windows x86/x64/ARM64 packages and legacy x86/x64 packages. Current servers are Nginx and IIS; no macOS archives. Earlier artifacts remain immutable.

- Add install-otel: exact-version Nginx APT extraction and upstream .NET Agent v1.17.0 download/digest/ACL handling, plus offline/HTTPS archives. No server upgrades or global IIS registration.
- Add uninstall with preview, ownership checks, RUM/OTel disable, configuration restore, state archive, and optional unchanged/unshared dependency removal.
- Remove unchanged OTel endpoint reachability from RUM-only lifecycle dependencies and prevent inherited filters from rewriting cached SDK assets after disable.
- Preserve browser SDK, initialization script, and CDN references during OTel-only operations.
- Load modules before http-in-include configuration; continue interrupted multi-file attachment recovery even when temporary native configuration cannot parse.
- Add dedicated-pool isolation, snapshot-before-recycle, old-worker waits, repeated recovery, RUM credential preservation, and expanded Windows acceptance contracts; native Windows was pending.
- Remove Apache implementations, configuration, build/install, attachment, Lua assets, examples, and scripts without a compatibility branch.
- Bundle stable SDK 3.3.21 with replay opt-in and same-origin/CDN configuration. Prepare SDK updates explicitly; deployed sites do not update in the background.

See the [user guide](USER-GUIDE.md), [dependency guide](docs/otel-installation.md), and [verification records](docs/verification.md). Earlier entries below are historical, not current Apache support.

## 0.1.0-preview.2 · 2026-10-20

- Add site-level server OTel for Linux Nginx, then-supported Linux Apache, and IIS/.NET with independent RUM/OTel switches.
- Support existing direct OTLP/gRPC collection and authenticated-gateway delivery through a Linux Collector or direct IIS OTLP/HTTP, keeping server tokens private.
- Reuse preview, backup, and rollback. Confirm active Linux generation in consecutive rounds to avoid graceful-reload worker races.
- Require an isolation-fixed Apache module and IIS 10 dedicated pool. Recycle IIS only when managed environment changes.
- Release shared exporters on detach, block new deployment during incomplete recovery, reject existing release output directories, and recheck source revision.
- Distribute CLI archives without native modules, .NET Agents, or Collector. See [OTel scope](docs/otel.md) and [verification](docs/verification.md).

## 0.1.0-preview.1 · 2026-10-20

Local preview with standalone Linux/Windows executables, TOML, manuals, SDK metadata/digest, and notices. No remote publication or macOS package at that stage.

- Support HTML injection for Nginx 1.9.4+, then-supported Linux Apache 2.4.23+, and IIS 7.5+.
- Add Linux site discovery, attachment diffs/confirmation/backups/recovery/undo and managed-digest drift protection.
- Configure reporting, sampling, collection, privacy, storage, and replay per site; replay defaults off.
- Default to same-origin SDK with optional CDN, pinned URL, and SRI. Review stable v3 SDK before release; bundle 3.3.21.
- Provide modern Windows/Linux x86/x64/ARM64 and legacy old-Windows/Linux x86/x64 packages.
- Use Sysnative to run the 64-bit Windows management bridge from x86 CLI on x64; native 32-bit Windows cannot manage IIS.

Historical Linux/Chrome evidence did not establish native Windows, old-kernel, or real receiver acceptance. See [compatibility](docs/compatibility.md) and [verification](docs/verification.md). Previews do not guarantee arbitrary server layouts, HTML fragments, streaming SSR, or automatic CSP relaxation.

Keep old TOML, ID, target, and state when upgrading compatible deployments. Ordinary rollback restores a saved deployment, not an externally overwritten CDN file.
