package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/fsnotify/fsnotify"
)

// shotRe: реальный формат скриншотов EFT:
//
//	2026-08-30[17-30]_388.36, 2.64, -114.48_0.00027, 0.98771, -0.00171, 0.15626_8.60 (0).png
//	└─дата─┘└время┘ └─── позиция X,Y,Z ──┘ └────── кватернион (qx,qy,qz,qw) ──────┘└тайм┘└сч.┘
//
// Файлы без координат (меню: «2026-08-30[17-27] (0).png») не матчатся.
var shotRe = regexp.MustCompile(
	`^\d{4}-\d{2}-\d{2}\[\d{2}-\d{2}\]_` +
		`(-?[\d.]+),\s*(-?[\d.]+),\s*(-?[\d.]+)_` +
		`(-?[\d.]+),\s*(-?[\d.]+),\s*(-?[\d.]+),\s*(-?[\d.]+)_` +
		`.*?\(\d+\)\.png$`)

// ParseScreenshotName разбирает имя скриншота: X, Y, Z и yaw из кватерниона.
func ParseScreenshotName(name string) (x, y, z, rotY float64, ok bool) {
	m := shotRe.FindStringSubmatch(name)
	if m == nil {
		return 0, 0, 0, 0, false
	}
	x, y, z = mustAtof(m[1]), mustAtof(m[2]), mustAtof(m[3])
	rotY = YawFromQuat(mustAtof(m[4]), mustAtof(m[5]), mustAtof(m[6]), mustAtof(m[7]))
	return x, y, z, rotY, true
}

func mustAtof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

var (
	ole32                    = syscall.NewLazyDLL("ole32.dll")
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	procSHGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

// FOLDERID_Documents {FDD39AD0-238F-46AF-ADB4-6C85480369C7}
var guidDocuments = syscall.GUID{
	Data1: 0xFDD39AD0,
	Data2: 0x238F,
	Data3: 0x46AF,
	Data4: [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7},
}

func knownFolderDocuments() (string, error) {
	var p *uint16
	r, _, _ := procSHGetKnownFolderPath.Call(
		uintptr(unsafe.Pointer(&guidDocuments)), 0, 0, uintptr(unsafe.Pointer(&p)))
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	if r != 0 {
		return "", fmt.Errorf("SHGetKnownFolderPath: 0x%x", r)
	}
	return utf16PtrToString(p), nil
}

// ScreenshotsDir — автоопределение папки скриншотов EFT.
// Known-folder Documents (учитывает перенаправление в OneDrive) + фоллбэки;
// возвращается первый существующий кандидат, иначе первый как путь «на будущее».
func ScreenshotsDir() (string, error) {
	home, _ := os.UserHomeDir()
	var candidates []string
	if kd, err := knownFolderDocuments(); err == nil && kd != "" {
		candidates = append(candidates, filepath.Join(kd, "Escape from Tarkov", "Screenshots"))
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "Documents", "Escape from Tarkov", "Screenshots"),
			filepath.Join(home, "OneDrive", "Documents", "Escape from Tarkov", "Screenshots"))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			return c, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return "", errors.New("не удалось определить папку скриншотов")
}

// Watcher — наблюдение за папкой скриншотов с возможностью рестарта.
type Watcher struct {
	w    *fsnotify.Watcher
	stop chan struct{}
	Dir  string
}

func (wt *Watcher) Close() error {
	close(wt.stop)
	return wt.w.Close()
}

// StartWatcher запускает fsnotify-наблюдение и стартовый скан новейшего скриншота.
func StartWatcher(dir string, st *State, md *MapData) (*Watcher, error) {
	if _, err := os.Stat(dir); err == nil {
		if name, ok := newestScreenshot(dir); ok {
			applyShot(name, st, md)
		}
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := w.Add(dir); err != nil {
		w.Close()
		return nil, err
	}
	wt := &Watcher{w: w, stop: make(chan struct{}), Dir: dir}
	go watchLoop(wt, st, md)
	return wt, nil
}

func watchLoop(wt *Watcher, st *State, md *MapData) {
	for {
		select {
		case <-wt.stop:
			return
		case ev, ok := <-wt.w.Events:
			if !ok {
				return
			}
			if !ev.Has(fsnotify.Create) && !ev.Has(fsnotify.Write) {
				continue
			}
			applyShot(filepath.Base(ev.Name), st, md)
		case _, ok := <-wt.w.Errors:
			if !ok {
				return
			}
		}
	}
}

func applyShot(name string, st *State, md *MapData) {
	x, y, z, rotY, ok := ParseScreenshotName(name)
	if !ok {
		if strings.HasSuffix(strings.ToLower(name), ".png") {
			dbgLog("skip (net koordinat): %s", name)
		}
		return
	}
	st.SetPosition(x, y, z, rotY)
	prev := md.Wanted()
	key, found := md.Detect(x, z, prev)
	if !found {
		// Точка вне всех карт — карту не меняем, позиция обновилась.
		dbgLog("map: unknown (X,Z=%.1f,%.1f) — derzhim %q", x, z, prev)
		return
	}
	if key != prev {
		log.Printf("map: %s (X,Z=%.1f,%.1f yaw=%.0f)", key, x, z, rotY)
	}
	md.SetWanted(key)
	dbgLog("shot %s\n\tx=%.2f y=%.2f z=%.2f yaw=%.2f -> map=%s", name, x, y, z, rotY, key)
}

// newestScreenshot возвращает имя новейшего подходящего файла в dir.
func newestScreenshot(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var bestName string
	var bestMod int64
	for _, e := range entries {
		if e.IsDir() || !shotRe.MatchString(strings.ToLower(e.Name())) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().UnixNano(); mt >= bestMod {
			bestMod, bestName = mt, e.Name()
		}
	}
	return bestName, bestName != ""
}
