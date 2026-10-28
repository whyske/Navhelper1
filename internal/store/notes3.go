package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// ─────────────── заметки календаря v3: несколько на день, повтор, цвет ───────────────
//
// Заметка бывает двух видов:
//   - разовая — привязана к дате Day (YYYY-MM-DD);
//   - еженедельная — привязана к дню недели Weekday (1 = пн … 7 = вс) и
//     действует на все такие дни начиная с недели From (YYYY-MM-DD) и далее.
//
// Таблица notes из v1/v2 не меняется (для отката на старую версию); её записи
// один раз переносятся сюда как разовые заметки.

// NoteColors — палитра цветов заметок (hex RGB). Пользователь выбирает любой.
var NoteColors = []string{
	"#E53935", // красный
	"#FB8C00", // оранжевый
	"#FDD835", // жёлтый
	"#43A047", // зелёный
	"#00ACC1", // бирюзовый
	"#1E88E5", // синий
	"#8E24AA", // фиолетовый
	"#6D4C41", // коричневый
	"#546E7A", // серый
}

// DefaultNoteColor — цвет по умолчанию.
const DefaultNoteColor = "#1E88E5"

// CalNote — заметка календаря.
type CalNote struct {
	ID      int64
	Day     string // разовая: дата; еженедельная: ""
	Weekday int    // 0 — разовая; 1..7 — пн..вс
	From    string // еженедельная: с какой даты действует
	Text    string
	Color   string // "#RRGGBB"
	At      time.Time
}

// Weekly — повторяющаяся заметка.
func (n CalNote) Weekly() bool { return n.Weekday >= 1 && n.Weekday <= 7 }

// ISOWeekday — 1 = понедельник … 7 = воскресенье.
func ISOWeekday(t time.Time) int {
	wd := int(t.Weekday())
	if wd == 0 {
		return 7
	}
	return wd
}

func (s *Store) migrateNotes3() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cal_notes (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			day        TEXT    NOT NULL DEFAULT '',
			weekday    INTEGER NOT NULL DEFAULT 0,
			from_day   TEXT    NOT NULL DEFAULT '',
			text       TEXT    NOT NULL,
			color      TEXT    NOT NULL DEFAULT '` + DefaultNoteColor + `',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cal_notes_day ON cal_notes(day)`,
		`CREATE INDEX IF NOT EXISTS idx_cal_notes_wd ON cal_notes(weekday)`,
		// Исключения еженедельных заметок: «в этот день не показывать».
		`CREATE TABLE IF NOT EXISTS cal_note_skips (
			note_id INTEGER NOT NULL,
			day     TEXT    NOT NULL,
			PRIMARY KEY(note_id, day)
		)`,
	}
	for _, st := range stmts {
		if _, err := s.db.Exec(st); err != nil {
			return fmt.Errorf("миграция заметок: %w", err)
		}
	}
	// Однократный перенос заметок v1/v2.
	if s.Setting("notes.v3_migrated", "") == "" {
		_, err := s.db.Exec(`INSERT INTO cal_notes(day, weekday, from_day, text, color, created_at)
			SELECT day, 0, '', text, ?, updated_at FROM notes WHERE text != ''`, DefaultNoteColor)
		if err != nil {
			return fmt.Errorf("перенос старых заметок: %w", err)
		}
		if err := s.SetSetting("notes.v3_migrated", "1"); err != nil {
			return err
		}
	}
	return nil
}

func scanCal(rows *sql.Rows) ([]CalNote, error) {
	defer rows.Close()
	var out []CalNote
	for rows.Next() {
		var n CalNote
		var ts int64
		if err := rows.Scan(&n.ID, &n.Day, &n.Weekday, &n.From, &n.Text, &n.Color, &ts); err != nil {
			return nil, err
		}
		n.At = time.Unix(ts, 0)
		out = append(out, n)
	}
	return out, rows.Err()
}

const calCols = `id, day, weekday, from_day, text, color, created_at`

// AddDayNote добавляет разовую заметку на дату.
func (s *Store) AddDayNote(day, text, color string) (int64, error) {
	if color == "" {
		color = DefaultNoteColor
	}
	r, err := s.db.Exec(`INSERT INTO cal_notes(day, weekday, from_day, text, color, created_at)
		VALUES(?,0,'',?,?,?)`, day, text, color, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

// AddWeeklyNote добавляет заметку на день недели (1..7), действующую с from.
func (s *Store) AddWeeklyNote(weekday int, from, text, color string) (int64, error) {
	if weekday < 1 || weekday > 7 {
		return 0, fmt.Errorf("неверный день недели %d", weekday)
	}
	if color == "" {
		color = DefaultNoteColor
	}
	r, err := s.db.Exec(`INSERT INTO cal_notes(day, weekday, from_day, text, color, created_at)
		VALUES('',?,?,?,?,?)`, weekday, from, text, color, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

// UpdateCalNote меняет текст и цвет заметки.
func (s *Store) UpdateCalNote(id int64, text, color string) error {
	_, err := s.db.Exec(`UPDATE cal_notes SET text = ?, color = ? WHERE id = ?`, text, color, id)
	return err
}

// DeleteCalNote удаляет заметку (для еженедельной — со всех недель).
func (s *Store) DeleteCalNote(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM cal_note_skips WHERE note_id = ?`, id); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM cal_notes WHERE id = ?`, id)
	return err
}

// SkipWeeklyOn убирает еженедельную заметку только с одного дня (остальные
// недели не затрагиваются).
func (s *Store) SkipWeeklyOn(id int64, day string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO cal_note_skips(note_id, day) VALUES(?,?)`, id, day)
	return err
}

// UnskipWeeklyOn возвращает заметку на день, с которого её убрали.
func (s *Store) UnskipWeeklyOn(id int64, day string) error {
	_, err := s.db.Exec(`DELETE FROM cal_note_skips WHERE note_id = ? AND day = ?`, id, day)
	return err
}

// SkipCount — сколько дней исключено у еженедельной заметки.
func (s *Store) SkipCount(id int64) int {
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM cal_note_skips WHERE note_id = ?`, id).Scan(&n)
	return n
}

// skipsBetween — множество «id|день» исключений в диапазоне.
func (s *Store) skipsBetween(from, to string) map[string]bool {
	out := map[string]bool{}
	rows, err := s.db.Query(`SELECT note_id, day FROM cal_note_skips WHERE day BETWEEN ? AND ?`, from, to)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var d string
		if rows.Scan(&id, &d) == nil {
			out[fmt.Sprintf("%d|%s", id, d)] = true
		}
	}
	return out
}

// WeeklyNotes — все еженедельные заметки (по дню недели, затем по id).
func (s *Store) WeeklyNotes() ([]CalNote, error) {
	rows, err := s.db.Query(`SELECT ` + calCols + ` FROM cal_notes WHERE weekday > 0 ORDER BY weekday, id`)
	if err != nil {
		return nil, err
	}
	return scanCal(rows)
}

// DayNotesAll — все разовые заметки, новые даты сверху.
func (s *Store) DayNotesAll() ([]CalNote, error) {
	rows, err := s.db.Query(`SELECT ` + calCols + ` FROM cal_notes WHERE weekday = 0 ORDER BY day DESC, id`)
	if err != nil {
		return nil, err
	}
	return scanCal(rows)
}

// NotesOn — все заметки, действующие в день t: сначала еженедельные, потом разовые.
func (s *Store) NotesOn(t time.Time) ([]CalNote, error) {
	day := t.Format("2006-01-02")
	rows, err := s.db.Query(`SELECT `+calCols+` FROM cal_notes
		WHERE (weekday = 0 AND day = ?) OR (weekday = ? AND from_day <= ?)
		ORDER BY weekday DESC, id`, day, ISOWeekday(t), day)
	if err != nil {
		return nil, err
	}
	list, err := scanCal(rows)
	if err != nil {
		return nil, err
	}
	skips := s.skipsBetween(day, day)
	out := list[:0]
	for _, n := range list {
		if n.Weekly() && skips[fmt.Sprintf("%d|%s", n.ID, day)] {
			continue
		}
		out = append(out, n)
	}
	return out, nil
}

// NotesRange — заметки по дням в диапазоне [from, to] включительно (для
// цветных отметок в сетке календаря). Ключ — YYYY-MM-DD.
func (s *Store) NotesRange(from, to time.Time) (map[string][]CalNote, error) {
	f, t := from.Format("2006-01-02"), to.Format("2006-01-02")
	rows, err := s.db.Query(`SELECT `+calCols+` FROM cal_notes
		WHERE (weekday = 0 AND day BETWEEN ? AND ?) OR (weekday > 0 AND from_day <= ?)
		ORDER BY id`, f, t, t)
	if err != nil {
		return nil, err
	}
	list, err := scanCal(rows)
	if err != nil {
		return nil, err
	}
	skips := s.skipsBetween(f, t)
	out := map[string][]CalNote{}
	var weekly []CalNote
	for _, n := range list {
		if n.Weekly() {
			weekly = append(weekly, n)
			continue
		}
		out[n.Day] = append(out[n.Day], n)
	}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		wd := ISOWeekday(d)
		var w []CalNote
		for _, n := range weekly {
			if n.Weekday == wd && n.From <= key && !skips[fmt.Sprintf("%d|%s", n.ID, key)] {
				w = append(w, n)
			}
		}
		if len(w) > 0 {
			out[key] = append(w, out[key]...)
		}
	}
	for k := range out {
		l := out[k]
		sort.SliceStable(l, func(i, j int) bool { return l[i].Weekly() && !l[j].Weekly() })
	}
	return out, nil
}
