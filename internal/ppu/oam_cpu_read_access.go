package ppu

import "github.com/maestroi/gomeboy/internal/types"

// cpuOAMReadAtDMGLastPixel refines the CPU-facing OAM read port during the
// short Mode-3 -> HBlank handover measured by AGE on DMG-C.
//
// When SCX is congruent to 3 modulo four, the last pixel has completed
// output before the PPU's scheduled HBlank event releases coarse OAM locks.
// This is CPU read access only: OAM writes, DMA and STAT still follow their
// existing independent event timing. Other physical models are not changed.
func (p *PPU) cpuOAMReadAtDMGLastPixel(defaultOpen bool) bool {
	if defaultOpen || !p.enabled || p.b.Model() != types.DMGABC {
		return defaultOpen
	}
	return p.scx&3 == 3 && p.lineState == EnterHBlank && p.lx == 168
}
