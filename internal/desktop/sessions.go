package desktop

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
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
		ui.Row(c).Height(34).Shrink(0).Margin(0, 12).Padding(0, 9).Gap(6).
			Radius(8).Background(ui.Hex("#f4f4f6")).Children(func() {
			ui.Icon(c, searchIcon).Size(15, 15).TextColor(a.theme.TextMuted)
			ui.TextInputBase(c.Key("host-search"), &a.query).Bind(&a.search).Grow(1).MinWidth(0).
				FontSize(12).Placeholder("搜索主机、标签").Label("搜索主机、地址或系统")
			ui.Text(c, "⌘K").FontSize(10).TextColor(a.theme.TextMuted).
				Padding(1, 4).Radius(4).Background(ui.Hex("#ffffff")).Border(1, a.theme.Border)
		})
		query := strings.ToLower(strings.TrimSpace(a.query))
		matches := 0
		var pendingMove func()
		ui.Scroll(c.Key("session-list")).Grow(1).MinHeight(0).Padding(12, 8).Gap(12).Children(func() {
			for _, group := range a.groups {
				ids := []hostID{}
				for _, id := range group.hosts {
					h := a.hosts[id]
					text := h.name + " " + h.user + " " + h.address + " " + systems[h.os].name + " " + group.name
					if strings.Contains(strings.ToLower(text), query) {
						ids = append(ids, id)
					}
				}
				if len(ids) == 0 && query != "" {
					continue
				}
				matches += len(ids)
				if move := a.catalogGroup(c, group, ids); move != nil {
					pendingMove = move
				}
			}
			if matches == 0 && query != "" {
				ui.Text(c, "没有匹配的主机").FontSize(12).TextColor(a.theme.TextMuted).Padding(12)
			}
		})
		// Reordering is applied after the render loop to keep item identities consistent within a frame.
		if pendingMove != nil {
			pendingMove()
		}
		ui.Row(c).Height(34).Shrink(0).Gap(8).Padding(0, 14).TextColor(a.theme.TextMuted).Children(func() {
			ui.Icon(c, serverIcon).Size(14, 14)
			ui.Text(c, fmt.Sprintf("%d 台主机", len(a.hosts))).FontSize(11)
		})
	})
}

func (a *app) catalogGroup(c *ui.Context, group *hostGroup, ids []hostID) func() {
	box := ui.Column(c).Key(group.id)
	var pendingMove func()
	if source, over := ui.DragOver[*hostGroup](box); over && source != group {
		_, y, _ := box.PointerPosition()
		dropMarker(box, y >= box.Bounds().H/2, a.theme.Accent)
	}
	if source, dropped := ui.Drop[*hostGroup](box); dropped {
		_, y, _ := box.PointerPosition()
		after := y >= box.Bounds().H/2
		pendingMove = func() { a.moveGroup(source, group, after) }
	}
	box.Children(func() {
		// Separate typed targets allow host drops inside a draggable group.
		content := ui.Column(c).FillWidth().Gap(2)
		_, hostOver := ui.DragOver[hostDrag](content)
		if drag, dropped := ui.Drop[hostDrag](content); dropped {
			position := len(group.hosts)
			pendingMove = func() {
				moveHost(drag, group, position)
				group.collapsed = false
			}
		}
		content.Children(func() {
			collapsed := group.collapsed && strings.TrimSpace(a.query) == ""
			header := ui.ButtonBase(c).Height(26).Padding(0, 8).Gap(5).Drag(group).
				Label(group.name).Expanded(!collapsed).TextColor(a.theme.TextMuted)
			if hostOver {
				header.TextColor(a.theme.Accent)
			}
			header.OnClick(func() { group.collapsed = !group.collapsed })
			header.ContextMenu(func(m *ui.Menu) {
				if m.Item("添加主机").Chosen() {
					a.startHostDraft()
					a.draft.group = group.name
				}
				if m.Item("新建分组").Chosen() {
					a.startGroupDraft()
				}
			})
			header.Children(func() {
				arrow := ui.Icon(c, chevronIcon).Size(11, 11)
				if collapsed {
					arrow.Rotate(-90)
				}
				ui.Text(c, group.name).FontSize(11)
				ui.Spacer(c)
				ui.Text(c, strconv.Itoa(len(ids))).FontSize(11)
			})
			ui.Column(c).Hidden(collapsed).Gap(2).Children(func() {
				for _, id := range ids {
					if move := a.hostRow(c, group, id); move != nil {
						pendingMove = move
					}
				}
				if len(group.hosts) == 0 {
					ui.Text(c, "拖动主机到此分组").FontSize(11).TextColor(a.theme.TextMuted).Padding(8)
				}
			})
		})
	})
	return pendingMove
}

func (a *app) hostRow(c *ui.Context, group *hostGroup, id hostID) func() {
	h := a.hosts[id]
	row := ui.ButtonBase(c).Key(int(id)).Height(50).Padding(0, 8).Gap(6).Radius(8).
		Role(ui.RoleListItem).Label(h.name).Description(h.user + "@" + h.address).
		TextColor(a.theme.Text)
	if row.Hovered() {
		row.Background(ui.Hex("#f7f7f9"))
	}
	activate := row.DoubleClicked()
	if row.Focused() && row.Shortcut(0, ui.KeyEnter) {
		activate = true
	}
	menu := func(m *ui.Menu) {
		if m.Item("新建会话").Chosen() {
			a.openHost(id)
		}
		if m.Item("复制连接地址").Chosen() {
			mygo.Clipboard.WriteText(fmt.Sprintf("%s@%s:%d", h.user, h.address, h.port))
		}
	}
	row.ContextMenu(menu)
	row.Children(func() {
		style := systems[h.os]
		ui.Box(c).Size(24, 24).Shrink(0).Radius(7).Background(ui.Hex(style.tint)).Center().
			Tooltip(h.system).Children(func() {
			ui.Image(c, style.icon).Size(16, 16).TextColor(ui.Hex(style.color))
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, h.name).FontSize(13).FontWeight(550).SingleLine().Ellipsis("…")
			ui.Text(c, h.user+"@"+h.address).Font("Menlo, monospace").FontSize(10.5).
				TextColor(a.theme.TextMuted).SingleLine().Ellipsis("…")
		})
		action := ui.ButtonBase(c).Key("host-actions").Size(24, 24).Shrink(0).Radius(7).
			Background(ui.Hex("#ededf0")).TextColor(a.theme.TextMuted).
			Label("更多操作 " + h.name).Description(h.system).Tooltip("更多操作")
		target := float32(0)
		if action.Hovered() || action.FocusVisible() {
			target = 1
		}
		action.Opacity(action.Animate("more", target, 140*time.Millisecond))
		action.Children(func() {
			ui.Icon(c, moreIcon).Size(16, 16)
		})
		action.Menu(menu)
		handle := ui.Box(c).Key("host-drag-handle").Absolute().Top(0).Bottom(0).Left(0).Right(32).
			Label("拖动主机 " + h.name)
		a.hostDragHandle(c, handle, hostDrag{id: id, group: group})
		if handle.DoubleClicked() {
			activate = true
		}
	})
	if activate {
		a.openHost(id)
	}
	if drag, over := ui.DragOver[hostDrag](row); over && drag.id != id {
		_, y, _ := row.PointerPosition()
		dropMarker(row, y >= row.Bounds().H/2, a.theme.Accent)
	}
	if drag, dropped := ui.Drop[hostDrag](row); dropped {
		_, y, _ := row.PointerPosition()
		position := slices.Index(group.hosts, id)
		if y >= row.Bounds().H/2 {
			position++
		}
		return func() { moveHost(drag, group, position) }
	}
	return nil
}

func dropMarker(element ui.Element, after bool, color ui.Color) {
	element.DrawOver(func(p *ui.Painter, r ui.Rect) {
		y := r.Y
		if after {
			y += r.H - 2
		}
		p.Fill(ui.Rect{X: r.X + 6, Y: y, W: r.W - 12, H: 2}, color, 1)
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
						id := hostID(len(a.hosts))
						a.hosts = append(a.hosts, host{
							name: strings.TrimSpace(a.draft.name), address: strings.TrimSpace(a.draft.address),
							user: strings.TrimSpace(a.draft.user), port: port,
							os:     slices.Index(names, a.draft.system),
							system: a.draft.system,
						})
						group := a.groups[slices.Index(groupNames, a.draft.group)]
						group.hosts = append(group.hosts, id)
						group.collapsed, a.query = false, ""
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
					a.groups = append(a.groups, &hostGroup{id: len(a.groups), name: name, hosts: []hostID{}})
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
