package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/store"
	"ru.navhelper/internal/timecalc"
)

// newCalcTab — модуль 1: калькулятор времени без ограничения 24 ч.
func newCalcTab(a *App) fyne.CanvasObject {
	calc := &timecalc.Calculator{}
	// Продолжаем с последнего результата из истории.
	if h, err := a.St.CalcHistory(1); err == nil && len(h) > 0 {
		calc.Value = timecalc.Duration(h[0].Sec)
	}

	result := widget.NewLabelWithStyle(calc.Value.String(), fyne.TextAlignCenter, fyne.TextStyle{Bold: true, Monospace: true})
	result.SizeName = "headingText"
	clock := widget.NewLabelWithStyle(calc.Value.Clock(), fyne.TextAlignCenter, fyne.TextStyle{Monospace: true})

	hours := widget.NewEntry()
	hours.SetPlaceHolder("часы (можно 150 или -5)")
	minutes := widget.NewEntry()
	minutes.SetPlaceHolder("минуты")

	var history []store.CalcEntry
	list := widget.NewList(
		func() int { return len(history) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			e := history[i]
			o.(*widget.Label).SetText(e.At.Format("02.01 15:04") + "   " + e.Expr)
		},
	)
	reload := func() {
		history, _ = a.St.CalcHistory(100)
		list.Refresh()
	}
	show := func() {
		result.SetText(calc.Value.String())
		clock.SetText(calc.Value.Clock() + "   (" + calc.Value.DaysString() + ")")
	}

	apply := func(op timecalc.Op) {
		d, err := timecalc.ParseParts(hours.Text, minutes.Text)
		if err != nil {
			dialog.ShowError(err, a.Win)
			return
		}
		expr := calc.Apply(op, d)
		_ = a.St.AddCalc(expr, calc.Value.String(), int64(calc.Value))
		hours.SetText("")
		minutes.SetText("")
		show()
		reload()
	}

	plus := widget.NewButton("+", func() { apply(timecalc.OpAdd) })
	plus.Importance = widget.HighImportance
	minus := widget.NewButton("−", func() { apply(timecalc.OpSub) })
	minus.Importance = widget.HighImportance
	reset := widget.NewButton("Сброс", func() {
		calc.Reset()
		_ = a.St.AddCalc("Сброс = 0 ч 00 мин", calc.Value.String(), 0)
		show()
		reload()
	})
	clear := widget.NewButton("Очистить историю", func() {
		dialog.ShowConfirm("История", "Удалить всю историю вычислений?", func(ok bool) {
			if ok {
				_ = a.St.ClearCalcHistory()
				reload()
			}
		}, a.Win)
	})

	show()
	reload()

	top := container.NewVBox(
		bold("Калькулятор времени (> 24 ч)"),
		result, clock,
		container.NewGridWithColumns(2, hours, minutes),
		container.NewGridWithColumns(3, plus, minus, reset),
		widget.NewSeparator(),
		container.NewBorder(nil, nil, widget.NewLabel("История"), clear),
	)
	return container.NewBorder(top, nil, nil, nil, list)
}
