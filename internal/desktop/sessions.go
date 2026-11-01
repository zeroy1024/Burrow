package desktop

import (
	"fmt"
	"slices"
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
		a.sidebarSearch(c.Key("host-search"), &a.page().query, &a.page().search, searchLabel, placeholder)
		a.catalogList(c)
		ui.Row(c).Shrink(0).Gap(t.Space.N8).Padding(t.C.Sider.FooterPaddingY, t.C.Sider.FooterPaddingX).
			BorderWidth(t.Size.Hairline, 0, 0, 0).BorderColor(tokenColor(t.C.Sider.FooterBorder)).
			TextColor(tokenColor(t.C.Sider.FooterText)).Children(func() {
			ui.Icon(c, serverIcon).Size(t.Size.IconSm, t.Size.IconSm)
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
			ui.Text(c, summary).FontSize(t.C.Sider.FooterSize)
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
						a.groups = append(a.groups, a.newCatalogGroup(name))
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
