package desktop

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/zeroy1024/Burrow/internal/locales"
	sshsession "github.com/zeroy1024/Burrow/internal/session"
	"github.com/zeroy1024/Burrow/internal/sshclient"
)

const defaultForwardListenHost = "127.0.0.1"

type sshForwardRule struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	ListenHost string `json:"listenHost,omitempty"`
	LocalPort  int    `json:"localPort"`
	TargetHost string `json:"targetHost,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
	SOCKS      bool   `json:"socks,omitempty"`
}

func (r sshForwardRule) target() (string, error) {
	if r.SOCKS {
		return "", nil
	}
	address, err := sshclient.ForwardTarget(r.TargetHost, r.TargetPort)
	if err != nil {
		return "", locales.NewError(locales.ForwardInvalidTarget)
	}
	return address, nil
}

func (r sshForwardRule) listenHost() string {
	if r.ListenHost == "" {
		return defaultForwardListenHost
	}
	return r.ListenHost
}

func parseForwardListenIP(value string) (netip.Addr, error) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		value = value[1 : len(value)-1]
	}
	ip, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, locales.NewError(locales.ForwardInvalidListenHost)
	}
	return ip.Unmap(), nil
}

func (r sshForwardRule) bindAddress() (netip.AddrPort, error) {
	if r.LocalPort < 1024 || r.LocalPort > 65535 {
		return netip.AddrPort{}, locales.NewError(locales.ForwardInvalidLocalPort)
	}
	ip, err := parseForwardListenIP(r.listenHost())
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ip, uint16(r.LocalPort)), nil
}

func (r sshForwardRule) localAddress() string {
	bind, _ := r.bindAddress()
	return bind.String()
}

func (r sshForwardRule) kind() string {
	if r.SOCKS {
		return locales.ForwardSOCKS()
	}
	return locales.ForwardLocal()
}

func (r sshForwardRule) title() string {
	if r.Name != "" {
		return r.Name
	}
	return r.kind()
}

type forwardRow struct {
	rule        sshForwardRule
	saved       bool
	run         *sshsession.Forward
	startError  locales.Text
	targetError locales.Text
	tested      bool
	testing     bool
}

// Rows and validation messages are presentation state; the listeners belong to the shell scope.
type sshForwardState struct {
	*sshsession.ShellServices
	rows []*forwardRow
}

func (a *app) forwardingHost(endpoint host) int {
	return slices.IndexFunc(a.hosts, func(saved host) bool {
		return saved.session.ID == endpoint.session.ID && saved.protocol == "" &&
			saved.address == endpoint.address && saved.port == endpoint.port && saved.user == endpoint.user
	})
}

func (a *app) savedForwardRows(endpoint host) []*forwardRow {
	index := a.forwardingHost(endpoint)
	if index < 0 || a.hosts[index].session.SSH == nil {
		return nil
	}
	rules := a.hosts[index].session.SSH.Forwards
	rows := make([]*forwardRow, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, &forwardRow{rule: rule, saved: true})
	}
	return rows
}

func (a *app) attachForwards(session *remoteSession, services *sshsession.ShellServices) *sshForwardState {
	group := session.connection
	state := group.forwarding
	if state == nil || state.ShellServices != services {
		state = &sshForwardState{ShellServices: services}
		if !session.temporary {
			state.rows = a.savedForwardRows(session.endpoint)
		}
		group.forwarding = state
	}
	return state
}

func (row *forwardRow) stop() {
	if run := row.run; run != nil {
		row.run = nil
		run.Close()
	}
	row.testing, row.tested = false, false
	row.startError, row.targetError = locales.Text{}, locales.Text{}
}

func (session *remoteSession) forwarding() *sshForwardState {
	if session.ssh != nil && session.state == connected {
		if state := session.ssh.forwarding; state != nil && state.Context().Err() == nil {
			return state
		}
	}
	return nil
}

func (a *app) startForward(state *sshForwardState, row *forwardRow) {
	if state.Context().Err() != nil || row.run != nil {
		return
	}
	row.stop()
	bind, err := row.rule.bindAddress()
	if err != nil {
		row.startError = locales.ErrorText(err)
		return
	}
	target, err := row.rule.target()
	if err != nil {
		row.startError = locales.ErrorText(err)
		return
	}
	var run *sshsession.Forward
	run, err = state.Listen(bind, target, func(err error) {
		a.window.Update(func() {
			if row.run != nil && row.run == run && run.Context().Err() == nil {
				row.tested = false
				row.targetError = locales.ErrorText(locales.Errorf(locales.ForwardTargetFailed, err))
			}
		})
	})
	if err != nil {
		row.startError = locales.ErrorText(locales.Errorf(locales.ForwardListenFailed, err))
		return
	}
	row.run = run
	a.sshWorkers.Go(func() {
		err := run.Wait()
		a.window.Update(func() {
			if row.run == run {
				row.stop()
				if err != nil {
					row.startError = locales.ErrorText(locales.Errorf(locales.ForwardListenFailed, err))
				}
			}
		})
	})
}

func (a *app) testForward(state *sshForwardState, row *forwardRow, target string) {
	run := row.run
	if run == nil || run.Context().Err() != nil || row.testing {
		return
	}
	ctx, cancel := context.WithTimeout(run.Context(), 5*time.Second)
	row.testing, row.tested, row.targetError = true, false, locales.Text{}
	a.sshWorkers.Go(func() {
		defer cancel()
		remote, err := state.Connection().DialContext(ctx, "tcp", target)
		if err == nil {
			remote.Close()
		}
		a.window.Update(func() {
			if row.run == run {
				row.testing, row.tested = false, err == nil
				row.targetError = locales.Text{}
				if err != nil {
					row.targetError = locales.ErrorText(locales.Errorf(locales.ForwardTargetFailed, err))
				}
			}
		})
	})
}

func (a *app) persistForward(endpoint host, before, after *sshForwardRule, done func(error)) {
	index := a.forwardingHost(endpoint)
	if index < 0 || a.catalogSaving {
		done(locales.NewError(locales.ForwardSavedHostChanged))
		return
	}
	profile := a.hosts[index]
	initializeSession(&profile)
	options := *profile.session.SSH
	options.Forwards = slices.Clone(options.Forwards)
	if before != nil {
		position := slices.IndexFunc(options.Forwards, func(rule sshForwardRule) bool { return rule.ID == before.ID })
		if position < 0 || options.Forwards[position] != *before {
			done(locales.NewError(locales.ForwardSavedRuleChanged))
			return
		}
		options.Forwards = slices.Delete(options.Forwards, position, position+1)
		if after != nil {
			options.Forwards = slices.Insert(options.Forwards, position, *after)
		}
	} else if after != nil {
		options.Forwards = append(options.Forwards, *after)
	}
	a.commitCatalogMoveAsync(func() { a.hosts[index].session.SSH = &options }, done)
}

func validateForwardRules(rules []sshForwardRule) error {
	ids := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.ID == "" || ids[rule.ID] {
			return errors.New("invalid or duplicate forwarding rule id")
		}
		ids[rule.ID] = true
		if _, err := rule.bindAddress(); err != nil {
			return err
		}
		if _, err := rule.target(); err != nil {
			return err
		}
	}
	return nil
}
