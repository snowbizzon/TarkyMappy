package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fmtPrice — формат цены: <1000 как есть, тысячи целиком, миллионы с 0.1.
func TestFmtPrice(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"}, {743, "743"}, {999, "999"},
		{1000, "1k"}, {49499, "49k"}, {49500, "50k"}, {999999, "1000k"},
		{1000000, "1m"}, {1150000, "1.2m"}, {1200000, "1.2m"}, {1249999, "1.2m"},
		{1250000, "1.3m"}, {10000000, "10m"},
	}
	for _, c := range cases {
		if got := fmtPrice(c.in); got != c.want {
			t.Errorf("fmtPrice(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Лоадер: валидный файл читается, даты/режимы на месте.
func TestLoadPricesAt(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "prices.json")
	raw := `{
	 "updated": "2026-09-05T12:00:00+00:00",
	 "modes": {
	  "regular": [{"n":"Graphics card","w":2,"h":1,"p":850000,"pp":425000},
	              {"n":"Ledex","w":1,"h":1,"p":73000,"pp":73000}],
	  "pve":     [{"n":"Bitcoin","w":1,"h":1,"p":500000,"pp":500000}]
	 }
	}`
	if err := os.WriteFile(path, []byte(raw), 0o666); err != nil {
		t.Fatal(err)
	}
	md.loadPricesAt(path)

	items, updated := md.Prices("regular")
	if len(items) != 2 || updated == "" {
		t.Fatalf("regular: %+v, updated=%q", items, updated)
	}
	if items[0].N != "Graphics card" || items[0].PP != 425000 {
		t.Fatalf("regular[0]: %+v", items[0])
	}
	if pve, _ := md.Prices("pve"); len(pve) != 1 || pve[0].N != "Bitcoin" {
		t.Fatalf("pve: %+v", pve)
	}
}

// Файла нет — не ошибка: остаётся пустой список (панель покажет подсказку).
func TestLoadPricesMissingFile(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	md.loadPricesAt(filepath.Join(t.TempDir(), "net", "prices.json"))
	items, updated := md.Prices("regular")
	if len(items) != 0 {
		t.Fatalf("want empty, got %+v", items)
	}
	if updated != "" {
		t.Fatalf("want empty updated, got %q", updated)
	}
}

// Битый JSON — не ошибка: данные не портятся (остаются пустыми).
func TestLoadPricesBrokenJSON(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	path := filepath.Join(t.TempDir(), "prices.json")
	if err := os.WriteFile(path, []byte("{не json"), 0o666); err != nil {
		t.Fatal(err)
	}
	md.loadPricesAt(path)
	if items, _ := md.Prices("regular"); len(items) != 0 {
		t.Fatalf("want empty after broken json, got %+v", items)
	}
}

// Разбор типового снимка: оба режима, поля на месте.
func TestPriceDataUnmarshal(t *testing.T) {
	raw := `{
	 "updated": "2026-09-05T12:00:00+00:00",
	 "modes": {
	  "regular": [{"n":"Graphics card","w":2,"h":1,"p":850000,"pp":425000}],
	  "pve":     [{"n":"Ledex","w":1,"h":1,"p":73000,"pp":73000}]
	 }
	}`
	var pd priceData
	if err := json.Unmarshal([]byte(raw), &pd); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pd.Updated == "" || len(pd.Modes) != 2 {
		t.Fatalf("bad priceData: %+v", pd)
	}
	r := pd.Modes["regular"]
	if len(r) != 1 || r[0].N != "Graphics card" || r[0].W != 2 || r[0].PP != 425000 {
		t.Fatalf("bad regular: %+v", r)
	}
}

// Prices() возвращает копию: изменение среза не портит данные в md.
func TestPricesReturnsCopy(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatalf("NewMapData: %v", err)
	}
	a, _ := md.Prices("regular")
	a = append(a, PriceItem{N: "fake"})
	b, _ := md.Prices("regular")
	for _, it := range b {
		if it.N == "fake" {
			t.Fatal("Prices: срез не копия, изменения протекли в md")
		}
	}
}

func TestShortDate(t *testing.T) {
	if got := shortDate("2026-09-05T12:34:56+00:00"); got != "05.09.2026" {
		t.Errorf("shortDate = %q", got)
	}
	if got := shortDate("x"); got != "x" {
		t.Errorf("shortDate(short) = %q", got)
	}
}
