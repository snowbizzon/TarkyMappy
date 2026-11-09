package main

import (
	"bytes"
	_ "embed"
	"image"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/hajimehoshi/ebiten/v2"
)

// Иконка окна (компас) — зашита в бинарник, внешние файлы не нужны.
//
//go:embed icon64.png
var iconPNG []byte

// debugLog — подробный лог (галочка «debug: on» в настройках).
var debugLog bool

func main() {
	// Паника не должна теряться молча (окно без консоли): стек — в лог.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	setupLog()

	st := NewState()
	md, err := NewMapData()
	if err != nil {
		log.Fatalf("data: %v", err)
	}

	cfg, cfgPath := LoadConfig()
	log.Printf("config: %s", cfgPath)
	if cfg.Zoom > 0 {
		st.SetZoom(cfg.Zoom)
	}
	st.SetVisible(cfg.Visible)
	debugLog = cfg.DebugLog

	// Стартуем с карты прошлого сеанса (перезапуск между рейдами — та же карта).
	if cfg.LastMap != "" {
		if _, ok := md.Cfg(cfg.LastMap); ok {
			md.SetWanted(cfg.LastMap)
			log.Printf("lastMap: %s", cfg.LastMap)
		}
	}

	// Окно: прозрачное, без рамки, поверх всех, клики насквозь.
	applyWindowGeometry(cfg)
	// Энергосбережение: логика работает на 30 тиках/с, а dirty-frame
	// пропускает перерисовку, пока состояние не изменилось. Экран хранит
	// предыдущий кадр, поэтому GPU в простое почти не нагружается.
	ebiten.SetMaxTPS(30)
	ebiten.SetScreenClearedEveryFrame(false)
	// Vsync off: FPS и так ограничен TPS, а лишний present в простое
	// (dirty-frame пропускает отрисовку) греет GPU без пользы.
	ebiten.SetVsyncEnabled(false)
	ebiten.SetWindowTitle("TarkyMappy")
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowMousePassthrough(true)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	if img, _, err := image.Decode(bytes.NewReader(iconPNG)); err == nil {
		ebiten.SetWindowIcon([]image.Image{img}) // иконку ставит сам ebiten (безопасный тред)
	} else {
		log.Printf("icon: %v", err)
	}

	// Папка скриншотов: из конфига, иначе автоопределение.
	dir := cfg.ScreenshotsDir
	if dir == "" {
		if d, err := ScreenshotsDir(); err == nil {
			dir = d
		} else {
			log.Printf("screenshots dir: %v", err)
		}
	} else if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		log.Printf("config dir net: %s", dir)
		if d, err := ScreenshotsDir(); err == nil {
			dir = d
		}
	}

	var wt *Watcher
	if dir != "" {
		if w, err := StartWatcher(dir, st, md); err == nil {
			wt = w
			log.Printf("watching %s", dir)
		} else {
			log.Printf("watcher: %v", err)
		}
	}
	// Зафиксировать фактическую папку в конфиге (создать его при первом запуске).
	cfg.ScreenshotsDir = dir
	if err := SaveConfig(cfgPath, cfg); err != nil {
		log.Printf("config save: %v", err)
	}

	// Quest Helper: вотчер push-логов EFT (quest started → AUTO-трекинг,
	// userConfirmed → мгновенный детект карты). Пустая папка — ручной режим.
	qw := NewQuestWatch(cfg.LogsDir, md, func(ids []string) {
		if c, p := LoadConfig(); p != "" {
			c.TrackedQuests = ids
			if err := SaveConfig(p, c); err != nil {
				log.Printf("config save: %v", err)
			}
		}
	})
	qw.RestoreTracked(cfg.TrackedQuests)
	// Backscan истории при каждом старте (не только при пустом tracked):
	// «мои» квесты подтягиваются сами, оффлайн-рейды push «quest started»
	// не пишут. trackLocked пропускает уже отслеживаемые — дубликатов нет.
	if cfg.LogsDir != "" {
		if n := qw.ScanHistory(); n > 0 {
			log.Printf("quest: avtoskan istorii: +%d", n)
		}
	}
	defer qw.Close()

	g := NewGame(st, md, cfg, cfgPath, func(newDir string) error {
		if wt != nil {
			wt.Close()
		}
		w, err := StartWatcher(newDir, st, md)
		if err != nil {
			wt = nil
			return err
		}
		wt = w
		log.Printf("watching %s", newDir)
		return nil
	}, qw)
	g.qw.SetDir(cfg.LogsDir)
	err = ebiten.RunGameWithOptions(g, &ebiten.RunGameOptions{
		ScreenTransparent: true,
		SkipTaskbar:       false, // иконка в панели задач: закрытие Alt+F4 / ПКМ по превью
	})
	if err != nil && err != ebiten.Termination {
		log.Printf("run: %v", err)
	}
	if wt != nil {
		wt.Close()
	}
	// Финальный lastMap: карта могла смениться автодетектом за сеанс.
	// Конфиг перечитываем — Game писал в него геометрию из своей копии.
	if c, p := LoadConfig(); p != "" {
		if key := md.Wanted(); key != "" && key != c.LastMap {
			c.LastMap = key
			if err := SaveConfig(p, c); err != nil {
				log.Printf("config save: %v", err)
			}
		}
	}
}

// applyWindowGeometry — размер и позиция окна из конфига с клампом в экран.
func applyWindowGeometry(cfg Config) {
	sw, sh := ebiten.ScreenSizeInFullscreen()
	w, h := winW, winH
	if cfg.WinW >= minWinW && cfg.WinH >= minWinH {
		w, h = clampI(cfg.WinW, minWinW, sw), clampI(cfg.WinH, minWinH, sh)
	}
	ebiten.SetWindowSize(w, h)
	ebiten.SetWindowSizeLimits(minWinW, minWinH, sw, sh)
	if cfg.WinX != 0 || cfg.WinY != 0 {
		ebiten.SetWindowPosition(clampI(cfg.WinX, -w+60, sw-60), clampI(cfg.WinY, 0, sh-60))
	}
}

// setupLog пишет лог рядом с exe (tarkymappy.log) — окно без консоли.
func setupLog() {
	path := "tarkymappy.log"
	if exe, err := os.Executable(); err == nil {
		path = filepath.Join(filepath.Dir(exe), "tarkymappy.log")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err == nil {
		log.SetOutput(f)
	}
}

// dbgLog — печатает в лог только при включённом debug-режиме.
func dbgLog(format string, args ...any) {
	if debugLog {
		log.Printf(format, args...)
	}
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
