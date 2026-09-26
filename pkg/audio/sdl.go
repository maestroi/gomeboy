//go:build !test

package audio

// typedef unsigned char Uint8;
// void AudioData(void *userdata, Uint8 *stream, int len);
import "C"

import (
	"time"
	"unsafe"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/pkg/gomeboy"
	"github.com/veandco/go-sdl2/sdl"
)

type audioVideoSource interface {
	Paused() bool
	Initialised() bool
	Speed() int
	StepFrames(int)
	Samples() ([]float32, uint32)
	FrameRGB() []byte
}

type gameBoySource struct{ gb *gameboy.GameBoy }

func (s gameBoySource) Paused() bool      { return s.gb.Paused() }
func (s gameBoySource) Initialised() bool { return s.gb.Initialised() }
func (s gameBoySource) Speed() int        { return s.gb.Speed() }
func (s gameBoySource) StepFrames(n int)  { s.gb.StepFrames(n) }
func (s gameBoySource) Samples() ([]float32, uint32) {
	return s.gb.APU.Samples()
}
func (s gameBoySource) FrameRGB() []byte {
	fb := s.gb.FrameBuffer()
	return unsafe.Slice(&(*fb)[0][0][0], 160*144*3)
}

type emulatorSource struct{ e *gomeboy.Emulator }

func (s emulatorSource) Paused() bool      { return s.e.Paused() }
func (s emulatorSource) Initialised() bool { return s.e.Initialised() }
func (s emulatorSource) Speed() int        { return s.e.Speed() }
func (s emulatorSource) StepFrames(n int)  { s.e.StepFrames(n) }
func (s emulatorSource) Samples() ([]float32, uint32) {
	return s.e.Samples()
}
func (s emulatorSource) FrameRGB() []byte { return s.e.Frame().RGB }

var (
	sampleBuffer []byte
	source       audioVideoSource
	frameBuffer  chan []byte
	tempFrame    []byte
	audioDeviceID sdl.AudioDeviceID
)

//export AudioData
func AudioData(userdata unsafe.Pointer, stream *C.Uint8, length C.int) {
	n := int(length)
	data := unsafe.Slice((*byte)(unsafe.Pointer(stream)), n)
	clear(data)

	if len(sampleBuffer) >= n {
		copy(data, sampleBuffer[:n])
		sampleBuffer = sampleBuffer[n:]
		return
	}
	if source == nil || source.Paused() || !source.Initialised() {
		return
	}

	speed := source.Speed()
	source.StepFrames(speed)

	if speed > 1 {
		// Turbo preserves hardware audio state while muting host output.
		source.Samples()
		publishFrame()
		return
	}

	samples, count := source.Samples()
	if count > 0 && len(samples) > 0 {
		byteCount := int(count) * 4
		if byteCount > len(samples)*4 {
			byteCount = len(samples) * 4
		}
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(&samples[0])), byteCount)
		combined := append(sampleBuffer, bytes...)
		copied := copy(data, combined)
		if copied < len(combined) {
			sampleBuffer = append(sampleBuffer[:0], combined[copied:]...)
		} else {
			sampleBuffer = sampleBuffer[:0]
		}
	}

	publishFrame()
}

func publishFrame() {
	if source == nil || frameBuffer == nil {
		return
	}
	frame := source.FrameRGB()
	if cap(tempFrame) < len(frame) {
		tempFrame = make([]byte, len(frame))
	} else {
		tempFrame = tempFrame[:len(frame)]
	}
	copy(tempFrame, frame)
	frameBuffer <- tempFrame
}

// OpenAudio retains the legacy internal Game Boy entry point.
func OpenAudio(g *gameboy.GameBoy, fb chan []byte) error {
	if g == nil {
		return nil
	}
	return openSource(gameBoySource{gb: g}, fb)
}

// OpenEmulator opens audio for the core-neutral public emulator, including GBA.
func OpenEmulator(e *gomeboy.Emulator, fb chan []byte) error {
	if e == nil {
		return nil
	}
	return openSource(emulatorSource{e: e}, fb)
}

func openSource(s audioVideoSource, fb chan []byte) error {
	if err := sdl.AudioInit("pulsewire"); err != nil {
		if err := sdl.InitSubSystem(sdl.INIT_AUDIO); err != nil {
			return err
		}
	}
	source = s
	frameBuffer = fb
	sampleBuffer = sampleBuffer[:0]

	var err error
	if audioDeviceID, err = sdl.OpenAudioDevice("", false, &sdl.AudioSpec{
		Freq:     sampleRate,
		Format:   sdl.AUDIO_F32,
		Channels: 2,
		Samples:  bufferSize,
		Callback: sdl.AudioCallback(C.AudioData),
	}, nil, 0); err != nil {
		// Keep emulation advancing even when the host has no audio device.
		go func() {
			dummyBuffer := make([]C.Uint8, bufferSize*4)
			ticker := time.NewTicker(time.Second / time.Duration(sampleRate/bufferSize))
			defer ticker.Stop()
			for range ticker.C {
				if source != nil {
					AudioData(nil, (*C.Uint8)(unsafe.Pointer(&dummyBuffer[0])), C.int(len(dummyBuffer)))
				}
			}
		}()
		return err
	}

	sdl.PauseAudioDevice(audioDeviceID, false)
	return nil
}

const (
	bufferSize = 1634
	sampleRate = 96000
)
