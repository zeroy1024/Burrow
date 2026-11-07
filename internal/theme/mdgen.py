"""Generate English token documentation and its Chinese counterpart."""

import json
import re
from pathlib import Path

from tokens_src import *
from gen import leaves, sem_leaf, rpal, fmt, COUNTS, RES, TYP

INTRO_ZH = f'''# Burrow 界面设计令牌

范围：整个 App 的界面（窗口、标题栏、rail、sider、分组、主机行、tab、分屏窗格及焦点、状态栏、玻璃渐隐栏、按钮、输入、徽章、tooltip、文件列表、传输队列、拖动排序）。
**不包含终端内容**：终端文字颜色、ANSI 调色板、终端字体、光标形状不在这里（只有焦点方案里非焦点窗格的空心光标描边色 `paneFocus.cursorRingUnfocused` 作为状态 token 保留）。终端卡片的*容器*（底色、圆角、描边）属于 surface token。

数值来源：`ssh-v6.html`、`common.css`、`parts/split_style.css`、`focus.py`（焦点方案 D）、`sider-drag.html`（拖动参数速查）、`ftp.html`、`logo/`（品牌蓝 #0a84ff），以及 `SSH-Tool-FTP-v3` 设计包。设计稿里没有、为补全体系推导的值都标了 **derived**（JSON 中 `$extensions.app.derived = true`，Go 注释 `[derived]`）。

| 层 | 数量 | 说明 |
|---|---|---|
| palette（primitives） | {COUNTS["palette"]} | 原始色值与刻度，其中 {COUNTS["derivedPalette"]} 个为推导值（derived） |
| semantic（每个主题一份） | {COUNTS["semantic"]} | Light = 当前设计；Dark 覆盖其中 {COUNTS["darkOverrides"]} 个（全部 derived） |
| component | {COUNTS["component"]} | 只引用 semantic；换主题时自动跟随 |

## 1. 三层结构与引用规则

```
palette.gray.900 (#1d1d1f)      ← 原始值，主题不改
   ↑
color.text.primary              ← semantic，主题只覆盖这一层
   ↑
siderItem.nameColor             ← component，只能引用 semantic
```

- **palette** 只放字面值。
- **semantic**（`theme.light` / `theme.dark`）只引用 `{{palette.*}}`。
- **component** 只引用 semantic 路径（不带主题前缀，如 `{{color.text.primary}}`），由当前主题解析。组件里不允许出现字面值，也不允许直接引用 palette。
- 深色主题 `theme.dark` 带 `"$extends": "light"`，只写覆盖项。

## 2. 命名规则

- 路径用 `.` 分层，键名 lowerCamelCase：`color.surface.controlStrong`、`siderItem.dotConnectingHalo`。
- semantic 按**用途**命名，不按色值：`color.border.subtle` 而不是 `color.gray125`。
- palette 用刻度：灰阶 `gray.0…900`（越大越深）；透明色 `black.a06` = rgba(0,0,0,.06)，`blueA.a16` = accent 16%。小数用下划线：`fontSize.12_5`、`ratio.0_35`。
- 状态后缀：`bgHover` / `bgSelected` / `bgPressed` / `bgOn`；焦点：`…Focused` / `…Unfocused` / `…Hover`。
- 玻璃上的变体加 `OnGlass`：`badge.okBgOnGlass`。
- Go 中自动转为 UpperCamelCase：`theme.Current().C.SiderItem.NameColor`、`t.Color.Text.Primary`；以数字开头的键加 `N`：`Palette.Gray.N900`。

## 3. 在 Go / MyGo 中使用

`theme.go` 是独立 package（`package theme`），只依赖标准库，不调用任何 MyGo API：

- 颜色：`image/color.NRGBA`（非预乘 alpha）。如果 MyGo 的颜色类型不同，写一个 `func toUI(c color.NRGBA) ui.Color` 适配即可。
- 尺寸：`float32`，逻辑像素（与设计稿 CSS px 1:1）。
- 阴影：`theme.Shadow`（`[]ShadowLayer{{X,Y,Blur,Spread,Color,Inset}}`）。`Blur=0, Spread>0` 是描边环。如果 MyGo 只支持单层阴影，就按顺序叠绘或用描边 + 阴影组合。
- 动效：`time.Duration`。弹簧 `theme.Spring` 带 `Stiffness/Damping`（让位）或 `Duration/Bounce`（落位、取消），分别对应 MyGo 的 `ui.SpringOptions{{Stiffness, Damping}}` / `{{Duration, Bounce}}`（`motion.spring.smooth` ≈ `ui.Smooth`）。
- 玻璃渐隐栏：`C.FadeBar` 给出 3 层 blur 半径与遮罩停点，以及着色起止 alpha（着色色 = 所在卡片底色）。如果 GPU 不支持 backdrop blur，退化为只画着色渐变。
- 取当前主题：`theme.Current()`；切换：`theme.Set(theme.Dark())`；监听：`cancel := theme.OnChange(func(t *theme.Theme){{ /* 请求重绘 */ }})`。
- 跟随系统外观：在系统外观变化回调里 `theme.Set(theme.ForAppearance(isDark))`（MyGo 的外观 API 以官方文档为准）。
- 控件接入：`internal/uikit` 消费 component token 并适配原生主题，内部布局保留框架行为。
- 状态过渡：显式使用 `motion.easing.out`，状态时长为 `motion.duration.focus`（160ms）；搜索几何变化使用 `search.easing`，数字滚动使用 `groupHeader.countMotion`。
- 减少动态：MyGo 自动将位移、缩放、旋转设为终值；视觉 alpha 保留透明度过渡，不让透明度依赖几何进度。
- 图标：普通 SVG 使用主题线宽，按逻辑名称和线宽缓存解析结果；品牌图形单独处理。
- OS logo 底色：`t.OSLogoBg(brand)` = 品牌色按 `opacity.osTint` 混入卡片色（浅色 9%，深色 18%）。

## 4. 新增一个主题

1. 需要新的原始色时，先加到 `tokens_src.py` 的 `PALETTE`（例如 `PALETTE['solarized']`）。
2. 像 `DARK` 一样写一个覆盖表：`{{'color.bg.window': P('solarized.base3'), ...}}`，只写 semantic 路径。
3. 在 `gen.py` 中注册（或者直接在 Go 里：`s := theme.LightSemantic(); s.Color.Accent.Default = theme.Hex("#268bd2"); t := theme.New("solar", false, s)`）。
4. 在 `internal/theme` 运行 `python3 -B gen.py && python3 -B mdgen.py && gofmt -w theme.go`，随后在仓库根目录运行已有构建、vet 和测试。
5. 组件层不用改；如果某个组件在新主题下不对，说明缺一个 semantic token，应该加 semantic，而不是在组件里写死。

## 5. 推导值（derived）清单

- 设计稿没画：`color.state.pressed`（按下）、`size.dropLine`（MyGo `ListState.Reorder` 插入线 2px）、组件 `menu.*`（按浮起层/拖动卡片推导）、`button.bgHover/bgPressed`、`iconButton.bgHover`。
- tab 拖动、窗格拖动没有单独设计：`reorderMotion.tabDisplace / paneDisplace / paneDrop` 沿用 sider 拖动的弹簧参数。
- 整个 Dark 主题（{COUNTS["darkOverrides"]} 项覆盖 + `palette.graphite / darkTint / whiteA / blackA / inkDark`）。
- FTP v3 danger 红色、原生文件面板补充尺寸与演示传输吞吐标记为 **derived**。侧栏旧连接状态与会话选中 token 已标 `deprecated`，保留生成供已有主题兼容，新组件不得引用；独立的 `siderItem.bgActive` 只表示目录条目的原生焦点反馈。
- 同一含义在稿子里出现过两个值时，取主稿：旧 sider 会话选中兼容 token 保留 `common.css` 的 `#efeff1`（`sider-drag.html` 里是 `#f2f2f4`）；新目录焦点反馈 `bgActive` 跟随 `color.state.selectedAccent`。图标线宽用 `icons.js` 默认 1.75（拖动稿用 1.7）。
- 检查深色品牌字形对比度（如 CentOS、Debian）；渲染时可向白色混合，不改写品牌原色。

## 6. Palette
'''

INTRO_EN = f'''# Burrow UI design tokens

Scope: the app UI, including windows, title bars, rail, sidebar, groups, host rows, tabs, splits/focus, status bars, glass fades, buttons, inputs, badges, tooltips, file lists, transfers, and drag ordering.
**Terminal contents are separate**: foreground/ANSI colors, terminal fonts, and cursor shapes are outside this model, except `paneFocus.cursorRingUnfocused`. Terminal card containers belong to surface tokens.

Design sources: `ssh-v6.html`, `common.css`, `parts/split_style.css`, `focus.py` (focus scheme D), `sider-drag.html` (drag parameters), `ftp.html`, `logo/` (brand blue #0a84ff), and `SSH-Tool-FTP-v3`. Values inferred to complete the system are **derived** (`$extensions.app.derived = true` in JSON; `[derived]` in Go).

| Layer | Count | Purpose |
|---|---|---|
| palette (primitives) | {COUNTS["palette"]} | Literal colors/scales, including {COUNTS["derivedPalette"]} derived values |
| semantic (per theme) | {COUNTS["semantic"]} | Light follows the design; Dark has {COUNTS["darkOverrides"]} derived overrides |
| component | {COUNTS["component"]} | Semantic references only; follows the current theme |

## 1. Layers and reference rules

```
palette.gray.900 (#1d1d1f)      <- primitive, unchanged by themes
   ^
color.text.primary             <- semantic, overridden by themes
   ^
siderItem.nameColor             <- component, references semantics only
```

- **palette** contains literals only.
- **semantic** (`theme.light` / `theme.dark`) references only `{{palette.*}}`.
- **component** references semantic paths without theme prefixes, such as `{{color.text.primary}}`, resolved by the current theme. No literals or direct palette references.
- Dark uses `"$extends": "light"` and contains overrides only.

## 2. Naming

- Dot-separated lowerCamelCase paths: `color.surface.controlStrong`, `siderItem.dotConnectingHalo`.
- Name semantics by purpose: `color.border.subtle`, not `color.gray125`.
- Palette scales include `gray.0…900` (darker as numbers rise), `black.a06` for rgba(0,0,0,.06), and `blueA.a16` for 16% accent. Decimals use underscores: `fontSize.12_5`, `ratio.0_35`.
- State suffixes: `bgHover`, `bgSelected`, `bgPressed`, `bgOn`; focus: `…Focused`, `…Unfocused`, `…Hover`.
- Glass variants use `OnGlass`, such as `badge.okBgOnGlass`.
- Go names become UpperCamelCase: `theme.Current().C.SiderItem.NameColor`, `t.Color.Text.Primary`. Numeric-leading keys get `N`, such as `Palette.Gray.N900`.

## 3. Go / MyGo usage

`theme.go` is an independent standard-library-only `package theme` and calls no MyGo APIs:

- Colors: non-premultiplied `image/color.NRGBA`; adapt to UI color types with a function such as `func toUI(c color.NRGBA) ui.Color`.
- Dimensions: `float32` logical pixels, corresponding 1:1 with design CSS px.
- Shadows: `theme.Shadow` (`[]ShadowLayer{{X,Y,Blur,Spread,Color,Inset}}`). `Blur=0, Spread>0` creates a border ring. Stack layers or combine border/shadow when only one native layer is available.
- Motion: `time.Duration`; `theme.Spring` has `Stiffness/Damping` for displacement or `Duration/Bounce` for drop/cancel, matching MyGo `ui.SpringOptions{{Stiffness, Damping}}` / `{{Duration, Bounce}}`. `motion.spring.smooth` approximates `ui.Smooth`.
- Glass fades: `C.FadeBar` specifies three blur radii/mask stops and tint alpha endpoints using the containing card color. Without GPU backdrop blur, draw tint gradients only.
- Current theme: `theme.Current()`; switch with `theme.Set(theme.Dark())`; subscribe with `cancel := theme.OnChange(func(t *theme.Theme){{ /* request redraw */ }})`.
- System appearance: call `theme.Set(theme.ForAppearance(isDark))` from appearance callbacks; use current official MyGo APIs.
- Controls: `internal/uikit` consumes component tokens and maps native themes while retaining framework layout behavior.
- Transitions use explicit `motion.easing.out` and `motion.duration.focus` (160ms). Search geometry uses `search.easing`; counter motion uses `groupHeader.countMotion`.
- Reduced motion ends geometric translation/scale/rotation immediately. Visual alpha retains fades independently of geometry progress.
- General SVGs use themed stroke widths and cache by logical icon/stroke width; brand marks stay separate.
- OS logo backgrounds use `t.OSLogoBg(brand)`, mixing brand into card via `opacity.osTint` (light 9%, dark 18%).

## 4. Add a theme

1. Add new primitives to `PALETTE` in `tokens_src.py`, such as `PALETTE['solarized']`.
2. Create semantic overrides like `DARK`: `{{'color.bg.window': P('solarized.base3'), ...}}`.
3. Register in `gen.py`, or in Go: `s := theme.LightSemantic(); s.Color.Accent.Default = theme.Hex("#268bd2"); t := theme.New("solar", false, s)`.
4. In `internal/theme`, run `python3 -B gen.py && python3 -B mdgen.py && gofmt -w theme.go`, then existing builds/vet/tests from the repository root.
5. Components need no changes. A theme-specific component issue usually needs a missing semantic token, not hardcoded component values.

## 5. Derived values

- Missing from designs: pressed state, `size.dropLine` (2px MyGo `ListState.Reorder` line), `menu.*` derived from raised/dragged surfaces, button hover/pressed, icon-button hover.
- Tab/pane drag reuse sidebar springs: `reorderMotion.tabDisplace / paneDisplace / paneDrop`.
- The complete Dark theme: {COUNTS["darkOverrides"]} overrides and `palette.graphite / darkTint / whiteA / blackA / inkDark`.
- FTP v3 danger red, extra native-pane dimensions, and demo throughput are derived. Deprecated sidebar connection/selection tokens remain generated for compatibility but must not be used by new components. `siderItem.bgActive` is catalog focus only.
- Conflicting designs follow the main source: legacy sidebar selection retains `common.css` `#efeff1` instead of drag-design `#f2f2f4`; current focus follows `color.state.selectedAccent`. Icon stroke uses 1.75 from `icons.js`, not drag-design 1.7.
- Check dark brand-glyph contrast (for example CentOS/Debian); glyph rendering can mix toward white without rewriting the original brand palette.

## 6. Palette
'''

def pv(token):
    return fmt(token.type, rpal(token.value))


def esc(value):
    return str(value).replace("|", "\\|")


translations = json.loads(Path("tokens_docs.en.json").read_text(encoding="utf-8"))


def english_note(note):
    if re.search(r"[\u3400-\u4dbf\u4e00-\u9fff]", note):
        return translations[note.strip()]
    return note


def document(english):
    out = [INTRO_EN if english else INTRO_ZH]
    note = english_note if english else lambda value: value

    def table(rows, headings):
        out.append("| " + " | ".join(headings) + " |")
        out.append("|" + "---|" * len(headings))
        for row in rows:
            out.append("| " + " | ".join(esc(value) for value in row) + " |")
        out.append("")

    for group, tree in PALETTE.items():
        out.append(f"### palette.{group}")
        table([
            (f"`palette.{group}.{path}`", pv(value),
             ("**derived** " if value.derived else "") + note(value.desc))
            for path, value in leaves(tree)
        ], ["token", "Value", "Notes"] if english
              else ["token", "\u503c", "\u5907\u6ce8"])

    out.append("## 7. Semantic (Light / Dark)\n" if english
               else "## 7. Semantic\uff08Light / Dark\uff09\n")
    for group, tree in SEM.items():
        out.append(f"### {group}")
        rows = []
        for path, value in leaves(tree, group + "."):
            same = RES["light"][path] == RES["dark"][path]
            rows.append((f"`{path}`", RES["light"][path],
                         "\uff1d" if same else RES["dark"][path],
                         ("**derived** " if value.derived else "") + note(value.desc)))
        table(rows, ["token", "Light", "Dark (derived)", "Purpose"] if english
              else ["token", "Light", "Dark\uff08derived\uff09", "\u7528\u9014"])

    out.append("## 8. Component\n\nReferences and resolved Light values; Dark follows the corresponding semantic values.\n"
               if english else "## 8. Component\n\n\u53ea\u5217\u5f15\u7528\u548c Light \u4e0b\u89e3\u6790\u51fa\u7684\u503c\uff1bDark \u4e0b\u81ea\u52a8\u8ddf\u968f\u5bf9\u5e94\u7684 semantic\u3002\n")
    for group, components in COMP.items():
        out.append(f"### {group}")
        rows = []
        for key, reference in components.items():
            path = f"{group}.{key}"
            description = DERIVED_COMP.get(path) or DERIVED_COMP.get(group) or sem_leaf("light", reference).desc
            if path in DEPRECATED_COMP:
                description = "**deprecated** " + note(DEPRECATED_COMP[path])
            else:
                description = note(description)
            derived = path in DERIVED_COMP or group in DERIVED_COMP
            rows.append((f"`{path}`", f"`{{{reference}}}`", RES["light"][reference],
                         ("**derived** " if derived else "") + description))
        table(rows, ["token", "Reference", "Light value", "Description"] if english
              else ["token", "\u5f15\u7528", "Light \u503c", "\u8bf4\u660e"])
    return "\n".join(out)


english = document(True)
chinese = document(False)
for filename, text in [("tokens.md", english), ("tokens.zh.md", chinese)]:
    heading, body = text.split("\n", 1)
    navigation = "[English](tokens.md) | [\u7b80\u4f53\u4e2d\u6587](tokens.zh.md)"
    Path(filename).write_text(heading + "\n\n" + navigation + body, encoding="utf-8")
print("Generated tokens.md and tokens.zh.md")
