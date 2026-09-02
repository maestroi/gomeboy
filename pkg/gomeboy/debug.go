package gomeboy

import "github.com/thelolagemann/gomeboy/internal/types"

// CPUState is a compact read-only snapshot of the SM83 CPU and interrupt
// state. It is intended for debuggers, agent diagnostics, and assertions; it
// does not expose mutable internal pointers.
type CPUState struct {
	PC, SP                 uint16
	A, F, B, C, D, E, H, L uint8
	IME                    bool
	IE, IF                 uint8
	Halted                 bool
	DoubleSpeed            bool
	HaltBug                bool
}

// CPUState returns the current CPU registers and interrupt state.
func (e *Emulator) CPUState() CPUState {
	c := e.gb.CPU.Snapshot()
	return CPUState{
		PC:          c.PC,
		SP:          c.SP,
		A:           c.A,
		F:           c.F,
		B:           c.B,
		C:           c.C,
		D:           c.D,
		E:           c.E,
		H:           c.H,
		L:           c.L,
		IME:         e.gb.Bus.InterruptsEnabled(),
		IE:          e.gb.Bus.Get(types.IE),
		IF:          e.gb.Bus.Get(types.IF),
		Halted:      c.Halted,
		DoubleSpeed: c.DoubleSpeed,
		HaltBug:     c.HaltBug,
	}
}

// PPUState is a compact read-only view of the PPU timing and display-control
// state. Querying it does not copy the framebuffer or the pixel FIFOs.
type PPUState struct {
	Mode       uint8
	LY, LX     uint8
	STAT       uint8
	LCDEnabled bool
	BGEnabled  bool
	WinEnabled bool
	ObjEnabled bool
	SCY, SCX   uint8
	WY, WX     uint8
	LYC        uint8
	CGBMode    bool
	Video      bool
}

// PPUState returns the current PPU timing and display-control state.
func (e *Emulator) PPUState() PPUState {
	p := e.gb.PPU.DebugState()
	return PPUState{
		Mode:       p.Mode,
		LY:         p.LY,
		LX:         p.LX,
		STAT:       p.STAT,
		LCDEnabled: p.LCDEnabled,
		BGEnabled:  p.BGEnabled,
		WinEnabled: p.WinEnabled,
		ObjEnabled: p.ObjEnabled,
		SCY:        p.SCY,
		SCX:        p.SCX,
		WY:         p.WY,
		WX:         p.WX,
		LYC:        p.LYC,
		CGBMode:    p.CGBMode,
		Video:      p.Video,
	}
}

// InstructionStep is the result of StepInstruction. Executed is false when
// the CPU serviced a pending interrupt before fetching another opcode.
type InstructionStep struct {
	PCBefore  uint16
	PCAfter   uint16
	Opcode    uint8
	Executed  bool
	Interrupt bool
	Frames    uint64
	Cycles    uint64
}

// StepInstruction advances the emulator by one debugger-level CPU step. It is
// much finer grained than StepFrame and is intended for diagnostics rather
// than high-throughput emulation.
func (e *Emulator) StepInstruction() InstructionStep {
	pc := e.gb.CPU.PC
	cycle := e.gb.Cycle()
	out := e.gb.StepInstruction()
	e.recordFlightInstruction()
	return InstructionStep{
		PCBefore:  pc,
		PCAfter:   e.gb.CPU.PC,
		Opcode:    out.Opcode,
		Executed:  out.Executed,
		Interrupt: out.Interrupt,
		Frames:    out.Frames,
		Cycles:    e.gb.Cycle() - cycle,
	}
}
