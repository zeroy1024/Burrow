package uikit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/assets"
	"github.com/zeroy1024/Burrow/internal/theme"
)

const (
	splashHoleTime       = 280 * time.Millisecond
	splashPeekAt         = 220 * time.Millisecond
	splashPeekTime       = 380 * time.Millisecond
	splashLeftEarAt      = 550 * time.Millisecond
	splashRightEarAt     = 700 * time.Millisecond
	splashEarTime        = 350 * time.Millisecond
	splashBlinkAt        = 819 * time.Millisecond
	splashBlinkTime      = 98 * time.Millisecond
	splashAnticipateAt   = 950 * time.Millisecond
	splashAnticipateTime = 72 * time.Millisecond
	splashDuckAt         = splashAnticipateAt + splashAnticipateTime
	splashDuckTime       = 178 * time.Millisecond
	splashMorphTime      = 470 * time.Millisecond
	splashVeilFadeAt     = 110 * time.Millisecond
	splashVeilFadeTime   = 415 * time.Millisecond
	splashHoleBounce     = .08
	splashPeekBounce     = .18
	splashRabbitRatio    = .266
	splashArtWidth       = 512
	splashArtHeight      = 446
	splashBackdropSize   = 128
)

var (
	splashBackdrop = splashSVG("backdrop")
	splashLeftEar  = splashSVG("left-ear")
	splashRightEar = splashSVG("right-ear")
	splashHead     = splashSVG("head")
	splashPrompt   = splashSVG("prompt")
	splashCursor   = splashSVG("cursor")
	splashInk      = ui.Hex("#111a2b")
	splashBlue     = Color(theme.Palette.Blue.N500)
)

func splashSVG(name string) *ui.SVG {
	data, err := assets.Splash.ReadFile("splash/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(data)
}

// StartupSplashFrame describes the current time in the startup scene and handoff.
type StartupSplashFrame struct {
	Age, OpeningAge, StillFade time.Duration
	Window                     ui.Color
	Opening, Reduced           bool
}

// StartupSplashOrigin is the point at which the logo and workspace meet.
func StartupSplashOrigin(width, height float32) ui.Point {
	return ui.Point{X: width * .5, Y: height * .595}
}

// DrawStartupSplash builds the vector artwork within the caller's input shield.
func DrawStartupSplash(c *ui.Context, frame StartupSplashFrame) {
	width, height := c.Size()
	origin := StartupSplashOrigin(width, height)
	opacity, morph := float32(1), float32(0)
	if frame.Opening {
		if frame.Reduced {
			opacity -= splashEase(frame.OpeningAge, frame.StillFade)
		} else {
			morph = splashEase(frame.OpeningAge, splashMorphTime)
			opacity -= splashEase(frame.OpeningAge-splashVeilFadeAt, splashVeilFadeTime)
		}
	}
	if opacity <= 0 {
		return
	}
	layer := ui.Box(c).Key("splash-scene").Absolute().Fill().PassThrough().Opacity(opacity)
	artScale := height * splashRabbitRatio / splashArtHeight
	hole, peek := float32(1), float32(1)
	if !frame.Reduced {
		hole = layer.AnimateTo("hole", splashAfter(frame.Age, 0), ui.Animation{Spring: Spring(
			theme.Spring{Duration: splashHoleTime, Bounce: splashHoleBounce},
		)})
		peek = layer.AnimateTo("peek", splashAfter(frame.Age, splashPeekAt), ui.Animation{Spring: Spring(
			theme.Spring{Duration: splashPeekTime, Bounce: splashPeekBounce},
		)})
	}
	holeWidth := splashMix(1, 583*artScale, hole)
	holeHeight := splashMix(1, 169*artScale, hole)
	size := ui.Point{X: splashMix(holeWidth, width, morph), Y: splashMix(holeHeight, height, morph)}
	center := ui.Point{X: origin.X, Y: splashMix(origin.Y, height/2, morph)}
	holeRect := ui.Rect{X: center.X - size.X/2, Y: center.Y - size.Y/2, W: size.X, H: size.Y}
	windowRadius := theme.Current().Radius.Window
	radius := ui.Point{X: splashMix(size.X/2, windowRadius, morph), Y: splashMix(size.Y/2, windowRadius, morph)}
	rim := (1 - morph) * min(1, hole)
	layer.Children(func() {
		// MyGo caches SVGs at their layout size. Stretch the small, smooth gradient
		// at paint time so a cold start doesn't rasterize a window-sized texture.
		ui.Image(c, splashBackdrop).Absolute().Size(splashBackdropSize, splashBackdropSize).
			Fit(ui.FillBox).TransformOrigin(0, 0).Scale(width/splashBackdropSize, height/splashBackdropSize).PassThrough()
		ui.Box(c).Absolute().Fill().PassThrough().Draw(func(p *ui.Painter, bounds ui.Rect) {
			if morph > 0 {
				p.Fill(bounds, frame.Window.Alpha(morph), 0)
				withSplashRoundRect(p, holeRect, radius, func(local ui.Rect) {
					p.Fill(local, splashInk.Mix(frame.Window, morph), radius.Y)
					p.Stroke(local, splashBlue.Alpha(.06*rim), radius.Y, 21*artScale)
					p.Stroke(local, splashBlue.Alpha(.13*rim), radius.Y, 10*artScale)
					p.Stroke(local, splashBlue.Alpha(.92*rim), radius.Y, 3.2*artScale)
				})
				return
			}
			outline := splashHolePath(holeRect, radius)
			p.FillPath(outline, splashInk)
			p.StrokePath(outline, 21*artScale, splashBlue.Alpha(.06*rim))
			p.StrokePath(outline, 10*artScale, splashBlue.Alpha(.13*rim))
			p.StrokePath(outline, 3.2*artScale, splashBlue.Alpha(.92*rim))
		})
		drawSplashRabbit(c, frame, origin, artScale, peek)
		if morph < 1 {
			ui.Box(c).Absolute().Fill().PassThrough().Draw(func(p *ui.Painter, _ ui.Rect) {
				if morph > 0 {
					withSplashRoundRect(p, holeRect, radius, func(local ui.Rect) {
						lip := ui.Rect{X: local.X, Y: local.Y + local.H - radius.Y, W: local.W, H: radius.Y}
						p.Clip(lip, 0, func() {
							p.Stroke(local, splashBlue.Alpha(.78*rim), radius.Y, 4.5*artScale)
							p.Stroke(local, ui.Hex("#8acbff").Alpha(.72*rim), radius.Y, 1.6*artScale)
						})
					})
					return
				}
				front := splashLowerLip(holeRect, radius)
				p.StrokePath(front, 4.5*artScale, splashBlue.Alpha(.78*rim))
				p.StrokePath(front, 1.6*artScale, ui.Hex("#8acbff").Alpha(.72*rim))
			})
		}
	})
}

// Stretch a native rounded rectangle to get elliptical corners without rebuilding
// and uploading path masks at each intermediate window size.
func withSplashRoundRect(p *ui.Painter, rect ui.Rect, radius ui.Point, draw func(ui.Rect)) {
	stretch := radius.X / radius.Y
	rect.W /= stretch
	p.Transform(ui.Scaling(stretch, 1), rect.X, rect.Y, func() { draw(rect) })
}

func drawSplashRabbit(c *ui.Context, frame StartupSplashFrame, origin ui.Point, scale, peek float32) {
	w, h := splashArtWidth*scale, splashArtHeight*scale
	top := origin.Y - h*peek
	var left, right, lean float32
	eye := float32(1)
	if !frame.Reduced {
		lift := splashProgress(frame.Age-splashPeekAt, splashPeekTime)
		lean = 7 * float32(math.Sin(math.Pi*float64(lift)))
		left = -lean - 17*splashEarFlick(frame.Age-splashLeftEarAt)
		right = -lean + 18*splashEarFlick(frame.Age-splashRightEarAt)
		blink := splashProgress(frame.Age-splashBlinkAt, splashBlinkTime)
		eye -= .92 * float32(math.Sin(math.Pi*float64(blink)))
		anticipate := splashEase(frame.Age-splashAnticipateAt, splashAnticipateTime)
		duck := splashProgress(frame.Age-splashDuckAt, splashDuckTime)
		top += h * (-.045*anticipate + 1.13*duck*duck)
	}
	if top >= origin.Y {
		return
	}
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(origin.Y).Clip().PassThrough().Children(func() {
		ui.Box(c).Absolute().Left(origin.X-w/2).Top(top).Size(w, h).PassThrough().
			TransformOrigin(.5, 1).Rotate(-lean * .23).Children(func() {
			ui.Image(c, splashLeftEar).Absolute().Fill().PassThrough().
				TransformOrigin((410.0-256)/splashArtWidth, (560.0-260)/splashArtHeight).Rotate(left)
			ui.Image(c, splashRightEar).Absolute().Fill().PassThrough().
				TransformOrigin((597.0-256)/splashArtWidth, (560.0-260)/splashArtHeight).Rotate(right)
			ui.Image(c, splashHead).Absolute().Fill().PassThrough()
			ui.Image(c, splashPrompt).Absolute().Fill().PassThrough()
			ui.Image(c, splashCursor).Absolute().Fill().PassThrough().Opacity(eye)
		})
	})
}

const splashCurve = .55228475

func splashHolePath(r ui.Rect, corner ui.Point) *ui.Path {
	rx, ry := corner.X, corner.Y
	right, bottom := r.X+r.W, r.Y+r.H
	dx, dy := rx*splashCurve, ry*splashCurve
	p := &ui.Path{}
	return p.MoveTo(r.X+rx, r.Y).
		LineTo(right-rx, r.Y).
		CubeTo(right-rx+dx, r.Y, right, r.Y+ry-dy, right, r.Y+ry).
		LineTo(right, bottom-ry).
		CubeTo(right, bottom-ry+dy, right-rx+dx, bottom, right-rx, bottom).
		LineTo(r.X+rx, bottom).
		CubeTo(r.X+rx-dx, bottom, r.X, bottom-ry+dy, r.X, bottom-ry).
		LineTo(r.X, r.Y+ry).
		CubeTo(r.X, r.Y+ry-dy, r.X+rx-dx, r.Y, r.X+rx, r.Y).Close()
}

func splashLowerLip(r ui.Rect, corner ui.Point) *ui.Path {
	rx, ry := corner.X, corner.Y
	right, bottom := r.X+r.W, r.Y+r.H
	dx, dy := rx*splashCurve, ry*splashCurve
	p := &ui.Path{}
	return p.MoveTo(right, bottom-ry).
		CubeTo(right, bottom-ry+dy, right-rx+dx, bottom, right-rx, bottom).
		LineTo(r.X+rx, bottom).
		CubeTo(r.X+rx-dx, bottom, r.X, bottom-ry+dy, r.X, bottom-ry)
}

func splashAfter(age, start time.Duration) float32 {
	if age > start {
		return 1
	}
	return 0
}

func splashProgress(age, duration time.Duration) float32 {
	return float32(min(1, max(0, float64(age)/float64(duration))))
}

func splashEase(age, duration time.Duration) float32 {
	t := splashProgress(age, duration)
	return t * t * (3 - 2*t)
}

func splashMix(from, to, progress float32) float32 {
	return from + (to-from)*progress
}

func splashEarFlick(age time.Duration) float32 {
	t := float64(splashProgress(age, splashEarTime))
	return float32(math.Sin(3.2*math.Pi*t) * (1 - t) * (1 - t))
}
