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

FTP 使用独立的 vsftpd 容器和 init 进程托管，控制端口及五个被动数据端口都只映射到本机。SSH 元数据保存在临时目录的 `server.json`，FTP 元数据保存在 `ftp-server.json`；两容器共享测试密码，连接与会话独立。旧测试中直接以 PID 1 运行 vsftpd 曾被独立 Python 客户端复现拒绝登录后退出，当前启动方式已验证错误密码后仍可继续登录。

## 传输层验证

```sh
go test -race -tags=integration ./... -count=1 -timeout=180s
```

覆盖密码、普通私钥、加密私钥和隔离 SSH Agent；错误凭据拒绝；首次主机密钥拒绝、保存、复用及变化拒绝；UTF-8 与原始颜色序列；远端 `stty size` 确认尺寸同步；四会话隔离；128 KiB 输出与退出码；握手等待和已连接会话的取消、关闭。

另覆盖共享连接下多个独立 Shell / SFTP 通道、最后引用释放、挂起创建取消的受控 SSH 测试，以及 SFTP / FTP 中文目录、导航、权限错误、取消和协议解析边界。SFTP 阻塞初始化的边界以受控流模拟，不宣称注入了真实 SSH 零窗口。

这些测试使用 `integration` build tag，普通 `go test ./...` 不会连接 Docker 服务器；不带标签的 SDK 生命周期测试仅使用进程内创建的本地 SSH 服务。Docker 目标必须是本机地址。Go 构建缓存不可写时，可为命令设置 `GOCACHE=/private/tmp/sshdesk-go-cache`。

## 原生窗口验证

```sh
go tool mygo build -skip-dmg
python3 -B -u scripts/ssh-integration-ui.py
python3 -B -u scripts/ssh-integration-ui.py --concurrent-split
python3 -B -u scripts/ssh-integration-ui.py --file-protocols
```

脚本通过 Go overlay 启用固定 MyGo 版本的 automation 插件，只在临时构建中将应用配置路径指向独立目录；不修改 `HOME`、正式构建或现有 SSH Desk 配置。它启动真实 macOS 窗口，以本地 Unix socket 发送点击、键盘和菜单操作，并从实际 Ghostty 实例读取文字与网格；不拍截图或进行视觉验收。

覆盖认证弹窗、首次信任拒绝与确认、终端输入、`vim` 日文编辑与 `top`、分屏默认键盘焦点、独立 Shell、尺寸变化、关闭窗格、断开、重连历史及四种认证。尺寸检查同时比较应用状态、Ghostty 网格和远端 `stty size`。脚本使用自己启动的 SSH Agent，仅加载临时公钥对应的密钥；结束时关闭自己启动的应用与 Agent。

默认模式还验证两套目录的增改删隔离、v2 迁移与备份、运行会话身份快照，以及同连接分屏不增加 TCP 或认证。`--concurrent-split` 验证同一次操作创建三个已连接通道、单 TCP / 单次认证及退出清理；不将额外的逐窗格键盘焦点自动化回归计为通过项。`--file-protocols` 验证真实文件页的临时与保存站点、目录浏览 / 权限错误 / 只读控件、SSH / SFTP 独立关闭和复用，以及 FTP 显式 SSH 目标选择。文件模式需要 `SSHDESK_FTP_ADDRESS`。

原生驱动会比较服务器连接数与认证次数，运行时不要并行执行 SDK Docker 测试，以免其他测试连接干扰统计。确认驱动成功需要真实应用的退出状态及竞态日志检查，不能只依据 LaunchServices `open` 的退出码。

脚本使用当前 arm64 应用包作为原生库和图标的资源模板，以带竞态检测的 overlay 可执行文件创建独立测试应用。只生成 overlay 而不启动窗口时，可加 `--prepare-only`。原生联调需要已解锁且可以获得键盘焦点的 macOS 图形会话；锁屏时脚本会快速报错。每次运行使用新的隔离配置，原生应用日志和观测文件保存在临时目录，不包含认证输入。

## 运行资源

服务器在脚本结束后继续运行，便于手动复验。可用下面命令查看本任务容器；需要停止时，使用启动结果中的确切容器名：

```sh
docker ps --filter label=dev.sshdesk.integration=true
docker stop 容器名
```

不要使用项目之外的服务器或现有凭据运行这些自动化测试，也不要将临时凭据或日志加入 Git。
