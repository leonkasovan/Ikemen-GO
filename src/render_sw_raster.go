//go:build !android && !armdevice

package main

import "math"

// swVertex is a quad corner: x, y in window coordinates (y-up, origin
// bottom-left — the same space GL33 renders into), plus normalized u, v.
type swVertex struct {
	x, y float32
	u, v float32
}

// swQuadState carries everything the rasterizer needs for one quad: the
// resolved shader uniforms and blend state of one SpriteDrawCall / RenderQuad.
// RenderQuad and flushSWQueue take instances from swQuadStatePool instead of
// allocating per draw: for large quads the state escapes into the row-pool jobs
// (via swRowDraw.q), and a fresh struct per quad was the largest flat
// allocation in the software renderer's profiles.
type swQuadState struct {
	isFlat           bool
	mask             int32
	isTrapez         bool
	x1x2x4x3         [4]float32 // trapezoid correction bounds (shader uniform)
	tint             [4]float32
	tex, pal         *swTexture
	add, mult        [3]float32
	alpha, gray, hue float32
	neg              bool
	eq               BlendEquation
	src, dst         BlendFunc
	blending         bool
	hasScissor       bool
	scissor          [4]int32

	// TTF font glyph rendering (render_sw_font.go).
	fontMode  bool
	fontCov   *swTexture // glyph coverage texture (depth 8)
	fontColor [4]float32 // min(textColor, 1), see font.frag.glsl
}

// swPalKey identifies a fully-resolved palette table: the palette texture
// (identity + data version) plus every PalFX state buildPalTable reads. See
// rasterizeQuadWindow for the memoization that turns the per-quad 2048-byte
// table build (whose backing array escaped to the heap through the runRows
// worker closures) into a pointer lookup after warm-up.
type swPalKey struct {
	palSerial uint64
	palVer    uint64
	mask      int32
	neg       bool
	alpha     uint32 // math.Float32bits
	hue       uint32
	gray      uint32
	add       [3]uint32
	mult      [3]uint32
	tint      [4]uint32
}

func swPalKeyFor(q *swQuadState) swPalKey {
	return swPalKey{
		palSerial: q.pal.serial,
		palVer:    q.pal.dataVersion,
		mask:      q.mask,
		neg:       q.neg,
		alpha:     math.Float32bits(q.alpha),
		hue:       math.Float32bits(q.hue),
		gray:      math.Float32bits(q.gray),
		add:       [3]uint32{math.Float32bits(q.add[0]), math.Float32bits(q.add[1]), math.Float32bits(q.add[2])},
		mult:      [3]uint32{math.Float32bits(q.mult[0]), math.Float32bits(q.mult[1]), math.Float32bits(q.mult[2])},
		tint:      [4]uint32{math.Float32bits(q.tint[0]), math.Float32bits(q.tint[1]), math.Float32bits(q.tint[2]), math.Float32bits(q.tint[3])},
	}
}

// swPalTableCacheMax caps the palette table memo. Each entry is 2048 bytes;
// a fight's distinct (palette × PalFX state) combos are small, so 1024 entries
// (~2 MB worst case) is generous. When exceeded the cache is rebuilt — a rare,
// single-frame event that only affects one draw call.
const swPalTableCacheMax = 1024

func isAxisRect(v0, v1, v2, v3 swVertex) bool {
	const eps = 0.001
	// Axis-aligned in window space...
	if Abs(v0.x-v1.x) > eps || Abs(v2.x-v3.x) > eps ||
		Abs(v0.y-v2.y) > eps || Abs(v1.y-v3.y) > eps {
		return false
	}
	// ...and with a uv map whose cross terms vanish (otherwise the two
	// triangles of the quad disagree and the bilinear fast path is wrong,
	// e.g. exact 90°/270° rotations).
	return Abs((v0.u-v1.u)-(v2.u-v3.u)) <= eps &&
		Abs((v0.v-v1.v)-(v2.v-v3.v)) <= eps
}

// pixelRange maps a window-coordinate interval [min,max) to the integer pixel
// indices whose fragment centers fall inside it (GL top-left fill rule).
func pixelRange(min, max float32) (int, int) {
	p0 := int(math.Ceil(float64(min - 0.5)))
	p1 := int(math.Ceil(float64(max-0.5))) - 1
	if p1 < p0 {
		return p0, p0 - 1
	}
	return p0, p1
}

// rasterizeQuadWindow draws one quad (v0..v3 in window coords) into the
// framebuffer. Triangles: (v0,v1,v2) and (v1,v2,v3), shared edge v1-v2 —
// the same triangulation GL33 uses for its triangle strip.
func (r *Renderer_SW) rasterizeQuadWindow(q *swQuadState, v0, v1, v2, v3 swVertex) {
	if !q.isFlat && q.tex == nil {
		return
	}
	// The immediate path only calls SetTexture("pal") for paletted sprites and
	// never clears it, so an RGBA (depth 24/32) sprite drawn right after a
	// paletted one would see a stale palette and get mis-sampled as 8-bit
	// indexed — garbling font digits, lifebars and other true-color sprites.
	// Only sample paletted when the source texture is actually 8-bit.
	if q.pal != nil && (q.tex == nil || q.tex.depth != 8) {
		q.pal = nil
	}
	mode := r.swBlendMode(q)
	var tab []byte
	if q.pal != nil {
		// Memoize the PalFX-applied palette table. buildPalTable runs 256
		// iterations of the float PalFX chain; rebuilding it per quad made the
		// returned 2048-byte array escape into the runRows worker closures and
		// heap-allocate on every draw. Keys are (palette identity + data
		// version + PalFX state), so palette edits and animated PalFX both
		// invalidate correctly while steady-state draws hit the cache.
		if r.palTableCache == nil {
			r.palTableCache = make(map[swPalKey]*[2048]byte)
		}
		key := swPalKeyFor(q)
		if p, ok := r.palTableCache[key]; ok {
			tab = p[:]
		} else {
			t := buildPalTable(q.pal, q)
			r.palTableCache[key] = &t
			tab = t[:]
			if len(r.palTableCache) > swPalTableCacheMax {
				r.palTableCache = make(map[swPalKey]*[2048]byte)
			}
		}
	}
	if !q.isTrapez && isAxisRect(v0, v1, v2, v3) {
		r.rasterRect(q, v0, v1, v2, v3, mode, tab)
	} else {
		r.rasterGeneric(q, v0, v1, v2, v3, mode, tab)
	}
}

// clipRangeX clips a pixel column range to the screen and the quad's scissor
// rect (top-down). Returns the inclusive range and whether it is non-empty.
func (r *Renderer_SW) clipRangeX(q *swQuadState, px0, px1 int) (int, int, bool) {
	if px0 < 0 {
		px0 = 0
	}
	if px1 >= int(r.w) {
		px1 = int(r.w) - 1
	}
	if q.hasScissor {
		sx, sw := int(q.scissor[0]), int(q.scissor[2])
		if px0 < sx {
			px0 = sx
		}
		if px1 >= sx+sw {
			px1 = sx + sw - 1
		}
	}
	return px0, px1, px0 <= px1
}

// clipRangeY clips a pixel row range to the screen and the quad's scissor rect.
func (r *Renderer_SW) clipRangeY(q *swQuadState, py0, py1 int) (int, int, bool) {
	if py0 < 0 {
		py0 = 0
	}
	if py1 >= int(r.h) {
		py1 = int(r.h) - 1
	}
	if q.hasScissor {
		sy, sh := int(q.scissor[1]), int(q.scissor[3])
		if py0 < sy {
			py0 = sy
		}
		if py1 >= sy+sh {
			py1 = sy + sh - 1
		}
	}
	return py0, py1, py0 <= py1
}

// ---- Axis-aligned rect fast path ----

func (r *Renderer_SW) rasterRect(q *swQuadState, v0, v1, v2, v3 swVertex, mode int, tab []byte) {
	xLeft := float32(math.Min(math.Min(float64(v0.x), float64(v1.x)), math.Min(float64(v2.x), float64(v3.x))))
	xRight := float32(math.Max(math.Max(float64(v0.x), float64(v1.x)), math.Max(float64(v2.x), float64(v3.x))))
	yTop := float32(math.Max(math.Max(float64(v0.y), float64(v1.y)), math.Max(float64(v2.y), float64(v3.y))))
	yBot := float32(math.Min(math.Min(float64(v0.y), float64(v1.y)), math.Min(float64(v2.y), float64(v3.y))))
	if xRight <= xLeft || yTop <= yBot {
		return
	}
	px0, px1 := pixelRange(xLeft, xRight)
	py0, py1 := pixelRange(float32(r.h)-yTop, float32(r.h)-yBot)
	px0, px1, okX := r.clipRangeX(q, px0, px1)
	py0, py1, okY := r.clipRangeY(q, py0, py1)
	if !okX || !okY {
		return
	}
	r.markDirty(int32(px0), int32(py0), int32(px1+1), int32(py1+1))

	wSpan := xRight - xLeft
	n := px1 - px0 + 1

	if q.isFlat {
		// Constant source color (FillRect): precompute once. The source scalars
		// are loop-invariant, so the blend dispatch is hoisted out of the pixel
		// loops — swRowDraw's kind picks a tight per-mode loop with no per-pixel
		// branch.
		rr, gg, bb, aa := applySpritePalfx(q.tint[0], q.tint[1], q.tint[2], q.tint[3], q, true, 1)
		sa := quant(aa)
		s0, s1, s2 := quant(rr), quant(gg), quant(bb)
		sp0 := sat8(mul255(s0, sa))
		sp1 := sat8(mul255(s1, sa))
		sp2 := sat8(mul255(s2, sa))
		var d swRowDraw
		d.r, d.q = r, q
		d.px0, d.px1, d.n = px0, px1, n
		d.s0, d.s1, d.s2 = s0, s1, s2
		d.sp0, d.sp1, d.sp2, d.sa = sp0, sp1, sp2, sa
		switch mode {
		case swBlendAddAlphaOver:
			// Dominant case: inline alpha-over, no per-pixel call or arrays.
			d.kind = swRowFlatAO
		case swBlendAddOneOne:
			d.kind = swRowFlatOneOne
		case swBlendAddSrcAlphaOne:
			d.kind = swRowFlatSrcAlphaOne
		case swBlendAddOneInvAlpha:
			d.kind = swRowFlatOneInvAlpha
		case swBlendAddZeroInvAlpha:
			d.kind = swRowFlatZeroInvAlpha
		case swBlendSubOneOne:
			d.kind = swRowFlatSubOneOne
		case swBlendSubSrcAlphaOne:
			d.kind = swRowFlatSubSrcAlphaOne
		default: // swBlendReplace
			d.kind = swRowFlatReplace
		}
		r.runRows(&d, py0, py1)
		return
	}

	tex := q.tex

	// u,v at the fragment center (px0, py0), and per-pixel / per-row steps.
	px := float32(px0)
	tx0 := (px + 0.5 - xLeft) / wSpan

	// The strip pairs (v0,v1) and (v2,v3) are the quad's two vertical edges,
	// but which one is the window-left edge depends on facing: a negative
	// scale (facing=-1, P2) swaps them, so u must be read from the actual
	// edge at each side or the sprite renders unmirrored.
	l0, l1, r0, r1 := v0, v1, v2, v3
	if v2.x < v0.x {
		l0, l1, r0, r1 = v2, v3, v0, v1
	}

	// UV stepping divisor: the FULL quad width in pixels, computed before
	// clipping. n (above) is the clipped visible count — a tile that is
	// partially off-screen or scissored must still sample the same texture
	// sub-range a per-fragment GPU would; dividing the step by n instead
	// squeezed the entire texture into the visible sliver, making stage
	// backgrounds look stretched while scrolling.
	fullPx0, fullPx1 := pixelRange(xLeft, xRight)
	wSpanPix := fullPx1 - fullPx0 + 1
	if wSpanPix < 1 {
		wSpanPix = 1
	}

	// The row-loop constants are packed into a stack swRowDraw; runRows copies
	// it into pooled jobs when the quad is large enough to parallelize. The
	// pixel loops live in swRowDraw.draw — no closures, so nothing escapes.
	var d swRowDraw
	d.r, d.q = r, q
	d.tex = tex
	d.px0, d.px1, d.n = px0, px1, n
	d.tx0 = tx0
	d.wSpanPix = wSpanPix
	d.l0, d.l1, d.r0, d.r1 = l0, l1, r0, r1

	if q.pal != nil {
		// Paletted: fixed-point texel stepping, table lookup per pixel.
		d.tab = tab
		if mode == swBlendAddAlphaOver {
			// Dominant case: alpha-over needs only the premultiplied rgb + alpha
			// from the table, so skip the raw-rgb loads and the generic per-pixel
			// blend call/switch entirely.
			d.kind = swRowPalAO
			r.runRows(&d, py0, py1)
			return
		}
		d.kind = swRowPalGeneric
		d.mode = mode
		r.runRows(&d, py0, py1)
		return
	}

	// RGBA source.
	d.mode = mode
	if tex.filter {
		d.kind = swRowRGBAFiltered
		r.runRows(&d, py0, py1)
		return
	}
	d.kind = swRowRGBANearest
	r.runRows(&d, py0, py1)
}

// ---- Generic affine-triangle path (rotation, trapezoid, tiled quads) ----

func (r *Renderer_SW) rasterGeneric(q *swQuadState, v0, v1, v2, v3 swVertex, mode int, tab []byte) {
	minX := float32(math.Min(math.Min(float64(v0.x), float64(v1.x)), math.Min(float64(v2.x), float64(v3.x))))
	maxX := float32(math.Max(math.Max(float64(v0.x), float64(v1.x)), math.Max(float64(v2.x), float64(v3.x))))
	minY := float32(math.Min(math.Min(float64(v0.y), float64(v1.y)), math.Min(float64(v2.y), float64(v3.y))))
	maxY := float32(math.Max(math.Max(float64(v0.y), float64(v1.y)), math.Max(float64(v2.y), float64(v3.y))))
	px0, px1 := pixelRange(minX, maxX)
	py0, py1 := pixelRange(float32(r.h)-maxY, float32(r.h)-minY)
	px0, px1, okX := r.clipRangeX(q, px0, px1)
	py0, py1, okY := r.clipRangeY(q, py0, py1)
	if !okX || !okY {
		return
	}
	r.markDirty(int32(px0), int32(py0), int32(px1+1), int32(py1+1))

	// Same treatment as rasterRect: the loop constants go into a stack
	// swRowDraw; the pixel loop lives in swRowDraw.draw (no escaping closure).
	var d swRowDraw
	d.r, d.q = r, q
	d.kind = swRowGeneric
	d.mode = mode
	d.tab = tab
	d.px0, d.px1 = px0, px1
	d.v0, d.v1, d.v2, d.v3 = v0, v1, v2, v3
	r.runRows(&d, py0, py1)
}

// baryWeights returns the unnormalized barycentric weights W1, W2 and the
// signed determinant for the triangle (t0, t1, t2) at point p.
func baryWeights(pX, pY float32, t0, t1, t2 swVertex) (W1, W2, det float32) {
	e0x := t1.x - t0.x
	e0y := t1.y - t0.y
	e1x := t2.x - t0.x
	e1y := t2.y - t0.y
	px := pX - t0.x
	py := pY - t0.y
	det = e0x*e1y - e0y*e1x
	W1 = px*e1y - py*e1x
	W2 = e0x*py - e0y*px
	return
}

// triInside tests barycentric weights against the triangle edges. w0Strict
// makes the edge opposite t0 exclusive (used for the quad's shared diagonal
// so it is covered exactly once).
func triInside(W0, W1, W2, det float32, w0Strict bool) bool {
	if det == 0 {
		return false
	}
	if det > 0 {
		if w0Strict && W0 <= 0 {
			return false
		}
		return W0 >= 0 && W1 >= 0 && W2 >= 0
	}
	if w0Strict && W0 >= 0 {
		return false
	}
	return W0 <= 0 && W1 <= 0 && W2 <= 0
}

// shadePix samples, applies PalFX and blends one pixel at normalized (u, v),
// writing into dst. The trapezoid correction that used to be here moved into
// rasterGeneric (it needs the fragment's window x), so the winX parameter was
// dropped.
func shadePix(dst *[4]byte, u, v float32, q *swQuadState, mode int, tab []byte) {
	var s0, s1, s2, sp0, sp1, sp2, sa int
	if q.fontMode {
		// TTF glyph: font.frag.glsl math. Coverage is bilinearly sampled from
		// the glyph texture; the color is min(textColor, 1) * (1,1,1,cov).
		cov := q.fontCov.sampleIndexBilinear(u, v)
		rr := q.fontColor[0]
		gg := q.fontColor[1]
		bb := q.fontColor[2]
		aa := q.fontColor[3] * float32(cov) / 255
		if q.hue != 0 {
			rr, gg, bb = swHueShift(rr, gg, bb, q.hue)
		}
		if q.neg {
			rr, gg, bb = aa-rr, aa-gg, aa-bb
		}
		if q.gray != 0 {
			avg := (rr + gg + bb) / 3
			gr := 1 - q.gray
			rr = avg + (rr-avg)*gr
			gg = avg + (gg-avg)*gr
			bb = avg + (bb-avg)*gr
		}
		rr += q.add[0] * aa
		gg += q.add[1] * aa
		bb += q.add[2] * aa
		rr *= q.mult[0]
		gg *= q.mult[1]
		bb *= q.mult[2]
		sa = quant(aa)
		s0, s1, s2 = quant(rr), quant(gg), quant(bb)
		sp0, sp1, sp2 = sat8(mul255(s0, sa)), sat8(mul255(s1, sa)), sat8(mul255(s2, sa))
	} else if q.isFlat {
		rr, gg, bb, aa := applySpritePalfx(q.tint[0], q.tint[1], q.tint[2], q.tint[3], q, true, 1)
		sa = quant(aa)
		s0, s1, s2 = quant(rr), quant(gg), quant(bb)
		sp0, sp1, sp2 = sat8(mul255(s0, sa)), sat8(mul255(s1, sa)), sat8(mul255(s2, sa))
	} else if q.tex == nil {
		return
	} else if q.pal != nil {
		e := q.tex.sampleIndex(u, v) * 8
		sp0, sp1, sp2 = int(tab[e]), int(tab[e+1]), int(tab[e+2])
		sa = int(tab[e+3])
		s0, s1, s2 = int(tab[e+4]), int(tab[e+5]), int(tab[e+6])
	} else {
		var rr, gg, bb, aa float32
		if q.tex.filter {
			rr, gg, bb, aa = q.tex.sampleRGBAFiltered(u, v)
		} else {
			rr, gg, bb, aa = q.tex.sampleRGBA(u, v)
		}
		if q.mask == -1 {
			aa = 1
		}
		rr, gg, bb, aa = applySpritePalfx(rr, gg, bb, aa, q, true, q.alpha)
		rr, gg, bb = tintMix(rr, gg, bb, aa, q.tint)
		sa = quant(aa)
		s0, s1, s2 = quant(rr), quant(gg), quant(bb)
		sp0, sp1, sp2 = sat8(mul255(s0, sa)), sat8(mul255(s1, sa)), sat8(mul255(s2, sa))
	}
	// mode is constant per draw; scalar dispatch like the rasterRect loops
	// (byte-identical to the old swBlendPix cases).
	switch mode {
	case swBlendAddAlphaOver:
		swBlendAlphaOver(dst, sp0, sp1, sp2, sa)
	case swBlendAddOneOne:
		swBlendAddOneOnePix(dst, s0, s1, s2, sa)
	case swBlendAddSrcAlphaOne:
		swBlendAddSrcAlphaOnePix(dst, sp0, sp1, sp2, sa)
	case swBlendAddOneInvAlpha:
		swBlendAddOneInvAlphaPix(dst, s0, s1, s2, sa)
	case swBlendAddZeroInvAlpha:
		swBlendAddZeroInvAlphaPix(dst, sa)
	case swBlendSubOneOne:
		swBlendSubOneOnePix(dst, s0, s1, s2, sa)
	case swBlendSubSrcAlphaOne:
		swBlendSubSrcAlphaOnePix(dst, sp0, sp1, sp2, sa)
	default: // swBlendReplace
		swBlendReplacePix(dst, s0, s1, s2, sa)
	}
}

// ---- Texture sampling ----

func swClampIdx(v, hi int) int {
	if v < 0 {
		return 0
	}
	if v >= hi {
		return hi - 1
	}
	return v
}

// sampleIndex returns the 8-bit palette index at normalized (u, v), nearest.
func (t *swTexture) sampleIndex(u, v float32) int {
	sx := swClampIdx(int(u*float32(t.width)), int(t.width))
	sy := swClampIdx(int(v*float32(t.height)), int(t.height))
	return int(t.data[sy*int(t.width)+sx])
}

// sampleIndexBilinear returns a bilinearly filtered 8-bit value (used for
// glyph coverage, mirroring the linear filter of the GL font atlas).
func (t *swTexture) sampleIndexBilinear(u, v float32) int {
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
	d := t.data
	c00 := float32(d[y0*w+x0])
	c10 := float32(d[y0*w+x1])
	c01 := float32(d[y1*w+x0])
	c11 := float32(d[y1*w+x1])
	c := c00 + (c10-c00)*fx + (c01-c00)*fy + (c00-c10-c01+c11)*fx*fy
	return int(c + 0.5)
}

// sampleRGBA returns the texel at normalized (u, v), nearest.
func (t *swTexture) sampleRGBA(u, v float32) (float32, float32, float32, float32) {
	sx := swClampIdx(int(u*float32(t.width)), int(t.width))
	sy := swClampIdx(int(v*float32(t.height)), int(t.height))
	bpp := int(t.depth / 8)
	if bpp < 1 {
		bpp = 1
	}
	o := (sy*int(t.width) + sx) * bpp
	d := t.data
	r := float32(d[o]) / 255
	g := float32(d[o+1]) / 255
	b := float32(d[o+2]) / 255
	a := float32(1)
	if t.depth >= 32 {
		a = float32(d[o+3]) / 255
	}
	return r, g, b, a
}

// sampleRGBAFiltered returns the bilinearly filtered texel (CLAMP_TO_EDGE).
// Same float32 math as the GL sprite shader's bilinear sample. Written without
// closures — the old lerp/px closures were the two hottest children of this
// function in profiles; inlined direct code lets the compiler keep the corner
// texels in registers.
func (t *swTexture) sampleRGBAFiltered(u, v float32) (float32, float32, float32, float32) {
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
	o00 := (y0*w + x0) * bpp
	o10 := (y0*w + x1) * bpp
	o01 := (y1*w + x0) * bpp
	o11 := (y1*w + x1) * bpp
	// Corner texels (alpha = 1 for 24-bit sources), then lerp rows and column:
	// r = lerp(lerp(r00,r10,fx), lerp(r01,r11,fx), fy). The float32 operation
	// order is unchanged from the closure version, so results are bit-identical.
	if t.depth >= 32 {
		r00, g00, b00, a00 := float32(d[o00])/255, float32(d[o00+1])/255, float32(d[o00+2])/255, float32(d[o00+3])/255
		r10, g10, b10, a10 := float32(d[o10])/255, float32(d[o10+1])/255, float32(d[o10+2])/255, float32(d[o10+3])/255
		r01, g01, b01, a01 := float32(d[o01])/255, float32(d[o01+1])/255, float32(d[o01+2])/255, float32(d[o01+3])/255
		r11, g11, b11, a11 := float32(d[o11])/255, float32(d[o11+1])/255, float32(d[o11+2])/255, float32(d[o11+3])/255
		r0 := r00 + (r10-r00)*fx
		g0 := g00 + (g10-g00)*fx
		b0f := b00 + (b10-b00)*fx
		a0 := a00 + (a10-a00)*fx
		r1 := r01 + (r11-r01)*fx
		g1 := g01 + (g11-g01)*fx
		b1f := b01 + (b11-b01)*fx
		a1 := a01 + (a11-a01)*fx
		return r0 + (r1-r0)*fy, g0 + (g1-g0)*fy, b0f + (b1f-b0f)*fy, a0 + (a1-a0)*fy
	}
	r00, g00, b00 := float32(d[o00])/255, float32(d[o00+1])/255, float32(d[o00+2])/255
	r10, g10, b10 := float32(d[o10])/255, float32(d[o10+1])/255, float32(d[o10+2])/255
	r01, g01, b01 := float32(d[o01])/255, float32(d[o01+1])/255, float32(d[o01+2])/255
	r11, g11, b11 := float32(d[o11])/255, float32(d[o11+1])/255, float32(d[o11+2])/255
	r0 := r00 + (r10-r00)*fx
	g0 := g00 + (g10-g00)*fx
	b0f := b00 + (b10-b00)*fx
	r1 := r01 + (r11-r01)*fx
	g1 := g01 + (g11-g01)*fx
	b1f := b01 + (b11-b01)*fx
	return r0 + (r1-r0)*fy, g0 + (g1-g0)*fy, b0f + (b1f-b0f)*fy, 1
}
