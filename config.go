package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config — пользовательские настройки: %AppData%\TarkyMappy\config.json.
type Config struct {
	ScreenshotsDir string  `json:"screenshotsDir"`
	Zoom           float64 `json:"zoom"`
	Visible        bool    `json:"visible"`
	ShowPmc        bool    `json:"showPmc"` // выходы ЧВК
	ShowScav       bool    `json:"showScav"`
	ShowShared     bool    `json:"showShared"`
	ShowHazards    bool    `json:"showHazards"`  // опасности: мины/снайперы/ловушки
	ManualMap      bool    `json:"manualMap"`    // ручной выбор карты вместо автодетекта
	ManualKey      string  `json:"manualKey"`    // выбранная карта (пусто — не выбрана)
	LastMap        string  `json:"lastMap"`      // последняя карта (старт нового сеанса с неё)
	IgnoreFloors   bool    `json:"ignoreFloors"` // фейлсвитч: плоская combined-карта вместо этажей
	DebugLog       bool    `json:"debugLog"`

	// Quest Helper
	LogsDir       string   `json:"logsDir"`       // папка логов EFT (пусто — ручной режим)
	TrackedQuests []string `json:"trackedQuests"` // отслеживаемые квесты (id, persist)
	QuestFaction  string   `json:"questFaction"`  // фильтр квестов: Any | BEAR | USEC
	QuestShowAll  bool     `json:"questShowAll"`  // панель: все квесты карты (false — только tracked)

	// Farm Helper
	FarmCats   int  `json:"farmCats"`   // битовая маска категорий лута (см. farmCats)
	FarmFloors bool `json:"farmFloors"` // маршрут по всем этажам (иначе только наземный)
	FarmStops  int  `json:"farmStops"`  // максимум точек маршрута (слайдер 5..35)

	// Локализация: ru | en (F9-кнопка, словарь lang.go)
	Lang string `json:"lang"`

	WinX int `json:"winX"` // геометрия окна (0 — не задано)
	WinY int `json:"winY"`
	WinW int `json:"winW"`
	WinH int `json:"winH"`
}

func DefaultConfig() Config {
	return Config{Zoom: 1.0, Visible: true, ShowPmc: true, ShowScav: true, ShowShared: true,
		ShowHazards: true, QuestFaction: "Any", FarmCats: 0b11111, FarmStops: 12, Lang: "ru"} // все категории фарма
}

// ConfigPath — путь конфига (os.UserConfigDir на Windows — %AppData%).
func ConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "TarkyMappy", "config.json"), nil
}

// LoadConfig читает конфиг из стандартного пути.
func LoadConfig() (Config, string) {
	path, _ := ConfigPath()
	return LoadConfigWith(path)
}

// LoadConfigWith читает конфиг по указанному пути; если его нет или он
// битый — возвращает дефолт. Терпит UTF-8 BOM (Notepad, PowerShell 5).
func LoadConfigWith(path string) (cfg Config, realPath string) {
	cfg = DefaultConfig()
	if path == "" {
		return cfg, ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, path
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), path
	}
	setLang(cfg.Lang) // валидация + применение (невалидный → ru)
	cfg.Lang = curLang
	return cfg, path
}

// SaveConfig пишет конфиг, создавая каталог при необходимости.
func SaveConfig(path string, cfg Config) error {
	if path == "" {
		return errors.New("путь конфига не задан")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
