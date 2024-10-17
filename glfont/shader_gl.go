//go:build gl

package glfont

import (
	"github.com/go-gl/gl/v2.1/gl"

	"fmt"
	"strings"
)

// newProgram links the frag and vertex shader programs
func newProgram(GLSLVersion uint, vertexShaderSource, fragmentShaderSource string) (uint32, error) {
	vertexShaderSource = fmt.Sprintf("#version %d\n", GLSLVersion) + vertexShaderSource
	fragmentShaderSource = fmt.Sprintf("#version %d\n", GLSLVersion) + fragmentShaderSource

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

// compileShader compiles the shader program
func compileShader(source string, shaderType uint32) (uint32, error) {
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

		return 0, fmt.Errorf("%v\nfailed to compile %v: %v", gl.GoStr(gl.GetString(gl.SHADING_LANGUAGE_VERSION)), source, log)
	}

	return shader, nil
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
