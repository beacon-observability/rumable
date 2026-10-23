# Rumable 文档内容与品牌约定

本流程参考 Browser SDK 的 publish-browser-sdk-docs，但根据 Rumable 的 CLI 和注入机制调整，不能照搬 npm 发布或 SDK 更新日志要求。

## 专题组织

目标目录是 `docs/zh/real-user-monitoring/web/rumable/`：

- `index.md`：适用场景、平台前提、安装入口、接入和验收。
- `configuration.md`：同域/CDN、同步/异步、完整与发现 TOML、采样和回放。
- `operations.md`：修改配置、SDK 更新、停用、回滚、卸载和排错。
- `.pages`：专题导航；上级 Web `.pages` 注册整个目录。

若目标仓库已有不同组织，以当前维护入口为准，不建立同主题的重复页面。使用相对链接连接已有的 app-access、sampling 等指南，不复制整套 Browser SDK 参数表。正式用户说明不以内部历史故障复盘开头，也不依赖私有源码仓库才能完成安装。

## 必须准确表达的行为

- CLI 改写 Web 服务器返回的 HTML，不是常驻采集服务；SDK 在浏览器执行，直接向 OpenWay / DataKit 上报。同域托管不等于同域上报。
- `source` 和 `loading` 独立；默认 bundled + async。同域 SDK 来自包内固定文件。CDN 模式仍使用同域初始化脚本。
- 同步是两个有序的外部普通 script，不带 async/defer；阻塞 HTML 解析。异步初始化前可能漏采。同步不能保证捕获 SDK 失败或初始化前的一切事件，也不会消除网络等待。
- 当前完整配置使用 `[sites.sdk]`，发现配置用 `[sdk]`；后者交给 `setup --discover`，不能直接交给 enable。
- CDN 内容在启用/探测时须与包内摘要一致。可变 URL 不锁定以后浏览器收到的内容；SRI 的跨域 CORS 要求和自建固定地址需单独说明，不宣传自动本地降级。
- 同域更新需要新 CLI + 对原站点重新 enable/setup；CLI 本身升级、latest 清单、业务 version、remoteConfiguration 都不自动替换已部署 SDK。回滚历史 URL 不能还原被覆盖的远程 CDN 文件。
- 保留部署 ID、目标和 state-dir；区分普通 rollback、Nginx attachment 回滚及 uninstall。停用后旧 HTML 缓存和已打开页面仍可能运行 SDK。
- 接入检查应包含 HTTP 注入、浏览器执行和应用数据入库的具体步骤。内部测试方式、未验证事项和验证进度留在研发记录或 MR，不放进用户指南。

## 模板与品牌

沿用当前 mkdocs 配置中的 `<<< ... >>>` 语法：

- 显示名称：`<<< custom_key.brand_name >>>`。
- 共享域名：`<<< custom_key.brand_main_domain >>>`。
- 只有实际验证两个品牌都发布了对应资源时，才使用 `https://static.<<< custom_key.brand_main_domain >>>/...` 作为共同的下载入口。
- `Rumable`、`dataflux-rum.js`、配置字段和命令是产品/接口标识，不按品牌改名。
- 需要 NPM 示例时，对完整代码块作品牌条件分支；TrueWatch 用 `@truewatchtech/browser-*`，另一分支用 `@cloudcare/browser-*`。Rumable 安装通常不需要 NPM。

Rumable 当前内置 SDK 的元数据来源是 Guance。若未验证另一品牌的安装包、SDK 摘要和下载入口，不要机械改域名并宣称支持。可将已验证的安装/配置内容放在对应品牌分支，另一分支直接引导到该品牌可用的通用 Web 接入方式；不要把品牌验证进度写入用户说明。将来有配套发布后依据证据扩展。

条件必须包住完整段落、表格或代码块，不能插入 JS/JSON/TOML 字符串内部。指南用功能前提解释兼容性，不固定某次发布号，也不使用内部提交号代替可获取的安装包。

## 长期有效的下载与命令

- 配置页和发布清单链接使用 `latest`；不要固定某次发布的下载 URL、包文件名、CLI 路径或带日期的“当前版本”快照。
- 安装脚本从同一次读取的清单解析版本、包名、摘要并保持一致。生成的实际安装命令可锁定这次发布；用户指南本身不维护这些具体值。
- 手动下载示例解析清单，而不是让维护者每发一版就改脚本常量。运维命令使用用户安装时记录的 CLI 路径。
- 功能受所选安装包能力限制，不能把 `latest` 等同于源码所有功能均已发布；不要为兼容而修改清单声明。
- 平台最低兼容版本、业务应用 version 示例和 Browser SDK 的稳定主版本路径属于不同用途，可以保留。

## 发布证据

维护文档前读取实际 manifest；核对 version、revision、capabilities、sdk.version/source/sha256。URL 可从仓库 README、发布脚本或用户指定入口获取。

若源码已有功能而公开包尚未支持，面向用户的指南继续描述已发布功能；提前准备的新内容留在 Draft MR，发布状态和依赖只写入 MR 描述。已确认测试及发布完成时，清理用户指南中残留的“未验证”“待发布”等表述，旧 CLI 问题改为可执行的升级步骤。不要假设下一版本号或编造下载链接；仅发布版本改变时不改指南，功能、配置或操作步骤改变时再维护相应内容。
