# 终端配色

`internal/termtheme` 管理配色文件、终端背景、前景、ANSI 16 色、xterm 256 色、光标和选区；`internal/theme` 继续管理窗格容器和焦点方案 D。真实 SSH 会话由 MyGo v0.7.3 内置 Ghostty 终端插件绘制，最低要求 macOS 13。内置 Catppuccin Latte、Catppuccin Mocha、Dracula、Nord。默认终端字体为 JetBrains Mono / Menlo / monospace，12.5px、19px 行高。

设置中的「终端配色」可跟随 App 明暗并分别指定浅色和深色配色，或固定一套配色。适配层将默认前景、背景、ANSI 16 色、光标色和选区色传给 Ghostty，切换配色会更新既有终端内容；真彩色保留原始 RGB 值。插件继续支持终端输出中的 256 色索引，但其公开 `Theme` API 只提供 16 色调色板：导入配色中自定义的 16–255 号索引目前不会传入真实终端，使用插件的 xterm 色表。配色解析器仍保留这些自定义索引，不丢弃源文件内容。

「终端背景」默认使用配色背景；仅选择「与卡片底色一致」时使用 `C.Pane.Bg`。终端后方填充和上下渐隐栏使用同一背景来源。Shell 标题、地址、操作按钮、状态栏、活动徽章及窗格内描边使用与实际终端背景匹配的局部 App token；侧栏和监控栏继续使用全局 App 外观。正文、选区、光标和终端滚动条由 Ghostty 绘制。

字体、字号、行高和光标偏好独立保存，切换配色不会修改。适配层将保存的 DIP 行高换算为插件所需的字体自然行高倍数。插件目前没有字距或粗体亮色设置 API，相应设置标为不可用；已有 `tracking` 和 `boldIsBright` 值继续保存在配置中，但不作用于真实终端。光标初始形状和闪烁传入新建终端选项，设置变更在下次连接时生效；远程程序可再次设置自己的光标样式。新建连接时若启用“减少动态效果”，默认光标不闪烁。

配置保存到系统用户配置目录的 `SSH Desk/terminal.json`，macOS 为 `~/Library/Application Support/SSH Desk/terminal.json`。导入的 Ghostty 配色文件保存在同级 `themes/` 目录，启动时重新导入。读取配置失败会显示错误并保护原文件；导入解析错误包含文件和行号。

完整配色许可证通过 `go:embed` 编入应用，可在设置的「关于 / 致谢」中查看。

## Shell 容器适配

局部 UI 颜色从现有浅／深 token 中按实际底色对比度选择；低对比的中性辅助文字和图标提升到该表面的主文字色，适配固定配色、跟随主题及卡片背景。解析结果按当前底色缓存，不按配色名称判断，也不在每帧创建整套主题。正文 ANSI、真彩色和选区继续由终端配色决定，终端滚动条由插件按其当前配色绘制。

顶部和底部的渐隐模糊仅覆盖标题栏、状态栏各自的高度，不向正文延伸。Ghostty 视图在上下栏之间布局，正文与各栏保留 `Space.N4`（4px）间距，插件按这一区域的实际字体网格计算行列数，并通过 SSH `WindowChange` 更新远程 PTY。激活态仅绘制现有 1.5px 蓝色边框，焦点方案 D 的输出透明度与饱和度继续应用于终端视图。

取色评估参考 W3C 的[文字对比度](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)与[控件对比度](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html)。现有 token 的优选和提升不等于任意导入底色均已达到完整 WCAG 要求。

## 验证边界

SSH 第一阶段接入真实连接、PTY 和 Ghostty 屏幕模型，包括键盘、IME、复制粘贴、屏幕控制序列及备用屏幕处理。应用启动不自动连接示例主机，用户明确打开会话后才连接；取消、断开、关闭及退出均清理相应连接，重连保留先前历史。密码与私钥口令只用于本次连接，不保存；应用专用 `known_hosts` 对未知主机显示确认，对已记录密钥变更或被撤销的主机拒绝连接。文件页现已接入真实 SFTP / FTP 远程目录只读浏览；同会话 SSH 分屏及 SSH / SFTP 跳转复用认证连接。文件写入、传输增强与服务器指标采集尚未接入。

本阶段 `go build ./...`、`go vet ./...`、`go test -race ./...` 及 `go tool mygo build -skip-dmg` 通过；另运行 MyGo v0.7.3 已有的 `TestExit`、`TestMirrorResize`、`TestHistory` 通过。新增带 `integration` 标签的 Docker OpenSSH 联调测试已在竞态检测下通过，覆盖认证、主机信任、UTF-8 与颜色字节、远端 PTY 尺寸、会话隔离、输出完整性和取消清理。真实原生窗口联调也在竞态检测下通过，覆盖密码与首次信任弹窗、四种认证、文字输入、Vim 日文编辑、top、分屏默认键盘焦点、独立 Shell、远端尺寸同步、关闭窗格与标签、重连历史和带活动连接退出应用。已确认应用退出后容器只剩 sshd 主进程，无测试 Shell 遗留。未开展视觉验收，复验方式见 [Docker SSH 联调](../dev/ssh-integration/README.md)。

历史配色接入中的测试覆盖文本原型的 256 色索引和真彩色、粗体／暗淡／反显、链接与选区，以及设置重启恢复、导入保真及冲突保护；当时还使用 MyGo `ui.NewTester` 检查过配色和分屏选区快照。这些记录描述旧文本渲染器，不能证明 Ghostty 适配层已通过相同的渲染或交互验收。

MyGo API 依据项目固定的 v0.7.3 源码及随包分发的 `docs/plugins/terminal.md` 核对。终端使用该版本已有的 `plugins/terminal`，SSH 传输使用 `golang.org/x/crypto/ssh`；`mygo dev` 和 `mygo build` 会下载、校验并打包所需的 Ghostty 原生库。
