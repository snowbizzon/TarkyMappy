# TarkyMappy — complete user guide (EN)

TarkyMappy is a transparent, click-through minimap overlay for **Escape from
Tarkov** (Windows 10/11, Go + Ebitengine). It reads the file name of the newest
in-game screenshot, detects the map from the player coordinates, rotates the
map around you, and draws extracts, hazards, quests, and a farm route.

> **Important:** the **first in-raid screenshot** is required to start.
> Without it the overlay has no player position and cannot center the map.
> Press `PrtSc` once after spawning — that is enough.

---

## 1. Requirements

- Windows 10 or 11 (64-bit).
- Escape from Tarkov in **Borderless** or **Windowed** mode. Exclusive
  fullscreen may hide the transparent overlay.
- EFT screenshots folder:
  `Documents\Escape from Tarkov\Screenshots`
  (the app also checks the OneDrive-redirected
  `OneDrive\Documents\Escape from Tarkov\Screenshots`).
- Python 3.8+ only if you want to update flea prices (standard library only).
- Go 1.27+ only if you build from source.

No administrator rights are required.

---

## 2. Quick start

1. Unpack the release or the `dist` folder. `TarkyMappy.exe` and the `prices`
   folder must stay together.
2. Run `TarkyMappy.exe`.
3. Enter a raid and take **one screenshot** (`PrtSc`).
4. The overlay appears and centers on your position.
5. If your screenshots folder is non-standard: `F9` → **Browse…** → select the
   `Screenshots` folder → **Apply**.
6. Keep playing. Every new screenshot updates the position.

First launch creates:
`%AppData%\TarkyMappy\config.json`.
The log is written next to the exe: `tarkymappy.log`.

---

## 3. Hotkeys

Hotkeys are global and work without focusing the overlay window.

| Key | Action |
|---|---|
| `F2` | Flea market prices: top items by price per slot, PvP/PvE tabs |
| `F3` | Farm Helper: loot categories, route stops, exit selection |
| `F4` | Quest Helper: tracked quests, objectives, keys, wiki |
| `F5` | Zoom in |
| `F6` | Zoom out |
| `F7` | Hide / show overlay |
| `F8` | Quit |
| `F9` | Settings |
| `F10` | Next map (manual pick) |
| `PageUp` / `PageDown` | Floor up / down on multi-floor maps |
| `?` in F9 | FAQ panel |

While a panel is open (`F2`, `F3`, `F4`, `F9`, FAQ), mouse clicks reach the
window. When the panel is closed the window becomes click-through again.

---

## 4. First screenshot and map detection

EFT screenshot file names contain the player position and view quaternion, for
example:

```
2026-08-30[17-30]_388.36, 2.64, -114.48_0.00027, 0.98771, -0.00171, 0.15626_8.60 (0).png
```

TarkyMappy parses **only the file name**:

- `X, Y, Z` — player position;
- the quaternion — view direction (`RotY`);
- `X,Z` selects the map;
- `Y` selects the floor.

The PNG content is never read or analyzed.

### Why the first screenshot matters

Before the first screenshot there are no coordinates. The app may remember the
last map (`lastMap`) and show it, but the player marker appears only after the
first screenshot. Therefore:

1. Enter a raid.
2. Press `PrtSc`.
3. Wait for the overlay to update.

If screenshots do not appear, check EFT settings: the game must save
screenshots to the `Screenshots` folder. Third-party overlays (Steam, NVIDIA,
Discord) may capture `PrtSc` — disable their screenshot hotkey or use the
in-game binding.

### Auto-detect and manual pick

- Auto-detect uses the map rectangles in `data/maps.json`.
- Customs and Reserve rectangles overlap. Without raid history the app can pick
  the wrong one — use `F10` or manual pick in `F9`.
- `F10` cycles maps and stores the choice (`Manual pick`).
- To return to auto-detect, clear the manual selection in `F9`.
- If the EFT `Logs` folder is configured, the map can be detected from the game
  log before the first screenshot (useful for offline/PvE).

---

## 5. What the overlay shows

> **Lighthouse has NOT been verified in-game.** At publication time the
> Lighthouse map is still locked for the author because of game progression, so
> it could not be tested in a live raid. The Lighthouse previews and data come
> from the embedded data, not from gameplay. Treat Lighthouse alignment,
> extracts, hazards and quests as **unverified** until the map is unlocked and
> checked in a real raid.

- **White arrow in the center** — you. The map rotates so your view is up.
- **Extracts**:
  - green — Scav;
  - blue — PMC;
  - yellow — shared;
  - purple with `T|` — transit to another map.
- **Floors**: `^` / `v` before a name means the extract is on another floor;
  it is drawn dimmer.
- **Hazards**:
  - red outline/dot — minefield;
  - orange outline — sniper zone;
  - gold `+` — claymore/trap.
- **Quests (F4)**: colored zones and dots, labels in the quest color;
  diamond `K|` — a door opened by a key required for a tracked quest.
- **Farm (F3)**: gold route line, loot cluster circles, HUD with the current
  target and exit distance.
- Bottom-left: map name, floor, and `X,Y,Z`.

Extract and hazard toggles are in `F9`.

---

## 6. Settings (F9)

- **EFT screenshots folder** — watched folder. **Browse…** selects it,
  **Apply** saves and restarts the watcher, **Close** closes the panel.
- **PMC / Scav / Shrd** — which extract types to show.
- **Hazards** — mines, sniper zones, traps.
- **Manual pick** — choose the map manually; the map list appears below.
  Click the selected map again to clear the choice.
- **Quests (F4)** — open the quest panel. Disabled while Farm is active.
- **Farm (F3)** — open the farm panel.
- **Prices (F2)** — open the price panel.
- **Ignore floors** — show the flat combined map instead of per-floor views.
- **Lang: EN/RU** — interface language (saved).
- **debug: on/off** — verbose `tarkymappy.log`.
- Green **?** — FAQ; red **X** — quit; gear on the right — back to Settings
  from any panel; corner grip — resize; top bar — drag the window.

Default window size is 400×400, minimum 260×260. The window is always on top,
transparent, and click-through while no panel is open.

---

## 7. Quest Helper (F4)

1. Press `F4`.
2. By default only **tracked** quests are shown ("Mine").
3. **All (N)** shows every quest of the current map.
4. A checkbox on a row enables/disables tracking.
5. Click the row for hints: objectives, required keys, floor, wiki link.
6. Faction filter: **Any / BEAR / USEC**.
7. **Browse…** — EFT `Logs` folder. When configured, quests taken in game are
   added automatically (`[AUTO]`), and the map can be detected from the log.
8. **Scan hist** — one-time scan of recent log sessions: every "quest started"
   event from history becomes tracked.

A quest with points on several maps has a different color on each map. This is
intentional: the color is tied to the "map + quest" pair so two quests on the
same map cannot share a color.

Keys: when open data has no key spawn coordinates, the app draws a diamond on
the door opened by the key and labels it `K|key name`. A key on another floor is
dimmer and has an arrow.

---

## 8. Farm Helper (F3)

1. Press `F3`.
2. Select loot categories: tech, meds, sale, weapons, expensive tech.
3. **Points** slider — maximum route stops (5–35).
4. **Generate** — build a route from the current position.
5. **New from here** — rebuild while excluding already visited stops.
6. **Reset** — turn farm mode off.
7. The exit button cycles exit choice; `auto` picks the farthest available
   exit.

The route is drawn as straight lines: open data has no navmesh, so lines do not
avoid walls. Treat them as direction hints, not a turn-by-turn navigator.

While Farm is active, quest markers are hidden and the quest panel is locked —
only one mode at a time.

---

## 9. Prices (F2)

- Tabs **PvP** (`regular`) and **PvE** (`pve`).
- Top items by price per inventory slot.
- The header shows the snapshot date.
- Data lives in `prices\prices.json` next to the exe and is re-read every time
  `F2` is opened.
- Update without rebuilding:

```powershell
python prices\update_prices.py
```

The script uses the public `tarkov.dev` API; if GraphQL is unavailable it falls
back to static `json.tarkov.dev` snapshots. Internet is needed only while
updating.

If `prices\prices.json` is missing or broken, the app does not crash: the panel
shows a hint and the error goes to the log.

---

## 10. Logs, config, reset

- Config: `%AppData%\TarkyMappy\config.json`.
- Log: `tarkymappy.log` next to the exe.
- To reset settings, close the app and delete `config.json`.
- The EFT `Logs` folder is set in `F4` or `F9`; an empty folder means manual
  quest tracking.
- `debug: on` enables verbose logging; `debug: off` is the default.

---

## 11. Build from source

You need **Go 1.27+**. ImageMagick is needed only for map regeneration, Python
only for price updates.

### Option 1: interactive build helpers

- `build_ru.bat` — Russian interface;
- `build_en.bat` — English interface.

The helper checks the source tree and Go, offers to install Go with winget,
checks prices, runs `go test`, `go vet`, `gofmt`, builds
`dist\TarkyMappy.exe`, copies `prices\` and docs, and can create
`TarkyMappy-win64.zip`.

Non-interactive smoke run:

```powershell
build_en.bat /auto
build_ru.bat /auto
```

### Option 2: manual

```powershell
go mod download
go test ./...
go vet ./...
go build -trimpath -ldflags="-s -w -H=windowsgui" -o dist\TarkyMappy.exe .
```

Keep the `prices` folder next to the exe:

```
dist\
  TarkyMappy.exe
  prices\
    prices.json
    update_prices.py
```

The executable icon is embedded through `rsrc_windows_amd64.syso`;
`icon64.png` is required by `go:embed` for the window icon.

### Regenerate maps (dev-time)

```powershell
powershell -File tools\make_maps.ps1 -Force
```

If `magick.exe` is not in `PATH`:

```powershell
powershell -File tools\make_maps.ps1 -Force -Magick 'C:\Program Files\ImageMagick-7.1.2-Q16-HDRI\magick.exe'
```

### Update data (dev-time)

Download the snapshots listed in `UPDATING.md` into `%TEMP%`, then:

```powershell
powershell -File tools\update_data.ps1
go test ./...
```

---

## 12. Known limitations

- Customs and Reserve bounds overlap: without raid history auto-detect may pick
  the wrong map. Use `F10`, manual pick, or the EFT `Logs` folder.
- Farm routes are straight lines and ignore walls.
- Floors are selected by `Y`; unusual positions may need `PageUp` / `PageDown`.
- Some underground floors are nearly empty in the source SVG; the app draws the
  ground floor underneath at partial opacity.
- Some strings may remain in English when `data/loc_ru.json` has no translation
  — this is an intentional fallback.
- Data is a snapshot; see `data/meta.json`. After a game patch, update data
  using `UPDATING.md`.
- Lighthouse uses custom artwork by reemr/re3mr.com; it cannot be regenerated
  from SVG and must be replaced manually.
- **Lighthouse is not verified in a live raid.** It is locked for the author by
  game progression; only embedded data and preview renders were checked. After
  unlocking the map, manually verify the player position, extracts, hazards and
  quest points.

---

## 13. Troubleshooting

| Problem | Fix |
|---|---|
| Overlay is invisible | Press `F7`; use Borderless; make sure the window is not behind the game. |
| No player marker | Take the first in-raid screenshot; check the screenshots folder in `F9`. |
| Wrong map | `F10` or manual pick in `F9`; set the EFT `Logs` folder in `F4`. |
| Extracts are missing | Check PMC/Scav/Shrd toggles in `F9`; some extracts are on another floor. |
| F2 is empty | `prices\prices.json` is missing; run `python prices\update_prices.py`. |
| Quests do not update | Set the `Logs` folder in `F4`; press `Scan hist`; check the log. |
| Window cannot be moved | Drag the top bar; resize with the bottom-right grip. |
| Antivirus / SmartScreen | Unblock the exe in file properties, or build from source. |
| Reset settings | Delete `%AppData%\TarkyMappy\config.json` while the app is closed. |

---

## 14. Privacy and safety

- At runtime the app **does not use the network**, does not read game memory,
  does not inject into the game process, and does not intercept traffic.
- It reads **screenshot file names** and, if configured, **EFT text logs** —
  locally.
- Screenshot image content is never analyzed.
- Network access is used only by `prices\update_prices.py` when updating prices.
- Config and logs stay on your machine; there is no telemetry.

This is an unofficial project and is not affiliated with Battlestate Games.
Check the game rules and use it at your own risk.

---

## 15. Uninstall

1. Press `F8`.
2. Delete the folder containing `TarkyMappy.exe` and `prices`.
3. Optionally delete `%AppData%\TarkyMappy\config.json` and
   `tarkymappy.log`.

---

## 16. License

Personal **non-commercial** use only. You may not sell, resell, monetize,
include in paid services, charge for access, or derive commercial benefit from
it. See `LICENSE` for the full terms. Maps and data are CC BY-NC-SA 4.0; see
`THIRD_PARTY_NOTICES.md`.
