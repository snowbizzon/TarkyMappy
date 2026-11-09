param(
    [ValidateSet('ru', 'en')]
    [string]$Lang = 'en',
    [switch]$Auto,
    [switch]$NoZip
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root
$isRU = ($Lang -eq 'ru')

function Say {
    param([string]$Ru, [string]$En, [string]$Color = 'Gray')
    $text = $En
    if ($isRU) { $text = $Ru }
    Write-Host $text -ForegroundColor $Color
}

function Rule {
    param([string]$Color = 'DarkCyan')
    Write-Host ('=' * 72) -ForegroundColor $Color
}

function Head {
    param([string]$Ru, [string]$En)
    Write-Host ''
    Rule 'Cyan'
    Say "  $Ru" "  $En" 'Cyan'
    Rule 'Cyan'
}

function Step {
    param([string]$Number, [string]$Ru, [string]$En)
    Write-Host ''
    Say "[$Number] $Ru" "[$Number] $En" 'Yellow'
}

function Ok {
    param([string]$Ru, [string]$En)
    Say "  [OK] $Ru" "  [OK] $En" 'Green'
}

function Warn {
    param([string]$Ru, [string]$En)
    Say "  [!] $Ru" "  [!] $En" 'Yellow'
}

function Pause {
    $message = 'Press Enter to exit...'
    if ($isRU) { $message = 'Нажмите Enter для выхода...' }
    Read-Host $message | Out-Null
}

function Fail {
    param([string]$Ru, [string]$En)
    Say "  [X] $Ru" "  [X] $En" 'Red'
    if (-not $Auto) { Pause }
    exit 1
}

function Ask {
    param([string]$Ru, [string]$En, [string]$Default = 'N')
    if ($Auto) { return $false }
    $prompt = $En
    if ($isRU) { $prompt = $Ru }
    $answer = Read-Host "$prompt [$Default]"
    if ([string]::IsNullOrWhiteSpace($answer)) { $answer = $Default }
    return ($answer -match '^(y|yes|д|да)$')
}

function Have {
    param([string]$Path)
    return (Test-Path -LiteralPath (Join-Path $Root $Path))
}

Head 'TarkyMappy — помощник сборки' 'TarkyMappy — build assistant'
Say 'Соберёт готовый TarkyMappy.exe, проверит тесты и сложит всё в папку dist.' 'Builds a ready TarkyMappy.exe, runs tests, and puts everything into dist.' 'White'
Say 'Ничего не отправляется в интернет, кроме go mod download при первой сборке.' 'Nothing is uploaded. The only network access is go mod download on the first build.' 'DarkGray'

# --- 1. Source tree ----------------------------------------------------------
Step '1/7' 'Проверка файлов проекта' 'Checking project files'
$required = @(
    'go.mod', 'go.sum', 'main.go', 'icon64.png', 'rsrc_windows_amd64.syso',
    'data\maps.json', 'data\meta.json', 'maps', 'prices\prices.json'
)
$missing = @()
foreach ($item in $required) {
    if (-not (Have $item)) { $missing += $item }
}
if ($missing.Count -gt 0) {
    Fail ("Не найдены: " + ($missing -join ', ')) ("Missing: " + ($missing -join ', '))
}
Ok 'Все обязательные файлы на месте' 'All required files are present'

# --- 2. Go -------------------------------------------------------------------
Step '2/7' 'Проверка Go 1.27+' 'Checking Go 1.27+'
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    Warn 'Go не найден в PATH' 'Go was not found in PATH'
    if (Ask 'Установить Go через winget?' 'Install Go with winget?' 'Y') {
        $winget = Get-Command winget -ErrorAction SilentlyContinue
        if (-not $winget) {
            Fail 'winget недоступен. Установите Go вручную: https://go.dev/dl/' 'winget is unavailable. Install Go manually: https://go.dev/dl/'
        }
        & winget install --id GoLang.Go -e --accept-source-agreements --accept-package-agreements
        $env:Path = "C:\Program Files\Go\bin;$env:Path"
        $go = Get-Command go -ErrorAction SilentlyContinue
    }
}
if (-not $go) {
    Fail 'Установите Go 1.27+ и запустите сборку снова.' 'Install Go 1.27+ and run the build again.'
}
$goVersion = (& go version) 2>&1
Say "  $goVersion" "  $goVersion" 'DarkGray'
if ($goVersion -notmatch 'go1\.(2[7-9]|[3-9][0-9])') {
    Fail 'Нужен Go 1.27 или новее. Обновите Go: https://go.dev/dl/' 'Go 1.27 or newer is required. Update Go: https://go.dev/dl/'
}
Ok 'Go подходит' 'Go version is supported'

# --- 3. Prices ---------------------------------------------------------------
Step '3/7' 'Проверка цен барахолки' 'Checking flea market prices'
if (Have 'prices\prices.json') {
    Ok 'prices\prices.json найден' 'prices\prices.json found'
} else {
    Warn 'prices\prices.json отсутствует' 'prices\prices.json is missing'
    $python = Get-Command python -ErrorAction SilentlyContinue
    if ($python -and (Ask 'Обновить цены сейчас? Нужен интернет.' 'Update prices now? Internet is required.' 'Y')) {
        & python (Join-Path $Root 'prices\update_prices.py')
        if ($LASTEXITCODE -ne 0) { Warn 'Не удалось обновить цены; F2 будет пустым.' 'Price update failed; F2 will be empty.' }
    }
}

# --- 4. Dependencies ---------------------------------------------------------
Step '4/7' 'Загрузка зависимостей' 'Downloading dependencies'
& go mod download
if ($LASTEXITCODE -ne 0) { Fail 'go mod download завершился с ошибкой' 'go mod download failed' }
Ok 'Зависимости загружены' 'Dependencies downloaded'

# --- 5. Tests and static checks ---------------------------------------------
Step '5/7' 'Тесты, vet и gofmt' 'Tests, vet and gofmt'
& go test ./...
if ($LASTEXITCODE -ne 0) { Fail 'Тесты не прошли. Сборка остановлена.' 'Tests failed. Build stopped.' }
Ok 'Тесты прошли' 'Tests passed'

& go vet ./...
if ($LASTEXITCODE -ne 0) { Fail 'go vet нашёл проблемы. Сборка остановлена.' 'go vet found problems. Build stopped.' }
Ok 'go vet чистый' 'go vet is clean'

$fmt = (& gofmt -l .)
if ($fmt) {
    Warn 'gofmt предлагает изменения:' 'gofmt suggests changes:'
    $fmt | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkYellow }
} else {
    Ok 'gofmt чистый' 'gofmt is clean'
}

# --- 6. Build ----------------------------------------------------------------
Step '6/7' 'Сборка TarkyMappy.exe' 'Building TarkyMappy.exe'
$dist = Join-Path $Root 'dist'
if (Test-Path $dist) { Remove-Item -LiteralPath $dist -Recurse -Force }
New-Item -ItemType Directory -Path $dist | Out-Null
$exe = Join-Path $dist 'TarkyMappy.exe'
& go build -trimpath -ldflags="-s -w -H=windowsgui" -o $exe .
if ($LASTEXITCODE -ne 0 -or -not (Test-Path $exe)) {
    Fail 'Не удалось собрать exe' 'Failed to build the exe'
}
$size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
Ok "Собран TarkyMappy.exe ($size МБ)" "Built TarkyMappy.exe ($size MB)"

# Copy runtime data and docs next to the exe.
Copy-Item -LiteralPath (Join-Path $Root 'prices') -Destination (Join-Path $dist 'prices') -Recurse -Force
Get-ChildItem -Path (Join-Path $dist 'prices') -Filter '__pycache__' -Recurse -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
foreach ($doc in @('README.md', 'FAQ.md', 'HOWTOUSE_RU.md', 'HOWTOUSE_EN.md', 'UPDATING.md', 'LICENSE', 'THIRD_PARTY_NOTICES.md')) {
    if (Have $doc) { Copy-Item -LiteralPath (Join-Path $Root $doc) -Destination $dist -Force }
}
Ok 'prices\ и документация скопированы в dist' 'prices\ and docs copied to dist'

# A short plain-text reminder for users who unzip the release.
$firstRU = @"
TarkyMappy - быстрый старт
==========================
1. Запустите TarkyMappy.exe.
2. Зайдите в рейд и сделайте ОДИН скриншот (PrtSc).
   Без первого скриншота оверлей не знает вашу позицию.
3. Скриншоты должны сохраняться в:
   Documents\Escape from Tarkov\Screenshots
4. Если папка другая: F9 -> Browse -> укажите её -> Apply.
5. Хоткеи: F2 цены, F3 фарм, F4 квесты, F5/F6 зум,
   F7 скрыть, F8 выход, F9 настройки, F10 смена карты,
   PgUp/PgDn этажи, ? справка.
6. Цены обновляются без пересборки:
   python prices\update_prices.py
Подробно: HOWTOUSE_RU.md
"@
$firstEN = @"
TarkyMappy - quick start
========================
1. Run TarkyMappy.exe.
2. Enter a raid and take ONE screenshot (PrtSc).
   Without the first screenshot the overlay cannot know your position.
3. Screenshots must be saved to:
   Documents\Escape from Tarkov\Screenshots
4. If your folder is different: F9 -> Browse -> select it -> Apply.
5. Hotkeys: F2 prices, F3 farm, F4 quests, F5/F6 zoom,
   F7 hide, F8 quit, F9 settings, F10 next map,
   PgUp/PgDn floors, ? help.
6. Prices can be updated without rebuilding:
   python prices\update_prices.py
Details: HOWTOUSE_EN.md
"@
[IO.File]::WriteAllText((Join-Path $dist 'README_FIRST_RU.txt'), $firstRU, (New-Object System.Text.UTF8Encoding($true)))
[IO.File]::WriteAllText((Join-Path $dist 'README_FIRST_EN.txt'), $firstEN, (New-Object System.Text.UTF8Encoding($false)))

# --- 7. Package --------------------------------------------------------------
Step '7/7' 'Упаковка' 'Packaging'
$zip = Join-Path $Root 'TarkyMappy-win64.zip'
if (-not $NoZip -and (Ask 'Создать TarkyMappy-win64.zip?' 'Create TarkyMappy-win64.zip?' 'Y')) {
    if (Test-Path $zip) { Remove-Item -LiteralPath $zip -Force }
    Compress-Archive -Path (Join-Path $dist '*') -DestinationPath $zip -Force
    Ok "Создан $zip" "Created $zip"
} else {
    Warn 'ZIP не создавался' 'ZIP was not created'
}

Head 'Готово' 'Done'
Say "  dist\TarkyMappy.exe" "  dist\TarkyMappy.exe" 'White'
Say '  Не забудьте первый скриншот в рейде — иначе позиции нет.' '  Do not forget the first in-raid screenshot — otherwise there is no position.' 'Yellow'
Say '  Проверка: запустите exe, нажмите F9 и укажите папку скриншотов.' '  Check: run the exe, press F9 and select the screenshots folder.' 'Gray'
if (-not $Auto) {
    if (Ask 'Открыть папку dist?' 'Open the dist folder?' 'Y') {
        Start-Process explorer.exe $dist
    }
    if (Ask 'Запустить TarkyMappy.exe сейчас?' 'Run TarkyMappy.exe now?' 'N') {
        Start-Process $exe
    }
    Pause
}
exit 0
