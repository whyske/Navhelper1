// Package navwarn загружает и разбирает навигационные предупреждения
// (НАВАРЕА, ПРИП, НАВИП) с официальных сайтов.
//
// Источники — HTML-страницы, на которых тексты предупреждений опубликованы
// в стандартном радиотелеграфном формате:
//
//	NAVAREA XX 165/26
//	POMORSKY STRAIT AND CHYOSHSKAYA GUBA.
//	CHART RUS 10100.
//	1. ROCKET LAUNCHING 252100 UTC TO 312100 UTC OCT ...
//	2. CANCEL THIS MSG 312200 UTC OCT 26.=
//	NNNN
package navwarn

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Warning — одно навигационное предупреждение.
type Warning struct {
	Area     string    // «NAVAREA XX», «ПРИП МУРМАНСК», «НАВИП» ...
	Number   string    // номер, например «165»
	Year     int       // год, например 2026
	Title    string    // первая строка (район)
	Body     string    // полный текст
	Charts   []string  // номера карт, упомянутые в тексте
	CancelAt time.Time // момент автоматической отмены (если указан)
	Source   string    // имя источника
}

// ID — уникальный ключ предупреждения.
func (w Warning) ID() string {
	return fmt.Sprintf("%s %s/%02d", w.Area, w.Number, w.Year%100)
}

// Expired сообщает, что срок действия истёк (по фразе CANCEL THIS MSG).
func (w Warning) Expired(now time.Time) bool {
	return !w.CancelAt.IsZero() && now.After(w.CancelAt)
}

// Source — источник предупреждений.
type Source struct {
	Name string
	URL  string
}

// DefaultSources — официальные источники, которые опрашиваются по умолчанию.
var DefaultSources = []Source{
	// ФГБУ «Главсевморпуть»: действующие NAVAREA XX и XXI (Арктика).
	{Name: "Главсевморпуть", URL: "https://nsr.rosatom.ru/en/navigational-and-weather-information/navarea/"},
	// ФГБУ «АМП Западной Арктики»: действующие ПРИП МУРМАНСК.
	{Name: "АМП Западной Арктики", URL: "https://www.mapm.ru/prip"},
}

// reHeader находит заголовок предупреждения. Группы: 1 — тип, 2 — район
// (римские цифры для NAVAREA или название станции для ПРИП), 3 — номер, 4 — год.
var reHeader = regexp.MustCompile(
	// \b в RE2 работает только для ASCII, поэтому перед кириллицей его не ставим.
	`(?i)(NAVAREA|НАВАРЕА|ПРИП|PRIP|НАВИП|NAVIP)\s+` +
		`([IVXА-ЯЁ][IVXА-ЯЁA-Z\-]*(?:\s+[А-ЯЁ][А-ЯЁ\-]+)?)?\s*` +
		`(\d{1,4})\s*/\s*(\d{2,4})\b`)

// reEnd — признак конца сообщения.
var reEnd = regexp.MustCompile(`(?m)^\s*NNNN\s*$|NNNN`)

// reCharts — номера карт: «CHART RUS 11114», «CHARTS RUS 12000, 12100», «КАРТЫ 38174».
var reCharts = regexp.MustCompile(`(?i)(?:CHARTS?|КАРТ[АЫ]?)\s+(?:RUS\s+)?((?:\d{4,5}(?:\s*[,—\-]\s*|\s+))*\d{4,5})`)

var reDigits = regexp.MustCompile(`\d{4,5}`)

// reCancel — «CANCEL THIS MSG 312200 UTC OCT 26».
var reCancel = regexp.MustCompile(`(?i)CANCEL\s+THIS\s+MSG\s+(\d{2})(\d{2})(\d{2})\s*UTC\s+([A-Z]{3})\s+(\d{2})`)

var months = map[string]time.Month{
	"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
	"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
}

// Client — загрузчик предупреждений.
type Client struct {
	HTTP *http.Client
}

// NewClient создаёт загрузчик с таймаутом.
//
// Важно: приложение НЕ добавляет в доверенные никаких собственных корневых
// сертификатов и ничего не устанавливает в систему устройства. TLS-проверка
// выполняется штатным хранилищем ОС (x509.SystemCertPool). Если сайт ведомства
// подписан сертификатом, которого нет в системе, источник просто пометится
// недоступным — остальные модули продолжат работать.
func NewClient() *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	tr.ForceAttemptHTTP2 = true
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second, Transport: tr}}
}

// Result — итог опроса одного источника.
type Result struct {
	Source   Source
	Warnings []Warning
	Err      error
}

// FetchAll опрашивает все источники параллельно. Ошибка одного
// источника не мешает остальным.
func (c *Client) FetchAll(ctx context.Context, sources []Source) []Result {
	out := make([]Result, len(sources))
	done := make(chan struct{}, len(sources))
	for i, src := range sources {
		go func(i int, src Source) {
			ws, err := c.Fetch(ctx, src)
			out[i] = Result{Source: src, Warnings: ws, Err: err}
			done <- struct{}{}
		}(i, src)
	}
	for range sources {
		<-done
	}
	return out
}

// Fetch скачивает страницу источника и разбирает предупреждения.
func (c *Client) Fetch(ctx context.Context, src Source) ([]Warning, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 13) NavHelper/1.0")
	req.Header.Set("Accept-Language", "ru,en;q=0.8")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	text := HTMLToText(decodeCharset(raw, resp.Header.Get("Content-Type")))
	return Parse(text, src.Name), nil
}

// Parse выделяет предупреждения из обычного текста.
func Parse(text, source string) []Warning {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	locs := reHeader.FindAllStringSubmatchIndex(text, -1)
	seen := map[string]bool{}
	var out []Warning
	for i, loc := range locs {
		kind := strings.ToUpper(text[loc[2]:loc[3]])
		region := ""
		if loc[4] >= 0 {
			region = strings.ToUpper(strings.TrimSpace(text[loc[4]:loc[5]]))
		}
		num := strings.TrimLeft(text[loc[6]:loc[7]], "0")
		if num == "" {
			num = "0"
		}
		year := normYear(text[loc[8]:loc[9]])

		area := normKind(kind)
		if region != "" {
			area += " " + region
		}

		end := len(text)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		body := text[loc[1]:end]
		if m := reEnd.FindStringIndex(body); m != nil {
			body = body[:m[0]]
		}
		body = cleanBody(body)
		// Слишком короткие фрагменты — это ссылки/упоминания, а не текст.
		if utf8.RuneCountInString(body) < 15 {
			continue
		}

		w := Warning{Area: area, Number: num, Year: year, Body: body, Source: source}
		w.Title = firstLine(body)
		w.Charts = extractCharts(text[loc[0]:end])
		w.CancelAt = parseCancel(body, year)
		if seen[w.ID()] {
			continue
		}
		seen[w.ID()] = true
		out = append(out, w)
	}
	Sort(out)
	return out
}

// Sort упорядочивает: по году и номеру, свежие сверху.
func Sort(ws []Warning) {
	sort.SliceStable(ws, func(i, j int) bool {
		if ws[i].Year != ws[j].Year {
			return ws[i].Year > ws[j].Year
		}
		ni, _ := strconv.Atoi(ws[i].Number)
		nj, _ := strconv.Atoi(ws[j].Number)
		if ni != nj {
			return ni > nj
		}
		return ws[i].Area < ws[j].Area
	})
}

func normKind(k string) string {
	switch k {
	case "NAVAREA", "НАВАРЕА":
		return "NAVAREA"
	case "ПРИП", "PRIP":
		return "ПРИП"
	default:
		return "НАВИП"
	}
}

func normYear(s string) int {
	y, _ := strconv.Atoi(s)
	if len(s) == 2 {
		if y < 70 {
			return 2000 + y
		}
		return 1900 + y
	}
	return y
}

func firstLine(body string) string {
	line := body
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		line = body[:i]
	}
	line = strings.TrimSpace(line)
	if utf8.RuneCountInString(line) > 90 {
		r := []rune(line)
		line = string(r[:90]) + "…"
	}
	return line
}

func extractCharts(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range reCharts.FindAllStringSubmatch(s, -1) {
		for _, n := range reDigits.FindAllString(m[1], -1) {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

// parseCancel вычисляет момент отмены из фразы «CANCEL THIS MSG ddhhmm UTC MON yy».
func parseCancel(body string, year int) time.Time {
	m := reCancel.FindStringSubmatch(body)
	if m == nil {
		return time.Time{}
	}
	day, _ := strconv.Atoi(m[1])
	hh, _ := strconv.Atoi(m[2])
	mm, _ := strconv.Atoi(m[3])
	mon, ok := months[strings.ToUpper(m[4])]
	if !ok {
		return time.Time{}
	}
	y := normYear(m[5])
	if y == 0 {
		y = year
	}
	return time.Date(y, mon, day, hh, mm, 0, 0, time.UTC)
}

// cleanBody убирает пустые строки, лишние пробелы и завершающий «=».
func cleanBody(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.Join(strings.Fields(l), " ")
		if l == "" {
			continue
		}
		out = append(out, l)
	}
	res := strings.Join(out, "\n")
	res = strings.TrimSpace(strings.TrimRight(res, "= \n"))
	return res
}

// ─────────────────────────── HTML → текст ───────────────────────────

var (
	reScript = regexp.MustCompile(`(?is)<(script|style|noscript|svg)[^>]*>.*?</(script|style|noscript|svg)>`)
	reBreak  = regexp.MustCompile(`(?i)<\s*(br|/p|/div|/li|/tr|/h[1-6]|p|div|li|tr|h[1-6])\b[^>]*>`)
	reTag    = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces = regexp.MustCompile(`[ \t\x{00A0}]+`)
)

// HTMLToText превращает HTML в простой текст с сохранением переносов строк.
func HTMLToText(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reBreak.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = reSpaces.ReplaceAllString(s, " ")
	return s
}

// decodeCharset переводит windows-1251 в UTF-8, если страница не в UTF-8.
func decodeCharset(b []byte, contentType string) string {
	ct := strings.ToLower(contentType)
	head := strings.ToLower(string(b[:min(len(b), 2048)]))
	is1251 := strings.Contains(ct, "1251") ||
		(!strings.Contains(ct, "utf-8") && strings.Contains(head, "windows-1251"))
	if !is1251 && utf8.Valid(b) {
		return string(b)
	}
	return DecodeCP1251(b)
}

// DecodeCP1251 декодирует байты в кодировке windows-1251.
func DecodeCP1251(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b) * 2)
	for _, c := range b {
		switch {
		case c < 0x80:
			sb.WriteByte(c)
		case c >= 0xC0:
			sb.WriteRune(rune(0x0410 + int(c) - 0xC0)) // А..я
		case c == 0xA8:
			sb.WriteRune('Ё')
		case c == 0xB8:
			sb.WriteRune('ё')
		case c == 0xB9:
			sb.WriteRune('№')
		case c == 0xAB:
			sb.WriteRune('«')
		case c == 0xBB:
			sb.WriteRune('»')
		case c == 0x96:
			sb.WriteRune('–')
		case c == 0x97:
			sb.WriteRune('—')
		case c == 0xA0:
			sb.WriteRune(' ')
		default:
			sb.WriteRune(' ')
		}
	}
	return sb.String()
}
