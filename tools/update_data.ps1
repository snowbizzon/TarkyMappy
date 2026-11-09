# Dev-утилита: собирает data/exits.json, data/quests.json, data/locks.json,
# data/farm.json и data/loc_ru.json из локальных снимков tarkov.dev.
# Использование:
#   1) скачать каталог в %TEMP%\tmmt_catalog.json     (json.tarkov.dev/regular/maps;
#      живой эндпоинт рвёт соединение на больших файлах — качать HTTP Range
#      кусками по ~1-2МБ (см. UPDATING.md, раздел 2) или брать снапшот web.archive.org)
#   2) скачать словарь в %TEMP%\tmmt_maps_en.json     (json.tarkov.dev/regular/maps_en)
#   3) скачать таски  в %TEMP%\tmmt_tasks.json        (json.tarkov.dev/regular/tasks)
#   4) скачать словарь в %TEMP%\tmmt_tasks_en.json    (json.tarkov.dev/regular/tasks_en)
#   5) скачать словарь в %TEMP%\tmmt_items_full_en.json (json.tarkov.dev/regular/items_en,
#      flat {"<id> Name"/"<id> ShortName": "текст"} — имена ключей квестов + фарм;
#      парсится regex'ом, ConvertFrom-Json PS 5.1 давится дублями ключей)
#   6) powershell -File tools\update_data.ps1
#   --- RU-локализация игровых данных -> data/loc_ru.json ---
#   7) скачать словарь в %TEMP%\tmmt_maps_ru.json   (json.tarkov.dev/regular/maps_ru)
#   8) скачать словарь в %TEMP%\tmmt_tasks_ru.json  (json.tarkov.dev/regular/tasks_ru)
#   9) скачать flat  в %TEMP%\tmmt_items_full_ru.json (json.tarkov.dev/regular/items_ru,
#      {"<id> Name"/"<id> ShortName": "текст"})
# Файлы 7-9 необязательны: нет файла — RU-подстановки для этой группы
# не строятся (нужные строки в игре останутся EN).
#
# СРЕЗ 2026-10-25 (патч 1.2.5.0.48146):
# - варианты карт (night-factory / ground-zero-21 / the-lab-dark) мапятся
#   алиасами на основную карту ТОЛЬКО в квестах; выходы/локи/фарм берутся
#   только с основных карт, иначе вариант перезапишет основные данные;
# - квесты Icebreaker/The Labyrinth/Terminal осознанно отбрасываются
#   (нет арта/bounds в открытых источниках);
# - предметы: словарь items_en/items_ru tarkov.dev (TarkovTracker
#   items.en.json мёртв с 2024-02 — НЕ использовать);
# - requiredKeys в objectives тоже даёт ключи квеста;
# - выходы чистятся: дедуп + отсев координат вне bounds+10%.
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

$cat = Get-Content "$env:TEMP\tmmt_catalog.json" -Raw | ConvertFrom-Json
$en  = (Get-Content "$env:TEMP\tmmt_maps_en.json" -Raw | ConvertFrom-Json).data
$bounds = Get-Content (Join-Path $root 'data\maps.json') -Raw | ConvertFrom-Json

# tdevId -> ключ карты в maps.json (customs, factory, ...)
$byId = @{}
foreach ($p in $bounds.PSObject.Properties) { $byId[$p.Value.tdevId] = $p.Name }

# Алиасы вариантов карт для КВЕСТОВ: тот же рендер/границы, отдельный каталог
# tarkov.dev (night-factory, ground-zero-21 (21+), the-lab-dark). Выходы/локи/
# фарм основных карт алиас НЕ должен трогать — иначе вариант их перезапишет.
$questAlias = @{
    '59fc81d786f774390775787e' = 'factory'    # night-factory
    '65b8d6f5cdde2479cb2a3125' = 'groundzero' # ground-zero-21
    '6a294a5b5eb5f9a1700417b7' = 'lab'        # the-lab-dark
}

# Карты без поддержки (нет арта/bounds в открытых источниках): квесты
# отбрасываем осознанно, но С ЧЁТНЫМ СЧЁТЧИКОМ, а не молча.
$knownUnmapped = @{
    '69af492a4819ea4ba10a69c5' = 'icebreaker'
    '6733700029c367a3d40b02af' = 'the-labyrinth'
    '65cc8f81a9aac3e77d0cfd3e' = 'terminal'
    '68236e8153654e8c1200798a' = 'ground-zero-tutorial'
}

function Resolve-QuestMap([string]$id) {
    $k = $byId[$id]
    if ($k) { return $k }
    return $questAlias[$id]
}

# Test-InBounds: точка внутри bbox карты + margin (доля стороны). Определён
# здесь, наверху: им пользуются и выходы, и фарм (движок PS — сверху вниз).
function Test-InBounds([double[][]]$b, [double]$x, [double]$z, [double]$margin) {
    $minx = [math]::Min($b[0][0], $b[1][0]); $maxx = [math]::Max($b[0][0], $b[1][0])
    $minz = [math]::Min($b[0][1], $b[1][1]); $maxz = [math]::Max($b[0][1], $b[1][1])
    $dx = ($maxx - $minx) * $margin; $dz = ($maxz - $minz) * $margin
    return ($x -ge ($minx - $dx)) -and ($x -le ($maxx + $dx)) -and ($z -ge ($minz - $dz)) -and ($z -le ($maxz + $dz))
}

function Resolve-Name([string]$key) {
    $prop = $en.PSObject.Properties[$key]
    if ($prop) { return $prop.Value }
    return $key
}

$out = [ordered]@{}
$nExtracts = 0
$nTransits = 0
$nDroppedExits = 0
foreach ($p in $cat.data.maps.PSObject.Properties) {
    $m = $p.Value
    $key = $byId[$m.id]
    if (-not $key) { continue } # карта без bounds (arena/terminal/...) не нужна
    $fb = $bounds.PSObject.Properties[$key].Value.svg.bounds

    # Дедуп по имя|фракция|позиция — tarkov.dev бывает отдаёт один выход дважды.
    $ex = [ordered]@{}
    foreach ($e in $m.extracts) {
        if (-not $e.position) { continue }
        # Мусорные координаты чужих карт (бывает в tarkov.dev) — вне bounds+10%
        # не рисуем вовсе (жёсткие выбросы вроде UN Roadblock на Ground Zero).
        if (-not (Test-InBounds $fb $e.position.x $e.position.z 0.10)) { $nDroppedExits++; continue }
        $sig = '{0}|{1}|{2}|{3}' -f $e.faction, $e.name, [math]::Round($e.position.x, 1), [math]::Round($e.position.z, 1)
        if ($ex.Contains($sig)) { continue }
        $ex[$sig] = [ordered]@{
            name    = (Resolve-Name $e.name)
            faction = $e.faction
            x       = [math]::Round($e.position.x, 2)
            y       = [math]::Round($e.position.y, 2)
            z       = [math]::Round($e.position.z, 2)
        }
        $nExtracts++
    }
    $tr = [ordered]@{}
    foreach ($t in $m.transits) {
        if (-not $t.position) { continue }
        if (-not (Test-InBounds $fb $t.position.x $t.position.z 0.10)) { $nDroppedExits++; continue }
        $sig = '{0}|{1}|{2}' -f $t.description, [math]::Round($t.position.x, 1), [math]::Round($t.position.z, 1)
        if ($tr.Contains($sig)) { continue }
        $tr[$sig] = [ordered]@{
            name = (Resolve-Name $t.description)
            x    = [math]::Round($t.position.x, 2)
            y    = [math]::Round($t.position.y, 2)
            z    = [math]::Round($t.position.z, 2)
        }
        $nTransits++
    }
    $out[$key] = [ordered]@{ extracts = @($ex.Values); transits = @($tr.Values) }
}

$json = ConvertTo-Json -InputObject $out -Depth 6
[System.IO.File]::WriteAllText((Join-Path $root 'data\exits.json'), $json, (New-Object System.Text.UTF8Encoding($false)))
"OK: data/exits.json ($nExtracts vyhodov, $nTransits tranzitov, otnuto $nDroppedExits)"

# ---------------------------------------------------------------------------
# data/quests.json — квесты с точками на картах (visit-зоны + предметы).
# Источники: %TEMP%\tmmt_tasks.json (id/координаты) + tmmt_tasks_en.json (имена).
# ---------------------------------------------------------------------------
$ten   = (Get-Content "$env:TEMP\tmmt_tasks_en.json" -Raw | ConvertFrom-Json).data
$tasks = (Get-Content "$env:TEMP\tmmt_tasks.json" -Raw | ConvertFrom-Json).data.tasks

# Словарь предметов (ключи квестов + лут фарма): json.tarkov.dev/regular/items_en,
# flat {"<id> Name"/"<id> ShortName": "текст"}. TarkovTracker items.en.json
# мёртв с 2024-02 (новые ключи не резолвит) — НЕ использовать.
$itemsShort = @{}
$itemsName = @{}
if (Test-Path "$env:TEMP\tmmt_items_full_en.json") {
    $rawItems = Get-Content "$env:TEMP\tmmt_items_full_en.json" -Raw
    foreach ($m in [regex]::Matches($rawItems, '"([0-9a-f]{24}) Name"\s*:\s*"([^"]+)"')) {
        $itemsName[$m.Groups[1].Value] = $m.Groups[2].Value
    }
    foreach ($m in [regex]::Matches($rawItems, '"([0-9a-f]{24}) ShortName"\s*:\s*"([^"]+)"')) {
        $itemsShort[$m.Groups[1].Value] = $m.Groups[2].Value
    }
}
function Resolve-ItemName([string]$id) {
    # краткое имя приоритетно (маркеры/подсказки компактнее)
    if ($itemsShort.ContainsKey($id)) { return $itemsShort[$id] }
    if ($itemsName.ContainsKey($id)) { return $itemsName[$id] }
    return $id
}

# id трейдера -> имя (локали tasks_en трейдеров не содержат; имена из снапшота
# json.tarkov.dev/regular/traders — normalizedName, Title Case)
$traders = @{
    '54cb50c76803fa8b248b4571' = 'Prapor'
    '54cb57776803fa99248b456e' = 'Therapist'
    '58330581ace78e27b8b10cee' = 'Skier'
    '5935c25fb3acc3127c3d8cd9' = 'Peacekeeper'
    '5a7c2eca46aef81a7ca2145d' = 'Mechanic'
    '5ac3b934156ae10c4430e83c' = 'Ragman'
    '5c0647fdd443bc2504c2d371' = 'Jaeger'
    '579dc571d53a0658a154fbec' = 'Fence'
    '638f541a29ffd1183d187f57' = 'Lightkeeper'
    '656f0f98d80a697f855d34b1' = 'BTR Driver'
    '6617beeaa9cfa777ca915b7c' = 'Ref'
    '68fe15910f29ba3fdbba9d54' = 'Taran'
    '68fe15990f29ba3fdbba9d55' = 'Radio Station'
    '688246518448b05efd61d461' = 'Mr Kerman'
    '688246958448b05efd61d462' = 'Voevoda'
    '69e0d6cc77b63940375b9173' = 'Survivor'
}

function Resolve-Task([string]$key) {
    $prop = $ten.PSObject.Properties[$key]
    if ($prop -and $prop.Value) { return [string]$prop.Value }
    return $key
}

function Add-QuestPoint($buckets, [string]$mapKey, $point) {
    if (-not $buckets.ContainsKey($mapKey)) {
        $buckets[$mapKey] = New-Object System.Collections.Generic.List[object]
    }
    $buckets[$mapKey].Add($point)
}

$questsOut = [ordered]@{}
$nPoints = 0
$droppedMaps = @{}
$unknownTraders = @{}
foreach ($p in $tasks.PSObject.Properties) {
    $q = $p.Value
    if (-not $traders.ContainsKey([string]$q.trader)) {
        # новый трейдер — не молча «Trader <id>», а валім сборку
        $unknownTraders[[string]$q.trader] = $q.id
    }
    $buckets = @{}
    foreach ($o in $q.objectives) {
        $label = Resolve-Task $o.description
        # visit/mark/place/... — зоны с контуром
        if ($o.zones -and $o.zones.Count -gt 0) {
            $seen = @{}
            foreach ($z in $o.zones) {
                $mk = Resolve-QuestMap $z.map
                if (-not $mk) { $droppedMaps[[string]$z.map] = 1 + [int]$droppedMaps[[string]$z.map]; continue }
                # day/night-дубли и варианты карт (groundzero vs ground-zero-21)
                # имеют одинаковые координаты — дедуп по карте+позиции
                $sig = '{0}|{1}|{2}' -f $mk, [math]::Round($z.position.x, 1), [math]::Round($z.position.z, 1)
                if ($seen[$sig]) { continue }
                $seen[$sig] = 1
                $ol = New-Object System.Collections.Generic.List[object]
                foreach ($pt in $z.outline) {
                    $ol.Add(@([math]::Round($pt.x, 2), [math]::Round($pt.z, 2)))
                }
                Add-QuestPoint $buckets $mk ([ordered]@{
                    kind    = 'visit'
                    label   = $label
                    x       = [math]::Round($z.position.x, 2)
                    y       = [math]::Round($z.position.y, 2)
                    z       = [math]::Round($z.position.z, 2)
                    outline = $ol
                })
                $nPoints++
            }
        }
        # findQuestItem/findItem — возможные позиции предметов
        if ($o.possibleLocations -and $o.possibleLocations.Count -gt 0) {
            foreach ($loc in $o.possibleLocations) {
                $mk = Resolve-QuestMap $loc.map
                if (-not $mk) { $droppedMaps[[string]$loc.map] = 1 + [int]$droppedMaps[[string]$loc.map]; continue }
                foreach ($pp in $loc.positions) {
                    Add-QuestPoint $buckets $mk ([ordered]@{
                        kind  = 'item'
                        label = $label
                        x     = [math]::Round($pp.x, 2)
                        y     = [math]::Round($pp.y, 2)
                        z     = [math]::Round($pp.z, 2)
                    })
                    $nPoints++
                }
            }
        }
    }
    if ($buckets.Count -eq 0) { continue }

    # Ключи квеста: neededKeys {map, keys[]} + objectives[].requiredKeys
    # (вложенные массивы, появились в снапшотах 2026) -> названия (дедуп).
    $keyNames = @()
    foreach ($nk in $q.neededKeys) {
        foreach ($kid in $nk.keys) {
            $nm = Resolve-ItemName $kid
            if ($nm -and ($keyNames -notcontains $nm)) { $keyNames += $nm }
        }
    }
    foreach ($o in $q.objectives) {
        foreach ($rk in @($o.requiredKeys)) {
            if (-not $rk) { continue }
            foreach ($kid in @($rk)) {
                $nm = Resolve-ItemName $kid
                if ($nm -and ($keyNames -notcontains $nm)) { $keyNames += $nm }
            }
        }
    }
    # Полные описания целей (блок «Podrobno» в подсказках).
    $objTexts = @()
    foreach ($o in $q.objectives) {
        $t = (Resolve-Task $o.description).Trim()
        if (-not $t) { continue }
        if ($o.optional) { $t = '(optional) ' + $t }
        if ($objTexts -notcontains $t) { $objTexts += $t }
    }

    $wiki = [string]$q.wikiLink
    $wiki = $wiki -replace 'https://web\.archive\.org/web/\d+/', ''
    if (-not ($wiki -like 'http*')) { $wiki = '' }
    $traderName = $traders[$q.trader]
    if (-not $traderName) { $traderName = 'Trader ' + $q.trader }

    foreach ($mk in $buckets.Keys) {
        $points = @($buckets[$mk] | Sort-Object { $_.label }, { $_.x })
        $quest = [ordered]@{
            id       = $q.id
            name     = (Resolve-Task $q.name)
            trader   = $traderName
            faction  = $q.factionName
            minLevel = $q.minPlayerLevel
            wiki     = $wiki
            points   = $points
        }
        if ($keyNames.Count -gt 0) { $quest['keys'] = @($keyNames) }
        if ($objTexts.Count -gt 0) { $quest['objectives'] = @($objTexts) }
        if (-not $questsOut.Contains($mk)) { $questsOut[$mk] = @() }
        $questsOut[$mk] = @($questsOut[$mk]) + $quest
    }
}

# списки квестов по алфавиту — стабильный diff
foreach ($k in @($questsOut.Keys)) {
    $questsOut[$k] = [ordered]@{ quests = @($questsOut[$k] | Sort-Object { $_.name }) }
}
# Отброшенные карты: известные без арта — честное предупреждение,
# НЕИЗВЕСТНЫЕ — ошибка (новую карту надо вносить осознанно).
foreach ($k in $droppedMaps.Keys) {
    if ($knownUnmapped.ContainsKey($k)) {
        Write-Output ("WARN: kvesty {0} otbrasheny (net arta): {1} Tochek" -f $knownUnmapped[$k], $droppedMaps[$k])
    } else {
        throw ("neizvestnyy map id v tasks: {0} — dobav' v data/maps.json, questAlias ili knownUnmapped" -f $k)
    }
}
if ($unknownTraders.Count -gt 0) {
    throw ("neizvestnye treydery: " + (($unknownTraders.GetEnumerator() | ForEach-Object { "$($_.Key) ($($_.Value))" }) -join ', ') + " — dopolni tablicu `$traders")
}
$json = ConvertTo-Json -InputObject $questsOut -Depth 7
[System.IO.File]::WriteAllText((Join-Path $root 'data\quests.json'), $json, (New-Object System.Text.UTF8Encoding($false)))
$total = 0; foreach ($k in $questsOut.Keys) { $total += $questsOut[$k].quests.Count }
"OK: data/quests.json ($total kvestov, $nPoints tochek)"

# ---------------------------------------------------------------------------
# data/locks.json — двери-замки карт: { ключ -> [{key, x, y, z}] }.
# Источник: locks[] в каталоге карт (id предмета-ключа + позиция двери).
# На карте замок рисуется только если его ключ нужен отслеживаемому квесту.
# ---------------------------------------------------------------------------
$locksOut = [ordered]@{}
$nLocks = 0
foreach ($p in $cat.data.maps.PSObject.Properties) {
    $m = $p.Value
    if (-not $m.locks -or $m.locks.Count -eq 0) { continue }
    $key = $byId[$m.id]
    if (-not $key) { continue }
    $seen = @{}
    $arr = @()
    foreach ($l in $m.locks) {
        if (-not $l.position) { continue }
        $nm = Resolve-ItemName $l.key
        $sig = '{0}|{1}|{2}|{3}' -f $nm, [math]::Round($l.position.x, 1), [math]::Round($l.position.y, 1), [math]::Round($l.position.z, 1)
        if ($seen[$sig]) { continue }
        $seen[$sig] = 1
        $arr += [ordered]@{
            key = $nm
            x   = [math]::Round($l.position.x, 2)
            y   = [math]::Round($l.position.y, 2)
            z   = [math]::Round($l.position.z, 2)
        }
        $nLocks++
    }
    if ($arr.Count -gt 0) { $locksOut[$key] = $arr }
}
$json = ConvertTo-Json -InputObject $locksOut -Depth 5
[System.IO.File]::WriteAllText((Join-Path $root 'data\locks.json'), $json, (New-Object System.Text.UTF8Encoding($false)))
"OK: data/locks.json ($nLocks zamekov)"

# ---------------------------------------------------------------------------
# data/farm.json — данные «Помощника фарма»: спавны + лут-точки по категориям.
# Категории: t=техлут убежища, m=медицина, s=продажа, h=дорогой техлут
# (акб/фазированные решётки/GPU/bitcoin/LEDX/Virtex), w=оружейный.
# Контейнеры категоризируются по типу (normalizedName); «тела» (scav-body,
# pmc-body, civilian-body, dead-scav, lab-technician-body) — шум, пропускаем.
# Loose-лут категоризируется по пулу предметов (white-list по имени).
# Словарь предметов общий с ключами квестов (items_en, парсинг выше).
# ---------------------------------------------------------------------------
$hiRe = 'phased array|car battery|graphics card|physical bitcoin|ledx|virtex|military power filter|tetriz'
$tlRe = 'wd-40|screw nut|relay|power cord|electric drill|spark plug|military cable|pc cpu|cpu fan|gas analyzer'
$catByType = @{
    'toolbox' = 't'; 'technical-supply-crate' = 't'; 'pc-block' = 't'; 'drawer' = 't'
    'medcase' = 'm'; 'medbag' = 'm'; 'medbag-smu06' = 'm'; 'medical-supply-crate' = 'm'
    'safe' = 's'; 'bank-safe' = 's'; 'cash-register' = 's'; 'bank-cash-register' = 's'
    'jacket' = 's'; 'duffle-bag' = 's'; 'plastic-suitcase' = 's'; 'shturmans-stash' = 's'
    'ground-cache' = 's'; 'buried-barrel-cache' = 's'; 'ration-supply-crate' = 's'; 'wooden-crate' = 's'
    'weapon-box' = 'w'; 'wooden-ammo-box' = 'w'; 'grenade-box' = 'w'
}
# Имена предметов для loose-лута — тот же словарь items_en, что и для ключей
# (парсинг выше); отдельно не качаем и не парсим.
$names = $itemsName
$farmOut = [ordered]@{}
$nSpots = 0
$nSpawns = 0
foreach ($p in $cat.data.maps.PSObject.Properties) {
    $m = $p.Value
    $key = $byId[$m.id]
    if (-not $key) { continue }
    $fb = $bounds.PSObject.Properties[$key].Value.svg.bounds  # для отсева мусорных координат вне карты
    $sp = @()
    foreach ($s in $m.spawns) {
        if (-not $s.position) { continue }
        if (-not (Test-InBounds $fb $s.position.x $s.position.z 0.05)) { continue }
        $pmc = $s.sides -contains 'pmc' -or $s.sides -contains 'all'
        if ($pmc -and $s.categories -contains 'player') {
            $sp += [ordered]@{
                x = [math]::Round($s.position.x, 1)
                y = [math]::Round($s.position.y, 1)
                z = [math]::Round($s.position.z, 1)
            }
        }
    }
    $spots = @()
    foreach ($e in $m.lootContainers) {
        if (-not $e.position) { continue }
        if (-not (Test-InBounds $fb $e.position.x $e.position.z 0.05)) { continue }
        $t = $cat.data.lootContainers.PSObject.Properties[$e.lootContainer].Value
        if (-not $t) { continue }
        $c = $catByType[$t.normalizedName]
        if (-not $c) { continue }
        $spots += [ordered]@{
            c = $c
            n = $t.normalizedName
            x = [math]::Round($e.position.x, 1)
            y = [math]::Round($e.position.y, 1)
            z = [math]::Round($e.position.z, 1)
        }
    }
    foreach ($l in $m.lootLoose) {
        if (-not $l.position -or -not $l.items) { continue }
        if (-not (Test-InBounds $fb $l.position.x $l.position.z 0.05)) { continue }
        $c = $null
        $hit = $null
        foreach ($iid in $l.items) {
            $nm = $names[$iid]
            if (-not $nm) { continue }
            if ($nm -imatch $hiRe) { $c = 'h'; $hit = $nm; break }  # дорогое — приоритет
            if (-not $c -and $nm -imatch $tlRe) { $c = 't'; $hit = $nm }
        }
        if (-not $c) { continue }
        $spots += [ordered]@{
            c = $c
            n = $hit
            x = [math]::Round($l.position.x, 1)
            y = [math]::Round($l.position.y, 1)
            z = [math]::Round($l.position.z, 1)
        }
    }
    if ($spots.Count -gt 0) {
        $farmOut[$key] = [ordered]@{ spawns = $sp; spots = $spots }
        $nSpots += $spots.Count
        $nSpawns += $sp.Count
    }
}
$json = ConvertTo-Json -InputObject $farmOut -Depth 5
[System.IO.File]::WriteAllText((Join-Path $root 'data\farm.json'), $json, (New-Object System.Text.UTF8Encoding($false)))
"OK: data/farm.json ($nSpots tochek, $nSpawns spawnov)"

# ---------------------------------------------------------------------------
# data/hazards.json — опасности: минные поля, снайперские зоны, прочее.
# Источник: hazards[] каталога карт tarkov.dev (hazardType: minefield/sniper/
# hazard; позиция + контур-зона). Клейморы Рогов в tarkov.dev ОТсутствуют —
# на Маяке добираются из PSD reemr (слой NewOverlay/Claymores, центры bbox;
# конвертация px->мир — калибровка по арту reemr) через необязательный
# %TEMP%\tmmt_clay_lh.json (массив [[x,z],...]).
# ---------------------------------------------------------------------------
$hzOut = [ordered]@{}
$nHz = 0
foreach ($p in $cat.data.maps.PSObject.Properties) {
    $m = $p.Value
    $key = $byId[$m.id]
    if (-not $key) { continue }
    $fb = $bounds.PSObject.Properties[$key].Value.svg.bounds
    $arr = @()
    $seen = @{}
    foreach ($h in $m.hazards) {
        if (-not $h.position) { continue }
        $t = switch ($h.hazardType) { 'minefield' { 'm' } 'sniper' { 's' } default { 'h' } }
        if (-not (Test-InBounds $fb $h.position.x $h.position.z 0.05)) { continue }
        $sig = '{0}|{1}|{2}' -f $t, [math]::Round($h.position.x, 1), [math]::Round($h.position.z, 1)
        if ($seen[$sig]) { continue }
        $seen[$sig] = 1
        $ol = New-Object System.Collections.Generic.List[object]
        foreach ($pt in $h.outline) {
            $ol.Add(@([math]::Round($pt.x, 2), [math]::Round($pt.z, 2)))
        }
        $arr += [ordered]@{
            t = $t
            x = [math]::Round($h.position.x, 2)
            y = [math]::Round($h.position.y, 2)
            z = [math]::Round($h.position.z, 2)
            ol = $ol
        }
        $nHz++
    }
    if ($key -eq 'lighthouse' -and (Test-Path "$env:TEMP\tmmt_clay_lh.json")) {
        foreach ($c in (Get-Content "$env:TEMP\tmmt_clay_lh.json" -Raw | ConvertFrom-Json)) {
            $arr += [ordered]@{ t = 'c'; x = $c[0]; y = 0; z = $c[1]; ol = @() }
            $nHz++
        }
    }
    if ($arr.Count -gt 0) { $hzOut[$key] = $arr }
}
$json = ConvertTo-Json -InputObject $hzOut -Depth 6
[System.IO.File]::WriteAllText((Join-Path $root 'data\hazards.json'), $json, (New-Object System.Text.UTF8Encoding($false)))
"OK: data/hazards.json ($nHz opasnostey)"

# ---------------------------------------------------------------------------
# data/loc_ru.json (9.5, Phase D) — компактный EN->RU словарь ИГРОВЫХ строк:
# имена выходов/транзитов, квестов, целей, ключей и лута фарма. Собирается
# только из строк, реально попавших в exits/quests/locks/farm.json (~100-300КБ
# вместо 2.8МБ сырых словарей). UI-строки лежат в коде (lang.go).
# Названия карт — в maps.json (locale.ru), сюда не дублируются.
# ---------------------------------------------------------------------------
$containerRU = @{
    'toolbox'                = 'Ящик с инструментами'
    'technical-supply-crate' = 'Тех. ящик'
    'pc-block'               = 'Системный блок'
    'drawer'                 = 'Ящик стола'
    'medcase'                = 'Медкейс'
    'medbag'                 = 'Мед. сумка'
    'medbag-smu06'           = 'Сумка СМУ-06'
    'medical-supply-crate'   = 'Мед. ящик'
    'safe'                   = 'Сейф'
    'bank-safe'              = 'Банковский сейф'
    'cash-register'          = 'Касса'
    'bank-cash-register'     = 'Банковская касса'
    'jacket'                 = 'Куртка'
    'duffle-bag'             = 'Спортивная сумка'
    'plastic-suitcase'       = 'Пластиковый чемодан'
    'shturmans-stash'        = 'Тайник Штурмана'
    'ground-cache'           = 'Наземный тайник'
    'buried-barrel-cache'    = 'Закопанная бочка'
    'ration-supply-crate'    = 'Ящик с пайками'
    'wooden-crate'           = 'Деревянный ящик'
    'weapon-box'             = 'Ящик с оружием'
    'wooden-ammo-box'        = 'Ящик с патронами'
    'grenade-box'            = 'Коробка с гранатами'
}

# Строки, которые реально встречаются в наших data/*.json (маппинг по ним).
$need = New-Object 'System.Collections.Generic.HashSet[string]'
$collect = { param($x) if ($x) { [void]$need.Add([string]$x) } }
$curExits = [System.IO.File]::ReadAllText((Join-Path $root 'data\exits.json')) | ConvertFrom-Json
foreach ($p in $curExits.PSObject.Properties) {
    foreach ($e in $p.Value.extracts) { & $collect $e.name }
    foreach ($t in $p.Value.transits) { & $collect $t.name }
}
$curQuests = [System.IO.File]::ReadAllText((Join-Path $root 'data\quests.json')) | ConvertFrom-Json
foreach ($p in $curQuests.PSObject.Properties) {
    foreach ($q in $p.Value.quests) {
        & $collect $q.name
        foreach ($o in $q.objectives) { & $collect $o }
        foreach ($k in $q.keys) { & $collect $k }
        foreach ($pt in $q.points) { & $collect $pt.label }
    }
}
$curLocks = [System.IO.File]::ReadAllText((Join-Path $root 'data\locks.json')) | ConvertFrom-Json
foreach ($p in $curLocks.PSObject.Properties) {
    foreach ($l in $p.Value) { & $collect $l.key }
}
$curFarm = [System.IO.File]::ReadAllText((Join-Path $root 'data\farm.json')) | ConvertFrom-Json
foreach ($p in $curFarm.PSObject.Properties) {
    foreach ($s in $p.Value.spots) { & $collect $s.n }
}
"LOC: уникальных строк к переводу: $($need.Count)"

$dict = [ordered]@{}
function Add-Loc([string]$en, [string]$ru) {
    if (-not $en -or -not $ru) { return }
    if ($en -eq $ru) { return }
    if (-not $dict.Contains($en)) { $dict[$en] = $ru }
}

# 1) Выходы: maps_en.data {tdevId->name} <-> maps_ru.data {tdevId->имя}.
$mapsRu = $null
if (Test-Path "$env:TEMP\tmmt_maps_ru.json") {
    $mapsRu = ([System.IO.File]::ReadAllText("$env:TEMP\tmmt_maps_ru.json") | ConvertFrom-Json).data
}
if ($mapsRu) {
    foreach ($p in $en.PSObject.Properties) {
        if ($need.Contains([string]$p.Value)) {
            $rp = $mapsRu.PSObject.Properties[$p.Name]
            if ($rp) { Add-Loc ([string]$p.Value) ([string]$rp.Value) }
        }
    }
}

# 2) Квесты и цели: tasks_en.data {id->text} <-> tasks_ru.data {id->текст}.
$tasksRu = $null
if (Test-Path "$env:TEMP\tmmt_tasks_ru.json") {
    $tasksRu = ([System.IO.File]::ReadAllText("$env:TEMP\tmmt_tasks_ru.json") | ConvertFrom-Json).data
}
if ($tasksRu) {
    foreach ($p in $ten.PSObject.Properties) {
        $ruTxt = $null
        $rp = $tasksRu.PSObject.Properties[$p.Name]
        if ($rp -and $rp.Value) { $ruTxt = [string]$rp.Value }
        if (-not $ruTxt) { continue }
        $enTxt = [string]$p.Value
        if ($need.Contains($enTxt)) { Add-Loc $enTxt $ruTxt }
        # Цели с префиксом "(optional) " (добавляется при сборе квестов):
        # в tasks_en/tasks_ru лежит БАЗОВЫЙ текст, поэтому ищем словарную
        # пару по базе, а префикс переносим в перевод (иначе 50+ строк
        # оставались бы EN).
        $optEn = '(optional) ' + $enTxt
        if ($need.Contains($optEn)) { Add-Loc $optEn ('(опционально) ' + $ruTxt) }
    }
}

# 3) Предметы/ключи: items_en flat {"<id> ShortName"/"<id> Name"} ->
#    items_ru flat (те же ключи). Short мапим на Short, Name на Name —
#    раньше Short подставляли полное RU-имя.
$itemsRuShort = @{}
$itemsRuName = @{}
if (Test-Path "$env:TEMP\tmmt_items_full_ru.json") {
    $rawItemsRu = [System.IO.File]::ReadAllText("$env:TEMP\tmmt_items_full_ru.json")
    foreach ($m in [regex]::Matches($rawItemsRu, '"([0-9a-f]{24}) Name"\s*:\s*"([^"]+)"')) {
        $itemsRuName[$m.Groups[1].Value] = $m.Groups[2].Value
    }
    foreach ($m in [regex]::Matches($rawItemsRu, '"([0-9a-f]{24}) ShortName"\s*:\s*"([^"]+)"')) {
        $itemsRuShort[$m.Groups[1].Value] = $m.Groups[2].Value
    }
}
if (($itemsShort.Count -gt 0 -or $itemsName.Count -gt 0) -and ($itemsRuShort.Count -gt 0 -or $itemsRuName.Count -gt 0)) {
    $ids = New-Object 'System.Collections.Generic.HashSet[string]'
    foreach ($k in $itemsShort.Keys) { [void]$ids.Add($k) }
    foreach ($k in $itemsName.Keys) { [void]$ids.Add($k) }
    foreach ($id in $ids) {
        if ($itemsShort.ContainsKey($id) -and $need.Contains($itemsShort[$id])) {
            $ruStr = $null
            if ($itemsRuShort.ContainsKey($id)) { $ruStr = $itemsRuShort[$id] }
            elseif ($itemsRuName.ContainsKey($id)) { $ruStr = $itemsRuName[$id] }
            if ($ruStr) { Add-Loc $itemsShort[$id] $ruStr }
        }
        if ($itemsName.ContainsKey($id) -and $need.Contains($itemsName[$id])) {
            $ruStr = $null
            if ($itemsRuName.ContainsKey($id)) { $ruStr = $itemsRuName[$id] }
            if ($ruStr) { Add-Loc $itemsName[$id] $ruStr }
        }
    }
}

# 4) Контейнеры фарма (normalizedName — RU-словарь в скрипте).
foreach ($p in $containerRU.GetEnumerator()) {
    if ($need.Contains([string]$p.Key)) { Add-Loc ([string]$p.Key) ([string]$p.Value) }
}

# Стабильный diff: ключи по алфавиту.
$outLoc = [ordered]@{}
foreach ($e in ($dict.GetEnumerator() | Sort-Object Key)) { $outLoc[$e.Key] = $e.Value }
$json = ConvertTo-Json -InputObject $outLoc -Depth 3
[System.IO.File]::WriteAllText((Join-Path $root 'data\loc_ru.json'), $json, (New-Object System.Text.UTF8Encoding($false)))
"OK: data/loc_ru.json ($($outLoc.Count) perevodov)"

