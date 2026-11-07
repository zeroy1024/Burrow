# Burrow UI design tokens

[English](tokens.md) | [简体中文](tokens.zh.md)
Scope: the app UI, including windows, title bars, rail, sidebar, groups, host rows, tabs, splits/focus, status bars, glass fades, buttons, inputs, badges, tooltips, file lists, transfers, and drag ordering.
**Terminal contents are separate**: foreground/ANSI colors, terminal fonts, and cursor shapes are outside this model, except `paneFocus.cursorRingUnfocused`. Terminal card containers belong to surface tokens.

Design sources: `ssh-v6.html`, `common.css`, `parts/split_style.css`, `focus.py` (focus scheme D), `sider-drag.html` (drag parameters), `ftp.html`, `logo/` (brand blue #0a84ff), and `SSH-Tool-FTP-v3`. Values inferred to complete the system are **derived** (`$extensions.app.derived = true` in JSON; `[derived]` in Go).

| Layer | Count | Purpose |
|---|---|---|
| palette (primitives) | 282 | Literal colors/scales, including 50 derived values |
| semantic (per theme) | 499 | Light follows the design; Dark has 111 derived overrides |
| component | 1113 | Semantic references only; follows the current theme |

## 1. Layers and reference rules

```
palette.gray.900 (#1d1d1f)      <- primitive, unchanged by themes
   ^
color.text.primary             <- semantic, overridden by themes
   ^
siderItem.nameColor             <- component, references semantics only
```

- **palette** contains literals only.
- **semantic** (`theme.light` / `theme.dark`) references only `{palette.*}`.
- **component** references semantic paths without theme prefixes, such as `{color.text.primary}`, resolved by the current theme. No literals or direct palette references.
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
- Shadows: `theme.Shadow` (`[]ShadowLayer{X,Y,Blur,Spread,Color,Inset}`). `Blur=0, Spread>0` creates a border ring. Stack layers or combine border/shadow when only one native layer is available.
- Motion: `time.Duration`; `theme.Spring` has `Stiffness/Damping` for displacement or `Duration/Bounce` for drop/cancel, matching MyGo `ui.SpringOptions{Stiffness, Damping}` / `{Duration, Bounce}`. `motion.spring.smooth` approximates `ui.Smooth`.
- Glass fades: `C.FadeBar` specifies three blur radii/mask stops and tint alpha endpoints using the containing card color. Without GPU backdrop blur, draw tint gradients only.
- Current theme: `theme.Current()`; switch with `theme.Set(theme.Dark())`; subscribe with `cancel := theme.OnChange(func(t *theme.Theme){ /* request redraw */ })`.
- System appearance: call `theme.Set(theme.ForAppearance(isDark))` from appearance callbacks; use current official MyGo APIs.
- Controls: `internal/uikit` consumes component tokens and maps native themes while retaining framework layout behavior.
- Transitions use explicit `motion.easing.out` and `motion.duration.focus` (160ms). Search geometry uses `search.easing`; counter motion uses `groupHeader.countMotion`.
- Reduced motion ends geometric translation/scale/rotation immediately. Visual alpha retains fades independently of geometry progress.
- General SVGs use themed stroke widths and cache by logical icon/stroke width; brand marks stay separate.
- OS logo backgrounds use `t.OSLogoBg(brand)`, mixing brand into card via `opacity.osTint` (light 9%, dark 18%).

## 4. Add a theme

1. Add new primitives to `PALETTE` in `tokens_src.py`, such as `PALETTE['solarized']`.
2. Create semantic overrides like `DARK`: `{'color.bg.window': P('solarized.base3'), ...}`.
3. Register in `gen.py`, or in Go: `s := theme.LightSemantic(); s.Color.Accent.Default = theme.Hex("#268bd2"); t := theme.New("solar", false, s)`.
4. In `internal/theme`, run `python3 -B gen.py && python3 -B mdgen.py && gofmt -w theme.go`, then existing builds/vet/tests from the repository root.
5. Components need no changes. A theme-specific component issue usually needs a missing semantic token, not hardcoded component values.

## 5. Derived values

- Missing from designs: pressed state, `size.dropLine` (2px MyGo `ListState.Reorder` line), `menu.*` derived from raised/dragged surfaces, button hover/pressed, icon-button hover.
- Tab/pane drag reuse sidebar springs: `reorderMotion.tabDisplace / paneDisplace / paneDrop`.
- The complete Dark theme: 111 overrides and `palette.graphite / darkTint / whiteA / blackA / inkDark`.
- FTP v3 danger red, extra native-pane dimensions, and demo throughput are derived. Deprecated sidebar connection/selection tokens remain generated for compatibility but must not be used by new components. `siderItem.bgActive` is catalog focus only.
- Conflicting designs follow the main source: legacy sidebar selection retains `common.css` `#efeff1` instead of drag-design `#f2f2f4`; current focus follows `color.state.selectedAccent`. Icon stroke uses 1.75 from `icons.js`, not drag-design 1.7.
- Check dark brand-glyph contrast (for example CentOS/Debian); glyph rendering can mix toward white without rewriting the original brand palette.

## 6. Palette

### palette.gray
| token | Value | Notes |
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
| token | Value | Notes |
|---|---|---|
| `palette.blue.50` | #eaf3fe |  |
| `palette.blue.75` | #e8f2fe |  |
| `palette.blue.100` | #d7e9fd | Folder icon fill |
| `palette.blue.300` | #5ba7f7 |  |
| `palette.blue.500` | #0a84ff | Brand blue / accent matching the logo |
| `palette.blue.600` | #0a6fd6 |  |
| `palette.blue.700` | #0a5bc4 |  |

### palette.green
| token | Value | Notes |
|---|---|---|
| `palette.green.50` | #ebf8ef |  |
| `palette.green.500` | #30c45a |  |
| `palette.green.700` | #1f8a3f |  |

### palette.amber
| token | Value | Notes |
|---|---|---|
| `palette.amber.50` | #fdf3e3 |  |
| `palette.amber.500` | #f5a623 |  |
| `palette.amber.600` | #c7861a |  |
| `palette.amber.700` | #b26a00 |  |

### palette.traffic
| token | Value | Notes |
|---|---|---|
| `palette.traffic.red` | #ff5f57 |  |
| `palette.traffic.yellow` | #febc2e |  |
| `palette.traffic.green` | #28c840 |  |

### palette.black
| token | Value | Notes |
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
| token | Value | Notes |
|---|---|---|
| `palette.white.a30` | rgba(255,255,255,0.3) |  |
| `palette.white.a42` | rgba(255,255,255,0.42) |  |
| `palette.white.a60` | rgba(255,255,255,0.6) |  |
| `palette.white.a70` | rgba(255,255,255,0.7) |  |
| `palette.white.a85` | rgba(255,255,255,0.85) |  |
| `palette.white.a90` | rgba(255,255,255,0.9) |  |
| `palette.white.a75` | rgba(255,255,255,0.75) | **derived** Existing white opacity specified by the design |

### palette.blueA
| token | Value | Notes |
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
| token | Value | Notes |
|---|---|---|
| `palette.greenA.a14` | rgba(48,196,90,0.14) |  |
| `palette.greenA.a15` | rgba(48,196,90,0.15) |  |

### palette.amberA
| token | Value | Notes |
|---|---|---|
| `palette.amberA.a14` | rgba(245,166,35,0.14) |  |
| `palette.amberA.a18` | rgba(245,166,35,0.18) |  |

### palette.ink
| token | Value | Notes |
|---|---|---|
| `palette.ink.a92` | rgba(30,30,32,0.92) | Tooltip background |

### palette.graphite
| token | Value | Notes |
|---|---|---|
| `palette.graphite.950` | #161618 | **derived** Dark theme |
| `palette.graphite.900` | #1c1c1e | **derived** Dark theme |
| `palette.graphite.850` | #202022 | **derived** Dark theme |
| `palette.graphite.800` | #252527 | **derived** Dark theme |
| `palette.graphite.750` | #2a2a2d | **derived** Dark theme |
| `palette.graphite.700` | #2e2e31 | **derived** Dark theme |
| `palette.graphite.650` | #333336 | **derived** Dark theme |
| `palette.graphite.600` | #3a3a3d | **derived** Dark theme |
| `palette.graphite.550` | #444448 | **derived** Dark theme |
| `palette.graphite.500` | #5a5a5f | **derived** Dark theme |
| `palette.graphite.400` | #6e6e73 | **derived** Dark theme |
| `palette.graphite.300` | #8e8e93 | **derived** Dark theme |
| `palette.graphite.250` | #98989d | **derived** Dark theme |
| `palette.graphite.200` | #a1a1a6 | **derived** Dark theme |
| `palette.graphite.150` | #c7c7cc | **derived** Dark theme |
| `palette.graphite.100` | #d1d1d6 | **derived** Dark theme |
| `palette.graphite.50` | #f5f5f7 | **derived** Dark theme |

### palette.darkTint
| token | Value | Notes |
|---|---|---|
| `palette.darkTint.blue300` | #4da3ff | **derived** Dark theme |
| `palette.darkTint.blue200` | #6cb4ff | **derived** Dark theme |
| `palette.darkTint.blueSubtle` | #16304d | **derived** Dark theme |
| `palette.darkTint.green400` | #32d74b | **derived** Dark theme |
| `palette.darkTint.green300` | #5fd97a | **derived** Dark theme |
| `palette.darkTint.greenSubtle` | #173523 | **derived** Dark theme |
| `palette.darkTint.amber400` | #ffb340 | **derived** Dark theme |
| `palette.darkTint.amber300` | #ffc15c | **derived** Dark theme |
| `palette.darkTint.amberSubtle` | #3d2e12 | **derived** Dark theme |
| `palette.darkTint.red400` | #ff453a | **derived** Dark danger color |
| `palette.darkTint.red300` | #ff6961 | **derived** Dark danger text |
| `palette.darkTint.redSubtle` | #3a1d1c | **derived** Dark subtle danger background |

### palette.whiteA
| token | Value | Notes |
|---|---|---|
| `palette.whiteA.a06` | rgba(255,255,255,0.06) | **derived** Dark theme |
| `palette.whiteA.a08` | rgba(255,255,255,0.08) | **derived** Dark theme |
| `palette.whiteA.a10` | rgba(255,255,255,0.1) | **derived** Dark theme |
| `palette.whiteA.a12` | rgba(255,255,255,0.12) | **derived** Dark theme |
| `palette.whiteA.a14` | rgba(255,255,255,0.14) | **derived** Dark theme |
| `palette.whiteA.a16` | rgba(255,255,255,0.16) | **derived** Dark theme |
| `palette.whiteA.a20` | rgba(255,255,255,0.2) | **derived** Dark theme |

### palette.blackA
| token | Value | Notes |
|---|---|---|
| `palette.blackA.a25` | rgba(0,0,0,0.25) | **derived** Dark theme |
| `palette.blackA.a30` | rgba(0,0,0,0.3) | **derived** Dark theme |
| `palette.blackA.a35` | rgba(0,0,0,0.35) | **derived** Dark theme |
| `palette.blackA.a40` | rgba(0,0,0,0.4) | **derived** Dark theme |
| `palette.blackA.a45` | rgba(0,0,0,0.45) | **derived** Dark theme |
| `palette.blackA.a50` | rgba(0,0,0,0.5) | **derived** Dark theme |

### palette.inkDark
| token | Value | Notes |
|---|---|---|
| `palette.inkDark.a95` | rgba(58,58,62,0.95) | **derived** Dark tooltip background |

### palette.space
| token | Value | Notes |
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
| token | Value | Notes |
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
| token | Value | Notes |
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
| token | Value | Notes |
|---|---|---|
| `palette.fontSize.10_5` | 10.5px |  |
| `palette.fontSize.11` | 11px |  |
| `palette.fontSize.11_5` | 11.5px |  |
| `palette.fontSize.12` | 12px |  |
| `palette.fontSize.12_5` | 12.5px |  |
| `palette.fontSize.13` | 13px |  |
| `palette.fontSize.15` | 15px |  |

### palette.lineHeight
| token | Value | Notes |
|---|---|---|
| `palette.lineHeight.14` | 14px |  |
| `palette.lineHeight.15` | 15px |  |
| `palette.lineHeight.16` | 16px |  |
| `palette.lineHeight.17` | 17px |  |
| `palette.lineHeight.18` | 18px |  |

### palette.fontWeight
| token | Value | Notes |
|---|---|---|
| `palette.fontWeight.400` | 400 |  |
| `palette.fontWeight.500` | 500 |  |
| `palette.fontWeight.600` | 600 |  |

### palette.fontFamily
| token | Value | Notes |
|---|---|---|
| `palette.fontFamily.system` | -apple-system, SF Pro Text, PingFang SC, Inter, Noto Sans CJK SC, system-ui, sans-serif |  |
| `palette.fontFamily.mono` | JetBrains Mono, Menlo, monospace | UI hostnames/paths only, not terminal content |

### palette.letterSpacing
| token | Value | Notes |
|---|---|---|
| `palette.letterSpacing.neg0_2` | -0.2px |  |
| `palette.letterSpacing.zero` | 0px |  |
| `palette.letterSpacing.pos0_3` | 0.3px |  |

### palette.blur
| token | Value | Notes |
|---|---|---|
| `palette.blur.1` | 1px |  |
| `palette.blur.1_5` | 1.5px |  |
| `palette.blur.3` | 3px |  |
| `palette.blur.4` | 4px |  |
| `palette.blur.5` | 5px |  |
| `palette.blur.6` | 6px |  |
| `palette.blur.9` | 9px |  |
| `palette.blur.16` | 16px |  |
| `palette.blur.8` | 8px |  |

### palette.ratio
| token | Value | Notes |
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
| token | Value | Notes |
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
| token | Value | Notes |
|---|---|---|
| `palette.speed.autoScroll` | 900 | px/s |
| `palette.speed.demoTransfer` | 1.30023e+07 | **derived** Demo transfer bytes/second, 12.4 MiB/s |

### palette.spring
| token | Value | Notes |
|---|---|---|
| `palette.spring.snappy` | stiffness 420 · damping 34 |  |
| `palette.spring.drop` | duration 320ms · bounce 0.15 |  |
| `palette.spring.smooth` | duration 300ms · bounce 0 |  |
| `palette.spring.content` | duration 350ms · bounce 0 |  |

### palette.easing
| token | Value | Notes |
|---|---|---|
| `palette.easing.easeOut` | cubic-bezier(0,0,0.58,1) |  |
| `palette.easing.standard` | cubic-bezier(0.25,0.1,0.25,1) | **derived** CSS ease |
| `palette.easing.inOut` | cubic-bezier(0.42,0,0.58,1) |  |

### palette.red
| token | Value | Notes |
|---|---|---|
| `palette.red.50` | #fdecec | **derived** Light Apple systemRed background |
| `palette.red.500` | #ff3b30 | **derived** Apple systemRed |
| `palette.red.700` | #d70015 | **derived** Apple systemRed text |

### palette.shadow
| token | Value | Notes |
|---|---|---|
| `palette.shadow.popover` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.12), 0px 2px 6px 0px rgba(0,0,0,0.06), inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | Transfer overlay |
| `palette.shadow.popoverDark` | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 10px 28px 0px rgba(0,0,0,0.45), 0px 2px 6px 0px rgba(0,0,0,0.25) | **derived** Dark transfer overlay |

### palette.accentChoice
| token | Value | Notes |
|---|---|---|
| `palette.accentChoice.purple` | #8e5cf7 |  |
| `palette.accentChoice.pink` | #ec4899 |  |
| `palette.accentChoice.orange` | #ff9500 |  |
| `palette.accentChoice.yellow` | #f5b400 |  |

## 7. Semantic (Light / Dark)

### color
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `color.bg.window` | #e7e7e8 | #1c1c1e | Window background: title bar, rail, gaps between cards |
| `color.surface.card` | #ffffff | #252527 | Content cards, sidebar, FTP panes |
| `color.surface.terminal` | #fbfbfc | #202022 | Terminal card container background, excluding terminal text colors |
| `color.surface.raised` | #ffffff | #2e2e31 | Raised surfaces: dragged cards, ghosts, selected segments |
| `color.surface.sunken` | #f4f4f5 | #2a2a2d | Search field |
| `color.surface.field` | #f6f6f7 | #2a2a2d | Breadcrumb path field |
| `color.surface.zebra` | #fafafb | rgba(255,255,255,0.06) | Alternating list rows |
| `color.surface.control` | #f2f2f4 | #2e2e31 | Neutral control background: icon buttons, segments, badges, queue icons |
| `color.surface.controlStrong` | #ececef | #333336 | Inline sidebar More button |
| `color.surface.keycap` | #ffffff | #2e2e31 | Keycap background |
| `color.surface.liftStack` | #f9f9fa | #333336 |  |
| `color.state.hover` | #f7f7f8 | #2a2a2d | Row hover |
| `color.state.selected` | #efeff1 | #333336 | Selected sidebar row |
| `color.state.pressed` | #e6e6e9 | #3a3a3d | **derived** Pressed state, absent from design |
| `color.state.onWindow` | #d6d6d8 | #3a3a3d | Active controls on window background: rail and title toggles |
| `color.state.selectedAccent` | #e8f2fe | #16304d | Selected file row |
| `color.state.tabHover` | rgba(255,255,255,0.45) | rgba(255,255,255,0.1) |  |
| `color.state.railHover` | rgba(0,0,0,0.045) | rgba(255,255,255,0.06) |  |
| `color.state.railPressed` | rgba(0,0,0,0.12) | rgba(255,255,255,0.12) |  |
| `color.border.card` | rgba(0,0,0,0.06) | rgba(255,255,255,0.08) | 0.5px outer card border |
| `color.border.terminal` | rgba(0,0,0,0.08) | rgba(255,255,255,0.1) | 0.5px inner terminal/pane border |
| `color.border.subtle` | #f0f0f2 | #333336 | Inner dividers: sidebar footer, status bars, progress tracks |
| `color.border.divider` | #f2f2f4 | #333336 | Header and table-header bottom lines |
| `color.border.tab` | #efeff1 | #333336 | Legacy tab line / FTP pane border |
| `color.border.control` | #e6e6e9 | #444448 | Button borders and vertical status separators |
| `color.border.keycap` | #e3e3e6 | #444448 | Keycap border |
| `color.border.separatorWindow` | #d2d2d6 | #444448 | Title-bar button-group divider |
| `color.border.onGlass` | rgba(0,0,0,0.1) | rgba(255,255,255,0.12) | Vertical glass-bar divider |
| `color.border.raised` | rgba(0,0,0,0.08) | rgba(255,255,255,0.1) | 0.5px raised-surface border |
| `color.border.hairlineStrong` | rgba(0,0,0,0.1) | rgba(255,255,255,0.12) | Dragged-card border |
| `color.border.controlHover` | #c7c7cc | #444448 | Hovered input border |
| `color.text.primary` | #1d1d1f | #f5f5f7 |  |
| `color.text.secondary` | #8e8e93 | #98989d | Secondary text, host subtitles, status bars |
| `color.text.tertiary` | #a1a1a6 | #6e6e73 | Close ×, keycaps, captions |
| `color.text.quaternary` | #b0b0b5 | #5a5a5f | Group counter |
| `color.text.muted` | #6e6e73 | #a1a1a6 | Unselected tabs, rail icons, button icons |
| `color.text.strong` | #3a3a3f | #d1d1d6 | Secondary button text |
| `color.text.action` | #4a4a50 | #c7c7cc | Title-bar action button |
| `color.text.neutralBadge` | #5b5b62 | #c7c7cc | Neutral badge text |
| `color.text.disabled` | #aeaeb2 | #6e6e73 | Unfocused pane host |
| `color.text.placeholder` | #c7c7cc | #5a5a5f | Breadcrumb separator |
| `color.text.onAccent` | #ffffff | ＝ |  |
| `color.text.accent` | #0a6fd6 | #4da3ff | Blue text: badges, notices, highlighted groups |
| `color.text.accentStrong` | #0a5bc4 | #6cb4ff | Selected filename |
| `color.icon.default` | #6e6e73 | #a1a1a6 |  |
| `color.icon.subtle` | #8e8e93 | ＝ |  |
| `color.icon.faint` | #a1a1a6 | #6e6e73 |  |
| `color.icon.folder` | #5ba7f7 | ＝ |  |
| `color.icon.folderFill` | #d7e9fd | #16304d |  |
| `color.accent.default` | #0a84ff | ＝ | Primary accent #0a84ff |
| `color.accent.subtle` | #eaf3fe | #16304d | Light blue background: upload, running, notices |
| `color.accent.tint035` | rgba(10,132,255,0.035) | ＝ | Drop-area background |
| `color.accent.tint06` | rgba(10,132,255,0.06) | ＝ | Accepting drop placeholder background |
| `color.accent.tint08` | rgba(10,132,255,0.08) | ＝ | Group drop highlight |
| `color.accent.tint10` | rgba(10,132,255,0.1) | rgba(10,132,255,0.18) | New-output badge / flash |
| `color.accent.tint13` | rgba(10,132,255,0.13) | ＝ | Auto-scroll zone gradient |
| `color.accent.tint15` | rgba(10,132,255,0.15) | ＝ | Progress track during auto-expansion |
| `color.accent.tint16` | rgba(10,132,255,0.16) | rgba(10,132,255,0.24) | Focus glow |
| `color.accent.tint18` | rgba(10,132,255,0.18) | ＝ | Flash inner border |
| `color.accent.tint28` | rgba(10,132,255,0.28) | ＝ | Placeholder inner border |
| `color.accent.tint35` | rgba(10,132,255,0.35) | ＝ | Annotation border |
| `color.accent.tint50` | rgba(10,132,255,0.5) | ＝ | Auto-scroll zone edge |
| `color.accent.hover` | #0a6fd6 | #4da3ff |  |
| `color.accent.pressed` | #0a5bc4 | #6cb4ff |  |
| `color.success.default` | #30c45a | #32d74b | Online dots and completed progress |
| `color.success.text` | #1f8a3f | #5fd97a |  |
| `color.success.subtle` | #ebf8ef | #173523 |  |
| `color.success.halo` | rgba(48,196,90,0.14) | ＝ | 3px online-dot halo |
| `color.success.onGlass` | rgba(48,196,90,0.15) | ＝ |  |
| `color.warning.default` | #f5a623 | #ffb340 | Connecting |
| `color.warning.text` | #c7861a | #ffc15c | Connecting text / bell |
| `color.warning.textStrong` | #b26a00 | #ffc15c |  |
| `color.warning.subtle` | #fdf3e3 | #3d2e12 |  |
| `color.warning.tint14` | rgba(245,166,35,0.14) | ＝ | Bell badge background |
| `color.warning.halo` | rgba(245,166,35,0.18) | ＝ |  |
| `color.status.offline` | #cfcfd4 | #5a5a5f | Offline dot |
| `color.status.paused` | #c7c7cc | #5a5a5f | Paused progress |
| `color.status.scrollbar` | rgba(0,0,0,0.32) | rgba(255,255,255,0.35) |  |
| `color.status.dividerHandle` | #c7c7cc | #5a5a5f |  |
| `color.status.danger` | #dc2626 | #ef4444 |  |
| `color.fill.quiet` | rgba(0,0,0,0.05) | rgba(255,255,255,0.08) | ⌘N chip and badges on glass |
| `color.fill.quieter` | rgba(0,0,0,0.04) | rgba(255,255,255,0.06) | Breadcrumbs on glass |
| `color.fill.placeholder` | rgba(0,0,0,0.035) | rgba(255,255,255,0.06) | Drag placeholder |
| `color.fill.placeholderEdge` | rgba(0,0,0,0.05) | rgba(255,255,255,0.08) |  |
| `color.fill.strong` | #1d1d1f | #f5f5f7 | Neutral progress fill |
| `color.fill.shimmer` | #fafafb | #3a3a3d | Skeleton shimmer highlight |
| `color.fill.quietHover` | rgba(0,0,0,0.08) | rgba(255,255,255,0.12) | Transfer capsule hover |
| `color.fill.skeleton` | #ececef | #444448 |  |
| `color.traffic.close` | #ff5f57 | ＝ |  |
| `color.traffic.minimize` | #febc2e | ＝ |  |
| `color.traffic.zoom` | #28c840 | ＝ |  |
| `color.traffic.edge` | rgba(0,0,0,0.15) | rgba(0,0,0,0.25) |  |
| `color.tooltip.bg` | rgba(30,30,32,0.92) | rgba(58,58,62,0.95) |  |
| `color.tooltip.text` | #ffffff | ＝ |  |
| `color.tooltip.kbd` | rgba(255,255,255,0.6) | ＝ |  |
| `color.glass.titleTab` | rgba(255,255,255,0.85) | rgba(255,255,255,0.12) | Selected title-bar tab |
| `color.glass.float` | rgba(255,255,255,0.7) | rgba(255,255,255,0.1) | Floating scroll-to-bottom capsule |
| `color.glass.bar` | rgba(255,255,255,0.42) | rgba(255,255,255,0.08) | v4 glass bar without fade |
| `color.glass.highlight` | rgba(255,255,255,0.9) | rgba(255,255,255,0.1) | Glass top inner highlight |
| `color.danger.default` | #ff3b30 | #ff453a | Failure dots and icons |
| `color.danger.text` | #d70015 | #ff6961 | Failure text |
| `color.danger.subtle` | #fdecec | #3a1d1c | Failure icon and badge backgrounds |
| `color.danger.tint14` | rgba(255,59,48,0.14) | rgba(255,69,58,0.22) | Input-error outer glow |
| `color.danger.hover` | #ff3b30 | #ff453a |  |
| `color.danger.pressed` | #ff3b30 | #ff6961 |  |
| `color.protocol.sftpBg` | #eaf3fe | #16304d | SFTP icon tile background |
| `color.protocol.sftpGlyph` | #0a84ff | ＝ | SFTP glyph |
| `color.protocol.ftpBg` | #f2f2f4 | #2e2e31 | FTP icon tile background |
| `color.protocol.ftpGlyph` | #8e8e93 | #98989d | FTP glyph |
| `color.overlay.backdrop` | rgba(0,0,0,0.4) | ＝ | Dialog backdrop |
| `color.accentChoices.blue` | #0a84ff | ＝ |  |
| `color.accentChoices.purple` | #8e5cf7 | ＝ |  |
| `color.accentChoices.pink` | #ec4899 | ＝ |  |
| `color.accentChoices.red` | #ff3b30 | ＝ |  |
| `color.accentChoices.orange` | #ff9500 | ＝ |  |
| `color.accentChoices.yellow` | #f5b400 | ＝ |  |
| `color.accentChoices.green` | #30c45a | ＝ |  |
| `color.accentChoices.graphite` | #8e8e93 | ＝ |  |
| `color.scrim` | rgba(0,0,0,0.18) | rgba(0,0,0,0.45) | Session dialog backdrop, including title bar |
| `color.keycap.inkBg` | rgba(255,255,255,0.16) | ＝ |  |
| `color.keycap.inkText` | rgba(255,255,255,0.75) | ＝ |  |
| `color.keycap.accentBg` | rgba(255,255,255,0.2) | ＝ |  |
| `color.toast.action` | #4da3ff | ＝ |  |
| `color.toast.ok` | #32d74b | ＝ |  |
| `color.toast.error` | #ff453a | ＝ |  |
| `color.toast.warning` | #ffb340 | ＝ |  |

### radius
| token | Light | Dark (derived) | Purpose |
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
| token | Light | Dark (derived) | Purpose |
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
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `size.hairline` | 0.5px | ＝ | 0.5px border |
| `size.divider` | 1px | ＝ |  |
| `size.focusRing` | 1.5px | ＝ | Focus-ring / dashed drop-frame width |
| `size.focusHalo` | 5px | ＝ | Focus glow spread |
| `size.dot` | 7px | ＝ |  |
| `size.dotSm` | 6px | ＝ |  |
| `size.dotHalo` | 3px | ＝ |  |
| `size.trafficLight` | 12px | ＝ |  |
| `size.icon` | 16px | ＝ |  |
| `size.iconMd` | 15px | ＝ |  |
| `size.iconSm` | 14px | ＝ |  |
| `size.iconStroke` | 1.75px | ＝ | Lucide-style stroke width |
| `size.controlXs` | 22px | ＝ |  |
| `size.controlSm` | 24px | ＝ |  |
| `size.controlMd` | 26px | ＝ |  |
| `size.control` | 28px | ＝ |  |
| `size.controlLg` | 30px | ＝ |  |
| `size.railButton` | 34px | ＝ |  |
| `size.titlebar` | 44px | ＝ |  |
| `size.rail` | 52px | ＝ |  |
| `size.sider` | 236px | ＝ |  |
| `size.titleLeft` | 296px | ＝ | Left title area: traffic lights/sidebar toggle, aligned to rail/sidebar |
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
| `size.dropLine` | 2px | ＝ | **derived** Insertion line (ListState.Reorder) |
| `size.fadeExtent` | 24px | ＝ | Glass-bar fade extending into content |
| `size.fadeExtentPane` | 22px | ＝ | Fade length inside split panes |
| `size.statsPanel` | 204px | ＝ | Monitoring sidebar in split view |
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
| `size.statsChartHeight` | 44px | ＝ | CPU trend chart |
| `size.statsChartStroke` | 1.3px | ＝ |  |
| `size.statsSectionGap` | 18px | ＝ |  |
| `size.statsNetChartHeight` | 20px | ＝ | Upload/download mini trends |
| `size.statsValueLarge` | 28px | ＝ | Main CPU value |
| `size.statsSkeletonLine` | 10px | ＝ | Top-process skeleton bars |
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
| `size.transferStatusWidth` | 80px | ＝ | **derived** Fixed transfer-status column width |
| `size.splitGripHeight` | 32px | ＝ |  |
| `size.transferPopoverHeader` | 32px | ＝ |  |
| `size.fileNameMinWidth` | 100px | ＝ |  |
| `size.fileGridIconSize` | 32px | ＝ |  |
| `size.transferDockMaxRows` | 5 | ＝ |  |
| `size.transferPopoverMaxRows` | 6 | ＝ |  |
| `size.transferConcurrency` | 3 | ＝ |  |
| `size.tooltipMaxWidth` | 320px | ＝ | Maximum tooltip width |
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
| `size.settingsShortcut` | 176px | ＝ |  |
| `size.keycap` | 18px | ＝ |  |
| `size.buttonLarge` | 32px | ＝ |  |
| `size.badgeSmall` | 18px | ＝ |  |
| `size.badgeMedium` | 20px | ＝ |  |
| `size.check` | 14px | ＝ |  |
| `size.radioDot` | 4.5px | ＝ |  |
| `size.panelHeader` | 46px | ＝ |  |
| `size.toast` | 340px | ＝ |  |
| `size.emptyMaxWidth` | 220px | ＝ |  |
| `size.keyValueRow` | 22px | ＝ |  |
| `size.sparklineHeight` | 34px | ＝ |  |
| `size.sparklineStroke` | 1.5px | ＝ |  |
| `size.meterHeight` | 4px | ＝ |  |
| `size.iconXs` | 12px | ＝ |  |
| `size.segmentedItem` | 24px | ＝ |  |
| `size.latencyWarn` | 150 | ＝ |  |
| `size.latencyBad` | 400 | ＝ |  |
| `size.tableNameMinWidth` | 160px | ＝ |  |
| `size.sparklineDotRadius` | 2.5px | ＝ | Trend endpoint |
| `size.sparklineHaloRadius` | 5px | ＝ | Trend endpoint halo |
| `size.meterWarnRatio` | 0.7 | ＝ |  |
| `size.meterCriticalRatio` | 0.9 | ＝ |  |
| `size.mcpTableMinWidth` | 1024px | ＝ |  |
| `size.mcpNameColumn` | 210px | ＝ |  |
| `size.mcpBadgeColumn` | 86px | ＝ |  |
| `size.mcpScopeColumn` | 130px | ＝ |  |
| `size.mcpTimeColumn` | 150px | ＝ |  |
| `size.mcpKeysColumn` | 115px | ＝ |  |
| `size.mcpAuditTimeColumn` | 72px | ＝ |  |
| `size.mcpAuditKeyColumn` | 148px | ＝ |  |
| `size.mcpAuditConnectionColumn` | 120px | ＝ |  |
| `size.mcpAuditActionColumn` | 96px | ＝ |  |
| `size.mcpAuditStatusColumn` | 104px | ＝ |  |
| `size.mcpAuditDurationColumn` | 56px | ＝ |  |
| `size.mcpPolicyModal` | 720px | ＝ |  |
| `size.mcpKeyModal` | 680px | ＝ |  |
| `size.mcpDialog` | 440px | ＝ |  |
| `size.mcpDrawerMinWidth` | 340px | ＝ |  |
| `size.mcpDrawerWidth` | 500px | ＝ |  |
| `size.mcpMainMinWidth` | 360px | ＝ |  |
| `size.mcpRuleModeWidth` | 120px | ＝ | Command-rule match-mode selector |
| `size.mcpPermissionColumn` | 76px | ＝ | Connection-grant capability column |

### font
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `font.family.ui` | -apple-system, SF Pro Text, PingFang SC, Inter, Noto Sans CJK SC, system-ui, sans-serif | ＝ |  |
| `font.family.mono` | JetBrains Mono, Menlo, monospace | ＝ |  |
| `font.size.caption2` | 10.5px | ＝ | ⌘N chip and count capsules |
| `font.size.caption` | 11px | ＝ | Keycaps, status badges, counters |
| `font.size.footnote` | 11.5px | ＝ | Group headers, subtitles, status bars |
| `font.size.small` | 12px | ＝ | Sidebar footer and secondary host information |
| `font.size.body` | 12.5px | ＝ | Tabs, buttons, lists |
| `font.size.bodyLg` | 13px | ＝ | Hostnames and headings |
| `font.size.title` | 15px | ＝ | Large heading from the sidebar-drag design |
| `font.weight.regular` | 400 | ＝ |  |
| `font.weight.medium` | 500 | ＝ |  |
| `font.weight.semibold` | 600 | ＝ |  |
| `font.weight.emptyPaneTitle` | 550 | ＝ |  |
| `font.lineHeight.tight` | 14px | ＝ |  |
| `font.lineHeight.sub` | 15px | ＝ |  |
| `font.lineHeight.body` | 16px | ＝ |  |
| `font.lineHeight.name` | 17px | ＝ |  |
| `font.lineHeight.keycap` | 12px | ＝ |  |
| `font.lineHeight.input` | 18px | ＝ | Align empty-input and Chinese-placeholder line heights across font fallback |
| `font.tracking.mono` | -0.2px | ＝ |  |
| `font.tracking.normal` | 0px | ＝ |  |
| `font.tracking.caps` | 0.3px | ＝ |  |
| `font.tracking.inputMono` | -0.1px | ＝ |  |

### shadow
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `shadow.window` | 0px 0px 0px 0.5px rgba(0,0,0,0.18), 0px 24px 60px 0px rgba(0,0,0,0.18), 0px 4px 14px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 24px 60px 0px rgba(0,0,0,0.5), 0px 4px 14px 0px rgba(0,0,0,0.3) |  |
| `shadow.card` | 0px 0px 0px 0.5px rgba(0,0,0,0.06) | 0px 0px 0px 0.5px rgba(255,255,255,0.08) | Outer card border |
| `shadow.paneOutline` | inset 0px 0px 0px 0.5px rgba(0,0,0,0.08) | inset 0px 0px 0px 0.5px rgba(255,255,255,0.1) | Pane inner border drawn after clipping |
| `shadow.titleTab` | 0px 0px 0px 0.5px rgba(0,0,0,0.07), 0px 1px 2px 0px rgba(0,0,0,0.05) | 0px 0px 0px 0.5px rgba(255,255,255,0.1), 0px 1px 2px 0px rgba(0,0,0,0.3) |  |
| `shadow.segment` | 0px 1px 2px 0px rgba(0,0,0,0.08) | ＝ |  |
| `shadow.float` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 4px 12px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 4px 12px 0px rgba(0,0,0,0.3) | Floating capsule |
| `shadow.tooltip` | 0px 4px 12px 0px rgba(0,0,0,0.18) | 0px 4px 12px 0px rgba(0,0,0,0.4) |  |
| `shadow.ghost` | 0px 8px 24px 0px rgba(0,0,0,0.12), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | 0px 8px 24px 0px rgba(0,0,0,0.45), 0px 0px 0px 0.5px rgba(255,255,255,0.12) | FTP drag ghost |
| `shadow.floatButton` | 0px 2px 8px 0px rgba(0,0,0,0.1), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | ＝ | FTP transfer button between panes |
| `shadow.lift` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 10px 28px 0px rgba(0,0,0,0.45), 0px 2px 6px 0px rgba(0,0,0,0.25) | Dragged card |
| `shadow.liftStack` | 0px 0px 0px 0.5px rgba(0,0,0,0.08), 0px 4px 10px 0px rgba(0,0,0,0.06) | ＝ | Stacked multiselection cards |
| `shadow.focusRing` | 0px 0px 0px 1.5px #0a84ff | ＝ | Focused pane: 1.5px blue border only |
| `shadow.hoverRing` | 0px 0px 0px 1px rgba(0,0,0,0.14) | 0px 0px 0px 1px rgba(255,255,255,0.2) | Unfocused pane hover |
| `shadow.flash` | inset 0px 0px 0px 1px rgba(10,132,255,0.18) | ＝ | Drop-flash inner border |
| `shadow.placeholder` | inset 0px 0px 0px 1px rgba(0,0,0,0.05) | inset 0px 0px 0px 1px rgba(255,255,255,0.08) |  |
| `shadow.placeholderAccent` | inset 0px 0px 0px 1px rgba(10,132,255,0.28) | ＝ |  |
| `shadow.osSelected` | 0px 0px 0px 0.5px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.12) | [deprecated: sidebar no longer shows connection/selection state] |
| `shadow.glassHighlight` | inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | inset 0px 0.5px 0px 0px rgba(255,255,255,0.1) |  |
| `shadow.dotOnline` | 0px 0px 0px 3px rgba(48,196,90,0.14) | ＝ | [deprecated: sidebar no longer shows connection/selection state] |
| `shadow.dotConnecting` | 0px 0px 0px 3px rgba(245,166,35,0.18) | ＝ | [deprecated: sidebar no longer shows connection/selection state] |
| `shadow.popover` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.12), 0px 2px 6px 0px rgba(0,0,0,0.06), inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | 0px 0px 0px 0.5px rgba(255,255,255,0.14), 0px 10px 28px 0px rgba(0,0,0,0.45), 0px 2px 6px 0px rgba(0,0,0,0.25) | Transfer overlay |
| `shadow.dialog` | 0px 10px 30px 0px rgba(0,0,0,0.3) | 0px 10px 30px 0px rgba(0,0,0,0.5) | Dialog shadow |
| `shadow.modal` | 0px 0px 0px 0.5px rgba(0,0,0,0.12), 0px 24px 64px 0px rgba(0,0,0,0.22), 0px 4px 16px 0px rgba(0,0,0,0.08) | 0px 0px 0px 0.5px rgba(255,255,255,0.12), 0px 24px 64px 0px rgba(0,0,0,0.55), 0px 4px 16px 0px rgba(0,0,0,0.3) | Session dialog |
| `shadow.inputFocus` | inset 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 3px rgba(10,132,255,0.16) | inset 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 3px rgba(10,132,255,0.24) | Focused input: inner ring and outer glow |
| `shadow.inputError` | inset 0px 0px 0px 1.5px #ff3b30, 0px 0px 0px 3px rgba(255,59,48,0.14) | inset 0px 0px 0px 1.5px #ff453a, 0px 0px 0px 3px rgba(255,69,58,0.22) | Input error, taking priority over focus |
| `shadow.switchKnob` | 0px 1px 2px 0px rgba(0,0,0,0.2) | ＝ | Switch thumb |
| `shadow.themeSelected` | 0px 0px 0px 3px rgba(10,132,255,0.16) | 0px 0px 0px 3px rgba(10,132,255,0.24) | Selected appearance-card glow |
| `shadow.controlFocus` | 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 5.5px rgba(10,132,255,0.16) | 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 5.5px rgba(10,132,255,0.24) |  |
| `shadow.cardInset` | inset 0px 0px 0px 0.5px rgba(0,0,0,0.06) | inset 0px 0px 0px 0.5px rgba(255,255,255,0.08) | UIKit inner card border without changing outer geometry |

### glass
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `glass.fade.blur1` | 1px | ＝ |  |
| `glass.fade.blur2` | 3px | ＝ |  |
| `glass.fade.blur3` | 5px | ＝ |  |
| `glass.fade.mask1Solid` | 0.45 | ＝ | Layer 1 opaque through 45%, transparent at 100% |
| `glass.fade.mask1End` | 1 | ＝ |  |
| `glass.fade.mask2Solid` | 0.4 | ＝ |  |
| `glass.fade.mask2End` | 0.78 | ＝ |  |
| `glass.fade.mask3Solid` | 0.3 | ＝ |  |
| `glass.fade.mask3End` | 0.6 | ＝ |  |
| `glass.fade.tintStart` | 0.92 | ＝ | Tint-gradient starting alpha using the containing card color |
| `glass.fade.tintMid` | 0.78 | ＝ |  |
| `glass.fade.tintMidStop` | 0.45 | ＝ |  |
| `glass.fade.tintEnd` | 0 | ＝ |  |
| `glass.float.blur` | 16px | ＝ |  |
| `glass.float.saturate` | 1.5 | ＝ |  |
| `glass.bar.blur` | 3px | ＝ |  |
| `glass.bar.saturate` | 1.8 | ＝ |  |
| `glass.paneActionBlur` | 4px | ＝ |  |
| `glass.paneRevealBlur` | 8px | ＝ |  |

### opacity
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `opacity.windowTint` | 0.18 | ＝ | **derived** Uniform tint strength over native macOS window material |
| `opacity.titlebarTint` | 0.92 | ＝ | **derived** Window tint strength over other platforms' native title material |
| `opacity.paneUnfocusedContent` | 0.5 | 0.45 |  |
| `opacity.paneUnfocusedSaturate` | 0.35 | ＝ |  |
| `opacity.paneUnfocusedStatus` | 0.55 | ＝ |  |
| `opacity.paneHoverContent` | 0.78 | ＝ |  |
| `opacity.paneHoverSaturate` | 0.7 | ＝ |  |
| `opacity.dragSource` | 0.4 | ＝ | Source row after dragging away while collapsed |
| `opacity.liftStackBack` | 0.8 | ＝ |  |
| `opacity.hidden` | 0 | ＝ |  |
| `opacity.osTint` | 0.09 | 0.18 | OS logo background: 9% brand color mixed into card |
| `opacity.liveBeatScale` | 1.35 | ＝ | Initial scale of the live status pulse |
| `opacity.dragRotation` | -1 | ＝ |  |
| `opacity.dragPreview` | 0.9 | ＝ |  |
| `opacity.previewHeader` | 0.96 | ＝ |  |
| `opacity.paneCloseScale` | 0.97 | ＝ |  |
| `opacity.dropEdgeRatio` | 0.3 | ＝ |  |
| `opacity.paneActionTint` | 0.9 | ＝ |  |
| `opacity.revealFrom` | 0.6 | ＝ | Initial opacity when expanding the local pane |
| `opacity.capsuleEnterScale` | 0.9 | ＝ |  |
| `opacity.popoverEnterScale` | 0.96 | ＝ |  |
| `opacity.splitDefaultRatio` | 0.5 | ＝ |  |
| `opacity.disabled` | 0.4 | ＝ |  |
| `opacity.modalEnterScale` | 0.96 | ＝ |  |
| `opacity.modalExitScale` | 0.98 | ＝ |  |
| `opacity.collapseContentFadeFactor` | 1.6 | ＝ |  |
| `opacity.sparklineFill` | 0.2 | ＝ | Top opacity of trend gradient fill |
| `opacity.sparklineHalo` | 0.18 | ＝ |  |

### motion
| token | Light | Dark (derived) | Purpose |
|---|---|---|---|
| `motion.duration.instant` | 0ms | ＝ |  |
| `motion.duration.lift` | 120ms | ＝ | Pick up |
| `motion.duration.hold` | 150ms | ＝ | Pick up after long press |
| `motion.duration.focus` | 160ms | ＝ | Focus/hover transition |
| `motion.duration.collapse` | 180ms | ＝ | Collapse all groups while dragging a group |
| `motion.duration.restore` | 220ms | ＝ | Restore expansion |
| `motion.duration.cancel` | 300ms | ＝ | Return animation on cancel |
| `motion.duration.drop` | 320ms | ＝ | Drop |
| `motion.duration.autoExpand` | 500ms | ＝ | Expand automatically on hover |
| `motion.duration.flash` | 600ms | ＝ | Drop flash |
| `motion.duration.shimmer` | 1400ms | ＝ | One skeleton shimmer cycle |
| `motion.duration.liveBeat` | 900ms | ＝ | Live status pulse per sample |
| `motion.duration.frameInterval` | 16ms | ＝ |  |
| `motion.duration.zoom` | 240ms | ＝ |  |
| `motion.duration.paneEnter` | 220ms | ＝ |  |
| `motion.duration.paneClose` | 160ms | ＝ |  |
| `motion.duration.dividerFade` | 120ms | ＝ |  |
| `motion.duration.sidebar` | 220ms | ＝ |  |
| `motion.duration.scrollbarHold` | 700ms | ＝ |  |
| `motion.duration.scrollbarFade` | 180ms | ＝ |  |
| `motion.duration.instantNonzero` | 1e-06ms | ＝ | 1 ns: MyGo zero Animation selects a default spring |
| `motion.duration.exit` | 120ms | ＝ | Overlay exit and capsule fade |
| `motion.duration.spin` | 800ms | ＝ | One connecting-spinner revolution |
| `motion.duration.paneReveal` | 220ms | ＝ |  |
| `motion.duration.connectionStage` | 300ms | ＝ | Duration of each demo SSH connection phase |
| `motion.duration.test` | 10000ms | ＝ |  |
| `motion.duration.settingsSwitchOut` | 80ms | ＝ |  |
| `motion.duration.fast` | 120ms | ＝ |  |
| `motion.duration.base` | 160ms | ＝ |  |
| `motion.duration.autoHide` | 3000ms | ＝ |  |
| `motion.duration.scrollGesture` | 300ms | ＝ | Maximum gap between events in one scroll gesture |
| `motion.easing.out` | cubic-bezier(0,0,0.58,1) | ＝ |  |
| `motion.easing.pane` | cubic-bezier(0.2,0.8,0.2,1) | ＝ |  |
| `motion.easing.search` | cubic-bezier(0.42,0,0.58,1) | ＝ |  |
| `motion.easing.in` | cubic-bezier(0.42,0,1,1) | ＝ |  |
| `motion.spring.displace` | stiffness 420 · damping 34 | ＝ | Displacement (approximately Snappy, 220ms) |
| `motion.spring.drop` | duration 320ms · bounce 0.15 | ＝ | Drop |
| `motion.spring.smooth` | duration 300ms · bounce 0 | ＝ | Cancel / smooth (ui.Smooth) |
| `motion.spring.zoom` | duration 240ms · bounce 0 | ＝ |  |
| `motion.spring.content` | duration 350ms · bounce 0 | ＝ | Numeric content change |
| `motion.drag.threshold` | 4px | ＝ | Movement threshold in px |
| `motion.drag.liftScale` | 1.03 | ＝ |  |
| `motion.drag.autoScrollMax` | 900 | ＝ | px/s, t² curve |
| `motion.drag.autoScrollZone` | 48px | ＝ |  |
| `motion.drag.standardScale` | 1.02 | ＝ |  |
| `motion.drag.standardRotation` | -0.6 | ＝ |  |
| `motion.demoTransferSpeed` | 1.30023e+07 | ＝ | **derived** Demo transfer throughput |

## 8. Component

References and resolved Light values; Dark follows the corresponding semantic values.

### window
| token | Reference | Light value | Description |
|---|---|---|---|
| `window.bg` | `{color.bg.window}` | #e7e7e8 | Window background: title bar, rail, gaps between cards |
| `window.tintOpacity` | `{opacity.windowTint}` | 0.18 | Uniform tint strength over native macOS window material |
| `window.radius` | `{radius.window}` | 12px |  |
| `window.shadow` | `{shadow.window}` | 0px 0px 0px 0.5px rgba(0,0,0,0.18), 0px 24px 60px 0px rgba(0,0,0,0.18), 0px 4px 14px 0px rgba(0,0,0,0.08) |  |
| `window.bodyPaddingRight` | `{space.8}` | 8px |  |
| `window.bodyPaddingBottom` | `{space.8}` | 8px |  |
| `window.gutter` | `{space.8}` | 8px |  |

### titlebar
| token | Reference | Light value | Description |
|---|---|---|---|
| `titlebar.height` | `{size.titlebar}` | 44px |  |
| `titlebar.tintOpacity` | `{opacity.titlebarTint}` | 0.92 | Window tint strength over other platforms' native title material |
| `titlebar.leftWidth` | `{size.titleLeft}` | 296px | Left title area: traffic lights/sidebar toggle, aligned to rail/sidebar |
| `titlebar.leftPaddingX` | `{space.16}` | 16px |  |
| `titlebar.leftGap` | `{space.8}` | 8px |  |
| `titlebar.trafficSize` | `{size.trafficLight}` | 12px |  |
| `titlebar.trafficClose` | `{color.traffic.close}` | #ff5f57 |  |
| `titlebar.trafficMinimize` | `{color.traffic.minimize}` | #febc2e |  |
| `titlebar.trafficZoom` | `{color.traffic.zoom}` | #28c840 |  |
| `titlebar.trafficEdge` | `{color.traffic.edge}` | rgba(0,0,0,0.15) |  |
| `titlebar.toggleIcon` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `titlebar.tabsGap` | `{space.2}` | 2px |  |
| `titlebar.actionsGap` | `{space.3}` | 3px |  |
| `titlebar.actionsPaddingRight` | `{space.10}` | 10px |  |

### tab
| token | Reference | Light value | Description |
|---|---|---|---|
| `tab.height` | `{size.control}` | 28px |  |
| `tab.radius` | `{radius.control}` | 7px |  |
| `tab.paddingLeft` | `{space.10}` | 10px |  |
| `tab.paddingRight` | `{space.8}` | 8px |  |
| `tab.gap` | `{space.7}` | 7px |  |
| `tab.fontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `tab.text` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `tab.textSelected` | `{color.text.primary}` | #1d1d1f |  |
| `tab.weightSelected` | `{font.weight.medium}` | 500 |  |
| `tab.bgSelected` | `{color.glass.titleTab}` | rgba(255,255,255,0.85) | Selected title-bar tab |
| `tab.shadowSelected` | `{shadow.titleTab}` | 0px 0px 0px 0.5px rgba(0,0,0,0.07), 0px 1px 2px 0px rgba(0,0,0,0.05) |  |
| `tab.close` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `tab.dotSize` | `{size.dotSm}` | 6px |  |
| `tab.addIcon` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `tab.contentHeight` | `{size.tabContentHeight}` | 20px |  |
| `tab.stripHeight` | `{size.tabStripHeight}` | 36px |  |
| `tab.dragHandleInset` | `{size.tabDragHandleInset}` | 30px |  |
| `tab.titleOffset` | `{size.tabTitleOffset}` | -1.5px |  |
| `tab.countOffset` | `{size.tabCountOffset}` | -0.5px |  |
| `tab.bgHover` | `{color.state.tabHover}` | rgba(255,255,255,0.45) |  |
| `tab.dotConnected` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `tab.dotError` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |
| `tab.ringDisconnected` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `tab.ringWidth` | `{size.focusRing}` | 1.5px | Focus-ring / dashed drop-frame width |
| `tab.spinner` | `{color.warning.default}` | #f5a623 | Connecting |
| `tab.spinnerTrack` | `{color.warning.halo}` | rgba(245,166,35,0.18) |  |
| `tab.spinnerSize` | `{size.tabSpinner}` | 10px |  |
| `tab.spinnerStroke` | `{size.iconStroke}` | 1.75px | Lucide-style stroke width |
| `tab.spinnerPeriod` | `{motion.duration.spin}` | 800ms | One connecting-spinner revolution |
| `tab.textDisconnected` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `tab.textErrorSelected` | `{color.danger.text}` | #d70015 | Failure text |
| `tab.stateTransition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |

### titleAction
| token | Reference | Light value | Description |
|---|---|---|---|
| `titleAction.size` | `{size.control}` | 28px |  |
| `titleAction.radius` | `{radius.control}` | 7px |  |
| `titleAction.icon` | `{color.text.action}` | #4a4a50 | Title-bar action button |
| `titleAction.iconSize` | `{size.iconMd}` | 15px |  |
| `titleAction.bgOn` | `{color.state.onWindow}` | #d6d6d8 | Active controls on window background: rail and title toggles |
| `titleAction.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `titleAction.bgPrimary` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `titleAction.iconPrimary` | `{color.text.onAccent}` | #ffffff |  |
| `titleAction.separator` | `{color.border.separatorWindow}` | #d2d2d6 | Title-bar button-group divider |
| `titleAction.separatorHeight` | `{size.separatorTall}` | 16px |  |
| `titleAction.separatorMarginX` | `{space.6}` | 6px |  |

### rail
| token | Reference | Light value | Description |
|---|---|---|---|
| `rail.width` | `{size.rail}` | 52px |  |
| `rail.gap` | `{space.6}` | 6px |  |
| `rail.paddingTop` | `{space.4}` | 4px |  |
| `rail.paddingBottom` | `{space.10}` | 10px |  |
| `rail.buttonSize` | `{size.railButton}` | 34px |  |
| `rail.buttonRadius` | `{radius.railButton}` | 8px |  |
| `rail.icon` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `rail.bgOn` | `{color.state.onWindow}` | #d6d6d8 | Active controls on window background: rail and title toggles |
| `rail.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `rail.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |

### sider
| token | Reference | Light value | Description |
|---|---|---|---|
| `sider.width` | `{size.sider}` | 236px |  |
| `sider.bg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `sider.radius` | `{radius.card}` | 10px |  |
| `sider.shadow` | `{shadow.card}` | 0px 0px 0px 0.5px rgba(0,0,0,0.06) | Outer card border |
| `sider.marginRight` | `{space.8}` | 8px |  |
| `sider.headerPaddingTop` | `{space.16}` | 16px |  |
| `sider.headerPaddingRight` | `{space.12}` | 12px |  |
| `sider.headerPaddingBottom` | `{space.10}` | 10px |  |
| `sider.headerPaddingLeft` | `{space.16}` | 16px |  |
| `sider.titleSize` | `{font.size.bodyLg}` | 13px | Hostnames and headings |
| `sider.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `sider.addButtonSize` | `{size.controlMd}` | 26px |  |
| `sider.addButtonRadius` | `{radius.controlSm}` | 6px |  |
| `sider.addButtonBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `sider.addButtonIcon` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `sider.listPaddingX` | `{space.8}` | 8px |  |
| `sider.listPaddingY` | `{space.2}` | 2px |  |
| `sider.footerBorder` | `{color.border.subtle}` | #f0f0f2 | Inner dividers: sidebar footer, status bars, progress tracks |
| `sider.footerPaddingX` | `{space.16}` | 16px |  |
| `sider.footerPaddingY` | `{space.10}` | 10px |  |
| `sider.footerText` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `sider.footerSize` | `{font.size.small}` | 12px | Sidebar footer and secondary host information |

### search
| token | Reference | Light value | Description |
|---|---|---|---|
| `search.height` | `{size.controlLg}` | 30px |  |
| `search.radius` | `{radius.control}` | 7px |  |
| `search.bg` | `{color.surface.sunken}` | #f4f4f5 | Search field |
| `search.paddingX` | `{space.9}` | 9px |  |
| `search.gap` | `{space.6}` | 6px |  |
| `search.marginX` | `{space.12}` | 12px |  |
| `search.marginBottom` | `{space.8}` | 8px |  |
| `search.text` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `search.fontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `search.kbdSize` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `search.kbdText` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `search.kbdBorder` | `{color.border.keycap}` | #e3e3e6 | Keycap border |
| `search.kbdBg` | `{color.surface.keycap}` | #ffffff | Keycap background |
| `search.kbdRadius` | `{radius.keycap}` | 4px |  |
| `search.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `search.easing` | `{motion.easing.search}` | cubic-bezier(0.42,0,0.58,1) |  |
| `search.lineHeight` | `{font.lineHeight.input}` | 18px | Align empty-input and Chinese-placeholder line heights across font fallback |

### groupHeader
| token | Reference | Light value | Description |
|---|---|---|---|
| `groupHeader.height` | `{size.groupRow}` | 30px |  |
| `groupHeader.radius` | `{radius.control}` | 7px |  |
| `groupHeader.paddingX` | `{space.8}` | 8px |  |
| `groupHeader.paddingTop` | `{space.12}` | 12px |  |
| `groupHeader.paddingBottom` | `{space.5}` | 5px |  |
| `groupHeader.gap` | `{space.4}` | 4px |  |
| `groupHeader.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `groupHeader.weight` | `{font.weight.medium}` | 500 |  |
| `groupHeader.text` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `groupHeader.count` | `{color.text.quaternary}` | #b0b0b5 | Group counter |
| `groupHeader.countSize` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `groupHeader.dropBg` | `{color.accent.tint08}` | rgba(10,132,255,0.08) | Group drop highlight |
| `groupHeader.dropText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `groupHeader.dropCount` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `groupHeader.expandTrack` | `{color.accent.tint15}` | rgba(10,132,255,0.15) | Progress track during auto-expansion |
| `groupHeader.expandFill` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `groupHeader.expandHeight` | `{size.progressThin}` | 2px |  |
| `groupHeader.expandDelay` | `{motion.duration.autoExpand}` | 500ms | Expand automatically on hover |
| `groupHeader.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `groupHeader.countMotion` | `{motion.spring.content}` | duration 350ms · bounce 0 | Numeric content change |

### siderItem
| token | Reference | Light value | Description |
|---|---|---|---|
| `siderItem.paddingTop` | `{space.6}` | 6px |  |
| `siderItem.paddingRight` | `{space.6}` | 6px |  |
| `siderItem.paddingBottom` | `{space.6}` | 6px |  |
| `siderItem.paddingLeft` | `{space.8}` | 8px |  |
| `siderItem.gap` | `{space.9}` | 9px |  |
| `siderItem.radius` | `{radius.control}` | 7px |  |
| `siderItem.spacing` | `{space.1}` | 1px |  |
| `siderItem.bgHover` | `{color.state.hover}` | #f7f7f8 | Row hover |
| `siderItem.bgActive` | `{color.state.selectedAccent}` | #e8f2fe | **derived** Added catalog focus background, independent of open sessions |
| `siderItem.bgSelected` | `{color.state.selected}` | #efeff1 | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.nameSize` | `{font.size.bodyLg}` | 13px | Hostnames and headings |
| `siderItem.nameWeight` | `{font.weight.medium}` | 500 |  |
| `siderItem.nameLineHeight` | `{font.lineHeight.name}` | 17px |  |
| `siderItem.nameColor` | `{color.text.primary}` | #1d1d1f |  |
| `siderItem.subSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `siderItem.subFamily` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `siderItem.subLineHeight` | `{font.lineHeight.sub}` | 15px |  |
| `siderItem.subTracking` | `{font.tracking.mono}` | -0.2px |  |
| `siderItem.subColor` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `siderItem.dotSize` | `{size.dot}` | 7px | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.dotHalo` | `{size.dotHalo}` | 3px | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.dotOnline` | `{color.success.default}` | #30c45a | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.dotOnlineHalo` | `{color.success.halo}` | rgba(48,196,90,0.14) | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.dotOffline` | `{color.status.offline}` | #cfcfd4 | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.dotConnecting` | `{color.warning.default}` | #f5a623 | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.dotConnectingHalo` | `{color.warning.halo}` | rgba(245,166,35,0.18) | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.connectingText` | `{color.warning.text}` | #c7861a | **deprecated** Sidebar no longer shows connection/selection state |
| `siderItem.moreSize` | `{size.controlSm}` | 24px |  |
| `siderItem.moreRadius` | `{radius.controlSm}` | 6px |  |
| `siderItem.moreBg` | `{color.surface.controlStrong}` | #ececef | Inline sidebar More button |
| `siderItem.moreIcon` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `siderItem.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |

### osLogo
| token | Reference | Light value | Description |
|---|---|---|---|
| `osLogo.size` | `{size.osLogo}` | 26px |  |
| `osLogo.radius` | `{radius.control}` | 7px |  |
| `osLogo.glyphSize` | `{size.osGlyph}` | 15px |  |
| `osLogo.tintMix` | `{opacity.osTint}` | 0.09 | OS logo background: 9% brand color mixed into card |
| `osLogo.bgBase` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `osLogo.bgSelected` | `{color.surface.card}` | #ffffff | **deprecated** Sidebar no longer shows connection/selection state |
| `osLogo.shadowSelected` | `{shadow.osSelected}` | 0px 0px 0px 0.5px rgba(0,0,0,0.08) | **deprecated** Sidebar no longer shows connection/selection state |

### card
| token | Reference | Light value | Description |
|---|---|---|---|
| `card.bg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `card.radius` | `{radius.card}` | 10px |  |
| `card.shadow` | `{shadow.cardInset}` | inset 0px 0px 0px 0.5px rgba(0,0,0,0.06) | UIKit inner card border without changing outer geometry |

### pane
| token | Reference | Light value | Description |
|---|---|---|---|
| `pane.bg` | `{color.surface.terminal}` | #fbfbfc | Terminal card container background, excluding terminal text colors |
| `pane.radius` | `{radius.pane}` | 10px |  |
| `pane.outline` | `{shadow.paneOutline}` | inset 0px 0px 0px 0.5px rgba(0,0,0,0.08) | Pane inner border drawn after clipping |
| `pane.gap` | `{space.8}` | 8px |  |
| `pane.headerHeight` | `{size.paneHeader}` | 36px |  |
| `pane.headerPaddingLeft` | `{space.14}` | 14px |  |
| `pane.headerPaddingRight` | `{space.8}` | 8px |  |
| `pane.headerGap` | `{space.7}` | 7px |  |
| `pane.headerFontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `pane.name` | `{color.text.primary}` | #1d1d1f |  |
| `pane.nameWeight` | `{font.weight.semibold}` | 600 |  |
| `pane.host` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `pane.hostSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `pane.hostFamily` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `pane.buttonSize` | `{size.controlSm}` | 24px |  |
| `pane.buttonIcon` | `{color.icon.subtle}` | #8e8e93 |  |
| `pane.iconSize` | `{size.iconSm}` | 14px |  |
| `pane.frameInterval` | `{motion.duration.frameInterval}` | 16ms |  |
| `pane.emptyIconSize` | `{size.emptyPaneIcon}` | 32px |  |
| `pane.emptyTitleSize` | `{size.emptyPaneTitleSize}` | 15px |  |
| `pane.emptyTitleWeight` | `{font.weight.emptyPaneTitle}` | 550 |  |
| `pane.contentTop` | `{space.10}` | 10px |  |
| `pane.contentSide` | `{space.8}` | 8px |  |
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
| token | Reference | Light value | Description |
|---|---|---|---|
| `paneFocus.ringFocused` | `{shadow.focusRing}` | 0px 0px 0px 1.5px #0a84ff | Focused pane: 1.5px blue border only |
| `paneFocus.ringHover` | `{shadow.hoverRing}` | 0px 0px 0px 1px rgba(0,0,0,0.14) | Unfocused pane hover |
| `paneFocus.nameFocused` | `{color.text.primary}` | #1d1d1f |  |
| `paneFocus.nameWeightFocused` | `{font.weight.semibold}` | 600 |  |
| `paneFocus.nameUnfocused` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `paneFocus.nameWeightUnfocused` | `{font.weight.medium}` | 500 |  |
| `paneFocus.hostUnfocused` | `{color.text.disabled}` | #aeaeb2 | Unfocused pane host |
| `paneFocus.contentOpacityUnfocused` | `{opacity.paneUnfocusedContent}` | 0.5 |  |
| `paneFocus.contentSaturateUnfocused` | `{opacity.paneUnfocusedSaturate}` | 0.35 |  |
| `paneFocus.statusOpacityUnfocused` | `{opacity.paneUnfocusedStatus}` | 0.55 |  |
| `paneFocus.contentOpacityHover` | `{opacity.paneHoverContent}` | 0.78 |  |
| `paneFocus.contentSaturateHover` | `{opacity.paneHoverSaturate}` | 0.7 |  |
| `paneFocus.buttonsOpacityUnfocused` | `{opacity.hidden}` | 0 |  |
| `paneFocus.cursorRingUnfocused` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `paneFocus.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `paneFocus.kbdSize` | `{font.size.caption2}` | 10.5px | ⌘N chip and count capsules |
| `paneFocus.kbdWeight` | `{font.weight.medium}` | 500 |  |
| `paneFocus.kbdRadius` | `{radius.chip}` | 5px |  |
| `paneFocus.kbdPaddingX` | `{space.5}` | 5px |  |
| `paneFocus.kbdPaddingY` | `{space.3}` | 3px |  |
| `paneFocus.kbdBg` | `{color.fill.quiet}` | rgba(0,0,0,0.05) | ⌘N chip and badges on glass |
| `paneFocus.kbdText` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `paneFocus.kbdBgFocused` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `paneFocus.kbdTextFocused` | `{color.text.onAccent}` | #ffffff |  |
| `paneFocus.kbdLineHeight` | `{font.lineHeight.keycap}` | 12px |  |

### activityBadge
| token | Reference | Light value | Description |
|---|---|---|---|
| `activityBadge.fontSize` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `activityBadge.radius` | `{radius.badge}` | 9px |  |
| `activityBadge.paddingLeft` | `{space.6}` | 6px |  |
| `activityBadge.paddingRight` | `{space.7}` | 7px |  |
| `activityBadge.paddingY` | `{space.2}` | 2px |  |
| `activityBadge.gap` | `{space.4}` | 4px |  |
| `activityBadge.dotSize` | `{size.dotSm}` | 6px |  |
| `activityBadge.outputText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `activityBadge.outputBg` | `{color.accent.tint10}` | rgba(10,132,255,0.1) | New-output badge / flash |
| `activityBadge.outputDot` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `activityBadge.bellText` | `{color.warning.text}` | #c7861a | Connecting text / bell |
| `activityBadge.bellBg` | `{color.warning.tint14}` | rgba(245,166,35,0.14) | Bell badge background |
| `activityBadge.bellIconSize` | `{size.bellIcon}` | 13px |  |

### fadeBar
| token | Reference | Light value | Description |
|---|---|---|---|
| `fadeBar.tint` | `{color.surface.terminal}` | #fbfbfc | Terminal card container background, excluding terminal text colors |
| `fadeBar.extent` | `{size.fadeExtent}` | 24px | Glass-bar fade extending into content |
| `fadeBar.extentPane` | `{size.fadeExtentPane}` | 22px | Fade length inside split panes |
| `fadeBar.blur1` | `{glass.fade.blur1}` | 1px |  |
| `fadeBar.blur2` | `{glass.fade.blur2}` | 3px |  |
| `fadeBar.blur3` | `{glass.fade.blur3}` | 5px |  |
| `fadeBar.mask1Solid` | `{glass.fade.mask1Solid}` | 0.45 | Layer 1 opaque through 45%, transparent at 100% |
| `fadeBar.mask1End` | `{glass.fade.mask1End}` | 1 |  |
| `fadeBar.mask2Solid` | `{glass.fade.mask2Solid}` | 0.4 |  |
| `fadeBar.mask2End` | `{glass.fade.mask2End}` | 0.78 |  |
| `fadeBar.mask3Solid` | `{glass.fade.mask3Solid}` | 0.3 |  |
| `fadeBar.mask3End` | `{glass.fade.mask3End}` | 0.6 |  |
| `fadeBar.tintStart` | `{glass.fade.tintStart}` | 0.92 | Tint-gradient starting alpha using the containing card color |
| `fadeBar.tintMid` | `{glass.fade.tintMid}` | 0.78 |  |
| `fadeBar.tintMidStop` | `{glass.fade.tintMidStop}` | 0.45 |  |
| `fadeBar.tintEnd` | `{glass.fade.tintEnd}` | 0 |  |

### statusBar
| token | Reference | Light value | Description |
|---|---|---|---|
| `statusBar.height` | `{size.statusBar}` | 34px |  |
| `statusBar.heightPane` | `{size.paneStatusBar}` | 32px |  |
| `statusBar.paddingX` | `{space.14}` | 14px |  |
| `statusBar.gap` | `{space.14}` | 14px |  |
| `statusBar.gapPane` | `{space.10}` | 10px |  |
| `statusBar.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `statusBar.text` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `statusBar.hostFamily` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `statusBar.hostColor` | `{color.text.primary}` | #1d1d1f |  |
| `statusBar.hostWeight` | `{font.weight.semibold}` | 600 |  |
| `statusBar.divider` | `{color.border.control}` | #e6e6e9 | Button borders and vertical status separators |
| `statusBar.dividerOnGlass` | `{color.border.onGlass}` | rgba(0,0,0,0.1) | Vertical glass-bar divider |
| `statusBar.dividerHeight` | `{size.separatorShort}` | 12px |  |

### badge
| token | Reference | Light value | Description |
|---|---|---|---|
| `badge.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `badge.fontSizeCompact` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `badge.paddingX` | `{space.8}` | 8px |  |
| `badge.paddingXCompact` | `{space.7}` | 7px |  |
| `badge.paddingY` | `{space.2}` | 2px |  |
| `badge.paddingYCompact` | `{space.1}` | 1px |  |
| `badge.radius` | `{radius.pill}` | 20px |  |
| `badge.gap` | `{space.5}` | 5px |  |
| `badge.neutralBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `badge.neutralText` | `{color.text.neutralBadge}` | #5b5b62 | Neutral badge text |
| `badge.neutralBgOnGlass` | `{color.fill.quiet}` | rgba(0,0,0,0.05) | ⌘N chip and badges on glass |
| `badge.okBg` | `{color.success.subtle}` | #ebf8ef |  |
| `badge.okText` | `{color.success.text}` | #1f8a3f |  |
| `badge.okDot` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `badge.okBgOnGlass` | `{color.success.onGlass}` | rgba(48,196,90,0.15) |  |
| `badge.runBg` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `badge.runText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `badge.heightSm` | `{size.badgeSmall}` | 18px |  |
| `badge.heightMd` | `{size.badgeMedium}` | 20px |  |
| `badge.warnBg` | `{color.warning.subtle}` | #fdf3e3 |  |
| `badge.warnText` | `{color.warning.textStrong}` | #b26a00 |  |
| `badge.warnDot` | `{color.warning.default}` | #f5a623 | Connecting |
| `badge.errBg` | `{color.danger.subtle}` | #fdecec | Failure icon and badge backgrounds |
| `badge.errText` | `{color.danger.text}` | #d70015 | Failure text |
| `badge.errDot` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |
| `badge.runDot` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `badge.neutralDot` | `{color.status.offline}` | #cfcfd4 | Offline dot |
| `badge.mutedBg` | `{color.surface.field}` | #f6f6f7 | Breadcrumb path field |
| `badge.mutedText` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |

### button
| token | Reference | Light value | Description |
|---|---|---|---|
| `button.height` | `{size.control}` | 28px |  |
| `button.radius` | `{radius.control}` | 7px |  |
| `button.paddingX` | `{space.9}` | 9px |  |
| `button.gap` | `{space.6}` | 6px |  |
| `button.fontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `button.text` | `{color.text.strong}` | #3a3a3f | Secondary button text |
| `button.border` | `{color.border.control}` | #e6e6e9 | Button borders and vertical status separators |
| `button.borderWidth` | `{size.divider}` | 1px |  |
| `button.primaryBg` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `button.primaryText` | `{color.text.onAccent}` | #ffffff |  |
| `button.bgHover` | `{color.state.hover}` | #f7f7f8 | **derived** Not specified in design |
| `button.bgPressed` | `{color.state.pressed}` | #e6e6e9 | **derived** Not specified in design |
| `button.bg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `button.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `button.heightSm` | `{size.controlSm}` | 24px |  |
| `button.heightLg` | `{size.buttonLarge}` | 32px |  |
| `button.paddingXSm` | `{space.9}` | 9px |  |
| `button.paddingXLg` | `{space.14}` | 14px |  |
| `button.secondaryBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `button.secondaryBgHover` | `{color.surface.controlStrong}` | #ececef | Inline sidebar More button |
| `button.plainBgHover` | `{color.state.hover}` | #f7f7f8 | Row hover |
| `button.primaryBgHover` | `{color.accent.hover}` | #0a6fd6 |  |
| `button.primaryBgPressed` | `{color.accent.pressed}` | #0a5bc4 |  |
| `button.dangerBg` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |
| `button.dangerBgHover` | `{color.danger.hover}` | #ff3b30 |  |
| `button.dangerBgPressed` | `{color.danger.pressed}` | #ff3b30 |  |
| `button.dangerText` | `{color.text.onAccent}` | #ffffff |  |
| `button.dangerPlainText` | `{color.danger.text}` | #d70015 | Failure text |
| `button.dangerPlainBgHover` | `{color.danger.subtle}` | #fdecec | Failure icon and badge backgrounds |
| `button.kbdGap` | `{space.6}` | 6px |  |
| `button.focusShadow` | `{shadow.controlFocus}` | 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 5.5px rgba(10,132,255,0.16) |  |

### iconButton
| token | Reference | Light value | Description |
|---|---|---|---|
| `iconButton.size` | `{size.controlMd}` | 26px |  |
| `iconButton.sizeSm` | `{size.controlSm}` | 24px |  |
| `iconButton.sizeXs` | `{size.controlXs}` | 22px |  |
| `iconButton.radius` | `{radius.controlSm}` | 6px |  |
| `iconButton.icon` | `{color.icon.default}` | #6e6e73 |  |
| `iconButton.iconSubtle` | `{color.icon.subtle}` | #8e8e93 |  |
| `iconButton.iconFaint` | `{color.icon.faint}` | #a1a1a6 |  |
| `iconButton.bg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `iconButton.bgHover` | `{color.state.hover}` | #f7f7f8 | **derived** Not specified in design |
| `iconButton.iconSize` | `{size.icon}` | 16px |  |
| `iconButton.stroke` | `{size.iconStroke}` | 1.75px | Lucide-style stroke width |
| `iconButton.bgPressed` | `{color.state.pressed}` | #e6e6e9 | Pressed state, absent from design |
| `iconButton.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `iconButton.bgOn` | `{color.state.pressed}` | #e6e6e9 | Pressed state, absent from design |
| `iconButton.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `iconButton.bgFilled` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `iconButton.iconHover` | `{color.text.primary}` | #1d1d1f |  |
| `iconButton.iconSizeSm` | `{size.iconSm}` | 14px |  |
| `iconButton.iconSizeXs` | `{size.iconXs}` | 12px |  |
| `iconButton.focusShadow` | `{shadow.controlFocus}` | 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 5.5px rgba(10,132,255,0.16) |  |

### floatButton
| token | Reference | Light value | Description |
|---|---|---|---|
| `floatButton.size` | `{size.controlLg}` | 30px |  |
| `floatButton.bg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `floatButton.shadow` | `{shadow.floatButton}` | 0px 2px 8px 0px rgba(0,0,0,0.1), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | FTP transfer button between panes |
| `floatButton.icon` | `{color.text.strong}` | #3a3a3f | Secondary button text |
| `floatButton.primaryBg` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `floatButton.primaryIcon` | `{color.text.onAccent}` | #ffffff |  |

### segmented
| token | Reference | Light value | Description |
|---|---|---|---|
| `segmented.bg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `segmented.radius` | `{radius.control}` | 7px |  |
| `segmented.padding` | `{space.2}` | 2px |  |
| `segmented.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `segmented.itemPaddingX` | `{space.9}` | 9px |  |
| `segmented.itemPaddingY` | `{space.2}` | 2px |  |
| `segmented.itemRadius` | `{radius.chip}` | 5px |  |
| `segmented.text` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `segmented.bgOn` | `{color.surface.raised}` | #ffffff | Raised surfaces: dragged cards, ghosts, selected segments |
| `segmented.textOn` | `{color.text.primary}` | #1d1d1f |  |
| `segmented.shadowOn` | `{shadow.segment}` | 0px 1px 2px 0px rgba(0,0,0,0.08) |  |
| `segmented.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `segmented.bgHover` | `{color.state.hover}` | #f7f7f8 | Row hover |
| `segmented.countText` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `segmented.countTextOn` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `segmented.itemHeightMd` | `{size.segmentedItem}` | 24px |  |
| `segmented.heightSm` | `{size.controlSm}` | 24px |  |

### breadcrumb
| token | Reference | Light value | Description |
|---|---|---|---|
| `breadcrumb.height` | `{size.controlMd}` | 26px |  |
| `breadcrumb.radius` | `{radius.controlSm}` | 6px |  |
| `breadcrumb.bg` | `{color.surface.field}` | #f6f6f7 | Breadcrumb path field |
| `breadcrumb.bgOnGlass` | `{color.fill.quieter}` | rgba(0,0,0,0.04) | Breadcrumbs on glass |
| `breadcrumb.paddingX` | `{space.8}` | 8px |  |
| `breadcrumb.gap` | `{space.3}` | 3px |  |
| `breadcrumb.text` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `breadcrumb.current` | `{color.text.primary}` | #1d1d1f |  |
| `breadcrumb.separator` | `{color.text.placeholder}` | #c7c7cc | Breadcrumb separator |
| `breadcrumb.iconSize` | `{size.iconXs}` | 12px |  |

### fileList
| token | Reference | Light value | Description |
|---|---|---|---|
| `fileList.headerHeight` | `{size.listRow}` | 30px |  |
| `fileList.headerText` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `fileList.headerSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `fileList.headerBorder` | `{color.border.divider}` | #f2f2f4 | Header and table-header bottom lines |
| `fileList.rowHeight` | `{size.listRow}` | 30px |  |
| `fileList.rowRadius` | `{radius.controlSm}` | 6px |  |
| `fileList.rowPaddingX` | `{space.8}` | 8px |  |
| `fileList.rowGap` | `{space.2}` | 2px |  |
| `fileList.fontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `fileList.zebra` | `{color.surface.zebra}` | #fafafb | Alternating list rows |
| `fileList.bgSelected` | `{color.state.selectedAccent}` | #e8f2fe | Selected file row |
| `fileList.textSelected` | `{color.text.accentStrong}` | #0a5bc4 | Selected filename |
| `fileList.meta` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `fileList.fileIcon` | `{color.icon.subtle}` | #8e8e93 |  |
| `fileList.folderIcon` | `{color.icon.folder}` | #5ba7f7 |  |
| `fileList.folderFill` | `{color.icon.folderFill}` | #d7e9fd |  |
| `fileList.paneHeaderHeight` | `{size.fileHeader}` | 40px |  |
| `fileList.headerBg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `fileList.columnGap` | `{space.16}` | 16px |  |
| `fileList.nameMinWidth` | `{size.tableNameMinWidth}` | 160px |  |

### progress
| token | Reference | Light value | Description |
|---|---|---|---|
| `progress.height` | `{size.progress}` | 5px |  |
| `progress.radius` | `{radius.bar}` | 3px |  |
| `progress.track` | `{color.border.subtle}` | #f0f0f2 | Inner dividers: sidebar footer, status bars, progress tracks |
| `progress.fill` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `progress.fillNeutral` | `{color.fill.strong}` | #1d1d1f | Neutral progress fill |
| `progress.fillPaused` | `{color.status.paused}` | #c7c7cc | Paused progress |
| `progress.fillDone` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `progress.heightMeter` | `{size.meterHeight}` | 4px |  |
| `progress.meterFill` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `progress.meterWarnAt` | `{size.meterWarnRatio}` | 0.7 |  |
| `progress.meterCritAt` | `{size.meterCriticalRatio}` | 0.9 |  |
| `progress.fillError` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |

### tooltip
| token | Reference | Light value | Description |
|---|---|---|---|
| `tooltip.bg` | `{color.tooltip.bg}` | rgba(30,30,32,0.92) |  |
| `tooltip.text` | `{color.tooltip.text}` | #ffffff |  |
| `tooltip.kbd` | `{color.tooltip.kbd}` | rgba(255,255,255,0.6) |  |
| `tooltip.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `tooltip.paddingX` | `{space.8}` | 8px |  |
| `tooltip.paddingY` | `{space.4}` | 4px |  |
| `tooltip.radius` | `{radius.controlSm}` | 6px |  |
| `tooltip.shadow` | `{shadow.tooltip}` | 0px 4px 12px 0px rgba(0,0,0,0.18) |  |
| `tooltip.arrow` | `{space.4}` | 4px |  |
| `tooltip.maxWidth` | `{size.tooltipMaxWidth}` | 320px | Maximum tooltip width |
| `tooltip.offset` | `{space.6}` | 6px |  |
| `tooltip.railOffset` | `{space.8}` | 8px |  |

### menu
| token | Reference | Light value | Description |
|---|---|---|---|
| `menu.bg` | `{color.surface.raised}` | #ffffff | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.radius` | `{radius.lift}` | 9px | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.shadow` | `{shadow.lift}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.itemHeight` | `{size.control}` | 28px | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.itemRadius` | `{radius.controlSm}` | 6px | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.itemPaddingX` | `{space.8}` | 8px | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.itemHover` | `{color.state.hover}` | #f7f7f8 | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.itemHoverText` | `{color.text.primary}` | #1d1d1f | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.fontSize` | `{font.size.body}` | 12.5px | **derived** Menu derived from raised/dragged surfaces, absent from design |
| `menu.separator` | `{color.border.subtle}` | #f0f0f2 | **derived** Menu derived from raised/dragged surfaces, absent from design |

### floatPill
| token | Reference | Light value | Description |
|---|---|---|---|
| `floatPill.bg` | `{color.glass.float}` | rgba(255,255,255,0.7) | Floating scroll-to-bottom capsule |
| `floatPill.blur` | `{glass.float.blur}` | 16px |  |
| `floatPill.saturate` | `{glass.float.saturate}` | 1.5 |  |
| `floatPill.shadow` | `{shadow.float}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 4px 12px 0px rgba(0,0,0,0.08) | Floating capsule |
| `floatPill.radius` | `{radius.pill}` | 20px |  |
| `floatPill.text` | `{color.text.strong}` | #3a3a3f | Secondary button text |
| `floatPill.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `floatPill.paddingLeft` | `{space.8}` | 8px |  |
| `floatPill.paddingRight` | `{space.10}` | 10px |  |
| `floatPill.paddingY` | `{space.4}` | 4px |  |

### dragLift
| token | Reference | Light value | Description |
|---|---|---|---|
| `dragLift.threshold` | `{motion.drag.threshold}` | 4px | Movement threshold in px |
| `dragLift.holdDelay` | `{motion.duration.hold}` | 150ms | Pick up after long press |
| `dragLift.scale` | `{motion.drag.liftScale}` | 1.03 |  |
| `dragLift.bg` | `{color.surface.raised}` | #ffffff | Raised surfaces: dragged cards, ghosts, selected segments |
| `dragLift.radius` | `{radius.lift}` | 9px |  |
| `dragLift.shadow` | `{shadow.lift}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | Dragged card |
| `dragLift.stackShadow` | `{shadow.liftStack}` | 0px 0px 0px 0.5px rgba(0,0,0,0.08), 0px 4px 10px 0px rgba(0,0,0,0.06) | Stacked multiselection cards |
| `dragLift.stackOffset` | `{space.5}` | 5px |  |
| `dragLift.stackInset` | `{space.6}` | 6px |  |
| `dragLift.stackBackOpacity` | `{opacity.liftStackBack}` | 0.8 |  |
| `dragLift.liftDuration` | `{motion.duration.lift}` | 120ms | Pick up |
| `dragLift.sourceOpacity` | `{opacity.dragSource}` | 0.4 | Source row after dragging away while collapsed |
| `dragLift.stackBg` | `{color.surface.liftStack}` | #f9f9fa |  |
| `dragLift.standardScale` | `{motion.drag.standardScale}` | 1.02 |  |
| `dragLift.standardRotation` | `{motion.drag.standardRotation}` | -0.6 |  |

### dropIndicator
| token | Reference | Light value | Description |
|---|---|---|---|
| `dropIndicator.placeholderBg` | `{color.fill.placeholder}` | rgba(0,0,0,0.035) | Drag placeholder |
| `dropIndicator.placeholderShadow` | `{shadow.placeholder}` | inset 0px 0px 0px 1px rgba(0,0,0,0.05) |  |
| `dropIndicator.placeholderAccentBg` | `{color.accent.tint06}` | rgba(10,132,255,0.06) | Accepting drop placeholder background |
| `dropIndicator.placeholderAccentShadow` | `{shadow.placeholderAccent}` | inset 0px 0px 0px 1px rgba(10,132,255,0.28) |  |
| `dropIndicator.lineColor` | `{color.accent.default}` | #0a84ff | **derived** MyGo ListState.Reorder insertion line |
| `dropIndicator.lineHeight` | `{size.dropLine}` | 2px | **derived** Same as above |
| `dropIndicator.zoneBorder` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `dropIndicator.zoneBorderWidth` | `{size.focusRing}` | 1.5px | Focus-ring / dashed drop-frame width |
| `dropIndicator.zoneBg` | `{color.accent.tint035}` | rgba(10,132,255,0.035) | Drop-area background |
| `dropIndicator.zoneRadius` | `{radius.dropZone}` | 8px |  |
| `dropIndicator.hintBg` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `dropIndicator.hintText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `dropIndicator.flashBg` | `{color.accent.tint10}` | rgba(10,132,255,0.1) | New-output badge / flash |
| `dropIndicator.flashShadow` | `{shadow.flash}` | inset 0px 0px 0px 1px rgba(10,132,255,0.18) | Drop-flash inner border |
| `dropIndicator.flashDuration` | `{motion.duration.flash}` | 600ms | Drop flash |
| `dropIndicator.flashEasing` | `{motion.easing.out}` | cubic-bezier(0,0,0.58,1) |  |
| `dropIndicator.autoScrollZone` | `{motion.drag.autoScrollZone}` | 48px |  |
| `dropIndicator.autoScrollMax` | `{motion.drag.autoScrollMax}` | 900 | px/s, t² curve |
| `dropIndicator.autoScrollTint` | `{color.accent.tint13}` | rgba(10,132,255,0.13) | Auto-scroll zone gradient |
| `dropIndicator.autoScrollEdge` | `{color.accent.tint50}` | rgba(10,132,255,0.5) | Auto-scroll zone edge |

### dragGhost
| token | Reference | Light value | Description |
|---|---|---|---|
| `dragGhost.bg` | `{color.surface.raised}` | #ffffff | Raised surfaces: dragged cards, ghosts, selected segments |
| `dragGhost.radius` | `{radius.dropZone}` | 8px |  |
| `dragGhost.shadow` | `{shadow.ghost}` | 0px 8px 24px 0px rgba(0,0,0,0.12), 0px 0px 0px 0.5px rgba(0,0,0,0.08) | FTP drag ghost |
| `dragGhost.paddingLeft` | `{space.10}` | 10px |  |
| `dragGhost.paddingRight` | `{space.12}` | 12px |  |
| `dragGhost.paddingY` | `{space.7}` | 7px |  |
| `dragGhost.fontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `dragGhost.countBg` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `dragGhost.countText` | `{color.text.onAccent}` | #ffffff |  |
| `dragGhost.countSize` | `{font.size.caption2}` | 10.5px | ⌘N chip and count capsules |
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
| token | Reference | Light value | Description |
|---|---|---|---|
| `reorderMotion.displace` | `{motion.spring.displace}` | stiffness 420 · damping 34 | Displacement (approximately Snappy, 220ms) |
| `reorderMotion.drop` | `{motion.spring.drop}` | duration 320ms · bounce 0.15 | Drop |
| `reorderMotion.cancel` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `reorderMotion.groupCollapse` | `{motion.duration.collapse}` | 180ms | Collapse all groups while dragging a group |
| `reorderMotion.groupRestore` | `{motion.duration.restore}` | 220ms | Restore expansion |
| `reorderMotion.tabDisplace` | `{motion.spring.displace}` | stiffness 420 · damping 34 | **derived** Tab/pane drag reuses sidebar parameters where not separately designed |
| `reorderMotion.paneDisplace` | `{motion.spring.displace}` | stiffness 420 · damping 34 | **derived** Same as above |
| `reorderMotion.paneDrop` | `{motion.spring.drop}` | duration 320ms · bounce 0.15 | **derived** Same as above |

### desktop
| token | Reference | Light value | Description |
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
| `desktop.dividerHandleRadius` | `{size.focusRing}` | 1.5px | Focus-ring / dashed drop-frame width |
| `desktop.trafficLightX` | `{space.12}` | 12px |  |
| `desktop.siderKeyboardStep` | `{space.10}` | 10px |  |
| `desktop.railHover` | `{color.state.railHover}` | rgba(0,0,0,0.045) |  |
| `desktop.railPressed` | `{color.state.railPressed}` | rgba(0,0,0,0.12) |  |
| `desktop.danger` | `{color.status.danger}` | #dc2626 |  |

### scrollbar
| token | Reference | Light value | Description |
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
| `scrollbar.gestureGap` | `{motion.duration.scrollGesture}` | 300ms | Maximum gap between events in one scroll gesture |

### dialog
| token | Reference | Light value | Description |
|---|---|---|---|
| `dialog.hostWidth` | `{size.hostDialogWidth}` | 340px |  |
| `dialog.groupWidth` | `{size.groupDialogWidth}` | 320px |  |
| `dialog.portWidth` | `{size.portFieldWidth}` | 90px |  |
| `dialog.titleSize` | `{size.dialogTitleSize}` | 18px |  |
| `dialog.bg` | `{color.surface.raised}` | #ffffff | Raised surfaces: dragged cards, ghosts, selected segments |
| `dialog.backdrop` | `{color.overlay.backdrop}` | rgba(0,0,0,0.4) | Dialog backdrop |
| `dialog.shadow` | `{shadow.dialog}` | 0px 10px 30px 0px rgba(0,0,0,0.3) | Dialog shadow |
| `dialog.radius` | `{radius.card}` | 10px |  |
| `dialog.padding` | `{space.20}` | 20px |  |
| `dialog.gap` | `{space.12}` | 12px |  |
| `dialog.viewportInset` | `{space.20}` | 20px |  |

### stats
| token | Reference | Light value | Description |
|---|---|---|---|
| `stats.headerHeight` | `{size.statsHeaderHeight}` | 44px |  |
| `stats.chartHeight` | `{size.statsChartHeight}` | 44px | CPU trend chart |
| `stats.chartStroke` | `{size.statsChartStroke}` | 1.3px |  |
| `stats.sectionGap` | `{size.statsSectionGap}` | 18px |  |
| `stats.netChartHeight` | `{size.statsNetChartHeight}` | 20px | Upload/download mini trends |
| `stats.valueLarge` | `{size.statsValueLarge}` | 28px | Main CPU value |
| `stats.hostBg` | `{color.surface.sunken}` | #f4f4f5 | Search field |
| `stats.hostRadius` | `{radius.card}` | 10px |  |
| `stats.processBar` | `{color.accent.tint10}` | rgba(10,132,255,0.1) | New-output badge / flash |
| `stats.processRadius` | `{radius.controlSm}` | 6px |  |
| `stats.skeleton` | `{color.border.subtle}` | #f0f0f2 | Inner dividers: sidebar footer, status bars, progress tracks |
| `stats.shimmer` | `{color.fill.shimmer}` | #fafafb | Skeleton shimmer highlight |
| `stats.shimmerPeriod` | `{motion.duration.shimmer}` | 1400ms | One skeleton shimmer cycle |
| `stats.skeletonLine` | `{size.statsSkeletonLine}` | 10px | Top-process skeleton bars |
| `stats.beatDuration` | `{motion.duration.liveBeat}` | 900ms | Live status pulse per sample |
| `stats.beatScale` | `{opacity.liveBeatScale}` | 1.35 | Initial scale of the live status pulse |
| `stats.beatEasing` | `{motion.easing.out}` | cubic-bezier(0,0,0.58,1) |  |

### catalog
| token | Reference | Light value | Description |
|---|---|---|---|
| `catalog.rowHeight` | `{size.catalogRowHeight}` | 40px |  |
| `catalog.listPaddingX` | `{space.8}` | 8px |  |
| `catalog.listPaddingY` | `{space.2}` | 2px |  |
| `catalog.rowGap` | `{space.2}` | 2px |  |
| `catalog.groupGap` | `{space.10}` | 10px |  |

### protocolIcon
| token | Reference | Light value | Description |
|---|---|---|---|
| `protocolIcon.size` | `{size.osLogo}` | 26px |  |
| `protocolIcon.radius` | `{radius.control}` | 7px |  |
| `protocolIcon.glyph` | `{size.osGlyph}` | 15px |  |
| `protocolIcon.sftpBg` | `{color.protocol.sftpBg}` | #eaf3fe | SFTP icon tile background |
| `protocolIcon.sftpGlyph` | `{color.protocol.sftpGlyph}` | #0a84ff | SFTP glyph |
| `protocolIcon.ftpBg` | `{color.protocol.ftpBg}` | #f2f2f4 | FTP icon tile background |
| `protocolIcon.ftpGlyph` | `{color.protocol.ftpGlyph}` | #8e8e93 | FTP glyph |

### connectionIndicator
| token | Reference | Light value | Description |
|---|---|---|---|
| `connectionIndicator.offline` | `{color.status.offline}` | #cfcfd4 | Offline dot |
| `connectionIndicator.connected` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `connectionIndicator.connectedHalo` | `{color.success.halo}` | rgba(48,196,90,0.14) | 3px online-dot halo |
| `connectionIndicator.connecting` | `{color.warning.default}` | #f5a623 | Connecting |
| `connectionIndicator.connectingHalo` | `{color.warning.halo}` | rgba(245,166,35,0.18) |  |
| `connectionIndicator.halo` | `{size.dotHalo}` | 3px |  |
| `connectionIndicator.stageDuration` | `{motion.duration.connectionStage}` | 300ms | Duration of each demo SSH connection phase |

### paneToggle
| token | Reference | Light value | Description |
|---|---|---|---|
| `paneToggle.size` | `{size.controlMd}` | 26px |  |
| `paneToggle.radius` | `{radius.controlSm}` | 6px |  |
| `paneToggle.icon` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `paneToggle.bgHover` | `{color.state.hover}` | #f7f7f8 | Row hover |
| `paneToggle.bgOn` | `{color.state.pressed}` | #e6e6e9 | Pressed state, absent from design |
| `paneToggle.iconOn` | `{color.text.primary}` | #1d1d1f |  |
| `paneToggle.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |

### filePaneFocus
| token | Reference | Light value | Description |
|---|---|---|---|
| `filePaneFocus.labelBgActive` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `filePaneFocus.labelTextActive` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `filePaneFocus.labelWeightActive` | `{font.weight.semibold}` | 600 |  |
| `filePaneFocus.labelTextInactive` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `filePaneFocus.labelWeightInactive` | `{font.weight.medium}` | 500 |  |
| `filePaneFocus.labelPaddingX` | `{space.8}` | 8px |  |
| `filePaneFocus.labelRadius` | `{radius.controlSm}` | 6px |  |
| `filePaneFocus.selBgActive` | `{color.state.selectedAccent}` | #e8f2fe | Selected file row |
| `filePaneFocus.selTextActive` | `{color.text.accentStrong}` | #0a5bc4 | Selected filename |
| `filePaneFocus.selBgInactive` | `{color.state.selected}` | #efeff1 | Selected sidebar row |
| `filePaneFocus.selTextInactive` | `{color.text.primary}` | #1d1d1f |  |
| `filePaneFocus.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `filePaneFocus.borderFocused` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `filePaneFocus.borderWidthFocused` | `{size.focusRing}` | 1.5px | Focus-ring / dashed drop-frame width |

### FTPSplit
| token | Reference | Light value | Description |
|---|---|---|---|
| `FTPSplit.gutter` | `{space.8}` | 8px |  |
| `FTPSplit.expand` | `{motion.spring.displace}` | stiffness 420 · damping 34 | Displacement (approximately Snappy, 220ms) |
| `FTPSplit.revealOpacityFrom` | `{opacity.revealFrom}` | 0.6 | Initial opacity when expanding the local pane |
| `FTPSplit.crossfade` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `FTPSplit.minLocal` | `{size.paneMinLocal}` | 320px |  |
| `FTPSplit.minRemote` | `{size.paneMinRemote}` | 420px |  |
| `FTPSplit.defaultRatio` | `{opacity.splitDefaultRatio}` | 0.5 |  |
| `FTPSplit.gripW` | `{space.4}` | 4px |  |
| `FTPSplit.gripH` | `{size.splitGripHeight}` | 32px |  |
| `FTPSplit.grip` | `{color.text.placeholder}` | #c7c7cc | Breadcrumb separator |
| `FTPSplit.gripActive` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |

### transferCapsule
| token | Reference | Light value | Description |
|---|---|---|---|
| `transferCapsule.height` | `{size.controlXs}` | 22px |  |
| `transferCapsule.radius` | `{radius.pill}` | 20px |  |
| `transferCapsule.bg` | `{color.fill.quiet}` | rgba(0,0,0,0.05) | ⌘N chip and badges on glass |
| `transferCapsule.bgHover` | `{color.fill.quietHover}` | rgba(0,0,0,0.08) | Transfer capsule hover |
| `transferCapsule.bgOpen` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `transferCapsule.textOpen` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `transferCapsule.text` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `transferCapsule.arrow` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `transferCapsule.barWidth` | `{size.capsuleBarWidth}` | 32px |  |
| `transferCapsule.barHeight` | `{size.capsuleBarHeight}` | 3px |  |
| `transferCapsule.barTrack` | `{color.accent.tint15}` | rgba(10,132,255,0.15) | Progress track during auto-expansion |
| `transferCapsule.barFill` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `transferCapsule.doneArrow` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `transferCapsule.doneFill` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `transferCapsule.error` | `{color.danger.text}` | #d70015 | Failure text |
| `transferCapsule.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `transferCapsule.exit` | `{motion.duration.exit}` | 120ms | Overlay exit and capsule fade |
| `transferCapsule.enterScale` | `{opacity.capsuleEnterScale}` | 0.9 |  |
| `transferCapsule.paddingLeft` | `{space.7}` | 7px |  |
| `transferCapsule.paddingRight` | `{space.9}` | 9px |  |
| `transferCapsule.gap` | `{space.6}` | 6px |  |
| `transferCapsule.paddingX` | `{space.8}` | 8px |  |
| `transferCapsule.paddingY` | `{space.2}` | 2px |  |
| `transferCapsule.fontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `transferCapsule.weight` | `{font.weight.medium}` | 500 |  |
| `transferCapsule.speedInterval` | `{motion.duration.autoExpand}` | 500ms | Expand automatically on hover |

### transferPopover
| token | Reference | Light value | Description |
|---|---|---|---|
| `transferPopover.width` | `{size.popover}` | 384px |  |
| `transferPopover.radius` | `{radius.popover}` | 12px |  |
| `transferPopover.bg` | `{color.glass.float}` | rgba(255,255,255,0.7) | Floating scroll-to-bottom capsule |
| `transferPopover.blur` | `{glass.float.blur}` | 16px |  |
| `transferPopover.saturate` | `{glass.float.saturate}` | 1.5 |  |
| `transferPopover.shadow` | `{shadow.popover}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.12), 0px 2px 6px 0px rgba(0,0,0,0.06), inset 0px 0.5px 0px 0px rgba(255,255,255,0.9) | Transfer overlay |
| `transferPopover.offset` | `{space.8}` | 8px |  |
| `transferPopover.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `transferPopover.exit` | `{motion.duration.exit}` | 120ms | Overlay exit and capsule fade |
| `transferPopover.enterScale` | `{opacity.popoverEnterScale}` | 0.96 |  |
| `transferPopover.enterY` | `{space.6}` | 6px |  |
| `transferPopover.padding` | `{space.6}` | 6px |  |
| `transferPopover.headerHeight` | `{size.transferPopoverHeader}` | 32px |  |
| `transferPopover.maxRows` | `{size.transferPopoverMaxRows}` | 6 |  |
| `transferPopover.dismissY` | `{space.24}` | 24px |  |
| `transferPopover.rowHeight` | `{size.queueRow}` | 56px |  |
| `transferPopover.exitY` | `{space.24}` | 24px |  |

### transferDock
| token | Reference | Light value | Description |
|---|---|---|---|
| `transferDock.bg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `transferDock.radius` | `{radius.card}` | 10px |  |
| `transferDock.headerHeight` | `{size.dockHeader}` | 42px |  |
| `transferDock.rowHeight` | `{size.dockRow}` | 50px |  |
| `transferDock.maxRows` | `{size.transferDockMaxRows}` | 5 |  |
| `transferDock.grow` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `transferDock.collapse` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `transferDock.concurrency` | `{size.transferConcurrency}` | 3 |  |
| `transferDock.progressWidth` | `{size.transferProgressWidth}` | 180px |  |
| `transferDock.compactBreakpoint` | `{size.fileCompactWidth}` | 720px |  |

### transferRow
| token | Reference | Light value | Description |
|---|---|---|---|
| `transferRow.upBg` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `transferRow.upIcon` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `transferRow.downBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `transferRow.downIcon` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `transferRow.doneBg` | `{color.success.subtle}` | #ebf8ef |  |
| `transferRow.doneIcon` | `{color.success.text}` | #1f8a3f |  |
| `transferRow.errBg` | `{color.danger.subtle}` | #fdecec | Failure icon and badge backgrounds |
| `transferRow.errIcon` | `{color.danger.text}` | #d70015 | Failure text |
| `transferRow.speedText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `transferRow.errorText` | `{color.danger.text}` | #d70015 | Failure text |
| `transferRow.runBg` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `transferRow.runText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `transferRow.runDot` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `transferRow.pausedBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `transferRow.pausedText` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `transferRow.pausedDot` | `{color.status.paused}` | #c7c7cc | Paused progress |
| `transferRow.doneBadgeBg` | `{color.success.subtle}` | #ebf8ef |  |
| `transferRow.doneBadgeText` | `{color.success.text}` | #1f8a3f |  |
| `transferRow.doneDot` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `transferRow.errBadgeBg` | `{color.danger.subtle}` | #fdecec | Failure icon and badge backgrounds |
| `transferRow.errBadgeText` | `{color.danger.text}` | #d70015 | Failure text |
| `transferRow.errDot` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |
| `transferRow.progressWidth` | `{size.transferProgressWidth}` | 180px | **derived** FTP v3 docked transfer-progress width |
| `transferRow.speedWidth` | `{size.transferSpeedWidth}` | 120px | **derived** FTP v3 docked transfer-speed width |
| `transferRow.statusWidth` | `{size.transferStatusWidth}` | 80px | **derived** FTP v3 fixed docked transfer-status width |
| `transferRow.nameMinWidth` | `{size.transferNameMinWidth}` | 120px | **derived** FTP v3 minimum transfer-name width |
| `transferRow.nameSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `transferRow.pathSize` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `transferRow.rowHeight` | `{size.queueRow}` | 56px |  |
| `transferRow.detailSize` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `transferRow.progressHeight` | `{size.progress}` | 5px |  |
| `transferRow.gap` | `{space.8}` | 8px |  |
| `transferRow.paddingX` | `{space.12}` | 12px |  |
| `transferRow.demoBytesPerSecond` | `{motion.demoTransferSpeed}` | 1.30023e+07 | Demo transfer throughput |
| `transferRow.speedInterval` | `{motion.duration.autoExpand}` | 500ms | Expand automatically on hover |
| `transferRow.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |

### filePane
| token | Reference | Light value | Description |
|---|---|---|---|
| `filePane.headerHeight` | `{size.fileToolbar}` | 44px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.statusHeight` | `{size.statusBar}` | 34px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.compactWidth` | `{size.fileCompactWidth}` | 720px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.searchWidth` | `{size.fileSearchWidth}` | 160px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.sizeWidth` | `{size.fileSizeWidth}` | 80px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.dateWidth` | `{size.fileDateWidth}` | 140px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.permissionsWidth` | `{size.filePermissionsWidth}` | 100px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.ownerWidth` | `{size.fileOwnerWidth}` | 90px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.gridCellWidth` | `{size.fileGridCellWidth}` | 120px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.gridCellHeight` | `{size.fileGridCellHeight}` | 100px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.rowInset` | `{space.8}` | 8px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.toolGap` | `{space.4}` | 4px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.contentPadding` | `{space.8}` | 8px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.nameMinWidth` | `{size.fileNameMinWidth}` | 100px | **derived** Additional FTP v3 native file-pane layout |
| `filePane.gridIconSize` | `{size.fileGridIconSize}` | 32px | **derived** Additional FTP v3 native file-pane layout |

### input
| token | Reference | Light value | Description |
|---|---|---|---|
| `input.height` | `{size.control}` | 28px |  |
| `input.paddingX` | `{space.9}` | 9px |  |
| `input.radius` | `{radius.control}` | 7px |  |
| `input.bg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `input.text` | `{color.text.primary}` | #1d1d1f |  |
| `input.fontSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `input.border` | `{color.border.control}` | #e6e6e9 | Button borders and vertical status separators |
| `input.borderHover` | `{color.border.separatorWindow}` | #d2d2d6 | Title-bar button-group divider |
| `input.borderFocus` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `input.borderWidth` | `{size.divider}` | 1px |  |
| `input.transition` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `input.gap` | `{space.7}` | 7px |  |
| `input.focusShadow` | `{shadow.inputFocus}` | inset 0px 0px 0px 1.5px #0a84ff, 0px 0px 0px 3px rgba(10,132,255,0.16) | Focused input: inner ring and outer glow |
| `input.errorShadow` | `{shadow.inputError}` | inset 0px 0px 0px 1.5px #ff3b30, 0px 0px 0px 3px rgba(255,59,48,0.14) | Input error, taking priority over focus |
| `input.placeholder` | `{color.text.placeholder}` | #c7c7cc | Breadcrumb separator |
| `input.icon` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `input.monoFont` | `{font.family.mono}` | JetBrains Mono, Menlo, monospace |  |
| `input.monoSize` | `{size.inputMonoSize}` | 12px |  |
| `input.monoTracking` | `{font.tracking.inputMono}` | -0.1px |  |
| `input.disabledBg` | `{color.surface.field}` | #f6f6f7 | Breadcrumb path field |
| `input.disabledText` | `{color.text.disabled}` | #aeaeb2 | Unfocused pane host |
| `input.disabledBorder` | `{color.border.subtle}` | #f0f0f2 | Inner dividers: sidebar footer, status bars, progress tracks |
| `input.caret` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `input.chipHeight` | `{size.controlXs}` | 22px |  |
| `input.chipRadius` | `{radius.chip}` | 5px |  |
| `input.chipBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `input.chipFontSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `input.selectWidth` | `{size.transferProgressWidth}` | 180px |  |
| `input.portWidth` | `{size.formLabel}` | 72px |  |
| `input.numberWidth` | `{size.formLabel}` | 72px |  |
| `input.lineHeight` | `{font.lineHeight.input}` | 18px | Align empty-input and Chinese-placeholder line heights across font fallback |

### paneReveal
| token | Reference | Light value | Description |
|---|---|---|---|
| `paneReveal.blur` | `{glass.paneRevealBlur}` | 8px |  |
| `paneReveal.duration` | `{motion.duration.paneReveal}` | 220ms |  |
| `paneReveal.easing` | `{motion.easing.out}` | cubic-bezier(0,0,0.58,1) |  |

### modal
| token | Reference | Light value | Description |
|---|---|---|---|
| `modal.width` | `{size.modal}` | 560px |  |
| `modal.radius` | `{radius.window}` | 12px |  |
| `modal.bg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `modal.shadow` | `{shadow.modal}` | 0px 0px 0px 0.5px rgba(0,0,0,0.12), 0px 24px 64px 0px rgba(0,0,0,0.22), 0px 4px 16px 0px rgba(0,0,0,0.08) | Session dialog |
| `modal.scrim` | `{color.scrim}` | rgba(0,0,0,0.18) | Session dialog backdrop, including title bar |
| `modal.topRatio` | `{size.modalTopRatio}` | 0.12 |  |
| `modal.topMin` | `{size.modalTopMin}` | 72px |  |
| `modal.bottomMargin` | `{space.24}` | 24px |  |
| `modal.headerPaddingTop` | `{space.18}` | 18px |  |
| `modal.headerPaddingRight` | `{space.16}` | 16px |  |
| `modal.headerPaddingBottom` | `{space.14}` | 14px |  |
| `modal.headerPaddingLeft` | `{space.24}` | 24px |  |
| `modal.headerGap` | `{space.12}` | 12px |  |
| `modal.iconTile` | `{size.modalIconTile}` | 32px |  |
| `modal.titleSize` | `{font.size.title}` | 15px | Large heading from the sidebar-drag design |
| `modal.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `modal.subtitleSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `modal.subtitle` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `modal.bodyPaddingX` | `{space.24}` | 24px |  |
| `modal.divider` | `{color.border.divider}` | #f2f2f4 | Header and table-header bottom lines |
| `modal.footerPaddingTop` | `{space.14}` | 14px |  |
| `modal.footerPaddingRight` | `{space.16}` | 16px |  |
| `modal.footerPaddingBottom` | `{space.16}` | 16px |  |
| `modal.footerPaddingLeft` | `{space.24}` | 24px |  |
| `modal.footerGap` | `{space.8}` | 8px |  |
| `modal.buttonPaddingX` | `{space.12}` | 12px |  |
| `modal.enterScale` | `{opacity.modalEnterScale}` | 0.96 |  |
| `modal.exitScale` | `{opacity.modalExitScale}` | 0.98 |  |
| `modal.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `modal.fade` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `modal.exit` | `{motion.duration.exit}` | 120ms | Overlay exit and capsule fade |
| `modal.reduceFade` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `modal.generateWidth` | `{size.credentialModal}` | 420px |  |
| `modal.protocolRadius` | `{radius.lift}` | 9px |  |
| `modal.protocolPadding` | `{space.3}` | 3px |  |
| `modal.protocolIcon` | `{size.controlXs}` | 22px |  |

### formRow
| token | Reference | Light value | Description |
|---|---|---|---|
| `formRow.labelWidth` | `{size.formLabel}` | 72px |  |
| `formRow.labelGap` | `{space.12}` | 12px |  |
| `formRow.rowGap` | `{space.12}` | 12px |  |
| `formRow.label` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `formRow.labelSize` | `{font.size.body}` | 12.5px | Tabs, buttons, lists |
| `formRow.labelHeight` | `{size.control}` | 28px |  |
| `formRow.error` | `{color.danger.text}` | #d70015 | Failure text |
| `formRow.errorSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `formRow.errorIcon` | `{size.formErrorIcon}` | 12px |  |
| `formRow.errorGap` | `{space.5}` | 5px |  |
| `formRow.description` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `formRow.descriptionSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `formRow.descGap` | `{space.4}` | 4px |  |

### testResult
| token | Reference | Light value | Description |
|---|---|---|---|
| `testResult.gap` | `{space.6}` | 6px |  |
| `testResult.fontSize` | `{font.size.small}` | 12px | Sidebar footer and secondary host information |
| `testResult.icon` | `{size.iconSm}` | 14px |  |
| `testResult.spinnerSize` | `{size.testSpinner}` | 13px |  |
| `testResult.testing` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `testResult.success` | `{color.success.text}` | #1f8a3f |  |
| `testResult.successIcon` | `{color.success.default}` | #30c45a | Online dots and completed progress |
| `testResult.fail` | `{color.danger.text}` | #d70015 | Failure text |
| `testResult.failIcon` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |
| `testResult.info` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `testResult.infoSize` | `{size.formErrorIcon}` | 12px |  |
| `testResult.crossfade` | `{motion.duration.focus}` | 160ms | Focus/hover transition |

### collapse
| token | Reference | Light value | Description |
|---|---|---|---|
| `collapse.headerHeight` | `{size.titlebar}` | 44px |  |
| `collapse.chevron` | `{size.collapseChevron}` | 12px |  |
| `collapse.chevronStroke` | `{size.collapseChevronStroke}` | 2.25px |  |
| `collapse.chevronColor` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `collapse.title` | `{color.text.primary}` | #1d1d1f |  |
| `collapse.titleSize` | `{font.size.bodyLg}` | 13px | Hostnames and headings |
| `collapse.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `collapse.summary` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `collapse.summarySize` | `{font.size.small}` | 12px | Sidebar footer and secondary host information |
| `collapse.duration` | `{motion.duration.collapse}` | 180ms | Collapse all groups while dragging a group |
| `collapse.chevronRotate` | `{size.collapseChevronRotate}` | 90px |  |
| `collapse.contentFadeFactor` | `{opacity.collapseContentFadeFactor}` | 1.6 |  |

### switch
| token | Reference | Light value | Description |
|---|---|---|---|
| `switch.w` | `{size.switchWidth}` | 32px |  |
| `switch.h` | `{size.switchHeight}` | 19px |  |
| `switch.knobSize` | `{size.switchKnob}` | 15px |  |
| `switch.inset` | `{space.2}` | 2px |  |
| `switch.off` | `{color.border.keycap}` | #e3e3e6 | Keycap border |
| `switch.on` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `switch.knob` | `{color.text.onAccent}` | #ffffff |  |
| `switch.knobShadow` | `{shadow.switchKnob}` | 0px 1px 2px 0px rgba(0,0,0,0.2) | Switch thumb |
| `switch.animation` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |

### settings
| token | Reference | Light value | Description |
|---|---|---|---|
| `settings.navWidth` | `{size.sider}` | 236px |  |
| `settings.navItemHeight` | `{size.modalIconTile}` | 32px |  |
| `settings.navItemRadius` | `{radius.control}` | 7px |  |
| `settings.navItemOn` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `settings.navTextOn` | `{color.text.accentStrong}` | #0a5bc4 | Selected filename |
| `settings.navIconOn` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `settings.navIconTile` | `{size.controlXs}` | 22px |  |
| `settings.headerPaddingTop` | `{space.16}` | 16px |  |
| `settings.headerPaddingRight` | `{space.14}` | 14px |  |
| `settings.headerPaddingBottom` | `{space.14}` | 14px |  |
| `settings.headerPaddingLeft` | `{size.settingsHeaderPaddingX}` | 32px |  |
| `settings.titleSize` | `{font.size.title}` | 15px | Large heading from the sidebar-drag design |
| `settings.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `settings.subtitleSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `settings.subtitle` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `settings.close` | `{size.control}` | 28px |  |
| `settings.contentMax` | `{size.settingsContent}` | 720px |  |
| `settings.contentPaddingY` | `{space.20}` | 20px |  |
| `settings.contentPaddingX` | `{size.settingsHeaderPaddingX}` | 32px |  |
| `settings.groupTitleSize` | `{font.size.small}` | 12px | Sidebar footer and secondary host information |
| `settings.groupTitleWeight` | `{font.weight.semibold}` | 600 |  |
| `settings.groupTitle` | `{color.text.muted}` | #6e6e73 | Unselected tabs, rail icons, button icons |
| `settings.groupTitleGap` | `{space.8}` | 8px |  |
| `settings.groupGap` | `{space.22}` | 22px |  |
| `settings.groupBg` | `{color.surface.zebra}` | #fafafb | Alternating list rows |
| `settings.groupRadius` | `{radius.card}` | 10px |  |
| `settings.groupBorder` | `{color.border.raised}` | rgba(0,0,0,0.08) | 0.5px raised-surface border |
| `settings.groupBorderWidth` | `{size.hairline}` | 0.5px | 0.5px border |
| `settings.rowMinHeight` | `{size.settingsRowHeight}` | 48px |  |
| `settings.rowPaddingY` | `{space.9}` | 9px |  |
| `settings.rowPaddingX` | `{space.14}` | 14px |  |
| `settings.rowDivider` | `{color.border.subtle}` | #f0f0f2 | Inner dividers: sidebar footer, status bars, progress tracks |
| `settings.rowDividerInset` | `{space.14}` | 14px |  |
| `settings.rowLabelSize` | `{font.size.bodyLg}` | 13px | Hostnames and headings |
| `settings.rowLabel` | `{color.text.primary}` | #1d1d1f |  |
| `settings.rowDescSize` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `settings.rowDesc` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `settings.fadeHeight` | `{size.settingsFadeHeight}` | 40px |  |
| `settings.enterY` | `{space.8}` | 8px |  |
| `settings.exitY` | `{space.4}` | 4px |  |
| `settings.enter` | `{motion.spring.smooth}` | duration 300ms · bounce 0 | Cancel / smooth (ui.Smooth) |
| `settings.fade` | `{motion.duration.focus}` | 160ms | Focus/hover transition |
| `settings.exit` | `{motion.duration.exit}` | 120ms | Overlay exit and capsule fade |
| `settings.switchY` | `{space.4}` | 4px |  |
| `settings.switchFadeOut` | `{motion.duration.settingsSwitchOut}` | 80ms |  |
| `settings.switchFadeIn` | `{motion.duration.exit}` | 120ms | Overlay exit and capsule fade |
| `settings.selectWidth` | `{size.settingsSelect}` | 220px |  |
| `settings.numberWidth` | `{size.settingsNumber}` | 64px |  |
| `settings.scrollbackWidth` | `{size.settingsScrollback}` | 96px |  |
| `settings.rowGap` | `{space.16}` | 16px |  |

### schemeCard
| token | Reference | Light value | Description |
|---|---|---|---|
| `schemeCard.radius` | `{radius.lift}` | 9px |  |
| `schemeCard.padding` | `{space.5}` | 5px |  |
| `schemeCard.border` | `{color.surface.controlStrong}` | #ececef | Inline sidebar More button |
| `schemeCard.borderWidth` | `{size.divider}` | 1px |  |
| `schemeCard.selected` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `schemeCard.selectedWidth` | `{size.progressThin}` | 2px |  |
| `schemeCard.previewHeight` | `{size.schemePreviewHeight}` | 66px |  |
| `schemeCard.previewRadius` | `{radius.controlSm}` | 6px |  |
| `schemeCard.previewFontSize` | `{size.schemePreviewFont}` | 10px |  |
| `schemeCard.previewLineHeight` | `{size.schemePreviewLineHeight}` | 16px |  |
| `schemeCard.stripeHeight` | `{size.progress}` | 5px |  |
| `schemeCard.stripeRadius` | `{radius.hair}` | 2px |  |
| `schemeCard.stripeGap` | `{space.2}` | 2px |  |
| `schemeCard.nameSize` | `{font.size.small}` | 12px | Sidebar footer and secondary host information |
| `schemeCard.nameWeight` | `{font.weight.medium}` | 500 |  |
| `schemeCard.badgeSize` | `{font.size.caption2}` | 10.5px | ⌘N chip and count capsules |
| `schemeCard.badgeWeight` | `{font.weight.semibold}` | 600 |  |
| `schemeCard.badgeBg` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `schemeCard.badgeText` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `schemeCard.badgeIcon` | `{size.schemeBadgeIcon}` | 10px |  |

### themeCard
| token | Reference | Light value | Description |
|---|---|---|---|
| `themeCard.aspect` | `{size.themeAspect}` | 1.6 |  |
| `themeCard.radius` | `{radius.lift}` | 9px |  |
| `themeCard.border` | `{color.surface.controlStrong}` | #ececef | Inline sidebar More button |
| `themeCard.borderWidth` | `{size.divider}` | 1px |  |
| `themeCard.selected` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `themeCard.selectedWidth` | `{size.progressThin}` | 2px |  |
| `themeCard.selectedShadow` | `{shadow.themeSelected}` | 0px 0px 0px 3px rgba(10,132,255,0.16) | Selected appearance-card glow |
| `themeCard.radio` | `{size.themeRadio}` | 14px |  |
| `themeCard.radioRing` | `{size.themeRadioRing}` | 4.5px |  |

### accentSwatch
| token | Reference | Light value | Description |
|---|---|---|---|
| `accentSwatch.size` | `{size.accentSwatch}` | 20px |  |
| `accentSwatch.gap` | `{space.10}` | 10px |  |
| `accentSwatch.ringGap` | `{size.accentRingGap}` | 2px |  |
| `accentSwatch.ringWidth` | `{size.accentRing}` | 1.5px |  |
| `accentSwatch.ringInset` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |

### shortcutField
| token | Reference | Light value | Description |
|---|---|---|---|
| `shortcutField.width` | `{size.settingsShortcut}` | 176px |  |
| `shortcutField.keyGap` | `{space.4}` | 4px |  |
| `shortcutField.keyText` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `shortcutField.prompt` | `{color.text.accent}` | #0a6fd6 | Blue text: badges, notices, highlighted groups |
| `shortcutField.controlGap` | `{space.8}` | 8px |  |

### helpTip
| token | Reference | Light value | Description |
|---|---|---|---|
| `helpTip.size` | `{size.iconSm}` | 14px |  |
| `helpTip.gap` | `{space.4}` | 4px |  |
| `helpTip.icon` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `helpTip.iconHover` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |

### kbd
| token | Reference | Light value | Description |
|---|---|---|---|
| `kbd.height` | `{size.keycap}` | 18px |  |
| `kbd.minWidth` | `{size.keycap}` | 18px |  |
| `kbd.paddingX` | `{space.5}` | 5px |  |
| `kbd.radius` | `{radius.keycap}` | 4px |  |
| `kbd.fontSize` | `{font.size.caption}` | 11px | Keycaps, status badges, counters |
| `kbd.bg` | `{color.surface.keycap}` | #ffffff | Keycap background |
| `kbd.border` | `{color.border.keycap}` | #e3e3e6 | Keycap border |
| `kbd.text` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `kbd.inkBg` | `{color.keycap.inkBg}` | rgba(255,255,255,0.16) |  |
| `kbd.inkText` | `{color.keycap.inkText}` | rgba(255,255,255,0.75) |  |
| `kbd.accentBg` | `{color.keycap.accentBg}` | rgba(255,255,255,0.2) |  |
| `kbd.accentText` | `{color.text.onAccent}` | #ffffff |  |

### divider
| token | Reference | Light value | Description |
|---|---|---|---|
| `divider.color` | `{color.border.divider}` | #f2f2f4 | Header and table-header bottom lines |
| `divider.width` | `{size.divider}` | 1px |  |
| `divider.verticalHeight` | `{size.separatorShort}` | 12px |  |

### spinner
| token | Reference | Light value | Description |
|---|---|---|---|
| `spinner.sizeXs` | `{size.iconXs}` | 12px |  |
| `spinner.sizeSm` | `{size.iconSm}` | 14px |  |
| `spinner.sizeMd` | `{size.icon}` | 16px |  |
| `spinner.color` | `{color.icon.default}` | #6e6e73 |  |
| `spinner.period` | `{motion.duration.spin}` | 800ms | One connecting-spinner revolution |

### check
| token | Reference | Light value | Description |
|---|---|---|---|
| `check.size` | `{size.check}` | 14px |  |
| `check.radius` | `{radius.keycap}` | 4px |  |
| `check.border` | `{color.border.keycap}` | #e3e3e6 | Keycap border |
| `check.on` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `check.mark` | `{color.text.onAccent}` | #ffffff |  |
| `check.radioDot` | `{size.radioDot}` | 4.5px |  |

### banner
| token | Reference | Light value | Description |
|---|---|---|---|
| `banner.paddingX` | `{space.12}` | 12px |  |
| `banner.paddingY` | `{space.9}` | 9px |  |
| `banner.radius` | `{radius.railButton}` | 8px |  |
| `banner.gap` | `{space.8}` | 8px |  |
| `banner.icon` | `{size.iconSm}` | 14px |  |
| `banner.neutralBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `banner.neutralText` | `{color.text.strong}` | #3a3a3f | Secondary button text |
| `banner.neutralIcon` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `banner.infoBg` | `{color.accent.subtle}` | #eaf3fe | Light blue background: upload, running, notices |
| `banner.infoText` | `{color.text.accentStrong}` | #0a5bc4 | Selected filename |
| `banner.infoIcon` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `banner.warnBg` | `{color.warning.subtle}` | #fdf3e3 |  |
| `banner.warnText` | `{color.warning.textStrong}` | #b26a00 |  |
| `banner.warnIcon` | `{color.warning.default}` | #f5a623 | Connecting |
| `banner.dangerBg` | `{color.danger.subtle}` | #fdecec | Failure icon and badge backgrounds |
| `banner.dangerText` | `{color.danger.text}` | #d70015 | Failure text |
| `banner.dangerIcon` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |

### panel
| token | Reference | Light value | Description |
|---|---|---|---|
| `panel.width` | `{size.statsPanel}` | 204px | Monitoring sidebar in split view |
| `panel.headerHeight` | `{size.panelHeader}` | 46px |  |
| `panel.paddingX` | `{space.14}` | 14px |  |
| `panel.sectionGap` | `{space.8}` | 8px |  |
| `panel.divider` | `{color.border.divider}` | #f2f2f4 | Header and table-header bottom lines |
| `panel.statLabel` | `{color.text.primary}` | #1d1d1f |  |
| `panel.statValue` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `panel.caption` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |

### sparkline
| token | Reference | Light value | Description |
|---|---|---|---|
| `sparkline.height` | `{size.sparklineHeight}` | 34px |  |
| `sparkline.stroke` | `{size.sparklineStroke}` | 1.5px |  |
| `sparkline.line` | `{color.accent.default}` | #0a84ff | Primary accent #0a84ff |
| `sparkline.fillAlpha` | `{opacity.sparklineFill}` | 0.2 | Top opacity of trend gradient fill |
| `sparkline.grid` | `{color.border.divider}` | #f2f2f4 | Header and table-header bottom lines |
| `sparkline.dotRadius` | `{size.sparklineDotRadius}` | 2.5px | Trend endpoint |
| `sparkline.haloRadius` | `{size.sparklineHaloRadius}` | 5px | Trend endpoint halo |
| `sparkline.haloAlpha` | `{opacity.sparklineHalo}` | 0.18 |  |

### keyValue
| token | Reference | Light value | Description |
|---|---|---|---|
| `keyValue.rowHeight` | `{size.keyValueRow}` | 22px |  |
| `keyValue.key` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `keyValue.value` | `{color.text.primary}` | #1d1d1f |  |
| `keyValue.valueDim` | `{color.text.tertiary}` | #a1a1a6 | Close ×, keycaps, captions |
| `keyValue.font` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |

### emptyState
| token | Reference | Light value | Description |
|---|---|---|---|
| `emptyState.iconTile` | `{size.titlebar}` | 44px |  |
| `emptyState.iconRadius` | `{radius.window}` | 12px |  |
| `emptyState.iconBg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
| `emptyState.icon` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `emptyState.title` | `{font.size.bodyLg}` | 13px | Hostnames and headings |
| `emptyState.detail` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `emptyState.maxWidth` | `{size.emptyMaxWidth}` | 220px |  |

### toast
| token | Reference | Light value | Description |
|---|---|---|---|
| `toast.width` | `{size.toast}` | 340px |  |
| `toast.radius` | `{radius.card}` | 10px |  |
| `toast.bg` | `{color.tooltip.bg}` | rgba(30,30,32,0.92) |  |
| `toast.text` | `{color.tooltip.text}` | #ffffff |  |
| `toast.detail` | `{color.tooltip.kbd}` | rgba(255,255,255,0.6) |  |
| `toast.action` | `{color.toast.action}` | #4da3ff |  |
| `toast.okIcon` | `{color.toast.ok}` | #32d74b |  |
| `toast.errIcon` | `{color.toast.error}` | #ff453a |  |
| `toast.warnIcon` | `{color.toast.warning}` | #ffb340 |  |
| `toast.shadow` | `{shadow.lift}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | Dragged card |
| `toast.paddingX` | `{space.14}` | 14px |  |
| `toast.paddingY` | `{space.10}` | 10px |  |
| `toast.gap` | `{space.8}` | 8px |  |
| `toast.timeout` | `{motion.duration.autoHide}` | 3000ms |  |

### popover
| token | Reference | Light value | Description |
|---|---|---|---|
| `popover.radius` | `{radius.window}` | 12px |  |
| `popover.padding` | `{space.12}` | 12px |  |
| `popover.bg` | `{color.surface.raised}` | #ffffff | Raised surfaces: dragged cards, ghosts, selected segments |
| `popover.shadow` | `{shadow.lift}` | 0px 0px 0px 0.5px rgba(0,0,0,0.1), 0px 10px 28px 0px rgba(0,0,0,0.16), 0px 2px 6px 0px rgba(0,0,0,0.08) | Dragged card |

### latency
| token | Reference | Light value | Description |
|---|---|---|---|
| `latency.warnMs` | `{size.latencyWarn}` | 150 |  |
| `latency.badMs` | `{size.latencyBad}` | 400 |  |
| `latency.normal` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `latency.warn` | `{color.warning.text}` | #c7861a | Connecting text / bell |
| `latency.bad` | `{color.danger.text}` | #d70015 | Failure text |

### listRow
| token | Reference | Light value | Description |
|---|---|---|---|
| `listRow.height` | `{size.listRow}` | 30px |  |
| `listRow.radius` | `{radius.controlSm}` | 6px |  |
| `listRow.paddingX` | `{space.8}` | 8px |  |
| `listRow.gap` | `{space.9}` | 9px |  |
| `listRow.meta` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |

### paneHeader
| token | Reference | Light value | Description |
|---|---|---|---|
| `paneHeader.heightCompact` | `{size.paneHeader}` | 36px |  |
| `paneHeader.heightToolbar` | `{size.fileHeader}` | 40px |  |
| `paneHeader.heightTitle` | `{size.panelHeader}` | 46px |  |
| `paneHeader.titleFont` | `{font.size.bodyLg}` | 13px | Hostnames and headings |
| `paneHeader.titleWeight` | `{font.weight.semibold}` | 600 |  |
| `paneHeader.paddingLeft` | `{space.14}` | 14px |  |
| `paneHeader.paddingRight` | `{space.8}` | 8px |  |
| `paneHeader.gap` | `{space.7}` | 7px |  |

### paneFooter
| token | Reference | Light value | Description |
|---|---|---|---|
| `paneFooter.height` | `{size.paneStatusBar}` | 32px |  |
| `paneFooter.paddingX` | `{space.14}` | 14px |  |
| `paneFooter.gap` | `{space.10}` | 10px |  |
| `paneFooter.text` | `{color.text.secondary}` | #8e8e93 | Secondary text, host subtitles, status bars |
| `paneFooter.font` | `{font.size.footnote}` | 11.5px | Group headers, subtitles, status bars |
| `paneFooter.divider` | `{color.border.divider}` | #f2f2f4 | Header and table-header bottom lines |

### mcp
| token | Reference | Light value | Description |
|---|---|---|---|
| `mcp.tableMinWidth` | `{size.mcpTableMinWidth}` | 1024px |  |
| `mcp.keyName` | `{size.mcpNameColumn}` | 210px |  |
| `mcp.keyState` | `{size.mcpBadgeColumn}` | 86px |  |
| `mcp.keyDirs` | `{size.mcpNameColumn}` | 210px |  |
| `mcp.keyUsed` | `{size.mcpTimeColumn}` | 150px |  |
| `mcp.policyName` | `{size.mcpNameColumn}` | 210px |  |
| `mcp.policyCategory` | `{size.mcpBadgeColumn}` | 86px |  |
| `mcp.policyConnections` | `{size.mcpScopeColumn}` | 130px |  |
| `mcp.policyKeys` | `{size.mcpKeysColumn}` | 115px |  |
| `mcp.policyState` | `{size.mcpScopeColumn}` | 130px |  |
| `mcp.auditTime` | `{size.mcpAuditTimeColumn}` | 72px |  |
| `mcp.auditKey` | `{size.mcpAuditKeyColumn}` | 148px |  |
| `mcp.auditConnection` | `{size.mcpAuditConnectionColumn}` | 120px |  |
| `mcp.auditAction` | `{size.mcpAuditActionColumn}` | 96px |  |
| `mcp.auditStatus` | `{size.mcpAuditStatusColumn}` | 104px |  |
| `mcp.auditDuration` | `{size.mcpAuditDurationColumn}` | 56px |  |
| `mcp.policyModalWidth` | `{size.mcpPolicyModal}` | 720px |  |
| `mcp.keyModalWidth` | `{size.mcpKeyModal}` | 680px |  |
| `mcp.dialogWidth` | `{size.mcpDialog}` | 440px |  |
| `mcp.drawerMinWidth` | `{size.mcpDrawerMinWidth}` | 340px |  |
| `mcp.drawerWidth` | `{size.mcpDrawerWidth}` | 500px |  |
| `mcp.mainMinWidth` | `{size.mcpMainMinWidth}` | 360px |  |
| `mcp.ruleModeWidth` | `{size.mcpRuleModeWidth}` | 120px | Command-rule match-mode selector |
| `mcp.permissionColumn` | `{size.mcpPermissionColumn}` | 76px | Connection-grant capability column |

### inset
| token | Reference | Light value | Description |
|---|---|---|---|
| `inset.padding` | `{space.10}` | 10px |  |
| `inset.radius` | `{radius.control}` | 7px |  |
| `inset.borderWidth` | `{size.hairline}` | 0.5px | 0.5px border |
| `inset.compactPaddingX` | `{space.8}` | 8px |  |
| `inset.compactPaddingY` | `{space.6}` | 6px |  |
| `inset.compactRadius` | `{radius.controlSm}` | 6px |  |
| `inset.bg` | `{color.surface.sunken}` | #f4f4f5 | Search field |
| `inset.border` | `{color.border.subtle}` | #f0f0f2 | Inner dividers: sidebar footer, status bars, progress tracks |
| `inset.raisedBg` | `{color.surface.card}` | #ffffff | Content cards, sidebar, FTP panes |
| `inset.warningBg` | `{color.warning.subtle}` | #fdf3e3 |  |
| `inset.warningBorder` | `{color.warning.default}` | #f5a623 | Connecting |
| `inset.dangerBg` | `{color.danger.subtle}` | #fdecec | Failure icon and badge backgrounds |
| `inset.dangerBorder` | `{color.danger.default}` | #ff3b30 | Failure dots and icons |

### iconTile
| token | Reference | Light value | Description |
|---|---|---|---|
| `iconTile.sizeSm` | `{size.controlSm}` | 24px |  |
| `iconTile.radiusSm` | `{radius.controlSm}` | 6px |  |
| `iconTile.size` | `{size.control}` | 28px |  |
| `iconTile.radius` | `{radius.control}` | 7px |  |
| `iconTile.sizeLg` | `{size.modalIconTile}` | 32px |  |
| `iconTile.radiusLg` | `{radius.railButton}` | 8px |  |
| `iconTile.bg` | `{color.surface.control}` | #f2f2f4 | Neutral control background: icon buttons, segments, badges, queue icons |
