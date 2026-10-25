package main

import (
	"strings"
	"testing"
)

func TestShiftAndDiacritics(t *testing.T) {
	// «Rü» закодировано как R + u#M0, все символы ≥0x90 сдвинуты на 0x90.
	enc := func(s string) string {
		var sb strings.Builder
		for _, r := range s {
			if r >= 0x80 || r >= 'A' && r <= 'z' || r >= '0' && r <= '9' || r == '#' {
				sb.WriteRune(r + 0x90)
			} else {
				sb.WriteRune(r)
			}
		}
		return sb.String()
	}
	if got := shift(enc("Port Ustka to Ru#M0gen")); got != "Port Ustka to Ru#M0gen" {
		t.Fatalf("shift: %q", got)
	}
	if got := shift(enc("Новороссийск")); got != "Новороссийск" {
		t.Fatalf("shift ru: %q", got)
	}
	cases := map[string]string{
		"Ru#M0gen":          "Rügen",
		"Lu#M0deritz":       "Lüderitz",
		"Ilhe#M1us":         "Ilhéus",
		"Bouveto#M9ya":      "Bouvetøya",
		"A#M8land":          "Åland",
		"Ac#M7ores":         "Açores",
		"Sa#M6o Tome#M1":    "São Tomé",
		"S#M4ventoji":       "Šventoji",
		"I#M3les Kerguelen": "Îles Kerguelen",
		"no markers":        "no markers",
	}
	for in, want := range cases {
		if got := diacritics(in); got != want {
			t.Errorf("diacritics(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPolyline(t *testing.T) {
	// Пример из документации Google: (38.5,-120.2), (40.7,-120.95), (43.252,-126.453).
	pts := decodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@")
	if len(pts) != 3 || pts[0][0] != 38.5 || pts[2][1] != -126.453 {
		t.Fatalf("decodePolyline: %v", pts)
	}
}

func TestRegionOf(t *testing.T) {
	if regionOf("Black Sea. Coast of Russia. Piers of Port Novorossiysk") != "Black Sea" {
		t.Error("regionOf")
	}
	if regionOf("Baltic Sea.") != "Baltic Sea" {
		t.Error("regionOf без второй части")
	}
}
