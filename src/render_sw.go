//go:build !android && !armdevice

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"unsafe"

	mgl "github.com/go-gl/mathgl/mgl32"
	"github.com/ikemen-engine/Ikemen-GO/packages/go-sdl2/sdl"
)

// swTexture is the CPU-side texture for the software renderer. Pixel data lives
// in ordinary Go memory: depth 8 = palette indices, 24 = RGB, 32 = RGBA bytes,
// palette textures (palSlot) = 256 RGBA entries. Sampling is done directly on
// the []byte; there is no GPU upload until the final present blit.
type swTexture struct {
	width, height, depth int32
	filter               bool // bilinear sampling (RGB sprites with RGBSpriteBilinearFilter)
	data                 []byte
	palSlot              bool // palette texture: data is w*h*4 RGBA bytes
	serial               uint64
	released             bool
}

func (t *swTexture) SetData(data []byte) {
	if data == nil || t.released {
		return
	}
	t.data = append(t.data[:0], data...)
}

func (t *swTexture) SetSubData(data []byte, x, y, width, height, stride int32) {
	if data == nil || t.released {
		return
	}
	bpp := t.depth / 8
	if bpp < 1 {
		bpp = 1
	}
	if stride <= 0 {
		stride = width * bpp
	}
	rows := height
	if int64(rows)*int64(stride) > int64(len(data)) {
		rows = int32(int64(len(data)) / int64(stride))
	}
	if rows < 1 {
		return
	}
	for row := int32(0); row < rows; row++ {
		dstOff := int((y+row)*t.width*bpp + x*bpp)
		src := data[row*stride : row*stride+width*bpp]
		if dstOff+int(width*bpp) > len(t.data) {
			n := len(t.data) - dstOff
			if n <= 0 {
				return
			}
			copy(t.data[dstOff:], src[:n])
			return
		}
		copy(t.data[dstOff:dstOff+int(width*bpp)], src)
	}
}

func (t *swTexture) SetDataG(data []byte, mag, min, ws, wt TextureSamplingParam) {
	t.filter = mag == TextureSamplingFilterLinear || mag == TextureSamplingFilterLinearMipMapNearest ||
		mag == TextureSamplingFilterLinearMipMapLinear
	t.SetData(data)
}

// SetPixelData stores float data. Model/HDR/data textures are never sampled by
// the software renderer (models are disabled), so this is a no-op.
func (t *swTexture) SetPixelData(data []float32) {}

func (t *swTexture) IsValid() bool {
	return t != nil && t.width != 0 && t.height != 0 && !t.released
}

func (t *swTexture) GetWidth() int32  { return t.width }
func (t *swTexture) GetHeight() int32 { return t.height }

// GetPalUV is only meaningful to shaders; the software rasterizer indexes the
// palette buffer directly.
func (t *swTexture) GetPalUV() [4]float32 { return [4]float32{0, 0.5, 1, 0} }

func (t *swTexture) GetSerial() uint64 { return t.serial }

func (t *swTexture) CopyData(src *Texture) {
	if src == nil || t.released {
		return
	}
	if s, ok := (*src).(*swTexture); ok && s.data != nil {
		n := Min(len(s.data), len(t.data))
		copy(t.data[:n], s.data[:n])
	}
}

func (t *swTexture) Release() {
	t.released = true
	t.data = nil
}

// Renderer_SW is a CPU-only rendering backend: the engine composes every frame
// into a system-memory framebuffer (r.pix), which is uploaded once per frame
// to a streaming SDL texture and presented via SDL_RenderCopy/Present.
// See RENDERER_PLAN.md.
type Renderer_SW struct {
	renderer *sdl.Renderer
	target   *sdl.Texture
	pix      []byte // RGBA bytes, top-down, pitch = w*4
	pitch    int
	w, h     int32

	// Virtual pipeline state for the immediate draw path (RenderQuad).
	curProjection, curModelview       mgl.Mat4
	curIsFlat, curIsRgba, curIsTrapez bool
	curMask                           int32
	curHue                            float32
	curTint                           [4]float32
	curPalUV                          [4]float32
	curX1x2x4x3                       [4]float32
	curAdd, curMult                   [3]float32
	curAlpha, curGray                 float32
	curNeg                            bool
	curTex, curPal                    Texture
	curBlendEq                        BlendEquation
	curBlendSrc, curBlendDst          BlendFunc
	curBlending                       bool
	curScissorOn                      bool
	curScissor                        [4]int32
	curPipeline                       string
	curVerts                          [16]float32
	curHasVerts                       bool
	customShaderLogged                map[string]bool

	// Dirty-rect tracking: only the touched region is uploaded at present.
	dirty                              bool
	dirtyX0, dirtyY0, dirtyX1, dirtyY1 int32
}

func (r *Renderer_SW) GetName() string { return "SDL2 Software" }

func (r *Renderer_SW) DebugInfo() string {
	return fmt.Sprintf("SDL2 Software Renderer — %dx%d framebuffer, CPU compositing", r.w, r.h)
}

func (r *Renderer_SW) Init() {
	r.w, r.h = sys.scrrect[2], sys.scrrect[3]
	r.pitch = int(r.w) * 4
	r.pix = make([]byte, int(r.w)*int(r.h)*4)

	var err error
	r.renderer, err = sdl.CreateRenderer(sys.window.Window, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		LogMessage("[SDL2 Software] SDL_CreateRenderer failed (%v); retrying with the software driver", err)
		r.renderer, err = sdl.CreateRenderer(sys.window.Window, -1, 0)
		if err != nil {
			panic(fmt.Sprintf("[SDL2 Software] SDL_CreateRenderer failed: %v", err))
		}
	}
	// The engine framebuffer stores pixels as [R,G,B,A] bytes. On little-endian
	// machines SDL_PIXELFORMAT_RGBA8888 expects bytes in the order [A,B,G,R]
	// (R is at the highest byte of the packed dword — see
	// SDL_PixelFormatEnumToMasks: PACKEDORDER_RGBA -> Rmask=0xFF000000). Only
	// SDL_PIXELFORMAT_ABGR8888 maps to memory bytes [R,G,B,A], so it is the
	// format that lets us upload the framebuffer directly, byte-compatible with
	// OpenGL's GL_RGBA8 on this architecture.
	r.target, err = r.renderer.CreateTexture(sdl.PIXELFORMAT_ABGR8888, sdl.TEXTUREACCESS_STREAMING, r.w, r.h)
	if err != nil {
		panic(fmt.Sprintf("[SDL2 Software] SDL_CreateTexture failed: %v", err))
	}
	// All blending is done by the engine; the texture is presented verbatim.
	_ = r.target.SetBlendMode(sdl.BLENDMODE_NONE)

	r.dirty = true
	r.dirtyX0, r.dirtyY0, r.dirtyX1, r.dirtyY1 = 0, 0, r.w, r.h
	r.customShaderLogged = make(map[string]bool)
	LogMessage("[SDL2 Software] Framebuffer %dx%d (CPU compositing)", r.w, r.h)
}

func (r *Renderer_SW) Close() {
	if r.target != nil {
		_ = r.target.Destroy()
		r.target = nil
	}
	if r.renderer != nil {
		_ = r.renderer.Destroy()
		r.renderer = nil
	}
	r.pix = nil
}

func (r *Renderer_SW) BeginFrame(clearColor bool) {
	drawCallStats.reset()
	lastRenderParams = nil
	resetSpriteQueue()
	if clearColor {
		// Fast zero of the framebuffer.
		for i := range r.pix {
			r.pix[i] = 0
		}
		r.markDirty(0, 0, r.w, r.h)
	}
	r.curHasVerts = false
	r.curScissorOn = false
	r.curBlending = false
	r.curPipeline = ""
}

func (r *Renderer_SW) EndFrame() {
	flushSpriteQueue()
	drawCallStats.logFrame(int(sys.frameCounter))
	r.dumpFrame()
	r.present()
}

// dumpFrame writes the framebuffer to a PNG every 60 frames when
// IKEMEN_SW_DUMP_DIR is set (visual-regression debugging).
func (r *Renderer_SW) dumpFrame() {
	dir := os.Getenv("IKEMEN_SW_DUMP_DIR")
	if dir == "" || sys.frameCounter%60 != 0 {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, int(r.w), int(r.h)))
	for y := 0; y < int(r.h); y++ {
		copy(img.Pix[y*int(r.w)*4:(y+1)*int(r.w)*4], r.pix[y*r.pitch:y*r.pitch+r.pitch])
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	fn := filepath.Join(dir, fmt.Sprintf("sw_frame_%05d.png", sys.frameCounter))
	if f, err := os.Create(fn); err == nil {
		_ = png.Encode(f, img)
		_ = f.Close()
		LogMessage("[SDL2 Software] dumped frame %d -> %s", sys.frameCounter, fn)
	}
}

func (r *Renderer_SW) Await() {}

func (r *Renderer_SW) present() {
	if r.target == nil || r.renderer == nil {
		return
	}
	if r.dirty {
		// r.pix is [R,G,B,A]; the streaming texture is ABGR8888 which on
		// little-endian is the same byte order, so the upload is a direct copy.
		// Only the region touched this frame is uploaded: SDL_UpdateTexture
		// expects the pixel pointer at the rect's top-left corner and the full
		// framebuffer row stride as pitch.
		w := r.dirtyX1 - r.dirtyX0
		h := r.dirtyY1 - r.dirtyY0
		if w > 0 && h > 0 {
			rect := &sdl.Rect{X: r.dirtyX0, Y: r.dirtyY0, W: w, H: h}
			off := int(r.dirtyY0)*r.pitch + int(r.dirtyX0)*4
			if err := r.target.Update(rect, unsafe.Pointer(&r.pix[off]), int(r.pitch)); err != nil {
				LogError("[SDL2 Software] texture update failed: %v", err)
			}
		}
		r.dirty = false
	}
	// The SDL backbuffer is not auto-cleared between presents. When the scaled
	// viewport (KeepAspect) is smaller than the window — e.g. a 16:9 storyboard
	// followed by a 4:3 stage fight — the letterbox strips would otherwise keep
	// the previous frame's pixels, leaving storyboard content visible in the
	// left/right (or top/bottom) bars. Every other backend (GL/D3D/Vulkan)
	// clears its full backbuffer each frame; clear here too so the strips are
	// black like the other renderers.
	_ = r.renderer.SetDrawColor(0, 0, 0, 255)
	_ = r.renderer.Clear()
	if sys.cfg.Video.KeepAspect {
		x, y, w, h := sys.window.GetScaledViewportSize()
		dst := &sdl.Rect{X: x, Y: y, W: w, H: h}
		_ = r.renderer.Copy(r.target, nil, dst)
	} else {
		_ = r.renderer.Copy(r.target, nil, nil)
	}
	r.renderer.Present()
	r.dumpWindow()
}

// dumpWindow writes the presented SDL backbuffer to a PNG every 60 frames when
// IKEMEN_SW_DUMP_DIR is set. Unlike dumpFrame (which captures the engine
// framebuffer), this captures what is actually blitted to the window, so
// letterbox/presentation issues are visible.
func (r *Renderer_SW) dumpWindow() {
	dir := os.Getenv("IKEMEN_SW_DUMP_DIR")
	if dir == "" || sys.frameCounter%60 != 0 {
		return
	}
	winW, winH := sys.window.GetSize()
	if winW <= 0 || winH <= 0 {
		return
	}
	buf := make([]byte, winW*winH*4)
	rect := &sdl.Rect{X: 0, Y: 0, W: int32(winW), H: int32(winH)}
	if err := r.renderer.ReadPixels(rect, sdl.PIXELFORMAT_ABGR8888, unsafe.Pointer(&buf[0]), winW*4); err != nil {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, winW, winH))
	for y := 0; y < winH; y++ {
		copy(img.Pix[y*winW*4:(y+1)*winW*4], buf[y*winW*4:(y+1)*winW*4])
	}
	fn := filepath.Join(dir, fmt.Sprintf("sw_win_%05d.png", sys.frameCounter))
	if f, err := os.Create(fn); err == nil {
		_ = png.Encode(f, img)
		_ = f.Close()
		LogMessage("[SDL2 Software] dumped window %d -> %s", sys.frameCounter, fn)
	}
}

// markDirty extends the per-frame dirty bounding box (framebuffer coords).
func (r *Renderer_SW) markDirty(x0, y0, x1, y1 int32) {
	if x1 <= 0 || y1 <= 0 || x0 >= r.w || y0 >= r.h {
		return
	}
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > r.w {
		x1 = r.w
	}
	if y1 > r.h {
		y1 = r.h
	}
	if !r.dirty {
		r.dirty = true
		r.dirtyX0, r.dirtyY0, r.dirtyX1, r.dirtyY1 = x0, y0, x1, y1
		return
	}
	if x0 < r.dirtyX0 {
		r.dirtyX0 = x0
	}
	if y0 < r.dirtyY0 {
		r.dirtyY0 = y0
	}
	if x1 > r.dirtyX1 {
		r.dirtyX1 = x1
	}
	if y1 > r.dirtyY1 {
		r.dirtyY1 = y1
	}
}

func (r *Renderer_SW) IsModelEnabled() bool  { return false }
func (r *Renderer_SW) IsShadowEnabled() bool { return false }

// ---- Textures ----

func (r *Renderer_SW) makeTexture(width, height, depth int32, filter, palSlot bool) *swTexture {
	textureSerialNumber++
	t := &swTexture{
		width: width, height: height, depth: depth,
		filter: filter, palSlot: palSlot, serial: textureSerialNumber,
	}
	bpp := depth / 8
	if bpp < 1 {
		bpp = 1
	}
	t.data = make([]byte, int(width)*int(height)*int(bpp))
	return t
}

func (r *Renderer_SW) newTexture(width, height, depth int32, filter bool) Texture {
	return r.makeTexture(width, height, depth, filter, false)
}

func (r *Renderer_SW) newPaletteTexture() Texture {
	return r.makeTexture(256, 1, 32, false, true)
}

func (r *Renderer_SW) newModelTexture(width, height, depth int32, filter bool) Texture {
	return r.newTexture(width, height, depth, filter)
}

func (r *Renderer_SW) newDataTexture(width, height int32) Texture {
	return r.makeTexture(width, height, 128, false, false)
}

func (r *Renderer_SW) newHDRTexture(width, height int32) Texture {
	return r.makeTexture(width, height, 128, false, false)
}

func (r *Renderer_SW) newCubeMapTexture(widthHeight int32, mipmap bool, lowestMipLevel int32) Texture {
	return r.makeTexture(widthHeight, widthHeight, 32, false, false)
}

// ---- Model path (disabled on this backend) ----

func (r *Renderer_SW) prepareShadowMapPipeline(bufferIndex uint32)                   {}
func (r *Renderer_SW) setShadowMapPipeline(a, b, c, d, e, f, g, h bool, i, j uint32) {}
func (r *Renderer_SW) ReleaseShadowPipeline()                                        {}
func (r *Renderer_SW) prepareModelPipeline(bufferIndex uint32, env *Environment)     {}
func (r *Renderer_SW) SetModelPipeline(eq BlendEquation, src, dst BlendFunc, depthTest, depthMask, doubleSided, invertFrontFace, useUV, useNormal, useTangent, useVertColor, useJoint0, useJoint1, useOutlineAttribute bool, numVertices, vertAttrOffset uint32) {
}
func (r *Renderer_SW) SetMeshOutlinePipeline(invertFrontFace bool, meshOutline float32) {}
func (r *Renderer_SW) ReleaseModelPipeline()                                            {}
func (r *Renderer_SW) SetModelUniformI(name string, val int)                            {}
func (r *Renderer_SW) SetModelUniformF(name string, values ...float32)                  {}
func (r *Renderer_SW) SetModelUniformFv(name string, values []float32)                  {}
func (r *Renderer_SW) SetModelUniformMatrix(name string, value []float32)               {}
func (r *Renderer_SW) SetModelUniformMatrix3(name string, value []float32)              {}
func (r *Renderer_SW) SetModelTexture(name string, t Texture)                           {}
func (r *Renderer_SW) SetShadowMapUniformI(name string, val int)                        {}
func (r *Renderer_SW) SetShadowMapUniformF(name string, values ...float32)              {}
func (r *Renderer_SW) SetShadowMapUniformFv(name string, values []float32)              {}
func (r *Renderer_SW) SetShadowMapUniformMatrix(name string, value []float32)           {}
func (r *Renderer_SW) SetShadowMapUniformMatrix3(name string, value []float32)          {}
func (r *Renderer_SW) SetShadowMapTexture(name string, t Texture)                       {}
func (r *Renderer_SW) SetShadowFrameTexture(i uint32)                                   {}
func (r *Renderer_SW) SetShadowFrameCubeTexture(i uint32)                               {}
func (r *Renderer_SW) SetModelVertexData(bufferIndex uint32, values []byte)             {}
func (r *Renderer_SW) SetModelIndexData(bufferIndex uint32, values ...uint32)           {}
func (r *Renderer_SW) RenderElements(mode PrimitiveMode, count, offset int)             {}
func (r *Renderer_SW) RenderShadowMapElements(mode PrimitiveMode, count, offset int)    {}
func (r *Renderer_SW) RenderCubeMap(envTexture Texture, cubeTexture Texture)            {}
func (r *Renderer_SW) RenderFilteredCubeMap(distribution int32, cubeTexture Texture, filteredTexture Texture, mipmapLevel, sampleCount int32, roughness float32) {
}
func (r *Renderer_SW) RenderLUT(distribution int32, cubeTexture Texture, lutTexture Texture, sampleCount int32) {
}

// ---- Virtual pipeline (immediate path) ----

func (r *Renderer_SW) EnableBlending(eq BlendEquation, src, dst BlendFunc) {
	r.curBlendEq, r.curBlendSrc, r.curBlendDst = eq, src, dst
	r.curBlending = true
}

func (r *Renderer_SW) DisableBlending() {
	r.curBlending = false
}

func (r *Renderer_SW) EnableScissor(x, y, width, height int32) {
	r.curScissorOn = true
	r.curScissor = [4]int32{x, y, width, height}
}

func (r *Renderer_SW) DisableScissor() {
	r.curScissorOn = false
}

func (r *Renderer_SW) SetUniformI(name string, val int) {
	switch name {
	case "isFlat":
		r.curIsFlat = val != 0
	case "isRgba":
		r.curIsRgba = val != 0
	case "isTrapez":
		r.curIsTrapez = val != 0
	case "mask":
		r.curMask = int32(val)
	case "neg":
		r.curNeg = val != 0
	}
}

func (r *Renderer_SW) SetUniformF(name string, values ...float32) {
	switch name {
	case "hue":
		if len(values) > 0 {
			r.curHue = values[0]
		}
	case "alpha":
		if len(values) > 0 {
			r.curAlpha = values[0]
		}
	case "gray":
		if len(values) > 0 {
			r.curGray = values[0]
		}
	case "tint":
		copy(r.curTint[:], values)
	case "palUV":
		copy(r.curPalUV[:], values)
	case "x1x2x4x3":
		copy(r.curX1x2x4x3[:], values)
	}
}

func (r *Renderer_SW) SetUniformFv(name string, values []float32) {
	switch name {
	case "tint":
		copy(r.curTint[:], values)
	case "palUV":
		copy(r.curPalUV[:], values)
	case "add":
		copy(r.curAdd[:], values)
	case "mult":
		copy(r.curMult[:], values)
	case "x1x2x4x3":
		copy(r.curX1x2x4x3[:], values)
	}
}

func (r *Renderer_SW) SetUniformMatrix(name string, value []float32) {
	if len(value) < 16 {
		return
	}
	var m mgl.Mat4
	copy(m[:], value[:16])
	if name == "projection" {
		r.curProjection = m
	} else if name == "modelview" {
		r.curModelview = m
	}
}

func (r *Renderer_SW) SetTexture(name string, tex Texture) {
	switch name {
	case "tex":
		r.curTex = tex
	case "pal":
		r.curPal = tex
	}
}

func (r *Renderer_SW) SetVertexData(values ...float32) {
	n := Min(len(values), 16)
	copy(r.curVerts[:n], values[:n])
	r.curHasVerts = n == 16
}

func (r *Renderer_SW) RenderQuad() {
	if !r.curHasVerts {
		return
	}
	r.curHasVerts = false
	if r.curPipeline != "" {
		if !r.customShaderLogged[r.curPipeline] {
			r.customShaderLogged[r.curPipeline] = true
			LogMessage("[SDL2 Software] Custom shader '%s' is not supported; rendering the sprite without it", r.curPipeline)
		}
	}

	// Debug: log all quads (with window-space bbox) for the first few frames
	// when IKEMEN_SW_DUMP_QUAD=1, so we can see exactly what is drawn where.
	if os.Getenv("IKEMEN_SW_DUMP_QUAD") == "1" && sys.frameCounter < 6 {
		mat := r.curProjection.Mul4(r.curModelview)
		hw := float32(r.w) * 0.5
		hh := float32(r.h) * 0.5
		var v [4]swVertex
		for i := 0; i < 4; i++ {
			p := mat.Mul4x1(mgl.Vec4{r.curVerts[i*4], r.curVerts[i*4+1], 0, 1})
			x, y := p.X(), p.Y()
			if p.W() != 0 {
				x /= p.W()
				y /= p.W()
			}
			x = (x + 1) * hw
			y = (y + 1) * hh
			v[i] = swVertex{x, y, r.curVerts[i*4+2], r.curVerts[i*4+3]}
		}
		minX := v[0].x
		minY := v[0].y
		maxX := v[0].x
		maxY := v[0].y
		for i := 1; i < 4; i++ {
			if v[i].x < minX {
				minX = v[i].x
			}
			if v[i].y < minY {
				minY = v[i].y
			}
			if v[i].x > maxX {
				maxX = v[i].x
			}
			if v[i].y > maxY {
				maxY = v[i].y
			}
		}
		var tdesc, pdesc string
		if t, ok := r.curTex.(*swTexture); ok && t != nil {
			tdesc = fmt.Sprintf("%dx%dd%d", t.width, t.height, t.depth)
		} else {
			tdesc = "none"
		}
		if p, ok := r.curPal.(*swTexture); ok && p != nil {
			pdesc = "pal"
		} else {
			pdesc = "-"
		}
		flatCol := ""
		if r.curIsFlat {
			flatCol = fmt.Sprintf(" tint=(%d,%d,%d,%d)",
				quant(r.curTint[0]), quant(r.curTint[1]), quant(r.curTint[2]), quant(r.curTint[3]))
		}
		palDbg := ""
		if p, ok := r.curPal.(*swTexture); ok && p != nil && len(p.data) >= 256*4 {
			palDbg = fmt.Sprintf(" pal[0]=(%d,%d,%d,%d) pal[242]=(%d,%d,%d,%d) pal[254]=(%d,%d,%d,%d)",
				p.data[0], p.data[1], p.data[2], p.data[3],
				p.data[242*4], p.data[242*4+1], p.data[242*4+2], p.data[242*4+3],
				p.data[254*4], p.data[254*4+1], p.data[254*4+2], p.data[254*4+3])
		}
		texDbg := ""
		if t, ok := r.curTex.(*swTexture); ok && t != nil && int64(len(t.data)) >= int64(t.width)*int64(t.height) {
			mid := int(t.width * (t.height / 2))
			texDbg = fmt.Sprintf(" tex[0]=%d tex[mid]=%d tex[mid+1]=%d tex[last]=%d",
				t.data[0], t.data[mid], t.data[mid+1], t.data[len(t.data)-1])
		}
		LogMessage("[SWDBG] Q tex=%s pal=%s flat=%v trapz=%v mask=%d win=(%.0f,%.0f)-(%.0f,%.0f) blend=%v alpha=%.3f eq=%d src=%d dst=%d%s%s%s",
			tdesc, pdesc, r.curIsFlat, r.curIsTrapez, r.curMask, minX, minY, maxX, maxY,
			r.curBlending, r.curAlpha, r.curBlendEq, r.curBlendSrc, r.curBlendDst, flatCol, palDbg, texDbg)
	}

	// Transform the quad strip vertices (x, y, u, v) by projection × modelview.
	// GL33 vertex shader does the same, then the GPU viewport maps NDC to pixels.
	// We replicate the viewport: NDC [-1,1] → window coords [0,w]×[0,h] (y-up).
	mat := r.curProjection.Mul4(r.curModelview)
	hw := float32(r.w) * 0.5
	hh := float32(r.h) * 0.5
	var v [4]swVertex
	for i := 0; i < 4; i++ {
		p := mat.Mul4x1(mgl.Vec4{r.curVerts[i*4], r.curVerts[i*4+1], 0, 1})
		x, y := p.X(), p.Y()
		if p.W() != 0 {
			x /= p.W()
			y /= p.W()
		}
		x = (x + 1) * hw
		y = (y + 1) * hh
		v[i] = swVertex{x, y, r.curVerts[i*4+2], r.curVerts[i*4+3]}
	}

	q := swQuadState{
		isFlat:     r.curIsFlat,
		mask:       r.curMask,
		isTrapez:   r.curIsTrapez,
		x1x2x4x3:   r.curX1x2x4x3,
		tint:       r.curTint,
		add:        r.curAdd,
		mult:       r.curMult,
		alpha:      r.curAlpha,
		gray:       r.curGray,
		hue:        r.curHue,
		neg:        r.curNeg,
		eq:         r.curBlendEq,
		src:        r.curBlendSrc,
		dst:        r.curBlendDst,
		blending:   r.curBlending,
		hasScissor: r.curScissorOn,
		scissor:    r.curScissor,
	}
	if t, ok := r.curTex.(*swTexture); ok {
		q.tex = t
	}
	if p, ok := r.curPal.(*swTexture); ok {
		q.pal = p
	}
	r.rasterizeQuadWindow(&q, v[0], v[1], v[2], v[3])
}

// flushSWQueue rasterizes the deferred sprite queue on the CPU. Called from
// flushSpriteQueueBatched (render_gl33.go) when the software backend is active.
func (r *Renderer_SW) flushSWQueue(queue []SpriteDrawCall) {
	for i := range queue {
		dc := &queue[i]
		q := swQuadState{
			isFlat:     dc.isFlat,
			mask:       dc.mask,
			isTrapez:   dc.isTrapez,
			x1x2x4x3:   dc.x1x2x4x3,
			tint:       dc.tint,
			add:        dc.spfx.add,
			mult:       dc.spfx.mult,
			alpha:      dc.alpha,
			gray:       dc.gray,
			hue:        dc.spfx.hue,
			neg:        dc.spfx.neg,
			eq:         dc.blendEq,
			src:        dc.blendSrc,
			dst:        dc.blendDst,
			blending:   true,
			hasScissor: dc.hasScissor,
			scissor:    dc.scissor,
		}
		if t, ok := dc.tex.(*swTexture); ok {
			q.tex = t
		}
		if p, ok := dc.paltex.(*swTexture); ok {
			q.pal = p
		}
		c := dc.corners
		// Corner -> vertex UV convention: the vertex UVs carry the absolute
		// sub-rect coords {u1,v1,u2,v2}, matching drawQuadsUV and the GLES32
		// instanced shader (v0 = p2 (u2,v2), v1 = p3 (u2,v1), v2 = p1 (u1,v2),
		// v3 = p4 (u1,v1)). Previously the 0/1 corners were paired with a texUV
		// field that the rasterizer never applied, so sub-rect sprites (SFF
		// atlas glyphs) sampled the whole texture.
		uv := dc.uv
		r.rasterizeQuadWindow(&q,
			swVertex{c[2], c[3], uv[2], uv[3]},
			swVertex{c[4], c[5], uv[2], uv[1]},
			swVertex{c[0], c[1], uv[0], uv[3]},
			swVertex{c[6], c[7], uv[0], uv[1]},
		)
	}
	drawCallStats.TotalBatches += len(queue)
}

// ---- Misc ----

func (r *Renderer_SW) ReadPixels(data []uint8, width, height int) {
	// GL-compatible: row 0 is the BOTTOM of the image.
	if width == int(r.w) && height == int(r.h) {
		for y := 0; y < height; y++ {
			src := r.pix[(height-1-y)*r.pitch : (height-1-y)*r.pitch+width*4]
			dst := data[y*width*4 : y*width*4+width*4]
			copy(dst, src[:width*4])
		}
		return
	}
	// Window size differs (letterboxing/resize): nearest-neighbor scale.
	sx := float64(r.w) / float64(width)
	sy := float64(r.h) / float64(height)
	for y := 0; y < height; y++ {
		srcY := int(float64(height-1-y) * sy)
		for x := 0; x < width; x++ {
			srcX := int(float64(x) * sx)
			srcOff := srcY*r.pitch + srcX*4
			dstOff := y*width*4 + x*4
			copy(data[dstOff:dstOff+4], r.pix[srcOff:srcOff+4])
		}
	}
}

func (r *Renderer_SW) PerspectiveProjectionMatrix(angle, aspect, near, far float32) mgl.Mat4 {
	return mgl.Perspective(angle, aspect, near, far)
}

func (r *Renderer_SW) OrthographicProjectionMatrix(left, right, bottom, top, near, far float32) mgl.Mat4 {
	return mgl.Ortho(left, right, bottom, top, near, far)
}

func (r *Renderer_SW) SetVSync(interval int) {
	if r.renderer != nil {
		_ = r.renderer.RenderSetVSync(interval != 0)
	}
}

func (r *Renderer_SW) NewWorkerThread() bool { return false }

func (r *Renderer_SW) LoadCustomSpriteShader(shaderName string, shaderData []byte) uint32 {
	return 0
}

func (r *Renderer_SW) UnloadCustomSpriteShader(shaderName string) {}

func (r *Renderer_SW) SetSpritePipeline(shaderName string) {
	r.curPipeline = shaderName
}

func (r *Renderer_SW) SetCustomUniforms(params [16]float32) {}

func (r *Renderer_SW) NeedsGrabPass() bool { return false }

func (r *Renderer_SW) ResolveBackBuffer() Texture { return nil }
