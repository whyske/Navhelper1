// Русские названия и классификация для раздела «ИМ УНиО».
//
// ═══════════════════════════════════════════════════════════════════════════
//  ЗАЧЕМ ЭТОТ ФАЙЛ
// ═══════════════════════════════════════════════════════════════════════════
// Источники (SeaLagom, национальные службы) отдают тексты на английском:
// «NAVAREA II», «Baltic Area EST-188/26», «Coastal Estonia 188».
// В приложении это надо показывать по-русски и группировать по типу выпуска:
//
//   НАВАРЕА — океанские предупреждения (20 районов, координируются странами)
//   ПРИП    — прибрежные предупреждения (Coastal warnings)
//   НАВИП   — навигационные извещения (локальные, не входящие в НАВАРЕА)
//
// Плюс номер выпуска ИМ УНиО (например «выпуск 46/2026»), чтобы их можно было
// раскладывать по выпадающим спискам.
//
// ВАЖНО: файл ничего не грузит из сети, только переводит и раскладывает
// уже загруженные данные. Его безопасно менять — сетевой код не затронут.
package navwarn

import (
	"fmt"
	"regexp"
	"strings"
)

// ───────────────────────────── типы выпусков ─────────────────────────────

// Kind — тип навигационного извещения.
type Kind string

const (
	KindNavarea Kind = "NAVAREA" // океанские предупреждения НАВАРЕА
	KindPrip    Kind = "PRIP"    // прибрежные предупреждения
	KindNavip   Kind = "NAVIP"   // навигационные извещения (локальные)
)

// KindRU — русское название типа для интерфейса.
func (k Kind) RU() string {
	switch k {
	case KindNavarea:
		return "НАВАРЕА"
	case KindPrip:
		return "ПРИП"
	case KindNavip:
		return "НАВИП"
	}
	return string(k)
}

// kindOrder — порядок групп в списке: сначала океанские, потом прибрежные.
var kindOrder = []Kind{KindNavarea, KindPrip, KindNavip}

// ──────────────────── русские названия районов НАВАРЕА ───────────────────

// navareaRU — название района НАВАРЕА по-русски по римской цифре.
// Координатор указан в скобках: он публикует предупреждения этого района.
var navareaRU = map[string]string{
	"I":    "РАЙОН I — Балтийское море (Швеция)",
	"II":   "РАЙОН II — Северное море (Великобритания)",
	"III":  "РАЙОН III — Средиземное и Чёрное моря (Испания)",
	"IV":   "РАЙОН IV — Северная Атлантика (США)",
	"V":    "РАЙОН V — Западная Атлантика (Бразилия)",
	"VI":   "РАЙОН VI — Юго-западная Атлантика (Аргентина)",
	"VII":  "РАЙОН VII — Южная Атлантика (ЮАР)",
	"VIII": "РАЙОН VIII — Индийский океан (Индия)",
	"IX":   "РАЙОН IX — Аравийское море (Пакистан)",
	"X":    "РАЙОН X — Австралия",
	"XI":   "РАЙОН XI — Юго-восточная Азия (Индонезия)",
	"XII":  "РАЙОН XII — Северо-западная часть Тихого океана (США)",
	"XIII": "РАЙОН XIII — Северо-западная Пацифика (Россия)",
	"XIV":  "РАЙОН XIV — Южная Пацифика (Новая Зеландия)",
	"XV":   "РАЙОН XV — Юго-восточная Пацифика (Чили)",
	"XVI":  "РАЙОН XVI — Восточная Пацифика (Перу)",
	"XVII": "РАЙОН XVII — Центральная Пацифика (Франция)",
	"XVIII": "РАЙОН XVIII — Южная Пацифика (Австралия)",
	"XIX":  "РАЙОН XIX — Северо-восточная Азия (Норвегия)",
	"XX":   "РАЙОН XX — Арктика, запад (Россия)",
	"XXI":  "РАЙОН XXI — Арктика, восток (Россия)",
}

// AreaRU переводит название района/страны на русский. Работает по ключевым
// словам: «Baltic Area EST-188/26» → «Балтика, Эстония».
// Возвращает пустую строку, если ничего не распознано — тогда показываем
// исходное название, чтобы информация не потерялась.
func AreaRU(s string) string {
	up := strings.ToUpper(s)

	// НАВАРЕА — приоритетный случай.
	if m := reAnyNavarea.FindStringSubmatch(up); m != nil {
		num := strings.TrimSpace(m[1])
		if ru, ok := navareaRU[num]; ok {
			return ru
		}
		return "РАЙОН " + num
	}

	// Страны и моря в прибрежных предупреждениях.
	type pair struct{ keys []string; ru string }
	table := []pair{
		{[]string{"ESTONIA", "EST-"}, "Эстония"},
		{[]string{"LATVIA"}, "Латвия"},
		{[]string{"LITHUANIA"}, "Литва"},
		{[]string{"FINLAND", "FINNISH"}, "Финляндия"},
		{[]string{"SWEDEN", "SWEDISH"}, "Швеция"},
		{[]string{"DENMARK", "DANISH"}, "Дания"},
		{[]string{"NORWAY", "NORWEGIAN"}, "Норвегия"},
		{[]string{"POLAND", "POLISH"}, "Польша"},
		{[]string{"GERMANY", "GERMAN"}, "Германия"},
		{[]string{"NETHERLANDS", "DUTCH"}, "Нидерланды"},
		{[]string{"BELGIUM"}, "Бельгия"},
		{[]string{"FRANCE", "FRENCH"}, "Франция"},
		{[]string{"SPAIN", "SPANISH"}, "Испания"},
		{[]string{"PORTUGAL"}, "Португалия"},
		{[]string{"ITALY", "ITALIAN"}, "Италия"},
		{[]string{"GREECE", "GREEK"}, "Греция"},
		{[]string{"TURKEY", "TURKISH"}, "Турция"},
		{[]string{"ROMANIA"}, "Румыния"},
		{[]string{"BULGARIA"}, "Болгария"},
		{[]string{"UKRAINE"}, "Украина"},
		{[]string{"GEORGIA"}, "Грузия"},
		{[]string{"RUSSIA", "RUSSIAN"}, "Россия"},
		{[]string{"JAPAN"}, "Япония"},
		{[]string{"CHINA", "CHINESE"}, "Китай"},
		{[]string{"KOREA"}, "Корея"},
		{[]string{"INDIA", "INDIAN"}, "Индия"},
		{[]string{"PAKISTAN"}, "Пакистан"},
		{[]string{"BRAZIL"}, "Бразилия"},
		{[]string{"ARGENTINA"}, "Аргентина"},
		{[]string{"CANADA"}, "Канада"},
		{[]string{"AUSTRALIA"}, "Австралия"},
		{[]string{"NEW ZEALAND"}, "Новая Зеландия"},
		{[]string{"USA", "UNITED STATES"}, "США"},
		{[]string{"EGYPT"}, "Египет"},
		{[]string{"MOROCCO"}, "Марокко"},
		{[]string{"SOUTH AFRICA"}, "ЮАР"},
		// Моря и заливы
		{[]string{"BLACK SEA"}, "Чёрное море"},
		{[]string{"AZOV"}, "Азовское море"},
		{[]string{"BALTIC"}, "Балтийское море"},
		{[]string{"GULF OF FINLAND"}, "Финский залив"},
		{[]string{"NORTH SEA"}, "Северное море"},
		{[]string{"BARENTS"}, "Баренцево море"},
		{[]string{"KARA SEA"}, "Карское море"},
		{[]string{"LAPTEV"}, "Море Лаптевых"},
		{[]string{"MEDITERRANEAN"}, "Средиземное море"},
		{[]string{"CASPIAN"}, "Каспийское море"},
		{[]string{"KRONSTADT"}, "Кронштадт"},
		{[]string{"MURMANSK"}, "Мурманск"},
		{[]string{"NOVOROSSIYSK"}, "Новороссийск"},
		{[]string{"SEVASTOPOL"}, "Севастополь"},
		{[]string{"VLADIVOSTOK"}, "Владивосток"},
		{[]string{"KALININGRAD"}, "Калининград"},
	}

	var found []string
	seen := map[string]bool{}
	for _, p := range table {
		for _, k := range p.keys {
			if strings.Contains(up, k) && !seen[p.ru] {
				seen[p.ru] = true
				found = append(found, p.ru)
			}
		}
	}
	if len(found) == 0 {
		return ""
	}
	// Море важнее страны: «Финский залив, Финляндия» читается странно,
	// поэтому если есть море/залив — показываем только его и страну рядом.
	return strings.Join(found, ", ")
}

// ─────────────────────── номер выпуска ИМ УНиО ───────────────────────────

var (
	// «2026 SERIES: 61, 242, 243» — серия выпусков года.
	reSeries = regexp.MustCompile(`(?i)(\d{4})\s+SERIES\s*:?\s*([\d,\s]+)`)
	// «NAVAREA I 244/26» — номер предупреждения.
	reWarnNum = regexp.MustCompile(`(?m)^\s*(NAVAREA\s+[IVX]+|\S+)\s+(\d{1,4})\s*/\s*(\d{2,4})\s*$`)
	// «EST-188/26» — номер прибрежного выпуска.
	reCoastalNum = regexp.MustCompile(`(?i)([A-Z]{2,10})[- ]?(\d{1,4})\s*/\s*(\d{2,4})`)
)

// IssueNumber — номер выпуска, к которому относится предупреждение.
// Нужен для выпадающих списков «выпуск 46/2026». Пустая строка, если номер
// определить не удалось.
func IssueNumber(w Warning) string {
	if w.Year > 0 && w.Number != "" {
		return fmt.Sprintf("%s/%d", w.Number, w.Year)
	}
	if w.Number != "" {
		return w.Number
	}
	return ""
}

// DetectKind определяет тип извещения по его тексту и источнику.
//
// Логика:
//   - если в тексте есть «NAVAREA» с номером района — это НАВАРЕА;
//   - если источник/заголовок говорит о прибрежных (Coastal, AVURNAV,
//     Local) — это ПРИП;
//   - иначе НАВИП (локальные извещения, не попавшие в НАВАРЕА).
func DetectKind(w Warning) Kind {
	up := strings.ToUpper(w.Title + " " + w.Area + " " + w.Body)

	// Явное указание НАВАРЕА в начале сообщения.
	if reNavareaHead.MatchString(w.Body) || reAnyNavarea.MatchString(w.Area) {
		return KindNavarea
	}
	// Прибрежные: так называют себя многие национальные службы.
	for _, kw := range []string{
		"COASTAL", "AVURNAV", "LOCAL WARNING", "LOCAL TOULON",
		"PRIB", "COASTAL WARNING",
	} {
		if strings.Contains(up, kw) {
			return KindPrip
		}
	}
	// Российские обозначения, если источник их отдаёт.
	if strings.Contains(up, "ПРИП") {
		return KindPrip
	}
	if strings.Contains(up, "НАВИП") || strings.Contains(up, "NAVIP") {
		return KindNavip
	}
	// Всё остальное — локальные навигационные извещения.
	return KindNavip
}

// Group — группа выпусков одного типа (НАВАРЕА / ПРИП / НАВИП).
type Group struct {
	Kind     Kind
	TitleRU  string // «НАВАРЕА — океанские предупреждения»
	Warnings []Warning
}

// GroupByKind раскладывает предупреждения по типам в фиксированном порядке.
// Пустые группы не возвращаются.
func GroupByKind(list []Warning) []Group {
	buckets := map[Kind][]Warning{}
	for _, w := range list {
		k := DetectKind(w)
		buckets[k] = append(buckets[k], w)
	}
	var out []Group
	for _, k := range kindOrder {
		ws := buckets[k]
		if len(ws) == 0 {
			continue
		}
		out = append(out, Group{Kind: k, TitleRU: kindTitleRU(k), Warnings: ws})
	}
	return out
}

// kindTitleRU — заголовок группы для выпадающего списка.
func kindTitleRU(k Kind) string {
	switch k {
	case KindNavarea:
		return "НАВАРЕА — океанские предупреждения"
	case KindPrip:
		return "ПРИП — прибрежные предупреждения"
	case KindNavip:
		return "НАВИП — навигационные извещения"
	}
	return string(k)
}

// ───────────────────── русские названия типов явлений ────────────────────

// phenomenonRU — перевод частых английских формулировок в текстах
// предупреждений. Применяется к заголовку, тело остаётся как в оригинале:
// полный машинный перевод морских текстов ненадёжен и может исказить смысл.
var phenomenonRU = []struct{ en, ru string }{
	{"GUNNERY", "Учебные стрельбы"},
	{"FIRING", "Стрельбы"},
	{"MILITARY EXERCISE", "Военные учения"},
	{"EXERCISE", "Учения"},
	{"ROCKET LAUNCHING", "Пуски ракет"},
	{"MISSILE", "Ракетные стрельбы"},
	{"MINE", "Минная опасность"},
	{"DRIFTING MINE", "Дрейфующие мины"},
	{"UNDERWATER OPERATIONS", "Подводные работы"},
	{"DIVING", "Водолазные работы"},
	{"SURVEY", "Промерные работы"},
	{"DREDGING", "Дноуглубительные работы"},
	{"CABLE", "Прокладка кабеля"},
	{"PIPELINE", "Прокладка трубопровода"},
	{"WRECK", "Затонувшее судно"},
	{"OBSTRUCTION", "Препятствие"},
	{"BUOY", "Буй"},
	{"LIGHT", "Огонь"},
	{"FAIRWAY", "Фарватер"},
	{"CHANNEL", "Канал"},
	{"CONSTRUCTION", "Строительные работы"},
	{"RESTRICTED", "Ограничения"},
	{"PROHIBITED", "Проход запрещён"},
	{"CLOSED", "Закрыто"},
	{"SEARCH AND RESCUE", "Поисково-спасательные работы"},
	{"POLLUTION", "Загрязнение"},
	{"OIL SPILL", "Разлив нефти"},
	{"FLOATING", "Плавучий объект"},
	{"TSUNAMI", "Цунами"},
	{"ICE", "Лёд"},
	{"STORM", "Шторм"},
}

// TranslateTitle переводит заголовок предупреждения на русский по ключевым
// словам. Если ничего не распознано — возвращает исходный заголовок, чтобы
// не терять информацию.
func TranslateTitle(title string) string {
	up := strings.ToUpper(title)
	var parts []string
	for _, p := range phenomenonRU {
		if strings.Contains(up, p.en) {
			parts = append(parts, p.ru)
		}
	}
	if len(parts) == 0 {
		return title
	}
	// Дубликаты убираем, порядок сохраняем.
	seen := map[string]bool{}
	var uniq []string
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	return strings.Join(uniq, ", ")
}
