// Модуль «Погода» v2.0 — судовой гидрометеорологический журнал на реальных
// данных (Open-Meteo + METAR). Заменяет старый internal/weather: направление
// ветра словами, диапазоны скорости, явления погоды текстом, волнение в
// баллах, видимость в кабельтовых, факт по METAR и оправдываемость прогноза
// в процентах — в формате судового журнала (см. internal/forecast).
package ui

import (
	"fmt"
	"image/color"
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
	"Финский залив (Кронштадт)":      {59.95, 29.50},
	"Кольский залив (Мурманск)":      {69.10, 33.45},
	"Цемесская бухта (Новороссийск)": {44.68, 37.83},
	"Владивосток (Амурский залив)":   {43.05, 131.80},
	"Калининград (Балтийск)":         {54.65, 19.85},
	"Баренцево море":                 {70.50, 38.00},
}

// wjPeriod — одна строка журнала.
type wjPeriod struct {
	Title    string
	Label    string
	Forecast []forecast.Line
	HasFact  bool
	Fact     []forecast.Line
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

	mu      sync.Mutex
	running bool
}

func newWeatherTab(a *App) *weatherTab {
	t := &weatherTab{a: a, st: a.St, client: forecast.NewClient()}
	t.place = bold("")
	t.status = wrapLabel("Загрузка данных…")
	t.box = container.NewVBox()

	refreshBtn := widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), func() { t.syncAsync() })
	refreshBtn.Importance = widget.HighImportance

	top := container.NewVBox(t.place, refreshBtn, t.status, widget.NewSeparator())
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
	d := int(t.st.SettingFloat(wjKeyDays, 3))
	if d < 1 {
		d = 1
	}
	if d > 7 {
		d = 7
	}
	return d
}

// reload — перечитать настройки и запросить данные заново (после смены
// точки наблюдения в настройках).
func (t *weatherTab) reload() {
	t.updatePlaceLabel()
}

// syncAsync — живой сбор данных в фоне, не чаще одного раза одновременно.
func (t *weatherTab) syncAsync() {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	t.running = true
	t.mu.Unlock()

	t.status.SetText("Загрузка прогноза и наблюдений…")
	go func() {
		periods, err := t.fetch(t.historyDays())
		fyne.Do(func() {
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()
			if err != nil {
				t.status.SetText("Ошибка: " + err.Error())
				return
			}
			t.status.SetText("Обновлено: " + time.Now().Format("02.01 15:04"))
			t.render(periods)
		})
	}()
}

// fetch — собирает данные с внешних источников. Каждый сетевой вызов — со
// своим таймаутом (общий таймаут на цепочку вызовов даёт ложные обрывы,
// см. V2_NOTES.md).
func (t *weatherTab) fetch(daysBack int) ([]wjPeriod, error) {
	lat := t.st.SettingFloat(wjKeyLat, wjDefaultLat)
	lon := t.st.SettingFloat(wjKeyLon, wjDefaultLon)
	c := t.client

	var st forecast.Station
	{
		c1, cancel := ctx(45 * time.Second)
		s, _ := c.NearestStation(c1, lat, lon)
		cancel()
		st = s
	}

	var live, arch map[time.Time]*forecast.Hour
	var off int
	{
		c2, cancel := ctx(45 * time.Second)
		h, o, err := c.Live(c2, lat, lon)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("прогноз погоды: %w", err)
		}
		live, off = h, o
	}
	{
		c3, cancel := ctx(45 * time.Second)
		h, _ := c.Archive(c3, lat, lon, daysBack)
		cancel()
		arch = h
	}

	var sea, seaPrev map[time.Time]*forecast.Hour
	{
		c4, cancel := ctx(45 * time.Second)
		cur, prev, _ := c.MarineHours(c4, lat, lon, daysBack, 3)
		cancel()
		sea, seaPrev = cur, prev
	}

	var obs []forecast.Obs
	if st.ICAO != "" {
		c5, cancel := ctx(45 * time.Second)
		o, _ := c.METAR(c5, st.ICAO, (daysBack+1)*24)
		cancel()
		obs = o
	}

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
			fa := forecast.SummarizeFact(obs, seaList, s, e, loc)
			if fa.Obs > 0 || fa.HasSea {
				p.HasFact = true
				p.Fact = forecast.FactLines(fa)
				p.Station = fa.Station
				p.Verdict = forecast.Verify(f, fa, now)
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
	head := container.NewHBox(bold(p.Title), widget.NewLabel("  ["+p.Label+"]"))
	rows := []fyne.CanvasObject{head}
	for _, l := range p.Forecast {
		rows = append(rows, container.NewHBox(widget.NewLabelWithStyle(l.Label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), wrapLabel(l.Value)))
	}
	if p.HasFact {
		rows = append(rows, widget.NewSeparator())
		factHead := "Факт"
		if p.Station != "" {
			factHead += " (" + p.Station + ")"
		}
		rows = append(rows, bold(factHead+":"))
		for _, l := range p.Fact {
			rows = append(rows, container.NewHBox(widget.NewLabelWithStyle(l.Label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), wrapLabel(l.Value)))
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
	days := widget.NewSelect([]string{"1", "3", "7"}, nil)
	days.SetSelected(a.St.Setting(wjKeyDays, "3"))

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
