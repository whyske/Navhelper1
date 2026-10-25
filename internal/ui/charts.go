package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/charts"
)

// newChartsTab — модуль 4: офлайн-поиск по номеру морской карты.
func newChartsTab(a *App, count int, importErr error) fyne.CanvasObject {
	info := wrapLabel(fmt.Sprintf("В каталоге карт: %d. %s", count, charts.Header(a.St.DB())))
	if importErr != nil {
		info.SetText(errText(importErr))
	}

	card := wrapLabel("Введите номер карты, например 38174, или часть названия.")

	var found []charts.Chart
	list := widget.NewList(
		func() int { return len(found) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c := found[i]
			o.(*widget.Label).SetText(fmt.Sprintf("%s  %s  (%s, %d)", c.Number, c.TitleRU, charts.FormatScale(c.Scale), c.Year))
		},
	)
	list.OnSelected = func(i widget.ListItemID) { card.SetText(describeChart(found[i])) }

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Номер карты или название")
	search := func() {
		q := strings.TrimSpace(entry.Text)
		found = nil
		list.UnselectAll()
		if c, ok := charts.Find(a.St.DB(), q); ok {
			found = []charts.Chart{c}
			card.SetText(describeChart(c))
		} else {
			res, err := charts.Search(a.St.DB(), q, 100)
			if err != nil {
				card.SetText(errText(err))
			} else if len(res) == 0 {
				card.SetText("Карта «" + q + "» в каталоге не найдена.")
			} else {
				found = res
				card.SetText(fmt.Sprintf("Найдено: %d. Выберите карту в списке.", len(res)))
			}
		}
		list.Refresh()
	}
	entry.OnSubmitted = func(string) { search() }
	btn := widget.NewButton("Найти", search)
	btn.Importance = widget.HighImportance

	top := container.NewVBox(
		bold("Поиск морской карты УНиО"),
		container.NewBorder(nil, nil, nil, btn, entry),
		card,
		widget.NewSeparator(),
	)
	return container.NewBorder(top, info, nil, nil, list)
}

// describeChart — карточка карты для экрана.
func describeChart(c charts.Chart) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Карта № %s", c.Number)
	if c.IntNo != "" {
		fmt.Fprintf(&b, " (INT %s)", c.IntNo)
	}
	b.WriteString("\n")
	if c.TitleRU != "" {
		fmt.Fprintf(&b, "Название: %s\n", c.TitleRU)
	}
	if c.TitleEN != "" {
		fmt.Fprintf(&b, "Title: %s\n", c.TitleEN)
	}
	region := c.RegionRU
	if region == "" {
		region = c.RegionEN
	}
	if region != "" {
		fmt.Fprintf(&b, "Регион: %s\n", region)
	}
	if c.Scale > 0 {
		fmt.Fprintf(&b, "Масштаб: %s (%s)\n", charts.FormatScale(c.Scale), charts.Kind(c.Scale))
	}
	if c.Year > 0 {
		fmt.Fprintf(&b, "Год издания: %d (по каталогу)\n", c.Year)
	} else {
		b.WriteString("Год издания: нет данных\n")
	}
	if cov := charts.Coverage(c); cov != "" {
		fmt.Fprintf(&b, "Покрытие: %s\n", cov)
	}
	if c.Source != "" {
		fmt.Fprintf(&b, "Источник: %s", c.Source)
	}
	return strings.TrimSpace(b.String())
}
