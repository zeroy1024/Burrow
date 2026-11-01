# 剩余 SDK 阻塞审计

以 `35939c2` 和项目固定的 MyGo v0.7.3 为准。这些问题在连续三轮 goal 审计中仍存在；前两轮已继续完成可独立实施的连接、传输、迁移与验证，没有将下列能力自行豁免。

本表是未完成项与解除条件，不是已实现的 API 清单。核心后端已提交、工作区干净，不能据此把“原型全部外观和现有设置效果保持不变”的完整目标标为完成。

| 能力 | 当前消费者 | 固定 SDK 证据与影响 | 最小解除条件 |
|---|---|---|---|
| Tracking / 字距 | `internal/desktop/ssh_terminal.go:sshFont`；`terminal_config.go` 保留原设置 | SDK `plugins/terminal/font.go:Font`只有Family/Size/Weight/Features/LineHeight/Thicken。View内部按原生网格计算和绘制，外围Element的字距不控制网格。原型默认−0.1px仍有实际差异；缩放字形不能视作字距实现 | SDK公开且真实实现cell advance/字距控制，并同步布局、命中、选择、光标与PTY列数 |
| BoldIsBright / 粗体提亮 | `settings.go`已有开关；`ssh_terminal.go:sshColors` | SDK支持字体粗体和调色板，但没有按“粗体属性＋ANSI 0–7”选择亮色的公开控制。整体改Palette会连非粗体一起改变；输入流改写也不能保持历史与程序状态完全一致 | SDK公开按cell样式进行亮色映射的选项，保持现有终端状态，并允许偏好即时改变 |
| 精确且实时的ScrollbackLines | `ssh_terminal.go`创建终端时按行数和列数估算字节预算 | SDK `Options.Scrollback`明确为字节预算，只在创建时传入；公开方法有ClearScrollback、SetScrollMultiplier，没有精确行数上限或live setter。清空全部、重建终端或反复重放输出不能冒充保留输出/滚动状态的原合同 | SDK公开精确行数容量及live更新，能够保留未被有意截断的历史、当前屏幕、选择和滚动锚点 |
| 真实终端滚动条接入UIKit | `ssh_terminal.go:sshTerminalView`调用SDK View；`uikit/scrollbar.go:Scrollbars`需要公开ScrollState | SDK View内部持有滚动视口、拖动和thumb状态；没有公开offset/range/getter/setter或接管入口。外围遮盖、绘制假thumb会破坏滚动命中或状态同步 | SDK公开滚动状态/操作或可委托的滚动条接口，让UIKit使用真实范围、位置与输入契约 |

## 可选处理方式

推荐保留目前的终端与会话架构，在 SDK 获得上述真实公开能力后接入；可以是上游版本或明确允许维护的相关依赖扩展。届时还需运行已有检查和相关受控验证，不能只加同名字段或包装。

维持当前 SDK 并接受现状，也只能把这些能力继续记为部分实现；需要明确调整验收范围后，才能据新范围判断 goal。当前没有这样的调整授权。

更换 VT/自建渲染器会重新承担文字、输入法、选择、滚动、键盘协议及资源生命周期，超出“保持现有 UI、固定 SDK 公开能力”的约束；不作为本轮可直接替代的完成结果。未使用私有接口、unsafe、近似字形缩放或空包装制造完成。

公开 MyGo v0.7.4 的字体/终端源码也已在独立临时依赖目录核对：仍没有前三项所需公开能力。其滚动条主题改进不能等同于把原生VT滚动状态交给UIKit；项目没有升级SDK。

## 已完成部分与验证

连接、文件与队列、正常退出恢复、Linux监控、密码版本绑定、旧加密PEM迁移及其受控证据见 [MVP状态](mvp-status.md)。已有race/vet/build、原生打包签名、loopback协议、真实hidden原生控制器验证分别记录，不把它们当作上述显示能力或原GUI像素一致性的证明。
