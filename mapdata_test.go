package main

import (
	"image/color"
	"math"
	"testing"
)

func TestMapDataDetectKnownSpawns(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		key  string
		x, z float64
	}{
		{"customs", 569, -55},         // спавн из каталога tarkov.dev
		{"factory", -45, 6},           // спавн
		{"lab", -118, -403},           // спавн
		{"lighthouse", 6.27, -869.48}, // Armored Train: реальные координаты Маяка
		// (спавн Улиц 270,381 больше не «накрывается» bbox Маяка — границы
		// откалиброваны по арту reemr и честно его не содержат)
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			key, ok := md.Detect(tc.x, tc.z, "")
			if !ok || key != tc.key {
				t.Fatalf("Detect(%f, %f) = %q (ok=%v), want %q", tc.x, tc.z, key, ok, tc.key)
			}
		})
	}
	if key, ok := md.Detect(1e9, 1e9, ""); ok {
		t.Fatalf("far point detected as %q", key)
	}
}

// Реальный кейс «меня запихало в угол»: точка на северо-западе таможни
// попадает в bounds семи карт; без истории скоринг margin²/area даёт reserve
// (его bbox накрывает точку «лучше») — выручают lastMap и гистерезис.
// С гистерезисом (current=customs) карта должна оставаться таможней.
func TestDetectHysteresis(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	const garagesX, garagesZ = -23.66, -135.16 // гаражи / мост, таможня
	if key, ok := md.Detect(garagesX, garagesZ, ""); !ok || key != "reserve" {
		t.Fatalf("bez istorii: Detect = %q (ok=%v), want reserve", key, ok)
	}
	for range 50 {
		if key, ok := md.Detect(garagesX, garagesZ, "customs"); !ok || key != "customs" {
			t.Fatalf("s istoriei: Detect = %q (ok=%v), want customs", key, ok)
		}
	}
	// Точка чуть за границей текущей карты — всё ещё она (допуск 10%).
	if key, ok := md.Detect(730, -100, "customs"); !ok || key != "customs" {
		t.Fatalf("kraj: Detect = %q (ok=%v), want customs", key, ok)
	}
	// Явный выход далеко за границы — смена карты.
	if key, ok := md.Detect(569, -55, "factory"); !ok || key == "factory" {
		t.Fatalf("smena karty: Detect = %q (ok=%v), want ne factory", key, ok)
	}
}

// Точки из живого лога юзера: старт с lastMap=customs — гистерезис
// должен держать таможню на всех реальных позициях рейда.
func TestUserLogPointsStayCustoms(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	pts := [][2]float64{
		{-114, 4.9}, {388.36, -114.48}, {283, -70}, {196.36, -114.9},
		{92.83, -132.18}, {83.02, -137.01}, {-23.66, -135.16}, {569, -55},
	}
	for _, p := range pts {
		if key, ok := md.Detect(p[0], p[1], "customs"); !ok || key != "customs" {
			t.Fatalf("Detect(%v, %v) = %q (ok=%v), want customs", p[0], p[1], key, ok)
		}
	}
}

// Все выходы из exits.json должны лежать в границах своей карты
// (допуск 10% — новые флейр-выходы 2026 физически чуть за старым артом,
// например Interchange «Path to River (Flare)»; фильтр данных в
// update_data.ps1 использует тот же допуск 10%).
func TestExitsAreInsideMapBounds(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range md.names {
		cfg := md.maps[key]
		b := cfg.SVG.Bounds
		extracts, transits := md.ExitsFor(key)
		for _, group := range [][]Exit{extracts, transits} {
			for _, e := range group {
				u, v := WorldToUV(b, e.X, e.Z)
				if u < -0.10 || u > 1.10 || v < -0.10 || v > 1.10 {
					t.Errorf("%s: exit %q uv=(%.3f, %.3f) вне границ", key, e.Name, u, v)
				}
			}
		}
	}
}

// У каждой карты из maps.json должен быть блок выходов и корректный PNG.
func TestAllMapsHaveExitsAndPNG(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	if len(md.names) != 10 {
		t.Fatalf("карт в maps.json: %d, ожидалось 10", len(md.names))
	}
	for _, key := range md.names {
		extracts, _ := md.ExitsFor(key)
		if len(extracts) == 0 {
			t.Errorf("%s: нет выходов", key)
		}
		img, err := decodeMapPNG(key, md.DefaultFloor(key))
		if err != nil {
			t.Errorf("%s: %v", key, err)
			continue
		}
		if img.Bounds().Dx() < 1024 || img.Bounds().Dy() < 512 {
			t.Errorf("%s: подозрительно маленькая текстура %v", key, img.Bounds())
		}
	}
}

// Этажи: автовыбор по Y (Reserve/Factory/Labs), гистерезис на границе,
// одноэтажные карты и combined-текстуры многоэтажных.
func TestFloors(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		key     string
		y       float64
		current string
		want    string
	}{
		{"reserve", -17, "", "Bunkers"},                   // D2
		{"reserve", -5, "", "Ground_Level"},               // наземные выходы
		{"reserve", -6.5, "Ground_Level", "Ground_Level"}, // гистерезис ±2 м
		{"reserve", -11, "Ground_Level", "Bunkers"},       // за гистерезисом — смена
		{"factory", -3.3, "", "Basement"},                 // Cellars
		{"factory", 0, "", "Ground_Floor"},
		{"factory", 8.7, "", "Second_Floor"}, // Office Window
		{"lab", -2, "", "Technical_Level"},
		{"lab", 1, "", "First_Level"},
		{"lab", 7, "", "Second_Level"},
		{"customs", -2, "", "Underground_Level"}, // подвалы таможни (эвристика)
		{"customs", 1, "", "Ground_Level"},       // SVG таможни многоэтажный
		{"customs", 5, "", "First_Floor"},
		{"factory", 1500, "", "Ground_Floor"}, // вне всех диапазонов — default
	}
	for _, tc := range cases {
		if got := md.FloorFor(tc.key, tc.y, tc.current); got != tc.want {
			t.Errorf("FloorFor(%s, %v, %q) = %q, want %q", tc.key, tc.y, tc.current, got, tc.want)
		}
	}
	// Все этажи и combined-рендеры многоэтажных карт должны быть в embed.
	for _, key := range md.names {
		cfg := md.maps[key]
		if len(cfg.SVG.Floors) <= 1 {
			continue
		}
		for _, f := range append(append([]string{}, cfg.SVG.Floors...), floorAll) {
			if _, err := decodeMapPNG(key, f); err != nil {
				t.Errorf("%s/%s: %v", key, f, err)
			}
		}
	}
	if _, err := decodeMapPNG("customs", "No_Such_Floor"); err == nil {
		t.Error("decodeMapPNG: несуществующий этаж должен давать ошибку")
	}
}

// Все опасности (мины/снайперы/ловушки) — в границах своей карты; на Маяке
// обязаны быть и мины (103 зоны), и клейморы моста (88, из арта reemr —).
func TestHazardsInsideBounds(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, key := range md.Names() {
		cfg := md.maps[key]
		b := cfg.SVG.Bounds
		for _, h := range md.HazardsFor(key) {
			u, v := WorldToUV(b, h.X, h.Z)
			if u < -0.05 || u > 1.05 || v < -0.05 || v > 1.05 {
				t.Errorf("%s: hazard %q (%v,%v) uv=(%.3f,%.3f) вне границ", key, h.T, h.X, h.Z, u, v)
			}
			switch h.T {
			case "m", "s", "c", "h":
			default:
				t.Errorf("%s: неизвестный тип опасности %q", key, h.T)
			}
			total++
		}
	}
	if total < 300 {
		t.Errorf("опасностей в hazards.json: %d, ожидалось >= 300", total)
	}
	lh := md.HazardsFor("lighthouse")
	var mines, clay int
	for _, h := range lh {
		switch h.T {
		case "m":
			mines++
		case "c":
			clay++
		}
	}
	if mines < 90 {
		t.Errorf("мин на Маяке: %d, ожидалось >= 90", mines)
	}
	if clay < 80 {
		t.Errorf("клейморов на Маяке: %d, ожидалось >= 80", clay)
	}
}

func TestWantedRoundtrip(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	if md.Wanted() != "" {
		t.Fatalf("initial wanted = %q", md.Wanted())
	}
	md.SetWanted("customs")
	if md.Wanted() != "customs" {
		t.Fatalf("wanted = %q", md.Wanted())
	}
}

// Палитра квестов: у двух квестов одной карты не бывает одинакового цвета,
// а цвета квестов не прилипают к служебным цветам выходов (hue-дистанция
// до scav/pmc/shared/transit >= 18°, RGB-дистанция >= 60, вкл. белый/стрелку).
func TestQuestPaletteUniqueAndDistinct(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	reserved := []color.RGBA{colScav, colPMC, colShared, colTransit, colArrow}
	hue := func(c color.RGBA) float64 {
		r, g, b := float64(c.R), float64(c.G), float64(c.B)
		mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
		if mx == mn {
			return -1 // оттенка нет (белый/серый)
		}
		d := mx - mn
		switch mx {
		case r:
			return math.Mod(60*math.Mod((g-b)/d, 6)+360, 360)
		case g:
			return math.Mod(60*((b-r)/d+2)+360, 360)
		default:
			return math.Mod(60*((r-g)/d+4)+360, 360)
		}
	}
	hueDist := func(a, b float64) float64 {
		if a < 0 || b < 0 {
			return 360
		}
		d := math.Abs(a - b)
		if d > 180 {
			d = 360 - d
		}
		return d
	}
	rgbDist2 := func(a, b color.RGBA) float64 {
		dr := float64(a.R) - float64(b.R) // каналы uint8: вычитать после каста
		dg := float64(a.G) - float64(b.G)
		db := float64(a.B) - float64(b.B)
		return dr*dr + dg*dg + db*db
	}
	// Сама палитра (3 круга яркости = 45 цветов) против служебных цветов:
	// RGB-дистанция >= 60 и hue >= 18° для всех кругов.
	for i := range len(questPaletteHues) * len(questColorV) {
		c := questColorFor(i)
		for _, rc := range reserved {
			if rgbDist2(c, rc) < 60*60 {
				t.Fatalf("questColorFor(%d) %v blizok k sluzhebnomu %v", i, c, rc)
			}
			if d := hueDist(hue(c), hue(rc)); d < 18 {
				t.Fatalf("questColorFor(%d) hue %.0f blizok k %v (hue %.0f, dist %.0f)", i, hue(c), rc, hue(rc), d)
			}
		}
	}
	// Пары одного круга яркости различаются оттенком >= 14° (RGB в секторе
	// сжимается к границам сегментов, hue — честный инвариант); разные круги
	// разделены яркостью (>= 0.22 V).
	for i := range len(questPaletteHues) * len(questColorV) {
		vi := questColorV[(i/len(questPaletteHues))%len(questColorV)]
		for j := range i {
			vj := questColorV[(j/len(questPaletteHues))%len(questColorV)]
			if math.Abs(vi-vj) >= 0.2 {
				continue
			}
			if d := hueDist(hue(questColorFor(i)), hue(questColorFor(j))); d < 13 { // 14° замысла минус квантизация uint8
				t.Fatalf("questColorFor(%d) i (%d) odnogo kruga s hue-dist %.2f (< 13)", i, j, d)
			}
		}
	}
	// Реальные данные: уникальность цветов внутри каждой карты.
	for _, key := range md.Names() {
		seen := map[color.RGBA]string{}
		for _, q := range md.QuestsFor(key) {
			c := md.QuestColor(key, q.ID)
			if prev, dup := seen[c]; dup {
				t.Fatalf("%s: %q и %q одного цвета %v", key, prev, q.Name, c)
			}
			seen[c] = q.Name
		}
	}
}

// TestLocFallback — RU-подстановка игровых строк: известная строка из
// loc_ru.json переводится, неизвестная остаётся EN, в en-локали словарь
// не применяется вовсе.
func TestLocFallback(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	defer setLang("ru")

	setLang("ru")
	if got := md.Loc("Crossroads"); got == "" || got == "Crossroads" {
		t.Errorf("Loc(Crossroads) ru = %q, want RU-perevod", got)
	}
	if got := md.Loc("medcase"); got != "Медкейс" {
		t.Errorf("Loc(medcase) ru = %q, want Медкейс", got)
	}
	if got := md.Loc("no.such.string"); got != "no.such.string" {
		t.Errorf("Loc(unknown) = %q, want ishodnuyu stroku", got)
	}
	if got := md.Loc(""); got != "" {
		t.Errorf("Loc(empty) = %q", got)
	}
	setLang("en")
	if got := md.Loc("Crossroads"); got != "Crossroads" {
		t.Errorf("Loc(Crossroads) en = %q, want EN", got)
	}
}

// TestMapNamesRU — каждая карта имеет непустое locale.ru: иначе
// RU-локаль покажет технический ключ (customs) вместо «Таможня».
func TestMapNamesRU(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range md.Names() {
		c, ok := md.Cfg(key)
		if !ok {
			t.Fatalf("Cfg(%q) missing", key)
		}
		if c.Locale.Ru == "" {
			t.Errorf("%q: pustoe locale.ru", key)
		}
		for _, r := range c.Locale.Ru {
			if r == 0xFFFD { // mojibake-маркер перекодировки
				t.Errorf("%q: locale.ru soderzhit bituyu runu U+FFFD", key)
			}
		}
	}
}

// TestGroundZeroRUName — official RU-имя карты (жалоба юзера: показывался
// «Приморск» вместо «Эпицентр»).
func TestGroundZeroRUName(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	c, ok := md.Cfg("groundzero")
	if !ok {
		t.Fatal("groundzero missing")
	}
	if got := c.Locale.Ru; got != "Эпицентр" {
		t.Errorf("groundzero locale.ru = %q, want Эпицентр", got)
	}
}

// TestExitsRUTranslated — все выходы/транзиты всех карт имеют RU-перевод
// в loc_ru.json (Loc(name) != name в локали ru).
func TestExitsRUTranslated(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	setLang("ru")
	defer setLang("en")
	for _, key := range md.names {
		extracts, transits := md.ExitsFor(key)
		for _, group := range [][]Exit{extracts, transits} {
			for _, e := range group {
				if got := md.Loc(e.Name); got == e.Name {
					t.Errorf("%s: exit %q bez RU-perevoda", key, e.Name)
				}
			}
		}
	}
}

// TestTradersHaveRU — каждый торговец из quests.json имеет RU-имя в словаре
// (жалоба: «Скир» вместо «Лыжник»).
func TestTradersHaveRU(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, key := range md.names {
		for _, q := range md.QuestsFor(key) {
			seen[q.Trader] = true
		}
	}
	for tr := range seen {
		ru, ok := tradersRU[tr]
		if !ok || ru == "" {
			t.Errorf("trader %q: net RU-imeni", tr)
		}
	}
	if !seen["Skier"] || tradersRU["Skier"] != "Лыжник" {
		t.Errorf("Skier RU = %q (found=%v)", tradersRU["Skier"], seen["Skier"])
	}
	if got := tradersRU["BTR Driver"]; got != "Водитель БТР" {
		t.Errorf("BTR Driver RU = %q", got)
	}
	// Официальная русская локализация: Fence = «Скупщик». «Барыга» — сленг,
	// в русской версии торговца с таким именем нет (жалоба юзера 2026-10-26).
	if got := tradersRU["Fence"]; got != "Скупщик" {
		t.Errorf("Fence RU = %q, want Скупщик", got)
	}
}
