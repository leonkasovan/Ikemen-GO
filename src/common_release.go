//go:build !debug

package main

import "time"

// DebugMem is false in normal builds; the [Mem] instrumentation is compiled
// out entirely. Build with `-tags debug` to enable (see common_debug.go).
const DebugMem = false

func memLog(format string, a ...any)                                                         {}
func memTextureCreated(tag string, width, height, depth int32, handle uint32, serial uint64) {}
func memTextureFreed(handle uint32, serial uint64)                                           {}
func memGPUBytesSub(width, height, depth int32)                                              {}
func memAtlasResize(oldW, oldH, newW, newH int32)                                            {}
func memGlyphs(low, high rune, fontCharCount, atlasCount int)                                {}
func memPalSlotAlloc() (used int64, peak int64)                                              { return 0, 0 }
func memPalSlotFree()                                                                        {}
func memPalSlotSetTotal(total int64)                                                         {}
func memPalhashAlloc()                                                                       {}
func memMonitorStart()                                                                       {}
func memSpriteStaged(bytes int)                                                              {}
func memSpriteDrawn(bytes int)                                                               {}
func memReportFinal()                                                                        {}

// startProfiler is a no-op in release builds. Build with -tags debug to
// enable the pprof HTTP server (see common_debug.go).
func startProfiler() {}

// PerfLog instrumentation is compiled out of release builds; every helper is
// a no-op so the match-loop hot path stays clean.
func perfRenderBegin() time.Time { return time.Time{} }
func perfRenderEnd(time.Time)    {}
func perfFrameRendered()         {}

func perfActionBegin() time.Time { return time.Time{} }
func perfActionEnd(time.Time)    {}

func perfCharBegin() time.Time { return time.Time{} }
func perfCharEnd(time.Time)    {}

func perfCmdBegin() time.Time { return time.Time{} }
func perfCmdEnd(time.Time)    {}

func perfPrepBegin() time.Time { return time.Time{} }
func perfPrepEnd(time.Time)    {}

func perfRunBegin() time.Time { return time.Time{} }
func perfRunEnd(time.Time)    {}

func perfFinBegin() time.Time { return time.Time{} }
func perfFinEnd(time.Time)    {}

func perfUpdBegin() time.Time { return time.Time{} }
func perfUpdEnd(time.Time)    {}

func perfFSBegin() time.Time { return time.Time{} }
func perfFSEnd(time.Time)    {}

func perfCollBegin() time.Time { return time.Time{} }
func perfCollEnd(time.Time)    {}

func perfLogicBegin() time.Time { return time.Time{} }
func perfLogicEnd(time.Time)    {}
func perfLoopIter()             {}

func perfGpuBegin() time.Time { return time.Time{} }
func perfGpuEnd(time.Time)    {}

func perfFlushBegin() time.Time { return time.Time{} }
func perfFlushEnd(time.Time)    {}

func perfSpriteHit()   {}
func perfBatchAdd(int) {}
func perfBreakFlat()   {}
func perfBreakBlend()  {}
func perfBreakRgba()   {}
func perfBreakTrapez() {}
func perfBreakMask()   {}
func perfBreakScis()   {}
func perfSlotSplit()   {}
func perfFrameLog()    {}
