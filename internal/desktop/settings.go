package desktop

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) syncAppearance() {
	dark := mygo.Theme.IsDark()
	if theme.Current().IsDark != dark {
		theme.Set(theme.ForAppearance(dark))
	}
}

func (a *app) setAppearance(index int) {
	a.appearance = index
	sources := [...]mygo.ThemeSource{mygo.ThemeLight, mygo.ThemeDark, mygo.ThemeSystem}
	mygo.Theme.SetSource(sources[index])
	// SetSource updates the native appearance synchronously on the UI thread;
	// some backends do not emit OnUpdated for changes initiated by this app.
	a.syncAppearance()
}

func (a *app) settings(c *ui.Context) {
	t := theme.Current()
	workspaceCard(c).Key("settings").Grow(1).
		Margin(0, t.C.Window.BodyPaddingRight, t.C.Window.BodyPaddingBottom, 0).
		Padding(t.Space.N24).Gap(t.Space.N18).Children(func() {
		ui.Text(c, "设置").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
		ui.Row(c).Gap(t.Space.N16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "外观").FontSize(t.C.Button.FontSize)
			p := t.C.Segmented
			ui.Row(c).Padding(p.Padding).Radius(p.Radius).Background(tokenColor(p.Bg)).
				Children(func() {
					for index, label := range []string{"浅色", "深色", "跟随系统"} {
						button := ui.ButtonBase(c).Key(label).Label(label).
							Selected(a.appearance == index).FontSize(p.FontSize).
							Padding(p.ItemPaddingY, p.ItemPaddingX).Radius(p.ItemRadius).
							TextColor(tokenColor(p.Text)).OnClick(func() { a.setAppearance(index) })
						if a.appearance == index {
							button.Background(tokenColor(p.BgOn)).TextColor(tokenColor(p.TextOn))
							applyShadow(button, p.ShadowOn)
						}
						button.Children(func() { ui.Text(c, label) })
					}
				})
		})
	})
}
