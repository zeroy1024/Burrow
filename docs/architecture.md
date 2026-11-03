# 架构与生命周期

## 代码入口

| 位置 | 职责 |
|---|---|
| [main.go](../main.go) | 应用入口 |
| [internal/desktop](../internal/desktop/) | 页面组合、会话、连接调度、持久化、凭据与任务生命周期 |
| [internal/sshclient](../internal/sshclient/) | SSH 连接、认证、主机信任、Shell/Exec/子系统与连接租约 |
| [internal/fileclient](../internal/fileclient/) | 本地、SFTP、FTP/FTPS 文件操作及可选能力 |
| [internal/uikit](../internal/uikit/) | 共享展示组件与 UI 状态 |
| [internal/theme](../internal/theme/) | 应用语义令牌、组件令牌及生成产物 |
| [internal/termtheme](../internal/termtheme/) | Ghostty 配色解析、内置配色及终端偏好 |
| [third_party/mygo-terminal](../third_party/mygo-terminal/) | 原生终端视图与最小本地扩展 |
| [third_party/pkg-sftp](../third_party/pkg-sftp/) | SFTP 私有暂存文件创建扩展 |

应用组合 UI 与业务，UIKit 只消费主题、显示数据和回调；网络、文件、钥匙串和后台任务由应用及协议层持有。

## 保存条目、运行会话与连接

保存条目描述下次如何连接，SSH 和文件目录分别维护。运行会话持有建立时的端点和认证快照，编辑目录不会静默修改活跃连接的身份。

SSH transport 与业务 channel 分离。同一会话分屏在原连接上新建独立 Shell channel，窗格移动、组合拆分和拖回标签保留原终端对象。单 channel 关闭不结束兄弟 channel；最后消费者释放共享连接。

主要入口：[ssh_sessions.go](../internal/desktop/ssh_sessions.go)、[ssh_terminal.go](../internal/desktop/ssh_terminal.go)、[file_connections.go](../internal/desktop/file_connections.go) 和 [pane_drag.go](../internal/desktop/pane_drag.go)。

## 页面、窗口与任务

SSH、文件传输、设置互斥构建当前页面，页面挂载不拥有业务任务生命周期。切页保留连接、目录历史、终端滚动与传输；关闭设置只清临时 UI，复用窗口生命周期内的稳定句柄。

普通关闭窗口隐藏同一个原生窗口；监控与延迟停止新增可见页面采样，后台传输继续。应用退出才统一收敛 worker、保存状态并释放连接。关闭最后标签的行为按偏好单独处理。

传输任务使用独立上下文和协议 client/subsystem。任务按文件标签身份分组，浏览视图关闭或站点配置改变不改向后台任务。执行与保存入口为 [transfer_backend.go](../internal/desktop/transfer_backend.go)、[transfer_store.go](../internal/desktop/transfer_store.go) 和 [workspace_store.go](../internal/desktop/workspace_store.go)。

## 文件一致性与持久化

- 写入通过任务独有的 `.part`，关闭和最终提交成功才完成；继续时检查源基线、部分内容摘要及目录中已完成项。
- 目标冲突由真实元数据决定。应用内协调相同/重叠目标；本地目标对大小写和 Unicode 规范等价作保守协调。
- 本地上传源、下载目标和相应清理使用固定目录句柄约束解析，递归不跟随符号链接。SFTP 私有暂存文件在创建请求中原子指定权限，确认后才发送内容。
- 配置使用校验、临时文件及原子替换，读取失败阻止覆盖。各配置文件独立保存，不构成跨文件事务或崩溃恢复日志。
- 密码与私钥口令由系统凭据接口管理；端点快照绑定凭据版本。数据格式和目录统一见[数据与凭据](storage.md)。

用户可见的恢复和失败语义见[文件管理与传输](file-transfer.md)，协议能力不足的行为见[已知限制](limitations.md)。

## 终端与本地依赖

Ghostty 维护终端状态，MyGo 绘制文字网格、选区、光标及原生滚动条。应用保留窗格布局、焦点、输入绑定、配色和上下栏；网格按正文实际字体和像素尺寸计算，并通知远端 PTY。

`ViewportBackdrop` 读取同一帧的相邻历史行，按网格坐标连续补画到栏下；它不创建第二个终端，不移动真实视口。毛玻璃作为覆盖层绘制。显示行为见[终端与配色](terminal-themes.md)。

[go.mod](../go.mod) 用 `replace` 接入两个本地模块，主 MyGo 仍固定 v0.7.3：

| 本地模块 | 扩展及维护依据 |
|---|---|
| `mygo-terminal` | 实时回滚上限、栏下历史补画与固定 Ghostty ABI；见 [FORK.md](../third_party/mygo-terminal/FORK.md) |
| `pkg-sftp` | `Client.OpenFileMode` 在创建时传入权限；见 [FORK.md](../third_party/pkg-sftp/FORK.md) |

保留上游许可证、资源 manifest 和来源说明。主题与组件约定见[主题接入](theme-integration.md)及[UIKit](uikit.md)。
