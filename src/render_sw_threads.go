//go:build !android && !armdevice

package main

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// swRowPool is a small persistent worker pool used to split large quads' pixel
// rows across CPU cores. Workers pull row-range closures from the jobs channel;
// the render thread runs one chunk itself and waits for the rest. Only quads
// above a pixel threshold are parallelized — small sprites stay single-threaded
// so the pool overhead never dominates.
type swRowPool struct {
	mu   sync.Mutex
	jobs chan func()
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
	swRows.jobs = make(chan func(), n)
	for i := 1; i < n; i++ {
		go func() {
			for fn := range swRows.jobs {
				fn()
				swRows.runs.Add(1)
			}
		}()
	}
}

// runRows runs fn over the inclusive row range [py0, py1] of a quad spanning
// pixel columns [px0, px1]. Large ranges are split across the worker pool; the
// main goroutine processes one chunk itself. Row chunks are disjoint, so the
// concurrent framebuffer writes never race.
func (r *Renderer_SW) runRows(px0, px1, py0, py1 int, fn func(a0, a1 int)) {
	rows := py1 - py0 + 1
	width := px1 - px0 + 1
	if rows < 2 || rows*width < swMinParallelPixels {
		fn(py0, py1)
		return
	}
	// Initialize the pool FIRST — the n<=1 check must see the real worker
	// count or the parallel path is never reached.
	initSWRows()
	if swRows.n <= 1 {
		fn(py0, py1)
		return
	}
	chunk := (rows + swRows.n - 1) / swRows.n
	var wg sync.WaitGroup
	y := py0
	submitted := 0
	for ; y <= py1 && submitted < swRows.n-1; submitted++ {
		e := y + chunk - 1
		if e > py1 {
			e = py1
		}
		y0, y1 := y, e
		wg.Add(1)
		swRows.jobs <- func() {
			fn(y0, y1)
			wg.Done()
		}
		y = e + 1
	}
	if y <= py1 {
		fn(y, py1)
	}
	wg.Wait()
}
