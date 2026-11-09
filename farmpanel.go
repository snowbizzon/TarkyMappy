package main

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// --- Панель Помощника фарма (F3) и отрисовка маршрута на карте. ---

const (
	fhTop     = 40  // строка «Karta» (+14 позиция, +28 предупреждение о карте)
	fhGridY   = 76  // верх сетки чекбоксов (2 строки x 3 колонки)
	fhRowH    = 20  // шаг строк чекбоксов
	fhExitY   = 118 // строка выбора выхода
	fhSliderY = 146 // слайдер «Punkty» (число точек маршрута)
	fhBtnY    = 172 // кнопки Generirovat / Novyi (Sbros — внизу у Zakryt)
	fhHintY   = 204 // верх блока маршрута
)

// farmActive — фарм включён (маршрут существует): квесты недоступны.
func (g *Game) farmActive() bool { return g.farmRoute != nil }

// setFarmMsg — статус/ошибка фарма (панель + плашка на карте, автосброс 3с).
func (g *Game) setFarmMsg(s string) {
	g.farmMsg, g.farmMsgAt = s, g.frame
}

// toggleFarm — F3: открыть/закрыть Помощник фарма (взаимоисключимо с F9/F4).
func (g *Game) toggleFarm() {
	if g.fhelper {
		g.closeFarmPanel()
		return
	}
	if g.settings.open {
		g.closeSettings()
	}
	if g.qhelper {
		g.closeQuestPanel()
	}
	if g.faqOpen {
		g.closeFAQ()
	}
	g.fhelper = true
	ebiten.SetWindowMousePassthrough(false)
}

func (g *Game) closeFarmPanel() {
	procReleaseCapture.Call()
	g.fhelper = false
	ebiten.SetWindowMousePassthrough(true)
	g.saveWindowGeometry()
}

// --- Геометрия панели. ---

// fhCbRect — зона чекбокса (row, col): 5 категорий + «Etazhi» в ячейке (1,2).
func (g *Game) fhCbRect(row, col int) image.Rectangle {
	colW := (g.w() - padX*2) / 3
	x := padX + col*colW
	y := fhGridY + row*fhRowH
	return image.Rect(x, y, x+colW-6, y+fhRowH)
}

// fhExitLabel — подпись кнопки выхода: имя, авто/выбор и дистанция от
// текущей позиции; имя выхода — RU через md.Loc.
func (g *Game) fhExitLabel() string {
	r := g.farmRoute
	var s string
	if r == nil {
		s = T("farm.exitnone")
	} else {
		tag := T("farm.auto")
		if !r.ExitAuto {
			tag = T("farm.pick")
		}
		d := 0
		if x, _, z, _, ok := g.st.Pos(); ok {
			d = int(math.Hypot(x-r.Exit.X, z-r.Exit.Z))
		} else {
			d = int(math.Hypot(r.Origin[0]-r.Exit.X, r.Origin[2]-r.Exit.Z))
		}
		s = T("farm.exitfmt", g.md.Loc(r.Exit.Name), tag, d)
	}
	maxW := g.w() - padX*2 - 110
	if maxW < 80 {
		maxW = 80
	}
	if bw := txtW(s) + 16; bw > maxW {
		n := (maxW - 16) / charW
		if n < 6 {
			n = 6
		}
		s = truncRunes(s, n)
	}
	return s
}

// fhExitRect — кнопка цикла выхода (справа в своей строке).
func (g *Game) fhExitRect() image.Rectangle {
	bw := txtW(g.fhExitLabel()) + 16
	return image.Rect(g.w()-padX-bw, fhExitY, g.w()-padX, fhExitY+btnH)
}

// fhSliderRect — трек слайдера «Punkty» (5..35 точек маршрута).
func (g *Game) fhSliderRect() image.Rectangle {
	return image.Rect(padX+74, fhSliderY+6, g.w()-padX-4, fhSliderY+16)
}

// fhSliderZone — расширенная клик-зона слайдера.
func (g *Game) fhSliderZone() image.Rectangle {
	r := g.fhSliderRect()
	return image.Rect(r.Min.X-10, fhSliderY-3, r.Max.X+10, fhSliderY+21)
}

// fhSliderValue — значение слайдера (5..35) по x курсора.
func (g *Game) fhSliderValue(mx int) int {
	r := g.fhSliderRect()
	t := float64(mx-r.Min.X) / float64(max(1, r.Dx()))
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return 5 + int(t*30+0.5)
}

// fhSliderX — x ручки слайдера для значения v.
func (g *Game) fhSliderX(v int) float64 {
	r := g.fhSliderRect()
	return float64(r.Min.X) + float64(r.Dx())*float64(v-5)/30
}

func (g *Game) fhGenRect() image.Rectangle {
	// 112px — влезает «Сгенерировать» при кириллическом шрифте.
	return image.Rect(padX, fhBtnY, padX+112, fhBtnY+btnH)
}

func (g *Game) fhNewRect() image.Rectangle {
	return image.Rect(padX+118, fhBtnY, padX+118+152, fhBtnY+btnH)
}

func (g *Game) fhResetRect() image.Rectangle {
	// Кнопка «Сброс» стоит в нижнем ряду рядом с «Закрыть», поэтому
	// остаётся доступной даже в самом узком окне.
	return image.Rect(padX+btnW+6, g.h()-36, padX+btnW+68, g.h()-10)
}

// fhVisible — сколько строк маршрута видно в блоке.
func fhVisible(r image.Rectangle) int {
	v := (r.Dy() - 14) / 15
	if v < 1 {
		return 1
	}
	return v
}

// fhHintsRect — блок текста маршрута (до нижней строки кнопок).
func (g *Game) fhHintsRect() image.Rectangle {
	return image.Rect(padX, fhHintY, g.w()-padX, g.h()-44)
}

// fhCloseRect — кнопка закрытия панели.
func (g *Game) fhCloseRect() image.Rectangle {
	return image.Rect(padX, g.h()-36, padX+btnW, g.h()-10)
}

// updateFarmPanel — drag/ресайз окна, скролл и клики панели фарма.
func (g *Game) updateFarmPanel() {
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

	if g.handleWindowChrome(mx, my, press, held) {
		return
	}

	// Слайдер «Punkty»: зажал — тянешь, значение живое, сохранение при
	// отпускании.
	if press && ptIn(mx, my, g.fhSliderZone()) {
		g.fhSliderDrag = true
	}
	if g.fhSliderDrag {
		if v := g.fhSliderValue(mx); v != g.cfg.FarmStops {
			g.cfg.FarmStops = v
		}
		if release || !held {
			g.fhSliderDrag = false
			g.saveCfg()
		}
	}

	// Колесо — прокрутка блока маршрута.
	if _, dy := ebiten.Wheel(); dy != 0 {
		hr := g.fhHintsRect()
		lines := len(g.farmLines(hr.Dx() / charW))
		max := lines - fhVisible(hr)
		if max < 0 {
			max = 0
		}
		g.fhScroll = clampI(g.fhScroll-int(dy), 0, max)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closeFarmPanel()
		return
	}

	switch {
	case clickIn(g.fhCloseRect()):
		g.closeFarmPanel()
	case clickIn(g.fhGenRect()):
		g.setFarmMsg(g.farmGenerate(false))
	case clickIn(g.fhNewRect()):
		g.setFarmMsg(g.farmGenerate(true))
	case clickIn(g.fhResetRect()):
		g.farmRoute = nil
		g.setFarmMsg("")
	case clickIn(g.fhExitRect()):
		g.farmCycleExit()
	default:
		for row := 0; row < 2; row++ {
			for col := 0; col < 3; col++ {
				i := row*3 + col
				if !clickIn(g.fhCbRect(row, col)) {
					continue
				}
				if i < len(farmCats) {
					g.cfg.FarmCats ^= farmCatBit(i)
				} else {
					g.cfg.FarmFloors = !g.cfg.FarmFloors
				}
				g.saveCfg()
			}
		}
	}
}

// frameFor — рамка кнопки произвольного размера (кэш по габаритам).
func (g *Game) frameFor(w, h int) *ebiten.Image {
	if g.frames == nil {
		g.frames = map[image.Point]*ebiten.Image{}
	}
	key := image.Point{X: w, Y: h}
	if f, ok := g.frames[key]; ok {
		return f
	}
	f := frameImage(w, h, colText)
	g.frames[key] = f
	return f
}

// drawFarmPanel — панель фарма: карта/позиция, категории, этажи, выход,
// кнопки и блок маршрута.
func (g *Game) drawFarmPanel(screen *ebiten.Image) {
	w, h := g.w(), g.h()
	screen.DrawImage(g.panel, &ebiten.DrawImageOptions{})

	// Полоса перетаскивания.
	text.Draw(screen, T("farm.title"), uiFace, padX, 19, colGold)
	hint := T("drag.hint")
	text.Draw(screen, hint, uiFace, w-8-txtW(hint), 19, colDim)
	for x := 4; x < w-4; x += 6 {
		screen.Set(x, dragBarH, colDim)
		screen.Set(x+1, dragBarH, colDim)
	}

	// Карта и позиция (панель открывается до скриншота — без позиции
	// генерация недоступна).
	key := g.md.Wanted()
	x, y, z, _, ok := g.st.Pos()
	pos := T("farm.nopos")
	if ok {
		pos = T("farm.pos", fmtF(x), fmtF(y), fmtF(z))
	}
	text.Draw(screen, T("farm.map", g.mapName(key)), uiFace, padX, fhTop, colText)
	text.Draw(screen, pos, uiFace, padX, fhTop+14, colDim)
	if g.farmMapMismatch() { // маршрут для другой карты
		text.Draw(screen, T("farm.mismatch", g.mapName(g.farmRoute.MapKey)),
			uiFace, padX, fhTop+28, colGold)
	}

	// Категории + «Etazhi» (ячейка (1,2)).
	for row := 0; row < 2; row++ {
		for col := 0; col < 3; col++ {
			i := row*3 + col
			r := g.fhCbRect(row, col)
			if i < len(farmCats) {
				g.drawFarmCheck(screen, r, g.cfg.FarmCats&farmCatBit(i) != 0, T(farmCats[i].Name))
			} else {
				g.drawFarmCheck(screen, r, g.cfg.FarmFloors, T("farm.floors"))
			}
		}
	}

	// Выбор выхода.
	text.Draw(screen, T("farm.exitlbl"), uiFace, padX, fhExitY+9, colDim)
	er := g.fhExitRect()
	drawButton(screen, g.frameFor(er.Dx(), er.Dy()), er, g.fhExitLabel(), colGold)

	// Слайдер «Точки»: сколько точек в маршруте.
	v := farmStopsN(g.cfg.FarmStops)
	text.Draw(screen, T("farm.stops", v), uiFace, padX, fhSliderY+9, colGold)
	tr := g.fhSliderRect()
	midy := float64(tr.Min.Y+tr.Max.Y) / 2
	drawLine(screen, float64(tr.Min.X), midy, float64(tr.Max.X), midy, colRing)
	kx := g.fhSliderX(v)
	drawLine(screen, float64(tr.Min.X), midy, kx, midy, colGold)
	screen.DrawImage(g.frameFor(8, 14), opAt(image.Point{X: int(kx) - 4, Y: fhSliderY + 2}))

	// Кнопки действий.
	cGen := colText
	if !ok || key == "" {
		cGen = colDim // без позиции генерация недоступна
	}
	drawButton(screen, g.frameFor(g.fhGenRect().Dx(), btnH), g.fhGenRect(), T("btn.gen"), cGen)
	drawButton(screen, g.frameFor(g.fhNewRect().Dx(), btnH), g.fhNewRect(), T("btn.new"), colText)
	drawButton(screen, g.frameFor(g.fhResetRect().Dx(), btnH), g.fhResetRect(), T("btn.reset"), colText)

	// Блок маршрута.
	g.drawFarmHints(screen)

	// Низ: закрыть + грип ресайза.
	drawButton(screen, g.frameBtn, g.fhCloseRect(), T("btn.close"), colDim)
	screen.DrawImage(g.grip, opAt(image.Point{X: w - gripS, Y: h - gripS}))
}

// drawFarmCheck — чекбокс + подпись (золотым, если включён).
func (g *Game) drawFarmCheck(screen *ebiten.Image, r image.Rectangle, on bool, label string) {
	p := image.Point{X: r.Min.X, Y: r.Min.Y + 4}
	screen.DrawImage(g.frameBox, opAt(p))
	if on {
		screen.DrawImage(g.check, opAt(p.Add(image.Point{X: 3, Y: 3})))
	}
	c := colDim
	if on {
		c = colGold
	}
	text.Draw(screen, label, uiFace, r.Min.X+18, r.Min.Y+11, c)
}

// drawFarmHints — блок текста маршрута с прокруткой (колесо) и мини-скроллбаром.
func (g *Game) drawFarmHints(screen *ebiten.Image) {
	r := g.fhHintsRect()
	fr := color.RGBA{96, 96, 96, 255}
	drawLine(screen, float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Min.Y), fr)
	drawLine(screen, float64(r.Min.X), float64(r.Max.Y-1), float64(r.Max.X), float64(r.Max.Y-1), fr)

	lines := g.farmLines(r.Dx() / charW)
	const lineH = 15
	visible := fhVisible(r)
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	g.fhScroll = clampI(g.fhScroll, 0, maxScroll)
	for i := g.fhScroll; i < len(lines) && i < g.fhScroll+visible; i++ {
		text.Draw(screen, lines[i].s, uiFace, r.Min.X+6, r.Min.Y+16+(i-g.fhScroll)*lineH, lines[i].c)
	}
	if maxScroll > 0 {
		trackTop, trackBot := float64(r.Min.Y+12), float64(r.Max.Y-12)
		drawLine(screen, float64(r.Max.X-2), trackTop, float64(r.Max.X-2), trackBot, colRing)
		thumbH := (trackBot - trackTop) * float64(visible) / float64(len(lines))
		thumbTop := trackTop + (trackBot-trackTop-thumbH)*float64(g.fhScroll)/float64(maxScroll)
		drawLine(screen, float64(r.Max.X-2), thumbTop, float64(r.Max.X-2), thumbTop+thumbH, colDim)
	}
}

// drawFarmRoute — маршрут на карте: контуры кластеров, линии пути,
// пронумерованные точки (посещённые — тусклые), выбранный выход — кольцом.
func (g *Game) drawFarmRoute(screen *ebiten.Image, key string, b [2][2]float64, iw, ih int, gm ebiten.GeoM) {
	r := g.farmRoute
	if r == nil || r.MapKey != key {
		return
	}
	w, h := g.w(), g.h()
	uv := func(x, z float64) (float64, float64) {
		u, v := WorldToUV(b, x, z)
		return gm.Apply(u*float64(iw), v*float64(ih))
	}
	// Слой контуров кластеров (переиспользуемый оверлей).
	if g.qhOverlay == nil || g.qhOverlay.Bounds().Dx() != w || g.qhOverlay.Bounds().Dy() != h {
		g.qhOverlay = ebiten.NewImage(w, h)
	}
	g.qhOverlay.Clear()

	// Контур кластера: полупрозрачная заливка + рамка (тусклее — посещён).
	for i := range r.Stops {
		s := &r.Stops[i]
		sx, sy := uv(s.X, s.Z)
		if sx < -120 || sx > float64(w)+120 || sy < -120 || sy > float64(h)+120 || len(s.Hull) < 3 {
			continue
		}
		fill, frame := colGold, colGold
		if s.Visited {
			fill.A, frame.A = 12, 60
		} else {
			fill.A, frame.A = 26, 160
		}
		hp := make([][2]float64, len(s.Hull))
		for j, o := range s.Hull {
			hp[j][0], hp[j][1] = uv(o[0], o[1])
		}
		fillPoly(g.qhOverlay, hp, fill, 0)
		for j := range hp {
			a, c := hp[j], hp[(j+1)%len(hp)]
			drawLine(g.qhOverlay, a[0], a[1], c[0], c[1], frame)
		}
	}

	// Линии пути: старт -> точки -> выход. толщина 2–3px,
	// наконечники направления, посещённый участок — тусклый пунктир.
	type spt struct {
		x, y    float64
		visited bool
	}
	pts := make([]spt, 0, len(r.Stops))
	target := -1
	for i := range r.Stops {
		px, py := uv(r.Stops[i].X, r.Stops[i].Z)
		pts = append(pts, spt{px, py, r.Stops[i].Visited})
		if target < 0 && !r.Stops[i].Visited {
			target = i
		}
	}
	ox, oy := uv(r.Origin[0], r.Origin[2])
	prev := spt{ox, oy, true}
	for i := range pts {
		switch {
		case prev.visited && pts[i].visited:
			drawDashLine(screen, prev.x, prev.y, pts[i].x, pts[i].y, colGoldDim, 2)
		case i == target:
			drawThickLine(screen, prev.x, prev.y, pts[i].x, pts[i].y, colGold, 3)
			drawArrowHead(screen, pts[i].x, pts[i].y, pts[i].x-prev.x, pts[i].y-prev.y, colGold, 9)
		default:
			drawThickLine(screen, prev.x, prev.y, pts[i].x, pts[i].y, colGoldMid, 2)
			drawArrowHead(screen, pts[i].x, pts[i].y, pts[i].x-prev.x, pts[i].y-prev.y, colGoldMid, 7)
		}
		prev = pts[i]
	}
	exx, exy := uv(r.Exit.X, r.Exit.Z)
	if prev.visited { // все точки пройдены — путь до выхода тоже пунктир
		drawDashLine(screen, prev.x, prev.y, exx, exy, colGoldDim, 2)
	} else {
		drawThickLine(screen, prev.x, prev.y, exx, exy, colGold, 3)
		drawArrowHead(screen, exx, exy, exx-prev.x, exy-prev.y, colGold, 9)
	}
	screen.DrawImage(g.qhOverlay, &ebiten.DrawImageOptions{})

	// стрелки у края окна для внеэкранных цели и выхода.
	if target >= 0 {
		g.drawEdgeArrow(screen, pts[target].x, pts[target].y, strconv.Itoa(target+1))
	}
	g.drawEdgeArrow(screen, exx, exy, "V")

	// Точки с номерами.
	for i := range pts {
		p := &pts[i]
		if p.x < -20 || p.x > float64(w)+20 || p.y < -20 || p.y > float64(h)+20 {
			continue
		}
		col := colGold
		if p.visited {
			col = colGoldDim
		}
		g.drawDot(screen, p.x, p.y, col)
		drawTextOutlined(screen, int(p.x)+8, int(p.y)-14, strconv.Itoa(i+1), col)
	}
	// Первая непосещённая — кольцо-цель.
	for i := range pts {
		if !pts[i].visited && pts[i].x >= -20 && pts[i].x <= float64(w)+20 && pts[i].y >= -20 && pts[i].y <= float64(h)+20 {
			g.ringMarker(screen, pts[i].x, pts[i].y, colGold)
			break
		}
	}
	// Выбранный выход — золотое кольцо.
	g.ringMarker(screen, exx, exy, colGold)
}

// ringMarker — окружность-выделение (цель маршрута / выбранный выход).
func (g *Game) ringMarker(screen *ebiten.Image, sx, sy float64, col color.RGBA) {
	const rad = 12.0
	const steps = 40
	for i := 0; i < steps; i++ {
		a0 := float64(i) / steps * 2 * math.Pi
		a1 := float64(i+1) / steps * 2 * math.Pi
		drawLine(screen, sx+rad*math.Cos(a0), sy+rad*math.Sin(a0),
			sx+rad*math.Cos(a1), sy+rad*math.Sin(a1), col)
	}
}

// drawEdgeArrow — стрелка у края окна к внеэкранной цели + метка (номер/V).
func (g *Game) drawEdgeArrow(screen *ebiten.Image, sx, sy float64, label string) {
	ex, ey, outside := edgePoint(g.w(), g.h(), sx, sy, 26)
	if !outside {
		return
	}
	dx, dy := ex-float64(g.w())/2, ey-float64(g.h())/2
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	drawArrowHead(screen, ex, ey, dx, dy, colGold, 10)
	drawTextOutlined(screen, int(ex-dx/n*18)-3, int(ey-dy/n*18)-6, label, colGold)
}

// drawFarmHUD — строка фарма поверх карты: цель и живые дистанции от
// ТЕКУЩЕЙ позиции; при несовпадении карты маршрута и окна — предупреждение.
func (g *Game) drawFarmHUD(screen *ebiten.Image, key string) {
	r := g.farmRoute
	if r == nil {
		return
	}
	if r.MapKey != key {
		s := T("farm.hud.mismatch", g.mapName(r.MapKey), g.mapName(key))
		drawTextOutlined(screen, (g.w()-txtW(s))/2, 26, s, colGold)
		return
	}
	h, ok := g.farmHUDTarget()
	if !ok {
		return
	}
	var s string
	if h.Done {
		s = T("farm.hud.done", g.md.Loc(h.ExitName), h.DistExit)
	} else {
		s = T("farm.hud", h.N, h.Total, h.Name, h.DistStop, g.md.Loc(h.ExitName), h.DistExit)
	}
	drawTextOutlined(screen, (g.w()-txtW(s))/2, 12, s, colGold)
}

// drawThickLine — линия толщиной px (параллельные смещения по перпендикуляру).
func drawThickLine(img *ebiten.Image, x0, y0, x1, y1 float64, col color.RGBA, px int) {
	dx, dy := x1-x0, y1-y0
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	nx, ny := -dy/n, dx/n
	for i := 0; i < px; i++ {
		off := float64(i) - float64(px-1)/2
		drawLine(img, x0+nx*off, y0+ny*off, x1+nx*off, y1+ny*off, col)
	}
}

// drawDashLine — пунктир (5px штрих / 5px пропуск).
func drawDashLine(img *ebiten.Image, x0, y0, x1, y1 float64, col color.RGBA, px int) {
	dx, dy := x1-x0, y1-y0
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	ux, uy := dx/n, dy/n
	const on = 5.0
	for d := 0.0; d < n; d += 2 * on {
		e := math.Min(d+on, n)
		drawThickLine(img, x0+ux*d, y0+uy*d, x0+ux*e, y0+uy*e, col, px)
	}
}

// drawArrowHead — наконечник направления в точке (tx,ty) вдоль (dx,dy).
func drawArrowHead(img *ebiten.Image, tx, ty, dx, dy float64, col color.RGBA, size float64) {
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	ux, uy := dx/n, dy/n
	px, py := -uy, ux
	drawLine(img, tx, ty, tx-size*ux+size*0.6*px, ty-size*uy+size*0.6*py, col)
	drawLine(img, tx, ty, tx-size*ux-size*0.6*px, ty-size*uy-size*0.6*py, col)
}
