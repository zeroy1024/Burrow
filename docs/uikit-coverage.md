# UIKit 69项目标覆盖报告

基线为 `f86abca`；最终冻结副本共 141 个文件，已做文件哈希核对。提交前 138 个运行时代码/模块/资源文件仍与已测副本一致，其余 3 个原有文档补充完成记录。报告按当前实际页面、组合和最新合同分类，不把同名函数或空壳视为完整通用规范。

## 覆盖口径

69项目标：已接入API 48 项；组合覆盖 19 项；无调用不纳入 2 项。原prompt中与最新合同冲突的额外能力在后表明确标记。

组合覆盖包含原生SDK宿主和多个现有组件的等效组合，不声称实现未使用的Tone/Size/新键盘能力。

## 69项逐项映射

| 层 | 目标 | 分类 | UIKit实际API/组合 | 当前范围 |
|---|---|---|---|---|
| L1.1 | Text | 组合覆盖 | `Text` (`internal/uikit/text.go:6`) | 薄Text入口未被desktop直接调用；实际排版通过Pane/Sider/Table/Stat/KeyValue等已接入组件组合，页面仍可使用原生Text作内容。未宣称实现TextStyle/Tone全矩阵。 |
| L1.2 | Icon | 组合覆盖 | `Icon` (`internal/uikit/icon.go:15`) | 薄Icon入口未被desktop直接调用；IconButton、LineIcon和领域图标承担现有图标外观。未新增IconName注册表或Size/Tone矩阵。 |
| L1.3 | Button | 已接入API | `Button` (`internal/uikit/button.go:22`) |  |
| L1.4 | IconButton | 已接入API | `IconButton` (`internal/uikit/icon.go:17`) |  |
| L1.5 | Kbd | 组合覆盖 | `Kbd` (`internal/uikit/kbd.go:26`) | Kbd由SearchField内部使用；终端窗格快捷键标签按PaneShortcut领域组件保留原几何。 |
| L1.6 | Badge | 组合覆盖 | `ConnectionBadge` (`internal/uikit/domain.go:58`), `ProtocolBadge` (`internal/uikit/domain.go:73`), `TransferBadge` (`internal/uikit/transfer.go:160`) | 由ConnectionBadge、ProtocolBadge、TransferBadge覆盖当前页面；没有通用Badge Tone/Size矩阵。 |
| L1.7 | Count | 组合覆盖 | `SiderGroupContents` (`internal/uikit/sider.go:79`), `SessionTabContents` (`internal/uikit/tabs.go:86`) | 由SiderGroupContents、SessionTabContents及传输筛选标签覆盖真实计数；没有独立CountAccent新表现。 |
| L1.8 | StatusDot | 组合覆盖 | `ConnectionDot` (`internal/uikit/domain.go:38`), `TabConnectionIndicator` (`internal/uikit/tabs.go:106`) | ConnectionDot与TabConnectionIndicator覆盖现有状态点/环；保留当前Pane/拖拽预览位置。 |
| L1.9 | StatusPill | 组合覆盖 | `ConnectionBadge` (`internal/uikit/domain.go:58`), `ProtocolBadge` (`internal/uikit/domain.go:73`), `TransferBadge` (`internal/uikit/transfer.go:160`) | 由连接、协议及传输Badge组合覆盖当前状态；没有同名通用StatusPill或新状态矩阵。 |
| L1.10 | Spinner | 组合覆盖 | `TabConnectionIndicator` (`internal/uikit/tabs.go:106`), `TestResult` (`internal/uikit/feedback.go:47`) | TabConnectionIndicator与TestResult内部的SDK Spinner覆盖现有进行中指示；没有额外独立Spinner消费者。 |
| L1.11 | Divider | 已接入API | `Divider` (`internal/uikit/feedback.go:14`), `SettingsDivider` (`internal/uikit/settings.go:65`) |  |
| L1.12 | Tip | 已接入API | `Tip` (`internal/uikit/tooltip.go:15`) |  |
| L2.1 | Input | 已接入API | `Input` (`internal/uikit/input.go:21`) |  |
| L2.2 | PasswordInput | 已接入API | `PasswordInput` (`internal/uikit/input.go:84`) |  |
| L2.3 | PortInput | 组合覆盖 | `Input` (`internal/uikit/input.go:21`) | 现有端口由Input字符串草稿及应用校验组合实现；空串/编辑中状态保留，没有把它强换成即时数值写入的NumberInput。 |
| L2.4 | SearchField | 已接入API | `SearchField` (`internal/uikit/search.go:21`) |  |
| L2.5 | Select[T] | 已接入API | `Select` (`internal/uikit/select.go:9`) | 已接入现有string Select与SelectNative；定制scheme/private-key项由应用绑定并复用公共样式。没有声称提供原prompt全部泛型Option/Disabled矩阵。 |
| L2.6 | Segmented | 已接入API | `Segmented` (`internal/uikit/segmented.go:13`) |  |
| L2.7 | Switch | 已接入API | `Switch` (`internal/uikit/toggle.go:6`) |  |
| L2.8 | Checkbox | 无调用不纳入 | — | 当前页面没有独立Checkbox消费者，未新增空壳。 |
| L2.9 | Radio[T] | 无调用不纳入 | — | 当前页面没有独立泛型Radio消费者；设置里的定制单选项属于明确应用composition。 |
| L2.10 | ChipInput | 已接入API | `ChipInput` (`internal/uikit/chipinput.go:11`), `Chip` (`internal/uikit/chipinput.go:29`), `ChipEntry` (`internal/uikit/chipinput.go:52`) |  |
| L2.11 | FormRow | 已接入API | `FormRow` (`internal/uikit/form.go:19`) |  |
| L2.12 | FormSection | 组合覆盖 | `Section` (`internal/uikit/section.go:16`), `SettingsGroup` (`internal/uikit/settings.go:31`) | 现有分组由Section/SettingsGroup/FormRow组合覆盖；不增加额外容器改变层级。 |
| L2.13 | FieldMessage | 已接入API | `FieldMessage` (`internal/uikit/form.go:47`) |  |
| L2.14 | Banner | 已接入API | `Banner` (`internal/uikit/feedback.go:17`) | 实际用于已打开会话编辑提示，沿用原Dialog notice样式；没有新增任意Tone/Action矩阵。 |
| L2.15 | SettingsGroup | 已接入API | `SettingsGroup` (`internal/uikit/settings.go:31`) |  |
| L2.16 | SettingsRow | 已接入API | `SettingsRow` (`internal/uikit/settings.go:79`) |  |
| L3.1 | Card | 已接入API | `Card` (`internal/uikit/card.go:8`) |  |
| L3.2 | Pane | 已接入API | `Pane` (`internal/uikit/pane.go:13`), `FilePane` (`internal/uikit/filepane.go:16`) |  |
| L3.3 | PaneHeader | 已接入API | `PaneHeader` (`internal/uikit/pane.go:57`), `FilePaneHeader` (`internal/uikit/filepane.go:26`) |  |
| L3.4 | PaneFooter | 已接入API | `PaneFooter` (`internal/uikit/pane.go:112`), `FilePaneFooter` (`internal/uikit/filepane.go:53`) |  |
| L3.5 | SidePanel | 组合覆盖 | `SidePanelBody` (`internal/uikit/sidepanel.go:8`), `SidePanelHeader` (`internal/uikit/sidepanel.go:16`), `SidePanelViewport` (`internal/uikit/sidepanel.go:27`) | 由SidePanelBody/Header/Viewport及应用开关/滚动状态组合。 |
| L3.6 | Section | 已接入API | `Section` (`internal/uikit/section.go:16`) |  |
| L3.7 | Modal | 已接入API | `Modal` (`internal/uikit/modal.go:20`), `Dialog` (`internal/uikit/modal.go:44`) |  |
| L3.8 | Popover | 组合覆盖 | `StylePopover` (`internal/uikit/popover.go:8`) |  |
| L3.9 | Menu | 组合覆盖 | `Element.Menu/ContextMenu`、`ui.Menu` | UIKit返回的Element.Menu/ContextMenu及ui.Menu使用系统绘制；菜单内容、locale和业务选择由应用负责。没有自绘或同名Menu封装。 |
| L3.10 | ToastViewport / Toast | 组合覆盖 | `Notify` (`internal/uikit/feedback.go:12`) | Notify复用框架c.Toast通道；viewport、计时与宿主行为由框架负责，不重复挂载，不新增OnAction业务回调。 |
| L3.11 | EmptyState | 已接入API | `EmptyState` (`internal/uikit/empty.go:45`), `EmptyMessage` (`internal/uikit/empty.go:34`), `TransferDockEmpty` (`internal/uikit/transfer.go:20`) | EmptyState覆盖终端/文件空工作区与连接态；EmptyMessage覆盖原内联空目录/无搜索结果；TransferDockEmpty保留原紧凑文字空态。 |
| L3.12 | ScrollFade | 已接入API | `ScrollFade` (`internal/uikit/scrollfade.go:15`) |  |
| L4.1 | Rail | 已接入API | `Rail` (`internal/uikit/rail.go:8`) |  |
| L4.2 | RailButton | 已接入API | `RailButton` (`internal/uikit/rail.go:25`) |  |
| L4.3 | Sider | 已接入API | `Sider` (`internal/uikit/sider.go:12`), `SiderHeader` (`internal/uikit/sider.go:25`), `SiderFooter` (`internal/uikit/sider.go:38`) |  |
| L4.4 | SiderGroup | 已接入API | `SiderGroup` (`internal/uikit/sider.go:55`) |  |
| L4.5 | SiderRow | 已接入API | `SiderRow` (`internal/uikit/sider.go:115`) |  |
| L4.6 | SessionTabs | 组合覆盖 | `SessionTabsViewport` (`internal/uikit/titlebar.go:21`), `SessionTabsScroll` (`internal/uikit/titlebar.go:25`), `SessionTabsRow` (`internal/uikit/titlebar.go:33`) | 由SessionTabsViewport/Scroll/Row、SessionTab及应用Tab状态/重排/拆离/停靠组合；没有替换成TabsBase。 |
| L4.7 | SessionTab | 已接入API | `SessionTab` (`internal/uikit/tabs.go:40`) |  |
| L4.8 | PathBar | 已接入API | `PathBar` (`internal/uikit/pathbar.go:32`) |  |
| L4.9 | HistoryButtons | 已接入API | `HistoryButtons` (`internal/uikit/filepane.go:101`) |  |
| L4.10 | PaneLabel | 已接入API | `PaneLabel` (`internal/uikit/filepane.go:79`) |  |
| L4.11 | CountTabs | 组合覆盖 | `TransferFilters` (`internal/uikit/transfer.go:59`) | 由TransferFilters的分段项与现有计数标签组合；计数格式与业务过滤状态仍由应用提供。 |
| L4.12 | NavItem | 组合覆盖 | `SettingsNavItem` (`internal/uikit/settings.go:102`) | SettingsNavItem实现现有设置页导航。 |
| L5.1 | DataTable | 已接入API | `DataTable` (`internal/uikit/table.go:40`) | 当前5/3列、固定token列宽、path行Key及原name选择策略已接入；列表业务状态与Drop/菜单/键盘由应用保留。 |
| L5.2 | SortHeader | 已接入API | `SortHeader` (`internal/uikit/sortheader.go:21`) | 仅名称/大小/修改时间可排序，权限/所有者保持禁用，列宽未新增拖拽能力。 |
| L5.3 | ListRow | 已接入API | `ListRow` (`internal/uikit/listrow.go:15`) |  |
| L5.4 | KeyValue | 已接入API | `KeyValue` (`internal/uikit/keyvalue.go:14`) |  |
| L5.5 | Meter | 已接入API | `Meter` (`internal/uikit/meter.go:9`), `MeterStat` (`internal/uikit/stat.go:17`) | Meter/MeterStat保留原中性色和尺寸；没有新增70%/90%阈值变色。 |
| L5.6 | Progress | 已接入API | `Progress` (`internal/uikit/progress.go:16`) | 确定进度已接入当前五个传输状态，颜色、Grow spring及progress键保持；没有新增不定进度。Role/Range仅做源码对照，未作运行时AX Range验证。 |
| L5.7 | Sparkline | 已接入API | `Sparkline` (`internal/uikit/sparkline.go:14`) | CPU原15点曲线与样式迁入；诊断验证绘制节点存在、相关几何/样式，未做像素验收。 |
| L5.8 | Stat | 已接入API | `Stat` (`internal/uikit/stat.go:8`) | Stat与MeterStat承载当前监控标题/数值/分组；不增加通用新布局。 |
| L5.9 | TransferRow | 已接入API | `TransferRow` (`internal/uikit/transfer.go:87`), `TransferIcon` (`internal/uikit/transfer.go:98`), `TransferNameColumn` (`internal/uikit/transfer.go:106`), `TransferName` (`internal/uikit/transfer.go:110`), `TransferPath` (`internal/uikit/transfer.go:115`), `TransferProgressColumn` (`internal/uikit/transfer.go:120`), `TransferSpeedColumn` (`internal/uikit/transfer.go:133`), `TransferDetail` (`internal/uikit/transfer.go:137`), `TransferStatus` (`internal/uikit/transfer.go:145`), `TransferActions` (`internal/uikit/transfer.go:149`), `TransferBadge` (`internal/uikit/transfer.go:160`) | 完整覆盖是TransferRow与TransferIcon/NameColumn/Name/Path/ProgressColumn/SpeedColumn/Detail/Status/Actions/Badge及Progress的组合，不能仅凭同名行壳计数；状态/格式/动作仍由应用负责。 |
| L6.1 | OsLogo | 已接入API | `OsLogo` (`internal/uikit/domain.go:11`) | 沿用原SVG和品牌底色；素材与未知系统策略保持。 |
| L6.2 | ProtocolIcon | 已接入API | `ProtocolIcon` (`internal/uikit/domain.go:19`) |  |
| L6.3 | HostLabel | 组合覆盖 | `SiderRowLabel` (`internal/uikit/sider.go:123`), `PaneTitle` (`internal/uikit/pane.go:74`), `PaneLogin` (`internal/uikit/pane.go:79`) | SiderRowLabel与PaneTitle/PaneLogin覆盖现有标题/主机文字；不添加额外嵌套结构。 |
| L6.4 | Latency | 已接入API | `Latency` (`internal/uikit/domain.go:83`) |  |
| L6.5 | PaneBadge | 组合覆盖 | `PaneShortcut` (`internal/uikit/pane.go:63`) | PaneShortcut实现当前窗格快捷键标签；没有更换原键帽外观。 |
| L6.6 | FileIcon | 已接入API | `FileIcon` (`internal/uikit/domain.go:29`) | 保持目录/普通文件图标和既有尺寸/颜色；未新增扩展名映射行为。 |
| L6.7 | TestResult | 已接入API | `TestResult` (`internal/uikit/feedback.go:47`) | 测试结果UI与tooltip已接入，应用仍管理测试任务/结果及取消；诊断不触发真实网络/Secrets。 |
| L6.8 | TerminalSurface | 已接入API | `TerminalSurface` (`internal/uikit/terminalsurface.go:13`), `PaneCard` (`internal/uikit/pane.go:18`) | UIKit拥有背景对比度纯策略；应用保留随accent失效的真实缓存、生命周期与滚动条选色。每会话scheme/card背景及chrome隔离已做无窗口对照。 |

## 明确保留的原生宿主composition

这7处保留内容绑定、locale、录制/输入作用域或应用业务动作；公共外观仍复用UIKit。没有为扫描归零增加无价值的一行Base别名。

| 源码 | 原生宿主 | 应用职责 |
|---|---|---|
| `internal/desktop/settings_licenses.go:75` | `PopoverBase` | 许可数据、locale、anchor与开关绑定属于应用内容；复用UIKit StylePopover。 |
| `internal/desktop/credentials_ui.go:55` | `ButtonBase` | 指纹内容、locale与本机剪贴板动作由应用管理，保留原宿主节点。 |
| `internal/desktop/settings_themes.go:76` | `SelectBase` | 终端配色绑定及定制内容保留，复用Select popup等公共样式。 |
| `internal/desktop/session_modal_advanced.go:66` | `SelectBase` | 每会话scheme与跟随全局预览依赖应用数据和绑定。 |
| `internal/desktop/session_modal.go:196` | `SegmentedBase` | 协议草稿绑定及changeProtocol业务保留，复用分段公共样式。 |
| `internal/desktop/session_modal.go:424` | `SelectBase` | 私钥ID与应用目录数据绑定保留，复用Select popup等公共样式。 |
| `internal/desktop/settings.go:242` | `ButtonBase` | 设置分段按钮组的索引、locale、点击回调及原 FocusGroup 键盘语义由应用负责。 |

系统菜单由原生SDK绘制，UIKit返回的Element承接Menu/ContextMenu；catalogHostMenu等业务菜单内容留在应用。没有新增自绘菜单或同名Menu封装。

## 原prompt中未纳入当前合同的能力

| 目标 | 分类 | 处理依据 |
|---|---|---|
| Spinner | 与最新合同冲突不采纳 | 保留reduce-motion三态和当前spinner策略，不按原prompt强制reduce仍旋转。 |
| Tip | 与最新合同冲突不采纳 | 保留当前TooltipBase定位/offset/样式；统一改用 Element.Tooltip 会改变当前的定位与样式，按既有合同保留 TooltipBase。 |
| Modal | 与最新合同冲突不采纳 | 保留现有退出提前 return、scrim、焦点、保存及测试生命周期；不加入 Dirty 二次确认或改动 Enter/⌘S/⌘T 快捷键。 |
| ToastViewport / Toast | 与最新合同冲突不采纳 | 保留框架原生toast生命周期/错误通道，避免重复挂viewport；原OnAction业务回调与无业务callback规则矛盾。 |
| Rail | 与最新合同冲突不采纳 | 保持三页平级、重复点击只进入，切页清理Settings临时状态，SSH/Files后台保留；页面激活仍经 switchSection，不让组件直接写 page 指针或重新引入 toggle。 |
| SiderRow | 与最新合同冲突不采纳 | 保留Draggable/自有目录重排、跨组、取消、与Pane停靠；不能机械替换成原prompt的ListState.Reorder/Drag。 |
| SessionTabs | 与最新合同冲突不采纳 | 保留现有自定义Tab/Pane拖拽/拆离/停靠和稳定句柄，不能改成TabsBase丢弃这些行为。 |
| DataTable | 与最新合同冲突不采纳 | 原prompt要求ui.List重写、列宽拖拽、额外键盘/读屏语义；本轮保持当前按name选择/path键、5/3列和固定列宽、原业务事件。 |
| SortHeader | 与最新合同冲突不采纳 | 保持仅名称/大小/修改时间可排序；权限/所有者禁用，保留原列宽。 |
| Meter | 与最新合同冲突不采纳 | 保留当前中性常态色；70%/90%新阈值变色不能在纯抽取中偷偷应用。 |
| Progress | 与最新合同冲突不采纳 | 保留五个已有传输状态的颜色、进度宽度和reduce-motion；未出现的不定进度新行为不新增。 |
| TerminalSurface | 与最新合同冲突不采纳 | 必须按每会话scheme/背景提供chrome，不能用全局theme.Install覆盖其他pane或持有旧主题。 |
| 原prompt验证/画廊/CI要求 | 与最新合同冲突不采纳 | 本轮不新增永久测试、组件画廊、截图、像素验收或新的CI禁用规则；仅复用临时无窗口fixture。 |
| 无消费者的通用变体矩阵 | 无调用不纳入 | 未新增通用Button Danger/Loading、Badge Tone/Size、TextStyle/Tone、IconName注册表、泛型Checkbox/Radio等未被当前页面消费的能力；不以空壳计数。 |

## 应用侧保留职责

- `windowColor`：平台窗口颜色字符串格式转换，非公共控件样式。
- `app.sidebarSearch`：获取应用快捷键绑定、active-page目标和稳定Handle，再调用UIKit.SearchField。
- `terminalSurface/updateTerminalSurfaceAccent`：应用持有accent surfaces与按背景缓存，负责失效/生命周期；纯对比度策略在UIKit。
- `terminalScrollbarColor`：从应用accent surfaces按对比度选择滚动条颜色，保留应用选色责任。

旧纯样式aliases在desktop及既有测试中为0。原生Row/Column/Box/Text/Icon继续用于页面内容与布局组合；这不代表遗漏一套公共控件外观。

## 验证与证据边界

14组隔离检查通过，基线与最终版日志及有效结构化观测值一致。覆盖77个会话/设置矩形、21场景554个导航/工作区矩形与326条关键层级记录、9块拖拽/调整、8个文件列表快照、传输五态两种宽度、监控两种高度及4种会话背景组合。后台会话/传输、paused保持、稳定Handle和设置清理/保存也已验证。

- 无窗口fixture，仅固定伪数据；不访问真配置、Secrets、网络或OS Sheet。
- 未生成截图或开展像素/视觉验收。
- 结构比较限选定节点/祖先记录；图表验证paintFn存在及几何/样式，不证明每像素或每条绘制路径。
- AX Role/Range保留仅由源码对照确认；早期observer的Range全零tuple未赋值，不构成运行时AX证据。
- Files/Transfers早期observer结构含无效Range零tuple，后续版本改为Monitor Draw bool；同一实际测量逻辑和有效断言未变，生产源未改。比较只移除无效零tuple并将缺失Draw解码为false；Monitor真实Draw=true与paintFn断言仍比较。

进度语义源码对照：基线 `internal/desktop/ftp_transfers.go:504` 与最终 `internal/uikit/progress.go:19` 均保留 `Role(ui.RoleProgress).Range(0, 1, float64(value))`（最终值来自Props）。零tuple未用作AX Range运行验证。
