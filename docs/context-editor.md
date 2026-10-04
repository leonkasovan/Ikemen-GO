# `src/editor_server.go` — Context

Architectural companion to [`docs/ikemen_editor.md`](./ikemen_editor.md), which
is the user-facing API and live-apply *reference*. This document is for changing
the code: what the invariants are, why they exist, and what will break if you
move them.

---

## 1. Scope

| File | Lines | Role |
| --- | --- | --- |
| `src/editor_server.go` | 4812 | The whole service: HTTP handlers, path/INI helpers, reflection walk, live-apply engine, and the embedded UI (`editorPageHTML`) |
| `src/editor_webview_windows.go` | 226 | Built-in window, `//go:build windows`, go-webview2 |
| `src/editor_webview_other.go` | 21 | Stubs returning "unsupported", `//go:build !windows` |
| `src/editor_server_test.go` | 3334 | 78 tests |
| `src/editor_webview_windows_test.go` | — | 4 tests |

One file on purpose: the service is a self-contained subsystem with no build-tag
variants, and its parts only make sense together (the schema walk feeds the
merge, which feeds the tree, which feeds the JSON the embedded page renders).
The only platform split is the window, behind four functions the service calls:

```go
editorWebViewOpen(url, title string, width, height uint) bool
editorWebViewSize() (uint, uint)
editorWebViewState() string
editorWebViewClose() bool
```

`editor_webview_other.go` implements all four as no-ops / `"unsupported"`, so
`openEditorView` falls through to the default browser.

### File map

The file is organised by comment banners — grep for
`^// ----` to navigate:

| Line | Section |
| --- | --- |
| 216 | Path / INI helpers |
| 480 | HTTP plumbing |
| 546 | Structural breakdown of the motif (`src/motif.go` → INI layout) |
| 923 | INI values of the configured motif / stage / character files |
| 1071 | API handlers |
| 1449 | Section tree: screens, background definitions, actions |
| 1693 | Stage / character breakdown (select.def) |
| 2099 | Sprite preview (`/api/sff`) |
| 2382 | Live apply (motif only) |
| 3487 | Pinned keys (`[Editor Pins]` in the engine config) |
| 3918 | Editor UI (single page, no external assets) |

---

## 2. Entry points

Two, and only two:

```
main.go:204      -httpservice flag  ──▶ startEditorHTTPService()
script.go:3508   Lua openEditor()   ──▶ openEditorAsync(view)
```

`startEditorHTTPService` is idempotent behind `editorMu` and never blocks; it
returns the base URL or `""` when port 6700 is taken. `openEditorAsync`
resolves the URL, reads the window size **on the engine thread** (the game
thread owns it and can change it on a resolution switch), then opens the
window on its own goroutine. That split exists because a cold WebView2 window
can take seconds to come up and the engine thread must not stall — see the
comment above `openEditorAsync`.

---

## 3. Threading model — the invariant that matters most

**The game loop reads `sys.motif` every frame. No HTTP goroutine may touch it.**

Every read or write of the motif, and every Lua call, is marshalled onto the
engine thread:

```go
editorRunOnMainThread(fn)        // 2s timeout, func() error
editorRunOnEngineThread(fn)      // 2s timeout, func() editorApplyOutcome
editorRunOnEngineThreadTimeout(d, fn)
```

Both post to `sys.mainThreadTask` (a buffered channel of 65536 drained by
`System.await` every frame in every game state) and block on a result channel.
Classification itself runs on the engine thread because it reads the live motif
(`editorApplyMotifSync` passes a closure that classifies and applies together).

**When you add a handler, decide per field whether it is engine-owned.**
`sys.motif`, `sys.luaLState`, `sys.cachedMotifTable` and `sys.loader.state` are
engine-owned. Path resolution, INI parsing, file stats and JSON encoding are
not.

Note the channel send is **non-blocking** (`select`/`default`), so a saturated
queue yields `"the engine task queue is full"` rather than deadlocking a
request forever.

### Lock inventory and ordering

| Lock | Guards | Notes |
| --- | --- | --- |
| `editorMu` | `editorRunning`, `editorBaseURL` | Service lifecycle only |
| `editorSaveMu` | `/api/save` and the reload core | Serialises a read-modify-write + live apply |
| `editorPinsMu` | Pin toggles | |
| `editorSffCache.mu` | Decoded `.sff` + PNG map | |
| `editorInfoCache.mu` | Cached `[Info]` blocks | |

`editorSaveMu` is the dangerous one: **`sync.Mutex` is not re-entrant.** The
convention that keeps it safe is the `Locked` suffix:

```go
editorReloadMotifSync()   // takes editorSaveMu itself  — call this when you don't hold it
editorReloadMotifLocked() // requires the caller holds it — editorHandleSave path
```

`TestEditorReloadLockSplit` pins both halves. If you add a function that can be
reached both with and without the lock held, follow this pattern rather than
trying to detect it.

Never hold `editorSaveMu` while doing file I/O on a large file — the reload
reads the whole motif.

---

## 4. Security model

Local-only by construction, with defence in depth:

- **Bind `127.0.0.1`**, never `0.0.0.0`. Hardcoded in `startEditorHTTPService`.
- **No CORS headers are set at all.** A web page on another origin can issue a
  request but the browser will not hand the response to script, so cross-origin
  *reads* are blocked.
- **Writes require `X-Editor-Request`.** A custom header forces a CORS
  preflight, which the service does not answer — so a cross-site POST cannot be
  sent from a page. Enforced once in `editorReadJSONRequest`.
- **Sandbox**: `editorSandboxPath` resolves relative to `sys.baseDir`, cleans,
  and rejects anything outside. Every file path in the service goes through it.
- **Read caps** (`editorMaxReadFile` 16 MiB, `editorMaxSFF` 256 MiB,
  `editorMaxBody` 1 MiB) — `stat` before read, so a huge blob is refused, never
  loaded. Extension allowlist for text reads (`editorTextExtensions`).
- **Atomic writes** (`editorWriteFileAtomic`): temp file in the same directory,
  mode preserved, `os.Rename` over the target.

There is **no authentication token**. Anything running as the same user on the
machine can drive the service. That is a deliberate trade for a local
single-player tool; don't widen the bind without adding auth.

`editorFileReadStatus` maps helper errors to status codes by matching the
message string (`"not found"` → 404, `"too large"` → 413, …). Adding a new
error string to a gated helper without updating it silently becomes a 400.

---

## 5. Reading a motif

Three sources are merged, and understanding which is which explains most of the
code.

**(a) Structural schema** — `buildEditorMotifStructure` walks `Motif{}` by
reflection (`src/motif.go`) and reports what INI the loader *can* assign:
section → dotted key → type + `default` tag. Memoised by `editorMotifStructure`
behind `editorStructureOnce`, because it depends only on the compiled type.
**The returned slice is shared — treat it as read-only.**

Three special cases the walk handles:

- `Motif.Music` has no tag; `[Music]` is filled by `parseMusicSection` →
  `editorMusicSchema()`.
- `map:`-tagged fields have user-defined section names → `Kind: "map"`, matched
  against file headers with the pattern as a regexp.
- Nested structs are namespaces (`menu.item.font`) unless tagged
  `flatten:"true"`.

**(b) Background definitions** — `<name>BGdef` layers and controllers are *not*
in the struct; `bgdef.go` collects them and `readBackGround` / `bgCtrl.read`
(`src/stage.go`) read them at load. Hand-written schemas
(`editorBGElementSchema`, `editorBGCtrlSchema`), marked `Runtime` in the output.

**(c) File values** — the parsed `.ini`, with the original header spelling
preserved via `editorRawSections` (section bodies for keyless sections like
`[Begin Action n]` are kept as `Raw`).

`editorMotifSections` merges them. Every key gets exactly one of four states,
which is what the ●/○/⚠ column renders:

| State | Meaning | Source |
| --- | --- | --- |
| defined | file sets it, engine reads it | schema ∩ file |
| missing | engine default applies | schema \ file |
| unknown | engine ignores it | file \ schema |
| *(runtime)* | valid as written — parsed outside the struct | `editorIsRuntimeSection` |

Ordering is `defined`, then `missing`, then `unknown`, stable within each group
(`editorKeyStateRank` + `sort.SliceStable`). **Adding a key to the schema
changes the table order** — `TestEditorApplyTitleInfoBlockLive` and friends
depend on it.

### A trap worth knowing

`editorDynamicKeyMatch` decides which file keys belong to a user-named map entry
(`cell.<c>-<r>.offset`, `menu.itemname.<name>`, `font1`). It accepts three
spellings — dotted prefix, bare map key, and name-first. It is intentionally
greedy, so a new user-named section must be checked against it or its keys will
land in the wrong section.

---

## 6. Saving and live apply

Order is fixed and matters:

```
1. write the file            (editorEditINIFile, line-based, atomic)
2. serialise                 (editorSaveMu held across 1-4)
3. classify on engine thread (editorMotifApplyClassify)
4. apply                     (SetValueUpdate / updateINIFile)
5. rebuild                   (snapshots → position pass → Lua table sync)
6. reload if needed          (editorReloadMotifLocked, automatic)
```

The file is written **first**, unconditionally. A live-apply failure leaves the
file correct and is reported separately (`applyError`), because a restart would
not help a key the engine has no field for.

### The four modes

`editorMotifApplyClassify` decides what a save can achieve:

| Mode | Meaning | Mechanism |
| --- | --- | --- |
| `editorApplyLive` | Drawing reads the field | assign + position pass + Lua table sync |
| `editorApplyRefresh` | A load-time pointer snapshots the field | refill **in place**, then position pass |
| `editorApplyReload` | Load-time only | write the INI, run `loadMotif()` |
| `editorApplyRestart` | Read once at script boot, into a structure nothing rebuilds | write the INI, tell the user |

The deciding question for 2 and 3 is *does a `PopulateDataPointers` snapshot
cover this field?* `editorMotifKeyDerivedPtrs` walks the query to the struct that
declares it and returns that struct's derived pointers:

```go
editorTextSpriteType, editorPalFxType, editorRectType, editorFadeType, editorAnimType
```

`editorHasRefillablePtr` then answers whether they can be refilled in place.
`*Anim` **cannot** — it carries element playback state only a rebuild resets —
which is why every `*Anim`-backed key is a reload. That single rule is why
`select_info.cell.bg.spr` and `select_info.stage.portrait.bg.offset` are reload
even though the field is a plain `[2]int32`.

### Never replace a snapshot — only refill it

`PopulateDataPointers` builds these pointers **once at load**, and `toLValue`
hands the Lua script userdata wrapping the Go pointer. The script holds a handle
to a *specific object*. Replacing it would leave every handle dangling.

```go
editorFillSnapshot / editorRefillSnapshots  // setTextSpriteInto, setFadeInto, ...
```

Hence `editorHasPopulatedSnapshot` gates `applyPostParsePosAdjustments`: that
pass dereferences every snapshot it walks, so it is only safe on a motif that
went through `PopulateDataPointers` (not a bare test fixture).

**The order in `editorReapplyMotifScreen` is load-bearing.** `setTextSpriteInto`
ends by calling `SetPos` with the struct's own `Offset`, restoring the pristine
`offsetInit`; only *then* does the position pass add container offsets. Running
the position pass alone would double-count the container offset and walk the
sprite further across the screen on every save. Don't reorder these two lines.

### The load-time Lua capture problem

This is the concept behind modes 3 and 4. The Go motif is not the only copy.
The script *also* copies values out at boot, into structures nothing rebuilds:

| Copy | Keys | Why it can't be fixed live |
| --- | --- | --- |
| `main.menu`, `options.menu`, `menu.t_menus` | `menu.itemname.*`, `menu.title.uppercase`, `title.text` | built once by `f_start` |
| `start.t_grid` + its draw list | `[Select Info]` `rows`, `columns`, `cell.*` geometry, `pos`, `showemptyboxes` | built once by `f_updateGrid` |
| `main.t_selGrid` / `t_selChars` | `[Files] select` | roster built at boot, only re-sized by reload |
| `t_modules` | `[Files] module` | `require`d once, Lua caches it |

The reload re-runs the first three builders (`editorRebuildAfterReload` →
`f_rebuildMenus`, `f_updateGrid`, `f_refreshSelectTitle`), which is why
`editorMotifMenuBuildKey` asks for a reload instead of a live apply. The last
two are genuinely restart-only.

### Scoped reload tails

`editorReloadTails` exists because position and text-geometry values under
`select_info.stage.*` only reach the screen through a reload — the block is laid
out when the motif loads. It is **deliberately prefix-scoped rather than a
blanket `*.offset` / `*.scale` rule**, because the same tails apply live
everywhere else (every `[Title Info]` key, for example) and a blanket rule would
demote hundreds of working keys.

Do not "tighten" this by narrowing the prefix. It was already tried and reverted:
`stage.offset` / `stage.scale` are `TextProperties` (`*TextSprite`), so they
would flip from reload to refresh and silently draw the boot copy. Adding a new
scope is one table row. See `TestEditorMotifReloadTailKey`.

---

## 7. Caches

Three, all with the same discipline: **validate against the file, bound the
size, never cache a failure.**

| Cache | Key | Bound | Validation |
| --- | --- | --- | --- |
| `editorSffCache` | path | `editorSffCacheMax` = 8 files | mtime + size |
| `editorInfoCache` | path | `editorInfoCacheMax` = 256 | mtime + size |
| `editorStructureOnce` | — | n/a | n/a (compiled type) |

`editorInfoCache` exists because `editorReadInfo` is called once per select.def
entry *and* once per file in the `stages/` scan; unmemoised it re-parsed every
`.def` behind the roster on every request (measured 13.7 ms / 2.95 MB per call
on a 6k-line `.def`). **Only successful parses are cached**, so a missing,
unreadable or `[Info]`-less file keeps returning the zero value.

`editorFileStat` is the shared identity helper. The `.sff` cache additionally
caches each sprite's encoded PNG on the entry, so paging back is free; the entry
is rebuilt when the file changes, which drops the PNGs with it.

---

## 8. Measured cost

Baseline on the editor read path (Ryzen 5 3500U, `static desktop` tags), used to
justify the caching decisions above:

| Operation | Before | After |
| --- | --- | --- |
| `editorMotifStructure` | 6.8 ms, 1.98 MB, 11,094 allocs | **24 ns, 0 B** |
| `editorRawSections` (6k-line `.def`) | 9.26 ms, 24 MB/s | **0.84 ms, 208 MB/s** |
| `editorReadInfo` | 13.7 ms, 2.95 MB per call | memoised |
| `json.MarshalIndent` of one `/api/motif` | **67 ms, 12.6 MB** | unchanged |

One `/api/motif` response is **~2 MB**: 36 sections, 9,577 schema keys,
`structure` 530 KB + `sections` 787 KB + `tree` 788 KB.

### Known open item — the payload

The embedded page reads `data.tree` for all rendering, `data.sections` only for
`.length` (the count in the hint), and never reads `data.structure` at all. The
tree also duplicates every key the sections already carry. Roughly 1.3 MB of the
2 MB is redundant.

This was **not** changed because it alters the wire format and `structure` may
be part of the API for external tooling. The natural fix is to make `structure`
opt-in (`?structure=1`) and replace `sections` with a count, leaving the tree as
the single carrier. Worth ~62% of the payload and the 67 ms encode — but it is
an API decision, not a clean-up.

---

## 9. Cross-file dependencies

The service borrows a lot from the engine. When one of these changes, the editor
is affected.

**Engine state (engine thread only):** `sys.motif`, `sys.luaLState`,
`sys.cachedMotifTable`, `sys.loader.state`, `sys.mainThreadTask`.
**Engine state (safe from any goroutine):** `sys.baseDir`, `sys.cmdFlags`,
`sys.middleOfMatch()`, `sys.gameRunning`, `sys.netplay()`.

**Helpers:** `LoadINIFile`, `LoadINIText`, `LoadText`, `preprocessINIContent`,
`NormalizeNewlines`, `SearchFile`, `SetValueUpdate`, `updateINIFile`,
`findFieldByINITag`, `parseQueryPath`, `toLValue`, `loadSffEx`, `defaultMotif`,
`Version`, `BuildTime`.

**Structs reflected on:** `Motif` (`src/motif.go`), and via
`editorDeclaredDerivedPtrs` the declaring struct of every key. The five snapshot
types and the properties structs that declare them:

| Snapshot | Declared as | By properties structs |
| --- | --- | --- |
| `*TextSprite` | `TextSpriteData` | `TextProperties`, `TextMapProperties`, `AnimationTextProperties` |
| `*PalFX` | `PalFxData` | `PalFxProperties` (note the lowercase `x`) |
| `*Rect` | `RectData` | `BoxCursorProperties`, `BoxBgProperties`, … |
| `*Fade` | `FadeData` | `FadeProperties` |
| `*Anim` | `AnimData` | `AnimationProperties`, `BgAnimationProperties`, … |

Do not guess a properties struct name from the Go type — `editorDeclaredDerivedPtrs`
matches on the snapshot *field type*, and several unrelated structs declare
`RectData *Rect`.

**Lua globals the service calls:** `loadMotif`, and via `editorCallLuaMethod`
`main.f_rebuildMenus`, `start.f_updateGrid`, `main.f_refreshSelectTitle`. A
missing table or method is a no-op, so an older script or a unit test without
Lua degrades quietly rather than erroring.

**Go modules:** `github.com/yuin/gopher-lua`, `gopkg.in/ini.v1`.

---

## 10. Extending it

| To add | Do this | Don't forget |
| --- | --- | --- |
| An enumerated key | a row in `editorKeyChoices`; for `type`, extend `editorChoicesFor` | the last path segment is matched, so `p1.face.trans` resolves too |
| A motif key | nothing — the walk is automatic | check `editorDynamicKeyMatch` if it is user-named |
| A live-apply rule | `editorMotifApplyClassify`, **before** the derived-pointer check if it is load-time | add to the negative list in `TestEditorMotifReloadTailKey` if it must *not* reload |
| A reload-forced scope | one row in `editorReloadTails` | — |
| A screen group | one row in `editorScreenGroups` (17 rows) | `Results` and `Pause` have no `Tags` — `groupFor` falls back to matching `results` / `pause` as a substring of the normalised section name |
| A background layer/controller key | `editorBGElementSchema` / `editorBGCtrlSchema` | the defaults are `newBackGround()`'s, not arbitrary |
| A select-grid rebuild trigger | `editorSelectGridQuery` | only add what is genuinely baked into `start.t_grid` |
| An endpoint | register in `startEditorHTTPService`; cap reads, sandbox paths, and require `X-Editor-Request` for writes | — |

---

## 11. Testing

```bash
make test-editor    # -run TestEditor, 82 tests (80 run, 2 opt-in WebView2)
make test-editor TESTFLAGS="-run TestFoo -v"   # quote it
```

`make test`/`make test-editor` build with the real cgo env, so run them after a
build. `make vet` is the right way to see vet issues (`test` passes `-vet=off`).

Two patterns are worth copying.

**Stand in for the engine thread.** Most tests that touch
`sys.mainThreadTask` would otherwise wait out the full timeout, so they drain the
channel themselves:

```go
stop := make(chan struct{})
go func() {
    for { select {
        case f := <-sys.mainThreadTask: f()
        case <-stop: return
    } }
}()
defer close(stop)
```

`TestEditorReloadLockSplit` is the reference for anything touching
`editorSaveMu`.

**Save and restore global engine state.** `sys.cmdFlags`, `sys.baseDir`,
`sys.motif` are process-wide; tests snapshot and restore them in a `defer`
(`TestEditorReloadLockSplit`, `TestEditorAgainstConfiguredMotif`).

Classification tests are table-driven over real `motif.go` structs, so a
motif change that shifts a key's mode fails loudly instead of silently changing
the applied/needsReload answer the UI shows.

---

## 12. Things that will bite you

- **`editorReloadTails` is coarse on purpose.** Narrowing the prefix breaks
  `stage.offset` / `stage.scale`. Already tried, reverted.
- **The schema slice is shared and memoised.** Mutating what
  `editorMotifStructure` returns corrupts every later request.
- **`editorFileReadStatus` matches on message text.** New error strings need a
  mapping or they become 400s.
- **`setTextSpriteInto` must run before `applyPostParsePosAdjustments`.** See §6.
- **`editorSyncMotifLuaTable` only replaces an existing entry** and runs no Lua
  code, which is what makes it safe between frames. Adding a `CallByParam` there
  would need real care about re-entrancy.
- **Reload is refused during a match, netplay/replay, and while loading** —
  the motif is drawn every frame and its snapshots are handed out as handles.
  `POST /api/reload` answers `409`; a save that needed one reports
  `needsReload` and tells the user to reload later.