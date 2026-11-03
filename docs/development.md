# 开发与构建

## 环境与运行

在 macOS 13 及以上系统准备 mise，从仓库根目录运行：

```sh
mise install
mise exec -- go run .
```

[mise.toml](../mise.toml) 固定 Go 1.27.3 与 uv 0.13.6；[go.mod](../go.mod) 固定 MyGo v0.7.3 及项目依赖。沿用这些版本和现有依赖管理方式。

首次运行或构建需要下载固定终端插件的原生 Ghostty VT 库，后续使用缓存。Go 模块名仍为 `sshdesk`，不影响产品名称 Burrow。

## 打包

```sh
mise run build
```

任务调用 [scripts/build-macos.py](../scripts/build-macos.py)：先由 MyGo 构建应用，再通过固定的 `dmgbuild==1.6.7` 生成 DMG，并使用系统 `ditto` 打包 ZIP。uv 在首次执行时准备兼容的 Python（3.10+）及脚本依赖。

名称、slug、版本和输出目录来自 [mygo.json](../mygo.json)，架构来自当前 Go 工具链的 `GOARCH`。以当前 0.1.1 的 Apple Silicon 构建为例：

```text
build/darwin-arm64/Burrow.app
build/darwin-arm64/burrow-0.1.1-arm64.zip
build/darwin-arm64/burrow-0.1.1-arm64.dmg
```

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
