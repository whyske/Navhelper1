package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/navcalc"
)

// Режимы калькулятора v3. Первый режим — прежний калькулятор времени
// (newCalcTab, код не менялся). Остальные — штурманские расчёты.
type calcMode struct {
	Name string
	Icon fyne.Resource
	View func(a *App) fyne.CanvasObject
}

func calcModes() []calcMode {
	return []calcMode{
		{"Время", theme.HistoryIcon(), newCalcTab},
		{"Путь", theme.ConfirmIcon(), func(a *App) fyne.CanvasObject { return newDistanceView(a) }},
		{"Скорость", theme.MediaFastForwardIcon(), func(a *App) fyne.CanvasObject { return newSpeedView(a) }},
		{"Время в пути", theme.ContentRedoIcon(), func(a *App) fyne.CanvasObject { return newTimeView(a) }},
		{"Топливо", theme.StorageIcon(), func(a *App) fyne.CanvasObject { return newFuelView(a) }},
		{"Течение", theme.MediaPlayIcon(), func(a *App) fyne.CanvasObject { return newCurrentView(a) }},
		{"Видимость", theme.VisibilityIcon(), func(a *App) fyne.CanvasObject { return newVisibilityView(a) }},
		{"Единицы", theme.MenuIcon(), func(a *App) fyne.CanvasObject { return newUnitsView(a) }},
	}
}

// newCalcSection — вкладка «Калькулятор»: переключатель режимов сверху,
// сам режим — ниже. Выбранный режим запоминается между запусками.
func newCalcSection(a *App) fyne.CanvasObject {
	modes := calcModes()
	names := make([]string, len(modes))
	byName := map[string]calcMode{}
	for i, m := range modes {
		names[i] = m.Name
		byName[m.Name] = m
	}
	cur := a.St.Setting("calc3.mode", modes[0].Name)
	if _, ok := byName[cur]; !ok {
		cur = modes[0].Name
	}

	body := container.NewStack()
	var showMode func(name string)
	showMode = func(name string) {
		m := byName[name]
		body.Objects = []fyne.CanvasObject{m.View(a)}
		body.Refresh()
	}

	sel := widget.NewSelect(names, func(s string) {
		if s == "" {
			return
		}
		_ = a.St.SetSetting("calc3.mode", s)
		showMode(s)
	})
	sel.SetSelected(cur)

	// Быстрые чипы режимов — на телефоне переключаться удобнее, чем через список.
	chips := container.NewHScroll(container.NewHBox())
	row := chips.Content.(*fyne.Container)
	buttons := make([]*widget.Button, len(names))
	for i, n := range names {
		n := n
		b := widget.NewButton(n, func() { sel.SetSelected(n) })
		b.Importance = widget.LowImportance
		buttons[i] = b
		row.Add(b)
	}
	sel.OnChanged = func(s string) {
		_ = a.St.SetSetting("calc3.mode", s)
		for i, n := range names {
			if n == s {
				buttons[i].Importance = widget.HighImportance
			} else {
				buttons[i].Importance = widget.LowImportance
			}
			buttons[i].Refresh()
		}
		showMode(s)
	}

	top := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabelWithStyle("Режим:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, sel),
		chips,
		widget.NewSeparator(),
	)
	sel.SetSelected(cur) // выставит подсветку и отрисует режим
	return container.NewBorder(container.NewPadded(top), nil, nil, nil, body)
}

// ─────────────────────────── общие поля формы ───────────────────────────

// calcField — подписанное поле ввода с подсказкой и единицей измерения.
func calcField(label, placeholder, unit string) (*widget.Entry, fyne.CanvasObject) {
	e := widget.NewEntry()
	e.SetPlaceHolder(placeholder)
	l := widget.NewLabelWithStyle(label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	var right fyne.CanvasObject
	if unit != "" {
		u := widget.NewLabel(unit)
		u.Importance = widget.LowImportance
		right = u
	}
	return e, container.NewVBox(l, container.NewBorder(nil, nil, nil, right, e))
}

// calcResult — крупная строка результата на светлой подложке.
type calcResult struct {
	main *canvas.Text
	sub  *widget.Label
	box  *fyne.Container
}

func newCalcResult() *calcResult {
	main := canvas.NewText("", theme.Color(theme.ColorNameForeground))
	main.Alignment = fyne.TextAlignCenter
	main.TextStyle = fyne.TextStyle{Bold: true}
	main.TextSize = theme.TextSize() * 1.8
	sub := widget.NewLabel("")
	sub.Alignment = fyne.TextAlignCenter
	sub.Wrapping = fyne.TextWrapWord
	sub.Importance = widget.LowImportance
	r := &calcResult{main: main, sub: sub}
	r.box = container.NewPadded(container.NewVBox(container.NewCenter(main), sub))
	return r
}

func (r *calcResult) setErr(err error) {
	r.main.Text = "—"
	r.main.Refresh()
	r.sub.SetText(errText(err))
}

func (r *calcResult) set(main, sub string) {
	r.main.Text = main
	r.main.Refresh()
	r.sub.SetText(sub)
}

// resultCard оборачивает результат в карточку.
func resultCard(r *calcResult) fyne.CanvasObject { return r.box }

// ─────────────────────────── режим «Путь» ───────────────────────────

func newDistanceView(a *App) fyne.CanvasObject {
	res := newCalcResult()
	speedE, speedRow := calcField("Скорость", "12", "уз")
	timeE, timeRow := calcField("Время", "2:30 (ч:мм)", "")
	speedE.SetText("12")
	timeE.SetText("0:00")

	calc := widget.NewButton("Рассчитать", func() {
		v, err := navcalc.ParseNum(speedE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		t, err := navcalc.ParseHM(timeE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		s := navcalc.Distance(v, t)
		res.set(navcalc.Format(s, 2)+" мор. миль",
			fmt.Sprintf("%s км · %s каб. · время %s",
				navcalc.Format(s*navcalc.NMkm, 2),
				navcalc.Format(s*10, 1),
				navcalc.HM(t)))
	})
	calc.Importance = widget.HighImportance

	help := wrapLabel("S = V × t. Скорость в узлах, время в часах и минутах.")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Пройденное расстояние"), speedRow, timeRow, calc, resultCard(res), help)))
}

// ─────────────────────────── режим «Скорость» ───────────────────────────

func newSpeedView(a *App) fyne.CanvasObject {
	res := newCalcResult()
	distE, distRow := calcField("Расстояние", "30", "мор. мили")
	timeE, timeRow := calcField("Время", "2:30 (ч:мм)", "")
	distE.SetText("30")
	timeE.SetText("2:30")

	calc := widget.NewButton("Рассчитать", func() {
		d, err := navcalc.ParseNum(distE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		t, err := navcalc.ParseHM(timeE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		v, err := navcalc.Speed(d, t)
		if err != nil {
			res.setErr(err)
			return
		}
		res.set(navcalc.Format(v, 2)+" уз",
			fmt.Sprintf("%s км/ч · %s м/с · %s миль за сутки",
				navcalc.Format(v*navcalc.NMkm, 2),
				navcalc.Format(v*navcalc.KnotMS, 2),
				navcalc.Format(v*24, 1)))
	})
	calc.Importance = widget.HighImportance

	help := wrapLabel("V = S / t. Узел — одна морская миля в час (1,852 км/ч).")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Скорость по расстоянию и времени"), distRow, timeRow, calc, resultCard(res), help)))
}

// ─────────────────────────── режим «Время в пути» ───────────────────────────

func newTimeView(a *App) fyne.CanvasObject {
	res := newCalcResult()
	distE, distRow := calcField("Расстояние", "100", "мор. мили")
	speedE, speedRow := calcField("Скорость", "12", "уз")
	depE, depRow := calcField("Время отхода", "22:40 или 28.10 22:40", "")
	distE.SetText("100")
	speedE.SetText("12")
	now := time.Now()
	depE.SetText(now.Format("02.01 15:04"))

	calc := widget.NewButton("Рассчитать", func() {
		d, err := navcalc.ParseNum(distE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		v, err := navcalc.ParseNum(speedE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		dep, err := navcalc.ParseClock(depE.Text, now)
		if err != nil {
			res.setErr(err)
			return
		}
		eta, t, err := navcalc.ETA(dep, d, v)
		if err != nil {
			res.setErr(err)
			return
		}
		days := eta.Sub(dep).Hours() / 24
		extra := "переход " + navcalc.HM(t) + " ч"
		if days >= 1 {
			extra += fmt.Sprintf(" · %s сут", navcalc.Format(days, 1))
		}
		res.set("Прибытие "+eta.Format("02.01 в 15:04"), extra)
	})
	calc.Importance = widget.HighImportance

	// Обратная задача: какая нужна скорость, чтобы прийти к сроку.
	toE, toRow := calcField("Нужно быть на месте", "30.10 08:00", "")
	toE.SetText(now.AddDate(0, 0, 2).Format("02.01 15:04"))
	rev := widget.NewButton("Обратный расчёт: нужная скорость", func() {
		d, err := navcalc.ParseNum(distE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		dep, err := navcalc.ParseClock(depE.Text, now)
		if err != nil {
			res.setErr(err)
			return
		}
		to, err := navcalc.ParseClock(toE.Text, now)
		if err != nil {
			res.setErr(err)
			return
		}
		v, t, err := navcalc.RequiredSpeed(d, dep, to)
		if err != nil {
			res.setErr(err)
			return
		}
		res.set("Нужная скорость: "+navcalc.Format(v, 2)+" уз",
			fmt.Sprintf("в вашем распоряжении %s ч (%s сут)", navcalc.HM(t), navcalc.Format(float64(t)/86400, 1)))
	})
	rev.Importance = widget.MediumImportance

	help := wrapLabel("t = S / V. Время отхода можно указать как «22:40» (сегодня) или «28.10 22:40».")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Время перехода и время прибытия"),
		distRow, speedRow, depRow, calc, resultCard(res),
		widget.NewSeparator(), toRow, rev, help)))
}

// ─────────────────────────── режим «Топливо» ───────────────────────────

func newFuelView(a *App) fyne.CanvasObject {
	res := newCalcResult()
	rateE, rateRow := calcField("Расход в час", "12", "л/ч")
	timeE, timeRow := calcField("Время хода", "6:30 (ч:мм)", "")
	rateE.SetText("12")
	timeE.SetText("6:30")

	calc := widget.NewButton("Рассчитать", func() {
		r, err := navcalc.ParseNum(rateE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		t, err := navcalc.ParseHM(timeE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		f := navcalc.Fuel(r, t)
		res.set(navcalc.Format(f, 1)+" л",
			fmt.Sprintf("за %s ч · резерв 15%% — %s л",
				navcalc.HM(t), navcalc.Format(f*1.15, 1)))
	})
	calc.Importance = widget.HighImportance

	help := wrapLabel("Расход = часовой расход × время хода. Резерв показан для планирования запаса.")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Расход топлива"), rateRow, timeRow, calc, resultCard(res), help)))
}

// ─────────────────────────── режим «Течение» ───────────────────────────

func newCurrentView(a *App) fyne.CanvasObject {
	res := newCalcResult()
	vE, vRow := calcField("Скорость судна", "10", "уз")
	ikE, ikRow := calcField("Истинный курс", "0", "°")
	vtE, vtRow := calcField("Скорость течения", "2", "уз")
	ktE, ktRow := calcField("Куда направлено течение", "90", "°")
	vE.SetText("10")
	ikE.SetText("0")
	vtE.SetText("2")
	ktE.SetText("90")

	calc := widget.NewButton("Рассчитать", func() {
		v, err := navcalc.ParseNum(vE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		ik, err := navcalc.ParseNum(ikE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		vt, err := navcalc.ParseNum(vtE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		kt, err := navcalc.ParseNum(ktE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		sog, cog := navcalc.Current(v, ik, vt, kt)
		d := navcalc.Drift(ik, cog)
		side := "вправо"
		if d < 0 {
			side = "влево"
		}
		res.set("Скорость над грунтом: "+navcalc.Format(sog, 2)+" уз",
			fmt.Sprintf("Путевой угол %s° · снос %s° %s",
				navcalc.Format(cog, 1), navcalc.Format(abs(d), 1), side))
	})
	calc.Importance = widget.HighImportance

	help := wrapLabel("Векторное сложение скорости судна и течения. «Куда направлено течение» — курс течения, а не откуда оно идёт.")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Течение: скорость и путевой угол над грунтом"),
		vRow, ikRow, vtRow, ktRow, calc, resultCard(res), help)))
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ─────────────────────────── режим «Видимость» ───────────────────────────

func newVisibilityView(a *App) fyne.CanvasObject {
	res := newCalcResult()
	eyeE, eyeRow := calcField("Высота глаза наблюдателя", "4", "м")
	objE, objRow := calcField("Высота предмета", "20", "м")
	eyeE.SetText("4")
	objE.SetText("20")

	calc := widget.NewButton("Рассчитать", func() {
		eye, err := navcalc.ParseNum(eyeE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		obj, err := navcalc.ParseNum(objE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		h := navcalc.Horizon(eye)
		d := navcalc.Visibility(eye, obj)
		res.set("Дальность видимости предмета: "+navcalc.Format(d, 2)+" мор. миль",
			fmt.Sprintf("горизонт с вашей высоты — %s миль · %s км",
				navcalc.Format(h, 2), navcalc.Format(d*navcalc.NMkm, 2)))
	})
	calc.Importance = widget.HighImportance

	help := wrapLabel("Dп = 2,08·(√h₁ + √h₂), Dе = 2,08·√h — геометрическая дальность без учёта рефракции и погоды.")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Дальность видимого горизонта"), eyeRow, objRow, calc, resultCard(res), help)))
}

// ─────────────────────────── режим «Единицы» ───────────────────────────

func newUnitsView(a *App) fyne.CanvasObject {
	groupNames := make([]string, len(navcalc.UnitGroups))
	for i, g := range navcalc.UnitGroups {
		groupNames[i] = g.Name
	}
	var group = navcalc.UnitGroups[0]

	unitNames := func(g navcalc.UnitGroup) []string {
		out := make([]string, len(g.Units))
		for i, u := range g.Units {
			out[i] = u.Name
		}
		return out
	}

	res := newCalcResult()
	fromSel := widget.NewSelect(unitNames(group), nil)
	toSel := widget.NewSelect(unitNames(group), nil)
	fromSel.SetSelected(group.Units[0].Name)
	toSel.SetSelected(group.Units[1].Name)
	valE := widget.NewEntry()
	valE.SetText("1")

	convert := func() {
		v, err := navcalc.ParseNum(valE.Text)
		if err != nil {
			res.setErr(err)
			return
		}
		out, err := navcalc.Convert(group, v, fromSel.Selected, toSel.Selected)
		if err != nil {
			res.setErr(err)
			return
		}
		res.set(navcalc.Format(out, 4)+" "+toSel.Selected,
			navcalc.Format(v, 4)+" "+fromSel.Selected)
	}
	valE.OnChanged = func(string) { convert() }
	fromSel.OnChanged = func(string) { convert() }
	toSel.OnChanged = func(string) { convert() }

	grpSel := widget.NewSelect(groupNames, func(s string) {
		for _, g := range navcalc.UnitGroups {
			if g.Name == s {
				group = g
			}
		}
		fromSel.Options = unitNames(group)
		toSel.Options = unitNames(group)
		fromSel.SetSelected(group.Units[0].Name)
		if len(group.Units) > 1 {
			toSel.SetSelected(group.Units[1].Name)
		}
		fromSel.Refresh()
		toSel.Refresh()
		convert()
	})
	grpSel.SetSelected(group.Name)

	swap := widget.NewButtonWithIcon("Поменять местами", theme.ViewRefreshIcon(), func() {
		f, t := fromSel.Selected, toSel.Selected
		fromSel.SetSelected(t)
		toSel.SetSelected(f)
		convert()
	})
	swap.Importance = widget.LowImportance

	help := wrapLabel("Перевод штурманских единиц: мили, кабельтовы, километры, узлы, км/ч, футы, сажени.")
	help.Importance = widget.LowImportance

	return container.NewVScroll(container.NewPadded(container.NewVBox(
		bold("Перевод единиц"), grpSel,
		container.NewGridWithColumns(2, fromSel, toSel),
		valE, swap, resultCard(res), help)))
}
