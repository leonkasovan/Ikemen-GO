# Ikemen-GO Lua Scripting Reference

This document describes the Lua API exposed by Ikemen-GO to Lua scripts and mods.
All functions listed here are available as globals unless otherwise noted.

---

## Table of Contents

1. [HTTP Library (`http`)](#1-http-library-http)
2. [Animation (`anim*`, `batchDraw`)](#2-animation)
3. [Background (`bg*`)](#3-background)
4. [Rectangle (`rect*`)](#4-rectangle)
5. [Text Image (`textImg*`)](#5-text-image)
6. [Font & SFF (`fontNew`, `sffNew`, `sndNew`)](#6-font--sff--snd)
7. [Sound & Music](#7-sound--music)
8. [Character & Stage Selection](#8-character--stage-selection)
9. [Match & Game Control](#9-match--game-control)
10. [Input & Controller](#10-input--controller)
11. [Screen & Rendering](#11-screen--rendering)
12. [Motif & Config](#12-motif--config)
13. [File & Utility](#13-file--utility)
14. [Debug](#14-debug)
15. [State & Save](#15-state--save)
16. [Netplay & Replay](#16-netplay--replay)
17. [Trigger Redirections](#17-trigger-redirections)
18. [Trigger Functions (CNS equivalents)](#18-trigger-functions-cns-equivalents)

---

## 1. HTTP Library (`http`)

The `http` global table provides HTTP client functionality backed by Go's `net/http`.
A persistent cookie jar is shared across all requests. Uses no external dependencies.

### Options Table

All request functions accept an optional `options` table:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `user_agent` | string | `"Mozilla/5.0 Ikemen-GO/1.0"` | User-Agent header |
| `follow_redirects` | boolean | `true` | Follow HTTP redirects |
| `timeout` | number | 30 | Request timeout in seconds |
| `headers` | table | — | Custom headers as `{["Name"] = "value"}` |

### Functions

#### `http.get(url [, options])` → `body, status, headers`

Perform an HTTP GET request.

```lua
local body, status, headers = http.get("https://api.example.com/data", {
    user_agent = "MyBot/1.0",
    headers = { ["Authorization"] = "Bearer token" },
})
if status == 200 then
    print("Got " .. #body .. " bytes")
end
```

#### `http.post(url, body [, options])` → `body, status, headers`

Perform an HTTP POST request. Defaults `Content-Type` to `application/x-www-form-urlencoded` if not set.

```lua
local body, status = http.post(
    "https://api.example.com/submit",
    '{"key":"value"}',
    { headers = { ["Content-Type"] = "application/json" } }
)
```

#### `http.head(url [, options])` → `body, status, headers`

Perform an HTTP HEAD request (no response body returned).

```lua
local _, status, headers = http.head("https://example.com/file.zip")
if status == 200 then
    print("Size: " .. (headers["Content-Length"] or "?"))
end
```

#### `http.set_cookie(filepath)`

Set a cookie jar file path. Resets the current jar. Subsequent requests will use/store cookies here.

```lua
http.set_cookie("session_cookies.txt")
```

#### `http.clear_cookies()`

Clear cookie jar and reset last URL tracking state.

#### `http.get_last_url()` → `string`

Get the final effective URL after redirects from the last request.

```lua
http.get("https://short.url/abc")
print(http.get_last_url())  -- final URL after redirects
```

#### `http.url_encode(str)` → `string`

Percent-encode a string for use in URLs.

```lua
local enc = http.url_encode("hello world")  -- "hello+world"
```

#### `http.url_decode(str)` → `string`

Decode a percent-encoded URL string.

#### `http.parse_url(url)` → `table`

Parse a URL into its components. Returns a table with fields: `scheme`, `host`, `port`, `path`, `query`.

```lua
local parts = http.parse_url("https://example.com:8080/api?key=val")
print(parts.host)   -- "example.com"
print(parts.port)   -- "8080"
print(parts.query)  -- "key=val"
```

---

## 2. Animation

Animation objects (`Anim`) represent drawable sprite animations. Create them with `animNew`, manipulate with the `animSet*` / `animAdd*` functions, and draw with `animDraw`.

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `animNew` | `sff?: Sff, actOrAnim: string\|Animation` | `Anim` | Create a new animation from a sprite file and action definition. |
| `animCopy` | `anim: Anim` | `Anim\|nil` | Create a copy of an animation. |
| `animDraw` | `anim: Anim, layer?: int16` | — | Queue drawing of an animation. |
| `animUpdate` | `anim: Anim, force?: boolean` | — | Advance animation by one tick. |
| `animReset` | `anim: Anim, parts?: table` | — | Reset animation (fully or by named parts: `"anim"`, `"pos"`, `"scale"`, `"window"`, `"velocity"`, `"palfx"`). |
| `animAddPos` | `anim: Anim, dx: float32, dy: float32` | — | Add offset to current position. |
| `animSetPos` | `anim: Anim, x?: float32, y?: float32` | — | Set position (omit axis to use initial offset). |
| `animSetVelocity` | `anim: Anim, vx: float32, vy: float32` | — | Set base velocity. |
| `animSetAccel` | `anim: Anim, ax: float32, ay: float32` | — | Set acceleration. |
| `animSetFriction` | `anim: Anim, fx: float32, fy: float32` | — | Set velocity friction. |
| `animApplyVel` | `target: Anim, source: Anim` | — | Copy velocity/accel from source to target. |
| `animSetScale` | `anim: Anim, sx: float32, sy: float32` | — | Set scale. |
| `animSetFacing` | `anim: Anim, facing: float32` | — | Set facing (`1` or `-1`). |
| `animSetAngle` | `anim: Anim, angle: float32` | — | Set rotation angle. |
| `animSetXAngle` | `anim: Anim, xangle: float32` | — | Set X-axis rotation. |
| `animSetYAngle` | `anim: Anim, yangle: float32` | — | Set Y-axis rotation. |
| `animSetXShear` | `anim: Anim, shear: float32` | — | Set X shear factor. |
| `animSetAlpha` | `anim: Anim, src: int16, dst: int16` | — | Set alpha blending. |
| `animSetLayerno` | `anim: Anim, layer: int16` | — | Set render layer. |
| `animSetLocalcoord` | `anim: Anim, width: float32, height: float32` | — | Set local coordinate system. |
| `animSetWindow` | `anim: Anim, x1, y1, x2, y2: float32` | — | Set clipping window. |
| `animSetMaxDist` | `anim: Anim, maxX: float32, maxY: float32` | — | Set maximum draw distance. |
| `animSetProjection` | `anim: Anim, proj: int32\|string` | — | Set projection (`"orthographic"`, `"perspective"`, `"perspective2"`). |
| `animSetFocalLength` | `anim: Anim, fLength: float32` | — | Set perspective focal length. |
| `animSetTile` | `anim: Anim, tileX: boolean, tileY: boolean, spacingX?: int32, spacingY?: int32` | — | Configure tiling. |
| `animSetColorKey` | `anim: Anim, index: int16` | — | Set transparent color key index. |
| `animSetColorPalette` | `anim: Anim, paletteId: int` | `Anim` | Set active palette mapping. |
| `animSetAnimation` | `anim: Anim, actOrAnim: string\|Animation` | — | Replace underlying animation data. |
| `animSetPalFX` | `anim: Anim, palfx: table` | — | Configure palette effects (time, add, mul, sinadd, sinmul, sincolor, sinhue, invertall, invertblend, color, hue). |
| `animPrepare` | `anim: Anim, charRef: int32` | `Anim` | Prepare animation per-character palette. |
| `animLoadPalettes` | `anim: Anim, param: int` | — | Load palettes from the sprite file. |
| `animPaletteGet` | `anim: Anim, paletteId: int` | `table` | Get palette as array of `{r, g, b, a}` entries. |
| `animPaletteSet` | `anim: Anim, paletteId: int, palette: table` | — | Set palette from array of `{r, g, b, a}` entries. |
| `animGetLength` | `anim: Anim` | `int32, int32` | Get effective length and raw totaltime. |
| `animGetSpriteInfo` | `anim: Anim, group?: uint16, number?: uint16` | `table\|nil` | Get sprite info (`Group`, `Number`, `Size`, `Offset`, `palidx`). |
| `animGetPreloadedCharData` | `charRef: int, group: int32, number: int32, keepLoop?: boolean` | `Anim\|nil` | Get preloaded character animation by group/number. |
| `animGetPreloadedStageData` | `stageRef: int, group: int32, number: int32, keepLoop?: boolean` | `Anim\|nil` | Get preloaded stage animation by group/number. |
| `animDebug` | `anim: Anim, prefix?: string` | — | Print animation debug info. |
| `batchDraw` | `batch: table` | — | Draw many animations at once. Each item: `{anim, x, y, facing, scale?, xshear?, angle?, xangle?, yangle?, projection?, focallength?, layerno?}`. |
| `loadAnimTable` | `path: string, sff?: Sff` | `table` | Load an AIR file and return an animation table. |
| `getAnimElemCount` | — | `int` | Get current character's animation element count. |
| `getAnimTimeSum` | — | `int32` | Get current character's accumulated animation time. |

---

## 3. Background

`BGDef` objects represent layered background definitions loaded from stage or motif config.

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `bgNew` | `sff: Sff, defPath: string, section: string, model?: Model, defaultLayer?: int32` | `BGDef` | Load a background definition. |
| `bgDraw` | `bg: BGDef, layer?: int32, x?: float32, y?: float32, scale?: float32` | — | Draw a background (layer 0=back, 1=front). |
| `bgReset` | `bg: BGDef` | — | Reset to initial state. |
| `bgDebug` | `bg: BGDef, prefix?: string` | — | Print background debug info. |

---

## 4. Rectangle

`Rect` objects are simple colored rectangles for UI overlays.

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `rectNew` | — | `Rect` | Create a new rectangle. |
| `rectDraw` | `rect: Rect, layer?: int16` | — | Queue drawing. |
| `rectUpdate` | `rect: Rect` | — | Update animation state. |
| `rectReset` | `rect: Rect` | — | Reset to defaults. |
| `rectSetColor` | `rect: Rect, r, g, b: int32` | — | Set RGB color. |
| `rectSetAlpha` | `rect: Rect, src: int32, dst: int32` | — | Set alpha blending. |
| `rectSetAlphaPulse` | `rect: Rect, min: int32, max: int32, time: int32` | — | Enable pulsed alpha. |
| `rectSetLayerno` | `rect: Rect, layer: int16` | — | Set draw layer. |
| `rectSetLocalcoord` | `rect: Rect, x: float32, y: float32` | — | Set local coordinate size. |
| `rectSetWindow` | `rect: Rect, x1, y1, x2, y2: float32` | — | Set clipping window. |
| `rectDebug` | `rect: Rect, prefix?: string` | — | Print debug info. |

---

## 5. Text Image

`TextSprite` objects render text using engine fonts with full transform support.

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `textImgNew` | — | `TextSprite` | Create a new text sprite. |
| `textImgDraw` | `ts: TextSprite, layer?: int16` | — | Queue drawing. |
| `textImgUpdate` | `ts: TextSprite` | — | Update state. |
| `textImgReset` | `ts: TextSprite, parts?: table` | — | Reset fully or by named parts. |
| `textImgSetText` | `ts: TextSprite, text: string` | — | Set text content. |
| `textImgAddText` | `ts: TextSprite, text: string` | — | Append to text content. |
| `textImgSetFont` | `ts: TextSprite, fnt: Fnt` | — | Assign a font. |
| `textImgSetBank` | `ts: TextSprite, bank: int32` | — | Set font bank index. |
| `textImgSetAlign` | `ts: TextSprite, align: int32` | — | Set text alignment. |
| `textImgSetColor` | `ts: TextSprite, r, g, b: int32, a?: int32` | — | Set RGBA color. |
| `textImgSetPos` | `ts: TextSprite, x?: float32, y?: float32` | — | Set position. |
| `textImgAddPos` | `ts: TextSprite, dx: float32, dy: float32` | — | Offset position. |
| `textImgSetVelocity` | `ts: TextSprite, vx: float32, vy: float32` | — | Set velocity. |
| `textImgSetAccel` | `ts: TextSprite, ax: float32, ay: float32` | — | Set acceleration. |
| `textImgSetFriction` | `ts: TextSprite, fx: float32, fy: float32` | — | Set friction. |
| `textImgApplyVel` | `ts: TextSprite, source: TextSprite` | — | Copy velocity from another text sprite. |
| `textImgSetScale` | `ts: TextSprite, sx: float32, sy: float32` | — | Set scale. |
| `textImgSetAngle` | `ts: TextSprite, angle: float32` | — | Set rotation angle. |
| `textImgSetXAngle` | `ts: TextSprite, xangle: float32` | — | Set X-axis rotation. |
| `textImgSetYAngle` | `ts: TextSprite, yangle: float32` | — | Set Y-axis rotation. |
| `textImgSetXShear` | `ts: TextSprite, xshear: float32` | — | Set X shear. |
| `textImgSetLayerno` | `ts: TextSprite, layer: int16` | — | Set draw layer. |
| `textImgSetLocalcoord` | `ts: TextSprite, width: float32, height: float32` | — | Set local coordinate space. |
| `textImgSetWindow` | `ts: TextSprite, x1, y1, x2, y2: float32` | — | Set clipping window. |
| `textImgSetMaxDist` | `ts: TextSprite, xDist: float32, yDist: float32` | — | Set max visible distance. |
| `textImgSetProjection` | `ts: TextSprite, proj: int32\|string` | — | Set projection mode. |
| `textImgSetFocalLength` | `ts: TextSprite, fLength: float32` | — | Set focal length. |
| `textImgSetTextDelay` | `ts: TextSprite, delay: float32` | — | Set per-character display delay. |
| `textImgSetTextSpacing` | `ts: TextSprite, xSpacing: float32, ySpacing: float32` | — | Set character spacing. |
| `textImgSetTextWrap` | `ts: TextSprite, wrap: boolean` | — | Enable/disable text wrapping. |
| `textImgGetTextWidth` | `ts: TextSprite, text: string` | `int32` | Measure text width in pixels. |
| `textImgDebug` | `ts: TextSprite, prefix?: string` | — | Print debug info. |

---

## 6. Font / SFF / SND

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `fontNew` | `filename: string, height?: int32` | `Fnt` | Load a font file. |
| `fontGetDef` | `font: Fnt` | `table` | Get font definition info. |
| `sffNew` | `filename?: string, isActPal?: boolean` | `Sff` | Load an SFF file or create empty SFF. |
| `sndNew` | `filename: string` | `Snd` | Load a SND audio file. |
| `sndPlay` | `snd: Snd, group: int32, number: int32, volumescale?: int32, pan?: float32, ...` | — | Play a sound from an SND object. |
| `sndPlaying` | `snd: Snd, group: int32, number: int32` | `boolean` | Check if a sound is currently playing. |
| `sndStop` | `snd: Snd, group: int32, number: int32` | — | Stop a sound. |
| `modelNew` | `filename: string` | `Model` | Load a glTF 3D model. |

---

## 7. Sound & Music

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `playBgm` | `params: table` | — | Play or control background music. |
| `stopBgm` | — | — | Stop background music. |
| `updateVolume` | — | — | Sync BGM volume from settings. |
| `playSnd` | `group?: int32, sound?: int32, volumescale?: int32, commonSnd?: boolean, channel?: int32, ...` | — | Play the current character's sound. |
| `stopSnd` | — | — | Stop all sounds for the current character. |
| `stopAllCharSounds` | — | — | Stop all character sounds. |
| `clearAllSound` | — | — | Stop all currently playing sounds. |
| `waveNew` | `path: string, group: int32, sound: int32, max?: uint32` | `Sound` | Load sound data from an SND container. |
| `wavePlay` | `s: Sound, group?: int32, number?: int32` | — | Play a `Sound` object. |

---

## 8. Character & Stage Selection

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `addChar` | `defpath: string, params?: string` | `boolean` | Add a character to the select screen. |
| `addStage` | `defpath: string, params?: string` | `boolean` | Add a stage to the select screen. |
| `selectChar` | `teamSide: int, charRef: int, palette: int` | `int` | Add a character to a team selection. |
| `selectStage` | `stageRef: int` | — | Select a stage by index. |
| `selectStart` | — | — | Clear selection and start match loading. |
| `clearSelected` | — | — | Clear all current selections. |
| `loadStart` | `params?: string` | — | Validate selections and start async loading. |
| `getCharFileName` | `charRef: int` | `string` | Get a character's `.def` file path. |
| `getCharName` | `charRef: int` | `string` | Get a character's display name. |
| `getCharInfo` | `charRef: int` | `table` | Get detailed character slot info. |
| `getCharAttachedInfo` | `def: string` | `table\|nil` | Read info from a character `.def` file. |
| `getCharMovelist` | `charRef: int` | `string` | Get the movelist file path. |
| `getCharRandomPalette` | `charRef: int` | `int32` | Get a random valid palette number. |
| `getCharSelectParams` | `charRef: int` | `table` | Get parsed select parameters. |
| `getStageInfo` | `stageRef: int` | `table\|nil` | Get stage slot info. |
| `getStageNo` | — | `int` | Get the currently selected stage index. |
| `getStageSelectParams` | `stageRef: int` | `table` | Get parsed stage select parameters. |
| `getSelectNo` | — | `int` | Get character's select slot index. |
| `validatePal` | `palReq: int, charRef: int` | `int` | Validate and return a valid palette number. |
| `preloadListChar` | `id: int32\|uint16, number?: uint16` | — | Mark a character sprite/anim for preloading. |
| `preloadListStage` | `id: int32\|uint16, number?: uint16` | — | Mark a stage sprite/anim for preloading. |
| `loading` | — | `boolean` | Check if resources are currently loading. |

---

## 9. Match & Game Control

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `game` | — | `int32, int` | Execute a full match. Returns winner side and controller number. |
| `gameRunning` | — | `boolean` | Check if a match is in progress. |
| `endMatch` | — | — | Signal match end with menu fade-out. |
| `refresh` | — | — | Advance one frame (logic, draw, fades). |
| `reload` | — | — | Schedule reload of chars, stage, fight screen. |
| `roundOver` | — | `boolean` | Check if the round is over. |
| `roundStart` | — | `boolean` | Check if this is round start frame. |
| `postMatch` | — | `boolean` | Check if post-match processing is active. |
| `resetRound` | — | — | Request a round reset. |
| `resetMatchData` | `fullReset: boolean` | — | Reset match runtime data. |
| `getWinnerTeam` | — | `int32` | Get winning team side. |
| `setTeamMode` | `teamSide: int, mode: int32, teamSize: int32` | — | Configure team mode and size. |
| `setMatchWins` | `teamSide: int, wins: int32` | — | Set wins required to win. |
| `setMatchMaxDrawGames` | `teamSide: int, count: int32` | — | Set max draw games. |
| `setMatchNo` | `matchNo: int32` | — | Set current match number. |
| `setWinCount` | `teamSide: int, wins: int32` | — | Set a team's win count. |
| `setConsecutiveWins` | `teamSide: int, wins: int32` | — | Set consecutive wins. |
| `getConsecutiveWins` | `teamSide: int` | `int32` | Get consecutive wins. |
| `setHomeTeam` | `teamSide: int` | — | Set home team. |
| `setRoundTime` | `time: int32` | — | Set round time limit (-1=infinite). |
| `getRoundTime` | — | `int32` | Get configured round time limit. |
| `setTime` | `time: int32` | — | Set current timer value. |
| `getMatchTime` | — | `int32` | Get accumulated match time. |
| `setTimeFramesPerCount` | `frames: int32` | — | Set frames per timer count. |
| `setGameMode` | `mode: string` | — | Set game mode identifier. |
| `setLife` | `life: int32` | — | Set current character's life. |
| `setPower` | `power: int32` | — | Set current character's power. |
| `setRedLife` | `value: int32` | — | Set current character's red life. |
| `setDizzyPoints` | `value: int32` | — | Set current character's dizzy points. |
| `setGuardPoints` | `value: int32` | — | Set current character's guard points. |
| `removeDizzy` | — | — | Clear current character's dizzy state. |
| `mapSet` | `name: string, value: float32, mapType?: string` | — | Set or add to a character map value. |
| `changeAnim` | `animNo: int32, elem?: int32, ffx?: boolean` | `boolean` | Change current character animation. |
| `changeState` | `stateNo: int32` | `boolean` | Change current character state. |
| `selfState` | `stateNo: int32` | — | Force current character into a state. |
| `setAILevel` | `level: float32` | — | Set current character's AI level. |
| `setCom` | `playerNo: int, level: float32` | — | Set AI level for a specific player. |
| `resetAILevel` | — | — | Reset all AI levels to human. |
| `setFightScreenElements` | `elements: table` | — | Enable/disable fight screen elements. |
| `setFightScreenScore` | `p1Score: float32, p2Score?: float32` | — | Set initial fight screen scores. |
| `setFightScreenTimer` | `time: int32` | — | Set initial fight screen timer. |
| `loadFightScreen` | `defPath?: string` | — | Load the fight screen definition. |
| `computeRanking` | `mode: string` | `boolean, int32` | Compute ranking data. |
| `runHiscore` | `mode?: string, place?: int32, endtime?: int32, nofade?: boolean, nobgs?: boolean, nooverlay?: boolean` | `boolean` | Run hiscore screen for one frame. |
| `setCredits` | `credits: int32` | — | Set current credits. |
| `getCredits` | — | `int32` | Get current credits. |
| `continued` | — | `boolean` | Check if current run used a continue. |
| `getFrameCount` | — | `int32` | Get global frame counter. |
| `getGameFPS` | — | `float32` | Get current gameplay FPS. |
| `getGameSpeed` | — | `int32` | Get game speed as percentage. |
| `setGameSpeed` | `speed: int` | — | Set global game speed. |
| `paused` | — | `boolean` | Check if gameplay is paused. |
| `getRandom` | — | `int32` | Return a 32-bit random number. |
| `sleep` | `seconds: number` | — | Block script execution for N seconds. |
| `synchronize` | — | `boolean` | Synchronize with netplay or external systems. |
| `esc` | `value?: boolean` | `boolean` | Get or set the global escape flag. |
| `shutdown` | — | `boolean` | Check if shutdown was requested. |
| `getSessionWarning` | — | `string` | Pop the current session warning message. |
| `version` | — | `string` | Get engine version and build time string. |
| `ikemenVersion` | — | `number` | Get Ikemen version as a float. |
| `getRuntimeOS` | — | `string` | Get the current OS identifier. |
| `getTimestamp` | `format?: string` | `string` | Get a formatted timestamp. |

---

## 10. Input & Controller

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `getInput` | `players: number\|table, tokens: string\|table, ...` | `boolean` | Check raw UI input for one or more players. |
| `getInputTime` | `players: number\|table, tokens: string\|table, ...` | `int32` | Get hold time of a raw input token. |
| `getKey` | `key?: string` | `string\|boolean` | Query or compare the last pressed key. |
| `getKeyText` | — | `string` | Get last input text from the current key event. |
| `resetKey` | — | — | Clear the last captured key/text input. |
| `isUIKeyAction` | `action: string` | `boolean` | Check if a UI action name is currently active. |
| `getLastInputController` | — | `int` | Get the last controller that produced UI input. |
| `setLastInputController` | `playerNo: int` | — | Set or clear the last UI input controller. |
| `getJoystickGUID` | `index: int` | `string` | Get a joystick's GUID string. |
| `getJoystickName` | `index: int` | `string` | Get a joystick's display name. |
| `getJoystickPresent` | `index: int` | `boolean` | Check if a joystick is connected. |
| `getJoystickKey` | `controllerIdx?: int` | `string, int` | Poll joystick input; returns key name and joystick index. |
| `setKeyConfig` | `playerNo: int, controllerId: int, mapping: table` | — | Configure keyboard/joystick bindings. |
| `setDefaultConfig` | `configType: string, playerNo: int, enabled?: table` | — | Apply default bindings. |
| `resetRemapInput` | — | — | Reset all input remapping. |
| `remapInput` | `srcPlayer: int32, dstPlayer: int32` | — | Remap a logical player input slot. |
| `getRemapInput` | `playerNo: int` | `int` | Get remap target for a player. |
| `setPlayers` | — | — | Resize player input config to match settings. |
| `resetTokenGuard` | — | — | Reset UI input token guard. |
| `commandAdd` | `name: string, command: string, time?: int32, bufferTime?: int32, ...` | — | Register a UI command definition. |
| `commandBufReset` | `playerNo?: int` | — | Reset command input buffers. |
| `commandGetState` | `playerNo: int, name: string` | `boolean` | Query current state of a named command. |
| `commandDebug` | `playerNo: int, prefix?: string` | — | Print command debug info. |
| `addHotkey` | `key: string, ctrl?: boolean, alt?: boolean, shift?: boolean, allowDuringPause?: boolean, debugOnly?: boolean, script: string` | `boolean` | Register a global keyboard shortcut. |
| `playerBufReset` | `playerNo?: int` | — | Reset player input buffers. |

---

## 11. Screen & Rendering

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `clearColor` | `r, g, b: int32, alpha?: int32` | — | Fill the screen with a solid color. |
| `fadeColor` | `mode: string, startFrame: int32, length: float64, r?: int32, g?: int32, b?: int32` | `boolean` | Draw a timed screen fade overlay. |
| `fadeInActive` | — | `boolean` | Check if global fade-in is active. |
| `fadeInInit` | `fade: Fade` | — | Initialize a Fade object from motif fade-in settings. |
| `fadeOutActive` | — | `boolean` | Check if global fade-out is active. |
| `fadeOutInit` | `fade: Fade` | — | Initialize a Fade object from motif fade-out settings. |
| `screenshot` | — | — | Take a screenshot on the next frame. |
| `toggleFullscreen` | `state?: boolean` | — | Toggle or set fullscreen mode. |
| `toggleVSync` | `mode?: int` | — | Toggle or set vertical sync mode. |
| `toggleWireframeDisplay` | `state?: boolean` | — | Toggle wireframe rendering. |
| `toggleLifebarDisplay` | `hide?: boolean` | — | Toggle lifebar visibility. |
| `toggleClsnDisplay` | `state?: boolean` | — | Toggle collision box display. |
| `toggleDebugDisplay` | `mode?: any, reverse?: boolean` | — | Toggle debug overlay. |
| `togglePause` | `state?: boolean` | — | Toggle or set game pause. |
| `toggleMaxPowerMode` | `state?: boolean` | — | Toggle max-power cheat mode. |
| `toggleNoSound` | `state?: boolean` | — | Toggle global mute. |
| `frameStep` | — | — | Enable single-frame stepping. |
| `setAccel` | `accel: float32` | — | Set debug time acceleration. |

---

## 12. Motif & Config

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `loadMotif` | `defPath?: string` | `table` | Load a motif and return its config table. |
| `modifyMotif` | `query: string, value: any` | — | Modify a motif value by query path. |
| `setMotifElements` | `elements: table` | — | Enable/disable major motif elements. |
| `loadGameOption` | `filename?: string` | `table` | Load game options from a config file. |
| `modifyGameOption` | `query: string, value: any` | — | Modify a game option by query path. |
| `saveGameOption` | `path?: string` | — | Save current game options to file. |
| `getGameParams` | — | `table` | Get the current game parameter table. |
| `loadIni` | `filename: string` | `table` | Load an INI file into a Lua table. |
| `loadStoryboard` | `defPath: string` | `table\|nil` | Load a storyboard and set as current. |
| `modifyStoryboard` | `query: string, value: any` | — | Modify the current storyboard. |
| `runStoryboard` | — | `boolean` | Run the current storyboard for one frame. |
| `getStoryboardScene` | — | `int\|nil` | Get the current storyboard scene index. |
| `getGameStats` | — | `table` | Read accumulated game statistics. |
| `getGameStatsJson` | — | `string` | Get game statistics as JSON. |
| `setGameStatsJson` | `json: string` | — | Restore game statistics from a JSON snapshot. |
| `resetGameStats` | — | — | Clear accumulated game statistics. |
| `setScore` | `teamSide: int, score: float32` | — | Set a team's score (alias: `resetScore`). |
| `resetScore` | `teamSide: int` | — | Reset a team's score to zero. |

---

## 13. File & Utility

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `fileExists` | `path: string` | `boolean` | Check if a file exists. |
| `loadText` | `path: string` | `string\|nil` | Load a text file and return its contents. |
| `getDirectoryFiles` | `rootPath: string` | `table` | Recursively list all files under a directory. |
| `searchFile` | `filename: string, dirs: table` | `string` | Search for a file in a list of directories. |
| `jsonDecode` | `path: string` | `any` | Decode a JSON file into Lua values. |
| `jsonEncode` | `value: any, path: string` | — | Encode a Lua value to a JSON file. |
| `getCommandLineFlags` | — | `table` | Get all command-line flags. |
| `getCommandLineValue` | `flagName: string` | `string\|nil` | Get a specific command-line flag value. |
| `puts` | `text: string` | — | Print text to stdout only. |
| `printConsole` | `text: string, appendLast?: boolean` | — | Print text to the in-game console and stdout. |
| `clearConsole` | — | — | Clear the in-game console. |
| `clear` | — | — | Clear all characters' clipboard text buffers. |
| `getClipboardString` | — | `string` | Get the system clipboard string. |

---

## 14. Debug

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `loadDebugFont` | `filename: string, scale?: float32` | — | Load the debug overlay font. |
| `loadDebugInfo` | `funcs: table` | — | Register Lua functions for debug info display. |
| `loadDebugStatus` | `funcName: string` | — | Register function to draw debug status. |
| `findEntityByName` | `text: string` | — | Find entity whose name contains text. |
| `findEntityByPlayerId` | `playerId: int32` | — | Find entity by player ID. |
| `findHelperById` | `helperId: int32` | — | Find helper by helper ID. |
| `toggleDebugDisplay` | `mode?: any, reverse?: boolean` | — | Toggle debug overlay. |
| `toggleClsnDisplay` | `state?: boolean` | — | Toggle collision box display. |
| `panicError` | `message: string` | — | Raise an immediate Lua error. |
| `getStateOwnerId` | — | `int32` | Get current state owner's player ID. |
| `getStateOwnerName` | — | `string` | Get current state owner's name. |
| `getStateOwnerPlayerNo` | — | `int` | Get current state owner's player number. |

---

## 15. State & Save

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `saveState` | — | — | Request saving current state on the next frame. |
| `loadState` | — | — | Request loading a saved state on the next frame. |

---

## 16. Netplay & Replay

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `enterNetPlay` | `host: string` | — | Enter netplay as client or host. |
| `exitNetPlay` | — | — | Exit netplay mode. |
| `netPlay` | — | `boolean` | Check if netplay is active. |
| `connected` | — | `boolean` | Check if the main menu network connection is established. |
| `enterReplay` | `path: string` | `boolean` | Enter replay playback from a file. |
| `exitReplay` | — | — | Exit replay mode. |
| `replayRecord` | `path: string` | — | Start recording input to a replay file. |
| `replayStop` | — | — | Stop replay recording. |

---

## 17. Trigger Redirections

These functions change the **character context** for trigger queries within a callback. They mirror CNS trigger redirections.

| Function | Description |
|----------|-------------|
| `enemy([idx])` | Redirect to an enemy (idx: 0-based, default nearest). |
| `enemyNear([idx])` | Redirect to nearest enemy. |
| `p2()` | Redirect to P2 slot. |
| `helper([id])` | Redirect to a helper (optionally by ID). |
| `helperIndex(idx)` | Redirect to helper by index. |
| `parent()` | Redirect to parent character. |
| `partner([idx])` | Redirect to a partner. |
| `player(n)` | Redirect to player N. |
| `playerId(id)` | Redirect by player ID. |
| `playerIndex(idx)` | Redirect by player index. |
| `root()` | Redirect to root character. |
| `stateOwner()` | Redirect to the state owner. |
| `target([id])` | Redirect to a target (optionally by ID). |

---

## 18. Trigger Functions (CNS equivalents)

These functions query character and game state, mirroring CNS/ZSS trigger expressions.
Most require a character context (set with a [redirection](#17-trigger-redirections) or the current character).

| Function | Parameters | Returns | Description |
|----------|-----------|---------|-------------|
| `anim()` | — | `number` | Current animation number. |
| `animElemNo(time)` | `time: int32` | `number` | Animation element number at given time. |
| `animElemTime(elem)` | `elem: int32` | `number` | Time since given animation element became active. |
| `animExist(n)` | `n: int32` | `boolean` | Check if an animation exists. |
| `animTime()` | — | `number` | Current animation time counter. |
| `backEdge()` | — | `number` | Back edge position. |
| `backEdgeBodyDist()` | — | `number` | Distance from body to back edge. |
| `backEdgeDist()` | — | `number` | Distance to back edge. |
| `bottomEdge()` | — | `number` | Bottom edge position. |
| `canRecover()` | — | `boolean` | Character can recover from fall. |
| `clsnOverlap(idx, cboxType)` | — | `boolean` | Collision box overlap (clsn1/clsn2/size). |
| `clsnVar(idx, vname)` | — | `any` | Collision box variable. |
| `ctrl()` | — | `boolean` | Character is in control. |
| `drawGame()` | — | `boolean` | Match ended in a draw. |
| `facing()` | — | `number` | Facing direction (`1` or `-1`). |
| `fvar(idx)` | `idx: int32` | `number` | Float variable by index. |
| `gameHeight()` | — | `number` | Game coordinate height. |
| `gameTime()` | — | `number` | Global game tick count. |
| `gameWidth()` | — | `number` | Game coordinate width. |
| `gethitVar(vname)` | `vname: string` | `any` | Get-hit variable (attr, damage, fall.*, hitcount, isbound, type, yvel, zvel, etc.). |
| `helperVar(vname)` | `vname: string` | `any` | Helper variable (clsnproxy, helpertype, id, keyctrl, etc.). |
| `hitByAttr(attr)` | `attr: string` | `boolean` | Check if hit by the given attribute string (e.g. `"S, NA"`). |
| `hitCount()` | — | `number` | Number of hits landed. |
| `hitDefAttr()` | — | `string` | Current hit definition attribute string. |
| `hitDefVar(vname)` | `vname: string` | `number\|string` | Hit definition variable. |
| `hitFall()` | — | `boolean` | Character in falling state from hit. |
| `hitOver()` | — | `boolean` | Hit pause is over. |
| `hitPauseTime()` | — | `number` | Hit pause time remaining. |
| `hitVelX()` | — | `number` | Hit velocity X (facing-relative). |
| `hitVelY()` | — | `number` | Hit velocity Y. |
| `hitVelZ()` | — | `number` | Hit velocity Z. |
| `id()` | — | `number` | Character unique ID. |
| `inCustomAnim()` | — | `boolean` | Character uses a custom animation. |
| `inCustomState()` | — | `boolean` | Character is in a custom state. |
| `inGuardDist()` | — | `boolean` | Enemy is within guard distance. |
| `inputTime(key)` | `key: string` | `number` | Hold time of a character input key. |
| `isAsserted(flag)` | `flag: string` | `boolean` | Check if a special flag is asserted. |
| `isHelper([id])` | — | `boolean` | Check if character is a helper. |
| `isHomeTeam()` | — | `boolean` | Character is on home team. |
| `jugglePoints([id])` | — | `number` | Remaining juggle points. |
| `layerNo()` | — | `number` | Character layer number. |
| `leftEdge()` | — | `number` | Left edge position. |
| `lerp(a, b, amount)` | `float32` | `number` | Linear interpolation (clamped). |
| `life()` | — | `number` | Current life. |
| `lifeMax()` | — | `number` | Maximum life. |
| `lose()` | — | `boolean` | Character has lost. |
| `loseKO()` | — | `boolean` | Lost by KO. |
| `loseTime()` | — | `boolean` | Lost by time. |
| `map(name)` | `name: string` | `number` | Custom map value by name. |
| `matchNo()` | — | `number` | Current match number. |
| `matchOver()` | — | `boolean` | Match is over. |
| `memberNo()` | — | `number` | Team member slot (1-based). |
| `motifVar(query)` | `query: string` | `any` | Read motif config value by query path. |
| `moveContact()` | — | `number` | Move contact counter. |
| `moveGuarded()` | — | `number` | Move guarded counter. |
| `moveHit()` | — | `number` | Move hit counter. |
| `moveHitVar(vname)` | `vname: string` | `any` | Move hit variable. |
| `moveType()` | — | `string` | Move type (`"I"/"A"/"H"`). |
| `name([n])` | — | `string` | Character name (or partner/enemy with n). |
| `numHelper([id])` | — | `number` | Number of helpers. |
| `numProj()` | — | `number` | Number of projectiles. |
| `numTarget([id])` | — | `number` | Number of targets. |
| `p2Life()` | — | `number` | P2 life. |
| `p2MoveType()` | — | `string` | P2 move type. |
| `p2StateNo()` | — | `number` | P2 state number. |
| `p2DistX()` | — | `number` | Center distance to P2, X axis. |
| `p2DistY()` | — | `number` | Center distance to P2, Y axis. |
| `palNo()` | — | `number` | Character palette number. |
| `physics()` | — | `string` | Current physics (`"S"/"C"/"A"/"N"`). |
| `posX()` | — | `number` | Position X. |
| `posY()` | — | `number` | Position Y. |
| `posZ()` | — | `number` | Position Z. |
| `power()` | — | `number` | Current power. |
| `powerMax()` | — | `number` | Maximum power. |
| `prevStateNo()` | — | `number` | Previous state number. |
| `projContactTime(id)` | `id: int32` | `number` | Projectile contact time. |
| `projGuardedTime(id)` | `id: int32` | `number` | Projectile guarded time. |
| `projHitTime(id)` | `id: int32` | `number` | Projectile hit time. |
| `projVar(id, idx, vname)` | — | `any` | Projectile variable. |
| `receivedDamage()` | — | `number` | Damage received this round. |
| `receivedHits()` | — | `number` | Hits received this round. |
| `redLife()` | — | `number` | Red life value. |
| `rightEdge()` | — | `number` | Right edge position. |
| `roundNo()` | — | `number` | Current round number. |
| `roundState()` | — | `number` | Round state (0=pre, 1=intro, 2=fight, 3=over). |
| `roundsWon()` | — | `number` | Rounds won. |
| `roundTime()` | — | `number` | Current round tick count. |
| `scaleX()` | — | `number` | Current scale X. |
| `scaleY()` | — | `number` | Current scale Y. |
| `score()` | — | `number` | Character score. |
| `screenHeight()` | — | `number` | Screen height in pixels. |
| `screenPosX()` | — | `number` | Character screen X. |
| `screenWidth()` | — | `number` | Screen width in pixels. |
| `selfAnimExist(n)` | `n: int32` | `boolean` | Character owns given animation. |
| `selfStateNoExist(n)` | `n: int32` | `boolean` | Character owns given state. |
| `sign(v)` | `v: float32` | `number` | Sign of value (`-1`/`0`/`1`). |
| `soundVar(id, vname)` | — | `any` | Sound channel variable. |
| `sprPriority()` | — | `number` | Sprite render priority. |
| `stageConst(name)` | `name: string` | `number` | Stage constant by name. |
| `stageTime()` | — | `number` | Stage elapsed time. |
| `stageVar(query)` | `query: string` | `any` | Stage variable by path. |
| `stateNo()` | — | `number` | Current state number. |
| `stateType()` | — | `string` | State type (`"S"/"C"/"A"/"L"`). |
| `sysFvar(idx)` | `idx: int32` | `number` | System float variable. |
| `sysVar(idx)` | `idx: int32` | `number` | System integer variable. |
| `teamLeader()` | — | `number` | Team leader player number. |
| `teamMode()` | — | `string` | Team mode (`"single"/"simul"/"turns"/"tag"`). |
| `teamSide()` | — | `number` | Team side (1 or 2). |
| `teamSize()` | — | `number` | Team size. |
| `time()` | — | `number` | Current state time. |
| `timeElapsed()` | — | `number` | Real time elapsed (ms). |
| `timeRemaining()` | — | `number` | Round time remaining. |
| `var(idx)` | `idx: int32` | `number` | Integer variable by index. |
| `velX()` | — | `number` | Velocity X. |
| `velY()` | — | `number` | Velocity Y. |
| `velZ()` | — | `number` | Velocity Z. |
| `win()` | — | `boolean` | Character won. |
| `winKO()` | — | `boolean` | Won by KO. |
| `winPerfect()` | — | `boolean` | Won perfect. |
| `winTime()` | — | `boolean` | Won by time. |
| `xShear()` | — | `number` | X shear value. |
| `zoomVar(arg)` | `arg: string` | `number` | Zoom variable (scale, pos.x, pos.y, lag, time). |
