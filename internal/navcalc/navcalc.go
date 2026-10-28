// Package navcalc — штурманские расчёты для режимов калькулятора v3.
// Время везде вводится и выводится в формате «часы:минуты» (timecalc).
package navcalc

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"ru.navhelper/internal/timecalc"
)

// Константы перевода.
const (
	NMkm     = 1.852 // морская миля, км
	Cable    = 0.1   // кабельтов, миль
	KnotMS   = 1852.0 / 3600
	FootM    = 0.3048
	FathomM  = 1.8288
	StatMile = 1.609344 // сухопутная миля, км
)

// ParseNum разбирает число: запятая или точка, пробелы игнорируются.
func ParseNum(s string) (float64, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, "−", "-")
	if s == "" {
		return 0, errors.New("пустое значение")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("неверное число «%s»", s)
	}
	return v, nil
}

// ParseHM — время «ч:мм» (допускается и «ч:мм:сс», и просто «ч»).
// В отличие от timecalc.Parse, число без двоеточия считается часами.
func ParseHM(s string) (timecalc.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("не указано время")
	}
	if !strings.Contains(s, ":") {
		h, err := ParseNum(s)
		if err != nil {
			return 0, err
		}
		return timecalc.Duration(math.Round(h * 3600)), nil
	}
	d, err := timecalc.Parse(s)
	if err != nil {
		return 0, err
	}
	return d, nil
}

// Hours — длительность в часах.
func Hours(d timecalc.Duration) float64 { return float64(d) / 3600 }

// FromHours — часы → длительность (округление до секунды).
func FromHours(h float64) timecalc.Duration { return timecalc.Duration(math.Round(h * 3600)) }

// HM — «ч:мм» с округлением до минуты.
func HM(d timecalc.Duration) string {
	m := int64(math.Round(float64(d) / 60))
	sign := ""
	if m < 0 {
		sign, m = "−", -m
	}
	return fmt.Sprintf("%s%d:%02d", sign, m/60, m%60)
}

// Distance — пройденное расстояние, мили: S = V·t.
func Distance(speedKn float64, t timecalc.Duration) float64 { return speedKn * Hours(t) }

// Speed — скорость, уз: V = S/t.
func Speed(distNM float64, t timecalc.Duration) (float64, error) {
	if t <= 0 {
		return 0, errors.New("время должно быть больше нуля")
	}
	return distNM / Hours(t), nil
}

// TravelTime — время на переход: t = S/V.
func TravelTime(distNM, speedKn float64) (timecalc.Duration, error) {
	if speedKn <= 0 {
		return 0, errors.New("скорость должна быть больше нуля")
	}
	return FromHours(distNM / speedKn), nil
}

// ParseClock — момент «ДД.ММ ЧЧ:ММ», «ДД.ММ.ГГГГ ЧЧ:ММ» или «ЧЧ:ММ» (сегодня).
func ParseClock(s string, now time.Time) (time.Time, error) {
	s = strings.Join(strings.Fields(s), " ")
	for _, l := range []string{"02.01.2006 15:04", "2.1.2006 15:04"} {
		if t, err := time.ParseInLocation(l, s, now.Location()); err == nil {
			return t, nil
		}
	}
	for _, l := range []string{"02.01 15:04", "2.1 15:04"} {
		if t, err := time.ParseInLocation(l, s, now.Location()); err == nil {
			return time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), nil
		}
	}
	for _, l := range []string{"15:04", "15.04"} {
		if t, err := time.ParseInLocation(l, s, now.Location()); err == nil {
			return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), nil
		}
	}
	return time.Time{}, fmt.Errorf("неверный момент «%s» — нужно ЧЧ:ММ или ДД.ММ ЧЧ:ММ", s)
}

// ETA — расчётное время прибытия.
func ETA(dep time.Time, distNM, speedKn float64) (time.Time, timecalc.Duration, error) {
	t, err := TravelTime(distNM, speedKn)
	if err != nil {
		return time.Time{}, 0, err
	}
	return dep.Add(time.Duration(t) * time.Second), t, nil
}

// RequiredSpeed — скорость, чтобы прийти к сроку.
func RequiredSpeed(distNM float64, from, to time.Time) (float64, timecalc.Duration, error) {
	t := timecalc.Duration(to.Sub(from) / time.Second)
	v, err := Speed(distNM, t)
	return v, t, err
}

// Fuel — расход топлива: часовой расход × время.
func Fuel(perHour float64, t timecalc.Duration) float64 { return perHour * Hours(t) }

// Current — скорость и путевой угол над грунтом при течении (векторное сложение).
// course — курс (истинный), deg; current set — направление течения («куда»), deg.
func Current(speed, course, curSpeed, curSet float64) (sog, cog float64) {
	r := math.Pi / 180
	vx := speed*math.Sin(course*r) + curSpeed*math.Sin(curSet*r)
	vy := speed*math.Cos(course*r) + curSpeed*math.Cos(curSet*r)
	sog = math.Hypot(vx, vy)
	cog = math.Mod(math.Atan2(vx, vy)/r+360, 360)
	return
}

// Drift — угол сноса течением (ПУ − ИК), со знаком: «+» вправо.
func Drift(course, cog float64) float64 {
	d := math.Mod(cog-course+540, 360) - 180
	return d
}

// Horizon — дальность видимого горизонта, мили: Dе = 2,08·√h (h — высота глаза, м).
func Horizon(eyeM float64) float64 {
	if eyeM < 0 {
		return 0
	}
	return 2.08 * math.Sqrt(eyeM)
}

// Visibility — дальность видимости предмета, мили: Dп = 2,08(√e + √h).
func Visibility(eyeM, objM float64) float64 {
	return Horizon(eyeM) + Horizon(objM)
}

// Format — число с запятой, без лишних нулей.
func Format(v float64, prec int) string {
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return strings.ReplaceAll(s, ".", ",")
}

// ─────────────────────────── перевод единиц ───────────────────────────

// Unit — единица измерения в группе (в базовых единицах группы).
type Unit struct {
	Name string
	K    float64 // множитель к базовой единице
}

// UnitGroup — группа взаимно переводимых единиц.
type UnitGroup struct {
	Name  string
	Units []Unit
}

// UnitGroups — расстояние (база — метр), скорость (м/с), глубина/высота (м).
var UnitGroups = []UnitGroup{
	{"Расстояние", []Unit{
		{"мор. мили", 1852}, {"кабельтовы", 185.2}, {"км", 1000}, {"м", 1},
		{"сух. мили", 1609.344},
	}},
	{"Скорость", []Unit{
		{"узлы", KnotMS}, {"км/ч", 1000.0 / 3600}, {"м/с", 1}, {"миль/ч (сух.)", 1609.344 / 3600},
	}},
	{"Глубина / высота", []Unit{
		{"м", 1}, {"футы", FootM}, {"морские сажени", FathomM},
	}},
}

// Convert переводит v из единицы from в to внутри группы.
func Convert(g UnitGroup, v float64, from, to string) (float64, error) {
	var kf, kt float64
	for _, u := range g.Units {
		if u.Name == from {
			kf = u.K
		}
		if u.Name == to {
			kt = u.K
		}
	}
	if kf == 0 || kt == 0 {
		return 0, errors.New("неизвестная единица")
	}
	return v * kf / kt, nil
}
