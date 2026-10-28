package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/datecalc"
)

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
