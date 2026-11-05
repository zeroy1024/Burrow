package desktop

import (
	"fmt"
	"maps"
	"net"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/shellcmd"
	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"
)

func (a *app) loadPolicyIntoDraft(pol *mcpPolicy) {
	if pol == nil {
		return
	}
	catSeg := 1
	if pol.Category == "ssh" {
		catSeg = 0
	}
	connections, keys := slices.Clone(pol.Connections), slices.Clone(pol.KeyIDs)
	if pol.AllConnections {
		connections = []string{}
	}
	if pol.AllKeys {
		keys = []string{}
	}
	var fileRules []mcpFileRule
	for _, r := range pol.FileRules {
		fileRules = append(fileRules, mcpFileRule{
			Side: r.Side, Kind: r.Kind, Path: r.Path, IncludeSubdir: r.IncludeSubdir,
			DeniedActions: maps.Clone(r.DeniedActions),
		})
	}
	if len(fileRules) == 0 {
		fileRules = []mcpFileRule{{
			Side: "remote", Kind: "dir", Path: "/var/www/shop/public/config", IncludeSubdir: true,
			DeniedActions: map[string]bool{"create": true, "write": true, "rename": true, "move": true, "delete": true},
		}}
	}
	cmdRules := slices.Clone(pol.CmdRules)
	if len(cmdRules) == 0 {
		cmdRules = []mcpCommandRule{
			{Mode: "command", Pattern: "rm -rf /"},
			{Mode: "command", Pattern: "mkfs.*"},
		}
	}
	a.mcp.policyDraft = mcpPolicyDraft{
		EditingID:    pol.ID,
		Version:      pol.Version,
		Name:         pol.Name,
		Description:  pol.Description,
		Enabled:      pol.Enabled,
		CategorySeg:  catSeg,
		Connections:  connections,
		SelectedKeys: keys,
		FileRules:    fileRules,
		CmdRules:     cmdRules,
	}
}

func (a *app) openNewMCPPolicyModal() {
	a.mcp.selectedPolicyID = ""
	a.mcp.policyModalOpen = true
	a.mcp.policyDraft = mcpPolicyDraft{
		Version:      1,
		Name:         "",
		Description:  "",
		Enabled:      true,
		CategorySeg:  0,
		Connections:  []string{},
		SelectedKeys: []string{},
		FileRules: []mcpFileRule{
			{
				Side: "remote", Kind: "dir", Path: "/etc/nginx/ssl", IncludeSubdir: true,
				DeniedActions: map[string]bool{"read": true, "write": true, "delete": true},
			},
		},
		CmdRules: []mcpCommandRule{
			{Mode: "command", Pattern: "rm -rf /"},
		},
	}
}

func (a *app) toggleMCPPolicy(id string) {
	a.toggleMCPBackendPolicy(id)
}

func (a *app) mcpPolicyModal(c *ui.Context) {
	if !a.mcp.policyModalOpen {
		return
	}
	t := theme.Current()
	draft := &a.mcp.policyDraft
	title := "编辑拦截策略"
	idBadge := fmt.Sprintf("%s · v%d", draft.EditingID, draft.Version)
	if draft.EditingID == "" {
		title = "新增拦截策略"
		idBadge = "草稿 · v1"
	}

	ruleInvalid := false
	if draft.CategorySeg == 0 {
		for _, r := range draft.CmdRules {
			ruleInvalid = ruleInvalid || commandRuleError(r) != ""
		}
	}

	saveRequested := false
	dialog := uikit.Modal(c.Key("mcp-policy-modal"), &a.mcp.policyModalOpen, uikit.ModalProps{
		Controller: &draft.modal,
		Disabled:   a.mcpBusy(),
		Title:      title, Subtitle: idBadge, Icon: uikit.IconSVG(uikit.IconShield),
		Width: t.C.Mcp.PolicyModalWidth, CloseLabel: "关闭策略窗口", Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Children(func() {
				if draft.CategorySeg == 1 {
					uikit.Text(c, "勾选即禁止；改名、移动、删除工具暂未提供。", uikit.TextProps{
						Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
					}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				} else {
					ui.Spacer(c)
				}
				if uikit.Button(c, "取消", uikit.ButtonProps{}).Clicked() {
					parts.RequestDismiss()
				}
				saveRequested = uikit.Button(c, "保存策略", uikit.ButtonProps{
					Variant: uikit.ButtonPrimary, Disabled: ruleInvalid || a.mcpBusy(),
				}).Clicked()
			})
		},
	}, func(parts uikit.ModalParts) {
		p := t.C.Modal
		headerHeight := p.HeaderPaddingTop + p.IconTile + p.HeaderPaddingBottom
		footerHeight := p.FooterPaddingTop + t.C.Button.Height + p.FooterPaddingBottom
		scroller := ui.Scroll(c).Key("mcp-policy-form-scroll").FillWidth().MinHeight(0).Shrink(1).
			MaxHeight(parts.Bounds.MaxHeight - headerHeight - footerHeight).TrackScroll(&draft.Scroll).
			Children(func() {
				ui.Column(c).FillWidth().Padding(t.Space.N12, p.BodyPaddingX).Gap(t.Space.N14).
					Children(func() { a.mcpPolicyFields(c) })
			})
		uikit.Scrollbars(c, parts.Panel, scroller, &draft.Scroll, uikit.ScrollbarProps{Label: "策略表单滚动条"})
	})
	if (saveRequested || dialog.Submit) && !ruleInvalid {
		a.saveMCPPolicyDraft()
	}
}

func (a *app) mcpPolicyFields(c *ui.Context) {
	t := theme.Current()
	draft := &a.mcp.policyDraft
	if draft.SaveError != "" {
		uikit.Banner(c, draft.SaveError, uikit.BannerProps{Tone: uikit.ToneDanger, Icon: uikit.IconSVG(uikit.IconAlertCircle)})
	}
	// 1. 基本信息与适用范围
	ui.Column(c).FillWidth().Gap(t.Space.N8).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			uikit.Text(c, "基本信息与适用范围", uikit.TextProps{Style: uikit.TextBodyStrong})
			ui.Spacer(c)
			uikit.Text(c, fmt.Sprintf("保存后更新至 v%d，启用后参与拦截", draft.Version+1), uikit.TextProps{
				Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
			})
		})
		uikit.FormRow(c, "策略名称", uikit.FormRowProps{Error: draft.NameError}, func() ui.Element {
			var name ui.Element
			ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
				name = uikit.Input(c, &draft.Name, uikit.InputProps{
					Label: "策略名称", Placeholder: "例如：生产配置保护", Error: draft.NameError,
				}).Grow(1)
				uikit.Text(c, "启用", uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
				uikit.Switch(c, &draft.Enabled, uikit.ToggleProps{
					Label: "启用该策略", Motion: uikit.MotionOverride(a.reduceMotion(c)),
				})
			})
			if name.Changed() && draft.NameError != "" {
				draft.NameError = ""
				c.Invalidate()
			}
			return name
		})
		uikit.FormRow(c, "用途说明", uikit.FormRowProps{}, func() ui.Element {
			return uikit.Input(c, &draft.Description, uikit.InputProps{
				Label: "用途说明", Placeholder: "请输入拦截目的与保护范围",
			})
		})
		uikit.FormRow(c, "策略分类", uikit.FormRowProps{Align: uikit.RowAlignCenter}, func() ui.Element {
			var category ui.Element
			ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
				previousCategory := draft.CategorySeg
				category = uikit.Segmented(c, &draft.CategorySeg, uikit.SegmentItems([]string{"命令策略", "文件策略"}), uikit.SegmentedProps{
					Size: uikit.SizeSm, Label: "策略分类",
				})
				if previousCategory != draft.CategorySeg {
					draft.Connections = []string{}
				}
				uikit.Text(c, "仅显示当前分类支持的连接", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
			})
			return category
		})
		uikit.FormRow(c, "适用连接", uikit.FormRowProps{}, func() ui.Element {
			candidates := a.mcpConnectionOptions(draft.CategorySeg, false)
			return uikit.MultiSelect(c.Key("policy-connections"), &draft.Connections, candidates, uikit.MultiSelectProps{
				Label: "适用连接", Placeholder: "全部连接（留空默认全部）",
			})
		})
		uikit.FormRow(c, "适用密钥", uikit.FormRowProps{}, func() ui.Element {
			options := []uikit.Option[string]{}
			for _, k := range a.mcp.keys {
				if k.State == "revoked" {
					continue
				}
				options = append(options, uikit.Option[string]{Value: k.ID, Label: k.Name})
			}
			return uikit.MultiSelect(c.Key("policy-keys"), &draft.SelectedKeys, options, uikit.MultiSelectProps{
				Label: "适用密钥", Placeholder: "全部密钥（留空默认全部）",
			})
		})
	})

	uikit.Divider(c, uikit.DividerProps{})

	// 2. 文件操作规则编辑器
	if draft.CategorySeg == 1 {
		ui.Column(c).FillWidth().Gap(t.Space.N8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				uikit.Text(c, "文件拦截规则", uikit.TextProps{Style: uikit.TextBodyStrong})
				ui.Spacer(c)
				if uikit.Button(c, "+ 添加路径规则", uikit.ButtonProps{Size: uikit.SizeSm}).Clicked() {
					draft.FileRules = append(draft.FileRules, mcpFileRule{
						Side: "remote", Kind: "dir", Path: "/data/protected", IncludeSubdir: true,
						DeniedActions: map[string]bool{"write": true, "delete": true},
					})
				}
			})
			actions := uikit.DefaultFilePolicyActions()
			remove := -1
			for idx := range draft.FileRules {
				ruleIdx := idx
				rule := &draft.FileRules[ruleIdx]
				uikit.Inset(c).Key(ruleIdx).FillWidth().Gap(t.Space.N8).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						uikit.Text(c, fmt.Sprintf("规则 #%d", ruleIdx+1), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
						ui.Spacer(c)
						if len(draft.FileRules) > 1 {
							if uikit.Button(c.Key("del-f"), "删除", uikit.ButtonProps{Variant: uikit.ButtonDangerPlain, Size: uikit.SizeSm}).Clicked() {
								remove = ruleIdx
							}
						}
					})
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
						uikit.FilterChip(c, "位置", &rule.Side, []uikit.Option[string]{
							{Value: "remote", Label: "远端路径"},
							{Value: "local", Label: "本地路径"},
						}, uikit.FilterChipProps{Key: fmt.Sprintf("side-%d", ruleIdx)})
						uikit.FilterChip(c, "类型", &rule.Kind, []uikit.Option[string]{
							{Value: "dir", Label: "目录"},
							{Value: "file", Label: "文件"},
						}, uikit.FilterChipProps{Key: fmt.Sprintf("kind-%d", ruleIdx)})
						uikit.Input(c.Key(fmt.Sprintf("path-%d", ruleIdx)), &rule.Path, uikit.InputProps{
							Mono: true, Label: "受保护路径", Placeholder: "/var/www/shop/public/config",
						}).Grow(1)
						if rule.Kind == "dir" {
							uikit.Checkbox(c.Key(fmt.Sprintf("sub-%d", ruleIdx)), &rule.IncludeSubdir, "包含子目录")
						}
					})
					uikit.PolicyActionMatrix(c.Key(fmt.Sprintf("act-%d", ruleIdx)), rule.DeniedActions, actions, func(id string, checked bool) {
						if rule.DeniedActions == nil {
							rule.DeniedActions = map[string]bool{}
						}
						rule.DeniedActions[id] = checked
					})
				})
			}
			if remove >= 0 {
				draft.FileRules = slices.Delete(draft.FileRules, remove, remove+1)
				c.Invalidate()
			}
		})
	}

	// 3. SSH 命令策略规则编辑器（含正则语法实时校验）
	if draft.CategorySeg == 0 {
		ui.Column(c).FillWidth().Gap(t.Space.N8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				uikit.Text(c, "命令拦截规则", uikit.TextProps{Style: uikit.TextBodyStrong})
				ui.Spacer(c)
				if uikit.Button(c, "+ 添加命令规则", uikit.ButtonProps{Size: uikit.SizeSm}).Clicked() {
					draft.CmdRules = append(draft.CmdRules, mcpCommandRule{Mode: "command", Pattern: "shutdown"})
				}
			})
			remove := -1
			for idx := range draft.CmdRules {
				cmdIdx := idx
				cmdRule := &draft.CmdRules[cmdIdx]
				ruleErr := commandRuleError(*cmdRule)
				uikit.Inset(c).Key(cmdIdx).FillWidth().Gap(t.Space.N4).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
						uikit.Select(c.Key(fmt.Sprintf("cmode-%d", cmdIdx)), &cmdRule.Mode, []uikit.Option[string]{
							{Value: "command", Label: "命令匹配"},
							{Value: "exact", Label: "精确匹配"},
							{Value: "regex", Label: "正则匹配"},
							{Value: "contains", Label: "模糊匹配"},
						}, uikit.SelectProps{Label: "匹配方式"}).
							Width(t.C.Mcp.RuleModeWidth).Shrink(0)
						uikit.Input(c.Key(fmt.Sprintf("cpat-%d", cmdIdx)), &cmdRule.Pattern, uikit.InputProps{
							Mono: true, Label: "匹配内容", Placeholder: "rm -rf /", Error: ruleErr,
						}).Grow(1)
						uikit.Badge(c, "命中拦截", uikit.BadgeProps{Tone: uikit.ToneDanger})
						if len(draft.CmdRules) > 1 {
							if uikit.Button(c.Key("del-c"), "删除", uikit.ButtonProps{Variant: uikit.ButtonDangerPlain, Size: uikit.SizeSm}).Clicked() {
								remove = cmdIdx
							}
						}
					})
					switch {
					case ruleErr != "":
						uikit.FieldMessage(c, ruleErr, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
					case cmdRule.Mode == "contains":
						uikit.FieldMessage(c, "模糊匹配按子串判定，rm 也会命中 format 等词，建议改用命令匹配。", uikit.FieldMessageProps{Tone: uikit.ToneWarning})
					}
					if cmdRule.Mode == "command" {
						uikit.Checkbox(c.Key(fmt.Sprintf("cicase-%d", cmdIdx)), &cmdRule.IgnoreCase, "参数忽略大小写")
					}
				})
			}
			if remove >= 0 {
				draft.CmdRules = slices.Delete(draft.CmdRules, remove, remove+1)
				c.Invalidate()
			}
			uikit.Text(c, "命令匹配按命令名和参数判定，可识别 &&、管道、sudo、bash -c、ssh 等组合写法；精确匹配完整命令；正则匹配表达式；模糊匹配包含文本，区分大小写。", uikit.TextProps{
				Style: uikit.TextCaption, Tone: uikit.ToneSecondary,
			})
			uikit.Text(c, "命令策略用于防误操作：无法解析的部分按关键词从严拦截，但不检查脚本文件和解释器（如 python -c）内部的操作，也不能限制已授权命令的文件访问。", uikit.TextProps{
				Style: uikit.TextCaption, Tone: uikit.ToneSecondary,
			})
		})
	}
}

func (a *app) saveMCPPolicyDraft() {
	a.saveMCPBackendPolicy()
}

func (a *app) currentMCPAudit() *mcpAuditItem {
	if a.mcp.detail != nil && a.mcp.detail.ID == a.mcp.selectedAuditID {
		return a.mcp.detail
	}
	for i := range a.mcp.audits {
		if a.mcp.audits[i].ID == a.mcp.selectedAuditID {
			return &a.mcp.audits[i]
		}
	}
	if a.mcp.selectedAuditID == "" && len(a.mcp.audits) > 0 {
		return &a.mcp.audits[0]
	}
	return nil
}

func (a *app) mcpAuditDrawer(c *ui.Context) {
	item := a.currentMCPAudit()
	if item == nil {
		return
	}
	t := theme.Current()
	panel := uikit.SidePanel(c, uikit.SidePanelProps{
		Title:           "操作详情",
		Width:           a.mcp.auditDrawerWidth,
		Scroll:          &a.mcp.drawerScroll,
		ContentPaddingX: t.Space.N14,
		Header: func() bool {
			closed := false
			ui.Row(c).FillWidth().Height(t.C.Panel.HeaderHeight).Padding(0, t.Space.N14).
				BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).
				AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
				uikit.Text(c, "操作详情", uikit.TextProps{Style: uikit.TextHeadline})
				if uikit.CopyField(c, item.ID, uikit.CopyFieldProps{Key: "copy-op-id", Label: "复制操作 ID"}).Copied {
					mygo.Clipboard.WriteText(item.ID)
				}
				uikit.StatusPill(c, item.Status)
				ui.Spacer(c)
				if uikit.IconButton(c, closeIcon, uikit.IconButtonProps{Key: "close-audit-drawer", Label: "关闭详情抽屉", Size: uikit.SizeSm}).Clicked() {
					closed = true
				}
			})
			return closed
		},
	}, func() {
		ui.Column(c).FillWidth().Padding(t.Space.N12, 0).Gap(t.Space.N12).Children(func() {
			// 1. 基本信息
			uikit.Text(c, "基本信息", uikit.TextProps{Style: uikit.TextBodyStrong})
			uikit.KeyValueGrid(c, 2, []uikit.KV{
				{
					Key: "密钥名称",
					Render: func() {
						uikit.Text(c, item.KeyName, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
					},
				},
				{Key: "密钥标识", Value: item.KeyID, ValueMono: true},
				{Key: "开始时间", Value: item.StartTime},
				{Key: "结束时间", Value: item.EndTime},
				{Key: "操作类型", Value: item.ActionType},
				{
					Key: "结果",
					Render: func() {
						uikit.StatusPill(c, item.Status)
					},
				},
			})

			// 2. 目标快照
			if item.ConnectionID != "" {
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					uikit.Text(c, "目标快照", uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Spacer(c)
					uikit.Text(c, "保留调用时的连接信息，后续修改不影响", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
				})
				uikit.KeyValueGrid(c, 2, []uikit.KV{
					{Key: "主机", Value: item.ConnHost + " (" + item.ConnName + ")", ValueMono: true},
					{Key: "端口", Value: item.ConnPort, ValueMono: true},
					{Key: "账号", Value: item.ConnUser, ValueMono: true},
					{Key: "协议", Value: item.ConnProtocol},
				})
			}

			// 3. 命令信息 或 文件信息
			if item.IsCommand {
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
					uikit.Text(c, "命令信息", uikit.TextProps{Style: uikit.TextBodyStrong})
					warnIcon := uikit.Icon(c, uikit.IconInfo, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneWarning})
					uikit.Tip(c, warnIcon, "命令原文可能包含敏感参数；审计按本地敏感数据管理，不承诺已自动识别全部秘密。")
					uikit.Text(c, "命令可能含敏感参数，请妥善保管审计记录", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneWarning})
				})
				if uikit.CodeBlock(c.Key("command-source"), "命令原文", item.CommandText).Copy {
					mygo.Clipboard.WriteText(item.CommandText)
				}
				uikit.KeyValue(c, []uikit.KV{
					{Key: "退出码", Value: item.ExitCodeText, ValueMono: true},
					{Key: "输出与状态", Value: item.OutputStatus},
				})
			} else if item.LocalPath != "" || item.RemotePath != "" {
				uikit.Divider(c, uikit.DividerProps{})
				uikit.Text(c, "文件信息", uikit.TextProps{Style: uikit.TextBodyStrong})
				var fileKVs []uikit.KV
				if item.LocalPath != "" {
					fileKVs = append(fileKVs, uikit.KV{Key: "本地路径", Value: item.LocalPath, ValueMono: true})
				}
				if item.RemotePath != "" {
					fileKVs = append(fileKVs, uikit.KV{Key: "远端路径", Value: item.RemotePath, ValueMono: true})
				}
				if item.TransferStat != "" {
					fileKVs = append(fileKVs, uikit.KV{Key: "传输统计", Value: item.TransferStat})
				}
				uikit.KeyValue(c, fileKVs)
			}

			uikit.Divider(c, uikit.DividerProps{})
			if !item.DetailLoaded {
				uikit.Text(c, "正在读取请求与响应…", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
			} else {
				if !item.PayloadSaved {
					uikit.Banner(c, "该记录未保存完整请求与响应正文，以下仅显示已有摘要。", uikit.BannerProps{Tone: uikit.ToneNeutral})
				}
				if item.FileBodyOmitted {
					uikit.Banner(c, "文件正文不写入审计；请求与响应中保留路径、字节数及处理结果。", uikit.BannerProps{Tone: uikit.ToneNeutral})
				}
				if uikit.CodeBlock(c.Key("audit-request"), "请求数据", item.RequestJSON).Copy {
					mygo.Clipboard.WriteText(item.RequestJSON)
				}
				if item.IsCommand && item.ResponseSaved && !item.ResponseTruncated {
					ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
						uikit.Segmented(c, &a.mcp.auditResponseTab, uikit.SegmentItems([]string{"输出", "JSON"}), uikit.SegmentedProps{
							Label: "响应查看方式", Size: uikit.SizeSm,
						})
					})
				}
				if item.ResponseTruncated {
					uikit.Banner(c, "响应超过审计采集上限，正文未保存；请查看响应摘要中的大小信息。", uikit.BannerProps{Tone: uikit.ToneWarning})
				}
				if !item.ResponseSaved && item.PayloadSaved {
					uikit.Text(c, "响应尚未记录；调用可能仍在进行，或在返回前中断。", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary})
				} else if item.IsCommand && item.ResponseSaved && !item.ResponseTruncated && a.mcp.auditResponseTab == 0 {
					if uikit.CodeBlock(c.Key("audit-stdout"), "标准输出 stdout", item.Stdout).Copy {
						mygo.Clipboard.WriteText(item.Stdout)
					}
					if uikit.CodeBlock(c.Key("audit-stderr"), "标准错误 stderr", item.Stderr).Copy {
						mygo.Clipboard.WriteText(item.Stderr)
					}
				} else if uikit.CodeBlock(c.Key("audit-response"), "响应数据", item.ResponseJSON).Copy {
					mygo.Clipboard.WriteText(item.ResponseJSON)
				}
			}

			// 4. 命中拦截策略快照卡片
			if len(item.PolicyHits) > 0 {
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					uikit.Text(c, "命中拦截策略", uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Spacer(c)
					uikit.Text(c, "保留判定时的策略规则，后续修改不影响", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
				})
				for i, hit := range item.PolicyHits {
					uikit.Inset(c, uikit.InsetProps{Tone: uikit.ToneWarning}).Key(i).FillWidth().Gap(t.Space.N6).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
							uikit.Badge(c, hit.Category, uikit.BadgeProps{Tone: uikit.ToneWarning})
							uikit.Text(c, hit.PolicyName, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
							uikit.Text(c, fmt.Sprintf("%s · v%d", hit.PolicyID, hit.Version), uikit.TextProps{
								Style: uikit.TextMono, Tone: uikit.ToneSecondary,
							})
							ui.Spacer(c)
							uikit.Badge(c, hit.HitCountText, uikit.BadgeProps{Tone: uikit.ToneWarning, Dot: true})
						})
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
							uikit.Text(c, "命中规则："+hit.RuleSummary, uikit.TextProps{Style: uikit.TextMono}).Grow(1).MinWidth(0)
							uikit.Text(c, hit.ForbiddenText, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneDanger})
						})
					})
				}
			}

			// 5. 关联任务卡片（含进度条、暂停/继续/取消 与 黄色等待用户确认块）
			if item.Task != nil {
				task := item.Task
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					uikit.Text(c, "关联任务", uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Spacer(c)
					uikit.Text(c, task.ID+" · 客户端断开后继续运行", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
				})
				uikit.Inset(c).FillWidth().Gap(t.Space.N8).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
						uikit.IconTile(c, uikit.IconTileProps{Size: uikit.SizeSm, Background: t.Color.Accent.Subtle}, func() {
							uikit.Icon(c, uikit.IconUpload, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneAccent})
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
							uikit.Text(c, task.Title, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
							uikit.Text(c, task.Subtitle, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).SingleLine().Ellipsis("…")
						})
						pauseLabel := "暂停"
						pauseIcon := uikit.IconPause
						if task.Paused {
							pauseLabel = "继续"
							pauseIcon = uikit.IconPlay
						}
						if task.CleanupOnly {
							pauseLabel = "重试清理"
						}
						if uikit.Button(c.Key("task-pause"), pauseLabel, uikit.ButtonProps{
							Size: uikit.SizeSm, Icon: pauseIcon, Disabled: task.CancelRequest,
						}).Clicked() {
							action := "pause"
							if task.Paused {
								action = "resume"
							}
							if task.CleanupOnly {
								action = "cancel"
							}
							a.controlMCPBackendTask(task.ID, action)
						}
						cancelLabel := "取消"
						if task.CancelRequest {
							cancelLabel = "已请求取消"
						}
						if !task.CleanupOnly && uikit.Button(c.Key("task-cancel"), cancelLabel, uikit.ButtonProps{
							Variant: uikit.ButtonDangerPlain, Size: uikit.SizeSm, Disabled: task.CancelRequest,
						}).Clicked() {
							a.controlMCPBackendTask(task.ID, "cancel")
						}
					})
					progTone := uikit.ProgressRunning
					if task.Paused {
						progTone = uikit.ProgressPaused
					}
					uikit.Progress(c, uikit.ProgressProps{
						Value: task.Fraction, Tone: progTone, Label: "任务传输进度",
						Motion: uikit.MotionOverride(a.reduceMotion(c)),
					}).FillWidth()
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						uikit.Text(c, fmt.Sprintf("%s / %s   %d%%", task.Transferred, task.Total, int(task.Fraction*100)), uikit.TextProps{
							Style: uikit.TextFootnote,
						}).FontFeatures("tnum")
						ui.Spacer(c)
						uikit.Text(c, task.Speed, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneAccent}).FontFeatures("tnum")
					})
					if task.WaitingPrompt != "" {
						res := uikit.Banner(c, task.WaitingPrompt, uikit.BannerProps{
							Tone:   uikit.ToneWarning,
							Icon:   uikit.IconSVG(uikit.IconAlertCircle),
							Action: "已确认并隐藏",
						})
						if res.Action {
							a.window.Show()
						}
					}
				})
			}

			// 6. 生命周期时间线
			if len(item.Timeline) > 0 {
				uikit.Divider(c, uikit.DividerProps{})
				uikit.Text(c, "生命周期", uikit.TextProps{Style: uikit.TextBodyStrong})
				uikit.Timeline(c, item.Timeline)
			}

			// 7. 逐文件明细
			if len(item.Files) > 0 {
				uikit.Divider(c, uikit.DividerProps{})
				uikit.Disclosure(c.Key("audit-files"), &a.mcp.auditFilesOpen, uikit.DisclosureProps{
					Title: "逐文件明细", TitleStyle: uikit.TextBodyStrong, Meta: fmt.Sprintf("%d 个文件", item.FileTotal),
					Motion: uikit.MotionOverride(a.reduceMotion(c)),
				}, func() {
					ui.Column(c).FillWidth().Gap(t.Space.N12).Children(func() {
						cntAll, cntOk, cntBlock, cntFail, cntSkip, cntPend := item.FileTotal, item.FileCounts["completed"], item.FileCounts["intercepted"], item.FileCounts["failed"], item.FileCounts["skipped"], item.FileCounts["running"]+item.FileCounts["pending"]+item.FileCounts["interrupted"]
						filterItems := []uikit.SegItem{
							{Label: "全部", Count: &cntAll},
							{Label: "成功", Count: &cntOk},
							{Label: "被拦截", Count: &cntBlock},
							{Label: "失败", Count: &cntFail},
							{Label: "跳过", Count: &cntSkip},
							{Label: "未完成", Count: &cntPend},
						}
						uikit.Segmented(c, &a.mcp.auditFileFilter, filterItems, uikit.SegmentedProps{Size: uikit.SizeSm, Label: "逐文件结果筛选"})
						ui.Column(c).FillWidth().Gap(t.Space.N4).Children(func() {
							for idx, f := range item.Files {
								if a.mcp.auditFileFilter == 1 && f.State != "ok" {
									continue
								}
								if a.mcp.auditFileFilter == 2 && f.State != "intercepted" {
									continue
								}
								if a.mcp.auditFileFilter == 3 && f.State != "failed" {
									continue
								}
								if a.mcp.auditFileFilter == 4 && f.State != "skipped" {
									continue
								}
								if a.mcp.auditFileFilter == 5 && f.State != "pending" {
									continue
								}
								rowTone := uikit.ToneNeutral
								icon := uikit.IconCheck
								tone := uikit.ToneSuccess
								stateLabel := "成功"
								switch f.State {
								case "intercepted":
									rowTone = uikit.ToneWarning
									icon = uikit.IconBan
									tone = uikit.ToneWarning
									stateLabel = "被策略拦截"
								case "failed":
									rowTone = uikit.ToneDanger
									icon = uikit.IconClose
									tone = uikit.ToneDanger
									stateLabel = "失败"
								case "skipped":
									icon = uikit.IconArrowRight
									tone = uikit.ToneSecondary
									stateLabel = "跳过"
								case "pending":
									icon = uikit.IconActivity
									tone = uikit.ToneAccent
									stateLabel = "未完成"
								}
								uikit.Inset(c, uikit.InsetProps{Tone: rowTone, Compact: true}).Key(idx).FillWidth().Children(func() {
									ui.Row(c).FillWidth().Gap(t.Space.N6).Children(func() {
										uikit.Icon(c, icon, uikit.IconProps{Size: uikit.SizeSm, Tone: tone})
										uikit.Text(c, f.Path, uikit.TextProps{Style: uikit.TextMono}).SingleLine().Ellipsis("…")
										if f.PolicyTag != "" {
											uikit.Badge(c, f.PolicyTag, uikit.BadgeProps{Tone: uikit.ToneWarning})
										}
										ui.Spacer(c)
										uikit.Text(c, f.Size, uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary}).FontFeatures("tnum")
										uikit.Text(c, stateLabel, uikit.TextProps{Style: uikit.TextCaption, Tone: tone})
									})
								})
							}
						})
					})
				})
			}
		})
	})
	if panel.Close {
		a.mcp.auditDrawerOpen = false
		c.Invalidate()
	}
}

func (a *app) openNewMCPKeyModal() {
	a.mcp.keyModal = mcpKeyModalDraft{
		Open:         true,
		Name:         "",
		Grants:       a.mcpGrants(),
		UploadDirs:   []string{},
		DownloadDirs: []string{},
	}
	// Default all grants to false as required by the security spec ("默认全关，按连接显式开启").
	for i := range a.mcp.keyModal.Grants {
		a.mcp.keyModal.Grants[i].AllowRead = false
		a.mcp.keyModal.Grants[i].AllowWrite = false
		a.mcp.keyModal.Grants[i].AllowExec = false
	}
}

func (a *app) openEditMCPKeyModal(id string) {
	for _, k := range a.mcp.keys {
		if k.ID == id {
			grants := slices.Clone(k.Grants)
			if len(grants) == 0 {
				grants = a.mcpGrants()
			}
			a.mcp.keyModal = mcpKeyModalDraft{
				Open:         true,
				EditingID:    k.ID,
				Name:         k.Name,
				Grants:       grants,
				UploadDirs:   slices.Clone(k.UploadDirs),
				DownloadDirs: slices.Clone(k.DownloadDirs),
			}
			return
		}
	}
}

func (a *app) setMCPKeyState(id, state string) {
	a.setMCPBackendKeyState(id, state, nil)
}

func (a *app) pickMCPDirectory(forUpload bool) {
	window := a.window
	if window == nil {
		return
	}
	title := "选择下载目标目录"
	if forUpload {
		title = "选择上传来源目录"
	}
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: title, Directory: true})
		window.Update(func() {
			if err != nil || len(paths) == 0 {
				return
			}
			if forUpload {
				if !slices.Contains(a.mcp.keyModal.UploadDirs, paths[0]) {
					a.mcp.keyModal.UploadDirs = append(a.mcp.keyModal.UploadDirs, paths[0])
				}
			} else {
				if !slices.Contains(a.mcp.keyModal.DownloadDirs, paths[0]) {
					a.mcp.keyModal.DownloadDirs = append(a.mcp.keyModal.DownloadDirs, paths[0])
				}
			}
		})
	}()
}

func (a *app) mcpDialogs(c *ui.Context) {
	a.mcpPolicyModal(c)
	a.mcpPolicyDeleteModal(c)
	a.mcpKeyModal(c)
	a.mcpKeyConfirmModal(c)
	a.mcpListenModal(c)
	a.mcpRetentionModal(c)
	a.mcpClientConfigModal(c)
}

func (a *app) mcpKeyModal(c *ui.Context) {
	if !a.mcp.keyModal.Open {
		return
	}
	t := theme.Current()
	draft := &a.mcp.keyModal
	title := "新增授权密钥"
	subtitle := "设置密钥名称、连接权限与本地目录"
	if draft.EditingID != "" {
		title = "编辑授权密钥"
		subtitle = draft.EditingID
	}
	if draft.OneTimeView {
		title = "授权密钥已生成"
		subtitle = "密钥仅显示一次，请复制保存后勾选确认"
	}

	uikit.Modal(c.Key("mcp-key-modal"), &draft.Open, uikit.ModalProps{
		Controller:      &draft.modal,
		Title:           title,
		Subtitle:        subtitle,
		Icon:            uikit.IconSVG(uikit.IconKey),
		Width:           t.C.Mcp.KeyModalWidth,
		Disabled:        a.mcpBusy(),
		DismissDisabled: draft.OneTimeView && !draft.SavedConfirmed,
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Children(func() {
				if draft.OneTimeView {
					uikit.Text(c, "密钥丢失后，请撤销旧密钥并生成新密钥。", uikit.TextProps{
						Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
					}).Grow(1)
					if uikit.Button(c, "完成并关闭", uikit.ButtonProps{
						Variant:  uikit.ButtonPrimary,
						Disabled: !draft.SavedConfirmed,
					}).Clicked() {
						parts.RequestDismiss()
					}
					return
				}
				uikit.Text(c, "密钥明文仅在创建时显示，请及时复制保存。", uikit.TextProps{
					Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
				}).Grow(1)
				if uikit.Button(c, "取消", uikit.ButtonProps{}).Clicked() {
					parts.RequestDismiss()
				}
				saveLabel := "生成密钥"
				if draft.EditingID != "" {
					saveLabel = "保存密钥"
				}
				if uikit.Button(c, saveLabel, uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Clicked() {
					a.submitMCPKeyModal()
				}
			})
		},
	}, func(parts uikit.ModalParts) {
		if draft.OneTimeView {
			uikit.ModalBody(c).Children(func() {
				uikit.OneTimeSecretCard(c, uikit.OneTimeSecretProps{
					KeyName:      draft.Name,
					Secret:       draft.GeneratedKey,
					ConfigJSON:   a.mcpClientConfigJSON(),
					Confirmed:    &draft.SavedConfirmed,
					OnCopySecret: func() { mygo.Clipboard.WriteText(draft.GeneratedKey) },
					OnCopyConfig: func() { mygo.Clipboard.WriteText(a.mcpClientConfigJSON()) },
				})
			})
			return
		}
		viewport := ui.Box(c).FillWidth().MaxHeight(parts.Bounds.MaxHeight - 150).MinHeight(0)
		viewport.Children(func() {
			scroller := ui.Scroll(c).Fill().TrackScroll(&draft.Scroll).Padding(t.Space.N8, t.C.Modal.BodyPaddingX, t.Space.N16)
			scroller.Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space.N14).Children(func() {
					// 第 1 段 · 名称
					ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
						uikit.Badge(c, "第 1 段 · 密钥名称", uikit.BadgeProps{Tone: uikit.ToneAccent})
						uikit.FormRow(c, "密钥名称", uikit.FormRowProps{Error: draft.NameError}, func() ui.Element {
							var name ui.Element
							ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
								name = uikit.Input(c, &draft.Name, uikit.InputProps{
									Label: "密钥名称", Placeholder: "例如：claude-desktop", Error: draft.NameError,
								}).Grow(1)
								uikit.Text(c, "建议每个客户端使用独立密钥", uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary})
							})
							if name.Changed() && draft.NameError != "" {
								draft.NameError = ""
								c.Invalidate()
							}
							return name
						})
					})

					// 第 2 段 · 连接授权矩阵
					ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
							uikit.Badge(c, "第 2 段 · 连接授权", uikit.BadgeProps{Tone: uikit.ToneAccent})
							ui.Spacer(c)
							uikit.Text(c, "默认不授予权限，启用后仍受拦截策略限制", uikit.TextProps{
								Style: uikit.TextCaption, Tone: uikit.ToneWarning,
							})
						})
						var matrixRows []uikit.PermissionMatrixRow
						for _, g := range draft.Grants {
							matrixRows = append(matrixRows, uikit.PermissionMatrixRow{
								ID: g.ConnectionID, Name: g.Name, Subtitle: g.Subtitle, Warning: g.Warning,
								SupportsFiles: g.SupportsFiles, SupportsSSH: g.SupportsSSH,
								AllowRead: g.AllowRead, AllowWrite: g.AllowWrite, AllowExec: g.AllowExec,
							})
						}
						uikit.PermissionMatrix(c, matrixRows, func(index int, capability string, enabled bool) {
							if index < 0 || index >= len(draft.Grants) {
								return
							}
							switch capability {
							case "read":
								draft.Grants[index].AllowRead = enabled
							case "write":
								draft.Grants[index].AllowWrite = enabled
							case "exec":
								draft.Grants[index].AllowExec = enabled
							}
						})
					})

					// 第 3 段 · 本地目录
					ui.Column(c).FillWidth().Gap(t.Space.N6).Children(func() {
						uikit.Badge(c, "第 3 段 · 目录授权", uikit.BadgeProps{Tone: uikit.ToneAccent})
						ui.Row(c).FillWidth().Gap(t.Space.N10).AlignItems(ui.Stretch).Children(func() {
							uikit.DirectoryListEditor(c.Key("up-dirs"), draft.UploadDirs, uikit.DirectoryListEditorProps{
								Title:      "上传来源目录",
								EmptyHint:  "尚未添加上传目录",
								FooterHint: "允许从目录及其子目录上传，仍受文件策略限制。",
								OnAdd:      func() { a.pickMCPDirectory(true) },
								OnRemove: func(idx int) {
									draft.UploadDirs = slices.Delete(draft.UploadDirs, idx, idx+1)
								},
							})
							uikit.DirectoryListEditor(c.Key("down-dirs"), draft.DownloadDirs, uikit.DirectoryListEditorProps{
								Title:      "下载目标目录",
								EmptyHint:  "尚未添加下载目录",
								FooterHint: "允许向目录及其子目录下载，仍受文件策略限制。",
								OnAdd:      func() { a.pickMCPDirectory(false) },
								OnRemove: func(idx int) {
									draft.DownloadDirs = slices.Delete(draft.DownloadDirs, idx, idx+1)
								},
							})
						})
					})
				})
			})
			uikit.Scrollbars(c, viewport, scroller, &draft.Scroll, uikit.ScrollbarProps{Label: "密钥授权表单滚动条"})
		})
	})
}

func (a *app) submitMCPKeyModal() {
	a.saveMCPBackendKey()
}

func (a *app) mcpKeyConfirmModal(c *ui.Context) {
	if !a.mcp.keyConfirm.Open {
		return
	}
	t := theme.Current()
	dlg := &a.mcp.keyConfirm
	isRevoke := dlg.Action == "revoke"
	title := fmt.Sprintf("停用密钥「%s」？", dlg.Name)
	btnLabel := "停用密钥"
	variant := uikit.ButtonPrimary
	if isRevoke {
		title = fmt.Sprintf("撤销密钥「%s」？", dlg.Name)
		btnLabel = "撤销密钥"
		variant = uikit.ButtonDanger
	}
	uikit.Modal(c.Key("mcp-key-confirm"), &dlg.Open, uikit.ModalProps{
		Controller: &dlg.modal,
		Title:      title,
		Compact:    true,
		Width:      t.C.Mcp.DialogWidth,
		Disabled:   a.mcpBusy(),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, "• 立即拒绝该密钥的后续 MCP 请求。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		uikit.Text(c, fmt.Sprintf("• 自动取消 %d 个排队任务，并向关联的执行中任务请求取消。", a.mcpQueuedTasks(dlg.KeyID)), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		uikit.Text(c, "• 保留历史审计记录与当时的密钥名称。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		if isRevoke {
			uikit.Banner(c, "若仅需临时暂停访问，请选择「停用密钥」；永久撤销不可恢复。", uikit.BannerProps{
				Tone: uikit.ToneDanger, Icon: uikit.IconSVG(uikit.IconAlertCircle),
			})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, "取消", uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			if uikit.Button(c, btnLabel, uikit.ButtonProps{Variant: variant}).Clicked() {
				state := "disabled"
				if isRevoke {
					state = "revoked"
				}
				a.setMCPBackendKeyState(dlg.KeyID, state, func(err error) {
					if err == nil {
						dlg.modal.Close()
					}
				})
			}
		})
	})
}

func (a *app) mcpPolicyDeleteModal(c *ui.Context) {
	if !a.mcp.policyDelete.Open {
		return
	}
	t := theme.Current()
	dlg := &a.mcp.policyDelete
	uikit.Modal(c.Key("mcp-policy-delete"), &dlg.Open, uikit.ModalProps{
		Controller: &dlg.modal,
		Title:      fmt.Sprintf("删除策略「%s」？", dlg.Name),
		Compact:    true,
		Width:      t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, "删除后，该策略将不再参与后续操作的拦截判定。此操作不可恢复，历史命中审计与规则快照保留。", uikit.TextProps{
			Style: uikit.TextSmall, Tone: uikit.ToneSecondary,
		})
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, "取消", uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			if uikit.Button(c, "删除策略", uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked() {
				a.deleteMCPBackendPolicy(dlg.PolicyID)
			}
		})
	})
}

func (a *app) mcpListenModal(c *ui.Context) {
	if !a.mcp.listenModalOpen {
		return
	}
	t := theme.Current()
	var host, hostError string
	uikit.Modal(c.Key("mcp-listen-modal"), &a.mcp.listenModalOpen, uikit.ModalProps{
		Title:   "编辑监听配置",
		Compact: true,
		Width:   t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, "仅支持本机监听，保存后同步更新客户端配置。", uikit.TextProps{
			Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
		})
		uikit.FormRow(c, "监听主机", uikit.FormRowProps{Align: uikit.RowAlignCenter}, func() ui.Element {
			input := uikit.Input(c, &a.mcp.hostDraft, uikit.InputProps{
				Label: "监听主机", Placeholder: "127.0.0.1", Mono: true, DeferStyle: true,
			})
			host = strings.TrimSpace(a.mcp.hostDraft)
			address, err := url.Parse("http://" + net.JoinHostPort(host, "1"))
			switch {
			case host == "":
				hostError = "请输入监听主机"
			case err != nil || address.Hostname() != host:
				hostError = "仅填写 IP 地址或主机名，不含协议、路径或端口"
			case strings.Contains(host, ":") && net.ParseIP(host) == nil:
				hostError = "请输入有效的 IPv6 地址，或不含端口的主机名"
			}
			uikit.StyleFormControl(input, hostError)
			if hostError != "" {
				uikit.FieldMessage(c, hostError, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
			}
			return input
		})
		uikit.FormRow(c, "监听端口", uikit.FormRowProps{Align: uikit.RowAlignCenter}, func() ui.Element {
			return uikit.PortInput(c, &a.mcp.portDraft, uikit.InputProps{Label: "监听端口", Placeholder: "7801"})
		})
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, "取消", uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			portNum, err := strconv.Atoi(a.mcp.portDraft)
			valid := hostError == "" && err == nil && portNum >= 1 && portNum <= 65535
			if uikit.Button(c, "保存配置", uikit.ButtonProps{Variant: uikit.ButtonPrimary, Disabled: !valid}).Clicked() {
				a.mcp.host = host
				a.mcp.port = a.mcp.portDraft
				a.configureMCPBackend()
				parts.RequestDismiss()
			}
		})
	})
}

func (a *app) mcpRetentionModal(c *ui.Context) {
	if !a.mcp.retentionOpen {
		return
	}
	t := theme.Current()
	uikit.Modal(c.Key("mcp-retention-modal"), &a.mcp.retentionOpen, uikit.ModalProps{
		Title:   "审计保留设置",
		Compact: true,
		Width:   t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, "清理以已结束的操作及关联逐文件明细为单位；活动任务、待核实状态和待恢复记录不在清理范围内。", uikit.TextProps{
			Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
			uikit.Text(c, "保留周期", uikit.TextProps{Style: uikit.TextSmall})
			previousDays := a.mcp.retentionDays
			uikit.FilterChip(c, "保留最近", &a.mcp.retentionDays, []uikit.Option[string]{
				{Value: "7", Label: "7 天"},
				{Value: "30", Label: "30 天"},
				{Value: "90", Label: "90 天"},
			}, uikit.FilterChipProps{Key: "ret-days"})
			if previousDays != a.mcp.retentionDays {
				a.configureMCPBackend()
			}
		})
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, "关闭", uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			if uikit.Button(c, "清理记录", uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked() {
				parts.Close()
				a.pruneMCPBackend()
			}
		})
	})
}

func (a *app) mcpClientConfigModal(c *ui.Context) {
	if !a.mcp.configModalOpen {
		return
	}
	t := theme.Current()
	cfg := a.mcpClientConfigJSON()
	uikit.Modal(c.Key("mcp-client-cfg-modal"), &a.mcp.configModalOpen, uikit.ModalProps{
		Title:   "客户端配置",
		Compact: true,
		Width:   t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, "配置已复制，请将 <YOUR_SECRET_KEY> 替换为已保存的密钥。", uikit.TextProps{
			Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
		})
		if uikit.CodeBlock(c.Key("client-config"), "配置内容", cfg).Copy {
			mygo.Clipboard.WriteText(cfg)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, "完成", uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Clicked() {
				parts.Close()
			}
		})
	})
}

// commandRuleError is the inline validation for one rule; saving stays
// disabled while any rule reports one.
func commandRuleError(r mcpCommandRule) string {
	if strings.TrimSpace(r.Pattern) == "" {
		return ""
	}
	switch r.Mode {
	case "regex":
		if _, err := regexp.Compile(r.Pattern); err != nil {
			return "正则表达式无效：" + err.Error()
		}
	case "command":
		if _, err := shellcmd.ParseRule(r.Pattern, r.IgnoreCase); err != nil {
			return err.Error()
		}
	}
	return ""
}
