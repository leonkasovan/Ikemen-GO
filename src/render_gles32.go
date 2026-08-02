// This is almost identical to render_gles.go except it uses a VAO
// for GLES 3.2 which is the main version that runs on modern
// Android (ARM). Work adapted from Leon Kasovan

//go:build android || armdevice

package main

import (
	"bytes"
	"container/list"
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	mgl "github.com/go-gl/mathgl/mgl32"
	gl "github.com/ikemen-engine/Ikemen-GO/packages/gl/v3.2/gles2"
	"github.com/ikemen-engine/Ikemen-GO/packages/go-sdl2/sdl"
	"golang.org/x/mobile/exp/f32"
)

//go:embed shaders/sprite_instanced.vert.glsl
var instancedVertShader string

//go:embed shaders/sprite_instanced.frag.glsl
var instancedFragShader string

// maxTexSlots is the number of unique (sprite, palette) texture pairs a single
// instanced batch may span. 7 sprite + 7 palette units + 2 reserved = 16,
// which stays within the Mali-G31 texture image unit limit on the R36S.
const maxTexSlots = 7

// instanceStrideFloats is the per-instance vertex attribute layout width:
// [0..7] corners | [8..11] x1x2x4x3 | [12..15] palUV | [16..19] uv |
// [20..23] tint | [24..27] negadd | [28..30] mult | [31..33] agrayhue |
// [34] texSlot | [35] palSlot
const instanceStrideFloats = 36

// texKey identifies a unique (sprite, palette) texture pair for slot budgeting.
type texKey struct{ tex, pal uint64 }

// texSlotInfo is one texture-slot entry. The slot index is the position in the
// fixed-size slot array (0..maxTexSlots-1), so no per-batch allocation occurs.
type texSlotInfo struct {
	key texKey
	tex Texture
	pal Texture
}

// ------------------------------------------------------------------
// ShaderProgram_GLES32

type ShaderProgram_GLES32 struct {
	program       uint32           // OpenGL handle
	attributes    map[string]int32 // Attribute name to location
	uniforms      map[string]int32 // Uniform name to location
	textures      map[string]int   // Sampler name to texture unit
	name          string           // For debugging
	needsGrabPass bool
}

var shaderCompileMutex sync.Mutex

// batchBreakLogDone gates the one-shot detailed batch-break diagnostic so it
// fires exactly once (on the first fight frame) and never again.
// Set to true initially so it only fires when explicitly re-armed in BeginFrame.
var batchBreakLogDone bool = false

func (r *Renderer_GLES32) newShaderProgram(vert, frag, geo, name string, crashWhenFail bool) (s *ShaderProgram_GLES32, err error) {
	// LOCK THE THREAD HERE
	shaderCompileMutex.Lock()
	defer shaderCompileMutex.Unlock()

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	LogDebug("[GLES] [LOCKED] Starting: %s", name)
	var vertObj, fragObj, prog uint32

	vertObj, err = r.compileShader(gl.VERTEX_SHADER, vert)
	if err != nil {
		return nil, err
	}
	LogDebug("[GLES] Vertex Obj created: %s", name)

	fragObj, err = r.compileShader(gl.FRAGMENT_SHADER, frag)
	if err != nil {
		return nil, err
	}
	LogDebug("[GLES] Frag Obj created: %s", name)

	// Activate the shader if it's there
	if len(geo) > 0 {
		if geoObj, err := r.compileShader(gl.GEOMETRY_SHADER, geo); chkEX(err, "Shader compilation error on "+name+"\n", crashWhenFail) {
			return nil, err
		} else {
			if prog, err = r.linkProgram(vertObj, fragObj, geoObj); chkEX(err, "Link program error on "+name+"\n", crashWhenFail) {
				return nil, err
			}
		}
	} else {
		LogDebug("[GLES] Entering linkProgram...")
		prog, err = r.linkProgram(vertObj, fragObj)
		if err != nil {
			return nil, err
		}
	}

	LogDebug("[GLES] Program linked, creating struct...")
	s = &ShaderProgram_GLES32{program: prog, name: name}
	s.attributes = make(map[string]int32)
	s.uniforms = make(map[string]int32)
	s.textures = make(map[string]int)

	LogDebug("[GLES] Shader initialization complete for: %s", name)
	return s, nil
}

func (r *ShaderProgram_GLES32) glStr(s string) *uint8 {
	return gl.Str(s + "\x00")
}

func (s *ShaderProgram_GLES32) RegisterAttributes(names ...string) {
	for _, name := range names {
		cstr := gl.Str(name + "\x00")
		loc := gl.GetAttribLocation(s.program, cstr)
		s.attributes[name] = loc
		LogDebug("[GLES] Attribute [%s] mapped to %d", name, loc)
	}
}

func (s *ShaderProgram_GLES32) RegisterUniforms(names ...string) {
	for _, name := range names {
		cstr := gl.Str(name + "\x00")
		loc := gl.GetUniformLocation(s.program, cstr)
		s.uniforms[name] = loc
		LogDebug("[GLES] Uniform [%s] mapped to %d", name, loc)
	}
}

func (s *ShaderProgram_GLES32) RegisterTextures(names ...string) {
	for _, name := range names {
		cstr := gl.Str(name + "\x00")
		loc := gl.GetUniformLocation(s.program, cstr)
		s.uniforms[name] = loc
		s.textures[name] = len(s.textures)
		LogDebug("[GLES] Texture [%s] mapped to %d", name, loc)
	}
}

func (r *Renderer_GLES32) compileShader(shaderType uint32, src string) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	// 1. SMART HEADER INJECTION
	// GLES 3.0+ drivers REQUIRE the version to be the very first line.
	// If your file doesn't have it, we add it here.
	fullSrc := src
	if !strings.HasPrefix(strings.TrimSpace(src), "#version") {
		// Anchor to 320 es for best feature compatibility
		header := "#version 320 es\n"
		fullSrc = header + src
	}

	// Ensure null-termination for CGO
	fullSrc = fullSrc + "\x00"

	typeName := "VERTEX"
	if shaderType == gl.FRAGMENT_SHADER {
		typeName = "FRAGMENT"
	}

	LogDebug("[GLES] Compiling %s Shader...", typeName)
	// LogDebug("[GLES] DEBUG SHADER SRC:\n%s", fullSrc) // Keep for emergencies

	// 2. MEMORY PINNING
	csource, free := gl.Strs(fullSrc)
	defer free()

	gl.ShaderSource(shader, 1, csource, nil)
	gl.CompileShader(shader)

	// 3. STATUS CHECK
	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, (*int32)(unsafe.Pointer(&status)))

	if status == 0 {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, (*int32)(unsafe.Pointer(&logLength)))

		if logLength > 0 {
			logBytes := make([]byte, logLength)
			gl.GetShaderInfoLog(shader, logLength, nil, (*uint8)(unsafe.Pointer(&logBytes[0])))
			err := fmt.Errorf("GLES %s Shader Err: %s", typeName, string(logBytes))
			LogError("[GLES] Error: %v", err)
			return 0, err
		}
		return 0, fmt.Errorf("GLES %s Shader Err: Unknown error", typeName)
	}

	LogDebug("[GLES] %s ready.", typeName)
	return shader, nil
}

func (r *Renderer_GLES32) linkProgram(params ...uint32) (program uint32, err error) {
	program = gl.CreateProgram()
	for _, param := range params {
		gl.AttachShader(program, param)
	}
	// if len(params) > 2 {
	// 	// Geometry Shader Params
	// 	gl.ProgramParameteri(program, gl.GEOMETRY_INPUT_TYPE, gl.TRIANGLES)
	// 	gl.ProgramParameteri(program, gl.GEOMETRY_OUTPUT_TYPE, gl.TRIANGLE_STRIP)
	// 	gl.ProgramParameteri(program, gl.GEOMETRY_VERTICES_OUT, 3*6)
	// }
	LogDebug("[GLES] Linking program...")
	gl.LinkProgram(program)
	// Mark shaders for deletion when the program is deleted
	for _, param := range params {
		gl.DetachShader(program, param)
		gl.DeleteShader(param)
	}

	var ok int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &ok)
	if ok == 0 {
		var size, l int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &size)
		if size > 0 {
			str := make([]byte, size+1)
			gl.GetProgramInfoLog(program, size, &l, &str[0])
			err = fmt.Errorf("Link error: %s", string(str[:l]))
		} else {
			err = fmt.Errorf("Unknown link error")
		}
		LogError("[GLES] Error: %v", err)
		gl.DeleteProgram(program)
		return 0, err
	}

	LogDebug("[GLES] Link Successful!")
	return program, nil
}

// ------------------------------------------------------------------
// Texture_GLES32

type Texture_GLES32 struct {
	width     int32
	height    int32
	depth     int32
	filter    bool
	handle    uint32 // GL side handle
	serial    uint64 // Go side serial number
	offsetX   int32  // Palette atlas: X offset within atlas (pixels). 0 for normal textures.
	offsetY   int32  // Palette atlas: Y offset within atlas (pixels). 0 for normal textures.
	palSlot   bool   // True if this is a sub-region of the palette atlas.
	atlasSize int32  // Size of the palette atlas when this slot was allocated. 0 for normal textures.
}

// Helper that wraps the actual GL call to generate a texture
func (r *Renderer_GLES32) generateTexture(width, height, depth int32, filter bool) *Texture_GLES32 {
	var h uint32
	gl.GenTextures(1, &h)

	// Ensure a unique ID even if GL reuses the handle
	textureSerialNumber++

	tex := &Texture_GLES32{
		width:  width,
		height: height,
		depth:  depth,
		filter: filter,
		handle: h,
		serial: textureSerialNumber,
	}

	runtime.SetFinalizer(tex, func(t *Texture_GLES32) {
		if t.handle == 0 {
			return // already released via Release()
		}
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})

	return tex
}

// Release immediately frees the GPU resources held by this texture.
// Safe to call multiple times — subsequent calls are no-ops.
// After Release(), the texture's handle is invalid and the finalizer
// will skip GL cleanup to prevent double-free.
func (t *Texture_GLES32) Release() {
	if t.handle == 0 {
		return
	}
	if !t.palSlot {
		// Regular texture (non-atlas): delete the GL handle.
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	}
	// Palette atlas slots share the atlas GL handle — only the finalizer
	// returns the slot to the free list. We just zero the handle here so
	// the finalizer guard skips GL deletion.
	t.handle = 0
}

// Creates a generic texture
func (r *Renderer_GLES32) newTexture(width, height, depth int32, filter bool) Texture {
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	t := r.generateTexture(width, height, depth, filter)

	return t
}

func (r *Renderer_GLES32) createPalAtlas() {
	r.palAtlasSize = PalAtlasSize
	r.palFreeSlots = list.New()

	// Create a single PalAtlasSize x PalAtlasSize RGBA texture
	r.palAtlas = r.newTexture(PalAtlasSize, PalAtlasSize, 32, false).(*Texture_GLES32)

	// Initialize the atlas to transparent
	clearData := make([]byte, PalAtlasSize*PalAtlasSize*4)
	r.palAtlas.SetData(clearData)

	// Fill the free slot queue (slotsPerRow * PalAtlasSize slots)
	for i := int32(0); i < PalAtlasSize*(PalAtlasSize/256); i++ {
		r.palFreeSlots.PushBack(int32(i))
	}

	LogMessage("[PalAtlas] Created %dx%d palette atlas with %d slots",
		PalAtlasSize, PalAtlasSize, PalAtlasSize*(PalAtlasSize/256))

	// Record total capacity for the slot-usage warning in the HEAP log.
	memPalSlotSetTotal(int64(PalAtlasSize * (PalAtlasSize / 256)))
}

func (r *Renderer_GLES32) autoResizeAtlas() {
	newSize := r.palAtlasSize * 2
	if newSize > 4096 {
		LogMessage("[PalAtlas] Already at maximum size %d — cannot resize further.", r.palAtlasSize)
		return
	}

	// Keep old atlas alive — existing palette textures still reference it via handle.
	r.oldPalAtlases = append(r.oldPalAtlases, r.palAtlas)

	oldSlotCount := r.palAtlasSize * (r.palAtlasSize / 256)

	// Update config for persistence (takes effect on next launch).
	sys.cfg.SetValueUpdate("Video.PaletteAtlasSize", newSize)
	sys.cfgCacheGen++
	sys.cachedCfgTable = nil
	PalAtlasSize = newSize

	// Create new, larger atlas.
	r.palAtlasSize = newSize
	r.palAtlas = r.newTexture(newSize, newSize, 32, false).(*Texture_GLES32)
	clearData := make([]byte, newSize*newSize*4)
	r.palAtlas.SetData(clearData)

	// Fill free slot queue with ONLY the new slots (old slots are in-use).
	for i := oldSlotCount; i < newSize*(newSize/256); i++ {
		r.palFreeSlots.PushBack(i)
	}

	memPalSlotSetTotal(int64(newSize * (newSize / 256)))

	LogMessage("[PalAtlas] Auto-resized from %dx%d to %dx%d — %d additional slots available (config saved)",
		newSize/2, newSize/2, newSize, newSize, newSize*(newSize/256)-oldSlotCount)
}

func (r *Renderer_GLES32) newPaletteTexture() Texture {
	if r.palFreeSlots == nil || r.palFreeSlots.Len() == 0 {
		// Try auto-resizing the atlas before falling back to standalone textures.
		r.autoResizeAtlas()
	}

	if r.palFreeSlots == nil || r.palFreeSlots.Len() == 0 {
		fmt.Printf("[PalAtlas] Out of palette slots! Creating fallback standalone texture.\n")
		return r.newTexture(256, 1, 32, false)
	}

	slot := r.palFreeSlots.Remove(r.palFreeSlots.Front()).(int32)
	memPalSlotAlloc()

	// Calculate slot position within the atlas.
	// Layout: slots arranged in rows of (PalAtlasSize / 256) per row.
	// Slot 0: (0, 0), Slot 1: (256, 0), ..., Slot 7: (1792, 0)
	// Slot 8: (0, 1), ..., etc.
	slotsPerRow := r.palAtlasSize / 256
	offsetY := slot / slotsPerRow
	offsetX := (slot % slotsPerRow) * 256

	t := &Texture_GLES32{
		width:     256,
		height:    1,
		depth:     32,
		filter:    false,
		handle:    r.palAtlas.handle,
		serial:    r.palAtlas.serial,
		offsetX:   offsetX,
		offsetY:   offsetY,
		palSlot:   true,
		atlasSize: r.palAtlasSize,
	}

	// When the texture is garbage collected (or explicitly released), return the
	// slot to the free list and decrement the usage counter.
	// The handle guard prevents double-return when Release() is called explicitly.
	sid := slot // capture by value for the closure
	runtime.SetFinalizer(t, func(t *Texture_GLES32) {
		if t.handle == 0 {
			return // already released or slot already returned
		}
		sys.mainThreadTask <- func() {
			memPalSlotFree()
			if r.palFreeSlots != nil {
				r.palFreeSlots.PushFront(sid)
			}
		}
	})

	return t
}

func (r *Renderer_GLES32) newModelTexture(width, height, depth int32, filter bool) Texture {
	return r.newTexture(width, height, depth, filter)
}

func (r *Renderer_GLES32) newDataTexture(width, height int32) Texture {
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	t := r.generateTexture(width, height, 32, false)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	return t
}

func (r *Renderer_GLES32) newHDRTexture(width, height int32) Texture {
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	t := r.generateTexture(width, height, 24, false)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.MIRRORED_REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.MIRRORED_REPEAT)
	return t
}

func (r *Renderer_GLES32) newCubeMapTexture(widthHeight int32, mipmap bool, lowestMipLevel int32) Texture {
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	t := r.generateTexture(widthHeight, widthHeight, 24, false)

	gl.BindTexture(gl.TEXTURE_CUBE_MAP, t.handle)
	for i := 0; i < 6; i++ {
		// https://stackoverflow.com/a/72244518 RGBA16F seems to work :), HALF_FLOAT is the type that gets us good results
		gl.TexImage2D(uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+i), 0, gl.RGBA16F, widthHeight, widthHeight, 0, gl.RGBA, gl.HALF_FLOAT, nil)
	}

	if mipmap {
		gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
		gl.GenerateMipmap(gl.TEXTURE_CUBE_MAP)
	} else {
		gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	}

	gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	return t
}

// Bind a texture and upload texel data to it
func (t *Texture_GLES32) SetData(data []byte) {
	bits := Max(t.depth, 8)
	internalFormat := t.MapSizedInternalFormat(bits)
	uploadFormat := t.MapUploadFormat(bits)
	uploadType := t.MapUploadType(bits)

	r := gfx.(*Renderer_GLES32)
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)

	if t.palSlot {
		// Palette atlas slot: write to sub-region of the shared atlas texture.
		// Only upload actual payload; nil means "do nothing" (atlas already allocated).
		if data != nil {
			gl.TexSubImage2D(gl.TEXTURE_2D, 0, t.offsetX, t.offsetY, t.width, t.height, uint32(uploadFormat), uploadType, unsafe.Pointer(&data[0]))
		}
	} else {
		// Normal texture: allocate storage.
		var interp int32 = gl.NEAREST
		if t.filter {
			interp = gl.LINEAR
		}

		if data != nil {
			gl.TexImage2D(gl.TEXTURE_2D, 0, int32(internalFormat), t.width, t.height, 0, uploadFormat, uploadType, unsafe.Pointer(&data[0]))
		} else {
			gl.TexImage2D(gl.TEXTURE_2D, 0, int32(internalFormat), t.width, t.height, 0, uploadFormat, uploadType, nil)
		}

		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, interp)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, interp)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	}
}

func (t *Texture_GLES32) SetSubData(data []byte, x, y, width, height, stride int32) {
	var interp int32 = gl.NEAREST
	if t.filter {
		interp = gl.LINEAR
	}

	r := gfx.(*Renderer_GLES32)
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)

	bits := Max(t.depth, 8)
	uploadFormat := t.MapUploadFormat(bits)
	uploadType := t.MapUploadType(bits)
	bytesPerPixel := t.depth / 8
	if bytesPerPixel < 1 {
		bytesPerPixel = 1
	}

	var rowLength int32 = 0
	if stride != width*bytesPerPixel {
		rowLength = stride / bytesPerPixel
	}

	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, rowLength)

	ptr := unsafe.Pointer(nil)
	if data != nil {
		ptr = unsafe.Pointer(&data[0])
	}

	if data != nil {
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, x, y, width, height, uint32(uploadFormat), uploadType, ptr)
	} else {
		gl.TexSubImage2D(gl.TEXTURE_2D, 0, x, y, width, height, uint32(uploadFormat), uploadType, nil)
	}

	if err := gl.GetError(); err != 0 {
		LogError("GL ERROR in SetSubData: %v | w:%d h:%d s:%d", err, width, height, stride)
	}

	if rowLength != 0 {
		gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	}

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, interp)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, interp)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
}

func (t *Texture_GLES32) SetDataG(data []byte, mag, min, ws, wt TextureSamplingParam) {
	bits := Max(t.depth, 8)
	internalFormat := t.MapSizedInternalFormat(bits)
	uploadFormat := t.MapUploadFormat(bits)
	uploadType := t.MapUploadType(bits)

	r := gfx.(*Renderer_GLES32)
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	gl.TexImage2D(gl.TEXTURE_2D, 0, int32(internalFormat), t.width, t.height, 0, uploadFormat, uploadType, unsafe.Pointer(&data[0]))
	gl.GenerateMipmap(gl.TEXTURE_2D)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, int32(mag))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, int32(min))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, int32(ws))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, int32(wt))
}

func (t *Texture_GLES32) SetPixelData(data []float32) {
	r := gfx.(*Renderer_GLES32)
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, t.width, t.height, 0, gl.RGBA, gl.FLOAT, unsafe.Pointer(&data[0]))
}

func (t Texture_GLES32) CopyData(src *Texture) {
	r := gfx.(*Renderer_GLES32)
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	gl.BindTexture(gl.TEXTURE_2D, 0) // Unbind whatever is currently bound
	srcES := (*src).(*Texture_GLES32)
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	r.bindFramebuffer(gl.READ_FRAMEBUFFER, fbo)
	gl.FramebufferTexture2D(gl.READ_FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, srcES.handle, 0)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	// Copy the old texture data into the top-left of the new, larger texture
	gl.CopyTexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, 0, 0, srcES.width, srcES.height)

	r.bindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	gl.DeleteFramebuffers(1, &fbo)
}

/*
// Not called anywhere
func (t *Texture_GLES32) SetRGBPixelData(data []float32) {
	r := gfx.(*Renderer_GLES32)
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.PixelStorei(gl.UNPACK_ROW_LENGTH, 0)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGB32F, t.width, t.height, 0, gl.RGB, gl.FLOAT, unsafe.Pointer(&data[0]))
}
*/

// Return whether texture has a valid handle
func (t *Texture_GLES32) IsValid() bool {
	return t.width != 0 && t.height != 0 && t.handle != 0
}

func (t *Texture_GLES32) GetWidth() int32 {
	return t.width
}

func (t *Texture_GLES32) GetHeight() int32 {
	return t.height
}

func (t *Texture_GLES32) GetPalUV() [4]float32 {
	if t.palSlot {
		// Palette atlas slot: compute UV relative to the atlas texture size at
		// allocation time — NOT the current global PalAtlasSize, which may have
		// changed due to a later auto-resize. Using the stale global would produce
		// wrong UVs when old palettes reference the old (smaller) atlas texture.
		atlasSize := float32(t.atlasSize)
		u1 := (float32(t.offsetX) + 0.5) / atlasSize
		v1 := (float32(t.offsetY) + 0.5) / atlasSize
		uSize := float32(256) / atlasSize
		vSize := float32(1) / atlasSize
		return [4]float32{u1, v1, uSize, vSize}
	}
	// Standalone 256x1 texture: sample the full width at middle row
	return [4]float32{0, 0.5, 1, 0}
}

func (t *Texture_GLES32) GetSerial() uint64 {
	return t.serial
}

func (t *Texture_GLES32) MapUploadType(i int32) uint32 {
	switch i {
	case 96, 128:
		return gl.FLOAT
	default:
		return gl.UNSIGNED_BYTE
	}
}

func (t *Texture_GLES32) MapUploadFormat(i int32) uint32 {
	switch i {
	case 8:
		return gl.RED
	case 24:
		return gl.RGB
	case 32:
		return gl.RGBA
	case 96:
		return gl.RGB
	case 128:
		return gl.RGBA
	default:
		return gl.RGBA
	}
}

func (t *Texture_GLES32) MapSizedInternalFormat(i int32) uint32 {
	switch i {
	case 8:
		return gl.R8
	case 24:
		return gl.RGB8
	case 32:
		return gl.RGBA8
	case 96:
		return gl.RGB32F
	case 128:
		return gl.RGBA32F
	default:
		return gl.RGBA8
	}
}

// ------------------------------------------------------------------
// Renderer_GLES32

type Renderer_GLES32 struct {
	fbo         uint32
	fbo_texture uint32
	// Normal rendering
	rbo_depth uint32
	// MSAA rendering
	fbo_f         uint32
	fbo_f_texture *Texture_GLES32
	// Shadow Map
	fbo_shadow              uint32
	fbo_shadow_cube_texture uint32
	fbo_env                 uint32
	// Post-processing FBOs
	fbo_pp         []uint32
	fbo_pp_texture []uint32
	// Post-processing shaders
	postVertBuffer   uint32
	postShaderSelect []*ShaderProgram_GLES32
	// Custom shaders
	customShaders   map[uint32]*ShaderProgram_GLES32
	customShaderMap map[string]uint32
	nextShaderID    uint32
	currentProgram  *ShaderProgram_GLES32
	grabTexture     *Texture_GLES32
	// Shader and vertex data for primitive rendering
	spriteShader *ShaderProgram_GLES32
	vertexBuffer uint32
	// vertexScratch is a reusable byte buffer for SetVertexData, avoiding a
	// per-quad []byte allocation via f32.Bytes on the render hot path.
	// gl.BufferData copies immediately, so the buffer can be safely reused.
	vertexScratch []byte
	// Instanced sprite rendering (Phase 4/5 batching)
	instancedSpriteShader *ShaderProgram_GLES32
	instanceVBO           uint32    // per-instance attribute VBO
	instanceVAO           uint32    // VAO for the instanced sprite pass
	instanceQuadVBO       uint32    // static corner-index VBO (2,3,1,4)
	instanceScratch       []float32 // reusable CPU-side instance buffer
	// boundTexUnits tracks which texture handle is bound to each texture unit
	// during the current instanced flush, so repeated binds across batches
	// (very common: consecutive batches reuse the same sprite/palette) are skipped.
	boundTexUnits [2 * maxTexSlots]uint32
	// Shader and index data for 3D model rendering
	shadowMapShader         *ShaderProgram_GLES32
	modelShader             *ShaderProgram_GLES32
	panoramaToCubeMapShader *ShaderProgram_GLES32
	cubemapFilteringShader  *ShaderProgram_GLES32
	modelVertexBuffer       [2]uint32
	modelIndexBuffer        [2]uint32
	spriteVAO               uint32
	modelVAO                uint32
	modelEnvVAO             uint32
	postVAO                 uint32

	enableModel  bool
	enableShadow bool
	debugMode    bool

	// RenderScale: internal render dimensions (may be smaller than window when RenderScale < 1)
	renderW int32
	renderH int32

	// useIntermediateFBO: when false, render directly to the default framebuffer (no FBO).
	// True when post-processing shaders are active or MSAA > 0.
	useIntermediateFBO bool

	// Palette atlas
	palAtlas      *Texture_GLES32   // Shared atlas texture for all palettes
	palAtlasSize  int32             // Size of atlas (e.g., 2048)
	palFreeSlots  *list.List        // Queue of free slot indices
	oldPalAtlases []*Texture_GLES32 // Kept alive so existing palette textures don't dangle
	GLES32State
}
type GLES32State struct {
	program             uint32
	depthTest           bool
	depthMask           bool
	invertFrontFace     bool
	doubleSided         bool
	blendEnabled        bool
	blendEquation       BlendEquation
	blendSrc            BlendFunc
	blendDst            BlendFunc
	scissorRect         [4]int32
	scissorEnabled      bool
	texCacheTexSerial   []uint64 // Unit to serial number. Sized per GPU
	texCacheLastUsed    []uint64 // Timer value when the slot was last used. Sized per GPU
	texCacheTimer       uint64   // Increments on every texture access
	uniformICache       map[uint32]int32
	uniformF1Cache      map[uint32]float32
	uniformF2Cache      map[uint32][2]float32
	uniformF3Cache      map[uint32][3]float32
	uniformF4Cache      map[uint32][4]float32
	useUV               bool
	useNormal           bool
	useTangent          bool
	useVertColor        bool
	useJoint0           bool
	useJoint1           bool
	useOutlineAttribute bool

	// FBO binding cache — avoids redundant gl.BindFramebuffer calls (hot path perf).
	curDrawFbo uint32
	curReadFbo uint32
}

func (r *Renderer_GLES32) GetName() string {
	return "OpenGL ES 3.2"
}

func (r *Renderer_GLES32) DebugInfo() string {
	return "" // No OOM tracking — GLES driver manages memory
}

// init 3D model shader
func (r *Renderer_GLES32) InitModelShader() error {
	var err error
	if r.enableShadow {
		r.modelShader, err = r.newShaderProgram(modelVertShader, "#define ENABLE_SHADOW\n"+modelFragShader, "", "Model Shader", false)
	} else {
		r.modelShader, err = r.newShaderProgram(modelVertShader, modelFragShader, "", "Model Shader", false)
	}
	if err != nil {
		return err
	}

	r.modelShader.RegisterAttributes("inVertexId", "position", "uv", "normalIn", "tangentIn", "vertColor", "joints_0", "joints_1", "weights_0", "weights_1", "outlineAttributeIn")

	r.modelShader.RegisterUniforms("model", "view", "projection", "normalMatrix", "unlit", "baseColorFactor", "add", "mult", "useTexture", "useNormalMap", "useMetallicRoughnessMap", "useEmissionMap", "neg", "gray", "hue",
		"enableAlpha", "alphaThreshold", "numJoints", "morphTargetWeight", "morphTargetOffset", "morphTargetTextureDimension", "numTargets", "numVertices",
		"metallicRoughness", "ambientOcclusionStrength", "emission", "environmentIntensity", "mipCount", "meshOutline",
		"cameraPosition", "environmentRotation", "texTransform", "normalMapTransform", "metallicRoughnessMapTransform", "ambientOcclusionMapTransform", "emissionMapTransform",
		"lightMatrices[0]", "lightMatrices[1]", "lightMatrices[2]", "lightMatrices[3]",
		"lights[0].direction", "lights[0].range", "lights[0].color", "lights[0].intensity", "lights[0].position", "lights[0].innerConeCos", "lights[0].outerConeCos", "lights[0].type", "lights[0].shadowBias", "lights[0].shadowMapFar",
		"lights[1].direction", "lights[1].range", "lights[1].color", "lights[1].intensity", "lights[1].position", "lights[1].innerConeCos", "lights[1].outerConeCos", "lights[1].type", "lights[1].shadowBias", "lights[1].shadowMapFar",
		"lights[2].direction", "lights[2].range", "lights[2].color", "lights[2].intensity", "lights[2].position", "lights[2].innerConeCos", "lights[2].outerConeCos", "lights[2].type", "lights[2].shadowBias", "lights[2].shadowMapFar",
		"lights[3].direction", "lights[3].range", "lights[3].color", "lights[3].intensity", "lights[3].position", "lights[3].innerConeCos", "lights[3].outerConeCos", "lights[3].type", "lights[3].shadowBias", "lights[3].shadowMapFar",
	)
	r.modelShader.RegisterTextures(
		"tex", "morphTargetValues", "jointMatrices",
		"normalMap", "metallicRoughnessMap", "ambientOcclusionMap", "emissionMap",
		"lambertianEnvSampler", "GGXEnvSampler", "GGXLUT",
		"shadowCubeMap",
	)

	if r.enableShadow {
		r.shadowMapShader, err = r.newShaderProgram(shadowVertShader, shadowFragShader, shadowGeoShader, "Shadow Map Shader", false)
		if err != nil {
			return err
		}

		r.shadowMapShader.RegisterAttributes("vertexId", "position", "vertColor", "uv", "joints_0", "joints_1", "weights_0", "weights_1")

		r.shadowMapShader.RegisterUniforms("model", "lightMatrix",
			"lights[0].type", "lights[1].type", "lights[2].type", "lights[3].type", "lights[0].position", "lights[1].position", "lights[2].position", "lights[3].position",
			"lights[0].shadowMapFar", "lights[1].shadowMapFar", "lights[2].shadowMapFar", "lights[3].shadowMapFar", "numJoints", "morphTargetWeight", "morphTargetOffset", "morphTargetTextureDimension",
			"numTargets", "numVertices", "enableAlpha", "alphaThreshold", "baseColorFactor", "useTexture", "texTransform", "layerOffset", "lightIndex")
		r.shadowMapShader.RegisterTextures("morphTargetValues", "jointMatrices", "tex")
	}

	r.panoramaToCubeMapShader, err = r.newShaderProgram(identVertShader, panoramaToCubeMapFragShader, "", "Panorama To Cubemap Shader", false)
	if err != nil {
		return err
	}
	r.panoramaToCubeMapShader.RegisterAttributes("VertCoord")
	r.panoramaToCubeMapShader.RegisterUniforms("currentFace")
	r.panoramaToCubeMapShader.RegisterTextures("panorama")

	r.cubemapFilteringShader, err = r.newShaderProgram(identVertShader, cubemapFilteringFragShader, "", "Cubemap Filtering Shader", false)
	if err != nil {
		return err
	}
	r.cubemapFilteringShader.RegisterAttributes("VertCoord")
	r.cubemapFilteringShader.RegisterUniforms("sampleCount", "distribution", "width", "currentFace", "roughness", "intensityScale", "isLUT")
	r.cubemapFilteringShader.RegisterTextures("cubeMap")

	// Configure modelEnvVAO
	gl.BindVertexArray(r.modelEnvVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)

	if loc, ok := r.cubemapFilteringShader.attributes["VertCoord"]; ok && loc >= 0 {
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointer(uint32(loc), 2, gl.FLOAT, false, 0, nil)
	}

	// Unbind for safety
	gl.BindVertexArray(0)

	return nil
}

// Render initialization.
// Creates the default shaders, the framebuffer and enables MSAA.
func (r *Renderer_GLES32) Init() {
	chk(gl.Init(func(name string) unsafe.Pointer {
		return eglGetProcAddress(name)
	}))
	if runtime.GOOS != "android" {
		LogMessage("Using OpenGL %v (%v)", gl.GoStr(gl.GetString(gl.VERSION)), gl.GoStr(gl.GetString(gl.RENDERER)))
	} else {
		LogMessage("[GLES] Using OpenGL %v (%v)", gl.GoStr(gl.GetString(gl.VERSION)), gl.GoStr(gl.GetString(gl.RENDERER)))
	}

	// LogDebug("[GLES] Querying Max Samples")
	// var maxSamples int32
	// gl.GetIntegerv(gl.MAX_SAMPLES, &maxSamples)
	// if sys.msaa > maxSamples {
	// 	sys.cfg.SetValueUpdate("Video.MSAA", maxSamples)
	// 	sys.msaa = maxSamples
	// }
	sys.msaa = 0
	LogDebug("[GLES] Past MSAA check")

	r.customShaders = make(map[uint32]*ShaderProgram_GLES32)
	r.customShaderMap = make(map[string]uint32)
	r.nextShaderID = 1
	r.currentProgram = nil
	var err error

	// Data buffers for rendering
	postVertData := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)

	r.enableModel = sys.cfg.Video.EnableModel
	r.enableShadow = sys.cfg.Video.EnableModelShadow

	if sys.cfg.Video.RendererDebugMode {
		r.EnableDebug()
	}

	// Generate VAO's
	gl.GenVertexArrays(1, &r.spriteVAO)
	gl.GenVertexArrays(1, &r.modelVAO)
	gl.GenVertexArrays(1, &r.modelEnvVAO)
	gl.GenVertexArrays(1, &r.postVAO)
	LogDebug("[GLES] Sprite, model and post VAO's generated")

	//LogDebug("[GLES] VAO Bound")

	// Generate buffers
	gl.GenBuffers(1, &r.vertexBuffer)
	LogDebug("[GLES] VertexBuffer Generated")

	gl.GenBuffers(1, &r.modelVertexBuffer[0])
	gl.GenBuffers(1, &r.modelVertexBuffer[1])
	LogDebug("[GLES] ModelVertexBuffers Generated")

	gl.GenBuffers(1, &r.modelIndexBuffer[0])
	gl.GenBuffers(1, &r.modelIndexBuffer[1])
	LogDebug("[GLES] ModelIndexBuffers Generated")

	gl.GenBuffers(1, &r.postVertBuffer)
	LogDebug("[GLES] PostVertBuffer Generated")

	// Initialize post-processing vertex buffer
	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)
	LogDebug("[GLES] Data Size: %d", len(postVertData))

	if len(postVertData) > 0 {
		gl.BufferData(gl.ARRAY_BUFFER, len(postVertData), unsafe.Pointer(&postVertData[0]), gl.STATIC_DRAW)
		LogDebug("[GLES] PostVertBuffer Data Uploaded")
	} else {
		LogError("[GLES] Error: postVertData is empty!")
	}

	// Unbind for safety
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)

	// Sprite shader
	r.spriteShader, _ = r.newShaderProgram(vertShader, fragShader, "", "Main Shader", true)
	r.spriteShader.RegisterAttributes("position", "uv")
	r.spriteShader.RegisterUniforms("modelview", "projection", "x1x2x4x3",
		"alpha", "tint", "mask", "neg", "gray", "add", "mult", "isFlat", "isRgba", "isTrapez", "hue", "palUV")
	r.spriteShader.RegisterTextures("pal", "tex")

	// Configure spriteVAO
	gl.BindVertexArray(r.spriteVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)

	locPos := r.spriteShader.attributes["position"]
	gl.EnableVertexAttribArray(uint32(locPos))
	gl.VertexAttribPointerWithOffset(uint32(locPos), 2, gl.FLOAT, false, 16, 0)

	locUV := r.spriteShader.attributes["uv"]
	gl.EnableVertexAttribArray(uint32(locUV))
	gl.VertexAttribPointerWithOffset(uint32(locUV), 2, gl.FLOAT, false, 16, 8)

	// Unbind for safety
	gl.BindVertexArray(0)

	// Instanced sprite shader (Phase 4/5 batching). Best-effort: if the driver
	// rejects it, batching silently falls back to the immediate path.
	if r.instancedSpriteShader, err = r.newShaderProgram(instancedVertShader, instancedFragShader, "", "Instanced Sprite Shader", false); err != nil {
		LogError("[GLES] Instanced sprite shader compile failed — sprite batching disabled: %v", err)
		r.instancedSpriteShader = nil
		// Route all sprite draws back through the immediate path.
		sys.cfg.Video.EnableSpriteBatching = false
	} else {
		// Attribute locations are explicit via layout(location=..) in the shader.
		r.instancedSpriteShader.RegisterUniforms("projection", "mask", "isRgba", "isTrapez", "isFlat")
		r.instancedSpriteShader.RegisterUniforms("texArray[0]", "palArray[0]")
		if r.instancedSpriteShader.uniforms["texArray[0]"] < 0 {
			// Some drivers reject the "[0]" form but accept the bare array name.
			r.instancedSpriteShader.RegisterUniforms("texArray", "palArray")
		}

		gl.GenBuffers(1, &r.instanceVBO)
		gl.GenBuffers(1, &r.instanceQuadVBO)
		gl.GenVertexArrays(1, &r.instanceVAO)
		gl.BindVertexArray(r.instanceVAO)

		// Static per-vertex attribute 0: corner index, triangle-strip order (2,3,1,4).
		gl.BindBuffer(gl.ARRAY_BUFFER, r.instanceQuadVBO)
		quadData := f32.Bytes(binary.LittleEndian, 2, 3, 1, 4)
		gl.BufferData(gl.ARRAY_BUFFER, len(quadData), unsafe.Pointer(&quadData[0]), gl.STATIC_DRAW)
		gl.EnableVertexAttribArray(0)
		gl.VertexAttribPointerWithOffset(0, 1, gl.FLOAT, false, 4, 0)

		// Per-instance attributes (divisor 1).
		gl.BindBuffer(gl.ARRAY_BUFFER, r.instanceVBO)
		stride := int32(instanceStrideFloats * 4)

		gl.EnableVertexAttribArray(2)
		gl.VertexAttribPointerWithOffset(2, 4, gl.FLOAT, false, stride, 0)
		gl.VertexAttribDivisor(2, 1)
		gl.EnableVertexAttribArray(3)
		gl.VertexAttribPointerWithOffset(3, 4, gl.FLOAT, false, stride, 16)
		gl.VertexAttribDivisor(3, 1)
		gl.EnableVertexAttribArray(4)
		gl.VertexAttribPointerWithOffset(4, 4, gl.FLOAT, false, stride, 32)
		gl.VertexAttribDivisor(4, 1)
		gl.EnableVertexAttribArray(5)
		gl.VertexAttribPointerWithOffset(5, 4, gl.FLOAT, false, stride, 48)
		gl.VertexAttribDivisor(5, 1)
		gl.EnableVertexAttribArray(6)
		gl.VertexAttribPointerWithOffset(6, 4, gl.FLOAT, false, stride, 64)
		gl.VertexAttribDivisor(6, 1)
		gl.EnableVertexAttribArray(7)
		gl.VertexAttribPointerWithOffset(7, 4, gl.FLOAT, false, stride, 80)
		gl.VertexAttribDivisor(7, 1)
		gl.EnableVertexAttribArray(8)
		gl.VertexAttribPointerWithOffset(8, 4, gl.FLOAT, false, stride, 96)
		gl.VertexAttribDivisor(8, 1)
		gl.EnableVertexAttribArray(9)
		gl.VertexAttribPointerWithOffset(9, 3, gl.FLOAT, false, stride, 112)
		gl.VertexAttribDivisor(9, 1)
		gl.EnableVertexAttribArray(10)
		gl.VertexAttribPointerWithOffset(10, 3, gl.FLOAT, false, stride, 124)
		gl.VertexAttribDivisor(10, 1)
		gl.EnableVertexAttribArray(11)
		gl.VertexAttribPointerWithOffset(11, 1, gl.FLOAT, false, stride, 136)
		gl.VertexAttribDivisor(11, 1)
		gl.EnableVertexAttribArray(12)
		gl.VertexAttribPointerWithOffset(12, 1, gl.FLOAT, false, stride, 140)
		gl.VertexAttribDivisor(12, 1)

		gl.BindVertexArray(0)
		gl.BindBuffer(gl.ARRAY_BUFFER, 0)

		r.instanceScratch = make([]float32, 0, instanceStrideFloats*512)
		if r.debugMode {
			LogMessage("[GLES] Instanced sprite batching initialized (%d floats/instance)", instanceStrideFloats)
		}
	}

	if r.enableModel {
		if err := r.InitModelShader(); err != nil {
			r.enableModel = false
		}
	}

	// Compile post-processing shaders
	// Because we only have one VAO, the attributes will only be set in EndFrame()

	// Pre-allocate the shader slice to accommodate all external shaders plus the identity shader
	r.postShaderSelect = make([]*ShaderProgram_GLES32, len(sys.cfg.Video.ExternalShaders)+1)

	// Configure postVAO
	gl.BindVertexArray(r.postVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)

	// External Shaders
	for i := 0; i < len(sys.cfg.Video.ExternalShaders); i++ {
		r.postShaderSelect[i], _ = r.newShaderProgram(string(sys.externalShaders[0][i])+"\x00", string(sys.externalShaders[1][i])+"\x00",
			"", fmt.Sprintf("Postprocess Shader #%v", i), true)
		r.postShaderSelect[i].RegisterAttributes("VertCoord") // "TexCoord" was registered but never used
		r.postShaderSelect[i].RegisterUniforms("Texture_GLES32", "TextureSize", "CurrentTime")

		// Configure postVAO for this specific shader's attribute location
		if loc, ok := r.postShaderSelect[i].attributes["VertCoord"]; ok && loc >= 0 {
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointer(uint32(loc), 2, gl.FLOAT, false, 0, nil)
		}
	}

	// Identity shader (no post-processing). This should be the last one in modern OpenGL
	identShader, _ := r.newShaderProgram(identVertShader, identFragShader, "", "Identity Postprocess", true)
	identShader.RegisterAttributes("VertCoord")
	//identShader.RegisterUniforms("Texture_GLES32", "TextureSize", "CurrentTime") // None of these are used

	// Configure postVAO for the identity shader's attribute location
	if loc, ok := identShader.attributes["VertCoord"]; ok && loc >= 0 {
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointer(uint32(loc), 2, gl.FLOAT, false, 0, nil)
	}

	// It should be the last one in modern OpenGL
	r.postShaderSelect[len(r.postShaderSelect)-1] = identShader

	// Compute internal render resolution (RenderScale support).
	// The scene is rendered into an FBO of this size, then upscaled to the full
	// window size in EndFrame(). A scale of 1.0 means no downscaling.
	{
		scale := sys.cfg.Video.RenderScale
		if scale <= 0 || scale > 1.0 {
			scale = 1.0
		}
		r.renderW = (int32(float32(sys.scrrect[2])*scale) / 2) * 2
		r.renderH = (int32(float32(sys.scrrect[3])*scale) / 2) * 2
		if r.renderW < 2 {
			r.renderW = 2
		}
		if r.renderH < 2 {
			r.renderH = 2
		}
		if scale < 1.0 {
			LogMessage("[GLES] RenderScale=%.2f → internal render size %dx%d (window %dx%d)",
				scale, r.renderW, r.renderH, sys.scrrect[2], sys.scrrect[3])
		}
	}

	// Determine whether we need an intermediate FBO.
	// If no post-processing shaders, no MSAA and no RenderScale downscaling, we
	// can render directly to the default framebuffer (FBO 0), eliminating the
	// main render-target FBO switch entirely. A scaled render MUST go through the
	// FBO so EndFrame can upscale the smaller texture to the window size.
	r.useIntermediateFBO = r.renderW != int32(sys.scrrect[2]) || r.renderH != int32(sys.scrrect[3]) ||
		len(sys.cfg.Video.ExternalShaders) > 0 || sys.msaa > 0
	if !r.useIntermediateFBO {
		LogMessage("[GLES] No post shaders, no MSAA, no scaling — skipping intermediate FBO (direct render to screen)")
	}

	// Unbind for safety
	gl.BindVertexArray(0)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)

	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)
	r.grabTexture = r.newTexture(sys.scrrect[2], sys.scrrect[3], 32, true).(*Texture_GLES32)
	r.grabTexture.SetData(nil)

	// create a texture for r.fbo
	gl.GenTextures(1, &r.fbo_texture)
	textureSerialNumber++

	gl.BindTexture(gl.TEXTURE_2D, r.fbo_texture)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	// Don't change this from gl.RGBA.
	// It breaks mixing between subtractive and additive.
	LogDebug("[GLES] Creating RGBA Textures")
	gl.TexImage2D(
		gl.TEXTURE_2D,
		0,
		gl.RGBA,
		r.renderW,
		r.renderH,
		0,
		gl.RGBA,
		gl.UNSIGNED_BYTE,
		nil,
	)

	r.fbo_pp = make([]uint32, 2)
	r.fbo_pp_texture = make([]uint32, 2)

	// Shaders might use negative values, so
	// we specify that we want signed pixels
	// r.fbo_pp_texture
	for i := 0; i < 2; i++ {
		gl.GenTextures(1, &(r.fbo_pp_texture[i]))
		textureSerialNumber++

		gl.BindTexture(gl.TEXTURE_2D, r.fbo_pp_texture[i])
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexImage2D(
			gl.TEXTURE_2D,
			0,
			gl.RGBA8_SNORM,
			sys.scrrect[2],
			sys.scrrect[3],
			0,
			gl.RGBA,
			gl.UNSIGNED_BYTE,
			nil,
		)
	}

	// done with r.fbo_texture, unbind it
	gl.BindTexture(gl.TEXTURE_2D, 0)

	//r.rbo_depth = gl.CreateRenderbuffer()
	gl.GenRenderbuffers(1, &r.rbo_depth)

	gl.BindRenderbuffer(gl.RENDERBUFFER, r.rbo_depth)
	if sys.msaa > 0 {
		//gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, int(sys.scrrect[2]), int(sys.scrrect[3]))
		gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, sys.msaa, gl.DEPTH_COMPONENT16, r.renderW, r.renderH)
	} else {
		gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, r.renderW, r.renderH)
	}
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)
	if sys.msaa > 0 {
		r.fbo_f_texture = r.newTexture(r.renderW, r.renderH, 32, false).(*Texture_GLES32)
		r.fbo_f_texture.SetData(nil)
	} else {
		//r.rbo_depth = gl.CreateRenderbuffer()
		//gl.BindRenderbuffer(gl.RENDERBUFFER, r.rbo_depth)
		//gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, int(sys.scrrect[2]), int(sys.scrrect[3]))
		//gl.BindRenderbuffer(gl.RENDERBUFFER, gl.NoRenderbuffer)
	}

	// create an FBO for our r.fbo, which is then for r.fbo_texture
	gl.GenFramebuffers(1, &r.fbo)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)

	if sys.msaa > 0 {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D_MULTISAMPLE, r.fbo_texture, 0)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)
		if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
			LogMessage("Framebuffer creation failed: 0x%x", status)
		}
		gl.GenFramebuffers(1, &r.fbo_f)
		r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_f)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_f_texture.handle, 0)
	} else {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_texture, 0)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)
	}

	// create our two FBOs for our post-processing needs
	for i := 0; i < 2; i++ {
		gl.GenFramebuffers(1, &(r.fbo_pp[i]))
		r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[i])
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_pp_texture[i], 0)
	}

	// create an FBO for our model stuff
	if r.enableModel {
		if r.enableShadow {
			// create FBO for shadow rendering
			gl.GenFramebuffers(1, &r.fbo_shadow)
			r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0)

			// create a single shadow cube texture (matching desktop now)
			gl.GenTextures(1, &r.fbo_shadow_cube_texture)
			textureSerialNumber++

			// for i := 0; i < 4; i++ {
			gl.BindTexture(gl.TEXTURE_CUBE_MAP_ARRAY, r.fbo_shadow_cube_texture)

			gl.TexStorage3D(gl.TEXTURE_CUBE_MAP_ARRAY, 1, gl.DEPTH_COMPONENT24, 1024, 1024, 4*6)
			gl.TexParameteri(gl.TEXTURE_CUBE_MAP_ARRAY, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
			gl.TexParameteri(gl.TEXTURE_CUBE_MAP_ARRAY, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
			gl.TexParameteri(gl.TEXTURE_CUBE_MAP_ARRAY, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
			gl.TexParameteri(gl.TEXTURE_CUBE_MAP_ARRAY, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

			r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_shadow)
			gl.FramebufferTexture(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, r.fbo_shadow_cube_texture, 0)
			bufs := []uint32{gl.NONE}
			gl.DrawBuffers(1, &bufs[0])
			gl.ReadBuffer(gl.NONE)

			// Allocate depth storage for each face
			// for face := 0; face < 6; face++ {
			// 	gl.TexImage2D(
			// 		uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X)+uint32(face),
			// 		0,
			// 		gl.DEPTH_COMPONENT24,
			// 		1024, 1024,
			// 		0,
			// 		gl.DEPTH_COMPONENT,
			// 		gl.UNSIGNED_INT,
			// 		nil,
			// 	)
			// }

			// gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
			// gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
			// gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
			// gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
			// Note: GL_TEXTURE_WRAP_R can also be set if desired:
			// gl.TexParameteri(gl.TEXTURE_CUBE_MAP, gl.TEXTURE_WRAP_R, gl.CLAMP_TO_EDGE)
			// }

			// Create (empty) FBO. We'll attach the proper cube-face when rendering each face.
			// r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_shadow)
			// Leave the depth attachment empty here — attach per-face with FramebufferTexture2D()
			// Restore default framebuffer binding
			// r.bindFramebuffer(gl.FRAMEBUFFER, 0)
		}
		gl.GenFramebuffers(1, &r.fbo_env)
	}

	r.bindFramebuffer(gl.FRAMEBUFFER, 0)

	// Create palette atlas
	r.createPalAtlas()

	r.InitStateCache()
}

func (r *Renderer_GLES32) Close() {
	// Palette atlas cleaned up via GC finalizer on palAtlas
	r.palAtlas = nil
	r.palFreeSlots = nil
	r.oldPalAtlases = nil
}

func (r *Renderer_GLES32) InitStateCache() {
	// Match standard OpenGL hardware defaults
	r.program = 0
	r.depthTest = false
	r.depthMask = true
	r.doubleSided = true
	r.invertFrontFace = false
	r.blendEnabled = false
	r.scissorEnabled = false

	// FBO binding cache — default framebuffer (0) is bound after Init.
	r.curDrawFbo = 0
	r.curReadFbo = 0

	// Force hardware synchronization
	gl.UseProgram(0)
	gl.Disable(gl.DEPTH_TEST)
	gl.DepthMask(true)
	gl.Disable(gl.CULL_FACE)
	gl.FrontFace(gl.CCW)
	gl.Disable(gl.BLEND)
	gl.Disable(gl.SCISSOR_TEST)

	// Check hardware texture limit
	// TODO: Maybe clamp the result
	var maxTex int32
	gl.GetIntegerv(gl.MAX_TEXTURE_IMAGE_UNITS, &maxTex)

	if r.debugMode {
		LogMessage("[GL Debug] GPU supports up to %d textures", maxTex)
	}

	// Initialize sprite texture cache
	r.texCacheTexSerial = make([]uint64, maxTex)
	r.texCacheLastUsed = make([]uint64, maxTex)

	// Initialize uniform cache
	r.uniformICache = make(map[uint32]int32, 32)
	r.uniformF1Cache = make(map[uint32]float32, 32)
	r.uniformF2Cache = make(map[uint32][2]float32, 32)
	r.uniformF3Cache = make(map[uint32][3]float32, 32)
	r.uniformF4Cache = make(map[uint32][4]float32, 32)
}

func (r *Renderer_GLES32) EnableDebug() {
	r.debugMode = true
	gl.Enable(gl.DEBUG_OUTPUT)
	gl.Enable(gl.DEBUG_OUTPUT_SYNCHRONOUS)

	gl.DebugMessageCallback(func(
		source uint32,
		gltype uint32,
		id uint32,
		severity uint32,
		length int32,
		message string,
		userParam unsafe.Pointer) {

		if severity == gl.DEBUG_SEVERITY_NOTIFICATION {
			return
		}

		LogMessage("[GL Debug] %s", message)

		// Crash here so the log catches it
		if severity == gl.DEBUG_SEVERITY_HIGH {
			panic("Critical OpenGL Error Detected!")
		}
	}, nil)

	LogMessage("[GL Debug] OpenGL ES debug mode enabled")
}

func (r *Renderer_GLES32) IsModelEnabled() bool {
	return r.enableModel
}

func (r *Renderer_GLES32) IsShadowEnabled() bool {
	return r.enableShadow
}

// bindFramebuffer wraps gl.BindFramebuffer with a state cache to skip
// redundant CGo calls on the render hot path.
func (r *Renderer_GLES32) bindFramebuffer(target uint32, fbo uint32) {
	switch target {
	case gl.FRAMEBUFFER:
		if r.curDrawFbo == fbo && r.curReadFbo == fbo {
			return
		}
		drawCallStats.FBOSwitches++
		gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
		r.curDrawFbo = fbo
		r.curReadFbo = fbo
	case gl.DRAW_FRAMEBUFFER:
		if r.curDrawFbo == fbo {
			return
		}
		drawCallStats.FBOSwitches++
		gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, fbo)
		r.curDrawFbo = fbo
	case gl.READ_FRAMEBUFFER:
		if r.curReadFbo == fbo {
			return
		}
		drawCallStats.FBOSwitches++
		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, fbo)
		r.curReadFbo = fbo
	}
}

func (r *Renderer_GLES32) BeginFrame(clearColor bool) {
	drawCallStats.reset()
	lastRenderParams = nil
	resetSpriteQueue()
	if r.useIntermediateFBO {
		r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	} else {
		// No post shaders / MSAA — render directly to the window surface.
		// BeginFrame is called once per game frame so we must manually reset
		// the FBO cache to ensure the first bindFramebuffer in EndFrame fires.
		x, y, w, h := sys.window.GetScaledViewportSize()
		r.bindFramebuffer(gl.FRAMEBUFFER, 0)
		gl.Viewport(x, y, w, h)
		if clearColor {
			gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		} else {
			gl.Clear(gl.DEPTH_BUFFER_BIT)
		}
		return
	}
	gl.Viewport(0, 0, r.renderW, r.renderH)
	if clearColor {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	} else {
		gl.Clear(gl.DEPTH_BUFFER_BIT)
	}
}

func (r *Renderer_GLES32) EndFrame() {
	// Flush any deferred sprite draws accumulated after the last
	// luaFlushDrawQueue (top-layer elements, fades, debug overlays).
	flushSpriteQueue()
	drawCallStats.logFrame(int(sys.frameCounter))

	if len(r.fbo_pp) == 0 {
		return
	}

	// When rendering directly to the default framebuffer (no post shaders, no MSAA),
	// BeginFrame already targeted FBO 0 — nothing left to do in EndFrame.
	if !r.useIntermediateFBO {
		return
	}

	// renderW/renderH: the size of the main render FBO (may be smaller when RenderScale < 1).
	// winW/winH: the full window/screen size used for post-processing FBOs and output.
	x, y := int32(0), int32(0)
	renderW, renderH := r.renderW, r.renderH
	winW, winH := int32(sys.scrrect[2]), int32(sys.scrrect[3])
	time := sdl.GetPerformanceCounter() // consistent time across all shaders

	if sys.msaa > 0 {
		r.bindFramebuffer(gl.DRAW_FRAMEBUFFER, r.fbo_f)
		r.bindFramebuffer(gl.READ_FRAMEBUFFER, r.fbo)
		gl.BlitFramebuffer(x, y, renderW, renderH, x, y, renderW, renderH, gl.COLOR_BUFFER_BIT, gl.LINEAR)
	}
	// Suppress unused-variable warnings for renderW/renderH when MSAA is 0.
	_ = renderW
	_ = renderH

	var scaleMode int32 // GL enum
	if sys.cfg.Video.WindowScaleMode {
		scaleMode = gl.LINEAR
	} else {
		scaleMode = gl.NEAREST
	}

	// set the viewport to the unscaled bounds for post-processing
	gl.Viewport(x, y, winW, winH)
	// clear both of our post-processing FBOs to make sure
	// nothing's there. the output is set later
	for i := 0; i < 2; i++ {
		r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[i])
		gl.Clear(gl.COLOR_BUFFER_BIT)
	}
	r.SetActiveTexture0() //gl.ActiveTexture(gl.TEXTURE0) // later referred to by Texture_GL

	fbo_texture := r.fbo_texture
	if sys.msaa > 0 {
		fbo_texture = r.fbo_f_texture.handle
	}

	// Reset global state
	r.DisableScissor()
	r.DisableBlending()
	r.SetDepthTest(false)
	r.SetDepthMask(false)

	for i := 0; i < len(r.postShaderSelect); i++ {
		postShader := r.postShaderSelect[i]

		// tell GL we want to use our shader program
		r.ChangeProgram(postShader.program)

		// tell GL to use our vertex array object
		// this'll be where our quad is stored
		gl.BindVertexArray(r.postVAO)

		// this is here because it is undefined
		// behavior to write to the same FBO
		if i%2 == 0 {
			// ping! our first post-processing FBO is the output
			r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[0])
			if i == 0 {
				// first pass, use fbo_texture
				gl.BindTexture(gl.TEXTURE_2D, fbo_texture)
			} else {
				// not the first pass, use the second post-processing FBO
				gl.BindTexture(gl.TEXTURE_2D, r.fbo_pp_texture[1])
			}
		} else {
			// pong! our second post-processing FBO is the output
			r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[1])
			// our first post-processing FBO is the input
			gl.BindTexture(gl.TEXTURE_2D, r.fbo_pp_texture[0])
		}

		if i >= len(r.postShaderSelect)-1 {
			// this is the last shader,
			// so we ask GL to scale it and output it
			// to FB0, the default frame buffer that the user sees
			x, y, width, height := sys.window.GetScaledViewportSize()
			gl.Viewport(x, y, width, height)
			r.bindFramebuffer(gl.FRAMEBUFFER, 0)
			// clear FB0 just to make sure
			gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		}

		// set post-processing parameters
		if loc, ok := postShader.uniforms["Texture_GLES32"]; ok && loc >= 0 {
			r.SetUniformISub(loc, 0)
		}
		if loc, ok := postShader.uniforms["TextureSize"]; ok && loc >= 0 {
			r.SetUniformFSub(loc, float32(winW), float32(winH))
		}
		if loc, ok := postShader.uniforms["CurrentTime"]; ok && loc >= 0 {
			r.SetUniformFSub(loc, float32(time))
		}

		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, scaleMode)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, scaleMode)

		// construct the quad and draw it
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	}
}

func (r *Renderer_GLES32) Await() {
	gl.Finish()
}

func (r *Renderer_GLES32) MapBlendEquation(i BlendEquation) uint32 {
	var BlendEquationLUT = map[BlendEquation]uint32{
		BlendAdd:             gl.FUNC_ADD,
		BlendReverseSubtract: gl.FUNC_REVERSE_SUBTRACT,
	}
	return BlendEquationLUT[i]
}

func (r *Renderer_GLES32) MapBlendFunction(i BlendFunc) uint32 {
	var BlendFunctionLUT = map[BlendFunc]uint32{
		BlendOne:              gl.ONE,
		BlendZero:             gl.ZERO,
		BlendSrcAlpha:         gl.SRC_ALPHA,
		BlendOneMinusSrcAlpha: gl.ONE_MINUS_SRC_ALPHA,
		BlendDstColor:         gl.DST_COLOR,
		BlendOneMinusDstColor: gl.ONE_MINUS_DST_COLOR,
	}
	return BlendFunctionLUT[i]
}

func (r *Renderer_GLES32) MapPrimitiveMode(i PrimitiveMode) uint32 {
	var PrimitiveModeLUT = map[PrimitiveMode]uint32{
		LINES:          gl.LINES,
		LINE_LOOP:      gl.LINE_LOOP,
		LINE_STRIP:     gl.LINE_STRIP,
		TRIANGLES:      gl.TRIANGLES,
		TRIANGLE_STRIP: gl.TRIANGLE_STRIP,
		TRIANGLE_FAN:   gl.TRIANGLE_FAN,
	}
	return PrimitiveModeLUT[i]
}

func (r *Renderer_GLES32) SetDepthTest(depthTest bool) {
	if depthTest != r.depthTest {
		r.depthTest = depthTest
		if depthTest {
			gl.Enable(gl.DEPTH_TEST)
			gl.DepthFunc(gl.LESS)
		} else {
			gl.Disable(gl.DEPTH_TEST)
		}
	}
}

// Note: This one defaults to enable so we must sync the cache early
func (r *Renderer_GLES32) SetDepthMask(depthMask bool) {
	if depthMask != r.depthMask {
		r.depthMask = depthMask
		gl.DepthMask(depthMask)
	}
}

func (r *Renderer_GLES32) SetFrontFace(invertFrontFace bool) {
	if invertFrontFace != r.invertFrontFace {
		r.invertFrontFace = invertFrontFace
		if invertFrontFace {
			gl.FrontFace(gl.CW)
		} else {
			gl.FrontFace(gl.CCW)
		}
	}
}

func (r *Renderer_GLES32) SetCullFace(doubleSided bool) {
	if doubleSided != r.doubleSided {
		r.doubleSided = doubleSided
		if !doubleSided {
			gl.Enable(gl.CULL_FACE)
			gl.CullFace(gl.BACK)
		} else {
			gl.Disable(gl.CULL_FACE)
		}
	}
}

// This should be called instead of gl.UseProgram()
func (r *Renderer_GLES32) ChangeProgram(prog uint32) {
	// Program already in use
	if r.program == prog {
		return
	}

	// Lazy release of sprite pipeline
	// We can't tell if the next thing we will draw is also a sprite, so this prevents releasing the pipeline after every single sprite
	if r.program == r.spriteShader.program {
		r.ReleasePipeline()
	}

	// Same for TTF fonts
	if r.program == gfxFont.(*FontRenderer_GLES32).shaderProgram.program {
		gfxFont.(*FontRenderer_GLES32).ReleaseFontPipeline()
	}

	// Switch program
	gl.UseProgram(prog)
	r.program = prog

	// Reset sprite texture cache
	for i := range r.texCacheTexSerial {
		r.texCacheTexSerial[i] = 0
		r.texCacheLastUsed[i] = 0
	}
	r.texCacheTimer = 1
}

func (r *Renderer_GLES32) EnableBlending(eq BlendEquation, src, dst BlendFunc) {
	if !r.blendEnabled {
		gl.Enable(gl.BLEND)
		r.blendEnabled = true
	}

	if eq != r.blendEquation {
		r.blendEquation = eq
		gl.BlendEquation(r.MapBlendEquation(eq))
	}

	if src != r.blendSrc || dst != r.blendDst {
		r.blendSrc = src
		r.blendDst = dst
		gl.BlendFunc(r.MapBlendFunction(src), r.MapBlendFunction(dst))
	}
}

func (r *Renderer_GLES32) DisableBlending() {
	if r.blendEnabled {
		gl.Disable(gl.BLEND)
		r.blendEnabled = false
		// Do not update blend equation cache because the hardware doesn't
	}
}

func (r *Renderer_GLES32) SetPipeline() {
	// Do nothing if we were already using the sprite shader
	if r.program == r.spriteShader.program {
		return
	}

	r.ChangeProgram(r.spriteShader.program)

	gl.BindVertexArray(r.spriteVAO)
}

func (r *Renderer_GLES32) ReleasePipeline() {
	gl.BindVertexArray(0)
	//r.DisableBlending()
}

func (r *Renderer_GLES32) prepareShadowMapPipeline(bufferIndex uint32) {
	r.ChangeProgram(r.shadowMapShader.program)

	gl.BindVertexArray(r.modelVAO)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_shadow)
	gl.Viewport(0, 0, 1024, 1024)
	// Removed gl.Enable(gl.TEXTURE_2D) — not needed / invalid in GLES3 core

	// Set global state
	r.SetDepthTest(true)
	r.SetDepthMask(true)
	r.DisableBlending()
	r.SetFrontFace(r.invertFrontFace)
	r.SetCullFace(r.doubleSided)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.modelVertexBuffer[bufferIndex])
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.modelIndexBuffer[bufferIndex])

	// Don't attach a depth texture here: attach the specific cube-face with
	// SetShadowFrameCubeTexture(...) before rendering each face.
	// Clearing must be done after the correct face is attached.

	r.SetActiveTexture0() // gl.ActiveTexture(gl.TEXTURE0)
}

func (r *Renderer_GLES32) setShadowMapPipeline(doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	r.SetFrontFace(invertFrontFace)
	r.SetCullFace(doubleSided)

	loc := r.shadowMapShader.attributes["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.shadowMapShader.attributes["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices

	if useUV {
		r.useUV = true
		loc = r.shadowMapShader.attributes["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else if r.useUV {
		r.useUV = false
		loc = r.shadowMapShader.attributes["uv"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib2f(uint32(loc), 0, 0)
	}

	if useNormal {
		offset += 12 * numVertices
	}
	if useTangent {
		offset += 16 * numVertices
	}
	if useVertColor {
		loc = r.shadowMapShader.attributes["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.shadowMapShader.attributes["vertColor"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		r.useJoint0 = true
		loc = r.shadowMapShader.attributes["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.shadowMapShader.attributes["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			r.useJoint1 = true
			loc = r.shadowMapShader.attributes["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.shadowMapShader.attributes["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else if r.useJoint1 {
			r.useJoint1 = false
			loc = r.shadowMapShader.attributes["joints_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.shadowMapShader.attributes["weights_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else if r.useJoint0 {
		r.useJoint0 = false
		r.useJoint1 = false
		loc = r.shadowMapShader.attributes["joints_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.attributes["weights_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.attributes["joints_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.attributes["weights_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}

func (r *Renderer_GLES32) ReleaseShadowPipeline() {
	loc := r.shadowMapShader.attributes["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.shadowMapShader.attributes["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.shadowMapShader.attributes["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib2f(uint32(loc), 0, 0)
	loc = r.shadowMapShader.attributes["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	loc = r.shadowMapShader.attributes["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.shadowMapShader.attributes["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.shadowMapShader.attributes["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.shadowMapShader.attributes["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	//gl.Disable(gl.TEXTURE_2D)
	r.SetDepthMask(true)
	r.SetDepthTest(false)
	//gl.Disable(gl.CULL_FACE)
	r.SetCullFace(true)
	r.DisableBlending()
	r.useUV = false
	r.useJoint0 = false
	r.useJoint1 = false
}

func (r *Renderer_GLES32) prepareModelPipeline(bufferIndex uint32, env *Environment) {
	r.ChangeProgram(r.modelShader.program)

	gl.BindVertexArray(r.modelVAO)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)

	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	r.SetDepthMask(true)
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	//gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.TEXTURE_CUBE_MAP)
	// Set global state
	r.EnableBlending(r.blendEquation, r.blendSrc, r.blendDst)
	r.SetDepthTest(true)
	r.SetFrontFace(r.invertFrontFace)
	r.SetCullFace(r.doubleSided)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.modelVertexBuffer[bufferIndex])
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.modelIndexBuffer[bufferIndex])
	// Bind the 4 cube-map textures to the corresponding sampler uniforms/units
	if r.enableShadow {
		loc, unit := r.modelShader.uniforms["shadowCubeMap"], r.modelShader.textures["shadowCubeMap"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP_ARRAY, r.fbo_shadow_cube_texture)
		gl.Uniform1i(loc, int32(unit))
		// for i := 0; i < 4; i++ {
		// 	name := fmt.Sprintf("shadowCubeMap%d", i)
		// 	loc := r.modelShader.uniforms[name]
		// 	unit := r.modelShader.textures[name] // texture unit assigned by RegisterTextures

		// 	gl.ActiveTexture(uint32(gl.TEXTURE0 + unit))
		// 	gl.BindTexture(gl.TEXTURE_CUBE_MAP, r.fbo_shadow_cube_textures[i])
		// 	gl.Uniform1i(loc, int32(unit))
		// }
	}
	if env != nil {
		loc, unit := r.modelShader.uniforms["lambertianEnvSampler"], r.modelShader.textures["lambertianEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, env.lambertianTexture.tex.(*Texture_GLES32).handle)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.uniforms["GGXEnvSampler"], r.modelShader.textures["GGXEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, env.GGXTexture.tex.(*Texture_GLES32).handle)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.uniforms["GGXLUT"], r.modelShader.textures["GGXLUT"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_2D, env.GGXLUT.tex.(*Texture_GLES32).handle)
		gl.Uniform1i(loc, int32(unit))

		loc = r.modelShader.uniforms["environmentIntensity"]
		gl.Uniform1f(loc, env.environmentIntensity)
		loc = r.modelShader.uniforms["mipCount"]
		gl.Uniform1i(loc, env.mipmapLevels)
		loc = r.modelShader.uniforms["environmentRotation"]
		rotationMatrix := mgl.Rotate3DX(math.Pi).Mul3(mgl.Rotate3DY(0.5 * math.Pi))
		rotationM := rotationMatrix[:]
		gl.UniformMatrix3fv(loc, 1, false, &rotationM[0])

	} else {
		loc, unit := r.modelShader.uniforms["lambertianEnvSampler"], r.modelShader.textures["lambertianEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, 0)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.uniforms["GGXEnvSampler"], r.modelShader.textures["GGXEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, 0)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.uniforms["GGXLUT"], r.modelShader.textures["GGXLUT"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_2D, 0)
		gl.Uniform1i(loc, int32(unit))
		loc = r.modelShader.uniforms["environmentIntensity"]
		gl.Uniform1f(loc, 0)
	}

	r.SetActiveTexture0() // gl.ActiveTexture(gl.TEXTURE0)
}

func (r *Renderer_GLES32) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace,
	useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1, useOutlineAttribute bool, numVertices, vertAttrOffset uint32) {
	r.SetDepthTest(depthTest)
	r.SetDepthMask(depthMask)
	r.SetFrontFace(invertFrontFace)
	r.SetCullFace(doubleSided)
	r.EnableBlending(eq, src, dst)

	loc := r.modelShader.attributes["inVertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.modelShader.attributes["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices

	if useUV {
		r.useUV = true
		loc = r.modelShader.attributes["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else if r.useUV {
		r.useUV = false
		loc = r.modelShader.attributes["uv"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib2f(uint32(loc), 0, 0)
	}

	if useNormal {
		r.useNormal = true
		loc = r.modelShader.attributes["normalIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
		offset += 12 * numVertices
	} else if r.useNormal {
		r.useNormal = false
		loc = r.modelShader.attributes["normalIn"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib3f(uint32(loc), 0, 0, 0)
	}
	if useTangent {
		r.useTangent = true
		loc = r.modelShader.attributes["tangentIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else if r.useTangent {
		r.useTangent = false
		loc = r.modelShader.attributes["tangentIn"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
	if useVertColor {
		r.useVertColor = true
		loc = r.modelShader.attributes["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else if r.useVertColor {
		r.useVertColor = false
		loc = r.modelShader.attributes["vertColor"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		r.useJoint0 = true
		loc = r.modelShader.attributes["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.modelShader.attributes["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			r.useJoint1 = true
			loc = r.modelShader.attributes["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.modelShader.attributes["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else if r.useJoint1 {
			r.useJoint1 = false
			loc = r.modelShader.attributes["joints_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.modelShader.attributes["weights_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else if r.useJoint0 {
		r.useJoint0 = false
		r.useJoint1 = false
		loc = r.modelShader.attributes["joints_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.attributes["weights_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.attributes["joints_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.attributes["weights_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
	if useOutlineAttribute {
		r.useOutlineAttribute = true
		loc = r.modelShader.attributes["outlineAttributeIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else if r.useOutlineAttribute {
		r.useOutlineAttribute = false
		loc = r.modelShader.attributes["outlineAttributeIn"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}

func (r *Renderer_GLES32) SetMeshOutlinePipeline(invertFrontFace bool, meshOutline float32) {
	r.SetFrontFace(invertFrontFace)
	r.SetDepthTest(true)
	r.SetDepthMask(true)

	loc := r.modelShader.uniforms["meshOutline"]
	gl.Uniform1f(loc, meshOutline)
}

func (r *Renderer_GLES32) ReleaseModelPipeline() {
	loc := r.modelShader.attributes["inVertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.attributes["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.attributes["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib2f(uint32(loc), 0, 0)
	loc = r.modelShader.attributes["normalIn"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib3f(uint32(loc), 0, 0, 0)
	loc = r.modelShader.attributes["tangentIn"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.attributes["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	loc = r.modelShader.attributes["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.attributes["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.attributes["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.attributes["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.attributes["outlineAttributeIn"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	//gl.Disable(gl.TEXTURE_2D)
	r.SetDepthMask(true)
	r.SetDepthTest(false)
	//gl.Disable(gl.CULL_FACE)
	r.SetCullFace(true)
	r.useUV = false
	r.useNormal = false
	r.useTangent = false
	r.useVertColor = false
	r.useJoint0 = false
	r.useJoint1 = false
	r.useOutlineAttribute = false
}

func (r *Renderer_GLES32) ReadPixels(data []uint8, width, height int) {
	// we defer the EndFrame(), SwapBuffers(), and BeginFrame() calls that were previously below now to
	// a single spot in order to prevent the blank screenshot bug on single digit FPS
	r.bindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
}

func (r *Renderer_GLES32) EnableScissor(x, y, width, height int32) {
	// Flip Y to OpenGL convention
	realY := sys.scrrect[3] - (y + height)

	if r.scissorEnabled &&
		r.scissorRect[0] == x && r.scissorRect[1] == realY &&
		r.scissorRect[2] == width && r.scissorRect[3] == height {
		return
	}

	if !r.scissorEnabled {
		gl.Enable(gl.SCISSOR_TEST)
		r.scissorEnabled = true
	}

	gl.Scissor(x, realY, width, height)
	r.scissorRect = [4]int32{x, realY, width, height}
}

func (r *Renderer_GLES32) DisableScissor() {
	if r.scissorEnabled {
		gl.Disable(gl.SCISSOR_TEST)
		r.scissorEnabled = false
		// Do not zero r.scissorRect here because the hardware retains the last rect even when the test is off
	}
}

func (r *Renderer_GLES32) SetUniformISub(loc int32, val int32) {
	if loc < 0 {
		return
	}

	// Cached path for the sprite shader
	if r.program == r.spriteShader.program {
		key := (r.program << 16) | uint32(loc)
		if old, exists := r.uniformICache[key]; exists && old == val {
			return
		}
		r.uniformICache[key] = val
	}

	gl.Uniform1i(loc, val)
}

func (r *Renderer_GLES32) SetUniformFSub(loc int32, values ...float32) {
	if loc < 0 || len(values) == 0 {
		return
	}

	// Cached path for the sprite shader
	if r.program == r.spriteShader.program {
		key := (r.program << 16) | uint32(loc)

		switch len(values) {
		case 1:
			if old, exists := r.uniformF1Cache[key]; exists && old == values[0] {
				return
			}
			r.uniformF1Cache[key] = values[0]
		case 2:
			v2 := [2]float32{values[0], values[1]}
			if old, exists := r.uniformF2Cache[key]; exists && old == v2 {
				return
			}
			r.uniformF2Cache[key] = v2
		case 3:
			v3 := [3]float32{values[0], values[1], values[2]}
			if old, exists := r.uniformF3Cache[key]; exists && old == v3 {
				return
			}
			r.uniformF3Cache[key] = v3
		case 4:
			v4 := [4]float32{values[0], values[1], values[2], values[3]}
			if old, exists := r.uniformF4Cache[key]; exists && old == v4 {
				return
			}
			r.uniformF4Cache[key] = v4
		}
	}

	// Uncached path
	switch len(values) {
	case 1:
		gl.Uniform1f(loc, values[0])
	case 2:
		gl.Uniform2f(loc, values[0], values[1])
	case 3:
		gl.Uniform3f(loc, values[0], values[1], values[2])
	case 4:
		gl.Uniform4f(loc, values[0], values[1], values[2], values[3])
	}
}

func (r *Renderer_GLES32) SetUniformFvSub(loc int32, values []float32) {
	if loc < 0 || len(values) == 0 {
		return
	}

	switch len(values) {
	case 1, 2, 3, 4:
		r.SetUniformFSub(loc, values...)
	case 8:
		gl.Uniform4fv(loc, 2, &values[0])
	default:
		gl.Uniform1fv(loc, int32(len(values)), &values[0])
	}
}

func (r *Renderer_GLES32) SetUniformI(name string, val int) {
	loc := r.currentProgram.uniforms[name]
	r.SetUniformISub(loc, int32(val))
}

func (r *Renderer_GLES32) SetUniformF(name string, values ...float32) {
	loc := r.currentProgram.uniforms[name]
	r.SetUniformFSub(loc, values...)
}

func (r *Renderer_GLES32) SetUniformFv(name string, values []float32) {
	loc := r.currentProgram.uniforms[name]
	r.SetUniformFvSub(loc, values)
}

// Caching matrices is as expensive as direct function calls
func (r *Renderer_GLES32) SetUniformMatrix(name string, value []float32) {
	loc, ok := r.currentProgram.uniforms[name]
	if ok && loc >= 0 {
		gl.UniformMatrix4fv(loc, 1, false, &value[0])
	}
}

func (r *Renderer_GLES32) SetModelUniformI(name string, val int) {
	loc, ok := r.modelShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Model uniform '%s' not registered", name)
		}
		return
	}
	r.SetUniformISub(loc, int32(val))
}

func (r *Renderer_GLES32) SetModelUniformF(name string, values ...float32) {
	loc, ok := r.modelShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Model uniform '%s' not registered", name)
		}
		return
	}
	r.SetUniformFSub(loc, values...)
}

func (r *Renderer_GLES32) SetModelUniformFv(name string, values []float32) {
	loc, ok := r.modelShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Model uniform '%s' not registered", name)
		}
		return
	}
	r.SetUniformFvSub(loc, values)
}

func (r *Renderer_GLES32) SetModelUniformMatrix(name string, value []float32) {
	loc, ok := r.modelShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Model uniform '%s' not registered", name)
		}
		return
	}
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES32) SetModelUniformMatrix3(name string, value []float32) {
	loc, ok := r.modelShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Model uniform '%s' not registered", name)
		}
		return
	}
	gl.UniformMatrix3fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES32) SetShadowMapUniformI(name string, val int) {
	loc, ok := r.shadowMapShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Shadow uniform '%s' not registered", name)
		}
		return
	}
	r.SetUniformISub(loc, int32(val))
}

func (r *Renderer_GLES32) SetShadowMapUniformF(name string, values ...float32) {
	loc, ok := r.shadowMapShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Shadow uniform '%s' not registered", name)
		}
		return
	}
	r.SetUniformFSub(loc, values...)
}

func (r *Renderer_GLES32) SetShadowMapUniformFv(name string, values []float32) {
	loc, ok := r.shadowMapShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Shadow uniform '%s' not registered", name)
		}
		return
	}
	r.SetUniformFvSub(loc, values)
}

func (r *Renderer_GLES32) SetShadowMapUniformMatrix(name string, value []float32) {
	loc, ok := r.shadowMapShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Shadow uniform '%s' not registered", name)
		}
		return
	}
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES32) SetShadowMapUniformMatrix3(name string, value []float32) {
	loc, ok := r.shadowMapShader.uniforms[name]
	if !ok || loc < 0 {
		if r.debugMode {
			LogMessage("[GL Debug] Shadow uniform '%s' not registered", name)
		}
		return
	}
	gl.UniformMatrix3fv(loc, 1, false, &value[0])
}

// Selects texture unit 0 as active and tells the cache it's dirty
// Prevents the sprite renderer from desyncing during texture maintenance
func (r *Renderer_GLES32) SetActiveTexture0() {
	gl.ActiveTexture(gl.TEXTURE0)

	if len(r.texCacheTexSerial) > 0 {
		r.texCacheTexSerial[0] = 0
		r.texCacheLastUsed[0] = 0
	}
}

func (r *Renderer_GLES32) SetTextureSub(uMap map[string]int32, tMap map[string]int, name string, tex Texture) {
	t := tex.(*Texture_GLES32)
	loc := uMap[name]

	// Cached path for the sprite shader
	// Note: The cache doesn't care if a texture is "tex" or "pal"
	if r.program == r.spriteShader.program {
		// Increment reference timer
		r.texCacheTimer++

		var oldestUnit int32 = 0
		var minTime uint64 = math.MaxUint64

		// Look for a hit or the oldest slot
		for i := range r.texCacheTexSerial {
			// If we find the texture already bound, that's a hit
			if r.texCacheTexSerial[i] == t.serial {
				r.texCacheLastUsed[i] = r.texCacheTimer
				r.SetUniformISub(loc, int32(i))
				return
			}

			// While searching, track the oldest slot in case we miss
			if r.texCacheLastUsed[i] < minTime {
				minTime = r.texCacheLastUsed[i]
				oldestUnit = int32(i)
			}
		}

		// Cache miss
		gl.ActiveTexture(gl.TEXTURE0 + uint32(oldestUnit))
		gl.BindTexture(gl.TEXTURE_2D, t.handle)

		// Update cache state
		r.texCacheTexSerial[oldestUnit] = t.serial
		r.texCacheLastUsed[oldestUnit] = r.texCacheTimer

		// Update uniform
		r.SetUniformISub(loc, oldestUnit)
		return
	}

	// Uncached path
	fixedUnit := uint32(tMap[name])
	gl.ActiveTexture(gl.TEXTURE0 + fixedUnit)
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	r.SetUniformISub(loc, int32(fixedUnit))
}

func (r *Renderer_GLES32) SetTexture(name string, tex Texture) {
	r.SetTextureSub(r.currentProgram.uniforms, r.currentProgram.textures, name, tex)
}

func (r *Renderer_GLES32) SetModelTexture(name string, tex Texture) {
	r.SetTextureSub(r.modelShader.uniforms, r.modelShader.textures, name, tex)
}

func (r *Renderer_GLES32) SetShadowMapTexture(name string, tex Texture) {
	r.SetTextureSub(r.shadowMapShader.uniforms, r.shadowMapShader.textures, name, tex)
}

func (r *Renderer_GLES32) SetShadowFrameTexture(i uint32) {
	// Backwards-compatible alias: treat i as combined index (light*6 + face)
	gl.FramebufferTexture(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, r.fbo_shadow_cube_texture, 0)
}

func (r *Renderer_GLES32) SetShadowFrameCubeTexture(i uint32) {
	gl.FramebufferTexture(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, r.fbo_shadow_cube_texture, 0)
	// 	// Interpret i as a combined index: lightIndex = i / 6, faceIndex = i % 6
	// 	lightIndex := int(i / 6)
	// 	faceIndex := int(i % 6)

	// 	// clamp lightIndex to available range
	// 	if lightIndex < 0 {
	// 		lightIndex = 0
	// 	}
	// 	if lightIndex >= len(r.fbo_shadow_cube_textures) {
	// 		lightIndex = len(r.fbo_shadow_cube_textures) - 1
	// 	}
	// 	if faceIndex < 0 {
	// 		faceIndex = 0
	// 	}
	// 	if faceIndex > 5 {
	// 		faceIndex = 5
	// 	}

	// 	tex := r.fbo_shadow_cube_textures[lightIndex]

	// 	// Attach the requested face of the cube map as the framebuffer depth attachment.
	// 	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_shadow)

	// 	target := uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X) + uint32(faceIndex)
	// 	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, target, tex, 0)

	// 	// Make sure draw/read buffers are set appropriately (depth-only FBO)
	// 	bufs := []uint32{gl.NONE}
	// 	gl.DrawBuffers(1, &bufs[0])
	// 	gl.ReadBuffer(gl.NONE)

	// // Clear the depth buffer for this face before rendering
	// gl.Clear(gl.DEPTH_BUFFER_BIT)
}

func (r *Renderer_GLES32) SetVertexData(values ...float32) {
	n := len(values) * 4
	if cap(r.vertexScratch) < n {
		r.vertexScratch = make([]byte, n)
	}
	data := r.vertexScratch[:n]
	for i, v := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(v))
	}
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
}

func (r *Renderer_GLES32) SetModelVertexData(bufferIndex uint32, values []byte) {
	gl.BindBuffer(gl.ARRAY_BUFFER, r.modelVertexBuffer[bufferIndex])
	gl.BufferData(gl.ARRAY_BUFFER, len(values), unsafe.Pointer(&values[0]), gl.STATIC_DRAW)
}

func (r *Renderer_GLES32) SetModelIndexData(bufferIndex uint32, values ...uint32) {
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, values)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.modelIndexBuffer[bufferIndex])
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(values)*4, unsafe.Pointer(&data.Bytes()[0]), gl.STATIC_DRAW)
}

func (r *Renderer_GLES32) RenderQuad() {
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
}

func (r *Renderer_GLES32) RenderElements(mode PrimitiveMode, count, offset int) {
	gl.DrawElementsWithOffset(r.MapPrimitiveMode(mode), int32(count), gl.UNSIGNED_INT, uintptr(offset))
}

func (r *Renderer_GLES32) RenderShadowMapElements(mode PrimitiveMode, count, offset int) {
	r.RenderElements(mode, count, offset)
}

func (r *Renderer_GLES32) RenderCubeMap(envTex Texture, cubeTex Texture) {
	envTexture := envTex.(*Texture_GLES32)
	cubeTexture := cubeTex.(*Texture_GLES32)
	textureSize := cubeTexture.width

	r.ChangeProgram(r.panoramaToCubeMapShader.program)

	gl.BindVertexArray(r.modelEnvVAO)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_env)
	gl.Viewport(0, 0, textureSize, textureSize)

	data := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)

	loc, unit := r.panoramaToCubeMapShader.uniforms["panorama"], r.panoramaToCubeMapShader.textures["panorama"]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, envTexture.handle)
	gl.Uniform1i(loc, int32(unit))

	for i := 0; i < 6; i++ {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+i), cubeTexture.handle, 0)

		gl.Clear(gl.COLOR_BUFFER_BIT)
		loc := r.panoramaToCubeMapShader.uniforms["currentFace"]
		gl.Uniform1i(loc, int32(i))

		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	}

	gl.BindVertexArray(0)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.GenerateMipmap(gl.TEXTURE_CUBE_MAP)
}

func (r *Renderer_GLES32) RenderFilteredCubeMap(distribution int32, cubeTex Texture, filteredTex Texture, mipmapLevel, sampleCount int32, roughness float32) {
	cubeTexture := cubeTex.(*Texture_GLES32)
	filteredTexture := filteredTex.(*Texture_GLES32)
	textureSize := filteredTexture.width
	currentTextureSize := textureSize >> mipmapLevel

	r.ChangeProgram(r.cubemapFilteringShader.program)

	gl.BindVertexArray(r.modelEnvVAO)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_env)
	gl.Viewport(0, 0, currentTextureSize, currentTextureSize)

	data := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)

	loc, unit := r.cubemapFilteringShader.uniforms["cubeMap"], r.cubemapFilteringShader.textures["cubeMap"]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.Uniform1i(loc, int32(unit))
	loc = r.cubemapFilteringShader.uniforms["sampleCount"]
	gl.Uniform1i(loc, sampleCount)
	loc = r.cubemapFilteringShader.uniforms["distribution"]
	gl.Uniform1i(loc, distribution)
	loc = r.cubemapFilteringShader.uniforms["width"]
	gl.Uniform1i(loc, textureSize)
	loc = r.cubemapFilteringShader.uniforms["roughness"]
	gl.Uniform1f(loc, roughness)
	loc = r.cubemapFilteringShader.uniforms["intensityScale"]
	gl.Uniform1f(loc, 1)
	loc = r.cubemapFilteringShader.uniforms["isLUT"]
	gl.Uniform1i(loc, 0)

	for i := 0; i < 6; i++ {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+i), filteredTexture.handle, mipmapLevel)

		gl.Clear(gl.COLOR_BUFFER_BIT)
		loc := r.cubemapFilteringShader.uniforms["currentFace"]
		gl.Uniform1i(loc, int32(i))

		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	}

	gl.BindVertexArray(0)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}

func (r *Renderer_GLES32) RenderLUT(distribution int32, cubeTex Texture, lutTex Texture, sampleCount int32) {
	cubeTexture := cubeTex.(*Texture_GLES32)
	lutTexture := lutTex.(*Texture_GLES32)
	textureSize := lutTexture.width

	r.ChangeProgram(r.cubemapFilteringShader.program)

	gl.BindVertexArray(r.modelEnvVAO)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo_env)
	gl.Viewport(0, 0, textureSize, textureSize)

	data := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)

	loc, unit := r.cubemapFilteringShader.uniforms["cubeMap"], r.cubemapFilteringShader.textures["cubeMap"]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.Uniform1i(loc, int32(unit))
	loc = r.cubemapFilteringShader.uniforms["sampleCount"]
	gl.Uniform1i(loc, sampleCount)
	loc = r.cubemapFilteringShader.uniforms["distribution"]
	gl.Uniform1i(loc, distribution)
	loc = r.cubemapFilteringShader.uniforms["width"]
	gl.Uniform1i(loc, textureSize)
	loc = r.cubemapFilteringShader.uniforms["roughness"]
	gl.Uniform1f(loc, 0)
	loc = r.cubemapFilteringShader.uniforms["intensityScale"]
	gl.Uniform1f(loc, 1)
	loc = r.cubemapFilteringShader.uniforms["currentFace"]
	gl.Uniform1i(loc, 0)
	loc = r.cubemapFilteringShader.uniforms["isLUT"]
	gl.Uniform1i(loc, 1)

	gl.BindTexture(gl.TEXTURE_2D, lutTexture.handle)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA16F, lutTexture.width, lutTexture.height, 0, gl.RGBA, gl.HALF_FLOAT, nil)

	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, lutTexture.handle, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)

	gl.BindVertexArray(0)
	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}

func (r *Renderer_GLES32) PerspectiveProjectionMatrix(angle, aspect, near, far float32) mgl.Mat4 {
	return mgl.Perspective(angle, aspect, near, far)
}

func (r *Renderer_GLES32) OrthographicProjectionMatrix(left, right, bottom, top, near, far float32) mgl.Mat4 {
	ret := mgl.Ortho(left, right, bottom, top, near, far)
	return ret
}

func (r *Renderer_GLES32) NewWorkerThread() bool {
	return false
}

func (r *Renderer_GLES32) SetVSync(interval int) {
	sdl.GLSetSwapInterval(interval)
}

func (r *Renderer_GLES32) LoadCustomSpriteShader(shaderName string, shaderData []byte) uint32 {
	if id, ok := r.customShaderMap[shaderName]; ok {
		return id
	}

	fragSource := string(shaderData)

	shader, err := r.newShaderProgram(vertShader, fragSource, "", "Custom Shader: "+shaderName, false)
	if err != nil {
		LogError("[GLES] Error: Failed to compile custom shader %s: %v", shaderName, err)
		return 0
	}

	shader.RegisterAttributes("position", "uv")
	shader.RegisterUniforms("modelview", "projection", "x1x2x4x3",
		"alpha", "tint", "mask", "neg", "gray", "add", "mult", "isFlat", "isRgba", "isTrapez", "hue",
		"iTime", "iResolution", "aspectRatio", "sTime")
	shader.RegisterTextures("pal", "tex", "tex1", "tex2", "bgl_RenderedTexture")

	shader.needsGrabPass = strings.Contains(fragSource, "bgl_RenderedTexture")

	id := r.nextShaderID
	r.nextShaderID++
	r.customShaders[id] = shader
	r.customShaderMap[shaderName] = id

	sys.appendToConsole(fmt.Sprintf("Loaded Custom Shader: %s (ID: %d, NeedsGrabPass: %v)", shaderName, id, shader.needsGrabPass))
	return id
}

func (r *Renderer_GLES32) UnloadCustomSpriteShader(shaderName string) {
	if id, exists := r.customShaderMap[shaderName]; exists {
		if shader, hasProg := r.customShaders[id]; hasProg {
			gl.DeleteProgram(shader.program)
			delete(r.customShaders, id)
			if r.currentProgram == shader {
				r.currentProgram = nil
			}
		}
		delete(r.customShaderMap, shaderName)
		//LogDebug("Unloaded Custom GLES Shader: %s", shaderName)
	}
}

func (r *Renderer_GLES32) SetSpritePipeline(shaderName string) {
	targetShader := r.spriteShader
	if shaderName != "" {
		if id, ok := r.customShaderMap[shaderName]; ok {
			if shader, ok := r.customShaders[id]; ok {
				targetShader = shader
			}
		}
	}

	if r.program != targetShader.program {
		r.currentProgram = targetShader
		r.ChangeProgram(targetShader.program)
		gl.BindVertexArray(r.spriteVAO)
	}
}

func (r *Renderer_GLES32) SetCustomUniforms(params [16]float32) {
	if r.currentProgram == nil {
		return
	}
	for i := 0; i < 16; i++ {
		loc := gl.GetUniformLocation(r.currentProgram.program, gl.Str(fmt.Sprintf("p%d\x00", i)))
		if loc >= 0 {
			gl.Uniform1f(loc, params[i])
		}
	}
}

func (r *Renderer_GLES32) NeedsGrabPass() bool {
	if r.currentProgram != nil {
		return r.currentProgram.needsGrabPass
	}
	return false
}

func (r *Renderer_GLES32) ResolveBackBuffer() Texture {
	r.SetActiveTexture0()
	gl.BindTexture(gl.TEXTURE_2D, r.grabTexture.handle)

	r.bindFramebuffer(gl.READ_FRAMEBUFFER, r.fbo)
	gl.ReadBuffer(gl.COLOR_ATTACHMENT0)

	gl.CopyTexSubImage2D(gl.TEXTURE_2D, 0, 0, 0, 0, 0, r.grabTexture.width, r.grabTexture.height)

	r.bindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	return r.grabTexture
}

// ------------------------------------------------------------------
// Instanced sprite batching (Phase 4/5)
// ------------------------------------------------------------------

func sameBatchKey(a, b *SpriteDrawCall) bool {
	if a.isFlat != b.isFlat ||
		a.blendEq != b.blendEq ||
		a.blendSrc != b.blendSrc ||
		a.blendDst != b.blendDst ||
		a.isRgba != b.isRgba ||
		a.isTrapez != b.isTrapez ||
		a.mask != b.mask ||
		a.hasScissor != b.hasScissor {
		return false
	}
	return a.scissor == b.scissor
}

func recordBatchBoundary(a, b *SpriteDrawCall) {
	if a.isFlat != b.isFlat {
		batchBreakFlat++
	}
	if a.blendEq != b.blendEq || a.blendSrc != b.blendSrc || a.blendDst != b.blendDst {
		batchBreakBlend++
	}
	if a.isRgba != b.isRgba {
		batchBreakRgba++
	}
	if a.isTrapez != b.isTrapez {
		batchBreakTrapez++
	}
	if a.mask != b.mask {
		batchBreakMask++
	}
	if a.hasScissor != b.hasScissor || a.scissor != b.scissor {
		batchBreakScis++
	}
}

// buildSlots maps each unique (sprite, palette) texture pair in a batch to a
// texture unit slot, writing into a fixed-size array so the render hot path
// allocates nothing. The slot index equals the array index. Returns the count.
func buildSlots(calls []SpriteDrawCall, slots *[maxTexSlots]texSlotInfo) int {
	count := 0
	for i := range calls {
		k := texKey{calls[i].texSerial, calls[i].palSerial}
		j := 0
		for ; j < count; j++ {
			if slots[j].key == k {
				break
			}
		}
		if j == count {
			slots[count] = texSlotInfo{key: k, tex: calls[i].tex, pal: calls[i].paltex}
			count++
		}
	}
	return count
}

// flushSpriteQueueBatched groups consecutive compatible calls in the deferred
// draw queue and issues one instanced draw call per batch. Texture identity is
// deliberately NOT part of the batch key (Phase 5): a batch may span up to
// maxTexSlots unique texture pairs, which removes the texture-change batch break
// that dominates the profiler output.
func flushSpriteQueueBatched(queue []SpriteDrawCall) {
	if r, ok := gfx.(*Renderer_GLES32); ok {
		r.flushSpriteBatches(queue)
	} else {
		// Defensive: no instanced backend available (should not happen on GLES32 builds).
		for i := range queue {
			if queue[i].isFlat {
				renderFlatCallImmediate(&queue[i])
			} else {
				renderSpriteImmediate(queue[i].rp)
			}
		}
	}
}

func (r *Renderer_GLES32) flushSpriteBatches(queue []SpriteDrawCall) {
	if len(queue) == 0 {
		return
	}
	if r.instancedSpriteShader == nil {
		// Shader failed to compile (batching should already be disabled, but be safe).
		for i := range queue {
			if queue[i].isFlat {
				renderFlatCallImmediate(&queue[i])
			} else {
				renderSpriteImmediate(queue[i].rp)
			}
		}
		return
	}

	// Painter's-algorithm correctness: the engine draws strictly in submission
	// order, so a global sort (the plan's Phase 4 idea) would reorder layers —
	// e.g. flat lifebar rects drawn mid-frame would end up over the characters.
	// Instead we only group *consecutive* calls sharing a batch key. Phase 5
	// still removes the dominant texture-change break by letting one batch span
	// up to maxTexSlots unique textures within a run.

	// One-shot detailed break log: on the first non-empty queue flush that has
	// DrawCallLog enabled, print every batch boundary and what field changed.
	// Set batchBreakLogDone=false in config to re-arm (it resets on restart).
	if sys.cfg.Video.DrawCallLog && !batchBreakLogDone && len(queue) > 50 {
		batchBreakLogDone = true
		for i := 1; i < len(queue); i++ {
			a, b := &queue[i-1], &queue[i]
			if !sameBatchKey(a, b) {
				reasons := ""
				if a.isFlat != b.isFlat {
					reasons += fmt.Sprintf(" isFlat:%v->%v", a.isFlat, b.isFlat)
				}
				if a.blendEq != b.blendEq {
					reasons += fmt.Sprintf(" blendEq:%v->%v", a.blendEq, b.blendEq)
				}
				if a.blendSrc != b.blendSrc {
					reasons += fmt.Sprintf(" blendSrc:%v->%v", a.blendSrc, b.blendSrc)
				}
				if a.blendDst != b.blendDst {
					reasons += fmt.Sprintf(" blendDst:%v->%v", a.blendDst, b.blendDst)
				}
				if a.isRgba != b.isRgba {
					reasons += fmt.Sprintf(" isRgba:%v->%v", a.isRgba, b.isRgba)
				}
				if a.isTrapez != b.isTrapez {
					reasons += fmt.Sprintf(" isTrapez:%v->%v", a.isTrapez, b.isTrapez)
				}
				if a.mask != b.mask {
					reasons += fmt.Sprintf(" mask:%v->%v", a.mask, b.mask)
				}
				if a.hasScissor != b.hasScissor {
					reasons += fmt.Sprintf(" hasScissor:%v->%v", a.hasScissor, b.hasScissor)
				}
				if a.scissor != b.scissor {
					reasons += fmt.Sprintf(" scissor:%v->%v", a.scissor, b.scissor)
				}
				LogMessage("[BATCHBREAK] q[%d->%d]%s", i-1, i, reasons)
			}
		}
	}

	// All batches in this flush use the same instanced pipeline and VAO.
	// Keep them bound across the flush to avoid two VAO binds per batch.
	r.ChangeProgram(r.instancedSpriteShader.program)
	gl.BindVertexArray(r.instanceVAO)
	defer gl.BindVertexArray(0)

	// Static uniforms and texture-unit layout are identical for every batch in
	// this flush; set them once instead of once per batch.
	proj := gfx.OrthographicProjectionMatrix(0, float32(sys.scrrect[2]), 0, float32(sys.scrrect[3]), -65535, 65535)
	if loc := r.instancedSpriteShader.uniforms["projection"]; loc >= 0 {
		gl.UniformMatrix4fv(loc, 1, false, &proj[0])
	}
	texUnits := [maxTexSlots]int32{0, 1, 2, 3, 4, 5, 6}
	palUnits := [maxTexSlots]int32{7, 8, 9, 10, 11, 12, 13}
	if texLoc := r.instancedSpriteShader.uniforms["texArray[0]"]; texLoc >= 0 {
		gl.Uniform1iv(texLoc, maxTexSlots, &texUnits[0])
	} else if texLoc := r.instancedSpriteShader.uniforms["texArray"]; texLoc >= 0 {
		gl.Uniform1iv(texLoc, maxTexSlots, &texUnits[0])
	}
	if palLoc := r.instancedSpriteShader.uniforms["palArray[0]"]; palLoc >= 0 {
		gl.Uniform1iv(palLoc, maxTexSlots, &palUnits[0])
	} else if palLoc := r.instancedSpriteShader.uniforms["palArray"]; palLoc >= 0 {
		gl.Uniform1iv(palLoc, maxTexSlots, &palUnits[0])
	}

	// Forget the per-unit bind cache: units may have been used by other code
	// paths (immediate sprites, post-processing) since the last flush.
	r.boundTexUnits = [2 * maxTexSlots]uint32{}

	numBatches := 0
	start := 0
	for start < len(queue) {
		end := start + 1
		for end < len(queue) {
			if !sameBatchKey(&queue[start], &queue[end]) {
				recordBatchBoundary(&queue[end-1], &queue[end])
				break
			}
			end++
		}
		if queue[start].isFlat {
			// Flat rects share one batch (no textures to budget).
			r.renderBatch(queue[start:end])
			numBatches++
		} else {
			// Split the same-key run when more than maxTexSlots textures appear.
			var runSlots [maxTexSlots]texKey
			slotCount := 0
			batchStart := start
			for i := start; i < end; i++ {
				k := texKey{queue[i].texSerial, queue[i].palSerial}
				j := 0
				for ; j < slotCount; j++ {
					if runSlots[j] == k {
						break
					}
				}
				if j == slotCount {
					if slotCount == maxTexSlots {
						r.renderBatch(queue[batchStart:i])
						numBatches++
						batchSlotSplits++
						batchStart = i
						slotCount = 0
					}
					runSlots[slotCount] = k
					slotCount++
				}
			}
			r.renderBatch(queue[batchStart:end])
			numBatches++
		}
		start = end
	}
	// Accumulate across the frame's flush passes (luaFlushDrawQueue + EndFrame)
	// so the [BATCH] log reports the real total batch count.
	drawCallStats.TotalBatches += numBatches
}

// renderBatch draws a slice of compatible SpriteDrawCalls in a single instanced
// draw call. All calls share the same batch key; they may span multiple texture
// pairs (up to maxTexSlots), each bound to its own texture unit.
func (r *Renderer_GLES32) renderBatch(calls []SpriteDrawCall) {
	if len(calls) == 0 {
		return
	}
	first := &calls[0]

	if first.hasScissor {
		r.EnableScissor(first.scissor[0], first.scissor[1], first.scissor[2], first.scissor[3])
	}

	r.SetUniformISub(r.instancedSpriteShader.uniforms["isFlat"], Btoi(first.isFlat))
	r.SetUniformISub(r.instancedSpriteShader.uniforms["isRgba"], Btoi(first.isRgba))
	r.SetUniformISub(r.instancedSpriteShader.uniforms["isTrapez"], Btoi(first.isTrapez))
	r.SetUniformISub(r.instancedSpriteShader.uniforms["mask"], first.mask)

	r.EnableBlending(first.blendEq, first.blendSrc, first.blendDst)

	var slotArr [maxTexSlots]texSlotInfo
	slotCount := buildSlots(calls, &slotArr)
	if !first.isFlat {
		for i := 0; i < slotCount; i++ {
			info := &slotArr[i]
			if info.tex != nil {
				if t, ok := info.tex.(*Texture_GLES32); ok && r.boundTexUnits[i] != t.handle {
					gl.ActiveTexture(gl.TEXTURE0 + uint32(i))
					gl.BindTexture(gl.TEXTURE_2D, t.handle)
					r.boundTexUnits[i] = t.handle
				}
			}
			if info.pal != nil {
				if p, ok := info.pal.(*Texture_GLES32); ok && r.boundTexUnits[maxTexSlots+i] != p.handle {
					gl.ActiveTexture(gl.TEXTURE0 + uint32(maxTexSlots) + uint32(i))
					gl.BindTexture(gl.TEXTURE_2D, p.handle)
					r.boundTexUnits[maxTexSlots+i] = p.handle
				}
			}
		}
	}

	// Pack per-instance data.
	n := len(calls)
	needed := n * instanceStrideFloats
	if cap(r.instanceScratch) < needed {
		r.instanceScratch = make([]float32, needed)
	}
	buf := r.instanceScratch[:needed]

	for i := range calls {
		dc := &calls[i]
		off := i * instanceStrideFloats
		copy(buf[off:off+8], dc.corners[:])
		copy(buf[off+8:off+12], dc.x1x2x4x3[:])
		copy(buf[off+12:off+16], dc.palUV[:])
		copy(buf[off+16:off+20], dc.uv[:])
		copy(buf[off+20:off+24], dc.tint[:])
		buf[off+24] = 0
		if dc.spfx.neg {
			buf[off+24] = 1
		}
		copy(buf[off+25:off+28], dc.spfx.add[:])
		copy(buf[off+28:off+31], dc.spfx.mult[:])
		buf[off+31] = dc.alpha
		buf[off+32] = dc.gray
		buf[off+33] = dc.spfx.hue
		slot := 0
		if !dc.isFlat {
			k := texKey{dc.texSerial, dc.palSerial}
			for j := 0; j < slotCount; j++ {
				if slotArr[j].key == k {
					slot = j
					break
				}
			}
		}
		buf[off+34] = float32(slot)
		buf[off+35] = float32(slot) // palette shares the sprite slot index
	}

	gl.BindBuffer(gl.ARRAY_BUFFER, r.instanceVBO)
	gl.BufferData(gl.ARRAY_BUFFER, needed*4, unsafe.Pointer(&buf[0]), gl.DYNAMIC_DRAW)

	// Single instanced draw call for the whole batch.
	gl.DrawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, int32(n))

	r.DisableScissor()
}
