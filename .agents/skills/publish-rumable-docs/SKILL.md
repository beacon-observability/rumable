---
name: publish-rumable-docs
description: 将 Rumable 的安装、Nginx/IIS 自动注入、SDK 来源与加载方式、配置和升级维护说明同步到 dataflux-doc 的中文 Web RUM 文档，并提交面向 release 的 GitLab MR。用户要求发布或更新 Rumable 文档中心内容时使用；不用于发布 CLI、OSS 安装包或 Browser SDK npm 包。
---

# Publish Rumable Docs

把 Rumable 的用户使用说明发布到文档中心，保持项目源码、文档仓库和安装包发布三个流程独立。只要求创建或修改本 skill 时，不自动执行文档发布。

## 定位仓库与范围

- Rumable 仓库：从当前项目的 `git rev-parse --show-toplevel` 确认。
- 文档仓库：默认是 Rumable 同级的 `../dataflux-doc`，允许使用用户指定的路径；确认 origin 指向 `zy-docs/dataflux-doc`。不存在时先定位已知工作目录，仍找不到再询问，不静默克隆。
- 本流程仅修改 `docs/zh/real-user-monitoring/web/`。Rumable 专题位于 `rumable/` 子目录；根据实际变化维护该目录和上级 `.pages`，必要时增加 `app-access.md` 的入口。其他语言、Session Replay 专题和站点配置不在默认范围内。
- 阅读当前两仓库适用的 AGENTS.md、文档仓库 README、最近的 `.pages` 和邻近页面。
- 首次使用前阅读 [内容与品牌约定](references/authoring.md)。命令变量使用 `RUMABLE_REPO`、`DOCS_REPO`、`DOC_BRANCH`、`SKILL_DIR`，不改写 HOME 等系统变量。

## 核对真实能力与发布状态

依据源码、测试和项目内文档确定用户可观察的行为，不根据提交标题推断：

| 主题 | Rumable 中的核对入口 |
| --- | --- |
| 安装、目标与命令 | USER-GUIDE.md、internal/rumable/cli.go、setup.go、discovery.go、scripts/install.sh、scripts/install.ps1 |
| SDK 来源、同步/异步、默认值 | config.go、assets.go、render.go、probe.go、assets/iis-rum.ps1、loading_test.go |
| 配置页与版本能力 | web/index.html、web/app.mjs、web/generator.mjs、web/generator_test.mjs、scripts/release.go |
| SDK 版本与更新 | internal/rumable/assets/sdk.json、scripts/sync-sdk.go、scripts/release.go、scripts/ci/ci.mjs |
| 启停、恢复、卸载 | deploy.go、attach.go、uninstall.go、docs/operations.md、docs/windows-acceptance.md |

路径没有目录前缀时在 `internal/rumable/` 下查找；文件变化时按当前源码重新定位。

读取实际发布入口的 manifest，在 MR 描述或交付记录中记录 version、revision、capabilities 和 sdk 元数据，区分源码已实现、安装包已发布、页面已更新及目标机器已验收。不能仅凭 `Version` 常量声称功能已发布。读取失败时明确未确认，避免编造版本或可用下载链接。

用户指南采用长期有效的入口：配置页和清单使用 `latest`；安装器、包名、摘要由同一次读取的清单解析。不要在指南中固定某次 Rumable 发布的 URL、包文件名、CLI 路径或“当前版本为...”快照。功能前提按所需能力说明，发布实况写入 MR；普通版本迭代不要求修改指南。平台最低兼容版本、业务 version 示例和 SDK 的稳定主版本路径不属于 Rumable 发布链接。

页面的能力检查可能拒绝旧包：核对 `rum-sdk-loading-v1`、`rum-discovery-sdk-v1` 等实际字段。未发布能力可在文档中明确标注所需新包；若页面会被误读为已上线，创建 Draft MR 并在描述中列明发布依赖，而不是擅自发布安装包。

Rumable 使用现有 Browser SDK 不等于发布了新版 SDK。只改 Rumable 功能时，不把它写入 `web/sdk-changelog.md`；使用 Rumable 版本说明表达变化。

## 准备文档分支

检查文档工作区；保留已有改动，不 stash、丢弃或夹带。优先使用干净的工作区；需要隔离时按当前会话规则创建独立工作区。

对干净的文档仓库执行：

```sh
git -C "$DOCS_REPO" fetch origin release
git -C "$DOCS_REPO" switch release
git -C "$DOCS_REPO" merge --ff-only --no-stat origin/release
git -C "$DOCS_REPO" switch -c "codex/docs-rumable-$DOC_SLUG"
```

使用本次任务的简短主题作为 DOC_SLUG。从最新远端 release 起步，不直接修改或推送 release。若分支已存在，先核对是否属于本次任务；不覆盖其他任务分支。

## 编写和验证

按用户任务组织：接入与前提 → SDK 配置 → 验收 → 更新与恢复。避免复制内部发布日志、私有 Git 地址或源码绝对路径到用户文档。新增页面登记 `.pages`，使用 Markdown 标题锚点或仓库支持的 `{#anchor}`，不要插入裸 `<a id>` 标签。

运行本 skill 的校验器：

```sh
python3 "$SKILL_DIR/scripts/check_dataflux_doc.py" --repo "$DOCS_REPO" --base origin/release
git -C "$DOCS_REPO" diff --check
```

校验器检查发布分支基线、允许路径、导航、模板与代码围栏、品牌域名、Browser 包名分支以及 Rumable 的固定发布引用；它不证明命令可执行或功能已经发布。再核对相对链接、CLI 选项、配置示例、功能前提和页面渲染。

使用仓库现有 Markdown/spelling 工具（可用时）检查本次页面。对品牌条件分别渲染 Guance / TrueWatch 内容；对 TOML 示例使用实际 CLI 或解析器检查。可用相同 Markdown 扩展构建局部预览；区分局部预览和完整站点构建，不把未运行的检查写成通过。不要为绕过检查添加 blanket ignore。

## 交付到文档中心

用户要求“发布到文档中心”时，本流程包含文档提交、推送主题分支和创建面向 release 的 MR。若只要求本地文档或 skill，停在相应范围。项目内 skill 的提交遵循用户对 Rumable 仓库的授权，不混入文档提交。

确认只包含本次文档后提交，按以下方式推送并创建 MR：

```sh
git -C "$DOCS_REPO" push --set-upstream origin "$DOC_BRANCH" \
  -o merge_request.create \
  -o merge_request.target=release \
  -o merge_request.title="$MR_TITLE"
```

复核远端 SHA、MR URL 及 source/target，创建成功后调用可用的 attach_artifact 关联 MR。已存在 MR 时更新该 MR，不重复创建。只有用户明确授权合并该文档 MR 时才合并；发布文档不隐含发布 CLI、上传 OSS、打 release tag 或发送群通知。

最后报告 skill/文档路径、版本状态、验证结果、文档提交与 MR 链接；准确区分已准备、已推送、待合并和已上线。
