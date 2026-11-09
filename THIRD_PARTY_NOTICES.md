# Third-party notices and attributions

TarkyMappy embeds community map artwork and community-maintained game data.
Those materials remain the property of their authors and are distributed under
their own licenses. The project code is covered by the repository `LICENSE`.

## Map artwork

- **TarkovTracker / tarkovdata** — SVG map sources used to rasterize the
  per-floor PNG maps.
  Source: https://github.com/TarkovTracker/tarkovdata
  License: **CC BY-NC-SA 4.0**
  https://creativecommons.org/licenses/by-nc-sa/4.0/
- **reemr / re3mr.com** — Lighthouse v1.7 map artwork used as the custom
  Lighthouse background (`maps/lighthouse__Ground_Level.jpg`).
  License: **CC BY-NC-SA 4.0**
  https://creativecommons.org/licenses/by-nc-sa/4.0/

The maps in this repository were converted from SVG to PNG/JPEG, split by
floor, and combined for the "flat map" mode. These are adaptations of the
original artwork and are distributed under the same CC BY-NC-SA 4.0 terms:
attribution is required, commercial use is not allowed, and adaptations must
be shared under the same license.

## Game data

- **tarkov.dev / json.tarkov.dev** — exits, transits, quests, objectives,
  traders, item names, loot containers, spawns, locks, and hazard positions.
  Source: https://tarkov.dev / https://json.tarkov.dev
  Data is used as a local offline snapshot; see `data/meta.json` for the
  snapshot date and game build.
- **Escape from Tarkov** — game names, item names, quest text, and related
  marks belong to Battlestate Games. This project is unofficial and is not
  affiliated with or endorsed by Battlestate Games.

## Go dependencies

The Go module graph is declared in `go.mod` / `go.sum`. Dependencies are not
vendored in this repository; their own licenses apply:

- Ebitengine (`github.com/hajimehoshi/ebiten/v2`) — Apache-2.0 / BSD-style
  components, see the upstream repository.
- fsnotify (`github.com/fsnotify/fsnotify`) — BSD-3-Clause.
- `golang.org/x/image`, `golang.org/x/sys`, `golang.org/x/text` — BSD-3-Clause.

## Fonts

The UI uses the Go Mono font from `golang.org/x/image/font/gofont/gomono`
(BSD-3-Clause), embedded through the Go module.

## Trademarks

Escape from Tarkov, Battlestate Games, and related names are trademarks of
their respective owners. TarkyMappy is an unofficial community project.
