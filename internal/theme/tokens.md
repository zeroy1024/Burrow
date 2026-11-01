# SSH 客户端 UI Design Tokens

范围：整个 App 的界面（窗口、标题栏、rail、sider、分组、主机行、tab、分屏窗格及焦点、状态栏、玻璃渐隐栏、按钮、输入、徽章、tooltip、文件列表、传输队列、拖动排序）。
**不包含终端内容**：终端文字颜色、ANSI 调色板、终端字体、光标形状不在这里（只有焦点方案里非焦点窗格的空心光标描边色 `paneFocus.cursorRingUnfocused` 作为状态 token 保留）。终端卡片的*容器*（底色、圆角、描边）属于 surface token。

数值来源：`ssh-v6.html`、`common.css`、`parts/split_style.css`、`focus.py`（焦点方案 D）、`sider-drag.html`（拖动参数速查）、`ftp.html`、`logo/`（品牌蓝 #0a84ff），以及 `SSH-Tool-FTP-v3` 设计包。设计稿里没有、为补全体系推导的值都标了 **derived**（JSON 中 `$extensions.app.derived = true`，Go 注释 `[derived]`）。

| 层 | 数量 | 说明 |
|---|---|---|
| palette（primitives） | 279 | 原始色值与刻度，其中 49 个为推导值（derived） |
| semantic（每个主题一份） | 426 | Light = 当前设计；Dark 覆盖其中 104 个（全部 derived） |
| component | 881 | 只引用 semantic；换主题时自动跟随 |

## 1. 三层结构与引用规则

```
palette.gray.900 (#1d1d1f)      ← 原始值，主题不改
   ↑
color.text.primary              ← semantic，主题只覆盖这一层
   ↑
siderItem.nameColor             ← component，只能引用 semantic
```

- **palette** 只放字面值。
- **semantic**（`theme.light` / `theme.dark`）只引用 `{palette.*}`。
- **component** 只引用 semantic 路径（不带主题前缀，如 `{color.text.primary}`），由当前主题解析。组件里不允许出现字面值，也不允许直接引用 palette。
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
- 阴影：`theme.Shadow`（`[]ShadowLayer{X,Y,Blur,Spread,Color,Inset}`）。`Blur=0, Spread>0` 是描边环。如果 MyGo 只支持单层阴影，就按顺序叠绘或用描边 + 阴影组合。
- 动效：`time.Duration`。弹簧 `theme.Spring` 带 `Stiffness/Damping`（让位）或 `Duration/Bounce`（落位、取消），分别对应 MyGo 的 `ui.SpringOptions{Stiffness, Damping}` / `{Duration, Bounce}`（`motion.spring.smooth` ≈ `ui.Smooth`）。
- 玻璃渐隐栏：`C.FadeBar` 给出 3 层 blur 半径与遮罩停点，以及着色起止 alpha（着色色 = 所在卡片底色）。如果 GPU 不支持 backdrop blur，退化为只画着色渐变。
- 取当前主题：`theme.Current()`；切换：`theme.Set(theme.Dark())`；监听：`cancel := theme.OnChange(func(t *theme.Theme){ /* 请求重绘 */ })`。
- 跟随系统外观：在系统外观变化回调里 `theme.Set(theme.ForAppearance(isDark))`（MyGo 的外观 API 以官方文档为准）。
- 控件接入：`uiadapt.go` 的按钮、输入框、弹窗、Tooltip 入口消费 component token；原生复合控件通过 `nativeTheme` 接入，内部布局保留框架行为。
- 状态过渡：`tokenTween` 显式使用 `motion.easing.out`，状态时长为 `motion.duration.focus`（160ms）；搜索几何变化使用 `search.easing`，数字滚动使用 `groupHeader.countMotion`。
- 减少动态：MyGo 自动将位移、缩放、旋转设为终值；`animateVisual` 使用颜色 alpha 通道保留透明度过渡，不让透明度依赖几何进度。
- 图标：普通线性 SVG 初始化时读取 `iconButton.stroke`；修改几何 token 后需重启。品牌图形保留独立描边。
- OS logo 底色：`t.OSLogoBg(brand)` = 品牌色按 `opacity.osTint` 混入卡片色（浅色 9%，深色 18%）。

## 4. 新增一个主题

1. 需要新的原始色时，先加到 `tokens_src.py` 的 `PALETTE`（例如 `PALETTE['solarized']`）。
2. 像 `DARK` 一样写一个覆盖表：`{'color.bg.window': P('solarized.base3'), ...}`，只写 semantic 路径。
3. 在 `gen.py` 中注册（或者直接在 Go 里：`s := theme.LightSemantic(); s.Color.Accent.Default = theme.Hex("#268bd2"); t := theme.New("solar", false, s)`）。
4. 在 `internal/theme` 运行 `python3 -B gen.py && python3 -B mdgen.py && gofmt -w theme.go`，随后在仓库根目录运行已有构建、vet 和测试。
5. 组件层不用改；如果某个组件在新主题下不对，说明缺一个 semantic token，应该加 semantic，而不是在组件里写死。

## 5. 推导值（derived）清单

- 设计稿没画：`color.state.pressed`（按下）、`size.dropLine`（MyGo `ListState.Reorder` 插入线 2px）、组件 `menu.*`（按浮起层/拖动卡片推导）、`button.bgHover/bgPressed`、`iconButton.bgHover`。
- tab 拖动、窗格拖动没有单独设计：`reorderMotion.tabDisplace / paneDisplace / paneDrop` 沿用 sider 拖动的弹簧参数。
- 整个 Dark 主题（104 项覆盖 + `palette.graphite / darkTint / whiteA / blackA / inkDark`）。
- FTP v3 danger 红色、原生文件面板补充尺寸与演示传输吞吐标记为 **derived**。侧栏状态与选中 token 已标 `deprecated`，保留生成供已有主题兼容，新组件不得引用。
- 同一含义在稿子里出现过两个值时，取主稿：sider 选中行用 `common.css` 的 `#efeff1`（`sider-drag.html` 里是 `#f2f2f4`）；图标线宽用 `icons.js` 默认 1.75（拖动稿用 1.7）。
- 已知限制：深色下部分深色品牌 logo（CentOS、Debian 的深色字形）对比度偏低，实现时可以给 OS glyph 加一个 `Mix(brand, white, .35)` 提亮。

## 6. Palette

### palette.gray
| token | 值 | 备注 |
|---|---|---|
| `palette.gray.0` | #ffffff |  |
| `palette.gray.25` | #fbfbfc |  |
| `palette.gray.40` | #fafafb |  |
| `palette.gray.50` | #f7f7f8 |  |
| `palette.gray.60` | #f6f6f7 |  |
| `palette.gray.75` | #f4f4f5 |  |
| `palette.gray.100` | #f2f2f4 |  |
| `palette.gray.125` | #f0f0f2 |  |
| `palette.gray.150` | #efeff1 |  |
| `palette.gray.175` | #ececef |  |
| `palette.gray.190` | #ededf0 |  |
| `palette.gray.200` | #e7e7e8 |  |
| `palette.gray.225` | #e6e6e9 |  |
| `palette.gray.250` | #e3e3e6 |  |
| `palette.gray.300` | #d6d6d8 |  |
| `palette.gray.325` | #d2d2d6 |  |
| `palette.gray.350` | #cfcfd4 |  |
| `palette.gray.400` | #c7c7cc |  |
| `palette.gray.450` | #b0b0b5 |  |
| `palette.gray.475` | #aeaeb2 |  |
| `palette.gray.500` | #a1a1a6 |  |
| `palette.gray.600` | #8e8e93 |  |
| `palette.gray.700` | #6e6e73 |  |
| `palette.gray.750` | #5b5b62 |  |
| `palette.gray.800` | #4a4a50 |  |
| `palette.gray.850` | #3a3a3f |  |
| `palette.gray.900` | #1d1d1f |  |

### palette.blue
| token | 值 | 备注 |
|---|---|---|
| `palette.blue.50` | #eaf3fe |  |
| `palette.blue.75` | #e8f2fe |  |
| `palette.blue.100` | #d7e9fd | 文件夹图标填充 |
| `palette.blue.300` | #5ba7f7 |  |
| `palette.blue.500` | #0a84ff | 品牌蓝 / accent（logo 同色） |
| `palette.blue.600` | #0a6fd6 |  |
| `palette.blue.700` | #0a5bc4 |  |

### palette.green
| token | 值 | 备注 |
|---|---|---|
| `palette.green.50` | #ebf8ef |  |
| `palette.green.500` | #30c45a |  |
| `palette.green.700` | #1f8a3f |  |

### palette.amber
| token | 值 | 备注 |
|---|---|---|
| `palette.amber.50` | #fdf3e3 |  |
| `palette.amber.500` | #f5a623 |  |
| `palette.amber.600` | #c7861a |  |
| `palette.amber.700` | #b26a00 |  |

### palette.traffic
| token | 值 | 备注 |
|---|---|---|
| `palette.traffic.red` | #ff5f57 |  |
| `palette.traffic.yellow` | #febc2e |  |
| `palette.traffic.green` | #28c840 |  |

### palette.black
| token | 值 | 备注 |
|---|---|---|
| `palette.black.a035` | rgba(0,0,0,0.035) |  |
| `palette.black.a04` | rgba(0,0,0,0.04) |  |
| `palette.black.a05` | rgba(0,0,0,0.05) |  |
| `palette.black.a06` | rgba(0,0,0,0.06) |  |
| `palette.black.a07` | rgba(0,0,0,0.07) |  |
| `palette.black.a08` | rgba(0,0,0,0.08) |  |
| `palette.black.a09` | rgba(0,0,0,0.09) |  |
| `palette.black.a10` | rgba(0,0,0,0.1) |  |
| `palette.black.a12` | rgba(0,0,0,0.12) |  |
| `palette.black.a14` | rgba(0,0,0,0.14) |  |
| `palette.black.a15` | rgba(0,0,0,0.15) |  |
| `palette.black.a16` | rgba(0,0,0,0.16) |  |
| `palette.black.a18` | rgba(0,0,0,0.18) |  |

### palette.white
| token | 值 | 备注 |
|---|---|---|
| `palette.white.a30` | rgba(255,255,255,0.3) |  |
| `palette.white.a42` | rgba(255,255,255,0.42) |  |
| `palette.white.a60` | rgba(255,255,255,0.6) |  |
| `palette.white.a70` | rgba(255,255,255,0.7) |  |
| `palette.white.a85` | rgba(255,255,255,0.85) |  |
| `palette.white.a90` | rgba(255,255,255,0.9) |  |

### palette.blueA
| token | 值 | 备注 |
|---|---|---|
| `palette.blueA.a035` | rgba(10,132,255,0.035) |  |
| `palette.blueA.a06` | rgba(10,132,255,0.06) |  |
| `palette.blueA.a08` | rgba(10,132,255,0.08) |  |
| `palette.blueA.a10` | rgba(10,132,255,0.1) |  |
| `palette.blueA.a13` | rgba(10,132,255,0.13) |  |
| `palette.blueA.a15` | rgba(10,132,255,0.15) |  |
| `palette.blueA.a16` | rgba(10,132,255,0.16) |  |
| `palette.blueA.a18` | rgba(10,132,255,0.18) |  |
| `palette.blueA.a28` | rgba(10,132,255,0.28) |  |
| `palette.blueA.a35` | rgba(10,132,255,0.35) |  |
| `palette.blueA.a50` | rgba(10,132,255,0.5) |  |
| `palette.blueA.a24` | rgba(10,132,255,0.24) |  |

### palette.greenA
| token | 值 | 备注 |
|---|---|---|
| `palette.greenA.a14` | rgba(48,196,90,0.14) |  |
| `palette.greenA.a15` | rgba(48,196,90,0.15) |  |

### palette.amberA
| token | 值 | 备注 |
|---|---|---|
| `palette.amberA.a14` | rgba(245,166,35,0.14) |  |
| `palette.amberA.a18` | rgba(245,166,35,0.18) |  |

### palette.ink
| token | 值 | 备注 |
|---|---|---|
| `palette.ink.a92` | rgba(30,30,32,0.92) | tooltip 底 |

### palette.graphite
| token | 值 | 备注 |
|---|---|---|
| `palette.graphite.950` | #161618 | **derived** 深色主题 |
| `palette.graphite.900` | #1c1c1e | **derived** 深色主题 |
| `palette.graphite.850` | #202022 | **derived** 深色主题 |
| `palette.graphite.800` | #252527 | **derived** 深色主题 |
| `palette.graphite.750` | #2a2a2d | **derived** 深色主题 |
| `palette.graphite.700` | #2e2e31 | **derived** 深色主题 |
| `palette.graphite.650` | #333336 | **derived** 深色主题 |
| `palette.graphite.600` | #3a3a3d | **derived** 深色主题 |
| `palette.graphite.550` | #444448 | **derived** 深色主题 |
| `palette.graphite.500` | #5a5a5f | **derived** 深色主题 |
| `palette.graphite.400` | #6e6e73 | **derived** 深色主题 |
| `palette.graphite.300` | #8e8e93 | **derived** 深色主题 |
| `palette.graphite.250` | #98989d | **derived** 深色主题 |
| `palette.graphite.200` | #a1a1a6 | **derived** 深色主题 |
| `palette.graphite.150` | #c7c7cc | **derived** 深色主题 |
| `palette.graphite.100` | #d1d1d6 | **derived** 深色主题 |
| `palette.graphite.50` | #f5f5f7 | **derived** 深色主题 |

### palette.darkTint
| token | 值 | 备注 |
|---|---|---|
| `palette.darkTint.blue300` | #4da3ff | **derived** 深色主题 |
| `palette.darkTint.blue200` | #6cb4ff | **derived** 深色主题 |
| `palette.darkTint.blueSubtle` | #16304d | **derived** 深色主题 |
| `palette.darkTint.green400` | #32d74b | **derived** 深色主题 |
| `palette.darkTint.green300` | #5fd97a | **derived** 深色主题 |
| `palette.darkTint.greenSubtle` | #173523 | **derived** 深色主题 |
| `palette.darkTint.amber400` | #ffb340 | **derived** 深色主题 |
| `palette.darkTint.amber300` | #ffc15c | **derived** 深色主题 |
| `palette.darkTint.amberSubtle` | #3d2e12 | **derived** 深色主题 |
| `palette.darkTint.red400` | #ff453a | **derived** 深色 danger |
| `palette.darkTint.red300` | #ff6961 | **derived** 深色 danger 文字 |
| `palette.darkTint.redSubtle` | #3a1d1c | **derived** 深色 danger 浅底 |

### palette.whiteA
| token | 值 | 备注 |
|---|---|---|
| `palette.whiteA.a06` | rgba(255,255,255,0.06) | **derived** 深色主题 |
| `palette.whiteA.a08` | rgba(255,255,255,0.08) | **derived** 深色主题 |
| `palette.whiteA.a10` | rgba(255,255,255,0.1) | **derived** 深色主题 |
| `palette.whiteA.a12` | rgba(255,255,255,0.12) | **derived** 深色主题 |
| `palette.whiteA.a14` | rgba(255,255,255,0.14) | **derived** 深色主题 |
| `palette.whiteA.a16` | rgba(255,255,255,0.16) | **derived** 深色主题 |
| `palette.whiteA.a20` | rgba(255,255,255,0.2) | **derived** 深色主题 |

### palette.blackA
| token | 值 | 备注 |
|---|---|---|
| `palette.blackA.a25` | rgba(0,0,0,0.25) | **derived** 深色主题 |
| `palette.blackA.a30` | rgba(0,0,0,0.3) | **derived** 深色主题 |
| `palette.blackA.a35` | rgba(0,0,0,0.35) | **derived** 深色主题 |
| `palette.blackA.a40` | rgba(0,0,0,0.4) | **derived** 深色主题 |
| `palette.blackA.a45` | rgba(0,0,0,0.45) | **derived** 深色主题 |
| `palette.blackA.a50` | rgba(0,0,0,0.5) | **derived** 深色主题 |

### palette.inkDark
| token | 值 | 备注 |
|---|---|---|
| `palette.inkDark.a95` | rgba(58,58,62,0.95) | **derived** 深色 tooltip 底 |

### palette.space
| token | 值 | 备注 |
|---|---|---|
| `palette.space.0` | 0px |  |
| `palette.space.1` | 1px |  |
| `palette.space.2` | 2px |  |
| `palette.space.3` | 3px |  |
| `palette.space.4` | 4px |  |
| `palette.space.5` | 5px |  |
| `palette.space.6` | 6px |  |
| `palette.space.7` | 7px |  |
| `palette.space.8` | 8px |  |
| `palette.space.9` | 9px |  |
| `palette.space.10` | 10px |  |
| `palette.space.12` | 12px |  |
| `palette.space.14` | 14px |  |
| `palette.space.15` | 15px |  |
| `palette.space.16` | 16px |  |
| `palette.space.18` | 18px |  |
| `palette.space.22` | 22px |  |
| `palette.space.24` | 24px |  |
| `palette.space.20` | 20px |  |

### palette.radius
| token | 值 | 备注 |
|---|---|---|
| `palette.radius.1` | 1px |  |
| `palette.radius.2` | 2px |  |
| `palette.radius.3` | 3px |  |
| `palette.radius.4` | 4px |  |
| `palette.radius.5` | 5px |  |
| `palette.radius.6` | 6px |  |
| `palette.radius.7` | 7px |  |
| `palette.radius.8` | 8px |  |
| `palette.radius.9` | 9px |  |
| `palette.radius.10` | 10px |  |
| `palette.radius.12` | 12px |  |
| `palette.radius.20` | 20px |  |

### palette.size
| token | 值 | 备注 |
|---|---|---|
| `palette.size.0_5` | 0.5px |  |
| `palette.size.1` | 1px |  |
| `palette.size.1_5` | 1.5px |  |
| `palette.size.1_75` | 1.75px |  |
| `palette.size.2` | 2px |  |
| `palette.size.3` | 3px |  |
| `palette.size.4` | 4px |  |
| `palette.size.5` | 5px |  |
| `palette.size.6` | 6px |  |
| `palette.size.7` | 7px |  |
| `palette.size.12` | 12px |  |
| `palette.size.14` | 14px |  |
| `palette.size.15` | 15px |  |
| `palette.size.16` | 16px |  |
| `palette.size.18` | 18px |  |
| `palette.size.22` | 22px |  |
| `palette.size.24` | 24px |  |
| `palette.size.26` | 26px |  |
| `palette.size.28` | 28px |  |
| `palette.size.30` | 30px |  |
| `palette.size.32` | 32px |  |
| `palette.size.34` | 34px |  |
| `palette.size.36` | 36px |  |
| `palette.size.40` | 40px |  |
| `palette.size.44` | 44px |  |
| `palette.size.48` | 48px |  |
| `palette.size.52` | 52px |  |
| `palette.size.56` | 56px |  |
| `palette.size.204` | 204px |  |
| `palette.size.236` | 236px |  |
| `palette.size.296` | 296px |  |
| `palette.size.10` | 10px |  |
| `palette.size.42` | 42px |  |
| `palette.size.50` | 50px |  |
| `palette.size.80` | 80px |  |
| `palette.size.90` | 90px |  |
| `palette.size.100` | 100px |  |
| `palette.size.120` | 120px |  |
| `palette.size.140` | 140px |  |
| `palette.size.160` | 160px |  |
| `palette.size.180` | 180px |  |
| `palette.size.240` | 240px |  |
| `palette.size.300` | 300px |  |
| `palette.size.320` | 320px |  |
| `palette.size.384` | 384px |  |
| `palette.size.420` | 420px |  |
| `palette.size.720` | 720px |  |

### palette.fontSize
| token | 值 | 备注 |
|---|---|---|
| `palette.fontSize.10_5` | 10.5px |  |
| `palette.fontSize.11` | 11px |  |
| `palette.fontSize.11_5` | 11.5px |  |
| `palette.fontSize.12` | 12px |  |
| `palette.fontSize.12_5` | 12.5px |  |
| `palette.fontSize.13` | 13px |  |
| `palette.fontSize.15` | 15px |  |

### palette.lineHeight
| token | 值 | 备注 |
|---|---|---|
| `palette.lineHeight.14` | 14px |  |
| `palette.lineHeight.15` | 15px |  |
| `palette.lineHeight.16` | 16px |  |
| `palette.lineHeight.17` | 17px |  |
| `palette.lineHeight.18` | 18px |  |

### palette.fontWeight
| token | 值 | 备注 |
|---|---|---|
| `palette.fontWeight.400` | 400 |  |
| `palette.fontWeight.500` | 500 |  |
| `palette.fontWeight.600` | 600 |  |

### palette.fontFamily
| token | 值 | 备注 |
|---|---|---|
| `palette.fontFamily.system` | -apple-system, SF Pro Text, PingFang SC, Inter, Noto Sans CJK SC, system-ui, sans-serif |  |
| `palette.fontFamily.mono` | JetBrains Mono, Menlo, monospace | 仅用于主机名/路径等 UI 文本，不用于终端内容 |

### palette.letterSpacing
| token | 值 | 备注 |
|---|---|---|
| `palette.letterSpacing.neg0_2` | -0.2px |  |
| `palette.letterSpacing.zero` | 0px |  |
| `palette.letterSpacing.pos0_3` | 0.3px |  |

### palette.blur
| token | 值 | 备注 |
|---|---|---|
| `palette.blur.1_5` | 1.5px |  |
| `palette.blur.4` | 4px |  |
| `palette.blur.5` | 5px |  |
| `palette.blur.6` | 6px |  |
| `palette.blur.9` | 9px |  |
| `palette.blur.16` | 16px |  |
| `palette.blur.8` | 8px |  |

### palette.ratio
| token | 值 | 备注 |
|---|---|---|
| `palette.ratio.0` | 0 |  |
| `palette.ratio.0_09` | 0.09 |  |
| `palette.ratio.0_15` | 0.15 |  |
| `palette.ratio.0_18` | 0.18 |  |
| `palette.ratio.0_3` | 0.3 |  |
| `palette.ratio.0_35` | 0.35 |  |
| `palette.ratio.0_4` | 0.4 |  |
| `palette.ratio.0_45` | 0.45 |  |
| `palette.ratio.0_5` | 0.5 |  |
| `palette.ratio.0_55` | 0.55 |  |
| `palette.ratio.0_6` | 0.6 |  |
| `palette.ratio.0_7` | 0.7 |  |
| `palette.ratio.0_78` | 0.78 |  |
| `palette.ratio.0_8` | 0.8 |  |
| `palette.ratio.0_92` | 0.92 |  |
| `palette.ratio.1` | 1 |  |
| `palette.ratio.1_03` | 1.03 |  |
| `palette.ratio.1_5` | 1.5 |  |
| `palette.ratio.1_8` | 1.8 |  |
| `palette.ratio.0_9` | 0.9 |  |
| `palette.ratio.0_96` | 0.96 |  |

### palette.duration
| token | 值 | 备注 |
|---|---|---|
| `palette.duration.0` | 0ms |  |
| `palette.duration.120` | 120ms |  |
| `palette.duration.150` | 150ms |  |
| `palette.duration.160` | 160ms |  |
| `palette.duration.180` | 180ms |  |
| `palette.duration.220` | 220ms |  |
| `palette.duration.300` | 300ms |  |
| `palette.duration.320` | 320ms |  |
| `palette.duration.500` | 500ms |  |
| `palette.duration.600` | 600ms |  |
| `palette.duration.800` | 800ms |  |
| `palette.duration.3000` | 3000ms |  |

### palette.speed
| token | 值 | 备注 |
|---|---|---|
| `palette.speed.autoScroll` | 900 | px/s |
| `palette.speed.demoTransfer` | 1.30023e+07 | **derived** 演示传输字节/秒，12.4 MiB/s |

### palette.spring
| token | 值 | 备注 |
|---|---|---|
| `palette.spring.snappy` | stiffness 420 · damping 34 |  |
| `palette.spring.drop` | duration 320ms · bounce 0.15 |  |
| `palette.spring.smooth` | duration 300ms · bounce 0 |  |
| `palette.spring.content` | duration 350ms · bounce 0 |  |

### palette.easing
| token | 值 | 备注 |
|---|---|---|
| `palette.easing.easeOut` | cubic-bezier(0,0,0.58,1) |  |
| `palette.easing.standard` | cubic-bezier(0.25,0.1,0.25,1) | **derived** CSS ease |
| `palette.easing.inOut` | cubic-bezier(0.42,0,0.58,1) |  |

### palette.red
| token | 值 | 备注 |
|---|---|---|
| `palette.red.50` | #fdecec | **derived** Apple systemRed 浅底 |
| `palette.red.500` | #ff3b30 | **derived** Apple systemRed |
| `palette.red.700` | #d70015 | **derived** Apple systemRed 文字 |

### palette.shadow
| token | 值 | 备注 |
|---|---|---|
| `palette.shadow.popover` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.12), 0px 2px 6px 0px rgba(0,0,0,0.06), inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | 传输浮层 |
| `palette.shadow.popoverDark` | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 10px 28px 0px rgba(0,0,0,0.45), 0px 2px 6px 0px rgba(0,0,0,0.25) | **derived** 深色传输浮层 |

### palette.accentChoice
| token | 值 | 备注 |
|---|---|---|
| `palette.accentChoice.purple` | #8e5cf7 |  |
| `palette.accentChoice.pink` | #ec4899 |  |
| `palette.accentChoice.orange` | #ff9500 |  |
| `palette.accentChoice.yellow` | #f5b400 |  |

## 7. Semantic（Light / Dark）

### color
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `color.bg.window` | #e7e7e8 | #1c1c1e | 窗口底色（标题栏、rail、卡片间缝隙） |
| `color.surface.card` | #ffffff | #252527 | 内容卡片 / sider / FTP 面板 |
| `color.surface.terminal` | #fbfbfc | #202022 | 终端卡片容器底色（仅容器，不含终端文字配色） |
| `color.surface.raised` | #ffffff | #2e2e31 | 浮起层：拖动卡片、ghost、分段控件选中 |
| `color.surface.sunken` | #f4f4f5 | #2a2a2d | 搜索框 |
| `color.surface.field` | #f6f6f7 | #2a2a2d | 面包屑路径框 |
| `color.surface.zebra` | #fafafb | rgba(255,255,255,0.06) | 列表斑马纹 |
| `color.surface.control` | #f2f2f4 | #2e2e31 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `color.surface.controlStrong` | #ececef | #333336 | sider 行内「更多」按钮 |
| `color.surface.keycap` | #ffffff | #2e2e31 | 键帽 kbd 底 |
| `color.surface.liftStack` | #f9f9fa | #333336 |  |
| `color.state.hover` | #f7f7f8 | #2a2a2d | 行悬停 |
| `color.state.selected` | #efeff1 | #333336 | sider 选中行 |
| `color.state.pressed` | #e6e6e9 | #3a3a3d | **derived** 按下（设计稿未画） |
| `color.state.onWindow` | #d6d6d8 | #3a3a3d | 窗口底色上的激活按钮（rail、标题栏开关） |
| `color.state.selectedAccent` | #e8f2fe | #16304d | 文件列表选中行 |
| `color.state.tabHover` | rgba(255,255,255,0.45) | rgba(255,255,255,0.1) |  |
| `color.state.railHover` | rgba(0,0,0,0.045) | rgba(255,255,255,0.06) |  |
| `color.state.railPressed` | rgba(0,0,0,0.12) | rgba(255,255,255,0.12) |  |
| `color.border.card` | rgba(0,0,0,0.06) | rgba(255,255,255,0.08) | 卡片 0.5px 外描边 |
| `color.border.terminal` | rgba(0,0,0,0.08) | rgba(255,255,255,0.1) | 终端/窗格卡片 0.5px 内描边 |
| `color.border.subtle` | #f0f0f2 | #333336 | 卡片内分隔线（sider footer、状态栏、进度轨道） |
| `color.border.divider` | #f2f2f4 | #333336 | 头部底线、表头底线 |
| `color.border.tab` | #efeff1 | #333336 | 旧版 tab 线 / FTP 面板描边 |
| `color.border.control` | #e6e6e9 | #444448 | 按钮描边、状态栏竖分隔 |
| `color.border.keycap` | #e3e3e6 | #444448 | kbd 描边 |
| `color.border.separatorWindow` | #d2d2d6 | #444448 | 标题栏按钮组分隔线 |
| `color.border.onGlass` | rgba(0,0,0,0.1) | rgba(255,255,255,0.12) | 玻璃栏内竖分隔 |
| `color.border.raised` | rgba(0,0,0,0.08) | rgba(255,255,255,0.1) | 浮起层 0.5px 描边 |
| `color.border.hairlineStrong` | rgba(0,0,0,0.1) | rgba(255,255,255,0.12) | 拖动卡片描边 |
| `color.border.controlHover` | #c7c7cc | #444448 | 输入框悬停描边 |
| `color.text.primary` | #1d1d1f | #f5f5f7 |  |
| `color.text.secondary` | #8e8e93 | #98989d | 次要文字、主机副标题、状态栏 |
| `color.text.tertiary` | #a1a1a6 | #6e6e73 | 关闭 ×、kbd、caption |
| `color.text.quaternary` | #b0b0b5 | #5a5a5f | 分组计数 |
| `color.text.muted` | #6e6e73 | #a1a1a6 | 未选中 tab、rail 图标、按钮图标 |
| `color.text.strong` | #3a3a3f | #d1d1d6 | 次级按钮文字 |
| `color.text.action` | #4a4a50 | #c7c7cc | 标题栏动作按钮 |
| `color.text.neutralBadge` | #5b5b62 | #c7c7cc | 中性徽章文字 |
| `color.text.disabled` | #aeaeb2 | #6e6e73 | 非焦点窗格主机 |
| `color.text.placeholder` | #c7c7cc | #5a5a5f | 面包屑分隔符 |
| `color.text.onAccent` | #ffffff | ＝ |  |
| `color.text.accent` | #0a6fd6 | #4da3ff | 蓝色文字（徽章、提示、高亮分组） |
| `color.text.accentStrong` | #0a5bc4 | #6cb4ff | 选中文件名 |
| `color.icon.default` | #6e6e73 | #a1a1a6 |  |
| `color.icon.subtle` | #8e8e93 | ＝ |  |
| `color.icon.faint` | #a1a1a6 | #6e6e73 |  |
| `color.icon.folder` | #5ba7f7 | ＝ |  |
| `color.icon.folderFill` | #d7e9fd | #16304d |  |
| `color.accent.default` | #0a84ff | ＝ | 主强调色 #0a84ff |
| `color.accent.subtle` | #eaf3fe | #16304d | 浅蓝底（上传、运行中、提示） |
| `color.accent.tint035` | rgba(10,132,255,0.035) | ＝ | 落位区底 |
| `color.accent.tint06` | rgba(10,132,255,0.06) | ＝ | 可落位占位底 |
| `color.accent.tint08` | rgba(10,132,255,0.08) | ＝ | 分组拖入高亮 |
| `color.accent.tint10` | rgba(10,132,255,0.1) | rgba(10,132,255,0.18) | 新输出徽章 / 闪光 |
| `color.accent.tint13` | rgba(10,132,255,0.13) | ＝ | 自动滚动区渐变 |
| `color.accent.tint15` | rgba(10,132,255,0.15) | ＝ | 进度条轨道（自动展开） |
| `color.accent.tint16` | rgba(10,132,255,0.16) | rgba(10,132,255,0.24) | 焦点光晕 |
| `color.accent.tint18` | rgba(10,132,255,0.18) | ＝ | 闪光内描边 |
| `color.accent.tint28` | rgba(10,132,255,0.28) | ＝ | 占位内描边 |
| `color.accent.tint35` | rgba(10,132,255,0.35) | ＝ | 标注描边 |
| `color.accent.tint50` | rgba(10,132,255,0.5) | ＝ | 自动滚动区边线 |
| `color.success.default` | #30c45a | #32d74b | 在线点、完成进度 |
| `color.success.text` | #1f8a3f | #5fd97a |  |
| `color.success.subtle` | #ebf8ef | #173523 |  |
| `color.success.halo` | rgba(48,196,90,0.14) | ＝ | 在线点 3px 光圈 |
| `color.success.onGlass` | rgba(48,196,90,0.15) | ＝ |  |
| `color.warning.default` | #f5a623 | #ffb340 | 连接中 |
| `color.warning.text` | #c7861a | #ffc15c | 连接中文字 / 响铃 |
| `color.warning.textStrong` | #b26a00 | #ffc15c |  |
| `color.warning.subtle` | #fdf3e3 | #3d2e12 |  |
| `color.warning.tint14` | rgba(245,166,35,0.14) | ＝ | 响铃徽章底 |
| `color.warning.halo` | rgba(245,166,35,0.18) | ＝ |  |
| `color.status.offline` | #cfcfd4 | #5a5a5f | 离线点 |
| `color.status.paused` | #c7c7cc | #5a5a5f | 暂停进度 |
| `color.status.scrollbar` | rgba(0,0,0,0.32) | rgba(255,255,255,0.35) |  |
| `color.status.dividerHandle` | #c7c7cc | #5a5a5f |  |
| `color.status.danger` | #dc2626 | #ef4444 |  |
| `color.fill.quiet` | rgba(0,0,0,0.05) | rgba(255,255,255,0.08) | ⌘N 芯片、玻璃上徽章 |
| `color.fill.quieter` | rgba(0,0,0,0.04) | rgba(255,255,255,0.06) | 玻璃上的面包屑 |
| `color.fill.placeholder` | rgba(0,0,0,0.035) | rgba(255,255,255,0.06) | 拖动占位 |
| `color.fill.placeholderEdge` | rgba(0,0,0,0.05) | rgba(255,255,255,0.08) |  |
| `color.fill.strong` | #1d1d1f | #f5f5f7 | 资源条填充 |
| `color.fill.quietHover` | rgba(0,0,0,0.08) | rgba(255,255,255,0.12) | 传输胶囊悬停 |
| `color.fill.skeleton` | #ececef | #444448 |  |
| `color.traffic.close` | #ff5f57 | ＝ |  |
| `color.traffic.minimize` | #febc2e | ＝ |  |
| `color.traffic.zoom` | #28c840 | ＝ |  |
| `color.traffic.edge` | rgba(0,0,0,0.15) | rgba(0,0,0,0.25) |  |
| `color.tooltip.bg` | rgba(30,30,32,0.92) | rgba(58,58,62,0.95) |  |
| `color.tooltip.text` | #ffffff | ＝ |  |
| `color.tooltip.kbd` | rgba(255,255,255,0.6) | ＝ |  |
| `color.glass.titleTab` | rgba(255,255,255,0.85) | rgba(255,255,255,0.12) | 标题栏选中 tab |
| `color.glass.float` | rgba(255,255,255,0.7) | rgba(255,255,255,0.1) | 悬浮胶囊（回到底部） |
| `color.glass.bar` | rgba(255,255,255,0.42) | rgba(255,255,255,0.08) | v4 玻璃栏（无渐隐时） |
| `color.glass.highlight` | rgba(255,255,255,0.9) | rgba(255,255,255,0.1) | 玻璃顶部内高光 |
| `color.danger.default` | #ff3b30 | #ff453a | 失败圆点、失败图标 |
| `color.danger.text` | #d70015 | #ff6961 | 失败文字 |
| `color.danger.subtle` | #fdecec | #3a1d1c | 失败图标底、失败徽章底 |
| `color.danger.tint14` | rgba(255,59,48,0.14) | rgba(255,69,58,0.22) | 错误输入框外光晕 |
| `color.protocol.sftpBg` | #eaf3fe | #16304d | SFTP 图标块底 |
| `color.protocol.sftpGlyph` | #0a84ff | ＝ | SFTP 字形 |
| `color.protocol.ftpBg` | #f2f2f4 | #2e2e31 | FTP 图标块底 |
| `color.protocol.ftpGlyph` | #8e8e93 | #98989d | FTP 字形 |
| `color.overlay.backdrop` | rgba(0,0,0,0.4) | ＝ | 弹窗遮罩 |
| `color.accentChoices.blue` | #0a84ff | ＝ |  |
| `color.accentChoices.purple` | #8e5cf7 | ＝ |  |
| `color.accentChoices.pink` | #ec4899 | ＝ |  |
| `color.accentChoices.red` | #ff3b30 | ＝ |  |
| `color.accentChoices.orange` | #ff9500 | ＝ |  |
| `color.accentChoices.yellow` | #f5b400 | ＝ |  |
| `color.accentChoices.green` | #30c45a | ＝ |  |
| `color.accentChoices.graphite` | #8e8e93 | ＝ |  |
| `color.scrim` | rgba(0,0,0,0.18) | rgba(0,0,0,0.45) | 会话弹窗遮罩，覆盖标题栏 |

### radius
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `radius.window` | 12px | ＝ |  |
| `radius.card` | 10px | ＝ |  |
| `radius.pane` | 10px | ＝ |  |
| `radius.lift` | 9px | ＝ |  |
| `radius.railButton` | 8px | ＝ |  |
| `radius.dropZone` | 8px | ＝ |  |
| `radius.control` | 7px | ＝ |  |
| `radius.controlSm` | 6px | ＝ |  |
| `radius.chip` | 5px | ＝ |  |
| `radius.keycap` | 4px | ＝ |  |
| `radius.bar` | 3px | ＝ |  |
| `radius.pill` | 20px | ＝ |  |
| `radius.badge` | 9px | ＝ |  |
| `radius.hair` | 2px | ＝ |  |
| `radius.tabTop` | 8px | ＝ |  |
| `radius.popover` | 12px | ＝ |  |

### space
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `space.0` | 0px | ＝ |  |
| `space.1` | 1px | ＝ |  |
| `space.2` | 2px | ＝ |  |
| `space.3` | 3px | ＝ |  |
| `space.4` | 4px | ＝ |  |
| `space.5` | 5px | ＝ |  |
| `space.6` | 6px | ＝ |  |
| `space.7` | 7px | ＝ |  |
| `space.8` | 8px | ＝ |  |
| `space.9` | 9px | ＝ |  |
| `space.10` | 10px | ＝ |  |
| `space.12` | 12px | ＝ |  |
| `space.14` | 14px | ＝ |  |
| `space.15` | 15px | ＝ |  |
| `space.16` | 16px | ＝ |  |
| `space.18` | 18px | ＝ |  |
| `space.22` | 22px | ＝ |  |
| `space.24` | 24px | ＝ |  |
| `space.20` | 20px | ＝ |  |

### size
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `size.hairline` | 0.5px | ＝ | 0.5px 描边 |
| `size.divider` | 1px | ＝ |  |
| `size.focusRing` | 1.5px | ＝ | 焦点环宽 / 虚线落位框宽 |
| `size.focusHalo` | 5px | ＝ | 焦点光晕扩散 |
| `size.dot` | 7px | ＝ |  |
| `size.dotSm` | 6px | ＝ |  |
| `size.dotHalo` | 3px | ＝ |  |
| `size.trafficLight` | 12px | ＝ |  |
| `size.icon` | 16px | ＝ |  |
| `size.iconMd` | 15px | ＝ |  |
| `size.iconSm` | 14px | ＝ |  |
| `size.iconStroke` | 1.75px | ＝ | Lucide 风格线宽 |
| `size.controlXs` | 22px | ＝ |  |
| `size.controlSm` | 24px | ＝ |  |
| `size.controlMd` | 26px | ＝ |  |
| `size.control` | 28px | ＝ |  |
| `size.controlLg` | 30px | ＝ |  |
| `size.railButton` | 34px | ＝ |  |
| `size.titlebar` | 44px | ＝ |  |
| `size.rail` | 52px | ＝ |  |
| `size.sider` | 236px | ＝ |  |
| `size.titleLeft` | 296px | ＝ | 标题栏左区（交通灯 + 侧栏开关），与 rail+sider 对齐 |
| `size.paneHeader` | 36px | ＝ |  |
| `size.statusBar` | 34px | ＝ |  |
| `size.paneStatusBar` | 32px | ＝ |  |
| `size.listRow` | 30px | ＝ |  |
| `size.fileHeader` | 40px | ＝ |  |
| `size.queueRow` | 56px | ＝ |  |
| `size.groupRow` | 30px | ＝ |  |
| `size.osLogo` | 26px | ＝ |  |
| `size.osGlyph` | 15px | ＝ |  |
| `size.progress` | 5px | ＝ |  |
| `size.progressThin` | 2px | ＝ |  |
| `size.dropLine` | 2px | ＝ | **derived** 插入线（ListState.Reorder） |
| `size.fadeExtent` | 24px | ＝ | 玻璃栏向内容延伸的渐隐长度 |
| `size.fadeExtentPane` | 22px | ＝ | 分屏窗格内的渐隐长度 |
| `size.statsPanel` | 204px | ＝ | 监控侧栏（分屏时） |
| `size.autoScrollZone` | 48px | ＝ |  |
| `size.separatorShort` | 12px | ＝ |  |
| `size.separatorTall` | 16px | ＝ |  |
| `size.separatorStatus` | 18px | ＝ |  |
| `size.windowWidth` | 1180px | ＝ |  |
| `size.windowHeight` | 780px | ＝ |  |
| `size.windowMinWidth` | 1000px | ＝ |  |
| `size.windowMinHeight` | 640px | ＝ |  |
| `size.trafficLightY` | 17px | ＝ |  |
| `size.siderMinWidth` | 236px | ＝ |  |
| `size.siderMaxWidth` | 320px | ＝ |  |
| `size.paneMinWidth` | 280px | ＝ |  |
| `size.paneMinHeight` | 160px | ＝ |  |
| `size.dragPreviewWidth` | 220px | ＝ |  |
| `size.dragPreviewHeight` | 140px | ＝ |  |
| `size.tabDetachDistance` | 8px | ＝ |  |
| `size.dragGrabX` | 30px | ＝ |  |
| `size.dragGrabY` | 18px | ＝ |  |
| `size.paneEnterOffset` | 12px | ＝ |  |
| `size.dividerHandleWidth` | 3px | ＝ |  |
| `size.dividerHandleHeight` | 28px | ＝ |  |
| `size.dividerKeyboardStep` | 8px | ＝ |  |
| `size.previewInset` | 4px | ＝ |  |
| `size.paneActionRevealPadding` | 40px | ＝ |  |
| `size.hostDialogWidth` | 340px | ＝ |  |
| `size.groupDialogWidth` | 320px | ＝ |  |
| `size.portFieldWidth` | 90px | ＝ |  |
| `size.dialogTitleSize` | 18px | ＝ |  |
| `size.tabContentHeight` | 20px | ＝ |  |
| `size.tabStripHeight` | 36px | ＝ |  |
| `size.tabDragHandleInset` | 30px | ＝ |  |
| `size.tabTitleOffset` | -1.5px | ＝ |  |
| `size.tabCountOffset` | -0.5px | ＝ |  |
| `size.scrollbarMinThumb` | 24px | ＝ |  |
| `size.scrollbarRevealZone` | 24px | ＝ |  |
| `size.terminalContentBottom` | 50px | ＝ |  |
| `size.emptyPaneTitleSize` | 15px | ＝ |  |
| `size.statsHeaderHeight` | 44px | ＝ |  |
| `size.statsChartHeight` | 34px | ＝ |  |
| `size.statsChartStroke` | 1.3px | ＝ |  |
| `size.statsSectionGap` | 20px | ＝ |  |
| `size.emptyPaneIcon` | 32px | ＝ |  |
| `size.bellIcon` | 13px | ＝ |  |
| `size.catalogRowHeight` | 40px | ＝ |  |
| `size.paneMinLocal` | 320px | ＝ |  |
| `size.paneMinRemote` | 420px | ＝ |  |
| `size.popover` | 384px | ＝ |  |
| `size.capsuleBarWidth` | 32px | ＝ |  |
| `size.capsuleBarHeight` | 3px | ＝ |  |
| `size.dockHeader` | 42px | ＝ |  |
| `size.dockRow` | 50px | ＝ |  |
| `size.tabSpinner` | 10px | ＝ |  |
| `size.fileToolbar` | 44px | ＝ |  |
| `size.fileCompactWidth` | 720px | ＝ |  |
| `size.fileSearchWidth` | 160px | ＝ |  |
| `size.fileSizeWidth` | 80px | ＝ |  |
| `size.fileDateWidth` | 140px | ＝ |  |
| `size.filePermissionsWidth` | 100px | ＝ |  |
| `size.fileOwnerWidth` | 90px | ＝ |  |
| `size.fileGridCellWidth` | 120px | ＝ |  |
| `size.fileGridCellHeight` | 100px | ＝ |  |
| `size.transferProgressWidth` | 180px | ＝ |  |
| `size.transferSpeedWidth` | 120px | ＝ |  |
| `size.transferNameMinWidth` | 120px | ＝ |  |
| `size.transferStatusWidth` | 80px | ＝ | **derived** 传输状态列固定宽度 |
| `size.splitGripHeight` | 32px | ＝ |  |
| `size.transferPopoverHeader` | 32px | ＝ |  |
| `size.fileNameMinWidth` | 100px | ＝ |  |
| `size.fileGridIconSize` | 32px | ＝ |  |
| `size.transferDockMaxRows` | 5 | ＝ |  |
| `size.transferPopoverMaxRows` | 6 | ＝ |  |
| `size.transferConcurrency` | 3 | ＝ |  |
| `size.tooltipMaxWidth` | 320px | ＝ | 提示最大宽度 |
| `size.previewLineHeight` | 6px | ＝ |  |
| `size.previewLineLong` | 0.78 | ＝ |  |
| `size.previewLineMedium` | 0.6 | ＝ |  |
| `size.previewLineShort` | 0.45 | ＝ |  |
| `size.modal` | 560px | ＝ |  |
| `size.formLabel` | 72px | ＝ |  |
| `size.settingsContent` | 720px | ＝ |  |
| `size.modalTopRatio` | 0.12 | ＝ |  |
| `size.modalTopMin` | 72px | ＝ |  |
| `size.modalIconTile` | 32px | ＝ |  |
| `size.inputMonoSize` | 12px | ＝ |  |
| `size.formErrorIcon` | 12px | ＝ |  |
| `size.testSpinner` | 13px | ＝ |  |
| `size.collapseChevron` | 12px | ＝ |  |
| `size.collapseChevronStroke` | 2.25px | ＝ |  |
| `size.collapseChevronRotate` | 90px | ＝ |  |
| `size.switchWidth` | 32px | ＝ |  |
| `size.switchHeight` | 19px | ＝ |  |
| `size.switchKnob` | 15px | ＝ |  |
| `size.settingsHeaderPaddingX` | 32px | ＝ |  |
| `size.settingsRowHeight` | 48px | ＝ |  |
| `size.settingsFadeHeight` | 40px | ＝ |  |
| `size.schemePreviewHeight` | 66px | ＝ |  |
| `size.schemePreviewFont` | 10px | ＝ |  |
| `size.schemePreviewLineHeight` | 16px | ＝ |  |
| `size.schemeBadgeIcon` | 10px | ＝ |  |
| `size.themeAspect` | 1.6 | ＝ |  |
| `size.themeRadio` | 14px | ＝ |  |
| `size.themeRadioRing` | 4.5px | ＝ |  |
| `size.accentSwatch` | 20px | ＝ |  |
| `size.accentRing` | 1.5px | ＝ |  |
| `size.accentRingGap` | 2px | ＝ |  |
| `size.credentialModal` | 420px | ＝ |  |
| `size.settingsSelect` | 220px | ＝ |  |
| `size.settingsNumber` | 64px | ＝ |  |
| `size.settingsScrollback` | 96px | ＝ |  |

### font
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `font.family.ui` | -apple-system, SF Pro Text, PingFang SC, Inter, Noto Sans CJK SC, system-ui, sans-serif | ＝ |  |
| `font.family.mono` | JetBrains Mono, Menlo, monospace | ＝ |  |
| `font.size.caption2` | 10.5px | ＝ | ⌘N 芯片、计数胶囊 |
| `font.size.caption` | 11px | ＝ | kbd、状态栏徽章、计数 |
| `font.size.footnote` | 11.5px | ＝ | 分组头、副标题、状态栏 |
| `font.size.small` | 12px | ＝ | sider footer、主机副信息 |
| `font.size.body` | 12.5px | ＝ | tab、按钮、列表 |
| `font.size.bodyLg` | 13px | ＝ | 主机名、标题 |
| `font.size.title` | 15px | ＝ | 大标题（sider-drag 版 sider 标题） |
| `font.weight.regular` | 400 | ＝ |  |
| `font.weight.medium` | 500 | ＝ |  |
| `font.weight.semibold` | 600 | ＝ |  |
| `font.weight.emptyPaneTitle` | 550 | ＝ |  |
| `font.lineHeight.tight` | 14px | ＝ |  |
| `font.lineHeight.sub` | 15px | ＝ |  |
| `font.lineHeight.body` | 16px | ＝ |  |
| `font.lineHeight.name` | 17px | ＝ |  |
| `font.lineHeight.keycap` | 12px | ＝ |  |
| `font.lineHeight.input` | 18px | ＝ | 统一空输入与中文占位文字行高，避免字体回退造成基线偏移 |
| `font.tracking.mono` | -0.2px | ＝ |  |
| `font.tracking.normal` | 0px | ＝ |  |
| `font.tracking.caps` | 0.3px | ＝ |  |
| `font.tracking.inputMono` | -0.1px | ＝ |  |

### shadow
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `shadow.window` | 0px 0px 0px 0.5px rgba(0,0,0,0.18), 0px 24px 60px 0px rgba(0,0,0,0.18), 0px 4px 14px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 24px 60px 0px rgba(0,0,0,0.5), 0px 4px 14px 0px rgba(0,0,0,0.3) |  |
| `shadow.card` | 0px 0px 0px 0.5px rgba(0,0,0,0.06) | 0px 0px 0px 0.5px rgba(255,255,255,0.08) | 卡片外描边 |
| `shadow.paneOutline` | inset 0px 0px 0px 0.5px rgba(0,0,0,0.08) | inset 0px 0px 0px 0.5px rgba(255,255,255,0.1) | 窗格内描边（clip 后叠加） |
| `shadow.titleTab` | 0px 0px 0px 0.5px rgba(0,0,0,0.07), 0px 1px 2px 0px rgba(0,0,0,0.05) | 0px 0px 0px 0.5px rgba(255,255,255,0.1), 0px 1px 2px 0px rgba(0,0,0,0.3) |  |
| `shadow.segment` | 0px 1px 2px 0px rgba(0,0,0,0.08) | ＝ |  |
| `shadow.float` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 4px 12px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 4px 12px 0px rgba(0,0,0,0.3) | 悬浮胶囊 |
| `shadow.tooltip` | 0px 4px 12px 0px rgba(0,0,0,0.18) | 0px 4px 12px 0px rgba(0,0,0,0.4) |  |
| `shadow.ghost` | 0px 8px 24px 0px rgba(0,0,0,0.12), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | 0px 8px 24px 0px rgba(0,0,0,0.45), 0px 0px 0px 0.5px rgba(255,255,255,0.12) | FTP 拖拽 ghost |
| `shadow.floatButton` | 0px 2px 8px 0px rgba(0,0,0,0.1), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | ＝ | FTP 中缝传输按钮 |
| `shadow.lift` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 10px 28px 0px rgba(0,0,0,0.45), 0px 2px 6px 0px rgba(0,0,0,0.25) | 拖动中卡片 |
| `shadow.liftStack` | 0px 0px 0px 0.5px rgba(0,0,0,0.08), 0px 4px 10px 0px rgba(0,0,0,0.06) | ＝ | 多选叠卡 |
| `shadow.focusRing` | 0px 0px 0px 1.5px #0a84ff | ＝ | 焦点窗格：仅 1.5px 蓝色描边 |
| `shadow.hoverRing` | 0px 0px 0px 1px rgba(0,0,0,0.14) | 0px 0px 0px 1px rgba(255,255,255,0.2) | 非焦点窗格悬停 |
| `shadow.flash` | inset 0px 0px 0px 1px rgba(10,132,255,0.18) | ＝ | 落位闪光内描边 |
| `shadow.placeholder` | inset 0px 0px 0px 1px rgba(0,0,0,0.05) | inset 0px 0px 0px 1px rgba(255,255,255,0.08) |  |
| `shadow.placeholderAccent` | inset 0px 0px 0px 1px rgba(10,132,255,0.28) | ＝ |  |
| `shadow.osSelected` | 0px 0px 0px 0.5px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.12) |  [deprecated: 侧栏不再显示状态/选中] |
| `shadow.glassHighlight` | inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | inset 0px 0.5px 0px 0px rgba(255,255,255,0.1) |  |
| `shadow.dotOnline` | 0px 0px 0px 3px rgba(48,196,90,0.14) | ＝ |  [deprecated: 侧栏不再显示状态/选中] |
| `shadow.dotConnecting` | 0px 0px 0px 3px rgba(245,166,35,0.18) | ＝ |  [deprecated: 侧栏不再显示状态/选中] |
| `shadow.popover` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.12), 0px 2px 6px 0px rgba(0,0,0,0.06), inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 10px 28px 0px rgba(0,0,0,0.45), 0px 2px 6px 0px rgba(0,0,0,0.25) | 传输浮层 |
| `shadow.dialog` | 0px 10px 30px 0px rgba(0,0,0,0.3) | 0px 10px 30px 0px rgba(0,0,0,0.5) | 弹窗阴影 |
| `shadow.modal` | 0px 0px 0px 0.5px rgba(0,0,0,0.12), 0px 24px 64px 0px rgba(0,0,0,0.22), 0px 4px 16px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.12), 0px 24px 64px 0px rgba(0,0,0,0.55), 0px 4px 16px 0px rgba(0,0,0,0.3) | 会话弹窗 |
| `shadow.inputFocus` | inset 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 3px rgba(10,132,255,0.16) | inset 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 3px rgba(10,132,255,0.24) | 输入框焦点：内环与外光晕 |
| `shadow.inputError` | inset 0px 0px 0px 1.5px #ff3b30, 0px 0px 0px 3px rgba(255,59,48,0.14) | inset 0px 0px 0px 1.5px #ff453a, 0px 0px 0px 3px rgba(255,69,58,0.22) | 输入框错误：优先于焦点 |
| `shadow.switchKnob` | 0px 1px 2px 0px rgba(0,0,0,0.2) | ＝ | 开关滑块 |
| `shadow.themeSelected` | 0px 0px 0px 3px rgba(10,132,255,0.16) | 0px 0px 0px 3px rgba(10,132,255,0.24) | 外观主题卡片选中光晕 |

### glass
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `glass.fade.blur1` | 1.5px | ＝ |  |
| `glass.fade.blur2` | 4px | ＝ |  |
| `glass.fade.blur3` | 9px | ＝ |  |
| `glass.fade.mask1Solid` | 0.45 | ＝ | 层 1 实心到 45%，100% 透明 |
| `glass.fade.mask1End` | 1 | ＝ |  |
| `glass.fade.mask2Solid` | 0.4 | ＝ |  |
| `glass.fade.mask2End` | 0.78 | ＝ |  |
| `glass.fade.mask3Solid` | 0.3 | ＝ |  |
| `glass.fade.mask3End` | 0.6 | ＝ |  |
| `glass.fade.tintStart` | 0.92 | ＝ | 着色渐变起点 alpha（取所在卡片底色） |
| `glass.fade.tintMid` | 0.78 | ＝ |  |
| `glass.fade.tintMidStop` | 0.45 | ＝ |  |
| `glass.fade.tintEnd` | 0 | ＝ |  |
| `glass.float.blur` | 16px | ＝ |  |
| `glass.float.saturate` | 1.5 | ＝ |  |
| `glass.bar.blur` | 5px | ＝ |  |
| `glass.bar.saturate` | 1.8 | ＝ |  |
| `glass.paneActionBlur` | 4px | ＝ |  |
| `glass.paneRevealBlur` | 8px | ＝ |  |

### opacity
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `opacity.paneUnfocusedContent` | 0.5 | 0.45 |  |
| `opacity.paneUnfocusedSaturate` | 0.35 | ＝ |  |
| `opacity.paneUnfocusedStatus` | 0.55 | ＝ |  |
| `opacity.paneHoverContent` | 0.78 | ＝ |  |
| `opacity.paneHoverSaturate` | 0.7 | ＝ |  |
| `opacity.dragSource` | 0.4 | ＝ | 拖走后源行（折叠时） |
| `opacity.liftStackBack` | 0.8 | ＝ |  |
| `opacity.hidden` | 0 | ＝ |  |
| `opacity.osTint` | 0.09 | 0.18 | OS logo 底 = 品牌色 9% 混入卡片色 |
| `opacity.dragRotation` | -1 | ＝ |  |
| `opacity.dragPreview` | 0.9 | ＝ |  |
| `opacity.previewHeader` | 0.96 | ＝ |  |
| `opacity.paneCloseScale` | 0.97 | ＝ |  |
| `opacity.dropEdgeRatio` | 0.3 | ＝ |  |
| `opacity.paneActionTint` | 0.9 | ＝ |  |
| `opacity.revealFrom` | 0.6 | ＝ | 本地面板展开起始透明度 |
| `opacity.capsuleEnterScale` | 0.9 | ＝ |  |
| `opacity.popoverEnterScale` | 0.96 | ＝ |  |
| `opacity.splitDefaultRatio` | 0.5 | ＝ |  |
| `opacity.disabled` | 0.4 | ＝ |  |
| `opacity.modalEnterScale` | 0.96 | ＝ |  |
| `opacity.modalExitScale` | 0.98 | ＝ |  |
| `opacity.collapseContentFadeFactor` | 1.6 | ＝ |  |

### motion
| token | Light | Dark（derived） | 用途 |
|---|---|---|---|
| `motion.duration.instant` | 0ms | ＝ |  |
| `motion.duration.lift` | 120ms | ＝ | 拿起 |
| `motion.duration.hold` | 150ms | ＝ | 长按拿起 |
| `motion.duration.focus` | 160ms | ＝ | 焦点/悬停切换 |
| `motion.duration.collapse` | 180ms | ＝ | 分组拖动时全部收起 |
| `motion.duration.restore` | 220ms | ＝ | 恢复展开 |
| `motion.duration.cancel` | 300ms | ＝ | 取消飞回 |
| `motion.duration.drop` | 320ms | ＝ | 落位 |
| `motion.duration.autoExpand` | 500ms | ＝ | 悬停自动展开 |
| `motion.duration.flash` | 600ms | ＝ | 落位闪光 |
| `motion.duration.frameInterval` | 16ms | ＝ |  |
| `motion.duration.zoom` | 240ms | ＝ |  |
| `motion.duration.paneEnter` | 220ms | ＝ |  |
| `motion.duration.paneClose` | 160ms | ＝ |  |
| `motion.duration.dividerFade` | 120ms | ＝ |  |
| `motion.duration.sidebar` | 220ms | ＝ |  |
| `motion.duration.scrollbarHold` | 700ms | ＝ |  |
| `motion.duration.scrollbarFade` | 180ms | ＝ |  |
| `motion.duration.instantNonzero` | 1e-06ms | ＝ | 1 ns: MyGo zero Animation selects a default spring |
| `motion.duration.exit` | 120ms | ＝ | 浮层退出、胶囊淡出 |
| `motion.duration.spin` | 800ms | ＝ | 连接中转圈一周 |
| `motion.duration.paneReveal` | 220ms | ＝ |  |
| `motion.duration.connectionStage` | 300ms | ＝ | SSH 演示连接每个阶段的停留时间 |
| `motion.duration.test` | 10000ms | ＝ |  |
| `motion.duration.settingsSwitchOut` | 80ms | ＝ |  |
| `motion.easing.out` | cubic-bezier(0,0,0.58,1) | ＝ |  |
| `motion.easing.pane` | cubic-bezier(0.2,0.8,0.2,1) | ＝ |  |
| `motion.easing.search` | cubic-bezier(0.42,0,0.58,1) | ＝ |  |
| `motion.easing.in` | cubic-bezier(0.42,0,1,1) | ＝ |  |
| `motion.spring.displace` | stiffness 420 · damping 34 | ＝ | 让位（≈ Snappy, 220ms） |
| `motion.spring.drop` | duration 320ms · bounce 0.15 | ＝ | 落位 |
| `motion.spring.smooth` | duration 300ms · bounce 0 | ＝ | 取消 / 平滑（ui.Smooth） |
| `motion.spring.zoom` | duration 240ms · bounce 0 | ＝ |  |
| `motion.spring.content` | duration 350ms · bounce 0 | ＝ | 数字内容变化 |
| `motion.drag.threshold` | 4px | ＝ | 移动阈值 px |
| `motion.drag.liftScale` | 1.03 | ＝ |  |
| `motion.drag.autoScrollMax` | 900 | ＝ | px/s，t² 曲线 |
| `motion.drag.autoScrollZone` | 48px | ＝ |  |
| `motion.demoTransferSpeed` | 1.30023e+07 | ＝ | **derived** 演示传输吞吐 |

## 8. Component

只列引用和 Light 下解析出的值；Dark 下自动跟随对应的 semantic。

### window
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `window.bg` | `{color.bg.window}` | #e7e7e8 | 窗口底色（标题栏、rail、卡片间缝隙） |
| `window.radius` | `{radius.window}` | 12px |  |
| `window.shadow` | `{shadow.window}` | 0px 0px 0px 0.5px rgba(0,0,0,0.18), 0px 24px 60px 0px rgba(0,0,0,0.18), 0px 4px 14px 0px rgba(0,0,0,0.08) |  |
| `window.bodyPaddingRight` | `{space.8}` | 8px |  |
| `window.bodyPaddingBottom` | `{space.8}` | 8px |  |
| `window.gutter` | `{space.8}` | 8px |  |

### titlebar
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `titlebar.height` | `{size.titlebar}` | 44px |  |
| `titlebar.leftWidth` | `{size.titleLeft}` | 296px | 标题栏左区（交通灯 + 侧栏开关），与 rail+sider 对齐 |
| `titlebar.leftPaddingX` | `{space.16}` | 16px |  |
| `titlebar.leftGap` | `{space.8}` | 8px |  |
| `titlebar.trafficSize` | `{size.trafficLight}` | 12px |  |
| `titlebar.trafficClose` | `{color.traffic.close}` | #ff5f57 |  |
| `titlebar.trafficMinimize` | `{color.traffic.minimize}` | #febc2e |  |
| `titlebar.trafficZoom` | `{color.traffic.zoom}` | #28c840 |  |
| `titlebar.trafficEdge` | `{color.traffic.edge}` | rgba(0,0,0,0.15) |  |
| `titlebar.toggleIcon` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `titlebar.tabsGap` | `{space.2}` | 2px |  |
| `titlebar.actionsGap` | `{space.3}` | 3px |  |
| `titlebar.actionsPaddingRight` | `{space.10}` | 10px |  |

### tab
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `tab.height` | `{size.control}` | 28px |  |
| `tab.radius` | `{radius.control}` | 7px |  |
| `tab.paddingLeft` | `{space.10}` | 10px |  |
| `tab.paddingRight` | `{space.8}` | 8px |  |
| `tab.gap` | `{space.7}` | 7px |  |
| `tab.fontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `tab.text` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `tab.textSelected` | `{color.text.primary}` | #1d1d1f |  |
| `tab.weightSelected` | `{font.weight.medium}` | 500 |  |
| `tab.bgSelected` | `{color.glass.titleTab}` | rgba(255,255,255,0.85) | 标题栏选中 tab |
| `tab.shadowSelected` | `{shadow.titleTab}` | 0px 0px 0px 0.5px rgba(0,0,0,0.07), 0px 1px 2px 0px rgba(0,0,0,0.05) |  |
| `tab.close` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `tab.dotSize` | `{size.dotSm}` | 6px |  |
| `tab.addIcon` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `tab.contentHeight` | `{size.tabContentHeight}` | 20px |  |
| `tab.stripHeight` | `{size.tabStripHeight}` | 36px |  |
| `tab.dragHandleInset` | `{size.tabDragHandleInset}` | 30px |  |
| `tab.titleOffset` | `{size.tabTitleOffset}` | -1.5px |  |
| `tab.countOffset` | `{size.tabCountOffset}` | -0.5px |  |
| `tab.bgHover` | `{color.state.tabHover}` | rgba(255,255,255,0.45) |  |
| `tab.dotConnected` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `tab.dotError` | `{color.danger.default}` | #ff3b30 | 失败圆点、失败图标 |
| `tab.ringDisconnected` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `tab.ringWidth` | `{size.focusRing}` | 1.5px | 焦点环宽 / 虚线落位框宽 |
| `tab.spinner` | `{color.warning.default}` | #f5a623 | 连接中 |
| `tab.spinnerTrack` | `{color.warning.halo}` | rgba(245,166,35,0.18) |  |
| `tab.spinnerSize` | `{size.tabSpinner}` | 10px |  |
| `tab.spinnerStroke` | `{size.iconStroke}` | 1.75px | Lucide 风格线宽 |
| `tab.spinnerPeriod` | `{motion.duration.spin}` | 800ms | 连接中转圈一周 |
| `tab.textDisconnected` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `tab.textErrorSelected` | `{color.danger.text}` | #d70015 | 失败文字 |
| `tab.stateTransition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### titleAction
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `titleAction.size` | `{size.control}` | 28px |  |
| `titleAction.radius` | `{radius.control}` | 7px |  |
| `titleAction.icon` | `{color.text.action}` | #4a4a50 | 标题栏动作按钮 |
| `titleAction.iconSize` | `{size.iconMd}` | 15px |  |
| `titleAction.bgOn` | `{color.state.onWindow}` | #d6d6d8 | 窗口底色上的激活按钮（rail、标题栏开关） |
| `titleAction.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `titleAction.bgPrimary` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `titleAction.iconPrimary` | `{color.text.onAccent}` | #ffffff |  |
| `titleAction.separator` | `{color.border.separatorWindow}` | #d2d2d6 | 标题栏按钮组分隔线 |
| `titleAction.separatorHeight` | `{size.separatorTall}` | 16px |  |
| `titleAction.separatorMarginX` | `{space.6}` | 6px |  |

### rail
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `rail.width` | `{size.rail}` | 52px |  |
| `rail.gap` | `{space.6}` | 6px |  |
| `rail.paddingTop` | `{space.4}` | 4px |  |
| `rail.paddingBottom` | `{space.10}` | 10px |  |
| `rail.buttonSize` | `{size.railButton}` | 34px |  |
| `rail.buttonRadius` | `{radius.railButton}` | 8px |  |
| `rail.icon` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `rail.bgOn` | `{color.state.onWindow}` | #d6d6d8 | 窗口底色上的激活按钮（rail、标题栏开关） |
| `rail.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `rail.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### sider
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `sider.width` | `{size.sider}` | 236px |  |
| `sider.bg` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `sider.radius` | `{radius.card}` | 10px |  |
| `sider.shadow` | `{shadow.card}` | 0px 0px 0px 0.5px rgba(0,0,0,0.06) | 卡片外描边 |
| `sider.marginRight` | `{space.8}` | 8px |  |
| `sider.headerPaddingTop` | `{space.16}` | 16px |  |
| `sider.headerPaddingRight` | `{space.12}` | 12px |  |
| `sider.headerPaddingBottom` | `{space.10}` | 10px |  |
| `sider.headerPaddingLeft` | `{space.16}` | 16px |  |
| `sider.titleSize` | `{font.size.bodyLg}` | 13px | 主机名、标题 |
| `sider.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `sider.addButtonSize` | `{size.controlMd}` | 26px |  |
| `sider.addButtonRadius` | `{radius.controlSm}` | 6px |  |
| `sider.addButtonBg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `sider.addButtonIcon` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `sider.listPaddingX` | `{space.8}` | 8px |  |
| `sider.listPaddingY` | `{space.2}` | 2px |  |
| `sider.footerBorder` | `{color.border.subtle}` | #f0f0f2 | 卡片内分隔线（sider footer、状态栏、进度轨道） |
| `sider.footerPaddingX` | `{space.16}` | 16px |  |
| `sider.footerPaddingY` | `{space.10}` | 10px |  |
| `sider.footerText` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `sider.footerSize` | `{font.size.small}` | 12px | sider footer、主机副信息 |

### search
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `search.height` | `{size.controlLg}` | 30px |  |
| `search.radius` | `{radius.control}` | 7px |  |
| `search.bg` | `{color.surface.sunken}` | #f4f4f5 | 搜索框 |
| `search.paddingX` | `{space.9}` | 9px |  |
| `search.gap` | `{space.6}` | 6px |  |
| `search.marginX` | `{space.12}` | 12px |  |
| `search.marginBottom` | `{space.8}` | 8px |  |
| `search.text` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `search.fontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `search.kbdSize` | `{font.size.caption}` | 11px | kbd、状态栏徽章、计数 |
| `search.kbdText` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `search.kbdBorder` | `{color.border.keycap}` | #e3e3e6 | kbd 描边 |
| `search.kbdBg` | `{color.surface.keycap}` | #ffffff | 键帽 kbd 底 |
| `search.kbdRadius` | `{radius.keycap}` | 4px |  |
| `search.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `search.easing` | `{motion.easing.search}` | cubic-bezier(0.42,0,0.58,1) |  |
| `search.lineHeight` | `{font.lineHeight.input}` | 18px | 统一空输入与中文占位文字行高，避免字体回退造成基线偏移 |

### groupHeader
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `groupHeader.height` | `{size.groupRow}` | 30px |  |
| `groupHeader.radius` | `{radius.control}` | 7px |  |
| `groupHeader.paddingX` | `{space.8}` | 8px |  |
| `groupHeader.paddingTop` | `{space.12}` | 12px |  |
| `groupHeader.paddingBottom` | `{space.5}` | 5px |  |
| `groupHeader.gap` | `{space.4}` | 4px |  |
| `groupHeader.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `groupHeader.weight` | `{font.weight.medium}` | 500 |  |
| `groupHeader.text` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `groupHeader.count` | `{color.text.quaternary}` | #b0b0b5 | 分组计数 |
| `groupHeader.countSize` | `{font.size.caption}` | 11px | kbd、状态栏徽章、计数 |
| `groupHeader.dropBg` | `{color.accent.tint08}` | rgba(10,132,255,0.08) | 分组拖入高亮 |
| `groupHeader.dropText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `groupHeader.dropCount` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `groupHeader.expandTrack` | `{color.accent.tint15}` | rgba(10,132,255,0.15) | 进度条轨道（自动展开） |
| `groupHeader.expandFill` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `groupHeader.expandHeight` | `{size.progressThin}` | 2px |  |
| `groupHeader.expandDelay` | `{motion.duration.autoExpand}` | 500ms | 悬停自动展开 |
| `groupHeader.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `groupHeader.countMotion` | `{motion.spring.content}` | duration 350ms · bounce 0 | 数字内容变化 |

### siderItem
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `siderItem.paddingTop` | `{space.6}` | 6px |  |
| `siderItem.paddingRight` | `{space.6}` | 6px |  |
| `siderItem.paddingBottom` | `{space.6}` | 6px |  |
| `siderItem.paddingLeft` | `{space.8}` | 8px |  |
| `siderItem.gap` | `{space.9}` | 9px |  |
| `siderItem.radius` | `{radius.control}` | 7px |  |
| `siderItem.spacing` | `{space.1}` | 1px |  |
| `siderItem.bgHover` | `{color.state.hover}` | #f7f7f8 | 行悬停 |
| `siderItem.bgSelected` | `{color.state.selected}` | #efeff1 | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.nameSize` | `{font.size.bodyLg}` | 13px | 主机名、标题 |
| `siderItem.nameWeight` | `{font.weight.medium}` | 500 |  |
| `siderItem.nameLineHeight` | `{font.lineHeight.name}` | 17px |  |
| `siderItem.nameColor` | `{color.text.primary}` | #1d1d1f |  |
| `siderItem.subSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `siderItem.subFamily` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `siderItem.subLineHeight` | `{font.lineHeight.sub}` | 15px |  |
| `siderItem.subTracking` | `{font.tracking.mono}` | -0.2px |  |
| `siderItem.subColor` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `siderItem.dotSize` | `{size.dot}` | 7px | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.dotHalo` | `{size.dotHalo}` | 3px | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.dotOnline` | `{color.success.default}` | #30c45a | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.dotOnlineHalo` | `{color.success.halo}` | rgba(48,196,90,0.14) | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.dotOffline` | `{color.status.offline}` | #cfcfd4 | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.dotConnecting` | `{color.warning.default}` | #f5a623 | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.dotConnectingHalo` | `{color.warning.halo}` | rgba(245,166,35,0.18) | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.connectingText` | `{color.warning.text}` | #c7861a | **deprecated** 侧栏不再显示状态/选中 |
| `siderItem.moreSize` | `{size.controlSm}` | 24px |  |
| `siderItem.moreRadius` | `{radius.controlSm}` | 6px |  |
| `siderItem.moreBg` | `{color.surface.controlStrong}` | #ececef | sider 行内「更多」按钮 |
| `siderItem.moreIcon` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `siderItem.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### osLogo
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `osLogo.size` | `{size.osLogo}` | 26px |  |
| `osLogo.radius` | `{radius.control}` | 7px |  |
| `osLogo.glyphSize` | `{size.osGlyph}` | 15px |  |
| `osLogo.tintMix` | `{opacity.osTint}` | 0.09 | OS logo 底 = 品牌色 9% 混入卡片色 |
| `osLogo.bgBase` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `osLogo.bgSelected` | `{color.surface.card}` | #ffffff | **deprecated** 侧栏不再显示状态/选中 |
| `osLogo.shadowSelected` | `{shadow.osSelected}` | 0px 0px 0px 0.5px rgba(0,0,0,0.08) | **deprecated** 侧栏不再显示状态/选中 |

### card
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `card.bg` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `card.radius` | `{radius.card}` | 10px |  |
| `card.shadow` | `{shadow.card}` | 0px 0px 0px 0.5px rgba(0,0,0,0.06) | 卡片外描边 |

### pane
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `pane.bg` | `{color.surface.terminal}` | #fbfbfc | 终端卡片容器底色（仅容器，不含终端文字配色） |
| `pane.radius` | `{radius.pane}` | 10px |  |
| `pane.outline` | `{shadow.paneOutline}` | inset 0px 0px 0px 0.5px rgba(0,0,0,0.08) | 窗格内描边（clip 后叠加） |
| `pane.gap` | `{space.8}` | 8px |  |
| `pane.headerHeight` | `{size.paneHeader}` | 36px |  |
| `pane.headerPaddingLeft` | `{space.14}` | 14px |  |
| `pane.headerPaddingRight` | `{space.8}` | 8px |  |
| `pane.headerGap` | `{space.7}` | 7px |  |
| `pane.headerFontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `pane.name` | `{color.text.primary}` | #1d1d1f |  |
| `pane.nameWeight` | `{font.weight.semibold}` | 600 |  |
| `pane.host` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `pane.hostSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `pane.hostFamily` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `pane.buttonSize` | `{size.controlSm}` | 24px |  |
| `pane.buttonIcon` | `{color.icon.subtle}` | #8e8e93 |  |
| `pane.iconSize` | `{size.iconSm}` | 14px |  |
| `pane.frameInterval` | `{motion.duration.frameInterval}` | 16ms |  |
| `pane.emptyIconSize` | `{size.emptyPaneIcon}` | 32px |  |
| `pane.emptyTitleSize` | `{size.emptyPaneTitleSize}` | 15px |  |
| `pane.emptyTitleWeight` | `{font.weight.emptyPaneTitle}` | 550 |  |
| `pane.contentTop` | `{space.10}` | 10px |  |
| `pane.contentSide` | `{space.14}` | 14px |  |
| `pane.contentBottom` | `{size.terminalContentBottom}` | 50px |  |
| `pane.actionTop` | `{space.6}` | 6px |  |
| `pane.actionRight` | `{space.8}` | 8px |  |
| `pane.actionWidth` | `{size.controlXs}` | 22px |  |
| `pane.actionHeight` | `{size.controlSm}` | 24px |  |
| `pane.actionGap` | `{space.2}` | 2px |  |
| `pane.actionRevealPadding` | `{size.paneActionRevealPadding}` | 40px |  |
| `pane.actionRadius` | `{radius.control}` | 7px |  |
| `pane.actionBlur` | `{glass.paneActionBlur}` | 4px |  |
| `pane.actionTint` | `{opacity.paneActionTint}` | 0.9 |  |

### paneFocus
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `paneFocus.ringFocused` | `{shadow.focusRing}` | 0px 0px 0px 1.5px #0a84ff | 焦点窗格：仅 1.5px 蓝色描边 |
| `paneFocus.ringHover` | `{shadow.hoverRing}` | 0px 0px 0px 1px rgba(0,0,0,0.14) | 非焦点窗格悬停 |
| `paneFocus.nameFocused` | `{color.text.primary}` | #1d1d1f |  |
| `paneFocus.nameWeightFocused` | `{font.weight.semibold}` | 600 |  |
| `paneFocus.nameUnfocused` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `paneFocus.nameWeightUnfocused` | `{font.weight.medium}` | 500 |  |
| `paneFocus.hostUnfocused` | `{color.text.disabled}` | #aeaeb2 | 非焦点窗格主机 |
| `paneFocus.contentOpacityUnfocused` | `{opacity.paneUnfocusedContent}` | 0.5 |  |
| `paneFocus.contentSaturateUnfocused` | `{opacity.paneUnfocusedSaturate}` | 0.35 |  |
| `paneFocus.statusOpacityUnfocused` | `{opacity.paneUnfocusedStatus}` | 0.55 |  |
| `paneFocus.contentOpacityHover` | `{opacity.paneHoverContent}` | 0.78 |  |
| `paneFocus.contentSaturateHover` | `{opacity.paneHoverSaturate}` | 0.7 |  |
| `paneFocus.buttonsOpacityUnfocused` | `{opacity.hidden}` | 0 |  |
| `paneFocus.cursorRingUnfocused` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `paneFocus.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `paneFocus.kbdSize` | `{font.size.caption2}` | 10.5px | ⌘N 芯片、计数胶囊 |
| `paneFocus.kbdWeight` | `{font.weight.medium}` | 500 |  |
| `paneFocus.kbdRadius` | `{radius.chip}` | 5px |  |
| `paneFocus.kbdPaddingX` | `{space.5}` | 5px |  |
| `paneFocus.kbdPaddingY` | `{space.3}` | 3px |  |
| `paneFocus.kbdBg` | `{color.fill.quiet}` | rgba(0,0,0,0.05) | ⌘N 芯片、玻璃上徽章 |
| `paneFocus.kbdText` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `paneFocus.kbdBgFocused` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `paneFocus.kbdTextFocused` | `{color.text.onAccent}` | #ffffff |  |
| `paneFocus.kbdLineHeight` | `{font.lineHeight.keycap}` | 12px |  |

### activityBadge
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `activityBadge.fontSize` | `{font.size.caption}` | 11px | kbd、状态栏徽章、计数 |
| `activityBadge.radius` | `{radius.badge}` | 9px |  |
| `activityBadge.paddingLeft` | `{space.6}` | 6px |  |
| `activityBadge.paddingRight` | `{space.7}` | 7px |  |
| `activityBadge.paddingY` | `{space.2}` | 2px |  |
| `activityBadge.gap` | `{space.4}` | 4px |  |
| `activityBadge.dotSize` | `{size.dotSm}` | 6px |  |
| `activityBadge.outputText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `activityBadge.outputBg` | `{color.accent.tint10}` | rgba(10,132,255,0.1) | 新输出徽章 / 闪光 |
| `activityBadge.outputDot` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `activityBadge.bellText` | `{color.warning.text}` | #c7861a | 连接中文字 / 响铃 |
| `activityBadge.bellBg` | `{color.warning.tint14}` | rgba(245,166,35,0.14) | 响铃徽章底 |
| `activityBadge.bellIconSize` | `{size.bellIcon}` | 13px |  |

### fadeBar
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `fadeBar.tint` | `{color.surface.terminal}` | #fbfbfc | 终端卡片容器底色（仅容器，不含终端文字配色） |
| `fadeBar.extent` | `{size.fadeExtent}` | 24px | 玻璃栏向内容延伸的渐隐长度 |
| `fadeBar.extentPane` | `{size.fadeExtentPane}` | 22px | 分屏窗格内的渐隐长度 |
| `fadeBar.blur1` | `{glass.fade.blur1}` | 1.5px |  |
| `fadeBar.blur2` | `{glass.fade.blur2}` | 4px |  |
| `fadeBar.blur3` | `{glass.fade.blur3}` | 9px |  |
| `fadeBar.mask1Solid` | `{glass.fade.mask1Solid}` | 0.45 | 层 1 实心到 45%，100% 透明 |
| `fadeBar.mask1End` | `{glass.fade.mask1End}` | 1 |  |
| `fadeBar.mask2Solid` | `{glass.fade.mask2Solid}` | 0.4 |  |
| `fadeBar.mask2End` | `{glass.fade.mask2End}` | 0.78 |  |
| `fadeBar.mask3Solid` | `{glass.fade.mask3Solid}` | 0.3 |  |
| `fadeBar.mask3End` | `{glass.fade.mask3End}` | 0.6 |  |
| `fadeBar.tintStart` | `{glass.fade.tintStart}` | 0.92 | 着色渐变起点 alpha（取所在卡片底色） |
| `fadeBar.tintMid` | `{glass.fade.tintMid}` | 0.78 |  |
| `fadeBar.tintMidStop` | `{glass.fade.tintMidStop}` | 0.45 |  |
| `fadeBar.tintEnd` | `{glass.fade.tintEnd}` | 0 |  |

### statusBar
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `statusBar.height` | `{size.statusBar}` | 34px |  |
| `statusBar.heightPane` | `{size.paneStatusBar}` | 32px |  |
| `statusBar.paddingX` | `{space.14}` | 14px |  |
| `statusBar.gap` | `{space.14}` | 14px |  |
| `statusBar.gapPane` | `{space.10}` | 10px |  |
| `statusBar.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `statusBar.text` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `statusBar.hostFamily` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `statusBar.hostColor` | `{color.text.primary}` | #1d1d1f |  |
| `statusBar.hostWeight` | `{font.weight.semibold}` | 600 |  |
| `statusBar.divider` | `{color.border.control}` | #e6e6e9 | 按钮描边、状态栏竖分隔 |
| `statusBar.dividerOnGlass` | `{color.border.onGlass}` | rgba(0,0,0,0.1) | 玻璃栏内竖分隔 |
| `statusBar.dividerHeight` | `{size.separatorShort}` | 12px |  |

### badge
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `badge.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `badge.fontSizeCompact` | `{font.size.caption}` | 11px | kbd、状态栏徽章、计数 |
| `badge.paddingX` | `{space.8}` | 8px |  |
| `badge.paddingXCompact` | `{space.7}` | 7px |  |
| `badge.paddingY` | `{space.2}` | 2px |  |
| `badge.paddingYCompact` | `{space.1}` | 1px |  |
| `badge.radius` | `{radius.pill}` | 20px |  |
| `badge.gap` | `{space.5}` | 5px |  |
| `badge.neutralBg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `badge.neutralText` | `{color.text.neutralBadge}` | #5b5b62 | 中性徽章文字 |
| `badge.neutralBgOnGlass` | `{color.fill.quiet}` | rgba(0,0,0,0.05) | ⌘N 芯片、玻璃上徽章 |
| `badge.okBg` | `{color.success.subtle}` | #ebf8ef |  |
| `badge.okText` | `{color.success.text}` | #1f8a3f |  |
| `badge.okDot` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `badge.okBgOnGlass` | `{color.success.onGlass}` | rgba(48,196,90,0.15) |  |
| `badge.runBg` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `badge.runText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |

### button
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `button.height` | `{size.control}` | 28px |  |
| `button.radius` | `{radius.control}` | 7px |  |
| `button.paddingX` | `{space.9}` | 9px |  |
| `button.gap` | `{space.6}` | 6px |  |
| `button.fontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `button.text` | `{color.text.strong}` | #3a3a3f | 次级按钮文字 |
| `button.border` | `{color.border.control}` | #e6e6e9 | 按钮描边、状态栏竖分隔 |
| `button.borderWidth` | `{size.divider}` | 1px |  |
| `button.primaryBg` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `button.primaryText` | `{color.text.onAccent}` | #ffffff |  |
| `button.bgHover` | `{color.state.hover}` | #f7f7f8 | **derived** 未画 |
| `button.bgPressed` | `{color.state.pressed}` | #e6e6e9 | **derived** 未画 |
| `button.bg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `button.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### iconButton
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `iconButton.size` | `{size.controlMd}` | 26px |  |
| `iconButton.sizeSm` | `{size.controlSm}` | 24px |  |
| `iconButton.sizeXs` | `{size.controlXs}` | 22px |  |
| `iconButton.radius` | `{radius.controlSm}` | 6px |  |
| `iconButton.icon` | `{color.icon.default}` | #6e6e73 |  |
| `iconButton.iconSubtle` | `{color.icon.subtle}` | #8e8e93 |  |
| `iconButton.iconFaint` | `{color.icon.faint}` | #a1a1a6 |  |
| `iconButton.bg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `iconButton.bgHover` | `{color.state.hover}` | #f7f7f8 | **derived** 未画 |
| `iconButton.iconSize` | `{size.icon}` | 16px |  |
| `iconButton.stroke` | `{size.iconStroke}` | 1.75px | Lucide 风格线宽 |
| `iconButton.bgPressed` | `{color.state.pressed}` | #e6e6e9 | 按下（设计稿未画） |
| `iconButton.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### floatButton
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `floatButton.size` | `{size.controlLg}` | 30px |  |
| `floatButton.bg` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `floatButton.shadow` | `{shadow.floatButton}` | 0px 2px 8px 0px rgba(0,0,0,0.1), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | FTP 中缝传输按钮 |
| `floatButton.icon` | `{color.text.strong}` | #3a3a3f | 次级按钮文字 |
| `floatButton.primaryBg` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `floatButton.primaryIcon` | `{color.text.onAccent}` | #ffffff |  |

### segmented
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `segmented.bg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `segmented.radius` | `{radius.control}` | 7px |  |
| `segmented.padding` | `{space.2}` | 2px |  |
| `segmented.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `segmented.itemPaddingX` | `{space.9}` | 9px |  |
| `segmented.itemPaddingY` | `{space.2}` | 2px |  |
| `segmented.itemRadius` | `{radius.chip}` | 5px |  |
| `segmented.text` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `segmented.bgOn` | `{color.surface.raised}` | #ffffff | 浮起层：拖动卡片、ghost、分段控件选中 |
| `segmented.textOn` | `{color.text.primary}` | #1d1d1f |  |
| `segmented.shadowOn` | `{shadow.segment}` | 0px 1px 2px 0px rgba(0,0,0,0.08) |  |
| `segmented.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `segmented.bgHover` | `{color.state.hover}` | #f7f7f8 | 行悬停 |

### breadcrumb
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `breadcrumb.height` | `{size.controlMd}` | 26px |  |
| `breadcrumb.radius` | `{radius.controlSm}` | 6px |  |
| `breadcrumb.bg` | `{color.surface.field}` | #f6f6f7 | 面包屑路径框 |
| `breadcrumb.bgOnGlass` | `{color.fill.quieter}` | rgba(0,0,0,0.04) | 玻璃上的面包屑 |
| `breadcrumb.paddingX` | `{space.8}` | 8px |  |
| `breadcrumb.gap` | `{space.3}` | 3px |  |
| `breadcrumb.text` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `breadcrumb.current` | `{color.text.primary}` | #1d1d1f |  |
| `breadcrumb.separator` | `{color.text.placeholder}` | #c7c7cc | 面包屑分隔符 |

### fileList
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `fileList.headerHeight` | `{size.listRow}` | 30px |  |
| `fileList.headerText` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `fileList.headerSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `fileList.headerBorder` | `{color.border.divider}` | #f2f2f4 | 头部底线、表头底线 |
| `fileList.rowHeight` | `{size.listRow}` | 30px |  |
| `fileList.rowRadius` | `{radius.controlSm}` | 6px |  |
| `fileList.rowPaddingX` | `{space.8}` | 8px |  |
| `fileList.rowGap` | `{space.12}` | 12px |  |
| `fileList.fontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `fileList.zebra` | `{color.surface.zebra}` | #fafafb | 列表斑马纹 |
| `fileList.bgSelected` | `{color.state.selectedAccent}` | #e8f2fe | 文件列表选中行 |
| `fileList.textSelected` | `{color.text.accentStrong}` | #0a5bc4 | 选中文件名 |
| `fileList.meta` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `fileList.fileIcon` | `{color.icon.subtle}` | #8e8e93 |  |
| `fileList.folderIcon` | `{color.icon.folder}` | #5ba7f7 |  |
| `fileList.folderFill` | `{color.icon.folderFill}` | #d7e9fd |  |
| `fileList.paneHeaderHeight` | `{size.fileHeader}` | 40px |  |

### progress
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `progress.height` | `{size.progress}` | 5px |  |
| `progress.radius` | `{radius.bar}` | 3px |  |
| `progress.track` | `{color.border.subtle}` | #f0f0f2 | 卡片内分隔线（sider footer、状态栏、进度轨道） |
| `progress.fill` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `progress.fillNeutral` | `{color.fill.strong}` | #1d1d1f | 资源条填充 |
| `progress.fillPaused` | `{color.status.paused}` | #c7c7cc | 暂停进度 |
| `progress.fillDone` | `{color.success.default}` | #30c45a | 在线点、完成进度 |

### tooltip
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `tooltip.bg` | `{color.tooltip.bg}` | rgba(30,30,32,0.92) |  |
| `tooltip.text` | `{color.tooltip.text}` | #ffffff |  |
| `tooltip.kbd` | `{color.tooltip.kbd}` | rgba(255,255,255,0.6) |  |
| `tooltip.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `tooltip.paddingX` | `{space.8}` | 8px |  |
| `tooltip.paddingY` | `{space.4}` | 4px |  |
| `tooltip.radius` | `{radius.controlSm}` | 6px |  |
| `tooltip.shadow` | `{shadow.tooltip}` | 0px 4px 12px 0px rgba(0,0,0,0.18) |  |
| `tooltip.arrow` | `{space.4}` | 4px |  |
| `tooltip.maxWidth` | `{size.tooltipMaxWidth}` | 320px | 提示最大宽度 |
| `tooltip.offset` | `{space.6}` | 6px |  |
| `tooltip.railOffset` | `{space.8}` | 8px |  |

### menu
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `menu.bg` | `{color.surface.raised}` | #ffffff | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.radius` | `{radius.lift}` | 9px | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.shadow` | `{shadow.lift}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.itemHeight` | `{size.control}` | 28px | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.itemRadius` | `{radius.controlSm}` | 6px | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.itemPaddingX` | `{space.8}` | 8px | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.itemHover` | `{color.accent.default}` | #0a84ff | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.itemHoverText` | `{color.text.onAccent}` | #ffffff | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.fontSize` | `{font.size.body}` | 12.5px | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |
| `menu.separator` | `{color.border.subtle}` | #f0f0f2 | **derived** 设计稿未画菜单，按浮起层/拖动卡片推导 |

### floatPill
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `floatPill.bg` | `{color.glass.float}` | rgba(255,255,255,0.7) | 悬浮胶囊（回到底部） |
| `floatPill.blur` | `{glass.float.blur}` | 16px |  |
| `floatPill.saturate` | `{glass.float.saturate}` | 1.5 |  |
| `floatPill.shadow` | `{shadow.float}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 4px 12px 0px rgba(0,0,0,0.08) | 悬浮胶囊 |
| `floatPill.radius` | `{radius.pill}` | 20px |  |
| `floatPill.text` | `{color.text.strong}` | #3a3a3f | 次级按钮文字 |
| `floatPill.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `floatPill.paddingLeft` | `{space.8}` | 8px |  |
| `floatPill.paddingRight` | `{space.10}` | 10px |  |
| `floatPill.paddingY` | `{space.4}` | 4px |  |

### dragLift
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `dragLift.threshold` | `{motion.drag.threshold}` | 4px | 移动阈值 px |
| `dragLift.holdDelay` | `{motion.duration.hold}` | 150ms | 长按拿起 |
| `dragLift.scale` | `{motion.drag.liftScale}` | 1.03 |  |
| `dragLift.bg` | `{color.surface.raised}` | #ffffff | 浮起层：拖动卡片、ghost、分段控件选中 |
| `dragLift.radius` | `{radius.lift}` | 9px |  |
| `dragLift.shadow` | `{shadow.lift}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | 拖动中卡片 |
| `dragLift.stackShadow` | `{shadow.liftStack}` | 0px 0px 0px 0.5px rgba(0,0,0,0.08), 0px 4px 10px 0px rgba(0,0,0,0.06) | 多选叠卡 |
| `dragLift.stackOffset` | `{space.5}` | 5px |  |
| `dragLift.stackInset` | `{space.6}` | 6px |  |
| `dragLift.stackBackOpacity` | `{opacity.liftStackBack}` | 0.8 |  |
| `dragLift.liftDuration` | `{motion.duration.lift}` | 120ms | 拿起 |
| `dragLift.sourceOpacity` | `{opacity.dragSource}` | 0.4 | 拖走后源行（折叠时） |
| `dragLift.stackBg` | `{color.surface.liftStack}` | #f9f9fa |  |

### dropIndicator
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `dropIndicator.placeholderBg` | `{color.fill.placeholder}` | rgba(0,0,0,0.035) | 拖动占位 |
| `dropIndicator.placeholderShadow` | `{shadow.placeholder}` | inset 0px 0px 0px 1px rgba(0,0,0,0.05) |  |
| `dropIndicator.placeholderAccentBg` | `{color.accent.tint06}` | rgba(10,132,255,0.06) | 可落位占位底 |
| `dropIndicator.placeholderAccentShadow` | `{shadow.placeholderAccent}` | inset 0px 0px 0px 1px rgba(10,132,255,0.28) |  |
| `dropIndicator.lineColor` | `{color.accent.default}` | #0a84ff | **derived** MyGo ListState.Reorder 插入线 |
| `dropIndicator.lineHeight` | `{size.dropLine}` | 2px | **derived** 同上 |
| `dropIndicator.zoneBorder` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `dropIndicator.zoneBorderWidth` | `{size.focusRing}` | 1.5px | 焦点环宽 / 虚线落位框宽 |
| `dropIndicator.zoneBg` | `{color.accent.tint035}` | rgba(10,132,255,0.035) | 落位区底 |
| `dropIndicator.zoneRadius` | `{radius.dropZone}` | 8px |  |
| `dropIndicator.hintBg` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `dropIndicator.hintText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `dropIndicator.flashBg` | `{color.accent.tint10}` | rgba(10,132,255,0.1) | 新输出徽章 / 闪光 |
| `dropIndicator.flashShadow` | `{shadow.flash}` | inset 0px 0px 0px 1px rgba(10,132,255,0.18) | 落位闪光内描边 |
| `dropIndicator.flashDuration` | `{motion.duration.flash}` | 600ms | 落位闪光 |
| `dropIndicator.flashEasing` | `{motion.easing.out}` | cubic-bezier(0,0,0.58,1) |  |
| `dropIndicator.autoScrollZone` | `{motion.drag.autoScrollZone}` | 48px |  |
| `dropIndicator.autoScrollMax` | `{motion.drag.autoScrollMax}` | 900 | px/s，t² 曲线 |
| `dropIndicator.autoScrollTint` | `{color.accent.tint13}` | rgba(10,132,255,0.13) | 自动滚动区渐变 |
| `dropIndicator.autoScrollEdge` | `{color.accent.tint50}` | rgba(10,132,255,0.5) | 自动滚动区边线 |

### dragGhost
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `dragGhost.bg` | `{color.surface.raised}` | #ffffff | 浮起层：拖动卡片、ghost、分段控件选中 |
| `dragGhost.radius` | `{radius.dropZone}` | 8px |  |
| `dragGhost.shadow` | `{shadow.ghost}` | 0px 8px 24px 0px rgba(0,0,0,0.12), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | FTP 拖拽 ghost |
| `dragGhost.paddingLeft` | `{space.10}` | 10px |  |
| `dragGhost.paddingRight` | `{space.12}` | 12px |  |
| `dragGhost.paddingY` | `{space.7}` | 7px |  |
| `dragGhost.fontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `dragGhost.countBg` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `dragGhost.countText` | `{color.text.onAccent}` | #ffffff |  |
| `dragGhost.countSize` | `{font.size.caption2}` | 10.5px | ⌘N 芯片、计数胶囊 |
| `dragGhost.width` | `{size.dragPreviewWidth}` | 220px |  |
| `dragGhost.height` | `{size.dragPreviewHeight}` | 140px |  |
| `dragGhost.headerHeight` | `{size.control}` | 28px |  |
| `dragGhost.lineHeight` | `{size.previewLineHeight}` | 6px |  |
| `dragGhost.lineRadius` | `{radius.bar}` | 3px |  |
| `dragGhost.lineColor` | `{color.fill.skeleton}` | #ececef |  |
| `dragGhost.lineLong` | `{size.previewLineLong}` | 0.78 |  |
| `dragGhost.lineMedium` | `{size.previewLineMedium}` | 0.6 |  |
| `dragGhost.lineShort` | `{size.previewLineShort}` | 0.45 |  |

### reorderMotion
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `reorderMotion.displace` | `{motion.spring.displace}` | stiffness 420 · damping 34 | 让位（≈ Snappy, 220ms） |
| `reorderMotion.drop` | `{motion.spring.drop}` | duration 320ms · bounce 0.15 | 落位 |
| `reorderMotion.cancel` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `reorderMotion.groupCollapse` | `{motion.duration.collapse}` | 180ms | 分组拖动时全部收起 |
| `reorderMotion.groupRestore` | `{motion.duration.restore}` | 220ms | 恢复展开 |
| `reorderMotion.tabDisplace` | `{motion.spring.displace}` | stiffness 420 · damping 34 | **derived** tab/窗格拖动未单独设计，沿用 sider 拖动参数 |
| `reorderMotion.paneDisplace` | `{motion.spring.displace}` | stiffness 420 · damping 34 | **derived** 同上 |
| `reorderMotion.paneDrop` | `{motion.spring.drop}` | duration 320ms · bounce 0.15 | **derived** 同上 |

### desktop
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `desktop.windowWidth` | `{size.windowWidth}` | 1180px |  |
| `desktop.windowHeight` | `{size.windowHeight}` | 780px |  |
| `desktop.windowMinWidth` | `{size.windowMinWidth}` | 1000px |  |
| `desktop.windowMinHeight` | `{size.windowMinHeight}` | 640px |  |
| `desktop.trafficLightY` | `{size.trafficLightY}` | 17px |  |
| `desktop.siderMinWidth` | `{size.siderMinWidth}` | 236px |  |
| `desktop.siderMaxWidth` | `{size.siderMaxWidth}` | 320px |  |
| `desktop.paneMinWidth` | `{size.paneMinWidth}` | 280px |  |
| `desktop.paneMinHeight` | `{size.paneMinHeight}` | 160px |  |
| `desktop.dragPreviewWidth` | `{size.dragPreviewWidth}` | 220px |  |
| `desktop.dragPreviewHeight` | `{size.dragPreviewHeight}` | 140px |  |
| `desktop.tabDetachDistance` | `{size.tabDetachDistance}` | 8px |  |
| `desktop.dragGrabX` | `{size.dragGrabX}` | 30px |  |
| `desktop.dragGrabY` | `{size.dragGrabY}` | 18px |  |
| `desktop.paneEnterOffset` | `{size.paneEnterOffset}` | 12px |  |
| `desktop.dividerHandleWidth` | `{size.dividerHandleWidth}` | 3px |  |
| `desktop.dividerHandleHeight` | `{size.dividerHandleHeight}` | 28px |  |
| `desktop.dividerKeyboardStep` | `{size.dividerKeyboardStep}` | 8px |  |
| `desktop.previewInset` | `{size.previewInset}` | 4px |  |
| `desktop.dragRotation` | `{opacity.dragRotation}` | -1 |  |
| `desktop.dragOpacity` | `{opacity.dragPreview}` | 0.9 |  |
| `desktop.previewHeaderOpacity` | `{opacity.previewHeader}` | 0.96 |  |
| `desktop.paneCloseScale` | `{opacity.paneCloseScale}` | 0.97 |  |
| `desktop.dropEdgeRatio` | `{opacity.dropEdgeRatio}` | 0.3 |  |
| `desktop.frameInterval` | `{motion.duration.frameInterval}` | 16ms |  |
| `desktop.zoomDuration` | `{motion.duration.zoom}` | 240ms |  |
| `desktop.paneEnterDuration` | `{motion.duration.paneEnter}` | 220ms |  |
| `desktop.paneCloseDuration` | `{motion.duration.paneClose}` | 160ms |  |
| `desktop.dividerFadeDuration` | `{motion.duration.dividerFade}` | 120ms |  |
| `desktop.sidebarDuration` | `{motion.duration.sidebar}` | 220ms |  |
| `desktop.instantDuration` | `{motion.duration.instantNonzero}` | 1e-06ms | 1 ns: MyGo zero Animation selects a default spring |
| `desktop.zoomSpring` | `{motion.spring.zoom}` | duration 240ms · bounce 0 |  |
| `desktop.motionEasing` | `{motion.easing.pane}` | cubic-bezier(0.2,0.8,0.2,1) |  |
| `desktop.dividerHandleColor` | `{color.status.dividerHandle}` | #c7c7cc |  |
| `desktop.dividerHandleRadius` | `{size.focusRing}` | 1.5px | 焦点环宽 / 虚线落位框宽 |
| `desktop.trafficLightX` | `{space.12}` | 12px |  |
| `desktop.siderKeyboardStep` | `{space.10}` | 10px |  |
| `desktop.railHover` | `{color.state.railHover}` | rgba(0,0,0,0.045) |  |
| `desktop.railPressed` | `{color.state.railPressed}` | rgba(0,0,0,0.12) |  |
| `desktop.danger` | `{color.status.danger}` | #dc2626 |  |

### scrollbar
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `scrollbar.holdDuration` | `{motion.duration.scrollbarHold}` | 700ms |  |
| `scrollbar.fadeDuration` | `{motion.duration.scrollbarFade}` | 180ms |  |
| `scrollbar.minThumb` | `{size.scrollbarMinThumb}` | 24px |  |
| `scrollbar.trackWidth` | `{space.12}` | 12px |  |
| `scrollbar.thumbWidth` | `{space.6}` | 6px |  |
| `scrollbar.thumbInset` | `{space.3}` | 3px |  |
| `scrollbar.endInset` | `{space.2}` | 2px |  |
| `scrollbar.revealZone` | `{size.scrollbarRevealZone}` | 24px |  |
| `scrollbar.color` | `{color.status.scrollbar}` | rgba(0,0,0,0.32) |  |

### dialog
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `dialog.hostWidth` | `{size.hostDialogWidth}` | 340px |  |
| `dialog.groupWidth` | `{size.groupDialogWidth}` | 320px |  |
| `dialog.portWidth` | `{size.portFieldWidth}` | 90px |  |
| `dialog.titleSize` | `{size.dialogTitleSize}` | 18px |  |
| `dialog.bg` | `{color.surface.raised}` | #ffffff | 浮起层：拖动卡片、ghost、分段控件选中 |
| `dialog.backdrop` | `{color.overlay.backdrop}` | rgba(0,0,0,0.4) | 弹窗遮罩 |
| `dialog.shadow` | `{shadow.dialog}` | 0px 10px 30px 0px rgba(0,0,0,0.3) | 弹窗阴影 |
| `dialog.radius` | `{radius.card}` | 10px |  |
| `dialog.padding` | `{space.20}` | 20px |  |
| `dialog.gap` | `{space.12}` | 12px |  |
| `dialog.viewportInset` | `{space.20}` | 20px |  |

### stats
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `stats.headerHeight` | `{size.statsHeaderHeight}` | 44px |  |
| `stats.chartHeight` | `{size.statsChartHeight}` | 34px |  |
| `stats.chartStroke` | `{size.statsChartStroke}` | 1.3px |  |
| `stats.sectionGap` | `{size.statsSectionGap}` | 20px |  |

### catalog
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `catalog.rowHeight` | `{size.catalogRowHeight}` | 40px |  |
| `catalog.listPaddingX` | `{space.8}` | 8px |  |
| `catalog.listPaddingY` | `{space.2}` | 2px |  |
| `catalog.rowGap` | `{space.2}` | 2px |  |
| `catalog.groupGap` | `{space.10}` | 10px |  |

### protocolIcon
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `protocolIcon.size` | `{size.osLogo}` | 26px |  |
| `protocolIcon.radius` | `{radius.control}` | 7px |  |
| `protocolIcon.glyph` | `{size.osGlyph}` | 15px |  |
| `protocolIcon.sftpBg` | `{color.protocol.sftpBg}` | #eaf3fe | SFTP 图标块底 |
| `protocolIcon.sftpGlyph` | `{color.protocol.sftpGlyph}` | #0a84ff | SFTP 字形 |
| `protocolIcon.ftpBg` | `{color.protocol.ftpBg}` | #f2f2f4 | FTP 图标块底 |
| `protocolIcon.ftpGlyph` | `{color.protocol.ftpGlyph}` | #8e8e93 | FTP 字形 |

### connectionIndicator
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `connectionIndicator.offline` | `{color.status.offline}` | #cfcfd4 | 离线点 |
| `connectionIndicator.connected` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `connectionIndicator.connectedHalo` | `{color.success.halo}` | rgba(48,196,90,0.14) | 在线点 3px 光圈 |
| `connectionIndicator.connecting` | `{color.warning.default}` | #f5a623 | 连接中 |
| `connectionIndicator.connectingHalo` | `{color.warning.halo}` | rgba(245,166,35,0.18) |  |
| `connectionIndicator.halo` | `{size.dotHalo}` | 3px |  |
| `connectionIndicator.stageDuration` | `{motion.duration.connectionStage}` | 300ms | SSH 演示连接每个阶段的停留时间 |

### paneToggle
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `paneToggle.size` | `{size.controlMd}` | 26px |  |
| `paneToggle.radius` | `{radius.controlSm}` | 6px |  |
| `paneToggle.icon` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `paneToggle.bgHover` | `{color.state.hover}` | #f7f7f8 | 行悬停 |
| `paneToggle.bgOn` | `{color.state.pressed}` | #e6e6e9 | 按下（设计稿未画） |
| `paneToggle.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `paneToggle.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### filePaneFocus
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `filePaneFocus.labelBgActive` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `filePaneFocus.labelTextActive` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `filePaneFocus.labelWeightActive` | `{font.weight.semibold}` | 600 |  |
| `filePaneFocus.labelTextInactive` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `filePaneFocus.labelWeightInactive` | `{font.weight.medium}` | 500 |  |
| `filePaneFocus.labelPaddingX` | `{space.8}` | 8px |  |
| `filePaneFocus.labelRadius` | `{radius.controlSm}` | 6px |  |
| `filePaneFocus.selBgActive` | `{color.state.selectedAccent}` | #e8f2fe | 文件列表选中行 |
| `filePaneFocus.selTextActive` | `{color.text.accentStrong}` | #0a5bc4 | 选中文件名 |
| `filePaneFocus.selBgInactive` | `{color.state.selected}` | #efeff1 | sider 选中行 |
| `filePaneFocus.selTextInactive` | `{color.text.primary}` | #1d1d1f |  |
| `filePaneFocus.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### FTPSplit
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `FTPSplit.gutter` | `{space.8}` | 8px |  |
| `FTPSplit.expand` | `{motion.spring.displace}` | stiffness 420 · damping 34 | 让位（≈ Snappy, 220ms） |
| `FTPSplit.revealOpacityFrom` | `{opacity.revealFrom}` | 0.6 | 本地面板展开起始透明度 |
| `FTPSplit.crossfade` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `FTPSplit.minLocal` | `{size.paneMinLocal}` | 320px |  |
| `FTPSplit.minRemote` | `{size.paneMinRemote}` | 420px |  |
| `FTPSplit.defaultRatio` | `{opacity.splitDefaultRatio}` | 0.5 |  |
| `FTPSplit.gripW` | `{space.4}` | 4px |  |
| `FTPSplit.gripH` | `{size.splitGripHeight}` | 32px |  |
| `FTPSplit.grip` | `{color.text.placeholder}` | #c7c7cc | 面包屑分隔符 |
| `FTPSplit.gripActive` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |

### transferCapsule
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `transferCapsule.height` | `{size.controlXs}` | 22px |  |
| `transferCapsule.radius` | `{radius.pill}` | 20px |  |
| `transferCapsule.bg` | `{color.fill.quiet}` | rgba(0,0,0,0.05) | ⌘N 芯片、玻璃上徽章 |
| `transferCapsule.bgHover` | `{color.fill.quietHover}` | rgba(0,0,0,0.08) | 传输胶囊悬停 |
| `transferCapsule.bgOpen` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `transferCapsule.textOpen` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `transferCapsule.text` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `transferCapsule.arrow` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `transferCapsule.barWidth` | `{size.capsuleBarWidth}` | 32px |  |
| `transferCapsule.barHeight` | `{size.capsuleBarHeight}` | 3px |  |
| `transferCapsule.barTrack` | `{color.accent.tint15}` | rgba(10,132,255,0.15) | 进度条轨道（自动展开） |
| `transferCapsule.barFill` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `transferCapsule.doneArrow` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `transferCapsule.doneFill` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `transferCapsule.error` | `{color.danger.text}` | #d70015 | 失败文字 |
| `transferCapsule.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `transferCapsule.exit` | `{motion.duration.exit}` | 120ms | 浮层退出、胶囊淡出 |
| `transferCapsule.enterScale` | `{opacity.capsuleEnterScale}` | 0.9 |  |
| `transferCapsule.paddingLeft` | `{space.7}` | 7px |  |
| `transferCapsule.paddingRight` | `{space.9}` | 9px |  |
| `transferCapsule.gap` | `{space.6}` | 6px |  |
| `transferCapsule.paddingX` | `{space.8}` | 8px |  |
| `transferCapsule.paddingY` | `{space.2}` | 2px |  |
| `transferCapsule.fontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `transferCapsule.weight` | `{font.weight.medium}` | 500 |  |
| `transferCapsule.speedInterval` | `{motion.duration.autoExpand}` | 500ms | 悬停自动展开 |

### transferPopover
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `transferPopover.width` | `{size.popover}` | 384px |  |
| `transferPopover.radius` | `{radius.popover}` | 12px |  |
| `transferPopover.bg` | `{color.glass.float}` | rgba(255,255,255,0.7) | 悬浮胶囊（回到底部） |
| `transferPopover.blur` | `{glass.float.blur}` | 16px |  |
| `transferPopover.saturate` | `{glass.float.saturate}` | 1.5 |  |
| `transferPopover.shadow` | `{shadow.popover}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.12), 0px 2px 6px 0px rgba(0,0,0,0.06), inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | 传输浮层 |
| `transferPopover.offset` | `{space.8}` | 8px |  |
| `transferPopover.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `transferPopover.exit` | `{motion.duration.exit}` | 120ms | 浮层退出、胶囊淡出 |
| `transferPopover.enterScale` | `{opacity.popoverEnterScale}` | 0.96 |  |
| `transferPopover.enterY` | `{space.6}` | 6px |  |
| `transferPopover.padding` | `{space.6}` | 6px |  |
| `transferPopover.headerHeight` | `{size.transferPopoverHeader}` | 32px |  |
| `transferPopover.maxRows` | `{size.transferPopoverMaxRows}` | 6 |  |
| `transferPopover.dismissY` | `{space.24}` | 24px |  |
| `transferPopover.rowHeight` | `{size.queueRow}` | 56px |  |
| `transferPopover.exitY` | `{space.24}` | 24px |  |

### transferDock
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `transferDock.bg` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `transferDock.radius` | `{radius.card}` | 10px |  |
| `transferDock.headerHeight` | `{size.dockHeader}` | 42px |  |
| `transferDock.rowHeight` | `{size.dockRow}` | 50px |  |
| `transferDock.maxRows` | `{size.transferDockMaxRows}` | 5 |  |
| `transferDock.grow` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `transferDock.collapse` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `transferDock.concurrency` | `{size.transferConcurrency}` | 3 |  |
| `transferDock.progressWidth` | `{size.transferProgressWidth}` | 180px |  |
| `transferDock.compactBreakpoint` | `{size.fileCompactWidth}` | 720px |  |

### transferRow
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `transferRow.iconTile` | `{size.control}` | 28px |  |
| `transferRow.iconRadius` | `{radius.control}` | 7px |  |
| `transferRow.upBg` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `transferRow.upIcon` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `transferRow.downBg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `transferRow.downIcon` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `transferRow.doneBg` | `{color.success.subtle}` | #ebf8ef |  |
| `transferRow.doneIcon` | `{color.success.text}` | #1f8a3f |  |
| `transferRow.errBg` | `{color.danger.subtle}` | #fdecec | 失败图标底、失败徽章底 |
| `transferRow.errIcon` | `{color.danger.text}` | #d70015 | 失败文字 |
| `transferRow.speedText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `transferRow.errorText` | `{color.danger.text}` | #d70015 | 失败文字 |
| `transferRow.runBg` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `transferRow.runText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `transferRow.runDot` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `transferRow.pausedBg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `transferRow.pausedText` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `transferRow.pausedDot` | `{color.status.paused}` | #c7c7cc | 暂停进度 |
| `transferRow.doneBadgeBg` | `{color.success.subtle}` | #ebf8ef |  |
| `transferRow.doneBadgeText` | `{color.success.text}` | #1f8a3f |  |
| `transferRow.doneDot` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `transferRow.errBadgeBg` | `{color.danger.subtle}` | #fdecec | 失败图标底、失败徽章底 |
| `transferRow.errBadgeText` | `{color.danger.text}` | #d70015 | 失败文字 |
| `transferRow.errDot` | `{color.danger.default}` | #ff3b30 | 失败圆点、失败图标 |
| `transferRow.progressWidth` | `{size.transferProgressWidth}` | 180px | **derived** FTP v3 停靠传输进度列宽 |
| `transferRow.speedWidth` | `{size.transferSpeedWidth}` | 120px | **derived** FTP v3 停靠传输速度列宽 |
| `transferRow.statusWidth` | `{size.transferStatusWidth}` | 80px | **derived** FTP v3 停靠传输状态列固定宽度 |
| `transferRow.nameMinWidth` | `{size.transferNameMinWidth}` | 120px | **derived** FTP v3 传输名称最小宽度 |
| `transferRow.nameSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `transferRow.pathSize` | `{font.size.caption}` | 11px | kbd、状态栏徽章、计数 |
| `transferRow.rowHeight` | `{size.queueRow}` | 56px |  |
| `transferRow.detailSize` | `{font.size.caption}` | 11px | kbd、状态栏徽章、计数 |
| `transferRow.progressHeight` | `{size.progress}` | 5px |  |
| `transferRow.gap` | `{space.8}` | 8px |  |
| `transferRow.paddingX` | `{space.12}` | 12px |  |
| `transferRow.demoBytesPerSecond` | `{motion.demoTransferSpeed}` | 1.30023e+07 | 演示传输吞吐 |
| `transferRow.speedInterval` | `{motion.duration.autoExpand}` | 500ms | 悬停自动展开 |
| `transferRow.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### filePane
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `filePane.headerHeight` | `{size.fileToolbar}` | 44px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.statusHeight` | `{size.statusBar}` | 34px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.compactWidth` | `{size.fileCompactWidth}` | 720px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.searchWidth` | `{size.fileSearchWidth}` | 160px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.sizeWidth` | `{size.fileSizeWidth}` | 80px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.dateWidth` | `{size.fileDateWidth}` | 140px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.permissionsWidth` | `{size.filePermissionsWidth}` | 100px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.ownerWidth` | `{size.fileOwnerWidth}` | 90px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.gridCellWidth` | `{size.fileGridCellWidth}` | 120px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.gridCellHeight` | `{size.fileGridCellHeight}` | 100px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.rowInset` | `{space.8}` | 8px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.toolGap` | `{space.4}` | 4px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.contentPadding` | `{space.8}` | 8px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.nameMinWidth` | `{size.fileNameMinWidth}` | 100px | **derived** FTP v3 原生文件面板布局补全 |
| `filePane.gridIconSize` | `{size.fileGridIconSize}` | 32px | **derived** FTP v3 原生文件面板布局补全 |

### input
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `input.height` | `{size.control}` | 28px |  |
| `input.paddingX` | `{space.9}` | 9px |  |
| `input.radius` | `{radius.control}` | 7px |  |
| `input.bg` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `input.text` | `{color.text.primary}` | #1d1d1f |  |
| `input.fontSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `input.border` | `{color.border.control}` | #e6e6e9 | 按钮描边、状态栏竖分隔 |
| `input.borderHover` | `{color.border.separatorWindow}` | #d2d2d6 | 标题栏按钮组分隔线 |
| `input.borderFocus` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `input.borderWidth` | `{size.divider}` | 1px |  |
| `input.transition` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `input.gap` | `{space.7}` | 7px |  |
| `input.focusShadow` | `{shadow.inputFocus}` | inset 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 3px rgba(10,132,255,0.16) | 输入框焦点：内环与外光晕 |
| `input.errorShadow` | `{shadow.inputError}` | inset 0px 0px 0px 1.5px #ff3b30, 0px 0px 0px 3px rgba(255,59,48,0.14) | 输入框错误：优先于焦点 |
| `input.placeholder` | `{color.text.placeholder}` | #c7c7cc | 面包屑分隔符 |
| `input.icon` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `input.monoFont` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `input.monoSize` | `{size.inputMonoSize}` | 12px |  |
| `input.monoTracking` | `{font.tracking.inputMono}` | -0.1px |  |
| `input.disabledBg` | `{color.surface.field}` | #f6f6f7 | 面包屑路径框 |
| `input.disabledText` | `{color.text.disabled}` | #aeaeb2 | 非焦点窗格主机 |
| `input.disabledBorder` | `{color.border.subtle}` | #f0f0f2 | 卡片内分隔线（sider footer、状态栏、进度轨道） |
| `input.caret` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `input.chipHeight` | `{size.controlXs}` | 22px |  |
| `input.chipRadius` | `{radius.chip}` | 5px |  |
| `input.chipBg` | `{color.surface.control}` | #f2f2f4 | 中性控件底：图标按钮 bg、分段控件、中性徽章、队列图标 |
| `input.chipFontSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `input.selectWidth` | `{size.transferProgressWidth}` | 180px |  |
| `input.portWidth` | `{size.formLabel}` | 72px |  |
| `input.numberWidth` | `{size.formLabel}` | 72px |  |
| `input.lineHeight` | `{font.lineHeight.input}` | 18px | 统一空输入与中文占位文字行高，避免字体回退造成基线偏移 |

### paneReveal
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `paneReveal.blur` | `{glass.paneRevealBlur}` | 8px |  |
| `paneReveal.duration` | `{motion.duration.paneReveal}` | 220ms |  |
| `paneReveal.easing` | `{motion.easing.out}` | cubic-bezier(0,0,0.58,1) |  |

### modal
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `modal.width` | `{size.modal}` | 560px |  |
| `modal.radius` | `{radius.window}` | 12px |  |
| `modal.bg` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
| `modal.shadow` | `{shadow.modal}` | 0px 0px 0px 0.5px rgba(0,0,0,0.12), 0px 24px 64px 0px rgba(0,0,0,0.22), 0px 4px 16px 0px rgba(0,0,0,0.08) | 会话弹窗 |
| `modal.scrim` | `{color.scrim}` | rgba(0,0,0,0.18) | 会话弹窗遮罩，覆盖标题栏 |
| `modal.topRatio` | `{size.modalTopRatio}` | 0.12 |  |
| `modal.topMin` | `{size.modalTopMin}` | 72px |  |
| `modal.bottomMargin` | `{space.24}` | 24px |  |
| `modal.headerPaddingTop` | `{space.18}` | 18px |  |
| `modal.headerPaddingRight` | `{space.16}` | 16px |  |
| `modal.headerPaddingBottom` | `{space.14}` | 14px |  |
| `modal.headerPaddingLeft` | `{space.24}` | 24px |  |
| `modal.headerGap` | `{space.12}` | 12px |  |
| `modal.iconTile` | `{size.modalIconTile}` | 32px |  |
| `modal.iconTileRadius` | `{radius.railButton}` | 8px |  |
| `modal.titleSize` | `{font.size.title}` | 15px | 大标题（sider-drag 版 sider 标题） |
| `modal.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `modal.subtitleSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `modal.subtitle` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `modal.bodyPaddingX` | `{space.24}` | 24px |  |
| `modal.divider` | `{color.border.divider}` | #f2f2f4 | 头部底线、表头底线 |
| `modal.footerPaddingTop` | `{space.14}` | 14px |  |
| `modal.footerPaddingRight` | `{space.16}` | 16px |  |
| `modal.footerPaddingBottom` | `{space.16}` | 16px |  |
| `modal.footerPaddingLeft` | `{space.24}` | 24px |  |
| `modal.footerGap` | `{space.8}` | 8px |  |
| `modal.buttonPaddingX` | `{space.12}` | 12px |  |
| `modal.enterScale` | `{opacity.modalEnterScale}` | 0.96 |  |
| `modal.exitScale` | `{opacity.modalExitScale}` | 0.98 |  |
| `modal.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `modal.fade` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `modal.exit` | `{motion.duration.exit}` | 120ms | 浮层退出、胶囊淡出 |
| `modal.reduceFade` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `modal.generateWidth` | `{size.credentialModal}` | 420px |  |
| `modal.protocolRadius` | `{radius.lift}` | 9px |  |
| `modal.protocolPadding` | `{space.3}` | 3px |  |
| `modal.protocolIcon` | `{size.controlXs}` | 22px |  |

### formRow
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `formRow.labelWidth` | `{size.formLabel}` | 72px |  |
| `formRow.labelGap` | `{space.12}` | 12px |  |
| `formRow.rowGap` | `{space.12}` | 12px |  |
| `formRow.label` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `formRow.labelSize` | `{font.size.body}` | 12.5px | tab、按钮、列表 |
| `formRow.labelHeight` | `{size.control}` | 28px |  |
| `formRow.error` | `{color.danger.text}` | #d70015 | 失败文字 |
| `formRow.errorSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `formRow.errorIcon` | `{size.formErrorIcon}` | 12px |  |
| `formRow.errorGap` | `{space.5}` | 5px |  |
| `formRow.description` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `formRow.descriptionSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `formRow.descGap` | `{space.4}` | 4px |  |

### testResult
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `testResult.gap` | `{space.6}` | 6px |  |
| `testResult.fontSize` | `{font.size.small}` | 12px | sider footer、主机副信息 |
| `testResult.icon` | `{size.iconSm}` | 14px |  |
| `testResult.spinnerSize` | `{size.testSpinner}` | 13px |  |
| `testResult.testing` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `testResult.success` | `{color.success.text}` | #1f8a3f |  |
| `testResult.successIcon` | `{color.success.default}` | #30c45a | 在线点、完成进度 |
| `testResult.fail` | `{color.danger.text}` | #d70015 | 失败文字 |
| `testResult.failIcon` | `{color.danger.default}` | #ff3b30 | 失败圆点、失败图标 |
| `testResult.info` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `testResult.infoSize` | `{size.formErrorIcon}` | 12px |  |
| `testResult.crossfade` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |

### collapse
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `collapse.headerHeight` | `{size.titlebar}` | 44px |  |
| `collapse.chevron` | `{size.collapseChevron}` | 12px |  |
| `collapse.chevronStroke` | `{size.collapseChevronStroke}` | 2.25px |  |
| `collapse.chevronColor` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `collapse.title` | `{color.text.primary}` | #1d1d1f |  |
| `collapse.titleSize` | `{font.size.bodyLg}` | 13px | 主机名、标题 |
| `collapse.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `collapse.summary` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `collapse.summarySize` | `{font.size.small}` | 12px | sider footer、主机副信息 |
| `collapse.duration` | `{motion.duration.collapse}` | 180ms | 分组拖动时全部收起 |
| `collapse.chevronRotate` | `{size.collapseChevronRotate}` | 90px |  |
| `collapse.contentFadeFactor` | `{opacity.collapseContentFadeFactor}` | 1.6 |  |

### switch
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `switch.w` | `{size.switchWidth}` | 32px |  |
| `switch.h` | `{size.switchHeight}` | 19px |  |
| `switch.knobSize` | `{size.switchKnob}` | 15px |  |
| `switch.inset` | `{space.2}` | 2px |  |
| `switch.off` | `{color.border.keycap}` | #e3e3e6 | kbd 描边 |
| `switch.on` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `switch.knob` | `{color.text.onAccent}` | #ffffff |  |
| `switch.knobShadow` | `{shadow.switchKnob}` | 0px 1px 2px 0px rgba(0,0,0,0.2) | 开关滑块 |
| `switch.animation` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |

### settings
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `settings.navWidth` | `{size.sider}` | 236px |  |
| `settings.navItemHeight` | `{size.modalIconTile}` | 32px |  |
| `settings.navItemRadius` | `{radius.control}` | 7px |  |
| `settings.navItemOn` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `settings.navTextOn` | `{color.text.accentStrong}` | #0a5bc4 | 选中文件名 |
| `settings.navIconOn` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `settings.navIconTile` | `{size.controlXs}` | 22px |  |
| `settings.headerPaddingTop` | `{space.16}` | 16px |  |
| `settings.headerPaddingRight` | `{space.14}` | 14px |  |
| `settings.headerPaddingBottom` | `{space.14}` | 14px |  |
| `settings.headerPaddingLeft` | `{size.settingsHeaderPaddingX}` | 32px |  |
| `settings.titleSize` | `{font.size.title}` | 15px | 大标题（sider-drag 版 sider 标题） |
| `settings.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `settings.subtitleSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `settings.subtitle` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `settings.close` | `{size.control}` | 28px |  |
| `settings.contentMax` | `{size.settingsContent}` | 720px |  |
| `settings.contentPaddingY` | `{space.20}` | 20px |  |
| `settings.contentPaddingX` | `{size.settingsHeaderPaddingX}` | 32px |  |
| `settings.groupTitleSize` | `{font.size.small}` | 12px | sider footer、主机副信息 |
| `settings.groupTitleWeight` | `{font.weight.semibold}` | 600 |  |
| `settings.groupTitle` | `{color.text.muted}` | #6e6e73 | 未选中 tab、rail 图标、按钮图标 |
| `settings.groupTitleGap` | `{space.8}` | 8px |  |
| `settings.groupGap` | `{space.22}` | 22px |  |
| `settings.groupBg` | `{color.surface.zebra}` | #fafafb | 列表斑马纹 |
| `settings.groupRadius` | `{radius.card}` | 10px |  |
| `settings.groupBorder` | `{color.border.raised}` | rgba(0,0,0,0.08) | 浮起层 0.5px 描边 |
| `settings.groupBorderWidth` | `{size.hairline}` | 0.5px | 0.5px 描边 |
| `settings.rowMinHeight` | `{size.settingsRowHeight}` | 48px |  |
| `settings.rowPaddingY` | `{space.9}` | 9px |  |
| `settings.rowPaddingX` | `{space.14}` | 14px |  |
| `settings.rowDivider` | `{color.border.subtle}` | #f0f0f2 | 卡片内分隔线（sider footer、状态栏、进度轨道） |
| `settings.rowDividerInset` | `{space.14}` | 14px |  |
| `settings.rowLabelSize` | `{font.size.bodyLg}` | 13px | 主机名、标题 |
| `settings.rowLabel` | `{color.text.primary}` | #1d1d1f |  |
| `settings.rowDescSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `settings.rowDesc` | `{color.text.secondary}` | #8e8e93 | 次要文字、主机副标题、状态栏 |
| `settings.groupFootnoteSize` | `{font.size.footnote}` | 11.5px | 分组头、副标题、状态栏 |
| `settings.groupFootnote` | `{color.text.tertiary}` | #a1a1a6 | 关闭 ×、kbd、caption |
| `settings.fadeHeight` | `{size.settingsFadeHeight}` | 40px |  |
| `settings.enterY` | `{space.8}` | 8px |  |
| `settings.exitY` | `{space.4}` | 4px |  |
| `settings.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | 取消 / 平滑（ui.Smooth） |
| `settings.fade` | `{motion.duration.focus}` | 160ms | 焦点/悬停切换 |
| `settings.exit` | `{motion.duration.exit}` | 120ms | 浮层退出、胶囊淡出 |
| `settings.switchY` | `{space.4}` | 4px |  |
| `settings.switchFadeOut` | `{motion.duration.settingsSwitchOut}` | 80ms |  |
| `settings.switchFadeIn` | `{motion.duration.exit}` | 120ms | 浮层退出、胶囊淡出 |
| `settings.selectWidth` | `{size.settingsSelect}` | 220px |  |
| `settings.numberWidth` | `{size.settingsNumber}` | 64px |  |
| `settings.scrollbackWidth` | `{size.settingsScrollback}` | 96px |  |

### schemeCard
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `schemeCard.radius` | `{radius.lift}` | 9px |  |
| `schemeCard.padding` | `{space.5}` | 5px |  |
| `schemeCard.border` | `{color.surface.controlStrong}` | #ececef | sider 行内「更多」按钮 |
| `schemeCard.borderWidth` | `{size.divider}` | 1px |  |
| `schemeCard.selected` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `schemeCard.selectedWidth` | `{size.progressThin}` | 2px |  |
| `schemeCard.previewHeight` | `{size.schemePreviewHeight}` | 66px |  |
| `schemeCard.previewRadius` | `{radius.controlSm}` | 6px |  |
| `schemeCard.previewFontSize` | `{size.schemePreviewFont}` | 10px |  |
| `schemeCard.previewLineHeight` | `{size.schemePreviewLineHeight}` | 16px |  |
| `schemeCard.stripeHeight` | `{size.progress}` | 5px |  |
| `schemeCard.stripeRadius` | `{radius.hair}` | 2px |  |
| `schemeCard.stripeGap` | `{space.2}` | 2px |  |
| `schemeCard.nameSize` | `{font.size.small}` | 12px | sider footer、主机副信息 |
| `schemeCard.nameWeight` | `{font.weight.medium}` | 500 |  |
| `schemeCard.badgeSize` | `{font.size.caption2}` | 10.5px | ⌘N 芯片、计数胶囊 |
| `schemeCard.badgeWeight` | `{font.weight.semibold}` | 600 |  |
| `schemeCard.badgeBg` | `{color.accent.subtle}` | #eaf3fe | 浅蓝底（上传、运行中、提示） |
| `schemeCard.badgeText` | `{color.text.accent}` | #0a6fd6 | 蓝色文字（徽章、提示、高亮分组） |
| `schemeCard.badgeIcon` | `{size.schemeBadgeIcon}` | 10px |  |

### themeCard
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `themeCard.aspect` | `{size.themeAspect}` | 1.6 |  |
| `themeCard.radius` | `{radius.lift}` | 9px |  |
| `themeCard.border` | `{color.surface.controlStrong}` | #ececef | sider 行内「更多」按钮 |
| `themeCard.borderWidth` | `{size.divider}` | 1px |  |
| `themeCard.selected` | `{color.accent.default}` | #0a84ff | 主强调色 #0a84ff |
| `themeCard.selectedWidth` | `{size.progressThin}` | 2px |  |
| `themeCard.selectedShadow` | `{shadow.themeSelected}` | 0px 0px 0px 3px rgba(10,132,255,0.16) | 外观主题卡片选中光晕 |
| `themeCard.radio` | `{size.themeRadio}` | 14px |  |
| `themeCard.radioRing` | `{size.themeRadioRing}` | 4.5px |  |

### accentSwatch
| token | 引用 | Light 值 | 说明 |
|---|---|---|---|
| `accentSwatch.size` | `{size.accentSwatch}` | 20px |  |
| `accentSwatch.gap` | `{space.10}` | 10px |  |
| `accentSwatch.ringGap` | `{size.accentRingGap}` | 2px |  |
| `accentSwatch.ringWidth` | `{size.accentRing}` | 1.5px |  |
| `accentSwatch.ringInset` | `{color.surface.card}` | #ffffff | 内容卡片 / sider / FTP 面板 |
