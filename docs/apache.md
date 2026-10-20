# Linux / Apache 接入与验收

适用范围：Apache 2.4.23+，静态网站、普通动态页面、反向代理与普通 chunked 响应。使用 Apache 原生 `mod_lua` 输出过滤器，不安装额外常驻进程。CLI 的 Linux / Go 兼容性、服务器版本与浏览器 SDK 要求分别判断，见[兼容性矩阵](compatibility.md)。

SDK 默认同域托管；站点可通过 `[sites.sdk] source="cdn"` 改为 CDN，初始化脚本和站点参数仍由同域资源提供，仍须保留 Alias。CDN URL、可选 SRI、CSP 及外部版本回滚限制见[配置参考](configuration.md#sdk-加载方式)。

## 首次部署

1. 先在测试 VirtualHost 准备站点与可匿名返回 200 的完整 HTML 页面，为每个真实 HTML handler 配置 probePaths。
2. 安装并加载 `lua`、`filter`、`alias`、`authz_core`、`headers` 模块。Debian/Ubuntu 可执行 `sudo a2enmod lua filter alias headers`，authz_core 通常已启用；其他发行版按其模块包和 LoadModule 配置操作。代理、压缩模块由原站点管理。
3. 执行 `rumable init --server apache --reporting openway --config apache.toml`，生成 ID 为 example 的精简配置；填 publicURL、路径、上报值和与服务一致的 binary/args。也可复制发布包的 `examples/apache.toml`（ID 为 apache-example）；以下采用 init 生成的 example，使用其他 ID 时替换对应路径和参数。Debian/Ubuntu 推荐 `/usr/sbin/apache2ctl`，保留 `/etc/apache2/envvars`；原生 httpd 可使用 `args = ["-f", "/etc/httpd/conf/httpd.conf"]`。不能将 systemctl 当作 binary。

```sh
sudo rumable render --config apache.toml --site example --state-dir /var/lib/rumable
cat /var/lib/rumable/sites/example/ATTACH.md
```

Linux 上优先执行 `sudo rumable attach --config apache.toml --site example --state-dir /var/lib/rumable`，自动从原生 -S 输出识别目标、展示差异并等待 yes。成功后无需设置 attachmentReviewed；`--dry-run` 仅预览，`--target FILE:LINE` 处理歧义。备份、故障恢复和 `rollback --attachment` 见[自动挂接手册](operations.md#自动首次挂接linux)。已有 AddOutputFilterByType、不明过滤链、rewrite/.htaccess 等转为下述人工挂接；自动流程不安装模块或重排未知业务过滤器。

以下为人工挂接方式：

在**目标 VirtualHost 内**、原有 Location/LocationMatch 等业务配置之后，挂接一次：

```apache
<VirtualHost *:443>
    # 原来的 TLS、DocumentRoot、ProxyPass、Location 等配置。
    Include "/var/lib/rumable/sites/example/apache.conf"
</VirtualHost>
```

不要放在全局 server 或嵌套 Location/Directory；建议一个 VirtualHost 对应一个 Rumable 站点，以路径规则控制范围。同机不同 VirtualHost 可以使用独立 ID。CLI 根据 `-t -D DUMP_INCLUDES` 确认 include 恰好一次；这不能证明任意业务配置的挂接语义，仍须审查真实 VirtualHost 与所有 HTML handler。

如果原配置存在 `ProxyPass / ...`，在其**前面**加保留路径排除，例如：

```apache
ProxyPass "/__rumable__/example/" "!"
ProxyPass "/" "http://backend/"
```

子路径应用使用 ATTACH.md 给出的完整保留路径。业务 RewriteRule、SetHandler、SetOutputFilter、AddOutputFilter 等也必须绕过该路径。受管资源 Location 清除继承的 smart FilterChain；这不会清除所有其他模块的过滤或路由。提交后的 probe 分别检查初始化与 SDK 的 identity / gzip 响应，解压后都必须匹配受管 SHA-256，不接受被业务过滤器或过期预压缩文件改写的脚本。外置 Worker 也检查两种编码的 MIME 和内容一致性，但不验证代码属于指定 SDK 版本。

Apache worker 仅需读取 `sites/<id>/public`、`sites/<id>/filters`，并遍历父目录；管理员拥有写权限。私有 Lua 文件在 filters 中，不由 Alias 暴露。不要把整个状态目录设置为网站根目录或将 state.json、事务日志设为可公开读取。

完成 ATTACH.md 中的压缩、缓存、权限、CSP 和继承检查，按发行版正常流程执行一次 configtest / graceful reload，然后设置 `apache.attachmentReviewed=true`：

```sh
sudo rumable enable --config apache.toml --site example --state-dir /var/lib/rumable
rumable probe --site example --state-dir /var/lib/rumable
rumable status --site example --state-dir /var/lib/rumable
sudo rumable disable --site example --state-dir /var/lib/rumable
sudo rumable rollback --site example --state-dir /var/lib/rumable
```

enable 自动包含预检，无需先执行独立 check。单站点可省略 --site；多站点需显式指定。后续修改 TOML 再 enable 即升级；SDK 参数、隐私与 Session Replay 的含义和默认值与 Nginx/IIS 相同。回放默认关闭，开启示例：

```toml
[sites.rum]
# 保留原有应用和上报配置。
sessionReplayEnabled = true
sessionReplaySampleRate = 20
```

## 压缩、缓存和继承

- 要求进入 Lua 过滤器的源 HTML 未压缩；已有压缩源直接旁路。可关闭后端源压缩，或由运维将 INFLATE 安排在 Rumable 之前；保留现有业务过滤器和输出 DEFLATE。不要盲目清除客户端 Accept-Encoding，否则 Apache 输出压缩也会受影响。
- Apache 输出 gzip 可继续使用。静态 `.gz`/`.br` 路由、预压缩重写和自定义过滤顺序须人工审查；工具探测 identity 和 gzip 两种页面及脚本响应，Brotli 未增加解码验收。
- 输出压缩必须晚于 RESOURCE 阶段的 Rumable 过滤器。旧版 Apache 的父层 AddOutputFilterByType DEFLATE 可能按 RESOURCE 插入而先压缩；在原压缩配置中改用显式 CONTENT_SET 链。保留原有 MIME 条件、压缩参数及业务过滤器，不要叠加两次 DEFLATE。例如：

```apache
FilterDeclare OUTPUT_GZIP CONTENT_SET
FilterProvider OUTPUT_GZIP DEFLATE "%{CONTENT_TYPE} =~ m|^text/html|"
FilterChain +OUTPUT_GZIP
```

- Rumable 将命名 RESOURCE 过滤器加入原业务链，按阶段先处理 HTML、后输出压缩。2.4.23 上的不兼容继承压缩已纳入回归：gzip 探测发现未注入后，enable 失败并恢复原受管状态。
- 只在选中 GET / 200 / text/html / 无源 Content-Encoding / 非 attachment 响应中插入。跨 bucket 保留至多 6 字节；锚点大小写不敏感，只处理第一处。没有 head 时正文不变，探测失败。UTF-16/32、HTML 片段、流式 SSR 不支持。
- 启用时，所选路径中的 GET/HEAD 清除 If-None-Match、If-Modified-Since、Range、If-Range，避免处理前返回旧 304/206。该范围内非 HTML 也失去这些请求头，请将资源/下载目录放入 excludePaths；其他方法保留条件头。
- 合格 HTML 去除 Content-Length、ETag、Last-Modified、Content-MD5、Digest、Accept-Ranges。HEAD 不注入正文，但同样清除 HTML 的原文验证头及长度，防止缓存据此验证改写后的 GET 页面。静态 HEAD handler 在内部经过过滤链的临时正文会被丢弃，防止 Apache 随后重新计算原长度；HTTP HEAD 本来就不发送正文。Host、认证、自定义请求头与安全响应头不被覆盖。没有锚点的合格 HTML 也会去除这些验证头。
- 加载 mod_cache 时，启用会对整个 VirtualHost 设置 CacheQuickHandler off，并 CacheDisable 每个 includePaths 前缀。这会影响前缀内非 HTML 和排除路径的缓存；停用去掉这些受管指令，恢复原配置。这样不会把带部署专用脚本的 HTML 写入本地 mod_cache，也不会被旧 quick cache 绕过。
- 不要在 include 后覆盖这些缓存设置。业务 FilterChain 在其原范围内保留，Rumable 加入命名 RESOURCE filter；后续 `.htaccess`、Location、LuaInherit、重复 provider 或显式清空链仍可能改变实际行为。每个 handler 都需探测，不能只测首页。
- 启停、升级与回滚后清理 CDN/外层代理、浏览器和 Service Worker 的 HTML 缓存，再重新加载验收。老初始化、SDK 和私有 Lua 文件保留，避免旧页面或旧 worker 读不到资源；不要提前删除。

启用先预检，保存事务日志与版本资源，再写入 apache.conf；原生配置检查通过后执行 `-k graceful`，随后 HTTP 验证。任一步失败恢复本次受管文件及原 HTTP 行为，不恢复整台 Apache 配置。受管 include 或版本资源存在外部修改时拒绝覆盖。业务配置的外部修改不会被回滚清理。

## 可执行运行验证

仓库根目录运行，需 OrbStack Docker、Go、curl，端口 19180/19181/19182 空闲：

```sh
docker pull httpd:2.4
scripts/smoke-apache.sh
# 从未修改的官方源码构建最低版本，固定源码 SHA-256：
docker build -f scripts/apache-2.4.23.Dockerfile -t rumable-apache:2.4.23 .
APACHE_IMAGE=rumable-apache:2.4.23 scripts/smoke-apache.sh
# 使用已解压的 Linux 发布二进制：
RUMABLE_BINARY=/absolute/path/to/rumable scripts/smoke-apache.sh
# 保留仅供本机测试的页面和本地接收器，便于 Chrome 验收：
KEEP=1 scripts/smoke-apache.sh
```

脚本支持 APACHE_IMAGE 替换兼容的 httpd 镜像，默认 `/usr/local/apache2` 布局。失败返回非零；未设置 KEEP 时清理本轮容器、网络和目录。KEEP 输出归属信息，完成 Chrome 验收后按该信息执行 `docker rm -f <server> <fixture>`、`docker network rm <network>`，只清理本轮现场。

脚本覆盖静态/反代/普通分块、首锚点、缺失锚点正文保留、gzip、下载/API、源压缩拒绝、条件/Range 请求、POST 条件头保留、缓存绕过与恢复、父业务过滤/安全头、独立 VirtualHost 多站点隔离、模块缺失、漏挂/重复 include、低版本阻止、幂等启停、升级回滚、首次尚未确认挂接的停用快照恢复、configtest/reload/提交后 HTTP 故障恢复及配置/私有 Lua 摘要漂移。

用本机 Chrome 打开 `http://127.0.0.1:19180/`，点击 Inspect recording、Fetch resource 和 Trigger test error，确认仅一个 Rumable 与 SDK 标签、页面交互正常。修改测试 TOML 的回放开关并 enable 后点击 New session，确认 recording 和 SDK 采样配置。Console 仅应包含主动触发的测试错误，Network 中脚本和上报应成功。接收端 `http://127.0.0.1:19181/__stats` 的事件及 replaySegments 只证明本地 SDK 发送；最终仍需有效 OpenWay/DataKit 和观测云入库验收。

实际执行的版本、摘要和验证结果见[验证记录](verification.md)。
