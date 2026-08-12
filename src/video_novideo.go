//go:build mugen

package main

// bgVideo is a stub for mugen builds. FFmpeg-backed stage video backgrounds
// require the FFmpeg libraries, which mugen builds do not link. A nil texture
// keeps the engine's existing "no video" code path active.
type bgVideo struct {
	texture Texture
}

func (bgv *bgVideo) Open(filename string, volume int, sm BgVideoScaleMode, sf BgVideoScaleFilter, loop bool) error {
	return Error("video backgrounds are not supported in mugen builds")
}
func (bgv *bgVideo) Tick() error        { return nil }
func (bgv *bgVideo) SetPlaying(on bool) {}
func (bgv *bgVideo) SetVisible(on bool) {}
func (bgv *bgVideo) Reset()             {}
func (bgv *bgVideo) updateAudioVolume() {}
func (bgv *bgVideo) MixerCleared()      {}
func (bgv *bgVideo) Close()             {}
