package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestLangKeyParity — словари ru/en должны покрывать один и тот же набор
// ключей: промах — «залипшая» строка другого языка в UI.
func TestLangKeyParity(t *testing.T) {
	for k, ru := range dictRU {
		if en, ok := dictEN[k]; !ok || en == "" {
			t.Errorf("dictEN: net pustogo perevoda dlya %q", k)
		}
		if ru == "" {
			t.Errorf("dictRU[%q]: pustoe znachenie", k)
		}
	}
	for k := range dictEN {
		if _, ok := dictRU[k]; !ok {
			t.Errorf("dictRU: net perevoda dlya %q", k)
		}
	}
}

// TestTFallback — T(): ru-локаль, en-локаль и фолбэк на сам ключ.
func TestTFallback(t *testing.T) {
	defer setLang("ru")

	setLang("ru")
	if got := T("btn.close"); got != "Закрыть" {
		t.Errorf("T(btn.close) ru = %q", got)
	}
	if got := T("farm.built", 5, "Armory"); !strings.Contains(got, "5") || !strings.Contains(got, "Armory") {
		t.Errorf("T(farm.built, 5, Armory) ru = %q", got)
	}
	setLang("en")
	if got := T("btn.close"); got != "Close" {
		t.Errorf("T(btn.close) en = %q", got)
	}
	// Ключа нет ни в одном словаре — ключ и возвращается.
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("T(no.such.key) = %q, want key itself", got)
	}
}

// TestNextLang — цикл кнопки F9 замыкается.
func TestNextLang(t *testing.T) {
	defer setLang("ru")
	setLang("ru")
	if nextLang() != "en" {
		t.Errorf("nextLang(ru) = %q", nextLang())
	}
	setLang("en")
	if nextLang() != "ru" {
		t.Errorf("nextLang(en) = %q", nextLang())
	}
	setLang("xx") // невалид — фолбэк ru
	if curLang != "ru" {
		t.Errorf("setLang(xx): curLang = %q", curLang)
	}
}

// TestTxtWCyrillic — кириллица рисуется (ширина > 0) и шрифт моноширинный.
func TestTxtWCyrillic(t *testing.T) {
	if charW <= 0 {
		t.Fatalf("charW = %d", charW)
	}
	if w := txtW("Таможня"); w <= 0 {
		t.Errorf("txtW(cyrillic) = %d", w)
	}
	if w := txtW("aaa"); w != 3*charW {
		t.Errorf("txtW(aaa) = %d, want %d", w, 3*charW)
	}
	// Моноширинность: любые буквы одной локали — одинаковой ширины.
	if a, b := runeW('a'), runeW('я'); a != b {
		t.Errorf("runeW(a)=%d != runeW(я)=%d", a, b)
	}
}

// TestTruncRunesClipTail — обрезки не ломают руны кириллицы.
func TestTruncRunesClipTail(t *testing.T) {
	s := "Таможня — старая заправка"
	got := truncRunes(s, 8)
	if utf8.RuneCountInString(got) != 8 || !strings.HasSuffix(got, "..") {
		t.Errorf("truncRunes = %q, want 8 рун с '..'", got)
	}
	if truncRunes(s, 100) != s {
		t.Errorf("truncRunes(n>len) изменил строку")
	}
	tail := clipTail(s, 8)
	if !strings.HasPrefix(tail, "...") || utf8.RuneCountInString(tail) != 8 {
		t.Errorf("clipTail = %q, want '...'+хвост в 8 рун", tail)
	}
	if clipTail(s, 100) != s {
		t.Errorf("clipTail(n>len) изменил строку")
	}
}

// TestConfigLangRoundTrip — язык сохраняется в конфиг; невалидный — ru.
func TestConfigLangRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	want := DefaultConfig()
	want.Lang = "en"
	if err := SaveConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadConfigWith(path)
	if got.Lang != "en" || curLang != "en" {
		t.Fatalf("lang = %q (cur %q), want en", got.Lang, curLang)
	}

	// Невалидное значение — сброс в ru и в конфиге, и в текущей локали.
	path2 := filepath.Join(t.TempDir(), "config.json")
	body := []byte(`{"lang":"de"}`)
	if err := os.WriteFile(path2, body, 0o644); err != nil {
		t.Fatal(err)
	}
	got2, _ := LoadConfigWith(path2)
	if got2.Lang != "ru" || curLang != "ru" {
		t.Fatalf("invalid lang: got %q (cur %q), want ru", got2.Lang, curLang)
	}
}

// TestTraderName — имена трейдеров в RU-локали переводятся, в EN — нет.
func TestTraderName(t *testing.T) {
	defer setLang("ru")
	setLang("ru")
	if traderName("Prapor") != "Прапор" {
		t.Errorf("traderName(Prapor) ru = %q", traderName("Prapor"))
	}
	setLang("en")
	if traderName("Prapor") != "Prapor" {
		t.Errorf("traderName(Prapor) en = %q", traderName("Prapor"))
	}
	if traderName("Unknown Trader") != "Unknown Trader" {
		t.Errorf("traderName(unknown) должен остаться как есть")
	}
}

// TestFontCoversDictRunes — каждый не-ASCII символ RU-словаря обязан иметь
// глиф в Go Mono (отсутствующий рисуется «квадратиком» — кракозябры).
func TestFontCoversDictRunes(t *testing.T) {
	seen := map[rune]bool{}
	for _, s := range dictRU {
		for _, r := range s {
			if r >= 128 {
				seen[r] = true
			}
		}
	}
	for r := range seen {
		if _, ok := uiFace.GlyphAdvance(r); !ok {
			t.Errorf("Go Mono: net glifa %q (U+%04X) — budut kvadratiki", r, r)
		}
	}
}
