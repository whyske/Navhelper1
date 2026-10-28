// Package charts — офлайн-каталог морских карт УНиО: номер → название,
// регион, масштаб, год издания. Данные собирает отдельная программа
// cmd/chartsync и кладёт в catalog.tsv.gz, который встраивается в
// приложение (go:embed) и при первом запуске импортируется в общую SQLite.
package charts

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

//go:embed catalog.tsv.gz
var catalogGz []byte

// Chart — карточка морской карты.
type Chart struct {
	Number   string // номер карты УНиО, например «38174»
	IntNo    string // номер INT (если есть)
	TitleRU  string
	TitleEN  string
	RegionEN string
	RegionRU string
	Scale    int // знаменатель масштаба: 5000 = 1:5 000
	Year     int // год издания
	LatMin   float64
	LatMax   float64
	LonMin   float64
	LonMax   float64
	Source   string
}

// Schema — таблица каталога (в общей базе приложения и в charts.db).
const Schema = `
CREATE TABLE IF NOT EXISTS charts (
	number    TEXT PRIMARY KEY,
	int_no    TEXT NOT NULL DEFAULT '',
	title_ru  TEXT NOT NULL DEFAULT '',
	title_en  TEXT NOT NULL DEFAULT '',
	region_en TEXT NOT NULL DEFAULT '',
	region_ru TEXT NOT NULL DEFAULT '',
	scale     INTEGER NOT NULL DEFAULT 0,
	year      INTEGER NOT NULL DEFAULT 0,
	lat_min   REAL NOT NULL DEFAULT 0,
	lat_max   REAL NOT NULL DEFAULT 0,
	lon_min   REAL NOT NULL DEFAULT 0,
	lon_max   REAL NOT NULL DEFAULT 0,
	source    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_charts_title ON charts(title_ru);
CREATE TABLE IF NOT EXISTS charts_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
`

// EncodeTSV сериализует карту в строку TSV.
func EncodeTSV(c Chart) string {
	clean := func(s string) string {
		return strings.NewReplacer("\t", " ", "\n", " ", "\r", " ").Replace(s)
	}
	return strings.Join([]string{
		clean(c.Number), clean(c.IntNo), clean(c.TitleRU), clean(c.TitleEN),
		clean(c.RegionEN), clean(c.RegionRU),
		strconv.Itoa(c.Scale), strconv.Itoa(c.Year),
		f4(c.LatMin), f4(c.LatMax), f4(c.LonMin), f4(c.LonMax), clean(c.Source),
	}, "\t")
}

func f4(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

// DecodeTSV разбирает строку TSV.
func DecodeTSV(line string) (Chart, bool) {
	p := strings.Split(line, "\t")
	if len(p) < 13 {
		return Chart{}, false
	}
	c := Chart{Number: p[0], IntNo: p[1], TitleRU: p[2], TitleEN: p[3],
		RegionEN: p[4], RegionRU: p[5], Source: p[12]}
	c.Scale, _ = strconv.Atoi(p[6])
	c.Year, _ = strconv.Atoi(p[7])
	c.LatMin, _ = strconv.ParseFloat(p[8], 64)
	c.LatMax, _ = strconv.ParseFloat(p[9], 64)
	c.LonMin, _ = strconv.ParseFloat(p[10], 64)
	c.LonMax, _ = strconv.ParseFloat(p[11], 64)
	return c, true
}

// Embedded возвращает каталог, встроенный в приложение.
func Embedded() ([]Chart, string, error) {
	return Decode(catalogGz)
}

// Decode распаковывает каталог gzip+TSV и возвращает список и строку-заголовок.
func Decode(gz []byte) ([]Chart, string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, "", err
	}
	defer zr.Close()
	sc := bufio.NewScanner(zr)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	var out []Chart
	header := ""
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			header = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			continue
		}
		if c, ok := DecodeTSV(line); ok {
			out = append(out, c)
		}
	}
	return out, header, sc.Err()
}

// InsertAll записывает каталог в таблицу charts (полная замена).
func InsertAll(db *sql.DB, list []Chart) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM charts`); err != nil {
		return err
	}
	st, err := tx.Prepare(`INSERT OR REPLACE INTO charts
		(number,int_no,title_ru,title_en,region_en,region_ru,scale,year,lat_min,lat_max,lon_min,lon_max,source)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, c := range list {
		if _, err := st.Exec(c.Number, c.IntNo, c.TitleRU, c.TitleEN, c.RegionEN, c.RegionRU,
			c.Scale, c.Year, c.LatMin, c.LatMax, c.LonMin, c.LonMax, c.Source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// EnsureImported создаёт таблицы и импортирует встроенный каталог, если
// он ещё не импортирован или изменился (сравнение по SHA-1).
func EnsureImported(db *sql.DB) (int, error) {
	if _, err := db.Exec(Schema); err != nil {
		return 0, err
	}
	sum := sha1.Sum(catalogGz)
	hash := hex.EncodeToString(sum[:])
	var cur string
	_ = db.QueryRow(`SELECT value FROM charts_meta WHERE key='hash'`).Scan(&cur)
	if cur == hash {
		return Count(db), nil
	}
	list, header, err := Embedded()
	if err != nil {
		return 0, fmt.Errorf("встроенный каталог повреждён: %w", err)
	}
	if err := InsertAll(db, list); err != nil {
		return 0, err
	}
	_, _ = db.Exec(`INSERT OR REPLACE INTO charts_meta(key,value) VALUES('hash',?),('header',?)`, hash, header)
	return len(list), nil
}

// Count — число карт в базе.
func Count(db *sql.DB) int {
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM charts`).Scan(&n)
	return n
}

// Header — строка с датой сборки каталога.
func Header(db *sql.DB) string {
	var h string
	_ = db.QueryRow(`SELECT value FROM charts_meta WHERE key='header'`).Scan(&h)
	return h
}

const cols = `number,int_no,title_ru,title_en,region_en,region_ru,scale,year,lat_min,lat_max,lon_min,lon_max,source`

func scanRows(rows *sql.Rows) ([]Chart, error) {
	defer rows.Close()
	var out []Chart
	for rows.Next() {
		var c Chart
		if err := rows.Scan(&c.Number, &c.IntNo, &c.TitleRU, &c.TitleEN, &c.RegionEN, &c.RegionRU,
			&c.Scale, &c.Year, &c.LatMin, &c.LatMax, &c.LonMin, &c.LonMax, &c.Source); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Find ищет карту по точному номеру (УНиО или INT).
func Find(db *sql.DB, number string) (Chart, bool) {
	number = normalizeNumber(number)
	if number == "" {
		return Chart{}, false
	}
	rows, err := db.Query(`SELECT `+cols+` FROM charts WHERE number = ? OR (int_no != '' AND int_no = ?) LIMIT 1`, number, number)
	if err != nil {
		return Chart{}, false
	}
	list, err := scanRows(rows)
	if err != nil || len(list) == 0 {
		return Chart{}, false
	}
	return list[0], true
}

// Search ищет по началу номера или по подстроке в названии/регионе.
func Search(db *sql.DB, q string, limit int) ([]Chart, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if n := normalizeNumber(q); n != "" {
		rows, err := db.Query(`SELECT `+cols+` FROM charts WHERE number LIKE ? ORDER BY number LIMIT ?`, n+"%", limit)
		if err != nil {
			return nil, err
		}
		return scanRows(rows)
	}
	// SQLite LIKE не понимает регистр кириллицы, поэтому фильтруем в Go.
	rows, err := db.Query(`SELECT ` + cols + ` FROM charts ORDER BY number`)
	if err != nil {
		return nil, err
	}
	all, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	lq := strings.ToLower(q)
	var out []Chart
	for _, c := range all {
		hay := strings.ToLower(c.TitleRU + " " + c.TitleEN + " " + c.RegionRU + " " + c.RegionEN)
		if strings.Contains(hay, lq) {
			out = append(out, c)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

// normalizeNumber оставляет только цифры: «№ 38 174» → «38174».
func normalizeNumber(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		} else if r != ' ' && r != '№' && r != '#' && r != '-' {
			return ""
		}
	}
	return sb.String()
}

// FormatScale — «1:5 000», «1:500 000».
func FormatScale(scale int) string {
	if scale <= 0 {
		return "—"
	}
	s := strconv.Itoa(scale)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return "1:" + strings.Join(parts, " ")
}

// Kind — тип карты по масштабу (классификация ГУНиО).
func Kind(scale int) string {
	switch {
	case scale <= 0:
		return ""
	case scale <= 25000:
		return "план"
	case scale < 100000:
		return "частная карта"
	case scale <= 1000000:
		return "путевая карта"
	default:
		return "генеральная карта"
	}
}

// Coverage — район покрытия в градусах и минутах.
func Coverage(c Chart) string {
	if c.LatMin == 0 && c.LatMax == 0 && c.LonMin == 0 && c.LonMax == 0 {
		return ""
	}
	return fmt.Sprintf("%s – %s, %s – %s",
		latStr(c.LatMin), latStr(c.LatMax), lonStr(c.LonMin), lonStr(c.LonMax))
}

func latStr(v float64) string { return dm(v, "N", "S", 2) }
func lonStr(v float64) string { return dm(v, "E", "W", 3) }

func dm(v float64, pos, neg string, w int) string {
	h := pos
	if v < 0 {
		h = neg
		v = -v
	}
	deg := int(v)
	min := (v - float64(deg)) * 60
	if min >= 59.95 {
		deg++
		min = 0
	}
	return fmt.Sprintf("%0*d°%04.1f′%s", w, deg, min, h)
}

// InBox возвращает листы, пересекающие прямоугольник (для превью «как на
// navysoft»: соседние листы вокруг выбранного). Слишком мелкие на экране листы
// (меньше minFrac от ширины окна) и обзорные карты, накрывающие окно целиком,
// отбрасываются, чтобы не захламлять превью.
func InBox(db *sql.DB, lonMin, latMin, lonMax, latMax, minFrac float64, limit int) ([]Chart, error) {
	rows, err := db.Query(`SELECT `+cols+` FROM charts
		WHERE lat_max > ? AND lat_min < ? AND lon_max > ? AND lon_min < ?
		  AND NOT (lat_min = 0 AND lat_max = 0)
		ORDER BY scale DESC`, latMin, latMax, lonMin, lonMax)
	if err != nil {
		return nil, err
	}
	all, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	w, h := lonMax-lonMin, latMax-latMin
	var out []Chart
	for _, c := range all {
		cw, ch := c.LonMax-c.LonMin, c.LatMax-c.LatMin
		if cw < w*minFrac && ch < h*minFrac {
			continue
		}
		if c.LonMin <= lonMin && c.LonMax >= lonMax && c.LatMin <= latMin && c.LatMax >= latMax {
			continue
		}
		out = append(out, c)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
