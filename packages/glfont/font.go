//go:build !gles2

package glfont

import (
	"fmt"

	// "github.com/go-gl/gl/v2.1/gl"
	"github.com/ikemen-engine/Ikemen-GO/packages/physfs"
)

// LoadFont loads the specified font at the given scale.
func (r *FontRenderer_GL21) LoadFont(file string, scale int32, windowWidth int, windowHeight int) (Font, error) {
	fd := physfs.OpenRead(file)
	if fd == nil {
		return nil, fmt.Errorf("can not open font file %s", file)
	}
	defer fd.Close()

	// Configure the default font vertex and fragment shaders
	program, err := r.newProgram(120, vertexFontShader, fragmentFontShader)
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
func (f *Font_GL21) SetColor(red float32, green float32, blue float32, alpha float32) {
	f.color.r = red
	f.color.g = green
	f.color.b = blue
	f.color.a = alpha
}

func (f *Font_GL21) UpdateResolution(windowWidth int, windowHeight int) {
	gl.UseProgram(f.program)
	resUniform := gl.GetUniformLocation(f.program, gl.Str("resolution\x00"))
	gl.Uniform2f(resUniform, float32(windowWidth), float32(windowHeight))
	gl.UseProgram(0)
}

// Printf draws a string to the screen, takes a list of arguments like printf
func (f *Font_GL21) Printf(x, y float32, scale float32, align int32, blend bool, window [4]int32, fs string, argv ...interface{}) error {

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
func (f *Font_GL21) renderGlyphBatch(batchChars []*character, indices []rune, vertices []float32) {
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
func (f *Font_GL21) Width(scale float32, fs string, argv ...interface{}) float32 {

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
