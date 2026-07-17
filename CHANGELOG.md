# Changelog

## [Unreleased]

### Added

- **Palette texture atlas (GL33)** — Replaced ~150 separate `256×1` GL palette textures with a single shared `2048×2048` atlas texture. Each palette is a `256×1` sub-region, providing 16,384 slots per atlas. Palette slots use `gl.TexSubImage2D` for efficient sub-region writes and share the atlas serial number so the texture cache hits instantly after the first palette bind per frame, reducing GPU state changes from ~15-19 per frame to 1.

- **Palette texture atlas (GLES32)** — Same optimization ported to the OpenGL ES 3.2 backend for Android: atlas allocation, slot recycling via GC finalizer, and `palUV` uniform for per-slot UV lookup.

- **Palette slot usage telemetry** — Added `palSlotsUsed`/`palSlotsMax` atomic counters (both backends) tracking current and peak concurrent palette atlas slot usage. The periodic `HEAP` log line reports `palSlots=%d(peak %d)`, where the first value is slots currently allocated and the second is the peak concurrent count seen so far (a process-lifetime high-water mark, not the configured atlas capacity). Use it to pick the minimum viable `PaletteAtlasSize`.

- **Configurable memory limit** — Added `[Debug] MemoryLimitMB` (`save/config.ini`, default: `256`). Controls Go's `debug.SetMemoryLimit()` to return freed pages to the OS. Set to `0` to disable (not recommended — may cause Task Manager to show 1 GB+ after loading SFFs). Clamped to ≥ 64 MB.

- **Configurable palette atlas size** — Added `[Video] PaletteAtlasSize` (`save/config.ini`, default: `1024`). Controls the palette atlas texture dimensions (square, RGBA). Clamped to ≥ 256 and rounded up to the next power of two. At 1024×1024 the atlas provides 4,096 palette slots (each 256×1) at 4 MB GPU memory, down from the previous default of 2048 (16,384 slots, 16 MB).

### Changed

- `PalAtlasSize` changed from a compile-time `const` to a runtime `var` initialized from config, enabling tuning without recompilation.
- `debug.SetMemoryLimit` now reads from config instead of being hardcoded to 256 MB.
- `PLANS.md` updated with completed palette atlas entries and corrected `CopyData` fix references.

### Fixed

- Compile error in `render_gl33.go` where `PalAtlasSize` type change (`int32` vs untyped const) caused loop variable type mismatch.

- **Palette atlas config init order** — `PalAtlasSize` was read from config AFTER `sys.init()` created the atlas at the Go default (2048). `GetPalUV()` used the config value (256), producing wrong UV coordinates → black sprites/backgrounds when `PaletteAtlasSize < 2048`. Fixed by moving the config read before `sys.init()`.

- **UV mismatch after atlas auto-resize** — `GetPalUV()` used the global `PalAtlasSize`, which changes after a resize. Old palette textures (binding the old, smaller atlas) computed UVs for the new, larger atlas — sampling only ~50% of palette colors → garbled sprites. Fixed by storing `atlasSize` per-texture at allocation time and using it in `GetPalUV()` instead of the global.
