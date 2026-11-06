package ui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/navwarn"
	"ru.navhelper/internal/store"
)

// warningsTab — раздел «ИМ УНиО»: навигационные предупреждения (НАВАРЕА,
// ПРИП, НАВИП) по регионам. Раньше искалась только Арктика, теперь выбирается
// бассейн (Чёрное/Азовское, Балтика и Финский залив, Арктика, Северная
// Атлантика, Средиземное, Дальний Восток, Каспий), а фильтр принимает номер
// района или карты и оставляет только подходящие выпуски.
type warningsTab struct {
	a       *App
	content fyne.CanvasObject
	status  *widget.Label
	list    *widget.List
	all     []store.WarningRow
	shown   []store.WarningRow
	filter  *widget.Entry
	region  *widget.Select

	mu      sync.Mutex
	running bool
}

// allRegionsKey — пункт выпадающего списка «все регионы».
const allRegionsKey = "Все районы"

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
	t.filter.SetPlaceHolder("Номер района / карты: III, 12345, Кронштадт…")
	t.filter.OnChanged = func(string) { t.applyFilter() }

	// Выбор бассейна: пустое значение — все регионы.
	names := []string{allRegionsKey}
	for _, r := range navwarn.Regions {
		names = append(names, r.Name)
	}
	t.region = widget.NewSelect(names, func(string) { t.applyFilter() })
	t.region.SetSelected(a.St.Setting("im.region", allRegionsKey))
	t.region.OnChanged = func(s string) {
		_ = a.St.SetSetting("im.region", s)
		t.applyFilter()
	}

	btn := widget.NewButton("Обновить", func() { t.refreshAsync(true) })
	btn.Importance = widget.HighImportance

	top := container.NewVBox(
		bold("Извещения мореплавателям и предупреждения"),
		t.region,
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

// regionKeyFor переводит выбранное название бассейна в ключ региона.
func regionKeyFor(name string) string {
	if name == "" || name == allRegionsKey {
		return ""
	}
	for _, r := range navwarn.Regions {
		if r.Name == name {
			return r.Key
		}
	}
	return ""
}

// applyFilter отбирает выпуски по выбранному бассейну и строке поиска.
func (t *warningsTab) applyFilter() {
	q := lower(t.filter.Text)
	rk := regionKeyFor(t.region.Selected)
	t.shown = t.shown[:0]
	for _, w := range t.all {
		if rk != "" && w.Region != rk {
			continue
		}
		if q != "" && !containsFold(w.Area+" "+w.Number+" "+w.Title+" "+w.Body, q) {
			continue
		}
		t.shown = append(t.shown, w)
	}
	sort.SliceStable(t.shown, func(i, j int) bool {
		if t.shown[i].Area != t.shown[j].Area {
			return t.shown[i].Area < t.shown[j].Area
		}
		return t.shown[i].Number < t.shown[j].Number
	})
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
	t.status.SetText("Загрузка извещений…")

	go func() {
		c, cancel := ctx(60 * time.Second)
		defer cancel()
		warns, errs := navwarn.NewClient().FetchAll(c)
		now := time.Now()

		rows := make([]store.WarningRow, 0, len(warns))
		bySource := map[string]int{}
		for _, w := range warns {
			if w.Expired(now) {
				continue
			}
			rows = append(rows, store.WarningRow{
				Area: w.Area, Number: w.Number, Year: w.Year,
				Title: w.Title, Body: w.Body, Source: w.Source,
				Region: w.Region,
			})
			bySource[w.Source]++
		}
		// Пустой ответ не затирает кэш (вероятно, сменилась вёрстка сайта).
		if len(rows) > 0 {
			_ = t.a.St.SaveWarnings("navwarn", rows)
		}

		msg := fmt.Sprintf("Получено: %d. ", len(rows))
		if len(errs) > 0 {
			msg += strings.Join(errs, "; ") + " "
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
