package gomeboy

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestSessionRecordingRoundTripAndReplay(t *testing.T) {
	source, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	source.StepFrames(2)
	recorder, err := source.StartSessionRecording(RecordingOptions{Metadata: map[string]string{
		"run_id": "test-run",
		"agent":  "unit-test",
	}})
	if err != nil {
		t.Fatal(err)
	}

	source.Press(ButtonA)
	source.StepFrame()
	source.Release(ButtonA)
	source.StepFrames(2)
	// Preserve a transition made on the final frame even though no more frame is stepped.
	source.Press(ButtonB)

	recording, err := recorder.Stop()
	if err != nil {
		t.Fatal(err)
	}
	wantHash, err := source.StateHashHex()
	if err != nil {
		t.Fatal(err)
	}
	if recording.StartFrame != 2 || recording.EndFrame != 5 || recording.DurationFrames() != 3 {
		t.Fatalf("recording frames = %d..%d duration=%d, want 2..5 duration=3", recording.StartFrame, recording.EndFrame, recording.DurationFrames())
	}
	if recording.FinalStateHash != wantHash {
		t.Fatalf("final hash = %s, want %s", recording.FinalStateHash, wantHash)
	}
	if len(recording.Inputs) != 3 || recording.Inputs[2].Frame != recording.EndFrame || !recording.Inputs[2].Pressed {
		t.Fatalf("unexpected inputs: %+v", recording.Inputs)
	}

	archive, err := recording.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseRecording(archive)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Metadata["run_id"] != "test-run" || parsed.Metadata["agent"] != "unit-test" {
		t.Fatalf("metadata lost in round trip: %+v", parsed.Metadata)
	}
	if !bytes.Equal(parsed.StartStateChecked(), recording.StartStateChecked()) {
		t.Fatal("checked start state changed during archive round trip")
	}

	replay, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if err := replay.ReplayRecording(parsed); err != nil {
		t.Fatal(err)
	}
	gotHash, err := replay.StateHashHex()
	if err != nil {
		t.Fatal(err)
	}
	if gotHash != wantHash {
		t.Fatalf("replay hash = %s, want %s", gotHash, wantHash)
	}
}

func TestReplayRecordingFramesVisitsInitialAndSteppedFrames(t *testing.T) {
	source, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	recorder, err := source.StartSessionRecording(RecordingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source.StepFrames(3)
	recording, err := recorder.Stop()
	if err != nil {
		t.Fatal(err)
	}

	replay, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()

	var frames []uint64
	if err := replay.ReplayRecordingFrames(recording, func(frame uint64, _ Frame) error {
		frames = append(frames, frame)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []uint64{0, 1, 2, 3}
	if len(frames) != len(want) {
		t.Fatalf("callback frames = %v, want %v", frames, want)
	}
	for i := range want {
		if frames[i] != want[i] {
			t.Fatalf("callback frames = %v, want %v", frames, want)
		}
	}
}

func TestSessionRecordingFileRoundTrip(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	recorder, err := e.StartSessionRecording(RecordingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e.StepFrame()
	recording, err := recorder.Stop()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "run"+RecordingFileExtension)
	if err := SaveRecording(path, recording); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadRecording(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.FinalStateHash != recording.FinalStateHash || loaded.EndFrame != recording.EndFrame {
		t.Fatalf("loaded recording differs: got %+v want %+v", loaded, recording)
	}
}

func TestReplayRecordingRejectsWrongROM(t *testing.T) {
	source, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()

	recorder, err := source.StartSessionRecording(RecordingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source.StepFrame()
	recording, err := recorder.Stop()
	if err != nil {
		t.Fatal(err)
	}

	otherROM := perfROM()
	otherROM[0x0200] ^= 0x01
	other, err := New(WithROMBytes(otherROM), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.ReplayRecording(recording); err == nil {
		t.Fatal("recording from another ROM was accepted")
	}
}

func TestStartSessionRecordingRejectsActiveInputRecording(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.StartInputRecording()
	if _, err := e.StartSessionRecording(RecordingOptions{}); err == nil {
		t.Fatal("session recording started while low-level input recording was active")
	}
	e.StopInputRecording()
}

func TestParseRecordingRejectsCorruptArchive(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	recorder, err := e.StartSessionRecording(RecordingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	e.StepFrame()
	recording, err := recorder.Stop()
	if err != nil {
		t.Fatal(err)
	}
	archive, err := recording.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}

	corrupt := append([]byte(nil), archive...)
	for i := len(corrupt) - 1; i >= 0 && i > len(corrupt)-32; i-- {
		corrupt[i] ^= 0xFF
	}
	if _, err := ParseRecording(corrupt); err == nil {
		t.Fatal("corrupt recording archive was accepted")
	}
}
