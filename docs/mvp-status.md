# MVP 能力和验证状态

以 `7decb9c` 为 UI 基线，合入本地 main `45d14ed`；仓库无远端，因此没有远端拉取或推送。原布局、三页互斥、独立目录及拖放模型保留，正文与事件接入真实后端。

`73159b6`后审查发现的14项问题与本轮实际API、生产消费者及新验证分列于[整改清单](mvp-remediation.md)。14项修复及已有检查已完成，第14项先受控复现再修；SSH/SFTP/FTP loopback、传输控制器与本地文件验证独立记录。真实OS特殊位跳过、原Docker集成未重跑和GUI未验收的边界如实保留，不沿用历史“完整接入”冒充所有设备已实测。

| 能力 | 状态 | 实际实现与消费者 |
|---|---|---|
| SSH真实终端 | 完整接入 | `sshclient` + `ssh_sessions.go/ssh_terminal.go`；终端窗格使用固定 MyGo Ghostty VT View，支持真实输入输出/PTY/resize/控制序列/IME |
| SSH配置 | 完整接入 | 密码/托管key/加密key/agent、完整跳板计划、Env/InitCommand/KeepAlive；正式及测试共用信任/认证 |
| 信任与凭据 | 完整接入 | 独立 known_hosts，认证前确认未知主机，拒绝变更；密码版本匹配，私钥/口令托管及失败补偿 |
| 会话生命周期 | 完整接入 | split新channel复用transport，move/detach/ungroup复用同一session；单channel关闭隔离，最后消费者释放；启动恢复离线 |
| 目录与文件CRUD | 完整接入 | `fileclient` local/SFTP/FTP API；浏览、历史、工具栏、文件操作Modal读取真实结果 |
| FTP配置 | 完整接入 | TLS none/explicit/implicit、passive/active、7种编码；正式连接及测试共用client |
| 传输队列 | 完整接入，按标签隔离 | 真实recursive manifest、字节/速度、全局并发调度、pause/resume/cancel/retry、独有part提交；列表、统计、批量操作及冲突/失败提示按具体文件标签隔离，离线标签仍显示自己的任务，无标签时为空；独立任务端点和client继续保留 |
| 正常退出恢复 | 按会话开关绑定，附旧记录边界 | 收敛worker后保存v4队列和v2工作区的标签身份；仅RestoreSessions开启且原标签恢复时绑定任务，离线显示、不自动连接/继续；新标签不继承站点历史。兼容旧格式，缺标签身份的旧检查点保留但不猜测归属；已完成记录仅随可恢复标签保存。格式/路径校验、读取失败保护、保存错误可见 |
| SSH延迟 | 完整接入 | `Connection.ProbeLatency`复用现有transport测SSH应用层往返时间；`updateSSHLatency`独立于监控，当前可见Shell按连接组共享，首次可见连接立即采样、每5秒刷新，3秒超时或10秒过期显示未知 |
| 窗口及栏半透明 | 已修正渲染回归，GUI待复测 | `syncWindowMaterial`即时切换原生VibrancyTitlebar，系统降低透明度强制停用；三页标题栏用92%主题Window.Bg着色，保留原灰色外观；真实终端ViewportBackdrop读取固定Ghostty的实际overscan行，使用自身绘制坐标及上下栏外侧圆角裁剪，修正焦点/非焦点不同离屏绘制路径的错位与直角底板；文件TableProps.Backdrop绘制实际条目，SSH上下栏和文件页头/页脚统一ScrollFade多层渐进模式（1/3/5px模糊，92%→78%→0各自背景着色），文件工具栏和列头共用唯一背景，SortHeader.Translucent清除自身底色，关闭半透明时恢复实体背景；原视口、4px留白及输入/拖动身份保留 |
| Linux监控 | 完整接入，附能力边界 | 主体`/proc`、`os-release`及可选SFTP `statvfs`；默认路由单接口、PID＋启动时间差分Top、单指标失败隔离；同transport切Shell保留基线，隐藏取消自有采集通道。实际API、预算及未实测边界见[监控覆盖清单](monitoring.md) |
| 真实终端字体/颜色/光标 | 当前验收范围完整接入 | 每会话scheme、字体/字号/行高、背景、光标通过公开API接入；终端字距遗留参数已移除，粗体提亮设置入口已删除 |
| 实时回滚行数 | 完整实现（接受页级误差） | 本地终端包公开`Options.ScrollbackLines` / `SetScrollbackLines(int) error`；创建时按物理显示行设置，`enforceScrollback`遍历全部SSH标签原地同步，含后台标签与非SSH页面；缩小淘汰最旧历史页，增大不恢复旧输出 |
| 真实终端滚动条风格 | 当前风格范围已有实现 | 原生VT与UIKit均为半透明胶囊、滚动/贴边显示与淡出；按用户要求保留原生实现，不再要求组件或参数完全相同，未做像素验收 |
| 拖动真实内容 | 完整接入，附边界 | 可见来源pointerdown捕获真实正文视口，后台裁切、单手势缓存；不可见来源/捕获失败用独立真实emulator Snapshot，不制造占位内容。SDK Snapshot不暴露历史视口位置，备用路径不能保证历史中部逐像素同位 |
| 协议之外的后端 | 明确不适用 | 未新增SSH服务端、FTP服务端、更新/反馈服务、遥测；监控仅Linux，其他系统显示不可用 |

“完整接入”指生产事件与真实数据路径已实现，不表示所有页面和设备行为都已实测。终端差异按用户明确调整后的范围记录；回滚接受页级误差，不宣称严格逐行。不能由编译通过或函数存在代替实际验证。

## 验证证据

- 最终原有 `go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...`、gofmt与diff检查通过。MyGo darwin/arm64原生打包与签名通过，产物 `build/darwin-arm64/SSH Desk.app`，18.7MB。
- 已有协议集成：受控 Docker loopback fixture，`go test -race -tags=integration ./internal/sshclient ./internal/fileclient -count=1 -timeout=180s`，0 skip、0 fail。
- 临时额外 smoke：20个不同分支最终通过，包含local真实CRUD/Lstat/取消、SFTP偏移与递归/七种编码、SSH key跳板与独立父连接、Env/InitCommand/PTY/Exec上限取消、Keepalive，以及FTP六组合/不可信CA拒绝。临时程序、服务和秘密已回收，没有新增永久测试。
- 协议日志保留于本轮 `/private/tmp/sshdesk-mvp-verify-zd9q9zub`。这一阶段验证SDK与命令式I/O，不将skip、编译或未运行用例称为通过。

## 使用限制

强制杀进程/断电没有逐笔崩溃日志；正常退出会收敛并记录队列；目录续传核验已完成子项，提交不确定时核对状态与摘要，不保证服务器支持的原子语义超过协议能力。源/目标符号链接传输不支持。FTP主动模式需要可回连网络条件，TLS需要服务器提供可信证书。无内嵌公钥的旧加密PEM通过运行时延迟解锁迁移，取消/失败保留原数据并可重试；公钥指纹来自真实解密结果，导入口令不写入配置或钥匙串。

2026-11-02用户允许忽略字距差异、删除粗体提亮开关、接受滚动条相同视觉风格，并明确授权最小MyGo扩展以实现实时回滚、接受页级误差。本地`third_party/mygo-terminal`由固定v0.7.3终端包派生，主MyGo版本与Ghostty内核保持不变；应用创建、拖动快照创建及全标签偏好同步使用真实行数接口。此设置不重建终端、不重连SSH、不操作传输任务；缩小上限可能按合同淘汰最旧历史页，未被淘汰的屏幕、历史、选择与滚动锚点由原内核继续维护。实际API及边界见[终端SDK差异](mvp-sdk-blockers.md)，本轮独立验证记录如下。

## 本轮实时回滚验证（2026-11-02）

- 项目已有`go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...`通过；本地终端子模块已有tests、race、vet亦通过。修改文件gofmt及diff检查通过。
- 重新运行`go tool mygo build -skip-dmg`，darwin/arm64应用生成并签名，`codesign --verify --deep --strict`通过。主MyGo仍v0.7.3；固定Ghostty原生资源manifest逐字节一致，终端插件仅2个文件有功能修改，15个文件只有内部import改名。
- 5组临时真实内核`-race`用例最终全部通过、0 skip/0 race：Terminal/VT/RenderState/Conn身份稳定；未淘汰的视口、选择、活动屏幕保留；缩小立即裁剪、增大允许后续增长；字节/行限制及零值语义；备用屏幕与同步输出；并发Feed/set/Close和关闭后错误；离屏功能帧在无追加输出时刷新缓存与滚动条。证据：[验证汇总](/private/tmp/sshdesk-live-scrollback-p33gieqk/verification-summary.json)。初次缓存权限及两次夹具假设错误的日志保留，没有算作通过；验证副本有效Go代码与生产一致。
- 临时应用overlay直接调用真实`enforceScrollback`，3个后台原生终端在设置/文件页面下同步缩小/增大；终端对象、连接写入/resize/关闭计数、SSH上下文及传输状态哨兵保留，`-race`通过。证据：[应用结果](/private/tmp/sshdesk-scrollback-app-rr_surlg/result.json)。该验证使用内存连接和任务状态哨兵，不冒称本轮重新运行了真实SSH/FTP网络任务。
- 本轮只做受控功能验证，未做GUI视觉验收。页级误差随宽度、样式和内容变化，不保证固定的多出行数；增大上限不能恢复已丢弃历史。临时模块、测试源码和overlay已移入回收站并确认不存在，非敏感日志/hash保留，未新增永久测试、截图基线或CI。


## 第二轮应用控制器验证

`887f4b1`后的复核修复了通道初始化竞争、PTY尺寸覆盖、隐藏监控重启、长初始命令背压、协议切换重连、同批冲突、同名并发、目录续传及不确定提交。

真实MyGo隐藏窗口事件循环运行10个进程用例、11条业务分支，最终退出均0且无race；包含暂停→保存v2 journal→新进程离线恢复→继续及SHA-256校验、取消清理和延迟RNTO回复的不确定提交恢复。证据：[本地汇总](/private/tmp/sshdesk-queue-native-x8expv1d/verification-summary.json)。临时工具/服务/秘密已回收；原失败验证器日志保留并明确未算通过。没有原GUI点击、截图或像素验收。

持有保存目录的浏览视图在显式重连时采用最新配置；后台传输保持入队的端点/跳板/凭据版本快照。重启后两者都不自动连接，不将跨workspace和queue的独立恢复对象说成仍复用同一个活跃transport。


补充复验：一次取消终止同批未完成任务和待处理冲突，保留已完成文件及独立批次，真实 `.part` 清理为零；真实隐藏原生进程 `-race` 退出0。补充证据位于 `/private/tmp/sshdesk-queue-cancel-b5wbz7br/verification-summary.json`，临时服务和文件均已回收。前11条分支没有为凑数量重复运行。


## 旧加密私钥迁移收尾

`catalog_migration.go`将无内嵌公钥的传统加密PEM迁移延迟到窗口就绪，通过原UIKit认证弹窗取得本次口令；worker解密计算真实公钥，复制原加密bytes，原子发布完整目录。取消、错口令、发布失败均补偿未发布文件并提供持续的重试Toast；编辑草稿/密钥任务存在时拒绝替换模型，发布后只清旧catalog临时引用，保留搜索/滚动句柄和会话任务。

真实hidden native `-race`控制器完成4个主场景、7种入口guard和发布前guard，均退出0，SDK Secrets三入口panic trap未触发，原始源文件bytes/mode/mtime未变；再以单独microcase验证两scope的旧分组/拖动引用清理、稳定句柄和新草稿组选择。证据 `/private/tmp/sshdesk-pem-migration-dhiayfcz/runner/result.json` 与 `supplement-manifest.json`。临时controller的两次夹具/构建问题明确保留日志并重试，未算作production通过；没有永久测试或GUI视觉验收。


终端四项差异已按用户后续决定收口：忽略字距差异、删除粗体开关、原生滚动条保持相同风格；实时回滚通过最小终端包扩展实现并接受页级误差。具体API、页面消费者与裁剪边界见[终端SDK差异](mvp-sdk-blockers.md)。

## 独立 SSH 延迟采样（2026-11-02）

`Connection.ProbeLatency(ctx)`计时现有加密连接上的轻量全局请求，成功或不支持回复均有效；与自动KeepAlive共用一个在途请求，调用方超时只结束等待。未回复的底层请求随transport关闭收敛，不额外持有连接lease，不改变最后一个Shell或文件消费者释放transport的规则。KeepAlive原有间隔/超时断线策略保留；关闭自动KeepAlive时，可见终端仍会发送显示延迟所需的探测。

`updateSSHLatency`只调度SSH页面当前可见窗格，隐藏窗口/切到文件或设置/非当前Tab不新增探测；放大窗格时只采样当前窗格所属连接组。首次可见就绪立即采样，后续5秒一次；同组Shell共享结果，移动/拆分不重建样本所属组。3秒超时、迟到超过3秒或10秒过期显示`—`，重新建立transport清除旧样本。监控命令不再写入页脚延迟，监控指标本轮未改。

已有`go test -race ./...`、`go vet ./...`、`go build ./...`通过，MyGo darwin/arm64应用重新打包并签名。未新增测试；现有用例不含新增延迟调度的专门断言，尚未进行真实服务器或GUI交互验收。此后监控改造已按用户接受的`os-release`/SFTP `statvfs`两项例外接入，状态与新验证分列于[监控覆盖清单](monitoring.md)。
