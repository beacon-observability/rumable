# 0.1.0-preview.2 本地交付校验记录

此页仅记录已发布压缩包的历史内容和校验结果；当前源码及使用方式以 README 和当前手册为准，旧包内使用方式以随包文档为准。

日期：2026-10-20（Asia/Singapore）。本次只提交本地 Git，未配置远程仓库或推送。

## 构建来源与文件

- 构建源码提交：`ec99b898c5ae177f8ce10a45b3036139321a0e30`；本地标签 `v0.1.0-preview.2` 指向此提交。
- 分发目录：`dist/0.1.0-preview.2/`；汇总包：`dist/0.1.0-preview.2.zip`，旁附 `.sha256`。
- Windows/Linux 各提供 x86、x64、ARM64 现代包及 x86、x64 legacy 包，共 10 包。现代包 Go 1.26.9，legacy 包 Go 1.20.14，CGO=0。
- 发布前同步官方稳定 v3 SDK，内置 3.3.21；各包记录 SDK 来源和摘要。原生 OTel 模块、.NET Agent、Collector 需另行安装。
- 沿用 `.gitignore`，dist 保留本机生成，不作为 Git 二进制历史提交；源码、构建脚本、最终用户文档、验收记录均已提交。旧 preview.1 产物保留。

每包 `build.json` 已检查版本、源码提交、编译器、目标架构与可执行文件 SHA-256；PE/ELF 机器类型、ZIP CRC、Linux 执行位、文档链接和许可齐全。全部 10 个最终可执行文件与 `preview2-rc3` 候选逐字节相同；包内指南、示例和脚本逐字节等于构建提交中的来源。归档中不标记未提交源码。

SHA256SUMS 摘要：`6d0c11534cf46b6769e135aff686169d0f0e6ab4a5978ec2b30895edba0f11e1`。

汇总 ZIP 摘要：`4ef12b284595f96c3655d16f48eef7bdd2309196a5788f7aa00aa22eaef47f73`。汇总 ZIP 的 CRC 和文件清单检查通过。

## 实际运行与边界

- OrbStack：最终相同程序在 Nginx 1.25.3/1.28.3 与 Apache 2.4.69 完整 OTel 流程通过，使用真实原生模块、Collector 和私有 OTLP 接收器；覆盖 DataKit gRPC、Dataway HTTP/protobuf/Token、站点/路径隔离、采样、独立启停、重载失败恢复和完整撤销。
- 最低 RUM 回归：Linux x86 对 Nginx 1.9.4、Linux x64 对 Apache 2.4.23 完整自动挂接流程通过，覆盖压缩、缓存、继承、确认期间变更拒绝、启停回滚和备份。
- 五个 Linux 包均实际执行 version、完整 RUM 配置生成及三种 OTel 示例 render/status；x86/x64 使用 OrbStack 仿真，共享现代内核。
- 两套编译器全量 test/vet、SDK/发布契约测试通过，Go 1.26.9 race 通过。IIS PowerShell mock、快照和解析通过。
- Windows 包完成交叉编译和格式检查；Windows/IIS/.NET Agent 实机、旧系统内核、原生 TLS 和真实 DataKit/Dataway/观测云入库仍未验收。私有接收器成功不是观测云入库证据。本轮未改变 Web SDK/初始化逻辑，未重跑 Chrome。

修复详情、复验命令及本机日志见[验证记录](verification.md)。用户从[使用指南](../USER-GUIDE.md)开始；OTel 依赖安装、Dataway 配置、迁移、独立启停和回滚见[OTel 手册](otel.md)。本记录在发行构建校验后另行提交，不改变上述构建源码与标签。

## 分包 SHA-256

| 文件 | SHA-256 |
|---|---|
| `rumable_0.1.0-preview.2_linux_386_legacy.tar.gz` | `021bc7618daa07abbe9e40228372634fa90898889ee6de0365084d575e07abef` |
| `rumable_0.1.0-preview.2_linux_amd64_legacy.tar.gz` | `074fd8a660bb3f92b308e7f4e9aa7d2e15f6e237c62da8ad285b9cbb73f6623a` |
| `rumable_0.1.0-preview.2_linux_amd64.tar.gz` | `3cc3a25a3267bdb3a39dbb12a5cd21983cd44c41b46fd69ba837d3b6746bf330` |
| `rumable_0.1.0-preview.2_linux_386.tar.gz` | `420cd5fbf914c764f32ef4bad3977c65648260413ab0b4723c40507d853c90d1` |
| `rumable_0.1.0-preview.2_linux_arm64.tar.gz` | `43fa2b71e89e4e0486d6728cd01ad2050cfe57bc3ebd7817645a64edcd1552d2` |
| `rumable_0.1.0-preview.2_windows_386.zip` | `755e3c49e01e27a1d4c3c83785e41bb28a51caf8c83d98bf431c447f74394b8f` |
| `rumable_0.1.0-preview.2_windows_arm64.zip` | `82fe2f5d3e9e3d6713a0b76ba8074b36c2c72e379798b8faf61bcb7ce2cf5604` |
| `rumable_0.1.0-preview.2_windows_amd64_legacy.zip` | `8b92ae219dcebdde6950a0471387eda1596eb03f6b250cd8b14ab45adb511a1a` |
| `rumable_0.1.0-preview.2_windows_amd64.zip` | `b446608602fbd6cf4d288151ae48da20ee06aef7d3013b03a8de318d7951229a` |
| `rumable_0.1.0-preview.2_windows_386_legacy.zip` | `e37bee4bffad666ec08729d2c5b080d0c0323ebb0e795fc9bce7d6a6a2d28fbf` |
