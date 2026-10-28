package charts

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// Цвета превью — по образцу navysoft.ru: листы карт залиты голубым
// (#99CCFF в каталоге, выбранный #66B2FF), море светлое, суша «бумажная».
var (
	PreviewSea    = color.NRGBA{0xd7, 0xe6, 0xf5, 0xff} // море
	PreviewLand   = color.NRGBA{0xf7, 0xf3, 0xe8, 0xff} // суша
	PreviewCoast  = color.NRGBA{0x8f, 0xa6, 0xbb, 0xff} // береговая линия
	PreviewSheet  = color.NRGBA{0x99, 0xcc, 0xff, 0x55} // соседние листы
	PreviewSheetE = color.NRGBA{0x5f, 0x9e, 0xd8, 0xff} // их рамка
	PreviewSel    = color.NRGBA{0x66, 0xb2, 0xff, 0x99} // выбранный лист
	PreviewSelE   = color.NRGBA{0x00, 0x6d, 0xc8, 0xff} // его рамка
)

// Viewport — окно просмотра превью в градусах.
type Viewport struct {
	LonMin, LatMin, LonMax, LatMax float64
}

// Center возвращает центр окна.
func (v Viewport) Center() (lon, lat float64) {
	return (v.LonMin + v.LonMax) / 2, (v.LatMin + v.LatMax) / 2
}

// FitView строит окно вокруг листа карты с полями pad (доля размера листа).
// Соотношение сторон подгоняется под w/h с учётом сжатия параллелей.
func FitView(c Chart, w, h int, pad float64) Viewport {
	latC := (c.LatMin + c.LatMax) / 2
	lonC := (c.LonMin + c.LonMax) / 2
	latSpan := math.Max((c.LatMax-c.LatMin)*(1+2*pad), 0.004)
	aspect := float64(w) / float64(h)
	cos := math.Cos(latC * math.Pi / 180)
	if cos < 0.05 {
		cos = 0.05
	}
	lonSpan := math.Max((c.LonMax-c.LonMin)*(1+2*pad), latSpan*aspect/cos)
	// Растягиваем меньшую сторону, чтобы лист не обрезался при любом соотношении.
	if lonSpan/latSpan < aspect*0.9999 {
		lonSpan = latSpan * aspect
	} else {
		latSpan = lonSpan / aspect
	}
	return Viewport{
		LonMin: lonC - lonSpan/2, LonMax: lonC + lonSpan/2,
		LatMin: math.Max(latC-latSpan/2, -89), LatMax: math.Min(latC+latSpan/2, 89),
	}
}

// project — равнопромежуточная проекция окна в пиксели.
type proj struct {
	v      Viewport
	w, h   float64
	x0, y0 float64
}

func newProj(v Viewport, w, h int) proj {
	return proj{v: v, w: float64(w), h: float64(h),
		x0: v.LonMin, y0: v.LatMax}
}

func (p proj) xy(lon, lat float64) (float32, float32) {
	return float32((lon - p.x0) / (p.v.LonMax - p.v.LonMin) * p.w),
		float32((p.y0 - lat) / (p.v.LatMax - p.v.LatMin) * p.h)
}

// RenderPreview рисует превью: море, сушу, соседние листы и выбранный лист.
func RenderPreview(dst *image.RGBA, v Viewport, sel Chart, near []Chart) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	draw.Draw(dst, dst.Bounds(), image.NewUniform(PreviewSea), image.Point{}, draw.Src)

	// Суша: заливка non-zero по кольцам (внешние CCW, дыры CW).
	rings, err := LandIn(v.LonMin, v.LatMin, v.LonMax, v.LatMax)
	if err == nil && len(rings) > 0 {
		p := newProj(v, w, h)
		r := vector.NewRasterizer(w, h)
		r.DrawOp = draw.Src
		for _, ring := range rings {
			if len(ring) < 3 {
				continue
			}
			x, y := p.xy(ring[0][0], ring[0][1])
			r.MoveTo(x, y)
			for _, pt := range ring[1:] {
				px, py := p.xy(pt[0], pt[1])
				r.LineTo(px, py)
			}
			r.ClosePath()
		}
		r.Draw(dst, dst.Bounds(), image.NewUniform(PreviewLand), image.Point{})
		strokeRings(dst, p, rings, PreviewCoast, 1.0)
	}

	// Соседние листы каталога.
	for _, c := range near {
		fillSheet(dst, v, c, PreviewSheet, PreviewSheetE, 1.0)
	}
	fillSheet(dst, v, sel, PreviewSel, PreviewSelE, 2.2)
	cornerMarks(dst, v, sel, PreviewSelE)
}

// fillSheet заливает прямоугольник листа и обводит его.
func fillSheet(dst *image.RGBA, v Viewport, c Chart, fill, edge color.Color, edgeW float32) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	p := newProj(v, w, h)
	x1, y1 := p.xy(c.LonMin, c.LatMax)
	x2, y2 := p.xy(c.LonMax, c.LatMin)
	if x2 < 0 || y2 < 0 || x1 > float32(w) || y1 > float32(h) {
		return
	}
	r := vector.NewRasterizer(w, h)
	r.DrawOp = draw.Over
	r.MoveTo(x1, y1)
	r.LineTo(x2, y1)
	r.LineTo(x2, y2)
	r.LineTo(x1, y2)
	r.ClosePath()
	r.Draw(dst, dst.Bounds(), image.NewUniform(fill), image.Point{})

	if edgeW > 0 {
		strokeRect(dst, x1, y1, x2, y2, edge, edgeW)
	}
}

// cornerMarks рисует «уголки» вокруг выбранного листа — так выделен лист на
// navysoft, и это читается даже когда лист мелкий на экране.
func cornerMarks(dst *image.RGBA, v Viewport, c Chart, col color.Color) {
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	p := newProj(v, w, h)
	x1, y1 := p.xy(c.LonMin, c.LatMax)
	x2, y2 := p.xy(c.LonMax, c.LatMin)
	const minArm = 7.0
	armX := math.Min(math.Max(float64(x2-x1)*0.35, minArm), 16)
	armY := math.Min(math.Max(float64(y2-y1)*0.35, minArm), 16)
	// Внешний контур на 3 пикселя шире листа, чтобы уголки не сливались с рамкой.
	ox, oy := float32(3), float32(3)
	L, T, R, B := x1-ox, y1-oy, x2+ox, y2+oy
	for _, s := range []struct {
		x, y, dx, dy float32
	}{
		{L, T, float32(armX), 0}, {L, T, 0, float32(armY)},
		{R, T, -float32(armX), 0}, {R, T, 0, float32(armY)},
		{L, B, float32(armX), 0}, {L, B, 0, -float32(armY)},
		{R, B, -float32(armX), 0}, {R, B, 0, -float32(armY)},
	} {
		if s.x < -20 || s.x > float32(w)+20 || s.y < -20 || s.y > float32(h)+20 {
			continue
		}
		line(dst, s.x, s.y, s.x+s.dx, s.y+s.dy, col, 2.2)
	}
}

func strokeRect(dst *image.RGBA, x1, y1, x2, y2 float32, col color.Color, wd float32) {
	line(dst, x1, y1, x2, y1, col, wd)
	line(dst, x2, y1, x2, y2, col, wd)
	line(dst, x2, y2, x1, y2, col, wd)
	line(dst, x1, y2, x1, y1, col, wd)
}

// strokeRings обводит кольца суши тонкой линией.
func strokeRings(dst *image.RGBA, p proj, rings []Ring, col color.Color, wd float32) {
	for _, ring := range rings {
		if len(ring) < 2 {
			continue
		}
		prevX, prevY := p.xy(ring[0][0], ring[0][1])
		for _, pt := range ring[1:] {
			x, y := p.xy(pt[0], pt[1])
			// Отбрасываем очень длинные «прыжки» через весь экран: это разрывы
			// колец после упрощения, а не реальный берег.
			dx, dy := x-prevX, y-prevY
			if dx*dx+dy*dy < 1e6 {
				line(dst, prevX, prevY, x, y, col, wd)
			}
			prevX, prevY = x, y
		}
	}
}

// line рисует отрезок толщиной wd с альфа-смешиванием.
func line(dst *image.RGBA, x1, y1, x2, y2 float32, col color.Color, wd float32) {
	if wd < 1 {
		wd = 1
	}
	dx, dy := x2-x1, y2-y1
	steps := int(math.Hypot(float64(dx), float64(dy)))
	if steps > 4000 {
		return
	}
	n := float32(steps)
	if n < 1 {
		n = 1
	}
	r := int(wd/2 + 0.5)
	for i := 0; i <= steps; i++ {
		t := float32(i) / n
		cx := int(x1 + dx*t)
		cy := int(y1 + dy*t)
		for yy := cy - r; yy <= cy+r; yy++ {
			for xx := cx - r; xx <= cx+r; xx++ {
				blend(dst, xx, yy, col)
			}
		}
	}
}

func blend(dst *image.RGBA, x, y int, col color.Color) {
	if !(image.Point{x, y}).In(dst.Rect) {
		return
	}
	cr, cg, cb, ca := col.RGBA()
	if ca == 0 {
		return
	}
	i := dst.PixOffset(x, y)
	d := dst.Pix[i : i+4 : i+4]
	a := float64(ca) / 65535
	// RGBA() возвращает предумноженные компоненты.
	dst.Pix[i+0] = uint8(float64(cr>>8) + float64(d[0])*(1-a))
	dst.Pix[i+1] = uint8(float64(cg>>8) + float64(d[1])*(1-a))
	dst.Pix[i+2] = uint8(float64(cb>>8) + float64(d[2])*(1-a))
	dst.Pix[i+3] = 0xff
}
