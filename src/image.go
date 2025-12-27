package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"regexp"
	"runtime"
	"strings"
	"unsafe"
	"slices"
)

type TransType int32

const (
	TT_none TransType = iota
	TT_add
	TT_sub
	TT_default
)

type PalFXDef struct {
	time        int32
	color       float32
	add         [3]int32
	mul         [3]int32
	sinadd      [3]int32
	sinmul      [3]int32
	sincolor    int32
	sinhue      int32
	cycletime   [4]int32
	invertall   bool
	invertblend int32
	hue         float32
	interpolate bool
	iadd        [6]int32
	imul        [6]int32
	icolor      [2]float32
	ihue        [2]float32
	itime       int32
}

func newPalFXDef() *PalFXDef {
	return &PalFXDef{
		color:  1,
		icolor: [...]float32{1, 1},
		mul:    [...]int32{256, 256, 256},
		imul:   [...]int32{256, 256, 256, 256, 256, 256},
	}
}

type PalFX struct {
	PalFXDef
	remap        []int
	allowNeg     bool // Can use negative color math
	sintime      [4]int32
	enable       bool
	eAllowNeg    bool
	eInvertall   bool
	eInvertblend int32
	eAdd         [3]int32
	eMul         [3]int32
	eColor       float32
	eHue         float32
	eInterpolate bool
	eiAdd        [3]int32
	eiMul        [3]int32
	eiColor      float32
	eiHue        float32
	eiTime       int32
}

func newPalFX() *PalFX {
	return &PalFX{}
}

func (pf *PalFX) clearWithNeg(neg bool) {
	pf.PalFXDef = *newPalFXDef()
	pf.allowNeg = neg
	for i := 0; i < len(pf.sintime); i++ {
		pf.sintime[i] = 0
	}
}

func (pf *PalFX) clear() {
	pf.clearWithNeg(false)
}

func (pf *PalFX) getSynFx(alpha [2]int32) *PalFX {
	if pf == nil || !pf.enable {
		if alpha[0] == -2 && sys.allPalFX.enable { // Sub magic number
			if pf == nil {
				pf = newPalFX()
			}
			pf.clear()
			pf.enable = true
			pf.eMul = pf.mul
			pf.eAdd = pf.add
			pf.eColor = pf.color
			pf.eHue = pf.hue
		} else {
			return sys.allPalFX
		}
	}
	if !sys.allPalFX.enable {
		return pf
	}
	synth := *pf
	synth.synthesize(sys.allPalFX, alpha)
	return &synth
}

func (pf *PalFX) getFxPal(pal []uint32, neg bool) []uint32 {
	p := pf.getSynFx([2]int32{0, 0})
	if !p.enable {
		return pal
	}
	if !p.eAllowNeg {
		neg = false
	}
	var m [3]int32
	if neg {
		for i := range m {
			m[i] = (p.eMul[(i+1)%3] + p.eMul[(i+2)%3]) >> 1
		}
	} else {
		m = p.eMul
	}
	a, sub := p.eAdd, uint32(0)
	for i := range a {
		if neg {
			a[i] *= -1
		}
		su := uint32(0)
		if a[i] < 0 {
			su = uint32(-Max(-255, a[i]))
			a[i] = 0
		}
		m[i] = Clamp(m[i], 0, 255*256)
		a[i] = Min(255*256*256/Max(1, m[i]), a[i])
		sub |= su << uint(i*8)
	}
	for i, c := range pal {
		alpha := c & 0xff000000
		if p.eInvertall {
			c = ^c
		}
		ac := float32(c&0xff+c>>8&0xff+c>>16&0xff) / 3
		c = uint32(float32(c&0xff)+(ac-float32(c&0xff))*(1-p.eColor)) |
			uint32(float32(c>>8&0xff)+(ac-float32(c>>8&0xff))*(1-p.eColor))<<8 |
			uint32(float32(c>>16&0xff)+(ac-float32(c>>16&0xff))*(1-p.eColor))<<16
		tmp := ((^c&sub)<<1 + (^c^sub)&0xfefefefe) & 0x01010100
		c = (c - sub + tmp) & ^(tmp - tmp>>8)
		tmp = (c&0xff + uint32(a[0])) * uint32(m[0]) >> 8
		tmp = (tmp|uint32(-Btoi(tmp&0xff00 != 0)))&0xff |
			(((c>>8&0xff)+uint32(a[1]))*uint32(m[1])>>8)<<8
		tmp = (tmp|uint32(-Btoi(tmp&0xff0000 != 0)<<8))&0xffff |
			(((c>>16&0xff)+uint32(a[2]))*uint32(m[2])>>8)<<16
		sys.workpal[i] = tmp | uint32(-Btoi(tmp&0xff000000 != 0)<<16) |
			alpha
	}
	return sys.workpal
}

func (pf *PalFX) getFcPalFx(transNeg bool, alpha [2]int32) (neg bool, grayscale float32,
	add, mul [3]float32, invblend int32, hue float32) {
	p := pf.getSynFx(alpha)
	if !p.enable {
		neg = false
		grayscale = 0
		for i := range add {
			add[i] = 0
		}
		for i := range mul {
			mul[i] = 1
		}
		return
	}
	neg = p.eInvertall
	grayscale = 1 - p.eColor
	invblend = p.eInvertblend
	hue = -(p.eHue * 180.0) * (math.Pi / 180.0)
	if !p.eAllowNeg {
		transNeg = false
	}
	for i, v := range p.eAdd {
		add[i] = float32(v) / 255
		if transNeg {
			//add[i] *= -1
			mul[i] = float32(p.eMul[(i+1)%3]+p.eMul[(i+2)%3]) / 512
		} else {
			mul[i] = float32(p.eMul[i]) / 256
		}
	}
	return
}

func (pf *PalFX) sinAdd(color *[3]int32) {
	if pf.cycletime[0] > 1 {
		st := 2 * math.Pi * float64(pf.sintime[0])
		if pf.cycletime[0] == 2 {
			st += math.Pi / 2
		}
		sin := math.Sin(st / float64(pf.cycletime[0]))
		for i := range *color {
			(*color)[i] += int32(sin * float64(pf.sinadd[i]))
		}
	}
}

func (pf *PalFX) sinMul(color *[3]int32) {
	if pf.cycletime[1] > 1 {
		st := 2 * math.Pi * float64(pf.sintime[1])
		if pf.cycletime[1] == 2 {
			st += math.Pi / 2
		}
		sin := math.Sin(st / float64(pf.cycletime[1]))
		for i := range *color {
			(*color)[i] += int32(sin * float64(pf.sinmul[i]))
		}
	}
}

func (pf *PalFX) sinColor(color *float32) {
	if pf.cycletime[2] > 1 {
		st := 2 * math.Pi * float64(pf.sintime[2])
		if pf.cycletime[2] == 2 {
			st += math.Pi / 2.0
		}
		sin := math.Sin(st / float64(pf.cycletime[2]))

		(*color) += float32(sin * (float64(pf.sincolor) / 256.0))

	}
}

func (pf *PalFX) sinHueshift(color *float32) {
	if pf.cycletime[3] > 1 {
		st := 2 * math.Pi * float64(pf.sintime[3])
		if pf.cycletime[3] == 2 {
			st += math.Pi / 2.0
		}
		sin := math.Sin(st / float64(pf.cycletime[3]))

		(*color) += float32(sin * (float64(pf.sinhue) / 256.0))

	}
}

func (pf *PalFX) interpolationUpdate() {
	if pf.eiTime < pf.itime {
		pf.eiTime++
	}
	t := float32(pf.eiTime) / float32(pf.itime)
	for i := 0; i < 3; i++ {
		pf.eiMul[i] = int32(Lerp(float32(pf.imul[i+3]), float32(pf.imul[i]), t))
		pf.eMul[i] = int32(float32(pf.eiMul[i]) * float32(pf.mul[i]) / 256)
		pf.eiAdd[i] = int32(Lerp(float32(pf.iadd[i+3]), float32(pf.iadd[i]), t))
		pf.eAdd[i] = pf.eiAdd[i] + pf.add[i]
	}
	pf.eiColor = Lerp(pf.icolor[1], pf.icolor[0], t)
	pf.eColor = pf.eiColor * pf.color
	pf.eiHue = Lerp(pf.ihue[1], pf.ihue[0], t)
	pf.eHue = pf.eiHue + pf.hue
}

func (pf *PalFX) step() {
	pf.enable = pf.time != 0
	if pf.enable {
		pf.eInterpolate = pf.interpolate
		if pf.eInterpolate {
			pf.interpolationUpdate()
		} else {
			pf.eMul = pf.mul
			pf.eAdd = pf.add
			pf.eColor = pf.color
			pf.eHue = pf.hue
		}
		pf.eInvertall = pf.invertall
		if pf.invertblend <= -2 && pf.eInvertall {
			pf.eInvertblend = 3
		} else {
			pf.eInvertblend = pf.invertblend
		}
		pf.eAllowNeg = pf.allowNeg
		pf.sinAdd(&pf.eAdd)
		pf.sinMul(&pf.eMul)
		pf.sinColor(&pf.eColor)
		pf.sinHueshift(&pf.eHue)
		if sys.tickFrame() {
			for i := 0; i < 4; i++ {
				if pf.cycletime[i] > 0 {
					pf.sintime[i] = (pf.sintime[i] + 1) % pf.cycletime[i]
				}
			}
			if pf.time > 0 {
				pf.time--
			}
		}
	}
}

func (pf *PalFX) synthesize(pfx *PalFX, alpha [2]int32) {
	if alpha[0] == -2 { // Sub magic number
		for i, a := range pfx.eAdd {
			pf.eAdd[i] = Clamp(pf.eAdd[i]-Abs(a), 0, 255)
			pf.eMul[i] = Clamp(pf.eMul[i]-a, 0, 255)
		}
	} else {
		for i, a := range pfx.eAdd {
			pf.eAdd[i] += a
		}
	}
	for i, m := range pfx.eMul {
		pf.eMul[i] = pf.eMul[i] * m / 256
	}

	pf.eHue += pfx.eHue
	pf.eColor *= pfx.eColor
	pf.eInvertall = pf.eInvertall != pfx.eInvertall

	if pfx.invertall {
		// Char blend inverse
		if pfx.invertblend == 1 {
			if alpha != [2]int32{0, 0} && pf.invertblend > -3 {
				pf.eInvertall = pf.invertall
			}
			switch {
			case pf.invertblend == 0:
				pf.eInvertblend = 1
			case pf.invertblend == 1:
				pf.eInvertblend = 0
			case pf.invertblend == -2:
				if pf.eInvertall {
					pf.eInvertall = false
					pf.eInvertblend = -2
				} else {
					pf.eInvertblend = 3
				}
			case pf.invertblend == 2:
				pf.eInvertall = pf.invertall
				pf.eInvertblend = -1
			case pf.invertblend == -1:
				pf.eInvertall = pf.invertall
				pf.eInvertblend = 2
			}
		}

		// Bg blend inverse
		if pf.invertblend == -3 {
			if pf.eInvertall {
				pf.eInvertblend = 3
			} else {
				pf.eInvertall = false
				pf.eInvertblend = -3
			}
		}
	}

}

func (pf *PalFX) setColor(r, g, b int32) {
	rNormalized := Clamp(r, 0, 255)
	gNormalized := Clamp(g, 0, 255)
	bNormalized := Clamp(b, 0, 255)

	pf.enable = true
	pf.eColor = 1
	pf.eHue = 0
	pf.eMul = [...]int32{
		256 * rNormalized >> 8,
		256 * gNormalized >> 8,
		256 * bNormalized >> 8,
	}
}

type PaletteList struct {
	palettes   [][]uint32
	paletteMap []int
	PalTable   map[[2]uint16]int
	numcols    map[[2]uint16]int

	// CHANGED: Texture Array Management
	ArrayTexture Texture // Single handle for the GL_TEXTURE_2D_ARRAY
	LayerSlots   []int   // Maps logical palette index (i) -> Physical GPU Array Layer
	FreeLayers   []int   // Stack of available (empty) layers for reuse
}

func (pl *PaletteList) init() {
	pl.palettes = nil
	pl.paletteMap = nil
	pl.PalTable = make(map[[2]uint16]int)
	pl.numcols = make(map[[2]uint16]int)
	
	// Initialize Texture Array logic
	// Note: Requires active GL context. If init() runs before window creation, 
	// check ArrayTexture == nil in SetSource and initialize lazily.
	// CHANGED: Defer Texture Creation to the Main Thread.
	// This prevents crashing when init() is called from a background loader.
	sys.mainThreadTask <- func() {
		pl.ArrayTexture = gfx.newPaletteTextureArray(int32(sys.cfg.Config.PaletteMax))
	}
	pl.LayerSlots = nil
	
	// Fill FreeLayers with all available indices (e.g., 255 down to 0)
	pl.FreeLayers = make([]int, 0, sys.cfg.Config.PaletteMax)
	for i := sys.cfg.Config.PaletteMax - 1; i >= 0; i-- {
		pl.FreeLayers = append(pl.FreeLayers, i)
	}
}

func (pl *PaletteList) SetSource(i int, p []uint32) {
	// 1. Manage PaletteMap (Remapping)
	if i < len(pl.paletteMap) {
		pl.paletteMap[i] = i
	} else {
		for i > len(pl.paletteMap) {
			pl.paletteMap = append(pl.paletteMap, len(pl.paletteMap))
		}
		pl.paletteMap = append(pl.paletteMap, i)
	}

	// 2. Manage Palettes (CPU Data)
	if i < len(pl.palettes) {
		pl.palettes[i] = p
	} else {
		for i > len(pl.palettes) {
			pl.palettes = append(pl.palettes, nil)
		}
		pl.palettes = append(pl.palettes, p)
	}

	// 3. Manage LayerSlots (GPU Index Mapping)
	if i >= len(pl.LayerSlots) {
		oldLen := len(pl.LayerSlots)
		targetLen := i + 1
		newSlots := make([]int, targetLen)
		copy(newSlots, pl.LayerSlots)
		pl.LayerSlots = newSlots
		for j := oldLen; j < targetLen; j++ {
			pl.LayerSlots[j] = -1
		}
	}

	// 4. Upload to Texture Array (Thread Safe)
	if p != nil {
		layer := pl.LayerSlots[i]
		if layer == -1 {
			if len(pl.FreeLayers) > 0 {
				layer = pl.FreeLayers[len(pl.FreeLayers)-1]
				pl.FreeLayers = pl.FreeLayers[:len(pl.FreeLayers)-1]
				pl.LayerSlots[i] = layer
			} else {
				return
			}
		}

		// CHANGED: Wrap OpenGL call in mainThreadTask
		// We capture 'p' (which is referenced by pl.palettes, so it won't be GC'd)
		// and 'layer' to use in the closure.
		sys.mainThreadTask <- func() {
			if len(p) > 0 {
				// unsafe.Slice requires the "unsafe" package to be imported
				byteData := unsafe.Slice((*byte)(unsafe.Pointer(&p[0])), len(p)*4)
				pl.ArrayTexture.SetPaletteLayer(byteData, int32(layer))
			}
		}
	}
}

func (pl *PaletteList) NewPal() (i int, p []uint32) {
	i, p = len(pl.palettes), make([]uint32, 256)
	pl.SetSource(i, p)
	return
}

func (pl *PaletteList) Get(i int) []uint32 {
	if i >= 0 && i < len(pl.paletteMap) {
		return pl.palettes[pl.paletteMap[i]]
	}
	return nil
}

// NEW: Helper to get the actual GPU layer index for rendering
// This handles the remapping logic (paletteMap) automatically.
func (pl *PaletteList) GetLayer(i int) float32 {
	if i < 0 || i >= len(pl.paletteMap) {
		return 0 // Return default layer 0 if out of bounds
	}
	// 1. Get the remapped index
	remappedIndex := pl.paletteMap[i]
	
	// 2. Check bounds of LayerSlots
	if remappedIndex < 0 || remappedIndex >= len(pl.LayerSlots) {
		return 0
	}

	// 3. Return the physical GPU layer
	layer := pl.LayerSlots[remappedIndex]
	if layer == -1 {
		return 0 // Not uploaded yet
	}
	return float32(layer)
}

func (pl *PaletteList) Remap(source int, destination int) {
	pl.paletteMap[source] = destination
}

func (pl *PaletteList) ResetRemap() {
	for i := range pl.paletteMap {
		pl.paletteMap[i] = i
	}
}

func (pl *PaletteList) GetPalMap() []int {
	pm := make([]int, len(pl.paletteMap))
	copy(pm, pl.paletteMap)
	return pm
}

func (pl *PaletteList) SwapPalMap(palMap *[]int) bool {
	if len(*palMap) != len(pl.paletteMap) {
		return false
	}
	*palMap, pl.paletteMap = pl.paletteMap, *palMap
	return true
}

// func PaletteToTexture(pal []uint32) Texture {
// 	// CHANGED: Use NewPaletteTextureArray(1) instead of newPaletteTexture()
// 	// This creates a Texture Array with Depth=1, compatible with 'sampler2DArray'.
// 	tx := gfx.newPaletteTextureArray(1)

// 	// Safely handle invalid palettes
// 	if len(pal) == 0 {
// 		sys.errLog.Printf("Invalid palette texture. Defaulting to none")
// 		// No data to upload, just return the empty array texture
// 		return tx
// 	}

// 	// CHANGED: Use UploadPaletteLayer (Layer 0) instead of SetData.
// 	// SetData usually defaults to GL_TEXTURE_2D, which would fail here.
// 	byteData := unsafe.Slice((*byte)(unsafe.Pointer(&pal[0])), len(pal)*4)
// 	tx.SetPaletteLayer(byteData, 0)

// 	return tx
// }

type SffHeader struct {
	Ver0, Ver1, Ver2, Ver3   byte
	FirstSpriteHeaderOffset  uint32
	FirstPaletteHeaderOffset uint32
	NumberOfSprites          uint32
	NumberOfPalettes         uint32
}

func (sh *SffHeader) Read(r io.Reader, lofs *uint32, tofs *uint32) error {
	buf := make([]byte, 12)
	n, err := r.Read(buf)
	if err != nil {
		return err
	}
	if string(buf[:n]) != "ElecbyteSpr\x00" {
		return Error("Unrecognized SFF file, invalid header")
	}
	read := func(x interface{}) error {
		return binary.Read(r, binary.LittleEndian, x)
	}
	if err := read(&sh.Ver3); err != nil {
		return err
	}
	if err := read(&sh.Ver2); err != nil {
		return err
	}
	if err := read(&sh.Ver1); err != nil {
		return err
	}
	if err := read(&sh.Ver0); err != nil {
		return err
	}
	var dummy uint32
	if err := read(&dummy); err != nil {
		return err
	}
	switch sh.Ver0 {
	case 1:
		sh.FirstPaletteHeaderOffset, sh.NumberOfPalettes = 0, 0
		if err := read(&sh.NumberOfSprites); err != nil {
			return err
		}
		if err := read(&sh.FirstSpriteHeaderOffset); err != nil {
			return err
		}
		if err := read(&dummy); err != nil {
			return err
		}
	case 2:
		for i := 0; i < 4; i++ {
			if err := read(&dummy); err != nil {
				return err
			}
		}
		if err := read(&sh.FirstSpriteHeaderOffset); err != nil {
			return err
		}
		if err := read(&sh.NumberOfSprites); err != nil {
			return err
		}
		if err := read(&sh.FirstPaletteHeaderOffset); err != nil {
			return err
		}
		if err := read(&sh.NumberOfPalettes); err != nil {
			return err
		}
		if err := read(lofs); err != nil {
			return err
		}
		if err := read(&dummy); err != nil {
			return err
		}
		if err := read(tofs); err != nil {
			return err
		}
	default:
		return Error("Unrecognized SFF version")
	}
	return nil
}

type Sprite struct {
	Pal      []uint32
	Tex      Texture
	UV       [4]float32 // u1,v1,u2,v2
	Group    uint16     // Group index: valid range 0–65535
	Number   uint16     // Sprite index: valid range 0–65535
	Size     [2]uint16
	Offset   [2]int16
	palidx   int
	rle      int
	coldepth byte
	paltemp  []uint32
	PalTex   Texture
	Sff      *Sff // Reference to parent SFF
}

func (s *Sprite) isBlank() bool {
	return s.Tex == nil || s.Size[0] == 0 || s.Size[1] == 0
}

func newSprite() *Sprite {
	return &Sprite{palidx: -1, UV: [4]float32{0, 0, 0, 0}}
}

func (s *Sprite) shareCopy(src *Sprite) {
	s.Pal = src.Pal
	s.Tex = src.Tex
	s.Size = src.Size
	if s.palidx < 0 {
		s.palidx = src.palidx
	}
	s.coldepth = src.coldepth
	//s.paltemp = src.paltemp
	//s.PalTex = src.PalTex
}

func (s *Sprite) GetPal(pl *PaletteList) []uint32 {
	if len(s.Pal) > 0 || s.coldepth > 8 {
		return s.Pal
	}
	return pl.Get(int(s.palidx)) //pl.palettes[pl.paletteMap[int(s.palidx)]]
}

// Replace the existing GetPalTex function with this one
func (s *Sprite) GetPalTex(pl *PaletteList) Texture {
	if pl == nil {
		return nil
	}
	// CHANGED: Return the shared Array Texture.
	// The specific layer index is retrieved via pl.GetLayer(s.palidx)
	// which is handled in the renderer (anim.go).
	return pl.ArrayTexture
}

func (s *Sprite) SetPxl(px []byte, atlas_8 *TextureAtlas) {
	if len(px) == 0 {
		return
	}
	if int64(len(px)) != int64(s.Size[0])*int64(s.Size[1]) {
		return
	}
	sys.mainThreadTask <- func() {
		if s.Sff == nil {
			fmt.Printf("Sff null but has Sprite %vx%v\n", s.Size[0], s.Size[1])
			return
		}
		if atlas_8 == nil { // No atlas, create individual texture
			s.Tex = gfx.newTexture(int32(s.Size[0]), int32(s.Size[1]), 8, false)
			s.Tex.SetData(px)
			// fmt.Printf("Warning: No atlas, create individual texture %vx%v.\n", s.Size[0], s.Size[1])
		} else { // Use atlas
			ok := false
			s.UV, ok = atlas_8.AddImage(int32(s.Size[0]), int32(s.Size[1]), px)
			if ok {
				s.Tex = atlas_8.texture
				// fmt.Printf("Added sprite %vx%v to atlas at UV %v,%v - %v,%v.\n", s.Size[0], s.Size[1], s.UV[0], s.UV[1], s.UV[2], s.UV[3])
			} else {
				s.Tex = nil
				fmt.Printf("Warning: Sprite atlas full. Skipping sprite %vx%v.\n", s.Size[0], s.Size[1])
			}
		}
	}
}

func (s *Sprite) SetRaw(data []byte, sprWidth int32, sprHeight int32, sprDepth int32) {
	sys.mainThreadTask <- func() {
		s.Tex = gfx.newTexture(sprWidth, sprHeight, sprDepth, sys.cfg.Video.RGBSpriteBilinearFilter)
		s.Tex.SetData(data)
	}
}

func (s *Sprite) readHeader(r io.Reader, ofs, size *uint32, link *uint16) error {
	read := func(x interface{}) error {
		return binary.Read(r, binary.LittleEndian, x)
	}
	if err := read(ofs); err != nil {
		return err
	}
	if err := read(size); err != nil {
		return err
	}
	if err := read(s.Offset[:]); err != nil {
		return err
	}
	if err := read(&s.Group); err != nil {
		return err
	}
	if err := read(&s.Number); err != nil {
		return err
	}
	if err := read(link); err != nil {
		return err
	}
	return nil
}

func (s *Sprite) readPcxHeader(r io.ReadSeeker, offset int64) error {
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("readPcxHeader seek error: %w", err)
	}
	read := func(rd io.Reader, x interface{}) error { // Helper takes io.Reader
		return binary.Read(rd, binary.LittleEndian, x)
	}
	var dummy uint16
	if err := read(r, &dummy); err != nil {
		return err
	}
	var encoding, bpp byte
	if err := read(r, &encoding); err != nil {
		return err
	}
	if err := read(r, &bpp); err != nil {
		return err
	}
	if bpp != 8 {
		return Error(fmt.Sprintf("Invalid PCX color depth: expected 8-bit, got %v", bpp))
	}
	var rect [4]uint16
	if err := read(r, rect[:]); err != nil {
		return err
	}
	if _, err := r.Seek(offset+66, io.SeekStart); err != nil { // Use r.Seek
		return fmt.Errorf("readPcxHeader seek to bpl error: %w", err)
	}
	var bpl uint16
	if err := read(r, &bpl); err != nil {
		return err
	}
	s.Size[0] = rect[2] - rect[0] + 1
	s.Size[1] = rect[3] - rect[1] + 1
	if encoding == 1 {
		s.rle = int(bpl)
	} else {
		s.rle = 0
	}
	return nil
}

func (s *Sprite) RlePcxDecode(rle []byte) (p []byte) {
	if len(rle) == 0 || s.rle <= 0 {
		return rle
	}
	p = make([]byte, int(s.Size[0])*int(s.Size[1]))
	i, j, k, w := 0, 0, 0, int(s.Size[0])
	for j < len(p) {
		n, d := 1, rle[i]
		if i < len(rle)-1 {
			i++
		}
		if d >= 0xc0 {
			n = int(d & 0x3f)
			d = rle[i]
			if i < len(rle)-1 {
				i++
			}
		}
		for ; n > 0; n-- {
			if k < w && j < len(p) {
				p[j] = d
				j++
			}
			k++
			if k == s.rle {
				k = 0
				n = 1
			}
		}
	}
	s.rle = 0
	return
}

func (s *Sprite) read(f io.ReadSeeker, sh *SffHeader, offset int64, datasize uint32,
	nextSubheader uint32, prev *Sprite, pl *PaletteList, c00 bool) error {
	if int64(nextSubheader) > offset {
		// Ignore datasize except last
		datasize = nextSubheader - uint32(offset)
	}
	read := func(x interface{}) error {
		return binary.Read(f, binary.LittleEndian, x)
	}
	var ps byte
	if err := read(&ps); err != nil {
		return err
	}
	paletteSame := ps != 0 && prev != nil
	if err := s.readPcxHeader(f, offset); err != nil {
		return err
	}
	f.Seek(offset+128, 0)
	var palSize uint32
	if c00 || paletteSame {
		palSize = 0
	} else {
		palSize = 768
	}
	if datasize < 128+palSize {
		datasize = 128 + palSize
	}
	px := make([]byte, datasize-(128+palSize))
	if err := read(px); err != nil {
		return err
	}
	if paletteSame {
		if prev != nil {
			s.palidx = prev.palidx
		}
		if s.palidx < 0 {
			s.palidx, _ = pl.NewPal()
		}
	} else {
		var pal []uint32
		s.palidx, pal = pl.NewPal()
		if c00 {
			f.Seek(offset+int64(datasize)-768, 0)
		}
		var rgb [3]byte
		for i := range pal {
			if err := read(rgb[:]); err != nil {
				return err
			}
			var alpha byte = 255
			if i == 0 {
				alpha = 0
			}
			pal[i] = uint32(alpha)<<24 | uint32(rgb[2])<<16 | uint32(rgb[1])<<8 | uint32(rgb[0])
		}
	}
	s.SetPxl(s.RlePcxDecode(px), nil)
	return nil
}

func (s *Sprite) readHeaderV2(r io.Reader, ofs *uint32, size *uint32,
	lofs uint32, tofs uint32, link *uint16) error {
	read := func(x interface{}) error {
		return binary.Read(r, binary.LittleEndian, x)
	}
	if err := read(&s.Group); err != nil {
		return err
	}
	if err := read(&s.Number); err != nil {
		return err
	}
	if err := read(s.Size[:]); err != nil {
		return err
	}
	if err := read(s.Offset[:]); err != nil {
		return err
	}
	if err := read(link); err != nil {
		return err
	}
	var format byte
	if err := read(&format); err != nil {
		return err
	}
	s.rle = -int(format)
	if err := read(&s.coldepth); err != nil {
		return err
	}
	if err := read(ofs); err != nil {
		return err
	}
	if err := read(size); err != nil {
		return err
	}
	var tmp uint16
	if err := read(&tmp); err != nil {
		return err
	}
	s.palidx = int(tmp)
	if err := read(&tmp); err != nil {
		return err
	}
	if tmp&1 == 0 {
		*ofs += lofs
	} else {
		*ofs += tofs
	}
	return nil
}

func (s *Sprite) Rle8Decode(rle []byte) (p []byte) {
	if len(rle) == 0 {
		return rle
	}
	p = make([]byte, int(s.Size[0])*int(s.Size[1]))
	i, j := 0, 0
	for j < len(p) {
		n, d := 1, rle[i]
		if i < len(rle)-1 {
			i++
		}
		if d&0xc0 == 0x40 {
			n = int(d & 0x3f)
			d = rle[i]
			if i < len(rle)-1 {
				i++
			}
		}
		for ; n > 0; n-- {
			if j < len(p) {
				p[j] = d
				j++
			}
		}
	}
	return
}

func (s *Sprite) Rle5Decode(rle []byte) (p []byte) {
	if len(rle) == 0 {
		return rle
	}
	p = make([]byte, int(s.Size[0])*int(s.Size[1]))
	i, j := 0, 0
	for j < len(p) {
		rl := int(rle[i])
		if i < len(rle)-1 {
			i++
		}
		dl := int(rle[i] & 0x7f)
		c := byte(0)
		if rle[i]>>7 != 0 {
			if i < len(rle)-1 {
				i++
			}
			c = rle[i]
		}
		if i < len(rle)-1 {
			i++
		}
		for {
			if j < len(p) {
				p[j] = c
				j++
			}
			rl--
			if rl < 0 {
				dl--
				if dl < 0 {
					break
				}
				c = rle[i] & 0x1f
				rl = int(rle[i] >> 5)
				if i < len(rle)-1 {
					i++
				}
			}
		}
	}
	return
}

func (s *Sprite) Lz5Decode(rle []byte) (p []byte) {
	if len(rle) == 0 {
		return rle
	}
	p = make([]byte, int(s.Size[0])*int(s.Size[1]))
	i, j, n := 0, 0, 0
	ct, cts, rb, rbc := rle[i], uint(0), byte(0), uint(0)
	if i < len(rle)-1 {
		i++
	}
	for j < len(p) {
		d := int(rle[i])
		if i < len(rle)-1 {
			i++
		}
		if ct&byte(1<<cts) != 0 {
			if d&0x3f == 0 {
				d = (d<<2 | int(rle[i])) + 1
				if i < len(rle)-1 {
					i++
				}
				n = int(rle[i]) + 2
				if i < len(rle)-1 {
					i++
				}
			} else {
				rb |= byte(d & 0xc0 >> rbc)
				rbc += 2
				n = int(d & 0x3f)
				if rbc < 8 {
					d = int(rle[i]) + 1
					if i < len(rle)-1 {
						i++
					}
				} else {
					d = int(rb) + 1
					rb, rbc = 0, 0
				}
			}
			for {
				if j < len(p) {
					p[j] = p[j-d]
					j++
				}
				n--
				if n < 0 {
					break
				}
			}
		} else {
			if d&0xe0 == 0 {
				n = int(rle[i]) + 8
				if i < len(rle)-1 {
					i++
				}
			} else {
				n = d >> 5
				d &= 0x1f
			}
			for ; n > 0; n-- {
				if j < len(p) {
					p[j] = byte(d)
					j++
				}
			}
		}
		cts++
		if cts >= 8 {
			ct, cts = rle[i], 0
			if i < len(rle)-1 {
				i++
			}
		}
	}
	return
}

func (s *Sprite) readV2(f io.ReadSeeker, offset int64, datasize uint32, atlas_8 *TextureAtlas) error {
	var px []byte
	var isRaw bool = false

	if s.rle > 0 {
		return nil

	} else if s.rle == 0 {
		f.Seek(offset, 0)
		px = make([]uint8, datasize)
		binary.Read(f, binary.LittleEndian, px)

		switch s.coldepth {
		case 8:
			// Do nothing, px is already in the expected format
		case 24, 32:
			isRaw = true
			s.SetRaw(px, int32(s.Size[0]), int32(s.Size[1]), int32(s.coldepth))
		default:
			return Error("Unknown color depth")
		}

	} else {
		f.Seek(offset+4, 0)
		format := -s.rle

		var rgba *image.RGBA
		var rect image.Rectangle

		if 2 <= format && format <= 4 {
			if datasize < 4 {
				datasize = 4
			}
			px = make([]byte, datasize-4)
			if err := binary.Read(f, binary.LittleEndian, px); err != nil {
				panic(err)
				//return err
			}
		}

		switch format {
		case 2:
			px = s.Rle8Decode(px)
		case 3:
			px = s.Rle5Decode(px)
		case 4:
			px = s.Lz5Decode(px)
		case 10:
			img, err := png.Decode(f)
			if err != nil {
				return err
			}
			pi, ok := img.(*image.Paletted)
			if ok {
				px = pi.Pix
			}
		case 11, 12:
			var ok bool = false
			isRaw = true

			// Decode PNG image to RGBA
			img, err := png.Decode(f)
			if err != nil {
				return err
			}

			rect = img.Bounds()
			rgba, ok = img.(*image.RGBA)

			if !ok {
				rgba = image.NewRGBA(rect)
				draw.Draw(rgba, rect, img, rect.Min, draw.Src)
			}
			s.SetRaw(rgba.Pix, int32(rect.Max.X-rect.Min.X), int32(rect.Max.Y-rect.Min.Y), 32)
		default:
			return Error("Unknown format")
		}
	}

	if !isRaw {
		s.SetPxl(px, atlas_8)
	}
	return nil
}

// Compare current palette to previous one and reuse if possible
// This saves a lot of palette operations when the same player has many sprites on screen
func (s *Sprite) CachePalette(pal []uint32) Texture {
	// 1. Fast Path: Hardware-optimized comparison
	if s.PalTex != nil && slices.Equal(s.paltemp, pal) {
		return s.PalTex
	}

	// 2. Prepare Data (Zero-Copy)
	byteData := unsafe.Slice((*byte)(unsafe.Pointer(&pal[0])), len(pal)*4)

	// 3. GPU Optimization: VRAM Reuse
	if s.PalTex == nil {
		// CHANGED: Create a 1-layer Texture Array
		// The shader expects 'sampler2DArray', so we cannot use a standard 2D texture anymore.
		s.PalTex = gfx.newPaletteTextureArray(1)
	}

	// CHANGED: Use UploadPaletteLayer (Layer 0) instead of SetData
	// SetData defaults to GL_TEXTURE_2D, which would fail or bind incorrectly.
	if len(pal) > 0 {
		s.PalTex.SetPaletteLayer(byteData, 0)
	}

	// 4. Memory Optimization: Buffer Reuse
	if cap(s.paltemp) < len(pal) {
		s.paltemp = make([]uint32, len(pal))
	}
	s.paltemp = s.paltemp[:len(pal)]
	copy(s.paltemp, pal)

	return s.PalTex
}

func (s *Sprite) Draw(x, y, xscale, yscale float32, rxadd float32, rot Rotation, fx *PalFX, window *[4]int32, palIndex float32) {
	x += float32(sys.gameWidth-320)/2 - xscale*float32(s.Offset[0])
	y += float32(sys.gameHeight-240) - yscale*float32(s.Offset[1])
	var rcx, rcy float32

	if rot.IsZero() {
		if xscale < 0 {
			x *= -1
		}
		if yscale < 0 {
			y *= -1
		}
		rcx, rcy = rcx*sys.widthScale, 0
	} else {
		rcx, rcy = (x+rcx)*sys.widthScale, y*sys.heightScale
		x, y = AbsF(xscale)*float32(s.Offset[0]), AbsF(yscale)*float32(s.Offset[1])
	}

	rp := RenderParams{
		tex:            s.Tex,
		paltex:         s.PalTex,
		PalIndex:       palIndex,
		size:           s.Size,
		x:              -x * sys.widthScale,
		y:              -y * sys.heightScale,
		tile:           notiling,
		xts:            xscale * sys.widthScale,
		xbs:            xscale * sys.widthScale,
		ys:             yscale * sys.heightScale,
		vs:             1,
		rxadd:          rxadd,
		xas:            1,
		yas:            1,
		rot:            rot,
		tint:           0,
		blendMode:      TT_add,
		blendAlpha:     [2]int32{int32(255 * sys.brightness), 0},
		mask:           0,
		pfx:            fx,
		window:         window,
		rcx:            rcx,
		rcy:            rcy,
		projectionMode: 0,
		fLength:        0,
		xOffset:        -xscale * float32(s.Offset[0]),
		yOffset:        -yscale * float32(s.Offset[1]),
		uv:             s.UV,
	}
	RenderSprite(rp)
}

type Sff struct {
	header  SffHeader
	sprites map[[2]uint16]*Sprite
	palList PaletteList
	// This is the sffCache key
	filename string
}
type Palette struct {
	palList PaletteList
}

func newSff() (s *Sff) {
	s = &Sff{sprites: make(map[[2]uint16]*Sprite)}
	s.palList.init()
	for i := uint16(1); i <= uint16(sys.cfg.Config.PaletteMax); i++ {
		s.palList.PalTable[[...]uint16{1, i}], _ = s.palList.NewPal()
	}
	return
}

func newPaldata() (p *Palette) {
	p = &Palette{}
	p.palList.init()
	for i := uint16(1); i <= uint16(sys.cfg.Config.PaletteMax); i++ {
		p.palList.PalTable[[...]uint16{1, i}], _ = p.palList.NewPal()
	}
	return
}

// A simple SFF cache storing shallow copies
type SffCacheEntry struct {
	sffData  Sff
	refCount int
}

var SffCache = map[string]*SffCacheEntry{}

func removeSFFCache(filename string) {
	if _, ok := SffCache[filename]; ok {
		delete(SffCache, filename)
	}
}

func loadSff(filename string, char bool, atlas_8 *TextureAtlas) (*Sff, error) {
	fmt.Printf("loadSff %v: %v\n", filename, atlas_8)
	// If this SFF is already in the cache, just return a copy
	if cached, ok := SffCache[filename]; ok {
		cached.refCount++
		s := cached.sffData
		return &s, nil
	}
	s := newSff()
	s.filename = filename
	f, err := OpenFile(filename)
	if err != nil {
		return nil, err
	}
	defer func() { chk(f.Close()) }()
	var lofs, tofs uint32
	if err := s.header.Read(f, &lofs, &tofs); err != nil {
		return nil, err
	}
	read := func(x interface{}) error {
		return binary.Read(f, binary.LittleEndian, x)
	}
	// fmt.Printf("SFF Version: %d.%d.%d.%d\n", s.header.Ver0, s.header.Ver1, s.header.Ver2, s.header.Ver3)
	// fmt.Printf("NumberOfSprites: %v\nNumberOfPalettes: %v\n", s.header.NumberOfSprites, s.header.NumberOfPalettes)
	if s.header.Ver0 != 1 {
		uniquePals := make(map[[2]uint16]int)
		for i := 0; i < int(s.header.NumberOfPalettes); i++ {
			f.Seek(int64(s.header.FirstPaletteHeaderOffset)+int64(i*16), 0)
			var gn_ [3]uint16
			if err := read(gn_[:]); err != nil {
				return nil, err
			}
			var link uint16
			if err := read(&link); err != nil {
				return nil, err
			}
			var ofs, siz uint32
			if err := read(&ofs); err != nil {
				return nil, err
			}
			if err := read(&siz); err != nil {
				return nil, err
			}
			var pal []uint32
			var idx int
			if old, ok := uniquePals[[...]uint16{gn_[0], gn_[1]}]; ok {
				idx = old
				pal = s.palList.Get(old)
				fmt.Printf("%v duplicated palette: %v,%v (%v/%v)\n", filename, gn_[0], gn_[1], i+1, s.header.NumberOfPalettes)
			} else if siz == 0 {
				idx = int(link)
				pal = s.palList.Get(idx)
			} else {
				f.Seek(int64(lofs+ofs), 0)
				pal = make([]uint32, 256)
				var rgba [4]byte
				for i := 0; i < int(siz)/4 && i < len(pal); i++ {
					if err := read(rgba[:]); err != nil {
						return nil, err
					}
					if s.header.Ver2 == 0 {
						if i == 0 {
							rgba[3] = 0
						} else {
							rgba[3] = 255
						}
					}
					pal[i] = uint32(rgba[3])<<24 | uint32(rgba[2])<<16 | uint32(rgba[1])<<8 | uint32(rgba[0])
				}
				idx = i
			}
			uniquePals[[...]uint16{gn_[0], gn_[1]}] = idx
			s.palList.SetSource(i, pal)
			s.palList.PalTable[[...]uint16{gn_[0], gn_[1]}] = idx
			s.palList.numcols[[...]uint16{gn_[0], gn_[1]}] = int(gn_[2])
			if i <= sys.cfg.Config.PaletteMax &&
				s.palList.PalTable[[...]uint16{1, uint16(i + 1)}] == s.palList.PalTable[[...]uint16{gn_[0], gn_[1]}] &&
				gn_[0] != 1 && gn_[1] != uint16(i+1) {
				s.palList.PalTable[[...]uint16{1, uint16(i + 1)}] = -1
			}
			if i <= sys.cfg.Config.PaletteMax && i+1 == int(s.header.NumberOfPalettes) {
				for j := i + 1; j < sys.cfg.Config.PaletteMax; j++ {
					delete(s.palList.PalTable, [...]uint16{1, uint16(j + 1)}) // Remove extra palette
				}
			}
		}
	}
	spriteList := make([]*Sprite, int(s.header.NumberOfSprites))
	var prev *Sprite
	shofs := int64(s.header.FirstSpriteHeaderOffset)
	// if useAtlas {	// Created in LoadFnt
	// 	s.atlas_8 = CreateTextureAtlas(512, 512, 8, false)
	// 	s.atlas_8.resize = true
	// }
	for i := 0; i < len(spriteList); i++ {
		f.Seek(shofs, 0)
		spriteList[i] = newSprite()
		spriteList[i].Sff = s
		var xofs, size uint32
		var indexOfPrevious uint16
		switch s.header.Ver0 {
		case 1:
			if err := spriteList[i].readHeader(f, &xofs, &size,
				&indexOfPrevious); err != nil {
				return nil, err
			}
		case 2:
			if err := spriteList[i].readHeaderV2(f, &xofs, &size,
				lofs, tofs, &indexOfPrevious); err != nil {
				return nil, err
			}
		}
		if size == 0 {
			if int(indexOfPrevious) < i {
				dst, src := spriteList[i], spriteList[int(indexOfPrevious)]
				sys.mainThreadTask <- func() {
					dst.shareCopy(src)
					dst.UV = src.UV
				}
			} else {
				spriteList[i].palidx = 0 // index out of range
			}
		} else {
			switch s.header.Ver0 {
			case 1:
				if err := spriteList[i].read(f, &s.header, shofs+32, size,
					xofs, prev, &s.palList,
					char && (prev == nil || spriteList[i].Group == 0 &&
						spriteList[i].Number == 0)); err != nil {
					return nil, err
				}
			case 2:
				if err := spriteList[i].readV2(f, int64(xofs), size, atlas_8); err != nil {
					return nil, err
				}
			}
			prev = spriteList[i]
		}
		if s.sprites[[...]uint16{spriteList[i].Group, spriteList[i].Number}] ==
			nil {
			s.sprites[[...]uint16{spriteList[i].Group, spriteList[i].Number}] =
				spriteList[i]
		}
		if s.header.Ver0 == 1 {
			shofs = int64(xofs)
		} else {
			shofs += 28
		}
	}
	SffCache[filename] = &SffCacheEntry{*s, 1}
	runtime.SetFinalizer(s, func(s *Sff) {
		if cached, ok := SffCache[filename]; ok {
			cached.refCount--
			if cached.refCount == 0 {
				delete(SffCache, filename)
			}
		}
	})
	// if atlas_8 != nil {
	// var defpal []uint32
	// if len(s.palList.palettes) > 0 {
	// 	defpal = s.palList.Get(0)
	// }
	// Schedule atlas saving on the main thread so we capture the uploaded
	// atlas contents after any queued AddImage() / SetData operations finish.
	// fmt.Printf("loadSff: scheduling delayed save of atlas_8.png (palette present=%t length=%d)", defpal != nil, len(defpal))
	// sys.mainThreadTask <- func() {
	// 	if atlas_8 == nil || atlas_8.texture == nil {
	// 		fmt.Printf("loadSff (delayed): atlas_8 or its texture is nil - cannot save atlas_8.png")
	// 		return
	// 	}
	// 	fmt.Printf("loadSff (delayed): performing SavePNG on atlas_8.png (texture valid=%t)", atlas_8.texture.IsValid())
	// 	if err := atlas_8.texture.SavePNG(filename+"_atlas_8.png", defpal); err != nil {
	// 		fmt.Printf("loadSff (delayed): SavePNG returned error: %v", err)
	// 	}
	// }
	// } else {
	// 	fmt.Printf("Atlas is empty for %v\n", filename)
	// }
	return s, nil
}

func loadCharPalettes(sff *Sff, filename string, ref int) error {
	f, err := OpenFile(filename)
	if err != nil {
		return err
	}
	h := sff.header
	read := func(x interface{}) error {
		return binary.Read(f, binary.LittleEndian, x)
	}
	var lofs, tofs uint32
	if err := h.Read(f, &lofs, &tofs); err != nil {
		return err
	}
	maxPal := int(sys.cfg.Config.PaletteMax)

	if sff.header.Ver0 != 1 {
		// SFF v2
		uniquePals := make(map[[2]uint16]int)
		loaded := make(map[int]bool)

		for headerIdx := 0; headerIdx < int(h.NumberOfPalettes); headerIdx++ {
			f.Seek(int64(h.FirstPaletteHeaderOffset)+int64(headerIdx*16), 0)

			var gn_ [3]uint16
			if err := read(gn_[:]); err != nil {
				return err
			}
			if gn_[0] != 1 || gn_[1] <= 0 {
				continue
			}

			destIdx := int(gn_[1]) - 1
			if destIdx < 0 || destIdx >= maxPal {
				continue
			}

			var link uint16
			if err := read(&link); err != nil {
				return err
			}
			var ofs, siz uint32
			if err := read(&ofs); err != nil {
				return err
			}
			if err := read(&siz); err != nil {
				return err
			}

			var pal []uint32
			// Reuse duplicate if already loaded
			if old, ok := uniquePals[[...]uint16{gn_[0], gn_[1]}]; ok {
				if old >= 0 && old < maxPal && loaded[old] {
					pal = sff.palList.Get(old)
					if pal == nil {
						continue
					}
					destIdx = old
					sys.errLog.Printf("%v duplicated palette: %v,%v (%v/%v)\n",
						filename, gn_[0], gn_[1], headerIdx+1, h.NumberOfPalettes)
				} else {
					continue
				}
			} else if siz == 0 {
				// Link must point to a previously loaded slot
				linkIdx := int(link) - 1
				if linkIdx >= 0 && linkIdx < maxPal && loaded[linkIdx] {
					pal = sff.palList.Get(linkIdx)
					if pal == nil {
						continue
					}
					destIdx = linkIdx
				} else {
					continue
				}
			} else {
				// Read palette data
				f.Seek(int64(lofs+ofs), 0)
				pal = make([]uint32, 256)
				var rgba [4]byte
				for j := 0; j < int(siz)/4 && j < len(pal); j++ {
					if err := read(rgba[:]); err != nil {
						return err
					}
					if sff.header.Ver2 == 0 {
						if j == 0 {
							rgba[3] = 0
						} else {
							rgba[3] = 255
						}
					}
					pal[j] = uint32(rgba[3])<<24 | uint32(rgba[2])<<16 | uint32(rgba[1])<<8 | uint32(rgba[0])
				}
			}
			// Register only if we have data
			if pal != nil {
				sff.palList.SetSource(destIdx, pal)
				loaded[destIdx] = true
				uniquePals[[...]uint16{gn_[0], gn_[1]}] = destIdx
				sff.palList.PalTable[[...]uint16{gn_[0], gn_[1]}] = destIdx
				sff.palList.numcols[[...]uint16{gn_[0], gn_[1]}] = int(gn_[2])
			}
		}
	} else {
		// SFF v1 (ACT files)
		c := sys.sel.charlist[ref]
		parts := strings.SplitAfterN(c.def, "/", -1)
		pathname := strings.Join(parts[:len(parts)-1], "")

		for x := 0; x < len(c.pal_files) && x < len(c.pal); x++ {
			target := int(c.pal[x]) - 1
			if target < 0 || target >= maxPal {
				continue
			}

			U, err := OpenFile(pathname + c.pal_files[x])
			if err != nil {
				fmt.Println("Failed to open " + c.pal_files[x])
				continue
			}

			pal := make([]uint32, 256)
			readOK := true
			for i := 255; i >= 0; i-- {
				var rgb [3]byte
				if _, err = io.ReadFull(U, rgb[:]); err != nil {
					readOK = false
					break
				}
				if i != 0 {
					pal[i] = uint32(255)<<24 | uint32(rgb[2])<<16 | uint32(rgb[1])<<8 | uint32(rgb[0])
				}
			}
			chk(U.Close())
			if !readOK {
				continue
			}
			sff.palList.SetSource(target, pal)
			sff.palList.PalTable[[2]uint16{1, uint16(x + 1)}] = target
			sff.palList.numcols[[2]uint16{1, uint16(x + 1)}] = 256
		}
	}
	return nil
}

func preloadSff(filename string, char bool, preloadSpr map[[2]uint16]bool, atlas_8 *TextureAtlas) (*Sff, []int32, error) {
	sff := newSff()
	f, err := OpenFile(filename)
	if err != nil {
		return nil, nil, err
	}
	defer func() { chk(f.Close()) }()
	h := &SffHeader{}
	var lofs, tofs uint32
	if err := h.Read(f, &lofs, &tofs); err != nil {
		return nil, nil, err
	}
	sff.filename = filename
	sff.header.Ver0 = h.Ver0
	sff.header.Ver1 = h.Ver1
	sff.header.Ver2 = h.Ver2
	sff.header.Ver3 = h.Ver3
	sff.header.FirstPaletteHeaderOffset = h.FirstPaletteHeaderOffset
	sff.header.NumberOfPalettes = h.NumberOfPalettes
	read := func(x interface{}) error {
		return binary.Read(f, binary.LittleEndian, x)
	}
	var shofs, xofs, size uint32 = h.FirstSpriteHeaderOffset, 0, 0
	var indexOfPrevious uint16
	var plShofs, plXofs, plSize uint32 = h.FirstPaletteHeaderOffset, 0, 0
	var plIndexOfPrevious uint16
	pl := &PaletteList{}
	pl.init()
	spriteList := make([]*Sprite, int(h.NumberOfSprites))
	var prev *Sprite
	preloadSprNum := len(preloadSpr)
	preloadRef := make(map[int]bool)
	headerXofs := make([]uint32, len(spriteList))
	headerSize := make([]uint32, len(spriteList))
	headerShofs32 := make([]int64, len(spriteList)) // só usado em Ver0 == 1

	//set various variables that let us know if a sprite should use it's own palette
	paletteState := 0
	currentPalette := 0
	var paletteLocation int64
	var initialPalLocation int64
	coloredPalette := -1
	if sff.header.Ver0 != 1 {
		for i := 0; i < int(h.NumberOfPalettes); i++ {
			f.Seek(int64(h.FirstPaletteHeaderOffset)+int64(i*16), 0)
			var gn_ [3]int16
			if err := read(gn_[:]); err != nil {
				return nil, nil, err
			}
			if gn_[0] == 1 && gn_[1] == 1 {
				coloredPalette = i
			}
		}
	}
	// if useAtlas {
	// 	sff.atlas_8 = CreateTextureAtlas(512, 512, 8, false)
	// 	sff.atlas_8.resize = true
	// }
	for i := 0; i < len(spriteList); i++ {
		spriteList[i] = newSprite()
		spriteList[i].Sff = sff
		f.Seek(int64(shofs), 0)
		switch h.Ver0 {
		case 1:
			if err := spriteList[i].readHeader(f, &xofs, &size, &indexOfPrevious); err != nil {
				return nil, nil, err
			}
			//Check if the sprite uses a shared palette or not, in this format every sprite needs to be checked.
			shofs = shofs + 32
			if i > 0 {
				var ps byte
				if err := read(&ps); err != nil {
					return nil, nil, err
				}
				if spriteList[i].Group == 0 && spriteList[i].Number == 0 {
					paletteState = 2
					paletteLocation = initialPalLocation
				} else if ps == 0 && paletteState == 2 {
					paletteState = 3
					currentPalette++
					paletteLocation = int64(xofs - 768)
				} else if ps == 0 {
					if currentPalette == 0 {
						paletteState = 1
					}
					currentPalette++
					paletteLocation = int64(xofs - 768)
				}
			} else {
				paletteLocation = int64(xofs - 768)
				initialPalLocation = paletteLocation
			}
			shofs = shofs - 32
			f.Seek(-1, 1) //Return to where it was.
			headerShofs32[i] = int64(shofs + 32)
		case 2:
			if err := spriteList[i].readHeaderV2(f, &xofs, &size,
				lofs, tofs, &indexOfPrevious); err != nil {
				return nil, nil, err
			}
		}
		headerXofs[i] = xofs
		headerSize[i] = size
		if _, ok := preloadSpr[[...]uint16{spriteList[i].Group, spriteList[i].Number}]; ok || (prev == nil && spriteList[i].palidx < 0) {
			if ok {
				ok = sff.sprites[[...]uint16{spriteList[i].Group, spriteList[i].Number}] == nil
			}
			// sprite
			if size == 0 {
				base := int(indexOfPrevious)
				copyPal := func(srcIdx int) {
					dst, src := spriteList[i], spriteList[srcIdx]
					sys.mainThreadTask <- func() { dst.shareCopy(src) }
					if spriteList[srcIdx].palidx < 0 || int(spriteList[srcIdx].palidx) >= len(pl.paletteMap) {
						spriteList[i].palidx = 0
					} else {
						spriteList[i].palidx = spriteList[srcIdx].palidx
					}
				}
				switch {
				case preloadRef[base]:
					copyPal(base)
				case base < i:
					bxofs, bsize := headerXofs[base], headerSize[base]
					if bsize > 0 {
						var err error
						switch h.Ver0 {
						case 1:
							err = spriteList[base].read(
								f, h, headerShofs32[base], bsize, bxofs, prev,
								pl, char && (prev == nil || spriteList[base].Group == 0 && spriteList[base].Number == 0),
							)
						case 2:
							err = spriteList[base].readV2(f, int64(bxofs), bsize, atlas_8)
						}
						if err != nil {
							return nil, nil, err
						}
						preloadRef[base] = true
						copyPal(base)
					} else {
						spriteList[i].palidx = 0
					}
				default:
					spriteList[i].palidx = 0 // index out of range
				}
			} else {
				switch h.Ver0 {
				case 1:
					if err := spriteList[i].read(f, h, int64(shofs+32), size, xofs, prev,
						pl, char && (prev == nil || spriteList[i].Group == 0 && spriteList[i].Number == 0)); err != nil {
						//pl, false); err != nil {
						return nil, nil, err
					}
				case 2:
					if err := spriteList[i].readV2(f, int64(xofs), size, atlas_8); err != nil {
						return nil, nil, err
					}
				}
				if ok {
					// palette
					plXofs = xofs
					if h.Ver0 == 1 {
						// Try palette from list
						spriteList[i].Pal = pl.Get(spriteList[i].palidx)
						if spriteList[i].palidx >= sys.cfg.Config.PaletteMax || spriteList[i].palidx < 0 {
							spriteList[i].palidx = 0
						}
						if spriteList[i].Pal == nil {
							spriteList[i].palidx = 0
						}
						// Base or shared sprites, force palette read from file (keeps shared ones in sync)
						if spriteList[i].Group == 0 && spriteList[i].Number == 0 || paletteState%2 == 0 {
							if paletteLocation > 0 {
								if _, err := f.Seek(paletteLocation, 0); err == nil {
									pal := make([]uint32, 256)
									var rgb [3]byte
									ok := true
									for c := range pal {
										if err := read(rgb[:]); err != nil {
											ok = false
											break
										}
										var alpha byte = 255
										if c == 0 {
											alpha = 0
										}
										pal[c] = uint32(alpha)<<24 | uint32(rgb[2])<<16 | uint32(rgb[1])<<8 | uint32(rgb[0])
									}
									if ok {
										spriteList[i].Pal = pal
										spriteList[i].palidx = 0 // shared
									}
								}
							}
							f.Seek(int64(xofs), 0) // reset pointer to sprite data
						} else {
							// Unique palette, keep as is
							spriteList[i].palidx = 1
						}
					} else if spriteList[i].coldepth <= 8 {
						plSize = 0
						plIndexOfPrevious = uint16(spriteList[i].palidx)
						ip := plIndexOfPrevious + 1
						for plSize == 0 && ip != plIndexOfPrevious {
							ip = plIndexOfPrevious
							plShofs = h.FirstPaletteHeaderOffset + uint32(ip)*16
							f.Seek(int64(plShofs)+6, 0)
							if err := read(&plIndexOfPrevious); err != nil {
								return nil, nil, err
							}
							if err := read(&plXofs); err != nil {
								return nil, nil, err
							}
							if err := read(&plSize); err != nil {
								return nil, nil, err
							}
						}
						f.Seek(int64(lofs+plXofs), 0)
						spriteList[i].Pal = make([]uint32, 256)
						var rgba [4]byte
						for j := 0; j < int(plSize)/4 && j < len(spriteList[i].Pal); j++ {
							if err := read(rgba[:]); err != nil {
								return nil, nil, err
							}
							if h.Ver2 == 0 {
								if j == 0 {
									rgba[3] = 0
								} else {
									rgba[3] = 255
								}
							}
							spriteList[i].Pal[j] = uint32(rgba[3])<<24 | uint32(rgba[2])<<16 | uint32(rgba[1])<<8 | uint32(rgba[0])
						}

						//Uses palidx as if it were a boolean, used later to know if sprite's palette should be removed in favor of character's selected palette
						if spriteList[i].palidx != coloredPalette {
							spriteList[i].palidx = 1
						} else {
							spriteList[i].palidx = 0
						}

					}
				}
				if prev == nil {
					prev = spriteList[i]
				}
			}
			preloadRef[i] = true
			if ok {
				sff.sprites[[...]uint16{spriteList[i].Group, spriteList[i].Number}] = spriteList[i]
				preloadSprNum--
				if preloadSprNum == 0 {
					break
				}
			}
		}
		if h.Ver0 == 1 {
			shofs = xofs
		} else {
			shofs += 28
		}
	}
	// selectable palettes
	var preSelPal []int32
	var selPal []int32
	if h.Ver0 != 1 && char {
		for i := 0; i < int(h.NumberOfPalettes); i++ {
			f.Seek(int64(h.FirstPaletteHeaderOffset)+int64(i*16), 0)
			var gn_ [3]int16
			if err := read(gn_[:]); err != nil {
				return nil, nil, err
			}
			if gn_[0] == 1 && gn_[1] >= 1 && gn_[1] <= int16(sys.cfg.Config.PaletteMax) {
				preSelPal = append(preSelPal, int32(gn_[1]))
			}
			if len(preSelPal) >= sys.cfg.Config.PaletteMax {
				break
			}
		}
		for i := 0; i <= sys.cfg.Config.PaletteMax; i++ {
			for k := 0; k < len(preSelPal); k++ {
				if preSelPal[k] == int32(i) {
					selPal = append(selPal, int32(i))
				}
			}
			if len(preSelPal) == len(selPal) {
				break
			}
		}
	}
	return sff, selPal, nil
}

func (s *Sff) GetSprite(g, n uint16) *Sprite {
	if g == 0xFFFF {
		return nil
	}
	return s.sprites[[2]uint16{g, n}]
}

func (s *Sff) getOwnPalSprite(g, n uint16, pl *PaletteList) *Sprite {
	sys.runMainThreadTask() // Generate texture
	sp := s.GetSprite(g, n)
	if sp == nil {
		return nil
	}
	osp, pal := *sp, sp.GetPal(pl)
	osp.Pal = make([]uint32, len(pal))
	copy(osp.Pal, pal)
	return &osp
}

func captureScreen() {
	width, height := sys.window.GetSize()
	pixdata := make([]uint8, 4*width*height)
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	gfx.ReadPixels(pixdata, width, height)
	for i := 0; i < 4*width*height; i++ {
		j := i
		if gfx.GetName() != "Vulkan 1.3.239" {
			var x, y int
			x = i % (width * 4)
			y = i / (width * 4)
			j = x + (height-1-y)*width*4
		}
		if i%4 == 3 {
			pixdata[i] = 255 // Set the alpha value to 255
		}
		img.Pix[j] = pixdata[i]
	}

	// Sanitize WindowTitle for use in filenames
	re := regexp.MustCompile(`[<>:"/\\|?*]`)
	safeTitle := re.ReplaceAllString(sys.cfg.Config.WindowTitle, "_")
	safeTitle = strings.TrimSpace(safeTitle)
	safeTitle = strings.ReplaceAll(safeTitle, " ", "_")
	if safeTitle == "" {
		safeTitle = "ikemen"
	}

	for i := sys.captureNum; i < 999; i++ {
		filename := fmt.Sprintf("%s%s%03d.png", sys.cfg.Config.ScreenshotFolder, safeTitle, i)
		if _, err := os.Stat(filename); os.IsNotExist(err) {
			file, _ := os.Create(filename)
			defer file.Close()
			png.Encode(file, img)
			sys.captureNum = i
			break
		}
	}
}
