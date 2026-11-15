// Раздел «ИМ УНиО».
//
// ═══════════════════════════════════════════════════════════════════════════
//  СТРУКТУРА РАЗДЕЛА
// ═══════════════════════════════════════════════════════════════════════════
// Два выпадающих списка:
//
//   1. ТИП — НАВАРЕА / ПРИП / НАВИП (с русскими пояснениями).
//   2. ВЫПУСК — номер выпуска внутри выбранного типа, например «244/2026».
//      Номер берётся из самого текста извещения (см. IssueNumber в rus.go).
//
// Ниже — список извещений выбранного выпуска. Нажатие открывает полный текст.
//
// Все названия и заголовки по-русски: перевод делает пакет navwarn (rus.go).
// Тело сообщения остаётся на языке оригинала — машинный перевод морских
// текстов ненадёжен, а ошибка в координатах или времени опасна.
//
// ═══════════════════════════════════════════════════════════════════════════
//  ВАЖНО: НЕ УДАЛЯТЬ ЭТОТ ФАЙЛ
// ═══════════════════════════════════════════════════════════════════════════
// На него ссылаются App.warnings в app.go и уведомления о новых выпусках
// (notify.go, pushNewWarnings).
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

// warningsTab — состояние раздела «ИМ УНиО».
type warningsTab struct {
	a       *App
	content fyne.CanvasObject
	status  *widget.Label
	list    *widget.List

	// Полный набор, загруженный из базы.
	all []store.WarningRow

	// Отфильтрованный набор, который сейчас показан в списке.
	shown []store.WarningRow

	// Выпадающие списки.
	kindSel  *widget.Select
	issueSel *widget.Select

	// Текущее выделение.
	curKind  navwarn.Kind
	curIssue string

	filter *widget.Entry

	mu      sync.Mutex
	running bool
}

// allIssuesKey — пункт списка выпусков «показать все».
const allIssuesKey = "Все выпуски"

func newWarningsTab(a *App) *warningsTab {
	t := &warningsTab{a: a}
	t.status = wrapLabel("")

	t.list = widget.NewList(
		func() int { return len(t.shown) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			w := t.shown[i]
			o.(*widget.Label).SetText(warningLine(w))
		},
	)
	t.list.OnSelected = func(i widget.ListItemID) {
		t.showFull(t.shown[i])
		t.list.UnselectAll()
	}

	// ── Список 1: тип извещения ──
	t.kindSel = widget.NewSelect(nil, func(s string) {
		t.curKind = kindFromLabel(s)
		t.rebuildIssues()
	})
	t.kindSel.PlaceHolder = "Тип извещения"

	// ── Список 2: номер выпуска ──
	t.issueSel = widget.NewSelect(nil, func(s string) {
		t.curIssue = s
		t.applyFilter()
	})
	t.issueSel.PlaceHolder = "Выпуск"

	t.filter = widget.NewEntry()
	t.filter.SetPlaceHolder("Поиск: номер района, карты, слово…")
	t.filter.OnChanged = func(string) { t.applyFilter() }

	btn := widget.NewButton("Обновить", func() { t.refreshAsync(true) })
	btn.Importance = widget.HighImportance

	top := container.NewVBox(
		bold("Извещения мореплавателям"),
		t.kindSel,
		t.issueSel,
		container.NewBorder(nil, nil, nil, btn, t.filter),
		t.status,
		widget.NewSeparator(),
	)
	t.content = container.NewBorder(top, nil, nil, nil, t.list)

	t.reload()
	return t
}

// warningLine — строка списка: тип, выпуск, русское название района.
func warningLine(w store.WarningRow) string {
	k := navwarn.DetectKind(toWarning(w))
	issue := navwarn.IssueNumber(toWarning(w))

	area := navwarn.AreaRU(w.Area + " " + w.Title)
	if area == "" {
		area = w.Area
	}
	title := navwarn.TranslateTitle(w.Title)
	if title == w.Title && area != "" {
		title = "" // не дублируем английский заголовок
	}

	parts := []string{k.RU()}
	if issue != "" {
		parts = append(parts, "вып. "+issue)
	}
	parts = append(parts, "— "+area)
	if title != "" {
		parts = append(parts, "("+title+")")
	}
	return strings.Join(parts, " ")
}

// toWarning переводит строку базы в тип navwarn.Warning для классификации.
func toWarning(w store.WarningRow) navwarn.Warning {
	return navwarn.Warning{
		Area:   w.Area,
		Number: w.Number,
		Year:   w.Year,
		Title:  w.Title,
		Body:   w.Body,
		Region: w.Region,
		Source: w.Source,
	}
}

// showFull открывает полный текст извещения.
func (t *warningsTab) showFull(w store.WarningRow) {
	warn := toWarning(w)
	kind := navwarn.DetectKind(warn).RU()
	issue := navwarn.IssueNumber(warn)
	area := navwarn.AreaRU(w.Area + " " + w.Title)
	if area == "" {
		area = w.Area
	}

	header := bold(fmt.Sprintf("%s, выпуск %s", kind, issue))
	sub := wrapLabel(area)
	sub.Importance = widget.LowImportance

	// Тело — моноширинным шрифтом, как в оригинале: важны отступы и координаты.
	body := widget.NewLabelWithStyle(w.Body, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	body.Wrapping = fyne.TextWrapWord

	meta := wrapLabel("Источник: " + w.Source + ", загружено " +
		w.FetchedAt.Local().Format("02.01.2006 15:04"))

	sc := container.NewVScroll(container.NewVBox(header, sub, widget.NewSeparator(), body, meta))
	d := dialog.NewCustom(fmt.Sprintf("%s %s", kind, issue), "Закрыть", sc, t.a.Win)
	d.Resize(fyne.NewSize(400, 600))
	d.Show()
}

func (t *warningsTab) reload() {
	t.all, _ = t.a.St.Warnings()
	t.rebuildKinds()
}

// rebuildKinds пересобирает список типов по фактически имеющимся данным.
func (t *warningsTab) rebuildKinds() {
	groups := navwarn.GroupByKind(t.rowsToWarnings())

	var labels []string
	for _, g := range groups {
		labels = append(labels, g.TitleRU)
	}
	if len(labels) == 0 {
		labels = []string{
			"НАВАРЕА — океанские предупреждения",
			"ПРИП — прибрежные предупреждения",
			"НАВИП — навигационные извещения",
		}
	}
	t.kindSel.Options = labels
	t.kindSel.Refresh()

	// Сохраняем выбор, если он ещё существует.
	want := kindTitleRU(t.curKind)
	found := false
	for _, l := range labels {
		if l == want {
			found = true
			break
		}
	}
	if !found {
		t.curKind = navwarn.KindNavarea
		if len(groups) > 0 {
			t.curKind = groups[0].Kind
		}
		t.kindSel.SetSelected(kindTitleRU(t.curKind))
	} else {
		t.kindSel.SetSelected(want)
	}
	t.rebuildIssues()
}

// kindTitleRU возвращает русский заголовок группы для типа.
func kindTitleRU(k navwarn.Kind) string {
	switch k {
	case navwarn.KindNavarea:
		return "НАВАРЕА — океанские предупреждения"
	case navwarn.KindPrip:
		return "ПРИП — прибрежные предупреждения"
	case navwarn.KindNavip:
		return "НАВИП — навигационные извещения"
	}
	return string(k)
}

// kindFromLabel переводит выбранную подпись обратно в тип.
func kindFromLabel(label string) navwarn.Kind {
	switch {
	case strings.HasPrefix(label, "НАВАРЕА"):
		return navwarn.KindNavarea
	case strings.HasPrefix(label, "ПРИП"):
		return navwarn.KindPrip
	case strings.HasPrefix(label, "НАВИП"):
		return navwarn.KindNavip
	}
	return navwarn.KindNavarea
}

// rebuildIssues пересобирает список выпусков для выбранного типа.
func (t *warningsTab) rebuildIssues() {
	var opts []string
	seen := map[string]bool{}
	for _, w := range t.all {
		if navwarn.DetectKind(toWarning(w)) != t.curKind {
			continue
		}
		issue := navwarn.IssueNumber(toWarning(w))
		if issue == "" || seen[issue] {
			continue
		}
		seen[issue] = true
		opts = append(opts, issue)
	}
	// Новые выпуски сверху: номер года/серии сортируем по убыванию.
	sort.Slice(opts, func(i, j int) bool { return opts[i] > opts[j] })

	withAll := append([]string{allIssuesKey}, opts...)
	t.issueSel.Options = withAll
	t.issueSel.Refresh()

	if seen[t.curIssue] {
		t.issueSel.SetSelected(t.curIssue)
	} else {
		t.curIssue = allIssuesKey
		t.issueSel.SetSelected(allIssuesKey)
	}
	t.applyFilter()
}

// rowsToWarnings переводит строки базы в тип navwarn.Warning.
func (t *warningsTab) rowsToWarnings() []navwarn.Warning {
	out := make([]navwarn.Warning, 0, len(t.all))
	for _, w := range t.all {
		out = append(out, toWarning(w))
	}
	return out
}

// applyFilter отбирает извещения по типу, выпуску и строке поиска.
func (t *warningsTab) applyFilter() {
	q := lower(t.filter.Text)
	t.shown = t.shown[:0]
	for _, w := range t.all {
		wn := toWarning(w)
		if navwarn.DetectKind(wn) != t.curKind {
			continue
		}
		if t.curIssue != "" && t.curIssue != allIssuesKey &&
			navwarn.IssueNumber(wn) != t.curIssue {
			continue
		}
		if q != "" && !containsFold(w.Area+" "+w.Number+" "+w.Title+" "+w.Body, q) {
			continue
		}
		t.shown = append(t.shown, w)
	}
	// Свежие выпуски сверху.
	sort.SliceStable(t.shown, func(i, j int) bool {
		if t.shown[i].Year != t.shown[j].Year {
			return t.shown[i].Year > t.shown[j].Year
		}
		return t.shown[i].Number > t.shown[j].Number
	})
	t.list.Refresh()

	if len(t.shown) == 0 {
		t.status.SetText("Нет извещений выбранного типа. Нажмите «Обновить».")
	}
}

// refreshAsync загружает извещения. Без force — не чаще раза в 30 минут.
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
	t.status.SetText("Загрузка извещений…")

	go func() {
		c, cancel := ctx(60 * time.Second)
		defer cancel()
		warns, errs := navwarn.NewClient().FetchAll(c)
		now := time.Now()

		rows := make([]store.WarningRow, 0, len(warns))
		for _, w := range warns {
			if w.Expired(now) {
				continue
			}
			rows = append(rows, store.WarningRow{
				Area: w.Area, Number: w.Number, Year: w.Year,
				Title: w.Title, Body: w.Body, Source: w.Source,
				Region: w.Region, URL: w.URL,
			})
		}

		// Снимок «до» — чтобы понять, какие выпуски новые (для уведомления).
		before := map[string]bool{}
		if old, err := t.a.St.Warnings(); err == nil {
			for _, w := range old {
				before[w.Area+"/"+w.Number+"/"+fmt.Sprint(w.Year)] = true
			}
		}

		// Пустой ответ не затирает кэш (вероятно, сменилась вёрстка сайта).
		if len(rows) > 0 {
			_ = t.a.St.SaveWarnings("navwarn", rows)
			// Уведомление о новых выпусках (если включено в настройках).
			t.a.notifier.pushNewWarnings(before, rows)
		}

		msg := fmt.Sprintf("Получено: %d", len(rows))
		if len(errs) > 0 {
			msg += ". " + strings.Join(errs, "; ")
		}
		fyne.Do(func() {
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()
			t.reload()
			t.status.SetText(msg)
		})
	}()
}
