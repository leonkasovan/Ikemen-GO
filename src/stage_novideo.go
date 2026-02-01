//go:build lite

package main

import (
	"math"
	"strings"
)

type backGround struct {
	_type                 BgType
	palfx                 *PalFX
	anim                  *Animation
	bga                   bgAction
	id                    int32
	start                 [2]float32
	xofs                  float32
	delta                 [2]float32
	width                 [2]int32
	xscale                [2]float32
	rasterx               [2]float32
	yscalestart           float32
	yscaledelta           float32
	actionno              int32
	startv                [2]float32
	startrad              [2]float32
	startsint             [2]int32
	startsinlt            [2]int32
	visible               bool
	enabled               bool
	positionlink          bool
	layerno               int32
	autoresizeparallax    bool
	autoresizeparallaxSet bool
	notmaskwindow         int32
	startrect             [4]int32
	windowdelta           [2]float32
	scalestart            [2]float32
	scaledelta            [2]float32
	zoomdelta             [2]float32
	zoomdeltaSet          bool
	zoomscaledelta        [2]float32
	xbottomzoomdelta      float32
	roundpos              bool
	rot                   Rotation
	fLength               float32
	projection            Projection
	xshear                float32
}

func (s *BGDef) action() {
	// TODO: We could merge stage and motif BGCtrl's further. A lot of it is the same
	for i := range s.bgc {
		bgc := &s.bgc[i]
		if bgc.starttime < 0 || (bgc.looptime >= 0 && bgc.starttime >= bgc.looptime) {
			continue
		}

		if bgc.looptime > 0 && bgc.endtime > bgc.looptime {
			bgc.endtime = bgc.looptime
		}

		active := false
		if s.time >= bgc.starttime {
			if bgc.looptime > 0 {
				duration := bgc.endtime - bgc.starttime
				if (s.time-bgc.starttime)%bgc.looptime <= duration {
					active = true
				}
			} else {
				if s.time <= bgc.endtime {
					active = true
				}
			}
		}

		if active {
			s.runBgCtrl(bgc)
		}
	}

	s.bga.action(true)
	if s.model != nil {
		s.model.step(1)
	}

	// Global time must be incremented after updating BGCtrl
	// https://github.com/ikemen-engine/Ikemen-GO/issues/2656
	s.time++

	link := 0
	for i, b := range s.bg {
		s.bg[i].bga.action(b.enabled)
		if i > 0 && b.positionlink {
			s.bg[i].bga.offset[0] += s.bg[link].bga.sinoffset[0]
			s.bg[i].bga.offset[1] += s.bg[link].bga.sinoffset[1]
		} else {
			link = i
		}
		if b.enabled {
			s.bg[i].anim.Action()
		}
	}
}

func readBackGround(is IniSection, link *backGround,
	sff *Sff, at AnimationTable, sProps StageProps, def string, startlayer int32) (*backGround, error) {
	bg := newBackGround(sff)
	typ := is["type"]
	if len(typ) == 0 {
		return bg, nil
	}
	switch typ[0] {
	case 'N', 'n':
		bg._type = BG_Normal
	case 'A', 'a':
		bg._type = BG_Anim
	case 'P', 'p':
		bg._type = BG_Parallax
	case 'D', 'd':
		bg._type = BG_Dummy
	default:
		return bg, nil
	}
	var tmp int32
	is.ReadI32("layerno", &bg.layerno)
	bg.layerno += startlayer

	if bg._type != BG_Dummy {
		var hasAnim bool
		if (bg._type != BG_Normal || len(is["spriteno"]) == 0) &&
			is.ReadI32("actionno", &bg.actionno) {
			if a := at.get(bg.actionno); a != nil {
				bg.anim = a
				hasAnim = true
			}
		}
		if hasAnim {
			if bg._type == BG_Normal {
				bg._type = BG_Anim
			}
		} else {
			var g, n int32
			if is.readI32ForStage("spriteno", &g, &n) {
				bg.anim.frames = []AnimFrame{*newAnimFrame()}
				bg.anim.frames[0].Group, bg.anim.frames[0].Number = g, n
			}
			if is.ReadI32("mask", &tmp) {
				if tmp != 0 {
					bg.anim.mask = 0
				} else {
					bg.anim.mask = -1
				}
			}
		}
	}
	is.ReadBool("positionlink", &bg.positionlink)
	if bg.positionlink && link != nil {
		bg.startv = link.startv
		bg.delta = link.delta
	}
	if _, ok := is["autoresizeparallax"]; ok {
		bg.autoresizeparallaxSet = true
		is.ReadBool("autoresizeparallax", &bg.autoresizeparallax)
	}
	is.readF32ForStage("start", &bg.start[0], &bg.start[1])
	if !bg.positionlink {
		is.readF32ForStage("delta", &bg.delta[0], &bg.delta[1])
	}
	is.readF32ForStage("scalestart", &bg.scalestart[0], &bg.scalestart[1])
	is.readF32ForStage("scaledelta", &bg.scaledelta[0], &bg.scaledelta[1])
	is.readF32ForStage("xshear", &bg.xshear)
	is.readF32ForStage("angle", &bg.rot.angle)
	is.readF32ForStage("xangle", &bg.rot.xangle)
	is.readF32ForStage("yangle", &bg.rot.yangle)
	is.readF32ForStage("focallength", &bg.fLength)
	if str, ok := is["projection"]; ok {
		switch strings.ToLower(strings.TrimSpace(str)) {
		case "orthographic":
			bg.projection = Projection_Orthographic
		case "perspective":
			bg.projection = Projection_Perspective
		case "perspective2":
			bg.projection = Projection_Perspective2
		}
	}
	is.readF32ForStage("xbottomzoomdelta", &bg.xbottomzoomdelta)
	is.readF32ForStage("zoomscaledelta", &bg.zoomscaledelta[0], &bg.zoomscaledelta[1])
	if is.readF32ForStage("zoomdelta", &bg.zoomdelta[0], &bg.zoomdelta[1]) {
		bg.zoomdeltaSet = true
	}
	if bg.zoomdelta[0] != math.MaxFloat32 && bg.zoomdelta[1] == math.MaxFloat32 {
		bg.zoomdelta[1] = bg.zoomdelta[0]
	}

	// Read transparency
	if data, ok := is["trans"]; ok {
		switch strings.ToLower(data) {
		case "add":
			bg.anim.mask = 0
			bg.anim.transType = TT_add
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 255
		case "add1":
			bg.anim.mask = 0
			bg.anim.transType = TT_add
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 128
		case "addalpha":
			bg.anim.mask = 0
			bg.anim.transType = TT_add
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 0 // In Mugen it defaults to this before reading the alpha
		case "sub":
			bg.anim.mask = 0
			bg.anim.transType = TT_sub
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 255
		case "none":
			// In Mugen this does the same as Default
			// TODO: Make ikemenversion fix it
			//bg.anim.transType = TT_none
			bg.anim.transType = TT_default
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 0
		case "default":
			bg.anim.transType = TT_default
			bg.anim.srcAlpha = 255
			bg.anim.dstAlpha = 0
		default:
			return nil, Error("Invalid trans type: " + data)
		}
	}

	// Read alpha if applicable
	if bg.anim.transType == TT_add || bg.anim.transType == TT_sub {
		if _, ok := is["alpha"]; ok {
			s, d := int32(bg.anim.srcAlpha), int32(bg.anim.dstAlpha)
			if is.readI32ForStage("alpha", &s, &d) {
				bg.anim.srcAlpha = int16(Clamp(s, 0, 255))
				bg.anim.dstAlpha = int16(Clamp(d, 0, 255))
			}
		}
	}

	if is.readI32ForStage("tile", &bg.anim.tile.xflag, &bg.anim.tile.yflag) {
		if bg._type == BG_Parallax {
			bg.anim.tile.yflag = 0
		}
		if bg.anim.tile.xflag < 0 {
			bg.anim.tile.xflag = math.MaxInt32
		}
	}
	if bg._type == BG_Parallax {
		if !is.readI32ForStage("width", &bg.width[0], &bg.width[1]) {
			is.readF32ForStage("xscale", &bg.rasterx[0], &bg.rasterx[1])
		}
		is.ReadF32("yscalestart", &bg.yscalestart)
		is.ReadF32("yscaledelta", &bg.yscaledelta)
	} else {
		is.ReadI32("tilespacing", &bg.anim.tile.xspacing, &bg.anim.tile.yspacing)
		//bg.anim.tile.yspacing = bg.anim.tile.xspacing
		if bg.actionno < 0 && len(bg.anim.frames) > 0 {
			group := bg.anim.frames[0].Group
			number := bg.anim.frames[0].Number
			if group >= 0 && number >= 0 {
				if spr := sff.GetSprite(uint16(group), uint16(number)); spr != nil {
					bg.anim.tile.xspacing += int32(spr.Size[0])
					bg.anim.tile.yspacing += int32(spr.Size[1])
				}
			}
		} else {
			if bg.anim.tile.xspacing == 0 {
				bg.anim.tile.xflag = 0
			}
			if bg.anim.tile.yspacing == 0 {
				bg.anim.tile.yflag = 0
			}
		}
	}
	if is.readI32ForStage("window", &bg.startrect[0], &bg.startrect[1],
		&bg.startrect[2], &bg.startrect[3]) {
		bg.startrect[2] = Max(0, bg.startrect[2]+1-bg.startrect[0])
		bg.startrect[3] = Max(0, bg.startrect[3]+1-bg.startrect[1])
		bg.notmaskwindow = 1
	}
	if is.readI32ForStage("maskwindow", &bg.startrect[0], &bg.startrect[1],
		&bg.startrect[2], &bg.startrect[3]) {
		bg.startrect[2] = Max(0, bg.startrect[2]-bg.startrect[0])
		bg.startrect[3] = Max(0, bg.startrect[3]-bg.startrect[1])
		bg.notmaskwindow = 0
	}
	is.readF32ForStage("windowdelta", &bg.windowdelta[0], &bg.windowdelta[1])
	is.ReadI32("id", &bg.id)
	is.readF32ForStage("velocity", &bg.startv[0], &bg.startv[1])
	for i := 0; i < 2; i++ {
		var name string
		if i == 0 {
			name = "sin.x"
		} else {
			name = "sin.y"
		}
		r, slt, st := float32(math.NaN()), float32(math.NaN()), float32(math.NaN())
		if is.readF32ForStage(name, &r, &slt, &st) {
			if !math.IsNaN(float64(r)) {
				bg.startrad[i], bg.bga.radius[i] = r, r
			}
			if !math.IsNaN(float64(slt)) {
				var slti int32
				is.readI32ForStage(name, &tmp, &slti)
				bg.startsinlt[i], bg.bga.sinlooptime[i] = slti, slti
			}
			if bg.bga.sinlooptime[i] > 0 && !math.IsNaN(float64(st)) {
				bg.bga.sintime[i] = int32(st*float32(bg.bga.sinlooptime[i])/360) %
					bg.bga.sinlooptime[i]
				if bg.bga.sintime[i] < 0 {
					bg.bga.sintime[i] += bg.bga.sinlooptime[i]
				}
				bg.startsint[i] = bg.bga.sintime[i]
			}
		}
	}
	if !is.ReadBool("roundpos", &bg.roundpos) {
		bg.roundpos = sProps.roundpos
	}
	return bg, nil
}

func (bg backGround) draw(pos [2]float32, drawscl, bgscl, stglscl float32,
	stgscl [2]float32, shakeY float32, isStage bool) {

	// Handle parallax scaling (type = 2)
	scalestartX := bg.scalestart[0]
	if bg._type == BG_Parallax && (bg.width[0] != 0 || bg.width[1] != 0) && bg.anim.spr != nil {
		bg.xscale[0] = float32(bg.width[0]) / float32(bg.anim.spr.Size[0])
		bg.xscale[1] = float32(bg.width[1]) / float32(bg.anim.spr.Size[0])
		scalestartX = AbsF(scalestartX)
		bg.xofs = scalestartX * ((-float32(bg.width[0]) / 2) + float32(bg.anim.spr.Offset[0])*bg.xscale[0])
		bg.anim.isParallax = true
	}

	// Calculate raster x ratio and base x scale
	xras := (bg.rasterx[1] - bg.rasterx[0]) / bg.rasterx[0]
	xbs, dx := bg.xscale[1], MaxF(0, bg.delta[0]*bgscl)

	// Initialize local scaling factors
	var sclx_recip, sclx, scly float32 = 1, 1, 1
	lscl := [...]float32{stglscl * stgscl[0], stglscl * stgscl[1]}

	// Handle zoom scaling if zoomdelta is specified
	var Yzoomdelta float32 = 1
	if bg.zoomdelta[0] != math.MaxFloat32 {
		sclx = drawscl + (1-drawscl)*(1-bg.zoomdelta[0])
		scly = drawscl + (1-drawscl)*(1-bg.zoomdelta[1])
		Yzoomdelta = bg.zoomdelta[1]
		if !bg.autoresizeparallax {
			sclx_recip = 1 + bg.zoomdelta[0]*((1/(sclx*lscl[0])*lscl[0])-1)
		}
	} else {
		sclx = MaxF(0, drawscl+(1-drawscl)*(1-dx))
		scly = MaxF(0, drawscl+(1-drawscl)*(1-MaxF(0, bg.delta[1]*bgscl)))
		Yzoomdelta = MaxF(0, bg.delta[1]*bgscl)
	}

	// Adjust x scale and x bottom zoom if autoresizeparallax is enabled
	if sclx != 0 && bg.autoresizeparallax {
		tmp := 1 / sclx
		if bg.xbottomzoomdelta != math.MaxFloat32 {
			xbs *= MaxF(0, drawscl+(1-drawscl)*(1-bg.xbottomzoomdelta*(xbs/bg.xscale[0]))) * tmp
		} else {
			xbs *= MaxF(0, drawscl+(1-drawscl)*(1-dx*(xbs/bg.xscale[0]))) * tmp
		}
		tmp *= MaxF(0, drawscl+(1-drawscl)*(1-dx*(xras+1)))
		xras -= tmp - 1
		xbs *= tmp
	}

	// Adjust scaling based on zoomscaledelta if available
	var xs3, ys3 float32 = 1, 1
	if bg.zoomscaledelta[0] != math.MaxFloat32 {
		xs3 = (drawscl + (1-drawscl)*(1-bg.zoomscaledelta[0])) / sclx
	}
	if bg.zoomscaledelta[1] != math.MaxFloat32 {
		ys3 = (drawscl + (1-drawscl)*(1-bg.zoomscaledelta[1])) / scly
	}

	// This handles the flooring of the camera position in MUGEN versions earlier than 1.0.
	var x, yScrollPos float32
	if bg.roundpos {
		x = bg.start[0] + bg.xofs - float32(Floor(pos[0]/stgscl[0]))*bg.delta[0] + bg.bga.offset[0]
		yScrollPos = float32(Floor(pos[1]/drawscl/stgscl[1])) * bg.delta[1]
		for i := 0; i < 2; i++ {
			pos[i] = float32(math.Floor(float64(pos[i])))
		}
	} else {
		x = bg.start[0] + bg.xofs - pos[0]/stgscl[0]*bg.delta[0] + bg.bga.offset[0]
		// Hires breaks ydelta scrolling vel, so bgscl was commented from here.
		yScrollPos = (pos[1] / drawscl / stgscl[1]) * bg.delta[1] // * bgscl
	}

	y := bg.start[1] - yScrollPos + bg.bga.offset[1]

	//In MUGEN, if Boundhigh is a positive value, the reference pos of yscaledelta will change
	var positiveBoundhigh float32
	if isStage && sys.cam.boundhigh > 0 {
		positiveBoundhigh = float32(sys.cam.boundhigh) * bg.delta[1] * bgscl / drawscl / stgscl[1]
	}
	// Calculate Y scaling based on vertical scroll position and delta
	ys2 := bg.scaledelta[1] * pos[1] * bg.delta[1] * bgscl / drawscl / stgscl[1]
	ys := ((100-(pos[1]-positiveBoundhigh)*bg.yscaledelta)*bgscl/bg.yscalestart)*bg.scalestart[1] + ys2
	xs := bg.scaledelta[0] * pos[0] * bg.delta[0] * bgscl / stgscl[0]
	x *= bgscl

	// Apply stage logic if BG is part of a stage
	if isStage {
		zoff := float32(sys.cam.zoffset) * stglscl
		y = y*bgscl + ((zoff-shakeY)/scly-zoff)/stglscl/stgscl[1]
		y -= sys.cam.aspectcorrection / (scly * stglscl * stgscl[1])
		y -= (sys.cam.zoomanchorcorrection / (scly * stglscl * stgscl[1])) * Yzoomdelta
	} else {
		y = y*bgscl + ((float32(sys.gameHeight)-shakeY)/stglscl/scly-240)/stgscl[1]
	}

	// Final scaling factors
	sclx *= lscl[0]
	scly *= stglscl * stgscl[1]

	// Calculate window scale
	var wscl [2]float32
	for i := range wscl {
		if bg.zoomdelta[i] != math.MaxFloat32 {
			wscl[i] = MaxF(0, drawscl+(1-drawscl)*(1-MaxF(0, bg.zoomdelta[i]))) * bgscl * lscl[i]
		} else {
			wscl[i] = MaxF(0, drawscl+(1-drawscl)*(1-MaxF(0, bg.windowdelta[i]*bgscl))) * bgscl * lscl[i]
		}
	}

	// Calculate window top left corner position
	rect := bg.startrect

	startrect0 := float32(rect[0]) - (pos[0])/stgscl[0]*bg.windowdelta[0] +
		(float32(sys.gameWidth)/2/sclx - float32(bg.notmaskwindow)*(float32(sys.gameWidth)/2)*(1/lscl[0]))
	startrect0 *= sys.widthScale * wscl[0]
	if !isStage && wscl[0] == 1 {
		// Screenpacks X coordinates start from left edge of screen
		startrect0 += float32(sys.gameWidth-320) / 2 * sys.widthScale
	}

	startrect1 := float32(rect[1]) - pos[1]/drawscl/stgscl[1]*bg.windowdelta[1]
	if isStage {
		zoff := float32(sys.cam.zoffset) * stglscl
		startrect1 += (zoff-shakeY)/scly - zoff/stglscl/stgscl[1]
		startrect1 -= sys.cam.aspectcorrection / scly
		startrect1 -= (sys.cam.zoomanchorcorrection / scly) * Yzoomdelta
	}
	startrect1 *= sys.heightScale * wscl[1]
	startrect1 -= shakeY

	// Determine final window
	rect[0] = int32(math.Floor(float64(startrect0)))
	rect[1] = int32(math.Floor(float64(startrect1)))
	rect[2] = int32(math.Floor(float64(startrect0 + (float32(rect[2]) * sys.widthScale * wscl[0]) - float32(rect[0]))))
	rect[3] = int32(math.Floor(float64(startrect1 + (float32(rect[3]) * sys.heightScale * wscl[1]) - float32(rect[1]))))

	// Render background if it's within the screen area
	if rect[0] < sys.scrrect[2] && rect[1] < sys.scrrect[3] && rect[0]+rect[2] > 0 && rect[1]+rect[3] > 0 {
		// Xshear offset correction
		xsoffset := -bg.xshear * SignF(bg.scalestart[1]) * (float32(bg.anim.spr.Offset[1]) * scly)

		if bg.rot.angle != 0 {
			xsoffset /= bg.rot.angle
		}

		// Choose render origin: top-left for screenpack/storyboard videos, center for everything else
		var rcx float32
		if bg._type != BG_Video || isStage {
			rcx = float32(sys.gameWidth) / 2
		}

		bg.anim.Draw(&rect, x-xsoffset, y, sclx, scly,
			bg.xscale[0]*bgscl*(scalestartX+xs)*xs3,
			xbs*bgscl*(scalestartX+xs)*xs3,
			ys*ys3, xras*x/(AbsF(ys*ys3)*lscl[1]*float32(bg.anim.spr.Size[1])*bg.scalestart[1])*sclx_recip*bg.scalestart[1]-bg.xshear,
			bg.rot, rcx, bg.palfx, 1, [2]float32{1, 1}, int32(bg.projection), bg.fLength, 0, false)
	}
}

func (s *Stage) action() {
	// Handle Music
	s.music.act()

	link, zlink := 0, -1
	canStep := sys.tickFrame() && !s.paused()

	// Update animations and controllers
	if canStep {
		s.bgCtrlAction()
		s.bga.action(true)

		if s.model != nil {
			s.model.step(sys.turbo)
		}
	}

	// Update BG elements
	for i, b := range s.bg {
		b.palfx.step()

		// BGPalFX can step even if the stage is paused
		if sys.bgPalFX.enable {
			// TODO: Finish proper synthesization of bgPalFX into PalFX from bg element
			// (Right now, bgPalFX just overrides all unique parameters from BG Elements' PalFX)
			// for j := 0; j < 3; j++ {
			// if sys.bgPalFX.invertall {
			// b.palfx.eAdd[j] = -b.palfx.add[j] * (b.palfx.mul[j]/256) + 256 * (1-(b.palfx.mul[j]/256))
			// b.palfx.eMul[j] = 256
			// }
			// b.palfx.eAdd[j] = int32((float32(b.palfx.eAdd[j])) * sys.bgPalFX.eColor)
			// b.palfx.eMul[j] = int32(float32(b.palfx.eMul[j]) * sys.bgPalFX.eColor + 256*(1-sys.bgPalFX.eColor))
			// }
			// b.palfx.synthesize(sys.bgPalFX)
			b.palfx.eAdd = sys.bgPalFX.eAdd
			b.palfx.eMul = sys.bgPalFX.eMul
			b.palfx.eColor = sys.bgPalFX.eColor
			b.palfx.eHue = sys.bgPalFX.eHue
			b.palfx.eInvertall = sys.bgPalFX.eInvertall
			b.palfx.eInvertblend = sys.bgPalFX.eInvertblend
			b.palfx.eAllowNeg = sys.bgPalFX.eAllowNeg
		}

		if canStep {
			s.bg[i].bga.action(b.enabled)
			if i > 0 && b.positionlink {
				bgasinoffset0 := s.bg[link].bga.sinoffset[0]
				bgasinoffset1 := s.bg[link].bga.sinoffset[1]
				if s.hires {
					bgasinoffset0 = bgasinoffset0 / 2
					bgasinoffset1 = bgasinoffset1 / 2
				}
				s.bg[i].bga.offset[0] += bgasinoffset0
				s.bg[i].bga.offset[1] += bgasinoffset1
			} else {
				link = i
			}
			if s.zoffsetlink >= 0 && zlink < 0 && b.id == s.zoffsetlink {
				zlink = i
				s.bga.offset[1] += b.bga.offset[1]
			}
		}

		if b.enabled && canStep {
			s.bg[i].anim.Action()
		}
	}

	// Update model PalFX
	if s.model != nil {
		s.model.pfx.step()
		if sys.bgPalFX.enable {
			s.model.pfx.eAdd = sys.bgPalFX.eAdd
			s.model.pfx.eMul = sys.bgPalFX.eMul
			s.model.pfx.eColor = sys.bgPalFX.eColor
			s.model.pfx.eHue = sys.bgPalFX.eHue
			s.model.pfx.eInvertall = sys.bgPalFX.eInvertall
			s.model.pfx.eInvertblend = sys.bgPalFX.eInvertblend
			s.model.pfx.eAllowNeg = sys.bgPalFX.eAllowNeg
		}
	}
}

func (s *Stage) reset() {
	s.stageTime = 0
	s.sff.palList.ResetRemap()
	s.bga.clear()
	for i := range s.bg {
		s.bg[i].reset()
	}
	if s.model != nil {
		s.model.reset()
	}
	// No need to reset BGCtrl at the moment. Tied to stagetime
}

// destroy stops any background video media so the stage can be safely discarded.
func (s *Stage) destroy() {
	// do nothing.
}

func (s *System) clearMatchSound() {
	s.stopAllCharSound()
}
