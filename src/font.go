package main

import (
	"encoding/binary"
	"math"
	"os"
	"regexp"
	"strings"
	"fmt"
)

type FontRenderer interface {
	Init()
	LoadFont(file string, scale int32, windowWidth int, windowHeight int) (interface{}, error)
	//LoadTrueTypeFont(program uint32, r io.Reader, scale int32, low, high rune, dir Direction) (Font, error)
	//newProgram(GLSLVersion uint, vertexShaderSource, fragmentShaderSource string) (uint32, error)
}

type Font interface {
	SetColor(red float32, green float32, blue float32, alpha float32)
	UpdateResolution(windowWidth int, windowHeight int)
	Printf(x, y float32, scale float32, align int32, blend bool, window [4]int32, fs string, argv ...interface{}) error
	//renderGlyphBatch(batchChars []*character, indices []rune, vertices []float32)
	Width(scale float32, fs string, argv ...interface{}) float32
}

type character struct {
	textureID uint32 // ID handle of the glyph texture
	uv        [4]float32
	width     int //glyph width
	height    int //glyph height
	advance   int //glyph advance
	bearingH  int //glyph bearing horizontal
	bearingV  int //glyph bearing vertical
}

type color struct {
	r float32
	g float32
	b float32
	a float32
}

// Direction represents the direction in which strings should be rendered.
type Direction uint8

// Known directions.
const (
	LeftToRight Direction = iota // E.g.: Latin
	RightToLeft                  // E.g.: Arabic
	TopToBottom                  // E.g.: Chinese
)

// FntCharImage stores sprite and position
type FntCharImage struct {
	ofs, w uint16
	img    []Sprite
}

// TtfFont implements TTF font rendering on supported platforms
type TtfFont interface {
	SetColor(red float32, green float32, blue float32, alpha float32)
	Width(scale float32, fs string, argv ...interface{}) float32
	Printf(x, y float32, scale float32, align int32, blend bool, window [4]int32, fs string, argv ...interface{}) error
	UpdateResolution(windowWidth int, windowHeight int)
}

// Fnt is a interface for basic font information
type Fnt struct {
	images    map[int32]map[rune]*FntCharImage
	palettes  [][256]uint32
	coldepth  []byte
	ver, ver2 uint16
	Type      string
	BankType  string
	Size      [2]uint16
	Spacing   [2]int32
	colors    int32
	offset    [2]int32
	ttf       TtfFont
	paltex    Texture
	atlas_8   *TextureAtlas

	// Batch Rendering Fields
    batchVertices []float32 // Persistent buffer to avoid allocation
    batchTex      Texture   // Current texture for the active batch
    batchPal      Texture   // Current palette for the active batch
    isBatching    bool      // Are we currently in a manual batch block?
}

func newFnt() *Fnt {
	return &Fnt{
		images:   make(map[int32]map[rune]*FntCharImage),
		BankType: "palette",
	}
}

func loadFnt(filename string, height int32) (*Fnt, error) {
	fmt.Printf("loadFnt %v\n", filename)
	if HasExtension(filename, ".fnt") {
		return loadFntV1(filename)
	}

	return loadFntV2(filename, height)
}

func loadFntV1(filename string) (*Fnt, error) {
	fmt.Printf("loadFntV1 %v\n", filename)
	f := newFnt()
	f.images[0] = make(map[rune]*FntCharImage)

	fp, err := os.Open(filename)

	if err != nil {
		return nil, Error("File not found")
	}

	defer func() { chk(fp.Close()) }()

	// Read header
	buf := make([]byte, 12)
	n, err := fp.Read(buf)

	// Error reading file
	if err != nil {
		return nil, err
	}

	// Error is not a valid fnt file
	if string(buf[:n]) != "ElecbyteFnt\x00" {
		return nil, Error("Unrecognized FNT file: " + string(buf[:n]))
	}

	read := func(x interface{}) error {
		return binary.Read(fp, binary.LittleEndian, x)
	}

	if err := read(&f.ver); err != nil {
		return nil, err
	}

	if err := read(&f.ver2); err != nil {
		return nil, err
	}

	var pcxDataOffset, pcxDataLength, txtDataOffset, txtDataLength uint32
	if err := read(&pcxDataOffset); err != nil {
		return nil, err
	}

	if err := read(&pcxDataLength); err != nil {
		return nil, err
	}

	if err := read(&txtDataOffset); err != nil {
		return nil, err
	}

	if err := read(&txtDataLength); err != nil {
		return nil, err
	}

	spr := newSprite()
	if err := spr.readPcxHeader(fp, int64(pcxDataOffset)); err != nil {
		return nil, err
	}

	fp.Seek(int64(pcxDataOffset)+128, 0)
	px := make([]byte, pcxDataLength-128-768)
	if err := read(px); err != nil {
		return nil, err
	}

	spr.Pal = make([]uint32, 256)
	var rgb [3]byte
	for i := range spr.Pal {
		if err := read(rgb[:]); err != nil {
			return nil, err
		}
		var alpha byte = 255
		if i == 0 {
			alpha = 0
		}
		spr.Pal[i] = uint32(alpha)<<24 | uint32(rgb[2])<<16 | uint32(rgb[1])<<8 | uint32(rgb[0])
	}

	px = spr.RlePcxDecode(px)
	fmt.Printf("loadFntV1: PCX %vx%v loaded\n", spr.Size[0], spr.Size[1])

	// Create Texture Atlas and Upload Texture to GPU
	sys.mainThreadTask <- func() {
		f.atlas_8 = CreateTextureAtlas(int32(spr.Size[0]), int32(spr.Size[1]), 8, false)
		f.atlas_8.texture.SetData(px)
		spr.Tex = f.atlas_8.texture
	}

	fp.Seek(int64(txtDataOffset), 0)
	buf = make([]byte, txtDataLength)
	if err := read(buf); err != nil {
		return nil, err
	}
	fmt.Printf("Data:\n%v\n", string(buf))
	lines := SplitAndTrim(string(buf), "\n")
	i := 0
	mapflg, defflg := true, true
	for {
		var name string
		for ; i < len(lines); i++ {
			name, _ = SectionName(lines[i])
			if len(name) > 0 {
				i++
				break
			}
		}
		if len(name) == 0 {
			break
		}
		switch name {
		case "map":
			if mapflg {
				mapflg = false
				re := regexp.MustCompile(`(\S+)(?:\s+(\S+)(?:\s+(\S+))?)?`)
				ofs := uint16(0)
				w := int32(0)
				for ; i < len(lines); i++ {
					if len(lines[i]) > 0 && lines[i][0] == '[' {
						break
					}
					cap := re.FindStringSubmatch(strings.SplitN(lines[i], ";", 2)[0])
					if len(cap) > 0 {
						var c rune
						if len(cap[1]) >= 2 && cap[1][0] == '0' &&
							(cap[1][1] == 'X' || cap[1][1] == 'x') {
							hex := strings.ToLower(cap[1][2:])
							for _, r := range hex {
								if '0' <= r && r <= '9' {
									c = c<<4 | (r - '0')
								} else if 'a' <= r && r <= 'f' {
									c = c<<4 | (r - 'a' + 10)
								} else {
									break
								}
							}
						} else {
							c = rune(cap[1][0])
						}
						if len(cap[2]) > 0 {
							ofs = I32ToU16(Atoi(cap[2]))
						}
						fci := &FntCharImage{ofs: ofs}
						f.images[0][c] = fci
						if len(cap[3]) > 0 {
							w = Atoi(cap[3])
							if w < 0 {
								ofs += I32ToU16(int32(ofs) - w)
								w = 0 - w
							}
							fci.w = I32ToU16(w)
							ofs += fci.w - f.Size[0]
						} else {
							fci.w = f.Size[0]
						}
					}
					ofs += f.Size[0]
				}
			}
		case "def":
			if defflg {
				defflg = false
				is := NewIniSection()
				is.Parse(lines, &i)
				loadDefInfo(f, filename, is, 0)
			}
		}
	}
	c := Min(255, int32(math.Ceil(float64(f.colors)/16))*16)
	f.palettes = make([][256]uint32, 255/c)
	for i := int32(0); int(i) < len(f.palettes); i++ {
		copy(f.palettes[i][:256-c], spr.Pal[:256-c])
		copy(f.palettes[i][256-c:], spr.Pal[256-c*(i+1):256-c*i])
	}

	for cc, fci := range f.images[0] {
		// 1. ALLOCATE MEMORY (Synchronous)
		fci.img = make([]Sprite, len(f.palettes))

		// 2. CAPTURE VARIABLES (Critical for closure)
		// Without this, the closure below will always use the 'fci' from the 
		// LAST iteration of the loop for every single task!
		fci := fci 
		cc := cc

		// 3. SCHEDULE TEXTURE ASSIGNMENT (Asynchronous / Main Thread)
		sys.mainThreadTask <- func() {
			// Setup Bank 0
			fci.img[0].shareCopy(spr)
			fci.img[0].Size[0] = fci.w
			fci.img[0].UV = [4]float32{float32(fci.ofs) / float32(spr.Size[0]), 0.0, float32(fci.ofs + fci.w) / float32(spr.Size[0]), 1.0}
			
			// Use the atlas texture which was created in the previous mainThreadTask
			fci.img[0].Tex = f.atlas_8.texture
			fmt.Printf("cc=%v bank=0 Tex=%v\n", cc, fci.img[0].Tex)

			// Setup other banks (dependent on bank 0)
			for i := 1; i < len(f.palettes); i++ {
				fci.img[i].shareCopy(&fci.img[0])
				fci.img[i].Size[0] = fci.w
				fci.img[i].UV = fci.img[0].UV
				fci.img[i].Tex = fci.img[0].Tex // Copy the valid texture
				fmt.Printf("cc=%v bank=%v Tex=%v\n", cc, i, fci.img[i].Tex)
			}
		}

		// 4. PALETTE ASSIGNMENT (Synchronous)
		for i, p := range f.palettes {
			fci.img[i].Offset[0], fci.img[i].Offset[1], fci.img[i].Pal = 0, 0, p[:]
		}
	}
	
	sys.mainThreadTask <- func() {
		if f.atlas_8 != nil {
			if err := f.atlas_8.texture.SavePNG(filename+"_atlas_8.png", f.palettes[0][:255]); err != nil {
				fmt.Printf("loadFntv1: SavePNG returned error: %v\n", err)
			} else {
				fmt.Printf("loadFntv1: %v saved\n", filename+"_atlas_8.png")
			}
		} else {
			fmt.Printf("Atlas is empty for %v\n", filename)
		}
	}
	return f, nil
}

func loadFntV2(filename string, height int32) (*Fnt, error) {
	f := newFnt()

	content, err := LoadText(filename)

	if err != nil {
		return nil, Error("File not found")
	}

	lines := SplitAndTrim(string(content), "\n")
	i := 0
	var name string

	for ; i < len(lines); i++ {
		name, _ = SectionName(lines[i])
		if len(name) > 0 {
			is := NewIniSection()
			i++
			is.Parse(lines, &i)
			i--
			switch name {
			case "def":
				loadDefInfo(f, filename, is, height)
			}
		}
	}
	return f, nil
}

func loadDefInfo(f *Fnt, filename string, is IniSection, height int32) {
	f.Type = strings.ToLower(is["type"])
	if _, ok := is["banktype"]; ok {
		f.BankType = strings.ToLower(is["banktype"])
	}
	ary := SplitAndTrim(is["size"], ",")
	if len(ary[0]) > 0 {
		f.Size[0] = I32ToU16(Atoi(ary[0]))
	}
	if len(ary) > 1 && len(ary[1]) > 0 {
		f.Size[1] = I32ToU16(Atoi(ary[1]))
	}
	ary = SplitAndTrim(is["spacing"], ",")
	if len(ary[0]) > 0 {
		f.Spacing[0] = Atoi(ary[0])
	}
	if len(ary) > 1 && len(ary[1]) > 0 {
		f.Spacing[1] = Atoi(ary[1])
	}
	f.colors = Clamp(Atoi(is["colors"]), 1, 255)
	ary = SplitAndTrim(is["offset"], ",")
	if len(ary[0]) > 0 {
		f.offset[0] = Atoi(ary[0])
	}
	if len(ary) > 1 && len(ary[1]) > 0 {
		f.offset[1] = Atoi(ary[1])
	}

	if len(is["file"]) > 0 {
		if f.Type == "truetype" {
			LoadFntTtf(f, filename, is["file"], height)
		} else {
			LoadFntSff(f, filename, is["file"])
		}
	}
}

func LoadFntSff(f *Fnt, fontfile string, filename string) {
	fileDir := SearchFile(filename, []string{fontfile, "font/", sys.motifDir, "", "data/"})
	sff, err := loadSff(fileDir, false, f.atlas_8)

	if err != nil {
		panic(err)
	}

	// FIX: Perform sprite extraction in the main thread task.
	// This ensures we wait for any async texture operations (SetPxl) 
	// initiated by loadSff to complete before we copy the sprite data.
	sys.mainThreadTask <- func() {
		// Load sprites
		var pal_default []uint32
		for k, sprite := range sff.sprites {
			s := sff.getOwnPalSprite(sprite.Group, sprite.Number, &sff.palList)
			if sprite.Group == 0 || f.BankType == "sprite" {
				if f.images[int32(sprite.Group)] == nil {
					f.images[int32(sprite.Group)] = make(map[rune]*FntCharImage)
				}
				if pal_default == nil && sff.header.Ver0 == 1 {
					pal_default = s.Pal
				}
				offsetX := uint16(s.Offset[0])
				sizeX := uint16(s.Size[0])

				fci := &FntCharImage{
					ofs: offsetX,
					w:   sizeX,
				}
				fci.img = make([]Sprite, 1)
				fci.img[0] = *s
				f.images[int32(sprite.Group)][rune(k[1])] = fci
			}
		}

		// Load palettes
		f.palettes = make([][256]uint32, sff.header.NumberOfPalettes)
		f.coldepth = make([]byte, sff.header.NumberOfPalettes)
		var idef int
		for i := 0; i < int(sff.header.NumberOfPalettes); i++ {
			var pal []uint32
			si, ok := sff.palList.PalTable[[...]uint16{0, uint16(i)}]
			if ok && si >= 0 {
				pal = sff.palList.Get(si)
				if i == 0 {
					idef = si
				}
				switch sff.palList.numcols[[...]uint16{0, uint16(i)}] {
				case 256:
					f.coldepth[i] = 8
				case 32:
					f.coldepth[i] = 5
				}
			} else {
				pal = sff.palList.Get(idef)
			}
			copy(f.palettes[i][:], pal)
		}
		if len(f.palettes) == 0 && pal_default != nil {
			f.palettes = make([][256]uint32, 1)
			copy(f.palettes[0][:], pal_default)
		}
	}
}

// BeginBatch resets the buffer and prepares for accumulating vertices
func (f *Fnt) BeginBatch() {
	f.isBatching = true
	f.batchVertices = f.batchVertices[:0]
	f.batchTex = nil
	f.batchPal = nil
}

// EndBatch flushes whatever is remaining in the buffer to the GPU
func (f *Fnt) EndBatch(rp RenderParams) {
	if len(f.batchVertices) > 0 {
		rp.tex = f.batchTex
		rp.paltex = f.batchPal
        // Note: rp.rot is for the whole string, but batchVertices handles per-char UV rotation
		RenderSpriteBatch(f.batchVertices, rp)
	}
	f.isBatching = false
}

// FlushBatch forces a render of the current buffer (used when texture changes)
func (f *Fnt) FlushBatch(rp RenderParams) {
    if len(f.batchVertices) > 0 {
        rp.tex = f.batchTex
        rp.paltex = f.batchPal
        RenderSpriteBatch(f.batchVertices, rp)
        f.batchVertices = f.batchVertices[:0]
    }
}

// CharWidth returns the width that has a specified character
func (f *Fnt) CharWidth(c rune, bt int32) int32 {
	if c == ' ' {
		return int32(f.Size[0])
	}
	fci := f.images[bt][c]
	if fci == nil {
		return 0
	}
	return int32(fci.w)
}

// TextWidth returns the width that has a specified text.
// This depends on each char's width and font spacing
func (f *Fnt) TextWidth(txt string, bank int32) (w int32) {
	if f.BankType != "sprite" {
		bank = 0
	}
	for i, c := range txt {
		if f.Type == "truetype" {
			w += int32(f.ttf.Width(1, string(c)))
		} else {
			cw := f.CharWidth(c, bank)
			// in mugen negative spacing matching char width seems to skip calc,
			// even for 1 symbol string (which normally shouldn't use spacing)
			if cw+f.Spacing[0] > 0 {
				w += cw
				if i < len(txt)-1 {
					w += f.Spacing[0]
				}
			}
		}
	}
	return
}

func (f *Fnt) getCharSpr(c rune, bank, bt int32) *Sprite {
	fci := f.images[bt][c]
	if fci == nil {
		return nil
	}

	if bank < int32(len(fci.img)) {
		return &fci.img[bank]
	}

	return &fci.img[0]
}

func (f *Fnt) drawChar(
	x, y,
	xscl, yscl float32,
	bank, bt int32,
	c rune, pal []uint32,
	rp RenderParams,
) float32 {
	if c == ' ' {
		return float32(f.Size[0]) * xscl
	}

	spr := f.getCharSpr(c, bank, bt)
	if spr == nil {
		fmt.Printf("drawChar: spr == nil for char=%c[%v] bank=%v bt=%v\n", c, c, bank, bt)
		return 0
	}
	if spr.Tex == nil {
		fmt.Printf("drawChar: spr.Tex == nil for char=%c[%v] bank=%v bt=%v\n", c, c, bank, bt)
		return 0
	}

	// In case of mismatched color depth between bank palette and the sprite's own palette,
	// Mugen 1.1 uses the latter, ignoring the bank
	if len(f.palettes) != 0 && len(f.coldepth) > int(bank) &&
		f.images[bt][c].img[0].coldepth != 32 &&
		f.coldepth[bank] != f.images[bt][c].img[0].coldepth {
		pal = f.images[bt][c].img[0].Pal[:] //palfx.getFxPal(f.images[bt][c].img[0].Pal[:], false)
	}

	x -= xscl * float32(spr.Offset[0])
	y -= yscl * float32(spr.Offset[1])
	if spr.coldepth <= 8 && f.paltex == nil {
		f.paltex = spr.CachePalette(pal)
	}

	// Update only the render parameters that change between each character
	rp.tex = spr.Tex
	rp.paltex = f.paltex
	rp.size = spr.Size
	rp.x = -x * sys.widthScale
	rp.y = -y * sys.heightScale
	rp.uv = spr.UV

	RenderSprite(rp)
	return float32(spr.Size[0]) * xscl
}

func (f *Fnt) Print(txt string, x, y, xscl, yscl, rxadd float32, rot Rotation, bank, align int32,
	window *[4]int32, palfx *PalFX, frgba [4]float32) {
	if !sys.frameSkip {
		if f.Type == "truetype" {
			f.DrawTtf(txt, x, y, xscl, yscl, align, true, window, frgba)
		} else {
			// DRAWCALL 20
			f.DrawTextBatch(txt, x, y, xscl, yscl, rxadd, rot, bank, align, window, palfx, frgba[3])
			// f.DrawText(txt, x, y, xscl, yscl, rxadd, rot, bank, align, window, palfx, frgba[3])
		}
	}
}

// DrawText prints on screen a specified text with the current font sprites
func (f *Fnt) DrawText(txt string, x, y, xscl, yscl, rxadd float32, rot Rotation, bank, align int32, window *[4]int32, palfx *PalFX, alpha float32) {

	if len(txt) == 0 || xscl == 0 || yscl == 0 {
		return
	}

	var bt int32
	if f.BankType == "sprite" {
		bt = bank
		bank = 0
	} else if bank < 0 || len(f.palettes) <= int(bank) {
		bank = 0
	}

	// not existing characters treated as space
	for i, c := range txt {
		if c != ' ' && f.images[bt][c] == nil {
			//txt = strings.Replace(txt, string(c), " ", -1)
			txt = txt[:i] + string(' ') + txt[i+1:]
		}
	}

	x += float32(f.offset[0])*xscl + float32(sys.gameWidth-320)/2
	y += float32(f.offset[1]-int32(f.Size[1])+1)*yscl + float32(sys.gameHeight-240)

	var rcx, rcy float32

	if rot.IsZero() {
		if xscl < 0 {
			x *= -1
		}
		if yscl < 0 {
			y *= -1
		}
		rcx, rcy = rcx*sys.widthScale, 0
	} else {
		rcx, rcy = (x+rcx)*sys.widthScale, y*sys.heightScale
		x, y = AbsF(xscl)*float32(f.offset[0]), AbsF(yscl)*float32(f.offset[1])
	}

	if align == 0 {
		x -= float32(f.TextWidth(txt, bank)) * xscl * 0.5
	} else if align < 0 {
		x -= float32(f.TextWidth(txt, bank)) * xscl
	}

	var pal []uint32
	if len(f.palettes) != 0 {
		pal = f.palettes[bank][:] //palfx.getFxPal(f.palettes[bank][:], false)
	}

	f.paltex = nil

	// Set the trans type
	tt := TT_none
	if alpha < 1.0 {
		tt = TT_add
	}

	alphaVal := int32(255 * sys.brightness * alpha)

	// Initialize common render parameters
	rp := RenderParams{
		tex:            nil,
		paltex:         nil,
		size:           [2]uint16{0, 0},
		x:              0,
		y:              0,
		tile:           notiling,
		xts:            xscl * sys.widthScale,
		xbs:            xscl * sys.widthScale,
		ys:             yscl * sys.heightScale,
		vs:             1,
		rxadd:          rxadd,
		xas:            1,
		yas:            1,
		rot:            rot,
		tint:           0,
		blendMode:      tt,
		blendAlpha:     [2]int32{alphaVal, 255 - alphaVal},
		mask:           0,
		pfx:            palfx,
		window:         window,
		rcx:            rcx,
		rcy:            rcy,
		projectionMode: 0,
		fLength:        0,
		xOffset:        0,
		yOffset:        0,
	}

	for _, c := range txt {
		x += f.drawChar(x, y, xscl, yscl, bank, bt, c, pal, rp) + xscl*float32(f.Spacing[0])
	}
}

func (f *Fnt) DrawTextBatch(txt string, x, y, xscl, yscl, rxadd float32, rot Rotation, bank, align int32, window *[4]int32, palfx *PalFX, alpha float32) {
	if len(txt) == 0 || xscl == 0 || yscl == 0 {
		return
	}

	var bt int32
	if f.BankType == "sprite" {
		bt = bank
		bank = 0
	} else if bank < 0 || len(f.palettes) <= int(bank) {
		bank = 0
	}

	// 1. Calculate Coordinates & Rotation
	x += float32(f.offset[0])*xscl + float32(sys.gameWidth-320)/2
	y += float32(f.offset[1]-int32(f.Size[1])+1)*yscl + float32(sys.gameHeight-240)

	var rcx, rcy float32
	if rot.IsZero() {
		if xscl < 0 {
			x *= -1
		}
		if yscl < 0 {
			y *= -1
		}
		rcx, rcy = rcx*sys.widthScale, 0
	} else {
		rcx, rcy = (x+rcx)*sys.widthScale, y*sys.heightScale
		x, y = AbsF(xscl)*float32(f.offset[0]), AbsF(yscl)*float32(f.offset[1])
	}

	// Handle Alignment
	if align == 0 {
		x -= float32(f.TextWidth(txt, bank)) * xscl * 0.5
	} else if align < 0 {
		x -= float32(f.TextWidth(txt, bank)) * xscl
	}

	// 2. Initialize Render Parameters
	tt := TT_none
	if alpha < 1.0 {
		tt = TT_add
	}
	alphaVal := int32(255 * sys.brightness * alpha)

	rp := RenderParams{
		tex:            nil,
		paltex:         nil,
		size:           [2]uint16{0, 0},
		x:              0,
		y:              0,
		tile:           notiling,
		xts:            xscl * sys.widthScale,
		xbs:            xscl * sys.widthScale,
		ys:             yscl * sys.heightScale,
		vs:             1,
		rxadd:          rxadd,
		xas:            1,
		yas:            1,
		rot:            rot,
		tint:           0,
		blendMode:      tt,
		blendAlpha:     [2]int32{alphaVal, 255 - alphaVal},
		mask:           0,
		pfx:            palfx,
		window:         window,
		rcx:            rcx,
		rcy:            rcy,
		projectionMode: 0,
		fLength:        0,
		xOffset:        0,
		yOffset:        0,
	}

	// 3. Buffer Management
	localBatch := !f.isBatching
	if localBatch {
		f.BeginBatch()
	}

	var pal []uint32
	if len(f.palettes) != 0 {
		pal = f.palettes[bank][:]
	}

	currentX := x

	// 4. Vertex Generation Loop
	for _, c := range txt {
		if c == ' ' {
			currentX += xscl*float32(f.Size[0]) + xscl*float32(f.Spacing[0])
			continue
		}

		spr := f.getCharSpr(c, bank, bt)
		if spr == nil {
			currentX += xscl*float32(f.Size[0]) + xscl*float32(f.Spacing[0])
			continue
		}
		if spr.Tex == nil {
			continue
		}

		// Palette Logic
		currentPalSlice := pal
		if len(f.palettes) != 0 && len(f.coldepth) > int(bank) &&
			f.images[bt][c].img[0].coldepth != 32 &&
			f.coldepth[bank] != f.images[bt][c].img[0].coldepth {
			currentPalSlice = f.images[bt][c].img[0].Pal[:]
		}

		// Palette Texture Logic
		var currentPalTex Texture
		if spr.coldepth <= 8 {
			if f.paltex == nil {
				f.paltex = spr.CachePalette(currentPalSlice)
			}
			currentPalTex = f.paltex
		}

		// Batch Flushing Logic
		// If the texture or palette changes, we MUST draw what we have so far
		if f.batchTex != nil && (spr.Tex != f.batchTex || currentPalTex != f.batchPal) {
			f.FlushBatch(rp)
		}

		f.batchTex = spr.Tex
		f.batchPal = currentPalTex

		// Geometry Calculation
		charX := currentX - xscl*float32(spr.Offset[0])
		charY := y - yscl*float32(spr.Offset[1])

		width := float32(spr.Size[0]) * xscl * sys.widthScale
		height := float32(spr.Size[1]) * yscl * sys.heightScale
		screenX := charX * sys.widthScale
		screenY := charY * sys.heightScale

		glTopY := float32(sys.scrrect[3]) - screenY
		glBottomY := glTopY - height

		u1, v1, u2, v2 := spr.UV[0], spr.UV[1], spr.UV[2], spr.UV[3]

		// FIX: Handle individual textures (V2/SFF without atlas) that have default zero UVs.
		// If all UVs are 0, assume the sprite uses the full texture.
		if u1 == 0 && v1 == 0 && u2 == 0 && v2 == 0 {
			u2, v2 = 1.0, 1.0
		}

		// Append to Persistent Buffer
		f.batchVertices = append(f.batchVertices,
			screenX, glBottomY, u1, v2,
			screenX+width, glBottomY, u2, v2,
			screenX, glTopY, u1, v1,
			screenX, glTopY, u1, v1,
			screenX+width, glBottomY, u2, v2,
			screenX+width, glTopY, u2, v1,
		)

		currentX += float32(spr.Size[0])*xscl + xscl*float32(f.Spacing[0])
	}

	// 5. Auto-End if local
	if localBatch {
		f.EndBatch(rp)
	}
}

func (f *Fnt) DrawTtf(txt string, x, y, xscl, yscl float32, align int32,
	blend bool, window *[4]int32, frgba [4]float32) {

	if len(txt) == 0 {
		return
	}

	if f.ttf != nil {
		f.ttf.UpdateResolution(int(sys.gameWidth), int(sys.gameHeight))
	}

	x += float32(f.offset[0])*xscl + float32(sys.gameWidth-320)/2
	//y += float32(f.offset[1]-int32(f.Size[1])+1)*yscl + float32(sys.gameHeight-240)

	win := [4]int32{(*window)[0], sys.scrrect[3] - ((*window)[1] + (*window)[3]),
		(*window)[2], (*window)[3]}

	f.ttf.SetColor(frgba[0], frgba[1], frgba[2], frgba[3])
	f.ttf.Printf(x, y, (xscl+yscl)/2, align, blend, win, "%s", txt) //x, y, scale, align, blend, window, string, printf args
}

type TextSprite struct {
	ownerid          int32
	id               int32
	text             string
	template         string
	params           []interface{}
	fnt              *Fnt
	bank, align      int32
	x, y, xscl, yscl float32
	xshear           float32
	angle            float32
	localScale       float32 // text sctrl
	offsetX          int32   // text sctrl
	lineSpacing      float32
	layerno          int16 // text sctrl
	palfx            *PalFX
	frgba            [4]float32 // ttf fonts
	forcecolor       bool
	removetime       int32 // text sctrl
	elapsedTicks     float32
	textDelay        float32
	velocity         [2]float32
	friction         [2]float32
	accel            [2]float32
	window           [4]int32
}

func NewTextSprite() *TextSprite {
	ts := &TextSprite{
		id:          -1,
		align:       1,
		x:           sys.luaSpriteOffsetX,
		xscl:        1,
		yscl:        1,
		window:      sys.scrrect,
		palfx:       newPalFX(),
		frgba:       [...]float32{1.0, 1.0, 1.0, 1.0},
		removetime:  1,
		layerno:     1,
		localScale:  1,
		offsetX:     0,
		lineSpacing: 10,
		textDelay:   0,
		velocity:    [2]float32{0.0, 0.0},
		friction:    [2]float32{1.0, 1.0},
		accel:       [2]float32{0.0, 0.0},
		xshear:      0,
		angle:       0,
	}
	ts.palfx.setColor(255, 255, 255)
	return ts
}

func (ts *TextSprite) SetLocalcoord(lx, ly float32) {
	v := lx
	if lx*3 > ly*4 {
		v = ly * 4 / 3
	}
	ts.localScale = float32(v / 320)
	ts.offsetX = -int32(math.Floor(float64(lx)/(float64(v)/320)-320) / 2)
}

func (ts *TextSprite) SetWindow(x, y, w, h float32) {
	ts.window[0] = int32((x + float32(sys.gameWidth-320)/2) * sys.widthScale)
	ts.window[1] = int32((y + float32(sys.gameHeight-240)) * sys.heightScale)
	ts.window[2] = int32(w*sys.widthScale + 0.5)
	ts.window[3] = int32(h*sys.heightScale + 0.5)
}

func (ts *TextSprite) SetColor(r, g, b, a int32) {
	ts.forcecolor = true
	ts.palfx.setColor(r, g, b)
	ts.frgba = [...]float32{float32(r) / 255, float32(g) / 255,
		float32(b) / 255, float32(a) / 255}
}

func (ts *TextSprite) SetTextVel() {
	ts.x += ts.velocity[0]
	ts.y += ts.velocity[1]
	for i := range ts.velocity {
		ts.velocity[i] *= ts.friction[i]
		ts.velocity[i] += ts.accel[i]
		if math.Abs(float64(ts.velocity[i])) < 0.1 && math.Abs(float64(ts.friction[i])) < 1 {
			ts.velocity[i] = 0
		}
	}
}

func (ts *TextSprite) Draw() {
	if sys.frameSkip || ts.fnt == nil || len(ts.text) == 0 {
		return
	}

	// Replace each tab with 4 spaces
	// We do this first so that length checks are accurate
	text := strings.ReplaceAll(ts.text, "\t", "    ")

	maxChars := int32(len(text))

	// If textDelay is greater than 0, it controls the maximum number of characters
	if ts.textDelay > 0 {
		// Offset the delay so that we show the first character immediately
		elapsed := ts.elapsedTicks + ts.textDelay
		maxChars = int32(elapsed / ts.textDelay)
	}

	maxChars = Clamp(maxChars, 0, int32(len(text)))

	// Control of total displayed characters
	totalCharsShown := 0

	lines := strings.Split(text, "\n")

	for i, line := range lines {
		lineLength := len(line)

		// Shows the characters progressively
		charsToShow := int(Min(int32(lineLength), maxChars-int32(totalCharsShown)))
		if charsToShow <= 0 {
			charsToShow = i
			continue
		}

		newY := ts.y + float32(i)*ts.yscl*ts.lineSpacing

		// Xshear offset correction
		xshear := -ts.xshear
		xsoffset := xshear * (float32(ts.fnt.offset[1]) * ts.yscl)

		// Draw the visible line
		if ts.fnt.Type == "truetype" {
			ts.fnt.DrawTtf(line[:charsToShow], ts.x, newY, ts.xscl, ts.yscl, ts.align, true, &ts.window, ts.frgba)
		} else {
			ts.fnt.DrawTextBatch(line[:charsToShow], ts.x-xsoffset, newY, ts.xscl, ts.yscl,
				xshear, Rotation{ts.angle, 0, 0}, ts.bank, ts.align, &ts.window, ts.palfx, ts.frgba[3])
			// ts.fnt.DrawText(line[:charsToShow], ts.x-xsoffset, newY, ts.xscl, ts.yscl,
			// 	xshear, Rotation{ts.angle, 0, 0}, ts.bank, ts.align, &ts.window, ts.palfx, ts.frgba[3])
		}

		totalCharsShown += charsToShow
		if totalCharsShown >= int(maxChars) {
			break
		}
	}
}
