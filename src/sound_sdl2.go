//go:build sdl

/*
sound.go:
cns File -> Snd.Play -> SoundChannels.Play -> SoundChannel.Play -> Sound.Play

sound_sdl2.go:
cns File -> Snd.Play -> Sound.Play
*/
package main

/*
#cgo LDFLAGS: -lSDL2 -lSDL2_mixer
#include <stdlib.h>
#if defined(__WIN32)
	#include <SDL2/SDL_mixer.h>
#else
	#include <SDL_mixer.h>
#endif
*/
import "C"
import (
	"encoding/binary"
	"fmt"
	"os"
	"unsafe"
)

const (
	audioOutLen    = 2048
	audioFrequency = 48000
	audioPrecision = 4
)

// ------------------------------------------------------------------
// Bgm

type Bgm struct {
	filename   string
	bgmVolume  int
	volRestore int
	loop       int
	// streamer   beep.StreamSeeker
	// ctrl       *beep.Ctrl
	// volctrl    *effects.Volume
	format  string
	freqmul float32
	// sampleRate beep.SampleRate
	startPos int
	music    *C.Mix_Music
}

func newBgm() *Bgm {
	return &Bgm{}
}

func (bgm *Bgm) Open(filename string, loop, bgmVolume, bgmLoopStart, bgmLoopEnd, startPosition int, freqmul float32) {
	// fmt.Printf("[sound_sdl2.go][bgm.Open] filename=%v, loop=%v, bgmVolume=%v, bgmLoopStart=%v, bgmLoopEnd=%v, startPosition =%v, freqmul=%v\n",
	// 	filename, loop, bgmVolume, bgmLoopStart, bgmLoopEnd, startPosition, freqmul)
	bgm.filename = filename
	bgm.loop = loop
	bgm.bgmVolume = bgmVolume
	bgm.freqmul = freqmul

	// Special value "" is used to stop music
	if filename == "" {
		C.Mix_HaltMusic()
		return
	}

	if HasExtension(bgm.filename, ".ogg") {
		bgm.music = C.Mix_LoadMUS(C.CString(filename))
		bgm.format = "ogg"
	} else if HasExtension(bgm.filename, ".mp3") {
		// bgm.music = C.Mix_LoadMUS(C.CString("sound\\" + filename))
		bgm.music = C.Mix_LoadMUS(C.CString(filename))
		bgm.format = "mp3"
	} else if HasExtension(bgm.filename, ".wav") {
		bgm.music = C.Mix_LoadMUS(C.CString(filename))
		bgm.format = "wav"
	} else if HasExtension(bgm.filename, ".mid") || HasExtension(bgm.filename, ".midi") {
		bgm.music = C.Mix_LoadMUS(C.CString(filename))
		bgm.format = "midi"
	} else {
		fmt.Printf("[sound_sdl2.go][bgm.Open] Unsupported file extension: %v\n", bgm.filename)
		return
	}
	if bgm.music == nil {
		fmt.Printf("[sound_sdl2.go][bgm.Open] Failed to load bgm: %v\n", C.GoString(C.Mix_GetError()))
		return
	}
	fmt.Printf("[sound_sdl2.go][bgm.Open] filename=%v bgm.format=%v\n", filename, bgm.format)
	if C.Mix_PlayMusic(bgm.music, -1) != 0 {
		fmt.Printf("Unable to play music: %s\n", C.GoString(C.Mix_GetError()))
		return
	}
}

func (bgm *Bgm) SetPaused(pause bool) {
	// fmt.Printf("[sound_sdl2.go][bgm.SetPaused]\n")
	if bgm.music == nil {
		return
	}
	if pause {
		C.Mix_PauseMusic()
	} else {
		C.Mix_ResumeMusic()
	}
}

func (bgm *Bgm) UpdateVolume() {
	fmt.Printf("[sound_sdl2.go][bgm.UpdateVolume] %v\n", sys.bgmVolume)
	C.Mix_VolumeMusic(C.int(C.MIX_MAX_VOLUME * sys.bgmVolume / 100))
}

func (bgm *Bgm) SetFreqMul(freqmul float32) {
	// do nothing
	fmt.Printf("[sound_sdl2.go][bgm.SetFreqMul]\n")
}

func (bgm *Bgm) SetLoopPoints(bgmLoopStart int, bgmLoopEnd int) {
	// do nothing
	fmt.Printf("[sound_sdl2.go][bgm.SetLoopPoints]\n")
}

func (bgm *Bgm) Seek(positionSample int) {
	// do nothing
	fmt.Printf("[sound_sdl2.go][bgm.Seek]\n")
}

func (bgm *Bgm) GetLength() int {
	fmt.Printf("[sound_sdl2.go][bgm.GetLength]\n")
	return 0
}

func (bgm *Bgm) GetPosition() int {
	fmt.Printf("[sound_sdl2.go][bgm.GetPosition]\n")
	return 0
}

func (bgm *Bgm) GetLoopStart() int {
	fmt.Printf("[sound_sdl2.go][bgm.GetLoopStart]\n")
	return 0
}

func (bgm *Bgm) GetLoopEnd() int {
	fmt.Printf("[sound_sdl2.go][bgm.GetLoopEnd]\n")
	return 0
}

func (bgm *Bgm) Loaded() bool {
	fmt.Printf("[sound_sdl2.go][bgm.Loaded]\n")
	return (bgm.music != nil)
}

// ------------------------------------------------------------------
// Sound ==> mix.Chunk

// type Sound struct {
// 	wavData []byte
// 	format  beep.Format
// 	length  int
// }

// type Chunk struct {
// 	allocated int32  // a boolean indicating whether to free abuf when the chunk is freed
// 	buf       *uint8 // pointer to the sample data, which is in the output format and sample rate
// 	len_      uint32 // length of abuf in bytes
// 	volume    uint8  // 0 = silent, 128 = max volume. This takes effect when mixing
// }

type Sound C.Mix_Chunk

func (s *Sound) Play(channel, loops int) (channel_ int32, err error) {
	// fmt.Printf("[sound_sdl2.go] Sound.Play channel=%v, loops=%v\n", channel, loops)
	channel_ = int32(C.Mix_PlayChannel(-1, (*C.Mix_Chunk)(s), C.int(loops)))
	if channel_ == -1 {
		err = Error(fmt.Sprintf("Unable to play sound effect: %s", C.GoString(C.Mix_GetError())))
		fmt.Println(err)
		return -1, err
	}
	return channel_, nil
}

// LoadWAV loads file for use as a sample. This is actually mix.LoadWAVRW(sdl.RWFromFile(file, "rb"), 1). This can load WAVE, AIFF, RIFF, OGG, and VOC files. Note: You must call SDL_OpenAudio before this. It must know the output characteristics so it can convert the sample for playback, it does this conversion at load time. Returns: a pointer to the sample as a mix.Chunk.
// (https://www.libsdl.org/projects/SDL_mixer/docs/SDL_mixer_19.html)
func LoadWAV(wavData []byte, size uint32) (chunk *Sound, err error) {
	chunk = (*Sound)(unsafe.Pointer(C.Mix_LoadWAV_RW(C.SDL_RWFromMem(unsafe.Pointer(&wavData[0]), (C.int)(size)), 1)))
	if chunk == nil {
		err = Error(fmt.Sprintf("Error LoadWAV: %s", C.GoString(C.Mix_GetError())))
		fmt.Println(err)
	}
	return
}

func readSound(f *os.File, size uint32) (*Sound, error) {
	if size < 128 {
		return nil, fmt.Errorf("Error readSound: wav size is too small")
	}
	wavData := make([]byte, size)
	if _, err := f.Read(wavData); err != nil {
		return nil, err
	}

	return LoadWAV(wavData, size)
}

// ------------------------------------------------------------------
// Snd

type Snd struct {
	table     map[[2]int32]*Sound
	ver, ver2 uint16
}

func newSnd() *Snd {
	// fmt.Printf("[sound_sdl2.go] newSndn\n")
	return &Snd{table: make(map[[2]int32]*Sound)}
}

func LoadSnd(filename string) (*Snd, error) {
	fmt.Printf("[sound_sdl2.go] LoadSnd %v\n", filename)
	return LoadSndFiltered(filename, func(gn [2]int32) bool { return gn[0] >= 0 && gn[1] >= 0 }, 0)
}

// Parse a .snd file and return an Snd structure with its contents
// The "keepItem" function allows to filter out unwanted waves.
// If max > 0, the function returns immediately when a matching entry is found. It also gives up after "max" non-matching entries.
func LoadSndFiltered(filename string, keepItem func([2]int32) bool, max uint32) (*Snd, error) {
	// fmt.Printf("[sound_sdl2.go] LoadSndFiltered %v\n", filename)
	s := newSnd()
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { chk(f.Close()) }()
	buf := make([]byte, 12)
	var n int
	if n, err = f.Read(buf); err != nil {
		return nil, err
	}
	if string(buf[:n]) != "ElecbyteSnd\x00" {
		return nil, Error("Unrecognized SND file, invalid header")
	}
	read := func(x interface{}) error {
		return binary.Read(f, binary.LittleEndian, x)
	}
	if err := read(&s.ver); err != nil {
		return nil, err
	}
	if err := read(&s.ver2); err != nil {
		return nil, err
	}
	var numberOfSounds uint32
	if err := read(&numberOfSounds); err != nil {
		return nil, err
	}
	var subHeaderOffset uint32
	if err := read(&subHeaderOffset); err != nil {
		return nil, err
	}
	loops := numberOfSounds
	if max > 0 && max < numberOfSounds {
		loops = max
	}
	for i := uint32(0); i < loops; i++ {
		f.Seek(int64(subHeaderOffset), 0)
		var nextSubHeaderOffset uint32
		if err := read(&nextSubHeaderOffset); err != nil {
			return nil, err
		}
		var subFileLength uint32
		if err := read(&subFileLength); err != nil {
			return nil, err
		}
		var num [2]int32
		if err := read(&num); err != nil {
			return nil, err
		}
		if keepItem(num) {
			_, ok := s.table[num]
			if !ok {
				tmp, err := readSound(f, subFileLength)
				if err != nil {
					sys.errLog.Printf("%v sound %v,%v can't be read: %v\n", filename, num[0], num[1], err)
					if max > 0 {
						return nil, err
					}
				} else {
					// Sound is corrupted and can't be played, so we export a warning message to the console
					if tmp == nil {
						sys.appendToConsole(fmt.Sprintf("WARNING: %v sound %v,%v is corrupted and can't be played, so it was disabled", filename, num[0], num[1]))
					}
					s.table[num] = tmp
					if max > 0 {
						break
					}
				}
			}
		}
		subHeaderOffset = nextSubHeaderOffset
	}
	return s, nil
}
func (s *Snd) Get(gn [2]int32) *Sound {
	// fmt.Printf("[sound_sdl2.go] Snd.Get %v\n", gn)
	return s.table[gn]
}
func (s *Snd) play(gn [2]int32, volumescale int32, pan float32, loopstart, loopend, startposition int) bool {
	// fmt.Printf("[sound_sdl2.go] Snd.Play %v\n", gn)
	_, err := s.Get(gn).Play(-1, 0)
	return err != nil
}
func (s *Snd) stop(gn [2]int32) {
	// fmt.Printf("[sound_sdl2.go] Snd.Stop %v\n", gn)
	C.Mix_HaltChannel(-1)
}

func loadFromSnd(filename string, g, s int32, max uint32) (*Sound, error) {
	fmt.Printf("[sound_sdl2.go] loadFromSnd filename=%v %v,%v\n", filename, g, s)
	// Load the snd file
	snd, err := LoadSndFiltered(filename, func(gn [2]int32) bool { return gn[0] == g && gn[1] == s }, max)
	if err != nil {
		return nil, err
	}
	tmp, ok := snd.table[[2]int32{g, s}]
	if !ok {
		return nil, nil
	}
	return tmp, nil
}

// ------------------------------------------------------------------
// SoundEffect (handles volume and panning)

type SoundEffect struct {
	// streamer beep.Streamer
	volume   float32
	ls, p    float32
	x        *float32
	priority int32
	channel  int32
	loop     int32
	freqmul  float32
}

// func (s *SoundEffect) Stream(samples [][2]float64) (n int, ok bool) {
// TODO: Test mugen panning in relation to PanningWidth and zoom settings
// lv, rv := s.volume, s.volume
// if sys.stereoEffects && (s.x != nil || s.p != 0) {
// 	var r float32
// 	if s.x != nil { // pan
// 		r = ((sys.xmax - s.ls**s.x) - s.p) / (sys.xmax - sys.xmin)
// 	} else { // abspan
// 		r = ((sys.xmax-sys.xmin)/2 - s.p) / (sys.xmax - sys.xmin)
// 	}
// 	sc := sys.panningRange / 100
// 	of := (100 - sys.panningRange) / 200
// 	lv = ClampF(s.volume*2*(r*sc+of), 0, 512)
// 	rv = ClampF(s.volume*2*((1-r)*sc+of), 0, 512)
// }

// n, ok = s.streamer.Stream(samples)
// for i := range samples[:n] {
// 	samples[i][0] *= float64(lv / 256)
// 	samples[i][1] *= float64(rv / 256)
// }
// 	n = 1
// 	ok = true
// 	return n, ok
// }

// ------------------------------------------------------------------
// SoundChannel

type SoundChannel struct {
	// streamer          beep.StreamSeeker
	sfx      *SoundEffect
	volume   float32
	ls, p    float32
	x        *float32
	priority int32
	// channel  int32	// use sfx.channel instead
	loop    int32
	freqmul float32
	// ctrl              *beep.Ctrl
	sound             *Sound
	stopOnGetHit      bool
	stopOnChangeState bool
}

func (s *SoundChannel) GetCtrl() *Sound {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.GetCtrl\n")
	return s.sound
}

func (s *SoundChannel) Play(sound *Sound, loop int32, freqmul float32, loopStart, loopEnd, startPosition int) {
	if sound == nil {
		return
	}
	// fmt.Printf("[sound_sdl2.go] SoundChannel.Play %v\n", sound)
	ch, err := sound.Play(-1, int(loop))
	if err == nil {
		s.sound = sound
		s.sfx = &SoundEffect{volume: 100, priority: 0, channel: ch, loop: loop, freqmul: freqmul}
	}
}
func (s *SoundChannel) IsPlaying() bool {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.IsPlaying\n")
	return s.sfx != nil && C.Mix_Playing(C.int(s.sfx.channel)) == 1
}
func (s *SoundChannel) SetPaused(pause bool) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.SetPaused\n")
	if s.sfx != nil {
		if pause {
			C.Mix_Pause(C.int(s.sfx.channel))
		} else {
			C.Mix_Resume(C.int(s.sfx.channel))
		}
	}
}
func (s *SoundChannel) GetPaused() bool {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.GetPaused\n")
	if s.sfx != nil {
		if C.Mix_Paused(C.int(s.sfx.channel)) == 1 {
		} else {
			return false
		}
	}
	return false
}
func (s *SoundChannel) Stop() {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.Stop\n")
	if s.sfx != nil {
		C.Mix_HaltChannel(C.int(s.sfx.channel))
	}
}
func (s *SoundChannel) SetVolume(vol float32) {
	if s.sfx != nil {
		s.sfx.volume = ClampF(vol, 0, 512)
		C.Mix_Volume(-1, C.int(C.MIX_MAX_VOLUME*vol/512))
	}
}
func (s *SoundChannel) SetPan(p, ls float32, x *float32) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.SetPan\n")
	// if s.ctrl != nil {
	// 	s.sfx.ls = ls
	// 	s.sfx.x = x
	// 	s.sfx.p = p * ls
	// }
}
func (s *SoundChannel) SetPriority(priority int32) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.SetPriority\n")
	// if s.ctrl != nil {
	// 	s.sfx.priority = priority
	// }
}
func (s *SoundChannel) SetChannel(channel int32) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.SetChannel\n")
	// if s.ctrl != nil {
	// 	s.sfx.channel = channel
	// }
}
func (s *SoundChannel) SetFreqMul(freqmul float32) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.SetFreqMul\n")
	// if s.ctrl != nil {
	// 	if s.sound != nil {
	// 		srcRate := s.sound.format.SampleRate
	// 		dstRate := beep.SampleRate(float32(sys.audioSampleRate) / freqmul)
	// 		if resampler, ok := s.ctrl.Streamer.(*beep.Resampler); ok {
	// 			speaker.Lock()
	// 			resampler.SetRatio(float64(srcRate) / float64(dstRate))
	// 			s.sfx.freqmul = freqmul
	// 			speaker.Unlock()
	// 		}
	// 	}
	// }
}
func (s *SoundChannel) SetLoopPoints(loopstart, loopend int) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.SetLoopPoints\n")
	// Set both at once, why not
	// if sl, ok := s.sfx.streamer.(*StreamLooper); ok {
	// 	if sl.loopstart != loopstart && sl.loopend != loopend {
	// 		speaker.Lock()
	// 		sl.loopstart = loopstart
	// 		sl.loopend = loopend
	// 		speaker.Unlock()
	// 		// Set one at a time
	// 	} else {
	// 		if sl.loopstart != loopstart {
	// 			speaker.Lock()
	// 			sl.loopstart = loopstart
	// 			speaker.Unlock()
	// 		} else if sl.loopend != loopend {
	// 			speaker.Lock()
	// 			sl.loopend = loopend
	// 			speaker.Unlock()
	// 		}
	// 	}
	// }
}
func (s *SoundChannel) Seek(channel int) {
	// fmt.Printf("[sound_sdl2.go] SoundChannel.Seek\n")
	// if s.ctrl != nil {
	// 	s.sfx.channel = channel
	// }
}

// ------------------------------------------------------------------
// SoundChannels (collection of prioritised sound channels)

type SoundChannels struct {
	channels  []SoundChannel
	volResume []float32
}

func newSoundChannels(size int32) *SoundChannels {
	s := &SoundChannels{}
	s.SetSize(size)
	return s
}
func (s *SoundChannels) SetSize(size int32) {
	if size > s.count() {
		c := make([]SoundChannel, size-s.count())
		v := make([]float32, size-s.count())
		s.channels = append(s.channels, c...)
		s.volResume = append(s.volResume, v...)
	} else if size < s.count() {
		for i := s.count() - 1; i >= size; i-- {
			s.channels[i].Stop()
		}
		s.channels = s.channels[:size]
		s.volResume = s.volResume[:size]
	}
}
func (s *SoundChannels) count() int32 {
	return int32(len(s.channels))
}
func (s *SoundChannels) New(ch int32, lowpriority bool, priority int32) *SoundChannel {
	// fmt.Printf("[sound_sdl2.go] SoundChannels.New ch=%v lowpriority=%v priority=%v\n", ch, lowpriority, priority)
	if ch >= 0 && ch < sys.wavChannels {
		for i := s.count() - 1; i >= 0; i-- {
			if s.channels[i].IsPlaying() && s.channels[i].sfx.channel == ch {
				if (lowpriority && priority <= s.channels[i].sfx.priority) || priority < s.channels[i].sfx.priority {
					return nil
				}
				s.channels[i].Stop()
				return &s.channels[i]
			}
		}
	}
	if s.count() < sys.wavChannels {
		s.SetSize(sys.wavChannels)
	}
	for i := sys.wavChannels - 1; i >= 0; i-- {
		if !s.channels[i].IsPlaying() {
			return &s.channels[i]
		}
	}
	return nil
}
func (s *SoundChannels) reserveChannel() *SoundChannel {
	for i := range s.channels {
		if !s.channels[i].IsPlaying() {
			return &s.channels[i]
		}
	}
	return nil
}
func (s *SoundChannels) Get(ch int32) *SoundChannel {
	if ch >= 0 && ch < s.count() {
		for i := range s.channels {
			if s.channels[i].IsPlaying() && s.channels[i].sfx != nil && s.channels[i].sfx.channel == ch {
				return &s.channels[i]
			}
		}
		//return &s.channels[ch]
	}
	return nil
}

func (s *SoundChannels) IsPlaying(sound *Sound) bool {
	for _, v := range s.channels {
		if v.IsPlaying() && v.sound == sound {
			return true
		}
	}
	return false
}
func (s *SoundChannels) Stop(sound *Sound) {
	for _, v := range s.channels {
		if v.IsPlaying() && v.sound == sound {
			v.Stop()
		}
	}
}
func (s *SoundChannels) StopAll() {
	for _, v := range s.channels {
		if v.IsPlaying() {
			v.Stop()
		}
	}
}
func (s *SoundChannels) Tick() {
	// for i := range s.channels {
	// 	if s.channels[i].sfx != nil {
	// 		if C.Mix_Playing(C.int(s.channels[i].sfx.channel)) == 0 {
	// 			s.channels[i].sound = nil
	// 		}
	// 	}
	// }
}

func (s *System) PlaySound(sound *Sound) {
	sound.Play(-1, 0)
}

func AudioInit() (err Error) {
	// Initialize SDL_mixer with MP3 and Ogg (MIX_INIT_MOD, MIX_INIT_FLAC) support
	if C.Mix_Init(C.MIX_INIT_MP3|C.MIX_INIT_OGG) == 0 {
		err = Error(fmt.Sprintf("Mix_Init error: %s\n", C.GoString(C.Mix_GetError())))
		fmt.Print(err)
		return err
	}
	if C.Mix_OpenAudio(C.int(sys.audioSampleRate), C.MIX_DEFAULT_FORMAT, 2, audioOutLen) != 0 {
		err = Error(fmt.Sprintf("Mix_OpenAudio error: %s\n", C.GoString(C.Mix_GetError())))
		fmt.Print(err)
		return err
	}
	fmt.Printf("[sound_sdl2.go] sys.bgmVolume=%v sys.wavVolume=%v sys.wavChannels=%v\n", sys.bgmVolume, sys.wavVolume, sys.wavChannels)
	C.Mix_VolumeMusic(C.int(C.MIX_MAX_VOLUME * sys.bgmVolume / 100))
	C.Mix_Volume(-1, C.int(C.MIX_MAX_VOLUME*sys.wavVolume/100))
	C.Mix_AllocateChannels(C.int(sys.wavChannels))
	return err
}

func AudioClose() {
	C.Mix_HaltChannel(-1) // Stop all channels
	C.Mix_HaltMusic()     // Stop music if playing
	C.Mix_CloseAudio()    // Close audio device
	C.Mix_Quit()          // Quit SDL_mixer
}
