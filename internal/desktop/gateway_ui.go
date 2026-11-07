package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
	"github.com/zeroy1024/Burrow/internal/vault"
	"github.com/zeroy1024/Burrow/internal/wggateway"
)

var gatewayKinds = []string{"ssh", "socks5", "wireguard"}

type gatewayDraft struct {
	initial                        [19]string
	wg                             wireguardDraft
	editing                        bool
	value                          gatewayConfig
	port, password                 string
	errorText                      locales.Text
	auth                           int // 0 无认证，1 用户名密码（SOCKS5）
	passwordChanged, clearPassword bool
	deleting                       bool
	scroll                         ui.ScrollState
	modal                          uikit.ModalController
}

func (d *gatewayDraft) inputValues() [19]string {
	values := [19]string{d.value.Name, d.value.Kind, "", d.value.SSHProfileID, "", "", d.port, strconv.Itoa(d.auth)}
	if d.value.Kind == "socks5" {
		values[2] = d.value.UpstreamID
	}
	if d.value.SOCKS != nil {
		values[4] = d.value.SOCKS.Address
		if d.auth == 1 {
			values[5] = d.value.SOCKS.Username
		}
	}
	if d.value.WG != nil {
		values[8], values[9] = d.value.WG.Endpoint, d.value.WG.PublicKey
		values[10], values[11], values[12] = d.wg.addresses, d.wg.allowed, d.wg.dns
		values[13], values[14], values[15] = d.wg.mtu, d.wg.keepalive, d.wg.listenPort
		values[16], values[17], values[18] = d.wg.private, d.wg.psk, strconv.FormatBool(d.wg.removePSK)
	}
	return values
}

// Test results live only for this run of the app; saving any gateway drops
// them, since downstream routes include it.
type gatewayUIState struct {
	selected     string
	collapsed    map[string]bool
	listScroll   ui.ScrollState
	scroll       ui.ScrollState
	open         bool
	draft        *gatewayDraft
	testGateway  string
	testCancel   context.CancelFunc
	testResults  <-chan connectionTestResult
	testProgress *gatewayTestProgress
	results      map[string]gatewayTestRecord
}

// gatewayTestRecord keeps how far a finished test got along the route it
// captured: hops before proven connected, and failed is the hop that did not
// (-1 when none was reached, as when the route could not be captured).
type gatewayTestRecord struct {
	result         connectionTestResult
	route          []gatewayHop
	proven, failed int
}

// gatewayTestView places a test on the route it captured, which can differ
// from the route shown once a host's gateway or the default changes.
type gatewayTestView struct {
	route                      []gatewayHop
	proven, connecting, failed int
}

func (v gatewayTestView) link(id string) (link, mark gatewayLinkState) {
	index := slices.IndexFunc(v.route, func(hop gatewayHop) bool { return hop.ID == id })
	switch {
	case index < 0:
	case index < v.proven:
		return gatewayLinkOK, gatewayLinkOK
	case index <= v.connecting:
		// Hops before a connecting SSH hop are in play until it connects.
		if index == v.connecting {
			return gatewayLinkFlow, gatewayLinkFlow
		}
		return gatewayLinkFlow, gatewayLinkIdle
	case index == v.failed:
		return gatewayLinkFail, gatewayLinkFail
	}
	return gatewayLinkIdle, gatewayLinkIdle
}

type gatewayRefKind uint8

const (
	gatewayRefDefault gatewayRefKind = iota
	gatewayRefHost
	gatewayRefGateway
	gatewayRefRuntime
)

func gatewayRefGroups() []string {
	return []string{locales.MessageAppSettings(), locales.MessageHostsAndSites(), locales.MessageDownstreamGateways(), locales.MessageRunning()}
}

type gatewayRef struct {
	kind          gatewayRefKind
	label, detail string
	host          hostID
	gateway       string
}

// hostGatewayOptions disables the gateways whose route would come back to
// the edited host, such as one built on the host itself.
func (a *app) hostGatewayOptions(d *hostDraft) []uikit.Option[string] {
	options := []uikit.Option[string]{{Value: "", Label: locales.MessageUseAppDefault()}, {Value: directGateway, Label: locales.MessageDirect()}}
	catalog := a.gatewayCatalog()
	for _, g := range a.gateways {
		option := uikit.Option[string]{Value: g.ID, Label: g.Name, Detail: gatewayKindLabel(g.Kind)}
		if d.editing {
			var loop *gatewayLoop
			if _, err := catalog.hostRoute(a.hosts[d.id].session.ID, g.ID, nil); errors.As(err, &loop) {
				option.Detail, option.Disabled = locales.MessageRoutesThroughThisHost(), true
			}
		}
		options = append(options, option)
	}
	return options
}

func (a *app) gatewayName(id string) string {
	if id == "" || id == directGateway {
		return locales.MessageDirect()
	}
	if g, ok := a.gatewayByID(id); ok {
		return g.Name
	}
	return locales.MessageTheGatewayNoLongerExists()
}

func (a *app) gatewayByID(id string) (gatewayConfig, bool) {
	index := slices.IndexFunc(a.gateways, func(g gatewayConfig) bool { return g.ID == id })
	if index < 0 {
		return gatewayConfig{}, false
	}
	return a.gateways[index], true
}

// gatewayChain lists the gateway and the gateways before it, first hop first.
// The saved catalog is validated, so its gateways always resolve.
func (a *app) gatewayChain(id string) []gatewayConfig {
	if id == "" {
		return nil
	}
	chain, _ := a.gatewayCatalog().chain(id)
	return chain
}

func (r gatewayCatalog) chain(id string) ([]gatewayConfig, error) {
	ids, err := r.gatewayRoute(id, nil)
	chain := make([]gatewayConfig, 0, len(ids))
	for _, hop := range ids {
		g, _ := r.gateway(hop)
		chain = append(chain, g)
	}
	return chain, err
}

func (a *app) gatewayHost(g gatewayConfig) (host, hostID, bool) {
	for index, h := range a.hosts {
		if h.session.ID == g.SSHProfileID && h.protocol == "" {
			return h, hostID(index), true
		}
	}
	return host{}, 0, false
}

// gatewayEndpoint is the gateway's own host and port; gatewayAddress adds the
// SSH user.
func (a *app) gatewayEndpoint(g gatewayConfig) string {
	if g.Kind == "wireguard" {
		return g.WG.Endpoint
	}
	if g.Kind == "ssh" {
		if h, _, ok := a.gatewayHost(g); ok {
			return h.address + ":" + strconv.Itoa(h.port)
		}
		return ""
	}
	return g.SOCKS.Address + ":" + strconv.Itoa(g.SOCKS.Port)
}

func (a *app) gatewayAddress(g gatewayConfig) string {
	if g.Kind == "ssh" {
		if h, _, ok := a.gatewayHost(g); ok {
			return h.user + "@" + h.address + ":" + strconv.Itoa(h.port)
		}
	}
	return a.gatewayEndpoint(g)
}

func gatewayKindLabel(kind string) string {
	if kind == "wireguard" {
		return "WireGuard"
	}
	if kind == "ssh" {
		return "SSH"
	}
	return "SOCKS5"
}

func gatewayKindLook(kind string) (*ui.SVG, color.NRGBA) {
	choices := theme.Current().Color.AccentChoices
	if kind == "wireguard" {
		return routeIcon, choices.Purple
	}
	if kind == "ssh" {
		return terminalIcon, choices.Blue
	}
	return globeIcon, choices.Purple
}

// gatewayTile tints a kind's glyph like an OS logo tile; a zero brand keeps
// the neutral control fill for the local and target ends of a route.
func gatewayTile(c *ui.Context, icon *ui.SVG, brand color.NRGBA, box, glyph, radius float32) ui.Element {
	t := theme.Current()
	background, ink := t.Color.Surface.Control, t.Color.Icon.Default
	if brand.A != 0 {
		background, ink = t.OSLogoBg(brand), uikit.OSLogoInk(brand)
	}
	return ui.Box(c).Size(box, box).Shrink(0).Radius(radius).Background(uikit.Color(background)).Center().PassThrough().Children(func() {
		ui.Icon(c, icon).Size(glyph, glyph).TextColor(uikit.Color(ink)).PassThrough()
	})
}

func gatewayKindTile(c *ui.Context, kind string, box, glyph, radius float32) ui.Element {
	icon, brand := gatewayKindLook(kind)
	return gatewayTile(c, icon, brand, box, glyph, radius)
}

func gatewayDashes(c *ui.Context, width float32) ui.Element {
	t := theme.Current()
	return ui.Box(c).Width(width).Height(t.Space.N8).Shrink(0).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
		var line ui.Path
		line.MoveTo(r.X, r.Y+r.H/2).LineTo(r.X+r.W, r.Y+r.H/2).Dash(0, t.Space.N4, t.Space.N4)
		p.StrokePath(&line, t.Size.FocusRing, uikit.Color(t.Color.Border.ControlHover))
	})
}

func hostKindLabel(h host) string {
	if h.protocol == "" {
		return locales.MessageSshHost()
	}
	return strings.ToUpper(h.protocol) + locales.MessageSite()
}

func (a *app) gatewayFollowers() int {
	count := 0
	for _, h := range a.hosts {
		if h.session.Gateway == "" {
			count++
		}
	}
	return count
}

func (a *app) gatewayReferences(id string) []gatewayRef {
	refs := []gatewayRef{}
	if a.defaultGateway == id {
		refs = append(refs, gatewayRef{kind: gatewayRefDefault, label: locales.MessageDefaultAppGateway(), detail: locales.GatewayDefaultFollowers(
			a.gatewayFollowers(),
		)})
	}
	for index, h := range a.hosts {
		if h.session.Gateway == id {
			refs = append(refs, gatewayRef{kind: gatewayRefHost, label: h.name, detail: hostKindLabel(h), host: hostID(index)})
		}
	}
	for _, g := range a.gateways {
		if g.UpstreamID == id {
			refs = append(refs, gatewayRef{kind: gatewayRefGateway, label: g.Name, detail: locales.MessageUsesThisGatewayUpstream(), gateway: g.ID})
		}
	}
	return append(refs, a.gatewayRuntimeReferences(id)...)
}

func (a *app) gatewayRuntimeReferences(id string) []gatewayRef {
	refs := []gatewayRef{}
	runtime := func(label string) {
		refs = append(refs, gatewayRef{kind: gatewayRefRuntime, label: label, detail: locales.MessageReleasedWhenClosed()})
	}
	if a.gatewayPool.inUse(id) {
		runtime(locales.MessageActiveGatewayConnection())
	}
	for _, group := range a.runtimeSSHGroups() {
		if slices.ContainsFunc(group.plan.route, func(h gatewayHop) bool { return h.ID == id }) {
			runtime(locales.MessageTerminalTransferOrRecoverableTab())
			break
		}
	}
	files := slices.Clone(a.fileSessions)
	for _, closed := range a.closedTabs {
		if closed.section == sectionFiles {
			files = append(files, closed.file)
		}
	}
	for _, task := range a.transfers.tasks {
		files = append(files, task.session)
	}
	if a.mcpBackend != nil {
		for _, session := range a.mcpBackend.sessions {
			if session.connection != nil && (session.connection.connection != nil || session.connection.connecting) {
				files = append(files, session)
			}
		}
	}
	for _, session := range files {
		route := session.endpoint.session.GatewayRoute
		if route != nil && slices.ContainsFunc(*route, func(h gatewayHop) bool { return h.ID == id }) {
			runtime(locales.MessageFileSessionOrTransfer())
			break
		}
	}
	return refs
}

func (a *app) gatewayLive(id string) bool { return len(a.gatewayRuntimeReferences(id)) > 0 }

func (a *app) setDefaultGateway(id string) {
	a.commitCatalogMoveAsync(
		func() { a.defaultGateway = id },
		func(err error) { a.persistenceNotice(locales.MessageUnableToSaveDefaultGateway, err) },
	)
}

func (a *app) selectGateway(id string) {
	if a.gatewayUI.selected != id {
		a.gatewayUI.selected, a.gatewayUI.scroll = id, ui.ScrollState{}
	}
}

// openGatewayReference edits a referencing host on its own page, where its
// gateway selection lives.
func (a *app) openGatewayReference(ref gatewayRef) {
	switch ref.kind {
	case gatewayRefGateway:
		a.selectGateway(ref.gateway)
	case gatewayRefHost:
		target := sectionSSH
		if a.hosts[ref.host].protocol != "" {
			target = sectionFiles
		}
		a.switchSection(target)
		a.startEditHostDraft(ref.host)
	}
}

func (a *app) startGatewayDraft(g gatewayConfig, deleting bool) {
	if g.ID == "" {
		kind := g.Kind
		if kind == "" {
			kind = "ssh"
		}
		g = gatewayConfig{ID: newSessionID(), Kind: kind}
	}
	port, auth := "1080", 0
	if g.SOCKS != nil {
		copy := *g.SOCKS
		g.SOCKS = &copy
		port = strconv.Itoa(copy.Port)
		if copy.Username != "" {
			auth = 1
		}
	}
	a.gatewayUI.draft = &gatewayDraft{value: g, port: port, auth: auth, deleting: deleting}
	if g.WG != nil {
		config := cloneWGConfig(*g.WG)
		a.gatewayUI.draft.value.WG = &config
		a.gatewayUI.draft.wg = newWireguardDraft(config)
	} else if g.Kind == "wireguard" {
		a.gatewayUI.draft.value.WG = &wggateway.Config{MTU: 1420}
		a.gatewayUI.draft.wg = newWireguardDraft(*a.gatewayUI.draft.value.WG)
	}
	a.gatewayUI.draft.initial = a.gatewayUI.draft.inputValues()
	a.gatewayUI.draft.editing = slices.ContainsFunc(a.gateways, func(value gatewayConfig) bool { return value.ID == g.ID })
	a.gatewayUI.open = true
}

func (a *app) gatewayMenu(g gatewayConfig) []uikit.MenuEntry {
	return []uikit.MenuEntry{
		{ID: "test", Label: locales.MessageTestConnection(), Icon: uikit.IconPlay, Disabled: a.gatewayTestRunning(g.ID), OnChoose: func() { a.startGatewayTest(g.ID) }},
		{ID: "edit", Label: locales.MessageEditGateway(), Icon: uikit.IconEdit, OnChoose: func() { a.startGatewayDraft(g, false) }},
		{ID: "default", Label: locales.MessageSetAsAppDefaultGateway(), Icon: uikit.IconRoute, Disabled: a.defaultGateway == g.ID, OnChoose: func() { a.setDefaultGateway(g.ID) }},
		{ID: "before-delete", Separator: true},
		{ID: "delete", Label: locales.MessageDeleteGateway(), Icon: uikit.IconTrash, Danger: true, OnChoose: func() { a.startGatewayDraft(g, true) }},
	}
}

// ---------- 页面 ----------

func (a *app) gatewayPage(c *ui.Context) {
	t := theme.Current()
	a.collectGatewayTest(c)
	if _, ok := a.gatewayByID(a.gatewayUI.selected); !ok && len(a.gateways) > 0 {
		a.selectGateway(a.gateways[0].ID)
	}
	ui.Column(c).Key("gateway-page").Fill().Gap(t.C.Window.Gutter).Children(func() {
		a.gatewayDefaultBar(c)
		if len(a.gateways) == 0 {
			a.gatewayEmptyState(c)
			return
		}
		ui.Row(c).Key("gateway-workspace").Grow(1).MinHeight(0).FillWidth().AlignItems(ui.Stretch).Gap(t.C.Window.Gutter).Children(func() {
			uikit.Card(c).Key("gateway-list").Width(a.page().sidebarWidth).Shrink(0).Children(func() { a.gatewayList(c) })
			uikit.Card(c).Key("gateway-detail").Grow(1).MinWidth(0).Children(func() { a.gatewayDetail(c) })
		})
	})
}

func (a *app) gatewayDefaultBar(c *ui.Context) {
	t := theme.Current()
	uikit.Card(c).Key("gateway-default-bar").FillWidth().Shrink(0).Padding(t.Space.N10, t.Space.N16).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N12).Children(func() {
			uikit.Icon(c, routeIcon, uikit.IconProps{Tone: uikit.ToneSecondary})
			uikit.Text(c, locales.MessageDefaultAppGateway(), uikit.TextProps{Style: uikit.TextBodyStrong}).Shrink(0)
			selected := a.defaultGateway
			glyph := t.C.IconButton.IconSizeXs
			options := []uikit.Option[string]{{Value: "", Label: locales.MessageDirect(), Detail: locales.MessageNoGateway(), Leading: func() {
				gatewayTile(c, uikit.IconSVG(uikit.IconLaptop), color.NRGBA{}, t.Space.N18, glyph, t.Radius.Chip)
			}}}
			for _, g := range a.gateways {
				options = append(options, uikit.Option[string]{Value: g.ID, Label: g.Name, Detail: gatewayKindLabel(g.Kind), Leading: func() {
					gatewayKindTile(c, g.Kind, t.Space.N18, glyph, t.Radius.Chip)
				}})
			}
			input := uikit.Select(
				c,
				&selected,
				options,
				uikit.SelectProps{Label: locales.MessageDefaultAppGateway(), Disabled: len(a.gateways) == 0},
			).
				Width(t.Size.Sider).Shrink(0)
			if input.Changed() {
				a.setDefaultGateway(selected)
			}
			ui.Row(c).MinWidth(0).Shrink(1).Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
				chain := a.gatewayChain(a.defaultGateway)
				if len(chain) == 0 {
					uikit.Text(
						c,
						locales.MessageConnectDirectlyFromThisComputer(),
						uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
					).SingleLine()
					return
				}
				uikit.Text(
					c,
					locales.MessageThisComputer(),
					uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
				).Shrink(0)
				for _, hop := range chain {
					uikit.Icon(c, uikit.IconArrowRight, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary})
					uikit.Text(c, hop.Name, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary, Mono: true}).
						SingleLine().Ellipsis("…").MinWidth(0)
				}
			})
			ui.Spacer(c)
			live := 0
			for _, g := range a.gateways {
				if a.gatewayLive(g.ID) {
					live++
				}
			}
			stat := func(label, value string, tone uikit.Tone) {
				ui.Row(c).Key(label).Gap(t.Space.N4).AlignItems(ui.Center).Shrink(0).Children(func() {
					uikit.Text(c, label, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
					uikit.Text(c, value, uikit.TextProps{Style: uikit.TextBodyStrong, Tone: tone}).FontFeatures("tnum")
				})
			}
			stat(locales.MessageUseDefault(), strconv.Itoa(a.gatewayFollowers()), uikit.ToneNeutral)
			stat(locales.GatewaysTitle(), strconv.Itoa(len(a.gateways)), uikit.ToneNeutral)
			liveTone := uikit.ToneNeutral
			if live > 0 {
				liveTone = uikit.ToneSuccess
			}
			stat(locales.MessageInUse(), strconv.Itoa(live), liveTone)
		})
	})
}

func (a *app) gatewayEmptyState(c *ui.Context) {
	t := theme.Current()
	uikit.Card(c).Key("gateway-empty").Grow(1).FillWidth().Center().Role(ui.RoleStatus).Label(locales.MessageNoGatewaysYet()).Children(func() {
		ui.Column(c).AlignItems(ui.Center).Gap(t.Space.N10).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Margin(0, 0, t.Space.N10, 0).Children(func() {
				tile := t.C.IconTile.SizeLg
				gatewayTile(c, uikit.IconSVG(uikit.IconLaptop), color.NRGBA{}, tile, t.Size.Icon, t.C.IconTile.RadiusLg)
				gatewayDashes(c, t.Space.N24*2)
				ui.Box(c).Size(tile+t.Space.N16, tile+t.Space.N16).Radius(t.Radius.Popover).Center().
					Background(uikit.Color(t.Color.Accent.Subtle)).Children(func() {
					ui.Icon(c, routeIcon).Size(t.Space.N22, t.Space.N22).TextColor(uikit.Color(t.Color.Accent.Default))
				})
				gatewayDashes(c, t.Space.N24*2)
				gatewayTile(c, serverIcon, color.NRGBA{}, tile, t.Size.Icon, t.C.IconTile.RadiusLg)
			})
			uikit.Text(c, locales.MessageNoGatewaysYet(), uikit.TextProps{Style: uikit.TextTitle})
			uikit.Text(c, locales.MessageGatewaysRouteYourConnectionsAddAnSshJumpHostOrSocks5ProxyThen(),
				uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).MaxWidth(t.C.EmptyState.MaxWidth).TextAlign(ui.Center)
			ui.Row(c).Gap(t.Space.N10).Margin(t.Space.N14, 0, 0, 0).Children(func() {
				choice := func(kind, title, detail string) {
					button := ui.ButtonBase(c).Key(kind).Width(t.Size.Sider).Padding(t.Space.N14).Gap(t.Space.N10).AlignItems(ui.Start).
						Radius(t.Radius.Card).Label(title).FocusRing(false)
					border, background := t.Color.Border.Control, ui.Transparent
					if button.Hovered() {
						border, background = t.Color.Border.ControlHover, uikit.Color(t.Color.State.Hover)
					}
					button.Border(t.Size.Divider, uikit.Color(border)).Background(background)
					if button.FocusVisible() {
						uikit.Shadow(button, t.Shadow.ControlFocus)
					}
					button.OnClick(func() { a.startGatewayDraft(gatewayConfig{Kind: kind}, false) })
					button.Children(func() {
						gatewayKindTile(c, kind, t.C.IconTile.Size, t.C.IconButton.IconSizeSm, t.C.IconTile.Radius)
						ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PassThrough().Children(func() {
							uikit.Text(c, title, uikit.TextProps{Style: uikit.TextHeadline}).PassThrough()
							uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).PassThrough()
						})
					})
				}
				choice("ssh", locales.MessageAddSshGateway(), locales.MessageUseASavedSshHostAsAJumpHostWithOptionalUpstreamGateways())
				choice("socks5", locales.MessageAddSocks5Proxy(), locales.MessageEnterAProxyAddressWithOptionalUsernameAndPasswordAuthentication())
				choice("wireguard", locales.WgAdd(), locales.WgDescription())
			})
		})
	})
}

// ---------- 列表 ----------

func (a *app) gatewayList(c *ui.Context) {
	page := a.page()
	live := 0
	for _, g := range a.gateways {
		if a.gatewayLive(g.ID) {
			live++
		}
	}
	result := uikit.Sider(c, locales.GatewaysTitle(), uikit.SiderProps{
		Label: locales.MessageGatewayList(), AddLabel: locales.MessageAddGateway(), AddIcon: plusIcon, FooterIcon: routeIcon,
		Footer: locales.GatewaysCount(len(a.gateways), live),
		Query:  &page.query,
		Search: uikit.SearchProps{Label: locales.MessageSearchGateways(), Placeholder: locales.MessageSearchGatewaysAndAddresses(), Icon: searchIcon,
			Handle: &page.search, Kbd: shortcutDisplay(a.shortcutBinding("host.search"))},
	}, func() { a.gatewayListBody(c) })
	result.Root.Width(page.sidebarWidth)
	if result.Add {
		a.startGatewayDraft(gatewayConfig{}, false)
	}
}

func (a *app) gatewayListBody(c *ui.Context) {
	t := theme.Current()
	state := &a.gatewayUI
	arrowAnimation := a.catalogDisplaceAnimation()
	query := strings.ToLower(strings.TrimSpace(a.page().query))
	viewport := ui.Box(c).Key("gateway-list-viewport").Grow(1).MinHeight(0).FillWidth().Clip()
	viewport.Children(func() {
		content := ui.Scroll(c.Key("gateway-list-scroll")).TrackScroll(&state.listScroll).Fill().MinHeight(0).
			Padding(t.C.Catalog.ListPaddingY, t.C.Catalog.ListPaddingX).Children(func() {
			ui.Column(c).FillWidth().Gap(t.C.Catalog.RowGap).Children(func() {
				shown := 0
				for _, kind := range gatewayKinds {
					items := []gatewayConfig{}
					for _, g := range a.gateways {
						if g.Kind == kind && (query == "" || strings.Contains(strings.ToLower(g.Name+" "+a.gatewayAddress(g)), query)) {
							items = append(items, g)
						}
					}
					if len(items) == 0 {
						continue
					}
					// A search shows every match without changing the saved collapse.
					open := query != "" || !state.collapsed[kind]
					title := locales.MessageSshGateway()
					if kind == "socks5" {
						title = locales.MessageSocks5Proxy()
					}
					if kind == "wireguard" {
						title = locales.WgGateway()
					}
					group := uikit.SiderGroup(c, uikit.SiderGroupProps{
						Key: "gateway-group-" + kind, Title: title, Count: len(items), Open: &open, Chevron: chevronIcon,
						Motion: uikit.MotionOverride(a.reduceMotion(c)), ArrowAnimation: &arrowAnimation, DeferPresentation: true,
					})
					group.Root.Transition(a.catalogTransition(catalogGroupDrag))
					if shown > 0 {
						group.Root.Margin(t.C.Catalog.GroupGap, 0, 0, 0)
					}
					group.Present(c, uikit.SiderGroupPresentation{DisableToggle: query != ""})
					if group.Toggled {
						if state.collapsed == nil {
							state.collapsed = map[string]bool{}
						}
						state.collapsed[kind] = !open
					}
					if open {
						for _, g := range items {
							a.gatewayRow(c, g)
						}
					}
					shown += len(items)
				}
				if shown == 0 {
					uikit.EmptyMessage(c, locales.MessageNoMatchingGateways(), uikit.EmptyCatalogSearch)
				}
			})
		})
		uikit.Scrollbars(
			c,
			viewport,
			content,
			&state.listScroll,
			uikit.ScrollbarProps{Label: locales.MessageGatewayListScrollbar()},
		)
	})
}

func (a *app) gatewayRow(c *ui.Context, g gatewayConfig) {
	t := theme.Current()
	p := t.C.SiderItem
	selected := a.gatewayUI.selected == g.ID
	address := a.gatewayAddress(g)
	row := ui.ButtonBase(c).Key(g.ID).Height(t.C.Catalog.RowHeight).Shrink(0).FillWidth().
		Padding(0, p.PaddingRight, 0, p.PaddingLeft).Gap(p.Gap).Radius(p.Radius).Role(ui.RoleListItem).
		Label(g.Name + "，" + address).Selected(selected).FocusRing(false).TextColor(uikit.Color(p.NameColor)).
		Transition(a.catalogTransition(catalogHostDrag))
	background := ui.Transparent
	if selected {
		background = uikit.Color(p.BgActive)
	} else if row.Hovered() {
		background = uikit.Color(p.BgHover)
	}
	row.Background(row.AnimateColor("row-bg", background, uikit.Tween(p.Transition)))
	if row.FocusVisible() {
		uikit.Shadow(row, t.Shadow.ControlFocus)
	}
	if row.Clicked() {
		a.selectGateway(g.ID)
	}
	hops := len(a.gatewayChain(g.ID))
	isDefault := a.defaultGateway == g.ID
	row.Children(func() {
		ui.Box(c).Shrink(0).PassThrough().Children(func() {
			gatewayKindTile(c, g.Kind, t.C.IconTile.Size, t.C.IconButton.IconSizeSm, t.C.IconTile.Radius)
			if a.gatewayLive(g.ID) {
				// The ring takes the row's resting color so the dot reads as cut into the tile.
				ring := uikit.Color(t.Color.Surface.Card)
				if selected {
					ring = uikit.Color(p.BgActive)
				}
				dot := t.Size.Dot + 2*t.Space.N2
				ui.Box(c).Absolute().Right(-t.Space.N3).Bottom(-t.Space.N3).Size(dot, dot).Radius(t.Radius.Pill).
					Background(uikit.Color(t.Color.Success.Default)).Border(t.Space.N2, ring).PassThrough()
			}
		})
		uikit.SiderRowLabel(c, g.Name, address)
		if isDefault || hops > 1 {
			ui.Column(c).Shrink(0).AlignItems(ui.End).Gap(t.Space.N2).PassThrough().Children(func() {
				if isDefault {
					uikit.Badge(c, locales.MessageDefault(), uikit.BadgeProps{Tone: uikit.ToneAccent}).PassThrough()
				}
				if hops > 1 {
					ui.Row(c).Gap(t.Space.N3).AlignItems(ui.Center).PassThrough().Children(func() {
						uikit.Icon(c, routeIcon, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary}).PassThrough()
						uikit.Text(
							c,
							locales.GatewayHopCount(hops),
							uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary},
						).PassThrough()
					})
				}
			})
		}
		more := uikit.SiderMore(c, row, uikit.SiderMoreProps{
			Icon: moreIcon, Label: locales.ActionsForName(g.Name),
		})
		uikit.RowMenuOn(c, row, more, a.gatewayMenu(g))
	})
}

// ---------- 详情 ----------

func (a *app) gatewayDetail(c *ui.Context) {
	t := theme.Current()
	state := &a.gatewayUI
	g, _ := a.gatewayByID(state.selected)
	refs := a.gatewayReferences(g.ID)
	live := slices.ContainsFunc(refs, func(ref gatewayRef) bool { return ref.kind == gatewayRefRuntime })
	ui.Row(c).Key("gateway-detail-header").FillWidth().Shrink(0).Padding(t.Space.N16, t.Space.N16, t.Space.N16, t.Space.N20).
		Gap(t.Space.N12).AlignItems(ui.Center).BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).Children(func() {
		gatewayKindTile(c, g.Kind, t.C.IconTile.SizeLg, t.Size.Icon, t.C.IconTile.RadiusLg)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
			ui.Row(c).MinWidth(0).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
				uikit.Text(c, g.Name, uikit.TextProps{Style: uikit.TextTitle}).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				uikit.Badge(c, gatewayKindLabel(g.Kind), uikit.BadgeProps{})
				if a.defaultGateway == g.ID {
					uikit.Badge(c, locales.MessageDefaultGateway(), uikit.BadgeProps{Tone: uikit.ToneAccent})
				}
				if live {
					uikit.Badge(c, locales.MessageInUse(), uikit.BadgeProps{Tone: uikit.ToneSuccess, Dot: true})
				}
			})
			uikit.Text(c, a.gatewayAddress(g), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary, Mono: true}).
				SingleLine().Ellipsis("…").Selectable()
		})
		ui.Row(c).Shrink(0).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
			uikit.Button(
				c,
				locales.MessageTestConnection(),
				uikit.ButtonProps{Icon: uikit.IconPlay, Disabled: a.gatewayTestRunning(g.ID)},
			).
				OnClick(func() { a.startGatewayTest(g.ID) })
			uikit.Button(
				c,
				locales.MessageEdit(),
				uikit.ButtonProps{Icon: uikit.IconEdit},
			).OnClick(func() { a.startGatewayDraft(g, false) })
			more := uikit.IconButton(
				c,
				moreIcon,
				uikit.IconButtonProps{Key: "gateway-more", Label: locales.MessageMoreGatewayActions(), Variant: uikit.IconFilled},
			)
			uikit.MenuOn(c, more, a.gatewayMenu(g))
		})
	})
	viewport := ui.Box(c).Key("gateway-detail-viewport").Grow(1).MinHeight(0).FillWidth().Clip()
	viewport.Children(func() {
		content := ui.Scroll(c.Key("gateway-detail-scroll")).TrackScroll(&state.scroll).Fill().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(t.Space.N18, t.Space.N20, t.Space.N24, t.Space.N20).Gap(t.Space.N24).Children(func() {
				ui.Column(c).Key("route").FillWidth().Children(func() {
					gatewaySectionTitle(c, locales.MessageConnectionRoute(), locales.MessageOutboundRouteForTerminalFileTransferAndMcpConnectionsUsingThisGateway(), nil)
					a.gatewayRoute(c, g)
					a.gatewayTestStrip(c, g)
				})
				ui.Column(c).Key("facts").FillWidth().Children(func() {
					gatewaySectionTitle(c, locales.MessageConfiguration(), "", nil)
					a.gatewayFacts(c, g)
				})
				ui.Column(c).Key("references").FillWidth().Children(func() {
					note := ""
					if len(refs) > 0 {
						note = locales.MessageRemoveAllReferencesBeforeDeletingThisGateway()
					}
					gatewaySectionTitle(c, locales.MessageReferences(), note, func() { uikit.Count(c, len(refs), uikit.CountProps{}) })
					a.gatewayReferenceGroups(c, refs)
				})
			})
		})
		uikit.Scrollbars(
			c,
			viewport,
			content,
			&state.scroll,
			uikit.ScrollbarProps{Label: locales.MessageGatewayDetailsScrollbar()},
		)
	})
}

// gatewaySectionTitle puts an optional count beside the title and an
// optional note at the trailing end.
func gatewaySectionTitle(c *ui.Context, title, note string, count func()) {
	t := theme.Current()
	ui.Row(c).FillWidth().Gap(t.Space.N8).AlignItems(ui.Center).Margin(0, 0, t.Space.N10, 0).Children(func() {
		uikit.Text(c, title, uikit.TextProps{Style: uikit.TextHeadline}).Shrink(0)
		if count != nil {
			count()
			ui.Spacer(c)
		}
		if note != "" {
			uikit.Text(c, note, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…").MinWidth(0)
		}
	})
}

type gatewayLinkState uint8

const (
	gatewayLinkIdle gatewayLinkState = iota
	gatewayLinkTarget
	gatewayLinkFlow
	gatewayLinkOK
	gatewayLinkFail
)

func (a *app) gatewayRoute(c *ui.Context, g gatewayConfig) {
	t := theme.Current()
	chain := a.gatewayChain(g.ID)
	test := a.gatewayTestHops(g.ID)
	reduced := a.reduceMotion(c)
	ui.Row(c).Key("gateway-route").FillWidth().Padding(t.Space.N24, t.Space.N18, t.Space.N22, t.Space.N18).
		Radius(t.Radius.Card).Background(uikit.Color(t.Color.Surface.Field)).AlignItems(ui.Center).Children(func() {
		gatewayRouteEnd(c, uikit.IconSVG(uikit.IconLaptop), locales.MessageOrigin(), locales.MessageThisComputer(), locales.MessageLocalNetwork(), false)
		for index, hop := range chain {
			label := locales.MessageDirect()
			if index > 0 {
				label = locales.MessageViaProxy()
				if chain[index-1].Kind == "ssh" {
					label = locales.MessageSshTunnel()
				}
				if chain[index-1].Kind == "wireguard" {
					label = "WireGuard"
				}
			}
			// Each hop lights once the test has connected through it.
			link, mark := test.link(hop.ID)
			gatewayRouteLink(c.Key("link-"+hop.ID), label, link, reduced)
			a.gatewayRouteNode(c, hop, hop.ID == g.ID, mark)
		}
		label := locales.MessageViaProxy()
		if g.Kind == "ssh" {
			label = locales.MessageSshTunnel()
		}
		if g.Kind == "wireguard" {
			label = "WireGuard"
		}
		// The test stops at this gateway, so the last leg stays an outline.
		gatewayRouteLink(c.Key("link-target"), label, gatewayLinkTarget, reduced)
		gatewayRouteEnd(c, serverIcon, locales.MessageDestination2(), locales.MessageTargetHost(), locales.MessageDeterminedByTheHostOrSite(), true)
	})
}

func gatewayRouteEnd(c *ui.Context, icon *ui.SVG, kind, title, detail string, target bool) {
	t := theme.Current()
	node := ui.Column(c).Key(kind).Basis(t.Size.Sider/2).Shrink(1).MinWidth(t.Space.N24*3).
		Padding(t.Space.N10).Gap(t.Space.N2).Radius(t.Radius.Lift).Border(t.Size.Divider, uikit.Color(t.Color.Border.Control))
	if target {
		node.Basis(t.Size.Sider*2/3).Border(t.Size.Divider, uikit.Color(t.Color.Border.ControlHover)).BorderStyle(ui.BorderDashed)
	}
	node.Children(func() {
		ui.Row(c).Gap(t.Space.N6).AlignItems(ui.Center).Margin(0, 0, t.Space.N6, 0).Children(func() {
			gatewayTile(c, icon, color.NRGBA{}, t.Space.N20, t.C.IconButton.IconSizeXs, t.Radius.Chip)
			uikit.Text(c, kind, uikit.TextProps{Style: uikit.TextCaption2, Tone: uikit.ToneTertiary})
		})
		uikit.Text(c, title, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…").MinWidth(0)
		uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…").MinWidth(0)
	})
}

func (a *app) gatewayRouteNode(c *ui.Context, hop gatewayConfig, current bool, mark gatewayLinkState) {
	t := theme.Current()
	var node ui.Element
	if current {
		node = uikit.Shadow(ui.Column(c).Key(hop.ID), t.Shadow.ControlFocus)
	} else {
		node = ui.ButtonBase(c).Key(hop.ID).Column().AlignItems(ui.Stretch).Label(locales.MessageViewGateway() + hop.Name).FocusRing(false)
		border := t.Color.Border.Raised
		if node.Hovered() {
			border = t.Color.Border.ControlHover
		}
		node.Border(t.Size.Divider, node.AnimateColor("border", uikit.Color(border), uikit.Tween(t.C.Button.Transition)))
		if node.FocusVisible() {
			uikit.Shadow(node, t.Shadow.ControlFocus)
		}
		node.OnClick(func() { a.selectGateway(hop.ID) })
	}
	node.Basis(t.Size.Sider * 3 / 4).Shrink(1).MinWidth(t.Space.N24 * 4).Padding(t.Space.N10).Gap(t.Space.N2).
		Radius(t.Radius.Lift).Background(uikit.Color(t.Color.Surface.Raised)).Children(func() {
		ui.Row(c).Gap(t.Space.N6).AlignItems(ui.Center).Margin(0, 0, t.Space.N6, 0).PassThrough().Children(func() {
			gatewayKindTile(c, hop.Kind, t.Space.N20, t.C.IconButton.IconSizeXs, t.Radius.Chip)
			uikit.Text(c, gatewayKindLabel(hop.Kind), uikit.TextProps{Style: uikit.TextCaption2, Tone: uikit.ToneTertiary}).PassThrough()
			ui.Spacer(c).PassThrough()
			switch mark {
			case gatewayLinkFlow:
				uikit.Spinner(c, uikit.SpinnerProps{Size: uikit.SizeXs, Tone: uikit.ToneAccent, Label: locales.MessageTesting2(),
					Motion: uikit.MotionOverride(a.reduceMotion(c))}).PassThrough()
			case gatewayLinkOK, gatewayLinkFail:
				icon, fill := uikit.IconCheck, t.Color.Success.Default
				if mark == gatewayLinkFail {
					icon, fill = uikit.IconClose, t.Color.Danger.Default
				}
				ui.Box(c).Size(t.Space.N18, t.Space.N18).Radius(t.Radius.Pill).Background(uikit.Color(fill)).Center().PassThrough().Children(func() {
					uikit.Icon(c, icon, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneOnAccent}).PassThrough()
				})
			}
		})
		uikit.Text(c, hop.Name, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…").MinWidth(0).PassThrough()
		endpoint := uikit.Text(c, a.gatewayEndpoint(hop), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary, Mono: true}).
			SingleLine().Ellipsis("…").MinWidth(0)
		uikit.Tip(c, endpoint, a.gatewayAddress(hop), "")
		if current {
			uikit.Badge(c, locales.MessageThisGateway(), uikit.BadgeProps{Tone: uikit.ToneOnAccent}).Absolute().
				Top(-t.C.Badge.HeightSm / 2).Left(t.Space.N10).PassThrough()
		}
	})
}

// gatewayRouteLink draws one leg of the route, with the way it is carried
// written above; a running test moves its dashes toward the gateway.
func gatewayRouteLink(c *ui.Context, label string, state gatewayLinkState, reduced bool) {
	t := theme.Current()
	ink, tone := t.Color.Border.ControlHover, uikit.ToneTertiary
	switch state {
	case gatewayLinkTarget:
		ink = t.Color.Text.Quaternary
	case gatewayLinkFlow:
		ink, tone = t.Color.Accent.Default, uikit.ToneAccent
	case gatewayLinkOK:
		ink = t.Color.Success.Default
	case gatewayLinkFail:
		ink = t.Color.Danger.Default
	}
	// The leg never narrows below its label; the nodes beside it shrink instead.
	labelWidth, _ := ui.ShapeText(label, uikit.TextFont(uikit.TextCaption2), 0).Size()
	ui.Column(c).Basis(t.Size.Control+t.Space.N16).Grow(1).Shrink(1).MinWidth(max(t.Size.ControlLg+t.Space.N10, labelWidth+2*t.Space.N6)).
		Padding(0, t.Space.N6).Gap(t.Space.N2).AlignItems(ui.Center).PassThrough().Children(func() {
		uikit.Text(c, label, uikit.TextProps{Style: uikit.TextCaption2, Tone: tone}).SingleLine().PassThrough()
		ui.Box(c).FillWidth().Height(t.Space.N10).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			y, stroke, head, end := r.Y+r.H/2, t.Size.FocusRing, t.Space.N4, r.X+r.W-t.Size.Divider
			var line ui.Path
			line.MoveTo(r.X, y).LineTo(end, y)
			switch state {
			case gatewayLinkTarget, gatewayLinkFail:
				line.Dash(0, t.Space.N4, t.Space.N4)
			case gatewayLinkFlow:
				const period = 450 * time.Millisecond
				offset := float32(0)
				if !reduced {
					offset = -float32(p.Now().UnixNano()%int64(period)) / float32(period) * t.Space.N10
					p.AnimationFrame()
				}
				line.Dash(offset, t.Space.N6, t.Space.N4)
			}
			p.StrokePath(&line, stroke, uikit.Color(ink))
			var arrow ui.Path
			arrow.MoveTo(end-head, y-head).LineTo(end, y).LineTo(end-head, y+head)
			p.StrokePath(&arrow, stroke, uikit.Color(ink))
		})
	})
}

func (a *app) gatewayTestStrip(c *ui.Context, g gatewayConfig) {
	t := theme.Current()
	p := t.C.Banner
	strip := func(background color.NRGBA, label string) ui.Element {
		return ui.Row(c).Key("gateway-test-strip").FillWidth().Margin(t.Space.N10, 0, 0, 0).Padding(p.PaddingY, p.PaddingX).
			Gap(p.Gap).Radius(p.Radius).Background(uikit.Color(background)).AlignItems(ui.Center).Role(ui.RoleStatus).Label(label)
	}
	retry := func() {
		uikit.Button(
			c,
			locales.MessageTestAgain(),
			uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm, Icon: uikit.IconRefresh},
		).
			Shrink(0).OnClick(func() { a.startGatewayTest(g.ID) })
	}
	chain := a.gatewayChain(g.ID)
	// The test's own route names its hops; the worker keeps its indexes within it.
	test := a.gatewayTestHops(g.ID)
	if a.gatewayTestRunning(g.ID) {
		hop := test.route[test.connecting].Name
		message := locales.GatewayConnecting(hop)
		if len(test.route) > 1 {
			message = fmt.Sprintf(locales.MessageConnectingToHop(), test.connecting+1, len(test.route), hop)
		}
		strip(p.InfoBg, message).Children(func() {
			uikit.Spinner(c, uikit.SpinnerProps{Size: uikit.SizeXs, Tone: uikit.ToneAccent, Label: locales.MessageTesting2(),
				Motion: uikit.MotionOverride(a.reduceMotion(c))})
			ui.Text(c, message).FontSize(t.Font.Size.Small).TextColor(uikit.Color(p.InfoText)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			uikit.Button(
				c,
				locales.CommonCancel(),
				uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm},
			).Shrink(0).OnClick(a.cancelGatewayTest)
		})
		return
	}
	record, tested := a.gatewayUI.results[g.ID]
	result := record.result
	if !tested {
		via := locales.MessageDirectly()
		if len(chain) > 1 {
			via = locales.MessageViaUpstreamGateway()
		}
		ui.Row(c).Key("gateway-test-hint").FillWidth().Margin(t.Space.N10, 0, 0, 0).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
			uikit.Icon(c, infoIcon, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary})
			uikit.Text(
				c,
				locales.GatewayTestHint(via, g.Kind),
				uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
			).
				Grow(1).MinWidth(0)
		})
		return
	}
	if result.Err == nil {
		strip(t.Color.Success.Subtle, result.Summary.String()).Children(func() {
			uikit.Icon(c, sessionCheckIcon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSuccess})
			uikit.Text(c, result.Summary.String(), uikit.TextProps{Style: uikit.TextBodyStrong, Tone: uikit.ToneSuccess}).Shrink(0)
			uikit.Text(c, result.Detail.String(), uikit.TextProps{Style: uikit.TextSmall}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			retry()
		})
		return
	}
	// Name the hop that failed: 「test」连接失败 rather than the gateway under test.
	summary := result.Summary.String()
	if test.failed >= 0 {
		summary = locales.GatewayFailureAtHop(test.route[test.failed].Name, summary)
	}
	strip(p.DangerBg, summary).AlignItems(ui.Start).Children(func() {
		uikit.Icon(c, sessionAlertIcon, uikit.IconProps{Size: uikit.SizeSm}).TextColor(uikit.Color(p.DangerIcon)).Margin(t.Space.N2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
			uikit.Text(c, summary, uikit.TextProps{Style: uikit.TextBodyStrong, Tone: uikit.ToneDanger})
			uikit.Text(c, result.Detail.String(), uikit.TextProps{Style: uikit.TextFootnote, Mono: true}).Selectable()
		})
		retry()
	})
}

func (a *app) gatewayFacts(c *ui.Context, g gatewayConfig) {
	t := theme.Current()
	type fact struct {
		label, value string
		mono         bool
		render       func()
	}
	tile, glyph := t.Space.N18, t.C.IconButton.IconSizeXs
	chip := func(key string, leading func(), label string, open func()) func() {
		return func() {
			button := ui.ButtonBase(c).Key(key).Height(t.Size.ControlSm).Padding(0, t.Space.N6, 0, t.Space.N3).Gap(t.Space.N6).
				Radius(t.Radius.ControlSm).AlignItems(ui.Center).Label(label).FocusRing(false)
			background := t.Color.Surface.Control
			if button.Hovered() {
				background = t.Color.Surface.ControlStrong
			}
			button.Background(button.AnimateColor("chip-bg", uikit.Color(background), uikit.Tween(t.C.Button.Transition)))
			if button.FocusVisible() {
				uikit.Shadow(button, t.Shadow.ControlFocus)
			}
			button.OnClick(open).Children(func() {
				leading()
				uikit.Text(c, label, uikit.TextProps{Style: uikit.TextSmall}).SingleLine().PassThrough()
				uikit.Icon(c, uikit.IconArrowRight, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary}).PassThrough()
			})
		}
	}
	// The gateway just before this one, whether chosen here (SOCKS5) or
	// followed from the source host (SSH).
	previous := func() { uikit.Text(c, locales.MessageDirect(), uikit.TextProps{}).Shrink(0) }
	if chain := a.gatewayChain(g.ID); len(chain) > 1 {
		u := chain[len(chain)-2]
		previous = chip("upstream", func() { gatewayKindTile(c, u.Kind, tile, glyph, t.Radius.Chip) }, u.Name,
			func() { a.selectGateway(u.ID) })
	}
	upstream := fact{label: locales.MessageUpstreamGateway(), render: func() {
		previous()
		if g.Kind == "ssh" {
			uikit.Text(
				c,
				locales.MessageUseSourceHostRoute(),
				uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary},
			).SingleLine().Ellipsis("…").MinWidth(0)
		}
	}}
	var facts []fact
	note := locales.MessageTheProxyResolvesTheTargetHostnameFtpFtpsThroughAGatewayRequiresPassive()
	var editHost func()
	if g.Kind == "ssh" {
		h, id, _ := a.gatewayHost(g)
		auth := map[string]string{"password": locales.CommonPassword(), "key": locales.MessagePrivateKeys(), "agent": "SSH Agent"}[h.session.Auth]
		if h.session.Auth == "password" && h.session.PasswordSaved {
			auth += locales.MessageSaved()
		}
		keepalive := locales.CommonClose()
		if h.session.SSH != nil && h.session.SSH.KeepAlive > 0 {
			keepalive = strconv.Itoa(h.session.SSH.KeepAlive) + locales.MessageS()
		}
		editHost = func() { a.openGatewayReference(gatewayRef{kind: gatewayRefHost, host: id}) }
		facts = []fact{
			{label: locales.MessageSourceHost(), render: chip("source", func() { gatewayTile(c, terminalIcon, color.NRGBA{}, tile, glyph, t.Radius.Chip) }, h.name, editHost)},
			{label: locales.MessageAddress(), value: a.gatewayAddress(g), mono: true},
			{label: locales.MessageAuthentication(), value: auth},
			{label: "Keepalive", value: keepalive},
			upstream,
			{label: locales.MessageConnectionSharing(), value: locales.MessageMultipleTargetsShareOneSshConnection()},
		}
		note = locales.GatewaySourceSettings(h.name)
	} else if g.Kind == "wireguard" {
		facts = []fact{
			{label: locales.WgEndpoint(), value: g.WG.Endpoint, mono: true},
			{label: locales.WgAddresses(), value: strings.Join(g.WG.Addresses, ", "), mono: true},
			{label: locales.WgAllowedIPs(), value: strings.Join(g.WG.AllowedIPs, ", "), mono: true},
			{label: locales.WgDNS(), value: strings.Join(g.WG.DNS, ", "), mono: true},
			{label: locales.WgPeerKey(), value: g.WG.PublicKey, mono: true},
			{label: locales.WgClientKey(), value: g.WG.ClientPublicKey, mono: true},
			{label: locales.WgMTU(), value: strconv.Itoa(g.WG.MTU), mono: true},
			{label: locales.MessageConnectionSharing(), value: locales.WgShared()},
		}
		note = locales.WgUserSpace() + " " + locales.WgFirstHop()
	} else {
		auth, user, password := locales.MessageNoAuthentication(), "—", "—"
		if g.SOCKS.Username != "" {
			auth, user, password = locales.MessageUsernameAndPassword(), g.SOCKS.Username, locales.MessageAskWhenConnecting()
			if g.SOCKS.PasswordSaved {
				password = locales.MessageSavedInVault()
			}
		}
		facts = []fact{
			{label: locales.MessageAddress(), value: a.gatewayEndpoint(g), mono: true},
			{label: locales.MessageAuthentication(), value: auth},
			{label: locales.MessageUsername(), value: user, mono: g.SOCKS.Username != ""},
			{label: locales.CommonPassword(), value: password},
			upstream,
			{label: locales.MessageConnectionMode(), value: locales.MessageASeparateConnectionForEachTarget()},
		}
	}
	for start := 0; start < len(facts); start += 2 {
		ui.Row(c).Key(start).FillWidth().Gap(t.Space.N24).Children(func() {
			for _, item := range facts[start:min(start+2, len(facts))] {
				ui.Row(c).Key(item.label).Grow(1).Basis(0).MinWidth(0).MinHeight(t.Size.ControlLg+t.Space.N6).
					Gap(t.Space.N12).AlignItems(ui.Center).BorderWidth(0, 0, t.Size.Divider, 0).
					BorderColor(uikit.Color(t.Color.Border.Subtle)).Children(func() {
					uikit.Text(c, item.label, uikit.TextProps{Tone: uikit.ToneSecondary}).Width(t.C.FormRow.LabelWidth).Shrink(0).SingleLine()
					if item.render != nil {
						item.render()
						return
					}
					uikit.Text(c, item.value, uikit.TextProps{Mono: item.mono}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Selectable()
				})
			}
		})
	}
	ui.Row(c).Key("note").FillWidth().Margin(t.Space.N12, 0, 0, 0).Gap(t.Space.N6).AlignItems(ui.Start).Children(func() {
		uikit.Icon(c, infoIcon, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary}).Margin(t.Space.N2, 0, 0, 0)
		uikit.Text(c, note, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Grow(1).MinWidth(0)
		if editHost != nil {
			uikit.Button(
				c,
				locales.MessageEditHost(),
				uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm},
			).Shrink(0).OnClick(editHost)
		}
	})
}

func (a *app) gatewayReferenceGroups(c *ui.Context, refs []gatewayRef) {
	t := theme.Current()
	if len(refs) == 0 {
		ui.Row(c).FillWidth().Padding(t.Space.N12, t.Space.N14).Gap(t.Space.N8).AlignItems(ui.Center).Radius(t.Radius.Lift).
			Border(t.Size.Divider, uikit.Color(t.Color.Border.Subtle)).Children(func() {
			uikit.Icon(c, sessionCheckIcon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSuccess})
			uikit.Text(
				c,
				locales.MessageNoSessionsGatewaysOrActiveConnectionsReferenceThisGatewayItCanBeDeleted(),
				uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
			)
		})
		return
	}
	groups := [][]gatewayRef{}
	for kind := range gatewayRefGroups() {
		group := []gatewayRef{}
		for _, ref := range refs {
			if ref.kind == gatewayRefKind(kind) {
				group = append(group, ref)
			}
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	// Groups alternate between two columns so a short group never leaves a
	// gap beside a long one.
	ui.Row(c).FillWidth().Gap(t.Space.N10).AlignItems(ui.Start).Children(func() {
		for column := range 2 {
			ui.Column(c).Key(column).Grow(1).Basis(0).MinWidth(0).Gap(t.Space.N10).Children(func() {
				for index := column; index < len(groups); index += 2 {
					group := groups[index]
					ui.Column(c).Key(group[0].kind).FillWidth().Padding(t.Space.N6).Radius(t.Radius.Lift).
						Border(t.Size.Divider, uikit.Color(t.Color.Border.Subtle)).Children(func() {
						ui.Row(c).Padding(t.Space.N4, t.Space.N6, t.Space.N6, t.Space.N6).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
							uikit.Text(c, gatewayRefGroups()[group[0].kind], uikit.TextProps{Style: uikit.TextLabel, Tone: uikit.ToneSecondary})
							uikit.Count(c, len(group), uikit.CountProps{})
						})
						for position, ref := range group {
							a.gatewayReferenceRow(c.Key(position), ref, true)
						}
					})
				}
			})
		}
	})
}

func (a *app) gatewayReferenceRow(c *ui.Context, ref gatewayRef, interactive bool) {
	t := theme.Current()
	navigable := interactive && (ref.kind == gatewayRefHost || ref.kind == gatewayRefGateway)
	var row ui.Element
	if navigable {
		label := locales.MessageEdit2() + ref.label
		if ref.kind == gatewayRefGateway {
			label = locales.MessageViewGateway() + ref.label
		}
		row = ui.ButtonBase(c).Label(label).FocusRing(false)
		background := ui.Transparent
		if row.Hovered() {
			background = uikit.Color(t.Color.State.Hover)
		}
		row.Background(row.AnimateColor("ref-bg", background, uikit.Tween(t.C.Button.Transition)))
		if row.FocusVisible() {
			uikit.Shadow(row, t.Shadow.ControlFocus)
		}
		row.OnClick(func() { a.openGatewayReference(ref) })
	} else {
		row = ui.Row(c).Label(ref.label)
	}
	row.FillWidth().Height(t.Size.ControlLg+t.Space.N2).Padding(0, t.Space.N6).Gap(t.Space.N8).Radius(t.Radius.ControlSm).
		AlignItems(ui.Center).Description(ref.detail).Children(func() {
		box, glyph, radius := t.C.IconTile.SizeSm, t.C.IconButton.IconSizeXs, t.C.IconTile.RadiusSm
		switch ref.kind {
		case gatewayRefDefault:
			gatewayTile(c, routeIcon, color.NRGBA{}, box, glyph, radius)
		case gatewayRefHost:
			protocol := map[string]uikit.Protocol{"": uikit.ProtocolSSH, "sftp": uikit.ProtocolSFTP, "ftp": uikit.ProtocolFTP}[a.hosts[ref.host].protocol]
			uikit.ProtocolIcon(c, protocol, uikit.SizeSm)
		case gatewayRefGateway:
			g, _ := a.gatewayByID(ref.gateway)
			gatewayKindTile(c, g.Kind, box, glyph, radius)
		default:
			gatewayTile(c, activityIcon, color.NRGBA{}, box, glyph, radius)
		}
		uikit.Text(c, ref.label, uikit.TextProps{}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").PassThrough()
		uikit.Text(c, ref.detail, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Shrink(0).SingleLine().PassThrough()
		if navigable {
			shown := float32(0)
			if row.Hovered() || row.FocusVisible() {
				shown = 1
			}
			uikit.Icon(c, uikit.IconArrowRight, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary}).PassThrough().
				Opacity(uikit.Visual(row, "ref-go", shown, t.C.Button.Transition))
		}
	})
}

// ---------- 测试 ----------

func (a *app) gatewayTestRunning(id string) bool {
	return a.gatewayUI.testResults != nil && a.gatewayUI.testGateway == id
}

// gatewayTestHops reports how many hops a gateway's test has connected, the
// hop a running test is connecting and the hop a finished test failed at;
// -1 marks neither.
func (a *app) gatewayTestHops(id string) gatewayTestView {
	if a.gatewayTestRunning(id) {
		progress := a.gatewayUI.testProgress
		return gatewayTestView{route: progress.route, proven: int(progress.proven.Load()), connecting: int(progress.current.Load()), failed: -1}
	}
	record, ok := a.gatewayUI.results[id]
	if !ok {
		return gatewayTestView{connecting: -1, failed: -1}
	}
	return gatewayTestView{route: record.route, proven: record.proven, connecting: -1, failed: record.failed}
}

func (a *app) cancelGatewayTest() {
	state := &a.gatewayUI
	if state.testCancel != nil {
		state.testCancel()
		state.testCancel = nil
	}
	state.testResults, state.testGateway, state.testProgress = nil, "", nil
}

func (a *app) startGatewayTest(id string) {
	a.cancelGatewayTest()
	state := &a.gatewayUI
	delete(state.results, id)
	progress := &gatewayTestProgress{}
	work, err := a.prepareGatewayTest(id, progress)
	if err != nil {
		a.recordGatewayTest(
			id,
			gatewayTestRecord{result: connectionTestResult{Summary: locales.Deferred(
				locales.MessageUnableToTestGateway,
			), Detail: locales.ErrorText(
				err,
			), Err: err}, failed: -1},
		)
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	results := make(chan connectionTestResult, 1)
	state.testGateway, state.testCancel, state.testResults, state.testProgress = id, cancel, results, progress
	a.sshWorkers.Go(func() { results <- work(ctx) })
}

func (a *app) recordGatewayTest(id string, record gatewayTestRecord) {
	if a.gatewayUI.results == nil {
		a.gatewayUI.results = map[string]gatewayTestRecord{}
	}
	a.gatewayUI.results[id] = record
}

// collectGatewayTest runs while the gateway page is shown; leaving the page
// cancels the test.
func (a *app) collectGatewayTest(c *ui.Context) {
	state := &a.gatewayUI
	if state.testResults == nil {
		return
	}
	select {
	case result := <-state.testResults:
		id, progress := state.testGateway, state.testProgress
		record := gatewayTestRecord{result: result, route: progress.route, proven: int(progress.proven.Load()), failed: -1}
		if result.Err != nil {
			record.failed = int(progress.current.Load())
		}
		a.cancelGatewayTest()
		a.recordGatewayTest(id, record)
		c.Announce(result.Summary.String())
	default:
		c.After(theme.Current().C.TestResult.Crossfade)
	}
}

// ---------- 弹窗 ----------

func (a *app) gatewayDialog(c *ui.Context) {
	state := &a.gatewayUI
	if !state.open || state.draft == nil {
		return
	}
	d := state.draft
	if d.deleting {
		a.gatewayDeleteDialog(c, d)
	} else {
		a.gatewayEditDialog(c, d)
	}
	if !state.open {
		d.password = ""
		d.wg.private, d.wg.psk = "", ""
		state.draft = nil
	}
}

func (a *app) gatewayDeleteDialog(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	refs := a.gatewayReferences(d.value.ID)
	uikit.Modal(c.Key(d.value.ID).Key("delete"), &a.gatewayUI.open, uikit.ModalProps{
		Controller: &d.modal, Title: locales.GatewayDeleteTitle(d.value.Name), Width: t.C.Dialog.HostWidth,
		Compact: true, Disabled: a.catalogSaving, Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		if len(refs) > 0 {
			uikit.Banner(c, locales.GatewayReferencesRemain(len(refs)),
				uikit.BannerProps{Tone: uikit.ToneWarning, Icon: sessionAlertIcon})
			ui.Column(c).FillWidth().Padding(t.Space.N4).Radius(t.Radius.Lift).Border(t.Size.Divider, uikit.Color(t.Color.Border.Subtle)).Children(func() {
				for index, ref := range refs {
					ref.detail = gatewayRefGroups()[ref.kind]
					a.gatewayReferenceRow(c.Key(index), ref, false)
				}
			})
		} else {
			message := locales.MessageThisCannotBeUndone()
			if d.value.SOCKS != nil && d.value.SOCKS.PasswordSaved {
				message += locales.MessageTheSavedSocks5PasswordWillAlsoBeRemovedFromVault()
			}
			if d.value.WG != nil {
				message += " " + locales.WgDeleteKeys()
			}
			uikit.Text(c, message, uikit.TextProps{Tone: uikit.ToneSecondary})
		}
		if !d.errorText.Empty() {
			uikit.Banner(c, d.errorText.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
		}
		ui.Row(c).FillWidth().Gap(t.Space.N8).Justify(ui.End).Children(func() {
			uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			uikit.Button(
				c,
				locales.CommonDelete(),
				uikit.ButtonProps{Variant: uikit.ButtonDanger, Disabled: len(refs) > 0},
			).OnClick(func() { a.deleteGateway(d) })
		})
	})
}

func (a *app) gatewayEditDialog(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	p := t.C.Modal
	title := locales.MessageAddGateway()
	if d.editing {
		title = locales.MessageEditGateway2()
	}
	dirty := d.inputValues() != d.initial || d.passwordChanged || d.clearPassword
	result := uikit.Modal(c.Key(d.value.ID), &a.gatewayUI.open, uikit.ModalProps{
		Controller: &d.modal, Title: title, Dirty: dirty, Disabled: a.catalogSaving,
		CloseKey: "gateway-close", CloseLabel: locales.MessageCloseGatewaySettings(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Header: func(uikit.ModalParts) {
			ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
				Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Key("gateway-header-" + d.value.Kind).Shrink(0).
					Transition(ui.ElementTransition{Duration: p.Fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).Children(func() {
					gatewayKindTile(c, d.value.Kind, t.C.IconTile.SizeLg, t.Size.Icon, t.C.IconTile.RadiusLg)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PaddingEnd(t.C.IconButton.Size).Children(func() {
					uikit.Text(c, title, uikit.TextProps{Style: uikit.TextTitle}).SingleLine()
					subtitle := locales.MessageAfterSavingSelectThisGatewayForHostsFileSitesOrTheAppDefault()
					if d.editing {
						subtitle = a.gatewayName(d.value.ID) + " · " + gatewayKindLabel(d.value.Kind)
					}
					uikit.Text(c, subtitle, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary}).SingleLine()
				})
			})
		},
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Children(func() {
				ui.Spacer(c)
				uikit.Button(
					c,
					locales.CommonCancel(),
					uikit.ButtonProps{Variant: uikit.ButtonPlain},
				).Padding(0, p.ButtonPaddingX).OnClick(parts.RequestDismiss)
				uikit.Button(
					c,
					locales.CommonSave(),
					uikit.ButtonProps{Variant: uikit.ButtonPrimary, Kbd: "↩", Disabled: d.wg.importing},
				).Padding(0, p.ButtonPaddingX).Shrink(0).
					OnClick(func() { a.saveGateway(d) })
			})
		},
	}, func(parts uikit.ModalParts) {
		headerHeight := p.HeaderPaddingTop + p.IconTile + p.HeaderPaddingBottom
		footerHeight := p.FooterPaddingTop + t.C.Button.Height + p.FooterPaddingBottom
		scroller := ui.Scroll(c).Key("gateway-form-scroll").FillWidth().MinHeight(0).Shrink(1).
			MaxHeight(parts.Bounds.MaxHeight - headerHeight - footerHeight).TrackScroll(&d.scroll).Children(func() {
			kind := slices.Index(gatewayKinds, d.value.Kind)
			ui.Column(c).FillWidth().Shrink(0).Padding(0, p.BodyPaddingX, t.Space.N8, p.BodyPaddingX).Children(func() {
				uikit.Segmented(c.Key("gateway-kind"), &kind, []uikit.SegItem{
					{Label: "SSH", IconSVG: terminalIcon, IconTone: uikit.ToneAccent},
					{Label: "Socks5", IconSVG: globeIcon, IconTone: uikit.ToneTertiary},
					{Label: "WireGuard", IconSVG: routeIcon, IconTone: uikit.ToneAccent},
				}, uikit.SegmentedProps{Label: locales.MessageGatewayType(), Fill: true, Cards: true})
			})
			if gatewayKinds[kind] != d.value.Kind {
				d.value.Kind, d.errorText = gatewayKinds[kind], locales.Text{}
				if d.value.Kind == "wireguard" {
					d.value.UpstreamID = ""
				}
			}
			uikit.FormSection(c, "", func() { a.gatewayDraftFields(c, d) }).
				Padding(t.Space.N8, p.BodyPaddingX, t.Space.N16, p.BodyPaddingX)
		})
		uikit.Scrollbars(
			c,
			parts.Panel,
			scroller,
			&d.scroll,
			uikit.ScrollbarProps{Label: locales.MessageGatewayFormScrollbar()},
		)
	})
	if result.Submit {
		a.saveGateway(d)
	}
}

func (a *app) gatewayDraftFields(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	uikit.FormRow(c, locales.CommonName(), uikit.FormRowProps{Key: "name"}, func() ui.Element {
		return uikit.Input(
			c,
			&d.value.Name,
			uikit.InputProps{Label: locales.MessageGatewayName(), Placeholder: locales.MessageForExampleBastionHk()},
		)
	})
	catalog := a.gatewayCatalog()
	// loops reports whether the draft, changed by edit, would reach itself.
	loops := func(edit func(*gatewayConfig)) bool {
		candidate := d.value
		edit(&candidate)
		var loop *gatewayLoop
		_, err := catalog.withGateway(candidate).gatewayRoute(candidate.ID, nil)
		return errors.As(err, &loop)
	}
	if d.value.Kind == "ssh" {
		options := []uikit.Option[string]{}
		for _, h := range a.hosts {
			if h.protocol != "" {
				continue
			}
			option := uikit.Option[string]{Value: h.session.ID, Label: h.name, Detail: h.user + "@" + h.address + ":" + strconv.Itoa(h.port)}
			if loops(func(g *gatewayConfig) { g.SSHProfileID = h.session.ID }) {
				option.Detail, option.Disabled = locales.MessageWouldCreateARoutingLoop(), true
			}
			options = append(options, option)
		}
		uikit.FormRow(c, locales.MessageSshHost(), uikit.FormRowProps{Key: "ssh-host", Help: locales.MessageUsesThisHostSAddressAuthenticationAndKeepaliveSettingsItsInitialCommandIs()}, func() ui.Element {
			return uikit.Select(
				c,
				&d.value.SSHProfileID,
				options,
				uikit.SelectProps{Label: locales.MessageSshHost(), Placeholder: locales.MessageSelectASavedSshHost()},
			)
		})
		uikit.FormRow(c, locales.MessageUpstreamGateway(), uikit.FormRowProps{Key: "upstream", Help: locales.MessageAnSshGatewayUsesItsSourceHostSGatewayRouteJustLikeA()}, func() ui.Element {
			return ui.Row(c).FillWidth().Height(t.C.Input.Height).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
				source, ok := catalog.sshHost(d.value.SSHProfileID)
				if !ok {
					uikit.Text(c, locales.MessageSelectAnSshHostFirst(), uikit.TextProps{Tone: uikit.ToneTertiary})
					return
				}
				selection := locales.MessageUseAppDefault()
				switch source.session.Gateway {
				case "":
				case directGateway:
					selection = locales.MessageDirect()
				default:
					selection = a.gatewayName(source.session.Gateway)
				}
				uikit.Text(c, locales.GatewayFollowName(source.name), uikit.TextProps{}).Shrink(0)
				uikit.Text(c, "· "+selection, uikit.TextProps{Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…").MinWidth(0)
			})
		})
	} else if d.value.Kind == "wireguard" {
		a.wireguardDraftFields(c, d)
	} else {
		if d.value.SOCKS == nil {
			d.value.SOCKS = &socksGateway{Port: 1080}
		}
		uikit.FormRow(c, locales.MessageProxyAddress(), uikit.FormRowProps{Key: "socks-address"}, func() ui.Element {
			return ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
				uikit.Input(
					c,
					&d.value.SOCKS.Address,
					uikit.InputProps{Label: locales.MessageSocks5Address(), Placeholder: locales.MessageHostnameOrIp(), Mono: true},
				).Grow(1)
				uikit.PortInput(
					c,
					&d.port,
					uikit.InputProps{Label: locales.MessageSocks5Port(), Width: t.C.Dialog.PortWidth},
				).Shrink(0)
			})
		})
		uikit.FormRow(c, locales.MessageAuthentication(), uikit.FormRowProps{Key: "socks-auth", Align: uikit.RowAlignCenter}, func() ui.Element {
			return uikit.Segmented(
				c,
				&d.auth,
				uikit.SegmentItems([]string{locales.MessageNoAuthentication(), locales.MessageUsernameAndPassword()}),
				uikit.SegmentedProps{Label: locales.MessageSocks5Authentication2(), Size: uikit.SizeSm},
			)
		})
		if d.auth == 1 {
			uikit.FormRow(c, locales.MessageUsername(), uikit.FormRowProps{Key: "socks-user"}, func() ui.Element {
				return uikit.Input(c, &d.value.SOCKS.Username, uikit.InputProps{Label: locales.MessageSocks5Username()})
			})
			placeholder, help := locales.MessageLeaveBlankToEnterWhenConnecting(), locales.MessageSavedPasswordsAreEncryptedInVault()
			if d.value.SOCKS.PasswordSaved {
				placeholder, help = locales.MessageSavedInVaultLeaveBlankToKeepUnchanged(), ""
			}
			uikit.FormRow(c, locales.CommonPassword(), uikit.FormRowProps{Key: "socks-password", Help: help}, func() ui.Element {
				input := uikit.PasswordInput(
					c,
					&d.password,
					uikit.InputProps{Label: locales.MessageSocks5Password(), Placeholder: placeholder},
				).Input
				if input.Changed() {
					d.passwordChanged = true
				}
				return input
			})
			if d.value.SOCKS.PasswordSaved {
				uikit.FormRow(c, locales.MessageSavedPassword(), uikit.FormRowProps{Key: "socks-clear", Inline: true}, func() ui.Element {
					return uikit.Checkbox(c, &d.clearPassword, locales.MessageClearSavedPasswords())
				})
			}
		}
		options := []uikit.Option[string]{{Value: "", Label: locales.MessageDirect(), Detail: locales.MessageLocalNetwork2()}}
		for _, g := range a.gateways {
			if g.ID == d.value.ID {
				continue
			}
			option := uikit.Option[string]{Value: g.ID, Label: g.Name, Detail: gatewayKindLabel(g.Kind)}
			if loops(func(candidate *gatewayConfig) { candidate.UpstreamID = g.ID }) {
				option.Detail, option.Disabled = locales.MessageWouldCreateARoutingLoop(), true
			}
			options = append(options, option)
		}
		uikit.FormRow(c, locales.MessageUpstreamGateway(), uikit.FormRowProps{Key: "upstream", Help: locales.MessageTheOutboundRouteUsedToConnectToTheProxyServer()}, func() ui.Element {
			return uikit.Select(c, &d.value.UpstreamID, options, uikit.SelectProps{Label: locales.MessageUpstreamGateway()})
		})
	}
	uikit.FormRow(c, locales.MessageRoutePreview(), uikit.FormRowProps{Key: "route-preview"}, func() ui.Element {
		return a.gatewayRoutePreview(c, d)
	})
	if !d.errorText.Empty() {
		uikit.Banner(c, d.errorText.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
	}
}

func (a *app) gatewayRoutePreview(c *ui.Context, d *gatewayDraft) ui.Element {
	t := theme.Current()
	// An SSH gateway's route is known once its source host is chosen.
	chain, err := []gatewayConfig{d.value}, error(nil)
	if d.value.Kind != "ssh" || d.value.SSHProfileID != "" {
		chain, err = a.gatewayCatalog().withGateway(d.value).chain(d.value.ID)
	}
	if err != nil {
		return ui.Row(c).FillWidth().Gap(t.Space.N6).Padding(t.Space.N8, t.Space.N10).AlignItems(ui.Center).
			Radius(t.Radius.Control).Background(uikit.Color(t.C.Banner.DangerBg)).Children(func() {
			uikit.Icon(c, sessionAlertIcon, uikit.IconProps{Size: uikit.SizeSm}).TextColor(uikit.Color(t.C.Banner.DangerIcon))
			uikit.Text(c, err.Error(), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneDanger}).Grow(1).MinWidth(0)
		})
	}
	chain = chain[:len(chain)-1]
	name := strings.TrimSpace(d.value.Name)
	if name == "" {
		name = locales.MessageNewGateway()
	}
	box, glyph := t.Space.N18, t.C.IconButton.IconSizeXs
	chip := func(key string, leading func(), label string, current, target bool) {
		e := ui.Row(c).Key(key).Height(t.Size.ControlSm).Padding(0, t.Space.N8, 0, t.Space.N3).Gap(t.Space.N6).
			Radius(t.Radius.ControlSm).AlignItems(ui.Center).Background(uikit.Color(t.Color.Surface.Raised)).
			Border(t.Size.Divider, uikit.Color(t.Color.Border.Raised))
		if current {
			e.Border(t.Size.FocusRing, uikit.Color(t.Color.Accent.Default))
		}
		if target {
			e.Padding(0, t.Space.N8).Background(ui.Transparent).Border(t.Size.Divider, uikit.Color(t.Color.Border.ControlHover)).BorderStyle(ui.BorderDashed)
		}
		e.Children(func() {
			if leading != nil {
				leading()
			}
			tone := uikit.ToneNeutral
			if target {
				tone = uikit.ToneSecondary
			}
			uikit.Text(c, label, uikit.TextProps{Style: uikit.TextSmall, Tone: tone}).SingleLine()
		})
	}
	arrow := func() {
		uikit.Icon(c, uikit.IconArrowRight, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary})
	}
	return ui.Row(c).FillWidth().Wrap().Gap(t.Space.N6).Padding(t.Space.N8, t.Space.N10).AlignItems(ui.Center).
		Radius(t.Radius.Control).Background(uikit.Color(t.Color.Surface.Field)).Children(func() {
		chip("local", func() { gatewayTile(c, uikit.IconSVG(uikit.IconLaptop), color.NRGBA{}, box, glyph, t.Radius.Chip) }, locales.MessageThisComputer(), false, false)
		for _, hop := range chain {
			arrow()
			chip(hop.ID, func() { gatewayKindTile(c, hop.Kind, box, glyph, t.Radius.Chip) }, hop.Name, false, false)
		}
		arrow()
		chip("self", func() { gatewayKindTile(c, d.value.Kind, box, glyph, t.Radius.Chip) }, name, true, false)
		arrow()
		chip("target", nil, locales.MessageTargetHost(), false, true)
	})
}

func (a *app) saveGateway(d *gatewayDraft) {
	if a.catalogSaving || d.wg.importing {
		return
	}
	g := d.value
	items := []vault.PlainItem{}
	deleted := []string{}
	if g.Kind == "ssh" {
		g.SOCKS, g.WG, g.UpstreamID = nil, nil, ""
	} else if g.Kind == "wireguard" {
		var err error
		g, items, err = a.prepareWireguardSave(d)
		if err != nil {
			d.errorText = locales.ErrorText(err)
			return
		}
	} else {
		g.WG = nil
		g.SSHProfileID = ""
		options := *g.SOCKS
		options.Port, _ = strconv.Atoi(d.port)
		options.Address = strings.TrimSpace(options.Address)
		options.Username = strings.TrimSpace(options.Username)
		if d.auth == 0 {
			options.Username = ""
		}
		if d.clearPassword || options.Username == "" {
			options.PasswordSaved = false
			options.PasswordRevision = ""
			deleted = append(deleted, gatewayCredentialKey(g.ID))
		}
		if d.passwordChanged && d.password != "" && options.Username != "" {
			if len(d.password) > 255 {
				d.errorText = locales.Deferred(locales.MessageTheSocks5PasswordCannotExceedBytes)
				return
			}
			options.PasswordSaved = true
			options.PasswordRevision = newSessionID()
			data, err := json.Marshal(sessionCredential{Revision: options.PasswordRevision, Secret: d.password})
			if err != nil {
				d.errorText = locales.ErrorText(err)
				return
			}
			defer clear(data)
			items = append(items, vault.PlainItem{
				ID: gatewayCredentialKey(g.ID), Kind: "gateway_password",
				Revision: options.PasswordRevision, Data: data,
			})
			deleted = nil
		}
		g.SOCKS = &options
	}
	if g.Kind == "ssh" {
		deleted = append(deleted, gatewayCredentialKey(g.ID))
	}
	if g.Kind == "wireguard" {
		deleted = append(deleted, gatewayCredentialKey(g.ID))
	} else {
		deleted = append(deleted, wgCredentialKey(g.ID))
	}
	g.Name = strings.TrimSpace(g.Name)
	for _, item := range items {
		defer clear(item.Data)
	}
	index := slices.IndexFunc(a.gateways, func(value gatewayConfig) bool { return value.ID == g.ID })
	previous := a.gateways
	a.gateways = slices.Clone(previous)
	if index < 0 {
		a.gateways = append(a.gateways, g)
	} else {
		a.gateways[index] = g
	}
	err := a.validateGateways()
	a.gateways = previous
	if err != nil {
		d.errorText = locales.ErrorText(err)
		return
	}
	changes, err := a.vault.Prepare(items, deleted)
	if err != nil {
		d.errorText = locales.ErrorText(err)
		return
	}
	a.commitCatalogSecretsAsync(changes, func() {
		if index < 0 {
			a.gateways = append(a.gateways, g)
		} else {
			a.gateways[index] = g
		}
	}, func(err error) {
		if err != nil {
			d.errorText = locales.ErrorText(err)
			return
		}
		a.cancelGatewayTest()
		a.gatewayUI.results = nil
		a.selectGateway(g.ID)
		d.password = ""
		d.wg.private, d.wg.psk = "", ""
		d.modal.Close()
	})
}

func (a *app) deleteGateway(d *gatewayDraft) {
	if refs := a.gatewayReferences(d.value.ID); len(refs) > 0 {
		d.errorText = locales.Deferred(locales.MessageTheGatewayIsStillReferenced)
		return
	}
	changes, err := a.vault.Prepare(nil, []string{gatewayCredentialKey(d.value.ID), wgCredentialKey(d.value.ID)})
	if err != nil {
		d.errorText = locales.ErrorText(err)
		return
	}
	a.commitCatalogSecretsAsync(changes, func() {
		a.gateways = slices.DeleteFunc(a.gateways, func(g gatewayConfig) bool { return g.ID == d.value.ID })
	}, func(err error) {
		if err != nil {
			d.errorText = locales.ErrorText(err)
			return
		}
		if a.gatewayUI.testGateway == d.value.ID {
			a.cancelGatewayTest()
		}
		delete(a.gatewayUI.results, d.value.ID)
		d.modal.Close()
	})
}
