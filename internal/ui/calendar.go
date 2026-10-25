package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/datecalc"
	"ru.navhelper/internal/store"
)

// newCalendarTab — модуль 2: календарь с заметками + расчёт интервала.
func newCalendarTab(a *App) fyne.CanvasObject {
	return container.NewAppTabs(
		container.NewTabItem("Заметки", notesView(a)),
		container.NewTabItem("Интервал", intervalView(a)),
	)
}

func notesView(a *App) fyne.CanvasObject {
	selected := time.Now()
	dayKey := func(t time.Time) string { return t.Format("2006-01-02") }

	title := bold("")
	noteText := wrapLabel("")

	var notes []store.Note
	list := widget.NewList(
		func() int { return len(notes) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			n := notes[i]
			d, _ := time.Parse("2006-01-02", n.Day)
			o.(*widget.Label).SetText(d.Format("02.01.2006") + " — " + n.Text)
		},
	)

	refresh := func() {
		title.SetText("Заметка на " + selected.Format("02.01.2006"))
		if n, ok := a.St.Note(dayKey(selected)); ok {
			noteText.SetText(n.Text)
		} else {
			noteText.SetText("— нет заметки —")
		}
		notes, _ = a.St.AllNotes()
		list.Refresh()
	}

	// В Fyne у календаря нет метода SetDate, поэтому при переходе к другому дню
	// виджет пересоздаётся внутри контейнера.
	var calBox *fyne.Container
	newCal := func(t time.Time) *widget.Calendar {
		return widget.NewCalendar(t, func(nt time.Time) {
			selected = nt
			refresh()
		})
	}
	calBox = container.NewStack(newCal(selected))
	rebuildCal := func(t time.Time) {
		calBox.Objects = []fyne.CanvasObject{newCal(t)}
		calBox.Refresh()
	}

	edit := widget.NewButton("Добавить / изменить", func() {
		entry := widget.NewMultiLineEntry()
		entry.SetMinRowsVisible(5)
		entry.Wrapping = fyne.TextWrapWord
		if n, ok := a.St.Note(dayKey(selected)); ok {
			entry.SetText(n.Text)
		}
		d := dialog.NewForm("Заметка "+selected.Format("02.01.2006"), "Сохранить", "Отмена",
			[]*widget.FormItem{widget.NewFormItem("", entry)},
			func(ok bool) {
				if !ok {
					return
				}
				if err := a.St.SetNote(dayKey(selected), entry.Text); err != nil {
					dialog.ShowError(err, a.Win)
				}
				refresh()
			}, a.Win)
		d.Resize(fyne.NewSize(380, 320))
		d.Show()
	})
	edit.Importance = widget.HighImportance

	del := widget.NewButton("Удалить", func() {
		if _, ok := a.St.Note(dayKey(selected)); !ok {
			return
		}
		dialog.ShowConfirm("Удаление", "Удалить заметку на "+selected.Format("02.01.2006")+"?", func(ok bool) {
			if ok {
				_ = a.St.SetNote(dayKey(selected), "")
				refresh()
			}
		}, a.Win)
	})

	// Нажатие на заметку в списке — переход к этому дню.
	list.OnSelected = func(i widget.ListItemID) {
		if t, err := time.ParseInLocation("2006-01-02", notes[i].Day, time.Local); err == nil {
			selected = t
			rebuildCal(t)
			refresh()
		}
		list.UnselectAll()
	}

	refresh()
	top := container.NewVBox(calBox, title, noteText, container.NewGridWithColumns(2, edit, del),
		widget.NewSeparator(), widget.NewLabel("Все заметки:"))
	return container.NewBorder(top, nil, nil, nil, list)
}

func intervalView(a *App) fyne.CanvasObject {
	now := time.Now()
	from := widget.NewEntry()
	from.SetPlaceHolder("ДД.ММ.ГГГГ ЧЧ:ММ")
	from.SetText(now.AddDate(0, 0, -1).Format(datecalc.Layout))
	to := widget.NewEntry()
	to.SetPlaceHolder("ДД.ММ.ГГГГ ЧЧ:ММ")
	to.SetText(now.Format(datecalc.Layout))

	result := wrapLabel("")
	result.TextStyle = fyne.TextStyle{Bold: true}
	extra := wrapLabel("")

	calc := widget.NewButton("Рассчитать", func() {
		f, err := datecalc.ParseDateTime(from.Text, time.Local)
		if err != nil {
			dialog.ShowError(err, a.Win)
			return
		}
		t, err := datecalc.ParseDateTime(to.Text, time.Local)
		if err != nil {
			dialog.ShowError(err, a.Win)
			return
		}
		text := datecalc.Describe(f, t)
		iv := datecalc.Between(f, t)
		result.SetText(text)
		hours := iv.TotalMinutes / 60
		extra.SetText("Всего: " + itoa(hours) + " ч " + itoa(iv.TotalMinutes%60) + " мин  (" + itoa(iv.TotalMinutes) + " мин)")
		_ = a.St.AddInterval(f, t, from.Text, to.Text, text)
	})
	calc.Importance = widget.HighImportance

	nowFrom := widget.NewButton("сейчас", func() { from.SetText(time.Now().Format(datecalc.Layout)) })
	nowTo := widget.NewButton("сейчас", func() { to.SetText(time.Now().Format(datecalc.Layout)) })

	return container.NewVScroll(container.NewVBox(
		bold("Время между двумя датами"),
		widget.NewLabel("С:"), container.NewBorder(nil, nil, nil, nowFrom, from),
		widget.NewLabel("До:"), container.NewBorder(nil, nil, nil, nowTo, to),
		calc, result, extra,
	))
}

func itoa(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	if n == 0 {
		return "0"
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
