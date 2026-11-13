package desktop

import (
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/store"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

type mcpKeyGrant struct {
	EndpointDigest        string
	ConnectionID          string
	Name                  string
	Warning               locales.Text
	Protocol, Endpoint    string // 原始协议名（ssh/sftp/ftp/ftps）与 user@host:port
	SupportsFiles         bool
	SupportsSSH           bool
	AllowRead, AllowWrite bool
	AllowExec             bool
	Sources               []store.MCPGrant
}

type mcpSecretKey struct {
	ID, Name, Prefix         string
	State                    string // "active", "disabled", "revoked"
	GrantsSummary            string
	PolicyHint               string
	UploadDirs, DownloadDirs []string
	LastUsed, CreatedAt      string
	Grants                   []mcpKeyGrant
}

type mcpFileRule struct {
	Side          string // "remote" or "local"
	Kind          string // "dir" or "file"
	Path          string
	IncludeSubdir bool
	DeniedActions map[string]bool
}

type mcpCommandRule struct {
	Mode       string // "command", "exact", "regex", or "contains"
	Pattern    string
	IgnoreCase bool // command mode only
}

type mcpPolicy struct {
	KeyIDs         []string
	ID             string
	Version        int
	Name           string
	Description    string
	Enabled        bool
	Category       string // "files" or "ssh"
	AllConnections bool
	Connections    []string
	ConnSummary    string
	ConnSub        string
	AllKeys        bool
	KeyNames       []string
	RulePrimary    string
	RuleSub        string
	HitsSummary    string
	FileRules      []mcpFileRule
	CmdRules       []mcpCommandRule
}

type mcpAuditFileItem struct {
	Path      string
	PolicyTag string
	Size      string
	State     string // "ok", "intercepted", "failed", "skipped", "pending"
}

type mcpAuditTask struct {
	ID            string
	Title         string
	Subtitle      string
	Transferred   string
	Total         string
	Fraction      float32
	Speed         string
	Paused        bool
	CancelRequest bool
	CleanupOnly   bool
	WaitingPrompt string
}

type mcpPolicyHitSnapshot struct {
	PolicyID      string
	PolicyName    string
	Version       int
	Category      string
	RuleSummary   string
	Action        string
	ForbiddenText string
	HitCountText  string
	MatchSaved    bool
	MatchText     string
	MatchContext  string
	MatchVia      string
	MatchFallback string
}

type mcpAuditItem struct {
	Action            string
	FileTotal         int
	FileCounts        map[string]int
	ConnectionID      string
	ID                string
	Time, Day         string
	KeyID, KeyName    string
	ConnName          string
	ConnProtocol      string
	ConnHost          string
	ConnPort          string
	ConnUser          string
	ActionType        string
	Summary           string
	Status            uikit.Status
	Duration          string
	StartTime         string
	EndTime           string
	IsCommand         bool
	CommandText       string
	ExitCodeText      string
	OutputStatus      string
	RequestJSON       string
	ResponseJSON      string
	Stdout            string
	Stderr            string
	ResponseSaved     bool
	ResponseTruncated bool
	FileBodyOmitted   bool
	PayloadSaved      bool
	DetailLoaded      bool
	LocalPath         string
	RemotePath        string
	TransferStat      string
	PolicyHits        []mcpPolicyHitSnapshot
	Task              *mcpAuditTask
	Timeline          []uikit.TimelineItem
	Files             []mcpAuditFileItem
}

type mcpKeyModalDraft struct {
	modal          uikit.ModalController
	Open           bool
	EditingID      string
	EditingMeta    locales.Text // 编辑时的副标题：密钥前缀与创建时间
	OneTimeView    bool
	GeneratedKey   string
	SavedConfirmed bool
	Name           string
	NameError      locales.Text
	Grants         []mcpKeyGrant
	UploadDirs     []string
	DownloadDirs   []string
	Scroll         ui.ScrollState
	// 连接较多时的搜索与「仅看已授权」。GrantedSnapshot 记录开启筛选时已授权的
	// 连接，取消勾选后该行仍保留，直到重新开启筛选。
	Query           string
	OnlyGranted     bool
	GrantedSnapshot map[string]bool
}

type mcpKeyConfirmDialog struct {
	modal  uikit.ModalController
	Open   bool
	KeyID  string
	Name   string
	Action string // "disable" or "revoke"
}

type mcpPolicyDeleteDialog struct {
	modal    uikit.ModalController
	Open     bool
	PolicyID string
	Name     string
}

type mcpPolicyDraft struct {
	modal        uikit.ModalController
	EditingID    string
	Version      int
	Name         string
	NameError    locales.Text
	SaveError    locales.Text
	Description  string
	Enabled      bool
	CategorySeg  int // 0 = 命令策略, 1 = 文件策略
	Connections  []string
	SelectedKeys []string
	FileRules    []mcpFileRule
	CmdRules     []mcpCommandRule
	Scroll       ui.ScrollState
}

type mcpUIState struct {
	auditResponseTab          int
	detail                    *mcpAuditItem
	keyPage                   int
	auditPolicyID             string
	query                     string
	refreshed                 time.Time
	auditTotal, auditFiltered int
	running                   bool
	host                      string
	port                      string
	uptime                    string
	todayCalls                int
	todayIntercepts           int
	activeTab                 int // 0 = Secret Keys, 1 = 拦截策略, 2 = 审计记录
	keysScroll                ui.ScrollState
	policiesScroll            ui.ScrollState
	auditScroll               ui.ScrollState
	drawerScroll              ui.ScrollState
	selectedKeyID             string
	selectedPolicyID          string
	selectedAuditID           string
	policyModalOpen           bool
	auditDrawerOpen           bool
	auditDrawerWidth          float32
	policyCategorySeg         int // 0 = 全部, 1 = 命令策略, 2 = 文件策略
	policyConnFilter          string
	policyKeyFilter           string
	policyStateFilter         string
	auditKeyFilter            string
	auditConnFilter           string
	auditTypeFilter           string
	auditTimeFilter           string
	auditResultFilter         string
	auditFileFilter           int
	auditFilesOpen            bool
	auditPage                 int
	policyPage                int
	retentionDays             string
	keys                      []mcpSecretKey
	policies                  []mcpPolicy
	audits                    []mcpAuditItem
	keyModal                  mcpKeyModalDraft
	keyConfirm                mcpKeyConfirmDialog
	policyDelete              mcpPolicyDeleteDialog
	policyDraft               mcpPolicyDraft
	listenModalOpen           bool
	hostDraft                 string
	portDraft                 string
	retentionOpen             bool
	configModalOpen           bool
}

func (a *app) initMCPUI() {
	a.mcp = mcpUIState{host: "127.0.0.1", port: "7801", activeTab: 2, auditDrawerWidth: theme.Current().C.Mcp.DrawerWidth,
		policyConnFilter: "all", policyKeyFilter: "all", policyStateFilter: "all",
		auditKeyFilter: "all", auditConnFilter: "all", auditTypeFilter: "all", auditTimeFilter: "today", auditResultFilter: "all",
		auditFilesOpen: true, auditPage: 1, policyPage: 1, keyPage: 1, retentionDays: "30"}
}

func (a *app) mcpListenAddress() string {
	return net.JoinHostPort(a.mcp.host, a.mcp.port)
}

func (a *app) mcpClientConfigJSON() string {
	return fmt.Sprintf("{\n  \"mcpServers\": {\n    \"burrow\": {\n      \"url\": %q,\n      \"headers\": {\n        \"Authorization\": \"Bearer <YOUR_SECRET_KEY>\"\n      }\n    }\n  }\n}", "http://"+a.mcpListenAddress()+"/mcp")
}

func (a *app) mcpPage(c *ui.Context) {
	t := theme.Current()
	ui.Column(c).Key("mcp-page").Fill().Gap(t.Space.N8).Children(func() {
		a.mcpServiceBar(c)
		if !a.mcp.running {
			res := uikit.Banner(c, locales.McpStoppedTransferNotice(a.mcpPausedTasks()), uikit.BannerProps{
				Tone:    uikit.ToneWarning,
				Icon:    uikit.IconSVG(uikit.IconInfo),
				Actions: []string{locales.MessageViewRelatedTasks()},
			})
			if res.ActionIndex == 0 {
				a.mcp.activeTab = 2
				for _, task := range a.transfers.tasks {
					if task.mcp != nil && task.state == transferPaused {
						a.mcp.selectedAuditID = task.mcp.OperationID
						a.mcp.auditDrawerOpen = true
						break
					}
				}
			}
		}
		workspace := ui.Row(c).Key("mcp-workspace").Grow(1).MinHeight(0).FillWidth().AlignItems(ui.Stretch)
		target := float32(0)
		if a.mcp.activeTab == 2 && a.mcp.auditDrawerOpen {
			target = 1
		}
		initialized := ui.Local(workspace, "audit-drawer-initialized", func() bool { return false })
		if !*initialized {
			workspace.Jump("audit-drawer-open", 0)
			*initialized = true
		}
		shown := workspace.AnimateTo("audit-drawer-open", target, a.positionAnimation(ui.Animation{Spring: uikit.Spring(t.Motion.Spring.Smooth)}))
		if a.reduceMotion(c) {
			shown = workspace.Jump("audit-drawer-open", target)
		}
		workspace.Children(func() {
			uikit.Card(c).Key("mcp-main-card").Grow(1).MinWidth(0).FillHeight().Children(func() {
				a.mcpMainCardContent(c)
			})
			if target != 0 || shown > 0 {
				windowWidth, _ := c.Size()
				minWidth := t.C.Mcp.DrawerMinWidth
				maxWidth := max(minWidth, windowWidth-t.C.Rail.Width-3*t.C.Window.Gutter-t.C.Mcp.MainMinWidth)
				a.mcp.auditDrawerWidth = min(max(a.mcp.auditDrawerWidth, minWidth), maxWidth)
				if target == 1 && workspace.Settled("audit-drawer-open") {
					divider := uikit.DrawerDivider(c, "mcp-audit-resize", locales.MessageResizeAuditDetails(), a.mcp.auditDrawerWidth)
					if uikit.ResizeGutter(c, divider, &a.mcp.auditDrawerWidth, minWidth, maxWidth, true) {
						c.Invalidate()
					}
				}
				ui.Box(c).Key("mcp-audit-drawer-viewport").Width(a.mcp.auditDrawerWidth * shown).
					MarginStart(t.C.Window.Gutter * shown).Shrink(0).FillHeight().Clip().Children(func() {
					uikit.Card(c).Key("mcp-audit-drawer").Width(a.mcp.auditDrawerWidth).
						Shrink(0).FillHeight().Children(func() { a.mcpAuditDrawer(c) })
				})
			}
		})
	})
}

func (a *app) mcpServiceBar(c *ui.Context) {
	t := theme.Current()
	uikit.Card(c).Key("mcp-service-bar").FillWidth().Shrink(0).Padding(t.Space.N10, t.Space.N16).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N14).Children(func() {
			sw := uikit.Switch(c, &a.mcp.running, uikit.ToggleProps{
				Label: locales.MessageEnableMcpService(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
			})
			if sw.Changed() {
				a.configureMCPBackend()
			}
			uikit.Text(c, locales.MessageMcpService(), uikit.TextProps{Style: uikit.TextBodyStrong})
			statusText := locales.MessageRunning()
			statusTone := uikit.ToneSuccess
			if !a.mcp.running {
				statusText = locales.MessageOff()
				statusTone = uikit.ToneNeutral
			}
			uikit.Badge(c, statusText, uikit.BadgeProps{Tone: statusTone, Dot: true, Size: uikit.SizeMd})

			ui.Row(c).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
				uikit.Text(
					c,
					locales.MessageListeningAddress(),
					uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
				)
				addr := a.mcpListenAddress()
				if uikit.CopyField(
					c,
					addr,
					uikit.CopyFieldProps{Key: "mcp-listen-addr", Label: locales.MessageCopyListeningAddress()},
				).Copied {
					mygo.Clipboard.WriteText("http://" + addr + "/mcp")
				}
			})

			activeKeys := 0
			for _, k := range a.mcp.keys {
				if k.State == "active" {
					activeKeys++
				}
			}
			enabledPolicies := 0
			for _, p := range a.mcp.policies {
				if p.Enabled {
					enabledPolicies++
				}
			}
			statPair := func(label, val string, warn bool) {
				ui.Row(c).Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
					uikit.Text(c, label, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneTertiary})
					tone := uikit.ToneNeutral
					if warn {
						tone = uikit.ToneWarning
					}
					uikit.Text(c, val, uikit.TextProps{Style: uikit.TextBodyStrong, Tone: tone}).FontFeatures("tnum")
				})
			}
			statPair(locales.MessageUptime(), a.mcp.uptime, false)
			statPair(locales.MessageEnableKey(), strconv.Itoa(activeKeys), false)
			statPair(locales.MessageEnablePolicy(), strconv.Itoa(enabledPolicies), false)
			statPair(locales.MessageCallsToday(), strconv.Itoa(a.mcp.todayCalls), false)
			statPair(locales.MessageBlockedToday(), strconv.Itoa(a.mcp.todayIntercepts), a.mcp.todayIntercepts > 0)

			ui.Spacer(c)
			ui.Row(c).Gap(t.Space.N6).Shrink(0).AlignItems(ui.Center).Children(func() {
				if uikit.IconButton(c, uikit.IconCopy, uikit.IconButtonProps{
					Key: "mcp-copy-config", Label: locales.MessageCopyClientConfiguration(),
				}).Clicked() {
					mygo.Clipboard.WriteText(a.mcpClientConfigJSON())
					a.mcp.configModalOpen = true
				}
				if uikit.IconButton(c, uikit.IconEdit, uikit.IconButtonProps{
					Key: "mcp-edit-listen", Label: locales.MessageEditListenerConfiguration(),
				}).Clicked() {
					a.mcp.hostDraft, a.mcp.portDraft = a.mcp.host, a.mcp.port
					a.mcp.listenModalOpen = true
				}
			})
		})
	})
}

func (a *app) mcpMainCardContent(c *ui.Context) {
	t := theme.Current()
	auditCount := a.mcp.auditTotal
	activeKeys := 0
	for _, k := range a.mcp.keys {
		if k.State == "active" {
			activeKeys++
		}
	}
	enabledPolicies := 0
	for _, p := range a.mcp.policies {
		if p.Enabled {
			enabledPolicies++
		}
	}
	ui.Column(c).Fill().Children(func() {
		// Top tab bar + contextual actions
		ui.Row(c).FillWidth().Shrink(0).Padding(t.Space.N10, t.Space.N14).
			BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).
			AlignItems(ui.Center).Gap(t.Space.N10).Wrap().Children(func() {
			items := []uikit.SegItem{
				{Label: locales.MessageAccessKeys(), Icon: uikit.IconKey, Count: &activeKeys},
				{Label: locales.MessageInterceptionPolicies(), Icon: uikit.IconShield, Count: &enabledPolicies},
				{Label: locales.MessageAuditLog(), Icon: uikit.IconDocument, Count: &auditCount},
			}
			uikit.Segmented(c, &a.mcp.activeTab, items, uikit.SegmentedProps{
				Motion: uikit.MotionOverride(a.reduceMotion(c)),
				Label:  locales.MessageMcpTabs(), CountMax: 99,
			})
			ui.Spacer(c)
			switch a.mcp.activeTab {
			case 0:
				if uikit.Button(
					c,
					locales.MessageNewKey(),
					uikit.ButtonProps{Variant: uikit.ButtonPrimary, Icon: uikit.IconPlus},
				).Clicked() {
					a.openNewMCPKeyModal()
				}
			case 1:
				if uikit.Button(
					c,
					locales.MessageNewPolicy(),
					uikit.ButtonProps{Variant: uikit.ButtonPrimary, Icon: uikit.IconPlus},
				).Clicked() {
					a.openNewMCPPolicyModal()
				}
			case 2:
				if uikit.Button(
					c,
					locales.McpRetentionAction(a.mcp.retentionDays),
					uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm},
				).Clicked() {
					a.mcp.retentionOpen = true
				}
				if uikit.IconButton(
					c,
					uikit.IconRefresh,
					uikit.IconButtonProps{Key: "audit-refresh", Label: locales.MessageRefreshAuditLog(), Size: uikit.SizeSm},
				).Clicked() {
					a.refreshMCPBackend()
				}
			}
		})
		switch a.mcp.activeTab {
		case 0:
			a.mcpKeysTab(c)
		case 1:
			a.mcpPoliciesTab(c)
		default:
			a.mcpAuditTab(c)
		}
	})
}

// Each MCP table lays out its header and rows from one column list.
func mcpKeyColumns(t *theme.Theme) []uikit.Column {
	p := t.C.Mcp
	return []uikit.Column{
		{ID: "name", Title: locales.MessageKeyNameId(), Width: p.KeyName},
		{ID: "state", Title: locales.MessageStatus(), Width: p.KeyState},
		{ID: "grants", Title: locales.MessageConnectionGrants()},
		{ID: "dirs", Title: locales.MessageDirectoryGrants(), Width: p.KeyDirs},
		{ID: "used", Title: locales.MessageLastUsedCreated(), Width: p.KeyUsed},
	}
}

func mcpPolicyColumns(t *theme.Theme) []uikit.Column {
	p := t.C.Mcp
	return []uikit.Column{
		{ID: "name", Title: locales.MessagePolicyNameDescription(), Width: p.PolicyName},
		{ID: "category", Title: locales.MessageCategory(), Width: p.PolicyCategory},
		{ID: "connections", Title: locales.MessageApplicableConnections(), Width: p.PolicyConnections},
		{ID: "keys", Title: locales.MessageApplicableKeys(), Width: p.PolicyKeys},
		{ID: "rules", Title: locales.MessageRuleSummary()},
		{ID: "state", Title: locales.MessageStatusMatches(), Width: p.PolicyState},
	}
}

func mcpAuditColumns(t *theme.Theme) []uikit.Column {
	p := t.C.Mcp
	return []uikit.Column{
		{ID: "time", Title: locales.MessageTime(), Width: p.AuditTime},
		{ID: "key", Title: locales.MessageKeyName(), Width: p.AuditKey},
		{ID: "connection", Title: locales.MessageTargetConnection(), Width: p.AuditConnection},
		{ID: "action", Title: locales.MessageActionType(), Width: p.AuditAction},
		{ID: "summary", Title: locales.MessageCommandOrPathSummary()},
		{ID: "status", Title: locales.MessageStatus(), Width: p.AuditStatus},
		{ID: "duration", Title: locales.MessageDuration(), Width: p.AuditDuration, Align: uikit.AlignEnd},
	}
}

func (a *app) mcpKeysTab(c *ui.Context) {
	t := theme.Current()
	// Keep active/disabled keys first and revoked keys at the bottom, matching the draft.
	ordered := slices.Clone(a.mcp.keys)
	slices.SortStableFunc(ordered, func(left, right mcpSecretKey) int {
		lRev := left.State == "revoked"
		rRev := right.State == "revoked"
		if lRev != rRev {
			if lRev {
				return 1
			}
			return -1
		}
		return 0
	})
	count := len(ordered)
	a.mcp.keyPage = min(max(a.mcp.keyPage, 1), max(1, (count+19)/20))
	offset := (a.mcp.keyPage - 1) * 20
	pageItems := ordered[min(offset, count):min(offset+20, count)]
	columns := mcpKeyColumns(t)
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		props := uikit.TableViewportProps{
			Label: locales.MessageAccessKeyList(), MinWidth: t.C.Mcp.TableMinWidth, PaddingY: t.Space.N8,
			Header: func() { uikit.TableHeaderRow(c, columns) },
		}
		if count == 0 {
			props.Empty = func() {
				uikit.EmptyState(c, uikit.EmptyProps{
					IconName: uikit.IconKey, Title: locales.MessageNoAccessKeysYet(),
					Detail: locales.MessageCreateAKeyAndGrantConnectionAccessSoAnMcpClientCanUse(), Action: locales.MessageNewKey(),
				}).Action.OnClick(a.openNewMCPKeyModal)
			}
		}
		uikit.TableViewport(c, &a.mcp.keysScroll, props, func() {
			ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
				for _, item := range pageItems {
					keyItem := item
					revoked := keyItem.State == "revoked"
					row := uikit.ListRow(c, uikit.ListRowProps{
						Key: keyItem.ID, Label: keyItem.Name, Table: true, PaddingY: t.Space.N10,
						Selected: a.mcp.selectedKeyID == keyItem.ID, Focused: true,
					})
					if revoked {
						row.Opacity(t.Opacity.Disabled)
					}
					row.OnClick(func() {
						a.mcp.selectedKeyID = keyItem.ID
					})
					var menuEntries []uikit.MenuEntry
					if !revoked {
						toggleLabel := locales.MessageDisableKey2()
						if keyItem.State == "disabled" {
							toggleLabel = locales.MessageEnableKey()
						}
						menuEntries = []uikit.MenuEntry{
							{
								ID: "edit", Label: locales.MessageEditKey(), Icon: uikit.IconEdit,
								OnChoose: func() { a.openEditMCPKeyModal(keyItem.ID) },
							},
							{
								ID: "toggle", Label: toggleLabel, Icon: uikit.IconPause,
								OnChoose: func() {
									if keyItem.State == "disabled" {
										a.setMCPKeyState(keyItem.ID, "active")
									} else {
										a.mcp.keyConfirm = mcpKeyConfirmDialog{
											Open: true, KeyID: keyItem.ID, Name: keyItem.Name, Action: "disable",
										}
									}
								},
							},
							{ID: "sep", Separator: true},
							{
								ID: "revoke", Label: locales.MessageRevokeKey2(), Icon: uikit.IconTrash, Danger: true,
								OnChoose: func() {
									a.mcp.keyConfirm = mcpKeyConfirmDialog{
										Open: true, KeyID: keyItem.ID, Name: keyItem.Name, Action: "revoke",
									}
								},
							},
						}
					}
					row.Children(func() {
						uikit.TableCell(c, columns[0]).Children(func() {
							uikit.Text(c, keyItem.Name, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
							uikit.Text(c, keyItem.ID+" · "+keyItem.Prefix, uikit.TextProps{
								Style: uikit.TextMono, Tone: uikit.ToneTertiary,
							}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[1]).AlignItems(ui.Start).Children(func() {
							st := uikit.StatusEnabled
							if keyItem.State == "disabled" {
								st = uikit.StatusDisabled
							} else if keyItem.State == "revoked" {
								st = uikit.StatusRevoked
							}
							uikit.StatusPill(c, st)
						})
						uikit.TableCell(c, columns[2]).Children(func() {
							uikit.Text(c, keyItem.GrantsSummary, uikit.TextProps{Style: uikit.TextSmall}).SingleLine().Ellipsis("…")
							uikit.Text(c, keyItem.PolicyHint, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[3]).Children(func() {
							upText := locales.MessageUploadNotAuthorized()
							if len(keyItem.UploadDirs) > 0 {
								upText = locales.MessageUpload2() + keyItem.UploadDirs[0]
							}
							downText := locales.MessageDownloadNotAuthorized()
							if len(keyItem.DownloadDirs) > 0 {
								downText = locales.MessageDownload2() + keyItem.DownloadDirs[0]
							}
							if revoked {
								upText, downText = "—", "—"
							}
							uikit.Text(c, upText, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…")
							uikit.Text(c, downText, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[4]).Children(func() {
							ui.Row(c).FillWidth().Gap(t.Space.N6).Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
									uikit.Text(c, keyItem.LastUsed, uikit.TextProps{Style: uikit.TextFootnote}).SingleLine().Ellipsis("…")
									uikit.Text(c, keyItem.CreatedAt, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
								})
								if len(menuEntries) > 0 {
									more := uikit.SiderMore(c, row, uikit.SiderMoreProps{
										Icon: moreIcon, Label: locales.ActionsForName(keyItem.Name),
									})
									uikit.RowMenuOn(c, row, more, menuEntries, uikit.MotionOverride(a.reduceMotion(c)))
								}
							})
						})
					})
				}
			})
		})
		uikit.Pagination(c, &a.mcp.keyPage, uikit.PaginationProps{
			Total: len(ordered), PageSize: 20,
			Summary: locales.McpKeysCount(len(ordered)),
		})
	})
}

func (a *app) mcpPoliciesTab(c *ui.Context) {
	t := theme.Current()
	connectionIDs := a.mcpConnectionFilterIDs(a.mcp.policyConnFilter)
	totalAll, totalSSH, totalFiles := len(a.mcp.policies), 0, 0
	for _, p := range a.mcp.policies {
		if p.Category == "ssh" {
			totalSSH++
		} else {
			totalFiles++
		}
	}
	var filtered []mcpPolicy
	for _, p := range a.mcp.policies {
		if a.mcp.policyCategorySeg == 1 && p.Category != "ssh" {
			continue
		}
		if a.mcp.policyCategorySeg == 2 && p.Category != "files" {
			continue
		}
		if a.mcp.policyStateFilter == "enabled" && !p.Enabled {
			continue
		}
		if a.mcp.policyStateFilter == "disabled" && p.Enabled {
			continue
		}
		if len(connectionIDs) > 0 && !slices.ContainsFunc(p.Connections, func(id string) bool {
			return slices.Contains(connectionIDs, id)
		}) {
			continue
		}
		if a.mcp.policyKeyFilter != "all" && !p.AllKeys && !slices.Contains(p.KeyIDs, a.mcp.policyKeyFilter) {
			continue
		}
		filtered = append(filtered, p)
	}

	count := len(filtered)
	a.mcp.policyPage = min(max(a.mcp.policyPage, 1), max(1, (count+19)/20))
	offset := (a.mcp.policyPage - 1) * 20
	pageItems := filtered[min(offset, count):min(offset+20, count)]
	columns := mcpPolicyColumns(t)
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		props := uikit.TableViewportProps{
			Label: locales.MessageInterceptionPolicies(), MinWidth: t.C.Mcp.TableMinWidth, PaddingY: t.Space.N8,
			Header: func() {
				uikit.TableToolbar(c).Children(func() {
					ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
						catItems := []uikit.SegItem{
							{Label: locales.MessageAll(), Count: &totalAll},
							{Label: locales.MessageCommandPolicy(), Count: &totalSSH},
							{Label: locales.MessageFilePolicy(), Count: &totalFiles},
						}
						uikit.Segmented(
							c,
							&a.mcp.policyCategorySeg,
							catItems,
							uikit.SegmentedProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Size: uikit.SizeSm, Label: locales.MessageFilterPolicyCategories()},
						)
						uikit.FilterChip(
							c,
							locales.MessageApplicableConnections(),
							&a.mcp.policyConnFilter,
							a.mcpConnectionOptions(-1, true),
							uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "pol-conn-filter"},
						)
						uikit.FilterChip(
							c,
							locales.MessageApplicableKeys(),
							&a.mcp.policyKeyFilter,
							a.mcpKeyOptions(true),
							uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "pol-key-filter"},
						)
						uikit.FilterChip(c, locales.MessageStatus(), &a.mcp.policyStateFilter, []uikit.Option[string]{
							{Value: "all", Label: locales.MessageAll()},
							{Value: "enabled", Label: locales.MessageEnabled()},
							{Value: "disabled", Label: locales.MessageDisabled()},
						}, uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "pol-state-filter"})
					})
					uikit.Banner(c, locales.MessageAnActionIsAllowedOnlyIfTheKeyIsAuthorizedAndNoEnabled(), uikit.BannerProps{
						Icon: uikit.IconSVG(uikit.IconShield),
					})
				})
				uikit.TableHeaderRow(c, columns)
			},
		}
		switch {
		case totalAll == 0:
			props.Empty = func() {
				uikit.EmptyState(c, uikit.EmptyProps{
					IconName: uikit.IconShield, Title: locales.MessageNoInterceptionPoliciesYet(),
					Detail: locales.MessageCreateAPolicyToBlockMcpCommandsOrFileOperationsThatMatchIts(), Action: locales.MessageNewPolicy(),
				}).Action.OnClick(a.openNewMCPPolicyModal)
			}
		case count == 0:
			props.Empty = func() {
				uikit.EmptyState(c, uikit.EmptyProps{
					IconName: uikit.IconShield, Title: locales.MessageNoMatchingResults(),
					Detail: locales.MessageNoInterceptionPoliciesMatchTheCurrentFilters(), Action: locales.MessageResetFilters(),
				}).Action.OnClick(func() {
					a.mcp.policyCategorySeg = 0
					a.mcp.policyConnFilter, a.mcp.policyKeyFilter, a.mcp.policyStateFilter = "all", "all", "all"
				})
			}
		}
		uikit.TableViewport(c, &a.mcp.policiesScroll, props, func() {
			ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
				for _, item := range pageItems {
					pol := item
					row := uikit.ListRow(c, uikit.ListRowProps{
						Key: pol.ID, Label: pol.Name, Table: true, PaddingY: t.Space.N10,
						Selected: a.mcp.selectedPolicyID == pol.ID, Focused: true,
					})
					row.OnClick(func() {
						a.mcp.selectedPolicyID = pol.ID
					})
					toggleLabel := locales.MessageDisablePolicy()
					if !pol.Enabled {
						toggleLabel = locales.MessageEnablePolicy()
					}
					entries := []uikit.MenuEntry{
						{
							ID: "edit", Label: locales.MessageEditPolicy(), Icon: uikit.IconEdit,
							OnChoose: func() {
								a.mcp.selectedPolicyID = pol.ID
								a.mcp.policyModalOpen = true
								a.loadPolicyIntoDraft(&pol)
							},
						},
						{
							ID: "toggle", Label: toggleLabel, Icon: uikit.IconPause,
							OnChoose: func() { a.toggleMCPPolicy(pol.ID) },
						},
						{
							ID: "audit", Label: locales.MessageViewMatchingAuditRecords(), Icon: uikit.IconDocument,
							OnChoose: func() {
								a.mcp.activeTab = 2
								a.mcp.auditResultFilter = "intercepted"
								a.mcp.auditPolicyID, a.mcp.auditPage = pol.ID, 1
								a.mcp.auditKeyFilter, a.mcp.auditConnFilter, a.mcp.auditTypeFilter = "all", "all", "all"
							},
						},
						{ID: "sep", Separator: true},
						{
							ID: "delete", Label: locales.MessageDeletePolicy2(), Icon: uikit.IconTrash, Danger: true,
							OnChoose: func() {
								a.mcp.policyDelete = mcpPolicyDeleteDialog{
									Open: true, PolicyID: pol.ID, Name: pol.Name,
								}
							},
						},
					}
					row.Children(func() {
						uikit.TableCell(c, columns[0]).Children(func() {
							uikit.Text(c, pol.Name, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
							uikit.Text(c, fmt.Sprintf("%s · v%d", pol.Description, pol.Version), uikit.TextProps{
								Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
							}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[1]).AlignItems(ui.Start).Children(func() {
							if pol.Category == "ssh" {
								uikit.Badge(c, locales.MessageCommandPolicy(), uikit.BadgeProps{Tone: uikit.ToneWarning})
							} else {
								uikit.Badge(c, locales.MessageFilePolicy(), uikit.BadgeProps{Tone: uikit.ToneAccent})
							}
						})
						uikit.TableCell(c, columns[2]).Children(func() {
							uikit.Text(c, pol.ConnSummary, uikit.TextProps{Style: uikit.TextFootnote}).SingleLine().Ellipsis("…")
							uikit.Text(c, pol.ConnSub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[3]).Children(func() {
							keyTitle, keySub := locales.MessageAllKeys(), locales.MessageIncludesKeysAddedLater()
							if !pol.AllKeys {
								keyTitle = strings.Join(pol.KeyNames, ", ")
								keySub = locales.McpSelectedKeysCount(len(pol.KeyNames))
							}
							uikit.Text(c, keyTitle, uikit.TextProps{Style: uikit.TextFootnote}).SingleLine().Ellipsis("…")
							uikit.Text(c, keySub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[4]).Children(func() {
							uikit.Text(c, pol.RulePrimary, uikit.TextProps{Style: uikit.TextMono}).SingleLine().Ellipsis("…")
							uikit.Text(c, pol.RuleSub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[5]).Children(func() {
							ui.Row(c).FillWidth().Gap(t.Space.N6).Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
									if pol.Enabled {
										uikit.StatusPill(c, uikit.StatusEnabled).AlignSelf(ui.Start)
									} else {
										uikit.StatusPill(c, uikit.StatusDisabled).AlignSelf(ui.Start)
									}
									tone := uikit.ToneTertiary
									uikit.Text(c, pol.HitsSummary, uikit.TextProps{Style: uikit.TextCaption, Tone: tone}).SingleLine().Ellipsis("…")
								})
								more := uikit.SiderMore(c, row, uikit.SiderMoreProps{
									Icon: moreIcon, Label: locales.ActionsForName(pol.Name),
								})
								if uikit.RowMenuOn(c, row, more, entries, uikit.MotionOverride(a.reduceMotion(c))) {
									a.mcp.selectedPolicyID = pol.ID
								}
							})
						})
					})
				}
			})
		})
		uikit.Pagination(c, &a.mcp.policyPage, uikit.PaginationProps{
			Total: len(filtered), PageSize: 20,
			Summary: locales.McpPoliciesCount(len(filtered)),
		})
	})
}

func (a *app) mcpAuditTab(c *ui.Context) {
	t := theme.Current()
	filtered := a.mcp.audits
	columns := mcpAuditColumns(t)

	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		props := uikit.TableViewportProps{
			Label: locales.MessageAuditLog(), MinWidth: t.C.Mcp.TableMinWidth, PaddingY: t.Space.N6,
			Header: func() {
				uikit.TableToolbar(c).Children(func() {
					ui.Row(c).FillWidth().Gap(t.Space.N8).Children(func() {
						uikit.FilterChip(
							c,
							locales.MessageAccessKeys(),
							&a.mcp.auditKeyFilter,
							a.mcpKeyOptions(true),
							uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "aud-key-filter"},
						)
						uikit.FilterChip(
							c,
							locales.MessageTargetConnection(),
							&a.mcp.auditConnFilter,
							a.mcpConnectionOptions(-1, true),
							uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "aud-conn-filter"},
						)
						uikit.FilterChip(c, locales.MessageActionType(), &a.mcp.auditTypeFilter, []uikit.Option[string]{
							{Value: "all", Label: locales.MessageAll()},
							{Value: "files_upload", Label: locales.MessageFileUpload()},
							{Value: "ssh_exec", Label: locales.MessageCommandExecution()},
							{Value: "files_read", Label: locales.MessageFileRead()},
							{Value: "policy.", Label: locales.MessagePolicyManagement()},
							{Value: "mcp.", Label: locales.MessageProtocolRequest()},
							{Value: "authentication", Label: locales.MessageAuthenticationFailed()},
						}, uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "aud-type-filter"})
						uikit.FilterChip(c, locales.MessageTime(), &a.mcp.auditTimeFilter, []uikit.Option[string]{
							{Value: "today", Label: locales.MessageToday2()},
							{Value: "7d", Label: locales.MessageLastDays()},
							{Value: "30d", Label: locales.MessageLastDays2()},
						}, uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "aud-time-filter"})
						uikit.FilterChip(c, locales.MessageResult(), &a.mcp.auditResultFilter, []uikit.Option[string]{
							{Value: "all", Label: locales.MessageAll()},
							{Value: "intercepted", Label: locales.MessageBlockedByPolicy()},
							{Value: "denied", Label: locales.MessagePermissionDeniedAuthenticationFailed()},
							{Value: "done", Label: locales.MessageCompleted()},
						}, uikit.FilterChipProps{Motion: uikit.MotionOverride(a.reduceMotion(c)), Key: "aud-res-filter"})
					})
				})
				uikit.TableHeaderRow(c, columns)
			},
		}
		if len(filtered) == 0 {
			props.Empty = func() {
				uikit.EmptyState(c, uikit.EmptyProps{
					IconName: uikit.IconDocument, Title: locales.MessageNoMatchingResults(),
					Detail: locales.MessageNoAuditRecordsMatchTheCurrentFilters(), Action: locales.MessageResetFilters(),
				}).Action.OnClick(func() {
					a.mcp.auditKeyFilter, a.mcp.auditConnFilter, a.mcp.auditTypeFilter, a.mcp.auditResultFilter = "all", "all", "all", "all"
				})
			}
		}
		uikit.TableViewport(c, &a.mcp.auditScroll, props, func() {
			ui.Column(c).FillWidth().Gap(t.Space.N2).Children(func() {
				for _, entry := range filtered {
					item := entry
					row := uikit.ListRow(c, uikit.ListRowProps{
						Key: item.ID, Label: item.KeyName + " " + item.ActionType, Table: true, PaddingY: t.Space.N8,
						Selected: a.mcp.selectedAuditID == item.ID && a.mcp.auditDrawerOpen, Focused: true,
					})
					row.OnClick(func() {
						a.mcp.selectedAuditID = item.ID
						a.mcp.auditDrawerOpen = true
					})
					row.Children(func() {
						uikit.TableCell(c, columns[0]).Children(func() {
							uikit.Text(c, item.Time, uikit.TextProps{Style: uikit.TextNumeric})
							uikit.Text(c, item.Day, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
						})
						uikit.TableCell(c, columns[1]).Children(func() {
							uikit.Text(c, item.KeyName, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[2]).Children(func() {
							uikit.Text(c, item.ConnName, uikit.TextProps{Style: uikit.TextSmall}).SingleLine().Ellipsis("…")
							uikit.Text(c, item.ConnProtocol, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
						})
						uikit.TableCell(c, columns[3]).Children(func() {
							ui.Row(c).FillWidth().Gap(t.Space.N4).Children(func() {
								icon := uikit.IconDocument
								if item.IsCommand {
									icon = uikit.IconTerminal
								} else if item.Action == "files_upload" {
									icon = uikit.IconUpload
								} else if strings.HasPrefix(item.Action, "policy.") {
									icon = uikit.IconShield
								}
								uikit.Icon(c, icon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSecondary})
								uikit.Text(c, item.ActionType, uikit.TextProps{Style: uikit.TextSmall}).SingleLine()
							})
						})
						uikit.TableCell(c, columns[4]).Children(func() {
							uikit.Text(c, item.Summary, uikit.TextProps{Style: uikit.TextMono}).SingleLine().Ellipsis("…")
						})
						uikit.TableCell(c, columns[5]).AlignItems(ui.Start).Children(func() {
							uikit.StatusPill(c, item.Status)
						})
						uikit.TableCell(c, columns[6]).Children(func() {
							uikit.Text(c, item.Duration, uikit.TextProps{Style: uikit.TextNumeric, Tone: uikit.ToneSecondary})
						})
					})
				}
			})
		})
		uikit.Pagination(c, &a.mcp.auditPage, uikit.PaginationProps{
			Total: a.mcp.auditFiltered, PageSize: 20,
			Summary: locales.McpAuditsCount(a.mcp.auditFiltered),
		})
	})
}
