package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	for i := 0; i < 3; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("открытие %d: %v", i, err)
		}
		s.Close()
	}
}

func TestSettings(t *testing.T) {
	s := openTemp(t)
	if s.Setting("x", "def") != "def" {
		t.Error("значение по умолчанию")
	}
	s.SetSetting("x", "1")
	s.SetSetting("x", "59.95")
	if s.Setting("x", "") != "59.95" || s.SettingFloat("x", 0) != 59.95 {
		t.Error("обновление настройки")
	}
}

func TestCalcHistory(t *testing.T) {
	s := openTemp(t)
	for i := 0; i < 105; i++ {
		if err := s.AddCalc("a + b", "150 ч 30 мин", int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := s.CalcHistory(1000)
	if len(h) != 100 {
		t.Fatalf("история ограничена 100, получено %d", len(h))
	}
	if h[0].Sec != 104 {
		t.Errorf("свежая запись сверху: %d", h[0].Sec)
	}
	s.ClearCalcHistory()
	h, _ = s.CalcHistory(10)
	if len(h) != 0 {
		t.Error("очистка истории")
	}
}

func TestNotesCRUD(t *testing.T) {
	s := openTemp(t)
	s.SetNote("2026-10-25", "Выход из Кронштадта")
	s.SetNote("2026-10-26", "Смена вахты")
	s.SetNote("2026-11-01", "Ноябрь")
	if n, ok := s.Note("2026-10-25"); !ok || n.Text != "Выход из Кронштадта" {
		t.Fatal("чтение заметки")
	}
	s.SetNote("2026-10-25", "Выход в 08:00")
	if n, _ := s.Note("2026-10-25"); n.Text != "Выход в 08:00" {
		t.Error("редактирование")
	}
	m, _ := s.NotesMonth(2026, time.October)
	if len(m) != 2 {
		t.Errorf("заметки за октябрь: %d", len(m))
	}
	s.SetNote("2026-10-26", "")
	if _, ok := s.Note("2026-10-26"); ok {
		t.Error("удаление пустым текстом")
	}
	all, _ := s.AllNotes()
	if len(all) != 2 || all[0].Day != "2026-11-01" {
		t.Errorf("все заметки: %+v", all)
	}
}

func TestWeatherUpsertAndHistory(t *testing.T) {
	s := openTemp(t)
	today := time.Now().Format("2006-01-02")
	w := WeatherDay{Day: today, Lat: 59.95, Lon: 29.5, VisibilityM: 42800, WindMS: 6.5, WaveM: 0.38,
		HasVis: true, HasWind: true, HasWave: true, Source: "test"}
	if err := s.SaveWeather(w); err != nil {
		t.Fatal(err)
	}
	w.WindMS = 9
	s.SaveWeather(w)
	got, ok := s.WeatherForDay(today, 59.95, 29.5)
	if !ok || got.WindMS != 9 || !got.HasWave {
		t.Fatalf("upsert: %+v", got)
	}
	old := w
	old.Day = time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	s.SaveWeather(old)
	h, _ := s.WeatherHistory(59.95, 29.5, 30)
	if len(h) != 1 {
		t.Errorf("история за 30 дней должна исключать запись 40-дневной давности: %d", len(h))
	}
}

func TestWarningsReplaceBySource(t *testing.T) {
	s := openTemp(t)
	s.SaveWarnings("A", []WarningRow{{Area: "NAVAREA XX", Number: "1", Year: 2026, Body: "x"},
		{Area: "NAVAREA XX", Number: "2", Year: 2026, Body: "y"}})
	s.SaveWarnings("B", []WarningRow{{Area: "ПРИП МУРМАНСК", Number: "5", Year: 2026, Body: "z"}})
	s.SaveWarnings("A", []WarningRow{{Area: "NAVAREA XX", Number: "3", Year: 2026, Body: "new"}})
	all, _ := s.Warnings()
	if len(all) != 2 {
		t.Fatalf("ожидалось 2 (A заменён, B сохранён), получено %d", len(all))
	}
	if s.LastWarningFetch().IsZero() {
		t.Error("время загрузки")
	}
}
