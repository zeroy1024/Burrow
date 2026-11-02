# UIKit 原始规格与接入覆盖清单

本轮重新读取 `SSH-Tool-UIKit.zip` 的 `prompt.md` 全文与三张设计图，基于当前源码补齐公共能力和页面接入。此前“48/49 个 API、组合覆盖、没有调用不实现”的统计不能证明设计完成，本表取代该口径。

## 分类和证据口径

- **完整实现**：该项当前合同中的子能力有真实渲染、事件或状态适配，并已接入适用页面；变体没有当前消费者时，公共能力仍实现，不强塞页面。
- **部分实现**：仍缺具体子能力或存在适用页面未接入。
- **未实现**：没有真实能力实现；不能凭同名函数或原生薄包装改为完整。
- **明确不适用**：仅用于下表明确列出的用户决定或固定 SDK 差异，不能按“当前没有调用”扩大到整个组件。

新增共享滚动条API及逐页面清单见[滚动条接入](scrollbars.md)。

后端审查整改保持上述公共组件与布局。文件目录链接通过`fileNavigation.link`和可选`DirectoryResolver`进入原导航路径，权限、精确名称、续传及native初始网格只接业务/终端层；逐项API、消费者及证据见[14项整改](mvp-remediation.md)。没有为这些后端能力新增UI平行组件。

2026-11-02真实后端接入后，终端正文改用固定MyGo的原生VT View；容器、表单、导航仍沿用UIKit。本表公共组件规格不代表原生正文具有全部UIKit排版/滚动能力。按用户后续决定，字距差异允许忽略、粗体提亮开关移除、滚动条只需相同风格；实时回滚使用固定终端包的最小公开扩展，按物理显示行设置并接受页级误差，见[MVP实际能力](mvp-status.md)。源码接入状态与本轮验证证据分开记录，不沿用原演示渲染结论。

状态是源码合同及调用链审计结论，**不等于所有交互、像素、VoiceOver 行为都已设备实测**。已有测试/构建/静态检查与未验证项分开记录在末节。组件数量及编译结果不作为逐项规格完成的替代证据。

## 分阶段推进

| 阶段 | 内容 | 交付 |
|---|---|---|
| 0 | 收尾当前12项修复、保留未提交工作 | 原修复留在工作树；实施前已保存差异和源码manifest |
| 1 | 重读原稿/三图、规格差距与令牌/基础层 | 语义类型、L1完整能力、专用token与每帧主题派生 |
| 2 | 表单/Modal与设置、会话实际接入 | 泛型表单、完整Chip/Search、Dirty确认、标准控件消费者 |
| 3 | 菜单/浮层/容器/导航 | 目录右键/更多共用Popover及模型、其他系统菜单适配、Toast生命周期、Header/Footer、导航和标签键盘 |
| 4 | 数据/领域与虚拟化 | 选择/源保活/Reveal/列宽/AX适配、阈值/不定进度/领域映射及页面 |
| 5 | 完整规格复核、文档与验证 | 逐项表、用户决定、SDK限制和验证结果；goal仅在全部授权事项完成后结束 |

## 69项逐项核对

| 编号 | 组件 | 状态 | 实际公共API | 已实现的子能力 | 页面消费者/适用接入 |
|---|---|---|---|---|---|
| L1.1 | Text | 完整实现 | [Text](../internal/uikit/text.go#L54) / [TextProps](../internal/uikit/text.go#L24) / [TextStyle](../internal/uikit/text.go#L10) / [TextFont](../internal/uikit/text.go#L30) | 九档排版、语义Tone、Mono与tnum数字；TextFont共享实测/富文本排版 | 设置及高亮slot、凭据、会话、侧栏、监控、文件页脚与传输普通文字 |
| L1.2 | Icon | 完整实现 | [Icon](../internal/uikit/icon.go#L48) / [IconName](../internal/uikit/iconsource.go#L11) / [IconProps](../internal/uikit/icon.go#L29) / [IconSVG](../internal/uikit/iconsource.go#L101) | 命名图标库和SVG素材入口；12/14/16尺寸、Tone；描边参与缓存key | 设置、凭据、菜单、表单及文件图标 |
| L1.3 | Button | 完整实现 | [Button](../internal/uikit/button.go#L30) / [ButtonProps](../internal/uikit/button.go#L19) | Secondary/Primary/Plain/Danger/DangerPlain；Sm/Md/Lg、前置图标、Kbd、Loading、禁用/焦点/交互态；危险及Disabled均由Props进入 | 会话提交、凭据/文件/站点删除、标签关闭确认、设置快捷键 |
| L1.4 | IconButton | 完整实现 | [IconButton](../internal/uikit/icon.go#L64) / [IconButtonProps](../internal/uikit/icon.go#L9) | Xs/Sm/Md、Filled/OnWindow/Plain、Toggle、Shortcut、Disabled、局部Surface；构造前稳定Key，IconOn按本帧Toggle值解析；状态在Props内传入 | 标题栏、更多/关闭、输入清除/眼睛、传输操作、PaneToggle；目录按钮使用Surface保留几何 |
| L1.5 | Kbd | 完整实现 | [Kbd](../internal/uikit/kbd.go#L26) / [KbdProps](../internal/uikit/kbd.go#L19) | Keycap/Flat/OnInk/OnAccent；装饰角色及描述；不注册或修改绑定 | 搜索、提交按钮、快捷键设置、菜单、Tooltip |
| L1.6 | Badge | 完整实现 | [Badge](../internal/uikit/badge.go#L17) / [BadgeProps](../internal/uikit/badge.go#L10) | 六种语义Tone、Dot、两档尺寸与局部Surface | 协议说明、传输排队及StatusPill |
| L1.7 | Count | 完整实现 | [Count](../internal/uikit/badge.go#L63) / [CountProps](../internal/uikit/badge.go#L59) | Quiet/Accent/Inline；tnum数字、胶囊/行内样式 | 分组、Tab附加窗格数、传输CountTabs |
| L1.8 | StatusDot | 完整实现 | [StatusDot](../internal/uikit/spinner.go#L98) / [StatusDotProps](../internal/uikit/spinner.go#L93) / [ConnState.Label](../internal/uikit/tabs.go#L22) | 断开环、连接中、已连接光晕、错误；Label；减少动态按确认方案静态 | SSH/文件Tab；未加入Sider |
| L1.9 | StatusPill | 完整实现 | [StatusPill](../internal/uikit/badge.go#L87) / [StatusPillProps](../internal/uikit/badge.go#L83) | 连接/连接中/断开/失败/进行中/完成/暂停七态；统一Badge和局部Surface | 终端页脚及传输行 |
| L1.10 | Spinner | 完整实现 | [Spinner](../internal/uikit/spinner.go#L18) / [TabSpinner](../internal/uikit/spinner.go#L62) / [SpinnerProps](../internal/uikit/spinner.go#L11) | 普通三尺寸、Tab环形、主题周期和减少动态；SDK原生进度角色 | Loading按钮、连接测试、Tab |
| L1.11 | Divider | 完整实现 | [Divider](../internal/uikit/divider.go#L13) / [DividerProps](../internal/uikit/divider.go#L8) | 水平/竖向、Inset；统一边线色与12pt竖线 | 监控分组、公共Footer模板和菜单 |
| L1.12 | Tip | 完整实现 | [Tip](../internal/uikit/tooltip.go#L15) / [StyleTooltip](../internal/uikit/tooltip.go#L9) | SDK TooltipBase计时/定位/关闭；Label及KbdOnInk快捷键 | 图标操作、文件历史、窗格分栏及输入操作 |
| L2.1 | Input | 完整实现 | [Input](../internal/uikit/input.go#L29) / [InputProps](../internal/uikit/input.go#L13) | 前置图标、尾部slot、只读、禁用、错误与键盘焦点；返回真实文本控件 | 会话、凭据、设置目录、分组与文件操作 |
| L2.2 | PasswordInput | 完整实现 | [PasswordInput](../internal/uikit/input.go#L123) / [PasswordParts](../internal/uikit/input.go#L119) | Local reveal、动态眼睛/标签；完整Input属性与真实Input/Toggle事件 | 会话密码/私钥口令及凭据口令 |
| L2.3 | PortInput | 完整实现 | [PortInput](../internal/uikit/input.go#L151) | 字符串中间态和范围错误；不以即时float写回破坏编辑 | SSH/SFTP/FTP端口，默认值/手改标记仍由草稿处理 |
| L2.4 | SearchField | 完整实现 | [SearchField](../internal/uikit/search.go#L33) / [SearchProps](../internal/uikit/search.go#L17) | Sunken/Filter、清除、Esc、Kbd、稳定Handle及整框点击；真实Input焦点驱动外框InputFocus，无新增Border | 三页侧栏及本地/远程文件过滤 |
| L2.5 | Select[T] | 完整实现 | [Select](../internal/uikit/select.go#L28) / [Option[T]](../internal/uikit/select.go#L11) / [SelectProps](../internal/uikit/select.go#L19) / [SelectPopupViewport](../internal/uikit/select.go#L144) | 泛型、单项禁用且键盘跳过、定制Leading/Detail、占位、统一滚动弹层 | 分组/跳板/编码、配色、私钥、语言/字体/密钥类型 |
| L2.6 | Segmented | 完整实现 | [Segmented](../internal/uikit/segmented.go#L34) / [SegItem](../internal/uikit/segmented.go#L9) / [SegmentedProps](../internal/uikit/segmented.go#L18) | 图标、计数、尺寸、Fill、方向键；构造前Key，Track显式聚焦转发选中Radio | 认证、协议、FTP模式/TLS、设置与CountTabs |
| L2.7 | Switch | 完整实现 | [Switch](../internal/uikit/toggle.go#L14) / [ToggleProps](../internal/uikit/toggle.go#L8) | SwitchBase、C.Switch绘制/动画、减少动态、禁用及标签 | 设置开关及外观预览 |
| L2.8 | Checkbox | 完整实现 | [Checkbox](../internal/uikit/toggle.go#L52) / [CheckProps](../internal/uikit/toggle.go#L48) | CheckboxBase、C.Check、Mixed标记及AX、禁用与焦点 | 记住口令、重名冲突应用全部；Mixed为完整可用变体 |
| L2.9 | Radio[T] | 完整实现 | [Radio](../internal/uikit/toggle.go#L92) / [CheckProps](../internal/uikit/toggle.go#L48) | RadioBase泛型值、选择与标签、C.Check外观和禁用 | 应用主题卡单选 |
| L2.10 | ChipInput | 完整实现 | [ChipInput](../internal/uikit/chipinput.go#L59) / [ChipInputProps](../internal/uikit/chipinput.go#L12) / [ChipInputResult](../internal/uikit/chipinput.go#L23) | 录入/逗号/Enter/失焦/Backspace/删除、最终校验与Changed、重复值稳定身份 | 环境变量使用PreserveCommas和外部草稿，逗号保留 |
| L2.11 | FormRow | 完整实现 | [FormRow](../internal/uikit/form.go#L29) / [FormRowProps](../internal/uikit/form.go#L16) / [FormRowParts](../internal/uikit/form.go#L23) | 固定标签列、Help/Error/Align，命名并聚焦真实控件 | 会话与凭据表单 |
| L2.12 | FormSection | 完整实现 | [FormSection](../internal/uikit/form.go#L66) | 主题标题、间距、语义组及body | 会话字段、认证/高级与凭据表单 |
| L2.13 | FieldMessage | 完整实现 | [FieldMessage](../internal/uikit/form.go#L82) / [FieldMessageProps](../internal/uikit/form.go#L77) | 语义Tone、图标、错误提示与变化Announce | 表单帮助和错误 |
| L2.14 | Banner | 完整实现 | [Banner](../internal/uikit/feedback.go#L23) / [BannerProps](../internal/uikit/feedback.go#L12) / [BannerResult](../internal/uikit/feedback.go#L18) | Neutral/Info/Warning/Danger、Icon/Action事件；状态角色与错误Announce | 已打开会话编辑提示使用外层Padding约束满宽Banner，提示实际高度计入表单滚动预算，避免横向溢出；设置与凭据错误 |
| L2.15 | SettingsGroup | 完整实现 | [SettingsGroup](../internal/uikit/settings.go#L58) / [SettingsGroupProps](../internal/uikit/settings.go#L50) | 标题/body/footnote/header actions与统一组外观 | 全部设置分类及凭据分组 |
| L2.16 | SettingsRow | 完整实现 | [SettingsRow](../internal/uikit/settings.go#L110) / [SettingsRowProps](../internal/uikit/settings.go#L100) | Title/Detail/Control/RichText slots、行距、缩进分隔 | 设置项、高亮搜索结果、凭据内容 |
| L3.1 | Card | 完整实现 | [Card](../internal/uikit/card.go#L13) / [CardProps](../internal/uikit/card.go#L8) | 背景、圆角、0.5pt内描边、Padding和Clip | 导航/设置/监控卡片与工作区空态 |
| L3.2 | Pane | 完整实现 | [Pane](../internal/uikit/pane.go#L33) / [PaneProps](../internal/uikit/pane.go#L19) / [PaneSlots](../internal/uikit/pane.go#L27) | Terminal/File类型、焦点/失焦、语义组、slots与局部surface；File焦点只绘制边框，不调用Selected覆写底色 | 终端和文件窗格；生命周期不归组件 |
| L3.3 | PaneHeader | 完整实现 | [PaneHeader](../internal/uikit/pane.go#L132) / [HeaderProps](../internal/uikit/pane.go#L114) / [HeaderResult](../internal/uikit/pane.go#L125) | 36/40/46密度、标题/说明/Leading/Center、Actions/Toggle/关闭及稳定Key | 终端、文件工具栏、Sider和SidePanel |
| L3.4 | PaneFooter | 完整实现 | [PaneFooter](../internal/uikit/pane.go#L267) / [FooterProps](../internal/uikit/pane.go#L255) / [FormatSelection](../internal/uikit/pane.go#L309) | Leading、Items、竖分隔、Trailing及选择数量/大小模板 | 文件页脚和终端页脚，已有布局通过显式slots保持 |
| L3.5 | SidePanel | 完整实现 | [SidePanel](../internal/uikit/sidepanel.go#L43) / [SidePanelProps](../internal/uikit/sidepanel.go#L28) / [SidePanelResult](../internal/uikit/sidepanel.go#L38) | 标题/字幕/关闭、持有者提供稳定滚动句柄；共享Scrollbars及溢出渐隐，仍保留自定义overlay slot | 服务器状态面板，原偏移/显隐状态迁入同一公共实现 |
| L3.6 | Section | 完整实现 | [Section](../internal/uikit/section.go#L16) / [SectionProps](../internal/uikit/section.go#L9) | CollapsibleBase、展开语义、高度/摘要与减少动态 | 会话高级设置 |
| L3.7 | Modal | 完整实现 | [Modal](../internal/uikit/modal.go#L60) / [ModalProps](../internal/uikit/modal.go#L11) / [ModalParts](../internal/uikit/modal.go#L36) / [ModalResult](../internal/uikit/modal.go#L52) | 标准头/body/footer、Dirty确认、Submit、退出动画、焦点圈定/恢复；外部退出控制器；Compact居中/内容宽与ContentGap | 会话/凭据；分组、站点删除、文件动作/冲突、SSH及文件标签关闭、Shell窗格关闭、内部丢弃确认全部同一Modal，无旧Dialog壳；活动窗格复用ConfirmCloseActive偏好，确认后仅关闭原目标，取消不结束会话 |
| L3.8 | Popover | 完整实现 | [Popover](../internal/uikit/popover.go#L26) / [PopoverProps](../internal/uikit/popover.go#L17) / [ClosePopover](../internal/uikit/popover.go#L54) | 四向定位、宽度、外点/Esc、开关与关闭、SDK避让/焦点、减少动态 | 许可证内容及公共菜单弹层 |
| L3.9 | Menu | 完整实现 | [MenuEntry](../internal/uikit/menu.go#L10) / [MenuOn](../internal/uikit/menu.go#L81) / [MenuButton](../internal/uikit/menu.go#L76) / [RowMenu](../internal/uikit/menu.go#L60) / [RowMenuOn](../internal/uikit/menu.go#L66) / [ContextMenuOn](../internal/uikit/menu.go#L72) / [NativeMenu](../internal/uikit/menu.go#L27) | 共享模型和renderer；Popover图标/Kbd/勾选/禁用/危险项/子菜单/键盘；RowMenuOn双入口共享状态；ContextMenuOn只响应右键/ShiftF10，不覆盖行的左键、导航、角色及展开状态 | SSH目录条目右键和更多共用“连接主机/编辑主机/复制地址/删除主机”，文件目录对应“连接站点/编辑站点/复制地址/删除站点”；分组菜单使用“添加主机（或添加站点）/新建分组/修改组名/删除分组”四字文案；侧栏添加菜单复用对应添加/新建动作；标题栏/凭据/文件更多及其他原生菜单适配 |
| L3.10 | ToastViewport / Toast | 完整实现 | [Toast](../internal/uikit/toast.go#L20) / [ToastProps](../internal/uikit/toast.go#L10) / [ToastViewport](../internal/uikit/toast.go#L52) / [Notify](../internal/uikit/toast.go#L41) | 类型、标题、详情、操作/关闭、快捷键；复用SDK队列/超时/悬停暂停/announce | 根视图一次viewport；即时及延迟错误使用ToneDanger+标题/详情，忙碌和不可写警告分类，普通提示保留Notify及去重ID |
| L3.11 | EmptyState | 完整实现 | [EmptyState](../internal/uikit/empty.go#L49) / [EmptyProps](../internal/uikit/empty.go#L17) / [EmptyMessage](../internal/uikit/empty.go#L37) | 图标tile、标题/详情/错误、操作与紧凑空态 | 无工作区、连接态、设置搜索、目录搜索EmptyCatalogSearch、空目录与传输空态 |
| L3.12 | ScrollFade | 完整实现 | [ScrollFade](../internal/uikit/scrollfade.go#L18) / [Edge](../internal/uikit/types.go#L44) / [ScrollFadeProps](../internal/uikit/scrollfade.go#L9) | 上下边、默认渐隐、玻璃多层mask/实体回退、Uniform整段玻璃背景、可布局Root | 真实/旧文本终端上下栏与文件页头（工具栏及列头）/页脚统一1/3/5px多层渐进模糊，着色按92%→78%→0向内容区逐渐透明；真实终端以本地插件ViewportBackdrop提供实际相邻历史行，文件表格以TableProps.Backdrop提供实际栏下条目；SidePanel及设置保留渐隐 |
| L4.1 | Rail | 完整实现 | [Rail](../internal/uikit/rail.go#L8) / [RailButtonProps](../internal/uikit/rail.go#L16) | 纵向TabList、方向键移焦、Enter激活、底部设置入口 | 三页导航，原switchSection负责真正切页 |
| L4.2 | RailButton | 完整实现 | [RailButton](../internal/uikit/rail.go#L25) | 页面选中/悬停/按下/焦点、提示、稳定Handle | SSH、文件、设置按钮；重复点击仅进入 |
| L4.3 | Sider | 完整实现 | [Sider](../internal/uikit/sider.go#L22) / [SiderProps](../internal/uikit/sider.go#L9) / [SiderResult](../internal/uikit/sider.go#L17) / [SiderHeader](../internal/uikit/sider.go#L81) / [SiderHeaderProps](../internal/uikit/sider.go#L71) | Title/Add/Search/List/Footer完整结构与UI结果；Header.HideAdd保留统一高度而不构建按钮 | SSH与文件独立目录；设置复用SiderHeader.HideAdd与SearchField，统一标题/搜索位置，不持有连接/持久化 |
| L4.4 | SiderGroup | 完整实现 | [SiderGroup](../internal/uikit/sider.go#L130) / [SiderGroupProps](../internal/uikit/sider.go#L106) / [SiderGroupResult.Present](../internal/uikit/sider.go#L143) | Count/Open/Body折叠、drop/focus/highlight；标准与自定义控制器使用同一呈现 | 目录分组，保持平级catalog-items与原排序控制器；两页分组接公共ContextMenuOn；修改组名复用Input/Modal，校验空名及同页重名，原地更新名称并使用原子保存/失败回滚，保留组ID、条目和展开状态；删除使用Modal确认，组内条目并入同页“个人”或首个其他分组；每页最后一个分组不可删除，不删除连接配置或结束会话/任务 |
| L4.5 | SiderRow | 完整实现 | [SiderRow](../internal/uikit/sider.go#L263) / [SiderRowProps](../internal/uikit/sider.go#L235) / [SiderRowResult.Present](../internal/uikit/sider.go#L280) | Logo/HostLabel、双击/Enter、更多/ShiftF10、Drag/Drop；原生Focused驱动独立active底色，失焦清除；无外部Selected/Active/连接State字段 | SSH/文件目录共用；先track再Present，稳定源身份和单击仅聚焦；不读取活动会话 |
| L4.6 | SessionTabs | 完整实现 | [SessionTabs](../internal/uikit/tabs.go#L290) / [SessionTabsProps](../internal/uikit/tabs.go#L277) / [TabsResult](../internal/uikit/tabs.go#L267) | 完整viewport/scroll/row、标准items/reorder事件、自定义driver slots与公共输入；隐藏滚动条、共用滚轮横移；新增标签后一次定位尾部加号，保留原生横向wheel/触控板 | SSH/File标题栏均直接接入；BuildTabs返回原加号按钮；原docking controller、父级、滚动状态及源身份保留 |
| L4.7 | SessionTab | 完整实现 | [SessionTab](../internal/uikit/tabs.go#L55) / [SessionTabInput](../internal/uikit/tabs.go#L217) / [SessionTabWidth](../internal/uikit/tabs.go#L108) / [SessionTabPreview](../internal/uikit/tabs.go#L123) | 单会话四种连接状态；组合四宫格+Count；实际测宽与无交互完整预览，共用标签内容；方向/Home/End/中键、焦点、新建/选中露出 | 两页真实标签；SSH组合及标签ghost用同一四宫格；面板→标签Morph接真实预览；键盘与工作区焦点契约保留 |
| L4.8 | PathBar | 完整实现 | [PathBar](../internal/uikit/pathbar.go#L35) / [PathBarProps](../internal/uikit/pathbar.go#L14) / [PathBarParts](../internal/uikit/pathbar.go#L26) | 实测宽度折叠；普通段与“…”菜单统一返回Chosen/Target/Index | 本地/远程路径由单一结果入口导航；本地绝对目录拼接保留根斜杠，普通段和折叠菜单均使用同一绝对Target，避免相对应用工作目录解析；保留Items，不再由页面另绑OnClick |
| L4.9 | HistoryButtons | 完整实现 | [HistoryButtons](../internal/uikit/filepane.go#L101) / [HistoryButtonProps](../internal/uikit/filepane.go#L97) | 前进/后退、禁用、Kbd提示与UI事件；页面保留已确认键位 | 文件历史，实际⌘←/⌘→保持 |
| L4.10 | PaneLabel | 完整实现 | [PaneLabel](../internal/uikit/filepane.go#L75) | 本地/远程、聚焦/失焦与单窗格样式 | 文件工具栏 |
| L4.11 | CountTabs | 完整实现 | [CountTabs](../internal/uikit/titlebar.go#L45) | 小号Segmented与真实Count；完整图标/计数数据项；横向标题行显式AlignSelf(Center) | 传输全部/进行中/完成分类，与标题及工具按钮垂直居中 |
| L4.12 | NavItem | 完整实现 | [SettingsNavItem](../internal/uikit/settings.go#L153) / [SettingsNavItemProps](../internal/uikit/settings.go#L146) | 图标、标签、选中/悬停；FocusVisible+ControlFocus，关闭SDK默认环，应用保留分类选择 | 设置分类导航 |
| L5.1 | DataTable | 完整实现 | [DataTable](../internal/uikit/table.go#L82) / [TableState](../internal/uikit/table.go#L31) / [TableRow](../internal/uikit/table.go#L67) / [TableProps](../internal/uikit/table.go#L44) / [TableResult](../internal/uikit/table.go#L75) | 真实List.Rows虚拟化；唯一path选择、排序、resize、对齐、完整键盘/屏外Reveal、拖动源保活、公开表格语义及两轴共享滚动条 | 文件列表；关联限制见下表，已获用户接受 |
| L5.2 | SortHeader | 完整实现 | [SortHeader](../internal/uikit/sortheader.go#L25) / [Column](../internal/uikit/table.go#L21) / [SortHeaderProps](../internal/uikit/sortheader.go#L10) | sticky表头、ScrollWith、单/多排序、resize/键盘调宽、对齐及列名/排序描述；Translucent清除自身HeaderBg，复用整段页头的共享渐进玻璃背景，停用时保留实体底色；调整后方列时固定前方弹性列，边界竖线与拖动命中区居中对齐 | 本地/远程DataTable内置表头，5/3列随原布局适配；实际行Backdrop和Underlay同时启用时接通玻璃态，不被原实色底遮挡，按用户最新要求随共享玻璃层渐隐；仅非Fixed列绘制共享Divider，悬停/拖动/键盘焦点高亮 |
| L5.3 | ListRow | 完整实现 | [ListRow](../internal/uikit/listrow.go#L19) / [ListRowProps](../internal/uikit/listrow.go#L9) | Leading/Title/Meta、选中/焦点/斑马/hover、行语义；选中meta不覆盖行色 | DataTable行；通用Leading/Title/Meta变体完整提供，未把多行凭据卡强缩成30px |
| L5.4 | KeyValue | 完整实现 | [KeyValue](../internal/uikit/keyvalue.go#L15) / [KV](../internal/uikit/keyvalue.go#L10) | 多行、Mono/Dim、专用行高/颜色/字体 | 监控系统信息、进程与预览元数据；监控实际消费`/proc`原始记录、默认路由接口差分及PID＋启动时间Top，单项不可用为—，见[后端覆盖](monitoring.md) |
| L5.5 | Meter | 完整实现 | [Meter](../internal/uikit/meter.go#L10) / [MeterProps](../internal/uikit/meter.go#L8) | lo/hi、Label/Value、RoleMeter/Range；正常黑、70%警告、90%危险 | CPU/内存/磁盘 |
| L5.6 | Progress | 完整实现 | [Progress](../internal/uikit/progress.go#L29) / [ProgressProps](../internal/uikit/progress.go#L10) | Running/Paused/Done/Error、Thin、确定/不定、减少动态静态条纹 | 传输五态与设置50%预览；不定变体完整提供 |
| L5.7 | Sparkline | 完整实现 | [Sparkline](../internal/uikit/sparkline.go#L14) / [SparklineProps](../internal/uikit/sparkline.go#L8) | 数值序列、Height/Tone、专用线宽/填充/主题色 | CPU曲线 |
| L5.8 | Stat | 完整实现 | [Stat](../internal/uikit/stat.go#L8) / [MeterStat](../internal/uikit/stat.go#L22) | 标题/右侧值、图形body及说明slots | 监控分组；CPU/内存为`/proc`值，磁盘容量为独立SFTP `StatFilesystem`结果；失败保留各卡片—及原布局，见[后端覆盖](monitoring.md) |
| L5.9 | TransferRow | 完整实现 | [TransferRow](../internal/uikit/transfer.go#L174) / [Transfer](../internal/uikit/transfer.go#L159) / [TransferRowResult](../internal/uikit/transfer.go#L169) | 展示快照、图标/名称/保文件名路径/进度/状态/操作；只运行速度Accent | 具体文件标签的传输队列；离线恢复仍显示归属任务，列表/统计/批量操作/冲突与失败反馈按当前标签隔离，暂停/继续/重试/定位/取消由原后台队列执行 |
| L6.1 | OsLogo | 完整实现 | [OsLogo](../internal/uikit/domain.go#L22) / [RegisterOS](../internal/uikit/domain.go#L20) / [OSAsset](../internal/uikit/domain.go#L12) | 名称/尺寸、品牌背景、未知Linux，资源无状态色 | SSH目录与预览 |
| L6.2 | ProtocolIcon | 完整实现 | [ProtocolIcon](../internal/uikit/domain.go#L47) / [Protocol](../internal/uikit/domain.go#L39) | SSH/SFTP/FTP、尺寸与专用配色 | 文件站点目录、会话协议 |
| L6.3 | HostLabel | 完整实现 | [HostLabel](../internal/uikit/domain.go#L155) / [HostLabelProps](../internal/uikit/domain.go#L150) / [MiddleText](../internal/uikit/table.go#L278) / [MiddlePath](../internal/uikit/domain.go#L236) / [MiddleLabel](../internal/uikit/domain.go#L203) / [MiddleLabelProps](../internal/uikit/domain.go#L189) | Inline/Stacked/Dim；MiddleLabel实际测宽、Prefix预留、MiddlePath保留文件名及完整AX原文；目录profile保留行高 | 目录、宿主标签、传输路径、凭据文件与SSH_AUTH_SOCK路径 |
| L6.4 | Latency | 完整实现 | [Latency](../internal/uikit/domain.go#L299) / [LatencyProps](../internal/uikit/domain.go#L294) | 默认ms/未知—、150/400阈值、可选前缀与局部Surface | 终端页脚由`sshLatencyMS`消费独立`Connection.ProbeLatency`，同连接组共享；每5秒采样，3秒超时或10秒过期为—，不再采用监控命令耗时 |
| L6.5 | PaneBadge | 完整实现 | [PaneBadge](../internal/uikit/domain.go#L258) | 数字序号/聚焦，独立C.PaneFocus几何 | 终端⌘N窗格标识，不冒充Kbd通用样式 |
| L6.6 | FileIcon | 完整实现 | [FileIcon](../internal/uikit/domain.go#L78) / [FileKind](../internal/uikit/domain.go#L64) / [FileIconProps](../internal/uikit/domain.go#L76) | 目录、代码、图像、归档、密钥/链接映射；选中图标跟随行色 | 文件列表及预览信息 |
| L6.7 | TestResult | 完整实现 | [TestResult](../internal/uikit/feedback.go#L84) / [TestState](../internal/uikit/feedback.go#L67) / [TestResultProps](../internal/uikit/feedback.go#L74) | Testing/OK(ms)/Fail详情、公共Spinner、完成去重Announce | 会话连接测试，任务/取消仍在app |
| L6.8 | TerminalSurface | 完整实现 | [TerminalSurface](../internal/uikit/terminalsurface.go#L21) / [ResolveTerminalColors](../internal/uikit/terminalsurface.go#L14) / [TerminalChrome](../internal/uikit/terminalsurface.go#L28) | 显式每会话scheme、BackgroundSource、前景/选区；chrome独立命名 | 终端实际surface及缓存适配、设置终端预览解析，不安装全局终端主题 |

## 复核后的接入整改

此前将下面这些能力统一标为“完整实现”的结论过满：公共实现存在，部分消费者仍沿用旧壳或在返回后覆盖公共状态。本节记录当前源码的逐项修正，矩阵中的状态以公共能力加这些实际消费者共同核对，不以函数存在或编译成功代替接入证据。

| 复核项 | 当前处理与实际消费者 | 状态 |
|---|---|---|
| 小型弹窗壳 | 删除旧`Dialog`薄封装；`ModalProps.Compact`保留居中、原GroupWidth/HostWidth内容宽加双侧Padding、N14间距，内部确认显式N12。分组、站点删除、文件动作/冲突与两种标签关闭全部使用同一实现 | 已修复 |
| 小型弹窗关闭/提交 | 取消/Esc/遮罩走`RequestDismiss`；编辑有效名称通过`SetDirty`确认。点击、原名称输入Enter和公共Cmd+Enter聚合后只执行一次；内部丢弃确认同样消费Submit。Compact不新增X，普通Enter未绑定为全局危险操作 | 已修复 |
| 浮层卸载与焦点 | 关闭动画结束时明确请求下一帧，确保已构造的全屏backdrop被SDK卸载并进入焦点恢复路径 | 已修复 |
| 后置Disabled | 标题栏、分栏、新建文件夹与文件动作按钮全部在公共Props传Disabled；剩余原生Disabled仅用于容器和原生菜单项 | 已修复 |
| 强调色Selected | 删除页面第二次大小写敏感Selected，以AccentSwatchProps里的EqualFold结果同时驱动视觉和组件状态 | 已修复 |
| 危险操作 | 站点/文件删除与关闭连接用Danger；文件菜单删除项置底并独立分隔，原四项目录菜单不变 | 已修复 |
| 名称校验 | 文件名称在本帧Input处理后重算，`StyleFormControl`与`FieldMessage`统一边框/invalid/状态描述；操作失败用Banner，编辑后清理上次操作错误 | 已修复 |
| 即时及延迟通知 | 已知失败均使用类型/标题/详情；延迟`fileWorkspaceState.notice`仅存ToastProps快照，触发时机与业务动作不变，原消息去重ID保留 | 已修复 |
| 图标交互态 | TransferAction与目录选择按钮用局部Surface设置闲置样式，保留公共hover/pressed/focus；PaneToggle委托IconButton.On/IconOn，保留原几何/选中色/提示 | 已修复 |
| Mono目录输入 | 默认本地目录使用InputProps.Mono，统一字号和字距，原业务值及保存流程不变 | 已修复 |
| 普通文字/终端预览 | 监控说明和小弹窗使用Text语义预设；特殊Caption2保留尺寸覆盖。预览使用ResolveTerminalColors、保留ANSI/光标专用绘制；减少动态下光标静态，复用真实终端周期常量 | 已修复 |
| 目录空态/活动胶囊 | EmptyCatalogSearch保持原Small/Secondary/Padding12；新增ActivityBadge(新输出/响铃)并真实接入，保留局部Surface、非对称padding、Tooltip和拖动祖先 | 已修复 |
| 设置渐隐 | 设置内容底部使用ScrollFade EdgeBottom，显式传原FadeHeight及背景，不改变布局 | 已修复 |
| 目录拖动菜单清理 | SiderRow禁用路径仍调用MenuOn执行已有关闭清理，即使拖动控制器本帧移除了菜单内容，避免残留开关状态 | 已修复 |

公共补充API：`IconButtonProps.IconOn`、`PaneIconButtonProps{Shortcut,Disabled}`、`ModalProps{Compact,ContentGap}`、`EmptyCatalogSearch`、`ActivityBadge/ActivityBadgeKind`。这些是实际消费者所需的适配，不引入业务状态或资源生命周期。

终端ANSI文本、光标绘制、协议默认端口交叉淡入、搜索高亮slot、40pt会话品牌图块，以及队列/历史/拖动控制器继续保留专用实现。它们具有明确尺寸、呈现或业务契约，不以普通Text/Badge强行替换。

## 第二次全面复核：九项问题与两项收口

上一轮静态复核和Go检查仍漏掉了SDK构造顺序、副作用和焦点竞争。下表逐项记录本轮修正；原“完整实现”不能解释为经过设备交互测试，也不能以函数存在或一次编译成功替代调用链核对。

| 编号 | 原问题 | 当前实现与消费者 | 状态 |
|---|---|---|---|
| F01 | Segmented在Radio构造后Key会panic | 同一SDK raw Context在Segment构造前设置Key，删除后置rekey；认证/协议/FTP/设置/传输分类共用实现 | 已修复 |
| F02 | FormRow标签聚焦Track，未进入真实Radio | 保留FormRow标签入口；下一构建pass发现Track被显式聚焦后，把焦点转至当前选中Segment。Track不成为最终焦点目标，无新布局节点/业务状态 | 已修复 |
| F03 | 标签键盘激活后被工作区Handle.Focus覆盖 | selectTab/selectFileTab只选择并保留原attention/cursor清理，避免closeSettings恢复旧焦点；鼠标/全局快捷键仍用activate并显式聚焦既有工作区句柄；两页及SessionTabs默认路径均核对 | 已修复 |
| F04 | PaneFile后置Selected改写Card背景/前景 | 去除Selected绘制副作用，原focused边框仍由同一Focused值绘制，原卡片底色/行色及布局保留 | 已修复 |
| F05 | Search缺统一输入焦点外观 | 真实Input.FocusVisible/IsDisabled驱动原Root的InputFocus阴影；不重画Sunken边框，Filter保留原Border，不加尺寸/命中层 | 已修复 |
| F06 | PathBar普通段不返回公共选择结果 | 普通段和overflow菜单都写Chosen/Target/Index；文件页面只处理bar.Chosen；保留公开Items供调用方做其他呈现 | 已修复 |
| F07 | 系统强调色色条未圆形裁剪 | 在自身Draw内调用Painter.Clip(r,W/2)，裁剪原色条，不改控件Size/Radius/选中环；不是仅给子树加Clip | 已修复 |
| F08 | Select高亮行勾选仍灰色 | Highlighted勾选Icon使用ToneOnAccent，与行文字一致；非高亮沿用原中性色 | 已修复 |
| F09 | 导航/主题卡/配色卡/标签焦点规格并存 | SettingsNavItem、ThemeCard、SchemeCard、SessionTab关闭SDK默认环，FocusVisible时叠加ControlFocus；原selected边框/选中阴影与几何保留 | 已修复 |
| C01 | 普通文字还在手拼排版 | 文件页脚、传输汇总/说明与设置普通标签使用Text；设置高亮slot复用TextFont/ToneColor。原Settings标签13pt regular和说明11.5pt Secondary显式保留，未机械改成Body 12.5 | 已收口 |
| C02 | 凭据路径/Socket路径尾截藏文件名 | 公共MiddleLabel/MiddleLabelProps扩展既有测宽实现，复用MiddleText/MiddlePath；以LayoutBounds未变换宽度测量并与DrawOver布局宽比较，拖动缩放不影响裁文；凭据保留Mono/原字号颜色，Socket先按同字体预留SSH_AUTH_SOCK=前缀，AX名称保留完整原文 | 已收口 |

本轮不修改布局算法、Tab/目录Drag源及祖先、三页互斥切换、独立目录、SSH连接/输出/滚动、目录历史、任务队列、配置保存或密钥任务。select/activate拆分只调整UI选择后的焦点目的地，不重建、不取消、不重连会话。

## 滚动条共享显隐

可控制区域统一使用公共`ScrollbarState/ScrollbarProps/Scrollbars`，原演示终端、目录、状态卡机制收敛至同一个实现，设置、会话表单、文件预览、双轴表格、传输列表、选择器与菜单均接入。真实VT保留SDK内部滚动条；按用户最新要求只需相同风格，不再要求公共组件或参数完全相同。顶部SSH/文件会话选项卡始终隐藏滚动条，公共SessionTabs保留滚轮横移和新标签自动露出。逐页面/卡片及已记录参数微差见[滚动条清单](scrollbars.md)，不把原生VT说成已接UIKit或已做像素验收。

## 传输完成反馈

传输成功只更新既有任务行的进度、已完成状态、用时和完成时间，不再弹出完成Toast。任务行的定位按钮、双击和右键“在文件夹中显示”保留；失败Toast与重试操作保留。公共Toast的Success能力继续可用，不为保留该变体强行给页面重复提示。

## 会话端口与环境变量内容起点

修正后已有测试、Go构建、go vet、格式/差异检查及MyGo应用打包通过；未做设备UI验收。

端口仍使用同一个原生Input编辑字符串，未编辑默认端口的淡入文字覆盖层把水平起点统一为Input.PaddingX+Input.BorderWidth；固定SDK的editor.originX来自contentX（padding+border），因此切换焦点不再相差一条边框宽度。协议/TLS默认端口更新淡入、空串编辑中间态及校验保留。

ChipInput的水平padding改用同一个Input.PaddingX，垂直padding、chip尺寸、换行/删除/提交逻辑保留；空态占位、编辑文字及首个标签边缘与普通Input内容区对齐。环境变量VALUE里的逗号仍按PreserveCommas保留，不改变配置或保存流程。

## 传输任务与会话归属

业务层通过`fileSession.restoreID`、`storedFileTab.ID`（工作区v2）与`storedTransferSession.ViewID`（队列v4）保存具体标签身份；`activeTransferSession/sessionTasks`只向公共TransferDock/TransferHeader/CountTabs/TransferRow提供当前标签的数据，包括离线标签。没有标签时保留空传输栏；新建同站点标签获得新身份，不按地址或profile合并历史。每个标签保留独立的传输滚动句柄，列表根Key也跟随标签；全局并发调度、独立client及后台worker生命周期保持原实现。重新打开刚关闭的标签复用原对象；定位到其他浏览器不修改任务归属。

RestoreSessions关闭时不绑定旧任务；开启时仅按恢复标签ID绑定，恢复不自动连接/继续。缺标签身份的旧v1/v2/v3记录无法可靠区分同站点的多个标签，保留未完成检查点，不猜测归属。已完成记录仅为可恢复原标签持久化。具体恢复和旧数据边界见[文件传输](file-transfer.md)。UIKit继续不持有业务归属、网络或持久化。

标题栏对齐由公共TitleBarLeading扣除标题行自身6px gap，SSH/文件调用都传入与正文一致的rail＋（侧栏宽＋8px gutter）×显示进度；保留原生交通灯最小空间、标签滚动、拖动和加号定位。

本轮已有项目`go test -race ./...`、`go vet ./...`、`go build ./...`和格式/差异检查通过，MyGo darwin/arm64应用打包通过。未新增测试，未操作用户真实配置或服务器；跨标签交互、实际进程重启与设备UI尚未实测，不以已有测试通过冒充新增路径的运行验收。

## 传输分类垂直对齐

修正后已有测试、Go构建、go vet、格式/差异检查及MyGo应用打包通过；未做设备UI验收。

CountTabs在横向标题行显式AlignSelf(Center)，覆盖Segmented为竖向表单保留的AlignSelf(Start)。原先Start会压过TransferHeader.AlignItems(Center)，把24pt分类组贴到标题行顶部。修正只调整该行的交叉轴对齐，不改变高度、字号、计数、筛选状态、队列或任务逻辑。

## 目录更多按钮的背景状态

本次背景修正后，已有测试、Go构建、go vet、格式/差异检查及MyGo应用打包均重新通过；未运行设备UI验收。

SiderMore使用IconPlain，行悬停或键盘焦点只控制按钮的可见性，不提供常驻底色；按钮自身hover使用已有SiderItem.MoreBg（通过局部Surface传给IconButton），与整行State.Hover区分；pressed仍由IconButton统一绘制。菜单打开时MenuOn保持BgOn与可见性，关闭后恢复原交互态。删除原先错误的IconFilled选择，避免“行hover显示按钮时有底色，按钮自身hover反而换成更弱背景”的反转。SSH和文件目录共用此实现；Sider添加按钮的Filled外观保留。

## 目录菜单双入口统一

SSH与文件目录的SiderRow不再注册原生ContextMenu。RowMenuOn把行右键、更多按钮左键、更多按钮右键及Shift+F10接入同一menuState与menuPopup，四项命令、图标、禁用、间距、分隔线、危险色和高亮规则均由同一模型/绘制产生。鼠标上下文入口使用窗口Overlay中的零尺寸点锚，依靠SDK AttachTo避让窗口边缘；按钮入口保持原锚点。弹层关闭后焦点返回真实行或按钮，不落在点锚。

不新增行HandleInput，不改变左键Draggable、双击/Enter连接、目录身份或业务会话。右键使用SDK RightClicked（右键释放时触发），不再调用操作系统菜单。原生菜单适配RowMenu/NativeMenu继续服务其他既有入口，未扩大此次视觉变更范围。

## 页面消费者索引

| 页面/入口 | 实际调用位置 |
|---|---|
| 三页标题栏与外框 | [app.go](../internal/desktop/app.go)按activeSection互斥构建；设置直接使用[TitleBar](../internal/uikit/titlebar.go#L9)，SSH/文件使用各自标签栏 |
| SSH工作区与标签 | [terminal.go](../internal/desktop/terminal.go)、[toolbar.go](../internal/desktop/toolbar.go)、[pane_drag.go](../internal/desktop/pane_drag.go) |
| 两个独立目录 | [sessions.go](../internal/desktop/sessions.go)、[catalog_sidebar.go](../internal/desktop/catalog_sidebar.go) |
| 文件工作区、列表与历史 | [ftp_ui.go](../internal/desktop/ftp_ui.go)、[ftp_files.go](../internal/desktop/ftp_files.go)、[ftp_sessions.go](../internal/desktop/ftp_sessions.go) |
| 传输、文件动作及预览 | [ftp_transfers.go](../internal/desktop/ftp_transfers.go)、[ftp_dialogs.go](../internal/desktop/ftp_dialogs.go) |
| 会话表单 | [session_modal.go](../internal/desktop/session_modal.go)、[session_modal_advanced.go](../internal/desktop/session_modal_advanced.go) |
| 设置与凭据 | [settings.go](../internal/desktop/settings.go)、[settings_themes.go](../internal/desktop/settings_themes.go)、[settings_shortcuts.go](../internal/desktop/settings_shortcuts.go)、[credentials_ui.go](../internal/desktop/credentials_ui.go) |
| 浮层与通知 | [settings_licenses.go](../internal/desktop/settings_licenses.go)、[app.go](../internal/desktop/app.go)、[settings_config.go](../internal/desktop/settings_config.go) |

## SSH组合标签与Shell拆离

组合标签及其拖动预览使用原四宫格（IconGrid、12px）替代状态点，保留附加Count与焦点Shell的标题/提示；单会话和文件标签仍显示原连接状态。组合右键菜单使用“拆分会话”替代“编辑会话…”，其他连接操作仍针对当前焦点Shell，关闭仍关闭整组。

`model.go`的`canSeparateSessions / ungroupTab / detachPaneToTab`只移动同一个`*remoteSession`。批量拆分按原窗格顺序相邻插入，活动组保留原焦点会话，后台组保留当前标签/焦点；拖出单个Shell则修复源组焦点及缩放、保留源显示比例，并激活新标签。关闭动画中的组合暂不可拆，防止把正在关闭的窗格拆成单标签后破坏原关闭流程；过期native payload在提交边界拒绝。两条路径均不调用newSession/openHost/connectSession/closeTab，不增加nextSession，不重置输出、解析状态、配置或滚动句柄，不写关闭历史。

公共[SessionTabWidth](../internal/uikit/tabs.go#L108)、[SessionTabPreview](../internal/uikit/tabs.go#L123)与真实标签共用内容和测量规则；[DragMorphGhost](../internal/uikit/drag.go#L43)、[DragMorphGhostProps](../internal/uikit/drag.go#L26)、[DragMorphSettled](../internal/uikit/drag.go#L85)接入`pane_drag.go`。演示/预览浮卡沿用transcript绘制；真实VT浮卡在pointerdown捕获可见源已绘制正文，后台裁切并按原morph缩放，保留真实历史视口；不可见或捕获失败使用独立真实emulator Snapshot。备用Snapshot不含SDK内部历史视口位置，不承诺历史中部逐像素同位。所有节点PassThrough，单手势资源清理；备用emulator只有被动本地Conn，不开Shell、不建网络连接、不改变源PTY。

进入标签栏只让手中的面板形态收拢为完整标签，浮卡持续按pointer/grab定位；周围真实标签和加号按原TabDisplace弹簧视觉让位，不插入空占位节点、不提前改模型。离开标签栏时反向变形，邻居及滚动范围平滑回收；松开后才通过原pendingDock提交，浮卡继续移动至真实新标签，并在形态/位置/内容交叉淡化均Settled后交接显示。真实插入接替让位宽度时清空临时space，避免尾部重复计算。快速native drop先构建一次真实面板形态；目标标签首个有效布局前保留计算的落点。减少动态时几何即时、透明度仍按原政策过渡。

固定SDK会在paintDrag中忽略拖动根的Opacity。`terminal.go`保留原身份栏Drag源/Key，仅让其内容子Row在Dragging时Opacity(0)，阻止SDK副本重复绘制悬浮身份文字；自定义预览保留。内容Row沿用原grow/min-width/gap规则，终端output/scroll祖先及会话句柄不变。原标签横向重排、向Shell停靠、交换、Esc取消、文件页及后台传输路径继续保留。

## 已确认决定与明确差异

| 能力 | 原稿/现状冲突及位置 | 本轮决定与实际处理 | 分类 |
|---|---|---|---|
| 会话未保存退出 | prompt Modal 的Dirty确认与原会话直接关闭不同；`session_modal.go`、`modal.go` | 用户确认现有会话也启用；X/取消/Esc/遮罩和密钥跳转统一请求确认，保存成功不额外确认 | 按原规格完成 |
| Tab减少动态 | prompt §2.7写继续旋转，总览图写静止3/4环；`spinner.go`、`tabs.go` | 用户确认静态指示；应用三态解析后传入，阶段推进不停止 | 继续旋转明确不适用 |
| 目录拖动外观 | prompt要求1.02/−0.6°；现有控制器1.03/无旋转；`drag.go`、`catalog_sidebar.go` | 用户确认目录保持1.03/无旋转；公共标准lift具备旋转，减少动态时禁用缩放/旋转 | 强行改变既有目录外观明确不适用 |
| 窗格连接提示 | prompt §12.5待确认；图中同时存在页头点/页脚胶囊；`terminal.go` | 用户确认只留页脚胶囊，实际页头点已移除；同一session.state，Sider不显示状态 | 维持页头点明确不适用 |
| 原生Grid关联 | SDK `ui/access.go` 无公开Grid/RowHeader及平台行列关联元数据；`table.go`、`sortheader.go` | 用户接受RoleTable/Row/Cell/ColumnHeader、列名/值/排序/选择描述适配。没有假称原生Grid关联或fork SDK | 原生Grid关联明确不适用；公开语义适配完成 |
| 端口和环境变量 | prompt的int端口/逗号分隔会破坏已确认输入契约；`input.go`、`chipinput.go` | Port保留字符串中间态；通用Chip仍支持逗号，ENV明确PreserveCommas | 破坏原输入契约的示例明确不适用 |
| Tooltip和Kbd | 原文同时要求Element.Tooltip纯字符串与Kbd内嵌，固定SDK字符串接口不能同时满足；`tooltip.go` | 使用SDK TooltipBase语义/计时/避让/焦点宿主，内容为Text+Kbd，不以Painter自绘浮层或自建计时器 | 纯字符串签名按公开API适配 |
| 全局Size零值 | 原稿同时要求Md=0和Badge Sm=0；`types.go` | SizeDefault=0按各组件默认，另有Xs/Sm/Md/Lg显式值；不靠同一零值冒充两种尺寸 | 签名/枚举适配 |
| 目录右键与更多外观 | 原要求右键原生、更多Popover，形成两套视觉；`menu.go`、`sider.go` | 2026-11-02用户明确调整为两个入口都使用图1公共菜单。RowMenuOn复用一个开关和menuPopup：右键鼠标位置，更多按钮锚点；其他既有原生菜单保留 | 最新明确约定覆盖原目录右键要求 |
| 已确认快捷键 | 原稿Cmd+T/历史Cmd+[示例与当前应用绑定不同；`app_actions.go`、`settings_shortcuts.go`、`filepane.go` | 保留Cmd+N新建、Modal Cmd+T测试等实际配置与⌘←/→历史；新增Ctrl+Tab/Shift循环和局部箭头/Home/End/中键，不改已有binding | 示例键位替换实际binding明确不适用 |
| 快捷键表列操作与滚动 | 原SDK Table支持列调宽/重排并包含内部滚动 | 按用户最新要求移除双击适宽、手动调宽、列重排及内层滚动；[ShortcutTable](../internal/uikit/settings.go#L13)固定列比例/顺序，按内容撑高，消费者为[shortcutSettings](../internal/desktop/settings_shortcuts.go#L45)；保留表格/行/单元格/列标题角色、稳定Handle/动作Key和录制/恢复默认 | 快捷键表列操作与独立滚动明确不适用；文件DataTable的通用能力保留 |
| 会话标签滚动条 | 此前横向标签接入共享显隐；`tabs.go`、`titlebar.go` | 用户最新要求两页始终隐藏，保留共享滚轮横移及新建/选中时一次ScrollIntoView；其他区域仍使用共享显隐 | 会话标签显示滚动条明确不适用 |
| 三页标题栏隔离 | 此前正文按activeSection选择，标题栏仍按上一个工作区section选择；`app.go` | 标题栏key与分支也改用activeSection；设置使用独立同高窗口拖动栏，不构建上一页的SessionTabs/加号/工作区工具。文件标签ScrollState/Handle提升至应用持有 | 按三页平级互斥要求补齐标题栏，保留会话和后台任务模型 |
| 目录条目active | 原设计排除Sider会话选中/连接状态；`sider.go` | 用户新增独立点击聚焦反馈：原生Focused驱动SiderItem.BgActive，单击显示、失焦消失，双击/Enter仍交给原工作区激活；拖动/落点反馈优先 | 按最新要求增加临时焦点外观；会话选中/连接状态仍不适用 |

## 状态与布局所有权

- `desktop`继续持有独立SSH/文件目录、三页激活、连接对象、输出/滚动、目录历史、传输队列、配置写入及独立密钥任务；UIKit不做网络、文件、持久化或钥匙串操作。
- Settings关闭的临时状态清理和稳定Handle仍由原应用流程负责。组件新增状态只包含reveal、菜单开关、焦点/鼠标、确认、测量和控件编辑等UI状态。
- 虚拟列表使用真实公开 `ui.List(...).Rows(func(ui.ListRow))`。TableState持有唯一path选择/cursor/anchor，不维护旧name map镜像；内层Drag先接收鼠标，因此不谎称原生List selector负责了该选择。源行获得焦点后由SDK额外保活，结束恢复list焦点；Reveal在构建前按路径定位索引。
- 表头和内容使用同一列宽模型；实际body viewport从HeaderHeight到FooterHeight，避免屏外定位被sticky头覆盖。Rows→真实条目Backdrop→Underlay→Header→外层toolbar/footer的绘制顺序保留玻璃效果且不遮表头。文件页工具栏和列头共用覆盖完整HeaderHeight的ScrollFade多层渐进背景，与SSH上下栏、文件页脚统一1/3/5px和92%→78%→0着色；列头在半透明有效时自身完全透明，避免实色或重复叠加独立列头底色造成白色条带。Material与着色均先于工具栏、列名和排序/拖动控件绘制，ScrollWith只横移内容。TableBackdropRow只供显示数据，PaintFileIcon与FileIcon共用图标/颜色，单元格共用字体、宽度、对齐和中间省略；装饰层按paint时的实际滚动偏移绘制，不创建第二组交互/读屏行。
- macOS窗口创建启用固定MyGo公开的VibrancyUnderWindow（NSVisualEffectView后方混合），syncWindowMaterial只在设置/系统降低透明度改变时切换原生材质；开关有效且c.Vibrancy可用时，Root统一叠加Window.Bg及Window.TintOpacity（18%）着色，三页header与content不再各自铺底，导航和卡片间隙共同透出同一层全局毛玻璃；停用透明时由Root恢复统一实体底色。卡片、终端背景及其栏内渐进模糊保留，其他平台沿用标题栏材质和不透明正文。不改变窗口、会话或后台传输的生命周期。终端正文的水平边距及与上下栏的间距统一使用C.Pane.ContentSide（8px），ViewportBackdrop只读同一RenderState的overscan，不移动真实视口；背景层在自身绘制空间计算网格原点，不再复用另一离屏纹理内的View坐标，上下栏分别以自身外侧圆角裁剪子绘制，避免GPU内层矩形裁剪抹掉卡片圆角。
- 默认名称列最小宽160，现有窄文件窗格显式传原布局约束；Header公共36/40/46齐全，现有Sider仍显式保留原外框高度。布局参数覆盖不删除公共规格。
- 原DataTable应用框架、左右文件窗格、分屏比例和拖动控制器继续复用。用户批准移除页头状态点会让标题前移；其他规范化外观包括内描边、状态色/阈值、数字对齐、Kbd和中间省略，不声称像素无差异。

## 验证结果与边界

2026-11-02半透明回归修复后，项目及本地终端插件的已有全套`go test -race ./...`、`go vet ./...`、`go build ./...`通过；最终缓存刷新调整后，另复跑已有绘制、滚动、滚动条拖动、选择/自动滚动、外观、占位图及初始尺寸相关用例通过。MyGo darwin/arm64打包签名通过，主MyGo/原生Ghostty版本及资源manifest未改。新增API为`ViewportBackdrop`、`TableProps.Backdrop`/`TableBackdropRow`和`PaintFileIcon`，实际消费者及状态所有权见上表；原始终端View和List滚动边界均保留，不借扩大交互视口恢复效果。没有新增测试、组件画廊、截图基线或CI，未进行GUI视觉验收、系统设置修改或真实服务器重连。没有内容经过栏下时，玻璃层显示原背景，不制造历史或文件条目。

2026-11-02截图反馈后的渲染修正：上述检查没有覆盖到实际GPU下的标题栏颜色、嵌套圆角裁剪和非焦点离屏坐标，不能证明上轮视觉正确。本次新增标题栏着色语义令牌及组件映射Titlebar.TintOpacity，重新生成theme.go/tokens.json/tokens.md；保留原生材质，在三页各自标题栏叠加92%主题窗口底色。ViewportBackdropOptions增加Radius，顶部/底部各自用外侧圆角的容器裁剪子绘制，移除会覆盖卡片圆角的矩形Painter.Clip；在自身子树的绘制矩形中计算网格原点，移除跨离屏纹理复用的painted坐标。原终端View的父级、位置、尺寸、4px留白、滚动和PTY尺寸控制未改。项目及终端插件已有全套race测试、go vet和Go构建通过；未新增测试或执行设备GUI验收，实际渲染仍待复测。

2026-11-02文件列头玻璃态收口：上版单独给列头增加模糊及着色，与工具栏下的渐隐着色叠加，未解决截图中的实色观感；先前已有检查不作为视觉通过证据。最终SortHeader.Translucent只清除自身背景，工具栏和列头共同使用完整HeaderHeight范围的ScrollFadeProps.Uniform（已有Glass.Bar.Blur、FadeBar.TintMid令牌），整段页头只保留一层模糊和一次着色。当时关闭半透明或系统降低透明度后回到实体底色；随后两页上下栏曾统一为Uniform；用户最新要求已恢复为1/3/5px多层渐进模糊，着色向内容区逐渐透明。本地/远程、3/5列共用；列头尺寸、ScrollWith、排序/列宽处理、虚拟行及业务句柄保留。本轮项目已有race测试、go vet和Go构建、格式及差异检查通过；设备UI仍待复测，不以静态检查替代实际显示验收。

2026-11-02组合标签、会话拆离及形态/排序动画完成后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改Go文件的`gofmt`与`git diff --check`均通过；依赖和已有测试文件未改。按固定MyGo v0.7.3公开接口及源码复核了pointer-held预览、真实会话指针复用、快速drop、pendingDock提交顺序、后台/活动组焦点、关闭动画保护、原生副本抑制、底部滚动哨兵、让位范围回收和真实标签交接。两次独立静态复核发现的坐标夹取/布局宽度/哨兵问题已按最终手持语义修正；没有把编译或静态复核当作视觉验收。未新增测试、组件画廊、截图基线或CI；未启动/重连真实服务器、未进行设备UI验收。

2026-11-02标题栏与侧栏对齐修复后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改Go文件的`gofmt`及`git diff --check`通过。文件标题栏增加与Workspace相同的Window.Gutter右留白（8px），位于滚动视口之外；设置标题栏与正文均按activeSection独立构建，窗口拖动栏高度保留；设置侧栏真实接入SiderHeader.HideAdd，与SSH/文件共用标题高度、字体、padding和居中规则，搜索继续使用同一SearchSunken。文件标签滚动位置/Handle由应用保留，连接、传输、保存和密钥任务的更新路径未改。未新增测试、重新打包或执行设备UI验收。

2026-11-02新标签尾部定位补齐后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改文件的`gofmt`及`git diff --check`通过。默认items路径与SSH/文件BuildTabs均返回实际加号按钮，在标签数量增加时，于选中标签之后请求加号ScrollIntoView；由SDK按完整新布局定位，不以旧MaxX估算尾部。首次构建仅初始化数量，保留恢复工作区的活动标签定位；关闭、重排与手动滚动不强制回尾。不改节点父级、菜单动作、SSH会话或后台传输；未新增测试，设备UI尚未实测。

2026-11-02分组拖动松开外观：公共[DragGroupLabel](../internal/uikit/drag.go#L173)与[DragGroupLabelProps](../internal/uikit/drag.go#L169)接收Dragging/Expanded，catalogDragPreview复用原catalogDragging阶段与catalogExpanded的真实显示状态。进入settling/canceling即恢复灰色标题、普通字重、真实展开箭头、CountQuiet计数及GroupHeader的padding/gap，并立即撤去拖动叠卡、卡片背景、缩放和阴影；只保留原回位位置弹簧。源标题仍由同一个ghost覆盖至回位完成，ghost不再保持“折叠＋台数叠卡”外观。SSH/文件侧栏共用；折叠配置、排序提交、取消、源身份及会话/后台传输控制器保持原样。已有`go test ./...`、`go build ./...`、`go vet ./...`、修改文件的`gofmt`及`git diff --check`通过；未新增测试，设备UI尚未复测。

2026-11-02侧栏目录焦点反馈接入后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改Go文件的`gofmt`及`git diff --check`均通过。SiderRow直接读取原生Focused控制active底色，不新增主机选择/持久化状态，不读取当前SSH/文件标签；原双击/Enter打开及工作区Handle.Focus流程保留。BgActive从tokens_src.py生成，跟随浅/深色与强调色；旧会话选中/连接状态token继续弃用。未新增测试或执行设备UI验收。

2026-11-02SSH/文件选项卡同步后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改文件的`gofmt`及`git diff --check`均通过。按固定SDK调用链复核新标签ScrollIntoView在布局后解析新内容范围；纵向wheel移至公共SessionTabs，原生横向wheel、中键、原SSH拖动按下记录/边缘自动滚动与源层级保留。文件站点打开入口去掉同站点去重，继续由openNewFileHost创建唯一会话与独立窗格；原会话、输出/目录历史、任务引用和连接更新未重置。未新增测试、重新打包或执行设备UI验收。

2026-11-02文件列宽修复后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改文件的`gofmt`及`git diff --check`均通过。代码修改限于共享SortHeader：避免前方弹性列抵消拖动增量，限制到最小宽度时不额外固定其他列，使用共享Divider标记实际命中区。复核本地/远程3列与5列共用路径；未改虚拟行身份、选择、文件拖动、会话或后台传输控制器，未新增测试或执行设备UI验收。

2026-11-02快捷键表取消列操作与内层滚动后，已有`go test ./...`、`go build ./...`、`go vet ./...`、修改文件的`gofmt`及`git diff --check`均通过。仅修改快捷键表呈现、对应临时UI状态与文档；录制/冲突/恢复默认回调、实际绑定、设置外层ScrollState及会话/传输控制器保持不变。本次未新增测试、重新打包应用或执行设备UI验收；下列应用打包记录属于此前验证。

2026-11-02共享滚动条调整后，重新运行已有测试、Go构建、go vet、格式/差异检查均通过；未新增测试。检索全部应用Scroll/List/ScrollHorizontal路径并逐项核对共享接入，原生系统选择器/菜单单独列明。横向标签输入、中键、拖动及虚拟列表继续复用原控制器和稳定句柄。快捷键表按后续明确要求取消列操作和内层滚动，使用设置页外层共享滚动条。设备UI/像素和真实任务运行仍未实测。

2026-11-02目录菜单双入口调整后，重新执行已有`go test ./...`、`go build ./...`、`go vet ./...`与格式/差异检查均通过；同一renderer与开关、鼠标点锚避让、Esc/外点焦点、右键释放及左键Drag隔离按固定SDK调用链交叉复核。没有把截图中的外观一致性冒称设备实测。当前源码重新记录132项runtime hash，不沿用上一轮源码的manifest证明这次调整。

最终代码冻结后，`go test ./...`、`go build ./...`、`go vet ./...`、修改Go文件的`gofmt`与`git diff --check`均通过。`go tool mygo build -skip-dmg`已完成darwin/arm64编译和本地签名，产物为`build/darwin-arm64/SSH Desk.app`。

`mise exec`因试写全局trusted-configs目录被沙箱拒绝；没有修改该目录，改用已安装的Go 1.27.3及进程内构建环境完成同一MyGo v0.7.3打包。依赖版本、已有测试文件不变。

本轮源码复核确认了单项禁用的键盘选择、构造前稳定Key、真实逗号输入/最终chip校验、Popup输入路由、中键与左键拖动隔离、虚拟选择与焦点保活、sticky/underlay层级、空态不随横滚消失及元数据预览等实际路径；这些是源码/SDK调用链证据，不冒称已运行交互测试。本次整改后重新冻结132个运行时代码/资源/模块文件并记录hash；最终文档收尾已核对该manifest全部未改变，未用上轮构建证明本轮代码。

历史14组隔离对照属于先前三批迁移，不能拿来证明本轮新菜单、虚拟化和完整变体已经通过运行时交互检查。

本轮不新增永久测试、组件画廊、截图基线或CI；不升级依赖、不合并、不推送。未进行设备UI/像素验收、真实SSH/FTP、真实密钥操作及原生VoiceOver行列关联验证。


## 真实后端接入后的具体差异（2026-11-02）

| 能力 | 当前状态 | 公共API/消费者与影响 |
|---|---|---|
| TerminalSurface配色与容器 | 完整实现 | `TerminalSurface/ResolveTerminalColors/Pane/Header/Footer`仍接终端卡片；`sshColors(session)`给原生VT显式每会话颜色，设置预览继续原渲染 |
| 真实终端字距 | 已移除遗留参数 | 配置字段、校验及旧文本渲染调用已删除；旧 JSON 字段忽略，不以缩放冒充支持 |
| 真实终端BoldIsBright开关 | 明确不适用 | 用户要求删除设置入口；字体分组、搜索索引及翻译文案已移除；旧配置与旧预览读取路径保留，真实终端使用SDK默认行为 |
| 真实终端ScrollbackLines | 完整实现（接受页级误差） | [本地终端包](../third_party/mygo-terminal/terminal.go)公开`Options.ScrollbackLines`及`Terminal.SetScrollbackLines(int) error`；[attachSSH](../internal/desktop/ssh_terminal.go)与独立拖动快照创建直接传行数，[enforceScrollback](../internal/desktop/workspace_store.go)原地同步全部已有SSH标签，包含非当前页/标签；同一terminal、连接与传输任务保留。物理显示行按页淘汰，缩小可丢弃最旧历史，增大不恢复旧输出 |
| 真实终端共享Scrollbar | 明确不适用 | 用户改为只需风格一致；保留原生半透明胶囊、滚动/贴边显示与淡出，其他区域继续公共Scrollbars。宽度/时长存在已记录微差，不声称同组件或像素一致 |
| 拖动真实屏幕 | 完整接入、备用路径有限制 | `Window.CapturePage`+单手势bitmap保留可见源视口；不可见来源用独立真实Snapshot，未覆盖历史视口位置能力 |

本轮已有协议/race/构建验证和未执行GUI/像素验收分别记入[MVP状态](mvp-status.md)。上述差异没有按“当前没有调用”豁免，没有用函数数量或构建成功代替完成证据。
