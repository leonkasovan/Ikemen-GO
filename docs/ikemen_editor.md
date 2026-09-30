# Ikemen-GO Built-in Editor Reference

Local-only web editor for motif / stage / character definitions.
Starts at boot with `-httpservice` or on demand from the title `Editor` menu.
Listens on `127.0.0.1:6700`. Writes require the `X-Editor-Request` header
and are jailed to the game folder (`editorSandboxPath`).

Implementation: `src/editor_server.go`, `src/editor_webview_windows.go`
(`github.com/jchv/go-webview2`, profile in `save/editor-webview`),
`external/script/main.lua` (`main.f_editor`), Lua globals `openEditor(view)`,
`editorURL(view)`, `closeEditor()`. Menu entries:
`menu.itemname.editor*` in `src/resources/defaultMotif.ini`
(`deploy/data/ikemen1`, `ikemen-480`; the mugen motif does not declare them).

---

## API

- `GET /` — single-page UI (Motif / Stage / Character tabs).
- `GET /api/status` — version, port, motif, select.def, webview state.
- `GET /api/motif` — motif broken down structurally: every section expanded
  with dotted keys from the `Motif` struct (`src/motif.go`, type + `default`
  tag), current value, defined / missing / unknown state; `<name>BGdef`
  layers and controllers expanded from `readBackGround` / `bgCtrl.read`
  (`src/stage.go`), marked `runtime`; `[Begin Action n]` and other keyless
  sections returned as `raw`. In the UI, type and default are tooltips on
  the key name and the value control, not a column; a blank default means
  the parser leaves the zero value alone. Sentinel defaults are spelled
  out: `zoomdelta` / `zoomscaledelta` / `xbottomzoomdelta` `(unset)`
  (`math.MaxFloat32` in `newBackGround`), `roundpos` `(stage default)`.
- `GET /api/stages`, `GET /api/characters` — select.def `[Characters]` /
  `[ExtraStages]` entries (params, `[Info]`), mirroring
  `external/script/main.lua` parsing.
- `GET /api/file?path=…` — any `.def` / `.ini` in the game folder as a
  section tree through the same renderer.
- `GET /api/sff?file=…&group=…&number=…` — one sprite as PNG (private
  `loadSffEx` decode with `keepPixels`, cached).
- `POST /api/save` — `{path, section, key, value, remove}`; line-based edit
  preserving order, comments, indentation, and inline-comment spacing.
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
  `Reload motif in engine` button for it; a save that needs a reload says
  "saved, reload the motif to apply". Besides swapping the motif table, the
  reload re-runs the builders for the two structures the script otherwise keeps
  from boot: the select screen's cell grid (`start.f_updateGrid`) and the menus
  (`main.f_rebuildMenus`: `main.menu`, the pause menus, the options menu, and
  the attract-vs-title group choice). See *Load-time Lua captures* below.

Enumerated keys render as combo boxes (`trans`, `projection`, `textwrap`,
`banktype`, `space`, `savedata`, `scalemode`, `scalefilter`, font `type`,
BG layer `type`, BG controller `type`); unknown values kept as `(custom)`.

---

## Live apply (motif only)

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
  `cell.spacing` / `cell.<c>-<r>` geometry also rebuilds the select screen's
  cell grid (`start.f_updateGrid`), which is otherwise only built at script
  load.
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
  to one of these says "saved, reload the motif to apply"; `POST /api/reload`
  (the `Reload motif in engine` button) then runs the Lua `loadMotif()` global
  on the engine thread, replaces the script's `motif` global with the rebuilt
  table, and re-runs the select-grid and menu builders — without restarting the
  game. Refused while a match runs, netplay / a replay is active, or assets are
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

## `[Select Info]` — 146 / 282 live

Classified against `SelectInfoProperties` (`src/motif.go:659`).

**assigned (32):** `rows`, `columns`, `wrapping`, `pos`, `showemptyboxes`,
`moveoveremptyboxes`, `coopqueue`, `cell.size`, `cell.spacing`,
`cell.random.switchtime`, `p1`–`p4.cursor.startcell` / `tween.factor` /
`move.snd`, `p1`–`p4.random.move.snd`, `p2.cursor.blink`,
`random.move.snd.cancel`, `stage.move.snd`, `stage.done.snd`, `cancel.snd`,
`p1/p2.name.spacing`, `stage.pos`.

The cell grid is its own case: `rows`, `columns`, `cell.size`, `cell.spacing`
and the `cell.<c>-<r>.offset` / `.spacing` / `.skip` overrides are assigned to
the struct, but the screen draws from a Lua grid (`start.t_grid`) built once at
script load, so the field alone would not move a cell. Those keys also rebuild
that grid in the running script (`start.f_updateGrid`, called by
`editorRebuildSelectGrid` after the apply and by `POST /api/reload`), so a save
shows on the next frame.

**refreshed (114):** `fadein.time`, `fadeout.time`, `title.offset` / `font` /
`layerno`, every `title.<mode>.text`, the ten `cell.*-N` override rows,
`p1`–`p4.cursor.active.anim` / `active.scale` / `done.spr` / `done.scale` /
`done.snd`, `p1/p2.name.offset` / `font` / `layerno`, `stage.font` /
`active.font` / `active2.font` / `done.font` / `layerno`.

**reload (136):** `cell.bg.*`, `cell.random.spr` / `scale`, `cell.slot.*`,
every `p1/p2.face.*` (incl. `done` / `random` / `loading` / `slot`), every
`face2.*`, `portrait.*`, `stage.portrait.*` — `*Anim`-only snapshots.

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
