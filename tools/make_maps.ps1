# Dev-utility: SVG-karty tarkovdata -> per-floor PNG maps/<key>__<Floor>.png (+ <key>__ALL.png).
# Etazhi = kornevye <g id="..."> SVG; vse etazhi v odnom viewBox -> UV-privyazka edina.
# defaultFloor rendreritsya 4096px, ostalnye etazhi i combined ALL - 2048px.
# -Force: pererenderit dazhe esli PNG svezhe SVG.
param([switch]$Force, [string]$Magick = "magick")

$ErrorActionPreference = 'Stop'
$root   = Split-Path -Parent $PSScriptRoot
$outDir = Join-Path $root 'maps'
$magick = $Magick
if (-not (Get-Command $magick -ErrorAction SilentlyContinue) -and -not (Test-Path $magick)) {
    throw "ImageMagick not found. Install it and add magick.exe to PATH, or run: .\tools\make_maps.ps1 -Magick 'C:\path\to\magick.exe'"
}
$base   = 'https://raw.githubusercontent.com/TarkovTracker/tarkovdata/master/maps'

$mapsPath = Join-Path $root 'data\maps.json'
$maps = Get-Content $mapsPath -Raw | ConvertFrom-Json
$changed = $false
New-Item -ItemType Directory -Force -Path $outDir | Out-Null

function Get-ViewBoxMax([string]$path) {
    $head = [IO.File]::ReadAllText($path)
    if ($head.Length -gt 4000) { $head = $head.Substring(0, 4000) }
    $m = [regex]::Match($head, 'viewBox="0 0 ([\d.]+) ([\d.]+)"')
    $v = [math]::Max([double]$m.Groups[1].Value, [double]$m.Groups[2].Value)
    if ($v -le 0) { $v = 1000 }
    return $v
}

# Rasterize: density-render SVG -> PNG s dolgoi storonoi $longSide.
function Rasterize([string]$svg, [string]$png, [int]$longSide) {
    $density = [math]::Ceiling(96 * $longSide / (Get-ViewBoxMax $svg))
    & $magick -background none -density $density $svg -resize ('{0}x{0}>' -f $longSide) $png 2>$null
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path $png)) { throw "magick: $svg -> $png" }
}

foreach ($p in $maps.PSObject.Properties) {
    $key  = $p.Name
    # Карты с кастомным артом (Lighthouse rework: фотарт reemr, 9.8) рендерит
    # НЕ tarkovdata-SVG — не перезатирать!
    if ($p.Value.svg.customArt) { Write-Output "$key`: custom art (svg.customArt) - skip"; continue }
    $file = $p.Value.svg.file
    $svg  = Join-Path $env:TEMP "tmmt_$file"
    if (-not (Test-Path $svg)) { curl.exe -sL "$base/$file" -o $svg }
    if (-not (Test-Path $svg) -or (Get-Item $svg).Length -lt 1000) { throw "download failed: $file" }

    # Fakticheskie etazhi - kornevye <g> dokumenta.
    $doc = New-Object System.Xml.XmlDocument
    $doc.LoadXml([IO.File]::ReadAllText($svg))
    $svgFloors = @($doc.svg.ChildNodes | Where-Object { $_.Name -eq 'g' } | ForEach-Object { $_.id })
    if ($svgFloors.Count -eq 0) { throw "\${file}: net kornevyh <g>" }

    # Sink svg.floors v maps.json: poryadok iz json (snizu vverh), novye - v konec.
    $jsonFloors = @($p.Value.svg.floors)
    $ordered = @($jsonFloors | Where-Object { $svgFloors -ccontains $_ })
    foreach ($f in $svgFloors) { if ($ordered -cnotcontains $f) { $ordered += $f } }
    if (($ordered -join ',') -cne ($jsonFloors -join ',')) {
        Write-Output "$key`: floors -> $($ordered -join ', ')"
        $p.Value.svg.floors = $ordered
        $changed = $true
    }
    $defaultFloor = $p.Value.svg.defaultFloor
    if ($ordered -cnotcontains $defaultFloor) { $defaultFloor = $ordered[0] }
    $multi = $ordered.Count -gt 1

    foreach ($floor in $ordered) {
        $png = Join-Path $outDir ("{0}__{1}.png" -f $key, $floor)
        if (-not $Force -and (Test-Path $png) -and (Get-Item $png).LastWriteTime -ge (Get-Item $svg).LastWriteTime) {
            Write-Output "$key/$floor`: kesh (skip)"
            continue
        }
        # Klon dokumenta: ubiraem chuzhie etazhi; ground pri ne-ground etazhe - prizrak 0.35.
        $d = New-Object System.Xml.XmlDocument
        $d.LoadXml([IO.File]::ReadAllText($svg))
        $isGround = $floor -match 'Ground'
        foreach ($c in @($d.svg.ChildNodes)) {
            if ($c.Name -ne 'g') { continue }
            if ($c.id -ceq $floor) { continue }
            if (-not $isGround -and $c.id -match 'Ground') {
                $c.SetAttribute('opacity', '0.35')
            } else {
                [void]$d.svg.RemoveChild($c)
            }
        }
        $tmp = Join-Path $env:TEMP ("tmmt_{0}__{1}.svg" -f $key, $floor)
        $d.Save($tmp)
        $longSide = 2048
        if ($floor -ceq $defaultFloor) { $longSide = 4096 }
        Rasterize $tmp $png $longSide
        Write-Output ("{0}/{1}: {2} KB" -f $key, $floor, [math]::Round((Get-Item $png).Length/1KB))
    }

    # Combined "ploskaya karta" (failswitch <Ignotirovat etazhi>) - tolko dlya mnogoetazhnyh.
    if ($multi) {
        $pngAll = Join-Path $outDir ("{0}__ALL.png" -f $key)
        if ($Force -or -not (Test-Path $pngAll) -or (Get-Item $pngAll).LastWriteTime -lt (Get-Item $svg).LastWriteTime) {
            Rasterize $svg $pngAll 2048
            Write-Output ("{0}/ALL: {1} KB" -f $key, [math]::Round((Get-Item $pngAll).Length/1KB))
        }
        $old = Join-Path $outDir "$key.png"
        if (Test-Path $old) { Remove-Item $old }
    }
}
if ($changed) {
    # UTF-8 БЕЗ BOM: PS 5.1 Set-Content -Encoding UTF8 пишет BOM, а Go
    # json.Unmarshal на BOM падает.
    [IO.File]::WriteAllText($mapsPath, ($maps | ConvertTo-Json -Depth 12), (New-Object System.Text.UTF8Encoding($false)))
}
'GOTOVO'


