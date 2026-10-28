package forecast

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Hour — почасовые данные модели (прогноз или архив прогноза).
type Hour struct {
	T        time.Time // UTC
	Speed    float64   // м/с
	Dir      float64   // °
	Gust     float64   // м/с
	HasWind  bool
	HasGust  bool
	Wave     float64 // значительная волна, м
	HasWave  bool
	VisM     float64
	HasVis   bool
	Code     int
	HasCode  bool
	Temp     float64
	HasTemp  bool
	Water    float64
	HasWater bool
}

// Forecast — прогноз на период в виде, как его записывают в журнал.
type Forecast struct {
	Start, End time.Time // начало/конец периода (UTC)
	Issued     time.Time // когда прогноз был получен/выпущен
	Source     string    // "snapshot" — сохранён приложением; "archive" — Previous Runs (выпуск за сутки)

	Dir, Dir2      float64 // преобладающее направление; Dir2 — во второй половине периода
	HasDir         bool
	Veer           bool // направление меняется к концу периода
	WindLo, WindHi int
	HasWind        bool
	GustHi         int
	HasGust        bool
	SeaLo, SeaHi   int
	WaveLo         float64
	WaveHi         float64
	HasSea         bool
	VisLo, VisHi   float64 // кбт
	HasVis         bool
	Cloud          int
	HasCloud       bool
	Phens          []Phen
	PhenText       string // «местами», «временами» и т.п. к каждому явлению уже применены
	DayLo, DayHi   float64
	HasDay         bool
	NightLo        float64
	NightHi        float64
	HasNight       bool
	WaterT         float64
	HasWater       bool
}

// Fact — фактическая погода за период по наблюдениям METAR (+волнение по модели).
type Fact struct {
	Start, End time.Time
	Station    string
	Obs        int     // наблюдений
	Coverage   float64 // доля часов периода с наблюдениями, 0..1
	Dir        float64
	HasDir     bool
	WindLo     int
	WindHi     int
	HasWind    bool
	GustHi     int
	HasGust    bool
	VisMin     float64 // кбт
	HasVis     bool
	Cloud      int
	HasCloud   bool
	Phens      []Phen
	DayLo      float64
	DayHi      float64
	HasDay     bool
	NightLo    float64
	NightHi    float64
	HasNight   bool
	SeaHi      int
	WaveHi     float64
	HasSea     bool
}

// isDay — дневные часы по местному времени: 09:00–20:59.
func isDay(t time.Time, loc *time.Location) bool {
	h := t.In(loc).Hour()
	return h >= 9 && h < 21
}

func inPeriod(t, start, end time.Time) bool { return !t.Before(start) && t.Before(end) }

// Summarize сворачивает почасовой прогноз за период в прогноз для журнала.
func Summarize(hours []Hour, start, end time.Time, loc *time.Location) Forecast {
	f := Forecast{Start: start, End: end}
	var dirs, sps, dirs1, sps1, dirs2, sps2, gusts, waves, vis, dayT, nightT, water []float64
	clouds := map[int]int{}
	type ph struct {
		p     Phen
		hours int
	}
	phens := map[string]*ph{}
	n := 0
	mid := start.Add(end.Sub(start) / 2)
	for _, h := range hours {
		if !inPeriod(h.T, start, end) {
			continue
		}
		n++
		if h.HasWind {
			dirs, sps = append(dirs, h.Dir), append(sps, h.Speed)
			if h.T.Before(mid) {
				dirs1, sps1 = append(dirs1, h.Dir), append(sps1, h.Speed)
			} else {
				dirs2, sps2 = append(dirs2, h.Dir), append(sps2, h.Speed)
			}
		}
		if h.HasGust {
			gusts = append(gusts, h.Gust)
		}
		if h.HasWave {
			waves = append(waves, h.Wave)
		}
		if h.HasVis {
			vis = append(vis, math.Min(h.VisM, VisMaxM))
		}
		if h.HasTemp {
			if isDay(h.T, loc) {
				dayT = append(dayT, h.Temp)
			} else {
				nightT = append(nightT, h.Temp)
			}
		}
		if h.HasWater {
			water = append(water, h.Water)
		}
		if h.HasCode {
			c, p := WMO(h.Code)
			clouds[c]++
			if p != nil {
				if e, ok := phens[p.Key]; ok {
					e.hours++
					if p.Severity > e.p.Severity {
						e.p = *p
					}
				} else {
					phens[p.Key] = &ph{*p, 1}
				}
			}
		}
	}
	if len(sps) > 0 {
		f.HasWind = true
		f.WindLo = int(math.Floor(Percentile(sps, 10)))
		f.WindHi = int(math.Ceil(Percentile(sps, 90)))
		if f.WindHi <= f.WindLo {
			f.WindHi = f.WindLo + 1
		}
		if d, ok := VectorMeanDir(dirs, sps); ok {
			f.Dir, f.HasDir = d, true
			d1, ok1 := VectorMeanDir(dirs1, sps1)
			d2, ok2 := VectorMeanDir(dirs2, sps2)
			if ok1 && ok2 && Rhumb(d1) != Rhumb(d2) && AngleDiff(d1, d2) >= 45 {
				f.Dir, f.Dir2, f.Veer = d1, d2, true
			}
		}
	}
	if len(gusts) > 0 {
		g := int(math.Ceil(Percentile(gusts, 95)))
		if g >= f.WindHi+3 {
			f.GustHi, f.HasGust = g, true
		}
	}
	if len(waves) > 0 {
		lo, hi := minMax(waves)
		f.WaveLo, f.WaveHi, f.HasSea = lo, hi, true
		f.SeaLo, f.SeaHi = SeaState(lo), SeaState(hi)
	}
	if len(vis) > 0 {
		lo, _ := minMax(vis)
		f.VisLo, f.VisHi, f.HasVis = VisCables(lo), VisCables(Percentile(vis, 50)), true
		if f.VisHi < f.VisLo {
			f.VisHi = f.VisLo
		}
	}
	if len(clouds) > 0 {
		best, bn := 0, -1
		for c, k := range clouds {
			if k > bn || (k == bn && c > best) {
				best, bn = c, k
			}
		}
		f.Cloud, f.HasCloud = best, true
	}
	var list []ph
	for _, e := range phens {
		list = append(list, *e)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].p.Severity > list[j].p.Severity })
	var parts []string
	for i, e := range list {
		if i >= 2 {
			break
		}
		f.Phens = append(f.Phens, e.p)
		t := e.p.Text
		if n > 0 && float64(e.hours)/float64(n) < 0.25 {
			if e.p.Key == "fog" {
				t += " местами"
			} else {
				t = "временами " + t
			}
		}
		parts = append(parts, t)
	}
	f.PhenText = strings.Join(parts, ", ")
	if len(dayT) > 0 {
		f.DayLo, f.DayHi = minMax(dayT)
		f.HasDay = true
	}
	if len(nightT) > 0 {
		f.NightLo, f.NightHi = minMax(nightT)
		f.HasNight = true
	}
	if len(water) > 0 {
		f.WaterT, f.HasWater = Percentile(water, 50), true
	}
	return f
}

// SummarizeFact сворачивает наблюдения METAR (и волнение модели) за период.
func SummarizeFact(obs []Obs, sea []Hour, start, end time.Time, loc *time.Location) Fact {
	f := Fact{Start: start, End: end}
	var dirs, sps, gusts, dayT, nightT []float64
	hoursSeen := map[int64]bool{}
	clouds := map[int]int{}
	phens := map[string]Phen{}
	minVis := math.Inf(1)
	for _, o := range obs {
		if !inPeriod(o.Time, start, end) {
			continue
		}
		f.Obs++
		if f.Station == "" {
			f.Station = o.Station
		}
		hoursSeen[o.Time.Unix()/3600] = true
		if o.HasWind {
			sps = append(sps, o.Speed)
			if o.HasDir {
				dirs = append(dirs, o.Dir)
			}
		}
		if o.HasGust {
			gusts = append(gusts, o.Gust)
		}
		if o.HasVis {
			minVis = math.Min(minVis, o.VisM)
		}
		if o.HasCloud {
			clouds[o.Cloud]++
		}
		for _, p := range o.Phens {
			if e, ok := phens[p.Key]; !ok || p.Severity > e.Severity {
				phens[p.Key] = p
			}
		}
		if o.HasTemp {
			if isDay(o.Time, loc) {
				dayT = append(dayT, o.Temp)
			} else {
				nightT = append(nightT, o.Temp)
			}
		}
	}
	total := end.Sub(start).Hours()
	if total > 0 {
		f.Coverage = math.Min(1, float64(len(hoursSeen))/total)
	}
	if len(sps) > 0 {
		lo, hi := minMax(sps)
		f.WindLo, f.WindHi, f.HasWind = int(math.Floor(lo)), int(math.Ceil(hi)), true
		if len(dirs) > 0 {
			var dsp []float64
			for _, o := range obs {
				if inPeriod(o.Time, start, end) && o.HasDir {
					dsp = append(dsp, o.Speed)
				}
			}
			if d, ok := VectorMeanDir(dirs, dsp); ok {
				f.Dir, f.HasDir = d, true
			}
		}
	}
	if len(gusts) > 0 {
		_, g := minMax(gusts)
		f.GustHi, f.HasGust = int(math.Round(g)), true
	}
	if !math.IsInf(minVis, 1) {
		f.VisMin, f.HasVis = VisCables(minVis), true
	}
	if len(clouds) > 0 {
		best, bn := 0, -1
		for c, k := range clouds {
			if k > bn || (k == bn && c > best) {
				best, bn = c, k
			}
		}
		f.Cloud, f.HasCloud = best, true
	}
	for _, p := range phens {
		f.Phens = append(f.Phens, p)
	}
	sort.Slice(f.Phens, func(i, j int) bool { return f.Phens[i].Severity > f.Phens[j].Severity })
	if len(dayT) > 0 {
		f.DayLo, f.DayHi = minMax(dayT)
		f.HasDay = true
	}
	if len(nightT) > 0 {
		f.NightLo, f.NightHi = minMax(nightT)
		f.HasNight = true
	}
	var waves []float64
	for _, h := range sea {
		if inPeriod(h.T, start, end) && h.HasWave {
			waves = append(waves, h.Wave)
		}
	}
	if len(waves) > 0 {
		_, hi := minMax(waves)
		f.WaveHi, f.SeaHi, f.HasSea = hi, SeaState(hi), true
	}
	return f
}

// ─────────────────────────── оправдываемость ───────────────────────────

// Check — проверка одного элемента прогноза.
type Check struct {
	Name     string
	Forecast string
	Fact     string
	OK       bool
}

// Verdict — итог проверки прогноза.
type Verdict struct {
	Checks  []Check
	Percent int  // оправдываемость, %; −1 — не определяется (нет наблюдений)
	Final   bool // период завершён и фактов достаточно (≥ 75% часов)
}

// visCat — градация видимости для сравнения: <0,5; 0,5–2; 2–5; 5–10; ≥10 км.
func visCat(cab float64) int {
	km := cab * CableM / 1000
	switch {
	case km < 0.5:
		return 0
	case km < 2:
		return 1
	case km < 5:
		return 2
	case km < 9.2:
		return 3
	}
	return 4
}

func hasPrecip(ps []Phen) bool {
	for _, p := range ps {
		if PrecipKey(p.Key) {
			return true
		}
	}
	return false
}

func hasKey(ps []Phen, k string) bool {
	for _, p := range ps {
		if p.Key == k {
			return true
		}
	}
	return false
}

// Verify сравнивает прогноз с фактом по элементам. Допуски — как принято
// при оценке морских прогнозов: направление ±45° (один румб в каждую
// сторону), скорость ±2 м/с к границам диапазона, порывы ±3 м/с,
// температура ±2°, волнение — попадание в диапазон баллов (±1 балл, если
// диапазон из одного балла), видимость — попадание в градацию.
func Verify(fc Forecast, fa Fact, now time.Time) Verdict {
	var v Verdict
	v.Percent = -1
	if fa.Obs == 0 {
		// Нет ни одного наблюдения — оправдываемость не определяется.
		return v
	}
	add := func(name, f, a string, ok bool) {
		v.Checks = append(v.Checks, Check{name, f, a, ok})
	}
	if fc.HasDir && fa.HasDir && fa.HasWind && fa.WindHi >= 3 {
		ok := AngleDiff(fc.Dir, fa.Dir) <= 45 || (fc.Veer && AngleDiff(fc.Dir2, fa.Dir) <= 45)
		fd := RhumbShort(fc.Dir)
		if fc.Veer {
			fd += "→" + RhumbShort(fc.Dir2)
		}
		add("Направление ветра", fd, RhumbShort(fa.Dir), ok)
	}
	if fc.HasWind && fa.HasWind {
		ok := fa.WindHi >= fc.WindLo-2 && fa.WindHi <= fc.WindHi+2
		add("Скорость ветра", fmt.Sprintf("%d–%d м/с", fc.WindLo, fc.WindHi),
			fmt.Sprintf("%d–%d м/с", fa.WindLo, fa.WindHi), ok)
	}
	if (fc.HasGust || fa.HasGust) && fc.HasWind && fa.HasWind {
		fg := fc.WindHi
		fs := "без порывов"
		if fc.HasGust {
			fg, fs = fc.GustHi, fmt.Sprintf("до %d м/с", fc.GustHi)
		}
		ag := fa.WindHi
		as := "не отмечены"
		if fa.HasGust {
			ag, as = fa.GustHi, fmt.Sprintf("%d м/с", fa.GustHi)
		}
		add("Порывы", fs, as, math.Abs(float64(ag-fg)) <= 3)
	}
	if fc.HasSea && fa.HasSea {
		lo, hi := fc.SeaLo, fc.SeaHi
		if lo == hi {
			lo, hi = lo-1, hi+1
		}
		add("Волнение", fmt.Sprintf("%s балла", rangeInt(fc.SeaLo, fc.SeaHi)),
			fmt.Sprintf("%d балла (по модели)", fa.SeaHi), fa.SeaHi >= lo && fa.SeaHi <= hi)
	}
	if fc.HasVis && fa.HasVis {
		c := visCat(fa.VisMin)
		add("Видимость", FmtCabRange(fc.VisLo, fc.VisHi), "мин. "+FmtCab(fa.VisMin),
			c >= visCat(fc.VisLo) && c <= visCat(fc.VisHi))
	}
	if fc.HasCloud && fa.Obs > 0 {
		fp, ap := hasPrecip(fc.Phens), hasPrecip(fa.Phens)
		add("Осадки", yesNo(fp, fc.PhenText), yesNo(ap, phenList(fa.Phens)), fp == ap)
		if hasKey(fc.Phens, "fog") || hasKey(fa.Phens, "fog") {
			ff, af := hasKey(fc.Phens, "fog"), hasKey(fa.Phens, "fog")
			add("Туман", yesNo(ff, "туман"), yesNo(af, "туман"), ff == af)
		}
	}
	if fc.HasDay && fa.HasDay {
		ok := fa.DayHi >= fc.DayLo-2 && fa.DayHi <= fc.DayHi+2
		add("t днём", FmtTempRange(fc.DayLo, fc.DayHi), "макс. "+FmtTemp(fa.DayHi), ok)
	}
	if fc.HasNight && fa.HasNight {
		ok := fa.NightLo >= fc.NightLo-2 && fa.NightLo <= fc.NightHi+2
		add("t ночью", FmtTempRange(fc.NightLo, fc.NightHi), "мин. "+FmtTemp(fa.NightLo), ok)
	}
	good := 0
	for _, c := range v.Checks {
		if c.OK {
			good++
		}
	}
	if n := len(v.Checks); n > 0 {
		v.Percent = int(math.Round(100 * float64(good) / float64(n)))
	}
	v.Final = !now.Before(fc.End) && fa.Coverage >= 0.75
	return v
}

func yesNo(b bool, text string) string {
	if !b {
		return "нет"
	}
	if text == "" {
		return "да"
	}
	return text
}

func rangeInt(a, b int) string {
	if a == b {
		return fmt.Sprint(a)
	}
	return fmt.Sprintf("%d–%d", a, b)
}

func phenList(ps []Phen) string {
	var s []string
	for _, p := range ps {
		if PrecipKey(p.Key) {
			s = append(s, p.Text)
		}
	}
	return strings.Join(s, ", ")
}
