# Configuration page and one-command setup

See the [user guide](../USER-GUIDE.md#3-one-command-setup) for operator steps. This page describes static-page integration, discovery, manifest contracts, and publishing. The product is Rumable; executable/schema/deployment identifiers remain compatible with rumable.

## Page and release entry points

The page lives in web/ and needs no build dependencies. Public copy is English. It can be served as static files or its generator module can be integrated into an existing console. Serve it over HTTP(S), not file://.

Use the actual published release base supplied by the operator. Relative paths are:

| Relative path | Purpose |
| --- | --- |
| latest/setup/index.html | Entry redirecting to the current stable version page |
| latest/manifest.json | Version, SDK metadata, digests, and artifactBase |
| latest/install.sh | Linux installer |
| latest/install.ps1 | Windows installer |
| <version>/setup/index.html | Immutable release page |
| <version>/manifest.json | Immutable manifest |

This documentation does not announce a new hosted endpoint. Do not substitute a guessed product domain. Package-default SDK delivery remains selected by omission of an explicit CDN URL; custom CDN URLs must serve identical package SDK bytes.

## Current flow

1. Enter browser RUM details, SDK source/loading, and Linux/Nginx or Windows/IIS. Website fields stay in advanced manual targeting.
2. Load a matching immutable/latest manifest. A hosted version page discovers ../manifest.json; a console can pass ?manifest= with the actual URL.
3. Generate TOML and a short installation command locally.
4. The versioned installer detects architecture/flavor, downloads the selected archive, checks its compiled-in SHA-256, installs the CLI, decodes temporary TOML, and runs setup --discover --yes. Manual targeting omits --discover.
5. Discovery selects a unique complete target or asks through the terminal. setup rechecks the selected target and composes render, Linux attachment, and enable under one operation lock.
6. Existing sites reuse their saved ID/URL/paths/state. Changed identity, unmanaged edits, or unfinished transactions stop deployment.

Base64 preserves configuration through shell quoting and is not encryption. Browser Client Tokens are public SDK credentials, not management keys. Config is not uploaded by the page. The installer deletes temporary TOML; operators must save the downloaded copy.

The page and setup handle RUM only. They do not install Nginx/IIS/URL Rewrite or deploy OTel Agents. Linux needs http_sub_module. IIS needs administrator access, URL Rewrite, static resources, anonymous read permissions, a dedicated started pool, and compression consent.

## Command structure

The actual page command pins a selected immutable version, even when discovered through latest:

~~~sh
bash -o pipefail -c 'curl --proto "=https" --proto-redir "=https" -fsSL "INSTALLER_URL_FROM_PAGE" | sudo sh -s -- --base-url "ARTIFACT_BASE_FROM_PAGE" --config "BASE64_CONFIG_FROM_PAGE" --discover'
~~~

~~~powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing 'INSTALLER_URL_FROM_PAGE').Content)) -BaseUrl 'ARTIFACT_BASE_FROM_PAGE' -Config 'BASE64_CONFIG_FROM_PAGE' -Discover
~~~

These are structural examples, not ready-to-run commands. Bash pipefail propagates download and installation failures. Windows requires administrator Windows PowerShell 5.1+, detects native x64/ARM64 and the real OS version, and selects modern/legacy. A 32-bit host on 64-bit Windows still uses native Program Files. Native 32-bit Windows is rejected before IIS installation.

Installers remain in /opt/rumable/<version> or native Program Files\Rumable\<version>; state remains /var/lib/rumable or ProgramData\Rumable. Normal installation extracts only the CLI; platform archives hold manuals/examples/notices. Automatic acceptance scripts remain in matching source tags.

Resolve artifactBase against the manifest URL before resolving installer/package file names. Older immutable manifests without artifactBase use their own directory. A later latest update must not change an already generated command's selected package. The downloaded installer is HTTPS; archive hashes are pinned inside it.

## Discovery and advanced targeting

- Nginx discovery uses PATH/standard binaries and default effective includes, not guessed custom service startup arguments. Use complete TOML for custom -c/-p, non-home verification, or unsupported layouts.
- IIS enumeration is read-only and covers running HTTP(S) sites/applications. Binding host/port/application define the URL. Wildcard bindings require actual input instead of invented localhost.
- --yes confirms changes, not an ambiguous site/address or missing compression permission. Terminal input does not consume a Linux download pipe.
- The Windows installer detects redirected/noninteractive input and passes --non-interactive. Explicit -NonInteractive/--non-interactive is supported. A unique complete target can proceed; ambiguity fails.
- New auto-... IDs are stable. Repeated setup preserves previous site IDs, URL ranges, and state even when Nginx line numbers move. The selected target is rechecked after terminal input.
- IIS compression consent covers static and dynamic attributes in the selected application. Original snapshots/inheritance support disable/rollback.
- Automatic probes usually cover the home page only. Redirects, authentication, fragment HTML, CSP/Workers, and multiple templates can require manual probe paths.
- Discovery TOML goes to setup --discover. Complete [[sites]] configuration goes to render/check/enable or manual setup.

## SDK and release compatibility

| Selection | Discovery format | Complete format | Manifest requirement |
| --- | --- | --- | --- |
| Bundled/async | sdk table can be omitted | sites.sdk can be omitted | Original setup capabilities |
| CDN/async | [sdk] source, optional url, integrity | [sites.sdk] equivalents | Discovery additionally needs rum-discovery-sdk-v1 |
| Bundled/CDN with sync | [sdk] loading=sync | [sites.sdk] loading=sync | rum-sdk-loading-v1; discovery also needs rum-discovery-sdk-v1 |

Default CDN delivery omits url and lets the CLI resolve the release-recorded source. An explicit custom HTTPS URL is retained in TOML. The page never renames protocol fields or turns arbitrary third-party SDKs into compatible packages.

Changing only a static page cannot add CLI support. Old manifests block incompatible command generation but still permit configuration download. Async loading omits loading for compatibility with older CDN-capable complete configurations. Source, loading, URL, and integrity are forwarded to discovery-selected sites.

sdk.version describes the package, not future bytes at a mutable CDN. Reading latest chooses an installation version; it neither upgrades deployed sites nor schedules updates.

## Field mapping

| Page concept | Compatible field |
| --- | --- |
| Application ID | applicationId |
| Browser gateway | site + clientToken; reporting selector openway |
| Direct collector | datakitOrigin; reporting selector datakit |
| Service/environment/application version | service, env, version |
| Session/error compensation | sessionSampleRate, sessionOnErrorSampleRate |
| Replay | sessionReplayEnabled plus both replay rates |
| Interaction and action privacy | trackUserInteractions, actionNameAttribute, enablePrivacyForActionName |
| Privacy/storage | defaultPrivacyLevel, sessionPersistence |
| Frames/compression/remote settings | enableLongAnimationFrame, compressIntakeRequests, remoteConfiguration |
| Cookie/subdomains/fallback | useSecureSessionCookie, trackSessionAcrossSubdomains, allowFallbackToLocalStorage |
| Worker and trace propagation | workerUrl, allowedTracingUrls, traceType, tracingSampleRate |

Advanced static options remain available through complete TOML. setUser, manual startView, callbacks, and function/RegExp matchers require application code. Manual view mode is not exposed in the page because a zero-code install cannot supply missing startView calls.

## CI and static hosting

scripts/release.go emits manifest schemaVersion/version/revision, SDK metadata, capabilities, installers, and package file/digest records. The manifest itself enters SHA256SUMS. Licenses stay in platform archives.

~~~text
rumable/
  <version>/
    install.sh
    install.ps1
    manifest.json
    SHA256SUMS
    sdk.json
    rumable_<version>_linux_amd64.tar.gz
    rumable_<version>_windows_amd64.zip
    ...other platform archives
    setup/
      index.html
      app.mjs
      generator.mjs
      i18n.mjs
      styles.css
~~~

See [GitLab/OSS release](gitlab-ci-release.md) for protected tags, credentials, immutable retries, the exact allowlist, and latest promotion. Source documents and test files are not public standalone release objects.

Upload and read back the complete immutable file set before publishing manifest. Promote stable latest only afterward, with no-store headers and manifest last. Redirect latest to the matching version page to avoid mixed HTML/JS/CSS. Older retries cannot roll latest backward; prereleases do not update it.

Use correct HTML MIME and inline disposition on an actual static-site domain. Some raw object-storage domains force HTML download despite document tags. Configure hosting/CORS externally. Cross-origin pages need permission to GET manifest; the publisher does not change ACL/CORS/domain/CDN settings.

The page disables command copying until a valid manifest loads. It does not fabricate success for nonexistent installers.

## Failure recovery and verification

Setup uses existing transactions and backup ownership. Failed restoration retains transaction.json and blocks new setup until rollback. Attachment recovery is separate. Assets remain for cached HTML until reviewed detach/uninstall.

The page does not relax CSP or guarantee arbitrary HTML. Every selected template should be represented in probePaths. Verify deployment, browser execution, and stored events separately.

~~~sh
python3 -m http.server 18872 --bind 127.0.0.1 --directory web
node --test web/generator_test.mjs web/i18n_test.mjs
node scripts/check-docs.mjs
go test ./...
bash scripts/smoke-onboarding.sh
bash scripts/smoke-discovery.sh
~~~

Smoke scripts use temporary containers and local download substitutes before release, executing real validation/install/attachment/lifecycle paths. To test actual published downloads, set RUMABLE_PUBLIC_MANIFEST to the operator-supplied real versioned URL. These tests do not modify host /opt, /var/lib, or Nginx.

Portable Windows installer tests are in scripts/test-windows-installer.ps1. Native API, permissions, and runtime behavior require a separate Windows environment.
