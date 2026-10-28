package charts

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"sync"
)

// coast50.json.gz — береговая линия Natural Earth 1:50m (public domain),
// упрощённая до полилиний [долгота, широта]. Нужна только для превью листа.
//
//go:embed coast50.json.gz
var coastGz []byte

var (
	coastOnce sync.Once
	coastData [][][2]float64
	coastErr  error
)

// CoastLines возвращает полилинии береговой линии.
func CoastLines() ([][][2]float64, error) {
	coastOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(coastGz))
		if err != nil {
			coastErr = err
			return
		}
		defer zr.Close()
		raw, err := io.ReadAll(zr)
		if err != nil {
			coastErr = err
			return
		}
		coastErr = json.Unmarshal(raw, &coastData)
	})
	return coastData, coastErr
}
