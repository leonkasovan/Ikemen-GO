Now let me provide a comprehensive breakdown of all the changes needed to add shader support to stage backgrounds:

## Complete File-by-File Changes to Add Shader Support to Stage Backgrounds

### **1. `src/stage.go` — backGround struct**

**Current (Line 111-151):**
```go
type backGround struct {
	// ... existing 40+ fields ...
	xshear                float32
}
```

**Changes needed:**

```diff
type backGround struct {
	// ... existing fields ...
	xshear                float32
+	shader                string          // NEW: Custom shader name
+	shaderParams          [16]float32     // NEW: Custom shader parameters (p0-p15)
}
```

**Why:** This allows each background to store its own shader reference and parameters, following the same pattern as `Explod` and `Char`.

---

### **2. `src/stage.go` — readBackGround() function (Line 177-483)**

**Add after line 481 (before `return bg, nil`):**

```diff
	if !is.ReadBool("roundpos", &bg.roundpos) {
		bg.roundpos = sProps.roundpos
	}
+	// Read shader if applicable
+	if shaderName, ok := is["shader"]; ok && len(shaderName) > 0 {
+		// Remove quotes if present
+		if len(shaderName) >= 2 && shaderName[0] == '"' && shaderName[len(shaderName)-1] == '"' {
+			shaderName = shaderName[1 : len(shaderName)-1]
+		}
+		bg.shader = strings.ToLower(shaderName)
+		
+		// Read shader parameters (shaderparam.p0 through shaderparam.p15)
+		for k, v := range is {
+			if strings.HasPrefix(strings.ToLower(k), "shaderparam.p") {
+				numStr := k[len("shaderparam.p"):]
+				if idx, err := strconv.Atoi(numStr); err == nil && idx >= 0 && idx <= 15 {
+					if val, err := strconv.ParseFloat(v, 32); err == nil {
+						bg.shaderParams[idx] = float32(val)
+					}
+				}
+			}
+		}
+	}
	return bg, nil
```

**Why:** This parses the `shader` and `shaderparam.*` keys from the INI section and stores them in the background struct.

**Example .def usage:**
```ini
[BG 0]
type = normal
spriteno = 0, 0
shader = "my_distortion"
shaderparam.p0 = 1.5
shaderparam.p1 = 0.5
```

---

### **3. `src/stage.go` — backGround.draw() function (Line 741)**

**Current (Line 741):**
```go
		bg.anim.Draw(&rect, x-xsoffset, y, sclx, scly,
			bg.xscale[0]*bgscl*(scalestartX+xs)*xs3,
			xbs*bgscl*(scalestartX+xs)*xs3,
			ys*ys3, xras*x/(Abs(ys*ys3)*lscl[1]*float32(bg.anim.spr.Size[1])*bg.scalestart[1])*sclx_recip*bg.scalestart[1]-bg.xshear,
			bg.rot, rcx, bg.palfx, 1, [2]float32{1, 1}, int32(bg.projection), bg.fLength, 0, false, CustomShaderRenderData{})
```

**Change to:**
```diff
		bg.anim.Draw(&rect, x-xsoffset, y, sclx, scly,
			bg.xscale[0]*bgscl*(scalestartX+xs)*xs3,
			xbs*bgscl*(scalestartX+xs)*xs3,
			ys*ys3, xras*x/(Abs(ys*ys3)*lscl[1]*float32(bg.anim.spr.Size[1])*bg.scalestart[1])*sclx_recip*bg.scalestart[1]-bg.xshear,
-			bg.rot, rcx, bg.palfx, 1, [2]float32{1, 1}, int32(bg.projection), bg.fLength, 0, false, CustomShaderRenderData{})
+			bg.rot, rcx, bg.palfx, 1, [2]float32{1, 1}, int32(bg.projection), bg.fLength, 0, false, bg.shader, bg.shaderParams)
```

**Why:** Passes the background's shader name and parameters to the animation draw function instead of always passing empty values.

---

### **4. `src/compiler_functions.go` — Add shader loading to stage section**

**Add a new function in the stage def parsing (around line 6984, in the `loadStage` function context):**

After BGdef parsing (around line 1330 in stage.go):

```go
// In loadStage() function, after the "BGdef" section is parsed:

	// Shaders group (NEW)
	if sec, _ := getSection("shaders"); sec != nil {
		for key, val := range sec {
			shaderPath := val
			shaderAlias := key

			// For Vulkan, append .spv extension if not present
			isVulkan := strings.HasPrefix(gfx.GetName(), "Vulkan")
			if isVulkan {
				if !strings.HasSuffix(strings.ToLower(shaderPath), ".spv") {
					shaderPath += ".spv"
				}
			}

			LoadFile(&shaderPath, []string{def, "", sys.motif.Def, "data/"}, func(filename string) error {
				f, err := OpenFile(filename)
				if err != nil {
					LogMessage("Failed to open stage shader file '%s': %v", filename, err)
					return err
				}
				defer f.Close()
				shaderData, err := io.ReadAll(f)
				if err != nil {
					LogMessage("Failed to read stage shader file '%s': %v", filename, err)
					return err
				}

				// Load on main thread
				sys.mainThreadTask <- func() {
					sys.shaderRefCount[shaderAlias] = 3
					gfx.LoadCustomSpriteShader(shaderAlias, shaderData)
				}
				return nil
			})
		}
	}
```

**Add to stage.go structure (around line 967):**
```go
type Stage struct {
	// ... existing fields ...
	customShaders []string  // NEW: Track loaded custom shaders
}
```

**Why:** Stages can define their own shaders similar to characters. This allows stages to load shader files once at initialization.

**Example .def usage:**
```ini
[Shaders]
bg_wave = wave.frag
bg_distortion = distortion.frag
```

---

### **5. Import statements in `stage.go` (if not already present)**

**Add at top of stage.go:**
```diff
+	"io"
+	"strconv"
```

These are needed for file reading and string-to-number conversion.

---

## Summary Table

| File | Change | Purpose |
|------|--------|---------|
| `src/stage.go` | Add `shader` and `shaderParams` to `backGround` struct | Store shader data per background |
| `src/stage.go` | Parse `shader:` and `shaderparam.pX:` in `readBackGround()` | Read shader config from `.def` |
| `src/stage.go` | Pass `bg.shader, bg.shaderParams` to `Draw()` | Apply shader when rendering |
| `src/stage.go` | Add `[Shaders]` section parsing in `loadStage()` | Load shader files at stage init |
| `src/stage.go` | Add `customShaders` to `Stage` struct | Track loaded shaders |
| `src/stage.go` | Add imports `"io"`, `"strconv"` | Required for new functionality |

---

## Usage Example (in stage .def file)

```ini
[Shaders]
bg_wave = effects/wave.frag
bg_distortion = effects/distortion.frag

[BG 0]
type = normal
spriteno = 0, 0
shader = "bg_wave"
shaderparam.p0 = 1.0
shaderparam.p1 = 2.0

[BG 1]
type = normal
spriteno = 0, 1
shader = "bg_distortion"
shaderparam.p0 = 0.5
```

This design maintains **consistency with the character shader implementation** and follows the same architectural patterns already proven in PR #3551.