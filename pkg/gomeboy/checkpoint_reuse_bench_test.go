package gomeboy

import "testing"

func BenchmarkCheckpointReusableCapture(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
	defer e.Close()
	var cp Checkpoint
	e.CheckpointInto(&cp)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.CheckpointInto(&cp)
	}
}

func BenchmarkCheckpointReusableRestore(b *testing.B) {
	e := newPerfEmulatorWithVideo(b, true, false)
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
