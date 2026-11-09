# TarkyMappy — FAQ

Transparent click-through minimap overlay for Escape from Tarkov. Offline: all
map/quest/loot/hazard data is embedded in the exe; flea prices are read from
`prices\prices.json` next to the exe. No game memory access, no network in-game.

## How it works

1. **The first screenshot is required.** Press **PrtSc** in game. EFT saves a
   screenshot with player coordinates in the file name to
   `Documents\Escape from Tarkov\Screenshots`. Without this first screenshot
   the overlay has no position and cannot center the map.
2. TarkyMappy watches that folder, reads position + view direction and rotates
   the map around you (you always face "up").
3. If you run the game in offline/PvE mode, map detection also reads the game
   log folder (set it in Settings → Browse, `Logs` dir of the game).

## Global hotkeys (work without focusing the window)

| Key | Action |
|-----|--------|
| F2 | Prices panel (flea market top ₽/slot, PvP/PvE tabs) |
| F3 | Farm Helper panel (loot route) |
| F4 | Quest Helper panel |
| F5 / F6 | Zoom in / out |
| F7 | Hide / show overlay |
| F8 | Quit app |
| F9 | Settings panel |
| F10 | Cycle to next map (sets manual pick) |
| PgUp / PgDn | Floor up / down (multi-floor maps) |
| ? (in F9) | This FAQ panel |

## Map overlay

- White arrow in the center — you; the map rotates so your view is up.
- Exits: **blue** — PMC, **green** — Scav, **yellow** — shared, **purple T|** —
  transit to another map. `^` / `v` before a name — exit on another floor.
- Hazards: **red outline + dot** — minefield, **orange outline** — sniper zone,
  **gold +** — trap/claymore. Toggle in Settings (checkbox "Hazards").
- Bottom-left line: map name, floor tag (U/G/T/F1..F5, FLAT) and your X,Y,Z.
- Farm mode HUD (top): `FARM n/N -> target ~m | exit ~m`.

## F9 Settings

- **EFT screenshots folder** — watched folder; **Browse…** to change,
  **Apply** to save and restart the watcher, **Close** to close the panel.
- Checkboxes **PMC / Scav / Shrd** — which exit types to show.
- **Hazards** — show mines / sniper zones / traps.
- **Manual pick** — choose the map yourself; the list below appears
  (click a map to select, click again to clear). Otherwise auto-detect.
- **Quests (F4)** — open/close the quest panel. Disabled while Farm is active.
- **Farm (F3)** — open/close the farm panel.
- **Prices (F2)** — open/close the prices panel.
- **Ignore floors** — flat combined map instead of per-floor view.
- **Lang: EN/RU** — interface language (saved).
- **debug: on/off** — verbose `tarkymappy.log`.
- Green **?** — FAQ panel; red **X** — quit; gear (right edge, also on any
  panel) — back to Settings; corner grip — resize; top bar — drag the window.

## F4 Quest Helper

- List shows **tracked** quests by default ("Mine"); **All (N)** — every quest
  of the current map. Selection is saved.
- Checkbox on a row — track/untrack; click the row — hints (objectives, keys,
  floor, wiki link). `[AUTO]` — quest taken from the game log automatically.
- Faction filter: **Any / BEAR / USEC**.
- **Browse…** — set the game `Logs` folder (enables auto-tracking and instant
  map detect in offline raids). **Scan hist** — re-scan recent log sessions.
- Tracked quests are drawn on the map in their own color (colored square).

## F3 Farm Helper

- **Categories** — what loot to include (tech / med / sale / weapons / expensive).
- **Points** — slider: max stops in the route (5–35).
- **Generate** — build a route from your position; **New from here** — rebuild
  without already visited points; **Reset** — turn farm off.
- **Exit** — cycle exit choice (auto = farthest); shown with distance.
- Route lines are straight (no navigation mesh in open data) — direction only.
- While farm is active the quest panel is locked (one mode at a time).

## F2 Prices

- Top items by price per inventory slot, tabs **PvP** / **PvE**.
- Data age is shown in the header. Update without rebuilding:
  `python prices\update_prices.py` (needs Python 3.8+, no extra libs).
  If tarkov.dev GraphQL is down it automatically falls back to static snapshots.

## Map data

- Sources: tarkov.dev snapshots (exits, quests, loot, hazards), map art by
  reemr/re3mr.com (Lighthouse) and TarkovTracker/tarkovdata (other maps),
  CC BY-NC-SA 4.0. Lighthouse art reflects the 1.1.5 rework.
- Data snapshot and patch version: see the first line of the FAQ panel (?).
- **Lighthouse is NOT verified in a live raid.** The map is still locked for the
  author by game progression, so its alignment, extracts, hazards and quests
  were checked only against embedded data and generated previews, not gameplay.
  Treat them as unverified until the map is unlocked and tested in-game.
- Files: `%AppData%\TarkyMappy\config.json` — settings;
  `tarkymappy.log` (next to the exe) — log and panic stack.

## Build from source

- Go 1.27+ is required.
- Run `build_ru.bat` or `build_en.bat` — the helper checks the tree, runs tests,
  builds `dist\TarkyMappy.exe`, and copies `prices\` and docs.
- Manual: `go build -trimpath -ldflags="-s -w -H=windowsgui" -o dist\TarkyMappy.exe .`
- Full instructions: [HOWTOUSE_EN.md](HOWTOUSE_EN.md) /
  [HOWTOUSE_RU.md](HOWTOUSE_RU.md).

## License

Personal, non-commercial use only. No selling, monetization, paid services, or
commercial benefit. Maps and data are CC BY-NC-SA 4.0. See
[LICENSE](LICENSE) and [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Known limitations

- West Customs and Reserve map rectangles overlap — without raid history
  auto-detect may pick the wrong one; use F10 / Manual pick (it remembers).
- Farm route lines ignore walls (straight lines).
- **Lighthouse was not verified in-game** because the author cannot access it
  yet due to game progression. Use the preview images as a reference only.
- Untranslated strings fall back to English in the RU interface.
