package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// QuestWatch — слежение за логами EFT (%Logs%\log_*\): push-notifications
// (type:12 "quest started" → AUTO, userConfirmed.location → карта) и
// application ("scene preset path:maps/... rcid:<LOC>.scenespreset" → карта,
// пишется во всех режимах: оффлайн/PvE/онлайн). Логи только дописываются,
// поэтому читаем приращения по offset (tail -f, тик 1 с).
type QuestWatch struct {
	mu      sync.Mutex
	tracked map[string]bool // questId -> tracked (AUTO из лога + ручные из панели)
	auto    map[string]bool // включены логом (метка [AUTO] в панели)
	order   []string        // порядок включения (стабильный список панели)

	dir      string
	pushFile string // текущий push-notifications-лог
	pushOff  int64  // сколько байт уже разобрано
	appFile  string // текущий application-лог
	appOff   int64
	fresh    bool // найденные на старте файлы — читать с конца (историю не трогаем)

	collect scanState // сборка JSON push-лога живёт между тиками (куски JSON)

	stop   chan struct{}
	md     *MapData
	onSave func(tracked []string)
	rev    atomic.Int64 // счётчик изменений tracked (dirty-frame рендера)
}

// Rev — версия набора tracked (растёт при любом изменении).
func (qw *QuestWatch) Rev() int64 { return qw.rev.Load() }

// scanState — состояние сборки JSON-блока push-лога между вызовами scanPush.
type scanState struct {
	mode  int          // 0 — служебные строки, 1 — ждём '{', 2 — собираем JSON
	depth int          // баланс скобок собираемого блока
	buf   bytes.Buffer // накопитель JSON
}

func (st *scanState) reset() {
	st.mode, st.depth = 0, 0
	st.buf.Reset()
}

// NewQuestWatch создаёт вотчер и запускает тик. dir может быть "" — ручной режим.
func NewQuestWatch(dir string, md *MapData, onSave func(tracked []string)) *QuestWatch {
	qw := &QuestWatch{
		tracked: map[string]bool{},
		auto:    map[string]bool{},
		dir:     dir,
		fresh:   true,
		stop:    make(chan struct{}),
		md:      md,
		onSave:  onSave,
	}
	go qw.loop()
	return qw
}

func (qw *QuestWatch) Close() {
	select {
	case <-qw.stop:
	default:
		close(qw.stop)
	}
}

// SetDir — смена папки логов (из F9). Пустая строка — ручной режим.
func (qw *QuestWatch) SetDir(dir string) {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	if qw.dir == dir {
		return
	}
	qw.dir = dir
	qw.pushFile, qw.pushOff = "", 0
	qw.appFile, qw.appOff = "", 0
	qw.fresh = true
	qw.collect.reset()
}

// --- Трекинг (панель + автоактивация) ---

// Tracked — id отслеживаемых квестов в порядке включения.
func (qw *QuestWatch) Tracked() []string {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	return append([]string(nil), qw.order...)
}

// IsTracked — отслеживается ли квест id.
func (qw *QuestWatch) IsTracked(id string) bool {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	return qw.tracked[id]
}

// SetTracked — включить/выключить отслеживание (панель, вручную).
func (qw *QuestWatch) SetTracked(id string, on bool) {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	if !on {
		delete(qw.auto, id)
	}
	qw.setLocked(id, on)
}

// IsAuto — квест был активирован автоматически из лога.
func (qw *QuestWatch) IsAuto(id string) bool {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	return qw.auto[id]
}

// RestoreTracked — начальная загрузка из конфига.
func (qw *QuestWatch) RestoreTracked(ids []string) {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	for _, id := range ids {
		qw.setLocked(id, true)
	}
}

func (qw *QuestWatch) setLocked(id string, on bool) {
	if id == "" {
		return
	}
	if on {
		if !qw.tracked[id] {
			qw.tracked[id] = true
			qw.order = append(qw.order, id)
			qw.rev.Add(1)
			qw.savedLocked()
		}
		return
	}
	if qw.tracked[id] {
		delete(qw.tracked, id)
		for i, o := range qw.order {
			if o == id {
				qw.order = append(qw.order[:i], qw.order[i+1:]...)
				break
			}
		}
		qw.rev.Add(1)
		qw.savedLocked()
	}
}

func (qw *QuestWatch) savedLocked() {
	if qw.onSave != nil {
		qw.onSave(append([]string(nil), qw.order...))
	}
}

// --- Тик и чтение лога ---

func (qw *QuestWatch) loop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-qw.stop:
			return
		case <-t.C:
			qw.pump()
		}
	}
}

// sessionDirs — каталоги log_* в dir, новейшие сверху. Время берём из имени
// log_YYYY.MM.DD_HH-MM-SS_…: час без ведущего нуля ломает лексикографику
// («9-28-44» строкой больше «14-22-33» — вотчер цеплялся за мёртвую сессию);
// не разобранное имя — fallback на mtime каталога.
func sessionDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	type dirTS struct {
		path string
		ts   int64
	}
	dirs := make([]dirTS, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "log_") {
			continue
		}
		ts := sessionDirTime(e.Name())
		if ts == 0 {
			if fi, err := e.Info(); err == nil {
				ts = fi.ModTime().UnixNano()
			}
		}
		dirs = append(dirs, dirTS{filepath.Join(dir, e.Name()), ts})
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].ts > dirs[j].ts })
	out := make([]string, len(dirs))
	for i, d := range dirs {
		out[i] = d.path
	}
	return out
}

// sessionDirTime — время из имени «log_2026.08.31_14-22-33_1.1.0.1» (0 — не разобрано).
func sessionDirTime(name string) int64 {
	parts := strings.SplitN(strings.TrimPrefix(name, "log_"), "_", 3)
	if len(parts) < 2 {
		return 0
	}
	d, tm := strings.Split(parts[0], "."), strings.Split(parts[1], "-")
	if len(d) != 3 || len(tm) != 3 {
		return 0
	}
	n := make([]int, 6)
	for i, s := range append(d, tm...) {
		v, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		n[i] = v
	}
	return time.Date(n[0], time.Month(n[1]), n[2], n[3], n[4], n[5], 0, time.UTC).UnixNano()
} // sessionLogs — пара логов новейшей сессии: push-notifications и application.
func sessionLogs(dir string) (push, app string, ok bool) {
	for _, d := range sessionDirs(dir) {
		files, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		var pushMod, appMod int64
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".log") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			if mt := info.ModTime().UnixNano(); mt >= pushMod && strings.Contains(f.Name(), "push-notifications") {
				pushMod, push = mt, filepath.Join(d, f.Name())
			}
			if mt := info.ModTime().UnixNano(); mt >= appMod && strings.Contains(f.Name(), "application") {
				appMod, app = mt, filepath.Join(d, f.Name())
			}
		}
		if push != "" || app != "" {
			return push, app, true
		}
	}
	return "", "", false
}

// trackFile — смена наблюдаемого файла: найденный на старте (fresh) читаем с
// конца, появившийся позже (рестарт игры) — с нуля.
func (qw *QuestWatch) trackFile(path *string, off *int64, target string) {
	if target == "" || target == *path {
		return
	}
	if qw.fresh {
		if fi, err := os.Stat(target); err == nil {
			*off = fi.Size()
		}
	} else {
		*off = 0
	}
	*path = target
	log.Printf("questwatch: %s", target)
}

func (qw *QuestWatch) pump() {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	if qw.dir != "" {
		push, app, ok := sessionLogs(qw.dir)
		if ok {
			prev := qw.pushFile
			qw.trackFile(&qw.pushFile, &qw.pushOff, push)
			if prev != qw.pushFile { // хвосты разных файлов не смешиваем
				qw.collect.reset()
			}
			qw.trackFile(&qw.appFile, &qw.appOff, app)
		}
	}
	qw.fresh = false
	qw.readPush()
	qw.readApp()
}

// readPush — приращение push-лога: "Got notification" + собираемый JSON.
func (qw *QuestWatch) readPush() {
	if qw.pushFile == "" {
		return
	}
	fi, err := os.Stat(qw.pushFile)
	if err != nil || fi.Size() <= qw.pushOff {
		return
	}
	f, err := os.Open(qw.pushFile)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(qw.pushOff, io.SeekStart); err != nil {
		return
	}
	qw.pushOff += scanPush(f, &qw.collect, qw.dispatch)
}

// scanPush — построчный разбор push-лога: маркер "Got notification", затем
// JSON (может приходить кусками между тиками — состояние в *st; хвост без
// '\n' недописан — дочитаем в следующий раз). Возвращает разобранные байты.
func scanPush(r io.Reader, st *scanState, emit func(body string)) int64 {
	br := bufio.NewReader(r)
	const gotMark = "Got notification"
	var off int64
	for {
		line, err := br.ReadString('\n')
		if len(line) == 0 || err != nil && !strings.HasSuffix(line, "\n") {
			return off
		}
		off += int64(len(line))
		switch st.mode {
		case 0:
			if strings.Contains(line, gotMark) {
				st.mode = 1
			}
		case 1:
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "{") {
				st.buf.Reset()
				st.buf.WriteString(t)
				st.depth = bracketDelta(t)
				st.mode = 2
			}
		case 2:
			st.buf.WriteByte('\n')
			st.buf.WriteString(line)
			st.depth += bracketDelta(line)
			if st.depth <= 0 {
				emit(st.buf.String())
				st.buf.Reset()
				st.mode = 0
			}
		}
	}
}

// readApp — приращение application-лога: строка "scene preset path:maps/…"
// пишется при загрузке сцены рейда во всех режимах (оффлайн в том числе).
func (qw *QuestWatch) readApp() {
	if qw.appFile == "" {
		return
	}
	fi, err := os.Stat(qw.appFile)
	if err != nil || fi.Size() <= qw.appOff {
		return
	}
	f, err := os.Open(qw.appFile)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(qw.appOff, io.SeekStart); err != nil {
		return
	}
	br := bufio.NewReader(f)
	for {
		line, err := br.ReadString('\n')
		if len(line) == 0 || err != nil && !strings.HasSuffix(line, "\n") {
			return
		}
		qw.appOff += int64(len(line))
		if key := mapFromSceneLine(line); key != "" {
			qw.setMapLocked(key, "scene preset")
		}
	}
}

// mapFromSceneLine — "…scene preset path:maps/customs_preset.bundle
// rcid:bigmap.scenespreset.asset" → "customs" (rcid = имя локации игры).
func mapFromSceneLine(line string) string {
	if !strings.Contains(line, "scene preset path:maps/") {
		return ""
	}
	const mark = "rcid:"
	i := strings.Index(line, mark)
	if i < 0 {
		return ""
	}
	rest := line[i+len(mark):]
	if j := strings.IndexAny(rest, ". \t\r\n"); j >= 0 {
		rest = rest[:j]
	}
	return mapKeyFromLocation(rest)
}

// bracketDelta — баланс фигурных скобок строки.
func bracketDelta(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			n++
		case '}':
			n--
		}
	}
	return n
}

// --- Разбор событий ---

type pushEnvelope struct {
	Type string `json:"type"`
}

type questStartedMsg struct {
	Type     string `json:"type"`
	DialogID string `json:"dialogId"`
	Message  struct {
		Type       int    `json:"type"`
		Text       string `json:"text"`
		TemplateID string `json:"templateId"`
	} `json:"message"`
}

type userConfirmedMsg struct {
	Type     string `json:"type"`
	Location string `json:"location"`
	RaidMode string `json:"raidMode"`
}

// logMapAliases — имя локации в логе → ключ карты maps.json. Ключи словаря
// нормализованы (lowercase, без '_'): application-лог пишет "Rezerv_Base",
// push "userConfirmed" — "bigmap"/"factory4_day".
var logMapAliases = map[string]string{
	"bigmap":        "customs",
	"factory4day":   "factory",
	"factory4night": "factory",
	"rezervbase":    "reserve",
	"laboratory":    "lab",
	"tarkovcity":    "streetsoftarkov",
	"sandbox":       "groundzero",
	"sandboxhigh":   "groundzero",
	"woods":         "woods",
	"shoreline":     "shoreline",
	"lighthouse":    "lighthouse",
	"interchange":   "interchange",
	"shoppingmall":  "interchange", // application-лог: ScenesPreset Shopping_Mall
	"woodsnight":    "woods",       // ночная версия woods (безопасный запас)
}

// normLocKey — нормализация имени локации: lowercase, без '_' и пробелов.
func normLocKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "")
	return strings.ReplaceAll(s, " ", "")
}

func mapKeyFromLocation(loc string) string {
	return logMapAliases[normLocKey(loc)]
}

// trackLocked — включение трекинга; вызывается под qw.mu (из pump→dispatch).
func (qw *QuestWatch) trackLocked(id string) {
	if id == "" || qw.tracked[id] {
		return
	}
	qw.tracked[id] = true
	qw.auto[id] = true
	qw.order = append(qw.order, id)
	qw.rev.Add(1) // dirty-frame: список изменился — панель перерисуется
	qw.savedLocked()
}

// setMapLocked — применение карты от любого источника лога (под qw.mu).
func (qw *QuestWatch) setMapLocked(key, src string) {
	if key == "" {
		return
	}
	if prev := qw.md.Wanted(); prev != key {
		log.Printf("map: %s (%s)", key, src)
	}
	qw.md.SetWanted(key)
}

// ScanHistory — разовый проход по последним ~8 сессиям log_*: все
// "quest started" из истории → tracked ([AUTO]). Карту из истории не берём.
// Возвращает число новых отслеживаемых квестов.
func (qw *QuestWatch) ScanHistory() int {
	qw.mu.Lock()
	defer qw.mu.Unlock()
	if qw.dir == "" {
		return 0
	}
	dirs := sessionDirs(qw.dir)
	if len(dirs) > 8 {
		dirs = dirs[:8]
	}
	n := 0
	for _, d := range dirs {
		files, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.Contains(f.Name(), "push-notifications") ||
				!strings.HasSuffix(f.Name(), ".log") {
				continue
			}
			file, err := os.Open(filepath.Join(d, f.Name()))
			if err != nil {
				continue
			}
			var st scanState
			scanPush(file, &st, func(body string) { n += qw.trackStartedLocked(body) })
			file.Close()
		}
	}
	return n
}

// dispatch разбирает JSON-блок: quest started → AUTO-трекинг,
// userConfirmed → смена карты (мгновенный детект до первого скриншота).
func (qw *QuestWatch) dispatch(body string) {
	var env pushEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return
	}
	switch env.Type {
	case "new_message":
		qw.trackStartedLocked(body)
	case "userConfirmed":
		var m userConfirmedMsg
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			return
		}
		qw.setMapLocked(mapKeyFromLocation(m.Location), "push userConfirmed")
	}
}

// trackStartedLocked — "quest started" из JSON-блока → tracked; 1 если новый.
// Вызывается под qw.mu (pump→dispatch, ScanHistory).
func (qw *QuestWatch) trackStartedLocked(body string) int {
	var env pushEnvelope
	if err := json.Unmarshal([]byte(body), &env); err != nil || env.Type != "new_message" {
		return 0
	}
	var m questStartedMsg
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return 0
	}
	if m.Message.Type != 12 || !strings.Contains(m.Message.Text, "quest started") {
		return 0
	}
	// templateId: "<questId> successMessageText <traderId> 0"
	parts := strings.Fields(m.Message.TemplateID)
	if len(parts) < 3 {
		return 0
	}
	id := parts[0]
	if qw.tracked[id] {
		return 0
	}
	if name, ok := qw.md.QuestByID(id); ok {
		log.Printf("quest: started %s (auto)", name.Name)
	} else {
		log.Printf("quest: started %s (auto, net v sreze)", questFallbackName(id))
	}
	qw.trackLocked(id)
	return 1
}

// questFallbackName — подпись квеста, отсутствующего в срезе tarkov.dev
// (часть id из PvE-профилей там отсутствует).
func questFallbackName(id string) string {
	if len(id) > 8 {
		return "Quest " + id[:8]
	}
	return "Quest " + id
}
