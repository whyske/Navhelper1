package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ───────────────── хранение прогнозов и выпусков ИМ УНиО (v4) ─────────────
//
// Зачем: внешние API отдают прогноз максимум на 3 суток вперёд и хранят
// прошлое ограниченно. Чтобы журнал не «худел», приложение при каждом
// обновлении складывает полученный прогноз в базу. Дальше журнал строится
// из базы, а сеть нужна только для добавления новых суток — старые записи
// остаются навсегда, сколько бы времени ни прошло.

// ForecastSnapshot — сохранённый прогноз на конкретные сутки в точке.
type ForecastSnapshot struct {
	ID       int64
	Day      string // YYYY-MM-DD — на какие сутки прогноз
	Lat      float64
	Lon      float64
	Issued   time.Time // когда прогноз был получен
	Source   string    // "open-meteo" и т. п.
	Payload  string    // JSON с построчным прогнозом
}

func (s *Store) migrateV4() error {
	stmts := []string{
		// Снапшоты прогнозов: одна запись на (сутки, точка, время выпуска).
		`CREATE TABLE IF NOT EXISTS forecast_snapshots (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			day        TEXT    NOT NULL,
			lat        REAL    NOT NULL,
			lon        REAL    NOT NULL,
			issued_at  INTEGER NOT NULL,
			source     TEXT    NOT NULL DEFAULT '',
			payload    TEXT    NOT NULL DEFAULT '',
			UNIQUE(day, lat, lon, issued_at)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_fc_snap_day ON forecast_snapshots(day DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_fc_snap_point ON forecast_snapshots(lat, lon, day DESC)`,

		// Выпуски ИМ УНиО (извещения мореплавателям): сохраняются локально,
		// чтобы список выпусков открывался без сети и старые выпуски не терялись.
		`CREATE TABLE IF NOT EXISTS notices (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			key        TEXT    NOT NULL UNIQUE,
			number     string  NOT NULL DEFAULT '',
			title      TEXT    NOT NULL DEFAULT '',
			region     TEXT    NOT NULL DEFAULT '',
			issued     TEXT    NOT NULL DEFAULT '',
			url        TEXT    NOT NULL DEFAULT '',
			body       TEXT    NOT NULL DEFAULT '',
			fetched_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_notices_region ON notices(region, issued DESC)`,

		// Отметки об уведомлениях: что уже показано, чтобы не дублировать.
		`CREATE TABLE IF NOT EXISTS notify_log (
			key     TEXT PRIMARY KEY,
			sent_at INTEGER NOT NULL
		)`,

		// Флаг «нужно уведомление» у заметок календаря.
		`ALTER TABLE cal_notes ADD COLUMN notify INTEGER NOT NULL DEFAULT 0`,

		// Регион и ссылка на источник у навигационных предупреждений:
		// раньше раздел охватывал только Арктику, теперь — все бассейны.
		`ALTER TABLE warnings ADD COLUMN region TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE warnings ADD COLUMN url TEXT NOT NULL DEFAULT ''`,
	}
	for _, st := range stmts {
		if _, err := s.db.Exec(st); err != nil {
			// ALTER TABLE падает, если колонка уже есть — это не ошибка.
			if isDuplicateColumn(err) {
				continue
			}
			return fmt.Errorf("миграция v4: %w\n%s", err, st)
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "duplicate column name") || contains(msg, "already exists")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > len(sub) && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// SaveForecastSnapshot сохраняет прогноз на сутки. Повторная запись с тем же
// временем выпуска обновляет payload, поэтому база не растёт бесконечно.
func (s *Store) SaveForecastSnapshot(day string, lat, lon float64, source string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// Округляем время выпуска до часа: за один час прогноз не меняется
	// настолько, чтобы хранить десятки копий.
	issued := time.Now().Truncate(time.Hour).Unix()
	_, err = s.db.Exec(
		`INSERT INTO forecast_snapshots(day, lat, lon, issued_at, source, payload)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(day, lat, lon, issued_at) DO UPDATE SET
			payload = excluded.payload, source = excluded.source`,
		day, lat, lon, issued, source, string(raw))
	return err
}

// ForecastSnapshots возвращает сохранённые прогнозы за последние days суток
// для точки — по одному (самому свежему) на каждые сутки.
func (s *Store) ForecastSnapshots(lat, lon float64, days int) (map[string]string, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.db.Query(
		`SELECT day, payload FROM forecast_snapshots
		 WHERE lat = ? AND lon = ? AND day >= ?
		 ORDER BY day DESC, issued_at DESC`, lat, lon, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var day, payload string
		if err := rows.Scan(&day, &payload); err != nil {
			return nil, err
		}
		if _, seen := out[day]; !seen {
			out[day] = payload
		}
	}
	return out, rows.Err()
}

// Notice — выпуск извещений мореплавателям (ИМ УНиО).
type Notice struct {
	ID      int64
	Key     string
	Number  string
	Title   string
	Region  string
	Issued  string
	URL     string
	Body    string
	Fetched time.Time
}

// SaveNotices сохраняет выпуски; существующие обновляются по key.
func (s *Store) SaveNotices(list []Notice) error {
	if len(list) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(
		`INSERT INTO notices(key, number, title, region, issued, url, body, fetched_at)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(key) DO UPDATE SET
			number = excluded.number, title = excluded.title, region = excluded.region,
			issued = excluded.issued, url = excluded.url, body = excluded.body,
			fetched_at = excluded.fetched_at`)
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now().Unix()
	for _, n := range list {
		if _, err := st.Exec(n.Key, n.Number, n.Title, n.Region, n.Issued, n.URL, n.Body, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Notices возвращает сохранённые выпуски, свежие сверху.
func (s *Store) Notices(region string, limit int) ([]Notice, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT id, key, number, title, region, issued, url, body, fetched_at FROM notices`
	var args []any
	if region != "" {
		q += ` WHERE region = ?`
		args = append(args, region)
	}
	q += ` ORDER BY issued DESC, id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notice
	for rows.Next() {
		var n Notice
		var ts int64
		if err := rows.Scan(&n.ID, &n.Key, &n.Number, &n.Title, &n.Region,
			&n.Issued, &n.URL, &n.Body, &ts); err != nil {
			return nil, err
		}
		n.Fetched = time.Unix(ts, 0)
		out = append(out, n)
	}
	return out, rows.Err()
}

// NoticeCount — число сохранённых выпусков.
func (s *Store) NoticeCount() int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM notices`).Scan(&n)
	return n
}

// WasNotified сообщает, показывали ли уже уведомление с таким ключом.
func (s *Store) WasNotified(key string) bool {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM notify_log WHERE key = ?`, key).Scan(&one)
	return err == nil && one == 1
}

// MarkNotified запоминает, что уведомление показано.
func (s *Store) MarkNotified(key string) error {
	_, err := s.db.Exec(
		`INSERT INTO notify_log(key, sent_at) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET sent_at = excluded.sent_at`,
		key, time.Now().Unix())
	return err
}

// SetNoteNotify включает или выключает уведомление для заметки.
func (s *Store) SetNoteNotify(id int64, on bool) error {
	_, err := s.db.Exec(`UPDATE cal_notes SET notify = ? WHERE id = ?`, b2i(on), id)
	return err
}

// NotifyNotes — заметки с включённым уведомлением.
func (s *Store) NotifyNotes() ([]CalNote, error) {
	rows, err := s.db.Query(`SELECT ` + calCols + ` FROM cal_notes WHERE notify = 1`)
	if err != nil {
		return nil, err
	}
	return scanCal(rows)
}

var _ = sql.ErrNoRows
