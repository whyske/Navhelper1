package weather

import (
	"context"
	"fmt"
	"math"
	"time"

	"ru.navhelper/internal/store"
)

// Ключи настроек, которые использует сервис погоды.
const (
	KeyLat       = "weather.lat"
	KeyLon       = "weather.lon"
	KeyPlace     = "weather.place"
	KeyHistDays  = "weather.history_days"
	KeyLastSync  = "weather.last_sync"
	DefaultLat   = 59.9500 // Финский залив, рейд Кронштадта
	DefaultLon   = 29.5000
	DefaultPlace = "Финский залив (Кронштадт)"
)

// Service связывает клиента Open-Meteo с локальной базой.
type Service struct {
	Store  *store.Store
	Client *Client
	// Now — источник текущего времени (подменяется в тестах).
	Now func() time.Time
}

// NewService создаёт сервис.
func NewService(st *store.Store) *Service {
	return &Service{Store: st, Client: NewClient(), Now: time.Now}
}

// Point возвращает точку наблюдения из настроек. Координаты округляются
// до 0,01°, чтобы ключ записи в базе был стабильным.
func (s *Service) Point() Point {
	return Point{
		Lat:   round2(s.Store.SettingFloat(KeyLat, DefaultLat)),
		Lon:   round2(s.Store.SettingFloat(KeyLon, DefaultLon)),
		Place: s.Store.Setting(KeyPlace, DefaultPlace),
	}
}

// HistoryDays — сколько дней истории хранить и показывать (7..30).
func (s *Service) HistoryDays() int {
	d := int(s.Store.SettingFloat(KeyHistDays, 14))
	if d < 7 {
		d = 7
	}
	if d > 30 {
		d = 30
	}
	return d
}

// SyncResult — итог синхронизации для отображения в UI.
type SyncResult struct {
	Saved   int    // сколько суточных записей записано/обновлено
	Skipped bool   // синхронизация не понадобилась
	Message string // текст для пользователя
}

// NeedSync сообщает, нужна ли загрузка: нет записи за сегодня или
// последняя синхронизация была больше 3 часов назад.
func (s *Service) NeedSync() bool {
	p := s.Point()
	today := s.Now().UTC().Format("2006-01-02")
	if _, ok := s.Store.WeatherForDay(today, p.Lat, p.Lon); !ok {
		return true
	}
	last, err := time.Parse(time.RFC3339, s.Store.Setting(KeyLastSync, ""))
	if err != nil {
		return true
	}
	return s.Now().Sub(last) > 3*time.Hour
}

// Sync загружает данные и сохраняет их. Если force=false и данные свежие,
// ничего не делает. За один запрос дозаполняются все пропущенные дни
// в пределах HistoryDays — это решает проблему, что мобильная ОС
// не даёт приложению работать в фоне каждый день.
func (s *Service) Sync(ctx context.Context, force bool) (SyncResult, error) {
	if !force && !s.NeedSync() {
		return SyncResult{Skipped: true, Message: "данные за сегодня уже есть"}, nil
	}
	p := s.Point()
	days := s.missingDays(p)
	list, err := s.Client.History(ctx, p, days)
	if err != nil {
		return SyncResult{}, err
	}
	today := s.Now().UTC().Format("2006-01-02")
	saved := 0
	for _, d := range list {
		// Прогнозные дни (после сегодняшнего) не сохраняем — только реальные.
		if d.Day > today {
			continue
		}
		if !d.HasVisibility && !d.HasWind && !d.HasWave {
			continue
		}
		err := s.Store.SaveWeather(store.WeatherDay{
			Day: d.Day, Lat: p.Lat, Lon: p.Lon, Place: p.Place,
			VisibilityM: d.VisibilityM, WindMS: d.WindMS, WaveM: d.WaveM,
			HasVis: d.HasVisibility, HasWind: d.HasWind, HasWave: d.HasWave,
			Source: "Open-Meteo",
		})
		if err != nil {
			return SyncResult{Saved: saved}, fmt.Errorf("сохранение %s: %w", d.Day, err)
		}
		saved++
	}
	_ = s.Store.SetSetting(KeyLastSync, s.Now().Format(time.RFC3339))
	return SyncResult{Saved: saved, Message: fmt.Sprintf("обновлено дней: %d", saved)}, nil
}

// missingDays считает, за сколько дней назад нужно запросить данные:
// от самой старой пропущенной даты в окне HistoryDays до сегодня.
func (s *Service) missingDays(p Point) int {
	window := s.HistoryDays()
	now := s.Now().UTC()
	for back := window; back >= 1; back-- {
		day := now.AddDate(0, 0, -back).Format("2006-01-02")
		if _, ok := s.Store.WeatherForDay(day, p.Lat, p.Lon); !ok {
			return back
		}
	}
	// Все прошлые дни есть — берём вчера и сегодня (вчера мог быть неполным).
	return 1
}

// Row — строка для отображения в UI, уже в морских единицах.
type Row struct {
	Day          string
	Cables       string // видимость, каб
	Beaufort     int
	BeaufortName string
	WindMS       float64
	WaveBall     int
	WaveName     string
	WaveM        float64
	HasVis       bool
	HasWind      bool
	HasWave      bool
}

// ToRow переводит сохранённую запись в морские единицы.
func ToRow(w store.WeatherDay) Row {
	r := Row{Day: w.Day, HasVis: w.HasVis, HasWind: w.HasWind, HasWave: w.HasWave,
		WindMS: w.WindMS, WaveM: w.WaveM}
	if w.HasVis {
		r.Cables = FormatCables(VisibilityToCables(w.VisibilityM))
	}
	if w.HasWind {
		r.Beaufort = Beaufort(w.WindMS)
		r.BeaufortName = BeaufortName(r.Beaufort)
	}
	if w.HasWave {
		r.WaveBall = WaveBall(w.WaveM)
		r.WaveName = WaveName(r.WaveBall)
	}
	return r
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
