package persistence

type request struct {
	work func() error
	done func(error)
}

// Writer executes database writes in order and dispatches their results to the owner.
type Writer struct {
	requests chan request
	stopped  chan struct{}
}

func NewWriter(post func(func())) *Writer {
	p := &Writer{requests: make(chan request, 64), stopped: make(chan struct{})}
	go func() {
		defer close(p.stopped)
		for r := range p.requests {
			err := r.work()
			if r.done != nil {
				post(func() { r.done(err) })
			}
		}
	}()
	return p
}

func (p *Writer) Close() {
	close(p.requests)
	<-p.stopped
}

func (p *Writer) Submit(work func() error, done func(error)) {
	p.requests <- request{work: work, done: done}
}
