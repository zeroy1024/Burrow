# Linux 服务器监控

监控面板通过当前 SSH 连接采集 Linux 服务器信息。打开面板后开始采样；关闭面板、切页或隐藏窗口会停止新增采集并取消面板自己的请求，不关闭 Shell 或传输任务。

## 指标含义

| 指标 | 含义与范围 |
|---|---|
| CPU | 两次有效采样之间的聚合使用率，排除 guest 重复计数；空闲包含 iowait |
| 负载 | 1、5、15 分钟系统负载 |
| 内存 | `MemTotal - MemAvailable`，不会把全部缓存算作已用 |
| 网络 | 所选默认路由接口的收发速率，不是所有网卡的合计 |
| 进程 Top | 区间 CPU 使用率最高的前 4 个可读进程；单核满载为 100%，多线程可超过 100% |
| 进程数量 | 当前 SSH 账号可见的进程数量，与 Top 的采集预算独立 |
| 系统信息 | 主机名、内核、发行版和运行时间 |
| 根盘 | 根挂载的设备/文件系统信息；具备可用 SFTP statvfs 能力时显示容量和使用率 |

CPU、网络与进程速率需要两次有效采样建立差分。切换接口、计数回退或进程身份变化时重建相应基线。进程以 PID 和启动时间识别，避免 PID 复用串入旧样本。

## 不可用与权限

显示 `—` 表示该项尚无有效数据或当前服务器无法提供。单项失败不清空其他有效指标；整个采集失败时，不会继续把旧数值显示为当前值。

网络优先选择有效 IPv4 默认路由，否则选择 IPv6。没有明确默认接口、或最低 metric 的默认路由存在歧义时，不会累加桥接、VLAN 或虚拟接口替代。

进程信息受账号权限、hidepid 和内核配置影响。Top 原始记录超过预算时，整个 Top 暂时不可用，不把截断的子集当作完整排名。

根盘容量依赖 SFTP 的 statvfs 扩展，还需确认 SFTP 与 Shell 看到同一个根挂载。服务器不支持 SFTP、没有扩展、被 chroot 隔离或权限不足时，其他监控仍可使用。

## 延迟与采样周期

Shell 状态栏延迟独立于监控开关：当前可见终端按连接组共享 SSH 应用层往返时间，每 5 秒刷新；超过 3 秒的结果或 10 秒内没有有效样本时显示未知。它不是 ICMP ping。

监控主体约每 2 秒采样，单次 Exec 限时 5 秒且只保留一个在途请求；实际间隔包含采集耗时。根盘容量约每 10 秒采样，独立请求限时 3 秒。隐藏窗口或非 SSH 页面不新增延迟采样。

## 开发说明

主体读取 `/proc/stat`、`loadavg`、`meminfo`、`net/dev`、路由、`uptime`、进程 stat、主机/内核和 mountinfo；发行版从 `/etc/os-release` 解析字面值。固定 `/bin/sh` 脚本使用 shell 内建，不调用 `ps`、`top`、`df` 等外部采集工具，也不执行服务器返回的文本。

进程原始记录预算为 512 KiB，全部记录留在既有 1 MiB Exec 输出上限内。根盘容量使用短生命周期的独立 SFTP subsystem，不复用文件浏览或传输 client。

源码入口：[monitor_collect.go](../internal/desktop/monitor_collect.go)、[monitor_parse.go](../internal/desktop/monitor_parse.go)、[status_backend.go](../internal/desktop/status_backend.go) 和 [ssh_latency.go](../internal/desktop/ssh_latency.go)。验证边界统一见[已知限制](limitations.md)。
