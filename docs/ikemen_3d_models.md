# 3D Models (glTF 2.0) — stages, screenpack, storyboards

Ikemen-GO can display 3D models in stages, the screenpack and storyboards. Models
use the glTF 2.0 format (`.glb` or `.gltf`), which can be exported from Blender
and most other modelling programs. This document describes the **current**
behavior; it is the local counterpart of the
[3D Model features wiki page](https://github.com/ikemen-engine/Ikemen-GO/wiki/3D-Model-features),
verified against the source.

Reference files in the repo: `deploy/stages/stage3d.def` (+ `stage3d.glb` /
`stage3d.sff`) and `deploy/stages/stage3d_outline.def` (+ `stage3d_outline.glb` /
`stage3d_outline.sff`). `stage3d_outline.def` is the same stage with the mesh
outline workflow applied (see section 6).

Source of truth:

* `src/model.go` — the glTF loader (`loadglTFModel`), node/material/animation
  `extras` parsing, `Model.draw` / `Model.drawShadow`, `Model.step`
* `src/stage.go` — `[BGdef] model =`, `[Model]` section, `[BGCTRLDEF]` /
  `[BGCTRL]` / `[BGCTRL3D]`, `Stage.drawModel`, `modifyBGCtrl3d`
* `src/bgdef.go` — screenpack `BGDef`: `scenenumber`, `modeloffset`,
  `modelrotate`, `modelscale`, `fov`/`near`/`far`
* `src/motif.go` — screenpack `[Files] model =` (loaded into the `BGDef`s)
* `src/storyboard.go` — storyboard `[SceneDef] model =`
* `src/camera.go` — stage camera defaults (`fov: 40, near: 0.1, far: 10000`)
* `src/resources/defaultConfig.ini` — `Video.EnableModel`, `Video.EnableModelShadow`

## 1. Overview & enabling

* Models are supported on the OpenGL 3.3, GLES 3.2, Vulkan and Direct3D 11
  renderers. The SDL2 software renderer ignores them (`IsModelEnabled() == false`).
* `[Video] EnableModel = 1` (default) toggles 3D model support globally;
  `EnableModelShadow = 1` (default) toggles model shadow mapping.
* A model draws in the same pass as stage background layers (section 9), can
  receive PalFX (the stage/screenpack `PalFX` applies via the model's own
  `PalFX` state) and is shadow-cast when `EnableModelShadow` is on.
* Textures: PNG / JPEG / embedded image data are recommended. Compressed DDS
  textures are not supported (see `[BGdef]` comment in `stage3d.def`).
* Make sure the **world origin** of the model sits at the camera's starting
  point (`stage3d.def`).

## 2. Where to declare a model

### 2.1 Stage — `[BGdef]` in the stage `.def`

```ini
[BGdef]
; Filename of the 3D model file (gltf/glb)
model = stage3d.glb

; .sff file, required by spec (and for stage portraits)
spr = stage3d.sff
```

File resolution uses the same `LoadFile` search paths as other stage assets
(stage `.def` dir, `""`, `data/`). The model is loaded on the loader thread and
uploaded to the GPU on the main thread. If the stage declares a model, the stage
is treated as Ikemen 1.0+ content.

### 2.2 Screenpack — `[Files]` in `motif.def`

```ini
[Files]
model = mymotif.glb
```

The screenpack model is shared by the screenpack's `BGDef`s (titlebg, selectbg,
versusbg, …) and is uploaded to GPU buffer slot 1. Each `BGDef` decides whether
and how to draw it via its `<bgname>def` section (section 9).

### 2.3 Storyboard — `[SceneDef]` in a storyboard

```ini
[SceneDef]
model = intro.glb
```

Scenes whose `<bg>def` section sets a `scenenumber` (section 9) draw the
storyboard model.

## 3. Stage reference: `stage3d.def` / `stage3d_outline.def`

### 3.1 `[Camera]` — the 3D projection

```ini
[Camera]
; Field of view (degrees). 30 is good for photorealistic scenes.
fov = 30
```

`fov` (degrees, default 40) is the vertical field of view used for the model's
perspective projection; `near` (default 0.1) and `far` (default 10000) are the
depth-clip planes. `fov` is also used by the `[Model] offset` → camera
`zoffset` math (below).

### 3.2 `[Model]` — placement and environment

```ini
[Model]
; X, Y, Z coordinates of the model relative to the camera
offset = 0,-0.2,-0.9

; Scale of the model
scale = 0.05, 0.05, 0.05
```

* `offset` — model position in stage units, relative to the camera start. The
  engine uses `offset[2]` (depth) and `offset[1]` (height) to derive the stage's
  `zoffset` automatically, so the floor lines up with the 2D floor. The model's
  world origin should be at the camera's starting point.
* `scale` — non-uniform XYZ scale of the whole model (default `1, 1, 1`).
  `stage3d.glb` is authored in metres, hence `0.05, 0.05, 0.05` (the standard
  MUGEN "one unit ≈ 20 px" convention).
* `environment` — optional HDR environment map file (`.hdr` / RGBE) used for
  image-based lighting of PBR materials. `environmentintensity` scales it
  (default 1). When present, the engine renders the HDR into a cube map +
  Lambertian + GGX prefiltered maps at load time.

`stage3d_outline.def` is identical except it loads `stage3d_outline.glb` (the
same scene with `meshOutline` extras) and sets `zoomout = .8` (see
`docs/ikemen_cns_scripting.md` for camera zoom semantics).

## 4. Standard glTF features supported

Refer to the glTF™ 2.0 Specification for the full set of features; the subset
Ikemen-GO implements:

* `COLOR_n` — **only `COLOR_0`** (per-vertex color; ubyte/ushort/float, 3 or 4
  components).
* `TEXCOORD_n` — **multiple texture coordinate sets are not supported**; all
  textures use `TEXCOORD_0`.
* `JOINTS_n` / `WEIGHTS_n` — up to **2 joint sets** (`JOINTS_0`/`WEIGHTS_0` and
  `JOINTS_1`/`WEIGHTS_1`) per primitive. A joint set without its matching
  `WEIGHTS_n` is a load error ("Primitive attribute JOINTS_n is specified but
  WEIGHTS_n is not specified."). Both sets are consumed by the model and
  shadow pipelines, so meshes can weight vertices to up to 8 groups.
* **Morph targets** — up to **8 active morphed attributes** per primitive
  (POSITION/NORMAL/TANGENT/TEXCOORD_0/COLOR_0 deltas), driven by node/mesh
  `weights` or animation channels.
* Primitive modes: points, lines, line loop, line strip, triangles, triangle
  strip, triangle fan.
* Materials — the PBR metallic-roughness model: `baseColorFactor`,
  `metallicFactor`, `roughnessFactor`, `baseColorTexture`,
  `metallicRoughnessTexture`, `normalTexture`, `occlusionTexture` (+
  `strength`), `emissiveFactor` / `emissiveTexture`, `alphaMode`
  (OPAQUE/MASK/BLEND), `alphaCutoff`, `doubleSided`.

## 5. Supported glTF extensions

| Extension | Notes |
|---|---|
| `KHR_animation_pointer` | Animate any supported JSON pointer (table below). |
| `KHR_lights_punctual` | Point / spot / directional lights; **up to 4 light sources** per scene. |
| `KHR_materials_unlit` | Unlit (emissive-only) material rendering. |
| `KHR_texture_transform` | `offset` / `rotation` / `scale` on any texture info (also animatable via `KHR_animation_pointer`). |

### 5.1 `KHR_animation_pointer` targets

Animations may target node TRS, morph weights, material factors and lights via
`KHR_animation_pointer` on the channel target. Supported pointers (all others
are rejected with a console error):

| JSON pointer | Animatable value |
|---|---|
| `/materials/{i}/pbrMetallicRoughness/baseColorFactor` | vec4 (AnimVec4) |
| `/materials/{i}/pbrMetallicRoughness/metallicFactor` | float |
| `/materials/{i}/pbrMetallicRoughness/roughnessFactor` | float |
| `/materials/{i}/pbrMetallicRoughness/extensions/KHR_texture_transform/{offset,scale,rotation}` | vec2 / vec2 / float |
| `/materials/{i}/normalTexture/extensions/KHR_texture_transform/{offset,scale,rotation}` | vec2 / vec2 / float |
| `/materials/{i}/occlusionTexture/strength` | float |
| `/materials/{i}/occlusionTexture/extensions/KHR_texture_transform/{offset,scale,rotation}` | vec2 / vec2 / float |
| `/materials/{i}/emissiveFactor` | vec3 |
| `/materials/{i}/emissiveTexture/extensions/KHR_texture_transform/{offset,scale,rotation}` | vec2 / vec2 / float |
| `/materials/{i}/alphaCutoff` | float |
| `/meshes/{i}/weights` (+ optional `/weights/{k}` for a single target) | morph weights |
| `/nodes/{i}/{translation,rotation,scale,weights}` (+ optional `/{k}`) | TRS / morph weights |
| `/extensions/KHR_lights_punctual/lights/{i}/{color,intensity,range}` | vec3 / float / float |
| `/extensions/KHR_lights_punctual/lights/{i}/spot/{innerConeAngle,outerConeAngle}` | float |

### 5.2 `KHR_lights_punctual`

```jsonc
"extensions": {
  "KHR_lights_punctual": {
    "lights": [
      { "type": "directional", "intensity": 1.0, "color": [1, 1, 1],
        "extras": { "shadowMapBias": 0.02 } }
    ]
  }
}
```

Light **type** is `point`, `spot` or `directional`. A node references a light
with the node extension `{"KHR_lights_punctual": {"light": 0}}`; the node's
position/direction is the light's position/direction. Spot lights take
`spot.innerConeAngle` / `spot.outerConeAngle`; point/spot take `range`. Up to
**4** lights per scene are used (extra light nodes are ignored).

Light and light-node `extras` control the shadow map (see section 6 for the
defaults and the `!= 0` override rule):

* `shadowMapNear`, `shadowMapFar`, `shadowMapBottom`, `shadowMapTop`,
  `shadowMapLeft`, `shadowMapRight` — the shadow-mapping frustum; it must cover
  all visible objects to produce correct results.
* `shadowMapBias` — bias to reduce shadow acne (raise it) / peter-panning
  (lower it).

## 6. Node `extras` (per-object rendering tweaks)

Every glTF node can carry an `extras` object. Values are read **case-sensitively**
(`loopCount` capital C matters — see section 7).

```jsonc
"extras": {
  "trans": "ADD",            // "ADD" | "SUB" | "MUL" | "NONE" (default)
  "id": 3,                   // integer, used by [BGCTRL3D] ctrlid (section 8)
  "layerNumber": 0,          // -1 | 0 | 1 (section 9)
  "castShadow": 1,           // 0/"false" disables shadow casting (default 1)
  "disableZTest": false,     // disables depth testing (default false)
  "disableZWrite": false,    // disables depth writing (default false)
  "meshOutline": 0.0         // > 0 draws an inverted-hull outline (section 6.1)
}
```

* `trans` — renders the object with the given blend mode (`ADD`, `SUB`, `MUL`;
  `NONE` default). PalFX synthesis treats these like the 2D `trans` modes.
* `id` — integer used to identify the node for `[BGCTRL3D]` / `ModifyBGCtrl3d`
  (see section 8). Can also be set on animations (section 7).
* `layerNumber` — which 2D layer the node draws on: `-1` (stage-model-only,
  before everything), `0` (default), `1` (front). Screenpack models default to
  0; stage scene-1 nodes default to 1 (section 9).
* `castShadow` — `0` / `"false"` disables this object casting a shadow.
* `disableZTest` / `disableZWrite` — depth-test/write toggles. `disableZWrite`
  is useful for rendering 2D-style billboards inside the model; both are
  "not recommended unless you know exactly what you're doing".

⚠️ **String comparison caveat:** `enabled`, `castShadow`, `disableZTest` and
`disableZWrite` are matched by raw JSON **string** equality against
`"0"`/`"false"`. JSON booleans and integers behave differently from the wiki's
"boolean"/"integer" types: `"castShadow": false` (boolean) or `0` (int) does
**not** disable shadow casting, while `"disableZTest": false` (boolean)
actually **disables** depth testing. In Blender set these custom properties as
STRING values (`"0"`/`"1"`), or omit them to keep the default.
* Light nodes additionally accept the shadow-map `extras` from section 5.2.

### 6.1 Mesh outline (inverted hull)

Ikemen-GO draws mesh outlines with the **inverted hull** method: back-facing
faces of the mesh are extruded and drawn as an outline. It only works on
**opaque meshes with back-face culling** (no `doubleSided`, no alpha blending).

```jsonc
"extras": { "meshOutline": 0.05 }
```

* `meshOutline` — extrusion amount; larger values draw a thicker outline.
  (`stage3d_outline.glb` in `deploy/stages` demonstrates this.)
* By default vertices are extruded along the vertex **normal**. Geometry with
  sharp edges usually needs a custom attribute:
  `_OUTLINE_ATTRIBUTE` — a per-vertex `vec4`; `xyz` is the extrusion direction,
  `w` the extrusion amount for that vertex. Without it the normal is used.
  When `meshOutline` is 0 the attribute does nothing.
* The outline respects the node's `trans`/PalFX, so an `ADD`-blended outline
  mesh gives the classic "glow outline" look.

## 7. Animation `extras`

Animations declared in the glTF play automatically (stepped with `Model.step`
every tick, including `turbo`), reset on round/scene reset, and support an
`extras` object:

```jsonc
"extras": {
  "id": 2,             // integer, used by [BGCTRL3D] ctrlid (section 8)
  "loopCount": -1,     // number of loops; -1 = infinite (default)
  "enabled": "1"       // "0"/"false" disables the animation at load
}
```

* `id` — same role as node `id`: lets `[BGCTRL3D]` / `ModifyBGCtrl3d` target
  this animation.
* `loopCount` — number of times the animation loops; `-1` (default) is
  infinity. Note the **capital C** — `loopcount` is ignored.
* `enabled` — `"0"` or `"false"` starts the animation disabled. `defaultEnabled`
  is captured so round resets restore it.

## 8. Controlling the model from the stage — `[BGCTRL3D]`

Stage-only. `[BGCTRLDEF]` / `[BGCTRL]` handle 2D backgrounds; **`[BGCTRL3D]`**
targets model nodes and animations by `ctrlid`:

```ini
; Play/stop an animation with id = 2
[BGCTRL3D]
ctrlid = 2
type = anim
value = 1               ; 1 = play (restart), 0 = stop
time = 0

; Toggle visibility of a node with id = 3
[BGCTRL3D]
ctrlid = 3
type = visible
value = 0               ; hide the node
time = 60, 120          ; start, end, looptime
```

* `ctrlid` matches the node/animation `id` extra (section 6/7). `-1` targets
  nothing for 3D (unlike 2D BGCtrl's "all backgrounds").
* Supported `type` values that affect the model:
  * `anim` — `value` != 0 plays (and restarts) the matched animations,
    `value` = 0 stops them (`GLTFAnimation.toggle`). 2D backgrounds targeted
    by the same ctrl are unaffected unless also matched.
  * `visible` — `value` != 0 shows, 0 hides the matched nodes.
* `time` follows the normal BGCtrl semantics: `start`, `end`, `looptime`
  (`time = start`, `time = start, end`, `time = start, end, looptime`).

From CNS/ZSS the `ModifyBGCtrl3d` state controller (see
`docs/ikemen_cns_scripting.md`) edits a `[BGCTRL3D]` entry at runtime —
`time` (start, end, looptime) and `value` (for `anim`: play/stop):

```ini
[State 200, restart bg anim]
Type = ModifyBGCtrl3d
Trigger1 = ...
ctrlid = 2
time = 0, -1, -1
value = 1
```

## 9. Scenes and layers

A glTF file can contain multiple **scenes**; Ikemen draws scene 0 and (for
stages) scene 1.

* **Stage model** — drawn in three passes matching the 2D layer draw order:
  * layer `-1` pass: nodes with `layerNumber = -1` (stage-model-only).
  * layer `0` pass: scene 0, nodes with `layerNumber` unset (default 0) or `0`.
  * layer `1` pass: scene 0 nodes with `layerNumber = 1`, **plus scene 1**
    (nodes default to layer 1).
  In other words: put background geometry in scene 0, foreground geometry
  (e.g. objects in front of the characters) in scene 1, and mix 2D BGs in via
  node `layerNumber`.
* **Screenpack / storyboard model** — each `BGDef` draws it only when its
  `<bgname>def` section sets a `scenenumber`:

  ```ini
  [TitleBgDef]
  scenenumber = 0          ; which glTF scene to draw (>= 0 enables drawing)
  modeloffset = 0, 0, 0    ; position (screenpack BGDef only)
  modelrotate = 0, 0, 0    ; Euler degrees (converted to radians internally)
  modelscale  = 1, 1, 1    ; scale
  fov  = 40                ; projection (default 40)
  near = 0.1
  far  = 10000
  ```

  Without a `scenenumber` the model is not drawn by that BGDef. These keys are
  read by the screenpack `BGDef` loader (`src/bgdef.go`) and are not available
  in stage `.def` files (stages use `[Model] offset`/`scale` instead; there is
  no stage-level model rotation).

## 10. Notes & limitations

* Animations, morphs and texture transforms are all float-stepped at 60 Hz with
  `turbo` applied (`anim.time += turbo / 60`). They keep running through
  character hitpause and only freeze when the stage itself pauses
  (`Stage.paused()`: superpause with `superpausebg`, or pause with `pausebg`).
* Models are **not** affected by sprite batching; each model draw is its own
  pipeline pass. Keep vertex counts reasonable for mobile/GLES targets.
* `shadowMap*` extras: values are applied only when **non-zero** — `0` keeps
  the engine default (`near` 0.1 point/spot / -20 directional, `far` 50,
  bottom/left -20, top/right 20, `bias` 0.02).
* Shadow mapping renders only when the scene has light nodes, during the
  **layer -1 draw pass**, and `EnableModelShadow` is on; up to 4 shadow-casting
  lights are used. Each light gets its **own frustum(s)** in a shared cube
  array texture (1024² D32, 4 cubes × 6 faces): a **point** light renders all
  6 faces (90° perspective each), a **directional** light a single orthographic
  frustum (`shadowMapLeft/Right/Bottom/Top/Near/Far`), a **spot** light a
  single 90° perspective frustum. Geometry is re-drawn once per light (with
  per-light bounding-box culling), and each primitive is frustum-culled against
  up to 6 face matrices for point lights. The **Direct3D 11 renderer never
  renders model shadows** (`Renderer_DX.IsShadowEnabled() == false`); GL 3.3 /
  GLES 3.2 / Vulkan honor the config (armdevice defaults it off).

  ⚠️ Two caveats in the current source: (1) directional/spot shadow and light
  matrices are built from the light node's **local** transform while the
  `position` uniform uses its **world** transform — a directional/spot light
  parented under a transformed node renders misplaced shadows; keep light nodes
  at the scene root. (2) The spot-light shadow lookup samples the shadow cube
  from raw clip-space `lightSpacePos` without the perspective divide, which is
  only correct when the light's clip `w ≈ 1`; prefer directional lights for
  shadow-critical setups.
* The stage model is uploaded to GPU buffer slot 0, the screenpack model to
  slot 1; both share the same vertex/index format.
* Model assets may be packed inside a ZIP archive; the loader resolves
  relative resources through the archive (`src/model.go` `IsZipPath` path).
* glTF JSON pointers in `KHR_animation_pointer` that are unsupported produce a
  console error at load (`invalid/unsupported JSON pointer: ...`).

## 11. Exporting from Blender

Blender ships a glTF 2.0 exporter (File ▸ Export ▸ glTF 2.0, Blender 2.80+).
The settings below match what Ikemen-GO actually reads (sections 4–7).

### Format and scene setup

* **Format: glTF Binary (.glb)** — single file with embedded textures; this is
  what the reference stages load (`model = stage3d.glb`). **glTF Separate**
  (`.gltf` + `.bin` + textures) also works; the engine resolves the relative
  resources, including inside ZIP archives (section 10).
* Model in **real-world metres** (glTF's unit). Blender is Z-up; the exporter
  converts to glTF's Y-up automatically. Keep 1 Blender unit = 1 m, then use
  `[Model] scale = 0.05, 0.05, 0.05` for the MUGEN conversion, exactly like
  `stage3d.def`. Put the **world origin where the camera starts**.
* Apply modifiers you rely on (the exporter can bake them), but keep the
  **Armature** modifier on skinned meshes — the exporter converts the rig to
  `JOINTS_n`/`WEIGHTS_n` itself.

### Export panel (main settings)

| Setting | What to use / why |
|---|---|
| Include ▸ Selected Objects | off (export the whole scene) |
| Include ▸ Data ▸ **Custom Properties** | **on** — without this, node `extras` (section 6) are dropped |
| Mesh ▸ Attributes | keep POSITION / NORMAL / TANGENT / TEXCOORD_0 / COLOR_0 |
| Mesh ▸ Attributes ▸ custom | attributes named with a leading `_` (e.g. `_OUTLINE_ATTRIBUTE`, section 6.1) are exported — create it as a per-vertex float4 (XYZ direction + W amount) |
| Mesh ▸ Vertex Colors | on (engine reads `COLOR_0` only) |
| Mesh ▸ Apply Modifiers | on for modifier-baked meshes; the Armature modifier can stay for skinned rigs |
| Texture ▸ Images | PNG/JPEG — the engine rejects DDS and WebP is not recommended |

Other notes:

* **One UV map per mesh** is safest — only `TEXCOORD_0` is used (section 4).
* **Skinning:** Blender writes the first 4 influences into `JOINTS_0`/
  `WEIGHTS_0` and the next 4 into `JOINTS_1`/`WEIGHTS_1` — the second set is
  now fully supported (section 4), so meshes can weight vertices to up to 8
  groups.
* **Shape keys** export as glTF morph targets and animate in-engine (up to 8
  active morphed attributes).
* **Lights:** point / spot / sun export as `KHR_lights_punctual`; the engine
  uses the first 4. Put `shadowMap*` extras on the **light object** (sections
  5.2 / 6).
* No mesh compression is needed or supported (Draco/Meshopt are not in the
  loader's extension list).

### What Blender cannot export

Blender's exporter does **not** emit:

* **`KHR_animation_pointer` channels** — material-factor / light / morph-weight
  animations via pointers (section 5.1). Only node TRS and shape-key
  animations export natively.
* **Animation `extras`** — `id`, `loopCount` (capital C) and `enabled`
  (section 7) must be added by editing the exported `.gltf` JSON (it is plain
  JSON) or with a small export script.

### Node extras as Blender custom properties

Set extras on the **object** (Properties ▸ Object ▸ Custom Properties) — the
engine reads node `extras`, so properties set on the mesh data are ignored.
Key names are case-sensitive and must match exactly:

* `trans` — **string** (`"ADD"`, `"SUB"`, `"MUL"`, `"NONE"`)
* `id`, `layerNumber` — **integers** (float/int custom properties export as
  JSON numbers)
* `meshOutline` — **float**
* `castShadow`, `disableZTest`, `disableZWrite` — **strings** `"0"` / `"1"`
  (see the string-comparison caveat in section 6: numbers and booleans do not
  behave like the wiki's types — a string `"0"` is what actually disables)

## Cross-references

* `docs/ikemen_cns_scripting.md` — `ModifyBGCtrl3d` state controller
* `docs/ikemen_zss_scripting.md` — controller list (incl. `ModifyBGCtrl3d`)
* `deploy/stages/stage3d.def`, `deploy/stages/stage3d_outline.def` — reference
  stage + outline workflow
* `src/model.go`, `src/stage.go`, `src/bgdef.go`, `src/motif.go`,
  `src/storyboard.go` — implementation
* glTF 2.0 Specification — full feature set reference