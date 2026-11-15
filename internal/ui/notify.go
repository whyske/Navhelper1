// Уведомления приложения.
//
// ═══════════════════════════════════════════════════════════════════════════
//  КАК ЭТО РАБОТАЕТ И ПОЧЕМУ ИМЕННО ТАК
// ═══════════════════════════════════════════════════════════════════════════
//
// Используется штатный механизм Fyne:
//   fyne.App.ScheduleNotification(n, when)  — поставить уведомление на время
//   fyne.App.SendNotification(n)            — показать немедленно
//
// На Android ScheduleNotification уходит в AlarmManager через нативный мост,
// поэтому уведомление придёт, даже если приложение закрыто. Для этого в
// AndroidManifest.xml обязателен <receiver org.golang.app.FyneNotificationReceiver>,
// иначе будильник не сработает (см. комментарий в манифесте).
//
// Расписание строится «на сутки вперёд» и пересобирается:
//   • при каждом запуске приложения;
//   • при возврате из фона;
//   • раз в час, пока приложение открыто.
// Полноценного фонового сервиса нет: Android убивает Go-процесс, поэтому
// разово запланированные будильники — единственный надёжный способ.
//
// ═══════════════════════════════════════════════════════════════════════════
//  ВАЖНО: НЕ УДАЛЯТЬ ЭТОТ ФАЙЛ
// ═══════════════════════════════════════════════════════════════════════════
// На него завязаны разделы «Часы», «Погода», «ИМ УНиО», «Календарь» и
// «Настройки» (переключатели). Если уведомления не нужны — их выключают
// галочками в настройках, а не удалением кода.
package ui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/store"
	"ru.navhelper/internal/sun"
)

// ─────────────────────────── ключи настроек уведомлений ───────────────────

const (
	// Каждый ключ — отдельный переключатель в разделе «Настройки».
	notifyKeySun     = "notify.sun"     // восход и заход солнца
	notifyKeyWeather = "notify.weather" // прогноз на сегодня
	notifyKeyIM      = "notify.im"      // новый выпуск ИМ УНиО
	notifyKeyNotes   = "notify.notes"   // напоминания из календаря
)

// notifyDefault — что включено при первом запуске. Солнце и заметки включены,
// погода выключена (это ежедневное сообщение, многих оно раздражает).
var notifyDefault = map[string]bool{
	notifyKeySun:     true,
	notifyKeyWeather: false,
	notifyKeyIM:      true,
	notifyKeyNotes:   true,
}

// notifier — менеджер уведомлений. Один на приложение.
type notifier struct {
	a *App

	mu      sync.Mutex
	running bool
	// send — функция отправки, подменяется в тестах.
	send func(*fyne.Notification)
	// schedule — функция планирования на будущее время.
	schedule func(*fyne.Notification, time.Time) error
}

// newNotifier создаёт менеджер, привязанный к приложению.
func newNotifier(a *App) *notifier {
	n := &notifier{a: a}
	n.send = a.FA.SendNotification
	n.schedule = func(nf *fyne.Notification, when time.Time) error {
		_, err := a.FA.ScheduleNotification(nf, when)
		return err
	}
	return n
}

// enabled сообщает, включён ли конкретный тип уведомлений.
func (n *notifier) enabled(key string) bool {
	def := notifyDefault[key]
	return n.a.St.Setting(key, boolStr(def)) == "1"
}

// setEnabled включает или выключает тип уведомлений.
func (n *notifier) setEnabled(key string, on bool) error {
	return n.a.St.SetSetting(key, boolStr(on))
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// ─────────────────────────────── отправка ────────────────────────────────

// sendOnce отправляет уведомление не чаще одного раза за ключ.
// Ключ хранится в таблице notify_log, чтобы после перезапуска не дублировать
// (например, «восход сегодня» не должен приходить дважды).
func (n *notifier) sendOnce(key, title, body string) bool {
	if n.a.St.WasNotified(key) {
		return false
	}
	n.send(fyne.NewNotification(title, body))
	_ = n.a.St.MarkNotified(key)
	return true
}

// ─────────────────────────── восход и заход солнца ───────────────────────

// scheduleSun ставит уведомления о восходе и заходе на сегодня и завтра
// для города, выбранного в разделе «Часы».
func (n *notifier) scheduleSun() {
	if !n.enabled(notifyKeySun) {
		return
	}
	city := n.a.St.Setting("clock.city", wjDefaultName)
	pt, ok := wjPresets[city]
	if !ok {
		return
	}
	lat, lon := pt[0], pt[1]
	// Часовой пояс берём по названию города (см. cityZones в clock.go),
	// а не по долготе — иначе Севастополь получает UTC+2 вместо UTC+3.
	loc := zoneForCity(city, lon)

	now := time.Now()
	for d := 0; d < 2; d++ { // сегодня и завтра
		day := now.In(loc).AddDate(0, 0, d)
		r := sun.Calculate(day, lat, lon, loc)
		if !r.HasSunrise || !r.HasSunset {
			continue // полярный день/ночь — восхода и захода нет
		}
		dateTag := day.Format("2006-01-02")

		if r.Sunrise.After(now) {
			key := "sunrise:" + city + ":" + dateTag
			title := "Восход солнца"
			body := fmt.Sprintf("%s — %s. Световой день %s.",
				city, r.Sunrise.Format("15:04"), fmtDuration(r.DayLength))
			_ = n.schedule(fyne.NewNotification(title, body), r.Sunrise)
			_ = key // ключ нужен только при немедленной отправке ниже
		}
		if r.Sunset.After(now) {
			title := "Заход солнца"
			body := fmt.Sprintf("%s — %s. Световой день %s.",
				city, r.Sunset.Format("15:04"), fmtDuration(r.DayLength))
			_ = n.schedule(fyne.NewNotification(title, body), r.Sunset)
		}
	}
}

// checkSunNow — если восход или заход уже наступил в последние 15 минут,
// а приложение как раз открыто, показать уведомление сразу. Нужно на случай,
// когда будильник не сработал (например, система ограничила фоновые задачи).
func (n *notifier) checkSunNow() {
	if !n.enabled(notifyKeySun) {
		return
	}
	city := n.a.St.Setting("clock.city", wjDefaultName)
	pt, ok := wjPresets[city]
	if !ok {
		return
	}
	loc := zoneForCity(city, pt[1])
	now := time.Now()
	r := sun.Calculate(now, pt[0], pt[1], loc)
	dateTag := now.In(loc).Format("2006-01-02")

	if r.HasSunrise && now.Sub(r.Sunrise) >= 0 && now.Sub(r.Sunrise) < 15*time.Minute {
		n.sendOnce("sunrise:"+city+":"+dateTag, "Восход солнца",
			fmt.Sprintf("%s — %s. Световой день %s.",
				city, r.Sunrise.Format("15:04"), fmtDuration(r.DayLength)))
	}
	if r.HasSunset && now.Sub(r.Sunset) >= 0 && now.Sub(r.Sunset) < 15*time.Minute {
		n.sendOnce("sunset:"+city+":"+dateTag, "Заход солнца",
			fmt.Sprintf("%s — %s. Световой день %s.",
				city, r.Sunset.Format("15:04"), fmtDuration(r.DayLength)))
	}
}

// fmtDuration — «9 ч 37 мин».
func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%d ч %d мин", h, m)
}

// ──────────────────────────────── погода ─────────────────────────────────

// pushWeather шлёт прогноз на сегодня. Вызывается из кода погоды после
// успешной загрузки данных, поэтому дополнительных запросов не делает.
func (n *notifier) pushWeather(periods []wjPeriod) {
	if !n.enabled(notifyKeyWeather) || len(periods) == 0 {
		return
	}
	day := wjDefaultName
	if v := n.a.St.Setting(wjKeyPlace, wjDefaultName); v != "" {
		day = v
	}
	// Берём период с меткой «ДЕЙСТВУЮЩИЙ» — это прогноз на текущие сутки.
	var target *wjPeriod
	for i := range periods {
		if periods[i].Label == "ДЕЙСТВУЮЩИЙ" {
			target = &periods[i]
			break
		}
	}
	if target == nil {
		target = &periods[0]
	}

	var parts []string
	for _, l := range target.Forecast {
		parts = append(parts, l.Label+": "+l.Value)
	}
	body := strings.Join(parts, "; ")
	if body == "" {
		return
	}
	// Одна сводка в сутки.
	key := "weather:" + day + ":" + time.Now().Format("2006-01-02")
	n.sendOnce(key, "Прогноз на сегодня — "+day, body)
}

// ──────────────────────────────── ИМ УНиО ────────────────────────────────

// pushNewWarnings сообщает о новых выпусках ИМ УНиО. Сравнивает текущий набор
// с тем, что уже лежит в базе: если появились новые ключи — уведомляет.
// Вызывается после успешной загрузки предупреждений.
func (n *notifier) pushNewWarnings(before map[string]bool, after []store.WarningRow) {
	if !n.enabled(notifyKeyIM) {
		return
	}
	var fresh []store.WarningRow
	for _, w := range after {
		key := w.Area + "/" + w.Number + "/" + fmt.Sprint(w.Year)
		if !before[key] {
			fresh = append(fresh, w)
		}
	}
	if len(fresh) == 0 {
		return
	}
	// Не больше трёх строк, иначе уведомление нечитаемо.
	const maxLines = 3
	var lines []string
	for i, w := range fresh {
		if i >= maxLines {
			lines = append(lines, fmt.Sprintf("…и ещё %d", len(fresh)-maxLines))
			break
		}
		t := w.Title
		if t == "" {
			t = w.Area + " " + w.Number
		}
		lines = append(lines, w.Area+" "+w.Number+"/"+fmt.Sprint(w.Year%100)+" — "+t)
	}
	title := "Новый выпуск ИМ УНиО"
	if len(fresh) > 1 {
		title = fmt.Sprintf("Новые выпуски ИМ УНиО: %d", len(fresh))
	}
	// Ключ включает дату и число новых — повторно за тот же день не придёт.
	key := "im:" + time.Now().Format("2006-01-02") + ":" + fmt.Sprint(len(fresh))
	n.sendOnce(key, title, strings.Join(lines, "\n"))
}

// ─────────────────────────── заметки календаря ───────────────────────────

// scheduleNotes ставит напоминания по заметкам с галочкой «уведомлять».
// Разовые заметки напоминают в день записи, повторяющиеся — в ближайший
// будущий такой день недели.
func (n *notifier) scheduleNotes() {
	if !n.enabled(notifyKeyNotes) {
		return
	}
	notes, err := n.a.St.NotifyNotes()
	if err != nil {
		return
	}
	now := time.Now()
	for _, note := range notes {
		text := strings.TrimSpace(note.Text)
		if text == "" {
			continue
		}

		var fireAt time.Time
		var dateTag string
		if note.Weekly() {
			// Ближайший такой день недели в 09:00.
			want := note.Weekday
			cur := int(now.In(time.Local).Weekday())
			if cur == 0 {
				cur = 7 // воскресенье в ISO — 7, а не 0
			}
			shift := (want - cur + 7) % 7
			d := now.AddDate(0, 0, shift)
			fireAt = time.Date(d.Year(), d.Month(), d.Day(), 9, 0, 0, 0, time.Local)
			if fireAt.Before(now.Add(time.Minute)) {
				fireAt = fireAt.AddDate(0, 0, 7)
			}
			dateTag = fireAt.Format("2006-01-02")
		} else {
			if note.Day == "" {
				continue
			}
			d, perr := time.ParseInLocation("2006-01-02", note.Day, time.Local)
			if perr != nil {
				continue
			}
			fireAt = time.Date(d.Year(), d.Month(), d.Day(), 9, 0, 0, 0, time.Local)
			dateTag = note.Day
			if fireAt.Before(now.Add(time.Minute)) {
				continue // дата уже прошла
			}
		}
		title := "Заметка на " + fireAt.Format("02.01")
		_ = n.schedule(fyne.NewNotification(title, text), fireAt)
		_ = dateTag
	}
}

// ─────────────────────────── общий пересбор расписания ───────────────────

// refresh пересобирает все расписания. Безопасно вызывать часто: параллельные
// вызовы отсекаются флагом running.
func (n *notifier) refresh() {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	n.mu.Unlock()

	go func() {
		defer func() {
			n.mu.Lock()
			n.running = false
			n.mu.Unlock()
		}()
		n.checkSunNow()
		n.scheduleSun()
		n.scheduleNotes()
	}()
}

// ─────────────────────────── форма настроек ───────────────────────────

// notificationsSettings — блок переключателей для раздела «Настройки».
func notificationsSettings(a *App) fyne.CanvasObject {
	n := a.notifier

	mk := func(key, label, hint string) fyne.CanvasObject {
		chk := widget.NewCheck(label, func(on bool) {
			_ = n.setEnabled(key, on)
			// Сразу пересобираем расписание: включили — уведомления появятся,
			// выключили — уже запланированные больше не нужны.
			n.refresh()
		})
		chk.SetChecked(n.enabled(key))
		h := wrapLabel(hint)
		h.Importance = widget.LowImportance
		return container.NewVBox(chk, h)
	}

	return container.NewVBox(
		bold("Уведомления"),
		wrapLabel("Приходят, даже когда приложение закрыто."),
		widget.NewSeparator(),
		mk(notifyKeySun, "Восход и заход солнца",
			"Для города, выбранного в разделе «Часы»."),
		mk(notifyKeyWeather, "Прогноз погоды на сегодня",
			"Одна сводка в сутки после обновления прогноза."),
		mk(notifyKeyIM, "Новый выпуск ИМ УНиО",
			"Когда появляются новые навигационные предупреждения."),
		mk(notifyKeyNotes, "Напоминания из календаря",
			"По заметкам с галочкой «Уведомлять»."),
	)
}
