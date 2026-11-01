package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
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
	ui.Box(c).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).Center().PassThrough().Children(func() {
		dot := ui.Box(c).Size(size, size).Radius(t.Radius.Pill).Background(color).PassThrough()
		if state != disconnected {
			dot.Border(t.C.ConnectionIndicator.Halo, halo).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).
				Background(ui.Transparent).Children(func() {
				ui.Box(c).Fill().Radius(t.Radius.Pill).Background(color).PassThrough()
			})
		}
	})
}

func (a *app) sidebar(c *ui.Context, shown float32) {
	t := theme.Current()
	directoryLabel, searchLabel, placeholder := "远程主机目录", "搜索主机、地址或系统", "搜索主机、标签"
	if a.section == sectionFiles {
		directoryLabel, searchLabel, placeholder = "文件站点目录", "搜索站点、地址或系统", "搜索站点、标签"
	}
	ui.Column(c).Width(a.page().sidebarWidth).FillHeight().Left(a.page().sidebarWidth * (shown - 1)).
		Label(directoryLabel).Children(func() {
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
			ui.TextInputBase(c.Key("host-search"), &a.page().query).Bind(&a.page().search).Fill().MinWidth(0).
				Padding(0, t.C.Search.PaddingX, 0, t.C.Search.Height).Radius(t.C.Search.Radius).
				FontSize(t.C.Search.FontSize).Placeholder(placeholder).Label(searchLabel)
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
			summary := fmt.Sprintf("%d 台主机", len(a.catalog().hosts))
			if a.section == sectionFiles {
				summary = fmt.Sprintf("%d 个站点", len(a.catalog().hosts))
			}
			ui.Text(c, summary).FontSize(t.C.Sider.FooterSize)
		})
	})
}

type hostDraft struct {
	name, address, user, port, group, system string
	protocol                                 string
	auth, keyPath                            string
	id                                       hostID
	editing, deleting, files                 bool
}

func (a *app) startHostDraft() {
	a.page().draft = hostDraft{
		user: "root", port: "22", group: a.catalog().groups[0].name, system: systems[0].name,
		protocol: "SFTP", auth: "agent", files: a.section == sectionFiles,
	}
	a.page().addHostOpen = true
}

func (a *app) hostDialog(c *ui.Context) {
	t := theme.Current()
	modal(c, &a.page().addHostOpen, func() {
		ui.Column(c).Width(t.C.Dialog.HostWidth).Gap(t.Space.N14).Children(func() {
			if a.page().draft.deleting {
				a.deleteSiteDialogContents(c)
				return
			}
			title := "添加 SSH 主机"
			if a.page().draft.editing {
				title = "编辑 SSH 主机"
			}
			if a.page().draft.files {
				title = "添加文件站点"
				if a.page().draft.editing {
					title = "编辑文件站点"
				}
			}
			ui.Text(c, title).FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "保存到主机目录，不会连接服务器。").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
			if a.page().draft.files {
				ui.Text(c, "协议").FontSize(t.Font.Size.Small)
				ui.Select(c, &a.page().draft.protocol, []string{"SFTP", "FTP"}).OnChange(func() {
					if a.page().draft.protocol == "FTP" {
						a.page().draft.port = "21"
					} else {
						a.page().draft.port = "22"
					}
				})
			}
			draftField(c, "名称", &a.page().draft.name)
			draftField(c, "主机地址", &a.page().draft.address)
			ui.Row(c).Gap(t.Space.N12).Children(func() {
				ui.Column(c).Grow(1).Gap(t.Space.N6).Children(func() { draftField(c, "用户名", &a.page().draft.user) })
				ui.Column(c).Width(t.C.Dialog.PortWidth).Gap(t.Space.N6).Children(func() { draftField(c, "端口", &a.page().draft.port) })
			})
			if !a.page().draft.files || a.page().draft.protocol != "FTP" {
				authMethods := []string{"agent", "password", "key"}
				authNames := []string{"SSH Agent", "密码", "私钥"}
				authName := authNames[slices.Index(authMethods, a.page().draft.auth)]
				ui.Text(c, "认证方式").FontSize(t.Font.Size.Small)
				ui.Select(c, &authName, authNames).OnChange(func() {
					a.page().draft.auth = authMethods[slices.Index(authNames, authName)]
				})
				if a.page().draft.auth == "key" {
					ui.Text(c, "私钥路径").FontSize(t.Font.Size.Small)
					ui.Row(c).Gap(t.Space.N8).Children(func() {
						textInput(c, &a.page().draft.keyPath).Label("私钥路径").Grow(1).MinWidth(0)
						button(c, "选择…").OnClick(func() {
							paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
								Parent: a.window, Title: "选择 SSH 私钥", ShowHiddenFiles: true,
							})
							if err != nil {
								c.Toast("选择私钥失败：" + err.Error())
								return
							}
							if len(paths) > 0 {
								a.page().draft.keyPath = paths[0]
							}
						})
					})
				}
				ui.Text(c, "密码和私钥口令在连接时输入，不保存到主机配置。").
					FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
			}
			ui.Text(c, "分组").FontSize(t.Font.Size.Small)
			groupNames := a.groupNames()
			ui.Select(c, &a.page().draft.group, groupNames)
			names := []string{}
			for _, system := range systems {
				names = append(names, system.name)
			}
			ui.Text(c, "系统").FontSize(t.Font.Size.Small)
			ui.Select(c, &a.page().draft.system, names)
			port, err := strconv.Atoi(a.page().draft.port)
			validPort := err == nil && port > 0 && port <= 65535
			validHost := strings.TrimSpace(a.page().draft.name) != "" && strings.TrimSpace(a.page().draft.address) != ""
			validUser := strings.TrimSpace(a.page().draft.user) != ""
			isFTP := a.page().draft.files && a.page().draft.protocol == "FTP"
			validKey := isFTP || a.page().draft.auth != "key" || strings.TrimSpace(a.page().draft.keyPath) != ""
			canAdd := validPort && validHost && validUser && validKey
			ui.Row(c).Justify(ui.End).Gap(t.Space.N8).Children(func() {
				button(c, "取消").OnClick(func() { a.page().addHostOpen = false })
				label := "添加"
				if a.page().draft.editing {
					label = "保存"
				}
				primaryButton(c, label).Disabled(!canAdd).
					OnClick(func() {
						err := a.commitCatalogMove(func() {
							protocol := ""
							if a.page().draft.files {
								protocol = strings.ToLower(a.page().draft.protocol)
							}
							h := host{
								name: strings.TrimSpace(a.page().draft.name), address: strings.TrimSpace(a.page().draft.address),
								user: strings.TrimSpace(a.page().draft.user), port: port,
								os: slices.Index(names, a.page().draft.system), system: a.page().draft.system, protocol: protocol,
								auth: a.page().draft.auth, keyPath: strings.TrimSpace(a.page().draft.keyPath),
							}
							if protocol == "ftp" {
								h.auth, h.keyPath = "password", ""
							}
							id := a.catalog().nextHostID
							group := a.catalog().groups[slices.Index(groupNames, a.page().draft.group)]
							if a.page().draft.editing {
								id = a.page().draft.id
								previous := a.catalog().hosts[id]
								previous.name, previous.address, previous.user = h.name, h.address, h.user
								previous.port, previous.os, previous.system = h.port, h.os, h.system
								previous.protocol = h.protocol
								previous.auth, previous.keyPath = h.auth, h.keyPath
								a.catalog().hosts[id] = previous
								for _, group := range a.catalog().groups {
									if group.name != a.page().draft.group {
										group.hosts = slices.DeleteFunc(group.hosts, func(candidate hostID) bool { return candidate == id })
									}
								}
							} else {
								a.catalog().hosts[id] = h
								a.catalog().nextHostID++
							}
							if !slices.Contains(group.hosts, id) {
								group.hosts = append(group.hosts, id)
							}
							a.setGroupCollapsed(group, false)
						})
						if err != nil {
							c.Toast("保存主机失败：" + err.Error())
							return
						}
						a.page().query = ""
						a.page().addHostOpen = false
					})
			})
		})
	})
}

func (a *app) startGroupDraft() {
	a.page().groupDraft = ""
	a.page().groupDialogOpen = true
}

func (a *app) groupDialog(c *ui.Context) {
	t := theme.Current()
	modal(c, &a.page().groupDialogOpen, func() {
		ui.Column(c).Width(t.C.Dialog.GroupWidth).Gap(t.Space.N14).Children(func() {
			ui.Text(c, "新建分组").FontSize(t.C.Dialog.TitleSize).FontWeight(t.Font.Weight.Semibold)
			ui.Text(c, "将主机拖入分组，整理主机目录。").FontSize(t.Font.Size.Small).TextColor(tokenColor(t.Color.Text.Secondary))
			textInput(c, &a.page().groupDraft).AutoFocus().Label("分组名称").Placeholder("分组名称").FillWidth()
			name := strings.TrimSpace(a.page().groupDraft)
			duplicate := slices.Contains(a.groupNames(), name)
			if duplicate {
				ui.Text(c, "此分组名称已存在").FontSize(t.Font.Size.Caption).TextColor(a.theme.Danger)
			}
			ui.Row(c).Gap(t.Space.N8).Justify(ui.End).Children(func() {
				button(c, "取消").OnClick(func() { a.page().groupDialogOpen = false })
				primaryButton(c, "创建").Disabled(name == "" || duplicate).OnClick(func() {
					if err := a.commitCatalogMove(func() {
						catalog := a.catalog()
						catalog.groups = append(catalog.groups, &hostGroup{id: catalog.nextGroupID, name: name, hosts: []hostID{}})
						catalog.nextGroupID++
					}); err != nil {
						c.Toast("保存分组失败：" + err.Error())
						return
					}
					a.page().groupDialogOpen, a.page().query = false, ""
				})
			})
		})
	})
}

func draftField(c *ui.Context, label string, value *string) {
	t := theme.Current()
	ui.Column(c).Gap(t.Space.N6).Children(func() {
		ui.Text(c, label).FontSize(t.Font.Size.Small)
		textInput(c, value).Label(label).FillWidth()
	})
}
