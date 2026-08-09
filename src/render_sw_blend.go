//go:build !android && !armdevice

package main

import (
	"fmt"
	"math"
)

// Per-pixel color math for the software renderer, ported from the GL33 sprite
// shader (shaders/sprite.frag.glsl) and the sibling C++ software renderer
// (ikemen-new-ultra/docs/SDL_SW_RENDERER.md). All blending is emulated on the
// CPU into the framebuffer bytes (R,G,B,A).

const swFP = 16 // fixed-point fractional bits for the rasterizer

// mul255 multiplies two 0..255 values and divides by 255 (rounded). This is the
// single hottest helper in the software rasterizer — called up to 5x per
// blended pixel from the swBlend*Pix helpers — so it must not use an integer
// division. ((a*b + 127) * 32897) >> 23 is a magic-multiply that is
// byte-exact with (a*b+127)/255 on all 65536 input pairs (verified
// exhaustively) while staying division-free, so blending matches the original
// rounding exactly (no visible shift). The intermediate (a*b+127)*32897 peaks
// at 2143305344, safely inside int32 (and int on 32-bit builds).
func mul255(a, b int) int { return ((a*b + 127) * 32897) >> 23 }

// mul255SASATab precomputes mul255(sa, sa) for sa in 0..255. The alpha-over
// and SrcAlpha-One formulas evaluate it per pixel; the 256-entry lookup
// replaces the 4-op magic-multiply chain with a single hot-L1 load. Byte-exact
// by construction (each entry is mul255(i, i), asserted by TestSWMul255SASATab).
var mul255SASATab [256]byte

func init() {
	for i := range mul255SASATab {
		mul255SASATab[i] = byte(mul255(i, i))
	}
}

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

// swBlendMode resolves the per-draw blend state to a mode constant. States the
// rasterizer cannot emulate fall back to swBlendReplace (opaque overwrite); each
// unsupported combination is reported once via warnUnmappedBlend instead of
// failing silently, so a new blend combo reaching this backend shows up in the
// log as an error instead of as mysteriously opaque sprites.
func (r *Renderer_SW) swBlendMode(q *swQuadState) int {
	if !q.blending {
		// Explicitly disabled blending: Replace is the intended result, not a
		// degradation, so it is not reported.
		return swBlendReplace
	}
	if q.eq == BlendReverseSubtract {
		switch {
		case q.src == BlendOne && q.dst == BlendOne:
			return swBlendSubOneOne
		case q.src == BlendSrcAlpha && q.dst == BlendOne:
			return swBlendSubSrcAlphaOne
		}
		r.warnUnmappedBlend(q)
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
	r.warnUnmappedBlend(q)
	return swBlendReplace
}

// warnUnmappedBlend logs one error per unique unsupported blend state (the same
// dedup pattern as the custom-shader warning in RenderQuad). BlendDstColor /
// BlendOneMinusDstColor factors are only produced by the model path today,
// which is disabled on this backend, so this is expected to stay quiet — if it
// fires, the new state should be added to the switches above.
func (r *Renderer_SW) warnUnmappedBlend(q *swQuadState) {
	// Nil-safe: newSWTestRenderer constructs the renderer without Init.
	if r.blendStateLogged == nil {
		r.blendStateLogged = make(map[string]bool)
	}
	key := fmt.Sprintf("%d/%d/%d", q.eq, q.src, q.dst)
	if r.blendStateLogged[key] {
		return
	}
	r.blendStateLogged[key] = true
	LogError("[SDL2 Software] Unsupported blend state (eq=%s src=%s dst=%s) fell back to opaque Replace; sprites using it will not blend",
		swBlendEqName(q.eq), swBlendFuncName(q.src), swBlendFuncName(q.dst))
}

func swBlendEqName(e BlendEquation) string {
	switch e {
	case BlendAdd:
		return "Add"
	case BlendReverseSubtract:
		return "ReverseSubtract"
	}
	return fmt.Sprintf("BlendEquation(%d)", e)
}

func swBlendFuncName(f BlendFunc) string {
	switch f {
	case BlendOne:
		return "One"
	case BlendZero:
		return "Zero"
	case BlendSrcAlpha:
		return "SrcAlpha"
	case BlendOneMinusSrcAlpha:
		return "OneMinusSrcAlpha"
	case BlendDstColor:
		return "DstColor"
	case BlendOneMinusDstColor:
		return "OneMinusDstColor"
	}
	return fmt.Sprintf("BlendFunc(%d)", f)
}

// swBlendAlphaOver blends one pixel with Add/SrcAlpha/OneMinusSrcAlpha using
// premultiplied source scalars — the dominant blend mode in the software
// rasterizer (characters, lifebars, effects). The destination is a *[4]byte
// (not a slice) so the hot pixel loops skip the per-pixel bounds checks a
// []byte param would re-emit for every dst[i]; callers convert once with
// (*[4]byte)(row[i*4:]).
func swBlendAlphaOver(dst *[4]byte, sp0, sp1, sp2, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(sp0 + dr - mul255(dr, sa))
	dst[1] = byte(sp1 + dg - mul255(dg, sa))
	dst[2] = byte(sp2 + db - mul255(db, sa))
	dst[3] = byte(int(mul255SASATab[sa]) + da - mul255(da, sa))
}

// Scalar blend helpers used by every rasterizer inner loop (paletted, RGBA
// filtered/nearest, flat, and the generic shadePix path). The blend mode is
// constant for an entire draw call, so the loops dispatch on scalars instead of
// building [3]int temporaries and switching per pixel (the former swBlendPix
// overhead). Each helper writes one framebuffer pixel through a *[4]byte (so
// the loops skip per-pixel slice bounds checks) and is byte-identical to the
// corresponding original swBlendPix case (frozen as swBlendPixRef in
// render_sw_test.go and verified by TestSWBlendHelpersMatchSwBlendPix).

// swBlendAddOneOnePix — Add, One, One (saturated add).
func swBlendAddOneOnePix(dst *[4]byte, s0, s1, s2, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(sat8(dr + s0))
	dst[1] = byte(sat8(dg + s1))
	dst[2] = byte(sat8(db + s2))
	dst[3] = byte(sat8(da + sa))
}

// swBlendAddSrcAlphaOnePix — Add, SrcAlpha, One (premultiplied source).
func swBlendAddSrcAlphaOnePix(dst *[4]byte, sp0, sp1, sp2, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(sat8(dr + sp0))
	dst[1] = byte(sat8(dg + sp1))
	dst[2] = byte(sat8(db + sp2))
	dst[3] = byte(sat8(da + int(mul255SASATab[sa])))
}

// swBlendAddOneInvAlphaPix — Add, One, OneMinusSrcAlpha.
func swBlendAddOneInvAlphaPix(dst *[4]byte, s0, s1, s2, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(dr + s0 - mul255(dr, sa))
	dst[1] = byte(dg + s1 - mul255(dg, sa))
	dst[2] = byte(db + s2 - mul255(db, sa))
	dst[3] = byte(da + sa - mul255(da, sa))
}

// swBlendAddZeroInvAlphaPix — Add, Zero, OneMinusSrcAlpha (scale dst by 1-sa).
func swBlendAddZeroInvAlphaPix(dst *[4]byte, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(dr - mul255(dr, sa))
	dst[1] = byte(dg - mul255(dg, sa))
	dst[2] = byte(db - mul255(db, sa))
	dst[3] = byte(da - mul255(da, sa))
}

// swBlendSubOneOnePix — ReverseSubtract, One, One.
func swBlendSubOneOnePix(dst *[4]byte, s0, s1, s2, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(sat8(dr - s0))
	dst[1] = byte(sat8(dg - s1))
	dst[2] = byte(sat8(db - s2))
	dst[3] = byte(sat8(da - sa))
}

// swBlendSubSrcAlphaOnePix — ReverseSubtract, SrcAlpha, One.
func swBlendSubSrcAlphaOnePix(dst *[4]byte, sp0, sp1, sp2, sa int) {
	dr, dg, db, da := int(dst[0]), int(dst[1]), int(dst[2]), int(dst[3])
	dst[0] = byte(sat8(dr - sp0))
	dst[1] = byte(sat8(dg - sp1))
	dst[2] = byte(sat8(db - sp2))
	dst[3] = byte(sat8(da - int(mul255SASATab[sa])))
}

// swBlendReplacePix — write the source color and alpha outright.
func swBlendReplacePix(dst *[4]byte, s0, s1, s2, sa int) {
	dst[0] = byte(s0)
	dst[1] = byte(s1)
	dst[2] = byte(s2)
	dst[3] = byte(sa)
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
