package desktop

import (
	"github.com/egoist/mygo/ui"
	"github.com/zeroy1024/Burrow/internal/locales"
	"github.com/zeroy1024/Burrow/internal/uikit"
)

func (a *app) showCatalogError(c *ui.Context) {
	if a.catalogError == nil {
		return
	}
	uikit.Toast(
		c,
		uikit.ToastProps{ID: "catalog-load-error", Title: locales.Deferred(
			locales.MessageUnableToLoadHostConfiguration,
		), Detail: locales.ErrorText(
			a.catalogError,
		), Tone: uikit.ToneDanger},
	)
	a.catalogError = nil
}
