# 共享 UI 组件

[internal/uikit](../internal/uikit/) 封装原生 MyGo 组件的外观、语义和 UI 状态，供桌面页面组合使用。业务工作由应用持有，不因组件挂载或卸载而创建、销毁。

## 组件入口

| 类别 | 主要文件 |
|---|---|
| 文字、图标、按钮、状态 | `text.go`、`icon.go`、`button.go`、`badge.go`、`spinner.go` |
| 输入、选择、表单 | `input.go`、`select.go`、`multiselect.go`、`form.go`、`toggle.go`、`chipinput.go` |
| 弹窗、菜单与反馈 | `modal.go`、`popover.go`、`menu.go`、`tooltip.go`、`toast.go` |
| 导航与工作区 | `rail.go`、`sider.go`、`tabs.go`、`pane.go`、`sidepanel.go` |
| 文件与表格 | `table.go`、`sortheader.go`、`filepane.go`、`pathbar.go` |
| 终端、滚动与监控 | `terminalsurface.go`、`scrollbar.go`、`scrollfade.go`、`meter.go`、`sparkline.go` |

具体参数以对应文件中的 Props、状态类型和公开函数为准。设计令牌来源及生成方式见[主题与设计令牌](theme-integration.md)。

## 状态与输入

组件保存焦点、悬停、弹层开关、选择展示、显隐时间和测量等 UI 状态；应用保留配置、文件、网络、钥匙串和任务对象。

同一对象使用稳定 Key 和窗口生命周期内的稳定 Handle。排序和跨组移动应保持业务身份，取消原生拖放时清理实际手势状态，不能把动画恢复位移当作新拖动。

标题栏使用 SDK 的 `TitleBar` 左右预留量避让系统窗口按钮。`ShortcutDisplay` 统一渲染平台快捷键提示，动作继续使用 `ui.Cmd`（macOS Command，其他平台 Control），不按显示字符串判断按键。

弹窗、菜单和 Tooltip 组合 SDK 的原生语义与定位能力；快捷键提示与实际事件绑定分开。关闭设置清理临时状态，同时保留需要复用的句柄。

`Modal.Disabled` 禁用整个弹窗；只需阻止关闭且保留内容交互时使用 `DismissDisabled`，例如一次性密钥展示期间仍须允许复制和勾选保存确认。

`Select`、`MultiSelect` 和窗口内菜单统一在选项行末显示选中对勾。`MultiSelect` 从给定选项中多选，已选项在输入框内以可换行标签展示，标签尾部的关闭按钮移除该项；选项弹层允许连续选择，通过点击外部或 Esc 关闭。

下拉与普通菜单选项的悬停、键盘焦点高亮使用中性底色和正文文字色，选中状态通过行尾对勾表示；危险菜单项沿用危险色反馈。

`Count` 的 `Max` 和 `Segmented` 的 `CountMax` 可限制计数展示；MCP 主标签页使用 99 上限，零值显示为 `0`，超出显示 `99+`，其他未配置上限的计数保留完整数字。

## 表格与滚动

文件 DataTable 基于真实 `ui.List(...).Rows(...)` 虚拟化，路径是选择、定位和源行保活的稳定身份。应用负责目录内容、排序与文件动作，共享 UI 负责显示和输入适配。

`TableViewport` 为 MCP 等自定义表格提供最小内容宽度与双向滚动。表头与数据内容共用视口宽度和最小宽度计算出的明确宽度，不在双向滚动容器内依赖百分比宽度，避免行内容变短后带动总宽度变化。筛选栏和表头保持在数据行上方，通过 `ScrollWith` 与数据行同步横移；分页栏放在视口外。表头与行内容使用相同的总内边距，避免展开详情压缩表格时列宽坍缩或错位。

`Scrollbars` 装饰既有 Scroll/List，不替换滚动节点或业务状态。滚动条位于非滚动 frame，随滚动或鼠标贴边显示；内容未溢出时不绘制。

`CodeBlock` 为完整文本提供有界的只读、选择和复制能力，每块内容只展示一组标题与复制按钮；调用方不重复添加同名标题。内部文本框通过 `TrackScroll` 接入共享 `Scrollbars`，隐藏 SDK 默认滚动条，沿用全局的宽度、颜色和显隐行为。

会话标签栏保留滚轮横移、选择后定位和新增标签尾部加号定位，滚动条始终隐藏。快捷键表随设置页自然滚动，不创建独立内层表格滚动。

真实 SSH 文字网格使用原生终端自身的滚动条；其他应用区域使用 UIKit 滚动条。两者的参数差异见[终端说明](terminal-themes.md)。

## 终端容器与栏下背景

`TerminalSurface` 接收会话配色及背景来源，`TerminalChrome` 适配周边 UI 对比度，`Pane` 承担容器与焦点展示；连接与终端对象继续由应用持有。

`ScrollFade` 提供栏内渐进模糊/着色，终端的 `ViewportBackdrop` 和文件表格的 `TableBackdrop` 提供实际相邻内容。显示用背景不拥有第二组交互或读屏行，不通过扩大真实输入视口改变业务网格。

MyGo 固定版本的静止 GPU 路径只保留最内层裁剪的圆角形状。终端补画先按网格边界限定范围，再在最内层应用完整卡片圆角；避免后加矩形裁剪覆盖圆角。

完整早期组件覆盖表、逐项整改和旧参数保存在[UIKit 历史覆盖清单](archive/uikit-coverage.md)。其中行号和阶段验收属于原记录，不能代替当前源码或原生视觉复测。
