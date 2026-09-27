# CHANGES

## Features

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
  schema behind its keys, so their key tables drop the *State* and
  *Type / default* columns, which the Motif view keeps.
- `GET /api/file?path=…`, `POST /api/save` — view and edit any `.def`/`.ini`
  file inside the game folder. Writes are line based, so ordering, comments and
  indentation are preserved.
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
- The *State* column of a key is `defined` (the file sets it), `missing` (the
  engine knows it and uses its `default` tag) or `unknown` (in the file but not
  in the editor's model). `unknown` only ever meant "not in my model", never
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
`menu.itemname.editor*` entries in the default motif, so every screenpack gets
it without changes. Selecting an item starts the service on demand and shows it
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
    single field to scope a rebuild to.
  Keys the engine has no field for, and keys whose screen cannot be rebuilt, are
  reported as not applied rather than as needing a restart, since a restart would
  not help either.  Measured on the bundled motif: 1205 keys assigned directly,
  2427 assigned and refreshed, 5816 reload-only.

  #### How each `[Title Info]` key is applied

  Every key of a whole `[Title Info]` block applies live, but not by one
  mechanism — each one needs a different one, which is why the block is a decent
  test of the classification. Verified against `deploy/` with the debug build
  (`-httpservice`, `POST /api/save`): all thirteen below return
  `applied=true, needsReload=false`, and saving each one twice produces no
  second diff and no drift.

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

  A no-op save now also leaves the file byte for byte identical. The engine's
  own motif aligns its inline comments with runs of spaces
  (`menu.boxcursor.visible = 0         ;Set to 1 …`), and the save collapsed
  those to a single space, so saving an unchanged value produced a diff in
  every aligned line. The whitespace in front of a `;` is now preserved, and
  only the value is replaced.

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
