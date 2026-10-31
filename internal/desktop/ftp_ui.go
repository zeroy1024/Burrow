package desktop

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"sshdesk/internal/theme"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

var (
	fileFolderIcon    = lineIcon(`<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10H3Z"/>`)
	fileDocumentIcon  = lineIcon(`<path d="M14 2H6v20h12V6Z"/><path d="M14 2v5h5M8 12h8M8 16h6"/>`)
	fileLaptopIcon    = lineIcon(`<rect x="4" y="4" width="16" height="12" rx="1"/><path d="M2 20h20l-2-4H4Z"/>`)
	fileNewFolderIcon = lineIcon(`<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10H3ZM12 10v6m-3-3h6"/>`)
	fileRefreshIcon   = lineIcon(`<path d="M20 8a8 8 0 1 0 0 8M20 3v6h-6"/>`)
	fileBackIcon      = lineIcon(`<path d="m14 6-6 6 6 6"/>`)
	fileForwardIcon   = lineIcon(`<path d="m10 6 6 6-6 6"/>`)
	fileSidebarOnIcon = lineIcon(`<rect x="3" y="4.5" width="18" height="15" rx="3"/><path d="M9 4.5v15"/><path d="M4 6h4v12H4Z" fill="currentColor" stroke="none"/>`)
)

func (a *app) fileWorkspace(c *ui.Context) {
	t := theme.Current()
	workspace := ui.Column(c).Key("file-workspace").Grow(1).MinWidth(0).MinHeight(0)
	workspace.Children(func() {
		session := a.currentFileSession()
		if session == nil {
			workspaceCard(c).Grow(1).FillWidth().Center().Gap(t.Space.N12).Children(func() {
				ui.Icon(c, filesIcon).Size(t.C.Pane.EmptyIconSize, t.C.Pane.EmptyIconSize).
					TextColor(tokenColor(t.Color.Text.Secondary))
				ui.Text(c, "选择站点，浏览远程文件").FontSize(t.C.Pane.EmptyTitleSize)
				ui.Text(c, "从左侧站点目录打开 SFTP / FTP 会话").TextColor(tokenColor(t.Color.Text.Secondary))
			})
		} else if session.state != connected {
			a.fileConnectionPage(c, session)
		} else {
			a.filePanels(c, session)
		}
		minFileHeight := t.C.FilePane.HeaderHeight + t.C.FileList.HeaderHeight +
			t.C.FileList.RowHeight + t.C.FilePane.StatusHeight
		a.transferDock(c, workspace.Bounds().H-minFileHeight-t.C.FTPSplit.Gutter)
	})
}

func (a *app) fileConnectionPage(c *ui.Context, session *fileSession) {
	t := theme.Current()
	workspaceCard(c).FillWidth().Grow(1).Center().Gap(t.Space.N12).Children(func() {
		ui.Icon(c, filesIcon).Size(t.C.Pane.EmptyIconSize, t.C.Pane.EmptyIconSize).
			TextColor(tokenColor(t.Color.Text.Secondary))
		message := "会话已断开"
		switch session.state {
		case connecting:
			message = "正在连接 " + a.hosts[session.host].name + "…"
		case connectionFailed:
			message = "连接失败"
		}
		ui.Text(c, message).FontSize(t.C.Pane.EmptyTitleSize).FontWeight(t.Font.Weight.Semibold)
		if session.errorText != "" {
			ui.Text(c, session.errorText).TextColor(tokenColor(t.Color.Danger.Text))
		}
		if session.state != connecting {
			button(c, "重新连接").OnClick(func() { a.reconnectFileSession(session) })
		}
		ui.Text(c, "演示会话 · 不连接真实服务器").FontSize(t.Font.Size.Caption).TextColor(tokenColor(t.Color.Text.Secondary))
	})
}

func (a *app) toggleLocalFiles(c *ui.Context) {
	a.files.localOpen = !a.files.localOpen
	if !a.files.localOpen {
		a.files.activeRemote = true
		if session := a.currentFileSession(); session != nil {
			session.remote.focus.Focus()
		}
	}
	a.saveFilePreferences(c)
}

func (a *app) filePanels(c *ui.Context, session *fileSession) {
	t := theme.Current()
	split := t.C.FTPSplit
	area := ui.Box(c).Key("file-panels").Grow(1).FillWidth().MinWidth(0).MinHeight(0).ClipX()
	if c.Shortcut(ui.Super|ui.Shift, ui.KeyL) {
		a.toggleLocalFiles(c)
	}
	width := area.Bounds().W
	if width <= 0 {
		c.AnimationFrame()
		return
	}
	available := width - split.Gutter
	localWidth := max(split.MinLocal, min(available*a.files.ratio, available-split.MinRemote))
	// The remote pane keeps its minimum; on a narrow window the local drawer
	// clips rather than reflowing its contents during a spring or resize.
	canExpand := available >= split.MinLocal+split.MinRemote
	if !canExpand {
		localWidth = split.MinLocal
	}
	target := float32(0)
	if a.files.localOpen && canExpand {
		target = 1
	}
	progress := area.AnimateTo("local-open", target, ui.Animation{Spring: tokenSpring(split.Expand)})
	opacity := split.RevealOpacityFrom + (1-split.RevealOpacityFrom)*min(max(progress, 0), 1)
	if c.Preferences().ReduceMotion {
		opacity = animateVisual(area, "local-fade", target, split.Crossfade)
	}
	remoteLeft := max(progress, 0) * (localWidth + split.Gutter)
	remoteTargetWidth := width
	if target == 1 {
		remoteTargetWidth -= localWidth + split.Gutter
	}
	area.Children(func() {
		localVisible := target != 0 || !area.Settled("local-open") || c.Preferences().ReduceMotion && opacity > 0
		localOverlay := c.Preferences().ReduceMotion && target == 0 && opacity > 0
		localPane := func() {
			x := -(1 - progress) * (localWidth + split.Gutter)
			if c.Preferences().ReduceMotion {
				x = 0
			}
			local := ui.Box(c).Key("local-card").Absolute().Left(x).Top(0).Bottom(0).
				Width(localWidth).Opacity(opacity).Disabled(target == 0)
			local.Children(func() { a.filePane(c, session, session.local, localWidth) })
		}
		if localVisible && !localOverlay {
			localPane()
		}
		ui.Box(c).Key("remote-card").Absolute().Left(remoteLeft).Right(0).Top(0).Bottom(0).Children(func() {
			a.filePane(c, session, session.remote, remoteTargetWidth)
		})
		if localVisible && localOverlay {
			localPane()
		}
		if target == 1 && area.Settled("local-open") {
			divider := ui.Box(c).Key("file-divider").Absolute().Left(localWidth).Top(0).Bottom(0).
				Width(split.Gutter).Cursor(ui.CursorResizeEW).Focusable().Role(ui.RoleSplitter).Label("调整本地面板宽度")
			dx, _, dragging := divider.Dragged()
			if dragging {
				a.files.ratio = min(max((localWidth+dx)/available, split.MinLocal/available), 1-split.MinRemote/available)
			}
			if a.files.resizing && !dragging {
				a.saveFilePreferences(c)
			}
			a.files.resizing = dragging
			if divider.DoubleClicked() {
				a.files.ratio = split.DefaultRatio
				a.saveFilePreferences(c)
			}
			if divider.Shortcut(0, ui.KeyLeft) {
				a.files.ratio = max(split.MinLocal/available, a.files.ratio-t.C.Desktop.SiderKeyboardStep/available)
				a.saveFilePreferences(c)
			}
			if divider.Shortcut(0, ui.KeyRight) {
				a.files.ratio = min(1-split.MinRemote/available, a.files.ratio+t.C.Desktop.SiderKeyboardStep/available)
				a.saveFilePreferences(c)
			}
			divider.Range(float64(split.MinLocal), float64(available-split.MinRemote), float64(localWidth))
			if divider.Hovered() || dragging || divider.FocusVisible() {
				color := split.Grip
				if dragging {
					color = split.GripActive
				}
				divider.Center().Children(func() {
					ui.Box(c).Size(split.GripW, split.GripH).Radius(split.GripW).Background(tokenColor(color)).PassThrough()
				})
			}
		}
	})
}

func (a *app) filePane(c *ui.Context, session *fileSession, pane *filePaneState, targetWidth float32) {
	t := theme.Current()
	compact := targetWidth < t.C.FilePane.CompactWidth
	card := ui.Box(c).Key(pane.remote).Fill().Radius(t.C.Card.Radius).Background(tokenColor(t.C.Card.Bg)).Clip().
		Focusable().FocusRing(false).Bind(&pane.focus).
		Label(map[bool]string{true: "远程文件", false: "本地文件"}[pane.remote])
	card.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		p.Stroke(rect, tokenColor(t.Color.Border.Card), t.C.Card.Radius, t.Size.Divider)
	})
	if card.FocusWithin() {
		a.files.activeRemote = pane.remote
	}
	if card.Clicked() {
		a.files.activeRemote = pane.remote
		pane.focus.Focus()
	}
	a.filePaneKeys(c, card, session, pane)
	headHeight := t.C.FilePane.HeaderHeight + t.C.FileList.HeaderHeight
	footerHeight := t.C.FilePane.StatusHeight
	var folderTarget string
	card.Children(func() {
		entries := pane.entries()
		ui.Scroll(c.Key("file-scroll")).Fill().TrackScroll(&pane.scroll).
			Padding(headHeight, t.C.FilePane.ContentPadding, footerHeight, t.C.FilePane.ContentPadding).
			ScrollbarInsets(headHeight, 0, footerHeight, 0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for i, entry := range entries {
					if target := a.fileRow(c, session, pane, entry, i, compact); target != "" {
						folderTarget = target
					}
				}
			})
			if len(entries) == 0 {
				ui.Text(c, "此文件夹为空").FillWidth().Padding(t.Space.N24).TextColor(tokenColor(t.Color.Text.Secondary))
			}
		})
		fileScrollEdge(c, headHeight, false)
		fileScrollEdge(c, footerHeight, true)
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).PassThrough().Children(func() {
			a.fileToolbar(c, session, pane, compact)
			a.fileColumns(c, pane, compact)
		})
		ui.Row(c).Absolute().Bottom(0).Left(0).Right(0).Height(footerHeight).Padding(0, t.C.StatusBar.PaddingX).
			Gap(t.Space.N8).Children(func() { a.fileStatus(c, session, pane, compact) })
	})
	a.fileDropTarget(c, card, session, pane, pane.path, folderTarget == "")
	if folderTarget == "" {
		pane.hoverFolder = ""
	}
	if folderTarget != "" {
		if pane.hoverFolder != folderTarget {
			pane.hoverFolder, pane.hoverStarted = folderTarget, c.Now()
		}
		if c.Now().Sub(pane.hoverStarted) >= t.C.GroupHeader.ExpandDelay {
			a.navigateFiles(c, session, pane, folderTarget)
			pane.hoverFolder = ""
		} else {
			c.After(t.C.GroupHeader.ExpandDelay - c.Now().Sub(pane.hoverStarted))
		}
	}
}

func (a *app) fileToolbar(c *ui.Context, session *fileSession, pane *filePaneState, compact bool) {
	t := theme.Current()
	ui.Row(c).Height(t.C.FilePane.HeaderHeight).Padding(0, t.Space.N8).
		Gap(t.C.FilePane.ToolGap).FillWidth().Children(func() {
		if pane.remote {
			label := "显示本地面板 ⌘⇧L"
			if a.files.localOpen {
				label = "隐藏本地面板 ⌘⇧L"
			}
			toggle := tooltip(c, ui.ToggleBase(c, &a.files.localOpen).Size(t.C.PaneToggle.Size, t.C.PaneToggle.Size).
				Radius(t.C.PaneToggle.Radius).Label(label), label)
			bg, icon := ui.Transparent, tokenColor(t.C.PaneToggle.Icon)
			glyph := sidebarIcon
			if toggle.Hovered() {
				bg = tokenColor(t.C.PaneToggle.BgHover)
			}
			if a.files.localOpen {
				bg, icon, glyph = tokenColor(t.C.PaneToggle.BgOn), tokenColor(t.C.PaneToggle.IconOn), fileSidebarOnIcon
			}
			animation := tokenTween(t.C.PaneToggle.Transition)
			toggle.Background(toggle.AnimateColor("bg", bg, animation)).TextColor(toggle.AnimateColor("fg", icon, animation)).
				Children(func() { ui.Icon(c, glyph).Size(t.C.IconButton.IconSize, t.C.IconButton.IconSize) })
			if toggle.Changed() {
				if !a.files.localOpen {
					a.files.activeRemote = true
				}
				a.saveFilePreferences(c)
			}
		}
		a.filePaneLabel(c, pane)
		iconButton(c, "后退", fileBackIcon).Disabled(pane.historyIndex == 0).
			OnClick(func() { a.fileHistory(c, session, pane, -1) })
		iconButton(c, "前进", fileForwardIcon).Disabled(pane.historyIndex+1 >= len(pane.history)).
			OnClick(func() { a.fileHistory(c, session, pane, 1) })
		if compact {
			a.fileSearchSlot(c, session, pane)
			label := "搜索当前文件夹"
			if pane.searchOpen {
				label = "收起搜索"
			}
			keyedIconButton(c, "file-search-toggle", label, searchIcon).
				Expanded(pane.searchOpen).OnClick(func() {
				pane.searchOpen = !pane.searchOpen
				if pane.searchOpen {
					pane.search.Focus()
					c.AnimationFrame()
				} else {
					pane.query = ""
					pane.focus.Focus()
				}
			})
		} else {
			a.fileBreadcrumbs(c, session, pane)
			fileSearchInput(c, pane).Width(t.C.FilePane.SearchWidth)
		}
		if pane.remote {
			iconButton(c, "上传文件（演示）", transferUpIcon).
				OnClick(func() { a.chooseUpload(c, session) })
		}
		iconButton(c, "新建文件夹", fileNewFolderIcon).Disabled(!fileWritable(pane, pane.path)).OnClick(func() {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "mkdir", name: "新建文件夹"}
		})
		iconButton(c, "刷新", fileRefreshIcon).OnClick(func() { c.Toast("演示目录已刷新") })
		iconButton(c, "更多文件操作", moreIcon).Menu(func(m *ui.Menu) { a.filePaneMenu(c, m, session, pane) })
	})
}

func (a *app) fileSearchSlot(c *ui.Context, session *fileSession, pane *filePaneState) {
	t := theme.Current()
	slot := ui.Row(c).Key("file-location-search").Grow(1).MinWidth(0).Height(t.C.Breadcrumb.Height).ClipX()
	target := float32(0)
	if pane.searchOpen {
		target = 1
	}
	progress := slot.AnimateTo("search-open", target, ui.Animation{
		Duration: t.C.Search.Transition, Ease: tokenEasing(t.C.Search.Easing),
	})
	opacity := animateVisual(slot, "search-opacity", target, t.C.Search.Transition)
	reduced := c.Preferences().ReduceMotion
	slot.Children(func() {
		location := ui.Box(c).Key("file-location").WidthPercent((1 - progress) * 100).Shrink(0).
			Height(t.C.Breadcrumb.Height).MinWidth(0).ClipX().Opacity(1 - opacity).Disabled(pane.searchOpen)
		search := ui.Box(c).Key("file-search").WidthPercent(progress * 100).Shrink(0).
			Height(t.C.Breadcrumb.Height).MinWidth(0).ClipX().Opacity(opacity).Disabled(!pane.searchOpen)
		if reduced {
			// Geometry switches immediately; both layers keep their width while fading.
			location.Absolute().Top(0).Left(0).WidthPercent(100).Hidden(opacity == 1)
			search.Absolute().Top(0).Left(0).WidthPercent(100).Hidden(opacity == 0)
		}
		location.Children(func() { a.fileBreadcrumbs(c, session, pane) })
		search.Children(func() { fileSearchInput(c, pane).Fill() })
	})
}

func fileSearchInput(c *ui.Context, pane *filePaneState) ui.Element {
	t := theme.Current()
	return textInput(c.Key("file-query"), &pane.query).Bind(&pane.search).
		MinWidth(0).Height(t.C.Breadcrumb.Height).Padding(0, t.C.Search.PaddingX).
		Radius(t.C.Search.Radius).Background(tokenColor(t.C.Search.Bg)).
		FontSize(t.C.Search.FontSize).FixedLineHeight(t.Font.LineHeight.Body).
		Label("搜索当前文件夹").Placeholder("搜索文件")
}

func (a *app) filePaneLabel(c *ui.Context, pane *filePaneState) {
	t := theme.Current()
	focus := t.C.FilePaneFocus
	label := "本地"
	if pane.remote {
		label = "远程"
	}
	active := a.files.activeRemote == pane.remote
	bg, color, weight := ui.Transparent, tokenColor(focus.LabelTextInactive), focus.LabelWeightInactive
	if a.files.localOpen && active {
		bg, color, weight = tokenColor(focus.LabelBgActive), tokenColor(focus.LabelTextActive), focus.LabelWeightActive
	}
	if !a.files.localOpen {
		color, weight = tokenColor(t.Color.Text.Primary), focus.LabelWeightActive
	}
	badge := ui.Row(c).Height(t.C.Breadcrumb.Height).Padding(0, focus.LabelPaddingX).Radius(focus.LabelRadius).Shrink(0)
	animation := tokenTween(focus.Transition)
	badge.Background(badge.AnimateColor("label-bg", bg, animation)).Children(func() {
		text := ui.Text(c, label).FontSize(t.Font.Size.Body).FontWeight(weight).SingleLine()
		text.TextColor(text.AnimateColor("label-fg", color, animation))
	})
}

func (a *app) fileBreadcrumbs(c *ui.Context, session *fileSession, pane *filePaneState) {
	t := theme.Current()
	parts := strings.Split(strings.Trim(pane.path, "/"), "/")
	labels, targets := []string{}, []string{}
	if pane.remote {
		labels = append(labels, a.hosts[session.host].name)
		targets = append(targets, "/")
	}
	current := ""
	if pane.remote {
		current = "/"
	}
	for _, part := range parts {
		if part != "" {
			current = path.Join(current, part)
			labels = append(labels, part)
			targets = append(targets, current)
		}
	}
	if len(labels) == 0 {
		labels, targets = []string{"/"}, []string{"/"}
	}
	family := strings.Join(t.Font.Family.Ui, ", ")
	font := ui.Font{Family: family, Size: t.Font.Size.Body, Weight: t.Font.Weight.Regular}
	measure := func(label string) float32 {
		width, _ := ui.ShapeText(label, font, 0).Size()
		return width
	}
	widths := make([]float32, len(labels))
	visible := make([]int, len(labels))
	for i, label := range labels {
		widths[i], visible[i] = measure(label), i
	}
	ellipsisWidth := measure("…")
	// Absolute content leaves the available width to the toolbar, rather than
	// letting a longer path or its collapsed form change its own width budget.
	bar := tooltip(c, ui.Box(c).Key("file-breadcrumbs").Grow(1).MinWidth(0).Clip().Height(t.C.Breadcrumb.Height).
		Radius(t.C.Breadcrumb.Radius).Background(tokenColor(t.C.Breadcrumb.BgOnGlass)).
		Font(family).FontSize(font.Size).FontWeight(font.Weight).Label("当前目录 "+pane.path), pane.path)
	width := bar.LayoutBounds().W
	available := max(0, width-2*t.C.Breadcrumb.PaddingX)
	required := func(indices []int) float32 {
		result := t.Size.Icon + float32(len(indices)-1)*t.Font.Size.Caption +
			float32(2*len(indices)-1)*t.C.Breadcrumb.Gap
		for _, index := range indices {
			if index < 0 {
				result += ellipsisWidth
			} else {
				result += widths[index]
			}
		}
		return result
	}
	bestWidth := required(visible)
	if bestWidth > available {
		// Keep the root and as many recent ancestors as fit, hiding the middle
		// only when doing so saves space. A very short ancestor can be narrower than “…”.
		for tail := 2; tail < len(labels); tail++ {
			candidate := []int{0, -1}
			for index := tail; index < len(labels); index++ {
				candidate = append(candidate, index)
			}
			if size := required(candidate); size < bestWidth {
				visible, bestWidth = candidate, size
			}
			if bestWidth <= available {
				break
			}
		}
	}
	// Bounds are from the previous frame. Rebuild once if this frame lays the
	// bar out at a new width, including after a resize or a search animation.
	bar.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		if r.W != width {
			c.Invalidate()
		}
	})
	bar.Children(func() {
		ui.Row(c).Absolute().Top(0).Bottom(0).Left(t.C.Breadcrumb.PaddingX).Right(t.C.Breadcrumb.PaddingX).
			Gap(t.C.Breadcrumb.Gap).AlignItems(ui.Center).Children(func() {
			icon := fileLaptopIcon
			if pane.remote {
				icon = serverIcon
			}
			ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(tokenColor(t.C.Breadcrumb.Text)).Shrink(0)
			for position, index := range visible {
				if position > 0 {
					ui.Icon(c, fileForwardIcon).Size(t.Font.Size.Caption, t.Font.Size.Caption).
						TextColor(tokenColor(t.C.Breadcrumb.Separator)).Shrink(0)
				}
				label, target := "…", ""
				if index < 0 {
					target = targets[visible[position+1]-1]
				} else {
					label, target = labels[index], targets[index]
				}
				key := struct {
					target   string
					overflow bool
				}{target: target, overflow: index < 0}
				item := tooltip(c, ui.ButtonBase(c).Key(key).Label(label), target).Shrink(1).MinWidth(0).
					TextColor(tokenColor(t.C.Breadcrumb.Text))
				if index == len(labels)-1 {
					item.TextColor(tokenColor(t.C.Breadcrumb.Current))
				}
				item.OnClick(func() { a.navigateFiles(c, session, pane, target) }).Children(func() {
					ui.Text(c, label).SingleLine().MinWidth(0).FontSize(font.Size)
				})
			}
		})
	})
}

func (a *app) fileColumns(c *ui.Context, pane *filePaneState, compact bool) {
	t := theme.Current()
	ui.Row(c).Height(t.C.FileList.HeaderHeight).Padding(0, t.C.FilePane.ContentPadding+t.C.FileList.RowPaddingX).
		Gap(t.C.FileList.RowGap).FillWidth().Children(func() {
		for i, label := range []string{"名称", "大小", "修改时间", "权限", "所有者"} {
			if compact && i > 2 {
				break
			}
			button := ui.ButtonBase(c).Key(label).Justify(ui.Start).
				TextColor(tokenColor(t.C.FileList.HeaderText)).Label("按" + label + "排序")
			if i == 0 {
				button.Grow(1).MinWidth(t.C.FilePane.NameMinWidth)
			} else {
				button.Width(fileColumnWidth(i, t)).Shrink(0)
			}
			if i > 2 {
				button.Disabled(true)
			}
			button.OnClick(func() {
				if pane.sortColumn == i {
					pane.sortDescending = !pane.sortDescending
				} else {
					pane.sortColumn, pane.sortDescending = i, false
				}
			})
			button.Children(func() {
				ui.Text(c, label).FontSize(t.C.FileList.HeaderSize).SingleLine()
				if pane.sortColumn == i {
					rotation := float32(180)
					if pane.sortDescending {
						rotation = 0
					}
					ui.Icon(c, chevronIcon).Size(t.Font.Size.Caption, t.Font.Size.Caption).Rotate(rotation)
				}
			})
		}
	})
}

func fileColumnWidth(index int, t *theme.Theme) float32 {
	switch index {
	case 1:
		return t.C.FilePane.SizeWidth
	case 2:
		return t.C.FilePane.DateWidth
	case 3:
		return t.C.FilePane.PermissionsWidth
	default:
		return t.C.FilePane.OwnerWidth
	}
}

func (a *app) fileRow(
	c *ui.Context,
	session *fileSession,
	pane *filePaneState,
	entry fileEntry,
	index int,
	compact bool,
) string {
	t := theme.Current()
	row := ui.Row(c).Key(entry.path).Height(t.C.FileList.RowHeight).FillWidth().Shrink(0).
		Padding(0, t.C.FileList.RowPaddingX).
		Gap(t.C.FileList.RowGap).Radius(t.C.FileList.RowRadius).Label(entry.name).Role(ui.RoleListItem)
	bg, text := ui.Transparent, tokenColor(t.Color.Text.Primary)
	if index%2 != 0 {
		bg = tokenColor(t.C.FileList.Zebra)
	}
	if row.Hovered() {
		bg = tokenColor(t.Color.State.Hover)
	}
	if pane.selected[entry.name] {
		if a.files.activeRemote == pane.remote {
			bg, text = tokenColor(t.C.FilePaneFocus.SelBgActive), tokenColor(t.C.FilePaneFocus.SelTextActive)
		} else {
			bg, text = tokenColor(t.C.FilePaneFocus.SelBgInactive), tokenColor(t.C.FilePaneFocus.SelTextInactive)
		}
	}
	animation := tokenTween(t.C.FilePaneFocus.Transition)
	row.Background(row.AnimateColor("selection", bg, animation)).
		TextColor(row.AnimateColor("selection-text", text, animation))
	if row.Clicked() {
		a.selectFile(pane, entry, row.ClickModifiers())
	}
	if row.DoubleClicked() {
		if entry.dir {
			a.navigateFiles(c, session, pane, entry.path)
		} else {
			c.Toast(entry.name + " · 演示文件")
		}
	}
	selection := pane.selection()
	if !pane.selected[entry.name] {
		selection = []fileEntry{entry}
	}
	dragValue := fileDrag{session: session, remote: pane.remote, entries: selection}
	row.Drag(dragValue)
	if row.Dragging() {
		a.files.drag, a.files.dragSeen = &dragValue, true
	}
	if drag := a.files.drag; drag != nil && drag.session == session && drag.remote == pane.remote &&
		slices.ContainsFunc(drag.entries, func(source fileEntry) bool { return source.path == entry.path }) {
		row.Opacity(t.C.DragLift.SourceOpacity)
	}
	if pane.reveal == entry.name {
		row.ScrollIntoView()
		pane.reveal = ""
	}
	row.ContextMenu(func(m *ui.Menu) {
		if !pane.selected[entry.name] {
			pane.selected = map[string]bool{entry.name: true}
			pane.anchor = entry.name
		}
		a.filePaneMenu(c, m, session, pane)
	})
	row.Children(func() {
		icon, color := fileDocumentIcon, t.C.FileList.FileIcon
		if entry.dir {
			icon, color = fileFolderIcon, t.C.FileList.FolderIcon
		}
		ui.Icon(c, icon).Size(t.Size.Icon, t.Size.Icon).TextColor(tokenColor(color)).Shrink(0)
		ui.Text(c, entry.name).FontSize(t.C.FileList.FontSize).SingleLine().Grow(1).MinWidth(0)
		sizeText := "—"
		if !entry.dir {
			sizeText = transferBytes(entry.size)
		}
		ui.Text(c, sizeText).Width(t.C.FilePane.SizeWidth).FontSize(t.C.FileList.FontSize).
			TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
		ui.Text(c, entry.modified.Format("01月02日 15:04")).Width(t.C.FilePane.DateWidth).
			FontSize(t.C.FileList.FontSize).TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
		if !compact {
			ui.Text(c, entry.permissions).Width(t.C.FilePane.PermissionsWidth).
				Font(strings.Join(t.Font.Family.Mono, ", ")).FontSize(t.Font.Size.Caption).
				TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
			ui.Text(c, entry.owner).Width(t.C.FilePane.OwnerWidth).FontSize(t.C.FileList.FontSize).
				TextColor(tokenColor(t.C.FileList.Meta)).SingleLine()
		}
	})
	if entry.dir {
		return a.fileDropTarget(c, row, session, pane, entry.path, true)
	}
	return ""
}

func (a *app) filePaneMenu(c *ui.Context, m *ui.Menu, session *fileSession, pane *filePaneState) {
	selection := pane.selection()
	label := "上传所选项目"
	if pane.remote {
		label = "下载所选项目"
	}
	if m.Item(label).Disabled(len(selection) == 0).Chosen() {
		target := session.remote
		if pane.remote {
			target = session.local
			a.files.localOpen = true
		}
		a.requestFileTransfer(c, fileDrag{session: session, remote: pane.remote, entries: selection}, target, target.path)
	}
	m.Separator()
	if m.Item("新建文件夹").Disabled(!fileWritable(pane, pane.path)).Chosen() {
		a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "mkdir", name: "新建文件夹"}
	}
	if m.Item("重命名").Disabled(len(selection) != 1 || !fileWritable(pane, pane.path)).Chosen() {
		a.files.dialog = &fileActionDialog{
			session: session, pane: pane, kind: "rename",
			name: selection[0].name, original: selection[0].name,
		}
	}
	if m.Item("删除所选项目").Disabled(len(selection) == 0 || !fileWritable(pane, pane.path)).Chosen() {
		a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "delete"}
	}
	if m.Item("复制路径").Disabled(len(selection) != 1).Chosen() {
		mygo.Clipboard.WriteText(selection[0].path)
	}
	if !pane.remote {
		m.Separator()
		if m.Item("打开演示项目 dist").Chosen() {
			a.navigateFiles(c, session, pane, "~/Projects/shop-site/dist")
		}
	}
}

func (a *app) filePaneKeys(c *ui.Context, card ui.Element, session *fileSession, pane *filePaneState) {
	if card.Shortcut(ui.Super, ui.KeyA) {
		pane.selected = map[string]bool{}
		for _, entry := range pane.entries() {
			pane.selected[entry.name] = true
		}
	}
	if card.Shortcut(0, ui.KeyTab) && a.files.localOpen {
		target := session.local
		if !pane.remote {
			target = session.remote
		}
		a.files.activeRemote = target.remote
		target.focus.Focus()
	}
	entries := pane.entries()
	if len(entries) == 0 {
		return
	}
	move := 0
	if card.Shortcut(0, ui.KeyDown) {
		move = 1
	}
	if card.Shortcut(0, ui.KeyUp) {
		move = -1
	}
	if move != 0 {
		index := -1
		for i, entry := range entries {
			if pane.anchor == entry.name {
				index = i
				break
			}
		}
		index = min(max(index+move, 0), len(entries)-1)
		a.selectFile(pane, entries[index], 0)
		pane.reveal = entries[index].name
	}
	if card.Shortcut(0, ui.KeyEnter) {
		selection := pane.selection()
		if len(selection) == 1 && selection[0].dir {
			a.navigateFiles(c, session, pane, selection[0].path)
		}
	}
	if card.Shortcut(ui.Super, ui.KeyLeft) {
		a.fileHistory(c, session, pane, -1)
	}
	if card.Shortcut(ui.Super, ui.KeyRight) {
		a.fileHistory(c, session, pane, 1)
	}
}

func (a *app) fileStatus(c *ui.Context, session *fileSession, pane *filePaneState, compact bool) {
	t := theme.Current()
	ui.Icon(c, map[bool]*ui.SVG{true: lockIcon, false: fileLaptopIcon}[pane.remote]).
		Size(t.Size.Icon, t.Size.Icon).TextColor(tokenColor(t.C.StatusBar.Text))
	if !compact || !pane.remote {
		label := "本机 · 演示"
		if pane.remote {
			h := a.hosts[session.host]
			label = h.user + "@" + h.name
		}
		ui.Text(c, label).FontSize(t.C.StatusBar.FontSize).FontWeight(t.C.StatusBar.HostWeight).SingleLine()
	}
	if pane.remote && !compact {
		protocol := "SFTP"
		if a.hosts[session.host].protocol == "ftp" {
			protocol = "FTP"
		}
		ui.Row(c).Padding(t.C.Badge.PaddingYCompact, t.C.Badge.PaddingXCompact).
			Gap(t.C.Badge.Gap).Radius(t.C.Badge.Radius).Background(tokenColor(t.C.Badge.OkBg)).Children(func() {
			ui.Box(c).Size(t.C.ActivityBadge.DotSize, t.C.ActivityBadge.DotSize).
				Radius(t.C.Badge.Radius).Background(tokenColor(t.C.Badge.OkDot))
			ui.Text(c, protocol+" 已连接 · 演示").FontSize(t.C.Badge.FontSizeCompact).
				TextColor(tokenColor(t.C.Badge.OkText)).SingleLine()
		})
	}
	count := fmt.Sprintf("%d 项", len(pane.directories[pane.path]))
	selection := pane.selection()
	if len(selection) > 0 {
		var total int64
		for _, entry := range selection {
			total += entry.size
		}
		count += fmt.Sprintf(" · 已选 %d 项（%s）", len(selection), transferBytes(total))
	}
	ui.Text(c, count).FontSize(t.C.StatusBar.FontSize).TextColor(tokenColor(t.C.StatusBar.Text)).SingleLine().MinWidth(0)
	ui.Spacer(c)
}

func (a *app) fileDropTarget(
	c *ui.Context,
	element ui.Element,
	session *fileSession,
	pane *filePaneState,
	directory string,
	decorate bool,
) string {
	t := theme.Current()
	drag, over := ui.DragOver[fileDrag](element)
	external := pane.remote && element.FileDragOver()
	valid := over && drag.remote != pane.remote && drag.session == session
	writable := fileWritable(pane, directory)
	if source, dropped := ui.Drop[fileDrag](element); dropped && source.session == session {
		a.requestFileTransfer(c, source, pane, directory)
	}
	if pane.remote {
		if paths := element.DroppedFiles(); len(paths) > 0 {
			a.uploadFinderPaths(c, session, paths, directory)
		}
	}
	if !(valid || external) || !decorate {
		return ""
	}
	if !writable {
		tooltip(c, element, "此目录不可写")
		return ""
	}
	element.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if directory == pane.path {
			p.Fill(r, tokenColor(t.C.DropIndicator.ZoneBg), t.C.Card.Radius)
			p.Stroke(r, tokenColor(t.C.DropIndicator.ZoneBorder), t.C.Card.Radius, t.C.DropIndicator.ZoneBorderWidth)
		} else {
			p.Fill(r, tokenColor(t.C.DropIndicator.PlaceholderAccentBg), t.C.FileList.RowRadius)
			p.Stroke(r, tokenColor(t.C.DropIndicator.ZoneBorder), t.C.FileList.RowRadius, t.C.DropIndicator.ZoneBorderWidth)
		}
	})
	if directory == pane.path {
		direction := "下载"
		if pane.remote {
			direction = "上传"
		}
		label := "松开以" + direction + "到 " + directory
		if valid {
			var total int64
			for _, entry := range drag.entries {
				total += entry.size
			}
			label += fmt.Sprintf(" · %d 项 · %s", len(drag.entries), transferBytes(total))
		}
		element.Children(func() {
			ui.Row(c).Absolute().Bottom(t.C.FilePane.StatusHeight + t.Space.N12).
				Left(t.Space.N12).Right(t.Space.N12).Center().PassThrough().Children(func() {
				ui.Text(c, label).Padding(t.Space.N6, t.Space.N12).Radius(t.Radius.Pill).
					Background(tokenColor(t.C.DropIndicator.HintBg)).
					TextColor(tokenColor(t.C.DropIndicator.HintText)).FontSize(t.Font.Size.Footnote)
			})
		})
		return ""
	}
	return directory
}

func (a *app) chooseUpload(c *ui.Context, session *fileSession) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.window, Title: "选择上传文件", Multiple: true,
		Message: "演示传输：仅读取文件名和大小，不上传内容。",
	})
	if err != nil {
		c.Toast("打开文件选择器失败：" + err.Error())
		return
	}
	a.uploadFinderPaths(c, session, paths, session.remote.path)
}

func (a *app) uploadFinderPaths(c *ui.Context, session *fileSession, paths []string, target string) {
	entries := []fileEntry{}
	for _, source := range paths {
		info, err := os.Stat(source)
		if err != nil {
			c.Toast("无法读取文件信息：" + err.Error())
			return
		}
		entries = append(entries, fileEntry{
			name: filepath.Base(source), path: source, size: info.Size(), dir: info.IsDir(),
			modified: info.ModTime(), permissions: info.Mode().Perm().String(), owner: "本机",
		})
	}
	a.requestFileTransfer(c, fileDrag{session: session, entries: entries}, session.remote, target)
}

func fileScrollEdge(c *ui.Context, height float32, bottom bool) {
	t := theme.Current()
	fade := t.C.FadeBar
	angle := float32(180)
	if bottom {
		angle = 0
	}
	band := func() ui.Element {
		e := ui.Box(c).Absolute().Left(0).Right(0).Height(height).PassThrough()
		if bottom {
			return e.Bottom(0)
		}
		return e.Top(0)
	}
	for _, layer := range []struct{ radius, solid, end float32 }{
		{radius: fade.Blur1, solid: fade.Mask1Solid, end: fade.Mask1End},
		{radius: fade.Blur2, solid: fade.Mask2Solid, end: fade.Mask2End},
		{radius: fade.Blur3, solid: fade.Mask3Solid, end: fade.Mask3End},
	} {
		band().Material(glass.Blur{
			Radius: layer.radius,
			Mask: &ui.LinearGradient{
				From: tokenColor(t.C.Card.Bg), To: ui.Transparent, Angle: angle,
				Start: layer.solid, End: layer.end,
			},
		})
	}
	background := tokenColor(t.C.Card.Bg)
	band().LinearGradient(ui.LinearGradient{Angle: angle, Stops: []ui.GradientStop{
		{Color: background.Alpha(fade.TintStart), At: 0},
		{Color: background.Alpha(fade.TintMid), At: fade.TintMidStop},
		{Color: background.Alpha(fade.TintEnd), At: 1},
	}})
}
