# 数据与凭据

## 默认数据位置

结构化业务数据统一保存在 `burrow.db`，使用 SQLite schema v1。应用数据目录为：

| 系统 | 正式版 | 开发版 |
|---|---|---|
| macOS | `~/Library/Application Support/Burrow/` | `~/Library/Application Support/Burrow Dev/` |
| Windows | `%LOCALAPPDATA%\Burrow\` | `%LOCALAPPDATA%\Burrow Dev\` |
| Linux | `$XDG_DATA_HOME/Burrow/`，未设置时为 `~/.local/share/Burrow/` | 对应 `Burrow Dev/` |

普通 `go run` 和 MyGo 开发包使用开发版命名空间；正式打包使用 Burrow。凭据服务分别为 `dev.burrow.app` 与 `dev.burrow.app.dev`。测试使用临时目录，原生 E2E 通过 overlay 使用独立目录和凭据 provider。

同一数据库同时只允许一个 Burrow 进程持有，防止两个界面用各自的旧内存快照互相覆盖。`burrow.db.lock` 是操作系统锁的载体，正常退出或进程结束会释放锁；不需要删除这个文件。

## 保存内容与边界

| 位置 | 内容 |
|---|---|
| SQLite `catalog_groups` / `profiles` | 分组、稳定 ID、排序、连接配置及凭据版本引用 |
| SQLite `managed_keys` | 公钥原文、密钥名称、类型、指纹及托管路径等元数据 |
| SQLite `settings` | 唯一一份应用及终端设置、快捷键、文件面板偏好 |
| SQLite `workspace` / `endpoints` | 布局、焦点、文件标签路径及独立的连接快照 |
| SQLite `transfer_*` | 批次、标签归属、端点/跳板快照、任务、递归清单、提交状态及检查点 |
| SQLite `host_keys` | 规范化主机名/IP、端口、完整公钥、算法、撤销标记与确认时间 |
| SQLite `themes` | 导入配色的名称、原文件名和原始字节，保留来源及许可注释 |
| 系统凭据存储 | 用户选择保存的密码、私钥口令 |
| `keys/` | 托管私钥及配套 `.pub` 文件；公钥的应用读取来源是数据库，外部导入不移动原文件 |
| 实际传输目标 | 下载/上传内容与未完成的 `.part` |

SQLite 不保存私钥明文、密码、私钥口令或文件内容，也不保存终端屏幕/回滚输出和监控曲线。数据库没有整体加密；站点地址、用户名、路径、环境变量和初始命令等配置应按私有数据处理，勿把秘密写入环境变量或命令配置。

缓存与日志保留独立边界。macOS 使用系统 Cache/Logs 目录；Windows 分别使用数据根下的 `cache/`、`logs/`；Linux 缓存使用系统 XDG 缓存目录。MyGo 仍管理系统语言偏好，在 Windows/Linux 可能生成 SDK 的 `locale` 文件；应用设置以数据库为准。

## 一致性与恢复

数据库访问集中在 [internal/store](../internal/store/)，使用 `database/sql + modernc.org/sqlite`，不依赖系统 SQLite 或 CGo。开启外键、WAL、`synchronous=FULL` 和 5 秒 busy timeout，每个 Store 仅保留一个数据库连接。

分组、条目、密钥元数据及关联工作区在事务内提交。UI 先构造不可变快照，数据库写入在后台串行执行；保存失败会报告错误，目录操作恢复内存快照。设置及布局采用 300ms 防抖，关闭/退出前显式保存。

新任务先保存再启动。传输 worker 独占清单，在安全点写入已确认偏移与 SHA-256 摘要；连续写入按约 1 秒或 4 MiB 的间隔保存，最终重命名前先保存提交意图，暂停/完成/取消与退出另行保存。崩溃恢复使用最近成功提交的检查点，仍需核对实际源、`.part` 和目标，不能保证恢复最后一字节。

文件系统、系统钥匙串和远端重命名不属于 SQLite 事务。现有资源准备、失败补偿、提交结果核对仍保留；数据库记录不能代替对真实文件的校验。私钥在 Unix 使用 `0600`，Windows 数据目录、数据库和私钥使用当前账号与 SYSTEM 的私有 ACL。

读取失败、损坏或不支持的 schema 会明确报错，不自动删除或重建数据库。schema 版本记录于 `PRAGMA user_version`；未来升级须新增明确的 schema 升级实现。

## 主机信任

SSH 客户端通过回调查询数据库，不调用系统 OpenSSH，也不读系统 `.ssh/known_hosts`。信任记录独立于保存条目，删除或编辑一个条目不会自动清除主机信任。

未知主机需要用户确认指纹。确认期间不持有数据库事务；写入时再次检查，防止其他连接先保存了不同的密钥。已变更和已撤销的密钥拒绝连接，不能用「首次信任」确认覆盖。

## 旧格式与备份

本次直接启用新数据模型，不读取、迁移或双写旧 `sessions.json`、`prefs.json`、`files.json`、`terminal.json`、`transfers.json`、`catalog.json`、`themes/` 和 `known_hosts`。已有磁盘文件保持原样，需在新版本重新配置；不提供旧格式自动回退。

常规备份先正常退出，再复制整个数据目录。运行中不能仅复制 `burrow.db` 而忽略 WAL；store 层提供 `Backup`，通过 SQLite `VACUUM INTO` 生成一致的独立数据库，当前没有备份界面。

备份数据库不包含系统凭据、`keys/` 私钥、实际传输文件和 `.part`，这些必须分别处理。跨设备恢复可能需要重新提供密码、私钥口令及调整本地路径。详情见[文件管理与传输](file-transfer.md)。
