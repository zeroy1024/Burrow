package desktop

import (
	"fmt"
	"path"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

func (a *app) fileDialogs(c *ui.Context) {
	if a.files.dialog != nil {
		if a.files.dialog.kind == "preview" {
			a.filePreviewModal(c, a.files.dialog)
		} else {
			a.fileActionModal(c)
		}
	}
	if a.files.conflict != nil {
		a.fileConflictModal(c)
	}
}

func (a *app) filePreviewModal(c *ui.Context, dialog *fileActionDialog) {
	t := theme.Current()
	entry := dialog.preview
	open := true
	kind, source, icon := "文件", "本地", uikit.IconDocument
	if entry.dir {
		kind, icon = "文件夹", uikit.IconFolder
	}
	if dialog.pane.remote {
		source = "远程"
	}
	uikit.Modal(c.Key("file-preview-dialog"), &open, uikit.ModalProps{
		Title: "文件预览", Subtitle: source + kind, Icon: uikit.IconSVG(icon), Width: t.C.Dialog.HostWidth,
		CloseLabel: "关闭文件预览", Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Children(func() {
				uikit.Button(c, "复制路径", uikit.ButtonProps{Icon: uikit.IconCopy}).OnClick(func() { mygo.Clipboard.WriteText(entry.path) })
				ui.Spacer(c)
				uikit.Button(c, "关闭", uikit.ButtonProps{Variant: uikit.ButtonPrimary}).OnClick(parts.RequestDismiss)
			})
		},
	}, func(parts uikit.ModalParts) {
		scroll := ui.Local(parts.Panel, "uikit.file-preview.scroll", func() ui.ScrollState { return ui.ScrollState{} })
		scroller := ui.Scroll(c).FillWidth().MinHeight(0).Shrink(1).TrackScroll(scroll).Children(func() {
			uikit.ModalBody(c).Children(func() {
				uikit.KeyValue(c, []uikit.KV{
					{Key: "名称", Value: entry.name}, {Key: "类型", Value: kind}, {Key: "大小", Value: transferBytes(entry.size)},
					{Key: "修改时间", Value: entry.modified.Format("2006-01-02 15:04:05")},
					{Key: "权限", Value: entry.permissions, Mono: true}, {Key: "所有者", Value: entry.owner},
				})
				field := uikit.FormRow(c, "路径", uikit.FormRowProps{Inline: true})
				field.Body.Children(func() {
					uikit.Input(c, &dialog.preview.path, uikit.InputProps{Label: "文件路径", Mono: true}).ReadOnly(true).FillWidth().AutoFocus()
				})
			})
		})
		uikit.Scrollbars(c, parts.Panel, scroller, scroll, uikit.ScrollbarProps{Label: "文件预览滚动条"})
	})
	if !open {
		a.files.dialog = nil
	}
}

func (a *app) fileActionModal(c *ui.Context) {
	t := theme.Current()
	dialog := a.files.dialog
	title, action, variant := "新建文件夹", "创建", uikit.ButtonPrimary
	switch dialog.kind {
	case "rename":
		title, action = "重命名", "保存"
	case "delete":
		title, action, variant = "删除文件", "删除", uikit.ButtonDanger
	}
	open := true
	var nameError string
	var submitted, clicked bool
	var current uikit.ModalParts
	result := uikit.Modal(c.Key(dialog), &open, uikit.ModalProps{
		Controller: &dialog.modal,
		Compact:    true, Width: t.C.Dialog.HostWidth, Title: title, Disabled: dialog.busy, Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		current = parts
		uikit.Text(c, "操作将应用到当前目录中的实际文件。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		if dialog.kind == "delete" {
			count := ui.Local(parts.Panel, "uikit.file-action.selection-count", func() int { return len(dialog.pane.selection()) })
			message := fmt.Sprintf("确认删除选中的 %d 项？", *count)
			if dialog.original != "" {
				message = fmt.Sprintf("确认删除「%s」？", dialog.original)
			}
			uikit.Text(c, message, uikit.TextProps{})
		} else {
			initial := ui.Local(parts.Panel, "uikit.file-action.initial-name", func() string { return dialog.name })
			input := uikit.Input(c, &dialog.name, uikit.InputProps{Label: "名称", DeferStyle: true}).AutoFocus().FillWidth()
			if input.Changed() {
				dialog.errorText = ""
			}
			nameError = fileActionNameError(dialog)
			uikit.StyleFormControl(input, nameError)
			parts.SetDirty(dialog.name != *initial)
			submitted = input.Submitted()
			if nameError != "" {
				uikit.FieldMessage(c, nameError, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
			}
		}
		if dialog.errorText != "" {
			uikit.Banner(c, dialog.errorText, uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
		}
		ui.Row(c).Key("actions").Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			clicked = uikit.Button(c, action, uikit.ButtonProps{Variant: variant, Disabled: nameError != "", Loading: dialog.busy}).Clicked()
		})
	})
	if (clicked || submitted || result.Submit) && nameError == "" {
		if a.applyFileAction(c, dialog) {
			current.SetDirty(false)
			dialog.modal.Close()
			return
		}
	}
	if !open {
		a.files.dialog = nil
	}
}

func fileActionNameError(dialog *fileActionDialog) string {
	if dialog.kind == "delete" {
		return ""
	}
	name := dialog.name
	invalid := strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00")
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

func (a *app) fileConflictModal(c *ui.Context) {
	t := theme.Current()
	conflict := a.files.conflict
	entry := conflict.entries[conflict.index]
	open := true
	var choice int
	result := uikit.Modal(c.Key(conflict), &open, uikit.ModalProps{
		Compact: true, Width: t.C.Dialog.HostWidth, Title: "目标中已存在同名项目", Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, entry.name, uikit.TextProps{Style: uikit.TextHeadline})
		uikit.Text(c, "目标："+path.Join(conflict.target, entry.name), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		uikit.Checkbox(c, &conflict.applyAll, "对所有重名项应用此选择")
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			if uikit.Button(c, "跳过", uikit.ButtonProps{}).Clicked() {
				choice = 2
			}
			if uikit.Button(c, "保留两者", uikit.ButtonProps{}).Clicked() {
				choice = 3
			}
			if uikit.Button(c, "覆盖", uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Clicked() {
				choice = 1
			}
		})
	})
	if choice == 0 && result.Submit {
		choice = 1
	}
	if choice != 0 {
		conflict.choice = choice
		result.Close()
	}
	if !open {
		if conflict.choice != 0 {
			a.resolveFileConflicts(c)
		} else {
			a.cancelFileConflict()
		}
	}
}
