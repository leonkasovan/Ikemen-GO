# Batching Strategy Plan — Low-End Device Optimization (R36S / OpenGL ES 3.2)

## Target
- **Device:** Anbernic R36S — ARM64 Linux, Mali-class GPU, 512 MB RAM
- **API:** OpenGL ES 3.2 (`render_gles32.go`, build tag `armdevice`)
- **Goal:** 60 FPS on R36S (currently ~15 FPS on a fight scene that runs 60 FPS on desktop)
- **Constraint:** Must not break desktop (GL 3.3) or Android paths

---

## 1. Root Cause Diagnosis

### What the profiler showed (desktop, but pattern applies to ARM)
- `glBindFramebuffer` → **79% cumulative CPU** on the render thread
- `glTexSubImage2D` / `SetData` → per-sprite texture re-upload before fixes
- Each `RenderSprite()` call issues ~15–20 `glUniform*` calls + 1 VBO upload + 1 draw call

### Why this is catastrophic on R36S
A typical fight frame issues **200–400 draw calls**, each with full state-change overhead:

```
RenderSprite() per sprite:
  SetSpritePipeline()        → glUseProgram (if shader changes)
  EnableScissor()            → glScissor + glEnable
  SetUniformMatrix x2        → 2× glUniformMatrix4fv
  SetUniformI x4             → 4× glUniform1i
  SetUniformF x4             → 4× glUniform1f
  SetUniformFv x3            → 3× glUniform3fv
  SetTexture x2              → 2× glActiveTexture + glBindTexture + glUniform1i
  SetVertexData              → glBufferData (16 floats)
  RenderQuad()               → glDrawArrays(TRIANGLE_STRIP, 0, 4)
```

On Mali GPUs (R36S), each state change triggers a pipeline flush. The driver must
re-validate shader state, re-fetch uniforms, and re-bind textures for every draw call.
With 300 draw calls/frame at 60 FPS = 18,000 GL state-change sequences per second.

### Key insight
Most sprites in a frame share the **same shader, blend mode, and scissor rect**.
Only the geometry, texture handle, palette UV, and PalFX differ per sprite.
Batching groups these into a single instanced draw call.

---

## 2. Codebase Map (files an executor must understand)

| File | Role |
|------|------|
| `src/render.go` | `RenderParams`, `RenderSprite()`, `DrawList`, `renderSpriteQuad()`, `drawQuadsUV()` |
| `src/render_gles32.go` | GLES 3.2 backend: `Renderer_GLES32`, `SetVertexData`, `RenderQuad`, `SetUniformI/F`, `SetSpritePipeline`, `bindFramebuffer` |
| `src/render_gl33.go` | Desktop GL 3.3 backend — parallel structure to GLES, must stay working |
| `src/anim.go` | `DrawList []*SpriteData`, `SpriteData` struct, `DrawList.draw()` |
| `src/system.go` | `luaFlushDrawQueue()`, `spriteList DrawList` |
| `src/config.go` | `Config.Video` struct — add new fields here |
| `src/shaders/sprite.vert.glsl` | Sprite vertex shader (shared GL/GLES/VK path) |
| `src/shaders/sprite.frag.glsl` | Sprite fragment shader |

### Key structs

```go
// src/render.go
type RenderParams struct {
    tex        Texture      // sprite texture (GL handle inside)
    paltex     Texture      // palette texture (atlas slot)
    size       [2]uint16
    x, y       float32
    tile       Tiling
    xts, xbs   float32
    ys, vs     float32
    rxadd      float32
    xas, yas   float32
    rot        Rotation
    tint       uint32
    blendMode  TransType
    blendAlpha [2]int32
    mask       int32
    pfx        *PalFX
    window     *[4]int32    // scissor rect
    rcx, rcy   float32
    projectionMode int32
    fLength    float32
    xOffset, yOffset float32
    shader     string       // custom shader name — batch breaker
    customShader CustomShaderRenderData
    UV         [4]float32
}

// src/render_gles32.go
type Renderer_GLES32 struct {
    spriteShader  *ShaderProgram_GLES32
    vertexBuffer  uint32        // single VBO, overwritten per draw
    vertexScratch []byte        // reusable scratch for SetVertexData
    spriteVAO     uint32
    GLES32State                 // uniform/texture/blend cache
    // ...
}

// src/anim.go
type SpriteData struct {
    anim       *Animation
    pfx        *PalFX
    pos        [2]float32
    scl        [2]float32
    trans      TransType
    alpha      [2]int32
    layerno    int32
    rot        Rotation
    window     [4]float32
    customShader CustomShaderRenderData
    // ...
}
type DrawList []*SpriteData
```

---


---

## 3. Phase 1 — Draw Call Instrumentation (prerequisite, ~1 day)

**Goal:** Add a zero-cost-when-off counter that records draw call count, batch count,
and the reason each batch breaks. One summary log line per frame. No per-sprite logging.

### 3.1 Add to `src/config.go` — `Config.Video`

```go
// Inside Config.Video struct:
DrawCallLog bool `ini:"DrawCallLog"` // Log batch stats once per frame (debug only)
```

Add default to `src/resources/defaultConfig.ini`:
```ini
[Video]
DrawCallLog = false
```

### 3.2 Add to `src/render.go` — new `DrawCallStats` type and global

```go
// DrawCallStats tracks per-frame batching metrics.
// All fields are reset at the start of BeginFrame.
// Only populated when sys.cfg.Video.DrawCallLog is true.
type DrawCallStats struct {
    TotalDrawCalls int
    TotalBatches   int
    BreakShader    int // batch broken by shader change
    BreakBlend     int // batch broken by blend mode / alpha change
    BreakScissor   int // batch broken by scissor rect change
    BreakTexture   int // batch broken by sprite texture change
    BreakPalTex    int // batch broken by palette texture change
    BreakTrapez    int // batch broken by trapezoid flag change
    BreakMask      int // batch broken by mask change
    BreakIsRgba    int // batch broken by RGBA vs paletted change
}

var drawCallStats DrawCallStats

func (s *DrawCallStats) reset() {
    *s = DrawCallStats{}
}

func (s *DrawCallStats) logFrame(frameNo int) {
    if !sys.cfg.Video.DrawCallLog {
        return
    }
    LogMessage("[BATCH] frame=%d draws=%d batches=%d breaks: shader=%d blend=%d scissor=%d tex=%d pal=%d trapez=%d mask=%d rgba=%d",
        frameNo,
        s.TotalDrawCalls, s.TotalBatches,
        s.BreakShader, s.BreakBlend, s.BreakScissor,
        s.BreakTexture, s.BreakPalTex,
        s.BreakTrapez, s.BreakMask, s.BreakIsRgba,
    )
}
```

### 3.3 Increment counters in `RenderSprite()` in `src/render.go`

At the top of `RenderSprite()`, before any GL calls, add:

```go
if sys.cfg.Video.DrawCallLog {
    drawCallStats.TotalDrawCalls++
}
```

To count batch breaks, add a helper that compares the current `RenderParams` against
the previous one. Store the previous params in a package-level var:

```go
var lastRenderParams *RenderParams // nil at frame start

func recordBatchBreak(rp *RenderParams) {
    if !sys.cfg.Video.DrawCallLog || lastRenderParams == nil {
        lastRenderParams = rp
        drawCallStats.TotalBatches++
        return
    }
    prev := lastRenderParams
    broke := false
    if rp.shader != prev.shader {
        drawCallStats.BreakShader++; broke = true
    }
    if rp.blendMode != prev.blendMode || rp.blendAlpha != prev.blendAlpha {
        drawCallStats.BreakBlend++; broke = true
    }
    if rp.window != prev.window {
        drawCallStats.BreakScissor++; broke = true
    }
    texSerial := func(t Texture) uint64 {
        if t == nil { return 0 }
        if tg, ok := t.(*Texture_GLES32); ok { return tg.serial }
        return 0
    }
    if texSerial(rp.tex) != texSerial(prev.tex) {
        drawCallStats.BreakTexture++; broke = true
    }
    if texSerial(rp.paltex) != texSerial(prev.paltex) {
        drawCallStats.BreakPalTex++; broke = true
    }
    isRgba := rp.paltex == nil
    prevIsRgba := prev.paltex == nil
    if isRgba != prevIsRgba {
        drawCallStats.BreakIsRgba++; broke = true
    }
    isTrapez := Abs(Abs(rp.xts)-Abs(rp.xbs)) > 0.001
    prevTrapez := Abs(Abs(prev.xts)-Abs(prev.xbs)) > 0.001
    if isTrapez != prevTrapez {
        drawCallStats.BreakTrapez++; broke = true
    }
    if rp.mask != prev.mask {
        drawCallStats.BreakMask++; broke = true
    }
    if broke {
        drawCallStats.TotalBatches++
    }
    lastRenderParams = rp
}
```

Call `recordBatchBreak(&rp)` at the top of `RenderSprite()` after the `IsValid()` guard.

### 3.4 Reset and log in render loop

In `src/render_gles32.go`, `BeginFrame()`:
```go
drawCallStats.reset()
lastRenderParams = nil
```

In `src/render_gles32.go`, `EndFrame()` (after post-processing, before SDL swap):
```go
drawCallStats.logFrame(int(sys.frameCounter))
```

Do the same for `render_gl33.go` `BeginFrame`/`EndFrame`.

### 3.5 Expected output (one line per frame, only when DrawCallLog=true)
```
[BATCH] frame=1234 draws=287 batches=94 breaks: shader=2 blend=41 scissor=8 tex=31 pal=0 trapez=4 mask=1 rgba=7
```

This tells you exactly which break condition dominates before investing in batching.
**Run this on R36S first.** The dominant break category determines Phase 3 priority.

---


---

## 4. Phase 2 — Quick Wins (independent of batching, ~1 day)

These are config-driven switches that give immediate FPS improvement on low-end
devices with no rendering correctness risk.

### 4.1 Resolution scaling (`src/config.go`)

Add to `Config.Video`:
```go
RenderScale float32 `ini:"RenderScale"` // 0.5–1.0; renders at this fraction of window size, upscaled
```

Default in `defaultConfig.ini`:
```ini
RenderScale = 1.0
```

**Implementation in `src/render_gles32.go` `Init()`:**

After the window size is known, compute internal render dimensions:
```go
renderW := int32(float32(sys.scrrect[2]) * sys.cfg.Video.RenderScale)
renderH := int32(float32(sys.scrrect[3]) * sys.cfg.Video.RenderScale)
// Clamp to even numbers (required by some GLES drivers)
renderW = (renderW / 2) * 2
renderH = (renderH / 2) * 2
```

Allocate `r.fbo_texture` at `renderW × renderH` instead of full window size.
In `EndFrame()`, blit the FBO to the default framebuffer with bilinear scaling:
```go
gl.BlitFramebuffer(0, 0, renderW, renderH,
    0, 0, sys.scrrect[2], sys.scrrect[3],
    gl.COLOR_BUFFER_BIT, gl.LINEAR)
```

**Impact:** At `RenderScale=0.75`, pixel fill is reduced to 56% (0.75²).
At `RenderScale=0.5`, fill drops to 25%. For R36S, `0.75` is a good first target.

### 4.2 Disable shadow and model rendering via config

`Config.Video.EnableModel` and `Config.Video.EnableModelShadow` already exist.
In `defaultConfig.ini` for the `armdevice` build, set:
```ini
EnableModel = false
EnableModelShadow = false
```

Add a build-tag-specific default config override in `src/util_armdevice.go`:
```go
func platformDefaultConfig(cfg *Config) {
    if cfg.Video.RenderScale == 0 {
        cfg.Video.RenderScale = 0.75
    }
    cfg.Video.EnableModel = false
    cfg.Video.EnableModelShadow = false
    cfg.Video.MSAA = 0
}
```

Call `platformDefaultConfig(&sys.cfg)` after loading config in `src/main.go`,
only when the loaded config has no explicit override (i.e., first run).

### 4.3 Disable MSAA on armdevice

MSAA on a Mali GPU at 1280×720 requires 4× the framebuffer memory and adds a
resolve pass. Force `MSAA = 0` via the platform default above.

The existing `sys.msaa` path in `render_gles32.go` `Init()` already handles `msaa == 0`
correctly — no code changes needed beyond the config default.

### 4.4 VSync cap

Ensure the SDL swap interval is 1 (not 0) on armdevice. Uncapped rendering
wastes CPU/GPU cycles. In `src/system_sdl.go`:
```go
// Already exists: gfx.SetVSync(sys.cfg.Video.VSync)
// Armdevice default config should have: VSync = 1
```

Add to `platformDefaultConfig`:
```go
if cfg.Video.VSync == 0 {
    cfg.Video.VSync = 1
}
```

---


---

## 5. Phase 3 — Deferred Draw Queue (prerequisite for batching, ~2 days)

Currently `RenderSprite()` issues GL commands immediately when called. Batching
requires collecting all draw calls for a frame first, then issuing them sorted and
grouped. The existing `DrawList` / `luaFlushDrawQueue` system only covers sprite
animations — `FillRect` and other calls bypass it. This phase unifies all 2D sprite
draw calls into a single deferred queue.

### 5.1 New type: `SpriteDrawCall` in `src/render.go`

```go
// SpriteDrawCall is a fully resolved, ready-to-render sprite command.
// It is enqueued by RenderSprite() and flushed in batch at end of frame.
type SpriteDrawCall struct {
    // Batch key — fields that, if different from the previous call, break the batch.
    shaderName   string
    blendMode    TransType
    blendAlpha   [2]int32
    scissor      [4]int32
    isRgba       bool   // paltex == nil
    isTrapez     bool
    mask         int32

    // Per-instance data — packed into the instance VBO.
    texSerial    uint64        // used for sort/group, not uploaded
    palSerial    uint64        // used for sort/group, not uploaded
    tex          Texture
    paltex       Texture
    modelview    [16]float32   // pre-computed modelview matrix
    spfx         ShaderPalFX
    alpha        float32
    blendEq      BlendEquation
    blendSrc     BlendFunc
    blendDst     BlendFunc
    // Quad geometry — output of renderSpriteQuad() pre-computation
    // Four corners: (x1,y1), (x2,y2), (x3,y3), (x4,y4) and UV
    x1,y1, x2,y2, x3,y3, x4,y4 float32
    uv           [4]float32
    palUV        [4]float32    // palette atlas UV
    tint         [4]float32
    proj         [16]float32   // projection matrix (same for all, but kept for completeness)
    customShader CustomShaderRenderData
}
```

### 5.2 Frame-scoped queue in `src/render.go`

```go
// spriteQueue holds all deferred sprite draw calls for the current frame.
// Reset at BeginFrame, flushed at end of luaFlushDrawQueue / after DrawList.draw().
var spriteQueue []SpriteDrawCall

func resetSpriteQueue() {
    spriteQueue = spriteQueue[:0]
}
```

### 5.3 Modify `RenderSprite()` in `src/render.go`

When batching is enabled (`sys.cfg.Video.EnableSpriteBatching`), instead of
issuing GL commands immediately, compute all derived values and enqueue:

```go
func RenderSprite(rp RenderParams) {
    if !rp.IsValid() {
        return
    }
    recordBatchBreak(&rp) // Phase 1 instrumentation

    if sys.cfg.Video.EnableSpriteBatching {
        enqueueSpriteDrawCall(rp)
        return
    }
    // existing immediate-mode path unchanged
    renderSpriteImmediate(rp)
}
```

`enqueueSpriteDrawCall` pre-computes everything that `renderSpriteImmediate` would
compute (spfx, projection, modelview, geometry) and appends to `spriteQueue`.
This avoids re-computing during the flush pass.

```go
func enqueueSpriteDrawCall(rp RenderParams) {
    initRenderSpriteQuad(&rp)

    spfx := ShaderPalFX{neg: false, add: [3]float32{0,0,0}, mult: [3]float32{1,1,1}}
    if rp.pfx != nil {
        spfx = rp.pfx.getFinalPalFx(rp.blendMode, rp.blendAlpha)
    }

    tint := [4]float32{
        float32(rp.tint&0xff)/255,
        float32(rp.tint>>8&0xff)/255,
        float32(rp.tint>>16&0xff)/255,
        float32(rp.tint>>24&0xff)/255,
    }

    proj := gfx.OrthographicProjectionMatrix(0, float32(sys.scrrect[2]), 0, float32(sys.scrrect[3]), -65535, 65535)
    modelview := mgl.Translate3D(0, float32(sys.scrrect[3]), 0)

    isTrapez := Abs(Abs(rp.xts)-Abs(rp.xbs)) > 0.001
    isRgba := rp.paltex == nil

    var palUV [4]float32
    if rp.paltex != nil {
        palUV = rp.paltex.GetPalUV()
    }

    texSerial, palSerial := uint64(0), uint64(0)
    if t, ok := rp.tex.(*Texture_GLES32); ok { texSerial = t.serial }
    if p, ok := rp.paltex.(*Texture_GLES32); ok { palSerial = p.serial }

    dc := SpriteDrawCall{
        shaderName: rp.customShader.name,
        blendMode:  rp.blendMode,
        blendAlpha: rp.blendAlpha,
        scissor:    *rp.window,
        isRgba:     isRgba,
        isTrapez:   isTrapez,
        mask:       rp.mask,
        texSerial:  texSerial,
        palSerial:  palSerial,
        tex:        rp.tex,
        paltex:     rp.paltex,
        modelview:  [16]float32(modelview),
        proj:       [16]float32(proj),
        spfx:       spfx,
        tint:       tint,
        palUV:      palUV,
        customShader: rp.customShader,
    }
    // geometry will be computed during flush via renderSpriteQuad
    // Store rp for geometry computation at flush time:
    // (alternative: compute geometry here — preferred for cleaner flush)
    spriteQueue = append(spriteQueue, dc)
}
```

### 5.4 Flush function `flushSpriteQueue()` in `src/render.go`

Called at the end of each draw layer (after all `DrawList.draw()` calls and
`luaFlushDrawQueue`). For now in non-batching mode this is a no-op; in batching
mode it sorts and renders:

```go
func flushSpriteQueue() {
    if len(spriteQueue) == 0 {
        return
    }
    if !sys.cfg.Video.EnableSpriteBatching {
        spriteQueue = spriteQueue[:0]
        return
    }
    // Phase 4 will sort and batch here.
    // For now: render each call individually (validates the deferred path).
    for i := range spriteQueue {
        renderSpriteFromCall(&spriteQueue[i])
    }
    spriteQueue = spriteQueue[:0]
}
```

### 5.5 Call sites

In `src/system.go`, `luaFlushDrawQueue()`, append at the end:
```go
flushSpriteQueue()
```

In `src/render_gles32.go`, `BeginFrame()`:
```go
resetSpriteQueue()
```

### 5.6 Config flag

Add to `Config.Video`:
```go
EnableSpriteBatching bool `ini:"EnableSpriteBatching"`
```

Default `defaultConfig.ini`:
```ini
EnableSpriteBatching = false
```

In `platformDefaultConfig` (`util_armdevice.go`):
```go
cfg.Video.EnableSpriteBatching = true
```

This flag lets you enable batching only on armdevice until it's proven stable,
then promote it to all platforms.

---


---

## 6. Phase 4 — Instanced Quad Batching (~3 days)

This is the main FPS gain. Instead of one `glDrawArrays` per sprite, group
consecutive compatible sprites and issue one `glDrawArraysInstanced` call per batch.

### 6.1 Batch key definition

Two draw calls can be in the same batch if and only if **all** of these match:

| Field | Source in `SpriteDrawCall` |
|-------|---------------------------|
| Shader name | `shaderName` |
| Blend mode + alpha | `blendMode`, `blendAlpha` |
| Blend equation/src/dst | `blendEq`, `blendSrc`, `blendDst` |
| Scissor rect | `scissor` |
| `isRgba` (paletted vs RGBA) | `isRgba` |
| `isTrapez` | `isTrapez` |
| `mask` | `mask` |
| Sprite texture handle | `texSerial` |
| Palette texture handle | `palSerial` |

Note: texture handle must match because the current shader uses a single `sampler2D tex`.
Phase 5 (texture arrays) removes the texture-handle constraint.

### 6.2 Per-instance data layout

Each instance packs its variable data into a flat `[]float32` stream:

```
// Per-instance vertex attribute layout (stride = 27 floats = 108 bytes)
// Attribute 2: modelview rows 0–3  (16 floats, 4 × vec4, location 2–5)
// Attribute 6: palUV               (4 floats, vec4,       location 6)
// Attribute 7: tint                (4 floats, vec4,       location 7)
// Attribute 8: spfx_neg_add_mult   (7 floats: 1+3+3,     location 8–9)
// Attribute 10: alpha_gray_hue     (3 floats,             location 10)
// Total: 16+4+4+7+3 = 34 floats per instance
```

Exact layout (flat `[]float32`, 34 floats × 4 bytes = 136 bytes per instance):
```
[0..15]  modelview matrix (column-major, 4×4)
[16..19] palUV (u1, v1, u2, v2 — from palette atlas slot)
[20..23] tint (r, g, b, a)
[24]     neg (0.0 or 1.0)
[25..27] add (r, g, b)
[28..30] mult (r, g, b)
[31]     alpha
[32]     gray
[33]     hue
```

### 6.3 New GPU buffers in `Renderer_GLES32`

Add to `Renderer_GLES32` struct in `src/render_gles32.go`:

```go
// Instanced sprite rendering
instanceVBO       uint32  // per-instance attribute VBO
instanceVAO       uint32  // VAO for instanced sprite pass
instanceScratch   []float32 // reusable CPU-side buffer
maxInstancesPerBatch int32 // e.g. 512
```

Initialize in `Init()`:
```go
r.maxInstancesPerBatch = 512
gl.GenBuffers(1, &r.instanceVBO)
gl.GenVertexArrays(1, &r.instanceVAO)

// Bind VAO and configure attributes
gl.BindVertexArray(r.instanceVAO)

// Attribute 0: position (xy) from static quad VBO
gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
gl.EnableVertexAttribArray(0)
gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 4*4, nil) // xy
// Attribute 1: uv from static quad VBO
gl.EnableVertexAttribArray(1)
gl.VertexAttribPointerWithOffset(1, 2, gl.FLOAT, false, 4*4, 2*4) // uv

// Attributes 2–10: per-instance data from instanceVBO
gl.BindBuffer(gl.ARRAY_BUFFER, r.instanceVBO)
stride := int32(34 * 4) // 34 floats per instance

// modelview: 4 vec4 attributes (location 2,3,4,5)
for col := 0; col < 4; col++ {
    loc := uint32(2 + col)
    gl.EnableVertexAttribArray(loc)
    gl.VertexAttribPointerWithOffset(loc, 4, gl.FLOAT, false, stride, uintptr(col*16))
    gl.VertexAttribDivisor(loc, 1) // advance once per instance
}
// palUV: location 6
gl.EnableVertexAttribArray(6)
gl.VertexAttribPointerWithOffset(6, 4, gl.FLOAT, false, stride, 16*4)
gl.VertexAttribDivisor(6, 1)
// tint: location 7
gl.EnableVertexAttribArray(7)
gl.VertexAttribPointerWithOffset(7, 4, gl.FLOAT, false, stride, 20*4)
gl.VertexAttribDivisor(7, 1)
// neg+add+mult (7 floats, packed as 2× vec4 at locations 8,9): location 8
gl.EnableVertexAttribArray(8)
gl.VertexAttribPointerWithOffset(8, 4, gl.FLOAT, false, stride, 24*4) // neg,add.rgb
gl.VertexAttribDivisor(8, 1)
gl.EnableVertexAttribArray(9)
gl.VertexAttribPointerWithOffset(9, 3, gl.FLOAT, false, stride, 28*4) // mult.rgb
gl.VertexAttribDivisor(9, 1)
// alpha+gray+hue: location 10
gl.EnableVertexAttribArray(10)
gl.VertexAttribPointerWithOffset(10, 3, gl.FLOAT, false, stride, 31*4)
gl.VertexAttribDivisor(10, 1)

gl.BindVertexArray(0)
```

Allocate scratch buffer:
```go
r.instanceScratch = make([]float32, int(r.maxInstancesPerBatch)*34)
```

### 6.4 Instanced vertex shader (`src/shaders/sprite_instanced.vert.glsl`) — new file

```glsl
#version 320 es
precision highp float;
precision highp int;

// Static per-vertex (unit quad, same for all instances)
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 uv;

// Per-instance attributes (divisor = 1)
layout(location = 2) in vec4 i_mv0;    // modelview col 0
layout(location = 3) in vec4 i_mv1;    // modelview col 1
layout(location = 4) in vec4 i_mv2;    // modelview col 2
layout(location = 5) in vec4 i_mv3;    // modelview col 3
layout(location = 6) in vec4 i_palUV;  // palette atlas UV
layout(location = 7) in vec4 i_tint;
layout(location = 8) in vec4 i_negadd; // x=neg, yzw=add.rgb
layout(location = 9) in vec3 i_mult;
layout(location = 10) in vec3 i_alphagray; // x=alpha, y=gray, z=hue

// Uniforms (shared per batch)
uniform mat4 projection;
uniform vec4 x1x2x4x3;
uniform int  isTrapez;

// Outputs to fragment shader
out vec2  texcoord;
out vec4  v_palUV;
out vec4  v_tint;
out float v_neg;
out vec3  v_add;
out vec3  v_mult;
out float v_alpha;
out float v_gray;
out float v_hue;

void main(void) {
    mat4 modelview = mat4(i_mv0, i_mv1, i_mv2, i_mv3);
    texcoord = uv;
    v_palUV  = i_palUV;
    v_tint   = i_tint;
    v_neg    = i_negadd.x;
    v_add    = i_negadd.yzw;
    v_mult   = i_mult;
    v_alpha  = i_alphagray.x;
    v_gray   = i_alphagray.y;
    v_hue    = i_alphagray.z;
    gl_Position = projection * (modelview * vec4(position, 0.0, 1.0));
}
```

### 6.5 Instanced fragment shader (`src/shaders/sprite_instanced.frag.glsl`) — new file

Same PalFX logic as `sprite.frag.glsl` but reads per-instance varyings instead of
uniforms. Keep the uniform fallback for the batch-level constants (`isRgba`, `mask`,
`tex`, `pal`, `isTrapez`, `x1x2x4x3`):

```glsl
#version 320 es
precision highp float;
precision highp int;

uniform sampler2D tex;
uniform sampler2D pal;
uniform vec4  x1x2x4x3;
uniform int   mask;
uniform bool  isFlat;
uniform bool  isRgba;
uniform bool  isTrapez;

in vec2  texcoord;
in vec4  v_palUV;
in vec4  v_tint;
in float v_neg;
in vec3  v_add;
in vec3  v_mult;
in float v_alpha;
in float v_gray;
in float v_hue;

out vec4 FragColor;

// (Copy rgb2hsv, hsv2rgb, hue_shift helpers verbatim from sprite.frag.glsl)
// ... (omitted for brevity, copy exactly)

void main(void) {
    // Same logic as sprite.frag.glsl main() but using v_* varyings
    // instead of uniforms for per-instance values.
    // Batch-level values (tex, pal, isRgba, isTrapez, mask) remain uniforms.
    vec4 c;
    vec3 neg_base = vec3(1.0);
    vec3 final_add = v_add;
    vec4 final_mul = vec4(v_mult, v_alpha);

    if (isFlat) {
        c = v_tint;
        neg_base *= c.a;
        final_add *= c.a;
        final_mul.rgb *= v_alpha;
    } else {
        vec2 _uv = texcoord;
        if (isTrapez) {
            vec2 bounds = mix(x1x2x4x3.zw, x1x2x4x3.xy, _uv.y);
            float gap = bounds[1] - bounds[0];
            if (abs(gap) < 0.0001) gap = 0.0001;
            _uv.x = (gl_FragCoord.x - bounds[0]) / gap;
        }
        c = texture(tex, _uv);
        if (isRgba) {
            if (mask == -1) c.a = 1.0;
            neg_base *= c.a;
            final_add *= c.a;
            final_mul.rgb *= v_alpha;
        } else {
            c = texture(pal, vec2(v_palUV.x + v_palUV.z * c.r * 0.9966, v_palUV.y));
            if (mask == -1) c.a = 1.0;
        }
    }
    if (v_hue != 0.0)  c.rgb = hue_shift(c.rgb, v_hue);
    if (v_neg != 0.0)  c.rgb = neg_base - c.rgb;
    c.rgb = mix(vec3((c.r+c.g+c.b)/3.0), c.rgb, 1.0 - v_gray);
    c.rgb += final_add;
    c    *= final_mul;
    if (!isFlat) c.rgb = mix(c.rgb, v_tint.rgb * c.a, v_tint.a);
    FragColor = c;
}
```

Embed both shaders in `src/render.go`:
```go
//go:embed shaders/sprite_instanced.vert.glsl
var instancedVertShader string
//go:embed shaders/sprite_instanced.frag.glsl
var instancedFragShader string
```

### 6.6 Compile the instanced shader in `Renderer_GLES32.Init()`

```go
r.instancedSpriteShader, err = r.newShaderProgram(
    instancedVertShader, instancedFragShader, "", "Instanced Sprite Shader", true)
// Register uniforms used per-batch:
r.instancedSpriteShader.RegisterUniforms(
    "projection", "x1x2x4x3", "isTrapez", "isFlat", "isRgba", "mask")
r.instancedSpriteShader.RegisterTextures("tex", "pal")
```

Add `instancedSpriteShader *ShaderProgram_GLES32` to `Renderer_GLES32` struct.

### 6.7 Implement `renderBatch()` in `src/render_gles32.go`

```go
// renderBatch draws a slice of compatible SpriteDrawCalls in a single
// instanced draw call. All calls in the slice must share the same batch key.
func (r *Renderer_GLES32) renderBatch(calls []SpriteDrawCall) {
    if len(calls) == 0 {
        return
    }
    first := &calls[0]

    // --- Batch-level GL state ---
    r.ChangeProgram(r.instancedSpriteShader.program)
    gl.BindVertexArray(r.instanceVAO)

    // Scissor
    r.EnableScissor(first.scissor[0], first.scissor[1], first.scissor[2], first.scissor[3])

    // Projection (same for all sprites in a frame)
    gl.UniformMatrix4fv(r.instancedSpriteShader.uniforms["projection"], 1, false, &first.proj[0])

    // Batch-constant uniforms
    r.SetUniformISub(r.instancedSpriteShader.uniforms["isFlat"], 0)
    r.SetUniformISub(r.instancedSpriteShader.uniforms["isRgba"], Btoi(first.isRgba))
    r.SetUniformISub(r.instancedSpriteShader.uniforms["isTrapez"], Btoi(first.isTrapez))
    r.SetUniformISub(r.instancedSpriteShader.uniforms["mask"], int32(first.mask))

    // Blend state
    r.EnableBlending(first.blendEq, first.blendSrc, first.blendDst)

    // Texture binding (same for all calls in batch)
    texUnit := r.instancedSpriteShader.textures["tex"]
    gl.ActiveTexture(uint32(gl.TEXTURE0 + texUnit))
    if t, ok := first.tex.(*Texture_GLES32); ok {
        gl.BindTexture(gl.TEXTURE_2D, t.handle)
    }
    gl.Uniform1i(r.instancedSpriteShader.uniforms["tex"], int32(texUnit))

    if !first.isRgba && first.paltex != nil {
        palUnit := r.instancedSpriteShader.textures["pal"]
        gl.ActiveTexture(uint32(gl.TEXTURE0 + palUnit))
        if p, ok := first.paltex.(*Texture_GLES32); ok {
            gl.BindTexture(gl.TEXTURE_2D, p.handle)
        }
        gl.Uniform1i(r.instancedSpriteShader.uniforms["pal"], int32(palUnit))
    }

    // --- Pack per-instance data ---
    n := len(calls)
    needed := n * 34
    if cap(r.instanceScratch) < needed {
        r.instanceScratch = make([]float32, needed)
    }
    buf := r.instanceScratch[:needed]

    for i, dc := range calls {
        off := i * 34
        copy(buf[off:off+16], dc.modelview[:])
        copy(buf[off+16:off+20], dc.palUV[:])
        copy(buf[off+20:off+24], dc.tint[:])
        buf[off+24] = float32(Btoi(dc.spfx.neg))
        buf[off+25] = dc.spfx.add[0]
        buf[off+26] = dc.spfx.add[1]
        buf[off+27] = dc.spfx.add[2]
        buf[off+28] = dc.spfx.mult[0]
        buf[off+29] = dc.spfx.mult[1]
        buf[off+30] = dc.spfx.mult[2]
        buf[off+31] = dc.alpha
        buf[off+32] = dc.spfx.gray
        buf[off+33] = dc.spfx.hue
    }

    gl.BindBuffer(gl.ARRAY_BUFFER, r.instanceVBO)
    gl.BufferData(gl.ARRAY_BUFFER, needed*4, unsafe.Pointer(&buf[0]), gl.DYNAMIC_DRAW)

    // --- Single draw call ---
    gl.DrawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, int32(n))

    gl.BindVertexArray(0)
    r.DisableScissor()
}
```

### 6.8 Sort and batch `spriteQueue` in `flushSpriteQueue()`

Replace the stub in `src/render.go`:

```go
func flushSpriteQueue() {
    if len(spriteQueue) == 0 {
        return
    }
    if !sys.cfg.Video.EnableSpriteBatching {
        spriteQueue = spriteQueue[:0]
        return
    }

    // Sort by batch key to maximise batch size.
    // Order within same key is preserved (stable sort) to maintain draw order.
    sort.SliceStable(spriteQueue, func(i, j int) bool {
        a, b := &spriteQueue[i], &spriteQueue[j]
        if a.shaderName != b.shaderName   { return a.shaderName < b.shaderName }
        if a.blendMode  != b.blendMode    { return a.blendMode  < b.blendMode  }
        if a.isRgba     != b.isRgba       { return !a.isRgba }
        if a.texSerial  != b.texSerial    { return a.texSerial  < b.texSerial  }
        if a.palSerial  != b.palSerial    { return a.palSerial  < b.palSerial  }
        return false
    })

    // Walk queue and flush when batch key changes or batch is full.
    start := 0
    maxBatch := 512
    for i := 1; i <= len(spriteQueue); i++ {
        if i == len(spriteQueue) || !sameBatchKey(&spriteQueue[i], &spriteQueue[start]) || i-start >= maxBatch {
            if r, ok := gfx.(*Renderer_GLES32); ok {
                r.renderBatch(spriteQueue[start:i])
            } else {
                // Fallback for GL33 or other renderers: render individually
                for k := start; k < i; k++ {
                    renderSpriteFromCall(&spriteQueue[k])
                }
            }
            start = i
        }
    }
    spriteQueue = spriteQueue[:0]
}

func sameBatchKey(a, b *SpriteDrawCall) bool {
    return a.shaderName == b.shaderName &&
        a.blendMode    == b.blendMode  &&
        a.blendAlpha   == b.blendAlpha &&
        a.blendEq      == b.blendEq    &&
        a.blendSrc     == b.blendSrc   &&
        a.blendDst     == b.blendDst   &&
        a.scissor      == b.scissor    &&
        a.isRgba       == b.isRgba     &&
        a.isTrapez     == b.isTrapez   &&
        a.mask         == b.mask       &&
        a.texSerial    == b.texSerial  &&
        a.palSerial    == b.palSerial
}
```

---


---

## 7. Phase 5 — Multi-Texture Binding to Remove Texture-Change Breaks (~2 days)

After Phase 4 the dominant remaining batch breaker (per instrumentation) is expected
to be `BreakTexture`. GLES 3.2 guarantees at least 16 texture image units. Binding
multiple sprite textures per batch eliminates per-texture draw calls.

### 7.1 Strategy: texture slot array in shader

Instead of one `sampler2D tex`, use an array of samplers:

```glsl
#define MAX_TEX_SLOTS 8
uniform sampler2D texArray[MAX_TEX_SLOTS];
```

Add a per-instance `int texSlot` (packed into the instance VBO as a float, cast in shader).
The batcher fills slots 0..N with unique textures in the batch, assigns each instance
its slot index.

### 7.2 New batch key

Remove `texSerial` and `palSerial` from the batch key. A batch can now span multiple
textures as long as:
- Number of unique `(texSerial, palSerial)` pairs ≤ `MAX_TEX_SLOTS`
- All other batch-key fields still match

### 7.3 Instance data layout change

Add 2 extra floats to the 34-float layout:

```
[34] texSlot  (float, cast to int in shader: int(round(v_texSlot)))
[35] palSlot
```

New stride: 36 floats = 144 bytes per instance.

### 7.4 Shader change

```glsl
layout(location = 11) in float v_texSlot;
layout(location = 12) in float v_palSlot;
uniform sampler2D texArray[8];
uniform sampler2D palArray[8];

// In main():
int ts = int(round(v_texSlot));
int ps = int(round(v_palSlot));
vec4 c = texture(texArray[ts], _uv);
// palette:
c = texture(palArray[ps], vec2(v_palUV.x + v_palUV.z * c.r * 0.9966, v_palUV.y));
```

**Note:** GLES 3.2 requires constant index for sampler arrays in some implementations.
Use a `switch(ts)` workaround if the driver rejects dynamic indexing:
```glsl
vec4 sampleTex(int slot, vec2 uv) {
    switch(slot) {
        case 0: return texture(texArray[0], uv);
        case 1: return texture(texArray[1], uv);
        // ... up to MAX_TEX_SLOTS-1
        default: return vec4(0.0);
    }
}
```

### 7.5 Batcher changes in `renderBatch()`

Before packing instance data, build a slot map:
```go
type texKey struct{ tex, pal uint64 }
slotMap := make(map[texKey]int, 8)
slots   := make([]texKey, 0, 8)

for i := range calls {
    k := texKey{calls[i].texSerial, calls[i].palSerial}
    if _, ok := slotMap[k]; !ok {
        slotMap[k] = len(slots)
        slots = append(slots, k)
    }
}
// Bind textures to units
for i, k := range slots {
    // find the Texture objects by serial (store in a map or carry pointers in SpriteDrawCall)
    gl.ActiveTexture(uint32(gl.TEXTURE0 + i))
    gl.BindTexture(gl.TEXTURE_2D, lookupTexHandle(k.tex))
    // ... bind pal to unit i+MAX_TEX_SLOTS
}
```

---

## 8. Phase 6 — FBO Pass Reduction (~1 day)

The profiler showed `glBindFramebuffer` at 79% cumulative CPU even on desktop.
On R36S this is worse because Mali drivers re-validate the entire pipeline on every
FBO switch.

### 8.1 Audit FBO switches per frame

Add FBO switch counter alongside `DrawCallStats`:

```go
type DrawCallStats struct {
    // ... existing fields
    FBOSwitches int
}
```

In `bindFramebuffer()` in `render_gles32.go`, increment when the target actually changes:
```go
func (r *Renderer_GLES32) bindFramebuffer(target uint32, fbo uint32) {
    // existing cache check...
    if target == gl.FRAMEBUFFER || target == gl.DRAW_FRAMEBUFFER {
        if r.curDrawFbo != fbo {
            drawCallStats.FBOSwitches++  // add this line
            r.curDrawFbo = fbo
            gl.BindFramebuffer(target, fbo)
        }
    }
    // ...
}
```

### 8.2 Merge post-processing passes

`EndFrame()` currently runs one FBO switch per post-processing shader. If only one
post shader is active (common case), skip the ping-pong and write directly to the
default framebuffer.

```go
if len(r.postShaderSelect) == 1 {
    // Single pass: read from fbo_texture, write to default framebuffer directly
    r.bindFramebuffer(gl.FRAMEBUFFER, 0)
    gl.Viewport(...)
    // draw fullscreen quad with postShaderSelect[0]
    // skip all ping-pong FBO allocations
}
```

### 8.3 Skip intermediate FBO when no post-processing

When `len(r.postShaderSelect) == 0`, render directly to the default framebuffer
(SDL window surface) without any intermediate FBO. Add a flag:

```go
r.useIntermediateFBO = len(r.postShaderSelect) > 0 || sys.msaa > 0
```

In `BeginFrame()`:
```go
if r.useIntermediateFBO {
    r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)
} else {
    r.bindFramebuffer(gl.FRAMEBUFFER, 0)
}
```

This eliminates the main render FBO switch entirely when no post-processing shaders
are configured — likely the default on R36S.

---

## 9. Testing Plan

### 9.1 Desktop regression test (after each phase)

```bash
make CONFIG=debug install
# Run, open fight screen, verify visual output unchanged
# Check log for any GL errors
```

Run the existing test suite:
```bash
cd src && go test ./... -tags "static debug"
```

### 9.2 Batch instrumentation verification (Phase 1)

Enable `DrawCallLog = true` in config.
Expected log at title screen: ~50–80 draws, ~30–50 batches.
Expected log in fight scene: ~200–350 draws, reduced batches after Phase 4.

### 9.3 Instanced rendering correctness checklist

Visual regression: compare fight screenshots before/after batching:
- [ ] Palettes render correctly (no palette bleed between sprites)
- [ ] PalFX (neg, add, mult, gray, hue) works on characters
- [ ] Shadows render correctly
- [ ] Stage backgrounds tile correctly
- [ ] Transparency (AddAlpha, Sub) renders correctly
- [ ] Custom shaders still work (they fall through to non-batched path)
- [ ] Trapezoid parallax backgrounds render correctly

### 9.4 R36S benchmark

Build for arm64 Linux:
```bash
make GOOS=linux GOARCH=arm64 CONFIG=release
```

Measure FPS with `sys.cfg.Video.DrawCallLog = true` to capture batch stats on device.

Target: **≥45 FPS** in a 2-character fight at 1280×720 with `RenderScale=0.75`.

---

## 10. Implementation Order (recommended)

| Phase | Task | Est. | Dependency |
|-------|------|------|------------|
| 1 | Draw call instrumentation | 0.5d | none |
| 2 | Quick wins (RenderScale, disable model/shadow, config defaults) | 0.5d | none |
| 3 | Deferred draw queue (`SpriteDrawCall`, `flushSpriteQueue`) | 1.5d | none |
| 4 | Instanced batching (new shader, `renderBatch`, sort+group) | 2d | Phase 3 |
| 6 | FBO pass reduction | 0.5d | none (independent) |
| 5 | Multi-texture slots | 1.5d | Phase 4 |

Phases 1, 2, and 6 are fully independent and can be done in parallel with Phase 3.

---

## 11. Config Reference (all new fields)

Add to `src/config.go` inside `Config.Video`:

```go
DrawCallLog          bool    `ini:"DrawCallLog"`          // default: false
RenderScale          float32 `ini:"RenderScale"`          // default: 1.0 (armdevice: 0.75)
EnableSpriteBatching bool    `ini:"EnableSpriteBatching"` // default: false (armdevice: true)
```

Add to `src/resources/defaultConfig.ini` under `[Video]`:
```ini
DrawCallLog = false
RenderScale = 1.0
EnableSpriteBatching = false
```

Override in `src/util_armdevice.go` `platformDefaultConfig()`:
```go
func platformDefaultConfig(cfg *Config) {
    if cfg.Video.RenderScale == 0 {
        cfg.Video.RenderScale = 0.75
    }
    cfg.Video.EnableModel = false
    cfg.Video.EnableModelShadow = false
    cfg.Video.MSAA = 0
    cfg.Video.EnableSpriteBatching = true
    if cfg.Video.VSync == 0 {
        cfg.Video.VSync = 1
    }
}
```

---

## 12. Notes and Caveats

- **Custom shaders** (`RenderParams.shader != ""`): always fall through to the
  non-batched immediate path. They are rare (per-character shader effects) and
  incompatible with the generic instanced shader.

- **`renderWithBlending` multi-pass sprites** (Sub, SubAdd blend modes): these call
  `renderPass()` 2× per sprite. The deferred queue must store the resolved
  `(blendEq, blendSrc, blendDst, alpha, spfx)` for each pass as separate
  `SpriteDrawCall` entries (one per pass), not one per sprite.

- **`FillRect`**: not a `RenderParams` call — keep it on the immediate path.
  It is infrequent (lifebars, overlays) and uses a flat color, not a sprite texture.

- **`renderSpriteHTile` tiling**: tiled sprites generate multiple quads per call.
  Each quad becomes one `SpriteDrawCall` entry. The geometry loop in
  `renderSpriteQuad` → `renderSpriteHTile` must be factored to emit into the queue
  rather than call `drawQuadsUV` directly.

- **Sort stability**: `sort.SliceStable` must be used so sprites with identical
  batch keys preserve their original draw order (required for layering correctness).

- **R36S texture unit limit**: Mali-G31 (common in RK3326 devices like R36S) has
  16 texture image units. Phase 5 should use `MAX_TEX_SLOTS = 7` (7 sprite + 7 pal
  + 2 reserved = 16) to stay within the limit.
