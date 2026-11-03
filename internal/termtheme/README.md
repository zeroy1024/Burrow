# Burrow 终端配色包（termtheme）

终端配色和 App 界面 token 是**两套东西**：
- **App token**（`package theme`）管窗格容器：圆角、内描边、焦点环、标题栏，以及卡片底色 `C.Pane.Bg`（只在「终端背景 = 与卡片底色一致」时铺进终端区）。
- **终端配色**（本包，`package termtheme`）管终端区里的颜色：背景、文字前景、ANSI 16 色、256 色、光标、选区。
- 非焦点窗格的变暗仍由焦点方案 D 在窗格层统一处理，**不会修改配色本身**。

## 包含的文件

| 文件 | 说明 |
|---|---|
| `scheme.go` | `Scheme` 结构体、`Parse` / `Load` / `Format` / `Write`、`Color(0–255)`、`Xterm256`、`ParseColor` / `Hex` |
| `state.go` | 内置配色（`embed`）、`Register` / `Import` / `Lookup` / `Names`、`Mode`（跟随主题 / 固定）、`Resolve` / `Apply`、`Current` / `Set` / `OnChange`、`BackgroundSource` + `TerminalBackground`、`Prefs` |
| `termtheme_test.go` | 单元测试 |
| `schemes/` | 内置配色文件（Ghostty 格式，无扩展名） |
| `LICENSES.md` | 配色的 MIT 许可证全文 |
| `licenses.go` | 内嵌许可证，供设置页展示 |

本包只依赖 Go 标准库，通过 `sshdesk/internal/termtheme` 接入应用。`schemes/` 必须与 `state.go` 相邻，供 `//go:embed` 读取。用户操作见[终端与配色](../../docs/terminal-themes.md)，检查命令见[开发与构建](../../docs/development.md)。

## 内置配色

| 名称 | 用途 | 来源 | 许可 |
|---|---|---|---|
| Catppuccin Latte | 浅色默认 | catppuccin/ghostty `themes/catppuccin-latte.conf`（commit b0b03cc） | MIT |
| Catppuccin Mocha | 深色默认 | catppuccin/ghostty `themes/catppuccin-mocha.conf`（commit b0b03cc） | MIT |
| Dracula | 可选 | iTerm2-Color-Schemes `ghostty/Dracula`（commit 3d8a6a1） | MIT |
| Nord | 可选 | iTerm2-Color-Schemes `ghostty/Nord`（commit 3d8a6a1） | MIT |

**对上游的改动：**
- Catppuccin Latte 的 `background` 和 `cursor-text` 从 `#eff1f5` 改成 `#fbfbfc`，和我们的终端卡片同色。文件里用 `# upstream:` 注释保留了原值。Latte 的文字色是按 `#eff1f5` 设计的，换到更亮的 `#fbfbfc` 对比度只会更高。
- 每个文件开头加了 `# name:` / `# source:` / `# license:` 注释；去掉了 Catppuccin 文件里的 `split-divider-color`（分屏分隔线归 App token 管）。颜色值统一写成 `#rrggbb`。
- 其余颜色均为上游原值。

## 终端背景（用户选项）

终端区铺底色由用户在设置里选择，保存在 `Prefs.Background`：

| 选项 | 值 | 效果 |
|---|---|---|
| 配色自带背景（默认） | `BackgroundScheme`（存为 `"scheme"`） | 任何模式、任何 App 外观下都用 `scheme.Background` |
| 与卡片底色一致 | `BackgroundCard`（存为 `"card"`） | 任何配色下都用 App 的卡片色 `C.Pane.Bg` |

- 应用适配层取色：`termtheme.TerminalBackground(s, theme.Current().Color.Surface.Terminal, prefs.Background)`。
- 窗格顶部 / 底部渐隐栏的着色色必须和这个颜色一致，否则会出现色块。
- 默认下：浅色跟随主题是 Latte `#fbfbfc`（和卡片同色）；深色跟随主题是 Mocha `#1e1e2e`；固定 Dracula 时无论 App 是浅色还是深色都是 `#282a36`。
- 选「与卡片底色一致」时，配色只提供文字 / 光标 / 选区颜色。深色配色放在浅色卡片上可能看不清，这是用户的选择，App 不做自动修正。
- 卡片的圆角、内描边、焦点环、标题栏始终来自 App token。

## 导入更多配色

支持 Ghostty 主题格式：`key = value`，`#` 开头是注释，未知的键会被忽略。读取的键：
`background`、`foreground`、`cursor-color`、`cursor-text`、`selection-background`、`selection-foreground`、`palette = N=#rrggbb`（0–15，也接受 16–255 的覆盖）、`bold-color`，以及非标准的 `link-color`。颜色可以写 `#rrggbb`、`rrggbb` 或 `#rgb`。

缺少某个键时：背景 / 前景用 Ghostty 默认值；光标 = 前景；光标文字 = 背景；选区为前景 / 背景反色；缺少的 ANSI 色用 xterm 标准色。

导入方式：
1. 从 [iTerm2-Color-Schemes](https://github.com/mbadolato/iTerm2-Color-Schemes) 的 `ghostty/` 目录下载任意文件（例如 `ghostty/GitHub Light Default`），或者用 Ghostty 本身的主题文件。
2. 在 App 里点「导入文件」，或者在代码里调用 `termtheme.Import(path)`。名称取 `# name:` 注释；没有这个注释时取文件名。
3. 应用将导入文件保存到自己的 `themes/` 目录，启动时重新导入；位置见[数据与凭据](../../docs/storage.md)。
4. 包内 `termtheme.Write(path, s)` 可写出 Ghostty 格式；这不表示应用提供独立的导出按钮。`link-color` 是非标准键，Ghostty 不识别。

解析器可保留 16–255 号索引色覆盖，但原生终端主题接口只接入 0–15 色，不能把解析能力等同于实际终端显示能力。

**许可提醒：** iTerm2-Color-Schemes 仓库本身是 MIT，但每个配色还有作者自己的许可。如果要把新配色**内置进 App 分发**，请先确认那个配色的许可，并把许可证全文加进 `LICENSES.md`。用户自己导入的不受影响。

## 和字体 / 光标设置的关系

字体、字号、行高、光标形状、光标闪烁、终端背景选项放在 `Prefs` 里，不属于配色，换配色时不会变。`DefaultPrefs()` 使用 JetBrains Mono 13px，行高 19px，块状光标并闪烁。终端字距字段及其旧渲染路径已移除。
