package audio

import (
	"reflect"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

type requestLog struct {
	fifos []FIFO
}

func (r *requestLog) request(fifo FIFO) {
	r.fifos = append(r.fifos, fifo)
}

func newTestAudio(t *testing.T) (*Audio, *bus.Bus, *requestLog) {
	t.Helper()
	b := bus.New(nil, nil)
	requests := &requestLog{}
	a := New(b, Hooks{RequestFIFO: requests.request})
	return a, b, requests
}


func TestHeadlessAdvanceBatchesHostSamplingWithoutChangingPSGState(t *testing.T) {
	big, _, _ := newTestAudio(t)
	chunked, _, _ := newTestAudio(t)

	for _, a := range []*Audio{big, chunked} {
		a.SetHeadless(true)
		a.psg.enabled = true
		a.psg.square[0] = squarePSG{
			enabled:   true,
			frequency: 1980,
			envelope:  psgEnvelope{dac: true, volume: 7},
		}
	}

	const total = uint32(250000)
	big.Advance(total)
	for remaining := total; remaining != 0; {
		step := uint32(997)
		if remaining < step {
			step = remaining
		}
		chunked.Advance(step)
		remaining -= step
	}

	if big.samplePhase != chunked.samplePhase {
		t.Fatalf("headless sample phase = %d, want %d", big.samplePhase, chunked.samplePhase)
	}
	if !reflect.DeepEqual(big.psg, chunked.psg) {
		t.Fatalf("batched headless PSG state differs:\nbig=%+v\nchunked=%+v", big.psg, chunked.psg)
	}
	if _, count := big.Samples(); count != 0 {
		t.Fatalf("headless batched advance buffered %d values", count)
	}
}

func TestFIFORegisterWritesPreserveLittleEndianSampleOrder(t *testing.T) {
	a, b, _ := newTestAudio(t)

	b.Write32(bus.IOStart+fifoAOffset, 0x807f01ff, bus.Access{})
	if got := a.FIFOLevel(FIFOA); got != 4 {
		t.Fatalf("FIFO A level = %d, want 4", got)
	}

	want := []int8{-1, 1, 127, -128}
	for i, sample := range want {
		a.TimerOverflow(0, 1)
		if got := a.CurrentSample(FIFOA); got != sample {
			t.Fatalf("sample %d = %d, want %d", i, got, sample)
		}
	}
	if got := a.FIFOLevel(FIFOA); got != 0 {
		t.Fatalf("FIFO A level after consumption = %d, want 0", got)
	}
}

func TestFIFOSupportsByteHalfwordAndWordWrites(t *testing.T) {
	a, b, _ := newTestAudio(t)

	b.Write8(bus.IOStart+fifoAOffset, 0x11, bus.Access{})
	b.Write16(bus.IOStart+fifoAOffset+2, 0x3322, bus.Access{})
	b.Write32(bus.IOStart+fifoAOffset, 0x77665544, bus.Access{})

	if got := a.FIFOLevel(FIFOA); got != 7 {
		t.Fatalf("FIFO A level = %d, want 7", got)
	}
	for i, want := range []int8{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77} {
		a.TimerOverflow(0, 1)
		if got := a.CurrentSample(FIFOA); got != want {
			t.Fatalf("sample %d = %02x, want %02x", i, byte(got), byte(want))
		}
	}
}

func TestSoundControlHighMasksBitsSelectsTimersAndResetsFIFOs(t *testing.T) {
	a, b, _ := newTestAudio(t)

	b.Write32(bus.IOStart+fifoAOffset, 0x04030201, bus.Access{})
	b.Write32(bus.IOStart+fifoBOffset, 0x08070605, bus.Access{})

	b.Write16(bus.IOStart+soundControlHighOffset, 0xffff, bus.Access{})
	if got := a.ControlHigh(); got != controlStoredMask {
		t.Fatalf("SOUNDCNT_H = %04x, want %04x", got, controlStoredMask)
	}
	if a.FIFOLevel(FIFOA) != 0 || a.FIFOLevel(FIFOB) != 0 {
		t.Fatalf("FIFO reset did not clear levels A=%d B=%d",
			a.FIFOLevel(FIFOA), a.FIFOLevel(FIFOB))
	}
	if got := a.Timer(FIFOA); got != 1 {
		t.Fatalf("FIFO A timer = %d, want 1", got)
	}
	if got := a.Timer(FIFOB); got != 1 {
		t.Fatalf("FIFO B timer = %d, want 1", got)
	}

	// Reset bits are write-only strobes and must read back clear.
	if got, _ := b.Read16(bus.IOStart+soundControlHighOffset, bus.Access{}); got != controlStoredMask {
		t.Fatalf("mapped SOUNDCNT_H = %04x, want %04x", got, controlStoredMask)
	}
}

func TestTimerSelectionConsumesOnlyMatchingFIFO(t *testing.T) {
	a, b, _ := newTestAudio(t)
	b.Write32(bus.IOStart+fifoAOffset, 0x04030201, bus.Access{})
	b.Write32(bus.IOStart+fifoBOffset, 0x08070605, bus.Access{})

	// FIFO A uses timer 0, FIFO B uses timer 1.
	b.Write16(bus.IOStart+soundControlHighOffset, controlBTimer, bus.Access{})

	a.TimerOverflow(0, 1)
	if got := a.FIFOLevel(FIFOA); got != 3 {
		t.Fatalf("timer 0 FIFO A level = %d, want 3", got)
	}
	if got := a.FIFOLevel(FIFOB); got != 4 {
		t.Fatalf("timer 0 touched FIFO B: level=%d", got)
	}

	a.TimerOverflow(1, 1)
	if got := a.FIFOLevel(FIFOA); got != 3 {
		t.Fatalf("timer 1 touched FIFO A: level=%d", got)
	}
	if got := a.FIFOLevel(FIFOB); got != 3 {
		t.Fatalf("timer 1 FIFO B level = %d, want 3", got)
	}
}

func TestFIFORequestsDMAAtHalfFullThreshold(t *testing.T) {
	a, b, requests := newTestAudio(t)
	// Keep the empty FIFO B on timer 1 so timer-0 requests isolate FIFO A.
	b.Write16(bus.IOStart+soundControlHighOffset, controlBTimer, bus.Access{})

	for i := 0; i < 4; i++ {
		b.Write32(bus.IOStart+fifoAOffset, uint32(i+1)*0x01010101, bus.Access{})
	}
	b.Write16(bus.IOStart+fifoAOffset, 0x1211, bus.Access{})
	if got := a.FIFOLevel(FIFOA); got != 18 {
		t.Fatalf("FIFO A setup level = %d, want 18", got)
	}

	a.TimerOverflow(0, 1)
	if got := a.FIFOLevel(FIFOA); got != 17 {
		t.Fatalf("FIFO A level after first sample = %d, want 17", got)
	}
	if len(requests.fifos) != 0 {
		t.Fatalf("DMA requested above threshold: %v", requests.fifos)
	}

	a.TimerOverflow(0, 1)
	if got := a.FIFOLevel(FIFOA); got != fifoDMAThreshold {
		t.Fatalf("FIFO A threshold level = %d, want %d", got, fifoDMAThreshold)
	}
	if len(requests.fifos) != 1 || requests.fifos[0] != FIFOA {
		t.Fatalf("threshold request = %v, want [FIFOA]", requests.fifos)
	}
}

func TestFIFOIsBoundedAndUnderflowOutputsSilence(t *testing.T) {
	a, b, _ := newTestAudio(t)

	for i := 0; i < 9; i++ {
		b.Write32(bus.IOStart+fifoBOffset, uint32(i+1)*0x01010101, bus.Access{})
	}
	if got := a.FIFOLevel(FIFOB); got != fifoCapacity {
		t.Fatalf("FIFO B overfill level = %d, want %d", got, fifoCapacity)
	}

	b.Write16(bus.IOStart+soundControlHighOffset, controlBTimer, bus.Access{})
	a.TimerOverflow(1, fifoCapacity)
	if got := a.FIFOLevel(FIFOB); got != 0 {
		t.Fatalf("FIFO B drain level = %d, want 0", got)
	}
	a.TimerOverflow(1, 1)
	if got := a.CurrentSample(FIFOB); got != 0 {
		t.Fatalf("FIFO B underflow sample = %d, want silence", got)
	}
}

func TestResetClearsDirectSoundState(t *testing.T) {
	a, b, _ := newTestAudio(t)
	b.Write16(bus.IOStart+soundControlHighOffset, controlALeft|controlBTimer, bus.Access{})
	b.Write32(bus.IOStart+fifoAOffset, 0x04030201, bus.Access{})
	a.TimerOverflow(0, 1)

	a.Reset()
	if a.ControlHigh() != 0 || a.SoundBias() != defaultSoundBias ||
		a.FIFOLevel(FIFOA) != 0 || a.FIFOLevel(FIFOB) != 0 ||
		a.CurrentSample(FIFOA) != 0 || a.CurrentSample(FIFOB) != 0 {
		t.Fatalf("reset state control=%04x bias=%04x levels=%d/%d samples=%d/%d",
			a.ControlHigh(), a.SoundBias(), a.FIFOLevel(FIFOA), a.FIFOLevel(FIFOB),
			a.CurrentSample(FIFOA), a.CurrentSample(FIFOB))
	}
}


func TestDirectSoundMixerAppliesRoutingAndVolume(t *testing.T) {
	a, b, _ := newTestAudio(t)
	b.Write16(bus.IOStart+soundBiasOffset, defaultSoundBias, bus.Access{})
	b.Write8(bus.IOStart+fifoAOffset, 0x40, bus.Access{})
	b.Write8(bus.IOStart+fifoBOffset, 0xe0, bus.Access{})
	// A: full volume to left only. B: half volume to right only and timer 1.
	b.Write16(bus.IOStart+soundControlHighOffset,
		controlVolumeA|controlALeft|controlBRight|controlBTimer, bus.Access{})

	a.TimerOverflow(0, 1)
	a.TimerOverflow(1, 1)
	left, right := a.CurrentOutput()
	if left != 0.5 {
		t.Fatalf("left mix = %f, want 0.5", left)
	}
	if right != -0.125 {
		t.Fatalf("right mix = %f, want -0.125", right)
	}
}

func TestDirectSoundMixerClipsThroughSoundBias(t *testing.T) {
	a, b, _ := newTestAudio(t)
	b.Write16(bus.IOStart+soundBiasOffset, defaultSoundBias, bus.Access{})
	b.Write8(bus.IOStart+fifoAOffset, 0x7f, bus.Access{})
	b.Write8(bus.IOStart+fifoBOffset, 0x7f, bus.Access{})
	b.Write16(bus.IOStart+soundControlHighOffset,
		controlVolumeA|controlVolumeB|controlALeft|controlBLeft|controlBTimer, bus.Access{})
	a.TimerOverflow(0, 1)
	a.TimerOverflow(1, 1)

	left, right := a.CurrentOutput()
	want := float32(511.0 / 512.0)
	if left != want || right != 0 {
		t.Fatalf("clipped mix = %f/%f, want %f/0", left, right, want)
	}

	// SOUNDBIAS exposes only bits 1-9 and 14-15.
	b.Write16(bus.IOStart+soundBiasOffset, 0xffff, bus.Access{})
	if got := a.SoundBias(); got != soundBiasMask {
		t.Fatalf("SOUNDBIAS = %04x, want %04x", got, soundBiasMask)
	}
}

func TestAdvanceProducesInterleavedHostRateSamples(t *testing.T) {
	a, b, _ := newTestAudio(t)
	b.Write8(bus.IOStart+fifoAOffset, 0x40, bus.Access{})
	b.Write16(bus.IOStart+soundControlHighOffset,
		controlVolumeA|controlALeft|controlARight|controlBTimer, bus.Access{})
	a.TimerOverflow(0, 1)

	a.Advance(174)
	if samples, count := a.Samples(); count != 0 || len(samples) != 0 {
		t.Fatalf("174 cycles produced %d values, want 0", count)
	}
	a.Advance(1)
	samples, count := a.Samples()
	if count != 2 || len(samples) != 2 {
		t.Fatalf("175 cycles produced %d values, want one stereo frame", count)
	}
	if samples[0] != 0.5 || samples[1] != 0.5 {
		t.Fatalf("stereo sample = %v, want [0.5 0.5]", samples)
	}
}

func TestHeadlessAndMuteAffectOnlyHostOutput(t *testing.T) {
	a, b, _ := newTestAudio(t)
	b.Write8(bus.IOStart+fifoAOffset, 0x40, bus.Access{})
	b.Write16(bus.IOStart+soundControlHighOffset,
		controlVolumeA|controlALeft|controlARight|controlBTimer, bus.Access{})
	a.TimerOverflow(0, 1)

	a.SetHeadless(true)
	a.Advance(350)
	if _, count := a.Samples(); count != 0 {
		t.Fatalf("headless audio buffered %d values", count)
	}
	if got := a.CurrentSample(FIFOA); got != 0x40 {
		t.Fatalf("headless mode changed Direct Sound latch: %d", got)
	}

	a.SetHeadless(false)
	a.SetMute(true)
	a.Advance(175)
	samples, count := a.Samples()
	if count == 0 {
		t.Fatal("muted output did not keep host sample cadence")
	}
	for i, sample := range samples {
		if sample != 0 {
			t.Fatalf("muted sample %d = %f, want 0", i, sample)
		}
	}
}
