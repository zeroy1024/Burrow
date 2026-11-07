package uikit

import (
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

type SiderProps struct {
	Label, AddLabel, Footer string
	AddIcon, FooterIcon     *ui.SVG
	AddMenu                 []MenuEntry
	Query                   *string
	Search                  SearchProps
}

type SiderResult struct {
	Root, AddButton, SearchInput ui.Element
	Add                          bool
}

func Sider(c *ui.Context, title string, props SiderProps, body func()) SiderResult {
	label := props.Label
	if label == "" {
		label = title
	}
	root := ui.Column(c).Width(theme.Current().Size.Sider).FillHeight().Role(ui.RoleGroup).Label(label)
	query := props.Query
	if query == nil {
		query = ui.Local(root, "uikit.sider.query", func() string { return "" })
	}
	result := SiderResult{Root: root}
	root.Children(func() {
		icon := props.AddIcon
		if icon == nil {
			icon = IconSVG(IconPlus)
		}
		addLabel := props.AddLabel
		if addLabel == "" {
			addLabel = locales.CommonAdd()
		}
		header := SiderHeader(c, SiderHeaderProps{Title: title, AddLabel: addLabel, AddIcon: icon})
		result.AddButton, result.Add = header.Add, header.Add.Clicked()
		if len(props.AddMenu) != 0 {
			MenuOn(c, header.Add, props.AddMenu)
		}
		search := props.Search
		if search.Label == "" {
			search.Label = locales.MessageSearch() + title
		}
		if search.Placeholder == "" {
			search.Placeholder = search.Label
		}
		search.Variant = SearchSunken
		if search.Icon == nil {
			search.Icon = IconSVG(IconSearch)
		}
		result.SearchInput = SearchField(c.Key("host-search"), query, search).Input
		ui.Column(c).Grow(1).MinHeight(0).FillWidth().Role(ui.RoleList).Label(title).Children(body)
		if props.Footer != "" {
			footerIcon := props.FooterIcon
			if footerIcon == nil {
				footerIcon = IconSVG(IconServer)
			}
			SiderFooter(c, footerIcon, props.Footer)
		}
	})
	return result
}

type SiderHeaderProps struct {
	Title, AddLabel string
	AddIcon         *ui.SVG
	HideAdd         bool
}

type SiderHeaderParts struct {
	Root, Add ui.Element
}

func SiderHeader(c *ui.Context, props SiderHeaderProps) SiderHeaderParts {
	t := theme.Current()
	p := t.C.Sider
	var parts SiderHeaderParts
	header := PaneHeader(c, HeaderProps{Density: DensityTitle, Title: props.Title, Center: func() {
		if !props.HideAdd {
			parts.Add = IconButton(c, props.AddIcon, IconButtonProps{Label: props.AddLabel, Variant: IconFilled, Size: SizeMd}).
				Size(p.AddButtonSize, p.AddButtonSize).Radius(p.AddButtonRadius)
		}
	}})
	parts.Root = header.Root.Height(p.HeaderPaddingTop+p.AddButtonSize+p.HeaderPaddingBottom).
		Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).Gap(0)
	return parts
}

func SiderFooter(c *ui.Context, icon *ui.SVG, text string) ui.Element {
	t := theme.Current()
	p := t.C.Sider
	return ui.Row(c).Shrink(0).Gap(t.Space.N8).Padding(p.FooterPaddingY, p.FooterPaddingX).
		BorderWidth(t.Size.Hairline, 0, 0, 0).BorderColor(Color(p.FooterBorder)).TextColor(Color(p.FooterText)).Children(func() {
		Icon(c, icon, IconProps{Size: SizeSm, Tone: ToneSecondary}).PassThrough()
		Text(c, text, TextProps{Style: TextSmall, Tone: ToneSecondary}).PassThrough()
	})
}

type SiderGroupProps struct {
	Key               any
	Title             string
	Count             int
	Open              *bool
	Body              func()
	Chevron           *ui.SVG
	Motion            MotionMode
	ArrowAnimation    *ui.Animation
	DeferPresentation bool
}

type SiderGroupPresentation struct {
	Highlighted, DisableToggle, CountDown bool
}

type SiderGroupResult struct {
	Root    ui.Element
	Toggled bool
	Dropped any
	Over    bool
	props   SiderGroupProps
}

func SiderGroup(c *ui.Context, props SiderGroupProps) SiderGroupResult {
	p := theme.Current().C.GroupHeader
	result := SiderGroupResult{props: props, Root: ui.ButtonBase(c).Key(props.Key).Height(p.Height).
		Shrink(0).FillWidth().Padding(0, p.PaddingX).Gap(p.Gap).Radius(p.Radius).
		Label(props.Title).Expanded(*props.Open).TextColor(Color(p.Text))}
	if !props.DeferPresentation {
		result.Present(c, SiderGroupPresentation{})
	}
	return result
}

// Present is also used after the application's custom gesture controller has
// updated the original source node, without nesting it in a new group parent.
func (result *SiderGroupResult) Present(c *ui.Context, view SiderGroupPresentation) {
	props := result.props
	if !props.DeferPresentation {
		_, result.Over = ui.DragOver[any](result.Root)
		if value, dropped := ui.Drop[any](result.Root); dropped {
			result.Dropped = value
		}
	}
	view.Highlighted = view.Highlighted || result.Over
	if !view.DisableToggle && result.Root.Clicked() {
		*props.Open = !*props.Open
		result.Toggled = true
	}
	result.Root.Expanded(*props.Open)
	StyleSiderGroup(result.Root, view.Highlighted)
	if result.Root.FocusVisible() {
		Shadow(result.Root, theme.Current().Shadow.ControlFocus)
	}
	arrow := props.Chevron
	if arrow == nil {
		arrow = IconSVG(IconChevronDown)
	}
	animation := Anim(c, theme.Current().C.GroupHeader.Transition, props.Motion)
	if props.ArrowAnimation != nil {
		animation = *props.ArrowAnimation
	}
	result.Root.Children(func() {
		SiderGroupContents(c, SiderGroupContentProps{Title: props.Title, Expanded: *props.Open,
			Highlighted: view.Highlighted, Count: props.Count, CountDown: view.CountDown,
			Chevron: arrow, ArrowAnimation: animation, Motion: props.Motion})
	})
	if *props.Open && props.Body != nil {
		props.Body()
	}
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
	Motion                MotionMode
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
	if MotionReduced(c, props.Motion) {
		countAnimation = ui.Animation{Duration: t.Motion.Duration.InstantNonzero}
	}
	countInk := p.Count
	if props.Highlighted {
		countInk = p.DropCount
	}
	Count(c, props.Count, CountProps{Style: CountQuiet}).Key("count").TextColor(Color(countInk)).
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
	Key                    any
	Logo                   func()
	Title, Subtitle        string
	DragValue              any
	Menu                   []MenuEntry
	MoreLabel, Description string
	DeferPresentation      bool
	Motion                 MotionMode
}

type SiderRowPresentation struct {
	InteractionDisabled bool
	Flashing, Seeding   bool
	Animation           *ui.Animation
}

type SiderRowResult struct {
	Root, More ui.Element
	Open, Menu bool
	Dropped    any
	Over       bool
	Seeded     bool
	props      SiderRowProps
}

// SiderRow uses native focus for its transient active background, independently
// of open sessions. The caller does not provide selection or connection state.
func SiderRow(c *ui.Context, props SiderRowProps) SiderRowResult {
	t := theme.Current()
	p := t.C.SiderItem
	result := SiderRowResult{props: props, Root: ui.ButtonBase(c).Key(props.Key).
		Height(t.C.Catalog.RowHeight).Shrink(0).FillWidth().Padding(0, p.PaddingRight, 0, p.PaddingLeft).
		Gap(p.Gap).Radius(p.Radius).Role(ui.RoleListItem).Label(props.Title + "，" + props.Subtitle).
		Description(props.Subtitle).TextColor(Color(p.NameColor))}
	if !props.DeferPresentation {
		result.Present(c, SiderRowPresentation{})
	}
	return result
}

func (result *SiderRowResult) SetMenu(entries []MenuEntry) {
	result.props.Menu = entries
}

func (result *SiderRowResult) Present(c *ui.Context, view SiderRowPresentation) {
	t := theme.Current()
	p := t.C.SiderItem
	props, row := result.props, result.Root
	nativeLift := false
	if !props.DeferPresentation {
		if props.DragValue != nil {
			row.Drag(props.DragValue).Cursor(ui.CursorDefault)
			if row.Dragging() {
				nativeLift = true
				row.Cursor(ui.CursorGrabbing).Opacity(t.C.DragLift.SourceOpacity)
			}
		}
		_, result.Over = ui.DragOver[any](row)
		if value, dropped := ui.Drop[any](row); dropped {
			result.Dropped = value
		}
	}
	if !props.DeferPresentation && props.DragValue != nil {
		pose := ui.Pose{Scale: 1}
		shadow := float32(0)
		if nativeLift {
			shadow = 1
			if !MotionReduced(c, props.Motion) {
				pose.Scale, pose.Rotate = t.C.DragLift.StandardScale, t.C.DragLift.StandardRotation
			}
		}
		row.AnimatePose(pose, Anim(c, t.C.DragLift.LiftDuration, props.Motion))
		ShadowOpacity(row, t.C.DragLift.Shadow, Visual(row, "uikit.sider-row.lift", shadow, t.C.DragLift.LiftDuration))
	}
	background := ui.Transparent
	if !view.InteractionDisabled {
		if row.Focused() {
			background = Color(p.BgActive)
		} else if row.Hovered() {
			background = Color(p.BgHover)
		}
	}
	key := "row-bg"
	animation := Tween(p.Transition)
	if view.Animation != nil {
		animation = *view.Animation
	}
	if result.Over {
		background = Color(t.C.DropIndicator.ZoneBg)
	}
	if view.Flashing {
		key, background = "landing-bg", ui.Transparent
		if view.Seeding {
			background = Color(t.C.DropIndicator.FlashBg)
		}
	}
	shown := row.AnimateColor(key, background, animation)
	row.Background(shown)
	if view.Flashing {
		ShadowOpacity(row, t.C.DropIndicator.FlashShadow, float32(shown.A)/float32(t.C.DropIndicator.FlashBg.A))
	}
	if row.FocusVisible() && !view.InteractionDisabled {
		Shadow(row, t.Shadow.ControlFocus)
	}
	result.Seeded = shown == background
	if !view.InteractionDisabled {
		result.Open = row.DoubleClicked() || row.Focused() && row.Shortcut(0, ui.KeyEnter)
	}
	row.Children(func() {
		if props.Logo != nil {
			props.Logo()
		}
		SiderRowLabel(c, props.Title, props.Subtitle)
		label := props.MoreLabel
		if label == "" {
			label = locales.MessageMoreActionsFor() + props.Title
		}
		result.More = SiderMore(c, row, SiderMoreProps{Label: label, Description: props.Description,
			Icon: IconSVG(IconMore), Disabled: view.InteractionDisabled})
		// Even when dragging has removed the menu entries, the disabled trigger
		// must pass through RowMenuOn once to clear a previously open popover.
		if len(props.Menu) != 0 || view.InteractionDisabled {
			result.Menu = RowMenuOn(c, row, result.More, props.Menu)
		}
	})
}

func SiderRowLabel(c *ui.Context, title, subtitle string) {
	HostLabel(c, title, subtitle, HostLabelProps{Stacked: true, Profile: HostLabelCatalog}).Grow(1)
}

type SiderMoreProps struct {
	Label, Description string
	Icon               *ui.SVG
	Disabled           bool
}

func SiderMore(c *ui.Context, row ui.Element, props SiderMoreProps) ui.Element {
	t := theme.Current()
	p := t.C.SiderItem
	surface := *t
	surface.C.IconButton.BgHover = p.MoreBg
	action := IconButton(c, props.Icon, IconButtonProps{Key: "host-actions", Label: props.Label,
		Size: SizeSm, Variant: IconPlain, Disabled: props.Disabled, Surface: &surface}).Description(props.Description)
	target := float32(0)
	if !props.Disabled && (row.Hovered() || row.FocusVisible() || action.FocusVisible()) {
		target = 1
	}
	return action.Opacity(Visual(action, "more", target, p.Transition))
}
