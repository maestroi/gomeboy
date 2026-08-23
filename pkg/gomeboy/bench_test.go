package gomeboy

import (
	"testing"
)

func newBenchEmulator(b *testing.B) *Emulator {
	b.Helper()
	e, err := New(WithROM(testROM), Headless())
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	// warm up so the first frame's one-time costs are not measured
	for i := 0; i < 10; i++ {
		e.StepFrame()
	}
	return e
}

// BenchmarkStepFrame measures the cost of advancing the emulator by one frame.
func BenchmarkStepFrame(b *testing.B) {
	e := newBenchEmulator(b)
	defer e.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

// BenchmarkFrame measures the cost of retrieving the rendered frame.
func BenchmarkFrame(b *testing.B) {
	e := newBenchEmulator(b)
	defer e.Close()
	e.StepFrame()
	b.ResetTimer()
	var n int
	for i := 0; i < b.N; i++ {
		f := e.Frame()
		n += len(f.RGB)
	}
	_ = n
}

// BenchmarkSaveState measures the cost of serializing a full save state.
func BenchmarkSaveState(b *testing.B) {
	e := newBenchEmulator(b)
	defer e.Close()
	b.ResetTimer()
	var n int
	for i := 0; i < b.N; i++ {
		state, err := e.SaveState()
		if err != nil {
			b.Fatalf("SaveState: %v", err)
		}
		n += len(state)
	}
	_ = n
}

// BenchmarkLoadState measures the cost of restoring a full save state.
func BenchmarkLoadState(b *testing.B) {
	e := newBenchEmulator(b)
	defer e.Close()
	state, err := e.SaveState()
	if err != nil {
		b.Fatalf("SaveState: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := e.LoadState(state); err != nil {
			b.Fatalf("LoadState: %v", err)
		}
	}
}

// BenchmarkRead8 measures the cost of a single memory read.
func BenchmarkRead8(b *testing.B) {
	e := newBenchEmulator(b)
	defer e.Close()
	b.ResetTimer()
	var n byte
	for i := 0; i < b.N; i++ {
		n = e.Read8(0xC000)
	}
	_ = n
}
