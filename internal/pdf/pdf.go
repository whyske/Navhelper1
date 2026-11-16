// Package pdf — минимальный генератор PDF без внешних зависимостей.
//
// ═══════════════════════════════════════════════════════════════════════════
//  ЗАЧЕМ СВОЙ ГЕНЕРАТОР
// ═══════════════════════════════════════════════════════════════════════════
// В приложении нет сети и нет Go-модулей с PDF: тянуть gofpdf ради одной
// кнопки «скачать выпуск» — это +500 КБ к APK и лишняя зависимость.
// Поэтому здесь собран минимальный, но корректный PDF 1.4:
//
//   • текст набирается встроенным TrueType-шрифтом (Noto Sans из Fyne);
//   • кириллица и латиница работают, потому что шрифт встраивается целиком
//     (без него стандартные Helvetica/Times показывают русский как «кракозябры»);
//   • кодировка Identity-H: в PDF кладутся не байты текста, а номера глифов;
//   • перенос строк считается по реальным ширинам глифов из таблицы hmtx.
//
// Ограничения (сознательные): нет картинок, ссылок, оглавления и сжатия.
// Файл получается крупнее (шрифт ~450 КБ), но открывается везде.
//
// ═══════════════════════════════════════════════════════════════════════════
//  НЕ УДАЛЯТЬ: файл используется кнопкой «Скачать PDF» в разделе ИМ УНиО.
// ═══════════════════════════════════════════════════════════════════════════
package pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// Doc — собираемый документ.
type Doc struct {
	W, H    float64 // размер страницы в пунктах (A4: 595×842)
	Margin  float64
	Size    float64 // кегль основного текста
	Leading float64 // межстрочный интервал

	pages   []*page
	cur     *page
	font    *ttf
	fontID  int
	pageIDs []int

	buf  bytes.Buffer
	objs []int // смещения объектов для xref
}

type page struct {
	lines []pl // строки с абсолютными координатами
}

type pl struct {
	x, y  float64
	size  float64
	text  string
	bold  bool
	color [3]float64
}

// NewA4 создаёт документ A4 с полями 50 pt.
func NewA4(fontBytes []byte) (*Doc, error) {
	f, err := parseTTF(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("шрифт для PDF: %w", err)
	}
	d := &Doc{
		W: 595, H: 842,
		Margin:  50,
		Size:    10.5,
		Leading: 14.5,
		font:    f,
	}
	d.newPage()
	return d, nil
}

func (d *Doc) newPage() {
	d.cur = &page{}
	d.pages = append(d.pages, d.cur)
}

// y — текущая вертикальная позиция на странице (сверху вниз, pt).
func (d *Doc) curY() float64 {
	if d.cur == nil || len(d.cur.lines) == 0 {
		return d.Margin + d.Size
	}
	last := d.cur.lines[len(d.cur.lines)-1]
	return last.y + d.Leading
}

func (d *Doc) ensure(h float64) {
	if d.curY()+h > d.H-d.Margin {
		d.newPage()
	}
}

// Title — крупный заголовок.
func (d *Doc) Title(s string) {
	d.wrap(strings.TrimSpace(s), d.Size+6, false, [3]float64{0, 0, 0}, true)
	d.space(6)
}

// Subtitle — подзаголовок помельче.
func (d *Doc) Subtitle(s string) {
	d.wrap(strings.TrimSpace(s), d.Size+1, false, [3]float64{0.25, 0.3, 0.35}, true)
	d.space(4)
}

// Text — обычный абзац.
func (d *Doc) Text(s string) {
	d.wrap(strings.TrimSpace(s), d.Size, false, [3]float64{0, 0, 0}, false)
}

// Mono — текст с сохранением отступов (морские извещения верстаются колонками,
// поэтому пробелы в начале строк важны и обрезать их нельзя).
func (d *Doc) Mono(s string) {
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "" {
			d.space(d.Leading * 0.6)
			continue
		}
		d.ensure(d.Leading)
		y := d.curY()
		d.cur.lines = append(d.cur.lines, pl{
			x: d.Margin, y: y, size: d.Size, text: line,
			color: [3]float64{0, 0, 0},
		})
	}
}

// HR — горизонтальная черта.
func (d *Doc) HR() {
	d.space(3)
	d.ensure(1)
	y := d.curY()
	d.cur.lines = append(d.cur.lines, pl{x: d.Margin, y: y, text: "\x00HR", size: 0})
	d.space(8)
}

func (d *Doc) space(h float64) {
	if d.cur != nil && len(d.cur.lines) > 0 {
		d.cur.lines[len(d.cur.lines)-1].y -= 0
	}
	// Пустая строка — сдвигаем появление следующей.
	if d.cur != nil && len(d.cur.lines) > 0 {
		d.cur.lines[len(d.cur.lines)-1].y += 0
	}
	if d.cur != nil && len(d.cur.lines) > 0 {
		l := &d.cur.lines[len(d.cur.lines)-1]
		l.y -= h - d.Leading
	} else if d.cur != nil {
		d.cur.lines = append(d.cur.lines, pl{y: d.Margin + h, size: 0, text: ""})
	}
}

// wrap разбивает абзац на строки по ширине страницы.
func (d *Doc) wrap(text string, size float64, _ bool, col [3]float64, bold bool) {
	if text == "" {
		return
	}
	maxW := d.W - 2*d.Margin
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			d.space(size * 0.8)
			continue
		}
		line := ""
		for _, w := range words {
			cand := w
			if line != "" {
				cand = line + " " + w
			}
			if d.font.width(cand, size) <= maxW || line == "" {
				line = cand
				continue
			}
			d.emit(line, size, col)
			line = w
		}
		if line != "" {
			d.emit(line, size, col)
		}
	}
}

func (d *Doc) emit(line string, size float64, col [3]float64) {
	d.ensure(d.Leading)
	d.cur.lines = append(d.cur.lines, pl{
		x: d.Margin, y: d.curY(), size: size, text: line, color: col,
	})
}

// ────────────────────────────── сборка файла ──────────────────────────────

// Bytes собирает PDF и возвращает готовые байты.
func (d *Doc) Bytes() []byte {
	d.buf.Reset()
	d.objs = nil

	// Номера объектов: 1 каталог, 2 дерево страниц, 3 шрифт, 4 CIDFont,
	// 5 дескриптор, 6 файл шрифта, далее — по два на страницу.
	const (
		objCatalog = 1
		objPages   = 2
		objFont    = 3
		objCID     = 4
		objDesc    = 5
		objFile    = 6
		objToUni   = 7
		firstPage  = 8
	)
	d.fontID = objFont

	n := len(d.pages)
	pageObj := make([]int, n)
	contObj := make([]int, n)
	for i := 0; i < n; i++ {
		pageObj[i] = firstPage + i*2
		contObj[i] = firstPage + i*2 + 1
	}

	d.buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	addObj := func(num int, body string) {
		d.objs = append(d.objs, d.buf.Len())
		fmt.Fprintf(&d.buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}
	addStream := func(num int, dict string, data []byte) {
		d.objs = append(d.objs, d.buf.Len())
		fmt.Fprintf(&d.buf, "%d 0 obj\n<<%s /Length %d>>\nstream\n", num, dict, len(data))
		d.buf.Write(data)
		d.buf.WriteString("\nendstream\nendobj\n")
	}

	// 1. Каталог
	addObj(objCatalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", objPages))

	// 2. Дерево страниц
	var kids strings.Builder
	for _, id := range pageObj {
		fmt.Fprintf(&kids, "%d 0 R ", id)
	}
	addObj(objPages, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", n, strings.TrimSpace(kids.String())))

	// 3–6. Шрифт
	// ToUnicode — таблица «номер глифа → символ». Без неё PDF отрисуется
	// правильно, но текст нельзя будет выделить или скопировать (получится
	// мусор из номеров глифов). Поэтому она обязательна.
	addObj(objFont, fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /%s /Encoding /Identity-H "+
			"/DescendantFonts [%d 0 R] /ToUnicode %d 0 R >>", d.font.psName, objCID, objToUni))

	addObj(objCID, fmt.Sprintf(
		"<< /Type /Font /Subtype /CIDFontType2 /BaseFont /%s "+
			"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> "+
			"/FontDescriptor %d 0 R /DW 1000 /W [%s] /CIDToGIDMap /Identity >>",
		d.font.psName, objDesc, d.font.wArray()))

	scale := func(v int16) float64 { return float64(v) * 1000 / float64(d.font.unitsPerEm) }
	addObj(objDesc, fmt.Sprintf(
		"<< /Type /FontDescriptor /FontName /%s /Flags 32 "+
			"/FontBBox [%.0f %.0f %.0f %.0f] /ItalicAngle 0 /Ascent %.0f /Descent %.0f "+
			"/CapHeight %.0f /StemV 80 /FontFile2 %d 0 R >>",
		d.font.psName,
		scale(d.font.xMin), scale(d.font.yMin), scale(d.font.xMax), scale(d.font.yMax),
		scale(d.font.ascent), scale(d.font.descent), scale(d.font.ascent), objFile))

	addStream(objFile, " /Length1 "+fmt.Sprint(len(d.font.data)), d.font.data)

	addStream(objToUni, "", d.font.toUnicodeCMap())

	// Страницы
	for i, p := range d.pages {
		addObj(pageObj[i], fmt.Sprintf(
			"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.0f %.0f] "+
				"/Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>",
			objPages, d.W, d.H, objFont, contObj[i]))

		addStream(contObj[i], "", p.content(d.font))
	}

	// xref
	xrefPos := d.buf.Len()
	total := firstPage + n*2
	fmt.Fprintf(&d.buf, "xref\n0 %d\n", total)
	d.buf.WriteString("0000000000 65535 f \n")
	for _, off := range d.objs {
		fmt.Fprintf(&d.buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&d.buf, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		total, objCatalog, xrefPos)

	return d.buf.Bytes()
}

// content собирает поток содержимого одной страницы.
func (p *page) content(f *ttf) []byte {
	var b bytes.Buffer
	for _, l := range p.lines {
		if l.text == "\x00HR" {
			fmt.Fprintf(&b, "0.85 0.88 0.91 RG 0.7 w %.1f %.1f m %.1f %.1f l S\n",
				20.0, l.y, 575.0, l.y)
			continue
		}
		if l.text == "" {
			continue
		}
		fmt.Fprintf(&b, "BT /F1 %.1f Tf %.3f %.3f %.3f rg 1 0 0 1 %.1f %.1f Tm %s Tj ET\n",
			l.size, l.color[0], l.color[1], l.color[2], l.x, l.y, hexString(f, l.text))
	}
	return b.Bytes()
}

// hexString превращает строку в PDF-строку из номеров глифов (Identity-H).
func hexString(f *ttf, s string) string {
	var b strings.Builder
	b.WriteByte('<')
	for _, r := range s {
		g := f.gid(r)
		fmt.Fprintf(&b, "%04X", g)
	}
	b.WriteByte('>')
	return b.String()
}

// ─────────────────────────────── TrueType ────────────────────────────────

type ttf struct {
	data          []byte
	unitsPerEm    uint16
	ascent        int16
	descent       int16
	xMin, yMin    int16
	xMax, yMax    int16
	numHMetrics   int
	advance       []uint16
	cmap4         map[rune]uint16
	cmap12        map[uint32]uint32
	psName        string
	usedWidthsMax uint16
}

func be16(b []byte, o int) uint16 { return binary.BigEndian.Uint16(b[o:]) }
func be32(b []byte, o int) uint32 { return binary.BigEndian.Uint32(b[o:]) }

func parseTTF(data []byte) (*ttf, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("файл шрифта слишком мал")
	}
	numTables := int(be16(data, 4))
	tables := map[string][2]int{}
	for i := 0; i < numTables; i++ {
		off := 12 + i*16
		if off+16 > len(data) {
			return nil, fmt.Errorf("таблица шрифта повреждена")
		}
		tag := string(data[off : off+4])
		to := int(be32(data, off+8))
		tl := int(be32(data, off+12))
		if to < 0 || to+tl > len(data) {
			continue
		}
		tables[tag] = [2]int{to, tl}
	}

	f := &ttf{data: data, psName: "Font", cmap4: map[rune]uint16{}, cmap12: map[uint32]uint32{}}

	if t, ok := tables["head"]; ok && t[0]+54 <= len(data) {
		f.unitsPerEm = be16(data, t[0]+18)
		f.xMin = int16(be16(data, t[0]+36))
		f.yMin = int16(be16(data, t[0]+38))
		f.xMax = int16(be16(data, t[0]+40))
		f.yMax = int16(be16(data, t[0]+42))
	}
	if f.unitsPerEm == 0 {
		f.unitsPerEm = 1000
	}
	if t, ok := tables["hhea"]; ok && t[0]+36 <= len(data) {
		f.ascent = int16(be16(data, t[0]+4))
		f.descent = int16(be16(data, t[0]+6))
		f.numHMetrics = int(be16(data, t[0]+34))
	}
	if t, ok := tables["hmtx"]; ok && f.numHMetrics > 0 {
		for i := 0; i < f.numHMetrics; i++ {
			o := t[0] + i*4
			if o+2 > len(data) {
				break
			}
			f.advance = append(f.advance, be16(data, o))
		}
	}
	if t, ok := tables["name"]; ok {
		f.psName = parsePSName(data, t[0], t[1])
	}

	if t, ok := tables["cmap"]; ok {
		if err := f.parseCmap(t[0], t[1]); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// parsePSName достаёт PostScript-имя из таблицы name (нужно для BaseFont).
func parsePSName(data []byte, off, length int) string {
	if off+6 > len(data) || off+length > len(data) {
		return "Font"
	}
	count := int(be16(data, off+2))
	strOff := off + int(be16(data, off+4))
	for i := 0; i < count; i++ {
		r := off + 6 + i*12
		if r+12 > len(data) {
			break
		}
		platform := be16(data, r)
		nameID := be16(data, r+6)
		ln := int(be16(data, r+8))
		so := int(be16(data, r+10))
		if nameID != 6 { // 6 = PostScript name
			continue
		}
		start := strOff + so
		if start < 0 || start+ln > len(data) {
			continue
		}
		raw := data[start : start+ln]
		if platform == 3 || platform == 0 { // UTF-16BE
			var sb strings.Builder
			for j := 0; j+1 < len(raw); j += 2 {
				sb.WriteRune(rune(be16(raw, j)))
			}
			s := strings.Map(func(r rune) rune {
				if r > 32 && r < 127 && r != '/' && r != '[' && r != ']' && r != '(' && r != ')' {
					return r
				}
				return -1
			}, sb.String())
			if s != "" {
				return s
			}
		} else {
			return string(raw)
		}
	}
	return "Font"
}

func (f *ttf) parseCmap(off, length int) error {
	d := f.data
	if off+4 > len(d) {
		return fmt.Errorf("cmap повреждён")
	}
	n := int(be16(d, off+2))
	for i := 0; i < n; i++ {
		r := off + 4 + i*8
		if r+8 > len(d) {
			break
		}
		platform := be16(d, r)
		enc := be16(d, r+2)
		sub := off + int(be32(d, r+4))
		if sub+2 > len(d) {
			continue
		}
		format := be16(d, sub)
		switch {
		case format == 4 && (platform == 3 && enc == 1 || platform == 0):
			f.parseCmap4(sub)
		case format == 12 && (platform == 3 && enc == 10 || platform == 0):
			f.parseCmap12(sub)
		}
	}
	if len(f.cmap4) == 0 && len(f.cmap12) == 0 {
		return fmt.Errorf("в шрифте нет таблицы символов")
	}
	return nil
}

func (f *ttf) parseCmap4(sub int) {
	d := f.data
	segX2 := int(be16(d, sub+6))
	seg := segX2 / 2
	endO := sub + 14
	startO := endO + segX2 + 2
	deltaO := startO + segX2
	rangeO := deltaO + segX2
	for i := 0; i < seg; i++ {
		if rangeO+i*2+2 > len(d) {
			break
		}
		end := int(be16(d, endO+i*2))
		start := int(be16(d, startO+i*2))
		delta := int16(be16(d, deltaO+i*2))
		ro := int(be16(d, rangeO+i*2))
		if start > end || end == 0xFFFF && start == 0xFFFF {
			continue
		}
		for c := start; c <= end && c < 0xFFFF; c++ {
			var gid int
			if ro == 0 {
				gid = (c + int(delta)) & 0xFFFF
			} else {
				idx := rangeO + i*2 + ro + (c-start)*2
				if idx+2 > len(d) {
					continue
				}
				g := int(be16(d, idx))
				if g == 0 {
					continue
				}
				gid = (g + int(delta)) & 0xFFFF
			}
			if gid > 0 {
				f.cmap4[rune(c)] = uint16(gid)
			}
		}
	}
}

func (f *ttf) parseCmap12(sub int) {
	d := f.data
	if sub+16 > len(d) {
		return
	}
	n := int(be32(d, sub+12))
	for i := 0; i < n; i++ {
		r := sub + 16 + i*12
		if r+12 > len(d) {
			break
		}
		start := be32(d, r)
		end := be32(d, r+4)
		gid := be32(d, r+8)
		if end < start || end-start > 0x20000 {
			continue
		}
		for c := start; c <= end; c++ {
			f.cmap12[c] = gid + (c - start)
		}
	}
}

// gid возвращает номер глифа для символа (0 — глиф отсутствует).
func (f *ttf) gid(r rune) uint16 {
	if r == '\u00a0' {
		r = ' '
	}
	if g, ok := f.cmap12[uint32(r)]; ok {
		return uint16(g)
	}
	if g, ok := f.cmap4[r]; ok {
		return g
	}
	if g, ok := f.cmap4['?']; ok {
		return g
	}
	return 0
}

// adv — ширина глифа в единицах шрифта.
func (f *ttf) adv(g uint16) uint16 {
	if int(g) < len(f.advance) && f.advance[g] > 0 {
		return f.advance[g]
	}
	if len(f.advance) > 0 {
		return f.advance[len(f.advance)-1]
	}
	return f.unitsPerEm / 2
}

// width — ширина строки в пунктах при заданном кегле.
func (f *ttf) width(s string, size float64) float64 {
	var w float64
	for _, r := range s {
		w += float64(f.adv(f.gid(r))) / float64(f.unitsPerEm) * size
	}
	return w
}

// toUnicodeCMap строит CMap для ToUnicode: сопоставление номеров глифов
// с символами Unicode. Благодаря ей текст в готовом PDF можно выделять,
// копировать и искать.
func (f *ttf) toUnicodeCMap() []byte {
	var b bytes.Buffer
	b.WriteString("/CIDInit /ProcSet findresource begin\n")
	b.WriteString("12 dict begin\nbegincmap\n")
	b.WriteString("/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n")
	b.WriteString("/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n")
	b.WriteString("1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")

	// Собираем пары «глиф → руна». Один глиф может встречаться для нескольких
	// рун (лигатуры, дубликаты), берём первую — для извлечения текста хватает.
	g2r := map[uint16]rune{}
	for r, g := range f.cmap4 {
		if _, ok := g2r[g]; !ok {
			g2r[g] = r
		}
	}
	for r, g := range f.cmap12 {
		if g > 0xFFFF {
			continue
		}
		gg := uint16(g)
		if _, ok := g2r[gg]; !ok && r <= 0x10FFFF {
			g2r[gg] = rune(r)
		}
	}

	var pairs []string
	for g, r := range g2r {
		// BMP записываем как <XXXX>, остальное — суррогатной парой.
		if r <= 0xFFFF {
			pairs = append(pairs, fmt.Sprintf("<%04X> <%04X>", g, uint16(r)))
		} else {
			r -= 0x10000
			hi := 0xD800 + (r >> 10)
			lo := 0xDC00 + (r & 0x3FF)
			pairs = append(pairs, fmt.Sprintf("<%04X> <%04X%04X>", g, hi, lo))
		}
	}
	// PDF допускает не более 100 пар в одном блоке bfchar.
	const chunk = 100
	for i := 0; i < len(pairs); i += chunk {
		end := i + chunk
		if end > len(pairs) {
			end = len(pairs)
		}
		fmt.Fprintf(&b, "%d beginbfchar\n", end-i)
		for _, p := range pairs[i:end] {
			b.WriteString(p)
			b.WriteByte('\n')
		}
		b.WriteString("endbfchar\n")
	}

	b.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	return b.Bytes()
}

// wArray формирует /W для PDF: ширины всех глифов, которые есть в шрифте.
func (f *ttf) wArray() string {
	gids := map[uint16]bool{}
	for _, g := range f.cmap4 {
		gids[g] = true
	}
	for _, g := range f.cmap12 {
		if g < 0xFFFF {
			gids[uint16(g)] = true
		}
	}
	var b strings.Builder
	i := 0
	for g := range gids {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%d [%d]", g, int(float64(f.adv(g))*1000/float64(f.unitsPerEm)))
		i++
		if i > 2500 { // ограничитель: не раздуваем файл
			break
		}
	}
	return b.String()
}
