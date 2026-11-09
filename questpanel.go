package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// --- Геометрия панели Quest Helper (второе F9-окно внутри оверлея). ---

const (
	qhFilterTop = 54  // строка фильтра фракций
	qhLogTop    = 82  // строка папки логов
	qhListTop   = 112 // верх списка квестов
	qhRowH      = 18  // высота строки квеста
	qhFbtnW     = 46  // ширина кнопки фильтра
)

// qhOpen — панель видима (переключается F4/кнопкой в настройках).
func (g *Game) qhOpen() bool { return g.qhelper }

// toggleQuestPanel — F4: открыть/закрыть Quest Helper (взаимоисключимо с F9).
// при активном фарме F4 — быстрый выключатель: маршрут сбрасывается
// и открываются квесты (иначе фарм невозможно выключить из узкого окна).
func (g *Game) toggleQuestPanel() {
	if g.qhelper {
		g.closeQuestPanel()
		return
	}
	if g.farmActive() {
		g.farmRoute = nil
		g.setFarmMsg(T("farm.qon"))
	}
	if g.settings.open {
		g.closeSettings()
	}
	if g.fhelper {
		g.closeFarmPanel()
	}
	if g.faqOpen {
		g.closeFAQ()
	}
	g.qhelper = true
	ebiten.SetWindowMousePassthrough(false)
}

func (g *Game) closeQuestPanel() {
	procReleaseCapture.Call()
	g.qhelper = false
	// Панели закрыты — карта снова клики-насквозь (F9 открыт быть не может:
	// панели взаимоисключимы).
	ebiten.SetWindowMousePassthrough(true)
	g.saveWindowGeometry()
}

// qhListRect — зона списка квестов; при выбранном квесте низ отдан подсказкам.
func (g *Game) qhListRect() image.Rectangle {
	bottom := g.h() - 44
	if g.qhSel != "" {
		bottom = g.qhHintsRect().Min.Y
	}
	return image.Rect(padX, qhListTop, g.w()-padX, bottom)
}

// qhHintsRect — блок подсказок выбранного квеста: до ~45% высоты окна,
// чтобы длинные цели и список ключей читались без обрезки.
func (g *Game) qhHintsRect() image.Rectangle {
	hh := g.h() * 45 / 100
	if hh < 150 {
		hh = 150
	}
	if hh > 340 {
		hh = 340
	}
	return image.Rect(padX, g.h()-44-hh, g.w()-padX, g.h()-44)
}

// qhFilterRect — i-я кнопка фильтра фракций (в строке «Filtr:»).
func (g *Game) qhFilterRect(i int) image.Rectangle {
	x := padX + 44 + i*(qhFbtnW+6)
	return image.Rect(x, qhFilterTop, x+qhFbtnW, qhFilterTop+btnH)
}

// qhLogBtnRect — кнопка выбора папки логов (справа в строке логов).
func (g *Game) qhLogBtnRect() image.Rectangle {
	return image.Rect(g.w()-padX-btnW, qhLogTop-4, g.w()-padX, qhLogTop+btnH-4)
}

// qhScanRect — кнопка «Skan istorii» (слева от «Obzor...»): разовый проход по
// последним сессиям log_* — все «quest started» из истории станут tracked.
func (g *Game) qhScanRect() image.Rectangle {
	return image.Rect(g.w()-padX-btnW*2-6, qhLogTop-4, g.w()-padX-btnW-6, qhLogTop+btnH-4)
}

// qhAllRect — переключатель списка: только tracked / все квесты карты
// (справа в строке заголовка карты).
func (g *Game) qhAllRect() image.Rectangle {
	return image.Rect(g.w()-padX-btnW, 20, g.w()-padX, 20+btnH)
}

// qhCloseRect — кнопка закрытия панели.
func (g *Game) qhCloseRect() image.Rectangle {
	return image.Rect(padX, g.h()-36, padX+btnW, g.h()-10)
}

// qhClearRect — кнопка «снять все галочки».
func (g *Game) qhClearRect() image.Rectangle {
	return image.Rect(padX+btnW+10, g.h()-36, padX+btnW*2+10, g.h()-10)
}

var qhFactions = []string{"Any", "BEAR", "USEC"}

// questRow — строка списка квестов.
type questRow struct {
	id     string // questId ("" — заголовок секции)
	name   string
	trader string
	color  color.RGBA
	auto   bool   // активирован из лога (AUTO)
	note   string // подпись: точек/уровень
	mapKey string // карта, где есть точки ("" — вне текущей карты)
}

// qhRows — содержимое списка. По умолчанию — только tracked-квесты (карты +
// вне карты); кнопкой «Vse» раскрывается весь список квестов карты.
func (g *Game) qhRows() []questRow {
	key := g.currentMapKey()
	fac := g.cfg.QuestFaction
	tracked := map[string]bool{}
	if g.qw != nil {
		for _, id := range g.qw.Tracked() {
			tracked[id] = true
		}
	}

	var rows []questRow
	add := func(q Quest, mapKey string) {
		if fac != "Any" && q.Faction != fac && q.Faction != "Any" {
			return
		}
		rows = append(rows, questRow{
			id: q.ID, name: g.md.Loc(q.Name), trader: traderName(q.Trader),
			color:  g.md.QuestColor(mapKey, q.ID),
			auto:   tracked[q.ID] && g.qwAuto(q.ID),
			note:   T("q.note", traderName(q.Trader), q.MinLevel, len(q.Points)),
			mapKey: mapKey,
		})
	}
	if key != "" {
		for _, q := range g.md.QuestsFor(key) {
			if tracked[q.ID] {
				add(q, key)
			}
		}
	}
	if g.cfg.QuestShowAll && key != "" {
		for _, q := range g.md.QuestsFor(key) {
			if !tracked[q.ID] {
				add(q, key)
			}
		}
	}
	if g.qw != nil {
		for _, id := range g.qw.Tracked() {
			if _, ok := g.md.QuestByID(id); !ok {
				rows = append(rows, questRow{
					id: id, name: questFallbackName(id), trader: "???",
					color: color.RGBA{170, 170, 170, 255},
					auto:  true, mapKey: "",
				})
			}
		}
	}
	return rows
}

// qwAuto — квест попал в tracked через лог (помечен в autoIDs).
func (g *Game) qwAuto(id string) bool {
	return g.qw != nil && g.qw.IsAuto(id)
}

// clampQHScroll — смещение списка в допустимых пределах.
func (g *Game) clampQHScroll(scroll, rows int) int {
	visible := g.qhListRect().Dy() / qhRowH
	max := rows - visible
	if max < 0 {
		max = 0
	}
	return clampI(scroll, 0, max)
}

// updateQuestPanel — drag/ресайз окна, скролл и клики панели квестов.
func (g *Game) updateQuestPanel() {
	mx, my := ebiten.CursorPosition()
	press := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	release := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if press {
		g.downX, g.downY, g.downOn = mx, my, true
	}
	clickIn := func(r image.Rectangle) bool {
		return release && g.downOn && ptIn(g.downX, g.downY, r) && ptIn(mx, my, r)
	}

	// Общий «каркас окна»: ресайз за грип и перенос за полосу (как в F9).
	if g.handleWindowChrome(mx, my, press, held) {
		return
	}

	// Колесо: над подсказками — прокрутка подсказок, иначе — списка.
	if _, dy := ebiten.Wheel(); dy != 0 {
		if g.qhSel != "" && ptIn(mx, my, g.qhHintsRect()) {
			g.qhHintScroll -= int(dy)
		} else {
			g.qhScroll = g.clampQHScroll(g.qhScroll-int(dy), len(g.qhRows()))
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closeQuestPanel()
		return
	}

	switch {
	case clickIn(g.qhCloseRect()):
		g.closeQuestPanel()
	case clickIn(g.qhClearRect()):
		if g.qw != nil {
			for _, id := range g.qw.Tracked() {
				g.qw.SetTracked(id, false)
			}
		}
	case clickIn(g.qhLogBtnRect()):
		if path, ok := FolderDialog(); ok && g.qw != nil {
			g.qw.SetDir(path)
			g.cfg.LogsDir = path
			g.saveCfg()
		}
	case clickIn(g.qhScanRect()):
		if g.qw != nil {
			if n := g.qw.ScanHistory(); n > 0 {
				log.Printf("quest: skan istorii: +%d", n)
			}
		}
	case clickIn(g.qhAllRect()):
		g.cfg.QuestShowAll = !g.cfg.QuestShowAll
		g.saveCfg()
	default:
		for i, f := range qhFactions {
			if clickIn(g.qhFilterRect(i)) {
				g.cfg.QuestFaction = f
				g.saveCfg()
				break
			}
		}
		if release && g.downOn {
			if row := g.qhRowAt(g.downX, g.downY); row != nil && ptIn(mx, my, g.qhRowZone(*row)) && row.id != "" && g.qw != nil {
				if g.downX <= g.qhListRect().Min.X+16 { // чекбокс — вкл/выкл трекинга
					g.qw.SetTracked(row.id, !g.qw.IsTracked(row.id))
				} else if g.qhSel == row.id { // строка — выбор квеста (подсказки)
					g.qhSel = "" // повторный клик — скрыть
				} else {
					g.qhSel = row.id
				}
			}
		}
	}
}

// qhRowAt — строка списка под точкой (nil — мимо списка).
func (g *Game) qhRowAt(x, y int) *questRow {
	r := g.qhListRect()
	if !ptIn(x, y, r) {
		return nil
	}
	rows := g.qhRows()
	scroll := g.clampQHScroll(g.qhScroll, len(rows))
	i := (y-r.Min.Y)/qhRowH + scroll
	if i < 0 || i >= len(rows) {
		return nil
	}
	return &rows[i]
}

// qhRowZone — клик-зона строки (чекбокс+имя).
func (g *Game) qhRowZone(row questRow) image.Rectangle {
	r := g.qhListRect()
	scroll := g.clampQHScroll(g.qhScroll, len(g.qhRows()))
	idx := g.qhRowIndex(row)
	if idx < 0 {
		return image.Rectangle{}
	}
	y := r.Min.Y + (idx-scroll)*qhRowH
	return image.Rect(r.Min.X, y, r.Max.X, y+qhRowH)
}

// qhRowIndex — позиция строки в текущем наборе.
func (g *Game) qhRowIndex(row questRow) int {
	rows := g.qhRows()
	for i := range rows {
		if rows[i].id == row.id && rows[i].name == row.name {
			return i
		}
	}
	return -1
}

// handleWindowChrome — общий каркас интерактивного окна: ресайз за грип
// (правый нижний угол) и перенос за верхнюю полосу. Возвращает true, если
// событие поглощено каркасом (клики панели в этот тик не обрабатываются).
func (g *Game) handleWindowChrome(mx, my int, press, held bool) bool {
	// Ресайз за грип.
	if !g.resizing && press && ptIn(mx, my, g.gripRect()) {
		g.resizing = true
		g.resStartX, g.resStartY = cursorScreenPos()
		g.resStartW, g.resStartH = ebiten.WindowSize()
	}
	if g.resizing {
		if held {
			cx, cy := cursorScreenPos()
			sw, sh := ebiten.ScreenSizeInFullscreen()
			nw := clampI(g.resStartW+cx-g.resStartX, minWinW, sw)
			nh := clampI(g.resStartH+cy-g.resStartY, minWinH, sh)
			// Ставим размер только при фактическом изменении — не штормим окно.
			if cw, ch := ebiten.WindowSize(); cw != nw || ch != nh {
				ebiten.SetWindowSize(nw, nh)
			}
		} else {
			g.resizing = false
			g.saveWindowGeometry()
		}
		return true
	}

	// Перемещение окна за верхнюю полосу — собственный drag:
	// пока зажата ЛКМ на полосе, тянем окно за курсор (SetWindowPosition).
	// Синтетический WM_NCLBUTTONDOWN в этом окне не работает.
	if !g.dragging && press && my <= dragBarH {
		g.dragging = true
		cx, cy := cursorScreenPos()
		wx, wy := ebiten.WindowPosition()
		g.dragOffX, g.dragOffY = wx-cx, wy-cy
	}
	if g.dragging {
		if held {
			cx, cy := cursorScreenPos()
			sw, sh := ebiten.ScreenSizeInFullscreen()
			w, _ := ebiten.WindowSize()
			nx := clampI(cx+g.dragOffX, -w+60, sw-60)
			ny := clampI(cy+g.dragOffY, 0, sh-60)
			if px, py := ebiten.WindowPosition(); px != nx || py != ny {
				ebiten.SetWindowPosition(nx, ny)
			}
		} else {
			g.dragging = false
			g.saveWindowGeometry()
		}
		return true
	}
	return false
}

// drawQuestPanel — панель Quest Helper: переключатель Moi/Vse, фильтр
// фракций, папка логов (+скан истории), список квестов с чекбоксами трекинга.
func (g *Game) drawQuestPanel(screen *ebiten.Image) {
	w, h := g.w(), g.h()
	screen.DrawImage(g.panel, &ebiten.DrawImageOptions{})

	// Полоса перетаскивания.
	text.Draw(screen, T("q.title"), uiFace, padX, 19, colText)
	hint := T("drag.hint")
	text.Draw(screen, hint, uiFace, w-8-txtW(hint), 19, colDim)
	for x := 4; x < w-4; x += 6 {
		screen.Set(x, dragBarH, colDim)
		screen.Set(x+1, dragBarH, colDim)
	}

	// Карта и счётчик квестов + переключатель списка (Moi/Vse).
	key := g.currentMapKey()
	all := g.md.QuestsFor(key)
	allLabel := T("q.mine")
	if g.cfg.QuestShowAll {
		allLabel = T("q.all")
	}
	drawButton(screen, g.frameBtn, g.qhAllRect(), fmt.Sprintf("%s (%d)", allLabel, len(all)), colDim)
	text.Draw(screen, T("q.map", g.mapName(key), len(all)), uiFace, padX, 38, colDim)

	// Фильтр фракций.
	text.Draw(screen, T("q.filter"), uiFace, padX, qhFilterTop+8, colDim)
	for i, f := range qhFactions {
		r := g.qhFilterRect(i)
		c := colDim
		if g.cfg.QuestFaction == f {
			c = colScav
		}
		drawButton(screen, g.frameBtn, r, f, c)
	}

	// Строка папки логов + кнопки скана истории и выбора.
	text.Draw(screen, T("q.logs"), uiFace, padX, qhLogTop+8, colDim)
	logStr := g.cfg.LogsDir
	if logStr == "" {
		logStr = T("q.nologs")
	}
	if maxChars := (w - padX*2 - btnW*2 - 22) / charW; utf8.RuneCountInString(logStr) > maxChars {
		logStr = clipTail(logStr, maxChars)
	}
	text.Draw(screen, logStr, uiFace, padX, qhLogTop+btnH+4, colText)
	drawButton(screen, g.frameBtn, g.qhScanRect(), T("btn.scan"), colText)
	drawButton(screen, g.frameBtn, g.qhLogBtnRect(), T("btn.browse"), colText)

	// Список квестов + подсказки выбранного квеста.
	g.drawQuestList(screen)
	g.drawQuestHints(screen)

	// Кнопки низа.
	drawButton(screen, g.frameBtn, g.qhCloseRect(), T("btn.close"), colText)
	drawButton(screen, g.frameBtn, g.qhClearRect(), T("btn.reset"), colDim)
	tip := T("q.tip")
	if max := (w - padX*2 - btnW*2 - 24) / charW; max > 4 {
		tip = truncRunes(tip, max)
	}
	text.Draw(screen, tip, uiFace, padX+btnW*2+18, g.h()-20, colDim)

	// Грип ресайза.
	screen.DrawImage(g.grip, opAt(image.Point{X: w - gripS, Y: h - gripS}))
}

// drawQuestList — строки квестов: чекбокс, цветной свотч, имя, AUTO, trader.
func (g *Game) drawQuestList(screen *ebiten.Image) {
	rows := g.qhRows()
	r := g.qhListRect()
	scroll := g.clampQHScroll(g.qhScroll, len(rows))
	visible := r.Dy() / qhRowH
	for i := scroll; i < len(rows) && i < scroll+visible; i++ {
		row := rows[i]
		y := r.Min.Y + (i-scroll)*qhRowH
		// Выделение выбранного квеста (рамка).
		if row.id != "" && row.id == g.qhSel {
			fr := row.color
			fr.A = 200
			drawLine(screen, float64(r.Min.X), float64(y+1), float64(r.Max.X), float64(y+1), fr)
			drawLine(screen, float64(r.Min.X), float64(y+qhRowH-2), float64(r.Max.X), float64(y+qhRowH-2), fr)
		}
		// Чекбокс.
		box := image.Rect(r.Min.X, y+3, r.Min.X+12, y+15)
		screen.DrawImage(g.frameBox, opAt(box.Min))
		if g.qw != nil && g.qw.IsTracked(row.id) {
			screen.DrawImage(g.check, opAt(box.Min.Add(image.Point{3, 3})))
		}
		// Цвет квеста.
		swatch := image.Rect(r.Min.X+18, y+4, r.Min.X+28, y+14)
		op := opAt(swatch.Min)
		op.ColorScale.ScaleWithColor(row.color)
		screen.DrawImage(g.dot, op)
		// Кнопка-подсказка «i» справа: заметный аффорданс выбора квеста.
		if row.id != "" {
			ib := image.Point{X: r.Max.X - 16, Y: y + 3}
			screen.DrawImage(g.frameBox, opAt(ib))
			if row.id == g.qhSel {
				fill := opAt(ib.Add(image.Point{3, 3}))
				fill.ColorScale.ScaleWithColor(row.color)
				screen.DrawImage(g.check, fill)
				text.Draw(screen, "i", uiFace, ib.X+2, ib.Y+11, colRing)
			} else {
				text.Draw(screen, "i", uiFace, ib.X+2, ib.Y+11, colDim)
			}
		}
		// AUTO + имя (руно-безопасная обрезка: кириллица).
		name := row.name
		suffix := ""
		if row.auto {
			suffix = T("q.auto")
		}
		lx := r.Min.X + 34
		maxName := (r.Max.X - lx - 126) / charW
		if maxName < 8 {
			maxName = 8
		}
		if utf8.RuneCountInString(name)+utf8.RuneCountInString(suffix) > maxName {
			cut := maxName - utf8.RuneCountInString(suffix) - 2
			if cut < 4 {
				cut = 4
			}
			name = truncRunes(name, cut)
		}
		col := colText
		if row.mapKey == "" {
			col = colDim // tracked вне текущей карты
		}
		text.Draw(screen, name+suffix, uiFace, lx, y+13, col)
		text.Draw(screen, row.trader, uiFace, r.Max.X-86, y+13, colDim)
	}
	if len(rows) == 0 {
		text.Draw(screen, T("q.none"), uiFace, r.Min.X+4, r.Min.Y+16, colDim)
	}
}

// --- Маркеры отслеживаемых квестов на карте ---

// drawLine — 1px линия (для рамок visit-зон).
func drawLine(img *ebiten.Image, x0, y0, x1, y1 float64, col color.Color) {
	steps := int(math.Hypot(x1-x0, y1-y0)) + 1
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		img.Set(int(x0+t*(x1-x0)), int(y0+t*(y1-y0)), col)
	}
}

// drawQuestMarkers — tracked-квесты текущей карты: item-точки, visit-зоны
// (полупрозрачный контур + рамка), подписи в цвете квеста. Чужой этаж —
// тусклее и с ^/v (как у выходов).
func (g *Game) drawQuestMarkers(screen *ebiten.Image, key string, b [2][2]float64, iw, ih int, gm ebiten.GeoM, playerY float64) {
	if g.qw == nil || g.farmActive() { // фарм активен — квестовые маркеры скрыты
		return
	}
	tracked := map[string]bool{}
	for _, id := range g.qw.Tracked() {
		tracked[id] = true
	}
	// Слоя контуров: чистится и переиспользуется между кадрами.
	w, h := g.w(), g.h()
	if g.qhOverlay == nil || g.qhOverlay.Bounds().Dx() != w || g.qhOverlay.Bounds().Dy() != h {
		g.qhOverlay = ebiten.NewImage(w, h)
	}
	g.qhOverlay.Clear()

	for _, q := range g.md.QuestsFor(key) {
		if !tracked[q.ID] {
			continue
		}
		qc := g.md.QuestColor(key, q.ID)
		for i := range q.Points {
			p := &q.Points[i]
			u, v := WorldToUV(b, p.X, p.Z)
			sx, sy := gm.Apply(u*float64(iw), v*float64(ih))
			if sx < -80 || sx > float64(w)+80 || sy < -20 || sy > float64(h)+20 {
				continue
			}
			col, arrow := g.exitStyle(qc, p.Y, playerY)
			label := arrow + g.md.Loc(q.Name)
			if utf8.RuneCountInString(label) > 30 {
				label = truncRunes(label, 30)
			}
			if p.Kind == "visit" && len(p.Outline) >= 3 {
				pts := make([][2]float64, len(p.Outline))
				in := false
				minx, miny, maxx, maxy := 1e9, 1e9, -1e9, -1e9
				for j, o := range p.Outline {
					ou, ov := WorldToUV(b, o[0], o[1])
					pts[j][0], pts[j][1] = gm.Apply(ou*float64(iw), ov*float64(ih))
					minx, miny = math.Min(minx, pts[j][0]), math.Min(miny, pts[j][1])
					maxx, maxy = math.Max(maxx, pts[j][0]), math.Max(maxy, pts[j][1])
				}
				in = maxx >= 0 && minx <= float64(w) && maxy >= 0 && miny <= float64(h)
				if in {
					fill := col
					fill.A = 36
					fillPoly(g.qhOverlay, pts, fill, 0)
					frame := col
					frame.A = 200
					for j := range pts {
						a, c := pts[j], pts[(j+1)%len(pts)]
						drawLine(g.qhOverlay, a[0], a[1], c[0], c[1], frame)
					}
					g.drawDot(screen, sx, sy, col)
					g.questLabel(screen, sx, sy, label, col)
				}
				continue
			}
			g.drawDot(screen, sx, sy, col)
			g.questLabel(screen, sx, sy, label, col)
		}
	}

	// Двери-замки для ключей отслеживаемых квестов: ромб + имя ключа.
	need := map[string]string{} // имя ключа -> questId (цвет маркера)
	for _, q := range g.md.QuestsFor(key) {
		if !tracked[q.ID] {
			continue
		}
		for _, kname := range q.Keys {
			need[kname] = q.ID
		}
	}
	for _, l := range g.md.LocksFor(key) {
		qid, ok := need[l.Key]
		if !ok {
			continue
		}
		u, v := WorldToUV(b, l.X, l.Z)
		sx, sy := gm.Apply(u*float64(iw), v*float64(ih))
		if sx < -80 || sx > float64(w)+80 || sy < -20 || sy > float64(h)+20 {
			continue
		}
		col, arrow := g.exitStyle(g.md.QuestColor(key, qid), l.Y, playerY)
		drawLock(screen, sx, sy, col)
		label := arrow + "K|" + g.md.Loc(l.Key)
		if utf8.RuneCountInString(label) > 30 {
			label = truncRunes(label, 30)
		}
		g.questLabel(screen, sx, sy, label, col)
	}
	screen.DrawImage(g.qhOverlay, &ebiten.DrawImageOptions{})
}

// drawLock — ромб-маркер замка (дверь, открываемая ключом квеста).
func drawLock(screen *ebiten.Image, sx, sy float64, col color.RGBA) {
	const s = 5.0
	drawLine(screen, sx, sy-s, sx+s, sy, col)
	drawLine(screen, sx+s, sy, sx, sy+s, col)
	drawLine(screen, sx, sy+s, sx-s, sy, col)
	drawLine(screen, sx-s, sy, sx, sy-s, col)
}

// questLabel — подпись квест-точки (в цвет квеста, с обводкой).
func (g *Game) questLabel(screen *ebiten.Image, sx, sy float64, label string, col color.RGBA) {
	lw := txtW(label)
	tx := int(sx) + 9
	if tx+lw > g.w()-2 {
		tx = int(sx) - 9 - lw
	}
	if tx < 2 {
		tx = 2
	}
	ty := int(sy) - 3
	if ty < 10 {
		ty = int(sy) + 13
	}
	drawTextOutlined(screen, tx, ty, label, col)
}

// hintLine — строка блока подсказок с цветом.
type hintLine struct {
	s string
	c color.RGBA
}

// drawQuestHints — блок подсказок выбранного квеста: цели (точки на карте),
// нужные ключи (с отметкой двери на карте), полные описания целей, wiki.
// Прокрутка — колесо мыши над блоком. Выбор — клик по строке списка.
func (g *Game) drawQuestHints(screen *ebiten.Image) {
	if g.qhSel == "" {
		return
	}
	r := g.qhHintsRect()

	// Плотный тёмный фон повышает контраст текста поверх карты.
	bgSize := image.Point{X: r.Dx() + 4, Y: r.Dy() + 4}
	if g.qhHintBg == nil || g.qhHintBg.Bounds().Dx() != bgSize.X || g.qhHintBg.Bounds().Dy() != bgSize.Y {
		g.qhHintBg = fillImage(bgSize.X, bgSize.Y, color.RGBA{10, 12, 16, 240})
	}
	screen.DrawImage(g.qhHintBg, opAt(image.Point{X: r.Min.X - 2, Y: r.Min.Y - 2}))
	fr := color.RGBA{96, 96, 96, 255}
	drawLine(screen, float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Min.Y), fr)
	drawLine(screen, float64(r.Min.X), float64(r.Max.Y-1), float64(r.Max.X), float64(r.Max.Y-1), fr)

	maxChars := (r.Dx() - 14) / charW
	if maxChars < 20 {
		maxChars = 20
	}
	q, ok := g.md.QuestByID(g.qhSel)
	if !ok {
		text.Draw(screen, questFallbackName(g.qhSel), uiFace, r.Min.X+6, r.Min.Y+16, colDim)
		text.Draw(screen, T("q.missing"), uiFace, r.Min.X+6, r.Min.Y+32, colDim)
		return
	}

	lines := make([]hintLine, 0, 64)
	sec := func(s string) { lines = append(lines, hintLine{s, color.RGBA{200, 205, 215, 255}}) }
	dim := func(s string) { lines = append(lines, hintLine{s, colDim}) }
	wrap := func(s, prefix, cont string, c color.RGBA) {
		for i, w := range wrapText(s, maxChars-2) {
			if i == 0 {
				lines = append(lines, hintLine{prefix + w, c})
			} else {
				lines = append(lines, hintLine{cont + w, colDim})
			}
		}
	}

	// Шапка: имя квеста (цвет квеста) + трейдер/уровень.
	key := g.currentMapKey()
	qc := g.md.QuestColor(key, q.ID)
	wrap(g.md.Loc(q.Name)+" — "+traderName(q.Trader)+", lvl "+fmt.Sprint(q.MinLevel), "", "  ", qc)

	// Цели на этой карте: точки с тегами этажей.
	sec(T("q.goals"))
	floored := len(g.md.Floors(key)) > 1
	if len(q.Points) == 0 {
		dim(T("q.nopoints"))
	}
	for _, p := range q.Points {
		label := g.md.Loc(p.Label)
		if label == "" {
			label = p.Kind
		}
		fl := ""
		if floored {
			if kf := g.md.FloorFor(key, p.Y, ""); kf != "" {
				fl = " [" + floorShort(kf) + "]"
			}
		}
		wrap(label+fl, "- ", "  ", colText)
	}

	// Ключи: название + этаж двери, если замок есть на этой карте.
	if len(q.Keys) > 0 {
		sec(T("q.keys"))
		locks := g.md.LocksFor(key)
		for _, kname := range q.Keys {
			note := ""
			for _, l := range locks {
				if l.Key != kname {
					continue
				}
				note = T("q.door")
				if fl := g.md.FloorFor(key, l.Y, ""); fl != "" && floored {
					note += " [" + floorShort(fl) + "]"
				}
				break
			}
			wrap(g.md.Loc(kname)+note, "- ", "  ", colText)
		}
	}

	// Полные описания всех целей (в т.ч. без точек на карте).
	if len(q.Objectives) > 0 {
		sec(T("q.detail"))
		for _, o := range q.Objectives {
			wrap(g.md.Loc(o), "- ", "  ", colText)
		}
	}
	if q.Wiki != "" {
		dim("WIKI: " + q.Wiki)
	}

	// Рендер с прокруткой (15px межстрочник) + мини-скроллбар.
	const lineH = 15
	visible := (r.Dy() - 20) / lineH
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	g.qhHintScroll = clampI(g.qhHintScroll, 0, maxScroll)
	for i := g.qhHintScroll; i < len(lines) && i < g.qhHintScroll+visible; i++ {
		text.Draw(screen, lines[i].s, uiFace, r.Min.X+6, r.Min.Y+16+(i-g.qhHintScroll)*lineH, lines[i].c)
	}
	if maxScroll > 0 {
		trackTop, trackBot := float64(r.Min.Y+12), float64(r.Max.Y-12)
		drawLine(screen, float64(r.Max.X-2), trackTop, float64(r.Max.X-2), trackBot, colRing)
		thumbH := (trackBot - trackTop) * float64(visible) / float64(len(lines))
		thumbTop := trackTop + (trackBot-trackTop-thumbH)*float64(g.qhHintScroll)/float64(maxScroll)
		drawLine(screen, float64(r.Max.X-2), thumbTop, float64(r.Max.X-2), thumbTop+thumbH, colDim)
	}
}

// wrapText — перенос по словам на строки шириной maxChars (рун, px=рун*charW).
func wrapText(s string, maxChars int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, len(s)/maxChars+1)
	cur := words[0]
	curW := txtW(cur)
	for _, w := range words[1:] {
		ww := txtW(w)
		if curW+charW+ww <= maxChars*charW {
			cur += " " + w
			curW += charW + ww
			continue
		}
		lines = append(lines, cur)
		cur, curW = w, ww
	}
	return append(lines, cur)
}
