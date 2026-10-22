# 开发、构建与发布验证

Windows 发布包只携带程序、文档、示例与许可证；IIS 验收脚本及 ASP.NET 夹具从同版本源码 tag 获取。在独立 Windows VM 以管理员运行；旧 Windows 选择 legacy 包，步骤和矩阵见 [Windows/IIS 验收手册](windows-acceptance.md)：

~~~powershell
.\scripts\windows-smoke.ps1 -Binary .\rumable.exe -ExpectedIISVersion 10.0 -ResultDir C:\RumableResults\iis10-01
~~~

覆盖三种压缩基线、规则继承、冲突、幂等启停、升级回滚和失败恢复，默认清理临时站点。退出码 0 / 1 / 2 分别表示通过 / 失败 / 部分验收；StaticOnly 不能算作完整通过。

```sh
go test ./...
go vet ./...
go build -o rumable ./cmd/rumable

# 已有 OrbStack Docker、Go、curl；不安装浏览器测试框架。
scripts/smoke-nginx.sh
NGINX_IMAGE=nginx:1.14-alpine scripts/smoke-nginx.sh
# 1.9.4 旧镜像格式不兼容当前 Docker，使用固定摘要的官方源码构建。
docker build -f scripts/nginx-1.9.4.Dockerfile -t rumable-nginx:1.9.4 .
NGINX_IMAGE=rumable-nginx:1.9.4 scripts/smoke-nginx.sh
```

`KEEP=1 scripts/smoke-nginx.sh` 保留本地验证站点，便于本机 Chrome 检查交互和事件；脚本打印需要清理的容器、网络和临时目录。接收端仅用于证明浏览器发送事件，不能替代观测云入库验收。

`RUMABLE_BINARY=/absolute/path/to/linux/rumable` 可替换脚本内默认构建的 CLI，用同一流程验收发布二进制；测试夹具仍需本机 Go 构建。二进制架构需能在所用 OrbStack 环境执行。

```sh
# 同份源码，两个真实编译器；仅设置 go.mod 不会让新 Go 支持旧 Windows。
# 默认先同步官方最新稳定 v3 SDK，再运行两套编译器的测试、vet 和打包。
go run scripts/release.go --go /path/to/go1.26.x/bin/go \
  --legacy-go /path/to/go1.20.14/bin/go --version 0.1.5
```

发布目录默认为 `dist/0.1.5/`，包括 10 个 Windows / Linux 包、两个安装器、配置页与版本元数据，共 19 个公开文件。每包携带二进制、示例、手册、SDK 元数据和许可证；目录中的 `SHA256SUMS` 校验压缩包、安装器、配置页及元数据。`build.json` 记录源码提交、架构、编译器与二进制摘要。IIS 及旧 Windows 的真实运行验证需在对应 Windows 主机进行，交叉编译不代表运行验收。

可单独执行 `go run scripts/sync-sdk.go` 同步并审查 SDK、版本、来源和 SHA-256；CDN 与官方 NPM 稳定版不一致、版本不是稳定 v3 或许可变更时中止。离线重建可显式使用 `release.go --skip-sdk-sync`，其含义是沿用已校验文件，不检查最新版本。站点不会自动更新；分发新二进制后执行 `enable` 升级，默认同域方式保留旧资源供缓存页面和回滚使用。

正式构建自动记录本地 Git HEAD，并要求工作区、暂存区和未跟踪源码均干净；同步更新 SDK 后应先审查、提交，再重新构建，避免将新源码标为旧提交。尚未提交的候选验证可显式使用 `--revision uncommitted`，其 build.json 会如实标记；分发包应使用已提交的源码。发布目录必须不存在；脚本拒绝覆盖旧包，构建完成前再次核对源码未变化。dist 按现有仓库规则不进入 Git，源码和发布验证记录提交到本地仓库。

## 验证同步与异步加载

```sh
node --test web/generator_test.mjs
go test ./...
# 同域与官方 CDN：模式切换、重复启用、回滚、停用及两种压缩响应。
CDN=1 scripts/smoke-attach.sh
# 留下仅用于本地的站点，在浏览器打开 http://127.0.0.1:19180/loading。
KEEP=1 CDN=1 SYNC=1 scripts/smoke-attach.sh
```

`/loading` 在首个业务脚本中记录 SDK 状态，同步模式预期显示 initialized。结束后按脚本输出清理专用容器、网络和临时目录。IIS 的规则、恢复和安装器契约可用 `scripts/test-powershell.ps1` 验证；原生 IIS 的最终验收仍需 Windows 主机。
