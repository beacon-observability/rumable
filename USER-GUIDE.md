# Rumable Preview 使用指南

版本：**0.1.0-preview.1**。本版面向测试和接入验证；请先在独立测试站点完成浏览器与观测云数据验收，再用于正式站点。

Rumable 在 Nginx、Apache 或 IIS 返回的完整 HTML 中加入观测云 RUM 脚本，无需修改业务代码。它是一次执行后退出的命令行工具，无需安装 Go、Node.js、数据库或常驻服务。

## 1. 选择下载包

在发布目录的 README 中点击对应压缩包。文件名中的 `386` 是真正的 **x86 32 位**，`amd64` 是 Intel/AMD **x64 64 位**，`arm64` 是 **ARM64**；这些架构不能随意替换。下表文件名均以 `rumable_0.1.0-preview.1_` 开头。

| 系统 / CPU | 文件名后缀 | 适用条件 |
|---|---|---|
| Windows x86 | `windows_386.zip` | Windows 10+ 的 32 位程序；IIS 管理需运行在 64 位 Windows 上 |
| Windows x64 | `windows_amd64.zip` | Windows 10+ / Server 2016+ x64；IIS 接入优先选择此包 |
| Windows ARM64 | `windows_arm64.zip` | Windows ARM64；CLI 与 IIS 均需目标机器验收 |
| 旧 Windows x86 | `windows_386_legacy.zip` | Windows 7 SP1+ 的 32 位程序；32 位 Windows 不支持 IIS 管理 |
| 旧 Windows x64 | `windows_amd64_legacy.zip` | Windows 7 SP1 / Server 2008 R2 SP1、Server 2012 / 2012 R2 等旧 x64 系统 |
| Linux x86 | `linux_386.tar.gz` | 32 位 Linux，内核 3.2+，CPU 支持 SSE2 |
| Linux x64 | `linux_amd64.tar.gz` | 64 位 Linux，内核 3.2+，使用基础 x64 指令集 |
| Linux ARM64 | `linux_arm64.tar.gz` | ARM64 Linux，ARMv8.0 基线；同时满足内核与架构要求 |
| 旧 Linux x86 | `linux_386_legacy.tar.gz` | 32 位 Linux，内核 2.6.32+、SSE2；旧内核需实机验收 |
| 旧 Linux x64 | `linux_amd64_legacy.tar.gz` | 64 位 Linux，内核 2.6.32+；旧内核需实机验收 |

现代包使用 Go 1.26.8，legacy 包使用 Go 1.20.14，均为无 CGO 依赖的独立可执行文件。Go 1.20 已停止维护；旧系统兼容范围不等于完成了旧系统安全和运行验收。完整约束见[兼容性矩阵](docs/compatibility.md)。

Windows x86 CLI 在 64 位系统上通过 Sysnative 选择 64 位 PowerShell；在真正的 32 位 Windows 上可执行 `version`、`init`、`render`、`status`、`probe`，不支持 IIS 的 check/enable/disable/rollback。Windows ARM64 包不代表已验证 ARM64 IIS；版本检查成功也不代表 IIS 模块可用。

## 2. 校验和解压

将 `SHA256SUMS` 与需要的压缩包下载到同一目录。只下载一个包时，比较该包对应的 SHA-256 行即可；下载完整发布目录时可执行：

```sh
sha256sum -c SHA256SUMS
mkdir rumable-0.1.0-preview.1
tar -xzf rumable_0.1.0-preview.1_linux_amd64.tar.gz -C rumable-0.1.0-preview.1
cd rumable-0.1.0-preview.1
```

Windows PowerShell：

```powershell
Get-FileHash .\rumable_0.1.0-preview.1_windows_amd64.zip -Algorithm SHA256
# 与 SHA256SUMS 内同名文件的摘要比较，然后解压到新目录。
Expand-Archive .\rumable_0.1.0-preview.1_windows_amd64.zip -DestinationPath C:\Tools\Rumable
C:\Tools\Rumable\rumable.exe version
```

旧 PowerShell 没有 `Get-FileHash` / `Expand-Archive` 时，使用 `certutil -hashfile 文件名 SHA256`，再通过资源管理器解压 ZIP。不要把工具安装到网站根目录，也不要在临时目录保存长期配置和状态。

包内 `build.json` 记录版本、架构、编译器、源码提交及可执行文件 SHA-256；`sdk.json` 记录内置 SDK 版本、官方来源与摘要。包内 `README.md` 是入口，第三方许可随包提供。

## 3. 准备接入信息

需要网站公开访问地址、可匿名返回 200 的完整 HTML 页面，以及观测云 Web 应用的 `applicationId`。上报方式二选一：

| 上报方式 | 在 `[sites.rum]` 中填写 |
|---|---|
| OpenWay | `applicationId`、`site`、`clientToken` |
| DataKit | `applicationId`、`datakitOrigin` |

`site` / `datakitOrigin` 使用完整的 HTTP(S) origin，不带路径、查询、认证信息或片段；HTTPS 网站必须使用 HTTPS 上报地址。`clientToken` 是浏览器接入令牌，**不能填写管理 API Key 或服务器密钥**。上报目标必须能被最终用户的浏览器访问。

## 4. Linux Nginx / Apache 接入

Nginx 要求 **1.9.4+ 且编译了 http_sub_module**；Apache 要求 **2.4.23+**，加载 `lua`、`filter`、`alias`、`authz_core`、`headers` 模块。工具不会安装服务器或启用模块。Apache 详细准备步骤见 [Apache 手册](docs/apache.md)。

先安装可执行文件，再使用管理员账号维护固定目录：

```sh
sudo install -m 0755 ./rumable /usr/local/bin/rumable
sudo -i
umask 022
mkdir -p /etc/rumable /var/lib/rumable
cd /etc/rumable
rumable version
rumable init --server nginx --reporting openway --site website
# Apache 改为 --server apache；DataKit 改为 --reporting datakit。
```

用文本编辑器修改生成的 `rumable.toml`，至少替换 `publicURL`、探测路径、服务器可执行文件及上报参数。例如一个 Nginx / OpenWay 站点：

```toml
schemaVersion = 1
[[sites]]
id = "website"
publicURL = "https://www.example.com/"
probePaths = ["/"]
includePaths = ["/"]
excludePaths = ["/downloads/"]

[sites.nginx]
binary = "/usr/sbin/nginx"
attachmentReviewed = false

[sites.rum]
applicationId = "REPLACE_APPLICATION_ID"
site = "https://REPLACE_OPENWAY_HOST"
clientToken = "REPLACE_CLIENT_TOKEN"
sessionSampleRate = 100
sessionReplayEnabled = false
```

`binary` / `args` 必须与实际运行的服务一致；自定义 Nginx 配置可加 `args = ["-c", "/etc/nginx/nginx.conf"]`。Apache 常用 `/usr/sbin/apache2ctl` 或 `/usr/local/apache2/bin/httpd`。不要填写 systemctl。

```sh
rumable render --state-dir /var/lib/rumable
rumable attach --state-dir /var/lib/rumable
# 查看目标站点、完整变更和备份位置，输入 yes 后首次挂接。
rumable enable --state-dir /var/lib/rumable
rumable status --state-dir /var/lib/rumable
rumable probe --state-dir /var/lib/rumable
```

`attach --dry-run` 只预览；多个候选时，按输出的真实位置选择 `--target /etc/nginx/sites-available/website:12`。共享 include、已有复杂过滤/重写或继承关系无法安全合并时，按 `render` 输出的 `ATTACH.md` 人工挂接，审查完成后才设置 `attachmentReviewed=true`。Nginx 1.9.4 的 http 层 sub_filter 风险不能由此选项绕过。

反代源 HTML 必须未压缩；工具保留业务 Host、认证和自定义请求头，只调整接入需要的压缩、条件请求和缓存行为。首次挂接的部分调整会持续到完全撤销挂接，详见[操作手册](docs/operations.md)。静态脚本目录须能被网站进程读取；只映射 `sites/<id>/public`，不要映射整个状态目录。

## 5. Windows IIS 接入

使用**管理员、64 位 Windows PowerShell 3+**，建议 5.1。目标必须安装 IIS 7.5+ 与 URL Rewrite 2.x。旧 Windows 使用 legacy 包；CLI 可启动不代表 IIS 环境已满足要求。无需执行 Linux 的 attach。

```powershell
New-Item C:\ProgramData\RumableConfig -ItemType Directory -Force | Out-Null
Set-Location C:\ProgramData\RumableConfig
$rumable = 'C:\Tools\Rumable\rumable.exe'
$state = 'C:\ProgramData\Rumable'
& $rumable init --server iis --reporting datakit --site website
# 编辑 rumable.toml：publicURL、probePaths、iis.site/application、applicationId、datakitOrigin。
```

首次创建全新状态目录时，只给管理员和 SYSTEM 管理权限，然后生成部署：

```powershell
New-Item $state -ItemType Directory -ErrorAction Stop | Out-Null
icacls $state /inheritance:r
icacls $state /grant:r '*S-1-5-18:(OI)(CI)(F)' '*S-1-5-32-544:(OI)(CI)(F)'
& $rumable render --state-dir $state
```

确认上述 ACL 命令均成功。不要对已有部署重复执行初始化 ACL。让站点**实际的匿名资源读取身份**只能遍历父目录、读取公共脚本；默认匿名账户通常是 `IUSR`，若站点改用了应用池身份，应替换 `$reader`：

```powershell
$reader = 'IUSR'
foreach ($p in @($state, "$state\sites", "$state\sites\website")) {
    icacls $p /grant ('{0}:(X)' -f $reader)
}
icacls "$state\sites\website\public" /grant ('{0}:(OI)(CI)(RX)' -f $reader)
```

在 IIS 中确认保留资源路径可匿名读取、JavaScript MIME 和 StaticFile handler 正常；全路径应用路由或入站重写须绕过该资源路径。资源目录外的状态、事务与备份文件不应向网站账户开放读取。

阅读生成的 `ATTACH.md`，确认所选应用静态压缩调整的影响；在 `[sites.iis]` 中显式设置 `allowCompressionChanges=true` 后启用：

```powershell
& $rumable enable --state-dir $state
& $rumable status --state-dir $state
& $rumable probe --state-dir $state
```

只修改指定站点/应用的受管规则和设置，不重启整台 IIS。应用或代理自身产生的压缩 HTML 需要先由应用解决。Windows/IIS 实机验收步骤见 [Windows 验收手册](docs/windows-acceptance.md)。

## 6. 回放、CDN 和采样

在已有 `[sites.rum]` 表内修改参数，不要重复声明同一个表：

```toml
sessionSampleRate = 100
sessionReplayEnabled = true
sessionReplaySampleRate = 20
```

回放默认关闭；开启后默认采用 mask 隐私遮罩。回放还受 RUM 会话采样控制；修改采样率后使用新会话验收。Blob Worker 需 CSP 明确允许 `worker-src blob:`，工具不会修改 CSP。

SDK 默认由网站同域提供，随新发布包在执行 enable 时升级。若使用 CDN，在该站点下单独增加与 `[sites.rum]` 同级的表：

```toml
[sites.sdk]
source = "cdn"
# url = "https://cdn.example.com/rum/3.3.21/dataflux-rum.js"
# integrity = true
```

省略 url 使用发布包记录的官方 CDN 地址。自建 CDN 必须提供与包内 SDK 相同的字节；integrity=true 开启 SRI，跨域时需 CDN 提供 CORS。官方可变地址当前默认不开 SRI；后续远程内容可能独立更新，工具不能恢复被 CDN 覆盖或删除的文件。需要固定版本和离线回滚时，使用默认同域方式或不可变 CDN URL。完整选项见[配置参考](docs/configuration.md)。

改好 TOML 后，再次执行同站点的 `enable --state-dir 原目录`。新脚本使用内容哈希路径，旧文件保留供缓存页面读取。

## 7. 验收、停用、回滚与升级

先执行 probe，随后用真实浏览器打开每种 HTML 页面：检查脚本只加载一次、页面交互正常、Console 没有非预期错误，Network 中出现成功的 RUM 请求；开启回放时使用新会话检查录制。最后进入观测云对应 Web 应用确认页面、资源、错误、操作及所需回放。**HTTP 探测成功不能证明实际入库。**

以下以 Linux 状态目录为例；Windows 将命令换成 `& $rumable`，状态目录换成 `$state`：

```sh
rumable disable --state-dir /var/lib/rumable     # 停用注入，保留旧脚本
rumable rollback --state-dir /var/lib/rumable    # 恢复上一次成功部署，可能重新启用
```

Linux 如需完全撤销首次自动挂接，先 disable，清理浏览器、CDN、代理和 Service Worker 的旧 HTML 缓存，再执行 `rollback --attachment --state-dir /var/lib/rumable` 并确认。普通 rollback 只切换部署版本，重复执行会在两个成功快照间切换。

升级时先备份 TOML、整个状态目录和服务器配置，将新包解压到新目录，核对版本后替换可执行文件；继续使用原 TOML、站点 ID、目标和状态目录，执行 enable。不重复 init/render，不删除旧资源。

## 8. 常见问题

| 提示 / 现象 | 处理 |
|---|---|
| exec format error / 不是有效的 Win32 应用程序 | 按操作系统、CPU 和系统位数重新选包；旧 Windows 用 legacy |
| 无法启动、缺少系统 API | 检查最低系统版本；交叉编译不代表所有旧系统已验收 |
| 候选不唯一 / 站点未识别 | 核对 publicURL、监听端口、binary/args，使用输出中的明确 target |
| 缺少模块 / http 层 sub_filter 风险 | 按具体错误修复服务器能力或配置，不能用 attachmentReviewed 绕过 |
| CSP / 压缩 / SDK 摘要失败 | 按错误位置检查策略、源压缩或过期脚本缓存，修复后再 enable |
| 页面没有 `</head>` | 保持正文不变；完整 HTML 才可接入，片段和流式 SSR 不支持 |
| 停用/回滚后浏览器仍是旧状态 | 清理 HTML 缓存并打开新页面；已打开的页面不会即时撤销 SDK |
| external change / 恢复冲突 | 保留现场和备份，审查外部修改；不要覆盖状态或强行删除事务 |
| operation.lock | 先确认没有正在运行的 Rumable，再按[恢复流程](docs/operations.md)处理遗留锁 |
| 浏览器没有数据 | 检查采样、CSP、浏览器可达性、CORS 和上报参数，再查看观测云 |

多个站点放在各自 `[[sites]]` 下，命令使用 `--site ID`。`id`、服务器目标和 publicURL 绑定部署身份；迁移应使用新 ID。状态目录、配置和备份含站点信息，应由管理员保存；不要公开上传真实配置。
