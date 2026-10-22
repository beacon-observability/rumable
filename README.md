# Rumable

通过 Nginx / IIS 改写 HTML 响应，为网站接入观测云 Web RUM，无需修改业务代码。浏览器直接向 OpenWay 或 DataKit 上报；CLI 执行完即退出。

[打开配置页](https://static.guance.com/rumable/latest/setup/index.html) · [安装与使用指南](USER-GUIDE.md) · [完整配置参考](docs/configuration.md)

## 先选择 SDK 来源与加载方式

| 配置 | 默认 | 可选行为 |
|---|---|---|
| SDK 来源 | 网站同域，SDK 随安装包提供 | 官方 CDN 或提供相同 SDK 文件的自建 HTTPS CDN |
| 加载方式 | 异步，下载不阻塞 HTML 解析 | 同步，先执行 SDK 和初始化脚本，再继续解析后续页面 |
| SDK 更新 | 同域版本随新包重新部署更新 | 可变 CDN 随远程文件和缓存更新；固定 CDN 由运维控制 |

同步加载可以更早初始化，但脚本下载会影响首屏；异步初始化前的事件可能漏采。加载方式与 SDK 来源可自由组合。页面的“应用版本”和“SDK 远程配置”不会升级 SDK 文件。

当前源码基线为 **0.1.5**，内置 SDK **3.3.21**，见 [sdk.json](internal/rumable/assets/sdk.json)。同步加载和自动发现时的 SDK 配置为新增能力，需发布配套页面与安装包后使用。旧清单缺少相应能力时，新页面会阻止生成不兼容命令；此次源码变更不会更新线上 latest。

## 从哪里开始

| 你的任务 | 阅读入口 |
|---|---|
| 第一次安装、选择同域/CDN 和同步/异步 | [使用指南](USER-GUIDE.md#1-sdk-注入加载与更新) |
| 已接入站点升级、停用或回滚 | [日常维护](USER-GUIDE.md#10-修改配置升级停用回滚和卸载) |
| 查询 TOML 参数、默认值和限制 | [配置参考](docs/configuration.md) |
| 排查压缩、缓存、CSP、配置漂移 | [运维手册](docs/operations.md) |
| 核对系统支持和已完成的验证 | [兼容性](docs/compatibility.md)、[验证记录](docs/verification.md) |
| 使用服务端 Trace | [OTel 手册](docs/otel.md)、[依赖安装与卸载](docs/otel-installation.md) |
| 维护配置页、安装器与发布流水线 | [页面集成约定](docs/one-click-onboarding.md)、[开发验证](docs/development.md)、[CI 发布](docs/gitlab-ci-release.md) |
| 处理旧 Windows 0.1.4 失败现场 | [恢复与复测](docs/windows-0.1.4-recovery.md) |

## 接入与后续更新

1. 在配置页填写应用参数，选择 SDK 来源、加载方式和 Linux / Windows。
2. 读取配套发布清单，保存 TOML，在目标服务器执行生成的命令。
3. 打开网站确认脚本执行，并在观测云确认数据入库。

页面和 setup 仅处理浏览器 RUM，服务端 OTel 单独配置。自动发现配置使用 `[rum]`、可选 `[sdk]`，通过 `setup --discover` 应用；完整站点配置使用 `[[sites]]`、`[sites.rum]`、`[sites.sdk]`，支持分步接入与 enable 更新。

已有站点继续使用原配置、接入标识和状态目录。以下命令为完整站点配置示例，路径和 ID 换成已有值：

```sh
rumable enable --config /etc/rumable/website.toml --state-dir /var/lib/rumable --site website --component rum
rumable status --state-dir /var/lib/rumable --site website
rumable probe --state-dir /var/lib/rumable --site website
```

只替换 CLI 不会更新已部署 SDK；本地 SDK 的完整升级步骤和 CDN 缓存、摘要、回滚限制见[SDK 更新机制](USER-GUIDE.md#14-sdk-如何更新和回滚)。停用、回滚和升级都需处理旧 HTML 缓存。HTTP 探测通过不等于浏览器执行或真实入库通过。

想先体验注入和浏览器请求，可运行[本地 Linux / Nginx 演示](examples/local-demo/README.md)。演示请求进入本地模拟接收端。
