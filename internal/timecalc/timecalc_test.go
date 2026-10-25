package timecalc

import "testing"

func TestFromPartsAndString(t *testing.T) {
	cases := []struct {
		h, m int64
		want string
	}{
		{0, 0, "0 ч 00 мин"},
		{150, 30, "150 ч 30 мин"},
		{0, 90, "1 ч 30 мин"},
		{-5, -15, "−5 ч 15 мин"},
		{24, 0, "24 ч 00 мин"},
		{1000, 59, "1000 ч 59 мин"},
	}
	for _, c := range cases {
		got := FromParts(c.h, c.m).String()
		if got != c.want {
			t.Errorf("FromParts(%d,%d) = %q, want %q", c.h, c.m, got, c.want)
		}
	}
}

func TestInt64Storage(t *testing.T) {
	d := FromParts(150, 30)
	if int64(d) != 150*3600+30*60 {
		t.Fatalf("хранение в секундах: %d", int64(d))
	}
}

func TestParseParts(t *testing.T) {
	cases := []struct {
		h, m string
		want Duration
	}{
		{"", "", 0},
		{"150", "30", FromParts(150, 30)},
		{"-5", "15", -FromParts(5, 15)},
		{"−5", "15", -FromParts(5, 15)},
		{"1,5", "", FromParts(1, 30)},
		{"0", "125", FromParts(2, 5)},
		{"", "-30", -FromParts(0, 30)},
	}
	for _, c := range cases {
		got, err := ParseParts(c.h, c.m)
		if err != nil {
			t.Fatalf("ParseParts(%q,%q): %v", c.h, c.m, err)
		}
		if got != c.want {
			t.Errorf("ParseParts(%q,%q) = %v, want %v", c.h, c.m, got, c.want)
		}
	}
	if _, err := ParseParts("abc", ""); err == nil {
		t.Error("ожидалась ошибка для «abc»")
	}
}

func TestParseString(t *testing.T) {
	cases := map[string]Duration{
		"150:30":        FromParts(150, 30),
		"-5:15":         -FromParts(5, 15),
		"150 ч 30 мин":  FromParts(150, 30),
		"2h30m":         FromParts(2, 30),
		"90":            FromParts(1, 30),
		"1 сут 2 ч":     FromParts(26, 0),
		"":              0,
		"10:00:30":      FromParts(10, 0) + 30,
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("Parse(%q) = %v (%d), want %v", in, got, int64(got), want)
		}
	}
}

func TestCalculatorBeyond24hAndNegative(t *testing.T) {
	var c Calculator
	c.Apply(OpAdd, FromParts(100, 45))
	c.Apply(OpAdd, FromParts(49, 45))
	if c.Value != FromParts(150, 30) {
		t.Fatalf("сумма: %v", c.Value)
	}
	expr := c.Apply(OpSub, FromParts(200, 0))
	if c.Value != -FromParts(49, 30) {
		t.Fatalf("разность: %v", c.Value)
	}
	if expr != "150 ч 30 мин − 200 ч 00 мин = −49 ч 30 мин" {
		t.Errorf("выражение: %q", expr)
	}
	c.Reset()
	if c.Value != 0 {
		t.Error("Reset не обнулил")
	}
}

func TestClockAndDays(t *testing.T) {
	d := FromParts(150, 30)
	if d.Clock() != "150:30" {
		t.Errorf("Clock = %q", d.Clock())
	}
	if d.DaysString() != "6 сут 06 ч 30 мин" {
		t.Errorf("DaysString = %q", d.DaysString())
	}
	if (-d).Clock() != "-150:30" {
		t.Errorf("neg Clock = %q", (-d).Clock())
	}
}

func TestPlural(t *testing.T) {
	cases := map[int64]string{1: "день", 2: "дня", 5: "дней", 11: "дней", 21: "день", 22: "дня", 111: "дней", 0: "дней"}
	for n, want := range cases {
		if got := Plural(n, "день", "дня", "дней"); got != want {
			t.Errorf("Plural(%d) = %s, want %s", n, got, want)
		}
	}
}
