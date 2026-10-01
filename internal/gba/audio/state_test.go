package audio

import (
	"reflect"
	"testing"
)

func TestStateRoundTripPreservesHardwareAndReceiverHostPolicy(t *testing.T) {
	source := &Audio{controlHigh: controlARight | controlALeft | controlATimer, soundBias: 0x0240, samplePhase: 12345}
	source.channel[0].fifo = byteFIFO{data: [fifoCapacity]byte{0x10, 0x20, 0x30}, head: 1, size: 2}
	source.channel[0].current = -12
	source.psg.enabled = true
	source.psg.frame = 5
	source.psg.frameCycles = 123
	source.psg.square[0] = squarePSG{
		enabled: true, duty: 2, position: 4, frequency: 1024, phaseCycles: 77,
		length: 31, lengthEnable: true,
		envelope: psgEnvelope{initial: 12, volume: 9, period: 3, timer: 2, increase: true, dac: true},
		sweepPeriod: 4, sweepShift: 2, sweepTimer: 1, sweepNegate: true, sweepShadow: 900, sweepEnable: true,
	}
	source.psg.wave.enabled = true
	source.psg.wave.ram[7] = 0xab
	source.psg.noise = noisePSG{enabled: true, lfsr: 0x4321, phaseCycles: 99}

	want := source.Snapshot()

	target := &Audio{buffer: make([]float32, outputBufferSize), headless: true}
	target.mute.Store(true)
	target.bufferPos = 10
	target.Restore(want)

	if got := target.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("audio hardware state after restore = %#v, want %#v", got, want)
	}
	if !target.headless || !target.mute.Load() {
		t.Fatal("restore changed receiver-owned headless/mute policy")
	}
	if target.bufferPos != 0 {
		t.Fatalf("restore retained stale host samples: bufferPos=%d", target.bufferPos)
	}
}
