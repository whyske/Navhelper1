// Package weather получает реальные метеорологические данные (Open-Meteo)
// и переводит их в морские единицы: кабельтовы, баллы Бофорта, баллы волнения.
package weather

import (
	"fmt"
	"math"
)

// CableLengthM — длина одного кабельтова в метрах (1 каб = 185,2 м).
const CableLengthM = 185.2

// beaufortUpper — нижние границы баллов Бофорта в м/с (шкала ВМО).
// Индекс элемента = балл, значение = скорость ветра, с которой начинается балл.
var beaufortUpper = []float64{
	0.3,  // 1
	1.6,  // 2
	3.4,  // 3
	5.5,  // 4
	8.0,  // 5
	10.8, // 6
	13.9, // 7
	17.2, // 8
	20.8, // 9
	24.5, // 10
	28.5, // 11
	32.7, // 12
}

// beaufortNames — русские названия силы ветра по шкале Бофорта.
var beaufortNames = []string{
	"штиль",
	"тихий",
	"лёгкий",
	"слабый",
	"умеренный",
	"свежий",
	"сильный",
	"крепкий",
	"очень крепкий",
	"штормовой",
	"сильный шторм",
	"жестокий шторм",
	"ураган",
}

// waveUpper — верхние границы высоты значительной волны (м) для баллов
// волнения 0..8 по шкале ВМО 3700 (Дугласа). Всё, что выше — 9 баллов.
var waveUpper = []float64{
	0.1,  // 1 — рябь
	0.5,  // 2 — лёгкое волнение
	1.25, // 3 — небольшое
	2.5,  // 4 — умеренное
	4.0,  // 5 — крупное
	6.0,  // 6 — высокое
	9.0,  // 7 — очень высокое
	14.0, // 8 — гигантское
}

// VisibilityToCables переводит видимость из метров в кабельтовы.
func VisibilityToCables(meters float64) float64 {
	return meters / CableLengthM
}

// CablesToVisibility переводит кабельтовы в метры.
func CablesToVisibility(cables float64) float64 {
	return cables * CableLengthM
}

// Beaufort переводит скорость ветра из м/с в баллы Бофорта (0..12).
func Beaufort(ms float64) int {
	if ms < 0 || math.IsNaN(ms) {
		return 0
	}
	for i, upper := range beaufortUpper {
		if ms < upper {
			return i
		}
	}
	return 12
}

// BeaufortName возвращает название балла Бофорта.
func BeaufortName(b int) string {
	if b < 0 {
		b = 0
	}
	if b >= len(beaufortNames) {
		b = len(beaufortNames) - 1
	}
	return beaufortNames[b]
}

// WaveBall переводит высоту значительной волны (м) в балл волнения (0..9).
func WaveBall(meters float64) int {
	if meters <= 0 || math.IsNaN(meters) {
		return 0
	}
	for i, upper := range waveUpper {
		if meters < upper {
			return i + 1
		}
	}
	return 9
}

// WaveName возвращает название балла волнения.
func WaveName(b int) string {
	names := []string{
		"зеркальное", "рябь", "лёгкие волны", "небольшие волны", "умеренные волны",
		"крупные волны", "высокие волны", "очень высокие", "гигантские", "чудовищные",
	}
	if b < 0 {
		b = 0
	}
	if b >= len(names) {
		b = len(names) - 1
	}
	return names[b]
}

// FormatCables форматирует видимость в кабельтовых (ровно 1 знак после запятой).
func FormatCables(cables float64) string {
	return fmt.Sprintf("%.1f", math.Round(cables*10)/10)
}

// MetersSince видимость в метрах по кабельтовым — для обратного пересчёта в тестах.
func MetersSince(cables float64) float64 { return cables * CableLengthM }
