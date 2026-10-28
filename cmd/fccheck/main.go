package main

import (
	"context"
	"fmt"
	"sort"
	"time"

	"ru.navhelper/internal/forecast"
)

// Живая проверка клиента на реальных данных (не входит в приложение).
func main() {
	// Важно: у каждого сетевого вызова — свой контекст с собственным
	// таймаутом, а не один общий на всю цепочку. Иначе одна медленная
	// попытка в начале съедает бюджет времени у всех вызовов после неё
	// (ровно это и давало ложные "context deadline exceeded" пачкой).
	call := func(d time.Duration, f func(context.Context) error) error {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		return f(ctx)
	}
	c := forecast.NewClient()
	lat, lon := 59.95, 29.5

	var st forecast.Station
	t0 := time.Now()
	err := call(60*time.Second, func(ctx context.Context) error {
		var e error
		st, e = c.NearestStation(ctx, lat, lon)
		return e
	})
	fmt.Printf("станция: %+v err=%v (%.1fs)\n", st, err, time.Since(t0).Seconds())

	var live map[time.Time]*forecast.Hour
	var off int
	err = call(30*time.Second, func(ctx context.Context) error {
		var e error
		live, off, e = c.Live(ctx, lat, lon)
		return e
	})
	fmt.Println("live часов:", len(live), "offset:", off, "err:", err)

	var arch map[time.Time]*forecast.Hour
	err = call(30*time.Second, func(ctx context.Context) error {
		var e error
		arch, e = c.Archive(ctx, lat, lon, 7)
		return e
	})
	fmt.Println("archive часов:", len(arch), "err:", err)

	var sea, seaPrev map[time.Time]*forecast.Hour
	err = call(30*time.Second, func(ctx context.Context) error {
		var e error
		sea, seaPrev, e = c.MarineHours(ctx, lat, lon, 7, 3)
		return e
	})
	fmt.Println("marine:", len(sea), len(seaPrev), "err:", err)

	var obs []forecast.Obs
	err = call(30*time.Second, func(ctx context.Context) error {
		var e error
		obs, e = c.METAR(ctx, st.ICAO, 168)
		return e
	})
	fmt.Println("metar:", len(obs), "err:", err)

	loc := time.FixedZone("local", off)
	now := time.Now()
	ln := now.In(loc)
	start := time.Date(ln.Year(), ln.Month(), ln.Day(), 9, 0, 0, 0, loc)
	if ln.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	merge := func(a map[time.Time]*forecast.Hour, w map[time.Time]*forecast.Hour) []forecast.Hour {
		var out []forecast.Hour
		for t, h := range a {
			hh := *h
			if s, ok := w[t]; ok {
				hh.Wave, hh.HasWave = s.Wave, s.HasWave
				if s.HasWater {
					hh.Water, hh.HasWater = s.Water, true
				}
			}
			out = append(out, hh)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].T.Before(out[j].T) })
		return out
	}
	liveH := merge(live, sea)
	archH := merge(arch, seaPrev)
	var seaList []forecast.Hour
	for _, h := range sea {
		seaList = append(seaList, *h)
	}
	for _, d := range []int{1, 0, -1, -2, -3} {
		s := start.AddDate(0, 0, d).UTC()
		e := s.Add(24 * time.Hour)
		src := liveH
		lbl := "ДЕЙСТВУЮЩИЙ"
		if d < 0 {
			src, lbl = archH, "АРХИВ (выпуск сутки назад)"
		}
		f := forecast.Summarize(src, s, e, loc)
		fmt.Printf("\n══ %s  [%s]\n", forecast.PeriodTitle(s, e, loc), lbl)
		for _, l := range forecast.ForecastLines(f) {
			fmt.Printf("  %-10s %s\n", l.Label, l.Value)
		}
		if d <= 0 {
			fa := forecast.SummarizeFact(obs, seaList, s, e, loc)
			fmt.Printf("  ── факт %s, наблюдений %d, покрытие %.0f%%\n", fa.Station, fa.Obs, fa.Coverage*100)
			for _, l := range forecast.FactLines(fa) {
				fmt.Printf("  %-10s %s\n", l.Label, l.Value)
			}
			v := forecast.Verify(f, fa, now)
			fmt.Printf("  ОПРАВДЫВАЕМОСТЬ %d%% (final=%v)\n", v.Percent, v.Final)
			for _, ch := range v.Checks {
				m := "✗"
				if ch.OK {
					m = "✓"
				}
				fmt.Printf("     %s %-18s %-30s | %s\n", m, ch.Name, ch.Forecast, ch.Fact)
			}
		}
	}
}
