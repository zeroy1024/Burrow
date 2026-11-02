# Linux 监控覆盖与边界

2026-11-02：保留服务器状态SidePanel、原有指标卡片与显隐交互。主采集经现有SSH transport的独立Exec通道运行固定`/bin/sh`脚本，只使用内建`read`、`printf`、条件/循环、重定向和算术；不调用`ps`、`top`、`df`、`stat`、`uname`、`hostname`、`cat`等外部工具。原始文本用NUL记录分帧，内容不作为shell代码执行，解析、差分及Top排序在客户端完成。

## 指标与消费者

| 能力 | 状态 | 实际来源 / API | 页面消费者与数据口径 |
|---|---|---|---|
| CPU、核心数与趋势 | 完整实现 | `monitorScript`读取`/proc/stat`；`parseMonitorCPU`、`completeMonitorRates` | CPU卡片及Sparkline；聚合CPU排除guest重复计数，idle包含iowait，两次有效采样后显示；核心数变化或计数异常先建立新基线 |
| 负载 | 完整实现 | `/proc/loadavg` | CPU卡片的1/5/15分钟负载 |
| 内存 | 完整实现 | `/proc/meminfo` | 内存卡片；`MemTotal - MemAvailable`，不把缓存全部算作已用；缺少所需字段显示— |
| 网络速率 | 完整实现（默认路由接口范围） | `/proc/net/dev`、`/proc/net/route`、`/proc/net/ipv6_route`、`/proc/uptime`；`monitorRouteInterface` | 网络上下行；优先有效IPv4默认路由、否则IPv6，选择最低metric的唯一接口；同接口两次采样相减，路由切换/计数回退重新建立基线 |
| 进程Top | 完整实现（采集预算范围） | `/proc/[pid]/stat`；`parseMonitorProcess`、`completeMonitorRates` | 前4个进程的区间CPU；PID＋启动时间识别，不混入复用PID；单核满载100%，多线程可超过100%；新进程等待下一次有效采样，退出/不可读记录不参与排名 |
| 进程数量 | 完整实现（当前账号可见范围） | `/proc/[0-9]*`目录 | 系统信息“进程”；与Top采集预算独立，数量不因Top超限截断 |
| 运行时间 | 完整实现 | `/proc/uptime` | 系统信息“运行时间” |
| 主机名、内核 | 完整实现 | `/proc/sys/kernel/hostname`、`/proc/sys/kernel/osrelease` | 系统信息，缺失为— |
| 发行版名称 | 完整实现（已接受的例外） | `/etc/os-release`；`monitorOSName`只解析`PRETTY_NAME`字面值 | 系统信息“系统”；不source、不展开变量、不猜测缺失的发行版 |
| 根盘设备/文件系统类型 | 完整实现 | `/proc/self/mountinfo`；`parseMonitorRoot` | 磁盘卡片说明，处理叠加根挂载和路径转义 |
| 根盘容量 | 完整实现（已接受的可选例外） | `fileclient.FilesystemStatter.StatFilesystem`、`sftpClient.StatFilesystem`、`readMonitorDisk` | 磁盘卡片；SFTP服务器需声明statvfs扩展，bytes=`Frsize × Blocks`，已用=`Total - Free`，百分比为已用/总量；缺扩展、权限不足、超时或根挂载不一致均为— |
| 采集失败隔离 | 完整实现 | `parseMonitorSample`按记录解析，`monitorNotice`按项列出不可用；磁盘有独立调度/截止时间 | 某项字段缺失/不合法不清空其他卡片；整个Exec失败或帧格式损坏时，本次/proc数据不可用，不显示旧数值冒充当前值 |
| 连接与生命周期 | 完整实现 | `monitorState.connection/ctx/generation`、`updateMonitor`、`stop` | 同一transport切换Shell保留趋势/基线；新Shell建立中暂停新增采样；切换transport重置；关面板、切页、隐藏窗口取消自有采集通道，不关闭SSH transport、浏览器SFTP或后台传输client |

## 范围与预算

网络表示选中默认路由接口上的计数，不是所有接口的合计，也不声称是全机或所有策略路由的总吞吐。没有明确默认接口、等成本默认路由存在歧义时显示—，不会退回累加桥接/VLAN/veth。正常采样时不展示底部说明字样，保留首次采集及异常提示；内核版本使用与其他系统信息值相同的文字颜色。

`/proc`主体约每2秒采样一次，Exec限时5秒且只保留一个在途请求；实际间隔包含采集耗时。进程原始记录预算512KiB，超过时本轮整个Top显示—，不把截断子集当作真正Top；数量、CPU、内存及其他记录继续可用。固定文件有各自的读取上限，所有预算留在既有1MiB Exec输出上限内。进程可见性及字段读取受SSH账号权限、hidepid和内核配置影响。

磁盘容量约每10秒采样，独立请求限时3秒，使用短生命周期的自有SFTP subsystem，完成即关闭，不重连、不重新认证、不复用活动浏览或传输client。SFTP可能有独立chroot，因此还需通过该subsystem读取`/proc/self/mountinfo`确认根挂载与Exec一致；无法确认时宁可显示—。不支持statvfs扩展时本次面板生命周期不再重复请求；其他错误留待下一轮采样，不改用外部命令。没有SFTP支持不影响其他监控卡片。

## 本轮验证

相关已有`go test ./internal/desktop ./internal/fileclient ./internal/sshclient`通过；最后功能调整后全项目`go test -race ./...`、`go vet ./...`、`go build ./...`通过。修改源码gofmt、tracked及新增文件diff检查通过；`go tool mygo build -skip-dmg`生成18.9MB的darwin/arm64应用并签名，`codesign --verify --deep --strict`通过。终端SDK本轮未改，未重复运行其独立测试。

固定脚本已通过`/bin/sh -n`；在空PATH的macOS环境直接执行，正确返回Linux `/proc`不可用的独立记录，没有外部工具诊断输出。源码核对了NUL帧、每项预算与进程上限、接口切换/计数回退、PID复用、根挂载匹配和各请求取消路径；这些核对不冒称运行时专项断言。

未新增测试或GUI视觉验收。现有测试没有监控解析/调度与statvfs新增能力的专门断言；当前Docker daemon未启动，未运行Linux成功采集、OpenWrt或SFTP statvfs端到端验证，不把编译或非Linux分支通过算成这些验证通过。
