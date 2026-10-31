package desktop

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func (a *app) fileDialogs(c *ui.Context) {
	if a.files.dialog != nil {
		a.fileActionModal(c)
	}
	if a.files.conflict != nil {
		a.fileConflictModal(c)
	}
}

func (a *app) fileActionModal(c *ui.Context) {
	t := theme.Current()
	dialog := a.files.dialog
	open := true
	modal(c.Key("file-action-dialog"), &open, func() {
		ui.Column(c).Width(t.C.Dialog.HostWidth).Gap(t.Space.N14).Children(func() {
			title, action := "新建文件夹", "创建"
			switch dialog.kind {
			case "rename":
				title, action = "重命名", "保存"
			case "delete":
				title, action = "删除文件", "删除"
			}
			ui.Text(c, title).FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "操作仅修改本次会话的演示目录。").
				FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
			var submitted bool
			if dialog.kind == "delete" {
				message := fmt.Sprintf("确认删除选中的 %d 项？", len(dialog.pane.selection()))
				if dialog.original != "" {
					message = fmt.Sprintf("确认删除「%s」？", dialog.original)
				}
				ui.Text(c, message).FontSize(t.Font.Size.Body)
			} else {
				input := textInput(c, &dialog.name).AutoFocus().Label("名称").FillWidth()
				submitted = input.Submitted()
			}
			nameError := fileActionNameError(dialog)
			message := nameError
			if dialog.errorText != "" {
				message = dialog.errorText
			}
			if message != "" {
				ui.Text(c, message).FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Danger.Text))
			}
			apply := func() {
				if a.applyFileAction(c, dialog) {
					a.files.dialog = nil
				}
			}
			ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
				button(c, "取消").OnClick(func() { a.files.dialog = nil })
				primaryButton(c, action).Disabled(nameError != "").OnClick(apply)
			})
			if submitted && nameError == "" {
				apply()
			}
		})
	})
	if !open {
		a.files.dialog = nil
	}
}

func fileActionNameError(dialog *fileActionDialog) string {
	if dialog.kind == "delete" {
		return ""
	}
	name := strings.TrimSpace(dialog.name)
	invalid := name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00")
	if invalid {
		return "请输入有效名称，不能包含 /，也不能使用 . 或 ..。"
	}
	for _, entry := range dialog.pane.directories[dialog.pane.path] {
		if dialog.kind == "rename" && entry.name == dialog.original {
			continue
		}
		if entry.name == name {
			return "此目录中已存在同名文件或文件夹。"
		}
	}
	return ""
}

func (a *app) applyFileAction(c *ui.Context, dialog *fileActionDialog) bool {
	pane := dialog.pane
	if !fileWritable(pane, pane.path) {
		dialog.errorText = "此目录不可写。"
		return false
	}
	if message := fileActionNameError(dialog); message != "" {
		dialog.errorText = message
		return false
	}
	name := strings.TrimSpace(dialog.name)
	entries := pane.directories[pane.path]
	switch dialog.kind {
	case "mkdir":
		entry := fileEntry{
			name: name, path: path.Join(pane.path, name), dir: true,
			modified: c.Now(), permissions: "rwxr-xr-x", owner: a.hosts[dialog.session.host].user,
		}
		pane.directories[pane.path] = append(entries, entry)
		pane.directories[entry.path] = []fileEntry{}
		pane.selected, pane.anchor, pane.reveal = map[string]bool{name: true}, name, name
	case "rename":
		index := slices.IndexFunc(entries, func(entry fileEntry) bool { return entry.name == dialog.original })
		if index < 0 {
			dialog.errorText = "要重命名的项目已不存在。"
			return false
		}
		oldPath, newPath := entries[index].path, path.Join(pane.path, name)
		entries[index].name, entries[index].path = name, newPath
		if oldPath != newPath && entries[index].dir {
			renameDemoDirectory(pane, oldPath, newPath)
		}
		pane.directories[pane.path] = entries
		delete(pane.selected, dialog.original)
		pane.selected[name] = true
		pane.anchor, pane.reveal = name, name
	case "delete":
		selected := pane.selection()
		if dialog.original != "" {
			selected = []fileEntry{}
			for _, entry := range entries {
				if entry.name == dialog.original {
					selected = append(selected, entry)
					break
				}
			}
		}
		for _, entry := range selected {
			if entry.dir {
				for directory := range pane.directories {
					if directory == entry.path || strings.HasPrefix(directory, entry.path+"/") {
						delete(pane.directories, directory)
					}
				}
			}
			entries = slices.DeleteFunc(entries, func(candidate fileEntry) bool { return candidate.name == entry.name })
			delete(pane.selected, entry.name)
		}
		pane.directories[pane.path] = entries
		pane.anchor, pane.reveal = "", ""
	}
	return true
}

func renameDemoDirectory(pane *filePaneState, oldPath, newPath string) {
	moved := map[string][]fileEntry{}
	for directory, entries := range pane.directories {
		if directory != oldPath && !strings.HasPrefix(directory, oldPath+"/") {
			continue
		}
		for index := range entries {
			entries[index].path = newPath + strings.TrimPrefix(entries[index].path, oldPath)
		}
		moved[newPath+strings.TrimPrefix(directory, oldPath)] = entries
		delete(pane.directories, directory)
	}
	for directory, entries := range moved {
		pane.directories[directory] = entries
	}
	for index, directory := range pane.history {
		if directory == oldPath || strings.HasPrefix(directory, oldPath+"/") {
			pane.history[index] = newPath + strings.TrimPrefix(directory, oldPath)
		}
	}
}

func (a *app) fileConflictModal(c *ui.Context) {
	t := theme.Current()
	conflict := a.files.conflict
	entry := conflict.entries[conflict.index]
	open := true
	modal(c.Key("file-conflict-dialog"), &open, func() {
		ui.Column(c).Width(t.C.Dialog.HostWidth).Gap(t.Space.N14).Children(func() {
			ui.Text(c, "目标中已存在同名项目").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, entry.name).FontSize(t.Font.Size.BodyLg).FontWeight(t.Font.Weight.Medium)
			ui.Text(c, "目标："+path.Join(conflict.target, entry.name)).FontSize(t.Font.Size.Small).
				TextColor(tokenColor(t.Color.Text.Secondary))
			ui.Checkbox(c, &conflict.applyAll, "对所有重名项应用此选择")
			ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
				button(c, "取消").OnClick(func() {
					a.files.conflict = nil
					open = false
				})
				choose := func(choice int) {
					conflict.choice = choice
					a.resolveFileConflicts(c)
				}
				button(c, "跳过").OnClick(func() { choose(2) })
				button(c, "保留两者").OnClick(func() { choose(3) })
				primaryButton(c, "覆盖").OnClick(func() { choose(1) })
			})
		})
	})
	if !open {
		a.files.conflict = nil
	}
}
