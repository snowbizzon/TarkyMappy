package main

import (
	"log"
	"syscall"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procGetAsyncKeyState = user32.NewProc("GetAsyncKeyState")
)

const (
	vkF2    = 0x71
	vkF3    = 0x72
	vkF4    = 0x73
	vkF5    = 0x74
	vkF6    = 0x75
	vkF7    = 0x76
	vkF8    = 0x77
	vkF9    = 0x78
	vkF10   = 0x79
	vkPrior = 0x21 // PageUp
	vkNext  = 0x22 // PageDown
)

// Hotkeys — глобальные клавиши (окно click-through, фокус окну не нужен).
type Hotkeys struct {
	prev [11]bool
}

func NewHotkeys() *Hotkeys { return &Hotkeys{} }

func keyDown(vk uintptr) bool {
	r, _, _ := procGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// Update опрашивает клавиши один раз за тик; действие — по фронту нажатия.
// cycleFloor(+1) — этаж выше (PageUp), cycleFloor(-1) — ниже (PageDown),
// quests — F4 (панель Quest Helper), farm — F3 (панель Помощника фарма),
// prices — F2 (панель Помощника цен).
func (h *Hotkeys) Update(st *State, quit, settings, nextMap func(), cycleFloor func(int), quests, farm, prices func()) {
	tap := func(i int, vk uintptr) bool {
		down := keyDown(vk)
		pressed := down && !h.prev[i]
		h.prev[i] = down
		return pressed
	}
	switch {
	case tap(0, vkF5):
		st.SetZoom(st.Zoom() * 1.25)
		log.Printf("zoom -> %.2f", st.Zoom())
	case tap(1, vkF6):
		st.SetZoom(st.Zoom() / 1.25)
		log.Printf("zoom -> %.2f", st.Zoom())
	case tap(2, vkF7):
		st.ToggleVisible()
	case tap(3, vkF8):
		quit()
	case tap(4, vkF9):
		settings()
	case tap(5, vkF10):
		nextMap()
	case tap(6, vkPrior):
		cycleFloor(1)
	case tap(7, vkNext):
		cycleFloor(-1)
	case tap(8, vkF4):
		quests()
	case tap(9, vkF3):
		farm()
	case tap(10, vkF2):
		prices()
	}
}
