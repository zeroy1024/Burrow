# 架构与生命周期

[English](architecture.md) | [简体中文](architecture.zh.md)

## 代码入口

| 位置 | 职责 |
|---|---|
| [main.go](../main.go) | 应用入口 |
| [internal/desktop](../internal/desktop/) | 页面组合、会话、连接调度、持久化、凭据与任务生命周期 |
| [internal/sshclient](../internal/sshclient/) | SSH 连接、认证、主机信任、Shell/Exec/子系统与连接租约 |
| [internal/wggateway](../internal/wggateway/) | 纯用户态 WireGuard/netstack、严格的单 Peer 导入、隧道 DNS 与身份握手验证 |
| [internal/fileclient](../internal/fileclient/) | 本地、SFTP、FTP/FTPS 文件操作及可选能力 |
| [internal/vault](../internal/vault/) | 凭据加密、解锁密钥和数据密钥生命周期、保护设置转换与配套备份 |
| [internal/store](../internal/store/) | SQLite schema、事务、主机信任与一致备份 |
| [internal/mcpserver](../internal/mcpserver/) | 本机 Streamable HTTP、密钥校验、授权与策略检查、工具和审计入口 |
| [internal/localfs](../internal/localfs/) | 平台文件规则：私有创建、Windows 句柄重命名与 ACL |
| [internal/locales](../internal/locales/) | 中英文 ARB/ICU 资源、类型化消息函数、跨帧提示和可本地化错误 |
| [internal/uikit](../internal/uikit/) | 共享展示组件与 UI 状态 |
| [internal/theme](../internal/theme/) | 应用语义令牌、组件令牌及生成产物 |
| [internal/termtheme](../internal/termtheme/) | Ghostty 配色解析、内置配色及终端偏好 |
| [third_party/mygo-terminal](../third_party/mygo-terminal/) | 原生终端视图与最小本地扩展 |
| [third_party/pkg-sftp](../third_party/pkg-sftp/) | SFTP 私有暂存文件创建扩展 |

应用组合 UI 与业务，UIKit 只消费主题、显示数据和回调；网络、文件、凭据和后台任务由应用及协议层持有。

## 保存条目、运行会话与连接

保存条目描述下次如何连接，SSH 和文件目录分别维护。运行会话持有建立时的端点和认证快照，编辑目录不会静默修改活跃连接的身份。

SSH transport 与业务 channel 分离。同一会话分屏在原连接上新建独立 Shell channel，窗格移动、组合拆分和拖回标签保留原终端对象。单 channel 关闭不结束兄弟 channel；最后消费者释放共享连接。

主要入口：[ssh_sessions.go](../internal/desktop/ssh_sessions.go)、[ssh_terminal.go](../internal/desktop/ssh_terminal.go)、[file_connections.go](../internal/desktop/file_connections.go) 和 [pane_drag.go](../internal/desktop/pane_drag.go)。

WireGuard 传输按客户端/Peer 身份在终端、传输、MCP 和测试之间共享。相同配置复用同一个 UDP 设备；同一身份仍在使用时，其他不兼容配置会报错，需先关闭相关使用者。每个使用者持有引用，最后释放时关闭设备及内存网络栈。此处与按作用域隔离的 SSH 传输不同，避免同一 WireGuard 身份的多个设备争抢对端地址。见 [WireGuard 网关](wireguard.zh.md)。

## 启动与解锁

进程先取得独占锁并打开数据库，加载解锁界面的主题和语言。新库默认创建本机自动解锁材料；开启启动解锁的库等待输入密码，旧库等待一次性迁移。解锁成功后只执行一次业务初始化，再恢复工作区、加载任务并按设置启动 MCP。

未解锁只构建启动界面，不执行主界面的业务 tick；菜单、连接、MCP HTTP 准入和任务接受入口受应用状态约束。未初始化退出只释放资源，不将空目录、默认设置或空工作区覆盖到数据库。

自动解锁统一使用私有本地文件。旧钥匙串库先读取原密钥，轮换并迁入文件方式，再验证提交结果；密钥缺失时不回退或生成替代密钥。旧钥匙串项目保留供历史备份恢复，可移植备份从内存数据密钥生成独立的文件封装。

设置切换和密码修改走现有持久化 worker，轮换数据密钥和全部密文后原子提交。转换期间拒绝新的业务准入，已认证的连接和任务继续，检查点仍可保存。无法确认提交结果或业务初始化失败时阻止继续使用，避免用部分初始化状态恢复并保存重复工作区。

## 页面、窗口与任务

SSH、文件传输、MCP、设置互斥构建当前页面，页面挂载不拥有业务任务生命周期。切页保留连接、目录历史、终端滚动与传输；关闭设置只清临时 UI，复用窗口生命周期内的稳定句柄。工作区可保存并恢复 SSH、文件或 MCP 的页面选择；设置页仍为临时页面。

关闭主窗口与应用退出使用同一流程，所有平台都会保存工作区与设置、停止 MCP 服务、暂停并保存传输任务，再关闭会话连接并退出进程。关闭最后标签的行为按偏好单独处理。

传输任务使用独立上下文和协议 client/subsystem。任务按文件标签身份分组，浏览视图关闭或站点配置改变不改向后台任务。执行与保存入口为 [transfer_backend.go](../internal/desktop/transfer_backend.go)、[transfer_store.go](../internal/desktop/transfer_store.go) 和 [workspace_store.go](../internal/desktop/workspace_store.go)。

## 文件一致性与持久化

- 写入通过任务独有的 `.part`，关闭和最终提交成功才完成；继续时检查源基线、部分内容摘要及目录中已完成项。
- 目标冲突由真实元数据决定。应用内协调相同/重叠目标；本地目标对大小写和 Unicode 规范等价作保守协调。
- 本地上传源、下载目标和相应清理使用固定目录句柄约束解析，递归不跟随符号链接。SFTP 私有暂存文件在创建请求中原子指定权限，确认后才发送内容。
- 结构化数据集中到 SQLite。目录和工作区使用事务，设置和布局防抖写入；传输 worker 在安全点提交清单、摘要和重命名意图。读取失败阻止覆盖，不自动重建数据库。
- 密码、托管私钥与已保存私钥口令统一由 Vault 加密后写入数据库；密文与配置在同一事务提交，端点快照继续绑定业务凭据版本。系统凭据服务仅供一次性迁移；所有平台的自动解锁密钥均使用私有本地文件。数据格式和目录统一见[数据与凭据](storage.zh.md)。

Unix 的本地元数据与权限位和 Windows 的 ACL 通过平台文件隔离；Windows 本地 client 提供 `PermissionPreserver`，供提交阶段复制并验证目标 ACL。私有文件创建与原生访问集中在 `localfs`，不在队列业务中加入 Win32 调用。SSH agent 同样拆分为 Unix socket / Windows 本机命名管道，复用原认证生命周期。

用户可见的恢复和失败语义见[文件管理与传输](file-transfer.zh.md)，协议能力不足的行为见[已知限制](limitations.zh.md)。

## 界面语言

应用复用 MyGo 的语言状态和资源生成器。`mygo.json` 声明 `internal/locales`，以简体中文为源语言，完整提供英文资源。设置中的语言值为稳定的 BCP 47 标签或空值（跟随系统），菜单和 MCP 展示快照随语言更新。跨帧错误和通知保存消息函数及参数，在显示时取词；业务错误保持原有错误链。显示文字不作为 MCP 筛选、授权或协议值。维护规则见[界面语言](i18n.zh.md)。

## 终端与本地依赖

Ghostty 维护终端状态，MyGo 绘制文字网格、选区、光标及原生滚动条。应用保留窗格布局、焦点、输入绑定、配色和上下栏；网格按正文实际字体和像素尺寸计算，并通知远端 PTY。

`ViewportBackdrop` 读取同一帧的相邻历史行，按网格坐标连续补画到栏下；它不创建第二个终端，不移动真实视口。毛玻璃作为覆盖层绘制。显示行为见[终端与配色](terminal-themes.zh.md)。

[go.mod](../go.mod) 用 `replace` 接入两个本地模块，主 MyGo 仍固定 v0.7.3：

| 本地模块 | 扩展及维护依据 |
|---|---|
| `mygo-terminal` | 实时回滚上限、栏下历史补画与固定 Ghostty ABI；见 [FORK.md](../third_party/mygo-terminal/FORK.md) |
| `pkg-sftp` | `Client.OpenFileMode` 在创建时传入权限；见 [FORK.md](../third_party/pkg-sftp/FORK.md) |

保留上游许可证、资源 manifest 和来源说明。主题与组件约定见[主题接入](theme-integration.zh.md)及[UIKit](uikit.zh.md)。
