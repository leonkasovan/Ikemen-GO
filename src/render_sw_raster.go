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
type swQuadState struct {
	isFlat           bool
	mask             int32
	isTrapez         bool
	x1x2x4x3         [4]float32 // trapezoid correction bounds (shader uniform)
	tint             [4]float32
	texUV            [4]float32 // {u1,v1,u2,v2} sprite rect within the texture
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
	mode := swBlendMode(q)
	var tab []byte
	if q.pal != nil {
		t := buildPalTable(q.pal, q)
		tab = t[:]
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
		// are loop-invariant, so the mode switch is hoisted out of the pixel
		// loops — each mode gets a tight inner loop with no per-pixel branch.
		rr, gg, bb, aa := applySpritePalfx(q.tint[0], q.tint[1], q.tint[2], q.tint[3], q, true, 1)
		sa := quant(aa)
		s0, s1, s2 := quant(rr), quant(gg), quant(bb)
		sp0 := sat8(mul255(s0, sa))
		sp1 := sat8(mul255(s1, sa))
		sp2 := sat8(mul255(s2, sa))
		switch mode {
		case swBlendAddAlphaOver:
			// Dominant case: inline alpha-over, no per-pixel call or arrays.
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendAlphaOver((*[4]byte)(dst[i*4:]), sp0, sp1, sp2, sa)
				}
			}
		case swBlendAddOneOne:
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendAddOneOnePix((*[4]byte)(dst[i*4:]), s0, s1, s2, sa)
				}
			}
		case swBlendAddSrcAlphaOne:
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendAddSrcAlphaOnePix((*[4]byte)(dst[i*4:]), sp0, sp1, sp2, sa)
				}
			}
		case swBlendAddOneInvAlpha:
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendAddOneInvAlphaPix((*[4]byte)(dst[i*4:]), s0, s1, s2, sa)
				}
			}
		case swBlendAddZeroInvAlpha:
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendAddZeroInvAlphaPix((*[4]byte)(dst[i*4:]), sa)
				}
			}
		case swBlendSubOneOne:
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendSubOneOnePix((*[4]byte)(dst[i*4:]), s0, s1, s2, sa)
				}
			}
		case swBlendSubSrcAlphaOne:
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendSubSrcAlphaOnePix((*[4]byte)(dst[i*4:]), sp0, sp1, sp2, sa)
				}
			}
		default: // swBlendReplace
			for py := py0; py <= py1; py++ {
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					swBlendReplacePix((*[4]byte)(dst[i*4:]), s0, s1, s2, sa)
				}
			}
		}
		return
	}

	tex := q.tex
	texW := int(tex.width)
	texH := int(tex.height)
	texStride := texW * int(tex.depth/8)
	texData := tex.data

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

	if q.pal != nil {
		// Paletted: fixed-point texel stepping, table lookup per pixel.
		if mode == swBlendAddAlphaOver {
			// Dominant case: alpha-over needs only the premultiplied rgb + alpha
			// from the table, so skip the raw-rgb loads and the generic per-pixel
			// blend call/switch entirely.
			for py := py0; py <= py1; py++ {
				wy := float32(r.h) - float32(py) - 0.5
				tL := (wy - l0.y) / (l1.y - l0.y)
				tR := (wy - r0.y) / (r1.y - r0.y)
				ul := l0.u + (l1.u-l0.u)*tL
				ur := r0.u + (r1.u-r0.u)*tR
				vl := l0.v + (l1.v-l0.v)*tL
				vr := r0.v + (r1.v-r0.v)*tR
				uFP := int32((ul+tx0*(ur-ul))*float32(texW)*(1<<swFP) + 0.5)
				vFP := int32((vl+tx0*(vr-vl))*float32(texH)*(1<<swFP) + 0.5)
				du := int32((ur - ul) * float32(texW) * (1 << swFP) / float32(wSpanPix))
				dv := int32((vr - vl) * float32(texH) * (1 << swFP) / float32(wSpanPix))
				dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
				for i := 0; i < n; i++ {
					sx := int(uFP >> swFP)
					if sx < 0 {
						sx = 0
					} else if sx >= texW {
						sx = texW - 1
					}
					sy := int(vFP >> swFP)
					if sy < 0 {
						sy = 0
					} else if sy >= texH {
						sy = texH - 1
					}
					e := int(texData[sy*texStride+sx]) * 8
					swBlendAlphaOver((*[4]byte)(dst[i*4:]), int(tab[e]), int(tab[e+1]), int(tab[e+2]), int(tab[e+3]))
					uFP += du
					vFP += dv
				}
			}
			return
		}
		for py := py0; py <= py1; py++ {
			wy := float32(r.h) - float32(py) - 0.5
			tL := (wy - l0.y) / (l1.y - l0.y)
			tR := (wy - r0.y) / (r1.y - r0.y)
			ul := l0.u + (l1.u-l0.u)*tL
			ur := r0.u + (r1.u-r0.u)*tR
			vl := l0.v + (l1.v-l0.v)*tL
			vr := r0.v + (r1.v-r0.v)*tR
			uFP := int32((ul+tx0*(ur-ul))*float32(texW)*(1<<swFP) + 0.5)
			vFP := int32((vl+tx0*(vr-vl))*float32(texH)*(1<<swFP) + 0.5)
			du := int32((ur - ul) * float32(texW) * (1 << swFP) / float32(wSpanPix))
			dv := int32((vr - vl) * float32(texH) * (1 << swFP) / float32(wSpanPix))
			dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
			// mode is constant for the whole draw, so dispatch once per pixel on
			// scalars loaded straight from the table — no [3]int temporaries and no
			// old swBlendPix call (its array params and per-pixel mode switch were
			// the hottest overhead in this loop). Each case is byte-identical to
			// the matching swBlendPix branch.
			for i := 0; i < n; i++ {
				sx := int(uFP >> swFP)
				if sx < 0 {
					sx = 0
				} else if sx >= texW {
					sx = texW - 1
				}
				sy := int(vFP >> swFP)
				if sy < 0 {
					sy = 0
				} else if sy >= texH {
					sy = texH - 1
				}
				e := int(texData[sy*texStride+sx]) * 8
				p := (*[4]byte)(dst[i*4:])
				// [0..2]=premul rgb, [3]=sa, [4..6]=raw rgb (see buildPalTable).
				switch mode {
				case swBlendAddOneOne:
					swBlendAddOneOnePix(p, int(tab[e+4]), int(tab[e+5]), int(tab[e+6]), int(tab[e+3]))
				case swBlendAddSrcAlphaOne:
					swBlendAddSrcAlphaOnePix(p, int(tab[e]), int(tab[e+1]), int(tab[e+2]), int(tab[e+3]))
				case swBlendAddOneInvAlpha:
					swBlendAddOneInvAlphaPix(p, int(tab[e+4]), int(tab[e+5]), int(tab[e+6]), int(tab[e+3]))
				case swBlendAddZeroInvAlpha:
					swBlendAddZeroInvAlphaPix(p, int(tab[e+3]))
				case swBlendSubOneOne:
					swBlendSubOneOnePix(p, int(tab[e+4]), int(tab[e+5]), int(tab[e+6]), int(tab[e+3]))
				case swBlendSubSrcAlphaOne:
					swBlendSubSrcAlphaOnePix(p, int(tab[e]), int(tab[e+1]), int(tab[e+2]), int(tab[e+3]))
				default: // swBlendReplace
					swBlendReplacePix(p, int(tab[e+4]), int(tab[e+5]), int(tab[e+6]), int(tab[e+3]))
				}
				uFP += du
				vFP += dv
			}
		}
		return
	}

	// RGBA source.
	if tex.filter {
		for py := py0; py <= py1; py++ {
			wy := float32(r.h) - float32(py) - 0.5
			tL := (wy - l0.y) / (l1.y - l0.y)
			tR := (wy - r0.y) / (r1.y - r0.y)
			ul := l0.u + (l1.u-l0.u)*tL
			ur := r0.u + (r1.u-r0.u)*tR
			vl := l0.v + (l1.v-l0.v)*tL
			vr := r0.v + (r1.v-r0.v)*tR
			uStep := (ur - ul) / float32(wSpanPix)
			vStep := (vr - vl) / float32(wSpanPix)
			u := ul + tx0*(ur-ul)
			vv := vl + tx0*(vr-vl)
			dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
			for i := 0; i < n; i++ {
				rr, gg, bb, aa := tex.sampleRGBAFiltered(u, vv)
				if q.mask == -1 {
					aa = 1
				}
				rr, gg, bb, aa = applySpritePalfx(rr, gg, bb, aa, q, true, q.alpha)
				rr, gg, bb = tintMix(rr, gg, bb, aa, q.tint)
				sa := quant(aa)
				s0, s1, s2 := quant(rr), quant(gg), quant(bb)
				sp0 := sat8(mul255(s0, sa))
				sp1 := sat8(mul255(s1, sa))
				sp2 := sat8(mul255(s2, sa))
				p := (*[4]byte)(dst[i*4:])
				// mode is constant per draw, so dispatch once per pixel on the
				// scalar source values — no [3]int temporaries and no per-pixel
				// mode switch. Each case is byte-identical to the old branch it
				// replaces.
				switch mode {
				case swBlendAddAlphaOver:
					swBlendAlphaOver(p, sp0, sp1, sp2, sa)
				case swBlendAddOneOne:
					swBlendAddOneOnePix(p, s0, s1, s2, sa)
				case swBlendAddSrcAlphaOne:
					swBlendAddSrcAlphaOnePix(p, sp0, sp1, sp2, sa)
				case swBlendAddOneInvAlpha:
					swBlendAddOneInvAlphaPix(p, s0, s1, s2, sa)
				case swBlendAddZeroInvAlpha:
					swBlendAddZeroInvAlphaPix(p, sa)
				case swBlendSubOneOne:
					swBlendSubOneOnePix(p, s0, s1, s2, sa)
				case swBlendSubSrcAlphaOne:
					swBlendSubSrcAlphaOnePix(p, sp0, sp1, sp2, sa)
				default: // swBlendReplace
					swBlendReplacePix(p, s0, s1, s2, sa)
				}
				u += uStep
				vv += vStep
			}
		}
		return
	}

	// RGBA nearest: fixed-point texel stepping, per-pixel PalFX.
	bpp := int(tex.depth / 8)
	if bpp < 1 {
		bpp = 1
	}
	for py := py0; py <= py1; py++ {
		wy := float32(r.h) - float32(py) - 0.5
		tL := (wy - l0.y) / (l1.y - l0.y)
		tR := (wy - r0.y) / (r1.y - r0.y)
		ul := l0.u + (l1.u-l0.u)*tL
		ur := r0.u + (r1.u-r0.u)*tR
		vl := l0.v + (l1.v-l0.v)*tL
		vr := r0.v + (r1.v-r0.v)*tR
		uFP := int32((ul+tx0*(ur-ul))*float32(texW)*(1<<swFP) + 0.5)
		vFP := int32((vl+tx0*(vr-vl))*float32(texH)*(1<<swFP) + 0.5)
		du := int32((ur - ul) * float32(texW) * (1 << swFP) / float32(wSpanPix))
		dv := int32((vr - vl) * float32(texH) * (1 << swFP) / float32(wSpanPix))
		dst := r.pix[py*r.pitch+px0*4 : py*r.pitch+(px1+1)*4]
		for i := 0; i < n; i++ {
			sx := int(uFP >> swFP)
			if sx < 0 {
				sx = 0
			} else if sx >= texW {
				sx = texW - 1
			}
			sy := int(vFP >> swFP)
			if sy < 0 {
				sy = 0
			} else if sy >= texH {
				sy = texH - 1
			}
			o := sy*texStride + sx*bpp
			rr := float32(texData[o]) / 255
			gg := float32(texData[o+1]) / 255
			bb := float32(texData[o+2]) / 255
			var aa float32 = 1
			if tex.depth >= 32 {
				aa = float32(texData[o+3]) / 255
			}
			if q.mask == -1 {
				aa = 1
			}
			rr, gg, bb, aa = applySpritePalfx(rr, gg, bb, aa, q, true, q.alpha)
			rr, gg, bb = tintMix(rr, gg, bb, aa, q.tint)
			sa := quant(aa)
			s0, s1, s2 := quant(rr), quant(gg), quant(bb)
			sp0 := sat8(mul255(s0, sa))
			sp1 := sat8(mul255(s1, sa))
			sp2 := sat8(mul255(s2, sa))
			p := (*[4]byte)(dst[i*4:])
			// Same scalar mode dispatch as the filtered loop above.
			switch mode {
			case swBlendAddAlphaOver:
				swBlendAlphaOver(p, sp0, sp1, sp2, sa)
			case swBlendAddOneOne:
				swBlendAddOneOnePix(p, s0, s1, s2, sa)
			case swBlendAddSrcAlphaOne:
				swBlendAddSrcAlphaOnePix(p, sp0, sp1, sp2, sa)
			case swBlendAddOneInvAlpha:
				swBlendAddOneInvAlphaPix(p, s0, s1, s2, sa)
			case swBlendAddZeroInvAlpha:
				swBlendAddZeroInvAlphaPix(p, sa)
			case swBlendSubOneOne:
				swBlendSubOneOnePix(p, s0, s1, s2, sa)
			case swBlendSubSrcAlphaOne:
				swBlendSubSrcAlphaOnePix(p, sp0, sp1, sp2, sa)
			default: // swBlendReplace
				swBlendReplacePix(p, s0, s1, s2, sa)
			}
			uFP += du
			vFP += dv
		}
	}
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

	for py := py0; py <= py1; py++ {
		pY := float32(r.h) - float32(py) - 0.5
		dstRow := r.pix[py*r.pitch : py*r.pitch+(px1+1)*4]
		for px := px0; px <= px1; px++ {
			pX := float32(px) + 0.5
			var u, vv float32
			var inside bool
			// Triangle 1: (v0, v1, v2); shared edge v1-v2 is exclusive here.
			W1, W2, det := baryWeights(pX, pY, v0, v1, v2)
			W0 := det - W1 - W2
			if triInside(W0, W1, W2, det, true) {
				inv := 1 / det
				u = v0.u + (W1*inv)*(v1.u-v0.u) + (W2*inv)*(v2.u-v0.u)
				vv = v0.v + (W1*inv)*(v1.v-v0.v) + (W2*inv)*(v2.v-v0.v)
				inside = true
			} else {
				// Triangle 2: (v1, v2, v3); shared edge v1-v2 is inclusive.
				W1, W2, det = baryWeights(pX, pY, v1, v2, v3)
				W0 = det - W1 - W2
				if triInside(W0, W1, W2, det, false) {
					inv := 1 / det
					u = v1.u + (W1*inv)*(v2.u-v1.u) + (W2*inv)*(v3.u-v1.u)
					vv = v1.v + (W1*inv)*(v2.v-v1.v) + (W2*inv)*(v3.v-v1.v)
					inside = true
				}
			}
			if !inside {
				continue
			}
			if q.isTrapez {
				// Shader trapezoid correction: rebuild u from the fragment's
				// window x and the interpolated v.
				b0 := q.x1x2x4x3[2] + vv*(q.x1x2x4x3[0]-q.x1x2x4x3[2])
				b1 := q.x1x2x4x3[3] + vv*(q.x1x2x4x3[1]-q.x1x2x4x3[3])
				gap := b1 - b0
				if gap > 0.0001 || gap < -0.0001 {
					u = (pX - b0) / gap
				} else {
					u = 0.5
				}
			}
			shadePix((*[4]byte)(dstRow[px*4:]), u, vv, q, mode, tab)
		}
	}
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
