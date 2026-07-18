# Changelog

## [Unreleased]

### Changed

- **Fully static Windows binary** — SDL2 is now linked statically from the
  repo's vendored `packages/go-sdl2/_libs/libSDL2_windows_amd64.a` (via the
  `-tags static` go-sdl2 build path) instead of the shared `SDL2.dll`, and the
  MinGW runtime (winpthread/gcc/stdc++) is linked statically via
  `-extldflags '-static -Wl,--defsym,__ms_vsscanf=__mingw_vsscanf'`. The
  `ldd` of `Ikemen_GO.exe` now lists only Windows system DLLs — no
  `libwinpthread-1.dll`, `libgcc_s_seh-1.dll`, `libstdc++-6.dll`, or
  `SDL2.dll`. The `delaylibs` step is removed and no DLLs are bundled.
  Fixes the original regression where the exe ran under the MSYS2 terminal but
  failed to launch from `cmd.exe`/Explorer ("procedure entry point
  clock_gettime64 could not be located").
- `PalAtlasSize` changed from a compile-time `const` to a runtime `var` initialized from config, enabling tuning without recompilation.
- `debug.SetMemoryLimit` now reads from config instead of being hardcoded to 256 MB.
- `PLANS.md` updated with completed palette atlas entries and corrected `CopyData` fix references.

### Added

- **Palette texture atlas (GL33)** — Replaced ~150 separate `256×1` GL palette textures with a single shared `2048×2048` atlas texture. Each palette is a `256×1` sub-region, providing 16,384 slots per atlas. Palette slots use `gl.TexSubImage2D` for efficient sub-region writes and share the atlas serial number so the texture cache hits instantly after the first palette bind per frame, reducing GPU state changes from ~15-19 per frame to 1.

- **Palette texture atlas (GLES32)** — Same optimization ported to the OpenGL ES 3.2 backend for Android: atlas allocation, slot recycling via GC finalizer, and `palUV` uniform for per-slot UV lookup.

- **Palette slot usage telemetry** — Added `palSlotsUsed`/`palSlotsMax` atomic counters (both backends) tracking current and peak concurrent palette atlas slot usage. The periodic `HEAP` log line reports `palSlots=%d(peak %d)`, where the first value is slots currently allocated and the second is the peak concurrent count seen so far (a process-lifetime high-water mark, not the configured atlas capacity). Use it to pick the minimum viable `PaletteAtlasSize`.

- **Configurable memory limit** — Added `[Debug] MemoryLimitMB` (`save/config.ini`, default: `256`). Controls Go's `debug.SetMemoryLimit()` to return freed pages to the OS. Set to `0` to disable (not recommended — may cause Task Manager to show 1 GB+ after loading SFFs). Clamped to ≥ 64 MB.

- **Configurable palette atlas size** — Added `[Video] PaletteAtlasSize` (`save/config.ini`, default: `1024`). Controls the palette atlas texture dimensions (square, RGBA). Clamped to ≥ 256 and rounded up to the next power of two. At 1024×1024 the atlas provides 4,096 palette slots (each 256×1) at 4 MB GPU memory, down from the previous default of 2048 (16,384 slots, 16 MB).

- **Deterministic GPU texture cleanup** (`render.go`, `render_gl33.go`, `render_gles32.go`, `render_vk.go`) — Added `Release()` to the `Texture` interface, enabling explicit GPU resource freeing when SFFs are unloaded or sprites are evicted. GL33/GLES32: queued `gl.DeleteTextures` on the main thread with `handle` zeroed to prevent double-free in finalizers. Vulkan: pushes to deferred destruction queue with `img = nil` guard. Palette atlas slots are no-ops (handled by finalizer slot recycling).

- **`paltemp` → `palhash` optimization** (`image.go`, `system.go`, `memdebug.go`) — Replaced per-sprite `[]uint32` palette cache copy (~1 KB heap allocation) with a `uint64` FNV-1a hash (8 bytes inline, zero heap). `CachePalTex()` now compares a single 64-bit integer instead of a 1024-byte slice. HEAP log shows `palthashes=%d palhashBytes=%d` (e.g. `159 palthashes, 1272 bytes` vs the old `159 KB`). Reduces per-sprite palette cache memory by ~99.2%.

### Fixed

- Compile error in `render_gl33.go` where `PalAtlasSize` type change (`int32` vs untyped const) caused loop variable type mismatch.
- Compile error in `render_vk.go` where `Release()` attempted `vk.ImageView(0)` and `vk.Sampler(0)` casts — these types are structs, not integer handles, so the cast was invalid. Removed unnecessary nil-out of imageView/sampler (finalizer guard only checks `t.img`).

- **Palette atlas config init order** — `PalAtlasSize` was read from config AFTER `sys.init()` created the atlas at the Go default (2048). `GetPalUV()` used the config value (256), producing wrong UV coordinates → black sprites/backgrounds when `PaletteAtlasSize < 2048`. Fixed by moving the config read before `sys.init()`.

- **UV mismatch after atlas auto-resize** — `GetPalUV()` used the global `PalAtlasSize`, which changes after a resize. Old palette textures (binding the old, smaller atlas) computed UVs for the new, larger atlas — sampling only ~50% of palette colors → garbled sprites. Fixed by storing `atlasSize` per-texture at allocation time and using it in `GetPalUV()` instead of the global.
