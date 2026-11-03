package desktop

import (
	"github.com/egoist/mygo/ui"
	"sshdesk/internal/uikit"
)

func (a *app) showCatalogError(c *ui.Context) {
	if a.catalogError == nil {
		return
	}
	uikit.Toast(c, uikit.ToastProps{ID: "catalog-load-error", Title: "无法加载主机配置", Detail: a.catalogError.Error(), Tone: uikit.ToneDanger})
	a.catalogError = nil
}
