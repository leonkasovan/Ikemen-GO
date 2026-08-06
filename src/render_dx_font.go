//go:build windows && !android && !armdevice

package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"math"
	"os"
	"unsafe"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

/*
#include <d3d11.h>
void* dx_create_cb(void* device, int size);
void* dx_create_vb(void* device, int size);
void dx_update_cb(void* ctx, void* cb, const void* data, int size);
void* dx_map_vb(void* ctx, void* vb);
void dx_unmap_vb(void* ctx, void* vb);
void dx_draw(void* ctx, int count, int start);
const char* dx_last_error(void);
void dx_release(void* obj);
*/
import "C"

type Font_DX struct {
	fontChar      map[rune]*character
	ttf           *truetype.Font
	scale         int32
	windowWidth   int
	windowHeight  int
	textures      []*TextureAtlas
	color         color
	shaderPalFX   ShaderPalFX
	batchVertices []float32
}

type FontRenderer_DX struct {
	vs, ps        unsafe.Pointer
	cb            unsafe.Pointer
	vb            unsafe.Pointer
	uniforms      dxFontUniforms
	cbDirty       bool
	vertexScratch []byte
}

func (r *FontRenderer_DX) Init(renderer interface{}) {
	rr := gfx.(*Renderer_DX)
	var blob unsafe.Pointer
	r.vs, blob = rr.compileShader(dxFontVS, true)
	C.dx_release(blob)
	r.ps, blob = rr.compileShader(dxFontPS, false)
	C.dx_release(blob)
	r.cb = C.dx_create_cb(rr.device, C.int(unsafe.Sizeof(dxFontUniforms{})))
	chkRes(r.cb)
	r.vb = C.dx_create_vb(rr.device, C.int(MaxFontBatchSize*6*4*4))
	chkRes(r.vb)
}

func (r *FontRenderer_DX) LoadFont(file string, scale int32, windowWidth int, windowHeight int) (interface{}, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	f, err := r.LoadTrueTypeFont(fd, scale, 32, 127, LeftToRight)
	if err != nil {
		return nil, err
	}
	f.windowWidth = windowWidth
	f.windowHeight = windowHeight
	return f, nil
}

func (f *Font_DX) SetColor(red float32, green float32, blue float32, alpha float32) {
	f.color.r = red
	f.color.g = green
	f.color.b = blue
	f.color.a = alpha
}

func (f *Font_DX) SetPalFX(state ShaderPalFX) {
	f.shaderPalFX = state
}

func (f *Font_DX) UpdateResolution(windowWidth int, windowHeight int) {
	f.windowWidth = windowWidth
	f.windowHeight = windowHeight
}

func (r *FontRenderer_DX) setFontPipeline() {
	rr := gfx.(*Renderer_DX)
	rr.bindShaders(r.vs, r.ps)
	rr.bindIL(rr.ilSprite)
	rr.bindTopology(dxTopoList)
	rr.bindVB(r.vb, 16)
}

func (r *FontRenderer_DX) flushUniforms() {
	if r.cbDirty {
		C.dx_update_cb(gfx.(*Renderer_DX).ctx, r.cb, unsafe.Pointer(&r.uniforms), C.int(unsafe.Sizeof(dxFontUniforms{})))
		r.cbDirty = false
	}
	gfx.(*Renderer_DX).bindCB(r.cb)
}

func (f *Font_DX) Printf(x, y float32, xscl, yscl float32, spacingXAdd float32,
	align int32, blend bool, window [4]int32,
	rxadd float32, rot Rotation, projectionMode int32, fLength float32, rcx, rcy float32,
	fs string, argv ...interface{}) error {

	text := fmt.Sprintf(fs, argv...)
	indices := []rune(text)
	r := gfx.(*Renderer_DX)
	fr := gfxFont.(*FontRenderer_DX)

	if len(indices) == 0 {
		return nil
	}

	fr.setFontPipeline()

	batchSize := Min(MaxFontBatchSize, int32(len(indices)))
	if cap(f.batchVertices) < int(batchSize*6*4) {
		f.batchVertices = make([]float32, 0, batchSize*6*4)
	} else {
		f.batchVertices = f.batchVertices[:0]
	}
	batchVertices := f.batchVertices

	if blend {
		r.EnableBlending(BlendAdd, BlendSrcAlpha, BlendOneMinusSrcAlpha)
	} else {
		r.DisableBlending()
	}

	r.EnableScissor(window[0], window[1], window[2], window[3])

	fr.cbDirty = true
	fr.uniforms.textColor = [4]float32{f.color.r, f.color.g, f.color.b, f.color.a}
	fr.uniforms.palAddGray = [4]float32{f.shaderPalFX.add[0], f.shaderPalFX.add[1], f.shaderPalFX.add[2], f.shaderPalFX.gray}
	fr.uniforms.palMulHue = [4]float32{f.shaderPalFX.mult[0], f.shaderPalFX.mult[1], f.shaderPalFX.mult[2], f.shaderPalFX.hue}
	fr.uniforms.palNeg = [4]float32{float32(Btoi(f.shaderPalFX.neg)), 0, 0, 0}
	fr.uniforms.resolution = [4]float32{float32(f.windowWidth), float32(f.windowHeight), 0, 0}

	alignScale := xscl
	if alignScale == 0 {
		alignScale = yscl
	}
	if align == 0 {
		x -= f.widthRunes(indices, alignScale, spacingXAdd) * 0.5
	} else if align < 0 {
		x -= f.widthRunes(indices, alignScale, spacingXAdd)
	}
	needsTransform := rxadd != 0 || !rot.IsZero()
	textureIndex := int32(-1)
	spacing := spacingXAdd * xscl
	renderedAny := false
	for i := range indices {
		runeIndex := indices[i]

		ch, ok := f.fontChar[runeIndex]

		if !ok {
			low := runeIndex - (runeIndex % 32)
			f.GenerateGlyphs(low, low+31)
			ch, ok = f.fontChar[runeIndex]
		}

		if !ok {
			continue
		}

		if int32(len(batchVertices)/24) >= batchSize || (textureIndex != -1 && textureIndex != int32(ch.textureID)) {
			f.renderGlyphBatch(batchVertices, uint32(textureIndex))
			batchVertices = batchVertices[:0]
		}
		textureIndex = int32(ch.textureID)

		if renderedAny {
			x += spacing
		}

		xpos := x + float32(ch.bearingH)*xscl
		ypos := y - float32(ch.height-ch.bearingV)*yscl
		w := float32(ch.width) * xscl
		h := float32(ch.height) * yscl

		x1, y1 := xpos+w, ypos
		x2, y2 := xpos, ypos
		x3, y3 := xpos, ypos+h
		x4, y4 := xpos+w, ypos+h
		if needsTransform {
			x1, y1, x2, y2, x3, y3, x4, y4 = transformTextQuad(
				x1, y1, x2, y2, x3, y3, x4, y4,
				rxadd, rot, projectionMode, fLength, rcx, rcy,
			)
		}

		batchVertices = append(batchVertices,
			x1, y1, ch.uv[2], ch.uv[1],
			x2, y2, ch.uv[0], ch.uv[1],
			x3, y3, ch.uv[0], ch.uv[3],

			x3, y3, ch.uv[0], ch.uv[3],
			x4, y4, ch.uv[2], ch.uv[3],
			x1, y1, ch.uv[2], ch.uv[1],
		)
		x += float32((ch.advance >> 6)) * xscl
		renderedAny = true
	}

	if len(batchVertices) > 0 {
		f.renderGlyphBatch(batchVertices, uint32(textureIndex))
	}

	r.DisableScissor()

	return nil
}

func (f *Font_DX) widthRunes(indices []rune, scale float32, spacingXAdd float32) float32 {
	if len(indices) == 0 {
		return 0
	}

	spacing := spacingXAdd * scale
	var width float32
	renderedAny := false

	for i := range indices {
		runeIndex := indices[i]

		ch, ok := f.fontChar[runeIndex]

		if !ok {
			low := runeIndex - (runeIndex % 32)
			f.GenerateGlyphs(low, low+31)
			ch, ok = f.fontChar[runeIndex]
		}

		if !ok {
			continue
		}

		if renderedAny {
			width += spacing
		}

		width += float32(ch.advance>>6) * scale
		renderedAny = true
	}

	return width
}

func (f *Font_DX) renderGlyphBatch(vertices []float32, textureIndex uint32) {
	fr := gfxFont.(*FontRenderer_DX)
	r := gfx.(*Renderer_DX)

	fr.flushUniforms()

	needed := len(vertices) * 4
	if cap(fr.vertexScratch) < needed {
		fr.vertexScratch = make([]byte, needed)
	}
	buf := fr.vertexScratch[:needed]
	for i, v := range vertices {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	p := C.dx_map_vb(r.ctx, fr.vb)
	if p == nil {
		panic("Direct3D 11: " + C.GoString(C.dx_last_error()))
	}
	copy(unsafe.Slice((*byte)(p), needed), buf)
	C.dx_unmap_vb(r.ctx, fr.vb)

	t := f.textures[textureIndex].texture.(*Texture_DX)
	r.bindSRV(0, t.srv)
	r.bindSampler(0, t.sampler)
	C.dx_draw(r.ctx, C.int(len(vertices)/4), 0)
}

func (f *Font_DX) Width(scale float32, spacingXAdd float32, fs string, argv ...interface{}) float32 {
	return f.widthRunes([]rune(fmt.Sprintf(fs, argv...)), scale, spacingXAdd)
}

func (f *Font_DX) GenerateGlyphs(low, high rune) error {
	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(f.ttf)
	c.SetFontSize(float64(f.scale))
	c.SetHinting(font.HintingFull)

	ttfFace := truetype.NewFace(f.ttf, &truetype.Options{
		Size:    float64(f.scale),
		DPI:     72,
		Hinting: font.HintingFull,
	})

	padding := 2

	for ch := low; ch <= high; ch++ {
		char := new(character)

		drawGlyph := true
		gBnd, gAdv, ok := ttfFace.GlyphBounds(ch)
		if !ok {
			drawGlyph = false
			if adv, advOK := ttfFace.GlyphAdvance(ch); advOK {
				gAdv = adv
			} else {
				gAdv = 0
			}
			gBnd = f.ttf.Bounds(fixed.Int26_6(f.scale))
		}

		gh := int32((gBnd.Max.Y - gBnd.Min.Y) >> 6)
		gw := int32((gBnd.Max.X - gBnd.Min.X) >> 6)

		if gw == 0 || gh == 0 {
			gBnd = f.ttf.Bounds(fixed.Int26_6(f.scale))
			gw = int32((gBnd.Max.X - gBnd.Min.X) >> 6)
			gh = int32((gBnd.Max.Y - gBnd.Min.Y) >> 6)

			if gw == 0 || gh == 0 {
				gw = 1
				gh = 1
			}
		}

		gAscent := int(-gBnd.Min.Y) >> 6
		gdescent := int(gBnd.Max.Y) >> 6

		char.width = int(gw) + (padding * 2)
		char.height = int(gh) + (padding * 2)
		char.advance = int(gAdv)
		char.bearingV = gdescent
		char.bearingH = (int(gBnd.Min.X) >> 6) - padding

		fg := image.White
		rect := image.Rect(0, 0, char.width, char.height)
		rgba := image.NewRGBA(rect)

		px := padding - (int(gBnd.Min.X) >> 6)
		py := padding + gAscent
		pt := freetype.Pt(px, py)

		c.SetClip(rgba.Bounds())
		c.SetDst(rgba)
		c.SetSrc(fg)
		if drawGlyph {
			if _, err := c.DrawString(string(ch), pt); err != nil {
				c2 := freetype.NewContext()
				c2.SetDPI(72)
				c2.SetFont(f.ttf)
				c2.SetFontSize(float64(f.scale))
				c2.SetHinting(font.HintingNone)
				c2.SetClip(rgba.Bounds())
				c2.SetDst(rgba)
				c2.SetSrc(fg)
				if _, err := c2.DrawString(string(ch), pt); err != nil {
					return err
				}
			}
		}

		var uv [4]float32
		textureIndex := 0
		w, h := int32(rgba.Rect.Dx()), int32(rgba.Rect.Dy())
		pix := rgba.Pix
		stride := int32(rgba.Stride)

		for {
			if textureIndex >= len(f.textures) {
				f.textures = append(f.textures, CreateTextureAtlas(256, 256, 32, true))
				memLog("Font atlas created: index=%d total=%d", len(f.textures)-1, len(f.textures))
			}

			var inserted bool
			uv, inserted = f.textures[textureIndex].AddImage(w, h, stride, pix)

			if inserted {
				break
			}

			textureIndex++
		}

		char.uv = uv
		char.textureID = uint32(textureIndex)

		f.fontChar[ch] = char
	}

	memGlyphs(low, high, len(f.fontChar), len(f.textures))
	return nil
}

func (r *FontRenderer_DX) LoadTrueTypeFont(reader io.Reader, scale int32, low, high rune, dir Direction) (*Font_DX, error) {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	ttf, err := truetype.Parse(data)
	if err != nil && err.Error() == "freetype: invalid TrueType format: bad kern table length" {
		ttf, err = truetype.Parse(stripKernTable(data))
	}
	if err != nil {
		return nil, err
	}

	f := new(Font_DX)
	f.fontChar = make(map[rune]*character)
	f.ttf = ttf
	f.scale = scale
	f.SetColor(1.0, 1.0, 1.0, 1.0)
	f.SetPalFX(NewShaderPalFX())
	f.textures = append(f.textures, CreateTextureAtlas(256, 256, 32, true))

	err = f.GenerateGlyphs(low, high)
	if err != nil {
		return nil, err
	}

	return f, nil
}
