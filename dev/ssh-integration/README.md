# Docker SSH 联调

此环境验证真实 SSH 传输和 macOS 原生应用窗口，不连接项目中的示例主机。

## 启动服务器

在仓库根目录运行：

```sh
python3 scripts/ssh-integration-server.py
```

脚本构建 Ubuntu 24.04 / OpenSSH 镜像，启动带 `dev.sshdesk.integration=true` 标签的独立容器，只将 SSH 端口随机映射到 `127.0.0.1`。不挂载主机目录或 Docker 卷。容器含完整 `vim`、`htop` 和 `top`，SSH 登录环境使用 `C.UTF-8`，用户名为 `sshdesk`。

SSH 已开启 `internal-sftp`；用户目录包含 `hello.txt`、`你好.txt`、`nested/inside.txt` 和不可读的 `denied/`，用于目录与权限验证。

最后一行 JSON 提供 `container`、`address` 和 `directory`。密码、普通私钥、加密私钥及其口令在该临时目录生成；目录权限为 `0700`，秘密文件权限为 `0600`，不写入仓库或构建参数。容器只接收临时公钥和通过标准输入设置的密码。

将 JSON 中的值用于两个任务环境变量：

```sh
export SSHDESK_TEST_ADDRESS='127.0.0.1:映射端口'
export SSHDESK_TEST_DIR='/private/tmp/sshdesk-integration-临时目录'
```

使用同一临时密码另起 FTP 服务器：

```sh
python3 -B scripts/ssh-integration-server.py --ftp
export SSHDESK_FTP_ADDRESS='127.0.0.1:FTP映射端口'
```

FTP 使用独立的 vsftpd 容器和 init 进程托管，控制端口及五个被动数据端口都只映射到本机。夹具允许已认证用户读写，每条连接限速 1 MiB/s，为暂停、取消和恢复提供可观测的传输窗口。SSH 元数据保存在临时目录的 `server.json`，FTP 元数据保存在 `ftp-server.json`；两容器共享测试密码，连接与会话独立。旧测试中直接以 PID 1 运行 vsftpd 曾被独立 Python 客户端复现拒绝登录后退出，当前启动方式已验证错误密码后仍可继续登录。

## 传输层验证

```sh
go test -race -tags=integration ./... -count=1 -timeout=180s
```

覆盖密码、普通私钥、加密私钥和隔离 SSH Agent；错误凭据拒绝；首次主机密钥拒绝、保存、复用及变化拒绝；UTF-8 与原始颜色序列；远端 `stty size` 确认尺寸同步；四会话隔离；128 KiB 输出与退出码；握手等待和已连接会话的取消、关闭。

另覆盖共享连接下多个独立 Shell / SFTP 通道、最后引用释放、挂起创建取消的受控 SSH 测试，以及 SFTP / FTP 中文目录、导航、权限错误、取消和协议解析边界。SFTP 阻塞初始化的边界以受控流模拟，不宣称注入了真实 SSH 零窗口。

文件安全回归还覆盖本地源和目标目录被替换为树外链接后的读写与清理、最终文件替换，以及真实 SFTP OPEN 请求原子携带排他创建和 `0600`。旧 part 的 `0644`、`0400`、`0000` 权限在续传写入前收紧，且保留已确认内容。本地文件回归不需要 Docker；SFTP 安全用例使用同一真实服务。

这些测试使用 `integration` build tag，普通 `go test ./...` 不会连接 Docker 服务器；不带标签的 SDK 生命周期测试仅使用进程内创建的本地 SSH 服务。Docker 目标必须是本机地址。Go 构建缓存不可写时，可为命令设置 `GOCACHE=/private/tmp/sshdesk-go-cache`。

## 原生窗口验证

```sh
go tool mygo build -skip-dmg
python3 -B -u scripts/ssh-integration-ui.py
python3 -B -u scripts/ssh-integration-ui.py --concurrent-split
python3 -B -u scripts/ssh-integration-ui.py --file-protocols
python3 -B -u scripts/ssh-integration-ui.py --core-e2e
```

脚本通过 Go overlay 启用固定 MyGo 版本的 automation 插件，只在临时构建中将应用配置路径指向独立目录；不修改 `HOME`、正式构建或现有 SSH Desk 配置。固定 SDK 复制到测试临时目录后通过独立 modfile 引用，不修改全局模块缓存或项目依赖。用户数据、旧配置迁移路径、本地默认目录、缓存和日志均隔离。测试专用凭据 provider 以 `0600` 文件保存临时秘密，保留应用的凭据调用与版本校验；这些测试不覆盖 macOS 钥匙串集成。

它启动真实 macOS 窗口，以本地 Unix socket 发送点击、键盘和菜单操作，并从实际 Ghostty 实例读取文字与网格。Snapshot 只读观测应用状态；不拍截图或进行视觉验收。

固定 SDK 的 automation 插件默认在后台格式化任意 Key；本轮已由 race 日志确认，指针 Key 会反射读取正在布局的对象。测试 overlay 将指针 Key 仅编码为类型和地址，避免这条竞争及对象内容披露；标量 Key、UI 行为和 race 检查保持原契约。失败诊断仅保存节点标签、角色、状态和几何信息，不保存输入框值或 Key 对象内容。

覆盖认证弹窗、首次信任拒绝与确认、终端输入、`vim` 日文编辑与 `top`、分屏默认键盘焦点、独立 Shell、尺寸变化、关闭窗格、断开、重连历史及四种认证。尺寸检查同时比较应用状态、Ghostty 网格和远端 `stty size`。脚本使用自己启动的 SSH Agent，仅加载临时公钥对应的密钥；结束时关闭自己启动的应用与 Agent。

默认模式还验证两套目录的增改删隔离、旧目录迁移为 `sessions.json` v1 和备份、运行会话身份快照，以及同连接分屏不增加 TCP 或认证。`--concurrent-split` 验证同一次操作创建三个已连接通道、单 TCP / 单次认证及退出清理；该模式通过测试菜单调用拆分动作，属于模型与协议集成，不能代替真实分屏输入验证。

`--file-protocols` 通过当前保存站点 UI 验证 SFTP / FTP 中文条目、目录导航与历史、权限错误后的连接可用性、文件操作控件、本地面板切换、错误 FTP 密码拒绝后的重新连接，以及 SSH / SFTP / FTP 独立关闭和实际连接释放。当前界面没有旧脚本依赖的跨页快捷按钮，该模式不计入临时快捷视图和跨页 transport 复用覆盖；共享 SSH / SFTP 通道生命周期仍由 Go 协议集成测试验证。

`--core-e2e` 通过生产界面的双击、表单、键盘、真实指针拖拽和应用退出菜单驱动操作，测试菜单仅用于焦点和只读观测。它覆盖：

- SSH 分屏的新 Shell 环境隔离、默认键盘焦点、关闭一个窗格后的原会话存活，以及拖动标签时关闭和重新打开。
- SFTP / FTP 双面板拖拽上传与下载，以实际服务器及本地文件的 SHA-256 核对结果；拖动中切页不会改变来源或意外创建任务。
- 真实 FTP 下载的暂停、继续、取消和部分文件清理，同一保存站点的两个标签分别显示和管理任务。
- 暂停受控 FTP 容器，使刷新应答在按住文件行时到达；释放前保留源，取消的 Drop 不产生传输。无论断言是否通过，都会解除本用例的容器暂停。
- 正常退出后启动同一个测试应用、复用同一隔离配置，检查任务按原标签离线恢复、确认偏移不变、显式继续后的实际内容和部分文件清理。

核心模式和文件模式均需要 `SSHDESK_FTP_ADDRESS`。每轮核心模式在两个容器内使用独立目录，可以重跑而不复用上轮目标文件。启动器分别检查每次原生进程的退出记录、race 日志以及 SSH / FTP 连接释放，并在本轮 `native-run-*/result.json` 写入结果；中途失败不能记为通过。

原生驱动会比较服务器连接数与认证次数，运行时不要并行执行 SDK Docker 测试，以免其他测试连接干扰统计。确认驱动成功需要真实应用的退出状态及竞态日志检查，不能只依据 LaunchServices `open` 的退出码。

脚本使用当前 arm64 应用包作为原生库和图标的资源模板，以带竞态检测的 overlay 可执行文件创建独立测试应用。只生成 overlay 而不启动窗口时，可加 `--prepare-only`。原生联调需要已解锁且可以获得键盘焦点的 macOS 图形会话；锁屏时脚本会快速报错。每次运行使用新的隔离配置，原生应用日志和观测文件保存在临时目录，不包含认证输入。

## 运行资源

服务器在脚本结束后继续运行，便于手动复验。可用下面命令查看本任务容器；需要停止时，使用启动结果中的确切容器名：

```sh
docker ps --filter label=dev.sshdesk.integration=true
docker stop 容器名
```

不要使用项目之外的服务器或现有凭据运行这些自动化测试，也不要将临时凭据或日志加入 Git。
