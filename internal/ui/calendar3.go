package ui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/store"
)

// ─────────────────────────── вспомогательное ───────────────────────────

var monthsRU = []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь",
	"Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}

var weekdaysShort = []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

var weekdaysFull = []string{"Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье"}

// weekdaysEvery — «каждый понедельник», «каждую среду»…
var weekdaysEvery = []string{"каждый понедельник", "каждый вторник", "каждую среду",
	"каждый четверг", "каждую пятницу", "каждую субботу", "каждое воскресенье"}

func parseHex(s string) color.NRGBA {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{0x1e, 0x88, 0xe5, 0xff}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{0x1e, 0x88, 0xe5, 0xff}
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}
}

func hexOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

// ─────────────────────────── выбор цвета ───────────────────────────

// colorPicker — ряд цветных кружков из палитры + «Другой…» (полный выбор цвета).
type colorPicker struct {
	widget.BaseWidget
	value    string
	win      fyne.Window
	box      *fyne.Container
	onChange func(string)
}

func newColorPicker(win fyne.Window, initial string) *colorPicker {
	if initial == "" {
		initial = store.DefaultNoteColor
	}
	p := &colorPicker{value: strings.ToUpper(initial), win: win}
	p.ExtendBaseWidget(p)
	p.box = container.NewGridWithColumns(5)
	p.rebuild()
	return p
}

func (p *colorPicker) rebuild() {
	p.box.Objects = nil
	inPalette := false
	for _, hx := range store.NoteColors {
		hx := hx
		sel := strings.EqualFold(hx, p.value)
		if sel {
			inPalette = true
		}
		p.box.Add(newSwatch(parseHex(hx), sel, func() { p.set(hx) }))
	}
	custom := "#FFFFFF"
	if !inPalette {
		custom = p.value
	}
	other := newSwatch(parseHex(custom), !inPalette, func() {
		d := dialog.NewColorPicker("Свой цвет", "Выберите цвет заметки", func(c color.Color) {
			p.set(hexOf(c))
		}, p.win)
		d.Advanced = true
		d.SetColor(parseHex(p.value))
		d.Show()
	})
	other.label = "…"
	p.box.Add(other)
	p.box.Refresh()
}

func (p *colorPicker) set(hx string) {
	p.value = strings.ToUpper(hx)
	p.rebuild()
	if p.onChange != nil {
		p.onChange(p.value)
	}
}

func (p *colorPicker) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(p.box)
}

// swatch — круглая кнопка цвета.
type swatch struct {
	widget.BaseWidget
	col      color.Color
	selected bool
	label    string
	tap      func()
}

func newSwatch(c color.Color, selected bool, tap func()) *swatch {
	s := &swatch{col: c, selected: selected, tap: tap}
	s.ExtendBaseWidget(s)
	return s
}

func (s *swatch) Tapped(*fyne.PointEvent) {
	if s.tap != nil {
		s.tap()
	}
}

func (s *swatch) MinSize() fyne.Size { return fyne.NewSize(40, 40) }

func (s *swatch) CreateRenderer() fyne.WidgetRenderer {
	circle := canvas.NewCircle(s.col)
	circle.StrokeColor = color.NRGBA{0xb0, 0xbc, 0xc8, 0xff}
	circle.StrokeWidth = 1
	if s.selected {
		circle.StrokeColor = color.NRGBA{0x1b, 0x26, 0x33, 0xff}
		circle.StrokeWidth = 3
	}
	objs := []fyne.CanvasObject{container.NewPadded(circle)}
	if s.label != "" {
		t := canvas.NewText(s.label, color.NRGBA{0x1b, 0x26, 0x33, 0xff})
		t.TextStyle = fyne.TextStyle{Bold: true}
		objs = append(objs, container.NewCenter(t))
	} else if s.selected {
		t := canvas.NewText("✓", color.White)
		t.TextStyle = fyne.TextStyle{Bold: true}
		objs = append(objs, container.NewCenter(t))
	}
	return widget.NewSimpleRenderer(container.NewStack(objs...))
}

// ─────────────────────────── сетка месяца ───────────────────────────

// monthGrid — календарь месяца с цветными отметками заметок в каждой клетке.
// Стандартный widget.Calendar Fyne не умеет раскрашивать дни, поэтому свой.
type monthGrid struct {
	a        *App
	month    time.Time // первое число показываемого месяца
	selected time.Time
	onSelect func(time.Time)

	title   *widget.Label
	grid    *fyne.Container
	content fyne.CanvasObject
}

func newMonthGrid(a *App, sel time.Time, onSelect func(time.Time)) *monthGrid {
	m := &monthGrid{a: a, selected: dateOnly(sel), onSelect: onSelect}
	m.month = time.Date(sel.Year(), sel.Month(), 1, 0, 0, 0, 0, time.Local)
	m.title = widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	prev := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), func() { m.shift(-1) })
	next := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() { m.shift(1) })
	today := widget.NewButton("Сегодня", func() { m.SetSelected(time.Now()); m.onSelect(m.selected) })
	today.Importance = widget.LowImportance
	head := container.NewBorder(nil, nil, prev, container.NewHBox(today, next), m.title)

	wd := container.NewGridWithColumns(7)
	for i, d := range weekdaysShort {
		l := widget.NewLabelWithStyle(d, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		if i >= 5 {
			l.Importance = widget.DangerImportance
		}
		wd.Add(l)
	}
	m.grid = container.NewGridWithColumns(7)
	m.content = container.NewVBox(head, wd, m.grid)
	m.Refresh()
	return m
}

func (m *monthGrid) shift(n int) {
	m.month = m.month.AddDate(0, n, 0)
	m.Refresh()
}

// SetSelected выбирает день и показывает его месяц.
func (m *monthGrid) SetSelected(t time.Time) {
	m.selected = dateOnly(t)
	m.month = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
	m.Refresh()
}

// Refresh перерисовывает сетку и цветные отметки.
func (m *monthGrid) Refresh() {
	m.title.SetText(fmt.Sprintf("%s %d", monthsRU[m.month.Month()-1], m.month.Year()))
	start := m.month.AddDate(0, 0, -(store.ISOWeekday(m.month) - 1))
	end := start.AddDate(0, 0, 41)
	notes, _ := m.a.St.NotesRange(start, end)
	today := dateOnly(time.Now())

	m.grid.Objects = nil
	for i := 0; i < 42; i++ {
		d := start.AddDate(0, 0, i)
		if i == 35 && d.Month() != m.month.Month() {
			break // шестая строка не нужна
		}
		var cols []color.Color
		for _, n := range notes[d.Format("2006-01-02")] {
			cols = append(cols, parseHex(n.Color))
		}
		day := d
		m.grid.Add(newDayCell(d, d.Month() == m.month.Month(), d.Equal(m.selected), d.Equal(today), cols,
			func() {
				m.selected = day
				if day.Month() != m.month.Month() {
					m.month = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.Local)
				}
				m.Refresh()
				m.onSelect(day)
			}))
	}
	m.grid.Refresh()
}

// dayCell — клетка дня: число и до 4 цветных полосок заметок.
type dayCell struct {
	widget.BaseWidget
	day                 time.Time
	inMonth, sel, today bool
	colors              []color.Color
	tap                 func()
}

func newDayCell(d time.Time, inMonth, sel, today bool, cols []color.Color, tap func()) *dayCell {
	c := &dayCell{day: d, inMonth: inMonth, sel: sel, today: today, colors: cols, tap: tap}
	c.ExtendBaseWidget(c)
	return c
}

func (c *dayCell) Tapped(*fyne.PointEvent) { c.tap() }

func (c *dayCell) MinSize() fyne.Size { return fyne.NewSize(40, 50) }

func (c *dayCell) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 8
	fg := color.Color(color.NRGBA{0x1b, 0x26, 0x33, 0xff})
	if wd := store.ISOWeekday(c.day); wd >= 6 {
		fg = color.NRGBA{0xc0, 0x30, 0x20, 0xff}
	}
	if !c.inMonth {
		fg = color.NRGBA{0xa8, 0xb2, 0xbd, 0xff}
	}
	switch {
	case c.sel:
		bg.FillColor = color.NRGBA{0x1f, 0x5f, 0x9e, 0xff}
		fg = color.White
	case c.today:
		bg.StrokeColor = color.NRGBA{0x1f, 0x5f, 0x9e, 0xff}
		bg.StrokeWidth = 2
	}
	num := canvas.NewText(strconv.Itoa(c.day.Day()), fg)
	num.Alignment = fyne.TextAlignCenter
	num.TextStyle = fyne.TextStyle{Bold: c.today || c.sel}

	return &dayCellRenderer{c: c, bg: bg, num: num}
}

type dayCellRenderer struct {
	c     *dayCell
	bg    *canvas.Rectangle
	num   *canvas.Text
	marks []fyne.CanvasObject
}

func (r *dayCellRenderer) Layout(s fyne.Size) {
	r.bg.Move(fyne.NewPos(2, 2))
	r.bg.Resize(fyne.NewSize(s.Width-4, s.Height-4))
	ts := r.num.MinSize()
	r.num.Move(fyne.NewPos((s.Width-ts.Width)/2, 6))
	r.num.Resize(ts)

	r.marks = r.marks[:0]
	n := len(r.c.colors)
	if n == 0 {
		return
	}
	show := n
	if show > 4 {
		show = 4
	}
	const dot, gap = 7, 3
	total := float32(show)*dot + float32(show-1)*gap
	x := (s.Width - total) / 2
	y := s.Height - dot - 7
	for i := 0; i < show; i++ {
		cc := r.c.colors[i]
		d := canvas.NewCircle(cc)
		if r.c.sel {
			d.StrokeColor = color.White
			d.StrokeWidth = 1
		}
		d.Move(fyne.NewPos(x+float32(i)*(dot+gap), y))
		d.Resize(fyne.NewSize(dot, dot))
		r.marks = append(r.marks, d)
	}
	if n > 4 {
		plus := canvas.NewText("+", r.num.Color)
		plus.TextSize = 10
		plus.Move(fyne.NewPos(x+total+1, y-4))
		r.marks = append(r.marks, plus)
	}
}

func (r *dayCellRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *dayCellRenderer) Refresh()           { r.Layout(r.c.Size()); canvas.Refresh(r.c) }
func (r *dayCellRenderer) Destroy()           {}
func (r *dayCellRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.bg, r.num}, r.marks...)
}

// ─────────────────────────── карточка заметки ───────────────────────────

// noteRow — заметка в списке: цветная полоса слева, текст с переносом,
// подпись (повтор / дата), кнопки «изменить» и «удалить».
func noteRow(n store.CalNote, sub string, onEdit, onDelete func()) fyne.CanvasObject {
	bar := canvas.NewRectangle(parseHex(n.Color))
	bar.SetMinSize(fyne.NewSize(6, 10))
	bar.CornerRadius = 3
	text := wrapLabel(n.Text)
	parts := []fyne.CanvasObject{text}
	if sub != "" {
		s := wrapLabel(sub)
		s.Importance = widget.LowImportance
		s.SizeName = theme.SizeNameCaptionText
		parts = append(parts, s)
	}
	edit := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), onEdit)
	edit.Importance = widget.LowImportance
	del := widget.NewButtonWithIcon("", theme.DeleteIcon(), onDelete)
	del.Importance = widget.LowImportance
	tint := parseHex(n.Color)
	tint.A = 0x1c
	bg := canvas.NewRectangle(tint)
	bg.CornerRadius = 8
	body := container.NewBorder(nil, nil, bar, container.NewHBox(edit, del), container.NewVBox(parts...))
	return container.NewStack(bg, container.NewPadded(body))
}

// noteDialog — форма заметки: текст, цвет и (для новой заметки) режим повтора.
// weekday: 0 — показать выбор «только этот день / каждый <день недели>».
func noteDialog(a *App, title string, day time.Time, n *store.CalNote, allowRepeat bool,
	onSave func(text, color string, weekly bool)) {
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapWord
	entry.SetMinRowsVisible(4)
	entry.SetPlaceHolder("Текст заметки")
	initColor := store.DefaultNoteColor
	if n != nil {
		entry.SetText(n.Text)
		initColor = n.Color
	}
	picker := newColorPicker(a.Win, initColor)

	items := []fyne.CanvasObject{entry, widget.NewLabelWithStyle("Цвет:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), picker}
	var repeat *widget.RadioGroup
	if allowRepeat {
		wd := store.ISOWeekday(day)
		repeat = widget.NewRadioGroup([]string{
			"Только " + day.Format("02.01.2006"),
			"Повторять " + weekdaysEvery[wd-1],
		}, nil)
		repeat.SetSelected(repeat.Options[0])
		items = append(items, widget.NewSeparator(), repeat)
	}
	content := container.NewVBox(items...)
	d := dialog.NewCustomConfirm(title, "Сохранить", "Отмена", content, func(ok bool) {
		if !ok {
			return
		}
		txt := strings.TrimSpace(entry.Text)
		if txt == "" {
			return
		}
		weekly := repeat != nil && repeat.Selected == repeat.Options[1]
		onSave(txt, picker.value, weekly)
	}, a.Win)
	d.Resize(fyne.NewSize(380, 460))
	d.Show()
	a.Win.Canvas().Focus(entry)
}

// ─────────────────────────── вкладка «Заметки» ───────────────────────────

func notesView(a *App, onChanged func()) (fyne.CanvasObject, func()) {
	selected := dateOnly(time.Now())
	title := bold("")
	listBox := container.NewVBox()
	var cal *monthGrid

	var refresh func()
	refresh = func() {
		wd := store.ISOWeekday(selected)
		title.SetText(fmt.Sprintf("%s, %s", selected.Format("02.01.2006"), strings.ToLower(weekdaysFull[wd-1])))
		listBox.Objects = nil
		notes, err := a.St.NotesOn(selected)
		if err != nil {
			listBox.Add(wrapLabel(errText(err)))
		}
		if len(notes) == 0 {
			l := wrapLabel("Заметок на этот день нет.")
			l.Importance = widget.LowImportance
			listBox.Add(l)
		}
		for _, n := range notes {
			n := n
			sub := ""
			if n.Weekly() {
				sub = "↻ " + weekdaysEvery[n.Weekday-1]
			}
			listBox.Add(noteRow(n, sub,
				func() {
					noteDialog(a, "Изменить заметку", selected, &n, false, func(text, col string, _ bool) {
						_ = a.St.UpdateCalNote(n.ID, text, col)
						refresh()
						cal.Refresh()
						onChanged()
					})
				},
				func() {
					done := func() { refresh(); cal.Refresh(); onChanged() }
					if !n.Weekly() {
						dialog.ShowConfirm("Удаление", "Удалить заметку?", func(ok bool) {
							if ok {
								_ = a.St.DeleteCalNote(n.ID)
								done()
							}
						}, a.Win)
						return
					}
					// Повторяющаяся: убрать только с этого дня или со всех недель.
					var d dialog.Dialog
					day := selected.Format("2006-01-02")
					onlyDay := widget.NewButton("Убрать только "+selected.Format("02.01"), func() {
						_ = a.St.SkipWeeklyOn(n.ID, day)
						d.Hide()
						done()
					})
					onlyDay.Importance = widget.HighImportance
					all := widget.NewButton("Удалить со всех недель", func() {
						_ = a.St.DeleteCalNote(n.ID)
						d.Hide()
						done()
					})
					all.Importance = widget.DangerImportance
					msg := wrapLabel("Заметка повторяется " + weekdaysEvery[n.Weekday-1] + ".")
					d = dialog.NewCustom("Удаление", "Отмена", container.NewVBox(msg, onlyDay, all), a.Win)
					d.Resize(fyne.NewSize(340, 220))
					d.Show()
				}))
		}
		listBox.Refresh()
	}

	cal = newMonthGrid(a, selected, func(t time.Time) {
		selected = t
		refresh()
	})

	add := widget.NewButtonWithIcon("Добавить заметку", theme.ContentAddIcon(), func() {
		noteDialog(a, "Заметка на "+selected.Format("02.01.2006"), selected, nil, true,
			func(text, col string, weekly bool) {
				var err error
				if weekly {
					_, err = a.St.AddWeeklyNote(store.ISOWeekday(selected), selected.Format("2006-01-02"), text, col)
				} else {
					_, err = a.St.AddDayNote(selected.Format("2006-01-02"), text, col)
				}
				if err != nil {
					dialog.ShowError(err, a.Win)
				}
				refresh()
				cal.Refresh()
				onChanged()
			})
	})
	add.Importance = widget.HighImportance

	refresh()
	page := container.NewVBox(cal.content, widget.NewSeparator(), title, add, listBox)
	reload := func() { cal.Refresh(); refresh() }
	return container.NewVScroll(container.NewPadded(page)), reload
}

// ─────────────────────── вкладка «Дни недели» ───────────────────────

// weekdaysView — заметки, привязанные к дню недели: выбираешь Пн…Вс, и
// заметка появляется на этом дне каждой следующей недели.
func weekdaysView(a *App, onChanged func()) (fyne.CanvasObject, func()) {
	cur := store.ISOWeekday(time.Now())
	listBox := container.NewVBox()
	head := bold("")

	var refresh func()
	var dayBtns []*widget.Button
	refresh = func() {
		for i, b := range dayBtns {
			if i+1 == cur {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.MediumImportance
			}
			b.Refresh()
		}
		head.SetText(weekdaysFull[cur-1] + " — " + weekdaysEvery[cur-1])
		listBox.Objects = nil
		all, _ := a.St.WeeklyNotes()
		cnt := 0
		for _, n := range all {
			if n.Weekday != cur {
				continue
			}
			cnt++
			n := n
			from, _ := time.ParseInLocation("2006-01-02", n.From, time.Local)
			sub := "действует с " + from.Format("02.01.2006")
			if k := a.St.SkipCount(n.ID); k > 0 {
				sub += fmt.Sprintf(" · пропущено дней: %d", k)
			}
			listBox.Add(noteRow(n, sub,
				func() {
					noteDialog(a, "Изменить заметку", from, &n, false, func(text, col string, _ bool) {
						_ = a.St.UpdateCalNote(n.ID, text, col)
						refresh()
						onChanged()
					})
				},
				func() {
					dialog.ShowConfirm("Удаление", "Удалить заметку со всех недель ("+weekdaysEvery[n.Weekday-1]+")?", func(ok bool) {
						if ok {
							_ = a.St.DeleteCalNote(n.ID)
							refresh()
							onChanged()
						}
					}, a.Win)
				}))
		}
		if cnt == 0 {
			l := wrapLabel("Пока нет заметок на " + strings.ToLower(weekdaysFull[cur-1]) + ".")
			l.Importance = widget.LowImportance
			listBox.Add(l)
		}
		listBox.Refresh()
	}

	days := container.NewGridWithColumns(7)
	for i, d := range weekdaysShort {
		wd := i + 1
		b := widget.NewButton(d, func() { cur = wd; refresh() })
		dayBtns = append(dayBtns, b)
		days.Add(b)
	}

	add := widget.NewButtonWithIcon("Добавить заметку на день недели", theme.ContentAddIcon(), func() {
		// Начинаем с ближайшего такого дня недели (сегодня, если совпадает).
		today := dateOnly(time.Now())
		start := today.AddDate(0, 0, (cur-store.ISOWeekday(today)+7)%7)
		noteDialog(a, "Заметка: "+weekdaysEvery[cur-1], start, nil, false, func(text, col string, _ bool) {
			if _, err := a.St.AddWeeklyNote(cur, today.Format("2006-01-02"), text, col); err != nil {
				dialog.ShowError(err, a.Win)
			}
			refresh()
			onChanged()
		})
	})
	add.Importance = widget.HighImportance

	help := wrapLabel("Выберите день недели и добавьте заметку — она будет показываться в календаре на этом дне каждой недели, начиная с текущей.")
	help.Importance = widget.LowImportance

	refresh()
	page := container.NewVBox(help, days, widget.NewSeparator(), head, add, listBox)
	return container.NewVScroll(container.NewPadded(page)), refresh
}

// ─────────────────────────── раздел «Календарь» ───────────────────────────

func newCalendarTab(a *App) fyne.CanvasObject {
	var reloadNotes, reloadWeek func()
	notes, rn := notesView(a, func() {
		if reloadWeek != nil {
			reloadWeek()
		}
	})
	week, rw := weekdaysView(a, func() {
		if reloadNotes != nil {
			reloadNotes()
		}
	})
	reloadNotes, reloadWeek = rn, rw
	tabs := container.NewAppTabs(
		container.NewTabItem("Заметки", notes),
		container.NewTabItem("Дни недели", week),
		container.NewTabItem("Интервал", intervalView(a)),
	)
	return tabs
}
