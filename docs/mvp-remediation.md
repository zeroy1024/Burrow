# MVP 后端审查整改

本轮承接`73159b6`后的14项审查问题，保留UI布局、目录独立存储、三页切换和后台任务持有关系。下表记录当前源码接入，运行验证单独列明；“完整实现”不代表所有设备和服务器已经实测。

| 编号 | 原问题 | 当前状态 | 实际API与生产消费者 |
|---|---|---|---|
| 1 | FTP普通LIST隐藏点文件，stage上传失败、递归遗漏 | 完整实现 | `ftpClient.readDirectory`优先MLSD，回退`LIST -a`及`listNames`；`ReadDir/Stat`和`buildTransferManifestAt/copyTransferNode`使用完整列表。NLST flags不支持时仅接受能与LIST完整核对的普通响应 |
| 2 | LIST裁剪真实前导空格/Tab，产生错误操作路径 | 完整实现 | `ftpListingFields`保留原文，`reconcileFTPNames`以NLST原名核对；DOS填充或名称有歧义时明确拒绝。`readFileDirectory`、文件动作及传输消费原始名称；重命名草稿按原文校验和提交 |
| 3 | 覆盖暂存文件改变已有目标权限 | 完整实现 | `Entry.Mode/ModeKnown`、可选`PermissionSetter.Chmod`及`preserveTransferPermissions`在最终Rename前按同一掩码恢复并回验原目标已知rwx及setuid/setgid/sticky；未知或无法设置/确认时拒绝覆盖。新目标保留创建策略，不强制依赖FTP SITE CHMOD；本地新stage默认0600 |
| 4 | SSH跳板TCP/隧道建立不受setup预算约束 | 完整实现 | `Config.SetupTimeout`覆盖认证与底层Dial；`dialSSHPlan`传每跳剩余预算，`Connection.DialContext`只用短context约束建立过程，已建立隧道保留独立长生命周期 |
| 5 | 暂停part前缀被修改仍可能显示成功 | 完整实现 | `transferCheckpoint{Offset,Digest}`与`prepareTransferResume/verifiedTransferPrefix`校验目标已确认前缀，长度或摘要不匹配明确失败；检查点只承认已确认写入偏移，不信任part长度本身 |
| 6 | FTP/SFTP下载继续时重新拉取源前缀 | 完整实现 | v3先从目标part重建并验证SHA-256，再对源调用`OpenRead(offset)`，FTP使用REST、SFTP使用Seek；v1/v2旧part首次继续作一次源/目标前缀对照后建立v3检查点 |
| 7 | FTP收到421仍显示连接正常 | 完整实现 | `ftpClient.begin`的finish识别421并关闭客户端；`fileRuntime`等待`Client.Done`后更新断开状态。550等普通操作拒绝不被当作断线 |
| 8 | case-only或规范等价名称重命名被拒绝 | 完整实现 | `file_actions.go`直接调用`Rename(NoReplace)`；Local确认同inode及等价名称，Local/SFTP使用随机暂存名两段NoReplace并在失败时安全回滚。FTP对同父目录等价名称也使用随机暂存名、预检后两段重命名及失败回滚，兼容服务器拒绝直接case覆盖；FTP仍无跨外部客户端原子保证 |
| 9 | 测试连接在切换认证后仍读取旧密码 | 完整实现 | `startDraftTest`仅在当前password认证且保留旧密码时读取`sessionPassword`；私钥、agent、匿名认证不被旧密码记录或钥匙串错误阻断 |
| 10 | SSH密码认证缺少keyboard-interactive/PAM | 完整实现 | `Config.KeyboardInteractive`由`sshRuntimeConfig`接真实问题处理器；原认证弹窗按echo标志收集回答，正式连接和测试共用，不把密码盲填进OTP等问题。SSH测试使用完整认证预算，避免交互回答仍被原型10秒UI预算截断；FTP测试预算保持原策略 |
| 11 | 列表不能显式进入目录符号链接 | 完整实现 | 可选`DirectoryResolver.ResolveDirectory`在`fileNavigation.link`路径调用；Local解析真实目录，SFTP跟随Stat后RealPath，FTP在gate内CWD后读实时PWD。仅`ErrNotDirectory`转元数据预览，权限/连接错误照常报告；传输Stat仍为Lstat |
| 12 | FTP丢弃权限及所有者元数据 | 完整实现 | `parseUnixFTPEntry`保留mode/owner/group，`parseMLSDEntry`读取服务器提供的Unix事实；文件表格/预览消费permissions/owner，覆盖事务使用实际mode，不伪造未报告属性 |
| 13 | 本地验后替换symlink可写入或递归删除树外 | 完整实现（darwin/linux严格路径） | `openLocalLeaf`从已打开父Root取得FD后Openat，最终段NOFOLLOW、新建EXCL、Fstat身份校验后才Truncate；`TreeRemover.RemoveTree`在稳定父/子FD下Unlinkat递归并逐项检查context。`removeFileTree`优先该能力 |
| 14 | native先以80×24消费大PTY输出，形成初始化竞态 | 完整实现 | 本地终端包`Options.Columns/Rows`在启动reader前创建正确VT网格；`attachSSH`使用实际Conn尺寸，独立拖动快照同样在New时传网格。后续View实际尺寸变化仍走原Resize，不重连、不重放 |

## 恢复、权限与边界

队列格式升至v3。检查点只持久化已确认偏移和摘要，不保存文件字节或SHA内部状态。恢复先读取目标前缀校验，再从源偏移继续；v1/v2兼容路径需要一次旧part前缀对照。无法读取或确认目标前缀时明确拒绝继续；普通首次上传仍不要求读回目标内容。

覆盖既有文件必须获取真实权限，在提交前按同一掩码设置并回验rwx及setuid/setgid/sticky；未知模式、服务器不支持或权限不足时保留原目标并报告。新文件不强制CHMOD，本地stage从0600开始。权限保留不等同于复制所有权、ACL、扩展属性或取得远端文件快照。

显式进入目录链接是浏览动作，不放开链接传输或递归跟随。Unix严格FD方案适用于darwin/linux；其他平台的Root适配不作为同等设备验收。远端协议不能承诺隔离其他客户端在原子操作之外的并发变化，FTP仍没有真正条件Rename。

## 本轮验证证据

- Local公开API临时`-race`验证9项通过：case-only重命名、不同目标NoReplace保护、新stage0600、Chmod、最终链接拒绝、递归不跟随树外链接、预取消、目录链接canonical导航、文件链接`ErrNotDirectory`。结果：[本地能力验证](/private/tmp/sshdesk-local-safety-s7_lj9ws/result-final.json)。首轮夹具将`/var`别名与`/private/var`真实路径混同的失败日志保留；更正canonical期望后通过，未为此修改生产逻辑。
- 第14项先确定复现：132×50的PTY输出在80×24内核落到错误位置，后续Resize不能修复；再以真实native路径完成修后3组验证。SSH另4组覆盖PAM多问题/echo、转发与排队deadline、建立后setup取消和兄弟通道隔离。[修前/修后及SSH汇总](/private/tmp/sshdesk-initial-grid-539y3k6y/verification-summary.json)保留首轮deadline错误及补跑证据；未冒称真人认证弹窗验收。
- 文件公共层已有`go test ./internal/fileclient ./internal/desktop`、`go vet ./internal/fileclient`通过；fileclient普通tag当前没有测试文件，只提供编译证据，不算协议通过。
- 真实loopback FTP覆盖LIST回退与MLSD两模式：隐藏stage的写入/Stat/权限/提交/offset、前后空格及成对名称不混淆、case重命名、Unix元数据、目录链接/普通文件链接和421断线，共15条协议断言通过。[FTP汇总](/private/tmp/sshdesk-ftp-fixes-1mijg17f/verification-summary.json)记录实际`0o644`兼容缺口及修正、夹具重用目标的失败和补验；最后通用550文件链接分类单独补验，未重复历史TLS/active组合。
- 传输12组主用例及5子例覆盖CP真实偏移、改写prefix拒绝、部分Write、legacy/v3往返、不可读target、源增长、权限0600/0755/0000、无setter的新建与覆盖边界等，最终race通过。[检查点汇总](/private/tmp/sshdesk-transfer-checkpoints-yp536anl/verification-summary.json)。特殊位另补真实sticky、模拟完整mask支持/拒绝/剥离；本环境真实setuid/setgid三子例跳过，未计通过，不能保留的权限会在Rename前明确失败。[特殊权限证据](/private/tmp/sshdesk-transfer-special-mode-7_9w48zp/verification-summary.json)。
- SFTP新增4项真实协议验证通过：普通文件/目录权限和实际Owner、显式链接浏览且Lstat仍拒绝传输链接、APFS case改名与不同目标保护、关闭单一SFTP不影响同transport兄弟通道。[SFTP汇总](/private/tmp/sshdesk-sftp-newcap-jp2df_54/verification-summary.json)明确夹具RENAME不作为外部并发原子保证证据、REALPATH未解析最终规范路径、额外setgid环境限制，未把它们当作通过。
- 全项目已有`go test -race ./...`、`go vet ./...`、`go build ./...`通过；终端子模块已有race测试通过。修改源码gofmt、tracked及新增文件diff检查通过。重新MyGo打包darwin/arm64应用18.8MB并签名，`codesign --verify --deep --strict`通过。检查在最后FTP兼容调整后完成；终端子模块无后续功能修改，未为文档更改重复运行测试。
- 未新增永久测试、截图基线或CI，未访问用户真实服务器/配置/凭据，未做GUI视觉验收。原Docker集成未重跑（本机Docker daemon未启动），上述独立loopback与临时控制器证据不冒称完整设备端到端验收。所有自有临时服务、源码、测试数据及独立环境已停止或回收，保留非敏感日志/hash；各汇总附清理记录。
