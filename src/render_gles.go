//go:build gles2

// IF YOU MAKE CHANGES TO THIS FILE, YOU MUST ALSO MAKE
// EQUIVALENT CHANGES TO render_gl_gl32.go

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"unsafe"

	mgl "github.com/go-gl/mathgl/mgl32"
	gl "github.com/leonkasovan/gl/v3.1/gles2"
	"golang.org/x/mobile/exp/f32"
)

//const GL_SHADER_VER = 120 // OpenGL 2.1

// ------------------------------------------------------------------
// ShaderProgram_GLES

type ShaderProgram_GLES struct {
	// Program
	program uint32
	// Attributes
	a map[string]int32
	// Uniforms
	u map[string]int32
	// Texture_GLES units
	t map[string]int
}

func (r *Renderer_GLES) newShaderProgram(vert, frag, geo, id string, crashWhenFail bool) (s *ShaderProgram_GLES, err error) {
	var vertObj, fragObj, geoObj, prog uint32
	if vertObj, err = r.compileShader(gl.VERTEX_SHADER, vert); chkEX(err, "Shader compliation error on "+id+"\n", crashWhenFail) {
		return nil, err
	}
	if fragObj, err = r.compileShader(gl.FRAGMENT_SHADER, frag); chkEX(err, "Shader compliation error on "+id+"\n", crashWhenFail) {
		return nil, err
	}
	if len(geo) > 0 {
		if geoObj, err = r.compileShader(gl.GEOMETRY_SHADER_EXT, geo); chkEX(err, "Shader compliation error on "+id+"\n", crashWhenFail) {
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
	s = &ShaderProgram_GLES{program: prog}
	s.a = make(map[string]int32)
	s.u = make(map[string]int32)
	s.t = make(map[string]int)
	return s, nil
}

func (r *ShaderProgram_GLES) glStr(s string) *uint8 {
	return gl.Str(s + "\x00")
}
func (s *ShaderProgram_GLES) RegisterAttributes(names ...string) {
	for _, name := range names {
		s.a[name] = gl.GetAttribLocation(s.program, s.glStr(name))
	}
}

func (s *ShaderProgram_GLES) RegisterUniforms(names ...string) {
	for _, name := range names {
		s.u[name] = gl.GetUniformLocation(s.program, s.glStr(name))
	}
}

func (s *ShaderProgram_GLES) RegisterTextures(names ...string) {
	for _, name := range names {
		s.u[name] = gl.GetUniformLocation(s.program, s.glStr(name))
		s.t[name] = len(s.t)
	}
}

func (r *Renderer_GLES) compileShader(shaderType uint32, src string) (shader uint32, err error) {
	shader = gl.CreateShader(shaderType)
	src = "#version 300 es\nprecision mediump float;\n" + src + "\x00"
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

func (r *Renderer_GLES) linkProgram(params ...uint32) (program uint32, err error) {
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
// Texture_GLES

type Texture_GLES struct {
	width  int32
	height int32
	depth  int32
	filter bool
	handle uint32
}

// Generate a new texture name
func (r *Renderer_GLES) newTexture(width, height, depth int32, filter bool) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GLES{width, height, depth, filter, h}
	runtime.SetFinalizer(t, func(t *Texture_GLES) {
		sys.mainThreadTask <- func() {
			gl.DeleteTextures(1, &t.handle)
		}
	})
	return
}

func (r *Renderer_GLES) newDataTexture(width, height int32) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GLES{width, height, 32, false, h}
	runtime.SetFinalizer(t, func(t *Texture_GLES) {
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
func (r *Renderer_GLES) newHDRTexture(width, height int32) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GLES{width, height, 24, false, h}
	runtime.SetFinalizer(t, func(t *Texture_GLES) {
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
func (r *Renderer_GLES) newCubeMapTexture(widthHeight int32, mipmap bool) (t Texture) {
	var h uint32
	gl.ActiveTexture(gl.TEXTURE0)
	gl.GenTextures(1, &h)
	t = &Texture_GLES{widthHeight, widthHeight, 24, false, h}
	runtime.SetFinalizer(t, func(t *Texture_GLES) {
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
func (t *Texture_GLES) SetData(data []byte) {
	var interp int32 = gl.NEAREST
	if t.filter {
		interp = gl.LINEAR
	}

	format := t.MapInternalFormat(Max(t.depth, 8))

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
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
func (t *Texture_GLES) SetDataG(data []byte, mag, min, ws, wt int32) {

	format := t.MapInternalFormat(Max(t.depth, 8))

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, int32(format), t.width, t.height, 0, format, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
	gl.GenerateMipmap(gl.TEXTURE_2D)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, mag)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, min)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, ws)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, wt)
}
func (t *Texture_GLES) SetPixelData(data []float32) {

	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA32F, t.width, t.height, 0, gl.RGBA, gl.FLOAT, unsafe.Pointer(&data[0]))
}
func (t *Texture_GLES) SetRGBPixelData(data []float32) {
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.PixelStorei(gl.UNPACK_ALIGNMENT, 1)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGB32F, t.width, t.height, 0, gl.RGB, gl.FLOAT, unsafe.Pointer(&data[0]))
}

// Return whether texture has a valid handle
func (t *Texture_GLES) IsValid() bool {
	return t.handle != 0
}

func (t *Texture_GLES) GetWidth() int32 {
	return t.width
}

func (t *Texture_GLES) MapInternalFormat(i int32) uint32 {
	var InternalFormatLUT = map[int32]uint32{
		8:  gl.LUMINANCE,
		24: gl.RGB,
		32: gl.RGBA,
	}
	return InternalFormatLUT[i]
}

// ------------------------------------------------------------------
// Renderer_GLES

type Renderer_GLES struct {
	fbo         uint32
	fbo_texture uint32
	// Normal rendering
	rbo_depth uint32
	// MSAA rendering
	fbo_f         uint32
	fbo_f_texture *Texture_GLES
	// Shadow Map
	fbo_env uint32
	// Postprocessing FBOs
	fbo_pp         []uint32
	fbo_pp_texture []uint32
	// Post-processing shaders
	postVertBuffer   uint32
	postShaderSelect []*ShaderProgram_GLES
	// Shader and vertex data for primitive rendering
	spriteShader *ShaderProgram_GLES
	vertexBuffer uint32
	// Shader and index data for 3D model rendering
	shadowMapShader         *ShaderProgram_GLES
	modelShader             *ShaderProgram_GLES
	panoramaToCubeMapShader *ShaderProgram_GLES
	cubemapFilteringShader  *ShaderProgram_GLES
	stageVertexBuffer       uint32
	stageIndexBuffer        uint32

	enableModel  bool
	enableShadow bool
}

func (r *Renderer_GLES) GetName() string {
	return "OpenGL ES 3.1"
}

// init 3D model shader
func (r *Renderer_GLES) InitModelShader() error {
	var err error
	r.modelShader, err = r.newShaderProgram(modelVertShader, modelFragShaderES, "", "Model Shader", false)
	if err != nil {
		return err
	}
	r.modelShader.RegisterAttributes("vertexId", "position", "uv", "normalIn", "tangentIn", "vertColor", "joints_0", "joints_1", "weights_0", "weights_1")
	r.modelShader.RegisterUniforms("model", "view", "projection", "farPlane", "normalMatrix", "unlit", "baseColorFactor", "add", "mult", "useTexture", "useNormalMap", "useMetallicRoughnessMap", "useEmissionMap", "neg", "gray", "hue",
		"enableAlpha", "alphaThreshold", "numJoints", "morphTargetWeight", "morphTargetOffset", "morphTargetTextureDimension", "numTargets", "numVertices",
		"metallicRoughness", "ambientOcclusionStrength", "emission", "environmentIntensity", "mipCount",
		"cameraPosition", "environmentRotation",
		"lightMatrices[0]", "lightMatrices[1]", "lightMatrices[2]", "lightMatrices[3]",
		"lights[0].direction", "lights[0].range", "lights[0].color", "lights[0].intensity", "lights[0].position", "lights[0].innerConeCos", "lights[0].outerConeCos", "lights[0].type", "lights[0].shadowBias", "lights[0].shadowMapFar",
		"lights[1].direction", "lights[1].range", "lights[1].color", "lights[1].intensity", "lights[1].position", "lights[1].innerConeCos", "lights[1].outerConeCos", "lights[1].type", "lights[1].shadowBias", "lights[1].shadowMapFar",
		"lights[2].direction", "lights[2].range", "lights[2].color", "lights[2].intensity", "lights[2].position", "lights[2].innerConeCos", "lights[2].outerConeCos", "lights[2].type", "lights[2].shadowBias", "lights[2].shadowMapFar",
		"lights[3].direction", "lights[3].range", "lights[3].color", "lights[3].intensity", "lights[3].position", "lights[3].innerConeCos", "lights[3].outerConeCos", "lights[3].type", "lights[3].shadowBias", "lights[3].shadowMapFar",
	)
	r.modelShader.RegisterTextures("tex", "morphTargetValues", "jointMatrices", "normalMap", "metallicRoughnessMap", "ambientOcclusionMap", "emissionMap", "lambertianEnvSampler", "GGXEnvSampler", "GGXLUT",
		"shadowCubeMap[0]", "shadowCubeMap[1]", "shadowCubeMap[2]", "shadowCubeMap[3]")

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
func (r *Renderer_GLES) Init() {
	r.enableModel = sys.cfg.Video.EnableModel
	r.enableShadow = false //sys.cfg.Video.EnableModelShadow
	chk(gl.Init())
	fmt.Printf("Using %v (%v)\n", gl.GoStr(gl.GetString(gl.VERSION)), gl.GoStr(gl.GetString(gl.RENDERER)))
	fmt.Printf("scrrect: %v,%v - %v,%v\n", sys.scrrect[0], sys.scrrect[1], sys.scrrect[2], sys.scrrect[3])
	fmt.Printf("gameWidth x gameHeight: %v,%v\n", sys.gameWidth, sys.gameHeight)
	fmt.Printf("widthScale x heightScale: %v,%v\n", sys.widthScale, sys.heightScale)

	// Store current timestamp
	sys.prevTimestamp = sys.GetTime()

	r.postShaderSelect = make([]*ShaderProgram_GLES, 1+len(sys.cfg.Video.ExternalShaders))

	// Data buffers for rendering
	postVertData := f32.Bytes(binary.LittleEndian, -1, -1, 1, -1, -1, 1, 1, 1)

	gl.GenBuffers(1, &r.postVertBuffer)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.postVertBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(postVertData), unsafe.Pointer(&postVertData[0]), gl.STATIC_DRAW)

	gl.GenBuffers(1, &r.vertexBuffer)
	gl.GenBuffers(1, &r.stageVertexBuffer)
	gl.GenBuffers(1, &r.stageIndexBuffer)

	// Sprite shader
	r.spriteShader, _ = r.newShaderProgram(vertShader, fragShader, "", "Main Shader", true)
	r.spriteShader.RegisterAttributes("position", "uv")
	r.spriteShader.RegisterUniforms("modelview", "projection", "x1x2x4x3",
		"alpha", "tint", "mask", "neg", "gray", "add", "mult", "isFlat", "isRgba", "isTrapez", "hue")
	r.spriteShader.RegisterTextures("pal", "tex")
	if r.enableModel {
		if err := r.InitModelShader(); err != nil {
			r.enableModel = false
		}
	}

	// Compile postprocessing shaders

	// Calculate total amount of shaders loaded.
	r.postShaderSelect = make([]*ShaderProgram_GLES, 1+len(sys.cfg.Video.ExternalShaders))

	// Ident shader (no postprocessing)
	r.postShaderSelect[0], _ = r.newShaderProgram(identVertShader, identFragShader, "", "Identity Postprocess", true)
	r.postShaderSelect[0].RegisterAttributes("VertCoord")
	r.postShaderSelect[0].RegisterUniforms("Texture_GLES", "TextureSize", "CurrentTime")

	// External Shaders
	for i := 0; i < len(sys.cfg.Video.ExternalShaders); i++ {
		r.postShaderSelect[1+i], _ = r.newShaderProgram(sys.externalShaders[0][i],
			sys.externalShaders[1][i], "", fmt.Sprintf("Postprocess Shader #%v", i+1), true)
		r.postShaderSelect[1+i].RegisterUniforms("Texture_GLES", "TextureSize", "CurrentTime")
	}

	gl.ActiveTexture(gl.TEXTURE0)

	// create a texture for r.fbo
	gl.GenTextures(1, &r.fbo_texture)

	gl.BindTexture(gl.TEXTURE_2D, r.fbo_texture)

	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)

	gl.TexImage2D(
		gl.TEXTURE_2D,
		0,
		gl.RGBA,
		sys.scrrect[2],
		sys.scrrect[3],
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
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.DEPTH_COMPONENT16, sys.scrrect[2], sys.scrrect[3])
	gl.BindRenderbuffer(gl.RENDERBUFFER, 0)

	// create an FBO for our r.fbo, which is then for r.fbo_texture
	gl.GenFramebuffers(1, &r.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_texture, 0)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.DEPTH_ATTACHMENT, gl.RENDERBUFFER, r.rbo_depth)

	// create our two FBOs for our postprocessing needs
	for i := 0; i < 2; i++ {
		gl.GenFramebuffers(1, &(r.fbo_pp[i]))
		gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo_pp[i])
		gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, r.fbo_pp_texture[i], 0)
	}

	// create an FBO for our model stuff
	if r.enableModel {
		gl.GenFramebuffers(1, &r.fbo_env)
	}

	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

func (r *Renderer_GLES) Close() {
}

func (r *Renderer_GLES) IsModelEnabled() bool {
	return r.enableModel
}

func (r *Renderer_GLES) IsShadowEnabled() bool {
	return r.enableShadow
}
func (r *Renderer_GLES) BeginFrame(clearColor bool) {
	sys.absTickCountF++
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	if clearColor {
		gl.Clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
	} else {
		gl.Clear(gl.DEPTH_BUFFER_BIT)
	}
}

func (r *Renderer_GLES) BlendReset() {
	gl.BlendEquation(r.MapBlendEquation(BlendAdd))
	gl.BlendFunc(r.MapBlendFunction(BlendSrcAlpha), r.MapBlendFunction(BlendOneMinusSrcAlpha))
}
func (r *Renderer_GLES) EndFrame() {
	x, y, width, height := int32(0), int32(0), int32(sys.scrrect[2]), int32(sys.scrrect[3])
	time := sys.GetTime() // consistent time across all shaders

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
		gl.Uniform1i(postShader.u["Texture_GLES"], 0)
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
		gl.DisableVertexAttribArray(uint32(loc))
	}
}

func (r *Renderer_GLES) Await() {
	gl.Finish()
}

func (r *Renderer_GLES) MapBlendEquation(i BlendEquation) uint32 {
	var BlendEquationLUT = map[BlendEquation]uint32{
		BlendAdd:             gl.FUNC_ADD,
		BlendReverseSubtract: gl.FUNC_REVERSE_SUBTRACT,
	}
	return BlendEquationLUT[i]
}

func (r *Renderer_GLES) MapBlendFunction(i BlendFunc) uint32 {
	var BlendFunctionLUT = map[BlendFunc]uint32{
		BlendOne:              gl.ONE,
		BlendZero:             gl.ZERO,
		BlendSrcAlpha:         gl.SRC_ALPHA,
		BlendOneMinusSrcAlpha: gl.ONE_MINUS_SRC_ALPHA,
	}
	return BlendFunctionLUT[i]
}

func (r *Renderer_GLES) MapPrimitiveMode(i PrimitiveMode) uint32 {
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

func (r *Renderer_GLES) SetPipeline(eq BlendEquation, src, dst BlendFunc) {
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

func (r *Renderer_GLES) ReleasePipeline() {
	loc := r.spriteShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.spriteShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	gl.Disable(gl.BLEND)
}

func (r *Renderer_GLES) prepareShadowMapPipeline() {
	gl.UseProgram(r.shadowMapShader.program)
	gl.Viewport(0, 0, 1024, 1024)
	gl.Enable(gl.TEXTURE_2D)
	gl.Disable(gl.BLEND)
	gl.Enable(gl.DEPTH_TEST)
	//gl.DepthFunc(gl.LESS)
	//gl.DepthMask(true)

	gl.BlendEquation(gl.FUNC_ADD)
	gl.BlendFunc(gl.ONE, gl.ZERO)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.stageVertexBuffer)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.stageIndexBuffer)
}
func (r *Renderer_GLES) setShadowMapPipeline(doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	if invertFrontFace {
		gl.FrontFace(gl.CW)
	} else {
		gl.FrontFace(gl.CCW)
	}
	if !doubleSided {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	} else {
		gl.Disable(gl.CULL_FACE)
	}

	loc := r.shadowMapShader.a["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.shadowMapShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices
	if useUV {
		loc = r.shadowMapShader.a["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else {
		loc = r.shadowMapShader.a["uv"]
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
		loc = r.shadowMapShader.a["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.shadowMapShader.a["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			loc = r.shadowMapShader.a["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.shadowMapShader.a["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else {
			loc = r.shadowMapShader.a["joints_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.shadowMapShader.a["weights_1"]
			gl.DisableVertexAttribArray(uint32(loc))
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else {
		loc = r.shadowMapShader.a["joints_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.a["weights_0"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.a["joints_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.shadowMapShader.a["weights_1"]
		gl.DisableVertexAttribArray(uint32(loc))
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}

func (r *Renderer_GLES) ReleaseShadowPipeline() {
	loc := r.modelShader.a["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	//gl.Disable(gl.TEXTURE_2D)
	gl.DepthMask(true)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
	gl.Disable(gl.BLEND)
}
func (r *Renderer_GLES) prepareModelPipeline(env *Environment) {
	gl.UseProgram(r.modelShader.program)
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.Viewport(0, 0, sys.scrrect[2], sys.scrrect[3])
	gl.Clear(gl.DEPTH_BUFFER_BIT)
	gl.Enable(gl.TEXTURE_2D)
	gl.Enable(gl.TEXTURE_CUBE_MAP)
	gl.Enable(gl.BLEND)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.stageVertexBuffer)
	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.stageIndexBuffer)
	if env != nil {
		loc, unit := r.modelShader.u["lambertianEnvSampler"], r.modelShader.t["lambertianEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, env.lambertianTexture.tex.(*Texture_GLES).handle)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["GGXEnvSampler"], r.modelShader.t["GGXEnvSampler"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_CUBE_MAP, env.GGXTexture.tex.(*Texture_GLES).handle)
		gl.Uniform1i(loc, int32(unit))
		loc, unit = r.modelShader.u["GGXLUT"], r.modelShader.t["GGXLUT"]
		gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
		gl.BindTexture(gl.TEXTURE_2D, env.GGXLUT.tex.(*Texture_GLES).handle)
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
func (r *Renderer_GLES) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1 bool, numVertices, vertAttrOffset uint32) {
	if depthTest {
		gl.Enable(gl.DEPTH_TEST)
		gl.DepthFunc(gl.LESS)
	} else {
		gl.Disable(gl.DEPTH_TEST)
	}
	gl.DepthMask(depthMask)
	if invertFrontFace {
		gl.FrontFace(gl.CW)
	} else {
		gl.FrontFace(gl.CCW)
	}
	if !doubleSided {
		gl.Enable(gl.CULL_FACE)
		gl.CullFace(gl.BACK)
	} else {
		gl.Disable(gl.CULL_FACE)
	}

	gl.BlendEquation(r.MapBlendEquation(eq))
	gl.BlendFunc(r.MapBlendFunction(src), r.MapBlendFunction(dst))

	loc := r.modelShader.a["vertexId"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 1, gl.INT, false, 0, uintptr(vertAttrOffset))
	offset := vertAttrOffset + 4*numVertices

	loc = r.modelShader.a["position"]
	gl.EnableVertexAttribArray(uint32(loc))
	gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
	offset += 12 * numVertices
	if useUV {
		loc = r.modelShader.a["uv"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 2, gl.FLOAT, false, 0, uintptr(offset))
		offset += 8 * numVertices
	} else {
		loc = r.modelShader.a["uv"]
		gl.VertexAttrib2f(uint32(loc), 0, 0)
	}
	if useNormal {
		loc = r.modelShader.a["normalIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 3, gl.FLOAT, false, 0, uintptr(offset))
		offset += 12 * numVertices
	} else {
		loc = r.modelShader.a["normalIn"]
		gl.VertexAttrib3f(uint32(loc), 0, 0, 0)
	}
	if useTangent {
		loc = r.modelShader.a["tangentIn"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.modelShader.a["tangentIn"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
	if useVertColor {
		loc = r.modelShader.a["vertColor"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
	} else {
		loc = r.modelShader.a["vertColor"]
		gl.VertexAttrib4f(uint32(loc), 1, 1, 1, 1)
	}
	if useJoint0 {
		loc = r.modelShader.a["joints_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		loc = r.modelShader.a["weights_0"]
		gl.EnableVertexAttribArray(uint32(loc))
		gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
		offset += 16 * numVertices
		if useJoint1 {
			loc = r.modelShader.a["joints_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
			loc = r.modelShader.a["weights_1"]
			gl.EnableVertexAttribArray(uint32(loc))
			gl.VertexAttribPointerWithOffset(uint32(loc), 4, gl.FLOAT, false, 0, uintptr(offset))
			offset += 16 * numVertices
		} else {
			loc = r.modelShader.a["joints_1"]
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
			loc = r.modelShader.a["weights_1"]
			gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		}
	} else {
		loc = r.modelShader.a["joints_0"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_0"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["joints_1"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
		loc = r.modelShader.a["weights_1"]
		gl.VertexAttrib4f(uint32(loc), 0, 0, 0, 0)
	}
}
func (r *Renderer_GLES) ReleaseModelPipeline() {
	loc := r.modelShader.a["vertexId"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["position"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["uv"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["vertColor"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_0"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["joints_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	loc = r.modelShader.a["weights_1"]
	gl.DisableVertexAttribArray(uint32(loc))
	//gl.Disable(gl.TEXTURE_2D)
	gl.DepthMask(true)
	gl.Disable(gl.DEPTH_TEST)
	gl.Disable(gl.CULL_FACE)
}
func (r *Renderer_GLES) ReadPixels(data []uint8, width, height int) {
	// we defer the EndFrame(), SwapBuffers(), and BeginFrame() calls that were previously below now to
	// a single spot in order to prevent the blank screenshot bug on single digit FPS
	gl.BindFramebuffer(gl.READ_FRAMEBUFFER, 0)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&data[0]))
}

func (r *Renderer_GLES) Scissor(x, y, width, height int32) {
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(x, sys.scrrect[3]-(y+height), width, height)
}

func (r *Renderer_GLES) DisableScissor() {
	gl.Disable(gl.SCISSOR_TEST)
}

func (r *Renderer_GLES) SetUniformI(name string, val int) {
	loc := r.spriteShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer_GLES) SetUniformF(name string, values ...float32) {
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

func (r *Renderer_GLES) SetUniformFv(name string, values []float32) {
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

func (r *Renderer_GLES) SetUniformMatrix(name string, value []float32) {
	loc := r.spriteShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES) SetTexture(name string, tex Texture) {
	t := tex.(*Texture_GLES)
	loc, unit := r.spriteShader.u[name], r.spriteShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer_GLES) SetModelUniformI(name string, val int) {
	loc := r.modelShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer_GLES) SetModelUniformF(name string, values ...float32) {
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
func (r *Renderer_GLES) SetModelUniformFv(name string, values []float32) {
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
func (r *Renderer_GLES) SetModelUniformMatrix(name string, value []float32) {
	loc := r.modelShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES) SetModelUniformMatrix3(name string, value []float32) {
	loc := r.modelShader.u[name]
	gl.UniformMatrix3fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES) SetModelTexture(name string, tex Texture) {
	t := tex.(*Texture_GLES)
	loc, unit := r.modelShader.u[name], r.modelShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer_GLES) SetShadowMapUniformI(name string, val int) {
	loc := r.shadowMapShader.u[name]
	gl.Uniform1i(loc, int32(val))
}

func (r *Renderer_GLES) SetShadowMapUniformF(name string, values ...float32) {
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
func (r *Renderer_GLES) SetShadowMapUniformFv(name string, values []float32) {
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
func (r *Renderer_GLES) SetShadowMapUniformMatrix(name string, value []float32) {
	loc := r.shadowMapShader.u[name]
	gl.UniformMatrix4fv(loc, 1, false, &value[0])
}

func (r *Renderer_GLES) SetShadowMapTexture(name string, tex Texture) {
	t := tex.(*Texture_GLES)
	loc, unit := r.shadowMapShader.u[name], r.shadowMapShader.t[name]
	gl.ActiveTexture((uint32(gl.TEXTURE0 + unit)))
	gl.BindTexture(gl.TEXTURE_2D, t.handle)
	gl.Uniform1i(loc, int32(unit))
}

func (r *Renderer_GLES) SetShadowFrameTexture(i uint32) {
	//do nothing in GLES
}

func (r *Renderer_GLES) SetShadowFrameCubeTexture(i uint32) {
	//do nothing in GLES
}

func (r *Renderer_GLES) SetVertexData(values ...float32) {
	data := f32.Bytes(binary.LittleEndian, values...)
	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(data), unsafe.Pointer(&data[0]), gl.STATIC_DRAW)
}
func (r *Renderer_GLES) SetStageVertexData(values []byte) {
	gl.BindBuffer(gl.ARRAY_BUFFER, r.stageVertexBuffer)
	gl.BufferData(gl.ARRAY_BUFFER, len(values), unsafe.Pointer(&values[0]), gl.STATIC_DRAW)
}
func (r *Renderer_GLES) SetStageIndexData(values ...uint32) {
	data := new(bytes.Buffer)
	binary.Write(data, binary.LittleEndian, values)

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.stageIndexBuffer)
	gl.BufferData(gl.ELEMENT_ARRAY_BUFFER, len(values)*4, unsafe.Pointer(&data.Bytes()[0]), gl.STATIC_DRAW)
}

func (r *Renderer_GLES) RenderQuad() {
	gl.DrawArrays(gl.TRIANGLE_STRIP, 0, 4)
}
func (r *Renderer_GLES) RenderElements(mode PrimitiveMode, count, offset int) {
	gl.DrawElementsWithOffset(r.MapPrimitiveMode(mode), int32(count), gl.UNSIGNED_INT, uintptr(offset))
}

func (r *Renderer_GLES) RenderCubeMap(envTex Texture, cubeTex Texture) {
	envTexture := envTex.(*Texture_GLES)
	cubeTexture := cubeTex.(*Texture_GLES)
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
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
	gl.BindTexture(gl.TEXTURE_CUBE_MAP, cubeTexture.handle)
	gl.GenerateMipmap(gl.TEXTURE_CUBE_MAP)
}
func (r *Renderer_GLES) RenderFilteredCubeMap(distribution int32, cubeTex Texture, filteredTex Texture, mipmapLevel, sampleCount int32, roughness float32) {
	cubeTexture := cubeTex.(*Texture_GLES)
	filteredTexture := filteredTex.(*Texture_GLES)
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
	gl.Uniform1i(loc, currentTextureSize)
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
	}
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}
func (r *Renderer_GLES) RenderLUT(distribution int32, cubeTex Texture, lutTex Texture, sampleCount int32) {
	cubeTexture := cubeTex.(*Texture_GLES)
	lutTexture := lutTex.(*Texture_GLES)
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
	gl.BindFramebuffer(gl.FRAMEBUFFER, r.fbo)
}
