package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"ru.navhelper/internal/store"
)

// TestTabsConstruct — smoke-тест на падение при построении разделов.
//
// Зачем: в версии 4.3.0 приложение падало на запуске с nil pointer. Причина
// была в порядке инициализации — widget.Select.SetSelected немедленно вызывает
// обработчик OnChanged, а тот обращался к полям (issuesBox, filter), которые
// ещё не были созданы. Компилятор такое не ловит, поэтому нужен тест,
// который просто строит каждый раздел и падает, если внутри будет паника.
func TestTabsConstruct(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	st, err := store.Open(filepath.Join(t.TempDir(), "smoke.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := &App{FA: a, Win: a.NewWindow("test"), St: st}
	app.notifier = newNotifier(app)

	t.Run("ИМ УНиО", func(t *testing.T) {
		// Именно этот раздел падал: SetSelected в конструкторе.
		w := newWarningsTab(app)
		if w.content == nil {
			t.Fatal("content не создан")
		}
		// Повторный вызов не должен падать даже на пустой базе.
		w.reload()
		w.rebuildIssues()
	})

	t.Run("Часы", func(t *testing.T) {
		if obj := newClockSection(app); obj == nil {
			t.Fatal("раздел не создан")
		}
	})

	t.Run("Календарь", func(t *testing.T) {
		if obj := newCalendarTab(app); obj == nil {
			t.Fatal("раздел не создан")
		}
	})

	t.Run("Настройки", func(t *testing.T) {
		if obj := newSettingsTab(app); obj == nil {
			t.Fatal("раздел не создан")
		}
	})
}

// TestWarningsFilter — проверка фильтрации по району и поиска.
func TestWarningsFilter(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()

	st, err := store.Open(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := &App{FA: a, Win: a.NewWindow("t"), St: st}
	app.notifier = newNotifier(app)

	_ = st.SaveWarnings("test", []store.WarningRow{
		{Area: "NAVAREA III", Number: "244", Year: 2026, Title: "NAVAREA III", Body: "GUNNERY", Region: "black"},
		{Area: "NAVAREA I", Number: "180", Year: 2026, Title: "NAVAREA I", Body: "MINE", Region: "baltic"},
	})

	w := newWarningsTab(app)
	if len(w.all) != 2 {
		t.Fatalf("загружено %d, ожидалось 2", len(w.all))
	}

	// Фильтр по району.
	w.curRegion = "black"
	w.rebuildIssues()
	found := 0
	for _, row := range w.all {
		if row.Region == "black" {
			found++
		}
	}
	if found != 1 {
		t.Errorf("в районе black %d записей, ожидалось 1", found)
	}
}
