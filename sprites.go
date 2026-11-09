package main

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// fillImage — одноцветное изображение w x h.
func fillImage(w, h int, col color.Color) *ebiten.Image {
	img := ebiten.NewImage(w, h)
	img.Fill(col)
	return img
}

// circleImage — заполненный круг диаметром d.
func circleImage(d int, col color.Color) *ebiten.Image {
	img := ebiten.NewImage(d, d)
	r := float64(d) / 2
	cx, cy := r-0.5, r-0.5
	for y := 0; y < d; y++ {
		for x := 0; x < d; x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, col)
			}
		}
	}
	return img
}

// arrowSprite — стрелка «вперёд» (вверх): тёмная обводка + белый корпус.
func arrowSprite(h int) *ebiten.Image {
	w := h
	out := ebiten.NewImage(w, h)
	pts := [][2]float64{
		{float64(w) / 2, 1},              // кончик
		{float64(w) - 3, float64(h) - 2}, // правый нижний
		{float64(w) / 2, float64(h) - 6}, // выемка хвоста
		{3, float64(h) - 2},              // левый нижний
	}
	fillPoly(out, pts, colRing, 1.6)
	fillPoly(out, pts, colArrow, 0)
	return out
}

// fillPoly — scanline-растеризация полигона; expand расширяет контур (обводка).
func fillPoly(img *ebiten.Image, pts [][2]float64, col color.Color, expand float64) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		fy := float64(y) + 0.5
		var xs []float64
		for i := 0; i < len(pts); i++ {
			a, c := pts[i], pts[(i+1)%len(pts)]
			if (a[1] <= fy && c[1] > fy) || (c[1] <= fy && a[1] > fy) {
				t := (fy - a[1]) / (c[1] - a[1])
				xs = append(xs, a[0]+t*(c[0]-a[0]))
			}
		}
		sortF(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			for x := int(xs[i] - expand); x <= int(xs[i+1]+expand); x++ {
				if x >= b.Min.X && x < b.Max.X {
					img.Set(x, y, col)
				}
			}
		}
	}
}

func sortF(s []float64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// gearSprite — шестерёнка d×d: кольцо с 8 зубьями и отверстием в центре.
func gearSprite(d int, col color.Color) *ebiten.Image {
	img := ebiten.NewImage(d, d)
	c := float64(d)/2 - 0.5
	rOut := c - 0.5
	rIn := rOut - float64(d)*0.22
	rHole := float64(d) * 0.18
	const sector = math.Pi / 4
	for y := 0; y < d; y++ {
		for x := 0; x < d; x++ {
			dx, dy := float64(x)-c, float64(y)-c
			dist := math.Hypot(dx, dy)
			if dist > rOut || dist < rHole {
				continue
			}
			if dist >= rIn {
				seg := math.Mod(math.Atan2(dy, dx)+math.Pi*4, sector)
				if seg < sector*0.22 || seg > sector*0.78 {
					continue
				}
			}
			img.Set(x, y, col)
		}
	}
	return img
}

// frameImage — прямоугольная рамка толщиной 1px.
func frameImage(w, h int, col color.Color) *ebiten.Image {
	img := ebiten.NewImage(w, h)
	for x := 0; x < w; x++ {
		img.Set(x, 0, col)
		img.Set(x, h-1, col)
	}
	for y := 0; y < h; y++ {
		img.Set(0, y, col)
		img.Set(w-1, y, col)
	}
	return img
}

// gripSprite — уголок-«грип» для ресайза: диагональные штрихи в правом нижнем углу.
func gripSprite(d int, col color.Color) *ebiten.Image {
	img := ebiten.NewImage(d, d)
	for n := 0; n < 3; n++ {
		start := 4 + n*5
		for i := start; i < d; i++ {
			img.Set(i, d-1-(i-start), col)
		}
	}
	return img
}
