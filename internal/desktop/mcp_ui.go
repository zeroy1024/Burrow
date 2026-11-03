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
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

type mcpKeyGrant struct {
	EndpointDigest          string
	ConnectionID            string
	Name, Subtitle, Warning string
	SupportsFiles           bool
	SupportsSSH             bool
	AllowRead, AllowWrite   bool
	AllowExec               bool
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
	Mode    string // "exact", "regex", or "contains"
	Pattern string
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
	WaitingPrompt string
}

type mcpPolicyHitSnapshot struct {
	PolicyID      string
	PolicyName    string
	Version       int
	Category      string
	RuleSummary   string
	ForbiddenText string
	HitCountText  string
}

type mcpAuditItem struct {
	FileTotal      int
	FileCounts     map[string]int
	ConnectionID   string
	ID             string
	Time, Day      string
	KeyID, KeyName string
	ConnName       string
	ConnProtocol   string
	ConnHost       string
	ConnPort       string
	ConnUser       string
	ActionType     string
	Summary        string
	Status         uikit.Status
	Duration       string
	StartTime      string
	EndTime        string
	IsCommand      bool
	CommandText    string
	ExitCodeText   string
	OutputStatus   string
	LocalPath      string
	RemotePath     string
	TransferStat   string
	PolicyHit      *mcpPolicyHitSnapshot
	Task           *mcpAuditTask
	Timeline       []uikit.TimelineItem
	Files          []mcpAuditFileItem
}

type mcpKeyModalDraft struct {
	Open           bool
	EditingID      string
	OneTimeView    bool
	GeneratedKey   string
	SavedConfirmed bool
	Name           string
	NameError      string
	Grants         []mcpKeyGrant
	UploadDirs     []string
	DownloadDirs   []string
	Scroll         ui.ScrollState
}

type mcpKeyConfirmDialog struct {
	Open   bool
	KeyID  string
	Name   string
	Action string // "disable" or "revoke"
}

type mcpPolicyDeleteDialog struct {
	Open     bool
	PolicyID string
	Name     string
}

type mcpPolicyDraft struct {
	EditingID    string
	Version      int
	Name         string
	NameError    string
	SaveError    string
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
	a.mcp = mcpUIState{host: "127.0.0.1", port: "7801", activeTab: 2, auditDrawerWidth: 500,
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
			res := uikit.Banner(c, fmt.Sprintf("MCP 服务已关闭：%d 个后台传输任务已暂停保存；重新开启服务不会自动继续旧任务。", a.mcpPausedTasks()), uikit.BannerProps{
				Tone:    uikit.ToneWarning,
				Icon:    uikit.IconSVG(uikit.IconInfo),
				Actions: []string{"查看关联任务"},
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
		shown := workspace.AnimateTo("audit-drawer-open", target, a.positionAnimation(uikit.Tween(t.C.Desktop.SidebarDuration)))
		if a.reduceMotion(c) {
			shown = workspace.Jump("audit-drawer-open", target)
		}
		workspace.Children(func() {
			uikit.Card(c).Key("mcp-main-card").Grow(1).MinWidth(0).FillHeight().Children(func() {
				a.mcpMainCardContent(c)
			})
			if target != 0 || shown > 0 {
				windowWidth, _ := c.Size()
				minWidth := float32(340)
				maxWidth := max(minWidth, windowWidth-t.C.Rail.Width-2*t.C.Window.Gutter-360-t.Space.N8)
				a.mcp.auditDrawerWidth = min(max(a.mcp.auditDrawerWidth, minWidth), maxWidth)
				if target == 1 && workspace.Settled("audit-drawer-open") {
					divider := ui.Box(c).Key("mcp-audit-resize").Absolute().Top(0).Bottom(0).
						Right(a.mcp.auditDrawerWidth).Width(t.Space.N8).Center().
						Cursor(ui.CursorResizeEW).Focusable().FocusRing(false).Role(ui.RoleSplitter).
						Label("调整审计详情宽度")
					delta, _, dragging := divider.Dragged()
					if dragging {
						divider.Focus()
					}
					if divider.Shortcut(0, ui.KeyLeft) {
						delta -= t.C.Desktop.SiderKeyboardStep
					}
					if divider.Shortcut(0, ui.KeyRight) {
						delta += t.C.Desktop.SiderKeyboardStep
					}
					if delta != 0 {
						a.mcp.auditDrawerWidth = min(max(a.mcp.auditDrawerWidth-delta, minWidth), maxWidth)
						c.Invalidate()
					}
					divider.Range(float64(minWidth), float64(maxWidth), float64(a.mcp.auditDrawerWidth))
					visible := float32(0)
					if divider.Hovered() || dragging || divider.FocusVisible() {
						visible = 1
					}
					uikit.StyleSidebarDivider(c, divider, visible, dragging)
				}
				ui.Box(c).Key("mcp-audit-drawer-viewport").Width(a.mcp.auditDrawerWidth * shown).
					MarginStart(t.Space.N8 * shown).Shrink(0).FillHeight().Clip().Children(func() {
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
			sw := uikit.Switch(c, &a.mcp.running, uikit.ToggleProps{Label: "启用 MCP 服务"})
			if sw.Changed() {
				a.configureMCPBackend()
			}
			uikit.Text(c, "MCP 服务", uikit.TextProps{Style: uikit.TextBodyStrong})
			statusText := "运行中"
			statusTone := uikit.ToneSuccess
			if !a.mcp.running {
				statusText = "未开启"
				statusTone = uikit.ToneNeutral
			}
			uikit.Badge(c, statusText, uikit.BadgeProps{Tone: statusTone, Dot: true, Size: uikit.SizeMd})

			ui.Row(c).Gap(t.Space.N6).AlignItems(ui.Center).Children(func() {
				uikit.Text(c, "监听地址", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
				addr := a.mcpListenAddress()
				if uikit.CopyField(c, addr, uikit.CopyFieldProps{Key: "mcp-listen-addr", Label: "复制监听地址"}).Copied {
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
			statPair("已运行", a.mcp.uptime, false)
			statPair("启用 Key", strconv.Itoa(activeKeys), false)
			statPair("拦截策略", strconv.Itoa(enabledPolicies), false)
			statPair("今日调用", strconv.Itoa(a.mcp.todayCalls), false)
			statPair("今日拦截", strconv.Itoa(a.mcp.todayIntercepts), a.mcp.todayIntercepts > 0)

			ui.Spacer(c)
			ui.Row(c).Gap(t.Space.N6).Shrink(0).AlignItems(ui.Center).Children(func() {
				if uikit.IconButton(c, uikit.IconCopy, uikit.IconButtonProps{
					Key: "mcp-copy-config", Label: "复制客户端配置",
				}).Clicked() {
					mygo.Clipboard.WriteText(a.mcpClientConfigJSON())
					a.mcp.configModalOpen = true
				}
				if uikit.IconButton(c, uikit.IconEdit, uikit.IconButtonProps{
					Key: "mcp-edit-listen", Label: "编辑监听地址配置",
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
				{Label: "授权密钥", Icon: uikit.IconKey, Count: &activeKeys},
				{Label: "拦截策略", Icon: uikit.IconShield, Count: &enabledPolicies},
				{Label: "审计记录", Icon: uikit.IconDocument, Count: &auditCount},
			}
			uikit.Segmented(c, &a.mcp.activeTab, items, uikit.SegmentedProps{
				Label: "MCP 功能标签页", CountMax: 99,
			})
			ui.Spacer(c)
			switch a.mcp.activeTab {
			case 0:
				if uikit.Button(c, "新增 Key", uikit.ButtonProps{Variant: uikit.ButtonPrimary, Icon: uikit.IconPlus}).Clicked() {
					a.openNewMCPKeyModal()
				}
			case 1:
				if uikit.Button(c, "新增策略", uikit.ButtonProps{Variant: uikit.ButtonPrimary, Icon: uikit.IconPlus}).Clicked() {
					a.openNewMCPPolicyModal()
				}
			case 2:
				if uikit.Button(c, "保留最近 "+a.mcp.retentionDays+" 天 · 清理", uikit.ButtonProps{Variant: uikit.ButtonPlain, Size: uikit.SizeSm}).Clicked() {
					a.mcp.retentionOpen = true
				}
				if uikit.IconButton(c, uikit.IconRetry, uikit.IconButtonProps{Key: "audit-refresh", Label: "刷新审计记录", Size: uikit.SizeSm}).Clicked() {
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
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		uikit.TableViewport(c, &a.mcp.keysScroll, uikit.TableViewportProps{
			Label: "授权密钥列表", MinWidth: 1024, PaddingY: t.Space.N8,
			Header: func() {
				// Column header
				ui.Row(c).FillWidth().Height(t.C.FileList.HeaderHeight).Shrink(0).Padding(0, t.Space.N20).
					BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).
					AlignItems(ui.Center).Gap(t.Space.N10).Children(func() {
					uikit.Text(c, "Key 名称 / 前缀", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(210).Shrink(0)
					uikit.Text(c, "状态", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(86).Shrink(0)
					uikit.Text(c, "已授权连接与能力摘要", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Grow(1).MinWidth(0).SingleLine()
					uikit.Text(c, "本地目录范围", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(210).Shrink(0)
					uikit.Text(c, "最近使用 / 创建", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(150).Shrink(0)
				})
			},
		}, func() {
			ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
				for _, item := range pageItems {
					keyItem := item
					selected := a.mcp.selectedKeyID == keyItem.ID
					revoked := keyItem.State == "revoked"
					row := ui.Row(c).Key(keyItem.ID).FillWidth().Padding(t.Space.N10, t.Space.N10).
						Radius(t.Radius.Control).Gap(t.Space.N10).AlignItems(ui.Center).Role(ui.RoleRow)
					bg := ui.Transparent
					if selected {
						bg = uikit.Color(t.Color.State.SelectedAccent)
					} else if row.Hovered() {
						bg = uikit.Color(t.Color.State.Hover)
					}
					row.Background(bg)
					if revoked {
						row.Opacity(t.Opacity.Disabled)
					}
					row.OnClick(func() {
						a.mcp.selectedKeyID = keyItem.ID
					})
					var menuEntries []uikit.MenuEntry
					if !revoked {
						toggleLabel := "停用密钥"
						if keyItem.State == "disabled" {
							toggleLabel = "启用密钥"
						}
						menuEntries = []uikit.MenuEntry{
							{
								ID: "edit", Label: "编辑密钥", Icon: uikit.IconEdit,
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
								ID: "revoke", Label: "撤销密钥", Icon: uikit.IconTrash, Danger: true,
								OnChoose: func() {
									a.mcp.keyConfirm = mcpKeyConfirmDialog{
										Open: true, KeyID: keyItem.ID, Name: keyItem.Name, Action: "revoke",
									}
								},
							},
						}
					}
					row.Children(func() {
						ui.Column(c).Width(210).Shrink(0).Gap(t.Space.N2).Children(func() {
							uikit.Text(c, keyItem.Name, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
							uikit.Text(c, keyItem.ID+" · "+keyItem.Prefix, uikit.TextProps{
								Style: uikit.TextMono, Tone: uikit.ToneTertiary,
							}).SingleLine().Ellipsis("…")
						})
						// Col 2: Status pill
						ui.Box(c).Width(86).Shrink(0).AlignItems(ui.Start).Children(func() {
							st := uikit.StatusEnabled
							if keyItem.State == "disabled" {
								st = uikit.StatusDisabled
							} else if keyItem.State == "revoked" {
								st = uikit.StatusRevoked
							}
							uikit.StatusPill(c, st)
						})
						// Col 3: Grants summary
						ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
							uikit.Text(c, keyItem.GrantsSummary, uikit.TextProps{Style: uikit.TextSmall}).SingleLine().Ellipsis("…")
							uikit.Text(c, keyItem.PolicyHint, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						// Col 4: Local dirs
						ui.Column(c).Width(210).Shrink(0).Gap(t.Space.N2).Children(func() {
							upText := "未授权本地上传"
							if len(keyItem.UploadDirs) > 0 {
								upText = "上传 " + keyItem.UploadDirs[0]
							}
							downText := "未授权本地下载"
							if len(keyItem.DownloadDirs) > 0 {
								downText = "下载 " + keyItem.DownloadDirs[0]
							}
							if revoked {
								upText, downText = "—", "—"
							}
							uikit.Text(c, upText, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…")
							uikit.Text(c, downText, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						// Col 5: Last used + hover more button
						ui.Row(c).Width(150).Shrink(0).AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
								uikit.Text(c, keyItem.LastUsed, uikit.TextProps{Style: uikit.TextFootnote}).SingleLine().Ellipsis("…")
								uikit.Text(c, keyItem.CreatedAt, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
							})
							if len(menuEntries) > 0 {
								more := uikit.SiderMore(c, row, uikit.SiderMoreProps{
									Icon: moreIcon, Label: keyItem.Name + " 操作菜单",
								})
								uikit.RowMenuOn(c, row, more, menuEntries)
							}
						})
					})
				}
			})
		})
		uikit.Pagination(c, &a.mcp.keyPage, uikit.PaginationProps{
			Total: len(ordered), PageSize: 20,
			Summary: fmt.Sprintf("共 %d 条授权密钥", len(ordered)),
		})
	})
}

func (a *app) mcpPoliciesTab(c *ui.Context) {
	t := theme.Current()
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
		if a.mcp.policyConnFilter != "all" && !slices.Contains(p.Connections, a.mcp.policyConnFilter) {
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
	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		uikit.TableViewport(c, &a.mcp.policiesScroll, uikit.TableViewportProps{
			Label: "拦截策略", MinWidth: 1024, PaddingY: t.Space.N8,
			Header: func() {
				// Filter bar
				ui.Row(c).FillWidth().Shrink(0).Padding(t.Space.N8, t.Space.N14).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
					catItems := []uikit.SegItem{
						{Label: "全部", Count: &totalAll},
						{Label: "命令策略", Count: &totalSSH},
						{Label: "文件策略", Count: &totalFiles},
					}
					uikit.Segmented(c, &a.mcp.policyCategorySeg, catItems, uikit.SegmentedProps{Size: uikit.SizeSm, Label: "策略分类筛选"})
					uikit.FilterChip(c, "适用连接", &a.mcp.policyConnFilter, a.mcpConnectionOptions(-1, true), uikit.FilterChipProps{Key: "pol-conn-filter"})
					uikit.FilterChip(c, "适用密钥", &a.mcp.policyKeyFilter, a.mcpKeyOptions(true), uikit.FilterChipProps{Key: "pol-key-filter"})
					uikit.FilterChip(c, "状态", &a.mcp.policyStateFilter, []uikit.Option[string]{
						{Value: "all", Label: "全部"},
						{Value: "enabled", Label: "启用"},
						{Value: "disabled", Label: "已停用"},
					}, uikit.FilterChipProps{Key: "pol-state-filter"})
				})
				// Rule principle banner
				ui.Box(c).FillWidth().Shrink(0).Padding(0, t.Space.N14, t.Space.N8, t.Space.N14).Children(func() {
					ui.Row(c).FillWidth().Padding(t.Space.N6, t.Space.N10).Radius(t.Radius.ControlSm).
						Background(uikit.Color(t.Color.Surface.Sunken)).Border(t.Size.Hairline, uikit.Color(t.Color.Border.Subtle)).
						AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
						uikit.Icon(c, uikit.IconShield, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSecondary})
						uikit.Text(c, "执行原则：Key 已获连接与能力授权，且未命中任何启用的拦截策略时才允许执行（命中即拒绝，无允许覆盖或优先级）", uikit.TextProps{
							Style: uikit.TextCaption, Tone: uikit.ToneSecondary,
						}).SingleLine().Ellipsis("…")
					})
				})
				// Table header
				ui.Row(c).FillWidth().Height(t.C.FileList.HeaderHeight).Shrink(0).Padding(0, t.Space.N20).
					BorderWidth(t.Size.Divider, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).
					AlignItems(ui.Center).Gap(t.Space.N10).Children(func() {
					uikit.Text(c, "策略名称 / 用途", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(210).Shrink(0)
					uikit.Text(c, "分类", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(84).Shrink(0)
					uikit.Text(c, "适用连接", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(130).Shrink(0)
					uikit.Text(c, "适用密钥", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(115).Shrink(0)
					uikit.Text(c, "规则摘要", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Grow(1).MinWidth(0).SingleLine()
					uikit.Text(c, "状态 / 命中", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(130).Shrink(0)
				})
			},
		}, func() {
			ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
				for _, item := range pageItems {
					pol := item
					selected := a.mcp.selectedPolicyID == pol.ID
					row := ui.Row(c).Key(pol.ID).FillWidth().Padding(t.Space.N10, t.Space.N10).
						Radius(t.Radius.Control).Gap(t.Space.N10).AlignItems(ui.Center).Role(ui.RoleRow)
					bg := ui.Transparent
					if selected {
						bg = uikit.Color(t.Color.State.SelectedAccent)
					} else if row.Hovered() {
						bg = uikit.Color(t.Color.State.Hover)
					}
					row.Background(bg)
					row.OnClick(func() {
						a.mcp.selectedPolicyID = pol.ID
					})
					toggleLabel := "停用策略"
					if !pol.Enabled {
						toggleLabel = "启用策略"
					}
					entries := []uikit.MenuEntry{
						{
							ID: "edit", Label: "编辑策略", Icon: uikit.IconEdit,
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
							ID: "audit", Label: "命中审计", Icon: uikit.IconDocument,
							OnChoose: func() {
								a.mcp.activeTab = 2
								a.mcp.auditResultFilter = "intercepted"
								a.mcp.auditPolicyID, a.mcp.auditPage = pol.ID, 1
								a.mcp.auditKeyFilter, a.mcp.auditConnFilter, a.mcp.auditTypeFilter = "all", "all", "all"
							},
						},
						{ID: "sep", Separator: true},
						{
							ID: "delete", Label: "删除策略", Icon: uikit.IconTrash, Danger: true,
							OnChoose: func() {
								a.mcp.policyDelete = mcpPolicyDeleteDialog{
									Open: true, PolicyID: pol.ID, Name: pol.Name,
								}
							},
						},
					}
					row.Children(func() {
						ui.Column(c).Width(210).Shrink(0).Gap(t.Space.N2).Children(func() {
							uikit.Text(c, pol.Name, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
							uikit.Text(c, fmt.Sprintf("%s · v%d", pol.Description, pol.Version), uikit.TextProps{
								Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
							}).SingleLine().Ellipsis("…")
						})
						ui.Box(c).Width(84).Shrink(0).AlignItems(ui.Start).Children(func() {
							if pol.Category == "ssh" {
								uikit.Badge(c, "命令策略", uikit.BadgeProps{Tone: uikit.ToneWarning})
							} else {
								uikit.Badge(c, "文件策略", uikit.BadgeProps{Tone: uikit.ToneAccent})
							}
						})
						ui.Column(c).Width(130).Shrink(0).Gap(t.Space.N2).Children(func() {
							uikit.Text(c, pol.ConnSummary, uikit.TextProps{Style: uikit.TextFootnote}).SingleLine().Ellipsis("…")
							uikit.Text(c, pol.ConnSub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						ui.Column(c).Width(115).Shrink(0).Gap(t.Space.N2).Children(func() {
							keyTitle, keySub := "全部 Key", "含后续新建 Key"
							if !pol.AllKeys {
								keyTitle = strings.Join(pol.KeyNames, ", ")
								keySub = fmt.Sprintf("指定 %d 个 Key", len(pol.KeyNames))
							}
							uikit.Text(c, keyTitle, uikit.TextProps{Style: uikit.TextFootnote}).SingleLine().Ellipsis("…")
							uikit.Text(c, keySub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary}).SingleLine().Ellipsis("…")
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
							uikit.Text(c, pol.RulePrimary, uikit.TextProps{Style: uikit.TextMono}).SingleLine().Ellipsis("…")
							uikit.Text(c, pol.RuleSub, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…")
						})
						ui.Row(c).Width(130).Shrink(0).AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
								if pol.Enabled {
									uikit.StatusPill(c, uikit.StatusEnabled).AlignSelf(ui.Start)
								} else {
									uikit.StatusPill(c, uikit.StatusDisabled).AlignSelf(ui.Start)
								}
								tone := uikit.ToneTertiary
								if strings.Contains(pol.HitsSummary, "今日") {
									tone = uikit.ToneWarning
								}
								uikit.Text(c, pol.HitsSummary, uikit.TextProps{Style: uikit.TextCaption, Tone: tone}).SingleLine().Ellipsis("…")
							})
							more := uikit.SiderMore(c, row, uikit.SiderMoreProps{
								Icon: moreIcon, Label: pol.Name + " 菜单",
							})
							if uikit.RowMenuOn(c, row, more, entries) {
								a.mcp.selectedPolicyID = pol.ID
							}
						})
					})
				}
			})
		})
		uikit.Pagination(c, &a.mcp.policyPage, uikit.PaginationProps{
			Total: len(filtered), PageSize: 20,
			Summary: fmt.Sprintf("共 %d 条拦截策略", len(filtered)),
		})
	})
}

func (a *app) mcpAuditTab(c *ui.Context) {
	t := theme.Current()
	filtered := a.mcp.audits

	ui.Column(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		uikit.TableViewport(c, &a.mcp.auditScroll, uikit.TableViewportProps{
			Label: "审计记录", MinWidth: 1024, PaddingY: t.Space.N6,
			Header: func() {
				// Filter bar
				ui.Row(c).FillWidth().Shrink(0).Padding(t.Space.N8, t.Space.N14).Gap(t.Space.N8).AlignItems(ui.Center).Children(func() {
					uikit.FilterChip(c, "授权密钥", &a.mcp.auditKeyFilter, a.mcpKeyOptions(true), uikit.FilterChipProps{Key: "aud-key-filter"})
					uikit.FilterChip(c, "目标连接", &a.mcp.auditConnFilter, a.mcpConnectionOptions(-1, true), uikit.FilterChipProps{Key: "aud-conn-filter"})
					uikit.FilterChip(c, "操作类型", &a.mcp.auditTypeFilter, []uikit.Option[string]{
						{Value: "all", Label: "全部"},
						{Value: "目录上传", Label: "目录上传"},
						{Value: "命令执行", Label: "命令执行"},
						{Value: "文件读取", Label: "文件读取"},
						{Value: "策略管理", Label: "策略管理"},
						{Value: "认证失败", Label: "认证失败"},
					}, uikit.FilterChipProps{Key: "aud-type-filter"})
					uikit.FilterChip(c, "时间", &a.mcp.auditTimeFilter, []uikit.Option[string]{
						{Value: "today", Label: "今天"},
						{Value: "7d", Label: "最近 7 天"},
						{Value: "30d", Label: "最近 30 天"},
					}, uikit.FilterChipProps{Key: "aud-time-filter"})
					uikit.FilterChip(c, "结果", &a.mcp.auditResultFilter, []uikit.Option[string]{
						{Value: "all", Label: "全部"},
						{Value: "intercepted", Label: "被策略拦截"},
						{Value: "denied", Label: "权限拒绝 / 认证失败"},
						{Value: "done", Label: "成功"},
					}, uikit.FilterChipProps{Key: "aud-res-filter"})
				})
				// Table header
				ui.Row(c).FillWidth().Height(t.C.FileList.HeaderHeight).Shrink(0).Padding(0, t.Space.N20).
					BorderWidth(t.Size.Divider, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).
					AlignItems(ui.Center).Gap(t.Space.N10).Children(func() {
					uikit.Text(c, "时间", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(72).Shrink(0)
					uikit.Text(c, "Key 名称", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(148).Shrink(0)
					uikit.Text(c, "目标连接", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(120).Shrink(0)
					uikit.Text(c, "操作类型", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(96).Shrink(0)
					uikit.Text(c, "命令或路径摘要", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Grow(1).MinWidth(0).SingleLine()
					uikit.Text(c, "状态", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(104).Shrink(0)
					uikit.Text(c, "耗时", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).Width(56).Shrink(0).TextAlign(ui.End)
				})
			},
		}, func() {
			if len(filtered) == 0 {
				uikit.EmptyState(c, uikit.EmptyProps{
					IconName: uikit.IconDocument, Title: "筛选无结果",
					Detail: "当前筛选条件下没有匹配的审计记录。", Action: "重置筛选条件",
				}).Action.OnClick(func() {
					a.mcp.auditKeyFilter, a.mcp.auditConnFilter, a.mcp.auditTypeFilter, a.mcp.auditResultFilter = "all", "all", "all", "all"
				})
				return
			}
			ui.Column(c).FillWidth().Gap(t.Space.N2).Children(func() {
				for _, entry := range filtered {
					item := entry
					selected := a.mcp.selectedAuditID == item.ID && a.mcp.auditDrawerOpen
					row := ui.Row(c).Key(item.ID).FillWidth().Padding(t.Space.N8, t.Space.N10).
						Radius(t.Radius.Control).Gap(t.Space.N10).AlignItems(ui.Center).Role(ui.RoleRow)
					bg := ui.Transparent
					if selected {
						bg = uikit.Color(t.Color.State.SelectedAccent)
					} else if row.Hovered() {
						bg = uikit.Color(t.Color.State.Hover)
					}
					row.Background(bg)
					row.OnClick(func() {
						a.mcp.selectedAuditID = item.ID
						a.mcp.auditDrawerOpen = true
					})
					row.Children(func() {
						ui.Column(c).Width(72).Shrink(0).Gap(t.Space.N1).Children(func() {
							uikit.Text(c, item.Time, uikit.TextProps{Style: uikit.TextNumeric})
							uikit.Text(c, item.Day, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
						})
						uikit.Text(c, item.KeyName, uikit.TextProps{Style: uikit.TextBodyStrong}).
							Width(148).Shrink(0).SingleLine().Ellipsis("…")
						ui.Column(c).Width(120).Shrink(0).Gap(t.Space.N1).Children(func() {
							uikit.Text(c, item.ConnName, uikit.TextProps{Style: uikit.TextSmall}).SingleLine().Ellipsis("…")
							uikit.Text(c, item.ConnProtocol, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
						})
						ui.Row(c).Width(96).Shrink(0).Gap(t.Space.N4).AlignItems(ui.Center).Children(func() {
							icon := uikit.IconDocument
							if item.IsCommand {
								icon = uikit.IconTerminal
							} else if item.ActionType == "目录上传" {
								icon = uikit.IconUpload
							} else if item.ActionType == "策略管理" {
								icon = uikit.IconShield
							}
							uikit.Icon(c, icon, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneSecondary})
							uikit.Text(c, item.ActionType, uikit.TextProps{Style: uikit.TextSmall}).SingleLine()
						})
						uikit.Text(c, item.Summary, uikit.TextProps{Style: uikit.TextMono}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
						ui.Box(c).Width(104).Shrink(0).AlignItems(ui.Start).Children(func() {
							uikit.StatusPill(c, item.Status)
						})
						uikit.Text(c, item.Duration, uikit.TextProps{Style: uikit.TextNumeric, Tone: uikit.ToneSecondary}).
							Width(56).Shrink(0).TextAlign(ui.End)
					})
				}
			})
		})
		uikit.Pagination(c, &a.mcp.auditPage, uikit.PaginationProps{
			Total: a.mcp.auditFiltered, PageSize: 20,
			Summary: fmt.Sprintf("共 %d 条审计记录", a.mcp.auditFiltered),
		})
	})
}
