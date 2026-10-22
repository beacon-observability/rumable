# Rumable 安装、配置与使用指南

从这里完成 SDK 选择、安装接入、验收和后续维护。普通安装不需要 Git、Go、Node.js 或数据库。安装包示例沿用 **0.1.5**；同步加载及自动发现中的 SDK 配置是当前源码新增能力，需要配套发布包。已发布的旧页面和旧安装包不会随源码修改而更新。

首次接入按第 1–4 节操作；离线或分步安装使用第 5–7 节；已接入站点查第 8–11 节。完整字段查[配置参考](docs/configuration.md)，压缩、缓存和恢复细节查[运维手册](docs/operations.md)，平台条件查[兼容性](docs/compatibility.md)。

## 目录

- [1. SDK 注入、加载与更新](#1-sdk-注入加载与更新)
- [2. 先选择安装方式和环境](#2-先选择安装方式和环境)
- [3. 准备 RUM 参数与网站条件](#3-准备-rum-参数与网站条件)
- [4. 使用页面和自动安装脚本](#4-使用页面和自动安装脚本)
- [5. 手动下载、校验并安装 CLI](#5-手动下载校验并安装-cli)
- [6. 手动配置 Linux / Nginx](#6-手动配置-linux--nginx)
- [7. 手动配置 Windows / IIS](#7-手动配置-windows--iis)
- [8. 配置路径、采样与回放](#8-配置路径采样与回放)
- [9. 检查是否接入成功](#9-检查是否接入成功)
- [10. 修改配置、升级、停用、回滚和卸载](#10-修改配置升级停用回滚和卸载)
- [11. 常见问题与恢复](#11-常见问题与恢复)
- [12. 服务端 OTel（独立可选能力）](#12-服务端-otel独立可选能力)

## 1. SDK 注入、加载与更新

### 1.1 SDK 如何注入

Rumable 通过 Nginx 的 HTML 替换或 IIS 的出站重写，在网站响应的标准 `<head>` 后加入脚本。业务源代码无需修改。CLI 执行完即退出；SDK 在访问者浏览器运行，直接向配置的 OpenWay / DataKit 上报。

SDK 来源和加载方式是两个独立选项：

| 选择 | SDK 从哪里下载 | 执行时机 |
|---|---|---|
| 网站同域 + 异步（默认） | 安装包内置 SDK，由网站同域提供 | 初始化脚本异步下载 SDK，完成后初始化 |
| 网站同域 + 同步 | 同上 | 先执行 SDK，再执行初始化脚本，随后继续解析页面 |
| CDN + 异步 | 官方或自建 CDN | 同域初始化脚本异步下载 CDN SDK，完成后初始化 |
| CDN + 同步 | 官方或自建 CDN | 先执行 CDN SDK，再执行同域初始化脚本，随后继续解析页面 |

同域资源位于网站的 `__rumable__/<站点ID>/` 下，以 `sdk.<摘要>.js`、`init.<摘要>.js` 命名。SDK 来源不改变数据上报地址；同域托管也不等于同域上报。

### 1.2 选择异步或同步

**异步**适合优先避免脚本下载阻塞 HTML 解析的站点。初始化脚本带 `async`，随后动态异步加载 SDK；初始化完成前的错误、请求和交互可能无法采集。

**同步**适合希望 SDK 在后续业务脚本之前初始化的站点。页面按顺序插入 SDK、初始化脚本两个外部普通脚本，不带 `async` 或 `defer`。两次加载和执行会阻塞 HTML 解析，网络慢或 CDN 故障可能延迟首屏，需评估后选择。同步也不保证 SDK 自身加载失败或初始化之前的事件被采集。

两种方式都不注入内联 JavaScript，不自动放宽 CSP。SDK 加载/初始化失败不会主动抛出初始化脚本异常；同步请求仍需等浏览器完成或判定失败后才继续。CDN 失败不会自动切换到本地副本。

### 1.3 在页面或 TOML 中配置

页面的“选择 SDK 来源与加载方式”支持同域 / CDN、异步 / 同步。右侧摘要与下载的 TOML 同步变化；切回同域时不会携带隐藏的 CDN 地址或 SRI 设置。

完整站点 TOML 在 `[sites.rum]` 同级增加：

```toml
[sites.sdk]
source = "bundled" # 网站同域；使用 CDN 时改为 "cdn"
loading = "async"  # 同步改为 "sync"
```

官方 CDN 示例：

```toml
[sites.sdk]
source = "cdn"
url = "https://static.guance.com/browser-sdk/v3/dataflux-rum.js"
loading = "sync" # 也可使用 "async"
integrity = false
```

自动发现 TOML 使用 `[sdk]`，参数相同；保留原有 `schemaVersion`、`server`、`[rum]`，通过 `setup --discover` 应用。不要把 `[sites.sdk]` 直接追加到发现配置，也不要把发现配置直接交给 `enable`。

省略 SDK 配置时仍是同域 + 异步。页面为了兼容旧安装包，默认组合不输出 SDK 表；异步模式省略 `loading`。同步命令要求清单声明 `rum-sdk-loading-v1`；自动发现时使用 CDN / 同步还要求 `rum-discovery-sdk-v1`。页面会阻止生成不兼容的命令，但可下载 TOML；使用下载的配置前也必须确认 CLI 支持对应字段。

CDN 地址必须是完整 HTTPS 脚本地址，不含账号密码、查询参数或片段。启用及探测时，远程内容必须与安装包的 SDK 摘要一致，自建 CDN 也一样。CSP 需允许实际 SDK 地址。`integrity=true` 会启用 SRI，跨域时需 CDN 允许 CORS；官方 v3 地址默认关闭，2026-10-22 核对未提供所需的 Access-Control-Allow-Origin 头。

### 1.4 SDK 如何更新和回滚

| 来源 | 更新机制 | 回滚能力 |
|---|---|---|
| 网站同域 | 固定随 Rumable 安装包提供；安装新包并对原站点重新执行 enable/setup 才升级 | 保留旧脚本和资源，支持恢复上一成功部署 |
| 官方可变 CDN | URL 不变，CDN 更新后浏览器可能在缓存过期后取得新版，不等待本机升级 | 只能恢复历史 URL，不能恢复被覆盖的远程文件 |
| 自建固定 CDN | 运维发布与安装包一致的固定 URL，再更新配置并应用；可配合 SRI 锁定内容 | 需要历史 URL 和文件仍然可用 |

同域版本更新由发布维护者先同步官方 SDK、核对稳定 v3 版本与许可、测试并提交，再发布新 Rumable 包。CI 使用已提交的 SDK 和摘要，不会在每次重试时追随最新 CDN。安装用户按[升级步骤](#102-升级-cli--sdk)应用新包；只替换 CLI、读取 latest 清单或刷新配置页都不会自动升级已部署站点。

可变 CDN 更新后，旧安装包的 `check/enable/probe` 可能因摘要不一致失败，此时需要升级到匹配的新包或使用匹配的固定 CDN 地址。正在打开的页面不会热替换 SDK。升级后处理旧 HTML 缓存，并用新页面和新会话验证实际采集与回放。

页面里的“应用版本”是业务标签，`remoteConfiguration` 调整采集参数，两者都不更新 SDK 文件。外置 Worker 需自行保持与实际 SDK 版本匹配。

## 2. 先选择安装方式和环境

| 你的情况 | 选择 |
|---|---|
| 服务器能访问 OSS，使用常见网站配置 | 第 4 节：页面生成一键安装命令 |
| 不希望在服务器在线下载安装器，或需要逐步审查配置变化 | 第 5 节安装 CLI，再按第 6 / 7 节操作 |
| 服务器不能访问外网 | 在联网机器下载完整压缩包及 SHA256SUMS，传到服务器校验后手动安装 |
| Nginx 使用自定义启动参数、首页跳转或需要指定验收路径 | 页面高级手动模式，或完整 TOML 手动配置 |
| 只需要服务端 Trace，不需要浏览器 RUM | 第 12 节；页面和 setup 当前只处理 RUM |

“手动安装 CLI”和“手动修改 Nginx 配置”是两回事。手动下载包之后，仍可让 `attach` 自动修改 Nginx 配置；只有自动挂接不支持的布局，才需要按生成的 `ATTACH.md` 人工修改。

| 环境 | 当前接入范围 |
|---|---|
| Linux + Nginx | 支持手动流程、自动站点发现和一键安装 |
| Windows + IIS | 已实现手动流程、一键安装和系统识别；仍需在目标 Windows 实机验收 |
| Windows + Nginx / macOS + Nginx | 不属于当前一键安装与自动挂接支持范围 |
| 真正 32 位 Windows | 有 x86 CLI 包，但不能执行 IIS 管理和一键接入 |

Windows 一键脚本要求 **管理员、PowerShell 5.1+、64 位 Windows**。手动 CLI 的 IIS 桥接基线为 **64 位 Windows PowerShell 3+**，推荐 5.1；旧系统必须选择 legacy 包。Nginx 要求 **1.9.4+ 且带 http_sub_module**；IIS 要求 **7.5+ 和 URL Rewrite 2.x**。安装脚本不安装这些服务器或组件。

本项目的 Linux ARM64 / Nginx 已完成实际 OSS 下载、注入和启停回滚验证；不能由此推断 Windows、其他架构或真实 RUM 入库已经通过。平台细节见[兼容性说明](docs/compatibility.md)，验证范围见[验证记录](docs/verification.md)。

## 3. 准备 RUM 参数与网站条件

### 3.1 从观测云 Web 应用取得参数

| 参数 | 填什么 | 是否必填 |
|---|---|---|
| applicationId | 要接入的 Web 应用 ID | 必填 |
| site | OpenWay 的 HTTP(S) 地址，例如 `https://openway.example.com` | OpenWay 模式必填 |
| clientToken | 该应用的浏览器 Client Token | OpenWay 模式必填 |
| datakitOrigin | 浏览器可访问的 DataKit 地址，例如 `https://datakit.example.com` | DataKit 模式必填 |
| service / env / version | 业务服务名、运行环境和业务版本 | 建议填写，便于查询区分 |

OpenWay 和 DataKit **二选一**。这里的 `site` 是上报端地址，不是业务网站地址，也不是 IIS 站点名称。上报地址只填写协议、主机和可选端口，不加 `/v1/write/rum`、查询参数或 Token；SDK 自己拼接请求路径。

HTTPS 网站需要 HTTPS 上报地址。地址必须从访问者的浏览器可达，仅服务器可达不够。Client Token 会随初始化脚本提供给浏览器；不要使用管理 API Key、SSH 密钥或服务端凭据。

本文的 `REPLACE_...`、`example.com` 都是占位值，必须替换才能验证真实数据。任意填写 appId/token 只能用于检查是否发出请求，不能当作接入真实应用成功。

### 3.2 确认网站可被检查

- 网站已运行。执行 CLI 的服务器能够通过网站地址访问目标实例，DNS、端口和 TLS 证书正常。负载均衡/CDN 后的网站还需保证验收请求确实到达本次修改的实例，不能命中其他节点或旧 HTML 缓存。
- 至少准备一个无需登录、直接返回 **HTTP 200、text/html、完整 HTML** 的验收页面。CLI 不跟随 301/302、不携带登录 Cookie。首页跳转时改用实际页面路径。
- 页面开头有标准 `<head>` 标签。`<HEAD>` 可以；`<head lang="...">`、`<head >` 或标签前的注释/属性中先出现 `<head>` 文本不符合当前注入检查。
- 每种页面模板及实际 HTML handler 至少有一个验收路径。只验证首页不能证明其他模板都可注入。
- 站点尚未通过其他方式接入同一 SDK。工具能识别常见已有接入，但不能识别所有藏在业务 bundle 中的动态初始化；需要同时检查业务现状。
- CSP 允许初始化脚本、SDK、上报地址以及所需 Worker；工具不会自动放宽 CSP。
- 源 HTML 不能已经是 gzip/br 压缩内容。Nginx 输出 gzip 可保留；代理源压缩、预压缩静态文件和应用中间件压缩需按检查结果处理。

### 3.3 区分三个安装位置

| 内容 | Linux 一键安装默认位置 | Windows 一键安装默认位置 |
|---|---|---|
| CLI | `/opt/rumable/0.1.5/rumable` | 原生 `%ProgramFiles%\Rumable\0.1.5\rumable.exe` |
| 运行状态、备份、版本化脚本 | `/var/lib/rumable` | `%ProgramData%\Rumable` |
| 你维护的 TOML | 页面下载后自行保存，例如 `/etc/rumable/website.toml` | 页面下载后自行保存，例如 `C:\ProgramData\RumableConfig\website.toml` |

安装器只解压 CLI；完整文档和示例在平台 ZIP/TAR 包中。安装器把命令中的配置解码到临时文件，结束后删除临时文件，**不会替你维护一份长期 TOML**。请保存页面下载的配置，以及版本、站点 ID、目标网站和状态目录。

状态目录不能放到网站根目录。它不等同于缓存，不能通过删除它来“重装”。同一个站点的后续命令始终使用同一个状态目录和接入标识。

## 4. 使用页面和自动安装脚本

### 4.1 打开页面并填写配置

打开 [latest 配置页](https://static.guance.com/rumable/latest/setup/index.html)，需要固定使用 0.1.5 时打开[固定版本页](https://static.guance.com/rumable/0.1.5/setup/index.html)。使用 `static.guance.com`；不要把页面链接替换成 OSS 原始域名，后者可能强制下载 HTML。本地 `file://` 页面可能无法正常加载模块或清单，优先使用线上页。

1. 选择 OpenWay 或 DataKit，填写 applicationId 和对应上报参数。
2. 填写 service、env、业务 version。初次验证建议会话采样 100%，回放先关闭；需要回放时按第 8 节单独开启并验收。
3. 在“选择 SDK 来源与加载方式”中选择网站同域 / CDN、异步 / 同步。CDN 默认填入官方 v3 地址；自建 CDN 可修改地址，按需开启 SRI。先阅读[加载与更新规则](#1-sdk-注入加载与更新)。
4. 选择 **Linux · Nginx** 或 **IIS · Windows**。默认不必填写网站 URL、IIS 名称或 Nginx 路径，安装时在服务器发现。
5. 确认页面已读取发布清单，并显示 Rumable 与 Web SDK 版本。清单读取失败时不会生成可复制的安装命令。
6. 下载 TOML 留存，切换到安装命令并复制完整内容。生成与复制均不会改动服务器；执行命令才开始接入。

页面只在本地浏览器序列化 RUM 和 SDK 加载参数，没有把配置提交到 OSS 的步骤。命令中的 Base64 是 TOML 的编码，**不是加密**，用来避免引号、换行等被终端改变。不要把包含接入令牌的完整命令贴到公开工单。

### 4.2 Linux 执行

登录实际运行 Nginx 的 Linux 环境，准备 root/sudo 权限和 `curl`、CA 证书、`sha256sum`、`tar`、`base64` 等基础工具。自动脚本识别 x86、x64、ARM64，选择现代 Linux 包；它不会按内核版本改选 legacy 包，旧内核走第 5 节。

页面生成的命令已经自带 Bash `pipefail`，脚本下载失败和安装失败都会返回非零。服务器需安装 Bash；不必额外设置当前终端。仍须检查成功日志、status 和 probe。

自动命令的结构如下，**请执行页面生成的实际命令，不要直接执行占位示例**：

```sh
bash -o pipefail -c 'curl --proto "=https" --proto-redir "=https" -fsSL "https://static.guance.com/rumable/0.1.5/install.sh" | sudo sh -s -- --base-url "https://static.guance.com/rumable/0.1.5/" --config "页面生成的Base64配置" --discover'
```

脚本下载同版本压缩包，校验内置 SHA-256，安装 CLI，再执行自动发现和接入。单个站点/地址直接选择；多个候选输入编号；通配名称无法确定访问地址时需输入可访问 URL。Nginx 来自 PATH 或标准位置，使用默认配置；不会猜测当前服务自定义的 `-c/-p`。

`--yes` 仅确认接入变更，不能代替站点或地址选择。无交互终端且候选不唯一时应停止，不会默认修改第一个站点。容器中的 Nginx 应在对应容器/配置命名空间内接入，并持久化配置及状态卷；不要在宿主机安装器里猜测另一个容器的目录。

### 4.3 Windows 执行

以管理员身份打开 Windows PowerShell 5.1，优先使用系统原生 64 位版本。粘贴页面生成的 **PowerShell 命令**，不要粘贴 Linux 的 curl/sh 命令，也不要在 cmd.exe 中直接运行。

```powershell
# 查看当前环境；这不是安装命令。
$PSVersionTable.PSVersion
[Environment]::Is64BitOperatingSystem
[Environment]::Is64BitProcess
```

安装器自动识别 Windows 实际版本与原生 CPU 架构，选择 x64/ARM64 和 modern/legacy 包。64 位系统上的 32 位 PowerShell 仍安装原生包，使用 ProgramW6432 定位原生 Program Files；真正的 32 位 Windows 在下载前拒绝 IIS 接入。ARM64 与旧 Windows 的实机兼容需要独立验证。

站点发现只列出运行中的 HTTP(S) 网站及应用。选择你的测试目标；多个绑定时选择对应 URL；无明确主机名的通配绑定可能需要输入 URL。如果静态压缩已开启，按提示确认是否允许关闭所选应用的静态压缩，或在页面高级选项中预先同意。

0.1.5 会隔离私有状态文件，并查询所选 IIS 应用的匿名读取身份和应用池身份：只开放公共脚本读取和父目录遍历。已有状态目录不会整体重置 ACL。安装器仍不调整匿名认证、StaticFile handler 或业务入站重写例外；401/403 等错误按第 7 节诊断。

不要给整个 `%ProgramData%\Rumable` 递归授予 Everyone/IIS 读取或写入权限。自动化测试可在页面命令末尾增加 `-NonInteractive`；候选不唯一或还需补充信息时立即报错，不等待选择。它不能代替目标选择或压缩许可。

### 4.4 哪些情况启用高级手动模式

展开高级选项，勾选“手动指定站点与路径”，再填写：

| 字段 | 示例及含义 |
|---|---|
| 网站地址 | `https://www.example.com/`；IIS 子应用用 `https://www.example.com/shop/` |
| 接入标识 | `website`；后续更新、停用和回滚保持不变 |
| 注入路径 | 每行一个路径前缀，例如 `/shop/` |
| 排除路径 | 每行一个路径前缀，例如 `/shop/downloads/`，排除优先 |
| 验收路径 | 真实返回 HTML 200 的完整 URL 路径，例如 `/shop/index.html` |
| Nginx 程序/配置 | `/usr/sbin/nginx` 与实际配置文件；额外 `-p/-e` 需直接编辑完整 TOML |
| IIS 网站/应用 | IIS 管理器中的网站名与 `/` 或 `/shop`，不是物理目录 |

自动模式下载的是 `schemaVersion + server + [rum]` 的**发现配置**，只能给 `setup --discover` 使用；高级手动模式和 `init` 产生 `[[sites]]` 的**完整站点配置**，可用于 render/check/enable/setup。两种配置不能混用。

如果完整 Nginx 配置仍有多个匹配候选，页面命令无法携带 `--target` 选择，改用第 6 节的分步 attach。自动挂接不支持的复杂 handler 也需人工审查。

### 4.5 成功后保存信息并检查状态

终端应打印 `Site ID: auto-...`（自动模式）和 `RUM setup complete`。以实际输出替换下方 ID；不要把占位 ID 当作真实值。

Linux：

```sh
sudo /opt/rumable/0.1.5/rumable status --state-dir /var/lib/rumable --site auto-替换为实际ID
sudo /opt/rumable/0.1.5/rumable probe --state-dir /var/lib/rumable --site auto-替换为实际ID
```

Windows：

```powershell
$programs = $env:ProgramFiles
if ([Environment]::Is64BitOperatingSystem -and -not [Environment]::Is64BitProcess) { $programs = $env:ProgramW6432 }
$rumable = Join-Path $programs 'Rumable\0.1.5\rumable.exe'
$state = Join-Path $env:ProgramData 'Rumable'
$siteId = 'auto-替换为实际ID'
& $rumable status --state-dir $state --site $siteId
& $rumable probe --state-dir $state --site $siteId
```

RUM 接入成功应看到 `rumEnabled=true`、`drift=""`、`pendingTransaction=false`，probe 成功。脚本成功只覆盖 HTTP 注入和资源可达，继续按第 9 节检查浏览器与真实数据。

## 5. 手动下载、校验并安装 CLI

### 5.1 下载入口与选包

固定版本根地址为 `https://static.guance.com/rumable/0.1.5/`。直接下载下表文件，**无需 OSS 目录列表或 Git 权限**。清单和校验文件分别为 [manifest.json](https://static.guance.com/rumable/0.1.5/manifest.json)、[SHA256SUMS](https://static.guance.com/rumable/0.1.5/SHA256SUMS)。

| 目标 | 包名（点击下载） |
|---|---|
| Linux x64 | [rumable_0.1.5_linux_amd64.tar.gz](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_linux_amd64.tar.gz) |
| Linux ARM64 | [rumable_0.1.5_linux_arm64.tar.gz](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_linux_arm64.tar.gz) |
| Linux x86 | [rumable_0.1.5_linux_386.tar.gz](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_linux_386.tar.gz) |
| 旧 Linux x64 | [rumable_0.1.5_linux_amd64_legacy.tar.gz](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_linux_amd64_legacy.tar.gz) |
| 旧 Linux x86 | [rumable_0.1.5_linux_386_legacy.tar.gz](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_linux_386_legacy.tar.gz) |
| Windows x64 | [rumable_0.1.5_windows_amd64.zip](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_windows_amd64.zip) |
| Windows ARM64 | [rumable_0.1.5_windows_arm64.zip](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_windows_arm64.zip) |
| Windows x86 CLI | [rumable_0.1.5_windows_386.zip](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_windows_386.zip) |
| 旧 Windows x64 | [rumable_0.1.5_windows_amd64_legacy.zip](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_windows_amd64_legacy.zip) |
| 旧 Windows x86 CLI | [rumable_0.1.5_windows_386_legacy.zip](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_windows_386_legacy.zip) |

`386` 是 x86 32 位，`amd64` 是 Intel/AMD x64，`arm64` 是 ARM64。现代 Windows 包面向 Windows 10 / Server 2016+；Windows 7 SP1 / Server 2008 R2 SP1、2012/2012 R2 等旧系统选 legacy。现代 Linux 包的内核基线为 3.2，legacy x86/x64 的基线为 2.6.32，仍须符合架构限制并实际验证。legacy 使用 Go 1.20.14，没有 legacy ARM64 包。详见[兼容矩阵](docs/compatibility.md)。

### 5.2 Linux 下载与安装示例

以下在 **Bash** 执行，以 x64 为例；ARM64、x86 或 legacy 请先改 `package`。若该版本已经安装，先核对现有版本和来源，跳过本节，勿覆盖已有文件。

```bash
set -euo pipefail
version=0.1.5
base="https://static.guance.com/rumable/$version"
package="rumable_${version}_linux_amd64.tar.gz"
work=$(mktemp -d)
cd "$work"
curl --proto '=https' --proto-redir '=https' -fsSL "$base/$package" -o "$package"
curl --proto '=https' --proto-redir '=https' -fsSL "$base/SHA256SUMS" -o SHA256SUMS
expected=$(awk -v f="$package" '$2 == f {print $1}' SHA256SUMS)
[[ "$expected" =~ ^[a-f0-9]{64}$ ]] || { echo '校验文件缺少唯一的包摘要'; exit 1; }
printf '%s  %s\n' "$expected" "$package" | sha256sum -c -

install_dir="/opt/rumable/$version"
sudo mkdir -p /opt/rumable
sudo mkdir "$install_dir"  # 已存在则停止，不覆盖原安装。
sudo tar -xzf "$package" -C "$install_dir"
sudo "$install_dir/rumable" version
```

只下载一个包时，仅校验该包；不要直接对完整 SHA256SUMS 执行 `sha256sum -c SHA256SUMS`，否则其他未下载文件会报缺失。校验完成后可清理本次下载临时目录，保留安装目录中的文档、示例和 `build.json`。

离线服务器从下载步骤之后继续，使用传入的原始压缩包和同版本 SHA256SUMS；安装无需访问 Git 或编译环境。CLI 仍需要访问实际网站做部署检查，浏览器上报另需能访问接收端。

### 5.3 Windows 下载与安装示例

在管理员 Windows PowerShell 5.1+ 执行。先按上一表选择包名；下面明确使用 x64 现代包，**不是自动识别脚本**。

```powershell
$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$version = '0.1.5'
$base = "https://static.guance.com/rumable/$version"
$package = "rumable_${version}_windows_amd64.zip"
$work = Join-Path $env:TEMP ('rumable-download-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
Invoke-WebRequest -UseBasicParsing "$base/$package" -OutFile (Join-Path $work $package)
Invoke-WebRequest -UseBasicParsing "$base/SHA256SUMS" -OutFile (Join-Path $work 'SHA256SUMS')
$pattern = '^[a-f0-9]{64}  ' + [regex]::Escape($package) + '$'
$lines = @(Get-Content (Join-Path $work 'SHA256SUMS') | Where-Object { $_ -match $pattern })
if ($lines.Count -ne 1) { throw '校验文件缺少唯一的包摘要' }
$expected = ($lines[0] -split '\s+')[0]
$actual = (Get-FileHash (Join-Path $work $package) -Algorithm SHA256).Hash
if ($actual -ine $expected) { throw '安装包 SHA-256 不匹配，停止安装' }
$programs = $env:ProgramFiles
if ([Environment]::Is64BitOperatingSystem -and -not [Environment]::Is64BitProcess) { $programs = $env:ProgramW6432 }
$install = Join-Path $programs "Rumable\$version"
if (Test-Path -LiteralPath $install) { throw '版本目录已存在，请先检查现有安装，不覆盖' }
Expand-Archive -LiteralPath (Join-Path $work $package) -DestinationPath $install
$rumable = Join-Path $install 'rumable.exe'
& $rumable version
if ($LASTEXITCODE -ne 0) { throw 'CLI 版本检查失败' }
```

仅有 PowerShell 3 的旧系统，可在其他机器下载匹配的 legacy 包与 SHA256SUMS，用 `certutil -hashfile 文件名 SHA256` 比较该行摘要，再使用资源管理器解压；不要为了直接运行上述下载示例而装错现代包。读取 `version` 和 `build.json` 核对实际版本、CPU、编译器及二进制摘要。

## 6. 手动配置 Linux / Nginx

### 6.1 准备固定目录和配置

以下在目标 Linux 的 root 会话执行。`config`、`state` 和 `site_id` 是后续操作的固定值；示例使用 `website`，多网站请使用不同 ID、不同配置文件，但共用同一受管状态目录。

```sh
sudo -i
umask 022
rumable=/opt/rumable/0.1.5/rumable
config=/etc/rumable/website.toml
state=/var/lib/rumable
site_id=website
install -d -m 0700 /etc/rumable
mkdir -p "$state"
"$rumable" init --server nginx --reporting openway --site "$site_id" --config "$config"
```

`init` 仅生成新文件，拒绝覆盖已有配置。用编辑器修改 `$config`，完整最小示例如下：

```toml
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
# 自定义配置/前缀时使用实际启动参数：
# args = ["-c", "/etc/nginx/nginx.conf", "-p", "/etc/nginx/"]

[sites.rum]
applicationId = "REPLACE_APPLICATION_ID"
site = "https://REPLACE_OPENWAY_HOST"
clientToken = "REPLACE_CLIENT_TOKEN"
service = "website"
env = "test"
version = "1.0.0"
sessionSampleRate = 100
sessionReplayEnabled = false
defaultPrivacyLevel = "mask"
```

DataKit 模式在同一个 `[sites.rum]` 表内删除 `site` / `clientToken`，填写 `datakitOrigin = "https://REPLACE_DATAKIT_HOST"`；其余不变，也可从 `init --reporting datakit` 开始。

`binary` 是 Nginx 程序绝对路径，不是 systemctl 或启动 shell 脚本。使用该程序和相同 `args` 检查 `-V`、`-t`；确保有 `--with-http_sub_module`，且读取的是实际服务配置。

### 6.2 生成、预览、挂接和启用

每一步成功后再执行下一步：

```sh
"$rumable" render --config "$config" --state-dir "$state" --site "$site_id"
"$rumable" attach --state-dir "$state" --site "$site_id" --dry-run
"$rumable" attach --state-dir "$state" --site "$site_id"
# 审查变更与备份路径，输入 yes。
"$rumable" enable --config "$config" --state-dir "$state" --site "$site_id"
"$rumable" status --state-dir "$state" --site "$site_id"
"$rumable" probe --state-dir "$state" --site "$site_id"
```

- `render` 生成停用状态、公共脚本和操作说明，不改业务 Nginx 配置。
- `attach --dry-run` 展示拟修改的真实文件及差异；`attach` 确认后备份、写入 include、检查配置、重载并检查资源，仍不启用注入。
- `enable` 预检并启用注入，包含配置检查、平滑重载和 HTTP 验证；失败尝试恢复前一部署。
- 独立 `check --config "$config" --state-dir "$state" --site "$site_id"` 可在挂接后提前诊断，不代替 enable。

多个匹配的 server 时，按实际候选列表指定目标，不照抄示例行号：

```sh
"$rumable" attach --state-dir "$state" --site "$site_id" --target /etc/nginx/sites-available/website:12 --dry-run
"$rumable" attach --state-dir "$state" --site "$site_id" --target /etc/nginx/sites-available/website:12
```

### 6.3 需要人工修改 Nginx 时

打开 `$state/sites/$site_id/ATTACH.md`。将生成的 `http.conf`、`server.conf`、`location.conf` 分别 include 到现有 `http`、目标 `server`、各实际 HTML handler。不要创建与业务竞争的 location，不替换原有 root/proxy_pass/try_files。

保留全部业务请求头与继承关系，按 ATTACH.md 调整源压缩、条件请求、缓存及过滤规则。自行保存原配置备份，使用真实 binary/args 执行 `nginx -t`，成功后重载。完成审查后，才在 TOML 将 `attachmentReviewed = true`，然后执行 enable。

人工挂接没有 Rumable 自动 attach 的原文件备份记录：`rollback --attachment` 不能自动撤销手工修改。卸载时先 disable、处理旧 HTML 缓存，再由你移除本次手工 include 并恢复相关业务指令，配置检查和重载成功后才归档受管状态。不要直接删除仍被 Nginx include 的目录。

自动挂接对代理 handler 调整的 Accept-Encoding、条件请求头和缓存等，在普通 disable 后仍可能保留，直到完整撤销挂接。影响范围是实际 handler，不仅限于 includePaths 中的 HTML。完整解释见[部署与恢复](docs/operations.md)。

## 7. 手动配置 Windows / IIS

### 7.1 确认网站与应用

在 IIS 管理器记录网站名称、应用路径和实际匿名文件读取身份。根应用路径是 `/`；子应用 `/shop` 对应 `publicURL=https://www.example.com/shop/`，路径配置使用 `/shop/`、`/shop/index.html`，不是物理磁盘目录。

以管理员身份使用系统原生 64 位 Windows PowerShell。URL Rewrite 2.x、Static Content、匿名访问和 JavaScript MIME 应已可用。全路径业务路由、ANCM 或入站重写需允许应用下 `/__rumable__/` 由静态文件处理器提供；虚拟目录本身不会自动覆盖业务 handler。

### 7.2 生成和修改配置

沿用第 5.3 节 `$rumable`；新终端需重新指向实际安装的 exe。

```powershell
$ErrorActionPreference = 'Stop'
$state = Join-Path $env:ProgramData 'Rumable'
$configDir = Join-Path $env:ProgramData 'RumableConfig'
$config = Join-Path $configDir 'website.toml'
$siteId = 'website'
New-Item -ItemType Directory -Path $configDir -Force | Out-Null
& $rumable init --server iis --reporting datakit --site $siteId --config $config
if ($LASTEXITCODE -ne 0) { throw '配置生成失败；已有文件请直接编辑' }
notepad.exe $config
```

将文件保存为 UTF-8，不使用 PowerShell 默认 UTF-16 重定向覆盖 TOML。完整配置示例：

```toml
schemaVersion = 1

[[sites]]
id = "website"
publicURL = "https://www.example.com/"
includePaths = ["/"]
excludePaths = ["/downloads/"]
probePaths = ["/"]

[sites.iis]
site = "Default Web Site"
application = "/"
allowCompressionChanges = false

[sites.rum]
applicationId = "REPLACE_APPLICATION_ID"
datakitOrigin = "https://REPLACE_DATAKIT_HOST"
service = "website"
env = "test"
version = "1.0.0"
sessionSampleRate = 100
sessionReplayEnabled = false
defaultPrivacyLevel = "mask"
```

OpenWay 模式将 `datakitOrigin` 替换为 `site` 和 `clientToken`。如果所选应用开启静态压缩，确认影响后把 `allowCompressionChanges` 改为 true；不允许时启用会被拒绝，不会自动替你同意。启用期间该应用全部静态资源可能失去静态压缩；停用恢复原局部值/继承关系。

### 7.3 准备状态与公共脚本权限

0.1.5 的 render 会保护新站点目录和私有文件；enable 会自动识别匿名读取身份与应用池身份，给 public 子树只读权限、父目录遍历权限。通常无需手工执行 icacls：

```powershell
& $rumable render --config $config --state-dir $state --site $siteId
if ($LASTEXITCODE -ne 0) { throw '生成部署失败；已有状态请直接检查，不重复初始化' }
```

已有失败现场不要重新 render，先按[旧事务恢复流程](docs/windows-0.1.4-recovery.md)处理。不要重置整个状态目录的 ACL。state.json、transaction.json 和桥接请求应仅允许 Administrators/SYSTEM 访问，网站账户只读取 `public` 中的脚本；配置 TOML 目录同样由管理员维护。

若公共资源仍返回 401/403，检查 IIS 实际匿名身份、NTFS 拒绝规则、Static Content/JavaScript MIME、业务 handler 及 `/__rumable__/` 的入站重写例外。自定义账户或域账户必须能在本机解析。匿名认证关闭时自动接入会明确拒绝，不会修改认证设置。

### 7.4 启用和验收

IIS 不执行 Linux 的 attach。阅读生成的 `ATTACH.md` 后运行：

```powershell
& $rumable check --config $config --state-dir $state --site $siteId
if ($LASTEXITCODE -ne 0) { throw '预检查失败，请先处理提示' }
& $rumable enable --config $config --state-dir $state --site $siteId
if ($LASTEXITCODE -ne 0) { throw '启用失败，请保留状态并检查恢复结果' }
& $rumable status --state-dir $state --site $siteId
& $rumable probe --state-dir $state --site $siteId
```

CLI 管理所选站点/应用的受管规则、资源映射和压缩属性，不执行整台 IIS 重启。check 在首次启用前不能证明尚未创建的资源映射能被匿名读取，enable 后的资源探测仍可能暴露 ACL/handler 问题。下一步必须进行浏览器验收。

## 8. 配置路径、采样与回放

### 8.1 站点与路径

| 字段 | 规则 |
|---|---|
| id | 以小写字母开头，最多 48 个小写字母、数字或短横线；整个生命周期保持一致 |
| publicURL | 业务网站/应用基地址，以 `/` 结尾；部署身份的一部分，不能原地换成另一个网站 |
| includePaths | 区分大小写的 URL 路径前缀，至少一个；`/shop` 也匹配 `/shopping`，目录应写 `/shop/` |
| excludePaths | 排除优先；空数组表示不额外排除业务路径 |
| probePaths | 站点 origin 下的完整路径；必须在注入范围内且不被排除，至少一个 |

例如 `publicURL=https://example.com/shop/`，`probePaths=["/shop/index.html"]` 请求的是 `https://example.com/shop/index.html`，不会再拼一次 `/shop/`。保留资源路径由工具自动排除。

路径规则决定服务器是否在 HTML 响应中注入 SDK，**不等于排除 SDK 上报的 Fetch/XHR 请求**，也不会在 SPA 已加载后切换路由时重新评估。动态用户、业务事件、手动 View 和函数回调仍需业务代码。

### 8.2 采样与会话回放

在已有 `[sites.rum]` 表内修改，不要再声明重复表：

```toml
sessionSampleRate = 100
sessionOnErrorSampleRate = 0
sessionReplayEnabled = true
sessionReplaySampleRate = 20
sessionReplayOnErrorSampleRate = 0
defaultPrivacyLevel = "mask"
```

回放默认关闭。上述表示 RUM 会话采样 100%，普通回放采样 20%；回放仍依赖会话采样。`mask` 是默认隐私等级。回放、普通请求压缩和远程配置可能需要 Worker；默认 Blob Worker 要求有效 CSP 允许 `blob:`，也可配置匹配 SDK 版本的同源 workerUrl。工具不会自动托管外置 Worker 或修改 CSP。

修改后重新 enable / 执行同目标安装命令；用新页面和新会话验收，避免已有会话的采样结果影响判断。旧页面不会被远程关闭录制。

其他静态参数见[配置参考](docs/configuration.md)。SDK 来源与加载方式见[第 1 节](#1-sdk-注入加载与更新)。

## 9. 检查是否接入成功

分四层验收，不以“脚本退出 0”替代全部结果：

| 层次 | 需要的证据 |
|---|---|
| 安装 | version 符合所选版本/架构，包校验通过 |
| 注入 | status 无漂移和未完成事务，probe 成功，实际 HTML 含一个本站 `script[data-rumable]` |
| 浏览器运行 | 初始化脚本和 SDK 返回 200、无非预期 Console 错误、原业务正常、出现 RUM 请求 |
| 后端入库 | 上报响应成功，并在观测云对应应用查到本次访问的事件/字段；有回放时另验回放可播放 |

在真实浏览器打开目标页面，开发者工具 Network 开启 Preserve log，再刷新：

1. 检查 document 响应、`init.<hash>.js` 和 `sdk.<hash>.js`。CDN 模式 SDK 为外部 URL。
2. Console 执行 `document.querySelectorAll('script[data-rumable]').length`，本次页面预期为 1。
3. 检查 `window.DATAFLUX_RUM` 与初始化报错。访问另一页面、点击按钮、发起业务请求，触发实际 SDK 行为。
4. Network 按 `/v1/write/rum` 筛选，等待 SDK 批量发送；检查实际请求与响应，不只看是否生成了 URL。模拟接收端返回 204 只证明测试端收到了请求，不代表观测云已接收。
5. 进入真实应用，根据 env/service、页面 URL 和时间窗口查询；跨域失败、appId/token 错误、采样未命中分别排查。

CLI 不主动证明浏览器 POST 成功，也不查询观测云。没有真实接入参数时，将数据入库标记为“未验证”。验收不要覆盖或清理无关网站的数据。

## 10. 修改配置、升级、停用、回滚和卸载

### 10.1 修改 RUM 参数

**自动模式：** 回到同版本配置页填写新参数，再执行生成的命令，重新选择同一站点。已有 ID、URL 和路径范围从状态复用；保存新下载的发现配置。不要把自动 ID 改成另一个手动 ID 去接管已有部署。

**手动模式：** 编辑原完整 TOML，再运行 enable。不重复 init/render，不删状态目录。

Linux（root 会话，沿用前述变量）：

```sh
"$rumable" enable --config "$config" --state-dir "$state" --site "$site_id"
"$rumable" probe --state-dir "$state" --site "$site_id"
```

Windows（管理员，沿用前述变量）：

```powershell
& $rumable enable --config $config --state-dir $state --site $siteId
& $rumable probe --state-dir $state --site $siteId
```

发现配置使用 setup --discover，不能直接给 enable。SDK 来源与加载方式可在页面调整后重新执行同目标命令；新旧发布包的兼容要求见第 1 节。要改变自动站点的 probePaths，请依据记录的原 ID/URL/目标整理完整 TOML，校对身份后再执行。Nginx binary/args、IIS site/application 和 publicURL 属于部署身份，不支持用普通更新迁移目标。

### 10.2 升级 CLI / SDK

同域 SDK 不自动追新。完整步骤为：备份 → 下载并校验包含目标 SDK 的新包 → 对原站点应用 → 检查版本与实际采集。CDN 地址更新后的缓存、摘要和回滚限制见[第 1 节](#1-sdk-注入加载与更新)。

备份原 TOML、完整状态目录及服务器配置；下载新版本到新目录并校验。将 `$rumable` 指向新版本，继续使用原 state/site/config 执行 enable，或用新版本配置页执行同目标 setup。只替换 CLI 不会立刻改变已部署 SDK；实际 enable/setup 后才更新站点。

升级后用 `rumable version` 查看当前程序内的 SDK 版本和摘要，用原 state/site 执行 `status` 查看已部署快照，再执行 `probe` 并检查浏览器请求与观测云入库。清理 CDN、代理及 Service Worker 的旧 HTML 缓存，以新页面、新会话验收。仅执行 `enable --component otel` 不更新 Web SDK；需要时使用 `enable --component rum`。

保留旧 CLI 和版本化资源。普通 rollback 恢复上一次部署的脚本与配置，不替你卸载新 CLI，也不自动恢复磁盘上的 TOML。回滚后若再次 enable，仍会按 TOML 重新应用，因此需要同步检查期望配置。

### 10.3 停用与回滚

Linux：

```sh
"$rumable" disable --state-dir "$state" --site "$site_id"
"$rumable" probe --state-dir "$state" --site "$site_id"
# 确实需要恢复前一成功状态时，再执行：
"$rumable" rollback --state-dir "$state" --site "$site_id"
```

Windows：

```powershell
& $rumable disable --state-dir $state --site $siteId
& $rumable probe --state-dir $state --site $siteId
# 确实需要恢复前一成功状态时，再执行：
& $rumable rollback --state-dir $state --site $siteId
```

| 操作 | 含义 |
|---|---|
| disable | 停止新 HTML 注入；保留旧脚本和资源映射，Nginx 首次挂接调整不全撤销 |
| rollback | 有中断事务时先恢复事务；否则切回前一个成功部署，可能重新启用 |
| rollback --attachment | 仅用于 Linux Nginx 自动挂接的恢复/撤销，与普通部署回滚不同 |
| uninstall | 恢复受管挂接/设置、移除资源映射并归档状态，之后普通 rollback 不再恢复该站点 |

普通历史仅维护当前与上一次成功快照，重复 rollback 会来回切换，不是不断往更早版本回退。停用不终止用户已经打开页面中的 SDK；旧 HTML 缓存也可能仍加载 SDK，需按浏览器/CDN/代理/Service Worker 的缓存策略处理。

### 10.4 完整卸载

先 disable，确认新页面不再注入，并处理仍引用旧脚本的 HTML 缓存。再预览并确认：

```sh
"$rumable" uninstall --state-dir "$state" --site "$site_id" --dry-run
"$rumable" uninstall --state-dir "$state" --site "$site_id"
```

Windows 对应：

```powershell
& $rumable uninstall --state-dir $state --site $siteId --dry-run
& $rumable uninstall --state-dir $state --site $siteId
```

默认不删除 CLI、外部 Web 服务器、Collector 或用户安装的依赖。状态归档在 state-dir 的 removed 子目录。先确认没有其他站点使用同一 CLI 版本，才手动清理该版本安装目录。

Linux 若只想恢复首次自动挂接但保留站点状态，可在 disable 和清缓存后使用 `rollback --attachment --dry-run` / `rollback --attachment`。外部配置已修改、其他站点后来挂接到同一个文件等情况可能阻止原文件恢复，需审查差异或按挂接顺序反向撤销。人工挂接按第 6.3 节处理。

## 11. 常见问题与恢复

**0.1.5 修复了 0.1.4 Windows 实测中的 IIS 注入与恢复故障。** 保留旧失败现场的用户，先用新版 CLI 按[恢复与复测流程](docs/windows-0.1.4-recovery.md)执行 rollback，再运行新页面的一键命令。原生 Windows / IIS 修复效果仍需在目标机器复测；下载和安装成功不等于注入成功。

| 现象 | 先检查 / 处理 |
|---|---|
| 打开页面变成下载 | 使用 static.guance.com 的 setup/index.html，不使用 OSS 原始域名 |
| 本地双击 HTML 为空或无法读版本 | 改用线上配置页；无需下载源码运行 |
| curl 报错但终端最后退出 0 | 旧 0.1.4 命令的管道退出码问题；改用 0.1.5 页面命令，检查实际安装日志与 status/probe |
| Nginx discovery requires Linux | 当前不支持 Windows/macOS Nginx 自动接入 |
| Nginx not found / 读错配置 | 使用完整 TOML 指定实际 binary、-c/-p；确保执行在真实服务环境 |
| multiple candidates / selection requires an interactive terminal | 使用交互终端选择；自动化环境改成完整目标配置，必要时先 attach --target |
| unknown configuration key: server/rum | 把发现配置当作完整配置使用了；使用 setup --discover 或生成 [[sites]] 配置 |
| expected HTTP 200，实际 301/302/401/403 | 使用直接返回完整 HTML 的验收路径；检查匿名访问、绑定、DNS、TLS 与命中实例 |
| no safe literal <head> | 检查页面模板及标签前同名文本；不能通过改用 </head> 随意绕过 |
| IIS 资源返回 401/403/404 | 检查实际匿名身份的 ACL、公共目录、StaticFile、JavaScript MIME、路由例外 |
| Static compression is enabled | 审查所选应用影响后明确允许压缩变更 |
| encoded response | 上游/应用仍输出压缩 HTML，或直出了 .gz/.br 文件；先处理来源 |
| CSP ... blocks | 根据实际策略放行脚本、上报和 Worker，工具不会改 CSP |
| existing RUM integration found | 检查业务已有 SDK，确定保留还是迁移，不能重复接入 |
| stale bootstrap URL / 注入版本不一致 | 清理旧 HTML 缓存、检查 CDN/负载均衡是否命中其他实例 |
| external change / asset drift | 外部修改了受管文件/规则，先审查差异，不能靠删除状态绕过 |
| unfinished transaction | 保留状态与日志，执行同 state-dir/site 的 rollback；先看恢复是否完整 |
| operation lock | 先确认相关 Rumable 进程确实已退出，备份现场，才能移除确认过期的 operation.lock |
| version 能运行但 IIS 操作失败 | CLI 架构/运行库与 IIS 管理组件是不同条件，分别检查 |
| 有请求但没有数据 | 核对 appId/token、采样、CORS/TLS、上报响应和查询时间；模拟接收端不写观测云 |

恢复顺序：保存失败输出和状态 → 检查是否仍有进程 → 确认锁归属 → 判断是普通 transaction.json 还是 attachment applying/restoring → 分别执行 rollback 或 Linux rollback --attachment → 再查 status/probe。恢复失败时保留日志，不手工编辑 state.json 或删除事务后重新安装。

若要给支持人员反馈，提供：Rumable/OS/Web 服务器版本、安装方式、脱敏配置、site ID、state-dir、失败阶段、退出码及 status/probe 结果。不要公开完整状态备份或令牌。

## 12. 服务端 OTel（独立可选能力）

页面和 `setup` 只处理浏览器 RUM。服务端 Trace 使用独立的 `[sites.otel]`，模块、Agent、权限和兼容范围与浏览器 SDK 分开检查。

- [OTel 使用手册](docs/otel.md)：Nginx / IIS、DataKit / Dataway、配置与验收。
- [依赖安装和卸载](docs/otel-installation.md)：模块、Agent、Collector 与恢复。
- `enable/disable --component otel` 保留当前 Web SDK 来源、加载方式、版本和初始化脚本；`--component rum` 只调整浏览器采集。
- 计划同时接入时，首次准备完整 RUM 与 OTel 配置。已有纯 RUM 站点不能通过普通升级直接添加 OTel；迁移要求见 OTel 手册。
