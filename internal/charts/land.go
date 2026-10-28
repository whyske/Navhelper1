package charts

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync"
)

// land.bin.gz — офлайн-тайлы суши (1°×1°) для превью карты (см. v3_NOTES.md).
// Формат (NHLD): заголовок "NHLD" + версия(u8) + ntiles(u32), затем индекс записей
// (x int16, y int16, off uint32, len uint32, flag uint8), затем блок данных.
// flag: 0 — тайл целиком вода (нет записи в данных), 1 — тайл целиком суша,
// 2 — смешанный тайл, кодирует кольца полигонов в локальных координатах
// 1/4096° от юго-западного угла тайла, varint zigzag по дельтам точек.
//
//go:embed land.bin.gz
var landGz []byte

// LandTile — один тайл 1°×1°, Flag=2 — полигоны суши в координатах [-0.5,0.5]
// (доля тайла, не градусы), готовые для прямого рисования.
type LandTile struct {
	Flag  uint8 // 0 вода, 1 суша целиком, 2 полигоны
	Rings [][][2]float64
}

// Ring — замкнутый контур в географических координатах [долгота, широта],
// используется для отрисовки суши и береговой линии в preview.go.
type Ring = [][2]float64

// LandIn возвращает кольца суши, попадающие в видовое окно
// [lonMin,latMin]–[lonMax,latMax] (в градусах), в абсолютных географических
// координатах — готовые для проекции в пикселях превью.
func LandIn(lonMin, latMin, lonMax, latMax float64) ([]Ring, error) {
	tiles, err := LandTiles()
	if err != nil {
		return nil, err
	}
	x0, x1 := int(math.Floor(lonMin)), int(math.Floor(lonMax))
	y0, y1 := int(math.Floor(latMin)), int(math.Floor(latMax))
	var out []Ring
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			// Нормализуем долготу в диапазон [-180,180), как в тайлах atlas.
			tx := ((x+180)%360+360)%360 - 180
			t, ok := tiles[[2]int16{int16(tx), int16(y)}]
			if !ok {
				continue
			}
			switch t.Flag {
			case 1:
				// Тайл целиком суша — рисуем его как квадрат.
				out = append(out, Ring{
					{float64(x), float64(y)}, {float64(x) + 1, float64(y)},
					{float64(x) + 1, float64(y) + 1}, {float64(x), float64(y) + 1},
				})
			case 2:
				for _, r := range t.Rings {
					ring := make(Ring, len(r))
					for i, p := range r {
						ring[i] = [2]float64{float64(x) + p[0], float64(y) + p[1]}
					}
					out = append(out, ring)
				}
			}
		}
	}
	return out, nil
}

var (
	landOnce  sync.Once
	landTiles map[[2]int16]LandTile
	landErr   error
)

// LandTiles распаковывает и разбирает встроенный атлас суши (однократно).
func LandTiles() (map[[2]int16]LandTile, error) {
	landOnce.Do(func() {
		landTiles, landErr = decodeLand(landGz)
	})
	return landTiles, landErr
}

// PreloadLand заранее распаковывает атлас суши в фоне при старте приложения,
// чтобы первое открытие превью карты не ждало разбора ~20 МБ данных.
func PreloadLand() error {
	_, err := LandTiles()
	return err
}

func decodeLand(gz []byte) (map[[2]int16]LandTile, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if len(raw) < 9 || string(raw[:4]) != "NHLD" {
		return nil, fmt.Errorf("land.bin: неверная сигнатура")
	}
	n := binary.LittleEndian.Uint32(raw[5:9])
	pos := 9
	type idxEnt struct {
		x, y int16
		off  uint32
		ln   uint32
		fl   uint8
	}
	idx := make([]idxEnt, n)
	for i := range idx {
		if pos+13 > len(raw) {
			return nil, fmt.Errorf("land.bin: обрезан индекс")
		}
		x := int16(binary.LittleEndian.Uint16(raw[pos:]))
		y := int16(binary.LittleEndian.Uint16(raw[pos+2:]))
		off := binary.LittleEndian.Uint32(raw[pos+4:])
		ln := binary.LittleEndian.Uint32(raw[pos+8:])
		fl := raw[pos+12]
		idx[i] = idxEnt{x, y, off, ln, fl}
		pos += 13
	}
	data := raw[pos:]
	out := make(map[[2]int16]LandTile, n)
	for _, e := range idx {
		t := LandTile{Flag: e.fl}
		if e.fl == 2 {
			if int(e.off+e.ln) > len(data) {
				return nil, fmt.Errorf("land.bin: обрезаны данные тайла")
			}
			t.Rings = decodeRings(data[e.off : e.off+e.ln])
		}
		out[[2]int16{e.x, e.y}] = t
	}
	return out, nil
}

// decodeRings разбирает кольца одного тайла: координаты возвращаются как
// доля тайла [0,1]×[0,1] от юго-западного угла (удобно умножать на размер
// тайла в градусах и складывать с координатами угла).
func decodeRings(b []byte) [][][2]float64 {
	const q = 4096.0
	pos := 0
	readVarint := func() int64 {
		var shift uint
		var v uint64
		for {
			if pos >= len(b) {
				return 0
			}
			c := b[pos]
			pos++
			v |= uint64(c&0x7f) << shift
			if c < 0x80 {
				break
			}
			shift += 7
		}
		return int64(v)
	}
	unzig := func(v int64) int64 { return (v >> 1) ^ -(v & 1) }

	if pos+2 > len(b) {
		return nil
	}
	nRings := int(binary.LittleEndian.Uint16(b[pos:]))
	pos += 2
	rings := make([][][2]float64, 0, nRings)
	for r := 0; r < nRings; r++ {
		if pos+2 > len(b) {
			break
		}
		npts := int(binary.LittleEndian.Uint16(b[pos:]))
		pos += 2
		pts := make([][2]float64, 0, npts)
		var px, py int64
		for i := 0; i < npts; i++ {
			dx := unzig(readVarint())
			dy := unzig(readVarint())
			px += dx
			py += dy
			pts = append(pts, [2]float64{float64(px) / q, float64(py) / q})
		}
		rings = append(rings, pts)
	}
	return rings
}
