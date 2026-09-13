//go:build !windows && (desktop || mugen)

package main

// selectRendererDX returns nil for non-Windows platforms.
// Direct3D 11 is only available on Windows.
func selectRendererDX() (Renderer, FontRenderer) {
	return nil, nil
}
