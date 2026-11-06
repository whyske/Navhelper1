// Уведомления приложения: восход/заход солнца, новые выпуски карт,
// новые извещения мореплавателям, готовность прогноза погоды и напоминания
// из календаря. Уведомления локальные (система Android), сеть не нужна
// для их показа — она нужна только для загрузки новых данных.
package ui

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/charts"
	"ru.navhelper/internal/navwarn"
	"ru.navhelper/internal/store"
	"ru.navhelper/internal/sun"
)

// notifyKey — ключи настроек уведомлений.
const (
	nkSun    = "notify.sun"
	nkCharts = "notify.charts"
	nkNotices = "notify.notices"
	nkWeather = "notify.weather"
	nkNotes   = "notify.notes"
)

// notifier отправляет локальные уведомления через fyne.App.SendNotification.
type notifier struct {
	a  *App
	st *store.Store
}

func newNotifier(a *App) *notifier { return &notifier{a: a, st: a.St} }

func (n *notifier) enabled(key string) bool {
	return n.st.Setting(key, "1") == "1"
}

// send показывает уведомление, если оно ещё не показывалось (по ключу).
func (n *notifier) send(key, title, body string) {
	if n.st.WasNotified(key) {
		return
	}
	n.a.FA.SendNotification(fyne.NewNotification(title, body))
	_ = n.st.MarkNotified(key)
}

// checkSunsets — уведомление о восходе и заходе для выбранного в «Часах» города.
// Время сверяется с текущим: уведомление уходит, если событие наступает
// в пределах получаса и ещё сегодня не отправлялось.
func (n *notifier) checkSunsets() {
	if !n.enabled(nkSun) {
		return
	}
	city := n.st.Setting("clock.city", wjDefaultName)
	p, ok := wjPresets[city]
	if !ok {
		return
	}
	loc := time.Local
	r := sun.Calculate(time.Now(), p[0], p[1], loc)
	now := time.Now()
	for _, ev := range []struct {
		t    time.Time
		ok   bool
		name string
	}{
		{r.Sunrise, r.HasSunrise, "Восход солнца"},
		{r.Sunset, r.HasSunset, "Заход солнца"},
	} {
		if !ev.ok {
			continue
		}
		d := ev.t.Sub(now)
		if d < 0 || d > 30*time.Minute {
			continue
		}
		key := fmt.Sprintf("sun|%s|%s", city, ev.t.Format("2006-01-02T15:04"))
		n.send(key, ev.name+" — "+city, "Сегодня в "+ev.t.Format("15:04"))
	}
}

// checkCharts — уведомление о новом выпуске каталога морских карт.
// Каталог встроен в приложение, поэтому «новый выпуск» — это смена версии
// встроенных данных (сравниваем хеш каталога с сохранённым).
func (n *notifier) checkCharts(count int) {
	if !n.enabled(nkCharts) || count == 0 {
		return
	}
	prev := n.st.Setting("notify.charts.seen", "")
	cur := fmt.Sprint(count)
	if prev == "" {
		_ = n.st.SetSetting("notify.charts.seen", cur)
		return
	}
	if prev != cur {
		n.send("charts|"+cur, "Обновлён каталог карт",
			fmt.Sprintf("Доступно листов: %d", count))
		_ = n.st.SetSetting("notify.charts.seen", cur)
	}
}

// checkNotices — уведомление о новых извещениях мореплавателям.
func (n *notifier) checkNotices() {
	if !n.enabled(nkNotices) {
		return
	}
	last := n.st.Setting("notify.notices.seen", "")
	all, err := n.st.Warnings()
	if err != nil || len(all) == 0 {
		return
	}
	// Новыми считаем записи, появившиеся после последней проверки.
	var fresh []store.WarningRow
	for _, w := range all {
		if w.FetchedAt.Format(time.RFC3339) > last {
			fresh = append(fresh, w)
		}
	}
	if len(fresh) == 0 {
		if last == "" {
			_ = n.st.SetSetting("notify.notices.seen", time.Now().Format(time.RFC3339))
		}
		return
	}
	w := fresh[0]
	body := fmt.Sprintf("%s %s/%02d", w.Area, w.Number, w.Year%100)
	if len(fresh) > 1 {
		body += fmt.Sprintf(" и ещё %d", len(fresh)-1)
	}
	n.send("notices|"+w.FetchedAt.Format(time.RFC3339), "Новые извещения мореплавателям", body)
	_ = n.st.SetSetting("notify.notices.seen", time.Now().Format(time.RFC3339))
}

// checkNotes — напоминания из календаря: заметки с включённой галочкой
// «уведомлять» на сегодня.
func (n *notifier) checkNotes() {
	if !n.enabled(nkNotes) {
		return
	}
	notes, err := n.st.NotifyNotes()
	if err != nil {
		return
	}
	today := time.Now().Format("2006-01-02")
	wd := store.ISOWeekday(time.Now())
	for _, note := range notes {
		due := note.Day == today || (note.Weekly() && note.Weekday == wd)
		if !due {
			continue
		}
		key := fmt.Sprintf("note|%d|%s", note.ID, today)
		n.send(key, "Напоминание", note.Text)
	}
}

// startNotifications запускает периодическую проверку всех уведомлений.
// Проверка идёт раз в 10 минут, пока приложение открыто; при входе
// в приложение проверка выполняется сразу.
func (a *App) startNotifications(chartCount int) {
	n := newNotifier(a)
	run := func() {
		n.checkCharts(chartCount)
		n.checkSunsets()
		n.checkNotices()
		n.checkNotes()
	}
	run()
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for range t.C {
			fyne.Do(run)
		}
	}()
}

// newNotifySettings — блок настроек уведомлений для раздела «Настройки».
func newNotifySettings(a *App) fyne.CanvasObject {
	rows := []fyne.CanvasObject{
		bold("Уведомления"),
		wrapLabel("Приложение может напоминать о событиях. Все уведомления локальные: " +
			"они не отправляются на сервер."),
	}
	items := []struct {
		key, title string
	}{
		{nkSun, "Восход и заход солнца"},
		{nkCharts, "Новый выпуск каталога карт"},
		{nkNotices, "Новые извещения мореплавателям (ИМ УНиО)"},
		{nkWeather, "Готовность прогноза погоды"},
		{nkNotes, "Напоминания из календаря"},
	}
	for _, it := range items {
		it := it
		chk := widget.NewCheck(it.title, func(on bool) {
			v := "0"
			if on {
				v = "1"
			}
			_ = a.St.SetSetting(it.key, v)
		})
		chk.SetChecked(a.St.Setting(it.key, "1") == "1")
		rows = append(rows, chk)
	}
	btn := widget.NewButtonWithIcon("Проверить уведомления сейчас", theme.InfoIcon(), func() {
		n := newNotifier(a)
		n.checkSunsets()
		n.checkNotices()
		n.checkNotes()
		a.FA.SendNotification(fyne.NewNotification("NavHelper", "Уведомления работают"))
	})
	rows = append(rows, btn)
	return container.NewVBox(rows...)
}

// notifyBellIcon — иконка уведомлений в списке разделов.
var notifyBellIcon fyne.Resource = theme.InfoIcon()

var _ = charts.Count
var _ = navwarn.NewClient
