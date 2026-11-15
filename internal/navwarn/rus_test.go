package navwarn

import (
	"testing"
)

func TestDetectKindNavarea(t *testing.T) {
	w := Warning{
		Area:  "NAVAREA III",
		Title: "NAVAREA III",
		Body:  "NAVAREA III 0122/22\n1.- POSSIBLE DRIFTING MINES",
	}
	if got := DetectKind(w); got != KindNavarea {
		t.Errorf("DetectKind = %v, want NAVAREA", got)
	}
}

func TestDetectKindPrip(t *testing.T) {
	w := Warning{
		Area:  "Baltic Area EST-188/26",
		Title: "Baltic Area EST-188/26",
		Body:  "Coastal warning. GUNNERY AREA.",
	}
	if got := DetectKind(w); got != KindPrip {
		t.Errorf("DetectKind = %v, want PRIP", got)
	}
}

func TestDetectKindNavip(t *testing.T) {
	w := Warning{
		Area:  "Local Japan Warnings 2nd RCG 02-0264",
		Title: "Local Japan Warnings 2nd RCG 02-0264",
		Body:  "Gunnery by self defence forces.",
	}
	// «Local Warnings» — это прибрежный тип, проверяем что не НАВАРЕА.
	if got := DetectKind(w); got == KindNavarea {
		t.Errorf("DetectKind = %v, не должен быть NAVAREA", got)
	}
}

func TestAreaRUTranslates(t *testing.T) {
	cases := []struct{ in, want string }{
		{"NAVAREA III", "РАЙОН III"},
		{"NAVAREA XX", "РАЙОН XX"},
		{"Baltic Area EST-188/26", "Балтийское море"},
		{"Coastal Estonia 188", "Эстония"},
		{"Black Sea", "Чёрное море"},
	}
	for _, c := range cases {
		got := AreaRU(c.in)
		if got == "" {
			t.Errorf("AreaRU(%q) = пусто, ожидалось %q", c.in, c.want)
			continue
		}
		if !containsStr(got, c.want) {
			t.Errorf("AreaRU(%q) = %q, ожидалось вхождение %q", c.in, got, c.want)
		}
	}
}

func containsStr(hay, needle string) bool {
	return len(hay) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(hay); i++ {
			if hay[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}

func TestTranslateTitle(t *testing.T) {
	if got := TranslateTitle("GUNNERY AREA"); got != "Учебные стрельбы" {
		t.Errorf("TranslateTitle = %q", got)
	}
	// Неизвестный текст возвращается как есть.
	if got := TranslateTitle("XYZ UNKNOWN"); got != "XYZ UNKNOWN" {
		t.Errorf("TranslateTitle вернул %q, ожидался исходный текст", got)
	}
}

func TestGroupByKindOrder(t *testing.T) {
	list := []Warning{
		{Area: "Baltic Area EST-1/26", Title: "Coastal", Body: "Coastal warning"},
		{Area: "NAVAREA III", Title: "NAVAREA III", Body: "NAVAREA III 1/26"},
	}
	groups := GroupByKind(list)
	if len(groups) != 2 {
		t.Fatalf("групп %d, ожидалось 2", len(groups))
	}
	// НАВАРЕА должна идти первой.
	if groups[0].Kind != KindNavarea {
		t.Errorf("первая группа %v, ожидалась NAVAREA", groups[0].Kind)
	}
}
