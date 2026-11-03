# 7154213599：Nginx 注入与 CDN 校验

## Range 请求

带 `Range` 的请求在 Nginx 正文过滤器启用前绕过 Rumable 注入，保留源站的范围响应处理。普通 HTML 请求继续注入，业务自有过滤器保持原有行为。

保留 `etag off` 和 `if_modified_since off` 缓存保护，避免静态 HTML 使用旧校验值返回 304。因此静态强 ETag `If-Range` 可以按 HTTP 语义回退为完整 200 响应，该响应不注入；日期形式的 `If-Range` 和源站 206 响应继续受支持。

## CDN SDK

`check/enable` 读取配置的 HTTPS SDK 文件，检查稳定 v3 版本、SDK 特征标记、JavaScript MIME、identity/gzip 字节一致性及可选 CORS。CDN SDK 无需与安装包内置版本一致。

`enable` 使用同一次解析得到的文件生成部署资源，保存实际版本、URL 和 SHA-256。部署后的 `probe` 和回滚按保存摘要校验，不会静默接受远程内容变化；停用不依赖 CDN 可达。bundled 模式继续使用内嵌清单。

CDN 来源由配置者选择并信任。版本和特征标记检查只识别候选 SDK 格式；摘要保证内容一致性，不证明官方来源、功能完整或采集兼容性。默认 `integrity=false`；开启时，SRI 使用本次启用保存的摘要。可变 CDN 内容变化后，旧部署的 `probe` 会报摘要不匹配，重新启用可更新保存的版本和摘要；开启 SRI 的旧页面会拒绝变化后的文件。

## Nginx 固定 head 标签

启用时复用 HTML lexer，从每个 `probePaths` 路径的预检响应取得真实文档的完整 head 起始标签，并保存到部署快照。按原始请求路径选择一个匹配值，忽略 query，不受常规 index/try_files 内部重写影响；每个请求只有一条 `sub_filter` 注入规则。禁用、OTel 独立操作及回滚保留保存的锚点。

支持固定属性、空白、换行和引号内的 `>`。预检拒绝缺失或自闭合 head、非正常文档开头，以及真实标签之前注释或属性中的同样标签文本。不增加 Nginx 模块，保留 Range、内容类型、状态码、缓存及排除路径门禁。

支持边界：

- 带属性的每个页面路径必须列入 `probePaths`；其他路径保持原有纯 `<head>` 匹配。
- 同一路径随 query、Cookie、语言或用户变化的 head 属性不支持；路径按预检 URL 的编码形式匹配。
- 标签最长 1024 字节，不支持 `$` 或 NUL。属性、引号、空白或顺序变化后重新启用；head 内部内容变化不影响匹配。
- Nginx 匹配不区分 ASCII 大小写，输出使用保存值。仅大小写变化可能被替换掩盖，无法保证原样保留或由 `probe` 检出。
- `probe` 检查已探测页面的可观察锚点变化并提示重新启用；CLI 不常驻监控。
- IIS 保持现有纯 `<head>` 要求。

## 验证

验证环境：Go 1.20.3 darwin/amd64、原生 Nginx 1.24.0、本地合成 HTML 和 TLS SDK 服务。验证范围为配置生成、状态事务及 HTTP 响应，不代表浏览器 SDK 执行或采集入库验收。

- 全量 `go test ./...`、`go vet ./...`、CLI 构建及 `git diff --check` 通过。
- `TestNginxHeadAnchors`、`TestNginxRangeResponses`、`TestCDNEnableUpgradeRollback` 连续 3 轮通过。
- Head：完整属性和正文保留、注入位置与次数、多路径、query、内部重写、排除路径、变化提示、重新启用、幂等、禁用、回滚，以及不安全预检失败时配置与状态保持不变。
- Range：静态/代理各 8 种 Range/If-Range 场景，普通 GET/HEAD、缓存校验及非 HTML 回归。
- CDN：不同于内嵌版本的 SDK、重复启用、远端升级、非法内容拒绝、历史摘要校验、回滚、同步 SRI 及离线停用。
- 单元测试覆盖 HTML 伪标签、缺失/自闭合标签、危险字符、长度、IIS 边界及快照保留；CDN 覆盖格式、MIME、主版本、预发布版本、重定向、压缩一致性与 CORS。

复跑原生 Nginx 集成测试：

```sh
RUMABLE_TEST_NGINX=/absolute/path/to/nginx go test ./internal/rumable \
  -run 'TestNginxHeadAnchors|TestNginxRangeResponses|TestCDNEnableUpgradeRollback' -count=3 -v
```

未设置 `RUMABLE_TEST_NGINX` 时，这些原生集成用例明确跳过；普通 `go test ./...` 不启动本机 Nginx。
