<p align="center">
  <img src="assets/icon.png" width="88" alt="Burrow 应用图标">
</p>

<h1 align="center">Burrow</h1>

<p align="center"><strong>终端、文件和服务器状态，一个原生工作区。</strong></p>
<p align="center">面向日常远程开发与运维的 SSH / SFTP / FTP / FTPS 桌面应用。</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-0A84FF" alt="MIT 许可证"></a>
  <img src="https://img.shields.io/badge/status-developer_preview-64748B" alt="开发预览阶段">
  <img src="https://img.shields.io/badge/macOS-13%2B-64748B" alt="macOS 13 及以上">
  <a href="https://linux.do"><img src="https://img.shields.io/badge/community-LINUX%20DO-F0B90B" alt="LINUX DO 社区"></a>
</p>

<p align="center">
  <a href="#主要功能">主要功能</a> ·
  <a href="#界面预览">界面预览</a> ·
  <a href="#获取与运行">获取与运行</a> ·
  <a href="docs/README.md">使用文档</a> ·
  <a href="CONTRIBUTING.md">参与贡献</a>
</p>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/readme/ssh-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="docs/screenshots/readme/ssh-light.png">
  <img src="docs/screenshots/readme/ssh-light.png" alt="Burrow 的 SSH 分屏工作区：主机目录、两个终端窗格和服务器状态面板" width="1424">
</picture>

<p align="center"><sub>同一工作区内整理连接、并排操作终端，并查看当前主机的状态。</sub></p>

## 主要功能

- **SSH 工作区**：多标签、组合分屏、窗格移动与放大；同一会话分屏复用 SSH 连接，每个 Shell 独立运行。支持密码、普通与加密私钥、SSH agent 和跳板连接。
- **双面板文件管理**：在本地与远程目录之间浏览和传输，支持 SFTP、FTP、显式与隐式 FTPS，以及搜索、新建、重命名、删除和重名处理。
- **可控的传输队列**：文件与目录上传下载、并发任务、暂停、继续、取消和重试；任务按文件标签隔离，并支持退出后的离线恢复。
- **服务器状态**：查看 Linux 主机的 CPU、内存、网络、进程和基础信息；服务器具备相应 SFTP 能力时显示根盘容量。
- **本机 MCP 服务**：通过 Streamable HTTP 向可信客户端开放授权范围内的 SSH 和文件操作，提供密钥、连接与能力授权、拦截策略及操作审计。
- **按习惯调整**：明暗外观、终端配色、字体、光标和快捷键；密码、托管私钥与已保存私钥口令由 Vault 加密保存。

界面使用 Go 与 [MyGo](https://github.com/egoist/mygo) 构建，终端由 [Ghostty VT](https://github.com/ghostty-org/ghostty) 内核驱动。

## 界面预览

截图来自当前应用的原生 UI，使用虚构主机、文件、指标和任务数据，仅用于界面展示。明暗 SSH 主图随 GitHub 页面主题切换；截图来源与复现方式见[展示图说明](docs/screenshots/readme/README.md)。

### 文件管理与传输

本地与远程目录并排显示，传输队列集中呈现进度和状态。

![Burrow 文件传输：本地与远程双面板，以及运行中、已暂停和已完成的示例任务](docs/screenshots/readme/files.png)

### MCP 授权与审计

让可信客户端使用已有连接，并在应用内查看操作记录、结果与策略拦截。

![Burrow MCP 页面：本机服务状态、授权密钥、拦截策略和操作审计](docs/screenshots/readme/mcp.png)

## 获取与运行

Burrow 当前处于 **0.x 开发预览阶段**。安装包与版本说明见 [GitHub Releases](https://github.com/zeroy1024/Burrow/releases)，也可按下面的步骤从源码运行。

| 平台 | 当前状态 |
|---|---|
| macOS Apple Silicon | 主要开发平台，系统要求 macOS 13+；提供 APP / DMG / ZIP 构建入口 |
| Windows x64 / ARM64 | 已接入跨平台编译与安装包构建；原生启动、安装、升级和卸载仍待验收 |
| macOS Intel / Linux | 保留构建目标，运行与桌面集成尚未完整验证 |

完整验证范围见[平台支持](docs/platform-support.md)。当前 macOS 发布签名为自签名，不提供 Apple 公证；说明见[签名与解锁](docs/macos-signing.md)。

### 从源码运行

先安装 [mise](https://mise.jdx.dev/)，再在仓库根目录运行：

```sh
mise install
mise exec -- go run .
```

首次运行可能下载终端所需的原生库。开发版使用独立的数据目录；新建 SSH 会话后选择「保存并连接」，首次连接时核对主机指纹。具体操作见[首次使用](docs/getting-started.md)。

### 开发与打包

工具链由 `mise.toml` 固定。测试、跨平台编译、正式签名和 `mise run build` 的要求见[开发与构建](docs/development.md)。完整 macOS 打包需要发布签名身份；日常开发可直接使用上面的源码运行命令。

GitHub Actions 为普通分支和 PR 执行敏感信息扫描、许可校验、现有测试与跨平台编译；与项目版本一致的 `v*` 标签触发 Windows 双架构和 macOS ARM64 打包。产物保存在 Actions 附件中，工作流不自动创建 Release。原有 Gitea 工作流继续保留。

## 数据与连接

- 密码、托管私钥和已保存私钥口令加密存入 SQLite；macOS 正式版的自动解锁密钥由系统钥匙串管理，其他构建使用本机密钥文件，也可开启应用密码解锁。数据库中的连接地址、路径和审计等元数据没有整体加密。
- 恢复工作区和传输任务时保持离线，不会自动登录服务器或继续传输。强制结束或断电仍不保证恢复最后进度。
- MCP 可在授权范围内执行远程命令和读写文件，仅向可信客户端提供密钥。命令与输出可能含敏感内容，分享诊断材料前需脱敏。

详细边界见[数据与凭据](docs/storage.md)、[MCP 说明](docs/mcp-server-plan.md)及[已知限制](docs/limitations.md)。漏洞请按 [SECURITY.md](SECURITY.md) 私密报告。

## 文档与贡献

| 使用应用 | 参与开发 |
|---|---|
| [首次使用](docs/getting-started.md) | [贡献指南](CONTRIBUTING.md) |
| [会话与设置](docs/session-settings.md) | [开发与构建](docs/development.md) |
| [文件管理与传输](docs/file-transfer.md) | [架构与生命周期](docs/architecture.md) |
| [终端与配色](docs/terminal-themes.md) | [主题与设计令牌](docs/theme-integration.md) |
| [服务器监控](docs/monitoring.md) | [协议与原生联调](dev/ssh-integration/README.md) |

更多内容见[文档索引](docs/README.md)。欢迎通过 [Issue](https://github.com/zeroy1024/Burrow/issues) 报告问题或提出建议，提交 PR 前请阅读贡献指南。

## 许可

Burrow 自有代码采用 [MIT 许可证](LICENSE)。第三方代码、原生库、字体、图标和配色保留各自许可，完整文本见[第三方声明](THIRD_PARTY_NOTICES.md)，并随原生应用分发。
