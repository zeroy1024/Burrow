package uikit

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/egoist/mygo/ui"
	"sshdesk/internal/theme"
)

type OSAsset struct {
	Icon  *ui.SVG
	Brand color.NRGBA
}

var osAssets = map[string]OSAsset{}

// RegisterOS installs application resources before the first view is built.
func RegisterOS(name string, asset OSAsset) { osAssets[strings.ToLower(name)] = asset }

func OsLogo(c *ui.Context, name string, size Size) {
	t := theme.Current()
	asset, ok := osAssets[strings.ToLower(name)]
	if !ok {
		asset, ok = osAssets["linux"]
	}
	if !ok {
		asset = OSAsset{Icon: domainLinux, Brand: t.Color.Text.Primary}
	}
	tile := domainSize(size, t.C.OsLogo.Size)
	glyph := tile * t.C.OsLogo.GlyphSize / t.C.OsLogo.Size
	ui.Box(c).Size(tile, tile).Shrink(0).Radius(t.C.OsLogo.Radius).
		Background(Color(t.OSLogoBg(asset.Brand))).Center().PassThrough().Children(func() {
		ui.Image(c, asset.Icon).Size(glyph, glyph).TextColor(Color(asset.Brand)).PassThrough()
	})
}

type Protocol uint8

const (
	ProtocolSFTP Protocol = iota
	ProtocolFTP
	ProtocolSSH
)

func ProtocolIcon(c *ui.Context, protocol Protocol, size Size) {
	p := theme.Current().C.ProtocolIcon
	background, foreground, icon := p.SftpBg, p.SftpGlyph, IconSVG(IconFolderLock)
	if protocol == ProtocolFTP {
		background, foreground, icon = p.FtpBg, p.FtpGlyph, IconSVG(IconFolder)
	}
	if protocol == ProtocolSSH {
		background = theme.Current().Color.Surface.Control
		foreground = theme.Current().Color.Icon.Default
		icon = IconSVG(IconTerminal)
	}
	tile := domainSize(size, p.Size)
	glyph := tile * p.Glyph / p.Size
	ui.Box(c).Size(tile, tile).Shrink(0).Radius(p.Radius).Background(Color(background)).
		Center().PassThrough().Children(func() { ui.Icon(c, icon).Size(glyph, glyph).TextColor(Color(foreground)).PassThrough() })
}

type FileKind uint8

const (
	FileRegular FileKind = iota
	FileDirectory
	FileCode
	FileImage
	FileArchive
	FileKey
	FileLink
)

type FileIconProps struct{ Selected, Focused bool }

func FileIcon(c *ui.Context, kind FileKind, name string, options ...FileIconProps) ui.Element {
	icon, ink := fileIconAppearance(kind, name, options...)
	return ui.Icon(c, icon).Size(theme.Current().Size.Icon, theme.Current().Size.Icon).TextColor(ink).Shrink(0).PassThrough()
}

// PaintFileIcon shares the actual file icon and selection colours with FileIcon
// for decorative drawings that must not create another input or reader target.
func PaintFileIcon(p *ui.Painter, rect ui.Rect, kind FileKind, name string, options ...FileIconProps) {
	icon, ink := fileIconAppearance(kind, name, options...)
	p.Icon(icon, rect, ink)
}

func fileIconAppearance(kind FileKind, name string, options ...FileIconProps) (*ui.SVG, ui.Color) {
	t := theme.Current()
	if kind == FileRegular {
		kind = fileKind(name)
	}
	icon, ink := IconSVG(IconDocument), t.C.FileList.FileIcon
	switch kind {
	case FileDirectory:
		icon, ink = IconSVG(IconFolder), t.C.FileList.FolderIcon
	case FileCode:
		icon = IconSVG(IconCode)
	case FileImage:
		icon = IconSVG(IconImage)
	case FileArchive:
		icon = IconSVG(IconArchive)
	case FileKey:
		icon = IconSVG(IconKey)
	case FileLink:
		icon = LineIcon(`<path d="m10 13 4-4m-6 6-2 2a4 4 0 0 1-6-6l4-4a4 4 0 0 1 6 0m4 2 2-2a4 4 0 0 1 6 6l-4 4a4 4 0 0 1-6 0"/>`)
	}
	if len(options) > 0 && options[0].Selected {
		ink = t.C.FilePaneFocus.SelTextInactive
		if options[0].Focused {
			ink = t.C.FilePaneFocus.SelTextActive
		}
	}
	return icon, Color(ink)
}

func fileKind(name string) FileKind {
	name = strings.ToLower(name)
	for _, extension := range []string{".tar.gz", ".tar.bz2", ".tgz", ".zip", ".gz", ".bz2", ".xz", ".7z", ".rar", ".tar"} {
		if strings.HasSuffix(name, extension) {
			return FileArchive
		}
	}
	for _, extension := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".avif", ".ico"} {
		if strings.HasSuffix(name, extension) {
			return FileImage
		}
	}
	for _, extension := range []string{".go", ".js", ".jsx", ".ts", ".tsx", ".css", ".html", ".json", ".yaml", ".yml", ".sh", ".py", ".rs", ".toml", ".xml"} {
		if strings.HasSuffix(name, extension) {
			return FileCode
		}
	}
	if strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".pub") || strings.HasSuffix(name, ".key") || strings.HasPrefix(name, "id_") {
		return FileKey
	}
	return FileRegular
}

func domainSize(size Size, standard float32) float32 {
	t := theme.Current()
	switch size {
	case SizeXs:
		return t.Size.Icon
	case SizeSm:
		return standard - t.Space.N2
	case SizeLg:
		return standard + t.Space.N4
	}
	return standard
}

type HostLabelProfile uint8

const (
	HostLabelDefault HostLabelProfile = iota
	HostLabelCatalog
)

type HostLabelProps struct {
	Stacked, Dim bool
	Profile      HostLabelProfile
}

func HostLabel(c *ui.Context, name, userHost string, props HostLabelProps) ui.Element {
	t := theme.Current()
	root := ui.Row(c).Gap(t.Space.N6).MinWidth(0).PassThrough()
	if props.Stacked {
		root.Column().Gap(t.Space.N2)
	}
	nameFont := ui.Font{Family: strings.Join(t.Font.Family.Ui, ", "), Size: t.Font.Size.Body, Weight: t.Font.Weight.Medium}
	metaFont := ui.Font{Family: strings.Join(t.Font.Family.Mono, ", "), Size: t.Font.Size.Footnote}
	nameLine, metaLine := float32(0), float32(0)
	metaInk := Color(t.Color.Text.Secondary)
	if props.Profile == HostLabelCatalog {
		p := t.C.SiderItem
		root.Gap(t.Space.N1)
		nameFont.Size, nameFont.Weight, nameLine = p.NameSize, p.NameWeight, p.NameLineHeight
		metaFont.Family, metaFont.Size, metaLine = strings.Join(p.SubFamily, ", "), p.SubSize, p.SubLineHeight
		metaInk = Color(p.SubColor)
	}
	return root.Children(func() {
		ink := t.Color.Text.Primary
		if props.Dim {
			ink = t.Color.Text.Secondary
		}
		nameLabel := MiddleLabel(c, name, MiddleLabelProps{Font: nameFont, Color: Color(ink), LineHeight: nameLine})
		metaLabel := MiddleLabel(c, userHost, MiddleLabelProps{Font: metaFont, Color: metaInk, LineHeight: metaLine})
		if props.Stacked {
			nameLabel.FillWidth()
			metaLabel.FillWidth()
		} else {
			nameLabel.Shrink(1)
			metaLabel.Grow(1)
		}
	})
}

type MiddleLabelProps struct {
	Font       ui.Font
	Color      ui.Color
	LineHeight float32
	Path       bool
	Prefix     string
}

func middleLabel(c *ui.Context, text string, font ui.Font, ink ui.Color, lineHeight float32, path bool) ui.Element {
	return MiddleLabel(c, text, MiddleLabelProps{Font: font, Color: ink, LineHeight: lineHeight, Path: path})
}

// MiddleLabel measures the available width while retaining the complete label
// for assistive technology. Prefix is preserved before shortening the content.
func MiddleLabel(c *ui.Context, text string, props MiddleLabelProps) ui.Element {
	box := ui.Box(c).MinWidth(0).PassThrough().Role(ui.RoleText).Label(props.Prefix + text)
	width := box.LayoutBounds().W
	shown := text
	if width > 0 {
		available := width
		if props.Prefix != "" {
			prefixWidth, _ := ui.ShapeText(props.Prefix, props.Font, 0).Size()
			available = max(0, width-prefixWidth)
		}
		if props.Path {
			shown = MiddlePath(text, available, props.Font)
		} else {
			shown = MiddleText(text, available, props.Font)
		}
	}
	box.Children(func() {
		text := ui.Text(c, props.Prefix+shown).Font(props.Font.Family).FontSize(props.Font.Size).FontWeight(props.Font.Weight).
			TextColor(props.Color).SingleLine().MinWidth(0).PassThrough()
		if props.LineHeight > 0 {
			text.FixedLineHeight(props.LineHeight)
			box.Height(props.LineHeight)
		}
	})
	box.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		if r.W != width {
			c.Invalidate()
		}
	})
	return box
}

// MiddlePath spends the available room on the basename before its directory.
func MiddlePath(text string, width float32, font ui.Font) string {
	if measured, _ := ui.ShapeText(text, font, 0).Size(); measured <= width {
		return text
	}
	separator := strings.LastIndexAny(text, "/\\")
	if separator < 0 {
		return MiddleText(text, width, font)
	}
	name := text[separator+1:]
	suffix := text[separator:]
	nameWidth, _ := ui.ShapeText(name, font, 0).Size()
	suffixWidth, _ := ui.ShapeText(suffix, font, 0).Size()
	if nameWidth >= width {
		return MiddleText(name, width, font)
	}
	markerWidth, _ := ui.ShapeText("…", font, 0).Size()
	if suffixWidth+markerWidth > width {
		return name
	}
	return MiddleText(text[:separator], width-suffixWidth, font) + suffix
}

func PaneBadge(c *ui.Context, n int, focused bool, surface *theme.Theme) ui.Element {
	return PaneShortcut(c, ShortcutDisplay(fmt.Sprintf("Cmd+%d", n)), focused, surface)
}

func ConnectionDot(c *ui.Context, state ConnState, size float32) {
	t := theme.Current()
	ink, halo := Color(t.C.ConnectionIndicator.Offline), Color(t.C.ConnectionIndicator.Offline)
	if state == ConnConnected {
		ink, halo = Color(t.C.ConnectionIndicator.Connected), Color(t.C.ConnectionIndicator.ConnectedHalo)
	}
	if state == ConnConnecting {
		ink, halo = Color(t.C.ConnectionIndicator.Connecting), Color(t.C.ConnectionIndicator.ConnectingHalo)
	}
	ui.Box(c).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).Center().PassThrough().Children(func() {
		dot := ui.Box(c).Size(size, size).Radius(t.Radius.Pill).Background(ink).PassThrough()
		if state != ConnDisconnected {
			dot.Border(t.C.ConnectionIndicator.Halo, halo).Size(size+2*t.C.ConnectionIndicator.Halo, size+2*t.C.ConnectionIndicator.Halo).
				Background(ui.Transparent).Children(func() {
				ui.Box(c).Fill().Radius(t.Radius.Pill).Background(ink).PassThrough()
			})
		}
	})
}

func ConnectionBadge(c *ui.Context, state ConnState, surface *theme.Theme) ui.Element {
	status := [...]Status{
		ConnDisconnected: StatusDisconnected, ConnConnecting: StatusConnecting,
		ConnConnected: StatusConnected, ConnError: StatusError,
	}[state]
	return StatusPill(c, status, StatusPillProps{Surface: surface})
}

func ProtocolBadge(c *ui.Context, protocol string) ui.Element {
	return Badge(c, protocol+" 已连接 · 演示", BadgeProps{Tone: ToneSuccess, Dot: true})
}

type LatencyProps struct {
	Surface *theme.Theme
	Prefix  string
}

func Latency(c *ui.Context, ms int, props LatencyProps) ui.Element {
	t := theme.Current()
	surface := props.Surface
	if surface == nil {
		surface = t
	}
	p := surface.C.Latency
	label := "—"
	ink := p.Normal
	if ms >= 0 {
		label = fmt.Sprintf("%d ms", ms)
		if float32(ms) >= p.BadMs {
			ink = p.Bad
		} else if float32(ms) >= p.WarnMs {
			ink = p.Warn
		}
	}
	if props.Prefix != "" {
		label = props.Prefix + " " + label
	}
	return ui.Text(c, label).FontSize(surface.C.StatusBar.FontSize).SingleLine().TextColor(Color(ink)).FontFeatures("tnum")
}
