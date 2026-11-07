package desktop

import (
	_ "embed"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"sshdesk/assets"
	"sshdesk/internal/locales"
	"sshdesk/internal/termtheme"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

// License texts retain the upstream copyright notices for bundled dependencies and assets.
//
//go:embed licenses/mygo.txt
var mygoLicense string

//go:embed licenses/ghostty.txt
var ghosttyLicense string

//go:embed licenses/sftp.txt
var sftpLicense string

//go:embed licenses/go.txt
var goLicense string

//go:embed licenses/sqlite.txt
var sqliteLicense string

//go:embed licenses/purego.txt
var puregoLicense string

//go:embed licenses/go-text.txt
var goTextLicense string

//go:embed licenses/x-crypto.txt
var cryptoLicense string

//go:embed licenses/go-winio.txt
var winioLicense string

//go:embed licenses/mcp.txt
var mcpLicense string

//go:embed licenses/x-image.txt
var imageLicense string

//go:embed licenses/x-net.txt
var netLicense string

//go:embed licenses/x-text.txt
var textLicense string

//go:embed licenses/jetbrains-mono.txt
var jetbrainsLicense string

//go:embed licenses/mvdan-sh.txt
var shLicense string

//go:embed licenses/simple-icons.txt
var simpleIconsLicense string

var burrowIcon = func() *ui.Bitmap {
	bitmap, err := ui.DecodeBitmap(assets.IconPNG)
	if err != nil {
		panic(err)
	}
	return bitmap
}()

func dependencyNotices() []struct{ name, license, text string } {
	return []struct{ name, license, text string }{
		{"MyGo", "MIT", mygoLicense},
		{locales.MessageGhosttyVtAndNativeDependencies(), "MIT / Apache-2.0 / BSD-3-Clause / Unicode-3.0", ghosttyLicense},
		{locales.MessagePkgSftpAndKrFs(), "BSD-2-Clause / BSD-3-Clause", sftpLicense},
		{locales.MessageGoStandardLibraryGolangOrgXSys(), "BSD-3-Clause", goLicense},
		{locales.MessageModerncSqliteAndRuntimeDependencies(), "BSD / MIT", sqliteLicense},
		{locales.MessageMcpGoSdkAndRuntimeDependencies(), "Apache-2.0 / MIT / BSD-3-Clause", mcpLicense},
		{"Lucide Icons " + uikit.LucideVersion, "ISC / MIT", uikit.LucideLicense},
		{locales.MessageSimpleIconsOperatingSystemLogos(), "CC0-1.0", simpleIconsLicense},
		{"golang.org/x/crypto/ssh", "BSD-3-Clause", cryptoLicense},
		{"go-winio", "MIT", winioLicense},
		{"PureGo", "Apache-2.0", puregoLicense},
		{"go-text/typesetting", "Unlicense / BSD-3-Clause", goTextLicense},
		{"golang.org/x/image", "BSD-3-Clause", imageLicense},
		{"golang.org/x/net", "BSD-3-Clause", netLicense},
		{"golang.org/x/text", "BSD-3-Clause", textLicense},
		{"mvdan.cc/sh", "BSD-3-Clause", shLicense},
		{"Catppuccin / Dracula / Nord", "MIT", termtheme.Licenses},
		{"JetBrains Mono", "SIL Open Font License 1.1", jetbrainsLicense},
	}
}

func (a *app) aboutSettings(c *ui.Context) {
	t := theme.Current()
	if a.settingMatches(locales.AboutTitle(), locales.MessageBurrowVersion()) {
		ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Padding(t.Space.N16).Children(func() {
			ui.Image(c, burrowIcon).Size(t.C.Modal.IconTile*2, t.C.Modal.IconTile*2)
			uikit.Text(c, "Burrow", uikit.TextProps{Style: uikit.TextTitle})
			ui.Row(c).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
				uikit.Text(c, "0.1.3", uikit.TextProps{Tone: uikit.ToneSecondary})
				uikit.Button(c, locales.CommonCopy(), uikit.ButtonProps{}).OnClick(func() {
					if err := mygo.Clipboard.Write(transfer.TextData("Burrow 0.1.3")); err != nil {
						a.settingsUI.notice = locales.ErrorText(err)
					}
				})
			})
		})
	}
	fields := make([]settingField, 0, len(dependencyNotices()))
	for index, notice := range dependencyNotices() {
		fields = append(fields, settingField{index, notice.name, notice.license, "", func() {
			anchor := uikit.Button(
				c,
				locales.MessageView(),
				uikit.ButtonProps{},
			).OnClick(func() { a.settingsUI.license = index + 1 })
			open := a.settingsUI.license == index+1
			var viewer ui.Element
			var scroll *ui.ScrollState
			panel := uikit.Popover(c, anchor, &open, uikit.PopoverProps{
				Width: t.C.Settings.ContentMax * 3 / 4, Label: notice.name + " · " + notice.license,
				Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}, func() {
				uikit.Text(c, notice.name+" · "+notice.license, uikit.TextProps{Style: uikit.TextBodyStrong}).FontWeight(t.Font.Weight.Semibold)
				viewer = ui.Scroll(c).FillWidth().MaxHeight(t.C.Settings.ContentMax / 3)
				scroll = ui.Local(viewer, "uikit.license.scroll", func() ui.ScrollState { return ui.ScrollState{} })
				viewer.TrackScroll(scroll).Children(func() {
					uikit.Text(c, notice.text, uikit.TextProps{Style: uikit.TextMono}).Selectable()
				})
			}).MaxHeight(t.C.Settings.ContentMax / 2)
			if viewer.ID() != 0 {
				uikit.Scrollbars(c, panel, viewer, scroll, uikit.ScrollbarProps{Label: locales.MessageLicenseTextScrollbar()})
			}
			if !open && a.settingsUI.license == index+1 {
				a.settingsUI.license = 0
			}
		}})
	}
	a.settingsGroup(c, "MessageOpenSourceLicenses", locales.MessageOpenSourceLicenses(), "", fields)
}
