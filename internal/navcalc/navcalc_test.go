package navcalc

import (
	"math"
	"testing"
	"time"
)

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestBasics(t *testing.T) {
	d, err := ParseHM("2:30")
	if err != nil || Hours(d) != 2.5 {
		t.Fatalf("ParseHM: %v %v", d, err)
	}
	d, _ = ParseHM("150:45")
	if HM(d) != "150:45" {
		t.Fatal(HM(d))
	}
	if s := Distance(12, d); !near(s, 1809, 0.01) {
		t.Fatalf("S=%v", s)
	}
	two, _ := ParseHM("2:30")
	v, _ := Speed(30, two)
	if v != 12 {
		t.Fatalf("V=%v", v)
	}
	tt, _ := TravelTime(100, 12)
	if HM(tt) != "8:20" {
		t.Fatal(HM(tt))
	}
	if _, err := Speed(10, 0); err == nil {
		t.Fatal("ждали ошибку")
	}
	if Format(12.50, 2) != "12,5" || Format(3, 2) != "3" {
		t.Fatal(Format(12.5, 2))
	}
}

func TestETA(t *testing.T) {
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	dep, err := ParseClock("22:40", now)
	if err != nil {
		t.Fatal(err)
	}
	eta, _, _ := ETA(dep, 100, 12)
	if eta.Format("02.01 15:04") != "29.10 07:00" {
		t.Fatal(eta)
	}
	to, _ := ParseClock("30.10 08:00", now)
	v, dt, _ := RequiredSpeed(240, dep, to)
	if HM(dt) != "33:20" || !near(v, 7.2, 1e-9) {
		t.Fatal(v, HM(dt))
	}
}

func TestCurrent(t *testing.T) {
	sog, cog := Current(10, 0, 2, 90)
	if !near(sog, math.Hypot(10, 2), 1e-9) || !near(cog, 11.31, 0.01) {
		t.Fatal(sog, cog)
	}
	if !near(Drift(350, 10), 20, 1e-9) || !near(Drift(10, 350), -20, 1e-9) {
		t.Fatal("drift")
	}
}

func TestUnits(t *testing.T) {
	v, _ := Convert(UnitGroups[0], 1, "мор. мили", "кабельтовы")
	if !near(v, 10, 1e-9) {
		t.Fatal(v)
	}
	v, _ = Convert(UnitGroups[1], 10, "узлы", "км/ч")
	if !near(v, 18.52, 1e-9) {
		t.Fatal(v)
	}
	if !near(Horizon(16), 8.32, 1e-9) {
		t.Fatal(Horizon(16))
	}
}
