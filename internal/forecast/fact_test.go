package forecast

import (
	"strings"
	"testing"
)

// Факт одной строкой — ровно тот вид, что нужен в журнале:
// «Ветер СЗ 4–6 м/с. Волнение 2 балла. Видимость 50 кбт. … Оправдываемость 90%.»
func TestFactSummaryFull(t *testing.T) {
	f := Fact{
		HasWind: true, WindLo: 4, WindHi: 6, HasDir: true, Dir: 315,
		HasSea: true, SeaHi: 2,
		HasVis: true, VisMin: 50,
		HasCloud: true, Cloud: 5, Obs: 20,
		HasWater: true, WaterT: 19,
		HasDay: true, DayHi: 23,
	}
	v := Verdict{Percent: 90}
	s := FactSummary(f, v)
	for _, want := range []string{"Ветер", "м/с", "Волнение 2 балла", "Видимость 50 кбт", "Оправдываемость прогноза 90%"} {
		if !strings.Contains(s, want) {
			t.Errorf("в факте нет %q: %s", want, s)
		}
	}
}

func TestFactSummaryEmpty(t *testing.T) {
	if s := FactSummary(Fact{}, Verdict{Percent: -1}); s != "" {
		t.Errorf("пустой факт должен давать пустую строку, получено %q", s)
	}
}
