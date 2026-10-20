# 0.1.0-preview.3 本地交付校验记录

日期：2026-10-20（Asia/Singapore）。本次仅提交本地 Git，未配置远程或推送。

## 构建来源与下载

- 构建源码提交：`0ee5e1c0ae2b04bee60a44f9a42a0a9838e90093`；本地标签 `v0.1.0-preview.3` 指向该提交。
- 发布目录：`dist/0.1.0-preview.3/`，其中 README 是下载索引；汇总包：`dist/0.1.0-preview.3.zip`，旁附 `.sha256`。
- Windows/Linux 各提供 x86、x64、ARM64 现代包，以及 x86、x64 legacy 包，共 10 包；不提供 macOS 包。现代包 Go 1.26.9、legacy 包 Go 1.20.14，CGO=0。
- 本版包含之前的功能修复、`install-otel`/`uninstall`、Nginx/IIS 的独立 RUM/OTel 管理；已移除 Apache。模块/Agent 由安装命令另行下载或使用离线包，Collector 不随包安装。
- 在线同步校验的稳定 Web SDK 仍为 3.3.21；来源及 SHA-256 随每包 sdk.json 提供。通过 enable 升级站点；OTel 单组件操作保留已部署浏览器版本。
- 沿用 dist 的 Git 忽略规则，压缩包保留在本机；源码、构建脚本、最终用户文档和验证记录提交 Git。preview.1/.2 原包未覆盖，原 SHA256SUMS 全部复核通过。

## 分发校验

正式构建从干净的上述源码提交执行，两套编译器的全量 test/vet、SDK/发布契约测试通过；构建结束再次验证源码未变化。每个 build.json 均记录已提交的源码、版本、架构、编译器及二进制摘要，不标记 uncommitted。

10 个归档的 SHA256SUMS、ZIP CRC、PE/ELF 机器类型、Linux 执行位、包内文档链接均检查通过；文档、脚本、示例和许可逐字节等于构建源码中的来源。全部 10 个最终可执行文件与实际测试的 preview3-rc1 候选逐字节相同。汇总 ZIP 的 CRC、文件清单和各条目内容逐项核对通过。

SHA256SUMS 摘要：`6248531227704d57103cb7e3fad6fd64de53fa4963939b6ec1c32d4f0a97972a`。

汇总 ZIP 摘要：`7fbdf61c4ff6e336e2045e9ea7e41cd7c81c1b5692a006943bde6d8b2a66e66b`；大小 32,487,174 字节。

## 实际验证与边界

- 五个 Linux 包实际执行 version、Nginx/IIS 两种上报配置生成及两种 OTel 示例 render/status；x86/x64 使用 OrbStack 仿真，共享现代 Linux 内核。
- 现代 Linux x86 包对最低 Nginx 1.9.4 完整 RUM smoke 通过；legacy Linux x64 包对 1.9.4 完整自动挂接测试通过，覆盖压缩、缓存、继承、独立启停、回滚、确认期间并发修改拒绝、提交/重载失败恢复、精确撤销与备份保留。
- Linux ARM64 包对 Nginx 1.25.3/module 0.1.0 和 1.28.3/module 0.1.2 完整 OTel 脚本通过。使用真实模块、Collector 0.163.1 和私有 OTLP 接收器，覆盖 Span、DataKit gRPC/Dataway protobuf 分支、采样/路径/站点隔离、共享 exporter、独立启停、接收端停机期间的 RUM 操作、双文件挂接、部分恢复重试、回滚与卸载。
- 1.25.3 使用临时容器内 APT 仓库实际下载/校验/提取模块，重复安装和受管依赖清理通过；不代表各发行版远程软件源及 ABI 已验收。
- PowerShell 7.4 / Linux amd64：快照、IIS OTel 池契约 mock、脚本解析及真实 Linux x64 CLI 可移植测试通过。此结果不等于 Windows IIS API 验收。
- Windows 五个包完成交叉编译、PE 格式及构建元数据检查；Windows/IIS/.NET Agent 实机、旧系统内核、真实 DataKit/Dataway/观测云入库尚未验收。SDK/初始化和页面交互未改动，本轮未重复 Chrome 验收。

最终用户从[使用指南](../USER-GUIDE.md)开始；自动安装、离线包、停用与卸载见[安装与卸载手册](otel-installation.md)。完整步骤与本机日志见[验证记录](verification.md)，Windows 可执行步骤见[验收手册](windows-acceptance.md)。本记录在正式构建校验后另行提交，不改变构建源码与标签。

## 分包 SHA-256

| 文件 | SHA-256 |
|---|---|
| `rumable_0.1.0-preview.3_linux_386.tar.gz` | `0313307da5e5e50e717e9aa907f9432fc0c5b9599158cdc9dda0b6c479f6aae1` |
| `rumable_0.1.0-preview.3_linux_386_legacy.tar.gz` | `f73854046a9479d2cdf3dfb4d0bc63abb1a42d0485e29ebea653fa0db27f143d` |
| `rumable_0.1.0-preview.3_linux_amd64.tar.gz` | `82fbb964291b09a1588bdeb1fac9933a4f13b647fc3ceff9ade0a02938f7834b` |
| `rumable_0.1.0-preview.3_linux_amd64_legacy.tar.gz` | `f408b9cd1465b5cc2fd6375280b17c2926d170534b71cd171738b9d4b7c3a33a` |
| `rumable_0.1.0-preview.3_linux_arm64.tar.gz` | `9c7903162a19d546e9a88f5ebb4e02b8127dcc5a71bbabf727ce94c3c9d53032` |
| `rumable_0.1.0-preview.3_windows_386.zip` | `e173b356d5184cba14b5e7ba02c5cb728d9a6e0c414bbb90c6bd8b38ba3fa591` |
| `rumable_0.1.0-preview.3_windows_386_legacy.zip` | `81db9ec8596f6c5ddbea84d49aff6db761cfa6d9a93e9dd5549e8d48f5b12933` |
| `rumable_0.1.0-preview.3_windows_amd64.zip` | `a98437746281a462e8647fdafc4f8b3979df5fa19bf14ef37a2f9a4064fdf036` |
| `rumable_0.1.0-preview.3_windows_amd64_legacy.zip` | `32a7b457812d8bc73787c8d08ee0a26e318a245ca15767d1478af006ff7970b1` |
| `rumable_0.1.0-preview.3_windows_arm64.zip` | `c9ae62dc2673f9851306041ca8f1351b2ed567d731f4ad511bbd9aeb94e797ba` |
