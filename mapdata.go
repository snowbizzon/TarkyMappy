package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // Lighthouse: фотарт reemr
	_ "image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed data/maps.json
var mapsJSON []byte

//go:embed data/exits.json
var exitsJSON []byte

//go:embed data/quests.json
var questsJSON []byte

//go:embed data/locks.json
var locksJSON []byte

//go:embed data/farm.json
var farmJSON []byte

//go:embed data/hazards.json
var hazardsJSON []byte

//go:embed data/loc_ru.json
var locRUJSON []byte

//go:embed data/meta.json
var metaJSON []byte

// DataMeta — паспорт среза данных (какой патч игры, когда собрано, из чего).
// Показывается в FAQ, чтобы было видно, когда данные устарели.
type DataMeta struct {
	GameBuild     string   `json:"gameBuild"`
	DataDate      string   `json:"dataDate"`
	Source        string   `json:"source"`
	MapArt        string   `json:"mapArt"`
	LighthouseArt string   `json:"lighthouseArt"`
	DeferredMaps  []string `json:"deferredMaps"`
}

//go:embed maps/*.png
//go:embed maps/*.jpg
var mapsFS embed.FS

// MapSVG — блок "svg" из maps.json (TarkovTracker).
type MapSVG struct {
	File               string                `json:"file"`
	CoordinateRotation float64               `json:"coordinateRotation"`
	Bounds             [2][2]float64         `json:"bounds"`
	Floors             []string              `json:"floors"`       // этажи снизу вверх (группы SVG)
	DefaultFloor       string                `json:"defaultFloor"` // этаж по умолчанию (ground)
	FloorBounds        map[string][2]float64 `json:"floorBounds"`  // диапазоны Y для автовыбора
}

// MapCfg — конфигурация одной карты.
type MapCfg struct {
	ID     int    `json:"id"`
	TdevID string `json:"tdevId"`
	Locale struct {
		En string `json:"en"`
		Ru string `json:"ru"`
	} `json:"locale"`
	SVG MapSVG `json:"svg"`
}

// Exit — точка выхода/транзита из data/exits.json.
type Exit struct {
	Name    string  `json:"name"`
	Faction string  `json:"faction"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Z       float64 `json:"z"`
}

type mapExits struct {
	Extracts []Exit `json:"extracts"`
	Transits []Exit `json:"transits"`
}

// QuestPoint — точка квеста: visit-зона (с контуром) или позиция предмета.
type QuestPoint struct {
	Kind    string       `json:"kind"` // "visit" | "item"
	Label   string       `json:"label"`
	X       float64      `json:"x"`
	Y       float64      `json:"y"` // высота — выбор этажа
	Z       float64      `json:"z"`
	Outline [][2]float64 `json:"outline,omitempty"` // контур visit-зоны [x,z]
}

// Quest — квест с точками на конкретной карте (из data/quests.json).
type Quest struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Trader     string       `json:"trader"`
	Faction    string       `json:"faction"` // Any | BEAR | USEC
	MinLevel   int          `json:"minLevel"`
	Wiki       string       `json:"wiki"`
	Keys       []string     `json:"keys,omitempty"`       // нужные ключи (названия)
	Objectives []string     `json:"objectives,omitempty"` // полные описания целей
	Points     []QuestPoint `json:"points"`
}

// Lock — дверь/замок на карте, открываемая ключом (из data/locks.json).
type Lock struct {
	Key string  `json:"key"` // название ключа
	X   float64 `json:"x"`
	Y   float64 `json:"y"` // высота — этаж двери
	Z   float64 `json:"z"`
}

// FarmPos — точка спавна игрока (data/farm.json, Помощник фарма).
type FarmPos struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// FarmSpot — лут-точка (контейнер или loose-пул) с категорией фарма.
type FarmSpot struct {
	C string  `json:"c"`           // 't' тех, 'm' мед, 's' продажа, 'h' дорогой тех, 'w' оружейный
	N string  `json:"n,omitempty"` // имя типа контейнера/предмета (подпись в маршруте)
	X float64 `json:"x"`
	Y float64 `json:"y"` // высота — фильтр этажей
	Z float64 `json:"z"`
}

// Hazard — опасность: 'm' минное поле, 's' снайперская зона,
// 'h' прочее, 'c' ловушка/клеймор (Маяк, из арта reemr). Ol — контур зоны.
type Hazard struct {
	T  string       `json:"t"`
	X  float64      `json:"x"`
	Y  float64      `json:"y"`
	Z  float64      `json:"z"`
	Ol [][2]float64 `json:"ol,omitempty"`
}

// farmMapData — данные фарма одной карты.
type farmMapData struct {
	Spawns []FarmPos  `json:"spawns"`
	Spots  []FarmSpot `json:"spots"`
}

// PriceItem — предмет топа барахолки (prices/prices.json, F2 Помощник цен).
type PriceItem struct {
	N  string `json:"n"`  // короткое имя (EN; RU — через md.Loc)
	W  int    `json:"w"`  // ширина в слотах
	H  int    `json:"h"`  // высота в слотах
	P  int    `json:"p"`  // средняя цена за 24ч, ₽
	PP int    `json:"pp"` // цена за 1 слот, ₽
}

// priceData — снимок цен барахолки: дата + топ по обоим режимам
// ("regular" — PvP, "pve"). Обновляется скриптом prices/update_prices.py.
type priceData struct {
	Updated string                 `json:"updated"`
	Modes   map[string][]PriceItem `json:"modes"`
}

type mapQuests struct {
	Quests []Quest `json:"quests"`
}

// MapData — локальные данные карт: конфиги, выходы, квесты, замки,
// кэш изображения текущей карты.
type MapData struct {
	mu      sync.Mutex
	maps    map[string]MapCfg
	names   []string // стабильный порядок ключей (для детерминизма)
	exits   map[string]mapExits
	quests  map[string]mapQuests
	locks   map[string][]Lock
	farm    map[string]farmMapData // Помощник фарма: спавны + лут-точки
	hazards map[string][]Hazard    // опасности: мины/снайперы/ловушки
	prices  priceData              // Помощник цен: топ барахолки (PvP/PvE)
	loc     map[string]string      // RU-подстановки игровых строк
	meta    DataMeta               // паспорт среза данных (FAQ)
	qindex  map[string]string      // questId -> ключ карты (QuestByID)
	qcolors map[string]color.RGBA  // questId -> уникальный цвет (палитра)
	wanted  atomic.Value           // string: карта, соответствующая последней позиции

	curKey string
	curImg *ebiten.Image
}

func NewMapData() (*MapData, error) {
	md := &MapData{}
	if err := json.Unmarshal(mapsJSON, &md.maps); err != nil {
		return nil, fmt.Errorf("maps.json: %w", err)
	}
	if err := json.Unmarshal(exitsJSON, &md.exits); err != nil {
		return nil, fmt.Errorf("exits.json: %w", err)
	}
	if err := json.Unmarshal(questsJSON, &md.quests); err != nil {
		return nil, fmt.Errorf("quests.json: %w", err)
	}
	if len(locksJSON) > 0 {
		if err := json.Unmarshal(locksJSON, &md.locks); err != nil {
			return nil, fmt.Errorf("locks.json: %w", err)
		}
	}
	if len(farmJSON) > 0 {
		if err := json.Unmarshal(farmJSON, &md.farm); err != nil {
			return nil, fmt.Errorf("farm.json: %w", err)
		}
	}
	if len(hazardsJSON) > 0 {
		if err := json.Unmarshal(hazardsJSON, &md.hazards); err != nil {
			return nil, fmt.Errorf("hazards.json: %w", err)
		}
	}
	// Цены барахолки — НЕ embed: читаются из prices/prices.json рядом с exe
	// (обновление без пересборки). Отсутствие файла — не ошибка.
	md.loadPrices()
	if len(locRUJSON) > 0 {
		if err := json.Unmarshal(locRUJSON, &md.loc); err != nil {
			return nil, fmt.Errorf("loc_ru.json: %w", err)
		}
	}
	if err := json.Unmarshal(metaJSON, &md.meta); err != nil {
		return nil, fmt.Errorf("meta.json: %w", err)
	}
	md.qindex = make(map[string]string, 512)
	for key, mq := range md.quests {
		for _, q := range mq.Quests {
			if _, dup := md.qindex[q.ID]; !dup {
				md.qindex[q.ID] = key
			}
		}
	}
	// Цвета: индекс квеста в (алфавитном) списке КАЖДОЙ карты → палитра без
	// совпадений между квестами и с цветами выходов. Один квест может иметь
	// точки на нескольких картах — цвет пары «карта+квест», чтобы внутри
	// видимой карты гарантированно не было дублей.
	md.qcolors = make(map[string]color.RGBA, 512)
	for _, key := range md.sortedMapKeys() {
		for i, q := range md.quests[key].Quests {
			md.qcolors[qcolorKey(key, q.ID)] = questColorFor(i)
		}
	}
	md.names = make([]string, 0, len(md.maps))
	for k := range md.maps {
		md.names = append(md.names, k)
	}
	slices.Sort(md.names)
	return md, nil
}

// sortedMapKeys — ключи карт с квестами в стабильном порядке.
func (md *MapData) sortedMapKeys() []string {
	keys := make([]string, 0, len(md.quests))
	for k := range md.quests {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Names — список ключей карт (отсортированный, стабильный порядок).
func (md *MapData) Names() []string {
	md.mu.Lock()
	defer md.mu.Unlock()
	return append([]string(nil), md.names...)
}

// mapHysteresis — допуск (доля стороны bounds), в пределах которого точка
// «за краем» карты всё ещё считается на ней. Границы карт в данных сильно
// перекрываются, а смена локации в игре возможна только новым рейдом,
// поэтому карта держится, пока точка в её (расширенных) границах.
const mapHysteresis = 0.10

// inBounds проверяет точку (x, z) с допуском margin (доля размера стороны).
func inBounds(b [2][2]float64, x, z, margin float64) bool {
	minX, maxX := b[0][0], b[1][0]
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minZ, maxZ := b[0][1], b[1][1]
	if minZ > maxZ {
		minZ, maxZ = maxZ, minZ
	}
	return x >= minX-(maxX-minX)*margin && x <= maxX+(maxX-minX)*margin &&
		z >= minZ-(maxZ-minZ)*margin && z <= maxZ+(maxZ-minZ)*margin
}

// Detect возвращает карту для точки (x, z).
// Гистерезис: пока точка в расширенных границах карты current — остаёмся на ней.
// Иначе — скоринг margin²/area: компромисс между min-area (прыгал в
// groundzero/lab у краёв) и max-margin (всегда выигрывали lighthouse/woods).
func (md *MapData) Detect(x, z float64, current string) (string, bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	if current != "" {
		if b, ok := md.maps[current]; ok && inBounds(b.SVG.Bounds, x, z, mapHysteresis) {
			return current, true
		}
	}
	best, bestScore := "", -1.0
	for _, key := range md.names {
		b := md.maps[key].SVG.Bounds
		u := (x - b[0][0]) / (b[1][0] - b[0][0])
		v := (z - b[0][1]) / (b[1][1] - b[0][1])
		if u < 0 || u > 1 || v < 0 || v > 1 {
			continue
		}
		m := math.Min(math.Min(u, 1-u), math.Min(v, 1-v))
		area := math.Abs((b[1][0] - b[0][0]) * (b[1][1] - b[0][1]))
		if s := m * m / area; s > bestScore {
			best, bestScore = key, s
		}
	}
	return best, best != ""
}

// floorAll — спец-этаж: combined-рендер всех этажей (фейлсвитч «плоская карта»).
const floorAll = "ALL"

// floorHyst — гистерезис смены этажа (м): текущий этаж держим, пока y не выйдет
// за его диапазон больше чем на floorHyst — без мигания на границах.
const floorHyst = 2.0

// Floors — список этажей карты снизу вверх.
func (md *MapData) Floors(key string) []string {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.maps[key].SVG.Floors
}

// DefaultFloor — этаж по умолчанию: из конфига, иначе Ground* или последний.
func (md *MapData) DefaultFloor(key string) string {
	md.mu.Lock()
	defer md.mu.Unlock()
	c := md.maps[key]
	if c.SVG.DefaultFloor != "" {
		return c.SVG.DefaultFloor
	}
	for _, f := range c.SVG.Floors {
		if strings.Contains(f, "Ground") {
			return f
		}
	}
	if n := len(c.SVG.Floors); n > 0 {
		return c.SVG.Floors[n-1]
	}
	return ""
}

// FloorFor — этаж по высоте y: сначала гистерезис к current, затем точное
// попадание в диапазон FloorBounds; вне диапазонов — DefaultFloor.
func (md *MapData) FloorFor(key string, y float64, current string) string {
	md.mu.Lock()
	defer md.mu.Unlock()
	c := md.maps[key]
	if len(c.SVG.Floors) <= 1 {
		return c.SVG.DefaultFloor
	}
	if !slices.Contains(c.SVG.Floors, current) {
		current = ""
	}
	if current != "" {
		if b, ok := c.SVG.FloorBounds[current]; ok && y >= b[0]-floorHyst && y < b[1]+floorHyst {
			return current
		}
	}
	for _, f := range c.SVG.Floors {
		if b, ok := c.SVG.FloorBounds[f]; ok && y >= b[0] && y < b[1] {
			return f
		}
	}
	return c.SVG.DefaultFloor
}

// EnsureImage возвращает (лениво загрузив и закэшировав) изображение карты key
// на этаже floor. Кэшируется только текущая пара ключ+этаж — RAM в бюджете.
func (md *MapData) EnsureImage(key, floor string) (*ebiten.Image, error) {
	md.mu.Lock()
	defer md.mu.Unlock()
	full := key + "|" + floor
	if full == md.curKey && md.curImg != nil {
		return md.curImg, nil
	}
	img, err := decodeMapPNG(key, floor)
	if err != nil {
		return nil, err
	}
	ni := ebiten.NewImageFromImage(img)
	if md.curImg != nil && md.curImg != ni {
		md.curImg.Deallocate()
		// Старьё после декода большого PNG держит GC в куче (пики RAM до
		// 800МБ) — возвращаем память ОС сразу после смены карты/этажа.
		debug.FreeOSMemory()
	}
	md.curKey, md.curImg = full, ni
	return ni, nil
}

// decodeMapPNG декодирует PNG карты из embed (без ebiten — тестируемо headless).
// Порядок: <key>__<floor>.png, <key>__<floor>.jpg (фото-арт), затем legacy <key>.png.
func decodeMapPNG(key, floor string) (image.Image, error) {
	var lastErr error
	for _, path := range []string{mapPNGPath(key, floor), mapJPEGPath(key, floor), mapPNGPath(key, "")} {
		f, err := mapsFS.Open(path)
		if err != nil {
			lastErr = err
			continue
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			lastErr = fmt.Errorf("decode %s: %w", path, err)
			continue
		}
		return img, nil
	}
	return nil, lastErr
}

func mapPNGPath(key, floor string) string {
	if floor == "" {
		return "maps/" + key + ".png"
	}
	return "maps/" + key + "__" + floor + ".png"
}

// mapJPEGPath — фото-подложки (не векторные): Lighthouse rework (арт reemr).
func mapJPEGPath(key, floor string) string {
	if floor == "" {
		return "maps/" + key + ".jpg"
	}
	return "maps/" + key + "__" + floor + ".jpg"
}

// ExitsFor возвращает выходы и транзиты карты key.
func (md *MapData) ExitsFor(key string) ([]Exit, []Exit) {
	md.mu.Lock()
	defer md.mu.Unlock()
	e := md.exits[key]
	return e.Extracts, e.Transits
}

// Loc — RU-подстановка игровой строки (имена выходов, квестов, целей,
// ключей и лута фарма из loc_ru.json). Только в локали ru; строки, которых
// нет в словаре, остаются EN (осознанный деград вместо пустых подписей).
func (md *MapData) Loc(s string) string {
	if curLang != "ru" || s == "" {
		return s
	}
	md.mu.Lock()
	r, ok := md.loc[s]
	md.mu.Unlock()
	if !ok || r == "" {
		return s
	}
	return r
}

// Meta — паспорт среза данных (патч, дата, источники) для FAQ.
func (md *MapData) Meta() DataMeta {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.meta
}

// Prices — топ предметов барахолки режима ("regular" — PvP, "pve") и дата
// снимка (из шапки prices.json). Возвращает копию — вызывающий может
// менять срез без блокировки.
func (md *MapData) Prices(mode string) ([]PriceItem, string) {
	md.mu.Lock()
	defer md.mu.Unlock()
	src := md.prices.Modes[mode]
	out := make([]PriceItem, len(src))
	copy(out, src)
	return out, md.prices.Updated
}

// pricesPath — обязательный путь данных цен: prices/prices.json рядом с exe.
func pricesPath() string {
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "prices", "prices.json")
	}
	return filepath.Join("prices", "prices.json")
}

// loadPrices перечитывает prices/prices.json рядом с exe (каждое открытие
// F2 — свежие цены без пересборки). Файл отсутствует/битый — не ошибка:
// остаётся пустой список, панель покажет подсказку.
func (md *MapData) loadPrices() { md.loadPricesAt(pricesPath()) }

func (md *MapData) loadPricesAt(path string) {
	var pd priceData
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &pd); err != nil {
			log.Printf("prices: %s: %v", filepath.Base(path), err)
			return
		}
	case os.IsNotExist(err):
		log.Printf("prices: %s нет — запусти prices/update_prices.py", path)
		return
	default:
		log.Printf("prices: %v", err)
		return
	}
	md.mu.Lock()
	md.prices = pd
	md.mu.Unlock()
}

// LocksFor возвращает двери-замки карты key (пустой срез — данных нет).
func (md *MapData) LocksFor(key string) []Lock {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.locks[key]
}

// FarmFor возвращает спавны и лут-точки карты key (данные фарма).
func (md *MapData) FarmFor(key string) ([]FarmPos, []FarmSpot) {
	md.mu.Lock()
	defer md.mu.Unlock()
	f := md.farm[key]
	return f.Spawns, f.Spots
}

// HazardsFor возвращает опасности карты key (мины/снайперы/ловушки).
func (md *MapData) HazardsFor(key string) []Hazard {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.hazards[key]
}

// QuestsFor возвращает квесты с точками на карте key (срез — копия верхнего уровня,
// сами Quest разделяются: данные read-only после загрузки).
func (md *MapData) QuestsFor(key string) []Quest {
	md.mu.Lock()
	defer md.mu.Unlock()
	return md.quests[key].Quests
}

// QuestByID ищет квест по id на любой карте (резолв questId из push-лога).
func (md *MapData) QuestByID(id string) (Quest, bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	key, ok := md.qindex[id]
	if !ok {
		return Quest{}, false
	}
	for _, q := range md.quests[key].Quests {
		if q.ID == id {
			return q, true
		}
	}
	return Quest{}, false
}

// SetWanted устанавливает желаемую карту (вызывается вотчером).
func (md *MapData) SetWanted(key string) { md.wanted.Store(key) }

// Wanted возвращает желаемую карту ("" если позиции ещё нет).
func (md *MapData) Wanted() string {
	v, _ := md.wanted.Load().(string)
	return v
}

// Cfg возвращает конфигурацию карты key.
func (md *MapData) Cfg(key string) (MapCfg, bool) {
	md.mu.Lock()
	defer md.mu.Unlock()
	c, ok := md.maps[key]
	return c, ok
}

// WantedMap определяет карту по последней позиции из st (без побочных эффектов).
func (md *MapData) WantedMap(st *State) string {
	x, _, z, _, ok := st.Pos()
	if !ok {
		return ""
	}
	key, _ := md.Detect(x, z, md.Wanted())
	return key
}

// --- Цвета квестов: палитра с гарантиями ---

// questPaletteHues — 15 оттенков, собранных в обход служебных цветов выходов:
// жёлтый shared ~48°, зелёный scav ~120°, синий PMC ~220°, фиолетовый
// transit ~275° — вокруг них запрещённые зоны ±20°. Соседние оттенки ≥14°,
// так что любые два квеста визуально различимы.
var questPaletteHues = [...]float64{11, 25, 71, 85, 99, 144, 158, 172, 186, 247, 299, 313, 327, 341, 355}

// questColorV — круги яркости: после исчерпания оттенков палитра повторяется
// темнее (3 круга = 45 уникальных сочетаний на карту — max 43 квеста).
var questColorV = [...]float64{0.95, 0.70, 0.48}

// questColorFor — цвет i-го квеста карты (порядок стабильный: quests.json
// отсортирован по имени, цвет не «прыгает» между запусками).
func questColorFor(i int) color.RGBA {
	h := questPaletteHues[i%len(questPaletteHues)]
	v := questColorV[(i/len(questPaletteHues))%len(questColorV)]
	r, g, b := hsvToRGB(h, 0.85, v)
	return color.RGBA{r, g, b, 255}
}

// qcolorKey — ключ «карта+квест» для карты цветов.
func qcolorKey(mapKey, id string) string { return mapKey + "\x00" + id }

// QuestColor — цвет квеста на карте mapKey из палитры (серый — квеста нет в
// срезе данных). Если на mapKey у квеста нет точек, берётся цвет с его
// «родной» карты — панель показывает квесты и с других карт.
func (md *MapData) QuestColor(mapKey, id string) color.RGBA {
	md.mu.Lock()
	defer md.mu.Unlock()
	if c, ok := md.qcolors[qcolorKey(mapKey, id)]; ok {
		return c
	}
	if own, ok := md.qindex[id]; ok {
		if c, ok := md.qcolors[qcolorKey(own, id)]; ok {
			return c
		}
	}
	return color.RGBA{170, 170, 170, 255}
}

func hsvToRGB(h, s, v float64) (r, g, b uint8) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var rf, gf, bf float64
	switch {
	case h < 60:
		rf, gf, bf = c, x, 0
	case h < 120:
		rf, gf, bf = x, c, 0
	case h < 180:
		rf, gf, bf = 0, c, x
	case h < 240:
		rf, gf, bf = 0, x, c
	case h < 300:
		rf, gf, bf = x, 0, c
	default:
		rf, gf, bf = c, 0, x
	}
	return uint8((rf + m) * 255), uint8((gf + m) * 255), uint8((bf + m) * 255)
}
