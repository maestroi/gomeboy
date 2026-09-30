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
	for _, model := range []types.Model{
		types.DMG0, types.DMGABC, types.MGB, types.SGB, types.SGB2,
		types.CGB0, types.CGBABC, types.CGBBC, types.CGBDE, types.AGB,
	} {
		t.Run(model.String(), func(t *testing.T) {
			g := NewGameBoy(AsModel(model), WithoutSaves())
			if err := g.LoadROMBytes(rom, "whichboot"); err != nil {
				t.Fatal(err)
			}
			g.StepFrames(2)

			// whichboot.gb v1.1 stores its startup timing capture in the first
			// HRAM variable block: LY, DIV and the reconstructed low six DIV
			// phase bits at FF85-FF87.
			t.Logf("whichboot %s: LY=%02X DIV=%02X fine=%02X platform=%02X",
				model,
				g.Bus.Get(0xFF85),
				g.Bus.Get(0xFF86),
				g.Bus.Get(0xFF87),
				g.Bus.Get(0xFF82),
			)
		})
	}
}
