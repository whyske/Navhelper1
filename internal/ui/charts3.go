package ui

import (
	"database/sql"
	"fmt"
	"image"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/charts"
)

const (
	previewW = 360
	previewH = 300
)

// chartPreview — превью листа карты в стиле navysoft: подложка с морем и
// сушей, залитые голубым соседние листы каталога и ярко выделенный выбранный
// лист с уголками. Текст подписи переносится и не наезжает на картинку.
func chartPreview(db *sql.DB, c charts.Chart) fyne.CanvasObject {
	if c.LatMin == 0 && c.LatMax == 0 {
		l := wrapLabel("Для этой карты в каталоге нет координат рамки — превью недоступно.")
		l.Importance = widget.LowImportance
		return l
	}
	v := charts.FitView(c, previewW, previewH, 0.45)
	img := image.NewRGBA(image.Rect(0, 0, previewW, previewH))
	near, _ := charts.InBox(db, v.LonMin, v.LatMin, v.LonMax, v.LatMax, 0.18, 24)
	charts.RenderPreview(img, v, c, near)

	pic := canvas.NewImageFromImage(img)
	pic.FillMode = canvas.ImageFillContain
	pic.SetMinSize(fyne.NewSize(previewW, previewH))
	pic.ScaleMode = canvas.ImageScaleSmooth
	pic.CornerRadius = 8

	cap := wrapLabel("Синим выделен лист " + c.Number + "; голубые прямоугольники — соседние листы каталога.")
	cap.Importance = widget.LowImportance
	osm := widget.NewLabel("Береговая линия © участники OpenStreetMap (ODbL)")
	osm.Importance = widget.LowImportance
	osm.SizeName = theme.SizeNameCaptionText
	osm.Wrapping = fyne.TextWrapWord
	return container.NewVBox(pic, cap, osm)
}

// newChartsTab — раздел «Карты»: поиск по номеру/названию, карточка описания
// и превью листа.
func newChartsTab(a *App, count int, importErr error) fyne.CanvasObject {
	db := a.St.DB()
	info := wrapLabel(fmt.Sprintf("В каталоге карт: %d. %s", count, charts.Header(a.St.DB())))
	if importErr != nil {
		info.SetText(errText(importErr))
	}

	hint := wrapLabel("Введите номер карты, например 38188, или часть названия.")
	previewBox := container.NewVBox()
	details := widget.NewLabel("")
	details.Wrapping = fyne.TextWrapWord

	var found []charts.Chart
	list := widget.NewList(
		func() int { return len(found) },
		func() fyne.CanvasObject { return wrapLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			c := found[i]
			o.(*widget.Label).SetText(fmt.Sprintf("%s  %s  (%s, %d)",
				c.Number, c.TitleRU, charts.FormatScale(c.Scale), c.Year))
		},
	)
	show := func(c charts.Chart) {
		details.SetText(describeChart(c))
		previewBox.Objects = []fyne.CanvasObject{chartPreview(db, c)}
		previewBox.Refresh()
	}
	list.OnSelected = func(i widget.ListItemID) {
		show(found[i])
	}

	entry := widget.NewEntry()
	entry.SetPlaceHolder("Номер карты или название")
	search := func() {
		q := strings.TrimSpace(entry.Text)
		found = nil
		list.UnselectAll()
		previewBox.Objects = nil
		previewBox.Refresh()
		if c, ok := charts.Find(a.St.DB(), q); ok {
			found = []charts.Chart{c}
			show(c)
			hint.SetText("Карта № " + c.Number)
		} else {
			res, err := charts.Search(a.St.DB(), q, 100)
			switch {
			case err != nil:
				hint.SetText(errText(err))
			case len(res) == 0:
				hint.SetText("Карта «" + q + "» в каталоге не найдена.")
				details.SetText("")
			default:
				found = res
				details.SetText("")
				hint.SetText(fmt.Sprintf("Найдено: %d. Выберите карту в списке.", len(res)))
			}
		}
		list.Refresh()
	}
	entry.OnSubmitted = func(string) { search() }
	btn := widget.NewButton("Найти", search)
	btn.Importance = widget.HighImportance

	// Верхняя часть — в прокрутке: иначе на низком экране подпись каталога
	// налезала на карточку (баг v2).
	head := container.NewVBox(
		bold("Поиск морской карты УНиО"),
		container.NewBorder(nil, nil, nil, btn, entry),
		hint,
		details,
		previewBox,
		widget.NewSeparator(),
	)
	top := container.NewVScroll(head)
	top.SetMinSize(fyne.NewSize(previewW, 380))
	return container.NewBorder(top, container.NewPadded(info), nil, nil, list)
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
