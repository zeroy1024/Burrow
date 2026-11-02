package desktop

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"sshdesk/internal/theme"
	"sshdesk/internal/uikit"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var (
	fileLaptopIcon    = uikit.LineIcon(`<rect x="4" y="4" width="16" height="12" rx="1"/><path d="M2 20h20l-2-4H4Z"/>`)
	fileNewFolderIcon = uikit.LineIcon(`<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10H3ZM12 10v6m-3-3h6"/>`)
	fileRefreshIcon   = uikit.LineIcon(`<path d="M20 8a8 8 0 1 0 0 8M20 3v6h-6"/>`)
	fileBackIcon      = uikit.LineIcon(`<path d="m14 6-6 6 6 6"/>`)
	fileForwardIcon   = uikit.LineIcon(`<path d="m10 6 6 6-6 6"/>`)
	fileSidebarOnIcon = uikit.LineIcon(`<rect x="3" y="4.5" width="18" height="15" rx="3"/><path d="M9 4.5v15"/><path d="M4 6h4v12H4Z" fill="currentColor" stroke="none"/>`)
)

func (a *app) fileWorkspace(c *ui.Context) {
	t := theme.Current()
	workspace := uikit.FileWorkspace(c)
	workspace.Children(func() {
		session := a.currentFileSession()
		if session == nil {
			uikit.EmptyState(c, uikit.EmptyProps{Kind: uikit.EmptyFiles, Icon: filesIcon,
				Title: "选择站点，浏览远程文件", Detail: "从左侧站点目录打开 SFTP / FTP 会话"})
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
	message := "会话已断开"
	switch session.state {
	case connecting:
		message = "正在连接 " + session.endpoint.name + "…"
	case connectionFailed:
		message = "连接失败"
	}
	props := uikit.EmptyProps{Kind: uikit.EmptyConnection, Icon: filesIcon, Title: message,
		Error: session.errorText, Detail: session.endpoint.user + "@" + session.endpoint.address}
	if session.state != connecting {
		props.Action = "重新连接"
	}
	parts := uikit.EmptyState(c, props)
	if props.Action != "" {
		parts.Action.OnClick(func() { a.reconnectFileSession(session) })
	}
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
	area := uikit.FilePanels(c)
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
	progress := area.AnimateTo("local-open", target, a.positionAnimation(ui.Animation{Spring: uikit.Spring(split.Expand)}))
	opacity := split.RevealOpacityFrom + (1-split.RevealOpacityFrom)*min(max(progress, 0), 1)
	if a.reduceMotion(c) {
		opacity = uikit.Visual(area, "local-fade", target, split.Crossfade)
	}
	remoteLeft := max(progress, 0) * (localWidth + split.Gutter)
	remoteTargetWidth := width
	if target == 1 {
		remoteTargetWidth -= localWidth + split.Gutter
	}
	area.Children(func() {
		localVisible := target != 0 || !area.Settled("local-open") || a.reduceMotion(c) && opacity > 0
		localOverlay := a.reduceMotion(c) && target == 0 && opacity > 0
		localPane := func() {
			x := -(1 - progress) * (localWidth + split.Gutter)
			if a.reduceMotion(c) {
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
			divider := uikit.FileDivider(c, localWidth)
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
			uikit.StyleFileDivider(c, divider, dragging)
		}
	})
}

func (a *app) filePane(c *ui.Context, session *fileSession, pane *filePaneState, targetWidth float32) {
	t := theme.Current()
	compact := targetWidth < t.C.FilePane.CompactWidth
	columns := fileTableColumns(t, compact)
	card := uikit.FilePane(c, pane.remote, map[bool]string{true: "远程文件", false: "本地文件"}[pane.remote], &pane.focus, a.files.activeRemote == pane.remote)
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
		var backdrop func(int) uikit.TableBackdropRow
		if a.translucencyEnabled(c) {
			backdrop = func(index int) uikit.TableBackdropRow {
				entry := entries[index]
				kind, values := fileTableRowData(entry)
				return uikit.TableBackdropRow{Values: values, Leading: func(p *ui.Painter, rect ui.Rect) {
					uikit.PaintFileIcon(p, rect, kind, entry.name, uikit.FileIconProps{
						Selected: pane.table.Selection.Has(entry.path), Focused: a.files.activeRemote == pane.remote,
					})
				}}
			}
		}
		table := uikit.DataTable(c.Key("file-scroll"), &pane.table, &pane.scroll, columns, uikit.TableProps{
			Count: len(entries), HeaderHeight: headHeight, FooterHeight: footerHeight,
			Width: targetWidth, Focused: a.files.activeRemote == pane.remote, Zebra: true, Label: map[bool]string{true: "远程文件", false: "本地文件"}[pane.remote],
			Key: func(index int) string { return entries[index].path }, RowLabel: func(index int) string { return entries[index].name }, Reveal: pane.reveal,
			Underlay: func() { a.fileScrollEdge(c, headHeight, false); a.fileScrollEdge(c, footerHeight, true) },
			Backdrop: backdrop,
		}, func(row uikit.TableRow, table uikit.TableParts) {
			if target := a.fileRow(c, session, pane, entries[row.Index], row, table); target != "" {
				folderTarget = target
			}
		}, func() {
			message := "此文件夹为空"
			if pane.loading {
				message = "正在读取目录…"
			} else if pane.errorText != "" {
				message = pane.errorText
			}
			uikit.EmptyMessage(c, message, uikit.EmptyFolder).PassThrough()
		})
		pane.reveal = ""
		if card.Focused() {
			table.Root.Focus()
		}
		if table.Open >= 0 && table.Open < len(entries) {
			entry := entries[table.Open]
			if entry.dir {
				a.navigateFiles(c, session, pane, entry.path)
			} else if entry.symlink {
				a.queueFileDirectory(session, pane, fileNavigation{path: entry.path, historyIndex: -1, link: &entry})
			} else {
				a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "preview", preview: entry}
			}
		}
		if table.Preview >= 0 && table.Preview < len(entries) && a.files.drag == nil {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "preview", preview: entries[table.Preview]}
		}
		if table.Delete >= 0 && len(pane.selection()) > 0 && fileWritable(pane, pane.path) {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "delete"}
		}
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).PassThrough().Children(func() {
			a.fileToolbar(c, session, pane, compact)
		})
		a.fileStatus(c, session, pane, compact)
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
	uikit.FilePaneHeader(c).Children(func() {
		if pane.remote {
			label := "显示本地面板"
			if a.files.localOpen {
				label = "隐藏本地面板"
			}
			toggle := uikit.PaneToggle(c, &a.files.localOpen, uikit.PaneToggleProps{Label: label, Icon: sidebarIcon, IconOn: fileSidebarOnIcon, Shortcut: "⌘⇧L"})
			if toggle.Changed() {
				if !a.files.localOpen {
					a.files.activeRemote = true
				}
				a.saveFilePreferences(c)
			}
		}
		a.filePaneLabel(c, pane)
		history := uikit.HistoryButtons(c, pane.historyIndex > 0, pane.historyIndex+1 < len(pane.history), fileBackIcon, fileForwardIcon, uikit.HistoryButtonProps{BackShortcut: "⌘←", ForwardShortcut: "⌘→"})
		history.Back.OnClick(func() { a.fileHistory(c, session, pane, -1) })
		history.Forward.OnClick(func() { a.fileHistory(c, session, pane, 1) })
		if compact {
			a.fileSearchSlot(c, session, pane)
			label := "搜索当前文件夹"
			if pane.searchOpen {
				label = "收起搜索"
			}
			uikit.IconButton(c, searchIcon, uikit.IconButtonProps{Key: "file-search-toggle", Label: label}).
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
			uikit.IconButton(c, transferUpIcon, uikit.IconButtonProps{Label: "上传文件"}).
				OnClick(func() { a.chooseUpload(c, session) })
		}
		uikit.IconButton(c, fileNewFolderIcon, uikit.IconButtonProps{Label: "新建文件夹", Disabled: !fileWritable(pane, pane.path)}).OnClick(func() {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "mkdir", name: "新建文件夹"}
		})
		uikit.IconButton(c, fileRefreshIcon, uikit.IconButtonProps{Label: "刷新"}).OnClick(func() { a.refreshFilePane(session, pane) })
		uikit.MenuOn(c, uikit.IconButton(c, moreIcon, uikit.IconButtonProps{Label: "更多文件操作"}), a.filePaneMenu(c, session, pane))
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
		Duration: t.C.Search.Transition, Ease: uikit.Easing(t.C.Search.Easing),
	})
	opacity := uikit.Visual(slot, "search-opacity", target, t.C.Search.Transition)
	reduced := a.reduceMotion(c)
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
	parts := uikit.SearchField(c.Key("file-query"), &pane.query, uikit.SearchProps{Variant: uikit.SearchFilter, Height: t.C.Breadcrumb.Height,
		Handle: &pane.search, Label: "搜索当前文件夹", Placeholder: "搜索文件"})
	return parts.Root.MinWidth(0)
}

func (a *app) filePaneLabel(c *ui.Context, pane *filePaneState) {
	label := "本地"
	if pane.remote {
		label = "远程"
	}
	uikit.PaneLabel(c, label, a.files.localOpen, a.files.activeRemote == pane.remote)
}

func (a *app) fileBreadcrumbs(c *ui.Context, session *fileSession, pane *filePaneState) {
	parts := strings.Split(strings.Trim(pane.path, "/"), "/")
	segments := []uikit.PathSegment{}
	if pane.remote {
		segments = append(segments, uikit.PathSegment{Label: session.endpoint.name, Target: "/"})
	}
	current := ""
	// Preserve the local root: relative breadcrumb targets would otherwise
	// resolve against the application's working directory instead of pane.path.
	if pane.remote || path.IsAbs(pane.path) {
		current = "/"
	}
	for _, part := range parts {
		if part != "" {
			current = path.Join(current, part)
			segments = append(segments, uikit.PathSegment{Label: part, Target: current})
		}
	}
	if len(segments) == 0 {
		segments = append(segments, uikit.PathSegment{Label: "/", Target: "/"})
	}
	icon := fileLaptopIcon
	if pane.remote {
		icon = serverIcon
	}
	bar := uikit.PathBar(c, uikit.PathBarProps{Path: pane.path, Segments: segments, Icon: icon, Separator: fileForwardIcon})
	if bar.Chosen {
		a.navigateFiles(c, session, pane, bar.Target)
	}
}

func fileTableColumns(t *theme.Theme, compact bool) []uikit.Column {
	columns := []uikit.Column{
		{ID: "name", Title: "名称", SortLabel: "按名称排序", MinWidth: t.C.FilePane.NameMinWidth, LeadingWidth: t.Size.Icon + t.C.FileList.ColumnGap, MiddleEllipsis: true, Sortable: true},
		{ID: "size", Title: "大小", SortLabel: "按大小排序", Width: t.C.FilePane.SizeWidth, MinWidth: t.C.FilePane.SizeWidth, Align: uikit.AlignEnd, Sortable: true, TextStyle: uikit.ColumnMetadata},
		{ID: "modified", Title: "修改时间", SortLabel: "按修改时间排序", Width: t.C.FilePane.DateWidth, MinWidth: t.C.FilePane.DateWidth, Sortable: true, TextStyle: uikit.ColumnMetadata},
		{ID: "permissions", Title: "权限", SortLabel: "按权限排序", Width: t.C.FilePane.PermissionsWidth, MinWidth: t.C.FilePane.PermissionsWidth, TextStyle: uikit.ColumnMonoMetadata},
		{ID: "owner", Title: "所有者", SortLabel: "按所有者排序", Width: t.C.FilePane.OwnerWidth, MinWidth: t.C.FilePane.OwnerWidth, TextStyle: uikit.ColumnMetadata},
	}
	if compact {
		return columns[:3]
	}
	return columns
}

func (a *app) fileRow(
	c *ui.Context,
	session *fileSession,
	pane *filePaneState,
	entry fileEntry,
	tableRow uikit.TableRow,
	table uikit.TableParts,
) string {
	t := theme.Current()
	row := tableRow.Element
	selection := pane.selection()
	if !pane.table.Selection.Has(entry.path) {
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
	row.ContextMenu(func(m *ui.Menu) {
		if !pane.table.Selection.Has(entry.path) {
			pane.table.Select(entry.path)
		}
		uikit.NativeMenu(m, a.filePaneMenu(c, session, pane))
	})
	row.Children(func() {
		kind, values := fileTableRowData(entry)
		uikit.FileIcon(c, kind, entry.name, uikit.FileIconProps{Selected: tableRow.Selected, Focused: tableRow.Focused})
		table.Cells(c, values)
	})
	if entry.dir {
		return a.fileDropTarget(c, row, session, pane, entry.path, true)
	}
	return ""
}

func fileTableRowData(entry fileEntry) (uikit.FileKind, []string) {
	kind, size := uikit.FileRegular, "—"
	if entry.dir {
		kind = uikit.FileDirectory
	} else {
		size = transferBytes(entry.size)
	}
	return kind, []string{entry.name, size, entry.modified.Format("01月02日 15:04"), entry.permissions, entry.owner}
}

func (a *app) filePaneMenu(c *ui.Context, session *fileSession, pane *filePaneState) []uikit.MenuEntry {
	selection := pane.selection()
	label, icon := "上传所选项目", uikit.IconUpload
	if pane.remote {
		label, icon = "下载所选项目", uikit.IconDownload
	}
	writable := fileWritable(pane, pane.path)
	entries := []uikit.MenuEntry{
		{ID: "transfer", Label: label, Icon: icon, Disabled: len(selection) == 0, OnChoose: func() {
			target := session.remote
			if pane.remote {
				target = session.local
				a.files.localOpen = true
			}
			a.requestFileTransfer(c, fileDrag{session: session, remote: pane.remote, entries: selection}, target, target.path)
		}},
		{ID: "transfer-divider", Separator: true},
		{ID: "mkdir", Label: "新建文件夹", Icon: uikit.IconFolder, Disabled: !writable, OnChoose: func() {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "mkdir", name: "新建文件夹"}
		}},
		{ID: "rename", Label: "重命名", Icon: uikit.IconEdit, Disabled: len(selection) != 1 || !writable, OnChoose: func() {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "rename", name: selection[0].name, original: selection[0].name}
		}},
		{ID: "copy-path", Label: "复制路径", Icon: uikit.IconCopy, Disabled: len(selection) != 1, OnChoose: func() { mygo.Clipboard.WriteText(selection[0].path) }},
	}
	if !pane.remote {
		entries = append(entries, uikit.MenuEntry{ID: "local-divider", Separator: true},
			uikit.MenuEntry{ID: "local-reveal", Label: "在文件管理器中显示", Icon: uikit.IconFolder, OnChoose: func() { mygo.Shell.ShowItemInFolder(pane.path) }})
	}
	entries = append(entries, uikit.MenuEntry{ID: "danger-divider", Separator: true},
		uikit.MenuEntry{ID: "delete", Label: "删除所选项目", Icon: uikit.IconTrash, Danger: true, Disabled: len(selection) == 0 || !writable, OnChoose: func() {
			a.files.dialog = &fileActionDialog{session: session, pane: pane, kind: "delete"}
		}})
	return entries
}

func (a *app) filePaneKeys(c *ui.Context, card ui.Element, session *fileSession, pane *filePaneState) {
	if card.Shortcut(0, ui.KeyTab) && a.files.localOpen {
		target := session.local
		if !pane.remote {
			target = session.remote
		}
		a.files.activeRemote = target.remote
		target.focus.Focus()
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
	selection := pane.selection()
	var total int64
	for _, entry := range selection {
		total += entry.size
	}
	count := uikit.FormatSelection(len(pane.directories[pane.path]), len(selection), total)
	items := []string{}
	source := ""
	if !compact || !pane.remote {
		source = "本机"
		if pane.remote {
			h := session.endpoint
			source = h.user + "@" + h.name
		}
	}
	showProtocol := pane.remote && !compact
	if !showProtocol {
		items = append(items, count)
	}
	footer := uikit.FilePaneFooter(c, uikit.FooterProps{
		LeadingSVG: map[bool]*ui.SVG{true: lockIcon, false: fileLaptopIcon}[pane.remote],
		Source:     source, Items: items, NoDividers: true,
	}).FontSize(t.C.StatusBar.FontSize)
	footer.Children(func() {
		if showProtocol {
			protocol := "SFTP"
			if session.endpoint.protocol == "ftp" {
				protocol = "FTP"
			}
			uikit.ProtocolBadge(c, protocol)
			uikit.Text(c, count, uikit.TextProps{Style: uikit.TextFootnote, Tone: uikit.ToneSecondary}).
				FontSize(t.C.StatusBar.FontSize).TextColor(uikit.Color(t.C.StatusBar.Text)).SingleLine().MinWidth(0)
		}
		ui.Spacer(c)
	})
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
		uikit.Tip(c, element, "此目录不可写")
		return ""
	}
	element.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if directory == pane.path {
			uikit.DrawDropIndicator(p, r, uikit.DropIndicatorProps{Background: uikit.Color(t.C.DropIndicator.ZoneBg),
				Border: uikit.Color(t.C.DropIndicator.ZoneBorder), Radius: t.C.Card.Radius, BorderWidth: t.C.DropIndicator.ZoneBorderWidth})
		} else {
			uikit.DrawDropIndicator(p, r, uikit.DropIndicatorProps{Background: uikit.Color(t.C.DropIndicator.PlaceholderAccentBg),
				Border: uikit.Color(t.C.DropIndicator.ZoneBorder), Radius: t.C.FileList.RowRadius, BorderWidth: t.C.DropIndicator.ZoneBorderWidth})
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
			uikit.DropHint(c, label, t.C.FilePane.StatusHeight+t.Space.N12)
		})
		return ""
	}
	return directory
}

func (a *app) chooseUpload(c *ui.Context, session *fileSession) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: a.window, Title: "选择上传文件", Multiple: true,
		Message: "上传到当前远程目录。",
	})
	if err != nil {
		uikit.Toast(c, uikit.ToastProps{ID: "打开文件选择器失败：" + err.Error(), Tone: uikit.ToneDanger, Title: "打开文件选择器失败", Detail: err.Error()})
		return
	}
	a.uploadFinderPaths(c, session, paths, session.remote.path)
}

func (a *app) uploadFinderPaths(c *ui.Context, session *fileSession, paths []string, target string) {
	entries := []fileEntry{}
	for _, source := range paths {
		info, err := os.Lstat(source)
		if err != nil {
			uikit.Toast(c, uikit.ToastProps{ID: "无法读取文件信息：" + err.Error(), Tone: uikit.ToneDanger, Title: "无法读取文件信息", Detail: err.Error()})
			return
		}
		entries = append(entries, fileEntry{
			name: filepath.Base(source), path: source, size: info.Size(), dir: info.IsDir(),
			modified: info.ModTime(), permissions: info.Mode().Perm().String(), owner: "本机",
		})
	}
	a.requestFileTransfer(c, fileDrag{session: session, entries: entries}, session.remote, target)
}

func (a *app) fileScrollEdge(c *ui.Context, height float32, bottom bool) {
	background := uikit.Color(theme.Current().C.Card.Bg)
	edge := uikit.EdgeTop
	if bottom {
		edge = uikit.EdgeBottom
	}
	uikit.ScrollFade(c, edge, uikit.ScrollFadeProps{
		Height: height, Background: background,
		Translucent: a.translucencyEnabled(c),
	})
}
