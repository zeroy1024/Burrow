package uikit

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
)

type BadgeProps struct {
	Tone    Tone
	Dot     bool
	Size    Size
	Surface *theme.Theme
}

func Badge(c *ui.Context, text string, props BadgeProps) ui.Element {
	t := theme.Current()
	if props.Surface != nil {
		t = props.Surface
	}
	p := t.C.Badge
	height, font, padding := p.HeightSm, p.FontSizeCompact, p.PaddingXCompact
	if props.Size == SizeMd || props.Size == SizeLg {
		height, font, padding = p.HeightMd, p.FontSize, p.PaddingX
	}
	background, ink, dot := p.NeutralBg, p.NeutralText, p.NeutralDot
	switch props.Tone {
	case ToneAccent:
		background, ink, dot = p.RunBg, p.RunText, p.RunDot
	case ToneSuccess:
		background, ink, dot = p.OkBg, p.OkText, p.OkDot
	case ToneWarning:
		background, ink, dot = p.WarnBg, p.WarnText, p.WarnDot
	case ToneDanger:
		background, ink, dot = p.ErrBg, p.ErrText, p.ErrDot
	case ToneMuted, ToneSecondary, ToneTertiary:
		background, ink, dot = p.MutedBg, p.MutedText, p.NeutralDot
	case ToneOnAccent:
		background, ink, dot = t.Color.Accent.Default, t.Color.Text.OnAccent, t.Color.Text.OnAccent
	}
	return ui.Row(c).Height(height).Shrink(0).Padding(0, padding).Gap(p.Gap).AlignItems(ui.Center).
		Radius(p.Radius).Background(Color(background)).TextColor(Color(ink)).Children(func() {
		if props.Dot {
			ui.Box(c).Size(t.Size.DotSm, t.Size.DotSm).Radius(p.Radius).Background(Color(dot)).PassThrough()
		}
		ui.Text(c, text).FontSize(font).SingleLine().PassThrough()
	})
}

type CountStyle uint8

const (
	CountQuiet CountStyle = iota
	CountAccent
	CountInline
)

type CountProps struct {
	Style CountStyle
	Max   int
}

func Count(c *ui.Context, count int, props CountProps) ui.Element {
	t := theme.Current()
	label := strconv.Itoa(count)
	if props.Max > 0 {
		label = strconv.Itoa(max(0, count))
		if count > props.Max {
			label = strconv.Itoa(props.Max) + "+"
		}
	}
	if props.Style == CountInline {
		label = "+" + label
	}
	e := ui.Text(c, label).FontFeatures("tnum").SingleLine().PassThrough()
	switch props.Style {
	case CountAccent:
		e.Height(t.C.Kbd.Height).MinWidth(t.C.Kbd.MinWidth).Padding(0, t.Space.N4).Radius(t.Radius.Pill).
			FontSize(t.Font.Size.Caption).FixedLineHeight(t.C.Kbd.Height).TextAlign(ui.Center).
			Background(Color(t.Color.Accent.Default)).TextColor(Color(t.Color.Text.OnAccent))
	case CountInline:
		e.FontSize(t.Font.Size.Caption).TextColor(Color(t.C.Tab.Text))
	default:
		e.FontSize(t.C.GroupHeader.CountSize).TextColor(Color(t.C.GroupHeader.Count))
	}
	return e
}

type StatusPillProps struct {
	Surface *theme.Theme
}

func StatusPill(c *ui.Context, status Status, options ...StatusPillProps) ui.Element {
	props := StatusPillProps{}
	if len(options) != 0 {
		props = options[0]
	}
	label, tone := locales.MessageConnected(), ToneSuccess
	switch status {
	case StatusConnecting:
		label, tone = locales.MessageConnecting2(), ToneWarning
	case StatusDisconnected:
		label, tone = locales.MessageDisconnected(), ToneNeutral
	case StatusError:
		label, tone = locales.MessageFailed(), ToneDanger
	case StatusRunning:
		label, tone = locales.MessageInProgress(), ToneAccent
	case StatusDone:
		label, tone = locales.MessageCompleted(), ToneSuccess
	case StatusPaused:
		label, tone = locales.MessagePaused(), ToneMuted
	case StatusIntercepted:
		label, tone = locales.MessageBlockedByPolicy(), ToneWarning
	case StatusPartialIntercepted:
		label, tone = locales.MessagePartiallyBlocked(), ToneWarning
	case StatusPermissionDenied:
		label, tone = locales.MessagePermissionDenied(), ToneDanger
	case StatusAuthFailed:
		label, tone = locales.MessageAuthenticationFailed(), ToneDanger
	case StatusPartialFailed:
		label, tone = locales.MessagePartiallyFailed(), ToneWarning
	case StatusRevoked:
		label, tone = locales.MessageRevoked(), ToneMuted
	case StatusDisabled:
		label, tone = locales.MessageDisabled(), ToneWarning
	case StatusEnabled:
		label, tone = locales.MessageEnabled(), ToneSuccess
	case StatusStopping:
		label, tone = locales.MessageStopping(), ToneWarning
	case StatusInterrupted:
		label, tone = locales.MessageInterruptedUnknown(), ToneMuted
	}
	return Badge(c, label, BadgeProps{Tone: tone, Dot: true, Surface: props.Surface}).Role(ui.RoleStatus).Label(label)
}

// ActivityBadgeKind selects an existing terminal activity presentation, not a
// connection state. Callers decide which snapshot notifications are present.
type ActivityBadgeKind uint8

const (
	ActivityNewOutput ActivityBadgeKind = iota
	ActivityBell
)

func ActivityBadge(c *ui.Context, kind ActivityBadgeKind, surface *theme.Theme) ui.Element {
	t := theme.Current()
	if surface != nil {
		t = surface
	}
	p := t.C.ActivityBadge
	badge := ui.Row(c).Shrink(0).Padding(p.PaddingY, p.PaddingRight, p.PaddingY, p.PaddingLeft).
		Radius(p.Radius)
	if kind == ActivityBell {
		return Tip(c, badge.Label(
			locales.MessageSessionBell(),
		), locales.MessageSessionBell()).Background(Color(p.BellBg)).Children(func() {
			ui.Icon(c, IconSVG(IconBell)).Size(p.BellIconSize, p.BellIconSize).TextColor(Color(p.BellText)).PassThrough()
		})
	}
	return badge.Gap(p.Gap).Background(Color(p.OutputBg)).PassThrough().Children(func() {
		ui.Box(c).Size(p.DotSize, p.DotSize).Radius(t.Radius.Pill).Background(Color(p.OutputDot)).PassThrough()
		ui.Text(c, locales.MessageNewOutput()).FontSize(p.FontSize).TextColor(Color(p.OutputText)).SingleLine().PassThrough()
	})
}
