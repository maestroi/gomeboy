//go:build test

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// The pinned AGE LY and STAT-mode ROMs render numerical timing results as
// high/low hex tiles at the end of their run. Decode those same tiles used
// by the regression screenshots and report which hardware phase first
// differs, without marking known XFAILs as passes or changing emulator state.
func TestAgePPUTimingDiagnostics(t *testing.T) {
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("pinned AGE corpus not installed: %v", err)
	}
	type tc struct {
		collection, rom string
		model           types.Model
		rows            int
		kind            string
	}
	for _, c := range []tc{
		{"ly", "ly-cgbE.gb", types.CGBDE, 9, "ly"},
		{"lcd-align-ly", "lcd-align-ly-cgbBC.gb", types.CGBBC, 9, "align"},
		{"lcd-align-ly", "lcd-align-ly-cgbE.gb", types.CGBDE, 9, "align"},
		{"stat-mode", "stat-mode-cgbE.gb", types.CGBDE, 14, "mode"},
		{"stat-mode", "stat-mode-dmgC-cgbBC.gb", types.DMGABC, 14, "mode"},
		{"stat-mode", "stat-mode-dmgC-cgbBC.gb", types.CGBBC, 14, "mode"},
		{"stat-mode", "stat-mode-ds-cgbBCE.gb", types.CGBBC, 14, "mode"},
		{"stat-mode", "stat-mode-ds-cgbBCE.gb", types.CGBDE, 14, "mode"},
		{"stat-mode", "stat-mode-ncmBC.gb", types.CGBBC, 14, "mode"},
		{"stat-mode", "stat-mode-ncmE.gb", types.CGBDE, 14, "mode"},
	} {
		t.Run(c.rom+"@"+c.model.String(), func(t *testing.T) {
			g, err := runGameboy(filepath.Join(ageROMPath,c.collection,c.rom),
				5,DebugBreakpoint,gameboy.Debug(),gameboy.AsModel(c.model))
			if err != nil {t.Fatal(err)}
			if !g.CPU.DebugBreakpoint {
				t.Log("AGE PPU ROM did not reach a numerical result breakpoint")
				return
			}
			bad, decoded := 0, 0
			for row:=0;row<c.rows;row++ {
				values, inverted, ok := ageCompactResultsFromBG(g,row)
				if !ok {
					t.Logf("AGE PPU numeric row undecodable: rom=%s model=%s row=%d",c.rom,c.model,row)
					return
				}
				decoded++
				cols:=8
				switch c.kind {
				case "align":
					cols=5
				case "mode":
					cols=3
					if row>=12 {cols=2}
				}
				for col:=0;col<cols;col++ {
					// LY reports the actual sampled register byte; an
					// inverted glyph means it differed from the ROM's
					// own embedded hardware expectation.
					// The align/mode ROMs report 0xff for matching
					// groups or the index of the first bad sample.
					wrong:=inverted[col]
					if c.kind!="ly" {wrong = values[col]!=0xff || inverted[col]}
					if wrong {
						bad++
						t.Logf("AGE PPU first divergence: rom=%s model=%s row=%d probe=%d value=%02x inverted=%t",
							c.rom,c.model,row,col,values[col],inverted[col])
					}
				}
			}
			t.Logf("AGE PPU compact timing: rom=%s model=%s rows_decoded=%d mismatching_probes=%d",
				c.rom,c.model,decoded,bad)
		})
	}
}
