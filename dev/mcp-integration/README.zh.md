# MCP 真实协议与任务生命周期联调

[English](README.md) | [简体中文](README.zh.md)

驱动经过真实本机 MCP HTTP、生产 `mcpDesktop` 适配器、原生窗口线程、既有连接管理和传输队列，最终访问隔离 Docker SSH/SFTP/FTP/FTPS 服务。传输内容通过独立服务器读取和 SHA-256 核对，使用 `-race` 构建；不使用替代后端，不做截图或视觉设计验收。

## 启动夹具

先按 [本机协议联调说明](../ssh-integration/README.zh.md) 运行 SSH 启动器，将最后一行 JSON 的目录写入 `SSHDESK_TEST_DIR`，并设置 `SSHDESK_TEST_ADDRESS`。所有临时凭据与证书保持在 `0700` 夹具目录，不能提交到仓库。

```sh
python3 -B scripts/ssh-integration-server.py
python3 -B scripts/ssh-integration-server.py --ftp
python3 -B scripts/ssh-integration-server.py --ftps
python3 -B scripts/ssh-integration-server.py --ftps --implicit
```

FTPS 始终校验临时 CA 和主机名，不安装系统证书，不使用 `InsecureSkipVerify`。只在测试 overlay 中为原有 `FTPConfig.TLSConfig` 注入本轮 CA。被动端口仅发布到回环地址；连续取消会短时占用多个数据连接，因此夹具预留足够端口，不能将端口耗尽误记为产品续传失败。

## 运行原生 MCP 场景

先构建当前平台的 `Burrow.app` 资源模板，或用 `--template` 指定已有应用包。需已解锁、可获得键盘焦点的 macOS 会话：

```sh
GOCACHE=/private/tmp/sshdesk-go-build mise exec -- python3 -B -u scripts/mcp-integration-ui.py \
  --fixture "$SSHDESK_TEST_DIR" \
  --template /absolute/path/to/Burrow.app
```

生产 UI 不变。现有原生集成 overlay 提供独立 SQLite 数据目录、临时凭据 provider 和仅测试构建存在的控制菜单；暂停/继续还会直接操作实际 MCP 详情按钮。协议调用带完整 MCP 版本元数据与认证头。调试输出只记录 FTP 动词和响应码，不记录参数、密码、密钥或文件正文。

覆盖 SSH 首次信任、输出/退出码、超时与 HTTP 断线取消（新旧 MCP 协议）、无 PTY/不运行终端初始化命令；命令/文件策略与历史快照；四种文件协议的读写、覆盖、目录上传下载；客户端未解码任务 ID 前断开后的任务发现；原生暂停继续、排队/运行取消、来源目录消失后的清理；密钥停用/撤销、授权缩减与跨 Key 隔离；服务停用/开启；审计写故障下禁止新执行但保留取消；正常退出及强制进程崩溃后的离线恢复、人工继续和内容校验。

每轮报告位于夹具目录的 `mcp-native-run-*/report.json`；失败保留各阶段日志和只读节点/模型诊断，不能仅凭构建成功、应用启动或部分场景通过认定完整联调完成。`--reuse-run` 只用于开发中复用已隔离的配置定位问题；最终全量验收必须使用全新配置。`--scenario lifecycle` 可缩小已知生命周期问题的复现范围。

`--scenario recovery` 单独检查正常退出和强制崩溃后的恢复，报告会注明所选场景，不代表其余场景通过。

与原协议 Docker 测试分别运行，避免干扰连接计数与端口容量。停止服务时仅使用本轮 metadata 中的准确容器名，不操作其他容器或用户站点。

2026-11-07 已核对源码适配：共享 SSH probe 及启动、终端 overlay 已适配当前本地化消息类型，两个完整 overlay 均通过 `-race` 编译。本次使用临时夹具进行编译检查，未重跑原生及远端 E2E；下述结果仍为历史记录。

## 2026-11-03 已完成验证

本轮在 macOS arm64、全新隔离 SQLite 和凭据目录下，运行默认 `all` 场景：**30/30 通过**。真实 MCP HTTP 使用 `2026-07-28` 与 `2025-06-18` 取消语义；服务器为本机 OpenSSH/internal-sftp 与 vsftpd，显式/隐式 FTPS 保持证书、主机名和数据通道会话复用检查。

| 范围 | 本轮实际观察 |
|---|---|
| SSH | 首次信任；无 PTY/无终端初始化副作用；stdout/stderr 分离；非零退出码；超时及两个协议版本的 HTTP 断线取消 |
| 授权与策略 | 命令远端副作用被拦；历史规则删除后快照保留；SFTP 创建/覆盖区别和 REALPATH 别名保护；只读及本地来源范围拒绝；慢速读取中收紧策略后不返回正文 |
| 四种文件协议 | UTF-8 列表和文本读写；创建冲突不覆盖；替换；目录上传下载含中文、嵌套和空文件；服务端独立 SHA-256 校验；不可信 FTPS 证书被拒 |
| 任务与撤权 | 响应未解码前断线仍可发现已接受任务；实际详情按钮暂停/继续；排队/运行取消；本地来源目录消失后的远端清理；Key 停用、启用、撤销；连接授权缩减仅取消受影响任务；其他 Key/连接工作保持独立 |
| 服务与审计 | 服务关闭保存为暂停、开启不自动继续；故障注入阻止无开始审计的 SSH 业务副作用，仍可取消自有任务，且如实返回审计未落盘 |
| 应用恢复 | 正常退出/重启保存私有 FTP 覆盖暂存，人工继续前旧 `0600` 目标不变，最终摘要一致；强制终止/重启保持离线，人工继续后摘要一致 |

完整报告：`/private/tmp/sshdesk-integration-lj9pctsy/mcp-native-run-sm2x49hr/report.json`，`status=passed`、`scenario=all`。同目录四个 `native-app-*.log` 均无 Go race detector 报告。夹具容量缺陷的独立复现记录：`/private/tmp/sshdesk-integration-lj9pctsy/ftp-passive-capacity-evidence.json`；旧 5 端口夹具和失败 part 保留，不作为产品缺陷自动重试。

另已通过 `go test -race ./...`、`go vet ./...`、macOS 本机构建、Windows amd64 交叉构建，以及带 `-race -tags=integration` 的 `internal/sshclient` 和 `internal/fileclient` 全部协议集成回归（本轮分别 9.250 秒、34.417 秒）。协议回归额外包含错误 CA/主机名拒绝、FTP 私有目录不可接管既有目录及限权失败的归属信息、稳定部分上传验证与续传、16 个并发被动监听容量。

原生 Windows、各第三方 AI 客户端界面、真实系统钥匙串、其他服务器实现与生产规模未纳入本轮；原生控制使用现有测试 overlay，不表示做过视觉验收。

## 2026-11-04 请求与响应审计增强

增强前的 MCP 功能已提交为 `9bc13ca`。本轮新增 schema v3 的 `mcp_payloads`，保存请求与结构化响应，详情可多行查看和复制命令、stdout/stderr 及 JSON；文件正文与认证信息排除，旧摘要明确区分。

已通过临时 overlay 中的真实本机 HTTP + 官方 SDK + 隔离 SQLite 检查：旧协议初始化、新协议工具发现/ping/调用、未知方法、失败参数、单次工具调用不重复记录、命令输出保留、文件正文省略、认证字段及已知 MCP Key 排除。测试业务适配器使用固定 SSH 输出和本地文件客户端，这项检查不代替真实远端 SSH / FTP 运行验收。

数据库检查覆盖空库、v1、v2、v3 的升级及重开，已有设置保留、请求/响应独立更新、后续摘要不清空正文、显式 JSON null、清理级联及明细写入失败时开始记录回滚。故障注入还验证审计入口失败时自有任务取消通道仍可用，并明确返回 `audit_persisted=false`。

本轮启动原生回归时发现 Mac 锁屏，按用户要求跳过原生窗口及完整 Docker 任务回归，未把之前的 30/30 结果当成本轮验证结果。
