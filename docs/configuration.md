# TOML 配置参考

浏览器 RUM 与服务端 OTel 分别使用 `[sites.rum]` / `[sites.otel]`。`rum.enabled` 省略时为 true，显式 false 时不要求浏览器凭据；省略 otel 表保持旧行为。服务端参数、采样实际能力、模块要求和三种服务器示例见 [OTel 配置手册](otel.md)。`enable/disable --component rum|otel` 保留另一组件的当前状态；`enable` 不带组件时按 TOML 应用两者，`disable` 不带组件时全部停用。

配置按 `[[sites]]` 隔离，SDK 参数放在该站点的 `[sites.rum]` 下。`rumable init` 默认生成精简配置，只展示接入和回放开关；`rumable init --full` 与[完整模板](../examples/full.toml)包含全部静态 SDK 参数。`--server nginx|apache|iis` 选择服务器，`--reporting openway|datakit` 选择上报方式，默认 nginx / openway。例如 `rumable init --server apache --reporting datakit`。所有组合默认关闭回放；IIS 初始配置不预先允许压缩调整。

init 默认站点 ID 为 example；可通过 `init --site website` 命名。单个配置站点的 render/check/enable 无需 --site；多个配置站点必须指定。attach/status/disable/rollback/probe 优先从受管状态选择：只有一个受管站点时无需原始 TOML，多个受管站点必须指定 --site。显式 --config、--state-dir、--site 的原用法保留。[Nginx 示例](../examples/nginx.toml)、[Apache 示例](../examples/apache.toml)和 [IIS 示例](../examples/iis.toml)也可直接编辑使用；注释字段为可选示例。

## 服务器目标

每个站点必须且只能配置一个 `[sites.nginx]`、`[sites.apache]` 或 `[sites.iis]`。`id`、`publicURL`、服务器类型及目标命令/参数绑定部署身份，不能原地迁移；迁移使用新 ID。SDK 参数和路径采集范围可修改后重新 enable。

| Apache 参数 | 类型 / 默认值 | 说明 |
|---|---|---|
| `binary` | 字符串，必填 | Linux `httpd` / `apache2ctl` / `apachectl` 的可执行路径；不会通过 shell 拼接执行 |
| `args` | 字符串数组，空 | 只允许成对 `-f 配置文件`、`-d ServerRoot`、`-D 自定义Define`；不允许注入配置、服务动作、进程或诊断模式 |
| `attachmentReviewed` | 布尔，false | 人工挂接时表示已按 ATTACH.md 审查；成功的自动 attach 记录可以替代 true，无需编辑 TOML。初始停用/恢复不要求为 true；不能绕过版本、模块、有效 include 和配置检查 |

Apache 运行操作限 Linux，macOS/Windows 可以预览生成文件；实际部署须保证生成路径与 Linux 命名空间一致，通常在 Linux 的最终固定目录重新 render，不能直接迁移路径不同的预览状态。资源和私有 Lua 过滤器使用内容哈希，并在启用/恢复前校验摘要。启用期间对 GET/HEAD 条件头、HTML 验证元数据、Range 和 mod_cache 的影响是固定正确性约束，详见 [Apache 手册](apache.md)。三个服务器共享以下全部 `[sites.rum]` 参数，包括 OpenWay / DataKit、回放、隐私与 Worker 设置。

范围以发布包内的 **SDK 3.3.21** 为准：已开放其公开类型定义中未废弃、能用静态 TOML 表达的初始化参数。依据为官方 NPM 包 [browser-core 3.3.21](https://registry.npmjs.org/@cloudcare/browser-core/-/browser-core-3.3.21.tgz) 中的 `types/index.d.ts`，并核对内嵌 SDK 的校验和默认值。发布构建前同步最新稳定 v3，并重新测试；同域方式在站点执行 enable 时升级。

## SDK 加载方式

三个服务器共享站点级 `[sites.sdk]`，与 `[sites.rum]` 同级；RUM、回放及采样参数在两种加载方式下相同。

| 参数 | 默认值 | 行为 |
|---|---|---|
| `source` | `"bundled"` | `bundled` 使用同域内置 SDK；`cdn` 由同域初始化脚本异步加载外部 SDK |
| `url` | 省略 | 仅用于 cdn，省略时采用发布包 sdk.json 的官方来源；显式 URL 必须是无认证、查询或片段的完整 HTTPS 脚本地址 |
| `integrity` | false | 仅用于 cdn；true 设置对应发布包 SHA-256 的 SRI 与 crossorigin=anonymous，并预检 CORS |

```toml
[sites.sdk]
source = "cdn"
# url = "https://cdn.example.com/rum/3.3.21/dataflux-rum.js"
# integrity = true
```

check/enable 及部署后 probe 检查 CDN 的 identity/gzip 内容、JavaScript MIME 和发布包摘要；自建 CDN 必须发布相同字节，不接受其他版本或精简版。CSP 必须允许同域初始化脚本、实际 CDN 脚本 URL、上报目标及所需 Worker，工具不会修改策略。CDN 不可达、摘要不一致或 SRI 缺少允许站点 Origin（或 `*`）的 Access-Control-Allow-Origin 时阻止启用。

官方 [CDN v3 地址](https://docs.guance.com/real-user-monitoring/web/app-access/) 是可变地址；2026-10-20 实测无 SRI 所需 CORS 头，默认 integrity=false 可直接加载。该模式的摘要校验只说明探测时内容匹配，不能固定后续浏览器收到的内容；CDN 更新可能让已启用页面先于工具升级。需要固定版本时使用不可变 CDN URL + integrity=true，或使用默认 bundled。外置 Worker 仍须匹配实际 SDK，切换版本后用新会话验证回放。

部署快照保存实际 URL、SDK 版本和摘要，回滚使用历史引用；工具不能恢复外部 CDN 文件。外部旧内容被覆盖/删除时回滚探测会失败并恢复本次受管配置。停用不依赖 CDN 可达；保留的本地 SDK 副本不会自动作为失败降级，加载失败不阻塞业务。源为 bundled 时填写非空 url 或 integrity=true 会被拒绝，避免配置被静默忽略。

未填写的参数使用下表默认值；除 Rumable 明确设置的采集默认值外，其余交给该版本 SDK。未知字段、错误类型、非法枚举、NaN/Infinity、无效 URL 和越界数值会在部署前拒绝。布尔值 `false` 和数值 `0` 会保留，不会被默认值覆盖。

上报地址必须是完整 HTTP(S) origin，可带末尾 `/`，不能带路径、查询串（包括空 `?`）或片段；显式端口须在 1–65535。固定 SDK 直接拼接查询参数，因此 `applicationId` / `clientToken` 不接受空白、控制字符和 `& # + % ;`，避免令牌或应用 ID 在传输时被改变；不要自行 URL 编码这些值。env/service/version 等普通标签仍按 JSON 正确转义。

## 应用、采样和采集

| 参数 | 类型 / 默认值 | 说明 |
|---|---|---|
| `applicationId` | 字符串，必填 | 观测云 Web 应用 ID |
| `site`、`clientToken` | 字符串 | OpenWay 地址与浏览器令牌 |
| `datakitOrigin` | 字符串 | DataKit origin，与 OpenWay 二选一；HTTPS 页面必须 HTTPS 上报 |
| `env`、`service`、`version` | 字符串 | 环境、服务和应用版本 |
| `sessionSampleRate` | 数值，100 | RUM 会话采样，0–100，支持小数 |
| `sessionOnErrorSampleRate` | 数值，0 | 未采样会话的错误补偿，0–100 |
| `sessionReplayEnabled` | 布尔，false | Rumable 的回放开关；关闭时传给 SDK 的两项回放采样率均为 0，且不调用录制 API |
| `sessionReplaySampleRate` | 数值，100 | 普通回放采样，0–100，仅开关开启时生效 |
| `sessionReplayOnErrorSampleRate` | 数值，0 | 错误触发回放采样，0–100；可与普通回放采样 0 组合，仅录制错误会话 |
| `telemetrySampleRate` | 数值，SDK 默认 100 | SDK 自身遥测采样，0–100；不决定业务 RUM 会话是否采集 |
| `trackUserInteractions` | 布尔，true | 自动采集用户操作 |
| `actionNameAttribute` | 可选字符串 | 从指定 HTML 属性获取操作名称 |
| `enablePrivacyForActionName` | 布尔，false | 对自动操作名称应用隐私规则；显式名称属性按 SDK 规则处理 |
| `defaultPrivacyLevel` | `allow` / `mask` / `mask-user-input`，默认 `mask` | DOM 回放与受隐私规则控制的数据脱敏 |
| `trackViewsManually` | 布尔，false | true 后需要业务调用 `startView()`；工具不代替业务管理 SPA 路由 |
| `enableLongAnimationFrame` | 布尔，false | SDK 的 Long Animation Frame 模式，取决于浏览器能力 |
| `enableExperimentalFeatures` | 字符串数组，空 | 此 SDK 支持 `"track_websockets"`；其他实验名称拒绝 |
| `excludedActivityUrls` | URL 字符串数组，空 | 排除影响页面 loading time 的活动；SDK 使用完整 URL 字符串匹配，不是禁采资源的规则 |

回放仍依赖 SDK 的 RUM 会话采样；常规与错误补偿 RUM 采样都为 0 时，回放采样不能单独恢复采集。错误回放会在浏览器缓冲错误前的记录，`isRecording()` 可能在错误发生前就为 true；应结合错误后上传和服务端回放验证。

回放开关开启且任一回放采样率大于 0 时，Rumable 在 SDK init 后调用一次 `startSessionReplayRecording()`。采样结果按会话保持，改变采样率后用新会话验收。远程配置开启时，即使本地两个采样率均为 0，也允许向 SDK 请求录制，由远程最终采样决定。

## 会话与本地存储

| 参数 | 类型 / 默认值 | 说明 |
|---|---|---|
| `sessionPersistence` | `cookie` / `local-storage`，默认 cookie | 会话存储策略；与同页其他官方 SDK 使用一致设置 |
| `useSecureSessionCookie` | 布尔，false | Secure Cookie，工具要求目标页面 HTTPS |
| `usePartitionedCrossSiteSessionCookie` | 布尔，false | 分区跨站 Cookie，工具要求 HTTPS；实际支持取决于浏览器 |
| `trackSessionAcrossSubdomains` | 布尔，false | 跨子域共享会话 |
| `allowFallbackToLocalStorage` | 布尔，false | 未显式选择存储策略时，Cookie 不可用可回退到 localStorage |
| `storeContextsToLocal` | 布尔，false | 持久化公共上下文 |
| `storeContextsKey` | 可选字符串 | 上下文存储键；省略时 SDK 自动生成 |

Cookie 的安全、分区与跨子域选项只影响 Cookie 模式；localStorage 按浏览器 origin 隔离。模板显式设置了 `sessionPersistence`，需要自动回退时应删除该字段。

## 上报与 Worker

| 参数 | 类型 / 默认值 | 说明 |
|---|---|---|
| `compressIntakeRequests` | 布尔，false | 压缩普通 RUM 请求，需要 Worker；与回放压缩不同 |
| `sendContentTypeByJson` | 布尔，false | 以 JSON Content-Type 上报，需接收端允许相应 CORS 请求 |
| `workerUrl` | 可选 URL | 回放与普通上报压缩的自托管 Worker |
| `replayCanvasWorkerUrl` | 可选 URL | Canvas 编码专用 Worker，不能代替 `workerUrl` |
| `remoteConfiguration` | 布尔，false | 开启 SDK 远程配置；需观测云支持，远程可覆盖静态 SDK 参数 |
| `silentMultipleInit` | 布尔，false | SDK 的重复初始化提示策略；不取消 Rumable 的防重和既有 SDK 保护 |

Worker URL 必须为根路径 `/workers/rum.js` 或与 `publicURL` 同源的绝对 HTTP(S) URL。文件需由运维托管，版本必须与内嵌 SDK **3.3.21** 匹配；工具不替换业务文件、不下载最新 Worker。`check` / `enable` 和 HTTP `probe` 检查活动的外部 Worker 返回 200 和 JavaScript MIME；这不能证明 Worker 内部代码正确。

`compressIntakeRequests` 与 `sendContentTypeByJson` 不能同时为 true：SDK 3.3.21 的 JSON 模式绕过普通请求压缩，工具会在部署前拒绝这一无效组合。回放片段的压缩不受这个互斥关系影响。

不指定地址时使用 SDK 内嵌 Blob Worker。开启普通压缩、任一回放通道或远程配置时，按 `worker-src` → `child-src` → `script-src` → `default-src` 检查有效 CSP。Blob Worker 保守要求显式 `blob:`；自托管 Worker 检查其 URL，通常可由 `'self'` 放行。Canvas 启用时再检查它的独立 Worker。远程配置可能改变压缩设置，因此即使本地关闭压缩也预检 Worker。工具不自动修改 CSP。

## Trace 与 Visitor ID

| 参数 | 类型 / 默认值 | 说明 |
|---|---|---|
| `allowedTracingUrls` | 数组，空 | 允许注入 Trace Header 的 URL；空数组关闭注入 |
| `traceType` | 枚举，SDK 默认 ddtrace | `ddtrace`、`zipkin`、`zipkin_single_header`、`w3c_traceparent`、`w3c_traceparent_64bit`、`skywalking_v3`、`jaeger` |
| `tracingSampleRate` | 数值，SDK 默认 100 | Trace 采样，0–100，不决定 RUM 会话采样 |
| `traceId128Bit` | 布尔，false | 128 位 Trace ID；效果取决于选定的 Trace 格式 |
| `allowTraceHeaderWithoutSession` | 布尔，false | 未采样会话也允许注入 Trace Header，不会因此补采 RUM |
| `visitorId.enabled` | 布尔，false | 对匹配的 Fetch/XHR 添加匿名 Visitor ID Header |
| `visitorId.header` | 小写 Header 名，启用时必填 | 禁止覆盖认证、安全、浏览器保留与链路 Header |
| `visitorId.match` | 非空 URL 字符串数组，启用时必填 | SDK 按同源 URL 前缀匹配规则选择请求 |
| `visitorId.ttlHours` | 正有限数值，SDK 默认 24 | Web Visitor ID 有效小时数；原生桥接场景由 App 管理 |

`allowedTracingUrls` 可以混合字符串和指定 Trace 格式的表：

```toml
[sites.rum]
# 保留应用及上报参数。
allowedTracingUrls = [
  "https://api.example.com/",
  { match = "https://api.example.com/v2/", traceType = "w3c_traceparent" },
]

[sites.rum.visitorId]
enabled = true
header = "x-rum-visitor-id"
match = ["https://api.example.com/"]
ttlHours = 24
```

URL 匹配项必须为绝对 HTTP(S) 字符串，允许查询串，不允许认证信息和片段；遵循 SDK 的字符串匹配语义。这里不把字符串当作 JavaScript 正则或函数执行。Trace 和 Visitor ID Header 跨域时，需要目标 API 的 CORS 明确允许对应 Header，HTTP 页面探测不能替代该验收。

## Canvas 2D 回放

| 参数 | 类型 / SDK 默认值 | 说明 |
|---|---|---|
| `replayCanvasEnabled` | 布尔，false | 需要回放开启且会话命中采样 |
| `replayCanvasMode` | `auto` / `manual`，auto | manual 需要业务调用 `snapshotCanvas()` |
| `replayCanvasSampling` | 正有限数值或 `"all"`，2 | 数值快照采样或 2D 命令录制；不是帧率 |
| `replayCanvasQuality` | 0–1 数值，或 low / medium / high，默认 0.4 | 字符串质量预设还会影响默认采样与节奏；显式参数覆盖预设 |
| `replayCanvasMimeType` | 字符串，image/webp | 支持 image/png、image/jpeg、image/webp |
| `replayCanvasMaxCanvasSize` | 正有限数值，1280 | 编码前最大边长 |
| `replayCanvasMaxEncodedBytes` | 正有限数值，40000 | 单张编码图像的字节预算 |
| `replayCanvasMaxConcurrentEncodes` | 正有限数值，1 | 并发编码预算，SDK 归整为整数 |
| `replayCanvasFlushImmediately` | 布尔，auto 为 false，manual 为 true | 是否立即将 Canvas 变更刷入回放片段 |
| `replayCanvasAutoInterval` | 正有限数值，250 | 每个 Canvas 的目标快照间隔，毫秒 |
| `replayCanvasAutoCooldown` | 正有限数值，默认跟随 interval | 成功后的冷却，毫秒 |
| `replayCanvasAutoUnchangedBackoff` | 正有限数值，3000 | 未变化状态回退，毫秒 |
| `replayCanvasAutoFailureBackoff` | 正有限数值，5000 | 编码失败回退，毫秒 |
| `replayCanvasAutoMaxPerRun` | 正有限数值，2 | 单次自动快照处理的 Canvas 数量预算 |

质量预设的默认值为：low = 0.25 / 1 / 500ms，medium = 0.4 / 2 / 250ms，high = 0.5 / 4 / 125ms（质量 / 采样 / interval）。模板显式填写预算时，这些填写值会覆盖预设派生值；需要完全跟随预设时删掉对应预算字段。

## 需要业务接入的能力

TOML 不执行 JavaScript。`beforeSend`、`injectTraceHeader`、`generateTraceId`、`shouldMaskNode`、`shouldRecordCanvas`、函数/RegExp URL 匹配器、`plugins` 需要业务提供代码。框架路由插件和额外 WebGL 插件也不由工具加载，Canvas 2D 开关不等于 WebGL 已接入。

本工具不从静态配置自动调用 `setUser()`、`setGlobalContext()`、业务事件、手动 View 和手动 Canvas 快照等运行时 API；动态值由业务提供，静态部署标识可使用 env/service/version。未开放 SDK 内部未公开字段。废弃别名使用规范字段：`datakitUrl` → `datakitOrigin`，`sampleRate` → `sessionSampleRate`，`trackInteractions` → `trackUserInteractions`，`allowedTracingOrigins` / `allowedDDTracingOrigins` → `allowedTracingUrls`，`useCrossSiteSessionCookie` → `usePartitionedCrossSiteSessionCookie`。
