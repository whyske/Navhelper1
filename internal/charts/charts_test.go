package charts

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestEmbeddedCatalog(t *testing.T) {
	list, header, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 6000 {
		t.Fatalf("в каталоге слишком мало карт: %d", len(list))
	}
	if header == "" {
		t.Error("нет заголовка каталога")
	}
}

func TestFind38174(t *testing.T) {
	db := openDB(t)
	n, err := EnsureImported(db)
	if err != nil {
		t.Fatal(err)
	}
	if n < 6000 {
		t.Fatalf("импортировано %d", n)
	}
	// Повторный импорт не нужен — хэш совпадает.
	if n2, _ := EnsureImported(db); n2 != n {
		t.Errorf("повторный импорт: %d vs %d", n2, n)
	}
	c, ok := Find(db, "38174")
	if !ok {
		t.Fatal("карта 38174 не найдена")
	}
	if c.TitleRU != "Причалы порта Новороссийск" || c.Scale != 5000 || c.Year != 2016 || c.RegionRU != "Чёрное море" {
		t.Errorf("38174: %+v", c)
	}
	if c.LatMin < 44.6 || c.LatMax > 44.8 || c.LonMin < 37.7 || c.LonMax > 37.9 {
		t.Errorf("координаты 38174: %+v", c)
	}
	if _, ok := Find(db, "№ 38 174"); !ok {
		t.Error("номер с «№» и пробелом")
	}
	if _, ok := Find(db, "00000"); ok {
		t.Error("несуществующая карта найдена")
	}
}

func TestSearch(t *testing.T) {
	db := openDB(t)
	EnsureImported(db)
	res, _ := Search(db, "381", 500)
	if len(res) == 0 {
		t.Fatal("поиск по префиксу номера")
	}
	for _, c := range res {
		if c.Number[:3] != "381" {
			t.Errorf("лишний результат %s", c.Number)
		}
	}
	res, _ = Search(db, "новороссийск", 50)
	found := false
	for _, c := range res {
		if c.Number == "38174" {
			found = true
		}
	}
	if !found {
		t.Error("поиск по названию без учёта регистра")
	}
}

func TestFormat(t *testing.T) {
	cases := map[int]string{5000: "1:5 000", 500000: "1:500 000", 5000000: "1:5 000 000", 750: "1:750", 0: "—"}
	for in, want := range cases {
		if got := FormatScale(in); got != want {
			t.Errorf("FormatScale(%d) = %s", in, got)
		}
	}
	if Kind(5000) != "план" || Kind(50000) != "частная карта" || Kind(500000) != "путевая карта" || Kind(5000000) != "генеральная карта" {
		t.Error("тип карты по масштабу")
	}
	c := Chart{LatMin: 44.7011, LatMax: 44.7353, LonMin: 37.7744, LonMax: 37.8564}
	if got := Coverage(c); got != "44°42.1′N – 44°44.1′N, 037°46.5′E – 037°51.4′E" {
		t.Errorf("Coverage = %s", got)
	}
}

func TestTSVRoundTrip(t *testing.T) {
	c := Chart{Number: "1", TitleRU: "А\tБ", Scale: 10, Year: 2020, LatMin: -1.5, Source: "x"}
	got, ok := DecodeTSV(EncodeTSV(c))
	if !ok || got.TitleRU != "А Б" || got.Scale != 10 || got.LatMin != -1.5 {
		t.Errorf("round trip: %+v", got)
	}
}
