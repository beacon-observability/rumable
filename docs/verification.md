# 实现验证记录

日期：2026-10-19–20（Asia/Singapore）。此记录区分代码检查、本地运行与真实观测云验收。

## 首次挂接自动化（2026-10-20）

新增 `attach`（检测、差异、确认、挂接）及 `rollback --attachment`（独立撤销/恢复），保留原来的人工挂接与部署 rollback。实际验证：

| 范围 | 本轮结果 |
|---|---|
| Go 与文件边界 | OrbStack Linux arm64，Go 1.20.14 / 1.26.8 test、vet，1.26.8 race 全部通过；原子替换保留模式、属主/组和 user xattr，先清除临时文件继承的默认 ACL，防止扩大读取权限；拒绝直接替换符号链接/硬链接及外部内容；两文件只写完一个时可恢复 |
| Nginx 最低 / 现代版 | 1.9.4 原始源码镜像、官方 1.28.3，自动挂接完整脚本通过；包括静态预压缩、输出 gzip、静态/反代/chunked、上游业务请求头保留、热缓存绕过和撤销后恢复缓存行为 |
| Apache 最低 / 现代版 | 2.4.23 原始源码镜像、官方 2.4.69，自动挂接完整脚本通过；包括原生有效站点识别、CONTENT_SET gzip、代理资源排除、目标 include 的 Location 及其他 VirtualHost include 的过滤规则隔离 |
| Legacy CLI | Go 1.20.14 Linux amd64 包在 OrbStack amd64 仿真下对 Nginx 1.28.3 完整自动挂接脚本通过；不代表旧 Linux 内核已运行验证 |
| 原有流程回归 | `smoke-nginx.sh` 在 1.9.4、`smoke-apache.sh` 在 2.4.23 通过，人工挂接、业务过滤规则、压缩、缓存、继承、升级及恢复流程保留 |
| Chrome | 本机真实页面初始化一次；DOM 中一个 Rumable 初始化脚本、一个 SDK；交互正常，控制台仅主动触发的测试异常。默认 replay 关闭，本地接收端记录 View 2 / Resource 6 / Action 1 / Error 1，无 replay；这是本地接收，不是观测云入库 |

自动挂接脚本同时覆盖：dry-run 不改业务文件、取消不提交、确认期间业务文件变更即拒绝、配置检查/重载/提交后 HTTP 三类失败恢复原字节、重复挂接不重载、站点隔离、启停/普通 rollback/完整撤销、外部编辑时停止覆盖、原始文件模式保留、备份保留及再次挂接。Nginx 1.9.4 还验证 http 层 sub_filter 在创建挂接备份/写业务文件之前即被阻止。

原生 include 回归覆盖 Nginx 共享源文件拒绝编辑、父/局部 proxy_set_header 列表、同内容 handler 的独立标记，以及 Apache DUMP_INCLUDES 父文件行号/层级展开；其他 VirtualHost 的 CONTENT_SET 声明不能放行本站未声明阶段的压缩链。未知 include 上下文和不安全合并拒绝。缓存恢复验证等待平滑重载的新旧 worker 交替完成，检查恢复缓存命中行为，不要求保留某一条旧缓存项。

可复现步骤（本机已安装 Go、OrbStack Docker；脚本创建并清理自己的容器）：

```sh
# 最低版本镜像只需构建一次；使用仓库固定来源/校验的 Dockerfile。
docker build -t rumable-nginx:1.9.4 -f scripts/nginx-1.9.4.Dockerfile .
docker build -t rumable-apache:2.4.23 -f scripts/apache-2.4.23.Dockerfile .
SERVER=nginx NGINX_IMAGE=rumable-nginx:1.9.4 bash scripts/smoke-attach.sh
SERVER=nginx NGINX_IMAGE=nginx:1.28-alpine bash scripts/smoke-attach.sh
SERVER=apache APACHE_IMAGE=rumable-apache:2.4.23 bash scripts/smoke-attach.sh
SERVER=apache APACHE_IMAGE=httpd:2.4 bash scripts/smoke-attach.sh
# 验证指定发布二进制时，额外设置 RUMABLE_BINARY=/绝对路径/rumable。
```

自动范围及持续影响见[操作手册](operations.md#自动首次挂接linux)。本轮未新增真实 Windows/IIS、旧内核、Brotli 模块或真实观测云 OpenWay/DataKit 入库验证。备份/规则自动化不等同于支持任意第三方模块或未知配置生成器。

## 已通过

| 验证 | 环境与结果 |
|---|---|
| 源码测试及 vet | Go 1.20.14、Go 1.26.8，OrbStack Linux arm64，含 TOML 解析依赖、CGO 关闭 |
| Race | Go 1.26.8，OrbStack Linux arm64 |
| Nginx 现代版 | 官方 nginx:1.28-alpine，实际 1.28.3，OrbStack arm64 |
| Nginx 旧版 | 官方 nginx:1.14-alpine，实际 1.14.2，OrbStack arm64 |
| Nginx 最低版本 | 官方 1.9.4 原始源码构建，OrbStack Debian 12 / arm64；有下述 http 层过滤规则限制 |
| 初始化脚本 | Node.js 内置 VM：重复执行只初始化一次、既有 SDK 不接管、资源加载失败不抛出业务异常 |
| Chrome | 本机 Chrome 155，真实加载内嵌官方 SDK 3.3.21；页面交互和本地事件发送通过 |
| PowerShell | Microsoft 官方 Linux PowerShell 容器进行语法检查；这不是 Windows/IIS API 运行验证 |
| 发布包 | 六个包的压缩结构和 SHA-256 校验通过；Windows PE 编译信息确认 legacy 使用 Go 1.20.14 |
| 发布包运行 | macOS arm64 版本命令；Go 1.26.8 Linux arm64 在 Nginx 1.14.2 上启用与探测；legacy Linux amd64 在 OrbStack amd64 仿真下 HTTP 探测通过 |

Nginx 两个版本使用同一脚本验证：

- 静态页面与反向代理页面注入；普通 chunked 响应；输出 gzip；强 ETag 移除；条件请求不落入旧 304。
- API 不注入；下载及排除范围的 HTML 即使含混合大小写 `</HeAd>` 也保持原始正文；已压缩上游不注入。
- 首次启用、重复启用、停用、回滚、配置升级及旧脚本保留。
- 注入一次真实的重载失败后恢复原配置及 HTTP 行为；外部修改受管文件时拒绝回滚覆盖。

两个官方镜像的本次摘要：

```text
nginx:1.28-alpine sha256:a8b39bd9cf0f83869a2162827a0caf6137ddf759d50a171451b335cecc87d236
nginx:1.14-alpine sha256:485b610fefec7ff6c463ced9623314a04ed67e3945b9c08d7e53a47f6d108dc7
```

## Linux/Nginx 最低版本与边界补测

本轮实际运行 **Nginx 1.9.4、1.14.2、1.28.3**，使用扩展后的同一 `scripts/smoke-nginx.sh`。运行环境为 OrbStack Linux arm64；这些结果不证明旧内核兼容。

1.9.4 的官方旧镜像是当前 Docker 不接受的 schema v1。新增 `scripts/nginx-1.9.4.Dockerfile`，从官方未修改源码构建，固定下载校验：

```text
source: https://nginx.org/download/nginx-1.9.4.tar.gz
source SHA-256: 479b0c03747ee6b2d4a21046f89b06d178a2881ea80cfef160451325788f2ba8
local image ID: sha256:0f1d8b0d14d3722471d7b290589d7a1800ec653003f4f509d8f1a35a3979ecb6
```

构建使用 Debian 12 / GCC 12，启用 http_sub 与 gzip_static；关闭夹具未使用的 auth_basic，避开现代 glibc 已删除的 crypt_data.current_salt；`-Wno-error` 仅取消旧源码警告被当作构建错误，不给 Nginx 打行为补丁。本轮验证 HTTP 注入链路，未覆盖该版本旧 TLS/OpenSSL 组合。

补测发现并处理：

- **1.9.4 上游崩溃**：继承 `http` 层 sub_filter 时，SDK 请求和服务器提前返回 503 均复现 worker signal 11。该缺陷在 [Nginx 1.9.5 修复](https://nginx.org/en/CHANGES)。受管资源和停用 handler 显式使用动态空匹配，避免恢复继承过滤；资源不执行业务正文过滤。业务规则仍须移到 `server` / `location`；后续自动预检会拒绝保留 http 层规则的启用，attachmentReviewed 不能绕过，见下一节。最终通过的 1.9.4 配置采用 server 层父规则及 location 层显式保留；不能将此结果推广为任意 http 层过滤配置都兼容。
- **非 HTML 误注入**：继承 sub_filter_types=* 时，原规则可能改写 JSON 内的 `</HeAd>`。新增独立 MIME 门控，真实 HTTP 验证 JSON 字节不变；源 Content-Encoding 门控同时覆盖上游和静态预压缩响应。
- **隐藏头继承丢失**：去掉覆盖父层列表的 proxy_hide_header，使用 sub_filter 自身移除改写 HTML 的 ETag / Last-Modified。真实 HTTP 验证 X-Backend-Secret 保持隐藏；无业务过滤器的独立 handler 在启用时去除验证头，停用后恢复强 ETag / Last-Modified。
- **资源安全头继承丢失**：将资源 location 的 add_header Cache-Control 改为 expires 1y；验证资源保留父层 X-Site-Security 并带一年缓存，脚本摘要不变。
- **压缩探测盲区**：提交后与独立 probe 同时请求 identity / gzip；gzip_static on 导致压缩页面未注入时，启用失败并自动恢复；gzip_static always 在源压缩预检时拒绝。新增 Go 回归测试保护 gzip 变体缺失注入、损坏 gzip、强制源压缩，以及解压后 8 MiB 上限。Brotli 静态模块仍需人工关闭，未执行 Brotli 运行验证。

三个版本均通过以下实际断言：

| 边界 | 本轮实际结果 |
|---|---|
| HTML 与压缩 | 静态、反代、普通 chunked、跨 chunk 的混合大小写 head 锚点均只注入一次；identity / 输出 gzip 都通过；已压缩源、下载、API、排除范围、缺失 head 和 503 不注入 |
| 静态缓存验证头 | 使用启用前的真实静态 ETag 和未来 If-Modified-Since 请求，仍返回已注入的完整 200 HTML，无原文验证头 |
| 代理缓存 | 启用前预热缓存，使用上游递增请求计数确认后续命中；启用、停用、升级、回滚保持缓存命中，返回当前应有的注入 URL；清除上游条件请求头防止 304 |
| 配置继承 | 保留 Host、Authorization、X-Tenant，清除 Accept-Encoding 和两种条件请求头；覆盖嵌套 location 继承、业务 sub_filter 同层保留、父层隐藏头和资源安全头 |
| 生命周期与资源 | 重复启用/停用不改变部署历史；升级更换初始化 URL；停用和回滚后旧、新资源均可访问 |
| 失败恢复 | 分别注入 nginx -t、reload、提交后 HTTP 503 失败；恢复原状态文件、配置及 HTTP 行为，不留下 transaction.json；失败不占用成功历史 |
| 外部修改 | 启用、停用、回滚均拒绝覆盖被外部改动的受管文件；业务拥有的配置改动在升级和回滚后保留 |
| worker 状态 | 最终完整流程中未出现 signal 退出或 alert / crit / emerg 运行日志 |

本机 Chrome 在 1.9.4 最终部署上确认：一个 Rumable 标签、一个 SDK 标签；初始化成功，默认 recording=false，Fetch resource 正常。Console 只有主动触发的测试错误，无 SDK/CSP/CORS 警告。本地接收端会话结束快照：View 5、Resource 6、Action 3、Error 1、Long Task 8，共 5 个接收请求；只证明本地发送，不代表观测云入库。截图保存在本机 `/tmp/rumable-nginx-1.9.4-chrome.png`，统计为 `/tmp/rumable-nginx-1.9.4-stats.json`。

Go 1.20.14、1.26.8 在 OrbStack arm64 的完整 test / vet，以及 Go 1.26.8 race 均通过。源码继续保持 go 1.20 基线。

另外将 Go 1.20.14、CGO_ENABLED=0、GOAMD64=v1 编译的 Linux amd64 CLI 传入 RUMABLE_BINARY，在 OrbStack amd64 仿真下跑完 1.9.4 的同一完整流程，全部通过；状态输出确认 go1.20.14 / linux / amd64。这个组合验证旧编译器 CLI 与最低 Nginx 的实际协作，仍不等于旧 Linux 内核或旧发行版真机验收。

复跑方式（仓库根目录，需 OrbStack、Go、curl、gzip，端口 19180/19181 空闲）：

```sh
docker build -f scripts/nginx-1.9.4.Dockerfile -t rumable-nginx:1.9.4 .
NGINX_IMAGE=rumable-nginx:1.9.4 scripts/smoke-nginx.sh
NGINX_IMAGE=nginx:1.14-alpine scripts/smoke-nginx.sh
NGINX_IMAGE=nginx:1.28-alpine scripts/smoke-nginx.sh
# 可选：使用发布包解压后的 Linux CLI 替代源码构建。
RUMABLE_BINARY=/absolute/path/to/linux/rumable NGINX_IMAGE=rumable-nginx:1.9.4 scripts/smoke-nginx.sh
```

脚本失败返回非零；正常运行会清理本轮容器、网络和目录。`KEEP=1` 保留现场并打印归属信息。最终日志位于本机 `/tmp/rumable-nginx-minimum-final.log`、`/tmp/rumable-nginx-1.14-hardened.log`、`/tmp/rumable-nginx-1.28-hardened.log`、`/tmp/rumable-nginx-minimum-legacy-go.log` 与 `/tmp/rumable-nginx-hardened-go.log`。OrbStack 共享目录中的故障配置修改由容器内执行，避免宿主写入与容器文件元数据短暂不同步影响断言。

## Nginx 1.9.4 自动预检复验

在首次最低版本补测后，将上述风险从人工挂接要求改为自动硬检查：

- 仅对版本号为 1.9.4 的 Nginx，从同一次 nginx -T 的标准输出读取文件转储；标准错误中的测试成功提示不混入配置。按 include 挂接层级递归检查，支持主文件、相对主配置目录的多层引用、引号路径及简单通配符；同一文件在不同层级使用时分别判断。
- check / enable 发现 http 层 sub_filter 即报告源文件、行号并返回失败，attachmentReviewed=true 不会放行。预检拒绝后不创建事务、不改写当前部署及历史、不 reload；check 本身仍保存失败检查记录。
- 普通 rollback 若会恢复启用状态，先检查再进入事务；候选文件写入后，native apply 在启用目标 reload 前再次检查，覆盖恢复路径。停用目标保留 nginx -t / HTTP 验证，不被版本专用规则阻挡。中断恢复若遇到不兼容外部配置，保留日志，等待修复后重试。
- 无法还原转储上下文、缺失 include、重复转储不一致、递归/过深展开和不支持的掩码均拒绝；使用显式路径或简单 * / ASCII ? 掩码后重试，详见[兼容性说明](compatibility.md)。不提供跳过开关；1.9.5+ 不应用这项版本专用限制。

新增回归用例保护真实风险及误判边界：直接/多行/带引号的指令，http 块位于附属配置，多层相对 include、通配符、带空格的路径、跨层重复文件，注释/引号/map 数据/变量中的同名文本，隐藏文件与显式转义点号，不完整/伪装的转储、缺失文件、空通配符、循环和展开上限。错误定位验证使用真正包含规则的文件与行号。

本轮 OrbStack 实跑：

| 环境 | 结果 |
|---|---|
| Nginx 1.9.4 原始源码镜像 / arm64 | attachmentReviewed=true 下，直接规则、多层 glob include、引号路径、同文件先 server 后 http 引用均拒绝；不支持掩码也拒绝。比较当前状态、原业务配置和 reload 计数，确认被拒绝的 enable 没有提交或 reload |
| Nginx 1.9.4 停用与恢复 | 在外部配置新增风险后仍能停用；回滚到启用状态被拒绝，状态、reload 计数和事务状态保持；修复业务配置后回滚成功 |
| Nginx 1.14.2 / 1.28.3 | http 层业务过滤器可通过预检，不套用 1.9.4 限制；原有完整压缩、缓存、继承、升级回滚和失败恢复流程继续通过 |
| Go 1.20.14 / 1.26.8 | OrbStack Linux arm64 的完整 test / vet 均通过；Go 1.26.8 race 通过 |
| legacy CLI / amd64 仿真 | Go 1.20.14 编译的 CLI 在 Nginx 1.9.4 上跑通同一完整流程，包含自动阻断、停用与回滚；输出确认 linux / amd64 / go1.20.14 |

真实配置修改和 reload 计数比对在容器内执行，避免共享目录的跨端元数据延迟影响拒绝断言。执行命令仍为上一节的 smoke-nginx.sh；该脚本按实际 Nginx 版本选择风险拒绝用例或较新版本允许用例。本轮没有改变浏览器脚本或页面行为，Chrome 与本地上报的证据仍为上一轮记录，未当作本轮重新执行。

本轮日志：`/tmp/rumable-nginx194-preflight-final.log`、`/tmp/rumable-nginx114-preflight-final.log`、`/tmp/rumable-nginx128-preflight-final.log`、`/tmp/rumable-nginx194-preflight-legacy.log`、`/tmp/rumable-preflight-legacy-go.log`、`/tmp/rumable-preflight-modern-go.log`，最终编译器检查与打包日志为 `/tmp/rumable-preflight-final-build.log`。旧内核、旧 TLS/OpenSSL、Brotli 与真实观测云入库验收状态不变。

## TOML 配置变更复验

- Go 1.20.14、Go 1.26.8 的测试与 vet，以及 Go 1.26.8 race 均通过；TOML 依赖没有提高 legacy 编译器要求。
- 覆盖生成配置读取与防覆盖、多站点两种上报模式隔离、Windows 字面量路径、注释与转义、显式零采样率，以及未知字段、重复定义、模式混用、错误类型和 NaN/Infinity 的拒绝行为。
- 两个 Nginx 版本均使用 TOML 配置完整重跑上述集成流程，包括配置升级、重载失败恢复和外部漂移保护。
- Windows 冒烟脚本改为生成 TOML，已通过 PowerShell 语法检查；Windows/IIS 实机验证仍待执行。

TOML 切换时未改变浏览器脚本；以下基础 RUM 结果保留此前实现验证记录。Session Replay 的浏览器复验单独记录在后面。

## Chrome 与上报证据

测试地址 `http://127.0.0.1:19180/`；在现有 Chrome 控制接口执行，不安装 Playwright 等框架。页面标题正确、内容正常；点击 Fetch resource 后显示 Fetched，再触发预期测试错误。

DOM 中只有一个 Rumable 初始化标签、一个同域官方 SDK 标签。SDK 的弃用参数警告已修正；复查时 Console 只有主动触发的 `Rumable intentional regression-test error`，没有当前 SDK 初始化、CSP 或 CORS 错误。

使用两种配置模式均向本地接收端实际发送事件：

| 模式 | 本次接收快照 |
|---|---|
| DataKit 配置 | View 4、Resource 6、Action 1、Error 1 |
| OpenWay 配置 | View 4、Resource 6、Action 2、Error 1 |

计数来自多次访问及 SDK 更新事件，用于证明事件类型实际产生，不是产品业务指标。接收端解析官方 SDK 的 DataKit line protocol，只保存类型计数，不保存原始遥测正文。

## Session Replay 复验

- Go 1.20.14、Go 1.26.8 测试与 vet，以及 Go 1.26.8 race 均通过。覆盖默认关闭、启用时的默认采样率、零与小数采样率、多站点隔离、无效数值拒绝及状态持久化；切换回放设置会生成新脚本 URL，保留旧页面资源。
- 对实际生成的初始化脚本运行 `scripts/test-bootstrap.cjs <init.js> <expected-rate>`，验证关闭、37.5% 和零采样率。每页最多初始化及请求录制一次，录制 API 在 init 后调用；已有 SDK 不接管，加载、初始化及录制 API 失败均隔离。默认 `mask` 遮罩及错误补偿采样关闭也已检查。
- Nginx 1.28.3、1.14.2 在 OrbStack 完整重跑冒烟流程，升级包含回放开关变更；验证真实重载失败后的恢复和回滚。IIS 桥接与 Windows 冒烟脚本通过 PowerShell 语法检查，未进行 Windows/IIS 实机验证。
- 本机 Chrome 使用内嵌官方 SDK 3.3.21：默认关闭与显式关闭时 `isRecording()=false`，开启并采样 100% 后为 `true`，开启但采样为 0 时为 `false`，回滚恢复录制。Fetch resource 交互正常，页面只有一个注入标签和一个 SDK 标签，Console 无警告或错误。
- 本地 DataKit 模式接收端累计收到 **13 个非空回放片段、10,229 字节压缩片段数据**。这是多次切换与访问的累计值，只证明浏览器实际上传回放；接收端仅保留片段数量和字节数，不保存回放正文，不验证观测云中的解码与播放。
- Worker CSP 的允许、阻止、回退和多策略交集通过边界测试；默认关闭或零采样率不额外要求 Worker 策略。验收时刷新页面并核对实际初始化脚本 URL，旧页面不会被服务器端开关即时终止录制。

## SDK 配置扩展复验

- 对照官方 NPM `browser-core 3.3.21` 的公开类型定义和内嵌 SDK，开放 54 个站点级 RUM 字段及 Visitor ID 子项。排除废弃别名、函数/RegExp、插件实例和未公开字段；完整范围与默认值见[配置参考](configuration.md)。
- 配置与持久化测试保护显式 false、复合 Trace URL 表、Visitor ID、Canvas 联合类型、跨站点设置隔离、数值/枚举/保留 Header 校验、JSON 与普通压缩互斥，以及状态往返不丢字段。
- Worker 边界覆盖内嵌与同域托管、Canvas 独立 Worker、错误回放和远程配置的需求、CSP 回退、默认端口/主机大小写，以及外部文件 200/JS MIME 检查。测试托管 Worker 仅验证可达性，不声称其业务代码正确。
- 实际生成脚本的 Node VM 验证包含配置传递、错误专用回放、总开关关闭、JSON 模式和远程配置模式；字符串中的模板标记、引号和 HTML 不改变代码结构。重复加载、既有 SDK 和失败隔离继续通过。
- 本机 Chrome 的真实 SDK 确认：关闭自动操作采集、mask-user-input 隐私、localStorage 会话、操作名称隐私、压缩及 Canvas 2D 选项实际进入 init；Fetch 交互收到 Trace 和 Visitor ID Header，Canvas 绘制正常，初始化无 SDK/CSP/CORS 警告或错误。
- 本地接收端成功解压普通 RUM 请求并统计到 View/Resource，未出现关闭的自动 Action。普通回放 0 / 错误回放 100 的新会话中主动触发错误后，收到 Error 和回放片段；Console 只有预期测试错误。关闭测试页后的累计快照为：5 个成功解压的 RUM 请求、Error 1、14 个回放片段（10,918 字节）。计数只证明本地发送，不证明真实观测云回放完整性。
- Nginx 1.28.3 与 1.14.2 在 OrbStack 重跑完整启停、升级、回滚和失败恢复流程。Go 1.20.14 / 1.26.8 的测试与 vet，以及 Go 1.26.8 race 复验通过。

远程配置使用 VM 与 Worker 预检验证，未连接真实观测云配置服务；跨域 API 的 CORS、分区 Cookie、不同浏览器、WebSocket、手动 View/Canvas 和真实 Canvas 回放仍需对应业务环境验收。Windows/IIS 仍未在本机 macOS 执行实机运行。

## 代码审查修复复验（2026-10-19）

- 修复配置边界：拒绝空查询标记、无效端口、片段及会破坏 SDK 查询参数的应用 ID / Client Token。普通标签字符串转义和显式 false / 0 继续保留。
- 修复 CSP 预检：同时检查真实哈希初始化脚本和 SDK、RUM / Replay / 远程配置的实际请求路径；修正完整路径、目录、编码分隔符、通配主机和端口匹配。跨域 HTML base 会导致根路径资源加载到其他域，现已阻止启用。同域 base 继续可用。
- 修复恢复边界：中断日志必须与受管目标一致；恢复前验证原资源；状态中的脚本和 SDK 引用必须绑定对应摘要。回归测试验证冲突时文件和日志保留，旧 SDK 摘要的探测使用历史快照，不依赖当前发布包的 SDK 文件名；新增 SDK 参数限制不会阻止读取历史部署用于停用和恢复，历史资源不被重写。省略与显式空 `nginx.args` 视为同一目标。
- 修复 IIS XML 快照对站点、应用和虚拟目录名称大小写的处理，避免漏存原压缩属性、规则和映射。`scripts/test-iis-snapshot.ps1` 在 OrbStack PowerShell 7.4 执行，验证 XML 读取、站点隔离及 JSON 往返；这是桥接函数测试，未调用 Windows IIS 配置 API。Windows 冒烟脚本也改为使用与实际站点名称大小写不同的配置，供实机验收。
- Go 1.20.14 / Go 1.26.8 的完整测试与 vet、现代编译器 race 检查通过；继续使用 `go 1.20` 源码基线与独立 legacy 编译器。
- OrbStack Nginx 1.14.2 / 1.28.3 完整流程通过：静态、反代、chunked、压缩、缓存条件请求、重复启用、停用、升级、回滚、重载失败恢复和外部修改拒绝。
- 本机 Chrome 复验默认回放关闭、唯一初始化；开启后实际 `isRecording()=true`，压缩、mask-user-input、localStorage、关闭自动操作及名称隐私选项进入 SDK，Fetch 请求带 Trace / Visitor ID Header。停用后页面只有业务脚本，回滚并排除旧 HTML 缓存后恢复。Console 仅有主动触发的测试错误。
- 该轮本地接收端收到 View 8 / Resource 18 / Action 2 / Error 1，成功解压 3 个普通 RUM 请求，收到 4 个回放片段、4,165 字节。Action 包含默认开启阶段的操作；这些累计计数证明本地发送，不代表真实观测云验收。实际生成脚本的 Node VM 防重、参数传递与失败隔离也通过。

## Linux / Apache 支持验收（2026-10-19–20）

新增独立 `[sites.apache]` TOML 目标、受管 apache.conf、私有内容哈希 Lua 文件及公共 SDK Alias，复用既有 SDK、CSP、双编码探测与事务恢复。此次实际运行环境如下，均为 OrbStack 的现代 Linux 内核，不代表旧发行版/内核真机验收：

| 服务器 | CLI / 环境 | 结果 |
|---|---|---|
| Apache 2.4.69 | 官方 httpd:2.4，Linux arm64，Go 1.26.8 的发布构建参数二进制 | 完整 smoke 通过 |
| Apache 2.4.23 | 官方原始源码，Debian 12 / GCC 12 / Lua 5.2，Linux arm64；Go 1.26.8 的发布构建参数二进制 | 完整 smoke 通过 |
| Apache 2.4.23 | 同上服务器，Go 1.20.14 / CGO=0 / GOAMD64=v1 的 Linux amd64 CLI，OrbStack 仿真执行 | 同一完整 smoke 通过 |
| Nginx 回归 | Go 1.26.8 Linux arm64 CLI，Nginx 1.9.4 与 1.28.3 | 原有完整脚本通过，含 1.9.4 http 层自动阻断 |
| 源码检查 | OrbStack Go 1.20.14 与 1.26.8 test / vet；Go 1.26.8 race | 通过 |

Apache 两个版本共用 `scripts/smoke-apache.sh`，覆盖静态/反代、普通 chunked 与跨块混合大小写锚点、第一处 head、无锚点正文逐字节保留、下载/API、已压缩源跳过、identity/gzip、真实静态 ETag/未来条件日期/Range、POST 条件头保留、Host/Authorization/X-Tenant、父业务过滤器与安全头、mod_cache 绕过及停用恢复、独立 VirtualHost 状态和脚本隔离。

补齐 HEAD 缓存边界：修复前实测选中静态 HTML 的 HEAD 仍带原强 ETag、Last-Modified 和原长度 86，可能验证旧 GET 表示；只 unset Content-Length 还会被 Apache 从内部临时正文重新计算。现在清理 GET/HEAD 条件请求，对 HTML HEAD 清理验证元数据，并丢弃其内部临时正文；HTTP HEAD 本来就不发送正文。静态与反代 HEAD 在 identity/gzip 两种请求中都返回 200，不带原验证头和长度；停用后条件 HEAD 恢复 304。现代/legacy CLI 的上述完整 Apache 流程均包含这一回归。复现日志 `/tmp/rumable-apache-head-regression-before.log`，修复后的源码验证日志 `/tmp/rumable-apache-head-fixed-source.log`，最终构建使用下列完整日志。

同时验证：模块缺失、include 未挂/重复、低版本阻止；首次 render 的 attachmentReviewed=false 停用快照可回滚；重复启停不改变历史；升级改变脚本 URL且保留旧资源；候选 configtest、graceful、提交后 HTTP 503 故障均恢复原状态且不留下事务；受管配置和私有 Lua 摘要漂移时启用/停用/回滚拒绝覆盖。最终流程无 Apache error/crit/alert/emerg 或未知过滤器运行日志。

最低版本实测修复了两个兼容点：旧 mod_lua 对响应头赋 nil 会在过滤器初次执行时报错，因此改用 mod_headers，按 Lua 标记清除两张响应头表；旧 mod_filter 的继承 AddOutputFilterByType DEFLATE 可能按 RESOURCE 执行，先压缩而错过注入。实际通过的挂接采用显式 CONTENT_SET 压缩链，保留原业务过滤链；生成 ATTACH.md 给出配置与影响。2.4.23 的回归还故意恢复不兼容压缩顺序，确认 gzip 探测阻止成功提交并恢复原受管部署，不能将通过结果推广为任意过滤顺序均兼容。

源码构建入口：`scripts/apache-2.4.23.Dockerfile`，使用未修改官方源码，SHA-256 与镜像记录：

```text
source: https://archive.apache.org/dist/httpd/httpd-2.4.23.tar.gz
source SHA-256: b71a13f56b8061c6b4086fdcc9ffdddd904449735eadec0f0e2947e33eec91d7
rumable-apache:2.4.23 image ID: sha256:0376e4b7c6e6e5bac61c7897c61e1ec1870cb15237ffa3394a6cc728156ad05b
httpd:2.4 image ID: sha256:c842ac797bd1fe8b3cd26d002ac9d948aa2814097544600600b35aef42e6785a
```

本机 Chrome 155 在最终 Apache 2.4.69 / Go 1.26.8 部署上复验：Rumable 与 SDK 标签各 1 个，默认 recording=false；通过 TOML 开启回放并 New session 后，SDK 显示采样率 100、recording=true。Fetch 交互正常；Console 只有主动触发的测试错误，无 SDK/CSP/CORS 警告。此前同一 Apache 版本的开发者工具检查另行确认初始化脚本、SDK、RUM 与回放请求返回 200。最终这一轮接收端快照：View 5、Resource 11、Action 4、Error 1，3 个回放片段，共 2281 个片段字节、8 次接收请求。证据为本机 `/tmp/rumable-apache-chrome.png` 和 `/tmp/rumable-apache-chrome-stats.json`；这些只证明本地发送，不证明观测云入库。

复跑命令与首次挂接步骤见 [Apache 手册](apache.md)。日志：`/tmp/rumable-apache-final-modern.log`、`/tmp/rumable-apache-minimum-modern.log`、`/tmp/rumable-apache-minimum.log`、`/tmp/rumable-apache-final-legacy.log`、`/tmp/rumable-apache-final-go.log`、`/tmp/rumable-apache-nginx194-regression.log`、`/tmp/rumable-apache-nginx128-regression.log`。未验证 Windows Apache、Apache 2.2、真实 TLS 终结、Brotli、任意自定义过滤/重写及发行版 apache2ctl/envvars/SELinux 组合；不据此宣称这些组合已经通过。

## 2026-10-20 优化复验

修复 Nginx 初始停用快照保留 attachmentReviewed=false 时重复 disable 被拒绝的问题：只在启用检查中要求人工挂接确认，停用仍检查模块、有效 include、配置和文件归属。旧二进制在 OrbStack / Nginx 1.9.4 上已复现该失败；扩展 scripts/smoke-nginx.sh，覆盖首次 false 拒绝 check/enable 且不创建事务或 reload、初始重复停用、首次启用后回滚到 false 快照、再次停用，以及候选配置失败后恢复该快照。

多页面探测改为每轮对相同脚本和 Worker 只检查一次，HTML 的 identity / gzip、注入版本及 CSP 仍逐页检查，不跨轮缓存。扩展现有 HTTP 边界测试：两个页面和一个共用 Worker 的请求由 10 次减少到 7 次；第二页 gzip 注入缺失、后续轮次脚本 MIME 改变、重复注入及历史 SDK 引用仍被正确处理。这个计数是脚本 gzip 校验扩展前的阶段结果，不代表端到端耗时测量；当前请求组成见下一节。

OrbStack 的 Go 1.20.14、1.26.8 全量 test / vet 与 Go 1.26.8 race 通过。使用此次 Go 1.26.8 Linux arm64 二进制完成 Nginx 1.9.4、1.28.3 和 Apache 2.4.23、2.4.69 的完整 smoke，含压缩、缓存、配置继承、幂等、升级/回滚、提交失败及漂移拒绝。日志：`/tmp/rumable-optimize-before.log`、`/tmp/rumable-optimize-go.log`、`/tmp/rumable-optimize-nginx194.log`、`/tmp/rumable-optimize-nginx128.log`、`/tmp/rumable-optimize-apache2423.log`、`/tmp/rumable-optimize-apache2469.log`。

Go 1.20.14 / CGO=0 / GOAMD64=v1 的 Linux amd64 legacy 二进制在 OrbStack 仿真下也完成 Nginx 1.9.4 同一完整 smoke（`/tmp/rumable-optimize-legacy.log`）。该结果验证旧编译器产物与最低 Nginx 的组合，不代表旧内核实机。

此次没有修改生成的 HTML/SDK/服务器规则，未重复 Chrome 交互验收；此前浏览器证据见上文。Windows/IIS 实机和真实观测云入库仍未验收。

## 2026-10-20 Linux 第二轮审查

本轮复现并修复两项问题：

1. Nginx 的注入 map 只激活 GET，HEAD 可能沿用原文 Last-Modified、Content-Length。旧二进制在 1.9.4 上返回原静态 HTML 长度 87；现将 GET/HEAD 一并激活原生过滤器的头部清理，HEAD 仍无正文。使用正则 map 项避免较长 HEAD 键超过旧构建默认的 32 字节 hash bucket，不要求改全局 map_hash_bucket_size。
2. 初始化脚本、SDK 和外置 Worker 之前只验 identity，过期 gzip 脚本可能通过验收。现检查两种编码的 MIME，受管脚本解压后都须匹配快照摘要；外置 Worker 的解码内容须一致，不据此证明其 SDK 版本或业务正确性。共享资源仍按每种编码每轮一次检查，不跨轮复用。两页加一个 Worker 的边界用例现在为 10 次请求：两页、初始化脚本、SDK、Worker 各两种编码。

新增回归覆盖静态、反代及已热 proxy_cache 的 HEAD identity/gzip 元数据，停用后的 HEAD 条件请求恢复；Nginx 真实 gzip_static 过期初始化/SDK 文件触发启用失败后，原状态逐字节恢复且不遗留事务。Apache 新增继承 SetOutputFilter DEFLATE 的脚本路径，验证 gzip 确实生效、解压字节与受管文件一致。HTTP 边界测试分别构造 gzip 初始化脚本、SDK、Worker 不一致，并保持第二页压缩缺失、重复注入、后续轮次重新检查和历史 SDK 引用覆盖。

| 本轮验证 | 环境与结果 |
|---|---|
| Go test / vet | OrbStack Linux arm64，Go 1.20.14 / 1.26.8 全量通过 |
| Race | Go 1.26.8 全量通过 |
| Nginx | 1.9.4、1.28.3，Go 1.26.8 Linux arm64，完整 smoke 通过 |
| Legacy CLI | Go 1.20.14 / CGO=0 / GOAMD64=v1 的 Linux amd64 CLI，OrbStack 仿真执行 Nginx 1.9.4 完整 smoke 通过 |
| Apache | 2.4.23、2.4.69，Go 1.26.8 Linux arm64，完整 smoke 通过 |
| Chrome | 本机 Nginx 1.28.3 页面，初始化与 SDK 标签各 1 个，initialized、recording=false；Fetch 显示 Fetched；Console 只有预期测试错误，无 SDK/CSP/CORS 警告 |

完整 smoke 还覆盖压缩 HTML、普通 chunked、缓存启停恢复、继承业务过滤器/请求头/安全响应头、多站点隔离、幂等、升级回滚、配置测试/重载/HTTP 故障恢复及受管项漂移拒绝。没有对业务配置自动放宽或覆盖，也未改动 SDK、RUM 参数或状态格式。历史快照仍按原字节恢复，需用新包重新 enable 才会生成新的 Nginx HEAD 规则。

Chrome 本地接收端最终统计为 View 3、Resource 6、Action 3、Error 1，共 3 次接收请求；回放默认关闭，无回放片段。证据：`/tmp/rumable-linux-review-chrome.png`、`/tmp/rumable-linux-review-chrome-stats.json`，只证明本地发送。日志：`/tmp/rumable-linux-review-head-before.log`、`/tmp/rumable-linux-review-gzip-before.log`、`/tmp/rumable-linux-review-go.log`、`/tmp/rumable-linux-review-nginx194.log`、`/tmp/rumable-linux-review-nginx128.log`、`/tmp/rumable-linux-review-apache2423.log`、`/tmp/rumable-linux-review-apache2469.log`、`/tmp/rumable-linux-review-legacy.log`。

本轮未执行旧内核、真实 TLS/Brotli、任意第三方过滤器、Windows/IIS 实机和真实 OpenWay/DataKit 入库验收。

## 2026-10-20 精简接入流程

首次接入统一为 init → 填精简 TOML → render/首次挂接 → enable；enable 已包含预检、服务器配置检查、生效和 HTTP 探测，独立 check 作为可选诊断。init 默认生成约二十行配置；--server 支持 nginx/apache/iis，--reporting 支持 openway/datakit，--site 可设置初始 ID，--full 保留全部静态 SDK 选项。回放仍默认关闭，IIS 模板不预先允许压缩调整。

配置只有一个站点时 render/check/enable 自动选择；status/disable/rollback/probe 从单个受管站点选择，不依赖原始 TOML。配置或受管目录存在多个候选时要求 --site，不猜测站点。render 直接打印实际挂接位置、检查/首次重载命令和完整 enable 命令；完整 ATTACH.md 保留压缩、继承、权限与缓存说明。终端命令的参数按宿主 shell 转义，Nginx ATTACH 中 include 路径也加引号。

本轮通过：

- Go 1.20.14 / 1.26.8 全量 test、vet 和 Go 1.26.8 race。12 种服务器/上报方式/精简或完整模板组合均经真实 CLI Run 写盘并按严格 TOML 解析，验证自定义 ID、正确平台与上报模式、默认关闭回放及 IIS 压缩权限；无效选项不创建文件。
- 单站点 render、缺失源 TOML 时的 status、多站点配置及受管目录歧义拒绝，拒绝后文件、历史、事务不被修改。POSIX shell 实际执行含空格、引号、美元符号及命令替换字面量的参数，内容保持一致。
- OrbStack Nginx 1.9.4 / Go 1.26.8 Linux arm64 完整 smoke；夹具由真实 init 生成精简 DataKit 配置再填写站点值，后续所有命令省略 --site。覆盖首次挂接、压缩、缓存、继承、启停、升级回滚、提交失败、漂移及 1.9.4 风险阻断。
- OrbStack Apache 2.4.23 / Go 1.26.8 Linux arm64 完整 smoke，以及 Go 1.20.14 Linux amd64 legacy CLI 在 OrbStack 仿真下执行 Nginx 1.28.3 同一精简接入/恢复流程。

验收脚本在容器命名空间内原子替换配置，避免 macOS bind mount 观察到宿主截断写入中的半份 TOML；工具对这类不完整配置继续拒绝操作，不自动忽略解析错误。日志：`/tmp/rumable-onboarding-go-final.log`、`/tmp/rumable-onboarding-final-nginx194.log`、`/tmp/rumable-onboarding-final-apache2423.log`、`/tmp/rumable-onboarding-final-legacy.log`。

本轮为 CLI、模板及挂接提示变更，未重复 Chrome、Windows/IIS 实机或真实观测云入库验收。SDK、注入过滤算法、事务格式与服务器最低版本保持原有约束；之前的浏览器与 Linux 审查证据见前文，不能将模板验证算作 Windows/IIS 运行通过。

## 2026-10-20 自动挂接复审、SDK 同步与 CDN 加载

本轮直接修复并补充回归：

- FILE:LINE 对应同一行内多个 server 块时，显式 target 不再选第一个；全局扫描本站 ID 标记、保留路径与 Nginx 变量/Apache Lua 过滤器，阻止新状态目录复用其他部署的名称，同时允许不同 ID 共享名称前缀。
- 确认后复查部署状态、备份、全部源配置、符号链接绑定与 Linux 文件身份；提交期间复查未受管输入，原子替换检查属主/组、ctime 和权限变化。历史备份排他创建，已有不同内容时停止覆盖。
- 撤销和失败恢复先重新读取有效 include 图；目标文件后来被其他站点复用、源文件或链接漂移、无法确定配置归属时停止并保留记录。具体回归构造了新 include 引用同一 handler，验证拒绝撤销且原文件保持。普通启停仍只修改受管部署文件。
- 发布脚本默认下载官方 v3 SDK，并对照官方 NPM browser-rum 的 latest 稳定版本和 MIT 许可，验证完整版标记并记录版本/来源/SHA-256；异常、预发布/新主版本或 CDN 尚未同步时中止。--skip-sdk-sync 仅用于已校验源码的离线重建。两套编译器的 test、同步脚本测试和 vet 通过后才打包。
- 新增站点 [sites.sdk] source=bundled/cdn、可选 HTTPS url 和 integrity；默认 bundled。CDN 仍使用同域初始化脚本，SDK 配置、防重、回放及故障隔离不变；启用/探测验证 identity/gzip MIME、字节摘要和实际 CSP，SRI 检查跨域 CORS。快照保留实际 CDN URL 与摘要，历史版本按快照恢复。

实际通过的检查：

| 环境 | 本轮结果 |
|---|---|
| Go | Linux Go 1.20.14 / 1.26.8 全量 test、vet；独立 SDK 同步脚本边界测试；Go 1.26.8 race |
| Nginx | OrbStack 1.9.4 / 1.28.3 自动挂接，确认与备份并发变化、原配置精确恢复、静态预压缩、反代头继承、热缓存、站点隔离、启停回滚及三类故障恢复通过 |
| Legacy CLI | Go 1.20.14、CGO=0、GOAMD64=v1 的 Linux amd64 包，在 OrbStack 仿真下通过 Nginx 1.9.4 自动挂接全流程 |
| Apache | OrbStack 2.4.23 / 2.4.69，自动挂接全流程及继承业务规则隔离通过 |
| 人工挂接回归 | Nginx 1.9.4 与 Apache 2.4.23 的原完整 smoke 通过，覆盖压缩、缓存、继承、HEAD、启停、升级回滚与失败恢复 |
| CDN | Nginx 1.28.3 真实官方 CDN：同域→CDN→同域回滚、HTTP 双编码与状态 URL 检查通过；本机 Chrome CDN 初始化、默认回放关闭、开启回放、新会话、停用及回滚通过 |

2026-10-20 实际同步核对：官方 CDN 和 NPM latest 均为 3.3.21，SHA-256 为 67c7d58349e376864f3297e9e6d4a60453ede6ff0c6f6b1900a74c2a44d4d3b1，与内置文件一致，未虚构版本升级。官方地址当前未返回 Access-Control-Allow-Origin，因此默认 CDN 不启用 SRI；自建 CDN 的可选 SRI 属性经真实生成脚本 + Node VM 验证，HTTPS HTTP 边界测试覆盖有/无 CORS、同域无需 CORS、摘要错误、过期 gzip 和历史摘要。未对自建 SRI CDN 做浏览器实网验收。

Chrome 页面只出现一个同域 init 和一个 https://static.guance.com/browser-sdk/v3/dataflux-rum.js 标签。默认 initialized、recording=false；TOML 开启后新会话显示 replayStatus=requested、recording=true、回放采样 100。Fetch 正常，Console 仅有主动触发的测试错误。停用后只剩业务脚本；回滚后使用不命中旧 HTML 缓存的新 URL，恢复 CDN 与录制。最后本地接收端计数为 View 12、Resource 22、Action 7、Error 1、Long Task 1，5 个回放片段、3696 个片段字节、17 次请求；这些只证明本地发送，不证明观测云入库。

证据：/tmp/rumable-cdn-final-go.log、/tmp/rumable-cdn-final-nginx194.log、/tmp/rumable-cdn-final-apache2423.log、/tmp/rumable-cdn-final-apache2469.log、/tmp/rumable-cdn-final-legacy.log、/tmp/rumable-cdn-final-nginx128-cdn.log、/tmp/rumable-cdn-manual-nginx.log、/tmp/rumable-cdn-manual-apache.log、/tmp/rumable-cdn-chrome.png、/tmp/rumable-cdn-replay-chrome.png、/tmp/rumable-cdn-rollback-chrome.png、/tmp/rumable-cdn-chrome-stats.json。验证标签页、容器和网络已清理。

可执行复验：

```sh
go test ./...
go test scripts/sync-sdk.go scripts/sync-sdk_test.go
go run scripts/sync-sdk.go
SERVER=nginx NGINX_IMAGE=rumable-nginx:1.9.4 scripts/smoke-attach.sh
SERVER=apache APACHE_IMAGE=rumable-apache:2.4.23 scripts/smoke-attach.sh
CDN=1 SERVER=nginx NGINX_IMAGE=nginx:1.28-alpine scripts/smoke-attach.sh
# CDN=1 需要访问官方 HTTPS CDN；KEEP=1 保留本轮站点供 Chrome 使用。
```

Windows/IIS、旧内核、真实 OpenWay/DataKit 入库仍未执行；CDN 模式的外部文件版本、缓存与可达性不在工具恢复范围，不能用 HTTP 探测替代浏览器和真实入库验收。发布时仍需审查新版 SDK 的参数兼容性与实际浏览器行为；同步下载不是自动完成这项验收。

## 2026-10-20 · 0.1.0-preview.1 Windows / Linux 发布验证

本次 preview 范围为 Windows 和 Linux，不生成 macOS 包。共 10 个目标：Windows 386/amd64/arm64 现代包、Windows 386/amd64 legacy 包；Linux 386/amd64/arm64 现代包、Linux 386/amd64 legacy 包。386 是实际 32 位程序，amd64 是 x64。现代编译器为 Go 1.26.8，legacy 为 Go 1.20.14；CGO=0，GO386=sse2、GOAMD64=v1、GOARM64=v8.0。

候选构建已经实际通过：

- 两套 Go 编译器的全量 test / vet、SDK 同步边界测试与 ZIP/tar 交付契约测试；发布来源回归实际创建临时 Git 仓库，验证错误提交、工作区修改、暂存修改和未跟踪源码均不能标记为干净的旧提交。Go 1.26.8 race 通过。SDK 同步确认官方稳定版仍为 3.3.21，摘要与内置文件一致。
- Windows PE 的 386/amd64/arm64 Machine、Linux ELF 的 386/amd64/arm64 Machine 与构建元数据一致；包内可执行文件 SHA-256、preview 版本标记和 ZIP CRC 通过。此项是交叉编译与归档检查，不是 Windows 运行验收。
- 五个 Linux 包全部在 OrbStack 执行 version、完整 TOML init、Nginx/DataKit 和 IIS/OpenWay 的 render/status；核对架构、编译器、preview 版本、停用状态与文件摘要。
- Linux 386 分别用 Go 1.20.14 / 1.26.8 编译并运行全量测试，均通过；是 OrbStack 的 x86 用户态仿真，不代表原生旧内核验收。
- 实际候选二进制执行完整自动挂接 smoke：Linux 386 / Go 1.26.8 对 Nginx 1.9.4、Linux amd64 / Go 1.26.8 对 Apache 2.4.23、Linux arm64 / Go 1.26.8 对 Nginx 1.28.3（含 CDN 切换）均通过。覆盖确认、压缩、缓存、继承、隔离、启停回滚、故障恢复和完整撤销。
- Windows x86 选取 Sysnative 的路径与不具备 64 位管理宿主时的拒绝，经 Go 边界测试验证。Windows PowerShell 资产解析与可移植脚本边界在 OrbStack PowerShell 7.4 / Linux amd64 执行通过（设置 TEMP=/tmp，运行 legacy Linux CLI）；不据此认定 Windows PowerShell 3/5.1 或 IIS API 实测通过。

最终用户入口为 USER-GUIDE.md；每包自带平台 README、TOML 示例、兼容性、操作/恢复说明、版本说明、SDK 元数据及第三方许可。发布脚本将用户版本写入实际可执行文件，build.json 记录源码提交、编译器、目标和二进制摘要；下载目录包含索引与 SHA256SUMS。最终分发包需与这些已运行候选二进制逐字节对照，并核对归档文档对应提交的源码。

证据日志：/tmp/rumable-preview-candidate-build.log、/tmp/rumable-preview-go-preflight.log、/tmp/rumable-preview-x86-tests.log、/tmp/rumable-preview-race.log、/tmp/rumable-preview-nginx-x86.log、/tmp/rumable-preview-apache-x64.log、/tmp/rumable-preview-nginx-arm64-cdn.log、/tmp/rumable-preview-powershell-portable.log。Windows 所有架构、IIS 7.5/10、旧 Windows / Linux 内核及真实 OpenWay/DataKit 入库继续列为未验收；Windows ARM64 包尤其不能视为 ARM64 IIS 能力保证。

## 尚未完成的环境验收

- Windows 10 / Server 2016+ 与旧 Windows 7 / Server 2008 R2 / Server 2012 的真实运行。
- IIS 10 与旧 IIS 7.5/8/8.5 的配置 API、压缩、缓存、规则继承和恢复行为；提供 `scripts/windows-smoke.ps1`，在非生产 Windows 以管理员执行。
- 旧 Linux 内核运行。容器共享现代宿主内核，不能证明 2.6.32 内核兼容。
- 真实 OpenWay / DataKit 服务及观测云入库：本次没有用户提供的有效应用参数或接收端凭证。本地接收端成功不代表真实入库验收。
- IE、旧浏览器、流式 SSR、自定义 nonce/Trusted Types CSP 等场景。

Windows 验证入口：

```powershell
.\scripts\windows-smoke.ps1 -Binary .\rumable.exe
# -ExpectedIISVersion 7.5 / 10.0 核对目标版本；-Keep 可保留本轮独立站点。
```

该脚本只创建自己的回环地址测试站点；正常结束后清理所建资源，不改造现有生产站点。完整步骤、矩阵和退出码见 [Windows/IIS 验收手册](windows-acceptance.md)。脚本没有在本次 macOS 环境执行 Windows/IIS 实机流程。

## Windows 验收脚本扩展验证

- 扩展为三应用压缩基线、父子规则保留、同名继承冲突、缓存/保留映射冲突、幂等启停、版本化资源保留、配置升级回滚、受管项漂移拒绝、压缩源拒绝及提交后 HTTP 失败恢复；最后核对应用间隔离。PreviousBinary 可验证旧包部署与新包升级，StaticOnly 明确标为部分验收。
- 结果包含环境与二进制摘要、分阶段状态、CLI 日志、受管状态及本轮站点局部配置。默认清理本轮站点、池、location 节点和临时目录；独立证据保留，Keep 后由配套脚本核对归属清理。
- 在 OrbStack 的 Microsoft PowerShell 7.4 容器执行可移植边界测试，实际调用 Go 1.20.14 Linux CLI，验证 UTF-8 TOML 生成、false/0/回放配置、状态读取、部署历史比较、预期失败日志与成功误判保护。HTTP 夹具验证 gzip 解码、真实 chunked、503 状态/正文、拒绝重定向和非回环目标。
- 上述为脚本语法及辅助函数运行验证。Windows PowerShell 3/5.1、ASP.NET 夹具、Microsoft.Web.Administration API、旧 Windows、IIS 7.5/10 和清理 API 仍需按矩阵实机执行；不将 Linux PowerShell 结果算为 Windows 验收。
