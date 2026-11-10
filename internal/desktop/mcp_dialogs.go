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
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/shellcmd"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"
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
	// A merged selection is stored with its member IDs. Show it once when
	// editing, while preserving policies scoped only to a legacy member.
	for _, c := range a.mcpBackend.connections {
		if c.CanonicalID != "" && slices.Contains(pol.Connections, c.CanonicalID) {
			connections = slices.DeleteFunc(connections, func(id string) bool { return id == c.ID })
		}
	}
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
	title := locales.MessageEditInterceptionPolicy()
	idBadge := fmt.Sprintf("%s · v%d", draft.EditingID, draft.Version)
	if draft.EditingID == "" {
		title = locales.MessageNewInterceptionPolicy()
		idBadge = locales.MessageDraftV1()
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
		Width: t.C.Mcp.PolicyModalWidth, CloseLabel: locales.MessageClosePolicyWindow(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Children(func() {
				if draft.CategorySeg == 1 {
					uikit.Text(c, locales.MessageCheckedActionsAreBlockedRenameMoveAndDeleteToolsAreNotAvailableYet(), uikit.TextProps{
						Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
					}).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				} else {
					ui.Spacer(c)
				}
				if uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).Clicked() {
					parts.RequestDismiss()
				}
				saveRequested = uikit.Button(c, locales.MessageSavePolicy(), uikit.ButtonProps{
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
		uikit.Scrollbars(
			c,
			parts.Panel,
			scroller,
			&draft.Scroll,
			uikit.ScrollbarProps{Label: locales.MessagePolicyFormScrollbar()},
		)
	})
	if (saveRequested || dialog.Submit) && !ruleInvalid {
		a.saveMCPPolicyDraft()
	}
}

func (a *app) mcpPolicyFields(c *ui.Context) {
	t := theme.Current()
	draft := &a.mcp.policyDraft
	if !draft.SaveError.Empty() {
		uikit.Banner(c, draft.SaveError.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: uikit.IconSVG(uikit.IconAlertCircle)})
	}
	// 1. 基本信息与适用范围
	ui.Column(c).FillWidth().Gap(t.Space.N8).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			uikit.Text(c, locales.MessageDetailsAndScope(), uikit.TextProps{Style: uikit.TextBodyStrong})
			ui.Spacer(c)
			uikit.Text(c, fmt.Sprintf(
				locales.MessageSavingUpdatesThisPolicyToVItTakesEffectWhenEnabled(),
				draft.Version+1,
			), uikit.TextProps{
				Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
			})
		})
		uikit.FormRow(c, locales.MessagePolicyName(), uikit.FormRowProps{Error: draft.NameError.String()}, func() ui.Element {
			var name ui.Element
			ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
				name = uikit.Input(c, &draft.Name, uikit.InputProps{
					Label: locales.MessagePolicyName(), Placeholder: locales.MessageForExampleProductionConfigurationProtection(), Error: draft.NameError.String(),
				}).Grow(1)
				uikit.Text(c, locales.MessageEnable(), uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
				uikit.Switch(c, &draft.Enabled, uikit.ToggleProps{
					Label: locales.MessageEnableThisPolicy(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
				})
			})
			if name.Changed() && !draft.NameError.Empty() {
				draft.NameError = locales.Text{}
				c.Invalidate()
			}
			return name
		})
		uikit.FormRow(c, locales.MessageDescription(), uikit.FormRowProps{}, func() ui.Element {
			return uikit.Input(c, &draft.Description, uikit.InputProps{
				Label: locales.MessageDescription(), Placeholder: locales.MessageDescribeWhatThisPolicyBlocksAndProtects(),
			})
		})
		uikit.FormRow(c, locales.MessagePolicyCategory(), uikit.FormRowProps{Align: uikit.RowAlignCenter}, func() ui.Element {
			var category ui.Element
			ui.Row(c).FillWidth().Gap(t.Space.N10).Children(func() {
				previousCategory := draft.CategorySeg
				category = uikit.Segmented(c, &draft.CategorySeg, uikit.SegmentItems(
					[]string{locales.MessageCommandPolicy(), locales.MessageFilePolicy()},
				), uikit.SegmentedProps{
					Size: uikit.SizeSm, Label: locales.MessagePolicyCategory(),
				})
				if previousCategory != draft.CategorySeg {
					draft.Connections = []string{}
				}
				uikit.Text(
					c,
					locales.MessageOnlyConnectionsSupportedByThisCategoryAreShown(),
					uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary},
				)
			})
			return category
		})
		uikit.FormRow(c, locales.MessageApplicableConnections(), uikit.FormRowProps{}, func() ui.Element {
			candidates := a.mcpConnectionOptions(draft.CategorySeg, false)
			// Keep a policy's explicit legacy member scopes selectable, rather
			// than silently replacing them with the whole merged connection.
			for _, connection := range a.mcpBackend.connections {
				if slices.Contains(draft.Connections, connection.ID) &&
					!slices.ContainsFunc(candidates, func(option uikit.Option[string]) bool { return option.Value == connection.ID }) {
					candidates = append(candidates, uikit.Option[string]{Value: connection.ID, Label: connection.Name + locales.MessageOriginalConfiguration()})
				}
			}
			return uikit.MultiSelect(c.Key("policy-connections"), &draft.Connections, candidates, uikit.MultiSelectProps{
				Label: locales.MessageApplicableConnections(), Placeholder: locales.MessageAllConnectionsLeaveEmptyForAll(),
			})
		})
		uikit.FormRow(c, locales.MessageApplicableKeys(), uikit.FormRowProps{}, func() ui.Element {
			options := []uikit.Option[string]{}
			for _, k := range a.mcp.keys {
				if k.State == "revoked" {
					continue
				}
				options = append(options, uikit.Option[string]{Value: k.ID, Label: k.Name})
			}
			return uikit.MultiSelect(c.Key("policy-keys"), &draft.SelectedKeys, options, uikit.MultiSelectProps{
				Label: locales.MessageApplicableKeys(), Placeholder: locales.MessageAllKeysLeaveEmptyForAll(),
			})
		})
	})

	uikit.Divider(c, uikit.DividerProps{})

	// 2. 文件操作规则编辑器
	if draft.CategorySeg == 1 {
		ui.Column(c).FillWidth().Gap(t.Space.N8).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				uikit.Text(c, locales.MessageFileInterceptionRules(), uikit.TextProps{Style: uikit.TextBodyStrong})
				ui.Spacer(c)
				if uikit.Button(c, locales.MessageAddPathRule(), uikit.ButtonProps{Size: uikit.SizeSm}).Clicked() {
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
						uikit.Text(
							c,
							fmt.Sprintf(locales.MessageRule(), ruleIdx+1),
							uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
						)
						ui.Spacer(c)
						if len(draft.FileRules) > 1 {
							if uikit.Button(
								c.Key("del-f"),
								locales.CommonDelete(),
								uikit.ButtonProps{Variant: uikit.ButtonDangerPlain, Size: uikit.SizeSm},
							).Clicked() {
								remove = ruleIdx
							}
						}
					})
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
						uikit.FilterChip(c, locales.MessageLocation(), &rule.Side, []uikit.Option[string]{
							{Value: "remote", Label: locales.MessageRemotePath()},
							{Value: "local", Label: locales.MessageLocalPath()},
						}, uikit.FilterChipProps{Key: fmt.Sprintf("side-%d", ruleIdx)})
						uikit.FilterChip(c, locales.MessageType(), &rule.Kind, []uikit.Option[string]{
							{Value: "dir", Label: locales.MessageDirectories()},
							{Value: "file", Label: locales.MessageFile()},
						}, uikit.FilterChipProps{Key: fmt.Sprintf("kind-%d", ruleIdx)})
						uikit.Input(c.Key(fmt.Sprintf("path-%d", ruleIdx)), &rule.Path, uikit.InputProps{
							Mono: true, Label: locales.MessageProtectedPath(), Placeholder: "/var/www/shop/public/config",
						}).Grow(1)
						if rule.Kind == "dir" {
							uikit.Checkbox(
								c.Key(fmt.Sprintf("sub-%d", ruleIdx)),
								&rule.IncludeSubdir,
								locales.MessageIncludeSubdirectories(),
							)
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
				uikit.Text(c, locales.MessageCommandInterceptionRules(), uikit.TextProps{Style: uikit.TextBodyStrong})
				ui.Spacer(c)
				if uikit.Button(c, locales.MessageAddCommandRule(), uikit.ButtonProps{Size: uikit.SizeSm}).Clicked() {
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
							{Value: "command", Label: locales.MessageCommandMatch()},
							{Value: "exact", Label: locales.MessageExactMatch()},
							{Value: "regex", Label: locales.MessageRegularExpression()},
							{Value: "contains", Label: locales.MessageSubstringMatch()},
						}, uikit.SelectProps{Label: locales.MessageMatchMode()}).
							Width(t.C.Mcp.RuleModeWidth).Shrink(0)
						uikit.Input(c.Key(fmt.Sprintf("cpat-%d", cmdIdx)), &cmdRule.Pattern, uikit.InputProps{
							Mono: true, Label: locales.MessageMatchPattern(), Placeholder: "rm -rf /", Error: ruleErr,
						}).Grow(1)
						uikit.Badge(c, locales.MessageBlockMatches(), uikit.BadgeProps{Tone: uikit.ToneDanger})
						if len(draft.CmdRules) > 1 {
							if uikit.Button(
								c.Key("del-c"),
								locales.CommonDelete(),
								uikit.ButtonProps{Variant: uikit.ButtonDangerPlain, Size: uikit.SizeSm},
							).Clicked() {
								remove = cmdIdx
							}
						}
					})
					switch {
					case ruleErr != "":
						uikit.FieldMessage(c, ruleErr, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
					case cmdRule.Mode == "contains":
						uikit.FieldMessage(
							c,
							locales.MessageSubstringMatchingAlsoMatchesRmInWordsSuchAsFormatCommandMatchingIs(),
							uikit.FieldMessageProps{Tone: uikit.ToneWarning},
						)
					}
					if cmdRule.Mode == "command" {
						uikit.Checkbox(
							c.Key(fmt.Sprintf("cicase-%d", cmdIdx)),
							&cmdRule.IgnoreCase,
							locales.MessageIgnoreCaseInArguments(),
						)
					}
				})
			}
			if remove >= 0 {
				draft.CmdRules = slices.Delete(draft.CmdRules, remove, remove+1)
				c.Invalidate()
			}
			uikit.Text(c, locales.MessageCommandMatchingChecksCommandNamesAndArgumentsIncludingCombinationsWithPipesSudoBash(), uikit.TextProps{
				Style: uikit.TextCaption, Tone: uikit.ToneSecondary,
			})
			uikit.Text(c, locales.MessageCommandPoliciesHelpPreventMistakesUnparseablePartsAreConservativelyBlockedByKeywordsThey(), uikit.TextProps{
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
		Title:           locales.MessageOperationDetails(),
		Width:           a.mcp.auditDrawerWidth,
		Scroll:          &a.mcp.drawerScroll,
		ContentPaddingX: t.Space.N14,
		Header: func() bool {
			closed := false
			ui.Row(c).FillWidth().Height(t.C.Panel.HeaderHeight).Padding(0, t.Space.N14).
				BorderWidth(0, 0, t.Size.Divider, 0).BorderColor(uikit.Color(t.Color.Border.Divider)).
				AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
				uikit.Text(c, locales.MessageOperationDetails(), uikit.TextProps{Style: uikit.TextHeadline})
				if uikit.CopyField(
					c,
					item.ID,
					uikit.CopyFieldProps{Key: "copy-op-id", Label: locales.MessageCopyOperationId()},
				).Copied {
					mygo.Clipboard.WriteText(item.ID)
				}
				uikit.StatusPill(c, item.Status)
				ui.Spacer(c)
				if uikit.IconButton(
					c,
					closeIcon,
					uikit.IconButtonProps{Key: "close-audit-drawer", Label: locales.MessageCloseDetailsDrawer(), Size: uikit.SizeSm},
				).Clicked() {
					closed = true
				}
			})
			return closed
		},
	}, func() {
		ui.Column(c).FillWidth().Padding(t.Space.N12, 0).Gap(t.Space.N12).Children(func() {
			// 1. 基本信息
			uikit.Text(c, locales.MessageDetails(), uikit.TextProps{Style: uikit.TextBodyStrong})
			uikit.KeyValueGrid(c, 2, []uikit.KV{
				{
					Key: locales.MessageKeyName(),
					Render: func() {
						uikit.Text(c, item.KeyName, uikit.TextProps{Style: uikit.TextBodyStrong}).SingleLine().Ellipsis("…")
					},
				},
				{Key: locales.MessageKeyId(), Value: item.KeyID, ValueMono: true},
				{Key: locales.MessageStarted(), Value: item.StartTime},
				{Key: locales.MessageFinished(), Value: item.EndTime},
				{Key: locales.MessageActionType(), Value: item.ActionType},
				{
					Key: locales.MessageResult(),
					Render: func() {
						uikit.StatusPill(c, item.Status)
					},
				},
			})

			// 2. 目标快照
			if item.ConnectionID != "" {
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					uikit.Text(c, locales.MessageTargetSnapshot(), uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Spacer(c)
					uikit.Text(
						c,
						locales.MessageConnectionDetailsCapturedAtCallTimeLaterChangesDoNotAffectThisRecord(),
						uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary},
					)
				})
				uikit.KeyValueGrid(c, 2, []uikit.KV{
					{Key: locales.MessageHost(), Value: item.ConnHost + " (" + item.ConnName + ")", ValueMono: true},
					{Key: locales.MessagePort(), Value: item.ConnPort, ValueMono: true},
					{Key: locales.MessageAccount(), Value: item.ConnUser, ValueMono: true},
					{Key: locales.MessageProtocol(), Value: item.ConnProtocol},
				})
			}

			// 3. 命令信息 或 文件信息
			if item.IsCommand {
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
					uikit.Text(c, locales.MessageCommandDetails(), uikit.TextProps{Style: uikit.TextBodyStrong})
					warnIcon := uikit.Icon(c, uikit.IconInfo, uikit.IconProps{Size: uikit.SizeSm, Tone: uikit.ToneWarning})
					uikit.Tip(
						c,
						warnIcon,
						locales.MessageCommandsMayContainSensitiveArgumentsTreatAuditRecordsAsSensitiveLocalDataAutomatic(),
					)
					uikit.Text(
						c,
						locales.MessageCommandsMayContainSensitiveArgumentsKeepAuditRecordsSecure(),
						uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneWarning},
					)
				})
				if uikit.CodeBlock(c.Key("command-source"), locales.MessageOriginalCommand(), item.CommandText).Copy {
					mygo.Clipboard.WriteText(item.CommandText)
				}
				uikit.KeyValue(c, []uikit.KV{
					{Key: locales.MessageExitCode(), Value: item.ExitCodeText, ValueMono: true},
					{Key: locales.MessageOutputAndStatus(), Value: item.OutputStatus},
				})
			} else if item.LocalPath != "" || item.RemotePath != "" {
				uikit.Divider(c, uikit.DividerProps{})
				uikit.Text(c, locales.MessageFileInformation(), uikit.TextProps{Style: uikit.TextBodyStrong})
				var fileKVs []uikit.KV
				if item.LocalPath != "" {
					fileKVs = append(fileKVs, uikit.KV{Key: locales.MessageLocalPath(), Value: item.LocalPath, ValueMono: true})
				}
				if item.RemotePath != "" {
					fileKVs = append(fileKVs, uikit.KV{Key: locales.MessageRemotePath(), Value: item.RemotePath, ValueMono: true})
				}
				if item.TransferStat != "" {
					fileKVs = append(fileKVs, uikit.KV{Key: locales.MessageTransferStatistics(), Value: item.TransferStat})
				}
				uikit.KeyValue(c, fileKVs)
			}

			uikit.Divider(c, uikit.DividerProps{})
			if !item.DetailLoaded {
				uikit.Text(
					c,
					locales.MessageLoadingRequestAndResponse(),
					uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary},
				)
			} else {
				if !item.PayloadSaved {
					uikit.Banner(
						c,
						locales.MessageTheFullRequestAndResponseWereNotSavedOnlyAvailableSummariesAreShown(),
						uikit.BannerProps{Tone: uikit.ToneNeutral},
					)
				}
				if item.FileBodyOmitted {
					uikit.Banner(
						c,
						locales.MessageFileContentsAreNotStoredInTheAuditRequestsAndResponsesRetainPaths(),
						uikit.BannerProps{Tone: uikit.ToneNeutral},
					)
				}
				if uikit.CodeBlock(c.Key("audit-request"), locales.MessageRequestData(), item.RequestJSON).Copy {
					mygo.Clipboard.WriteText(item.RequestJSON)
				}
				if item.IsCommand && item.ResponseSaved && !item.ResponseTruncated {
					ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
						uikit.Segmented(c, &a.mcp.auditResponseTab, uikit.SegmentItems(
							[]string{locales.MessageOutput(), "JSON"},
						), uikit.SegmentedProps{
							Label: locales.MessageResponseView(), Size: uikit.SizeSm,
						})
					})
				}
				if item.ResponseTruncated {
					uikit.Banner(
						c,
						locales.MessageTheResponseExceededTheAuditCollectionLimitAndItsBodyWasNotSaved(),
						uikit.BannerProps{Tone: uikit.ToneWarning},
					)
				}
				if !item.ResponseSaved && item.PayloadSaved {
					uikit.Text(
						c,
						locales.MessageNoResponseHasBeenRecordedTheCallMayStillBeRunningOrMay(),
						uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneSecondary},
					)
				} else if item.IsCommand && item.ResponseSaved && !item.ResponseTruncated && a.mcp.auditResponseTab == 0 {
					if uikit.CodeBlock(c.Key("audit-stdout"), locales.MessageStandardOutputStdout(), item.Stdout).Copy {
						mygo.Clipboard.WriteText(item.Stdout)
					}
					if uikit.CodeBlock(c.Key("audit-stderr"), locales.MessageStandardErrorStderr(), item.Stderr).Copy {
						mygo.Clipboard.WriteText(item.Stderr)
					}
				} else if uikit.CodeBlock(c.Key("audit-response"), locales.MessageResponseData(), item.ResponseJSON).Copy {
					mygo.Clipboard.WriteText(item.ResponseJSON)
				}
			}

			// 4. 命中拦截策略快照卡片
			if len(item.PolicyHits) > 0 {
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					uikit.Text(c, locales.MessageMatchedInterceptionPolicies(), uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Spacer(c)
					uikit.Text(
						c,
						locales.MessageRulesCapturedAtDecisionTimeLaterChangesDoNotAffectThisRecord(),
						uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary},
					)
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
							uikit.Text(
								c,
								locales.MessageMatchedRule()+hit.RuleSummary,
								uikit.TextProps{Style: uikit.TextMono},
							).Grow(1).MinWidth(0)
							uikit.Text(c, hit.ForbiddenText, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneDanger})
						})
						if hit.MatchSaved {
							basis := locales.MessageTextMatch()
							if strings.HasPrefix(hit.RuleSummary, "command: ") {
								basis = locales.MessageCommandMatch()
							}
							if hit.MatchFallback != "" {
								basis = locales.MessageConservativeKeywordRejection() + hit.MatchFallback
							}
							uikit.Text(
								c,
								locales.MessageDecisionBasis()+basis,
								uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary},
							)
							if hit.MatchVia != "" {
								uikit.Text(
									c,
									locales.MessageVia()+hit.MatchVia,
									uikit.TextProps{Style: uikit.TextMono, Tone: uikit.ToneSecondary},
								)
							}
							if uikit.CodeBlock(c.Key("matched"), locales.MessageMatchedFragment(), hit.MatchText).Copy {
								mygo.Clipboard.WriteText(hit.MatchText)
							}
							if hit.MatchContext != "" && hit.MatchContext != hit.MatchText {
								if uikit.CodeBlock(c.Key("match-context"), locales.MessageContainingText(), hit.MatchContext).Copy {
									mygo.Clipboard.WriteText(hit.MatchContext)
								}
							}
						}
					})
				}
			}

			// 5. 关联任务卡片（含进度条、暂停/继续/取消 与 黄色等待用户确认块）
			if item.Task != nil {
				task := item.Task
				uikit.Divider(c, uikit.DividerProps{})
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					uikit.Text(c, locales.MessageRelatedTask(), uikit.TextProps{Style: uikit.TextBodyStrong})
					ui.Spacer(c)
					uikit.Text(
						c,
						task.ID+locales.MessageContinuesAfterTheClientDisconnects(),
						uikit.TextProps{Style: uikit.TextCaption, Tone: uikit.ToneTertiary},
					)
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
						pauseLabel := locales.MessagePause()
						pauseIcon := uikit.IconPause
						if task.Paused {
							pauseLabel = locales.MessageResume()
							pauseIcon = uikit.IconPlay
						}
						if task.CleanupOnly {
							pauseLabel = locales.MessageRetryCleanup()
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
						cancelLabel := locales.CommonCancel()
						if task.CancelRequest {
							cancelLabel = locales.MessageCancellationRequested()
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
						Value: task.Fraction, Tone: progTone, Label: locales.MessageTaskTransferProgress(),
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
							Action: locales.MessageAcknowledgedAndHidden(),
						})
						if res.Action {
							a.showMainWindow()
						}
					}
				})
			}

			// 6. 生命周期时间线
			if len(item.Timeline) > 0 {
				uikit.Divider(c, uikit.DividerProps{})
				uikit.Text(c, locales.MessageLifecycle(), uikit.TextProps{Style: uikit.TextBodyStrong})
				uikit.Timeline(c, item.Timeline)
			}

			// 7. 逐文件明细
			if len(item.Files) > 0 {
				uikit.Divider(c, uikit.DividerProps{})
				uikit.Disclosure(c.Key("audit-files"), &a.mcp.auditFilesOpen, uikit.DisclosureProps{
					Title: locales.MessagePerFileDetails(), TitleStyle: uikit.TextBodyStrong, Meta: locales.CommonFileCount(
						item.FileTotal,
					),
					Motion: uikit.MotionOverride(a.reduceMotion(c)),
				}, func() {
					ui.Column(c).FillWidth().Gap(t.Space.N12).Children(func() {
						cntAll, cntOk, cntBlock, cntFail, cntSkip, cntPend := item.FileTotal, item.FileCounts["completed"], item.FileCounts["intercepted"], item.FileCounts["failed"], item.FileCounts["skipped"], item.FileCounts["running"]+item.FileCounts["pending"]+item.FileCounts["interrupted"]
						filterItems := []uikit.SegItem{
							{Label: locales.MessageAll(), Count: &cntAll},
							{Label: locales.MessageSucceeded(), Count: &cntOk},
							{Label: locales.MessageIntercepted(), Count: &cntBlock},
							{Label: locales.MessageFailed(), Count: &cntFail},
							{Label: locales.MessageSkip(), Count: &cntSkip},
							{Label: locales.MessageIncomplete(), Count: &cntPend},
						}
						uikit.Segmented(
							c,
							&a.mcp.auditFileFilter,
							filterItems,
							uikit.SegmentedProps{Size: uikit.SizeSm, Label: locales.MessageFilterPerFileResults()},
						)
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
								stateLabel := locales.MessageSucceeded()
								switch f.State {
								case "intercepted":
									rowTone = uikit.ToneWarning
									icon = uikit.IconBan
									tone = uikit.ToneWarning
									stateLabel = locales.MessageBlockedByPolicy()
								case "failed":
									rowTone = uikit.ToneDanger
									icon = uikit.IconClose
									tone = uikit.ToneDanger
									stateLabel = locales.MessageFailed()
								case "skipped":
									icon = uikit.IconArrowRight
									tone = uikit.ToneSecondary
									stateLabel = locales.MessageSkip()
								case "pending":
									icon = uikit.IconActivity
									tone = uikit.ToneAccent
									stateLabel = locales.MessageIncomplete()
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
			grants := cloneMCPGrants(k.Grants)
			if len(grants) == 0 {
				grants = a.mcpGrants()
			}
			a.mcp.keyModal = mcpKeyModalDraft{
				Open:      true,
				EditingID: k.ID,
				EditingMeta: locales.Concat(
					locales.Concat(locales.Raw(k.Prefix), locales.Deferred(locales.MessageCreated)),
					locales.Raw(k.CreatedAt),
				),
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
	title := locales.MessageChooseDownloadDestination()
	if forUpload {
		title = locales.MessageChooseUploadSource()
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
	title := locales.MessageNewAccessKey()
	subtitle := locales.MessageGenerateCredentialsForAnMcpClientAndGrantPermissionsPerConnection()
	if draft.EditingID != "" {
		title = locales.MessageEditAccessKey()
		subtitle = draft.EditingMeta.String()
	}
	if draft.OneTimeView {
		title = locales.MessageAccessKeyCreated()
		subtitle = locales.MessageThisKeyIsShownOnlyOnceCopyAndSaveItThenConfirmBelow()
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
					uikit.Text(c, locales.MessageIfYouLoseThisKeyRevokeItAndGenerateANewOne(), uikit.TextProps{
						Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
					}).Grow(1)
					if uikit.Button(c, locales.MessageDone(), uikit.ButtonProps{
						Variant:  uikit.ButtonPrimary,
						Disabled: !draft.SavedConfirmed,
					}).Clicked() {
						parts.RequestDismiss()
					}
					return
				}
				if draft.EditingID == "" {
					uikit.Text(c, locales.MessageTheKeyIsShownInPlainTextOnlyOnceAfterCreationCopyAnd(), uikit.TextProps{
						Style: uikit.TextCaption, Tone: uikit.ToneTertiary,
					}).Grow(1)
				} else {
					ui.Spacer(c)
				}
				if uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).Clicked() {
					parts.RequestDismiss()
				}
				saveLabel := locales.MessageGenerateKey()
				if draft.EditingID != "" {
					saveLabel = locales.MessageSaveKey()
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
				// 分区之间只靠留白区分；表单不是分步向导，不再编号。
				ui.Column(c).FillWidth().Gap(t.Space.N24).Children(func() {
					a.mcpKeyNameField(c, draft)
					a.mcpKeyGrantsSection(c, draft)
					a.mcpKeyDirsSection(c, draft)
				})
			})
			uikit.Scrollbars(
				c,
				viewport,
				scroller,
				&draft.Scroll,
				uikit.ScrollbarProps{Label: locales.MessageKeyAuthorizationFormScrollbar()},
			)
		})
	})
}

// mcpKeyFilterMin 是密钥弹窗显示连接搜索与「仅看已授权」的连接数下限（超过才显示）。
const mcpKeyFilterMin = 8

// mcpKeySectionHeader 是密钥弹窗的分区标题：标题加一行说明，trailing 放右侧摘要。
func mcpKeySectionHeader(c *ui.Context, title, description string, trailing func()) {
	t := theme.Current()
	ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space.N12).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space.N2).Children(func() {
			uikit.Text(c, title, uikit.TextProps{Style: uikit.TextBodyStrong})
			uikit.Text(c, description, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary})
		})
		if trailing != nil {
			trailing()
		}
	})
}

// mcpKeyNameField 把标签放在输入框上方，说明或错误放在下方。
func (a *app) mcpKeyNameField(c *ui.Context, draft *mcpKeyModalDraft) {
	t := theme.Current()
	ui.Column(c).FillWidth().Gap(t.Space.N6).Role(ui.RoleGroup).Label(locales.MessageKeyName()).Children(func() {
		var name ui.Element
		uikit.Text(c, locales.MessageKeyName(), uikit.TextProps{Style: uikit.TextLabel}).OnClick(func() { name.Focus() })
		name = uikit.Input(c, &draft.Name, uikit.InputProps{
			Label: locales.MessageKeyName(), Placeholder: locales.MessageForExampleClaudeDesktop(), Error: draft.NameError.String(),
		})
		if name.Changed() && !draft.NameError.Empty() {
			draft.NameError = locales.Text{}
			c.Invalidate()
		}
		if !draft.NameError.Empty() {
			name.Error(draft.NameError.String())
			uikit.FieldMessage(c, draft.NameError.String(), uikit.FieldMessageProps{Tone: uikit.ToneDanger})
		} else {
			uikit.FieldMessage(
				c,
				locales.MessageUseASeparateKeyForEachClientSoYouCanDisableOrRevoke(),
				uikit.FieldMessageProps{},
			)
		}
	})
}

func (a *app) mcpKeyGrantsSection(c *ui.Context, draft *mcpKeyModalDraft) {
	t := theme.Current()
	granted, execs := 0, 0
	for _, g := range draft.Grants {
		if g.AllowRead || g.AllowWrite || g.AllowExec {
			granted++
		}
		if g.AllowExec {
			execs++
		}
	}
	ui.Column(c).FillWidth().Gap(t.Space.N10).Children(func() {
		mcpKeySectionHeader(c, locales.MessageConnectionPermissions(), locales.MessageNoPermissionsAreGrantedByDefaultGrantedActionsRemainSubjectToInterceptionPolicies(), func() {
			tone := uikit.ToneSecondary
			if granted > 0 {
				tone = uikit.ToneAccent
			}
			uikit.Text(c, locales.McpAuthorizedConnections(granted, len(draft.Grants)), uikit.TextProps{
				Style: uikit.TextFootnote, Tone: tone,
			}).Shrink(0)
		})

		filtering := len(draft.Grants) > mcpKeyFilterMin
		if filtering {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N12).Children(func() {
				uikit.SearchField(c.Key("grant-query"), &draft.Query, uikit.SearchProps{
					Variant: uikit.SearchFilter, Icon: uikit.IconSVG(uikit.IconSearch),
					Label: locales.MessageSearchConnections(), Placeholder: locales.MessageSearchNamesOrAddresses(),
				}).Root.Grow(1).MinWidth(0)
				if uikit.Checkbox(c.Key("only-granted"), &draft.OnlyGranted, locales.MessageShowAuthorizedOnly()).Changed() {
					// 记下开启时已授权的连接，之后取消勾选不会让该行立刻消失。
					draft.GrantedSnapshot = nil
					if draft.OnlyGranted {
						draft.GrantedSnapshot = map[string]bool{}
						for _, g := range draft.Grants {
							if g.AllowRead || g.AllowWrite || g.AllowExec {
								draft.GrantedSnapshot[g.ConnectionID] = true
							}
						}
					}
				}
			})
		}

		query := strings.ToLower(strings.TrimSpace(draft.Query))
		var rows []uikit.PermissionMatrixRow
		for _, g := range draft.Grants {
			if filtering && draft.OnlyGranted && !draft.GrantedSnapshot[g.ConnectionID] {
				continue
			}
			if filtering && query != "" && !strings.Contains(strings.ToLower(g.Name+" "+g.Endpoint), query) {
				continue
			}
			rows = append(rows, uikit.PermissionMatrixRow{
				ID: g.ConnectionID, Name: g.Name, Protocol: g.Protocol, Endpoint: g.Endpoint,
				Warning: g.Warning.String(), Merged: len(g.Sources),
				SupportsFiles: g.SupportsFiles, SupportsSSH: g.SupportsSSH,
				AllowRead: g.AllowRead, AllowWrite: g.AllowWrite, AllowExec: g.AllowExec,
			})
		}
		empty := locales.MessageNoMatchingConnections()
		if len(draft.Grants) == 0 {
			empty = locales.MessageNoSavedConnectionsYet()
		}
		uikit.PermissionMatrix(c, rows, empty, func(id, capability string, enabled bool) {
			for i := range draft.Grants {
				if draft.Grants[i].ConnectionID == id {
					draft.Grants[i].setPermission(capability, enabled)
					// 标题处的计数与下方命令执行提示在矩阵之前绘制，需要重绘一帧。
					c.Invalidate()
					return
				}
			}
		})

		if execs > 0 {
			uikit.Banner(c, locales.McpCommandGrantWarning(execs),
				uikit.BannerProps{Tone: uikit.ToneWarning, Icon: uikit.IconSVG(uikit.IconAlertCircle)})
		} else {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N6).Children(func() {
				uikit.Icon(c, uikit.IconInfo, uikit.IconProps{Size: uikit.SizeXs, Tone: uikit.ToneSecondary})
				uikit.Text(c, locales.MessageSshFileAccessUsesSftpCommandExecutionCanReadAndWriteRemoteFiles(), uikit.TextProps{
					Style: uikit.TextCaption, Tone: uikit.ToneSecondary,
				}).Grow(1).MinWidth(0)
			})
		}
	})
}

func (a *app) mcpKeyDirsSection(c *ui.Context, draft *mcpKeyModalDraft) {
	t := theme.Current()
	ui.Column(c).FillWidth().Gap(t.Space.N10).Children(func() {
		mcpKeySectionHeader(c, locales.MessageLocalDirectories(), locales.MessageLimitMcpTransfersToTheseLocalDirectoriesAndTheirSubdirectoriesFilePoliciesStill(), nil)
		uikit.Inset(c, uikit.InsetProps{Raised: true}).FillWidth().Padding(0).Clip().Children(func() {
			uikit.DirectoryListEditor(c.Key("up-dirs"), draft.UploadDirs, uikit.DirectoryListEditorProps{
				Title:       locales.MessageUploadSources(),
				Description: locales.MessageLocalFilesCanBeReadFromTheseDirectoriesAndUploadedToTheRemote(),
				Icon:        uikit.IconUpload,
				OnAdd:       func() { a.pickMCPDirectory(true) },
				OnRemove: func(idx int) {
					draft.UploadDirs = slices.Delete(draft.UploadDirs, idx, idx+1)
				},
			})
			uikit.Divider(c)
			uikit.DirectoryListEditor(c.Key("down-dirs"), draft.DownloadDirs, uikit.DirectoryListEditorProps{
				Title:       locales.MessageDownloadDestinations(),
				Description: locales.MessageRemoteFilesCanBeDownloadedToTheseDirectories(),
				Icon:        uikit.IconDownload,
				OnAdd:       func() { a.pickMCPDirectory(false) },
				OnRemove: func(idx int) {
					draft.DownloadDirs = slices.Delete(draft.DownloadDirs, idx, idx+1)
				},
			})
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
	title := fmt.Sprintf(locales.MessageDisableKey(), dlg.Name)
	btnLabel := locales.MessageDisableKey2()
	variant := uikit.ButtonPrimary
	if isRevoke {
		title = fmt.Sprintf(locales.MessageRevokeKey(), dlg.Name)
		btnLabel = locales.MessageRevokeKey2()
		variant = uikit.ButtonDanger
	}
	uikit.Modal(c.Key("mcp-key-confirm"), &dlg.Open, uikit.ModalProps{
		Controller: &dlg.modal,
		Title:      title,
		Compact:    true,
		Width:      t.C.Mcp.DialogWidth,
		Disabled:   a.mcpBusy(),
	}, func(parts uikit.ModalParts) {
		uikit.Text(
			c,
			locales.MessageImmediatelyRejectSubsequentMcpRequestsUsingThisKey(),
			uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
		)
		uikit.Text(
			c,
			locales.McpCancelQueuedTasks(a.mcpQueuedTasks(dlg.KeyID)),
			uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
		)
		uikit.Text(
			c,
			locales.MessageRetainAuditHistoryAndTheKeyNameRecordedAtTheTime(),
			uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
		)
		if isRevoke {
			uikit.Banner(c, locales.MessageToSuspendAccessTemporarilyChooseDisableKeyPermanentRevocationCannotBeUndone(), uikit.BannerProps{
				Tone: uikit.ToneDanger, Icon: uikit.IconSVG(uikit.IconAlertCircle),
			})
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).Clicked() {
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
		Title:      fmt.Sprintf(locales.MessageDeletePolicy(), dlg.Name),
		Compact:    true,
		Width:      t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, locales.MessageAfterDeletionThisPolicyWillNoLongerBlockFutureOperationsThisCannotBe(), uikit.TextProps{
			Style: uikit.TextSmall, Tone: uikit.ToneSecondary,
		})
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			if uikit.Button(c, locales.MessageDeletePolicy2(), uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked() {
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
		Title:   locales.MessageEditListenerConfiguration(),
		Compact: true,
		Width:   t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, locales.MessageOnlyLocalListeningIsSupportedSavingAlsoUpdatesTheClientConfiguration(), uikit.TextProps{
			Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
		})
		uikit.FormRow(c, locales.MessageListeningHost(), uikit.FormRowProps{Align: uikit.RowAlignCenter}, func() ui.Element {
			input := uikit.Input(c, &a.mcp.hostDraft, uikit.InputProps{
				Label: locales.MessageListeningHost(), Placeholder: "127.0.0.1", Mono: true, DeferStyle: true,
			})
			host = strings.TrimSpace(a.mcp.hostDraft)
			address, err := url.Parse("http://" + net.JoinHostPort(host, "1"))
			switch {
			case host == "":
				hostError = locales.MessageEnterAListeningHost()
			case err != nil || address.Hostname() != host:
				hostError = locales.MessageEnterOnlyAnIpAddressOrHostnameWithoutASchemePathOrPort()
			case strings.Contains(host, ":") && net.ParseIP(host) == nil:
				hostError = locales.MessageEnterAValidIpv6AddressOrAHostnameWithoutAPort()
			}
			uikit.StyleFormControl(input, hostError)
			if hostError != "" {
				uikit.FieldMessage(c, hostError, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
			}
			return input
		})
		uikit.FormRow(c, locales.MessageListeningPort(), uikit.FormRowProps{Align: uikit.RowAlignCenter}, func() ui.Element {
			return uikit.PortInput(
				c,
				&a.mcp.portDraft,
				uikit.InputProps{Label: locales.MessageListeningPort(), Placeholder: "7801"},
			)
		})
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			portNum, err := strconv.Atoi(a.mcp.portDraft)
			valid := hostError == "" && err == nil && portNum >= 1 && portNum <= 65535
			if uikit.Button(
				c,
				locales.MessageSaveConfiguration(),
				uikit.ButtonProps{Variant: uikit.ButtonPrimary, Disabled: !valid},
			).Clicked() {
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
		Title:   locales.MessageAuditRetention(),
		Compact: true,
		Width:   t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, locales.MessageCleanupRemovesFinishedOperationsAndTheirPerFileDetailsActiveTasksUnverifiedStates(), uikit.TextProps{
			Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space.N8).Children(func() {
			uikit.Text(c, locales.MessageRetentionPeriod(), uikit.TextProps{Style: uikit.TextSmall})
			previousDays := a.mcp.retentionDays
			uikit.FilterChip(c, locales.MessageKeepTheLast(), &a.mcp.retentionDays, []uikit.Option[string]{
				{Value: "7", Label: locales.MessageDays()},
				{Value: "30", Label: locales.MessageDays2()},
				{Value: "90", Label: locales.MessageDays3()},
			}, uikit.FilterChipProps{Key: "ret-days"})
			if previousDays != a.mcp.retentionDays {
				a.configureMCPBackend()
			}
		})
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, locales.CommonClose(), uikit.ButtonProps{}).Clicked() {
				parts.RequestDismiss()
			}
			if uikit.Button(c, locales.MessageCleanUpRecords(), uikit.ButtonProps{Variant: uikit.ButtonDanger}).Clicked() {
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
		Title:   locales.MessageClientConfiguration(),
		Compact: true,
		Width:   t.C.Mcp.DialogWidth,
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, locales.MessageConfigurationCopiedReplaceYourSecretKeyWithYourSavedKey(), uikit.TextProps{
			Style: uikit.TextFootnote, Tone: uikit.ToneSecondary,
		})
		if uikit.CodeBlock(c.Key("client-config"), locales.MessageConfigurationContent(), cfg).Copy {
			mygo.Clipboard.WriteText(cfg)
		}
		ui.Row(c).FillWidth().Justify(ui.End).Gap(t.Space.N8).Children(func() {
			if uikit.Button(c, locales.MessageDone2(), uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Clicked() {
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
			return locales.MessageInvalidRegularExpression() + err.Error()
		}
	case "command":
		if _, err := shellcmd.ParseRule(r.Pattern, r.IgnoreCase); err != nil {
			return err.Error()
		}
	}
	return ""
}
