package audio

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func enablePSG(t *testing.T, a *Audio, b *bus.Bus) {
	t.Helper()
	b.Write16(bus.IOStart+soundControlXOffset, 0x0080, bus.Access{})
	if got := a.soundControlX(); got&0x80 == 0 {
		t.Fatalf("SOUNDCNT_X master enable = %04x", got)
	}
}

func TestPSGRegistersMaskAndMasterDisable(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	b.Write16(bus.IOStart+sound1ControlLowOffset, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlHighOffset, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlXOffset, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+soundControlLowOffset, 0xffff, bus.Access{})

	if got, _ := b.Read16(bus.IOStart+sound1ControlLowOffset, bus.Access{}); got != 0x007f {
		t.Fatalf("SOUND1CNT_L = %04x, want 007f", got)
	}
	if got, _ := b.Read16(bus.IOStart+sound1ControlHighOffset, bus.Access{}); got != 0xffc0 {
		t.Fatalf("SOUND1CNT_H = %04x, want ffc0", got)
	}
	if got, _ := b.Read16(bus.IOStart+sound1ControlXOffset, bus.Access{}); got != 0x4000 {
		t.Fatalf("SOUND1CNT_X = %04x, want 4000", got)
	}
	if got, _ := b.Read16(bus.IOStart+soundControlLowOffset, bus.Access{}); got != 0xff77 {
		t.Fatalf("SOUNDCNT_L = %04x, want ff77", got)
	}
	if got := a.soundControlX(); got&0x81 != 0x81 {
		t.Fatalf("SOUNDCNT_X status = %04x, want master+channel1", got)
	}

	b.Write16(bus.IOStart+soundControlXOffset, 0, bus.Access{})
	if got := a.soundControlX(); got != 0 {
		t.Fatalf("SOUNDCNT_X after disable = %04x, want 0", got)
	}
	if got, _ := b.Read16(bus.IOStart+sound1ControlHighOffset, bus.Access{}); got != 0 {
		t.Fatalf("SOUND1CNT_H survived master disable: %04x", got)
	}
	if got, _ := b.Read16(bus.IOStart+soundControlLowOffset, bus.Access{}); got != 0 {
		t.Fatalf("SOUNDCNT_L survived master disable: %04x", got)
	}
}

func TestSquarePSGMixesThroughRoutingAndRatio(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	// PSG ratio=100%. Route channel 1 to both sides at NR50 volume 7.
	b.Write16(bus.IOStart+soundControlHighOffset, 0x0002, bus.Access{})
	b.Write16(bus.IOStart+soundControlLowOffset, 0x1177, bus.Access{})
	// Duty 1 starts high; envelope starts at volume 15 with DAC enabled.
	b.Write16(bus.IOStart+sound1ControlHighOffset, 0xf040, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlXOffset, 0x8000, bus.Access{})

	left, right := a.CurrentOutput()
	const want = float32(240.0 / 512.0)
	if left != want || right != want {
		t.Fatalf("square PSG mix = %f/%f, want %f/%f", left, right, want, want)
	}
}

func TestPSGEnvelopeAndLengthFrameSequencer(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	// Initial volume 1, increase, period 1.
	b.Write16(bus.IOStart+sound1ControlHighOffset, 0x1940, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlXOffset, 0x8000, bus.Access{})
	if got := a.psg.square[0].envelope.volume; got != 1 {
		t.Fatalf("trigger envelope volume = %d, want 1", got)
	}

	a.SetHeadless(true)
	a.Advance(uint32(psgFramePeriod * 7))
	if got := a.psg.square[0].envelope.volume; got != 2 {
		t.Fatalf("frame envelope volume = %d, want 2", got)
	}

	// Re-trigger with length=1 and length-enable. Length clocks on frame 0/2/4/6.
	b.Write16(bus.IOStart+sound1ControlHighOffset, 0x19ff, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlXOffset, 0xc000, bus.Access{})
	if !a.psg.square[0].enabled {
		t.Fatal("channel 1 did not enable on trigger")
	}
	// Current frame is 7; the next frame is 0, which clocks length.
	a.Advance(uint32(psgFramePeriod))
	if a.psg.square[0].enabled {
		t.Fatal("channel 1 length expiry did not disable channel")
	}
}

func TestPSGSweepOverflowDisablesChannel(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	// Sweep +50% from frequency 2047 overflows immediately on trigger.
	b.Write16(bus.IOStart+sound1ControlLowOffset, 0x0011, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlHighOffset, 0xf040, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlXOffset, 0x87ff, bus.Access{})
	if a.psg.square[0].enabled {
		t.Fatal("sweep overflow left channel 1 enabled")
	}
	if got := a.soundControlX(); got&1 != 0 {
		t.Fatalf("SOUNDCNT_X channel 1 status remained set: %04x", got)
	}
}

func TestWavePSGUsesBankedWaveRAMAndGBA75PercentVolume(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	// Select bank 1 so CPU accesses the opposite bank 0, then seed sample 0.
	b.Write16(bus.IOStart+sound3ControlLowOffset, 0x00c0, bus.Access{})
	b.Write8(bus.IOStart+waveRAMOffset, 0xf0, bus.Access{})
	// Play bank 0, DAC on. Volume code bit 2 is GBA's 75% mode.
	b.Write16(bus.IOStart+sound3ControlLowOffset, 0x0080, bus.Access{})
	b.Write16(bus.IOStart+sound3ControlHighOffset, 0x8000, bus.Access{})
	b.Write16(bus.IOStart+sound3ControlXOffset, 0x8000, bus.Access{})
	if got := a.psg.wave.sample(); got != 11 {
		t.Fatalf("wave 75%% sample = %d, want 11", got)
	}
	if got := a.soundControlX(); got&(1<<2) == 0 {
		t.Fatalf("SOUNDCNT_X wave status not set: %04x", got)
	}
}

func TestNoisePSGTriggerAndLengthStatus(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	// Length=1, max starting envelope volume, short noise period.
	b.Write16(bus.IOStart+sound4ControlLowOffset, 0xf03f, bus.Access{})
	b.Write16(bus.IOStart+sound4ControlHighOffset, 0xc000, bus.Access{})
	if got := a.soundControlX(); got&(1<<3) == 0 {
		t.Fatalf("SOUNDCNT_X noise status not set: %04x", got)
	}
	if a.psg.noise.lfsr != 0x7fff {
		t.Fatalf("noise trigger LFSR = %04x, want 7fff", a.psg.noise.lfsr)
	}

	a.SetHeadless(true)
	a.Advance(uint32(psgFramePeriod * 2))
	if a.psg.noise.enabled {
		t.Fatal("noise length expiry did not disable channel")
	}
}

func TestPSGAndDirectSoundShareMixer(t *testing.T) {
	a, b, _ := newTestAudio(t)

	// Direct Sound A: +64, full volume, left only.
	b.Write8(bus.IOStart+fifoAOffset, 0x40, bus.Access{})
	b.Write16(bus.IOStart+soundControlHighOffset,
		0x0002|controlVolumeA|controlALeft|controlBTimer, bus.Access{})
	a.TimerOverflow(0, 1)

	enablePSG(t, a, b)
	// Square 1 contributes to the right only.
	b.Write16(bus.IOStart+soundControlLowOffset, 0x0170, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlHighOffset, 0xf040, bus.Access{})
	b.Write16(bus.IOStart+sound1ControlXOffset, 0x8000, bus.Access{})

	left, right := a.CurrentOutput()
	if left != 0.5 {
		t.Fatalf("combined left = %f, want Direct Sound 0.5", left)
	}
	if right <= 0 {
		t.Fatalf("combined right = %f, want positive PSG output", right)
	}
}


func TestPSGFrequencyByteWritesPreserveWriteOnlyLatch(t *testing.T) {
	a, b, _ := newTestAudio(t)
	enablePSG(t, a, b)

	// NR13/NR14 are split across byte lanes even though frequency bits read as 0.
	b.Write8(bus.IOStart+sound1ControlXOffset, 0xaa, bus.Access{})
	b.Write8(bus.IOStart+sound1ControlXOffset+1, 0x87, bus.Access{})
	if got := a.psg.square[0].frequency; got != 0x7aa {
		t.Fatalf("channel 1 frequency = %03x, want 7aa", got)
	}
	if got := a.psg.raw[2] & 0x8000; got != 0 {
		t.Fatalf("trigger bit remained latched in raw register: %04x", a.psg.raw[2])
	}

	// A later low-byte write must not retrigger the channel from the old trigger bit.
	a.psg.square[0].position = 3
	b.Write8(bus.IOStart+sound1ControlXOffset, 0x55, bus.Access{})
	if got := a.psg.square[0].position; got != 3 {
		t.Fatalf("write-only trigger was replayed by low-byte write: position=%d", got)
	}
	if got := a.psg.square[0].frequency; got != 0x755 {
		t.Fatalf("channel 1 frequency after low-byte update = %03x, want 755", got)
	}
}
