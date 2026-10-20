# 版本说明

## 0.1.0-preview.2 · 2026-10-20

- 增加 Linux Nginx、Linux Apache、IIS/.NET 的站点级 OTel 配置，RUM/OTel 独立开关和组件启停。
- 支持 DataKit OTLP/gRPC；Dataway 使用 Linux Collector 转换或 IIS/.NET 直接 OTLP/HTTP，Token 保留在服务器侧。
- 复用差异确认、挂接备份和部署回滚，增加 Linux 配置生效标记，连续确认配置代次，避免平滑重载的新旧 worker 交接窗口中提前返回。
- Apache 要求包含请求隔离修复的模块；IIS 要求 IIS 10 和独占应用池，只在环境值真正变化时回收目标池。
- 撤销挂接释放共享 exporter，未完成恢复阻止新部署；构建拒绝覆盖已有发布目录，并在完成前复核源码提交。
- 使用说明和实际验收边界见 [OTel 手册](docs/otel.md)及[验证记录](docs/verification.md)。本版提供 Windows/Linux x86、x64、ARM64 现代包及适用的 legacy 包；不包含原生模块、.NET Agent 或 Collector。

## 0.1.0-preview.1 · 2026-10-20

本地 preview 发布，提供 Windows / Linux 独立可执行文件、TOML 示例、使用指南、SDK 来源/摘要和第三方许可。未发布到远程 GitLab，不包含 macOS 发布包。

- 支持 Nginx 1.9.4+、Linux Apache 2.4.23+ 和 IIS 7.5+ 的完整 HTML 注入。
- Linux 首次挂接支持站点识别、差异预览、确认、备份、故障恢复和撤销；启停与部署回滚有受管文件摘要和外部变更保护。
- 站点使用 TOML 配置 OpenWay / DataKit、采样、采集、隐私、会话与回放；Session Replay 默认关闭。
- SDK 默认同域，亦可按站点配置 CDN、固定 URL 和可选 SRI。发布构建前同步并校验官方稳定 v3 SDK；当前内置 3.3.21。
- 提供 Windows x86/x64/ARM64、Linux x86/x64/ARM64；旧 Windows 和旧 x86 Linux 提供 Go 1.20.14 legacy 包。
- Windows x86 CLI 在 64 位系统通过 Sysnative 调用 64 位 PowerShell；真正 32 位 Windows 不支持 IIS 配置管理。

Linux Nginx / Apache 与本机 Chrome 已有运行记录；Windows/IIS、旧内核和真实观测云入库仍须目标环境验收。支持范围与实际证据分别见[兼容性矩阵](docs/compatibility.md)和[验证记录](docs/verification.md)。Preview 不支持任意服务器配置、HTML 片段或流式 SSR，不自动放宽 CSP。

旧部署升级：保留原 TOML、站点 ID、服务器目标与状态目录；换用对应平台的新可执行文件后执行 enable。普通 rollback 恢复上一成功部署，不会恢复已被外部 CDN 覆盖的文件。
