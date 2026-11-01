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
	a.page().groupDialogOpen = true
}

func (a *app) groupDialog(c *ui.Context) {
	t := theme.Current()
	page := a.page()
	create := false
	var name string
	valid := false
	result := uikit.Modal(c.Key("new-group-dialog"), &page.groupDialogOpen, uikit.ModalProps{
		Title: "新建分组", Compact: true, Width: t.C.Dialog.GroupWidth,
		Motion: uikit.MotionOverride(a.reduceMotion(c)),
	}, func(parts uikit.ModalParts) {
		uikit.Text(c, "将主机拖入分组，整理主机目录。", uikit.TextProps{Style: uikit.TextSmall, Tone: uikit.ToneSecondary})
		input := uikit.Input(c, &page.groupDraft, uikit.InputProps{DeferStyle: true}).
			AutoFocus().Label("分组名称").Placeholder("分组名称").FillWidth()
		name = strings.TrimSpace(page.groupDraft)
		duplicate := slices.Contains(a.groupNames(), name)
		valid = name != "" && !duplicate
		message := ""
		if duplicate {
			message = "此分组名称已存在"
			uikit.FieldMessage(c, message, uikit.FieldMessageProps{Tone: uikit.ToneDanger})
		}
		uikit.StyleFormControl(input, message)
		parts.SetDirty(name != "")
		ui.Row(c).Gap(t.Space.N8).Justify(ui.End).Children(func() {
			uikit.Button(c, "取消", uikit.ButtonProps{}).OnClick(parts.RequestDismiss)
			create = uikit.Button(c, "创建", uikit.ButtonProps{Variant: uikit.ButtonPrimary, Disabled: !valid}).Clicked()
		})
	})
	if (create || result.Submit) && valid {
		if err := a.commitCatalogMove(func() {
			a.groups = append(a.groups, a.newCatalogGroup(name))
		}); err != nil {
			uikit.Toast(c, uikit.ToastProps{Tone: uikit.ToneDanger, Title: "保存分组失败", Detail: err.Error()})
			return
		}
		page.groupDialogOpen, page.query = false, ""
	}
}

func draftField(c *ui.Context, label string, value *string) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Small)
		uikit.Input(c, value, uikit.InputProps{}).Label(label).FillWidth()
	})
}
