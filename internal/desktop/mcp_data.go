package desktop

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/mcpserver"
	"sshdesk/internal/store"
	"sshdesk/internal/uikit"
)

type mcpData struct {
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
	service, err := mcpserver.New(a.ctx, a.store, b, func() {
		if a.window != nil {
			a.window.Update(func() { a.refreshMCPBackend() })
		}
	})
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
					a.persistenceNotice("启动 MCP 服务失败", err)
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
	if a.mcp.auditConnFilter != "all" {
		f.ConnectionID = a.mcp.auditConnFilter
	}
	if a.mcp.auditTypeFilter != "all" {
		if a.mcp.auditTypeFilter == "策略管理" {
			f.ActionPrefix = "policy."
		} else {
			f.Action = mcpActionValue(a.mcp.auditTypeFilter)
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
	signature := fmt.Sprintf("%+v/%s", f, a.mcp.selectedAuditID)
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
		a.persistenceNotice("读取 MCP 连接失败", err)
		return
	}
	b.connections = connections
	b.refreshing = true
	a.mcp.refreshed = time.Now()
	filter := a.mcpFilter()
	detailID := a.mcp.selectedAuditID
	var data mcpData
	a.persist(func() error { var err error; data, err = a.readMCPData(filter, detailID); return err }, func(err error) {
		b.refreshing = false
		if err != nil {
			a.persistenceNotice("读取 MCP 数据失败", err)
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
			for _, g := range key.Grants {
				if g.ConnectionID == grants[i].ConnectionID {
					grants[i].AllowRead, grants[i].AllowWrite, grants[i].AllowExec = g.Read, g.Write, g.Exec
					grants[i].EndpointDigest = g.EndpointDigest
					var capabilities []string
					if g.Read {
						capabilities = append(capabilities, "读")
					}
					if g.Write {
						capabilities = append(capabilities, "写")
					}
					if g.Exec {
						capabilities = append(capabilities, "执行")
					}
					if len(capabilities) > 0 {
						summary = append(summary, grants[i].Name+" ("+strings.Join(capabilities, "/")+")")
					}
				}
			}
		}
		lastUsed := "从未使用"
		if !key.LastUsedAt.IsZero() {
			lastUsed = key.LastUsedAt.Local().Format("01-02 15:04")
		}
		a.mcp.keys = append(a.mcp.keys, mcpSecretKey{ID: key.ID, Name: key.Name, Prefix: key.Prefix, State: key.State,
			GrantsSummary: strings.Join(summary, "、"), PolicyHint: "受当前启用的拦截策略约束", Grants: grants,
			UploadDirs: key.UploadDirs, DownloadDirs: key.DownloadDirs, LastUsed: lastUsed, CreatedAt: key.CreatedAt.Local().Format("2006-01-02 15:04")})
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
		for _, c := range b.connections {
			matches := slices.Contains(p.ConnectionIDs, c.ID)
			if p.AllConnections {
				matches = (c.Protocol == "ssh") == (p.Category == "ssh")
				if matches {
					view.Connections = append(view.Connections, c.ID)
				}
			}
			if matches {
				names = append(names, c.Name)
			}
		}
		view.ConnSummary = strings.Join(names, ", ")
		view.ConnSub = fmt.Sprintf("%d 个连接", len(p.ConnectionIDs))
		if p.AllConnections {
			view.ConnSummary, view.ConnSub = "全部文件连接", "含后续新增连接"
			if p.Category == "ssh" {
				view.ConnSummary = "全部 SSH 连接"
			}
		}
		if p.Category == "ssh" {
			for _, r := range p.CommandRules {
				view.CmdRules = append(view.CmdRules, mcpCommandRule{Mode: r.Mode, Pattern: r.Pattern})
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
		view.RuleSub = fmt.Sprintf("共 %d 条规则", len(view.CmdRules)+len(view.FileRules))
		a.mcp.policies = append(a.mcp.policies, view)
	}
	a.mcp.audits = nil
	a.mcp.detail = nil
	if data.detailMissing {
		a.mcp.selectedAuditID = ""
		a.mcp.auditDrawerOpen = false
	}
	if data.detail != nil {
		op := data.detail
		view, err := mcpAuditView(*op, data.files[op.ID], data.tasks[op.TaskID])
		if err != nil {
			a.persistenceNotice("解析 MCP 审计失败", err)
			return
		}
		view.FileTotal, view.FileCounts = data.fileTotals[op.ID], data.fileStates[op.ID]
		a.mcp.detail = &view
	}
	for _, op := range data.operations {
		view, err := mcpAuditView(op, data.files[op.ID], data.tasks[op.TaskID])
		if err != nil {
			a.persistenceNotice("解析 MCP 审计失败", err)
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

func (a *app) mcpGrants() []mcpKeyGrant {
	var grants []mcpKeyGrant
	if a.mcpBackend == nil {
		return grants
	}
	for _, c := range a.mcpBackend.connections {
		grants = append(grants, mcpKeyGrant{ConnectionID: c.ID, Name: c.Name,
			Subtitle:      strings.ToUpper(c.Protocol) + " · " + c.User + "@" + c.Host,
			SupportsFiles: c.Protocol != "ssh", SupportsSSH: c.Protocol == "ssh"})
	}
	return grants
}

func (a *app) mcpConnectionOptions(category int, all bool) []uikit.Option[string] {
	var options []uikit.Option[string]
	if all {
		options = append(options, uikit.Option[string]{Value: "all", Label: "全部"})
	}
	if a.mcpBackend == nil {
		return options
	}
	for _, c := range a.mcpBackend.connections {
		if category == 0 && c.Protocol != "ssh" || category == 1 && c.Protocol == "ssh" {
			continue
		}
		options = append(options, uikit.Option[string]{Value: c.ID, Label: c.Name + " (" + strings.ToUpper(c.Protocol) + ")"})
	}
	return options
}

func (a *app) mcpKeyOptions(all bool) []uikit.Option[string] {
	var options []uikit.Option[string]
	if all {
		options = append(options, uikit.Option[string]{Value: "all", Label: "全部"})
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
		a.persistenceNotice("MCP 不可用", errors.New("MCP 后端未成功初始化"))
		return
	}
	if b.busy {
		return
	}
	b.busy = true
	a.sshWorkers.Go(func() {
		err := work()
		a.window.Update(func() {
			b.busy = false
			if err != nil {
				a.persistenceNotice("MCP 操作失败", err)
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
		a.persistenceNotice("无效的监听端口", err)
		return
	}
	days, err := strconv.Atoi(retention)
	if err != nil {
		a.persistenceNotice("无效的审计保留周期", err)
		return
	}
	a.mcpWork(func() error {
		return a.mcpBackend.service.Configure(a.ctx, mcpserver.Config{Host: host, Port: portNumber, Enabled: enabled, RetentionDays: days})
	}, nil)
}

func (a *app) pruneMCPBackend() {
	days, err := strconv.Atoi(a.mcp.retentionDays)
	if err != nil {
		a.persistenceNotice("无效的保留周期", err)
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
		if g.AllowRead || g.AllowWrite || g.AllowExec {
			key.Grants = append(key.Grants, store.MCPGrant{ConnectionID: g.ConnectionID, EndpointDigest: g.EndpointDigest, Read: g.AllowRead, Write: g.AllowWrite, Exec: g.AllowExec})
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
			draft.NameError = err.Error()
			return
		}
		a.mcp.selectedKeyID = key.ID
		if draft.EditingID != "" {
			draft.Open = false
		} else {
			draft.GeneratedKey, draft.OneTimeView, draft.SavedConfirmed = secret, true, false
		}
	})
}

func (a *app) saveMCPBackendPolicy() {
	form := &a.mcp.policyDraft
	form.NameError, form.SaveError = "", ""
	if strings.TrimSpace(form.Name) == "" {
		form.NameError = "请输入策略名称"
	} else if len(form.Name) > 200 {
		form.NameError = "策略名称过长，请缩短后重试（最多 200 字节）"
	}
	if form.NameError != "" {
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
			p.CommandRules = append(p.CommandRules, store.MCPCommandRule{Mode: r.Mode, Pattern: r.Pattern})
		}
	} else {
		for _, r := range draft.FileRules {
			p.FileRules = append(p.FileRules, store.MCPFileRule{Side: r.Side, Kind: r.Kind, Path: r.Path, IncludeSubdirs: r.IncludeSubdir, DeniedActions: maps.Clone(r.DeniedActions)})
		}
	}
	if err := mcpserver.ValidatePolicy(p); err != nil {
		form.SaveError = err.Error()
		form.Scroll.ScrollTo(0, 0, ui.Animation{})
		return
	}
	a.mcpWork(func() error { return a.mcpBackend.service.SavePolicy(a.ctx, p) }, func(err error) {
		if err == nil {
			a.mcp.policyModalOpen = false
		} else {
			form.SaveError = err.Error()
			form.Scroll.ScrollTo(0, 0, ui.Animation{})
		}
	})
}

func (a *app) setMCPBackendKeyState(id, state string) {
	for _, key := range a.mcpBackend.keys {
		if key.ID == id {
			key.State = state
			a.mcpWork(func() error { return a.mcpBackend.service.UpdateKey(a.ctx, key) }, nil)
			return
		}
	}
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
			a.mcp.policyDelete.Open = false
			a.mcp.selectedPolicyID = ""
		}
	})
}

func (a *app) controlMCPBackendTask(id, action string) {
	number, err := strconv.Atoi(id)
	if err != nil {
		a.persistenceNotice("无效的任务标识", err)
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
	return map[string]string{"connections_list": "连接发现", "ssh_exec": "命令执行", "files_list": "目录浏览", "files_stat": "文件信息", "files_read": "文件读取", "files_write": "文件写入", "files_upload": "目录上传", "files_download": "目录下载", "authentication": "认证失败", "tasks_list": "任务列表", "tasks_get": "任务查询", "tasks_cancel": "取消任务"}[value]
}

func mcpActionValue(name string) string {
	for _, value := range []string{"ssh_exec", "files_list", "files_stat", "files_read", "files_write", "files_upload", "files_download", "authentication"} {
		if mcpActionName(value) == name {
			return value
		}
	}
	return name
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
	view := mcpAuditItem{ID: op.ID, KeyID: op.KeyID, KeyName: op.KeyName, ConnectionID: op.ConnectionID,
		Time: op.StartedAt.Local().Format("15:04:05"), Day: op.StartedAt.Local().Format("01-02"),
		ConnName: target.Name, ConnHost: target.Host, ConnPort: strconv.Itoa(target.Port), ConnUser: target.User,
		ConnProtocol: strings.ToUpper(target.Protocol), ActionType: mcpActionName(op.Action), Status: mcpUIStatus(op.Status),
		StartTime: op.StartedAt.Local().Format("2006-01-02 15:04:05"), EndTime: "进行中", IsCommand: op.Action == "ssh_exec",
		CommandText: request.Command, RemotePath: request.Path, LocalPath: request.Local, OutputStatus: op.Error, Summary: request.Path}
	if view.ActionType == "" {
		view.ActionType = "管理操作"
	}
	if view.IsCommand {
		view.Summary = request.Command
	}
	if !op.FinishedAt.IsZero() {
		view.EndTime = op.FinishedAt.Local().Format("2006-01-02 15:04:05")
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
	view.ExitCodeText = "—（未知或未执行）"
	if result.ExitCode != nil {
		view.ExitCodeText = strconv.Itoa(*result.ExitCode)
	}
	if view.IsCommand && op.Error == "" {
		view.OutputStatus = fmt.Sprintf("stdout %s · stderr %s · 截断 %t", transferBytes(int64(result.Stdout)), transferBytes(int64(result.Stderr)), result.Truncated)
	}
	for _, event := range op.Timeline {
		view.Timeline = append(view.Timeline, uikit.TimelineItem{Stage: event.Stage, Detail: event.Detail, Time: event.At.Local().Format("15:04:05"), Tone: uikit.ToneSecondary})
	}
	if len(op.Hits) > 0 {
		h := op.Hits[0]
		category := "文件策略"
		if h.Category == "ssh" {
			category = "命令策略"
		}
		view.PolicyHit = &mcpPolicyHitSnapshot{PolicyID: h.PolicyID, PolicyName: h.Name, Category: category, Version: h.Version, RuleSummary: h.Rule, ForbiddenText: h.Action, HitCountText: fmt.Sprintf("命中 %d 条规则", len(op.Hits))}
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
		state := map[string]string{"completed": "ok", "intercepted": "intercepted", "failed": "failed", "skipped": "skipped", "running": "pending", "pending": "pending"}[f.Status]
		view.Files = append(view.Files, mcpAuditFileItem{Path: f.Path, State: state, Size: transferBytes(f.Bytes), PolicyTag: f.Action})
	}
	if task.ID > 0 {
		fraction := float32(0)
		if task.Total > 0 {
			fraction = float32(task.Done) / float32(task.Total)
		}
		view.Task = &mcpAuditTask{ID: strconv.Itoa(task.ID), Title: view.ActionType + " → " + target.Name,
			Subtitle: task.State, Transferred: transferBytes(task.Done), Total: transferBytes(task.Total), Fraction: fraction,
			Paused: task.State == "paused" || task.State == "interrupted" || task.State == "failed" || task.State == "intercepted" || task.State == "partial_intercepted", CancelRequest: task.State == "cancel_requested" || task.State == "cancelled" || task.State == "completed"}
		if task.State == "waiting_user" && len(op.Timeline) > 0 {
			view.Task.WaitingPrompt = "等待用户：" + op.Timeline[len(op.Timeline)-1].Detail
		}
		view.TransferStat = transferBytes(task.Done) + " / " + transferBytes(task.Total)
	}
	return view, nil
}
