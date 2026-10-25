package weather

import (
	"math"
	"testing"
)

func TestVisibilityToCables(t *testing.T) {
	cases := []struct {
		m, want float64
	}{
		{0, 0},
		{185.2, 1},
		{1852, 10},     // 1 морская миля = 10 кабельтовых
		{18520, 100},   // 10 миль
		{42800, 231.1}, // реальное значение Open-Meteo для Финского залива
	}
	for _, c := range cases {
		got := math.Round(VisibilityToCables(c.m)*10) / 10
		if got != c.want {
			t.Errorf("VisibilityToCables(%v) = %v, want %v", c.m, got, c.want)
		}
	}
	if FormatCables(VisibilityToCables(1000)) != "5.4" {
		t.Errorf("FormatCables(1000 м) = %s", FormatCables(VisibilityToCables(1000)))
	}
	if math.Abs(CablesToVisibility(VisibilityToCables(12345))-12345) > 1e-9 {
		t.Error("обратный пересчёт")
	}
}

func TestBeaufortBoundaries(t *testing.T) {
	cases := []struct {
		ms   float64
		want int
	}{
		{-1, 0}, {0, 0}, {0.29, 0},
		{0.3, 1}, {1.5, 1},
		{1.6, 2}, {3.3, 2},
		{3.4, 3}, {5.4, 3},
		{5.5, 4}, {6.5, 4}, {7.9, 4},
		{8.0, 5}, {10.7, 5},
		{10.8, 6}, {13.8, 6},
		{13.9, 7}, {17.1, 7},
		{17.2, 8}, {20.7, 8},
		{20.8, 9}, {24.4, 9},
		{24.5, 10}, {28.4, 10},
		{28.5, 11}, {32.6, 11},
		{32.7, 12}, {60, 12},
	}
	for _, c := range cases {
		if got := Beaufort(c.ms); got != c.want {
			t.Errorf("Beaufort(%v) = %d, want %d", c.ms, got, c.want)
		}
	}
	if BeaufortName(0) != "штиль" || BeaufortName(12) != "ураган" || BeaufortName(99) != "ураган" {
		t.Error("названия баллов Бофорта")
	}
	if len(beaufortNames) != 13 {
		t.Errorf("должно быть 13 названий, есть %d", len(beaufortNames))
	}
}

func TestWaveBall(t *testing.T) {
	cases := []struct {
		m    float64
		want int
	}{
		{0, 0}, {-0.5, 0},
		{0.05, 1},
		{0.1, 2}, {0.38, 2},
		{0.5, 3}, {1.2, 3},
		{1.25, 4}, {2.4, 4},
		{2.5, 5}, {3.9, 5},
		{4.0, 6}, {5.9, 6},
		{6.0, 7}, {8.9, 7},
		{9.0, 8}, {13.9, 8},
		{14.0, 9}, {20, 9},
	}
	for _, c := range cases {
		if got := WaveBall(c.m); got != c.want {
			t.Errorf("WaveBall(%v) = %d, want %d", c.m, got, c.want)
		}
	}
	if WaveName(0) != "зеркальное" || WaveName(9) != "чудовищные" {
		t.Error("названия баллов волнения")
	}
}

func TestNaNSafe(t *testing.T) {
	if Beaufort(math.NaN()) != 0 || WaveBall(math.NaN()) != 0 {
		t.Error("NaN должен давать 0")
	}
}
