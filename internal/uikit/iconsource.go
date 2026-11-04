package uikit

import (
	"embed"
	"encoding/xml"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type IconName string

const (
	IconClose        IconName = "close"
	IconSearch       IconName = "search"
	IconEye          IconName = "eye"
	IconEyeOff       IconName = "eye-off"
	IconChevronDown  IconName = "chevron-down"
	IconCheck        IconName = "check"
	IconCheckCircle  IconName = "check-circle"
	IconAlertCircle  IconName = "alert-circle"
	IconPlus         IconName = "plus"
	IconMore         IconName = "more"
	IconSidebar      IconName = "sidebar"
	IconTerminal     IconName = "terminal"
	IconFiles        IconName = "files"
	IconSettings     IconName = "settings"
	IconInfo         IconName = "info"
	IconServer       IconName = "server"
	IconActivity     IconName = "activity"
	IconGrid         IconName = "grid"
	IconExpand       IconName = "expand"
	IconBell         IconName = "bell"
	IconLock         IconName = "lock"
	IconCode         IconName = "code"
	IconArrowLeft    IconName = "arrow-left"
	IconArrowRight   IconName = "arrow-right"
	IconFolder       IconName = "folder"
	IconFolderLock   IconName = "folder-lock"
	IconDocument     IconName = "document"
	IconUpload       IconName = "upload"
	IconDownload     IconName = "download"
	IconTrash        IconName = "trash"
	IconPause        IconName = "pause"
	IconPlay         IconName = "play"
	IconRetry        IconName = "retry"
	IconPin          IconName = "pin"
	IconCopy         IconName = "copy"
	IconEdit         IconName = "edit"
	IconImage        IconName = "image"
	IconArchive      IconName = "archive"
	IconKey          IconName = "key"
	IconMCP          IconName = "mcp"
	IconShield       IconName = "shield"
	IconBan          IconName = "ban"
	IconSplit        IconName = "split"
	IconRestore      IconName = "restore"
	IconAppearance   IconName = "appearance"
	IconPalette      IconName = "palette"
	IconTextCursor   IconName = "text-cursor"
	IconLaptop       IconName = "laptop"
	IconFolderPlus   IconName = "folder-plus"
	IconRefresh      IconName = "refresh"
	IconSidebarClose IconName = "sidebar-close"
	IconLink         IconName = "link"
)

const LucideVersion = "1.67.0"

//go:embed icons/lucide/*.svg
var lucideIcons embed.FS

// LucideLicense includes the ISC and Feather-derived MIT notices.
//
//go:embed icons/lucide/LICENSE
var LucideLicense string

var iconFiles = map[IconName]string{
	IconClose:        "x",
	IconSearch:       "search",
	IconEye:          "eye",
	IconEyeOff:       "eye-off",
	IconChevronDown:  "chevron-down",
	IconCheck:        "check",
	IconCheckCircle:  "circle-check",
	IconAlertCircle:  "circle-alert",
	IconPlus:         "plus",
	IconMore:         "ellipsis",
	IconSidebar:      "panel-left",
	IconTerminal:     "square-terminal",
	IconFiles:        "folder-sync",
	IconSettings:     "settings",
	IconInfo:         "info",
	IconServer:       "server",
	IconActivity:     "activity",
	IconGrid:         "layout-grid",
	IconExpand:       "maximize-2",
	IconBell:         "bell",
	IconLock:         "lock",
	IconCode:         "code-xml",
	IconArrowLeft:    "chevron-left",
	IconArrowRight:   "chevron-right",
	IconFolder:       "folder",
	IconFolderLock:   "folder-lock",
	IconDocument:     "file-text",
	IconUpload:       "upload",
	IconDownload:     "download",
	IconTrash:        "trash",
	IconPause:        "pause",
	IconPlay:         "play",
	IconRetry:        "rotate-ccw",
	IconPin:          "pin",
	IconCopy:         "copy",
	IconEdit:         "pencil",
	IconImage:        "image",
	IconArchive:      "archive",
	IconKey:          "key-round",
	IconShield:       "shield",
	IconBan:          "ban",
	IconSplit:        "columns-2",
	IconRestore:      "minimize-2",
	IconAppearance:   "contrast",
	IconPalette:      "palette",
	IconTextCursor:   "text-cursor",
	IconLaptop:       "laptop",
	IconFolderPlus:   "folder-plus",
	IconRefresh:      "refresh-cw",
	IconSidebarClose: "panel-left-close",
	IconLink:         "link",
}

// The cache holds immutable SVG geometry, keyed by the current token stroke.
// Theme colors remain inherited by the rendered icon on every frame.
var namedIcons sync.Map

func IconSVG(name IconName) *ui.SVG {
	stroke := theme.Current().C.IconButton.Stroke
	key := struct {
		Name   IconName
		Stroke float32
	}{name, stroke}
	if icon, ok := namedIcons.Load(key); ok {
		return icon.(*ui.SVG)
	}
	var shapes string
	if name == IconMCP {
		// Lucide excludes brand marks; keep the MCP mark separate from UI assets.
		shapes = `<path d="M7.5 4.5 4.5 7.5a3.5 3.5 0 0 0 5 5l6-6a3.5 3.5 0 0 1 5 5l-3 3"/><path d="m16.5 19.5 3-3a3.5 3.5 0 0 0-5-5l-6 6a3.5 3.5 0 0 1-5-5l3-3"/>`
	} else {
		file, ok := iconFiles[name]
		if !ok {
			panic(fmt.Sprintf("uikit: unknown icon %q", name))
		}
		data, err := lucideIcons.ReadFile("icons/lucide/" + file + ".svg")
		if err != nil {
			panic(err)
		}
		var document struct {
			Shapes string `xml:",innerxml"`
		}
		if err := xml.Unmarshal(data, &document); err != nil {
			panic(err)
		}
		shapes = document.Shapes
	}
	icon, _ := namedIcons.LoadOrStore(key, strokedIcon(shapes, stroke))
	return icon.(*ui.SVG)
}

// BrandIcon keeps brand geometry independent from the application's UI stroke.
func BrandIcon(shapes string) *ui.SVG { return strokedIcon(shapes, 1.6) }

// LogoIcon is a filled single-color 24×24 logo path drawn in the text color.
func LogoIcon(path string) *ui.SVG {
	return ui.MustParseSVG(fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="currentColor" d="%s"/></svg>`, path))
}

func strokedIcon(shapes string, stroke float32) *ui.SVG {
	return ui.MustParseSVG(fmt.Appendf(nil, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"
fill="none" stroke="currentColor" stroke-width="%g" stroke-linecap="round" stroke-linejoin="round">%s</svg>`,
		stroke, shapes))
}
