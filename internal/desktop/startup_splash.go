package desktop

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

const (
	splashOpeningAt    = 1120 * time.Millisecond
	splashStillTime    = 300 * time.Millisecond
	splashStillFade    = 160 * time.Millisecond
	splashOpeningTime  = 880 * time.Millisecond
	splashDeadline     = 2700 * time.Millisecond
	splashPartLead     = 70 * time.Millisecond
	splashPartStagger  = 45 * time.Millisecond
	splashPartTime     = 450 * time.Millisecond
	splashPartScaleLag = 40 * time.Millisecond
	splashInitialScale = .10
	splashPartBounce   = .12
)

type splashPart uint8

const (
	splashSidebar splashPart = iota
	splashRail
	splashTabs
	splashMainPane
	splashSplitPane
	splashStatus
)

type startupSplash struct {
	started, opening time.Time
	played, visible  bool
	reduced          bool
	returnFocus      bool
}

func (s *startupSplash) arm() {
	if !s.played {
		s.played, s.visible = true, true
	}
}

func (s *startupSplash) dismiss() {
	if s.visible {
		s.visible, s.returnFocus = false, true
	}
}

func (s *startupSplash) advance(now time.Time, ready, reduced bool) {
	if !s.visible {
		return
	}
	if s.started.IsZero() {
		s.started, s.reduced = now, reduced
	}
	if now.Sub(s.started) >= splashDeadline || reduced && !s.reduced {
		s.dismiss()
		return
	}
	start, duration := splashOpeningAt, splashOpeningTime
	if s.reduced {
		start, duration = splashStillTime, splashStillFade
	}
	if ready && s.opening.IsZero() && now.Sub(s.started) >= start {
		s.opening = now
	}
	if !s.opening.IsZero() && now.Sub(s.opening) >= duration {
		s.dismiss()
	}
}

func (s *startupSplash) behindWorkspace() bool {
	return s.visible && !s.reduced && !s.opening.IsZero()
}

func (a *app) advanceStartupSplash(c *ui.Context) {
	if !a.splash.visible {
		return
	}
	switch a.security.status {
	case "locked", "migration", "blocked":
		a.splash.dismiss()
	default:
		a.splash.advance(c.Now(), a.security.ready, a.reduceMotion(c) || c.Preferences().ReduceMotion)
	}
}

func (a *app) drawStartupSplash(c *ui.Context) {
	s := &a.splash
	frame := uikit.StartupSplashFrame{
		Age:       c.Now().Sub(s.started),
		Window:    uikit.Color(theme.Current().C.Window.Bg),
		Reduced:   s.reduced,
		Opening:   !s.opening.IsZero(),
		StillFade: splashStillFade,
	}
	if frame.Opening {
		frame.OpeningAge = c.Now().Sub(s.opening)
	}
	uikit.DrawStartupSplash(c, frame)
}

func (a *app) startupSplashOverlay(c *ui.Context) {
	s := &a.splash
	ui.Overlay(c.Key("startup-splash"), func() {
		shield := uikit.InputShield(c).Modal().AutoFocus().Role(ui.RoleButton).Label(locales.MessageSkip()).
			OnClick(s.dismiss).HandleInput(func(event ui.InputEvent) bool {
			switch event.Kind {
			case ui.InputKeyDown, ui.InputPointerDown:
				s.dismiss()
			}
			return true
		})
		if !s.behindWorkspace() {
			shield.Children(func() { a.drawStartupSplash(c) })
		}
	})
	c.AnimationFrame()
}

func (s *startupSplash) pose(c *ui.Context, part ui.Element, order splashPart) {
	if !s.visible || s.reduced {
		return
	}
	delay := splashPartLead + time.Duration(order)*splashPartStagger
	moving := !s.opening.IsZero() && c.Now().Sub(s.opening) >= delay
	start := ui.Pose{}
	targetScale := float32(1)
	motion := ui.Animation{Spring: uikit.Spring(theme.Spring{Duration: splashPartTime, Bounce: splashPartBounce})}
	if !moving {
		bounds := part.LayoutBounds()
		width, height := c.Size()
		origin := uikit.StartupSplashOrigin(width, height)
		start = ui.Pose{
			X:      origin.X - bounds.X - bounds.W/2,
			Y:      origin.Y - bounds.Y - bounds.H/2,
			Rotate: float32(order)*2.4 - 6,
			Fade:   1,
		}
		targetScale = splashInitialScale
		motion = ui.Animation{Duration: time.Nanosecond}
	}
	scaling := motion
	if moving {
		scaling.Delay = splashPartScaleLag
	}
	scale := part.AnimateTo("startup-splash-scale", targetScale, scaling)
	// Composite the moving control as one layer while its descendants stay at layout size.
	part.Cache().TransformOrigin(.5, .5).Scale(scale, scale).AnimatePose(start, motion)
}
