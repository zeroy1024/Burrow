package desktop

import (
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo/ui"
)

const maxTerminalPanes = 4

type terminalLayout struct {
	session *remoteSession
	first   *terminalLayout
	second  *terminalLayout
	stacked bool
	ratio   float32
}

type panePlacement struct {
	session *remoteSession
	rect    ui.Rect
}

type splitPlacement struct {
	node         *terminalLayout
	rect, parent ui.Rect
}

func (n *terminalLayout) minimum() ui.Point {
	t := theme.Current()
	if n.session != nil {
		return ui.Point{X: t.C.Desktop.PaneMinWidth, Y: t.C.Desktop.PaneMinHeight}
	}
	a, b := n.first.minimum(), n.second.minimum()
	if n.stacked {
		return ui.Point{X: max(a.X, b.X), Y: a.Y + b.Y + t.C.Pane.Gap}
	}
	return ui.Point{X: a.X + b.X + t.C.Pane.Gap, Y: max(a.Y, b.Y)}
}

func (n *terminalLayout) placements(r ui.Rect, panes *[]panePlacement, splits *[]splitPlacement) {
	if n.session != nil {
		*panes = append(*panes, panePlacement{session: n.session, rect: r})
		return
	}
	first, second, gap := n.divide(r)
	*splits = append(*splits, splitPlacement{node: n, rect: gap, parent: r})
	n.first.placements(first, panes, splits)
	n.second.placements(second, panes, splits)
}

func (n *terminalLayout) divide(r ui.Rect) (first, second, gap ui.Rect) {
	t := theme.Current()
	first, second, gap = r, r, r
	a, b := n.first.minimum(), n.second.minimum()
	if n.stacked {
		available := r.H - t.C.Pane.Gap
		first.H = min(max(available*n.ratio, a.Y), available-b.Y)
		second.Y, second.H = r.Y+first.H+t.C.Pane.Gap, available-first.H
		gap.Y, gap.H = r.Y+first.H, t.C.Pane.Gap
	} else {
		available := r.W - t.C.Pane.Gap
		first.W = min(max(available*n.ratio, a.X), available-b.X)
		second.X, second.W = r.X+first.W+t.C.Pane.Gap, available-first.W
		gap.X, gap.W = r.X+first.W, t.C.Pane.Gap
	}
	return first, second, gap
}

// Preserve displayed proportions when removing a pane releases a size constraint.
func (n *terminalLayout) copyAt(r ui.Rect) *terminalLayout {
	t := theme.Current()
	copy := *n
	if n.session == nil {
		first, second, _ := n.divide(r)
		copy.ratio = first.W / (r.W - t.C.Pane.Gap)
		if n.stacked {
			copy.ratio = first.H / (r.H - t.C.Pane.Gap)
		}
		copy.first, copy.second = n.first.copyAt(first), n.second.copyAt(second)
	}
	return &copy
}

type dropZone uint8

const (
	dropCenter dropZone = iota
	dropLeft
	dropRight
	dropTop
	dropBottom
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

func (n *terminalLayout) insert(session *remoteSession, zone dropZone) {
	old := &terminalLayout{session: n.session}
	added := &terminalLayout{session: session}
	n.session, n.stacked, n.ratio = nil, zone == dropTop || zone == dropBottom, .5
	n.first, n.second = old, added
	if zone == dropLeft || zone == dropTop {
		n.first, n.second = added, old
	}
}

func (n *terminalLayout) count() int {
	if n.session != nil {
		return 1
	}
	return n.first.count() + n.second.count()
}

func (n *terminalLayout) firstSession() *remoteSession {
	if n.session != nil {
		return n.session
	}
	return n.first.firstSession()
}

func (n *terminalLayout) sessions() []*remoteSession {
	if n.session != nil {
		return []*remoteSession{n.session}
	}
	return append(n.first.sessions(), n.second.sessions()...)
}

func (n *terminalLayout) find(session *remoteSession) *terminalLayout {
	if n.session != nil {
		if n.session == session {
			return n
		}
		return nil
	}
	if found := n.first.find(session); found != nil {
		return found
	}
	return n.second.find(session)
}

func (n *terminalLayout) remove(session *remoteSession) *terminalLayout {
	if n.session != nil {
		if n.session == session {
			return nil
		}
		return n
	}
	n.first, n.second = n.first.remove(session), n.second.remove(session)
	if n.first == nil {
		return n.second
	}
	if n.second == nil {
		return n.first
	}
	return n
}
