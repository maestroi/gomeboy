package gomeboy

import "testing"

func newAgentPerfEmulator(b *testing.B, noVideo bool) *Emulator {
	b.Helper()
	opts := []Option{WithROMBytes(perfROM()), Headless()}
	if noVideo {
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

func BenchmarkPerfStepFrameHeadlessVideo(b *testing.B) {
	e := newAgentPerfEmulator(b, false)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

func BenchmarkPerfStepFrameHeadlessNoVideo(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

func BenchmarkPerfStepFrames60HeadlessVideo(b *testing.B) {
	e := newAgentPerfEmulator(b, false)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrames(60)
	}
}

func BenchmarkPerfStepFrames60HeadlessNoVideo(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrames(60)
	}
}

func BenchmarkPerfCheckpointInto(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	var cp Checkpoint
	e.CheckpointInto(&cp)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.CheckpointInto(&cp)
	}
}

func BenchmarkPerfSaveState(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := e.SaveState(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPerfRestoreCheckpoint(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	var cp Checkpoint
	e.CheckpointInto(&cp)
	if err := e.RestoreCheckpoint(&cp); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := e.RestoreCheckpoint(&cp); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPerfLoadState(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	state, err := e.SaveState()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := e.LoadState(state); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPerfReadInto4K(b *testing.B) {
	e := newAgentPerfEmulator(b, true)
	defer e.Close()
	dst := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.ReadInto(0xC000, dst)
	}
}
