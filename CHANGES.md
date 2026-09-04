# CHANGES

## Features

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
