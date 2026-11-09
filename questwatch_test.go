package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Тестовые куски реального push-лога (PvE, 2026-08-30).
const qwQuestStarted = `2026-08-30 14:13:21.307|1.1.0.1.46911|Info|push-notifications|Got notification | ChatMessageReceived
{
  "type": "new_message",
  "eventId": "6a941051ef1f2d15612b2479",
  "dialogId": "54cb50c76803fa8b248b4571",
  "message": {
    "_id": "6a941051720b9c2f710634dc19",
    "uid": "54cb50c76803fa8b248b4571",
    "type": 12,
    "dt": 1788088401,
    "text": "quest started",
    "templateId": "61604635c725987e815b1a46 successMessageText 54cb50c76803fa8b248b4571 0",
    "items": {
      "stash": "6a941051720b9c2f710634dd",
      "data": [
        { "_id": "x", "_tpl": "5449016a4bdc2d6f028b456f" }
      ]
    }
  }
}
`

const qwQuestFinished = `2026-08-30 14:20:00.000|1.1.0.1.46911|Info|push-notifications|Got notification | ChatMessageReceived
{
  "type": "new_message",
  "message": {
    "type": 12,
    "text": "quest finished",
    "templateId": "61604635c725987e815b1a46 successMessageText 54cb50c76803fa8b248b4571 0"
  }
}
`

const qwUserConfirmed = `2026-08-30 14:15:10.220|1.1.0.1.46911|Info|push-notifications|Got notification | UserConfirmed
{
  "type": "userConfirmed",
  "eventId": "6a9410befe69a36fbc252624",
  "profileid": "669a8b94fff0fc592206d6f3",
  "status": "Busy",
  "location": "bigmap",
  "raidMode": "Online",
  "mode": "deathmatch",
  "shortId": "2H443W",
  "additional_info": []
}
`

// Сценарий «игра уже запущена, вотчер стартует, потом приходит событие»:
// история не читается (fresh), дописанный хвост парсится,
// quest started → tracked, userConfirmed.location → карта.
func TestQuestWatchTail(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "log_2026.08.30_14-09-52_x")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "2026.08.30_14-09-52_x push-notifications_000.log")
	if err := os.WriteFile(logPath, []byte("2026-08-30 14:00:00|Info|push-notifications|start\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	var saved [][]string
	qw := NewQuestWatch(root, md, func(ids []string) { saved = append(saved, ids) })
	defer qw.Close()

	time.Sleep(1200 * time.Millisecond) // первый тик: fresh-захват хвоста
	if qw.IsTracked("61604635c725987e815b1a46") {
		t.Fatal("istoriya dolzhna byt proignorirovana (fresh)")
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(qwQuestStarted); err != nil {
		t.Fatal(err)
	}
	f.Close()
	qw.pump()
	if !qw.IsTracked("61604635c725987e815b1a46") {
		t.Fatal("quest started ne aktivirovan")
	}
	if len(saved) == 0 || len(saved[len(saved)-1]) != 1 {
		t.Fatalf("onSave ne vyzvan: %v", saved)
	}

	f, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(qwQuestFinished); err != nil {
		t.Fatal(err)
	}
	f.Close()
	qw.pump()
	if !qw.IsTracked("61604635c725987e815b1a46") {
		t.Fatal("quest finished snyal tracking")
	}

	f, err = os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(qwUserConfirmed); err != nil {
		t.Fatal(err)
	}
	f.Close()
	qw.pump()
	if key := md.Wanted(); key != "customs" {
		t.Fatalf("userConfirmed -> %q, want customs", key)
	}

	qw.pump()
	if got := qw.Tracked(); len(got) != 1 || got[0] != "61604635c725987e815b1a46" {
		t.Fatalf("tracked = %v", got)
	}
}

// JSON, пришедший двумя кусками (между тиками), собирается по балансу скобок;
// новый log_* (рестарт игры) подхватывается и читается с нуля.
func TestQuestWatchSplitJSONAndRotate(t *testing.T) {
	root := t.TempDir()
	logDir := filepath.Join(root, "log_x")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(logDir, "x push-notifications_000.log")
	if err := os.WriteFile(logPath, []byte("head\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	qw := NewQuestWatch(root, md, nil)
	defer qw.Close()
	time.Sleep(1100 * time.Millisecond)
	qw.pump()

	// Рестарт игры: более новый каталог log_y.
	logDir2 := filepath.Join(root, "log_y")
	if err := os.MkdirAll(logDir2, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath2 := filepath.Join(logDir2, "y push-notifications_000.log")
	half := len(qwQuestStarted) / 2
	if err := os.WriteFile(logPath2, []byte(qwQuestStarted[:half]), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // тик: захват log_y (новый — с нуля)
	qw.pump()
	if qw.IsTracked("61604635c725987e815b1a46") {
		t.Fatal("polovina JSON ne dolzhna parsit'sya")
	}

	af, err := os.OpenFile(logPath2, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := af.WriteString(qwQuestStarted[half:]); err != nil {
		t.Fatal(err)
	}
	af.Close()
	qw.pump()
	if !qw.IsTracked("61604635c725987e815b1a46") {
		t.Fatal("vtoraya polovina JSON ne sobralas")
	}
}

// Алиасы локаций → ключи карт. Нормализация: lowercase + без '_'/пробелов
// (application-лог пишет "Rezerv_Base", userConfirmed — "factory4_day").
func TestMapAliases(t *testing.T) {
	cases := map[string]string{
		"bigmap":         "customs",
		"BigMap":         "customs",
		"factory4_day":   "factory",
		"factory4_night": "factory",
		"rezervbase":     "reserve",
		"Rezerv_Base":    "reserve",
		"laboratory":     "lab",
		"tarkovcity":     "streetsoftarkov",
		"sandbox":        "groundzero",
		"sandbox_high":   "groundzero",
		"lighthouse":     "lighthouse",
		"Lighthouse":     "lighthouse",
		"Shopping_Mall":  "interchange", // rcid:Shopping_Mall
		"shoppingmall":   "interchange",
		"woods_night":    "woods",
		"":               "",
		"unknown_zone":   "",
	}
	for loc, want := range cases {
		if got := mapKeyFromLocation(loc); got != want {
			t.Errorf("mapKeyFromLocation(%q) = %q, want %q", loc, got, want)
		}
	}
}

// Строки "scene preset" из application-лога → карта. Пишутся во всех режимах
// рейда (оффлайн в т.ч.), поэтому детект работает и без userConfirmed.
func TestScenePresetLine(t *testing.T) {
	cases := map[string]string{
		"2026-08-31 09-27-10.412|1.1.0.1.46911|Info|EFT|scene preset path:maps/bigmap_preset.bundle rcid:bigmap.scenespreset.asset loaded":         "customs",
		"2026-08-31 09-30-00.000|1.1.0.1.46911|Info|EFT|scene preset path:maps/rezerv_preset.bundle rcid:Rezerv_Base.scenespreset.asset loaded":    "reserve",
		"2026-08-31 09-31-00.000|1.1.0.1.46911|Info|EFT|scene preset path:maps/sandbox_preset.bundle rcid:Sandbox_high.scenespreset.asset loaded":  "groundzero",
		"2026-08-31 09-32-00.000|1.1.0.1.46911|Info|EFT|scene preset path:maps/lighthouse_preset.bundle rcid:Lighthouse.scenespreset.asset loaded": "lighthouse",
		"2026-08-31 09-33-00.000|1.1.0.1.46911|Debug|EFT|no scene here":                                                                            "",
		"2026-08-31 09-34-00.000|1.1.0.1.46911|Info|EFT|scene preset path:maps/arena_preset.bundle rcid:Unknown_Map.scenespreset.asset loaded":     "",
	}
	for line, want := range cases {
		if got := mapFromSceneLine(line); got != want {
			t.Errorf("mapFromSceneLine(%q) = %q, want %q", line, got, want)
		}
	}
}

// Скан истории: «quest started» из старых сессий log_* → tracked ([AUTO]);
// карта из истории НЕ применяется; повторный скан дубликатов не даёт.
func TestQuestWatchScanHistory(t *testing.T) {
	root := t.TempDir()
	d1 := filepath.Join(root, "log_2026.08.30_14-09-52")
	d2 := filepath.Join(root, "log_2026.08.31_09-28-44")
	for _, d := range []string{d1, d2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(d1, "push-notifications_000.log"),
		[]byte(qwQuestStarted), 0o644); err != nil {
		t.Fatal(err)
	}
	started2 := strings.Replace(qwQuestStarted,
		"61604635c725987e815b1a46", "5447a8654bdc2dfc338f4a0f", -1)
	if err := os.WriteFile(filepath.Join(d2, "push-notifications_000.log"),
		[]byte(started2), 0o644); err != nil {
		t.Fatal(err)
	}

	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	qw := NewQuestWatch(root, md, nil)
	defer qw.Close()
	if n := qw.ScanHistory(); n != 2 {
		t.Fatalf("ScanHistory = %d, want 2", n)
	}
	if !qw.IsTracked("61604635c725987e815b1a46") || !qw.IsTracked("5447a8654bdc2dfc338f4a0f") {
		t.Fatal("kvesty iz istorii ne otslezhivayutsya")
	}
	if n := qw.ScanHistory(); n != 0 {
		t.Fatalf("povtornyy skan = %d, want 0", n)
	}
	if key := md.Wanted(); key != "" {
		t.Fatalf("karta primenilas iz istorii: %s", key)
	}
}

// Трекинг: restore из конфига, ручное включение/выключение, порядок, persist.
func TestQuestWatchTrackedPersist(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	var saved [][]string
	qw := NewQuestWatch("", md, func(ids []string) { saved = append(saved, ids) })
	defer qw.Close()
	qw.RestoreTracked([]string{"111111111111111111111111", "222222222222222222222222"})
	qw.SetTracked("333333333333333333333333", true)
	qw.SetTracked("222222222222222222222222", false)
	got := qw.Tracked()
	if len(got) != 2 || got[0] != "111111111111111111111111" || got[1] != "333333333333333333333333" {
		t.Fatalf("tracked = %v", got)
	}
	if last := saved[len(saved)-1]; len(last) != 2 {
		t.Fatalf("saved = %v", last)
	}
	qw.SetTracked("111111111111111111111111", false)
	qw.SetTracked("333333333333333333333333", false)
	if n := len(qw.Tracked()); n != 0 {
		t.Fatalf("tracked ne pust: %v", qw.Tracked())
	}
}

// Реальный fixture-лог (если есть): повторный полный прогон pump по копии.
func TestQuestWatchOnRealFixture(t *testing.T) {
	if _, err := os.Stat("LogsFromGame"); err != nil {
		t.Skip("net LogsFromGame")
	}
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	qw := NewQuestWatch("LogsFromGame", md, nil)
	defer qw.Close()
	qw.pump() // fresh — только захват хвоста, событий быть не должно
	if n := len(qw.Tracked()); n != 0 {
		t.Fatalf("fixture history prochitan: %v", qw.Tracked())
	}
}

// Сортировка сессий: час без ведущего нуля («9-…» > «14-…» строкой) не должен
// уводить вотчер на старую сессию — время парсится из имени log_*.
func TestSessionDirsOrder(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "log_2026.08.31_9-28-44_1.1.0.1.46911")
	fresh := filepath.Join(root, "log_2026.08.31_14-22-33_1.1.0.1.46911")
	junk := filepath.Join(root, "log_bez_daty")
	for _, d := range []string{old, fresh, junk} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{old, fresh} {
		name := filepath.Base(d) + " push-notifications_000.log"
		if err := os.WriteFile(filepath.Join(d, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	dirs := sessionDirs(root)
	ifresh, iold := -1, -1
	for i, d := range dirs {
		switch {
		case strings.HasSuffix(d, "14-22-33_1.1.0.1.46911"):
			ifresh = i
		case strings.HasSuffix(d, "9-28-44_1.1.0.1.46911"):
			iold = i
		}
	}
	if ifresh < 0 || iold < 0 {
		t.Fatalf("sessionDirs = %v", dirs)
	}
	if ifresh > iold {
		t.Fatalf("novaya sessiya nizhe staroy (chas bez nulya): %v", dirs)
	}
	push, _, ok := sessionLogs(root)
	if !ok || !strings.Contains(push, "14-22-33") {
		t.Fatalf("sessionLogs vzyal %q (ok=%v)", push, ok)
	}
}
