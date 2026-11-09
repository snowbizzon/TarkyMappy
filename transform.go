package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// WorldToUV — мировые координаты (x, z) -> нормализованные (u, v) на текстуре карты.
// Формула TarkovTracker (прод-вьюер): u=(x-b00)/(b10-b00), v=(z-b01)/(b11-b01).
// Работает при любом порядке углов bounds.
func WorldToUV(b [2][2]float64, x, z float64) (u, v float64) {
	u = (x - b[0][0]) / (b[1][0] - b[0][0])
	v = (z - b[0][1]) / (b[1][1] - b[0][1])
	return u, v
}

// AxisSigns — знаки производных du/dx и dv/dz.
func AxisSigns(b [2][2]float64) (su, sv float64) {
	return sign(b[1][0] - b[0][0]), sign(b[1][1] - b[0][1])
}

func sign(f float64) float64 {
	switch {
	case f > 0:
		return 1
	case f < 0:
		return -1
	default:
		return 0
	}
}

// YawFromQuat — yaw в градусах [0,360) из кватерниона Unity (x, y, z, w).
// Порядок Эйлеров Unity ZXY: yaw = atan2(2(w·y + x·z), 1 − 2(y² + z²)).
func YawFromQuat(qx, qy, qz, qw float64) float64 {
	s := 2 * (qw*qy + qx*qz)
	c := 1 - 2*(qy*qy+qz*qz)
	yaw := math.Atan2(s, c) * 180 / math.Pi
	if yaw < 0 {
		yaw += 360
	}
	return yaw
}

// FacingDir — единичный вектор взгляда игрока в координатах текстуры (u вправо, v вниз).
// RotY в градусах, по часовой (Unity yaw): dir_world = (sinY, cosY) в осях (x, z).
func FacingDir(b [2][2]float64, rotY float64) (du, dv float64) {
	su, sv := AxisSigns(b)
	r := rotY * math.Pi / 180
	du, dv = su*math.Sin(r), sv*math.Cos(r)
	n := math.Hypot(du, dv)
	if n == 0 {
		return 0, 0
	}
	return du / n, dv / n
}

// PlayerTexPoint — точка игрока в пикселях текстуры.
func PlayerTexPoint(b [2][2]float64, x, z float64, iw, ih int) (px, py float64) {
	u, v := WorldToUV(b, x, z)
	return u * float64(iw), v * float64(ih)
}

// FullGeoM — матрица текстура->экран: игрок в центре окна (cx,cy),
// направление взгляда — вверх экрана.
// Порядок вызовов = порядок применения к точке (ebiten GeoM премультиплицирует):
// сдвиг к игроку, масштаб, поворот вокруг игрока, сдвиг в центр окна.
func FullGeoM(b [2][2]float64, x, z, rotY, zoom float64, iw, ih, sw, sh int) ebiten.GeoM {
	du, dv := FacingDir(b, rotY)
	psi := math.Atan2(-du, -dv) // поворот карты, чтобы взгляд смотрел вверх (0,-1)

	var m ebiten.GeoM
	px, py := PlayerTexPoint(b, x, z, iw, ih)
	m.Translate(-px, -py)                     // 1) игрок -> (0,0)
	m.Scale(zoom, zoom)                       // 2) зум
	m.Rotate(psi)                             // 3) поворот карты
	m.Translate(float64(sw)/2, float64(sh)/2) // 4) в центр окна
	return m
}
