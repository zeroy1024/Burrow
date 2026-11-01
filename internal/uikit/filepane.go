package uikit

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

func FileWorkspace(c *ui.Context) ui.Element {
	return ui.Column(c).Key("file-workspace").Grow(1).MinWidth(0).MinHeight(0)
}

func FilePanels(c *ui.Context) ui.Element {
	return ui.Box(c).Key("file-panels").Grow(1).FillWidth().MinWidth(0).MinHeight(0).ClipX()
}

func FilePane(c *ui.Context, key any, label string, focus *ui.Handle, focused bool) ui.Element {
	return Pane(c, PaneProps{Key: key, Kind: PaneFile, Label: label, Focused: focused}).Fill().
		Focusable().FocusRing(false).Bind(focus)
}

func FilePaneHeader(c *ui.Context) ui.Element {
	t := theme.Current()
	return PaneHeader(c, HeaderProps{Density: DensityToolbar}).Root.Height(t.C.FilePane.HeaderHeight).Padding(0, t.Space.N8).
		Gap(t.C.FilePane.ToolGap).FillWidth()
}

type PaneToggleProps struct {
	Label        string
	Shortcut     string
	Icon, IconOn *ui.SVG
}

func PaneToggle(c *ui.Context, on *bool, props PaneToggleProps) ui.Element {
	t := theme.Current()
	p := t.C.PaneToggle
	surface := *t
	surface.C.IconButton.Size, surface.C.IconButton.Radius = p.Size, p.Radius
	surface.C.IconButton.Icon, surface.C.IconButton.IconOn = p.Icon, p.IconOn
	surface.C.IconButton.BgHover, surface.C.IconButton.BgOn = p.BgHover, p.BgOn
	surface.C.IconButton.Transition = p.Transition
	return IconButton(c, props.Icon, IconButtonProps{Label: props.Label, Shortcut: props.Shortcut,
		On: on, IconOn: props.IconOn, Surface: &surface})
}

func FilePaneFooter(c *ui.Context, props ...FooterProps) ui.Element {
	t := theme.Current()
	footer := FooterProps{}
	if len(props) != 0 {
		footer = props[0]
	}
	return PaneFooter(c, footer).Absolute().Bottom(0).Left(0).Right(0).Height(t.C.FilePane.StatusHeight).
		Padding(0, t.C.StatusBar.PaddingX).Gap(t.Space.N8)
}

func FileDivider(c *ui.Context, left float32) ui.Element {
	t := theme.Current()
	return ui.Box(c).Key("file-divider").Absolute().Left(left).Top(0).Bottom(0).
		Width(t.C.FTPSplit.Gutter).Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).Label("调整本地面板宽度")
}

func StyleFileDivider(c *ui.Context, divider ui.Element, dragging bool) {
	t := theme.Current()
	p := t.C.FTPSplit
	if divider.Hovered() || dragging || divider.FocusVisible() {
		ink := p.Grip
		if dragging {
			ink = p.GripActive
		}
		divider.Center().Children(func() {
			ui.Box(c).Size(p.GripW, p.GripH).Radius(p.GripW).Background(Color(ink)).PassThrough()
		})
	}
}

func PaneLabel(c *ui.Context, label string, localOpen, active bool) ui.Element {
	t := theme.Current()
	focus := t.C.FilePaneFocus
	bg, ink, weight := ui.Transparent, Color(focus.LabelTextInactive), focus.LabelWeightInactive
	if localOpen && active {
		bg, ink, weight = Color(focus.LabelBgActive), Color(focus.LabelTextActive), focus.LabelWeightActive
	}
	if !localOpen {
		ink, weight = Color(t.Color.Text.Primary), focus.LabelWeightActive
	}
	badge := ui.Row(c).Height(t.C.Breadcrumb.Height).Padding(0, focus.LabelPaddingX).Radius(focus.LabelRadius).Shrink(0)
	animation := Tween(focus.Transition)
	return badge.Background(badge.AnimateColor("label-bg", bg, animation)).Children(func() {
		text := ui.Text(c, label).FontSize(t.Font.Size.Body).FontWeight(weight).SingleLine()
		text.TextColor(text.AnimateColor("label-fg", ink, animation))
	})
}

type HistoryParts struct {
	Back, Forward ui.Element
}

type HistoryButtonProps struct {
	BackShortcut, ForwardShortcut string
}

func HistoryButtons(c *ui.Context, canBack, canForward bool, backIcon, forwardIcon *ui.SVG, options ...HistoryButtonProps) HistoryParts {
	props := HistoryButtonProps{}
	if len(options) != 0 {
		props = options[0]
	}
	return HistoryParts{
		Back:    IconButton(c, backIcon, IconButtonProps{Key: "后退", Label: "后退", Shortcut: props.BackShortcut, Disabled: !canBack}),
		Forward: IconButton(c, forwardIcon, IconButtonProps{Key: "前进", Label: "前进", Shortcut: props.ForwardShortcut, Disabled: !canForward}),
	}
}
