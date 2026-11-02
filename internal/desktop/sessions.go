package desktop

import (
	"fmt"
	"slices"
	"strings"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo/ui"
)

func (a *app) sidebar(c *ui.Context, shown float32) {
	directoryLabel, searchLabel, placeholder := "远程主机目录", "搜索主机、地址或系统", "搜索主机、标签"
	title, addLabel := "远程会话", "添加主机"
	if a.section == sectionFiles {
		directoryLabel, searchLabel, placeholder = "文件站点目录", "搜索站点、地址或系统", "搜索站点、标签"
		title, addLabel = "文件传输", "添加站点"
	}
	count := 0
	for _, h := range a.hosts {
		if a.catalogModeMatches(h) {
			count++
		}
	}
	summary := fmt.Sprintf("%d 台主机", count)
	if a.section == sectionFiles {
		summary = fmt.Sprintf("%d 个站点", count)
	}
	newBinding := a.shortcutBinding("session.new")
	newModifiers, newKey, _ := bindingToKey(newBinding)
	result := uikit.Sider(c, title, uikit.SiderProps{
		Label: directoryLabel, AddLabel: "添加", AddIcon: plusIcon, Footer: summary, FooterIcon: serverIcon,
		Query: &a.page().query,
		Search: uikit.SearchProps{Label: searchLabel, Placeholder: placeholder, Icon: searchIcon,
			Handle: &a.page().search, Kbd: shortcutDisplay(a.shortcutBinding("host.search"))},
		AddMenu: []uikit.MenuEntry{
			{ID: "new-group", Label: localize(c, "新建分组"), Icon: uikit.IconFolder, OnChoose: a.startGroupDraft},
			{ID: "new-host", Label: localize(c, addLabel), Icon: uikit.IconPlus,
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
	title, action, detail := "新建分组", "创建", "将主机拖入分组，整理主机目录。"
	dialogContext := c.Key("new-group-dialog")
	if target != nil {
		title, action, detail = "重命名分组", "保存", "输入新的分组名称。"
		dialogContext = c.Key("rename-group-dialog").Key(target)
	}
	save := false
	var name string
	valid := false
	result := uikit.Modal(dialogContext, &page.groupDialogOpen, uikit.ModalProps{
		Title: localize(c, title), Compact: true, Width: t.C.Dialog.GroupWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, localize(c, detail), uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		input := uikit.Input(c, &page.groupDraft, uikit.InputProps{DeferStyle: true}).
			AutoFocus().Label(localize(c, "分组名称")).Placeholder(localize(c, "分组名称")).FillWidth()
		name = strings.TrimSpace(page.groupDraft)
		duplicate := slices.ContainsFunc(a.catalogGroups(), func(group *hostGroup) bool {
			return group != target && group.name == name
		})
		valid = name != "" && !duplicate && (target == nil || slices.Contains(a.groups, target) && name != target.name)
		message := ""
		if duplicate {
			message = localize(c, "此分组名称已存在")
			uikit.FieldMessage(c, message, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
		}
		uikit.StyleFormControl(input, message)
		dirty := name != ""
		if target != nil {
			dirty = page.groupDraft != target.name
		}
		parts.SetDirty(dirty)
		ui.Row(c).Gap(t.Space.N8).Justify(ui.End).Children(func() {
			uikit.Button(c, localize(c, "取消"), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			save = uikit.Button(c, localize(c, action), uikit.ButtonProps{Variant: uikit.ButtonPrimary, Disabled: !valid}).Clicked()
		})
	})
	if (save || result.Submit) && valid {
		if err := a.commitCatalogMove(func() {
			if target != nil {
				target.name = name
			} else {
				a.groups = append(a.groups, a.newCatalogGroup(name))
			}
		}); err != nil {
			uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "保存分组失败", Detail: err.Error()})
			return
		}
		page.groupDialogOpen = false
		if target == nil {
			page.query = ""
		}
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
		Title: localize(c, "删除分组？"), Compact: true, Width: t.C.Dialog.GroupWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		detail := fmt.Sprintf(localize(c, "删除「%s」分组，连接配置、已打开的会话和传输任务会保留。"), group.name)
		if valid && len(group.hosts) > 0 {
			detail = fmt.Sprintf(localize(c, "删除「%s」分组后，组内 %d 个条目将移至「%s」。连接配置、已打开的会话和传输任务会保留。"), group.name, len(group.hosts), destination.name)
		}
		uikit.Text(c, detail, uikit.TextProps{})
		ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
			uikit.Button(c, localize(c, "取消"), uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			remove = uikit.Button(c, localize(c, "删除"), uikit.ButtonProps{Variant: uikit.ButtonDanger, Disabled: !valid}).Clicked()
		})
	})
	if (remove || result.Submit) && valid {
		if err := a.commitCatalogMove(func() {
			destination.hosts = append(destination.hosts, group.hosts...)
			a.groups = slices.DeleteFunc(a.groups, func(candidate *hostGroup) bool { return candidate == group })
		}); err != nil {
			uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "删除分组失败", Detail: err.Error()})
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
		page.groupDialogOpen = false
	}
	if !page.groupDialogOpen {
		page.groupDelete = nil
	}
}

func draftField(c *ui.Context, label string, value *string) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Small)
		uikit.Input(c, value, uikit.InputProps{}).Label(label).FillWidth()
	})
}
