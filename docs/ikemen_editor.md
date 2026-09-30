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
  "saved, reload the motif to apply". Screens that captured their section in a
  Lua local at load (the pause-menu entries in `menu.t_menus`) keep drawing it
  until they are reopened.

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
`applyReason` / `applyError`, plus `applyWarning` when the value is live but
part of it could not take effect (a font index with no `[Files]` entry keeps
the old typeface; reloading would not load it either).

Three modes, decided by walking the key to the struct that declares it
(`editorMotifApplyClassify`, `src/editor_server.go:2452`):

- **assigned** — no snapshot covers the field; drawing reads it directly.
  The screen is still re-applied (position pass, Lua table sync).
- **refreshed** — the field is snapshotted (`*TextSprite` / `*PalFX` /
  `*Rect` / `*Fade` via `PopulateDataPointers`), so the snapshots are
  refilled in place (`setTextSpriteInto` / `setFadeInto` / `setPalFxInto` /
  `setRectInto`), the position pass re-runs, and the Lua motif table key is
  rewritten (`editorSyncMotifLuaTable`). Replaces nothing: `main.lua` holds
  handles to these objects.
- **reload** — load-time only: background definitions, `localcoord`,
  `[Music]`, `[Files]` asset paths, user-named map sections — or an
  `*Anim`-only snapshot (element state a refill would not reset). A save to
  one of these says "saved, reload the motif to apply"; `POST /api/reload`
  (the `Reload motif in engine` button) then runs the Lua `loadMotif()`
  global on the engine thread and replaces the script's `motif` global with
  the rebuilt table — without restarting the game. Refused while a match
  runs, netplay / a replay is active, or assets are loading.

Measured on the bundled motif: 1205 assigned, 2427 refreshed, 5816
reload-only.

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
