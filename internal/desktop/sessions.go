package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo/ui"
)

func statusDot(c *ui.Context, state connectionState, size float32) {
	t := theme.Current()
	color := tokenColor(t.C.ConnectionIndicator.Offline)
	halo := tokenColor(t.C.ConnectionIndicator.Offline)
	if state == connected {
		color = tokenColor(t.C.ConnectionIndicator.Connected)
		halo = tokenColor(t.C.ConnectionIndicator.ConnectedHalo)
	}
	if state == connecting {
		color = tokenColor(t.C.ConnectionIndicator.Connecting)
		halo = tokenColor(t.C.ConnectionIndicator.ConnectingHalo)
	}
	ui.Box(c).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).Center().Children(func() {
		dot := ui.Box(c).Size(size, size).Radius(t.Radius.Pill).Background(color)
		if state != disconnected {
			dot.Border(t.C.ConnectionIndicator.Halo, halo).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).
				Background(ui.Transparent).Children(func() {
				ui.Box(c).Fill().Radius(t.Radius.Pill).Background(color)
			})
		}
	})
}

func (a *app) sidebar(c *ui.Context, shown float32) {
	t := theme.Current()
	ui.Column(c).Width(a.sidebarWidth).FillHeight().Left(a.sidebarWidth * (shown - 1)).
		Label("远程主机目录").Children(func() {
		ui.Row(c).Shrink(0).Padding(
			t.C.Sider.HeaderPaddingTop,
			t.C.Sider.HeaderPaddingRight,
			t.C.Sider.HeaderPaddingBottom,
			t.C.Sider.HeaderPaddingLeft,
		).Children(func() {
			title := "远程会话"
			if a.section == sectionFiles {
				title = "文件传输"
			}
			ui.Text(c, title).FontSize(t.C.Sider.TitleSize).FontWeight(t.C.Sider.TitleWeight)
			ui.Spacer(c)
			iconButton(c, "添加", plusIcon).Size(t.C.Sider.AddButtonSize, t.C.Sider.AddButtonSize).
				Radius(t.C.Sider.AddButtonRadius).Background(tokenColor(t.C.Sider.AddButtonBg)).Menu(func(m *ui.Menu) {
				if m.Item("新建分组").Chosen() {
					a.startGroupDraft()
				}
				label := "添加主机"
				if a.section == sectionFiles {
					label = "添加站点"
				}
				if m.Item(label).Chosen() {
					a.startHostDraft()
				}
			})
		})
		ui.Row(c).Height(t.C.Search.Height).Shrink(0).
			Margin(0, t.C.Search.MarginX, t.C.Search.MarginBottom, t.C.Search.MarginX).
			Radius(t.C.Search.Radius).Background(tokenColor(t.C.Search.Bg)).Children(func() {
			ui.TextInputBase(c.Key("host-search"), &a.query).Bind(&a.search).Fill().MinWidth(0).
				Padding(0, t.C.Search.PaddingX, 0, t.C.Search.Height).Radius(t.C.Search.Radius).
				FontSize(t.C.Search.FontSize).Placeholder("搜索主机、标签").Label("搜索主机、地址或系统")
			// The icon shares the input's bounds without intercepting text editing.
			ui.Row(c).Absolute().Top(0).Right(0).Bottom(0).Left(0).
				Padding(0, t.C.Search.PaddingX).PassThrough().Children(func() {
				ui.Icon(c, searchIcon).Size(t.Size.IconMd, t.Size.IconMd).TextColor(tokenColor(t.C.Search.Text)).PassThrough()
			})
		})
		a.catalogList(c)
		ui.Row(c).Shrink(0).Gap(t.Space.N8).Padding(t.C.Sider.FooterPaddingY, t.C.Sider.FooterPaddingX).
			BorderWidth(t.Size.Hairline, 0, 0, 0).BorderColor(tokenColor(t.C.Sider.FooterBorder)).
			TextColor(tokenColor(t.C.Sider.FooterText)).Children(func() {
			ui.Icon(c, serverIcon).Size(t.Size.IconSm, t.Size.IconSm)
			summary := fmt.Sprintf("%d 台主机", len(a.hosts))
			if a.section == sectionFiles {
				summary = fmt.Sprintf("%d 个站点", len(a.hosts))
			}
			ui.Text(c, summary).FontSize(t.C.Sider.FooterSize)
		})
	})
}

type hostDraft struct {
	name, address, user, port, group, system string
	protocol                                 string
	id                                       hostID
	editing, deleting, files                 bool
}

func (a *app) startHostDraft() {
	a.draft = hostDraft{
		user: "root", port: "22", group: a.groups[0].name, system: systems[0].name,
		protocol: "SFTP", files: a.section == sectionFiles,
	}
	a.addHostOpen = true
}

func (a *app) hostDialog(c *ui.Context) {
	t := theme.Current()
	ui.Modal(c, &a.addHostOpen, func() {
		ui.Column(c).Width(t.C.Dialog.HostWidth).Gap(t.Space.N14).Children(func() {
			if a.draft.deleting {
				a.deleteSiteDialogContents(c)
				return
			}
			title := "添加 SSH 主机"
			if a.draft.files {
				title = "添加文件站点"
				if a.draft.editing {
					title = "编辑文件站点"
				}
			}
			ui.Text(c, title).FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "保存到主机目录，不会连接服务器。").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
			if a.draft.files {
				ui.Text(c, "协议").FontSize(t.Font.Size.Small)
				ui.Select(c, &a.draft.protocol, []string{"SFTP", "FTP"}).OnChange(func() {
					if a.draft.protocol == "FTP" {
						a.draft.port = "21"
					} else {
						a.draft.port = "22"
					}
				})
			}
			draftField(c, "名称", &a.draft.name)
			draftField(c, "主机地址", &a.draft.address)
			ui.Row(c).Gap(t.Space.N12).Children(func() {
				ui.Column(c).Grow(1).Gap(t.Space.N6).Children(func() { draftField(c, "用户名", &a.draft.user) })
				ui.Column(c).Width(t.C.Dialog.PortWidth).Gap(t.Space.N6).Children(func() { draftField(c, "端口", &a.draft.port) })
			})
			ui.Text(c, "分组").FontSize(t.Font.Size.Small)
			groupNames := a.groupNames()
			ui.Select(c, &a.draft.group, groupNames)
			names := []string{}
			for _, system := range systems {
				names = append(names, system.name)
			}
			ui.Text(c, "系统").FontSize(t.Font.Size.Small)
			ui.Select(c, &a.draft.system, names)
			port, err := strconv.Atoi(a.draft.port)
			validPort := err == nil && port > 0 && port <= 65535
			validHost := strings.TrimSpace(a.draft.name) != "" && strings.TrimSpace(a.draft.address) != ""
			validUser := strings.TrimSpace(a.draft.user) != ""
			canAdd := validPort && validHost && validUser
			ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
				ui.Button(c, "取消").OnClick(func() { a.addHostOpen = false })
				label := "添加"
				if a.draft.editing {
					label = "保存"
				}
				ui.PrimaryButton(c, label).Disabled(!canAdd).
					OnClick(func() {
						err := a.commitCatalogMove(func() {
							protocol := ""
							if a.draft.files {
								protocol = strings.ToLower(a.draft.protocol)
							}
							h := host{
								name: strings.TrimSpace(a.draft.name), address: strings.TrimSpace(a.draft.address),
								user: strings.TrimSpace(a.draft.user), port: port,
								os: slices.Index(names, a.draft.system), system: a.draft.system, protocol: protocol,
							}
							id := hostID(len(a.hosts))
							group := a.groups[slices.Index(groupNames, a.draft.group)]
							if a.draft.editing {
								id = a.draft.id
								previous := a.hosts[id]
								previous.name, previous.address, previous.user = h.name, h.address, h.user
								previous.port, previous.os, previous.system = h.port, h.os, h.system
								previous.protocol = h.protocol
								a.hosts[id] = previous
								for _, group := range a.groups {
									if group.name != a.draft.group {
										group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
									}
								}
							} else {
								a.hosts = append(a.hosts, h)
							}
							if !slices.Contains(group.hosts, id) {
								group.hosts = append(group.hosts, id)
							}
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
	t := theme.Current()
	ui.Modal(c, &a.groupDialogOpen, func() {
		ui.Column(c).Width(t.C.Dialog.GroupWidth).Gap(t.Space.N14).Children(func() {
			ui.Text(c, "新建分组").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "将主机拖入分组，整理主机目录。").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
			ui.TextInput(c, &a.groupDraft).AutoFocus().Label("分组名称").Placeholder("分组名称").FillWidth()
			name := strings.TrimSpace(a.groupDraft)
			duplicate := slices.Contains(a.groupNames(), name)
			if duplicate {
				ui.Text(c, "此分组名称已存在").FontSize(t.Font.Size.Caption).TextColor(a.theme.Danger)
			}
			ui.Row(c).Gap(t.Space.N8).Justify(ui.End).Children(func() {
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
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Small)
		ui.TextInput(c, value).Label(label).FillWidth()
	})
}
