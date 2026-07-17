//go:build !debug

package main

// DebugMem is false in normal builds; the [Mem] instrumentation is compiled
// out entirely. Build with `-tags debug` to enable (see memdebug.go).
const DebugMem = false

func memLog(format string, a ...any)                        {}
func memTextureCreated(width, height, depth int32, handle uint32, serial uint64) {}
func memTextureFreed(handle uint32, serial uint64)           {}
func memGPUBytesSub(width, height, depth int32)             {}
func memAtlasResize(oldW, oldH, newW, newH int32)         {}
func memGlyphs(low, high rune, fontCharCount, atlasCount int) {}
func memPalSlotAlloc() (used int64, peak int64)              { return 0, 0 }
func memPalSlotFree()                                        {}
func memPalSlotSetTotal(total int64)                         {}
func memPalhashAlloc()                                        {}
func memMonitorStart()                                       {}

