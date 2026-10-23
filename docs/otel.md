# Optional server-side OpenTelemetry

Rumable can configure request tracing for Linux Nginx and Windows IIS/.NET independently of browser RUM and Session Replay. The compatible CLI generates configuration, attaches, checks, activates, probes, and restores owned settings. Native modules, the .NET Agent, receivers, and Collectors are external runtime dependencies.

The initial server-side workflow was introduced in 0.1.2 and later received lifecycle fixes. Use matching release binaries and source-tag acceptance scripts. Public support claims remain limited to the environments and telemetry actually verified in [verification records](verification.md).

## Support scope

| Target | Instrumentation | Requirements |
| --- | --- | --- |
| Nginx | Upstream ngx_otel_module | Linux, Nginx 1.25.3+, module matching version/distribution/architecture; plaintext compatibility includes module 0.1.0; TLS/custom CA needs 0.1.2+ |
| IIS/.NET Framework | Upstream .NET Automatic Instrumentation | IIS 10, Windows 10/Server 2016+, Windows PowerShell 5.1, .NET Framework 4.6.2+, dedicated v4 Integrated pool |
| IIS/ASP.NET Core | CLR profiler and startup hook | Same platform baseline, No Managed Code pool, matching x64/x86 profiler; actual hosting mode must be validated |

IIS OTel currently targets x64 Windows with x64/x86 pools. ARM64 is rejected, including emulated management hosts. Static files, Classic ASP, and pure reverse proxies are outside .NET application instrumentation. Old Nginx/Windows can remain eligible for RUM without being eligible for OTel.

The documented acceptance scope is request traces. Application-specific methods and additional language capabilities need their own instrumentation and validation. No new metrics, logs, profiling, or Security capability is implied.

## Install dependencies

Use `install-otel` and [dependency management](otel-installation.md). Omit modulePath/agentPath for managed installation, or explicitly provide an existing externally managed dependency. Linux attachment can load the configured module; IIS applies pool environment directly.

The default automatic IIS dependency remains upstream .NET Agent v1.17.0. This documentation change does not substitute another Agent distribution.

## Component configuration

~~~toml
[sites.rum]
enabled = false

[sites.otel]
enabled = true
backend = "datakit"
endpoint = "http://127.0.0.1:4317"
serviceName = "web-gateway"
modulePath = "/usr/lib/nginx/modules/ngx_otel_module.so"
sampleRate = 100
includePaths = ["/"]
excludePaths = ["/health"]
~~~

This is a table fragment; adapt a complete [Nginx](../examples/otel-nginx.toml) or [IIS](../examples/otel-iis.toml) configuration. Omitted rum.enabled is still true. Omitted otel means no server instrumentation. An otel table reserves module/exporter attachment even when enabled is false, so native dependencies remain necessary.

If you expect to enable RUM later without reattachment, review and configure RUM from the first attachment. An OTel-only attachment does not prepare HTML compression/cache/header changes. Adding RUM later requires a new reviewed attachment plan.

RUM and OTel path ranges are independent. Nginx OTel defaults to / and excludes reserved resources. IIS instruments the dedicated pool and does not accept path filters.

| Field | Meaning |
| --- | --- |
| enabled | False by default; independent of RUM |
| backend | Compatible enum datakit or dataway |
| endpoint | Direct collector gRPC origin with port, or authenticated gateway base ending in /otel |
| serviceName | Required stable service name; shared across a managed Nginx instance |
| sampleRate | 0–100; Nginx supports two decimal places; IIS uses parentbased_traceidratio |
| includePaths / excludePaths | Linux URL prefixes; IIS rejects these |
| modulePath | Absolute Linux native-module path; omit for install-otel |
| trustedCA | Absolute Linux PEM path for HTTPS native export; omitted uses system trust |
| collectorEndpoint | Linux authenticated-gateway mode only: existing loopback plaintext gRPC receiver |
| tokenEnv | Server environment variable containing the gateway token, not a literal token |
| agentPath | IIS Agent path; omit for managed installation |
| runtime | IIS netframework or dotnet |
| install.archive / install.url / install.sha256 | Verified custom distribution; see dependency guide |

RUM session and server trace sampling do not substitute for one another. IIS sampling converts percentages to 0–1. Nginx uses Trace ID based proportional selection.

## Receiver modes

The schema retains its existing enum names for compatibility.

**Direct collector, backend=datakit:** use an OTLP/gRPC origin such as http://127.0.0.1:4317 with an explicit port and no HTTP trace path. Enable the receiver first. A remote collector must be reachable from the server. Browser HTTPS does not force local server-side transport to use HTTPS.

**Authenticated gateway, backend=dataway:** the existing protocol requires a /otel base and a receiver compatible with gateway version 1.17.0+ behavior. It exports OTLP/HTTP protobuf to /otel/v1/traces with X-Token. Set tokenEnv to a server-side workspace-token variable, not a browser Client Token or management key.

~~~text
Nginx -> OTLP/gRPC -> existing Collector -> OTLP/HTTP protobuf + X-Token -> gateway
IIS/.NET -> OTLP/HTTP protobuf + X-Token -> gateway
~~~

Nginx's native exporter uses gRPC, so this gateway mode requires an externally managed Collector. The CLI generates Collector JSON and prints its path; it does not install, start, reconfigure, or roll back the Collector service or its other pipelines. One shared exporter uses one corresponding Collector.

~~~toml
[sites.otel]
enabled = true
backend = "dataway"
endpoint = "https://gateway.example.com/otel"
collectorEndpoint = "http://127.0.0.1:4317"
tokenEnv = "RUMABLE_DATAWAY_TOKEN"
serviceName = "web-gateway"
modulePath = "/usr/lib/nginx/modules/ngx_otel_module.so"
~~~

Keep tokenEnv's value in the administrator/service environment. To prepare a candidate external configuration without changing deployment state:

~~~sh
umask 077
rumable render --config rumable.toml --collector-config > /etc/otelcol/rumable-next.json
~~~

Choose a new loopback port for a new Collector configuration, start it, then enable. Retain the previous Collector through the rollback window. Deployment rollback restores the old receiving address, not the external Collector's destination. Merely rewriting a file does not move traffic to a new gateway.

IIS needs no Collector in this mode. Ordinary enable or enable --component otel resolves tokenEnv from the invoking administrator process and updates pool credentials when changed. Rollback restores saved credentials without rereading the current token. Credentials enter IIS environment and private state/backups; restrict ACLs. They do not enter public browser assets or summary output.

## Nginx attachment

Use install-otel or prepare a version/ABI-matched module manually:

~~~sh
rumable render --config nginx-otel.toml
rumable attach --config nginx-otel.toml --dry-run
rumable attach --config nginx-otel.toml
rumable enable --config nginx-otel.toml
~~~

The actual main file gets load_module before events/http, including when http lives in an include. Exporter and service are http-level; tracing and context propagation apply to target server/locations. Preview, backup, and recovery can cover multiple files.

A native Nginx module has one process-wide exporter/resource configuration. Managed sites on the same instance must agree on backend, endpoint, collectorEndpoint, serviceName, modulePath, trustedCA, and tokenEnv. Site switches, ranges, and sampling may differ; spans retain the compatible rumable.site.id attribute.

Attach the first configured owner before dependent sites. Other sites reuse the shared entry. You cannot undo the owner's first attachment while dependents still use it. Disabling only its collection leaves the shared module/exporter entry available. Fully detached old sites stop reserving it; unfinished attachment recovery continues to block.

Competing state roots and unmanaged existing otel directives are not adopted. Loaded module paths must match absolute modulePath; normalize relative existing paths manually. To change shared exporter parameters, disable and undo dependent attachments in reverse order, then plan consistent new settings.

## IIS installation and activation

Use a verified compatible Agent. Do not call the upstream global Register-OpenTelemetryForIIS as part of this workflow; the CLI only manages the selected dedicated pool.

~~~powershell
.\rumable.exe render --config .\iis-otel.toml --state-dir C:\ProgramData\Rumable
.\rumable.exe check --config .\iis-otel.toml --state-dir C:\ProgramData\Rumable
.\rumable.exe enable --config .\iis-otel.toml --state-dir C:\ProgramData\Rumable
~~~

No Linux attach command is needed. netframework uses COR profiler; dotnet uses CORECLR and startup hooks with the matching win-x64/win-x86 profiler. Switching runtime clears old owned settings; restore uses saved original values. Unrelated environment variables are retained.

Shared/stopped pools, changed identity/runtime/bitness/membership, and unmanaged instrumentation in machine, WAS/W3SVC, pool, or effective ASP.NET Core environment block conflicting operations. Signal-specific OTLP overrides are included in drift checks. Disabling cannot remove external instrumentation, so resolve conflicts before lifecycle changes.

OTel-only operation does not require URL Rewrite or alter HTML/CSP/compression. ASP.NET Core requires No Managed Code; the CLI does not change pool CLR/pipeline settings. See [upstream IIS guidance](https://github.com/open-telemetry/opentelemetry-dotnet-instrumentation/blob/v1.17.0/docs/iis-instrumentation.md).

Configuration commit and pool recycle are separate calls. Persist the committed snapshot before rechecking ownership and activating. Combined RUM/OTel changes recycle once. Unchanged OTel-only environment is idempotent; RUM activation needs can still require recycle.

Wait up to 15 seconds for old workers. Timeout or a stopped pool fails without killing requests. Failed changes restore and reactivate original settings. If runtime recovery remains incomplete, retain transaction.json and retry rollback after fixing the cause. A crash between native commit and local snapshot recording may require manual review. No claim of zero warmup or zero disruption is made.

## Lifecycle and acceptance

~~~sh
rumable enable
rumable enable --component otel
rumable disable --component otel
rumable disable --component rum
rumable status
rumable probe
rumable rollback
rumable disable
rumable rollback --attachment
~~~

Use explicit site/state selectors for multiple sites. Plain enable applies both configured switches; single-component commands preserve the other component's deployed values. OTel-only operations preserve browser assets and CDN references. RUM-only operations preserve unchanged OTel credentials and do not require unchanged receivers to be reachable. Rotate credentials with ordinary enable or the OTel component.

An older RUM attachment without OTel entries cannot be upgraded merely by appending a table. Disable and undo the original automatic attachment, preserve backups/cache assets, then render and attach a reviewed new deployment with new identity/state. Review preexisting IIS mappings explicitly.

Configuration, commit, reload/activation, or HTTP failure attempts managed restoration. External drift stops overwrite. Linux active-generation probes establish fresh connections and require two successful rounds separated by 200 ms to avoid old/new graceful-reload worker races. Disable does not cancel in-flight requests or already queued spans.

Endpoint preflight currently proves TCP connectivity. HTTP probes prove configuration/injection, not OTLP authentication or storage. Send an application request with a known W3C traceparent and find that Trace ID at the receiver, checking service.name and parent/child relationships. Browser correlation also needs allowedTracingUrls with w3c_traceparent and appropriate cross-origin permissions.

Source-tag `scripts/smoke-otel.sh` exercises actual Linux modules, Collector, and private OTLP fixtures. Historical runs used ARM64 Docker/OrbStack. Set RUMABLE_BINARY to test a matching release artifact. Private fixture receipt is not production ingestion acceptance.

Windows commands and evidence requirements are in [Windows OTel acceptance](windows-acceptance.md#7-iis-10-otel-acceptance). Scripts are obtained from the matching source tag, not platform archives.
