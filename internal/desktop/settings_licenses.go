package desktop

import (
	_ "embed"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

// License texts are copied verbatim from the dependency versions in go.mod.
//
//go:embed licenses/mygo.txt
var mygoLicense string

//go:embed licenses/purego.txt
var puregoLicense string

//go:embed licenses/go-text.txt
var goTextLicense string

//go:embed licenses/x-crypto.txt
var cryptoLicense string

//go:embed licenses/x-image.txt
var imageLicense string

//go:embed licenses/x-net.txt
var netLicense string

//go:embed licenses/x-text.txt
var textLicense string

//go:embed licenses/jetbrains-mono.txt
var jetbrainsLicense string

var dependencyNotices = []struct{ name, license, text string }{
	{"MyGo", "MIT", mygoLicense},
	{"golang.org/x/crypto/ssh", "BSD-3-Clause", cryptoLicense},
	{"PureGo", "Apache-2.0", puregoLicense},
	{"go-text/typesetting", "Unlicense / BSD-3-Clause", goTextLicense},
	{"golang.org/x/image", "BSD-3-Clause", imageLicense},
	{"golang.org/x/net", "BSD-3-Clause", netLicense},
	{"golang.org/x/text", "BSD-3-Clause", textLicense},
	{"Catppuccin / Dracula / Nord", "MIT", termtheme.Licenses},
	{"JetBrains Mono", "SIL Open Font License 1.1", jetbrainsLicense},
}

func (a *app) aboutSettings(c *ui.Context) {
	t := theme.Current()
	if a.settingMatches("关于", "版本 SSH Desk") {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Padding(t.Space.N16).Children(func() {
			uikit.Icon(c, terminalIcon, uikit.IconProps{Tone: uikit.ToneAccent}).Size(t.C.Modal.IconTile*2, t.C.Modal.IconTile*2)
			uikit.Text(c, localize(c, "SSH Desk"), uikit.TextProps{Style: uikit.TextTitle})
			ui.Row(c).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
				uikit.Text(c, localize(c, "0.1.0 · 原型构建"), uikit.TextProps{Tone: uikit.ToneSecondary})
				uikit.Button(c, localize(c, "复制"), uikit.ButtonProps{}).OnClick(func() {
					if err := mygo.Clipboard.Write(transfer.TextData("SSH Desk 0.1.0 (prototype)")); err != nil {
						a.settingsUI.notice = err.Error()
					}
				})
			})
			uikit.Text(c, localize(c, "当前为本地原型构建，尚未配置更新与反馈服务。"), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
		})
	}
	fields := make([]settingField, 0, len(dependencyNotices))
	for index, notice := range dependencyNotices {
		fields = append(fields, settingField{notice.name, notice.license, func() {
			anchor := uikit.Button(c, localize(c, "查看"), uikit.ButtonProps{}).OnClick(func() { a.settingsUI.license = index + 1 })
			open := a.settingsUI.license == index+1
			var viewer ui.Element
			var scroll *ui.ScrollState
			panel := uikit.Popover(c, anchor, &open, uikit.PopoverProps{
				Width: t.C.Settings.ContentMax * 3 / 4, Label: localize(c, notice.name+" · "+notice.license),
				Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}, func() {
				uikit.Text(c, localize(c, notice.name+" · "+notice.license), uikit.TextProps{Style: uikit.TextBodyStrong}).FontWeight(t.Font.Weight.Semibold)
				viewer = ui.Scroll(c).FillWidth().MaxHeight(t.C.Settings.ContentMax / 3)
				scroll = ui.Local(viewer, "uikit.license.scroll", func() ui.ScrollState { return ui.ScrollState{} })
				viewer.TrackScroll(scroll).Children(func() {
					uikit.Text(c, localize(c, notice.text), uikit.TextProps{Style: uikit.TextMono}).Selectable()
				})
			}).MaxHeight(t.C.Settings.ContentMax / 2)
			if viewer.ID() != 0 {
				uikit.Scrollbars(c, panel, viewer, scroll, uikit.ScrollbarProps{Label: "许可证正文滚动条"})
			}
			if !open && a.settingsUI.license == index+1 {
				a.settingsUI.license = 0
			}
		}})
	}
	a.settingsGroup(c, "开源许可", "", fields)
}
