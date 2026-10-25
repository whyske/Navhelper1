package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/navwarn"
	"ru.navhelper/internal/store"
)

// warningsTab — модуль 3: навигационные предупреждения (НАВАРЕА, ПРИП, НАВИП).
type warningsTab struct {
	a       *App
	content fyne.CanvasObject
	status  *widget.Label
	list    *widget.List
	all     []store.WarningRow
	shown   []store.WarningRow
	filter  *widget.Entry

	mu      sync.Mutex
	running bool
}

func newWarningsTab(a *App) *warningsTab {
	t := &warningsTab{a: a}
	t.status = wrapLabel("")
	t.list = widget.NewList(
		func() int { return len(t.shown) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			w := t.shown[i]
			o.(*widget.Label).SetText(fmt.Sprintf("%s %s/%02d — %s", w.Area, w.Number, w.Year%100, w.Title))
		},
	)
	t.list.OnSelected = func(i widget.ListItemID) {
		w := t.shown[i]
		body := widget.NewLabelWithStyle(w.Body, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		body.Wrapping = fyne.TextWrapWord
		sc := container.NewVScroll(container.NewVBox(body,
			widget.NewLabel("Источник: "+w.Source+", загружено "+w.FetchedAt.Local().Format("02.01.2006 15:04"))))
		d := dialog.NewCustom(fmt.Sprintf("%s %s/%02d", w.Area, w.Number, w.Year%100), "Закрыть", sc, t.a.Win)
		d.Resize(fyne.NewSize(400, 600))
		d.Show()
		t.list.UnselectAll()
	}

	t.filter = widget.NewEntry()
	t.filter.SetPlaceHolder("Фильтр: район, номер карты, слово…")
	t.filter.OnChanged = func(string) { t.applyFilter() }

	btn := widget.NewButton("Обновить", func() { t.refreshAsync(true) })
	btn.Importance = widget.HighImportance

	top := container.NewVBox(bold("Действующие навигационные предупреждения"),
		container.NewBorder(nil, nil, nil, btn, t.filter), t.status, widget.NewSeparator())
	t.content = container.NewBorder(top, nil, nil, nil, t.list)
	t.reload()
	return t
}

func (t *warningsTab) reload() {
	t.all, _ = t.a.St.Warnings()
	t.applyFilter()
}

func (t *warningsTab) applyFilter() {
	q := lower(t.filter.Text)
	t.shown = t.shown[:0]
	for _, w := range t.all {
		if q == "" || containsFold(w.Area+" "+w.Number+" "+w.Title+" "+w.Body, q) {
			t.shown = append(t.shown, w)
		}
	}
	t.list.Refresh()
}

// refreshAsync загружает предупреждения из всех источников. Без force —
// не чаще раза в 30 минут.
func (t *warningsTab) refreshAsync(force bool) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	last := t.a.St.LastWarningFetch()
	if !force && !last.IsZero() && time.Since(last) < 30*time.Minute {
		t.mu.Unlock()
		t.status.SetText(fmt.Sprintf("В кэше: %d. Обновлено %s", len(t.all), last.Local().Format("02.01 15:04")))
		return
	}
	t.running = true
	t.mu.Unlock()
	t.status.SetText("Загрузка предупреждений…")

	go func() {
		c, cancel := ctx(45 * time.Second)
		defer cancel()
		results := navwarn.NewClient().FetchAll(c, navwarn.DefaultSources)
		now := time.Now()
		msg := ""
		for _, r := range results {
			if r.Err != nil {
				msg += r.Source.Name + ": нет связи. "
				continue
			}
			rows := make([]store.WarningRow, 0, len(r.Warnings))
			for _, w := range r.Warnings {
				if w.Expired(now) {
					continue
				}
				rows = append(rows, store.WarningRow{Area: w.Area, Number: w.Number, Year: w.Year,
					Title: w.Title, Body: w.Body, Source: r.Source.Name})
			}
			// Пустой ответ не затирает кэш (вероятно, сменилась вёрстка сайта).
			if len(rows) > 0 {
				_ = t.a.St.SaveWarnings(r.Source.Name, rows)
			}
			msg += fmt.Sprintf("%s: %d. ", r.Source.Name, len(rows))
		}
		fyne.Do(func() {
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()
			t.reload()
			t.status.SetText(msg + "Всего в кэше: " + fmt.Sprint(len(t.all)))
		})
	}()
}
