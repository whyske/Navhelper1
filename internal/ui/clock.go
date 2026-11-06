// Раздел «Часы»: настенный циферблат с бегущими стрелками и выбор города
// с восходом/заходом солнца. Стрелки идут по московскому времени.
package ui

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/sun"
)

var moscowTZ = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Moscow"); err == nil {
		return l
	}
	return time.FixedZone("MSK", 3*3600)
}()

// analogClock — круглый циферблат с часовой, минутной и секундной стрелками.
type analogClock struct {
	widget.BaseWidget
	t    time.Time
	size fyne.Size

	face   *canvas.Circle
	ticks  []*canvas.Line
	hour   *canvas.Line
	minute *canvas.Line
	second *canvas.Line
	hub    *canvas.Circle
}

func newAnalogClock() *analogClock {
	c := &analogClock{t: time.Now().In(moscowTZ)}
	c.ExtendBaseWidget(c)
	c.face = canvas.NewCircle(color.NRGBA{0xff, 0xff, 0xff, 0xff})
	c.face.StrokeColor = color.NRGBA{0x1f, 0x5f, 0x9e, 0xff}
	c.face.StrokeWidth = 4
	for i := 0; i < 12; i++ {
		c.ticks = append(c.ticks, canvas.NewLine(color.NRGBA{0x1b, 0x26, 0x33, 0xff}))
	}
	c.hour = canvas.NewLine(color.NRGBA{0x1b, 0x26, 0x33, 0xff})
	c.hour.StrokeWidth = 7
	c.minute = canvas.NewLine(color.NRGBA{0x1b, 0x26, 0x33, 0xff})
	c.minute.StrokeWidth = 5
	c.second = canvas.NewLine(color.NRGBA{0xc0, 0x30, 0x20, 0xff})
	c.second.StrokeWidth = 2
	c.hub = canvas.NewCircle(color.NRGBA{0xc0, 0x30, 0x20, 0xff})
	return c
}

// SetTime переставляет стрелки.
func (c *analogClock) SetTime(t time.Time) {
	c.t = t
	c.redraw()
}

func (c *analogClock) CreateRenderer() fyne.WidgetRenderer { return &analogClockRenderer{c: c} }

func (c *analogClock) allObjects() []fyne.CanvasObject {
	out := []fyne.CanvasObject{c.face}
	for _, ln := range c.ticks {
		out = append(out, ln)
	}
	return append(out, c.hour, c.minute, c.second, c.hub)
}

func (c *analogClock) redraw() {
	s := c.size
	if s.Width <= 0 || s.Height <= 0 {
		return
	}
	cx, cy := s.Width/2, s.Height/2
	r := float32(math.Min(float64(s.Width), float64(s.Height))) / 2 * 0.92

	c.face.Move(fyne.NewPos(cx-r, cy-r))
	c.face.Resize(fyne.NewSize(r*2, r*2))

	pt := func(angleDeg float64, length float32) fyne.Position {
		a := (angleDeg - 90) * math.Pi / 180
		return fyne.NewPos(cx+float32(math.Cos(a))*length, cy+float32(math.Sin(a))*length)
	}
	for i, ln := range c.ticks {
		ang := float64(i) * 30
		inner := r * 0.80
		if i%3 == 0 {
			inner = r * 0.72
			ln.StrokeWidth = 3
		} else {
			ln.StrokeWidth = 1.5
		}
		ln.Position1 = pt(ang, r*0.92)
		ln.Position2 = pt(ang, inner)
	}

	h := float64(c.t.Hour()%12) + float64(c.t.Minute())/60
	m := float64(c.t.Minute()) + float64(c.t.Second())/60

	c.hour.Position1, c.hour.Position2 = fyne.NewPos(cx, cy), pt(h*30, r*0.50)
	c.minute.Position1, c.minute.Position2 = fyne.NewPos(cx, cy), pt(m*6, r*0.72)
	c.second.Position1, c.second.Position2 = fyne.NewPos(cx, cy), pt(float64(c.t.Second())*6, r*0.80)

	hr := r * 0.06
	c.hub.Move(fyne.NewPos(cx-hr, cy-hr))
	c.hub.Resize(fyne.NewSize(hr*2, hr*2))

	for _, o := range c.allObjects() {
		o.Refresh()
	}
}

type analogClockRenderer struct{ c *analogClock }

func (r *analogClockRenderer) Layout(size fyne.Size) {
	r.c.size = size
	r.c.redraw()
}
func (r *analogClockRenderer) MinSize() fyne.Size { return fyne.NewSize(220, 220) }
func (r *analogClockRenderer) Refresh()           { r.Layout(r.c.size) }
func (r *analogClockRenderer) Destroy()           {}
func (r *analogClockRenderer) Objects() []fyne.CanvasObject {
	return r.c.allObjects()
}

// ─────────────────────────────────────────────────────────── раздел «Часы» ──

// approxZone — приблизительный часовой пояс точки по её долготе
// (округление до получаса). Нужен для показа местного времени города.
func approxZone(lon float64) *time.Location {
	off := math.Round(lon/15*2) / 2
	return time.FixedZone(fmt.Sprintf("UTC%+g", off), int(off*3600))
}

func newClockSection(a *App) fyne.CanvasObject {
	clock := newAnalogClock()

	mskText := canvas.NewText("", color.NRGBA{0x1b, 0x26, 0x33, 0xff})
	mskText.TextStyle = fyne.TextStyle{Bold: true, Monospace: true}
	mskText.TextSize = theme.TextSize() * 1.9
	mskText.Alignment = fyne.TextAlignCenter

	dateText := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{})
	dateText.Importance = widget.LowImportance

	sunTitle := bold("")
	sunInfo := wrapLabel("")

	names := make([]string, 0, len(wjPresets))
	for k := range wjPresets {
		names = append(names, k)
	}
	sort.Strings(names)

	cur := a.St.Setting("clock.city", wjDefaultName)
	if _, ok := wjPresets[cur]; !ok {
		cur = wjDefaultName
	}

	updateSun := func() {
		p, ok := wjPresets[cur]
		if !ok {
			return
		}
		lat, lon := p[0], p[1]
		cityLoc := approxZone(lon)
		now := time.Now().In(cityLoc)
		r := sun.Calculate(now, lat, lon, cityLoc)

		sunTitle.SetText(cur)
		switch {
		case r.PolarDay:
			sunInfo.SetText("Полярный день — солнце не заходит за горизонт.")
		case r.PolarNight:
			sunInfo.SetText("Полярная ночь — солнце не показывается из-за горизонта.")
		case r.HasSunrise && r.HasSunset:
			h := int(r.DayLength.Hours())
			mn := int(r.DayLength.Minutes()) % 60
			sunInfo.SetText(fmt.Sprintf(
				"Восход: %s\nЗаход: %s\nСветовой день: %d ч %d мин\n"+
					"Местное время города: %s (приблизительно по долготе)",
				r.Sunrise.Format("15:04"), r.Sunset.Format("15:04"), h, mn,
				now.Format("15:04")))
		default:
			sunInfo.SetText("Нет данных для этой точки.")
		}
	}

	citySel := widget.NewSelect(names, func(s string) {
		cur = s
		_ = a.St.SetSetting("clock.city", s)
		updateSun()
	})
	citySel.PlaceHolder = "Выбрать город…"
	citySel.SetSelected(cur)
	updateSun()

	tick := func() {
		now := time.Now()
		msk := now.In(moscowTZ)
		clock.SetTime(msk)
		mskText.Text = msk.Format("15:04:05")
		mskText.Refresh()
		dateText.SetText(msk.Format("2 января 2006, ") + weekdayRU(msk) + " · московское время")
	}
	tick()
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for range t.C {
			fyne.Do(tick)
		}
	}()

	header := container.NewVBox(
		container.NewCenter(container.NewGridWrap(fyne.NewSize(250, 250), clock)),
		container.NewCenter(mskText),
		container.NewCenter(dateText),
		widget.NewSeparator(),
		bold("Восход и заход солнца"),
		citySel,
		sunTitle,
		sunInfo,
	)
	return container.NewVScroll(container.NewPadded(header))
}

// weekdayRU — день недели по-русски для date-строки.
func weekdayRU(t time.Time) string {
	days := []string{"воскресенье", "понедельник", "вторник", "среда",
		"четверг", "пятница", "суббота"}
	return days[int(t.Weekday())]
}
