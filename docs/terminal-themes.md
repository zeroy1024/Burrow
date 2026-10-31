# 终端配色

`internal/termtheme` 管理终端背景、前景、ANSI 16 色、xterm 256 色、光标和选区；`internal/theme` 继续管理窗格容器和焦点方案 D。内置 Catppuccin Latte、Catppuccin Mocha、Dracula、Nord。默认终端字体为 JetBrains Mono / Menlo / monospace，12.5px、19px 行高、-0.1px 字距。

设置中的「终端配色」可跟随 App 明暗并分别指定浅色和深色配色，或固定一套配色。颜色索引、默认色标记和真彩色保留在会话输出中，渲染时解析，切换配色会更新既有输出。真彩色保留原始 RGB 值。

「终端背景」默认使用配色背景；仅选择「与卡片底色一致」时使用 `C.Pane.Bg`。终端后方填充和上下渐隐栏使用同一背景来源。Shell 标题、地址、操作按钮、状态栏、活动徽章、非焦点光标描边及窗格内描边使用与实际终端背景匹配的局部 App token；侧栏和监控栏继续使用全局 App 外观。字体、字号、行高、字距和光标偏好独立保存，切换配色不会修改。

配置保存到系统用户配置目录的 `SSH Desk/terminal.json`，macOS 为 `~/Library/Application Support/SSH Desk/terminal.json`。导入的 Ghostty 配色文件保存在同级 `themes/` 目录，启动时重新导入。读取配置失败会显示错误并保护原文件；导入解析错误包含文件和行号。

完整配色许可证通过 `go:embed` 编入应用，可在设置的「关于 / 致谢」中查看。

## Shell 容器适配

局部 UI 颜色从现有浅／深 token 中按实际底色对比度选择；低对比的中性辅助文字和图标提升到该表面的主文字色。滚动条独立比较浅／深半透明滑块与底色合成后的对比度，适配固定配色、跟随主题及卡片背景。解析结果按当前底色缓存，不按配色名称判断，也不在每帧创建整套主题。正文 ANSI、真彩色和选区继续由终端配色决定。

顶部和底部的渐隐模糊仅覆盖标题栏、状态栏各自的高度，不向正文延伸。正文与上下栏均保留 `Space.N4`（4px）间距：分屏顶部留白为 `HeaderHeight + Space.N4`，底部留白为 `StatusBar.HeightPane + Space.N4`，拖动预览使用相同间距；滚动途中内容经过栏内模糊区的效果保留。激活态仅绘制现有 1.5px 蓝色边框，去掉蓝色泛光和浮起阴影，焦点方案 D 的输出透明度与饱和度不变。

取色评估参考 W3C 的[文字对比度](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)与[控件对比度](https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html)。现有 token 的优选和提升不等于任意导入底色均已达到完整 WCAG 要求。

## 验证边界

当前项目仍为演示客户端，没有 SSH / PTY、终端键盘输入或完整屏幕控制序列处理。SGR 和 OSC 8 的支持用于现有完整行输出，不能等同于已经验证 `vim`、`htop` 等真实交互程序；这部分验收需在真实终端传输及屏幕模型接入后进行。

本次验证：`go build ./...`、`go vet ./...`、`go test ./...` 通过，`gofmt -l .` 无输出。回归测试覆盖 256 色索引和真彩色、粗体／暗淡／反显、链接与选区、设置重启恢复、导入保真及冲突保护。终端渲染源码的硬编码颜色检索无匹配（排除测试中的颜色样本）。

初次配色接入时使用 MyGo `ui.NewTester` 检查了 Latte、Mocha、固定 Dracula、卡片背景、设置页面及分屏选区的渲染快照，并验证主题按钮应用和选中文字切换窗格焦点。本轮 Shell 容器取色、顶部留白及焦点边框调整仅运行上述已有检查并进行静态复核，未新增测试或进行视觉验收。尚未人工操作原生文件选择框，也未运行真实 SSH 程序或测量 GPU 性能；这些不计为已通过的端到端验收。

MyGo API 依据官方 [文本](https://mygo.egoist.dev/docs/ui/text)、[样式](https://mygo.egoist.dev/docs/ui/styling)、[桌面 API](https://mygo.egoist.dev/docs/native) 文档，并与项目固定的 v0.7.3 源码核对。没有引入 Web 渲染或新增第三方依赖。
