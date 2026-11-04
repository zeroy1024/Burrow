package desktop

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

var (
	sidebarIcon  = uikit.IconSVG(uikit.IconSidebar)
	splitIcon    = uikit.IconSVG(uikit.IconSplit)
	terminalIcon = uikit.IconSVG(uikit.IconTerminal)
	filesIcon    = uikit.IconSVG(uikit.IconFiles)
	mcpIcon      = uikit.IconSVG(uikit.IconMCP)
	settingsIcon = uikit.IconSVG(uikit.IconSettings)
)

var (
	plusIcon       = uikit.IconSVG(uikit.IconPlus)
	closeIcon      = uikit.IconSVG(uikit.IconClose)
	searchIcon     = uikit.IconSVG(uikit.IconSearch)
	chevronIcon    = uikit.IconSVG(uikit.IconChevronDown)
	moreIcon       = uikit.IconSVG(uikit.IconMore)
	appearanceIcon = uikit.IconSVG(uikit.IconAppearance)
	paletteIcon    = uikit.IconSVG(uikit.IconPalette)
	textCursorIcon = uikit.IconSVG(uikit.IconTextCursor)
	infoIcon       = uikit.IconSVG(uikit.IconInfo)
	serverIcon     = uikit.IconSVG(uikit.IconServer)
	activityIcon   = uikit.IconSVG(uikit.IconActivity)
	gridIcon       = uikit.IconSVG(uikit.IconGrid)
	expandIcon     = uikit.IconSVG(uikit.IconExpand)
	restoreIcon    = uikit.IconSVG(uikit.IconRestore)
	bellIcon       = uikit.IconSVG(uikit.IconBell)
	lockIcon       = uikit.IconSVG(uikit.IconLock)
	codeIcon       = uikit.IconSVG(uikit.IconCode)
	ubuntuIcon     = uikit.BrandIcon(`
<circle cx="12" cy="12" r="6.2" stroke-width="3"/>
<circle cx="20" cy="12" r="2.8" fill="currentColor" stroke="none"/>
<circle cx="8" cy="5" r="2.8" fill="currentColor" stroke="none"/>
<circle cx="8" cy="19" r="2.8" fill="currentColor" stroke="none"/>
`)
	debianIcon = uikit.BrandIcon(`
<path d="M13 11c4 0 4-5 1-6-4-2-8 2-8 6 0 6 5 10 11 9-5-1-8-4-8-8 0-3 2-5 5-4 2 1 1 4-1 3Z"/>
`)
	centosIcon = uikit.BrandIcon(`
<path d="M12 2 22 12 12 22 2 12ZM5 5h14v14H5ZM12 2v20M2 12h20M5 5l14 14M19 5 5 19" stroke-width="1"/>
`)
	alpineIcon = uikit.BrandIcon(`
<circle cx="12" cy="12" r="10" fill="currentColor" stroke="none"/>
<path d="m5 17 7-9 7 9-4-2-3-4-4 6Z" fill="white" stroke="none"/>
`)
	fedoraIcon = uikit.BrandIcon(`
<circle cx="12" cy="12" r="10" fill="currentColor" stroke="none"/>
<path d="M15 5c-3-1-4 1-4 4v7c0 4-6 4-6 0 0-3 3-4 9-4M8 9h9" stroke="white" stroke-width="1.7"/>
`)
	linuxIcon = uikit.BrandIcon(`
<ellipse cx="12" cy="14" rx="6" ry="8" fill="currentColor"/>
<ellipse cx="12" cy="15" rx="4" ry="5.5" fill="white" stroke="none"/>
<circle cx="12" cy="6" r="3.5" fill="currentColor"/>
<path d="m10 7 2 2 2-2M8 19l-3 2h5m6-2 3 2h-5" stroke="currentColor"/>
<circle cx="11" cy="5" r=".5" fill="white"/>
<circle cx="13" cy="5" r=".5" fill="white"/>
`)
	archIcon = uikit.BrandIcon(`<path d="m12 2 10 20-7-4-3-6-3 6-7 4Z" fill="currentColor" stroke="none"/>`)
)

type systemStyle struct {
	name  string
	color color.NRGBA
	icon  *ui.SVG
}

const linuxSystem = 5

var systems = []systemStyle{
	{name: "Ubuntu", color: color.NRGBA{R: 0xed, G: 0x54, B: 0x22, A: 0xff}, icon: ubuntuIcon},
	{name: "Debian", color: color.NRGBA{R: 0xbd, G: 0x17, B: 0x47, A: 0xff}, icon: debianIcon},
	{name: "CentOS", color: color.NRGBA{R: 0x65, G: 0x59, B: 0xaa, A: 0xff}, icon: centosIcon},
	{name: "Alpine", color: color.NRGBA{R: 0x08, G: 0xb7, B: 0x89, A: 0xff}, icon: alpineIcon},
	{name: "Fedora", color: color.NRGBA{R: 0x4c, G: 0x9e, B: 0xd8, A: 0xff}, icon: fedoraIcon},
	{name: "Linux", color: color.NRGBA{R: 0x34, G: 0x34, B: 0x38, A: 0xff}, icon: linuxIcon},
	{name: "Arch", color: color.NRGBA{R: 0x15, G: 0x99, B: 0xd3, A: 0xff}, icon: archIcon},
}

func init() {
	for _, system := range systems {
		uikit.RegisterOS(system.name, uikit.OSAsset{Icon: system.icon, Brand: system.color})
	}
}

var (
	folderIcon     = uikit.IconSVG(uikit.IconFolder)
	folderLockIcon = uikit.IconSVG(uikit.IconFolderLock)
)
