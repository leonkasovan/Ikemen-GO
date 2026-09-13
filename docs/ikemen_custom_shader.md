# Custom Sprite Shaders (stage, characters, explods, projectiles)

This is the single guide for Ikemen-GO custom sprite shaders: the `.def`
`[Shaders]` loader, the state controllers that apply them, fragment-shader
authoring, and the Shadertoy → Ikemen conversion workflow. It merges the former
`docs/custom_shader.md` (character side) and `docs/shadertoy_conversion.md`
(stage side).

Feature introduced in PR #3551 ("feat: custom shader") and since extended with
per-object lifetimes, extra shader textures (`tex1`/`tex2`) and projectile
support. This document describes the **current** behavior.

Source of truth:

* `src/char.go` — `[Shaders]` loader in `(*Char).load`, `Char.customShader`,
  `Explod.customShader`, `Projectile.customShader`, per-tick stepping in
  `Char.update` / `Explod.update` / `Projectile.update`
* `src/stage.go` — stage-side `[Shaders]` loader, `backGround.shader` /
  `shaderParams` per `[BG n]`
* `src/bytecode.go` — `shaderSet`, `explod`, `modifyExplod`, `projectile`,
  `modifyProjectile` state controllers; `OC_ex2_shader` trigger
* `src/compiler_functions.go` — `shaderSub` (shared `shader` / `shaderparam.p*` /
  `shadertex1.*` / `shadertex2.*` parameter parser)
* `src/render.go` — `ShaderTexture`, `CustomShader`, `CustomShaderRenderData`,
  uniform upload in the sprite draw path
* `src/render_gl33.go`, `src/render_gles32.go`, `src/render_vk.go`,
  `src/render_dx.go` — `LoadCustomSpriteShader` / `UnloadCustomSpriteShader`
  per backend
* `src/system.go` — `isValidCustomShader`, `cleanCustomShaders`, `shaderRefCount`
* `deploy/stages/*.frag` — working example shaders (screenpack)

## 1. Concepts and data flow

A *custom shader* is a fragment shader that replaces the default sprite shader
for one draw. It is loaded once per alias and referenced by name:

```
[Shaders] in .def ──► gfx.LoadCustomSpriteShader(alias, bytes)      (loaded once, main thread)
                           │
State controllers ──► CustomShader{name, params[16], time, sTime, tex1, tex2}
  (ShaderSet, Explod, Projectile, Modify*)    │ per draw
                           ▼
        CustomShaderRenderData{name, params, time, sTime, tex1, tex2}  (SpriteData.customShader)
                           ▼
        renderer switches sprite pipeline to the custom program,
        sets uniforms, draws, then restores the default sprite shader
```

Where a custom shader can be applied:

| Target | Where you set it | Notes |
|---|---|---|
| Stage background element | `[BG n]` `shader = "alias"` (+ `shaderparam.p*`) | See section 3.1. |
| The character itself (body sprite) | `ShaderSet` | Includes helpers (`redirectid` supported). Shadow and reflection reuse the same render data. |
| Explod | `Explod`, `ModifyExplod` | `shadertime` duration; cleared when the explod is removed. |
| Projectile | `Projectile`, `ModifyProjectile` | `shadertime` duration; cleared when the projectile is removed. |

An explod created with `syncparams = 1` also inherits the owner's
`customShader` (name, params, textures) along with the other synced params.

## 2. Loading shaders — the `[Shaders]` section

Declare shaders in a `.def`. Characters use their own `.def` (also accepted in
localized `[Shaders]` sections via the usual `lan` prefix handling); stages use
the stage `.def`:

```ini
[Shaders]
kfm_zss_distortion = distortion.frag
rain               = rain_glass.frag
```

* The **left side is the alias** used from CNS/ZSS/`[BG n]`; the **right side
  is the file path**. Aliases are matched case-insensitively (ini keys and
  names in state controllers are lowercased by the engine).
* File resolution (`LoadFile`): character `.def` directory, working directory,
  `data/`. For stages additionally the stage `.def` dir and `sys.motif.Def` are
  tried first. Optional surrounding quotes are allowed.
* Backend suffix is appended automatically by the loaders
  (`(*Char).load`, `readBackGround`):
  * Vulkan → `.spv` (SPIR-V, required)
  * Direct3D → `.cso` (precompiled DXBC)
  * OpenGL / GLES → the `.frag` GLSL source is used as-is
  Ship `foo.frag` plus `foo.frag.spv` / `foo.frag.cso` under the same basename
  to support all backends.
* Files are read on the loader thread but compiled on the main thread
  (`sys.mainThreadTask`). Load/compile failures are logged to the console
  (e.g. `[GL Error] Failed to compile custom shader ...`).
* Lifetime management: each load marks the alias in `sys.shaderRefCount`
  (count 3). `Loader` calls `cleanCustomShaders()` after every load cycle; an
  alias no longer declared by any char or the stage is unloaded after the count
  drains. In practice this means shaders are freed a few load cycles after the
  last user is gone.

**Namespaces are global.** All custom shaders (chars + stage) share one alias
map, and aliases remain usable by the entire character. Prefix aliases with the
character name (`kfm_zss_…`) to avoid collisions with other content.

## 3. Applying shaders

### 3.1 Stage backgrounds (`[BG n]`)

```ini
[BG 0]
type = normal
spriteno = 0, 0
start = 0, 0
delta = 0, 0
mask = 0
shader = "rain"
;shaderparam.p0 = 1.0
;shaderparam.p1 = 0.2
```

* `shader = "alias"` — quotes stripped, lowercased. `shaderparam.p0` …
  `shaderparam.p15` are parsed case-insensitively into the BG's param array;
  unset params are `0`. There is no per-BG lifetime or `shadertex*` — the
  shader stays active while the BG exists, and stage shaders get no `tex1`/
  `tex2` and a constant `sTime = 0`.
* The draw call passes `CustomShaderRenderData{name: bg.shader, params: bg.shaderParams}`.

### 3.2 ShaderSet (the character's own sprite)

```ini
[State 200, distortion]
Type     = ShaderSet
Trigger1 = Time = 5
shader   = "kfm_zss_distortion"
shaderparam.p0 = 2.0
shaderparam.p1 = gametime/60.0
shadertex1.anim = 196        ; optional extra texture: animation number
;shadertex1.spr  = 10, 0     ; or: sprite group,number
;shadertex2.anim = 197       ; optional second extra texture
time     = 120               ; ticks; omit = -1 (infinite)
;redirectid = 1              ; apply to another char (e.g. a helper) instead
```

| Parameter | Meaning |
|---|---|
| `shader` | Alias from `[Shaders]`, **must be quoted**. Empty string (`""`) removes the shader. Unregistered names are rejected with a console warning. |
| `shaderparam.p0` … `shaderparam.p15` | Float uniforms, each an expression evaluated at run time. Unset params stay `0`. |
| `shadertex1.anim` / `shadertex1.spr` | Bind one of the char's own animations/sprites as `tex1` (see section 4). |
| `shadertex2.anim` / `shadertex2.spr` | Same for `tex2`. |
| `time` | Duration in ticks. Default `-1` = until cleared. `0` clears the whole shader (name, params, textures). |
| `redirectid` | Target a different char. |

Duration note: `ShaderSet.time` defaults to infinite. The shader survives state
changes and is only cleared when `time` runs out, or on round resets
(`clearState`/`prepareNextRound`). While the char is in hitpause the countdown
pauses and `sTime` stops advancing.

### 3.3 Explod / ModifyExplod

```ini
[State 200, fx explod]
Type          = Explod
Trigger1      = Time = 0
anim          = 196
removetime    = 60
shader        = "kfm_zss_wobble"
shaderparam.p0 = 0.5
shadertime    = 60              ; omit = -1 (infinite, removed with the explod)

[State 200, change fx]
Type           = ModifyExplod
Trigger1       = 1
ID             = 192
shader         = "kfm_zss_disintegrate"
shaderparam.p0 = gametime/60.0
```

Same sub-parameters as `ShaderSet` (`shader`, `shaderparam.p*`,
`shadertex1.*`, `shadertex2.*`) plus `shadertime`. For `ModifyExplod`, if the
explod had no shader yet and no `shadertime` is given, duration defaults to
infinite; `shadertime = 0` clears it.

### 3.4 Projectile / ModifyProjectile

`Projectile` and `ModifyProjectile` accept the identical set (`shader`,
`shaderparam.p*`, `shadertex1.*`, `shadertex2.*`, `shadertime`), applied to
every projectile matched by the controller.

### 3.5 The `shader` trigger

CNS/ZSS boolean trigger (compiled through the same quoted-string path as
`helpername` etc.):

```ini
[State 200, end]
Type  = ShaderSet
Trigger1 = shader = "kfm_zss_distortion"   ; active?
Trigger2 = shader != "kfm_zss_wobble"      ; not this one
Trigger3 = shader = ""                     ; none active
```

The comparison is against the *lowercased* alias currently active on the char.
In Lua debug scripts, `shader("name")` tests the debug char's shader and bare
`shader` (no argument) tests "any shader active".

## 4. Extra shader textures (tex1 / tex2)

Each character-side shader slot can carry up to two auxiliary textures sampled
in the fragment shader as `tex1` and `tex2` (stage BGs cannot set these). They
can be an animation (auto-advanced every tick the effect is active via
`ShaderTexture.step()`) or a single sprite:

```ini
shadertex1.anim = 196    ; use anim 196 (advances each tick, loops)
shadertex1.spr  = 10, 0  ; or a static sprite group,number
```

* Textures are resolved from the owning char (`getSelfAnimSprite` / the char's
  SFF), so `ownpal`-style considerations do not apply — the raw sprite texture
  is bound.
* At draw time the engine uploads them under sampler names `tex1`/`tex2`
  (Vulkan bindings 5 and 6). When unset, Vulkan binds a dummy texture; on
  GL/GLES the sampler simply reads as black — guard with a `p*` flag or always
  assign one.
* Paletted sprites: `ensureTex()` uploads the sprite as an 8-bit texture; the
  palette for `tex`/`pal` applies to the *main* sprite, not to `tex1`/`tex2`,
  which behave like RGBA sprite textures.

Typical use: a noise/flow-map animation feeding a distortion shader.

## 5. Writing the fragment shader

Custom shaders are **fragment-only** — the engine supplies the vertex shader.
Copy the skeleton below (dual GL/Vulkan header, as in `deploy/stages/*.frag`
from the screenpack). Do not invent bindings; Vulkan expects exactly:

| Binding | Content |
|---|---|
| 0 | vertex UBO (engine) |
| 1 | fragment UBO: `x1x2x4x3, tint, add, mult, alpha, gray, hue, mask, isFlat, isRgba, isTrapez, neg, iTime, iResolution, aspectRatio, sTime` |
| 2 | `tex` — the sprite being drawn |
| 3 | `pal` — palette texture (paletted sprites) |
| 4 | `bgl_RenderedTexture` — grab pass (only if declared) |
| 5 | `tex1` |
| 6 | `tex2` |

Push constants (Vulkan, std430, offset 16 after `palUV`): `p0`…`p15`.

```glsl
//When converting to SPV, specify version 450
//#version 450 core

#if __VERSION__ >= 450
	#define COMPAT_TEXTURE texture
	layout(binding=1) uniform UniformBufferObject{
		vec4 x1x2x4x3; vec4 tint; vec3 add; vec3 mult; float alpha, gray, hue; int mask; bool isFlat, isRgba, isTrapez, neg;
		float iTime; vec2 iResolution; float aspectRatio; float sTime;
	};
	layout(push_constant, std430) uniform u{
		vec4 palUV; float p0,p1,p2,p3,p4,p5,p6,p7; float p8,p9,p10,p11,p12,p13,p14,p15;
	};
	layout(binding=2) uniform sampler2D tex;
	layout(binding=3) uniform sampler2D pal;
	// + the next line ONLY for GrabPass shaders:
	// layout(binding=4) uniform sampler2D bgl_RenderedTexture;
	layout(binding=5) uniform sampler2D tex1;   // only if you use shadertex1
	layout(binding=6) uniform sampler2D tex2;   // only if you use shadertex2
	layout(location=0) in vec2 texcoord;
	layout(location=0) out vec4 FragColor;
#else
	#define COMPAT_VARYING in
	#define COMPAT_TEXTURE texture
	#ifdef GL_ES
		precision highp float; precision highp int;
	#endif
	out vec4 FragColor;
	uniform sampler2D tex; uniform sampler2D pal;
	// uniform sampler2D bgl_RenderedTexture; // ONLY for GrabPass shaders
	uniform sampler2D tex1; uniform sampler2D tex2;
	uniform vec4 x1x2x4x3; uniform vec4 tint; uniform vec4 palUV; uniform vec3 add, mult;
	uniform float alpha, gray, hue; uniform int mask; uniform bool isFlat, isRgba, isTrapez, neg;
	uniform float p0,p1,p2,p3,p4,p5,p6,p7; uniform float p8,p9,p10,p11,p12,p13,p14,p15;
	uniform float iTime; uniform vec2 iResolution; uniform float aspectRatio; uniform float sTime;
	COMPAT_VARYING vec2 texcoord;
#endif

void main(){
	vec2 fragCoord = gl_FragCoord.xy;
	// ... effect here, sample the sprite with texture(tex, texcoord)
	// (or the pal / GetIkemenPixel pattern from deploy/stages/*.frag
	//  for paletted/PalFX-correct sampling), end with:
	FragColor = vec4(col, 1.0);
}
```

SPV/CSO note: the first two lines are the convention — keep `//#version 450
core` commented for GL, enable/specify version 450 when compiling to `.spv`.

### Engine-provided values

| GLSL name | Value | Notes |
|---|---|---|
| `iTime` | seconds since match start (`gameTime/60`), or `frameCounter/60` outside a match | Global clock. Animate with this, optionally scaled by a `p*`. |
| `sTime` | float frame counter of **this** effect instance | Char-side: starts at 0 when the shader is applied; freezes during hitpause (chars) / pause (explods). Stage BGs always see `0`. Prefer over `iTime` for per-sprite loops. |
| `iResolution` | `vec2` backbuffer size in pixels (`sys.scrrect[2], sys.scrrect[3]`) | Base all `gl_FragCoord` math on it. |
| `aspectRatio` | current aspect / fight aspect | Aspect-ratio correction. |
| `p0`…`p15` | `shaderparam.p*` values | Unset = `0`; write `(p0 != 0.0) ? p0 : default`. |
| `tex`, `pal`, `palUV`, `tint`, `add`, `mult`, `alpha`, `gray`, `hue`, `mask`, `isFlat`, `isRgba`, `isTrapez`, `neg`, `x1x2x4x3` | current sprite/PalFX state | Same meaning as the default sprite shader. |
| `texcoord` | 0–1 sprite UV | The input the quad provides. |
| `tex1`, `tex2` | auxiliary textures from `shadertex1.*` / `shadertex2.*` | Char-side only; see section 4. |
| `bgl_RenderedTexture` | resolved backbuffer | Only if declared; see GrabPass below. |

Not available: `iMouse`, `iDate`, `iFrame`, `iTimeDelta`, `iChannel0-3`,
`iSampleRate` — see section 6 for replacement patterns.

### GrabPass (backbuffer sampling)

Declaring `bgl_RenderedTexture` **anywhere in the shader text** — including a
comment — flags the program as needing a grab pass
(`needsGrabPass = strings.Contains(fragSource, "bgl_RenderedTexture")` on
GL/GLES; the compiled blob is scanned the same way on Vulkan/DX). When active,
the engine resolves the backbuffer every frame before the draw, so use it only
when you actually need to distort background content:

```glsl
vec2 suv = gl_FragCoord.xy / iResolution;
vec3 bg  = texture(bgl_RenderedTexture, suv + 0.02*sin(suv*20.0 + iTime)).rgb;
FragColor = vec4(bg, alpha);
```

The grab pass only contains what was drawn **before** this sprite, so on
`layerno = 0` background elements the buffer may be nearly empty (per
`shader_example.def`: GrabPass only distorts content drawn *under* the shadered
sprite).

### Three shader kinds (pick one)

1. **Procedural fullscreen** (`icy_moon.frag`, `impact_planet.frag`, `fbm_clouds.frag`,
   `rain_glass.frag`): ignore `tex`/`GetIkemenPixel`, compute from `gl_FragCoord` +
   `iTime` + `p*`. Omit `GetIkemenPixel`/hsv helpers to stay short.
2. **Sprite distort** (`distortion.frag`, `disintegrate.frag`, `planar_clouds.frag`):
   perturb `texcoord`, sample via `GetIkemenPixel(uv)` (handles flat/RGBA/paletted,
   trapez, PalFX, tint). Copy that function verbatim.
3. **GrabPass / background distort** (`wobble.frag`): declare `bgl_RenderedTexture`
   (binding 4). The substring alone triggers backbuffer resolve
   (`src/render_gl33.go:2599`, `src/render_vk.go:8281`, `src/render.go:1096-1101`).
   Sample with screen UV: `vec2 suv = gl_FragCoord.xy / iResolution;`
   `texture(bgl_RenderedTexture, distorted_suv)`.

## 6. Shadertoy → Ikemen mapping table

| Shadertoy | Ikemen replacement |
|---|---|
| `void mainImage(out vec4 c, in vec2 f)` | `void main(){ vec2 fragCoord = gl_FragCoord.xy; … FragColor = …; }` |
| `iResolution.xy` | same (engine-provided). `uv = (fragCoord - 0.5*res)/res.y` pattern kept. |
| `iTime` | same, optionally scaled: `float tt = iTime * ((p0!=0.0)?p0:1.0);` (`rain_glass.frag`, `fbm_clouds.frag`, `icy_moon.frag:230`). |
| `iMouse` | **No equivalent.** Use `p1…p15` knobs; `0,0` = auto-drift fallback. Precedent: `impact_planet.frag:212` (`p4` = mouseX 0..1, 0 = auto), `rain_glass.frag` (`p1/p2` = sphere XY, `p3` = radius). |
| `iChannel0-3` + `textureLod` | Procedural hash replacement. Precedent: `impact_planet.frag:67-74` (`shHash2` instead of `iChannel0` texture fetch). |
| `iFrame`, `iTimeDelta`, `iDate` | Derive from `iTime`/`sTime`, or drop. |
| `fragColor` output | `FragColor` (capital F). |
| `texcoord` (0..1 sprite UV) | Only for sprite-distort shaders (`distortion.frag:152-174` offsets `texcoord`, samples `GetIkemenPixel(sampleUV)`). Fullscreen procedural shaders ignore it and use `gl_FragCoord`. |

Hash helpers (`hash11/hash21`, `shHash1/shHash2`, `ctHash`) collide with builtins on
some drivers — rename with a file prefix if you get overload errors
(`fbm_clouds.frag:84`, `impact_planet.frag:53`).

## 7. Worked example: `rain_glass.frag`

Original: rain + drips + mouse sphere. Converted 1:1 except:

* `mainImage` inlined into `main()`, `fragCoord = gl_FragCoord.xy`.
* `iTime` → `tt` with `p0` timescale.
* Mouse branch replaced:
  ```glsl
  vec2 c;
  if (p1 != 0.0 || p2 != 0.0) c = vec2(p1, p2);
  else c = vec2(0.34*sin(tt*0.55), 0.13*cos(tt*0.8));
  float rad = (p3 != 0.0) ? p3 : 0.20;
  ```
* Documented in header: `p0=timeScale, p1=sphereX, p2=sphereY, p3=radius`,
  plus `// ponytail: 304 taps/pixel, lower DROPS/DRIPS if slow`.

## 8. Backend notes and limitations

* **SDL2 Software renderer**: custom shaders are a no-op — the loader and
  setters exist but do nothing; draws fall back to the default look.
* **OpenGL 3.3 / GLES 3.2**: `.frag` GLSL source compiled at load; uniforms
  `p0`…`p15` are floats looked up by name (unused ones are simply not set).
* **Old Mali GPUs (Bifrost r13, e.g. RK3326/R36S)**: the driver silently
  renders programs with **zero active texture samplers as black** — compile
  and link succeed, draws issue without GL errors, mapping shows `-1`s (all
  normal). Fully procedural frags (`clouds.frag` before the fix) hit this.
  Keep one non-constant-foldable sample under `#ifdef GL_ES` (see
  `deploy/stages/clouds.frag` keep-alive tail). Note `t * 0.0` does **not**
  count — the compiler folds it away and drops the sampler. The engine logs
  `WARNING: custom shader … performs no texture sampling` at load when the
  frag source contains no sampling call.
* **Vulkan**: `.spv` (SPIR-V) is **required**; the loader appends `.spv` to the
  path. `p0`…`p15` arrive via push constants (fragment stage, offset 16, 64
  bytes). Unset `tex1`/`tex2` bind a dummy texture to avoid descriptor errors —
  keep bindings 5/6 declared in the SPIR-V even if unused.
* **Direct3D 11**: `.cso` (precompiled DXBC) is required; the loader appends
  `.cso`. Each `p` is stored as a `float4(p,0,0,0)` in the constant buffer.
* A draw with a shader name that fails to resolve (unloaded, failed compile)
  silently falls back to the default sprite shader
  (`SetSpritePipeline` keeps the current program).
* Custom-shader draws bypass sprite batching (they run on the immediate path,
  like 3D-projection sprites) and restore the default program afterwards. Many
  shadered sprites on screen = many program switches + potential grab passes;
  keep effects cheap and grab passes rare.
* GrabPass costs a backbuffer resolve every frame the shader is drawn — only
  declare `bgl_RenderedTexture` where you truly need background distortion.

## 9. Checklist before shipping

* [ ] Alias is lowercase-safe, unique (prefix with char name), listed once in `[Shaders]`, quoted where used.
* [ ] `foo.frag` shipped for GL/GLES; `foo.frag.spv` and `foo.frag.cso` present for Vulkan/DX (loader appends extensions automatically).
* [ ] Dual `#if __VERSION__ >= 450` header, bindings 1–6 correct, no invented uniforms.
* [ ] No `iMouse`/`iChannel`/`mainImage` leftovers (a comment mentioning them is fine).
* [ ] `bgl_RenderedTexture` declared only when needed (a stray comment enables the grab pass!).
* [ ] Procedural (sampler-less) frags carry the `#ifdef GL_ES` sampler keep-alive (old Mali renders them black otherwise).
* [ ] Every used `p*` documented in the shader header and defaulted: `(pX != 0.0) ? pX : dflt`.
* [ ] `tex1`/`tex2` either always assigned from the state controller or handled as black in the shader.
* [ ] `shadertime`/`time` semantics chosen deliberately: omit = infinite, `0` = clear.
* [ ] Effect verified with `config=debug` console: look for compile errors and `Loaded Custom Shader: <name> (NeedsGrabPass: …)` lines.
* [ ] Performance annotated (tap/step counts) — custom-shader draws are never batched.

## Cross-references

* `docs/ikemen_cns_scripting.md` — `ShaderSet` state controller and the `shader` trigger reference
* `docs/ikemen_zss_scripting.md` — ZSS controller list (incl. `ShaderSet`)
* `docs/ikemen_lua_scripting.md` — Lua `shader([name])` trigger
* `deploy/stages/*.frag` — working example shaders from the screenpack