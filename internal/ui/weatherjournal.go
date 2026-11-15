// Модуль «Погода» v2.0 — судовой гидрометеорологический журнал на реальных
// данных (Open-Meteo + METAR). Заменяет старый internal/weather: направление
// ветра словами, диапазоны скорости, явления погоды текстом, волнение в
// баллах, видимость в кабельтовых, факт по METAR и оправдываемость прогноза
// в процентах — в формате судового журнала (см. internal/forecast).
package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/forecast"
	"ru.navhelper/internal/store"
)

// Ключи настроек точки наблюдения.
const (
	wjKeyLat      = "weather.lat"
	wjKeyLon      = "weather.lon"
	wjKeyPlace    = "weather.place"
	wjKeyDays     = "weather.history_days"
	wjDefaultLat  = 59.9500 // Финский залив, рейд Кронштадта
	wjDefaultLon  = 29.5000
	wjDefaultName = "Финский залив (Кронштадт)"
)

var wjPresets = map[string][2]float64{
	"Севастополь (Севастопольская бухта)": {44.6166, 33.5254},
	"Финский залив (Кронштадт)":           {59.95, 29.50},
	"Кольский залив (Мурманск)":           {69.10, 33.45},
	"Цемесская бухта (Новороссийск)":      {44.68, 37.83},
	"Владивосток (Амурский залив)":        {43.05, 131.80},
	"Калининград (Балтийск)":              {54.65, 19.85},
	"Баренцево море":                      {70.50, 38.00},
}

// wjPeriod — одна строка журнала.
type wjPeriod struct {
	Title    string
	Label    string
	Forecast []forecast.Line
	HasFact  bool
	Fact     []forecast.Line
	FactText string // факт одной строкой в формате судового журнала
	Station  string
	Verdict  forecast.Verdict
}

// weatherTab — вкладка «Погода».
type weatherTab struct {
	a      *App
	st     *store.Store
	client *forecast.Client

	content fyne.CanvasObject
	place   *widget.Label
	status  *widget.Label
	box     *fyne.Container

	// Индикация загрузки: спиннер + текст. Показываются только пока идёт
	// запрос, чтобы на медленном интернете было видно, что приложение живо.
	spinner     *widget.ProgressBarInfinite
	loading     *fyne.Container
	refreshBtn  *widget.Button
	citySel     *widget.Select

	mu      sync.Mutex
	running bool
}

func newWeatherTab(a *App) *weatherTab {
	t := &weatherTab{a: a, st: a.St, client: forecast.NewClient()}
	t.place = bold("")
	t.status = wrapLabel("Загрузка данных…")
	t.box = container.NewVBox()

	// Спиннер и надпись «Загрузка…» — единственный признак того, что запрос
	// ещё идёт. На медленном соединении без него казалось, что кнопка не работает.
	t.spinner = widget.NewProgressBarInfinite()
	t.loading = container.NewVBox(t.spinner, wrapLabel("Загрузка прогноза…"))
	t.loading.Hide()

	t.refreshBtn = widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), func() { t.syncAsync() })
	t.refreshBtn.Importance = widget.HighImportance
	refreshBtn := t.refreshBtn

	names := make([]string, 0, len(wjPresets))
	for k := range wjPresets {
		names = append(names, k)
	}
	sort.Strings(names)
	t.citySel = widget.NewSelect(names, nil)
	citySel := t.citySel
	citySel.PlaceHolder = "Выбрать город / акваторию…"
	if cur := t.st.Setting(wjKeyPlace, wjDefaultName); cur != "" {
		if _, ok := wjPresets[cur]; ok {
			citySel.SetSelected(cur)
		}
	}
	citySel.OnChanged = func(s string) {
		p, ok := wjPresets[s]
		if !ok {
			return
		}
		_ = t.st.SetSetting(wjKeyPlace, s)
		_ = t.st.SetSetting(wjKeyLat, fmt.Sprint(p[0]))
		_ = t.st.SetSetting(wjKeyLon, fmt.Sprint(p[1]))
		t.updatePlaceLabel()
		t.syncAsync()
	}

	top := container.NewVBox(t.place, citySel, refreshBtn, t.loading, t.status, widget.NewSeparator())
	scroll := container.NewVScroll(t.box)
	t.content = container.NewBorder(top, nil, nil, nil, scroll)

	t.updatePlaceLabel()
	t.syncAsync()
	return t
}

func (t *weatherTab) updatePlaceLabel() {
	lat := t.st.SettingFloat(wjKeyLat, wjDefaultLat)
	lon := t.st.SettingFloat(wjKeyLon, wjDefaultLon)
	place := t.st.Setting(wjKeyPlace, wjDefaultName)
	t.place.SetText(fmt.Sprintf("Погода: %s (%.4f, %.4f)", place, lat, lon))
}

func (t *weatherTab) historyDays() int {
	d := int(t.st.SettingFloat(wjKeyDays, 30))
	if d < 1 {
		d = 1
	}
	if d > 30 {
		d = 30
	}
	return d
}

// reload — перечитать настройки и запросить данные заново (после смены
// точки наблюдения в настройках).
func (t *weatherTab) reload() {
	t.updatePlaceLabel()
}

// syncAsync — сбор данных в фоне, не чаще одного раза одновременно.
//
// Что делает индикацию: пока идёт запрос, показываются бегущая полоса
// «Загрузка прогноза…», кнопка «Обновить» и выбор города блокируются,
// чтобы нельзя было запустить второй запрос поверх первого.
func (t *weatherTab) syncAsync() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.mu.Unlock()

	fyne.Do(func() {
		t.loading.Show()
		t.spinner.Start()
		t.refreshBtn.Disable()
		t.citySel.Disable()
		t.status.SetText("Идёт загрузка, подождите…")
	})

	go func() {
		periods, err := t.fetch(t.historyDays())
		fyne.Do(func() {
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()

			t.spinner.Stop()
			t.loading.Hide()
			t.refreshBtn.Enable()
			t.citySel.Enable()

			if err != nil {
				t.status.SetText("⚠ " + err.Error())
				return
			}
			t.status.SetText("Обновлено: " + time.Now().Format("02.01 15:04"))
			t.render(periods)
			// Уведомление «прогноз на сегодня» (если включено в настройках).
			// Данные уже загружены, дополнительных запросов не делаем.
			t.a.notifier.pushWeather(periods)
		})
	}()
}

// fetch собирает данные с внешних источников.
//
// Все независимые сетевые вызовы идут ПАРАЛЛЕЛЬНО. Раньше они шли строгой
// цепочкой (станция → прогноз → архив → море → METAR), поэтому раздел
// открывался очень долго: время складывалось из всех запросов подряд.
// Теперь это время самого медленного запроса плюс один короткий второй круг
// для METAR (он зависит от кода станции).
// fetch собирает данные за период. Запросов всего четыре, и они идут параллельно:
//
//   1. Live      — прогноз на сегодня и ближайшие дни (Open-Meteo forecast);
//   2. Archive   — что Open-Meteo прогнозировал за прошедшие дни (для проверки
//                  оправдываемости прогноза);
//   3. Marine    — волнение и температура воды (Open-Meteo marine), прогноз и факт;
//   4. Observed  — фактическая погода за прошедшие дни (ERA5, архив Open-Meteo).
//
// ВАЖНО: раньше сюда добавлялись поиск ближайшего аэропорта и METAR
// (aviationweather.gov). Они медленные и часто не отвечают, из-за них раздел
// грузился по минуте. Поэтому они убраны: фактическая погода берётся из ERA5.
// Не возвращайте их без необходимости.
func (t *weatherTab) fetch(daysBack int) ([]wjPeriod, error) {
	lat := t.st.SettingFloat(wjKeyLat, wjDefaultLat)
	lon := t.st.SettingFloat(wjKeyLon, wjDefaultLon)
	c := t.client

	var (
		wg       sync.WaitGroup
		live     map[time.Time]*forecast.Hour
		arch     map[time.Time]*forecast.Hour
		sea      map[time.Time]*forecast.Hour
		seaPrev  map[time.Time]*forecast.Hour
		observed map[time.Time]*forecast.Hour
		off      int
		liveErr  error
	)

	// Один общий таймаут на все запросы: если интернет совсем плохой,
	// раздел вернёт ошибку через 30 секунд, а не зависнет.
	shared, cancelAll := ctx(30 * time.Second)
	defer cancelAll()

	wg.Add(4)
	go func() {
		defer wg.Done()
		live, off, liveErr = c.Live(shared, lat, lon)
	}()
	go func() {
		defer wg.Done()
		arch, _ = c.Archive(shared, lat, lon, daysBack)
	}()
	go func() {
		defer wg.Done()
		sea, seaPrev, _ = c.MarineHours(shared, lat, lon, daysBack, 3)
	}()
	go func() {
		defer wg.Done()
		observed, _ = c.Observed(shared, lat, lon, daysBack)
	}()
	wg.Wait()

	if liveErr != nil {
		return nil, fmt.Errorf("прогноз погоды: %w", liveErr)
	}

	// Переменная st больше не нужна (станции нет). Если понадобится снова
	// подписать факт кодом станции — верните поиск станции выше.
	var obs []forecast.Obs

	loc := time.FixedZone("local", off)
	now := time.Now()
	ln := now.In(loc)
	start := time.Date(ln.Year(), ln.Month(), ln.Day(), 9, 0, 0, 0, loc)
	if ln.Before(start) {
		start = start.AddDate(0, 0, -1)
	}

	merge := func(a, wave map[time.Time]*forecast.Hour) []forecast.Hour {
		var out []forecast.Hour
		for tm, h := range a {
			hh := *h
			if s, ok := wave[tm]; ok {
				hh.Wave, hh.HasWave = s.Wave, s.HasWave
				if s.HasWater {
					hh.Water, hh.HasWater = s.Water, true
				}
			}
			out = append(out, hh)
		}
		return out
	}
	liveH := merge(live, sea)
	archH := merge(arch, seaPrev)
	var seaList []forecast.Hour
	for _, h := range sea {
		seaList = append(seaList, *h)
	}
	// Фактическая погода из архива (реанализ ERA5) — идёт отдельным источником,
	// чтобы блок «Факт» заполнялся даже без METAR ближайшего аэропорта.
	var obsList []forecast.Hour
	for _, h := range observed {
		hh := *h
		if s, ok := sea[h.T]; ok {
			hh.Wave, hh.HasWave = s.Wave, s.HasWave
			if s.HasWater {
				hh.Water, hh.HasWater = s.Water, true
			}
		}
		obsList = append(obsList, hh)
	}

	var days []int
	days = append(days, 2, 1, 0)
	for i := 1; i <= daysBack; i++ {
		days = append(days, -i)
	}

	var out []wjPeriod
	for _, d := range days {
		s := start.AddDate(0, 0, d).UTC()
		e := s.Add(24 * time.Hour)
		src, label := liveH, "ПРЕДСТОЯЩИЙ"
		switch {
		case d == 0:
			label = "ДЕЙСТВУЮЩИЙ"
		case d < 0:
			src, label = archH, "ПРОШЕДШИЙ"
		}
		f := forecast.Summarize(src, s, e, loc)
		p := wjPeriod{Title: forecast.PeriodTitle(s, e, loc), Label: label, Forecast: forecast.ForecastLines(f)}
		if d <= 0 {
			// Факт: сначала METAR (приборные наблюдения), при его отсутствии —
			// архив реанализа ERA5, который содержит ветер, видимость, погоду,
			// температуру воздуха и воды.
			fa := forecast.SummarizeFact(obs, seaList, s, e, loc)
			if fa.Obs == 0 {
				fa = forecast.SummarizeObserved(obsList, seaList, s, e, loc)
				fa.Source = "архив ERA5"
			} else {
				fa.Source = "METAR " + fa.Station
			}
			if fa.Obs > 0 || fa.HasSea {
				p.HasFact = true
				p.Fact = forecast.FactLines(fa)
				p.Station = fa.Station
				p.Verdict = forecast.Verify(f, fa, now)
				p.FactText = forecast.FactSummary(fa, p.Verdict)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// render — строит карточки журнала в виджете.
func (t *weatherTab) render(periods []wjPeriod) {
	t.box.Objects = nil
	for _, p := range periods {
		t.box.Add(t.card(p))
	}
	t.box.Refresh()
}

func (t *weatherTab) card(p wjPeriod) fyne.CanvasObject {
	rows := []fyne.CanvasObject{headRow(p.Title, p.Label)}
	for _, l := range p.Forecast {
		rows = append(rows, fieldRow(l.Label, l.Value))
	}
	if p.HasFact {
		rows = append(rows, widget.NewSeparator())
		factHead := "Факт"
		if p.Station != "" {
			factHead += " (" + p.Station + ")"
		}
		rows = append(rows, bold(factHead+":"))

		// Факт одной строкой в формате судового журнала.
		if p.FactText != "" {
			fact := wrapLabel(p.FactText)
			fact.TextStyle = fyne.TextStyle{Bold: true}
			rows = append(rows, fact)
		} else {
			for _, l := range p.Fact {
				rows = append(rows, fieldRow(l.Label, l.Value))
			}
		}

		if p.Verdict.Percent >= 0 {
			pct := widget.NewLabelWithStyle(fmt.Sprintf("Оправдываемость прогноза: %d%%", p.Verdict.Percent), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			rows = append(rows, pct)
			for _, c := range p.Verdict.Checks {
				mark := "✗"
				if c.OK {
					mark = "✓"
				}
				rows = append(rows, wrapLabel(fmt.Sprintf("%s %s: прогноз — %s; факт — %s", mark, c.Name, c.Forecast, c.Fact)))
			}
		}
	}
	bg := canvas.NewRectangle(color.NRGBA{0xff, 0xff, 0xff, 0xff})
	bg.CornerRadius = 10
	bg.StrokeColor = color.NRGBA{0xd3, 0xdc, 0xe6, 0xff}
	bg.StrokeWidth = 1
	return container.NewPadded(container.NewStack(bg, container.NewPadded(container.NewVBox(rows...))))
}

func parseFloatWJ(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	return strconv.ParseFloat(s, 64)
}

// newWeatherSettingsForm — форма выбора точки наблюдения (вызывается из
// вкладки «Настройки»).
func newWeatherSettingsForm(a *App) fyne.CanvasObject {
	place := widget.NewEntry()
	place.SetText(a.St.Setting(wjKeyPlace, wjDefaultName))
	lat := widget.NewEntry()
	lat.SetText(fmt.Sprintf("%.4f", a.St.SettingFloat(wjKeyLat, wjDefaultLat)))
	lon := widget.NewEntry()
	lon.SetText(fmt.Sprintf("%.4f", a.St.SettingFloat(wjKeyLon, wjDefaultLon)))
	days := widget.NewSelect([]string{"1", "3", "7", "14", "30"}, nil)
	days.SetSelected(a.St.Setting(wjKeyDays, "30"))

	names := make([]string, 0, len(wjPresets))
	for k := range wjPresets {
		names = append(names, k)
	}
	preset := widget.NewSelect(names, func(s string) {
		p := wjPresets[s]
		place.SetText(s)
		lat.SetText(fmt.Sprintf("%.4f", p[0]))
		lon.SetText(fmt.Sprintf("%.4f", p[1]))
	})
	preset.PlaceHolder = "Выбрать акваторию…"

	save := widget.NewButton("Сохранить", func() {
		la, err1 := parseFloatWJ(lat.Text)
		lo, err2 := parseFloatWJ(lon.Text)
		if err1 != nil || err2 != nil || la < -90 || la > 90 || lo < -180 || lo > 180 {
			dialog.ShowInformation("Ошибка", "Широта: -90…90, долгота: -180…180 (десятичные градусы).", a.Win)
			return
		}
		_ = a.St.SetSetting(wjKeyPlace, strings.TrimSpace(place.Text))
		_ = a.St.SetSetting(wjKeyLat, fmt.Sprint(la))
		_ = a.St.SetSetting(wjKeyLon, fmt.Sprint(lo))
		_ = a.St.SetSetting(wjKeyDays, days.Selected)
		if a.weather != nil {
			a.weather.reload()
			a.weather.syncAsync()
		}
		dialog.ShowInformation("Настройки", "Сохранено. Погода обновляется для новой точки.", a.Win)
	})
	save.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Акватория", preset),
		widget.NewFormItem("Название", place),
		widget.NewFormItem("Широта", lat),
		widget.NewFormItem("Долгота", lon),
		widget.NewFormItem("Глубина истории, дн.", days),
	)

	about := wrapLabel(
		"Источники данных:\n" +
			"• Прогноз и архив прогнозов — Open-Meteo (Forecast + Previous Runs + Marine API), без ключа.\n" +
			"• Факт — METAR ближайшего аэропорта (aviationweather.gov, с резервом OGIMET) и анализ волнения.\n" +
			"• Направление ветра — 8 румбов, скорость и волнение — диапазоном, видимость — в кабельтовых.\n" +
			"• Оправдываемость прогноза считается только для периодов, где уже есть факт.",
	)
	return container.NewVBox(form, save, widget.NewSeparator(), about)
}
