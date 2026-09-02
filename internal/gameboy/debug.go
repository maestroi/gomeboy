package gameboy

import "github.com/thelolagemann/gomeboy/internal/cpu"

// StepInstruction advances the emulator by one debugger-level CPU step while
// preserving the same save/load serialization guarantee as frame stepping.
// The returned Frames count is folded into GameBoy's public frame counter.
func (g *GameBoy) StepInstruction() cpu.InstructionStep {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.running = true
	out := g.CPU.StepInstruction()
	g.frames += out.Frames
	g.running = false
	return out
}
