# 服务端 OpenTelemetry 自动插桩

Rumable 可管理 Linux Nginx 和 Windows IIS/.NET 的请求 Trace，与浏览器 RUM、Session Replay 分别配置。它负责生成配置、挂接、预检、启停、配置生效探测和受管项回滚。原生模块、.NET Agent、DataKit 和 Collector 是外部运行依赖，需先安装；CLI 不会偷偷升级服务器、下载未经匹配的模块或重启整个 IIS。

本页适用于 **0.1.2**。直接使用与目标平台匹配的新包，无需为 CLI 安装 Go。这里只采集 HTTP 请求链路；应用内部方法、数据库等仍由对应语言 Agent/SDK 负责。

本版已包含 IIS 事务分阶段、旧 worker 等待，以及 Linux 单组件启停和多文件恢复修复。验收脚本与夹具仅保留在仓库；请检出与 CLI 版本相同的发布 tag 后运行脚本，实际运行边界见[验证记录](verification.md)。

## 支持范围

| 目标 | 插桩方式 | 本版要求 |
|---|---|---|
| Nginx | 官方 `ngx_otel_module` | Linux，Nginx 1.25.3+，与 Nginx 版本、发行版、架构匹配的模块；明文兼容模块 0.1.0；TLS/自定义 CA 需要 0.1.2+ |
| IIS/.NET Framework | 官方 .NET Automatic Instrumentation | IIS 10，Windows 10/Server 2016+，Windows PowerShell 5.1，.NET Framework 4.6.2+，v4 Integrated 独占应用池 |
| IIS/ASP.NET Core | 同一 Agent 的 CLR profiler + startup hook | 同上，池的 CLR Version 必须为 No Managed Code，按池位数安装 win-x64/win-x86 Agent；托管方式须在目标主机验收 |

IIS 的 OTel 首版支持 x64 Windows 上的 x64/x86 应用池，ARM64 会拒绝启用。静态文件、Classic ASP、纯反向代理不是 .NET 应用插桩的覆盖范围。旧 Nginx 1.9.4、旧 Windows/IIS 7.5 仍可使用原有 RUM；不能因 legacy Go CLI 能运行就推断 OTel 模块/Agent 也兼容。

## 自动安装与卸载

本版提供 `install-otel` 和 `uninstall`。省略 modulePath/agentPath，执行 install-otel 自动准备受管依赖，再按原流程 render/attach/enable；卸载默认保留依赖，`--remove-dependencies` 按归属、摘要与共享引用清理。完整命令、离线包、Windows 步骤及限制见[安装与卸载](otel-installation.md)。下文手工安装方式仍可用于已有依赖。旧 preview.2 包不包含这些新命令。

## 按站点配置和独立开关

直接复制对应示例：[Nginx](../examples/otel-nginx.toml)、[IIS](../examples/otel-iis.toml)。它们默认仅开启服务端 OTel，无需浏览器 applicationId。

```toml
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
```

`rum.enabled` 省略时仍为 true，保持旧配置行为；省略整个 otel 表时不加载 OTel。存在 otel 表表示首次挂接会预留模块及 exporter，即使 `otel.enabled=false` 也需要匹配的模块/系统。`enabled` 只控制采集，停用后保留模块和配置入口，完整移除站点使用 `uninstall`；只撤销 Linux 首次挂接仍可使用 `disable` 后的 `rollback --attachment`。

OTel 的路径范围与 RUM 独立，省略 includePaths 时覆盖 `/`，不会沿用 RUM 常见的 `/api/` 排除项。两者都是区分大小写的路径前缀，排除优先，自动跳过 `/__rumable__/`。IIS 按整个独占应用池插桩，不接受 OTel 路径过滤参数。

`[sites.otel]` 参数如下；不会将服务端参数传给 Web SDK：

| 参数 | 默认值 / 契约 |
|---|---|
| `enabled` | false；独立于 `rum.enabled` |
| `backend` | 必填，`datakit` 或 `dataway` |
| `endpoint` | 必填；DataKit 为带端口的 gRPC origin；Dataway 为 `/otel` 基础地址，禁止内嵌凭证、query 和 fragment |
| `serviceName` | 必填；实例共享的服务名，禁止配置变量展开 |
| `sampleRate` | 100；Nginx 支持两位小数，IIS 为父级优先的比例采样 |
| `includePaths` / `excludePaths` | Linux 默认 `[/]` / 空；IIS 不支持 |
| `modulePath` | Linux 可省略，由 install-otel 选择；显式填写须为匹配 ABI 的绝对路径，首次挂接后固定 |
| `trustedCA` | Nginx HTTPS 原生出口的绝对 PEM 路径；省略时使用系统 CA |
| `collectorEndpoint` | Linux Dataway 必填；本机 `http://127.0.0.1:PORT` 或 `http://localhost:PORT`，不得与 DataKit/IIS 混用 |
| `tokenEnv` | Dataway 必填；空间 Token 的环境变量名，不是 Token 明文 |
| `agentPath` | IIS 可省略，由 install-otel 选择；显式填写为外部维护的官方 .NET Agent 绝对路径 |
| `install.archive` / `install.url` / `install.sha256` | 可选，自定义离线/HTTPS 包及必填摘要，见[安装手册](otel-installation.md) |
| `runtime` | IIS 必填，`netframework` 或 `dotnet` |

Nginx 采样率支持 0–100，最多两位小数，按 Trace ID 比例抽样；IIS 使用 parentbased_traceidratio，百分比转换为 0–1。它们与 RUM 会话采样率互不替代。

若计划未来独立启用两种能力，首次就配置好 RUM 参数并保留 `rum.enabled=true`，先执行 `enable --component otel`；这会只启用服务端采集。以纯 OTel 配置首次挂接时，工具不会为未来 RUM 改写压缩、缓存和业务请求头；之后添加 RUM 需要重新审查并挂接 HTML 改写配置。

## DataKit 和 Dataway

DataKit 模式使用 OTLP/gRPC origin，必须包含端口，不带 `/otel/v1/traces` 等 HTTP 路径。先启用 DataKit 的 OpenTelemetry gRPC 接收，例如 `127.0.0.1:4317`；远端 DataKit 使用服务器实际可达的地址。HTTPS 网站不要求服务端 OTLP 也跟随浏览器协议，它可以访问本地 HTTP 接收器。

Dataway 要求 **1.17.0+**，使用 `/otel/v1/traces` 接收 OTLP/HTTP protobuf；配置基础地址写到 `/otel`。空间 Token 通过环境变量指定，不能填写浏览器 RUM clientToken 或观测云管理 API Key。[Dataway 协议](https://docs.guance.com/deployment/dataway/)、[DataKit 协议](https://docs.guance.com/integrations/opentelemetry/)。

Linux 原生模块仅支持 gRPC，需要已有 Collector 转换：

```toml
[sites.otel]
enabled = true
backend = "dataway"
endpoint = "https://dataway.example.com/otel"
collectorEndpoint = "http://127.0.0.1:4317"
tokenEnv = "RUMABLE_DATAWAY_TOKEN"
serviceName = "web-gateway"
modulePath = "/usr/lib/nginx/modules/ngx_otel_module.so"
```

```text
Nginx → OTLP/gRPC → 已运行的 Collector → OTLP/HTTP protobuf + X-Token → Dataway
IIS/.NET → OTLP/HTTP protobuf + X-Token → Dataway
```

首次 render 会在私有 `sites/<id>/collector/<id>/` 生成内容哈希命名的 Collector JSON 配置（JSON 是合法 YAML），status 显示确切路径。它引用 `${env:RUMABLE_DATAWAY_TOKEN}`，不包含 Token 值；接收器仅绑定本地 `127.0.0.1`。本版要求转换用 Collector 与服务器同机，避免生成的明文 loopback 接收器与实际连接的地址或 TLS 设置不一致；DataKit 模式仍可直连远端 gRPC 接收器。

把 Token 配置到 Collector 服务的环境中，并以生成的文件启动它，例如在已设置环境变量的终端执行：

```sh
otelcol validate --config /etc/rumable/.rumable/sites/gateway/collector/gateway/otel.HASH.json
otelcol --config /etc/rumable/.rumable/sites/gateway/collector/gateway/otel.HASH.json
```

上面第二条为前台运行示例，生产环境交由现有服务管理器托管。Rumable 不管理或回滚外部 Collector 服务，也不会修改其其他 pipeline。多个站点共享同一 exporter 时只启动一个对应 Collector。

变更 Dataway 目标/Token 环境变量名时，可以先生成候选配置，不修改部署状态：

```sh
umask 077
rumable render --config rumable.toml --collector-config > /etc/otelcol/rumable-next.json
```

先为新配置选择新的 collectorEndpoint 端口并启动接收器，再执行 enable；旧 Collector 保留到回滚窗口结束。普通 rollback 只恢复服务器配置和旧接收地址，不会让外部 Collector 自动切回旧出口。切勿仅更新文件却让接收器继续转发到旧的 Dataway。

IIS 不需要 Collector。修改 tokenEnv 对应环境变量的值后，执行普通 enable 或 enable --component otel 会更新池内凭据；未改变的值保持幂等。回滚恢复保存的旧凭据，不从当前环境重新解析。Agent 的 OTLP HTTP exporter 使用 `tokenEnv` 中的值生成 X-Token 头；环境变量必须存在于执行 CLI 的管理员进程。Token 会随应用池环境进入 IIS 配置及私有状态/备份，必须限制它们的 ACL；不会写入公开 SDK 资源、状态摘要或浏览器脚本。

## Nginx 自动挂接

优先使用 `install-otel`；手工方式可从官方仓库安装版本匹配的 `nginx-module-otel`；发行版 Nginx 与官方模块不一定 ABI 兼容。设置真实 modulePath，再执行：

```sh
rumable render --config nginx-otel.toml
rumable attach --config nginx-otel.toml --dry-run
rumable attach --config nginx-otel.toml
# 核对实际差异后输入 yes，完成备份、加载模块、挂接和 nginx -t / reload。
rumable enable --config nginx-otel.toml
```

加载模块属于 main 层，exporter 和 service.name 属于 http 层，采集开关和上下文传播属于目标 server/location。自动挂接将 load_module 放在实际主配置文件开头，即使 http 块位于后续 include 文件中，也保证在 events/http 之前加载。业务 include 与主文件可能同时变更，预览、备份和撤销覆盖每个文件。Nginx 原生模块的 exporter 和 service.name 是实例共享的，因此同一受管实例的多个站点必须使用一致的 backend、endpoint、collectorEndpoint、serviceName、modulePath、trustedCA 和 tokenEnv；不一致会拒绝。每个站点的开关、路径和采样率可以不同，Span 附带 `rumable.site.id`。

第一个配置站点拥有共享 exporter/module 入口，需先挂接它；后续站点复用，不重复定义 exporter。其他站点仍依赖该入口时禁止撤销拥有者的首次挂接。停用拥有者的采集不会移除入口，不影响其他站点。已完成撤销挂接的旧站点不再占用 exporter 配置；未完成的挂接必须先恢复。跨不同 state-dir 同时管理同一实例不受支持。已有未受管 OTel 配置需要先人工处理，工具不会接管。

已加载模块的路径必须与 `modulePath` 的绝对路径一致；相对 load_module 入口需要先人工统一。多站点共享实例修改 exporter 参数时，应先停用并按挂接逆序撤销依赖站点，再重新规划一致的配置，不能逐站点切换共享出口。

## IIS 安装和操作

预先解压并校验官方 [.NET 自动插桩 Agent](https://opentelemetry.io/docs/zero-code/dotnet/)，设置 agentPath。不要调用官方全局 `Register-OpenTelemetryForIIS`，它会重启整个 IIS。本工具仅通过 Microsoft.Web.Administration 设置目标独占应用池的环境，并在发生变更时回收该池。

```powershell
# 使用 64 位 Windows PowerShell 5.1，以管理员运行；先填好 iis-otel.toml。
.\rumable.exe render --config .\iis-otel.toml --state-dir C:\ProgramData\Rumable
.\rumable.exe check --config .\iis-otel.toml --state-dir C:\ProgramData\Rumable
.\rumable.exe enable --config .\iis-otel.toml --state-dir C:\ProgramData\Rumable
```

IIS 自动完成应用池配置挂接，不需要 Linux 的 attach 命令。按实际池位数选择 win-x64/win-x86 profiler；netframework 使用 COR profiler，dotnet 使用 CORECLR profiler及启动钩子。切换时清理旧的受管 runtime 设置，恢复时按保存的原值处理。业务的其他环境变量不受影响。

共享池、机器级及 WAS/W3SVC 服务级 OTel/profiler/startup 设置、池中未受管的 OTel 覆盖项、已有非本工具管理的插桩配置、池身份/运行时/位数或归属应用变化会阻止相关操作。包括 OTEL_EXPORTER_OTLP_TRACES_ENDPOINT 等信号级覆盖项的外部修改也会进入漂移检查。ASP.NET Core 应用有效 aspNetCore/environmentVariables 中的 OTel/profiler 项也会拒绝，防止 web.config 或继承配置覆盖池参数；其优先级见[微软 ANCM 文档](https://learn.microsoft.com/en-us/aspnet/core/host-and-deploy/iis/web-config#configuration-of-environment-variables)。移除池设置不能消除外部插桩，因此冲突会阻止改变池插桩状态的停用/恢复，需先人工解决。

启用 OTel 不要求 URL Rewrite，不改压缩、HTML 或 CSP；同时启用 RUM 时才检查 RUM 的模块和压缩要求。ASP.NET Core 必须使用 No Managed Code 池；工具不会自动改变池的 CLR/管线设置。[官方 IIS 说明](https://github.com/open-telemetry/opentelemetry-dotnet-instrumentation/blob/main/docs/iis-instrumentation.md)同时说明了这些池和全局环境配置条件。

IIS 的配置提交和池回收分为两次调用：先记录已提交快照，再校验快照/归属并回收目标池。仅操作 OTel 且环境值未变、也没有 RUM 激活需求时不回收；RUM 配置切换同样需要回收，同次变更只回收一次。最多等待旧 worker 退出 15 秒，超时或池未 Started 即失败，不强杀业务进程。失败恢复会重新回收原配置，即使上次恢复已写回配置；恢复仍失败则保留 transaction.json，解决进程/外部配置问题后执行 rollback 重试。配置提交完成但尚未记录快照时进程被强制终止，若现场不能与日志匹配，仍停止自动覆盖并要求人工审查。不能保证 .NET 应用无需预热或无短暂回收影响。

## 启停、回滚和验收

```sh
rumable enable                       # 按 TOML 同时应用两个开关
rumable enable --component otel       # 只开启/更新 OTel，保留 RUM 当前状态
rumable disable --component otel      # 只停用 OTel
rumable disable --component rum       # 只停用 RUM，保留 OTel
rumable status                       # rumEnabled / otelEnabled / Collector 路径
rumable probe
rumable rollback                     # 恢复上一次成功部署的两个开关和受管配置
rumable disable                      # 两者均停用
rumable rollback --attachment        # Linux：确认后恢复首次业务配置备份
```

原有 RUM 部署不带 OTel 模块入口，不能仅补一张表后假定已经挂接。升级时先停用并撤销原自动挂接，保留旧状态备份和缓存所需资产，然后以新站点 ID/状态目录重新 render、预览及挂接；IIS 残留的旧资源映射需按原部署记录审查处理。不会静默删除旧配置或接管未知规则。

单组件命令保留另一组件已部署的参数与开关，不应用其尚未部署的 TOML 修改。`enable/disable --component otel` 同时保留已部署的浏览器 SDK 版本、初始化脚本和 CDN 地址；更新 Web SDK 时使用普通 `enable` 或 `enable --component rum`。Nginx/IIS 的 RUM 单组件操作不因未变更的 OTel 接收端不可达而失败，服务器配置检查与提交后的 HTTP 探测仍执行。IIS 的 RUM 单组件操作还保留已部署的 Dataway Token，不重新读取当前进程的 Token，也不重新检查未变化的 Agent/接收端；OTel 环境不变时不会回收池。普通 enable/enable --component otel 才应用 Token 轮换。rollback 会检查目标 Agent/池条件，并使用已保存凭据，无需当前 Token 环境变量。所有多站点命令应显式传入 --site。

配置检查、提交、reload/池回收、HTTP 探测失败时恢复本次受管变更；首次挂接的候选配置令原生解析失败时，也会在来源与受管字节未漂移的前提下恢复原文件。发现外部修改时停止覆盖并保留事务。HTTP 探测每次重新建立连接，Linux 专属配置生效探针进一步区分新旧 worker，并在相隔 200ms 的连续两轮均成功后才确认启停，覆盖 Nginx 先启动新 worker、再通知旧 worker 退出的窗口。停用不保证中止已有请求，已排队的 Span 可以继续上报。

预检的接收地址检查目前是 TCP 可达；probe 校验 HTTP 和受管配置是否生效，不能证明 OTLP 鉴权成功或观测云入库。向应用发带已知 W3C traceparent 的请求，在观测云搜索该 Trace ID，确认 service.name 和上下游关系，才算接入验收。浏览器关联还需在 RUM 中设置 allowedTracingUrls / w3c_traceparent，并满足跨域请求要求。

以下验收命令在仓库同版本 tag 的根目录执行，源码与测试夹具不随安装包分发。Windows 获取步骤见 [Windows 验收手册](windows-acceptance.md#3-执行完整验收)。

Linux 回归脚本针对本机 ARM64 OrbStack，Nginx 镜像和 Collector 均使用 Linux arm64；需 Go 和 OrbStack/Docker。有源码时直接构建 CLI，二进制包可用 RUMABLE_BINARY 指向与测试容器架构匹配的 Linux 可执行文件。脚本：[smoke-otel.sh](https://gitlab.jiagouyun.com/guance/rumable/-/blob/release-oss-v0.1.2/scripts/smoke-otel.sh)，使用真实原生模块和 Collector、私有 OTLP HTTP 接收夹具，覆盖独立开关、站点/路径隔离、升级及回滚。Windows 实机脚本：[windows-otel-smoke.ps1](https://gitlab.jiagouyun.com/guance/rumable/-/blob/release-oss-v0.1.2/scripts/windows-otel-smoke.ps1)：

```powershell
.\scripts\windows-otel-smoke.ps1 -Binary C:\Rumable\rumable.exe `
  -AgentPath 'C:\Program Files\OpenTelemetry .NET AutoInstrumentation' `
  -Backend datakit -Endpoint http://127.0.0.1:4317 -ResultDir C:\RumableResults\otel-x64-01
# 32 位应用池：再执行一遍并加 -X86。
# Dataway：先在当前进程安全设置 RUMABLE_DATAWAY_TOKEN，再替换这两个参数：
# -Backend dataway -Endpoint https://dataway.example.com/otel
# 已安装 URL Rewrite 2 时加 -WithRUM，覆盖两种能力的独立启停。
```

该脚本创建两个独立站点/池，以实际环境值和 PID 验证目标进程变更、重复启停不回收、旁站不变、提交后 HTTP 失败恢复、共享池/外部修改拒绝，以及凭据缺失时的回滚；WithRUM 另测双开关和 RUM 操作不改变 OTel 凭据/PID。结果目录必须为新目录，保存 cli.log、report.json 和待查询的 Trace ID；默认清理临时站点，日志保留，Keep 会保留两组资源。报告 passed 只代表脚本断言通过，仍需查询 Trace 才能证明入库。完整命令与结果读取见[Windows 验收](windows-acceptance.md#7-iis-10-otel-专项验收)。
