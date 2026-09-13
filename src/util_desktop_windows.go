//go:build windows && (desktop || mugen)

package main

// selectRendererDX returns the Direct3D 11 renderer and font renderer.
// This is Windows-only since Renderer_DX/FontRenderer_DX are Windows-only types.
func selectRendererDX() (Renderer, FontRenderer) {
	return &Renderer_DX{}, &FontRenderer_DX{}
}
