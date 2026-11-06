package sun

import (
	"testing"
	"time"
)

func TestCalculateSevastopol(t *testing.T) {
	loc := time.FixedZone("MSK", 3*3600)
	d := time.Date(2026, 6, 21, 0, 0, 0, 0, loc)
	r := Calculate(d, 44.6166, 33.5254, loc)
	if !r.HasSunrise || !r.HasSunset {
		t.Fatal("ожидались восход и заход на широте Севастополя")
	}
	if !r.Sunrise.Before(r.Sunset) {
		t.Fatalf("восход %v должен быть раньше захода %v", r.Sunrise, r.Sunset)
	}
	if r.Sunrise.Hour() < 3 || r.Sunrise.Hour() > 6 {
		t.Errorf("неожиданный час восхода %d", r.Sunrise.Hour())
	}
	if r.Sunset.Hour() < 19 || r.Sunset.Hour() > 21 {
		t.Errorf("неожиданный час захода %d", r.Sunset.Hour())
	}
	if r.DayLength < 14*time.Hour || r.DayLength > 16*time.Hour {
		t.Errorf("неожиданная длительность дня %v", r.DayLength)
	}
}

func TestPolarNight(t *testing.T) {
	loc := time.UTC
	d := time.Date(2026, 12, 21, 0, 0, 0, 0, loc)
	if r := Calculate(d, 78.0, 15.0, loc); !r.PolarNight {
		t.Errorf("на 78N в день зимнего солнцестояния ожидалась полярная ночь")
	}
}

func TestPolarDay(t *testing.T) {
	loc := time.UTC
	d := time.Date(2026, 6, 21, 0, 0, 0, 0, loc)
	if r := Calculate(d, 78.0, 15.0, loc); !r.PolarDay {
		t.Errorf("на 78N в день летнего солнцестояния ожидался полярный день")
	}
}
