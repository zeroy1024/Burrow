package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

func statusDot(c *ui.Context, state connectionState, size float32) {
	color := ui.Hex("#cfd0d5")
	if state == connected {
		color = ui.Hex("#28c15a")
	}
	if state == connecting {
		color = ui.Hex("#f5a21a")
	}
	ui.Box(c).Size(size+6, size+6).Center().Children(func() {
		dot := ui.Box(c).Size(size, size).Radius(99).Background(color)
		if state != disconnected {
			dot.Border(3, color.Alpha(0.16)).Size(size+6, size+6).
				Background(ui.Transparent).Children(func() {
				ui.Box(c).Fill().Radius(99).Background(color)
			})
		}
	})
}

func (a *app) sidebar(c *ui.Context, shown float32) {
	ui.Column(c).Width(a.sidebarWidth).FillHeight().Left(a.sidebarWidth * (shown - 1)).
		Label("远程主机目录").Children(func() {
		ui.Row(c).Height(52).Shrink(0).Padding(0, 16).Children(func() {
			ui.Text(c, "远程会话").FontSize(15).FontWeight(650)
			ui.Spacer(c)
			iconButton(c, "添加", plusIcon).Background(ui.Hex("#f3f3f5")).Menu(func(m *ui.Menu) {
				if m.Item("新建分组").Chosen() {
					a.startGroupDraft()
				}
				if m.Item("添加主机").Chosen() {
					a.startHostDraft()
				}
			})
		})
		ui.Row(c).Height(34).Shrink(0).Margin(0, 12).
			Radius(8).Background(ui.Hex("#f4f4f6")).Children(func() {
			ui.TextInputBase(c.Key("host-search"), &a.query).Bind(&a.search).Fill().MinWidth(0).
				Padding(0, 9, 0, 30).Radius(8).
				FontSize(12).Placeholder("搜索主机、标签").Label("搜索主机、地址或系统")
			// The icon shares the input's bounds without intercepting text editing.
			ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).
				Padding(0, 9).PassThrough().Children(func() {
				ui.Icon(c, searchIcon).Size(15, 15).TextColor(a.theme.TextMuted).PassThrough()
			})
		})
		a.catalogList(c)
		ui.Row(c).Height(34).Shrink(0).Gap(8).Padding(0, 14).TextColor(a.theme.TextMuted).Children(func() {
			ui.Icon(c, serverIcon).Size(14, 14)
			ui.Text(c, fmt.Sprintf("%d 台主机", len(a.hosts))).FontSize(11)
		})
	})
}

type hostDraft struct {
	name, address, user, port, group, system string
}

func (a *app) startHostDraft() {
	a.draft = hostDraft{user: "root", port: "22", group: a.groups[0].name, system: systems[0].name}
	a.addHostOpen = true
}

func (a *app) hostDialog(c *ui.Context) {
	ui.Modal(c, &a.addHostOpen, func() {
		ui.Column(c).Width(340).Gap(14).Children(func() {
			ui.Text(c, "添加 SSH 主机").FontSize(18).FontWeight(650)
			ui.Text(c, "添加到本次演示，不会连接服务器。").FontSize(12).TextColor(a.theme.TextMuted)
			draftField(c, "名称", &a.draft.name)
			draftField(c, "主机地址", &a.draft.address)
			ui.Row(c).Gap(12).Children(func() {
				ui.Column(c).Grow(1).Gap(6).Children(func() { draftField(c, "用户名", &a.draft.user) })
				ui.Column(c).Width(90).Gap(6).Children(func() { draftField(c, "端口", &a.draft.port) })
			})
			ui.Text(c, "分组").FontSize(12)
			groupNames := a.groupNames()
			ui.Select(c, &a.draft.group, groupNames)
			names := []string{}
			for _, system := range systems {
				names = append(names, system.name)
			}
			ui.Text(c, "系统").FontSize(12)
			ui.Select(c, &a.draft.system, names)
			port, err := strconv.Atoi(a.draft.port)
			validPort := err == nil && port > 0 && port <= 65535
			validHost := strings.TrimSpace(a.draft.name) != "" && strings.TrimSpace(a.draft.address) != ""
			validUser := strings.TrimSpace(a.draft.user) != ""
			canAdd := validPort && validHost && validUser
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.addHostOpen = false })
				ui.PrimaryButton(c, "添加").Disabled(!canAdd).
					OnClick(func() {
						err := a.commitCatalogMove(func() {
							id := hostID(len(a.hosts))
							a.hosts = append(a.hosts, host{
								name: strings.TrimSpace(a.draft.name), address: strings.TrimSpace(a.draft.address),
								user: strings.TrimSpace(a.draft.user), port: port,
								os: slices.Index(names, a.draft.system), system: a.draft.system,
							})
							group := a.groups[slices.Index(groupNames, a.draft.group)]
							group.hosts = append(group.hosts, id)
							group.collapsed = false
						})
						if err != nil {
							c.Toast("保存主机失败：" + err.Error())
							return
						}
						a.query = ""
						a.addHostOpen = false
					})
			})
		})
	})
}

func (a *app) startGroupDraft() {
	a.groupDraft = ""
	a.groupDialogOpen = true
}

func (a *app) groupDialog(c *ui.Context) {
	ui.Modal(c, &a.groupDialogOpen, func() {
		ui.Column(c).Width(320).Gap(14).Children(func() {
			ui.Text(c, "新建分组").FontSize(18).FontWeight(650)
			ui.Text(c, "将主机拖入分组，整理主机目录。").FontSize(12).TextColor(a.theme.TextMuted)
			ui.TextInput(c, &a.groupDraft).AutoFocus().Label("分组名称").Placeholder("分组名称").FillWidth()
			name := strings.TrimSpace(a.groupDraft)
			duplicate := slices.Contains(a.groupNames(), name)
			if duplicate {
				ui.Text(c, "此分组名称已存在").FontSize(11).TextColor(a.theme.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.groupDialogOpen = false })
				ui.PrimaryButton(c, "创建").Disabled(name == "" || duplicate).OnClick(func() {
					if err := a.commitCatalogMove(func() {
						a.groups = append(a.groups, &hostGroup{id: len(a.groups), name: name, hosts: []hostID{}})
					}); err != nil {
						c.Toast("保存分组失败：" + err.Error())
						return
					}
					a.groupDialogOpen, a.query = false, ""
				})
			})
		})
	})
}

func draftField(c *ui.Context, label string, value *string) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(12)
		ui.TextInput(c, value).Label(label).FillWidth()
	})
}
