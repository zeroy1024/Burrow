# 主题与设计令牌

## 来源与生成产物

[internal/theme/tokens_src.py](../internal/theme/tokens_src.py) 是应用设计令牌的唯一来源，定义原始色板、语义值、深色覆盖和组件映射。

| 文件 | 用途 |
|---|---|
| `tokens_src.py` | 令牌来源 |
| `gen.py` | 生成 `theme.go` 与 `tokens.json` |
| `mdgen.py` | 生成 [tokens.md](../internal/theme/tokens.md) |
| `theme.go` | 应用使用的 Go 类型、浅/深主题及组件映射 |
| `accent.go` | 强调色适配 |

三层关系为 `palette → semantic → component`。界面优先读取组件令牌，缺少时使用语义令牌，不直接读取原始色板。

## 修改与生成

先编辑 `tokens_src.py`，然后从仓库根目录执行：

```sh
(
  cd internal/theme
  python3 -B gen.py
  python3 -B mdgen.py
  gofmt -w theme.go
)
```

检查来源和派生产物的差异，再运行[相应检查](development.md#已有检查)。不要只改 `theme.go`、`tokens.json`、`tokens.md`，否则下次生成会覆盖变更。

## 外观与组件适配

每次根视图构建通过 [uikit.Install](../internal/uikit/theme.go) 将当前语义颜色映射到 MyGo 主题。浅色、深色、跟随系统及强调色由应用设置控制；组件在构建时读取 `theme.Current()`。

颜色转换、多层阴影、动效和原生主题适配集中在 UIKit；阴影按令牌层序反向追加，保留首层在上的语义。减少动态效果由应用与系统偏好共同参与，布局位移和展示透明度分别处理。

新增颜色、尺寸或动效时，在来源中定义语义，再建立组件映射；同一交互的渲染、命中区域、行数计算和拖动预览使用一致的几何来源。

## 应用主题与终端配色

应用主题负责导航、控件、窗格及布局；[internal/termtheme](../internal/termtheme/) 负责终端前景、背景、ANSI 色、光标与选区。二者独立，切换应用明暗不会重置字体或固定终端配色。

`TerminalBackground` 决定终端实际底色，正文铺底、栏下补画和渐隐着色必须一致。`TerminalChrome` 按实际背景选择周边 UI 的明暗与中性文字颜色；缓存与强调色失效由应用持有。

终端布局间距由 `C.Pane.ContentSide` 提供。上/下模糊只覆盖各自栏的高度；历史文字补画范围按实际网格边界确定，不能把栏边缘直接当作文字裁剪边界。

组件职责见[共享 UI 组件](uikit.md)。
