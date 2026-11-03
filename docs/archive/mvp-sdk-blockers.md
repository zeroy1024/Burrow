> 历史归档：2026-11-03 整理，来源为 `e98cf61` 的文档快照。
> 文中的阶段计划、决定和验证结果仅代表记录当时的状态；当前说明见[文档索引](../README.md)，归档规则见[历史文档](README.md)。

# 终端 SDK 差异与当前验收范围

以当前源码、项目固定MyGo v0.7.3及最小本地终端包扩展为准。此前四项差异曾阻塞完整goal；2026-11-02用户明确允许忽略字距差异、删除粗体提亮开关、滚动条只需保持风格一致，并授权回滚按行实时生效、接受页级误差。本地扩展、应用接入与受控验证均已完成；原四项不再阻塞用户调整后的验收范围。

本表区分当前接受范围与尚未实现的能力，不把公开接口存在或编译成功当作完整实现证据。

| 能力 | 当前消费者 | 实际API与影响 | 当前验收状态 |
|---|---|---|---|
| 终端字距 | 配置结构、校验及旧文本渲染调用均已移除 | SDK未公开网格字距；旧 JSON 字段按未知字段忽略，新保存配置不再包含字距。未用缩放字形替代 | 已按用户要求移除遗留参数 |
| BoldIsBright / 粗体提亮开关 | `settings.go`字体分组及搜索索引已移除入口，`localize.go`移除对应文案 | 真实终端继续SDK默认粗体行为；旧配置字段及旧预览读取路径保留，避免改写既有配置契约 | 明确不适用：用户要求删除开关，不再扩展真实终端映射能力 |
| 实时ScrollbackLines | `ssh_terminal.go:attachSSH`、`terminal_preview.go:nativeTerminalDragContents`创建时直接传行数；`workspace_store.go:enforceScrollback`原地同步全部已有SSH标签 | `third_party/mygo-terminal`公开`Options.ScrollbackLines`、`Terminal.SetScrollbackLines(int) error`，真实绑定内核行数选项；物理显示行计数、按页淘汰，非严格逐行 | 完整实现：按用户接受的页级误差合同，修改设置同步后台标签，保留同一终端与连接，不操作后台传输任务 |
| 真实终端滚动条风格 | `ssh_terminal.go:sshTerminalView`使用原生VT滚动条，其他区域使用`uikit.Scrollbars` | 两者均为半透明胶囊、无轨道底色、滚动或贴边显示并淡出；原生4/8px、停留1000ms/淡出300ms，UIKit6px、700ms/180ms | 当前风格范围已有实现；用户不再要求同一组件或参数完全相同，未做像素验收 |

## 已接入的回滚扩展与边界

`go.mod`以`replace github.com/egoist/mygo/sshdesk-terminal => ./third_party/mygo-terminal`接入固定终端包的最小本地派生；主MyGo仍为v0.7.3，同一Ghostty构建来源不变。终端包保留许可证和来源说明，公开创建参数及带锁、失效重绘的运行时setter。正行数且未显式设置字节上限时，不再额外受上游默认10MiB字节上限提前截断；显式字节上限仍按公开契约与行数限制共同生效。

固定SDK在`plugins/terminal/internal/libbuild/main.go`锁定Ghostty提交`befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028`。该版本[公开头文件](https://github.com/ghostty-org/ghostty/blob/befcdfd2c3a1cb24d9ec886e93c95b2b5daa7028/include/ghostty/vt/terminal.h#L1914)已有`GHOSTTY_TERMINAL_OPT_SCROLLBACK_MAX_LINES = 28`；减少上限会立即淘汰可移除的完整历史页。它限制物理显示行（自动换行分别计数），实际保留行数存在页级误差，可能略多或略少，取决于页面内容与裁剪边界。此前未区分“MyGo未公开”与“底层已支持按行估计”，在此更正。

设置变化由全局`enforceScrollback`在下一帧同步所有已有SSH标签，包含不显示的标签及文件/设置页面下的SSH对象；不只依赖当前可见终端的渲染同步。创建终端与独立拖动快照直接使用行数参数，保留旧`session.output`路径的按行裁剪；没有清空并重放输出、重建终端或重新建立连接。

缩小上限会淘汰最旧的合格完整历史页；当前屏幕及未被淘汰的历史、选择和滚动锚点由同一内核维护。增大上限允许后续保留更多历史，不能恢复已被淘汰的数据。严格逐行不超过设定值不属于用户接受的本轮范围，也未通过近似字形缩放、项目层绕过私有接口或空包装制造完成。

扩展前曾在独立临时目录核对公开MyGo v0.7.4，其上游API缺项不再用于否认当前本地扩展。项目未升级主MyGo或Ghostty；原生滚动条仍按用户接受的风格范围保留，不假称已接UIKit。

## 已完成部分与验证

连接、文件与队列、正常退出恢复、Linux监控、密码版本绑定、旧加密PEM迁移及其受控证据见 [MVP状态](mvp-status.md)。已有race/vet/build、原生打包签名、loopback协议、真实hidden原生控制器验证分别记录，不把它们当作上述显示能力或原GUI像素一致性的证明。

本次回滚扩展重新通过项目和本地插件的已有测试、race、vet、构建及原生打包签名。另有5组真实内核临时验证与应用全标签同步验证，具体结果及证据见[MVP状态的本轮记录](mvp-status.md#本轮实时回滚验证2026-11-02)。临时验证源码已回收，未新增永久测试或进行GUI视觉验收。
