package uikit

import (
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type IconName string

const (
	IconClose       IconName = "close"
	IconSearch      IconName = "search"
	IconEye         IconName = "eye"
	IconEyeOff      IconName = "eye-off"
	IconChevronDown IconName = "chevron-down"
	IconCheck       IconName = "check"
	IconCheckCircle IconName = "check-circle"
	IconAlertCircle IconName = "alert-circle"
	IconPlus        IconName = "plus"
	IconMore        IconName = "more"
	IconSidebar     IconName = "sidebar"
	IconTerminal    IconName = "terminal"
	IconFiles       IconName = "files"
	IconSettings    IconName = "settings"
	IconInfo        IconName = "info"
	IconServer      IconName = "server"
	IconActivity    IconName = "activity"
	IconGrid        IconName = "grid"
	IconExpand      IconName = "expand"
	IconBell        IconName = "bell"
	IconLock        IconName = "lock"
	IconCode        IconName = "code"
	IconArrowLeft   IconName = "arrow-left"
	IconArrowRight  IconName = "arrow-right"
	IconFolder      IconName = "folder"
	IconFolderLock  IconName = "folder-lock"
	IconDocument    IconName = "document"
	IconUpload      IconName = "upload"
	IconDownload    IconName = "download"
	IconTrash       IconName = "trash"
	IconPause       IconName = "pause"
	IconPlay        IconName = "play"
	IconRetry       IconName = "retry"
	IconPin         IconName = "pin"
	IconCopy        IconName = "copy"
	IconEdit        IconName = "edit"
	IconImage       IconName = "image"
	IconArchive     IconName = "archive"
	IconKey         IconName = "key"
)

var iconShapes = map[IconName]string{
	IconClose:       `<path d="m7 7 10 10M17 7 7 17"/>`,
	IconSearch:      `<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/>`,
	IconEye:         `<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>`,
	IconEyeOff:      `<path d="m3 3 18 18M10.6 5.1A12 12 0 0 1 12 5c6.5 0 10 7 10 7a16 16 0 0 1-4.1 4.9M6.2 6.2A17 17 0 0 0 2 12s3.5 7 10 7a12 12 0 0 0 5-1.1M10 10a3 3 0 0 0 4 4"/>`,
	IconChevronDown: `<path d="m7 10 5 5 5-5"/>`,
	IconCheck:       `<path d="m5 12 4 4L19 6"/>`,
	IconCheckCircle: `<circle cx="12" cy="12" r="9"/><path d="m7 12 3 3 7-7"/>`,
	IconAlertCircle: `<circle cx="12" cy="12" r="9"/><path d="M12 7v6m0 4h.01"/>`,
	IconPlus:        `<path d="M12 5v14M5 12h14"/>`,
	IconMore:        `<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`,
	IconSidebar:     `<rect x="3" y="4.5" width="18" height="15" rx="3"/><path d="M9 4.5v15"/>`,
	IconTerminal:    `<rect x="3" y="4.5" width="18" height="15" rx="3"/><path d="m7 9 3 3-3 3m6 0h4"/>`,
	IconFiles:       `<path d="M21 10V8a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h5"/><path d="M13 13h8m-3-3 3 3-3 3m3 3h-8m3-3-3 3 3 3"/>`,
	IconSettings:    `<path d="M10 2h4l.5 3.2 2.1 1.2 3-1.2 2 3.5-2.4 2v2.6l2.4 2-2 3.5-3-1.2-2.1 1.2L14 22h-4l-.5-3.2-2.1-1.2-3 1.2-2-3.5 2.4-2v-2.6l-2.4-2 2-3.5 3 1.2 2.1-1.2Z"/><circle cx="12" cy="12" r="3"/>`,
	IconInfo:        `<circle cx="12" cy="12" r="9"/><path d="M12 11v6m-1-6h1m-1 6h2"/><circle cx="12" cy="7" r=".75" fill="currentColor" stroke="none"/>`,
	IconServer:      `<rect x="3" y="3" width="18" height="7" rx="2"/><rect x="3" y="14" width="18" height="7" rx="2"/><path d="M7 6.5h.01M7 17.5h.01M12 6.5h5M12 17.5h5"/>`,
	IconActivity:    `<path d="M2 12h5l3-9 4 18 3-9h5"/>`,
	IconGrid:        `<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>`,
	IconExpand:      `<path d="M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7"/>`,
	IconBell:        `<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/>`,
	IconLock:        `<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V6a4 4 0 0 1 8 0v4"/>`,
	IconCode:        `<path d="M8 3H6v7l-2 2 2 2v7h2M16 3h2v7l2 2-2 2v7h-2"/>`,
	IconArrowLeft:   `<path d="m14 6-6 6 6 6"/>`,
	IconArrowRight:  `<path d="m10 6 6 6-6 6"/>`,
	IconFolder:      `<path d="M20 20H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 2h9a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2Z"/>`,
	IconFolderLock:  `<path d="M8 20H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 2h9a2 2 0 0 1 2 2v2"/><rect x="12" y="15" width="10" height="7" rx="2"/><path d="M15 15v-2a2 2 0 0 1 4 0v2"/>`,
	IconDocument:    `<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6M8 13h8M8 17h6"/>`,
	IconUpload:      `<path d="M12 16V3m-5 5 5-5 5 5M4 15v5h16v-5"/>`,
	IconDownload:    `<path d="M12 3v13m-5-5 5 5 5-5M4 15v5h16v-5"/>`,
	IconTrash:       `<path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7m4-7v7"/>`,
	IconPause:       `<path d="M8 5v14M16 5v14"/>`,
	IconPlay:        `<path d="m8 4 12 8-12 8Z"/>`,
	IconRetry:       `<path d="M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-1l2 3M4 15l2 3a7 7 0 0 0 12-1"/>`,
	IconPin:         `<path d="m9 3 6 0-1 6 4 4v2h-5v7h-2v-7H6v-2l4-4Z"/>`,
	IconCopy:        `<rect x="8" y="8" width="13" height="13" rx="2"/><path d="M16 8V3H3v13h5"/>`,
	IconEdit:        `<path d="m16 3 5 5-12 12-6 1 1-6Z"/><path d="m13 6 5 5"/>`,
	IconImage:       `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8" cy="8" r="2"/><path d="m21 15-5-5-6 7-3-3-4 5"/>`,
	IconArchive:     `<path d="M4 8h16v13H4Z"/><rect x="3" y="3" width="18" height="5" rx="1"/><path d="M10 12h4"/>`,
	IconKey:         `<circle cx="8" cy="8" r="5"/><path d="m12 12 9 9m-3-3 3-3m-6 0 3-3"/>`,
}

// The cache holds immutable SVG geometry, keyed by the current token stroke.
// Theme colors remain inherited by the rendered icon on every frame.
var namedIcons sync.Map

func IconSVG(name IconName) *ui.SVG {
	shapes, ok := iconShapes[name]
	if !ok {
		panic(fmt.Sprintf("uikit: unknown icon %q", name))
	}
	stroke := theme.Current().C.IconButton.Stroke
	key := struct {
		Name   IconName
		Stroke float32
	}{name, stroke}
	if icon, ok := namedIcons.Load(key); ok {
		return icon.(*ui.SVG)
	}
	icon, _ := namedIcons.LoadOrStore(key, strokedIcon(shapes, stroke))
	return icon.(*ui.SVG)
}

func LineIcon(shapes string) *ui.SVG {
	return strokedIcon(shapes, theme.Current().C.IconButton.Stroke)
}

// BrandIcon keeps brand geometry independent from the application's UI stroke.
func BrandIcon(shapes string) *ui.SVG { return strokedIcon(shapes, 1.6) }

func strokedIcon(shapes string, stroke float32) *ui.SVG {
	return ui.MustParseSVG(fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"
fill="none" stroke="currentColor" stroke-width="%g" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		stroke, shapes))
}
