package theme

import (
	"image/color"
	"testing"
)

func TestLightMatchesMockups(t *testing.T) {
	th := Light()
	checks := map[string][2]color.NRGBA{
		"window bg":    {th.C.Window.Bg, Hex("#e7e7e8")},
		"card":         {th.C.Card.Bg, Hex("#ffffff")},
		"pane bg":      {th.C.Pane.Bg, Hex("#fbfbfc")},
		"accent":       {th.Color.Accent.Default, Hex("#0a84ff")},
		"text primary": {th.Color.Text.Primary, Hex("#1d1d1f")},
		"selected row": {th.C.SiderItem.BgSelected, Hex("#efeff1")},
		"rail on":      {th.C.Rail.BgOn, Hex("#d6d6d8")},
		"focus ring":   {th.C.PaneFocus.RingFocused[0].Color, Hex("#0a84ff")},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %v, want %v", name, c[0], c[1])
		}
	}
	if th.C.PaneFocus.ContentOpacityUnfocused != 0.5 || th.C.PaneFocus.Transition.Milliseconds() != 160 {
		t.Error("focus D values changed")
	}
	if th.C.ReorderMotion.Displace.Stiffness != 420 || th.C.ReorderMotion.Displace.Damping != 34 {
		t.Error("spring changed")
	}
}

func TestSwap(t *testing.T) {
	var got *Theme
	cancel := OnChange(func(x *Theme) { got = x })
	defer cancel()
	Set(Dark())
	if got == nil || !Current().IsDark || Current().C.Window.Bg == Light().C.Window.Bg {
		t.Fatal("dark swap failed")
	}
	// components follow semantic overrides automatically
	if Current().C.Sider.Bg != Current().Color.Surface.Card {
		t.Fatal("component not derived from semantic")
	}
	Set(Light())
	if Current().IsDark {
		t.Fatal("swap back failed")
	}
}
