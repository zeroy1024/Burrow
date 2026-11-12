# 服务器监控

[English](monitoring.md) | [简体中文](monitoring.zh.md)

Burrow 通过每个已有终端连接成功的 SSH 连接采集服务器信息。终端连接成功后即开始采样，与监控面板是否打开无关，因此打开面板时可直接看到已采集的趋势。连接结束时丢弃其样本；监控请求不会关闭 Shell 或传输任务。

## 远端平台

采集按远端平台选用实现，统一输出主机基础信息和监控指标。Linux、macOS、Windows 均采集基础信息；监控指标目前只有 Linux 提供，macOS、Windows 和其他系统的监控面板提示“暂不支持监控”，不执行 Linux 采集脚本或反复报错。

- 自动识别：SSH 服务端标识含 `OpenSSH_for_Windows` 时视为 Windows；否则执行 `uname -snr`，`Linux`、`Darwin` 分别对应 Linux、macOS，`MINGW` / `MSYS` / `CYGWIN` 视为 Windows，其余记为其他系统。`uname` 无法执行时再尝试 Windows 采集命令，成功即为 Windows；仍失败则在测试连接和监控面板提示可手动指定。
- 手动指定：会话「高级 · 远端系统」可选自动识别、Linux、macOS 或 Windows。指定后不再识别，适用于 Windows 默认 shell 为 WSL / Cygwin、服务端改写了标识等情况。
- 平台写入主机基础信息。此前保存的记录均来自 Linux 采集，读取时按 Linux 处理。监控首次连接某主机时等待识别完成后开始采样，之后复用已保存的平台。
- MCP `connections_list` 为 SSH 连接返回 `os`（`linux` / `darwin` / `windows` / `other`），以及同一平台最近一次采集到的 `os_name`、`arch`；Windows 另返回默认 shell `shell`。尚未识别时省略。

## 指标含义

| 指标 | 含义与范围 |
|---|---|
| CPU | 两次有效采样之间的聚合使用率，排除 guest 重复计数；空闲包含 iowait |
| 负载 | 1、5、15 分钟系统负载 |
| 内存 | `MemTotal - MemAvailable`，不会把全部缓存算作已用 |
| 网络 | 所选默认路由接口的收发速率，不是所有网卡的合计 |
| 进程 Top | 区间 CPU 使用率最高的前 4 个可读进程；单核满载为 100%，多线程可超过 100% |
| 进程数量 | 当前 SSH 账号可见的进程数量，与 Top 的采集预算独立 |
| 系统信息 | 主机名、内核和发行版使用共享的主机基础信息；运行时间实时采集 |
| 根盘 | 根挂载的设备/文件系统信息；具备可用 SFTP statvfs 能力时显示容量和使用率 |

CPU、网络与进程速率需要两次有效采样建立差分。切换接口、计数回退或进程身份变化时重建相应基线。进程以 PID 和启动时间识别，避免 PID 复用串入旧样本。

## 主机基础信息

SSH 测试连接成功后识别平台并读取主机名、系统名称与版本、内核和 CPU 架构：

| 平台 | 采集方式 | 说明 |
|---|---|---|
| Linux | `/bin/sh` 读取 `/proc/sys/kernel/hostname`、`osrelease`、`/etc/os-release`，`uname -m` 取架构 | 系统名为 `PRETTY_NAME` |
| macOS | `/bin/sh` 执行 `sw_vers -productName / -productVersion`、`uname -n / -r / -m` | 系统名如 `macOS 27.2`，内核为 Darwin 版本 |
| Windows | `powershell.exe -NoProfile -NonInteractive -EncodedCommand`，经 WMI 读取 `Win32_OperatingSystem`、注册表 `CurrentVersion` 与 `OpenSSH\DefaultShell` | 系统名与 `DisplayVersion` 随系统语言；内核为 `Version.UBR`；架构取 `PROCESSOR_ARCHITECTURE`；默认 shell 未设置时为 `cmd` |
| 其他系统 | 仅保存 `uname` 结果 | 系统名、主机名与内核版本 |

Windows 脚本兼容 Windows PowerShell 2.0 及以上（不使用 `Get-CimInstance`、`[ordered]`、`ConvertTo-Json`），错误在脚本内捕获并随结果返回，每个字段以 base64 UTF-8 输出以避开控制台代码页；编码命令约 2.6 KB，低于 cmd.exe 命令行上限。Windows 采集需要启动一次 PowerShell，约 1 秒。结果先保留在编辑草稿中，保存会话时写入 SQLite；取消编辑不会修改已保存的信息。每次新建 SSH 连接，在第一个终端就绪后后台刷新并保存一次，同一连接的多个终端共享结果，不依赖监控面板是否打开。

会话图标、系统描述和监控面板共用这份数据。图标使用各系统官方 logo 的单色字形（Simple Icons 16.35.0，CC0；Windows 按官方四窗格绘制），按平台和 `/etc/os-release` 的 `ID` 匹配：Ubuntu、Debian、CentOS、Alpine、Fedora、Arch、Red Hat（`rhel`）、Rocky Linux（`rocky`）、AlmaLinux、openSUSE（`opensuse-*`）、Kali、Linux Mint（`linuxmint`）、Raspberry Pi OS（`raspbian`）、deepin，以及 macOS、Windows 和 FreeBSD；其他 Linux 发行版与未识别主机使用通用 Tux。图标底色为品牌色混入卡片色，字形默认用官方色，与底色对比度不足时向黑或白调整（浅色 3:1、深色 4.5:1）。断开连接后仍可查看上次成功的结果；采集时间仅在内部用于判断记录新旧，不在面板显示。运行时间、进程数量以及各项使用率不作为基础信息缓存。

缓存绑定会话 ID、地址、端口、用户和完整跳板路径，修改目标后不会复用原目标的信息；已经连接的终端仍展示其实际连接目标的信息。基础信息独立存储在数据库的 `hostFacts` 记录中，不更新连接配置的修订时间。旧采集结果不能覆盖更新的记录。

采集单次限时 5 秒，失败不影响 SSH 连接，也不清空同一目标已有的成功结果。测试结果和监控面板会显示采集失败原因，数据库读写错误使用现有错误提示。

## 不可用与权限

显示 `—` 表示该项尚无有效数据或当前服务器无法提供。单项失败不清空其他有效指标；整个采集失败时，不会继续把旧数值显示为当前值。

网络优先选择有效 IPv4 默认路由，否则选择 IPv6。没有明确默认接口、或最低 metric 的默认路由存在歧义时，不会累加桥接、VLAN 或虚拟接口替代。

进程信息受账号权限、hidepid 和内核配置影响。Top 原始记录超过预算时，整个 Top 暂时不可用，不把截断的子集当作完整排名。

根盘容量依赖 SFTP 的 statvfs 扩展，还需确认 SFTP 与 Shell 看到同一个根挂载。服务器不支持 SFTP、没有扩展、被 chroot 隔离或权限不足时，其他监控仍可使用。

## 延迟与采样周期

Shell 状态栏每 5 秒读取已建立直连的内核 TCP RTT 估计值，同一连接组的终端共享结果。读取过程不发送 SSH 请求，也不会关闭连接；读取失败或 10 秒内没有成功读取时显示未知（`—`）。连接空闲时可能保留先前流量的估计值；它表示网络 RTT，不是 SSH 命令响应时间或 ICMP ping。macOS 使用 TCP_CONNECTION_INFO，Linux 使用 TCP_INFO，Windows 使用 SIO_TCP_INFO。

SOCKS 代理、SSH 跳板和内置 WireGuard 网络栈连接的目标 RTT 显示为未知：这些连接由自定义拨号器提供，Burrow 不会将代理或第一跳套接字的 RTT 当作最终服务器的延迟。可选的 SSH 健康检测仍通过配置的路由与 SSH 目标交互。

**会话 → 高级 → TCP 保活**设置可用 TCP 套接字的空闲时间和探测间隔，连续三次探测失败后判定不可达；默认 30 秒，0 为关闭。直连 SSH 和 SOCKS 连接配置其系统 TCP 套接字；SSH 隧道沿用外层连接自己的策略；内置 WireGuard 网络栈没有可在此配置的系统 TCP 套接字，其持久保活仍是独立网关设置。已有 `keepAlive` 数值现在只配置 TCP，不会启用 SSH 请求。

**SSH 健康检测**单独显式启用：0 为默认关闭，旧配置也默认关闭。正数间隔仅在未收到数据达到空闲时间后发送 OpenSSH 风格全局请求；收到 SSH 数据会重置失败计数，连续三个实际检测到的空闲周期没有数据时才关闭连接。即使服务端忽略请求，也至多保留一个未完成请求。仅对已确认兼容的服务端启用，不支持该请求的服务端可能直接断开连接。实现不包含厂商特例。

监控主体约每 2 秒采样，单次 Exec 限时 5 秒且只保留一个在途请求；实际间隔包含采集耗时。根盘容量约每 10 秒采样，独立请求限时 3 秒。隐藏窗口或非 SSH 页面不新增延迟采样。监控在其他页面继续采样；应用不构建画面时采样暂停，间隔超过约 2 分钟的趋势窗口后恢复时，趋势重新开始。

## 面板展示

面板跟随当前聚焦的终端。顶部卡片显示会话名、系统图标和系统名，状态点区分实时、采集中、采集失败、连接中、连接失败、已断开和未监控，实时状态下每收到一次采样跳动一次；采集失败或部分指标不可用时，原因显示在卡片下方。会话未连接时只保留基础信息，可断开重连的会话提示在终端中按 Enter 重新连接。尚无数值的项目显示骨架占位：首次采样前和速率等待第二次采样时带流光；采集失败、服务器无法提供该项或开启减少动态效果时保持静止。

- CPU 主数字旁显示 1、5、15 分钟负载，1 分钟负载超过核数时变为警示色。趋势图纵轴按峰值在 5、10、25、50、100% 中取档，档位与峰值写在图下方。
- 内存、磁盘以百分比为主值，条形使用主题色，达到 70%、90% 时分别变为警示色和危险色。磁盘设备名与文件系统相同时只显示一次。
- 网络上下行各带迷你趋势，两者共用纵轴。
- 进程 Top 行底色按单核占用比例绘制，显示为 0.0% 的行降为次要文字。
- 基础信息可折叠，折叠后标题行仍显示运行时间。

CPU 和网络趋势在客户端保留同一连接最近 60 次有效采样（约 2 分钟），即图表的完整周期。隐藏面板、切换终端或切页时保留，采集失败或连接结束时清空。

终端已连接时，窗格页脚依次显示延迟以及最新的 CPU、内存百分比和下行/上行速率，均为图标加数值。它们在终端连接后同时出现，尚无数值时显示 `—`，未连接时全部隐藏；不支持监控的主机只显示延迟。窗格宽度小于 560 px 时省略网络速率，小于 380 px 时四项指标均不显示。

## 开发说明

监控主体读取 `/proc/stat`、`loadavg`、`meminfo`、`net/dev`、路由、`uptime`、进程 stat 和 mountinfo。基础信息单独读取 `/proc/sys/kernel/hostname`、`/proc/sys/kernel/osrelease`，并从 `/etc/os-release` 解析字面值。两者共用固定 `/bin/sh` 采集和记录解析机制，除基础信息用 `uname -m` 读取架构外只使用 shell 内建，不调用 `ps`、`top`、`df` 等外部采集工具，也不执行服务器返回的文本。

进程原始记录预算为 512 KiB，全部记录留在既有 1 MiB Exec 输出上限内。根盘容量使用短生命周期的独立 SFTP subsystem，不复用文件浏览或传输 client。

平台实现位于 `internal/hostinfo`：`Collector` 负责基础信息并创建 `Sampler`，`Sampler` 保存计算差分所需的上次样本，统一输出 `Metrics`；平台文件以 `_host.go` 结尾，避免被 Go 当作本机编译目标的构建约束。

源码入口：[hostinfo.go](../internal/hostinfo/hostinfo.go)、[linux_host.go](../internal/hostinfo/linux_host.go)、[darwin_host.go](../internal/hostinfo/darwin_host.go)、[windows_host.go](../internal/hostinfo/windows_host.go)、[host_facts.go](../internal/desktop/host_facts.go)、[status_backend.go](../internal/desktop/status_backend.go) 和 [ssh_latency.go](../internal/desktop/ssh_latency.go)。验证边界统一见[已知限制](limitations.zh.md)。
