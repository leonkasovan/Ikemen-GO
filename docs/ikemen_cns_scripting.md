# Ikemen GO — CNS Scripting Reference

CNS (Character State) is the original Mugen scripting format used to define character behavior in Ikemen GO. Files use the `.cns`, `.st`, or `.cmd` extension. Ikemen GO extends this format with many additional state controllers and triggers beyond what Mugen 1.1 supported.

---

## Table of Contents

- [File Structure](#file-structure)
- [Character Definition File](#character-definition-file)
- [Command File](#command-file)
- [State File Format](#state-file-format)
- [StateDef Properties](#statedef-properties)
- [Trigger System](#trigger-system)
- [State Controllers](#state-controllers)
- [Triggers (Expressions)](#triggers-expressions)
- [Operators](#operators)
- [Variables](#variables)

---

## File Structure

A character in Ikemen GO is defined by a `.def` file that references other files:

```ini
[Info]
name = "CharacterName"
displayname = "Character Name"
author = "Author"
mugenversion = 1.1
; For Ikemen-native characters:
; ikemenversion = 0.99

[Files]
cmd = chars/mychar/mychar.cmd    ; Command definitions + states
st = chars/mychar/mychar.cns     ; Primary state file
st1 = chars/mychar/extra.cns     ; Additional state files
stcommon = chars/mychar/common.cns ; Character-specific common states
```

**File loading order:**
1. State files (`st`, `st0`, `st1`, ...)
2. Command file (`cmd`)
3. Character-specific common states (`stcommon`)
4. Global common states (configured in engine settings)

States loaded later do **not** override states from earlier files (except negative states, which can be overridden by characters with `ikemenversion`).

---

## Command File

The command file (`.cmd`) defines input sequences the player can execute. It contains `[Remap]`, `[Defaults]`, and `[Command]` sections.

### Remap Section

Remaps logical buttons to physical ones:

```ini
[Remap]
x = x
y = y
z = z
a = a
b = b
c = c
s = s
d = d
w = w
m = m
```

### Defaults Section

Sets default parameters for all commands:

```ini
[Defaults]
command.time = 15
command.steptime = 15
command.buffer.time = 1
command.autogreater = 0
command.buffer.hitpause = 0
command.buffer.pauseend = 0
command.buffer.shared = 0
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `command.time` | int | Maximum time (in ticks) to complete the command |
| `command.steptime` | int | Maximum time between individual steps |
| `command.buffer.time` | int | How long a completed command stays active (min 1) |
| `command.autogreater` | bool | Auto-detect greater-than-step commands |
| `command.buffer.hitpause` | bool | Buffer commands during hit pause |
| `command.buffer.pauseend` | bool | Buffer commands at end of pause |
| `command.buffer.shared` | bool | Share buffer across command lists |

### Command Section

Each command defines a named input sequence:

```ini
[Command]
name = "QCF_x"
command = ~D, DF, F, x
time = 15
buffer.time = 1
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `name` | string | Command name (referenced by `command` trigger) |
| `command` | string | Input sequence using directional and button notation |
| `time` | int | Max time to complete (overrides default) |
| `steptime` | int | Max time between steps |
| `buffer.time` | int | How long completed command stays active |
| `autogreater` | bool | Auto-detect greater-than steps |
| `buffer.hitpause` | bool | Buffer during hit pause |
| `buffer.pauseend` | bool | Buffer at end of pause |
| `buffer.shared` | bool | Share buffer |

**Command notation:**

| Symbol | Meaning |
|--------|---------|
| `U`, `D`, `F`, `B` | Up, Down, Forward, Back |
| `UF`, `UB`, `DF`, `DB` | Diagonal directions |
| `a`–`c`, `x`–`z`, `s`, `d`, `w`, `m` | Buttons |
| `~` | Release (must release button/direction) |
| `$` | Hold (direction must be held) |
| `/` | Must be held for at least 1 tick |
| `>` | Strict: no other inputs between steps |
| `+` | Simultaneous press |

---

## State File Format

CNS state files use an INI-like format. Each state is defined by a `[StateDef]` header followed by one or more `[State]` blocks containing controllers.

### Basic Structure

```ini
; Comments start with semicolons

[StateDef 200]           ; State number 200
type = S                 ; Standing state
movetype = A             ; Attack move type
physics = S              ; Standing physics
anim = 200               ; Animation 200
ctrl = 0                 ; No control

[State 200, Description]
type = ChangeState
trigger1 = AnimTime = 0
value = 0
ctrl = 1
```

---

## StateDef Properties

The `[StateDef <number>]` section defines properties for a state:

| Property | Values | Description |
|----------|--------|-------------|
| `type` | `S`, `C`, `A`, `L`, `U` | State type: Standing, Crouching, Air, Liedown, Unchanged |
| `movetype` | `I`, `A`, `H`, `U` | Move type: Idle, Attack, GetHit, Unchanged |
| `physics` | `S`, `C`, `A`, `N`, `U` | Physics: Standing, Crouching, Air, None, Unchanged |
| `anim` | int | Animation to change to on state entry |
| `velset` | float, float, float | Set velocity (x, y, z) on entry |
| `ctrl` | bool | Set control flag on entry |
| `poweradd` | int | Power to add on entry |
| `juggle` | int | Juggle points required |
| `facep2` | bool | Face opponent on entry |
| `hitdefpersist` | bool | Keep HitDef when entering this state |
| `movehitpersist` | bool | Keep move hit counters |
| `hitcountpersist` | bool | Keep hit count |
| `sprpriority` | int | Sprite drawing priority |

**Special state numbers:**
- Negative states (e.g., `-1`, `-2`, `-3`) run every tick regardless of current state
- State `-10` / `+1` is a special auto-increment state

---

## Trigger System

Each `[State]` block has a `type` parameter specifying the controller and trigger conditions that determine when it executes.

### Trigger Format

```ini
[State 200, Attack]
type = ChangeState
triggerall = Alive                    ; Must always be true
trigger1 = command = "QCF_x"         ; First condition set
trigger1 = power >= 1000             ; AND with above
trigger2 = command = "QCF_y"         ; OR alternative
trigger2 = power >= 2000             ; AND with this trigger2
value = 1000
```

**Rules:**
- `triggerall` — Must be true for any trigger set to activate (AND with everything)
- `trigger1` — First condition set. Multiple `trigger1` lines are ANDed together
- `trigger2` — Second condition set (ORed with trigger1). Multiple `trigger2` lines are ANDed
- `trigger3`, `trigger4`, ... — Additional OR sets
- `triggerall = 0` — Disables the entire controller

### Controller Properties

| Property | Type | Description |
|----------|------|-------------|
| `type` | string | State controller type (mandatory) |
| `persistent` | int | How often to execute (1 = every tick, 0 = once per state entry, N = every N ticks) |
| `ignorehitpause` | bool | Execute even during hit pause |

---

## State Controllers

### Movement & Position

#### ChangeState
Changes the character's current state.

```ini
type = ChangeState
value = 200         ; State number
ctrl = 1            ; Set control (optional)
anim = 200          ; Set animation (optional)
```

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `value` | int | Yes | Target state number |
| `ctrl` | bool | No | Control flag |
| `anim` | int | No | Animation number |
| `readplayerid` | int | No | Read player ID for animation (Ikemen) |
| `continue` | bool | No | Continue animation from current frame (Ikemen) |

#### SelfState
Same as ChangeState but returns control to the character's own state file.

#### VelSet / VelAdd / VelMul
Set, add to, or multiply velocity.

```ini
type = VelSet
x = -5.0
y = -8.0
z = 0                ; Ikemen extension
```

#### PosSet / PosAdd
Set or add to position.

```ini
type = PosAdd
x = 5.0
y = 0
z = 0                ; Ikemen extension
```

#### PosFreeze
Freeze position for the current tick.

```ini
type = PosFreeze
value = 1
```

#### Gravity
Apply standard gravity acceleration.

```ini
type = Gravity
```

#### Turn
Turn the character to face the opposite direction.

```ini
type = Turn
```

---

### State & Control

#### StateTypeSet
Set the character's state type, move type, and/or physics.

```ini
type = StateTypeSet
statetype = A        ; S, C, A, L
movetype = A         ; I, A, H
physics = A          ; S, C, A, N
```

#### CtrlSet
Set whether the player has control.

```ini
type = CtrlSet
value = 1
```

---

### Life, Power & Resources

#### LifeAdd / LifeSet

```ini
type = LifeAdd
value = -50
kill = 1             ; Can this kill? (default: 1)
absolute = 0         ; Ignore defence scaling
```

#### PowerAdd / PowerSet

```ini
type = PowerAdd
value = 100
```

#### RedLifeAdd / RedLifeSet (Ikemen)

```ini
type = RedLifeAdd
value = -20
absolute = 0
```

#### DizzyPointsAdd / DizzyPointsSet (Ikemen)

```ini
type = DizzyPointsAdd
value = 100
```

#### GuardPointsAdd / GuardPointsSet (Ikemen)

```ini
type = GuardPointsAdd
value = -50
```

#### ScoreAdd (Ikemen)

```ini
type = ScoreAdd
value = 100
```

---

### Variables

#### VarSet / VarAdd

```ini
type = VarSet
var(0) = 5           ; Integer variable
fvar(0) = 1.5        ; Float variable
sysvar(0) = 10       ; System integer variable
sysfvar(0) = 2.5     ; System float variable
```

Or using `v` / `fv` shorthand:

```ini
type = VarSet
v = 0
value = 5
```

#### ParentVarSet / ParentVarAdd
Set/add to parent character's variables.

#### RootVarSet / RootVarAdd (Ikemen)
Set/add to root character's variables.

#### VarRandom
Set a variable to a random value.

```ini
type = VarRandom
v = 0
range = 0, 100       ; Min, Max
```

#### VarRangeSet
Set a range of variables at once.

```ini
type = VarRangeSet
value = 0            ; Value to set
first = 0            ; First variable index
last = 59            ; Last variable index
fvalue = 0.0         ; Float value
firstfloat = 0
lastfloat = 39
```

#### MapSet / MapAdd / MapReset (Ikemen)
Named key-value variable storage.

```ini
type = MapSet
map = "mykey"
value = 100
```

```ini
type = MapAdd
map = "mykey"
value = 1
```

#### ParentMapSet / ParentMapAdd / RootMapSet / RootMapAdd / TeamMapSet / TeamMapAdd (Ikemen)
Set/add to parent/root/teammate map variables.

---

### Combat

#### HitDef
The primary attack definition controller. This is the most complex controller with 70+ parameters.

```ini
type = HitDef
attr = S, NA                  ; Standing, Normal Attack
hitflag = MAF                 ; Can hit: Mid, Air, Fall
guardflag = MA                ; Can guard: Mid, Air
animtype = Light              ; Light, Medium, Hard, Back, Up, Diagup
damage = 20, 5                ; Hit damage, Guard damage
getpower = 50, 25             ; Power gained on hit, guard
givepower = 25, 12            ; Power given to opponent

; Pause/shake
pausetime = 8, 8              ; Self pause, opponent pause
guard.pausetime = 8, 8

; Velocities
ground.velocity = -5.0, 0     ; Ground hit velocity x, y
air.velocity = -3.0, -4.0     ; Air hit velocity
guard.velocity = -3.0         ; Guard pushback

; Sound
hitsound = S5, 0              ; S = own, F = fight (common) sounds
guardsound = S6, 0

; Spark
sparkno = S0                  ; Hit spark animation
guard.sparkno = S0
sparkxy = 0, -40              ; Spark position offset

; State transitions
p1stateno = -1                ; Attacker state on hit
p2stateno = -1                ; Defender state on hit
p1getp2facing = 0
p2facing = 0

; Fall properties
fall = 0                      ; Force fall
fall.recover = 1              ; Can recover from fall
fall.recovertime = 4
fall.damage = 0
fall.xvelocity = 0
fall.yvelocity = -4.5

; Priority
priority = 4, Hit             ; Priority level, type (Hit/Dodge/Miss)

; Advanced
numhits = 1
hitsound.channel = -1
guardsound.channel = -1
ground.type = High            ; High, Low, Trip, None
air.type = High
id = 0
chainid = -1
nochainid = -1

; Power (Ikemen extensions)
hitpower = 0
guardpower = 0

; Score (Ikemen)
score = 0
score.guard = 0

; Guard-related (Ikemen)
guardcount = 1
guardko = 0
guarddist = 0
guardbreak = 0
```

**Attribute string format:** `<statetype>, <attack_type>`
- State types: `S` (standing), `C` (crouching), `A` (air)
- Attack types: `NA` (normal attack), `SA` (special attack), `HA` (hyper attack), `NT` (normal throw), `ST` (special throw), `HT` (hyper throw), `NP` (normal projectile), `SP` (special projectile), `HP` (hyper projectile)

#### ReversalDef
Defines a reversal/counter attack. Uses similar parameters to HitDef with `reversal.attr` for specifying which attacks can be reversed.

#### HitOverride
Override the default hit response.

```ini
type = HitOverride
attr = SCA, AA, AP, AT       ; Attributes to override
slot = 0                      ; Override slot (0-7)
stateno = 200                 ; State to go to
time = 1                      ; Duration
forceair = 0
```

#### ModifyHitDef (Ikemen)
Modify an active HitDef. Same parameters as HitDef.

#### ModifyReversalDef (Ikemen)
Modify an active ReversalDef.

---

### Hit Response

#### HitBy / NotHitBy
Specify what attacks can/cannot hit this character.

**New syntax (Ikemen):**
```ini
type = HitBy
attr = SCA, AA, AP, AT
slot = 0
time = 1
```

**Legacy syntax:**
```ini
type = HitBy
value = SCA, AA, AP, AT      ; Slot 0
value2 = SCA, NA              ; Slot 1
time = 1
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `attr` | string | Attack attributes (new syntax) |
| `slot` | int | Slot number |
| `time` | int | Duration |
| `stack` | bool | Stack with existing restrictions |
| `playerno` | int | Filter by player number |
| `playerid` | int | Filter by player ID |
| `value` | string | Legacy: Slot 0 attributes |
| `value2` | string | Legacy: Slot 1 attributes |

#### HitVelSet
Apply the hit velocity to the character.

```ini
type = HitVelSet
x = 1                         ; Apply X velocity
y = 1                         ; Apply Y velocity
z = 0                         ; Apply Z velocity (Ikemen)
```

#### HitFallDamage
Trigger fall damage from the last hit.

#### HitFallVel
Apply fall velocity from the last hit.

#### HitFallSet
Set fall state properties.

```ini
type = HitFallSet
value = -1                    ; -1 = no change, 0/1 = set
xvel = 0                      ; X velocity on fall
yvel = 0                      ; Y velocity on fall
```

#### HitAdd
Add to the hit counter.

```ini
type = HitAdd
value = 1
```

#### MoveHitReset
Reset the move contact/hit/guarded counters.

```ini
type = MoveHitReset
```

#### GetHitVarSet (Ikemen)
Directly set GetHitVar properties.

---

### Projectiles

#### Projectile
Create a projectile. Includes all HitDef parameters plus projectile-specific ones.

```ini
type = Projectile
projid = 100
projanim = 200
projhitanim = 201
projremanim = 202
projcancelanim = 203
velocity = 5.0, 0
velmul = 1.0, 1.0
remvelocity = 0, 0
accel = 0, 0
projscale = 1.0, 1.0
projangle = 0
projremove = 1
projremovetime = -1
projmisstime = 0
projhits = 1
projpriority = 1
projsprpriority = 3
projedgebound = 40
projstagebound = 40
projheightbound = -240, 1
offset = 20, -60
postype = p1                  ; p1, p2, front, back, left, right, none
projlayer = 0
```

#### ModifyProjectile (Ikemen)
Modify an existing projectile by ID. Same parameters as Projectile.

---

### Helpers

#### Helper
Create a helper character.

```ini
type = Helper
helpertype = normal           ; normal or player
name = "MyHelper"
id = 1000
pos = 0, 0
postype = p1                  ; p1, p2, front, back, left, right, none
facing = 1
stateno = 5000
keyctrl = 0
ownpal = 1
size.xscale = 1.0
size.yscale = 1.0
size.ground.back = 15
size.ground.front = 16
size.air.back = 12
size.air.front = 12
size.height = 60
size.proj.doscale = 0
immortal = 0                  ; Ikemen: cannot be KO'd
preserve = 0                  ; Ikemen: preserve across rounds
standby = 0                   ; Ikemen: start in standby mode
```

#### DestroySelf
Destroy the current helper.

```ini
type = DestroySelf
recursive = 1                 ; Also destroy child helpers
removeexplods = 1             ; Remove associated explods
```

---

### Visual Effects

#### Explod
Create a visual effect/explosion.

```ini
type = Explod
anim = F100                   ; F = fightfx, S = own anim
id = -1
pos = 0, -60
postype = p1
facing = 1
vfacing = 1
bindtime = -1
velocity = 0, 0
accel = 0, 0
scale = 1.0, 1.0
removetime = -2               ; -2 = animation end, -1 = infinite
sprpriority = 2
ontop = 0
ownpal = 0
removeongethit = 0
ignorehitpause = 0
; Transparency
trans = none                  ; none, add, add1, sub, default, addalpha
alpha = 256, 0                ; Source, Dest (for addalpha)
; Interpolation (Ikemen)
interpolate.blend = 0
interpolate.scale = 0
interpolate.angle = 0
interpolate.offset = 0
```

#### ModifyExplod (Ikemen)
Modify an existing explod by ID/index.

#### RemoveExplod
Remove explods.

```ini
type = RemoveExplod
id = -1                       ; -1 = all
index = -1                    ; Specific index
```

#### ExplodBindTime
Set the bind time for an explod.

```ini
type = ExplodBindTime
id = 100
time = 0
value = 5
```

#### AfterImage / AfterImageTime
Create/control after-image trail effects.

```ini
type = AfterImage
time = 30
length = 20
paladd = 0, 0, 0
palmul = 1.0, 1.0, 1.0
timegap = 1
framegap = 4
trans = add
```

---

### Sound

#### PlaySnd

```ini
type = PlaySnd
value = S100, 0               ; S = own, F = common
channel = -1                  ; -1 = auto, 0+ = specific channel
volume = 100
pan = 0
loop = 0
freqmul = 1.0                 ; Frequency multiplier
priority = 0
```

#### StopSnd

```ini
type = StopSnd
channel = 1
```

#### SndPan

```ini
type = SndPan
channel = 1
pan = 0
abspan = 0
```

---

### Display & Palette

#### PalFX / AllPalFX / BgPalFX
Palette effects applied to the character, all characters, or background.

```ini
type = PalFX
time = 30
add = 100, 100, 100           ; RGB add
mul = 256, 256, 256            ; RGB multiply (256 = 1.0)
sinadd = 0, 0, 0, 0           ; Sinusoidal add (R, G, B, period)
color = 256                    ; Color level (256 = full, 0 = grayscale)
hue = 0                        ; Hue rotation
invertall = 0                  ; Invert colors
```

#### Trans
Set transparency mode.

```ini
type = Trans
trans = addalpha
alpha = 200, 128
```

**Trans types:** `none`, `add`, `add1`, `sub`, `default`, `addalpha`

#### AngleSet / AngleAdd / AngleMul
Control sprite rotation.

```ini
type = AngleSet
value = 45.0
```

#### AngleDraw
Draw with rotation. This controls whether angle values are actually applied.

```ini
type = AngleDraw
value = 1
scale = 1.0, 1.0
```

#### SprPriority
Set sprite drawing priority.

```ini
type = SprPriority
value = 2
```

#### RemapPal
Remap palette colors. Source and destination palette groups.

```ini
type = RemapPal
source = 1, 1
dest = 1, 2
```

#### RemapSprite (Ikemen)
Remap sprite graphics.

---

### Screen Effects

#### EnvShake
Shake the screen.

```ini
type = EnvShake
time = 20
freq = 60.0
ampl = -4
phase = 90
dir = 0                       ; 0 = vertical, non-zero = horizontal (Ikemen)
```

#### EnvColor
Fill the screen with a color.

```ini
type = EnvColor
value = 255, 255, 255
time = 10
under = 0
```

#### FallEnvShake
Apply environmental shake based on the last hit's fall properties.

---

### Target Manipulation

#### TargetState
Force a target into a specific state.

```ini
type = TargetState
value = 5000
id = -1                       ; Target ID, -1 = all
```

#### TargetBind / BindToTarget
Bind to/from a target.

```ini
type = TargetBind
id = -1
time = 1
pos = 0, 0
```

#### TargetLifeAdd / TargetPowerAdd
Add life or power to targets.

```ini
type = TargetLifeAdd
id = -1
value = -100
kill = 1
absolute = 0
```

#### TargetVelSet / TargetVelAdd
Set or add to target velocity.

#### TargetFacing
Set target facing direction.

#### TargetDrop
Release a target.

```ini
type = TargetDrop
excludeid = -1
keepone = 1
```

#### TargetDizzyPointsAdd / TargetGuardPointsAdd / TargetRedLifeAdd / TargetScoreAdd (Ikemen)
Add to target's dizzy points, guard points, red life, or score.

---

### Binding

#### BindToParent / BindToRoot
Bind helper to parent or root character.

```ini
type = BindToParent
time = 1
facing = 0
pos = 0, 0
```

---

### Pause

#### Pause

```ini
type = Pause
time = 30
movetime = 0
pausebg = 1
endcmdbuftime = 0
```

#### SuperPause

```ini
type = SuperPause
time = 30
movetime = 0
pausebg = 1
anim = -1
pos = 0, 0
darken = 1
p2defmul = 1.0
poweradd = 0
unhittable = 1
```

---

### Assertions

#### AssertSpecial
Set special behavior flags.

```ini
type = AssertSpecial
flag = invisible
flag2 = noshadow
```

**Mugen character flags:**
`invisible`, `noairguard`, `noautoturn`, `nocrouchguard`, `nojugglecheck`, `noshadow`, `nostandguard`, `nowalk`, `unguardable`

**Mugen global flags:**
`globalnoshadow`, `nobardisplay`, `nobg`, `nofg`, `noko`, `nokoslow`, `nokosnd`, `nomusic`, `roundnotover`, `timerfreeze`, `intro`

**Ikemen character flags:**
`animatehitpause`, `animfreeze`, `autoguard`, `drawunder`, `noaibuttonjam`, `noaicheat`, `noailevel`, `noairjump`, `nobrake`, `nocombodisplay`, `nocornerpush`, `nocrouch`, `nodizzypointsdamage`, `nofacedisplay`, `nofacep2`, `nofallcount`, `nofalldefenceup`, `nofallhitflag`, `nofastrecoverfromliedown`, `nogetupfromliedown`, `noguardbardisplay`, `noguarddamage`, `noguardko`, `noguardpointsdamage`, `nohardcodedkeys`, `nohitdamage`, `noinput`, `nointroreset`, `nojump`, `nokofall`, `nokovelocity`, `nolifebaraction`, `nolifebardisplay`, `nomakedust`, `nonamedisplay`, `nopowerbardisplay`, `noredlifedamage`, `noscore`, `nostand`, `nostunbardisplay`, `noturntarget`, `nowinicondisplay`, `postroundinput`, `projtypecollision`, `runfirst`, `runlast`, `sizepushonly`, `nodestroyself`

**Ikemen global flags:**
`camerafreeze`, `globalnoko`, `roundnotskip`, `roundfreeze`, `skipfightdisplay`, `skipkodisplay`, `skiprounddisplay`, `skipwindisplay`

| Parameter | Type | Description |
|-----------|------|-------------|
| `flag` | string | Primary flag (mandatory) |
| `flag2` ... `flag8` | string | Additional flags |
| `enabled` | bool | Enable or disable the assertion |

---

### Collision

#### Width
Set collision box width.

```ini
type = Width
edge = 0, 0                  ; Front, Back
player = 0, 0
value = 0, 0
```

#### AttackDist
Set the distance at which this attack triggers opponent guarding.

```ini
type = AttackDist
value = 160
```

#### AttackMulSet
Set attack multiplier affecting all damage dealt.

```ini
type = AttackMulSet
value = 1.0                   ; Damage multiplier
; Ikemen extensions:
redlife = 1.0
dizzypoints = 1.0
guardpoints = 1.0
```

#### DefenceMulSet
Set defence multiplier.

```ini
type = DefenceMulSet
value = 1.0
; Ikemen extensions:
multype = 0                   ; 0 = multiply, 1 = add
onhit = 0                     ; Apply only when hit
```

#### PlayerPush
Enable/disable push collision with other characters.

```ini
type = PlayerPush
value = 1
```

#### ScreenBound
Keep the character within screen bounds.

```ini
type = ScreenBound
value = 1
movecamera = 0, 0
```

#### OverrideClsn (Ikemen)
Override collision boxes.

#### TransformClsn (Ikemen)
Apply transformation to collision boxes.

---

### Misc Controllers

#### MakeDust
Create dust effect particles.

```ini
type = MakeDust
pos = 0, 0
pos2 = 0, 0
spacing = 3
```

#### GameMakeAnim
Create a game-level animation (not attached to any character).

#### VictoryQuote
Set the victory quote index.

```ini
type = VictoryQuote
value = 0                     ; -1 = random
```

#### Null
Does nothing. Can be used as a placeholder.

```ini
type = Null
```

---

### Ikemen-Specific Controllers

#### AssertCommand
Assert a command as being active.

```ini
type = AssertCommand
name = "MyCommand"
```

#### AssertInput
Assert an input button state.

#### AssertAnalogVector
Assert an analog stick vector.

#### Camera (Ikemen)
Control the camera.

```ini
type = Camera
pos = 0, 0
```

#### Depth (Ikemen)
Set Z-axis depth.

#### Dialogue (Ikemen)
Trigger dialogue text.

#### DizzySet (Ikemen)
Set the dizzy state directly.

#### GuardBreakSet (Ikemen)
Set the guard break state.

#### Height (Ikemen)
Modify collision height.

#### GroundLevelOffset (Ikemen)
Adjust ground level for this character.

#### LifebarAction (Ikemen)
Trigger lifebar animations.

#### LoadFile / SaveFile (Ikemen)
Load/save data from/to files.

#### LoadState / SaveState (Ikemen)
Load/save character state snapshots.

#### MatchRestart (Ikemen)
Restart the current match.

#### ModifyBGCtrl / ModifyBGCtrl3d (Ikemen)
Modify background control elements.

#### ModifyBgm (Ikemen)
Modify background music.

#### ModifyPlayer (Ikemen)
Modify another player's properties.

#### ModifySnd (Ikemen)
Modify a playing sound.

#### ModifyStageBG (Ikemen)
Modify stage background elements.

#### ModifyStageVar (Ikemen)
Modify stage variables.

#### ModifyShadow / ModifyReflection (Ikemen)
Modify shadow/reflection rendering properties.

#### ModifyText (Ikemen)
Modify on-screen text objects.

#### PlayBgm (Ikemen)
Play background music.

#### PrintToConsole (Ikemen)
Print debug text to the console.

#### RoundTimeAdd / RoundTimeSet (Ikemen)
Add to or set the round timer.

#### ShiftInput (Ikemen)
Shift input assignment to a different player.

#### Storyboard (Ikemen)
Launch a storyboard sequence.

#### TagIn / TagOut (Ikemen)
Tag team member in or out.

#### TargetAdd (Ikemen)
Add a target to the target list.

#### TeamMapSet / TeamMapAdd (Ikemen)
Set/add to team member's map variables.

#### Text (Ikemen)
Create on-screen text.

#### TransformSprite (Ikemen)
Apply transformations to sprite rendering.

#### Zoom (Ikemen)
Control camera zoom.

```ini
type = Zoom
pos = 0, 0
scale = 1.0
lag = 0
time = -1
```

#### ForceFeedback (Ikemen)
Trigger rumble/haptic feedback.

```ini
type = ForceFeedback
waveform = 0
time = 10
freq = 0
ampl = 0
```

---

## Triggers (Expressions)

Triggers are expressions evaluated in conditions. They return values that can be compared or combined with operators.

### Redirect Prefixes

Redirects allow querying another character's properties:

| Redirect | Syntax | Description |
|----------|--------|-------------|
| `player(N)` | `player(1), life` | Player by index (required arg) |
| `playerid(N)` | `playerid(5), pos x` | Player by unique ID |
| `playerindex(N)` | `playerindex(0), anim` | Player by slot index |
| `helperindex(N)` | `helperindex(0), var(0)` | Helper by index |
| `parent` | `parent, life` | Parent character |
| `root` | `root, stateno` | Root character |
| `helper(ID)` | `helper(1000), pos x` | Helper by ID |
| `target(ID)` | `target(-1), stateno` | Target by ID |
| `partner(N)` | `partner, life` | Team partner |
| `enemy(N)` | `enemy, stateno` | Enemy by index |
| `enemynear(N)` | `enemynear, p2dist x` | Nearest enemy |
| `p2` | `p2, life` | Opponent (Player 2) |
| `stateowner` | `stateowner, var(0)` | State owner |

### Basic State Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `anim` | int | Current animation number |
| `animelemno(N)` | int | Animation element number at time N |
| `animelemtime(N)` | int | Time since animation element N started |
| `animexist(N)` | bool | Does animation N exist |
| `animtime` | int | Time remaining in current animation (negative until end) |
| `ctrl` | bool | Does player have control |
| `facing` | int | Facing direction (1 = right, -1 = left) |
| `stateno` | int | Current state number |
| `prevstateno` | int | Previous state number |
| `statetype` | compare | State type (`= S`, `= C`, `= A`, `= L`) |
| `movetype` | compare | Move type (`= I`, `= A`, `= H`) |
| `physics` | compare | Physics type (`= S`, `= C`, `= A`, `= N`) (Ikemen) |
| `time` | int | Ticks in current state |
| `alive` | bool | Is character alive |
| `id` | int | Unique character ID |

### Position & Velocity Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `pos x` / `pos y` / `pos z` | float | Character position |
| `vel x` / `vel y` / `vel z` | float | Character velocity |
| `screenpos x` / `screenpos y` | float | Screen-space position |
| `camerapos x` / `camerapos y` | float | Camera position |
| `p2dist x` / `p2dist y` / `p2dist z` | float | Distance to opponent |
| `p2bodydist x` / `p2bodydist y` / `p2bodydist z` | float | Body distance to opponent |
| `parentdist x` / `parentdist y` / `parentdist z` | float | Distance to parent |
| `rootdist x` / `rootdist y` / `rootdist z` | float | Distance to root |
| `backedge` | float | Back screen edge position |
| `backedgedist` | float | Distance to back edge |
| `backedgebodydist` | float | Body distance to back edge |
| `frontedge` | float | Front screen edge position |
| `frontedgedist` | float | Distance to front edge |
| `frontedgebodydist` | float | Body distance to front edge |
| `leftedge` / `rightedge` | float | Left/right edge position |
| `topedge` / `bottomedge` | float | Top/bottom edge position |
| `topbounddist` / `botbounddist` | float | Distance to top/bottom bound |
| `topboundbodydist` / `botboundbodydist` | float | Body distance to top/bottom bound |
| `stagebackedgedist` | float | Distance to stage back edge (Ikemen) |
| `stagefrontedgedist` | float | Distance to stage front edge (Ikemen) |

### Life, Power & Resource Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `life` | int | Current life |
| `lifemax` | int | Maximum life |
| `p2life` | int | Opponent's life |
| `power` | int | Current power |
| `powermax` | int | Maximum power |
| `canrecover` | bool | Can recover from knockdown |
| `redlife` | int | Red (recoverable) life (Ikemen) |
| `dizzy` | bool | Is dizzy (Ikemen) |
| `dizzypoints` | int | Current dizzy points (Ikemen) |
| `dizzypointsmax` | int | Maximum dizzy points (Ikemen) |
| `guardbreak` | bool | Is guard broken (Ikemen) |
| `guardcount` | int | Guard hit counter (Ikemen) |
| `guardpoints` | int | Current guard points (Ikemen) |
| `guardpointsmax` | int | Maximum guard points (Ikemen) |
| `score` | int | Current score (Ikemen) |
| `scoretotal` | int | Total score (Ikemen) |

### Hit-Related Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `movecontact` | int | Ticks since move made contact (0 = none) |
| `movehit` | int | Ticks since move hit |
| `moveguarded` | int | Ticks since move was guarded |
| `movereversed` | int | Ticks since move was reversed |
| `movecountered` | int | Ticks since move was countered (Ikemen) |
| `hitcount` | int | Hits landed in current state |
| `uniqhitcount` | int | Unique hits landed |
| `hitover` | bool | Is being-hit sequence over |
| `hitpausetime` | int | Remaining hit pause duration |
| `hitshakeover` | bool | Is hit shake finished |
| `hitfall` | bool | Is in fall state from hit |
| `hitvel x` / `hitvel y` / `hitvel z` | float | Hit-induced velocity |
| `hitdefattr` | compare | Current HitDef attributes |
| `hitbyattr` | compare | Hit-by attributes |
| `reversaldefattr` | compare | ReversalDef attributes (Ikemen) |
| `inguarddist` | bool | Is opponent in guard distance |
| `hitoverridden` | bool | Was hit overridden (Ikemen) |
| `receiveddamage` | int | Total received damage (Ikemen) |
| `receivedhits` | int | Total received hits (Ikemen) |

### GetHitVar Sub-Properties

Access via `gethitvar(<property>)`:

| Property | Return | Description |
|----------|--------|-------------|
| `animtype` | int | Hit animation type |
| `air.animtype` | int | Air hit animation type |
| `ground.animtype` | int | Ground hit animation type |
| `fall.animtype` | int | Fall animation type |
| `type` | int | Hit type |
| `airtype` | int | Air hit type |
| `groundtype` | int | Ground hit type |
| `damage` | int | Hit damage |
| `hitcount` | int | Hit count |
| `fallcount` | int | Fall count |
| `hitshaketime` | int | Hit shake time |
| `hittime` | int | Hit stun time |
| `slidetime` | int | Slide time |
| `ctrltime` | int | Time until control recovery |
| `xoff` / `yoff` / `zoff` | float | Position offsets |
| `xvel` / `yvel` / `zvel` | float | Velocities |
| `xaccel` / `yaccel` / `zaccel` | float | Acceleration |
| `chainid` | int | Chain ID |
| `guarded` | bool | Was the hit guarded |
| `isbound` | bool | Is bound |
| `fall` | bool | Triggers fall |
| `fall.damage` | int | Fall damage |
| `fall.xvel` / `fall.yvel` / `fall.zvel` | float | Fall velocities |
| `fall.recover` | bool | Can recover from fall |
| `fall.time` | int | Fall time |
| `fall.recovertime` | int | Fall recovery time |
| `fall.kill` | bool | Fall kill flag |
| `fall.envshake.time` | int | Fall env shake duration |
| `fall.envshake.freq` | float | Fall env shake frequency |
| `fall.envshake.ampl` | int | Fall env shake amplitude |
| `fall.envshake.phase` | float | Fall env shake phase |
| `fall.envshake.mul` | float | Fall env shake multiplier |
| `fall.envshake.dir` | int | Fall env shake direction |
| `attr` | compare | Hit attribute flags |
| `dizzypoints` | int | Dizzy points dealt (Ikemen) |
| `guardpoints` | int | Guard points dealt (Ikemen) |
| `playerid` | int | Hitter's player ID (Ikemen) |
| `playerno` | int | Hitter's player number (Ikemen) |
| `projid` | int | Projectile ID (Ikemen) |
| `teamside` | int | Hitter's team side (Ikemen) |
| `redlife` | int | Red life dealt (Ikemen) |
| `score` | int | Score given (Ikemen) |
| `hitdamage` / `guarddamage` | int | Specific damage values (Ikemen) |
| `power` / `hitpower` / `guardpower` | int | Power values (Ikemen) |
| `kill` | bool | Kill flag (Ikemen) |
| `priority` | int | Hit priority (Ikemen) |
| `guardcount` | int | Guard count (Ikemen) |
| `facing` | int | Facing on hit (Ikemen) |
| `ground.velocity.x` / `.y` / `.z` | float | Ground velocity (Ikemen) |
| `air.velocity.x` / `.y` / `.z` | float | Air velocity (Ikemen) |
| `down.velocity.x` / `.y` / `.z` | float | Down velocity (Ikemen) |
| `guard.velocity.x` / `.y` / `.z` | float | Guard velocity (Ikemen) |
| `airguard.velocity.x` / `.y` / `.z` | float | Air guard velocity (Ikemen) |
| `frame` | int | Hit frame (Ikemen) |
| `down.recover` | bool | Down recovery (Ikemen) |
| `down.recovertime` | int | Down recovery time (Ikemen) |
| `guardflag` | compare | Guard flag (Ikemen) |
| `stand.friction` | float | Standing friction (Ikemen) |
| `crouch.friction` | float | Crouching friction (Ikemen) |
| `keepstate` | bool | Keep state flag (Ikemen) |
| `guardko` | bool | Guard KO (Ikemen) |

### Match & Round Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `roundstate` | int | Round state (0=pre, 1=round display, 2=fight, 3=win, 4=outro) |
| `roundno` | int | Current round number |
| `roundswon` | int | Rounds won by this team |
| `roundsexisted` | int | Rounds this character has existed |
| `matchno` | int | Match number |
| `matchover` | bool | Is the match over |
| `win` | bool | Won the round |
| `winko` | bool | Won by KO |
| `winperfect` | bool | Won with full life |
| `wintime` | bool | Won by time |
| `winclutch` | bool | Won in clutch (Ikemen) |
| `winspecial` | bool | Won with special move (Ikemen) |
| `winhyper` | bool | Won with hyper move (Ikemen) |
| `lose` | bool | Lost the round |
| `loseko` | bool | Lost by KO |
| `losetime` | bool | Lost by time |
| `drawgame` | bool | Draw result |
| `firstattack` | bool | Landed first attack (Ikemen) |
| `decisiveround` | bool | Is this a decisive round (Ikemen) |
| `consecutivewins` | int | Consecutive wins (Ikemen) |

### Player & Team Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `name` | compare | Character name (string comparison) |
| `p1name` ... `p8name` | compare | Player names (p5-p8 Ikemen) |
| `authorname` | compare | Character author |
| `displayname` | compare | Display name (Ikemen) |
| `palno` | int | Current palette number |
| `teammode` | compare | Team mode (`= single`, `= simul`, `= turns`, `= tag`) |
| `teamside` | int | Team side (1 or 2) |
| `ishometeam` | bool | Is on the home team |
| `ishelper` | bool | Is this a helper character |
| `numhelper(ID)` | int | Number of helpers (optionally with specific ID) |
| `numenemy` | int | Number of enemies |
| `numpartner` | int | Number of partners |
| `numtarget(ID)` | int | Number of targets |
| `numexplod(ID)` | int | Number of explods |
| `numproj` | int | Number of active projectiles |
| `numprojid(ID)` | int | Number of projectiles with specific ID |
| `parentexist` | bool | Does parent exist |
| `playeridexist(ID)` | bool | Does player with ID exist |
| `playerno` | int | Player number (Ikemen) |
| `playernoexist(N)` | bool | Does player number exist (Ikemen) |
| `playerindexexist(N)` | bool | Does player index exist (Ikemen) |
| `helperindexexist(N)` | bool | Does helper index exist (Ikemen) |
| `numplayer` | int | Total number of players (Ikemen) |
| `teamleader` | int | Team leader index (Ikemen) |
| `teamsize` | int | Team size (Ikemen) |
| `memberno` | int | Member number in team (Ikemen) |
| `ratiolevel` | int | Ratio level (Ikemen) |
| `index` | int | Entity index (Ikemen) |

### Game & Timing Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `gametime` | int | Total game ticks |
| `gamewidth` | int | Game area width |
| `gameheight` | int | Game area height |
| `screenwidth` | int | Screen pixel width |
| `screenheight` | int | Screen pixel height |
| `camerazoom` | float | Camera zoom level |
| `tickspersecond` | int | Ticks per second |
| `ailevel` | int | AI level (0-8) |
| `ailevelf` | float | AI level as float (Ikemen) |
| `random` | int | Random number 0-999 |
| `fighttime` | int | Total fight time (Ikemen) |
| `roundtime` | int | Current round timer (Ikemen) |
| `stagetime` | int | Stage time (Ikemen) |
| `timeelapsed` | int | Time elapsed (Ikemen) |
| `timeremaining` | int | Time remaining (Ikemen) |
| `timetotal` | int | Total match time (Ikemen) |

### Projectile Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `projcontact(ID)` | bool | Projectile made contact |
| `projhit(ID)` | bool | Projectile hit |
| `projguarded(ID)` | bool | Projectile was guarded |
| `projcontacttime(ID)` | int | Time since projectile contact |
| `projhittime(ID)` | int | Time since projectile hit |
| `projguardedtime(ID)` | int | Time since projectile guarded |
| `projcanceltime(ID)` | int | Time since projectile cancelled |

### Mathematics Functions

| Function | Arguments | Return | Description |
|----------|-----------|--------|-------------|
| `abs(x)` | 1 | same | Absolute value |
| `ceil(x)` | 1 | int | Round up |
| `floor(x)` | 1 | int | Round down |
| `exp(x)` | 1 | float | e^x |
| `ln(x)` | 1 | float | Natural log |
| `log(x, base)` | 2 | float | Logarithm |
| `cos(x)` | 1 | float | Cosine (radians) |
| `sin(x)` | 1 | float | Sine (radians) |
| `tan(x)` | 1 | float | Tangent (radians) |
| `acos(x)` | 1 | float | Arc cosine |
| `asin(x)` | 1 | float | Arc sine |
| `atan(x)` | 1 | float | Arc tangent |
| `atan2(y, x)` | 2 | float | Two-argument arc tangent (Ikemen) |
| `float(x)` | 1 | float | Convert to float (Ikemen) |
| `sign(x)` | 1 | int | Sign: -1, 0, or 1 (Ikemen) |
| `round(x, places)` | 2 | int | Round to decimal places (Ikemen) |
| `min(a, b)` | 2 | same | Minimum (Ikemen) |
| `max(a, b)` | 2 | same | Maximum (Ikemen) |
| `clamp(val, min, max)` | 3 | same | Clamp value (Ikemen) |
| `lerp(a, b, t)` | 3 | same | Linear interpolation (Ikemen) |
| `rad(x)` | 1 | float | Degrees to radians (Ikemen) |
| `deg(x)` | 1 | float | Radians to degrees (Ikemen) |
| `randomrange(min, max)` | 2 | int | Random in range (Ikemen) |

### Constants

| Constant | Value |
|----------|-------|
| `pi` | 3.14159... |
| `e` | 2.71828... |

### Resolution Scaling

| Trigger | Description |
|---------|-------------|
| `const240p(x)` | Scale value from 240p baseline |
| `const480p(x)` | Scale value from 480p baseline |
| `const720p(x)` | Scale value from 720p baseline |
| `const1080p(x)` | Scale value from 1080p baseline (Ikemen) |

### Conditional

| Function | Syntax | Description |
|----------|--------|-------------|
| `ifelse(cond, a, b)` | 3 args | Returns `a` if cond is true, else `b` |
| `cond(cond, a, b)` | 3 args | Like ifelse but treats undefined separately |

### Const() Properties

Character constants accessed via `const(<property>)`:

**Data:**
`data.life`, `data.power`, `data.attack`, `data.defence`, `data.fall.defence_up`, `data.fall.defence_mul`, `data.liedown.time`, `data.airjuggle`, `data.sparkno`, `data.guard.sparkno`, `data.hitsound.channel`, `data.guardsound.channel`, `data.ko.echo`, `data.volume`, `data.intpersistindex`, `data.floatpersistindex`, `data.guardpoints`, `data.dizzypoints`

**Size:**
`size.xscale`, `size.yscale`, `size.ground.back`, `size.ground.front`, `size.air.back`, `size.air.front`, `size.height`, `size.attack.dist`, `size.proj.attack.dist`, `size.proj.doscale`, `size.head.pos.x`, `size.head.pos.y`, `size.mid.pos.x`, `size.mid.pos.y`, `size.shadowoffset`, `size.draw.offset.x`, `size.draw.offset.y`, `size.weight`, `size.pushfactor`

**Velocity:**
`velocity.walk.fwd.x`, `velocity.walk.back.x`, `velocity.run.fwd.x`, `velocity.run.fwd.y`, `velocity.run.back.x`, `velocity.run.back.y`, `velocity.jump.y`, `velocity.jump.neu.x`, `velocity.jump.fwd.x`, `velocity.jump.back.x`, `velocity.runjump.y`, `velocity.runjump.fwd.x`, `velocity.runjump.back.x`, `velocity.airjump.y`, `velocity.airjump.neu.x`, `velocity.airjump.fwd.x`, `velocity.airjump.back.x` (and many more including up/down/z axis variants)

**Movement:**
`movement.airjump.num`, `movement.airjump.height`, `movement.yaccel`, `movement.stand.friction`, `movement.crouch.friction`, `movement.stand.friction.threshold`, `movement.crouch.friction.threshold`, `movement.air.gethit.*`, `movement.down.bounce.*`, `movement.down.friction.threshold`

### StageVar() Properties

Stage variables accessed via `stagevar(<property>)`:

**Info:** `info.name`, `info.author`, `info.displayname`, `info.mugenversion`, `info.ikemenversion`

**Camera:** `camera.boundleft`, `camera.boundright`, `camera.boundhigh`, `camera.boundlow`, `camera.verticalfollow`, `camera.floortension`, `camera.tensionhigh`, `camera.tensionlow`, `camera.tension`, `camera.tensionvel`, `camera.cuthigh`, `camera.cutlow`, `camera.startzoom`, `camera.zoomout`, `camera.zoomin`, `camera.zoomindelay`, `camera.zoominspeed`, `camera.zoomoutspeed`, `camera.yscrollspeed`, `camera.ytension.enable`, `camera.autocenter`, `camera.lowestcap`

**Player Info:** `playerinfo.leftbound`, `playerinfo.rightbound`, `playerinfo.topbound`, `playerinfo.botbound`, `playerinfo.p1startx`, `playerinfo.p2startx`, `playerinfo.p1starty`, `playerinfo.p2starty`, `playerinfo.p1startz`, `playerinfo.p2startz`, `playerinfo.p1facing`, `playerinfo.p2facing`

**Stage Info:** `stageinfo.localcoord.x`, `stageinfo.localcoord.y`, `stageinfo.autoturn`, `stageinfo.resetbg`, `stageinfo.xscale`, `stageinfo.yscale`, `stageinfo.zoffset`, `stageinfo.zoffsetlink`

**Scaling:** `scaling.topz`, `scaling.botz`, `scaling.topscale`, `scaling.botscale`

**Bounds:** `bound.screenleft`, `bound.screenright`

**Shadow:** `shadow.intensity`, `shadow.color.r/g/b`, `shadow.yscale`, `shadow.ydelta`, `shadow.fade.range.begin/end`, `shadow.xshear`, `shadow.offset.x/y`

**Reflection:** `reflection.intensity`, `reflection.ydelta`, `reflection.yscale`, `reflection.offset.x/y`, `reflection.fade.range.begin/end`, `reflection.xshear`, `reflection.color.r/g/b`

### Ikemen-Specific Triggers

#### Animation

| Trigger | Return | Description |
|---------|--------|-------------|
| `animlength` | int | Total anim duration |
| `animplayerno` | int | Player owning current anim |
| `spriteplayerno` | int | Player providing sprites |
| `prevanim` | int | Previous animation |
| `selfanimexist(N)` | bool | Does own animation N exist |
| `incustomanim` | bool | In a custom animation |
| `incustomstate` | bool | In a custom state |

#### AnimElemVar Properties

Access via `animelemvar(<property>)`: `image`, `time`, `group`, `xoffset`, `yoffset`, `xscale`, `yscale`, `angle`, `alphasource`, `alphadest`, `hflip`, `vflip`, `numclsn1`, `numclsn2`

#### Display

| Trigger | Return | Description |
|---------|--------|-------------|
| `alpha source` / `alpha dest` | int | Alpha transparency values |
| `angle` | float | Current rotation angle |
| `xangle` / `yangle` | float | X/Y rotation angles |
| `scale x` / `scale y` / `scale z` | float | Scale values |
| `offset x` / `offset y` | float | Offset values |
| `xshear` | float | X shear value |
| `sprpriority` | int | Sprite priority |
| `layerno` | int | Layer number |

#### Combat

| Trigger | Return | Description |
|---------|--------|-------------|
| `attack` | float | Attack multiplier |
| `attackmul` | float | Attack multiplier |
| `defence` | float | Defence multiplier |
| `defencemul` | float | Defence multiplier |
| `combocount` | int | Current combo count |
| `airjumpcount` | int | Air jumps performed |
| `pausetime` | int | Remaining pause time |
| `groundangle` | float | Ground angle |

#### State Checks

| Trigger | Return | Description |
|---------|--------|-------------|
| `prevstatetype` | compare | Previous state type |
| `prevmovetype` | compare | Previous move type |
| `standby` | bool | In standby mode |
| `introstate` | int | Intro state |
| `outrostate` | int | Outro state |
| `selfstatenoexist(N)` | bool | Does state N exist in own files |

#### Input

| Trigger | Return | Description |
|---------|--------|-------------|
| `command = "name"` | bool | Is command active (requires `=` or `!=`) |
| `selfcommand = "name"` | bool | Is own command active |
| `inputtime(key)` | int | Frames since key input |

**InputTime keys:** `B`, `D`, `F`, `U`, `L`, `R`, `N`, `a`, `b`, `c`, `x`, `y`, `z`, `s`, `d`, `w`, `m`

| Trigger | Return | Description |
|---------|--------|-------------|
| `analog(axis)` | float | Analog stick value |

**Analog axes:** `leftx`, `lefty`, `rightx`, `righty`, `lefttrigger`, `righttrigger`

#### Game Mode

| Trigger | Return | Description |
|---------|--------|-------------|
| `gamemode = "mode"` | bool | Current game mode |
| `gameoption(opt)` | int | Game option value |
| `gamevar(prop)` | int | Game variable |
| `fightscreenstate(state)` | bool | Fight screen state |
| `fightscreenvar(prop)` | varies | Fight screen variable |
| `motifstate(state)` | int | Motif state |
| `motifvar(prop)` | varies | Motif variable |
| `isasserted(flag)` | bool | Is assertion flag active |
| `ishost` | bool | Is network host |
| `ikemenversion` | float | Ikemen version |
| `mugenversion` | float | Mugen version |

#### Var Triggers

| Trigger | Return | Description |
|---------|--------|-------------|
| `var(N)` | int | Integer variable N |
| `fvar(N)` | float | Float variable N |
| `sysvar(N)` | int | System integer variable N |
| `sysfvar(N)` | float | System float variable N |
| `map(name)` | varies | Named map variable |

#### Explod/Projectile/Sound Inspection

| Trigger | Syntax | Description |
|---------|--------|-------------|
| `explodvar(id, idx, prop)` | 3 args | Query explod properties |
| `projvar(id, idx, prop)` | 3 args | Query projectile properties |
| `soundvar(id, prop)` | 2 args | Query sound properties |
| `bgmvar(prop)` | 1 arg | Query BGM properties |
| `envshakevar(prop)` | 1 arg | Query env shake |
| `zoomvar(prop)` | 1 arg | Query zoom state |
| `palfxvar(prop)` | 1 arg | Query palette effect |
| `spritevar(prop)` | 1 arg | Query sprite properties |
| `movehitvar(prop)` | 1 arg | Query move hit data |
| `helpervar(prop)` | 1 arg | Query helper configuration |
| `clsnvar(type, id, prop)` | 3 args | Query collision box |
| `clsnoverlap(...)` | varies | Collision overlap test |
| `projclsnoverlap(...)` | varies | Projectile collision overlap |
| `stagebgvar(id, layer, prop)` | 3 args | Query stage BG element |

---

## Operators

### Arithmetic

| Operator | Description | Example |
|----------|-------------|---------|
| `+` | Addition | `life + 100` |
| `-` | Subtraction | `life - 50` |
| `*` | Multiplication | `vel x * 2` |
| `/` | Division | `power / 1000` |
| `%` | Modulo | `gametime % 60` |
| `**` | Exponentiation | `2 ** 8` |
| `-` (unary) | Negation | `-vel x` |

### Comparison

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equal | `stateno = 200` |
| `!=` | Not equal | `anim != 0` |
| `<` | Less than | `life < 500` |
| `<=` | Less or equal | `power <= 1000` |
| `>` | Greater than | `time > 10` |
| `>=` | Greater or equal | `vel y >= 0` |

### Range (CNS-specific)

```ini
trigger1 = stateno = [200, 299]     ; stateno between 200 and 299 inclusive
trigger1 = stateno = (199, 300)     ; same but exclusive on both ends
trigger1 = stateno = [200, 300)     ; inclusive start, exclusive end
```

### Logical

| Operator | Description | Example |
|----------|-------------|---------|
| `&&` | Logical AND | `alive && ctrl` |
| `\|\|` | Logical OR | `stateno = 200 \|\| stateno = 210` |
| `!` | Logical NOT | `!ctrl` |

### Bitwise

| Operator | Description |
|----------|-------------|
| `&` | Bitwise AND |
| `\|` | Bitwise OR |
| `^` | Bitwise XOR |
| `~` | Bitwise NOT |

### Operator Precedence (highest to lowest)

1. `**` (exponentiation)
2. `*`, `/`, `%`
3. `+`, `-`
4. `>`, `>=`, `<`, `<=`
5. `=`, `!=`
6. `&` (bitwise AND)
7. `^` (bitwise XOR)
8. `|` (bitwise OR)
9. `&&` (logical AND)
10. `^^` (logical XOR)
11. `||` (logical OR)

---

## Variables

### Integer Variables
- `var(0)` through `var(59)` — Character integer variables
- `sysvar(0)` through `sysvar(4)` — System integer variables

### Float Variables
- `fvar(0)` through `fvar(39)` — Character float variables
- `sysfvar(0)` through `sysfvar(4)` — System float variables

### Map Variables (Ikemen)
Named key-value pairs: `map("keyname")`. Can store any numeric value and are accessed by string keys rather than indices.

### Variable Persistence
By default, variables persist across state transitions within a round. Variables with indices below `data.intpersistindex` / `data.floatpersistindex` persist across rounds.

---

*This document was generated from the Ikemen GO source code (`compiler.go` and `compiler_functions.go`).*
