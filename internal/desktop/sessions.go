package desktop

import (
	"fmt"
	"slices"
	"strings"

	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/theme"
	"github.com/zeroy1024/Burrow/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) sidebar(c *ui.Context, shown float32) {
	directoryLabel, searchLabel, placeholder := locales.MessageRemoteHostCatalog(), locales.MessageSearchHostsAddressesOrOperatingSystems(), locales.MessageSearchHostsOrTags()
	title, addLabel := locales.MessageRemoteSessions(), locales.MessageAddHost()
	if a.section == sectionFiles {
		directoryLabel, searchLabel, placeholder = locales.MessageFileSiteCatalog(), locales.MessageSearchSitesAddressesOrOperatingSystems(), locales.MessageSearchSitesOrTags()
		title, addLabel = locales.FilesTitle(), locales.MessageAddSite()
	}
	count := 0
	for _, h := range a.hosts {
		if a.catalogModeMatches(h) {
			count++
		}
	}
	summary := locales.HostsCount(count)
	if a.section == sectionFiles {
		summary = locales.SitesCount(count)
	}
	newBinding := a.shortcutBinding("session.new")
	newModifiers, newKey, _ := bindingToKey(newBinding)
	result := uikit.Sider(c, title, uikit.SiderProps{
		Label: directoryLabel, AddLabel: locales.CommonAdd(), AddIcon: plusIcon, Footer: summary, FooterIcon: serverIcon,
		Query: &a.page().query,
		Search: uikit.SearchProps{Label: searchLabel, Placeholder: placeholder, Icon: searchIcon,
			Handle: &a.page().search, Kbd: shortcutDisplay(a.shortcutBinding("host.search"))},
		AddMenu: []uikit.MenuEntry{
			{ID: "new-group", Label: locales.MessageNewGroup(), Icon: uikit.IconFolder, OnChoose: a.startGroupDraft},
			{ID: "new-host", Label: addLabel, Icon: uikit.IconPlus,
				Shortcut: shortcutDisplay(newBinding), Modifiers: newModifiers, ShortcutKey: newKey, OnChoose: a.startHostDraft},
		},
	}, func() { a.catalogList(c) })
	result.Root.Width(a.page().sidebarWidth).Left(a.page().sidebarWidth * (shown - 1))
}

func (a *app) startGroupDraft() {
	a.page().groupDraft = ""
	a.page().groupDelete = nil
	a.page().groupRename = nil
	a.page().groupDialogOpen = true
}

func (a *app) startGroupRename(group *hostGroup) {
	page := a.page()
	page.groupDraft, page.groupRename, page.groupDelete = group.name, group, nil
	page.groupDialogOpen = true
}

func (a *app) groupDialog(c *ui.Context) {
	t := theme.Current()
	page := a.page()
	if page.groupDelete != nil {
		a.deleteGroupDialog(c)
		return
	}
	target := page.groupRename
	title, action, detail := locales.MessageNewGroup(), locales.MessageCreate(), locales.MessageDragHostsIntoGroupsToOrganizeTheCatalog()
	dialogContext := c.Key("new-group-dialog")
	if target != nil {
		title, action, detail = locales.MessageRenameGroup2(), locales.CommonSave(), locales.MessageEnterANewGroupName()
		dialogContext = c.Key("rename-group-dialog").Key(target)
	}
	save := false
	var name string
	valid := false
	result := uikit.Modal(dialogContext, &page.groupDialogOpen, uikit.ModalProps{
		Controller: &page.groupModal,
		Title:      title, Compact: true, Width: t.C.Dialog.GroupWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, detail, uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		input := uikit.Input(c, &page.groupDraft, uikit.InputProps{DeferStyle: true}).
			AutoFocus().Label(locales.MessageGroupName()).Placeholder(locales.MessageGroupName()).FillWidth()
		name = strings.TrimSpace(page.groupDraft)
		duplicate := slices.ContainsFunc(a.catalogGroups(), func(group *hostGroup) bool {
			return group != target && group.name == name
		})
		valid = name != "" && !duplicate && (target == nil || slices.Contains(a.groups, target) && name != target.name)
		message := ""
		if duplicate {
			message = locales.MessageThisGroupNameAlreadyExists()
			uikit.FieldMessage(c, message, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
		}
		uikit.StyleFormControl(input, message)
		dirty := name != ""
		if target != nil {
			dirty = page.groupDraft != target.name
		}
		parts.SetDirty(dirty)
		ui.Row(c).Gap(t.Space.N8).Justify(ui.End).Children(func() {
			uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			save = uikit.Button(c, action, uikit.ButtonProps{Variant: uikit.ButtonPrimary, Disabled: !valid}).Clicked()
		})
	})
	if (save || result.Submit) && valid {
		a.commitCatalogMoveAsync(func() {
			if target != nil {
				target.name = name
			} else {
				a.groups = append(a.groups, a.newCatalogGroup(name))
			}
		}, func(err error) {
			if err != nil {
				a.persistenceNotice(locales.MessageUnableToSaveGroup, err)
				return
			}
			page.groupModal.Close()
			if target == nil {
				page.query = ""
			}
		})
	}
	if !page.groupDialogOpen {
		page.groupRename = nil
	}
}

func (a *app) deleteGroupDialog(c *ui.Context) {
	page := a.page()
	group := page.groupDelete
	destination := a.groupDeleteDestination(group)
	valid := destination != nil && slices.Contains(a.groups, group)
	remove := false
	t := theme.Current()
	result := uikit.Modal(c.Key(group), &page.groupDialogOpen, uikit.ModalProps{
		Controller: &page.groupModal,
		Title:      locales.MessageDeleteGroup2(), Compact: true, Width: t.C.Dialog.GroupWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		detail := fmt.Sprintf(
			locales.MessageDeleteGroupConnectionProfilesOpenSessionsAndTransfersWillBeRetained(),
			group.name,
		)
		if valid && len(group.hosts) > 0 {
			detail = locales.GroupDeleteAndMove(group.name, len(group.hosts), destination.name)
		}
		uikit.Text(c, detail, uikit.TextProps{})
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, locales.CommonCancel(), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			remove = uikit.Button(
				c,
				locales.CommonDelete(),
				uikit.ButtonProps{Variant: uikit.ButtonDanger, Disabled: !valid},
			).Clicked()
		})
	})
	if (remove || result.Submit) && valid {
		a.commitCatalogMoveAsync(func() {
			destination.hosts = append(destination.hosts, group.hosts...)
			a.groups = slices.DeleteFunc(a.groups, func(candidate *hostGroup) bool { return candidate == group })
		}, func(err error) {
			if err != nil {
				a.persistenceNotice(locales.MessageUnableToDeleteGroup, err)
				return
			}
			if page.lastGroup == group {
				page.lastGroup = destination
			}
			key := catalogKey{kind: catalogGroupDrag, id: group.id}
			delete(page.catalogItems, key)
			if page.catalogFlash != nil && *page.catalogFlash == key {
				page.catalogFlash = nil
			}
			page.groupModal.Close()
		})
	}
	if !page.groupDialogOpen {
		page.groupDelete = nil
	}
}

func draftField(c *ui.Context, label string, value *string) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		uikit.Text(c, label, uikit.TextProps{Style: uikit.TextSmall})
		uikit.Input(c, value, uikit.InputProps{}).Label(label).FillWidth()
	})
}
