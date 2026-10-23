# OTel 自动安装与卸载

本页适用于 **0.1.0-preview.3** 及之后版本，直接使用对应 Windows/Linux 发布包中的 CLI，在实际服务器上执行，安装、render、attach、enable 始终使用相同的绝对 `--state-dir`。

## 最短流程

复制 `examples/otel-nginx.toml` 或 `examples/otel-iis.toml`，填写站点、服务器和上报地址。省略 `modulePath` / `agentPath`，让工具选择受管路径：

```sh
rumable install-otel --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
# 展示下载来源、目标目录和影响，输入 yes；自动化可加 --yes。
rumable render --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
rumable attach --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
rumable enable --config /etc/rumable/otel.toml --state-dir /var/lib/rumable
```

`install-otel --dry-run` 展示安装计划，不下载或安装依赖，不改服务器配置。重复安装验证已有受管文件并复用，不覆盖文件。安装不自动挂接或启用采集；现有 `attach` 继续展示精确业务配置差异并确认。DataKit 或 Linux Dataway 转换用 Collector 需先按 [OTel 手册](otel.md)运行。

| 平台 | 自动完成 | 先决条件及边界 |
|---|---|---|
| Nginx Linux x64 / ARM64 | 从已配置 APT 索引选择依赖**准确 Nginx 版本**的 `nginx-module-otel`；下载、校验索引 SHA-256、私有解包并执行原生装载检查 | Nginx 1.25.3+、APT/dpkg-deb 和模块所需系统运行库；已有可信 nginx.org 软件源及最新索引。工具不添加软件源、不升级 Nginx、不执行包安装脚本。发行版/自编译 Nginx ABI 不匹配时拒绝 |
| IIS Windows x64，x64/x86 池 | 下载官方 .NET Agent **v1.17.0**，校验摘要并解包两个位数的 Agent，向专属依赖树授予 IIS_IUSRS 读取/执行权限 | IIS 10、Windows 10/Server 2016+、管理员的 64 位 Windows PowerShell 5.1、独占 .NET 池；不全局注册或重启 IIS。Windows ARM64、旧 Windows/IIS 仍为 RUM-only |

依赖放在 `<state-dir>-otel/<服务器类型>-<摘要>/`，与私有状态目录相邻。Linux 模块与 IIS Agent 路径由工具确定，无需回填 TOML。同一 state-dir 内、同一服务器实例及相同安装源会复用依赖。不要跨 state-dir 接管相同服务器；不要移动依赖目录。IIS 权限设置失败会保留 pendingACL 标记并阻止 enable；再次 install-otel 可继续完成，也可按归属清理。IIS 的父目录还需允许池身份遍历，CLI 不放宽状态目录或父目录 ACL；若使用额外受限的父目录，应按现场权限策略授予遍历权限。

## Windows 可执行步骤

```powershell
# 以管理员打开 64 位 Windows PowerShell 5.1，先编辑示例，省略 agentPath。
$state = 'C:\ProgramData\Rumable'
.\rumable.exe install-otel --config .\otel-iis.toml --state-dir $state --dry-run
.\rumable.exe install-otel --config .\otel-iis.toml --state-dir $state
.\rumable.exe render --config .\otel-iis.toml --state-dir $state
.\rumable.exe enable --config .\otel-iis.toml --state-dir $state
```

IIS 不需要 `attach`。静态文件、Classic ASP 和纯反向代理不属于 .NET Agent 覆盖。安装命令不改变池的 CLR、管线或归属；启用仍会检查这些条件。

## 离线包与已有依赖

已有由运维维护的模块或 Agent，可继续显式填写 `modulePath` / `agentPath` 并跳过 `install-otel`；工具不会采用、覆盖或卸载这些文件。

隔离环境、RPM Nginx 或自定义兼容发行包使用校验过的离线包，也可通过固定 HTTPS 地址下载。二选一，URL 不包含凭据、query 或 fragment，必须提供完整、小写 SHA-256：

```toml
[sites.otel.install]
archive = "/srv/packages/nginx-module-otel.deb"
# url = "https://packages.example.com/fixed/module.tar.gz"
sha256 = "替换为实际64位小写十六进制摘要"
```

Windows 本地路径使用 TOML 单引号，例如 `archive = 'C:\Packages\agent.zip'`。支持 ZIP、tar、tar.gz、Nginx `.deb`；`.deb` 需要 dpkg-deb。RPM 文件本身不支持解包，应在可信构建环境生成下表目录结构的 ZIP/tar.gz。包内链接、设备文件、路径穿越和重复条目会被拒绝；不运行包内安装脚本。

| 离线内容 | 约定 |
|---|---|
| Nginx | 包内恰好一个 `ngx_otel_module.so`，必须匹配目标 ABI；保留 LICENSE/copyright |
| IIS | 官方 Windows ZIP 的根目录布局：`win-x64`、`win-x86`、`net`、`netfx` 等，包含启动钩子及所有运行依赖 |

每次安装记录来源、SHA-256、平台和各文件摘要，保存为 `rumable-install.json`。依赖版本不会在后台更新；升级需新安装来源及重新规划挂接。已挂接的 Linux `modulePath` 固定，不能覆盖正在使用的模块。

## 停用与完整卸载

仅关闭 OTel，保留 RUM 和以后重新开启的能力：

```sh
rumable disable --component otel --state-dir /var/lib/rumable --site gateway
```

完整移除**指定站点的 RUM 和 OTel 部署**：

```sh
rumable uninstall --state-dir /var/lib/rumable --site gateway --dry-run
rumable uninstall --state-dir /var/lib/rumable --site gateway
# 同时删除自动安装且未修改、未被其他站点及回滚快照引用的依赖：
rumable uninstall --state-dir /var/lib/rumable --site gateway --remove-dependencies
```

Windows 对应 `.\rumable.exe uninstall --state-dir $state --site gateway`，其余参数相同。卸载默认保留依赖；也可以先卸载站点，之后再加 `--remove-dependencies` 清理。仅安装、尚未 render 时，也可用原配置和该选项移除依赖。`--yes` 用于已审查的非交互操作。

卸载会展示恢复差异，确认后重新校验状态、业务文件和 IIS 受管项，先停用两种采集，再恢复 Linux 首次挂接或 IIS 原规则/压缩/池环境，并移除本次创建的资源映射。共享 exporter 的从站点须先卸载；依赖清理检查同 state-dir 的其他当前及上次快照。清理前还检查生效配置及 IIS 池、机器/服务和 ASP.NET Core 环境的现场引用；状态丢失也不会删除仍被引用的依赖。新建/修改过的依赖文件会阻止清理。非受管模块、Agent、Collector、系统软件包、软件源和 CLI 本身都不删除。

必须先清理浏览器、代理、CDN 中的旧 HTML；卸载会移除专属资源映射。若 IIS 资源映射在首次启用之前已存在，卸载会恢复并保留它及原位资源目录，仅归档状态，避免破坏非本次创建的映射。其他情况下，状态、原始配置备份及版本化资源归档在 `<state-dir>/removed/<id>-<时间>/`，仍含私密配置，限制 ACL。卸载归档后普通 `rollback` 不再恢复该站点；需要重新部署时重新 render/attach/enable，人工审计可查归档。未完成的部署事务需先 `rollback`；挂接事务需先 `rollback --attachment`。卸载失败停留在已完成阶段，修复原因后重复同一卸载命令；不会为了恢复已卸载的采集而重新启用它。

依赖删除先改名隔离，再删除文件；Windows 文件仍被进程占用时可能需要稍后重试，或按输出清理隔离残留。只在确认站点已经停止依赖它后手动清理。不要直接删除仍被 `load_module`/池 profiler 引用的目录。

安装并启用后，按 [OTel 手册](otel.md#启停回滚和验收)发起业务请求，在对应应用中确认 Trace 数据。
