// Package workspace owns pane topology, tab selection and saved workspace layouts.
package workspace

const MaxPanes = 4

// Layout is a split tree. A leaf owns one session and a branch owns two children.
type Layout[T any] struct {
	Session *T         `json:"session,omitempty"`
	First   *Layout[T] `json:"first,omitempty"`
	Second  *Layout[T] `json:"second,omitempty"`
	Stacked bool       `json:"stacked,omitempty"`
	Ratio   float32    `json:"ratio,omitempty"`
}

type Tab[T any] struct {
	Root      *Layout[T]
	Focused   *T
	Maximized bool
}

type Side uint8

const (
	Center Side = iota
	Left
	Right
	Top
	Bottom
)

func (n *Layout[T]) Insert(session *T, zone Side) {
	old := &Layout[T]{Session: n.Session}
	added := &Layout[T]{Session: session}
	n.Session, n.Stacked, n.Ratio = nil, zone == Top || zone == Bottom, .5
	n.First, n.Second = old, added
	if zone == Left || zone == Top {
		n.First, n.Second = added, old
	}
}

func (n *Layout[T]) Count() int {
	if n.Session != nil {
		return 1
	}
	return n.First.Count() + n.Second.Count()
}

func (n *Layout[T]) FirstSession() *T {
	if n.Session != nil {
		return n.Session
	}
	return n.First.FirstSession()
}

func (n *Layout[T]) Sessions() []*T {
	if n.Session != nil {
		return []*T{n.Session}
	}
	return append(n.First.Sessions(), n.Second.Sessions()...)
}

func (n *Layout[T]) Find(session *T) *Layout[T] {
	if n.Session != nil {
		if n.Session == session {
			return n
		}
		return nil
	}
	if found := n.First.Find(session); found != nil {
		return found
	}
	return n.Second.Find(session)
}

func (n *Layout[T]) Remove(session *T) *Layout[T] {
	if n.Session != nil {
		if n.Session == session {
			return nil
		}
		return n
	}
	n.First, n.Second = n.First.Remove(session), n.Second.Remove(session)
	if n.First == nil {
		return n.Second
	}
	if n.Second == nil {
		return n.First
	}
	return n
}

func NewTab[T any](pane *T) Tab[T] {
	return Tab[T]{Root: &Layout[T]{Session: pane}, Focused: pane}
}
