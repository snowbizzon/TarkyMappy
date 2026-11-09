package main

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
)

// --- FAQ-панель: зелёная «?» в F9 — справка по цветам, фарму и клавишам. ---

// toggleFAQ — открыть/закрыть справку (взаимоисключимо с F9/F3/F4).
func (g *Game) toggleFAQ() {
	if g.faqOpen {
		g.closeFAQ()
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
	g.faqOpen = true
	ebiten.SetWindowMousePassthrough(false)
}

func (g *Game) closeFAQ() {
	procReleaseCapture.Call()
	g.faqOpen = false
	g.faqScroll = 0
	ebiten.SetWindowMousePassthrough(true)
	g.saveWindowGeometry()
}

// faqContentRect — зона текста (между заголовком и нижней кнопкой).
func (g *Game) faqContentRect() image.Rectangle {
	return image.Rect(padX, 30, g.w()-padX, g.h()-44)
}

// faqVisible — сколько строк справки видно.
func faqVisible(r image.Rectangle) int {
	v := (r.Dy() - 8) / 15
	if v < 1 {
		return 1
	}
	return v
}

// faqLines — содержимое справки (maxChars — ширина блока в символах).
func (g *Game) faqLines(maxChars int) []hintLine {
	head := color.RGBA{200, 205, 215, 255}
	type row struct {
		s string
		c color.RGBA
	}
	var rows []row
	add := func(s string, c color.RGBA) { rows = append(rows, row{s, c}) }

	meta := g.md.Meta()
	add(fmt.Sprintf(T("faq.data"), meta.GameBuild, meta.DataDate), head)
	if meta.LighthouseArt != "" {
		add(T("faq.lhart"), colDim)
	}
	add("", colDim)
	add(T("faq.exits"), head)
	add(T("faq.exit.pmc"), colPMC)
	add(T("faq.exit.scav"), colScav)
	add(T("faq.exit.shared"), colShared)
	add(T("faq.exit.transit"), colTransit)
	add(T("faq.exit.floor"), colDim)
	add(T("faq.hazards"), colMine)
	add("", colDim)
	add(T("faq.farm"), colGold)
	add(T("faq.farm.pts"), colGold)
	add(T("faq.farm.visited"), colGoldDim)
	add(T("faq.farm.ring"), colGold)
	add(T("faq.farm.lines"), colGold)
	add(T("faq.farm.arrow"), colGold)
	add(T("faq.farm.hud"), colGold)
	add("", colDim)
	add(T("faq.panel"), head)
	add(T("faq.panel.cats"), colText)
	add(T("faq.panel.stops"), colText)
	add(T("faq.panel.exit"), colText)
	add(T("faq.panel.gen"), colText)
	add(T("faq.panel.new"), colText)
	add(T("faq.panel.reset"), colText)
	add(T("faq.panel.prtsc"), colDim)
	add("", colDim)
	add(T("faq.qpanel"), head)
	add(T("faq.qpanel.track"), colText)
	add(T("faq.qpanel.mine"), colText)
	add(T("faq.qpanel.auto"), colText)
	add("", colDim)
	add(T("faq.keys"), head)
	add(T("faq.keys.f2"), colText)
	add(T("faq.keys.f3"), colText)
	add(T("faq.keys.f8"), colText)
	add(T("faq.keys.gear"), colText)
	add(T("faq.keys.grip"), colText)

	var lines []hintLine
	for _, r := range rows {
		if r.s == "" {
			lines = append(lines, hintLine{"", colDim})
			continue
		}
		for i, w := range wrapText(r.s, maxChars) {
			if i == 0 {
				lines = append(lines, hintLine{w, r.c})
			} else {
				lines = append(lines, hintLine{"  " + w, r.c})
			}
		}
	}
	return lines
}

// updateFAQ — каркас окна, скролл, Esc, Zakryt.
func (g *Game) updateFAQ() {
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
	if _, dy := ebiten.Wheel(); dy != 0 {
		cr := g.faqContentRect()
		max := len(g.faqLines(cr.Dx()/charW)) - faqVisible(cr)
		if max < 0 {
			max = 0
		}
		g.faqScroll = clampI(g.faqScroll-int(dy), 0, max)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.closeFAQ()
		return
	}
	if clickIn(g.fhCloseRect()) {
		g.closeFAQ()
	}
}

// drawFAQ — панель справки: заголовок, текст со скроллом и мини-скроллбаром,
// Zakryt внизу, грип в углу.
func (g *Game) drawFAQ(screen *ebiten.Image) {
	w, h := g.w(), g.h()
	screen.DrawImage(g.panel, &ebiten.DrawImageOptions{})
	text.Draw(screen, T("faq.title"), uiFace, padX, 19, colGold)
	hint := T("drag.hint")
	text.Draw(screen, hint, uiFace, w-8-txtW(hint), 19, colDim)
	for x := 4; x < w-4; x += 6 {
		screen.Set(x, dragBarH, colDim)
		screen.Set(x+1, dragBarH, colDim)
	}
	cr := g.faqContentRect()
	lines := g.faqLines(cr.Dx() / charW)
	visible := faqVisible(cr)
	maxScroll := len(lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	g.faqScroll = clampI(g.faqScroll, 0, maxScroll)
	for i := g.faqScroll; i < len(lines) && i < g.faqScroll+visible; i++ {
		text.Draw(screen, lines[i].s, uiFace, cr.Min.X, cr.Min.Y+12+(i-g.faqScroll)*15, lines[i].c)
	}
	if maxScroll > 0 {
		trackTop, trackBot := float64(cr.Min.Y+8), float64(cr.Max.Y-8)
		drawLine(screen, float64(cr.Max.X-2), trackTop, float64(cr.Max.X-2), trackBot, colRing)
		thumbH := (trackBot - trackTop) * float64(visible) / float64(len(lines))
		thumbTop := trackTop + (trackBot-trackTop-thumbH)*float64(g.faqScroll)/float64(maxScroll)
		drawLine(screen, float64(cr.Max.X-2), thumbTop, float64(cr.Max.X-2), thumbTop+thumbH, colDim)
	}
	drawButton(screen, g.frameBtn, g.fhCloseRect(), T("btn.close"), colDim)
	screen.DrawImage(g.grip, opAt(image.Point{X: w - gripS, Y: h - gripS}))
}
