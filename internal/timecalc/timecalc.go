// Package timecalc реализует арифметику времени без привязки к 24-часовому
// циферблату. Любой интервал хранится как int64 — число секунд, поэтому
// результат может быть больше суток («150 ч 30 мин») и отрицательным.
package timecalc

import (
	"fmt"
	"strconv"
	"strings"
)

// Duration — интервал времени в секундах.
type Duration int64

const (
	Second Duration = 1
	Minute Duration = 60
	Hour   Duration = 3600
	Day    Duration = 86400
)

// FromParts собирает интервал из часов и минут. Минуты не ограничены
// диапазоном 0..59: 90 минут дадут 1 ч 30 мин. Знак берётся у каждого
// слагаемого отдельно, поэтому FromParts(-5, -15) = −5 ч 15 мин.
func FromParts(hours, minutes int64) Duration {
	return Duration(hours)*Hour + Duration(minutes)*Minute
}

// ParseParts разбирает два текстовых поля «часы» и «минуты».
// Пустое поле = 0. Допускаются знак минус и дробная часть часов
// («1,5» ч = 1 ч 30 мин). Если в поле часов стоит минус, минуты
// тоже считаются отрицательными: «-5» ч «15» мин = −5 ч 15 мин.
func ParseParts(hoursText, minutesText string) (Duration, error) {
	h, hNeg, err := parseNumber(hoursText)
	if err != nil {
		return 0, fmt.Errorf("часы: %w", err)
	}
	m, mNeg, err := parseNumber(minutesText)
	if err != nil {
		return 0, fmt.Errorf("минуты: %w", err)
	}
	hs := int64(h*3600 + 0.5*sign(h))
	ms := int64(m*60 + 0.5*sign(m))
	if hNeg && !mNeg && ms > 0 {
		ms = -ms
	}
	return Duration(hs + ms), nil
}

// Parse разбирает одну строку: «150:30», «-5:15», «150 ч 30 мин», «2h30m», «90»
// (просто число — минуты).
func Parse(s string) (Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}
	neg := false
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "−") {
		neg = true
		s = strings.TrimLeft(s, "-−")
		s = strings.TrimSpace(s)
	}
	var total Duration
	if strings.Contains(s, ":") {
		// Формат Ч:ММ или Ч:ММ:СС.
		parts := strings.Split(s, ":")
		if len(parts) > 3 {
			return 0, fmt.Errorf("неверный формат %q", s)
		}
		mult := []Duration{Hour, Minute, Second}
		for i, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			v, err := strconv.ParseInt(p, 10, 64)
			if err != nil || v < 0 {
				return 0, fmt.Errorf("неверное число %q", p)
			}
			total += Duration(v) * mult[i]
		}
	} else {
		// Формат с суффиксами: число + единица (ч/h, м/мин/m, с/сек/s).
		// Число без единицы трактуется как минуты.
		var num strings.Builder
		var unit strings.Builder
		flush := func() error {
			if num.Len() == 0 {
				unit.Reset()
				return nil
			}
			raw := strings.ReplaceAll(num.String(), ",", ".")
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return fmt.Errorf("неверное число %q", raw)
			}
			var k Duration
			// Порядок проверок важен: «сут» начинается с «с», поэтому
			// сутки проверяются раньше секунд.
			switch u := unit.String(); {
			case strings.HasPrefix(u, "сут"), strings.HasPrefix(u, "д"), strings.HasPrefix(u, "d"):
				k = Day
			case strings.HasPrefix(u, "ч"), strings.HasPrefix(u, "h"):
				k = Hour
			case strings.HasPrefix(u, "с"), strings.HasPrefix(u, "s"):
				k = Second
			case u == "", strings.HasPrefix(u, "м"), strings.HasPrefix(u, "m"):
				k = Minute
			default:
				return fmt.Errorf("неизвестная единица %q", u)
			}
			total += Duration(v*float64(k) + 0.5)
			num.Reset()
			unit.Reset()
			return nil
		}
		for _, r := range s {
			switch {
			case r >= '0' && r <= '9' || r == '.' || r == ',':
				if unit.Len() > 0 {
					if err := flush(); err != nil {
						return 0, err
					}
				}
				num.WriteRune(r)
			case r == ' ' || r == '\t':
				// пробел ничего не меняет
			default:
				if num.Len() == 0 && unit.Len() == 0 {
					continue // мусор в начале, например «+»
				}
				unit.WriteRune(r)
			}
		}
		if err := flush(); err != nil {
			return 0, err
		}
	}
	if neg {
		total = -total
	}
	return total, nil
}

// parseNumber разбирает число из поля ввода (пусто → 0).
func parseNumber(s string) (float64, bool, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "−", "-")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.ReplaceAll(s, " ", "")
	if s == "" || s == "-" || s == "+" {
		return 0, strings.HasPrefix(s, "-"), nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, fmt.Errorf("«%s» — не число", s)
	}
	return v, strings.HasPrefix(s, "-"), nil
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// Abs возвращает модуль интервала.
func (d Duration) Abs() Duration {
	if d < 0 {
		return -d
	}
	return d
}

// Split раскладывает интервал на часы, минуты и секунды (все — без знака)
// и отдельно возвращает признак отрицательности.
func (d Duration) Split() (neg bool, hours, minutes, seconds int64) {
	neg = d < 0
	a := int64(d.Abs())
	hours = a / 3600
	minutes = (a % 3600) / 60
	seconds = a % 60
	return
}

// String — «150 ч 30 мин», «−5 ч 15 мин», «0 ч 00 мин». Секунды
// показываются, только если они не равны нулю.
func (d Duration) String() string {
	neg, h, m, s := d.Split()
	sgn := ""
	if neg {
		sgn = "−"
	}
	if s != 0 {
		return fmt.Sprintf("%s%d ч %02d мин %02d с", sgn, h, m, s)
	}
	return fmt.Sprintf("%s%d ч %02d мин", sgn, h, m)
}

// Clock — компактная запись «150:30» / «-5:15».
func (d Duration) Clock() string {
	neg, h, m, _ := d.Split()
	sgn := ""
	if neg {
		sgn = "-"
	}
	return fmt.Sprintf("%s%d:%02d", sgn, h, m)
}

// DaysString — «6 сут 06 ч 30 мин» (для длинных интервалов).
func (d Duration) DaysString() string {
	neg, h, m, _ := d.Split()
	sgn := ""
	if neg {
		sgn = "−"
	}
	return fmt.Sprintf("%s%d сут %02d ч %02d мин", sgn, h/24, h%24, m)
}

// Op — операция калькулятора.
type Op int

const (
	OpAdd Op = iota
	OpSub
)

// Symbol возвращает знак операции.
func (o Op) Symbol() string {
	if o == OpSub {
		return "−"
	}
	return "+"
}

// Calculator — накопительный калькулятор: текущее значение + история.
type Calculator struct {
	Value Duration
}

// Apply применяет операцию и возвращает выражение для истории,
// например «150 ч 30 мин + 2 ч 15 мин = 152 ч 45 мин».
func (c *Calculator) Apply(op Op, arg Duration) string {
	before := c.Value
	switch op {
	case OpAdd:
		c.Value += arg
	case OpSub:
		c.Value -= arg
	}
	return fmt.Sprintf("%s %s %s = %s", before, op.Symbol(), arg, c.Value)
}

// Reset обнуляет калькулятор.
func (c *Calculator) Reset() { c.Value = 0 }

// Plural выбирает русскую форму слова по числу: Plural(5, "день", "дня", "дней").
func Plural(n int64, one, few, many string) string {
	if n < 0 {
		n = -n
	}
	n100 := n % 100
	n10 := n % 10
	switch {
	case n100 >= 11 && n100 <= 14:
		return many
	case n10 == 1:
		return one
	case n10 >= 2 && n10 <= 4:
		return few
	default:
		return many
	}
}
