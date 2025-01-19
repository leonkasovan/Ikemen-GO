//go:build !gles2

package main

import (
	glfw "github.com/leonkasovan/glfw/v3.5/glfw"

	"github.com/leonkasovan/glfont"
)

func (s *System) loadGfx() {
	if s.cfg.Video.RenderMode == "OpenGL 2.1" {
		gfx = &Renderer_GL21{}
		gfxFont = &glfont.FontRenderer_GL21{}
	} else {
		gfx = &Renderer_GL32{}
		gfxFont = &glfont.FontRenderer_GL32{}
	}
}

func (s *System) initGfx() {
	if sys.cfg.Video.RenderMode == "OpenGL 3.2" {
		glfw.WindowHint(glfw.ContextVersionMajor, 3)
		glfw.WindowHint(glfw.ContextVersionMinor, 2)
		glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
		glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	} else {
		glfw.WindowHint(glfw.ContextVersionMajor, 2)
		glfw.WindowHint(glfw.ContextVersionMinor, 1)
	}
}
