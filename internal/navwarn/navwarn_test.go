package navwarn

import "testing"

func TestRegionForNavareaNumbers(t *testing.T) {
	cases := []struct{ text, want string }{
		{"NAVAREA III 169/26 BLACK SEA NORTHWESTERN PART", "black"},
		{"NAVAREA I 238/26 SOUTHERN NORTH SEA", "baltic"},
		{"NAVAREA XX 165/26 POMORSKY STRAIT", "arctic"},
		{"NAVAREA XII 45/26 OKHOTSK SEA", "pacific"},
	}
	for _, c := range cases {
		if got := regionFor(c.text); got != c.want {
			t.Errorf("regionFor(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

func TestRegionForCoastalKeywords(t *testing.T) {
	cases := []struct{ text, want string }{
		{"Baltic Area POL-303/26", "baltic"},
		{"Russia Murmansk Coastal 415/26", "arctic"},
		{"Turkey Antalya Navtex", "black"},
	}
	for _, c := range cases {
		if got := regionFor(c.text); got != c.want {
			t.Errorf("regionFor(%q) = %q, want %q", c.text, got, c.want)
		}
	}
}

// «KARA» встречается в «KARACHI» — не должно давать Арктику.
func TestRegionNoSubstringFalsePositive(t *testing.T) {
	if got := regionFor("PAKISTAN KARACHI PORT CLOSED"); got == "arctic" {
		t.Errorf("KARACHI ошибочно отнесён к Арктике")
	}
}

func TestSplitNavareaByHeaders(t *testing.T) {
	content := "\r\n NAVAREA III 169/26\r\n BLACK SEA.\r\n 1. TEST ONE.=\r\n" +
		"\r\n NAVAREA III 170/26\r\n AZOV SEA.\r\n 2. TEST TWO.=\r\n"
	list := splitNavarea("NAVAREA III", content)
	if len(list) != 2 {
		t.Fatalf("ожидалось 2 предупреждения, получено %d", len(list))
	}
	if list[0].Number != "169" || list[1].Number != "170" {
		t.Errorf("номера разобраны неверно: %q, %q", list[0].Number, list[1].Number)
	}
}
