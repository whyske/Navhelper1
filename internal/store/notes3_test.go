package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCalNotesV3(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// 28.10.2026 — среда.
	wed := time.Date(2026, 10, 28, 0, 0, 0, 0, time.Local)
	if ISOWeekday(wed) != 3 {
		t.Fatalf("weekday %d", ISOWeekday(wed))
	}
	s.AddDayNote("2026-10-28", "Отход 08:00", "#E53935")
	s.AddDayNote("2026-10-28", "Получить карты", "#43A047")
	s.AddWeeklyNote(1, "2026-10-26", "Учебная тревога", "#FDD835") // каждый понедельник

	on, _ := s.NotesOn(wed)
	if len(on) != 2 {
		t.Fatalf("на среду ожидалось 2, есть %d", len(on))
	}
	mon := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local)
	on, _ = s.NotesOn(mon)
	if len(on) != 1 || on[0].Text != "Учебная тревога" {
		t.Fatalf("понедельник: %+v", on)
	}
	// До начала действия — нет.
	on, _ = s.NotesOn(time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local))
	if len(on) != 0 {
		t.Fatalf("до from: %+v", on)
	}
	m, _ := s.NotesRange(time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), time.Date(2026, 11, 30, 0, 0, 0, 0, time.Local))
	mons := 0
	for _, l := range m {
		for _, n := range l {
			if n.Weekly() {
				mons++
			}
		}
	}
	if mons != 6 { // 26.10, 2, 9, 16, 23, 30.11
		t.Fatalf("понедельников с заметкой %d", mons)
	}
	if len(m["2026-10-28"]) != 2 {
		t.Fatal("range day notes")
	}
}

func TestNotesMigrationFromV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s, _ := Open(path)
	_, _ = s.db.Exec(`DELETE FROM settings WHERE key='notes.v3_migrated'`)
	s.SetNote("2026-10-25", "старая заметка")
	s.Close()
	s, _ = Open(path)
	defer s.Close()
	on, _ := s.NotesOn(time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local))
	if len(on) != 1 || on[0].Text != "старая заметка" {
		t.Fatalf("%+v", on)
	}
	s.Close()
	s, _ = Open(path) // повторно не дублирует
	on, _ = s.NotesOn(time.Date(2026, 10, 25, 0, 0, 0, 0, time.Local))
	if len(on) != 1 {
		t.Fatalf("дубль: %d", len(on))
	}
}

func TestWeeklySkip(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id, _ := s.AddWeeklyNote(1, "2026-10-26", "Вахта", "#E53935")
	m1 := time.Date(2026, 11, 2, 0, 0, 0, 0, time.Local)
	m2 := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)
	if err := s.SkipWeeklyOn(id, "2026-11-02"); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.NotesOn(m1); len(on) != 0 {
		t.Fatalf("02.11 исключён, а заметок %d", len(on))
	}
	if on, _ := s.NotesOn(m2); len(on) != 1 {
		t.Fatalf("09.11 должна остаться, а заметок %d", len(on))
	}
	r, _ := s.NotesRange(time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local), time.Date(2026, 11, 30, 0, 0, 0, 0, time.Local))
	if len(r["2026-11-02"]) != 0 || len(r["2026-11-09"]) != 1 || len(r["2026-11-16"]) != 1 {
		t.Fatalf("диапазон: %v", r)
	}
	if s.SkipCount(id) != 1 {
		t.Fatal("счётчик пропусков")
	}
	_ = s.DeleteCalNote(id)
	if s.SkipCount(id) != 0 {
		t.Fatal("пропуски не удалены вместе с заметкой")
	}
}
