//go:build !android && !armdevice

package main

import "math"

// Per-pixel color math for the software renderer, ported from the GL33 sprite
// shader (shaders/sprite.frag.glsl) and the sibling C++ software renderer
// (ikemen-new-ultra/docs/SDL_SW_RENDERER.md). All blending is emulated on the
// CPU into the framebuffer bytes (R,G,B,A).

const swFP = 16 // fixed-point fractional bits for the rasterizer

func mul255(a, b int) int { return (a*b + 127) / 255 }

func sat8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// quant converts a [0,1] float color channel to an 8-bit value (rounding, like
// GL's float -> UNORM8 conversion).
func quant(v float32) int {
	if v <= 0 {
		return 0
	}
	if v >= 1 {
		return 255
	}
	return int(v*255 + 0.5)
}

func quant255(v float32) int {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return int(v + 0.5)
}

// ---- Hue shift (exact port of rgb2hsv/hsv2rgb from sprite.frag.glsl) ----

func swRgb2Hsv(r, g, b float32) (float32, float32, float32) {
	p0, p1, p2, p3 := float32(0.0), float32(-1.0/3.0), float32(2.0/3.0), float32(-1.0)
	var q0, q1, q2, q3 float32
	if b < g {
		q0, q1, q2, q3 = b, g, p2, p3
	} else {
		q0, q1, q2, q3 = g, b, p0, p1
	}
	var w0, w1, w2, w3 float32
	if q0 < r {
		w0, w1, w2, w3 = q0, q1, q2, r
	} else {
		w0, w1, w2, w3 = r, q1, q3, q0
	}
	d := w0 - float32(math.Min(float64(w3), float64(w1)))
	const e = 1.0e-10
	h := float32(math.Abs(float64(w2 + (w3-w1)/(6*d+e))))
	s := d / (w0 + e)
	return h, s, w0
}

func swHsv2Rgb(h, s, v float32) (float32, float32, float32) {
	K0, K1, K2, K3 := float32(1.0), float32(2.0/3.0), float32(1.0/3.0), float32(3.0)
	p0 := float32(math.Abs(float64(mod1f(h+K0)*6 - K3)))
	p1 := float32(math.Abs(float64(mod1f(h+K1)*6 - K3)))
	p2 := float32(math.Abs(float64(mod1f(h+K2)*6 - K3)))
	clamp := func(x float32) float32 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}
	mix0 := clamp(p0 - K0)
	mix1 := clamp(p1 - K0)
	mix2 := clamp(p2 - K0)
	m := 1 - s
	r := (m + s*mix0) * v
	g := (m + s*mix1) * v
	b := (m + s*mix2) * v
	return r, g, b
}

func mod1f(x float32) float32 {
	m := float32(math.Mod(float64(x), 1.0))
	if m < 0 {
		m += 1
	}
	return m
}

func swHueShift(r, g, b, dhue float32) (float32, float32, float32) {
	h, s, v := swRgb2Hsv(r, g, b)
	return swHsv2Rgb(mod1f(h+dhue), s, v)
}

// applySpritePalfx applies the shader's PalFX chain (sprite.frag.glsl) to a
// sampled source color. premul mirrors the shader's isFlat/isRgba branches
// (neg_base and final_add scaled by source alpha); paletted sources pass
// premul=false (neg_base=1, add unsealed). alphaMul is the per-pass alpha.
func applySpritePalfx(r, g, b, a float32, q *swQuadState, premul bool, alphaMul float32) (float32, float32, float32, float32) {
	if q.hue != 0 {
		r, g, b = swHueShift(r, g, b, q.hue)
	}
	if q.neg {
		if premul {
			r, g, b = a-r, a-g, a-b
		} else {
			r, g, b = 1-r, 1-g, 1-b
		}
	}
	if q.gray != 0 {
		avg := (r + g + b) / 3
		gr := 1 - q.gray
		r = avg + (r-avg)*gr
		g = avg + (g-avg)*gr
		b = avg + (b-avg)*gr
	}
	if premul {
		r += q.add[0] * a
		g += q.add[1] * a
		b += q.add[2] * a
	} else {
		r += q.add[0]
		g += q.add[1]
		b += q.add[2]
	}
	r *= q.mult[0]
	g *= q.mult[1]
	b *= q.mult[2]
	// The per-pass alpha multiplies the source color only for flat/RGBA sources.
	// For paletted sources the shader's final_mul.rgb stays `mult` (alpha only
	// scales the alpha channel, driving the SrcAlpha blend factor). The addalpha
	// pass 2 can carry alpha >> 1 (e.g. 64.25 with mult=0.5), so multiplying
	// RGB by alpha here would blow the color out to white (the reflection bug).
	if premul {
		r *= alphaMul
		g *= alphaMul
		b *= alphaMul
	}
	a *= alphaMul
	return r, g, b, a
}

// tintMix replicates the shader's final `c.rgb = mix(c.rgb, tint.rgb*c.a, tint.a)`.
func tintMix(r, g, b, a float32, tint [4]float32) (float32, float32, float32) {
	if tint[3] == 0 {
		return r, g, b
	}
	tr := tint[0] * a
	tg := tint[1] * a
	tb := tint[2] * a
	ta := tint[3]
	return r + (tr-r)*ta, g + (tg-g)*ta, b + (tb-b)*ta
}

// Blend modes for the inner loops. Values encode (equation, src factor, dst
// factor) so the hot loop is a single switch.
const (
	swBlendAddOneOne       = iota // Add, One, One — saturated add
	swBlendAddSrcAlphaOne         // Add, SrcAlpha, One
	swBlendAddOneInvAlpha         // Add, One, OneMinusSrcAlpha
	swBlendAddAlphaOver           // Add, SrcAlpha, OneMinusSrcAlpha
	swBlendAddZeroInvAlpha        // Add, Zero, OneMinusSrcAlpha
	swBlendSubOneOne              // ReverseSubtract, One, One
	swBlendSubSrcAlphaOne         // ReverseSubtract, SrcAlpha, One
	swBlendReplace                // no blending (or unmapped state)
)

// swBlendMode resolves the per-draw blend state to a mode constant.
func swBlendMode(q *swQuadState) int {
	if !q.blending {
		return swBlendReplace
	}
	if q.eq == BlendReverseSubtract {
		switch {
		case q.src == BlendOne && q.dst == BlendOne:
			return swBlendSubOneOne
		case q.src == BlendSrcAlpha && q.dst == BlendOne:
			return swBlendSubSrcAlphaOne
		}
		return swBlendReplace
	}
	switch {
	case q.src == BlendOne && q.dst == BlendOne:
		return swBlendAddOneOne
	case q.src == BlendSrcAlpha && q.dst == BlendOne:
		return swBlendAddSrcAlphaOne
	case q.src == BlendOne && q.dst == BlendOneMinusSrcAlpha:
		return swBlendAddOneInvAlpha
	case q.src == BlendSrcAlpha && q.dst == BlendOneMinusSrcAlpha:
		return swBlendAddAlphaOver
	case q.src == BlendZero && q.dst == BlendOneMinusSrcAlpha:
		return swBlendAddZeroInvAlpha
	}
	return swBlendReplace
}

// swBlendPix blends one pixel. s = raw source rgb, sp = source rgb premultiplied
// by source alpha, sa = source alpha (all 0..255); dst is the framebuffer byte
// row at the pixel.
func swBlendPix(dst []byte, s [3]int, sp [3]int, sa int, mode int) {
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

// buildPalTable applies the full PalFX chain (plus per-pass alpha, tint and
// premultiplication) to a palette texture once per draw call, so the inner
// loops only do a table lookup. Layout per entry (8 bytes):
//
//	[0..2] premultiplied rgb (s * sa), [3] sa, [4..6] raw rgb, [7] unused
func buildPalTable(pal *swTexture, q *swQuadState) [2048]byte {
	var tab [2048]byte
	pd := pal.data
	for i := 0; i < 256; i++ {
		o := i * 4
		r := float32(pd[o]) / 255
		g := float32(pd[o+1]) / 255
		b := float32(pd[o+2]) / 255
		a := float32(pd[o+3]) / 255
		if q.mask == -1 {
			a = 1
		}
		r, g, b, a = applySpritePalfx(r, g, b, a, q, false, q.alpha)
		r, g, b = tintMix(r, g, b, a, q.tint)
		sa := quant(a)
		sr := quant(r)
		sg := quant(g)
		sb := quant(b)
		e := i * 8
		tab[e+0] = byte(sat8(mul255(sr, sa)))
		tab[e+1] = byte(sat8(mul255(sg, sa)))
		tab[e+2] = byte(sat8(mul255(sb, sa)))
		tab[e+3] = byte(sa)
		tab[e+4] = byte(sr)
		tab[e+5] = byte(sg)
		tab[e+6] = byte(sb)
	}
	return tab
}
