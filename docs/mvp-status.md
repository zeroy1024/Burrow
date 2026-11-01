# MVP 能力和验证状态

以 `7decb9c` 为 UI 基线，合入本地 main `45d14ed`；仓库无远端，因此没有远端拉取或推送。原布局、三页互斥、独立目录及拖放模型保留，正文与事件接入真实后端。

| 能力 | 状态 | 实际实现与消费者 |
|---|---|---|
| SSH真实终端 | 完整接入 | `sshclient` + `ssh_sessions.go/ssh_terminal.go`；终端窗格使用固定 MyGo Ghostty VT View，支持真实输入输出/PTY/resize/控制序列/IME |
| SSH配置 | 完整接入 | 密码/托管key/加密key/agent、完整跳板计划、Env/InitCommand/KeepAlive；正式及测试共用信任/认证 |
| 信任与凭据 | 完整接入 | 独立 known_hosts，认证前确认未知主机，拒绝变更；密码版本匹配，私钥/口令托管及失败补偿 |
| 会话生命周期 | 完整接入 | split新channel复用transport，move/detach/ungroup复用同一session；单channel关闭隔离，最后消费者释放；启动恢复离线 |
| 目录与文件CRUD | 完整接入 | `fileclient` local/SFTP/FTP API；浏览、历史、工具栏、文件操作Modal读取真实结果 |
| FTP配置 | 完整接入 | TLS none/explicit/implicit、passive/active、7种编码；正式连接及测试共用client |
| 传输队列 | 完整接入 | 真实recursive manifest、字节/速度、并发、冲突、pause/resume/cancel/retry、独有part提交；独立任务端点和client |
| 正常退出恢复 | 完整接入 | 收敛worker后保存队列/清单/part；重启离线；格式/路径校验、读取失败保护、保存错误可见 |
| Linux监控 | 完整接入 | 现有全部指标卡片读取真实采样；CPU/net差分；失败/缺工具显示不可用；隐藏即停采样而不关闭SSH |
| 真实终端字体/颜色/光标 | 部分实现 | 每会话scheme、字体/字号/行高、背景、光标通过公开API接入；Tracking/BoldIsBright无公开入口 |
| 精确回滚行数 | 部分实现 | 创建时按行数/列数估算字节预算；SDK不提供精确行数或live setter；配置保存和设置预览保留 |
| 真实终端滚动条统一 | 部分实现 | UIKit滚动区域仍统一；native View 的滚动条由SDK内部绘制，没有公开ScrollState可交给UIKit，不声称同样式 |
| 拖动真实内容 | 完整接入，附边界 | 可见来源pointerdown捕获真实正文视口，后台裁切、单手势缓存；不可见来源/捕获失败用独立真实emulator Snapshot，不制造占位内容。SDK Snapshot不暴露历史视口位置，备用路径不能保证历史中部逐像素同位 |
| 协议之外的后端 | 明确不适用 | 未新增SSH服务端、FTP服务端、更新/反馈服务、遥测；监控仅Linux，其他系统显示不可用 |

“完整接入”指生产事件与真实数据路径已实现，不表示所有页面和设备行为都已实测。固定SDK的显示差异明确列为部分实现，不能由编译通过或函数存在改写为完整。

## 验证证据

- 最终原有 `go test ./...`、`go test -race ./...`、`go vet ./...`、`go build ./...`、gofmt与diff检查通过。MyGo darwin/arm64原生打包与签名通过，产物 `build/darwin-arm64/SSH Desk.app`，18.5MB。
- 已有协议集成：受控 Docker loopback fixture，`go test -race -tags=integration ./internal/sshclient ./internal/fileclient -count=1 -timeout=180s`，0 skip、0 fail。
- 临时额外 smoke：20个不同分支最终通过，包含local真实CRUD/Lstat/取消、SFTP偏移与递归/七种编码、SSH key跳板与独立父连接、Env/InitCommand/PTY/Exec上限取消、Keepalive，以及FTP六组合/不可信CA拒绝。临时程序、服务和秘密已回收，没有新增永久测试。
- 协议日志保留于本轮 `/private/tmp/sshdesk-mvp-verify-zd9q9zub`。它们验证SDK与命令式I/O，未执行应用GUI、队列弹窗和像素验收；不将skip、编译或未运行用例称为通过。

## 使用限制

强制杀进程/断电没有逐笔崩溃日志；正常退出队列才有完整恢复保证。源/目标符号链接传输不支持。FTP主动模式需要可回连网络条件，TLS需要服务器提供可信证书。缺少内嵌公钥的旧加密PEM在无口令迁移时明确失败并保留原数据，不伪造fingerprint。

保持固定SDK和不重设计UI的约束下，本轮选择真实原生终端而不自行重写VT或使用私有接口。上表显示偏好和滚动条差异仍是具体未完成项，不能视为原设计全部能力已在真实终端达到同等效果。
