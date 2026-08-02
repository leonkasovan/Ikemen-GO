# CHANGES

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

## Platform notes

- All GLES32 changes are renderer-scoped; gl33/Vulkan paths untouched.
- Desktop (gl33) unaffected by the scissor fix and bind cache: no RenderScale
  viewport scaling, no instanced path.
- `RGBA8_SNORM` post-FBO textures (upstream, both backends) report
  `GL_FRAMEBUFFER_INCOMPLETE_ATTACHMENT` on Mali (cosmetic 0x506; 1-shader
  post pass bypasses fbo_pp). Desktop GPUs accept SNORM render targets.
