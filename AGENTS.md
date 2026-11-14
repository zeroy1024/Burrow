# AGENTS.md

本文件是给在本仓库工作的 AI 代理和自动化工具的约束。贡献流程见 [CONTRIBUTING.zh.md](CONTRIBUTING.zh.md)，构建与发布见 [docs/development.zh.md](docs/development.zh.md)。

## 强约束

- **版本与 CHANGELOG 必须同步**。修改 `mygo.json` 的 `version` 时，必须在同一提交中为该版本在 [CHANGELOG.md](CHANGELOG.md) 写入 `## <版本> - <日期>` 小节：先英文，再以 `**简体中文**` 分隔写对应中文，并把 `## Unreleased` 中已发布的条目移入。该小节就是应用内更新窗口展示的更新说明，缺失时发布构建失败。不得为了通过构建而留空或写占位内容。
- **用户可见的变更随改动写入 `## Unreleased`**，中英文各一条，描述行为而非实现。
- **推送前必须通过 E2E 门禁**。执行 `git push` 前，在待推送的提交上运行并通过下列检查；仅修改文档（`*.md`、`docs/`）的推送除外。统一入口为 `mise run gate`（[scripts/gate.py](scripts/gate.py)），它自行启动并移除本轮夹具；各层覆盖范围见[联调与原生 E2E](dev/ssh-integration/README.zh.md)与 [MCP 联调](dev/mcp-integration/README.zh.md)。
  - **每次推送**：[已有检查](docs/development.zh.md#已有检查)和 `mise run gate`（协议集成层）。被跳过的集成用例会使门禁失败。
  - **按路径追加原生 UI E2E**：推送包含 `internal/desktop/`、`internal/uikit/`、`third_party/mygo-terminal/`、`scripts/ssh-integration-ui.py`、`scripts/ssh_e2e_scenarios.py` 或 `scripts/ssh_e2e_coverage.py` 的改动时，运行 `mise run gate native`；包含 `internal/mcpserver/` 或 `scripts/mcp-integration-ui.py` 的改动时，运行 `mise run gate mcp`。推送 `v*` 版本标签前运行 `mise run gate all`。
  - `native` 与 `mcp` 层会占用键盘焦点，启动前先告知用户。不同层不得并行运行。
  - 环境不满足（无 Docker、锁屏、非 macOS 等）或检查失败时不得推送，也不得以修改跳过白名单、削弱断言或部分通过代替；说明阻塞与已运行范围，由用户决定。
- **不使用 UPX**。不得为任何平台的可执行文件或动态库重新引入 UPX 或其他可执行文件压缩器。
- **更新签名私钥不进仓库**。`MYGO_UPDATER_PRIVATE_KEY` 只存在于维护者的密钥管理和仓库 Secret 中；不得写入代码、配置、日志或提交记录，也不得更换 `mygo.json` 中的 `updates.publicKey`，否则已安装的应用将无法再更新。
