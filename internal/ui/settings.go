package ui

import (
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// newSettingsTab — общие настройки (точка наблюдения для вкладки «Погода»).
// Форма вынесена в weatherjournal.go вместе с её ключами хранения.
func newSettingsTab(a *App) fyne.CanvasObject {
	// Точка наблюдения для «Погоды» + блок переключателей уведомлений.
	// НЕ УДАЛЯТЬ вызов notificationsSettings: без него в настройках исчезнет
	// управление уведомлениями (см. notify.go).
	weather := newWeatherSettingsForm(a)
	notif := notificationsSettings(a)
	page := container.NewVBox(
		weather,
		widget.NewSeparator(),
		notif,
	)
	return container.NewVScroll(container.NewPadded(page))
}

func parseFloat(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	return strconv.ParseFloat(s, 64)
}

func lower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func containsFold(hay, lq string) bool {
	return strings.Contains(strings.Map(unicode.ToLower, hay), lq)
}
