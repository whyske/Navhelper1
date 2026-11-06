package ui

import (
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// newSettingsTab — общие настройки: точка наблюдения для «Погоды»
// и переключатели уведомлений.
func newSettingsTab(a *App) fyne.CanvasObject {
	weather := newWeatherSettingsForm(a)
	notify := newNotifySettings(a)
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		weather,
		widget.NewSeparator(),
		notify,
	)))
}

func parseFloat(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	return strconv.ParseFloat(s, 64)
}

func lower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func containsFold(hay, lq string) bool {
	return strings.Contains(strings.Map(unicode.ToLower, hay), lq)
}
