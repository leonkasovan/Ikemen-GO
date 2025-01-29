package glfont

import (
	"io"
)

type FontRenderer interface {
	LoadFont(file string, scale int32, windowWidth int, windowHeight int) (Font, error)
	LoadTrueTypeFont(program uint32, r io.Reader, scale int32, low, high rune, dir Direction) (Font, error)
	newProgram(GLSLVersion uint, vertexShaderSource, fragmentShaderSource string) (uint32, error)
}

type Font interface {
	SetColor(red float32, green float32, blue float32, alpha float32)
	UpdateResolution(windowWidth int, windowHeight int)
	Printf(x, y float32, scale float32, align int32, blend bool, window [4]int32, fs string, argv ...interface{}) error
	renderGlyphBatch(batchChars []*character, indices []rune, vertices []float32)
	Width(scale float32, fs string, argv ...interface{}) float32
}

type character struct {
	textureID uint32 // ID handle of the glyph texture
	width     int    //glyph width
	height    int    //glyph height
	advance   int    //glyph advance
	bearingH  int    //glyph bearing horizontal
	bearingV  int    //glyph bearing vertical
}

type color struct {
	r float32
	g float32
	b float32
	a float32
}

// Direction represents the direction in which strings should be rendered.
type Direction uint8

// Known directions.
const (
	LeftToRight Direction = iota // E.g.: Latin
	RightToLeft                  // E.g.: Arabic
	TopToBottom                  // E.g.: Chinese
)

type FontRenderer_GL21 struct {
}

type FontRenderer_GL32 struct {
}

type FontRenderer_GLES struct {
}

type FontRenderer_GLES32 struct {
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
