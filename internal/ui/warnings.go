package ui

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/navwarn"
	"ru.navhelper/internal/store"
)

// warningsTab — раздел «ИМ УНиО»: навигационные предупреждения
// (НАВАРЕА, ПРИП, НАВИП) по регионам, с сохранением в локальную базу.
type warningsTab struct {
	a       *App
	content fyne.CanvasObject
	status  *widget.Label
	list    *widget.List
	region  *widget.Select

	all    []store.WarningRow
	shown  []store.WarningRow
	filter *widget.Entry

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
			line := fmt.Sprintf("%s %s/%02d — %s", w.Area, w.Number, w.Year%100, w.Title)
			o.(*widget.Label).SetText(line)
		},
	)
	t.list.OnSelected = func(i widget.ListItemID) {
		w := t.shown[i]
		body := widget.NewLabelWithStyle(w.Body, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		body.Wrapping = fyne.TextWrapWord
		footer := wrapLabel("Источник: " + w.Source + ", загружено " +
			w.FetchedAt.Local().Format("02.01.2006 15:04"))
		footer.Importance = widget.LowImportance
		sc := container.NewVScroll(container.NewVBox(body, widget.NewSeparator(), footer))
		d := dialog.NewCustom(fmt.Sprintf("%s %s/%02d", w.Area, w.Number, w.Year%100), "Закрыть", sc, t.a.Win)
		d.Resize(fyne.NewSize(400, 600))
		d.Show()
		t.list.UnselectAll()
	}

	// Выбор региона: раньше приложение искало только Арктику, теперь — все
	// основные бассейны, где ходят российские суда.
	regionNames := []string{"Все регионы"}
	for _, r := range navwarn.Regions {
		regionNames = append(regionNames, r.Name)
	}
	t.region = widget.NewSelect(regionNames, func(string) { t.applyFilter() })
	t.region.SetSelected(regionNames[0])

	t.filter = widget.NewEntry()
	t.filter.SetPlaceHolder("Фильтр: район, номер карты, слово…")
	t.filter.OnChanged = func(string) { t.applyFilter() }

	btn := widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), func() { t.refreshAsync(true) })
	btn.Importance = widget.HighImportance

	top := container.NewVBox(
		bold("Извещения мореплавателям и навигационные предупреждения"),
		container.NewBorder(nil, nil, widget.NewLabel("Регион:"), nil, t.region),
		container.NewBorder(nil, nil, nil, btn, t.filter),
		t.status,
		widget.NewSeparator(),
	)
	t.content = container.NewBorder(top, nil, nil, nil, t.list)
	t.reload()
	return t
}

func (t *warningsTab) reload() {
	t.all, _ = t.a.St.Warnings()
	t.applyFilter()
}

// regionKeyByName возвращает ключ региона по его отображаемому имени.
func regionKeyByName(name string) string {
	for _, r := range navwarn.Regions {
		if r.Name == name {
			return r.Key
		}
	}
	return ""
}

func (t *warningsTab) applyFilter() {
	q := lower(t.filter.Text)
	key := regionKeyByName(t.region.Selected)
	t.shown = t.shown[:0]
	for _, w := range t.all {
		if key != "" && w.Region != key {
			continue
		}
		if q == "" || containsFold(w.Area+" "+w.Number+" "+w.Title+" "+w.Body, q) {
			t.shown = append(t.shown, w)
		}
	}
	sort.SliceStable(t.shown, func(i, j int) bool {
		if t.shown[i].Area != t.shown[j].Area {
			return t.shown[i].Area < t.shown[j].Area
		}
		return t.shown[i].Number > t.shown[j].Number
	})
	t.list.Refresh()
}

// refreshAsync загружает предупреждения из всех источников.
// Без force — не чаще раза в 30 минут.
func (t *warningsTab) refreshAsync(force bool) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	last := t.a.St.LastWarningFetch()
	if !force && !last.IsZero() && time.Since(last) < 30*time.Minute {
		t.mu.Unlock()
		t.status.SetText(fmt.Sprintf("В кэше: %d. Обновлено %s",
			len(t.all), last.Local().Format("02.01 15:04")))
		return
	}
	t.running = true
	t.mu.Unlock()
	t.status.SetText("Загрузка предупреждений…")

	go func() {
		c, cancel := ctx(60 * time.Second)
		defer cancel()
		warnings, errs := navwarn.NewClient().FetchAll(c)
		now := time.Now()

		rows := make([]store.WarningRow, 0, len(warnings))
		for _, w := range warnings {
			if w.Expired(now) {
				continue
			}
			rows = append(rows, store.WarningRow{
				Area: w.Area, Number: w.Number, Year: w.Year,
				Title: w.Title, Body: w.Body, Source: w.Source,
				Region: w.Region, URL: w.URL,
			})
		}
		// Пустой ответ не затирает кэш: вероятно, изменилась вёрстка источника.
		if len(rows) > 0 {
			_ = t.a.St.SaveWarnings("navwarn", rows)
		}
		msg := fmt.Sprintf("Загружено: %d. ", len(rows))
		if len(errs) > 0 {
			msg += "Не всё получено: " + fmt.Sprint(len(errs)) + " ист. "
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
