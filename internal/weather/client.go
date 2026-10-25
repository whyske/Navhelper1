package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Point — координаты точки наблюдения.
type Point struct {
	Lat   float64
	Lon   float64
	Place string
}

// Daily — сводка погоды за один день в исходных единицах Open-Meteo.
type Daily struct {
	Day          string  // YYYY-MM-DD
	VisibilityM  float64 // средняя видимость за день, м
	WindMS       float64 // максимальная скорость ветра за день, м/с
	WaveM        float64 // максимальная высота значительной волны, м
	HasVisibility bool
	HasWind       bool
	HasWave       bool
}

// Client — клиент Open-Meteo (Forecast API + Marine API).
type Client struct {
	HTTP *http.Client
}

// NewClient создаёт клиента с таймаутом на запрос.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 25 * time.Second}}
}

// Current возвращает текущую погоду точки в виде одного Daily за сегодня.
func (c *Client) Current(ctx context.Context, p Point) (Daily, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", p.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", p.Lon))
	q.Set("current", "visibility,wind_speed_10m")
	q.Set("wind_speed_unit", "ms")
	q.Set("timezone", "UTC")

	var resp struct {
		Current struct {
			Time       string   `json:"time"`
			Visibility *float64 `json:"visibility"`
			WindSpeed  *float64 `json:"wind_speed_10m"`
		} `json:"current"`
	}
	if err := c.getJSON(ctx, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), &resp); err != nil {
		return Daily{}, fmt.Errorf("текущая погода: %w", err)
	}
	out := Daily{}
	// Время приходит как 2026-10-25T10:00 — берём дату.
	if len(resp.Current.Time) >= 10 {
		out.Day = resp.Current.Time[:10]
	}
	if resp.Current.Visibility != nil {
		out.VisibilityM = *resp.Current.Visibility
		out.HasVisibility = true
	}
	if resp.Current.WindSpeed != nil {
		out.WindMS = *resp.Current.WindSpeed
		out.HasWind = true
	}

	// Волнение берём из морского API за сегодня.
	wave, ok := c.waveForDay(ctx, p, out.Day)
	if ok {
		out.WaveM = wave
		out.HasWave = true
	}
	return out, nil
}

// History возвращает историю за последние days дней (включая сегодня).
// Используется для дозаполнения пропусков, когда приложение не открывали.
func (c *Client) History(ctx context.Context, p Point, days int) ([]Daily, error) {
	if days < 1 {
		days = 1
	}
	if days > 92 {
		days = 92
	}

	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", p.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", p.Lon))
	q.Set("daily", "visibility_mean,wind_speed_10m_max")
	q.Set("wind_speed_unit", "ms")
	q.Set("past_days", fmt.Sprint(days))
	q.Set("forecast_days", "1")
	q.Set("timezone", "UTC")

	var fc struct {
		Daily struct {
			Time           []string  `json:"time"`
			VisibilityMean []*float64 `json:"visibility_mean"`
			WindSpeedMax   []*float64 `json:"wind_speed_10m_max"`
		} `json:"daily"`
	}
	if err := c.getJSON(ctx, "https://api.open-meteo.com/v1/forecast?"+q.Encode(), &fc); err != nil {
		return nil, fmt.Errorf("история погоды: %w", err)
	}

	out := make([]Daily, 0, len(fc.Daily.Time))
	for i, day := range fc.Daily.Time {
		d := Daily{Day: day}
		if v, ok := at(fc.Daily.VisibilityMean, i); ok {
			d.VisibilityM = v
			d.HasVisibility = true
		}
		if v, ok := at(fc.Daily.WindSpeedMax, i); ok {
			d.WindMS = v
			d.HasWind = true
		}
		out = append(out, d)
	}

	// Волнение за те же дни — отдельным запросом к Marine API.
	mq := url.Values{}
	mq.Set("latitude", fmt.Sprintf("%.4f", p.Lat))
	mq.Set("longitude", fmt.Sprintf("%.4f", p.Lon))
	mq.Set("daily", "wave_height_max")
	mq.Set("past_days", fmt.Sprint(days))
	mq.Set("forecast_days", "1")
	mq.Set("timezone", "UTC")

	var marine struct {
		Daily struct {
			Time          []string  `json:"time"`
			WaveHeightMax []*float64 `json:"wave_height_max"`
		} `json:"daily"`
	}
	// Ошибку морского запроса игнорируем: для inland-точек волн просто нет.
	if err := c.getJSON(ctx, "https://marine-api.open-meteo.com/v1/marine?"+mq.Encode(), &marine); err == nil {
		waves := map[string]float64{}
		for i, day := range marine.Daily.Time {
			if v, ok := at(marine.Daily.WaveHeightMax, i); ok {
				waves[day] = v
			}
		}
		for i := range out {
			if w, ok := waves[out[i].Day]; ok {
				out[i].WaveM = w
				out[i].HasWave = true
			}
		}
	}
	return out, nil
}

// waveForDay возвращает высоту волны за конкретный день.
func (c *Client) waveForDay(ctx context.Context, p Point, day string) (float64, bool) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", p.Lat))
	q.Set("longitude", fmt.Sprintf("%.4f", p.Lon))
	q.Set("daily", "wave_height_max")
	q.Set("past_days", "1")
	q.Set("forecast_days", "1")
	q.Set("timezone", "UTC")

	var marine struct {
		Daily struct {
			Time          []string  `json:"time"`
			WaveHeightMax []*float64 `json:"wave_height_max"`
		} `json:"daily"`
	}
	if err := c.getJSON(ctx, "https://marine-api.open-meteo.com/v1/marine?"+q.Encode(), &marine); err != nil {
		return 0, false
	}
	for i, d := range marine.Daily.Time {
		if v, ok := at(marine.Daily.WaveHeightMax, i); ok && d == day {
			return v, true
		}
	}
	return 0, false
}

// getJSON выполняет GET и разбирает JSON.
func (c *Client) getJSON(ctx context.Context, rawURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "NavHelper/1.0 (Go; Android)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// at безопасно достаёт i-й элемент массива с возможными null.
func at(arr []*float64, i int) (float64, bool) {
	if i < 0 || i >= len(arr) || arr[i] == nil || *arr[i] != *arr[i] {
		return 0, false
	}
	return *arr[i], true
}
