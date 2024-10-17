//go:build gl

package glfont

import (
	"fmt"
	"os"

	gl "github.com/go-gl/gl/v2.1/gl"
)

// Direction represents the direction in which strings should be rendered.
type Direction uint8

// Known directions.
const (
	LeftToRight Direction = iota // E.g.: Latin
	RightToLeft                  // E.g.: Arabic
	TopToBottom                  // E.g.: Chinese
)

type color struct {
	r float32
	g float32
	b float32
	a float32
}

// LoadFont loads the specified font at the given scale.
func LoadFont(file string, scale int32, windowWidth int, windowHeight int, GLSLVersion uint) (*Font, error) {
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	// Configure the default font vertex and fragment shaders
	program, err := newProgram(GLSLVersion, vertexFontShader, fragmentFontShader)
	if err != nil {
		panic(err)
	}

	// Activate corresponding render state
	gl.UseProgram(program)

	//set screen resolution
	resUniform := gl.GetUniformLocation(program, gl.Str("resolution\x00"))
	gl.Uniform2f(resUniform, float32(windowWidth), float32(windowHeight))

	return LoadTrueTypeFont(program, fd, scale, 32, 127, LeftToRight)
}

// SetColor allows you to set the text color to be used when you draw the text
func (f *Font) SetColor(red float32, green float32, blue float32, alpha float32) {
	f.color.r = red
	f.color.g = green
	f.color.b = blue
	f.color.a = alpha
}

func (f *Font) UpdateResolution(windowWidth int, windowHeight int) {
	gl.UseProgram(f.program)
	resUniform := gl.GetUniformLocation(f.program, gl.Str("resolution\x00"))
	gl.Uniform2f(resUniform, float32(windowWidth), float32(windowHeight))
	gl.UseProgram(0)
}

func (f *Font) SetBatchMode(batchMode bool) {
	f.batchMode = batchMode
}

// Printf draws a string to the screen, takes a list of arguments like printf
func (f *Font) Printf(x, y float32, scale float32, align int32, blend bool, window [4]int32, fs string, argv ...interface{}) error {

	indices := []rune(fmt.Sprintf(fs, argv...))

	if len(indices) == 0 {
		return nil
	}

	// Buffer to store vertex data for multiple glyphs
	var batchVertices []float32
	var batchChars []*character

	if !f.batchMode {
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
	}
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
		// xpos := x + float32(ch.bearingH)*scale
		// ypos := y - float32(ch.height-ch.bearingV)*scale
		// w := float32(ch.width) * scale
		// h := float32(ch.height) * scale
		// vertices := []float32{
		// 	xpos + w, ypos, 1.0, 0.0,
		// 	xpos, ypos, 0.0, 0.0,
		// 	xpos, ypos + h, 0.0, 1.0,

		// 	xpos, ypos + h, 0.0, 1.0,
		// 	xpos + w, ypos + h, 1.0, 1.0,
		// 	xpos + w, ypos, 1.0, 0.0,
		// }
		// Example for adjusting a single character's vertices with UV coordinates
		xpos := x + float32(ch.bearingH)*scale
		ypos := y - (float32(ch.height)-float32(ch.bearingV))*scale
		w := float32(ch.width) * scale
		h := float32(ch.height) * scale

		uvX := ch.uvX
		uvY := ch.uvY
		uvWidth := ch.uvWidth
		uvHeight := ch.uvHeight

		// Calculate actual UV coordinates
		uvLeft := uvX
		uvTop := uvY
		uvRight := uvX + uvWidth
		uvBottom := uvY + uvHeight

		vertices := []float32{
			// First Triangle
			xpos, ypos, uvLeft, uvTop, // Top-Left
			xpos + w, ypos, uvRight, uvTop, // Top-Right
			xpos, ypos + h, uvLeft, uvBottom, // Bottom-Left
			// Second Triangle
			xpos, ypos + h, uvLeft, uvBottom, // Bottom-Left (Repeated)
			xpos + w, ypos, uvRight, uvTop, // Top-Right (Repeated)
			xpos + w, ypos + h, uvRight, uvBottom, // Bottom-Right
		}

		// Append glyph vertices to the batch buffer
		batchVertices = append(batchVertices, vertices...)
		batchChars = append(batchChars, ch)

		// Now advance cursors for next glyph (note that advance is number of 1/64 pixels)
		x += float32((ch.advance >> 6)) * scale // Bitshift by 6 to get value in pixels (2^6 = 64 (divide amount of 1/64th pixels by 64 to get amount of pixels))
	}

	if f.batchMode {
		batchKey := BatchKey{blend, window}
		if _, ok := f.batches[batchKey]; !ok {
			f.batches[batchKey] = make([]*FontBatchData, 0)
		}
		f.batches[batchKey] = append(f.batches[batchKey], &FontBatchData{batchChars, indices, batchVertices, blend, window})
	} else {
		// Render any remaining glyphs in the batch
		if len(batchVertices) > 0 {
			f.renderGlyphBatch(batchVertices) // , indices, batchVertices)
		}
		//clear opengl textures and programs
		gl.BindVertexArray(0)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		gl.UseProgram(0)
		gl.Disable(gl.BLEND)
		gl.Disable(gl.SCISSOR_TEST)
	}
	return nil
}

type BatchKey struct {
	blend  bool
	window [4]int32
}
type FontBatchData struct {
	batchChars []*character
	indices    []rune
	vertices   []float32
	blend      bool
	window     [4]int32
}

func (f *Font) PrintBatch() bool {
	if !f.batchMode {
		return false
	}

	for batchKey, batch := range f.batches {
		vertices := make([]float32, 0)
		for i := 0; i < len(batch); i++ {
			vertices = append(vertices, batch[i].vertices...)
		}
		gl.Enable(gl.BLEND)
		if batchKey.blend {
			gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
		}

		//restrict drawing to a certain part of the window
		gl.Enable(gl.SCISSOR_TEST)
		gl.Scissor(batchKey.window[0], batchKey.window[1], batchKey.window[2], batchKey.window[3])

		// Activate corresponding render state
		gl.UseProgram(f.program)
		//set text color
		gl.Uniform4f(gl.GetUniformLocation(f.program, gl.Str("textColor\x00")), f.color.r, f.color.g, f.color.b, f.color.a)
		//set screen resolution
		//resUniform := gl.GetUniformLocation(f.program, gl.Str("resolution\x00"))
		//gl.Uniform2f(resUniform, float32(2560), float32(1440))

		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindVertexArray(f.vao)

		f.renderGlyphBatch(vertices) //batch.batchChars, batch.indices, batch.vertices)
		gl.BindVertexArray(0)
		gl.BindTexture(gl.TEXTURE_2D, 0)
		gl.UseProgram(0)
		gl.Disable(gl.BLEND)
		gl.Disable(gl.SCISSOR_TEST)
	}
	for i := range f.batches {
		delete(f.batches, i)
	}
	return true
}

func (f *Font) renderGlyphBatch(vertices []float32) {
	// Bind the texture atlas
	gl.ActiveTexture(gl.TEXTURE0) // Ensure TEXTURE0 is active if using multiple textures
	f.atlas.Bind(gl.TEXTURE_2D)   // Bind the texture atlas

	// Bind the buffer and update its data
	gl.BindBuffer(gl.ARRAY_BUFFER, f.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	// Specify how OpenGL should interpret the vertex data
	// Position
	vertAttrib := uint32(gl.GetAttribLocation(f.program, gl.Str("vert\x00")))
	gl.EnableVertexAttribArray(vertAttrib)
	gl.VertexAttribPointer(vertAttrib, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(0))
	defer gl.DisableVertexAttribArray(vertAttrib)

	texCoordAttrib := uint32(gl.GetAttribLocation(f.program, gl.Str("vertTexCoord\x00")))
	gl.EnableVertexAttribArray(texCoordAttrib)
	gl.VertexAttribPointer(texCoordAttrib, 2, gl.FLOAT, false, 4*4, gl.PtrOffset(2*4))
	defer gl.DisableVertexAttribArray(texCoordAttrib)

	// Draw all glyphs in a single draw call
	gl.DrawArrays(gl.TRIANGLES, 0, int32(len(vertices)/4)) // Each vertex has 4 floats

	// Cleanup
	gl.DisableVertexAttribArray(0)
	gl.DisableVertexAttribArray(1)
	gl.BindBuffer(gl.ARRAY_BUFFER, 0)
	gl.BindTexture(gl.TEXTURE_2D, 0) // Unbind the texture atlas if necessary
}

// Width returns the width of a piece of text in pixels
func (f *Font) Width(scale float32, fs string, argv ...interface{}) float32 {

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
