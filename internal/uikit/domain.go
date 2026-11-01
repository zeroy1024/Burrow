package uikit

import (
	"fmt"
	"image/color"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func OsLogo(c *ui.Context, icon *ui.SVG, brand color.NRGBA) {
	t := theme.Current()
	ui.Box(c).Size(t.C.OsLogo.Size, t.C.OsLogo.Size).Shrink(0).Radius(t.C.OsLogo.Radius).
		Background(Color(t.OSLogoBg(brand))).Center().Children(func() {
		ui.Image(c, icon).Size(t.C.OsLogo.GlyphSize, t.C.OsLogo.GlyphSize).TextColor(Color(brand))
	})
}

func ProtocolIcon(c *ui.Context, ftp bool, sftpIcon, ftpIcon *ui.SVG) {
	p := theme.Current().C.ProtocolIcon
	background, foreground, icon := p.SftpBg, p.SftpGlyph, sftpIcon
	if ftp {
		background, foreground, icon = p.FtpBg, p.FtpGlyph, ftpIcon
	}
	ui.Box(c).Size(p.Size, p.Size).Shrink(0).Radius(p.Radius).Background(Color(background)).
		Center().Children(func() { ui.Icon(c, icon).Size(p.Glyph, p.Glyph).TextColor(Color(foreground)) })
}

func FileIcon(c *ui.Context, directory bool, folder, document *ui.SVG) ui.Element {
	t := theme.Current()
	icon, ink := document, t.C.FileList.FileIcon
	if directory {
		icon, ink = folder, t.C.FileList.FolderIcon
	}
	return ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(Color(ink)).Shrink(0)
}

func ConnectionDot(c *ui.Context, state ConnState, size float32) {
	t := theme.Current()
	ink, halo := Color(t.C.ConnectionIndicator.Offline), Color(t.C.ConnectionIndicator.Offline)
	if state == ConnConnected {
		ink, halo = Color(t.C.ConnectionIndicator.Connected), Color(t.C.ConnectionIndicator.ConnectedHalo)
	}
	if state == ConnConnecting {
		ink, halo = Color(t.C.ConnectionIndicator.Connecting), Color(t.C.ConnectionIndicator.ConnectingHalo)
	}
	ui.Box(c).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).Center().PassThrough().Children(func() {
		dot := ui.Box(c).Size(size, size).Radius(t.Radius.Pill).Background(ink).PassThrough()
		if state != ConnDisconnected {
			dot.Border(t.C.ConnectionIndicator.Halo, halo).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).
				Background(ui.Transparent).Children(func() {
				ui.Box(c).Fill().Radius(t.Radius.Pill).Background(ink).PassThrough()
			})
		}
	})
}

func ConnectionBadge(c *ui.Context, state ConnState, dotSize float32, surface *theme.Theme) {
	p := surface.C.Badge
	label, background, ink := "已连接", Color(p.OkBg), Color(p.OkText)
	switch {
	case state == ConnConnecting:
		label, background, ink = "连接中", Color(surface.Color.Warning.Subtle), Color(surface.Color.Warning.Text)
	case state != ConnConnected:
		label, background, ink = "未连接", Color(p.NeutralBg), Color(p.NeutralText)
	}
	ui.Row(c).Padding(p.PaddingY, p.PaddingXCompact).Gap(p.Gap).Radius(p.Radius).Background(background).Children(func() {
		ui.Box(c).Size(dotSize, dotSize).Radius(p.Radius).Background(ink)
		ui.Text(c, label).FontSize(p.FontSizeCompact).TextColor(ink)
	})
}

func ProtocolBadge(c *ui.Context, protocol string) {
	t := theme.Current()
	p := t.C.Badge
	ui.Row(c).Padding(p.PaddingYCompact, p.PaddingXCompact).Gap(p.Gap).Radius(p.Radius).Background(Color(p.OkBg)).Children(func() {
		ui.Box(c).Size(t.C.ActivityBadge.DotSize, t.C.ActivityBadge.DotSize).Radius(p.Radius).Background(Color(p.OkDot))
		ui.Text(c, protocol+" 已连接 · 演示").FontSize(p.FontSizeCompact).TextColor(Color(p.OkText)).SingleLine()
	})
}

// Latency retains the existing neutral text; no new warning thresholds are imposed.
func Latency(c *ui.Context, ms int, known bool, surface *theme.Theme) ui.Element {
	label := "延迟 —"
	if known {
		label = fmt.Sprintf("延迟 %d ms", ms)
	}
	return ui.Text(c, label).FontSize(surface.C.StatusBar.FontSize).SingleLine()
}
