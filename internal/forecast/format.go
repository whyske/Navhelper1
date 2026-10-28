package forecast

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Line — строка журнала: подпись и значение.
type Line struct {
	Label string
	Value string
}

func capFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// PeriodTitle — «с 09:00 29.09 по 09:00 30.09» в местном времени.
func PeriodTitle(start, end time.Time, loc *time.Location) string {
	s, e := start.In(loc), end.In(loc)
	return fmt.Sprintf("с %s %s по %s %s", s.Format("15:04"), s.Format("02.01"), e.Format("15:04"), e.Format("02.01"))
}

func windDirText(dir float64, has bool) string {
	if !has {
		return "переменный"
	}
	return RhumbName(dir)
}

// ForecastLines — прогноз в строках журнала.
func ForecastLines(f Forecast) []Line {
	var out []Line
	if f.HasWind {
		s := fmt.Sprintf("%s %d–%d м/с", windDirText(f.Dir, f.HasDir), f.WindLo, f.WindHi)
		if f.HasGust {
			s += fmt.Sprintf(", порывы до %d м/с", f.GustHi)
		}
		if f.Veer {
			s += ", к концу периода " + RhumbName(f.Dir2)
		}
		out = append(out, Line{"Ветер", capFirst(s)})
	}
	if f.HasSea {
		out = append(out, Line{"Волнение", fmt.Sprintf("%s балла, волна %s м",
			rangeInt(f.SeaLo, f.SeaHi), waveRange(f.WaveLo, f.WaveHi))})
	}
	if f.HasVis {
		out = append(out, Line{"Видимость", FmtCabRange(f.VisLo, f.VisHi)})
	}
	if f.HasCloud {
		w := CloudText(f.Cloud)
		if f.PhenText != "" {
			w += ", " + f.PhenText
		}
		out = append(out, Line{"Погода", capFirst(w)})
	}
	var t []string
	if f.HasDay {
		t = append(t, "днём "+FmtTempRange(f.DayLo, f.DayHi))
	}
	if f.HasNight {
		t = append(t, "ночью "+FmtTempRange(f.NightLo, f.NightHi))
	}
	if len(t) > 0 {
		out = append(out, Line{"t воздуха", strings.Join(t, ", ")})
	}
	if f.HasWater {
		out = append(out, Line{"t воды", FmtTemp(f.WaterT)})
	}
	return out
}

// FactLines — фактическая погода в строках журнала.
func FactLines(f Fact) []Line {
	var out []Line
	if f.HasWind {
		dir := "переменный"
		if f.HasDir && f.WindHi >= 2 {
			dir = RhumbName(f.Dir)
		}
		s := fmt.Sprintf("%s %d–%d м/с", dir, f.WindLo, f.WindHi)
		if f.HasGust {
			s += fmt.Sprintf(", порывы %d м/с", f.GustHi)
		}
		out = append(out, Line{"Ветер", capFirst(s)})
	}
	if f.HasSea {
		out = append(out, Line{"Волнение", fmt.Sprintf("%d балла, волна до %s м (по модели)", f.SeaHi, fmtM(f.WaveHi))})
	}
	if f.HasVis {
		out = append(out, Line{"Видимость", "мин. " + FmtCab(f.VisMin)})
	}
	if f.Obs > 0 {
		w := ""
		if f.HasCloud {
			w = CloudText(f.Cloud)
		}
		var ps []string
		for _, p := range f.Phens {
			ps = append(ps, p.Text)
		}
		if len(ps) > 0 {
			if w != "" {
				w += ", "
			}
			w += strings.Join(ps, ", ")
		}
		if w == "" {
			w = "без явлений"
		}
		out = append(out, Line{"Погода", capFirst(w)})
	}
	var t []string
	if f.HasDay {
		t = append(t, "днём до "+FmtTemp(f.DayHi))
	}
	if f.HasNight {
		t = append(t, "ночью до "+FmtTemp(f.NightLo))
	}
	if len(t) > 0 {
		out = append(out, Line{"t воздуха", strings.Join(t, ", ")})
	}
	return out
}

func fmtM(m float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", m), ".", ",", 1)
}

func waveRange(lo, hi float64) string {
	l, h := fmtM(lo), fmtM(hi)
	if l == h {
		return l
	}
	return l + "–" + h
}
