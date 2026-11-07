package desktop

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/egoist/mygo/i18n"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/locales"
	"sshdesk/internal/mcpserver"
	"sshdesk/internal/store"
	"sshdesk/internal/uikit"
)

type mcpData struct {
	payload                             *store.MCPAuditPayload
	detailView                          *mcpAuditItem
	detail                              *store.MCPOperation
	detailMissing                       bool
	fileTotals                          map[string]int
	fileStates                          map[string]map[string]int
	keys                                []store.MCPKey
	policies                            []store.MCPPolicy
	operations                          []store.MCPOperation
	files                               map[string][]store.MCPFileEvent
	tasks                               map[int]store.MCPTask
	total, filtered, calls, intercepted int
}

func (a *app) initMCPBackend() error {
	b := &mcpDesktop{a: a, sessions: make(map[string]*fileSession)}
	a.mcpBackend = b
	service, err := mcpserver.New(
		a.ctx,
		a.store,
		b,
		appVersion(),
		func() {
			if a.window != nil {
				a.window.Update(func() { a.refreshMCPBackend() })
			}
		},
	)
	if err != nil {
		return err
	}
	b.service = service
	b.connections, err = b.captureConnections()
	if err != nil {
		return err
	}
	data, err := a.readMCPData(store.MCPOperationFilter{Limit: 20}, "")
	if err != nil {
		return err
	}
	a.applyMCPData(data)
	return nil
}

func (a *app) startMCPBackend() {
	if !a.businessReady() {
		return
	}
	if a.mcpBackend == nil || a.mcpBackend.service == nil {
		return
	}
	s := a.mcpBackend.service
	cfg, _, _, _ := s.Status()
	if cfg.Enabled {
		a.sshWorkers.Go(func() {
			err := s.Start(a.ctx)
			a.window.Update(func() {
				if err != nil {
					a.persistenceNotice(locales.MessageUnableToStartTheMcpService, err)
				}
				a.refreshMCPBackend()
			})
		})
	}
}

func (a *app) mcpFilter() store.MCPOperationFilter {
	if a.mcp.auditResultFilter != "intercepted" {
		a.mcp.auditPolicyID = ""
	}
	f := store.MCPOperationFilter{Limit: 20, Offset: max(a.mcp.auditPage-1, 0) * 20}
	if a.mcp.auditKeyFilter != "all" {
		f.KeyID = a.mcp.auditKeyFilter
	}
	f.ConnectionIDs = a.mcpConnectionFilterIDs(a.mcp.auditConnFilter)
	if a.mcp.auditTypeFilter != "all" {
		if a.mcp.auditTypeFilter == "policy." {
			f.ActionPrefix = "policy."
		} else if a.mcp.auditTypeFilter == "mcp." {
			f.ActionPrefix = "mcp."
		} else {
			f.Action = a.mcp.auditTypeFilter
		}
	}
	switch a.mcp.auditResultFilter {
	case "intercepted":
		f.Statuses = []string{"intercepted", "partial_intercepted"}
		f.PolicyID = a.mcp.auditPolicyID
	case "denied":
		f.Statuses = []string{"permission_denied", "authentication_failed"}
	case "done":
		f.Status = "completed"
	}
	if a.mcp.auditTimeFilter == "today" {
		now := time.Now()
		f.Since = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	} else if a.mcp.auditTimeFilter == "7d" {
		f.Since = time.Now().AddDate(0, 0, -7)
	} else if a.mcp.auditTimeFilter == "30d" {
		f.Since = time.Now().AddDate(0, 0, -30)
	}
	return f
}

func (a *app) tickMCPBackend(c *ui.Context) {
	if a.mcpBackend == nil || a.mcpBackend.service == nil {
		return
	}
	f := a.mcpFilter()
	signature := fmt.Sprintf("%+v/%s/%t", f, a.mcp.selectedAuditID, a.mcp.auditDrawerOpen)
	if a.mcp.query != signature {
		a.mcp.query = signature
		a.refreshMCPBackend()
	}
	if a.activeSection() == sectionMCP {
		if time.Since(a.mcp.refreshed) >= time.Second {
			a.refreshMCPBackend()
		}
		c.After(time.Second)
	}
}

func (a *app) readMCPData(filter store.MCPOperationFilter, detailID string) (mcpData, error) {
	data := mcpData{files: make(map[string][]store.MCPFileEvent), fileTotals: make(map[string]int), fileStates: make(map[string]map[string]int), tasks: make(map[int]store.MCPTask)}
	var err error
	data.keys, err = a.store.MCPKeys(a.ctx)
	if err != nil {
		return data, err
	}
	data.policies, err = a.store.MCPPolicies(a.ctx)
	if err != nil {
		return data, err
	}
	data.operations, data.filtered, err = a.store.MCPOperations(a.ctx, filter)
	if err != nil {
		return data, err
	}
	_, data.total, err = a.store.MCPOperations(a.ctx, store.MCPOperationFilter{Limit: 1})
	if err != nil {
		return data, err
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	_, data.calls, err = a.store.MCPOperations(a.ctx, store.MCPOperationFilter{Since: today, Limit: 1, ToolsOnly: true})
	if err != nil {
		return data, err
	}
	_, data.intercepted, err = a.store.MCPOperations(a.ctx, store.MCPOperationFilter{Since: today, Statuses: []string{"intercepted", "partial_intercepted"}, Limit: 1})
	if err != nil {
		return data, err
	}
	operations := slices.Clone(data.operations)
	if detailID != "" && !slices.ContainsFunc(operations, func(op store.MCPOperation) bool { return op.ID == detailID }) {
		op, err := a.store.MCPOperation(a.ctx, detailID)
		if errors.Is(err, sql.ErrNoRows) {
			data.detailMissing = true
		} else if err != nil {
			return data, err
		} else {
			data.detail = &op
			operations = append(operations, op)
		}
	}
	for _, op := range operations {
		if op.ID == detailID {
			selected := op
			data.detail = &selected
		}
		data.files[op.ID], data.fileTotals[op.ID], data.fileStates[op.ID], err = a.store.MCPFileSummary(a.ctx, op.ID, 1000, 0)
		if err != nil {
			return data, err
		}
		if op.TaskID > 0 {
			task, err := a.store.MCPTask(a.ctx, op.TaskID)
			if err != nil {
				return data, err
			}
			data.tasks[task.ID] = task
		}
	}
	if detailID != "" {
		payload, err := a.store.MCPPayload(a.ctx, detailID)
		if err == nil {
			data.payload = &payload
		} else if !errors.Is(err, sql.ErrNoRows) {
			return data, err
		}
	}
	if data.detail != nil {
		op := data.detail
		view, err := mcpAuditView(*op, data.files[op.ID], data.tasks[op.TaskID])
		if err != nil {
			return data, err
		}
		view.FileTotal, view.FileCounts = data.fileTotals[op.ID], data.fileStates[op.ID]
		if err := mcpAuditPayloadView(&view, *op, data.payload); err != nil {
			return data, err
		}
		data.detailView = &view
	}
	return data, nil
}

func (a *app) refreshMCPBackend() {
	b := a.mcpBackend
	if b == nil || b.service == nil {
		return
	}
	if b.refreshing {
		b.refreshDue = true
		return
	}
	connections, err := b.captureConnections()
	if err != nil {
		a.persistenceNotice(locales.MessageUnableToReadMcpConnections, err)
		return
	}
	b.connections = connections
	b.refreshing = true
	a.mcp.refreshed = time.Now()
	filter := a.mcpFilter()
	detailID := a.mcp.selectedAuditID
	if !a.mcp.auditDrawerOpen {
		detailID = ""
	}
	var data mcpData
	a.persist(func() error { var err error; data, err = a.readMCPData(filter, detailID); return err }, func(err error) {
		b.refreshing = false
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToReadMcpData, err)
		} else {
			a.applyMCPData(data)
		}
		if b.refreshDue {
			b.refreshDue = false
			a.refreshMCPBackend()
		}
	})
}

func (a *app) applyMCPData(data mcpData) {
	b := a.mcpBackend
	b.keys, b.policies = data.keys, data.policies
	cfg, running, started, _ := b.service.Status()
	a.mcp.running, a.mcp.host, a.mcp.port = running, cfg.Host, strconv.Itoa(cfg.Port)
	if !a.mcp.retentionOpen {
		a.mcp.retentionDays = strconv.Itoa(cfg.RetentionDays)
	}
	a.mcp.uptime = "—"
	if running {
		a.mcp.uptime = time.Since(started).Round(time.Second).String()
	}
	a.mcp.todayCalls, a.mcp.todayIntercepts = data.calls, data.intercepted
	a.mcp.auditTotal, a.mcp.auditFiltered = data.total, data.filtered
	a.mcp.keys = nil
	for _, key := range data.keys {
		grants := a.mcpGrants()
		var summary []string
		for i := range grants {
			grants[i].loadPermissions(key.Grants)
			var capabilities []string
			if grants[i].AllowRead {
				capabilities = append(capabilities, locales.MessageRead())
			}
			if grants[i].AllowWrite {
				capabilities = append(capabilities, locales.MessageWrite())
			}
			if grants[i].AllowExec {
				capabilities = append(capabilities, locales.MessageExecute())
			}
			if len(capabilities) > 0 {
				summary = append(summary, grants[i].Name+" ("+strings.Join(capabilities, "/")+")")
			} else if !grants[i].Warning.Empty() {
				summary = append(summary, grants[i].Name+locales.MessageRequiresAuthorizationAgain())
			}
		}
		lastUsed := locales.MessageNeverUsed()
		if !key.LastUsedAt.IsZero() {
			lastUsed = localizedDate(key.LastUsedAt.Local(), i18n.MediumStyle, i18n.ShortStyle)
		}
		a.mcp.keys = append(a.mcp.keys, mcpSecretKey{ID: key.ID, Name: key.Name, Prefix: key.Prefix, State: key.State,
			GrantsSummary: strings.Join(
				summary,
				locales.CommonListSeparator(),
			), PolicyHint: locales.MessageSubjectToEnabledInterceptionPolicies(), Grants: grants,
			UploadDirs: key.UploadDirs, DownloadDirs: key.DownloadDirs, LastUsed: lastUsed, CreatedAt: localizedDate(key.CreatedAt.Local(), i18n.MediumStyle, i18n.ShortStyle)})
	}
	a.mcp.policies = nil
	for _, p := range data.policies {
		view := mcpPolicy{ID: p.ID, Version: p.Version, Name: p.Name, Description: p.Description, Enabled: p.Enabled,
			Category: p.Category, AllConnections: p.AllConnections, Connections: slices.Clone(p.ConnectionIDs),
			AllKeys: p.AllKeys, KeyIDs: slices.Clone(p.KeyIDs), HitsSummary: "—"}
		for _, key := range data.keys {
			if slices.Contains(p.KeyIDs, key.ID) {
				view.KeyNames = append(view.KeyNames, key.Name)
			}
		}
		var names []string
		seen := map[string]bool{}
		for _, c := range b.connections {
			matches := slices.Contains(p.ConnectionIDs, c.ID)
			if p.AllConnections {
				matches = p.Category == "files" && c.Supports("read") || p.Category == "ssh" && c.Supports("exec")
				if matches {
					view.Connections = append(view.Connections, c.ID)
				}
			}
			if matches && !seen[c.DisplayID()] {
				seen[c.DisplayID()] = true
				names = append(names, c.Name)
			}
		}
		view.ConnSummary = strings.Join(names, ", ")
		view.ConnSub = locales.McpSelectedConnectionsCount(len(names))
		if p.AllConnections {
			view.ConnSummary, view.ConnSub = locales.MessageAllConnections(), locales.MessageIncludesConnectionsAddedLater()
		}
		if p.Category == "ssh" {
			for _, r := range p.CommandRules {
				view.CmdRules = append(view.CmdRules, mcpCommandRule{Mode: r.Mode, Pattern: r.Pattern, IgnoreCase: r.IgnoreCase})
			}
		} else {
			for _, r := range p.FileRules {
				view.FileRules = append(view.FileRules, mcpFileRule{Side: r.Side, Kind: r.Kind, Path: r.Path, IncludeSubdir: r.IncludeSubdirs, DeniedActions: r.DeniedActions})
			}
		}
		if len(view.CmdRules) > 0 {
			view.RulePrimary = view.CmdRules[0].Pattern
		}
		if len(view.FileRules) > 0 {
			view.RulePrimary = view.FileRules[0].Path
		}
		view.RuleSub = locales.McpRuleCount(len(view.CmdRules) + len(view.FileRules))
		if slices.ContainsFunc(view.CmdRules, func(r mcpCommandRule) bool { return r.Mode == "contains" }) {
			view.RuleSub += locales.MessageIncludesFuzzyMatchingFalsePositivesArePossible()
		}
		a.mcp.policies = append(a.mcp.policies, view)
	}
	a.mcp.audits = nil
	a.mcp.detail = nil
	if data.detailMissing {
		a.mcp.selectedAuditID = ""
		a.mcp.auditDrawerOpen = false
	}
	a.mcp.detail = data.detailView
	for _, op := range data.operations {
		view, err := mcpAuditView(op, data.files[op.ID], data.tasks[op.TaskID])
		if err != nil {
			a.persistenceNotice(locales.MessageUnableToParseMcpAuditData, err)
			return
		}
		view.FileTotal, view.FileCounts = data.fileTotals[op.ID], data.fileStates[op.ID]
		a.mcp.audits = append(a.mcp.audits, view)
	}
	if a.mcp.selectedAuditID == "" && len(a.mcp.audits) > 0 {
		a.mcp.selectedAuditID = a.mcp.audits[0].ID
	}
	a.mcp.refreshed = time.Now()
	if a.window != nil {
		a.window.Invalidate()
	}
}

func (a *app) mcpConnectionOptions(category int, all bool) []uikit.Option[string] {
	var options []uikit.Option[string]
	if all {
		options = append(options, uikit.Option[string]{Value: "all", Label: locales.MessageAll()})
	}
	if a.mcpBackend == nil {
		return options
	}
	for _, c := range a.mcpBackend.connections {
		if category == 0 && !c.Supports("exec") || category == 1 && !c.Supports("read") {
			continue
		}
		if c.ID != c.DisplayID() {
			continue
		}
		options = append(options, uikit.Option[string]{Value: c.ID, Label: c.Name + " (" + strings.ToUpper(c.Protocol) + ")"})
	}
	return options
}

func (a *app) mcpKeyOptions(all bool) []uikit.Option[string] {
	var options []uikit.Option[string]
	if all {
		options = append(options, uikit.Option[string]{Value: "all", Label: locales.MessageAll()})
	}
	for _, k := range a.mcp.keys {
		if k.State != "revoked" || all {
			options = append(options, uikit.Option[string]{Value: k.ID, Label: k.Name})
		}
	}
	return options
}

func (a *app) mcpWork(work func() error, done func(error)) {
	b := a.mcpBackend
	if b == nil || b.service == nil {
		a.persistenceNotice(
			locales.MessageMcpUnavailable,
			locales.NewError(locales.MessageTheMcpBackendWasNotInitializedSuccessfully),
		)
		return
	}
	if b.busy {
		a.persistenceNotice(
			locales.MessageMcpOperationNotPerformed,
			locales.NewError(locales.MessageAnotherMcpManagementOperationIsInProgressTryAgainShortly),
		)
		a.refreshMCPBackend()
		return
	}
	b.busy = true
	a.sshWorkers.Go(func() {
		err := work()
		a.window.Update(func() {
			b.busy = false
			if err != nil {
				a.persistenceNotice(locales.MessageMcpOperationFailed, err)
			}
			if done != nil {
				done(err)
			}
			a.refreshMCPBackend()
		})
	})
}

func (a *app) mcpBusy() bool { return a.mcpBackend != nil && a.mcpBackend.busy }

func (a *app) mcpQueuedTasks(keyID string) int {
	count := 0
	for _, task := range a.transfers.tasks {
		if task.mcp != nil && task.mcp.KeyID == keyID && task.state == transferQueued {
			count++
		}
	}
	return count
}

func (a *app) mcpPausedTasks() int {
	count := 0
	for _, task := range a.transfers.tasks {
		if task.mcp != nil && task.state == transferPaused {
			count++
		}
	}
	return count
}

func (a *app) configureMCPBackend() {
	host, port, enabled, retention := a.mcp.host, a.mcp.port, a.mcp.running, a.mcp.retentionDays
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		a.persistenceNotice(locales.MessageInvalidListeningPort, err)
		return
	}
	days, err := strconv.Atoi(retention)
	if err != nil {
		a.persistenceNotice(locales.MessageInvalidAuditRetentionPeriod, err)
		return
	}
	a.mcpWork(func() error {
		return a.mcpBackend.service.Configure(a.ctx, mcpserver.Config{Host: host, Port: portNumber, Enabled: enabled, RetentionDays: days})
	}, nil)
}

func (a *app) pruneMCPBackend() {
	days, err := strconv.Atoi(a.mcp.retentionDays)
	if err != nil {
		a.persistenceNotice(locales.MessageInvalidRetentionPeriod, err)
		return
	}
	a.mcpWork(func() error { return a.store.MCPPrune(a.ctx, time.Now().AddDate(0, 0, -days)) }, nil)
}

func (a *app) saveMCPBackendKey() {
	draft := &a.mcp.keyModal
	key := store.MCPKey{ID: draft.EditingID, Name: draft.Name, State: "active", UploadDirs: slices.Clone(draft.UploadDirs), DownloadDirs: slices.Clone(draft.DownloadDirs)}
	if key.ID != "" {
		for _, old := range a.mcpBackend.keys {
			if old.ID == key.ID {
				key.State = old.State
			}
		}
	}
	for _, g := range draft.Grants {
		for _, source := range g.Sources {
			if source.Read || source.Write || source.Exec {
				key.Grants = append(key.Grants, source)
			}
		}
	}
	var secret string
	a.mcpWork(func() error {
		if key.ID != "" {
			return a.mcpBackend.service.UpdateKey(a.ctx, key)
		}
		var err error
		key, secret, err = a.mcpBackend.service.CreateKey(a.ctx, key)
		return err
	}, func(err error) {
		if err != nil {
			draft.NameError = locales.ErrorText(err)
			return
		}
		a.mcp.selectedKeyID = key.ID
		if draft.EditingID != "" {
			draft.modal.Close()
		} else {
			draft.GeneratedKey, draft.OneTimeView, draft.SavedConfirmed = secret, true, false
		}
	})
}

func (a *app) saveMCPBackendPolicy() {
	form := &a.mcp.policyDraft
	form.NameError, form.SaveError = locales.Text{}, locales.Text{}
	if strings.TrimSpace(form.Name) == "" {
		form.NameError = locales.Deferred(locales.MessageEnterAPolicyName)
	} else if len(form.Name) > 200 {
		form.NameError = locales.Deferred(locales.MessageThePolicyNameIsTooLongUseNoMoreThanBytes)
	}
	if !form.NameError.Empty() {
		form.Scroll.ScrollTo(0, 0, ui.Animation{})
		return
	}
	draft := a.mcp.policyDraft
	p := store.MCPPolicy{ID: draft.EditingID, Version: draft.Version + 1, Name: draft.Name, Description: draft.Description,
		Enabled: draft.Enabled, AllConnections: len(draft.Connections) == 0, AllKeys: len(draft.SelectedKeys) == 0,
		Category: "files", ConnectionIDs: slices.Clone(draft.Connections), KeyIDs: slices.Clone(draft.SelectedKeys)}
	if draft.CategorySeg == 0 {
		p.Category = "ssh"
		for _, r := range draft.CmdRules {
			p.CommandRules = append(p.CommandRules, store.MCPCommandRule{Mode: r.Mode, Pattern: r.Pattern, IgnoreCase: r.Mode == "command" && r.IgnoreCase})
		}
	} else {
		for _, r := range draft.FileRules {
			p.FileRules = append(p.FileRules, store.MCPFileRule{Side: r.Side, Kind: r.Kind, Path: r.Path, IncludeSubdirs: r.IncludeSubdir, DeniedActions: maps.Clone(r.DeniedActions)})
		}
	}
	if err := mcpserver.ValidatePolicy(p); err != nil {
		form.SaveError = locales.ErrorText(err)
		form.Scroll.ScrollTo(0, 0, ui.Animation{})
		return
	}
	a.mcpWork(func() error { return a.mcpBackend.service.SavePolicy(a.ctx, p) }, func(err error) {
		if err == nil {
			form.modal.Close()
		} else {
			form.SaveError = locales.ErrorText(err)
			form.Scroll.ScrollTo(0, 0, ui.Animation{})
		}
	})
}

func (a *app) setMCPBackendKeyState(id, state string, done func(error)) {
	a.mcpWork(func() error { return a.mcpBackend.service.SetKeyState(a.ctx, id, state) }, done)
}

func (a *app) toggleMCPBackendPolicy(id string) {
	for _, p := range a.mcpBackend.policies {
		if p.ID == id {
			a.mcpWork(func() error {
				return a.mcpBackend.service.SetPolicyEnabled(a.ctx, id, !p.Enabled)
			}, nil)
			return
		}
	}
}

func (a *app) deleteMCPBackendPolicy(id string) {
	a.mcpWork(func() error { return a.mcpBackend.service.DeletePolicy(a.ctx, id) }, func(err error) {
		if err == nil {
			a.mcp.policyDelete.modal.Close()
			a.mcp.selectedPolicyID = ""
		}
	})
}

func (a *app) controlMCPBackendTask(id, action string) {
	number, err := strconv.Atoi(id)
	if err != nil {
		a.persistenceNotice(locales.MessageInvalidTaskId, err)
		return
	}
	a.mcpWork(func() error { return a.mcpBackend.ControlTask(a.ctx, number, action) }, nil)
}

func (a *app) stopMCPForQuit(done func(error)) {
	if a.mcpBackend == nil || a.mcpBackend.service == nil {
		done(nil)
		return
	}
	a.sshWorkers.Go(func() {
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		defer cancel()
		err := a.mcpBackend.service.Stop(ctx)
		a.window.Update(func() { done(err) })
	})
}

func mcpActionName(value string) string {
	if strings.HasPrefix(value, "mcp.") {
		return locales.MessageProtocolRequest()
	}
	return map[string]string{"connections_list": locales.MessageConnectionDiscovery(), "ssh_exec": locales.MessageCommandExecution(), "files_list": locales.MessageDirectoryListing(), "files_stat": locales.MessageFileInformation(), "files_read": locales.MessageFileRead(), "files_write": locales.MessageFileWrite(), "files_upload": locales.MessageFileUpload(), "files_download": locales.MessageFileDownload(), "authentication": locales.MessageAuthenticationFailed(), "tasks_list": locales.MessageTaskList(), "tasks_get": locales.MessageTaskLookup(), "tasks_cancel": locales.MessageTaskCancellation()}[value]
}

func mcpFileActionName(value string) string {
	if value == "exec" {
		return locales.MessageCommandExecution()
	}
	for _, action := range uikit.DefaultFilePolicyActions() {
		if action.ID == value {
			return action.Label
		}
	}
	return value
}

func mcpStageName(value string) string {
	switch value {
	case "accepted":
		return locales.McpStateAccepted()
	case "validated":
		return locales.McpStateValidated()
	case "queued":
		return locales.MessageQueued()
	case "running":
		return locales.MessageRunning()
	case "waiting":
		return locales.McpStateWaiting()
	case "waiting_user":
		return locales.McpStateWaitingUser()
	case "paused":
		return locales.MessagePaused()
	case "completed":
		return locales.MessageCompleted()
	case "failed":
		return locales.MessageFailed()
	case "intercepted":
		return locales.MessageBlockedByPolicy()
	case "partial_intercepted":
		return locales.MessagePartiallyBlocked()
	case "permission_denied":
		return locales.MessagePermissionDenied()
	case "authentication_failed":
		return locales.MessageAuthenticationFailed()
	case "cancel_requested":
		return locales.MessageCancellationRequested()
	case "cancelled":
		return locales.McpStateCancelled()
	case "timed_out":
		return locales.McpStateTimedOut()
	case "interrupted":
		return locales.McpStateInterrupted()
	case "skipped":
		return locales.McpStateSkipped()
	case "cleanup_pending":
		return locales.McpStateCleanupPending()
	case "cleaning":
		return locales.McpStateCleaning()
	default:
		return value
	}
}

func mcpUIStatus(value string) uikit.Status {
	switch value {
	case "queued":
		return uikit.StatusConnecting
	case "running", "waiting", "waiting_user":
		return uikit.StatusRunning
	case "paused", "interrupted":
		return uikit.StatusPaused
	case "completed":
		return uikit.StatusDone
	case "intercepted":
		return uikit.StatusIntercepted
	case "partial_intercepted":
		return uikit.StatusPartialIntercepted
	case "permission_denied":
		return uikit.StatusPermissionDenied
	case "authentication_failed":
		return uikit.StatusAuthFailed
	case "cancel_requested", "cancelling":
		return uikit.StatusStopping
	default:
		return uikit.StatusError
	}
}

func mcpAuditView(op store.MCPOperation, files []store.MCPFileEvent, task store.MCPTask) (mcpAuditItem, error) {
	var request struct {
		Command string `json:"command"`
		Path    string `json:"path"`
		Local   string `json:"local_path"`
	}
	if err := json.Unmarshal(op.Request, &request); err != nil {
		return mcpAuditItem{}, err
	}
	var target mcpserver.Connection
	if err := json.Unmarshal(op.Target, &target); err != nil {
		return mcpAuditItem{}, err
	}
	view := mcpAuditItem{Action: op.Action, ID: op.ID, KeyID: op.KeyID, KeyName: op.KeyName, ConnectionID: op.ConnectionID,
		Time:     localizedDate(op.StartedAt.Local(), i18n.NoStyle, i18n.MediumStyle),
		Day:      localizedDateOptions(op.StartedAt.Local(), i18n.DateOptions{Skeleton: "MMMd"}),
		ConnName: target.Name, ConnHost: target.Host, ConnPort: strconv.Itoa(target.Port), ConnUser: target.User,
		ConnProtocol: strings.ToUpper(target.Protocol), ActionType: mcpActionName(op.Action), Status: mcpUIStatus(op.Status),
		StartTime: localizedDate(op.StartedAt.Local(), i18n.MediumStyle, i18n.MediumStyle), EndTime: locales.MessageInProgress(), IsCommand: op.Action == "ssh_exec",
		CommandText: request.Command, RemotePath: request.Path, LocalPath: request.Local, OutputStatus: op.Error, Summary: request.Path}
	if view.ActionType == "" {
		view.ActionType = locales.MessageManagementOperation()
	}
	if view.IsCommand {
		view.Summary = strings.Join(strings.Fields(request.Command), " ")
	} else if strings.HasPrefix(op.Action, "mcp.") {
		view.Summary = strings.TrimPrefix(op.Action, "mcp.")
	}
	if !op.FinishedAt.IsZero() {
		view.EndTime = localizedDate(op.FinishedAt.Local(), i18n.MediumStyle, i18n.MediumStyle)
		view.Duration = op.FinishedAt.Sub(op.StartedAt).Round(time.Millisecond).String()
	} else {
		view.Duration = time.Since(op.StartedAt).Round(time.Second).String()
	}
	var result struct {
		ExitCode  *int `json:"exit_code"`
		Stdout    int  `json:"stdout_bytes"`
		Stderr    int  `json:"stderr_bytes"`
		Truncated bool `json:"truncated"`
	}
	if err := json.Unmarshal(op.Result, &result); err != nil {
		return view, err
	}
	view.ExitCodeText = locales.MessageUnknownOrNotExecuted()
	if result.ExitCode != nil {
		view.ExitCodeText = strconv.Itoa(*result.ExitCode)
	}
	if view.IsCommand && op.Error == "" {
		view.OutputStatus = locales.McpOutputSummary(
			transferBytes(int64(result.Stdout)),
			transferBytes(int64(result.Stderr)),
			strconv.FormatBool(result.Truncated),
		)
	}
	for _, event := range op.Timeline {
		view.Timeline = append(view.Timeline, uikit.TimelineItem{
			Stage: mcpStageName(event.Stage), Detail: event.Detail,
			Time: localizedDate(event.At.Local(), i18n.NoStyle, i18n.MediumStyle),
			Tone: uikit.ToneSecondary,
		})
	}
	for i, h := range op.Hits {
		category := locales.MessageFilePolicy()
		if h.Category == "ssh" {
			category = locales.MessageCommandPolicy()
		}
		view.PolicyHits = append(view.PolicyHits, mcpPolicyHitSnapshot{PolicyID: h.PolicyID, PolicyName: h.Name, Category: category, Version: h.Version, RuleSummary: h.Rule, Action: h.Action, ForbiddenText: mcpFileActionName(h.Action), HitCountText: fmt.Sprintf(
			locales.MessageMatchOf(),
			i+1,
			len(op.Hits),
		)})
	}
	latest := make(map[string]int)
	var summaries []store.MCPFileEvent
	for _, f := range files {
		key := f.Side + "\x00" + f.Path
		if i, ok := latest[key]; ok {
			summaries[i] = f
		} else {
			latest[key] = len(summaries)
			summaries = append(summaries, f)
		}
	}
	for _, f := range summaries {
		state := map[string]string{"completed": "ok", "intercepted": "intercepted", "failed": "failed", "skipped": "skipped", "running": "pending", "pending": "pending", "interrupted": "pending"}[f.Status]
		view.Files = append(view.Files, mcpAuditFileItem{Path: f.Path, State: state, Size: transferBytes(f.Bytes), PolicyTag: mcpFileActionName(f.Action)})
	}
	if task.ID > 0 {
		fraction := float32(0)
		if task.Total > 0 {
			fraction = float32(task.Done) / float32(task.Total)
		}
		view.Task = &mcpAuditTask{ID: strconv.Itoa(task.ID), Title: view.ActionType + " → " + target.Name,
			Subtitle: mcpStageName(task.State), Transferred: transferBytes(task.Done), Total: transferBytes(task.Total), Fraction: fraction,
			Paused: task.State == "paused" || task.State == "interrupted" || task.State == "failed" || task.State == "intercepted" || task.State == "partial_intercepted", CancelRequest: task.State == "cancel_requested" || task.State == "cancelled" || task.State == "completed"}
		if op.Action == "files_write" {
			view.Task.CleanupOnly = true
			view.Task.Title = locales.MessageOwnedTemporaryFileCleanup() + target.Name
			view.Task.Subtitle = map[string]string{"cleanup_pending": locales.MessageWaitingForAManualCleanupRetry(), "cleaning": locales.MessageCleaningUp(), "completed": locales.MessageCleanupComplete()}[task.State]
			view.Task.Paused = task.State == "cleanup_pending"
			view.Task.CancelRequest = task.State != "cleanup_pending"
		}
		if task.State == "waiting_user" && len(op.Timeline) > 0 {
			view.Task.WaitingPrompt = locales.MessageWaitingForUser() + op.Timeline[len(op.Timeline)-1].Detail
		}
		view.TransferStat = transferBytes(task.Done) + " / " + transferBytes(task.Total)
	}
	return view, nil
}

func mcpAuditPayloadView(view *mcpAuditItem, op store.MCPOperation, payload *store.MCPAuditPayload) error {
	view.DetailLoaded = true
	pretty := func(value []byte) (string, error) {
		if len(value) == 0 {
			return "", nil
		}
		var formatted bytes.Buffer
		if err := json.Indent(&formatted, value, "", "  "); err != nil {
			return "", err
		}
		return formatted.String(), nil
	}
	request, response := op.Request, op.Result
	if payload != nil {
		view.PayloadSaved = true
		request, response = payload.Request, payload.Response
		view.ResponseSaved = payload.Response != nil
	}
	var err error
	view.RequestJSON, err = pretty(request)
	if err != nil {
		return err
	}
	view.ResponseJSON, err = pretty(response)
	if err != nil {
		return err
	}
	if view.ResponseSaved {
		var metadata struct {
			Truncated bool `json:"capture_truncated"`
		}
		if err := json.Unmarshal(response, &metadata); err != nil {
			return err
		}
		view.ResponseTruncated = metadata.Truncated
	}
	view.FileBodyOmitted = view.PayloadSaved && (op.Action == "files_read" || op.Action == "files_write")
	if view.IsCommand && view.CommandText == "" && payload != nil {
		var envelope map[string]any
		if err := json.Unmarshal(request, &envelope); err != nil {
			return err
		}
		if params, ok := envelope["params"].(map[string]any); ok {
			if arguments, ok := params["arguments"].(map[string]any); ok {
				view.CommandText, _ = arguments["command"].(string)
			}
		}
	}
	if view.ResponseSaved && view.IsCommand {
		type toolResult struct {
			StructuredContent struct {
				Stdout string `json:"stdout"`
				Stderr string `json:"stderr"`
				Error  struct {
					Hits []struct {
						PolicyID string `json:"policy_id"`
						Version  int    `json:"policy_version"`
						Rule     string `json:"rule"`
						Action   string `json:"action"`
						Matched  string `json:"matched"`
						Context  string `json:"context"`
						Via      string `json:"via"`
						Fallback string `json:"fallback"`
					} `json:"hits"`
				} `json:"error"`
			} `json:"structuredContent"`
		}
		var envelope struct {
			Data struct {
				Result    *toolResult `json:"result"`
				SSEEvents []struct {
					Result *toolResult `json:"result"`
				} `json:"sse_events"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response, &envelope); err != nil {
			return err
		}
		applyResult := func(result *toolResult) {
			if result == nil {
				return
			}
			content := result.StructuredContent
			view.Stdout, view.Stderr = content.Stdout, content.Stderr
			for _, evidence := range content.Error.Hits {
				for i := range view.PolicyHits {
					hit := &view.PolicyHits[i]
					if hit.PolicyID != evidence.PolicyID || hit.Version != evidence.Version ||
						hit.RuleSummary != evidence.Rule || hit.Action != evidence.Action {
						continue
					}
					hit.MatchSaved = true
					hit.MatchText, hit.MatchContext = evidence.Matched, evidence.Context
					hit.MatchVia, hit.MatchFallback = evidence.Via, evidence.Fallback
				}
			}
		}
		applyResult(envelope.Data.Result)
		for _, event := range envelope.Data.SSEEvents {
			applyResult(event.Result)
		}
	}
	return nil
}
