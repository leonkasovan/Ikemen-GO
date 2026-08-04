//go:build !android && !armdevice

package main

import (
	"fmt"
	"image"
	"io"
	"io/ioutil"
	"os"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// FontRenderer_SW renders TTF text straight into the software framebuffer.
// Glyph shapes are rasterized once with freetype (coverage maps), then each
// Printf draws quads with the same math as shaders/font.frag.glsl.
type FontRenderer_SW struct{}

func (r *FontRenderer_SW) Init(renderer interface{}) {}

func (r *FontRenderer_SW) LoadFont(file string, scale int32, windowWidth int, windowHeight int) (interface{}, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	f, err := r.LoadTrueTypeFont(fd, scale, 32, 127)
	if err != nil {
		return nil, err
	}
	f.windowWidth = windowWidth
	f.windowHeight = windowHeight
	return f, nil
}

func (r *FontRenderer_SW) LoadTrueTypeFont(reader io.Reader, scale int32, low, high rune) (*Font_SW, error) {
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

	f := new(Font_SW)
	f.fontChar = make(map[rune]*swGlyph)
	f.ttf = ttf
	f.scale = scale
	f.SetColor(1.0, 1.0, 1.0, 1.0)
	f.SetPalFX(NewShaderPalFX())

	err = f.GenerateGlyphs(low, high)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type swGlyph struct {
	width, height int32 // includes the 2px padding
	bearingH      int32
	bearingV      int32
	advance       int32 // 1/64 px
	cov           []byte
	tex           *swTexture // lazy coverage texture
}

type Font_SW struct {
	fontChar     map[rune]*swGlyph
	ttf          *truetype.Font
	scale        int32
	windowWidth  int
	windowHeight int
	color        color
	shaderPalFX  ShaderPalFX
}

func (f *Font_SW) SetColor(red float32, green float32, blue float32, alpha float32) {
	f.color.r = red
	f.color.g = green
	f.color.b = blue
	f.color.a = alpha
}

func (f *Font_SW) SetPalFX(state ShaderPalFX) {
	f.shaderPalFX = state
}

func (f *Font_SW) UpdateResolution(windowWidth int, windowHeight int) {
	f.windowWidth = windowWidth
	f.windowHeight = windowHeight
}

func (f *Font_SW) Width(scale float32, spacingXAdd float32, fs string, argv ...interface{}) float32 {
	return f.widthRunes([]rune(fmt.Sprintf(fs, argv...)), scale, spacingXAdd)
}

func (f *Font_SW) widthRunes(indices []rune, scale float32, spacingXAdd float32) float32 {
	if len(indices) == 0 {
		return 0
	}
	spacing := spacingXAdd * scale
	var width float32
	renderedAny := false
	for _, runeIndex := range indices {
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

func (f *Font_SW) GenerateGlyphs(low, high rune) error {
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

	// Padding to prevent cropping (see font_gl33.go).
	padding := 2

	for ch := low; ch <= high; ch++ {
		g := new(swGlyph)

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

		g.width = gw + int32(padding*2)
		g.height = gh + int32(padding*2)
		g.advance = int32(gAdv)
		g.bearingV = int32(gBnd.Max.Y) >> 6
		g.bearingH = int32((int(gBnd.Min.X)>>6) - padding)

		alpha := image.NewAlpha(image.Rect(0, 0, int(g.width), int(g.height)))
		px := padding - (int(gBnd.Min.X) >> 6)
		py := padding + gAscent
		c.SetClip(alpha.Bounds())
		c.SetDst(alpha)
		c.SetSrc(image.White)
		if drawGlyph {
			if _, err := c.DrawString(string(ch), freetype.Pt(px, py)); err != nil {
				c2 := freetype.NewContext()
				c2.SetDPI(72)
				c2.SetFont(f.ttf)
				c2.SetFontSize(float64(f.scale))
				c2.SetHinting(font.HintingNone)
				c2.SetClip(alpha.Bounds())
				c2.SetDst(alpha)
				c2.SetSrc(image.White)
				if _, err := c2.DrawString(string(ch), freetype.Pt(px, py)); err != nil {
					return err
				}
			}
		}
		g.cov = alpha.Pix
		f.fontChar[ch] = g
	}
	return nil
}

// Printf draws a string into the framebuffer. Text coordinates are in "text
// space" (top-left origin, resolution = windowWidth × windowHeight), matching
// the GL font pipeline.
func (f *Font_SW) Printf(x, y float32, xscl, yscl float32, spacingXAdd float32,
	align int32, blend bool, window [4]int32,
	rxadd float32, rot Rotation, projectionMode int32, fLength float32, rcx, rcy float32,
	fs string, argv ...interface{}) error {

	text := fmt.Sprintf(fs, argv...)
	indices := []rune(text)
	if len(indices) == 0 {
		return nil
	}

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

		gfx.(*Renderer_SW).drawFontGlyph(f, ch, x1, y1, x2, y2, x3, y3, x4, y4, window, blend)

		x += float32(ch.advance>>6) * xscl
		renderedAny = true
	}
	return nil
}

// drawFontGlyph rasterizes one glyph quad (text space, y down) into the
// framebuffer, replicating font.frag.glsl.
func (r *Renderer_SW) drawFontGlyph(f *Font_SW, g *swGlyph,
	x1, y1, x2, y2, x3, y3, x4, y4 float32, window [4]int32, blend bool) {

	if g.tex == nil {
		t := r.newTexture(g.width, g.height, 8, true).(*swTexture)
		t.SetData(g.cov)
		g.tex = t
	}

	rw := float32(f.windowWidth)
	rh := float32(f.windowHeight)
	if rw < 1 {
		rw = 1
	}
	if rh < 1 {
		rh = 1
	}
	scaleX := float32(r.w) / rw
	scaleY := float32(r.h) / rh
	toWin := func(tx, ty float32) (float32, float32) {
		return tx * scaleX, (rh - ty) * scaleY
	}

	q := swQuadState{
		isFlat:     false,
		mask:       0,
		texUV:      [4]float32{0, 0, 1, 1},
		fontMode:   true,
		fontCov:    g.tex,
		fontColor:  [4]float32{Min(f.color.r, 1), Min(f.color.g, 1), Min(f.color.b, 1), Min(f.color.a, 1)},
		add:        f.shaderPalFX.add,
		mult:       f.shaderPalFX.mult,
		alpha:      1,
		gray:       f.shaderPalFX.gray,
		hue:        f.shaderPalFX.hue,
		neg:        f.shaderPalFX.neg,
		blending:   true,
		hasScissor: true,
		scissor:    window,
	}
	mode := swBlendAddAlphaOver
	if !blend {
		mode = swBlendReplace
	}

	ax, ay := toWin(x1, y1)
	bx, by := toWin(x2, y2)
	cx, cy := toWin(x3, y3)
	dx, dy := toWin(x4, y4)
	// Vertex order follows drawQuadsUV's convention: (x2,y2) (x3,y3) (x1,y1)
	// (x4,y4). That splits the quad cleanly along a diagonal so the two strip
	// triangles (v0,v1,v2) / (v1,v2,v3) cover it exactly once. Passing the
	// corners in top-pair-first order (x1,y1) (x2,y2) (x3,y3) (x4,y4) makes the
	// triangles overlap in a wedge, double-blending glyph coverage and making
	// text render visibly thicker/darker on its upper-left side.
	r.rasterGeneric(&q,
		swVertex{bx, by, 0, 0}, // (x2,y2) top-left
		swVertex{cx, cy, 0, 1}, // (x3,y3) bottom-left
		swVertex{ax, ay, 1, 0}, // (x1,y1) top-right
		swVertex{dx, dy, 1, 1}, // (x4,y4) bottom-right
		mode, nil)
}
