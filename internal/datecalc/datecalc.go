// Package datecalc считает интервалы между датами и временем для модуля
// «Календарь»: «с 12.03.2025 14:30 до 21.10.2026 09:15 прошло N дней, N часов, N минут».
package datecalc

import (
	"fmt"
	"strings"
	"time"

	"ru.navhelper/internal/timecalc"
)

// Layout — формат ввода даты и времени.
const Layout = "02.01.2006 15:04"

// DateLayout — формат только даты.
const DateLayout = "02.01.2006"

// ParseDateTime разбирает «12.03.2025 14:30» или «12.03.2025» (время = 00:00)
// в местном часовом поясе.
func ParseDateTime(s string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.Local
	}
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, ",", " ")), " ")
	s = strings.ReplaceAll(s, "/", ".")
	for _, layout := range []string{Layout, "2.1.2006 15:04", "02.01.2006 15.04", DateLayout, "2.1.2006"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("неверная дата «%s», нужен формат ДД.ММ.ГГГГ ЧЧ:ММ", s)
}

// Interval — разница между двумя моментами.
type Interval struct {
	Negative     bool  // вторая дата раньше первой
	Days         int64 // полных суток
	Hours        int64 // 0..23
	Minutes      int64 // 0..59
	TotalMinutes int64 // всего минут (без знака)
	TotalSeconds int64 // со знаком
}

// Between считает интервал от from до to по абсолютному времени
// (переходы на летнее время учитываются автоматически через UTC).
func Between(from, to time.Time) Interval {
	sec := int64(to.Sub(from) / time.Second)
	iv := Interval{TotalSeconds: sec, Negative: sec < 0}
	if sec < 0 {
		sec = -sec
	}
	totalMin := sec / 60
	iv.TotalMinutes = totalMin
	iv.Days = totalMin / (24 * 60)
	iv.Hours = (totalMin % (24 * 60)) / 60
	iv.Minutes = totalMin % 60
	return iv
}

// String — «587 дней 18 часов 45 минут».
func (iv Interval) String() string {
	s := fmt.Sprintf("%d %s %d %s %d %s",
		iv.Days, timecalc.Plural(iv.Days, "день", "дня", "дней"),
		iv.Hours, timecalc.Plural(iv.Hours, "час", "часа", "часов"),
		iv.Minutes, timecalc.Plural(iv.Minutes, "минута", "минуты", "минут"))
	if iv.Negative {
		return "−(" + s + ")"
	}
	return s
}

// Describe — полная фраза для экрана.
func Describe(from, to time.Time) string {
	iv := Between(from, to)
	verb := "прошло"
	if iv.Negative {
		verb = "осталось (дата «до» раньше «с»)"
	}
	totalH := iv.TotalMinutes / 60
	return fmt.Sprintf("С %s до %s %s:\n%s\n= %d ч %02d мин всего (%d мин)",
		from.Format(Layout), to.Format(Layout), verb,
		Interval{Days: iv.Days, Hours: iv.Hours, Minutes: iv.Minutes}.String(),
		totalH, iv.TotalMinutes%60, iv.TotalMinutes)
}
