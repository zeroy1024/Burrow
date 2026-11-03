# 开发与构建

## 环境与运行

准备 mise，从仓库根目录运行；各系统的实际运行验收状态见[平台支持](platform-support.md)：

```sh
mise install
mise exec -- go run .
```

[mise.toml](../mise.toml) 固定 Go 1.27.3 与 uv 0.13.6；[go.mod](../go.mod) 固定 MyGo v0.7.3 及项目依赖。沿用这些版本和现有依赖管理方式。

普通 `go run` 使用 Burrow Dev 数据目录与独立凭据命名空间，正式打包使用 Burrow。数据模型、备份及旧格式边界见[数据与凭据](storage.md)。SQLite 驱动固定为 `modernc.org/sqlite v1.61.1`，不要求系统 SQLite 或 CGo。

首次运行或构建需要下载固定终端插件的原生 Ghostty VT 库，后续使用缓存。Go 模块名仍为 `sshdesk`，不影响产品名称 Burrow。

## 打包

```sh
mise run build
```

任务调用 [scripts/build.py](../scripts/build.py)，默认构建当前主机的系统和架构，也可指定多个目标：

```sh
mise run build -- --platform windows/amd64
mise run build -- --platform windows/amd64,windows/arm64
```

支持的构建目标为 `darwin`、`windows`、`linux` 的 `amd64` / `arm64`。macOS 完整打包必须在 macOS 上运行，沿用 [scripts/build-macos.py](../scripts/build-macos.py)：MyGo 生成应用，固定 `dmgbuild==1.6.7` 生成 DMG，系统 `ditto` 生成 ZIP。uv 在首次执行时准备兼容的 Python（3.10+）及脚本依赖。

名称、slug、版本和输出目录来自 [mygo.json](../mygo.json)。以当前 0.1.1 的 Apple Silicon 构建为例：

```text
build/darwin-arm64/Burrow.app
build/darwin-arm64/burrow-0.1.1-arm64.zip
build/darwin-arm64/burrow-0.1.1-arm64.dmg
```

Windows 使用 MyGo 的 EXE/安装程序构建流程，再生成同时包含 EXE 与 `ghostty-vt.dll` 的便携 ZIP：

```text
build/windows-amd64/Burrow.exe
build/windows-amd64/ghostty-vt.dll
build/windows-amd64/burrow-0.1.1-windows-amd64.zip
```

Windows 本机构建时，MyGo 会准备所需 NSIS；从 macOS/Linux 交叉打包安装程序需要主机提供 `makensis`。缺少 NSIS 时仍生成 EXE 与便携 ZIP，并明确报告未生成安装程序，不将它算作安装包验证通过。安装程序名称为 `burrow-setup-0.1.1-amd64.exe`，ARM64 对应目录及文件名使用 `arm64`。

仅构建应用包：

```sh
mise exec -- go tool mygo build -skip-dmg
```

DMG 使用 [assets/dmg/background.tiff](../assets/dmg/background.tiff) 的浅色背景和 Applications 快捷方式，支持拖拽安装；ZIP 保留应用包结构及资源信息。发布版本时需同时核对 `mygo.json` 与 [settings_licenses.go](../internal/desktop/settings_licenses.go) 的版本显示。

## 已有检查

在仓库根目录运行：

```sh
mise exec -- go test ./...
mise exec -- go test -race ./...
mise exec -- go vet ./...
mise exec -- go build ./...
git diff --check
```

本地终端扩展是嵌套 Go module，不会随根目录的 `./...` 自动运行其测试。修改该模块时另行执行：

```sh
mise exec -- go -C third_party/mygo-terminal test ./...
mise exec -- go -C third_party/mygo-terminal test -race ./...
mise exec -- go -C third_party/mygo-terminal vet ./...
mise exec -- go -C third_party/mygo-terminal build ./...
```

只读格式检查可使用 `gofmt -l` 配合本次修改的 Go 文件；不要通过全仓格式化覆盖无关改动。暂存后执行 `git diff --cached --check`，以包含新增文件。

若受限环境无法写入默认 Go 构建缓存，可为相应命令指定任务缓存目录，例如：

```sh
GOCACHE=/private/tmp/sshdesk-go-build mise exec -- go test ./...
```

检查通过只说明本次实际运行的范围。协议、原生窗口和视觉验证的区别见[验证范围](limitations.md#验证如何解读)。

## Gitea CI

两个 workflow 都使用现有的 `ubuntu-latest` runner 标签，在任务容器内准备工具，不修改 runner 宿主机。mise 读取仓库的 `mise.toml`，使用固定的 Go、uv 版本；action 固定到 commit，mise 本体固定为 2026.10.25。

当前 rootless runner 限制容器内的 UID/GID 切换；APT 使用 `APT::Sandbox::User=root`，并将下载缓存放在当前任务的 `RUNNER_TEMP` 下，避免切换到 `_apt` 或访问镜像中 `_apt` 专属目录失败，不增加容器能力或修改宿主机权限。

| Workflow | 触发条件 | 内容 |
|---|---|---|
| [提交检查](../.gitea/workflows/check.yml) | push、PR 和手动触发 | 应用及终端扩展的 Go 格式、Python 语法、现有 race 测试、vet、Linux 编译和 Windows 双架构交叉检查 |
| [编译打包](../.gitea/workflows/package.yml) | main 更新、`v*` 标签和手动触发 | Windows amd64/arm64 的 NSIS EXE 安装程序、便携 ZIP 和 SHA-256 摘要 |

`pkg-sftp` 保留上游格式，不纳入格式门禁；其独立模块执行 vet 时关闭 `stdmethods`：上游的 `RawPacket.ReadFrom` 和 `RequestPacket.ReadFrom` 接收缓冲区与长度限制，并非 `io.ReaderFrom`，该分析项会误报。其余 vet 分析项保留，应用和终端扩展仍执行完整 vet。原生 UI、视觉验收及需要 Docker 服务的 SSH/FTP 集成测试不在这个基础提交检查中。

打包任务安装 NSIS 并调用同一 `mise run build` 入口。它会检查两个架构的安装程序和 ZIP 均存在、ZIP 完整且包含本次 EXE/DLL，再生成 `SHA256SUMS`。缺少安装程序时任务失败，不能以只生成便携包算作打包完成。

产物按 `burrow-windows-amd64`、`burrow-windows-arm64` 保存为 Actions 附件，保留 14 天；workflow 不创建 Release 或发布到外部仓库。上传使用 Runner 5 的 action 兼容层，需保留默认的 `runner.patch_actions: true`。标签名不会自动修改 `mygo.json` 的版本，创建版本标签前先更新项目版本。

当前只有 Linux runner，打包 workflow 覆盖 Windows EXE/ZIP；macOS APP/DMG/ZIP 继续在 macOS 构建，MSI 尚未接入。CI 成功不替代 Windows 原生安装、升级、卸载和功能验收。

可在 macOS/Linux 上进行目标编译与静态检查，例如：

```sh
GOOS=windows GOARCH=amd64 mise exec -- go build ./...
GOOS=windows GOARCH=amd64 mise exec -- go vet ./...
```

交叉编译不能运行 Windows 原生测试。应在 Windows 上另行执行已有检查和[原生验收清单](platform-support.md#windows-原生验收)；当前自动化窗口驱动仍为 macOS 专用。

## 协议与原生 E2E

[dev/ssh-integration](../dev/ssh-integration/) 和 [dev/ftp-integration](../dev/ftp-integration/) 提供 Docker 夹具，启动入口为 [scripts/ssh-integration-server.py](../scripts/ssh-integration-server.py)。它们使用本机随机端口和临时凭据，不应连接现有私有站点。

配置夹具输出的 `SSHDESK_TEST_ADDRESS`、`SSHDESK_TEST_DIR` 和需要的 `SSHDESK_FTP_ADDRESS` 后，协议测试入口为：

```sh
mise exec -- go test -race -tags=integration ./... -count=1 -timeout=180s
```

原生驱动入口为 `python3 -B -u scripts/ssh-integration-ui.py`，提供 `--concurrent-split`、`--file-protocols` 和 `--core-e2e`。需要已解锁且可获得键盘焦点的 macOS 图形会话；与 SDK Docker 测试分开运行，避免连接计数互相干扰。

参数、隔离目录、覆盖断言和夹具停止方式统一维护在[联调与原生 E2E](../dev/ssh-integration/README.md)。

## 修改入口

代码职责见[架构与生命周期](architecture.md)。主题修改使用[生成流程](theme-integration.md#修改与生成)，不要手改派生文件；本地依赖修改先阅读各自的 `FORK.md`。文档只描述当前行为，阶段验证另存[历史归档](archive/README.md)。
