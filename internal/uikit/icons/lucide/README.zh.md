# Lucide 图标素材

[English](README.md) | [简体中文](README.zh.md)

通用界面图标统一使用 [Lucide](https://lucide.dev/) 官方 SVG，按需收录并随 Go 程序嵌入，运行与构建均无需在线获取图标或引入 JavaScript 包。

- 版本：`1.67.0`
- 上游提交：`4b203ec9b6002fd74fd5dadc8612220e64289305`
- 素材来源：<https://github.com/lucide-icons/lucide/tree/4b203ec9b6002fd74fd5dadc8612220e64289305/icons>
- 许可证：[LICENSE](LICENSE)，保留上游完整 ISC 与 Feather 派生图标的 MIT 声明，并在应用“开源许可”中展示。

SVG 文件保持上游原样，逻辑名称与文件名的对应关系集中在 [iconsource.go](../../iconsource.go) 的 `iconFiles` 中。渲染时保留原始图形，统一使用应用主题的描边宽度、颜色与图标尺寸，并按逻辑名称和描边宽度缓存解析结果。

新增图标时，从上述固定提交取得对应 SVG，增加 `IconName` 和 `iconFiles` 映射；业务页面通过 `IconSVG`、`Icon` 或 `IconButton` 使用，不再自行拼写通用 SVG 路径。更新版本时同步核对素材、映射、`LucideVersion`、本文件和许可证。

刷新统一使用 `refresh-cw`，重试使用 `rotate-ccw`；侧栏使用 `panel-left`，关闭本地面板使用 `panel-left-close`；分栏与窗格缩放使用 `columns-2`、`maximize-2`、`minimize-2`。设置说明的悬停问号使用 `circle-question-mark`。窗格页脚的延迟与监控指标使用 `zap`、`cpu`、`memory-stick`、`arrow-down`、`arrow-up`。

Lucide 不收录品牌标识。Burrow 应用图标、操作系统标识和现有 MCP 标识单独保留，品牌图形继续通过 `BrandIcon` 或专用素材入口渲染。
