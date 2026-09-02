package gomeboy

import "testing"

func TestRunCPUUntilMemoryWatchpoint(t *testing.T) {
	e, err := New(WithROMBytes(agentWriteROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	e.StepInstruction() // LD HL,$C000; next opcode mutates C000.
	beforeFrame := e.FrameCount()
	stop, err := e.RunCPUUntil(8, MemoryWatchpoint(0xC000))
	if err != nil {
		t.Fatal(err)
	}
	if !stop.Matched || stop.CPUSteps != 1 {
		t.Fatalf("RunCPUUntil stop = %+v, want watchpoint after one CPU step", stop)
	}
	if stop.FramesStepped != 0 || e.FrameCount() != beforeFrame {
		t.Fatalf("watchpoint required a display frame: stop=%+v frame=%d", stop, e.FrameCount())
	}
	if got := e.Peek8(0xC000); got != 1 {
		t.Fatalf("C000 = %02X, want 01", got)
	}
}

func TestInstructionSteppingMatchesFrameStep(t *testing.T) {
	frameEmu, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer frameEmu.Close()

	cpuEmu, err := frameEmu.Fork()
	if err != nil {
		t.Fatal(err)
	}
	defer cpuEmu.Close()

	frameEmu.StepFrame()
	for steps := 0; cpuEmu.FrameCount() < frameEmu.FrameCount(); steps++ {
		if steps > 100000 {
			t.Fatal("instruction stepping did not reach the next frame")
		}
		cpuEmu.StepInstruction()
	}

	want, err := frameEmu.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	got, err := cpuEmu.StateHash()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("instruction stepping diverged from StepFrame:\n got %x\nwant %x", got, want)
	}
}
