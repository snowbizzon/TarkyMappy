package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	want := Config{
		ScreenshotsDir: `C:\Games\EFT\Screenshots`, Zoom: 2.5, Visible: false,
		ShowPmc: true, ShowScav: false, ShowShared: true,
		ManualMap: true, ManualKey: "customs", LastMap: "customs",
		DebugLog: true,
		WinX:     100, WinY: 200, WinW: 320, WinH: 300,
		Lang: "ru", // LoadConfigWith валидирует и проставляет локаль
	}
	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, p := LoadConfigWith(path)
	if p != path {
		t.Fatalf("path = %q", p)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestLoadConfigDefaultsWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	got, _ := LoadConfigWith(path)
	if !reflect.DeepEqual(got, DefaultConfig()) {
		t.Fatalf("got %+v, want default %+v", got, DefaultConfig())
	}
}

func TestLoadConfigToleratesBOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	// PowerShell 5 / Notepad пишут UTF-8 с BOM.
	body := []byte{0xEF, 0xBB, 0xBF}
	body = append(body, []byte(`{"screenshotsDir":"C:\\shots","zoom":2,"visible":false}`)...)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadConfigWith(path)
	// Полей show*/debugLog/win* в JSON нет — должны остаться значения по умолчанию.
	want := DefaultConfig()
	want.ScreenshotsDir, want.Zoom, want.Visible = `C:\shots`, 2, false
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
