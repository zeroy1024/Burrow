package desktop

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/locales"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
	"sshdesk/internal/vault"
	"sshdesk/internal/wggateway"
)

type wireguardDraft struct {
	addresses, allowed, dns    string
	mtu, keepalive, listenPort string
	private, psk               string
	removePSK, importing       bool
}

func newWireguardDraft(config wggateway.Config) wireguardDraft {
	return wireguardDraft{
		addresses: strings.Join(config.Addresses, ", "), allowed: strings.Join(config.AllowedIPs, ", "),
		dns: strings.Join(config.DNS, ", "), mtu: strconv.Itoa(config.MTU),
		keepalive: strconv.Itoa(config.Keepalive), listenPort: strconv.Itoa(config.ListenPort),
	}
}

func (d *gatewayDraft) wireguardClientPublicKey() string {
	if d.wg.private == "" {
		if d.value.WG.CredentialRevision != "" {
			return d.value.WG.ClientPublicKey
		}
		return ""
	}
	// Incomplete input has no preview; saving reports its validation error.
	public, err := wggateway.PublicKey(strings.TrimSpace(d.wg.private))
	if err != nil {
		return ""
	}
	return public
}

func (a *app) wireguardDraftFields(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	if d.value.WG == nil {
		d.value.WG = &wggateway.Config{MTU: 1420}
		d.wg = newWireguardDraft(*d.value.WG)
	}
	uikit.FormRow(c, "WireGuard", uikit.FormRowProps{Key: "wg-import", Help: locales.WgUserSpace()}, func() ui.Element {
		return uikit.Button(c, locales.WgImport(), uikit.ButtonProps{Disabled: d.wg.importing}).OnClick(func() { a.importWireguard(d) })
	})
	field := func(key, label string, value *string, placeholder, help string) {
		uikit.FormRow(c, label, uikit.FormRowProps{Key: key, Help: help}, func() ui.Element {
			return uikit.Input(c, value, uikit.InputProps{Label: label, Placeholder: placeholder, Mono: true})
		})
	}
	field("wg-endpoint", locales.WgEndpoint(), &d.value.WG.Endpoint, "vpn.example.com:51820", locales.WgFirstHop())
	field("wg-addresses", locales.WgAddresses(), &d.wg.addresses, "10.0.0.2/32", locales.WgAddressHelp())
	field("wg-peer-key", locales.WgPeerKey(), &d.value.WG.PublicKey, "", "")
	keyHelp := locales.WgSecretHelp()
	if d.value.WG.CredentialRevision != "" {
		keyHelp = locales.WgKeepKeys()
	}
	uikit.FormRow(c, locales.WgPrivateKey(), uikit.FormRowProps{Key: "wg-private", Help: keyHelp}, func() ui.Element {
		input := uikit.PasswordInput(c, &d.wg.private, uikit.InputProps{Label: locales.WgPrivateKey(), Mono: true})
		return input.Root
	})
	uikit.FormRow(c, locales.WgClientKey(), uikit.FormRowProps{Key: "wg-client-key"}, func() ui.Element {
		return ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			uikit.Text(c, d.wireguardClientPublicKey(), uikit.TextProps{Mono: true}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Selectable()
			uikit.Button(c, locales.WgGenerate(), uikit.ButtonProps{Size: uikit.SizeSm}).OnClick(func() {
				key, err := wggateway.GenerateKey()
				if err != nil {
					d.errorText = locales.ErrorText(err)
					return
				}
				d.wg.private = key
			})
		})
	})
	uikit.FormRow(c, locales.WgPSK(), uikit.FormRowProps{Key: "wg-psk", Help: locales.WgPSKHelp()}, func() ui.Element {
		return ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
			input := uikit.PasswordInput(c, &d.wg.psk, uikit.InputProps{Label: locales.WgPSK(), Mono: true})
			input.Root.Grow(1).MinWidth(0)
			if input.Input.Changed() && d.wg.psk != "" {
				d.wg.removePSK = false
			}
			uikit.Button(c, locales.WgRemovePSK(), uikit.ButtonProps{Size: uikit.SizeSm, Disabled: d.wg.removePSK}).OnClick(func() {
				d.wg.removePSK, d.wg.psk = true, ""
			})
		})
	})
	field("wg-allowed", locales.WgAllowedIPs(), &d.wg.allowed, "10.0.0.0/24", "")
	field("wg-dns", locales.WgDNS(), &d.wg.dns, "10.0.0.1", locales.WgDNSHelp())
	field("wg-mtu", locales.WgMTU(), &d.wg.mtu, "1420", "")
	field("wg-keepalive", locales.WgKeepalive(), &d.wg.keepalive, "0", "")
	field("wg-listen", locales.WgListenPort(), &d.wg.listenPort, "0", locales.WgListenHelp())
}

func (a *app) importWireguard(d *gatewayDraft) {
	if d.wg.importing {
		return
	}
	d.wg.importing = true
	window, title := a.window, locales.WgChooseConfig()
	a.sshWorkers.Go(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: title})
		var config *wggateway.Config
		var credential wggateway.Credential
		name := ""
		if err == nil && len(paths) > 0 {
			name = strings.TrimSuffix(filepath.Base(paths[0]), filepath.Ext(paths[0]))
			var file *os.File
			file, err = os.Open(paths[0])
			if err == nil {
				var data []byte
				data, err = io.ReadAll(io.LimitReader(file, wggateway.MaxConfigSize+1))
				closeErr := file.Close()
				if err == nil {
					err = closeErr
				}
				if err == nil {
					var imported wggateway.Config
					imported, credential, err = wggateway.Parse(data)
					if err == nil {
						config = &imported
					}
				}
				clear(data)
			}
		}
		window.Update(func() {
			defer func() { credential.PrivateKey, credential.PresharedKey = "", "" }()
			if a.gatewayUI.draft != d || !a.gatewayUI.open {
				return
			}
			d.wg.importing = false
			if d.value.Kind != "wireguard" {
				return
			}
			if err != nil {
				d.errorText = locales.ErrorText(err)
				return
			}
			if config != nil {
				d.value.WG, d.wg = config, newWireguardDraft(*config)
				d.wg.private, d.wg.psk, d.wg.removePSK = credential.PrivateKey, credential.PresharedKey, credential.PresharedKey == ""
				d.errorText = locales.Text{}
				if strings.TrimSpace(d.value.Name) == "" {
					d.value.Name = name
				}
			}
		})
	})
}

func (a *app) prepareWireguardSave(d *gatewayDraft) (gatewayConfig, []vault.PlainItem, error) {
	g := d.value
	g.SOCKS, g.SSHProfileID, g.UpstreamID = nil, "", ""
	config := cloneWGConfig(*g.WG)
	config.Endpoint, config.PublicKey = strings.TrimSpace(config.Endpoint), strings.TrimSpace(config.PublicKey)
	config.Addresses, config.AllowedIPs, config.DNS = wggateway.SplitList(d.wg.addresses), wggateway.SplitList(d.wg.allowed), wggateway.SplitList(d.wg.dns)
	var err error
	config.MTU, err = strconv.Atoi(strings.TrimSpace(d.wg.mtu))
	if err != nil {
		return g, nil, locales.NewError(locales.WgInvalidMTU)
	}
	config.Keepalive, err = strconv.Atoi(strings.TrimSpace(d.wg.keepalive))
	if err != nil {
		return g, nil, locales.NewError(locales.WgInvalidKeepalive)
	}
	config.ListenPort, err = strconv.Atoi(strings.TrimSpace(d.wg.listenPort))
	if err != nil {
		return g, nil, locales.NewError(locales.WgInvalidListenPort)
	}
	var items []vault.PlainItem
	if config.CredentialRevision == "" || d.wg.private != "" || d.wg.psk != "" || d.wg.removePSK {
		var credential wggateway.Credential
		if config.CredentialRevision != "" {
			credential, err = a.wgGatewayCredential(a.ctx, gatewayHop{ID: g.ID, WG: &config})
			if err != nil {
				return g, nil, err
			}
		}
		if d.wg.private != "" {
			credential.PrivateKey = strings.TrimSpace(d.wg.private)
		}
		if credential.PrivateKey == "" {
			return g, nil, locales.NewError(locales.WgMissingPrivateKey)
		}
		if d.wg.removePSK {
			credential.PresharedKey = ""
		}
		if d.wg.psk != "" {
			credential.PresharedKey = strings.TrimSpace(d.wg.psk)
		}
		config.ClientPublicKey, err = wggateway.PublicKey(credential.PrivateKey)
		if err != nil {
			return g, nil, err
		}
		if credential.PresharedKey != "" {
			key, err := wggateway.DecodeKey(credential.PresharedKey)
			if err != nil {
				return g, nil, err
			}
			clear(key)
		}
		config.CredentialRevision = newSessionID()
		credential.Revision = config.CredentialRevision
		data, err := json.Marshal(credential)
		if err != nil {
			return g, nil, err
		}
		items = append(items, vault.PlainItem{ID: wgCredentialKey(g.ID), Kind: "gateway_password", Revision: credential.Revision, Data: data})
	}
	g.WG = &config
	return g, items, nil
}
