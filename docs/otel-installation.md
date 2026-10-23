# OTel dependency installation and uninstall

These commands are available from 0.1.0-preview.3 onward. Run the matching Windows/Linux CLI on the real server and use the same absolute state directory for install, render, attach, and enable.

## Short workflow

Copy [otel-nginx.toml](../examples/otel-nginx.toml) or [otel-iis.toml](../examples/otel-iis.toml), set actual target and receiver values, and omit modulePath/agentPath for managed installation:

~~~sh
rumable install-otel --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
rumable render --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
rumable attach --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
rumable enable --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
~~~

Install displays sources, destination, and effects before confirmation; reviewed automation can pass --yes. --dry-run neither downloads nor installs. Repeat installation verifies and reuses managed files without overwriting. Installing a dependency does not attach or enable collection. Prepare an external receiver or Collector first as described in the [OTel guide](otel.md).

| Platform | Automatic operation | Prerequisites and boundary |
| --- | --- | --- |
| Linux Nginx x64/ARM64 | Select an nginx-module-otel APT package requiring the exact Nginx version; download, verify index SHA-256, privately extract, and check native loading | Nginx 1.25.3+, APT/dpkg-deb, native libraries, a trusted configured repository with current indexes. No repository addition, Nginx upgrade, or package installation scripts |
| x64 Windows IIS with x64/x86 pool | Download upstream .NET Agent v1.17.0, verify digest, extract both architectures, and grant IIS_IUSRS read/execute on its dependency tree | IIS 10, Windows 10/Server 2016+, administrator 64-bit Windows PowerShell 5.1, dedicated .NET pool. No global registration or IIS restart |

ABI mismatch rejects installation. Windows ARM64 and old Windows/IIS remain outside OTel even when RUM works.

Dependencies live beside private state under `<state-dir>-otel/<server>-<digest>/`. Paths are resolved by the CLI. Matching sources/instances within one state root reuse dependencies. Do not move the directory or manage one instance through competing state roots.

An IIS ACL failure retains pendingACL and blocks enable; repeat installation can finish or ownership-aware cleanup can remove it. Parent traversal still needs the actual pool identity. The CLI does not relax private-state or arbitrary parent ACLs.

## Windows commands

~~~powershell
# Administrator 64-bit Windows PowerShell 5.1; edit TOML and omit agentPath.
$state = 'C:\ProgramData\Rumable'
.\rumable.exe install-otel --config .\otel-iis.toml --state-dir $state --dry-run
.\rumable.exe install-otel --config .\otel-iis.toml --state-dir $state
.\rumable.exe render --config .\otel-iis.toml --state-dir $state
.\rumable.exe enable --config .\otel-iis.toml --state-dir $state
~~~

IIS does not require attach. Static files, Classic ASP, and pure reverse proxies are outside .NET instrumentation scope. Installation does not alter pool CLR, pipeline, or ownership; enable checks them independently.

## Offline archives and external dependencies

For operator-managed dependencies, explicitly set modulePath/agentPath and skip install-otel. Those files are not adopted, overwritten, or removed.

Use a verified archive for isolated/RPM/custom environments, or an immutable HTTPS URL. Select either archive or URL, always with its actual 64-character lowercase SHA-256. URLs cannot contain credentials, query, or fragment.

~~~toml
[sites.otel.install]
archive = "/srv/packages/nginx-module-otel.deb"
# url = "https://packages.example.com/fixed/module.tar.gz"
sha256 = "REPLACE_WITH_ACTUAL_64_CHARACTER_LOWERCASE_SHA256"
~~~

Windows paths can use TOML literal strings, for example `archive = 'C:\Packages\agent.zip'`. ZIP, tar, tar.gz, and Nginx .deb are supported; .deb needs dpkg-deb. RPM itself is not extracted; repackage in a trusted build environment. Links, devices, traversal, and duplicate archive entries are rejected. Archive installation scripts are not executed.

| Archive | Required layout |
| --- | --- |
| Nginx | Exactly one matching ngx_otel_module.so; retain LICENSE/copyright |
| IIS | Upstream Windows ZIP root with win-x64, win-x86, net, netfx, startup hooks, and all runtime dependencies |

rumable-install.json records source, hash, platform, and file digests. There are no background dependency updates. A new source requires a new installation and attachment plan. An attached Linux modulePath is fixed; never overwrite a loaded module.

## Disable or uninstall

Disable only server tracing:

~~~sh
rumable disable --component otel --state-dir /var/lib/rumable --site gateway
~~~

Preview and remove the entire selected site's RUM and OTel integration:

~~~sh
rumable uninstall --state-dir /var/lib/rumable --site gateway --dry-run
rumable uninstall --state-dir /var/lib/rumable --site gateway
rumable uninstall --state-dir /var/lib/rumable --site gateway --remove-dependencies
~~~

Uninstall retains dependencies unless --remove-dependencies is selected. Cleanup can be retried after site removal. An installed dependency not yet rendered can also be removed using its original configuration. Use --yes only after review.

The operation verifies state, business files, and native ownership again after confirmation; disables both components; restores initial Linux attachment or IIS rules/compression/pool environment; and removes owned asset mappings. Remove shared-exporter dependent sites before their owner.

Dependency removal checks current/previous snapshots, effective native configuration, IIS machine/service/pool/application references, and individual file digests. Missing state does not authorize deletion of live referenced files. New or changed files block cleanup. External modules/Agents, Collector services, system packages/repositories, and the CLI itself are not removed.

Address cached HTML first. When an IIS resource mapping predated integration, it and its original assets are preserved. Otherwise private state, backups, and assets are archived under `<state-dir>/removed/<id>-<timestamp>/`. Restrict access. Ordinary rollback cannot re-create an uninstalled site; redeploy through render/attach/enable.

Recover pending deployment or attachment transactions first. Failed uninstall remains at its completed phase; fix the cause and repeat. It does not re-enable collection merely to undo a completed uninstall phase.

Dependency deletion first renames into quarantine. Windows process-held files may need retry or reviewed cleanup after confirming no live reference. Never delete a directory still referenced by load_module or a pool profiler.

After enable, send business requests and verify their traces at the actual receiver; see [acceptance](otel.md#lifecycle-and-acceptance).
