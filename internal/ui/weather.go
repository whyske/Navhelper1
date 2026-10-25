package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/weather"
)

// weatherTab — модуль 5: ежедневные реальные погодные данные.
type weatherTab struct {
	a       *App
	svc     *weather.Service
	content fyne.CanvasObject

	place  *widget.Label
	today  *widget.Label
	status *widget.Label
	list   *widget.List
	rows   []weather.Row

	mu      sync.Mutex
	running bool
}

func newWeatherTab(a *App) *weatherTab {
	t := &weatherTab{a: a, svc: weather.NewService(a.St)}
	t.place = bold("")
	t.today = wrapLabel("")
	t.today.TextStyle = fyne.TextStyle{Monospace: true}
	t.status = wrapLabel("")
	t.list = widget.NewList(
		func() int { return len(t.rows) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(rowShort(t.rows[i]))
		},
	)
	btn := widget.NewButton("Обновить сейчас", func() { t.syncAsync(true) })
	btn.Importance = widget.HighImportance

	top := container.NewVBox(t.place, t.today, btn, t.status, widget.NewSeparator(),
		widget.NewLabel("История (видимость / ветер / волнение):"))
	t.content = container.NewBorder(top, nil, nil, nil, t.list)
	t.reload()
	return t
}

// reload перечитывает историю из базы.
func (t *weatherTab) reload() {
	p := t.svc.Point()
	t.place.SetText(fmt.Sprintf("Погода: %s (%.4f, %.4f)", p.Place, p.Lat, p.Lon))
	hist, err := t.a.St.WeatherHistory(p.Lat, p.Lon, t.svc.HistoryDays())
	t.rows = t.rows[:0]
	if err == nil {
		for _, w := range hist {
			t.rows = append(t.rows, weather.ToRow(w))
		}
	}
	today := time.Now().UTC().Format("2006-01-02")
	if w, ok := t.a.St.WeatherForDay(today, p.Lat, p.Lon); ok {
		t.today.SetText(rowLong(weather.ToRow(w)))
	} else {
		t.today.SetText("За сегодня данных пока нет.")
	}
	t.list.Refresh()
}

// syncAsync запускает сбор в фоне (не чаще одного одновременно).
func (t *weatherTab) syncAsync(force bool) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	if !force && !t.svc.NeedSync() {
		t.mu.Unlock()
		t.status.SetText("Данные за сегодня уже сохранены. " + lastSync(t.a))
		return
	}
	t.running = true
	t.mu.Unlock()

	t.status.SetText("Загрузка данных Open-Meteo…")
	go func() {
		c, cancel := ctx(40 * time.Second)
		defer cancel()
		res, err := t.svc.Sync(c, true)
		fyne.Do(func() {
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()
			if err != nil {
				t.status.SetText("Нет связи: " + err.Error() + ". Показаны сохранённые данные.")
			} else {
				t.status.SetText("Готово: " + res.Message + ". " + lastSync(t.a))
			}
			t.reload()
		})
	}()
}

func lastSync(a *App) string {
	s := a.St.Setting(weather.KeyLastSync, "")
	if tm, err := time.Parse(time.RFC3339, s); err == nil {
		return "Последнее обновление: " + tm.Local().Format("02.01 15:04")
	}
	return ""
}

func rowLong(r weather.Row) string {
	s := "Сегодня (" + r.Day + "):\n"
	if r.HasVis {
		s += fmt.Sprintf("Видимость: %s каб\n", r.Cables)
	} else {
		s += "Видимость: нет данных\n"
	}
	if r.HasWind {
		s += fmt.Sprintf("Ветер: %d балл. Бофорта — %s (%.1f м/с)\n", r.Beaufort, r.BeaufortName, r.WindMS)
	} else {
		s += "Ветер: нет данных\n"
	}
	if r.HasWave {
		s += fmt.Sprintf("Волнение: %d балл. — %s (h = %.2f м)", r.WaveBall, r.WaveName, r.WaveM)
	} else {
		s += "Волнение: нет данных (точка на суше?)"
	}
	return s
}

func rowShort(r weather.Row) string {
	d, err := time.Parse("2006-01-02", r.Day)
	day := r.Day
	if err == nil {
		day = d.Format("02.01")
	}
	vis, wind, wave := "—", "—", "—"
	if r.HasVis {
		vis = r.Cables + " каб"
	}
	if r.HasWind {
		wind = fmt.Sprintf("%d Бф", r.Beaufort)
	}
	if r.HasWave {
		wave = fmt.Sprintf("%d балл. (%.1f м)", r.WaveBall, r.WaveM)
	}
	return fmt.Sprintf("%s   %s   %s   %s", day, vis, wind, wave)
}
