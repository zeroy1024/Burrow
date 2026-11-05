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

## 界面图标

通用界面图标使用固定版本的 Lucide SVG，素材、来源与许可见 [Lucide 素材说明](../internal/uikit/icons/lucide/README.md)。统一通过 `uikit.IconName`、`IconSVG`、`Icon`、`IconButton` 使用，沿用主题的尺寸、颜色和描边设置；业务页面不再单独定义通用 SVG 路径。应用、操作系统与 MCP 品牌标识独立保留。

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

Windows 打包要求主机 PATH 上有 UPX 5.3.0 或更新版本，以支持 Windows ARM64。构建入口会临时编译 [Go 命令代理](../scripts/upx-go/main.go)，在 MyGo 的 Windows `go build` 完成后执行 UPX LZMA 压缩与完整性校验，再由 MyGo 签名并生成安装程序。压缩保留图标和重定位信息，`ghostty-vt.dll` 保持原样；不修改 Go 工具链或 MyGo 源码。构建期间只在子进程 PATH 中启用代理。

本地构建仍生成同时包含压缩后的 EXE 与 `ghostty-vt.dll` 的便携 ZIP：

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

`mygo.json` 的资源列表将根目录 `LICENSE` 与完整 `THIRD_PARTY_NOTICES.md` 一并安装到应用资源目录：macOS 为 `.app/Contents/Resources/`，Windows/Linux 为可执行文件所在目录。新增或更新依赖许可后先运行 `python3 scripts/licenses.py`，再构建；GitHub CI 会校验汇总内容以及打包资源。

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

## GitHub CI

[提交检查](../.github/workflows/check.yml)在分支 push、PR 和手动触发时运行，使用托管 `ubuntu-24.04`。检查范围沿用 Gitea：Go 格式、Python 语法、根模块与终端扩展的已有 race 测试和 vet、Linux 编译、Windows 双架构交叉编译；另检查第三方许可汇总，并使用固定版本、校验 SHA-256 的 Gitleaks 扫描工作文件和检出仓库的全部历史引用。检出保留完整历史且不持久化凭据；工作流仅授予 `contents: read`，使用普通 `pull_request`，无需提供自定义仓库 Secret。

[编译打包](../.github/workflows/package.yml)仅由 `v*` 标签触发，并要求标签与 `mygo.json` 一致，例如版本 `0.1.1` 对应 `v0.1.1`。Windows 使用 `ubuntu-24.04` 交叉构建 amd64/arm64 的 UPX NSIS 安装程序；macOS 使用 Apple Silicon `macos-15` 构建 ARM64 DMG。安装程序校验包括 EXE、DLL 和两个许可文件，macOS 校验 APP 内许可资源及 DMG。各附件附有 `SHA256SUMS`，保留 14 天；工作流不创建或更新 GitHub Release。

Actions 均固定到提交 SHA，工具链沿用 `mise.toml`；APT 使用托管 Ubuntu runner 的 `sudo`，不复用 Gitea 的 rootless 容器设置。原生窗口、协议 Docker 联调、Windows 安装/升级/卸载、签名和公证状态需要单独验收，不能从工作流成功推断。

### 公开与发布前检查

- 更新 `mygo.json` 和关于页的版本号，生成并检查第三方声明：`python3 scripts/licenses.py --check`。
- 使用 Gitleaks 8.30.1 或兼容版本在本地扫描计划公开的提交历史和工作文件；额外检查所有本地 refs，确认不会带出 stash、内部快照或未准备公开的分支。扫描结果留在仓库外并开启完整脱敏。

  ```sh
  gitleaks git --log-opts='HEAD --full-history' --redact=100 --no-banner .
  gitleaks git --log-opts='--all --full-history' --redact=100 --no-banner .
  gitleaks dir --redact=100 --no-banner .
  ```

- 人工确认截图、二进制素材、提交作者邮箱、历史提交消息和诊断材料的公开范围。正则扫描不能证明这些内容无敏感信息；SQLite、私钥、钥匙串内容和原始 MCP 审计不得作为问题附件公开。
- 在 GitHub 启用私密漏洞报告、Secret scanning / Push protection，并为主分支配置所需检查；这些是仓库设置，不由本地文件自动启用。
- 首次只推选定分支及必要标签，避免 `--mirror` 把内部 refs 一并公开。确认 GitHub 检查及产物校验通过后，再人工整理 Release 附件和版本说明，注明实际平台验收及签名状态。

## Gitea CI

检查和 Windows 打包使用现有的 `ubuntu-latest` runner，在任务容器内准备工具。macOS DMG 使用 `macos-arm64` 的原生 host runner；需要另行注册带有 `macos-arm64:host` 标签的 Apple Silicon macOS runner，主机需有 Xcode Command Line Tools 和运行 JavaScript actions 所需的 Node。该 runner 尚待配置，在其上线前 macOS 作业会等待匹配的 runner。mise 读取仓库的 `mise.toml`，使用固定的 Go、uv 版本；action 固定到 commit，mise 本体固定为 2026.10.25。

当前 rootless runner 限制容器内的 UID/GID 切换；APT 使用 `APT::Sandbox::User=root`，并将下载缓存放在当前任务的 `RUNNER_TEMP` 下，避免切换到 `_apt` 或访问镜像中 `_apt` 专属目录失败，不增加容器能力或修改宿主机权限。

| Workflow | 触发条件 | 内容 |
|---|---|---|
| [提交检查](../.gitea/workflows/check.yml) | 分支 push、PR 和手动触发；标签 push 不触发 | 应用及终端扩展的 Go 格式、Python 语法、现有 race 测试、vet、Linux 编译和 Windows 双架构交叉检查 |
| [编译打包](../.gitea/workflows/package.yml) | 仅 `v*` 标签 push | Windows amd64/arm64 的 UPX NSIS 安装程序、macOS ARM64 DMG 和 SHA-256 摘要 |

`pkg-sftp` 保留上游格式，不纳入格式门禁；其独立模块执行 vet 时关闭 `stdmethods`：上游的 `RawPacket.ReadFrom` 和 `RequestPacket.ReadFrom` 接收缓冲区与长度限制，并非 `io.ReaderFrom`，该分析项会误报。其余 vet 分析项保留，应用和终端扩展仍执行完整 vet。原生 UI、视觉验收及需要 Docker 服务的 SSH/FTP 集成测试不在这个基础提交检查中。

Windows 打包任务安装 NSIS、7-Zip，并下载固定 UPX 5.3.0 的官方 Linux 主机工具，验证预置 SHA-256 后调用同一 `mise run build` 入口。它会解开两个架构的安装程序，核对其中的 EXE/DLL 与本次构建完全一致，再生成 `SHA256SUMS`。缺少安装程序时任务失败，不能以只生成便携包算作打包完成。

macOS 继续使用原有 APP 签名、LZMA DMG 和 ZIP 构建流程，不启用 UPX 或文件系统透明压缩。CI 只上传校验通过的 DMG 与摘要。

产物按 `burrow-windows-amd64`、`burrow-windows-arm64`、`burrow-macos-arm64` 保存为 Actions 附件，分别包含安装程序或 DMG 以及摘要，保留 14 天；本地生成的便携 ZIP 不上传，workflow 不创建 Release 或发布到外部仓库。上传使用 Runner 5 的 action 兼容层，需保留默认的 `runner.patch_actions: true`。标签名不会自动修改 `mygo.json` 的版本，创建版本标签前先更新项目版本。

UPX 内部完整性检查和安装程序内容校验不能替代 Windows 原生启动、DLL 加载、安装、升级、卸载及杀软误报检查。macOS host runner 的 DMG 构建与安装亦需在其上线后验证；MSI 尚未接入。

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
