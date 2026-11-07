package uikit

import (
	"math"
	"time"

	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

func Spring(s theme.Spring) ui.SpringOptions {
	if s.Stiffness != 0 || s.Damping != 0 {
		return ui.SpringOptions{Stiffness: s.Stiffness, Damping: s.Damping}
	}
	return ui.SpringOptions{Duration: s.Duration, Bounce: s.Bounce}
}

func Easing(b theme.Bezier) ui.Easing {
	return ui.CubicBezier(b[0], b[1], b[2], b[3])
}

func Tween(duration time.Duration) ui.Animation {
	return ui.Animation{Duration: duration, Ease: Easing(theme.Current().Motion.Easing.Out)}
}

func MotionReduced(c *ui.Context, mode MotionMode) bool {
	switch mode {
	case MotionReduce:
		return true
	case MotionAllow:
		return false
	}
	return c.Preferences().ReduceMotion
}

func MotionOverride(reduced bool) MotionMode {
	if reduced {
		return MotionReduce
	}
	return MotionAllow
}

func Dur(c *ui.Context, duration time.Duration, mode ...MotionMode) time.Duration {
	policy := MotionSystem
	if len(mode) != 0 {
		policy = mode[0]
	}
	if MotionReduced(c, policy) {
		return theme.Current().Motion.Duration.InstantNonzero
	}
	return duration
}

func Anim(c *ui.Context, duration time.Duration, mode ...MotionMode) ui.Animation {
	return Tween(Dur(c, duration, mode...))
}

// Visual encodes opacity in an alpha channel so reduced motion keeps color
// and opacity fades while MyGo suppresses positional scalar animations.
func Visual(e ui.Element, key any, target float32, duration time.Duration) float32 {
	c := ui.Color{A: uint8(math.Round(float64(target * 255)))}
	value := e.AnimateColor(key, c, Tween(duration))
	if value.A == c.A {
		return target
	}
	return float32(value.A) / 255
}

// Reduced resolves the application's preference against the system setting.
func Reduced(c *ui.Context, mode string) bool {
	switch mode {
	case "on":
		return true
	case "off":
		return false
	}
	return c.Preferences().ReduceMotion
}

// Position suppresses application-controlled movement. MyGo still applies the
// system preference itself; a zero Animation would select its default spring.
func Position(animation ui.Animation, mode string) ui.Animation {
	if mode == "on" {
		return ui.Animation{Duration: theme.Current().Motion.Duration.InstantNonzero}
	}
	return animation
}
