//go:build test

package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// Dumps AGE lcd-align-ly LY_VALUES (9 groups x 5 delays x 8 reads at 0xc000).
func TestLCDAlignLYDump(t *testing.T) {
	if os.Getenv("LCD_ALIGN_DUMP") == "" {
		t.Skip("set LCD_ALIGN_DUMP=1")
	}
	for _, c := range []struct {
		rom   string
		model types.Model
	}{{"lcd-align-ly-cgbBC.gb", types.CGBBC}, {"lcd-align-ly-cgbE.gb", types.CGBDE}} {
		g, err := runGameboy(filepath.Join(ageROMPath, "lcd-align-ly", c.rom), 5, DebugBreakpoint, gameboy.Debug(), gameboy.AsModel(c.model))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("== %s %s\n", c.rom, c.model)
		for i := 0; i < 45; i++ {
			for j := 0; j < 8; j++ {
				fmt.Printf("%02x ", g.Bus.Get(uint16(0xc000+i*8+j)))
			}
			fmt.Println()
		}
	}
}
