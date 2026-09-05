package apu

import "testing"

func TestSamplesShrinksBurstBufferAfterDrain(t *testing.T) {
	a := &APU{buffer: make([]float32, bufferSize)}

	// Turbo/fast-forward can produce a burst larger than the normal audio
	// buffer before the consumer drains it. The buffer may grow for that burst,
	// but every drain must restore the baseline length so later bursts cannot
	// compound onto an already-grown slice.
	for burst := 0; burst < 8; burst++ {
		a.buffer = append(a.buffer, make([]float32, bufferSize/2)...)
		a.bufferPos = uint32(len(a.buffer))
		burstLen := len(a.buffer)

		samples, count := a.Samples()
		if int(count) != burstLen {
			t.Fatalf("burst %d: Samples count = %d, want %d", burst, count, burstLen)
		}
		if len(samples) != burstLen {
			t.Fatalf("burst %d: returned sample length = %d, want %d", burst, len(samples), burstLen)
		}
		if a.bufferPos != 0 {
			t.Fatalf("burst %d: bufferPos = %d after drain, want 0", burst, a.bufferPos)
		}
		if len(a.buffer) != bufferSize {
			t.Fatalf("burst %d: backing buffer length = %d after drain, want baseline %d", burst, len(a.buffer), bufferSize)
		}
	}
}
