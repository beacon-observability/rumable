# 平台与版本兼容性

服务端 OTel 使用独立的 [模块/Agent 兼容矩阵](otel.md#支持范围)：Nginx 1.25.3+；Apache 为 Linux x64 并要求包含请求隔离修复的模块；IIS 为 IIS 10 / Windows 10或Server 2016+ / PowerShell 5.1，且应用池独占。原有 RUM、Go CLI 和浏览器 SDK 的支持范围不因此改变。安装原生模块不等于旧系统已经通过 OTel 运行验收。

## 编译器与操作系统是两个独立约束

源码使用 `go 1.20`，标准库 API 与 TOML 解析依赖均兼容 Go 1.20（固定 `github.com/BurntSushi/toml v1.6.0`，上游最低 Go 1.18）。现代包使用 Go 1.26.x，legacy 包必须真正使用 **Go 1.20.14** 编译；不使用非官方 Go runtime 补丁。

| 发布包 | 编译器 | 目标范围 | 实际验证要求 |
|---|---|---|---|
| Windows 386 / amd64 / arm64 现代版 | Go 1.26.x | Windows 10+ / Server 2016+，选择对应 CPU / 系统位数 | 各目标 Windows 上实际运行 |
| Windows 386 / amd64 legacy | Go 1.20.14 | Windows 7 SP1+ / Server 2008 R2 SP1、Server 2012 / 2012 R2 等适用的旧 x86/x64 系统 | 每种目标系统独立验收 |
| Linux 386 / amd64 / arm64 现代版 | Go 1.26.x | 内核 3.2+；架构还有自身限制 | OrbStack 运行与原生目标验收 |
| Linux 386 / amd64 legacy | Go 1.20.14 | 旧 Linux x86/x64，内核至少 2.6.32；不支持 CentOS 5 的 2.6.18 | 旧内核必须真机/虚拟机验收 |

本次 preview 仅提供 Windows / Linux，共 10 个包，不提供 macOS 包。所有发布包设置 `CGO_ENABLED=0`，不依赖宿主机 glibc；amd64 使用 `GOAMD64=v1`，386 使用 `GO386=sse2`，ARM64 使用 `GOARM64=v8.0`。这不移除内核、Windows API 或平台最低要求。容器共享宿主机内核，**不能**用旧 Linux 用户空间容器证明旧内核兼容。

Go 1.20 已停止官方维护，legacy 包有标准库安全修复缺失这一明确限制；优先使用现代包，旧包只用于无法升级的环境。旧系统证书链和 TLS 设置也需要单独检查，不关闭证书验证来实现兼容。

TOML 依赖版本依据：[上游 go.mod](https://github.com/BurntSushi/toml/blob/v1.6.0/go.mod)。

依据：[Go 最低运行要求](https://go.dev/wiki/MinimumRequirements)、[Go 1.21 Windows 支持调整](https://go.dev/doc/go1.21#windows)、[Go 1.20 Windows 说明](https://go.dev/doc/go1.20#windows)、[Go 维护政策](https://go.dev/doc/devel/release#policy)。

## Nginx

- 使用 `sub_filter` 变量替换，技术能力最低为 **1.9.4**；`check` 同时验证 `--with-http_sub_module`，不会只看版本字符串。
- 默认 Nginx Linux；Windows 版 Nginx 未列入首版运行验收。
- 不依赖 njs、Lua、OpenResty、`add_header_inherit` 或新版本专用指令，能够使用较旧的 Nginx。
- 技术最低版本不等于生产推荐版本。优先使用仍受维护的发行包；旧版本兼容性测试不解决其自身安全缺陷。
- 压缩、已有过滤器、请求头继承和缓存配置比版本号更容易影响接入，必须完成首次挂接审查。

**1.9.4 的自动阻断**：该版本继承 `http` 层 sub_filter 时可能导致 worker 崩溃，此问题在 1.9.5 修复，见[官方变更记录](https://nginx.org/en/CHANGES)。`check`、`enable` 从 `nginx -T` 读取有效配置，按真实挂接上下文递归追踪 include，发现任何 `http` 层 sub_filter 就报告文件、行号并拒绝启用。即使目标 server/location 已覆盖父规则，或 attachmentReviewed=true，仍保守阻止。版本号仍报告 1.9.4 的补丁包也受同一限制，不提供跳过开关。

字面量路径、带空格的引号路径、相对主配置目录的多层 include、`*` / ASCII `?` 掩码和同文件在不同层级重复引用均可检查；注释、引号内容、map 数据中的同名文本不会误判成 http 指令。普通 server/location 层业务 sub_filter 可以继续使用，受管 handler 仍需显式保留业务规则。

配置转储缺失或不一致、include 上下文不完整、循环/过深嵌套及不支持的掩码会直接拒绝。为避免 Go 与 POSIX glob 的差异漏检，方括号/字符类掩码、通配目录后接 `..`、涉及非 ASCII 文件名的 `?` 掩码不自动判定；改为显式路径或简单 `*` 掩码后重试。这项范围限定只用于 1.9.4 风险检查，1.9.5+ 不受此版本专用限制。

普通 rollback 若会恢复启用状态，也先执行该检查；部署候选文件写入后、reload 前再次检查启用目标，包含中断恢复。检查失败不会擅自修改业务配置。停用不受此版本专用检查阻挡，仍须通过 nginx -t 和 HTTP 停用探测。attach 在修改业务配置前也执行同一检查；attachmentReviewed 保留用于其他人工首次挂接条件，不能替代自动检查。

依据：[Nginx sub_filter 模块](https://nginx.org/en/docs/http/ngx_http_sub_module.html)。

符合注入条件的 HTML HEAD 使用与 GET 相同的过滤头部清理，不发送正文。方法 map 使用正则项，兼容旧版默认 map_hash_bucket_size。脚本探测同时验证 identity / gzip，旧的 .gz 资源或压缩缓存不能仅凭 identity 校验通过验收。

## Apache

- Linux **Apache 2.4.23+**，加载 lua_module、filter_module、alias_module、authz_core_module、headers_module；代理、gzip、缓存场景还需站点本身使用的模块。
- 2.4.5 引入 LuaOutputFilter；2.4.23 提供用于确认有效挂接的 DUMP_INCLUDES。CLI 同时检查版本、已加载模块和受管 include 恰好一次；不因为 attachmentReviewed=true 放过缺失能力。
- Apache 2.2、上游版本号低于 2.4.23 的发行版回移补丁包和 Windows Apache 不支持。最低能力版本不代表生产版本建议；CLI 的 Go/内核兼容性仍按上表单独判断。
- 不依赖进程外过滤服务或修改业务代码。`mod_lua` 必须由运维预先安装、启用，工具不会自动安装系统模块。
- 挂接仅限目标 VirtualHost，使用命名 RESOURCE smart filter；输出压缩须在 CONTENT_SET。旧版继承的 AddOutputFilterByType DEFLATE 可能先压缩，需按生成说明改为显式阶段的压缩链；gzip 注入缺失会使启用失败并恢复。其他输出过滤器和晚于 include 的 Location/.htaccess 可能改变顺序，必须覆盖所有 handler 进行探测。编码源响应不改写，UTF-16/32 和流式 SSR 不支持。
- 启用时对目标路径绕过 mod_cache、清除 GET/HEAD 条件请求及 Range；HTML 的 HEAD 也清理原文验证头与长度。影响范围和停用恢复见 [Apache 手册](apache.md)。

依据：[Lua 输出过滤器与分块接口](https://httpd.apache.org/docs/2.4/mod/mod_lua.html#luaoutputfilter)、[2.4.23 官方变更记录](https://archive.apache.org/dist/httpd/CHANGES_2.4.23)、[smart filter](https://httpd.apache.org/docs/2.4/mod/mod_filter.html)。实际运行版本、环境和限制见 [verification.md](verification.md)。

## IIS 与 PowerShell

- 技术基线 **IIS 7.5+**，需要 **URL Rewrite 2.x**。IIS 10 是现代验收目标，7.5/8/8.5 是旧系统兼容目标，均需实际 Windows 验证。
- 使用系统 Microsoft.Web.Administration API 和 Windows PowerShell，不依赖 .NET 6/8、PowerShell 7、ASP.NET Core 或 ARR 来注入页面。
- 需要 **64 位 PowerShell 3.0+**（建议 5.1）。Windows 7 / Server 2008 R2 的默认 PowerShell 2.0 不满足 CLI 与 IIS 桥接脚本的内部 JSON 接口要求，需要运维预先部署适用的 WMF；工具不会自动安装系统组件。
- TOML 配置文件使用 UTF-8；旧 PowerShell 的 `Out-File` 默认可能生成 UTF-16，请使用 `rumable init` 或显式 UTF-8 写入配置。
- 动态应用或反代生成的 HTML 必须在进入 URL Rewrite 时未压缩。工具只调整目标应用的 IIS 静态压缩；不擅自修改后端应用中间件。
- 配置被锁定、已有保留路径映射、`rewriteBeforeCache=true`、ACL 或 MIME 条件不满足时，需先由运维解决。
- 提供 Windows x86 32 位 CLI。它在 64 位 Windows 上经 Sysnative 调用 64 位 PowerShell；真正 32 位 Windows 只能执行版本、配置生成、状态读取与 HTTP 探测，不能管理 IIS。Windows ARM64 CLI 已交叉编译，ARM64 IIS 仍需原生配置 API / PowerShell 能力和实机验收。
- HTTP 探测接受标准中的历史 JavaScript MIME（如 `application/x-javascript`），避免把旧站点的合法脚本误判成不可执行内容；依据 [MIME Sniffing 标准](https://mimesniff.spec.whatwg.org/#javascript-mime-type)。

依据：[URL Rewrite 2 模块及限制](https://learn.microsoft.com/en-us/iis/extensions/url-rewrite-module/using-url-rewrite-module-20)。

## 浏览器

旧 Windows CLI 可运行，不意味着 IE 或旧版浏览器能运行当前 SDK。浏览器兼容性以官方 SDK 版本的要求为准；首版在本机 Chrome 验证，不承诺未经验证的 IE 支持。

可选 CDN 加载不改变 Nginx/Apache/IIS 的最低版本或 CLI 的 Go 基线。浏览器另须能访问 HTTPS CDN，CSP 允许实际脚本地址；开启 integrity 还要求浏览器支持 SRI、跨域 CDN 支持 CORS。CDN 模式已在本机 Chrome 验证，不能据此扩展旧浏览器或 Windows/IIS 实机的验收结论。

## 验证状态

实现验证记录见 [verification.md](verification.md)。Windows/IIS 与旧内核未执行真实运行验收时，不应据此文档宣称生产兼容已经通过。

Windows 的可执行步骤和逐系统验收矩阵见 [windows-acceptance.md](windows-acceptance.md)。脚本记录实际 OS/SP、PS/CLR、IIS/URL Rewrite、二进制版本和摘要；旧主机优先读取包内 build.json 阻止误用现代包。完整验收与 StaticOnly 部分验收分别标记。

## 自动首次挂接

仅 Linux 执行 `attach` / `rollback --attachment`，沿用 Nginx 1.9.4+ / Apache 2.4.23+ 和 Go 1.20 基线。macOS 可生成配置，Windows/IIS 接入流程保持原样。自动范围、压缩和缓存的持续影响、符号链接/共享 include、确认及外部编辑冲突见[操作手册](operations.md#自动首次挂接linux)。不能安全识别的模块/继承语义使用人工 ATTACH.md，不以自动化承诺支持任意配置。
