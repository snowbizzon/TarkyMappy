package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
)

const (
	winW     = 400 // стартовый размер окна (дефолт, если в конфиге нет валидного)
	winH     = 400
	minWinW  = 260 // пределы ручного ресайза
	minWinH  = 260
	gripS    = 18 // сторона угла-грипа ресайза
	dragBarH = 26 // высота полосы перетаскивания панели настроек
)

var (
	colBackground = color.RGBA{16, 18, 22, 210}
	colText       = color.RGBA{225, 225, 225, 255}
	colDim        = color.RGBA{150, 150, 150, 255}
	colScav       = color.RGBA{110, 200, 110, 255}
	colPMC        = color.RGBA{110, 160, 235, 255}
	colShared     = color.RGBA{235, 205, 90, 255}
	colTransit    = color.RGBA{200, 130, 235, 255}
	colRing       = color.RGBA{20, 20, 20, 230}
	colArrow      = color.RGBA{245, 245, 245, 255}
	colGold       = color.RGBA{212, 175, 55, 255} // фарм-маршрут (вне квест-палитры)
	colGoldDim    = color.RGBA{212, 175, 55, 90}  // посещённые точки/участки
	colGoldMid    = color.RGBA{212, 175, 55, 170} // будущие сегменты маршрута
)

// Game — реализация ebiten.Game.
type Game struct {
	st       *State
	md       *MapData
	hk       *Hotkeys
	cfg      Config
	cfgPath  string
	applyDir func(string) error // смена папки вотчера (из настроек)
	quitReq  bool
	qw       *QuestWatch // Quest Helper: логи EFT + tracked-квесты

	gw, gh int // фактический размер окна (обновляет Layout)

	settings     Settings
	qhelper      bool          // панель Quest Helper открыта (F4)
	qhScroll     int           // прокрутка списка квестов
	qhHintScroll int           // прокрутка блока подсказок
	qhSel        string        // выбранный квест — блок подсказок (что/где/куда)
	qhHintBg     *ebiten.Image // фон блока подсказок (переиспользуется)

	qhOverlay *ebiten.Image // слой контуров visit-зон/кластеров фарма (переиспользуется)

	// Farm Helper: панель + активный маршрут.
	fhelper   bool                          // панель фарма открыта (F3)
	fhScroll  int                           // прокрутка блока маршрута
	farmRoute *FarmRoute                    // активный маршрут (nil — фарм выключен)
	farmMsg   string                        // статус/ошибка (панель + плашка на карте)
	farmMsgAt int                           // тик установки farmMsg (автосброс ~3с)
	frame     int                           // счётчик тиков
	frames    map[image.Point]*ebiten.Image // кэш рамок кнопок (frameFor)

	fhSliderDrag bool // тянем слайдер «Punkty»

	faqOpen   bool          // FAQ-панель открыта (зелёная «?» в F9)
	faqScroll int           // прокрутка FAQ
	faqBtn    *ebiten.Image // зелёная кнопка «?» (кэш по hover)
	faqHover  bool

	pricesOpen  bool   // панель цен барахолки открыта (F2)
	priceScroll int    // прокрутка списка цен
	priceMode   string // режим барахолки: "regular" (PvP) / "pve"

	dirty     bool    // кадр изменился — нужна перерисовка (энергосбережение)
	lastSig   drawSig // снимок состояния прошлого отрисованного кадра
	idleSince time.Time
	curTPS    int // текущий MaxTPS (динамическое снижение в простое)

	resizing                                   bool
	resStartX, resStartY, resStartW, resStartH int

	dragging           bool
	dragOffX, dragOffY int // смещение курсора относительно окна при drag

	downX, downY int // точка нажатия ЛКМ (клик = press+release в одной зоне)
	downOn       bool

	floorKey, floorOv, floorCur string // этажи: карта, ручной оверрайд, авто-текущий

	panel      *ebiten.Image // фоновая плашка (размера окна)
	dot        *ebiten.Image // 12x12 тёмный круг (кайма)
	dots       map[color.RGBA]*ebiten.Image
	arrow      *ebiten.Image
	gear       *ebiten.Image
	check      *ebiten.Image // 6x6 отметка чекбокса
	grip       *ebiten.Image
	frameField *ebiten.Image
	frameBtn   *ebiten.Image
	frameBox   *ebiten.Image
	closeBtn   *ebiten.Image // красная кнопка выключения (кэш по hover/размеру)
	closeTip   *ebiten.Image // плашка-тултип «Close App»
	closeHover bool
	ready      bool
}

func NewGame(st *State, md *MapData, cfg Config, cfgPath string, applyDir func(string) error, qw *QuestWatch) *Game {
	return &Game{
		st: st, md: md, hk: NewHotkeys(),
		cfg: cfg, cfgPath: cfgPath, applyDir: applyDir, qw: qw,
		dirty:     true, // первый кадр рисуем всегда
		dots:      map[color.RGBA]*ebiten.Image{},
		idleSince: time.Now(),
		curTPS:    30,
	}
}

// w/h — текущие логические размеры окна (до первого Layout — дефолт).
func (g *Game) w() int {
	if g.gw > 0 {
		return g.gw
	}
	return winW
}

func (g *Game) h() int {
	if g.gh > 0 {
		return g.gh
	}
	return winH
}

func (g *Game) Update() error {
	g.frame++
	g.hk.Update(g.st, g.quit, g.toggleSettings, g.nextMap, g.cycleFloor, g.toggleQuestPanel, g.toggleFarm, g.togglePrices)
	if g.quitReq {
		return ebiten.Termination
	}
	if g.farmMsg != "" && g.frame-g.farmMsgAt > 90 { // плашка ~3с
		g.farmMsg = ""
	}
	g.farmUpdate() // посещённость точек / сброс при смене карты
	if g.settings.open {
		g.updateSettings()
	} else if g.qhelper {
		g.updateQuestPanel()
	} else if g.fhelper {
		g.updateFarmPanel()
	} else if g.faqOpen {
		g.updateFAQ()
	} else if g.pricesOpen {
		g.updatePricePanel()
	}
	// Dirty-frame: кадр перерисовываем только при реальном изменении.
	if g.sig() != g.lastSig {
		g.dirty = true
		g.idleSince = time.Now()
	}
	// Динамический TPS: 5с без изменений состояния — логика 10 тиков/с
	// (hotkeys/вотчер продолжают работать, отклик кнопки <=100мс).
	want := 30
	if time.Since(g.idleSince) > 5*time.Second {
		want = 10
	}
	if want != g.curTPS {
		ebiten.SetMaxTPS(want)
		g.curTPS = want
	}
	return nil
}

// drawSig — снимок всего, что влияет на кадр. Равен прошлому — Draw пропускает
// отрисовку (экран сохраняет предыдущий кадр), GPU в простое не нагружен.
type drawSig struct {
	x, y, z, rotY, zoom           float64
	hasPos, vis                   bool
	w, h                          int
	mapK, floor                   string
	settingsOpen, qhelper         bool
	qhScroll, qhHintScroll        int
	qhSel, dirText, setStatus     string
	statusOK, delActive           bool
	listScroll                    int
	fac, manualKey                string
	showAll, manualMap            bool
	showScav, showPmc, showShared bool
	showHazards                   bool
	ignoreFloors, debugLog        bool
	qwRev                         int64
	fhelper, farmHas              bool
	fhScroll                      int
	farmVer                       int64
	farmCats                      int
	farmFloors                    bool
	farmStops                     int
	farmMsg                       string
	faqOpen                       bool
	faqScroll                     int
	pricesOpen                    bool
	priceScroll                   int
	priceMode                     string
	lang                          string
	mx, my                        int
}

// sig — текущий снимок состояния кадра.
func (g *Game) sig() drawSig {
	x, y, z, rotY, ok := g.st.Pos()
	s := drawSig{
		x: x, y: y, z: z, rotY: rotY, hasPos: ok,
		zoom: g.st.Zoom(), vis: g.st.Visible(),
		w: g.w(), h: g.h(),
		settingsOpen: g.settings.open, qhelper: g.qhelper,
		qhScroll: g.qhScroll, qhHintScroll: g.qhHintScroll, qhSel: g.qhSel,
		fac: g.cfg.QuestFaction, manualKey: g.cfg.ManualKey, manualMap: g.cfg.ManualMap,
		showAll:  g.cfg.QuestShowAll,
		showScav: g.cfg.ShowScav, showPmc: g.cfg.ShowPmc, showShared: g.cfg.ShowShared,
		showHazards:  g.cfg.ShowHazards,
		ignoreFloors: g.cfg.IgnoreFloors, debugLog: g.cfg.DebugLog,
	}
	if s.settingsOpen {
		s.dirText = string(g.settings.dir)
		s.setStatus = g.settings.status
		s.statusOK = g.settings.statusOK
		s.delActive = g.settings.delDelay > 0
		s.listScroll = g.settings.listScroll
	}
	if g.qw != nil {
		s.qwRev = g.qw.Rev()
	}
	s.fhelper = g.fhelper
	s.farmHas = g.farmRoute != nil
	s.fhScroll = g.fhScroll
	s.farmCats = g.cfg.FarmCats
	s.farmFloors = g.cfg.FarmFloors
	s.farmStops = farmStopsN(g.cfg.FarmStops)
	s.farmMsg = g.farmMsg
	s.faqOpen = g.faqOpen
	s.faqScroll = g.faqScroll
	s.pricesOpen = g.pricesOpen
	s.priceScroll = g.priceScroll
	s.priceMode = g.priceMode
	s.lang = g.cfg.Lang
	if s.farmHas {
		s.farmVer = g.farmRoute.Ver
	}
	s.mapK = g.currentMapKey()
	if s.hasPos && s.mapK != "" {
		s.floor = g.displayFloor(s.mapK, s.y)
	}
	s.mx, s.my = ebiten.CursorPosition()
	return s
}

// toggleSettings — F9: открыть/закрыть панель настроек.
func (g *Game) toggleSettings() {
	if g.settings.open {
		g.closeSettings()
		return
	}
	if g.qhelper {
		g.closeQuestPanel()
	}
	if g.fhelper {
		g.closeFarmPanel()
	}
	if g.faqOpen {
		g.closeFAQ()
	}
	if len(g.settings.dir) == 0 {
		g.settings.dir = []rune(g.cfg.ScreenshotsDir)
	}
	g.settings.SetOpen(true)
}

func (g *Game) closeSettings() {
	procReleaseCapture.Call() // страховка: снять захват мыши после drag/resize
	g.settings.SetOpen(false)
	g.saveWindowGeometry()
}

// quit — F8: вернуть click-through, если панель была открыта, и выйти.
func (g *Game) quit() {
	g.closeSettings()
	g.quitReq = true
}

// saveWindowGeometry — позиция, размер окна и последняя карта в конфиг.
func (g *Game) saveWindowGeometry() {
	x, y := ebiten.WindowPosition()
	w, h := ebiten.WindowSize()
	g.cfg.WinX, g.cfg.WinY, g.cfg.WinW, g.cfg.WinH = x, y, w, h
	g.cfg.LastMap = g.currentMapKey()
	g.saveCfg()
}

// currentMapKey — карта, которую юзер видит сейчас (manual или автодетект).
func (g *Game) currentMapKey() string {
	if g.cfg.ManualMap && g.cfg.ManualKey != "" {
		return g.cfg.ManualKey
	}
	return g.md.Wanted()
}

// mapName — отображаемое имя карты: RU-локаль из maps.json (Locale.Ru),
// иначе ключ. Ключи в конфиге/маршрутах не переводятся — только отображение.
func (g *Game) mapName(key string) string {
	if curLang == "ru" {
		if c, ok := g.md.Cfg(key); ok && c.Locale.Ru != "" {
			return c.Locale.Ru
		}
	}
	return key
}

// nextMap — F10: следующая карта по циклу (быстрая смена прямо в рейде).
func (g *Game) nextMap() {
	keys := g.md.Names()
	if len(keys) == 0 {
		return
	}
	next := keys[(slices.Index(keys, g.currentMapKey())+1)%len(keys)] // i<0 -> первая
	if g.cfg.ManualMap {
		g.cfg.ManualKey = next
	} else {
		g.md.SetWanted(next)
	}
	g.cfg.LastMap = next
	g.saveCfg()
	log.Printf("map -> %s (F10)", next)
}

// displayFloor — этаж для рендера: одноэтажная карта — её единственный этаж;
// фейлсвитч IgnoreFloors — combined ALL; ручной оверрайд; иначе авто по Y.
func (g *Game) displayFloor(key string, y float64) string {
	if len(g.md.Floors(key)) <= 1 {
		return g.md.DefaultFloor(key)
	}
	if g.cfg.IgnoreFloors {
		return floorAll
	}
	if g.floorOv != "" {
		return g.floorOv
	}
	g.floorCur = g.md.FloorFor(key, y, g.floorCur)
	return g.floorCur
}

// cycleFloor — PageUp/PageDown: ручной перебор этажей текущей карты.
func (g *Game) cycleFloor(delta int) {
	key := g.currentMapKey()
	floors := g.md.Floors(key)
	if len(floors) <= 1 || g.cfg.IgnoreFloors {
		return
	}
	if g.floorOv == "" {
		g.floorOv = g.floorCur
		if g.floorOv == "" {
			g.floorOv = g.md.DefaultFloor(key)
		}
	}
	i := slices.Index(floors, g.floorOv)
	if i < 0 {
		i = 0
	}
	g.floorOv = floors[(i+delta+len(floors))%len(floors)]
	g.floorCur = g.floorOv
	log.Printf("floor -> %s (manual, %s)", g.floorOv, key)
}

// exitStyle — выход на другом этаже (|dy| > 5 м): тусклый цвет и стрелка
// "^"/"v" (выше/ниже); в фейлсвитче все выходы полнояркие.
func (g *Game) exitStyle(c color.RGBA, exitY, playerY float64) (color.RGBA, string) {
	if g.cfg.IgnoreFloors || math.Abs(exitY-playerY) <= 5 {
		return c, ""
	}
	c.A = 110
	if exitY > playerY {
		return c, "^"
	}
	return c, "v"
}

// isUndergroundFloor — подземный этаж (тоннели/подвалы/бункеры): его PNG
// почти пустой, поэтому при рендере подкладываем наземную подложку.
func isUndergroundFloor(f string) bool {
	return strings.Contains(f, "Underground") || strings.Contains(f, "Basement") ||
		strings.Contains(f, "Bunkers")
}

// floorShort — короткое имя этажа для подписи: U/G/T, F1..F5, FLAT.
func floorShort(f string) string {
	switch {
	case f == floorAll:
		return "FLAT"
	case isUndergroundFloor(f):
		return "U"
	case strings.Contains(f, "Ground"):
		return "G"
	case strings.Contains(f, "Technical"):
		return "T"
	}
	for w, n := range floorNum {
		if strings.HasPrefix(f, w) {
			return "F" + n
		}
	}
	return f
}

var floorNum = map[string]string{"First": "1", "Second": "2", "Third": "3", "Fourth": "4", "Fifth": "5"}

func (g *Game) saveCfg() {
	if err := SaveConfig(g.cfgPath, g.cfg); err != nil {
		log.Printf("config save: %v", err)
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	if !g.ready {
		g.panel = fillImage(winW, winH, colBackground)
		g.dot = circleImage(12, colRing)
		g.arrow = arrowSprite(18)
		g.gear = gearSprite(22, colText)
		g.check = fillImage(6, 6, colText)
		g.grip = gripSprite(gripS, colText)
		g.frameField = frameImage(g.fieldRect().Dx(), fieldH, colText)
		g.frameBtn = frameImage(btnW, btnH, colText)
		g.frameBox = frameImage(12, 12, colText)
		g.ready = true
	}
	if g.panel.Bounds().Dx() != g.w() || g.panel.Bounds().Dy() != g.h() {
		g.panel = fillImage(g.w(), g.h(), colBackground)
		g.frameField = frameImage(g.fieldRect().Dx(), fieldH, colText) // ширина поля зависит от окна
		g.dirty = true
	}
	if !g.dirty { // состояние не менялось — прошлый кадр ещё на экране
		return
	}
	g.dirty = false
	g.lastSig = g.sig()
	screen.Clear()
	if g.settings.open {
		g.drawSettings(screen)
		g.drawGear(screen)
		return
	}
	if g.qhelper {
		g.drawQuestPanel(screen)
		g.drawGear(screen)
		return
	}
	if g.fhelper {
		g.drawFarmPanel(screen)
		g.drawGear(screen)
		return
	}
	if g.faqOpen {
		g.drawFAQ(screen)
		g.drawGear(screen)
		return
	}
	if g.pricesOpen {
		g.drawPricePanel(screen)
		g.drawGear(screen)
		return
	}
	if !g.st.Visible() {
		g.drawGear(screen)
		return
	}
	x, y, z, rotY, ok := g.st.Pos()
	if !ok {
		g.drawMsg(screen, "TarkyMappy", T("hud.nopos"))
		g.drawGear(screen)
		return
	}
	key := g.md.Wanted()
	if g.cfg.ManualMap && g.cfg.ManualKey != "" {
		key = g.cfg.ManualKey // ручной выбор — приоритет над автодетектом
	}
	cfg, found := g.md.Cfg(key)
	if key == "" || !found {
		g.drawMsg(screen, T("hud.nomap"), fmt.Sprintf("X,Z = %s, %s", fmtF(x), fmtF(z)))
		g.drawGear(screen)
		return
	}
	if key != g.floorKey { // смена карты — ручной этаж сбрасываем
		g.floorKey, g.floorOv, g.floorCur = key, "", ""
	}
	floor := g.displayFloor(key, y)
	img, err := g.md.EnsureImage(key, floor)
	if err != nil {
		g.drawMsg(screen, T("hud.nomap"), fmt.Sprintf("X,Z = %s, %s", fmtF(x), fmtF(z)))
		g.drawGear(screen)
		return
	}

	// Карта: игрок в центре, взгляд — вверх.
	gm := FullGeoM(cfg.SVG.Bounds, x, z, rotY, g.st.Zoom(), img.Bounds().Dx(), img.Bounds().Dy(), g.w(), g.h())
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
	op.GeoM = gm
	if isUndergroundFloor(floor) {
		// Подземные этажи — почти пустые «тоннельные» PNG (customs/groundzero/
		// shoreline/streets): подкладываем наземный этаж полупрозрачно, иначе
		// карта выглядит прозрачной.
		if base, err := g.md.EnsureImage(key, g.md.DefaultFloor(key)); err == nil && base != img {
			opB := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
			opB.GeoM = gm
			opB.ColorScale.ScaleAlpha(0.4)
			screen.DrawImage(base, opB)
		}
	}
	screen.DrawImage(img, op)

	// Маркеры выходов (той же матрицей) с подписями имён (RU через md.Loc).
	extracts, transits := g.md.ExitsFor(key)
	b, iw, ih := cfg.SVG.Bounds, img.Bounds().Dx(), img.Bounds().Dy()

	// Опасности: минные поля/снайперские зоны контурами, ловушки «+» —
	// под выходами, над подложкой.
	if g.cfg.ShowHazards {
		g.drawHazards(screen, key, b, iw, ih, gm, y)
	}

	for i := range extracts {
		if !g.exitVisible(extracts[i].Faction) {
			continue
		}
		if u, v, in := exitUV(b, extracts[i], iw, ih, g.w(), g.h(), gm); in {
			col, arrow := g.exitStyle(exitColor(extracts[i].Faction), extracts[i].Y, y)
			g.drawExit(screen, u, v, col, arrow+g.md.Loc(extracts[i].Name), false)
		}
	}
	for i := range transits {
		if u, v, in := exitUV(b, transits[i], iw, ih, g.w(), g.h(), gm); in {
			col, arrow := g.exitStyle(colTransit, transits[i].Y, y)
			g.drawExit(screen, u, v, col, arrow+g.md.Loc(transits[i].Name), true)
		}
	}

	// Маршрут фарма (золото) + отслеживаемые квесты (скрыты при активном фарме).
	g.drawFarmRoute(screen, key, b, iw, ih, gm)
	g.drawFarmHUD(screen, key)
	g.drawQuestMarkers(screen, key, b, iw, ih, gm, y)

	// Стрелка игрока — всегда в центре.
	opA := &ebiten.DrawImageOptions{}
	opA.GeoM.Translate(float64(g.w())/2-float64(g.arrow.Bounds().Dx())/2,
		float64(g.h())/2-float64(g.arrow.Bounds().Dy())/2)
	screen.DrawImage(g.arrow, opA)

	// Текущая карта и координаты — внизу слева (сразу видно, «где я»).
	mapLabel := g.mapName(key)
	if g.cfg.ManualMap && g.cfg.ManualKey != "" {
		mapLabel += " " + T("tag.manual")
	}
	if len(g.md.Floors(key)) > 1 {
		mapLabel += " " + floorShort(floor)
	}
	drawTextOutlined(screen, 4, g.h()-4,
		fmt.Sprintf("%s %s,%s,%s", mapLabel, fmtF(x), fmtF(y), fmtF(z)), color.RGBA{170, 178, 190, 235})
	if g.farmMsg != "" { // статус фарма над строкой карты
		drawTextOutlined(screen, 4, g.h()-18, g.farmMsg, colGold)
	}
	g.drawGear(screen)
}

// exitVisible — фильтр выходов по фракции (галочки в настройках).
func (g *Game) exitVisible(faction string) bool {
	switch faction {
	case "scav":
		return g.cfg.ShowScav
	case "pmc":
		return g.cfg.ShowPmc
	default:
		return g.cfg.ShowShared
	}
}

func exitUV(b [2][2]float64, e Exit, iw, ih, w, h int, gm ebiten.GeoM) (sx, sy float64, in bool) {
	u, v := WorldToUV(b, e.X, e.Z)
	sx, sy = gm.Apply(u*float64(iw), v*float64(ih))
	in = sx >= -8 && sx <= float64(w)+8 && sy >= -8 && sy <= float64(h)+8
	return sx, sy, in
}

func exitColor(faction string) color.RGBA {
	switch faction {
	case "scav":
		return colScav
	case "pmc":
		return colPMC
	default:
		return colShared
	}
}

// Цвета опасностей: как у re3mr — мины красным, снайперы оранжевым,
// ловушки (клейморы) золотым «+».
var (
	colMine    = color.RGBA{225, 60, 60, 255}
	colMineDim = color.RGBA{225, 60, 60, 80}
	colSniper  = color.RGBA{235, 150, 40, 255}
	colSnipDim = color.RGBA{235, 150, 40, 80}
	colClay    = color.RGBA{212, 175, 55, 255}
	colClayDim = color.RGBA{212, 175, 55, 80}
)

// drawHazards — опасности карты: зоны (минные/снайперские) полигоном,
// ловушки-клейморы золотым «+». Чужой этаж (|dy|>5 м) — тусклее, как выходы.
func (g *Game) drawHazards(screen *ebiten.Image, key string, b [2][2]float64, iw, ih int, gm ebiten.GeoM, playerY float64) {
	mute := func(col color.RGBA, y float64) color.RGBA {
		if g.cfg.IgnoreFloors || math.Abs(y-playerY) <= 5 {
			return col
		}
		dim := col
		dim.A = 70
		return dim
	}
	px := func(p [2]float64) (float64, float64) {
		u, v := WorldToUV(b, p[0], p[1])
		return gm.Apply(u*float64(iw), v*float64(ih))
	}
	for _, hz := range g.md.HazardsFor(key) {
		switch hz.T {
		case "m", "s":
			col := colMine
			dim := colMineDim
			if hz.T == "s" {
				col, dim = colSniper, colSnipDim
			}
			col = mute(col, hz.Y)
			if len(hz.Ol) >= 3 { // контур зоны (в фейлсвитче этажей — полнояркий)
				for i := range hz.Ol {
					x0, y0 := px(hz.Ol[i])
					x1, y1 := px(hz.Ol[(i+1)%len(hz.Ol)])
					drawLine(screen, x0, y0, x1, y1, mute(dim, hz.Y))
				}
			}
			if sx, sy, ok := g.hazardScreen(b, hz.X, hz.Z, iw, ih, gm, g.w(), g.h()); ok {
				g.drawDot(screen, sx, sy, mute(col, hz.Y))
			}
		case "c":
			if sx, sy, ok := g.hazardScreen(b, hz.X, hz.Z, iw, ih, gm, g.w(), g.h()); ok {
				g.drawClaymore(screen, sx, sy, mute(colClay, hz.Y))
			}
		default:
			if sx, sy, ok := g.hazardScreen(b, hz.X, hz.Z, iw, ih, gm, g.w(), g.h()); ok {
				g.drawDot(screen, sx, sy, mute(colMine, hz.Y))
			}
		}
	}
}

// hazardScreen — мировая точка опасности -> экран (+ видимость в окне).
func (g *Game) hazardScreen(b [2][2]float64, x, z float64, iw, ih int, gm ebiten.GeoM, w, h int) (float64, float64, bool) {
	u, v := WorldToUV(b, x, z)
	sx, sy := gm.Apply(u*float64(iw), v*float64(ih))
	return sx, sy, sx >= -8 && sx <= float64(w)+8 && sy >= -8 && sy <= float64(h)+8
}

// drawClaymore — золотой «+» в тёмном круге (стиль иконки re3mr).
func (g *Game) drawClaymore(screen *ebiten.Image, sx, sy float64, col color.RGBA) {
	drawLine(screen, sx-5, sy, sx+5, sy, col)
	drawLine(screen, sx, sy-5, sx, sy+5, col)
	drawLine(screen, sx-5, sy-5, sx+5, sy+5, colRing)
	drawLine(screen, sx+5, sy-5, sx-5, sy+5, colRing)
}

func (g *Game) drawDot(screen *ebiten.Image, sx, sy float64, col color.RGBA) {
	r := 6.0
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(sx-r, sy-r)
	screen.DrawImage(g.dot, op)
	tint, ok := g.dots[col]
	if !ok {
		tint = fillImage(8, 8, col)
		g.dots[col] = tint
	}
	op2 := &ebiten.DrawImageOptions{}
	op2.GeoM.Translate(sx-4, sy-4)
	screen.DrawImage(tint, op2)
}

// drawExit — точка выхода + подпись имени (руно-безопасная обрезка).
func (g *Game) drawExit(screen *ebiten.Image, sx, sy float64, col color.RGBA, name string, transit bool) {
	g.drawDot(screen, sx, sy, col)
	label := name
	if transit {
		label = "T|" + name
	}
	lw := txtW(label)
	tx := int(sx) + 9
	if tx+lw > g.w()-2 { // не влезает справа — слева от точки
		tx = int(sx) - 9 - lw
	}
	if tx < 2 { // узкое окно — обрезать хвост подписи (руно-безопасно)
		if maxW := g.w() - 4; maxW > 4*charW {
			label = truncRunes(label, maxW/charW)
		}
		tx = 2
	}
	ty := int(sy) - 3
	if ty < 10 {
		ty = int(sy) + 13
	}
	drawTextOutlined(screen, tx, ty, label, col)
}

// drawTextOutlined — текст с 1px обводкой (4 сдвига), читаемость на карте.
func drawTextOutlined(screen *ebiten.Image, x, y int, s string, col color.RGBA) {
	for _, d := range [4][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
		text.Draw(screen, s, uiFace, x+d[0], y+d[1], colRing)
	}
	text.Draw(screen, s, uiFace, x, y, col)
}

func (g *Game) drawMsg(screen *ebiten.Image, l1, l2 string) {
	screen.DrawImage(g.panel, &ebiten.DrawImageOptions{})
	text.Draw(screen, "TarkyMappy", uiFace, 12, 24, colText)
	text.Draw(screen, l1, uiFace, 12, 44, colText)
	text.Draw(screen, l2, uiFace, 12, 60, colText)
}

// Layout — логический размер следует за окном (ручной ресайз).
func (g *Game) Layout(outW, outH int) (int, int) {
	g.gw, g.gh = outW, outH
	return outW, outH
}

func fmtF(f float64) string { return fmt.Sprintf("%.1f", f) }
