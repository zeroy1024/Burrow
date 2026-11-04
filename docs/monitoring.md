# 服务器监控

监控面板通过当前 SSH 连接采集服务器信息。打开面板后开始采样；关闭面板、切页或隐藏窗口会停止新增采集并取消面板自己的请求，不关闭 Shell 或传输任务。

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
| Windows | `powershell.exe -NoProfile -NonInteractive -EncodedCommand`，读取 `Win32_OperatingSystem`、注册表 `CurrentVersion` 与 `OpenSSH\DefaultShell` | 系统名与 `DisplayVersion` 随系统语言；内核为 `Version.UBR`；架构取 `PROCESSOR_ARCHITECTURE`；默认 shell 未设置时为 `cmd` |
| 其他系统 | 仅保存 `uname` 结果 | 系统名、主机名与内核版本 |

Windows 脚本兼容 Windows PowerShell 5.1，错误在脚本内捕获并随结果返回，结果以 base64 输出以避开控制台代码页；编码命令约 2.6 KB，低于 cmd.exe 命令行上限。Windows 采集需要启动一次 PowerShell，约 1 秒。结果先保留在编辑草稿中，保存会话时写入 SQLite；取消编辑不会修改已保存的信息。每次新建 SSH 连接，在第一个终端就绪后后台刷新并保存一次，同一连接的多个终端共享结果，不依赖监控面板是否打开。

会话图标、系统描述和监控面板共用这份数据。图标按发行版 `ID` 匹配现有资源；未知 Linux 发行版使用通用企鹅。断开连接后仍可查看上次成功的结果；采集时间仅在内部用于判断记录新旧，不在面板显示。运行时间、进程数量以及各项使用率不作为基础信息缓存。

缓存绑定会话 ID、地址、端口、用户和完整跳板路径，修改目标后不会复用原目标的信息；已经连接的终端仍展示其实际连接目标的信息。基础信息独立存储在数据库的 `hostFacts` 记录中，不更新连接配置的修订时间。旧采集结果不能覆盖更新的记录。

采集单次限时 5 秒，失败不影响 SSH 连接，也不清空同一目标已有的成功结果。测试结果和监控面板会显示采集失败原因，数据库读写错误使用现有错误提示。

## 不可用与权限

显示 `—` 表示该项尚无有效数据或当前服务器无法提供。单项失败不清空其他有效指标；整个采集失败时，不会继续把旧数值显示为当前值。

网络优先选择有效 IPv4 默认路由，否则选择 IPv6。没有明确默认接口、或最低 metric 的默认路由存在歧义时，不会累加桥接、VLAN 或虚拟接口替代。

进程信息受账号权限、hidepid 和内核配置影响。Top 原始记录超过预算时，整个 Top 暂时不可用，不把截断的子集当作完整排名。

根盘容量依赖 SFTP 的 statvfs 扩展，还需确认 SFTP 与 Shell 看到同一个根挂载。服务器不支持 SFTP、没有扩展、被 chroot 隔离或权限不足时，其他监控仍可使用。

## 延迟与采样周期

Shell 状态栏延迟独立于监控开关：当前可见终端按连接组共享 SSH 应用层往返时间，每 5 秒刷新；超过 3 秒的结果或 10 秒内没有有效样本时显示未知。它不是 ICMP ping。

监控主体约每 2 秒采样，单次 Exec 限时 5 秒且只保留一个在途请求；实际间隔包含采集耗时。根盘容量约每 10 秒采样，独立请求限时 3 秒。隐藏窗口或非 SSH 页面不新增延迟采样。

## 开发说明

监控主体读取 `/proc/stat`、`loadavg`、`meminfo`、`net/dev`、路由、`uptime`、进程 stat 和 mountinfo。基础信息单独读取 `/proc/sys/kernel/hostname`、`/proc/sys/kernel/osrelease`，并从 `/etc/os-release` 解析字面值。两者共用固定 `/bin/sh` 采集和记录解析机制，除基础信息用 `uname -m` 读取架构外只使用 shell 内建，不调用 `ps`、`top`、`df` 等外部采集工具，也不执行服务器返回的文本。

进程原始记录预算为 512 KiB，全部记录留在既有 1 MiB Exec 输出上限内。根盘容量使用短生命周期的独立 SFTP subsystem，不复用文件浏览或传输 client。

平台实现位于 `internal/hostinfo`：`Collector` 负责基础信息并创建 `Sampler`，`Sampler` 保存计算差分所需的上次样本，统一输出 `Metrics`；平台文件以 `_host.go` 结尾，避免被 Go 当作本机编译目标的构建约束。

源码入口：[hostinfo.go](../internal/hostinfo/hostinfo.go)、[linux_host.go](../internal/hostinfo/linux_host.go)、[darwin_host.go](../internal/hostinfo/darwin_host.go)、[windows_host.go](../internal/hostinfo/windows_host.go)、[host_facts.go](../internal/desktop/host_facts.go)、[status_backend.go](../internal/desktop/status_backend.go) 和 [ssh_latency.go](../internal/desktop/ssh_latency.go)。验证边界统一见[已知限制](limitations.md)。
