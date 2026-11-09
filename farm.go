package main

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// --- Помощник фарма (F3): кластеры лута, маршрут к выходу, посещённость. ---

// FarmCat — категория лута (галочки панели F3, биты маски FarmCats).
type FarmCat struct {
	Code string // код в data/farm.json
	Name string // ключ локализации подписи (lang.go)
}

var farmCats = [...]FarmCat{
	{"t", "farm.cat.t"},
	{"m", "farm.cat.m"},
	{"s", "farm.cat.s"},
	{"h", "farm.cat.h"},
	{"w", "farm.cat.w"},
}

// farmCatBit — бит категории i в маске конфига.
func farmCatBit(i int) int { return 1 << i }

// farmCatIndex — индекс категории по коду (-1 — неизвестный код).
func farmCatIndex(c string) int {
	for i := range farmCats {
		if farmCats[i].Code == c {
			return i
		}
	}
	return -1
}

const (
	farmClusterR     = 35  // радиус слияния лут-точек в кластер (м)
	farmVisitR       = 30  // радиус автоотметки «посещено» (м)
	farmFloorPenalty = 120 // штраф смены этажа (м эквивалента пути)
	farmAnchorR      = 80  // радиус привязки имени-ориентира к кластеру (м)
	farmKeyR         = 60  // радиус поиска ключей рядом с кластером (м)
)

// farmStopsN — лимит точек маршрута из конфига (слайдер F3 «Punkty», 5..35).
func farmStopsN(n int) int {
	if n < 5 {
		return 5
	}
	if n > 35 {
		return 35
	}
	return n
}

// FarmCluster — скопление лут-точек: кандидат в точки маршрута.
type FarmCluster struct {
	X, Y, Z float64        // центроид
	Items   map[string]int // лут по именам (подпись маршрута)
	Total   int
	Hull    [][2]float64 // выпуклая оболочка [x,z] (контур на карте)
}

// FarmStop — точка маршрута (кластер + подписи + посещённость).
type FarmStop struct {
	X, Y, Z float64
	Floor   string // этаж (тег [G]/[F1]/[U] в маршруте)
	Hull    [][2]float64
	Items   map[string]int
	Total   int
	Name    string   // «sektor D4» + якорь (« — Obschagi»)
	Keys    []string // ключи с дверьми рядом
	Visited bool
	Dist    int // путь от предыдущей точки (м)
}

// FarmRoute — маршрут фарма: порядок точек + выбранный выход.
type FarmRoute struct {
	MapKey   string
	Floors   bool // строился с учётом всех этажей
	Origin   [3]float64
	Stops    []FarmStop
	Exit     Exit
	ExitAuto bool  // выход выбран автоматически (самый дальний)
	Ver      int64 // версия (посещённость/перестроения — для dirty-frame)
}

// clusterFarmSpots — кластеризация точек: объединение в радиусе
// farmClusterR (BFS по соседним ячейкам сетки). Детерминировано: обход по
// исходному порядку точек, сортировки по стабильным ключам.
func clusterFarmSpots(spots []FarmSpot) []FarmCluster {
	cell := float64(farmClusterR)
	grid := make(map[[2]int][]int, len(spots))
	for i := range spots {
		k := [2]int{int(math.Floor(spots[i].X / cell)), int(math.Floor(spots[i].Z / cell))}
		grid[k] = append(grid[k], i)
	}
	seen := make([]bool, len(spots))
	var out []FarmCluster
	for i := range spots {
		if seen[i] {
			continue
		}
		seen[i] = true
		queue := []int{i}
		members := make([]int, 0, 16)
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			members = append(members, cur)
			cx, cz := int(math.Floor(spots[cur].X/cell)), int(math.Floor(spots[cur].Z/cell))
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					for _, j := range grid[[2]int{cx + dx, cz + dz}] {
						if !seen[j] && math.Hypot(spots[cur].X-spots[j].X, spots[cur].Z-spots[j].Z) <= cell {
							seen[j] = true
							queue = append(queue, j)
						}
					}
				}
			}
		}
		cl := FarmCluster{
			Items: make(map[string]int, 4),
		}
		xs := make([]float64, 0, len(members))
		ys := make([]float64, 0, len(members))
		zs := make([]float64, 0, len(members))
		for _, j := range members {
			s := spots[j]
			if s.N != "" {
				cl.Items[s.N]++
			}
			cl.Total++
			xs = append(xs, s.X)
			ys = append(ys, s.Y)
			zs = append(zs, s.Z)
		}
		cl.X, cl.Y, cl.Z = avg(xs), avg(ys), avg(zs)
		cl.Hull = convexHull(xs, zs)
		out = append(out, cl)
	}
	return out
}

func avg(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	var s float64
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// convexHull — выпуклая оболочка (алгоритм Эндрю, обход против часовой).
func convexHull(xs, zs []float64) [][2]float64 {
	n := len(xs)
	if n == 0 {
		return nil
	}
	if n < 3 {
		h := make([][2]float64, n)
		for i := range h {
			h[i] = [2]float64{xs[i], zs[i]}
		}
		return h
	}
	pts := make([][2]float64, n)
	for i := range pts {
		pts[i] = [2]float64{xs[i], zs[i]}
	}
	slices.SortFunc(pts, func(a, b [2]float64) int {
		if a[0] != b[0] {
			return cmpF(a[0], b[0])
		}
		return cmpF(a[1], b[1])
	})
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	var lower, upper [][2]float64
	for _, p := range pts {
		for len(lower) >= 2 && cross(lower[len(lower)-2], lower[len(lower)-1], p) <= 0 {
			lower = lower[:len(lower)-1]
		}
		lower = append(lower, p)
	}
	for i := len(pts) - 1; i >= 0; i-- {
		p := pts[i]
		for len(upper) >= 2 && cross(upper[len(upper)-2], upper[len(upper)-1], p) <= 0 {
			upper = upper[:len(upper)-1]
		}
		upper = append(upper, p)
	}
	return append(lower[:len(lower)-1], upper[:len(upper)-1]...)
}

func cmpF(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// farmSector — сектор карты (сетка 8x8 поверх bounds): A1..H8.
func farmSector(b [2][2]float64, x, z float64) string {
	u, v := WorldToUV(b, x, z)
	return string("ABCDEFGH"[clampI(int(u*8), 0, 7)]) + strconv.Itoa(clampI(int(v*8), 0, 7)+1)
}

// farmAnchor — именованный ориентир (выход, дверь ключа, квестовая зона).
type farmAnchor struct {
	name string
	x, z float64
}

// farmAnchors — ориентиры карты для осмысленных подписей кластеров:
// выходы и двери-ключи. Квестовые метки НЕ используются — иначе в
// подсказки фарма попадают длинные строки целей квестов.
func (g *Game) farmAnchors(key string) []farmAnchor {
	var an []farmAnchor
	ex, _ := g.md.ExitsFor(key)
	for _, e := range ex {
		an = append(an, farmAnchor{g.md.Loc(e.Name), e.X, e.Z})
	}
	for _, l := range g.md.LocksFor(key) {
		an = append(an, farmAnchor{shortLandmark(g.md.Loc(l.Key)), l.X, l.Z})
	}
	return an
}

// shortLandmark — короткое имя ориентира из названия ключа:
// «Ключ от комнаты 114» -> «Комната 114», «Ключ от времянки» -> «Времянки».
func shortLandmark(name string) string {
	for _, pre := range []string{"Ключ от ", "Ключ "} {
		if rest, ok := strings.CutPrefix(name, pre); ok {
			name = rest
			break
		}
	}
	if name == "" {
		return name
	}
	// «комнаты 114» читается хуже — именительный падеж.
	for bad, good := range map[string]string{
		"комнаты ":   "Комната ",
		"квартиры ":  "Квартира ",
		"помещения ": "Помещение ",
		"кабинета ":  "Кабинет ",
	} {
		if rest, ok := strings.CutPrefix(name, bad); ok {
			return good + rest
		}
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// farmStopName — «сектор D4» + ближайший ориентир в радиусе farmAnchorR.
func farmStopName(b [2][2]float64, an []farmAnchor, x, z float64) string {
	name := T("farm.sector", farmSector(b, x, z))
	best, bestD := "", float64(farmAnchorR)
	for _, a := range an {
		if d := math.Hypot(a.x-x, a.z-z); d < bestD {
			best, bestD = a.name, d
		}
	}
	if best != "" {
		return name + " — " + best
	}
	return name
}

// farmStopKeys — ключи с дверьми в радиусе farmKeyR (макс 3, без дублей).
// Для многоэтажных карт добавляется тег этажа двери: «X [F2]».
func farmStopKeys(md *MapData, key string, locks []Lock, x, z float64) []string {
	multi := len(md.Floors(key)) > 1
	var keys []string
	for _, l := range locks {
		if math.Hypot(l.X-x, l.Z-z) > farmKeyR {
			continue
		}
		if slices.ContainsFunc(keys, func(s string) bool {
			return strings.HasPrefix(s, l.Key)
		}) {
			continue
		}
		name := l.Key
		if multi {
			if fl := md.FloorFor(key, l.Y, ""); fl != "" && fl != md.DefaultFloor(key) {
				name += " [" + floorShort(fl) + "]"
			}
		}
		keys = append(keys, name)
		if len(keys) == 3 {
			break
		}
	}
	return keys
}

// orderFarmStops — жадный «ближайший сосед» от origin; смена этажа стоит
// farmFloorPenalty метров. Возвращает порядок индексов stops.
func orderFarmStops(origin [3]float64, stops []FarmStop, floorRank func(float64) int) []int {
	left := make([]int, len(stops))
	for i := range left {
		left[i] = i
	}
	cur := origin
	curFloor := floorRank(origin[1])
	order := make([]int, 0, len(stops))
	for len(left) > 0 {
		best, bestD := -1, math.MaxFloat64
		for _, j := range left {
			d := math.Hypot(stops[j].X-cur[0], stops[j].Z-cur[2])
			if floorRank(stops[j].Y) != curFloor {
				d += farmFloorPenalty
			}
			if d < bestD {
				best, bestD = j, d
			}
		}
		if best < 0 {
			break
		}
		order = append(order, best)
		cur = [3]float64{stops[best].X, 0, stops[best].Z}
		curFloor = floorRank(stops[best].Y)
		nl := left[:0]
		for _, j := range left {
			if j != best {
				nl = append(nl, j)
			}
		}
		left = nl
	}
	return order
}

// farmPickExit — самый дальний выход от точки (автовыбор).
func farmPickExit(cands []Exit, fromX, fromZ float64) Exit {
	best, bestD := cands[0], -1.0
	for _, e := range cands {
		if d := math.Hypot(e.X-fromX, e.Z-fromZ); d > bestD {
			best, bestD = e, d
		}
	}
	return best
}

// farmGroundSpots — фильтр «только наземный этаж» (Etazhi выключены).
func farmGroundSpots(md *MapData, key string, spots []FarmSpot) []FarmSpot {
	def := md.DefaultFloor(key)
	kept := make([]FarmSpot, 0, len(spots))
	for _, s := range spots {
		if md.FloorFor(key, s.Y, "") == def {
			kept = append(kept, s)
		}
	}
	return kept
}

// excludeFarmVisited — убирает точки рядом с посещёнными остановками
// (кнопка «Novyi s etoy tochki»).
func excludeFarmVisited(spots []FarmSpot, visited []FarmStop) []FarmSpot {
	if len(visited) == 0 {
		return spots
	}
	kept := make([]FarmSpot, 0, len(spots))
	for _, s := range spots {
		near := false
		for _, o := range visited {
			if math.Hypot(o.X-s.X, o.Z-s.Z) <= farmClusterR {
				near = true
				break
			}
		}
		if !near {
			kept = append(kept, s)
		}
	}
	return kept
}

// farmGenerate — построить маршрут от текущей позиции до самого дальнего
// доступного выхода. keepOld=true («Novyi s etoy tochki») — кластеры рядом с
// посещёнными точками исключаются. Возвращает статус для панели.
func (g *Game) farmGenerate(keepOld bool) string {
	key := g.md.Wanted()
	x, y, z, _, ok := g.st.Pos()
	if !ok || key == "" {
		return T("farm.needpos")
	}
	cfg, found := g.md.Cfg(key)
	if !found {
		return T("hud.nomap")
	}
	_, spots := g.md.FarmFor(key)
	if len(spots) == 0 {
		return T("farm.nodata")
	}
	// Фильтр категорий.
	sel := make([]FarmSpot, 0, len(spots))
	for _, s := range spots {
		if i := farmCatIndex(s.C); i >= 0 && g.cfg.FarmCats&farmCatBit(i) != 0 {
			sel = append(sel, s)
		}
	}
	if len(sel) == 0 {
		return T("farm.nocat")
	}
	// Этажи: по умолчанию только наземный.
	if !g.cfg.FarmFloors {
		sel = farmGroundSpots(g.md, key, sel)
	}
	// Исключение посещённого.
	if keepOld && g.farmRoute != nil {
		var old []FarmStop
		for _, s := range g.farmRoute.Stops {
			if s.Visited {
				old = append(old, s)
			}
		}
		sel = excludeFarmVisited(sel, old)
	}
	if len(sel) == 0 {
		return T("farm.allvisited")
	}
	cls := clusterFarmSpots(sel)
	if len(cls) == 0 {
		return T("farm.nothing")
	}
	// Топ-N кластеров по числу лута (N — слайдер «Точки», по умолчанию 12).
	slices.SortFunc(cls, func(a, b FarmCluster) int { return b.Total - a.Total })
	if n := farmStopsN(g.cfg.FarmStops); len(cls) > n {
		cls = cls[:n]
	}
	// Выход: самый дальний из доступных по фракции.
	ex, _ := g.md.ExitsFor(key)
	cands := make([]Exit, 0, len(ex))
	for _, e := range ex {
		if g.exitVisible(e.Faction) {
			cands = append(cands, e)
		}
	}
	cands = exitDedup(cands)
	if len(cands) == 0 {
		return T("farm.noexits")
	}
	r := &FarmRoute{
		MapKey:   key,
		Floors:   g.cfg.FarmFloors,
		Origin:   [3]float64{x, y, z},
		ExitAuto: true,
	}
	r.Exit = farmPickExit(cands, x, z)
	// регенерация не затирает ручной выбор выхода. Совпадение сперва
	// по полной идентичности, потом по имени (формат данных менялся —
	// близнецы «одно имя, одна точка» у разных фракций).
	if g.farmRoute != nil && !g.farmRoute.ExitAuto {
		prev := g.farmRoute.Exit
		i := slices.IndexFunc(cands, func(e Exit) bool { return exitSame(e, prev) })
		if i < 0 {
			i = slices.IndexFunc(cands, func(e Exit) bool { return e.Name == prev.Name })
		}
		if i >= 0 {
			r.ExitAuto = false
			r.Exit = cands[i]
		}
	}
	// Точки маршрута из кластеров: подписи, ключи, этажи.
	an := g.farmAnchors(key)
	locks := g.md.LocksFor(key)
	r.Stops = make([]FarmStop, len(cls))
	for i := range cls {
		c := cls[i]
		r.Stops[i] = FarmStop{
			X: c.X, Y: c.Y, Z: c.Z,
			Hull:  c.Hull,
			Items: c.Items,
			Total: c.Total,
			Floor: g.md.FloorFor(key, c.Y, ""),
			Name:  farmStopName(cfg.SVG.Bounds, an, c.X, c.Z),
			Keys:  farmStopKeys(g.md, key, locks, c.X, c.Z),
		}
	}
	g.farmRoute = r
	g.farmReorder()
	g.fhScroll = 0
	return T("farm.built", len(r.Stops), g.md.Loc(r.Exit.Name))
}

// farmReorder — перестановка точек жадным «ближайшим соседом» к выбранному
// выходу; пересчёт дистанций. Visited и подписи точек сохраняются.
func (g *Game) farmReorder() {
	r := g.farmRoute
	if r == nil {
		return
	}
	floors := g.md.Floors(r.MapKey)
	rank := func(y float64) int {
		if i := slices.Index(floors, g.md.FloorFor(r.MapKey, y, "")); i >= 0 {
			return i
		}
		return 0
	}
	order := orderFarmStops(r.Origin, r.Stops, rank)
	ordered := make([]FarmStop, len(order))
	px, pz := r.Origin[0], r.Origin[2]
	for i, j := range order {
		s := r.Stops[j]
		s.Dist = int(math.Hypot(s.X-px, s.Z-pz))
		ordered[i] = s
		px, pz = s.X, s.Z
	}
	r.Stops = ordered
	r.Ver++
}

// farmCycleExit — цикл выбора выхода: avto -> выходы по списку -> avto.
func (g *Game) farmCycleExit() {
	r := g.farmRoute
	if r == nil {
		g.setFarmMsg(T("farm.noroute"))
		return
	}
	ex, _ := g.md.ExitsFor(r.MapKey)
	cands := make([]Exit, 0, len(ex))
	for _, e := range ex {
		if g.exitVisible(e.Faction) {
			cands = append(cands, e)
		}
	}
	cands = exitDedup(cands)
	if len(cands) == 0 {
		g.setFarmMsg(T("farm.noexits2"))
		return
	}
	// Совпадение по ПОЛНОЙ идентичности (имя+фракция+позиция): в данных
	// 2026-10 есть близнецы «одно имя, одна точка» у разных фракций и даже
	// два scav-дубля (Railroad to Reserve, y 3.83/3.95). Сравнение только
	// по имени зацикливало выбор между близнецами и не доходило до avto.
	find := func() int {
		return slices.IndexFunc(cands, func(e Exit) bool {
			return exitSame(e, r.Exit)
		})
	}
	if r.ExitAuto {
		// первый клик — выход СЛЕДУЮЩИЙ за автовыбранным, чтобы
		// выбор был виден сразу (cands[0] часто совпадал с автовыбором).
		idx := find()
		r.ExitAuto = false
		r.Exit = cands[(max(idx, 0)+1)%len(cands)]
	} else {
		idx := find()
		// Текущего выхода нет в списке (устаревший маршрут после
		// обновления данных) — тихо возвращаемся к автовыбору.
		if idx < 0 || idx+1 >= len(cands) {
			r.ExitAuto = true
			r.Exit = farmPickExit(cands, r.Origin[0], r.Origin[2])
		} else {
			r.Exit = cands[idx+1]
		}
	}
	g.farmReorder()
}

// exitSame — полная идентичность выхода (см. farmCycleExit).
func exitSame(a, b Exit) bool {
	return a.Name == b.Name && a.Faction == b.Faction && a.X == b.X && a.Z == b.Z
}

// exitDedup — убирает полные дубли выхода (одно имя/фракция/позиция, разная
// только высота y). Такие близнецы есть в данных 2026-10 (Railroad to
// Reserve): без дедупа цикл выбора ходит между ними и не доходит до avto.
func exitDedup(cands []Exit) []Exit {
	out := cands[:0:0]
	for _, e := range cands {
		if !slices.ContainsFunc(out, func(o Exit) bool { return exitSame(o, e) }) {
			out = append(out, e)
		}
	}
	return out
}

// farmUpdate — тик фарма: сброс маршрута при смене карты и автоотметка
// посещённых точек (радиус farmVisitR).
func (g *Game) farmUpdate() {
	if g.farmRoute == nil {
		return
	}
	if g.md.Wanted() != g.farmRoute.MapKey {
		g.farmRoute = nil
		g.farmMsg = ""
		return
	}
	x, _, z, _, ok := g.st.Pos()
	if !ok {
		return
	}
	for i := range g.farmRoute.Stops {
		s := &g.farmRoute.Stops[i]
		if !s.Visited && math.Hypot(x-s.X, z-s.Z) <= farmVisitR {
			s.Visited = true
			g.farmRoute.Ver++
		}
	}
}

// farmLines — строки блока маршрута (панель F3). maxChars — ширина блока в
// символах. Посещённые точки — префикс «OK » тусклым цветом.
func (g *Game) farmLines(maxChars int) []hintLine {
	var lines []hintLine
	add := func(s string, c color.RGBA) {
		for i, w := range wrapText(s, maxChars) {
			if i == 0 {
				lines = append(lines, hintLine{w, c})
			} else {
				lines = append(lines, hintLine{"  " + w, colDim})
			}
		}
	}
	r := g.farmRoute
	if r == nil {
		add(T("farm.hint.none"), colGold)
		add(T("farm.hint.how"), colDim)
		add(T("farm.hint.visited"), colDim)
		return lines
	}
	tag := T("farm.auto")
	if !r.ExitAuto {
		tag = T("farm.pick")
	}
	add(T("farm.route", len(r.Stops), g.md.Loc(r.Exit.Name), tag), colGold)
	if r.Floors {
		add(T("farm.floornote"), colDim)
	}
	for i := range r.Stops {
		s := &r.Stops[i]
		c, pre := colText, ""
		if s.Visited {
			c, pre = colDim, "OK "
		}
		fl := ""
		if s.Floor != "" && len(g.md.Floors(r.MapKey)) > 1 {
			fl = " [" + floorShort(s.Floor) + "]"
		}
		add(fmt.Sprintf("%s%d. %s%s ~%dm", pre, i+1, s.Name, fl, s.Dist), c)
		if len(s.Items) > 0 {
			names := make([]string, 0, len(s.Items))
			for n := range s.Items {
				names = append(names, n)
			}
			slices.Sort(names)
			parts := make([]string, 0, len(names))
			for _, n := range names {
				parts = append(parts, fmt.Sprintf("%s x%d", g.md.Loc(n), s.Items[n]))
			}
			add("   "+strings.Join(parts, ", "), c)
		}
		if len(s.Keys) > 0 {
			add(T("farm.keys", strings.Join(s.Keys, ", ")), colGold)
		}
	}
	add(T("farm.exitline", g.md.Loc(r.Exit.Name), r.Exit.Faction), colGold)
	return lines
}

// farmMapMismatch — маршрут построен для другой карты, чем показана
// (типично при ManualMap: позиция на customs, окно на interchange).
func (g *Game) farmMapMismatch() bool {
	return g.farmRoute != nil && g.farmRoute.MapKey != g.currentMapKey()
}

// truncStr — обрезка строки до n символов с «..» (HUD, чтобы влезал).
func truncStr(s string, n int) string { return truncRunes(s, n) }

// farmHUDTarget — срез для HUD-строки: следующая непосещённая точка
// (в порядке маршрута) и живые дистанции от ТЕКУЩЕЙ позиции игрока.
type farmHUDTarget struct {
	N, Total           int
	Name               string
	DistStop, DistExit int
	ExitName           string
	Done               bool // все точки пройдены — цель выход
}

func (g *Game) farmHUDTarget() (farmHUDTarget, bool) {
	r := g.farmRoute
	if r == nil {
		return farmHUDTarget{}, false
	}
	x, _, z, _, ok := g.st.Pos()
	if !ok {
		return farmHUDTarget{}, false
	}
	out := farmHUDTarget{Total: len(r.Stops), ExitName: r.Exit.Name}
	out.DistExit = int(math.Hypot(x-r.Exit.X, z-r.Exit.Z))
	for i := range r.Stops {
		if !r.Stops[i].Visited {
			out.N = i + 1
			out.Name = truncStr(r.Stops[i].Name, 18)
			out.DistStop = int(math.Hypot(x-r.Stops[i].X, z-r.Stops[i].Z))
			return out, true
		}
	}
	out.Done = true
	return out, true
}

// edgePoint — точка на краю окна (с отступом margin) по лучу из центра к
// цели (экранные координаты). outside=false — цель и так внутри окна.
func edgePoint(w, h int, tx, ty, margin float64) (ex, ey float64, outside bool) {
	cx, cy := float64(w)/2, float64(h)/2
	hw, hh := cx-margin, cy-margin
	dx, dy := tx-cx, ty-cy
	if math.Abs(dx) <= hw && math.Abs(dy) <= hh {
		return tx, ty, false
	}
	if dx == 0 && dy == 0 {
		return cx, cy, false
	}
	t := math.Min(hw/math.Abs(dx), hh/math.Abs(dy))
	if math.IsInf(t, 0) || math.IsNaN(t) || t <= 0 {
		return cx, cy, false
	}
	return cx + dx*t, cy + dy*t, true
}
