package main

import (
	"math"
	"testing"
)

// bounds Таможни из data/maps.json (первый угол — maxX/minZ).
var customsB = [2][2]float64{{698, -307}, {-371, 237}}

func TestYawFromQuat(t *testing.T) {
	const sin45, cos45 = 0.7071067811865476, 0.7071067811865476
	tests := []struct {
		name           string
		qx, qy, qz, qw float64
		want           float64
	}{
		{"единичный кватернион", 0, 0, 0, 1, 0},
		{"чистый Y +90", 0, sin45, 0, cos45, 90},
		{"чистый Y +180", 0, 1, 0, 0, 180},
		{"чистый Y +270", 0, -sin45, 0, cos45, 270},
		{"реальный скрин 1 (≈162°)", 0.00027, 0.98771, -0.00171, 0.15626, 162.0},
		{"реальный скрин 2 (≈168°)", -0.00025, 0.99430, 0.00229, 0.10663, 167.8},
		{"реальный скрин 3 (≈273°)", 0.01726, -0.68871, 0.01623, 0.72465, 272.9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := YawFromQuat(tc.qx, tc.qy, tc.qz, tc.qw)
			if math.Abs(got-tc.want) > 0.6 {
				t.Fatalf("got %f, want %f", got, tc.want)
			}
			if got < 0 || got >= 360 {
				t.Fatalf("yaw out of range: %f", got)
			}
		})
	}
}

func TestWorldToUV(t *testing.T) {
	tests := []struct {
		name string
		b    [2][2]float64
		x, z float64
		u, v float64
	}{
		{"углы совпадают с границами", customsB, 698, -307, 0, 0},
		{"второй угол", customsB, -371, 237, 1, 1},
		{"реальный спавн Таможни", customsB, 569, -55, 129.0 / 1069.0, 252.0 / 544.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, v := WorldToUV(tc.b, tc.x, tc.z)
			if math.Abs(u-tc.u) > 1e-9 || math.Abs(v-tc.v) > 1e-9 {
				t.Fatalf("got (%f, %f), want (%f, %f)", u, v, tc.u, tc.v)
			}
		})
	}
}

func TestWorldToUVSwappedBounds(t *testing.T) {
	b := [2][2]float64{{-371, 237}, {698, -307}} // обратный порядок углов
	u, v := WorldToUV(b, 569, -55)
	if math.Abs(u-(1-129.0/1069.0)) > 1e-9 || math.Abs(v-(1-252.0/544.0)) > 1e-9 {
		t.Fatalf("swap bounds: got (%f, %f)", u, v)
	}
}

func TestFacingDir(t *testing.T) {
	tests := []struct {
		name   string
		b      [2][2]float64
		rotY   float64
		du, dv float64
	}{
		// Таможня: su=-1 (x растёт влево карты), sv=+1 (z растёт вниз карты)
		{"rotY=0: взгляд «вниз текстуры»", customsB, 0, 0, 1},
		{"rotY=90: взгляд «влево текстуры»", customsB, 90, -1, 0},
		{"rotY=180: взгляд «вверх текстуры»", customsB, 180, 0, -1},
		{"rotY=270: взгляд «вправо текстуры»", customsB, 270, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			du, dv := FacingDir(tc.b, tc.rotY)
			if math.Abs(du-tc.du) > 1e-9 || math.Abs(dv-tc.dv) > 1e-9 {
				t.Fatalf("got (%f, %f), want (%f, %f)", du, dv, tc.du, tc.dv)
			}
		})
	}
}

func TestFullGeoMPlayerCentred(t *testing.T) {
	const iw, ih = 4096, 2063
	x, z := 569.0, -55.0
	for _, rotY := range []float64{0, 45, 90, 180, 315} {
		gm := FullGeoM(customsB, x, z, rotY, 1, iw, ih, winW, winH)
		px, py := PlayerTexPoint(customsB, x, z, iw, ih)
		sx, sy := gm.Apply(px, py)
		if math.Abs(sx-winW/2) > 1e-6 || math.Abs(sy-winH/2) > 1e-6 {
			t.Fatalf("rotY=%f: player at (%f, %f), want (%d, %d)", rotY, sx, sy, winW/2, winH/2)
		}
	}
}

func TestFullGeoMFacingIsUp(t *testing.T) {
	// Точка текстуры, смещённая от игрока по направлению взгляда,
	// должна оказаться НА ЭКРАНЕ выше центра.
	const iw, ih = 4096, 2063
	x, z, rotY := 569.0, -55.0, 90.0
	du, dv := FacingDir(customsB, rotY) // (-1, 0): влево текстуры
	gm := FullGeoM(customsB, x, z, rotY, 1, iw, ih, winW, winH)
	px, py := PlayerTexPoint(customsB, x, z, iw, ih)
	sx, sy := gm.Apply(px+du*300, py+dv*300)
	if sy >= winH/2 {
		t.Fatalf("facing point below center: (%f, %f)", sx, sy)
	}
}

func TestZoomScalesDistance(t *testing.T) {
	const iw, ih = 4096, 2063
	x, z, rotY := 569.0, -55.0, 0.0
	px, py := PlayerTexPoint(customsB, x, z, iw, ih)
	du, dv := FacingDir(customsB, rotY)
	g1 := FullGeoM(customsB, x, z, rotY, 1, iw, ih, winW, winH)
	g2 := FullGeoM(customsB, x, z, rotY, 2, iw, ih, winW, winH)
	_, y1 := g1.Apply(px+du*100, py+dv*100)
	_, y2 := g2.Apply(px+du*100, py+dv*100)
	if math.Abs((winH/2-y2)/(winH/2-y1)-2) > 1e-6 {
		t.Fatalf("zoom does not double distance: %f vs %f", y1, y2)
	}
}
