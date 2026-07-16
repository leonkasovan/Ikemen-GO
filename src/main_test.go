package main

import (
	"fmt"
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests — sprite and palette allocation correctness
// ---------------------------------------------------------------------------

func TestSpritePendingData(t *testing.T) {
	spr := newSprite()
	spr.Size = [2]uint16{64, 64}
	data := make([]byte, 64*64)
	for i := range data {
		data[i] = byte(i)
	}
	spr.SetPxl(data)

	if spr.pendingDepth != 8 {
		t.Errorf("SetPxl: expected pendingDepth=8, got %d", spr.pendingDepth)
	}
	if len(spr.pendingData) != 64*64 {
		t.Errorf("SetPxl: expected pendingData len=%d, got %d", 64*64, len(spr.pendingData))
	}
	if spr.pendingW != 64 || spr.pendingH != 64 {
		t.Errorf("SetPxl: expected 64x64, got %dx%d", spr.pendingW, spr.pendingH)
	}
	if spr.pendingFilter {
		t.Error("SetPxl: pendingFilter should be false for 8-bit")
	}
	if spr.isBlank() {
		t.Error("sprite with pendingData should not be blank")
	}
}

func TestSpriteBlank(t *testing.T) {
	spr := newSprite()
	if !spr.isBlank() {
		t.Error("new sprite should be blank")
	}

	// Sprite with zero width is blank
	spr.Size = [2]uint16{0, 100}
	if !spr.isBlank() {
		t.Error("sprite with width 0 should be blank")
	}

	// Sprite with pending data is NOT blank
	spr.Size = [2]uint16{16, 16}
	spr.SetPxl(make([]byte, 256))
	if spr.isBlank() {
		t.Error("sprite with pending data should NOT be blank")
	}
}

func TestPaletteNewPal(t *testing.T) {
	pl := &PaletteList{}
	pl.init()

	idx1, pal1 := pl.NewPal()
	if idx1 != 0 {
		t.Errorf("first palette index should be 0, got %d", idx1)
	}
	if len(pal1) != 256 {
		t.Errorf("palette should have 256 colors, got %d", len(pal1))
	}

	idx2, _ := pl.NewPal()
	if idx2 != 1 {
		t.Errorf("second palette index should be 1, got %d", idx2)
	}
}

func TestPaletteSetSource(t *testing.T) {
	pl := &PaletteList{}
	pl.init()
	pl.NewPal() // index 0

	customPal := make([]uint32, 256)
	customPal[0] = 0xFF0000FF
	customPal[1] = 0xFFFF0000
	pl.SetSource(1, customPal)

	got := pl.Get(1)
	if got[0] != 0xFF0000FF {
		t.Errorf("pal[0]: expected 0xFF0000FF, got 0x%08X", got[0])
	}
	if got[1] != 0xFFFF0000 {
		t.Errorf("pal[1]: expected 0xFFFF0000, got 0x%08X", got[1])
	}

	// Out-of-range index should fall back to index 0 (which is empty / all zeros)
	gotBad := pl.Get(999)
	if gotBad == nil {
		t.Fatal("fallback should return a non-nil palette")
	}
	if len(gotBad) != 256 {
		t.Errorf("fallback palette should have 256 colors, got %d", len(gotBad))
	}
}

func TestPaletteRemap(t *testing.T) {
	pl := &PaletteList{}
	pl.init()
	pl.NewPal() // index 0, real=0
	pl.NewPal() // index 1, real=1

	pl.Remap(0, 1) // logical 0 → real 1
	got := pl.Get(0)
	// The palette at real index 1 was just allocated, it has all zeros except... actually NewPal allocates 256 uint32s, all zero.
	if len(got) != 256 {
		t.Errorf("remapped palette should have 256 colors, got %d", len(got))
	}
}

func TestPal32ToBytes(t *testing.T) {
	// 256-color palette
	pal := make([]uint32, 256)
	pal[0] = 0x00000000
	pal[1] = 0xFFFFFFFF
	pal[255] = 0xFF0000FF

	b := Pal32ToBytes(pal)
	if len(b) != 1024 {
		t.Errorf("expected 1024 bytes for 256 colors, got %d", len(b))
	}

	// Empty palette
	empty := Pal32ToBytes(nil)
	if empty != nil {
		t.Error("expected nil for empty palette")
	}

	// Short palette — should pad to 256
	short := make([]uint32, 32)
	short[0] = 0xFF00FF00
	b2 := Pal32ToBytes(short)
	if len(b2) != 1024 {
		t.Errorf("expected 1024 bytes for padded palette, got %d", len(b2))
	}
}

// ---------------------------------------------------------------------------
// Benchmarks — allocation speed and heap impact
// ---------------------------------------------------------------------------

func BenchmarkNewSprite(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		spr := newSprite()
		runtime.KeepAlive(spr)
	}
}

func BenchmarkSpriteSetPxl(b *testing.B) {
	b.ReportAllocs()
	sizes := []struct{ w, h int }{{16, 16}, {32, 32}, {64, 64}, {128, 128}}
	for _, sz := range sizes {
		b.Run(fmt.Sprintf("%dx%d", sz.w, sz.h), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				spr := newSprite()
				spr.Size = [2]uint16{uint16(sz.w), uint16(sz.h)}
				data := make([]byte, sz.w*sz.h)
				spr.SetPxl(data)
				runtime.KeepAlive(spr)
			}
		})
	}
}

func BenchmarkNewPal(b *testing.B) {
	b.ReportAllocs()
	pl := &PaletteList{}
	pl.init()
	for i := 0; i < b.N; i++ {
		idx, pal := pl.NewPal()
		runtime.KeepAlive(idx)
		runtime.KeepAlive(pal)
	}
}

func BenchmarkPal32ToBytes(b *testing.B) {
	b.ReportAllocs()
	pal := make([]uint32, 256)
	for i := range pal {
		pal[i] = uint32(i) * 0x01010101
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := Pal32ToBytes(pal)
		runtime.KeepAlive(out)
	}
}

// BenchmarkSFFLoad simulates the memory pattern of loading a font SFF
// (e.g. data/ikemen1/fonts/Action.sff with 564 glyphs). Each glyph is a
// small 8-bit sprite that gets SetPxl'd.
func BenchmarkSFFLoad(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		n := 500
		sprites := make([]*Sprite, 0, n)
		for j := 0; j < n; j++ {
			spr := newSprite()
			w, h := 16+(j*7)%40, 16+(j*13)%40
			spr.Size = [2]uint16{uint16(w), uint16(h)}
			data := make([]byte, w*h)
			spr.SetPxl(data)
			sprites = append(sprites, spr)
		}
		runtime.KeepAlive(sprites)
	}
}

// BenchmarkStartupHeapDelta measures the total heap allocation when simulating
// the full SFF loading sequence (all fonts, fight.sff, chars, stage, gofx).
// It reports B/sprite and total sprites to catch regressions from changes
// like eager ensureTex() or cache eviction policies.
func BenchmarkStartupHeapDelta(b *testing.B) {
	b.ReportAllocs()

	// SFF sizes observed during a typical startup with 2× kfm on kfm stage
	sffs := []struct {
		name  string
		count int
	}{
		{"fight.sff", 117},
		{"fightfx.sff", 310},
		{"Action.sff", 564},
		{"Menu2Small.sff", 94},
		{"pixel.sff", 94},
		{"HitNum.sff", 40},
		{"Timer.sff", 11},
		{"PowerbarNum.sff", 11},
		{"Round.sff", 14},
		{"ComboCounter.sff", 10},
		{"Menu2.sff", 94},
		{"kfm.sff (P1)", 281},
		{"kfm.sff (P2)", 281},
		{"gofx.sff", 18},
		{"stage kfm.sff", 8},
	}

	for i := 0; i < b.N; i++ {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		totalSprites := 0

		for _, sff := range sffs {
			sprites := make([]*Sprite, sff.count)
			for j := 0; j < sff.count; j++ {
				spr := newSprite()
				w, h := 20+(j*7)%80, 20+(j*13)%80
				spr.Size = [2]uint16{uint16(w), uint16(h)}
				data := make([]byte, w*h)
				spr.SetPxl(data)
				sprites[j] = spr
			}
			totalSprites += sff.count
			runtime.KeepAlive(sprites)
		}

		runtime.ReadMemStats(&after)
		var allocated uint64
		if after.HeapAlloc > before.HeapAlloc {
			allocated = after.HeapAlloc - before.HeapAlloc
		}
		b.ReportMetric(float64(allocated)/float64(totalSprites), "B/sprite")
		b.ReportMetric(float64(totalSprites), "sprites")
	}
}



// ---------------------------------------------------------------------------
// Memory overhead regression tests — catch struct bloat
// ---------------------------------------------------------------------------

func TestSpriteStructOverhead(t *testing.T) {
	var before, after runtime.MemStats

	// Settle the heap
	runtime.GC()
	runtime.ReadMemStats(&before)

	n := 20000
	sprites := make([]*Sprite, n)
	for i := 0; i < n; i++ {
		sprites[i] = newSprite()
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	// Keep the sprites alive until after the measurement, otherwise Go's
	// GC might collect them between ReadMemStats calls.
	runtime.KeepAlive(sprites)

	// Guard against uint64 underflow: GC can free background allocations between
	// the two snapshots, making after.HeapAlloc < before.HeapAlloc.
	if after.HeapAlloc <= before.HeapAlloc {
		t.Logf("heap did not increase (before=%d after=%d) after allocating %d sprites — "+
			"GC interference, skipping overhead assertion", before.HeapAlloc, after.HeapAlloc, n)
		return
	}

	totalBytes := after.HeapAlloc - before.HeapAlloc
	perSprite := float64(totalBytes) / float64(n)

	t.Logf("Sprite{} overhead: %.1f B/sprite (%d total for %d sprites)",
		perSprite, totalBytes, n)

	// A Sprite is ~208 bytes (struct fields + Texture interface pointer + 3 slices).
	// With GC/allocator overhead, 400 B/sprite is a generous upper bound.
	if perSprite > 400 {
		t.Errorf("Sprite struct overhead too high: %.1f B/sprite (limit 400)", perSprite)
	}
}

func TestSpriteWithDataOverhead(t *testing.T) {
	var before, after runtime.MemStats

	// Settle the heap first
	runtime.GC()
	runtime.ReadMemStats(&before)

	n := 1000
	sprites := make([]*Sprite, n)
	for i := 0; i < n; i++ {
		spr := newSprite()
		spr.Size = [2]uint16{32, 32}
		data := make([]byte, 32*32)
		spr.SetPxl(data)
		sprites[i] = spr
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	// Keep the sprites alive until after the measurement.
	runtime.KeepAlive(sprites)

	// Guard against uint64 underflow
	if after.HeapAlloc <= before.HeapAlloc {
		t.Logf("heap did not increase (before=%d after=%d) after allocating %d sprites — "+
			"GC interference, skipping overhead assertion", before.HeapAlloc, after.HeapAlloc, n)
		return
	}

	totalBytes := after.HeapAlloc - before.HeapAlloc
	perSprite := float64(totalBytes) / float64(n)

	t.Logf("Sprite{32×32, pendingData=%.1fKB}: %.1f B/sprite (%d total for %d sprites)",
		float64(32*32)/1024, perSprite, totalBytes, n)

	// ~208 B struct + 1024 B pixel data = ~1.2 KB.
	// Allow 2.5 KB to account for allocator rounding.
	if perSprite > 2500 {
		t.Errorf("Sprite+data overhead too high: %.1f B/sprite (limit 2500)", perSprite)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func assertEq[T comparable](t *testing.T, want, got T, msg string) {
	t.Helper()
	if want != got {
		t.Errorf("%s: want %v, got %v", msg, want, got)
	}
}
