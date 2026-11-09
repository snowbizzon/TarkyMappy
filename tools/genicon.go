//go:build ignore

// genicon — генератор иконки TarkyMappy (круглый бейдж-компас).
// Рендер с суперсэмплингом 4x4: base 256x256, downscale до 64.
// Пишет app.ico (64 = 32bpp BMP, 256 = PNG) в корень и в bin/.
// Запуск: go run tools/genicon.go
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

const (
	base   = 256 // базовый рендер
	ss     = 4   // сэмплов на пиксель по оси
	cx, cy = 128.0, 128.0
)

var (
	colSteel    = [3]float64{201, 212, 222} // светлый сталь (обводка)
	colBadge    = [3]float64{21, 27, 35}    // тёмный графит (фон)
	colInner    = [3]float64{42, 53, 66}    // внутренний тонкий ринг
	colTickMain = [3]float64{143, 161, 179} // главные тики N/E/S/W
	colTickSub  = [3]float64{61, 75, 90}    // диагональные тики
	colWhite    = [3]float64{244, 247, 250} // северный луч иглы
	colGreen    = [3]float64{61, 200, 113}  // южный луч + точка выхода
)

// inPoly — even-odd ray casting.
func inPoly(px, py float64, poly [][2]float64) bool {
	inside := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		xi, yi := poly[i][0], poly[i][1]
		xj, yj := poly[j][0], poly[j][1]
		if (yi > py) != (yj > py) && px < (xj-xi)*(py-yi)/(yj-yi)+xi {
			inside = !inside
		}
	}
	return inside
}

// inSeg — прямоугольная полоса вдоль угла a, радиусы r0..r1, полуширина w.
func inSeg(px, py, a, r0, r1, w float64) bool {
	dx, dy := px-cx, py-cy
	d := math.Hypot(dx, dy)
	if d < r0 || d > r1 {
		return false
	}
	lx, ly := math.Cos(a), math.Sin(a)
	return math.Abs(lx*dy-ly*dx) <= w // расстояние до луча
}

// ring — попадание в кольцо r0..r1.
func ring(px, py, r0, r1 float64) bool {
	d := math.Hypot(px-cx, py-cy)
	return d >= r0 && d <= r1
}

// sample — цвет пикселя (px,py) и покрытие (0..1 для краёв бейджа).
func sample(px, py float64) ([3]float64, float64) {
	if !ring(px, py, 0, 126) {
		return colBadge, 0 // прозрачный фон
	}
	if ring(px, py, 118, 126) { // внешняя стальная обводка
		return colSteel, 1
	}

	// 1) тики компаса: главные по N/E/S/W, короче — по диагоналям
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		if i%2 == 0 {
			if inSeg(px, py, a, 96, 112, 2.2) {
				return colTickMain, 1
			}
		} else if inSeg(px, py, a, 100, 108, 1.6) {
			return colTickSub, 1
		}
	}

	// 2) игла: северный (белый) и южный (зелёный) лучи
	needleN := [][2]float64{{cx - 22, cy + 22}, {cx + 22, cy + 22}, {cx + 5, cy - 90}, {cx - 5, cy - 90}}
	needleS := [][2]float64{{cx - 15, cy + 2}, {cx + 15, cy + 2}, {cx + 4, cy + 68}, {cx - 4, cy + 68}}
	if inPoly(px, py, needleN) {
		return colWhite, 1
	}
	if inPoly(px, py, needleS) {
		return colGreen, 1
	}

	// 3) ось: стальной ринг + тёмный центр
	if ring(px, py, 13, 15) {
		return colSteel, 1
	}
	// 4) точка-«выход»: правый низ, r=78, угол 40°, с тёмной обводкой
	ex, ey := cx+78*math.Cos(40*math.Pi/180), cy+78*math.Sin(40*math.Pi/180)
	if de := math.Hypot(px-ex, py-ey); de <= 9 {
		return colGreen, 1
	} else if de <= 12 {
		return colBadge, 1
	}

	return colBadge, 1
}

func main() {
	// Рендер base с SSAA.
	img := image.NewRGBA(image.Rect(0, 0, base, base))
	for y := 0; y < base; y++ {
		for x := 0; x < base; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					c, cov := sample(px, py)
					r += c[0] * cov
					g += c[1] * cov
					b += c[2] * cov
					a += cov * 255
				}
			}
			n := float64(ss * ss)
			img.SetRGBA(x, y, color.RGBA{
				uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n),
			})
		}
	}

	// 64x64 box-downscale.
	img64 := downscale(img, 64)

	// ICO: entry 0 = 32 (BMP 32bpp), entry 1 = 64 (BMP 32bpp), entry 2 = 256 (PNG).
	var png256 bytes.Buffer
	if err := png.Encode(&png256, img); err != nil {
		panic(err)
	}
	var bmp64, bmp32 bytes.Buffer
	writeBMP(&bmp64, img64)
	writeBMP(&bmp32, downscale(img, 32))

	size0, size1, size2 := bmp32.Len(), bmp64.Len(), png256.Len()
	off0 := 6 + 16*3
	off1, off2 := off0+size0, off0+size0+size1

	var out bytes.Buffer
	out.Write([]byte{0, 0, 1, 0, 3, 0}) // ICONDIR: type=icon, count=3
	writeDir(&out, 32, 32, uint32(size0), uint32(off0))
	writeDir(&out, 64, 64, uint32(size1), uint32(off1))
	writeDir(&out, 0, 0, uint32(size2), uint32(off2)) // 0 = 256
	out.Write(bmp32.Bytes())
	out.Write(bmp64.Bytes())
	out.Write(png256.Bytes())

	for _, p := range []string{"app.ico"} {
		if err := os.WriteFile(p, out.Bytes(), 0o644); err != nil {
			panic(err)
		}
	}
	println("app.ico:", out.Len(), "bytes")

	// Иконка окна для go:embed (main.go -> ebiten.SetWindowIcon).
	if err := savePNG("icon64.png", img64); err != nil {
		panic(err)
	}
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func writeDir(w *bytes.Buffer, wd, ht int, size, off uint32) {
	w.Write([]byte{byte(wd), byte(ht), 0, 0, 1, 0, 32, 0})
	put32(w, size)
	put32(w, off)
}

func put32(w *bytes.Buffer, v uint32) {
	w.Write([]byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)})
}

// downscale — box-downscale до n×n (n должно нацело делить 256).
func downscale(img *image.RGBA, n int) *image.RGBA {
	k := base / n
	out := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a int
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
					c := img.RGBAAt(x*k+dx, y*k+dy)
					r += int(c.R) * int(c.A)
					g += int(c.G) * int(c.A)
					b += int(c.B) * int(c.A)
					a += int(c.A)
				}
			}
			kk := k * k
			if a == 0 {
				continue
			}
			out.SetRGBA(x, y, color.RGBA{
				uint8(r / a), uint8(g / a), uint8(b / a), uint8(a / kk),
			})
		}
	}
	return out
}

// writeBMP — BITMAPINFOHEADER + BGRA снизу-вверх + AND-маска из альфы.
// AND-маска: бит 1 = прозрачный пиксель (A < 128), 0 = взять из XOR.
// Нулевая маска (без альфы) рендерится старыми путями как чёрный квадрат.
func writeBMP(w *bytes.Buffer, img *image.RGBA) {
	n := img.Bounds().Dx()
	hdr := make([]byte, 40)
	put := func(i int, v uint32) {
		hdr[i] = byte(v)
		hdr[i+1] = byte(v >> 8)
		hdr[i+2] = byte(v >> 16)
		hdr[i+3] = byte(v >> 24)
	}
	put(0, 40)
	put(4, uint32(n))
	put(8, uint32(n*2)) // XOR+AND
	put(12, 1<<16|32)   // planes=1, bpp=32
	w.Write(hdr)
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			c := img.RGBAAt(x, y)
			w.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	// AND-маска 1bpp, строка выровнена до 4 байт (для 32/64 и так кратна).
	row := make([]byte, n/8)
	for y := n - 1; y >= 0; y-- {
		var rowBits byte
		for x := 0; x < n; x++ {
			if img.RGBAAt(x, y).A < 128 {
				rowBits |= 1 << (7 - uint(x%8))
			}
			if x%8 == 7 {
				row[x/8] = rowBits
				rowBits = 0
			}
		}
		w.Write(row)
	}
}
