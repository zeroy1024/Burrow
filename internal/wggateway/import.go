package wggateway

import (
	"bufio"
	"strconv"
	"strings"

	"sshdesk/internal/locales"
)

const MaxConfigSize = 1 << 20

// Parse accepts a single-peer client configuration. OS integration settings
// and executable hooks are rejected rather than passed to wg-quick or a shell.
func Parse(data []byte) (Config, Credential, error) {
	c := Config{MTU: 1420}
	var credential Credential
	if len(data) > MaxConfigSize {
		return c, credential, locales.NewError(locales.WgConfigTooLarge)
	}
	section, peers, interfaces, line := "", 0, 0, 0
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), MaxConfigSize)
	for scanner.Scan() {
		line++
		text, _, _ := strings.Cut(scanner.Text(), "#")
		text = strings.TrimSpace(text)
		if text == "" || strings.HasPrefix(text, ";") {
			continue
		}
		if strings.HasPrefix(text, "[") {
			switch text {
			case "[Interface]":
				interfaces++
				section = "interface"
			case "[Peer]":
				peers++
				section = "peer"
			default:
				return c, Credential{}, locales.Errorf(locales.WgConfigLine, line)
			}
			if interfaces > 1 || peers > 1 {
				return c, Credential{}, locales.NewError(locales.WgSinglePeer)
			}
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || section == "" {
			return c, Credential{}, locales.Errorf(locales.WgConfigLine, line)
		}
		field := section + "." + key
		if seen[field] && key != "Address" && key != "DNS" && key != "AllowedIPs" {
			return c, Credential{}, locales.Errorf(locales.WgConfigLine, line)
		}
		seen[field] = true
		var err error
		switch field {
		case "interface.PrivateKey":
			credential.PrivateKey = value
		case "interface.Address":
			c.Addresses = append(c.Addresses, SplitList(value)...)
		case "interface.DNS":
			c.DNS = append(c.DNS, SplitList(value)...)
		case "interface.MTU":
			c.MTU, err = strconv.Atoi(value)
		case "interface.ListenPort":
			c.ListenPort, err = strconv.Atoi(value)
		case "peer.PublicKey":
			c.PublicKey = value
		case "peer.Endpoint":
			c.Endpoint = value
		case "peer.AllowedIPs":
			c.AllowedIPs = append(c.AllowedIPs, SplitList(value)...)
		case "peer.PersistentKeepalive":
			c.Keepalive, err = strconv.Atoi(value)
		case "peer.PresharedKey":
			credential.PresharedKey = value
		default:
			return c, Credential{}, locales.Errorf(locales.WgUnsupportedSetting, line)
		}
		if err != nil {
			return c, Credential{}, locales.Errorf(locales.WgConfigLine, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return c, Credential{}, locales.NewError(locales.WgConfigTooLarge)
	}
	if interfaces != 1 || peers != 1 {
		return c, Credential{}, locales.NewError(locales.WgSinglePeer)
	}
	var err error
	c.ClientPublicKey, err = PublicKey(credential.PrivateKey)
	if err != nil {
		return c, Credential{}, err
	}
	if credential.PresharedKey != "" {
		key, err := DecodeKey(credential.PresharedKey)
		if err != nil {
			return c, Credential{}, err
		}
		clear(key)
	}
	if err := c.Validate(); err != nil {
		return c, Credential{}, err
	}
	return c, credential, nil
}
