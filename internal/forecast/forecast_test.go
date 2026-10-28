package forecast

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestRhumb(t *testing.T) {
	cases := map[float64]string{0: "С", 350: "С", 22.4: "С", 22.6: "СВ", 45: "СВ", 135: "ЮВ", 225: "ЮЗ", 315: "СЗ", -45: "СЗ", 360: "С"}
	for d, want := range cases {
		if got := RhumbShort(d); got != want {
			t.Errorf("RhumbShort(%v)=%s, want %s", d, got, want)
		}
	}
	if RhumbName(315) != "северо-западный" || RhumbName(135) != "юго-восточный" {
		t.Error("RhumbName")
	}
}

func TestVectorMeanDir(t *testing.T) {
	d, ok := VectorMeanDir([]float64{350, 10}, []float64{5, 5})
	if !ok || AngleDiff(d, 0) > 0.5 {
		t.Errorf("350/10 → %v ok=%v, want ~0", d, ok)
	}
	if _, ok := VectorMeanDir([]float64{0, 180}, []float64{5, 5}); ok {
		t.Error("противоположные ветра должны давать «переменный»")
	}
}

func TestParseMETAR(t *testing.T) {
	// Реальные сводки, полученные 25.10.2026.
	o := ParseMETAR("METAR ULLI 251230Z 14004MPS 9999 -SHRA FEW008 BKN016CB OVC033 10/08 Q1014 R10L/290050 NOSIG", time.Now())
	if !o.HasWind || o.Speed != 4 || !o.HasDir || o.Dir != 140 {
		t.Errorf("ветер: %+v", o)
	}
	if !o.HasVis || o.VisM != VisMaxM {
		t.Errorf("видимость 9999 → %v", o.VisM)
	}
	if len(o.Phens) != 1 || o.Phens[0].Key != "showers" || o.Phens[0].Text != "небольшой ливневый дождь" {
		t.Errorf("явления: %+v", o.Phens)
	}
	if !o.HasCloud || o.Cloud != 4 || !o.HasTemp || o.Temp != 10 {
		t.Errorf("облачность/температура: %+v", o)
	}

	o = ParseMETAR("METAR UUEE 251200Z 27012G20KT 0800 R24L/1000N FG VV002 M02/M03 Q1020 TEMPO 0300 FG", time.Now())
	if math.Abs(o.Speed-12*ktToMS) > 1e-9 || !o.HasGust || math.Abs(o.Gust-20*ktToMS) > 1e-9 {
		t.Errorf("узлы→м/с: %+v", o)
	}
	if o.VisM != 800 || o.Temp != -2 {
		t.Errorf("vis/temp: %v %v", o.VisM, o.Temp)
	}
	if len(o.Phens) != 1 || o.Phens[0].Key != "fog" {
		t.Errorf("TEMPO-часть не должна попадать в факт: %+v", o.Phens)
	}

	o = ParseMETAR("METAR URSS 251230Z VRB01MPS CAVOK 21/13 Q1016 NOSIG", time.Now())
	if o.HasDir || o.Speed != 1 || o.VisM != VisMaxM || o.Cloud != 0 {
		t.Errorf("VRB/CAVOK: %+v", o)
	}
	o = ParseMETAR("METAR ULMM 251230Z 15002MPS 4000 1500NE -RADZ BR SCT019 05/03 Q1012", time.Now())
	if o.VisM != 1500 || len(o.Phens) != 2 {
		t.Errorf("минимальная видимость по секторам / явления: vis=%v phens=%+v", o.VisM, o.Phens)
	}
	o = ParseMETAR("METAR ULLI 251300Z 18005MPS 9999 VCSH SCT020 10/08 Q1014", time.Now())
	if len(o.Phens) != 0 {
		t.Errorf("VC — явления в окрестностях, не на станции: %+v", o.Phens)
	}
}

func TestVisCables(t *testing.T) {
	cases := map[float64]float64{12000: 50, 10000: 50, 9000: 50, 6500: 35, 3000: 16, 600: 3, 300: 1.5}
	for m, want := range cases {
		if got := VisCables(m); got != want {
			t.Errorf("VisCables(%v)=%v want %v", m, got, want)
		}
	}
	if FmtCabRange(20, 40) != "20–40 кбт" || FmtCabRange(35, 50) != "35–50 кбт и более" || FmtCabRange(50, 50) != "50 кбт и более" {
		t.Error("FmtCabRange")
	}
	if FmtCab(1.5) != "1,5 кбт" {
		t.Error(FmtCab(1.5))
	}
}

func TestSeaState(t *testing.T) {
	cases := map[float64]int{0: 0, 0.05: 1, 0.3: 2, 1.0: 3, 2.0: 4, 3.0: 5, 7: 7}
	for h, want := range cases {
		if got := SeaState(h); got != want {
			t.Errorf("SeaState(%v)=%d want %d", h, got, want)
		}
	}
}

// Синтетический период по образцу журнала: СВ 10–15 м/с, порывы до 20,
// волнение 4–5 баллов, видимость 20–40 кбт, небольшой дождь.
func samplePeriod() ([]Hour, time.Time, time.Time, *time.Location) {
	loc := time.FixedZone("MSK", 3*3600)
	start := time.Date(2026, 9, 29, 9, 0, 0, 0, loc).UTC()
	end := start.Add(24 * time.Hour)
	var hs []Hour
	for i := 0; i < 24; i++ {
		t := start.Add(time.Duration(i) * time.Hour)
		code := 3
		if i%3 == 0 {
			code = 61
		}
		temp := 15.0
		if isDay(t, loc) {
			temp = 19 + float64(i%4)
		}
		hs = append(hs, Hour{
			T: t, HasWind: true, Speed: 10 + float64(i%6), Dir: 40 + float64(i%3)*5,
			HasGust: true, Gust: 15 + float64(i%6),
			HasWave: true, Wave: 1.4 + float64(i%5)*0.4,
			HasVis: true, VisM: 3800 + float64(i%4)*1200,
			HasCode: true, Code: code, HasTemp: true, Temp: temp,
		})
	}
	return hs, start, end, loc
}

func TestSummarizeJournal(t *testing.T) {
	hs, start, end, loc := samplePeriod()
	f := Summarize(hs, start, end, loc)
	if !f.HasDir || RhumbShort(f.Dir) != "СВ" {
		t.Errorf("направление %v", f.Dir)
	}
	if f.WindLo != 10 || f.WindHi != 15 {
		t.Errorf("ветер %d–%d", f.WindLo, f.WindHi)
	}
	if !f.HasGust || f.GustHi < 18 {
		t.Errorf("порывы %+v", f)
	}
	if f.SeaLo != 4 || f.SeaHi != 5 {
		t.Errorf("волнение %d–%d", f.SeaLo, f.SeaHi)
	}
	if f.VisLo != 20 || f.VisHi < 25 || f.VisHi > 40 {
		t.Errorf("видимость %v–%v", f.VisLo, f.VisHi)
	}
	lines := ForecastLines(f)
	txt := ""
	for _, l := range lines {
		txt += l.Label + ": " + l.Value + "\n"
	}
	for _, want := range []string{"Северо-восточный 10–15 м/с, порывы до", "4–5 балла", "кбт", "небольшой дождь", "днём +19…+22°", "ночью +15°"} {
		if !strings.Contains(txt, want) {
			t.Errorf("нет %q в\n%s", want, txt)
		}
	}
	if PeriodTitle(start, end, loc) != "с 09:00 29.09 по 09:00 30.09" {
		t.Error(PeriodTitle(start, end, loc))
	}
}

func TestVerify(t *testing.T) {
	hs, start, end, loc := samplePeriod()
	fc := Summarize(hs, start, end, loc)
	var obs []Obs
	for i := 0; i < 48; i++ {
		tm := start.Add(time.Duration(i) * 30 * time.Minute)
		temp := 15.0
		if isDay(tm, loc) {
			temp = 21
		}
		o := Obs{Time: tm, HasWind: true, HasDir: true, Dir: 45, Speed: 13, HasVis: true, VisM: 6000,
			HasCloud: true, Cloud: 4, HasTemp: true, Temp: temp}
		if i == 10 {
			o.HasGust, o.Gust = true, 19
			o.Phens = []Phen{{"rain", "небольшой дождь", 2}}
		}
		obs = append(obs, o)
	}
	fa := SummarizeFact(obs, hs, start, end, loc)
	v := Verify(fc, fa, end.Add(time.Hour))
	if !v.Final {
		t.Error("период завершён и покрыт — должен быть Final")
	}
	if v.Percent != 100 {
		for _, c := range v.Checks {
			t.Logf("%-18s прогноз=%-24s факт=%-22s %v", c.Name, c.Forecast, c.Fact, c.OK)
		}
		t.Errorf("совпадающий факт → %d%%, want 100", v.Percent)
	}

	// Ветер развернулся на юг и усилился, дождя не было → оправдываемость падает.
	for i := range obs {
		obs[i].Dir, obs[i].Speed, obs[i].Phens = 190, 22, nil
	}
	fa = SummarizeFact(obs, hs, start, end, loc)
	v = Verify(fc, fa, end.Add(time.Hour))
	if v.Percent >= 70 {
		t.Errorf("несовпадение ветра и осадков → %d%%, ожидаю < 70", v.Percent)
	}
	if len(FactLines(fa)) == 0 {
		t.Error("FactLines пуст")
	}
}
