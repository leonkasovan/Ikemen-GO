# CHANGES

## Features

### fix: a motif reload blanked the `[Select Info]` title text
`src/editor_server.go`, `src/editor_server_test.go`

`POST /api/reload` swaps in a freshly parsed motif table, so its select title
`TextSpriteData` is a new, empty sprite. The title text is copied into that
sprite only when a game mode is picked (`main.f_setSelectTitle`), so a reload
left the select title blank until the player picked a mode again — reported
after saving an unrelated reload-only key (`[Select Info]
p1.teammenu.item.cursor.offset`). The reload path now re-applies the current
mode's text as part of its rebuild step, alongside the menu and select-grid
rebuilds (`main.f_refreshSelectTitle`, the same call the live `title.*` refresh
uses). Covered by `TestEditorRebuildAfterReloadCallsLua`.

### feat: filter box on the editor key tables
`src/editor_server.go`

Each key table (Motif / Stage / Character) now has a filter box above it that
narrows the visible rows by the KEY column. Matching is a case-insensitive
substring, applied live as you type; only non-matching rows are hidden, so the
Save / Del targets and the input focus are untouched. The filter text is kept
per table, so a save or a reload that re-renders the section restores it.

### fix: editing `[Select Info]` `title.offset` / `title.font` blanked the title
`src/iniutils.go`, `src/editor_server_test.go`

`[Select Info]` `title` is a `TextMapProperties`: its text is a mode keyed map
(`title.text.arcade`, `title.text.versus`, ...) that the Lua script copies into
the title's `*TextSprite` when a mode is picked (`main.t_itemname`:
`textImgSetText`). The editor's live refresh rebuilds a screen's snapshots from
the struct, and `setTextSpriteInto` read `Text` only when it was a Go string.
For the map it read nothing and assigned the empty default back, so saving
`title.offset` or `title.font` blanked the drawn title until a mode was picked
again. The refill now leaves the sprite's `text` and `textInit` untouched when
the owning `Text` is not a string, while still applying the offset / font.
Covered by `TestEditorSelectInfoTitleRefreshKeepsText`.

An audit of the other `TextMapProperties` owners found the same path: `record`
(filled by `start.f_getRecordText` from `record.text[gameMode()]`), the menu
items (`ItemProperties`), the text input, `[Title Info] connecting`,
`[Hiscore Info] title` and `[Warning Info] text`. They are all covered by the
same guard. `[Option Info] title` and `[Replay Info] title` are plain
`TextProperties` (a Go string), so the refill still re-reads them, which
`TestEditorTextMapRefillKeepsText` checks alongside `record`.

### fix: saving a live motif key walked every screen's TextSprites off screen
`src/motif.go`, `src/font.go`, `src/iniutils.go`, `src/editor_server_test.go`

`applyPostParsePosAdjustments` is global: it shifts every screen's TextSprites
by their container offset (`Menu.Pos` and friends). `Anim.SetPos` had been made
idempotent (recomputed from the struct `Offset`), but the TextSprite half still
read `offsetInit` back and added the shift on top of itself. Since the editor
refills only the edited screen, every other screen's `offsetInit` already held
the shift, so saving `[Select Info] title.offset` moved the title menu item
texts from `(159, 158)` to `(318, 316)` on a 320x240 canvas — off screen, so the
main menu came up with no text. `setTextSpriteInto` now records the struct
declared offset in a new `TextSprite.offsetBase`, and the pass recomputes from
it, making the pass idempotent for text as well. Covered by
`TestEditorSaveDoesNotWalkOtherScreenTexts`.

### fix: an edited `[Select Info]` `title.text.<mode>` now shows without re-picking the mode
`external/script/main.lua`, `src/editor_server.go`, `src/editor_server_test.go`

The select title's text is copied out of the mode keyed map when the player
picks a mode (`main.t_itemname`). A live edit to `title.text.<mode>` synced the
Lua motif table but left the sprite drawing the boot copy, so the change only
appeared after re-entering the mode. The 16 mode pick sites now go through
`main.f_setSelectTitle(key)`, which also records the key, and
`main.f_refreshSelectTitle()` re-applies it. The editor calls it after any
`[Select Info] title.*` refresh (`editorRebuildSelectTitle`), so offset / font /
layerno and the mode text all reach the running select screen. Covered by
`TestEditorRebuildSelectTitleCallsLua` and `TestEditorSelectTitleQuery`.

### feat: built-in editor web service (`-httpservice`, port 6700) and Editor menu
`src/editor_server.go`, `src/main.go`, `src/script.go`,
`external/script/main.lua`, `src/resources/defaultMotif.ini`

New command line flag `-httpservice` starts a local-only HTTP service
(`127.0.0.1:6700`) that serves a single page editor plus a small JSON API:

- `GET /api/status` — engine version, port, configured motif and select.def.
- `GET /api/motif` — the motif in `[Config] Motif` broken down structurally:
  every section expanded with the dotted keys `src/motif.go` is able to assign
  (reflection over the `Motif` struct, including type and `default` tag), the
  value each key currently has, and keys the engine does not recognize.
  The background definitions the engine parses at load time instead of through
  the struct — the layers of a `<name>BGdef` block (`[TitleBG Background Sky]`,
  `[VersusBG 5]`, …) and the controllers (`[TitleBGctrl …]`, `[TitleBGctrldef]`,
  …) — are expanded the same way, from `readBackGround` and `bgCtrl.read` in
  `src/stage.go`, so they get a type, a default and the missing/unknown states
  too. They are marked `runtime` in the payload.
  `tree` groups the same sections the way the `Motif` struct declares them
  (Title, Select, Versus, Continue, Victory, …) — inside a group the order of
  the file is kept — nests the layers of every `<name>BGdef` under their
  background definition and hangs each `[Begin Action n]` under the layer that
  plays it (`actionno = n`). Sections whose body is not `key = value`
  (`[Begin Action n]`, `[Infobox Text]`, …) have no keys to show, so their whole
  content is returned in `raw` and displayed as is.
- `GET /api/stages`, `GET /api/characters` — select.def `[Characters]` /
  `[ExtraStages]` entries (stage list, params, `[Info]`) mirroring the parsing
  in `external/script/main.lua`. The Stage and Character views both pick a file
  from a combo box (name and .def, with missing ones flagged) and show it as a
  section tree built like the motif one, through the same renderer, so they
  cannot drift apart: in a stage a background block (`[BGdef]`, `[BG n]`,
  `[bgctrldef]`, `[bgctrl]`) owns its layers and controllers, and each
  `[Begin Action n]` hangs under the layer that plays it. Neither view has a
  schema behind its keys, so their key tables drop the *State* column, which
  the Motif view keeps. Type and default are tooltips on the key name and the
  value control rather than a column, in every view.
- `GET /api/file?path=…`, `POST /api/save` — view and edit any `.def`/`.ini`
  file inside the game folder. Writes are line based, so ordering, comments and
  indentation are preserved.
- `POST /api/reload` — reload the configured motif from disk into the running
  engine (runs the Lua `loadMotif()` global on the engine thread and replaces
  the script's `motif` global with the rebuilt table, which every menu reads
  every frame — swapping the Go struct alone would leave the old visuals on
  screen; a broken file fails the protected call and keeps the running motif),
  so reload-only keys no longer need a restart. Requires `X-Editor-Request`;
  refused with 409 mid-match / netplay / replay / asset load, 404 with no
  motif. Motif view `Reload motif in engine` button; needs-reload saves say
  "saved, reload the motif to apply". Applied saves also report `applyWarning`
  when a font index has no `[Files]` entry and keeps the old typeface.
- Enumerated keys are rendered as a combo box instead of a text field. The value
  sets come from the engine parsers: `trans` (`default`, `none`, `add`, `add1`,
  `addalpha`, `sub`, `subadd`), `projection` (`orthographic`, `perspective`,
  `perspective2`), `textwrap` (`w`, `1`, `none`), `banktype` (`palette`,
  `sprite`), `space` (`stage`, `screen`), `savedata` (`map`, `var`, `fvar`) and
  the three overloaded `type` keys — background layers (`normal`, `anim`,
  `parallax`, `video`, `dummy`), background controllers (`anim`, `visible`,
  `enable`, `null`, `palfx`, `posset`, `posadd`, `remappal`, `sinx`, `siny`,
  `velset`, `veladd`) and fonts (`truetype`, `bitmap`). A value the engine does
  not know is kept and shown as `(custom)`.
- The *State* column of a key is an icon — `●` defined (the file sets it),
  `○` missing (the engine knows it and uses its `default` tag) or `⚠`
  unknown (in the file but not in the editor's model), with the state name as
  a tooltip. `unknown` only ever meant "not in my model", never
  "the engine ignores it", so the three parsers that are not driven by `ini`
  tags are modelled as well: `[Music]` (the `splitMusicKey` key splitter), the
  map fields whose section name comes from the file (`ResultsScreen`,
  `PauseMenu`, …) and the background layers of a `<name>BGdef`, which the
  engine reads at load time and which are therefore listed as they are.
- `GET /api/sff?file=chars/kfm/kfm.sff&group=9000&number=0` — one sprite of a
  `.sff` as a PNG, so a page can show it in a plain `<img>`. 8 bit sprites are
  paletted and go through the sprite's palette, 24 and 32 bit sprites carry
  their own colors. The editor decodes the file privately and keeps the pixels
  on the CPU (`loadSffEx` with `keepPixels`), because a borrowed or already
  uploaded `Sff` no longer holds any; the result is cached, so paging through
  sprites does not re-read the file. The Stage and Character views each put a
  sprite preview at the top of their right-hand panel, defaulting to the `.sff`
  next to the character / stage `.def` being edited.

New `Editor` submenu in the title menu (Motif / Stage / Character) is defined by
`menu.itemname.editor*` entries in the default motif
(`src/resources/defaultMotif.ini`, shipped in `deploy/data/ikemen1` /
`ikemen-480`), so screenpacks based on it get it without changes. The mugen
motif (`src/resources/defaultMugenMotif.ini`, `deploy/data/mugen1`) does not
declare it. Selecting an item starts the service on demand and shows it
in a **built in WebView2 window** on Windows
(`github.com/jchv/go-webview2`, pure Go, no cgo, embedded `WebView2Loader.dll`),
with the profile kept in `save/editor-webview`. The window lives on its own
locked OS thread, reuses itself when another view is picked, and the browser is
only used as a fallback (other platforms, or when the WebView2 runtime is
missing). Lua globals: `openEditor(view)`, `editorURL(view)` and
`closeEditor()`. `openEditor()` returns as soon as the editor has been asked to
  open: the window is created on its own thread, and waiting for it used to block
  the engine thread for up to 20s on a cold start, freezing the game while the
  menu item was picked. Its return value is therefore "the editor was asked to
  open", and only false when the service could not start at all. A second request
  while a cold start is in flight joins that one instead of opening a competing
  window, and the window size is read on the engine thread before the goroutine
  starts, since the game window it follows can change on a resolution switch.
  The service is bound to loopback, writes require the
`X-Editor-Request` header (blocks cross-site writes) and reject any path outside
the game folder.
- A **Save in the Motif view is applied to the running engine** wherever that is
  possible, so an edit shows up without a restart. The file on disk is written
  first (unchanged behaviour), then the in-memory motif is brought back in line
  with `SetValueUpdate` / `updateINIFile`, so a later `Motif.Save` cannot write
  the old value back over the edit — that in-memory copy used to drift silently
  after every save. The apply is posted to the engine thread and waited for,
  since the game loop reads the motif every frame.
  Most keys **cannot** be applied by assignment alone, because the motif is a
  declaration that the loader compiles into live objects:
  `PopulateDataPointers` fills a `*TextSprite` / `*Anim` / `*PalFX` / `*Rect` /
  `*Fade` from the owning struct, and only while that pointer is nil, so drawing
  reads the snapshot rather than the field. `TextProperties` (every `font`,
  `textwrap`, `offset`, `scale`, …) and `AnimationProperties` are snapshotted
  whole, which is most of the motif. So a save falls into one of three modes,
  decided by walking the key to the struct that *declares* the field rather than
  from a hand-kept list:
  - **assigned** — nothing snapshots the field, so drawing reads it directly.
    The screen is still re-applied, because a plain field can be consumed by the
    load time passes rather than drawn: `menu.pos` is never drawn, the position
    pass adds it to the position of the already built `TextSprites`.
  - **refreshed** — the field is snapshotted, so assigning it is not enough on
    its own. Every applied key re-applies its screen: the snapshots are refilled
    in place (`setTextSpriteInto` / `setFadeInto` / `setPalFxInto` /
    `setRectInto`, split out of `SetTextSprite` / `SetFade` / `SetPalFx` /
    `SetRect`), then `applyPostParsePosAdjustments` runs again, then the Lua
    motif table is updated. Only the screen the key lives under is touched
    (the top level Motif field).
    Refilling rather than rebuilding is the whole trick, and it covers every
    snapshot type. The Lua system script is handed a *handle* to each of these
    objects — `toLValue` turns `*TextSprite` / `*Fade` / `*PalFX` / `*Rect`
    into userdata wrapping the Go pointer, which `menu.lua` draws and mutates
    through (`textImgSetWindow` on `sec.menu.item.active.TextSpriteData`,
    `rectSetWindow` on `sec.boxcursor.RectData`) — and a handle is only valid
    for the object it was given, so replacing the object would leave the script
    reading the load time values. `SetPos` also writes `offsetInit`, so running
    the position pass on its own would add `Menu.Pos` to an `offsetInit` that
    already contains it and walk the sprite further on every save; the refill
    restores the pristine offset from the struct first, exactly as the loader
    does it.
    A snapshot that is *only* an `*Anim` (`AnimationProperties`, `GlyphProperties`)
    still needs a reload: an `*Anim` carries element state that a refill would
    not reset. A struct that declares one alongside a refilled snapshot
    (`FadeProperties`, which has both `AnimData` and `FadeData`) is fine, because
    `setFadeInto` reads the anim along the way.
    The script also holds a *plain* copy of the motif table and reads values out
    of it directly — which assigning the Go field alone does not reach.
    `editorSyncMotifLuaTable` rewrites that one key of the cached table on the
    engine thread, replacing only a key the table already has and running no
    Lua code.
  - **reload** — the key is only read at load time (background definitions,
    `localcoord`, `[Music]`, the `[Files]` asset paths) or sits under a user
    named section (`[Survival Results Screen]`, `[Pause Menu]`), which has no
    single field to scope a rebuild to. `POST /api/reload` (`Reload motif in
    engine` button) runs the Lua `loadMotif()` global and swaps the script's
    `motif` global for these, so no restart is needed; refused while a match
    runs.
  Keys the engine has no field for, and keys whose screen cannot be rebuilt, are
  reported as not applied rather than as needing a restart, since a restart would
  not help either.  Measured on the bundled motif: 1205 keys assigned directly,
  2427 assigned and refreshed, 5816 reload-only.

  #### How each `[Title Info]` key is applied

  Every key of a whole `[Title Info]` block applies live, but not by one
  mechanism — each one needs a different one, which is why the block is a decent
  test of the classification. Nine of the thirteen below are covered by the
  automated tests (`TestEditorApplyTitleInfoBlockLive` /
  `TestEditorApplyTitleInfoBlockReachesIni` in `src/editor_server_test.go`); the
  remaining four (`menu.window.margins.y`, `menu.window.visibleitems`,
  `menu.boxcursor.visible`, `menu.boxcursor.tween.snap`) were verified by hand
  against `deploy/` with the debug build (`-httpservice`, `POST /api/save`).
  Hand-verified: all thirteen return `applied=true, needsReload=false`, and
  saving each one twice produces no second diff and no drift (the double-save
  assertion is automated only for `menu.pos`).

  | Key | Route | Why that one is needed |
  | --- | --- | --- |
  | `fadein.time`, `fadeout.time` | refilled `*Fade` | `FadeProperties.FadeData` is a snapshot, and `main.lua` keeps the handle it was given to pass to `fadeInInit` / `fadeOutInit`. |
  | `menu.pos` | position pass | Never drawn. `applyPostParsePosAdjustments` adds it to the already built `TextSprite`s, so only re-running that pass moves the menu. |
  | `menu.tween.factor` | Lua table | Read as `m.tween.factor` in `main.menuUpdate`; the Go field is not consulted. |
  | `menu.item.spacing` | Lua table | Read as `m.item.spacing[2]` in `main.f_menuWindow` and the scroll code. |
  | `menu.window.margins.y` | Lua table | Read as `t.window.margins.y[1]` / `[2]` in `main.f_menuWindow`. |
  | `menu.window.visibleitems` | Lua table | Read as `m.window.visibleitems` to size the window, and as `motif[main.group].menu.window.visibleitems` in the cursor move. `0` means "all items". |
  | `menu.boxcursor.visible` | Lua table | Read as `m.boxcursor.visible` to decide whether `rectUpdate(m.boxcursor.RectData)` runs at all, so `0` hides the cursor. |
  | `menu.boxcursor.tween.snap` | Lua table | Copied to `sec.boxCursorData.snap` each frame, where it decides whether the cursor jumps to the row or tweens. |
  | `menu.item.font`, `menu.item.active.font` | refilled `*TextSprite` | `TextProperties.TextSpriteData` is the snapshot `menu.lua` draws with `textImgDraw(sec.movelist.title.TextSpriteData)` and friends; `font[0]` is the font, `font[1]` the bank, `font[3..5]` the active colour. |
  | `menu.item.layerno`, `menu.item.active.layerno` | refilled `*TextSprite` | Same snapshot: `layerno` is read off the `TextSprite`, not off the field. |

  `menu.boxcursor.*` deserves a note: `BoxCursorProperties` also declares
  `RectData *Rect`, so these keys are classified **refreshed** and the cursor
  rect is refilled as well. The refill is harmless there and the Lua table write
  is what actually changes the behaviour — `rectSetWindow` / `rectUpdate` run
  every frame from the table, so the edit shows on the next frame either way.

  #### Which `[Select Info]` keys apply live

  Classified the same way (`editorMotifApplyClassify` against
  `SelectInfoProperties` in `src/motif.go`): of the 282 keys of a full
  `[Select Info]` block, 146 apply live — 32 assigned directly, 114 assigned
  and refreshed — and 136 need a reload.

  - **assigned** — grid behaviour and plain values: `rows`, `columns`,
    `wrapping`, `pos`, `showemptyboxes`, `moveoveremptyboxes`, `coopqueue`,
    `cell.size`, `cell.spacing`, `cell.random.switchtime`,
    `p1`-`p4.cursor.startcell` / `tween.factor` / `move.snd`,
    `p1`-`p4.random.move.snd`, `p2.cursor.blink`, `random.move.snd.cancel`,
    `stage.move.snd`, `stage.done.snd`, `cancel.snd`, `p1/p2.name.spacing`,
    `stage.pos`.
  - **refreshed** — snapshotted text and fades, refilled in place:
    `fadein.time`, `fadeout.time`, `p1`-`p4.cursor.active.*` /
    `done.spr` / `done.scale` / `done.snd`, `title.offset` / `font` /
    `layerno`, every `title.<mode>.text`, the ten `cell.*-N` override rows,
    `p1/p2.name.offset` / `font` / `layerno`, `stage.font` / `active.font` /
    `active2.font` / `done.font` / `layerno`.
  - **reload** — `*Anim`-only snapshots, which carry element state a refill
    would not reset: `cell.bg.*`, `cell.random.spr` / `scale`, `cell.slot.*`,
    every `p1/p2.face.*` (including `done` / `random` / `loading` / `slot`),
    every `face2.*`, `portrait.*` and `stage.portrait.*`.

  #### Which `[Option Info]` keys apply live

  Classified the same way (`editorMotifApplyClassify` against
  `OptionInfoProperties` in `src/motif.go`): of the 114 keys of a full
  `[Option Info]` block, 106 apply live — 14 assigned directly, 92 assigned
  and refreshed — and 8 need a reload.

  - **assigned** — plain values: `menu.pos`, `menu.item.spacing`,
    `menu.window.margins.y`, `menu.window.visibleitems`,
    `menu.title.uppercase`, `cursor.move.snd`, `cursor.done.snd`,
    `cancel.snd`, `keymenu.p1/p2.menuoffset`, `keymenu.pos`,
    `keymenu.item.spacing`, `keymenu.window.margins.y`,
    `keymenu.window.visibleitems`.
  - **refreshed** — snapshotted text, fades, rects and overlays, refilled in
    place: `fadein.*`, `fadeout.*`, `title.*`, every
    `menu.item.*` (including `selected` / `value` / `info` and their `active`
    variants), `menu.boxcursor.*`, `menu.boxbg.*`, every
    `menu.valuename.*`, `textinput.*` (including the `overlay`),
    `keymenu.p1/p2.playerno.*`, `keymenu.item.value/info.*.offset`,
    `keymenu.boxcursor.coords`, every `keymenu.itemname.*`.
  - **reload** — `*Anim`-only snapshots: `menu.arrow.up.*` and
    `menu.arrow.down.*`.

  #### Which `[VS Screen]` keys apply live

  Classified the same way (`editorMotifApplyClassify` against
  `VsScreenProperties` in `src/motif.go`): of the 139 keys of a full
  `[VS Screen]` block, 49 apply live — 31 assigned directly, 18 assigned
  and refreshed — and 90 need a reload.

  - **assigned** — plain values: `time`, `p1/p2.num` / `spacing` / `padding`,
    `p1/p2.name.num` / `spacing`, `orderselect.enabled`, every `pN.key`,
    `done.key`, `skip.key`, `p1/p2.value.icon.spacing`, `p1/p2.value.snd`,
    `stage.pos`, `timer.count` / `framespercount` / `displaytime`,
    `done.time`.
  - **refreshed** — snapshotted text and fades, refilled in place:
    `fadein.time`, `fadeout.time`, `match.*`, `p1/p2.name.offset` / `font` /
    `layerno`, `stage.text` / `offset` / `font` / `scale`, `timer.offset` /
    `font` / `scale` / `text`.
  - **reload** — `*Anim`-only snapshots: `p1/p2.anim` / `offset` / `facing` /
    `scale` / `window` / `applypal`, `p1/p2.done.anim`, every `pN.icon.*`,
    every `pN.value.icon.*` / `value.empty.icon.*`, `stage.portrait.*`
    (including `bg`), `loading.*` (including `done` / `wait`).

  A no-op save now also leaves the file byte for byte identical. The engine's
  own motif aligns its inline comments with runs of spaces
  (`menu.boxcursor.visible = 0         ;Set to 1 …`), and the save collapsed
  those to a single space, so saving an unchanged value produced a diff in
  every aligned line. The whitespace in front of a `;` is now preserved, and
  only the value is replaced.

### fix: editor edits to `[Select Info]` rows / columns did not move the select grid
`src/editor_server.go`, `external/script/start.lua`

The select screen's cell grid (`start.t_grid`, built from the `main.t_selGrid` /
`main.t_selChars` cells) is assembled once when the script loads, from
`motif.select_info.rows * motif.select_info.columns`, the cell size / spacing and
the per-cell `offset` / `spacing` / `skip` overrides. The live apply treated
these keys as plain assignments, so the struct field and the Lua motif table
changed while the grid the draw loop walks kept its boot dimensions — a saved
`rows` / `columns` edit therefore changed nothing on screen.

- `external/script/start.lua`: the grid build moved into `start.f_updateGrid()`
  (still called once at load), which grows `main.t_selGrid` to the current cell
  count, rebuilds `start.t_grid`, re-maps each character's row / column, pulls a
  now out-of-range cursor back inside and flags the draw list for rebuild.
  `start.f_selectReset` now walks only the visible cells, so a grid shrunk
  after an edit cannot index past `start.t_grid`.
- `src/editor_server.go`: a save that touches `rows`, `columns`, `cell.size`,
  `cell.spacing` or a `cell.<c>-<r>.offset` / `.spacing` / `.skip` override
  (`editorSelectGridQuery`) asks the script to rebuild the grid
  (`editorRebuildSelectGrid`) after the value is applied, and `POST /api/reload`
  rebuilds it as well. The keys stay classified as assigned (live).

### fix: saving a live motif key walked every `*Anim` further across the screen
`src/motif.go` — `applyPostParsePosAdjustments`

`Anim.SetPos` overwrites `offsetInit`, and the position pass derived the shift
from `offsetInit` itself (`a.SetPos(a.offsetInit[0]+dx, …)`), so the first run
— at boot — already stored the shifted position as the base, and every
re-apply added the shift on top of itself again. Saving `[Select Info]` `rows`
therefore moved `p1.teammenu.item.cursor.anim` (and every other Anim the pass
touches: menu arrows, teammenu titles/icons, face slots, portraits), one shift
per save, and the draw loop (`main.f_animPosDraw`, which resets to `offsetInit`
every frame) amplified it. The pass now recomputes each Anim from its
struct-declared `Offset` plus the container shift, making re-applying
idempotent; `offsetInit` keeps its shifted meaning for the draw/reset paths.
Covered by `TestEditorSelectRowsSaveKeepsCursorPos`
(`src/editor_server_test.go`, run with `make test-editor`).

### fix: editor saves to `[Select Info]` pos / showemptyboxes never reached the screen
`src/editor_server.go`, `external/script/start.lua`

The select screen draws from a cached draw list (`staticDrawList`) whose items
bake in `motif.select_info.pos` and the grid cells at build time; the list is
rebuilt only when `start.needUpdateDrawList` is set. Saving `pos` synced the
Lua table but set no flag, so the old positions kept drawing — and the same
applied to `showemptyboxes`, which is only read while that list is built. Both
keys now trigger the grid refresh (`start.f_updateGrid`, which flags the list),
like rows / columns / the cell geometry already did. Covered by the updated
`TestEditorSelectGridQuery` and `TestEditorRebuildSelectGridCallsLua`.

A single `cell.spacing = 2` additionally never looked like a change: the
generic array parser turns it into `[2, 0]`, so only the row pitch moved by
2px. `getCellSpacing` now mirrors a lone x into y (`{2, 0}` → `{2, 2}`),
matching the per-cell override right above it and the Mugen convention the
default motif documents ("spacing accepts only x value which is used for both
coordinates"). Applies at load and live alike, since both go through it.

### fix: editor motif reload rebuilds the menus; boot-only keys report "restart needed"
`src/editor_server.go`, `external/script/main.lua`, `external/script/menu.lua`,
`external/script/options.lua`

The motif reload only swapped the motif table and refilled snapshots, so keys
the system scripts read once at load kept drawing the boot copy. The menus are
the biggest case: `main.f_start`, `menu.f_start` and `options.f_start` copy the
itemname labels and the menu title out of `motif` at boot.

- `main.f_rebuildMenus()` (new) recomputes `main.group` / `main.background` from
  `[Attract Mode] enabled` and re-runs `main.f_start`, `menu.f_start` and
  `options.f_start`. `POST /api/reload` calls it after swapping the motif table,
  so itemname, attract-mode, menu-title and pause-menu edits apply without a
  restart. `menu.f_start` was made re-entrant (it re-binds the built-in
  `[Pause Menu]` entry and rebuilds `menu.t_vardisplayPointers` instead of
  appending); `options.f_start` resets `options.t_vardisplayPointers` for the
  same reason.
- `editorMotifApplyClassify` now reports those keys as **reload** instead of a
  live/refresh apply the menu tables would not reflect: `[Attract Mode]
  enabled`, `menu.itemname.*` / `keymenu.itemname.*`, `menu.title.uppercase`,
  and the `title_info` / `option_info` / `attract_mode` `title.text` drawn as
  the menu title.
- A fourth apply mode, **restart**, reports values the reload cannot rebuild:
  `[Files] module` (Lua `require` caches it) and `[Files] select` (the boot pass
  builds the character / stage roster from it). The save response adds
  `needsRestart` and the UI says "saved, restart the game to apply" instead of
  implying a reload would work.

### feat: static libvpx build for WebM alpha (VP8/VP9)
`Makefile`

New `libvpx` target (`make libvpx`) that downloads libvpx v1.15.2 and builds it
**decoder-only** — VP8/VP9 decoders enabled, encoders/tools/examples/docs/
webm-io disabled, `--enable-pic`, static — ~1.5 MB, installed under
`build/prefix/lib` alongside FFmpeg.

- FFmpeg configure now uses `--enable-libvpx` with the `libvpx_vp8`/`libvpx_vp9`
  decoders (replacing the native `vp8`/`vp9`) so WebM videos carrying a second
  VP8/VP9 alpha payload decode correctly.
- `$(FFMPEG_LIBS)` is an order-only dependency of `$(LIBVPX_LIB)`; `release`
  builds libvpx before FFmpeg, and `ffmpeg` alone builds both.
- `--target=` wired for win64/win32/darwin (amd64/arm64)/linux (amd64/arm64),
  `generic-gnu` fallback otherwise.
- FFmpeg's nasm invocation is wrapped to silence the deprecated `$`-hex warning
  from FFmpeg n7.1 (`yuv2yuvX.asm`) under nasm ≥ 2.16.

### refactor: remove eager large-sprite upload from SFF load
`src/image.go` — `loadSff`, `src/video_ffmpeg.go`

Removed the unconditional eager GPU upload of sprites >128 KB staged during
`loadSff` (batched uploads on the main thread / one-task-per-frame queue on the
loader path). Large sprites now upload lazily via `ensureTex()` on first render
like every other sprite.

- Faster SFF loads and no GPU-memory spike from never-drawn sprites; the
  tradeoff is larger sprites keep their pixel data in CPU `pendingData` until
  first drawn.
- `[Debug] EagerSpriteTextures = 1` still forces eager upload at
  `SetPxl`/`SetRaw` for A/B benchmarking.
- Also removed the `reisen.SetLogLevel(reisen.LogLevelWarning)` init — the
  vendored library's default log level is now used.

## Performance

### fix: RenderScale scissor clipping (GLES32)
`src/render_gles32.go` — `EnableScissor`

Scissor rects are computed in scrrect (game render target) space, but the GL
viewport is `renderW × renderH` when `RenderScale < 1`. The unscaled scissor
landed at `1/RenderScale` too far right/down, clipping the left portion of
every scissored draw (lifebar fills: "green bar only 50–100%" symptom).

- Scale x/y/width/height by `renderW/scrrect[2]`, `renderH/scrrect[3]` when
  they differ.
- Flip Y against `renderH` instead of `scrrect[3]`.
- Only affects the GLES32 renderer (gl33 has no RenderScale viewport scaling).

### perf: texture bind cache for instanced batches (GLES32)
`src/render_gles32.go` — `boundTexUnits`

Per-flush cache of texture handles per texture unit. `renderBatch` skips
redundant `glActiveTexture`/`glBindTexture` when the unit already holds the
handle (consecutive batches heavily reuse sprites/palettes).

Measured: flush 5.9ms → 4.5ms, FPS 40 → ~43.

### perf: hoist static batch state to flush level (GLES32)
`src/render_gles32.go` — `flushSpriteBatches`

Instanced pipeline (program/VAO), projection matrix, and `texArray`/`palArray`
uniforms set once per flush instead of once per batch.

### perf: audio normalizer pow removal
`src/sound.go` — `NormalizerLR.process`

`math.Pow(x, 64)` and `math.Pow(x, 3)` per sample (44100 Hz × 2ch) replaced
with repeated squaring / two multiplies. Identical math, ~7% system CPU saved,
ALSA underruns mostly eliminated.

## Diagnostics

### feat: PerfLog frame timing instrumentation (debug builds only)
`src/common_debug.go`, `src/common_release.go`, `src/system.go`, `src/char.go`,
`src/render.go`, `src/config.go`, `src/system_sdl.go`

Per-frame breakdown of the match loop when `[Video] PerfLog = 1`:
`[FRAME] render/action(chars/upd/fs/coll)/logic/gpu/flush/sprites/batches`.

- Build-tag split: `//go:build debug` real instrumentation, `!debug` no-op
  stubs. Release builds carry zero overhead.
- `[FRAME]` and `[FPS]` output via the standard log pipeline (debug only).
- Config comment updated: PerfLog is a debug-build feature.

### refactor: replace fmt.Printf/Println with standard Log methods
`src/config.go`, `src/common.go`, `src/hiscore_rank.go`, `src/iniutils.go`,
`src/motif.go`, `src/render_gles32.go`, `src/render_vk.go`, `src/rollback.go`,
`src/script.go`, `src/system_sdl.go`

All active `fmt.Printf`/`fmt.Println` diagnostics converted to
`LogMessage`/`LogWarn`/`LogError`/`LogDebug`.

- Warnings → `LogWarn`; errors → `LogError`; debug dumps → `LogDebug`;
  status/events → `LogMessage`.
- `fmt` import removed from `hiscore_rank.go` (now unused).
- Kept as-is: `-h` help (interactive console), commented-out debug prints.
- Note: `logWrite` is a no-op in release builds — these messages now appear
  only in debug builds.

### feat: framebuffer switch statistics (GL33)
`src/render_gl33.go` — `bindFramebuffer`

`drawCallStats.FBOSwitches` is now incremented on every actual
`gl.BindFramebuffer` call in the GL33 renderer (GLES32 already counted since
the bind-cache refactor). Feeds the batch-breakdown profiling output; no
behavior change.

### feat: GLES32 texture memory accounting
`src/render_gles32.go` — `generateTexture`/`Release`/finalizer,
`newPaletteTexture`

Debug builds now track every GLES32 texture allocation/deallocation via
`memTextureCreated`/`memTextureFreed`/`memGPUBytesSub`, mirroring the existing
GL33 instrumentation: alive-texture count, current and peak GPU bytes are
reported through the `[Mem]` log. Palette-atlas slots are counted at
256×1×32 (1 KB each), with per-slot logging throttled (1st, then every 10th)
so match-load bursts don't spam the log.

Also: `newDataTexture`/`newHDRTexture` depth changed 32/24 → **128**, i.e.
RGBA8/RGB8 → **RGBA32F**, aligning GLES32 with the GL33 backend. Float payloads
now upload with `GL_FLOAT` (`MapUploadType(128)`). GPU memory for these
textures (joint-matrix skin textures, GGXLUT, HDR env maps) grows ~4–5× —
expected, and now accurately reflected in the GPU-byte accounting.

## Upstream sync (merge e599a5ce)

Merged remote `develop-update` (upstream PRs ~#3939–#3944) into this branch.
User-visible items:

- **Rollback** — `hijackRunMatch`/`simulateFrame`/`runFrame` decoupled from the
  `*System` argument (global `sys`); extra round-skip/round-advance logging;
  fix for rollback matches hanging before the victory screen.
- **Explods** — unified pause handling (`pauseBool`/`pauseStatus`), unified
  removal (`flagForRemoval`), fixes for explods removed while paused, binding
  during slow game speeds, and delayed PalFX; PalFX `step()` split into
  `refresh()` (values) + `tickTimers()` (timers).
- **BGs/stage** — yscaledelta/parallax vertical scaling mismatch, BG window
  signs and full-res offsets, stage position snapping disabled, stage videos
  pause with the game.
- **SFFv1** — duplicate base palette handling, shared-sprite palette
  preservation, legacy 0,0 sprite handling.
- **Misc** — sprite-font color parameter combines with PalFX instead of
  overwriting it (`withFontRgba`), video transparency fix, framerate setting
  fixes, storyboard scene indexing, `[Infobox Text]` localization, checksum
  changes reverted, Vulkan OOM fallback (later reverted upstream).

## Platform notes

- libvpx is built decoder-only from source on all platforms; Windows remains
  fully static (no new runtime DLLs).
- All GLES32 changes are renderer-scoped; gl33/Vulkan paths untouched.
- Desktop (gl33) unaffected by the scissor fix and bind cache: no RenderScale
  viewport scaling, no instanced path.
- `RGBA8_SNORM` post-FBO textures (upstream, both backends) report
  `GL_FRAMEBUFFER_INCOMPLETE_ATTACHMENT` on Mali (cosmetic 0x506; 1-shader
  post pass bypasses fbo_pp). Desktop GPUs accept SNORM render targets.
