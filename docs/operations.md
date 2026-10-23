# 部署、缓存与恢复

本手册以下流程默认浏览器 RUM。服务端 OTel 的模块安装、自动加载、实例/应用池共享边界、Dataway Collector、组件启停与回滚限制见 [OTel 操作手册](otel.md)。纯 OTel 挂接不为 RUM 改写压缩、缓存或上游业务请求头。已有 OTel 表时，Linux attach 可按预览加载指定原生模块；可先执行 install-otel 自动准备并校验受管依赖；完整站点卸载使用 uninstall，详见[安装与卸载](otel-installation.md)。显式指定的外部依赖仍由运维维护。

SDK 来源和同步/异步的选择见[使用指南](../USER-GUIDE.md#4-sdk-注入加载与更新)，字段见[配置参考](configuration.md#sdk-加载方式)。切换来源或加载方式需重新 enable/setup；同步时探测会核对 SDK 在初始化脚本之前、无 async/defer 属性及正确的 SRI。两种脚本都会随停用移除，历史快照保留原加载方式。

## 首次挂接

1. 创建观测云 Web 应用，取得对应上报方式的参数。只使用浏览器 Client Token，不使用管理 API Key。
2. 明确目标站点、所有 HTML handler 与路径前缀；每个 handler 至少配置一个实际可访问的 `probePaths`。
3. 使用 `init --server nginx|iis --reporting openway|datakit` 生成精简 TOML，填写网站、服务器目标及上报值；高级参数需要时用新文件执行 init --full 或参考完整模板。`render` 到最终受管目录，终端直接列出可复制的 attach、enable 命令和人工挂接说明；该目录需在服务和工具使用的文件系统命名空间中保持同一路径。容器内应在容器内执行 CLI，或确保生成路径与挂载路径一致。
4. Linux Nginx 执行 `attach`，预览并输入 yes 确认；复杂配置按生成的 `ATTACH.md` 人工挂接 include。IIS 安装 URL Rewrite 2.x、准备管理员权限、静态资源读取 ACL、JavaScript MIME 和匿名访问。
5. 执行 `enable`，自动完成预检、配置检查、生效和 HTTP 探测；独立 check 是可选的提前诊断步骤。自动 attach 完成后可直接启用；人工挂接须检查、加载并设置 attachmentReviewed=true。失败时按诊断解决压缩、CSP、缓存及现有规则冲突。
6. 在 Chrome 验证 SDK 两个资源正常加载、初始化无报错，检查上报请求的 HTTPS/CORS、状态和响应，再在观测云确认入库。

`includePaths` 按 URL 路径前缀工作，不是文件扩展名规则。IIS 规则区分大小写，与 Nginx 保持一致。`/__rumable__/` 是应用保留路径，不允许和业务路由共用。

## 自动首次挂接（Linux）

```sh
rumable render
rumable attach --dry-run         # 只读识别和差异预览，不写业务配置或备份
rumable attach                   # 展示当前差异，输入 yes 后执行
rumable enable
# 重复 attach 验证已有挂接，不重复插入或重载。
# 多候选、通配名称、端口映射、默认站点或主机名不匹配时，明确选择输出中的真实位置：
rumable attach --target /etc/nginx/sites-available/example:12
# 自动化流水线显式确认：attach --yes；仍打印差异，并在提交前重新识别配置。
```

使用与服务一致的 binary、args、环境和权限，在 Linux 的实际配置命名空间执行。工具不安装软件、启用模块或猜测服务进程参数；从配置的 binary/args 调用 Nginx `-V/-T`，按 publicURL 的准确主机名和端口选择唯一 server。歧义时列出 FILE:LINE，要求显式 --target；这只选择候选，不能绕过安全检查。HTTPS 默认 443，HTTP 默认 80。多 IP 上同名同端口的站点也要求选择。变更后原行号可能变化，下次发现以新列表为准。

Nginx 在真实 http、目标 server 和现有 handler 中加入独立标记的 include，不创建竞争的业务 location。复制有效的父 proxy_set_header 列表时保留 Host、Authorization、自定义头和变量；已有局部列表保持其原来的继承边界。目标代理 handler 清除上游 Accept-Encoding、If-None-Match、If-Modified-Since 并设置 proxy_cache off；安装 gzip_static/brotli 模块时关闭目标 handler 的预压缩直出。这些首次挂接调整在普通 disable 后仍保留，完整撤销挂接才恢复原值，差异会明确展示。输出 gzip 可以继续使用。includePaths 控制注入，挂接调整作用于整个 handler，包括其中非 HTML/排除路径；需要较窄范围时使用独立业务 location 或人工挂接。

自动合并支持常见静态和 proxy_pass 配置、嵌套 location 及简单 include。已有 sub_filter/局部验证头指令、if/rewrite、预压缩 try_files、FastCGI/uWSGI/SCGI/gRPC、SSI/附加正文等需要人工审查，报告文件和行号。Nginx 1.9.4 的 http 层 sub_filter 在写文件前即阻止，不能由 --yes 或 attachmentReviewed 放行。

同一 FILE:LINE 对应多个块时拒绝显式选择，需先将各块分行。首次挂接扫描整个有效配置中的本站 ID 标记、保留资源路径和 Nginx 变量名称；发现其他目录或站点已占用相同名称时拒绝，不能用新的 state-dir 接管已有部署。

配置中的符号链接解析到真实文件，保留链接；被多处引用的待修改 include 拒绝自动修改，以免影响其他站点。先展示逐项增删差异和备份位置；输入 yes（或显式 --yes）后重新获取有效配置并核对全部读取文件的摘要，变化则要求重新预览。随后将完整原字节、文件模式及差异以 0600 保存到 `sites/<id>/attachment.json`，再原子替换业务配置，保留属主、组、模式和可读取的扩展属性（包括 ACL/SELinux 标签），拒绝直接替换符号链接及硬链接文件。使用同一个 state-dir 串行维护站点；外部配置工具也应在此期间停止修改配置。

确认后还会核对部署状态、备份、符号链接指向及 Linux 文件身份/元数据；每次写入和重载前复查未受管输入。历史挂接备份采用排他创建，已有不同内容时停止覆盖。撤销及失败恢复重新识别有效 include 图，待恢复文件变为共享或无法确定归属时停止；未受管输入出现变化也停止并保留恢复记录。旧备份没有新增输入指纹时仍执行原字节/模式检查与已完成挂接的有效图检查。任意外部进程不遵守维护锁时不存在跨进程原子事务保证，不能在确认后继续并行发布服务器配置。

候选须通过服务器配置检查，再做平滑重载并探测原页面、初始化脚本和所选 SDK 的 identity 和 gzip 版本。初始注入保持关闭，成功后才写入 attached 状态；enable 继续独立检查 CSP、注入和上报条件。配置检查、重载、HTTP/资源失败时恢复本次涉及文件并重新检查、重载、探测。中断或恢复失败保留 applying/restoring 记录，阻止普通部署操作，使用 `rollback --attachment` 重试。多文件只恢复一部分时，磁盘上的临时配置可能无法通过 nginx -T；只要未受管输入未变且各受管文件仍匹配保存的 Before/After 字节，就能继续完成恢复；先按后文检查并移除确认已失效的进程锁。此命令与普通部署 rollback 相互独立。

```sh
rumable disable
# 清理仍引用 SDK 的浏览器、CDN、代理、Service Worker HTML 缓存。
rumable rollback --attachment --dry-run
rumable rollback --attachment   # 展示撤销差异，输入 yes 后恢复首次挂接前的文件
```

完整撤销会移除同域资源映射，因此必须先 disable 并处理旧缓存页面。业务配置若与备份中的受管后版本不一致（包括无关编辑或后来挂接的其他站点），停止覆盖并报告冲突，不会把旧备份覆盖到新业务配置。先审查差异，可按挂接顺序反向撤销，或人工移除指定标记及恢复已调整的指令。普通启停只验证自动标记并修改受管部署文件，不回写业务文件。状态显示 attachment 的 phase、target 和 backup。成功撤销后仍保留备份和版本化资源；再次 attach 将旧记录归档为 `attachment.<时间>.json`，不会丢失原备份。备份含原服务器配置，state-dir 必须位于网站根目录之外并由管理员控制。

## 压缩与缓存

Nginx 的过滤器处理源 HTML，然后可以由 Nginx 做输出 gzip。反代请求应清除 `Accept-Encoding`，同时保留原有 Host/认证/自定义请求头。清除上游条件请求头，防止上游 304 绕过新版本注入。静态站点不能直接选取 `.gz`/`.br` 文件；只有安装相应模块时才能写对应 `gzip_static off` / `brotli_static off` 指令。

Nginx 注入同时检查实际 HTML MIME 与源响应的 Content-Encoding；父层 `sub_filter_types *` 不会让 Rumable 改写 JSON 或 JavaScript。需要保留的业务 sub_filter 必须与 location include 显式放在同一层；停用时保留一个动态空匹配规则，避免启停改变规则继承；普通停用、`disable --component rum` 和修改 TOML 关闭 RUM 都遵守这一约束，缓存页面仍可获取字节未变的旧脚本。纯 OTel 首次挂接仍不添加 RUM 过滤器，也不额外要求 http_sub_module。业务规则自己的改写范围仍由原配置决定。

`check` 在 identity 页面上预检源压缩和 CSP。提交后的探测及独立 `probe` 还请求 gzip 变体并解压核对注入与版本；`gzip_static on` 导致只有压缩页面未注入时，启用失败并恢复受管状态。`gzip_static always` 连 identity 页面也返回压缩内容，会在预检时直接拒绝。探测解压后的正文仍受 8 MiB 限制。Brotli 预压缩须在注入路径上关闭。

多页面 `probe` 和提交/恢复探测逐页检查 HTML 的 identity、gzip、注入版本及 CSP；同一轮内相同初始化脚本、SDK 和 Worker 的每种编码只检查一次。初始化脚本和 SDK 的两种响应在解压后都必须匹配受管 SHA-256；外置 Worker 检查 MIME，并要求 identity / gzip 解压后的内容一致，不因此证明 Worker 代码属于指定 SDK 版本。共用资源的检查结果不跨轮复用，重试仍会重新验证。这样增加 probePaths 不会重复下载同一编码的 SDK，也不会漏检后续页面的压缩或 CSP 差异。过期的脚本 .gz 文件、错误压缩缓存或仅 gzip 生效的业务过滤器也会使启用/升级失败；工具保留这些非受管文件，由运维审查并修复。

Nginx 1.9.4 的 `http` 层 sub_filter 继承可能在 SDK 请求或提前返回的错误响应中导致 worker 崩溃。`check` / `enable` 自动根据 nginx -T 的有效配置和 include 层级检查，发现风险时报告源文件及行号并拒绝启用，attachmentReviewed=true 无法放行。普通回滚到启用状态、候选配置 reload 前也会检查；停用保留原有路径。先将业务规则移到 `server` / `location`，或升级到 1.9.5+，再重新 check / enable。

检查只使用同一次 nginx -T 的转储，不把附属文件当作独立顶层配置；相对 include 按主配置目录解析，同一文件在 server 与 http 引用时会分别核对。未知/缺失的 include 上下文、异常转储或不支持的掩码会拒绝，支持范围和处理方法见[兼容性说明](compatibility.md)。check 会保存失败诊断；启用前拒绝不会创建事务、改写受管部署或 reload。中断恢复若在写回候选后仍遇到风险，会保留事务日志；先解决外部业务配置，再重试 rollback，不要删除日志绕过。

IIS 出站 HTML 重写要求输入正文未压缩。所选应用的 IIS 静态和动态压缩都需要关闭，避免压缩响应绕过注入。

IIS RUM 要求所选应用使用独占应用池，子应用也应分配自己的池。配置变更会回收该池并等待旧工作进程退出，再进行 HTTP 验收；进程内会话可能丢失，请在可接受回收影响的窗口操作。工具不自动重新分配应用池，不回收共享池，不执行 iisreset。应用池归属一经记录，外部修改池名或加入其他应用会使后续变更拒绝执行。

工具在允许后关闭所选应用的 `doStaticCompression` 和 `doDynamicCompression`，分别保存原局部值及属性是否存在；停用/卸载时恢复值或删除新增属性，恢复原来的继承关系。影响范围是所选 IIS 应用的压缩，其他站点及后端中间件的压缩设置不变。文件级覆盖和上游压缩仍可能导致探测失败。

旧快照如果未管理动态压缩，CLI 在首次变更前读取它的实际局部值，并先写入事务日志；失败恢复还原这份实际基线。恢复旧事务不会把缺失字段当作 false，也不会擅自删除动态压缩属性。主动回滚到历史启用版本时恢复旧 SDK/配置，同时保持动态压缩关闭；最终停用恢复首次接管时的原值。

IIS 预检查使用浏览器压缩请求。提交配置、持久化事务快照并完成应用池回收后，先检查携带 If-None-Match、If-Modified-Since 及两者组合的普通刷新，再检查无条件浏览器请求、gzip 和 identity；304 或正文漏注入立即触发恢复，后续成功不能覆盖首次失败。ETag 检查使用 `*` 匹配任意旧表示，日期检查使用未来时间。一次 identity 请求可能预热 IIS 缓存并掩盖后续 gzip 失败，因此手动测试也应先检查浏览器首次刷新。

启用时，所选应用增加一条入站规则：对 includePaths 内、未排除且 Accept 包含 text/html 的 GET，清空请求中的 `If-None-Match`、`If-Modified-Since`，防止源站 304 让浏览器继续使用安装前或错误压缩的旧正文。规则在本应用的业务入站规则前执行，不改 URL、不停止后续规则；SDK 资源、排除路径、其他方法及不接受 HTML 的 API 请求不受影响。入站时尚不知道响应 MIME，所以同路径下以 HTML 导航方式访问的 JSON/下载也会失去条件 GET 优化，正文仍由出站 MIME/下载条件旁路。需要保留其条件缓存时，请加入 excludePaths。

工具只在目标应用的 allowedServerVariables 中补充这两个请求头的权限，并记录原有局部项和继承；停用/卸载删除本次新增权限，保留原有权限。旧快照首次变更时记录实际基线；历史启用版本回滚保留新防护，失败事务恢复精确回到变更前。配置锁定、局部 remove 冲突、受管规则或权限漂移都会报错。父级或业务规则可能覆盖请求头；实际 HTTP 条件探测失败时不会判为接入成功。

HTTP 验收仅覆盖 `probePaths`，默认自动发现通常只有首页；安装结果列出实际检查的 URL。此防护针对到达 IIS 的条件 GET，不能清除完全不发网络请求的浏览器新鲜缓存、Service Worker 或外层 CDN 缓存。接入后，在安装前已打开的浏览器页面执行普通刷新，并检查首页和其他主要业务页面：应返回 200、包含恰好一条当前初始化脚本，且业务内容完整。失败时先保留现场、请求响应及缓存证据，定位后再按相应层的策略处理缓存。

IIS 失败探测保存到私有站点状态目录的 `last-operation-probe-failure.json`；恢复探测失败另存 `last-recovery-probe-failure.json`。文件保留最近一次对应阶段的失败，即使自动回滚成功也不会删除。日志含请求时间、HTTP 状态、缓存相关响应头，以及 200 错误正文的解压后 SHA-256、预期/实际受管初始化脚本路径，不保存正文、Cookie 或任意第三方脚本 URL。旧文件不代表本次操作失败，应核对时间。

Nginx 活跃 HTML handler 关闭本地 ETag/条件日期处理；符合注入条件的 GET 与 HEAD 都激活过滤器的头部清理，移除原文 Content-Length、ETag、Last-Modified，HEAD 不输出正文。方法匹配使用正则 map 项，兼容旧版默认 map_hash_bucket_size，无需修改全局桶大小。不新增 proxy_hide_header，因此保留原有隐藏响应头列表。停用后恢复原验证头行为，但业务自己的过滤规则仍可能移除这些头。IIS 对选定 HTML 清除这两个验证头。HTTP 探测拒绝仍携带原文强 ETag 的改写页面。范围和继承的影响须在首次挂接时审查。Nginx 历史快照按原字节回滚，需要新版本头部修复时重新 enable；IIS 启用快照按上述压缩及条件请求防护迁移。

同域哈希资源通过 `expires 1y` 提供一年缓存，不在资源 location 新增 add_header，保留旧 Nginx 的父层安全/CORS 响应头继承。资源路径不执行继承的业务 sub_filter，避免改变 SDK 字节。

启用、停用、升级和回滚都需要考虑已缓存的 HTML：源站配置变化无法强制清除浏览器、CDN、反向代理或 Service Worker 中的页面。按站点缓存策略执行清理，并用真实浏览器复查。脚本使用内容哈希，旧版本文件始终保留，防止旧 HTML 引用 404。

## CSP 与上报

- 同域初始化脚本和所选 SDK 地址都需要被 `script-src-elem` / `script-src` / `default-src` 的有效策略允许；CDN 模式需额外放行实际 CDN URL。
- 预检分别检查真实哈希初始化脚本和 SDK URL。CSP 路径以 `/` 结尾才作为目录前缀，否则按完整路径匹配；允许一个脚本不代表另一个也被允许。路径按分段解码比较，见 [CSP 路径匹配规范](https://www.w3.org/TR/CSP3/#match-paths)。
- `connect-src` 需允许 OpenWay 或 DataKit 的准确 HTTPS origin；限制路径时须包含 `/v1/write/rum`、启用回放时的 `/v1/write/rum/replay`、启用远程配置时的 `/v1/env_variable`。多 CSP 策略取交集，增加一个宽松头不会覆盖原有严格策略。
- 跨域 HTML `<base href>` 会改变根路径脚本和 Worker 的加载域名，预检会阻止启用；同域 base 可用。工具不改写业务的 base 标签。
- 工具采用保守的常见 CSP 匹配，不能证明任意复杂策略或运行时 CSP 的兼容；严格 nonce、strict-dynamic、Trusted Types 不自动绕过。
- 浏览器直连上报：内部 DataKit 地址对服务器可达，并不代表对访问者浏览器可达。跨域预检与实际 POST 均需由接收端正确允许；HTTP 探测不替代浏览器验证。
- Session Replay 按站点的 `sessionReplayEnabled`、`sessionReplaySampleRate` 和 `sessionReplayOnErrorSampleRate` 开启，默认关闭；总开关关闭时本地普通及错误回放采样均为 0。
- 回放、RUM 压缩、Canvas 及远程配置的 Worker 预检按 `worker-src` → `child-src` → `script-src` → `default-src` 取有效策略。内嵌 Worker 保守要求显式 `blob:`；`workerUrl` / `replayCanvasWorkerUrl` 可以改为同域托管，工具检查 URL、200 和 JS MIME，不验证第三方 Worker 代码逻辑，不自动修改 CSP。
- 默认 `mask` 遮罩；其他隐私级别、错误补偿、Cookie/持久化、Trace/Visitor ID、Canvas 2D 等可配置，见[配置参考](configuration.md)。函数、插件、动态用户和业务上下文仍需业务代码。启用手动 View/Canvas 模式后，必须由业务调用对应 API；远程配置可能覆盖静态 SDK 参数。
- `window.__RUMABLE__.replayStatus="requested"` 只表示调用录制 API，不能证明会话命中采样或上传成功；在 Chrome 检查 `DATAFLUX_RUM.isRecording()`、上传和观测云。新设置对新加载页面生效，采样变更应用于新会话；停用及回滚不终止已经打开页面的录制。
- 只有基础自动采集；业务登录用户 ID、自定义上下文和业务事件需要业务自行接入，不会从页面或 Cookie 猜测。

## 事务与漂移保护

变更持有受管目录的 `operation.lock`。文件逐个通过同目录临时文件替换，操作前保存 `transaction.json`；多文件更新由日志和恢复流程保护，并非一次原子目录切换。

成功后只保留当前和上一次成功快照。`rollback` 在两者之间切换；重复回滚会切回另一个快照。失败的变更不会占用成功历史。资源文件和已创建的同域资源映射保留，确保旧缓存页面仍能加载同域 SDK。CDN 快照保留实际 URL 和摘要，但外部内容由 CDN 管理，工具不能恢复被覆盖/删除的远程文件；固定版本和完整离线恢复优先使用默认 bundled，或不可变 CDN URL 加 SRI。详见[SDK 加载方式](configuration.md#sdk-加载方式)。

Nginx 的 attachmentReviewed 只限制启用检查；首次 render 保存的 false 不阻止初始停用状态或回滚到该状态后的重复 disable。停用仍须通过有效 include、服务器配置和受管文件检查，不能用此绕过配置错误或漂移保护。

工具在启停、回滚前检查受管文件与资源摘要。IIS 还比对所拥有规则、虚拟目录、局部压缩属性及 OTel 池名、身份、位数、CLR、管线、应用归属和插桩环境（含未受管 OTEL 信号覆盖项）。发现外部修改时拒绝覆盖；只修改业务无关配置不会被恢复流程回滚。

中断事务恢复在写配置前检查原快照资源摘要，并确认日志内前后快照属于当前受管站点的相同目标。快照中的初始化脚本及 SDK 引用必须与资源摘要一致。IIS XML 快照按其站点/应用路径不区分大小写的语义读取，保留原压缩属性和资源映射。

新配置必须满足当前发布包的 SDK 参数校验。历史快照恢复原资源字节，因此只校验部署目标、路径及摘要，不将新的 SDK 参数限制套用到历史快照，避免升级后无法停用或恢复旧部署。修正参数后仍可使用原站点 ID 启用新配置。

进程被强制终止时：

1. 确认没有 Rumable 进程运行，备份受管目录及现场服务器配置。
2. 确认锁为该退出进程遗留后，手动移除 `operation.lock`。
3. 使用同一 `--state-dir` 与 `--site` 执行 `rollback` 恢复部署事务；首次挂接的 applying/restoring 状态使用 `rollback --attachment`。
4. 出现外部文件变更、IIS 提交后但日志未记录的中断窗口，或恢复后的 HTTP 探测仍失败时，工具保留日志并停止。审查现场差异，不要直接删除日志或覆盖生产配置。

IIS 没有跨配置提交与本地日志的分布式事务；遇到无法确认归属的状态，采用停止覆盖的保守恢复策略。备份中若存在旧 SDK 版本，使用对应发布包恢复，且保留版本化资源。

IIS RUM 和 OTel 均先提交配置、持久化快照，再以独立调用校验并回收目标独占池。RUM 的启用、更新、停用、历史回滚和事务恢复都包含生效步骤；同次 RUM/OTel 变更只回收一次。旧版本首次接管时记录池归属并执行一次激活，之后完全相同的重复启停仅验证 HTTP，不反复回收。变更需等待旧 worker 退出（最多 15 秒）；回收/HTTP 失败时恢复原配置并再次回收，等待旧进程退出。恢复尚未完成时保留事务，后续 rollback 会重试进程恢复，不以“配置值已经相同”认定成功。池未 Started、长请求阻止旧 worker 退出或存在外部修改时应先解决对应条件，不强制结束业务进程。回收不会替代 HTTP 校验，也不证明所有缓存层已失效。详细 OTel 条件和发布包范围见 [OTel 手册](otel.md)。

部署前可在独立 Windows VM 按 [Windows/IIS 验收手册](windows-acceptance.md)运行完整流程。脚本创建自己的站点、应用池及父子规则，模拟提交后的 HTTP 探测失败并验证恢复；不对整台 IIS 恢复配置备份。Keep 现场使用配套 windows-cleanup.ps1 按报告归属清理。

## 常见结果

| 结果 | 含义与处理 |
|---|---|
| `missing effective Nginx include` | include 未加载或路径不一致，按 ATTACH.md 检查真实 handler |
| `no safe literal <head>` | 需要标准 `<head>` 开始标签（大小写不限，无属性或额外空白），且此前不能有同名文本。检查实际模板，勿改回可能误中 JS 字符串的 `</head>`；探测需覆盖全部受管模板 |
| `existing RUM integration found` | 业务已有 SDK，应保留现有接入或先明确迁移 |
| `encoded response` | 上游/静态源仍返回压缩内容，先解决源压缩 |
| `CSP ... blocks` | 修改站点的实际安全策略后重试；工具不会自动放宽 |
| `stale bootstrap URL` | 先保留失败输出与探测证据，核对目标池生效状态、请求命中的实例和实际 HTML 缓存，再处理已定位的缓存 |
| `external change` / `asset drift` | 受管项被其他操作修改，先审查现场差异 |
| `injection and script availability only` | HTML/文件检查通过，不等于浏览器执行或观测云入库成功 |
