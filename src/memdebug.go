//go:build debug

package main

import (
	"runtime"
	"sync/atomic"
	"time"
)

// DebugMem is true only when the engine is built with `-tags debug`.
// It gates the [Mem] logging used for the asset-loading / drawing
// memory analysis described in PLANS.md.
const DebugMem = true

// Live counters for the GL33 asset-texture pool.
var (
	memTextureAlive int64
	memGPUBytes     uint64
	memGPUBytesPeak uint64
	palSlotsUsed    int64 // Current palette atlas slots allocated
	palSlotsMax     int64 // Peak palette atlas slots allocated
	palSlotsTotal   int64 // Total palette atlas slots available
	palhashCount    int64 // Number of sprites with computed palhash (8 bytes each)

	// Lazy-texture evaluation counters. A sprite is "pending" once its pixel
	// data is staged (SetPxl/SetRaw) and "realized" once ensureTex() actually
	// uploads it to the GPU (i.e. it was drawn at least once). The difference
	// at shutdown is the set of sprites that were loaded but never drawn — their
	// pixel buffers stayed resident in the Go heap and never cost GPU memory.
	memSpritePending      int64 // sprites whose pixel data was staged
	memSpritePendingBytes int64 // total staged pixel bytes
	memSpriteRealized     int64 // sprites whose texture was created (drawn)
	memSpriteRealizedBytes int64 // total realized pixel bytes
)

// memSpriteStaged records that a sprite's CPU pixel buffer was staged for lazy
// (or eager) upload. Called from Sprite.SetPxl / SetRaw.
func memSpriteStaged(bytes int) {
	atomic.AddInt64(&memSpritePending, 1)
	atomic.AddInt64(&memSpritePendingBytes, int64(bytes))
}

// memSpriteDrawn records that a sprite's GPU texture was actually created from
// its staged pixel data. Called from Sprite.ensureTex when it uploads.
func memSpriteDrawn(bytes int) {
	atomic.AddInt64(&memSpriteRealized, 1)
	atomic.AddInt64(&memSpriteRealizedBytes, int64(bytes))
}

// memReportFinal logs a one-shot summary at shutdown, quantifying how many
// staged sprites were never drawn (heap-resident, never uploaded to the GPU).
func memReportFinal() {
	pending := atomic.LoadInt64(&memSpritePending)
	realized := atomic.LoadInt64(&memSpriteRealized)
	pendingBytes := atomic.LoadInt64(&memSpritePendingBytes)
	realizedBytes := atomic.LoadInt64(&memSpriteRealizedBytes)

	neverDrawn := pending - realized
	if neverDrawn < 0 {
		neverDrawn = 0
	}
	neverDrawnBytes := pendingBytes - realizedBytes
	if neverDrawnBytes < 0 {
		neverDrawnBytes = 0
	}
	var pct int64
	if pending > 0 {
		pct = neverDrawn * 100 / pending
	}
	memLog("FINAL: staged sprites=%d (%dMB) | drawn=%d (%dMB) | never-drawn=%d (%d%%, ~%dMB retained in heap, never uploaded to GPU)",
		pending, pendingBytes/1e6,
		realized, realizedBytes/1e6,
		neverDrawn, pct, neverDrawnBytes/1e6)
}

// memPalSlotSetTotal records the total number of palette slots in the atlas.
// This is set once by createPalAtlas() during renderer init.
func memPalSlotSetTotal(total int64) {
	atomic.StoreInt64(&palSlotsTotal, total)
}

func memBpp(depth int32) int32 {
	bpp := depth / 8
	if bpp < 1 {
		bpp = 1
	}
	return bpp
}

// memLog writes a [Mem] prefixed line to the engine log (stderr).
func memLog(format string, a ...any) {
	LogMessage("[Mem] "+format, a...)
}

// memTextureCreated records a GL asset-texture allocation (generateTexture).
func memTextureCreated(width, height, depth int32, handle uint32, serial uint64) {
	n := atomic.AddInt64(&memTextureAlive, 1)
	bytes := uint64(width) * uint64(height) * uint64(memBpp(depth))
	newTotal := atomic.AddUint64(&memGPUBytes, bytes)
	// Track peak GPU memory
	for {
		peak := atomic.LoadUint64(&memGPUBytesPeak)
		if newTotal <= peak || atomic.CompareAndSwapUint64(&memGPUBytesPeak, peak, newTotal) {
			break
		}
	}
	memLog("Texture created: %dx%dx%d handle=%d serial=%d alive=%d gpuBytes=%d",
		width, height, depth, handle, serial, n, newTotal)
}

// memTextureFreed records a GL asset-texture deallocation (finalizer).
func memTextureFreed(handle uint32, serial uint64) {
	n := atomic.AddInt64(&memTextureAlive, -1)
	memLog("Texture freed: handle=%d serial=%d alive=%d", handle, serial, n)
}

// memGPUBytesSub removes a freed texture's bytes estimate (finalizer).
func memGPUBytesSub(width, height, depth int32) {
	bytes := uint64(width) * uint64(height) * uint64(memBpp(depth))
	atomic.AddUint64(&memGPUBytes, ^bytes+1) // subtract via two's complement
}

// memAtlasResize logs atlas resizes. CopyData now copies the old region (B1 fixed).
func memAtlasResize(oldW, oldH, newW, newH int32) {
	cw, ch := oldW, oldH
	if cw > newW {
		cw = newW
	}
	if ch > newH {
		ch = newH
	}
	memLog("Atlas resize %dx%d -> %dx%d (CopyData copies %dx%d old region, PLANS B1 fixed)",
		oldW, oldH, newW, newH, cw, ch)
}

// memGlyphs logs font glyph/atlas growth (GenerateGlyphs).
func memGlyphs(low, high rune, fontCharCount, atlasCount int) {
	memLog("Font glyphs generated: runes=%d-%d fontChar=%d atlases=%d",
		low, high, fontCharCount, atlasCount)
}

// memMonitorStart launches a background goroutine logging GC / heap stats.
// It logs a periodic heap snapshot plus a line whenever a GC cycle runs.
// memPalSlotAlloc records one palette atlas slot allocation and returns the
// current number of used slots and the peak (max concurrent) slots so far.
func memPalSlotAlloc() (used int64, peak int64) {
	u := atomic.AddInt64(&palSlotsUsed, 1)
	// Track peak (CAS loop — same pattern as memGPUBytesPeak)
	for {
		m := atomic.LoadInt64(&palSlotsMax)
		if u <= m {
			return u, m
		}
		if atomic.CompareAndSwapInt64(&palSlotsMax, m, u) {
			return u, u
		}
	}
}

// memPalSlotFree records one palette atlas slot release (via GC finalizer).
func memPalSlotFree() {
	atomic.AddInt64(&palSlotsUsed, -1)
}

// memPalhashAlloc records that a sprite has computed a palhash (currently 8 bytes).
// Replaces the old paltemp allocation that held a full 1 KB palette copy.
func memPalhashAlloc() {
	atomic.AddInt64(&palhashCount, 1)
}

func memMonitorStart() {
	go func() {
		var lastNumGC uint32
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if m.NumGC != lastNumGC {
				pause := m.PauseNs[(m.NumGC+255)%256]
				memLog("GC #%d: pause=%dms heapAlloc=%dMB heapSys=%dMB heapObjects=%d goroutines=%d texturesAlive=%d",
					m.NumGC, pause/1e6, m.HeapAlloc/1e6, m.Sys/1e6,
					m.HeapObjects, runtime.NumGoroutine(), atomic.LoadInt64(&memTextureAlive))
				lastNumGC = m.NumGC
			}
					used := atomic.LoadInt64(&palSlotsUsed)
			peak := atomic.LoadInt64(&palSlotsMax)
			total := atomic.LoadInt64(&palSlotsTotal)
			paltemps := atomic.LoadInt64(&palhashCount)
			memLog("HEAP: alloc=%dMB sys=%dMB objects=%d texturesAlive=%d palSlots=%d/peak=%d/total=%d palthashes=%d palhashBytes=%d gpuBytes=%d peakGPUBytes=%d",
				m.HeapAlloc/1e6, m.Sys/1e6, m.HeapObjects,
				atomic.LoadInt64(&memTextureAlive),
				used, peak, total,
				paltemps, paltemps*8,
				atomic.LoadUint64(&memGPUBytes),
				atomic.LoadUint64(&memGPUBytesPeak))
			// Warn if palette slot usage exceeds the atlas capacity
			if total > 0 && used >= total {
				memLog("WARNING: Palette atlas exhausted! Used %d of %d slots — consider increasing PaletteAtlasSize",
					used, total)
			} else if total > 0 && used >= total*90/100 {
				memLog("WARNING: Palette atlas nearly full! Used %d of %d slots (%d%%)",
					used, total, used*100/total)
			}
		}
	}()
}
