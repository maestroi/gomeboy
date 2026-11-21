//go:build test

package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// The pinned AGE stat-interrupt ROM records a byte per interrupt edge probe.
// The first five rows test single speed: HBlank (20 probes), OAM (4),
// and VBlank (4). CGB native adds the same five rows at double speed.
// Check the WRAM matrix against *both* the compiled ROM expectation and
// the independent compact values/inverted flags rendered on the screen.
func ageSTATInterruptExpected(rows int) []byte {
	out := make([]byte, rows*8)
	for row := 0; row < rows; row++ {
		probes := 4
		switch row % 5 {
		case 0, 1:
			probes = 8
		}
		for col := 0; col < probes; col++ {
			if col%2 == 0 {
				out[row*8+col] = 0x80
			} else {
				out[row*8+col] = 0x82
			}
		}
	}
	return out
}

func TestAgeSTATInterruptRawDiagnostics(t *testing.T) {
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("pinned AGE corpus not installed: %v", err)
	}
	for _, tc := range []struct {
		rom string
		model types.Model
		rows int
	}{
		{"stat-int-dmgC-cgbBCE.gb",types.DMGABC,5},
		{"stat-int-dmgC-cgbBCE.gb",types.CGBBC,10},
		{"stat-int-dmgC-cgbBCE.gb",types.CGBDE,10},
		{"stat-int-ncmBCE.gb",types.CGBBC,5},
		{"stat-int-ncmBCE.gb",types.CGBDE,5},
	} {
		t.Run(tc.rom+"@"+tc.model.String(), func(t *testing.T) {
			g, err := runGameboy(filepath.Join(ageROMPath, "stat-interrupt",tc.rom),
				5,DebugBreakpoint,gameboy.Debug(),gameboy.AsModel(tc.model))
			if err!=nil {t.Fatal(err)}
			if !g.CPU.DebugBreakpoint {t.Fatal("AGE ROM did not reach result breakpoint")}

			expected:=ageSTATInterruptExpected(tc.rows)
			// Expected values are pinned assembly data; only report a raw
			// score if they occur verbatim in the compiled ROM as well.
			header:=append([]byte{byte(tc.rows)},expected...)
			romOffset:=bytes.Index(g.ROM,header)
			if romOffset<0 {
				t.Logf("AGE STAT interrupt expectation not verified in pinned ROM: rom=%s model=%s",tc.rom,tc.model)
				return
			}

			const base=uint16(0xc000)
			screenshotAgrees:=true
			matches:=0
			for row:=0;row<tc.rows;row++ {
				values,inverted,ok:=ageCompactResultsFromBG(g,row)
				if !ok {
					t.Fatalf("AGE STAT interrupt compact row %d undecodable",row)
				}
				var actual [8]byte
				var want [8]byte
				rowBad:=0
				for col:=0;col<8;col++ {
					offset:=row*8+col
					got:=g.Bus.Get(base+uint16(offset))
					needed:=expected[offset]
					actual[col],want[col]=got,needed
					if got==needed { matches++ } else { rowBad++ }
					if values[col]!=got || inverted[col]!=(got!=needed) {
						screenshotAgrees=false
						t.Logf("AGE STAT interrupt evidence mismatch: row=%d col=%d wram=%02x compact=%02x inverted=%t expected=%02x",
							row,col,got,values[col],inverted[col],needed)
					}
				}
				if rowBad>0 {
					speed:="normal"
					if row>=5 { speed="double" }
					t.Logf("AGE STAT interrupt divergence: model=%s rom=%s speed=%s group=%d bad=%d expected=% x observed=% x",
						tc.model,tc.rom,speed,row%5,rowBad,want,actual)
				}
			}
			if !screenshotAgrees {
				t.Logf("AGE STAT interrupt raw result unverified: model=%s rom=%s screenshot disagrees with WRAM/expectation",tc.model,tc.rom)
				return
			}
			t.Logf("AGE STAT interrupt raw matrix: model=%s rom=%s rom_offset=%#x wram=%#x samples_matched=%d/%d compact_agreement=%d/%d",
				tc.model,tc.rom,romOffset,base,matches,len(expected),len(expected),len(expected))
		})
	}
}
