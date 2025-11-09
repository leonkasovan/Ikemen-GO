//go:build opengles31
package main

import (
	"fmt"
	"os"
	"strings"
	"image"
	"image/draw"
	"io"
	"io/ioutil"

	gl "github.com/leonkasovan/gl/v3.1/gles2"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type FontRenderer_GL struct {
}

var fragmentFontShader = `
#if __VERSION__ >= 130
#define COMPAT_VARYING in
#define COMPAT_ATTRIBUTE in
#define COMPAT_TEXTURE texture
#define COMPAT_FRAGCOLOR FragColor
out vec4 FragColor;
#else
#define COMPAT_VARYING varying
#define COMPAT_ATTRIBUTE attribute
#define COMPAT_TEXTURE texture2D
#define COMPAT_FRAGCOLOR gl_FragColor
#endif

COMPAT_VARYING vec2 fragTexCoord;

uniform sampler2D tex;
uniform vec4 textColor;

void main()
{
    vec4 sampled = vec4(1.0, 1.0, 1.0, COMPAT_TEXTURE(tex, fragTexCoord).r);
    COMPAT_FRAGCOLOR = min(textColor, vec4(1.0, 1.0, 1.0, 1.0)) * sampled;
}` + "\x00"

var vertexFontShader = `
#if __VERSION__ >= 130
#define COMPAT_VARYING out
#define COMPAT_ATTRIBUTE in
#define COMPAT_TEXTURE texture
#else
#define COMPAT_VARYING varying
#define COMPAT_ATTRIBUTE attribute
#define COMPAT_TEXTURE texture2D
#endif

//vertex position
COMPAT_ATTRIBUTE vec2 vert;

//pass through to fragTexCoord
COMPAT_ATTRIBUTE vec2 vertTexCoord;

//window res
uniform vec2 resolution;

//pass to frag
COMPAT_VARYING vec2 fragTexCoord;

void main() {
   // convert the rectangle from pixels to 0.0 to 1.0
   vec2 zeroToOne = vert / resolution;

   // convert from 0->1 to 0->2
   vec2 zeroToTwo = zeroToOne * 2.0;

   // convert from 0->2 to -1->+1 (clipspace)
   vec2 clipSpace = zeroToTwo - 1.0;

   fragTexCoord = vertTexCoord;

   gl_Position = vec4(clipSpace * vec2(1, -1), 0, 1);
}` + "\x00"

func (r *FontRenderer_GL) Init() {
}

// LoadFont loads the specified font at the given scale.
func (r *FontRenderer_GL) LoadFont(file string, scale int32, windowWidth int, windowHeight int) (interface{}, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	// Configure the default font vertex and fragment shaders
	program, err := r.newProgram(vertexFontShader, fragmentFontShader)
	if err != nil {
		panic(err)
	}

	// Activate corresponding render state
	gl.UseProgram(program)

	//set screen resolution
	resUniform := gl.GetUniformLocation(program, gl.Str("resolution\x00"))
	gl.Uniform2f(resUniform, float32(windowWidth), float32(windowHeight))

	return r.LoadTrueTypeFont(program, fd, scale, 32, 127, LeftToRight)
}

// SetColor allows you to set the text color to be used when you draw the text
func (f *Font_GL) SetColor(red float32, green float32, blue float32, alpha float32) {
	f.color.r = red
	f.color.g = green
	f.color.b = blue
	f.color.a = alpha
}

func (f *Font_GL) UpdateResolution(windowWidth int, windowHeight int) {
	gl.UseProgram(f.program)
	resUniform := gl.GetUniformLocation(f.program, gl.Str("resolution\x00"))
	gl.Uniform2f(resUniform, float32(windowWidth), float32(windowHeight))
	gl.UseProgram(0)
}

// Printf draws a string to the screen, takes a list of arguments like printf
func (f *Font_GL) Printf(x, y float32, scale float32, align int32, blend bool, window [4]int32, fs string, argv ...interface{}) error {

	indices := []rune(fmt.Sprintf(fs, argv...))

	if len(indices) == 0 {
		return nil
	}

	// Buffer to store vertex data for multiple glyphs
	var batchVertices []float32
	var batchChars []*character
	//setup blending mode
	gl.Enable(gl.BLEND)
	if blend {
		gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	}

	//restrict drawing to a certain part of the window
	gl.Enable(gl.SCISSOR_TEST)
	gl.Scissor(window[0], window[1], window[2], window[3])

	// Activate corresponding render state
	gl.UseProgram(f.program)
	//set text color
	gl.Uniform4f(gl.GetUniformLocation(f.program, gl.Str("textColor\x00")), f.color.r, f.color.g, f.color.b, f.color.a)
	//set screen resolution
	//resUniform := gl.GetUniformLocation(f.program, gl.Str("resolution\x00"))
	//gl.Uniform2f(resUniform, float32(2560), float32(1440))

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindVertexArray(f.vao)

	//calculate alignment position
	if align == 0 {
		x -= f.Width(scale, fs, argv...) * 0.5
	} else if align < 0 {
		x -= f.Width(scale, fs, argv...)
	}

	// Iterate through all characters in string
	for i := range indices {

		//get rune
		runeIndex := indices[i]

		//find rune in fontChar list
		ch, ok := f.fontChar[runeIndex]

		//load missing runes in batches of 32
		if !ok {
			low := runeIndex - (runeIndex % 32)
			f.GenerateGlyphs(low, low+31)
			ch, ok = f.fontChar[runeIndex]
		}

		//skip runes that are not in font chacter range
		if !ok {
			//fmt.Printf("%c %d\n", runeIndex, runeIndex)
			continue
		}

		//calculate position and size for current rune
		xpos := x + float32(ch.bearingH)*scale
		ypos := y - float32(ch.height-ch.bearingV)*scale
		w := float32(ch.width) * scale
		h := float32(ch.height) * scale
		vertices := []float32{
			xpos + w, ypos, 1.0, 0.0,
			xpos, ypos, 0.0, 0.0,
			xpos, ypos + h, 0.0, 1.0,

			xpos, ypos + h, 0.0, 1.0,
			xpos + w, ypos + h, 1.0, 1.0,
			xpos + w, ypos, 1.0, 0.0,
		}
		// Append glyph vertices to the batch buffer
		batchVertices = append(batchVertices, vertices...)
		batchChars = append(batchChars, ch)

		// Now advance cursors for next glyph (note that advance is number of 1/64 pixels)
		x += float32((ch.advance >> 6)) * scale // Bitshift by 6 to get value in pixels (2^6 = 64 (divide amount of 1/64th pixels by 64 to get amount of pixels))
	}

	// Render any remaining glyphs in the batch
	if len(batchVertices) > 0 {
		f.renderGlyphBatch(batchChars, indices, batchVertices)
	}

	//clear opengl textures and programs
	gl.BindVertexArray(0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
	gl.UseProgram(0)
	gl.Disable(gl.BLEND)
	gl.Disable(gl.SCISSOR_TEST)

	return nil
}

// Helper function to render a batch of glyphs
func (f *Font_GL) renderGlyphBatch(batchChars []*character, indices []rune, vertices []float32) {
	// Bind the buffer and update its data
	gl.BindBuffer(gl.ARRAY_BUFFER, f.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.DYNAMIC_DRAW)

	// Iterate over each glyph in the batch
	for i := 0; i < len(vertices)/24; i++ {
		// Determine the texture ID for the current glyph
		textureID := batchChars[i].textureID

		// Bind the texture
		gl.BindTexture(gl.TEXTURE_2D, textureID)

		// Render the current glyph
		gl.DrawArrays(gl.TRIANGLES, int32(i*6), 6)
	}

	// Unbind the buffer and texture
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindTexture(gl.TEXTURE_2D, 0)
}

// Width returns the width of a piece of text in pixels
func (f *Font_GL) Width(scale float32, fs string, argv ...interface{}) float32 {

	var width float32

	indices := []rune(fmt.Sprintf(fs, argv...))

	if len(indices) == 0 {
		return 0
	}

	// Iterate through all characters in string
	for i := range indices {

		//get rune
		runeIndex := indices[i]

		//find rune in fontChar list
		ch, ok := f.fontChar[runeIndex]

		//load missing runes in batches of 32
		if !ok {
			low := runeIndex & rune(32-1)
			f.GenerateGlyphs(low, low+31)
			ch, ok = f.fontChar[runeIndex]
		}

		//skip runes that are not in font chacter range
		if !ok {
			//fmt.Printf("%c %d\n", runeIndex, runeIndex)
			continue
		}

		// Now advance cursors for next glyph (note that advance is number of 1/64 pixels)
		width += float32((ch.advance >> 6)) * scale // Bitshift by 6 to get value in pixels (2^6 = 64 (divide amount of 1/64th pixels by 64 to get amount of pixels))

	}

	return width
}

// newProgram links the frag and vertex shader programs
func (r *FontRenderer_GL) newProgram(vertexShaderSource, fragmentShaderSource string) (uint32, error) {
	vertexShaderSource = "#version 300 es\nprecision mediump float;\n" + vertexShaderSource
	fragmentShaderSource = "#version 300 es\nprecision mediump float;\n" + fragmentShaderSource
	compileShader := func(source string, shaderType uint32) (uint32, error) {
		shader := gl.CreateShader(shaderType)

		csources, free := gl.Strs(source)
		gl.ShaderSource(shader, 1, csources, nil)
		free()
		gl.CompileShader(shader)

		var status int32
		gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
		if status == gl.FALSE {
			var logLength int32
			gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

			log := strings.Repeat("\x00", int(logLength+1))
			gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))

			return 0, fmt.Errorf("%v\nfailed to compile Font shader %v: %v", gl.GoStr(gl.GetString(gl.SHADING_LANGUAGE_VERSION)), source, log)
		}

		return shader, nil
	}

	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}

	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return 0, err
	}

	program := gl.CreateProgram()

	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)

		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))

		return 0, fmt.Errorf("%v\nfailed to link program: %v", gl.GoStr(gl.GetString(gl.SHADING_LANGUAGE_VERSION)), log)
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	return program, nil
}

type Font_GL struct {
	fontChar map[rune]*character
	ttf      *truetype.Font
	scale    int32
	vao      uint32
	vbo      uint32
	program  uint32
	texture  uint32 // Holds the glyph texture id.
	color    color
}

// GenerateGlyphs builds a set of textures based on a ttf files gylphs
func (f *Font_GL) GenerateGlyphs(low, high rune) error {
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
		var texture uint32
		gl.GenTextures(1, &texture)
		gl.BindTexture(gl.TEXTURE_2D, texture)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
		gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
		gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA, int32(rgba.Rect.Dx()), int32(rgba.Rect.Dy()), 0,
			gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(rgba.Pix))

		char.textureID = texture

		//add char to fontChar list
		f.fontChar[ch] = char
	}

	gl.BindTexture(gl.TEXTURE_2D, 0)
	return nil
}

// LoadTrueTypeFont builds OpenGL buffers and glyph textures based on a ttf file
func (r *FontRenderer_GL) LoadTrueTypeFont(program uint32, reader io.Reader, scale int32, low, high rune, dir Direction) (Font, error) {
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	// Read the truetype font.
	ttf, err := truetype.Parse(data)
	if err != nil {
		return nil, err
	}

	//make Font stuct type
	f := new(Font_GL)
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
	gl.GenVertexArrays(1, &f.vao)
	gl.GenBuffers(1, &f.vbo)
	gl.BindVertexArray(f.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, f.vbo)

	gl.BufferData(gl.ARRAY_BUFFER, 6*4*4, nil, gl.STATIC_DRAW)

	vertAttrib := uint32(gl.GetAttribLocation(f.program, gl.Str("vert\x00")))
	gl.EnableVertexAttribArray(vertAttrib)
	gl.VertexAttribPointer(vertAttrib, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	defer gl.DisableVertexAttribArray(vertAttrib)

	texCoordAttrib := uint32(gl.GetAttribLocation(f.program, gl.Str("vertTexCoord\x00")))
	gl.EnableVertexAttribArray(texCoordAttrib)
	gl.VertexAttribPointer(texCoordAttrib, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))
	defer gl.DisableVertexAttribArray(texCoordAttrib)

	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindVertexArray(0)

	return f, nil
}