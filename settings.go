package main

import (
	"image"
	"image/color"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

var (
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procReleaseCapture   = user32.NewProc("ReleaseCapture")

	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGlobalSize    = kernel32.NewProc("GlobalSize")
	procRtlMoveMemory = kernel32.NewProc("RtlMoveMemory")
)

const (
	padX      = 16
	fieldH    = 26
	btnW      = 82
	btnH      = 26
	listRowH  = 16  // высота строки списка карт
	manualTop = 118 // строка галочки Manual
	listTop   = 156 // верх списка карт
)

// Settings — состояние панели настроек.
type Settings struct {
	open       bool
	dir        []rune
	status     string
	statusOK   bool
	delDelay   int
	listScroll int // смещение списка карт (в строках)
}

// --- Геометрия панели (динамическая: окно таскается и ресайзится). ---

func (g *Game) fieldRect() image.Rectangle {
	w := g.w() - padX*2 - btnW - 8
	if w < 90 {
		w = 90
	}
	return image.Rect(padX, 48, padX+w, 48+fieldH)
}

func (g *Game) browseRect() image.Rectangle {
	f := g.fieldRect()
	return image.Rect(f.Max.X+8, f.Min.Y, f.Max.X+8+btnW, f.Max.Y)
}

func (g *Game) applyRect() image.Rectangle {
	return image.Rect(padX, g.h()-76, padX+btnW, g.h()-76+btnH)
}

func (g *Game) closeRect() image.Rectangle {
	return image.Rect(padX+btnW+10, g.h()-76, padX+btnW+10+btnW, g.h()-76+btnH)
}

// debugRect — переключатель debug.log, правый нижний угол (над грипом).
func (g *Game) debugRect() image.Rectangle {
	return image.Rect(g.w()-112, g.h()-46, g.w()-16, g.h()-30)
}

// langRect — кнопка цикла языка RU/EN: строка красной X, слева от неё
// (не пересекается ни с debug, ни с галочкой этажей при любом размере окна).
func (g *Game) langRect() image.Rectangle {
	return image.Rect(g.w()-230, g.h()-70, g.w()-126, g.h()-54)
}

// langZone — клик-зона кнопки языка.
func (g *Game) langZone() image.Rectangle {
	r := g.langRect()
	return image.Rect(r.Min.X-3, r.Min.Y-1, r.Max.X+3, r.Max.Y+1)
}

// floorsRect — галочка «Игнорировать этажи» в строке Apply/Close.
// Размещена правее кнопок, чтобы не пересекаться со строкой подсказок.
func (g *Game) floorsRect() image.Rectangle {
	return image.Rect(padX+btnW*2+24, g.h()-70, padX+btnW*2+36, g.h()-58)
}

// floorsZone — клик-зона галочки этажей (рамка + подпись).
func (g *Game) floorsZone() image.Rectangle {
	r := g.floorsRect()
	return image.Rect(r.Min.X-3, r.Min.Y-1, r.Min.X+100, r.Max.Y+3)
}

// closeAppRect — красная кнопка выключения (правый нижний угол, над debug).
func (g *Game) closeAppRect() image.Rectangle {
	return image.Rect(g.w()-112, g.h()-70, g.w()-16, g.h()-54)
}

// closeAppTipRect — плашка-подсказка «Close App» левее кнопки
// (над кнопкой расположена зелёная кнопка «?» — FAQ).
func (g *Game) closeAppTipRect() image.Rectangle {
	return image.Rect(g.w()-252, g.h()-72, g.w()-116, g.h()-56)
}

// gripRect — зона ресайза в правом нижнем углу.
func (g *Game) gripRect() image.Rectangle {
	return image.Rect(g.w()-gripS, g.h()-gripS, g.w(), g.h())
}

// gearRect — зона шестерёнки на правом краю окна.
func (g *Game) gearRect() image.Rectangle {
	return image.Rect(g.w()-30, g.h()/2-12, g.w()-8, g.h()/2+10)
}

// cbLabels — подписи чекбоксов выходов+опасности: ключи локализации.
var cbLabels = []string{"exit.pmc", "exit.scav", "exit.shared", "exit.haz"}

// cbLabel — подпись чекбокса в текущем языке.
func cbLabel(i int) string { return T(cbLabels[i]) }

// cbRect — рамка чекбокса i.
func (g *Game) cbRect(i int) image.Rectangle {
	x := padX + i*88
	return image.Rect(x, 98, x+12, 110)
}

// cbZone — клик-зона чекбокса (рамка + подпись, ширина по txtW для кириллицы).
// До y=111: строка Manual начинается с y=112 — зоны не пересекаются.
func (g *Game) cbZone(i int) image.Rectangle {
	r := g.cbRect(i)
	return image.Rect(r.Min.X-3, 94, r.Min.X+21+txtW(cbLabel(i)), 111)
}

// manualRect — рамка галочки «Manual vybor karty».
func (g *Game) manualRect() image.Rectangle {
	return image.Rect(padX, manualTop, padX+12, manualTop+12)
}

// manualZone — клик-зона галочки ручного выбора.
func (g *Game) manualZone() image.Rectangle {
	return image.Rect(padX-3, manualTop-1, padX+144, manualTop+18)
}

// qhToggleRect — чекбокс «Quests (F4)» на строке Manual (правее).
func (g *Game) qhToggleRect() image.Rectangle {
	return image.Rect(padX+150, manualTop, padX+162, manualTop+12)
}

// qhToggleZone — клик-зона чекбокса Quests.
func (g *Game) qhToggleZone() image.Rectangle {
	r := g.qhToggleRect()
	return image.Rect(r.Min.X-3, r.Min.Y-1, r.Min.X+90, r.Max.Y+4)
}

// farmToggleRect — чекбокс «Farm (F3)» на строке Manual.
func (g *Game) farmToggleRect() image.Rectangle {
	return image.Rect(padX+255, manualTop, padX+267, manualTop+12)
}

// farmToggleZone — клик-зона чекбокса Farm.
func (g *Game) farmToggleZone() image.Rectangle {
	r := g.farmToggleRect()
	return image.Rect(r.Min.X-3, r.Min.Y-1, r.Min.X+85, r.Max.Y+4)
}

// pricesToggleRect — чекбокс «Цены (F2)» (строка Apply/Close, правее
// «Игнорировать этажи» — старое место налезало на заголовок списка карт).
func (g *Game) pricesToggleRect() image.Rectangle {
	f := g.floorsRect()
	return image.Rect(f.Max.X+94, f.Min.Y, f.Max.X+106, f.Max.Y)
}

// pricesToggleZone — клик-зона чекбокса Цены.
func (g *Game) pricesToggleZone() image.Rectangle {
	r := g.pricesToggleRect()
	return image.Rect(r.Min.X-3, r.Min.Y-1, r.Min.X+86, r.Max.Y+3)
}

// faqRect — зелёная кнопка «?» (FAQ) над красной X.
func (g *Game) faqRect() image.Rectangle {
	return image.Rect(g.w()-112, g.h()-90, g.w()-16, g.h()-74)
}

// listRect — зона списка карт (видна при ManualMap).
func (g *Game) listRect() image.Rectangle {
	bottom := g.h() - 100 // над статусом и кнопками
	if bottom < listTop+listRowH {
		bottom = listTop + listRowH
	}
	return image.Rect(padX, listTop, g.w()-padX, bottom)
}

func (g *Game) listCols() int {
	if g.w() >= 340 {
		return 2
	}
	return 1
}

func (g *Game) listColW() int { return (g.w() - padX*2) / g.listCols() }

// listRows — всего строк списка для текущего набора карт.
func (g *Game) listRows(keys []string) int { return (len(keys) + g.listCols() - 1) / g.listCols() }

// clampScroll — смещение списка в допустимых пределах.
func (g *Game) clampScroll(scroll, rows int) int {
	visible := g.listRect().Dy() / listRowH
	max := rows - visible
	if max < 0 {
		max = 0
	}
	return clampI(scroll, 0, max)
}

func (s *Settings) SetOpen(open bool) {
	if s.open == open {
		return
	}
	s.open = open
	// Панель кликабельна; карта — клики насквозь.
	// Фокус окно получает от первого клика юзера (без Alt-трюков).
	ebiten.SetWindowMousePassthrough(!open)
}

func (s *Settings) setStatus(msg string, ok bool) {
	s.status, s.statusOK = msg, ok
}

// updateSettings — drag/ресайз окна, ввод и клики в открытой панели.
func (g *Game) updateSettings() {
	s := &g.settings
	mx, my := ebiten.CursorPosition()
	press := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	release := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if press {
		g.downX, g.downY, g.downOn = mx, my, true
	}
	// clickIn — нажали и отпустили в одной зоне: первый клик по неактивному
	// окну не «сгорает», промах в соседнюю зону ничего не переключает.
	clickIn := func(r image.Rectangle) bool {
		return release && g.downOn && ptIn(g.downX, g.downY, r) && ptIn(mx, my, r)
	}

	// Общий каркас окна: ресайз за грип, перенос за верхнюю полосу.
	if g.handleWindowChrome(mx, my, press, held) {
		return
	}

	// Ввод пути.
	for _, r := range ebiten.InputChars() {
		if r >= 32 {
			s.dir = append(s.dir, r)
		}
	}
	if ebiten.IsKeyPressed(ebiten.KeyBackspace) {
		if len(s.dir) > 0 && s.delDelay == 0 {
			s.dir = s.dir[:len(s.dir)-1]
			s.delDelay = 24
		} else if s.delDelay > 0 {
			s.delDelay--
		}
	} else {
		s.delDelay = 0
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		if t := clipboardText(); t != "" {
			s.dir = append(s.dir[:0], []rune(strings.TrimSpace(t))...)
		}
	}

	// Колесо — прокрутка списка карт.
	if g.cfg.ManualMap {
		if _, dy := ebiten.Wheel(); dy != 0 {
			keys := g.md.Names()
			g.settings.listScroll = g.clampScroll(g.settings.listScroll-int(dy), g.listRows(keys))
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closeSettings()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.applySettings()
		return
	}
	switch {
	case clickIn(g.browseRect()):
		if path, ok := FolderDialog(); ok {
			s.dir = []rune(path)
			s.status = ""
		}
	case clickIn(g.applyRect()):
		g.applySettings()
	case clickIn(g.closeRect()):
		g.closeSettings()
	case clickIn(g.closeAppRect()):
		g.quit()
	case clickIn(g.faqRect()): // зелёная «?» — FAQ
		g.toggleFAQ()
		return
	case clickIn(g.farmToggleZone()): // чекбокс Farm (F3)
		g.toggleFarm()
		return
	case clickIn(g.pricesToggleZone()): // чекбокс Цены (F2)
		g.togglePrices()
		return
	case clickIn(g.langZone()): // цикл языка RU/EN с сохранением в конфиг (строка X)
		g.cfg.Lang = nextLang()
		setLang(g.cfg.Lang)
		g.saveCfg()
		dbgLog("lang -> %s", g.cfg.Lang)
	case clickIn(g.debugRect()):
		g.cfg.DebugLog = !g.cfg.DebugLog
		debugLog = g.cfg.DebugLog
		g.saveCfg()
		dbgLog("debug log -> %v", debugLog)
	case clickIn(g.floorsZone()):
		g.cfg.IgnoreFloors = !g.cfg.IgnoreFloors // фейлсвитч плоской карты
		g.saveCfg()
		dbgLog("ignore floors -> %v", g.cfg.IgnoreFloors)
	case clickIn(g.manualZone()):
		g.cfg.ManualMap = !g.cfg.ManualMap
		g.saveCfg()
		dbgLog("manual map -> %v (%q)", g.cfg.ManualMap, g.cfg.ManualKey)
	case clickIn(g.qhToggleZone()):
		if g.farmActive() { // при фарме квесты недоступны (серый пункт)
			g.setFarmMsg(T("farm.blockq"))
			return
		}
		g.toggleQuestPanel()
		return
	case clickIn(g.listRect()) && g.cfg.ManualMap:
		g.pickMapAt(mx, my)
	default:
		if release && g.downOn {
			if i, ok := g.checkboxAt(mx, my); ok && ptIn(g.downX, g.downY, g.cbZone(i)) {
				switch i {
				case 0:
					g.cfg.ShowPmc = !g.cfg.ShowPmc
				case 1:
					g.cfg.ShowScav = !g.cfg.ShowScav
				case 2:
					g.cfg.ShowShared = !g.cfg.ShowShared
				case 3:
					g.cfg.ShowHazards = !g.cfg.ShowHazards // мины/снайперы/ловушки
				}
				g.saveCfg()
			}
		}
	}
}

// checkboxAt — индекс чекбокса под точкой.
func (g *Game) checkboxAt(x, y int) (int, bool) {
	for i := range cbLabels {
		if ptIn(x, y, g.cbZone(i)) {
			return i, true
		}
	}
	return 0, false
}

// pickMapAt — выбор карты в списке ручного выбора (повторный клик — снять).
func (g *Game) pickMapAt(mx, my int) {
	keys := g.md.Names()
	r := g.listRect()
	row := (my-r.Min.Y)/listRowH + g.settings.listScroll
	col := (mx - r.Min.X) / g.listColW()
	i := row*g.listCols() + col
	if i < 0 || i >= len(keys) {
		return
	}
	if g.cfg.ManualKey == keys[i] {
		g.cfg.ManualKey = ""
	} else {
		g.cfg.ManualKey = keys[i]
	}
	g.saveCfg()
	dbgLog("manual map key -> %q", g.cfg.ManualKey)
}

// drawMapList — список карт (2 колонки при широком окне, скролл колесом).
func (g *Game) drawMapList(screen *ebiten.Image) {
	keys := g.md.Names()
	if len(keys) == 0 {
		return
	}
	cols, colW := g.listCols(), g.listColW()
	r := g.listRect()
	scroll := g.clampScroll(g.settings.listScroll, g.listRows(keys))
	visible := r.Dy() / listRowH
	for i, k := range keys {
		row, col := i/cols, i%cols
		if row < scroll || row >= scroll+visible {
			continue
		}
		x := r.Min.X + col*colW
		y := r.Min.Y + (row-scroll)*listRowH
		box := image.Rect(x, y+2, x+10, y+12)
		screen.DrawImage(g.frameBox, opAt(box.Min))
		colr := colText
		if g.cfg.ManualKey == k {
			screen.DrawImage(g.check, opAt(box.Min.Add(image.Point{2, 2})))
			colr = colScav
		}
		text.Draw(screen, g.mapName(k), uiFace, x+16, y+12, colr)
	}
}

// applySettings — проверить папку, сохранить конфиг, перезапустить вотчер.
func (g *Game) applySettings() {
	s := &g.settings
	dir := strings.TrimSpace(string(s.dir))
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		s.setStatus(T("set.nodir"), false)
		return
	}
	if dir != g.cfg.ScreenshotsDir {
		g.cfg.ScreenshotsDir = dir
		g.saveCfg()
		if g.applyDir != nil {
			if err := g.applyDir(dir); err != nil {
				s.setStatus("Watcher: "+err.Error(), false)
				return
			}
		}
	}
	s.setStatus("OK: "+dir, true)
	g.closeSettings()
}

func (g *Game) drawSettings(screen *ebiten.Image) {
	s := &g.settings
	w, h := g.w(), g.h()
	screen.DrawImage(g.panel, &ebiten.DrawImageOptions{})

	// Полоса перетаскивания: заголовок, подсказка, пунктирная линия.
	text.Draw(screen, T("set.title"), uiFace, padX, 19, colText)
	hint := T("drag.hint")
	text.Draw(screen, hint, uiFace, w-8-txtW(hint), 19, colDim)
	for x := 4; x < w-4; x += 6 {
		screen.Set(x, dragBarH, colDim)
		screen.Set(x+1, dragBarH, colDim)
	}

	text.Draw(screen, T("set.dir"), uiFace, padX, 40, colDim)

	screen.DrawImage(g.frameField, opAt(g.fieldRect().Min))
	str := string(s.dir)
	if maxChars := (g.fieldRect().Dx() - 8) / charW; utf8.RuneCountInString(str) > maxChars {
		str = clipTail(str, maxChars)
	}
	text.Draw(screen, str, uiFace, g.fieldRect().Min.X+4, g.fieldRect().Min.Y+18, colText)

	drawButton(screen, g.frameBtn, g.browseRect(), T("btn.browse"), colText)

	// Чекбоксы выходов.
	text.Draw(screen, T("set.exits"), uiFace, padX, 88, colDim)
	ons := []bool{g.cfg.ShowPmc, g.cfg.ShowScav, g.cfg.ShowShared, g.cfg.ShowHazards}
	for i := range cbLabels {
		r := g.cbRect(i)
		screen.DrawImage(g.frameBox, opAt(r.Min))
		if ons[i] {
			screen.DrawImage(g.check, opAt(r.Min.Add(image.Point{3, 3})))
		}
		text.Draw(screen, cbLabel(i), uiFace, r.Max.X+4, r.Max.Y-1, colText)
	}

	// Ручной выбор карты: галочка + список.
	mr := g.manualRect()
	screen.DrawImage(g.frameBox, opAt(mr.Min))
	if g.cfg.ManualMap {
		screen.DrawImage(g.check, opAt(mr.Min.Add(image.Point{3, 3})))
	}
	text.Draw(screen, T("set.manual"), uiFace, mr.Max.X+4, mr.Max.Y-1, colText)

	// «Quests (F4)» — переключатель панели квестов (та же строка, правее).
	// При активном фарме кнопка неактивна: квесты и фарм не совмещаются.
	qr := g.qhToggleRect()
	screen.DrawImage(g.frameBox, opAt(qr.Min))
	if g.qhelper {
		screen.DrawImage(g.check, opAt(qr.Min.Add(image.Point{3, 3})))
	}
	qc := colText
	if g.farmActive() {
		qc = colDim
	}
	text.Draw(screen, T("set.quests"), uiFace, qr.Max.X+4, qr.Max.Y-1, qc)

	// «Farm (F3)» — переключатель панели фарма.
	frr := g.farmToggleRect()
	screen.DrawImage(g.frameBox, opAt(frr.Min))
	if g.fhelper || g.farmActive() {
		screen.DrawImage(g.check, opAt(frr.Min.Add(image.Point{3, 3})))
	}
	text.Draw(screen, T("set.farm"), uiFace, frr.Max.X+4, frr.Max.Y-1, colGold)

	// «Цены (F2)» — переключатель панели барахолки (своя строка под Manual).
	pr := g.pricesToggleRect()
	screen.DrawImage(g.frameBox, opAt(pr.Min))
	if g.pricesOpen {
		screen.DrawImage(g.check, opAt(pr.Min.Add(image.Point{3, 3})))
	}
	text.Draw(screen, T("set.prices"), uiFace, pr.Max.X+4, pr.Max.Y-1, colGold)
	if g.cfg.ManualMap {
		if g.cfg.ManualKey == "" {
			text.Draw(screen, T("set.pickmap"), uiFace, padX, listTop-8, colDim)
		} else {
			text.Draw(screen, T("set.map", g.mapName(g.cfg.ManualKey)), uiFace, padX, listTop-8, colScav)
		}
		g.drawMapList(screen)
	}

	if s.status != "" {
		col := color.RGBA{110, 200, 110, 255}
		if !s.statusOK {
			col = color.RGBA{235, 110, 110, 255}
		}
		text.Draw(screen, clipTail(s.status, (w-2*padX)/charW), uiFace, padX, h-92, col)
	}

	drawButton(screen, g.frameBtn, g.applyRect(), T("btn.apply"), colText)
	drawButton(screen, g.frameBtn, g.closeRect(), T("btn.close"), colText)

	// debug.log — правый нижний угол, над грипом.
	dc := colDim
	if g.cfg.DebugLog {
		dc = colScav
	}
	drawButton(screen, g.frameBtn, g.debugRect(), "debug: "+onOff(g.cfg.DebugLog), dc)

	// Кнопка языка RU/EN — строка красной X, слева от неё.
	drawButton(screen, g.frameFor(g.langRect().Dx(), btnH), g.langRect(),
		T("lang.btn", strings.ToUpper(g.cfg.Lang)), colText)

	// «Игнорировать этажи» — фейлсвитч плоской карты (слева на строке debug).
	fr := g.floorsRect()
	screen.DrawImage(g.frameBox, opAt(fr.Min))
	if g.cfg.IgnoreFloors {
		screen.DrawImage(g.check, opAt(fr.Min.Add(image.Point{3, 3})))
	}
	fc := colText
	if g.cfg.IgnoreFloors {
		fc = colScav
	}
	text.Draw(screen, T("set.floors"), uiFace, fr.Max.X+4, fr.Max.Y-1, fc)

	// Красная кнопка выключения (правый нижний угол) + тултип «Close App».
	g.drawCloseApp(screen)

	// Грип ресайза — в самом углу.
	screen.DrawImage(g.grip, opAt(image.Point{X: w - gripS, Y: h - gripS}))

	// Подсказка хоткеев — две строки.
	tip1, tip2 := T("set.tip1"), T("set.tip2")
	if max := (w - padX - gripS - 8) / charW; max > 4 {
		tip1 = truncRunes(tip1, max)
		tip2 = truncRunes(tip2, max)
	}
	text.Draw(screen, tip1, uiFace, padX, h-24, colDim)
	text.Draw(screen, tip2, uiFace, padX, h-12, colDim)
}

func drawButton(screen *ebiten.Image, frame *ebiten.Image, r image.Rectangle, label string, col color.RGBA) {
	screen.DrawImage(frame, opAt(r.Min))
	lw := txtW(label)
	text.Draw(screen, label, uiFace, r.Min.X+(r.Dx()-lw)/2, r.Min.Y+18, col)
}

// drawCloseApp — красная кнопка выключения; при наведении — подсветка и
// плашка «Close App». Клик — то же, что F8 (корректное завершение).
func (g *Game) drawCloseApp(screen *ebiten.Image) {
	cr := g.closeAppRect()
	mx, my := ebiten.CursorPosition()
	hover := ptIn(mx, my, cr)
	if hover != g.closeHover || g.closeBtn == nil ||
		g.closeBtn.Bounds().Dx() != cr.Dx() || g.closeBtn.Bounds().Dy() != cr.Dy() {
		g.closeHover = hover
		cc := color.RGBA{185, 55, 55, 255}
		if hover {
			cc = color.RGBA{235, 80, 80, 255}
		}
		g.closeBtn = fillImage(cr.Dx(), cr.Dy(), cc)
	}
	screen.DrawImage(g.closeBtn, opAt(cr.Min))
	cl := txtW("X")
	text.Draw(screen, "X", uiFace, cr.Min.X+(cr.Dx()-cl)/2, cr.Min.Y+12, colText)

	// Зелёная «?» (FAQ) над красной X.
	fr := g.faqRect()
	fhover := ptIn(mx, my, fr)
	if fhover != g.faqHover || g.faqBtn == nil ||
		g.faqBtn.Bounds().Dx() != fr.Dx() || g.faqBtn.Bounds().Dy() != fr.Dy() {
		g.faqHover = fhover
		fc := color.RGBA{60, 145, 60, 255}
		if fhover {
			fc = color.RGBA{95, 200, 95, 255}
		}
		g.faqBtn = fillImage(fr.Dx(), fr.Dy(), fc)
	}
	screen.DrawImage(g.faqBtn, opAt(fr.Min))
	fl := txtW("?")
	text.Draw(screen, "?", uiFace, fr.Min.X+(fr.Dx()-fl)/2, fr.Min.Y+12, colText)

	if hover {
		tw := cr.Dx() + 40
		if g.closeTip == nil || g.closeTip.Bounds().Dx() != tw {
			g.closeTip = frameImage(tw, 16, colDim)
		}
		tr := g.closeAppTipRect()
		screen.DrawImage(g.closeTip, opAt(tr.Min))
		lw := txtW(T("close.app"))
		drawTextOutlined(screen, tr.Min.X+(tr.Dx()-lw)/2, tr.Min.Y+11, T("close.app"), colText)
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func opAt(p image.Point) *ebiten.DrawImageOptions {
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(p.X), float64(p.Y))
	return op
}

func ptIn(x, y int, r image.Rectangle) bool {
	return x >= r.Min.X && x < r.Max.X && y >= r.Min.Y && y < r.Max.Y
}

// drawGear — шестерёнка на правом краю (ярче при открытых настройках).
func (g *Game) drawGear(screen *ebiten.Image) {
	op := opAt(g.gearRect().Min)
	op.ColorScale.ScaleAlpha(0.55)
	if g.settings.open {
		op.ColorScale.ScaleAlpha(1.8) // компенсация: итог 1.0
	}
	screen.DrawImage(g.gear, op)
}

// pointX — Win32 POINT.
type pointX struct{ X, Y int32 }

// cursorScreenPos — позиция курсора в экранных координатах (для ресайза).
func cursorScreenPos() (int, int) {
	var p pointX
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return int(p.X), int(p.Y)
}

// clipboardText — текст из буфера обмена (CF_UNICODETEXT).
func clipboardText() string {
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return ""
	}
	defer procCloseClipboard.Call()
	h, _, _ := syscall.SyscallN(procGetClipboardData.Addr(), 13) // CF_UNICODETEXT
	if h == 0 {
		return ""
	}
	size, _, _ := syscall.SyscallN(procGlobalSize.Addr(), h) // размер блока
	if size == 0 {
		return ""
	}
	if size > 8192 {
		size = 8192
	}
	buf := make([]uint16, size/2)
	// Копирование по uintptr-адресу — без uintptr->Pointer конверсий.
	syscall.SyscallN(procRtlMoveMemory.Addr(),
		uintptr(unsafe.Pointer(unsafe.SliceData(buf))), h, size)
	for i, r := range buf {
		if r == 0 {
			return strings.TrimRight(syscall.UTF16ToString(buf[:i]), "\r\n")
		}
	}
	return strings.TrimRight(syscall.UTF16ToString(buf), "\r\n")
}
