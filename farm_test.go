package main

import (
	"math"
	"slices"
	"testing"
)

func TestFarmCatIndex(t *testing.T) {
	for i, c := range farmCats {
		if got := farmCatIndex(c.Code); got != i {
			t.Fatalf("farmCatIndex(%q) = %d, want %d", c.Code, got, i)
		}
	}
	if got := farmCatIndex("x"); got != -1 {
		t.Fatalf("farmCatIndex(\"x\") = %d, want -1", got)
	}
}

func TestClusterFarmSpots(t *testing.T) {
	mk := func(x, z float64) FarmSpot { return FarmSpot{C: "s", N: "safe", X: x, Y: 2, Z: z} }
	spots := []FarmSpot{
		mk(0, 0), mk(10, 5), mk(5, 12), // кластер 1 (цепочка)
		mk(200, 200), mk(210, 195), // кластер 2
		mk(1000, 1000), // одиночка
	}
	cls := clusterFarmSpots(spots)
	if len(cls) != 3 {
		t.Fatalf("clusters = %d, want 3", len(cls))
	}
	if cls[0].Total != 3 || cls[1].Total != 2 || cls[2].Total != 1 {
		t.Fatalf("totals = %d,%d,%d, want 3,2,1", cls[0].Total, cls[1].Total, cls[2].Total)
	}
	if cls[0].Items["safe"] != 3 {
		t.Fatalf("items = %v, want safe x3", cls[0].Items)
	}
	// Детерминизм: два прогона дают идентичные кластеры.
	again := clusterFarmSpots(spots)
	for i := range again {
		if again[i].X != cls[i].X || again[i].Z != cls[i].Z || again[i].Total != cls[i].Total {
			t.Fatalf("nondeterministic clustering at %d", i)
		}
	}
	if len(cls[0].Hull) == 0 {
		t.Fatal("empty hull")
	}
}

func TestConvexHull(t *testing.T) {
	xs := []float64{0, 10, 10, 0, 5, 5}
	zs := []float64{0, 0, 10, 10, 5, 1} // квадрат + центр + внутренняя
	hull := convexHull(xs, zs)
	if len(hull) != 4 {
		t.Fatalf("hull len = %d, want 4", len(hull))
	}
	for _, p := range hull {
		if p[0] == 5 && p[1] == 5 {
			t.Fatal("inner point in hull")
		}
	}
	// Площадь квадрата 10x10: Shoelace по порядку обхода (без пересечений).
	area := 0.0
	for i := range hull {
		a, b := hull[i], hull[(i+1)%len(hull)]
		area += a[0]*b[1] - b[0]*a[1]
	}
	if math.Abs(math.Abs(area)/2-100) > 1e-9 {
		t.Fatalf("area = %v, want 100", area/2)
	}
}

func TestOrderFarmStops(t *testing.T) {
	// Все на одном этаже: порядок — ближайший сосед.
	stops := []FarmStop{
		{X: 100, Z: 0}, {X: 200, Z: 0}, {X: 300, Z: 0},
	}
	rank0 := func(float64) int { return 0 }
	order := orderFarmStops([3]float64{0, 0, 0}, stops, rank0)
	if !slices.Equal(order, []int{0, 1, 2}) {
		t.Fatalf("order = %v, want [0 1 2]", order)
	}
	// Смена этажа дороже пути: ближняя точка на чужом этаже пропускается.
	floored := []FarmStop{
		{X: 50, Y: 10}, // этаж 1 (+120 штрафа)
		{X: 120, Y: 0}, // этаж 0
		{X: 140, Y: 0}, // этаж 0
	}
	rank := func(y float64) int {
		if y > 5 {
			return 1
		}
		return 0
	}
	order = orderFarmStops([3]float64{0, 0, 0}, floored, rank)
	if !slices.Equal(order, []int{1, 2, 0}) {
		t.Fatalf("order = %v, want [1 2 0] (floor penalty)", order)
	}
}

func TestFarmPickExit(t *testing.T) {
	cands := []Exit{
		{Name: "Near", X: 0, Z: 0},
		{Name: "Far", X: 1000, Z: 0},
	}
	// Автовыбор — самый дальний выход от игрока (конец маршрута).
	if e := farmPickExit(cands, 100, 0); e.Name != "Far" {
		t.Fatalf("pick = %s, want Far", e.Name)
	}
	if e := farmPickExit(cands, 1200, 0); e.Name != "Near" {
		t.Fatalf("pick = %s, want Near (противоположный край)", e.Name)
	}
}

func TestFarmSector(t *testing.T) {
	b := [2][2]float64{{0, 0}, {800, 800}}
	if got := farmSector(b, 50, 50); got != "A1" {
		t.Fatalf("sector = %s, want A1", got)
	}
	if got := farmSector(b, 750, 750); got != "H8" {
		t.Fatalf("sector = %s, want H8", got)
	}
	if got := farmSector(b, -100, 900); got != "A8" { // клампы краёв
		t.Fatalf("sector = %s, want A8", got)
	}
}

func TestExcludeFarmVisited(t *testing.T) {
	spots := []FarmSpot{
		{C: "s", X: 0, Z: 0},
		{C: "s", X: 10, Z: 0},  // рядом с посещённой
		{C: "s", X: 500, Z: 0}, // далеко
	}
	visited := []FarmStop{{X: 5, Z: 0, Visited: true}}
	kept := excludeFarmVisited(spots, visited)
	if len(kept) != 1 || kept[0].X != 500 {
		t.Fatalf("kept = %v, want только дальнюю точку", kept)
	}
	if got := excludeFarmVisited(spots, nil); len(got) != 3 {
		t.Fatalf("без посещённых — все точки, got %d", len(got))
	}
}

func TestFarmStopKeys(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	const key = "customs" // многоэтажная: Ground + First + Underground
	locks := []Lock{
		{Key: "A", X: 10, Z: 10, Y: 0},  // наземный — без тега
		{Key: "B", X: 10, Z: 10, Y: 5},  // дубль-точка, но другой ключ: First_Floor -> [F1]
		{Key: "C", X: 20, Z: 20, Y: -2}, // Underground -> [U]
		{Key: "Far", X: 1000, Z: 1000},
	}
	keys := farmStopKeys(md, key, locks, 0, 0)
	want := []string{"A", "B [F1]", "C [U]"}
	if !slices.Equal(keys, want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	// Одноэтажная карта — тегов этажа нет.
	keys = farmStopKeys(md, "factory", []Lock{{Key: "A", X: 1, Z: 1, Y: 5}}, 0, 0)
	if !slices.Equal(keys, []string{"A"}) {
		t.Fatalf("factory keys = %v, want [A]", keys)
	}
}

// Ориентиры фарма не должны содержать строк квестов — только выходы и ключи.
func TestFarmAnchorsNoQuests(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{md: md}
	for _, key := range md.names {
		quest := map[string]bool{}
		for _, q := range md.QuestsFor(key) {
			for i := range q.Points {
				if p := &q.Points[i]; p.Kind == "visit" && p.Label != "" {
					quest[md.Loc(p.Label)] = true
				}
			}
		}
		for _, a := range g.farmAnchors(key) {
			if quest[a.name] {
				t.Errorf("%s: ориентир фарма %q взят из квеста", key, a.name)
			}
		}
	}
}

func TestShortLandmark(t *testing.T) {
	cases := map[string]string{
		"Ключ от комнаты 114":      "Комната 114",
		"Ключ от времянки":         "Времянки",
		"Ключ-карта откладной нож": "Ключ-карта откладной нож",
		"Crossroads": "Crossroads",
	}
	for in, want := range cases {
		if got := shortLandmark(in); got != want {
			t.Errorf("shortLandmark(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFarmDataBounds(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	maps := 0
	for _, key := range md.Names() {
		spawns, spots := md.FarmFor(key)
		if len(spots) == 0 {
			continue
		}
		maps++
		cfg, ok := md.Cfg(key)
		if !ok {
			t.Fatalf("карта %s без конфига", key)
		}
		for _, s := range spots {
			if farmCatIndex(s.C) < 0 {
				t.Fatalf("%s: неизвестная категория %q", key, s.C)
			}
			if !inBounds(cfg.SVG.Bounds, s.X, s.Z, 0.15) {
				t.Fatalf("%s: точка %v вне границ карты", key, s)
			}
		}
		for _, p := range spawns {
			if !inBounds(cfg.SVG.Bounds, p.X, p.Z, 0.15) {
				t.Fatalf("%s: спавн %v вне границ карты", key, p)
			}
		}
	}
	if maps < 10 {
		t.Fatalf("карт с данными фарма = %d, want >= 10", maps)
	}
}

// TestFarmGenerateIntegration — сквозной прогон конвейера фарма без GUI:
// генерация от позиции, автовыход (самый дальний), автоотметка посещения,
// регенерация с исключением посещённого, цикл выбора выхода.
func TestFarmGenerateIntegration(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	const key = "customs"
	spawns, _ := md.FarmFor(key)
	if len(spawns) == 0 {
		t.Fatal("нет данных фарма для customs")
	}
	st := NewState()
	g := NewGame(st, md, DefaultConfig(), "", nil, nil)
	md.SetWanted(key)
	st.SetPosition(spawns[0].X, spawns[0].Y, spawns[0].Z, 0)

	if msg := g.farmGenerate(false); g.farmRoute == nil {
		t.Fatalf("маршрут не построен: %s", msg)
	}
	r := g.farmRoute
	if r.MapKey != key || !r.ExitAuto {
		t.Fatalf("route = %s auto=%v", r.MapKey, r.ExitAuto)
	}
	if len(r.Stops) == 0 || len(r.Stops) > farmStopsN(g.cfg.FarmStops) {
		t.Fatalf("stops = %d, want 1..%d", len(r.Stops), farmStopsN(g.cfg.FarmStops))
	}
	// Автовыход — самый дальний доступный от старта.
	ex, _ := md.ExitsFor(key)
	var vis []Exit
	for _, e := range ex {
		if g.exitVisible(e.Faction) {
			vis = append(vis, e)
		}
	}
	if want := farmPickExit(vis, r.Origin[0], r.Origin[2]); want.Name != r.Exit.Name {
		t.Fatalf("exit = %s, want %s", r.Exit.Name, want.Name)
	}
	// Путь: первая точка ближе к старту, чем последняя (жадный NN).
	d0 := math.Hypot(r.Stops[0].X-r.Origin[0], r.Stops[0].Z-r.Origin[2])
	dl := math.Hypot(r.Stops[len(r.Stops)-1].X-r.Origin[0], r.Stops[len(r.Stops)-1].Z-r.Origin[2])
	if dl < d0-farmClusterR {
		t.Fatalf("последняя точка (%.0fm) ближе первой (%.0fm) — маршрут не от старта", dl, d0)
	}
	// Автоотметка посещения: телепорт к точке 1.
	before := r.Stops[0]
	st.SetPosition(before.X, before.Y, before.Z, 0)
	g.farmUpdate()
	if !r.Stops[0].Visited {
		t.Fatal("точка 1 не помечена посещённой")
	}
	// «Novyi s etoy tochki»: посещённый кластер исключён.
	g.farmGenerate(true)
	r2 := g.farmRoute
	if r2 == nil {
		t.Fatal("регенерация потеряла маршрут")
	}
	for _, s := range r2.Stops {
		if math.Hypot(s.X-before.X, s.Z-before.Z) <= farmClusterR {
			t.Fatalf("посещённый кластер (%.0f,%.0f) не исключён", before.X, before.Z)
		}
	}
	// Цикл выхода: avto -> следующий ЗА автовыбранным — клик сразу
	// даёт видимую смену выхода.
	autoName := r2.Exit.Name
	g.farmCycleExit()
	if r2.ExitAuto || r2.Exit.Name == autoName {
		t.Fatalf("цикл: %s auto=%v — ожидался другой выход", r2.Exit.Name, r2.ExitAuto)
	}
	// Регенерация сохраняет ручной выбор выхода.
	g.farmGenerate(true)
	if g.farmRoute.ExitAuto || g.farmRoute.Exit.Name != r2.Exit.Name {
		t.Fatalf("ручной выход потерян: %s auto=%v", g.farmRoute.Exit.Name, g.farmRoute.ExitAuto)
	}
	// Добор цикла до возврата в avto (стартуем с индекса k — кликов
	// len(cands)-k, поэтому крутим до факта, не по счётчику).
	for i := 0; i <= len(vis); i++ {
		if g.farmRoute.ExitAuto {
			break
		}
		g.farmCycleExit()
	}
	if !g.farmRoute.ExitAuto {
		t.Fatal("цикл: выход не вернулся в avto")
	}
}

func TestFarmStopsClamp(t *testing.T) {
	for _, c := range []struct{ in, want int }{
		{0, 5}, {4, 5}, {5, 5}, {12, 12}, {35, 35}, {36, 35}, {100, 35},
	} {
		if got := farmStopsN(c.in); got != c.want {
			t.Fatalf("farmStopsN(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestEdgePoint(t *testing.T) {
	w, h, m := 400, 300, 26.0
	// Внутри окна (в т.ч. в пределах отступа) — стрелка не нужна.
	if _, _, out := edgePoint(w, h, 200, 150, m); out {
		t.Fatal("центр окна определён как внеэкранный")
	}
	if _, _, out := edgePoint(w, h, 210, 160, m); out {
		t.Fatal("точка в пределах отступа определена как внеэкранная")
	}
	// Справа далеко — точка на правой границе с отступом (w-26=374).
	ex, ey, out := edgePoint(w, h, 2000, 150, m)
	if !out {
		t.Fatal("далёкая цель не определена как внеэкранная")
	}
	if math.Abs(ex-374) > 1 || math.Abs(ey-150) > 1 {
		t.Fatalf("edge = (%.1f, %.1f), want (374, 150)", ex, ey)
	}
	// Диагональ — упирается в нижнюю границу (h-26=274), x раньше края.
	ex, ey, out = edgePoint(w, h, 2000, 1500, m)
	if !out || math.Abs(ey-274) > 1 || ex > 375 {
		t.Fatalf("edge = (%.1f, %.1f, %v), want y=274", ex, ey, out)
	}
}

func TestFarmGroundFilter(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, key := range md.Names() {
		_, spots := md.FarmFor(key)
		if len(spots) == 0 || len(md.Floors(key)) <= 1 {
			continue
		}
		found = true
		ground := farmGroundSpots(md, key, spots)
		if len(ground) == 0 {
			t.Fatalf("%s: фильтр земли опустошил данные", key)
		}
		def := md.DefaultFloor(key)
		for _, s := range ground {
			if f := md.FloorFor(key, s.Y, ""); f != def {
				t.Fatalf("%s: точка y=%.1f на этаже %q, want %q", key, s.Y, f, def)
			}
		}
	}
	if !found {
		t.Fatal("нет многоэтажных карт с данными фарма")
	}
}
