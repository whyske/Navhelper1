package forecast

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client — загрузчик реальных данных: Open-Meteo (прогноз, архив прогнозов,
// морской API) и METAR aviationweather.gov (наблюдения).
type Client struct {
	HTTP *http.Client
}

// NewClient — клиент с таймаутом на попытку и повторами.
func NewClient() *Client {
	c := &Client{HTTP: &http.Client{Timeout: 30 * time.Second}}
	// Прогрев: первое TLS-соединение к aviationweather.gov из свежего
	// процесса Go иногда зависает по 10+ с (проверено многократно — ровно
	// тот же запрос вторым заходом укладывается в 0,1-0,3 с). Запасного
	// источника METAR без этой особенности нет, поэтому прогреваем
	// соединение в фоне сразу при создании клиента, не блокируя вызывающий
	// код: к моменту, когда пользователь откроет вкладку погоды, соединение
	// уже установлено.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://aviationweather.gov/", nil)
		if err != nil {
			return
		}
		req.Header.Set("User-Agent", "navhelper/2.0")
		if resp, err := c.HTTP.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	return c
}

const (
	maxAttempts    = 4
	attemptTimeout = 6 * time.Second
)

// getJSON — GET с повторами при сетевой ошибке, 429 или 5xx.
func (c *Client) getJSON(ctx context.Context, u string, out any) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		if i > 0 {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-ctx.Done():
				return err
			}
		}
		actx, cancel := context.WithTimeout(ctx, attemptTimeout)
		err = c.getOnce(actx, u, out)
		cancel()
		if err == nil || ctx.Err() != nil || !retryable(err) {
			return err
		}
	}
	return err
}

type httpStatusErr struct {
	code   int
	reason string
}

func (e httpStatusErr) Error() string {
	if e.reason != "" {
		return fmt.Sprintf("HTTP %d: %s", e.code, e.reason)
	}
	return fmt.Sprintf("HTTP %d", e.code)
}

func retryable(err error) bool {
	if he, ok := err.(httpStatusErr); ok {
		return he.code >= 500 || he.code == 429
	}
	return true
}

func (c *Client) getOnce(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "navhelper/2.0")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(body, &e)
		return httpStatusErr{resp.StatusCode, e.Reason}
	}
	return json.Unmarshal(body, out)
}

type omHourly struct {
	UTCOffset int                        `json:"utc_offset_seconds"`
	Hourly    map[string]json.RawMessage `json:"hourly"`
}

func (h omHourly) times() ([]time.Time, error) {
	var ts []string
	if err := json.Unmarshal(h.Hourly["time"], &ts); err != nil {
		return nil, fmt.Errorf("нет времени в ответе")
	}
	out := make([]time.Time, len(ts))
	for i, s := range ts {
		t, err := time.Parse("2006-01-02T15:04", s)
		if err != nil {
			return nil, err
		}
		out[i] = t.UTC()
	}
	return out, nil
}

func (h omHourly) floats(key string) []*float64 {
	var v []*float64
	if raw, ok := h.Hourly[key]; ok {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

func at(a []*float64, i int) (float64, bool) {
	if i < len(a) && a[i] != nil && !math.IsNaN(*a[i]) {
		return *a[i], true
	}
	return 0, false
}

func pointQuery(lat, lon float64) url.Values {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("timezone", "GMT")
	return q
}

// atmo загружает атмосферную часть. suffix = "" для действующего прогноза,
// "_previous_day1" для архива прогнозов (как они были выпущены сутки назад).
func (c *Client) atmo(ctx context.Context, lat, lon float64, base, suffix string, pastDays, fcDays int) (map[time.Time]*Hour, int, error) {
	vars := []string{"wind_speed_10m", "wind_direction_10m", "wind_gusts_10m", "weather_code", "temperature_2m"}
	if suffix == "" {
		vars = append(vars, "visibility") // архива видимости у Open-Meteo нет
	}
	var req []string
	for _, v := range vars {
		req = append(req, v+suffix)
	}
	q := pointQuery(lat, lon)
	q.Set("hourly", strings.Join(req, ","))
	q.Set("wind_speed_unit", "ms")
	q.Set("past_days", fmt.Sprint(pastDays))
	q.Set("forecast_days", fmt.Sprint(fcDays))
	var r omHourly
	if err := c.getJSON(ctx, base+"?"+q.Encode(), &r); err != nil {
		return nil, 0, err
	}
	ts, err := r.times()
	if err != nil {
		return nil, 0, err
	}
	sp, dr, gu := r.floats("wind_speed_10m"+suffix), r.floats("wind_direction_10m"+suffix), r.floats("wind_gusts_10m"+suffix)
	wc, tt, vi := r.floats("weather_code"+suffix), r.floats("temperature_2m"+suffix), r.floats("visibility")
	out := map[time.Time]*Hour{}
	for i, t := range ts {
		h := &Hour{T: t}
		s, ok1 := at(sp, i)
		d, ok2 := at(dr, i)
		if ok1 && ok2 {
			h.Speed, h.Dir, h.HasWind = s, d, true
		}
		if g, ok := at(gu, i); ok {
			h.Gust, h.HasGust = g, true
		}
		if code, ok := at(wc, i); ok {
			h.Code, h.HasCode = int(code), true
		}
		if v, ok := at(tt, i); ok {
			h.Temp, h.HasTemp = v, true
		}
		if suffix == "" {
			if v, ok := at(vi, i); ok {
				h.VisM, h.HasVis = v, true
			}
		}
		if h.HasWind || h.HasCode || h.HasTemp {
			out[t] = h
		}
	}
	return out, r.UTCOffset, nil
}

// MarineHours — волнение и температура воды. cur — действующий прогноз и
// анализ за прошедшие дни; prev — архив прогноза волнения (выпуск сутки назад).
func (c *Client) MarineHours(ctx context.Context, lat, lon float64, pastDays, fcDays int) (cur, prev map[time.Time]*Hour, err error) {
	q := pointQuery(lat, lon)
	q.Set("hourly", "wave_height,sea_surface_temperature,wave_height_previous_day1")
	q.Set("past_days", fmt.Sprint(pastDays))
	q.Set("forecast_days", fmt.Sprint(fcDays))
	var r omHourly
	if err := c.getJSON(ctx, "https://marine-api.open-meteo.com/v1/marine?"+q.Encode(), &r); err != nil {
		return nil, nil, err
	}
	ts, err := r.times()
	if err != nil {
		return nil, nil, err
	}
	wh, st, wp := r.floats("wave_height"), r.floats("sea_surface_temperature"), r.floats("wave_height_previous_day1")
	cur, prev = map[time.Time]*Hour{}, map[time.Time]*Hour{}
	for i, t := range ts {
		h := &Hour{T: t}
		if v, ok := at(wh, i); ok {
			h.Wave, h.HasWave = v, true
		}
		if v, ok := at(st, i); ok {
			h.Water, h.HasWater = v, true
		}
		if h.HasWave || h.HasWater {
			cur[t] = h
		}
		if v, ok := at(wp, i); ok {
			prev[t] = &Hour{T: t, Wave: v, HasWave: true}
		}
	}
	return cur, prev, nil
}

// Live — действующий прогноз (вчера + 3 суток вперёд) и смещение часового пояса точки.
func (c *Client) Live(ctx context.Context, lat, lon float64) (map[time.Time]*Hour, int, error) {
	// timezone=auto нужен, только чтобы узнать смещение местного времени.
	q := pointQuery(lat, lon)
	q.Set("timezone", "auto")
	q.Set("hourly", "temperature_2m")
	q.Set("forecast_days", "1")
	var tz omHourly
	if err := c.getJSON(ctx, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), &tz); err != nil {
		return nil, 0, err
	}
	hs, _, err := c.atmo(ctx, lat, lon, "https://api.open-meteo.com/v1/forecast", "", 1, 3)
	return hs, tz.UTCOffset, err
}

// Archive — прогнозы на прошедшие дни, какими они были выпущены сутки назад.
func (c *Client) Archive(ctx context.Context, lat, lon float64, pastDays int) (map[time.Time]*Hour, error) {
	hs, _, err := c.atmo(ctx, lat, lon, "https://previous-runs-api.open-meteo.com/v1/forecast", "_previous_day1", pastDays, 1)
	return hs, err
}

// Station — станция METAR.
type Station struct {
	ICAO string  `json:"icao"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Km   float64 `json:"km"`
}

// HaversineKm — расстояние по дуге большого круга, км.
func HaversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*p/2), 2) +
		math.Cos(lat1*p)*math.Cos(lat2*p)*math.Pow(math.Sin((lon2-lon1)*p/2), 2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// NearestStation — ближайший аэропорт, который реально передаёт METAR
// (за последние 6 часов была хотя бы одна сводка).
func (c *Client) NearestStation(ctx context.Context, lat, lon float64) (Station, error) {
	// Используем stationinfo, а не metar?bbox: тот же справочник станций,
	// но без привязки к агрегации наблюдений за период — отвечает стабильно
	// быстро даже при "холодном" первом запросе к aviationweather.gov,
	// тогда как metar?bbox именно на первом обращении к новой площади
	// иногда собирает ответ по 10+ с (проверено и curl, и Go одинаково).
	var lastErr error
	for _, d := range []float64{1, 2, 4, 8} {
		bbox := fmt.Sprintf("%.3f,%.3f,%.3f,%.3f", lat-d, lon-d*2, lat+d, lon+d*2)
		var raw []struct {
			ICAO string  `json:"icaoId"`
			Site string  `json:"site"`
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
		}
		u := "https://aviationweather.gov/api/data/stationinfo?format=json&bbox=" + bbox
		if err := c.getJSON(ctx, u, &raw); err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return Station{}, err
			}
			continue // пробуем следующий, более широкий радиус
		}
		var list []Station
		seen := map[string]bool{}
		for _, s := range raw {
			if s.ICAO == "" || seen[s.ICAO] {
				continue
			}
			seen[s.ICAO] = true
			list = append(list, Station{ICAO: s.ICAO, Name: s.Site, Lat: s.Lat, Lon: s.Lon,
				Km: HaversineKm(lat, lon, s.Lat, s.Lon)})
		}
		if len(list) > 0 {
			sort.Slice(list, func(i, j int) bool { return list[i].Km < list[j].Km })
			return list[0], nil
		}
	}
	if lastErr != nil {
		return Station{}, lastErr
	}
	return Station{}, fmt.Errorf("в радиусе ~450 км нет станций METAR")
}

// METAR — наблюдения станции за последние hours часов.
func (c *Client) METAR(ctx context.Context, icao string, hours int) ([]Obs, error) {
	var raw []struct {
		ReportTime string `json:"reportTime"`
		RawOb      string `json:"rawOb"`
	}
	u := fmt.Sprintf("https://aviationweather.gov/api/data/metar?format=json&ids=%s&hours=%d", url.QueryEscape(icao), hours)
	if err := c.getJSON(ctx, u, &raw); err != nil {
		// Запасной источник — архив OGIMET: те же реальные сводки METAR.
		if obs, oerr := c.metarOgimet(ctx, icao, hours); oerr == nil && len(obs) > 0 {
			return obs, nil
		}
		return nil, err
	}
	var out []Obs
	for _, r := range raw {
		t, err := time.Parse(time.RFC3339, r.ReportTime)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05.000Z", r.ReportTime)
		}
		if err != nil || r.RawOb == "" {
			continue
		}
		out = append(out, ParseMETAR(r.RawOb, t.UTC()))
	}
	return out, nil
}

// metarOgimet — METAR из архива OGIMET. Формат строки:
// ULLI,2026,10,27,00,00,METAR ULLI 270000Z 19005MPS 9999 ... =
func (c *Client) metarOgimet(ctx context.Context, icao string, hours int) ([]Obs, error) {
	end := time.Now().UTC()
	beg := end.Add(-time.Duration(hours) * time.Hour)
	u := fmt.Sprintf("https://www.ogimet.com/cgi-bin/getmetar?icao=%s&begin=%s&end=%s",
		url.QueryEscape(icao), beg.Format("200601021504"), end.Format("200601021504"))
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		actx, cancel := context.WithTimeout(ctx, 15*time.Second)
		body, err := c.getRaw(actx, u)
		cancel()
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		var out []Obs
		for _, line := range strings.Split(string(body), "\n") {
			p := strings.SplitN(strings.TrimSpace(line), ",", 7)
			if len(p) != 7 {
				continue
			}
			t, err := time.Parse("2006,01,02,15,04", strings.Join(p[1:6], ","))
			if err != nil {
				continue
			}
			out = append(out, ParseMETAR(strings.TrimSuffix(p[6], "="), t.UTC()))
		}
		return out, nil
	}
	return nil, lastErr
}

// getRaw — GET с проверкой статуса, возвращает тело ответа.
func (c *Client) getRaw(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "navhelper/2.0")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, httpStatusErr{code: resp.StatusCode}
	}
	return body, nil
}
