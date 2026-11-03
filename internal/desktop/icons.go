package desktop

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

var (
	sidebarIcon = uikit.LineIcon(`
  <rect x="3" y="4.5" width="18" height="15" rx="3"/>
  <path d="M9 4.5v15"/>
`)
	splitIcon = uikit.LineIcon(`
  <rect x="3" y="4.5" width="18" height="15" rx="3"/>
  <path d="M12 4.5v15"/>
`)
	terminalIcon = uikit.LineIcon(`
  <rect x="3" y="4.5" width="18" height="15" rx="3"/>
  <path d="m7 9 3 3-3 3m6 0h4"/>
`)
	filesIcon = uikit.LineIcon(`
  <path d="M21 10V8a2 2 0 0 0-2-2h-6l-2-2H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h5"/>
  <path d="M13 13h8m-3-3 3 3-3 3m3 3h-8m3-3-3 3 3 3"/>
`)
	mcpIcon = uikit.LineIcon(`
  <path d="M7.5 4.5 4.5 7.5a3.5 3.5 0 0 0 5 5l6-6a3.5 3.5 0 0 1 5 5l-3 3"/>
  <path d="m16.5 19.5 3-3a3.5 3.5 0 0 0-5-5l-6 6a3.5 3.5 0 0 1-5-5l3-3"/>
`)
	settingsIcon = uikit.LineIcon(`
  <path d="M10 2h4l.5 3.2 2.1 1.2 3-1.2 2 3.5-2.4 2v2.6l2.4 2-2 3.5-3-1.2-2.1 1.2L14 22h-4
           l-.5-3.2-2.1-1.2-3 1.2-2-3.5 2.4-2v-2.6l-2.4-2 2-3.5 3 1.2 2.1-1.2Z"/>
  <circle cx="12" cy="12" r="3"/>
`)
)

var (
	plusIcon       = uikit.LineIcon(`<path d="M12 5v14M5 12h14"/>`)
	closeIcon      = uikit.LineIcon(`<path d="m7 7 10 10M17 7 7 17"/>`)
	searchIcon     = uikit.LineIcon(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/>`)
	chevronIcon    = uikit.LineIcon(`<path d="m7 10 5 5 5-5"/>`)
	moreIcon       = uikit.LineIcon(`<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>`)
	appearanceIcon = uikit.LineIcon(`
<circle cx="12" cy="12" r="9"/>
<path d="M12 3a9 9 0 0 1 0 18Z" fill="currentColor" stroke="none"/>
`)
	paletteIcon = uikit.LineIcon(`
<path d="M12 3a9 9 0 0 0 0 18h1a2 2 0 0 0 1.6-3.2l-.4-.6a1.4 1.4 0 0 1 1.1-2.2H17a4 4 0 0 0 4-4c0-4.4-4-8-9-8Z"/>
<circle cx="7" cy="11" r="1"/><circle cx="10" cy="7" r="1"/>
<circle cx="15" cy="7" r="1"/><circle cx="18" cy="11" r="1"/>
`)
	textCursorIcon = uikit.LineIcon(`<path d="M3 6V4h12v2M9 4v16m-3 0h6m5-17h4m-2 0v18m-2 0h4"/>`)
	infoIcon       = uikit.LineIcon(`
<circle cx="12" cy="12" r="9"/><path d="M12 11v6m-1-6h1m-1 6h2"/>
<circle cx="12" cy="7" r=".75" fill="currentColor" stroke="none"/>
`)
	serverIcon = uikit.LineIcon(`
<rect x="3" y="3" width="18" height="7" rx="2"/>
<rect x="3" y="14" width="18" height="7" rx="2"/>
<path d="M7 6.5h.01M7 17.5h.01M12 6.5h5M12 17.5h5"/>
`)
	activityIcon = uikit.LineIcon(`<path d="M2 12h5l3-9 4 18 3-9h5"/>`)
	gridIcon     = uikit.LineIcon(`
<rect x="3" y="3" width="7" height="7" rx="1"/>
<rect x="14" y="3" width="7" height="7" rx="1"/>
<rect x="3" y="14" width="7" height="7" rx="1"/>
<rect x="14" y="14" width="7" height="7" rx="1"/>
`)
	expandIcon  = uikit.LineIcon(`<path d="M14 4h6v6M10 20H4v-6M20 4l-7 7M4 20l7-7"/>`)
	restoreIcon = uikit.LineIcon(`<path d="M14 4v6h6M10 20v-6H4M20 4l-6 6M4 20l6-6"/>`)
	bellIcon    = uikit.LineIcon(`<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4"/>`)
	lockIcon    = uikit.LineIcon(`<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V6a4 4 0 0 1 8 0v4"/>`)
	codeIcon    = uikit.LineIcon(`<path d="M8 3H6v7l-2 2 2 2v7h2M16 3h2v7l2 2-2 2v7h-2"/>`)
	ubuntuIcon  = uikit.BrandIcon(`
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
	folderIcon = uikit.LineIcon(`
  <path d="M20 20H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 2h9a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2Z"/>
`)
	folderLockIcon = uikit.LineIcon(`
  <path d="M8 20H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 2h9a2 2 0 0 1 2 2v2"/>
  <rect x="12" y="15" width="10" height="7" rx="2"/><path d="M15 15v-2a2 2 0 0 1 4 0v2"/>
`)
)
