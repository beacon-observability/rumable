# 页面配置与一键接入

页面位于 `web/`，是无构建依赖的静态页面。参数来自当前 Rumable 的 `RUM` 配置和内置 Web SDK 3.3.21 的初始化能力。它可以直接部署到静态站点，或将生成器模块接入现有控制台。

产品长期使用[latest 在线配置页](https://static.guance.com/rumable/latest/setup/index.html)或[latest 发布清单](https://static.guance.com/rumable/latest/manifest.json)。固定版本 `0.1.2` 仍可通过对应版本目录访问。Windows 一键安装与启停回滚测试见 [Windows 验收手册](windows-acceptance.md#一键安装入口验收windows-x64)。

## 当前可用流程

1. 页面填写 SDK 配置与接入目标，立即生成可下载的 TOML。
2. 读取 CI 发布到 OSS 的 latest 或固定版本 `manifest.json`。
3. 根据服务器类型生成一条带配置的命令：Linux 下载并运行 `install.sh`，IIS 下载并运行 `install.ps1`。
4. 安装脚本自动识别架构、下载指定版本包，按脚本内固定摘要校验 SHA-256，安装版本化 CLI，解码临时 TOML，执行 `rumable setup --yes`。
5. `setup` 在单次操作锁内完成首次 render、Nginx attach、enable；已有站点复用原状态进行更新。目标变化、外部修改或未完成事务会阻止操作。

配置在浏览器本地生成，无配置服务依赖，不上传到 OSS。命令中的 Base64 只是防止 shell 转义改变 TOML，不是加密；包含的 clientToken 是浏览器公开令牌，不得填写管理 API Key。临时配置在结束时删除；受管状态保存配置，后续 disable/rollback 不依赖临时文件。需要修改时重新生成命令或使用下载的 TOML 执行 enable。

当前一键入口仅做 RUM，默认同域提供 SDK；不静默安装 Nginx、IIS、URL Rewrite，不部署 OTel Agent。Nginx 仍需 http_sub_module。IIS 需要可匿名访问且有读取权限的 public 资源、URL Rewrite 和管理员权限；页面显式提供静态压缩变更选项。Windows 命令要求 x64、PowerShell 5.1 与现代受支持 Windows；legacy 包继续使用手工流程。

## 页面生成的短命令

Linux 命令结构如下（域名和配置为示意，实际由页面填入）：

```sh
curl --proto '=https' --proto-redir '=https' -fsSL 'https://下载域名/rumable/0.1.2/install.sh' | sudo sh -s -- --base-url 'https://下载域名/rumable/0.1.2/' --config '<Base64 RUM 配置>'
```

Windows 在管理员身份的 64 位 Windows PowerShell 5.1 中执行页面生成的命令，结构如下。`<Base64 RUM 配置>` 必须由页面填写后生成，不能原样执行：

```powershell
& ([scriptblock]::Create((Invoke-WebRequest -UseBasicParsing 'https://static.guance.com/rumable/0.1.2/install.ps1').Content)) -BaseUrl 'https://static.guance.com/rumable/0.1.2/' -Config '<Base64 RUM 配置>'
```

Windows CLI 安装到 `%ProgramFiles%\Rumable\0.1.2\rumable.exe`，状态保存在 `%ProgramData%\Rumable`。保留页面下载的 TOML，配置变更后可重新生成命令；启停与回滚始终使用相同接入标识和状态目录。一键安装器只提取 CLI；发布包不含自动验收脚本，测试时从仓库同版本 tag 获取脚本并指定 CLI 路径，详见 Windows 验收手册。

只有下载入口、发布目录和配置留在命令里，校验、安装、清理和 setup 均由脚本处理。`--base-url` 从 manifest 的 `artifactBase` 相对地址推导，不需要 CI 配置公开下载域名；旧固定版本清单没有此字段时沿用清单目录。Linux 自动选择 x86（32 位）/ x64 / ARM64，IIS 使用 x64 PowerShell 安装脚本。读取 latest 后生成的命令仍绑定一个固定版本；安装期间 latest 更新不会改变已选包。安装脚本通过 HTTPS 获取，包摘要由脚本固定；CI 同时校验脚本和包的完整性。

产品也可固定调用 `https://static.guance.com/rumable/latest/install.sh` 或 `https://static.guance.com/rumable/latest/install.ps1`，base URL 使用同目录 `https://static.guance.com/rumable/latest/`，安装器自行转到脚本内绑定的版本。需要审计或复现时优先保存页面生成的固定版本命令与 TOML。

## SDK 字段对应

| 页面配置 | SDK / Rumable 字段 |
|---|---|
| 应用 ID | applicationId |
| OpenWay / DataKit | site + clientToken / datakitOrigin，互斥 |
| 服务、环境、业务版本 | service、env、version |
| 会话及错误补偿采样 | sessionSampleRate、sessionOnErrorSampleRate |
| 回放开关、普通和错误回放采样 | sessionReplayEnabled 控制 bootstrap 是否启动录制；sessionReplaySampleRate、sessionReplayOnErrorSampleRate 传入 SDK。关闭时归零 |
| 交互采集与名称隐私 | trackUserInteractions、actionNameAttribute、enablePrivacyForActionName |
| 默认隐私、会话存储 | defaultPrivacyLevel、sessionPersistence |
| 长动画帧、压缩、远程配置 | enableLongAnimationFrame、compressIntakeRequests、remoteConfiguration |
| 会话跨子域、HTTPS Cookie、存储回退 | trackSessionAcrossSubdomains、useSecureSessionCookie、allowFallbackToLocalStorage |
| Worker、链路范围、协议、采样 | workerUrl、allowedTracingUrls、traceType、tracingSampleRate |

表单开放常用静态字段，其余已支持的高级静态配置可使用完整 TOML 模板。`setUser`、自定义上下文、`startView`、`beforeSend`、函数/正则匹配器等需要业务代码，不伪装成纯配置开关。`trackViewsManually` 未在页面提供，避免零代码接入后没有业务调用 startView 而丢失视图。远程配置可能覆盖静态选项；Worker、链路跨域请求和真实入库仍需目标环境验收。

## CI → OSS 约定

复用 `scripts/release.go` 打包。新增输出 `manifest.json`，包含 schemaVersion、version、revision、SDK 元数据、`rum-setup-v1` / `rum-install-v1` 能力、安装脚本及各平台包的文件名和 SHA-256；manifest 本身进入 SHA256SUMS。依赖许可证随包发布。

CI 与 OSS 变量、受保护 tag、重试和验收见 [GitLab CI 发布说明](gitlab-ci-release.md)。实际目录：

```text
rumable/
  <version>/
    install.sh
    install.ps1
    manifest.json
    SHA256SUMS
    rumable_<version>_linux_amd64.tar.gz
    rumable_<version>_linux_arm64.tar.gz
    rumable_<version>_windows_amd64.zip
    sdk.json
    ...其他平台包（文档、配置示例和许可证位于包内）
    setup/
      index.html
      app.mjs
      generator.mjs
      styles.css
```

CI 先验证、打包并上传安装脚本、全部包和配置页，核对远端摘要，再发布 manifest；报告输出固定版本的 OSS 路径，供控制台选择。相同版本内容变化会被拒绝；下一次发布使用新版本号，不覆盖已有版本。OSS 使用 HTTPS；跨域部署页面时允许页面域名 GET manifest。同版本 `setup/index.html` 自动预填清单地址；也可用 `?manifest=https://<OSS域名>/rumable/<version>/manifest.json` 指定，在控制台集成时由平台注入地址。

正式版本完成后再更新 `rumable/latest/` 的配置页入口、安装器和清单；清单最后更新，不长期缓存。latest 页面跳到同版本页面，避免 HTML/CSS/JS 混用。旧版本重试不会让 latest 回退，预发布版本不更新 latest。产品读取清单后按 `artifactBase` 解析具体包地址，不能直接把含版本号的包名拼到 latest 目录。

实际 OSS 凭据由项目 CI 管理，不需要公开域名变量。页面在未读到有效清单时禁用“复制接入命令”，仍可预览/下载 TOML；不会生成指向不存在安装包的假成功命令。安装脚本使用所选固定版本及构建时写入的包摘要，不在执行时漂移到 latest。

## 恢复行为

接入失败由现有事务机制恢复注入配置；恢复失败保留 transaction.json，下一次 setup 必须先 rollback。首次 Nginx 挂接也有独立备份。已经挂接的资源位置保留，避免失败期间收到 HTML 的浏览器无法加载旧 SDK；清理 HTML 缓存后，可 disable，再 rollback --attachment --yes 撤销首次挂接。页面开关是下次部署的期望配置，不是在线管理主机的即时开关。

不放宽 CSP，也不保证任意 HTML 可直接接入。当前文本过滤要求标准 `<head>`；每种受管页面模板都应纳入 probePaths。HTTP 检查通过只证明注入和资源可用，页面可另用实际 RUM 事件到达情况判断接入成功；不要把命令复制或脚本退出码当作观测云已收到数据。

## 本地验证

```sh
python3 -m http.server 18872 --bind 127.0.0.1 --directory web
node --test web/generator_test.mjs
go test ./...
bash scripts/smoke-onboarding.sh
```

最后一项在临时 Docker Nginx 中执行页面生成的真实 Linux 命令；OSS 下载用本地脚本和归档替代，实际执行校验、安装、首次接入、重复运行、升级、回滚、停用和撤销。不会对宿主机 /opt、/var/lib 或 Nginx 进行修改。Windows 命令及 IIS 原生行为仍需 Windows 环境验收。
