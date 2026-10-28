package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"ru.navhelper/internal/charts"
)

const (
	previewW = 340
	previewH = 260
)

// chartPreview рисует береговую линию вокруг листа карты и красной рамкой
// показывает зону самого листа (по координатам из каталога УНиО).
func chartPreview(c charts.Chart) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.NRGBA{0xe9, 0xf1, 0xf8, 0xff})
	bg.SetMinSize(fyne.NewSize(previewW, previewH))
	if c.LatMin == 0 && c.LatMax == 0 {
		return container.NewStack(bg, container.NewCenter(wrapLabel("Для этой карты в каталоге нет координат рамки.")))
	}

	latC := (c.LatMin + c.LatMax) / 2
	lonC := (c.LonMin + c.LonMax) / 2
	// Вид шире листа в 5 раз (минимум 1,5°), чтобы было видно берег вокруг.
	latSpan := math.Max((c.LatMax-c.LatMin)*5, 1.5)
	lonSpan := latSpan * float64(previewW) / float64(previewH) / math.Cos(latC*math.Pi/180)
	lonL, latT := lonC-lonSpan/2, latC+latSpan/2
	lonR, latB := lonL+lonSpan, latT-latSpan

	px := func(lon float64) float32 { return float32((lon - lonL) / lonSpan * previewW) }
	py := func(lat float64) float32 { return float32((latT - lat) / latSpan * previewH) }
	inView := func(p [2]float64) bool {
		return p[0] >= lonL && p[0] <= lonR && p[1] >= latB && p[1] <= latT
	}

	layer := container.NewWithoutLayout()
	if coast, err := charts.CoastLines(); err == nil {
		col := color.NRGBA{0x7f, 0x9c, 0xbb, 0xff}
		for _, pl := range coast {
			for i := 1; i < len(pl); i++ {
				a, b := pl[i-1], pl[i]
				if !inView(a) && !inView(b) {
					continue
				}
				ln := canvas.NewLine(col)
				ln.StrokeWidth = 1.2
				ln.Position1 = fyne.NewPos(px(a[0]), py(a[1]))
				ln.Position2 = fyne.NewPos(px(b[0]), py(b[1]))
				layer.Add(ln)
			}
		}
	}

	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeColor = color.NRGBA{0xc0, 0x30, 0x20, 0xff}
	frame.StrokeWidth = 2.5
	x1, x2 := px(c.LonMin), px(c.LonMax)
	y1, y2 := py(c.LatMax), py(c.LatMin)
	frame.Move(fyne.NewPos(x1, y1))
	frame.Resize(fyne.NewSize(x2-x1, y2-y1))
	layer.Add(frame)

	caption := wrapLabel("Красная рамка — зона листа " + c.Number + ".")
	return container.NewVBox(container.NewStack(bg, layer), caption)
}
