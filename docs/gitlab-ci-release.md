# GitLab CI / OSS 发布

沿用 browser-sdk 的发布方式：受保护的发布 tag 触发、`cloudcare-tools` Shell Runner、Docker 内构建、`ali-oss` 上传、`OSS_*` 环境变量、resource group 串行发布及不含凭据的结果报告。此流程只发布 Rumable 安装脚本、安装包和静态配置页，不发布 npm 包。

## 首次配置

在 GitLab Settings → CI/CD → Variables 配置以下变量，Environment scope 为 `*`，关闭变量引用展开。密钥使用 Masked 和 Protected；实际值不进入源码、命令参数或报告。

| 变量 | 用途 |
|---|---|
| `OSS_REGION` | 地域，例如 `oss-cn-hangzhou` |
| `OSS_BUCKET` | 存放安装包和页面的 bucket |
| `OSS_ACCESS_KEY_ID` / `OSS_ACCESS_KEY_SECRET` | 发布账号，只需目标 `rumable/*` 的 GetObject / PutObject 权限 |
| `OSS_STS_TOKEN` | 可选，配套的 STS 临时凭据 |

CI 不配置公开下载域名，只按固定 object key 上传。页面由所选 manifest 的 HTTPS 地址推导下载目录，并通过 `--base-url` 传给安装脚本；同一套产物可使用不同 OSS/CDN 域名。

配置 bucket/CDN 对 `rumable/*` 的匿名读取和正确 MIME 类型；跨域使用配置页时允许其域名 GET 清单。脚本不自动修改 bucket ACL、CORS、域名或 CDN。上传后的 SHA-256 回读使用 OSS API，不能代替公开域名/CDN 的下载验收。

在 Settings → Repository → Protected tags 保护 `release-*`，仅允许发布负责人创建。启用 `cloudcare-tools` Runner，安装 Docker，并允许访问镜像仓库、Go 模块代理、npm registry 和 OSS。与 browser-sdk 相同，不使用 Shell executor 会忽略的 GitLab `image` 字段。

构建镜像包含 Go 1.26.1、Go 1.20.14 和 Node 22。可通过 `MODERN_GO_IMAGE`、`LEGACY_GO_IMAGE`、`RELEASE_NODE_IMAGE` 指向内部镜像或固定 digest；发布脚本仍校验编译器分别为 1.26.x 和 1.20.14。Node 仅供 CI 和本地页面测试，最终用户无需安装 Node/Go。

Runner 的 Go 模块下载默认使用 `GOPROXY=https://goproxy.cn,direct`，保留 `GOSUMDB=sum.golang.org` 校验；可用同名 CI 变量覆盖，两项会显式传入构建容器。不要通过关闭摘要校验绕过网络故障。

## 版本准备与触发

发布脚本在 Rumable 源码 Git 仓库中运行；Windows / Linux 分发包用于安装验收，不含 Git 元数据。

先更新并审核源码、`internal/rumable/config.go` 的 `Version` 和版本说明；如需升级内置 SDK，在准备阶段运行 `go run scripts/sync-sdk.go`，将 SDK 与元数据一并提交。CI 验证其摘要，使用 tag 中已提交的 SDK，不在重试时下载变化的 latest。

将待发布改动合入 `main` 并推送，确认工作区干净、`main` 与 `origin/main` 一致，再使用仓库发布脚本创建与源码 Version 相同的发布 tag。当前版本为 `0.1.2`：

```sh
git switch main
git pull --ff-only origin main

# 只检查版本、工作区和远端 tag，不创建或推送 tag
bash scripts/release-oss.sh 0.1.2 --dry-run

# 创建附注 tag 并推送，触发 GitLab 的 OSS 发布流水线
bash scripts/release-oss.sh 0.1.2
```

省略版本号时读取源码 Version，默认远端为 `origin`，可用 `--remote NAME` 指定已配置的远端。脚本检查实际 push URL，只推送当前提交的一个发布 tag，不提交代码或推送分支，不覆盖不同提交的同名 tag。推送失败保留本地 tag，修复原因后可重跑；远端已存在且提交相同时只报告现状，不会再次触发流水线。远端 tag 回读通过不等于 OSS 发布成功，仍需查看 GitLab 流水线及发布报告。

普通分支、普通 `v...` tag、未保护的 tag 均不发布；不需要 GitLab API token。脚本不会配置受保护 tag 规则；首次使用必须完成前述 GitLab 设置。CI 不修改版本、不提交代码或推送分支。正式版本全部验证成功后更新 latest，预发布版本只发布自身版本目录。

## 执行顺序

1. 核对受保护 tag、源码 Version、commit、OSS 变量。
2. 按独立 lockfile 安装发布依赖，运行页面/发布脚本回归。
3. 使用两套 Go 编译器分别运行全量 test/vet、SDK 同步和归档测试，生成全部 10 个平台包。
4. 生成同版本 `install.sh`、`install.ps1`、`setup/` 配置页、manifest、SDK 元数据和 SHA256SUMS。公开目录只允许 10 个平台包、2 个安装器、4 个页面文件及 3 个元数据/校验文件，共 19 个对象；上传前拒绝任何额外文件，即使其摘要正确。使用文档、配置示例和许可证只放在平台包内。
5. 预读 OSS 全部同名对象：相同字节可重试，任何不同字节均停止，不覆盖旧版本。
6. 逐项上传并 GET 回读校验 SHA-256；全部通过后才上传和验证 `manifest.json`。报告写入脚本、页面和清单的 OSS object key。
7. 正式版本更新 `latest/setup/index.html`、两个安装器，再更新 `latest/manifest.json`，逐项回读校验。latest 使用 `Cache-Control: no-store, max-age=0`；固定版本仍使用长期不可变缓存。

当前目录为 `rumable/0.1.2/...`（版本号不带 preview 后缀），后续使用 `rumable/<version>/...`，配置页是 `rumable/<version>/setup/index.html`，清单是 `rumable/<version>/manifest.json`。脚本路径为同目录的 `install.sh` 和 `install.ps1`，内置本次构建版本及包摘要，不包含下载域名。配置页会预填同版本清单地址；用户读取版本、填写 Web SDK 参数后即可复制接入命令。

发布完成后的公开入口：

- 配置页：`https://static.guance.com/rumable/0.1.2/setup/index.html`
- 清单：`https://static.guance.com/rumable/0.1.2/manifest.json`
- Linux / Windows 安装器：`https://static.guance.com/rumable/0.1.2/install.sh`、`https://static.guance.com/rumable/0.1.2/install.ps1`
- Windows x64：`https://static.guance.com/rumable/0.1.2/rumable_0.1.2_windows_amd64.zip`

产品集成使用下面四个固定地址：

| 固定入口 | 用途 |
|---|---|
| `https://static.guance.com/rumable/latest/setup/index.html` | 跳转到最新正式版配置页 |
| `https://static.guance.com/rumable/latest/manifest.json` | 读取版本、包摘要、`artifactBase` |
| `https://static.guance.com/rumable/latest/install.sh` | Linux 安装入口 |
| `https://static.guance.com/rumable/latest/install.ps1` | Windows 安装入口 |

清单的 `artifactBase` 为 `../<version>/`，通过 `new URL(manifest.artifactBase, manifestURL)` 得到版本目录，再解析包或安装器的 `file`。页面即使读取 latest 清单，也生成固定版本命令。直接使用 latest 安装器时，将 base URL 设为 `https://static.guance.com/rumable/latest/`；脚本会按自身固定版本转换到对应目录，避免旧脚本与新包混用。

latest 只允许版本号向前推进，比较数字版本号；旧 tag 重试发现更高版本时会跳过 latest。同版本但清单内容不同会拒绝。latest 多对象更新不是事务，但每个页面/安装器都指向已经完整发布的固定版本；失败时可重试，清单最后更新。CDN 应遵守 latest 的不缓存响应头，不对该目录配置强制长期缓存。

公开域名仅用于文档与接入入口，不成为 CI 变量。发布后同时核对清单中的 revision 与 tag 提交、公开下载的 SHA-256 与 SHA256SUMS；本地旧测试包的 revision 为 `uncommitted` 时，不要用它替代 CI 的正式构建产物。

上传设置 `x-oss-forbid-overwrite`，同时由 `rumable-oss-release` resource group 串行执行。该头在已开启/暂停版本控制的 bucket 上无效，参见 [OSS PutObject 说明](https://www.alibabacloud.com/help/zh/oss/developer-reference/putobject)；因此同一 `rumable/*` 前缀须仅由这套流水线写入，禁止其他发布者并发修改。脚本仍会预读拒绝不同内容并回读核对，不会修改 bucket 的版本控制设置。

## 分发范围

从 0.1.2 开始，平台包只包含可执行文件、build.json、SDK 元数据、使用文档、配置示例和第三方许可证。Go 源码、测试夹具、验收脚本、CI 脚本和 Git 元数据均不进入平台包，也不单独上传 OSS。构建脚本和上传器分别使用明确的文件清单，避免将整个仓库或测试目录作为发布目录。

自动验收使用仓库中与安装包相同的 `release-oss-v<version>` tag，按 [Windows 验收手册](windows-acceptance.md)指定已下载的二进制。安装器和配置页的 JS 是运行必需的公开文件。已有 0.1.1 历史包不覆盖，修正通过 0.1.2 发布，latest 指向新版本。

## 重试与验收

上传不是多文件事务。失败时，报告记录已验证对象，可能已有部分文件存在；新 manifest 不会在前序对象未通过校验时发布。修复网络或变量后，Retry 同一个 tag 的失败 job，已存在的相同文件被复用。需要改源码或编译器时准备新版本；不要强推/复用发布 tag，不要覆盖已有版本目录。

报告 `release-report.json` 和 `dist/` 作为 GitLab artifacts 保留 30 天，失败也保留。`status=verified` 表示完整文件集已在 OSS API 回读确认；还须从公开地址访问配置页、读取清单、下载核对包，并在目标服务器验收接入、停用和回滚。Windows/IIS 和真实观测云事件入库需对应环境验证。

推送 `main` 本身不发布。创建受保护发布 tag 才会启动发布流水线；Windows/IIS 验收另按 [Windows 测试手册](windows-acceptance.md)执行，不能由 CI 成功推断 Windows 已通过。

本地发布逻辑测试不需要真实凭据：

```sh
node --test scripts/ci/ci_test.mjs web/generator_test.mjs scripts/installer_test.mjs scripts/release-oss_test.mjs
go test scripts/release.go scripts/release_test.go
```
