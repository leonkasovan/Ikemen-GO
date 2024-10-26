package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/cespare/xxhash"
	mgl "github.com/go-gl/mathgl/mgl32"
	atlas "github.com/ikemen-engine/Ikemen-GO/glh"
	"golang.org/x/exp/maps"
)

// The global, platform-specific rendering backend
var gfx = &Renderer{}

var UIMode bool = false

// Blend constants
type BlendFunc int

const (
	BlendOne = BlendFunc(iota)
	BlendZero
	BlendSrcAlpha
	BlendOneMinusSrcAlpha
)

type BlendEquation int

const (
	BlendAdd = BlendEquation(iota)
	BlendReverseSubtract
)

// Rotation holds rotation parameters
type Rotation struct {
	angle, xangle, yangle float32
}

func (r *Rotation) IsZero() bool {
	return r.angle == 0 && r.xangle == 0 && r.yangle == 0
}

// Tiling holds tiling parameters
type Tiling struct {
	x, y, sx, sy int32
}

var notiling = Tiling{}

// RenderParams holds the common data for all sprite rendering functions
type RenderParams struct {
	// Sprite texture and palette texture
	tex    *Texture
	paltex *Texture
	// Size, position, tiling, scaling and rotation
	size     [2]uint16
	x, y     float32
	tile     Tiling
	xts, xbs float32
	ys, vs   float32
	rxadd    float32
	xas, yas float32
	rot      Rotation
	// Transparency, masking and palette effects
	tint  uint32 // Sprite tint for shadows
	trans int32  // Mugen transparency blending
	mask  int32  // Mask for transparency
	pfx   *PalFX
	// Clipping
	window *[4]int32
	// Rotation center
	rcx, rcy float32
	// Perspective projection
	projectionMode int32
	fLength        float32
	xOffset        float32
	yOffset        float32
	// Texture atlas
	atlas *atlas.TextureAtlas
}

type BatchRenderGlobals struct {
	serializeBuffer         bytes.Buffer
	floatConvertBuffer      []byte
	vertexDataBuffer        [][]float32
	vertexDataBufferCounter int
	vertexCacheBuffer       []map[uint64]bool
}

type RenderUniformData struct {
	atlas    *atlas.TextureAtlas
	window   [4]int32
	eq       BlendEquation // int
	src, dst BlendFunc     // int
	proj     mgl.Mat4
	tex      uint32
	paltex   uint32
	isRgba   int
	mask     int32
	isTropez int
	isFlat   int

	neg        int
	grayscale  float32
	hue        float32
	padd       [3]float32
	pmul       [3]float32
	tint       [4]float32
	alpha      float32
	modelView  mgl.Mat4
	trans      int32
	invblend   int32
	vertexData []float32
	// Possibly implement later
	//x1x2x4x3        [][]float32
	seqNo           int
	forSprite       bool
	UIMode          bool
	isTTF           bool
	ttf             *TtfFont
	vertexDataCache map[uint64]bool
}

type Texture struct {
	width                       int32
	height                      int32
	depth                       int32
	filter                      bool
	handle                      uint32
	atlased                     bool
	region                      atlas.AtlasRegion
	atlas                       *atlas.TextureAtlas
	atlasKey                    AtlasBufferKey
	uvX, uvY, uvWidth, uvHeight float32 // UV coordinates in texture space (0.0 to 1.0)
}

func (rp *RenderParams) IsValid() bool {
	return rp.tex.IsValid() && IsFinite(rp.x+rp.y+rp.xts+rp.xbs+rp.ys+rp.vs+
		rp.rxadd+rp.rot.angle+rp.rcx+rp.rcy)
}

func drawQuads(rd *RenderUniformData, modelview mgl.Mat4, x1, y1, x2, y2, x3, y3, x4, y4, uvX, uvY, uvWidth, uvHeight float32, useAtlas bool) {
	if rd == nil {
		gfx.SetUniformMatrix("modelview", modelview[:])
		gfx.SetUniformF("x1x2x4x3", x1, x2, x4, x3) // this uniform is optional
		fmt.Println("drawQuads 1")
		gfx.SetVertexData(
			x2, y2, 1, 1,
			x3, y3, 1, 0,
			x1, y1, 0, 1,
			x4, y4, 0, 0)

		gfx.RenderQuad()
	} else {
		if useAtlas {
			uvLeft := uvX + uvWidth
			uvTop := uvY
			uvRight := uvX
			uvBottom := uvY + uvHeight

			rd.AppendVertexData([]float32{
				// Adjust vertices to rotate texture counterclockwise
				// First triangle
				x2, y2, uvLeft, uvBottom, // Rotate UVs counterclockwise
				x3, y3, uvLeft, uvTop,
				x1, y1, uvRight, uvBottom,

				// Second triangle
				x1, y1, uvRight, uvBottom, // Keep consistent UV rotation
				x3, y3, uvLeft, uvTop,
				x4, y4, uvRight, uvTop,
			})
		} else {
			rd.AppendVertexData([]float32{
				x2, y2, 1, 1,
				x3, y3, 1, 0,
				x1, y1, 0, 1,

				x1, y1, 0, 1,
				x3, y3, 1, 0,
				x4, y4, 0, 0,
			})
		}
		rd.modelView = modelview
	}
}

// Render a quad with optional horizontal tiling
// func rmTileHSub(modelview mgl.Mat4, x1, y1, x2, y2, x3, y3, x4, y4, dy, width float32, rp RenderParams) {
// rmTileHSub(rd, modelview, x1d, y1d, x2d, y2d, x3d, y3d, x4d, y4d, rp.size[0], rp.tex.uvX, rp.tex.uvY, rp.tex.uvWidth, rp.tex.uvHeight, rp.tile, rp.rcx, rp.atlas != nil)
/*func rmTileHSub(rd *RenderUniformData, modelview mgl.Mat4, x1, y1, x2, y2, x3, y3, x4, y4, dy, width float32, rp RenderParams, useAtlas bool) {
	//            p3
	//    p4 o-----o-----o- - -o
	//      /      |      \     ` .
	//     /       |       \       `.
	//    o--------o--------o- - - - o
	//   p1         p2
	topdist := (x3 - x4) * (((float32(rp.tile.sx) + width) / rp.xas) / width)
	botdist := (x2 - x1) * (((float32(rp.tile.sx) + width) / rp.xas) / width)
	if AbsF(topdist) >= 0.01 {
		db := (x4 - rp.rcx) * (botdist - topdist) / AbsF(topdist)
		x1 += db
		x2 += db
	}

	// Compute left/right tiling bounds (or right/left when topdist < 0)
	xmax := float32(sys.scrrect[2])
	left, right := int32(0), int32(1)
	if rp.tile.x != 0 {
		if topdist >= 0.01 {
			left = 1 - int32(math.Ceil(float64(MaxF(x3/topdist, x2/botdist))))
			right = int32(math.Ceil(float64(MaxF((xmax-x4)/topdist, (xmax-x1)/botdist))))
		} else if topdist <= -0.01 {
			left = 1 - int32(math.Ceil(float64(MaxF((xmax-x3)/-topdist, (xmax-x2)/-botdist))))
			right = int32(math.Ceil(float64(MaxF(x4/-topdist, x1/-botdist))))
		}
		if rp.tile.x != 1 {
			left = 0
			right = Min(right, Max(rp.tile.x, 1))
		}
	}

	// Draw all quads in one loop
	for n := left; n < right; n++ {
		x1d, x2d := x1+float32(n)*botdist, x2+float32(n)*botdist
		x3d, x4d := x3+float32(n)*topdist, x4+float32(n)*topdist
		mat := modelview
		if !rp.rot.IsZero() {
			mat = mat.Mul4(mgl.Translate3D(rp.rcx+float32(n)*botdist, rp.rcy+dy, 0))
			//modelview = modelview.Mul4(mgl.Scale3D(1, rp.vs, 1))
			mat = mat.Mul4(mgl.Rotate3DZ(rp.rot.angle * math.Pi / 180.0).Mat4())
			mat = mat.Mul4(mgl.Translate3D(-(rp.rcx + float32(n)*botdist), -(rp.rcy + dy), 0))
		}

		// drawQuads(mat, x1d, y1, x2d, y2, x3d, y3, x4d, y4)
		drawQuads(rd, modelview, x1d, y1, x2d, y2, x3d, y3, x4d, y4, rp.tex.uvX, rp.tex.uvY, rp.tex.uvWidth, rp.tex.uvHeight, true)
	}
}*/

func rmTileHSub(rd *RenderUniformData, modelview mgl.Mat4, x1, y1, x2, y2, x3, y3, x4, y4, width float32, rp RenderParams, useAtlas bool) {
	//            p3
	//    p4 o-----o-----o- - -o
	//      /      |      \     ` .
	//     /       |       \       `.
	//    o--------o--------o- - - - o
	//   p1         p2
	topdist := (x3 - x4) * (1 + float32(rp.tile.sx)/width)
	botdist := (x2 - x1) * (1 + float32(rp.tile.sx)/width)
	if AbsF(topdist) >= 0.01 {
		db := (x4 - rp.rcx) * (botdist - topdist) / AbsF(topdist)
		x1 += db
		x2 += db
	}

	// Compute left/right tiling bounds (or right/left when topdist < 0)
	xmax := float32(sys.scrrect[2])
	left, right := int32(0), int32(1)
	if topdist >= 0.01 {
		left = 1 - int32(math.Ceil(float64(MaxF(x3/topdist, x2/botdist))))
		right = int32(math.Ceil(float64(MaxF((xmax-x4)/topdist, (xmax-x1)/botdist))))
	} else if topdist <= -0.01 {
		left = 1 - int32(math.Ceil(float64(MaxF((xmax-x3)/-topdist, (xmax-x2)/-botdist))))
		right = int32(math.Ceil(float64(MaxF(x4/-topdist, x1/-botdist))))
	}

	if rp.tile.x != 1 {
		left = 0
		right = Min(right, Max(rp.tile.x, 1))
	}

	buffer := make([]float32, 0)
	xs := make([][]float32, 0)
	// Draw all quads in one loop
	for n := left; n < right; n++ {
		x1d, x2d := x1+float32(n)*botdist, x2+float32(n)*botdist
		x3d, x4d := x3+float32(n)*topdist, x4+float32(n)*topdist
		if sys.batchMode {
			if useAtlas {
				uvLeft := rp.tex.uvX + rp.tex.uvWidth
				uvTop := rp.tex.uvY
				uvRight := rp.tex.uvX
				uvBottom := rp.tex.uvY + rp.tex.uvHeight

				buffer = append(buffer, []float32{
					// Adjust vertices to rotate texture counterclockwise
					// First triangle
					x2d, y2, uvLeft, uvBottom, // Rotate UVs counterclockwise
					x3d, y3, uvLeft, uvTop,
					x1d, y1, uvRight, uvBottom,

					// Second triangle
					x1d, y1, uvRight, uvBottom, // Keep consistent UV rotation
					x3d, y3, uvLeft, uvTop,
					x4d, y4, uvRight, uvTop,
				}...)
			} else {
				buffer = append(buffer, []float32{
					x2d, y2, 1, 1,
					x3d, y3, 1, 0,
					x1d, y1, 0, 1,

					x1d, y1, 0, 1,
					x3d, y3, 1, 0,
					x4d, y4, 0, 0,
				}...)

			}
		} else {
			buffer = append(buffer, []float32{
				x2d, y2, 1, 1,
				x3d, y3, 1, 0,
				x1d, y1, 0, 1,
				x4d, y4, 0, 0,
			}...)
		}
		xs = append(xs, []float32{x1d, x2d, x4d, x3d})
	}
	if len(buffer) > 0 {
		if rd == nil {
			gfx.SetVertexData(buffer...)
			vertex := int32(0)
			for i := 0; i < len(xs); i++ {
				gfx.SetUniformMatrix("modelview", modelview[:])
				gfx.SetUniformF("x1x2x4x3", xs[i][0], xs[i][1], xs[i][2], xs[i][3]) // this uniform is optional
				gfx.RenderQuadAtIndex(vertex)
				vertex += 4
			}
		} else {
			rd.modelView = modelview
			rd.AppendVertexData(buffer)
		}
	}
}

func rmTileSub(modelview mgl.Mat4, rp RenderParams, rd *RenderUniformData) {
	x1, y1 := rp.x+rp.rxadd*rp.ys*float32(rp.size[1]), rp.rcy+((rp.y-rp.ys*float32(rp.size[1]))-rp.rcy)*rp.vs
	x2, y2 := x1+rp.xbs*float32(rp.size[0]), y1
	x3, y3 := rp.x+rp.xts*float32(rp.size[0]), rp.rcy+(rp.y-rp.rcy)*rp.vs
	x4, y4 := rp.x, y3
	//var pers float32
	//if AbsF(rp.xts) < AbsF(rp.xbs) {
	//	pers = AbsF(rp.xts) / AbsF(rp.xbs)
	//} else {
	//	pers = AbsF(rp.xbs) / AbsF(rp.xts)
	//}
	if !rp.rot.IsZero() && rp.tile.x == 0 && rp.tile.y == 0 {

		if rp.vs != 1 {
			y1 = rp.rcy + ((rp.y - rp.ys*float32(rp.size[1])) - rp.rcy)
			y2 = y1
			y3 = rp.y
			y4 = y3
		}
		if rp.projectionMode == 0 {
			modelview = modelview.Mul4(mgl.Translate3D(rp.rcx, rp.rcy, 0))
		} else if rp.projectionMode == 1 {
			// This is the inverse of the orthographic projection matrix
			matrix := mgl.Mat4{float32(sys.scrrect[2] / 2.0), 0, 0, 0, 0, float32(sys.scrrect[3] / 2), 0, 0, 0, 0, -65535, 0, float32(sys.scrrect[2] / 2), float32(sys.scrrect[3] / 2), 0, 1}
			modelview = modelview.Mul4(mgl.Translate3D(0, -float32(sys.scrrect[3]), rp.fLength))
			modelview = modelview.Mul4(matrix)
			modelview = modelview.Mul4(mgl.Frustum(-float32(sys.scrrect[2])/2/rp.fLength, float32(sys.scrrect[2])/2/rp.fLength, -float32(sys.scrrect[3])/2/rp.fLength, float32(sys.scrrect[3])/2/rp.fLength, 1.0, 65535))
			modelview = modelview.Mul4(mgl.Translate3D(-float32(sys.scrrect[2])/2.0, float32(sys.scrrect[3])/2.0, -rp.fLength))
			modelview = modelview.Mul4(mgl.Translate3D(rp.rcx, rp.rcy, 0))
		} else if rp.projectionMode == 2 {
			matrix := mgl.Mat4{float32(sys.scrrect[2] / 2.0), 0, 0, 0, 0, float32(sys.scrrect[3] / 2), 0, 0, 0, 0, -65535, 0, float32(sys.scrrect[2] / 2), float32(sys.scrrect[3] / 2), 0, 1}
			//modelview = modelview.Mul4(mgl.Translate3D(0, -float32(sys.scrrect[3]), 2048))
			modelview = modelview.Mul4(mgl.Translate3D(rp.rcx-float32(sys.scrrect[2])/2.0-rp.xOffset, rp.rcy-float32(sys.scrrect[3])/2.0+rp.yOffset, rp.fLength))
			modelview = modelview.Mul4(matrix)
			modelview = modelview.Mul4(mgl.Frustum(-float32(sys.scrrect[2])/2/rp.fLength, float32(sys.scrrect[2])/2/rp.fLength, -float32(sys.scrrect[3])/2/rp.fLength, float32(sys.scrrect[3])/2/rp.fLength, 1.0, 65535))
			modelview = modelview.Mul4(mgl.Translate3D(rp.xOffset, -rp.yOffset, -rp.fLength))
		}

		modelview = modelview.Mul4(mgl.Scale3D(1, rp.vs, 1))
		modelview = modelview.Mul4(
			mgl.Rotate3DX(-rp.rot.xangle * math.Pi / 180.0).Mul3(
				mgl.Rotate3DY(rp.rot.yangle * math.Pi / 180.0)).Mul3(
				mgl.Rotate3DZ(rp.rot.angle * math.Pi / 180.0)).Mat4())
		modelview = modelview.Mul4(mgl.Translate3D(-rp.rcx, -rp.rcy, 0))

		drawQuads(rd, modelview, x1, y1, x2, y2, x3, y3, x4, y4, rp.tex.uvX, rp.tex.uvY, rp.tex.uvWidth, rp.tex.uvHeight, rp.atlas != nil)
		return
	}
	if rp.tile.y == 1 && rp.xbs != 0 {
		x1d, y1d, x2d, y2d, x3d, y3d, x4d, y4d := x1, y1, x2, y2, x3, y3, x4, y4
		n := 0
		var xy []float32
		for {
			x1d, y1d = x4d, y4d+rp.ys*rp.vs*((float32(rp.tile.sy)+float32(rp.size[1]))/rp.yas-float32(rp.size[1]))
			x2d, y2d = x3d, y1d
			x3d = x4d - rp.rxadd*rp.ys*float32(rp.size[1]) + (rp.xts/rp.xbs)*(x3d-x4d)
			y3d = y2d + rp.ys*rp.vs*float32(rp.size[1])
			x4d = x4d - rp.rxadd*rp.ys*float32(rp.size[1])
			if AbsF(y3d-y4d) < 0.01 {
				break
			}
			y4d = y3d
			if rp.ys*((float32(rp.tile.sy)+float32(rp.size[1]))/rp.yas) < 0 {
				if y1d <= float32(-sys.scrrect[3]) && y4d <= float32(-sys.scrrect[3]) {
					break
				}
			} else if y1d >= 0 && y4d >= 0 {
				break
			}
			n += 1
			xy = append(xy, x1d, x2d, x3d, x4d, y1d, y2d, y3d, y4d)
		}
		for {
			if len(xy) == 0 {
				break
			}
			x1d, x2d, x3d, x4d, y1d, y2d, y3d, y4d, xy = xy[len(xy)-8], xy[len(xy)-7], xy[len(xy)-6], xy[len(xy)-5], xy[len(xy)-4], xy[len(xy)-3], xy[len(xy)-2], xy[len(xy)-1], xy[:len(xy)-8]
			if (0 > y1d || 0 > y4d) &&
				(y1d > float32(-sys.scrrect[3]) || y4d > float32(-sys.scrrect[3])) {
				rmTileHSub(rd, modelview, x1d, y1d, x2d, y2d, x3d, y3d, x4d, y4d, float32(rp.size[0]), rp, rp.atlas != nil)
			}
		}
	}
	if rp.tile.y == 0 || rp.xts != 0 {
		n := rp.tile.y
		for {
			if rp.ys*((float32(rp.tile.sy)+float32(rp.size[1]))/rp.yas) > 0 {
				if y1 <= float32(-sys.scrrect[3]) && y4 <= float32(-sys.scrrect[3]) {
					break
				}
			} else if y1 >= 0 && y4 >= 0 {
				break
			}
			if (0 > y1 || 0 > y4) &&
				(y1 > float32(-sys.scrrect[3]) || y4 > float32(-sys.scrrect[3])) {
				rmTileHSub(rd, modelview, x1, y1, x2, y2, x3, y3, x4, y4, float32(rp.size[0]), rp, rp.atlas != nil)
			}
			if rp.tile.y != 1 && n != 0 {
				n--
			}
			if n == 0 {
				break
			}
			x4, y4 = x1, y1-rp.ys*rp.vs*((float32(rp.tile.sy)+float32(rp.size[1]))/rp.yas-float32(rp.size[1]))
			x3, y3 = x2, y4
			x2 = x1 + rp.rxadd*rp.ys*float32(rp.size[1]) + (rp.xbs/rp.xts)*(x2-x1)
			y2 = y3 - rp.ys*rp.vs*float32(rp.size[1])
			x1 = x1 + rp.rxadd*rp.ys*float32(rp.size[1])
			if AbsF(y1-y2) < 0.01 {
				break
			}
			y1 = y2
		}
	}
}

func rmInitSub(rp *RenderParams) {
	if rp.vs < 0 {
		rp.vs *= -1
		rp.ys *= -1
		rp.rot.angle *= -1
		rp.rot.xangle *= -1
	}
	if rp.tile.x == 0 {
		rp.tile.sx = 0
	} else if rp.tile.sx > 0 {
		rp.tile.sx -= int32(rp.size[0])
	}
	if rp.tile.y == 0 {
		rp.tile.sy = 0
	} else if rp.tile.sy > 0 {
		rp.tile.sy -= int32(rp.size[1])
	}
	if rp.xts >= 0 {
		rp.x *= -1
	}
	rp.x += rp.rcx
	rp.rcy *= -1
	if rp.ys < 0 {
		rp.y *= -1
	}
	rp.y += rp.rcy
}

func processBatch(batch []RenderUniformData) {
	if len(batch) == 0 {
		return
	}
	srd := batch[0]

	// Maybe do this better later
	if srd.isTTF {
		(*srd.ttf).PrintBatch()
		return
	}

	var vertices []float32
	for i := 0; i < len(batch); i++ {
		vertices = append(vertices, batch[i].vertexData...)
	}

	//for i := 0; i < len(batch); i++ {
	UIMode = srd.UIMode
	if srd.forSprite {
		gfx.Scissor(srd.window[0], srd.window[1], srd.window[2], srd.window[3])
	}
	gfx.SetPipeline(srd.eq, srd.src, srd.dst)
	gfx.SetUniformMatrix("projection", srd.proj[:])

	if srd.forSprite {
		if srd.atlas != nil {
			gfx.SetTextureWithAtlas("tex", srd.atlas)
		} else {
			gfx.SetTextureWithHandle("tex", srd.tex)
		}
		if srd.paltex != 0xFFFFFFFF {
			gfx.SetTextureWithHandle("pal", srd.paltex)
		}
		if gfx.setInitialUniforms || srd.isRgba != gfx.lastUsedInBatch.isRgba {
			gfx.SetUniformI("isRgba", int(srd.isRgba))
		}
		if gfx.setInitialUniforms || srd.mask != gfx.lastUsedInBatch.mask {
			gfx.SetUniformI("mask", int(srd.mask))
		}
		if gfx.setInitialUniforms || srd.isTropez != gfx.lastUsedInBatch.isTropez {
			gfx.SetUniformI("isTrapez", srd.isTropez)
		}
		if gfx.setInitialUniforms || srd.neg != gfx.lastUsedInBatch.neg {
			gfx.SetUniformI("neg", srd.neg)
		}
		if gfx.setInitialUniforms || srd.grayscale != gfx.lastUsedInBatch.grayscale {
			gfx.SetUniformF("gray", srd.grayscale)
		}
		if gfx.setInitialUniforms || srd.hue != gfx.lastUsedInBatch.hue {
			gfx.SetUniformF("hue", srd.hue)
		}
		if gfx.setInitialUniforms || srd.padd != gfx.lastUsedInBatch.padd {
			gfx.SetUniformFv("add", srd.padd[:])
		}
		if gfx.setInitialUniforms || srd.pmul != gfx.lastUsedInBatch.pmul {
			gfx.SetUniformFv("mult", srd.pmul[:])
		}
		if gfx.setInitialUniforms || srd.alpha != gfx.lastUsedInBatch.alpha {
			gfx.SetUniformF("alpha", srd.alpha)
		}
	}

	if gfx.setInitialUniforms || srd.isFlat != gfx.lastUsedInBatch.isFlat {
		gfx.SetUniformI("isFlat", int(srd.isFlat))
	}
	if gfx.setInitialUniforms || srd.tint != gfx.lastUsedInBatch.tint {
		gfx.SetUniformFv("tint", srd.tint[:])
	}
	if gfx.setInitialUniforms || srd.modelView != gfx.lastUsedInBatch.modelView {
		gfx.SetUniformMatrix("modelview", srd.modelView[:])
	}

	// Implement chunking and rendering
	maxVerticesPerBatch := int(sys.maxBatchSize) * 6 * 4
	for start := 0; start < len(vertices); start += maxVerticesPerBatch {
		end := start + maxVerticesPerBatch
		if end > len(vertices) {
			end = len(vertices)
		}
		// fmt.Printf("start=%v end=%v len(chunk)=%v len(vertices)=%v\n", start, end, len(vertices[start:end]), len(vertices))
		chunk := vertices[start:end]
		gfx.SetVertexData(chunk...)
		gfx.RenderQuadBatchAtIndex(int32(start), int32((end-start)/4)) // Assuming RenderQuadBatchAtIndex is implemented
	}

	gfx.ReleasePipeline()

	if srd.forSprite {
		gfx.DisableScissor()
	}

	gfx.lastUsedInBatch = srd
	//if gfx.setInitialUniforms {
	gfx.setInitialUniforms = true
	//}
	//}

}

func RenderSprite(rp RenderParams) {
	if !rp.IsValid() {
		return
	}

	rmInitSub(&rp)

	neg, grayscale, padd, pmul, invblend, hue := false, float32(0), [3]float32{0, 0, 0}, [3]float32{1, 1, 1}, int32(0), float32(0)
	tint := [4]float32{float32(rp.tint&0xff) / 255, float32(rp.tint>>8&0xff) / 255,
		float32(rp.tint>>16&0xff) / 255, float32(rp.tint>>24&0xff) / 255}

	if rp.pfx != nil {
		blending := rp.trans
		//if rp.trans == -2 || rp.trans == -1 || (rp.trans&0xff > 0 && rp.trans>>10&0xff >= 255) {
		//	blending = true
		//}
		neg, grayscale, padd, pmul, invblend, hue = rp.pfx.getFcPalFx(false, int(blending))
		//if rp.trans == -2 && invblend < 1 {
		//padd[0], padd[1], padd[2] = -padd[0], -padd[1], -padd[2]
		//}
	}

	proj := mgl.Ortho(0, float32(sys.scrrect[2]), 0, float32(sys.scrrect[3]), -65535, 65535)
	modelview := mgl.Translate3D(0, float32(sys.scrrect[3]), 0)

	gfx.Scissor(rp.window[0], rp.window[1], rp.window[2], rp.window[3])

	renderWithBlending(func(eq BlendEquation, src, dst BlendFunc, a float32) {

		gfx.SetPipeline(eq, src, dst)

		gfx.SetUniformMatrix("projection", proj[:])
		gfx.SetTexture("tex", rp.tex)
		if rp.paltex == nil {
			gfx.SetUniformI("isRgba", 1)
		} else {
			gfx.SetTexture("pal", rp.paltex)
			gfx.SetUniformI("isRgba", 0)
		}
		gfx.SetUniformI("mask", int(rp.mask))
		gfx.SetUniformI("isTrapez", int(Btoi(AbsF(AbsF(rp.xts)-AbsF(rp.xbs)) > 0.001)))
		gfx.SetUniformI("isFlat", 0)

		gfx.SetUniformI("neg", int(Btoi(neg)))
		gfx.SetUniformF("gray", grayscale)
		gfx.SetUniformF("hue", hue)
		gfx.SetUniformFv("add", padd[:])
		gfx.SetUniformFv("mult", pmul[:])
		gfx.SetUniformFv("tint", tint[:])
		gfx.SetUniformF("alpha", a)

		rmTileSub(modelview, rp, nil)

		gfx.ReleasePipeline()
	}, rp.trans, rp.paltex != nil, invblend, &neg, &padd, &pmul, rp.paltex == nil)

	gfx.DisableScissor()
}

func renderWithBlending(render func(eq BlendEquation, src, dst BlendFunc, a float32), trans int32, correctAlpha bool, invblend int32, neg *bool, acolor *[3]float32, mcolor *[3]float32, isrgba bool) {
	blendSourceFactor := BlendSrcAlpha
	if !correctAlpha {
		blendSourceFactor = BlendOne
	}
	Blend := BlendAdd
	BlendI := BlendReverseSubtract
	if invblend >= 1 {
		Blend = BlendReverseSubtract
		BlendI = BlendAdd
	}
	switch {
	// Add blend mode(255,255)
	case trans == -1:
		if invblend >= 1 && acolor != nil {
			(*acolor)[0], (*acolor)[1], (*acolor)[2] = -acolor[0], -acolor[1], -acolor[2]
		}
		if invblend == 3 && neg != nil {
			*neg = false
		}
		render(Blend, blendSourceFactor, BlendOne, 1)
	// Sub blend mode
	case trans == -2:
		if invblend >= 1 && acolor != nil {
			(*acolor)[0], (*acolor)[1], (*acolor)[2] = -acolor[0], -acolor[1], -acolor[2]
		}
		if invblend == 3 && neg != nil {
			*neg = false
		}
		render(BlendI, blendSourceFactor, BlendOne, 1)
	case trans <= 0:
	// Add1(128,128)
	case trans < 255:
		Blend = BlendAdd
		if !isrgba && (invblend >= 2 || invblend <= -1) && acolor != nil && mcolor != nil {
			src, dst := trans&0xff, trans>>10&0xff
			// Summ of add components
			gc := AbsF(acolor[0]) + AbsF(acolor[1]) + AbsF(acolor[2])
			v3, al := MaxF((gc*255)-float32(dst+src), 512)/128, (float32(src+dst) / 255)
			rM, gM, bM := mcolor[0]*al, mcolor[1]*al, mcolor[2]*al
			(*mcolor)[0], (*mcolor)[1], (*mcolor)[2] = rM, gM, bM
			render(BlendAdd, BlendZero, BlendOneMinusSrcAlpha, al)
			render(Blend, blendSourceFactor, BlendOne, al*Pow(v3, 4))
		} else {
			render(Blend, blendSourceFactor, BlendOneMinusSrcAlpha, float32(trans)/255)
		}
	// None
	case trans < 512:
		render(BlendAdd, blendSourceFactor, BlendOneMinusSrcAlpha, 1)
	// AddAlpha
	default:
		src, dst := trans&0xff, trans>>10&0xff
		if dst < 255 {
			render(Blend, BlendZero, BlendOneMinusSrcAlpha, 1-float32(dst)/255)
		}

		if src > 0 {
			if invblend >= 1 && dst >= 255 {
				if invblend >= 2 {
					if invblend == 3 && neg != nil {
						*neg = false
					}
					if acolor != nil {
						(*acolor)[0], (*acolor)[1], (*acolor)[2] = -acolor[0], -acolor[1], -acolor[2]
					}
				}
				Blend = BlendReverseSubtract
			} else {
				Blend = BlendAdd
			}
			if !isrgba && (invblend >= 2 || invblend <= -1) && acolor != nil && mcolor != nil && src < 255 {
				// Summ of add components
				gc := AbsF(acolor[0]) + AbsF(acolor[1]) + AbsF(acolor[2])
				v3, ml, al := MaxF((gc*255)-float32(dst+src), 512)/128, (float32(src) / 255), (float32(src+dst) / 255)
				rM, gM, bM := mcolor[0]*ml, mcolor[1]*ml, mcolor[2]*ml
				(*mcolor)[0], (*mcolor)[1], (*mcolor)[2] = rM, gM, bM
				render(Blend, blendSourceFactor, BlendOne, al*Pow(v3, 3))
			} else {
				render(Blend, blendSourceFactor, BlendOne, float32(src)/255)
			}
		}
	}
}

func FillRect(rect [4]int32, color uint32, trans int32) {
	r := float32(color>>16&0xff) / 255
	g := float32(color>>8&0xff) / 255
	b := float32(color&0xff) / 255

	modelview := mgl.Translate3D(0, float32(sys.scrrect[3]), 0)
	proj := mgl.Ortho(0, float32(sys.scrrect[2]), 0, float32(sys.scrrect[3]), -65535, 65535)

	x1, y1 := float32(rect[0]), -float32(rect[1])
	x2, y2 := float32(rect[0]+rect[2]), -float32(rect[1]+rect[3])

	renderWithBlending(func(eq BlendEquation, src, dst BlendFunc, a float32) {
		gfx.SetPipeline(eq, src, dst)
		gfx.SetVertexData(
			x2, y2, 1, 1,
			x2, y1, 1, 0,
			x1, y2, 0, 1,
			x1, y1, 0, 0)

		gfx.SetUniformMatrix("modelview", modelview[:])
		gfx.SetUniformMatrix("projection", proj[:])
		gfx.SetUniformI("isFlat", 1)
		gfx.SetUniformF("tint", r, g, b, a)
		gfx.RenderQuad()
		gfx.ReleasePipeline()
	}, trans, true, 0, nil, nil, nil, false)
}

func BatchParam(rp *RenderUniformData) {
	if rp != nil {
		sys.paramList = append(sys.paramList, *rp)
	}
}

/*
Get the uniforms that would be generated by this set of
RenderParams, without actually rendering
*/
func CalculateRenderData(rp RenderParams) {
	if !rp.IsValid() {
		return
	}

	rmInitSub(&rp)

	rd := NewRenderUniformData()
	rd.forSprite = true
	rd.UIMode = UIMode

	neg, grayscale, padd, pmul, invblend, hue := false, float32(0), [3]float32{0, 0, 0}, [3]float32{1, 1, 1}, int32(0), float32(0)
	tint := [4]float32{float32(rp.tint&0xff) / 255, float32(rp.tint>>8&0xff) / 255,
		float32(rp.tint>>16&0xff) / 255, float32(rp.tint>>24&0xff) / 255}

	if rp.pfx != nil {
		blending := rp.trans
		//if rp.trans == -2 || rp.trans == -1 || (rp.trans&0xff > 0 && rp.trans>>10&0xff >= 255) {
		//	blending = true
		//}
		neg, grayscale, padd, pmul, invblend, hue = rp.pfx.getFcPalFx(false, int(blending))
		//if rp.trans == -2 && invblend < 1 {
		//padd[0], padd[1], padd[2] = -padd[0], -padd[1], -padd[2]
		//}
	}

	proj := mgl.Ortho(0, float32(sys.scrrect[2]), 0, float32(sys.scrrect[3]), -65535, 65535)
	modelview := mgl.Translate3D(0, float32(sys.scrrect[3]), 0)
	rd.window = *rp.window

	// gfx.Scissor(rp.window[0], rp.window[1], rp.window[2], rp.window[3])
	renderWithBlending(func(eq BlendEquation, src, dst BlendFunc, a float32) {
		rmTileSub(modelview, rp, &rd)
		rd.tex = rp.tex.handle
		rd.atlas = rp.atlas
		rd.eq = eq
		rd.src = src
		rd.dst = dst
		rd.proj = proj
		rd.tex = rp.tex.handle
		if rp.paltex == nil {
			rd.isRgba = 1
			rd.paltex = 0xFFFFFFFF
		} else {
			rd.paltex = rp.paltex.handle
			rd.isRgba = 0
		}
		rd.mask = rp.mask
		rd.isTropez = int(Btoi(AbsF(AbsF(rp.xts)-AbsF(rp.xbs)) > 0.001))
		rd.isFlat = 0
		rd.neg = int(Btoi(neg))
		rd.grayscale = grayscale
		rd.hue = hue
		rd.padd = padd
		rd.pmul = pmul
		rd.tint = tint
		rd.alpha = a
		//rd.modelView = modelview
		//rd.trans = rp.trans
		//rd.invblend = invblend
		BatchParam(&rd)
		rd.seqNo = sys.curSDRSeqNo
		sys.curSDRSeqNo++
		// fmt.Printf("In Prerender: eq: %d src %d dst %d a %f seqNo: %d \n", eq, src, dst, a, rd.seqNo)

	}, rp.trans, rp.paltex != nil, invblend, &neg, &padd, &pmul, rp.paltex == nil)
}

func CalculateRectData(rect [4]int32, color uint32, trans int32) {
	rd := NewRenderUniformData()
	rd.UIMode = UIMode

	r := float32(color>>16&0xff) / 255
	g := float32(color>>8&0xff) / 255
	b := float32(color&0xff) / 255

	modelview := mgl.Translate3D(0, float32(sys.scrrect[3]), 0)
	proj := mgl.Ortho(0, float32(sys.scrrect[2]), 0, float32(sys.scrrect[3]), -65535, 65535)

	x1, y1 := float32(rect[0]), -float32(rect[1])
	x2, y2 := float32(rect[0]+rect[2]), -float32(rect[1]+rect[3])

	renderWithBlending(func(eq BlendEquation, src, dst BlendFunc, a float32) {

		rd.eq = eq
		rd.src = src
		rd.dst = dst
		// rd.vertexData = append(rd.vertexData, []float32{
		// 	x1, y2, 0, 1,
		// 	x1, y1, 0, 0,
		// 	x2, y1, 1, 0,

		// 	x1, y2, 0, 1,
		// 	x2, y1, 1, 0,
		// 	x2, y2, 1, 1,
		// }...)
		rd.AppendVertexData([]float32{
			x1, y2, 0, 1,
			x1, y1, 0, 0,
			x2, y1, 1, 0,

			x1, y2, 0, 1,
			x2, y1, 1, 0,
			x2, y2, 1, 1,
		})
		rd.modelView = modelview
		rd.proj = proj
		rd.isFlat = 1
		rd.tint = [4]float32{r, g, b, a}
		rd.trans = trans
		rd.invblend = 0
		BatchParam(&rd)
		rd.seqNo = sys.curSDRSeqNo
		// fmt.Printf("In Prerender: eq: %d src %d dst %d a %f seqNo: %d \n", eq, src, dst, a, rd.seqNo)

		sys.curSDRSeqNo++
	}, trans, true, 0, nil, nil, nil, false)
}

func NewRenderUniformData() RenderUniformData {
	rud := RenderUniformData{}
	if len(sys.batchGlobals.vertexDataBuffer) == 0 {
		sys.batchGlobals.vertexDataBuffer = make([][]float32, 256)

		for i := 0; i < 256; i++ {
			sys.batchGlobals.vertexDataBuffer[i] = make([]float32, 0, 24)
		}
	}
	if len(sys.batchGlobals.vertexCacheBuffer) == 0 {
		sys.batchGlobals.vertexCacheBuffer = make([]map[uint64]bool, 256)

		for i := 0; i < 256; i++ {
			sys.batchGlobals.vertexCacheBuffer[i] = make(map[uint64]bool)
		}
	}

	if len(sys.batchGlobals.vertexDataBuffer) > sys.batchGlobals.vertexDataBufferCounter {
		rud.vertexData = sys.batchGlobals.vertexDataBuffer[sys.batchGlobals.vertexDataBufferCounter]
		rud.vertexDataCache = sys.batchGlobals.vertexCacheBuffer[sys.batchGlobals.vertexDataBufferCounter]
		sys.batchGlobals.vertexDataBufferCounter++
	} else {
		sys.batchGlobals.vertexDataBuffer = append(sys.batchGlobals.vertexDataBuffer, make([]float32, 0, 24))
		rud.vertexData = sys.batchGlobals.vertexDataBuffer[sys.batchGlobals.vertexDataBufferCounter]
		sys.batchGlobals.vertexCacheBuffer = append(sys.batchGlobals.vertexCacheBuffer, make(map[uint64]bool))
		rud.vertexDataCache = sys.batchGlobals.vertexCacheBuffer[sys.batchGlobals.vertexDataBufferCounter]
		sys.batchGlobals.vertexDataBufferCounter++
	}
	return rud
}

/* Do not be afraid of this */
/*
	Take the Uniform data and turn it into a series of bytes to be hashed.
	XXHASH is super fast, this shouldn't be a problem.
*/
func (r *RenderUniformData) Serialize() ([]byte, error) {
	buf := &sys.batchGlobals.serializeBuffer
	buf.Reset()

	if r.atlas != nil {
		if err := binary.Write(buf, binary.LittleEndian, r.atlas.Handle()); err != nil {
			return nil, err
		}
	} else {
		if err := binary.Write(buf, binary.LittleEndian, uint32(0)); err != nil {
			return nil, err
		}
	}
	if err := binary.Write(buf, binary.LittleEndian, r.window[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(r.eq)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(r.src)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(r.dst)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.proj[:]); err != nil {
		return nil, err
	}
	if r.atlas == nil {
		if err := binary.Write(buf, binary.LittleEndian, r.tex); err != nil {
			return nil, err
		}
	} else {
		if err := binary.Write(buf, binary.LittleEndian, uint32(0)); err != nil {
			return nil, err
		}
	}
	if err := binary.Write(buf, binary.LittleEndian, r.paltex); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, int32(r.isRgba)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.mask); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(r.isTropez)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(r.isFlat)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, int32(r.neg)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.grayscale); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.hue); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.padd[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.pmul[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.tint[:]); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.alpha); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.modelView[:]); err != nil {
		return nil, err
	}

	if err := binary.Write(buf, binary.LittleEndian, r.UIMode); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, r.isTTF); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func batchF32Encode(data []float32) []byte {
	sys.batchGlobals.floatConvertBuffer = sys.batchGlobals.floatConvertBuffer[:0]
	for _, f := range data {
		u := math.Float32bits(f)
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, u)
		sys.batchGlobals.floatConvertBuffer = append(sys.batchGlobals.floatConvertBuffer, b...)
	}
	return sys.batchGlobals.floatConvertBuffer
}

func (r *RenderUniformData) AppendVertexData(vertices []float32) {
	data := batchF32Encode(vertices)

	hash := xxhash.Sum64(data)
	if _, ok := r.vertexDataCache[hash]; !ok {
		r.vertexData = append(r.vertexData, vertices...)
		r.vertexDataCache[hash] = true
	}
}

func BatchRender() {
	if !sys.batchMode {
		return
	}
	var currentBatch []RenderUniformData
	var lastHash uint64
	drawsReduced := 0

	for i := 0; i < len(sys.paramList); i++ {
		entry := sys.paramList[i]
		data, _ := entry.Serialize()
		currentHash := xxhash.Sum64(data)
		if i == 0 || currentHash != lastHash {
			if len(currentBatch) > 0 {
				processBatch(currentBatch)
				drawsReduced += len(currentBatch) - 1
				currentBatch = []RenderUniformData{}
			}
		}

		currentBatch = append(currentBatch, entry)
		lastHash = currentHash

		if i == len(sys.paramList)-1 && len(currentBatch) > 0 {
			processBatch(currentBatch)
			drawsReduced += len(currentBatch) - 1
		}
	}

	for i := 0; i < len(sys.batchGlobals.vertexDataBuffer); i++ {
		sys.batchGlobals.vertexDataBuffer[i] = sys.batchGlobals.vertexDataBuffer[i][:0]
		maps.Clear(sys.batchGlobals.vertexCacheBuffer[i])
	}
	sys.paramList = sys.paramList[:0]
	sys.batchGlobals.vertexDataBufferCounter = 0
	sys.curSDRSeqNo = 0
	//fmt.Println(drawsReduced)
	//gfx.Flush()
}
