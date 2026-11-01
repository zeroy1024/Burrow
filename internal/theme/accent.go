package theme

import "image/color"

// WithAccent rebuilds the accent-dependent tokens without mutating the source
// theme. Protocol and status colors retain their independent meanings.
func WithAccent(t *Theme, c color.NRGBA) *Theme {
	s := t.Semantic
	s.Color.Accent.Default = c
	s.Color.Accent.Tint035 = WithAlpha(c, .035)
	s.Color.Accent.Tint06 = WithAlpha(c, .06)
	s.Color.Accent.Tint08 = WithAlpha(c, .08)
	s.Color.Accent.Tint10 = WithAlpha(c, .10)
	s.Color.Accent.Tint13 = WithAlpha(c, .13)
	s.Color.Accent.Tint15 = WithAlpha(c, .15)
	s.Color.Accent.Tint16 = WithAlpha(c, .16)
	s.Color.Accent.Tint18 = WithAlpha(c, .18)
	s.Color.Accent.Tint28 = WithAlpha(c, .28)
	s.Color.Accent.Tint35 = WithAlpha(c, .35)
	s.Color.Accent.Tint50 = WithAlpha(c, .50)
	mix, textBase := .09, color.NRGBAModel.Convert(color.Black).(color.NRGBA)
	if t.IsDark {
		mix, textBase = .18, color.NRGBAModel.Convert(color.White).(color.NRGBA)
		s.Color.Accent.Tint16 = WithAlpha(c, .24)
	}
	s.Color.Accent.Subtle = Mix(s.Color.Surface.Card, c, mix)
	s.Color.Text.Accent = Mix(c, textBase, .15)
	s.Color.Text.AccentStrong = Mix(c, textBase, .25)
	s.Color.State.SelectedAccent = s.Color.Accent.Subtle

	// Shadows are slices, so copy the affected layers before recoloring them.
	s.Shadow.FocusRing = recolorShadow(s.Shadow.FocusRing, c)
	s.Shadow.InputFocus = recolorShadow(s.Shadow.InputFocus, c)
	s.Shadow.InputFocus[1].Color = s.Color.Accent.Tint16
	s.Shadow.Flash = recolorShadow(s.Shadow.Flash, c)
	s.Shadow.PlaceholderAccent = recolorShadow(s.Shadow.PlaceholderAccent, c)
	s.Shadow.ThemeSelected = recolorShadow(s.Shadow.ThemeSelected, c)
	s.Shadow.ThemeSelected[0].Color = s.Color.Accent.Tint16
	return New(t.Name, t.IsDark, s)
}

func recolorShadow(source Shadow, c color.NRGBA) Shadow {
	layers := append(Shadow(nil), source...)
	for i := range layers {
		alpha := layers[i].Color.A
		layers[i].Color = c
		layers[i].Color.A = alpha
	}
	return layers
}
