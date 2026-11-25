package profile

import (
	"net/netip"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/sshclient"
)

const DefaultForwardListenHost = "127.0.0.1"

type ForwardRule struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	ListenHost string `json:"listenHost,omitempty"`
	LocalPort  int    `json:"localPort"`
	TargetHost string `json:"targetHost,omitempty"`
	TargetPort int    `json:"targetPort,omitempty"`
	SOCKS      bool   `json:"socks,omitempty"`
}

func (r ForwardRule) Target() (string, error) {
	if r.SOCKS {
		return "", nil
	}
	address, err := sshclient.ForwardTarget(r.TargetHost, r.TargetPort)
	if err != nil {
		return "", locales.NewError(locales.ForwardInvalidTarget)
	}
	return address, nil
}

func (r ForwardRule) EffectiveListenHost() string {
	if r.ListenHost == "" {
		return DefaultForwardListenHost
	}
	return r.ListenHost
}

func ParseForwardListenIP(value string) (netip.Addr, error) {
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

func (r ForwardRule) BindAddress() (netip.AddrPort, error) {
	if r.LocalPort < 1024 || r.LocalPort > 65535 {
		return netip.AddrPort{}, locales.NewError(locales.ForwardInvalidLocalPort)
	}
	ip, err := ParseForwardListenIP(r.EffectiveListenHost())
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ip, uint16(r.LocalPort)), nil
}

func (r ForwardRule) LocalAddress() string {
	bind, _ := r.BindAddress()
	return bind.String()
}

func (r ForwardRule) Kind() string {
	if r.SOCKS {
		return locales.ForwardSOCKS()
	}
	return locales.ForwardLocal()
}

func (r ForwardRule) Title() string {
	if r.Name != "" {
		return r.Name
	}
	return r.Kind()
}
