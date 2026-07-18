# Ikemen GO — ZSS Scripting Reference

ZSS (Z State Script) is Ikemen GO's native scripting language for character state definitions. It replaces Mugen's INI-based CNS format with a structured, C-like syntax that supports functions, control flow, local variables, and stricter error checking.

---

## Table of Contents

- [Overview](#overview)
- [File Structure](#file-structure)
- [StateDef Declaration](#statedef-declaration)
- [State Controllers](#state-controllers)
- [Control Flow](#control-flow)
- [Functions](#functions)
- [Variables](#variables)
- [Expressions & Triggers](#expressions--triggers)
- [Operators](#operators)
- [Key Differences from CNS](#key-differences-from-cns)

---

## Overview

ZSS files use the `.zss` extension. Compared to CNS:

- **Stricter error handling**: ZSS does not tolerate malformed expressions or missing parameters
- **Structured control flow**: `if`/`else`, `switch`/`case`, `for`/`while` loops
- **Functions**: Reusable code blocks with arguments and return values
- **Local variables**: Scoped with `let` and `$` prefix
- **Semicolons** terminate statements
- **Curly braces** `{}` delimit blocks
- **All triggers and state controllers from CNS are available** — the trigger/controller set is identical

---

## File Structure

A ZSS file contains `[StateDef]` blocks and `[Function]` definitions at the top level. The character `.def` file references `.zss` files in the same way as `.cns` files:

```ini
; In the .def file:
[Files]
st = chars/mychar/mychar.zss
st1 = chars/mychar/extra.zss
cmd = chars/mychar/mychar.cmd     ; Commands are still in .cmd format
```

If a filename in the `.def` doesn't have an extension, Ikemen first tries loading it as-is, then attempts appending `.zss`.

### File Loading Priority

1. State files (`st`, `st0`, `st1`, ...)
2. Command file (`cmd`) — states embedded in the command file
3. Character stcommon (`stcommon`)
4. Global common states

States loaded later do **not** replace states from earlier files. Negative states can be overridden by characters declaring `ikemenversion`.

---

## StateDef Declaration

### Syntax

```
[StateDef <number>; <properties>]
<state body>
```

The state number and properties are declared inside square brackets with semicolons separating properties. The body follows directly without additional brackets.

### Examples

```
[StateDef 200; Type: S; MoveType: A; Physics: S; Anim: 200; Ctrl: 0]

if Time = 0 {
    PlaySnd{value: S1,0};
}
if AnimTime = 0 {
    ChangeState{value: 0; ctrl: 1};
}
```

### StateDef Properties

Properties use a `Key: Value` syntax separated by semicolons:

| Property | Values | Description |
|----------|--------|-------------|
| `Type` | `S`, `C`, `A`, `L`, `U` | State type |
| `MoveType` | `I`, `A`, `H`, `U` | Move type |
| `Physics` | `S`, `C`, `A`, `N`, `U` | Physics type |
| `Anim` | int | Animation on entry |
| `Ctrl` | bool | Control on entry |
| `VelSet` | float, float, float | Velocity on entry (x, y, z) |
| `PowerAdd` | int | Power added on entry |
| `Juggle` | int | Juggle points |
| `FaceP2` | bool | Face opponent on entry |
| `HitDefPersist` | bool | Preserve HitDef |
| `MoveHitPersist` | bool | Preserve move hit counters |
| `HitCountPersist` | bool | Preserve hit count |
| `SprPriority` | int | Sprite priority |

### Special State Numbers

- **Negative states** (`-1`, `-2`, `-3`): Run every tick
- **State +1**: Auto-increment state, declared as `[StateDef +1]`
- **Constants**: `[StateDef Const(<name>)]` — Uses a constant for the state number

```
[StateDef -1]
; Runs every tick regardless of current state

[StateDef +1]
; Auto-increment state number

[StateDef Const(MySpecialState)]
; Uses a defined constant value
```

---

## State Controllers

In ZSS, state controllers use a function-call-like syntax with curly braces and semicolons:

```
ControllerName{param1: value1; param2: value2};
```

### Syntax Rules

1. Controller name is case-insensitive
2. Parameters are `name: value` pairs separated by semicolons
3. The controller block is terminated by `};`
4. Expressions can be used for values

### Examples

```
ChangeState{value: 200; ctrl: 1; anim: 200};

VelSet{x: -5.0; y: -8.0};

HitDef{
    attr: S, NA;
    damage: 30, 5;
    getpower: 50, 25;
    pausetime: 8, 8;
    guard.pausetime: 8, 8;
    sparkno: S0;
    hitsound: S5, 0;
    ground.velocity: -5.0, 0;
    air.velocity: -3.0, -4.0;
};

PlaySnd{value: S100, 0; channel: -1; volume: 100};

Explod{
    anim: F100;
    pos: 0, -60;
    postype: p1;
    removetime: -2;
    scale: 1.0, 1.0;
};
```

### All Available Controllers

ZSS supports the exact same set of state controllers as CNS. See the [CNS Scripting Reference](ikemen_cns_scripting.md#state-controllers) for the complete list of controllers and their parameters. The only difference is the syntax:

**CNS format:**
```ini
[State 200, Attack]
type = HitDef
trigger1 = AnimElem = 3
attr = S, NA
damage = 30, 5
```

**ZSS format:**
```
if AnimElem = 3 {
    HitDef{attr: S, NA; damage: 30, 5};
}
```

### Controller Categories Summary

**Movement:** `ChangeState`, `SelfState`, `VelSet`, `VelAdd`, `VelMul`, `PosSet`, `PosAdd`, `PosFreeze`, `Gravity`, `Turn`

**State:** `StateTypeSet`, `CtrlSet`

**Combat:** `HitDef`, `ReversalDef`, `Projectile`, `HitBy`, `NotHitBy`, `HitOverride`, `HitVelSet`, `HitFallSet`, `HitFallVel`, `HitFallDamage`, `HitAdd`, `MoveHitReset`, `AttackDist`, `AttackMulSet`, `DefenceMulSet`

**Resources:** `LifeAdd`, `LifeSet`, `PowerAdd`, `PowerSet`, `RedLifeAdd`, `RedLifeSet`, `DizzyPointsAdd`, `DizzyPointsSet`, `GuardPointsAdd`, `GuardPointsSet`, `ScoreAdd`

**Variables:** `VarSet`, `VarAdd`, `ParentVarSet`, `ParentVarAdd`, `RootVarSet`, `RootVarAdd`, `VarRandom`, `VarRangeSet`, `MapSet`, `MapAdd`, `MapReset`, `ParentMapSet`, `ParentMapAdd`, `RootMapSet`, `RootMapAdd`, `TeamMapSet`, `TeamMapAdd`

**Helpers:** `Helper`, `DestroySelf`

**Targets:** `TargetState`, `TargetBind`, `BindToTarget`, `TargetLifeAdd`, `TargetPowerAdd`, `TargetVelSet`, `TargetVelAdd`, `TargetFacing`, `TargetDrop`, `TargetAdd`, `TargetDizzyPointsAdd`, `TargetGuardPointsAdd`, `TargetRedLifeAdd`, `TargetScoreAdd`

**Visual:** `Explod`, `ModifyExplod`, `RemoveExplod`, `ExplodBindTime`, `AfterImage`, `AfterImageTime`, `PalFX`, `AllPalFX`, `BgPalFX`, `EnvColor`, `Trans`, `AngleSet`, `AngleAdd`, `AngleMul`, `AngleDraw`, `SprPriority`, `RemapPal`, `RemapSprite`, `Width`, `Offset`, `ScreenBound`

**Sound:** `PlaySnd`, `StopSnd`, `SndPan`, `ModifySnd`, `PlayBgm`, `ModifyBgm`

**Screen:** `EnvShake`, `FallEnvShake`, `Zoom`, `Camera`

**Assertions:** `AssertSpecial`, `AssertCommand`, `AssertInput`, `AssertAnalogVector`

**Binding:** `BindToParent`, `BindToRoot`, `PlayerPush`

**Pause:** `Pause`, `SuperPause`

**Modify (Ikemen):** `ModifyHitDef`, `ModifyReversalDef`, `ModifyProjectile`, `ModifyPlayer`, `ModifyStageVar`, `ModifyStageBG`, `ModifyBGCtrl`, `ModifyBGCtrl3d`, `ModifyShadow`, `ModifyReflection`, `ModifyText`, `GetHitVarSet`

**Misc:** `Null`, `VictoryQuote`, `MakeDust`, `GameMakeAnim`, `ForceFeedback`, `DisplayToClipboard`, `AppendToClipboard`, `ClearClipboard`, `PrintToConsole`, `Text`, `Dialogue`, `Storyboard`, `MatchRestart`, `TagIn`, `TagOut`, `ShiftInput`, `LoadFile`, `SaveFile`, `LoadState`, `SaveState`, `LifebarAction`, `RoundTimeAdd`, `RoundTimeSet`, `DizzySet`, `GuardBreakSet`, `GroundLevelOffset`, `Height`, `Depth`, `OverrideClsn`, `TransformClsn`, `TransformSprite`

---

## Control Flow

ZSS supports structured programming constructs that replace the trigger system used in CNS.

### If / Else

```
if <condition> {
    <statements>
} else if <condition> {
    <statements>
} else {
    <statements>
}
```

**Examples:**

```
if Time = 0 {
    VelSet{x: 5.0; y: -8.0};
    PlaySnd{value: S1,0};
}

if command = "QCF_x" && power >= 1000 {
    ChangeState{value: 1000};
} else if command = "QCF_y" && power >= 2000 {
    ChangeState{value: 1100};
}

if statetype = A {
    if vel y > 0 {
        ChangeState{value: 52};
    }
}
```

### Switch / Case

```
switch <expression> {
case <value>:
    <statements>
case <value>:
    <statements>
default:
    <statements>
}
```

Switch statements are internally converted to if/else-if/else chains.

**Example:**

```
switch anim {
case 200:
    ChangeState{value: 210};
case 210:
    ChangeState{value: 220};
default:
    ChangeState{value: 0};
}
```

### For Loop

```
for <init>; <condition>; <step> {
    <statements>
}
```

**Example:**

```
for let $i = 0; $i < 10; $i = $i + 1 {
    Explod{
        anim: F100;
        pos: $i * 20, 0;
        removetime: 30;
    };
}
```

### While Loop

```
while <condition> {
    <statements>
}
```

**Example:**

```
let $count = numhelper(1000);
while $count > 0 {
    $count = $count - 1;
    ; Do something for each helper
}
```

### Break / Continue

Inside loops, `break` exits the loop and `continue` skips to the next iteration:

```
for let $i = 0; $i < 20; $i = $i + 1 {
    if $i = 5 {
        continue;
    }
    if $i = 15 {
        break;
    }
    ; Process
}
```

**Loop safety:** Loops have a maximum iteration count of 2500 to prevent infinite loops.

---

## Block Attributes

### IgnoreHitPause

Blocks can be marked to execute during hit pause:

```
ignorehitpause if Time = 0 {
    PlaySnd{value: S1,0};
}
```

Or for an entire block without a condition:

```
ignorehitpause {
    ; Everything here runs during hit pause
    VelSet{x: 0; y: 0};
}
```

IgnoreHitPause is inherited by child blocks — if an outer block has it, inner blocks automatically inherit it.

### Persistent

Controls how often a block re-executes. In ZSS, persistent is applied as a block-level attribute:

```
persistent(2) if command = "QCF_x" {
    ChangeState{value: 1000};
}
```

| Value | Behavior |
|-------|----------|
| `persistent(0)` | Execute only once per state entry (aka infinite persistent) |
| `persistent(2)` | Execute every 2 ticks |
| `persistent(N)` | Execute every N ticks |

**Note:** `persistent(1)` is an error in ZSS (it's the default behavior and would be meaningless). Persistent cannot be used in negative states or functions.

### Combining Attributes

Attributes can be combined on a single block:

```
ignorehitpause persistent(0) if AnimElem = 1 {
    PlaySnd{value: S0,0};
}
```

---

## Functions

ZSS supports reusable functions with arguments and return values.

### Definition

```
[Function <name>(<args>) <returns>]
<body>
```

Functions are defined at the top level alongside StateDefs, inside `[Function ...]` sections.

### Basic Function (no args, no returns)

```
[Function MyEffect()]
Explod{
    anim: F100;
    pos: 0, -60;
    removetime: -2;
};
PlaySnd{value: S5,0};
```

### Function with Arguments

```
[Function SpawnProjectile(xvel, yvel)]
Projectile{
    projid: 100;
    projanim: 300;
    velocity: $xvel, $yvel;
    offset: 30, -50;
};
```

Arguments are accessed with the `$` prefix inside the function body.

### Function with Return Values

```
[Function CalculateDamage(baseDmg) result]
let $result = $baseDmg * attack;
if life < lifemax / 4 {
    $result = $result * 1.5;
}
```

Return values are declared after the closing parenthesis of arguments and before the closing `]`. They are also local variables accessible via `$`.

### Calling Functions

Functions are called with the `call` keyword:

```
; Call with no return values
call MyEffect();

; Call with arguments
call SpawnProjectile(5.0, -2.0);

; Call with return value capture
let $dmg = call CalculateDamage(30);

; Multiple return values
let $a, $b = call SomeFunction(1, 2, 3);
```

### Function Scoping Rules

- Functions defined in the same file cannot be duplicated
- If a function name exists from a previous file, the first definition wins
- Functions are stored globally per character — all states can call any function
- The `ignorehitpause` attribute is automatically inherited in function bodies
- `persistent` cannot be used inside functions

---

## Variables

### Local Variables (ZSS-specific)

ZSS introduces local variables scoped to the current state or function:

```
let $counter = 0;
let $name = "test";
let $x, $y = 10, 20;              ; Multiple assignment
```

- Must start with `$`
- Declared with `let`
- Scoped to the containing state/function
- Can hold any numeric value
- Name restrictions: cannot use reserved words

### Integer Variables (shared with CNS)

```
var(0) := 5;                       ; ZSS assignment syntax for indexed vars
fvar(0) := 1.5;
sysvar(0) := 10;
sysfvar(0) := 2.5;
```

These are also accessible via controllers:

```
VarSet{var(0) = 5};
VarAdd{var(0) = 1};
```

### Map Variables

Named key-value storage:

```
MapSet{map: "health_threshold"; value: 300};

; Access in expressions
if map("health_threshold") > 0 {
    ; ...
}
```

### Assignment in Expressions

In ZSS, local variables can be assigned inline:

```
let $x = pos x;
$x = $x + 10;                     ; Reassignment
let $_, $result = call MyFunc();   ; Discard first return with _
```

The `_` identifier discards a value (useful for unused return values).

---

## Expressions & Triggers

ZSS uses the exact same expression and trigger system as CNS. All triggers, redirections, mathematical functions, and constants documented in the [CNS Scripting Reference](ikemen_cns_scripting.md#triggers-expressions) apply identically to ZSS.

### Quick Trigger Reference

#### Redirections

```
player(1), life
parent, stateno
root, pos x
helper(1000), animtime
target(-1), pos y
partner, life
enemy, stateno
enemynear, p2dist x
playerid(5), alive
p2, life
stateowner, var(0)
playerindex(0), anim
helperindex(0), pos x
```

#### Common Triggers

```
; State
stateno                    ; Current state number
statetype = S              ; State type comparison
movetype = A               ; Move type comparison
time                       ; Ticks in current state
ctrl                       ; Has control
anim                       ; Current animation
animtime                   ; Time remaining in animation

; Position/Velocity
pos x                      ; X position
vel y                      ; Y velocity
p2dist x                   ; Horizontal distance to opponent
screenpos x                ; Screen position

; Life/Power
life                       ; Current life
power                      ; Current power
alive                      ; Is alive

; Hit
movecontact                ; Move made contact
movehit                    ; Move hit
moveguarded                ; Move guarded
hitcount                   ; Hits in current state
gethitvar(damage)          ; Damage from last hit
gethitvar(fall)            ; Fall flag from last hit

; Match
roundstate                 ; Round state
roundno                    ; Round number
matchover                  ; Match is over

; Math
abs(vel x)                 ; Absolute value
floor(pos y)               ; Floor
ceil(fvar(0))              ; Ceiling
sin(angle)                 ; Sine
max(life, 0)               ; Maximum
clamp(var(0), 0, 100)      ; Clamp
lerp(0, 100, 0.5)          ; Interpolation
randomrange(0, 10)          ; Random in range

; Conditional
ifelse(alive, 1, 0)        ; Inline conditional
cond(ctrl, 200, 0)         ; Conditional with undefined handling

; Variables
var(0)                     ; Integer variable
fvar(0)                    ; Float variable
map("mykey")               ; Map variable
$localvar                  ; Local variable (ZSS only)
```

#### Constants

```
const(data.life)           ; Character max life
const(data.attack)         ; Base attack
const(size.xscale)         ; X scale
const(velocity.run.fwd.x)  ; Forward run speed

stagevar(info.name)        ; Stage name
stagevar(camera.boundleft) ; Camera bound

pi                         ; 3.14159...
e                          ; 2.71828...
```

---

## Operators

### Arithmetic

| Operator | Description |
|----------|-------------|
| `+` | Addition |
| `-` | Subtraction |
| `*` | Multiplication |
| `/` | Division |
| `%` | Modulo |
| `**` | Exponentiation |

### Comparison

| Operator | Description |
|----------|-------------|
| `=` | Equal |
| `!=` | Not equal |
| `<` | Less than |
| `<=` | Less or equal |
| `>` | Greater than |
| `>=` | Greater or equal |

### Range

```
stateno = [200, 299]       ; Inclusive range
stateno = (200, 300)       ; Exclusive range
stateno = [200, 300)       ; Half-open range
```

### Logical

| Operator | Description |
|----------|-------------|
| `&&` | Logical AND |
| `\|\|` | Logical OR |
| `!` | Logical NOT |
| `^^` | Logical XOR |

### Bitwise

| Operator | Description |
|----------|-------------|
| `&` | AND |
| `\|` | OR |
| `^` | XOR |
| `~` | NOT |

### Operator Precedence (highest to lowest)

1. `**`
2. `*`, `/`, `%`
3. `+`, `-`
4. `>`, `>=`, `<`, `<=`
5. `=`, `!=`
6. `&`
7. `^`
8. `|`
9. `&&`
10. `^^`
11. `||`

---

## Key Differences from CNS

### Syntax Comparison

| Feature | CNS | ZSS |
|---------|-----|-----|
| State definition | `[StateDef 200]` + INI properties | `[StateDef 200; Type: S; ...]` |
| Controller | `[State 200]\ntype = HitDef\ntrigger1 = ...` | `if ... { HitDef{...}; }` |
| Conditions | `triggerall`, `trigger1`, `trigger2` | `if`/`else if`/`else` |
| Line termination | Newline | Semicolon `;` |
| Comments | `;` | `#` |
| Variables | `var(0)`, `fvar(0)` | `$localvar`, `var(0)`, `fvar(0)` |
| Functions | Not supported | `[Function name(args) rets]` |
| Loops | Not supported | `for`, `while` |
| Switch | Not supported | `switch`/`case`/`default` |
| Error handling | Lenient (ignores many errors) | Strict (fails on errors) |
| Block delimiters | Section headers `[State ...]` | Curly braces `{}` |

### CNS Trigger System vs ZSS Control Flow

**CNS:**
```ini
[State -1, Super Move]
type = ChangeState
triggerall = alive
triggerall = ctrl
trigger1 = command = "QCF_x"
trigger1 = power >= 1000
trigger2 = command = "QCF_y"
trigger2 = power >= 2000
value = 1000
```

**ZSS equivalent:**
```
if alive && ctrl {
    if command = "QCF_x" && power >= 1000 || command = "QCF_y" && power >= 2000 {
        ChangeState{value: 1000};
    }
}
```

Or using structured conditions:

```
if alive && ctrl {
    if command = "QCF_x" && power >= 1000 {
        ChangeState{value: 1000};
    } else if command = "QCF_y" && power >= 2000 {
        ChangeState{value: 1000};
    }
}
```

### Controller Syntax

**CNS:**
```ini
[State 200, HitDef]
type = HitDef
trigger1 = AnimElem = 3
attr = S, NA
damage = 30, 5
getpower = 50, 25
pausetime = 8, 8
```

**ZSS:**
```
if AnimElem = 3 {
    HitDef{
        attr: S, NA;
        damage: 30, 5;
        getpower: 50, 25;
        pausetime: 8, 8;
    };
}
```

### IgnoreHitPause / Persistent

**CNS:**
```ini
[State 200, Sound]
type = PlaySnd
trigger1 = AnimElem = 1
value = S1, 0
ignorehitpause = 1
persistent = 0
```

**ZSS:**
```
ignorehitpause persistent(0) if AnimElem = 1 {
    PlaySnd{value: S1, 0};
}
```

### Variable Usage

**CNS:**
```ini
[State 200, SetVar]
type = VarSet
trigger1 = Time = 0
var(0) = 5

[State 200, Check]
type = ChangeState
trigger1 = var(0) >= 5
value = 210
```

**ZSS:**
```
if Time = 0 {
    let $counter = 5;
}
if $counter >= 5 {
    ChangeState{value: 210};
}
```

Or using indexed vars for persistence:

```
if Time = 0 {
    VarSet{var(0) = 5};
}
if var(0) >= 5 {
    ChangeState{value: 210};
}
```

---

## Complete Example

A full ZSS state file demonstrating key features:

```
# Helper function for common attack effects
[Function AttackEffect(sndGroup, sndIndex)]
PlaySnd{value: S$sndGroup, $sndIndex};
if FVar(0) > 0 {
    Explod{
        anim: F100;
        pos: 0, -60;
        postype: p1;
        removetime: -2;
    };
}

# Standing light punch
[StateDef 200; Type: S; MoveType: A; Physics: S; Anim: 200; Ctrl: 0]

if Time = 0 {
    call AttackEffect(5, 0);
}

if AnimElem = 3 {
    HitDef{
        attr: S, NA;
        hitflag: MAF;
        guardflag: MA;
        animtype: Light;
        damage: 30, 5;
        getpower: 50, 25;
        givepower: 25, 12;
        pausetime: 8, 8;
        guard.pausetime: 8, 8;
        sparkno: S0;
        hitsound: S5, 0;
        guardsound: S6, 0;
        ground.type: High;
        ground.velocity: -5.0, 0;
        air.velocity: -3.0, -4.0;
        guard.velocity: -3.0;
    };
}

if AnimTime = 0 {
    ChangeState{value: 0; ctrl: 1};
}

# Negative state: command processing
[StateDef -1]

if alive && roundstate = 2 {
    # Super move
    if ctrl && command = "QCF_a" && power >= 1000 {
        ChangeState{value: 1000};
    }

    # Special moves
    if ctrl && statetype = S {
        if command = "QCF_x" {
            ChangeState{value: 500};
        } else if command = "QCB_x" {
            ChangeState{value: 600};
        }
    }

    # Basic attacks
    if ctrl && statetype = S {
        switch 1 {
        case command = "x":
            ChangeState{value: 200};
        case command = "y":
            ChangeState{value: 210};
        case command = "z":
            ChangeState{value: 220};
        }
    }
}

# Reusable projectile function
[Function FireProjectile(speed, angle)]
let $xvel = $speed * cos(rad($angle));
let $yvel = $speed * sin(rad($angle));
Projectile{
    projid: 100;
    projanim: 300;
    velocity: $xvel, $yvel;
    offset: 30, -50;
    projremovetime: 120;
    projhits: 1;
    attr: S, SP;
    damage: 40, 10;
};
```

---

*This document was generated from the Ikemen GO source code (`compiler.go` and `compiler_functions.go`). All triggers and state controllers are shared between CNS and ZSS — only the syntax differs.*
