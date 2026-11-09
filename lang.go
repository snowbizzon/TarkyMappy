package main

import (
	"fmt"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
)

// --- Локализация: UI-строки RU/EN + игровые данные через md.Loc. ---

// langOrder — порядок цикла языка (кнопка в F9). Config.Lang хранит текущий.
var langOrder = []string{"ru", "en"}

// curLang — текущий язык UI (глобально: весь рендер читает его через T/Loc).
var curLang = "ru"

func nextLang() string {
	for i, l := range langOrder {
		if l == curLang {
			return langOrder[(i+1)%len(langOrder)]
		}
	}
	return langOrder[0]
}

func setLang(l string) {
	for _, ok := range langOrder {
		if ok == l {
			curLang = l
			return
		}
	}
	curLang = "ru"
}

// T — перевод ключа в текущем языке с фолбэком на второй язык, затем на
// сам ключ ("%s"-форматирование).
func T(key string, a ...any) string {
	first, second := dictEN, dictRU
	if curLang == "ru" {
		first, second = dictRU, dictEN
	}
	s := key
	if d, ok := first[key]; ok && d != "" {
		s = d
	} else if d, ok := second[key]; ok && d != "" {
		s = d
	}
	if len(a) > 0 {
		return fmt.Sprintf(s, a...)
	}
	return s
}

// traderName — русское имя трейдера (в данных — EN); нет в словаре — EN.
func traderName(name string) string {
	if curLang == "ru" {
		if r, ok := tradersRU[name]; ok {
			return r
		}
	}
	return name
}

// Имена торговцев — по официальной русской локализации/вики (fandom «Торговля»):
// Fence = Скупщик («Барыга» — сленг, в русской версии такого торговца нет),
// Ragman = Барахольщик, Skier = Лыжник, Lightkeeper = Смотритель.
// Новые торговцы 2026 (Taran/Voevoda/...) — по traders_ru tarkov.dev.
var tradersRU = map[string]string{
	"Prapor":        "Прапор",
	"Therapist":     "Терапевт",
	"Skier":         "Лыжник",
	"Peacekeeper":   "Миротворец",
	"Mechanic":      "Механик",
	"Ragman":        "Барахольщик",
	"Jaeger":        "Егерь",
	"Fence":         "Скупщик",
	"Lightkeeper":   "Смотритель",
	"Ref":           "Реф",
	"BTR Driver":    "Водитель БТР",
	"Radio Station": "Радиостанция",
	"Taran":         "Таран",
	"Mr Kerman":     "Мистер Керман",
	"Mr. Kerman":    "Мистер Керман",
	"Voevoda":       "Воевода",
	"Survivor":      "Выживший",
}

var dictRU = map[string]string{
	// Заголовки / общие
	"set.title":  "Настройки",
	"q.title":    "Квест-помощник",
	"farm.title": "Фарм-помощник",
	"faq.title":  "FAQ — справка",
	"faq.data":   "ДАННЫЕ: патч %s, срез %s",
	"faq.lhart":  "- Маяк: арт reemr/re3mr.com v1.7 (CC BY-NC-SA 4.0)",
	"drag.hint":  "- тяни за полосу -",
	"btn.browse": "Обзор...",
	"btn.apply":  "Применить",
	"btn.close":  "Закрыть",
	"btn.gen":    "Сгенерировать",
	"btn.new":    "Новый с этой точки",
	"btn.reset":  "Сброс",
	"btn.scan":   "История",
	"lang.btn":   "Язык: %s",
	"close.app":  "Выход",
	"tag.manual": "(ручн.)",
	// Настройки (F9)
	"set.dir":     "Папка скриншотов EFT:",
	"set.exits":   "Выходы:",
	"exit.pmc":    "ЧВК",
	"exit.haz":    "Опасности",
	"exit.scav":   "Дикий",
	"exit.shared": "Общ.",
	"set.manual":  "Ручной выбор",
	"set.quests":  "Квесты (F4)",
	"set.farm":    "Фарм (F3)",
	"set.prices":  "Цены (F2)",

	// Помощник цен (F2): топ барахолки, ₽/слот.
	"prices.title":   "Барахолка — цена за слот",
	"prices.tab.pvp": "PvP",
	"prices.tab.pve": "PvE",
	"prices.updated": "данные от %s",
	"prices.hint":    "Средняя цена за 24ч; справа — цена за 1 слот инвентаря",
	"prices.empty":   "Нет данных: запусти update_prices.py из папки prices и открой F2 снова",
	"prices.row":     "%2d. %s [%dx%d] %s | %s/слот",
	"faq.keys.f2":    "- F2 цены барахолки (топ за слот, PvP/PvE)",
	"set.pickmap":    "выбери карту:",
	"set.map":        "карта: %s",
	"set.floors":     "Игнор. этажи",
	"set.tip1":       "F3 фарм; F4 квесты; F5/F6 зум; F7 скрыть",
	"set.tip2":       "F8 выход; F9 настройки; PgUp/PgDn этаж; ? — FAQ",
	"set.nodir":      "Папка не найдена",
	"farm.blockq":    "Фарм активен: сброс в F3 (Сброс) или клавиша F4",
	// HUD / карта
	"hud.nopos": "Нажми PrtSc в игре (F9 — настройки)",
	"hud.nomap": "Карта не определена",
	// Фарм-панель (F3)
	"farm.nopos":        "позиция неизвестна — нажми PrtSc в игре",
	"farm.pos":          "позиция: %s, %s, %s",
	"farm.map":          "Карта: %s",
	"farm.mismatch":     "ВНИМАНИЕ: маршрут на карте %s",
	"farm.cat.t":        "Тех для убежища",
	"farm.cat.m":        "Медицина",
	"farm.cat.s":        "На продажу",
	"farm.cat.h":        "Дорогой тех",
	"farm.cat.w":        "Оружейный",
	"farm.floors":       "Этажи: все",
	"farm.exitlbl":      "Выход маршрута:",
	"farm.exitnone":     "Выход: (нет маршрута)",
	"farm.auto":         "авто",
	"farm.pick":         "выбор",
	"farm.exitfmt":      "Выход: %s (%s, %dм)",
	"farm.stops":        "Точки: %d",
	"farm.hud.mismatch": "Фарм: маршрут на карте %s — смотришь %s",
	"farm.hud.done":     "ФАРМ: все точки пройдены -> выход %s ~%dм",
	"farm.hud":          "ФАРМ %d/%d -> %s ~%dм (прямо) | выход %s ~%dм",
	// Статусы фарма
	"farm.needpos":    "Нажми PrtSc в игре — позиция неизвестна",
	"farm.nodata":     "Нет данных фарма для этой карты",
	"farm.nocat":      "Ни одна категория не выбрана",
	"farm.allvisited": "Рядом всё пройдено — смени категорию или Этажи",
	"farm.nothing":    "Ничего не найдено",
	"farm.noexits":    "Доступных выходов нет (смотри галочки фракций)",
	"farm.noexits2":   "Доступных выходов нет",
	"farm.built":      "Маршрут: точек %d -> выход %s",
	"farm.noroute":    "Сначала нажми Сгенерировать",
	"farm.qon":        "фарм сброшен — квесты включены",
	// Блок маршрута (F3)
	"farm.hint.none":    "Маршрута нет: выбери категории и нажми Сгенерировать.",
	"farm.hint.how":     "Маршрут строится от текущей позиции до самого далёкого доступного выхода.",
	"farm.hint.visited": "Точки рядом с тобой (~30 м) тускнеют как пройденные.",
	"farm.route":        "МАРШРУТ: точек %d -> выход %s (%s)",
	"farm.floornote":    "[Этажи: все] смена этажа ~ +120 м пути",
	"farm.keys":         "   нужен ключ: %s",
	"farm.exitline":     "-> Выход: %s (%s)",
	"farm.sector":       "сектор %s",
	"farm.keyanch":      "Ключ: %s",
	// Квест-панель (F4)
	"q.mine":     "Мои",
	"q.all":      "Все",
	"q.map":      "Карта: %s (%d квестов)",
	"q.filter":   "Фильтр:",
	"q.logs":     "Папка логов EFT:",
	"q.nologs":   "<не выбрана — ручной режим>",
	"q.tip":      "i — подсказки; чекбокс — трек",
	"q.none":     "Нет квестов на карте / фильтре",
	"q.auto":     " [АВТО]",
	"q.note":     "%s lvl%d, точек: %d",
	"q.missing":  "квеста нет в срезе данных (tarkov.dev)",
	"q.goals":    "ЦЕЛИ НА КАРТЕ:",
	"q.nopoints": "- точек на этой карте нет",
	"q.keys":     "КЛЮЧИ:",
	"q.door":     " — дверь на карте",
	"q.detail":   "ПОДРОБНО:",
	// FAQ
	"faq.exits":        "ВЫХОДЫ НА КАРТЕ:",
	"faq.exit.pmc":     "- синий — выход ЧВК (PMC)",
	"faq.exit.scav":    "- зелёный — выход Дикого (scav)",
	"faq.exit.shared":  "- жёлтый — общий выход ЧВК и Дикого (напр. «Перекрёсток»)",
	"faq.exit.transit": "- фиолетовый T| — транзит на другую карту",
	"faq.exit.floor":   "- ^ / v перед названием — выход на другом этаже",
	"faq.hazards":      "- красные зоны и точки — МИНЫ, оранжевые — снайперы, зол. + — ловушки",
	"faq.farm":         "ФАРМ-ПОМОЩНИК (F3) — золотой цвет:",
	"faq.farm.pts":     "- нумерованные точки — кластеры лута",
	"faq.farm.visited": "- тусклый пунктир — уже пройдено (~30м)",
	"faq.farm.ring":    "- кольцо — следующая цель / выход маршрута",
	"faq.farm.lines":   "- линии — по ПРЯМОЙ (направление, не дорога)",
	"faq.farm.arrow":   "- стрелка у края окна — цель/выход за экраном",
	"faq.farm.hud":     "- HUD сверху: ФАРМ n/N -> цель ~м (прямо) | выход ~м",
	"faq.panel":        "ПАНЕЛЬ ФАРМА (F3):",
	"faq.panel.cats":   "- категории — что искать; Этажи: все — по всем этажам (+120м)",
	"faq.panel.stops":  "- Точки — сколько точек в маршруте (5..35)",
	"faq.panel.exit":   "- Выход: авто (самый далёкий) или по клику",
	"faq.panel.gen":    "- Сгенерировать — маршрут от твоей позиции",
	"faq.panel.new":    "- Новый с этой точки — заново без пройденных",
	"faq.panel.reset":  "- Сброс или F4 — выключить фарм (квесты снова доступны)",
	"faq.panel.prtsc":  "- нужен PrtSc в игре — иначе позиция неизвестна",
	"faq.qpanel":       "ПАНЕЛЬ КВЕСТОВ (F4):",
	"faq.qpanel.track": "- галочка — включить трекинг; клик по строке — подсказки",
	"faq.qpanel.mine":  "- Мои / Все — только трекинг или все квесты карты",
	"faq.qpanel.auto":  "- Авто — квест взят из логов игры",
	"faq.keys":         "КЛАВИШИ И КНОПКИ:",
	"faq.keys.f3":      "- F3 фарм; F4 квесты; F5/F6 зум; F7 скрыть оверлей",
	"faq.keys.f8":      "- F8 выход; F9 настройки; F10 след. карта; PgUp/PgDn этаж",
	"faq.keys.gear":    "- шестерёнка (справа) — настройки; красная X — закрыть приложение",
	"faq.keys.grip":    "- уголок в углу — изменить размер; верхняя полоса — перенести окно",
}

var dictEN = map[string]string{
	// Заголовки / общие
	"set.title":  "Settings",
	"q.title":    "Quest Helper",
	"farm.title": "Farm Helper",
	"faq.title":  "FAQ — help",
	"faq.data":   "DATA: patch %s, snapshot %s",
	"faq.lhart":  "- Lighthouse art: reemr/re3mr.com v1.7 (CC BY-NC-SA 4.0)",
	"drag.hint":  "- drag by the bar -",
	"btn.browse": "Browse...",
	"btn.apply":  "Apply",
	"btn.close":  "Close",
	"btn.gen":    "Generate",
	"btn.new":    "New from here",
	"btn.reset":  "Reset",
	"btn.scan":   "History",
	"lang.btn":   "Lang: %s",
	"close.app":  "Close App",
	"tag.manual": "(manual)",
	// Настройки (F9)
	"set.dir":     "EFT screenshots folder:",
	"set.exits":   "Exits:",
	"exit.pmc":    "PMC",
	"exit.haz":    "Hazards",
	"exit.scav":   "Scav",
	"exit.shared": "Shrd",
	"set.manual":  "Manual pick",
	"set.quests":  "Quests (F4)",
	"set.farm":    "Farm (F3)",
	"set.prices":  "Prices (F2)",

	// Price Helper (F2): flea market top, ₽/slot.
	"prices.title":   "Flea market — price per slot",
	"prices.tab.pvp": "PvP",
	"prices.tab.pve": "PvE",
	"prices.updated": "data from %s",
	"prices.hint":    "24h average; right — price per 1 inventory slot",
	"prices.empty":   "No data: run update_prices.py from the prices folder, then reopen F2",
	"prices.row":     "%2d. %s [%dx%d] %s | %s/slot",
	"faq.keys.f2":    "- F2 flea market prices (top ₽/slot, PvP/PvE)",
	"set.pickmap":    "pick a map:",
	"set.map":        "map: %s",
	"set.floors":     "Ignore floors",
	"set.tip1":       "F3 farm; F4 quests; F5/F6 zoom; F7 hide",
	"set.tip2":       "F8 quit; F9 settings; PgUp/PgDn floor; ? — FAQ",
	"set.nodir":      "Folder not found",
	"farm.blockq":    "Farm active: reset in F3 (Reset) or F4 key",
	// HUD / карта
	"hud.nopos": "Press PrtSc in game (F9 — settings)",
	"hud.nomap": "Map not detected",
	// Фарм-панель (F3)
	"farm.nopos":        "position unknown — Press PrtSc in game",
	"farm.pos":          "position: %s, %s, %s",
	"farm.map":          "Map: %s",
	"farm.mismatch":     "WARNING: route is for map %s",
	"farm.cat.t":        "Hideout tech",
	"farm.cat.m":        "Medical",
	"farm.cat.s":        "For sale",
	"farm.cat.h":        "Expensive tech",
	"farm.cat.w":        "Weapon",
	"farm.floors":       "Floors: all",
	"farm.exitlbl":      "Route exit:",
	"farm.exitnone":     "Exit: (no route)",
	"farm.auto":         "auto",
	"farm.pick":         "picked",
	"farm.exitfmt":      "Exit: %s (%s, %dm)",
	"farm.stops":        "Stops: %d",
	"farm.hud.mismatch": "Farm: route is for map %s — you see %s",
	"farm.hud.done":     "FARM: all stops visited -> exit %s ~%dm",
	"farm.hud":          "FARM %d/%d -> %s ~%dm (straight) | exit %s ~%dm",
	// Статусы фарма
	"farm.needpos":    "Press PrtSc in game — position unknown",
	"farm.nodata":     "No farm data for this map",
	"farm.nocat":      "No category selected",
	"farm.allvisited": "Everything visited nearby — change category or Floors",
	"farm.nothing":    "Nothing found",
	"farm.noexits":    "No available exits (check faction checkboxes)",
	"farm.noexits2":   "No available exits",
	"farm.built":      "Route: %d stops -> exit %s",
	"farm.noroute":    "Press Generate first",
	"farm.qon":        "farm reset — quests enabled",
	// Блок маршрута (F3)
	"farm.hint.none":    "No route: pick categories and press Generate.",
	"farm.hint.how":     "The route is built from your position to the farthest available exit.",
	"farm.hint.visited": "Stops near you (~30 m) dim as visited.",
	"farm.route":        "ROUTE: %d stops -> exit %s (%s)",
	"farm.floornote":    "[Floors: all] floor change ~ +120 m of path",
	"farm.keys":         "   key needed: %s",
	"farm.exitline":     "-> Exit: %s (%s)",
	"farm.sector":       "sector %s",
	"farm.keyanch":      "Key: %s",
	// Квест-панель (F4)
	"q.mine":     "Mine",
	"q.all":      "All",
	"q.map":      "Map: %s (%d quests)",
	"q.filter":   "Filter:",
	"q.logs":     "EFT logs folder:",
	"q.nologs":   "<not picked — manual mode>",
	"q.tip":      "i — hints; checkbox — track",
	"q.none":     "No quests on map / filter",
	"q.auto":     " [AUTO]",
	"q.note":     "%s lvl%d, %d points",
	"q.missing":  "quest not in the tarkov.dev snapshot",
	"q.goals":    "GOALS ON MAP:",
	"q.nopoints": "- no points on this map",
	"q.keys":     "KEYS:",
	"q.door":     " — door on map",
	"q.detail":   "DETAILS:",
	// FAQ
	"faq.exits":        "MAP EXITS:",
	"faq.exit.pmc":     "- blue — PMC exit",
	"faq.exit.scav":    "- green — Scav exit",
	"faq.exit.shared":  "- yellow — shared exit",
	"faq.exit.transit": "- purple T| — transit to another map",
	"faq.exit.floor":   "- ^ / v before the name — exit on another floor",
	"faq.hazards":      "- red zones/dots — MINES, orange — snipers, gold + — traps",
	"faq.farm":         "FARM HELPER (F3) — gold color:",
	"faq.farm.pts":     "- numbered points — loot clusters",
	"faq.farm.visited": "- dim dashes — already visited (~30m)",
	"faq.farm.ring":    "- ring — next target / route exit",
	"faq.farm.lines":   "- lines — STRAIGHT direction, not a road",
	"faq.farm.arrow":   "- arrow at window edge — target/exit off-screen",
	"faq.farm.hud":     "- HUD on top: FARM n/N -> target ~m | exit ~m",
	"faq.panel":        "FARM PANEL (F3):",
	"faq.panel.cats":   "- categories — what to look for; Floors: all — across floors (+120m)",
	"faq.panel.stops":  "- Stops — how many points in the route (5..35)",
	"faq.panel.exit":   "- Exit: auto (farthest) or by click",
	"faq.panel.gen":    "- Generate — route from your position",
	"faq.panel.new":    "- New from here — redo without visited ones",
	"faq.panel.reset":  "- Reset or F4 — turn farm off (quests available again)",
	"faq.panel.prtsc":  "- PrtSc in game is required — otherwise position unknown",
	"faq.qpanel":       "QUEST PANEL (F4):",
	"faq.qpanel.track": "- checkbox — enable tracking; click a row — hints",
	"faq.qpanel.mine":  "- Mine / All — tracked only or all map quests",
	"faq.qpanel.auto":  "- Auto — quest taken from game logs",
	"faq.keys":         "KEYS AND BUTTONS:",
	"faq.keys.f3":      "- F3 farm; F4 quests; F5/F6 zoom; F7 hide overlay",
	"faq.keys.f8":      "- F8 quit; F9 settings; F10 next map; PgUp/PgDn floor",
	"faq.keys.gear":    "- gear (right) — settings; red X — close the app",
	"faq.keys.grip":    "- corner grip — resize; top bar — move window",
}

// --- Шрифт: Go Mono с кириллицей (basicfont 7x13 — только латиница). ---

var (
	uiFace   font.Face
	faceOnce sync.Once
	charW    int // ширина базового глифа (моно) — замена хардкода «7»
	glyphW   = map[rune]int{}
)

// face — ленивая инициализация встроенного Go Mono (новых зависимостей нет:
// golang.org/x/image/font/gofont уже в go.sum через x/image). При ошибке —
// basicfont (кириллица не отрисуется, но приложение живёт). Инициализация
// на старте пакета: charW используется в делениях и обязан быть валидным
// до первого text.Draw.
func init() { face() }

func face() font.Face {
	faceOnce.Do(func() {
		uiFace, charW = basicfont.Face7x13, 7
		f, err := opentype.Parse(gomono.TTF)
		if err != nil {
			return
		}
		nf, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 13, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			return
		}
		uiFace = nf
		if w, ok := nf.GlyphAdvance('M'); ok && w.Ceil() > 0 {
			charW = w.Ceil()
		}
	})
	return uiFace
}

// runeW — ширина глифа в px (кэш по руне).
func runeW(r rune) int {
	face()
	if w, ok := glyphW[r]; ok {
		return w
	}
	w := charW
	if adv, ok := uiFace.GlyphAdvance(r); ok && adv.Ceil() > 0 {
		w = adv.Ceil()
	}
	glyphW[r] = w
	return w
}

// txtW — ширина строки в px; руно-безопасно (байтовый len врали на кириллице).
func txtW(s string) int {
	n := 0
	for _, r := range s {
		n += runeW(r)
	}
	return n
}

// truncRunes — обрезка до n РУН с ".." (байтовые срезы рвали кириллицу).
func truncRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	if n <= 2 {
		return string(r[:n])
	}
	return string(r[:n-2]) + ".."
}

// clipTail — "..."+хвост: последние maxChars рун (статусы, длинные пути).
func clipTail(s string, maxChars int) string {
	r := []rune(s)
	if len(r) <= maxChars {
		return s
	}
	if maxChars < 4 {
		maxChars = 4
	}
	return "..." + string(r[len(r)-maxChars+3:])
}
