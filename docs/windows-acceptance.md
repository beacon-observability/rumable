# Windows / IIS 验收

以下为 RUM 的 IIS 7.5+ 验收。服务端 OTel 另用 [windows-otel-smoke.ps1](../scripts/windows-otel-smoke.ps1)，仅针对 IIS 10 的独占 .NET 应用池，配置和可执行步骤见 [OTel 手册](otel.md#iis-安装和操作)。本轮没有 Windows 实机，不能把 PowerShell 解析/mock 结果当作 IIS 或 Agent 运行通过。

发布包提供 Windows x86/x64/ARM64 CLI；本手册的 IIS 流程仍要求 64 位 Windows 与 64 位 Windows PowerShell。x86 CLI 在 x64 系统经 Sysnative 运行管理桥接；真正 32 位 Windows 不运行本 IIS 验收。脚本核对实际架构与 build.json，ARM64 必须在对应原生系统独立验收。

本流程在目标 Windows 上运行，只创建一个随机命名的 Rumable-Acceptance 测试站点、专用应用池和三个应用，绑定 127.0.0.1:19182。默认完成后清理，证据单独保存。它不重启 IIS、不安装组件、不改变已有站点、全局压缩或配置委派。

本机 macOS / OrbStack 的脚本边界测试不能证明 Windows / IIS 已通过。下表每行都必须在对应实机或 Windows VM 单独执行，并保留报告。

### OrbStack 能验证什么

[OrbStack 使用 Linux 虚拟机和共享 Linux 内核](https://docs.orbstack.dev/architecture)，不能直接运行 Windows / IIS。Linux 上的 PowerShell 7 可以检查脚本语法、配置序列化与模拟接口；Windows 可执行文件的交叉编译也可以在这里完成。这些检查不能验证 Windows PowerShell 5.1、IIS 配置提交、Windows 权限或应用池行为。

完整验收应准备独立的 x64 Windows Server 2016+ / IIS 10 测试机，按下方步骤运行。现有 `windows-smoke.ps1` 覆盖 CLI 的注入、启停、升级和回滚；新的一键 `install.ps1` 下载、校验、安装及 `setup` 入口还需单独在该测试机验收，不能由现有 CLI 脚本通过推断安装器已通过。

## 一键安装入口验收（Windows x64）

本节验证从 OSS 安装到 IIS 生效的完整入口，可交给 Windows 上的 Codex 在独立测试站点执行。需要管理员身份的 64 位 Windows PowerShell 5.1、Windows 10 / Server 2016+ x64、IIS 与 URL Rewrite 2.x。旧 Windows、x86 或 ARM64 包按后面的手工流程分别验收。

1. 发布成功后打开 `https://static.guance.com/rumable/0.1.0/setup/index.html`，读取发布版本，确认 `0.1.0`。选 IIS，填写真实测试应用的 applicationId、OpenWay / DataKit 配置、网站地址、IIS 站点/应用路径，并为测试设置独立接入标识，例如 `windows-test`。网站验收路径必须返回带标准 `<head>` 的 HTML；按实际需要勾选静态压缩变更许可。
2. 下载并保存 TOML，在管理员 PowerShell 中执行页面生成的一条安装命令。它下载 `install.ps1`，校验同版本 x64 ZIP，安装 CLI，再运行 `setup`。记录命令退出结果；日志或截图中不要暴露令牌。
3. 检查状态及页面资源：

   ```powershell
   $rumable = Join-Path $env:ProgramFiles 'Rumable\0.1.0\rumable.exe'
   $state = Join-Path $env:ProgramData 'Rumable'
   $site = 'windows-test' # 与页面中的接入标识保持一致
   & $rumable version
   & $rumable status --state-dir $state --site $site
   & $rumable probe --state-dir $state --site $site
   ```

   要求 `rumEnabled=true`、`drift` 为空、`pendingTransaction=false`，探测成功。同源初始化脚本和 SDK 返回 200。浏览器中只出现一个本次部署的 `script[data-rumable]`，原有业务功能正常；额外验证包含 JavaScript 字符串 `"</head>"` 的页面未被改坏。
4. 再次执行相同一键命令，确认没有重复注入。修改测试环境 env，重新生成命令更新，再执行 `rollback --state-dir $state --site $site`；验证恢复上一配置的初始化脚本 URL。
5. 执行 `disable --state-dir $state --site $site`，确认新请求的 HTML 不再注入；旧版本静态资源应保留给缓存页面。再次执行 `rollback` 应恢复刚停用前的状态，最后再 `disable` 清理测试站点的注入。以上命令均由 `& $rumable` 调用，状态目录和接入标识保持一致。
6. 如出现未完成事务，保留 `%ProgramData%\Rumable` 中的状态、日志和备份，记录失败阶段，再用 CLI 的 `rollback` 恢复；不要删除状态后盲目重装。需要验证原生提交故障、压缩继承和多应用隔离时，继续运行下方完整自动验收。

一键安装不提取测试工具。完整工具另从 `https://static.guance.com/rumable/0.1.0/rumable_0.1.0_windows_amd64.zip` 下载，按 `SHA256SUMS` 校验后解压，执行后面的 `scripts/windows-smoke.ps1`。CLI 测试成功与真实观测云入库分别记录；实际事件须在浏览器请求收到 2xx 后查询观测云确认。

反馈结果请包含 Windows / IIS / PowerShell 版本、Rumable 版本、下载包 SHA-256、接入标识、首次/重复安装、停用、配置回滚、业务页面与数据上报结果；完整自动验收另附 `report.json` 和脱敏的 `cli.log`。

## 1. 选择环境和发布包

| 目标环境（x64） | IIS | Rumable 包 | PowerShell 验收 |
|---|---|---|---|
| Windows 7 SP1 / Server 2008 R2 SP1 | 7.5 | windows_amd64_legacy-go1.20.14 | 64 位 Windows PowerShell 3.0，另跑 5.1 |
| Server 2012 / 2012 R2 | 8.0 / 8.5 | windows_amd64_legacy-go1.20.14 | 系统版本及 5.1 分别记录 |
| Windows 10 / Server 2016+ | 10.0 | windows_amd64_go1.26.x | 64 位 Windows PowerShell 5.1 |
| 现代 Windows 上验证 legacy 包 | 10.0 | windows_amd64_legacy-go1.20.14 | 补充验证，不能替代旧系统行 |

旧 Windows 必须使用真实 Go 1.20.14 编译的 legacy 包。脚本先检查解压目录的 build.json，再记录可执行文件 version 输出和 SHA-256；旧系统发现现代包元数据时会在启动二进制前拒绝。单独复制 exe、缺少 build.json 时，只能在尝试启动后判断，建议保留完整发布包。

旧系统自带 PowerShell 2.0 不满足要求；应先按[微软 WMF 说明](https://learn.microsoft.com/en-us/powershell/scripting/windows-powershell/wmf-overview)部署适用的 Windows PowerShell 和 .NET 依赖。脚本按 PS 3 / .NET 4 基线编写，仍需对应矩阵实跑；不使用 PowerShell 7 来代替旧 Windows 验收。Go、WMF、IIS 与浏览器的兼容性分别记录。

## 2. 准备独立测试机

完整流程需要：

- IIS 静态内容、静态内容压缩、匿名访问、管理 API，以及 URL Rewrite 2.x 的出站规则能力。
- 已注册的 64 位 ASP.NET 4.x Integrated handler，用于动态 HTML、普通 chunked、应用生成 gzip 和提交失败后的 HTTP 恢复测试。它是测试夹具的依赖，不是 Rumable 注入所有业务站点的前提。
- 管理员权限、空闲的回环端口；默认 19182，可用 Port 参数改变。

在一次性 Server 2016+ VM 上，可先用服务器角色安装命令准备 IIS / ASP.NET，再安装官方 URL Rewrite 2.x：

~~~powershell
Install-WindowsFeature Web-Server,Web-Static-Content,Web-Stat-Compression,Web-Asp-Net45,Web-Mgmt-Console
~~~

Windows 10 / Windows 7 可在“启用或关闭 Windows 功能”准备相应 IIS 功能；Server 2008 R2 使用服务器管理器安装 IIS 7.5 和静态压缩。旧系统另需适用的 .NET 4.x；微软的[旧 IIS ASP.NET 注册步骤](https://learn.microsoft.com/en-us/visualstudio/debugger/remote-debugging-aspnet-on-a-remote-iis-7-5-computer)使用 Framework64 下的 aspnet_regiis.exe -ir。组件安装和必要重启应在验收前完成，不要在已有生产服务器上试跑角色安装命令。

静态 gzip 测试会预热文件，但不修改全局 httpCompression 参数。MIME、最小文件大小、CPU 或命中频率限制导致始终不压缩时，测试会失败并保留记录；不要把“配置值为 true”当作线上 gzip 已生效。参见[微软压缩测试条件](https://learn.microsoft.com/en-us/iis/extensions/iis-compression/using-iis-compression#testing-iis-compression)。

## 3. 执行完整验收

用资源管理器完整解压 Windows ZIP 到对应目录，保留 scripts、docs、build.json；旧 PS 3 不需要使用 Expand-Archive。先核对 ZIP 的 SHA-256 与随发布提供的 SHA256SUMS。

以下命令在“以管理员身份运行”的命令行执行，使用系统 64 位 Windows PowerShell：

~~~powershell
# Windows 7 SP1 / Server 2008 R2 SP1，进入解压好的 legacy 包
Set-Location C:\Rumable\legacy
& "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-smoke.ps1 -Binary .\rumable.exe -ExpectedIISVersion 7.5 -ResultDir C:\RumableResults\iis75-legacy-01
$LASTEXITCODE

# IIS 10，进入解压好的现代包
Set-Location C:\Rumable\modern
& "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-smoke.ps1 -Binary .\rumable.exe -ExpectedIISVersion 10.0 -ResultDir C:\RumableResults\iis10-modern-01
$LASTEXITCODE
~~~

结果目录必须不存在；每次运行使用新的目录，避免覆盖证据。若当前进程是 32 位，则应从 Sysnative 启动 64 位 PowerShell，或重新打开系统 64 位控制台。脚本会拒绝 32 位进程和 PowerShell 7。

同一台 IIS 10 上验证旧二进制部署、新二进制升级及回滚：

~~~powershell
Set-Location C:\Rumable\modern
& "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-smoke.ps1 -Binary .\rumable.exe -PreviousBinary C:\Rumable\legacy\rumable.exe -ExpectedIISVersion 10.0 -ResultDir C:\RumableResults\iis10-upgrade-01
~~~

PreviousBinary 必须支持相同 TOML schema 和 CLI。每个应用会先由旧包 render / enable，再由新包改变初始化配置、enable / rollback。未提供 PreviousBinary 时，仍测试当前包的配置升级与回滚。旧 Windows 上不得提供现代包作为 PreviousBinary。

只安装静态站点角色时可先运行部分验收：

~~~powershell
& "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-smoke.ps1 -Binary .\rumable.exe -ExpectedIISVersion 7.5 -StaticOnly -ResultDir C:\RumableResults\iis75-static-01
~~~

StaticOnly 跳过动态、chunked、源 gzip 和提交后 HTTP 失败恢复，结果固定为 partial、退出码 2；它不能替代完整验收。ExpectedIISVersion 对 IIS 8 / 8.5 分别填写 8.0 / 8.5。

## 4. 实际覆盖与成功条件

| 场景 | 断言 |
|---|---|
| 压缩原值与继承 | 三个应用分别继承父级开启、显式 true、显式 false；核对真实 gzip、启用后的关闭、停用后的局部属性存在性/原值及有效值 |
| 压缩变更许可 | 原本开启且未许可时拒绝；原本关闭时无需许可也可 check |
| 规则继承 | 父子业务 Header 规则持续有效；父级存在同名空间 Rumable 规则时拒绝；回滚保留部署期间的非受管父级变更 |
| 配置冲突 | rewriteBeforeCache=true、保留资源虚拟目录冲突被拒绝，原映射保留 |
| HTML 与旁路 | 静态、动态 HTML 均只有一个当前标签；普通 chunked 检查真实 Transfer-Encoding；排除路径、无锚点和 JSON 静态内容字节不变，动态下载 / JSON / 无锚点不注入 |
| 资源与缓存头 | 初始化脚本、SDK、元数据和许可文件同域可达且摘要正确；改写 HTML 无原文强 ETag；停用及升级回滚后旧资源仍可加载 |
| 幂等和升级 | 重复启用不改部署历史；改变 env / replay 参数生成新脚本 URL；rollback 恢复原 URL |
| 外部修改 | 改动受管 IIS 规则或资源后，disable / rollback 拒绝并保留外部修改；测试最后归还原值 |
| 压缩源不兼容 | 应用主动生成 gzip 时拒绝新部署，部署历史不变；不擅自修改应用中间件 |
| 提交后的恢复 | 文件计数使预检首次 200、提交后探测 503；断言第二次请求确实发生、旧部署恢复、无漂移和未完成日志 |
| 多应用隔离 | 三个应用最后均停用，压缩基线各自恢复，父站点设置不变 |
| 清理 | 仅清理本轮站点、专用池、对应 location 节点和临时目录；证据目录保留 |

“提交后的恢复”测试制造的是 HTTP 验证失败，不等于模拟了所有 IIS 配置 API 提交异常、断电或并发管理员写入。ARR / 代理源、真实云上报、CSP nonce、不同浏览器和会话回放播放仍需业务环境验收。

IIS 启停与升级后须按业务缓存策略清理旧 HTML。脚本使用无缓存请求核对新页面，不证明浏览器、CDN 或服务工作线程中已缓存的页面同步失效；尤其不要把旧 HTML 的条件请求 304 当成新内容已经完成注入。

## 5. 读取报告、保留与清理

~~~powershell
$report = Get-Content -LiteralPath C:\RumableResults\iis10-modern-01\report.json -Raw | ConvertFrom-Json
$report.result
$report.environment
$report.steps | Format-Table name,result,elapsedMs -AutoSize
$report.cleanup
~~~

退出码 0 表示本脚本范围内全部通过且清理成功（或显式 Keep）；1 表示环境、断言、证据写入或清理失败；2 表示部分验收。CLI 的预期拒绝也会记录在 cli.log，它们不会被算成脚本失败；意外成功则会使脚本失败。报告包含 OS / SP、IIS / URL Rewrite、PS / CLR、二进制版本及摘要、各步结果。deployment 目录保存本轮受管状态与资源，test-site-locations.xml 只保存本轮测试站点的局部配置，不导出整台服务器配置。

需要保留现场时，在完整运行命令末尾添加 -Keep 开关。每个应用正常结束时均停用；发生失败时可能保留失败阶段的配置，按报告排查。之后由原运行账户的管理员执行：

~~~powershell
& "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -NoLogo -NoProfile -ExecutionPolicy Bypass -File C:\Rumable\modern\scripts\windows-cleanup.ps1 -Report C:\RumableResults\iis10-modern-01\report.json
~~~

清理脚本核对随机测试名称、当前账户 TEMP 下的目录、站点物理路径，以及其他站点是否引用该池；不满足归属条件时停止。可重复执行，原报告保留。重启或强制终止后，先确认没有本轮脚本或 CLI 运行，再使用报告清理；不要对整台 IIS 执行恢复备份或 iisreset。

## 6. 浏览器与真实上报另行验收

自动流程使用 0% RUM / 遥测采样和不可用的本地接收地址，避免把测试数据写入任何真实应用。回放参数仅用于脚本升级与回滚验证，不代表录制上传已验证。

在独立业务测试站点配置真实 OpenWay / DataKit、100% 接入采样与所需回放开关后，用受 SDK 支持的现代 Chrome 检查 Console / Network、唯一初始化、交互、错误、回放 Worker 与观测云数据。旧 Windows 服务器通过本脚本，不代表 IE 或旧 Chrome 支持当前 SDK，也不代表真实接收端的 CORS / TLS 已通过。

## 7. IIS 10 OTel 专项验收

本节适用于 preview.3 及之后版本。使用对应发布包的 Windows CLI 和随包脚本，两者保持同一版本。前述 IIS 7.5 RUM 验收不能替代本节。

准备 Windows 10/Server 2016+、IIS 10、64 位 Windows PowerShell 5.1、ASP.NET 4.x Integrated、.NET Framework 4.6.2+，预先安装并校验匹配目标池位数的官方 .NET Agent。机器/WAS/W3SVC 不得已有全局插桩。DataKit 先启用 gRPC 接收；Dataway 使用有效空间 Token，提前以已有安全方式设置到管理员进程的 tokenEnv 环境变量中。可选 WithRUM 需要 URL Rewrite 2 和静态文件能力。

已有 Go 的 Windows 开发终端可在仓库根目录构建：

```powershell
$env:CGO_ENABLED='0'
go build -buildvcs=false -o C:\RumableTest\rumable.exe .\cmd\rumable
```

管理员的 64 位 Windows PowerShell 5.1 执行。ResultDir 每次必须不存在；默认占用 19780/19781 两个回环端口：

```powershell
$ps="$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe"
$agent='C:\Program Files\OpenTelemetry .NET AutoInstrumentation'
& $ps -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-otel-smoke.ps1 `
  -Binary C:\RumableTest\rumable.exe -AgentPath $agent `
  -Backend datakit -Endpoint http://127.0.0.1:4317 -ResultDir C:\RumableResults\otel-x64-01
$LASTEXITCODE

# 32 位应用池；Agent 须带 win-x86 profiler。WithRUM 同时验证独立开关。
& $ps -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-otel-smoke.ps1 `
  -Binary C:\RumableTest\rumable.exe -AgentPath $agent -X86 -WithRUM `
  -Backend datakit -Endpoint http://127.0.0.1:4317 -ResultDir C:\RumableResults\otel-x86-rum-01

# 当前进程已安全设置 RUMABLE_DATAWAY_TOKEN 后，另跑 Dataway：
& $ps -NoProfile -ExecutionPolicy Bypass -File .\scripts\windows-otel-smoke.ps1 `
  -Binary C:\RumableTest\rumable.exe -AgentPath $agent -WithRUM `
  -Backend dataway -Endpoint https://dataway.example.com/otel -ResultDir C:\RumableResults\otel-dataway-01
```

| 场景 | 脚本实际断言 |
|---|---|
| 独占池与旁站 | 新建两个随机站点/池；目标池环境生效，旁站池环境、HTTP service 值和 PID 不变；共享目标池时拒绝启用 |
| 重复启停 | 重复 enable/disable 不改变成功历史，实际服务请求的 PID 不变 |
| 独立开关（WithRUM） | RUM/OTel 分别启停，注入一次且资源摘要正确；RUM 操作不改变 OTel 环境/PID，Dataway 当前 Token 暂时缺失也可操作 RUM |
| 采样配置 | 0/100 转换为池环境 0/1；不将环境值当作采样统计或入库证明 |
| 失败恢复 | 改变 serviceName 后应用主动返回 503；CLI 必须失败，旧历史、旧环境和实际服务代次恢复，无 transaction.json 残留 |
| 外部修改 | 受管 serviceName 或新增信号级 exporter 覆盖时拒绝停用，保留外部值 |
| 回滚 | 恢复已保存 profiler/凭据，实际应用显示恢复的 serviceName；Dataway 当前 Token 缺失时仍可回滚 |
| 清理与报告 | 只清理两组测试资源，ResultDir 保留；池账户只能遍历状态父目录、读取公共脚本，不获准读取状态/事务文件 |

读取结果：

```powershell
$report=Get-Content C:\RumableResults\otel-dataway-01\report.json -Raw | ConvertFrom-Json
$report.result
$report.steps | Format-Table name,result,time -AutoSize
$report.cleanup
$report.traceId # 到观测云查询 Trace，核对 service.name 与上下游。
```

退出码 0 代表本脚本断言与清理通过（或显式 Keep），失败返回非零；report.json 与 cli.log 保留环境、二进制摘要和阶段结果，不保存原始 Token。默认纯 OTel 运行不覆盖 RUM 联合场景，应另跑 WithRUM。脚本是 .NET Framework 夹具；ASP.NET Core 的 No Managed Code、托管方式与 Agent 加载仍须在业务测试应用验收。

Keep 会保留两组站点/池与受限状态目录。之后在同一账户的管理员 64 位 PowerShell 中，按报告归属清理：

```powershell
Add-Type -Path "$env:SystemRoot\System32\inetsrv\Microsoft.Web.Administration.dll"
. .\scripts\windows-iis-helpers.ps1
Remove-TestDeployment $report.site (Join-Path $env:TEMP $report.site)
Remove-TestDeployment $report.peer (Join-Path $env:TEMP $report.peer)
```

本轮 macOS/OrbStack 没有 Windows/IIS。实际池回收、IIS API、Windows PowerShell 5.1、Agent 加载和真实入库，均需保存目标环境结果后才能标为通过。
