package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/store"
	"ru.navhelper/internal/timecalc"
)

// Цвета клавиш в духе allcalc.ru: светлые цифровые, серые служебные,
// акцентные операции и «=».
var (
	keyDigit = color.NRGBA{0xf4, 0xf6, 0xf9, 0xff}
	keyFunc  = color.NRGBA{0xdd, 0xe3, 0xea, 0xff}
	keyOp    = color.NRGBA{0xff, 0xe2, 0xb8, 0xff}
	keyEq    = color.NRGBA{0x1f, 0x6f, 0xb5, 0xff}
	keyClear = color.NRGBA{0xf6, 0xd0, 0xcc, 0xff}
	lcdBg    = color.NRGBA{0xe6, 0xee, 0xd8, 0xff}
	lcdInk   = color.NRGBA{0x1c, 0x24, 0x1a, 0xff}
	lcdDim   = color.NRGBA{0x56, 0x62, 0x50, 0xff}
)

// calcKey — тапаемая клавиша с собственным фоном (стандартная кнопка Fyne
// не даёт раскрашивать каждую клавишу).
type calcKey struct {
	widget.BaseWidget
	label string
	bg    color.Color
	fg    color.Color
	tap   func()
	rect  *canvas.Rectangle
}

func newCalcKey(label string, bg, fg color.Color, tap func()) *calcKey {
	k := &calcKey{label: label, bg: bg, fg: fg, tap: tap}
	k.ExtendBaseWidget(k)
	return k
}

func (k *calcKey) Tapped(*fyne.PointEvent) {
	if k.tap == nil {
		return
	}
	// короткая «вспышка» нажатия
	k.rect.FillColor = color.NRGBA{0xb0, 0xbc, 0xc8, 0xff}
	k.rect.Refresh()
	k.tap()
	go func() {
		fyne.Do(func() {
			k.rect.FillColor = k.bg
			k.rect.Refresh()
		})
	}()
}

func (k *calcKey) CreateRenderer() fyne.WidgetRenderer {
	k.rect = canvas.NewRectangle(k.bg)
	k.rect.CornerRadius = 10
	k.rect.StrokeColor = color.NRGBA{0xc4, 0xcc, 0xd6, 0xff}
	k.rect.StrokeWidth = 1
	t := canvas.NewText(k.label, k.fg)
	t.Alignment = fyne.TextAlignCenter
	t.TextSize = theme.TextSize() * 1.6
	t.TextStyle = fyne.TextStyle{Bold: true}
	return widget.NewSimpleRenderer(container.NewStack(k.rect, container.NewCenter(t)))
}

func (k *calcKey) MinSize() fyne.Size { return fyne.NewSize(56, 56) }

// newCalcTab — модуль 1: калькулятор времени без ограничения 24 ч.
func newCalcTab(a *App) fyne.CanvasObject {
	kp := &timecalc.Keypad{}
	if h, err := a.St.CalcHistory(1); err == nil && len(h) > 0 {
		kp.SetValue(timecalc.Duration(h[0].Sec))
	}

	// ── дисплей ──
	exprT := canvas.NewText("", lcdDim)
	exprT.Alignment = fyne.TextAlignTrailing
	exprT.TextSize = theme.TextSize() * 1.05
	mainT := canvas.NewText("", lcdInk)
	mainT.Alignment = fyne.TextAlignTrailing
	mainT.TextSize = theme.TextSize() * 3
	mainT.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	hintT := canvas.NewText("", lcdDim)
	hintT.Alignment = fyne.TextAlignTrailing
	hintT.TextSize = theme.TextSize() * 0.95
	errT := canvas.NewText("", color.NRGBA{0xc0, 0x30, 0x20, 0xff})
	errT.Alignment = fyne.TextAlignTrailing
	errT.TextSize = theme.TextSize() * 0.9

	lcd := canvas.NewRectangle(lcdBg)
	lcd.CornerRadius = 12
	lcd.StrokeColor = color.NRGBA{0xa9, 0xb8, 0x96, 0xff}
	lcd.StrokeWidth = 2
	display := container.NewStack(lcd, container.NewPadded(container.NewVBox(exprT, mainT, hintT, errT)))

	// ── история ──
	var history []store.CalcEntry
	list := widget.NewList(
		func() int { return len(history) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			e := history[i]
			o.(*widget.Label).SetText(e.At.Format("02.01 15:04") + "   " + e.Expr)
		},
	)
	list.OnSelected = func(i widget.ListItemID) {
		kp.SetValue(timecalc.Duration(history[i].Sec))
		list.UnselectAll()
	}
	reload := func() {
		history, _ = a.St.CalcHistory(100)
		list.Refresh()
	}

	show := func() {
		exprT.Text = kp.Expr()
		mainT.Text = kp.Display()
		// уменьшаем шрифт для длинных чисел, чтобы не вылезали за экран
		size := theme.TextSize() * 3
		if n := len([]rune(mainT.Text)); n > 9 {
			size = theme.TextSize() * 3 * 9 / float32(n)
		}
		mainT.TextSize = size
		hintT.Text = kp.Hint()
		errT.Text = kp.Err
		exprT.Refresh()
		mainT.Refresh()
		hintT.Refresh()
		errT.Refresh()
	}

	press := func(key string) func() {
		return func() {
			if res := kp.Press(key); res != "" {
				_ = a.St.AddCalc(res, kp.Value().String(), int64(kp.Value()))
				reload()
			}
			show()
		}
	}

	k := func(label string) fyne.CanvasObject {
		bg, fg := keyDigit, lcdInk
		switch label {
		case "+", "−", "×", "÷":
			bg = keyOp
		case "=":
			bg, fg = keyEq, color.NRGBA{0xff, 0xff, 0xff, 0xff}
		case "C":
			bg = keyClear
		case "⌫", "±", ":", ",":
			bg = keyFunc
		}
		return newCalcKey(label, bg, fg, press(label))
	}

	grid := container.NewGridWithColumns(4,
		k("C"), k("⌫"), k("±"), k("÷"),
		k("7"), k("8"), k("9"), k("×"),
		k("4"), k("5"), k("6"), k("−"),
		k("1"), k("2"), k("3"), k("+"),
		k(":"), k("0"), k(","), k("="),
	)

	clear := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("История", "Удалить всю историю вычислений?", func(ok bool) {
			if ok {
				_ = a.St.ClearCalcHistory()
				reload()
			}
		}, a.Win)
	})
	clear.Importance = widget.LowImportance

	help := widget.NewLabel("«150:30» — 150 ч 30 мин · «5» — 5 ч · × и ÷ — на число")
	help.Importance = widget.LowImportance
	help.Wrapping = fyne.TextWrapWord
	help.Alignment = fyne.TextAlignCenter

	show()
	reload()

	top := container.NewVBox(display, help, grid)
	histHead := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("История (нажмите — продолжить с результата)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		clear)
	_ = layout.NewSpacer
	return container.NewBorder(container.NewPadded(top), nil, nil, nil,
		container.NewBorder(container.NewPadded(histHead), nil, nil, nil, list))
}
