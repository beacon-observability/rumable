# Rumable

当前版本：**0.1.0-preview.2**。Windows / Linux 用户从[使用指南](USER-GUIDE.md)开始；平台选包、安装、TOML、启用、回放、CDN、升级和恢复步骤均在其中。

通过 Nginx / Apache / IIS 的 HTML 响应改写，为网站启用观测云 RUM Web SDK。业务代码无需修改；初始化脚本由网站同域提供，SDK 默认同域托管，也可按站点使用 CDN，浏览器直接向 OpenWay 或 DataKit 上报。

自动挂接、差异预览、备份及恢复的范围见[操作手册](docs/operations.md#自动首次挂接linux)。

Go 实现，TOML 解析器编译进单个可执行文件，无常驻进程、数据库或 Node.js 运行依赖。内嵌官方 SDK **3.3.21**，校验信息见 [sdk.json](internal/rumable/assets/sdk.json)，第三方许可随发布包提供。

本版包含 **Nginx / Apache / IIS/.NET 服务端 OTel 请求 Trace**：`[sites.otel]` 与 `[sites.rum].enabled` 独立配置，支持 DataKit、Dataway、自动挂接及受管配置回滚。Dataway 在 Linux 通过已有 Collector 转换协议，IIS/.NET 可直连；原生模块/Agent 有独立兼容范围。安装、示例和验收见 [OTel 使用手册](docs/otel.md)。0.1.0-preview.2 包含 OTel 支持；原 0.1.0-preview.1 包保持不变。

站点使用官方 CDN 时，在 TOML 中加入以下配置，再执行 `enable`；省略此表仍使用同域内置 SDK。

```toml
[sites.sdk]
source = "cdn"
```

可用 `url` 指向提供相同 SDK 字节的 HTTPS CDN；`integrity=true` 开启 SRI，要求 CDN 支持 CORS。官方 v3 地址目前没有该 CORS 响应头，默认不设置 SRI。启用时校验版本摘要及 CSP；可变 CDN 后续更新会直接影响浏览器，旧远程内容无法由本工具恢复。需要固定版本和完整离线回滚时使用默认同域方式，或固定 URL 并开启 SRI。详见[SDK 加载配置](docs/configuration.md#sdk-加载方式)。

## 三步接入

在服务器的固定工作目录中执行，例如 `/etc/rumable`。后续继续使用同一目录，配置和状态不要放在网站根目录或临时目录。使用有权检查、重载服务器及写入受管目录的账号。

```sh
# 1. 生成精简 TOML；选择 nginx / apache / iis 和 openway / datakit。
rumable init --server nginx --reporting openway
# 编辑 rumable.toml：网站地址、服务器目标、applicationId 和上报参数。

# 2. 生成部署；终端会列出本机路径、首次挂接位置和可复制命令。
rumable render
# 自动识别站点并展示变更；输入 yes 后挂接、备份、检查并平滑重载。
rumable attach

# 3. 自动预检、部署、检查服务器配置、平滑生效并探测。
rumable enable
```

默认 `rumable.toml` 与 `.rumable` 都在当前目录；单站点省略 `--site`，多站点必须显式指定。`render` 只准备停用状态，不自动改写业务配置。Linux 的 `attach` 按主机名和端口选择唯一站点，确认后挂接；成功后无需修改 `attachmentReviewed`。可先 `attach --dry-run`，歧义时按候选列表加 `--target FILE:LINE`。无法安全自动合并的配置使用 `ATTACH.md` 人工挂接并设置 `attachmentReviewed=true`。首次挂接审查压缩、继承、权限、缓存和 CSP；后续修改 TOML 后只需再次 `enable`。独立的 `check` 可用于提前诊断，启用时无需重复执行。

```sh
rumable status
rumable probe
rumable disable
rumable disable --component otel # 只停用服务端采集，保留 RUM
rumable rollback
# 完全撤销首次自动挂接（先 disable，清理缓存页面，再确认恢复原服务器文件）：
rumable rollback --attachment
# 不在原目录时，显式提供路径：
rumable status --state-dir /etc/rumable/.rumable
```

`enable` 失败尝试恢复原配置；HTML 与脚本均检查 identity / gzip。`disable`、`rollback`、`status` 使用受管状态，单个受管站点即使原 TOML 遗失也无需补写 ID；多个受管站点用 `--site` 选择。HTTP 成功代表注入和脚本可达，仍需在浏览器与观测云确认实际采集。

## Nginx

自动挂接由 `attach` 完成；人工方式将生成的三个 include 分别放入现有 `http`、目标 `server`、每个 HTML handler 的 `location`。反代保留完整业务请求头配置，只按 ATTACH.md 合并源压缩和条件请求设置；需要保留的业务 sub_filter 与 location include 显式放在同一层。静态站点审查预压缩文件。工具不会创建竞争 location 或覆盖业务 handler。

Nginx 最低要求 **1.9.4 + http_sub_module**。1.9.4 的 `check`、`enable` 会自动追踪有效配置的 include 层级，发现 `http` 层 sub_filter 即拒绝；`attachmentReviewed=true` 不能绕过。先将这类业务规则移到 `server` / `location` 层，或升级到 1.9.5+。无法可靠判定配置时也会拒绝，具体支持范围见[兼容性说明](docs/compatibility.md)。

## Apache

支持 **Linux Apache 2.4.23+**，需要加载 `mod_lua`、`mod_filter`、`mod_alias`、`mod_authz_core`、`mod_headers`。Debian/Ubuntu 默认使用 `apache2ctl` 保留发行版 envvars；其他发行版修改生成的命令路径。也可使用 [examples/apache.toml](examples/apache.toml)。

```sh
rumable init --server apache --reporting datakit --config apache.toml
# 填写网站、服务器目标、applicationId 和 datakitOrigin。
rumable render --config apache.toml
rumable attach --config apache.toml
# 预览并输入 yes；复杂配置按 ATTACH.md 人工挂接。
rumable enable --config apache.toml
```

使用内容哈希命名的私有 Lua 输出过滤器，跨普通响应分块处理首个 `</head>`；没有锚点时正文不变。公共 SDK 由独立 Alias 提供。启停和回滚通过配置检查后执行 `-k graceful`，复用同一事务、摘要和 HTTP 探测保护。Session Replay 仍由该站点 `[sites.rum]` 控制，默认关闭。

启用期间，所选 GET/HEAD 路径会清除条件请求和 Range 头，HTML 的 HEAD 响应也会去除原文验证头和长度；存在 `mod_cache` 时，对整个 VirtualHost 关闭 quick handler，并按 includePaths 前缀绕过缓存，避免旧 HTML 绕过过滤或缓存旧脚本。停用移除这些受管设置。反代源 HTML 必须未压缩，Apache 输出 gzip 可以保留。首次挂接、权限、缓存影响与可执行验证步骤见 [Apache 接入手册](docs/apache.md)。Apache 2.2、2.4.23 之前及 Windows Apache 不在本版支持范围。

## IIS

在目标 64 位 Windows 上以管理员运行匹配架构的可执行文件，IIS 接入优先使用 x64 包。需要 **IIS 7.5+、URL Rewrite 2.x、64 位 PowerShell 3.0+**；推荐 PowerShell 5.1。旧 Windows 必须选择 legacy 发布包，详见[兼容矩阵](docs/compatibility.md)。

```powershell
.\rumable.exe init --server iis --reporting datakit --config .\iis.toml
# 填写目标站点、应用、上报参数；审查静态压缩影响后允许 allowCompressionChanges。
.\rumable.exe render --config .\iis.toml --state-dir C:\ProgramData\Rumable
.\rumable.exe enable --config .\iis.toml --state-dir C:\ProgramData\Rumable
```

工具通过 Microsoft.Web.Administration API 管理选定站点/应用的 `rumable-<id>-*` 出站规则、前置条件、`/__rumable__` 虚拟目录和局部静态压缩设置。保留其他业务配置，不重启整个 IIS。压缩调整影响所选应用的静态资源，启用前必须审查 `allowCompressionChanges`。

首次部署还需配置文件 ACL：管理员管理状态目录，IIS 进程仅需读取 `sites\<id>\public` 及遍历其父目录；保留状态文件的访问限制。SDK 文件需匿名可访问并按 JavaScript MIME 类型提供。不要将整个状态目录设为网站根目录。

如果应用存在全路径路由、ANCM 或入站重写，需要在首次挂接时让保留资源路径绕过这些规则并由 StaticFile handler 提供。虚拟目录本身不会覆盖业务的 handler 配置；启用后的资源校验不通过时会恢复配置。

## 配置与命令

- 配置示例：[Nginx / OpenWay](examples/nginx.toml)、[Apache / OpenWay](examples/apache.toml)、[IIS / DataKit](examples/iis.toml)。两种上报方式均可用于三个服务器；单站点自动选择，多站点通过 `--site` 定位。
- 默认配置文件为 `rumable.toml`，使用 UTF-8 编码；每个站点使用 `[[sites]]`，其下配置 `[sites.nginx]`、`[sites.apache]`、`[sites.iis]` 三选一，以及 `[sites.rum]`。支持注释，拒绝未知字段和重复定义。`init` 默认生成精简的 Nginx / OpenWay 配置；`--server` / `--reporting` 切换目标，`--full` 列出完整静态 SDK 参数。
- Windows 路径建议使用 TOML 单引号字面量，例如 `binary = 'C:\nginx\nginx.exe'`，避免反斜杠转义。
- 内部部署状态、事务日志及 CLI 结构化输出保持 JSON；已有受管状态可直接继续停用和回滚，用户配置需按 TOML 示例转换。
- `id` 为稳定标识，禁止修改已挂接站点的服务器目标或 `publicURL`；迁移时使用新 ID 并重新挂接。
- `includePaths`、`excludePaths` 是**区分大小写的 URL 路径前缀**；排除优先。`/shop` 同时匹配 `/shopping`，目录范围请写 `/shop/`。
- 路径规则只控制 HTML 响应是否注入，不过滤 SDK 已采集的资源请求，也不在 SPA 内部切换路由时重新评估。
- `probePaths` 必须覆盖实际 HTML handler；它们需要匿名 HTTP 200 完整页面。工具不携带生产登录凭证、不跟随重定向；登录后页面仍可被服务器规则覆盖，但需另行浏览器验收。
- `publicURL` 为网站或 IIS 应用的基地址，子应用示例：`https://example.com/app/`，对应 `iis.application=/app`，探测路径填 `/app/`。
- OpenWay：`applicationId`、`site`、`clientToken`；DataKit：`applicationId`、`datakitOrigin`，禁止混用。上报地址必须是 origin（无路径、查询和认证信息），HTTPS 网站不能配置 HTTP 上报地址。
- `clientToken` 是随页面发布的浏览器令牌。**不得填入观测云管理 API Key 或服务器密钥**。CLI 状态输出不展示 SDK 配置。
- 默认 `sessionSampleRate=100`，适合接入验收；生产采样率显式设置为 0–100。会话重放、错误补偿、普通 RUM 压缩上报和 Trace Header 注入默认关闭，均可按站点配置。完整参数与限制见[配置参考](docs/configuration.md)。
- `init` 拒绝覆盖现有配置；`render` 拒绝覆盖已注册部署。修改 TOML 后运行 `enable` 升级。
- `check` 检查本机能力、受管文件漂移、抽样页面和 CSP；它不会证明浏览器 CORS、SDK 执行或观测云入库成功。
- `probe --path /page` 可检查单页；`probe` 默认检查受管部署版本。`version` 显示实际编译器、平台、SDK 版本及 SHA-256。

## Session Replay

各站点在自己的 `[sites.rum]` 中显式配置，例如：

```toml
[sites.rum]
# 保留原有 applicationId、OpenWay 或 DataKit 等配置。
sessionReplayEnabled = true
sessionReplaySampleRate = 20
```

`sessionReplayEnabled` 默认 `false`，关闭时即使填写了采样率也不会启动录制。`sessionReplaySampleRate` 为 0–100 的有限数值，支持小数；省略时为 100，仅在开关开启时生效；普通与错误回放采样率都为 0 且关闭远程配置时不启动录制。SDK 先选择 RUM 会话，再决定回放；错误补偿为 0、两者为 50 和 20 时，预期约 10% 的全部会话录制回放。错误补偿采样默认关闭，可分别配置 `sessionOnErrorSampleRate` 和 `sessionReplayOnErrorSampleRate`；后者仍受回放总开关控制。

修改后对目标站点运行 `enable`（包含预检，多站点用 `--site` 选择），会生成新的初始化脚本 URL；`rollback` 可恢复上一版设置。SDK 的采样结果在当前会话内保持，修改采样率后应使用新会话验收。已打开页面和已缓存 HTML 不会被服务器配置即时更新；关闭后需清理 HTML 缓存并重新加载页面，详见[操作手册](docs/operations.md)。

默认使用 `defaultPrivacyLevel="mask"` 遮罩，可按站点调整隐私、操作采集、会话存储、Trace、Visitor ID、请求压缩和 Canvas 2D 录制等参数。`init --full` 与[完整模板](examples/full.toml)列出全部可静态配置项；字段含义、默认值、Worker/CSP 条件、URL 匹配格式及需要业务代码的能力见[完整配置参考](docs/configuration.md)。工具不自动放宽 CSP，外部 Worker 需由运维同域托管并匹配 SDK 版本。

回放与普通上报压缩由 Worker 完成。Blob Worker 的有效 CSP 需显式允许 `blob:`；自托管 Worker 则检查其 URL 和可达性。远程配置可能覆盖静态 SDK 参数，因此预检保守检查其潜在 Worker 需求。

## 注入边界

默认在首个字面量 `</head>` 前插入异步外部脚本，仅改写 HTML；支持 UTF-8 等 ASCII 兼容编码。Nginx / Apache 限定 GET / HTTP 200，IIS 前置条件限定 GET / HTML。三个服务器均跳过专用资源路径、下载和非 HTML；已压缩源响应不注入。非完整 HTML、UTF-16/32、流式 SSR 不在首版范围。

异步脚本不会阻塞业务解析。初始化前的错误、请求和交互不保证采集。脚本设置页面全局防重标记；发现业务已有 `DATAFLUX_RUM` 时不接管。双层服务器重复注入会被 HTTP 探测识别，应只在最终返回 HTML 的一层启用。

不修改 CSP，不注入 inline script，不放开 `unsafe-inline`。检查所有 CSP 响应头与 meta 策略；nonce-only、strict-dynamic、Trusted Types 等需要站点专门适配的策略会被拒绝。SDK 托管同域不代表数据上报也同域；浏览器仍需能访问并获准连接上报端点。

更多部署、缓存和恢复说明见[操作手册](docs/operations.md)。

## 构建与验证

Windows 发布包携带完整 IIS 验收脚本及 ASP.NET 测试夹具。在独立 Windows VM 以管理员运行；旧 Windows 选择 legacy 包，步骤和矩阵见 [Windows/IIS 验收手册](docs/windows-acceptance.md)：

~~~powershell
.\scripts\windows-smoke.ps1 -Binary .\rumable.exe -ExpectedIISVersion 10.0 -ResultDir C:\RumableResults\iis10-01
~~~

覆盖三种压缩基线、规则继承、冲突、幂等启停、升级回滚和失败恢复，默认清理临时站点。退出码 0 / 1 / 2 分别表示通过 / 失败 / 部分验收；StaticOnly 不能算作完整通过。

```sh
go test ./...
go vet ./...
go build -o rumable ./cmd/rumable

# 已有 OrbStack Docker、Go、curl；不安装浏览器测试框架。
scripts/smoke-nginx.sh
scripts/smoke-apache.sh
NGINX_IMAGE=nginx:1.14-alpine scripts/smoke-nginx.sh
# 1.9.4 旧镜像格式不兼容当前 Docker，使用固定摘要的官方源码构建。
docker build -f scripts/nginx-1.9.4.Dockerfile -t rumable-nginx:1.9.4 .
NGINX_IMAGE=rumable-nginx:1.9.4 scripts/smoke-nginx.sh
```

`KEEP=1 scripts/smoke-nginx.sh` 保留本地验证站点，便于本机 Chrome 检查交互和事件；脚本打印需要清理的容器、网络和临时目录。接收端仅用于证明浏览器发送事件，不能替代观测云入库验收。

`RUMABLE_BINARY=/absolute/path/to/linux/rumable` 可替换脚本内默认构建的 CLI，用同一流程验收发布二进制；测试夹具仍需本机 Go 构建。二进制架构需能在所用 OrbStack 环境执行。

```sh
# 同份源码，两个真实编译器；仅设置 go.mod 不会让新 Go 支持旧 Windows。
# 默认先同步官方最新稳定 v3 SDK，再运行两套编译器的测试、vet 和打包。
go run scripts/release.go --go /path/to/go1.26.x/bin/go \
  --legacy-go /path/to/go1.20.14/bin/go --version 0.1.0-preview.2
```

发布目录默认为 `dist/0.1.0-preview.2/`，包括下载索引、最终用户指南和 10 个 Windows / Linux 包。每包携带二进制、示例、手册、SDK 元数据和许可证；目录中的 `SHA256SUMS` 校验压缩包和附带文档。`build.json` 记录源码提交、架构、编译器与二进制摘要。IIS 及旧 Windows 的真实运行验证需在对应 Windows 主机进行，交叉编译不代表运行验收。

可单独执行 `go run scripts/sync-sdk.go` 同步并审查 SDK、版本、来源和 SHA-256；CDN 与官方 NPM 稳定版不一致、版本不是稳定 v3 或许可变更时中止。离线重建可显式使用 `release.go --skip-sdk-sync`，其含义是沿用已校验文件，不检查最新版本。站点不会自动更新；分发新二进制后执行 `enable` 升级，默认同域方式保留旧资源供缓存页面和回滚使用。

正式构建自动记录本地 Git HEAD，并要求工作区、暂存区和未跟踪源码均干净；同步更新 SDK 后应先审查、提交，再重新构建，避免将新源码标为旧提交。尚未提交的候选验证可显式使用 `--revision uncommitted`，其 build.json 会如实标记；分发包应使用已提交的源码。发布目录必须不存在；脚本拒绝覆盖旧包，构建完成前再次核对源码未变化。dist 按现有仓库规则不进入 Git，源码和发布验证记录提交到本地仓库。
