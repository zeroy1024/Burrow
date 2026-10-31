# Design token 主题接入说明

## 当前接入规则

`internal/theme/tokens_src.py` 是 App token 的唯一来源，生成 `theme.go`、`tokens.json`、`tokens.md`。SSH、文件传输和设置共用 `uiadapt.go` 的适配入口；终端正文、ANSI 配色、字体及光标参数由 `internal/termtheme` 管理。终端周边 UI 按实际终端背景选择局部明暗 token，详见 [终端配色说明](terminal-themes.md)。

- 普通按钮与主按钮使用 `button` / `primaryButton`，输入框使用 `textInput`，弹窗使用 `modal`。这些入口保留 MyGo Base API 的输入、焦点和关闭行为，尺寸、颜色、描边、圆角、遮罩和阴影读取对应 component token。
- 所有应用主动创建的文字提示通过 `tooltip` / `styleTooltip` 共用 `C.Tooltip`。导航提示仍位于按钮右侧，其余提示位于上方并由 MyGo 处理边缘避让。
- 普通线性图标在初始化解析 SVG 时读取 `C.IconButton.Stroke`，浅／深主题共用这一几何参数；修改线宽后需重启应用。OS 品牌资源保留原有描边和形状。
- `C.PaneFocus.RingFocused` 本身只包含单层蓝色描边，渲染端完整消费，不再截取阴影数组。
- `tokenTween` 明确使用 `motion.easing.out`，按钮、列表、分段控件和两种会话标签的状态过渡使用各自映射到 `motion.duration.focus` 的 160ms token。拿起保持 120ms，让位、落位、取消继续使用现有弹簧。
- 搜索几何变化使用 `C.Search.Transition/Easing`；分组数字滚动显式使用 `C.GroupHeader.CountMotion`（350ms 弹簧）。有变化文案的切换按钮采用固定 key，避免文案变化重建动画状态。
- 减少动态效果时由 MyGo 禁用位移、缩放和旋转；颜色／透明度使用 `AnimateColor` 通道保留过渡。文件搜索、FTP 本地面板和监控面板将几何与透明度分开处理。
- Shell 拖动预览使用 `C.DragGhost` 的 220×140px 静态骨架卡片，按实际终端背景选择明暗颜色；`C.PaneReveal` 定义落位后的 8px → 0、220ms 模糊过渡。落位窗格直接采用最终尺寸，首帧绘制完成后启动渐清晰，不等待模拟数据或网络输出，不再创建终端快照和放大飞入动画。减少动态效果时保留这段局部视觉过渡。
- SSH 和文件会话共用 `tabConnectionIndicator` 的状态槽与旋转参数。SSH 演示连接的阶段间隔由 `C.ConnectionIndicator.StageDuration`（300ms）提供，连接日志沿用终端 ANSI 配色与字体；标签、Shell 状态栏和监控等待态读取同一会话状态。

系统菜单、文件选择器保留系统外观。设置中的 `Select`、`NumberInput`、`Checkbox` 仍使用 MyGo 控件及 `nativeTheme` 的全局映射，内部布局和框架自带提示遵循 MyGo；本次没有复制这些复合控件的内部实现。原生手势阈值／长按等待、Tooltip 显示等待仍由框架管理。未使用或已废弃的设计 token 不代表当前界面已实现对应功能。

## 历史接入记录

以下保留首次主题接入时的差异与验证记录。FTP 未实现、固定终端文字配色、焦点光晕等描述仅代表当时状态；当前实现以上述规则及代码为准。历史截图与构建结果不作为本轮视觉验收证据。

原界面的浅色样式散落在 Go 常量、组件和动画中，设置入口不可用。本次将现有原生 MyGo 界面接入生成式主题包，设置支持浅色、深色、跟随系统，切换无需重启。目录保留 main 的单击打开、同级稳定 Key 拖放、排序预览、跨组移动和配置持久化，不新增 FTP 功能或第三方依赖。

## 实现

- `internal/theme/tokens_src.py` 是唯一 token 源；`gen.py` 产出 `theme.go`、`tokens.json`，`mdgen.py` 产出 `tokens.md`。未导入压缩包的独立 `go.mod`，主题测试直接在项目 module 内运行。
- 每次 build 读取 `theme.Current()`，由 `nativeTheme` 完整构建 MyGo 控件主题；UI 优先读取 component，缺失时读取 semantic，不读取 Palette。
- `theme.OnChange` 更新原生窗口底色并调用 `Window.Invalidate`。`mygo.Theme.OnUpdated`、`IsDark`、`SetSource(ThemeLight/ThemeDark/ThemeSystem)` 同步系统外观。
- `uiadapt.go` 集中完成 `color.NRGBA → ui.Color`、多层 `Shadow/InsetShadow`、两类 `ui.SpringOptions`、`ui.CubicBezier` 的适配。MyGo 阴影按追加顺序绘制，适配层反向追加，保留 token 首层在上的语义；零 blur 的 spread 环保留为阴影环，内描边叠在未裁剪 wrapper 上。
- 三层渐隐玻璃使用 `glass.Blur{Radius, Mask: *ui.LinearGradient}`，不绘制上下分隔线。
- 位移使用 `Context.Preferences().ReduceMotion` 与 `MotionDuration`。MyGo 标量动画在减少动态时会全部瞬时完成，视觉透明度/饱和度改经 `AnimateColor` 的 alpha 通道过渡（中途有 8 bit 量化，终值恢复精确 token 值）。监控栏几何与透明度分离，拖放恢复阶段持续请求帧。
- 目录渲染与槽位计算统一读取 `C.Catalog`：固定行高40px、列表paddingY2/X8、行间距2px、组间距10px，避免预览落点与实际行错位。保留 main 搜索覆盖层与 PassThrough 图标。配置加载、原子保存、保存失败回滚与读取失败写入保护保持 main 实现。
- 终端正文和光标保留原固定颜色、Menlo 11px 字体、行高、尺寸和闪烁实现。正文与 cursor 经常量名归一化后与原代码相同；容器、焦点、身份栏和底栏使用 token。

真实 API 依据：项目固定的 MyGo v0.7.3 源码，以及 [UI](https://mygo.egoist.dev/docs/ui)、[样式](https://mygo.egoist.dev/docs/ui/styling)、[拖放](https://mygo.egoist.dev/docs/ui/drag-and-drop)、[Motion](https://mygo.egoist.dev/docs/ui/motion)、[Transitions](https://mygo.egoist.dev/docs/ui/transitions) 文档。

## 浅色值差异

任务同时要求像素一致和“发现差异时以 token 为准”；两者在现有代码中无法同时满足。以下采用后者，不把实际设计差异计为抗锯齿误差。未变化的值不重复列出；新增交互列为新增，未擅自给主机目录增加会话选中状态。

| 文件 | 属性 | 旧值 | 新值 / token |
| --- | --- | --- | --- |
| app.go | rail 按钮尺寸 | 32 | 34 / Rail.ButtonSize |
| app.go | 卡片圆角 | 12 | 10 / Card.Radius |
| app.go | rail 上下 padding | 6 / 8 | 4 / 10 |
| app.go | rail 前景色 | #77777c | #6e6e73 |
| app.go | rail 激活底色 | 黑 8% | #d6d6d8 |
| app.go | 工作区外侧/卡片间隙 | 6 | 8 / Window.Gutter |
| app.go | sider 初始/最小宽度 | 260 / 240 | 236 / 236；最小值与 token 初始宽度对齐 |
| app.go | 监控宽度 | 220 | 204 / Size.StatsPanel |
| app.go | rail tooltip paddingY / 字号 / 圆角 | 5 / 12 / 5 | 4 / 11.5 / 6 |
| app.go | tooltip 底色 / 字色 | #1d1d1f / #fff | rgba(30,30,32,.92) / #fff |
| app.go | tooltip 阴影 | y2 blur8 黑20% | y4 blur12 黑18% |
| uiadapt.go | 原生控件字体大小 / 圆角 | MyGo 13 / 6 | 12.5 / 7 |
| uiadapt.go | 原生控件底色 | MyGo #f4f4f5 | #f2f2f4 |
| uiadapt.go | 原生控件悬停 / 按下 | MyGo #e9e9ec / #dddde1 | #f7f7f8 / #e6e6e9 |
| uiadapt.go | 原生控件边色 | #eeeef0 | #e6e6e9 |
| uiadapt.go | 原生选择 / 焦点颜色 | MyGo rgba(37,99,235,.25/.55) | FileList.BgSelected / Accent.Tint50 |
| uiadapt.go | 原生控件强调色悬停/按下 | MyGo #1d4ed8 / #1e40af | #0a84ff |
| uiadapt.go | 原生警告 / 成功色 | MyGo #d97706 / #16a34a | #f5a623 / #30c45a |
| toolbar.go | 标题栏高 | 48 | 44（仍遵守原生 titlebar 最小占位） |
| toolbar.go | 通用图标按钮尺寸 / 圆角 | 28 / 7 | 26 / 6 |
| toolbar.go | 图标按钮前景 / hover / pressed | #8e8e93 / 黑5.5% / 黑10% | #6e6e73 / #f7f7f8 / #e6e6e9 |
| toolbar.go | actions 右 padding / 激活底色 | 8 / 黑6.5% | 10 / #d6d6d8 |
| toolbar.go | 标签 gap / 高 / 圆角 | 4 / 32 / 8 | 2 / 28 / 7 |
| toolbar.go | 标签 padding 左右 / 内容 gap | 9 / 9 / 6 | 10 / 8 / 7 |
| toolbar.go | 标签字号 / 字重 / 非选中字色 | 12 / 550 / #1d1d1f | 12.5 / 500 / #6e6e73 |
| toolbar.go | 选中标签底色 / 阴影 | 白100% / .5px黑6%环 | 白85% / .5px黑7%环 + y1 blur2 黑5% |
| toolbar.go | 标签让位 | 150ms | stiffness420 / damping34 |
| sessions.go | sider 标题字号 / 字重 | 15 / 650 | 13 / 600 |
| sessions.go | 添加按钮尺寸 / 圆角 / 底色 | 28 / 7 / #f3f3f5 | 26 / 6 / #f2f2f4 |
| sessions.go | sider 标题 padding | 固定52高、水平14 | 上16右12下10左16，自然高度 |
| sessions.go | 搜索高 / 圆角 / 底色 | 34 / 8 / #f4f4f6 | 30 / 7 / #f4f4f5 |
| sessions.go | 搜索字号 / 图标尺寸 | 12 / 15 | 12.5 / 18；保留 main 无键帽的全宽输入 |
| sessions.go | 搜索底 margin | 0 | 8 |
| catalog_sidebar.go / catalog_drag.go | 列表垂直 padding | 12 | 2；渲染与槽位计算同步 |
| sessions.go | footer | 固定高34、水平14、字号11 | paddingY10/X16、字号12，新增 .5px #f0f0f2 顶边 |
| catalog_sidebar.go | 分组 gap / 圆角 / 字号 / weight | 5 / 8 / 11 / 默认 | 4 / 7 / 11.5 / 500；保留 main 30px 固定高度 |
| catalog_sidebar.go | 分组计数字色 | #8e8e93 | #b0b0b5；保留 main 水平8、垂直0 padding |
| catalog_sidebar.go | 主机行右padding / gap / 圆角 / hover | 8 / 6 / 8 / #f7f7f9 | 6 / 9 / 7 / #f7f7f8；保留 main 40px 固定高度 |
| catalog_sidebar.go | 主机名 weight / 副标题字号 | 550 / 10.5 | 500 / 11.5 |
| catalog_sidebar.go | UI 等宽字体 | Menlo | JetBrains Mono, Menlo, monospace；不影响终端 |
| catalog_sidebar.go | OS logo 方块 / 图形 | 24 / 16 | 26 / 15 |
| catalog_sidebar.go | OS logo 底色 | 各品牌预设浅色 | OSLogoBg：品牌色9%混入当前卡片色 |
| catalog_sidebar.go | 更多按钮圆角 / hover底色 / 前景 / 淡入 | 7 / #ededf0 / #8e8e93 / 120ms | 6 / #ececef / #6e6e73 / 160ms；保留 main 默认透明、按钮悬停才显底色 |
| sessions.go | 在线 / 离线 / 连接中色 | #28c15a / #cfd0d5 / #f5a21a | #30c45a / #cfcfd4 / #f5a623 |
| sessions.go | 状态点 halo alpha | 16% | 在线14%、连接中18% |
| sessions.go | 弹窗标题字重 | 650 | 600 |
| catalog_sidebar.go | 拖动叠卡底色 / opacity / 阴影 | #f9f9fa / 1 / .5px黑8%边 | 主题 liftStack / .8 / 完整 StackShadow（含 y4 blur10 黑6%）；主卡 lift 改读120ms token曲线 |
| status.go | 标题字重 / 次级字号 / 详情 gap | 650 / 10 / 11 | 600 / 11 / 12 |
| status.go | 图表填充 / 曲线 | #f5f5f7 / #414145 | #fafafb / #1d1d1f |
| status.go | 进度轨道 / 填充 / 圆角 | #f0f0f3 / #29292d / 99 | #f0f0f2 / #1d1d1f / 3 |
| terminal.go | 空窗格提示字号 / drop 底 / 圆角 | 12 / 蓝12% / 10 | 12.5 / 蓝3.5% / 8 |
| terminal.go | pane drop 底 | 蓝12% | 蓝3.5% |
| terminal.go | card 背板 | #fff（其上绘制#fbfbfc） | 统一 #fbfbfc |
| terminal.go | 身份栏左 padding / gap / 主机名字号 | 10 / 5 / 12 | 14 / 7 / 12.5 |
| terminal.go | 登录地址字号 / 字体 | 10 / Menlo | 11.5 / token UI mono |
| terminal.go | 身份栏状态点 | 6 | 7 |
| terminal.go | 状态栏高 / 横padding / gap / 字号 | 28 / 10 / 8 / 10 | 32 / 14 / 10 / 11.5 |
| terminal.go | 已连接 badge 底 / 字色与点 | #e4f5ea / #2aa255 | #ebf8ef / #1f8a3f |
| terminal.go | 未连接 badge 底 / 字色 | #f0f0f3 / #8e8e93 | #f2f2f4 / #5b5b62 |
| terminal.go | badge 横padding / gap / 圆角 / 点 / 字号 | 6 / 4 / 99 / 5 / 10 | 7 / 5 / 20 / 6 / 11 |
| terminal.go | 单窗格动作胶囊底 | 白90% | pane #fbfbfc 90% |
| terminal_focus.go | 焦点阴影 | 1.5px 蓝环 | 同环 + 5px蓝16% halo + y8 blur24 黑10% |
| terminal_focus.go | 活动 badge 左padding / 字号 / 圆点半径 | 7 / 10 / 99 | 6 / 11 / 20（6px圆点外形不变） |
| terminal.go | 渐隐栏 | 单4px blur，白96% smoothstep | 1.5/4/9px三层 blur，取pane底色，alpha .92/.78/0、stop 0/.45/1，延伸22px |
| terminal_scroll.go | thumb 圆角 / 底部位置 | 99 / 28px栏 | 20（形状不变） / 32px栏 |
| pane_drag.go | 拿起 scale / 圆角 / tween | .96 / 10 / 220ms | 1.03 / 9 / 120ms |
| pane_drag.go | 拿起阴影 | y16 blur40 黑18% | .5px黑10%环、y10 blur28黑16%、y2 blur6黑8% |
| pane_drag.go | 取消 / 落位弹簧 | 220ms / 220ms | 300ms bounce0 / 320ms bounce.15 |
| pane_drag.go | 标签自动滚动 | 边缘28px、4px/frame | 48px、最大900px/s，深度平方曲线 |
| pane_drag.go | 占位底 / 虚线 | 黑2% / 黑12% 1.5px | 黑3.5% / 黑5% 1px |
| pane_drag.go | pane快照header 横padding/gap/字号/点 | 12 / 6 / 12 / 6 | 14 / 7 / 12.5 / 7 |
| pane_drag.go | pane快照footer 高/padding/字号/底色 | 28 / 12 / 10 / 白96% | 32 / 14 / 11.5 / pane底96% |
| pane_drag.go | tab快照 padding/gap/字号/weight/圆角 | 9 / 6 / 12 / 550 / 8 | 10 / 7 / 12.5 / 500 / 7 |
| pane_motion.go | pane让位弹簧 | duration220ms | stiffness420 / damping34 |

## 限制与未覆盖项

- MyGo v0.7.3 `Element.Drag` / `DraggableOptions` 不暴露手势启动阈值和长按参数，因此 `C.DragLift.Threshold/HoldDelay` 仍无法动态覆盖原生手势。目录列表沿用 main 的 Draggable 状态机；其垂直自动滚动、主机/分组 ghost、叠卡、占位和闪光已全部接入 token。标签横向自动滚动和工作区预览同样使用 token，未改依赖或并行维护旧目录实现。
- 现有 sider 是主机目录，未显示会话选中/连接状态，所以没有可接入的 selected OS logo 状态；连接状态在 tab/窗格显示。FTP 页面、文件列表、面包屑、队列尚未实现，保留其 token，不添加业务页面。
- 深色容器中终端普通文字仍为原 #1d1d1f，因“不改终端内容颜色”的约束而对比度较低；未擅自新增终端主题。
- 外观选择仅保存在当前进程；主机目录、分组、折叠与排序沿用 main 保存到用户配置目录的 `SSH Desk/catalog.json`。系统外观事件已接监听，但没有实际切换 macOS 全局外观或“减少动态”设置。

## 验证

- `go build ./...`、`go vet ./...`、`go test ./...` 通过；`gofmt -l .` 无输出；`git diff --check` 通过。
- 题目给定颜色扫描只命中终端文字/cursor与 OS 品牌资源。动画扫描只命中适配器字段映射和保持原状的终端 cursor 500ms 闪烁。
- `go tool mygo build -skip-dmg` 成功生成并 ad-hoc 签名本地 macOS arm64 应用包。
- 合入 main 新目录实现之前，临时副本中执行 MyGo 原生软件渲染器14场景（主界面、四类分屏、悬停、新输出、响铃、连接中、侧栏悬停、pane/tab/host/group拖动），共14张baseline及60张final截图（另增空窗格复验）。14场景及补充空窗格的Light→Dark→Light逐像素差异全部为0；减少动态下位移瞬时、视觉透明度渐变的检查通过。
- 上述历史快照中，原版→当时浅色有21.486%–50.898%像素变化，明显超出抗锯齿边缘，不标记“像素一致”验收通过。原始数据和截图在工作区忽略目录 `.cache/theme-review/`，不是产品文件；这些截图不代表当前新目录交互已通过视觉验收。
- 合入 main 新目录实现之前，实际启动本地应用包，通过设置按钮切到深色并检查主界面、切到“跟随系统”读取当前浅色外观、再手动切回浅色；窗口、tab、sider、pane容器、状态栏同步重绘，无需重启。没有更改 macOS 系统设置，因此后续系统外观事件与原生减少动态事件仍待设备验证。

## 新增 semantic token

为项目已有窗口约束、滚动条、弹窗和pane生命周期补充 67 项，component仅引用这些semantic路径；没有复制palette到UI。完整类型与主题覆盖见 `internal/theme/tokens.md`。

| semantic路径 | 类型 | 浅色值 |
| --- | --- | --- |
| `color.state.tabHover` | color | `rgba(255,255,255,.45)` |
| `color.state.railHover` | color | `rgba(0,0,0,.045)` |
| `color.state.railPressed` | color | `{palette.black.a12}` |
| `color.status.scrollbar` | color | `rgba(0,0,0,.32)` |
| `color.status.dividerHandle` | color | `{palette.gray.400}` |
| `color.status.danger` | color | `#dc2626` |
| `size.windowWidth` | dimension | `1180` |
| `size.windowHeight` | dimension | `780` |
| `size.windowMinWidth` | dimension | `1000` |
| `size.windowMinHeight` | dimension | `640` |
| `size.trafficLightY` | dimension | `17` |
| `size.siderMinWidth` | dimension | `236` |
| `size.siderMaxWidth` | dimension | `320` |
| `size.paneMinWidth` | dimension | `280` |
| `size.paneMinHeight` | dimension | `160` |
| `size.dragPreviewWidth` | dimension | `360` |
| `size.dragPreviewHeight` | dimension | `260` |
| `size.tabDetachDistance` | dimension | `8` |
| `size.dragGrabX` | dimension | `30` |
| `size.dragGrabY` | dimension | `18` |
| `size.paneEnterOffset` | dimension | `12` |
| `size.dividerHandleWidth` | dimension | `3` |
| `size.dividerHandleHeight` | dimension | `28` |
| `size.dividerKeyboardStep` | dimension | `8` |
| `size.previewInset` | dimension | `4` |
| `size.paneActionRevealPadding` | dimension | `40` |
| `size.hostDialogWidth` | dimension | `340` |
| `size.groupDialogWidth` | dimension | `320` |
| `size.portFieldWidth` | dimension | `90` |
| `size.dialogTitleSize` | dimension | `18` |
| `size.tabContentHeight` | dimension | `20` |
| `size.tabStripHeight` | dimension | `36` |
| `size.tabDragHandleInset` | dimension | `30` |
| `size.tabTitleOffset` | dimension | `-1.5` |
| `size.tabCountOffset` | dimension | `-0.5` |
| `size.scrollbarMinThumb` | dimension | `24` |
| `size.scrollbarRevealZone` | dimension | `24` |
| `size.terminalContentBottom` | dimension | `50` |
| `size.emptyPaneTitleSize` | dimension | `15` |
| `size.statsHeaderHeight` | dimension | `44` |
| `size.statsChartHeight` | dimension | `34` |
| `size.statsChartStroke` | dimension | `1.3` |
| `size.statsSectionGap` | dimension | `20` |
| `font.weight.emptyPaneTitle` | fontWeight | `550` |
| `font.lineHeight.keycap` | dimension | `12` |
| `glass.paneActionBlur` | dimension | `4` |
| `opacity.dragRotation` | number | `-1` |
| `opacity.dragPreview` | number | `0.9` |
| `opacity.previewHeader` | number | `0.96` |
| `opacity.paneCloseScale` | number | `0.97` |
| `opacity.dropEdgeRatio` | number | `0.3` |
| `opacity.paneActionTint` | number | `0.9` |
| `motion.duration.frameInterval` | duration | `16` |
| `motion.duration.zoom` | duration | `240` |
| `motion.duration.paneEnter` | duration | `220` |
| `motion.duration.paneClose` | duration | `160` |
| `motion.duration.dividerFade` | duration | `120` |
| `motion.duration.sidebar` | duration | `220` |
| `motion.duration.scrollbarHold` | duration | `700` |
| `motion.duration.scrollbarFade` | duration | `180` |
| `motion.duration.instantNonzero` | duration | `1e-06` |
| `motion.easing.pane` | cubicBezier | `[0.2, 0.8, 0.2, 1]` |
| `motion.spring.zoom` | spring | `{'duration': 240, 'bounce': 0}` |
| `size.emptyPaneIcon` | dimension | `32` |
| `size.bellIcon` | dimension | `13` |
| `size.catalogRowHeight` | dimension | `40` |
| `color.surface.liftStack` | color | `#f9f9fa`（dark 使用 graphite.650） |
