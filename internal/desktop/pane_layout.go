package desktop

import (
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/workspace"

	"github.com/egoist/mygo/ui"
)

const maxTerminalPanes = workspace.MaxPanes

type terminalLayout = workspace.Layout[remoteSession]
type paneTab = workspace.Tab[remoteSession]

type panePlacement struct {
	session  *remoteSession
	rect     ui.Rect
	resizing bool
}

type splitPlacement struct {
	node         *terminalLayout
	rect, parent ui.Rect
}

func minimumLayoutSize(n *terminalLayout) ui.Point {
	t := theme.Current()
	if n.Session != nil {
		return ui.Point{X: t.C.Desktop.PaneMinWidth, Y: t.C.Desktop.PaneMinHeight}
	}
	a, b := minimumLayoutSize(n.First), minimumLayoutSize(n.Second)
	if n.Stacked {
		return ui.Point{X: max(a.X, b.X), Y: a.Y + b.Y + t.C.Pane.Gap}
	}
	return ui.Point{X: a.X + b.X + t.C.Pane.Gap, Y: max(a.Y, b.Y)}
}

func placeLayout(n *terminalLayout, r ui.Rect, panes *[]panePlacement, splits *[]splitPlacement) {
	if n.Session != nil {
		*panes = append(*panes, panePlacement{session: n.Session, rect: r})
		return
	}
	first, second, gap := divideLayout(n, r)
	*splits = append(*splits, splitPlacement{node: n, rect: gap, parent: r})
	placeLayout(n.First, first, panes, splits)
	placeLayout(n.Second, second, panes, splits)
}

func divideLayout(n *terminalLayout, r ui.Rect) (first, second, gap ui.Rect) {
	t := theme.Current()
	first, second, gap = r, r, r
	a, b := minimumLayoutSize(n.First), minimumLayoutSize(n.Second)
	if n.Stacked {
		available := r.H - t.C.Pane.Gap
		first.H = min(max(available*n.Ratio, a.Y), available-b.Y)
		second.Y, second.H = r.Y+first.H+t.C.Pane.Gap, available-first.H
		gap.Y, gap.H = r.Y+first.H, t.C.Pane.Gap
	} else {
		available := r.W - t.C.Pane.Gap
		first.W = min(max(available*n.Ratio, a.X), available-b.X)
		second.X, second.W = r.X+first.W+t.C.Pane.Gap, available-first.W
		gap.X, gap.W = r.X+first.W, t.C.Pane.Gap
	}
	return first, second, gap
}

// Preserve displayed proportions when removing a pane releases a size constraint.
func copyLayoutAt(n *terminalLayout, r ui.Rect) *terminalLayout {
	t := theme.Current()
	copy := *n
	if n.Session == nil {
		first, second, _ := divideLayout(n, r)
		copy.Ratio = first.W / (r.W - t.C.Pane.Gap)
		if n.Stacked {
			copy.Ratio = first.H / (r.H - t.C.Pane.Gap)
		}
		copy.First, copy.Second = copyLayoutAt(n.First, first), copyLayoutAt(n.Second, second)
	}
	return &copy
}

type dropZone = workspace.Side

const (
	dropCenter = workspace.Center
	dropLeft   = workspace.Left
	dropRight  = workspace.Right
	dropTop    = workspace.Top
	dropBottom = workspace.Bottom
)

func zoneAt(x, y, w, h float32, edgeOnly bool) dropZone {
	t := theme.Current()
	x, y = x/w, y/h
	edge := t.C.Desktop.DropEdgeRatio
	if !edgeOnly && x >= edge && x <= 1-edge && y >= edge && y <= 1-edge {
		return dropCenter
	}
	zone, nearest := dropLeft, x
	for _, edge := range []struct {
		zone     dropZone
		distance float32
	}{
		{zone: dropRight, distance: 1 - x},
		{zone: dropTop, distance: y},
		{zone: dropBottom, distance: 1 - y},
	} {
		if edge.distance < nearest {
			zone, nearest = edge.zone, edge.distance
		}
	}
	return zone
}
