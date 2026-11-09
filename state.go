package main

import "sync"

// State — потокобезопасное состояние оверлея.
type State struct {
	mu      sync.RWMutex
	hasPos  bool
	x, y, z float64 // y — высота (выбор этажа карты)
	rotY    float64
	zoom    float64
	visible bool
}

func NewState() *State {
	return &State{zoom: 1.0, visible: true}
}

// SetPosition вызывается вотчером при новом скриншоте.
func (s *State) SetPosition(x, y, z, rotY float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasPos, s.x, s.y, s.z, s.rotY = true, x, y, z, rotY
}

func (s *State) Pos() (x, y, z, rotY float64, ok bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.x, s.y, s.z, s.rotY, s.hasPos
}

func (s *State) SetZoom(z float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if z < 0.06 { // ещё вдвое дальше — карта целиком в окне
		z = 0.06
	}
	if z > 8 {
		z = 8
	}
	s.zoom = z
}

func (s *State) Zoom() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.zoom
}

func (s *State) SetVisible(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = v
}

func (s *State) ToggleVisible() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = !s.visible
}

func (s *State) Visible() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.visible
}
