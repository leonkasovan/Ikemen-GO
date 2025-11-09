//go:build required
// +build required

// Package dummy prevents go tooling from stripping the c dependencies.
package dummy

import (
	// Prevent go tooling from stripping out the c source files.
	_ "github.com/leonkasovan/glfw/v3.5/glfw/glfw/deps/glad"
	_ "github.com/leonkasovan/glfw/v3.5/glfw/glfw/deps/mingw"
	_ "github.com/leonkasovan/glfw/v3.5/glfw/glfw/deps/wayland"
)
