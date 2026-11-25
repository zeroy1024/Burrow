package workspace

import "slices"

type Tabs[T any] struct {
	Items  []*T
	Active int
}

func (t *Tabs[T]) Current() *T {
	if len(t.Items) == 0 {
		return nil
	}
	return t.Items[t.Active]
}

func (t *Tabs[T]) Remove(index int) {
	t.Items = slices.Delete(t.Items, index, index+1)
	switch {
	case len(t.Items) == 0:
		t.Active = 0
	case index < t.Active:
		t.Active--
	case t.Active >= len(t.Items):
		t.Active = len(t.Items) - 1
	}
}

func (t *Tabs[T]) Move(from, to int) {
	active, item := t.Current(), t.Items[from]
	t.Items = slices.Delete(t.Items, from, from+1)
	t.Items = slices.Insert(t.Items, to, item)
	t.Active = slices.Index(t.Items, active)
}

func (t *Tabs[T]) RestoreOrder(order []*T) {
	active := t.Current()
	t.Items = slices.Clone(order)
	t.Active = slices.Index(t.Items, active)
}
