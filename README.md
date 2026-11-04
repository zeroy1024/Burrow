# Burrow

Burrow 是一款原生 SSH 与文件传输桌面应用，提供多标签终端、分屏、SFTP / FTP / FTPS、传输队列和 Linux 服务器监控。界面使用 Go 与 MyGo 构建，终端由 Ghostty VT 内核驱动。

macOS 运行基线为 13 及以上系统；Windows amd64/arm64 的平台适配和构建入口已接入，原生运行验收尚未完成。各平台状态见[平台支持](docs/platform-support.md)。

## 主要功能

- **SSH 终端**：密码、私钥、加密私钥、SSH agent 和跳板连接；支持终端输入、Unicode / IME、选择与复制粘贴。
- **多会话工作区**：标签、组合分屏、窗格移动与放大；同一会话分屏复用 SSH 连接，每个 Shell 独立运行。
- **文件管理**：本地与远程双面板，支持 SFTP、FTP 和 FTPS，以及目录导航、搜索、排序、新建、重命名和删除。
- **文件传输**：文件和目录上传下载、并发任务、暂停、继续、取消、重试和重名处理；队列按文件标签隔离。
- **服务器监控**：Linux CPU、内存、网络、进程和系统信息；具备相应 SFTP 能力时显示根盘容量。
- **本机 MCP**：Streamable HTTP、授权密钥、连接与能力授权、SSH / 文件拦截策略及审计；复用现有连接和传输队列。macOS 已通过真实 SSH/SFTP/FTP/显式及隐式 FTPS 和任务生命周期联调，验证范围见 [MCP 联调说明](dev/mcp-integration/README.md)。
- **个性化与恢复**：明暗主题、终端配色、字体、光标、快捷键，以及离线恢复工作区和传输任务。

## 快速开始

已具备 mise 的开发环境可在仓库根目录运行：

```sh
mise install
mise exec -- go run .
```

首次运行可能下载终端所需的原生库。应用初始目录为空，不会自动连接服务器。进入 SSH 页面，新建会话并填写地址和认证信息，选择「保存并连接」即可开始；首次连接需要核对主机指纹。

详细步骤见[首次使用](docs/getting-started.md)。构建 `.app`、ZIP 和 DMG：

```sh
mise run build
```

工具链要求、产物位置和验证命令见[开发与构建](docs/development.md)。

Gitea 的普通分支推送和 PR 只运行[提交检查](.gitea/workflows/check.yml)；推送 `v*` 发布标签才触发[编译打包](.gitea/workflows/package.yml)，产出 Windows amd64/arm64 的 UPX 安装程序和 macOS ARM64 DMG。runner 要求与附件说明见[Gitea CI](docs/development.md#gitea-ci)。

## 使用须知

- 结构化数据统一使用 SQLite；密码和私钥口令使用系统凭据存储，托管私钥保留独立文件。详见[数据与凭据](docs/storage.md)。
- 恢复工作区和任务时保持离线，不会自动登录服务器或继续传输。
- 运行中保存传输检查点，正常退出再停稳任务并保存；强制结束或断电仍不保证恢复最后进度。
- 服务器能力会影响续传、覆盖、权限保留和监控。详见[已知限制与验证范围](docs/limitations.md)。

## 文档

完整入口见[文档索引](docs/README.md)。

| 使用应用 | 开发与维护 |
|---|---|
| [首次使用](docs/getting-started.md) | [开发与构建](docs/development.md) |
| [平台支持与验收状态](docs/platform-support.md) | [架构与生命周期](docs/architecture.md) |
| [会话与设置](docs/session-settings.md) | [联调与原生 E2E](dev/ssh-integration/README.md) |
| [终端与配色](docs/terminal-themes.md) | [主题与设计令牌](docs/theme-integration.md) |
| [文件管理与传输](docs/file-transfer.md) | [共享 UI 组件](docs/uikit.md) |
| [Linux 监控](docs/monitoring.md) | [历史归档](docs/archive/README.md) |

MCP 的接入、数据边界、工具契约和验证范围见 [MCP Server 功能方案](docs/mcp-server-plan.md)。页面设计交接见 [MCP 页面设计功能清单](docs/mcp-page-design-brief.md)。

阶段计划、整改和历史验收记录保存在[历史归档](docs/archive/README.md)，不作为当前使用说明。
