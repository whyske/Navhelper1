package forecast

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Obs — одно наблюдение METAR, приведённое к морским единицам.
type Obs struct {
	Time     time.Time
	Station  string
	Raw      string
	Dir      float64 // откуда дует, °
	HasDir   bool
	Speed    float64 // средняя скорость, м/с
	HasWind  bool
	Gust     float64 // порывы, м/с
	HasGust  bool
	VisM     float64 // преобладающая/минимальная видимость, м (потолок 10 км)
	HasVis   bool
	Phens    []Phen
	Cloud    int // 0..4
	HasCloud bool
	Temp     float64
	HasTemp  bool
}

const ktToMS = 0.514444

var (
	reWind  = regexp.MustCompile(`^(\d{3}|VRB)(\d{2,3})(?:G(\d{2,3}))?(MPS|KT)$`)
	reVis   = regexp.MustCompile(`^(\d{4})(NDV|N|NE|E|SE|S|SW|W|NW)?$`)
	reVisSM = regexp.MustCompile(`^(\d{1,2})SM$`)
	reTemp  = regexp.MustCompile(`^(M?\d{2})/(M?\d{2})?$`)
	reCloud = regexp.MustCompile(`^(FEW|SCT|BKN|OVC|VV)(\d{3}|///)`)
	reWx    = regexp.MustCompile(`^(\+|-|VC)?(MI|BC|PR|DR|BL|SH|TS|FZ)?((?:DZ|RA|SN|SG|PL|GR|GS|UP)+|BR|FG|FU|HZ|SQ|FC|SS|DS)?$`)
)

// ParseMETAR разбирает строку METAR. Трендовая часть (TEMPO, BECMG, NOSIG)
// и примечания (RMK) отбрасываются — это прогноз, а не наблюдение.
func ParseMETAR(raw string, t time.Time) Obs {
	o := Obs{Raw: raw, Time: t}
	body := " " + raw + " "
	for _, cut := range []string{" NOSIG ", " TEMPO ", " BECMG ", " RMK ", " TREND "} {
		if i := strings.Index(body, cut); i >= 0 {
			body = body[:i]
		}
	}
	tokens := strings.Fields(body)
	minVis := math.Inf(1)
	for i, tok := range tokens {
		if i == 1 && len(tok) == 4 {
			o.Station = tok
		}
		if m := reWind.FindStringSubmatch(tok); m != nil {
			k := 1.0
			if m[4] == "KT" {
				k = ktToMS
			}
			sp, _ := strconv.ParseFloat(m[2], 64)
			o.Speed, o.HasWind = sp*k, true
			if m[1] != "VRB" {
				d, _ := strconv.ParseFloat(m[1], 64)
				o.Dir, o.HasDir = d, sp > 0
			}
			if m[3] != "" {
				g, _ := strconv.ParseFloat(m[3], 64)
				o.Gust, o.HasGust = g*k, true
			}
			continue
		}
		if tok == "CAVOK" {
			minVis = math.Min(minVis, VisMaxM)
			o.Cloud, o.HasCloud = 0, true
			continue
		}
		// Видимость стоит после ветра; не путаем с группой времени/QNH.
		if o.HasWind && !o.HasTemp {
			if m := reVis.FindStringSubmatch(tok); m != nil {
				v, _ := strconv.ParseFloat(m[1], 64)
				if v >= 9999 {
					v = VisMaxM
				}
				minVis = math.Min(minVis, v)
				continue
			}
			if m := reVisSM.FindStringSubmatch(tok); m != nil {
				v, _ := strconv.ParseFloat(m[1], 64)
				minVis = math.Min(minVis, math.Min(v*1609.34, VisMaxM))
				continue
			}
		}
		if m := reTemp.FindStringSubmatch(tok); m != nil && o.HasWind {
			o.Temp, o.HasTemp = metarTemp(m[1]), true
			continue
		}
		switch tok {
		case "NSC", "SKC", "CLR", "NCD":
			if !o.HasCloud {
				o.Cloud, o.HasCloud = 0, true
			}
			continue
		}
		if m := reCloud.FindStringSubmatch(tok); m != nil {
			c := map[string]int{"FEW": 1, "SCT": 2, "BKN": 3, "OVC": 4, "VV": 4}[m[1]]
			if !o.HasCloud || c > o.Cloud {
				o.Cloud = c
			}
			o.HasCloud = true
			continue
		}
		if o.HasWind && !o.HasTemp && tok != "" {
			if p := wxPhen(tok); p != nil {
				o.Phens = append(o.Phens, *p)
			}
		}
	}
	if !math.IsInf(minVis, 1) {
		o.VisM, o.HasVis = minVis, true
	}
	return o
}

func metarTemp(s string) float64 {
	neg := strings.HasPrefix(s, "M")
	v, _ := strconv.ParseFloat(strings.TrimPrefix(s, "M"), 64)
	if neg {
		return -v
	}
	return v
}

// wxPhen переводит группу текущей погоды METAR в явление.
// Явления «в окрестностях» (VC) не учитываются — их не было на станции.
func wxPhen(tok string) *Phen {
	m := reWx.FindStringSubmatch(tok)
	if m == nil || (m[2] == "" && m[3] == "") {
		return nil
	}
	if m[1] == "VC" {
		return nil
	}
	inten, desc, ph := m[1], m[2], m[3]
	lvl := 3
	if inten == "-" {
		lvl = 2
	} else if inten == "+" {
		lvl = 4
	}
	adj := func(weak, mid, strong string) string {
		switch lvl {
		case 2:
			return weak
		case 4:
			return strong
		}
		return mid
	}
	switch {
	case desc == "TS":
		if strings.Contains(ph, "GR") || strings.Contains(ph, "GS") {
			return &Phen{"thunder", "гроза с градом", 8}
		}
		if ph != "" {
			return &Phen{"thunder", "гроза с дождём", 7}
		}
		return &Phen{"thunder", "гроза", 7}
	case ph == "FG":
		if desc == "BC" || desc == "PR" || desc == "MI" {
			return &Phen{"fog", "туман местами", 5}
		}
		return &Phen{"fog", "туман", 6}
	case ph == "BR":
		return &Phen{"mist", "дымка", 1}
	case ph == "HZ" || ph == "FU":
		return &Phen{"mist", "мгла", 1}
	case strings.Contains(ph, "RA") && strings.Contains(ph, "SN"):
		return &Phen{"sleet", adj("небольшой мокрый снег", "мокрый снег", "сильный мокрый снег"), lvl + 1}
	case desc == "FZ" && (strings.Contains(ph, "RA") || strings.Contains(ph, "DZ")):
		return &Phen{"rain", "переохлаждённый дождь", 5}
	case desc == "SH" && strings.Contains(ph, "SN"):
		return &Phen{"snow", adj("небольшой ливневый снег", "ливневый снег", "сильный ливневый снег"), lvl}
	case desc == "SH":
		return &Phen{"showers", adj("небольшой ливневый дождь", "ливневый дождь", "сильный ливень"), lvl}
	case strings.HasPrefix(ph, "DZ"):
		return &Phen{"drizzle", adj("слабая морось", "морось", "сильная морось"), lvl - 1}
	case strings.HasPrefix(ph, "RA"):
		return &Phen{"rain", adj("небольшой дождь", "дождь", "сильный дождь"), lvl}
	case strings.HasPrefix(ph, "SN") || strings.HasPrefix(ph, "SG"):
		return &Phen{"snow", adj("небольшой снег", "снег", "сильный снег"), lvl}
	case strings.Contains(ph, "GR") || strings.Contains(ph, "GS") || strings.Contains(ph, "PL"):
		return &Phen{"snow", "град", 5}
	case ph == "SQ":
		return &Phen{"squall", "шквал", 6}
	}
	return nil
}
