package ui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/weather"
)

// newSettingsTab — общие настройки (хранятся в таблице settings общей БД).
func newSettingsTab(a *App) fyne.CanvasObject {
	place := widget.NewEntry()
	place.SetText(a.St.Setting(weather.KeyPlace, weather.DefaultPlace))
	lat := widget.NewEntry()
	lat.SetText(fmt.Sprintf("%.4f", a.St.SettingFloat(weather.KeyLat, weather.DefaultLat)))
	lon := widget.NewEntry()
	lon.SetText(fmt.Sprintf("%.4f", a.St.SettingFloat(weather.KeyLon, weather.DefaultLon)))
	days := widget.NewSelect([]string{"7", "14", "30"}, nil)
	days.SetSelected(a.St.Setting(weather.KeyHistDays, "30"))

	presets := map[string][2]float64{
		"Финский залив (Кронштадт)": {59.95, 29.50},
		"Кольский залив (Мурманск)": {69.10, 33.45},
		"Цемесская бухта (Новороссийск)": {44.68, 37.83},
		"Владивосток (Амурский залив)": {43.05, 131.80},
		"Калининград (Балтийск)": {54.65, 19.85},
		"Баренцево море": {70.50, 38.00},
	}
	names := make([]string, 0, len(presets))
	for k := range presets {
		names = append(names, k)
	}
	preset := widget.NewSelect(names, func(s string) {
		p := presets[s]
		place.SetText(s)
		lat.SetText(fmt.Sprintf("%.4f", p[0]))
		lon.SetText(fmt.Sprintf("%.4f", p[1]))
	})
	preset.PlaceHolder = "Выбрать акваторию…"

	save := widget.NewButton("Сохранить", func() {
		la, err1 := parseFloat(lat.Text)
		lo, err2 := parseFloat(lon.Text)
		if err1 != nil || err2 != nil || la < -90 || la > 90 || lo < -180 || lo > 180 {
			dialog.ShowInformation("Ошибка", "Широта: -90…90, долгота: -180…180 (десятичные градусы).", a.Win)
			return
		}
		_ = a.St.SetSetting(weather.KeyPlace, strings.TrimSpace(place.Text))
		_ = a.St.SetSetting(weather.KeyLat, fmt.Sprint(la))
		_ = a.St.SetSetting(weather.KeyLon, fmt.Sprint(lo))
		_ = a.St.SetSetting(weather.KeyHistDays, days.Selected)
		if a.weather != nil {
			a.weather.reload()
			a.weather.syncAsync(true)
		}
		dialog.ShowInformation("Настройки", "Сохранено. Погода обновляется для новой точки.", a.Win)
	})
	save.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Акватория", preset),
		widget.NewFormItem("Название", place),
		widget.NewFormItem("Широта", lat),
		widget.NewFormItem("Долгота", lon),
		widget.NewFormItem("История, дней", days),
	)

	about := wrapLabel(
		"Источники данных:\n" +
			"• Погода — Open-Meteo (Forecast + Marine API), без ключа. Видимость → кабельтовы (1 каб = 185,2 м), " +
			"ветер → баллы Бофорта (ВМО), волна → баллы волнения (шкала ВМО 3700).\n" +
			"• Предупреждения — ФГБУ «Главсевморпуть» (NAVAREA XX/XXI), ПРИП АМП Западной Арктики. Кэш для офлайна.\n" +
			"• Каталог карт — электронный каталог УНиО (НавиСофт), встроен в приложение, работает офлайн.\n\n" +
			"Android не запускает приложение в фоне по расписанию без нативного кода, поэтому данные собираются при " +
			"открытии и раз в час, пока приложение открыто. Пропущенные дни погоды дозаполняются автоматически.")

	return container.NewVScroll(container.NewVBox(bold("Точка для погоды"), form, save, widget.NewSeparator(), about))
}

func parseFloat(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	return strconv.ParseFloat(s, 64)
}

func lower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func containsFold(hay, lq string) bool {
	return strings.Contains(strings.Map(unicode.ToLower, hay), lq)
}
