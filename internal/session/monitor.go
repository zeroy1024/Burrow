package session

import (
	"context"
	"errors"
	"time"

	"github.com/zeroy1024/Burrow/internal/hostinfo"
)

const (
	sampleInterval = 2 * time.Second
	sampleTimeout  = 5 * time.Second
	MonitorGap     = sampleInterval + sampleTimeout
)

// Monitor samples on a worker until the last shell or transport closes.
// publish receives the wall-clock sample time, including gaps across sleep.
func (s *ShellServices) Monitor(sampler hostinfo.Sampler, publish func(time.Time, hostinfo.Metrics, error)) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	var last time.Time
	for s.ctx.Err() == nil {
		select {
		case <-s.ctx.Done():
			return
		case <-s.Connection().Done():
			return
		case <-timer.C:
		}
		started := time.Now().Round(0)
		metrics, err := s.sample(sampler)
		at := time.Now().Round(0)
		if err == nil {
			if at.Before(started) || at.Sub(started) > sampleTimeout {
				sampler.Reset()
				timer.Reset(0)
				continue
			}
			if !last.IsZero() && (at.Before(last) || at.Sub(last) > MonitorGap) {
				metrics.CPU, metrics.Down, metrics.Up, metrics.Top = nil, nil, nil, nil
			}
			last = at
		}
		publish(at, metrics, err)
		if errors.Is(err, hostinfo.ErrUnsupported) {
			return
		}
		timer.Reset(sampleInterval)
	}
}

func (s *ShellServices) sample(sampler hostinfo.Sampler) (hostinfo.Metrics, error) {
	release, err := s.Connection().Retain()
	if err != nil {
		return hostinfo.Metrics{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(s.ctx, sampleTimeout)
	defer cancel()
	return sampler.Sample(ctx)
}
