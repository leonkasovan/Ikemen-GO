//go:build required
// +build required

// Package dummy prevents go tooling from stripping the c dependencies.
package dummy

import (
	// Prevent go tooling from stripping out the c source files.
	_ "glfw/v3.5/glfw/glfw/deps/glad"
	_ "glfw/v3.5/glfw/glfw/deps/mingw"
	_ "glfw/v3.5/glfw/glfw/deps/wayland"
)
