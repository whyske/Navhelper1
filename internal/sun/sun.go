// Package sun считает время восхода и захода Солнца по широте/долготе и дате.
// Используется уравнение восхода (NOAA / Sunrise equation) — точность около
// 1–2 минут, этого достаточно для справочного раздела «Часы».
// Вычисления полностью офлайн, сетевых запросов не требуется.
package sun

import (
	"math"
	"time"
)

// Times — результат расчёта на заданную календарную дату.
type Times struct {
	Sunrise    time.Time
	Sunset     time.Time
	HasSunrise bool
	HasSunset  bool
	DayLength  time.Duration
	PolarDay   bool // солнце не заходит
	PolarNight bool // солнце не восходит
}

const deg = math.Pi / 180

// julianDay — юлианская дата на полдень календарной даты y-m-d.
func julianDay(y, m, d int) float64 {
	if m <= 2 {
		y--
		m += 12
	}
	a := y / 100
	b := 2 - a + a/4
	return math.Floor(365.25*float64(y+4716)) + math.Floor(30.6001*float64(m+1)) + float64(d) + float64(b) - 1524.5
}

// fromJulian переводит дробную JD (UTC) в time.Time UTC.
func fromJulian(jd float64) time.Time {
	jd += 0.5
	z := math.Floor(jd)
	f := jd - z
	var a float64
	if z < 2299161 {
		a = z
	} else {
		alpha := math.Floor((z - 1867216.25) / 36524.25)
		a = z + 1 + alpha - math.Floor(alpha/4)
	}
	b := a + 1524
	c := math.Floor((b - 122.1) / 365.25)
	d := math.Floor(365.25 * c)
	e := math.Floor((b - d) / 30.6001)
	dayFrac := b - d - math.Floor(30.6001*e) + f
	day := math.Floor(dayFrac)
	hours := (dayFrac - day) * 24
	var month float64
	if e < 14 {
		month = e - 1
	} else {
		month = e - 13
	}
	var year float64
	if month > 2 {
		year = c - 4716
	} else {
		year = c - 4715
	}
	h := int(hours)
	min := int((hours - float64(h)) * 60)
	sec := int(((hours-float64(h))*60 - float64(min)) * 60)
	return time.Date(int(year), time.Month(int(month)), int(day), h, min, sec, 0, time.UTC)
}

// Calculate считает восход/заход для точки lat,lon (градусы, lon на восток)
// на дату date (берутся только год/месяц/день). Результат — в часовом поясе loc.
func Calculate(date time.Time, lat, lon float64, loc *time.Location) Times {
	jd := julianDay(date.Year(), int(date.Month()), date.Day())
	n := math.Round(jd - 2451545.0 + 0.0008)

	jstar := n - lon/360
	m := math.Mod(357.5291+0.98560028*jstar, 360)
	if m < 0 {
		m += 360
	}
	mr := m * deg
	c := 1.9148*math.Sin(mr) + 0.0200*math.Sin(2*mr) + 0.0003*math.Sin(3*mr)
	lambda := math.Mod(m+102.9372+c+180, 360)
	if lambda < 0 {
		lambda += 360
	}
	lr := lambda * deg

	jtransit := 2451545.0 + jstar + 0.0053*math.Sin(mr) - 0.0069*math.Sin(2*lr)

	sinDelta := math.Sin(lr) * math.Sin(23.4397*deg)
	delta := math.Asin(sinDelta)

	phi := lat * deg
	cosW0 := (math.Sin(-0.833*deg) - math.Sin(phi)*sinDelta) / (math.Cos(phi) * math.Cos(delta))

	var out Times
	switch {
	case cosW0 > 1:
		out.PolarNight = true
		return out
	case cosW0 < -1:
		out.PolarDay = true
		return out
	}
	w0 := math.Acos(cosW0) / deg

	out.Sunrise = fromJulian(jtransit - w0/360).In(loc)
	out.Sunset = fromJulian(jtransit + w0/360).In(loc)
	out.HasSunrise, out.HasSunset = true, true
	out.DayLength = out.Sunset.Sub(out.Sunrise)
	return out
}
