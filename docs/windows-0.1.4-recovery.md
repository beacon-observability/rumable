# 用 0.1.5 恢复 Windows 0.1.4 失败现场并复测

日期：2026-10-22。0.1.4 实机报告已确认下载、摘要、架构识别和发现成功，但 IIS 注入与恢复失败。修复版本为 **0.1.5**；旧 0.1.4 文件保持不变。本文适用于用 0.1.5 正式包接续已保留的测试现场。不要直接在未完成事务上重复一键安装，应先执行下面的恢复流程。

## 故障与修复范围

1. `ConfigurationElementCollection.Add` 返回元素，原桥接输出在 JSON 前混入 `Attributes ...`，Go 解析失败时 IIS 已提交。现在显式丢弃所有相关返回值，且标准错误不并入 JSON。
2. JSON null 排除列表在 PowerShell 中形成空正则分支 `^(?:|/__rumable__/)`，配合 negate 排除了全部页面。现在仅为非空路径构造条件，并保留 SDK 保留路径排除。
3. 旧事务的 `nativeCommitted=false`、after 无 native 时，rollback 会基于日志中的原目标、脚本 URL、路径和原始压缩配置重建可能的精确结果，兼容上述 0.1.4 错误规则及已恢复但保留映射的状态。通过后先保存已核对快照，再恢复。额外配置改动、未知 OTel 或卸载事务仍拒绝自动接管。
4. Windows CLI 检查标准输入控制台属性；安装器将重定向输入/非交互主机识别传给 CLI，避免 CONIN$ 导致隐藏等待。显式 `--non-interactive` / `-NonInteractive` 可固定自动化行为。
5. 私有状态、事务、桥接请求和临时写入文件用受保护 Administrators/SYSTEM DACL。新站点目录不继承宽泛读取权限；已有目录不做整体重置，仅收紧已知私有文件。公开资源查询实际匿名身份和应用池身份，仅授权 public 子树读取、父目录遍历，拒绝 public 重解析点。停用/回滚保留公开资源权限供缓存页面使用。

控制台句柄行为参考 [Microsoft Console Handles](https://learn.microsoft.com/en-us/windows/console/console-handles)：即使标准输入已重定向也能单独打开 CONIN$，所以“能打开 CONIN$”不等于可交互。

## 先恢复保留现场

以管理员运行 **64 位 Windows PowerShell 5.1**。现代 x64 主机下载 [0.1.5 Windows x64 ZIP](https://static.guance.com/rumable/0.1.5/rumable_0.1.5_windows_amd64.zip) 和 [SHA256SUMS](https://static.guance.com/rumable/0.1.5/SHA256SUMS)，按[手动安装步骤](../USER-GUIDE.md#5-手动下载校验并安装-cli)校验、解压到新目录；其他系统按指南选对应包。保留旧 0.1.4 程序以便比对。不要覆盖 Program Files 中的原版本，不重新 render，不删除 transaction.json、state.json 或 operation.lock。

将下面的 `$cli`、`$id` 替换为本次 0.1.5 包路径及原报告的接入 ID；`$state` 必须是原状态目录。先把该站点的 state.json、transaction.json 和原测试报告复制到仅管理员可读的证据目录。

```powershell
$cli = 'C:\Rumable\0.1.5\rumable.exe'
$state = 'C:\ProgramData\Rumable'
$id = '替换为原 auto-接入标识'
& $cli version
if ($LASTEXITCODE -ne 0) { throw '0.1.5 程序不能运行' }
& $cli status --state-dir $state --site $id
& $cli rollback --state-dir $state --site $id
if ($LASTEXITCODE -ne 0) { throw '恢复未完成，保留现场并提交原始错误' }
& $cli status --state-dir $state --site $id
& $cli probe --state-dir $state --site $id
if ($LASTEXITCODE -ne 0) { throw '恢复后的 HTTP 验证未通过' }
```

预期 `rumEnabled=false`、`pendingTransaction=false`、drift 为空，目标页面无注入，原始压缩属性恢复；保留的 `/__rumable__` 映射可供旧缓存使用。不要使用旧二进制运行新的启用动作。若报“exact pending RUM operation”不一致，记录当前受管规则、映射和压缩属性供人工核对，不通过删日志强行绕过。

## 原目标重新接入与生命周期

恢复通过后，打开 [0.1.5 配置页](https://static.guance.com/rumable/0.1.5/setup/index.html)或 [latest 入口](https://static.guance.com/rumable/latest/setup/index.html)，填写原测试 RUM 参数并选择 Windows，复制一键命令执行。保留原状态目录、选择原目标，核对接入 ID 未变。

也可以使用原测试配置。若是完整 `[[sites]]` TOML，运行下列命令；若是页面自动发现 `server + [rum]` TOML，使用 `setup --discover --config ... --state-dir ... --yes`，在交互终端选择原站点，不给该命令传 --site。

```powershell
$config = 'D:\测试证据目录\原完整配置.toml'
& $cli enable --config $config --state-dir $state --site $id
if ($LASTEXITCODE -ne 0) { throw '启用失败' }
& $cli status --state-dir $state --site $id
& $cli probe --state-dir $state --site $id
```

- 首页和第二个 HTML 页面各一条注入；初始化脚本和 SDK HTTP 200、摘要正确；浏览器自动运行 SDK。虚拟凭据只验实际请求，不能声明真实应用入库。
- 未选中站点的配置及 HTTP 正文不变；JSON、下载和排除路径保持原文。
- 重复同配置安装不新增规则，ID 保持；修改 env 后启用成功，再 rollback 恢复上一配置；disable 后无注入，随后重新 enable 成功。每步检查 pendingTransaction=false 和 drift 为空。
- 检查 state.json/transaction.json 的 ACL 无 IUSR、IIS_IUSRS 或 Everyone 读取权限；公开 SDK 在默认 IUSR、匿名用户为空且使用池身份这两种配置分别验收。自定义身份需单独复测。
- 至少两个候选站点时，用输入重定向启动以及显式 --non-interactive 各测一次，要求快速非零退出，配置/状态不变；正常交互仍可选择 A，B 不变。唯一目标且无需补 URL/压缩许可时可无交互成功。

## 新建隔离站点的自动测试

没有 Git 权限的用户直接按上一节和[Windows 验收手册](windows-acceptance.md)执行，OSS 包包含 CLI、文档与配置示例，不包含源码或测试脚本。

开发者有仓库权限时，从 `release-oss-v0.1.5` 获取源码和测试脚本，指定已校验的正式 CLI。脚本另开一组随机命名、仅绑定回环地址的测试站点，不影响旧现场：

```powershell
.\scripts\windows-smoke.ps1 -Binary 'C:\Rumable\0.1.5\rumable.exe' -StaticOnly -Keep
```

StaticOnly 完成后退出码 **2** 表示静态范围通过、动态范围未测；不应误报完整通过。装有 ASP.NET 4.x 时去掉 StaticOnly 覆盖动态响应。Keep 保留站点供浏览器验收，清理按仓库 scripts/windows-cleanup.ps1 流程。IIS 与 URL Rewrite 必须已安装，脚本不执行 iisreset。

原生权限与重定向输入单元检查（仓库同版本源码、Go 1.26.x、管理员终端）：

```powershell
go test ./internal/rumable -run 'TestWindowsPrivateFilesAndLegacyPublicACL|TestDiscoveryRejectsRedirectedWindowsInput' -v
if ($LASTEXITCODE -ne 0) { throw '原生 Windows 检查失败' }
```

本机已完成 Go/Node、PowerShell/Linux 模拟边界与 Linux Nginx 验证；以上 NTFS、Windows PowerShell 5.1 和 IIS 实机步骤必须回到 Windows 执行，不能用交叉编译通过代替。
