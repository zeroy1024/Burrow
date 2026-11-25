package assets

import "embed"

//go:embed icon.png
var IconPNG []byte

//go:embed tray-template.png
var TrayTemplatePNG []byte

//go:embed splash/*.svg
var Splash embed.FS
