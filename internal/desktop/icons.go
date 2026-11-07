package desktop

import (
	"image/color"

	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/uikit"
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
	routeIcon      = uikit.IconSVG(uikit.IconRoute)
	globeIcon      = uikit.IconSVG(uikit.IconGlobe)
)

type systemStyle struct {
	name  string
	color color.NRGBA
	icon  *ui.SVG
	// ids are the os-release ID values (or other-platform names) shown with
	// this logo.
	ids []string
}

// Indexes are persisted for file connections; append new systems only.
const (
	linuxSystem   = 5
	macOSSystem   = 7
	windowsSystem = 8
)

func brand(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 0xff}
}

// Colors are the logos' official ones; uikit.OSLogoInk adjusts a glyph that
// lacks contrast in the current theme.
var systems = []systemStyle{
	{name: "Ubuntu", color: brand(0xE95420), icon: uikit.LogoIcon(ubuntuLogo), ids: []string{"ubuntu"}},
	{name: "Debian", color: brand(0xA81D33), icon: uikit.LogoIcon(debianLogo), ids: []string{"debian"}},
	{name: "CentOS", color: brand(0x262577), icon: uikit.LogoIcon(centosLogo), ids: []string{"centos"}},
	{name: "Alpine", color: brand(0x0D597F), icon: uikit.LogoIcon(alpineLogo), ids: []string{"alpine"}},
	{name: "Fedora", color: brand(0x51A2DA), icon: uikit.LogoIcon(fedoraLogo), ids: []string{"fedora"}},
	{name: "Linux", color: brand(0xFCC624), icon: uikit.LogoIcon(linuxLogo)},
	{name: "Arch", color: brand(0x1793D1), icon: uikit.LogoIcon(archLogo), ids: []string{"arch"}},
	{name: "macOS", color: brand(0x000000), icon: uikit.LogoIcon(appleLogo), ids: []string{"macos"}},
	{name: "Windows", color: brand(0x0078D4), icon: uikit.LogoIcon(windowsLogo), ids: []string{"windows"}},
	{name: "Red Hat", color: brand(0xEE0000), icon: uikit.LogoIcon(redHatLogo), ids: []string{"rhel"}},
	{name: "Rocky Linux", color: brand(0x10B981), icon: uikit.LogoIcon(rockyLogo), ids: []string{"rocky"}},
	{name: "AlmaLinux", color: brand(0x000000), icon: uikit.LogoIcon(almaLogo), ids: []string{"almalinux"}},
	{name: "openSUSE", color: brand(0x73BA25), icon: uikit.LogoIcon(openSUSELogo),
		ids: []string{"opensuse", "opensuse-leap", "opensuse-tumbleweed", "opensuse-slowroll", "opensuse-microos"}},
	{name: "Kali", color: brand(0x557C94), icon: uikit.LogoIcon(kaliLogo), ids: []string{"kali"}},
	{name: "Linux Mint", color: brand(0x86BE43), icon: uikit.LogoIcon(mintLogo), ids: []string{"linuxmint"}},
	{name: "Raspberry Pi OS", color: brand(0xA22846), icon: uikit.LogoIcon(raspberryPiLogo), ids: []string{"raspbian"}},
	{name: "deepin", color: brand(0x007CFF), icon: uikit.LogoIcon(deepinLogo), ids: []string{"deepin"}},
	{name: "FreeBSD", color: brand(0xAB2B28), icon: uikit.LogoIcon(freeBSDLogo), ids: []string{"freebsd"}},
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
