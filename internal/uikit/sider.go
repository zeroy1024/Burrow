package uikit

import (
	"strconv"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func Sider(c *ui.Context, label string) ui.Element {
	return ui.Column(c).FillHeight().Label(label)
}

type SiderHeaderProps struct {
	Title, AddLabel string
	AddIcon         *ui.SVG
}

type SiderHeaderParts struct {
	Root, Add ui.Element
}

func SiderHeader(c *ui.Context, props SiderHeaderProps) SiderHeaderParts {
	t := theme.Current()
	p := t.C.Sider
	parts := SiderHeaderParts{Root: ui.Row(c).Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft)}
	parts.Root.Children(func() {
		ui.Text(c, props.Title).FontSize(p.TitleSize).FontWeight(p.TitleWeight)
		ui.Spacer(c)
		parts.Add = IconButton(c, props.AddIcon, IconButtonProps{Label: props.AddLabel}).Size(p.AddButtonSize, p.AddButtonSize).
			Radius(p.AddButtonRadius).Background(Color(p.AddButtonBg))
	})
	return parts
}

func SiderFooter(c *ui.Context, icon *ui.SVG, text string) ui.Element {
	t := theme.Current()
	p := t.C.Sider
	return ui.Row(c).Shrink(0).Gap(t.Space.N8).Padding(p.FooterPaddingY, p.FooterPaddingX).
		BorderWidth(t.Size.Hairline, 0, 0, 0).BorderColor(Color(p.FooterBorder)).TextColor(Color(p.FooterText)).Children(func() {
		ui.Icon(c, icon).Size(t.Size.IconSm, t.Size.IconSm)
		ui.Text(c, text).FontSize(p.FooterSize)
	})
}

type SiderGroupProps struct {
	Key      any
	Title    string
	Expanded bool
}

// SiderGroup builds geometry before the caller tracks its drag gesture.
func SiderGroup(c *ui.Context, props SiderGroupProps) ui.Element {
	p := theme.Current().C.GroupHeader
	return ui.ButtonBase(c).Key(props.Key).Height(p.Height).Shrink(0).FillWidth().Padding(0, p.PaddingX).
		Gap(p.Gap).Radius(p.Radius).Label(props.Title).Expanded(props.Expanded).TextColor(Color(p.Text))
}

func StyleSiderGroup(header ui.Element, highlighted bool) {
	p := theme.Current().C.GroupHeader
	background := ui.Transparent
	if highlighted {
		background = Color(p.DropBg)
	}
	header.Background(header.AnimateColor("header-bg", background, Tween(p.Transition)))
}

type SiderGroupContentProps struct {
	Title                 string
	Expanded, Highlighted bool
	Count                 int
	CountDown             bool
	Chevron               *ui.SVG
	ArrowAnimation        ui.Animation
}

func SiderGroupContents(c *ui.Context, props SiderGroupContentProps) {
	t := theme.Current()
	p := t.C.GroupHeader
	text, chevron := Color(p.Text), Color(p.Text)
	if props.Highlighted {
		text, chevron = Color(p.DropText), Color(p.DropCount)
	}
	rotation := float32(-90)
	if props.Expanded {
		rotation = 0
	}
	animation := Tween(p.Transition)
	arrow := ui.Icon(c, props.Chevron).Key("chevron").Size(t.Font.Size.Caption, t.Font.Size.Caption)
	arrow.TextColor(arrow.AnimateColor("color", chevron, animation)).AnimatePose(ui.Pose{Rotate: rotation}, props.ArrowAnimation)
	label := ui.Text(c, props.Title).FontSize(p.FontSize).FontWeight(p.Weight)
	label.TextColor(label.AnimateColor("color", text, animation))
	ui.Spacer(c)
	countAnimation := ui.Animation{Spring: Spring(p.CountMotion)}
	ui.Text(c, strconv.Itoa(props.Count)).Key("count").FontSize(p.CountSize).TextColor(Color(p.Count)).
		ContentTransition(ui.ContentTransition{Numeric: true, Down: props.CountDown, Animation: &countAnimation})
}

func SiderGroupExpand(c *ui.Context, progress float32) {
	p := theme.Current().C.GroupHeader
	ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(p.ExpandHeight).
		Background(Color(p.ExpandTrack)).PassThrough().Children(func() {
		ui.Box(c).Absolute().Top(0).Bottom(0).Left(0).WidthPercent(progress * 100).Background(Color(p.ExpandFill)).PassThrough()
	})
}

type SiderRowProps struct {
	Key             any
	Title, Subtitle string
}

// SiderRow deliberately has no selection or connection-state properties.
func SiderRow(c *ui.Context, props SiderRowProps) ui.Element {
	t := theme.Current()
	p := t.C.SiderItem
	return ui.ButtonBase(c).Key(props.Key).Height(t.C.Catalog.RowHeight).Shrink(0).FillWidth().
		Padding(0, p.PaddingRight, 0, p.PaddingLeft).Gap(p.Gap).Radius(p.Radius).
		Role(ui.RoleListItem).Label(props.Title).Description(props.Subtitle).TextColor(Color(p.NameColor))
}

func SiderRowLabel(c *ui.Context, title, subtitle string) {
	t := theme.Current()
	p := t.C.SiderItem
	ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N1).Children(func() {
		ui.Text(c, title).FontSize(p.NameSize).FontWeight(p.NameWeight).FixedLineHeight(p.NameLineHeight).SingleLine().Ellipsis("…")
		ui.Text(c, subtitle).Font(strings.Join(p.SubFamily, ", ")).FontSize(p.SubSize).FixedLineHeight(p.SubLineHeight).
			TextColor(Color(p.SubColor)).SingleLine().Ellipsis("…")
	})
}

type SiderMoreProps struct {
	Label, Description string
	Icon               *ui.SVG
	Disabled           bool
}

func SiderMore(c *ui.Context, row ui.Element, props SiderMoreProps) ui.Element {
	t := theme.Current()
	p := t.C.SiderItem
	action := Tip(c, ui.ButtonBase(c).Key("host-actions").Size(p.MoreSize, p.MoreSize).Shrink(0).Radius(p.MoreRadius).
		TextColor(Color(p.MoreIcon)).Label(props.Label), "更多操作").Description(props.Description)
	background := ui.Transparent
	if !props.Disabled && action.Hovered() {
		background = Color(p.MoreBg)
	}
	action.Background(action.AnimateColor("background", background, Tween(p.Transition)))
	target := float32(0)
	if !props.Disabled && (row.Hovered() || row.FocusVisible() || action.FocusVisible()) {
		target = 1
	}
	return action.Opacity(Visual(action, "more", target, p.Transition)).Disabled(props.Disabled).
		Children(func() { ui.Icon(c, props.Icon).Size(t.Size.Icon, t.Size.Icon) })
}
