# TarkyMappy — UPDATE.TXT

Подробная инструкция: что делать, когда BSG выпустили патч/вайп/новую карту.
Определи свой сценарий по таблице и иди в нужный раздел.

| Что случилось в игре | Раздел | Пересборка exe |
|---|---|---|
| Изменились цены барахолки (вообще всегда) | [1](#1-цены-барахолки-самое-частое) | **нет** |
| Поменяли выходы/транзиты, квесты, ключи, RU-имена | [2](#2-точки-данных-выходыквестыключифармлокализация) | да |
| Переработали существующую карту (этажи, здания) | [3](#3-переработка-существующих-карт) | да |
| Добавили совсем новую карту | [4](#4-совсем-новая-карта) | да |
| Добавили новые предметы | [5](#5-новые-предметы) | обычно нет |
| — | [6. Финал: тесты, сборка, smoke](#6-финал-любого-сценария-с-пересборкой) | |

Принцип проекта: **данные карт/квестов зашиты в exe** (embed, офлайн-работа),
обновляются только через пересборку из исходников. Исключение — цены F2:
они лежат файлом рядом с exe и обновляются без пересборки.

Требования: Go 1.27+, Python 3.8+ (только stdlib), ImageMagick (команда
`magick` в `PATH` или параметр `-Magick` у `tools/make_maps.ps1`), curl.
Все скрипты — PowerShell 5.1, запуск из корня проекта.

---

## 1. Цены барахолки (самое частое)

Ничего пересобирать не нужно. Папка `prices/` должна лежать рядом с
`TarkyMappy.exe`:

```powershell
cd prices
python update_prices.py
```

- Скрипт качает `avg24hPrice` обоих режимов (PvP `regular`, PvE `pve`),
  считает цену за слот, кладёт топ-50 в `prices/prices.json` **рядом со скриптом**.
- Опции: `--top 100` (размер топа), `--out путь` (куда писать),
  `--source auto|graphql|static` (по умолчанию `auto`).
- Два источника (с 2026-10): GraphQL `api.tarkov.dev/graphql` и
  **статические снапшоты** `json.tarkov.dev/<mode>/items` (фолбэк).
  `auto` = сперва GraphQL; при `HTTP 422 "GraphQL server unavailable"`
  скрипт сам уходит в статику — большие файлы качает HTTP Range кусками.
  Имена предметов берутся из `items_en` по id (в статике имена-заглушки).
- Приложение перечитывает файл при **каждом открытии F2** и при старте —
  достаточно закрыть и снова открыть F2.
- `prices.json` битый/удалён — приложение не падает: F2 покажет подсказку,
  в `tarkymappy.log` будет запись.

---

## 2. Точки данных: выходы/квесты/ключи/фарм/локализация

Затрагивается: `data/exits.json`, `data/quests.json`, `data/locks.json`,
`data/farm.json`, `data/loc_ru.json`.

### Шаг 1. Скачать свежие снапшоты в %TEMP%

Файлы кладутся под фиксированными именами (скрипт ищет именно их):

| Файл в %TEMP% | Источник | Обязателен |
|---|---|---|
| `tmmt_catalog.json` | `json.tarkov.dev/regular/maps` (живой; при обрывах — HTTP Range кусками, см. раздел 2) | да |
| `tmmt_maps_en.json` | `json.tarkov.dev/regular/maps_en` | да |
| `tmmt_tasks.json` | `json.tarkov.dev/regular/tasks` | да |
| `tmmt_tasks_en.json` | `json.tarkov.dev/regular/tasks_en` | да |
| `tmmt_items_full_en.json` | `json.tarkov.dev/regular/items_en` (flat `{"<id> ShortName"/"<id> Name"}`; имена ключей квестов + фарм) | да |
| `tmmt_maps_ru.json` | `json.tarkov.dev/regular/maps_ru` | нет |
| `tmmt_tasks_ru.json` | `json.tarkov.dev/regular/tasks_ru` | нет |
| `tmmt_items_full_ru.json` | `json.tarkov.dev/regular/items_ru` (flat) | нет |

TarkovTracker `items.en.json` больше НЕ используется (репозиторий мёртв
с 2024-02, новые ключи не резолвит).

Пример скачивания (для RU-словарей то же самое с `_ru`):

```powershell
curl.exe -sL "https://json.tarkov.dev/regular/maps_en" -o "$env:TEMP\tmmt_maps_en.json"
```

Если живой эндпоинт `json.tarkov.dev` недоступен — ищи свежий снапшот на
web.archive.org. RU-файлы необязательны: нет файла — RU-подстановки этой
группы не строятся, в игре останется EN.

Политика карт:
- варианты карт `night-factory` / `ground-zero-21` / `the-lab-dark`
  автоматически идут в квесты `factory` / `groundzero` / `lab` (алиасы
  в `update_data.ps1`, `$questAlias`); выходы/локи/фарм — только с
  основных карт;
- квесты карт без арта (`icebreaker`, `the-labyrinth`, `terminal`) —
  WARN со счётчиком точек и отбрасываются; добавить поддержку можно,
  только внеся карту в `data/maps.json` (арт + bounds);
- НЕИЗВЕСТНЫЙ map id или трейдер = ошибка сборки (штатный guard).

### Шаг 2. Пересобрать данные

```powershell
powershell -File tools\update_data.ps1
```

Скрипт перезапишет все пять JSON и выведет `OK: ...` по каждому. Ключевое:
фильтрация карт идёт по `tdevId` из `data/maps.json` — карты без bounds
(arena и пр.) отбрасываются автоматически.

### Шаг 3. Проверить

```powershell
go test ./...
```

Стражи: каждый выход обязан попасть в bbox своей карты; непустые RU-имена
всех 10 карт без mojibake (`TestMapNamesRU`); покрытие глифов шрифта
(`TestFontCoversDictRunes`); паритет ключей RU/EN словарей.

Грабли (из опыта):
- RU/данные-JSON **без BOM**; PS-скрипты — UTF-8 **с BOM**;
- RU-файлы читать только `[IO.File]::ReadAllText`, НЕ `Get-Content -Raw`;
- кракозябры вида «Р—Р°РІРѕРґ» — двойная перекодировка UTF-8→CP1251→UTF-8,
  ловится `TestMapNamesRU`;
- после любой регенерации смотреть `git diff data/` глазами: аномальные
  сдвиги координат = битый снапшот.

---

## 3. Переработка существующих карт

Затрагивается: `maps/<key>__<Floor>.png`, `maps/<key>__ALL.png`,
поле `svg.floors` в `data/maps.json`.

```powershell
powershell -File tools\make_maps.ps1          # по умолчанию — только устаревшее
powershell -File tools\make_maps.ps1 -Force   # перерендерить всё принудительно
powershell -File tools\make_maps.ps1 -Magick 'C:\Program Files\ImageMagick-7.1.2-Q16-HDRI\magick.exe'
```

`-Magick` нужен, если `magick.exe` не добавлен в `PATH`; по умолчанию скрипт
ищет команду `magick`.

**Карты с `svg.customArt: true` (сейчас Lighthouse) скрипт НЕ трогает** —
их арт не из tarkovdata (Lighthouse: растр reemr/re3mr.com v1.7, CC BY-NC-SA 4.0,
`maps/lighthouse__Ground_Level.jpg`), перерендерить его из SVG нельзя.
Если у reemr выйдет новая версия — скачать PNG, обрезать легенду, свести
bounds (калибровка по арту reemr) и заменить jpg руками.

Что делает скрипт:
1. Для каждой карты из `data/maps.json` скачивает SVG с
   `raw.githubusercontent.com/TarkovTracker/tarkovdata/master/maps/<file>`
   в `%TEMP%\tmmt_<file>` (кэш: если файл уже есть и не битый — не качает).
2. Этажи = корневые `<g id="...">` SVG. Новые этажи дописываются В КОНЕЦ
   списка `svg.floors` в `maps.json` (порядок «снизу вверх»), пропавшие
   выкидываются — скрипт сам синхронизирует и печатает `key: floors -> ...`.
3. defaultFloor рендерится 4096px, остальные этажи и `__ALL` — 2048px
   (ImageMagick, density считается из viewBox; путь задаётся параметром
   `-Magick`, по умолчанию используется `magick` из `PATH`).
4. На не-ground этажах ground рисуется призраком (opacity 0.35).

Проверка руками: запустить приложение, включить карту, попереключать этажи,
убедиться что маркеры выходов сидят на своих местах (изменили координаты
выходов — сначала раздел 2). Старый однослойный `maps/<key>.png` скрипт
удаляет сам, если карта стала многоэтажной.

---

## 4. Совсем новая карта

0. **Проверить источник.** Нужны: SVG у TarkovTracker/tarkovdata (папка
   `maps/`) и запись в их `maps.json` (bounds + coordinateRotation). Нет
   там — карта не поддерживается, останавливаемся (иначе UV-математика
   не сойдётся).
1. **Добавить запись в `data/maps.json`** (руками):
   - ключ: латиницей, малыми буквами (`groundzero`, …) — это имя папки
     PNG и ключ во всех data-файлах;
   - `tdevId`: id карты из `tmmt_catalog.json` (для связи с выходами);
   - `bounds [[X0,Z0],[X1,Z1]]` и `coordinateRotation` (90/180/270) —
     из maps.json TarkovTracker, один в один;
   - `svg { file, floors: [], defaultFloor }` — `floors` заполнит
     `make_maps.ps1` (шаг 3);
   - `locale { en, ru }`.
2. **RU-название.** Если в снапшоте locale `ru` нет (прецеденты: Ground
   Zero, Streets) — вписать официальное имя BSG руками: «Приморск»,
   «Улицы Таркова». `TestMapNamesRU` требует непустое `ru` без U+FFFD.
3. **Отрендерить:** `powershell -File tools\make_maps.ps1` — скачает SVG,
   синхронизирует этажи в `maps.json`, отрендерит PNG.
4. **Выходы/квесты:** `update_data.ps1` подхватит новую карту автоматически
   (фильтр по `tdevId`), ничего дописывать не надо.
5. **Тест границ:** `go test ./...` — если bounds кривые, тест упадёт:
   выходы новой карты не попадут в её bbox.
6. **Проверка в игре:** скриншот на спавне → точка игрока на карте, выходы
   совпадают с игрой, поворот взгляда корректен. Сверить пару позиций из
   `tarkymappy.log` (X,Z → map) с реальными спавнами.

---

## 5. Новые предметы

- **Для F2 (цены):** ничего не делать — скрипт тянет весь каталог целиком
  при каждом запуске, новые предметы попадают в топ сами. Пересборка не нужна.
- **Для квестов/ключей (F4):** свежие снапшоты `tasks`/`tasks_en`/
  `items_en`/`items_full_en` + `update_data.ps1` (раздел 2). Новые предметы
  в целях квестов подхватятся из снапшота.
- **RU-имена новых предметов:** докачать `_ru` словари и пересобрать
  `loc_ru.json` тем же `update_data.ps1`. Чего в словарях нет — останется
  EN, это штатное поведение (покрытие ~95%).
- **Фарм-контейнеры:** RU-имена контейнеров в API без локалей — захардкожены
  словарём `$containerRU` в `update_data.ps1`. Новый контейнер фарма =
  дописать пару руками в этот словарь.

---

## 6. Финал (любой сценарий с пересборкой)

```powershell
# интерактивный помощник: тесты, vet, сборка и папка dist
build_ru.bat
# или
build_en.bat

# либо вручную
gofmt -l .            # пустой вывод
go vet ./...
go test ./...
go build -ldflags "-H=windowsgui" -o TarkyMappy.exe .
```

Smoke-проверка перед раскаткой:
1. Запуск: старт без паник, `tarkymappy.log` чистый.
2. Скриншот в игре → карта определяется, точка/выходы на месте.
3. F2 — вкладки PvP/PvE, дата снимка в шапке; F9/F3/F4/FAQ — открываются,
   mutually exclusive, клик-сквозь работает.
4. Смена языка RU/EN, сохранение настроек, повторный запуск.

Важно: папка `prices/` живёт **рядом с exe** и пересборкой из исходников
не затирается — при переносе exe на другую машину копируй `TarkyMappy.exe`
вместе с `prices/` (там же `update_prices.py` для будущих обновлений).

---

## 7. Шпаргалка (copy-paste)

```powershell
# --- цены (еженедельно / после вайпа экономики) ---
python prices\update_prices.py        # при 422 сам уйдёт в статический снапшот

# --- полный апдейт данных после патча BSG ---
# 0) скачать снапшоты в %TEMP% (таблица из раздела 2);
#    большие файлы json.tarkov.dev качать HTTP Range кусками (см. раздел 2)
powershell -File tools\update_data.ps1     # exits/quests/locks/farm/loc_ru
powershell -File tools\make_maps.ps1 -Force # SVG -> PNG + синк этажей
gofmt -l . ; go vet ./... ; go test ./...
go build -ldflags "-H=windowsgui" -o TarkyMappy.exe .
# 1) smoke: запуск, скриншот, F2/F4, лог
```

При проблемах с источниками: tarkov.dev API отдаёт 422 «GraphQL server
unavailable» — их сервер лежит, ретраи не помогут, пробовать позже или
брать снапшот web.archive.org.


