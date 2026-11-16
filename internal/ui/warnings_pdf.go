// Сохранение выпусков ИМ УНиО в PDF.
//
// ═══════════════════════════════════════════════════════════════════════════
//  КУДА СОХРАНЯЕТСЯ ФАЙЛ
// ═══════════════════════════════════════════════════════════════════════════
// Файл пишется в личную папку приложения (Storage) — там Android не требует
// разрешений. Затем показывается диалог с полным путём и кнопкой «Открыть»,
// которая передаёт файл системе через file:// ... точнее, через намерение
// просмотра. Если системного просмотрщика нет, пользователь всё равно видит,
// куда сохранён файл, и может забрать его через файловый менеджер.
//
// PDF собирается пакетом internal/pdf: свой минимальный генератор со встроенным
// шрифтом, поэтому кириллица в документе корректна и текст копируется.
//
// ═══════════════════════════════════════════════════════════════════════════
//  НЕ УДАЛЯТЬ. Используется кнопками в warnings.go.
// ═══════════════════════════════════════════════════════════════════════════
package ui

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"ru.navhelper/internal/navwarn"
	"ru.navhelper/internal/pdf"
	"ru.navhelper/internal/store"
)

// newCardBG — фон карточки с рамкой и скруглением.
func newCardBG() *canvas.Rectangle {
	bg := canvas.NewRectangle(color.NRGBA{0xff, 0xff, 0xff, 0xff})
	bg.CornerRadius = 10
	bg.StrokeColor = color.NRGBA{0xd3, 0xdc, 0xe6, 0xff}
	bg.StrokeWidth = 1
	return bg
}

// buildIssuePDF собирает PDF одного выпуска: заголовок, сведения и тексты
// всех извещений, сгруппированные по типу.
func buildIssuePDF(rows []store.WarningRow, issueNum, regionName string) ([]byte, error) {
	doc, err := pdf.NewA4(theme.DefaultTextFont().Content())
	if err != nil {
		return nil, err
	}

	doc.Title("Извещения мореплавателям — выпуск " + issueNum)
	doc.Subtitle(regionName)
	doc.Subtitle("Сформировано в NavHelper " + time.Now().Format("02.01.2006 15:04"))
	doc.HR()

	groups := navwarn.GroupByKind(rowsToWarnings(rows))
	if len(groups) == 0 {
		doc.Text("Извещений в выпуске нет.")
		return doc.Bytes(), nil
	}

	for _, g := range groups {
		doc.Title(g.TitleRU)
		for _, w := range g.Warnings {
			doc.Subtitle(fmt.Sprintf("%s %s/%d", w.Area, w.Number, w.Year))
			if tr := navwarn.TranslateTitle(w.Title); tr != w.Title {
				doc.Text(tr)
			}
			// Текст — как в первоисточнике, без перевода.
			doc.Mono(w.Body)
			if w.Source != "" {
				doc.Text("Источник: " + w.Source)
			}
			doc.HR()
		}
	}
	return doc.Bytes(), nil
}

// buildMessagePDF собирает PDF одного извещения.
func buildMessagePDF(w navwarn.Warning, issueNum, areaRU string) ([]byte, error) {
	doc, err := pdf.NewA4(theme.DefaultTextFont().Content())
	if err != nil {
		return nil, err
	}
	kind := navwarn.DetectKind(w).RU()
	doc.Title(fmt.Sprintf("%s, выпуск %s", kind, issueNum))
	if areaRU != "" {
		doc.Subtitle(areaRU)
	}
	doc.Subtitle("Сформировано в NavHelper " + time.Now().Format("02.01.2006 15:04"))
	doc.HR()
	if tr := navwarn.TranslateTitle(w.Title); tr != w.Title {
		doc.Text(tr)
		doc.HR()
	}
	doc.Mono(w.Body)
	if w.Source != "" {
		doc.HR()
		doc.Text("Источник: " + w.Source)
	}
	return doc.Bytes(), nil
}

// saveIssuePDF сохраняет весь выпуск и предлагает открыть файл.
func (t *warningsTab) saveIssuePDF(it *issue) {
	regionName := allRegionsKey
	if t.regionSel.Selected != "" {
		regionName = t.regionSel.Selected
	}
	data, err := buildIssuePDF(it.Messages, it.Number, regionName)
	if err != nil {
		dialog.ShowError(err, t.a.Win)
		return
	}
	name := "IM_" + strings.NewReplacer("/", "-").Replace(it.Number) + ".pdf"
	t.writePDF(name, data)
}

// saveMessagePDF сохраняет одно извещение.
func (t *warningsTab) saveMessagePDF(w navwarn.Warning, issueNum string) {
	area := navwarn.AreaRU(w.Area + " " + w.Title)
	data, err := buildMessagePDF(w, issueNum, area)
	if err != nil {
		dialog.ShowError(err, t.a.Win)
		return
	}
	name := "IM_" + strings.NewReplacer("/", "-", " ", "_").Replace(issueNum) + "_" +
		strings.NewReplacer("/", "-", " ", "_").Replace(w.Number) + ".pdf"
	t.writePDF(name, data)
}

// writePDF записывает файл в хранилище приложения и показывает результат.
func (t *warningsTab) writePDF(name string, data []byte) {
	w, err := t.a.FA.Storage().Save(name)
	if err != nil {
		// Если файл уже есть — перезаписываем.
		_ = t.a.FA.Storage().Remove(name)
		w, err = t.a.FA.Storage().Save(name)
		if err != nil {
			dialog.ShowError(fmt.Errorf("не удалось сохранить PDF: %w", err), t.a.Win)
			return
		}
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		dialog.ShowError(fmt.Errorf("не удалось записать PDF: %w", err), t.a.Win)
		return
	}
	w.Close()

	// Показываем, где лежит файл, и даём кнопку «Открыть».
	uri := storage.NewFileURI(name)
	if root := t.a.FA.Storage().RootURI(); root != nil {
		if child, err := storage.Child(root, name); err == nil {
			uri = child
		}
	}

	size := fmt.Sprintf("%.1f КБ", float64(len(data))/1024)
	info := wrapLabel("Файл сохранён: " + name + " (" + size + ")\n\n" +
		"Путь: " + uri.Path() + "\n\n" +
		"Если файл нужно отправить или открыть в другой программе, " +
		"воспользуйтесь файловым менеджером: папка приложения NavHelper.")

	open := widget.NewButtonWithIcon("Открыть", theme.DocumentIcon(), func() {
		// Fyne ждёт *url.URL; fyne.URI — другой тип. Собираем file:// вручную.
		u, perr := url.Parse(uri.String())
		if perr != nil {
			dialog.ShowInformation("Не удалось открыть",
				"Файл лежит здесь:\n"+uri.Path(), t.a.Win)
			return
		}
		if err := t.a.FA.OpenURL(u); err != nil {
			dialog.ShowInformation("Не удалось открыть",
				"Просмотрщик PDF не найден. Файл лежит здесь:\n"+uri.Path(), t.a.Win)
		}
	})
	open.Importance = widget.HighImportance

	content := container.NewVBox(info, open)
	d := dialog.NewCustom("Готово", "Закрыть", content, t.a.Win)
	d.Resize(fyne.NewSize(380, 300))
	d.Show()
}
