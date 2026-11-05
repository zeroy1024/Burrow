package uikit

import (
	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

type SectionProps struct {
	Key            any
	Title, Summary string
	Chevron        *ui.SVG
	Reduced        bool
}

func Section(c *ui.Context, open *bool, contentHeight *float32, props SectionProps, body func()) ui.Element {
	t := theme.Current()
	p := t.C.Collapse
	var trigger ui.Element
	root := ui.Column(c).FillWidth().BorderWidth(t.Size.Divider, 0, 0, 0).BorderColor(Color(t.C.Modal.Divider))
	if props.Key != nil {
		root.Key(props.Key)
	}
	root.Children(func() {
		parts := ui.CollapsibleBase(c, open)
		trigger = parts.Trigger.FillWidth().Height(p.HeaderHeight).Padding(0, t.C.Modal.BodyPaddingX).
			Gap(t.Space.N8).AlignItems(ui.Center).Label(props.Title)
		target := float32(0)
		if *open {
			target = 1
		}
		progress := trigger.AnimateTo("advanced-progress", target, Tween(p.Duration))
		if props.Reduced {
			progress = target
		}
		trigger.Children(func() {
			ui.Icon(c, props.Chevron).Size(p.Chevron, p.Chevron).Rotate(-p.ChevronRotate + p.ChevronRotate*progress).
				TextColor(Color(p.ChevronColor)).PassThrough()
			ui.Text(c, props.Title).FontSize(p.TitleSize).FontWeight(p.TitleWeight).TextColor(Color(p.Title)).PassThrough()
			summary := ui.Text(c, props.Summary).Grow(1).MinWidth(0).SingleLine().FontSize(p.SummarySize).TextColor(Color(p.Summary)).PassThrough()
			summary.Opacity(Visual(summary, "summary", 1-target, t.Motion.Duration.Exit))
		})
		if !*open && progress == 0 {
			return
		}
		clip := ui.Column(c).FillWidth().Shrink(0).ClipY().Disabled(!*open)
		if progress < 1 {
			clip.Height(progress * *contentHeight)
		}
		clip.Children(func() {
			inner := ui.Column(c).FillWidth().Shrink(0).Padding(0, t.C.Modal.BodyPaddingX, t.Space.N20, t.C.Modal.BodyPaddingX).Gap(t.C.FormRow.RowGap)
			inner.Opacity(min(1, progress*p.ContentFadeFactor))
			inner.Children(body)
			if height := inner.Bounds().H; height > 0 {
				*contentHeight = height
			}
		})
		if *contentHeight == 0 {
			c.AnimationFrame()
		}
	})
	return trigger
}

type DisclosureProps struct {
	Title string
	// TitleStyle and TitleTone match the headings around the disclosure.
	TitleStyle TextStyle
	TitleTone  Tone
	// Meta sits at the trailing end of the header, before the arrow.
	Meta   string
	Motion MotionMode
}

// Disclosure is a collapsible section within a panel or drawer: a header
// with the title, optional meta and an arrow that turns as body opens below.
func Disclosure(c *ui.Context, open *bool, props DisclosureProps, body func()) ui.Element {
	t := theme.Current()
	p := t.C.Collapse
	return ui.Column(c).FillWidth().Children(func() {
		parts := ui.CollapsibleBase(c, open)
		reduced := MotionReduced(c, props.Motion)
		progress := parts.Progress()
		if reduced {
			progress = 0
			if *open {
				progress = 1
			}
		}
		trigger := parts.Trigger.FillWidth().Gap(t.Space.N4).Radius(t.Radius.ControlSm).FocusRing(false).Label(props.Title)
		if trigger.FocusVisible() {
			Shadow(trigger, t.C.Button.FocusShadow)
		}
		trigger.Children(func() {
			Text(c, props.Title, TextProps{Style: props.TitleStyle, Tone: props.TitleTone}).SingleLine().PassThrough()
			ui.Spacer(c).PassThrough()
			if props.Meta != "" {
				Text(c, props.Meta, TextProps{Style: TextCaption, Tone: ToneTertiary}).FontFeatures("tnum").SingleLine().PassThrough()
			}
			ui.Icon(c, IconSVG(IconChevronDown)).Size(p.Chevron, p.Chevron).
				Rotate(-p.ChevronRotate * (1 - progress)).TextColor(Color(p.ChevronColor)).PassThrough()
		})
		panel := func() {
			ui.Column(c).FillWidth().Padding(t.Space.N10, 0, 0, 0).Children(body)
		}
		// The SDK's panel only honors the system motion preference.
		if reduced {
			if *open {
				panel()
			}
			return
		}
		parts.Panel(panel)
	})
}
