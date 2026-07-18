# Memory Debugging Plan: Asset Loading & Drawing Pipeline

## 1. Overview

This document outlines memory analysis and debugging strategies for the Ikemen-GO engine's asset loading and rendering pipeline. The analysis covers six core source files:

| File | Role |
|------|------|
| `src/image.go` | SFF parsing, sprite decoding, palette management, texture creation |
| `src/render.go` | Render pipeline, TextureAtlas, blending, `RenderSprite`, palette atlas interface |
| `src/render_gl33.go` | GL33 texture lifecycle, shader programs, FBO setup, GC finalizers, GL33 palette atlas |
| `src/render_gles32.go` | GLES32 texture lifecycle, shader programs, FBO setup, GLES32 palette atlas |
| `src/font.go` | Font loading (FNT v1/v2, SFF-based), text drawing, palette caching |
| `src/font_gl33.go` | TTF glyph atlas generation, font shader, batch rendering |
| `src/bgdef.go` | Background layer rendering |

---

## 2. Key Memory Structures & Their Lifecycle

### 2.1 Texture (`Texture_GL33` / `Texture_GLES32`)

**Location:** `render_gl33.go:176-187`, `render_gles32.go:218-228`

```go
type Texture_GL33 struct {  // (same layout as Texture_GLES32)
    width, height, depth int32
    filter bool
    handle uint32    // GL handle
    serial uint64    // Unique ID
    offsetX int32    // Palette atlas: X offset within atlas (pixels)
    offsetY int32    // Palette atlas: Y offset within atlas (pixels)
    palSlot bool     // True if this is a sub-region of the palette atlas
    atlasSize int32  // Atlas size at allocation time (0 for normal textures)
}
```

- **Allocation:** `generateTexture()` at `render_gl33.go:190-217` / `render_gles32.go:231-252`
- **Deallocation:** Via `runtime.SetFinalizer` — GC-triggered, posts `gl.DeleteTextures` to main thread
- **No explicit Release() API exists** — GPU memory freed only when Go GC collects the struct

**Debug points:**
- Monitor `textureSerialNumber` (global counter, `render.go:148`) — monotonically increasing, never wraps
- Each `SetFinalizer` closure captures the handle — if the texture struct is kept alive by any reference, the GPU handle leaks
- Palette slot textures (`.palSlot == true`) share the atlas texture's `handle` and `serial` — their finalizer only returns the slot to the free list, it does NOT call `gl.DeleteTextures`

### 2.2 Sprite

**Location:** `image.go:567-579`

```go
type Sprite struct {
    Pal      []uint32   // 256-color palette (1KB)
    Tex      Texture    // GPU texture handle
    Group    uint16
    Number   uint16
    Size     [2]uint16
    Offset   [2]int16
    palidx   int
    rle      int
    coldepth byte
    paltemp  []uint32   // Cached palette for change detection (1KB)
    PalTex   Texture    // Per-sprite palette texture
}
```

- **Lifetime:** Stored in `Sff.sprites` map (`image.go:1430`)
- **Sharing:** `shareCopy()` (line 716-734) copies `Pal` by reference, defers `Tex` copy to main thread
- **No eviction** — sprites stay loaded for the entire match/session

**Debug points:**
- `paltemp` (line 577): 1KB per sprite with palette effects — compare against `CachePalTex` call frequency
- `PalTex` (line 578): per-sprite palette GPU texture (1KB GPU) — separate from `PaletteList.PalTex`

### 2.3 PaletteList

**Location:** `image.go:395-401`

```go
type PaletteList struct {
    palettes   [][]uint32        // CPU palette data
    paletteMap []int             // Remapping table
    PalTable   map[[2]uint16]int // Key lookup
    numcols    map[[2]uint16]int
    PalTex     []Texture         // GPU palette textures
}
```

- **Allocation:** `SetSource()` grows all three arrays in lockstep (line 411-430)
- **`NewPal()`** allocates `make([]uint32, 256)` per call (line 433)
- **GPU textures** in `PalTex[]` are created lazily via `NewTextureFromPalette`

**Debug points:**
- `SetSource()` uses `append` in a loop to grow — potential for over-allocation if sprites are loaded out of order
- `PalTex[]` grows with `append(pl.PalTex, nil)` — nil entries waste slice capacity

### 2.4 TextureAtlas

**Location:** `render.go:1003-1011`

```go
type TextureAtlas struct {
    texture Texture
    width, height, depth int32
    filter  bool
    resize  bool
    skyline *list.List
}
```

- **Creation:** `CreateTextureAtlas(256, 256, 32, true)` — 256KB GPU (256x256 RGBA)
- **Resize:** `Resize()` (line 1195-1209) creates new texture, calls `CopyData` to preserve existing content
- **CopyData is now fixed** for all backends: GL33 reads source via `gl.GetTexImage` → CPU → `gl.TexSubImage2D`; GLES32 uses FBO `CopyTexSubImage2D`; Vulkan uses `vk.CmdCopyImage` with transfer barrier

**Debug points:**
- `extrudeAtlasImage()` (line 1027-1059) allocates `(w+2)*(h+2)*bpp` bytes per glyph insertion
- `clearTexture()` (line 1021-1024) allocates `width*height*bpp` bytes for zero-fill
- Font atlases have `resize: false` so resize path is unused for fonts — only `TextureAtlas.Resize` exercises `CopyData`

### 2.5 Font (`Font_GL33`)

**Location:** `font_gl33.go:20-29`

```go
type Font_GL33 struct {
    fontChar     map[rune]*character
    ttf          *truetype.Font
    textures     []*TextureAtlas
    // ...
}
```

- **Glyph atlas:** Starts at 256x256 RGBA (256KB GPU), grows via new atlases appended to `textures[]`
- **`fontChar` map:** One `*character` per glyph — includes UV coords, dimensions, advance
- **`GenerateGlyphs()`** (line 328-473): Creates `image.RGBA` per glyph, renders freetype into it, uploads to atlas

**Debug points:**
- Each glyph creates a temporary `image.NewRGBA(rect)` — small but numerous
- `f.textures` grows unbounded as new atlases are created
- `f.paltexCache` (font.go:91) maps `*uint32` pointers to textures — no eviction

### 2.6 Fnt (sprite-based fonts)

**Location:** `font.go:76-92`

```go
type Fnt struct {
    images      map[int32]map[rune]*FntCharImage
    palettes    [][256]uint32   // Embedded palettes
    paltexCache map[*uint32]Texture
    // ...
}
```

- **`loadFntV1()`** (line 148-343): Creates `[]Sprite` per character × palette count
- **`LoadFntSff()`** (line 412-471): Clones sprites from SFF, copies palettes

**Debug points:**
- `f.images[bt][c].img = make([]Sprite, len(f.palettes))` — 96 bytes × palette count per character
- `drawChar()` palette cache (line 562-580): Three-tier cache (fast path → map lookup → create)

---

## 3. SFF Loading Memory Flow

### 3.1 `loadSff()` — Full Load (`image.go:1497-1619`)

```
For each sprite:
  1. newSprite()                          — 96 bytes
  2. readHeader/readHeaderV2()            — header only, no pixel data
  3. If size > 0:
     a. read() or readV2()                — decodes pixel data
     b. RlePcxDecode/Rle8Decode/etc.      — allocates width*height bytes
     c. SetPxl() → mainThreadTask        — posts texture creation to main thread
     d. read() also reads palette          — allocates 256*4 = 1024 bytes
  4. s.sprites[key] = spriteList[i]       — stores pointer
```

**Peak memory per sprite:** `compressed_data + decoded_pixels + 1024 (palette) + GPU_texture`

### 3.2 `preloadSff()` — Selective Load (`image.go:1622-1907`)

- Only loads sprites in `preloadSpr` map
- Still reads ALL headers (to build index)
- Allocates `headerXofs`, `headerSize`, `headerShofs32` slices = `N * (4+4+8) = 16N` bytes

### 3.3 `readV2()` — Decode Paths (`image.go:1174-1257`)

| Format | Allocation | Notes |
|--------|-----------|-------|
| Raw (rle==0, 8bpp) | `make([]byte, datasize)` | Direct pixel data |
| Raw (24/32bpp) | `make([]byte, datasize)` | Passed to `SetRaw` |
| RLE8 | `make([]byte, datasize-4)` + decode output `w*h` | Double buffer |
| RLE5 | `make([]byte, datasize-4)` + decode output `w*h` | Double buffer |
| LZ5 | `make([]byte, datasize-4)` + decode output `w*h` | Double buffer |
| PNG10 | `png.Decode` → `*image.Paletted` | Uses `pi.Pix` directly |
| PNG11/12 | `png.Decode` → `image.RGBA` → possibly `draw.Draw` conversion | 2-3 intermediate images |

---

## 4. Rendering Pipeline Memory

### 4.1 `RenderSprite()` (`render.go:662-765`)

**Per-call allocations:** Essentially zero on the hot path.
- `RenderParams` passed by value (stack)
- `ShaderPalFX` is a local struct
- `renderPass` closure captures value types + pointers
- `renderSpriteQuad` uses `mgl.Mat4` on stack

### 4.2 `RenderWithBlending()` (`render.go:767-937`)

- `TT_subadd` mode: Saves/restores `origState := *spfx` — stack copy, no heap
- Multi-pass rendering creates no allocations

### 4.3 Font Batch Rendering (`font_gl33.go:122-261`)

- **`batchVertices := make([]float32, 0, batchSize*6*4)`** — allocated per `Printf` call
- With `MaxFontBatchSize=250`: up to `250*6*4*4 = 24KB` per call
- Reset via `batchVertices = batchVertices[:0]` (line 213) — reuses capacity within a call

### 4.4 Post-Processing (`render_gl33.go:1080-1179`)

- All FBOs and textures pre-allocated at init
- Ping-pong between `fbo_pp[0]` and `fbo_pp[1]` — bind-only, no allocations
- `DrawArrays` with pre-filled vertex buffer

### 4.5 BGDef Drawing (`bgdef.go:337-363`)

- No per-frame allocations
- Iterates `s.bg` slice, calls `b.draw()` which renders sprites normally
- `action()` called once per tick (guarded by `lastTick`)

---

## 5. Identified Issues

### 5.1 Bugs

| # | Severity | Issue | Location |
|---|----------|-------|----------|
| B1 | **Fixed** | `Texture_GL33.CopyData()` was a no-op — now reads source via `gl.GetTexImage` and writes to destination via `gl.TexSubImage2D`. GLES32: FBO `CopyTexSubImage2D`. Vulkan: `vk.CmdCopyImage`. | `render_gl33.go:387-418` |
| B2 | **Medium** | `Pal32ToBytes()` returns `unsafe.Slice` of local `padded` variable — backing array may be GC'd while slice is in use | `image.go:499-502` |

### 5.2 Memory Waste

| # | Priority | Issue | Impact | Location |
|---|----------|-------|--------|----------|
| W1 | Medium | `paltemp` stores full 256-entry `[]uint32` per sprite for comparison | 1KB per sprite with palette effects | `image.go:1283` |
| W2 | Medium | `Font_GL33.Printf` allocates `batchVertices` (~24KB) per text draw call | Per-frame GC pressure | `font_gl33.go:143` |
| W3 | Low | RLE decode allocates double buffers (input + output) temporarily | Short-lived, bounded by sprite size | `image.go:863,1029,1057,1101` |
| W4 | Low | `Fnt.paltexCache` has no eviction | Grows with unique palette count | `font.go:91,578` |
| W5 | Low | `readV2` PNG path creates 2-3 intermediate images | Short-lived, bounded per sprite | `image.go:1222-1247` |

### 5.3 No Issues Found

- `RenderSprite` and `drawQuads` are allocation-free in the hot path
- `BGDef.Draw` has no per-frame allocations
- `EndFrame` post-processing uses pre-created FBOs
- SFF reuse via `findActiveSff` effectively prevents duplicate loading
- Palette texture caching in `drawChar` is effective (3-tier cache)

---

## 6. Debugging Strategy

### 6.1 Runtime Memory Monitoring

Add logging to track allocations at key points:

```
// In generateTexture() — track GPU texture creation
LogMessage("[Mem] Texture created: %dx%dx%d (handle=%d, serial=%d)", ...)
LogMessage("[Mem] Texture freed: handle=%d (serial=%d)", ...)

// In loadSff() — track sprite loading
LogMessage("[Mem] SFF loaded: %s — %d sprites, %d palettes", filename, ...)
LogMessage("[Mem] SFF borrowed: %s (reuse from %s)", filename, source)

// In CachePalTex() — track palette texture churn
LogMessage("[Mem] PalTex cache miss: sprite=%p, new palette=%d colors", ...)
```

### 6.2 Heap Profile Analysis

Use Go's built-in profiling:

```go
import "runtime/pprof"

// Start CPU profile
f, _ := os.Create("cpu.prof")
pprof.StartCPUProfile(f)
defer pprof.StopCPUProfile()

// Heap profile at key moments
f, _ := os.Create("heap.prof")
pprof.WriteHeapProfile(f)
f.Close()
```

**Key profiling points:**
1. Before/after `loadSff()` — measure per-character memory cost
2. Before/after font loading — measure atlas memory
3. During gameplay — measure per-frame allocation rate
4. During palette changes — measure texture churn

### 6.3 GPU Memory Tracking

For GL33, track via `glGetIntegerv`:

```go
// Track GPU texture memory
var memKB int32
gl.GetIntegerv(0x9048, &memKB) // GL_TEXTURE_MEMORY_USED_ATI (vendor-specific)

// Count active textures
var texCount int32
gl.GetIntegerv(gl.ACTIVE_TEXTURES, &texCount)
```

### 6.4 GC Pressure Analysis

Monitor GC frequency and pause times:

```go
var lastGCStats debug.MemStats
debug.ReadMemStats(&lastGCStats)

// In main loop:
var m debug.MemStats
debug.ReadMemStats(&m)
if m.NumGC != lastGCStats.NumGC {
    LogMessage("[Mem] GC #%d: pause=%dms, heap=%dMB, objects=%d",
        m.NumGC, m.PauseNs[(m.NumGC+255)%256]/1e6,
        m.HeapAlloc/1e6, m.HeapObjects)
    lastGCStats = m
}
```

---

## 7. Optimization Recommendations

### 7.0 Completed (Build / Packaging)

- [x] **Fully static Windows binary** (`Makefile`, `packages/go-sdl2/sdl/sdl_cgo_static.go`, `packages/go-sdl2/_libs/`) — SDL2 now links statically from the vendored `libSDL2_windows_amd64.a` (and `libSDL2_windows_386.a` for Win32) via the go-sdl2 `static` build tag, replacing the shared `SDL2.dll` + `delaylibs` step. The MinGW runtime (winpthread/gcc/stdc++) links statically via `-extldflags '-static -Wl,--defsym,__ms_vsscanf=__mingw_vsscanf'`. Verified: `make clean && make` succeeds, `ldd Ikemen_GO.exe` lists only Windows system DLLs (no `libwinpthread-1.dll` / `SDL2.dll`), and the exe launches from `cmd.exe` with no `clock_gettime64` crash. Removed the now-dead `MINGW_STATIC_LIBS`, `SHARED_PKGS`, and `delaylibs` machinery from the Makefile.

### 7.1 Completed (P0)

- [x] **Pool `batchVertices`** (`font_gl33.go`, `font_gles32.go`, `font_vk.go`) — per-call `make([]float32)` replaced with struct field + cap check, eliminating ~24KB heap allocation per `Printf` call.
- [x] **Font SFF cache** (`image.go` + `font.go`) — added `fontSffCache` map + `registerFontSff()` to fix `findActiveSff()` missing font SFFs, eliminating duplicate disk loads on screen transitions.

### 7.2 Completed (P1)

- [x] **Lazy texture creation** (`Sprite.pendingData` fields in `image.go` + `ensureTex()` calls in `anim.go`, `render.go`, `font.go`, `char.go`) — GPU textures created on first render instead of eagerly during SFF loading. Eliminates ~220 MB of immediate GPU texture allocation for stage BG layers.
- [x] **Sprite-based font glyph atlas** (`render.go` + `font.go`) — Font glyph sprites packed into a `TextureAtlas` instead of individual GL textures. Added `UV` sub-texture support to `RenderParams` + `drawQuadsUV()`. Atlas index encoded in UV w-component for multi-atlas support. Eliminates ~94 individual GL textures per sprite-based font (replaced by 1-2 atlas textures).
- [x] **SFF data release after atlas texture creation** (`font.go`) — After packing glyph pixel data into the atlas via `AddImage`, release the per-glyph `pendingData`/`pendingDepth` on the cloned font sprites. The CPU-side pixel buffer is no longer needed since the GPU atlas holds the data. Frees backing pixel arrays for each glyph in a sprite-based font.
- [x] **Font atlas caching across screen transitions** (`font.go` + `image.go`) — Added `fontAtlasCache` global map to keep GPU atlas textures alive across `Fnt` GC cycles. On subsequent loads of the same font SFF, the atlas and UV map are reused instead of being rebuilt from scratch. Eliminates atlas rebuild cost (~14 fonts × 64KB GPU churn + CPU glyph re-upload) on every screen transition.
- [x] **Fix `CopyData` for all render backends** (`render_gl33.go`, `render_gles32.go`, `render_vk.go`) — GL33: `gl.GetTexImage` → CPU → `gl.TexSubImage2D`. GLES32: FBO → `gl.CopyTexSubImage2D`. Vulkan: `vk.CmdCopyImage` with transfer barrier. All three now preserve atlas content during resize, fixing the silent data loss bug (PLANS B1).
- [x] **Palette texture atlas (GL33)** (`render_gl33.go`, `render.go`, `shaders/sprite.frag.glsl`) — Replaced ~150 separate `256×1` GL palette textures with a single shared `2048×2048` atlas texture. Each palette is a `256×1` sub-region in the atlas, providing 16,384 slots. Added `palSlot` flag for proper sub-region writes via `gl.TexSubImage2D` and `palUV` uniform for per-slot UV coordinate lookup in the fragment shader. All palette slots share the atlas `serial` number so the texture cache hits instantly after the first palette bind per frame, reducing texture unit switches from ~15-19 per frame to 1.
- [x] **Palette texture atlas (GLES32)** (`render_gles32.go`) — Identical implementation to GL33: `createPalAtlas()`, atlas-based `newPaletteTexture()` with slot allocation/GC recycling, `palSlot`-aware `SetData()` using `gl.TexSubImage2D` for sub-region writes, and `GetPalUV()` returning atlas UV with 0.5 pixel centering. Same cache optimization via shared atlas serial.
- [x] **Palette slot usage counter** (`memdebug.go`, `render_gl33.go`, `render_gles32.go`) — Added `palSlotsUsed`/`palSlotsMax` atomic counters tracking concurrent palette atlas slot usage. `memPalSlotAlloc()` is called on slot allocation, `memPalSlotFree()` in the GC finalizer. Peak usage reported via `palSlots=%d/%d` in the periodic HEAP log line, enabling users to determine the minimum viable `PaletteAtlasSize` for their content.
- [x] **Palette atlas capacity warning** (`memdebug.go`, `render_gl33.go`, `render_gles32.go`) — HEAP log now shows `palSlots=used/peak/total` (total = atlas capacity). Warns at ≥90% usage and ≥100% (exhausted). `memPalSlotSetTotal()` records the capacity from `createPalAtlas()`.
- [x] **Palette atlas auto-resize** (`render_gl33.go`, `render_gles32.go`) — When `newPaletteTexture()` finds the free slot queue empty, it calls `autoResizeAtlas()` before falling back to standalone textures. Doubles the atlas (cap 4096), persists the new size to config via `sys.cfg.SetValueUpdate("Video.PaletteAtlasSize")`, creates a new larger atlas, and fills only the new slot indices. Old atlas kept alive in `oldPalAtlases[]` slice to prevent dangling GL handles on existing palette textures. The resized config takes effect immediately and persists across restarts.

- [x] **Fix palette config init order** (`main.go`) — Moved `PalAtlasSize = sys.cfg.Video.PaletteAtlasSize` from AFTER `sys.init()` to BEFORE it. Previously the atlas was created at the Go default (2048) while `GetPalUV()` used the config value (256), producing 8×-scaled UV coordinates that caused black sprites and backgrounds on startup with smaller config values.

- [x] **Fix UV mismatch after atlas auto-resize** (`render_gl33.go`, `render_gles32.go`) — Added `atlasSize int32` field to `Texture_GL33`/`Texture_GLES32`, captured at allocation time from `r.palAtlasSize`. `GetPalUV()` now uses `t.atlasSize` (per-texture) instead of the global `PalAtlasSize`. After auto-resize, old palette textures binding the old (smaller) atlas would compute UVs for the new (larger) atlas, sampling only half the palette colors → garbled/black sprites. The fix ensures each texture's UV computation always matches its actual GPU texture dimensions.

### 7.3 Configurable Settings

- [x] **`[Debug] MemoryLimitMB`** (`config.go`, `main.go`, `defaultConfig.ini`) — Replaced hardcoded `debug.SetMemoryLimit(256*1024*1024)` with config value from `save/config.ini`. Clamped to ≥ 64 MB (0 = disabled). Default: 256 MB.
- [x] **`[Video] PaletteAtlasSize`** (`config.go`, `render.go`, `defaultConfig.ini`) — Replaced hardcoded `const PalAtlasSize = 2048` with config-driven `var PalAtlasSize int32`. Clamped to ≥ 256 and rounded up to next power of two. Default: 2048 (16,384 palette slots).

### 7.4 Completed

- [x] **Texture.Release()** (`render.go`, `render_gl33.go`, `render_gles32.go`, `render_vk.go`) — Added `Release()` to the `Texture` interface. GL33/GLES32: queues `gl.DeleteTextures` on the main thread + zeros `handle`. VK: pushes `VkImage`/view/sampler/allocation to deferred destruction queue + sets `img = nil`. All three backends have double-free guards in their GC finalizers. Palette atlas slots are no-ops (shared atlas handle owned by finalizer).
- [x] **`paltemp` → `palhash`** (`image.go`, `system.go`, `memdebug.go`) — Replaced per-sprite `[]uint32` palette cache copy (~1 KB heap alloc) with `uint64` FNV-1a hash (8 bytes inline, no heap alloc). `CachePalTex()` now compares a single 64-bit integer instead of a 1024-byte slice. Reduces per-sprite palette cache memory by ~99.2%.

### 7.5 Remaining Opportunities

1. **SFF eviction** — When characters are removed from `sys.cgi`, release their SFF data
2. **Pool RLE decode buffers** — `sync.Pool` for `[]byte` buffers
3. **Fix `Pal32ToBytes` for non-256 palettes** — Return properly retained slice
4. **Virtual texture streaming** — Load sprites on-demand
5. **GPU memory budget** — Track total GPU allocation and evict LRU textures

---

## 8. Files to Modify for Debugging

| File | Change | Purpose |
|------|--------|---------|
| `src/render_gl33.go` | Add logging in `generateTexture()` and finalizer | Track GPU texture lifecycle |
| `src/image.go` | Add logging in `loadSff()`, `CachePalTex()` | Track asset loading and palette churn |
| `src/render.go` | Add allocation counters in `TextureAtlas` | Track atlas memory usage |
| `src/font_gl33.go` | Add glyph count logging in `GenerateGlyphs()` | Track font atlas growth |
| `src/system.go` | Add GC stats logging in main loop | Monitor GC pressure |

---

## 9. Test Scenarios

1. **Large SFF load:** Load a character with 1000+ sprites, monitor peak memory
2. **Rapid palette switching:** Change palettes every frame, measure texture churn
3. **Text-heavy screen:** Display 100+ TextSprites simultaneously, measure batch vertex allocation
4. **Long session:** Run for 30+ minutes, check for memory leaks (growing heap without release)
5. **Stage with background layers:** Load complex stages, verify no per-frame allocations in `BGDef.Draw`

---

## 10. Success Criteria

- [ ] No GPU texture leaks (handle count stable after loading completes)
- [ ] Per-frame heap allocations < 10KB during steady-state gameplay
- [ ] GC pause times < 5ms (no single large allocation causing long pauses)
- [ ] SFF reuse works correctly (second character with same SFF reuses all textures)
- [ ] Font atlas count stable after all visible glyphs are loaded


---

# Memory Usage Review: Asset Loading & Drawing Pipeline

## Scope
Review of memory allocation patterns, lifecycle, and potential issues across: SFF loading, sprite/texture management, palette handling, font rendering, the GL33 rendering pipeline, and background layer drawing.

---

## 1. Texture Lifecycle — GL33 Backend

### Findings

**`render_gl33.go:202-208` — GC-triggered deletion is the only cleanup path.**
Every texture created via `generateTexture()` gets a `runtime.SetFinalizer` that posts `gl.DeleteTextures` to the main thread. There is no explicit "free" API — GPU texture handles are released only when Go's GC collects the `Texture_GL33` struct. This means:
- GPU memory is not released until GC runs (potentially much later than when a texture becomes unreachable).
- If many textures are created in a burst (e.g., loading a large SFF), GPU memory peaks before GC catches up.

**`render_gl33.go:387-418` — `Texture_GL33.CopyData()` was fixed.**
Previously a no-op, it now reads source via `gl.GetTexImage` → CPU buffer → `gl.TexSubImage2D`. GLES32 uses FBO `gl.CopyTexSubImage2D`. Vulkan uses `vk.CmdCopyImage`. All three backends preserve atlas content during resize, fixing the silent data loss bug (PLANS B1).

**`render_gl33.go:186-209` — `textureSerialNumber` is a global uint64 incremented on every `generateTexture` call.**
It's only used to detect stale texture handles in the texture cache. The counter itself is fine, but it means the cache (`texCacheTexSerial`/`texCacheLastUsed`) is invalidated on every `ChangeProgram` call (`render_gl33.go:1289-1294`), causing a full reset of the texture binding cache whenever switching between sprite and model shaders.

### Recommendations
- Consider adding an explicit `Release()` method on the `Texture` interface for deterministic GPU cleanup (called when an SFF is unloaded or a character is removed).
- Fix `CopyData` for GL33 or remove the call from `Resize` to avoid false confidence.
- The finalizer approach is acceptable for GC-managed textures, but consider calling `runtime.GC()` or `debug.SetGCPercent()` hints during loading phases to avoid GPU memory buildup.

---

## 2. SFF Loading — Sprite & Palette Allocations

### Findings

**`image.go:1497-1619` — `loadSff()` loads ALL sprites eagerly.**
Each sprite calls `read()` or `readV2()`, which:
1. Allocates a `[]byte` for RLE-decoded pixel data: `make([]byte, width*height)` (`image.go:863`, `:1029`, `:1057`, `:1101`). For large sprites this can be significant.
2. Creates a GPU texture via `SetPxl` or `SetRaw` which posts to `mainThreadTask`.
3. Palettes are stored as `[]uint32` (256 colors × 4 bytes = 1KB each) in `PaletteList.palettes`.

**`image.go:1174-1257` — `readV2()` PNG path has multi-allocation decode.**
- Format 11/12 (RGBA PNG): `png.Decode` → `image.RGBA` → `draw.Draw` to convert. This creates 2-3 intermediate images before the final `[]byte` is uploaded to GPU. The intermediate `img` (result of `png.Decode`) is not explicitly freed — it relies on GC.
- Format 10 (paletted PNG): `png.Decode` → `*image.Paletted` → uses `pi.Pix` directly. More efficient.

**`image.go:943` — RLE decode buffers: `px := make([]byte, rleSize)` then `s.RlePcxDecode(px)` allocates ANOTHER `[]byte`.**
The decode functions (`Rle8Decode`, `Rle5Decode`, `Lz5Decode`) each allocate `make([]byte, width*height)` for output. So loading one PCX-encoded sprite uses ~2× the pixel buffer temporarily (input compressed + output decoded). These are short-lived but happen for every sprite.

**`image.go:1463-1488` — `findActiveSff()` reuses SFF by pointer sharing.**
If the same SFF file is referenced by multiple characters or the stage, `findActiveSff` returns the existing `*Sff` pointer. This is a significant memory saver — avoids duplicating all sprites and palettes. However, the `sff.sprites` map is never pruned. Once loaded, all sprites remain in memory for the lifetime of the match.

**`image.go:2089-2099` — `cloneSpriteWithPal()` allocates a new `[]uint32` palette copy for each cloned sprite.**
This is called during font loading (`font.go:423`) and fight screen face setup. Each clone creates a 1KB palette allocation even if the palette is shared.

**`image.go:1800-1819` — Palette reading in `preloadSff` allocates `pal := make([]uint32, 256)` per sprite that needs a palette.**
This is expected but worth noting — if a character has 50 palettes, that's 50 × 1KB = 50KB just for palette data.

### Recommendations
- For `readV2` PNG format 11/12, consider using `png.Decode` with a streaming decoder or reusing decode buffers across sprites.
- RLE decode buffers could be pooled with `sync.Pool` since sprite dimensions are bounded by SFF constraints.
- Consider adding an SFF eviction policy when characters are no longer needed.

---

## 3. Palette Texture Management

### Findings

**`image.go:1262-1286` — `CachePalTex()` is effective but stores `paltemp` as a full copy.**
Each sprite that uses palette effects stores a `[]uint32` copy of the previous palette for comparison (`paltemp = append([]uint32{}, pal...)`). This means every sprite with palette effects holds an extra 1KB. For a character with hundreds of sprites, this adds up.

**`image.go:487-503` — `Pal32ToBytes()` has a fast path for 256-color palettes using `unsafe.Slice`.**
When palette is exactly 256 colors, it returns a zero-copy view into the existing `[]uint32`. When palette is shorter (e.g., 16 or 32 colors from SFFv2), it allocates a padded `[]uint32, 256` which is a temporary allocation per upload.

**`image.go:505-517` — `NewTextureFromPalette()` creates a 256×1 palette texture (1KB GPU).**
Palette textures are 256×1 RGBA = 1KB on GPU. This is very efficient.

**`char.go:216`, `:4352`, `:4378`, `:4400` — `NewTextureFromPalette` called during palette changes.**
Each palette swap creates a new 1KB GPU texture. The old one is GC'd via finalizer. During palette selection or palette cycling, this creates churn. The `PalTex` array on `PaletteList` stores references, so old textures should become unreachable when overwritten.

### Recommendations
- The `paltemp` per-sprite copy is the main waste. Consider storing only a hash/checksum instead of the full 256-entry array for comparison.
- `Pal32ToBytes` for non-256 palettes could use a pre-allocated reusable buffer.

---

## 4. Font Rendering Memory

### Findings

**`font_gl33.go:440` — Font glyph atlases start at 256×256 RGBA (256KB GPU) and grow.**
Atlas creation: `CreateTextureAtlas(256, 256, 32, true)` creates a 256×256×4 = 256KB texture. New atlases are appended as needed. There is no limit on atlas count, but in practice a font rarely needs more than 2-4 atlases for ASCII ranges.

**`font_gl33.go:199` — Missing glyphs trigger batch loading: `f.GenerateGlyphs(low, low+31)`.**
Loading 32 glyphs at a time. Each glyph creates a temporary `image.RGBA` (small, ~20×20 pixels), renders freetype into it, then uploads to the atlas via `AddImage`. The intermediate `rgba` images are small and short-lived.

**`font.go:91-99` — `Fnt.paltexCache` maps `*uint32` pointers to `Texture`.**
This cache uses pointer identity (`&pal[0]`) as keys. It grows unbounded — every unique palette base pointer gets a new texture. In practice this is bounded by the number of distinct palettes used, but there's no eviction.

**`font.go:562-580` — `drawChar` palette texture caching is effective.**
The three-tier cache (fast-path `lastPalBase` check → `paltexCache` lookup → `NewTextureFromPalette`) avoids creating new textures per character. Only bank changes or palette changes trigger new uploads.

**`font.go:324-342` — `loadFntV1` creates `[]Sprite` per character × palette.**
`fci.img = make([]Sprite, len(f.palettes))` — if a font has 16 palettes and 200 characters, that's 3200 `Sprite` structs. Each Sprite is small (no texture, just shared via `shareCopy`), but the slice overhead adds up.

**`font_gl33.go:143` — `batchVertices` is re-allocated per Printf call.**
`batchVertices := make([]float32, 0, batchSize*6*4)` — with `MaxFontBatchSize=250`, this allocates `250*6*4*4 = 24KB` per text draw call. This is a per-frame allocation that could be pooled.

### Recommendations
- Pool `batchVertices` in `Font_GL33.Printf` — it's allocated on every text draw call.
- Consider capping or cleaning up `paltexCache` when fonts are reloaded.
- The `loadFntV1` sprite×palette matrix is inherently needed but the `[]Sprite` could be replaced with a flat array to reduce overhead.

---

## 5. Rendering Pipeline — Per-Frame Allocations

### Findings

**`render.go:662-765` — `RenderSprite()` is allocation-free on the happy path.**
`RenderParams` is passed by value (stack-allocated). `ShaderPalFX` is a local struct. `renderPass` is a closure but captures only value types and pointers. The `renderSpriteQuad` path uses `mgl.Mat4` on the stack. No heap allocations in the hot path.

**`render.go:1027-1059` — `extrudeAtlasImage()` allocates a new `[]byte` per call.**
`out := make([]byte, int(outStride*outHeight))` — for a 32×32 glyph, this is `(34*4)*(34) = ~4.6KB`. This is called during font glyph generation, not per-frame, so it's acceptable.

**`render.go:1195-1209` — `TextureAtlas.Resize()` allocates a new texture + clear buffer.**
The `clearTexture` call at line 1203 allocates `make([]byte, width*height*bpp)` to zero-fill the new texture. For a 512×512 RGBA atlas, this is 1MB. The `CopyData` is a no-op (bug noted above), so the clear + upload is wasted work if CopyData were fixed.

**`render_gl33.go:1069-1078` — `BeginFrame()` clears buffers.**
The FBO clear is a GPU operation, not a CPU allocation. This is efficient.

**`render_gl33.go:1080-1179` — `EndFrame()` post-processing loop.**
No allocations — all textures and FBOs are pre-created at init. The ping-pong between `fbo_pp[0]` and `fbo_pp[1]` is bind-only.

**`render.go:287-304` — `drawQuads()` sets vertex data per quad.**
`gfx.SetVertexData(...)` passes 8 floats as arguments. No allocation — these go directly to the GPU via `gl.BufferData`.

**`bgdef.go:337-363` — `BGDef.Draw()` has no per-frame allocations.**
It iterates `s.bg` slices and calls `b.draw()` which renders sprites normally. The `action()` method is called once per tick (guarded by `lastTick`).

### Recommendations
- The rendering pipeline is well-optimized for per-frame allocation avoidance. The main concerns are loading-time allocations, not runtime.

---

## 6. Identified Issues Summary

### Bugs
| Priority | Issue | Location |
|----------|-------|----------|
| **High** | `Texture_GL33.CopyData()` is a no-op — atlas resize silently loses all glyph data | `render_gl33.go:379-381` |
| **Medium** | `Pal32ToBytes()` for non-256 palettes returns `unsafe.Slice` of a local `padded` variable — the backing array may be GC'd while the slice is still in use | `image.go:499-502` |

### Memory Waste
| Priority | Issue | Impact | Location |
|----------|-------|--------|----------|
| **Medium** | `paltemp` stores full 256-entry `[]uint32` per sprite for comparison | 1KB per sprite with palette effects | `image.go:1283` |
| **Medium** | `Font_GL33.Printf` allocates `batchVertices` ([]float32, ~24KB) per text draw call | Per-frame GC pressure | `font_gl33.go:143` |
| **Low** | RLE decode allocates double buffers (input + output) temporarily | Short-lived, bounded by sprite size | `image.go:863,1029,1057,1101` |
| **Low** | `Fnt.paltexCache` has no eviction | Grows with unique palette count | `font.go:91,578` |
| **Low** | `readV2` PNG path creates 2-3 intermediate images before final upload | Short-lived, bounded per sprite | `image.go:1222-1247` |

### No Issues Found
- `RenderSprite` and `drawQuads` are allocation-free in the hot path
- `BGDef.Draw` has no per-frame allocations
- `EndFrame` post-processing uses pre-created FBOs
- SFF reuse via `findActiveSff` effectively prevents duplicate loading
- Palette texture caching in `drawChar` is effective (3-tier cache)

---

## 7. Files Involved

| File | Role |
|------|------|
| `src/image.go` | Sprite loading, SFF parsing, palette management, texture creation |
| `src/render.go` | Render pipeline, TextureAtlas, blending, `RenderSprite` |
| `src/render_gl33.go` | GL33 texture creation, shader programs, FBO setup, finalizers |
| `src/font.go` | Font loading (FNT v1/v2, SFF fonts), text drawing, palette caching |
| `src/font_gl33.go` | TTF glyph atlas generation, font shader, batch rendering |
| `src/bgdef.go` | Background layer rendering (no memory concerns found) |
| `src/char.go` | Character palette texture creation (callers of `NewTextureFromPalette`) |

---

## 8. Verification

To verify any changes:
1. Load a character with a large SFF (e.g., one with 1000+ sprites) and monitor GPU memory via GPU profiler or `glGetIntegerv(GL_TEXTURE_MEMORY)`.
2. Change palettes rapidly in select screen and verify no texture leaks (old palette textures should become unreachable).
3. Draw text with different fonts/banks and verify `paltexCache` doesn't grow unbounded.
4. Test font atlas generation with non-Latin characters (CJK) to exercise atlas resizing.
5. Verify `CopyData` fix by testing any future code that calls `TextureAtlas.Resize`.

