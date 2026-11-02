package desktop

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/fileclient"
	"sshdesk/internal/uikit"
)

func (a *app) applyFileAction(c *ui.Context, dialog *fileActionDialog) bool {
	if dialog.busy {
		return false
	}
	if message := fileActionNameError(dialog); message != "" {
		dialog.errorText = message
		return false
	}
	pane := dialog.pane
	run := dialog.session.localRuntime
	if pane.remote {
		run = dialog.session.runtime
	}
	if run == nil || run.client == nil || run.ctx.Err() != nil {
		dialog.errorText = "文件会话未连接"
		return false
	}
	name, directory, kind := dialog.name, pane.path, dialog.kind
	selected := slices.Clone(pane.selection())
	if dialog.original != "" {
		selected = nil
		for _, entry := range pane.directories[directory] {
			if entry.name == dialog.original {
				selected = []fileEntry{entry}
				break
			}
		}
		if len(selected) == 0 {
			dialog.errorText = "操作的项目已不存在，请刷新目录。"
			return false
		}
	}
	dialog.busy, dialog.errorText = true, ""
	client, ctx := run.client, run.ctx
	a.sshWorkers.Go(func() {
		var err error
		var reveal string
		switch kind {
		case "mkdir":
			reveal = path.Join(directory, name)
			err = client.Mkdir(ctx, reveal)
		case "rename":
			reveal = path.Join(directory, name)
			if selected[0].path != reveal {
				err = client.Rename(ctx, selected[0].path, reveal, fileclient.RenameOptions{NoReplace: true})
			}
		case "delete":
			if len(selected) == 0 {
				err = errors.New("未选择要删除的项目")
			}
			for index, entry := range selected {
				if err != nil {
					break
				}
				if err = removeFileTree(ctx, client, entry.path, 0); err != nil {
					err = fmt.Errorf("已删除 %d/%d 项；%s：%w", index, len(selected), entry.name, err)
				}
			}
		default:
			err = fmt.Errorf("不支持的文件操作 %q", kind)
		}
		a.window.Update(func() {
			dialog.busy = false
			if err != nil {
				dialog.errorText = err.Error()
				if a.files.dialog != dialog {
					a.files.notice = &uikit.ToastProps{ID: "file-action-error", Tone: uikit.ToneDanger, Title: "文件操作失败", Detail: err.Error()}
				}
			} else {
				if a.files.dialog == dialog {
					a.files.dialog = nil
				}
				pane.table.Clear()
				pane.reveal = reveal
			}
			if run.ctx.Err() == nil {
				a.refreshFilePane(dialog.session, pane)
			}
		})
	})
	return false
}

// Lstat is repeated at every level: links are unlinked, never traversed.
// A malicious server cannot turn a recursive deletion into a parent traversal.
func removeFileTree(ctx context.Context, client fileclient.Client, location string, depth int) error {
	if remover, ok := client.(fileclient.TreeRemover); ok {
		return remover.RemoveTree(ctx, location)
	}
	if depth > 256 {
		return errors.New("目录层级过深，已停止删除")
	}
	entry, err := client.Stat(ctx, location)
	if err != nil {
		return err
	}
	if entry.Directory && !entry.Symlink {
		entries, err := client.ReadDir(ctx, location)
		if err != nil {
			return err
		}
		for _, child := range entries {
			if child.Name == "" || child.Name == "." || child.Name == ".." || strings.ContainsAny(child.Name, "/\x00") {
				return errors.New("服务器返回了非法目录项")
			}
			if err := removeFileTree(ctx, client, path.Join(location, child.Name), depth+1); err != nil {
				return err
			}
		}
	}
	return client.Remove(ctx, location, entry.Directory && !entry.Symlink)
}
