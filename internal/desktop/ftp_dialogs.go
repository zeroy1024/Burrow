package desktop

import (
	"fmt"
	"github.com/egoist/mygo/i18n"
	"path"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/uikit"
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
	kind, source, icon := locales.MessageFile(), locales.MessageLocal(), uikit.IconDocument
	if entry.dir {
		kind, icon = locales.MessageFolder(), uikit.IconFolder
	}
	if dialog.pane.remote {
		source = locales.MessageRemote()
	}
	uikit.Modal(c.Key("file-preview-dialog"), &open, uikit.ModalProps{
		Title: locales.MessageFilePreview(), Subtitle: locales.FilesPreviewSubtitle(
			source,
			kind,
		), Icon: uikit.IconSVG(icon), Width: t.C.Dialog.HostWidth,
		CloseLabel: locales.MessageCloseFilePreview(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
		Footer: func(parts uikit.ModalParts) {
			uikit.ModalFooter(c).Children(func() {
				uikit.Button(
					c,
					locales.MessageCopyPath(),
					uikit.ButtonProps{Icon: uikit.IconCopy},
				).OnClick(func() { mygo.Clipboard.WriteText(entry.path) })
				ui.Spacer(c)
				uikit.Button(
					c,
					locales.CommonClose(),
					uikit.ButtonProps{Variant: uikit.ButtonPrimary},
				).OnClick(parts.RequestDismiss)
			})
		},
	}, func(parts uikit.ModalParts) {
		scroll := ui.Local(parts.Panel, "uikit.file-preview.scroll", func() ui.ScrollState { return ui.ScrollState{} })
		scroller := ui.Scroll(c).FillWidth().MinHeight(0).Shrink(1).TrackScroll(scroll).Children(func() {
			uikit.ModalBody(c).Children(func() {
				uikit.KeyValue(c, []uikit.KV{
					{Key: locales.CommonName(), Value: entry.name}, {Key: locales.MessageType(), Value: kind}, {Key: locales.MessageSize(), Value: transferBytes(entry.size)},
					{Key: locales.MessageModified(), Value: localizedDate(entry.modified, i18n.MediumStyle, i18n.MediumStyle)},
					{Key: locales.MessagePermissions(), Value: entry.permissions, Mono: true}, {Key: locales.MessageOwner(), Value: entry.owner},
				})
				field := uikit.FormRow(c, locales.MessagePath(), uikit.FormRowProps{Inline: true})
				field.Body.Children(func() {
					uikit.Input(
						c,
						&dialog.preview.path,
						uikit.InputProps{Label: locales.MessageFilePath(), Mono: true},
					).ReadOnly(true).FillWidth().AutoFocus()
				})
			})
		})
		uikit.Scrollbars(c, parts.Panel, scroller, scroll, uikit.ScrollbarProps{Label: locales.MessageFilePreviewScrollbar()})
	})
	if !open {
		a.files.dialog = nil
	}
}

func (a *app) fileActionModal(c *ui.Context) {
	t := theme.Current()
	dialog := a.files.dialog
	title, action, variant := locales.MessageNewFolder(), locales.MessageCreate(), uikit.ButtonPrimary
	switch dialog.kind {
	case "rename":
		title, action = locales.MessageRename(), locales.CommonSave()
	case "delete":
		title, action, variant = locales.MessageDeleteFile(), locales.CommonDelete(), uikit.ButtonDanger
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
		uikit.Text(
			c,
			locales.MessageThisOperationAffectsActualFilesInTheCurrentDirectory(),
			uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
		)
		if dialog.kind == "delete" {
			count := ui.Local(parts.Panel, "uikit.file-action.selection-count", func() int { return len(dialog.pane.selection()) })
			message := locales.FilesDeleteSelectedConfirm(*count)
			if dialog.original != "" {
				message = fmt.Sprintf(locales.MessageDelete2(), dialog.original)
			}
			uikit.Text(c, message, uikit.TextProps{})
		} else {
			initial := ui.Local(parts.Panel, "uikit.file-action.initial-name", func() string { return dialog.name })
			input := uikit.Input(
				c,
				&dialog.name,
				uikit.InputProps{Label: locales.CommonName(), DeferStyle: true},
			).AutoFocus().FillWidth()
			if input.Changed() {
				dialog.errorText = locales.Text{}
			}
			nameError = fileActionNameError(dialog)
			uikit.StyleFormControl(input, nameError)
			parts.SetDirty(dialog.name != *initial)
			submitted = input.Submitted()
			if nameError != "" {
				uikit.FieldMessage(c, nameError, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
			}
		}
		if !dialog.errorText.Empty() {
			uikit.Banner(c, dialog.errorText.String(), uikit.BannerProps{Tone: uikit.ToneDanger, Icon: sessionAlertIcon})
		}
		ui.Row(c).Key("actions").Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			clicked = uikit.Button(c, action, uikit.ButtonProps{
				Variant: variant, Disabled: nameError != "", Loading: dialog.busy,
				Motion: uikit.MotionOverride(a.reduceMotion(c)),
			}).Clicked()
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
		return locales.MessageEnterAValidNameWithoutAndDoNotUseOr()
	}
	for _, entry := range dialog.pane.directories[dialog.pane.path] {
		if dialog.kind == "rename" && entry.name == dialog.original {
			continue
		}
		if entry.name == name {
			return locales.MessageAFileOrFolderWithThisNameAlreadyExistsInThisDirectory()
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
		Compact: true, Width: t.C.Dialog.HostWidth, Title: locales.MessageAnItemWithThisNameAlreadyExists(), Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, entry.name, uikit.TextProps{Style: uikit.TextHeadline})
		uikit.Text(
			c,
			locales.MessageDestination()+path.Join(conflict.target, entry.name),
			uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary},
		)
		uikit.Checkbox(
			c, &conflict.applyAll, locales.MessageApplyThisChoiceToAllConflicts(),
			uikit.CheckProps{Motion: uikit.MotionOverride(a.reduceMotion(c))},
		)
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			if uikit.Button(c, locales.MessageSkip(), uikit.ButtonProps{}).Clicked() {
				choice = 2
			}
			if uikit.Button(c, locales.MessageKeepBoth(), uikit.ButtonProps{}).Clicked() {
				choice = 3
			}
			if uikit.Button(c, locales.MessageOverwrite(), uikit.ButtonProps{Variant: uikit.ButtonPrimary}).Clicked() {
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
