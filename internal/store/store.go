// Package store отвечает за работу с базой SQLite приложения:
// схема, миграции и все запросы. Используется один файл базы navhelper.db,
// который лежит в приватном каталоге приложения.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store — обёртка над базой данных приложения.
type Store struct {
	db *sql.DB
}

// Open открывает (или создаёт) базу данных по указанному пути и применяет миграции.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("не удалось создать каталог %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite3", path+"?_busy_timeout=8000&_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть базу: %w", err)
	}
	// Мобильная база одна: держим единственное соединение, иначе
	// на Android ловим «database is locked».
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("база недоступна: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	if err := s.migrateNotes3(); err != nil {
		return nil, err
	}
	return s, nil
}

// Close закрывает базу.
func (s *Store) Close() error { return s.db.Close() }

// DB возвращает соединение — нужно пакету charts, который держит
// каталог карт в той же общей базе.
func (s *Store) DB() *sql.DB { return s.db }

// migrate применяет схему. Все запросы идемпотентны (IF NOT EXISTS),
// поэтому миграция безопасна при каждом запуске.
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS calc_history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			expr       TEXT    NOT NULL,
			result     TEXT    NOT NULL,
			result_sec INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_calc_history_created ON calc_history(created_at DESC)`,

		`CREATE TABLE IF NOT EXISTS notes (
			day        TEXT PRIMARY KEY,
			text       TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS intervals (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			from_ts    INTEGER NOT NULL,
			to_ts      INTEGER NOT NULL,
			from_label TEXT NOT NULL DEFAULT '',
			to_label   TEXT NOT NULL DEFAULT '',
			result     TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS weather_daily (
			day            TEXT    NOT NULL,
			lat            REAL    NOT NULL,
			lon            REAL    NOT NULL,
			place          TEXT    NOT NULL DEFAULT '',
			visibility_m   REAL    NOT NULL DEFAULT 0,
			wind_ms        REAL    NOT NULL DEFAULT 0,
			wave_m         REAL    NOT NULL DEFAULT 0,
			has_visibility INTEGER NOT NULL DEFAULT 0,
			has_wind       INTEGER NOT NULL DEFAULT 0,
			has_wave       INTEGER NOT NULL DEFAULT 0,
			source         TEXT    NOT NULL DEFAULT '',
			created_at     INTEGER NOT NULL,
			PRIMARY KEY(day, lat, lon)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_weather_day ON weather_daily(day DESC)`,

		`CREATE TABLE IF NOT EXISTS warnings (
			key        TEXT PRIMARY KEY,
			area       TEXT NOT NULL,
			number     TEXT NOT NULL,
			year       INTEGER NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			body       TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL DEFAULT '',
			fetched_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_warnings_area ON warnings(area)`,
	}
	for _, st := range stmts {
		if _, err := s.db.Exec(st); err != nil {
			return fmt.Errorf("миграция не применена: %w\n%s", err, st)
		}
	}
	return nil
}

// ─────────────────────────── настройки ───────────────────────────

// Setting возвращает строковую настройку или значение по умолчанию.
func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// SetSetting записывает настройку.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// SettingFloat возвращает числовую настройку.
func (s *Store) SettingFloat(key string, def float64) float64 {
	var v float64
	if err := s.db.QueryRow(`SELECT CAST(value AS REAL) FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

// ─────────────────────────── история калькулятора ───────────────────────────

// CalcEntry — запись истории вычислений.
type CalcEntry struct {
	ID     int64
	Expr   string
	Result string
	Sec    int64
	At     time.Time
}

// AddCalc сохраняет выражение и его результат.
func (s *Store) AddCalc(expr, result string, sec int64) error {
	_, err := s.db.Exec(
		`INSERT INTO calc_history(expr, result, result_sec, created_at) VALUES(?,?,?,?)`,
		expr, result, sec, time.Now().Unix())
	if err != nil {
		return err
	}
	// Держим в истории не больше 100 записей.
	_, err = s.db.Exec(
		`DELETE FROM calc_history WHERE id NOT IN
			(SELECT id FROM calc_history ORDER BY id DESC LIMIT 100)`)
	return err
}

// CalcHistory возвращает последние записи истории (свежие сверху).
func (s *Store) CalcHistory(limit int) ([]CalcEntry, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(
		`SELECT id, expr, result, result_sec, created_at
		 FROM calc_history ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CalcEntry
	for rows.Next() {
		var e CalcEntry
		var ts int64
		if err := rows.Scan(&e.ID, &e.Expr, &e.Result, &e.Sec, &ts); err != nil {
			return nil, err
		}
		e.At = time.Unix(ts, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ClearCalcHistory очищает историю вычислений.
func (s *Store) ClearCalcHistory() error {
	_, err := s.db.Exec(`DELETE FROM calc_history`)
	return err
}

// ─────────────────────────── заметки календаря ───────────────────────────

// Note — заметка на конкретный день.
type Note struct {
	Day  string // YYYY-MM-DD
	Text string
	At   time.Time
}

// SetNote создаёт или обновляет заметку дня. Пустой текст удаляет заметку.
func (s *Store) SetNote(day, text string) error {
	if text == "" {
		_, err := s.db.Exec(`DELETE FROM notes WHERE day = ?`, day)
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO notes(day, text, updated_at) VALUES(?,?,?)
		 ON CONFLICT(day) DO UPDATE SET text = excluded.text, updated_at = excluded.updated_at`,
		day, text, time.Now().Unix())
	return err
}

// Note возвращает заметку дня.
func (s *Store) Note(day string) (Note, bool) {
	var n Note
	var ts int64
	err := s.db.QueryRow(`SELECT day, text, updated_at FROM notes WHERE day = ?`, day).
		Scan(&n.Day, &n.Text, &ts)
	if err != nil {
		return Note{}, false
	}
	n.At = time.Unix(ts, 0)
	return n, true
}

// NotesMonth возвращает заметки за месяц (year, month 1..12) по дням.
func (s *Store) NotesMonth(year int, month time.Month) (map[string]string, error) {
	prefix := fmt.Sprintf("%04d-%02d-", year, int(month))
	rows, err := s.db.Query(`SELECT day, text FROM notes WHERE day LIKE ?`, prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var d, t string
		if err := rows.Scan(&d, &t); err != nil {
			return nil, err
		}
		out[d] = t
	}
	return out, rows.Err()
}

// AllNotes возвращает все заметки (для списка).
func (s *Store) AllNotes() ([]Note, error) {
	rows, err := s.db.Query(`SELECT day, text, updated_at FROM notes ORDER BY day DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Note
	for rows.Next() {
		var n Note
		var ts int64
		if err := rows.Scan(&n.Day, &n.Text, &ts); err != nil {
			return nil, err
		}
		n.At = time.Unix(ts, 0)
		out = append(out, n)
	}
	return out, rows.Err()
}

// AddInterval сохраняет выполненный расчёт интервала между датами.
func (s *Store) AddInterval(from, to time.Time, fromLabel, toLabel, result string) error {
	_, err := s.db.Exec(
		`INSERT INTO intervals(from_ts, to_ts, from_label, to_label, result, created_at)
		 VALUES(?,?,?,?,?,?)`,
		from.Unix(), to.Unix(), fromLabel, toLabel, result, time.Now().Unix())
	return err
}

// ─────────────────────────── погода ───────────────────────────

// WeatherDay — сохранённая суточная погода.
type WeatherDay struct {
	Day         string
	Lat, Lon    float64
	Place       string
	VisibilityM float64
	WindMS      float64
	WaveM       float64
	HasVis      bool
	HasWind     bool
	HasWave     bool
	Source      string
}

// SaveWeather вставляет или обновляет запись погоды за день.
func (s *Store) SaveWeather(w WeatherDay) error {
	_, err := s.db.Exec(
		`INSERT INTO weather_daily(day, lat, lon, place, visibility_m, wind_ms, wave_m,
			has_visibility, has_wind, has_wave, source, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(day, lat, lon) DO UPDATE SET
			place = excluded.place,
			visibility_m = excluded.visibility_m,
			wind_ms = excluded.wind_ms,
			wave_m = excluded.wave_m,
			has_visibility = excluded.has_visibility,
			has_wind = excluded.has_wind,
			has_wave = excluded.has_wave,
			source = excluded.source,
			created_at = excluded.created_at`,
		w.Day, w.Lat, w.Lon, w.Place, w.VisibilityM, w.WindMS, w.WaveM,
		b2i(w.HasVis), b2i(w.HasWind), b2i(w.HasWave), w.Source, time.Now().Unix())
	return err
}

// WeatherForDay сообщает, есть ли запись за день и точку.
func (s *Store) WeatherForDay(day string, lat, lon float64) (WeatherDay, bool) {
	var w WeatherDay
	var hv, hw, hwl int
	err := s.db.QueryRow(
		`SELECT day, lat, lon, place, visibility_m, wind_ms, wave_m,
			has_visibility, has_wind, has_wave, source
		 FROM weather_daily WHERE day = ? AND lat = ? AND lon = ?`, day, lat, lon).
		Scan(&w.Day, &w.Lat, &w.Lon, &w.Place, &w.VisibilityM, &w.WindMS, &w.WaveM,
			&hv, &hw, &hwl, &w.Source)
	if err != nil {
		return WeatherDay{}, false
	}
	w.HasVis, w.HasWind, w.HasWave = hv == 1, hw == 1, hwl == 1
	return w, true
}

// WeatherHistory возвращает историю погоды за последние days дней.
func (s *Store) WeatherHistory(lat, lon float64, days int) ([]WeatherDay, error) {
	if days <= 0 {
		days = 14
	}
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(
		`SELECT day, lat, lon, place, visibility_m, wind_ms, wave_m,
			has_visibility, has_wind, has_wave, source
		 FROM weather_daily WHERE lat = ? AND lon = ? AND day >= ?
		 ORDER BY day DESC`, lat, lon, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WeatherDay
	for rows.Next() {
		var w WeatherDay
		var hv, hw, hwl int
		if err := rows.Scan(&w.Day, &w.Lat, &w.Lon, &w.Place, &w.VisibilityM,
			&w.WindMS, &w.WaveM, &hv, &hw, &hwl, &w.Source); err != nil {
			return nil, err
		}
		w.HasVis, w.HasWind, w.HasWave = hv == 1, hw == 1, hwl == 1
		out = append(out, w)
	}
	return out, rows.Err()
}

// ─────────────────────────── предупреждения ───────────────────────────

// WarningRow — сохранённое навигационное предупреждение.
type WarningRow struct {
	Key       string
	Area      string
	Number    string
	Year      int
	Title     string
	Body      string
	Source    string
	FetchedAt time.Time
}

// SaveWarnings заменяет набор предупреждений одним источником целиком:
// удаляет старые записи этого источника и пишет новые.
func (s *Store) SaveWarnings(source string, list []WarningRow) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM warnings WHERE source = ?`, source); err != nil {
		return err
	}
	now := time.Now().Unix()
	for _, w := range list {
		key := w.Area + "/" + w.Number + "/" + fmt.Sprint(w.Year)
		if _, err := tx.Exec(
			`INSERT INTO warnings(key, area, number, year, title, body, source, fetched_at)
			 VALUES(?,?,?,?,?,?,?,?)
			 ON CONFLICT(key) DO UPDATE SET
				title = excluded.title, body = excluded.body,
				source = excluded.source, fetched_at = excluded.fetched_at`,
			key, w.Area, w.Number, w.Year, w.Title, w.Body, source, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Warnings возвращает сохранённые предупреждения (свежие сверху).
func (s *Store) Warnings() ([]WarningRow, error) {
	rows, err := s.db.Query(
		`SELECT key, area, number, year, title, body, source, fetched_at
		 FROM warnings ORDER BY year DESC, number DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WarningRow
	for rows.Next() {
		var w WarningRow
		var ts int64
		if err := rows.Scan(&w.Key, &w.Area, &w.Number, &w.Year, &w.Title,
			&w.Body, &w.Source, &ts); err != nil {
			return nil, err
		}
		w.FetchedAt = time.Unix(ts, 0)
		out = append(out, w)
	}
	return out, rows.Err()
}

// LastWarningFetch возвращает время последней загрузки предупреждений.
func (s *Store) LastWarningFetch() time.Time {
	var ts int64
	if err := s.db.QueryRow(`SELECT MAX(fetched_at) FROM warnings`).Scan(&ts); err != nil {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
