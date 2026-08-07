# DirectX Renderer — Research

Status: **Phases 0–1 implemented and verified; Phase 3 (model pipeline)
partially implemented (A+B: core model pass, unlit/base-color materials).** A
working D3D 11 backend (`render_dx.go`, `render_dx_font.go`, 8 embedded HLSL
shaders) renders a full 8-player kfm match at solid 60 FPS on an Intel GPU
and draws GLB-model stage backgrounds (`stage3d.def`-style) through the real
model pipeline. Set `RenderMode = Direct3D 11` in `config.ini` to use it.
Remaining: Phase 3 C–E (shadow maps, cubemap/IBL passes), Phase 2 polish,
Phase 4 default promotion.

This doc maps how the Direct3D backend slots into Ikemen-GO's renderer
architecture and tracks phase completion.

## 1. Why DirectX

Ikemen-GO already ships four renderers (`OpenGL 3.3`, `OpenGL ES 3.2`,
`Vulkan 1.3`, `SDL2 Software`). A Direct3D backend is motivated by:

- **Driver lottery.** On the Windows handhelds this project targets (R36S,
  ROG Ally, generic ARM clamshells), OpenGL support is the weakest link —
  vendors ship broken/old GL drivers, MSAA and external shaders already have
  to be force-disabled on the software renderer (`fb2ee390`). Direct3D 11 is
  the one GPU API guaranteed present on every Windows 7+ machine with WDDM;
  D3D12 on every Windows 10+ machine. A DX backend removes GL as a failure
  mode entirely.
- **Single-vendor clarity.** Microsoft owns the stack end-to-end — no
  per-GPU vendor behavior matrix, no "works on Intel GL, breaks on AMD GL."
- **Tooling.** PIX for Windows, the Visual Studio Graphics Debugger, and
  `d3d12` debug layers give frame capture / pipeline introspection that GL
  debuggers can't match on this codebase.
- **Maintenance fit.** The Vulkan backend (`render_vk.go`, 8992 lines, uses
  `github.com/Eiton/vulkan` via Cgo) already proves the project can host a
  full Cgo-native-API GPU backend. Direct3D is the same shape of work.

What DirectX does **not** buy us: it's Windows-only, so it does not replace
GL/GLES on Linux/macOS/Android/ARM-handheld Linux — it is a *complementary*
desktop-Windows backend and the new default candidate there.

## 2. How the current architecture is structured

| File | Role |
|------|------|
| `src/render.go` | `Renderer` interface (≈45 methods) + `Texture` interface. Globals `gfx Renderer`, `gfxFont FontRenderer`. Embeds GLSL shaders via `//go:embed`. |
| `src/font.go` | `FontRenderer` interface (`Init`, `LoadFont`) + `Font` interface (`SetColor`/`SetPalFX`/`Printf`/`Width`). |
| `src/render_gl33.go` | OpenGL 3.3 — desktop default. |
| `src/render_gles32.go` | OpenGL ES 3.2 — mobile (`//go:build android || armdevice`). |
| `src/render_vk.go` | Vulkan 1.3 — Cgo to `github.com/Eiton/vulkan`, embeds SPIR-V `.spv`. |
| `src/render_sw*.go` | SDL2 Software — CPU rasterizer, fallback. |
| `src/util_desktop.go`, `util_android.go`, `util_armdevice.go` | `selectRenderer(cfgVal string) (Renderer, FontRenderer)` — build-tagged per platform; string switch on `cfg.Video.RenderMode`. |
| `src/system.go:389` | `gfx, gfxFont = selectRenderer(s.cfg.Video.RenderMode)`. |
| `src/system_sdl.go`, `src/char.go` | Backend-name special cases (see §6). |

`selectRenderer` (desktop) is a plain string switch:

```go
case "OpenGL 3.3":   gfx = &Renderer_GL33{};  gfxFont = &FontRenderer_GL33{}
case "Vulkan 1.3":  gfx = &Renderer_VK{};    gfxFont = &FontRenderer_VK{}
case "SDL2 Software": gfx = &Renderer_SW{};  gfxFont = &FontRenderer_SW{}
default:            gfx = &Renderer_GL33{};  gfxFont = &FontRenderer_GL33{}
```

**Minimum-viable surface.** `render_sw.go` shows what a partial `Renderer`
implementation is allowed to skip — it stubs the model/shadow/cubemap PBR
pipeline:

```go
func (r *Renderer_SW) IsModelEnabled() bool  { return false }
func (r *Renderer_SW) IsShadowEnabled() bool { return false }
// prepareShadowMapPipeline / prepareModelPipeline / RenderShadowMapElements /
// RenderCubeMap / RenderFilteredCubeMap / RenderLUT → empty bodies
```

`model.go` gates all model/shadow/IBL work on `gfx.IsModelEnabled()` /
`gfx.IsShadowEnabled()`, and `render.go:1096` gates the grab pass on
`gfx.NeedsGrabPass()`. **A first DX backend can ship as a 2D-sprite + font
renderer with models/shadows/cubemaps stubbed off**, exactly like `render_sw`
does today, then grow later. This is the single most important scoping fact.

## 3. Which Direct3D version

| | D3D9 | **D3D11** | D3D12 |
|---|---|---|---|
| Min Windows | XP+ | 7+ (all Win10/11) | 10+ |
| Complexity vs. VK backend | lower | **comparable to GLES32** | ≥ Vulkan |
| Shader model | SM2/3 | SM5 (fxc) | SM6 (dxc) |
| Debug tooling | limited | PIX ✓ | PIX ✓ |
| Go binding maturity | `gonutz/d3d9` (old, simple) | best options (§4) | `gogpu/wgpu` D3D12 bindings |
| Worth it | No — too legacy | **Yes — recommended first impl** | Later, if profiling shows D3D11 CPU-bound. |

**Recommendation: D3D11.** Covers every target Windows machine, its
concepts (device/swapchain/PSO/root-signature-lite/recording) map almost
1:1 to what the backends already do, and its shader compiler (`fxc` → SM5)
is a single self-contained `d3dcompiler_47.dll` call. D3D12's gain is
multithreaded command recording and lower-overhead descriptors — worth it
only if D3D11 profiling shows the driver as the bottleneck, which is
unlikely for a sprite-heavy 2D + light-3D workload. D3D9 is too legacy.

## 4. Go binding options (verified, pkg.go.dev)

Four routes, in increasing project-fit:

1. **`github.com/gonutz/d3d9`** — Direct3D 9, thin, ~dead. Skip per §3.
2. **`github.com/deploymenttheory/go-bindings-win32`** —
   `bindings/win32/graphics/direct3d` (plus `…/direct3d/dxc` and
   `…/direct3d/fxc` sub-packages for the shader compilers). v0.2.1, MIT,
   actively published (Jul 2026). Generated Win32 API surface — the
   closest thing to "official" Go D3D bindings, DXC/FXC included.
3. **`github.com/gogpu/wgpu/hal/dx12/d3d12`** — low-level D3D12 COM
   bindings, battle-tested as the WebGPU-on-Windows backend. v0.30.35,
   very recent. Use only if going D3D12.
4. **Raw Cgo with MinGW headers** — the project already builds Cgo with
   the MSYS2/MinGW-w64 toolchain (SDL2, FFmpeg, XMP all built from source
   and linked statically). MinGW-w64 ships `d3d11.h`, `dxgi.h`,
   `d3dcompiler.h`; D3D11 is a system-DLL API (`d3d11.dll`, `dxgi.dll`,
   `d3dcompiler_47.dll`) loaded at link time. No third-party dependency,
   no upstream churn, identical build discipline to SDL2.

**Recommendation: route 4 (raw MinGW Cgo) for the API, with
`deploymenttheory/go-bindings-win32/…/direct3d/fxc` (or direct Cgo to
`D3DCompile`) for shader compilation.** This matches the project's
"build everything from source, link statically, no surprise DLLs"
philosophy and avoids pinning a young third-party binding for the whole
device/swapchain surface. If the raw-Cgo device/swapchain boilerplate
proves painful during prototyping, fall back to `deploymenttheory` for
those calls only.

## 5. Renderer interface → D3D11 mapping

| Renderer method | D3D11 realization |
|---|---|
| `Init` | `CreateDXGIFactory`, `D3D11CreateDevice`, `IDXGISwapChain` (flip-discard, DXGI_FORMAT_R8G8B8A8_UNORM). SDL2 already owns the `HWND` — get it from the existing `Window` via `SDL_GetWindowWMInfo` (sysWMInfo.info.win.window) and pass into `IDXGIFactory::CreateSwapChain`. |
| `Close` | `Release` device/swapchain/context, deferred-destroy queue drain. |
| `BeginFrame` | `OMSetRenderTargets(backbuffer RTV)`, `ClearRenderTargetView`. |
| `EndFrame` | `Present(1/0, 0)` honoring `cfg.Video.VSync`. |
| `Await` | D3D11 is implicitly sequential per context; map to an `ID3D11Fence` wait or a NO-OP for D3D11 (real need only arises on D3D12). |
| `newTexture` | `CreateTexture2D` (D3D11_USAGE_DEFAULT) + `CreateShaderResourceView`; upload via `UpdateSubresource`. |
| `newPaletteTexture` / palette atlas | 256×N R8_UNORM or R8G8B8A8_UNORM atlas, SRV, magicres/point sampler. Same scheme as GL backends. |
| `EnableBlending(eq,src,dst)` | `OMSetBlendState` built from a smallBlendState cache keyed by (eq,src,dst). |
| `EnableScissor` | `RSSetScissorRects` (also assertion: D3D11 scissor testing needs `RSSetState` with `ScissorEnable=TRUE`). |
| `SetUniform*` / `SetTexture` | Constant buffer (UBO analogue) writes; SRV/sampler bind. Mirror `Renderer_VK`'s uniform staging. |
| `SetVertexData` | Per-call `D3D11_USAGE_DYNAMIC` vertex buffer, `IASetVertexBuffers` + `IASetIndexBuffer` + `IASetInputLayout`. |
| `RenderQuad` | Draw therecords, IA topology triangle-strip, 4 verts, `Draw(4,0)`. |
| `RenderElements` | `DrawIndexed`. |
| `LoadCustomSpriteShader` / `SetSpritePipeline` | Create the PS directly from `.cso` bytecode (`CreatePixelShader`); `needsGrabPass` via RDEF parsing (§7.1); cache `ID3D11PixelShader` in a map[shaderName]. "Pipeline" = the (PS, blend, sampler) tuple. |
| `NeedsGrabPass` / `ResolveBackBuffer` | Copy backbuffer to staging (`CopyResource` + `Map`) for readback, return as a `Texture_DX`. |
| `ReadPixels` | Same staging readback path. |
| `PerspectiveProjectionMatrix` / `OrthographicProjectionMatrix` | Pure math (mgl), renderer-agnostic — reuse `render.go` math, just return the matrix into a cbuffer. |
| `prepareModelPipeline` / `SetModelPipeline` / `SetMeshOutlinePipeline` / `SetModelUniform*` / `SetModelTexture` / `SetModelVertexData` / `SetModelIndexData` / `RenderElements` | **Implemented (Phase A+B).** `IsModelEnabled()` is `true`; models render through the real pipeline. SoA vertex layout: slot 0 = sequential vertexId prefix (R32_UINT, `VERTEXID` semantic), slots 1–10 = per-attribute blocks with per-slot stride + byte offset (mirrors GL/VK multi-binding). HLSL port of `model.vert/frag.glsl` (`model_vs.hlsl` / `model_ps.hlsl`, embedded, compiled at init via `D3DCompile`). |
| `prepareShadowMapPipeline` / `RenderCubeMap` / `RenderFilteredCubeMap` / `RenderLUT` / `RenderShadowMapElements` | **Stub (Phase C)** — `IsShadowEnabled()` is `false`, shadow/IBL passes not yet implemented. |
| `IsModelEnabled` | `true` (Phase A+B). |
| `IsShadowEnabled` | `false` until Phase C. |
| `SetVSync` | flip the `Present` interval. |
| `NewWorkerThread` | `false` for D3D11 (immediate-context is single-threaded; MTK would need D3D12). |

`Texture` interface → `Texture_DX`{ `ID3D11Resource`, `ID3D11ShaderResourceView`, width/height/offset/uvst/palSlot/serial } — identical shape to `Texture_VK`.
`FontRenderer` → `FontRenderer_DX`{ `Init`, `LoadFont` } backed by a glyph atlas texture + the ported `font.hlsl`.

## 6. Backend-name special-casing (the two sites that key off `GetName()`)

A DX renderer must declare a stable `GetName()`, e.g. `"Direct3D 11"`, and
the existing name-keyed sites must learn it:

- `src/system_sdl.go:100` — branch on `renderName == "SDL2 Software"` to
  force VSync off (software present stalls). A DX renderer wants **VSync
  on by default** (real GPU present), so it simply falls through the
  existing default branch — no change needed unless DX debug flags are
  added there.
- `src/char.go:3875` — `isVulkan := strings.HasPrefix(gfx.GetName(), "Vulkan")`,
  and if Vulkan it appends `.spv` to user shader paths. A DX renderer needs
  the **same treatment for its bytecode extension**:

  ```go
  isDX := strings.HasPrefix(gfx.GetName(), "Direct3D")
  if isDX && !strings.HasSuffix(strings.ToLower(shaderPath), ".cso") {
      shaderPath += ".cso"
  }
  ```

No other `GetName()` branches exist in `src/` — that's the complete list.
`system.go:450` (`else if strings.HasPrefix(renderName, "Vulkan")`) is a
gamma/ debug toggle; harmless to leave DX out of, optionally extend later.

## 7. Shaders

VK embeds precompiled `.spv`; GL backends embed GLSL source and compile at
runtime. For DX the natural target is **HLSL compiled to bytecode (`.cso`)
offline via `fxc` (SM5) and embedded via `//go:embed shaders/*.cso`**, then
passed to `D3DCreateBlob`/loaded straight into `CreateVertexShader`/
`CreatePixelShader` at init. This matches the VK workflow (precompiled blobs,
no runtime compiler dependency) and stays determinate.

The shaders to port (all already exist as GLSL/SPIR-V under `src/shaders/`):

sprite.vert/frag, sprite_instanced.vert/frag, font.vert/frag,
ident.vert/frag, model.vert/frag (≈20 KB — only for the full-3D phase),
shadow.vert/frag (+ shadow.geo — D3D11 has no geometry-shader-free
point-shadow path; keep the GS), panoramaToCubeMap.frag,
cubemapFiltering.frag.

**MVP needs only: sprite, sprite_instanced, font, ident** (4 vertex + 3
fragment). Port those by hand; keep GLSL semantics (column-major via
`cbuffer`, `SV_Position`, `TEXCOORD`/`COLOR` semantics). HLSL ⇄ GLSL is
near-mechanical for this shader set; the largest is `model.frag.glsl` which
is deferred to the model/shadow phase anyway.

Do **not** transpile GLSL→HLSL at build time. A hand-written HLSL set is
~1 day, auditable, and removes a SPIRV-Cross/glslang toolchain dependency
from the build.

### 7.1 Custom sprite shaders & grab-pass detection

User sprite shaders (the `shaders` block in char defs) ship as precompiled
`.cso` bytecode — `char.go` appends `.cso` for DX, mirroring the `.spv`
treatment for Vulkan. Two things consume them:

- **PS creation.** The bytecode is handed straight to `CreatePixelShader`
  (`dx_create_ps`); `D3DCompile` is only for HLSL *source*, never for `.cso`
  bytes. (An early bug ran the bytecode through `D3DCompile`, which can
  never succeed — the fixed path creates the PS directly from the blob.)
- **Grab-pass detection.** `needsGrabPass` (does the shader sample
  `bgl_RenderedTexture`?) is decided by `dxbcHasResource`, which parses the
  DXBC container's **RDEF** chunk for the resource name instead of
  `bytes.Contains` over the whole blob. `bytes.Contains` was fragile — a
  coincidental match in SHDR/STAT bytecode would false-positive — and
  blind to the resource table entirely.

#### DXBC container header

The container header is not a fixed 20 bytes: modern `d3dcompiler_47`
stores a 16-byte checksum, pushing the chunk-offset array to byte 32.
`dxbcRDEFChunk` tries both layouts and bounds-checks every offset.

| Field | Classic (old fxc) | Modern (d3dcompiler_47) |
|---|---|---|
| Magic | `"DXBC"` @ 0 | `"DXBC"` @ 0 |
| Checksum | 4 bytes @ 4 | 16 bytes @ 4 |
| Version | @ 12 | @ 20 |
| Total size | @ 16 | @ 24 |
| `numChunks` | @ 20 | @ 28 |
| Chunk offsets | @ 24 | @ 32 |

Each chunk is an 8-byte header (fourcc + size) followed by its data; the
RDEF chunk is located by its `RDEF` fourcc.

#### RDEF layouts

The RDEF chunk layout is **shared between modern and classic compilers** —
only the header size differs:

| Offset | Field |
|---|---|
| 0x08 | bound-resource count |
| 0x0c | header size = binding-table start |
| table | 32-byte `D3D11_SHADER_INPUT_BIND_DESC` entries |
| tail | null-terminated names; `Name` is a byte offset relative to the RDEF data start |

Modern `d3dcompiler_47` (Windows 10+) inserts a 32-byte `"RD11"` block at
`0x1c`, making the header 60 bytes for `ps_5_0`/`vs_5_0`; classic fxc (e.g.
`"HLSL Shader Compiler 6.3.9600"`, Windows 8.1 era) has a 28-byte header
with no `RD11` block. Both were verified byte-for-byte against real compiler
output (modern `d3dcompiler_47` 10.x blobs and classic 6.3.9600 `.cso` files
from the community). The parser reads the count and header-size fields
directly, so it does not depend on the `RD11` marker at all.

`rdefTableHasName` walks the binding table, resolves each `Name` offset, and
compares the null-terminated string against `bgl_RenderedTexture`. All reads
are bounds-checked, and the table-fit check uses division so crafted
count/offset fields cannot overflow 32-bit `int` (`ARCH=386`).

`src/render_dx_test.go` pins this down: a real `d3dcompiler_47`-compiled
blob fixture, synthetic modern/classic containers (the classic builder
mirrors the verified 6.3.9600 layout), malformed inputs, and crafted
huge-offset cases.

## 8. Files & integration points

New:
- `src/render_dx.go` — `Renderer_DX`, `Texture_DX`, the D3D11 device/swapchain
  (Cgo). `//go:build windows && !android`. `GetName()` → `"Direct3D 11"`.
- `src/render_dx_font.go` — `FontRenderer_DX`. Same build tag.
- `src/shaders/*.cso` — compiled HLSL bytecodes, embedded.

Touched:
- `src/util_desktop.go` — add `case "Direct3D 11": gfx = &Renderer_DX{}; gfxFont = &FontRenderer_DX{}`.
- `src/char.go:3875` — add `isDX` shader-extension branch (§6).
- `Makefile` — Windows link line: add `d3d11`, `dxgi`, `d3dcompiler` to
  `CGO_LDFLAGS` (system DLLs; MinGW links them dynamically). No new static
  build step (unlike SDL2/FFmpeg/XMP). Optionally a `.cso` build rule:
  `%.cso: %.hlsl` → `fxc /T vs_5_0 /E main /O3 /Fo $@ $<`.
- `deploy/save/config.ini` — document `RenderMode = Direct3D 11`; default
  stays `OpenGL 3.3` until DX is proven, then promote per platform.

No changes to: `render.go`, GL/GLES/VK/SW renderers, the model/shadow/lua
pipeline (all gated behind `IsModelEnabled()`/`IsShadowEnabled()`).

## 9. Phased plan

| Phase | Scope | ≈ Size | Status |
|---|---|---|---|
| **0. Spike** | Cgo D3D11 device + swapchain on an SDL2 HWND; clear to a color; `Present`. Validates the Win32-via-SDL `HWND` extraction and the static-link path. | ~300 LoC, days | **done** — device + flip-discard swapchain created on the SDL2 `HWND`; FXC/D3DCompile path verified; linked against `-ld3d11 -ldxgi -ld3dcompiler` under the Makefile's static-runtime link. |
| **1. MVP renderer** | Sprite + font + palette atlas + custom sprite shaders + blend + scissor + grab pass. `IsModelEnabled/IsShadowEnabled → false`; stub model/shadow/cube/LUT. | ~1.7k LoC | **done** — `render_dx.go` (D3D11 Cgo shim + `Renderer_DX`) + `render_dx_font.go` (`FontRenderer_DX`/`Font_DX`) wired through `util_desktop.go`; 6 HLSL shaders compiled at runtime via `D3DCompile`. Renders an 8-player kfm match at solid 60 FPS on Intel graphics. MSAA (2/4/8) supported; models/shadows/cubemaps stubbed. Sprite batching rides the `flushSpriteQueueBatched` generic fallback like the other non-GL33 backends. Grab-pass detection parses the DXBC RDEF (§7.1). |
| **2. Feature parity** | RenderScale path, MSAA, VSync(-1 auto, 0 off, 1 on), RendererDebugMode → `ID3D11InfoQueue`, ReadPixels, PIX-friendly object naming. | concurrent with P1 | **mostly done** — MSAA (2/4/8), VSync (`Present` interval), ReadPixels (bottom-up, matches GL), ResizeBuffers-on-window-resize all work. Not done: RenderScale (mirrors GL33, which ignores it), InfoQueue debug-name tagging, advanced `ID3D11InfoQueue` filtering. |
| **3. Full 3D** | Port model/shadow/cubemap/IBL HLSL, implement `prepareModel*`/`prepareShadowMap*`/`RenderCubeMap`/`RenderFilteredCubeMap`/`RenderLUT`. Bring `IsModelEnabled/IsShadowEnabled` to `true`. | +3–5k LoC, 2–4 weeks | **A+B done, C–E not started** — A+B implements the core model pass: `model_vs.hlsl`/`model_ps.hlsl` ports, the 11-slot SoA input layout (`VERTEXID` slot 0 + attribute blocks), `SetModelPipeline`/`SetModelUniform*`/`SetModelTexture`/`SetMeshOutlinePipeline`/`SetModelVertexData`/`SetModelIndexData`, model cbuffers, and model-path `RenderElements` (all 11 slots bound). `IsModelEnabled()` → `true`; unlit/base-color materials and morph targets/skinning (joint/morph textures) render. `IsShadowEnabled()` stays `false`; `prepareShadowMapPipeline`, `RenderCubeMap`, `RenderFilteredCubeMap`, `RenderLUT` remain stubs for Phase C. |
| **4. Promote** | Default desktop `RenderMode = Direct3D 11`; keep GL as fallback. | config flip | **not started** — default stays `OpenGL 3.3` until DK11 is proven across more hardware/drivers. |

A realistic "it's the Windows default" milestone is end of Phase 2; Phase 3
only matters once model/IBL scenes are exercised in matches.

## 10. Risks & open questions

- **HWND extraction.** Resolved: `Renderer_DX.Init` reads the SDL `SysWMInfo`
  via `sys.window.GetWMInfo()` and pulls the `HWND` out of the `dummy` union
  field with a `reflect` offset (no SDL header needed in the C shim).
- **fxc HLSL quirk (discovered during P1).** MinGW's `d3dcompiler_47` rejects
  non-trivial scalar splats like `float3((a+b+c)/3.0)` with X3014 — the
  shaders use explicit `float3(s, s, s)` instead. Plain `float3(scalar)` with
  a bare variable name works; only parenthesized expressions trip it.
- **RDEF format varies by d3dcompiler version (discovered during P1).**
  Modern `d3dcompiler_47` (Windows 10+) emits an `"RD11"`-block RDEF
  (60-byte header); older compilers (e.g. fxc `"6.3.9600"`, Windows 8.1
  era) emit the same layout with a 28-byte header and no `RD11` block. Both
  use resource count @ 8 and table start @ 12, so one parser handles both
  (§7.1) — verified byte-for-byte against real compiler output from each
  era.
- **ResizeBuffers `DXGI_ERROR_INVALID_CALL` (0x887a0001) on window resize.**
  D3D11 rejects `ResizeBuffers` while any view of the swapchain back buffer
  is bound to the pipeline — `EndFrame`'s final post pass leaves
  `backbufferRTV` bound, so the next frame's resize failed and retried every
  frame. `checkResize` now unbinds the render target (`OMSetRenderTargets`
  NULL, via `dx_unbind_rt`) before resizing and defers while the window is
  minimized (SDL `WINDOW_MINIMIZED`), retrying once restored.
- **Missing sprite uniforms in `SetUniformF` (discovered during P2).** The
  HLSL `SpriteUniforms` cbuffer declares `x1x2x4x3` and `tint`, but DX's
  `SetUniformF` had no cases for them — `x1x2x4x3` is written by
  `drawQuadsUV` for **every** quad and is required by the trapezoid path
  (`isTrapez=1`) to remap the UV horizontally. With it stuck at
  `(0,0,0,0)` the shader computed `gap = 0` → `uv.x = NaN`, so parallax
  floors (stage `[BG Floor] type = parallax`, e.g. the bundled
  `stageZ.def` / `interactivestage.def`) rendered as a solid black box at
  the characters' feet, and flat-color rects (`tint` via `SetUniformF`,
  used by `[BG Colour]` layers and screenfills) got a stale tint. Fixed by
  adding both cases (mirrors `Renderer_VK.SetUniformF`); covered by the
  `TestDXShadowSilhouette` trapezoid pass, which fails without the fix.
- **Stage-model pass hides sprites/HUD (resolved).** The model pass binds its
  vertex buffer through `dx_set_vb_slots`, which bypasses `bindVB`'s state
  tracker (`state.vb`/`state.vbStride`). The next sprite/font `bindVB(r.vb,
  16)` therefore saw the stale state and early-returned, leaving the MODEL
  vertex buffer bound on input slot 0 — the sprite quad then read the
  model's vertexId prefix as positions and degenerated to a ~1px sliver.
  On `stage3d.def` every sprite (characters, lifebar) vanished while the
  stage background rendered. `RenderElements`' model branch now clears
  `state.vb` before the direct slot binding, and `ReleaseModelPipeline`
  restores `rsDefault`/`dsOff` (the model pass leaves back-face culling +
  depth-test states bound, and the sprite path only re-binds the rasterizer
  through the scissor helpers, which early-return when the scissor state is
  unchanged — the font/flat paths would otherwise inherit the model's cull
  state). Verified by `TestDXSpriteAfterModel`, which reproduces the exact
  model-pass → sprite-pass frame flow headlessly.
- **3D-model stages render black (resolved for unlit stages in Phase A+B).**
  Stage backgrounds that are entirely a GLB/glTF model (`[BGdef] model = …`,
  e.g. the bundled `stage3d.def`) now render through the implemented model
  pipeline — `IsModelEnabled() → true`, and the unlit-material path
  (`matMisc.z = 1`, no lights, no environment) produces base color instead of
  a black box. Verified by the headless `TestDXModelPipeline` probe, which
  compiles the embedded model shaders, builds the 11-slot SoA input layout,
  and draws a triangle through the real `SetModelPipeline`/`RenderElements`
  chain. Stages that need shadow maps / IBL (lit PBR materials, `[Light]` /
  `[Environment]` sections) still require Phase C.
- **Legacy d3dcompiler HLSL quirks (discovered during Phase A+B).** MinGW's
  `d3dcompiler_47` rejects scalar-splat constructors (`float3(1.0)`,
  `float3(expr)`) with X3014 — the model shaders spell out all components
  (`float3(1.0, 1.0, 1.0)`). It also normalizes semantics: `JOINTS0`/`JOINTS1`
  compile to semantic name `JOINTS` with index 0/1 (same for `WEIGHTS`), so
  the model input layout must declare `JOINTS`/`WEIGHTS` with `SemanticIndex`
  rather than the literal `JOINTS0`/`JOINTS1` names or
  `CreateInputLayout` fails with `E_INVALIDARG`.
- **Static link.** SDL2/FFmpeg/XMP are built from source and static-linked.
  D3D is a Windows system API — `d3d11.dll`/`dxgi.dll`/`d3dcompiler_47.dll`
  stay dynamic (they're OS components, not redistributable libs). Document
  this as a deliberate exception to the "fully static binary" rule.
- **ARM64 Windows.** D3D11 + `d3dcompiler_47.dll` exist on Windows-on-ARM
  (used by the Surface line), so `ARCH=arm64` builds should work; untested.
  Needs a smoke test before claiming ARM-Windows support.
- **Shader parity test.** Port `render_sw_test.go`'s byte-exact blend
  tests: render a known sprite set with DX and diff the framebuffer against
  the GL33 reference at the same resolution. Catches HLSL-translation
  rounding bugs early (e.g. `mul`/`mad` precision, `saturate` vs manual clamp).
- **D3D12 escape hatch.** If D3D11 profiling shows the immediate context
  as a CPU bottleneck under heavy sprite batching, the natural next step
  is D3D12 with `gogpu/wgpu`'s bindings — reusing the HLSL and the
  `Renderer_DX` type skeleton. Out of scope for this proposal.
- **`AGS`/vendor extensions** (AMD/NVIDIA) — not needed for parity; skip.

## 11. TL;DR

Direct3D 11 via raw MinGW Cgo + `D3DCompile` (runtime HLSL compilation),
scaffolded as `render_dx.go` behind `//go:build windows && !android`, wired
through `util_desktop.go` as `RenderMode = Direct3D 11`. Phases 0–1 are done;
Phase 3 A+B is done (core model pipeline): `model_vs.hlsl`/`model_ps.hlsl`
ports, the 11-slot SoA input layout (`VERTEXID` prefix slot + per-attribute
blocks), `SetModelPipeline`/`SetModelUniform*`/`SetModelTexture`/`SetModelIndexData`
and the model-path `RenderElements`, so GLB stage backgrounds render
(`stage3d.def`-style, unlit path) — verified by `TestDXModelPipeline`. Custom
user sprite shaders follow the Vulkan pattern (`.cso` bytecode,
`NeedsGrabPass` detected by parsing the DXBC RDEF section — one parser
handles both the modern `RD11` and classic 28-byte-header layouts, §7.1).
External post shaders are also `.vert.cso`/`.frag.cso`. The remaining work
is Phase 3 C–E (shadow maps, cubemap/IBL), Phase 2 polish, and Phase 4
default promotion.