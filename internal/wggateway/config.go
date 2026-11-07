// Package wggateway provides application-local WireGuard networking. It uses
// an in-memory IP stack and ordinary UDP sockets, never an OS TUN interface.
package wggateway

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
)

type Config struct {
	Endpoint           string   `json:"endpoint"`
	PublicKey          string   `json:"publicKey"`
	ClientPublicKey    string   `json:"clientPublicKey"`
	Addresses          []string `json:"addresses"`
	AllowedIPs         []string `json:"allowedIPs"`
	DNS                []string `json:"dns,omitempty"`
	MTU                int      `json:"mtu"`
	Keepalive          int      `json:"keepalive,omitempty"`
	ListenPort         int      `json:"listenPort,omitempty"`
	CredentialRevision string   `json:"credentialRevision"`
}

type Credential struct {
	Revision     string `json:"revision"`
	PrivateKey   string `json:"privateKey"`
	PresharedKey string `json:"presharedKey,omitempty"`
}

func DecodeKey(value string) ([]byte, error) {
	if len(value) != 44 {
		return nil, locales.NewError(locales.WgInvalidKey)
	}
	key, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, locales.NewError(locales.WgInvalidKey)
	}
	var nonzero byte
	for _, b := range key {
		nonzero |= b
	}
	if nonzero == 0 {
		clear(key)
		return nil, locales.NewError(locales.WgInvalidKey)
	}
	return key, nil
}

func PublicKey(private string) (string, error) {
	key, err := DecodeKey(private)
	if err != nil {
		return "", err
	}
	defer clear(key)
	secret, err := ecdh.X25519().NewPrivateKey(key)
	if err != nil {
		return "", locales.NewError(locales.WgInvalidKey)
	}
	return base64.StdEncoding.EncodeToString(secret.PublicKey().Bytes()), nil
}

func GenerateKey() (string, error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	data := key.Bytes()
	defer clear(data)
	return base64.StdEncoding.EncodeToString(data), nil
}

func (c Config) Validate() error {
	for _, value := range []string{c.PublicKey, c.ClientPublicKey} {
		key, err := DecodeKey(value)
		if err != nil {
			return err
		}
		clear(key)
	}
	if c.PublicKey == c.ClientPublicKey {
		return locales.NewError(locales.WgSameKeys)
	}
	host, port, err := net.SplitHostPort(c.Endpoint)
	number, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || strings.TrimSpace(host) == "" || strings.ContainsAny(host, "\r\n\t /\\") || number < 1 || number > 65535 {
		return locales.NewError(locales.WgInvalidEndpoint)
	}
	if len(c.Addresses) == 0 {
		return locales.NewError(locales.WgInvalidAddress)
	}
	seen := map[netip.Addr]bool{}
	for _, value := range c.Addresses {
		address, err := netip.ParsePrefix(value)
		if err != nil || address.Addr().IsUnspecified() || address.Addr().IsMulticast() || seen[address.Addr()] {
			return locales.NewError(locales.WgInvalidAddress)
		}
		seen[address.Addr()] = true
	}
	if len(c.AllowedIPs) == 0 {
		return locales.NewError(locales.WgInvalidAllowedIPs)
	}
	for _, value := range c.AllowedIPs {
		if _, err := netip.ParsePrefix(value); err != nil {
			return locales.NewError(locales.WgInvalidAllowedIPs)
		}
	}
	for _, value := range c.DNS {
		address, err := netip.ParseAddr(value)
		if err != nil || address.IsUnspecified() || address.IsMulticast() || address.Zone() != "" {
			return locales.NewError(locales.WgInvalidDNS)
		}
		if !c.Allows(address) {
			return locales.NewError(locales.WgDNSOutsideAllowedIPs)
		}
	}
	if c.MTU < 1280 || c.MTU > 65535 {
		return locales.NewError(locales.WgInvalidMTU)
	}
	if c.Keepalive < 0 || c.Keepalive > 65535 {
		return locales.NewError(locales.WgInvalidKeepalive)
	}
	if c.ListenPort < 0 || c.ListenPort > 65535 {
		return locales.NewError(locales.WgInvalidListenPort)
	}
	return nil
}

func (c Config) Allows(address netip.Addr) bool {
	for _, value := range c.AllowedIPs {
		prefix, err := netip.ParsePrefix(value)
		if err == nil && prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (c Config) Identity() string { return c.ClientPublicKey + "/" + c.PublicKey }

func SplitList(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
}
