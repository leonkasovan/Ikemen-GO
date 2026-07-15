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
)

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
			memLog("HEAP: alloc=%dMB sys=%dMB objects=%d texturesAlive=%d gpuBytes=%d peakGPUBytes=%d",
				m.HeapAlloc/1e6, m.Sys/1e6, m.HeapObjects,
				atomic.LoadInt64(&memTextureAlive), atomic.LoadUint64(&memGPUBytes),
				atomic.LoadUint64(&memGPUBytesPeak))
		}
	}()
}
