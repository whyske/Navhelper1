// Package forecast — морской прогноз погоды в формате судового журнала
// и расчёт его оправдываемости по реальным наблюдениям.
//
// Источники (все данные реальные, ничего не досчитывается «на глаз»):
//   - прогноз: Open-Meteo Forecast API (почасовой прогноз модели);
//   - прогноз «как он был выпущен» для прошедших суток: Open-Meteo
//     Previous Runs API (переменные *_previous_day1 — прогноз сутки назад);
//   - волнение: Open-Meteo Marine API (прогноз и *_previous_day1);
//   - факт: METAR ближайшего аэропорта (aviationweather.gov) — реальные
//     наблюдения: ветер, порывы, видимость, явления, температура;
//   - факт по волнению: Marine API (реанализ/анализ модели) — помечается
//     в интерфейсе как «по модели», потому что приборного наблюдения волн
//     в открытом доступе нет.
package forecast

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// CableM — длина морского кабельтова, м (0,1 морской мили).
const CableM = 185.2

// VisMaxM — потолок видимости. METAR кодирует «10 км и более» как 9999,
// а модели над морем выдают 30–60 км, что для журнала неправдоподобно.
// Всё, что ≥ 10 км, считаем «50 кбт и более» (≈ 9,26 км = 50 кбт).
const VisMaxM = 10000.0

// ─────────────────────────── румбы ───────────────────────────

var rhumbShort = []string{"С", "СВ", "В", "ЮВ", "Ю", "ЮЗ", "З", "СЗ"}
var rhumbAdj = []string{
	"северный", "северо-восточный", "восточный", "юго-восточный",
	"южный", "юго-западный", "западный", "северо-западный",
}

// Rhumb возвращает индекс восьмирумбового направления 0..7 для угла,
// ОТКУДА дует ветер (метеорологическая конвенция).
func Rhumb(deg float64) int {
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return int(math.Floor((d+22.5)/45)) % 8
}

// RhumbShort — «СЗ», «ЮВ» ...
func RhumbShort(deg float64) string { return rhumbShort[Rhumb(deg)] }

// RhumbName — «северо-западный» ...
func RhumbName(deg float64) string { return rhumbAdj[Rhumb(deg)] }

// AngleDiff — наименьшая разница двух направлений, 0..180°.
func AngleDiff(a, b float64) float64 {
	d := math.Abs(math.Mod(a-b, 360))
	if d > 180 {
		d = 360 - d
	}
	return d
}

// VectorMeanDir — преобладающее направление ветра как векторное среднее
// (взвешенное по скорости), чтобы 350° и 10° давали 0°, а не 180°.
// ok=false, если ветер в сумме не имеет направления (штиль/переменный).
func VectorMeanDir(dirs, speeds []float64) (float64, bool) {
	var u, v, w float64
	for i := range dirs {
		s := 1.0
		if i < len(speeds) {
			s = speeds[i]
		}
		r := dirs[i] * math.Pi / 180
		u += s * math.Sin(r)
		v += s * math.Cos(r)
		w += s
	}
	if w == 0 || math.Hypot(u, v) < 0.15*w {
		return 0, false
	}
	d := math.Atan2(u, v) * 180 / math.Pi
	if d < 0 {
		d += 360
	}
	return d, true
}

// ─────────────────────────── погода (WMO) ───────────────────────────

// Phen — явление погоды. Severity — порядок важности в тексте прогноза.
type Phen struct {
	Key      string // "rain", "snow", "fog", "drizzle", "showers", "thunder", "sleet"
	Text     string // по-русски: «небольшой дождь»
	Severity int
}

// Cloud — облачность 0..4: ясно, малооблачно, переменная, облачно, пасмурно.
var cloudText = []string{"ясно", "малооблачно", "переменная облачность", "облачно", "пасмурно"}

// CloudText — текст облачности по уровню 0..4.
func CloudText(c int) string {
	if c < 0 {
		c = 0
	}
	if c > 4 {
		c = 4
	}
	return cloudText[c]
}

// WMO разбирает код погоды WMO (Open-Meteo weather_code) на облачность
// и явление. Облачность для кодов с осадками принимается «облачно».
func WMO(code int) (cloud int, ph *Phen) {
	switch code {
	case 0:
		return 0, nil
	case 1:
		return 1, nil
	case 2:
		return 2, nil
	case 3:
		return 4, nil
	case 45:
		return 4, &Phen{"fog", "туман", 6}
	case 48:
		return 4, &Phen{"fog", "туман с изморозью", 6}
	case 51:
		return 4, &Phen{"drizzle", "слабая морось", 1}
	case 53:
		return 4, &Phen{"drizzle", "морось", 2}
	case 55:
		return 4, &Phen{"drizzle", "сильная морось", 3}
	case 56, 57:
		return 4, &Phen{"drizzle", "переохлаждённая морось", 5}
	case 61:
		return 4, &Phen{"rain", "небольшой дождь", 2}
	case 63:
		return 4, &Phen{"rain", "дождь", 3}
	case 65:
		return 4, &Phen{"rain", "сильный дождь", 4}
	case 66, 67:
		return 4, &Phen{"rain", "переохлаждённый дождь", 5}
	case 71:
		return 4, &Phen{"snow", "небольшой снег", 2}
	case 73:
		return 4, &Phen{"snow", "снег", 3}
	case 75:
		return 4, &Phen{"snow", "сильный снег", 4}
	case 77:
		return 4, &Phen{"snow", "снежные зёрна", 2}
	case 80:
		return 3, &Phen{"showers", "небольшой ливневый дождь", 2}
	case 81:
		return 3, &Phen{"showers", "ливневый дождь", 3}
	case 82:
		return 3, &Phen{"showers", "сильный ливень", 4}
	case 85:
		return 3, &Phen{"snow", "небольшой снегопад", 2}
	case 86:
		return 3, &Phen{"snow", "сильный снегопад", 4}
	case 95:
		return 3, &Phen{"thunder", "гроза", 7}
	case 96, 99:
		return 3, &Phen{"thunder", "гроза с градом", 8}
	}
	return 2, nil
}

// PrecipKey — является ли явление осадками.
func PrecipKey(k string) bool {
	switch k {
	case "rain", "drizzle", "snow", "showers", "sleet", "thunder":
		return true
	}
	return false
}

// ─────────────────────────── видимость ───────────────────────────

// VisCables — видимость, м → кабельтовы с округлением, как пишут в журнале:
// до 5 кбт — до 0,5; до 20 — до целых; дальше — до 5 кбт; потолок 50.
func VisCables(m float64) float64 {
	if m >= VisMaxM {
		return 50
	}
	c := m / CableM
	switch {
	case c < 5:
		return math.Round(c*2) / 2
	case c < 20:
		return math.Round(c)
	default:
		c = math.Round(c/5) * 5
		if c > 50 {
			c = 50
		}
		return c
	}
}

// FmtCab — «35 кбт», «0,5 кбт», «50 кбт и более».
func FmtCab(c float64) string {
	if c >= 50 {
		return "50 кбт и более"
	}
	if c != math.Trunc(c) {
		return strings.Replace(fmt.Sprintf("%.1f кбт", c), ".", ",", 1)
	}
	return fmt.Sprintf("%.0f кбт", c)
}

// FmtCabRange — «20–40 кбт», «35–50 кбт и более», «50 кбт и более».
func FmtCabRange(lo, hi float64) string {
	if lo >= 50 {
		return "50 кбт и более"
	}
	if lo == hi {
		return FmtCab(lo)
	}
	l := FmtCab(lo)
	l = strings.TrimSuffix(l, " кбт")
	return l + "–" + FmtCab(hi)
}

// ─────────────────────────── волнение ───────────────────────────

// waveUpper — верхние границы значительной волны (м) для баллов 1..8
// шкалы степени волнения ВМО 3700 (Дугласа). Выше — 9 баллов.
var waveUpper = []float64{0.1, 0.5, 1.25, 2.5, 4.0, 6.0, 9.0, 14.0}

// SeaState — балл волнения 0..9 по высоте значительной волны.
func SeaState(h float64) int {
	if h <= 0 || math.IsNaN(h) {
		return 0
	}
	for i, u := range waveUpper {
		if h < u {
			return i + 1
		}
	}
	return 9
}

// ─────────────────────────── статистика ───────────────────────────

// Percentile — p-й процентиль (0..100), линейная интерполяция.
func Percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	if len(s) == 1 {
		return s[0]
	}
	r := p / 100 * float64(len(s)-1)
	i := int(math.Floor(r))
	if i >= len(s)-1 {
		return s[len(s)-1]
	}
	return s[i] + (s[i+1]-s[i])*(r-float64(i))
}

func minMax(xs []float64) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, x := range xs {
		lo = math.Min(lo, x)
		hi = math.Max(hi, x)
	}
	return lo, hi
}

// FmtTemp — «+19°», «−3°», «0°».
func FmtTemp(t float64) string {
	r := int(math.Round(t))
	switch {
	case r > 0:
		return fmt.Sprintf("+%d°", r)
	case r < 0:
		return fmt.Sprintf("−%d°", -r)
	}
	return "0°"
}

// FmtTempRange — «+19…+22°», «−3…+1°», «+5°».
func FmtTempRange(lo, hi float64) string {
	l, h := int(math.Round(lo)), int(math.Round(hi))
	if l == h {
		return FmtTemp(lo)
	}
	return strings.TrimSuffix(FmtTemp(float64(l)), "°") + "…" + FmtTemp(float64(h))
}
