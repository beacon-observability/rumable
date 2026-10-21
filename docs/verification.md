# 实现验证记录

## 0.1.4 简洁接入与 Windows 自动识别（2026-10-21）

- 本地 Go 全量 test/vet、发布归档测试与 28 项 Node 测试通过。新增自动发现配置严格解析、实际页面输出到 Go 的契约、单/多站点、绑定歧义、无终端拒绝、稳定 ID、已有路径保留、HTTPS 上报限制及 IIS 压缩许可等覆盖。
- OrbStack Nginx：`smoke-discovery.sh` 实际执行生成的一键命令，覆盖自动发现、首次注入、重复安装、配置更新/回滚、停用与精确撤销。双站点场景中，无终端不修改配置；带终端的管道安装只注入选中的站点，未选中站点不变。`smoke-onboarding.sh` 原手动模式也通过，含错误摘要拒绝及不支持 HTML 的失败后重试。此轮下载先使用本地包替身，校验/解压/安装/注入实际执行；脚本支持发布后以 `RUMABLE_PUBLIC_MANIFEST` 切换真实 OSS 下载验收。
- Chrome 验证默认无需网站地址即可生成 Linux/IIS 自动发现 TOML，高级手动模式才显示并校验网站字段；旧 0.1.2 清单明确拒绝自动发现命令。线上页面自动读取匹配版本清单。
- Windows PowerShell 契约测试在官方 Linux PowerShell 7.4 镜像及实际 CI 镜像通过：现代/legacy 与 x86/x64/ARM64 选包、WOW64 原生目录、原生桥接编译、发现参数传递、真实 ZIP 与 SHA-256 校验、重复安装、错误输入和失败清理。早期 CI 镜像补齐 ICU 后本地测试通过，但 0.1.3 流水线 319770 在 Runner 下载 apt 依赖时停滞、尚未上传 OSS；因此取消该次任务，0.1.4 改为官方 PowerShell 独立容器执行同一测试，去掉额外 apt 依赖。Windows API、IIS 和 Windows PowerShell 5.1 未在本机执行，需按 Windows 验收手册完成原生测试。
- 发布器测试覆盖固定版/latest HTML 的 MIME 与 inline 头；使用自定义域名访问配置页，OSS 原始域名的强制下载策略不能由页面 HTML 修复。公开文件清单仍为 19 个固定版文件加 4 个 latest 入口，无测试源码上传。

实际发布与公开下载结果以对应 tag 流水线、release-report.json 及公开地址验收记录为准。HTTP 注入验证不等于浏览器真实事件已入库。

## 0.1.2 分发范围修正（2026-10-21）

本地 Go 归档回归及 27 项 Node 测试通过。“额外文件具有正确校验和仍须拒绝上传”的发布回归覆盖 Go 测试源码、PowerShell 验收脚本、ASPX 夹具、页面测试、额外文档和隐藏配置文件；均在任何 OSS 写入前拒绝。包内文档的相对链接检查通过。Windows 验收工具改从同版本源码 tag 获取。

本轮不更改注入、启停或回滚逻辑。CI 重新运行两套 Go 编译器的 test/vet、Node 发布回归，构建并校验全部 10 个平台包；实际发布结果以对应流水线和 release-report.json 为准。Windows/IIS 实机与真实观测云入库仍需目标环境验收。


## 0.1.1 latest 与 CI 依赖下载（2026-10-21）

- 20 项 Node 测试通过：latest 清单生成的 Linux / Windows 命令绑定版本目录；安装器将 latest base 转为脚本固定版本；入口仅在不可变清单验证后发布，禁止旧版回退，预发布跳过；更新失败保留旧清单、重试修复，摘要回读失败拒绝完成。
- Go 全量测试与发布归档/安装器契约测试通过；shell 语法及 diff 检查通过。OrbStack Nginx 的完整一键安装流程从 latest 清单生成命令，校验、安装、预检失败重试、幂等、升级回滚、停用及精确撤销通过。日志 `/tmp/rumable-latest-onboarding-smoke.log`，OSS 下载仍使用本地替身。
- 首个 `release-oss-v0.1.0` 流水线 319659 在下载 Go 依赖时连接 proxy.golang.org 超时，尚未进入打包和 OSS 上传。后续 CI 显式传入 GOPROXY / GOSUMDB，默认使用 goproxy.cn 并保留摘要校验；旧 tag 保留，不覆盖。
- Windows 安装器原生执行、IIS 及观测云入库仍由目标 Windows 环境验收。发布成功只证明构建、文件上传和入口校验，不代表上述运行验收通过。

## 0.1.0 短命令安装器与本地测试包（2026-10-21）

- 安装逻辑独立为 `install.sh` / `install.ps1`，版本号使用 `0.1.0`。CI 不再要求公开下载域名；页面从清单地址推导下载目录。Linux 安装脚本自动识别 386 / amd64 / arm64，Windows 一键入口使用 amd64。
- 页面、CI 与安装器边界的 10 项 Node 测试通过。真实 shell 覆盖 32 位及 64 位架构选择、非 root、非法地址、无效配置和缺失参数拒绝；Go 发布安装器契约测试及 shell 语法检查通过。
- 最终 smoke-onboarding 在 OrbStack Nginx 容器通过：实际执行短命令读取脚本、归档摘要校验、安装、首次 setup、预检失败重试、幂等、升级、回滚、停用与精确撤销。下载替身提供本地脚本与归档，没有连接真实 OSS。日志 `/tmp/rumable-short-installer-smoke-final.log`。
- 本机 Go 1.26.1 / Go 1.20.14 生成全部 10 个平台归档，候选版本 `0.1.0`、revision `uncommitted`。46 个发布文件（含两个安装器、配置页和清单）通过实际 SHA256SUMS / manifest 校验，内存 OSS 替身完成 46 个文件上传回读，manifest 最后处理。日志 `/tmp/rumable-short-installer-release.log`。
- Chrome 复核当前页面无 PREVIEW 标记，显示 Linux 32 位 / x64 / ARM64 自动识别提示，未发现控制台错误。
- 新增发布打 tag 脚本后，全部 15 项 Node 测试通过。5 组实际临时 Git / bare remote 用例验证只推送指定附注 tag、预演不写入、版本与未提交文件拒绝、同名 tag 冲突拒绝、服务端拒绝后重试，以及独立 push URL / 多目标检查；未对项目远端推送。发布说明补入压缩包，避免包内一键接入手册链接缺失。

Windows 包仍仅完成构建与包校验，未执行 Windows PowerShell 5.1、`install.ps1` 或 IIS 原生验收；详见 [Windows 验收流程及 OrbStack 边界](windows-acceptance.md)。本地测试包来自未提交源码，不代表 Git tag 或 OSS 已发布。真实 OSS / GitLab Runner 与观测云入库仍待验收。

## 注入、IIS 恢复与页面接入（2026-10-21，未发布源码）

- Go 1.26.1 / Go 1.20.14：全量 test/vet 通过。新增 HTML 上下文、伪标签、歧义锚点、IIS 保留映射后恢复重试及外部变更拒绝回归；Web 页面输出通过实际 Go TOML/SDK 配置校验。测试约 15 秒，其中 IIS 故障恢复用例包含真实探测重试等待。
- Nginx 1.28.3 / Docker ARM64：完整 smoke-nginx 通过，涵盖静态/反代、分块开始标签、identity/gzip、缓存/HEAD、启停、升级回滚和配置/重载/HTTP 故障恢复。原始 `window.template="</head>"` 字符串保持完整。
- Chrome 实际打开 `/script-head`：可见 `Business script passed`，单个 `script[data-rumable]`，初始化状态 `initialized`，没有 JavaScript 错误。验收夹具的业务内联脚本使用测试 nonce；SDK 使用同域资源。
- 页面浏览器验收：OpenWay/DataKit 互斥、回放开关和小数采样、超范围采样拒绝、IIS 字段展开、会话存储回退联动及 TOML 预览均通过。Node 生成器回归通过，验证配置转义、清单平台/路径/摘要、Linux shell 语法与 Windows 命令内容。
- smoke-onboarding：在临时 Nginx 容器中执行页面生成的实际 Linux 命令，覆盖安装包校验失败不改站点、首次 setup、预检失败后重试、幂等、升级、回滚、停用和恢复原配置。下载以本地归档替代 OSS，校验、解包、安装和 CLI/Nginx 路径实际执行。
- CI/发布逻辑：9 项 Node 测试通过（页面 4 项、CI 5 项），涵盖受保护 tag/版本校验、缺包/坏摘要、远端冲突、失败后重试、回读失败不发布清单及报告不含凭据。Shell 语法检查与 CI YAML 解析通过。
- 实际候选打包：本机 Go 1.26.1 / Go 1.20.14 生成全部 10 个平台包；候选标识为 `0.1.0-preview.4 / uncommitted`，不代表已准备/发布该版本。44 个发布文件（含配置页）通过真实 SHA256SUMS/manifest 预检，并用内存 OSS 替身验证上传/回读顺序，manifest 最后处理。日志 `/tmp/rumable-onboarding-release-check.log`。

Windows/IIS 原生 API、PowerShell 安装命令实机运行、旧系统内核以及真实观测云入库未在本轮验收；IIS 恢复仍为原生桥接 mock 边界测试。没有发布 OSS 包，因此未验证真实 OSS 下载/CORS/CI 上传。CI 镜像构建因镜像下载过慢在本地停止，完整容器流程及 GitLab Runner 尚未运行通过。当前文本过滤器要求标准 `<head>`；探测覆盖不能替代所有动态 HTML 模板的约束。

日期：2026-10-19–20（Asia/Singapore）。此记录区分代码检查、本地运行与真实观测云验收。

## 0.1.0-preview.3 发行候选复验（2026-10-20）

本版汇总以下历次修复、OTel 自动安装/卸载及 Apache 移除；旧 preview.1/.2 压缩包保留。此处使用发布脚本生成的 `preview3-rc1` 包内程序验证，不是另行构建的调试程序。正式 dist 在源码提交后生成，并逐个核对最终可执行文件与候选相同；最终源码提交、标签及归档摘要另存本版交付校验记录。

| 实际执行范围 | 结果与限制 |
|---|---|
| 两套编译器 | OrbStack Linux arm64：Go 1.26.9、Go 1.20.14 均完成全量 test/vet、SDK 同步测试和发布归档契约测试 |
| SDK 同步 | 在线同步并校验官方稳定 v3，仍为 3.3.21，SDK 字节未变；SHA-256：`67c7d58349e376864f3297e9e6d4a60453ede6ff0c6f6b1900a74c2a44d4d3b1` |
| 10 个候选归档 | 校验 SHA256SUMS、ZIP CRC、PE/ELF 机器类型、build.json 的版本/编译器/二进制摘要、Linux 执行位、包内文档链接；均通过 |
| 五个 Linux 包 | 现代 x86/x64/ARM64、legacy x86/x64 均实际运行 version；生成 Nginx/IIS、OpenWay/DataKit 四类配置，并对 Nginx/IIS OTel 示例执行 render/status。x86/x64 使用 OrbStack 仿真，不能证明旧内核兼容 |
| Nginx 1.9.4 RUM | 现代 Linux x86 发布程序完整 smoke 通过：静态/反代/普通 chunked、identity/gzip、缓存验证头、HEAD、父层过滤器及业务头继承、独立停用保留旧脚本、升级/回滚、提交和重载失败恢复、文件漂移拒绝 |
| Nginx 1.9.4 自动挂接 | legacy Linux x64 发布程序完整脚本通过：站点隔离、精确差异与确认、幂等、压缩/热缓存、确认期间并发变化拒绝、启停/回滚、失败恢复、原始配置精确撤销和备份保留 |
| Nginx OTel | Linux ARM64 发布程序：Nginx 1.25.3/module 0.1.0 与 1.28.3/module 0.1.2 两套完整脚本通过。真实 Span 经 Collector 0.163.1 到私有 OTLP 接收器；覆盖 DataKit gRPC/Dataway protobuf 分支、采样、站点/路径隔离、共享 exporter、独立启停、接收端停机时 RUM 操作、分离 main/http 配置挂接、失败恢复、部分恢复重试、回滚和卸载 |
| 自动安装/依赖清理 | 1.25.3 使用临时容器内 APT 测试仓库，实际下载、SHA 校验、私有解包和模块装载检查通过；重复安装、卸载及未共享受管依赖删除通过。不代表远程软件源/其他发行版 ABI 已验收 |
| IIS PowerShell | OrbStack Linux amd64 / PowerShell 7.4：快照恢复、OTel 独占池契约 mock、脚本解析与真实 Linux x64 CLI 的可移植边界测试通过；TEMP=/tmp。未执行 Windows PowerShell 或 IIS 原生 API |

本机日志位于 `/tmp/rumable-otel-research/`：`preview3-candidate-build.log`、`preview3-matrix.log`、`preview3-rum194.log`、`preview3-attach194.log`、`preview3-otel125.log`、`preview3-otel128.log`、`preview3-powershell.log`。使用仓库现有 smoke/test 脚本，RUMABLE_BINARY 指向包内程序；APT 流程另加 INSTALL_OTEL=1、NGINX_APT_FIXTURE=1。命令示例见下方各功能验证章节。

Windows 的五个包仅完成交叉编译、构建元数据和 PE 格式检查；Windows/IIS/.NET Agent 实机、旧操作系统内核及真实 DataKit/Dataway/观测云入库仍未验收。私有接收器证明协议分支和 Trace 发送，不代表观测云数据已入库。SDK 本体、初始化逻辑和页面交互未改动，本轮未重复 Chrome 验收。以下“未发布源码”均为本次打包之前的历史阶段记录。

## 功能缺陷复审第二轮（2026-10-20，未发布源码）

本轮继续只检查功能正确性和恢复，不开展安全审查。实际复现并修复两项会阻断自动部署/恢复的问题：

| 问题 | 修复前实际结果 | 修复与回归 |
|---|---|---|
| http 块通过主配置后部的 include 引入，OTel 模块入口插错文件 | Nginx 1.25.3 报 `load_module directive is specified too late in /work/http.conf:3`；首次挂接失败，原文件被恢复 | 保存实际主配置文件位置，将模块入口插在主文件开头。完整 OTel 脚本改用 main/http 两个文件，并在预览、安装及撤销后逐个比较原字节 |
| 多文件恢复中断后，磁盘配置暂时不可解析，重试被永久阻止 | 先恢复 main 并保留已挂接的 http，将记录置为 restoring，真实 nginx -t 报 unknown directive otel_trace；随后 rollback --attachment 因不能执行原生配置检查而拒绝继续 | applying/restoring 两种未完成状态均可在核对原输入及 Before/After 字节后继续恢复；完整回归实际制造上述中断，再运行 rollback --attachment，恢复两文件和 detached 状态，随后卸载成功 |

本轮实际验证：

- 本机 Go 1.26.1、OrbStack Linux arm64 Go 1.20.14：全量 test / vet 通过；发布归档契约测试通过。
- OrbStack Nginx 1.25.3 / ngx_otel_module 0.1.0、Collector 0.163.1：使用真实原生模块、临时 APT 测试仓库和私有 OTLP 接收器，完整 OTel 流程通过；覆盖双文件挂接、DataKit/Dataway 协议分支、Span、路径/站点隔离、独立启停、共享 exporter、重载失败恢复、部分恢复重试、卸载和依赖清理。并不表示真实 DataKit/Dataway 账户入库已验收。
- OrbStack Nginx 1.9.4：完整自动挂接脚本通过，包含压缩、热缓存、业务头继承、确认期变更拒绝、启停/回滚、三类提交故障恢复、精确撤销及备份保留。
- Go 1.20.14 / CGO=0：Windows/Linux x86、x64、ARM64 六个平台交叉编译通过；Windows 可执行文件未实机运行。

复现日志：`/tmp/rumable-otel-research/review2-before-split-http.log`、`review2-before-restore-retry.log`。通过的完整日志：同目录 `review2-fixed-otel125.log`、`review2-fixed-attach194.log`。复验使用现有 smoke-otel.sh 与 smoke-attach.sh，RUMABLE_BINARY 指向本轮源码构建的 Linux arm64 程序。未修改浏览器 SDK/页面交互，未重复 Chrome 验收；IIS/Windows 实机及真实观测云入库仍未验收。未重新生成 dist。

## 功能缺陷复审（2026-10-20，未发布源码）

本轮只检查功能正确性、独立启停与部署恢复，不开展安全审查。复现并修复三项问题：

| 缺陷 | 修复前证据 | 修复及回归 |
|---|---|---|
| Linux RUM 单组件操作依赖 OTel 接收端 | 实际停止 Collector 后，`disable --component rum` 报 `OTel receiver TCP connection failed`，尚未提交 RUM 变更 | RUM 单组件预检不重新校验未变更的 OTel 接收端；原生配置检查和 HTTP 生效探测保留。现有 OTel 全流程增加 Collector 停止期间对依赖站点开启、关闭 RUM 的断言 |
| RUM 单独停用改变 Nginx 过滤器继承 | Nginx 1.9.4 上，单独停用后旧初始化脚本第 18 字节被父层业务过滤器改写，HTTP 200 仍成功，缓存页面可能无法初始化 | 已准备 RUM 的部署始终保留动态空匹配过滤器；纯 OTel 首次部署仍不添加 RUM 过滤器。完整 RUM 脚本新增父层规则与停用后的原始脚本字节比较，1.9.4 和 1.28.3 均通过 |
| OTel 单组件启用意外升级浏览器 SDK | Go 真实部署事务使用模拟 IIS API，预置旧 SDK/初始化脚本后，OTel 单独启用改变其版本和 URL，回归用例失败 | 从已部署快照保留浏览器版本、脚本、资源摘要和 CDN 引用，服务器规则使用保存的脚本 URL；预检不再拿当前 CLI 的 SDK 校验未修改的旧部署。回归覆盖同域和 HTTPS CDN，执行真实 HTTP、事务落盘和状态读取 |

实际完成的验证：

- 本机 Go 1.26.1、OrbStack Linux arm64 Go 1.20.14：全量 test、vet 通过；发布归档契约测试通过。OrbStack Go 1.26.9 全量 race 检查通过。
- OrbStack Nginx 1.9.4、官方 nginx:1.28-alpine（1.28.3）：完整 RUM smoke 通过，含静态/反代/chunked、identity/gzip、缓存、HEAD、继承、独立停用、升级回滚、故障恢复及漂移拒绝。
- OrbStack Nginx 1.25.3 / ngx_otel_module 0.1.0、Collector 0.163.1：真实 Span 经 Collector 到私有 OTLP 接收器全流程通过，包括临时 APT 仓库的模块安装、Collector 不可达期间的 RUM 独立启停、共享 exporter、回滚、失败恢复和卸载。Collector 故障用例置于 Span 接收断言之后；不承诺原生 SDK 在接收端停机期间不丢弃 Span。
- Go 1.20.14 / CGO=0：Windows/Linux 的 x86、x64、ARM64 六个平台构建通过，仅为编译验证。

修复前运行日志：`/tmp/rumable-otel-research/review-before-rum.log`、`review-before-otel.log`。修复后的完整日志：同目录 `review-fixed-rum194.log`、`review-fixed-rum128.log`、`review-fixed-otel125.log`。复验使用现有 `scripts/smoke-nginx.sh`、`scripts/smoke-otel.sh`，命令与后续对应验证章节相同，RUMABLE_BINARY 指向本轮源码构建程序。

Windows/IIS 实机、真实 DataKit/Dataway/观测云入库仍未验收；IIS 结果为 Go 事务与 API mock，不等于原生配置提交/进程回收实测。未修改 Web SDK 本体、初始化逻辑或页面交互，未重复 Chrome 验收。未重新生成 preview dist。

## 移除 Apache 支持（2026-10-20，未发布源码）

当前源码只支持 Nginx 和 IIS。已删除 Apache 的 RUM/OTel 配置类型、渲染、模块安装/构建、自动挂接、原生检查、Lua 资源、专用测试、示例和验收脚本；共享站点选择中的 VirtualHost 分支与 Lua 名称检查也已移除。不保留旧配置或状态的兼容分支。发布清单与当前使用文档同步更新；既有 preview 压缩包、标签及其历史交付记录不重写。

| 本轮实际验证 | 环境与结果 |
|---|---|
| Go 行为测试与 vet | 本机 Go 1.26.1、OrbStack Linux arm64 Go 1.20.14 全量通过；发布归档契约测试通过 |
| Nginx RUM | OrbStack arm64：1.9.4 完整 RUM smoke 通过，覆盖静态/反代/chunked、identity/gzip、缓存验证头和 HEAD、配置继承、启停/升级/回滚、失败恢复及漂移拒绝 |
| Nginx 自动挂接 | 1.9.4、官方 1.28-alpine（1.28.3）完整脚本通过；覆盖站点隔离、压缩/热缓存、确认期间配置和备份并发变化拒绝、失败恢复、精确撤销、原文件模式及备份保留 |
| Nginx OTel 安装与卸载 | 1.25.3 / 官方模块 0.1.0、Collector 0.163.1：离线 deb 与临时容器内 APT 测试仓库两种安装流程均通过；重复安装、真实 Span、DataKit gRPC/Dataway protobuf 分支、独立启停、共享 exporter、重载失败恢复、回滚、归档和依赖移除通过。不代表远程 APT 仓库签名或真实后端入库验收 |
| IIS 保护 | PowerShell 7.4 / Linux amd64：快照、OTel 池契约 mock、脚本解析及真实 Linux CLI 的可移植边界测试通过；未执行 Windows/IIS API |
| 六个平台编译 | Go 1.20.14 / CGO=0：Windows 与 Linux 的 x86、x64、ARM64 构建通过；Windows 程序未实机执行，未生成新 dist |
| 文档与脚本 | 当前 Markdown 本地文件链接检查、受影响 Bash 脚本语法检查及 git diff --check 通过 |

使用现有脚本复验，RUMABLE_BINARY 指向当前源码构建的 Linux arm64 程序：

```sh
NGINX_IMAGE=rumable-nginx:1.9.4 bash scripts/smoke-nginx.sh
NGINX_IMAGE=rumable-nginx:1.9.4 bash scripts/smoke-attach.sh
NGINX_IMAGE=nginx:1.28-alpine bash scripts/smoke-attach.sh
INSTALL_OTEL=1 NGINX_IMAGE=rumable-nginx-otel:1.25.3 \
  NGINX_OTEL_ARCHIVE=/absolute/path/nginx-module-otel.deb \
  OTELCOL_BINARY=/absolute/path/otelcol PYTHON=python3.12 bash scripts/smoke-otel.sh
# 后一命令增加 NGINX_APT_FIXTURE=1 覆盖容器内实际 APT 流程。
```

本机运行日志：`/tmp/rumable-otel-research/remove-apache-rum194.log`、`remove-apache-nginx194.log`、`remove-apache-nginx128.log`、`remove-apache-otel.log`、`remove-apache-otel-offline.log`（后四个同目录）。未改变 Web SDK、初始化和页面交互逻辑，未重复 Chrome 验收；Windows/IIS 实机、旧内核及真实观测云入库仍未验收。以下为此前各轮的验证记录，不表示本轮重新执行。

## OTel 自动安装与卸载（2026-10-20，未发布源码）

新增 `install-otel` 和 `uninstall`；已有 preview.2 dist 保持不变。用户步骤见[安装与卸载](otel-installation.md)。本轮实际执行：

| 范围 | 结果及边界 |
|---|---|
| Go 行为测试 / 检查 | 本机 Go 1.26.1、OrbStack Linux Go 1.20.14 全部 Go 测试通过；vet 通过；发布脚本测试通过 |
| 归档与文件归属 | ZIP/tar 路径穿越、Windows 路径别名、链接、重复条目、错误 SHA、未知/修改文件及新增目录拒绝；共享站点/回滚引用阻止依赖删除；模拟丢失状态但 Agent 仍有现场引用时拒绝清理；IIS 权限设置未完成会阻止预检；确认期间状态改变拒绝 |
| Nginx 1.25.3 ARM64 | OrbStack，官方 module 0.1.0 `.deb` 的离线安装，以及真实 apt-cache / apt-get / dpkg-deb 安装路径均通过。APT 用临时容器内的本地测试仓库，不代表远端仓库配置/签名验收。识别官方 `nginx-r1.25.3` 虚拟 ABI 依赖；原生加载检查、重复安装、dry-run、真实 Span、RUM/OTel 独立启停、共享 exporter、重载失败恢复、回滚、归档及依赖移除通过，最终业务配置与原文件相同 |
| Nginx 1.9.4 ARM64 | 实际容器执行 install-otel，下载前拒绝 OTel，未创建依赖目录；原有 RUM 支持范围不变 |
| IIS 恢复边界 | Go 测试执行真实部署事务和日志，模拟 Windows API：卸载提交/激活失败恢复并可重试，归档保留，原先已存在的映射及资源不被移走；PE 边界拒绝错误位数的 profiler |
| PowerShell | OrbStack Linux amd64 / PowerShell 7.4：真实 Restore-Nodes 函数验证普通 rollback 保留映射、完整卸载移除新建映射、拒绝其他部署的映射；OTel 池契约模拟及可移植脚本测试通过。可移植测试设置 TEMP=/tmp，使用 Linux legacy CLI。不能据此认定 Windows PowerShell 5.1 或 IIS API 已实测 |
| 交叉编译 | Go 1.20.14 构建 Windows/Linux 的 386、amd64、arm64 均通过；这些是编译验证，未生成新 preview dist，Windows 可执行文件没有实机运行 |

Linux 安装模式复用 `scripts/smoke-otel.sh`，并验证原生 Trace 经 Collector 发送到本地 OTLP/protobuf 接收夹具。Nginx 测试 `.deb` SHA-256 为 `7a39abec8ac3b0f3f00f03d26c2264915bb9c48ce1e1a638f41a8407dd8bcf74`。可在已有匹配测试镜像和 Collector 的 OrbStack 环境执行：

```sh
# Nginx 离线安装；加 NGINX_APT_FIXTURE=1 覆盖实际 APT 流程（仅临时容器内添加测试源）。
INSTALL_OTEL=1 NGINX_IMAGE=rumable-nginx-otel:1.25.3 \
  NGINX_OTEL_ARCHIVE=/absolute/path/nginx-module-otel.deb \
  OTELCOL_BINARY=/absolute/path/otelcol PYTHON=python3.12 bash scripts/smoke-otel.sh
```

仍待目标环境验收：Windows 官方 Agent 下载、解包、IIS_IUSRS ACL、锁定 DLL 清理、IIS 实际应用池与 .NET 运行；非 Debian/APT 的自定义模块分发；真实 DataKit/Dataway 鉴权和观测云入库。本次没有修改浏览器 SDK 或初始化逻辑，未重复执行浏览器验收。

## 首次挂接自动化（2026-10-20）

新增 `attach`（检测、差异、确认、挂接）及 `rollback --attachment`（独立撤销/恢复），保留原来的人工挂接与部署 rollback。实际验证：

| 范围 | 本轮结果 |
|---|---|
| Go 与文件边界 | OrbStack Linux arm64，Go 1.20.14 / 1.26.8 test、vet，1.26.8 race 全部通过；原子替换保留模式、属主/组和 user xattr，先清除临时文件继承的默认 ACL，防止扩大读取权限；拒绝直接替换符号链接/硬链接及外部内容；两文件只写完一个时可恢复 |
| Nginx 最低 / 现代版 | 1.9.4 原始源码镜像、官方 1.28.3，自动挂接完整脚本通过；包括静态预压缩、输出 gzip、静态/反代/chunked、上游业务请求头保留、热缓存绕过和撤销后恢复缓存行为 |
| Legacy CLI | Go 1.20.14 Linux amd64 包在 OrbStack amd64 仿真下对 Nginx 1.28.3 完整自动挂接脚本通过；不代表旧 Linux 内核已运行验证 |
| 原有流程回归 | `smoke-nginx.sh` 在 1.9.4 通过，人工挂接、业务过滤规则、压缩、缓存、继承、升级及恢复流程保留 |
| Chrome | 本机真实页面初始化一次；DOM 中一个 Rumable 初始化脚本、一个 SDK；交互正常，控制台仅主动触发的测试异常。默认 replay 关闭，本地接收端记录 View 2 / Resource 6 / Action 1 / Error 1，无 replay；这是本地接收，不是观测云入库 |

自动挂接脚本同时覆盖：dry-run 不改业务文件、取消不提交、确认期间业务文件变更即拒绝、配置检查/重载/提交后 HTTP 三类失败恢复原字节、重复挂接不重载、站点隔离、启停/普通 rollback/完整撤销、外部编辑时停止覆盖、原始文件模式保留、备份保留及再次挂接。Nginx 1.9.4 还验证 http 层 sub_filter 在创建挂接备份/写业务文件之前即被阻止。

原生 include 回归覆盖 Nginx 共享源文件拒绝编辑、父/局部 proxy_set_header 列表、同内容 handler 的独立标记。未知 include 上下文和不安全合并拒绝。缓存恢复验证等待平滑重载的新旧 worker 交替完成，检查恢复缓存命中行为，不要求保留某一条旧缓存项。

可复现步骤（本机已安装 Go、OrbStack Docker；脚本创建并清理自己的容器）：

```sh
# 最低版本镜像只需构建一次；使用仓库固定来源/校验的 Dockerfile。
docker build -t rumable-nginx:1.9.4 -f scripts/nginx-1.9.4.Dockerfile .
NGINX_IMAGE=rumable-nginx:1.9.4 bash scripts/smoke-attach.sh
NGINX_IMAGE=nginx:1.28-alpine bash scripts/smoke-attach.sh
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

## 2026-10-20 优化复验

修复 Nginx 初始停用快照保留 attachmentReviewed=false 时重复 disable 被拒绝的问题：只在启用检查中要求人工挂接确认，停用仍检查模块、有效 include、配置和文件归属。旧二进制在 OrbStack / Nginx 1.9.4 上已复现该失败；扩展 scripts/smoke-nginx.sh，覆盖首次 false 拒绝 check/enable 且不创建事务或 reload、初始重复停用、首次启用后回滚到 false 快照、再次停用，以及候选配置失败后恢复该快照。

多页面探测改为每轮对相同脚本和 Worker 只检查一次，HTML 的 identity / gzip、注入版本及 CSP 仍逐页检查，不跨轮缓存。扩展现有 HTTP 边界测试：两个页面和一个共用 Worker 的请求由 10 次减少到 7 次；第二页 gzip 注入缺失、后续轮次脚本 MIME 改变、重复注入及历史 SDK 引用仍被正确处理。这个计数是脚本 gzip 校验扩展前的阶段结果，不代表端到端耗时测量；当前请求组成见下一节。

OrbStack 的 Go 1.20.14、1.26.8 全量 test / vet 与 Go 1.26.8 race 通过。使用此次 Go 1.26.8 Linux arm64 二进制完成 Nginx 1.9.4、1.28.3 的完整 smoke，含压缩、缓存、配置继承、幂等、升级/回滚、提交失败及漂移拒绝。日志：`/tmp/rumable-optimize-before.log`、`/tmp/rumable-optimize-go.log`、`/tmp/rumable-optimize-nginx194.log`、`/tmp/rumable-optimize-nginx128.log`。

Go 1.20.14 / CGO=0 / GOAMD64=v1 的 Linux amd64 legacy 二进制在 OrbStack 仿真下也完成 Nginx 1.9.4 同一完整 smoke（`/tmp/rumable-optimize-legacy.log`）。该结果验证旧编译器产物与最低 Nginx 的组合，不代表旧内核实机。

此次没有修改生成的 HTML/SDK/服务器规则，未重复 Chrome 交互验收；此前浏览器证据见上文。Windows/IIS 实机和真实观测云入库仍未验收。

## 2026-10-20 Linux 第二轮审查

本轮复现并修复两项问题：

1. Nginx 的注入 map 只激活 GET，HEAD 可能沿用原文 Last-Modified、Content-Length。旧二进制在 1.9.4 上返回原静态 HTML 长度 87；现将 GET/HEAD 一并激活原生过滤器的头部清理，HEAD 仍无正文。使用正则 map 项避免较长 HEAD 键超过旧构建默认的 32 字节 hash bucket，不要求改全局 map_hash_bucket_size。
2. 初始化脚本、SDK 和外置 Worker 之前只验 identity，过期 gzip 脚本可能通过验收。现检查两种编码的 MIME，受管脚本解压后都须匹配快照摘要；外置 Worker 的解码内容须一致，不据此证明其 SDK 版本或业务正确性。共享资源仍按每种编码每轮一次检查，不跨轮复用。两页加一个 Worker 的边界用例现在为 10 次请求：两页、初始化脚本、SDK、Worker 各两种编码。

新增回归覆盖静态、反代及已热 proxy_cache 的 HEAD identity/gzip 元数据，停用后的 HEAD 条件请求恢复；Nginx 真实 gzip_static 过期初始化/SDK 文件触发启用失败后，原状态逐字节恢复且不遗留事务。HTTP 边界测试分别构造 gzip 初始化脚本、SDK、Worker 不一致，并保持第二页压缩缺失、重复注入、后续轮次重新检查和历史 SDK 引用覆盖。

| 本轮验证 | 环境与结果 |
|---|---|
| Go test / vet | OrbStack Linux arm64，Go 1.20.14 / 1.26.8 全量通过 |
| Race | Go 1.26.8 全量通过 |
| Nginx | 1.9.4、1.28.3，Go 1.26.8 Linux arm64，完整 smoke 通过 |
| Legacy CLI | Go 1.20.14 / CGO=0 / GOAMD64=v1 的 Linux amd64 CLI，OrbStack 仿真执行 Nginx 1.9.4 完整 smoke 通过 |
| Chrome | 本机 Nginx 1.28.3 页面，初始化与 SDK 标签各 1 个，initialized、recording=false；Fetch 显示 Fetched；Console 只有预期测试错误，无 SDK/CSP/CORS 警告 |

完整 smoke 还覆盖压缩 HTML、普通 chunked、缓存启停恢复、继承业务过滤器/请求头/安全响应头、多站点隔离、幂等、升级回滚、配置测试/重载/HTTP 故障恢复及受管项漂移拒绝。没有对业务配置自动放宽或覆盖，也未改动 SDK、RUM 参数或状态格式。历史快照仍按原字节恢复，需用新包重新 enable 才会生成新的 Nginx HEAD 规则。

Chrome 本地接收端最终统计为 View 3、Resource 6、Action 3、Error 1，共 3 次接收请求；回放默认关闭，无回放片段。证据：`/tmp/rumable-linux-review-chrome.png`、`/tmp/rumable-linux-review-chrome-stats.json`，只证明本地发送。日志：`/tmp/rumable-linux-review-head-before.log`、`/tmp/rumable-linux-review-gzip-before.log`、`/tmp/rumable-linux-review-go.log`、`/tmp/rumable-linux-review-nginx194.log`、`/tmp/rumable-linux-review-nginx128.log`、`/tmp/rumable-linux-review-legacy.log`。

本轮未执行旧内核、真实 TLS/Brotli、任意第三方过滤器、Windows/IIS 实机和真实 OpenWay/DataKit 入库验收。

## 2026-10-20 精简接入流程

首次接入统一为 init → 填精简 TOML → render/首次挂接 → enable；enable 已包含预检、服务器配置检查、生效和 HTTP 探测，独立 check 作为可选诊断。init 默认生成约二十行配置；--server 支持 nginx/iis，--reporting 支持 openway/datakit，--site 可设置初始 ID，--full 保留全部静态 SDK 选项。回放仍默认关闭，IIS 模板不预先允许压缩调整。

配置只有一个站点时 render/check/enable 自动选择；status/disable/rollback/probe 从单个受管站点选择，不依赖原始 TOML。配置或受管目录存在多个候选时要求 --site，不猜测站点。render 直接打印实际挂接位置、检查/首次重载命令和完整 enable 命令；完整 ATTACH.md 保留压缩、继承、权限与缓存说明。终端命令的参数按宿主 shell 转义，Nginx ATTACH 中 include 路径也加引号。

本轮通过：

- Go 1.20.14 / 1.26.8 全量 test、vet 和 Go 1.26.8 race。12 种服务器/上报方式/精简或完整模板组合均经真实 CLI Run 写盘并按严格 TOML 解析，验证自定义 ID、正确平台与上报模式、默认关闭回放及 IIS 压缩权限；无效选项不创建文件。
- 单站点 render、缺失源 TOML 时的 status、多站点配置及受管目录歧义拒绝，拒绝后文件、历史、事务不被修改。POSIX shell 实际执行含空格、引号、美元符号及命令替换字面量的参数，内容保持一致。
- OrbStack Nginx 1.9.4 / Go 1.26.8 Linux arm64 完整 smoke；夹具由真实 init 生成精简 DataKit 配置再填写站点值，后续所有命令省略 --site。覆盖首次挂接、压缩、缓存、继承、启停、升级回滚、提交失败、漂移及 1.9.4 风险阻断。
- Go 1.20.14 Linux amd64 legacy CLI 在 OrbStack 仿真下执行 Nginx 1.28.3 同一精简接入/恢复流程。

验收脚本在容器命名空间内原子替换配置，避免 macOS bind mount 观察到宿主截断写入中的半份 TOML；工具对这类不完整配置继续拒绝操作，不自动忽略解析错误。日志：`/tmp/rumable-onboarding-go-final.log`、`/tmp/rumable-onboarding-final-nginx194.log`、`/tmp/rumable-onboarding-final-legacy.log`。

本轮为 CLI、模板及挂接提示变更，未重复 Chrome、Windows/IIS 实机或真实观测云入库验收。SDK、注入过滤算法、事务格式与服务器最低版本保持原有约束；之前的浏览器与 Linux 审查证据见前文，不能将模板验证算作 Windows/IIS 运行通过。

## 2026-10-20 自动挂接复审、SDK 同步与 CDN 加载

本轮直接修复并补充回归：

- FILE:LINE 对应同一行内多个 server 块时，显式 target 不再选第一个；全局扫描本站 ID 标记、保留路径与 Nginx 变量，阻止新状态目录复用其他部署的名称，同时允许不同 ID 共享名称前缀。
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
| 人工挂接回归 | Nginx 1.9.4 的原完整 smoke 通过，覆盖压缩、缓存、继承、HEAD、启停、升级回滚与失败恢复 |
| CDN | Nginx 1.28.3 真实官方 CDN：同域→CDN→同域回滚、HTTP 双编码与状态 URL 检查通过；本机 Chrome CDN 初始化、默认回放关闭、开启回放、新会话、停用及回滚通过 |

2026-10-20 实际同步核对：官方 CDN 和 NPM latest 均为 3.3.21，SHA-256 为 67c7d58349e376864f3297e9e6d4a60453ede6ff0c6f6b1900a74c2a44d4d3b1，与内置文件一致，未虚构版本升级。官方地址当前未返回 Access-Control-Allow-Origin，因此默认 CDN 不启用 SRI；自建 CDN 的可选 SRI 属性经真实生成脚本 + Node VM 验证，HTTPS HTTP 边界测试覆盖有/无 CORS、同域无需 CORS、摘要错误、过期 gzip 和历史摘要。未对自建 SRI CDN 做浏览器实网验收。

Chrome 页面只出现一个同域 init 和一个 https://static.guance.com/browser-sdk/v3/dataflux-rum.js 标签。默认 initialized、recording=false；TOML 开启后新会话显示 replayStatus=requested、recording=true、回放采样 100。Fetch 正常，Console 仅有主动触发的测试错误。停用后只剩业务脚本；回滚后使用不命中旧 HTML 缓存的新 URL，恢复 CDN 与录制。最后本地接收端计数为 View 12、Resource 22、Action 7、Error 1、Long Task 1，5 个回放片段、3696 个片段字节、17 次请求；这些只证明本地发送，不证明观测云入库。

证据：/tmp/rumable-cdn-final-go.log、/tmp/rumable-cdn-final-nginx194.log、/tmp/rumable-cdn-final-legacy.log、/tmp/rumable-cdn-final-nginx128-cdn.log、/tmp/rumable-cdn-manual-nginx.log、/tmp/rumable-cdn-chrome.png、/tmp/rumable-cdn-replay-chrome.png、/tmp/rumable-cdn-rollback-chrome.png、/tmp/rumable-cdn-chrome-stats.json。验证标签页、容器和网络已清理。

可执行复验：

```sh
go test ./...
go test scripts/sync-sdk.go scripts/sync-sdk_test.go
go run scripts/sync-sdk.go
NGINX_IMAGE=rumable-nginx:1.9.4 scripts/smoke-attach.sh
CDN=1 NGINX_IMAGE=nginx:1.28-alpine scripts/smoke-attach.sh
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
- 实际候选二进制执行完整自动挂接 smoke：Linux 386 / Go 1.26.8 对 Nginx 1.9.4、Linux arm64 / Go 1.26.8 对 Nginx 1.28.3（含 CDN 切换）均通过。覆盖确认、压缩、缓存、继承、隔离、启停回滚、故障恢复和完整撤销。
- Windows x86 选取 Sysnative 的路径与不具备 64 位管理宿主时的拒绝，经 Go 边界测试验证。Windows PowerShell 资产解析与可移植脚本边界在 OrbStack PowerShell 7.4 / Linux amd64 执行通过（设置 TEMP=/tmp，运行 legacy Linux CLI）；不据此认定 Windows PowerShell 3/5.1 或 IIS API 实测通过。

最终用户入口为 USER-GUIDE.md；每包自带平台 README、TOML 示例、兼容性、操作/恢复说明、版本说明、SDK 元数据及第三方许可。发布脚本将用户版本写入实际可执行文件，build.json 记录源码提交、编译器、目标和二进制摘要；下载目录包含索引与 SHA256SUMS。最终分发包需与这些已运行候选二进制逐字节对照，并核对归档文档对应提交的源码。

证据日志：/tmp/rumable-preview-candidate-build.log、/tmp/rumable-preview-go-preflight.log、/tmp/rumable-preview-x86-tests.log、/tmp/rumable-preview-race.log、/tmp/rumable-preview-nginx-x86.log、/tmp/rumable-preview-nginx-arm64-cdn.log、/tmp/rumable-preview-powershell-portable.log。Windows 所有架构、IIS 7.5/10、旧 Windows / Linux 内核及真实 OpenWay/DataKit 入库继续列为未验收；Windows ARM64 包尤其不能视为 ARM64 IIS 能力保证。

## 2026-10-20 · 服务端 OTel 源码验收

本节针对本轮源码，既有 `0.1.0-preview.1` dist 未重新生成。RUM、服务端 OTel 与 CLI 系统兼容范围分别记录，不能用编译或 HTTP 探测替代真实入库。

| 验证对象 | 实际执行与结果 |
|---|---|
| Go | OrbStack Linux arm64，Go 1.20.14 / 1.26.9 全量 test、vet 和发布归档契约测试通过；1.26.9 race 通过 |
| Windows 构建 | Go 1.26.9 交叉编译 Windows 386/amd64/arm64，通过；不是 Windows 运行验证 |
| Nginx OTel | Linux arm64：Nginx 1.25.3 + 官方模块 0.1.0、Nginx 1.28.3 + 官方模块 0.1.2，真实 Trace 链路全流程通过 |
| Collector | 实际运行官方 Collector 0.163.1，原生模块发送 OTLP/gRPC，Collector 转成 OTLP/HTTP protobuf 至私有测试接收器 |
| 原有 RUM | Nginx 1.9.4 自动挂接与完整 RUM smoke 回归通过，覆盖压缩、缓存、继承、HEAD、启停、升级及恢复 |
| Chrome | 本机 Chrome / Nginx 1.28.3：同时启用时一个 init、一个 SDK 标签，SDK 状态 initialized、Fetch 返回正常 JSON；停用 RUM 后无脚本、SDK=false、交互正常，Console 无错误/警告 |
| IIS 脚本 | OrbStack PowerShell 7.4 / Linux amd64：桥接与实机验收脚本解析、可移植快照和 OTel mock 契约测试通过；未运行 Windows/IIS/.NET Agent |

真实原生 Trace 验证包含：已知 W3C Trace ID、上游上下文、service.name 实际进入 protobuf、DataKit 配置的 gRPC 分支、Dataway 配置的 `/otel/v1/traces` 与 X-Token、RUM/OTel 独立启停、重复启用、0/100 采样、API 路径、排除路径和未受管 VirtualHost 不采集；第二个受管站点复用 exporter，在拥有者停用采集后仍上报，依赖关系阻止提前移除拥有者入口。重载失败恢复原状态和历史，按逆序撤销两站点后，业务配置逐字节等于原文件。

另外在 Nginx 1.25.3 执行纯 OTel 首次挂接：保留输出 gzip、原强 ETag、继承的 Host/Authorization，无 HTML 注入，JSON 页面可探测，API Trace 到达私有接收器，撤销后恢复原文件。原生装载失败的真实候选配置也已验证自动恢复；旧模块使用 HTTPS 在写入/创建挂接备份前被拒绝。

回归中修复的具体边界：旧版 ngx_otel_module 使用不带 http:// 的 gRPC host:port，允许 origin 末尾 `/` 时规范化；旧模块不伪装支持 TLS。探测重新建立连接，避免被 keep-alive 固定到旧 worker，并检查 OTel 配置代次。依赖站点正常提交的启停配置不会被误判成外部修改；未提交的编辑仍阻止恢复。

IIS mock 覆盖协议、Token 转义、采样百分比、x86/x64/runtime 切换、业务环境变量保留、回滚池名边界；快照测试覆盖提交前池归属/元数据/环境漂移与未变化压缩的零写入。Go 测试另覆盖 Dataway Token 轮换触发部署、单组件操作不应用另一组件尚未部署的 TOML 参数、共享 exporter 冲突、状态/私有资源校验及外部修改恢复保护。

本轮可追溯构建材料：Nginx 1.25.3 模块 `nginx-module-otel_1.25.3+0.1.0-1~bookworm_arm64.deb`，SHA-256 `7a39abec8ac3b0f3f00f03d26c2264915bb9c48ce1e1a638f41a8407dd8bcf74`。

主要证据：`/tmp/rumable-otel-nginx-final.log`、`/tmp/rumable-otel-nginx125-final.log`、`/tmp/rumable-otel-builder.log`、`/tmp/rumable-otel-nginx-native-failure.log`、`/tmp/rumable-otel-nginx-only.log`、`/tmp/rumable-otel-nginx-origin-slash.log`、`/tmp/rumable-otel-go120.log`、`/tmp/rumable-otel-go126.log`、`/tmp/rumable-otel-powershell.log`、`/tmp/rumable-otel-legacy-nginx.log`。临时日志仅在本机保留，不随分发包提供。

复验从源码目录运行：

```sh
go test ./...
go vet ./...
# 先准备匹配原生模块的镜像和 Linux arm64 Collector；本脚本针对本机 ARM64 OrbStack。
OTELCOL_BINARY=/path/to/linux-arm64/otelcol NGINX_IMAGE=rumable-nginx-otel:1.25.3 bash scripts/smoke-otel.sh
```

模块与 Collector 安装、IIS 管理员执行步骤见 [OTel 使用手册](otel.md)。Nginx 镜像分别预装对应官方 deb。镜像是本轮本机验证环境，不是已发布的可下载镜像。

未完成：Windows/IIS 10 原生 API、实际池回收、.NET Framework/ASP.NET Core Agent、Windows 7/Server 2008 R2 等旧系统；原生 TLS 握手/证书验收；真实 DataKit/Dataway 服务与观测云入库。Dataway 验证使用模拟空间 Token 和私有协议接收器，不能称为真实账号接入。IIS OTel 仅支持独占 .NET 应用池，静态页/Classic ASP/纯代理没有 .NET Span；旧 IIS 继续按 RUM 验收，不承诺 OTel。

## 2026-10-20 · 0.1.0-preview.2 审查与候选包验收

本次重新生成包含 OTel 的 Windows / Linux preview；旧 `0.1.0-preview.1` 文件保留。发布矩阵仍为 10 包：两个系统各有 x86/x64/ARM64 现代包及 x86/x64 legacy 包。现代编译器为 Go 1.26.9，legacy 为 Go 1.20.14，CGO=0。

审查和运行验证发现并修复：

- 已撤销挂接的旧站点仍占用共享 exporter，阻止新配置；现在已撤销站点释放占用，未完成的挂接/恢复仍阻止新部署。回归覆盖三种状态。
- IIS 环境集合的序列化顺序变化可能触发无意义的池回收；现在按名称和值比较。PowerShell mock 验证仅调整顺序不回收，改变 service.name 才回收。
- 初版候选包在 Nginx 1.25.3 真实运行时复现停用返回后仍接收新请求并产生 Span。官方平滑重载在启动新 worker 后等待 100ms，随后才通知旧 worker 退出；现在要求相隔 200ms 的连续两轮配置/HTTP 探测成功。回归模拟新→旧→新→新代次，完整原生流程复跑通过。已有请求和排队 Span 仍不承诺即时终止。
- 发布构建现在拒绝覆盖已有输出目录，在写入最终校验清单前重新核对干净的源码提交；对已有候选目录执行构建确实拒绝，原 SHA256SUMS 不变。

最终候选 `preview2-rc3` 的实际可执行文件用于以下验证；最终分发构建从干净的本地 Git 提交生成，并逐包比较 binarySHA256，以确保交付程序与候选一致。

| 范围 | 实际执行结果 |
|---|---|
| Go | OrbStack Linux arm64：Go 1.20.14 / 1.26.9 全量 test、vet、SDK 同步及发布归档契约测试通过；1.26.9 race 通过 |
| SDK | 发布前在线同步官方稳定 v3，仍为 3.3.21，SHA-256 `67c7d58349e376864f3297e9e6d4a60453ede6ff0c6f6b1900a74c2a44d4d3b1` |
| Nginx OTel | 候选 Linux arm64 程序对 Nginx 1.25.3 / 官方模块 0.1.0、1.28.3 / 官方模块 0.1.2 完整流程通过；真实模块→Collector→私有 OTLP 接收器，包含 DataKit gRPC 和 Dataway HTTP/protobuf/Token 分支 |
| 最低 RUM 版本 | 候选 Linux x86 对 Nginx 1.9.4 的完整自动挂接回归通过，覆盖压缩、缓存、继承、确认、启停回滚、并发修改拒绝及备份保留 |
| 五个 Linux 包 | x86/x64/ARM64 现代包和 x86/x64 legacy 包在 OrbStack 实际执行 version、完整 RUM init、随包 OTel 示例 render/status，均通过；x86/x64 使用仿真，共享现代内核 |
| Windows 与归档 | 交叉编译全部五个 Windows 包；PE/ELF 机器类型、编译器、preview 标记、二进制 SHA-256、ZIP CRC、Linux 可执行模式、包内文档链接和 OTel 示例检查通过 |
| PowerShell | OrbStack PowerShell 7.4/Linux：IIS 快照与 OTel mock 通过；RUM/OTel 桥接和 Windows OTel 验收脚本解析通过。未执行 Windows/IIS API |

本轮不改浏览器 SDK 或初始化行为，Chrome 的实际证据沿用上一节 OTel 源码验收记录，没有将其计为本轮重新执行。各包携带最终用户指南、OTel 示例、原生模块安装/构建和 Dataway 配置步骤、Windows 实机脚本；原生模块、.NET Agent、Collector 需另行安装，不包含在 CLI 中。

主要日志：`/tmp/rumable-preview2-rc3-build.log`、`/tmp/rumable-preview2-race.log`、`/tmp/rumable-preview2-otel-nginx125.log`、`/tmp/rumable-preview2-otel-nginx128.log`、`/tmp/rumable-preview2-rum-nginx194.log`、`/tmp/rumable-preview2-linux-matrix.log`、`/tmp/rumable-preview2-review-ps.log`、`/tmp/rumable-preview2-existing-output.log`。这些是本机日志，不随包分发。Windows/IIS 实机、旧内核、原生 TLS、真实 DataKit/Dataway 与观测云入库仍未验收；私有协议接收器成功不能替代真实入库。

## 2026-10-20 · Windows/IIS OTel 加固复审（未发布源码）

本轮针对 preview.2 之后的源码，未覆盖原发布包或改变 v0.1.0-preview.2 标签。当前主机为 macOS，OrbStack 仅有 Linux 环境，没有 Windows/IIS 测试机；下面区分真实 Go 事务运行、PowerShell mock、交叉编译和仍需实机执行的项目。

修复与保护的具体行为：

| 问题 | 修复及回归 |
|---|---|
| IIS Recycle 返回值污染桥接 JSON | [官方 API](https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.administration.applicationpool.recycle)返回 ObjectState，原 mock 的 void 签名漏检。生产代码显式丢弃返回值，mock 改为返回状态，验证回收有动作时仍无多余输出；未将此合同检查算作 IIS 实机运行 |
| 单独操作 RUM 会读取当前 Dataway Token | RUM 命令保留已部署 OTel 环境与凭据摘要，不再重新检查未变化的 Agent/接收器。Go 测试覆盖 Token 缺失及轮换且接收端不可达时的幂等 RUM 操作；PS mock 验证实际环境保留，不触发回收 |
| 回收失败先于事务记录，恢复后的进程状态无法确认 | IIS apply/restore 只提交配置；Go 先持久化已提交快照，再以独立 activate 调用校验归属并回收。真实 Go 日志/状态代码覆盖回收失败、恢复回收再次失败、已恢复配置的后续重试，以及重试期间外部池修改拒绝 |
| 旧 worker 在 overlapped recycle 后继续使用旧 exporter | 回收前记录本池 worker，最多等待其退出 15 秒；不强杀进程。超时或池未 Started 时失败并保留恢复日志。PS mock 使用真实 Linux 子进程及 .NET Process API 验证等待，不将它算成 IIS 回收实测 |
| 外部 OTel 设置绕过站点参数或停用 | 增加 WAS/W3SVC 服务环境检查、所有 OTEL_* 和通用 profiler path 的漂移捕获；ASP.NET Core 检查有效 aspNetCore/environmentVariables，拒绝应用级覆盖。PS mock 覆盖共享池、服务/应用覆盖、冲突时零写入及未知信号级 exporter |
| Agent/池条件与回滚凭据混在一起 | 普通回滚重新检查目标 Agent/池条件，同时使用已保存 Token。ASP.NET Core 要求 No Managed Code；Windows ARM64 经 x64 仿真宿主也拒绝。mock 覆盖这些前置条件、x86/x64 profiler 参数和保存凭据恢复 |
| 验收脚本无法按已有清理契约删除临时资源 | 资源名改为既有受限清理格式；新增独立 ResultDir，清理失败影响最终结果，日志不随临时站点删除。两个测试池用于隔离/PID 断言；WithRUM 覆盖独立启停与缺失 Token；失败恢复检查实际应用环境代次，重复启停检查 PID/历史 |

实际执行：

- OrbStack Linux arm64：Go 1.20.14 / 1.26.9 全量 test、vet 通过；Go 1.26.9 race 和发布归档契约测试通过。新增 IIS 边界测试仅替换 Windows API 调用，执行真实事务落盘、历史、回收调用顺序、失败恢复与普通启用/停用快照回滚；另含提交后 HTTP 503 的恢复。
- Microsoft PowerShell 7.4 / Linux amd64：受影响 PS 脚本解析、IIS 快照及 OTel 契约 mock 通过。快照回归覆盖池名、身份、用户、位数、CLR、管线、归属应用和环境漂移；重复环境顺序变化不写入/不回收。
- Go 1.26.9 交叉编译 Windows x86/x64/ARM64，Go 1.20.14 编译 Windows x86/x64；五个 PE 的 Machine、Go 编译器、GOOS/GOARCH 和 CGO=0 检查通过，未执行 Windows 程序。
- 共用部署代码的 Linux 运行回归：Nginx 1.28.3 / 模块 0.1.2（arm64），完整原生 OTel→Collector→私有接收器流程通过，包括独立启停、故障恢复和撤销。

本机日志：`/tmp/rumable-iis-otel-hardening-go120.log`、`/tmp/rumable-iis-otel-hardening-go126.log`、`/tmp/rumable-iis-otel-hardening-race.log`、`/tmp/rumable-iis-otel-hardening-powershell.log`、`/tmp/rumable-iis-otel-hardening-build.log`、`/tmp/rumable-iis-otel-hardening-pe.log`、`/tmp/rumable-iis-otel-hardening-nginx.log`。

尚未执行：Windows PowerShell 5.1、Microsoft.Web.Administration 的实际提交/池回收、IIS WorkerProcesses 枚举、ACL/双站点清理、真实 .NET Framework/ASP.NET Core Agent、旧 Windows、TLS 和 DataKit/Dataway/观测云入库。增强后的 Windows 验收脚本只做解析检查，未把脚本中的待执行断言列为通过。源码变更未改 Web SDK/初始化行为，本轮未重跑 Chrome。

Windows 可直接执行的 DataKit、Dataway、x86 池和 WithRUM 命令、结果读取及保留资源清理见 [Windows/IIS OTel 专项步骤](windows-acceptance.md#7-iis-10-otel-专项验收)。配置提交与本地事务记录之间仍有不可原子化窗口；无法匹配日志时停止覆盖，保留现场人工审查。

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
