package navwarn

import (
	"strings"
	"testing"
	"time"
)

// Фрагмент реальной страницы nsr.rosatom.ru (октябрь 2026).
const sampleNSR = `<div class="news-list"><p>NAVAREA XX 165/26<br>
 POMORSKY STRAIT AND CHYOSHSKAYA GUBA.<br>
 CHART RUS 10100. <br>
 1. ROCKET LAUNCHING 252100 UTC TO 312100 UTC OCT<br>
 NAVIGATION PROHIBITED IN TERRITORIAL WATERS<br>
 DANGEROUS OUTSIDE IN AREAS BOUNDED BY:<br>
 A. 68-55.20N 051-20.35E, 68-40.47N 050-49.44E,<br>
 2. CANCEL THIS MSG 312200 UTC OCT 26.=<br>
 NNNN</p>
<p>NAVAREA XX 164/26<br>
 BARENTS SEA.<br>
 CHART RUS 11114.<br>
 1. MISSILE FIRINGS 0400 TO 1500 UTC DAILY<br>
 25 TO 28 OCT NAVIGATION PROHIBITED IN<br>
 2. CANCEL THIS MSG 241600 UTC OCT 26.=<br>
 NNNN</p>
<p>NAVAREA XX 102/26 CHARTS RUS 12000.<br>
 BARENTS SEA.<br>
 ANCHORAGE EQUIPMENT ESTABLISHED IN:<br>
 1. 69-43.78N 033-59.77E,<br>
 2. 69-43.80N 034-00.23E.=<br>
 NNNN</p>
<p>NAVAREA XX 165/26<br> POMORSKY STRAIT (дубль на английской версии страницы).<br>NNNN</p>
</div>`

func TestParseNSR(t *testing.T) {
	ws := Parse(HTMLToText(sampleNSR), "test")
	if len(ws) != 3 {
		for _, w := range ws {
			t.Logf("%s | %s", w.ID(), w.Title)
		}
		t.Fatalf("ожидалось 3 предупреждения (без дубля), получено %d", len(ws))
	}
	w := ws[0]
	if w.ID() != "NAVAREA XX 165/26" || w.Year != 2026 {
		t.Errorf("первое: %s (%d)", w.ID(), w.Year)
	}
	if w.Title != "POMORSKY STRAIT AND CHYOSHSKAYA GUBA." {
		t.Errorf("заголовок: %q", w.Title)
	}
	if len(w.Charts) != 1 || w.Charts[0] != "10100" {
		t.Errorf("карты: %v", w.Charts)
	}
	wantCancel := time.Date(2026, 10, 31, 22, 0, 0, 0, time.UTC)
	if !w.CancelAt.Equal(wantCancel) {
		t.Errorf("отмена: %v", w.CancelAt)
	}
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	if w.Expired(now) {
		t.Error("165/26 ещё действует 25.10")
	}
	if !ws[1].Expired(now) {
		t.Error("164/26 отменено 24.10 16:00 UTC")
	}
	if ws[2].Number != "102" || ws[2].Charts[0] != "12000" || !ws[2].CancelAt.IsZero() {
		t.Errorf("102/26: %+v", ws[2])
	}
	if strings.Contains(w.Body, "<br>") || strings.Contains(w.Body, "NNNN") {
		t.Error("в тексте остались теги или NNNN")
	}
}

func TestParsePRIPCyrillic(t *testing.T) {
	text := `ДЕЙСТВУЮЩИЕ ПРИП МУРМАНСК НА 240000 ОКТ 2026 ГОДА
ПРИП МУРМАНСК 278/26 КАРТЫ 15004, 15005
КОЛЬСКИЙ ЗАЛИВ. ДНОУГЛУБИТЕЛЬНЫЕ РАБОТЫ 25 ПО 30 ОКТ
РАЙОН 69-05.0N 033-05.0E. ПЛАВАНИЕ С ОСТОРОЖНОСТЬЮ.
NNNN
ПРИП МУРМАНСК 96/24
БАРЕНЦЕВО МОРЕ. ОГОНЬ МАЯКА НЕ ГОРИТ.
NNNN`
	ws := Parse(text, "test")
	if len(ws) != 2 {
		t.Fatalf("ожидалось 2, получено %d: %+v", len(ws), ws)
	}
	if ws[0].Area != "ПРИП МУРМАНСК" || ws[0].Number != "278" || ws[0].Year != 2026 {
		t.Errorf("первое: %+v", ws[0])
	}
	if len(ws[0].Charts) != 2 || ws[0].Charts[1] != "15005" {
		t.Errorf("карты: %v", ws[0].Charts)
	}
	if ws[1].Year != 2024 {
		t.Errorf("год второго: %d", ws[1].Year)
	}
}

func TestDecodeCP1251(t *testing.T) {
	// «ПРИП Ёж» в windows-1251.
	b := []byte{0xCF, 0xD0, 0xC8, 0xCF, 0x20, 0xA8, 0xE6}
	if got := DecodeCP1251(b); got != "ПРИП Ёж" {
		t.Errorf("DecodeCP1251 = %q", got)
	}
	html := append([]byte(`<meta charset="windows-1251">`), b...)
	if got := decodeCharset(html, "text/html"); !strings.HasSuffix(got, "ПРИП Ёж") {
		t.Errorf("decodeCharset = %q", got)
	}
}
