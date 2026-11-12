# AGENTS.md

本文件是给在本仓库工作的 AI 代理和自动化工具的约束。贡献流程见 [CONTRIBUTING.zh.md](CONTRIBUTING.zh.md)，构建与发布见 [docs/development.zh.md](docs/development.zh.md)。

## 强约束

- **版本与 CHANGELOG 必须同步**。修改 `mygo.json` 的 `version` 时，必须在同一提交中为该版本在 [CHANGELOG.md](CHANGELOG.md) 写入 `## <版本> - <日期>` 小节：先英文，再以 `**简体中文**` 分隔写对应中文，并把 `## Unreleased` 中已发布的条目移入。该小节就是应用内更新窗口展示的更新说明，缺失时发布构建失败。不得为了通过构建而留空或写占位内容。
- **用户可见的变更随改动写入 `## Unreleased`**，中英文各一条，描述行为而非实现。
- **不使用 UPX**。不得为任何平台的可执行文件或动态库重新引入 UPX 或其他可执行文件压缩器。
- **更新签名私钥不进仓库**。`MYGO_UPDATER_PRIVATE_KEY` 只存在于维护者的密钥管理和仓库 Secret 中；不得写入代码、配置、日志或提交记录，也不得更换 `mygo.json` 中的 `updates.publicKey`，否则已安装的应用将无法再更新。
