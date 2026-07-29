package reisen

// #cgo LDFLAGS: -lavutil -lavformat -lavcodec -lswscale -lavfilter
// #include <libavcodec/avcodec.h>
// #include <libavformat/avformat.h>
// #include <libavutil/avutil.h>
// #include <libavutil/imgutils.h>
// #include <libavutil/opt.h>
// #include <libavutil/pixdesc.h>
// #include <libswscale/swscale.h>
// #include <libavfilter/avfilter.h>
// #include <libavfilter/buffersrc.h>
// #include <libavfilter/buffersink.h>
// #include <inttypes.h>
import "C"
import (
	"fmt"
	"unsafe"
)

// VideoStream is a streaming holding
// video frames.
type VideoStream struct {
	baseStream
	swsCtx    *C.struct_SwsContext
	rgbaFrame *C.AVFrame
	bufSize   C.int

	// libavfilter graph (frame-level)
	filterGraph     *C.AVFilterGraph
	bufSrcCtx       *C.AVFilterContext
	bufSinkCtx      *C.AVFilterContext
	filteredFrame   *C.AVFrame
	filterGraphDesc string
	// sws (fallback) lazy-init tracking
	targetW         C.int
	targetH         C.int
	swsAlg          InterpolationAlgorithm
	lastSrcW        C.int
	lastSrcH        C.int
	lastSrcFmt      C.int
}

// AspectRatio returns the fraction of the video
// stream frame aspect ratio (1/0 if unknown).
func (video *VideoStream) AspectRatio() (int, int) {
	return int(video.codecParams.sample_aspect_ratio.num),
		int(video.codecParams.sample_aspect_ratio.den)
}

// Width returns the width of the video
// stream frame.
func (video *VideoStream) Width() int {
	return int(video.codecParams.width)
}

// Height returns the height of the video
// stream frame.
func (video *VideoStream) Height() int {
	return int(video.codecParams.height)
}

// OpenDecode opens the video stream for
// decoding with default parameters.
func (video *VideoStream) Open() error {
	return video.OpenDecode(
		int(video.codecParams.width),
		int(video.codecParams.height),
		InterpolationBicubic)
}

// OpenDecode opens the video stream for
// decoding with the specified parameters.
func (video *VideoStream) OpenDecode(width, height int, alg InterpolationAlgorithm) error {
	err := video.open()

	if err != nil {
		return err
	}

	video.rgbaFrame = C.av_frame_alloc()

	if video.rgbaFrame == nil {
		return fmt.Errorf(
			"couldn't allocate a new RGBA frame")
	}

	// Remember desired output for swscale fallback; swsCtx is created lazily on first frame.
	video.targetW = C.int(width)
	video.targetH = C.int(height)

	video.bufSize = C.av_image_get_buffer_size(
		C.AV_PIX_FMT_RGBA, C.int(width), C.int(height), 1)

	if video.bufSize < 0 {
		return fmt.Errorf(
			"%d: couldn't get the buffer size", video.bufSize)
	}

	buf := (*C.uint8_t)(unsafe.Pointer(
		C.av_malloc(bufferSize(video.bufSize))))

	if buf == nil {
		return fmt.Errorf(
			"couldn't allocate an AV buffer")
	}

	status := C.av_image_fill_arrays(&video.rgbaFrame.data[0],
		&video.rgbaFrame.linesize[0], buf, C.AV_PIX_FMT_RGBA,
		C.int(width), C.int(height), 1)

	if status < 0 {
		return fmt.Errorf(
			"%d: couldn't fill the image arrays", status)
	}

	// Defer SWS context creation until we see the first frame (FFmpeg 8 requires valid src fmt).
	video.swsCtx = nil
	video.swsAlg = alg

	return nil
}

// ApplyVideoFilterGraph builds and enables a libavfilter graph that will run
// on decoded frames (e.g. "scale=640:480:flags=bilinear,format=rgba").
//
// Call this AFTER Open()/OpenDecode(). We replace any existing graph, but
// actual construction is deferred until the first decoded frame so that the
// buffer source matches the frame's colorspace/range/SAR/etc.
func (video *VideoStream) ApplyVideoFilterGraph(graph string) error {
	video.RemoveVideoFilterGraph()
	video.filterGraphDesc = graph
	return nil
}

// buildFilterGraphFromFrame creates the libavfilter graph and buffer source so that
// its properties MATCH the very first decoded frame. This prevents “Changing video
// frame properties…” warnings on FFmpeg 7.x.
func (video *VideoStream) buildFilterGraphFromFrame(f *C.AVFrame) error {
	// Allocate graph
	video.filterGraph = C.avfilter_graph_alloc()
	if video.filterGraph == nil {
		return fmt.Errorf("couldn't allocate AVFilterGraph")
	}

	// Prepare buffer source (input) and buffer sink (output)
	nameBuf := C.CString("buffer"); defer C.free(unsafe.Pointer(nameBuf))
	bufSrc := C.avfilter_get_by_name(nameBuf)
	nameBufSink := C.CString("buffersink"); defer C.free(unsafe.Pointer(nameBufSink))
	bufSink := C.avfilter_get_by_name(nameBufSink)
	if bufSrc == nil || bufSink == nil {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("missing buffer/buffersink filters in libavfilter")
	}

	nameIn := C.CString("in");  defer C.free(unsafe.Pointer(nameIn))
	nameOut := C.CString("out"); defer C.free(unsafe.Pointer(nameOut))

	var ret C.int
	// Create buffer with a minimal arg string so pix_fmt is pinned
	tbNum := video.inner.time_base.num
	tbDen := video.inner.time_base.den
	sar := f.sample_aspect_ratio
	if sar.num == 0 || sar.den == 0 {
		sar.num, sar.den = 1, 1
	}
	args := fmt.Sprintf(
		"video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d",
		int(f.width), int(f.height), int(f.format),
		int(tbNum), int(tbDen),
		int(sar.num), int(sar.den),
	)
	csArgs := C.CString(args)
	defer C.free(unsafe.Pointer(csArgs))

	ret = C.avfilter_graph_create_filter(&video.bufSrcCtx, bufSrc, nameIn, csArgs, nil, video.filterGraph)
	if ret < 0 {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("%d: couldn't create buffer source", int(ret))
	}
	ret = C.avfilter_graph_create_filter(&video.bufSinkCtx, bufSink, nameOut, nil, nil, video.filterGraph)
	if ret < 0 {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("%d: couldn't create buffer sink", int(ret))
	}

	// Set source parameters from the *first frame* so buffer exactly matches incoming properties.
	params := C.av_buffersrc_parameters_alloc()
	if params == nil {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("couldn't alloc AVBufferSrcParameters")
	}
	// Fill params
	params.time_base = video.inner.time_base
	params.width = f.width
	params.height = f.height
	params.format = f.format
	params.sample_aspect_ratio = f.sample_aspect_ratio
	// Colorspace/range/chroma when specified; leave as default otherwise
	if f.colorspace != C.AVCOL_SPC_UNSPECIFIED {
		params.color_space = (C.enum_AVColorSpace)(f.colorspace)
	}
	if f.color_range != C.AVCOL_RANGE_UNSPECIFIED {
		params.color_range = (C.enum_AVColorRange)(f.color_range)
	}
	// NOTE: Older FFmpeg (e.g., 7.1) AVBufferSrcParameters has no chroma_location field,
	// and AVFrame exposes it as 'chroma_location' (not chroma_sample_location).
	// We do not set it to stay source-compatible across 7.1 and 8.x.
	// if f.chroma_location != C.AVCHROMA_LOC_UNSPECIFIED {
	//	params.chroma_location = (C.enum_AVChromaLocation)(f.chroma_location)
	// }
	// Apply
	if ret = C.av_buffersrc_parameters_set(video.bufSrcCtx, params); ret < 0 {
		C.av_free(unsafe.Pointer(params))
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("%d: couldn't set buffersrc parameters", int(ret))
	}
	C.av_free(unsafe.Pointer(params))

	// Wrap our endpoints for graph parsing: [in] -> user graph -> [out]
	inouts := C.avfilter_inout_alloc()
	outputs := C.avfilter_inout_alloc()
	if inouts == nil || outputs == nil {
		if inouts != nil {
			C.avfilter_inout_free(&inouts)
		}
		if outputs != nil {
			C.avfilter_inout_free(&outputs)
		}
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("couldn't allocate AVFilterInOut")
	}
	defer C.avfilter_inout_free(&inouts)
	defer C.avfilter_inout_free(&outputs)

	// outputs: our source node named "in"
	outputs.name = C.av_strdup(nameIn)
	outputs.filter_ctx = video.bufSrcCtx
	outputs.pad_idx = 0
	outputs.next = nil

	// inputs: our sink node named "out"
	inouts.name = C.av_strdup(nameOut)
	inouts.filter_ctx = video.bufSinkCtx
	inouts.pad_idx = 0
	inouts.next = nil

	csGraph := C.CString(video.filterGraphDesc)
	defer C.free(unsafe.Pointer(csGraph))
	ret = C.avfilter_graph_parse_ptr(video.filterGraph, csGraph, &inouts, &outputs, nil)
	if ret < 0 {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("%d: couldn't parse filter graph", int(ret))
	}

	ret = C.avfilter_graph_config(video.filterGraph, nil)
	if ret < 0 {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("%d: couldn't configure filter graph", int(ret))
	}

	// Allocate a reusable frame for filtered output
	video.filteredFrame = C.av_frame_alloc()
	if video.filteredFrame == nil {
		video.RemoveVideoFilterGraph()
		return fmt.Errorf("couldn't allocate filtered frame")
	}

	return nil
}

// RemoveVideoFilterGraph frees the current libavfilter graph (if any).
func (video *VideoStream) RemoveVideoFilterGraph() {
	if video.filteredFrame != nil {
		C.av_frame_free(&video.filteredFrame)
		video.filteredFrame = nil
	}
	if video.filterGraph != nil {
		// Will free all contained filter contexts as well.
		C.avfilter_graph_free(&video.filterGraph)
		video.filterGraph = nil
		video.bufSrcCtx = nil
		video.bufSinkCtx = nil
		video.filterGraphDesc = ""
	}
}

// ReadFrame reads the next frame from the stream.
func (video *VideoStream) ReadFrame() (Frame, bool, error) {
	return video.ReadVideoFrame()
}

// ReadVideoFrame reads the next video frame
// from the video stream.
func (video *VideoStream) ReadVideoFrame() (*VideoFrame, bool, error) {
	return video.readVideoFrameInto(nil)
}

// ReadVideoFrameInto is like ReadVideoFrame but copies pixel data into the
// caller-supplied buffer instead of allocating a new one.  buf must be large
// enough for the output frame (width * height * 4 bytes); if it is nil or too
// small a new buffer is allocated as usual.  The returned VideoFrame.Pix
// points directly into buf, so the caller must not modify buf until the frame
// has been consumed.
func (video *VideoStream) ReadVideoFrameInto(buf []byte) (*VideoFrame, bool, error) {
	return video.readVideoFrameInto(buf)
}

// readVideoFrameInto is the shared implementation for ReadVideoFrame and
// ReadVideoFrameInto.
func (video *VideoStream) readVideoFrameInto(buf []byte) (*VideoFrame, bool, error) {
	ok, err := video.read()

	if err != nil {
		return nil, false, err
	}

	if ok && video.skip {
		return nil, true, nil
	}

	// No more data.
	if !ok {
		return nil, false, nil
	}

	// Lazily build the filtergraph using the *first* decoded frame’s properties.
	if video.filterGraph == nil && video.filterGraphDesc != "" {
		if err := video.buildFilterGraphFromFrame(video.frame); err != nil {
			// Silent fallback to sws_scale; clear desc so we don't retry each frame.
			video.RemoveVideoFilterGraph()
			// (No error returned — caller continues with sws_scale path.)
		}
	}

	// If a libavfilter graph is active, run frame through it and return filtered RGBA.
	if video.filterGraph != nil && video.bufSrcCtx != nil && video.bufSinkCtx != nil && video.filteredFrame != nil {
		// Push decoded frame into the graph
		if ret := C.av_buffersrc_add_frame_flags(video.bufSrcCtx, video.frame, C.AV_BUFFERSRC_FLAG_KEEP_REF); ret < 0 {
			return nil, false, fmt.Errorf("%d: couldn't add frame to buffersrc", int(ret))
		}
		// Pull filtered frame
		if ret := C.av_buffersink_get_frame(video.bufSinkCtx, video.filteredFrame); ret < 0 {
			// EAGAIN should be rare here; treat as no frame available
			return nil, true, nil
		}

		// Extract RGBA bytes (stride-aware copy to a tight buffer)
		w := int(video.filteredFrame.width)
		h := int(video.filteredFrame.height)
		stride := int(video.filteredFrame.linesize[0])
		rowBytes := w * 4
		size := rowBytes * h
		if len(buf) < size {
			buf = make([]byte, size)
		}
		src := uintptr(unsafe.Pointer(video.filteredFrame.data[0]))
		for y := 0; y < h; y++ {
			dstOff := y * rowBytes
			srcPtr := unsafe.Pointer(src + uintptr(y*stride))
			copy(buf[dstOff:dstOff+rowBytes], C.GoBytes(srcPtr, C.int(rowBytes)))
		}
		// We’re done with the filtered frame contents
		C.av_frame_unref(video.filteredFrame)

		frame := newVideoFrame(video, int64(video.frame.pts),
			0, 0,
			w, h, buf)
		return frame, true, nil
	}

	// No filtergraph: fall back to sws_scale to RGBA at the requested size.
	// Lazily (re)create swsCtx from the *actual* source frame properties.
	srcW := video.frame.width
	srcH := video.frame.height
	srcFmt := video.frame.format
	if srcW <= 0 || srcH <= 0 {
		// Guard against invalid frame dims; skip this frame.
		return nil, true, nil
	}
	if srcFmt == C.int(C.AV_PIX_FMT_NONE) || srcFmt < 0 {
		// Source format unknown — skip; decoder should set it on subsequent frames.
		return nil, true, nil
	}
	if video.swsCtx == nil ||
		video.lastSrcW != srcW || video.lastSrcH != srcH || video.lastSrcFmt != srcFmt {
		// (Re)build SWS context using the real source fmt/size.
		if video.swsCtx != nil {
			C.sws_freeContext(video.swsCtx)
			video.swsCtx = nil
		}
		video.swsCtx = C.sws_getContext(
			srcW, srcH, (C.enum_AVPixelFormat)(srcFmt),
			video.targetW, video.targetH,
			C.AV_PIX_FMT_RGBA, C.int(video.swsAlg),
			nil, nil, nil,
		)
		if video.swsCtx == nil {
			// Cannot build SWS; skip frame (caller still progresses decode).
			return nil, true, nil
		}
		video.lastSrcW = srcW
		video.lastSrcH = srcH
		video.lastSrcFmt = srcFmt
	}

	C.sws_scale(video.swsCtx, &video.frame.data[0],
		&video.frame.linesize[0], 0,
		srcH,
		&video.rgbaFrame.data[0],
		&video.rgbaFrame.linesize[0])

	swsSize := int(video.bufSize)
	if len(buf) < swsSize {
		buf = make([]byte, swsSize)
	}
	copy(buf[:swsSize], C.GoBytes(unsafe.Pointer(video.rgbaFrame.data[0]), video.bufSize))
	frame := newVideoFrame(video, int64(video.frame.pts),
		0, 0,
		int(video.codecCtx.width), int(video.codecCtx.height), buf[:swsSize])

	return frame, true, nil
}

// Close closes the video stream for decoding.
func (video *VideoStream) Close() error {
	err := video.close()

	if err != nil {
		return err
	}

	// Free libavfilter graph (if any)
	video.RemoveVideoFilterGraph()

	if video.swsCtx != nil {
		C.sws_freeContext(video.swsCtx)
		video.swsCtx = nil
	}
	C.av_free(unsafe.Pointer(video.rgbaFrame))
	video.rgbaFrame = nil

	return nil
}
