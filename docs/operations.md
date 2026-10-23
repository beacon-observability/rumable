# Deployment, caching, and recovery

This guide describes browser RUM operations. Native dependencies, instance/pool sharing, Collector configuration, and separate OTel lifecycle controls are described in the [OTel guide](otel.md) and [dependency guide](otel-installation.md). OTel-only first attachment does not add RUM compression, cache, or request-header changes.

SDK source and loading mode are independent; see [SDK delivery](configuration.md#sdk-delivery). Reapply setup/enable to change either. Synchronous probes verify SDK-before-initialization order, no async/defer, and correct SRI. Disable removes both injected tags; snapshots retain the original mode.

## First deployment

1. Obtain the browser application ID and reporting parameters. Use a browser Client Token, not a management key.
2. Identify the actual website, HTML handlers, path prefixes, and at least one accessible probe URL for each template/handler.
3. Generate TOML using `init --server nginx|iis --reporting openway|datakit`. Edit the actual server and reporting values. Use init --full or the [full example](../examples/full.toml) for advanced static options.
4. Render into the final stable state directory. The CLI and server must see generated paths in the same filesystem namespace. Run inside the container or use matching mounts.
5. On Linux, preview and confirm attach; unsupported layouts use ATTACH.md and operator-managed backups. On IIS, prepare URL Rewrite, administrator permissions, anonymous static access, ACLs, and JavaScript MIME.
6. Enable. The operation includes preflight, native configuration validation, activation, and HTTP probes. Check is an optional earlier diagnostic.
7. Open the website, inspect script initialization and browser CORS/TLS requests, then confirm stored events at the receiver.

Paths are URL prefixes, not extension patterns. IIS matching is case-sensitive like Nginx. The application's `/__rumable__/` namespace is reserved.

## Automatic first attachment on Linux

~~~sh
rumable render
rumable attach --dry-run
rumable attach
rumable enable
~~~

Dry run is read-only and creates neither business configuration nor backups. Attach displays changes and requires yes; `--yes` is explicit noninteractive confirmation. Repeated attachment verifies ownership without duplicate includes or reloads.

Use the service's real binary, arguments, permissions, and environment. The CLI calls Nginx -V/-T and matches publicURL's exact hostname and port. HTTP/HTTPS default to 80/443. Ambiguous names, bindings, defaults, or multiple IPs require a printed `FILE:LINE` through `--target`. A target choice never bypasses safety checks. Multiple blocks on one line must be separated first.

The tool places marked includes into the effective http, target server, and existing HTML handlers. It does not create competing business locations. Effective inherited proxy_set_header lists are copied where needed, preserving Host, Authorization, custom headers, and variables. Existing local lists keep their inheritance boundary.

Target proxy handlers clear upstream Accept-Encoding, If-None-Match, and If-Modified-Since and disable proxy_cache. Available precompressed-file modules are disabled for that handler. These first-attachment changes can affect non-HTML/excluded requests in the same handler and remain after ordinary disable until attachment is undone. Output gzip may remain enabled.

Common static/proxy_pass handlers, nested locations, and simple includes can be merged automatically. Existing sub_filter, local validator directives, if/rewrite, precompressed try_files, FastCGI/uWSGI/SCGI/gRPC, SSI, or appended bodies require manual review. Nginx 1.9.4 http-level sub_filter is rejected before writes.

The complete effective configuration is checked for conflicting site markers, asset paths, and variable names. A different state root cannot take over an existing deployment.

Symlinks are resolved while preserving the link itself. Shared includes, hardlinks, and ambiguous file ownership are rejected. Before confirmation, show per-file diffs and backup locations. After confirmation, reread configuration and check every input digest, deployment state, backup, symlink target, file identity, and metadata.

Original bytes, mode, and differences are saved privately to `sites/<id>/attachment.json` before atomic replacement. Linux ownership, group, modes, and readable extended attributes including ACL/SELinux metadata are retained. Existing different historical backups are not overwritten.

Use one state root and serialize all configuration maintenance. External tools must not continue writing during attachment. There is no cross-process atomic guarantee against another administrator ignoring the maintenance window.

Candidate configuration must pass native validation, graceful reload, and probes for the original page, initialization script, and selected SDK in identity/gzip variants. Initial injection remains off until enable. Configuration/reload/HTTP failure attempts to restore all changed files and verify again.

Interrupted or incomplete restoration retains applying/restoring records and blocks ordinary operations. Use `rollback --attachment`. Partial multi-file restoration can continue despite a temporarily unparseable nginx -T when unmanaged inputs are unchanged and managed files match saved Before/After bytes. Ownership, include sharing, and unmanaged-input changes can stop recovery for review.

~~~sh
rumable disable
# Address old browser, CDN, proxy, and Service Worker HTML caches first.
rumable rollback --attachment --dry-run
rumable rollback --attachment
~~~

Undo removes asset mappings, so old HTML must no longer need them. If business configuration differs from saved post-attachment bytes, including later changes for another site, restoration stops. Review differences, undo attachments in reverse order where appropriate, or restore selected directives manually.

Ordinary lifecycle operations change managed deployment files and validate markers; they do not rewrite business configuration. Completed attachment rollback retains backups and versioned assets. Reattachment archives the old record as attachment.<timestamp>.json. Backups contain original server configuration; keep state outside the document root under administrator control.

## Compression and caching

### Nginx

The filter needs uncompressed source HTML; Nginx may compress output afterward. Proxy requests must clear source compression and conditional validators while preserving effective business request headers. Static .gz/.br files must not bypass filtering. Only use gzip_static/brotli_static directives when their modules exist.

Injection is gated on actual HTML MIME and source Content-Encoding, even with inherited sub_filter_types *. Keep business sub_filter directives at the same level as the generated location include where required. Disabled RUM retains an empty dynamic matching rule so enabling/disabling does not accidentally change filter inheritance or rewrite cached SDK assets. OTel-only attachment does not require http_sub_module.

Preflight checks identity HTML and CSP. Post-commit and standalone probes additionally request/decode gzip. An unmodified gzip response fails enable and restores managed state. gzip_static always can fail preflight itself. Decoded bodies remain limited to 8 MiB; Brotli precompressed content must be disabled on injection paths.

Each page is checked for identity/gzip injection generation and CSP. Shared initialization scripts, SDKs, and Workers are checked once per encoding per probe round, not cached across rounds. Scripts/SDKs must match saved hashes after decoding. Workers require executable MIME and identical identity/gzip decoded bytes, which does not prove implementation/version correctness. Stale .gz files and compression-cache differences remain operator-owned and can block deployment.

For Nginx 1.9.4, unsafe inherited http-level sub_filter is detected through one effective nginx -T include graph. It is not bypassed by attachmentReviewed. Unknown/unsupported include contexts fail closed. See [compatibility](compatibility.md#nginx-194-guard). A preflight rejection does not create a deployment transaction or reload. Recovery that already wrote a candidate retains its journal if the guard fails; correct external configuration before retrying.

Eligible HTML GET and HEAD activate filter-related cleanup of original Content-Length, ETag, and Last-Modified; HEAD has no body. Regex method maps support old bucket defaults. No new proxy_hide_header list changes inherited hidden headers. Disable restores original validator behavior, subject to business filters. Strong original ETags on rewritten pages are rejected.

### IIS

Outbound rewriting requires uncompressed source HTML. With explicit consent, both doStaticCompression and doDynamicCompression are disabled for the selected application. Original values and whether attributes were locally present are saved separately. Disable/uninstall restores original values or removes introduced attributes to restore inheritance. Other sites and backend middleware are not changed; file-level overrides can still cause failure.

Each managed application, including a child, needs its own started pool. Lifecycle changes recycle that pool and wait for old workers before HTTP verification; in-process sessions may be lost. Shared/stopped pools and changed ownership are rejected. There is no automatic pool allocation, forced worker termination, or iisreset.

Before first change, old snapshots without dynamic-compression ownership capture the effective local baseline into the transaction. Missing historical fields are not interpreted as false. Rollback to an enabled historical version restores old SDK/configuration while retaining required compression protection; final disable restores the original baseline.

After commit, journal persistence, and pool activation, probes first check ordinary refreshes with If-None-Match, If-Modified-Since, and both headers; then unconditional browser, gzip, and identity variants. A first 304, missing/duplicate tag, stale script, or residual injection fails immediately. Later success cannot replace initial failure. ETag * and a future date exercise old representations.

A managed inbound rule clears both conditional request headers for included, non-excluded GET paths whose Accept includes text/html. It runs before business inbound rules without changing the URL or stopping subsequent rules. SDK assets, excluded paths, other methods, and API requests not accepting HTML bypass it.

Response MIME is not known during inbound processing: JSON/download navigation on the same paths can lose conditional-GET optimization while outbound MIME/download checks still prevent injection. Add such paths to excludePaths when required.

Two application-scoped allowedServerVariables permissions are added with original local items/inheritance recorded. Disable/uninstall removes only introduced permissions. Old snapshots capture the actual baseline before mutation. Locked settings, local remove conflicts, ownership drift, and later business overrides can stop operation; actual conditional HTTP verification must pass.

Checks cover probePaths, often only the home page in automatic mode. They cannot clear fresh browser cache, Service Workers, or outer CDN caches that make no origin request. Preserve a pre-installation browser and verify its first ordinary refresh on home and another important page. Require actual 200 responses and one current tag with intact business content. Preserve failure evidence before changing the responsible cache policy.

Private `last-operation-probe-failure.json` and `last-recovery-probe-failure.json` retain the latest failure for each phase, including after successful automatic restoration. They contain timestamps, status, selected cache headers, decoded wrong-body SHA-256, and expected/observed managed script paths; they exclude bodies, cookies, and arbitrary third-party script URLs. Check timestamps before treating old evidence as current failure.

### Shared lifecycle considerations

Hashed same-origin assets have one-year caching. Asset locations avoid added header lists that would change old Nginx inheritance and bypass inherited business body filters. Old files remain for cached HTML.

Enable, update, disable, and rollback do not invalidate HTML already held by browsers, proxies, CDNs, or Service Workers. Follow actual cache policy and verify in a browser. Already open pages can keep reporting after disable. Nginx historical snapshots restore exact old bytes; re-enable with the new CLI for new generated-header fixes. IIS enabled snapshots follow its compression and conditional-request migration rules.

## CSP and reporting

- The effective script-src-elem/script-src/default-src must permit each actual initialization and SDK URL. CDN loading needs its real script URL.
- Path permissions ending in / act as directory prefixes; others match complete paths. Check both scripts independently. See [CSP path matching](https://www.w3.org/TR/CSP3/#match-paths).
- connect-src needs the receiver origin and, when path-restricted, /v1/write/rum, optional /v1/write/rum/replay, and optional /v1/env_variable.
- Multiple CSP policies intersect; adding a permissive header does not override a restrictive one.
- Cross-origin HTML base URLs redirect root-relative resources and are rejected. Same-origin base is allowed; business tags are not rewritten.
- Nonce-only, strict-dynamic, Trusted Types, or runtime policies require site-specific review. Preflight does not prove all possible CSP configurations.
- Browsers connect directly to the receiver. Server access to an internal collector is insufficient. Validate actual browser preflight and POST exchanges.
- Replay defaults off. Its requested status only proves the API call, not sampling, upload, or playback.
- Blob Workers conservatively require explicit blob:. Self-hosted Workers need correct same-origin URLs, executable MIME, and matching SDK implementation.
- Remote settings can override static values. Manual views, users, business context/events, and manual Canvas APIs require application code. The tool does not extract login identity from page content or cookies.

## Transactions and interrupted recovery

Deployment operations use a state-root lock, saved snapshots, hashes, native ownership checks, and a private transaction journal. A failed configuration/reload/activation/probe attempts to restore the previous managed state. External edits stop automatic overwrite.

Before restoring, verify snapshot assets and the same site/target identity. Historical resource bytes are restored without applying newly tightened SDK-field validation to old snapshots; new configuration must satisfy current validation.

If a process was terminated:

1. Confirm no matching CLI process is running and back up private state and actual server configuration.
2. Remove operation.lock only after confirming it belongs to the exited process.
3. Use the same state root and site ID with rollback; use rollback --attachment for applying/restoring first attachment.
4. If external drift, an ambiguous native commit, or recovery HTTP failure remains, retain the journal and review the actual differences.

IIS commits configuration, persists a snapshot, then separately verifies ownership and recycles the pool. Combined RUM/OTel changes recycle once. A first upgrade from older RUM state activates once; identical later operations are idempotent. Wait for old workers up to 15 seconds; timeout or a stopped pool fails without killing business processes.

Restoration also reactivates and waits, even when configuration bytes already match. Incomplete runtime recovery must not be treated as successful solely because stored values are correct. Configuration commit and local journaling are not distributed atomic transactions; ambiguous ownership stops for manual review.

## Common results

| Result | Meaning/action |
| --- | --- |
| missing effective Nginx include | Check actual handler contexts and paths in ATTACH.md |
| no safe literal head | Use a standard opening head without attributes or extra whitespace; cover every template |
| existing RUM integration found | Keep the existing integration or plan an explicit migration |
| encoded response | Resolve source compression |
| CSP blocks | Review and explicitly change the actual site policy |
| stale bootstrap URL | Preserve evidence, inspect active workers/routing/cache |
| external change / asset drift | Review unmanaged edits before proceeding |
| injection and script availability only | Browser execution and actual receiver ingestion are still separate |

Use an isolated VM for [Windows acceptance](windows-acceptance.md). Test cleanup operates only on report-owned resources, not a whole-server configuration restore.
