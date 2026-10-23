# TOML configuration reference

Use the [user guide](../USER-GUIDE.md) for installation, [SDK loading](../USER-GUIDE.md#4-sdk-loading-and-updates) for delivery choices, and [operations](operations.md) for caching and recovery. Public product documentation uses Rumable; the compatible command and configuration schema remain unchanged.

## Configuration formats

Complete configuration uses `[[sites]]`, `[sites.rum]`, and optional `[sites.sdk]` / `[sites.otel]`. Discovery configuration uses top-level `server`, `[rum]`, and optional `[sdk]` and is applied with `setup --discover`, not directly with enable. Unknown fields are rejected; synchronous loading requires a compatible release.

Omitted `rum.enabled` means true. Explicit false removes the requirement for browser credentials. Omitted `otel` preserves RUM-only behavior. `enable/disable --component rum|otel` preserves the other component's deployed settings; an OTel-only operation also preserves its browser SDK and CDN reference. Plain enable applies both TOML component settings; plain disable disables both.

For IIS, RUM-only commands preserve deployed OTel credentials. Apply server-token rotation with ordinary enable or `enable --component otel`. Rollback restores saved credentials and still checks target compatibility. ASP.NET Core requires `runtime="dotnet"` and a No Managed Code pool; .NET Framework requires `runtime="netframework"` and v4 Integrated. The CLI does not change CLR settings.

`rumable init` produces a minimal configuration. `init --full` and [full.toml](../examples/full.toml) expose all supported static SDK options. `--server nginx|iis` selects the server; `--reporting openway|datakit` selects gateway or direct-collector reporting. Defaults are nginx/openway. Replay and IIS compression consent default to off.

The default new site ID is example; override it with `init --site website`. Render/check/enable can omit `--site` only for a single configured site. State-based commands can omit it only for a single managed site. Keep explicit config, state, and site selectors for automation.

## Server identity

Each site requires exactly one `[sites.nginx]` or `[sites.iis]` table. The ID, public URL, server type, and actual server target/arguments bind deployment identity. Use a new ID for target migration. Sampling, SDK options, and path ranges can be updated with enable.

IIS applications, including children, need dedicated started pools. Lifecycle changes can recycle the pool and affect in-process sessions. Saved pool ownership and membership are checked; external changes are not silently adopted.

Both server types use the same browser fields below. The current bundled baseline is SDK 3.3.21. Supported fields are non-deprecated public initialization options expressible as static TOML. Callback, plugin, and dynamic runtime APIs are outside that boundary. SDK bytes and metadata are reviewed and committed before release; tag CI uses those pinned bytes.

## SDK delivery

`[sites.sdk]` is a sibling of `[sites.rum]`. Discovery uses `[sdk]`.

| Field | Default | Meaning |
| --- | --- | --- |
| `source` | `"bundled"` | Same-origin bundled SDK; `"cdn"` selects external loading |
| `loading` | `"async"` | Asynchronous; `"sync"` loads and executes SDK and initialization scripts in order, blocking HTML parsing |
| `url` | Omitted | CDN only; omission uses the package-recorded source. An explicit URL must be a full HTTPS script URL without credentials, query, or fragment |
| `integrity` | false | CDN only; enables the package SHA-256 SRI value and anonymous cross-origin mode |

~~~toml
[sites.sdk]
source = "cdn"
loading = "async"
# Omit url to retain the package-default source.
# A custom URL must serve the same SDK bytes as the package.
integrity = false
~~~

Check, enable, and probe validate CDN identity/gzip content, JavaScript MIME, and the package digest. A custom CDN must serve identical bytes. SRI requires CORS allowing the website origin or `*`. CSP must allow the same-origin initialization script, actual SDK URL, receiver, and required Workers; policies are never rewritten automatically.

A mutable package-default CDN is not an immutable version pin. A successful hash check only describes the bytes observed at probe time. For reproducibility, choose bundled assets or an immutable HTTPS CDN URL with SRI. Workers must match the SDK used by the browser.

Snapshots retain the resolved URL, SDK version, digest, and loading mode. Rollback cannot restore deleted or overwritten CDN files. If the old external resource is unavailable, rollback probing can fail and restore the attempted managed change. Disable does not require CDN availability. There is no automatic fallback to the local SDK copy.

Asynchronous mode uses an async same-origin initialization script that loads the SDK and initializes it on load. Synchronous mode injects SDK then initialization script without async/defer; failures are recorded as load-error or init-error. Both modes use a standard opening `<head>`, external scripts, and ASCII-compatible HTML. Nginx injection targets GET/200 HTML; IIS targets GET/HTML, excluding downloads, assets, and encoded source bodies.

Avoid duplicate SDK integrations and double injection by stacked servers. Strict nonce, strict-dynamic, and Trusted Types policies need site-specific integration. Same-origin SDK delivery does not make the reporting endpoint same-origin.

## Validation and defaults

Unknown fields, wrong types, invalid enums, NaN/Infinity, invalid URLs, and out-of-range values fail before deployment. Explicit false and zero values are preserved. Defaults below describe tool defaults or the pinned SDK, as indicated.

Browser reporting endpoints are HTTP(S) origins with optional trailing slash, no paths, queries, fragments, or credentials; explicit ports must be 1–65535. SDK 3.3.21 concatenates applicationId/clientToken into query parameters, so whitespace, control characters, and `& # + % ;` are rejected. Do not pre-encode those values. Ordinary labels are JSON-escaped.

## Application, sampling, and collection

| Field | Type/default | Meaning |
| --- | --- | --- |
| `enabled` | Boolean, true | Enable browser RUM |
| `applicationId` | Required string when enabled | Browser application ID |
| `site`, `clientToken` | Strings | Browser gateway origin and browser Client Token |
| `datakitOrigin` | String | Direct collector origin; mutually exclusive with gateway fields |
| `env`, `service`, `version` | Strings | Environment, service, and business application version |
| `sessionSampleRate` | Number, 100 | RUM session sampling, 0–100 |
| `sessionOnErrorSampleRate` | Number, 0 | Error compensation for otherwise unsampled sessions, 0–100 |
| `sessionReplayEnabled` | Boolean, false | Tool-level replay switch; off forces both replay rates to zero and skips the recording API |
| `sessionReplaySampleRate` | Number, 100 | Normal replay sampling, 0–100; effective only with replay enabled |
| `sessionReplayOnErrorSampleRate` | Number, 0 | Error replay sampling, 0–100; can be combined with zero normal replay |
| `telemetrySampleRate` | Number, SDK default 100 | SDK diagnostic telemetry, independent of business sessions |
| `trackUserInteractions` | Boolean, true | Automatic user interactions |
| `actionNameAttribute` | Optional string | HTML attribute supplying action names |
| `enablePrivacyForActionName` | Boolean, false | Privacy rules for automatic action names; explicit names follow SDK rules |
| `defaultPrivacyLevel` | allow / mask / mask-user-input; mask | Privacy processing for replay and affected data |
| `trackViewsManually` | Boolean, false | Requires application `startView()` calls |
| `enableLongAnimationFrame` | Boolean, false | Browser-dependent Long Animation Frame support |
| `enableExperimentalFeatures` | String array, empty | Only `"track_websockets"` is accepted by this SDK |
| `excludedActivityUrls` | URL string array, empty | Exact URL activity matching for loading-time calculation; not a resource collection denylist |

Replay depends on RUM session sampling. Zero normal and error-compensation session rates cannot be reversed by replay sampling alone. Error replay may buffer before an error; recording status alone is not evidence of upload. Verify the resulting replay at the receiver.

With replay enabled and a positive replay channel, the initialization script requests recording once after init. When remote configuration is enabled, it may request recording even with both local replay rates zero, leaving final sampling to the SDK. Sampling decisions persist for the session; verify changes in a new session.

## Session and local storage

| Field | Type/default | Meaning |
| --- | --- | --- |
| `sessionPersistence` | cookie / local-storage; cookie | Session persistence, consistent with other SDKs on the page |
| `useSecureSessionCookie` | Boolean, false | Requires an HTTPS website |
| `usePartitionedCrossSiteSessionCookie` | Boolean, false | Requires HTTPS and browser support |
| `trackSessionAcrossSubdomains` | Boolean, false | Shared cookie session across subdomains |
| `allowFallbackToLocalStorage` | Boolean, false | Cookie fallback only when persistence is not explicitly selected |
| `storeContextsToLocal` | Boolean, false | Persist common context |
| `storeContextsKey` | Optional string | Context key; SDK-generated when omitted |

Cookie options affect cookie mode. Local storage is origin-scoped. Remove an explicitly configured sessionPersistence field when using automatic fallback.

## Transport and Workers

| Field | Type/default | Meaning |
| --- | --- | --- |
| `compressIntakeRequests` | Boolean, false | Ordinary RUM request compression using a Worker; separate from replay compression |
| `sendContentTypeByJson` | Boolean, false | JSON Content-Type; receiver must allow corresponding CORS requests |
| `workerUrl` | Optional URL | Self-hosted replay/transport Worker |
| `replayCanvasWorkerUrl` | Optional URL | Canvas encoder Worker; does not replace workerUrl |
| `remoteConfiguration` | Boolean, false | Receiver-supported remote settings may override static SDK settings |
| `silentMultipleInit` | Boolean, false | SDK duplicate-init diagnostics; does not disable deployment conflict protection |

Workers use root-relative paths such as `/workers/rum.js` or absolute HTTP(S) URLs matching publicURL's origin. Host files matching SDK 3.3.21 yourself. The CLI does not download a new Worker or replace business files. Checks verify active Workers return 200 and JavaScript MIME, not their internal implementation.

compressIntakeRequests and sendContentTypeByJson cannot both be true: the pinned SDK bypasses ordinary compression in JSON mode. Replay compression is separate.

Omitted URLs use embedded Blob Workers. Compression, replay, Canvas, and remote configuration can require Workers. CSP is checked through worker-src, child-src, script-src, then default-src. Blob Workers conservatively require explicit `blob:`. A self-hosted Worker can use the effective same-origin permission. Remote settings may enable compression, so Worker preflight remains necessary even when local compression is off.

## Trace context and Visitor ID

| Field | Type/default | Meaning |
| --- | --- | --- |
| `allowedTracingUrls` | Array, empty | Allowed trace-header destinations; empty disables injection |
| `traceType` | Enum, SDK default ddtrace | ddtrace, zipkin, zipkin_single_header, w3c_traceparent, w3c_traceparent_64bit, skywalking_v3, jaeger |
| `tracingSampleRate` | Number, SDK default 100 | Trace sampling, 0–100, independent of RUM sampling |
| `traceId128Bit` | Boolean, false | Depends on the selected propagation format |
| `allowTraceHeaderWithoutSession` | Boolean, false | Header injection without a sampled session; does not create RUM data |
| `visitorId.enabled` | Boolean, false | Anonymous Visitor ID header on matching fetch/XHR |
| `visitorId.header` | Required lowercase header when enabled | Cannot override authentication, security, browser-reserved, or trace headers |
| `visitorId.match` | Required nonempty URL array when enabled | SDK same-origin URL-prefix matching |
| `visitorId.ttlHours` | Positive finite number, SDK default 24 | Browser Visitor ID lifetime; native bridges can manage it separately |

~~~toml
[sites.rum]
# Retain the existing application and reporting fields.
allowedTracingUrls = [
  "https://api.example.com/",
  { match = "https://api.example.com/v2/", traceType = "w3c_traceparent" },
]

[sites.rum.visitorId]
enabled = true
header = "x-rum-visitor-id"
match = ["https://api.example.com/"]
ttlHours = 24
~~~

URL match strings must be absolute HTTP(S), without credentials or fragments; queries are permitted. Strings follow SDK matching semantics and are not executed as JavaScript functions or regular expressions. Cross-origin trace and Visitor ID headers require explicit API CORS support. HTTP page probes do not verify that browser exchange.

## Canvas 2D replay

| Field | Type/SDK default | Meaning |
| --- | --- | --- |
| `replayCanvasEnabled` | Boolean, false | Requires replay and a sampled session |
| `replayCanvasMode` | auto / manual; auto | Manual requires `snapshotCanvas()` |
| `replayCanvasSampling` | Positive number or "all"; 2 | Snapshot sampling or 2D command recording, not frames per second |
| `replayCanvasQuality` | 0–1 or low / medium / high; 0.4 | Presets also influence derived sampling and timing |
| `replayCanvasMimeType` | image/webp | image/png, image/jpeg, or image/webp |
| `replayCanvasMaxCanvasSize` | Positive finite number, 1280 | Maximum side before encoding |
| `replayCanvasMaxEncodedBytes` | Positive finite number, 40000 | Encoded-image byte budget |
| `replayCanvasMaxConcurrentEncodes` | Positive finite number, 1 | Concurrent encoding budget, normalized to an integer |
| `replayCanvasFlushImmediately` | Boolean; false for auto, true for manual | Flush Canvas changes immediately |
| `replayCanvasAutoInterval` | Positive finite number, 250 | Target snapshot interval per canvas, milliseconds |
| `replayCanvasAutoCooldown` | Positive finite number; follows interval | Cooldown after success, milliseconds |
| `replayCanvasAutoUnchangedBackoff` | Positive finite number, 3000 | Backoff for unchanged content, milliseconds |
| `replayCanvasAutoFailureBackoff` | Positive finite number, 5000 | Encoding failure backoff, milliseconds |
| `replayCanvasAutoMaxPerRun` | Positive finite number, 2 | Canvas budget per automatic run |

Preset quality/sampling/interval values are low = 0.25/1/500 ms, medium = 0.4/2/250 ms, and high = 0.5/4/125 ms. Explicit fields override derived preset values; remove them to follow the preset.

## Features requiring application code

Static TOML cannot supply callbacks such as beforeSend, injectTraceHeader, generateTraceId, shouldMaskNode, or shouldRecordCanvas; function/RegExp URL matchers; or plugins. Framework routing and WebGL plugins are not loaded automatically. Canvas 2D options do not imply WebGL support.

The tool does not invoke setUser, setGlobalContext, custom business events, manual views, or manual Canvas snapshots with dynamic business values. Use env/service/version for static deployment labels. Private SDK fields are not exposed.

Use canonical names instead of deprecated aliases: datakitUrl → datakitOrigin; sampleRate → sessionSampleRate; trackInteractions → trackUserInteractions; allowedTracingOrigins / allowedDDTracingOrigins → allowedTracingUrls; useCrossSiteSessionCookie → usePartitionedCrossSiteSessionCookie.
