package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestParseScreenshotName(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		x, y, z, r float64
		ok         bool
	}{
		{
			// Реальный скрин в рейде: позиция с пробелами + кватернион + таймер.
			name: "реальный скрин в рейде",
			file: "2026-08-30[17-30]_388.36, 2.64, -114.48_0.00027, 0.98771, -0.00171, 0.15626_8.60 (0).png",
			x:    388.36, y: 2.64, z: -114.48, r: 162.0, ok: true,
		},
		{
			name: "реальный скрин, отрицательный кватернион",
			file: "2026-08-30[15-04]_-23.66, -5.48, -135.16_0.01726, -0.68871, 0.01623, 0.72465_15.47 (0).png",
			x:    -23.66, y: -5.48, z: -135.16, r: 272.9, ok: true,
		},
		{
			name: "без пробелов после запятых",
			file: "2026-08-30[17-30]_388.36,2.64,-114.48_0.00027,0.98771,-0.00171,0.15626_8.60 (0).png",
			x:    388.36, y: 2.64, z: -114.48, r: 162.0, ok: true,
		},
		{name: "главное меню (без координат)", file: "2026-08-30[17-27] (0).png", ok: false},
		{name: "не в рейде (одно число)", file: "2026-08-30[15-25]_16.50 (0).png", ok: false},
		{name: "левый файл", file: "screenshot.png", ok: false},
		{name: "не png", file: "2026-08-30[17-30]_1, 2, 3_0, 1, 0, 0_8.60 (0).jpg", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y, z, r, ok := ParseScreenshotName(tc.file)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && (math.Abs(x-tc.x) > 1e-9 || math.Abs(y-tc.y) > 1e-9 ||
				math.Abs(z-tc.z) > 1e-9 || math.Abs(r-tc.r) > 0.6) {
				t.Fatalf("got (%f, %f, %f, %f), want (%f, %f, %f, %f)", x, y, z, r, tc.x, tc.y, tc.z, tc.r)
			}
		})
	}
}

func TestNewestScreenshot(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("2026-08-30[10-00]_1, 2, 3_0, 1, 0, 0_8.00 (0).png")
	write("2026-08-30[17-27] (0).png") // меню — не считается
	write("readme.txt")
	write("2026-08-30[11-00]_4, 5, 6_0, 1, 0, 0_9.00 (0).png")

	name, ok := newestScreenshot(dir)
	if !ok || name != "2026-08-30[11-00]_4, 5, 6_0, 1, 0, 0_9.00 (0).png" {
		t.Fatalf("newest = %q (ok=%v)", name, ok)
	}
}

func TestScreenshotsDirEndsWithKnownPath(t *testing.T) {
	dir, err := ScreenshotsDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "Screenshots" {
		t.Fatalf("unexpected dir: %s", dir)
	}
}
