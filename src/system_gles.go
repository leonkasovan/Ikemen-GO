//go:build gles2

package main

import (
	"glfw"

	"github.com/leonkasovan/glfont"
)

func (s *System) loadGfx() {
	gfx = &Renderer_GLES{}
	gfxFont = &glfont.FontRenderer_GLES{}
}

func (s *System) initGfx() {
	glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 0)
	glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
}
