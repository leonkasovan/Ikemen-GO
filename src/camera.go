package main

import "math"

type stageCamera struct {
	startx               int32
	boundleft            int32
	boundright           int32
	boundhigh            int32
	boundlow             int32
	verticalfollow       float32
	floortension         int32
	tensionhigh          int32
	tensionlow           int32
	tension              int32
	tensionvel           float32
	overdrawhigh         int32 //TODO: not implemented
	overdrawlow          int32
	cuthigh              int32
	cutlow               int32
	localcoord           [2]int32
	localscl             float32
	zoffset              int32
	ztopscale            float32
	drawOffsetY          float32
	startzoom            float32
	zoomin               float32
	zoomout              float32
	ytensionenable       bool
	zoomanchor           bool
	fov                  float32
	yshift               float32
	far                  float32
	near                 float32
	aspectCorrection     float32
	zoomAnchorCorrection float32
}

func newStageCamera() *stageCamera {
	return &stageCamera{verticalfollow: 0.2, tensionvel: 1, tension: 50,
		cuthigh: 0, cutlow: 0,
		localcoord: [...]int32{320, 240}, localscl: float32(sys.gameWidth / 320),
		ztopscale: 1, startzoom: 1, zoomin: 1, zoomout: 1, ytensionenable: false, fov: 40, yshift: 0, far: 10000, near: 0.1}
}

type CameraView int

const (
	Fighting_View CameraView = iota
	Follow_View
	Free_View
)

type Camera struct {
	stageCamera
	View                            CameraView
	ZoomEnable, ZoomActive          bool
	ZoomDelayEnable                 bool
	ZoomMin, ZoomMax, ZoomSpeed     float32
	zoomdelay                       float32
	Pos, ScreenPos, Offset          [2]float32
	XMin, XMax                      float32
	Scale, MinScale                 float32
	boundL, boundR, boundH, boundLo float32
	minYZoomDelta                   float32
	zoff                            float32
	screenZoff                      float32
	halfWidth                       float32
	FollowChar                      *Char
}

func newCamera() *Camera {
	return &Camera{View: Fighting_View, ZoomMin: 5.0 / 6, ZoomMax: 15.0 / 14, ZoomSpeed: 12}
}
func (c *Camera) Reset() {
	c.minYZoomDelta = 1
	for _, b := range sys.stage.bg {
		if b.zoomdelta[1] < c.minYZoomDelta {
			c.minYZoomDelta = b.zoomdelta[1]
		}
	}
	c.ZoomEnable = c.ZoomActive && (c.stageCamera.zoomin != 1 || c.stageCamera.zoomout != 1)
	c.boundL = float32(c.boundleft-c.startx)*c.localscl - ((1-c.zoomout)*100*c.zoomout)*(1/c.zoomout)*(1/c.zoomout)*1.6*(float32(sys.gameWidth)/320)
	c.boundR = float32(c.boundright-c.startx)*c.localscl + ((1-c.zoomout)*100*c.zoomout)*(1/c.zoomout)*(1/c.zoomout)*1.6*(float32(sys.gameWidth)/320)
	c.halfWidth = float32(sys.gameWidth) / 2
	c.XMin = c.boundL - c.halfWidth/c.BaseScale()
	c.XMax = c.boundR + c.halfWidth/c.BaseScale()
	c.aspectCorrection = 0
	c.zoomAnchorCorrection = 0
	if float32(c.localcoord[1])*c.localscl-float32(sys.gameHeight) < 0 {
		c.aspectCorrection = MinF(0, (float32(c.localcoord[1])*c.localscl-float32(sys.gameHeight))+MinF((float32(sys.gameHeight)-float32(c.localcoord[1])*c.localscl)/2, float32(c.overdrawlow)*c.localscl))
	} else if float32(c.localcoord[1])*c.localscl-float32(sys.gameHeight) > 0 {
		if c.cuthigh+c.cutlow <= 0 {
			c.aspectCorrection = float32(Ceil(float32(c.localcoord[1])*c.localscl) - sys.gameHeight)
		} else {
			diff := Ceil(float32(c.localcoord[1])*c.localscl) - sys.gameHeight
			tmp := Ceil(float32(c.cuthigh)*c.localscl) * diff / (Ceil(float32(c.cuthigh)*c.localscl) + Ceil(float32(c.cutlow)*c.localscl))
			if diff-tmp <= c.cutlow {
				c.aspectCorrection = float32(tmp)
			} else {
				c.aspectCorrection = float32(diff - Ceil(float32(c.cutlow)*c.localscl))
			}
		}

	}
	c.boundH = float32(c.boundhigh) * c.localscl
	c.boundLo = float32(Max(c.boundhigh, c.boundlow)) * c.localscl

	xminscl := float32(sys.gameWidth) / (float32(sys.gameWidth) - c.boundL +
		c.boundR)
	//yminscl := float32(sys.gameHeight) / (240 - MinF(0, c.boundH))
	c.MinScale = MaxF(c.zoomout, MinF(c.zoomin, xminscl))
	c.screenZoff = float32(c.zoffset-c.localcoord[1])*c.localscl + 240 - c.drawOffsetY
	if c.boundhigh > 0 {
		//c.boundH += float32(c.boundhigh) * c.localscl
		c.screenZoff -= float32(c.boundhigh) * c.localscl
	}
}
func (c *Camera) Init() {
	c.Reset()
	c.View = Fighting_View
}
func (c *Camera) Update(scl, x, y float32) {
	c.Scale = c.BaseScale() * scl
	c.zoff = float32(c.zoffset) * c.localscl
	if sys.stage.stageCamera.zoomanchor {
		c.zoomAnchorCorrection = c.zoff - (float32(sys.gameHeight) + c.aspectCorrection - (float32(sys.gameHeight)-c.zoff+c.aspectCorrection)*scl)
	}
	for i := 0; i < 2; i++ {
		c.Offset[i] = sys.stage.bga.offset[i] * sys.stage.localscl * scl
	}
	c.ScreenPos[0] = x - c.halfWidth/c.Scale - c.Offset[0]
	c.ScreenPos[1] = y - (c.GroundLevel()-float32(sys.gameHeight-240)*scl)/
		c.Scale - c.Offset[1]
	c.Pos[0] = x
	c.Pos[1] = y
}
func (c *Camera) ScaleBound(scl, sclmul float32) float32 {
	if c.ZoomEnable {
		if sys.debugPaused() {
			sclmul = 1
		} else if sys.turbo < 1 {
			sclmul = Pow(sclmul, sys.turbo)
		}
		return MaxF(c.MinScale, MinF(c.zoomin, scl*sclmul))
	}
	return 1
}
func (c *Camera) XBound(scl, x float32) float32 {
	return ClampF(x,
		c.boundL-c.halfWidth+c.halfWidth/scl,
		c.boundR+c.halfWidth-c.halfWidth/scl)
}
func (c *Camera) YBound(scl, y float32) float32 {
	if c.verticalfollow <= 0 {
		return MaxF(0, c.boundLo)
	} else {
		bound := ClampF(y,
			c.boundH*scl,
			c.boundLo*scl)
		return bound
	}
}
func (c *Camera) BaseScale() float32 {
	return c.ztopscale
}
func (c *Camera) GroundLevel() float32 {
	return c.zoff - c.aspectCorrection - c.zoomAnchorCorrection
}
func (c *Camera) ResetZoomdelay() {
	c.zoomdelay = 0
}
func (c *Camera) action(x, y *float32, leftest, rightest, lowest, highest,
	vmin, vmax float32, pause bool) (sclMul float32) {
	switch c.View {
	case Fighting_View:
		tension := MaxF(0, c.halfWidth/c.Scale-float32(c.tension)*c.localscl)
		tmp, vx := (leftest+rightest)/2, vmin+vmax
		// Set base horizontal vel
		vel := float32(3)
		if sys.intro > sys.lifebar.ro.ctrl_time+1 {
			vel = c.halfWidth
		} else if pause {
			vel = 2
		}
		vel *= 2 * c.tensionvel
		// Apply base vel to average vel
		if tmp < 0 {
			vx -= vel
		} else {
			vx += vel
		}
		// Interpolate horizontal vel through GameSpeed/Turbo
		if sys.debugPaused() {
			vx = 0
		} else {
			vx *= MinF(1, sys.turbo)
		}
		// Make sure chars will stay behind tension limits if one of them isn't in a corner
		if vx < 0 {
			tmp = MaxF(leftest+tension, tmp)
			if vx < tmp {
				vx = MinF(0, tmp)
			}
		} else {
			tmp = MinF(rightest-tension, tmp)
			if vx > tmp {
				vx = MaxF(0, tmp)
			}
		}
		*x += vx
		ftension, vfollow, ftensionlow := float32(c.floortension)*c.localscl-c.drawOffsetY, c.verticalfollow, -c.drawOffsetY
		if c.ytensionenable {
			heightValue := (240 / (float32(sys.gameWidth) / float32(c.localcoord[0])))
			ftension = (heightValue/c.Scale - float32(c.tensionhigh) - float32(c.drawOffsetY) - (heightValue - float32(c.zoffset))) * c.localscl
			vfollow = 1
		}
		if ftension < 0 {
			ftension += 240*2 - float32(c.localcoord[1])*c.localscl - 240*c.Scale
			if ftension < 0 {
				ftension = 0
			}
		}
		if highest < -ftension {
			*y = (highest + ftension + MaxF(0, lowest+ftensionlow)) * Pow(vfollow,
				MinF(1, 1/Pow(c.Scale, 4)))
		} else if lowest > -ftensionlow {
			*y = (lowest + ftensionlow) * Pow(vfollow,
				MinF(1, 1/Pow(c.Scale, 4)))
		} else {
			*y = c.Pos[1]
		}
		tmp = (rightest + sys.screenright) - (leftest - sys.screenleft) -
			float32(sys.gameWidth-320)
		if tmp < 0 {
			tmp = 0
		}
		tmp = MaxF(220/c.Scale, float32(math.Sqrt(float64(Pow(tmp, 2)+
			Pow(lowest+float32(c.tensionlow)*c.localscl+67-highest, 2)))))
		sclMul = tmp * c.Scale / MaxF(c.Scale, (400-80*MaxF(1, c.Scale))*
			Pow(2, c.ZoomSpeed-2))
		if sclMul >= 3/Pow(2, c.ZoomSpeed) {
			sclMul = MaxF(3.0/4, 67.0/64-sclMul*Pow(2, c.ZoomSpeed-6))
		} else {
			sclMul = MinF(4.0/3, Pow((Pow(2, c.ZoomSpeed)+3)/Pow(2, c.ZoomSpeed)-
				sclMul, 64))
		}
		// Zoom delay
		if c.ZoomDelayEnable && sclMul > 1 {
			sclMul = (sclMul-1)*Pow(c.zoomdelay, 8) + 1
			if tmp*sclMul > sys.xmax-sys.xmin {
				sclMul = (sys.xmax - sys.xmin) / tmp
			}
			if sys.tickNextFrame() {
				c.zoomdelay = MinF(1, c.zoomdelay+1.0/32)
			}
		} else {
			c.zoomdelay = 0
		}
	case Follow_View:
		*x = c.FollowChar.pos[0]
		*y = c.FollowChar.pos[1] * Pow(c.verticalfollow, MinF(1, 1/Pow(c.Scale, 4)))
		sclMul = 1
	case Free_View:
		*x = c.Pos[0]
		*y = c.Pos[1]
		sclMul = 1
	}
	return
}
