//go:build preview

package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type previewCase struct {
	name    string
	title   string
	legend  string
	cfg     func(*Config)
	tracked []string
	farm    bool
}

// TestGenerateLighthousePreviews renders the Lighthouse map with different
// overlay combinations. Lighthouse is not verified in a live raid: the map is
// still locked for the author by game progression, so these previews are
// rendered from embedded data only. The test is a developer utility and is
// excluded from the normal build/test run by the "preview" build tag.
//
// Run:
//
//	go test -tags preview -run TestGenerateLighthousePreviews -v
func TestGenerateLighthousePreviews(t *testing.T) {
	if testing.Short() {
		t.Skip("preview generation is disabled in -short mode")
	}
	outDir := filepath.Join("docs", "screenshots")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []previewCase{
		{
			name:   "lighthouse_01_exits",
			title:  "Lighthouse — extracts and transits",
			legend: "green = Scav • blue = PMC • yellow = shared • purple = transit",
			cfg: func(c *Config) {
				c.ShowPmc, c.ShowScav, c.ShowShared = true, true, true
				c.ShowHazards = false
			},
		},
		{
			name:   "lighthouse_02_hazards",
			title:  "Lighthouse — hazards",
			legend: "red = minefield • orange = sniper zone • gold + = claymore/trap",
			cfg: func(c *Config) {
				c.ShowPmc, c.ShowScav, c.ShowShared = true, true, true
				c.ShowHazards = true
			},
		},
		{
			name:   "lighthouse_03_quests",
			title:  "Lighthouse — tracked quests",
			legend: "colored zones = quest visit areas • diamonds = key doors",
			cfg: func(c *Config) {
				c.ShowPmc, c.ShowScav, c.ShowShared = true, true, true
				c.ShowHazards = false
			},
			tracked: []string{
				"674492b6909d2013670a347a", // Ask for Directions
				"6179ac7511973d018217d0b9", // Easy Job
				"6179b4d1bca27a099552e04e", // Revision - Lighthouse
				"63966fccac6f8f3c677b9d89", // Snatch
				"6179b3bdc7560e13d23eeb8d", // Corporate Secrets
			},
		},
		{
			name:   "lighthouse_04_farm",
			title:  "Lighthouse — farm route",
			legend: "gold line = route • circles = loot clusters • exit = farthest available",
			cfg: func(c *Config) {
				c.ShowPmc, c.ShowScav, c.ShowShared = true, true, true
				c.ShowHazards = false
				c.FarmCats = 0b11111
				c.FarmFloors = true
				c.FarmStops = 12
			},
			farm: true,
		},
		{
			name:   "lighthouse_05_all",
			title:  "Lighthouse — all overlays",
			legend: "exits + hazards + tracked quests + key doors",
			cfg: func(c *Config) {
				c.ShowPmc, c.ShowScav, c.ShowShared = true, true, true
				c.ShowHazards = true
			},
			tracked: []string{
				"674492b6909d2013670a347a", // Ask for Directions
				"66d9cbb67b491f9d5304f6e6", // Is This a Reference?
				"6179b4d1bca27a099552e04e", // Revision - Lighthouse
				"6a45208043b8d7604d00b8d5", // The Huntsman Path - Administrator
			},
		},
	}

	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	pg := &previewGame{md: md, cases: cases, outDir: outDir}
	ebiten.SetWindowSize(900, 900)
	ebiten.SetWindowTitle("TarkyMappy preview")
	if err := ebiten.RunGame(pg); err != nil && err != ebiten.Termination {
		// Some Windows/GLFW combinations return an error while destroying the
		// hidden preview window after all frames were rendered. Files are
		// already on disk, so only fail when nothing was written.
		if pg.current == 0 {
			t.Fatal(err)
		}
		t.Logf("window teardown warning: %v", err)
	}
	if pg.err != nil {
		t.Fatal(pg.err)
	}
	if pg.current != len(cases) {
		t.Fatalf("rendered %d/%d previews", pg.current, len(cases))
	}
}

type previewGame struct {
	md      *MapData
	cases   []previewCase
	outDir  string
	current int
	drawn   bool
	err     error
}

func (p *previewGame) Update() error {
	if p.err != nil {
		return ebiten.Termination
	}
	if p.drawn {
		p.drawn = false
		p.current++
	}
	if p.current >= len(p.cases) {
		return ebiten.Termination
	}
	return nil
}

func (p *previewGame) Draw(screen *ebiten.Image) {
	if p.current >= len(p.cases) || p.drawn || p.err != nil {
		return
	}
	p.drawn = true
	tc := p.cases[p.current]
	if err := p.render(screen, tc); err != nil {
		p.err = err
	}
}

func (p *previewGame) Layout(outW, outH int) (int, int) {
	return 900, 900
}

func (p *previewGame) render(screen *ebiten.Image, tc previewCase) error {
	cfg := DefaultConfig()
	cfg.ManualMap = true
	cfg.ManualKey = "lighthouse"
	cfg.Lang = "en"
	cfg.Zoom = 0.28
	setLang("en")
	if tc.cfg != nil {
		tc.cfg(&cfg)
	}

	st := NewState()
	qw := NewQuestWatch("", p.md, nil)
	defer qw.Close()
	if len(tc.tracked) > 0 {
		qw.RestoreTracked(tc.tracked)
	}
	g := NewGame(st, p.md, cfg, "", func(string) error { return nil }, qw)
	g.Layout(900, 900)
	st.SetZoom(cfg.Zoom)
	st.SetPosition(0, 6, -350, 0)
	p.md.SetWanted("lighthouse")
	if tc.farm {
		if msg := g.farmGenerate(false); msg == "" {
			return os.ErrInvalid
		}
	}

	g.Draw(screen)
	pixels := make([]byte, 4*900*900)
	screen.ReadPixels(pixels)
	shot := &image.RGBA{Pix: pixels, Stride: 4 * 900, Rect: image.Rect(0, 0, 900, 900)}

	const pad = 50
	canvas := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{10, 12, 16, 255}), image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(pad, pad, pad+900, pad+900), shot, image.Point{}, draw.Over)

	drawText := func(x, y int, text string, col color.RGBA) {
		d := font.Drawer{
			Dst:  canvas,
			Src:  image.NewUniform(col),
			Face: uiFace,
			Dot:  fixed.P(x, y),
		}
		d.DrawString(text)
	}
	drawText(pad, 30, tc.title, color.RGBA{225, 225, 225, 255})
	drawText(pad, 960, "Unverified: Lighthouse is locked by game progression; this is a data render, not a live raid.", color.RGBA{235, 180, 80, 255})
	drawText(pad, 982, tc.legend, color.RGBA{150, 150, 150, 255})

	f, err := os.Create(filepath.Join(p.outDir, tc.name+".png"))
	if err != nil {
		return err
	}
	if err := png.Encode(f, canvas); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
