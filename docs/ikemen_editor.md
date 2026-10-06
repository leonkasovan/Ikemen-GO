# Ikemen-GO Built-in Editor Reference

Local-only web editor for motif / stage / character definitions.
Starts at boot with `-httpservice` or on demand from the title `Editor` menu (`external/script/main.lua` `main.f_editor`).
The `EDITOR → STAGE` entry runs the community Stage Viewer mode when its module
is installed (`external/mods/stageviewer/`), so the in-engine stage select is the
picker and the viewer match is the viewport: the stage it loads is the `.def`
the Stage view edits, and edits to it reach the running engine without a restart
(see *Live apply (stage .def)* below).
Listens on `127.0.0.1:6700`. Writes require the `X-Editor-Request` header
and are jailed to the game folder (`editorSandboxPath`). The built-in window lives
on Windows (`src/editor_webview_windows.go`, `github.com/jchv/go-webview2`,
profile in `save/editor-webview`) and other platforms fall back to the default
browser (`src/editor_webview_other.go`).Implemented in `src/editor_server.go`, `src/editor_webview_windows.go`,
`src/editor_webview_other.go`; the UI is embedded in the binary as
`editorPageHTML`. The Lua entry point is `main.f_editor` in
`external/script/main.lua` (`editorURL(view)` builds the URL with
`editorNormalizeView`, `openEditor(view)` starts the service on demand and opens
the window; `closeEditor()` destroys the window when one is open).

---

## API

- `GET /` — single-page UI (Motif / Stage / Character tabs).
- `GET /api/status` — version, port, motif, select.def, webview state, the two
  refresh endpoints, and the stage the engine has loaded (`stage`, `stageLoaded`:
  the `.def` path a stage save would reach, empty when none is loaded).
- `GET /api/motif` — motif broken down structurally: every section expanded
  with dotted keys from the `Motif` struct (`src/motif.go`, type + `default`
  tag), current value, defined / missing / unknown state; `<name>BGdef`
  layers and controllers expanded from `readBackGround` / `bgCtrl.read`
  (`src/stage.go`), marked `runtime`; `[Begin Action n]` and other keyless
  sections returned as `raw`.  In the UI, type and default are tooltips on the key name and the value control, not a column; a blank default means the parser leaves the zero value
alone. Every key table has a filter box above it that narrows the visible rows
  by the KEY column (case-insensitive substring, live as you type, "*" matches any run); the text is
  kept per table, so a save or reload that re-renders the section restores it. Sentinel defaults are spelled
  out: `zoomdelta` / `zoomscaledelta` / `xbottomzoomdelta` `(unset)`
  (`math.MaxFloat32` in `newBackGround`), `roundpos` `(stage default)`.
- `GET /api/stages`, `GET /api/characters` — select.def `[Characters]` /
  `[ExtraStages]` entries (params, `[Info]`), mirroring
  `external/script/main.lua` parsing.
- `GET /api/file?path=…` — any text file with an allowlisted extension
  (`.def .ini .txt .cfg .air .cmd .cns .st .json .lua .md`) in the game folder
  as a section tree through the same renderer; other extensions are refused
  (`415`) and oversized files are refused (`413`).
- `GET /api/reload-req` — same as `/api/reload` but the service also runs the
  interpreter bootstrap Lua; for external tooling only.
- `GET /api/sff?file=…&group=…&number=…` — one sprite as PNG (private
  `loadSffEx` decode with `keepPixels`, cached; the loaded SFF is kept in a
  small LRU cache keyed by path and validated by mtime / size).
- `POST /api/save` — `{path, section, key, value, remove, index}`; line-based
  edit preserving order, comments, indentation, and inline-comment spacing.
  `index` is the position of a `[BG ...]` layer section in the file (the
  position of the layer in the loaded stage's `s.bg`), `-1` for every other
  section: it is what lets a stage layer edit reach the right runtime layer.
- `POST /api/save-req` — same as `/api/save` but the service also runs the
  interpreter bootstrap Lua; for external tooling only.
- `GET /api/pins`, `POST /api/pins` — pinned keys. Every key table leads
  with a Pin column (◈ pinned, ◇ unpinned); pinned rows float to the top of
  their section, the rest keeps its order. Toggling posts
  `{path, section, key, pinned}` (needs the `X-Editor-Request` header) and
  stores one `path|section|key = 1` line per pin in `[Editor Pins]` of the
  engine config, so the marks survive a restart. The section is written with
  a line-based edit and the engine otherwise ignores it.
- `POST /api/reload` — reload the configured motif from disk into the running
  engine, so edits to reload-only keys show up without restarting the game.
  Runs the Lua `loadMotif()` global on the engine thread (re-parse, swap
  `sys.motif`, rebuilt table with the menu itemname overlays) and replaces the
  script's `motif` global with the result, which every menu screen reads every
  frame — replacing the Go struct alone would leave the script drawing the old
  table. A broken file fails the protected call and keeps the running motif.
  Requires the `X-Editor-Request` header like `/api/save`. Refused with
  `409` while a match runs, netplay / a replay is active, or assets are
  loading; `404` when no motif is configured. The Motif view has a
   `Reload motif in engine` button for it; a save that needs a reload runs it
   automatically, and only says "saved, reload the motif to apply" when the
   reload is refused. Besides swapping the motif table, the
  reload re-runs the builders for the two structures the script otherwise keeps
  from boot: the select screen's cell grid (`start.f_updateGrid`) and the menus
  (`main.f_rebuildMenus`: `main.menu`, the pause menus, the options menu, and
  the attract-vs-title group choice). See *Load-time Lua captures* below.
- `POST /api/reloadstage` — re-read the loaded stage's `.def` from disk into the
  running engine, for the stage keys that are only read when the stage loads
  (`[BGdef]`, `[Info]`, `[Music]`, `[BG ...]` controllers, `localcoord`, a
  removed key). Unlike the motif reload it **works during a match**: it sets
  `sys.reloadFlg` + `sys.reloadStageFlg` on the engine thread, which the match
  loop consumes by restarting the match (`winp = -2`), dropping `sys.stage` and
  letting `Loader.loadStage` parse the file again. No character slot is marked
  for reload, so the loader keeps the resident characters (`Same char kept`)
  rather than re-parsing their defs — the stage is what is rebuilt. Requires the
  `X-Editor-Request` header; refused with `409` during netplay / a replay or
  while assets are loading. The Stage view has a `Reload stage in engine` button
  for it; it is never run automatically by a save (a match restart behind the
  user's back would be hostile), so a structural stage save only reports "saved,
  reload the stage to apply".

Enumerated keys render as combo boxes (`trans`, `projection`, `textwrap`,
`banktype`, `space`, `savedata`, `scalemode`, `scalefilter`, font `type`,
BG layer `type`, BG controller `type`); unknown values kept as `(custom)`.

- Web view state from `editorWebViewState()`: `open:<title>`, `opening`,
  `ready`, `no-runtime`, `unsupported` (other platforms).

---

## File access limits

Every whole-file read the service performs is gated, so a request cannot pull
an arbitrary amount of data into memory, and every write lands atomically.

**Read caps.** The file size is `stat`ed before anything is read; a file over
the cap is refused with `413 Request Entity Too Large` instead of being loaded.
(`src/editor_server.go`)

- `editorMaxReadFile` (16 MiB) — INI / def / select.def / engine-config text
  reads: `/api/file`, `/api/stages`, `/api/characters`, `/api/save`, the pins
  store, and the internal motif / select.def / `[Info]` lookups.
- `editorMaxSFF` (256 MiB) — a sprite container opened by `/api/sff`.
- `editorMaxBody` (1 MiB) — a JSON request body (`editorReadJSONRequest`, via
  `io.LimitReader`).

The text helpers share one gate: `editorFileWithinCap` (existing file + size
check) and `editorReadableTextFile` (sandbox resolve + existence + extension
allowlist + cap). `editorFileReadStatus` maps their errors to the HTTP status:
`404` not found, `413` too large, `415` unsupported type, `400` otherwise.

**Extension allowlist.** `/api/file` and the text helpers only read files whose
extension is in `editorTextExtensions` — `.def .ini .txt .cfg .air .cmd .cns
.st .json .lua .md`. Anything else is refused with `415 Unsupported Media
Type`, so the endpoint cannot parse an arbitrary binary as INI. Engine-resolved
paths (the configured motif, select.def, `[Info]` defs, the engine config) are
not re-checked against the allowlist; they are only capped.

**Atomic writes.** `editorWriteFileAtomic` writes to a temporary file in the
same directory, restores the target's mode, then renames it over the target
(`os.Rename` replaces an existing file on every supported platform, Windows
included). A reader therefore never sees a half-written file, and a failed
write leaves the original untouched. `editorWriteTextFile` (used by `/api/save`)
and the pin editor (`/api/pins`) both go through it, preserving ordering,
comments, and line endings.

---

## Live apply (motif)

A Motif-view save writes the file first, then brings the in-memory motif in
line (`SetValueUpdate` / `updateINIFile`) so a later `Motif.Save` cannot
overwrite the edit, and — when possible — pushes the value into the running
engine without a restart. Posted to the engine thread and waited for
(`editorApplyTimeout` 2s). Response fields: `applied`, `needsReload`,
`needsRestart`, `applyReason` / `applyError`, plus `applyWarning` when the value
is live but part of it could not take effect (a font index with no `[Files]`
entry keeps the old typeface; reloading would not load it either).

Four modes, decided by walking the key to the struct that declares it
(`editorMotifApplyClassify`, `src/editor_server.go:2565`):

- **assigned** — no snapshot covers the field; drawing reads it directly.
  The screen is still re-applied (position pass, Lua table sync). A value can
  also be baked into a load-time Lua structure rather than drawn from the
  field: applying `[Select Info]` `rows` / `columns` / `cell.size` /
  `cell.spacing` / `cell.<c>-<r>` geometry — and `pos` / `showemptyboxes`,
  which are baked into the cached draw list built from that grid — also
  rebuilds the select screen's cell grid (`start.f_updateGrid`), which is
  otherwise only built at script load.
- **refreshed** — the field is snapshotted (`*TextSprite` / `*PalFX` /
  `*Rect` / `*Fade` via `PopulateDataPointers`), so the snapshots are
  refilled in place (`setTextSpriteInto` / `setFadeInto` / `setPalFxInto` /
  `setRectInto`), the position pass re-runs, and the Lua motif table key is
  rewritten (`editorSyncMotifLuaTable`). Replaces nothing: `main.lua` holds
  handles to these objects.
- **reload** — load-time only: background definitions, `localcoord`,
  `[Music]`, `[Files]` asset paths, user-named map sections, the keys the menu
  builders read (itemname labels, the menu title, `[Attract Mode] enabled`) —
   or an `*Anim`-only snapshot (element state a refill would not reset). A save
   to one of these reloads automatically; only when the reload is refused does
   it say "saved, reload the motif to apply". `POST /api/reload`
   (the `Reload motif in engine` button) then runs the Lua `loadMotif()` global
   on the engine thread, replaces the script's `motif` global with the rebuilt
    table, and re-runs the select-grid, menu and select-title builders
    (`editorRebuildAfterReload`) — without restarting the game. The title refresh
  matters because the rebuilt table carries a fresh, empty title `TextSprite`,
  whose text is only copied in from the mode keyed map when a mode is picked
  (`main.f_setSelectTitle`). Refused while a match runs, netplay / a replay is active, or assets are
  loading.
- **restart** — the value is read once while the script boots into a structure
  the reload cannot rebuild (`[Files] module`, which Lua `require`s once and
  caches; `[Files] select`, which the boot pass turns into the character /
  stage roster). The file is still written and the in-memory copy kept in line,
  but the save says "saved, restart the game to apply" (response field
  `needsRestart`); reloading would not help.

Measured on the bundled motif: 1205 assigned, 2427 refreshed, 5816
reload-only.

---

## Live apply (stage .def)

The same order as a motif save — the file is written first, unconditionally —
and then a second live apply runs when the saved file is the `.def` of the stage
the engine has loaded (`sys.stage`, compared case-insensitively after
`editorSandboxPath`, `editorStageApplySync`). Anything else is a plain write, so
a character `.def`, or a stage that is not the loaded one, changes nothing in
the engine beyond the file.

Two modes only (`editorStageKeyMode`), because a stage has no load-time
snapshots to refill:

| Mode | Meaning | Keys |
| --- | --- | --- |
| assigned | the field is read by the draw path every frame, and the engine itself assigns it at runtime (`modifyStageVar` / `modifyStageBG`) or while parsing (`loadStage`) | the tables below |
| reload | read when the stage loads | everything else: `[BGdef]`, `[Info]`, `[Music]`, `localcoord`, `[BG ...]` controllers and structural layer keys (`type`, `path`, `tile`, `positionlink`, …), any key removal |

Sections and keys applied live (`editorStageLiveKeys`, one case per entry in
`editorStageApplyValue`):

| Section | Keys |
| --- | --- |
| `[StageInfo]` | `zoffset`, `zoffsetlink`, `autoturn`, `resetbg`, `xscale`, `yscale` |
| `[Bound]` | `screenleft`, `screenright` |
| `[PlayerInfo]` | `leftbound`, `rightbound`, `topbound`, `botbound`, `p1startx`/`p1starty`/`p1startz`/`p1facing`, `p2startx`/`p2starty`/`p2startz`/`p2facing` |
| `[Camera]` | `boundleft`, `boundright`, `boundhigh`, `boundlow`, `floortension`, `tension`, `tensionhigh`, `tensionlow`, `cuthigh`, `cutlow`, `verticalfollow`, `tensionvel`, `startzoom`, `zoomin`, `zoomout`, `zoomindelay`, `zoominspeed`, `zoomoutspeed`, `yscrollspeed`, `autocenter`, `lowestcap` |
| `[Shadow]` / `[Reflection]` | `intensity`, `color` (+ `layerno` for the reflection), `xscale`, `yscale`, `xshear`, `angle`, `xangle`, `yangle`, `focallength`, `ydelta`, `fade.range`, `offset`, `window`, `projection` |
| `[BG ...]` | `start`, `delta`, `layerno`, `xshear`, `angle`, `xangle`, `yangle`, `focallength`, `projection`, `scalestart`, `scaledelta`, `velocity`, `mask`, `spriteno`, `actionno`, `trans`, `alpha` |

The assignment is the loader's own: `editorStageApplyValue` builds the
`IniSection` the parser would see (the trimmed value under the file's key) and
calls the same `ReadI32` / `ReadF32` / `ReadBool` / `readI32ForStage` /
`readF32ForStage` helpers `loadStage` and `readBackGround` call, so the running
stage ends up in the state a reload would produce. The consequences worth
knowing:

- **`[Bound]` and `[PlayerInfo]` values are stored raw.** `loadStage` reads
  them *after* the localcoord ratio has already scaled the 320-based defaults
  (`src/stage.go`), so a 640-wide stage keeps `screenleft = 20` as `20`, not
  `40`. Scaling them in the live apply would make the ground shift on the next
  reload. `TestEditorStageLiveApplyMatchesLoadStage` pins the equivalence.
- **`[StageInfo]` `xscale` / `yscale` are doubled on a hires stage**, like
  `loadStage` does for an explicit value.
- **Camera keys refresh the running camera** (`sys.cam.stageCamera =
  s.stageCamera; sys.cam.Reset()`), copied from `modifyStageVar`; that is what
  makes the edit visible on the next frame. `[Camera] tensionlow` also sets
  `ytensionenable`, mirroring `loadStage`.
- **`[PlayerInfo]` start / facing keys warn**: they are consumed when the next
  round spawns the characters, so the save answers `applyWarning` "the new start
  position applies from the next round".
- **`[Shadow] color` is forced black on a Mugen 1.1 stage**
  (`ikemenver == 0`, `mugenver == 1.1`), and both colours are clamped per
  channel, exactly as the loader reads them.
- **`[BG ...]` keys need the layer's index** (`index` in the save request,
  `bgIndex` on the tree node): the JS counts the sections whose header starts
  with the `bg` token, in file order, which is the order `loadStage` collects
  `s.bg` in. A layer the engine does not collect (`[BG0]`, no space) gets `-1`
  and its saves ask for a reload instead of touching the wrong layer.
- **`actionno`** swaps the animation (`bg.changeAnim`) and reports an error when
  the stage has no such action; `spriteno` and `alpha` warn when they do not
  apply to the layer's type / blend; `velocity` writes both `startv` (what
  `backGround.reset` restores) and `bga.vel` (what moves the layer this round).

A save to a reload-only stage key reports `needsReload` with the reason and does
**not** run anything: the `Reload stage in engine` button (`POST
/api/reloadstage`) restarts the match against the freshly parsed file. The
automatic reload of a motif save is gated on the saved file *being* the motif,
so a stage save never triggers it.

**Gates.** `editorStageBlocked` refuses netplay and replays (the stage is
rollback state — `state.go` clones `stageList` and the per-stage runtime state)
and a running asset load; a match is explicitly allowed, because editing the
stage during a match is the point.

**Title menu.** `EDITOR → STAGE` (`main.t_itemname['editorstage']` in
`external/script/main.lua`) runs `main.t_itemname.stageviewer()` — defined by the
community Stage Viewer module, autoloaded from `external/mods/**/*.lua` — when
it is installed, opens the editor on `?view=stage`, and then continues into the
mode's select screen, which is the in-engine stage picker. Without the module
the entry only opens the editor, as before. The Stage view's `Use stage loaded
in engine` action selects the `.def` `/api/status` reports (full path first,
file name as fallback), so the stage picked in the game is the one being
edited.

---

## Load-time Lua captures (live-apply blind spots)

The four modes above describe what a save can do to the Go motif and its
snapshots, and the reload also rebuilds the structures the script would
otherwise keep from boot (the select cell grid and the menus). This is what is
left once those builders run:

| Copy made in | Motif keys | Classified | What applies the edit |
| --- | --- | --- | --- |
| `main.group` / `main.background` (`main.lua`, top level) | `[Attract Mode] enabled` | reload | the reload re-runs `main.f_rebuildMenus`, which re-picks the attract vs title group |
| `main.menu` (`main.f_start`) | `[Title Info]` / `[Attract Mode]` `menu.itemname.*`, `menu.title.uppercase`, the section `title.text` | reload | the reload re-runs `main.f_start` through `main.f_rebuildMenus` |
| `options.menu` / `t_keyCfg` (`options.f_start`, top level) | `[Option Info]` `menu.itemname.*`, `keymenu.itemname.*`, `menu.title.uppercase`, `title.text` | reload | the reload re-runs `options.f_start` |
| `menu.t_menus` / `menu[id]` (`menu.f_start`) | `[<name> Pause Menu]` values | reload | the reload re-runs `menu.f_start`, which re-binds every entry to the new section tables |
| `t_modules` (`main.lua`, top level) | `[Files] module` | restart | a restart: the module is `require`d once and Lua caches it, so a reload cannot load it |
| `main.t_selGrid` / `main.t_selChars` (`main.lua`, top level) | `[Files] select` | restart | a restart: the boot pass turns select.def into the roster, and the reload only re-sizes the grid |

Why the rest are live: `editorSyncMotifLuaTable` rewrites the key inside the
very table the script holds, and the snapshot refill writes through the handles
the script was given, so anything the script reads **out of the motif table
every frame** is live. The rows above are values the script read out of the
motif once, into a copy it then draws; the reload re-runs the builders for the
menu and grid copies, and the two restart rows are the ones it does not rebuild.

`[Select Info]` `rows` / `columns` / `cell.size` / `cell.spacing` /
`cell.<c>-<r>` geometry stays classified **live**: the apply itself rebuilds the
grid (`start.f_updateGrid`), and a reload rebuilds it as well.

---

## `[Title Info]` — all 13 live

Every key applies (`applied=true, needsReload=false`):

| Key | Route |
| --- | --- |
| `fadein.time`, `fadeout.time` | refreshed `*Fade` |
| `menu.pos` | position pass |
| `menu.tween.factor` | Lua table |
| `menu.item.spacing` | Lua table |
| `menu.window.margins.y` | Lua table |
| `menu.window.visibleitems` | Lua table |
| `menu.boxcursor.visible` | Lua table (+ harmless `*Rect` refill) |
| `menu.boxcursor.tween.snap` | Lua table (+ harmless `*Rect` refill) |
| `menu.item.font`, `menu.item.active.font` | refreshed `*TextSprite` |
| `menu.item.layerno`, `menu.item.active.layerno` | refreshed `*TextSprite` |

`menu.boxcursor.*` classifies **refreshed** (`BoxCursorProperties` declares
`RectData *Rect`); the Lua table write is what changes behaviour.
`TestEditorApplyTitleInfoBlockLive` covers 9 keys; the 4
`menu.window.*` / `menu.boxcursor.*` keys were hand-verified on `deploy/`.

---

## `[Select Info]` — 121 / 282 live

Classified against `SelectInfoProperties` (`src/motif.go:659`).

**assigned (35):** `rows`, `columns`, `wrapping`, `pos`, `showemptyboxes`,
`moveoveremptyboxes`, `coopqueue`, `cell.size`, `cell.spacing`,
`cell.random.switchtime`, `p1`–`p4.cursor.startcell` / `tween.factor` /
`move.snd`, `p1`–`p4.cursor.done.snd`, `p1`–`p4.random.move.snd`,
`p2.cursor.blink`, `random.move.snd.cancel`, `stage.move.snd`,
`stage.done.snd`, `cancel.snd`, `p1/p2.name.spacing`.

The cell grid is its own case: `rows`, `columns`, `cell.size`, `cell.spacing`
and the `cell.<c>-<r>.offset` / `.spacing` / `.skip` overrides are assigned to
the struct, but the screen draws from a Lua grid (`start.t_grid`) built once at
script load, so the field alone would not move a cell. `pos` and
`showemptyboxes` are assigned too, but they are baked into the cached draw
list built from that grid, which is likewise only rebuilt on demand. Those
keys also rebuild that grid in the running script (`start.f_updateGrid`,
called by `editorRebuildSelectGrid` after the apply and by `POST /api/reload`),
which flags the draw list for rebuild, so a save shows on the next frame. A
lone `cell.spacing = 2` covers both axes (Mugen convention, as with the
per-cell overrides); without that it would parse as `[2, 0]`.

**refreshed (86):** `fadein.time`, `fadeout.time`, `title.offset` / `font` /
`layerno`, every `title.<mode>.text`, the ten `cell.*-N` override rows,
`p1/p2.name.offset` / `font` / `layerno`, `stage.font` /
`active.font` / `active2.font` / `done.font` / `layerno`.

The map-backed texts here (`title`, `record`, and elsewhere the menu items,
text input, `[Title Info] connecting`, `[Hiscore Info] title` and
`[Warning Info] text`) keep their text in a mode keyed map the script fills in
at runtime, so there is no single Go string for a refill to read.
`setTextSpriteInto` (`src/iniutils.go`) therefore keeps the sprite's current
`text` / `textInit` for them instead of assigning the empty default, which is
what lets `title.offset` / `title.font` refresh the sprite without blanking the
drawn title. Plain `TextProperties` (`[Option Info] title`, `[Replay Info]
title`) still re-read their Go string. Covered by
`TestEditorSelectInfoTitleRefreshKeepsText` and `TestEditorTextMapRefillKeepsText`.

Because the select title's text comes from the script's mode pick, the editor
also calls `main.f_refreshSelectTitle()` (`editorRebuildSelectTitle`) after any
`[Select Info] title.*` refresh, so an edited `title.text.<mode>` shows without
re-picking the mode (`main.f_setSelectTitle` records the current key). The
position pass is global and now idempotent for TextSprites too
(`TextSprite.offsetBase`), so editing one screen does not walk another's text
off screen (`TestEditorSaveDoesNotWalkOtherScreenTexts`).

**reload (161):** `cell.bg.*`, `cell.random.spr` / `scale`, `cell.slot.*`,
every `p1`–`p4.cursor.active.*` / `done.*` / `preview.*` visual (`anim`,
`spr`, `offset`, `facing`, `scale`, … — the plain `done.snd` stays live),
`stage.pos`, `stage.offset` / `scale` and the `active` / `active2` / `done`
offset / scale, every `p1/p2.face.*` (incl. `done` / `random` / `loading` /
`slot`), every `face2.*`, `portrait.*`, `stage.portrait.*` — `*Anim`-only
snapshots, except the stage geometry, which only reaches the screen through
a reload.

---

## `[Option Info]` — 106 / 114 live

Classified against `OptionInfoProperties` (`src/motif.go:1020`).

**assigned (14):** `menu.pos`, `menu.item.spacing`,
`menu.window.margins.y`, `menu.window.visibleitems`,
`menu.title.uppercase`, `cursor.move.snd`, `cursor.done.snd`, `cancel.snd`,
`keymenu.p1/p2.menuoffset`, `keymenu.pos`, `keymenu.item.spacing`,
`keymenu.window.margins.y`, `keymenu.window.visibleitems`.

**refreshed (92):** `fadein.*`, `fadeout.*`, `title.*`, every `menu.item.*`
(incl. `selected` / `value` / `info` + `active` variants),
`menu.boxcursor.*`, `menu.boxbg.*`, every `menu.valuename.*`,
`textinput.*` (incl. `overlay`), `keymenu.p1/p2.playerno.*`,
`keymenu.item.value/info.*.offset`, `keymenu.boxcursor.coords`, every
`keymenu.itemname.*`.

**reload (8):** `menu.arrow.up.*`, `menu.arrow.down.*` — `*Anim`-only.

---

## `[VS Screen]` — 49 / 139 live

Classified against `VsScreenProperties` (`src/motif.go:764`).

**assigned (31):** `time`, `p1/p2.num` / `spacing` / `padding`,
`p1/p2.name.num` / `spacing`, `orderselect.enabled`, every `pN.key`,
`done.key`, `skip.key`, `p1/p2.value.icon.spacing`, `p1/p2.value.snd`,
`stage.pos`, `timer.count` / `framespercount` / `displaytime`, `done.time`.

**refreshed (18):** `fadein.time`, `fadeout.time`, `match.*`,
`p1/p2.name.offset` / `font` / `layerno`, `stage.text` / `offset` / `font` /
`scale`, `timer.offset` / `font` / `scale` / `text`.

**reload (90):** `p1/p2.anim` / `offset` / `facing` / `scale` / `window` /
`applypal`, `p1/p2.done.anim`, every `pN.icon.*`, every
`pN.value.icon.*` / `value.empty.icon.*`, `stage.portrait.*` (incl. `bg`),
`loading.*` (incl. `done` / `wait`) — `*Anim`-only snapshots.

---

## Other sections

Same rule applies everywhere: plain fields assign, snapshotted text / fade /
rect / palfx refresh, `*Anim`-only snapshots, `localcoord`, `[Music]`,
`[Files]` assets, `*BGdef` blocks and user-named map sections
(`[Survival Results Screen]`, `[Pause Menu]`, …) reload. Keys the engine has
no field for report `applied=false` with a reason — a restart would not help
either.
