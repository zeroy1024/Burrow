# Changelog

Every released version has a `## <version> - <date>` section, English first, then Simplified Chinese. The release build reads the section of the version in `mygo.json` as the notes shown in the app's update window, and fails when it is missing.

## Unreleased

- Add the GitHub repository address to **Settings → About**, with buttons to open it in the default browser and copy the address.
- Show a confirmation after copying the project address or app version in **Settings → About**.
- Refine interface animations with smoother spring motion for dialogs, sidebars, drawers, segmented tabs, and switches, plus smooth transitions when switching main sections, settings categories, dropdowns, menus, and server monitor meters.

**简体中文**

- 在**设置 → 关于**中新增 GitHub 项目地址，支持在默认浏览器中打开和复制地址。
- 在**设置 → 关于**中复制项目地址或应用版本后显示成功提示。
- 优化全局界面动效：弹窗、侧边栏、抽屉、分段切换条与开关采用更顺滑的弹簧过渡，并在主导航切换、设置分类切换、菜单与下拉框展开以及监控进度条变化时提供平滑过渡。

## 0.1.9 - 2026-11-13

- Add in-app updates: Burrow checks GitHub Releases in the background, shows the release notes, and installs signed updates. **Settings → About → Software update** has a manual check, the automatic-check switch, the update channel (Stable or Preview), a system-proxy switch, and an optional GitHub mirror URL.
- Windows executables are no longer compressed with UPX.

**简体中文**

- 新增应用内更新：Burrow 在后台检查 GitHub Release，展示更新说明并安装带签名的更新。**设置 → 关于 → 软件更新**提供手动检查、自动检查开关、更新渠道（正式版或预览版）、系统代理开关和可选的 GitHub 镜像地址。
- Windows 可执行文件不再使用 UPX 压缩。

## 0.1.8 - 2026-11-12

- Fix the light, dark, and fixed terminal color-scheme dropdowns in Settings, which could not open.
- Show latency from passive TCP RTT statistics on macOS, Linux, and Windows instead of automatic SSH requests. This removes the RouterOS-specific workaround and the request that caused immediate disconnects.
- Show target RTT as unknown (`—`) through SOCKS proxies, SSH jump hosts, and the built-in WireGuard stack.
- Use the keepalive interval for TCP keepalive, 30 seconds by default, and add a separate **SSH health check** setting that is off by default.
- Keep per-hop keepalive and health-check settings in SSH gateway snapshots and connection reuse decisions.

**简体中文**

- 修复设置页的浅色、深色和固定终端配色下拉框无法展开的问题。
- macOS、Linux 和 Windows 的延迟展示改用被动 TCP RTT 统计，不再自动发送 SSH 请求；移除 RouterOS 特例及导致连接立即断开的请求。
- 经 SOCKS 代理、SSH 跳板和内置 WireGuard 网络栈时，目标 RTT 显示为未知（`—`）。
- 原有保活间隔用于 TCP 保活，默认 30 秒；新增独立的 **SSH 健康检测**设置，默认关闭。
- SSH 网关快照与连接复用判断保留各跳的保活和健康检测设置。

## 0.1.7 - 2026-11-12

- Add read-only AI session tabs that show each access key's commands, streamed output, file operations, and results. Closed tabs can be reopened from the audit details or the tray panel.
- Keep collecting monitoring samples for connected SSH terminals while the status panel is closed. Pane footers show latency, CPU, memory, and network rates where supported.
- Fix AI session cards that kept the old state after a failed transfer was retried.
- Raise the default MCP `ssh_exec` timeout to 300 seconds, with a maximum of 1800 seconds.

**简体中文**

- 新增只读 AI 会话选项卡，展示各访问密钥执行的命令、实时输出、文件操作和结果；关闭后可从审计详情或托盘面板重新打开。
- 已连接终端的 SSH 连接在状态面板关闭后继续采集监控，窗格页脚按服务端支持情况显示延迟、CPU、内存和网络速率。
- 修复传输失败后重试时 AI 会话卡片停留在旧状态的问题。
- MCP `ssh_exec` 默认超时提高到 300 秒，最高 1800 秒。

## 0.1.6 - 2026-11-11

- Fix a crash when choosing **Quick add to SFTP…** from an SSH host or **Quick add to SSH…** from an SFTP site.
- Allow saved hosts and sites to be deleted after cancelling their related transfers. Transferred files and MCP audit history are kept.
- Move automatic vault unlock to private local key files on all platforms. Existing macOS Keychain-backed vaults migrate once with their original key.

**简体中文**

- 修复 SSH 主机的**快捷添加到 SFTP…**和 SFTP 站点的**快捷添加到 SSH…**点击闪退的问题。
- 支持取消关联传输后删除已保存的主机或站点；已传输文件及 MCP 审计历史保留。
- 所有平台的凭据库自动解锁统一使用私有本地密钥文件；原 macOS 钥匙串凭据库通过原密钥一次性迁移。

## 0.1.5 - 2026-11-10

- Add background running: closing the main window can quit or only hide it, keeping enabled MCP, connections, and transfers. Quitting on close stays the default.
- Add a tray panel on macOS and Windows for MCP status and address, today's calls, SSH sessions and latency, and running transfers. Linux keeps the native tray menu.
- Add **Hide Dock icon** and **⌘Q only closes the window** options on macOS, both off by default.
- Launching Burrow again brings the existing window forward; background authentication requests show the main window.

**简体中文**

- 新增后台运行设置：关闭主窗口时可选择退出或仅隐藏窗口，后台保留已启用的 MCP、连接和传输；默认仍为关闭即退出。
- macOS 和 Windows 新增托盘面板，可查看及启停 MCP、复制接入地址、查看今日调用次数、SSH 会话与延迟以及进行中的传输；Linux 继续使用原生托盘菜单。
- macOS 新增「隐藏程序坞图标」和「⌘Q 仅关闭窗口」选项，默认关闭。
- 再次启动 Burrow 会唤醒已有窗口；后台认证请求会显示主窗口。

## 0.1.4 - 2026-11-09

- Add a userspace WireGuard gateway for terminals, SFTP, passive FTP/FTPS, connection tests, and MCP remote operations. It can be the first hop of an SSH / SOCKS5 path and needs no system TUN driver.
- Add Simplified Chinese, English, and follow-system interface languages.
- Authorize MCP file reading, file writing, and command execution separately per connection. Equivalent SSH / SFTP profiles are shown together.
- Redesign the MCP access key dialog and fix member matching of merged connections in block policies and audit filters.

**简体中文**

- 新增纯用户态 WireGuard 网关，为终端、SFTP、被动模式 FTP/FTPS、连接测试及 MCP 远端操作提供应用内隧道；可作为 SSH / SOCKS5 多级路径的第一跳，无需系统 TUN 驱动。
- 界面支持简体中文、English 和跟随系统。
- MCP 连接授权支持独立配置文件读取、文件写入和命令执行权限；等价 SSH / SFTP 配置合并展示。
- 重新设计 MCP 授权密钥弹窗，修复合并连接在拦截策略和审计筛选中的成员匹配。

## 0.1.3 - 2026-11-06

- Add a gateways page with SSH / SOCKS5 gateways, multi-hop paths, an app-wide default route, and per-connection gateways for SSH, file transfers, and MCP.
- Add path display, reference management, and per-hop connectivity tests for gateways.
- Add quick profile copies between SSH hosts and SFTP sites.
- Fix multi-line command audit summaries that showed only an ellipsis and some command policy mismatches.
- Upgrade the database to schema v5 on first launch, with an automatic backup. Existing SSH jump hosts become gateways.

**简体中文**

- 新增独立网关页面，支持 SSH / SOCKS5、多级路径、应用默认出口及按连接选择网关，覆盖 SSH、文件传输和 MCP。
- 网关页面提供连接路径、引用管理和逐跳连通性测试。
- SSH 主机与 SFTP 站点可互相快捷添加配置。
- 修复多行命令审计摘要仅显示省略号及部分命令策略误匹配的问题。
- 首次启动将数据库升级至 schema v5 并自动备份；原有 SSH 跳板配置转换为网关。

## 0.1.2 - 2026-11-05

- First developer preview on GitHub.
- SSH tabs and split panes with password, private key, SSH agent, and jump host authentication.
- Dual-pane SFTP / FTP / FTPS file management with a transfer queue that can pause, resume, and recover offline.
- Linux server status, terminal color schemes, keyboard shortcuts, and encrypted credential storage.
- Local MCP service with connection and capability authorization, block policies, and an audit log.

**简体中文**

- 首个 GitHub 开发预览发行版。
- SSH 多标签与组合分屏，支持密码、私钥、SSH agent 和跳板连接。
- SFTP / FTP / FTPS 双面板文件管理与传输队列，支持暂停、继续和离线恢复。
- Linux 服务器状态、终端配色、快捷键及加密凭据存储。
- 本机 MCP 服务、连接与能力授权、拦截策略及操作审计。
