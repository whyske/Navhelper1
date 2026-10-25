// Команда chartsync — сборщик каталога морских карт УНиО.
//
// Источник: электронный каталог бумажных карт ООО «НавиСофт» (официальный
// распространитель УНиО МО РФ), https://navysoft.ru/chartindex. Каталог
// отдаётся одним JSON-файлом; строки в нём закодированы сдвигом кода символа
// на 0x90, названия используют маркеры диакритики «#M0..#M9», а покрытие
// карты — полилинии Google Encoded Polyline.
//
// Результат — сжатый TSV internal/charts/catalog.tsv.gz, который встраивается
// в приложение через go:embed. Дополнительно (флаг -db) можно выгрузить
// самостоятельную базу charts.db.
//
// Запуск:
//
//	go run ./cmd/chartsync                       # скачать и собрать
//	go run ./cmd/chartsync -in navysoft.json     # из локального файла
//	go run ./cmd/chartsync -db charts.db         # плюс отдельная SQLite
//	go run ./cmd/chartsync -overrides fix.csv    # ручные поправки
package main

import (
	"bufio"
	"compress/gzip"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/text/unicode/norm"

	"ru.navhelper/internal/charts"
)

const (
	catalogURL = "https://navysoft.ru/data/og4qcofkc32.json"
	refererURL = "https://navysoft.ru/chartindex"
)

func main() {
	in := flag.String("in", "", "локальный JSON каталога (по умолчанию — скачать с navysoft.ru)")
	out := flag.String("out", "internal/charts/catalog.tsv.gz", "куда записать встраиваемый каталог")
	dbPath := flag.String("db", "", "дополнительно записать отдельную базу SQLite (charts.db)")
	overrides := flag.String("overrides", "", "CSV с поправками: number;year;scale;title_ru")
	flag.Parse()

	raw, err := loadRaw(*in)
	if err != nil {
		log.Fatalf("загрузка каталога: %v", err)
	}
	list, err := decodeCatalog(raw)
	if err != nil {
		log.Fatalf("разбор каталога: %v", err)
	}
	log.Printf("НавиСофт: %d карт", len(list))

	if *overrides != "" {
		n, err := applyOverrides(list, *overrides)
		if err != nil {
			log.Fatalf("поправки: %v", err)
		}
		log.Printf("применено поправок: %d", n)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].Number < list[j].Number })
	if err := writeTSV(*out, list); err != nil {
		log.Fatalf("запись %s: %v", *out, err)
	}
	log.Printf("записан %s", *out)

	if *dbPath != "" {
		if err := writeDB(*dbPath, list); err != nil {
			log.Fatalf("запись %s: %v", *dbPath, err)
		}
		log.Printf("записана база %s", *dbPath)
	}
	printStats(list)
}

// loadRaw читает каталог из файла или скачивает его.
func loadRaw(path string) ([]byte, error) {
	if path != "" {
		return os.ReadFile(path)
	}
	req, _ := http.NewRequest(http.MethodGet, catalogURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 NavHelper-chartsync/1.0")
	req.Header.Set("Referer", refererURL)
	cl := &http.Client{Timeout: 90 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// decodeCatalog разбирает JSON НавиСофт. Формат строки:
// [номер, номер_INT, порядковый, название_en, название_ru, год, масштаб, геометрия]
func decodeCatalog(raw []byte) ([]charts.Chart, error) {
	var rows [][]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]charts.Chart, 0, len(rows))
	for _, r := range rows {
		if len(r) < 7 {
			continue
		}
		str := func(i int) string {
			var s string
			_ = json.Unmarshal(r[i], &s)
			return strings.TrimSpace(diacritics(shift(s)))
		}
		c := charts.Chart{
			Number:  str(0),
			IntNo:   str(1),
			TitleEN: str(3),
			TitleRU: str(4),
			Source:  "НавиСофт/УНиО",
		}
		c.Year, _ = strconv.Atoi(str(5))
		c.Scale, _ = strconv.Atoi(str(6))
		c.RegionEN = regionOf(c.TitleEN)
		c.RegionRU = charts.TranslateRegion(c.RegionEN)
		if len(r) > 7 {
			var geom interface{}
			if json.Unmarshal(r[7], &geom) == nil {
				var b bbox
				b.reset()
				collectPolylines(geom, &b)
				if b.ok() {
					c.LatMin, c.LatMax, c.LonMin, c.LonMax = b.latMin, b.latMax, b.lonMin, b.lonMax
				}
			}
		}
		if c.Number == "" {
			continue
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("каталог пуст — вероятно, изменился формат")
	}
	return out, nil
}

// shift снимает обфускацию: символы с кодом ≥ 0x90 сдвинуты на 0x90.
func shift(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= 0x90 {
			r -= 0x90
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// combining — маркеры «#Mx» → комбинируемые знаки Unicode.
var combining = map[byte]rune{
	'0': '\u0308', // умлаут: ü
	'1': '\u0301', // акут: é
	'2': '\u0300', // гравис: è
	'3': '\u0302', // циркумфлекс: î
	'4': '\u030C', // гачек: š
	'5': '\u0304', // макрон: ī
	'6': '\u0303', // тильда: ñ, ã
	'7': '\u0327', // седиль: ç
	'8': '\u030A', // кружок: å
}

// diacritics заменяет «o#M9» на «ø», «a#M8» на «å» и т.д.
func diacritics(s string) string {
	if !strings.Contains(s, "#M") {
		return s
	}
	var sb strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		if rs[i] == '#' && i+2 < len(rs)+1 && i+1 < len(rs) && rs[i+1] == 'M' {
			if i+2 < len(rs) {
				code := rs[i+2]
				i += 2
				if code == '9' { // перечёркнутая буква: ø / Ø
					str := []rune(sb.String())
					if n := len(str); n > 0 {
						switch str[n-1] {
						case 'o':
							str[n-1] = 'ø'
						case 'O':
							str[n-1] = 'Ø'
						}
						sb.Reset()
						sb.WriteString(string(str))
					}
					continue
				}
				if c, ok := combining[byte(code)]; ok {
					sb.WriteRune(c)
				}
				continue
			}
		}
		sb.WriteRune(rs[i])
	}
	return norm.NFC.String(sb.String())
}

// regionOf — первая часть английского названия: «Black Sea. Coast of Russia…» → «Black Sea».
func regionOf(en string) string {
	if i := strings.Index(en, ". "); i > 0 {
		return en[:i]
	}
	return strings.TrimSuffix(en, ".")
}

// ─────────────────────────── геометрия ───────────────────────────

type bbox struct{ latMin, latMax, lonMin, lonMax float64 }

func (b *bbox) reset() { b.latMin, b.lonMin, b.latMax, b.lonMax = 999, 999, -999, -999 }
func (b *bbox) ok() bool { return b.latMax >= b.latMin }
func (b *bbox) add(lat, lon float64) {
	b.latMin, b.latMax = math.Min(b.latMin, lat), math.Max(b.latMax, lat)
	b.lonMin, b.lonMax = math.Min(b.lonMin, lon), math.Max(b.lonMax, lon)
}

// collectPolylines рекурсивно обходит геометрию и декодирует все строки-полилинии.
func collectPolylines(v interface{}, b *bbox) {
	switch t := v.(type) {
	case string:
		for _, p := range decodePolyline(t) {
			b.add(p[0], p[1])
		}
	case []interface{}:
		for _, x := range t {
			collectPolylines(x, b)
		}
	}
}

// decodePolyline — алгоритм Google Encoded Polyline (точность 1e-5).
func decodePolyline(s string) [][2]float64 {
	var pts [][2]float64
	idx, lat, lng := 0, 0, 0
	for idx < len(s) {
		var vals [2]int
		for k := 0; k < 2; k++ {
			res, sh := 0, 0
			for {
				if idx >= len(s) {
					return pts
				}
				b := int(s[idx]) - 63
				idx++
				res |= (b & 0x1f) << sh
				sh += 5
				if b < 0x20 {
					break
				}
			}
			if res&1 != 0 {
				vals[k] = ^(res >> 1)
			} else {
				vals[k] = res >> 1
			}
		}
		lat += vals[0]
		lng += vals[1]
		pts = append(pts, [2]float64{float64(lat) / 1e5, float64(lng) / 1e5})
	}
	return pts
}

// ─────────────────────────── поправки ───────────────────────────

// applyOverrides — ручные поправки из CSV «number;year;scale;title_ru».
// Пустое поле не меняет значение. Нужны, когда карта переиздана позже
// снимка каталога: так комбинируются несколько источников.
func applyOverrides(list []charts.Chart, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(bufio.NewReader(f))
	r.Comma = ';'
	r.FieldsPerRecord = -1
	r.Comment = '#'
	idx := map[string]int{}
	for i, c := range list {
		idx[c.Number] = i
	}
	n := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return n, err
		}
		if len(rec) == 0 {
			continue
		}
		i, ok := idx[strings.TrimSpace(rec[0])]
		if !ok {
			continue
		}
		if len(rec) > 1 && strings.TrimSpace(rec[1]) != "" {
			list[i].Year, _ = strconv.Atoi(strings.TrimSpace(rec[1]))
		}
		if len(rec) > 2 && strings.TrimSpace(rec[2]) != "" {
			list[i].Scale, _ = strconv.Atoi(strings.TrimSpace(rec[2]))
		}
		if len(rec) > 3 && strings.TrimSpace(rec[3]) != "" {
			list[i].TitleRU = strings.TrimSpace(rec[3])
		}
		list[i].Source += " + поправка"
		n++
	}
	return n, nil
}

// ─────────────────────────── вывод ───────────────────────────

func writeTSV(path string, list []charts.Chart) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	w := bufio.NewWriter(gz)
	fmt.Fprintf(w, "# navhelper charts catalog; generated %s; source %s\n",
		time.Now().UTC().Format("2006-01-02"), catalogURL)
	for _, c := range list {
		fmt.Fprintln(w, charts.EncodeTSV(c))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return gz.Close()
}

func writeDB(path string, list []charts.Chart) error {
	_ = os.Remove(path)
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(charts.Schema); err != nil {
		return err
	}
	return charts.InsertAll(db, list)
}

func printStats(list []charts.Chart) {
	years := map[int]int{}
	maxY := 0
	for _, c := range list {
		years[c.Year]++
		if c.Year > maxY {
			maxY = c.Year
		}
	}
	log.Printf("последний год издания в каталоге: %d", maxY)
	for _, n := range []string{"38174", "11114", "21001"} {
		for _, c := range list {
			if c.Number == n {
				log.Printf("  %s — %s, 1:%d, %d г. (%s)", c.Number, c.TitleRU, c.Scale, c.Year, c.RegionRU)
			}
		}
	}
}
