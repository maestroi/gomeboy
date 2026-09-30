package gameboy

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

func loadWhichbootFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "tests", "roms", "whichboot", "whichboot.gb")
	rom, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("whichboot fixture not installed; run bash tests/fetch-whichboot.sh")
	}
	if err != nil {
		t.Fatal(err)
	}
	return rom
}

func TestWhichbootCapturedHLETiming(t *testing.T) {
	rom := loadWhichbootFixture(t)
	tests := []struct {
		model         types.Model
		ly, div, fine byte
		fineMax       byte
	}{
		{types.DMG0, 0x91, 0x18, 0x0D, 0x0D},
		{types.DMGABC, 0x00, 0xAB, 0x34, 0x34},
		{types.MGB, 0x00, 0xAB, 0x34, 0x34},
		// SGB transfer time depends on header bit contents; whichboot treats
		// 0x12-0x19 as the hardware-valid fine-phase range.
		{types.SGB, 0x00, 0xD8, 0x12, 0x19},
		{types.SGB2, 0x00, 0xD8, 0x12, 0x19},
		{types.CGB0, 0x90, 0x20, 0x2B, 0x2B},
		{types.CGBABC, 0x90, 0x1E, 0x28, 0x28},
		{types.CGBBC, 0x90, 0x1E, 0x28, 0x28},
		{types.CGBDE, 0x90, 0x1E, 0x28, 0x28},
		{types.AGB, 0x90, 0x1E, 0x29, 0x29},
	}

	for _, tc := range tests {
		t.Run(tc.model.String(), func(t *testing.T) {
			g := NewGameBoy(AsModel(tc.model), WithoutSaves())
			if err := g.LoadROMBytes(rom, "whichboot"); err != nil {
				t.Fatal(err)
			}
			g.StepFrames(2)

			// whichboot.gb v1.1 stores its startup timing capture in the first
			// HRAM variable block: LY, DIV and the reconstructed low six DIV
			// phase bits at FF85-FF87.
			ly := g.Bus.Get(0xFF85)
			div := g.Bus.Get(0xFF86)
			fine := g.Bus.Get(0xFF87)
			if ly != tc.ly || div != tc.div || fine < tc.fine || fine > tc.fineMax {
				t.Fatalf("whichboot timing = LY:%02X DIV:%02X.%02X, want LY:%02X DIV:%02X.%02X-%02X",
					ly, div, fine, tc.ly, tc.div, tc.fine, tc.fineMax)
			}
		})
	}
}
