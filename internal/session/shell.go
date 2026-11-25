package session

import (
	"context"
	"errors"
	"time"

	"github.com/zeroy1024/Burrow/internal/sshclient"
)

// Shell owns a single PTY channel, independently of any terminal view.
// Open runs on a worker; Close and the ready callback run on the group's owner.
type Shell struct {
	Request  Request
	cancel   context.CancelFunc
	stream   *sshclient.Conn
	services *ShellServices
}

func NewShell(parent context.Context) *Shell {
	ctx, cancel := context.WithCancel(parent)
	return &Shell{Request: Request{Context: ctx}, cancel: cancel}
}

func (s *Shell) Context() context.Context { return s.Request.Context }
func (s *Shell) Stream() *sshclient.Conn  { return s.stream }
func (s *Shell) Services() *ShellServices { return s.services }

// Close withdraws a pending open or closes only this shell's channel.
func (s *Shell) Close() {
	s.EndServices()
	s.cancel()
	s.Request.Cancel()
	if s.stream != nil {
		s.stream.Close()
	}
}

// EndServices drops this shell's interest in monitoring and forwarding.
func (s *Shell) EndServices() {
	if s.services != nil {
		s.services.release()
		s.services = nil
	}
}

func (s *Shell) Open(connection *sshclient.Connection, opts sshclient.ShellOptions,
	post func(func()), ready func(),
) {
	parent := s.Context()
	ctx, cancel := context.WithCancel(parent)
	setup, endSetup := context.WithTimeout(ctx, 2*time.Minute)
	defer endSetup()
	stop := context.AfterFunc(setup, cancel)
	defer stop()
	var channel *sshclient.Conn
	var err error
	for {
		if err = connection.WaitChannelReady(setup); err != nil {
			break
		}
		channel, err = connection.OpenShell(ctx, opts)
		if !errors.Is(err, sshclient.ErrChannelPending) {
			break
		}
	}
	if err != nil {
		if reason := setup.Err(); reason != nil {
			err = reason
		}
		cancel()
		post(func() {
			s.Request.Complete()
			if parent.Err() == nil {
				s.Request.Failed(err)
			}
		})
		return
	}
	// Attach the reader before waiting for a potentially large initial echo.
	post(func() {
		s.Request.Complete()
		if ctx.Err() != nil || parent.Err() != nil {
			channel.Close()
			return
		}
		s.stream = channel
		s.services = s.Request.transport.holdShell()
		ready()
	})
	select {
	case <-channel.InitializationDone():
	case <-setup.Done():
	}
	stop()
	setupErr := setup.Err()
	endSetup()
	if setupErr != nil {
		cancel()
		if parent.Err() == nil && opts.OnError != nil {
			opts.OnError(setupErr)
		}
	}
	channel.Wait()
	post(func() {
		cancel()
		s.EndServices()
	})
}
