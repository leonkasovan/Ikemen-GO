//go:build !android && !armdevice

package main

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// swRowPool is a small persistent worker pool used to split large quads' pixel
// rows across CPU cores. Workers pull row-range jobs from the jobs channel; the
// render thread runs one chunk itself and waits for the rest. Only quads above
// a pixel threshold are parallelized — small sprites stay single-threaded so
// the pool overhead never dominates.
type swRowPool struct {
	mu   sync.Mutex
	jobs chan *swRowJob
	n    int
	runs atomic.Int64 // worker jobs executed (used by tests to verify the pool runs)
}

var swRows = &swRowPool{}

// swMinParallelPixels is the quad area (px) above which rows are split across
// the pool. Below this the spawn/sync overhead is not worth it.
const swMinParallelPixels = 16384

// initSWRows starts the pool workers (lazily, on the first large quad).
func initSWRows() {
	swRows.mu.Lock()
	defer swRows.mu.Unlock()
	if swRows.jobs != nil {
		return
	}
	n := runtime.NumCPU()
	if n < 1 {
		n = 1
	}
	if n > 16 {
		n = 16 // memory bandwidth saturates well before this
	}
	swRows.n = n
	swRows.jobs = make(chan *swRowJob, n)
	for i := 1; i < n; i++ {
		go func() {
			for job := range swRows.jobs {
				job.draw.draw(job.a0, job.a1)
				job.wg.Done()
				swRows.runs.Add(1)
			}
		}()
	}
}

// swRowJob is one row-range chunk of a parallel quad. The draw data is copied
// by value into the pooled job (a memcpy, no allocation), so the rasterizer's
// per-quad row closures — which used to escape into the workers and heap-
// allocate a fresh capture environment per chunk — are gone. Jobs are returned
// to swRowJobPool by the submitting goroutine once every worker is done.
type swRowJob struct {
	draw swRowDraw
	a0   int
	a1   int
	wg   sync.WaitGroup
}

var swRowJobPool = sync.Pool{
	New: func() any { return &swRowJob{} },
}

// runRows runs the row draw over the inclusive row range [py0, py1] of a quad
// spanning pixel columns [d.px0, d.px1]. Large ranges are split across the
// worker pool (each chunk gets a pooled job holding a copy of the draw data);
// the main goroutine processes one chunk itself. Row chunks are disjoint, so
// the concurrent framebuffer writes never race.
func (r *Renderer_SW) runRows(d *swRowDraw, py0, py1 int) {
	rows := py1 - py0 + 1
	width := d.px1 - d.px0 + 1
	if rows < 2 || rows*width < swMinParallelPixels {
		d.draw(py0, py1)
		return
	}
	// Initialize the pool FIRST — the n<=1 check must see the real worker
	// count or the parallel path is never reached.
	initSWRows()
	if swRows.n <= 1 {
		d.draw(py0, py1)
		return
	}
	chunk := (rows + swRows.n - 1) / swRows.n
	var jobs [16]*swRowJob
	y := py0
	submitted := 0
	for ; y <= py1 && submitted < swRows.n-1; submitted++ {
		e := y + chunk - 1
		if e > py1 {
			e = py1
		}
		job := swRowJobPool.Get().(*swRowJob)
		job.draw = *d // copy the per-quad constants into the pooled job
		job.a0, job.a1 = y, e
		job.wg.Add(1)
		jobs[submitted] = job
		swRows.jobs <- job
		y = e + 1
	}
	if y <= py1 {
		d.draw(y, py1)
	}
	for i := 0; i < submitted; i++ {
		jobs[i].wg.Wait()
		swRowJobPool.Put(jobs[i])
	}
}

// Row-draw kinds: which loop swRowDraw.draw runs. The flat modes are separate
// kinds so the blend dispatch stays OUTSIDE the pixel loops (no per-pixel
// branch), exactly as the original closure-per-mode rasterRect did.
const (
	swRowFlatAO = iota // Add, SrcAlpha, OneMinusSrcAlpha
	swRowFlatOneOne
	swRowFlatSrcAlphaOne
	swRowFlatOneInvAlpha
	swRowFlatZeroInvAlpha
	swRowFlatSubOneOne
	swRowFlatSubSrcAlphaOne
	swRowFlatReplace
	swRowPalAO        // paletted, alpha-over fast path
	swRowPalGeneric   // paletted, per-pixel mode dispatch
	swRowRGBAFiltered // RGBA bilinear
	swRowRGBANearest  // RGBA nearest
	swRowGeneric      // affine-triangle path (rotation/trapezoid/tiled)
)

// swRowDraw is the per-quad row rasterizer: the loop-invariant constants the
// pixel loops need, with the loop bodies in draw(). A fresh value lives on the
// caller's stack (zero cost for small quads, which never leave it) and is
// memcpy'd into pooled jobs for the parallel path. There are no closures, so a
// large quad no longer heap-allocates a closure capture environment — the
// per-quad row closures used to be the second-largest allocation source in the
// software renderer's profiles (behind the swQuadState itself).
type swRowDraw struct {
	r        *Renderer_SW
	q        *swQuadState
	kind     int
	mode     int
	tab      []byte
	tex      *swTexture
	px0, px1 int
	n        int
	tx0      float32
	wSpanPix int
	l0, l1   swVertex // rect path: sorted vertical edges (window-left/right)
	r0, r1   swVertex
	v0, v1   swVertex // generic path: the quad's four corners
	v2, v3   swVertex
	s0, s1   int // flat path: raw quantized source
	s2       int
	sp0, sp1 int // flat path: premultiplied source (sat8(mul255(s, sa)))
	sp2      int
	sa       int
}

// draw rasterizes pixel rows [a0, a1] of the quad described by d. The row
// chunks are disjoint, so concurrent calls (pool workers + the render thread)
// never write overlapping framebuffer bytes.
func (d *swRowDraw) draw(a0, a1 int) {
	r := d.r
	pix := r.pix
	pitch := r.pitch
	switch d.kind {
	case swRowFlatAO:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendAlphaOver((*[4]byte)(dst[i*4:]), d.sp0, d.sp1, d.sp2, d.sa)
			}
		}
	case swRowFlatOneOne:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendAddOneOnePix((*[4]byte)(dst[i*4:]), d.s0, d.s1, d.s2, d.sa)
			}
		}
	case swRowFlatSrcAlphaOne:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendAddSrcAlphaOnePix((*[4]byte)(dst[i*4:]), d.sp0, d.sp1, d.sp2, d.sa)
			}
		}
	case swRowFlatOneInvAlpha:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendAddOneInvAlphaPix((*[4]byte)(dst[i*4:]), d.s0, d.s1, d.s2, d.sa)
			}
		}
	case swRowFlatZeroInvAlpha:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendAddZeroInvAlphaPix((*[4]byte)(dst[i*4:]), d.sa)
			}
		}
	case swRowFlatSubOneOne:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendSubOneOnePix((*[4]byte)(dst[i*4:]), d.s0, d.s1, d.s2, d.sa)
			}
		}
	case swRowFlatSubSrcAlphaOne:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendSubSrcAlphaOnePix((*[4]byte)(dst[i*4:]), d.sp0, d.sp1, d.sp2, d.sa)
			}
		}
	case swRowFlatReplace:
		for py := a0; py <= a1; py++ {
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				swBlendReplacePix((*[4]byte)(dst[i*4:]), d.s0, d.s1, d.s2, d.sa)
			}
		}
	case swRowPalAO:
		// Paletted alpha-over: only the premultiplied rgb + alpha from the
		// table are needed, so skip the raw-rgb loads and the generic
		// per-pixel blend call/switch entirely.
		tex := d.tex
		texW := int(tex.width)
		texH := int(tex.height)
		texStride := texW * int(tex.depth/8)
		texData := tex.data
		for py := a0; py <= a1; py++ {
			wy := float32(r.h) - float32(py) - 0.5
			tL := (wy - d.l0.y) / (d.l1.y - d.l0.y)
			tR := (wy - d.r0.y) / (d.r1.y - d.r0.y)
			ul := d.l0.u + (d.l1.u-d.l0.u)*tL
			ur := d.r0.u + (d.r1.u-d.r0.u)*tR
			vl := d.l0.v + (d.l1.v-d.l0.v)*tL
			vr := d.r0.v + (d.r1.v-d.r0.v)*tR
			uFP := int32((ul+d.tx0*(ur-ul))*float32(texW)*(1<<swFP) + 0.5)
			vFP := int32((vl+d.tx0*(vr-vl))*float32(texH)*(1<<swFP) + 0.5)
			du := int32((ur - ul) * float32(texW) * (1 << swFP) / float32(d.wSpanPix))
			dv := int32((vr - vl) * float32(texH) * (1 << swFP) / float32(d.wSpanPix))
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
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
				swBlendAlphaOver((*[4]byte)(dst[i*4:]), int(d.tab[e]), int(d.tab[e+1]), int(d.tab[e+2]), int(d.tab[e+3]))
				uFP += du
				vFP += dv
			}
		}
	case swRowPalGeneric:
		tex := d.tex
		texW := int(tex.width)
		texH := int(tex.height)
		texStride := texW * int(tex.depth/8)
		texData := tex.data
		tab := d.tab
		for py := a0; py <= a1; py++ {
			wy := float32(r.h) - float32(py) - 0.5
			tL := (wy - d.l0.y) / (d.l1.y - d.l0.y)
			tR := (wy - d.r0.y) / (d.r1.y - d.r0.y)
			ul := d.l0.u + (d.l1.u-d.l0.u)*tL
			ur := d.r0.u + (d.r1.u-d.r0.u)*tR
			vl := d.l0.v + (d.l1.v-d.l0.v)*tL
			vr := d.r0.v + (d.r1.v-d.r0.v)*tR
			uFP := int32((ul+d.tx0*(ur-ul))*float32(texW)*(1<<swFP) + 0.5)
			vFP := int32((vl+d.tx0*(vr-vl))*float32(texH)*(1<<swFP) + 0.5)
			du := int32((ur - ul) * float32(texW) * (1 << swFP) / float32(d.wSpanPix))
			dv := int32((vr - vl) * float32(texH) * (1 << swFP) / float32(d.wSpanPix))
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			// mode is constant for the whole draw, so dispatch once per pixel on
			// scalars loaded straight from the table — no [3]int temporaries and no
			// old swBlendPix call (its array params and per-pixel mode switch were
			// the hottest overhead in this loop). Each case is byte-identical to
			// the matching swBlendPix branch.
			for i := 0; i < d.n; i++ {
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
				switch d.mode {
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
	case swRowRGBAFiltered:
		tex := d.tex
		for py := a0; py <= a1; py++ {
			wy := float32(r.h) - float32(py) - 0.5
			tL := (wy - d.l0.y) / (d.l1.y - d.l0.y)
			tR := (wy - d.r0.y) / (d.r1.y - d.r0.y)
			ul := d.l0.u + (d.l1.u-d.l0.u)*tL
			ur := d.r0.u + (d.r1.u-d.r0.u)*tR
			vl := d.l0.v + (d.l1.v-d.l0.v)*tL
			vr := d.r0.v + (d.r1.v-d.r0.v)*tR
			uStep := (ur - ul) / float32(d.wSpanPix)
			vStep := (vr - vl) / float32(d.wSpanPix)
			u := ul + d.tx0*(ur-ul)
			vv := vl + d.tx0*(vr-vl)
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
				rr, gg, bb, aa := tex.sampleRGBAFiltered(u, vv)
				if d.q.mask == -1 {
					aa = 1
				}
				rr, gg, bb, aa = applySpritePalfx(rr, gg, bb, aa, d.q, true, d.q.alpha)
				rr, gg, bb = tintMix(rr, gg, bb, aa, d.q.tint)
				sa := quant(aa)
				s0, s1, s2 := quant(rr), quant(gg), quant(bb)
				sp0 := int(mul255Tab[sa][s0])
				sp1 := int(mul255Tab[sa][s1])
				sp2 := int(mul255Tab[sa][s2])
				p := (*[4]byte)(dst[i*4:])
				// mode is constant per draw, so dispatch once per pixel on the
				// scalar source values — no [3]int temporaries and no per-pixel
				// mode switch. Each case is byte-identical to the old branch it
				// replaces.
				switch d.mode {
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
	case swRowRGBANearest:
		tex := d.tex
		texW := int(tex.width)
		texH := int(tex.height)
		texStride := texW * int(tex.depth/8)
		texData := tex.data
		bpp := int(tex.depth / 8)
		if bpp < 1 {
			bpp = 1
		}
		for py := a0; py <= a1; py++ {
			wy := float32(r.h) - float32(py) - 0.5
			tL := (wy - d.l0.y) / (d.l1.y - d.l0.y)
			tR := (wy - d.r0.y) / (d.r1.y - d.r0.y)
			ul := d.l0.u + (d.l1.u-d.l0.u)*tL
			ur := d.r0.u + (d.r1.u-d.r0.u)*tR
			vl := d.l0.v + (d.l1.v-d.l0.v)*tL
			vr := d.r0.v + (d.r1.v-d.r0.v)*tR
			uFP := int32((ul+d.tx0*(ur-ul))*float32(texW)*(1<<swFP) + 0.5)
			vFP := int32((vl+d.tx0*(vr-vl))*float32(texH)*(1<<swFP) + 0.5)
			du := int32((ur - ul) * float32(texW) * (1 << swFP) / float32(d.wSpanPix))
			dv := int32((vr - vl) * float32(texH) * (1 << swFP) / float32(d.wSpanPix))
			dst := pix[py*pitch+d.px0*4 : py*pitch+(d.px1+1)*4]
			for i := 0; i < d.n; i++ {
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
				if d.q.mask == -1 {
					aa = 1
				}
				rr, gg, bb, aa = applySpritePalfx(rr, gg, bb, aa, d.q, true, d.q.alpha)
				rr, gg, bb = tintMix(rr, gg, bb, aa, d.q.tint)
				sa := quant(aa)
				s0, s1, s2 := quant(rr), quant(gg), quant(bb)
				sp0 := int(mul255Tab[sa][s0])
				sp1 := int(mul255Tab[sa][s1])
				sp2 := int(mul255Tab[sa][s2])
				p := (*[4]byte)(dst[i*4:])
				// Same scalar mode dispatch as the filtered loop above.
				switch d.mode {
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
	case swRowGeneric:
		// Affine-triangle path: per-pixel barycentric weights, PalFX and blend
		// via shadePix.
		for py := a0; py <= a1; py++ {
			pY := float32(r.h) - float32(py) - 0.5
			dstRow := pix[py*pitch : py*pitch+(d.px1+1)*4]
			for px := d.px0; px <= d.px1; px++ {
				pX := float32(px) + 0.5
				var u, vv float32
				var inside bool
				// Triangle 1: (v0, v1, v2); shared edge v1-v2 is exclusive here.
				W1, W2, det := baryWeights(pX, pY, d.v0, d.v1, d.v2)
				W0 := det - W1 - W2
				if triInside(W0, W1, W2, det, true) {
					inv := 1 / det
					u = d.v0.u + (W1*inv)*(d.v1.u-d.v0.u) + (W2*inv)*(d.v2.u-d.v0.u)
					vv = d.v0.v + (W1*inv)*(d.v1.v-d.v0.v) + (W2*inv)*(d.v2.v-d.v0.v)
					inside = true
				} else {
					// Triangle 2: (v1, v2, v3); shared edge v1-v2 is inclusive.
					W1, W2, det = baryWeights(pX, pY, d.v1, d.v2, d.v3)
					W0 = det - W1 - W2
					if triInside(W0, W1, W2, det, false) {
						inv := 1 / det
						u = d.v1.u + (W1*inv)*(d.v2.u-d.v1.u) + (W2*inv)*(d.v3.u-d.v1.u)
						vv = d.v1.v + (W1*inv)*(d.v2.v-d.v1.v) + (W2*inv)*(d.v3.v-d.v1.v)
						inside = true
					}
				}
				if !inside {
					continue
				}
				if d.q.isTrapez {
					// Shader trapezoid correction: rebuild u from the fragment's
					// window x and the interpolated v.
					b0 := d.q.x1x2x4x3[2] + vv*(d.q.x1x2x4x3[0]-d.q.x1x2x4x3[2])
					b1 := d.q.x1x2x4x3[3] + vv*(d.q.x1x2x4x3[1]-d.q.x1x2x4x3[3])
					gap := b1 - b0
					if gap > 0.0001 || gap < -0.0001 {
						u = (pX - b0) / gap
					} else {
						u = 0.5
					}
				}
				shadePix((*[4]byte)(dstRow[px*4:]), u, vv, d.q, d.mode, d.tab)
			}
		}
	default:
		// Unreachable: every swRow* kind constant has a case above. If a new
		// kind is added without a loop here, quads would silently vanish — log
		// loudly instead (same dedup philosophy as warnUnmappedBlend).
		LogError("[SDL2 Software] unhandled row draw kind %d", d.kind)
	}
}
