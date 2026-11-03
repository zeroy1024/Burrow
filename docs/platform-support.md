# 平台支持与验收状态

共用 SSH、传输队列、会话和持久化逻辑，系统差异通过平台文件及 MyGo 的原生接口实现。构建成功、能够启动和功能验收分别记录。

## 当前目标

| 系统 / 架构 | 构建与产物 | 原生验证状态 |
|---|---|---|
| macOS arm64 | APP、DMG、ZIP；系统基线为 macOS 13 | 已有测试/race、静态检查及标准打包；本次未执行 GUI 验收 |
| macOS amd64 | 同一构建入口保留该目标 | 本次未在 Intel 机器运行 |
| Windows amd64 / arm64 | 已在与 runner 相同的 Linux 镜像中生成 NSIS EXE 安装程序及便携 ZIP，包含对应 `ghostty-vt.dll` | 尚无 Windows 原生运行结果；最低系统版本亦待原生验证确认 |
| Linux amd64 | 已有目标编译检查；构建入口复用 MyGo | 未验证发行版依赖、X11/Wayland、系统凭据及桌面交互 |
| Linux arm64 | 构建入口可选择 | 本次未验证 |

构建方式与产物位置见[开发与构建](development.md)。Windows 本机构建由 MyGo 准备 NSIS；交叉构建安装程序需主机提供 `makensis`。当前 macOS 环境未安装 NSIS；Ubuntu 24.04 amd64 镜像已验证两种 Windows 架构的 NSIS 安装程序、ZIP 内容及 SHA-256 摘要。Gitea 的实际触发和附件上传仍需在 workflow 提交后验收。

## 已接入的平台差异

- 本地文件元数据隔离：Unix 使用 uid/gid 和 mode，Windows 显示 ACL，避免引用不存在的 `syscall.Stat_t`。
- Windows 私有文件在创建时设置当前账号与 SYSTEM 的受保护 DACL；已有暂存文件写入前收紧访问规则，下载覆盖前复制并验证目标 DACL。
- 文件打开和重命名基于已持有的目录/文件句柄，不通过「先检查不存在再重命名」实现禁止覆盖；文件名按目标系统组件规则校验。
- agent 认证和检测共用平台连接入口：Unix socket 或 Windows 本机命名管道，保留原超时、关闭及取消机制。
- SQLite 使用纯 Go 驱动，Windows 数据位于 `%LOCALAPPDATA%`，与缓存分目录；开发/正式目录和凭据服务隔离。
- 凭据存储使用 MyGo 接口；快捷键、系统窗口按钮预留区和普通关窗行为按平台处理。

Windows 命名管道使用固定 `go-winio v0.6.4`，其许可证随应用「关于」展示；没有升级主 MyGo 或 Ghostty。

## Windows 原生验收

下列项目仍需在实际 Windows 会话中完成，不能由交叉编译替代：

- 启动 EXE/解压便携包，检查 DLL 加载、显示缩放、窗口按钮、最小化与任务栏恢复。
- SSH 密码、普通/加密私钥、agent、首次信任与主机密钥变更拒绝。
- 终端输入、IME、复制粘贴、分屏焦点、窗口 resize 及关闭单窗格隔离。
- SFTP/FTP 上传下载，新目标提交、覆盖与保留两者，权限/文件占用错误，以及暂停、继续、取消和退出恢复。
- 私钥和暂存文件 ACL、覆盖后目标 ACL，路径替换、保留名称及不支持文件名的错误行为。
- Credential Manager 保存/读取，SQLite 文件锁、WAL 与私有 ACL，正常退出及重启后离线恢复。
- 使用生成的 EXE 安装程序验证安装、快捷方式、升级、卸载及数据保留。

现有原生 E2E 启动器使用 macOS `.app` 和自动化 overlay，尚未迁移为 Windows 驱动。验证结果应记录实际提交、系统版本、架构和未覆盖项，规则见[验证范围](limitations.md#验证如何解读)。
