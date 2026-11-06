// Package ui — интерфейс «NavHelper» на Fyne.
// Боковое выезжающее меню (drawer), разделы: Часы, Калькулятор, Календарь,
// Карты, Погода, ИМ УНиО, Настройки. Все модули работают с одной базой SQLite.
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/charts"
	"ru.navhelper/internal/store"
)

// App — общее состояние приложения.
type App struct {
	FA  fyne.App
	Win fyne.Window
	St  *store.Store

	weather  *weatherTab
}

// Run открывает базу, строит drawer-меню и запускает главный цикл.
func Run(a fyne.App) {
	a.Settings().SetTheme(marineTheme{})
	w := a.NewWindow("NavHelper")
	w.Resize(fyne.NewSize(420, 760))

	dbPath := storageDir(a)
	st, err := store.Open(filepath.Join(dbPath, "navhelper.db"))
	if err != nil {
		w.SetContent(widget.NewLabel("Ошибка базы данных: " + err.Error()))
		w.ShowAndRun()
		return
	}

	app := &App{FA: a, Win: w, St: st}

	// Пока каталог карт импортируется (первый запуск), показываем заставку.
	w.SetContent(container.NewCenter(container.NewVBox(
		widget.NewProgressBarInfinite(),
		widget.NewLabel("Подготовка каталога морских карт…"),
	)))

	// Подложка превью карт распаковывается в фоне, чтобы первое превью
	// открылось мгновенно.
	go func() { _ = charts.PreloadLand() }()

	go func() {
		n, ierr := charts.EnsureImported(st.DB())
		fyne.Do(func() {
			app.build(n, ierr)
		})
	}()

	// Возврат на передний план: обновить погоду и предупреждения, если пора.
	a.Lifecycle().SetOnEnteredForeground(func() {
		if app.weather != nil {
			app.weather.syncAsync()
		}
	})
	a.Lifecycle().SetOnStopped(func() { _ = st.Close() })

	w.ShowAndRun()
}

// build создаёт drawer-меню после импорта каталога.
func (a *App) build(chartCount int, importErr error) {
	a.weather = newWeatherTab(a)

	items := []navItem{
		{"Часы", theme.InfoIcon(), newClockSection(a)},
		{"Калькулятор", calculatorIcon, newCalcSection(a)},
		{"Календарь", theme.ListIcon(), newCalendarTab(a)},
		{"Карты", theme.SearchIcon(), newChartsTab(a, chartCount, importErr)},
		{"Погода", theme.VisibilityIcon(), a.weather.content},
	}
	root := newNavRoot(items, "NavHelper")
	a.Win.SetContent(root)

	// Сбор данных при открытии приложения.
	a.weather.syncAsync()

	// Уведомления: восход/заход солнца, новый каталог карт, новые извещения,
	// напоминания из календаря. Проверка идёт, пока приложение открыто.

	// Пока приложение открыто — проверка раз в час (в фоне ОС код не исполняется;
	// пропущенные дни погоды дозаполняются через past_days при следующем запуске).
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for range t.C {
			fyne.Do(func() {
				a.weather.syncAsync()
			})
		}
	}()
}

// storageDir возвращает каталог для файла базы данных.
//
// На Android путьStorage Fyne приходит в виде file:///data/user/0/.../files,
// поэтому Path() может вернуть не файловый путь, а URI. Проверяем результат:
// если каталога нет и создать его нельзя — откатываемся на os.UserConfigDir.
func storageDir(a fyne.App) string {
	var candidates []string
	if u := a.Storage().RootURI(); u != nil {
		p := u.Path()
		if p == "" {
			p = u.String()
		}
		// Отрезаем схему file:// если Path() её сохранил.
		p = strings.TrimPrefix(p, "file://")
		if p != "" {
			candidates = append(candidates, p)
		}
	}
	if d, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(d, "navhelper"))
	}
	candidates = append(candidates, filepath.Join(os.TempDir(), "navhelper"))

	for _, dir := range candidates {
		if err := os.MkdirAll(dir, 0o755); err == nil {
			// Проверяем, что туда реально можно писать.
			probe := filepath.Join(dir, ".write-probe")
			if err := os.WriteFile(probe, []byte("ok"), 0o644); err == nil {
				_ = os.Remove(probe)
				return dir
			}
		}
	}
	return "."
}

// ctx — контекст сетевого запроса с таймаутом.
func ctx(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// wrapLabel — метка с переносом слов.
func wrapLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// bold — жирная метка.
func bold(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

// fieldRow — строка «Жирная подпись: значение», где значение переносится по
// словам и оставляет справа немного места для кнопок (если таковые есть).
func fieldRow(label, value string) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(label+":", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v := wrapLabel(value)
	return container.NewBorder(nil, nil, l, nil, v)
}

// headRow — заголовок карточки с меткой статуса справа (ПРОШЕДШИЙ/СЕГОДНЯ…),
// без наложения текста друг на друга при длинном заголовке.
func headRow(title, tag string) fyne.CanvasObject {
	t := bold(title)
	if tag == "" {
		return t
	}
	tagL := widget.NewLabelWithStyle("["+tag+"]", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
	return container.NewBorder(nil, nil, nil, tagL, t)
}

// errText — текст ошибки для вывода пользователю.
func errText(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return fmt.Sprintf("⚠ Ошибка: %s", s)
}
