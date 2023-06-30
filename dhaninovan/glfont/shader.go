package glfont

import (
	gl "github.com/ikemen-engine/Ikemen-GO/dhaninovan/gl-js"

	"fmt"
)

// newProgram links the frag and vertex shader programs
func newProgram(GLSLVersion uint, vertexShaderSource, fragmentShaderSource string) (gl.Program, error) {
	vertexShaderSource = fmt.Sprintf("#version %d es\n", GLSLVersion) + vertexShaderSource
	fragmentShaderSource = fmt.Sprintf("#version %d es\n", GLSLVersion) + fragmentShaderSource

	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return gl.Program{Value: 0}, err
	}

	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return gl.Program{Value: 0}, err
	}

	program := gl.CreateProgram()

	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	if gl.GetProgrami(program, gl.LINK_STATUS) == gl.FALSE {
		return gl.Program{Value: 0}, fmt.Errorf("%v\nfailed to link program: %v", gl.GetString(gl.SHADING_LANGUAGE_VERSION), gl.GetProgramInfoLog(program))
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	return program, nil
}

// compileShader compiles the shader program
func compileShader(source string, shaderType uint32) (gl.Shader, error) {
	shader := gl.CreateShader(gl.Enum(shaderType))

	gl.ShaderSource(shader, source)
	gl.CompileShader(shader)

	if gl.GetShaderi(shader, gl.COMPILE_STATUS) == gl.FALSE {
		return gl.Shader{Value: 0}, fmt.Errorf("%v\nfailed to compile %v: %v", gl.GetString(gl.SHADING_LANGUAGE_VERSION), source, gl.GetShaderInfoLog(shader))
	}

	return shader, nil
}

var fragmentFontShader = `
#if __VERSION__ >= 130
precision mediump float;
precision mediump sampler2D;
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
}`

//}` + "\x00"

var vertexFontShader = `
#if __VERSION__ >= 130
precision mediump float;
precision mediump sampler2D;
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
}`

//}` + "\x00"
