//go:build required
// +build required

// Package dummy prevents go tooling from stripping the c dependencies.
package dummy

import (
	// Prevent go tooling from stripping out the c source files.
	_ "github.com/ikemen-engine/Ikemen-GO/packages/glfw/glfw/deps/wayland"
)