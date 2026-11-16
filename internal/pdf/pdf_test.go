package pdf

import (
	"bytes"
	"os"
	"testing"

	"fyne.io/fyne/v2/theme"
)

// TestPDFBuild проверяет, что документ с кириллицей собирается и имеет
// корректную структуру: заголовок %PDF, xref, EOF.
func TestPDFBuild(t *testing.T) {
	// Берём шрифт прямо из Fyne: он уже вкомпилирован в бинарник,
	// поэтому путь к файлу в тесте не нужен.
	fontData := theme.DefaultTextFont().Content()
	if len(fontData) == 0 {
		t.Skip("шрифт недоступен")
	}
	d, err := NewA4(fontData)
	if err != nil {
		t.Fatal(err)
	}
	d.Title("НАВАРЕА III, выпуск 244/2026")
	d.Subtitle("РАЙОН III — Средиземное и Чёрное моря (Испания)")
	d.HR()
	d.Text("Учебные стрельбы в районе 42°00′ N 004°30′ E. Проход запрещён с 15 по 20 ноября.")
	d.Mono("NAVAREA III 244/26\n\n1. GUNNERY AREA.\nCHARTS AFFECTED: 2231, 2232")
	d.Text("Настоящее извещение действует до отмены.")

	b := d.Bytes()
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) {
		t.Fatal("нет заголовка PDF")
	}
	if !bytes.Contains(b, []byte("%%EOF")) {
		t.Fatal("нет концевого маркера")
	}
	if !bytes.Contains(b, []byte("/Identity-H")) {
		t.Fatal("нет кодировки Identity-H")
	}
	// Глифы кириллицы должны быть в содержимом как hex-строки.
	if !bytes.Contains(b, []byte("Tj")) {
		t.Fatal("нет операторов вывода текста")
	}
	t.Logf("PDF собран: %d байт", len(b))
	if os.Getenv("PDF_OUT") != "" {
		_ = os.WriteFile(os.Getenv("PDF_OUT"), b, 0o644)
	}
}

// TestFontMetrics проверяет, что метрики шрифта разобраны и кириллица
// имеет ненулевые глифы — иначе в PDF получатся пустые квадраты.
func TestFontMetrics(t *testing.T) {
	f, err := parseTTF(theme.DefaultTextFont().Content())
	if err != nil {
		t.Fatal(err)
	}
	if f.unitsPerEm == 0 {
		t.Fatal("unitsPerEm = 0")
	}
	for _, r := range "АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯабвгдеёжзийклмнопрстуфхцчшщъыьэюя" {
		if f.gid(r) == 0 {
			t.Errorf("нет глифа для %q", r)
		}
	}
	if f.width("Проверка", 10) <= 0 {
		t.Error("ширина строки = 0")
	}
}
