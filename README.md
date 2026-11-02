# SSH Desk

使用 Go 1.27.3 / MyGo v0.7.3 构建的原生 SSH、SFTP、FTP/FTPS 桌面应用。当前分支保留 UIKit 原型的三页布局、独立目录和拖拽交互，接入真实连接、终端、文件操作、传输队列与 Linux 监控。

## 运行与构建

```sh
mise install
mise exec -- go run .
mise run build
```

真实原生终端要求 macOS 13 或更新系统。首次运行或构建由固定 MyGo 插件获取其原生 Ghostty VT 库。应用不预填演示服务器，不在启动或恢复工作区时自动连接。

在 macOS 上执行 `mise run build`，一次生成裸 `.app`、ZIP 和 DMG 三种产物；ZIP 保留应用包结构及 macOS 资源信息。MyGo 构建应用，固定版本的 `dmgbuild` 生成自定义 DMG；`mise install` 安装 Go 和 uv，uv 在首次构建时准备兼容的 Python（3.10+）和脚本依赖。名称、版本和输出目录读取 `mygo.json`，架构使用当前 Go 工具链的 `GOARCH`。按当前配置，Apple Silicon 构建产物为：

```text
build/darwin-arm64/SSH Desk.app
build/darwin-arm64/ssh-desk-0.1.0-arm64.zip
build/darwin-arm64/ssh-desk-0.1.0-arm64.dmg
```

DMG 固定使用 `assets/dmg/background.tiff` 中的 Burrow 浅色背景，包含 1x/2x 多分辨率图像以适配 Retina。窗口为 660×400，应用与 Applications 快捷方式分别位于 `(170, 180)` 和 `(490, 180)`，支持拖拽安装；背景不会随安装者的系统外观切换。仅需生成 `.app` 时，仍可使用 `mise exec -- go tool mygo build -skip-dmg`。

## 已接通的功能

- SSH：密码、托管私钥、加密私钥、SSH agent、跳板链、PTY/窗口大小、真实键盘输入输出、Unicode/IME、控制序列、备用屏幕、选择和复制粘贴。环境变量、初始命令、Keepalive 与每会话配色实际传入连接/终端。
- 主机信任：应用独立 `known_hosts`；未知指纹在认证前确认，已变更或撤销的密钥拒绝连接。测试连接与正式连接共用认证和信任规则。
- 会话：单击侧栏只聚焦，双击新开独立会话；SSH 与文件页都允许同一条目打开多次。组合拆分、窗格移动和拖回标签只移动现有对象；分屏新 Shell 复用原 transport 的独立 channel。
- 文件：真实本地、SFTP、FTP/FTPS 浏览；目录历史、面包屑、过滤、排序、元数据、新建文件夹、重命名与递归删除。文件页和 SSH 页独立保存条目，删除仅作用于对应目录。
- FTP：plain、显式/隐式 TLS；主动/被动数据连接；七种界面已提供的文件名编码。TLS 证书正常验证，不降级为不验证；主动模式要求服务器能回连本机通告地址。
- 传输：文件及目录真实上传/下载、实际字节/速度/总量、并发上限、暂停/继续、取消/重试、递归冲突策略和目标定位。任务按具体文件标签隔离，离线恢复仍显示原标签任务；关闭启动恢复会话后，新标签不继承旧任务。传输栏“找回任务”可重新打开未恢复任务的离线标签，供用户继续或取消清理。每个任务使用独立 client/subsystem，不随浏览标签关闭或切页取消。
- 文件写入：任务独有随机 `.part`，成功关闭流且目标提交成功后才完成。继续前校验源基线及真实偏移；不支持 REST 或替换 Rename 的服务器明确报错，保留原文件和可追踪的部分文件。
- 恢复：保存完整分屏树、比例、焦点及文件标签；恢复为离线状态。正常退出先暂停并停稳传输，再原子保存队列、递归清单和 `.part` 记录；重启后由用户继续或取消清理。
- 延迟：通过现有 SSH 连接独立测量应用层往返时间，当前可见终端每5秒刷新，同连接组共享结果；与监控开关无关，超时/过期显示未知。
- 监控：通过当前 SSH 连接与 POSIX shell 内建读取 `/proc`，不调用外部采集工具；发行版直接读 `/etc/os-release`，根盘容量用独立 SFTP `statvfs`。网络只统计默认路由选中的接口，Top 使用 PID＋启动时间的区间 CPU 差分；单项失败显示 `—`，隐藏面板停止采集。数据口径和限制见[监控覆盖清单](docs/monitoring.md)。

SSH、文件传输和设置按平级互斥方式渲染。切页保留连接、输出、终端滚动、目录历史和后台任务；设置关闭仅清理临时 UI 状态，配置、稳定句柄及密钥任务保留。普通关闭窗口隐藏同一个原生窗口，后台任务继续；Dock 激活重新显示。退出应用才收敛任务并释放 transport。

## 存储与边界

目录保存到 MyGo 应用用户数据目录的 `sessions.json`，偏好使用既有 `prefs.json`、`files.json`、`terminal.json` 和主题目录。主线旧版 `catalog.json` v0/v2 在新目录不存在时迁移，保留源文件和备份；外部私钥复制到应用托管目录，原件不移动。无内嵌公钥的传统加密 PEM 会在应用窗口就绪后请求口令解锁迁移，完整校验后才发布新目录；取消或失败保留源数据，并通过原错误提示重试。该导入口令不写钥匙串。密钥、密码和私钥口令不写入 JSON；密码存系统钥匙串并绑定配置版本，旧任务不会误取同 UUID 新密码。

正常退出恢复使用 `transfers.json`。这不是逐笔崩溃日志：强制结束进程、系统断电期间新建的部分文件不保证已被记录。递归传输拒绝符号链接；删除使用 Lstat，不跟随链接。

SFTP私有暂存文件通过固定`pkg/sftp v1.13.12`的[最小本地扩展](third_party/pkg-sftp/FORK.md)在创建请求中原子指定0600，确认后才发送内容；FTP无法提供同等保护时拒绝受限读取权限文件的覆盖。上传源、下载目标与本地取消清理在每次执行期间固定所选本地目录句柄，阻止后代目录替换为树外链接后改向读取或写入。

当前队列v3保存已确认偏移和SHA-256摘要，恢复先核对目标part前缀，再从源偏移继续；旧v1/v2首次继续需要一次前缀对照。覆盖已有文件要求真实权限且能实际设置/确认；新目标不强制CHMOD，本地stage从0600创建。目录链接可显式进入，源链接传输仍拒绝；darwin/linux本地打开和递归删除以稳定FD防止链接替换。审查后的14项实际状态与本轮验证边界见[整改清单](docs/mvp-remediation.md)。

真实终端按用户确认的范围处理：字距差异允许忽略，粗体提亮开关已移除，滚动条保留原生的半透明胶囊及滚动/贴边显隐风格。回滚使用固定MyGo v0.7.3终端包的最小本地扩展 `Options.ScrollbackLines` / `SetScrollbackLines`，按物理显示行限制并实时应用于全部已有SSH标签；自动换行分别计数，接受Ghostty按页淘汰带来的误差。缩小上限会丢弃最旧的合格历史页，增大不能恢复已淘汰输出；同一终端对象、SSH连接和后台传输任务不因修改设置而重建或重启。旧配置保留兼容读取，界面结构未重设计；网格与滚动条由MyGo终端组件绘制，Ghostty维护终端状态，未做逐像素等同验收。

完整能力、差异与验证范围见 [MVP 状态](docs/mvp-status.md)、[实施记录](docs/mvp-plan.md)、[文件传输](docs/file-transfer.md)、[UIKit 覆盖](docs/uikit-coverage.md)。

## 检查

```sh
mise exec -- go test ./...
mise exec -- go test -race ./...
mise exec -- go vet ./...
mise exec -- go build ./...
```

真实协议集成 fixture 位于 `dev/ssh-integration`、`dev/ftp-integration`。测试使用受控 loopback 服务及临时凭据，具体环境变量见 `*_integration_test.go`；未配置 fixture 的 skip 不算协议通过。本地与 SFTP 安全回归已固化为测试；原生 E2E 入口为 `python3 -B -u scripts/ssh-integration-ui.py --core-e2e`，需要已解锁的 macOS 图形会话。启动、隔离方式和覆盖范围见[联调与 E2E 说明](dev/ssh-integration/README.md)。没有截图基线或 CI。
