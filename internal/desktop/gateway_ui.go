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
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
	"sshdesk/internal/vault"
)

var gatewayKinds = []string{"ssh", "socks5"}

type gatewayDraft struct {
	initial                        [8]string
	editing                        bool
	value                          gatewayConfig
	port, password, errorText      string
	auth                           int // 0 无认证，1 用户名密码（SOCKS5）
	passwordChanged, clearPassword bool
	deleting                       bool
	scroll                         ui.ScrollState
	modal                          uikit.ModalController
}

func (d *gatewayDraft) inputValues() [8]string {
	values := [8]string{d.value.Name, d.value.Kind, "", d.value.SSHProfileID, "", "", d.port, strconv.Itoa(d.auth)}
	if d.value.Kind == "socks5" {
		values[2] = d.value.UpstreamID
	}
	if d.value.SOCKS != nil {
		values[4] = d.value.SOCKS.Address
		if d.auth == 1 {
			values[5] = d.value.SOCKS.Username
		}
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

var gatewayRefGroups = [...]string{"应用设置", "主机与站点", "下游网关", "运行中"}

type gatewayRef struct {
	kind          gatewayRefKind
	label, detail string
	host          hostID
	gateway       string
}

// hostGatewayOptions disables the gateways whose route would come back to
// the edited host, such as one built on the host itself.
func (a *app) hostGatewayOptions(d *hostDraft) []uikit.Option[string] {
	options := []uikit.Option[string]{{Value: "", Label: "跟随应用默认"}, {Value: directGateway, Label: "直连"}}
	catalog := a.gatewayCatalog()
	for _, g := range a.gateways {
		option := uikit.Option[string]{Value: g.ID, Label: g.Name, Detail: gatewayKindLabel(g.Kind)}
		if d.editing {
			var loop *gatewayLoop
			if _, err := catalog.hostRoute(a.hosts[d.id].session.ID, g.ID, nil); errors.As(err, &loop) {
				option.Detail, option.Disabled = "会经过本主机", true
			}
		}
		options = append(options, option)
	}
	return options
}

func (a *app) gatewayName(id string) string {
	if id == "" || id == directGateway {
		return "直连"
	}
	if g, ok := a.gatewayByID(id); ok {
		return g.Name
	}
	return "网关已不存在"
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
	if kind == "ssh" {
		return "SSH"
	}
	return "SOCKS5"
}

func gatewayKindLook(kind string) (*ui.SVG, color.NRGBA) {
	choices := theme.Current().Color.AccentChoices
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
		return "SSH 主机"
	}
	return strings.ToUpper(h.protocol) + " 站点"
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
		refs = append(refs, gatewayRef{kind: gatewayRefDefault, label: "应用默认出口", detail: fmt.Sprintf("跟随默认的 %d 个会话", a.gatewayFollowers())})
	}
	for index, h := range a.hosts {
		if h.session.Gateway == id {
			refs = append(refs, gatewayRef{kind: gatewayRefHost, label: h.name, detail: hostKindLabel(h), host: hostID(index)})
		}
	}
	for _, g := range a.gateways {
		if g.UpstreamID == id {
			refs = append(refs, gatewayRef{kind: gatewayRefGateway, label: g.Name, detail: "以本网关为上游", gateway: g.ID})
		}
	}
	return append(refs, a.gatewayRuntimeReferences(id)...)
}

func (a *app) gatewayRuntimeReferences(id string) []gatewayRef {
	refs := []gatewayRef{}
	runtime := func(label string) {
		refs = append(refs, gatewayRef{kind: gatewayRefRuntime, label: label, detail: "关闭后解除"})
	}
	if a.gatewayPool.inUse(id) {
		runtime("正在使用的网关连接")
	}
	for _, group := range a.runtimeSSHGroups() {
		if slices.ContainsFunc(group.plan.route, func(h gatewayHop) bool { return h.ID == id }) {
			runtime("终端、传输或可恢复标签")
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
			runtime("文件会话或传输任务")
			break
		}
	}
	return refs
}

func (a *app) gatewayLive(id string) bool { return len(a.gatewayRuntimeReferences(id)) > 0 }

func (a *app) setDefaultGateway(id string) {
	a.commitCatalogMoveAsync(func() { a.defaultGateway = id }, func(err error) { a.persistenceNotice("保存默认网关失败", err) })
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
	a.gatewayUI.draft.initial = a.gatewayUI.draft.inputValues()
	a.gatewayUI.draft.editing = slices.ContainsFunc(a.gateways, func(value gatewayConfig) bool { return value.ID == g.ID })
	a.gatewayUI.open = true
}

func (a *app) gatewayMenu(g gatewayConfig) []uikit.MenuEntry {
	return []uikit.MenuEntry{
		{ID: "test", Label: "测试连接", Icon: uikit.IconPlay, Disabled: a.gatewayTestRunning(g.ID), OnChoose: func() { a.startGatewayTest(g.ID) }},
		{ID: "edit", Label: "编辑网关…", Icon: uikit.IconEdit, OnChoose: func() { a.startGatewayDraft(g, false) }},
		{ID: "default", Label: "设为应用默认出口", Icon: uikit.IconRoute, Disabled: a.defaultGateway == g.ID, OnChoose: func() { a.setDefaultGateway(g.ID) }},
		{ID: "before-delete", Separator: true},
		{ID: "delete", Label: "删除网关…", Icon: uikit.IconTrash, Danger: true, OnChoose: func() { a.startGatewayDraft(g, true) }},
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
			uikit.Text(c, "应用默认出口", uikit.TextProps{Style: uikit.TextBodyStrong}).Shrink(0)
			selected := a.defaultGateway
			glyph := t.C.IconButton.IconSizeXs
			options := []uikit.Option[string]{{Value: "", Label: "直连", Detail: "不经过网关", Leading: func() {
				gatewayTile(c, uikit.IconSVG(uikit.IconLaptop), color.NRGBA{}, t.Space.N18, glyph, t.Radius.Chip)
			}}}
			for _, g := range a.gateways {
				options = append(options, uikit.Option[string]{Value: g.ID, Label: g.Name, Detail: gatewayKindLabel(g.Kind), Leading: func() {
					gatewayKindTile(c, g.Kind, t.Space.N18, glyph, t.Radius.Chip)
				}})
			}
			input := uikit.Select(c, &selected, options, uikit.SelectProps{Label: "应用默认出口", Disabled: len(a.gateways) == 0}).
				Width(t.Size.Sider).Shrink(0)
			if input.Changed() {
				a.setDefaultGateway(selected)
			}
			ui.Row(c).MinWidth(0).Shrink(1).Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
				chain := a.gatewayChain(a.defaultGateway)
				if len(chain) == 0 {
					uikit.Text(c, "连接直接从本机发出", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).SingleLine()
					return
				}
				uikit.Text(c, "本机", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).Shrink(0)
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
			stat("跟随默认", strconv.Itoa(a.gatewayFollowers()), uikit.ToneNeutral)
			stat("网关", strconv.Itoa(len(a.gateways)), uikit.ToneNeutral)
			liveTone := uikit.ToneNeutral
			if live > 0 {
				liveTone = uikit.ToneSuccess
			}
			stat("使用中", strconv.Itoa(live), liveTone)
		})
	})
}

func (a *app) gatewayEmptyState(c *ui.Context) {
	t := theme.Current()
	uikit.Card(c).Key("gateway-empty").Grow(1).FillWidth().Center().Role(ui.RoleStatus).Label("还没有连接网关").Children(func() {
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
			uikit.Text(c, "还没有连接网关", uikit.TextProps{Style: uikit.TextTitle})
			uikit.Text(c, "网关决定连接从哪里出发。添加 SSH 跳板或 SOCKS5 代理后，可以在主机和文件站点中选择经由哪个网关，也可以设为应用默认出口。",
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
				choice("ssh", "添加 SSH 网关", "复用已保存的 SSH 主机作为跳板，可继续串联上游")
				choice("socks5", "添加 SOCKS5 代理", "填写代理地址，支持无认证或用户名密码认证")
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
	result := uikit.Sider(c, "网关", uikit.SiderProps{
		Label: "网关列表", AddLabel: "新增网关", AddIcon: plusIcon, FooterIcon: routeIcon,
		Footer: fmt.Sprintf("%d 个网关 · %d 个使用中", len(a.gateways), live),
		Query:  &page.query,
		Search: uikit.SearchProps{Label: "搜索网关", Placeholder: "搜索网关、地址", Icon: searchIcon,
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
					title := "SSH 网关"
					if kind == "socks5" {
						title = "SOCKS5 代理"
					}
					group := uikit.SiderGroup(c, uikit.SiderGroupProps{
						Key: "gateway-group-" + kind, Title: title, Count: len(items), Open: &open, Chevron: chevronIcon,
						Motion: uikit.MotionOverride(a.reduceMotion(c)), DeferPresentation: true,
					})
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
					uikit.EmptyMessage(c, "没有匹配的网关", uikit.EmptyCatalogSearch)
				}
			})
		})
		uikit.Scrollbars(c, viewport, content, &state.listScroll, uikit.ScrollbarProps{Label: "网关列表滚动条"})
	})
}

func (a *app) gatewayRow(c *ui.Context, g gatewayConfig) {
	t := theme.Current()
	p := t.C.SiderItem
	selected := a.gatewayUI.selected == g.ID
	address := a.gatewayAddress(g)
	row := ui.ButtonBase(c).Key(g.ID).Height(t.C.Catalog.RowHeight).Shrink(0).FillWidth().
		Padding(0, p.PaddingRight, 0, p.PaddingLeft).Gap(p.Gap).Radius(p.Radius).Role(ui.RoleListItem).
		Label(g.Name + "，" + address).Selected(selected).FocusRing(false).TextColor(uikit.Color(p.NameColor))
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
	uikit.ContextMenuOn(c, row, a.gatewayMenu(g))
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
					uikit.Badge(c, "默认", uikit.BadgeProps{Tone: uikit.ToneAccent}).PassThrough()
				}
				if hops > 1 {
					ui.Row(c).Gap(t.Space.N3).AlignItems(ui.Center).PassThrough().Children(func() {
						uikit.Icon(c, routeIcon, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary}).PassThrough()
						uikit.Text(c, fmt.Sprintf("%d 跳", hops), uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).PassThrough()
					})
				}
			})
		}
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
					uikit.Badge(c, "默认出口", uikit.BadgeProps{Tone: uikit.ToneAccent})
				}
				if live {
					uikit.Badge(c, "使用中", uikit.BadgeProps{Tone: uikit.ToneSuccess, Dot: true})
				}
			})
			uikit.Text(c, a.gatewayAddress(g), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary, Mono: true}).
				SingleLine().Ellipsis("…").Selectable()
		})
		ui.Row(c).Shrink(0).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
			uikit.Button(c, "测试连接", uikit.ButtonProps{Icon: uikit.IconPlay, Disabled: a.gatewayTestRunning(g.ID)}).
				OnClick(func() { a.startGatewayTest(g.ID) })
			uikit.Button(c, "编辑", uikit.ButtonProps{Icon: uikit.IconEdit}).OnClick(func() { a.startGatewayDraft(g, false) })
			more := uikit.IconButton(c, moreIcon, uikit.IconButtonProps{Key: "gateway-more", Label: "更多网关操作", Variant: uikit.IconFilled})
			uikit.MenuOn(c, more, a.gatewayMenu(g))
		})
	})
	viewport := ui.Box(c).Key("gateway-detail-viewport").Grow(1).MinHeight(0).FillWidth().Clip()
	viewport.Children(func() {
		content := ui.Scroll(c.Key("gateway-detail-scroll")).TrackScroll(&state.scroll).Fill().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(t.Space.N18, t.Space.N20, t.Space.N24, t.Space.N20).Gap(t.Space.N24).Children(func() {
				ui.Column(c).Key("route").FillWidth().Children(func() {
					gatewaySectionTitle(c, "连接路径", "终端、文件传输和 MCP 经此网关时的出站顺序", nil)
					a.gatewayRoute(c, g)
					a.gatewayTestStrip(c, g)
				})
				ui.Column(c).Key("facts").FillWidth().Children(func() {
					gatewaySectionTitle(c, "配置", "", nil)
					a.gatewayFacts(c, g)
				})
				ui.Column(c).Key("references").FillWidth().Children(func() {
					note := ""
					if len(refs) > 0 {
						note = "删除网关前需先解除全部引用"
					}
					gatewaySectionTitle(c, "引用", note, func() { uikit.Count(c, len(refs), uikit.CountProps{}) })
					a.gatewayReferenceGroups(c, refs)
				})
			})
		})
		uikit.Scrollbars(c, viewport, content, &state.scroll, uikit.ScrollbarProps{Label: "网关详情滚动条"})
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
		gatewayRouteEnd(c, uikit.IconSVG(uikit.IconLaptop), "起点", "本机", "本地网络", false)
		for index, hop := range chain {
			label := "直连"
			if index > 0 {
				label = "经代理"
				if chain[index-1].Kind == "ssh" {
					label = "SSH 通道"
				}
			}
			// Each hop lights once the test has connected through it.
			link, mark := test.link(hop.ID)
			gatewayRouteLink(c.Key("link-"+hop.ID), label, link, reduced)
			a.gatewayRouteNode(c, hop, hop.ID == g.ID, mark)
		}
		label := "经代理"
		if g.Kind == "ssh" {
			label = "SSH 通道"
		}
		// The test stops at this gateway, so the last leg stays an outline.
		gatewayRouteLink(c.Key("link-target"), label, gatewayLinkTarget, reduced)
		gatewayRouteEnd(c, serverIcon, "终点", "目标主机", "由主机或站点决定", true)
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
		node = ui.ButtonBase(c).Key(hop.ID).Column().AlignItems(ui.Stretch).Label("查看网关 " + hop.Name).FocusRing(false)
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
				uikit.Spinner(c, uikit.SpinnerProps{Size: uikit.SizeXs, Tone: uikit.ToneAccent, Label: "正在测试",
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
			uikit.Badge(c, "本网关", uikit.BadgeProps{Tone: uikit.ToneOnAccent}).Absolute().
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
		uikit.Button(c, "重新测试", uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm, Icon: uikit.IconRefresh}).
			Shrink(0).OnClick(func() { a.startGatewayTest(g.ID) })
	}
	chain := a.gatewayChain(g.ID)
	// The test's own route names its hops; the worker keeps its indexes within it.
	test := a.gatewayTestHops(g.ID)
	if a.gatewayTestRunning(g.ID) {
		hop := test.route[test.connecting].Name
		message := "正在连接「" + hop + "」…"
		if len(test.route) > 1 {
			message = fmt.Sprintf("正在连接第 %d/%d 跳「%s」…", test.connecting+1, len(test.route), hop)
		}
		strip(p.InfoBg, message).Children(func() {
			uikit.Spinner(c, uikit.SpinnerProps{Size: uikit.SizeXs, Tone: uikit.ToneAccent, Label: "正在测试",
				Motion: uikit.MotionOverride(a.reduceMotion(c))})
			ui.Text(c, message).FontSize(t.Font.Size.Small).TextColor(uikit.Color(p.InfoText)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			uikit.Button(c, "取消", uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm}).Shrink(0).OnClick(a.cancelGatewayTest)
		})
		return
	}
	record, tested := a.gatewayUI.results[g.ID]
	result := record.result
	if !tested {
		scope := "检查 SOCKS5 协商与认证方式"
		if g.Kind == "ssh" {
			scope = "检查 SSH 握手、主机信任与认证"
		}
		via := "直接"
		if len(chain) > 1 {
			via = "经上游"
		}
		ui.Row(c).Key("gateway-test-hint").FillWidth().Margin(t.Space.N10, 0, 0, 0).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
			uikit.Icon(c, infoIcon, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneTertiary})
			uikit.Text(c, "测试会"+via+"连接本网关，"+scope+"；不访问目标主机。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary}).
				Grow(1).MinWidth(0)
		})
		return
	}
	if result.Err == nil {
		strip(t.Color.Success.Subtle, result.Summary).Children(func() {
			uikit.Icon(c, sessionCheckIcon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSuccess})
			uikit.Text(c, result.Summary, uikit.TextProps{Style: uikit.TextBodyStrong, Tone: uikit.ToneSuccess}).Shrink(0)
			uikit.Text(c, result.Detail, uikit.TextProps{Style: uikit.TextSmall}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			retry()
		})
		return
	}
	// Name the hop that failed: 「test」连接失败 rather than the gateway under test.
	summary := result.Summary
	if test.failed >= 0 {
		summary = "「" + test.route[test.failed].Name + "」" + strings.TrimPrefix(summary, "网关")
	}
	strip(p.DangerBg, summary).AlignItems(ui.Start).Children(func() {
		uikit.Icon(c, sessionAlertIcon, uikit.IconProps{Size: uikit.SizeSm}).TextColor(uikit.Color(p.DangerIcon)).Margin(t.Space.N2, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
			uikit.Text(c, summary, uikit.TextProps{Style: uikit.TextBodyStrong, Tone: uikit.ToneDanger})
			uikit.Text(c, result.Detail, uikit.TextProps{Style: uikit.TextFootnote, Mono: true}).Selectable()
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
	previous := func() { uikit.Text(c, "直连", uikit.TextProps{}).Shrink(0) }
	if chain := a.gatewayChain(g.ID); len(chain) > 1 {
		u := chain[len(chain)-2]
		previous = chip("upstream", func() { gatewayKindTile(c, u.Kind, tile, glyph, t.Radius.Chip) }, u.Name,
			func() { a.selectGateway(u.ID) })
	}
	upstream := fact{label: "上游网关", render: func() {
		previous()
		if g.Kind == "ssh" {
			uikit.Text(c, "跟随源主机", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…").MinWidth(0)
		}
	}}
	var facts []fact
	note := "目标主机名交由代理解析；FTP / FTPS 经网关时仅支持被动模式。"
	var editHost func()
	if g.Kind == "ssh" {
		h, id, _ := a.gatewayHost(g)
		auth := map[string]string{"password": "密码", "key": "私钥", "agent": "SSH Agent"}[h.session.Auth]
		if h.session.Auth == "password" && h.session.PasswordSaved {
			auth += " · 已保存"
		}
		keepalive := "关闭"
		if h.session.SSH != nil && h.session.SSH.KeepAlive > 0 {
			keepalive = strconv.Itoa(h.session.SSH.KeepAlive) + " 秒"
		}
		editHost = func() { a.openGatewayReference(gatewayRef{kind: gatewayRefHost, host: id}) }
		facts = []fact{
			{label: "源主机", render: chip("source", func() { gatewayTile(c, terminalIcon, color.NRGBA{}, tile, glyph, t.Radius.Chip) }, h.name, editHost)},
			{label: "地址", value: a.gatewayAddress(g), mono: true},
			{label: "认证", value: auth},
			{label: "Keepalive", value: keepalive},
			upstream,
			{label: "连接复用", value: "多个目标共享同一条 SSH 连接"},
		}
		note = "地址、认证、Keepalive 和上游均取自 SSH 主机「" + h.name + "」及其连接网关，修改请编辑该主机。网关不执行其初始命令。"
	} else {
		auth, user, password := "无认证", "—", "—"
		if g.SOCKS.Username != "" {
			auth, user, password = "用户名密码", g.SOCKS.Username, "连接时输入"
			if g.SOCKS.PasswordSaved {
				password = "已保存在 Vault"
			}
		}
		facts = []fact{
			{label: "地址", value: a.gatewayEndpoint(g), mono: true},
			{label: "认证", value: auth},
			{label: "用户名", value: user, mono: g.SOCKS.Username != ""},
			{label: "密码", value: password},
			upstream,
			{label: "连接方式", value: "每个目标单独建立连接"},
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
			uikit.Button(c, "编辑主机", uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm}).Shrink(0).OnClick(editHost)
		}
	})
}

func (a *app) gatewayReferenceGroups(c *ui.Context, refs []gatewayRef) {
	t := theme.Current()
	if len(refs) == 0 {
		ui.Row(c).FillWidth().Padding(t.Space.N12, t.Space.N14).Gap(t.Space.N8).AlignItems(ui.Center).Radius(t.Radius.Lift).
			Border(t.Size.Divider, uikit.Color(t.Color.Border.Subtle)).Children(func() {
			uikit.Icon(c, sessionCheckIcon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSuccess})
			uikit.Text(c, "没有会话、网关或运行中的连接引用此网关，可以直接删除。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		})
		return
	}
	groups := [][]gatewayRef{}
	for kind := range gatewayRefGroups {
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
							uikit.Text(c, gatewayRefGroups[group[0].kind], uikit.TextProps{Style: uikit.TextLabel, Tone: uikit.ToneSecondary})
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
		label := "编辑 " + ref.label
		if ref.kind == gatewayRefGateway {
			label = "查看网关 " + ref.label
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
		a.recordGatewayTest(id, gatewayTestRecord{result: connectionTestResult{Summary: "无法测试网关", Detail: err.Error(), Err: err}, failed: -1})
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
		c.Announce(result.Summary)
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
		state.draft = nil
	}
}

func (a *app) gatewayDeleteDialog(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	refs := a.gatewayReferences(d.value.ID)
	uikit.Modal(c.Key(d.value.ID).Key("delete"), &a.gatewayUI.open, uikit.ModalProps{
		Controller: &d.modal, Title: "删除网关「" + d.value.Name + "」？", Width: t.C.Dialog.HostWidth,
		Compact: true, Disabled: a.catalogSaving, Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		if len(refs) > 0 {
			uikit.Banner(c, fmt.Sprintf("仍有 %d 项引用。请先将它们改为其他网关或直连，再删除此网关。", len(refs)),
				uikit.BannerProps{Tone: uikit.ToneWarning, Icon: sessionAlertIcon})
			ui.Column(c).FillWidth().Padding(t.Space.N4).Radius(t.Radius.Lift).Border(t.Size.Divider, uikit.Color(t.Color.Border.Subtle)).Children(func() {
				for index, ref := range refs {
					ref.detail = gatewayRefGroups[ref.kind]
					a.gatewayReferenceRow(c.Key(index), ref, false)
				}
			})
		} else {
			message := "删除后不可恢复。"
			if d.value.SOCKS != nil && d.value.SOCKS.PasswordSaved {
				message += "已保存的 SOCKS5 密码会一并从 Vault 移除。"
			}
			uikit.Text(c, message, uikit.TextProps{Tone: uikit.ToneSecondary})
		}
		if d.errorText != "" {
			uikit.Banner(c, d.errorText, uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
		}
		ui.Row(c).FillWidth().Gap(t.Space.N8).Justify(ui.End).Children(func() {
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			uikit.Button(c, "删除", uikit.ButtonProps{Variant: uikit.ButtonDanger, Disabled: len(refs) > 0}).OnClick(func() { a.deleteGateway(d) })
		})
	})
}

func (a *app) gatewayEditDialog(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	p := t.C.Modal
	title := "新增网关"
	if d.editing {
		title = "编辑网关"
	}
	dirty := d.inputValues() != d.initial || d.passwordChanged || d.clearPassword
	result := uikit.Modal(c.Key(d.value.ID), &a.gatewayUI.open, uikit.ModalProps{
		Controller: &d.modal, Title: title, Dirty: dirty, Disabled: a.catalogSaving,
		CloseKey: "gateway-close", CloseLabel: "关闭网关设置", Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Header: func(uikit.ModalParts) {
			ui.Row(c).FillWidth().Shrink(0).Padding(p.HeaderPaddingTop, p.HeaderPaddingRight, p.HeaderPaddingBottom, p.HeaderPaddingLeft).
				Gap(p.HeaderGap).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Key("gateway-header-" + d.value.Kind).Shrink(0).
					Transition(ui.ElementTransition{Duration: p.Fade, Enter: &ui.Motion{}, Exit: &ui.Motion{}}).Children(func() {
					gatewayKindTile(c, d.value.Kind, t.C.IconTile.SizeLg, t.Size.Icon, t.C.IconTile.RadiusLg)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).PaddingEnd(t.C.IconButton.Size).Children(func() {
					uikit.Text(c, title, uikit.TextProps{Style: uikit.TextTitle}).SingleLine()
					subtitle := "保存后可在主机、文件站点和应用默认出口中选择"
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
				uikit.Button(c, "取消", uikit.ButtonProps{Variant: uikit.ButtonPlain}).Padding(0, p.ButtonPaddingX).OnClick(parts.RequestDismiss)
				uikit.Button(c, "保存", uikit.ButtonProps{Variant: uikit.ButtonPrimary, Kbd: "↩"}).Padding(0, p.ButtonPaddingX).Shrink(0).
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
					{Label: "SSH 网关", IconSVG: terminalIcon, IconTone: uikit.ToneAccent, Detail: "复用已保存的 SSH 主机"},
					{Label: "SOCKS5 代理", IconSVG: globeIcon, IconTone: uikit.ToneTertiary, Detail: "经代理服务器转发连接"},
				}, uikit.SegmentedProps{Label: "网关类型", Fill: true, Cards: true})
			})
			if gatewayKinds[kind] != d.value.Kind {
				d.value.Kind, d.errorText = gatewayKinds[kind], ""
			}
			uikit.FormSection(c, "", func() { a.gatewayDraftFields(c, d) }).
				Padding(t.Space.N8, p.BodyPaddingX, t.Space.N16, p.BodyPaddingX)
		})
		uikit.Scrollbars(c, parts.Panel, scroller, &d.scroll, uikit.ScrollbarProps{Label: "网关表单滚动条"})
	})
	if result.Submit {
		a.saveGateway(d)
	}
}

func (a *app) gatewayDraftFields(c *ui.Context, d *gatewayDraft) {
	t := theme.Current()
	uikit.FormRow(c, "名称", uikit.FormRowProps{Key: "name"}, func() ui.Element {
		return uikit.Input(c, &d.value.Name, uikit.InputProps{Label: "网关名称", Placeholder: "例如 bastion-hk"})
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
				option.Detail, option.Disabled = "会形成环路", true
			}
			options = append(options, option)
		}
		uikit.FormRow(c, "SSH 主机", uikit.FormRowProps{Key: "ssh-host", Help: "地址、认证与 Keepalive 取自该主机；不执行其初始命令。"}, func() ui.Element {
			return uikit.Select(c, &d.value.SSHProfileID, options, uikit.SelectProps{Label: "SSH 主机", Placeholder: "选择已保存的 SSH 主机"})
		})
		uikit.FormRow(c, "上游网关", uikit.FormRowProps{Key: "upstream", Help: "SSH 网关沿用源主机的「连接网关」，与直接连接该主机时的路径一致；请在主机设置中修改。"}, func() ui.Element {
			return ui.Row(c).FillWidth().Height(t.C.Input.Height).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
				source, ok := catalog.sshHost(d.value.SSHProfileID)
				if !ok {
					uikit.Text(c, "选择 SSH 主机后确定", uikit.TextProps{Tone: uikit.ToneTertiary})
					return
				}
				selection := "跟随应用默认"
				switch source.session.Gateway {
				case "":
				case directGateway:
					selection = "直连"
				default:
					selection = a.gatewayName(source.session.Gateway)
				}
				uikit.Text(c, "跟随「"+source.name+"」", uikit.TextProps{}).Shrink(0)
				uikit.Text(c, "· "+selection, uikit.TextProps{Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…").MinWidth(0)
			})
		})
	} else {
		if d.value.SOCKS == nil {
			d.value.SOCKS = &socksGateway{Port: 1080}
		}
		uikit.FormRow(c, "代理地址", uikit.FormRowProps{Key: "socks-address"}, func() ui.Element {
			return ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
				uikit.Input(c, &d.value.SOCKS.Address, uikit.InputProps{Label: "SOCKS5 地址", Placeholder: "主机名或 IP", Mono: true}).Grow(1)
				uikit.PortInput(c, &d.port, uikit.InputProps{Label: "SOCKS5 端口", Width: t.C.Dialog.PortWidth}).Shrink(0)
			})
		})
		uikit.FormRow(c, "认证", uikit.FormRowProps{Key: "socks-auth", Align: uikit.RowAlignCenter}, func() ui.Element {
			return uikit.Segmented(c, &d.auth, uikit.SegmentItems([]string{"无认证", "用户名密码"}), uikit.SegmentedProps{Label: "SOCKS5 认证", Size: uikit.SizeSm})
		})
		if d.auth == 1 {
			uikit.FormRow(c, "用户名", uikit.FormRowProps{Key: "socks-user"}, func() ui.Element {
				return uikit.Input(c, &d.value.SOCKS.Username, uikit.InputProps{Label: "SOCKS5 用户名"})
			})
			placeholder, help := "留空则连接时输入", "保存的密码经 Vault 加密存储。"
			if d.value.SOCKS.PasswordSaved {
				placeholder, help = "已保存在 Vault · 留空保持不变", ""
			}
			uikit.FormRow(c, "密码", uikit.FormRowProps{Key: "socks-password", Help: help}, func() ui.Element {
				input := uikit.PasswordInput(c, &d.password, uikit.InputProps{Label: "SOCKS5 密码", Placeholder: placeholder}).Input
				if input.Changed() {
					d.passwordChanged = true
				}
				return input
			})
			if d.value.SOCKS.PasswordSaved {
				uikit.FormRow(c, "已保存密码", uikit.FormRowProps{Key: "socks-clear", Inline: true}, func() ui.Element {
					return uikit.Checkbox(c, &d.clearPassword, "清除已保存的密码")
				})
			}
		}
		options := []uikit.Option[string]{{Value: "", Label: "直连", Detail: "本机网络"}}
		for _, g := range a.gateways {
			if g.ID == d.value.ID {
				continue
			}
			option := uikit.Option[string]{Value: g.ID, Label: g.Name, Detail: gatewayKindLabel(g.Kind)}
			if loops(func(candidate *gatewayConfig) { candidate.UpstreamID = g.ID }) {
				option.Detail, option.Disabled = "会形成环路", true
			}
			options = append(options, option)
		}
		uikit.FormRow(c, "上游网关", uikit.FormRowProps{Key: "upstream", Help: "代理服务器经由哪个出口连接。"}, func() ui.Element {
			return uikit.Select(c, &d.value.UpstreamID, options, uikit.SelectProps{Label: "上游网关"})
		})
	}
	uikit.FormRow(c, "路径预览", uikit.FormRowProps{Key: "route-preview"}, func() ui.Element {
		return a.gatewayRoutePreview(c, d)
	})
	if d.errorText != "" {
		uikit.Banner(c, d.errorText, uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
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
		name = "新网关"
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
		chip("local", func() { gatewayTile(c, uikit.IconSVG(uikit.IconLaptop), color.NRGBA{}, box, glyph, t.Radius.Chip) }, "本机", false, false)
		for _, hop := range chain {
			arrow()
			chip(hop.ID, func() { gatewayKindTile(c, hop.Kind, box, glyph, t.Radius.Chip) }, hop.Name, false, false)
		}
		arrow()
		chip("self", func() { gatewayKindTile(c, d.value.Kind, box, glyph, t.Radius.Chip) }, name, true, false)
		arrow()
		chip("target", nil, "目标主机", false, true)
	})
}

func (a *app) saveGateway(d *gatewayDraft) {
	if a.catalogSaving {
		return
	}
	g := d.value
	g.Name = strings.TrimSpace(g.Name)
	items := []vault.PlainItem{}
	deleted := []string{}
	if g.Kind == "ssh" {
		g.SOCKS, g.UpstreamID = nil, ""
	} else {
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
				d.errorText = "SOCKS5 密码不能超过 255 字节"
				return
			}
			options.PasswordSaved = true
			options.PasswordRevision = newSessionID()
			data, err := json.Marshal(sessionCredential{Revision: options.PasswordRevision, Secret: d.password})
			if err != nil {
				d.errorText = err.Error()
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
		d.errorText = err.Error()
		return
	}
	changes, err := a.vault.Prepare(items, deleted)
	if err != nil {
		d.errorText = err.Error()
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
			d.errorText = err.Error()
			return
		}
		a.cancelGatewayTest()
		a.gatewayUI.results = nil
		a.selectGateway(g.ID)
		d.password = ""
		d.modal.Close()
	})
}

func (a *app) deleteGateway(d *gatewayDraft) {
	if refs := a.gatewayReferences(d.value.ID); len(refs) > 0 {
		d.errorText = "网关仍被引用"
		return
	}
	changes, err := a.vault.Prepare(nil, []string{gatewayCredentialKey(d.value.ID)})
	if err != nil {
		d.errorText = err.Error()
		return
	}
	a.commitCatalogSecretsAsync(changes, func() {
		a.gateways = slices.DeleteFunc(a.gateways, func(g gatewayConfig) bool { return g.ID == d.value.ID })
	}, func(err error) {
		if err != nil {
			d.errorText = err.Error()
			return
		}
		if a.gatewayUI.testGateway == d.value.ID {
			a.cancelGatewayTest()
		}
		delete(a.gatewayUI.results, d.value.ID)
		d.modal.Close()
	})
}
