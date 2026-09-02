package gomeboy

import (
	"bytes"
	"testing"
)

func agentWriteROM() []byte {
	rom := perfROM()
	// LD HL,$C000; INC (HL); JP $0103
	copy(rom[0x0100:], []byte{0x21, 0x00, 0xC0, 0x34, 0xC3, 0x03, 0x01})
	return rom
}

func TestDebuggerStepAndState(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	before := e.CPUState()
	if before.PC != 0x0100 {
		t.Fatalf("initial PC = %04X, want 0100", before.PC)
	}
	step := e.StepInstruction()
	if !step.Executed || step.Interrupt || step.Opcode != 0xC3 {
		t.Fatalf("StepInstruction = %+v, want executed JP opcode C3", step)
	}
	if step.PCBefore != 0x0100 || step.PCAfter != 0x0100 {
		t.Fatalf("JP step PCs = %04X -> %04X, want 0100 -> 0100", step.PCBefore, step.PCAfter)
	}
	if step.Cycles == 0 {
		t.Fatal("StepInstruction reported zero cycles")
	}
	if e.FrameCount() != 0 {
		t.Fatalf("single instruction advanced frame counter to %d", e.FrameCount())
	}
	ppu := e.PPUState()
	if ppu.Video {
		t.Fatal("PPUState.Video = true under WithoutVideo")
	}
}

func TestConditionsAndRunUntil(t *testing.T) {
	e, err := New(WithROMBytes(agentWriteROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Load HL so the next instruction is the guaranteed memory mutation.
	e.StepInstruction()
	changed := MemoryChanged(0xC000)
	if changed.Match(e) {
		t.Fatal("MemoryChanged matched on its baseline observation")
	}
	e.StepInstruction()
	if !changed.Match(e) {
		t.Fatal("MemoryChanged did not match after INC (HL)")
	}

	stop, err := e.RunUntil(5, ConditionFunc("third frame", func(e *Emulator) bool {
		return e.FrameCount() >= 3
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !stop.Matched || stop.Frame != 3 || stop.Condition != "third frame" {
		t.Fatalf("RunUntil stop = %+v", stop)
	}
}

func TestMemoryTracker(t *testing.T) {
	e, err := New(WithROMBytes(agentWriteROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.StepInstruction() // LD HL,$C000
	tracker, err := e.TrackMemory(MemoryRange{Start: 0xC000, Length: 1})
	if err != nil {
		t.Fatal(err)
	}
	before := e.Peek8(0xC000)
	e.StepInstruction() // INC (HL)
	changes := e.ChangesSince(tracker)
	if len(changes) != 1 {
		t.Fatalf("ChangesSince returned %d changes, want 1: %+v", len(changes), changes)
	}
	if changes[0].Address != 0xC000 || changes[0].Before != before || changes[0].After != before+1 {
		t.Fatalf("unexpected change: %+v", changes[0])
	}
	if again := e.ChangesSince(tracker); len(again) != 0 {
		t.Fatalf("unchanged second observation returned %+v", again)
	}
}

func TestFlightRecorderAndInputRecording(t *testing.T) {
	e, err := New(WithROMBytes(agentWriteROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.StepInstruction() // LD HL,$C000
	if err := e.EnableFlightRecorder(FlightRecorderOptions{
		Capacity:     8,
		Memory:       []MemoryRange{{Start: 0xC000, Length: 1}},
		RecordFrames: true,
	}); err != nil {
		t.Fatal(err)
	}
	e.StartInputRecording()
	e.Press(ButtonA)
	e.StepInstruction() // INC (HL), sampled by debugger path
	e.Release(ButtonA)
	e.StepFrame()

	inputs := e.StopInputRecording()
	if len(inputs) != 2 || !inputs[0].Pressed || inputs[1].Pressed {
		t.Fatalf("input log = %+v", inputs)
	}
	events := e.FlightEvents()
	var haveInput, haveMemory, haveFrame bool
	for _, event := range events {
		switch event.Kind {
		case FlightInput:
			haveInput = true
		case FlightMemoryChange:
			if event.Address == 0xC000 {
				haveMemory = true
			}
		case FlightFrame:
			haveFrame = true
		}
	}
	if !haveInput || !haveMemory || !haveFrame {
		t.Fatalf("flight events missing kinds: input=%v memory=%v frame=%v; %+v", haveInput, haveMemory, haveFrame, events)
	}
}

func TestInputReplayIsDeterministic(t *testing.T) {
	makeEmu := func() *Emulator {
		e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	source := makeEmu()
	defer source.Close()
	source.StartInputRecording()
	source.Press(ButtonA)
	source.StepFrame()
	source.Release(ButtonA)
	source.StepFrame()
	events := source.StopInputRecording()
	want, err := source.StateHash()
	if err != nil {
		t.Fatal(err)
	}

	replay := makeEmu()
	defer replay.Close()
	if err := replay.ReplayInputs(events, 2); err != nil {
		t.Fatal(err)
	}
	got, err := replay.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("replayed state hash differs:\n got %x\nwant %x", got, want)
	}
}

func TestForkIsIndependent(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.StepFrames(3)

	fork, err := e.Fork()
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Close()

	a, err := e.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	b, err := fork.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("fork hash differs at creation:\nsource %x\nfork   %x", a, b)
	}
	if fork.PPUState().Video {
		t.Fatal("fork did not preserve WithoutVideo")
	}

	fork.Press(ButtonA)
	b, err = fork.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	a2, err := e.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	if a2 != a {
		t.Fatal("mutating fork changed source state")
	}
	if b == a {
		t.Fatal("fork mutation did not change fork state")
	}
}

func TestCheckedStateMetadataAndValidation(t *testing.T) {
	e, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	e.StepFrames(3)
	wantHash, err := e.StateHash()
	if err != nil {
		t.Fatal(err)
	}

	state, err := e.SaveStateChecked()
	if err != nil {
		t.Fatal(err)
	}
	meta, err := InspectCheckedState(state)
	if err != nil {
		t.Fatal(err)
	}
	if meta.FormatVersion != 1 || meta.Frame != 3 || meta.ROMSHA256 == "" || meta.PayloadSHA256 == "" {
		t.Fatalf("bad checked-state metadata: %+v", meta)
	}

	e.StepFrames(2)
	if err := e.LoadStateChecked(state); err != nil {
		t.Fatal(err)
	}
	gotHash, err := e.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	if gotHash != wantHash {
		t.Fatalf("checked-state restore hash differs:\n got %x\nwant %x", gotHash, wantHash)
	}

	corrupt := append([]byte(nil), state...)
	corrupt[len(corrupt)-1] ^= 0xFF
	if _, err := InspectCheckedState(corrupt); err == nil {
		t.Fatal("corrupted checked state was accepted")
	}

	otherROM := perfROM()
	otherROM[0x0200] ^= 0x01
	other, err := New(WithROMBytes(otherROM), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.LoadStateChecked(state); err == nil {
		t.Fatal("checked state from another ROM was accepted")
	}
}

func TestStateHashIgnoresFramebufferOutput(t *testing.T) {
	video, err := New(WithROMBytes(perfROM()), Headless())
	if err != nil {
		t.Fatal(err)
	}
	defer video.Close()
	noVideo, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer noVideo.Close()

	video.StepFrames(5)
	noVideo.StepFrames(5)
	h1, err := video.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := noVideo.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(h1[:], h2[:]) {
		t.Fatalf("execution hash differs only because video output is disabled:\nvideo %x\nnoVideo %x", h1, h2)
	}
}
