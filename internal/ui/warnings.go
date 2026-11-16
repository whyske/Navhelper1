// Раздел «ИМ УНиО» — извещения мореплавателям.
//
// ═══════════════════════════════════════════════════════════════════════════
//  СТРУКТУРА (как просил пользователь)
// ═══════════════════════════════════════════════════════════════════════════
//
//  1. Выпадающий список РАЙОНОВ (бассейны: Чёрное/Азовское, Балтика, Арктика…).
//  2. Под ним — СПИСОК ВЫПУСКОВ этого района в виде сворачиваемых карточек.
//  3. Нажатие на выпуск разворачивает его содержимое: группы ПРИП / НАВИП /
//     НАВАРЕА (показываются только те типы, что реально есть в базе).
//  4. Нажатие на тип ОТКРЫВАЕТ ПОЛНЫЙ ТЕКСТ извещения с кнопкой «Скачать PDF».
//  5. Кнопка «Скачать PDF» на карточке выпуска сохраняет весь выпуск.
//
// ═══════════════════════════════════════════════════════════════════════════
//  ПОЧЕМУ КАРТОЧКИ, А НЕ widget.List
// ═══════════════════════════════════════════════════════════════════════════
// Раньше список строился через widget.List с многострочными подписями.
// У widget.List высота строки фиксирована, поэтому длинные названия районов
// (например «НАВАРЕА вып. 99/2026 — РАЙОН XIII — Северо-западная Пацифика
// (Россия)») залезали на соседние строки — именно это было видно на
// скриншоте. Теперь каждая запись — карточка в VBox: высота считается
// естественно, наложений быть не может.
//
// ═══════════════════════════════════════════════════════════════════════════
//  ЯЗЫК ТЕКСТА
// ═══════════════════════════════════════════════════════════════════════════
// Заголовки и навигация — по-русски. Сам ТЕКСТ извещения выводится на языке
// первоисточника (английский у НАВАРЕА/прибрежных служб, русский у ГУНиО):
// машинный перевод координат, времени и названий объектов недопустим.
//
// ═══════════════════════════════════════════════════════════════════════════
//  НЕ УДАЛЯТЬ ФАЙЛ
// ═══════════════════════════════════════════════════════════════════════════
// На него ссылаются App.warnings (app.go) и уведомления о новых выпусках
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
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/navwarn"
	"ru.navhelper/internal/store"
)

// warningsTab — состояние раздела «ИМ УНиО».
type warningsTab struct {
	a       *App
	content fyne.CanvasObject
	status  *widget.Label

	// Полный набор из базы.
	all []store.WarningRow

	// Текущий выбранный бассейн (ключ из navwarn.Regions, "" — все).
	curRegion string

	regionSel *widget.Select
	issuesBox *fyne.Container
	filter    *widget.Entry

	mu      sync.Mutex
	running bool
}

const allRegionsKey = "Все районы"

func newWarningsTab(a *App) *warningsTab {
	t := &warningsTab{a: a}
	t.status = wrapLabel("")

	// ── ВАЖНО: порядок создания объектов критичен ──
	// SetSelected у Select СРАЗУ вызывает обработчик OnChanged. Раньше
	// обработчик обращался к t.issuesBox и t.filter, которых ещё не было,
	// и приложение падало с nil pointer уже на запуске.
	// Поэтому сначала создаём всё, что нужно обработчикам, и только потом
	// задаём выбранное значение.
	t.issuesBox = container.NewVBox()
	t.filter = widget.NewEntry()
	t.filter.SetPlaceHolder("Поиск: номер выпуска, карты, слово…")
	t.filter.OnChanged = func(string) { t.rebuildIssues() }

	// ── 1. Районы ──
	names := []string{allRegionsKey}
	for _, r := range navwarn.Regions {
		names = append(names, r.Name)
	}
	t.regionSel = widget.NewSelect(names, func(s string) {
		t.curRegion = regionKeyFor(s)
		_ = a.St.SetSetting("im.region", s)
		t.rebuildIssues()
	})

	sel := a.St.Setting("im.region", allRegionsKey)
	ok := false
	for _, n := range names {
		if n == sel {
			ok = true
			break
		}
	}
	if !ok {
		sel = allRegionsKey
	}
	t.curRegion = regionKeyFor(sel)
	// Теперь обработчик безопасен: issuesBox и filter уже существуют.
	t.regionSel.SetSelected(sel)

	btn := widget.NewButtonWithIcon("Обновить", theme.ViewRefreshIcon(), func() { t.refreshAsync(true) })
	btn.Importance = widget.HighImportance

	scroll := container.NewVScroll(t.issuesBox)

	top := container.NewVBox(
		bold("Извещения мореплавателям"),
		t.regionSel,
		container.NewBorder(nil, nil, nil, btn, t.filter),
		t.status,
		widget.NewSeparator(),
	)
	t.content = container.NewBorder(top, nil, nil, nil, scroll)

	t.reload()
	return t
}

// regionKeyFor переводит название бассейна в ключ региона.
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

func (t *warningsTab) reload() {
	t.all, _ = t.a.St.Warnings()
	t.rebuildIssues()
}

// toWarning переводит строку базы в тип navwarn.Warning.
func toWarning(w store.WarningRow) navwarn.Warning {
	return navwarn.Warning{
		Area: w.Area, Number: w.Number, Year: w.Year,
		Title: w.Title, Body: w.Body, Region: w.Region, Source: w.Source,
	}
}

// issue — один выпуск: все его извещения, сгруппированные по типу.
type issue struct {
	Number   string // «244/2026»
	Messages []store.WarningRow
}

// rebuildIssues строит список выпусков выбранного района.
func (t *warningsTab) rebuildIssues() {
	// Страховка: обработчики Select/Entry могут сработать раньше, чем
	// поля созданы. Молча выходим вместо падения (см. комментарий в
	// newWarningsTab о порядке инициализации).
	if t.issuesBox == nil || t.filter == nil || t.regionSel == nil {
		return
	}
	t.issuesBox.Objects = nil

	q := lower(t.filter.Text)

	// Собираем выпуски: ключ = номер/год. В выпуск попадают все извещения
	// этого номера, независимо от типа (НАВАРЕА/ПРИП/НАВИП).
	byIssue := map[string]*issue{}
	for _, w := range t.all {
		if t.curRegion != "" && w.Region != t.curRegion {
			continue
		}
		if q != "" && !containsFold(w.Area+" "+w.Number+" "+w.Title+" "+w.Body, q) {
			continue
		}
		wn := toWarning(w)
		num := navwarn.IssueNumber(wn)
		if num == "" {
			num = w.Number
		}
		if num == "" {
			continue
		}
		it, ok := byIssue[num]
		if !ok {
			it = &issue{Number: num}
			byIssue[num] = it
		}
		it.Messages = append(it.Messages, w)
	}

	// Сортировка: свежие выпуски сверху. Сравниваем номер как число, а при
	// равенстве — год, чтобы «99/2026» не оказывался выше «950/2026».
	list := make([]*issue, 0, len(byIssue))
	for _, it := range byIssue {
		list = append(list, it)
	}
	sort.SliceStable(list, func(i, j int) bool {
		ni, yi := splitIssue(list[i].Number)
		nj, yj := splitIssue(list[j].Number)
		if yi != yj {
			return yi > yj
		}
		return ni > nj
	})

	if len(list) == 0 {
		t.status.SetText("Нет извещений по выбранному району.")
		t.issuesBox.Refresh()
		return
	}

	for _, it := range list {
		t.issuesBox.Add(t.issueCard(it))
	}
	t.status.SetText(fmt.Sprintf("Выпусков: %d, извещений: %d", len(list), len(t.all)))
	t.issuesBox.Refresh()
}

// splitIssue разбирает «244/2026» на число и год.
func splitIssue(s string) (num, year int) {
	parts := strings.SplitN(s, "/", 2)
	fmt.Sscanf(parts[0], "%d", &num)
	if len(parts) == 2 {
		fmt.Sscanf(parts[1], "%d", &year)
		if year < 100 {
			year += 2000
		}
	}
	return
}

// issueCard — сворачиваемая карточка выпуска.
//
// Свёрнутая: «Выпуск 244/2026 · РАЙОН III …» и стрелка.
// Развёрнутая: строки типов (НАВАРЕА / ПРИП / НАВИП) и кнопка «Скачать PDF».
func (t *warningsTab) issueCard(it *issue) fyne.CanvasObject {
	// Заголовок: район первого извещения выпуска.
	area := ""
	if len(it.Messages) > 0 {
		area = navwarn.AreaRU(it.Messages[0].Area + " " + it.Messages[0].Title)
		if area == "" {
			area = it.Messages[0].Area
		}
	}

	head := bold(fmt.Sprintf("Выпуск %s", it.Number))
	sub := wrapLabel(area)
	sub.Importance = widget.LowImportance

	chevron := widget.NewLabelWithStyle("▸", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})

	// ── Содержимое: типы, которые реально есть в этом выпуске ──
	details := container.NewVBox()
	groups := navwarn.GroupByKind(rowsToWarnings(it.Messages))
	for _, g := range groups {
		g := g
		row := widget.NewButton(g.TitleRU, func() {
			t.openMessage(g.Warnings[0], it.Number)
		})
		row.Alignment = widget.ButtonAlignLeading
		details.Add(row)
	}
	if len(groups) == 0 {
		details.Add(wrapLabel("Нет данных"))
	}

	// Кнопка «Скачать PDF» — сохраняет ВЕСЬ выпуск.
	pdfBtn := widget.NewButtonWithIcon("Скачать PDF", theme.DownloadIcon(), func() {
		t.saveIssuePDF(it)
	})
	pdfBtn.Importance = widget.LowImportance
	details.Add(widget.NewSeparator())
	details.Add(pdfBtn)

	details.Hide()

	toggle := widget.NewButton("", func() {
		if details.Visible() {
			details.Hide()
			chevron.SetText("▸")
		} else {
			details.Show()
			chevron.SetText("▾")
		}
	})
	toggle.Importance = widget.LowImportance

	titleRow := container.NewBorder(nil, nil, nil, chevron, container.NewVBox(head, sub))
	inner := container.NewVBox(container.NewStack(titleRow, toggle), details)

	bg := newCardBG()
	return container.NewPadded(container.NewStack(bg, container.NewPadded(inner)))
}

// rowsToWarnings переводит строки базы в navwarn.Warning.
func rowsToWarnings(rows []store.WarningRow) []navwarn.Warning {
	out := make([]navwarn.Warning, 0, len(rows))
	for _, w := range rows {
		out = append(out, toWarning(w))
	}
	return out
}

// openMessage показывает полный текст извещения.
func (t *warningsTab) openMessage(w navwarn.Warning, issueNum string) {
	kind := navwarn.DetectKind(w).RU()
	area := navwarn.AreaRU(w.Area + " " + w.Title)
	if area == "" {
		area = w.Area
	}

	head := bold(fmt.Sprintf("%s, выпуск %s", kind, issueNum))
	areaLbl := wrapLabel(area)
	areaLbl.Importance = widget.LowImportance

	// Блок под заголовком: район и, если удалось, перевод заголовка по-русски.
	subBox := container.NewVBox(areaLbl)
	if tr := navwarn.TranslateTitle(w.Title); tr != w.Title {
		trLbl := wrapLabel(tr)
		trLbl.TextStyle = fyne.TextStyle{Italic: true}
		subBox.Add(trLbl)
	}

	// Тело — моноширинно и без перевода: важны отступы и координаты.
	body := widget.NewLabelWithStyle(w.Body, fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	body.Wrapping = fyne.TextWrapWord

	meta := wrapLabel("Источник: " + w.Source)
	meta.Importance = widget.LowImportance

	dl := widget.NewButtonWithIcon("Скачать PDF", theme.DownloadIcon(), func() {
		t.saveMessagePDF(w, issueNum)
	})
	dl.Importance = widget.HighImportance

	content := container.NewVScroll(container.NewVBox(
		head, subBox, widget.NewSeparator(), body, widget.NewSeparator(), meta, dl))
	d := dialog.NewCustom(fmt.Sprintf("%s %s", kind, issueNum), "Закрыть", content, t.a.Win)
	d.Resize(fyne.NewSize(400, 600))
	d.Show()
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

		// Снимок «до» — чтобы определить, какие выпуски новые (для уведомления).
		before := map[string]bool{}
		if old, err := t.a.St.Warnings(); err == nil {
			for _, w := range old {
				before[w.Area+"/"+w.Number+"/"+fmt.Sprint(w.Year)] = true
			}
		}

		// Пустой ответ не затирает кэш (вероятно, сменилась вёрстка сайта).
		if len(rows) > 0 {
			_ = t.a.St.SaveWarnings("navwarn", rows)
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
