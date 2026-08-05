//go:build !android && !armdevice

package main

import (
	"math"
	"math/rand"
	"testing"

	mgl "github.com/go-gl/mathgl/mgl32"
)

// Self-check for the software rasterizer: constructs synthetic quads and
// verifies the framebuffer pixels. Run with `go test ./src -run TestSW`.

func newSWTestRenderer(w, h int32) *Renderer_SW {
	r := &Renderer_SW{w: w, h: h, pitch: int(w) * 4}
	r.pix = make([]byte, int(w)*int(h)*4)
	return r
}

func swPix(r *Renderer_SW, x, y int) (int, int, int, int) {
	o := y*r.pitch + x*4
	return int(r.pix[o]), int(r.pix[o+1]), int(r.pix[o+2]), int(r.pix[o+3])
}

func swState() *swQuadState {
	return &swQuadState{
		alpha: 1, mult: [3]float32{1, 1, 1},
		blending: true, eq: BlendAdd,
		src: BlendOne, dst: BlendOneMinusSrcAlpha,
	}
}

// mul255 must be byte-exact with the rounded divide-by-255 it replaces. The
// magic multiply ((a*b+127)*32897)>>23 keeps the hot pixel loops division-free;
// if a refactor ever drifts it, alpha-over edges pick up a visible ±1 (the
// "ROUND 1" text regression).
func TestSWMul255Exact(t *testing.T) {
	// The sa² lookup table used by the alpha-over / SrcAlpha-One helpers must
	// match mul255(i, i) for every entry.
	for i := 0; i < 256; i++ {
		if int(mul255SASATab[i]) != mul255(i, i) {
			t.Fatalf("mul255SASATab[%d] = %d, want %d", i, mul255SASATab[i], mul255(i, i))
		}
	}
	for a := 0; a <= 255; a++ {
		for b := 0; b <= 255; b++ {
			if got := mul255(a, b); got != (a*b+127)/255 {
				t.Fatalf("mul255(%d,%d) = %d, want %d", a, b, got, (a*b+127)/255)
			}
		}
	}
}

// Full-screen opaque flat fill must produce the exact color.
func TestSWFlatFill(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	q := swState()
	q.isFlat = true
	q.tint = [4]float32{1, 0.5, 0, 1} // orange, opaque
	r.rasterizeQuadWindow(q,
		swVertex{64, 48, 1, 1}, // bottom-right
		swVertex{64, 0, 1, 0},  // top-right
		swVertex{0, 48, 0, 1},  // bottom-left
		swVertex{0, 0, 0, 0},   // top-left
	)
	for _, p := range [][2]int{{0, 0}, {63, 47}, {32, 24}} {
		rr, gg, bb, _ := swPix(r, p[0], p[1])
		if rr != 255 || gg != 128 || bb != 0 {
			t.Fatalf("flat fill at %v: got (%d,%d,%d), want (255,128,0)", p, rr, gg, bb)
		}
	}
}

// A scaled paletted sprite: 2x2 texture with a 2-entry palette.
func TestSWPalettedSprite(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	tex := &swTexture{width: 2, height: 2, depth: 8, serial: 1}
	tex.data = []byte{0, 1, 1, 0} // checkerboard of indices
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 2}
	pal.data = make([]byte, 1024)
	pal.data[0], pal.data[1], pal.data[2], pal.data[3] = 255, 0, 0, 255 // idx 0: red, opaque
	pal.data[4], pal.data[5], pal.data[6], pal.data[7] = 0, 0, 255, 255 // idx 1: blue, opaque

	q := swState()
	q.tex = tex
	q.pal = pal
	q.mask = -1
	// Draw the sprite at (8, 8) size 32x16 in window coords (y-up). It covers
	// framebuffer rows 24..39. Texture 2x2 = {0,1;1,0} (row 0 on top of the
	// texture, but v=1 at the quad's top edge maps to texture row 1).
	// v at window y=17.5 (py=30) is 0.594 → texture row 1: left=blue, right=red.
	r.rasterizeQuadWindow(q,
		swVertex{40, 24, 1, 1},
		swVertex{40, 8, 1, 0},
		swVertex{8, 24, 0, 1},
		swVertex{8, 8, 0, 0},
	)
	rr, gg, bb, _ := swPix(r, 12, 30)
	if rr != 0 || gg != 0 || bb != 255 {
		t.Fatalf("expected blue (idx 1) at (12,30), got (%d,%d,%d)", rr, gg, bb)
	}
	rr, gg, bb, _ = swPix(r, 30, 30)
	if rr != 255 || gg != 0 || bb != 0 {
		t.Fatalf("expected red (idx 0) at (30,30), got (%d,%d,%d)", rr, gg, bb)
	}
	// Bottom edge (py=39, v≈0.03) → texture row 0: left=red.
	rr, gg, bb, _ = swPix(r, 12, 39)
	if rr != 255 || gg != 0 || bb != 0 {
		t.Fatalf("expected red at (12,39), got (%d,%d,%d)", rr, gg, bb)
	}
}

// Alpha-over blend: draw a semi-transparent sprite over a red background.
func TestSWAlphaOver(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	// Background: opaque green fill.
	bg := swState()
	bg.isFlat = true
	bg.tint = [4]float32{0, 1, 0, 1}
	r.rasterizeQuadWindow(bg,
		swVertex{64, 48, 1, 1}, swVertex{64, 0, 1, 0},
		swVertex{0, 48, 0, 1}, swVertex{0, 0, 0, 0})

	// Foreground: full-screen flat red at 50% alpha (tint.a = 0.5).
	fg := swState()
	fg.isFlat = true
	fg.tint = [4]float32{1, 0, 0, 0.5}
	fg.src = BlendSrcAlpha // alpha-over needs SrcAlpha
	r.rasterizeQuadWindow(fg,
		swVertex{64, 48, 1, 1}, swVertex{64, 0, 1, 0},
		swVertex{0, 48, 0, 1}, swVertex{0, 0, 0, 0})

	rr, gg, bb, _ := swPix(r, 32, 24)
	// 0.5*red + 0.5*green ≈ (128,128,0)
	if Abs(rr-128) > 2 || Abs(gg-128) > 2 || bb != 0 {
		t.Fatalf("alpha blend at (32,24): got (%d,%d,%d), want ~(128,128,0)", rr, gg, bb)
	}
}

// Exercises the immediate path exactly as renderSpriteImmediate does: uniforms
// + SetVertexData + RenderQuad with a full-screen paletted sprite.
func TestSWImmediatePath(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	tex := &swTexture{width: 2, height: 2, depth: 8, serial: 5}
	tex.data = []byte{0, 1, 1, 0}
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 6}
	pal.data = make([]byte, 1024)
	pal.data[0], pal.data[1], pal.data[2], pal.data[3] = 255, 0, 0, 255
	pal.data[4], pal.data[5], pal.data[6], pal.data[7] = 0, 0, 255, 255

	proj := mgl.Ortho(0, 64, 0, 48, -65535, 65535)
	mv := mgl.Translate3D(0, 48, 0)
	r.SetUniformMatrix("projection", proj[:])
	r.SetUniformMatrix("modelview", mv[:])
	r.SetUniformI("isFlat", 0)
	r.SetUniformI("isRgba", 0)
	r.SetUniformI("isTrapez", 0)
	r.SetUniformI("mask", -1)
	r.SetUniformFv("tint", []float32{0, 0, 0, 0})
	r.SetUniformFv("add", []float32{0, 0, 0})
	r.SetUniformFv("mult", []float32{1, 1, 1})
	r.SetUniformF("alpha", 1)
	r.SetUniformF("gray", 0)
	r.SetUniformF("hue", 0)
	r.SetUniformI("neg", 0)
	r.EnableBlending(BlendAdd, BlendOne, BlendOne)
	r.SetTexture("tex", tex)
	r.SetTexture("pal", pal)

	// Full-screen quad in sprite space (y down, negative), like drawQuadsUV:
	// y ∈ [-48, 0] + modelview translate (0,48) → window y ∈ [0,48].
	r.SetVertexData(
		64, -48, 1, 1,
		64, 0, 1, 0,
		0, -48, 0, 1,
		0, 0, 0, 0,
	)
	r.RenderQuad()

	rr, gg, bb, _ := swPix(r, 16, 6)
	if rr != 255 || gg != 0 || bb != 0 {
		t.Fatalf("immediate path top: got (%d,%d,%d), want red (texture row 0, col 0)", rr, gg, bb)
	}
	rr, gg, bb, _ = swPix(r, 16, 40)
	if rr != 0 || gg != 0 || bb != 255 {
		t.Fatalf("immediate path bottom: got (%d,%d,%d), want blue (texture row 1, col 0)", rr, gg, bb)
	}
}

// A mirrored (facing=-1) quad: negative scale swaps the vertex pairs so the
// u=1 edge is at the window-left. The fast path must sample from the edge it
// is actually at, or the sprite renders unmirrored (P2 faces same way as P1).
func TestSWMirroredSprite(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	tex := &swTexture{width: 2, height: 2, depth: 8, serial: 7}
	tex.data = []byte{0, 1, 1, 0} // left column = index 0 (red), right = 1 (blue)
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 8}
	pal.data = make([]byte, 1024)
	pal.data[0], pal.data[1], pal.data[2], pal.data[3] = 255, 0, 0, 255 // idx 0: red
	pal.data[4], pal.data[5], pal.data[6], pal.data[7] = 0, 0, 255, 255 // idx 1: blue

	q := swState()
	q.tex = tex
	q.pal = pal
	q.mask = -1
	// Mirrored layout: v0/v1 (carrying u2=1) at min x, v2/v3 (u1=0) at max x.
	// Window-left part must show the texture's right column (u≈1), i.e. the
	// sprite is mirrored. Sampling the lower part (v≈0.72 → texture row 1):
	// window-left = tex[1][1] = red, window-right = tex[1][0] = blue.
	r.rasterizeQuadWindow(q,
		swVertex{40, 24, 0, 1},
		swVertex{40, 8, 0, 0},
		swVertex{8, 24, 1, 1},
		swVertex{8, 8, 1, 0},
	)
	// The original unmirrored layout would show blue on the window-left here;
	// the mirror must swap it to the texture's right texel.
	rr, gg, bb, _ := swPix(r, 10, 28)
	if rr != 255 || gg != 0 || bb != 0 {
		t.Fatalf("mirrored left: got (%d,%d,%d), want red (texture col 1, row 1)", rr, gg, bb)
	}
	rr, gg, bb, _ = swPix(r, 34, 28)
	if rr != 0 || gg != 0 || bb != 255 {
		t.Fatalf("mirrored right: got (%d,%d,%d), want blue at window-right", rr, gg, bb)
	}
}

// A stale palette must not mis-sample an RGBA sprite: the immediate path only
// sets SetTexture("pal") for paletted sprites and never clears it, so a 32-bit
// sprite drawn after a paletted one carries the old palette in its state. The
// rasterizer must ignore the palette for non-8-bit sources (regression: garbled
// font digits / lifebar sprites drawn as palette indices).
func TestSWStalePaletteIgnoredOnRGBA(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 20}
	pal.data = make([]byte, 1024)
	pal.data[0], pal.data[1], pal.data[2], pal.data[3] = 255, 0, 0, 255 // idx 0: red
	pal.data[4], pal.data[5], pal.data[6], pal.data[7] = 0, 0, 255, 255 // idx 1: blue

	// RGBA 2x2: {green, red; blue, yellow} (row 0 on top).
	tex32 := &swTexture{width: 2, height: 2, depth: 32, serial: 21}
	tex32.data = []byte{
		0, 255, 0, 255, 255, 0, 0, 255,
		0, 0, 255, 255, 255, 255, 0, 255,
	}

	// Simulate the stale state: palette set, but the texture is 32-bit.
	q := swState()
	q.tex = tex32
	q.pal = pal
	q.mask = -1
	r.rasterizeQuadWindow(q,
		swVertex{64, 48, 1, 1},
		swVertex{64, 0, 1, 0},
		swVertex{0, 48, 0, 1},
		swVertex{0, 0, 0, 0},
	)
	// The quad maps v=1 at the top edge (y=48), so the top of the screen shows
	// texture row 1 ({blue, yellow}) and the bottom shows row 0 ({green, red}).
	// (12,6) is in the upper half → row 1, left column → blue.
	rr, gg, bb, _ := swPix(r, 12, 6)
	if rr != 0 || gg != 0 || bb != 255 {
		t.Fatalf("stale-pal RGBA upper: got (%d,%d,%d), want (0,0,255)", rr, gg, bb)
	}
	// (30,40) is in the lower half → row 0, left column → green.
	rr, gg, bb, _ = swPix(r, 30, 40)
	if rr != 0 || gg != 255 || bb != 0 {
		t.Fatalf("stale-pal RGBA lower: got (%d,%d,%d), want (0,255,0)", rr, gg, bb)
	}
}

// RGBA nearest sampling must index the 32-bit texture with a 4-byte stride
// (regression: (sy*stride+sx)*3 read wrong bytes for depth-32 sprites).
func TestSWRGBANearestStride(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	tex := &swTexture{width: 2, height: 2, depth: 32, serial: 22}
	tex.data = []byte{
		0, 255, 0, 255, 255, 0, 0, 255,
		0, 0, 255, 255, 255, 255, 0, 255,
	}
	q := swState()
	q.tex = tex
	q.mask = -1
	r.rasterizeQuadWindow(q,
		swVertex{64, 48, 1, 1},
		swVertex{64, 0, 1, 0},
		swVertex{0, 48, 0, 1},
		swVertex{0, 0, 0, 0},
	)
	// v=1 at the top edge → row 1 at the top, row 0 at the bottom. (40,40) is in
	// the lower half → row 0; x=40 → right column → texel[0][1] = red.
	rr, gg, bb, aa := swPix(r, 40, 40)
	if rr != 255 || gg != 0 || bb != 0 || aa != 255 {
		t.Fatalf("RGBA nearest lower-right: got (%d,%d,%d,%d), want (255,0,0,255)", rr, gg, bb, aa)
	}
}

// A font-mode glyph quad must cover the quad exactly once. The old vertex
// order (TR, TL, BL, BR) made the two strip triangles overlap in a wedge near
// the top-left, double-blending coverage there (text rendered thicker/darker).
// With the drawQuadsUV order (TL, BL, TR, BR) every pixel is drawn once.
func TestSWFontGlyphNoOverlap(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	cov := &swTexture{width: 1, height: 1, depth: 8, serial: 30}
	cov.data = []byte{128} // 50% coverage everywhere

	q := &swQuadState{
		isFlat:    false,
		mask:      0,
		texUV:     [4]float32{0, 0, 1, 1},
		fontMode:  true,
		fontCov:   cov,
		fontColor: [4]float32{1, 1, 1, 1},
		add:       [3]float32{0, 0, 0},
		mult:      [3]float32{1, 1, 1},
		alpha:     1,
		gray:      0,
		hue:       0,
		neg:       false,
		blending:  true,
		eq:        BlendAdd,
		src:       BlendSrcAlpha,
		dst:       BlendOneMinusSrcAlpha,
	}
	// Full-screen glyph quad in window coords (y-up), font vertex order:
	// v0=(x2,y2) TL(0,48), v1=(x3,y3) BL(0,0), v2=(x1,y1) TR(64,48), v3=(x4,y4) BR(64,0).
	r.rasterGeneric(q,
		swVertex{0, 48, 0, 0},
		swVertex{0, 0, 0, 1},
		swVertex{64, 48, 1, 0},
		swVertex{64, 0, 1, 1},
		swBlendAddAlphaOver, nil)
	// Single 50%-coverage blend of white over black = (128,128,128,64). A
	// double blend would give (192,192,192,95). (10,30) lies in the overlap
	// wedge of the old triangle ordering.
	for _, p := range [][2]int{{10, 30}, {30, 10}, {20, 20}} {
		rr, gg, bb, aa := swPix(r, p[0], p[1])
		if rr != 128 || gg != 128 || bb != 128 || aa != 64 {
			t.Fatalf("font glyph at %v: got (%d,%d,%d,%d), want (128,128,128,64) (coverage must be blended once)", p, rr, gg, bb, aa)
		}
	}
}

// addalpha's blend pass 2 arrives with alpha > 1 and mult carrying the real
// source factor (e.g. (SrcAlpha, One) with alpha=64.25, mult=0.5). The GL
// shader multiplies paletted RGB by mult only; the software palette table must
// do the same or the source color blows out to white (regression: the stage
// reflection rendered as a white band over the floor).
func TestSWPalettedPass2Alpha(t *testing.T) {
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 40}
	pal.data = make([]byte, 1024)
	for i := 0; i < 256; i++ {
		// Entry 254 = opaque white, everything else = opaque gray.
		v := byte(200)
		if i == 254 {
			v = 255
		}
		pal.data[i*4], pal.data[i*4+1], pal.data[i*4+2], pal.data[i*4+3] = v, v, v, 255
	}
	// addalpha 128,128 pass 2: mult = 128/255, alpha = src*dst-driven value.
	q := swState()
	q.mult = [3]float32{0.50196, 0.50196, 0.50196}
	q.alpha = 64.251
	q.mask = 0
	tab := buildPalTable(pal, q)
	e := 254 * 8
	rr := int(tab[e+4]) // raw rgb
	if rr > 150 {
		t.Fatalf("paletted pass-2 raw rgb blew out: got %d, want ~%d (mult 0.5 * 255)", rr, 128)
	}
	sa := int(tab[e+3])
	if sa != 255 {
		t.Fatalf("paletted pass-2 alpha: got %d, want 255 (alpha>1 clamps to 1)", sa)
	}
}

// A quad that is only partially on-screen (the edge tiles of a tiled stage
// background) must sample the same texture sub-range a per-fragment GPU would.
// Regression: the UV step divided by the clipped pixel count instead of the
// full quad width, squeezing the entire texture into the visible sliver —
// stage backgrounds looked "stretched" while scrolling.
func TestSWClippedQuadUV(t *testing.T) {
	// 4x1 texture: red, green, blue, yellow columns.
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 50}
	pal.data = make([]byte, 1024)
	cols := [][4]byte{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255}}
	for i := 0; i < 4; i++ {
		copy(pal.data[i*4:], cols[i][:])
	}

	rgba := &swTexture{width: 4, height: 1, depth: 32, serial: 51}
	rgba.data = make([]byte, 16)
	for i := 0; i < 4; i++ {
		copy(rgba.data[i*4:], cols[i][:])
	}

	// Right-clipped quad: x in [32,96) on a 64px screen → only [32,64) visible.
	// Correct u at a pixel is (px+0.5-32)/64; the bug used /visible-width(32)
	// so px 48 → u=0.51 (blue) and px 63 → u=0.98 (yellow) instead of green.
	draw := func(tex *swTexture, usePal bool) func() {
		return func() {
			r := newSWTestRenderer(64, 16)
			q := swState()
			q.tex = tex
			q.mask = -1
			if usePal {
				q.pal = pal
			}
			r.rasterizeQuadWindow(q,
				swVertex{96, 16, 1, 0},
				swVertex{96, 0, 1, 0},
				swVertex{32, 16, 0, 0},
				swVertex{32, 0, 0, 0},
			)
			for _, p := range [][3]int{{48, 8, 1}, {63, 8, 1}} { // x, y, want green col
				rr, gg, bb, _ := swPix(r, p[0], p[1])
				if gg != 255 || rr != 0 || bb != 0 {
					t.Fatalf("clipped quad px%d: got (%d,%d,%d), want green", p[0], rr, gg, bb)
				}
			}
		}
	}

	palTex := &swTexture{width: 4, height: 1, depth: 8, serial: 52}
	palTex.data = []byte{0, 1, 2, 3}
	draw(palTex, true)()
	draw(rgba, false)()

	// Left-clipped quad: x in [-32,32) → only [0,32) visible. px 8 must show
	// the blue column (u≈0.63), not yellow (the bug's u≈0.76).
	{
		r := newSWTestRenderer(64, 16)
		q := swState()
		q.tex = palTex
		q.pal = pal
		q.mask = -1
		r.rasterizeQuadWindow(q,
			swVertex{32, 16, 1, 0},
			swVertex{32, 0, 1, 0},
			swVertex{-32, 16, 0, 0},
			swVertex{-32, 0, 0, 0},
		)
		rr, gg, bb, _ := swPix(r, 8, 8)
		if bb != 255 || rr != 0 || gg != 0 {
			t.Fatalf("left-clipped quad px8: got (%d,%d,%d), want blue", rr, gg, bb)
		}
	}

	// Filtered RGBA path: 2x1 red/blue, right-clipped as above. px 63 →
	// u≈0.49 (red-dominant bilinear mix); the bug sampled u≈0.98 (pure blue).
	{
		tex := &swTexture{width: 2, height: 1, depth: 32, filter: true, serial: 53}
		tex.data = []byte{255, 0, 0, 255, 0, 0, 255, 255}
		r := newSWTestRenderer(64, 16)
		q := swState()
		q.tex = tex
		q.mask = -1
		r.rasterizeQuadWindow(q,
			swVertex{96, 16, 1, 0},
			swVertex{96, 0, 1, 0},
			swVertex{32, 16, 0, 0},
			swVertex{32, 0, 0, 0},
		)
		rr, _, bb, _ := swPix(r, 63, 8)
		if rr < 50 || bb > 150 {
			t.Fatalf("clipped filtered px63: got (%d,..,%d), want red-dominant mix", rr, bb)
		}
	}
}

// The scalar blend helpers (swBlendAddOneOne & co., used by the paletted
// rasterizer loop) must be byte-identical to swBlendPix's per-mode cases for
// every mode and every input — the rasterizer rewrote swBlendPix calls into
// per-pixel helper dispatch and any drift shows up as off-by-one colors.
func TestSWBlendHelpersMatchSwBlendPix(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))
	for mode := swBlendAddOneOne; mode <= swBlendReplace; mode++ {
		for iter := 0; iter < 4000; iter++ {
			var dst, got [4]byte
			for i := range dst {
				dst[i] = byte(rng.Intn(256))
			}
			var s, sp [3]int
			for i := range s {
				s[i] = rng.Intn(256)
				sp[i] = rng.Intn(256)
			}
			sa := rng.Intn(256)
			var expect [4]byte
			copy(expect[:], dst[:])
			swBlendPixRef(expect[:], s, sp, sa, mode)
			copy(got[:], dst[:])
			switch mode {
			case swBlendAddOneOne:
				swBlendAddOneOnePix((*[4]byte)(got[:]), s[0], s[1], s[2], sa)
			case swBlendAddSrcAlphaOne:
				swBlendAddSrcAlphaOnePix((*[4]byte)(got[:]), sp[0], sp[1], sp[2], sa)
			case swBlendAddOneInvAlpha:
				swBlendAddOneInvAlphaPix((*[4]byte)(got[:]), s[0], s[1], s[2], sa)
			case swBlendAddAlphaOver:
				swBlendAlphaOver((*[4]byte)(got[:]), sp[0], sp[1], sp[2], sa)
			case swBlendAddZeroInvAlpha:
				swBlendAddZeroInvAlphaPix((*[4]byte)(got[:]), sa)
			case swBlendSubOneOne:
				swBlendSubOneOnePix((*[4]byte)(got[:]), s[0], s[1], s[2], sa)
			case swBlendSubSrcAlphaOne:
				swBlendSubSrcAlphaOnePix((*[4]byte)(got[:]), sp[0], sp[1], sp[2], sa)
			default:
				swBlendReplacePix((*[4]byte)(got[:]), s[0], s[1], s[2], sa)
			}
			if expect != got {
				t.Fatalf("mode %d: ref=%v helpers=%v (s=%v sp=%v sa=%d dst=%v)",
					mode, expect, got, s, sp, sa, dst)
			}
		}
	}
}

// The closure-free sampleRGBAFiltered must be bit-identical to the original
// closure implementation across the whole (u,v) domain and texture sizes — a
// 1-ULP float drift would shift blended pixel rounding (the same class of
// regression as the mul255 approximation).
func TestSWRGBAFilteredStable(t *testing.T) {
	makeTex := func(w, h int, depth int, data []byte) *swTexture {
		return &swTexture{width: int32(w), height: int32(h), depth: int32(depth), filter: true, data: data}
	}
	textures := []*swTexture{
		makeTex(1, 1, 32, []byte{10, 20, 30, 40}),
		makeTex(2, 2, 32, []byte{
			0, 255, 0, 255, 255, 0, 0, 255,
			0, 0, 255, 255, 255, 255, 0, 255,
		}),
		makeTex(4, 3, 32, []byte{
			255, 0, 0, 255, 0, 255, 0, 255, 0, 0, 255, 255, 255, 255, 255, 255,
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1,
		}),
		makeTex(3, 5, 24, []byte{ // 24-bit: alpha must stay 1
			255, 0, 0, 0, 255, 0, 0, 0, 255,
			1, 2, 3, 4, 5, 6, 7, 8, 9,
			9, 8, 7, 6, 5, 4, 3, 2, 1,
			128, 64, 32, 16, 8, 4, 2, 1, 0,
			0, 1, 2, 3, 4, 5, 6, 7, 8,
		}),
	}
	for _, tex := range textures {
		for i := 0; i <= 256; i++ {
			u := float32(i) / 256
			for j := 0; j <= 256; j++ {
				v := float32(j) / 256
				rn, gn, bn, an := tex.sampleRGBAFiltered(u, v)
				ro, go_, bo, ao := sampleRGBAFilteredOld(tex, u, v)
				if rn != ro || gn != go_ || bn != bo || an != ao {
					t.Fatalf("%dx%x d%d u=%v v=%v: new=(%v,%v,%v,%v) old=(%v,%v,%v,%v)",
						tex.width, tex.height, tex.depth, u, v, rn, gn, bn, an, ro, go_, bo, ao)
				}
			}
		}
	}
}

// The original swBlendPix (removed from production once the scalar helpers
// replaced it everywhere), frozen as the reference for the equivalence test
// above. NOTE: if the blend helpers are ever intentionally changed, this frozen
// copy and the test must be updated to the new reference implementation.
func swBlendPixRef(dst []byte, s [3]int, sp [3]int, sa int, mode int) {
	dr := int(dst[0])
	dg := int(dst[1])
	db := int(dst[2])
	da := int(dst[3])
	var nr, ng, nb, na int
	switch mode {
	case swBlendAddOneOne:
		nr, ng, nb = sat8(dr+s[0]), sat8(dg+s[1]), sat8(db+s[2])
		na = sat8(da + sa)
	case swBlendAddSrcAlphaOne:
		nr, ng, nb = sat8(dr+sp[0]), sat8(dg+sp[1]), sat8(db+sp[2])
		na = sat8(da + mul255(sa, sa))
	case swBlendAddOneInvAlpha:
		nr = dr + s[0] - mul255(dr, sa)
		ng = dg + s[1] - mul255(dg, sa)
		nb = db + s[2] - mul255(db, sa)
		na = da + sa - mul255(da, sa)
	case swBlendAddAlphaOver:
		nr = sp[0] + dr - mul255(dr, sa)
		ng = sp[1] + dg - mul255(dg, sa)
		nb = sp[2] + db - mul255(db, sa)
		na = mul255(sa, sa) + da - mul255(da, sa)
	case swBlendAddZeroInvAlpha:
		nr = dr - mul255(dr, sa)
		ng = dg - mul255(dg, sa)
		nb = db - mul255(db, sa)
		na = da - mul255(da, sa)
	case swBlendSubOneOne:
		nr, ng, nb = sat8(dr-s[0]), sat8(dg-s[1]), sat8(db-s[2])
		na = sat8(da - sa)
	case swBlendSubSrcAlphaOne:
		nr, ng, nb = sat8(dr-sp[0]), sat8(dg-sp[1]), sat8(db-sp[2])
		na = sat8(da - mul255(sa, sa))
	default: // swBlendReplace
		nr, ng, nb, na = s[0], s[1], s[2], sa
	}
	dst[0] = byte(nr)
	dst[1] = byte(ng)
	dst[2] = byte(nb)
	dst[3] = byte(na)
}

// The pre-refactor closure implementation of sampleRGBAFiltered, frozen for the
// stability test above. NOTE: if the sampler is ever intentionally changed
// (e.g. integer bilinear), this frozen copy and the test must be updated to
// the new reference implementation.
func sampleRGBAFilteredOld(t *swTexture, u, v float32) (float32, float32, float32, float32) {
	w := int(t.width)
	h := int(t.height)
	x0 := int(math.Floor(float64(u*float32(w) - 0.5)))
	y0 := int(math.Floor(float64(v*float32(h) - 0.5)))
	fx := u*float32(w) - 0.5 - float32(x0)
	fy := v*float32(h) - 0.5 - float32(y0)
	x0 = swClampIdx(x0, w)
	y0 = swClampIdx(y0, h)
	x1 := swClampIdx(x0+1, w)
	y1 := swClampIdx(y0+1, h)
	bpp := int(t.depth / 8)
	if bpp < 1 {
		bpp = 1
	}
	d := t.data
	lerp := func(a, b, f float32) float32 { return a + (b-a)*f }
	px := func(o int) (float32, float32, float32, float32) {
		r := float32(d[o]) / 255
		g := float32(d[o+1]) / 255
		b := float32(d[o+2]) / 255
		a := float32(1)
		if t.depth >= 32 {
			a = float32(d[o+3]) / 255
		}
		return r, g, b, a
	}
	r00, g00, b00, a00 := px(y0*w*bpp + x0*bpp)
	r10, g10, b10, a10 := px(y0*w*bpp + x1*bpp)
	r01, g01, b01, a01 := px(y1*w*bpp + x0*bpp)
	r11, g11, b11, a11 := px(y1*w*bpp + x1*bpp)
	r := lerp(lerp(r00, r10, fx), lerp(r01, r11, fx), fy)
	g := lerp(lerp(g00, g10, fx), lerp(g01, g11, fx), fy)
	b := lerp(lerp(b00, b10, fx), lerp(b01, b11, fx), fy)
	a := lerp(lerp(a00, a10, fx), lerp(a01, a11, fx), fy)
	return r, g, b, a
}

// The RGBA filtered loop in alpha-over mode with exact-pixel expectations: a
// 2x1 filtered texture {opaque red, half-alpha blue} drawn over an opaque green
// fill, with the quad sized so each pixel center samples a texel center exactly
// (fx = fy = 0, no bilinear mixing). Verifies the scalar-dispatch switch in the
// filtered loop wires the right scalars (premul sp for SrcAlpha modes).
func TestSWRGBAFilteredAlphaOver(t *testing.T) {
	r := newSWTestRenderer(4, 2)
	// Background: opaque green fill.
	bg := swState()
	bg.isFlat = true
	bg.tint = [4]float32{0, 1, 0, 1}
	r.rasterizeQuadWindow(bg,
		swVertex{4, 2, 1, 1}, swVertex{4, 0, 1, 0},
		swVertex{0, 2, 0, 1}, swVertex{0, 0, 0, 0})

	// Foreground: filtered 2x1 {red opaque, blue 50%} in alpha-over mode.
	tex := &swTexture{width: 2, height: 1, depth: 32, filter: true, serial: 60}
	tex.data = []byte{255, 0, 0, 255, 0, 0, 255, 128}
	q := swState()
	q.tex = tex
	q.mask = 0 // keep the texel's real alpha (mask -1 would force aa = 1)
	q.src = BlendSrcAlpha
	r.rasterizeQuadWindow(q,
		swVertex{2, 2, 1, 1}, swVertex{2, 0, 1, 0},
		swVertex{0, 2, 0, 1}, swVertex{0, 0, 0, 0})

	// Pixel column 0 samples texel 0 (opaque red): covers green fully.
	// Pixel column 1 samples texel 1 (blue, sa = quant(128/255) = 128):
	//   g = 255 - mul255(255,128) = 127, b = mul255(255,128) = 128,
	//   a = mul255(128,128) + 255 - mul255(255,128) = 191.
	for py := 0; py < 2; py++ {
		rr, gg, bb, aa := swPix(r, 0, py)
		if rr != 255 || gg != 0 || bb != 0 || aa != 255 {
			t.Fatalf("filtered alpha-over col 0 row %d: got (%d,%d,%d,%d), want (255,0,0,255)", py, rr, gg, bb, aa)
		}
		rr, gg, bb, aa = swPix(r, 1, py)
		if rr != 0 || gg != 127 || bb != 128 || aa != 191 {
			t.Fatalf("filtered alpha-over col 1 row %d: got (%d,%d,%d,%d), want (0,127,128,191)", py, rr, gg, bb, aa)
		}
	}
}

// The flat path's outer mode dispatch must wire s (raw) vs sp (premul) to the
// right helpers. A semi-transparent red source distinguishes them over black:
// AddOneOne uses raw s → (255,0,0), AddSrcAlphaOne uses premul sp → (128,0,0),
// so a swapped argument in any flat switch case fails these exact checks.
func TestSWFlatFillModeWiring(t *testing.T) {
	cases := []struct {
		name string
		src  BlendFunc
		dst  BlendFunc
		want [4]int
	}{
		{"AddOneOne", BlendOne, BlendOne, [4]int{255, 0, 0, 128}},
		{"AddSrcAlphaOne", BlendSrcAlpha, BlendOne, [4]int{128, 0, 0, 64}},
	}
	for _, c := range cases {
		r := newSWTestRenderer(4, 2)
		q := swState()
		q.isFlat = true
		q.tint = [4]float32{1, 0, 0, 0.5} // semi-transparent red
		q.src = c.src
		q.dst = c.dst
		r.rasterizeQuadWindow(q,
			swVertex{4, 2, 1, 1}, swVertex{4, 0, 1, 0},
			swVertex{0, 2, 0, 1}, swVertex{0, 0, 0, 0})
		rr, gg, bb, aa := swPix(r, 2, 1)
		if rr != c.want[0] || gg != c.want[1] || bb != c.want[2] || aa != c.want[3] {
			t.Fatalf("flat %s: got (%d,%d,%d,%d), want %v", c.name, rr, gg, bb, aa, c.want)
		}
	}
}

// Rotated quad through the generic path (45° rotation of a 4x4 sprite).
func TestSWRotatedQuad(t *testing.T) {
	r := newSWTestRenderer(64, 48)
	tex := &swTexture{width: 4, height: 4, depth: 8, serial: 3}
	tex.data = []byte{0, 0, 0, 0, 0, 1, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0}
	pal := &swTexture{width: 256, height: 1, depth: 32, palSlot: true, serial: 4}
	pal.data = make([]byte, 1024)
	pal.data[0], pal.data[1], pal.data[2], pal.data[3] = 0, 0, 0, 0 // transparent
	pal.data[4], pal.data[5], pal.data[6], pal.data[7] = 255, 255, 255, 255

	q := swState()
	q.tex = tex
	q.pal = pal
	q.mask = -1
	// Rotated 45° around the sprite center at (32, 24), radius ~2.83.
	cx, cy := float32(32), float32(24)
	rot := func(x, y float32) (float32, float32) {
		dx, dy := x-2, y-2 // sprite-space center (4x4 sprite)
		return cx + dx*0.7071 - dy*0.7071, cy + dx*0.7071 + dy*0.7071
	}
	x1, y1 := rot(0, 4)
	x2, y2 := rot(4, 4)
	x3, y3 := rot(4, 0)
	x4, y4 := rot(0, 0)
	r.rasterizeQuadWindow(q,
		swVertex{x2, y2, 1, 1},
		swVertex{x3, y3, 1, 0},
		swVertex{x1, y1, 0, 1},
		swVertex{x4, y4, 0, 0},
	)
	// Center pixel must be white (sprite index 1).
	rr, gg, bb, _ := swPix(r, 32, 24)
	if rr != 255 || gg != 255 || bb != 255 {
		t.Fatalf("rotated center: got (%d,%d,%d), want white", rr, gg, bb)
	}
	// Corner far from the sprite must be untouched (transparent bg → black).
	rr, gg, bb, _ = swPix(r, 4, 4)
	if rr != 0 || gg != 0 || bb != 0 {
		t.Fatalf("rotated corner: got (%d,%d,%d), want black", rr, gg, bb)
	}
}
