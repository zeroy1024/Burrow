package desktop

import (
	"context"
	"time"

	"sshdesk/internal/mcpserver"
	"sshdesk/internal/store"
)

type mcpPromptObserverKey struct{}

func (a *app) mcpPromptChanged(ctx context.Context, waiting bool, detail string) {
	if observer, ok := ctx.Value(mcpPromptObserverKey{}).(func(bool, string)); ok {
		observer(waiting, detail)
	}
	if id, taskID := mcpserver.OperationID(ctx); id != "" {
		a.window.Update(func() { a.recordMCPWaiting(id, taskID, waiting, detail) })
	}
}

func (b *mcpDesktop) observeGroup(group *sshConnectionGroup) {
	if group == nil {
		return
	}
	group.plan.promptChanged = func(waiting bool, detail string) {
		b.a.window.Update(func() {
			for request := range group.waiters {
				if request.mcpOperationID != "" {
					b.a.recordMCPWaiting(request.mcpOperationID, request.mcpTaskID, waiting, detail)
				}
			}
		})
	}
}

func (a *app) recordMCPWaiting(id string, taskID int, waiting bool, detail string) {
	if a.mcpBackend == nil || a.mcpBackend.service == nil {
		return
	}
	a.persist(func() error {
		op, err := a.store.MCPOperation(a.ctx, id)
		if err != nil {
			return err
		}
		if !op.FinishedAt.IsZero() || op.Status != "running" && op.Status != "queued" && op.Status != "waiting_user" {
			return nil
		}
		op.Status = "running"
		if waiting {
			op.Status = "waiting_user"
		}
		op.Timeline = append(op.Timeline, store.MCPEvent{Stage: op.Status, Detail: detail, At: time.Now().UTC()})
		if taskID > 0 {
			task, err := a.store.MCPTask(a.ctx, taskID)
			if err != nil {
				return err
			}
			if task.State != "running" && task.State != "queued" && task.State != "waiting_user" {
				return nil
			}
			task.State, task.UpdatedAt = op.Status, time.Now().UTC()
			if err := a.store.MCPSaveTask(a.ctx, task); err != nil {
				return err
			}
		}
		return a.store.MCPFinish(a.ctx, op)
	}, func(err error) {
		if err != nil {
			a.persistenceNotice("记录 MCP 待处理状态失败", err)
			a.mcpBackend.service.FailStorage(err)
		}
		a.refreshMCPBackend()
	})
}
