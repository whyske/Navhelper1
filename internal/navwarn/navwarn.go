// Пакет navwarn: загрузка навигационных предупреждений (НАВАРЕА, ПРИП, НАВИП)
// и извещений мореплавателям (ИМ УНиО).
//
// Источники (все открытые, без ключей):
//   - SeaLagom NAVAREA API — действующие предупреждения по всем 20 районам
//     НАВАРЕА (Атлантика, Средиземное и Чёрное море, Балтика, Арктика и т. д.);
//   - SeaLagom RSS — прибрежные предупреждения (COASTAL) по странам;
//   - ИМ ГУНиО — извещения мореплавателям (через публичные ленты публикаций).
//
// Приложение не устанавливает никаких сертификатов и не отключает проверку TLS.
package navwarn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Region — крупный район, по которому группируются предупреждения в интерфейсе.
type Region struct {
	Key   string   // "black", "baltic", ...
	Name  string   // «Чёрное и Азовское море»
	Areas []string // какие НАВАРЕА/прибрежные районы сюда входят
}

// Regions — районы, которые показывает раздел «ИМ».
// Раньше приложение искало только Арктику; теперь охвачены основные
// бассейны, где реально ходят российские суда.
var Regions = []Region{
	{Key: "black", Name: "Чёрное и Азовское море", Areas: []string{"NAVAREA III", "Black Sea", "Azov"}},
	{Key: "baltic", Name: "Балтика и Финский залив", Areas: []string{"NAVAREA I", "Baltic", "Kronstadt", "Finland", "Denmark"}},
	{Key: "arctic", Name: "Арктика и Северный морской путь", Areas: []string{"NAVAREA XX", "NAVAREA XXI", "Barents", "Kara", "Laptev"}},
	{Key: "north", Name: "Северная Атлантика и Северное море", Areas: []string{"NAVAREA II", "North Sea", "Norwegian"}},
	{Key: "med", Name: "Средиземное море", Areas: []string{"NAVAREA III", "Mediterranean"}},
	{Key: "pacific", Name: "Дальний Восток и Тихий океан", Areas: []string{"NAVAREA XII", "NAVAREA XIII", "NAVAREA XIV", "Japan", "Pacific"}},
	{Key: "caspian", Name: "Каспий", Areas: []string{"Caspian"}},
}

// Warning — одно навигационное предупреждение.
type Warning struct {
	Area     string    // «НАВАРЕА III», «ПРИП МУРМАНСК»…
	Number   string    // номер предупреждения
	Year     int       // год
	Title    string    // первая строка — район/суть
	Body     string    // полный текст
	Region   string    // ключ региона из Regions
	Charts   []string  // упомянутые карты
	CancelAt time.Time // срок автоматической отмены
	URL      string    // ссылка на сообщение
	Source   string    // имя источника
	Fetched  time.Time
}

// ID — уникальный ключ предупреждения.
func (w Warning) ID() string {
	return fmt.Sprintf("%s|%s|%d", w.Area, w.Number, w.Year)
}

// Expired — срок действия истёк.
func (w Warning) Expired(now time.Time) bool {
	return !w.CancelAt.IsZero() && now.After(w.CancelAt)
}

// Client — загрузчик предупреждений.
type Client struct {
	HTTP *http.Client
}

// NewClient создаёт клиент с таймаутом. Собственных корневых сертификатов
// приложение не добавляет и в систему ничего не устанавливает.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 25 * time.Second}}
}

const (
	navareaURL = "https://www.sealagom.com/api/v1/navarea/"
	rssURL     = "https://www.sealagom.com/feeds/warnings.json"
)

// navareaItem — ответ NAVAREA API.
// navareaItem — ответ NAVAREA API. Поля, которые API может отдать как число,
// строку или массив, объявлены как json.RawMessage: формат источника менялся,
// и жёсткая типизация ломала разбор целиком (пустой список предупреждений).
type navareaItem struct {
	ID      int             `json:"id"`
	Title   string          `json:"title"`
	Content string          `json:"content_full"`
	TimeUpd string          `json:"time_upd"`
	Country string          `json:"country"`
	Active  json.RawMessage `json:"active_messages"`
	Arch   json.RawMessage `json:"archived_messages"`
}

type navareaResp struct {
	Count   int           `json:"count"`
	Results []navareaItem `json:"results"`
}

// FetchAll загружает предупреждения из всех источников параллельно.
// Возвращает объединённый список и по одному сообщению об ошибке на источник.
func (c *Client) FetchAll(ctx context.Context) ([]Warning, []string) {
	type res struct {
		list []Warning
		err  string
	}
	ch := make(chan res, 2)
	go func() {
		l, err := c.fetchNavarea(ctx)
		ch <- res{l, err}
	}()
	go func() {
		l, err := c.fetchCoastal(ctx)
		ch <- res{l, err}
	}()

	var out []Warning
	var errs []string
	for i := 0; i < 2; i++ {
		r := <-ch
		out = append(out, r.list...)
		if r.err != "" {
			errs = append(errs, r.err)
		}
	}
	// Убираем дубликаты по ID.
	seen := map[string]bool{}
	uniq := out[:0]
	for _, w := range out {
		if seen[w.ID()] {
			continue
		}
		seen[w.ID()] = true
		uniq = append(uniq, w)
	}
	sort.Slice(uniq, func(i, j int) bool {
		if uniq[i].Area != uniq[j].Area {
			return uniq[i].Area < uniq[j].Area
		}
		return uniq[i].Number > uniq[j].Number
	})
	return uniq, errs
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "navhelper/4.0 (+https://github.com/whyske/Navhelper1)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// fetchNavarea читает NAVAREA API: по каждому району отдаётся действующее
// предупреждение целиком, а внутри текста их может быть несколько.
func (c *Client) fetchNavarea(ctx context.Context) ([]Warning, string) {
	body, err := c.get(ctx, navareaURL)
	if err != nil {
		return nil, "НАВАРЕА: " + err.Error()
	}
	var r navareaResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, "НАВАРЕА: ответ не разобран"
	}
	var out []Warning
	now := time.Now()
	for _, it := range r.Results {
		area := strings.TrimSpace(it.Title)
		if area == "" {
			continue
		}
		region := regionFor(area + " " + it.Content)
		for _, w := range splitNavarea(area, it.Content) {
			w.Region = region
			w.Source = "НАВАРЕА (SeaLagom)"
			w.Fetched = now
			w.URL = fmt.Sprintf("%s%d/", navareaURL, it.ID)
			out = append(out, w)
		}
	}
	return out, ""
}

// rssItem — элемент ленты прибрежных предупреждений.
type rssItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	ContentText string   `json:"content_text"`
	Summary     string   `json:"summary"`
	DatePub     string   `json:"date_published"`
	URL         string   `json:"url"`
	Tags        []string `json:"tags"`
}

type rssFeed struct {
	Items []rssItem `json:"items"`
}

// fetchCoastal читает ленту прибрежных предупреждений (COASTAL): Балтика,
// Северное море, Черное море, Мурманск и другие районы.
func (c *Client) fetchCoastal(ctx context.Context) ([]Warning, string) {
	body, err := c.get(ctx, rssURL)
	if err != nil {
		return nil, "Прибрежные предупреждения: " + err.Error()
	}
	var f rssFeed
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, "Прибрежные предупреждения: ответ не разобран"
	}
	now := time.Now()
	var out []Warning
	for _, it := range f.Items {
		title := strings.TrimSpace(it.Title)
		if title == "" {
			continue
		}
		area, num, year := parseCoastalTitle(title)
		if area == "" {
			continue
		}
		text := it.ContentText
		if text == "" {
			text = it.Summary
		}
		text = stripTags(text)
		pub := parseRFC822(it.DatePub)
		out = append(out, Warning{
			Area:     area,
			Number:   num,
			Year:     year,
			Title:    title,
			Body:     strings.TrimSpace(title + "\n\n" + text),
			Region:   regionFor(title + " " + text),
			URL:      it.URL,
			Source:   "Прибрежные предупреждения (SeaLagom)",
			Fetched:  now,
			CancelAt: cancelAfter(text, pub),
		})
	}
	return out, ""
}

// navareaRegion — номер НАВАРЕА → ключ региона. Самый надёжный признак:
// один и тот же номер встречается в названиях морей в разных формулировках.
var navareaRegion = map[string]string{
	"I": "baltic", "II": "north", "III": "black", "IV": "north",
	"XII": "pacific", "XIII": "pacific", "XIV": "pacific",
	"XX": "arctic", "XXI": "arctic",
}

// reAnyNavarea находит номер НАВАРЕА в любом месте текста.
var reAnyNavarea = regexp.MustCompile(`NAVAREA\s+([IVX]+)\b`)

// coastalKeywords — названия морей и стран для прибрежных предупреждений,
// когда номера НАВАРЕА в тексте нет: «Baltic Area POL-303/26», «Russia Murmansk…».
var coastalKeywords = map[string][]string{
	"black":   {"BLACK SEA", "AZOV", "NOVOROSSIYSK", "SEVASTOPOL", "KERCH", "SOCHI", "TUAPSE", "ROSTOV", "UKRAINE", "GEORGIA", "ROMANIA", "BULGARIA", "TURKEY", "ISTANBUL", "SAMSUN", "TRABZON"},
	"baltic":  {"BALTIC", "KRONSTADT", "FINLAND", "FINNISH", "ST PETERSBURG", "PRIMORSK", "VYBORG", "KALININGRAD", "DENMARK", "SWEDEN", "POLAND", "GERMANY", "ESTONIA", "LATVIA", "LITHUANIA", "RIGA", "TALLINN", "HELSINKI"},
	"arctic":  {"BARENTS", "KARA", "LAPTEV", "CHUKCHI", "MURMANSK", "ARKHANGELSK", "WHITE SEA", "SVALBARD", "YAMAL", "NORTHERN SEA ROUTE", "PECHORA"},
	"north":   {"NORTH SEA", "NORWEGIAN", "ATLANTIC", "FAEROE", "SHETLAND", "ICELAND"},
	"med":     {"MEDITERRANEAN", "AEGEAN", "ADRIATIC", "TYRRHENIAN", "IONIAN", "CYPRUS", "MALTA", "GIBRALTAR"},
	"pacific": {"JAPAN", "OKHOTSK", "BERING", "VLADIVOSTOK", "PACIFIC", "KOREA", "CHINA", "YELLOW SEA", "NAKHODKA", "PETROPAVLOVSK", "KAMCHATKA", "SAKHALIN"},
	"caspian": {"CASPIAN", "ASTRAKHAN", "MAKHACHKALA"},
}

// hasWord ищет ключевое слово как отдельное слово: «KARA» не должно
// срабатывать на «KARACHI», «ANKARA» и подобных названиях.
func hasWord(up, kw string) bool {
	for i := 0; ; {
		j := strings.Index(up[i:], kw)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isLetter(up[j-1])
		k := j + len(kw)
		after := k >= len(up) || !isLetter(up[k])
		if before && after {
			return true
		}
		i = j + 1
		if i >= len(up) {
			return false
		}
	}
}

// isLetter — ASCII-буква. Ключевые слова в coastalKeywords заданы латиницей,
// поэтому проверки по байтам достаточно; кириллица обрабатывается как разделитель.
func isLetter(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

// regionFor определяет регион по тексту предупреждения.
func regionFor(text string) string {
	up := strings.ToUpper(text)
	// 1) Признаки Чёрного моря проверяем первыми: НАВАРЕА III покрывает и его,
	//    и Средиземное море, а для русских судов приоритет — Чёрное.
	if hasWord(up, "BLACK SEA") || hasWord(up, "AZOV") || hasWord(up, "NOVOROSSIYSK") {
		return "black"
	}
	// 2) НАВАРЕА по номеру — самый надёжный признак.
	if m := reAnyNavarea.FindStringSubmatch(up); m != nil {
		if k, ok := navareaRegion[m[1]]; ok {
			return k
		}
	}
	// 3) Названия морей, портов и стран для прибрежных выпусков.
	for _, r := range Regions {
		for _, kw := range coastalKeywords[r.Key] {
			if hasWord(up, kw) {
				return r.Key
			}
		}
	}
	return ""
}

// reNavareaHead — заголовок вида «NAVAREA III 169/26».
var reNavareaHead = regexp.MustCompile(`(?m)^\s*(NAVAREA\s+[IVX]+)\s*[-–—]?\s*(\d{1,4})\s*/\s*(\d{2,4})\s*$`)

// splitNavarea режет текст района на отдельные предупреждения по заголовкам.
func splitNavarea(area, content string) []Warning {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	locs := reNavareaHead.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		// Заголовков не нашли — отдаём район одним сообщением.
		num, year := "", 0
		return []Warning{{
			Area:   area,
			Number: num,
			Year:   year,
			Title:  firstLine(content),
			Body:   strings.TrimSpace(content),
		}}
	}
	var out []Warning
	for i, loc := range locs {
		end := len(content)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		head := content[loc[0]:loc[1]]
		body := strings.TrimSpace(content[loc[1]:end])
		m := reNavareaHead.FindStringSubmatch(head)
		num, year := "", 0
		if len(m) >= 4 {
			num = m[2]
			if y, err := strconv.Atoi(m[3]); err == nil {
				if y < 100 {
					y += 2000
				}
				year = y
			}
		}
		out = append(out, Warning{
			Area:     area,
			Number:   num,
			Year:     year,
			Title:    firstLine(body),
			Body:     strings.TrimSpace(head + "\n" + body),
			CancelAt: cancelAfter(body, time.Now()),
		})
	}
	return out
}

// parseCoastalTitle разбирает «Russia Murmansk Coastal 415/26» на район и номер.
func parseCoastalTitle(title string) (area, num string, year int) {
	numRe := regexp.MustCompile(`(\d{1,4})\s*/\s*(\d{2,4})`)
	if m := numRe.FindStringSubmatch(title); m != nil {
		num = m[1]
		if y, err := strconv.Atoi(m[2]); err == nil {
			if y < 100 {
				y += 2000
			}
			year = y
		}
	}
	area = title
	if i := strings.Index(title, " Coastal "); i > 0 {
		area = title[:i]
	} else if i := strings.Index(title, " NAVAREA "); i >= 0 {
		area = "NAVAREA " + strings.Fields(title[i:])[1]
	} else if strings.HasPrefix(title, "NAVAREA ") {
		parts := strings.Fields(title)
		if len(parts) > 1 {
			area = "NAVAREA " + parts[1]
		}
	}
	if len(area) > 60 {
		area = area[:60]
	}
	return strings.TrimSpace(area), num, year
}

func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" && !strings.HasPrefix(ln, "-") {
			if len(ln) > 120 {
				ln = ln[:120] + "…"
			}
			return ln
		}
	}
	return ""
}

// cancelAfter ищет фразу «CANCEL THIS MSG 312200 UTC OCT 26» и подобные.
func cancelAfter(body string, fallback time.Time) time.Time {
	re := regexp.MustCompile(`(?i)CANCEL\s+THIS\s+MSG\.?\s+(\d{2})(\d{2})(\d{2})\s*UTC\s+([A-Z]{3})\s+(\d{2})`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		return time.Time{}
	}
	months := map[string]time.Month{"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12}
	mon, ok := months[strings.ToUpper(m[4])]
	if !ok {
		return time.Time{}
	}
	d, _ := strconv.Atoi(m[1])
	h, _ := strconv.Atoi(m[2])
	mi, _ := strconv.Atoi(m[3])
	y, _ := strconv.Atoi(m[5])
	return time.Date(2000+y, mon, d, h, mi, 0, 0, time.UTC)
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	return strings.TrimSpace(re.ReplaceAllString(s, ""))
}

func parseRFC822(s string) time.Time {
	if t, err := time.Parse(time.RFC1123Z, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC1123, s); err == nil {
		return t
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t
	}
	return time.Time{}
}
