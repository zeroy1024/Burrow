package desktop

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
)

func localizedDate(value time.Time, date, clock i18n.Style) string {
	return localizedDateOptions(value, i18n.DateOptions{Date: date, Time: clock})
}

func localizedDateOptions(value time.Time, options i18n.DateOptions) string {
	locale := mygo.App.Region().FormatLocale(mygo.App.Locale())
	return i18n.FormatDate(locale, value, options)
}

func (a *app) refreshLocalizedViews() {
	if a.businessReady() {
		a.refreshMCPBackend()
	}
	if a.window != nil {
		a.window.Invalidate()
	}
}
