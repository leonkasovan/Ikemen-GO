//go:build debug

package main

import (
	"sync/atomic"
	"testing"
)

// ---------------------------------------------------------------------------
// Memory tracking tests — only compiled with `-tags debug`
// ---------------------------------------------------------------------------

func TestMemTracking(t *testing.T) {
	if !DebugMem {
		t.Skip("build with -tags debug to test memory tracking")
	}

	// Reset counters
	atomic.StoreInt64(&memTextureAlive, 0)
	atomic.StoreUint64(&memGPUBytes, 0)
	atomic.StoreUint64(&memGPUBytesPeak, 0)

	// Create 256×256×32 texture
	memTextureCreated(256, 256, 32, 1, 1)
	assertEq(t, int64(1), atomic.LoadInt64(&memTextureAlive), "textures alive")
	assertEq(t, uint64(256*256*4), atomic.LoadUint64(&memGPUBytes), "gpu bytes")
	assertEq(t, uint64(256*256*4), atomic.LoadUint64(&memGPUBytesPeak), "peak gpu bytes")

	// Create larger texture (updates peak)
	memTextureCreated(512, 512, 32, 2, 2)
	expected := uint64(256*256*4 + 512*512*4)
	assertEq(t, expected, atomic.LoadUint64(&memGPUBytes), "total gpu bytes")
	assertEq(t, expected, atomic.LoadUint64(&memGPUBytesPeak), "peak after larger")

	// Create another texture (total grows, peak follows)
	memTextureCreated(128, 128, 32, 3, 3)
	expected = uint64(256*256*4 + 512*512*4 + 128*128*4)
	assertEq(t, expected, atomic.LoadUint64(&memGPUBytes), "total after third")
	assertEq(t, expected, atomic.LoadUint64(&memGPUBytesPeak), "peak after third (total grew)")

	// Free one texture — total drops but peak stays at max
	memTextureFreed(1, 1)
	memGPUBytesSub(256, 256, 32)
	assertEq(t, int64(2), atomic.LoadInt64(&memTextureAlive), "after free")
	assertEq(t, uint64(512*512*4+128*128*4), atomic.LoadUint64(&memGPUBytes), "after free bytes")

	// Peak never drops after freeing
	assertEq(t, expected, atomic.LoadUint64(&memGPUBytesPeak), "peak never decreases")
}

func TestMemBpp(t *testing.T) {
	if !DebugMem {
		t.Skip("build with -tags debug to test memory tracking")
	}

	tests := []struct {
		depth int32
		want  int32
	}{
		{8, 1},
		{24, 3},
		{32, 4},
		{16, 2},
		{0, 1}, // minimum 1
		{1, 1}, // minimum 1
	}
	for _, tt := range tests {
		got := memBpp(tt.depth)
		if got != tt.want {
			t.Errorf("memBpp(%d) = %d, want %d", tt.depth, got, tt.want)
		}
	}
}
