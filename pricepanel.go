package main

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// --- Помощник цен (F2): топ барахолки ₽/слот, вкладки PvP/PvE. ---
// Данные — prices/prices.json рядом с exe; обновляются prices/update_prices.py
// без пересборки приложения.

// togglePrices — открыть/закрыть панель цен (взаимоисключимо с F9/F3/F4/FAQ).
func (g *Game) togglePrices() {
	if g.pricesOpen {
		g.closePricePanel()
		return
	}
	if g.settings.open {
		g.closeSettings()
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
	g.pricesOpen = true
	g.md.loadPrices() // свежие цены при каждом открытии — пересборка не нужна
	ebiten.SetWindowMousePassthrough(false)
}

func (g *Game) closePricePanel() {
	procReleaseCapture.Call()
	g.pricesOpen = false
	g.priceScroll = 0
	ebiten.SetWindowMousePassthrough(true)
	g.saveWindowGeometry()
}

// priceModeIdx — 0 = PvP ("regular"), 1 = PvE ("pve").
func (g *Game) priceModeIdx() int {
	if g.priceMode == "pve" {
		return 1
	}
	return 0
}

func priceModeKey(i int) string {
	if i == 1 {
		return "pve"
	}
	return "regular"
}

// priceTabRect — вкладка [PvP | PvE] в шапке панели.
func priceTabRect(i int) image.Rectangle {
	x := padX + i*(btnW+6)
	return image.Rect(x, 32, x+btnW, 32+btnH)
}

// priceContentRect — зона списка (под вкладками, над кнопкой Zakryt).
func (g *Game) priceContentRect() image.Rectangle {
	return image.Rect(padX, 66, g.w()-padX, g.h()-44)
}

// priceVisible — сколько строк списка видно.
func priceVisible(r image.Rectangle) int {
	v := (r.Dy() - 8) / 15
	if v < 1 {
		return 1
	}
	return v
}

// fmtPrice — короткий формат цены: 743 / 50k / 1.2m (соглашение).
func fmtPrice(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 1000000:
		k := (n + 500) / 1000 // округление до целых тысяч
		return strconv.Itoa(k) + "k"
	default:
		t := (n + 50000) / 100000 // десятые доли миллиона
		if t%10 == 0 {
			return strconv.Itoa(t/10) + "m"
		}
		return strconv.Itoa(t/10) + "." + strconv.Itoa(t%10) + "m"
	}
}

// priceLines — строки панели: шапка-легенда + топ предметов режима.
func (g *Game) priceLines() []hintLine {
	items, _ := g.md.Prices(priceModeKey(g.priceModeIdx()))
	lines := make([]hintLine, 0, len(items)+1)
	if len(items) == 0 {
		lines = append(lines, hintLine{T("prices.empty"), colDim})
		return lines
	}
	lines = append(lines, hintLine{T("prices.hint"), colDim})
	cr := g.priceContentRect()
	maxChars := cr.Dx() / charW
	for i := range items {
		it := &items[i]
		s := fmt.Sprintf(T("prices.row"), i+1, g.md.Loc(it.N), it.W, it.H,
			fmtPrice(it.P), fmtPrice(it.PP))
		lines = append(lines, hintLine{clipTail(s, maxChars), colText})
	}
	return lines
}

// updatePricePanel — вкладки, скролл, Esc, Zakryt.
func (g *Game) updatePricePanel() {
	mx, my := ebiten.CursorPosition()
	press := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	release := inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)
	if press {
		g.downX, g.downY, g.downOn = mx, my, true
	}
	clickIn := func(r image.Rectangle) bool {
		return release && g.downOn && ptIn(g.downX, g.downY, r) && ptIn(mx, my, r)
	}
	if g.handleWindowChrome(mx, my, press, held) {
		return
	}
	for i := 0; i < 2; i++ { // вкладки PvP/PvE
		if clickIn(priceTabRect(i)) && g.priceModeIdx() != i {
			g.priceMode = priceModeKey(i)
			g.priceScroll = 0
			return
		}
	}
	if _, dy := ebiten.Wheel(); dy != 0 {
		cr := g.priceContentRect()
		max := len(g.priceLines()) - priceVisible(cr)
		if max < 0 {
			max = 0
		}
		g.priceScroll = clampI(g.priceScroll-int(dy), 0, max)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closePricePanel()
		return
	}
	if clickIn(g.fhCloseRect()) {
		g.closePricePanel()
	}
}

// drawPricePanel — заголовок + дата снимка, вкладки [PvP | PvE], список
// топа с прокруткой и мини-скроллбаром, Zakryt внизу, грип в углу.
func (g *Game) drawPricePanel(screen *ebiten.Image) {
	w, h := g.w(), g.h()
	screen.DrawImage(g.panel, &ebiten.DrawImageOptions{})
	text.Draw(screen, T("prices.title"), uiFace, padX, 19, colGold)
	hint := T("drag.hint")
	text.Draw(screen, hint, uiFace, w-8-txtW(hint), 19, colDim)
	for x := 4; x < w-4; x += 6 {
		screen.Set(x, dragBarH, colDim)
		screen.Set(x+1, dragBarH, colDim)
	}

	// Вкладки режимов (активная — золотом).
	for i := 0; i < 2; i++ {
		active := g.priceModeIdx() == i
		col := colText
		if active {
			col = colGold
		}
		drawButton(screen, g.frameBtn, priceTabRect(i), T(priceTabLangKey(i)), col)
	}
	// Дата снимка цен — справа от вкладок.
	if _, updated := g.md.Prices(priceModeKey(g.priceModeIdx())); updated != "" {
		text.Draw(screen, T("prices.updated", shortDate(updated)), uiFace,
			priceTabRect(1).Max.X+10, priceTabRect(1).Max.Y-1, colDim)
	}

	cr := g.priceContentRect()
	lines := g.priceLines()
	visible := priceVisible(cr)
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	g.priceScroll = clampI(g.priceScroll, 0, maxScroll)
	for i := g.priceScroll; i < len(lines) && i < g.priceScroll+visible; i++ {
		s, c := lines[i].s, lines[i].c
		// Подсветка цены за слот в строках топа (лёгкий сплит по «|»).
		if j := strings.LastIndexByte(s, '|'); j >= 0 {
			text.Draw(screen, s[:j], uiFace, cr.Min.X, cr.Min.Y+12+(i-g.priceScroll)*15, c)
			text.Draw(screen, s[j:], uiFace, cr.Min.X+j*charW, cr.Min.Y+12+(i-g.priceScroll)*15, colGold)
		} else {
			text.Draw(screen, s, uiFace, cr.Min.X, cr.Min.Y+12+(i-g.priceScroll)*15, c)
		}
	}
	if maxScroll > 0 {
		trackTop, trackBot := float64(cr.Min.Y+8), float64(cr.Max.Y-8)
		drawLine(screen, float64(cr.Max.X-2), trackTop, float64(cr.Max.X-2), trackBot, colRing)
		thumbH := (trackBot - trackTop) * float64(visible) / float64(len(lines))
		thumbTop := trackTop + (trackBot-trackTop-thumbH)*float64(g.priceScroll)/float64(maxScroll)
		drawLine(screen, float64(cr.Max.X-2), thumbTop, float64(cr.Max.X-2), thumbTop+thumbH, colDim)
	}
	drawButton(screen, g.frameBtn, g.fhCloseRect(), T("btn.close"), colDim)
	screen.DrawImage(g.grip, opAt(image.Point{X: w - gripS, Y: h - gripS}))
}

// priceTabLangKey — ключ локали вкладки режима.
func priceTabLangKey(i int) string {
	if i == 1 {
		return "prices.tab.pve"
	}
	return "prices.tab.pvp"
}

// shortDate — 2026-09-05T12:34:56+00:00 -> 05.09.2026.
func shortDate(iso string) string {
	if len(iso) < 10 {
		return iso
	}
	return iso[8:10] + "." + iso[5:7] + "." + iso[0:4]
}
