//go:build gles2

package main

import (
	glfw "github.com/leonkasovan/glfw/v3.5/glfw"

	"github.com/leonkasovan/glfont"
)

func (s *System) loadGfx() {
	if s.cfg.Video.RenderMode == "OpenGL ES 3.2" {
		gfx = &Renderer_GLES32{}
		gfxFont = &glfont.FontRenderer_GLES32{}
	} else {
		gfx = &Renderer_GLES{}
		gfxFont = &glfont.FontRenderer_GLES{}
	}
}

func (s *System) initGfx() {
	glfw.WindowHint(glfw.ClientAPI, glfw.OpenGLESAPI)
	if s.cfg.Video.RenderMode == "OpenGL ES 3.2" {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 2)
	} else {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 0)
	}
	glfw.WindowHint(glfw.ContextCreationAPI, glfw.EGLContextAPI)
}
