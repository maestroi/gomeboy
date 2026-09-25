package cpu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

func TestBootMatchesWhichbootHardwareRegisterFingerprints(t *testing.T) {
	// Hardware reference values are taken from whichboot.gb's documented
	// boot-ROM fingerprints:
	// https://github.com/nitro2k01/whichboot.gb/blob/545436fb485f9006d47e0f26f0ceec76cd8e3a07/README.md#reference-values-for-hardware
	tests := []struct {
		name          string
		model         types.Model
		af, bc, de, hl uint16
	}{
		{name: "DMG0", model: types.DMG0, af: 0x0100, bc: 0xFF13, de: 0x00C1, hl: 0x8403},
		{name: "DMG", model: types.DMGABC, af: 0x01B0, bc: 0x0013, de: 0x00D8, hl: 0x014D},
		{name: "MGB", model: types.MGB, af: 0xFFB0, bc: 0x0013, de: 0x00D8, hl: 0x014D},
		{name: "SGB", model: types.SGB, af: 0x0100, bc: 0x0014, de: 0x0000, hl: 0xC060},
		{name: "SGB2", model: types.SGB2, af: 0xFF00, bc: 0x0014, de: 0x0000, hl: 0xC060},
		{name: "CGB0", model: types.CGB0, af: 0x1180, bc: 0x0000, de: 0xFF56, hl: 0x000D},
		{name: "CGB", model: types.CGBABC, af: 0x1180, bc: 0x0000, de: 0xFF56, hl: 0x000D},
		{name: "AGB", model: types.AGB, af: 0x1180, bc: 0x0100, de: 0xFF56, hl: 0x000D},
	}

	pair := func(hi, lo uint8) uint16 {
		return uint16(hi)<<8 | uint16(lo)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var c CPU
			c.Boot(tc.model)

			if got := pair(c.A, c.F); got != tc.af {
				t.Errorf("AF = %04X, want %04X", got, tc.af)
			}
			if got := pair(c.B, c.C); got != tc.bc {
				t.Errorf("BC = %04X, want %04X", got, tc.bc)
			}
			if got := pair(c.D, c.E); got != tc.de {
				t.Errorf("DE = %04X, want %04X", got, tc.de)
			}
			if got := pair(c.H, c.L); got != tc.hl {
				t.Errorf("HL = %04X, want %04X", got, tc.hl)
			}
			if c.SP != 0xFFFE {
				t.Errorf("SP = %04X, want FFFE", c.SP)
			}
			if c.PC != 0x0100 {
				t.Errorf("PC = %04X, want 0100", c.PC)
			}
		})
	}
}
