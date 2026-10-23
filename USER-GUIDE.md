# Rumable installation and user guide

Follow this sequence: choose an environment, prepare application parameters, install the integration, and verify events. Normal installation does not require Git, Go, Node.js, or a database. The executable remains `rumable` for deployment compatibility.

Download packages from [GitHub Releases](https://github.com/beacon-observability/rumable/releases/latest) and use the [English configuration page](https://beacon-observability.github.io/rumable/latest/setup/index.html). Example application domains and `REPLACE_...` configuration values below are placeholders.

## Contents

- [1. Choose an environment](#1-choose-an-environment)
- [2. Prepare application details](#2-prepare-application-details)
- [3. One-command setup](#3-one-command-setup)
- [4. SDK loading and updates](#4-sdk-loading-and-updates)
- [5. Download and verify the CLI](#5-download-and-verify-the-cli)
- [6. Configure Linux/Nginx manually](#6-configure-linuxnginx-manually)
- [7. Configure Windows/IIS manually](#7-configure-windowsiis-manually)
- [8. Paths, sampling, and replay](#8-paths-sampling-and-replay)
- [9. Verify the integration](#9-verify-the-integration)
- [10. Maintenance](#10-maintenance)
- [11. Troubleshooting](#11-troubleshooting)
- [12. Optional server-side OTel](#12-optional-server-side-otel)

## 1. Choose an environment

| Situation | Approach |
| --- | --- |
| Common website configuration and network access | Use the page-generated installation command |
| Offline installation or step-by-step change review | Download a platform archive, then use the manual flow |
| Custom Nginx startup arguments or verification paths | Use advanced manual targeting or complete TOML |
| Server-side traces without browser RUM | Use the separate OTel flow |

Manual CLI installation still allows automatic Nginx attachment. Edit server configuration yourself only when automatic attachment cannot safely handle the layout.

| Environment | Scope |
| --- | --- |
| Linux/Nginx | Manual setup, site discovery, automatic attachment, and one-command installation |
| Windows/IIS | Manual setup, discovery, and one-command installation |
| Windows/Nginx or macOS/Nginx | Outside the automatic installation and attachment scope |
| Native 32-bit Windows | x86 CLI supports limited commands; IIS management is unavailable |

Nginx RUM needs 1.9.4+ with `http_sub_module`. IIS RUM needs IIS 7.5+, URL Rewrite 2.x, Static Content, and anonymous access to the script resources. The Windows installer requires an administrator, 64-bit Windows, and Windows PowerShell 5.1+. The manual IIS bridge baseline is 64-bit Windows PowerShell 3.0; 5.1 is recommended.

The IIS application must use a started, dedicated pool, including child applications. Enable, update, disable, and rollback can recycle the pool and wait for old workers. In-process sessions may be lost. The CLI does not allocate pools, recycle shared pools, or restart all of IIS.

See [compatibility](docs/compatibility.md) for separate CLI, operating-system, module, Agent, and browser requirements.

## 2. Prepare application details

### Application parameters

Create or select a browser-monitoring application at your receiver and obtain its connection details.

| Field | Meaning |
| --- | --- |
| `applicationId` | Browser-monitoring application ID; required |
| `site` and `clientToken` | Browser gateway origin and browser Client Token |
| `datakitOrigin` | Direct browser collector origin; mutually exclusive with `site` and `clientToken` |
| `service`, `env`, `version` | Logical service, environment, and application version |

The compatible reporting selectors remain `openway` and `datakit`. They identify the existing gateway and direct-collector modes. The `site` field is a reporting address, not the website URL or IIS site name.

Enter only the scheme, host, and optional port, such as `https://intake.example.com`. Do not append API paths, queries, tokens, or fragments. HTTPS websites require HTTPS browser reporting. The endpoint must be reachable from visitors' browsers, not just the server.

Client Tokens enter public browser initialization scripts. Never substitute management API keys or server-side credentials. Placeholder credentials can demonstrate requests but cannot prove real ingestion.

### Website prerequisites

- The CLI can reach the target instance through its public URL with working DNS and TLS.
- Each HTML template and effective handler has a verification path returning an anonymous HTTP 200 `text/html` document. Probes do not follow redirects or send login cookies.
- Documents contain a literal standard `<head>` opening tag. Case variants work; attributes or extra whitespace are outside the supported anchor form. Earlier tag-shaped text can make the anchor ambiguous.
- There is no conflicting integration of the same SDK. Also inspect dynamic application bundles; preflight cannot detect every hidden initialization.
- CSP permits the initialization script, SDK, receiver, and required Workers.
- Source HTML is uncompressed when it reaches the response filter. Nginx output gzip can remain enabled.

Ensure verification traffic reaches the modified instance rather than another load-balancer node or cached HTML.

### Installation locations

| Content | Linux installer default | Windows installer default |
| --- | --- | --- |
| CLI | `/opt/rumable/<version>/rumable` | Native `%ProgramFiles%\Rumable\<version>\rumable.exe` |
| State, backups, versioned assets | `/var/lib/rumable` | `%ProgramData%\Rumable` |
| Operator-maintained TOML | For example, `/etc/rumable/website.toml` | For example, `C:\ProgramData\RumableConfig\website.toml` |

The installer extracts the CLI and deletes its temporary decoded TOML when finished. Save the page-downloaded configuration yourself. Keep the actual version, site ID, URL, executable path, and state directory.

State must remain outside the website root. Do not delete it to reinstall. Continue managing the same site with the same ID and state directory.

## 3. One-command setup

Open the release's `setup/index.html` over HTTP(S), fill in the application details, load its manifest, save the TOML, and execute the generated command on the Web server. The configuration page is in English.

1. Choose browser gateway or direct collector and enter the required parameters.
2. Set service, environment, and application version. For first verification, use 100% session sampling with replay disabled.
3. Choose SDK source and loading mode independently.
4. Choose Linux/Nginx or Windows/IIS. Automatic discovery does not require a website URL in advance.
5. Load the release manifest and confirm CLI and SDK versions. Commands remain unavailable without a valid manifest.
6. Save TOML and copy the complete generated command.

Configuration is serialized locally in the browser. Base64 protects TOML from shell quoting changes; it is not encryption. Copying a command does not modify the server. Executing it authorizes the installation and selected site changes.

### Linux/Nginx

Run inside the actual Nginx host or container namespace. You need root/sudo, Bash, curl, CA certificates, sha256sum, tar, and base64. The installer chooses modern x86, x64, or ARM64 packages. It does not select legacy packages by kernel version; use manual installation for old kernels.

The following illustrates the structure only; use the actual command generated by the page:

~~~sh
bash -o pipefail -c 'curl --proto "=https" --proto-redir "=https" -fsSL "INSTALLER_URL_FROM_PAGE" | sudo sh -s -- --base-url "ARTIFACT_BASE_FROM_PAGE" --config "CONFIG_FROM_PAGE" --discover'
~~~

Bash pipefail propagates both download and installation failures. The installer verifies its pinned archive hash, installs the CLI, discovers sites, and runs setup. A unique complete target is selected automatically; multiple sites or URLs require terminal selection. Wildcard bindings may require an accessible URL.

Discovery reads the Nginx default configuration from PATH or standard binary locations. It does not infer custom `-c` or `-p` service arguments. Containers must persist the actual server configuration and state volumes.

### Windows/IIS

Open administrator Windows PowerShell 5.1, preferably the native 64-bit host. Execute the generated PowerShell command, not the Linux command or a cmd.exe command.

~~~powershell
# Environment checks only.
$PSVersionTable.PSVersion
[Environment]::Is64BitOperatingSystem
[Environment]::Is64BitProcess
~~~

The command has this structure; replace all placeholders with the page-generated values:

~~~powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing 'INSTALLER_URL_FROM_PAGE').Content)) -BaseUrl 'ARTIFACT_BASE_FROM_PAGE' -Config 'CONFIG_FROM_PAGE' -Discover
~~~

The installer detects the real OS version and native x64/ARM64 architecture, including from 32-bit PowerShell. Native Program Files is used. Native 32-bit Windows is rejected before IIS installation.

Discovery lists running HTTP(S) sites and applications. Select the intended target and binding. Allow disabling both static and dynamic compression for the selected application when required, either in the page's advanced options or at the terminal prompt.

The CLI protects private files and grants public-script read access to the actual anonymous and pool identities. It does not broadly reset existing directory ACLs, change anonymous authentication, replace handlers, or adjust application routing exceptions. Never recursively grant IIS or Everyone access to the entire state directory.

For unattended use, `-NonInteractive` prevents waiting on hidden input. A unique complete target can proceed; ambiguity or missing permission fails. This flag does not supply a target choice or compression consent.

### Advanced manual targeting

Enable manual targeting for custom Nginx arguments, redirects, authentication, custom probe paths, or a specific IIS child application.

| Field | Example |
| --- | --- |
| Public URL | `https://www.example.com/` or `https://www.example.com/shop/` |
| Site ID | `website`; keep it stable |
| Included/excluded paths | Case-sensitive URL prefixes, one per line; exclusion wins |
| Probe paths | Paths returning anonymous HTML 200, covering each template |
| Nginx binary/config | Actual binary and effective configuration paths |
| IIS site/application | IIS site name and `/` or `/shop`, not disk paths |

Discovery TOML uses `schemaVersion`, `server`, `[rum]`, and optional `[sdk]`. Apply it only through `setup --discover`. Complete TOML uses `[[sites]]` and can be used by render, check, enable, or setup. Do not mix the formats.

The page command cannot pass an attachment `--target` selection. If multiple Nginx candidates remain or handlers require manual review, use the staged flow in section 6.

### Save deployment details

Record the printed site ID, URL, executable path, state root, and backup location. The installer does not automatically add the CLI to PATH.

~~~sh
rumable=/opt/rumable/0.1.9/rumable
sudo "$rumable" status --state-dir /var/lib/rumable --site REPLACE_SITE_ID
sudo "$rumable" probe --state-dir /var/lib/rumable --site REPLACE_SITE_ID
~~~

~~~powershell
$rumable = Read-Host 'Full path to the installed rumable.exe'
$state = Join-Path $env:ProgramData 'Rumable'
$siteId = 'REPLACE_SITE_ID'
& $rumable status --state-dir $state --site $siteId
& $rumable probe --state-dir $state --site $siteId
~~~

For enabled RUM, expect `rumEnabled=true`, empty `drift`, `pendingTransaction=false`, and a passing probe. Then verify the browser and stored events in section 9. A completed one-command installation does not require repeating the manual installation sections.

## 4. SDK loading and updates

### Injection and loading

Nginx response filtering or IIS outbound rewriting inserts external scripts after the standard `<head>`. Business source files are unchanged. Resource URLs retain `__rumable__/<site-id>/`, with content-hashed SDK and initialization filenames.

| Source/loading | Behavior |
| --- | --- |
| Bundled/asynchronous, the default | Same-origin initialization script asynchronously loads the same-origin SDK |
| Bundled/synchronous | Same-origin SDK and initialization script execute in order before HTML parsing continues |
| CDN/asynchronous | Same-origin initialization script asynchronously loads the CDN SDK |
| CDN/synchronous | CDN SDK executes before the same-origin initialization script; HTML parsing waits |

Asynchronous loading can miss early events. Synchronous downloads and execution can delay first paint, especially with a slow or unavailable CDN. Both modes use external scripts and leave CSP unchanged. A failed CDN request has no automatic local fallback.

### Configuration examples

Start with this complete Linux/Nginx configuration. Replace application details, website, and binary path:

~~~toml
schemaVersion = 1

[[sites]]
id = "website"
publicURL = "https://www.example.com/"
includePaths = ["/"]
excludePaths = ["/downloads/"]
probePaths = ["/"]

[sites.nginx]
binary = "/usr/sbin/nginx"
attachmentReviewed = false

[sites.rum]
applicationId = "REPLACE_APPLICATION_ID"
site = "https://intake.example.com"
clientToken = "REPLACE_CLIENT_TOKEN"
service = "website"
env = "production"
version = "1.0.0"
sessionSampleRate = 100
sessionReplayEnabled = false
defaultPrivacyLevel = "mask"

[sites.sdk]
source = "bundled"
loading = "async"
~~~

The block above is same-origin/asynchronous. For each other combination, replace only its `[sites.sdk]` table; do not append a duplicate table:

~~~toml
# Same-origin/synchronous.
[sites.sdk]
source = "bundled"
loading = "sync"
~~~

~~~toml
# CDN/asynchronous; omitted url uses the source recorded in the package.
[sites.sdk]
source = "cdn"
loading = "async"
integrity = false
~~~

~~~toml
# CDN/synchronous; omitted url uses the source recorded in the package.
[sites.sdk]
source = "cdn"
loading = "sync"
integrity = false
~~~

An optional `url = "https://cdn.example.com/sdk/3.3.21/rum.js"` illustrates a custom immutable CDN, not an existing hosted SDK. Supply your own actual URL serving the exact package SDK bytes. SRI requires CORS and a digest matching the release. With bundled mode, omit `url` and leave integrity false.

For direct collector reporting, remove `site` and `clientToken` and set `datakitOrigin = "https://collector.example.com"` in `[sites.rum]`. For IIS, replace `[sites.nginx]` with `[sites.iis]` as shown in section 7.

### SDK updates and rollback

Bundled SDK files update only when the newer CLI is used to run `setup`, `enable`, or `enable --component rum` on the existing site. Merely replacing the CLI does not change deployed assets. `enable --component otel` preserves the browser SDK, initialization script, and CDN reference.

A mutable CDN can change what visitors receive after deployment. Probe-time hash validation does not freeze future responses. Use bundled assets or immutable CDN URLs for reproducible delivery. Rollback restores saved references and local bytes, not deleted or overwritten external CDN files.

Keep original site IDs, target definitions, state, and old assets. Content-hashed assets remain available for cached HTML. Retest with a new browser session after SDK or sampling changes. The application version field and remote configuration do not upgrade SDK files.

## 5. Download and verify the CLI

Download from [GitHub Releases](https://github.com/beacon-observability/rumable/releases/latest). The fixed 0.1.9 download base is `https://github.com/beacon-observability/rumable/releases/download/v0.1.9/`. Assets include the manifest, SHA256SUMS, installers, and platform archives. The [Pages manifest](https://beacon-observability.github.io/rumable/latest/manifest.json) resolves downloads through an absolute, version-pinned `artifactBase`. See [GitHub releases](docs/github-release.md) for download commands and publication details.

| OS | Architectures | Flavor |
| --- | --- | --- |
| Linux | 386, amd64, arm64 | Modern |
| Windows | 386, amd64, arm64 | Modern |
| Linux and Windows | 386, amd64 | Legacy, built with Go 1.20.14 |

Names are `rumable_<version>_<os>_<arch>`, optionally followed by `_legacy`, and `.tar.gz` for Linux or `.zip` for Windows. See [compatibility](docs/compatibility.md) before choosing a package. Legacy packages lack later Go standard-library security fixes.

For offline installation, download the archive and matching SHA256SUMS on a connected machine, transfer them, and verify the archive on the target. Compare against the trusted release manifest as well as a locally downloaded checksum file.

~~~sh
# Run in the directory containing the archive and SHA256SUMS.
sha256sum -c SHA256SUMS --ignore-missing
~~~

Extract to a new versioned directory, preserve the archive layout, and run `rumable version`. Do not overwrite an existing installation directory. Keep `build.json` and verify version, architecture, compiler, source revision, and binary digest.

On Windows PowerShell 5.1:

~~~powershell
$archive = 'C:\Downloads\REPLACE_PACKAGE.zip'
(Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash
# Compare the result with the trusted release checksum before extracting.
$install = 'C:\Rumable\0.1.9'
if (Test-Path -LiteralPath $install) { throw 'Installation directory already exists' }
Expand-Archive -LiteralPath $archive -DestinationPath $install
& (Join-Path $install 'rumable.exe') version
if ($LASTEXITCODE -ne 0) { throw 'CLI verification failed' }
~~~

Older PowerShell can use `certutil -hashfile PACKAGE.zip SHA256` and Explorer extraction. Normal installation requires neither repository access nor development tools. Automatic acceptance scripts are obtained separately from matching source tags.

## 6. Configure Linux/Nginx manually

Run in the real server namespace with appropriate permissions. Use stable absolute paths:

~~~sh
sudo -i
rumable=/opt/rumable/0.1.9/rumable
config=/etc/rumable/website.toml
state=/var/lib/rumable
site_id=website
install -d -m 0700 /etc/rumable
"$rumable" init --server nginx --reporting openway --site "$site_id" --config "$config"
~~~

`init` refuses to overwrite an existing file. Edit the generated TOML, or adapt [examples/nginx.toml](examples/nginx.toml). Keep the actual binary and startup options, for example `args = ["-c", "/etc/nginx/nginx.conf", "-p", "/etc/nginx/"]`. The binary is Nginx itself, not systemctl or a shell service wrapper.

Proceed only after each command succeeds:

~~~sh
"$rumable" render --config "$config" --state-dir "$state" --site "$site_id"
"$rumable" attach --state-dir "$state" --site "$site_id" --dry-run
"$rumable" attach --state-dir "$state" --site "$site_id"
"$rumable" enable --config "$config" --state-dir "$state" --site "$site_id"
"$rumable" status --state-dir "$state" --site "$site_id"
"$rumable" probe --state-dir "$state" --site "$site_id"
~~~

`render` prepares disabled state and assets. `attach` previews and backs up server changes, checks configuration, reloads, and probes resources. Injection remains disabled until `enable`. A separate `check` is an optional early diagnostic, not a substitute for enable.

When multiple candidates are printed, choose the actual `FILE:LINE` with `attach --target`. Recheck candidate locations after server edits; example line numbers are not stable.

For unsupported layouts, use the generated `ATTACH.md` to include `http.conf`, `server.conf`, and `location.conf` in their actual contexts. Preserve application handlers, root/proxy_pass/try_files, headers, and inheritance. Save your own original backups, test and reload the real configuration, then set `attachmentReviewed=true`.

Manual attachment does not create automatic attachment backups. `rollback --attachment` cannot undo manual server edits. Disable first, address cached HTML, then manually remove the integration and restore reviewed directives. Never delete files still referenced by Nginx includes.

Automatic attachment can change compression, conditional request headers, and caching across the effective handler, including excluded or non-HTML requests. Ordinary disable does not undo every first-attachment adjustment. See [operations](docs/operations.md).

## 7. Configure Windows/IIS manually

Record the IIS site name, application path, pool, and anonymous file identity. A child application `/shop` needs `publicURL = "https://www.example.com/shop/"` and URL paths under `/shop/`. Use a separate started pool for each integrated application.

Use administrator native 64-bit Windows PowerShell and the actual installed CLI:

~~~powershell
$rumable = 'C:\Rumable\0.1.9\rumable.exe'
$config = 'C:\ProgramData\RumableConfig\website.toml'
$state = Join-Path $env:ProgramData 'Rumable'
New-Item -ItemType Directory -Force (Split-Path $config) | Out-Null
& $rumable init --server iis --reporting datakit --site website --config $config
~~~

Edit the generated application and reporting parameters. The server table has this form:

~~~toml
[sites.iis]
site = "Default Web Site"
application = "/"
allowCompressionChanges = true
~~~

Only grant compression-change consent after reviewing its effect on both static and dynamic responses in that application. The original local attributes and inheritance are saved for restoration.

~~~powershell
& $rumable render --config $config --state-dir $state --site website
if ($LASTEXITCODE -ne 0) { throw 'Render failed' }
& $rumable check --config $config --state-dir $state --site website
if ($LASTEXITCODE -ne 0) { throw 'Preflight failed' }
& $rumable enable --config $config --state-dir $state --site website
if ($LASTEXITCODE -ne 0) { throw 'Enable failed; retain recovery evidence' }
& $rumable status --state-dir $state --site website
& $rumable probe --state-dir $state --site website
~~~

IIS does not use the Linux `attach` command. Existing handlers and inbound routing must allow the application's `/__rumable__/` resources to be served anonymously by StaticFile. A virtual directory alone does not override application handlers.

For 401/403, inspect the effective anonymous identity and NTFS traversal/read permissions. For 404, inspect the mapping, routing, and handler selection. For invalid MIME, ensure JavaScript MIME configuration. Keep private state inaccessible to website users.

Updates and recovery verify saved pool ownership. Do not rename a managed pool or add another application to it without first planning a controlled deployment change.

## 8. Paths, sampling, and replay

Path rules are case-sensitive prefixes with exclusion taking priority. They are not extension patterns or JavaScript regular expressions. Keep at least one valid probe path for every selected template and actual HTML handler. SDK resource paths are reserved and excluded automatically.

RUM session sampling and trace sampling are independent. Session Replay defaults to disabled. To opt in:

~~~toml
[sites.rum]
# Keep the existing application and reporting fields.
sessionSampleRate = 100
sessionReplayEnabled = true
sessionReplaySampleRate = 20
sessionReplayOnErrorSampleRate = 0
defaultPrivacyLevel = "mask"
~~~

This table illustrates edits to an existing `[sites.rum]` table, not a standalone configuration. Error-only replay can combine zero normal replay sampling with positive error replay sampling. Replay still depends on RUM session eligibility. New sampling settings should be tested in a new session.

Worker-dependent features need the effective CSP permissions. Self-hosted Workers must use same-origin URLs and match the actual SDK version. Manual views, user identity, business events, callbacks, and Canvas snapshots require application API calls; injection does not supply dynamic business data.

See the [complete field reference](docs/configuration.md) for privacy, storage, tracing, Visitor ID, Canvas 2D, and remote configuration.

## 9. Verify the integration

Verify four separate layers:

1. **Deployment:** correct site, empty drift, no pending transaction, and successful probe.
2. **HTML and assets:** one current managed initialization tag per selected document; correct SDK order for synchronous loading; initialization script and SDK return executable JavaScript with matching hashes.
3. **Browser:** SDK initializes without errors; inspect Console/Network, CORS, TLS, CSP, Workers, interactions, errors, and any replay uploads.
4. **Receiver:** search the intended application for actual stored events, service/environment/version, and replay where enabled.

`window.__RUMABLE__.status` can expose initialization status. A replay status of `requested` only means the recording API was invoked; it does not prove sampling, upload, or playback.

For IIS, preserve a browser with pre-installation cached HTML. Check the first ordinary refresh of the home page and another important page before extra identity requests or manual probes warm caches. Expect an actual 200 and one current initialization script, with business content intact.

A mock receiver returning 204 or a browser returning 2xx is not proof of real ingestion. CLI probes do not execute JavaScript or prove receiver authentication. Keep these results separate.

## 10. Maintenance

Keep the same site ID, server target, and state root. Save TOML before editing. Complete configuration can be reapplied:

~~~sh
rumable enable --config /etc/rumable/website.toml --state-dir /var/lib/rumable --site website --component rum
rumable status --state-dir /var/lib/rumable --site website
rumable probe --state-dir /var/lib/rumable --site website
~~~

Discovery configuration must be reapplied with `setup --discover` or through a newly generated command. An existing deployment reuses its identity and paths. Replacing the CLI alone does not update SDK files.

~~~sh
# Stop new browser injection while preserving server-side OTel.
rumable disable --state-dir /var/lib/rumable --site website --component rum

# Restore the previous successful deployment, or recover a pending transaction.
rumable rollback --state-dir /var/lib/rumable --site website

# Preview full removal of the site's managed integration.
rumable uninstall --state-dir /var/lib/rumable --site website --dry-run
rumable uninstall --state-dir /var/lib/rumable --site website
~~~

Plain `enable` applies both component settings from TOML. Plain `disable` disables both. Single-component operations preserve the deployed settings of the other component. RUM-only operations retain unchanged server credentials; OTel-only operations retain the deployed browser SDK.

Every lifecycle change must account for browser, CDN, proxy, and Service Worker HTML caches. Disable does not stop scripts in already open pages. Content-hashed assets remain for older HTML until full mapping removal.

For automatic Nginx attachment, after disable and cache handling:

~~~sh
rumable rollback --state-dir /var/lib/rumable --site website --attachment --dry-run
rumable rollback --state-dir /var/lib/rumable --site website --attachment
~~~

This restores the first-attachment backup only when ownership and saved bytes still match. It is separate from ordinary deployment rollback. Uninstall archives private state and removes owned mappings; ordinary rollback cannot restore an uninstalled site. Optional `--remove-dependencies` only removes unchanged, unshared managed dependencies. See [dependency management](docs/otel-installation.md).

## 11. Troubleshooting

| Result | Next step |
| --- | --- |
| Missing effective include | Verify actual Nginx contexts and filesystem paths against ATTACH.md |
| No safe literal head | Inspect the real template and standard opening tag; do not substitute a closing tag |
| Existing RUM integration | Review the existing SDK deployment before migrating |
| Encoded response | Resolve upstream or precompressed source HTML |
| CSP blocks a resource | Explicitly update the site's actual policy, then retry |
| Stale bootstrap URL | Preserve evidence; inspect target workers, routing, and the actual HTML cache |
| External change or asset drift | Review the difference before restoring anything |
| Pending transaction | Recover using the same CLI state root and site ID |

If a process was terminated, first confirm it is no longer running, back up state and server configuration, and remove only a confirmed stale `operation.lock`. Then use ordinary `rollback` for a deployment transaction or `rollback --attachment` for an attachment transaction. Never delete transaction records to bypass ownership checks.

IIS failures can leave `last-operation-probe-failure.json` and `last-recovery-probe-failure.json` in private state. Compare timestamps; retained evidence can describe an earlier attempt. IIS configuration commit and local journaling are not one atomic transaction. Ambiguous state stops automatic recovery for review.

For retained 0.1.4/0.1.5 Windows failures, follow the [historical recovery procedure](docs/windows-0.1.4-recovery.md).

## 12. Optional server-side OTel

Server-side tracing is independent of browser RUM. The configuration page and `setup` do not deploy it.

Use [OTel configuration and lifecycle](docs/otel.md), [dependency installation and uninstall](docs/otel-installation.md), and the [Windows acceptance procedure](docs/windows-acceptance.md). Nginx native modules and .NET Agents have stricter requirements than the CLI or RUM injection. Successful compilation or local TCP connectivity does not prove Agent startup, authentication, or stored traces.
