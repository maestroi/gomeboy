package gomeboy

import "testing"

// Run these directly with:
// go test ./pkg/gomeboy -run '^$' -bench '^BenchmarkPerf' -benchmem
//
// perfROM returns a minimal 32 KiB ROM that spins at 0x0100. It is intentionally
// self-contained so performance benchmarks do not depend on gitignored test ROMs.
func perfROM() []byte {
	rom := make([]byte, 32*1024)
	// JP 0x0100
	rom[0x0100] = 0xC3
	rom[0x0101] = 0x00
	rom[0x0102] = 0x01
	copy(rom[0x0134:0x0144], []byte("GOMEBENCH"))
	rom[0x0147] = 0x00 // ROM only
	rom[0x0148] = 0x00 // 32 KiB
	rom[0x0149] = 0x00 // no RAM
	return rom
}

func newPerfEmulatorWithVideo(b *testing.B, headless, video bool) *Emulator {
	b.Helper()
	opts := []Option{WithROMBytes(perfROM())}
	if headless {
		opts = append(opts, Headless())
	}
	if !video {
		opts = append(opts, WithoutVideo())
	}
	e, err := New(opts...)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	for i := 0; i < 10; i++ {
		e.StepFrame()
	}
	return e
}

func BenchmarkPerfStepFrameHeadless(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

func BenchmarkPerfStepFrameAudio(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, false, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

func BenchmarkPerfStepFrames60Headless(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrames(60)
	}
}

func BenchmarkPerfPeekInto4K(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, true)
	defer e.Close()
	dst := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.PeekInto(0xC000, dst)
	}
}

func BenchmarkPerfStepFrameHeadlessNoVideo(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

func BenchmarkPerfStepFrames60HeadlessNoVideo(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrames(60)
	}
}

func BenchmarkPerfCheckpointRoundTrip(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
	defer e.Close()
	var cp Checkpoint
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.CheckpointInto(&cp)
		e.RestoreCheckpoint(&cp)
	}
}

func BenchmarkPerfSaveStateRoundTrip(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state, err := e.SaveState()
		if err != nil {
			b.Fatal(err)
		}
		if err := e.LoadState(state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPerfReadInto4K(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
	defer e.Close()
	dst := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.ReadInto(0xC000, dst)
	}
}
