import json,pickle
from tokens_src import *
from gen import leaves, sem_leaf, rpal, fmt, COUNTS, RES, TYP
def pv(t): return fmt(t.type, rpal(t.value))
def esc(s): return str(s).replace('|','\\|')
out=[]
A=out.append
A(f'''# SSH 客户端 UI Design Tokens

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
- 控件接入：`uiadapt.go` 的按钮、输入框、弹窗、Tooltip 入口消费 component token；原生复合控件通过 `nativeTheme` 接入，内部布局保留框架行为。
- 状态过渡：`tokenTween` 显式使用 `motion.easing.out`，状态时长为 `motion.duration.focus`（160ms）；搜索几何变化使用 `search.easing`，数字滚动使用 `groupHeader.countMotion`。
- 减少动态：MyGo 自动将位移、缩放、旋转设为终值；`animateVisual` 使用颜色 alpha 通道保留透明度过渡，不让透明度依赖几何进度。
- 图标：普通线性 SVG 初始化时读取 `iconButton.stroke`；修改几何 token 后需重启。品牌图形保留独立描边。
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
- 已知限制：深色下部分深色品牌 logo（CentOS、Debian 的深色字形）对比度偏低，实现时可以给 OS glyph 加一个 `Mix(brand, white, .35)` 提亮。

## 6. Palette
''')
def tbl(rows,head):
    A('| '+' | '.join(head)+' |'); A('|'+'---|'*len(head))
    for r in rows: A('| '+' | '.join(esc(x) for x in r)+' |')
    A('')
for g,tree in PALETTE.items():
    A(f'### palette.{g}')
    tbl([(f'`palette.{g}.{p}`',pv(v),('**derived** ' if v.derived else '')+v.desc) for p,v in leaves(tree)],['token','值','备注'])
A('## 7. Semantic（Light / Dark）\n')
for g,tree in SEM.items():
    A(f'### {g}')
    rows=[]
    for p,v in leaves(tree,g+'.'):
        dk=sem_leaf('dark',p); same=RES['light'][p]==RES['dark'][p]
        rows.append((f'`{p}`',RES['light'][p],'＝' if same else RES['dark'][p],('**derived** ' if v.derived else '')+v.desc))
    tbl(rows,['token','Light','Dark（derived）','用途'])
A('## 8. Component\n\n只列引用和 Light 下解析出的值；Dark 下自动跟随对应的 semantic。\n')
for g,d in COMP.items():
    A(f'### {g}')
    rows=[]
    for k,r in d.items():
        note=DERIVED_COMP.get(f'{g}.{k}') or DERIVED_COMP.get(g) or sem_leaf('light',r).desc
        if f'{g}.{k}' in DEPRECATED_COMP: note='**deprecated** '+DEPRECATED_COMP[f'{g}.{k}']
        der=f'{g}.{k}' in DERIVED_COMP or g in DERIVED_COMP
        rows.append((f'`{g}.{k}`',f'`{{{r}}}`',RES['light'][r],('**derived** ' if der else '')+note))
    tbl(rows,['token','引用','Light 值','说明'])
open('tokens.md','w').write('\n'.join(out))
print(len(out))
