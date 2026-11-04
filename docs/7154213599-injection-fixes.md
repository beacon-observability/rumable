# 7154213599：Nginx 注入与 CDN 校验

## Range 请求

带 `Range` 的请求在 Nginx 正文过滤器启用前绕过 Rumable 注入，保留源站的范围响应处理。普通 HTML 请求继续注入，业务自有过滤器保持原有行为。

保留 `etag off` 和 `if_modified_since off` 缓存保护，避免静态 HTML 使用旧校验值返回 304。因此静态强 ETag `If-Range` 可以按 HTTP 语义回退为完整 200 响应，该响应不注入；日期形式的 `If-Range` 和源站 206 响应继续受支持。

## CDN SDK

`check/enable` 读取配置的 HTTPS SDK 文件，检查稳定 v3 版本、SDK 特征标记、JavaScript MIME、identity/gzip 字节一致性及可选 CORS。CDN SDK 无需与安装包内置版本一致。

`enable` 使用同一次解析得到的文件生成部署资源，保存实际版本、URL 和 SHA-256。部署后的 `probe` 和回滚按保存摘要校验，不会静默接受远程内容变化；停用不依赖 CDN 可达。bundled 模式继续使用内嵌清单。

CDN 来源由配置者选择并信任。版本和特征标记检查只识别候选 SDK 格式；摘要保证内容一致性，不证明官方来源、功能完整或采集兼容性。默认 `integrity=false`；开启时，SRI 使用本次启用保存的摘要。可变 CDN 内容变化后，旧部署的 `probe` 会报摘要不匹配，重新启用可更新保存的版本和摘要；开启 SRI 的旧页面会拒绝变化后的文件。

## Nginx head 标签

新启用恢复普通 `<head>` 匹配，大小写不敏感。`probePaths` 只做验收，不生成逐路径注入规则。旧 0.1.9 属性规则只用于历史兼容，更新前检查模板；禁用与回滚可恢复原配置。不新增模块或安装参数。完整边界见 [普通 head 注入](nginx-plain-head.md)。

## 验证入口

原生 Nginx 集成测试需设置 `RUMABLE_TEST_NGINX`，覆盖普通 head、多页面、0.1.9 迁移/失败保护/禁用/回滚、Range 与 CDN 生命周期：

```sh
RUMABLE_TEST_NGINX=/absolute/path/to/nginx go test ./internal/rumable \
  -run 'TestNginxPlainHead|TestNginxLegacyHeadMigration|TestNginxRangeResponses|TestCDNEnableUpgradeRollback' -count=3 -v
```

未设置该变量时原生用例明确跳过。浏览器验收与 HTTP 检查不等同于共享空间的数据入库验证。
