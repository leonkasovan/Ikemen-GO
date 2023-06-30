package glfont

import (
	"fmt"
	"image"
	"image/draw"
	"io"
	"io/ioutil"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	gl "github.com/ikemen-engine/Ikemen-GO/dhaninovan/gl-js"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// A Font allows rendering of text to an OpenGL context.
type Font struct {
	fontChar map[rune]*character
	ttf      *truetype.Font
	scale    int32
	vao      uint32
	vbo      gl.Buffer
	program  gl.Program
	texture  gl.Texture // Holds the glyph texture id.
	color    color
}

type character struct {
	textureID gl.Texture // ID handle of the glyph texture
	width     int        //glyph width
	height    int        //glyph height
	advance   int        //glyph advance
	bearingH  int        //glyph bearing horizontal
	bearingV  int        //glyph bearing vertical
}

// GenerateGlyphs builds a set of textures based on a ttf files gylphs
func (f *Font) GenerateGlyphs(low, high rune) error {
	//create a freetype context for drawing
	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetFont(f.ttf)
	c.SetFontSize(float64(f.scale))
	c.SetHinting(font.HintingFull)

	//create new face to measure glyph dimensions
	ttfFace := truetype.NewFace(f.ttf, &truetype.Options{
		Size:    float64(f.scale),
		DPI:     72,
		Hinting: font.HintingFull,
	})

	//make each gylph
	for ch := low; ch <= high; ch++ {
		char := new(character)

		gBnd, gAdv, ok := ttfFace.GlyphBounds(ch)
		if ok != true {
			return fmt.Errorf("ttf face glyphBounds error")
		}

		gh := int32((gBnd.Max.Y - gBnd.Min.Y) >> 6)
		gw := int32((gBnd.Max.X - gBnd.Min.X) >> 6)

		//if gylph has no dimensions set to a max value
		if gw == 0 || gh == 0 {
			gBnd = f.ttf.Bounds(fixed.Int26_6(f.scale))
			gw = int32((gBnd.Max.X - gBnd.Min.X) >> 6)
			gh = int32((gBnd.Max.Y - gBnd.Min.Y) >> 6)

			//above can sometimes yield 0 for font smaller than 48pt, 1 is minimum
			if gw == 0 || gh == 0 {
				gw = 1
				gh = 1
			}
		}

		//The glyph's ascent and descent equal -bounds.Min.Y and +bounds.Max.Y.
		gAscent := int(-gBnd.Min.Y) >> 6
		gdescent := int(gBnd.Max.Y) >> 6

		//set w,h and adv, bearing V and bearing H in char
		char.width = int(gw)
		char.height = int(gh)
		char.advance = int(gAdv)
		char.bearingV = gdescent
		char.bearingH = (int(gBnd.Min.X) >> 6)

		//create image to draw glyph
		fg, bg := image.White, image.Black
		rect := image.Rect(0, 0, int(gw), int(gh))
		rgba := image.NewRGBA(rect)
		draw.Draw(rgba, rgba.Bounds(), bg, image.ZP, draw.Src)

		//set the glyph dot
		px := 0 - (int(gBnd.Min.X) >> 6)
		py := (gAscent)
		pt := freetype.Pt(px, py)

		// Draw the text from mask to image
		c.SetClip(rgba.Bounds())
		c.SetDst(rgba)
		c.SetSrc(fg)
		_, err := c.DrawString(string(ch), pt)
		if err != nil {
			return err
		}

		// Generate texture
		// var texture gl.Texture
		texture := gl.CreateTexture()
		gl.BindTexture(gl.TEXTURE_2D, texture)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexImage2D(gl.TEXTURE_2D, 0, rgba.Rect.Dx(), rgba.Rect.Dy(),
			gl.RGBA, gl.UNSIGNED_BYTE, rgba.Pix)

		char.textureID = texture

		//add char to fontChar list
		f.fontChar[ch] = char
	}

	gl.BindTexture(gl.TEXTURE_2D, gl.Texture{Value: 0})
	return nil
}

// LoadTrueTypeFont builds OpenGL buffers and glyph textures based on a ttf file
func LoadTrueTypeFont(program gl.Program, r io.Reader, scale int32, low, high rune, dir Direction) (*Font, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Read the truetype font.
	ttf, err := truetype.Parse(data)
	if err != nil {
		return nil, err
	}

	//make Font stuct type
	f := new(Font)
	f.fontChar = make(map[rune]*character)
	f.ttf = ttf
	f.scale = scale
	f.program = program            //set shader program
	f.SetColor(1.0, 1.0, 1.0, 1.0) //set default white

	err = f.GenerateGlyphs(low, high)
	if err != nil {
		return nil, err
	}

	// Configure VAO/VBO for texture quads
	f.vao = gl.CreateVertexArray()
	//gl.GenBuffers(1, &f.vbo)
	f.vbo = gl.CreateBuffer()
	gl.BindVertexArray(f.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, f.vbo)

	gl.BufferInit(gl.ARRAY_BUFFER, 6*4*4, gl.STATIC_DRAW)

	vertAttrib := gl.GetAttribLocation(f.program, "vert")
	gl.EnableVertexAttribArray(vertAttrib)
	gl.VertexAttribPointer(vertAttrib, 2, gl.FLOAT, false, 4*4, 0)
	defer gl.DisableVertexAttribArray(vertAttrib)

	texCoordAttrib := gl.GetAttribLocation(f.program, "vertTexCoord")
	gl.EnableVertexAttribArray(texCoordAttrib)
	gl.VertexAttribPointer(texCoordAttrib, 2, gl.FLOAT, false, 4*4, 2*4)
	defer gl.DisableVertexAttribArray(texCoordAttrib)

	gl.BindBuffer(gl.ARRAY_BUFFER, gl.Buffer{Value: 0})
	gl.BindVertexArray(0)

	return f, nil
}
