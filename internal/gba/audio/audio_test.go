package audio

import (
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
	if a.ControlHigh() != 0 || a.FIFOLevel(FIFOA) != 0 || a.FIFOLevel(FIFOB) != 0 ||
		a.CurrentSample(FIFOA) != 0 || a.CurrentSample(FIFOB) != 0 {
		t.Fatalf("reset state control=%04x levels=%d/%d samples=%d/%d",
			a.ControlHigh(), a.FIFOLevel(FIFOA), a.FIFOLevel(FIFOB),
			a.CurrentSample(FIFOA), a.CurrentSample(FIFOB))
	}
}
