package datecalc

import (
	"strings"
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*3600)

func TestExampleFromTask(t *testing.T) {
	from, err := ParseDateTime("12.03.2025 14:30", msk)
	if err != nil {
		t.Fatal(err)
	}
	to, err := ParseDateTime("21.10.2026 09:15", msk)
	if err != nil {
		t.Fatal(err)
	}
	iv := Between(from, to)
	// 12.03.2025 → 21.10.2026 = 588 дней; 14:30 → 09:15 = минус 5 ч 15 мин.
	if iv.Days != 587 || iv.Hours != 18 || iv.Minutes != 45 || iv.Negative {
		t.Fatalf("получено %+v", iv)
	}
	if got := iv.String(); got != "587 дней 18 часов 45 минут" {
		t.Errorf("String = %q", got)
	}
}

func TestNegativeInterval(t *testing.T) {
	a, _ := ParseDateTime("02.01.2026 00:00", msk)
	b, _ := ParseDateTime("01.01.2026 23:00", msk)
	iv := Between(a, b)
	if !iv.Negative || iv.Days != 0 || iv.Hours != 1 || iv.Minutes != 0 {
		t.Fatalf("получено %+v", iv)
	}
}

func TestDateOnlyAndLeapYear(t *testing.T) {
	a, err := ParseDateTime("28.02.2024", msk)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ParseDateTime("01.03.2024", msk)
	if iv := Between(a, b); iv.Days != 2 {
		t.Fatalf("високосный год: %+v", iv)
	}
}

func TestParseVariants(t *testing.T) {
	for _, s := range []string{"1.3.2025 9:05", "01.03.2025 09:05", "01/03/2025 09:05", " 01.03.2025   09:05 "} {
		got, err := ParseDateTime(s, msk)
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if got.Day() != 1 || got.Month() != 3 || got.Hour() != 9 || got.Minute() != 5 {
			t.Errorf("%q → %v", s, got)
		}
	}
	if _, err := ParseDateTime("32.13.2025", msk); err == nil {
		t.Error("ожидалась ошибка")
	}
}

func TestDescribe(t *testing.T) {
	a, _ := ParseDateTime("01.01.2026 00:00", msk)
	b, _ := ParseDateTime("03.01.2026 01:01", msk)
	d := Describe(a, b)
	if !strings.Contains(d, "2 дня 1 час 1 минута") || !strings.Contains(d, "49 ч 01 мин") {
		t.Errorf("Describe = %q", d)
	}
}
