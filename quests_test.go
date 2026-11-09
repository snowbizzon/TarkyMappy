package main

import (
	"image/color"
	"testing"
)

// Все точки квестов должны лежать в границах своей карты (допуск 5%, как у выходов),
// а высота Y — попадать в диапазон одного из этажей (visit-зоны висят на этаже).
func TestQuestPointsInsideBoundsAndFloors(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, key := range md.names {
		cfg := md.maps[key]
		b := cfg.SVG.Bounds
		for _, q := range md.QuestsFor(key) {
			if len(q.Points) == 0 {
				t.Errorf("%s: квест %q без точек", key, q.Name)
				continue
			}
			for i, p := range q.Points {
				u, v := WorldToUV(b, p.X, p.Z)
				if u < -0.05 || u > 1.05 || v < -0.05 || v > 1.05 {
					t.Errorf("%s: %q точка %d (%s) uv=(%.3f, %.3f) вне границ", key, q.Name, i, p.Label, u, v)
				}
				if len(cfg.SVG.Floors) > 1 && !questYOnAnyFloor(cfg, p.Y) {
					t.Errorf("%s: %q точка %d y=%v вне всех диапазонов этажей", key, q.Name, i, p.Y)
				}
				total++
			}
		}
	}
	if total < 500 {
		t.Errorf("слишком мало точек квестов: %d", total)
	}
}

func questYOnAnyFloor(cfg MapCfg, y float64) bool {
	for _, f := range cfg.SVG.Floors {
		if b, ok := cfg.SVG.FloorBounds[f]; ok && y >= b[0] && y < b[1] {
			return true
		}
	}
	return false
}

// Санитарные поля квестов: id/имя/трейдер/фракция заполнены, фракция из набора.
func TestQuestFieldsSanity(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	factions := map[string]bool{"Any": true, "BEAR": true, "USEC": true}
	seen := map[string]bool{}
	for _, key := range md.names {
		for _, q := range md.QuestsFor(key) {
			if len(q.ID) != 24 || q.Name == "" || q.Trader == "" {
				t.Errorf("%s: квест %q с пустыми полями (id=%q, trader=%q)", key, q.Name, q.ID, q.Trader)
			}
			if !factions[q.Faction] {
				t.Errorf("%s: %q неизвестная фракция %q", key, q.Name, q.Faction)
			}
			for _, p := range q.Points {
				if p.Kind != "visit" && p.Kind != "item" {
					t.Errorf("%s: %q неизвестный kind %q", key, q.Name, p.Kind)
				}
				if p.Kind == "visit" && len(p.Outline) < 3 {
					t.Errorf("%s: %q visit-зона без контура", key, q.Name)
				}
			}
			seen[q.ID] = true
		}
	}
	if len(seen) < 200 {
		t.Errorf("слишком мало уникальных квестов: %d", len(seen))
	}
}

// Квест из снапшота резолвится по карте; questId из PvE-лога юзера
// (61604635c725987e815b1a46) в срезе tarkov.dev ОТСУТСТВУЕТ — questwatch
// обязан переживать такие id без паники (fallback «неизвестный квест»).
func TestQuestLookup(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, q := range md.QuestsFor("groundzero") {
		if q.ID == "657315ddab5a49b71f098853" { // First in Line
			found = true
			if q.Trader != "Therapist" {
				t.Errorf("First in Line trader = %q, want Therapist", q.Trader)
			}
		}
	}
	if !found {
		t.Error("First in Line не найден на groundzero")
	}
	for _, key := range md.names {
		for _, q := range md.QuestsFor(key) {
			if q.ID == "61604635c725987e815b1a46" {
				t.Errorf("%s: неожиданно найден PvE-квест %q", key, q.Name)
			}
		}
	}
}

// Цвет квеста: детерминирован; отсутствующий в данных id — серый fallback.
func TestQuestColor(t *testing.T) {
	md, err := NewMapData()
	if err != nil {
		t.Fatal(err)
	}
	c1 := md.QuestColor("groundzero", "657315ddab5a49b71f098853")
	c2 := md.QuestColor("groundzero", "657315ddab5a49b71f098853")
	if c1 != c2 {
		t.Fatal("QuestColor недетерминирован")
	}
	if c1 == (color.RGBA{170, 170, 170, 255}) {
		t.Fatal("известный квест получил серый fallback")
	}
	if md.QuestColor("customs", "net-takogo-kvesta") != (color.RGBA{170, 170, 170, 255}) {
		t.Fatal("неизвестный квест должен давать серый fallback")
	}
}
