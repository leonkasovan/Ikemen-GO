//go:build opengles31
// This is almost identical to render_gles.go except it uses a VAO
// for GLES 3.1 which is the minimum version that runs on modern
// macOS (Intel and ARM). Work adapted from assemblaj/fantasma

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"unsafe"
	"image"
    clr "image/color" // Alias as clr to match your GL33 code
    "image/png"
    "os"

	gl "github.com/ikemen-engine/Ikemen-GO/packages/gl/v3.1/gles2"
	mgl "github.com/go-gl/mathgl/mgl32"
	"golang.org/x/mobile/exp/f32"
)

// ------------------------------------------------------------------
// ShaderProgram_GL

type ShaderProgram_GL struct {
	// Program
	program uint32
	// Attributes
	a map[string]int32
	// Uniforms
	u map[string]int32
	// Texture_GL units
	t map[string]int
}

func (r *Renderer_GL) newShaderProgram(vert, frag, geo, id string, crashWhenFail bool) (s *ShaderProgram_GL, err error) {
	var vertObj, fragObj, geoObj, prog uint32
	if vertObj, err = r.compileShader(gl.VERTEX_SHADER, vert); chkEX(err, "Vertex Shader compilation error on "+id+"\n", crashWhenFail) {
		return nil, err
	}
	if fragObj, err = r.compileShader(gl.FRAGMENT_SHADER, frag); chkEX(err, "Fragment Shader compilation error on "+id+"\n", crashWhenFail) {
		return nil, err
	}
	if len(geo) > 0 {
		if geoObj, err = r.compileShader(gl.GEOMETRY_SHADER_EXT, geo); chkEX(err, "Geo Shader compilation error on "+id+"\n", crashWhenFail) {
			return nil, err
		}
		if prog, err = r.linkProgram(vertObj, fragObj, geoObj); chkEX(err, "Link program error on "+id+"\n", crashWhenFail) {
			return nil, err
		}
	} else {
		if prog, err = r.linkProgram(vertObj, fragObj); chkEX(err, "Link program error on "+id+"\n", crashWhenFail) {
			return nil, err
		}
	}
	s = &ShaderProgram_GL{program: prog}
	s.a = make(map[string]int32)
	s.u = make(map[string]int32)
	s.t = make(map[string]int)
	return s, nil
}
func (r *ShaderProgram_GL) glStr(s string) *uint8 {
	return gl.Str(s + "\x00")
}
func (s *ShaderProgram_GL) RegisterAttributes(names ...string) {
	for _, name := range names {
		s.a[name] = gl.GetAttribLocation(s.program, s.glStr(name))
	}
}

func (s *ShaderProgram_GL) RegisterUniforms(names ...string) {
	for _, name := range names {
		s.u[name] = gl.GetUniformLocation(s.program, s.glStr(name))
	}
}

func (s *ShaderProgram_GL) RegisterTextures(names ...string) {
	for _, name := range names {
		s.u[name] = gl.GetUniformLocation(s.program, s.glStr(name))
		s.t[name] = len(s.t)
	}
}

func (r *Renderer_GL) compileShader(shaderType uint32, src string) (shader uint32, err error) {
	shader = gl.CreateShader(shaderType)
	// fmt.Println(src)
	if (shaderType == gl.GEOMETRY_SHADER_EXT) {
		src = "#version 310 es\n#extension GL_EXT_geometry_shader : require\nprecision mediump float;\n" + src + "\x00"
	} else {
		src = "#version 310 es\nprecision mediump float;\n" + src + "\x00"
	}
	s, _ := gl.Strs(src)
	var l int32 = int32(len(src) - 1)
	gl.ShaderSource(shader, 1, s, &l)
	gl.CompileShader(shader)
	var ok int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &ok)
	if ok == 0 {
		//var err error
		var size, l int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &size)
		if size > 0 {
			str := make([]byte, size+1)
			gl.GetShaderInfoLog(shader, size, &l, &str[0])
			err = Error(str[:l])
		} else {
			err = Error("Unknown shader compile error")
		}
		//chk(err)
		gl.DeleteShader(shader)
		//panic(Error("Shader compile error"))
		return 0, err
	}
	return shader, nil
}

func (r *Renderer_GL) linkProgram(params ...uint32) (program uint32, err error) {
	program = gl.CreateProgram()
	for _, param := range params {
		gl.AttachShader(program, param)
	}
	if len(params) > 2 {
		// Geometry Shader Params
		gl.ProgramParameteri(program, gl.GEOMETRY_LINKED_INPUT_TYPE_EXT, gl.TRIANGLES)
		gl.ProgramParameteri(program, gl.GEOMETRY_LINKED_OUTPUT_TYPE_EXT, gl.TRIANGLE_STRIP)
		gl.ProgramParameteri(program, gl.GEOMETRY_LINKED_VERTICES_OUT_EXT, 3*6)
	}
	gl.LinkProgram(program)
	// Mark shaders for deletion when the program is deleted
	for _, param := range params {
		gl.DeleteShader(param)
	}
	var ok int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &ok)
	if ok == 0 {
		//var err error
		var size, l int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &size)
		if size > 0 {
			str := make([]byte, size+1)
			gl.GetProgramInfoLog(program, size, &l, &str[0])
			err = Error(str[:l])
		} else {
			err = Error("Unknown link error")
		}
		//chk(err)
		gl.DeleteProgram(program)
		//panic(Error("Link error"))
		return 0, err
	}
	return program, nil
}

// ------------------------------------------------------------------
// Texture_GL

type Texture_GL struct {
	width  int32
	height int32
	depth  int32
	filter bool
	handle uint32
}

// Generate a new texture name
func (r *Renderer_GL) newTexture(width, height, depth int32, filter bool) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GL{width, height, depth, filter, h}
	runtime.SetFinalizer(t, func(t *Texture_GL) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	return
}

func (r *Renderer_GL) newPaletteTexture() Texture {
	return r.newTexture(256, 1, 32, false)
}

func (r *Renderer_GL) newModelTexture(width, height, depth int32, filter bool) Texture {
	return r.newTexture(width, height, depth, filter)
}

func (r *Renderer_GL) newDataTexture(width, height int32) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GL{width, height, 32, false, h}
	runtime.SetFinalizer(t, func(t *Texture_GL) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	gl.BindTexture(gl.TEXTURE_2D, h)
	//gl.TexImage2D(gl.TEXTURE_2D, 0, 32, t.width, t.height, 0, 36, gl.FLOAT, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	return
}
func (r *Renderer_GL) newHDRTexture(width, height int32) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GL{width, height, 24, false, h}
	runtime.SetFinalizer(t, func(t *Texture_GL) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	gl.BindTexture(gl.TEXTURE_2D, h)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.MIRRORED_REPEAT)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.MIRRORED_REPEAT)
	return
}
func (r *Renderer_GL) newCubeMapTexture(widthHeight int32, mipmap bool, lowestMipLevel int32) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GL{widthHeight, widthHeight, 24, false, h}
	runtime.SetFinalizer(t, func(t *Texture_GL) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, h)
	for i := 0; i < 6; i++ {
		gl.TexImage2D(uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+i), 0, gl.RGB32F, widthHeight, widthHeight, 0, gl.RGB, gl.FLOAT, nil)
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
	return
}

// Bind a texture and upload texel data to it
func (t *Texture_GL) SetData(data []byte) {
	var interp int32 = gl.NEAREST
	if t.filter {
		interp = gl.LINEAR
	}

	format := t.MapInternalFormat(Max(t.depth, 8))

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	// Ensure any pending GL commands which upload texture data finish before
	// reading with GetTexImage. This avoids reading an incomplete texture on
	// some drivers where uploads are deferred.
	gl.Finish()
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	if data != nil {
		gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	} else {
		gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, nil)
	}

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, interp)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, interp)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
}
func (t *Texture_GL) SetSubData(data []byte, x, y, width, height int32) {
	format := t.MapInternalFormat(Max(t.depth, 8))

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	if data != nil {
		gl.TexSubImage2D(
			gl.TEXTURE_2D,
			0,
			x, y,
			width, height,
			format,
			gl.UNSIGNED_BYTE,
			unsafe.Pointer(&data[0]),
		)
	} else {
		gl.TexSubImage2D(
			gl.TEXTURE_2D,
			0,
			x, y,
			width, height,
			format,
			gl.UNSIGNED_BYTE,
			nil,
		)
	}
}
func (t *Texture_GL) SetDataG(data []byte, mag, min, ws, wt TextureSamplingParam) {

	format := t.MapInternalFormat(Max(t.depth, 8))

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	gl.GenerateMipmap(gl.TEXTURE_2D)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, t.MapTextureSamplingParam(mag))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, t.MapTextureSamplingParam(min))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, t.MapTextureSamplingParam(ws))
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, t.MapTextureSamplingParam(wt))
}
func (t *Texture_GL) SetPixelData(data []float32) {

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, t.width, t.height, 0, gl.RGBA, gl.FLOAT, unsafe.Pointer(&data[0]))
}

func (t *Texture_GL) CopyData(src *Texture) {
	srcTex, ok := (*src).(*Texture_GL)
	if !ok || srcTex == nil || !srcTex.IsValid() || !t.IsValid() {
		return
	}
	if t.width != srcTex.width || t.height != srcTex.height {
		return
	}

	// 1. SAVE THE CURRENT FRAMEBUFFER BINDING
	// If we don't do this, we break the rendering loop by unbinding the main FBO.
	var prevFBO int32
	gl.GetIntegerv(gl.FRAMEBUFFER_BINDING, &prevFBO)

	// Create source FBO
	var srcFBO uint32
	gl.GenFramebuffers(1, &srcFBO)
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, srcFBO)
	gl.FramebufferTexture2D(gl.READ_FRAMEBUFFER, gl.COLOR_ATTACHMENT0,
		gl.TEXTURE_2D, srcTex.handle, 0)

	// Create destination FBO
	var dstFBO uint32
	gl.GenFramebuffers(1, &dstFBO)
	gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, dstFBO)
	gl.FramebufferTexture2D(gl.DRAW_FRAMEBUFFER, gl.COLOR_ATTACHMENT0,
		gl.TEXTURE_2D, t.handle, 0)

	// Check status before blitting
	if gl.CheckFramebufferStatus(gl.READ_FRAMEBUFFER) == gl.FRAMEBUFFER_COMPLETE &&
		gl.CheckFramebufferStatus(gl.DRAW_FRAMEBUFFER) == gl.FRAMEBUFFER_COMPLETE {
		
		gl.BlitFramebuffer(0, 0, t.width, t.height,
			0, 0, t.width, t.height,
			gl.COLOR_BUFFER_BIT, gl.NEAREST)
	}

	// Cleanup Temp FBOs
	gl.DeleteFramebuffers(1, &srcFBO)
	gl.DeleteFramebuffers(1, &dstFBO)

	// 2. RESTORE THE PREVIOUS FRAMEBUFFER
	gl.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
}

// Return whether texture has a valid handle
func (t *Texture_GL) IsValid() bool {
	return t.width != 0 && t.height != 0 && t.handle != 0
}

func (t *Texture_GL) GetWidth() int32 {
	return t.width
}

func (t *Texture_GL) GetHeight() int32 {
	return t.height
}

func (t *Texture_GL) MapInternalFormat(i int32) uint32 {
	var InternalFormatLUT = map[int32]uint32{
		8:   gl.RED,        // single-channel (Red)
		24:  gl.RGB,      // 8 bits per RGB channel
		32:  gl.RGBA,     // 8 bits per RGBA channel
		96:  gl.RGB32F,    // 32-bit float per RGB channel
		128: gl.RGBA32F,   // 32-bit float per RGBA channel
	}

	return InternalFormatLUT[i]
}

func (t *Texture_GL) MapTextureSamplingParam(i TextureSamplingParam) int32 {
	var SamplingParam = map[TextureSamplingParam]int32{
		TextureSamplingFilterNearest:              gl.NEAREST,
		TextureSamplingFilterLinear:               gl.LINEAR,
		TextureSamplingFilterNearestMipMapNearest: gl.NEAREST_MIPMAP_NEAREST,
		TextureSamplingFilterLinearMipMapNearest:  gl.LINEAR_MIPMAP_NEAREST,
		TextureSamplingFilterNearestMipMapLinear:  gl.NEAREST_MIPMAP_LINEAR,
		TextureSamplingFilterLinearMipMapLinear:   gl.LINEAR_MIPMAP_LINEAR,
		TextureSamplingWrapClampToEdge:            gl.CLAMP_TO_EDGE,
		TextureSamplingWrapMirroredRepeat:         gl.MIRRORED_REPEAT,
		TextureSamplingWrapRepeat:                 gl.REPEAT,
	}

	return SamplingParam[i]
}
func (t *Texture_GL) SavePNG(filename string, pal []uint32) error {
	fmt.Printf("SavePNG: filename=%q width=%d height=%d depth=%d handle=%d\n",
		filename, t.width, t.height, t.depth, t.handle)

	if !t.IsValid() {
		fmt.Printf("SavePNG: texture is not valid (handle=%d, w=%d, h=%d)\n", t.handle, t.width, t.height)
		return Error("texture not valid")
	}

	// --- 1. Read Texture Data from GPU (GLES Workaround) ---

	// Save the current framebuffer so we don't break the render loop
	var prevFBO int32
	gl.GetIntegerv(gl.FRAMEBUFFER_BINDING, &prevFBO)

	// Create a temporary FBO to read the texture
	var fbo uint32
	gl.GenFramebuffers(1, &fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, t.handle, 0)

	// Check FBO status
	status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER)
	if status != gl.FRAMEBUFFER_COMPLETE {
		gl.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
		gl.DeleteFramebuffers(1, &fbo)
		return Error(fmt.Sprintf("SavePNG: temp framebuffer incomplete: 0x%x", status))
	}

	// Read pixels as RGBA (safest for GLES compatibility)
	// We read 4 bytes per pixel even if depth is 8.
	rawSize := int(t.width) * int(t.height) * 4
	rawData := make([]byte, rawSize)
	gl.ReadPixels(0, 0, t.width, t.height, gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&rawData[0]))

	// Restore state and cleanup
	gl.BindFramebuffer(gl.FRAMEBUFFER, uint32(prevFBO))
	gl.DeleteFramebuffers(1, &fbo)

	// --- 2. Process Data ---

	var data []byte

	if t.depth == 8 {
		// EXTRACT INDICES:
		// If the texture is 8-bit (indexed), the index is in the Red channel.
		// Since we read RGBA, we take every 4th byte.
		count := int(t.width) * int(t.height)
		data = make([]byte, count)
		for i := 0; i < count; i++ {
			data[i] = rawData[i*4] // Extract R component as index
		}
	} else {
		// Use raw RGBA data
		data = rawData
	}

	// --- 3. Palette Handling (Ported from render_gl33.go) ---

	if t.depth == 8 {
		// 8-bit texture: data contains palette indices.
		if pal != nil && len(pal) >= 256 {
			// Build Go palette from uint32 entries (A<<24 | B<<16 | G<<8 | R)
			gpal := make(clr.Palette, 256)
			for i := 0; i < 256; i++ {
				c := pal[i]
				r := uint8(c & 0xff)
				g := uint8((c >> 8) & 0xff)
				b := uint8((c >> 16) & 0xff)
				a := uint8((c >> 24) & 0xff)
				gpal[i] = clr.RGBA{R: r, G: g, B: b, A: a}
			}

			img := image.NewPaletted(image.Rect(0, 0, int(t.width), int(t.height)), gpal)
			n := copy(img.Pix, data)
			if n != len(img.Pix) {
				fmt.Printf("SavePNG: copied %d bytes into paletted image (expected %d)\n", n, len(img.Pix))
			}

			// Fix invisible palette entries if they are used
			usedCounts := make([]int, 256)
			for _, idx := range img.Pix {
				usedCounts[int(idx)]++
			}
			forced := []int{}
			for i := 0; i < 256; i++ {
				if usedCounts[i] > 0 {
					_, _, _, ca := gpal[i].RGBA()
					if ca>>8 == 0 { // alpha == 0
						r, g, b, _ := gpal[i].RGBA()
						gpal[i] = clr.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
						forced = append(forced, i)
					}
				}
			}
			if len(forced) > 0 {
				fmt.Printf("SavePNG: forced %d palette entries to opaque\n", len(forced))
			}

			f, err := os.Create(filename)
			if err != nil {
				return err
			}
			defer f.Close()

			if err := png.Encode(f, img); err != nil {
				fmt.Printf("SavePNG: png.Encode returned error: %v\n", err)
				return err
			}
			return nil
		}

		// Fallback: expand indices into an opaque grayscale NRGBA
		grayImg := image.NewNRGBA(image.Rect(0, 0, int(t.width), int(t.height)))
		for i := 0; i < len(data) && 4*i+3 < len(grayImg.Pix); i++ {
			v := data[i]
			grayImg.Pix[4*i+0] = v
			grayImg.Pix[4*i+1] = v
			grayImg.Pix[4*i+2] = v
			grayImg.Pix[4*i+3] = 255
		}
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()

		if err := png.Encode(f, grayImg); err != nil {
			return err
		}
		return nil
	}

	// --- 4. Standard RGBA Saving ---

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	
	// Create NRGBA image
	normalImg := image.NewNRGBA(image.Rect(0, 0, int(t.width), int(t.height)))
	// Note: glReadPixels returns data from bottom-left. Standard PNG is top-left.
	// Typically you might need to flip rows here, but we'll copy directly to match current behavior.
	copy(normalImg.Pix, data)

	if err := png.Encode(f, normalImg); err != nil {
		fmt.Printf("SavePNG: png.Encode(rgba) returned error: %v\n", err)
		return err
	}

	return nil
}

// ------------------------------------------------------------------
// Renderer_GL

type Renderer_GL struct {
	fbo         uint32
	fbo_texture uint32
	// Normal rendering
	rbo_depth uint32
	// MSAA rendering
	fbo_f         uint32
	fbo_f_texture *Texture_GL
	// Shadow Map
	fbo_shadow              uint32
	fbo_shadow_cube_texture uint32
	fbo_env                 uint32
	// Postprocessing FBOs
	fbo_pp         []uint32
	fbo_pp_texture []uint32
	// Post-processing shaders
	postVertBuffer   uint32
	postShaderSelect []*ShaderProgram_GL
	// Shader and vertex data for primitive rendering
	spriteShader      *ShaderProgram_GL
	vertexBuffer      uint32
	vertexBufferBatch uint32
	// Shader and index data for 3D model rendering
	shadowMapShader         *ShaderProgram_GL
	modelShader             *ShaderProgram_GL
	panoramaToCubeMapShader *ShaderProgram_GL
	cubemapFilteringShader  *ShaderProgram_GL
	modelVertexBuffer       [2]uint32
	modelIndexBuffer        [2]uint32
	vao                     uint32

	enableModel  bool
	enableShadow bool
	GLState
}
type GLState struct {
	depthTest           bool
	depthMask           bool
	invertFrontFace     bool
	doubleSided         bool
	blendEquation       BlendEquation
	blendSrc            BlendFunc
	blendDst            BlendFunc
	useUV               bool
	useNormal           bool
	useTangent          bool
	useVertColor        bool
	useJoint0           bool
	useJoint1           bool
	useOutlineAttribute bool
}

func (r *Renderer_GL) GetName() string {
	return "OpenGL ES 3.1"
}

// init 3D model shader
func (r *Renderer_GL) InitModelShader() error {
	var err error
	if r.enableShadow {
		r.modelShader, err = r.newShaderProgram(modelVertShader, "#define ENABLE_SHADOW\n"+modelFragShader, "", "Model Shader", true)
	} else {
		r.modelShader, err = r.newShaderProgram(modelVertShader, modelFragShader, "", "Model Shader", true)
	}
	if err != nil {
		return err
	}
	r.modelShader.RegisterAttributes("vertexId", "position", "uv", "normalIn", "tangentIn", "vertColor", "joints_0", "joints_1", "weights_0", "weights_1", "outlineAttributeIn")
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
	r.modelShader.RegisterTextures("tex", "morphTargetValues", "jointMatrices", "normalMap", "metallicRoughnessMap", "ambientOcclusionMap", "emissionMap", "lambertianEnvSampler", "GGXEnvSampler", "GGXLUT",
		"shadowCubeMap")

	if r.enableShadow {
		r.shadowMapShader, err = r.newShaderProgram(shadowVertShader, shadowFragShader, "", "Shadow Map Shader", false)
		if err != nil {
			return err
		}
		r.shadowMapShader.RegisterAttributes("vertexId", "position", "vertColor", "uv", "joints_0", "joints_1", "weights_0", "weights_1")
		r.shadowMapShader.RegisterUniforms("model", "lightMatrices[0]", "lightMatrices[1]", "lightMatrices[2]", "lightMatrices[3]", "lightMatrices[4]", "lightMatrices[5]",
			"lightMatrices[6]", "lightMatrices[7]", "lightMatrices[8]", "lightMatrices[9]", "lightMatrices[10]", "lightMatrices[11]",
			"lightMatrices[12]", "lightMatrices[13]", "lightMatrices[14]", "lightMatrices[15]", "lightMatrices[16]", "lightMatrices[17]",
			"lightMatrices[18]", "lightMatrices[19]", "lightMatrices[20]", "lightMatrices[21]", "lightMatrices[22]", "lightMatrices[23]",
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
	return nil
}

// Render initialization.
// Creates the default shaders, the framebuffer and enables MSAA.
func (r *Renderer_GL) Init() {
	if err := gl.Init(sys.GetProcAddress()); err != nil {
    	fmt.Println("gl.Init() failed:", err)
	} else {
		fmt.Println("gl.Init() success:")
	}
	sys.errLog.Printf("Using %v (%v)",
		gl.GoStr(gl.GetString(gl.VERSION)), gl.GoStr(gl.GetString(gl.RENDERER)))

	// Query max samples and clamp requested msaa
	var maxSamples int32
	gl.GetIntegerv(gl.MAX_SAMPLES, &maxSamples)
	if sys.msaa > maxSamples {
		sys.cfg.SetValueUpdate("Video.MSAA", maxSamples)
		sys.msaa = maxSamples
	}

	// Store current timestamp
	sys.prevTimestamp = sys.GetTime()

	// Prepare post shader slots (1 base + external)
	r.postShaderSelect = make([]*ShaderProgram_GL, 1+len(sys.cfg.Video.ExternalShaders))

	// Data buffers for rendering (post-process quad)
	postVertData := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)

	r.enableModel = sys.cfg.Video.EnableModel
	r.enableShadow = sys.cfg.Video.EnableModelShadow

	// Vertex array / buffers (VAO exists in GLES3)
	gl.GenVertexArrays(1, &r.vao)
	gl.BindVertexArray(r.vao)

	gl.GenBuffers(1, &r.postVertBuffer)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(postVertData), unsafe.Pointer(&postVertData[0]), gl.STATIC_DRAW)

	gl.GenBuffers(1, &r.vertexBuffer)
	gl.GenBuffers(2, &r.modelVertexBuffer[0])
	gl.GenBuffers(2, &r.modelIndexBuffer[0])
	gl.GenBuffers(1, &r.vertexBufferBatch)

	// Sprite shader
	r.spriteShader, _ = r.newShaderProgram(vertShader, fragShader, "", "Main Shader", true)
	r.spriteShader.RegisterAttributes("position", "uv")
	r.spriteShader.RegisterUniforms("modelview", "projection", "x1x2x4x3",
		"alpha", "tint", "mask", "neg", "gray", "add", "mult", "isFlat", "isRgba", "isTrapez", "hue", "uvRect", "useUV")
	r.spriteShader.RegisterTextures("pal", "tex")

	if r.enableModel {
		if err := r.InitModelShader(); err != nil {
			// If model shader initialization fails, disable models quietly
			sys.errLog.Printf("InitModelShader failed, disabling model support: %v", err)
			r.enableModel = false
		}
	}

	// Compile postprocessing shaders
	// Recreate selection array to exact size
	r.postShaderSelect = make([]*ShaderProgram_GL, 1+len(sys.cfg.Video.ExternalShaders))

	// External Shaders
	for i := 0; i < len(sys.cfg.Video.ExternalShaders); i++ {
		r.postShaderSelect[i], _ = r.newShaderProgram(
			string(sys.externalShaders[0][i])+"\x00",
			string(sys.externalShaders[1][i])+"\x00",
			"", fmt.Sprintf("Postprocess Shader #%v", i), true)
		r.postShaderSelect[i].RegisterAttributes("VertCoord", "TexCoord")
		loc := r.postShaderSelect[i].a["TexCoord"]
		// attribute: location, size=3, type=float, normalized=false, stride=5*4, offset=2*4
		gl.VertexAttribPointer(uint32(loc), 3, gl.FLOAT, false, 5*4, gl.PtrOffset(2*4))
		gl.EnableVertexAttribArray(uint32(loc))
		r.postShaderSelect[i].RegisterUniforms("Texture_GL", "TextureSize", "CurrentTime")
	}

	// Identity postprocess shader (last slot)
	identShader, _ := r.newShaderProgram(identVertShader, identFragShader, "", "Identity Postprocess", true)
	identShader.RegisterAttributes("VertCoord", "TexCoord")
	identShader.RegisterUniforms("Texture_GL", "TextureSize", "CurrentTime")
	r.postShaderSelect[len(r.postShaderSelect)-1] = identShader

	// Enable MSAA if requested
	if sys.msaa > 0 {
		// OpenGL ES 3.1 doesn’t support this
	}

	gl.ActiveTexture(gl.TEXTURE0)

	// create a texture for r.fbo
	gl.GenTextures(1, &r.fbo_texture)

	// Bind multisample texture or regular 2D texture
	if sys.msaa > 0 {
		// OpenGL ES 3.1 doesn’t support this
	} else {
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_texture)

		// Set filtering/wrap only for non-multisample textures
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	}

	// Use sized internal formats in GLES3.1
	if sys.msaa > 0 {
		// OpenGL ES 3.1 doesn’t support this
	} else {
		// Normal 2D texture: use RGBA8 + UNSIGNED_BYTE to match original semantics
		gl.TexImage2D(
			gl.TEXTURE_2D,
			0,
			gl.RGBA8, // sized internal format on GLES
			sys.scrrect[2],
			sys.scrrect[3],
			0,
			gl.RGBA,
			gl.UNSIGNED_BYTE,
			nil,
		)
	}

	// Prepare postprocessing textures (we need signed representation for shader math).
	// GLES 3.1 does NOT guarantee RGBA8_SNORM is core. Use RGBA32F to safely represent negative values.
	r.fbo_pp = make([]uint32, 2)
	r.fbo_pp_texture = make([]uint32, 2)

	for i := 0; i < 2; i++ {
		gl.GenTextures(1, &(r.fbo_pp_texture[i]))
		gl.BindTexture(gl.TEXTURE_2D, r.fbo_pp_texture[i])
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

		// Use float format so shaders can safely store negative values (original used SNORM).
		// RGBA32F + FLOAT is universally available in GLES3.1 (but heavier on memory).
		gl.TexImage2D(
			gl.TEXTURE_2D,
			0,
			gl.RGBA32F, // internal format
			sys.scrrect[2],
			sys.scrrect[3],
			0,
			gl.RGBA,
			gl.FLOAT,
			nil,
		)
	}

	// done with r.fbo_texture, unbind it
	gl.BindTexture(gl.TEXTURE_2D, 0)

	// Depth renderbuffer
	gl.GenRenderbuffers(1, &r.rbo_depth)

	gl.BindRenderbuffer(gl.RENDERBUFFER, r.rbo_depth)
	if sys.msaa > 0 {
		// multisample depth renderbuffer storage (supported in GLES3)
		gl.RenderbufferStorageMultisample(gl.RENDERBUFFER, sys.msaa, gl.DEPTH_COMPONENT16, sys.scrrect[2], sys.scrrect[3])
	} else {
		gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, sys.scrrect[2], sys.scrrect[3])
	}
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)

	// If MSAA path is used we create an intermediate texture to blit to
	if sys.msaa > 0 {
		r.fbo_f_texture = r.newTexture(sys.scrrect[2], sys.scrrect[3], 32, false).(*Texture_GL)
		r.fbo_f_texture.SetData(nil)
	}

	// create an FBO for our r.fbo, which is then for r.fbo_texture
	gl.GenFramebuffers(1, &r.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)

	if sys.msaa > 0 {
		// Attach the multisampled texture and the multisampled depth RBO
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D_MULTISAMPLE, r.fbo_texture, 0)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)
		if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
			sys.errLog.Printf("framebuffer create failed: 0x%x", status)
			fmt.Printf("framebuffer create failed: 0x%x \n", status)
		}

		// Create additional framebuffer for resolved (single-sample) texture
		gl.GenFramebuffers(1, &r.fbo_f)
		gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_f)
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_f_texture.handle, 0)
	} else {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_texture, 0)
		gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)
	}

	// create our two FBOs for our postprocessing needs
	for i := 0; i < 2; i++ {
		gl.GenFramebuffers(1, &(r.fbo_pp[i]))
		gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[i])
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_pp_texture[i], 0)
	}

	// Model-related FBOs (shadow / env)
	if r.enableModel {
		if r.enableShadow {
			// GLES3.1 does not guarantee support for texture cube map arrays (TEXTURE_CUBE_MAP_ARRAY).
			// If your target supports GL_EXT_texture_cube_map_array or GLES3.2, you can implement cube array here.
			// For portability, disable shadow cube-array setup when running on core ES3.1.
			sys.errLog.Printf("cube map array framebuffer for shadows is not created: GLES3.1 core lacks TEXTURE_CUBE_MAP_ARRAY. Shadow disabled.")
			r.enableShadow = false
		}

		// env framebuffer (still create)
		gl.GenFramebuffers(1, &r.fbo_env)
	}

	// Unbind framebuffer
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

func (r *Renderer_GL) Close() {
}

func (r *Renderer_GL) IsModelEnabled() bool {
	return r.enableModel
}

func (r *Renderer_GL) IsShadowEnabled() bool {
	return r.enableShadow
}

func (r *Renderer_GL) BeginFrame(clearColor bool) {
	sys.absTickCountF++
	gl.BindVertexArray(r.vao)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	if clearColor {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	} else {
		gl.Clear(gl.DEPTH_BUFFER_BIT)
	}
}

func (r *Renderer_GL) BlendReset() {
	gl.BlendEquation(r.MapBlendEquation(BlendAdd))
	gl.BlendFunc(r.MapBlendFunction(BlendSrcAlpha), r.MapBlendFunction(BlendOneMinusSrcAlpha))
}
func (r *Renderer_GL) EndFrame() {
	if len(r.fbo_pp) == 0 {
		return
	}
	// tell GL to use our vertex array object
	// this'll be where our quad is stored
	gl.BindVertexArray(r.vao)

	x, y, width, height := int32(0), int32(0), int32(sys.scrrect[2]), int32(sys.scrrect[3])
	time := sys.GetTime() // consistent time across all shaders

	if sys.msaa > 0 {
		gl.BindFramebuffer(gl.DRAW_FRAMEBUFFER, r.fbo_f)
		gl.BindFramebuffer(gl.READ_FRAMEBUFFER, r.fbo)
		gl.BlitFramebuffer(x, y, width, height, x, y, width, height, gl.COLOR_BUFFER_BIT, gl.LINEAR)
	}

	var scaleMode int32 // GL enum
	if sys.cfg.Video.WindowScaleMode {
		scaleMode = gl.LINEAR
	} else {
		scaleMode = gl.NEAREST
	}

	// set the viewport to the unscaled bounds for post-processing
	gl.Viewport(x, y, width, height)
	// clear both of our post-processing FBOs to make sure
	// nothing's there. the output is set later
	for i := 0; i < 2; i++ {
		gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[i])
		gl.Clear(gl.COLOR_BUFFER_BIT)
	}
	gl.ActiveTexture(gl.TEXTURE0) // later referred to by Texture_GL

	fbo_texture := r.fbo_texture
	if sys.msaa > 0 {
		fbo_texture = r.fbo_f_texture.handle
	}

	// disable blending
	gl.Disable(gl.BLEND)

	for i := 0; i < len(r.postShaderSelect); i++ {
		postShader := r.postShaderSelect[i]

		// this is here because it is undefined
		// behavior to write to the same FBO
		if i%2 == 0 {
			// ping! our first post-processing FBO is the output
			gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[0])
			if i == 0 {
				// first pass, use fbo_texture
				gl.BindTexture(gl.TEXTURE_2D, fbo_texture)
			} else {
				// not the first pass, use the second post-processing FBO
				gl.BindTexture(gl.TEXTURE_2D, r.fbo_pp_texture[1])
			}
		} else {
			// pong! our second post-processing FBO is the output
			gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[1])
			// our first post-processing FBO is the input
			gl.BindTexture(gl.TEXTURE_2D, r.fbo_pp_texture[0])
		}

		if i >= len(r.postShaderSelect)-1 {
			// this is the last shader,
			// so we ask GL to scale it and output it
			// to FB0, the default frame buffer that the user sees
			x, y, width, height := sys.window.GetScaledViewportSize()
			gl.Viewport(x, y, width, height)
			gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
			// clear FB0 just to make sure
			gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
		}

		// tell GL we want to use our shader program
		gl.UseProgram(postShader.program)

		// set post-processing parameters
		gl.Uniform1i(postShader.u["Texture_GL"], 0)
		gl.Uniform2f(postShader.u["TextureSize"], float32(width), float32(height))
		gl.Uniform1f(postShader.u["CurrentTime"], float32(time))
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, scaleMode)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, scaleMode)

		// this actually draws the image to the FBO
		// by constructing a quad (2 tris)
		gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)

		// construct the UVs of the quad
		loc := postShader.a["VertCoord"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointer(uint32(loc), 2, gl.FLOAT, false, 0, nil)

		// construct the quad and draw it
		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
		sys.nDrawcall++
		gl.DisableVertexAttribArray(uint32(loc))
	}
	sys.Drawcall = sys.nDrawcall
	sys.nDrawcall = 0
}

func (r *Renderer_GL) Await() {
	gl.Finish()
}

func (r *Renderer_GL) MapBlendEquation(i BlendEquation) uint32 {
	var BlendEquationLUT = map[BlendEquation]uint32{
		BlendAdd:             gl.FUNC_ADD,
		BlendReverseSubtract: gl.FUNC_REVERSE_SUBTRACT,
	}
	return BlendEquationLUT[i]
}

func (r *Renderer_GL) MapBlendFunction(i BlendFunc) uint32 {
	var BlendFunctionLUT = map[BlendFunc]uint32{
		BlendOne:              gl.ONE,
		BlendZero:             gl.ZERO,
		BlendSrcAlpha:         gl.SRC_ALPHA,
		BlendOneMinusSrcAlpha: gl.ONE_MINUS_SRC_ALPHA,
		BlendOneMinusDstColor: gl.ONE_MINUS_DST_COLOR,
		BlendDstColor:         gl.DST_COLOR,
	}
	return BlendFunctionLUT[i]
}

func (r *Renderer_GL) MapPrimitiveMode(i PrimitiveMode) uint32 {
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

func (r *Renderer_GL) SetDepthTest(depthTest bool) {
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

func (r *Renderer_GL) SetDepthMask(depthMask bool) {
	if depthMask != r.depthMask {
		r.depthMask = depthMask
		gl.DepthMask(depthMask)
	}
}

func (r *Renderer_GL) SetFrontFace(invertFrontFace bool) {
	if invertFrontFace != r.invertFrontFace {
		r.invertFrontFace = invertFrontFace
		if invertFrontFace {
			gl.FrontFace(gl.CW)
		} else {
			gl.FrontFace(gl.CCW)
		}
	}
}
func (r *Renderer_GL) SetCullFace(doubleSided bool) {
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
func (r *Renderer_GL) SetBlending(eq BlendEquation, src, dst BlendFunc) {
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

func (r *Renderer_GL) SetPipeline(eq BlendEquation, src, dst BlendFunc) {
	gl.BindVertexArray(r.vao)
	gl.UseProgram(r.spriteShader.program)

	gl.BlendEquation(r.MapBlendEquation(eq))
	gl.BlendFunc(r.MapBlendFunction(src), r.MapBlendFunction(dst))
	gl.Enable(gl.BLEND)

	// Must bind buffer before enabling attributes
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	loc := r.spriteShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 16, 0)
	loc = r.spriteShader.a["uv"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 16, 8)
}

func (r *Renderer_GL) SetPipelineBatch() {
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBufferBatch)
	loc := r.spriteShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 16, 0)
	loc = r.spriteShader.a["uv"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 16, 8)
}

func (r *Renderer_GL) ReleasePipeline() {
	loc := r.spriteShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.spriteShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.Disable(gl.BLEND)
}

func (r *Renderer_GL) prepareShadowMapPipeline(bufferIndex uint32) {
	// OpenGL ES 3.1 doesn’t support this
}
func (r *Renderer_GL) setShadowMapPipeline(doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	r.SetFrontFace(invertFrontFace)
	r.SetCullFace(doubleSided)

	loc := r.shadowMapShader.a["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.shadowMapShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices
	if useUV {
		r.useUV = true
		loc = r.modelShader.a["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else if r.useUV {
		r.useUV = false
		loc = r.modelShader.a["uv"]
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
		loc = r.shadowMapShader.a["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.shadowMapShader.a["vertColor"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		r.useJoint0 = true
		loc = r.modelShader.a["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.modelShader.a["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			r.useJoint1 = true
			loc = r.modelShader.a["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.modelShader.a["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else if r.useJoint1 {
			r.useJoint1 = false
			loc = r.modelShader.a["joints_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.modelShader.a["weights_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else if r.useJoint0 {
		r.useJoint0 = false
		r.useJoint1 = false
		loc = r.modelShader.a["joints_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["joints_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}

func (r *Renderer_GL) ReleaseShadowPipeline() {
	loc := r.modelShader.a["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib2f(uint32(loc), 0, 0)
	loc = r.modelShader.a["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	loc = r.modelShader.a["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	//gl.Disable(gl.TEXTURE_2D)
	gl.DepthMask(true)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.BLEND)
	r.useUV = false
	r.useJoint0 = false
	r.useJoint1 = false
}
func (r *Renderer_GL) prepareModelPipeline(bufferIndex uint32, env *Environment) {
	gl.UseProgram(r.modelShader.program)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.TEXTURE_CUBE_MAP)
	gl.Enable(gl.BLEND)

	if r.depthTest {
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthFunc(gl.LESS)
	} else {
		gl.Disable(gl.DEPTH_TEST)
	}
	gl.DepthMask(r.depthMask)
	if r.invertFrontFace {
		gl.FrontFace(gl.CW)
	} else {
		gl.FrontFace(gl.CCW)
	}
	if !r.doubleSided {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	} else {
		gl.Disable(gl.CULL_FACE)
	}
	gl.BlendEquation(r.MapBlendEquation(r.blendEquation))
	gl.BlendFunc(r.MapBlendFunction(r.blendSrc), r.MapBlendFunction(r.blendDst))

	gl.BindBuffer(gl.ARRAY_BUFFER, r.modelVertexBuffer[bufferIndex])
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.modelIndexBuffer[bufferIndex])
	if r.enableShadow {
		// OpenGL ES 3.1 doesn’t support this
	}
	if env != nil {
		loc, unit := r.modelShader.u["lambertianEnvSampler"], r.modelShader.t["lambertianEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, env.lambertianTexture.tex.(*Texture_GL).handle)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["GGXEnvSampler"], r.modelShader.t["GGXEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, env.GGXTexture.tex.(*Texture_GL).handle)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["GGXLUT"], r.modelShader.t["GGXLUT"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_2D, env.GGXLUT.tex.(*Texture_GL).handle)
		gl.Uniform1i(loc, int32(unit))

		loc = r.modelShader.u["environmentIntensity"]
		gl.Uniform1f(loc, env.environmentIntensity)
		loc = r.modelShader.u["mipCount"]
		gl.Uniform1i(loc, env.mipmapLevels)
		loc = r.modelShader.u["environmentRotation"]
		rotationMatrix := mgl.Rotate3DX(math.Pi).Mul3(mgl.Rotate3DY(0.5 * math.Pi))
		rotationM := rotationMatrix[:]
		gl.UniformMatrix3fv(loc, 1, false, &rotationM[0])

	} else {
		loc, unit := r.modelShader.u["lambertianEnvSampler"], r.modelShader.t["lambertianEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, 0)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["GGXEnvSampler"], r.modelShader.t["GGXEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, 0)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["GGXLUT"], r.modelShader.t["GGXLUT"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_2D, 0)
		gl.Uniform1i(loc, int32(unit))
		loc = r.modelShader.u["environmentIntensity"]
		gl.Uniform1f(loc, 0)
	}
}
func (r *Renderer_GL) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1, useOutlineAttribute bool, numVertices, vertAttrOffset uint32) {
	r.SetDepthTest(depthTest)
	r.SetDepthMask(depthMask)
	r.SetFrontFace(invertFrontFace)
	r.SetCullFace(doubleSided)
	r.SetBlending(eq, src, dst)

	loc := r.modelShader.a["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.modelShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices
	if useUV {
		r.useUV = true
		loc = r.modelShader.a["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else if r.useUV {
		r.useUV = false
		loc = r.modelShader.a["uv"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib2f(uint32(loc), 0, 0)
	}
	if useNormal {
		r.useNormal = true
		loc = r.modelShader.a["normalIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
		offset += 12 * numVertices
	} else if r.useNormal {
		r.useNormal = false
		loc = r.modelShader.a["normalIn"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib3f(uint32(loc), 0, 0, 0)
	}
	if useTangent {
		r.useTangent = true
		loc = r.modelShader.a["tangentIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else if r.useTangent {
		r.useTangent = false
		loc = r.modelShader.a["tangentIn"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
	if useVertColor {
		r.useVertColor = true
		loc = r.modelShader.a["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else if r.useVertColor {
		r.useVertColor = false
		loc = r.modelShader.a["vertColor"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		r.useJoint0 = true
		loc = r.modelShader.a["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.modelShader.a["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			r.useJoint1 = true
			loc = r.modelShader.a["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.modelShader.a["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else if r.useJoint1 {
			r.useJoint1 = false
			loc = r.modelShader.a["joints_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.modelShader.a["weights_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else if r.useJoint0 {
		r.useJoint0 = false
		r.useJoint1 = false
		loc = r.modelShader.a["joints_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["joints_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
	if useOutlineAttribute {
		r.useOutlineAttribute = true
		loc = r.modelShader.a["outlineAttributeIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else if r.useOutlineAttribute {
		r.useOutlineAttribute = false
		loc = r.modelShader.a["outlineAttributeIn"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}
func (r *Renderer_GL) SetMeshOulinePipeline(invertFrontFace bool, meshOutline float32) {
	r.SetFrontFace(invertFrontFace)
	r.SetDepthTest(true)
	r.SetDepthMask(true)

	loc := r.modelShader.u["meshOutline"]
	gl.Uniform1f(loc, meshOutline)
}
func (r *Renderer_GL) ReleaseModelPipeline() {
	loc := r.modelShader.a["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib2f(uint32(loc), 0, 0)
	loc = r.modelShader.a["normalIn"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib3f(uint32(loc), 0, 0, 0)
	loc = r.modelShader.a["tangentIn"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	loc = r.modelShader.a["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	loc = r.modelShader.a["outlineAttributeIn"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	//gl.Disable(gl.TEXTURE_2D)
	gl.DepthMask(true)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	r.useUV = false
	r.useNormal = false
	r.useTangent = false
	r.useVertColor = false
	r.useJoint0 = false
	r.useJoint1 = false
	r.useOutlineAttribute = false
}

func (r *Renderer_GL) ReadPixels(data []uint8, width, height int) {
	// we defer the EndFrame(), SwapBuffers(), and BeginFrame() calls that were previously below now to
	// a single spot in order to prevent the blank screenshot bug on single digit FPS
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
}

func (r *Renderer_GL) Scissor(x, y, width, height int32) {
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(x, sys.scrrect[3]-(y+height), width, height)
}

func (r *Renderer_GL) DisableScissor() {
	gl.Disable(gl.SCISSOR_TEST)
}

func (r *Renderer_GL) SetUniformI(name string, val int) {
	loc := r.spriteShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer_GL) SetUniformF(name string, values ...float32) {
	loc := r.spriteShader.u[name]
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

func (r *Renderer_GL) SetUniformFv(name string, values []float32) {
	loc := r.spriteShader.u[name]
	switch len(values) {
	case 2:
		gl.Uniform2fv(loc, 1, &values[0])
	case 3:
		gl.Uniform3fv(loc, 1, &values[0])
	case 4:
		gl.Uniform4fv(loc, 1, &values[0])
	}
}

func (r *Renderer_GL) SetUniformMatrix(name string, value []float32) {
	loc := r.spriteShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GL) SetTexture(name string, tex Texture) {
	t := tex.(*Texture_GL)
	loc, unit := r.spriteShader.u[name], r.spriteShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer_GL) SetModelUniformI(name string, val int) {
	loc := r.modelShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer_GL) SetModelUniformF(name string, values ...float32) {
	loc := r.modelShader.u[name]
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
func (r *Renderer_GL) SetModelUniformFv(name string, values []float32) {
	loc := r.modelShader.u[name]
	switch len(values) {
	case 2:
		gl.Uniform2fv(loc, 1, &values[0])
	case 3:
		gl.Uniform3fv(loc, 1, &values[0])
	case 4:
		gl.Uniform4fv(loc, 1, &values[0])
	case 8:
		gl.Uniform4fv(loc, 2, &values[0])
	}
}
func (r *Renderer_GL) SetModelUniformMatrix(name string, value []float32) {
	loc := r.modelShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GL) SetModelUniformMatrix3(name string, value []float32) {
	loc := r.modelShader.u[name]
	gl.UniformMatrix3fv(loc, 1, false, &value[0])
}

func (r *Renderer_GL) SetModelTexture(name string, tex Texture) {
	t := tex.(*Texture_GL)
	loc, unit := r.modelShader.u[name], r.modelShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer_GL) SetShadowMapUniformI(name string, val int) {
	loc := r.shadowMapShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer_GL) SetShadowMapUniformF(name string, values ...float32) {
	loc := r.shadowMapShader.u[name]
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
func (r *Renderer_GL) SetShadowMapUniformFv(name string, values []float32) {
	loc := r.shadowMapShader.u[name]
	switch len(values) {
	case 2:
		gl.Uniform2fv(loc, 1, &values[0])
	case 3:
		gl.Uniform3fv(loc, 1, &values[0])
	case 4:
		gl.Uniform4fv(loc, 1, &values[0])
	case 8:
		gl.Uniform4fv(loc, 2, &values[0])
	}
}
func (r *Renderer_GL) SetShadowMapUniformMatrix(name string, value []float32) {
	loc := r.shadowMapShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GL) SetShadowMapUniformMatrix3(name string, value []float32) {
	loc := r.shadowMapShader.u[name]
	gl.UniformMatrix3fv(loc, 1, false, &value[0])
}

func (r *Renderer_GL) SetShadowMapTexture(name string, tex Texture) {
	t := tex.(*Texture_GL)
	loc, unit := r.shadowMapShader.u[name], r.shadowMapShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer_GL) SetShadowFrameTexture(i uint32) {
	// OpenGL ES 3.1 doesn’t support this
}

func (r *Renderer_GL) SetShadowFrameCubeTexture(i uint32) {
	// OpenGL ES 3.1 doesn’t support this
}

func (r *Renderer_GL) SetVertexData(values ...float32) {
	data := f32.Bytes(binary.LittleEndian, values...)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
}
func (r *Renderer_GL) SetVertexDataArray(values []float32) {
	if len(values) == 0 {
		return
	}
	data := f32.Bytes(binary.LittleEndian, values...)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBufferBatch)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
}
func (r *Renderer_GL) SetModelVertexData(bufferIndex uint32, values []byte) {
	gl.BindBuffer(gl.ARRAY_BUFFER, r.modelVertexBuffer[bufferIndex])
	gl.BufferData(gl.ARRAY_BUFFER, len(values), unsafe.Pointer(&values[0]), gl.STATIC_DRAW)
}
func (r *Renderer_GL) SetModelIndexData(bufferIndex uint32, values ...uint32) {
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, values)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.modelIndexBuffer[bufferIndex])
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(values)*4, unsafe.Pointer(&data.Bytes()[0]), gl.STATIC_DRAW)
}

func (r *Renderer_GL) RenderQuad() {
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	sys.nDrawcall++
}
func (r *Renderer_GL) RenderQuadBatch(vertexCount int32) {
	gl.DrawArrays(gl.TRIANGLES, 0, vertexCount)
	sys.nDrawcall++
}
func (r *Renderer_GL) RenderElements(mode PrimitiveMode, count, offset int) {
	gl.DrawElementsWithOffset(r.MapPrimitiveMode(mode), int32(count), gl.UNSIGNED_INT, uintptr(offset))
	sys.nDrawcall++
}
func (r *Renderer_GL) RenderShadowMapElements(mode PrimitiveMode, count, offset int) {
	r.RenderElements(mode, count, offset)
}

func (r *Renderer_GL) RenderCubeMap(envTex Texture, cubeTex Texture) {
	envTexture := envTex.(*Texture_GL)
	cubeTexture := cubeTex.(*Texture_GL)
	textureSize := cubeTexture.width
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_env)
	gl.Viewport(0, 0, textureSize, textureSize)
	gl.UseProgram(r.panoramaToCubeMapShader.program)
	loc := r.panoramaToCubeMapShader.a["VertCoord"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, 0)
	data := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
	loc, unit := r.panoramaToCubeMapShader.u["panorama"], r.panoramaToCubeMapShader.t["panorama"]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, envTexture.handle)
	gl.Uniform1i(loc, int32(unit))
	for i := 0; i < 6; i++ {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+i), cubeTexture.handle, 0)

		gl.Clear(gl.COLOR_BUFFER_BIT)
		loc := r.panoramaToCubeMapShader.u["currentFace"]
		gl.Uniform1i(loc, int32(i))

		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
		sys.nDrawcall++
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.GenerateMipmap(gl.TEXTURE_CUBE_MAP)
}
func (r *Renderer_GL) RenderFilteredCubeMap(distribution int32, cubeTex Texture, filteredTex Texture, mipmapLevel, sampleCount int32, roughness float32) {
	cubeTexture := cubeTex.(*Texture_GL)
	filteredTexture := filteredTex.(*Texture_GL)
	textureSize := filteredTexture.width
	currentTextureSize := textureSize >> mipmapLevel
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_env)
	gl.Viewport(0, 0, currentTextureSize, currentTextureSize)
	gl.UseProgram(r.cubemapFilteringShader.program)
	loc := r.cubemapFilteringShader.a["VertCoord"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, 0)
	data := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
	loc, unit := r.cubemapFilteringShader.u["cubeMap"], r.cubemapFilteringShader.t["cubeMap"]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.Uniform1i(loc, int32(unit))
	loc = r.cubemapFilteringShader.u["sampleCount"]
	gl.Uniform1i(loc, sampleCount)
	loc = r.cubemapFilteringShader.u["distribution"]
	gl.Uniform1i(loc, distribution)
	loc = r.cubemapFilteringShader.u["width"]
	gl.Uniform1i(loc, textureSize)
	loc = r.cubemapFilteringShader.u["roughness"]
	gl.Uniform1f(loc, roughness)
	loc = r.cubemapFilteringShader.u["intensityScale"]
	gl.Uniform1f(loc, 1)
	loc = r.cubemapFilteringShader.u["isLUT"]
	gl.Uniform1i(loc, 0)
	for i := 0; i < 6; i++ {
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, uint32(gl.TEXTURE_CUBE_MAP_POSITIVE_X+i), filteredTexture.handle, mipmapLevel)

		gl.Clear(gl.COLOR_BUFFER_BIT)
		loc := r.cubemapFilteringShader.u["currentFace"]
		gl.Uniform1i(loc, int32(i))

		gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
		sys.nDrawcall++
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}
func (r *Renderer_GL) RenderLUT(distribution int32, cubeTex Texture, lutTex Texture, sampleCount int32) {
	cubeTexture := cubeTex.(*Texture_GL)
	lutTexture := lutTex.(*Texture_GL)
	textureSize := lutTexture.width
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_env)
	gl.Viewport(0, 0, textureSize, textureSize)
	gl.UseProgram(r.cubemapFilteringShader.program)
	loc := r.cubemapFilteringShader.a["VertCoord"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, 0)
	data := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
	loc, unit := r.cubemapFilteringShader.u["cubeMap"], r.cubemapFilteringShader.t["cubeMap"]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.Uniform1i(loc, int32(unit))
	loc = r.cubemapFilteringShader.u["sampleCount"]
	gl.Uniform1i(loc, sampleCount)
	loc = r.cubemapFilteringShader.u["distribution"]
	gl.Uniform1i(loc, distribution)
	loc = r.cubemapFilteringShader.u["width"]
	gl.Uniform1i(loc, textureSize)
	loc = r.cubemapFilteringShader.u["roughness"]
	gl.Uniform1f(loc, 0)
	loc = r.cubemapFilteringShader.u["intensityScale"]
	gl.Uniform1f(loc, 1)
	loc = r.cubemapFilteringShader.u["currentFace"]
	gl.Uniform1i(loc, 0)
	loc = r.cubemapFilteringShader.u["isLUT"]
	gl.Uniform1i(loc, 1)

	gl.BindTexture(gl.TEXTURE_2D, lutTexture.handle)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, lutTexture.width, lutTexture.height, 0, gl.RGBA, gl.FLOAT, nil)

	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, lutTexture.handle, 0)
	gl.Clear(gl.COLOR_BUFFER_BIT)
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
	sys.nDrawcall++
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}

func (r *Renderer_GL) PerspectiveProjectionMatrix(angle, aspect, near, far float32) mgl.Mat4 {
	return mgl.Perspective(angle, aspect, near, far)
}

func (r *Renderer_GL) OrthographicProjectionMatrix(left, right, bottom, top, near, far float32) mgl.Mat4 {
	ret := mgl.Ortho(left, right, bottom, top, near, far)
	return ret
}

func (r *Renderer_GL) NewWorkerThread() bool {
	return false
}

func (r *Renderer_GL) SetVSync() {
}
