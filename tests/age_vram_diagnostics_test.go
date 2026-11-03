//go:build test

package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// AGE writes per-SCX timing mismatch positions to its final BG tile map.
// Each compact result is 0xFF when the entire probe group matches, or the
// zero-based index of the *first* mismatching read. AGE prints two hex tiles
// per result, using inverted tiles for an unexpected value. This diagnostic
// reads those tiles without changing the emulator's PPU or CPU behavior.
func ageHexNibble(tile byte) (byte, bool) {
	// The two AGE font styles have offsets 0/64 for the first digit
	// and 128/192 for the second. Both contain the same ASCII glyph.
	switch glyph := tile & 0x3f; {
	case glyph >= '0'-32 && glyph <= '9'-32:
		return glyph - ('0' - 32), true
	case glyph >= 'A'-32 && glyph <= 'F'-32:
		return glyph - ('A' - 32) + 10, true
	default:
		return 0, false
	}
}

func ageCompactResultsFromBG(g *gameboy.GameBoy, row int) ([8]byte, [8]bool, bool) {
	var values [8]byte
	var inverse [8]bool
	// AGE's TEST_RESULTS table starts on BG row 2. Four header tiles are
	// followed by eight bytes printed as high/low hex glyph pairs.
	base := uint16(0x1800 + (row+2)*32 + 4)
	for col := 0; col < 8; col++ {
		hi := g.Bus.GetVRAM(base+uint16(col)*2, 0)
		lo := g.Bus.GetVRAM(base+uint16(col)*2+1, 0)
		a, okA := ageHexNibble(hi)
		b, okB := ageHexNibble(lo)
		if !okA || !okB {
			return values, inverse, false
		}
		values[col] = a<<4 | b
		inverse[col] = hi&0x40 != 0
	}
	return values, inverse, true
}


// ageVRAMFirstSCXExpected contains the pinned AGE CGB-B/C/E timing table's
// normal-speed SCX=0 probes (5 groups of 12 reads). We use this fingerprint
// only to locate the raw timing buffer in WRAM0; the ROM itself still judges
// whether a test passes.
var ageVRAMFirstSCXExpected = [...]byte{
	0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0xff,
	0x00, 0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0xff,
	0x00, 0x00, 0xff, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00,
	0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00,
	0xff, 0x00, 0xff, 0x00, 0xff, 0x00, 0x00, 0x00, 0xff, 0x00, 0xff, 0x00,
}

// ageVRAMRawTimingBase locates the ROM's first SCX=0 raw result group in
// WRAM0, independent of how its RGBDS linker placed the "timing-results"
// section. The fingerprint contains both blocked and accessible reads; reject
// poor matches instead of logging a plausible but unrelated RAM address.
func ageVRAMRawTimingBase(g *gameboy.GameBoy) (uint16, int, bool) {
	best, address := -1, uint16(0)
	for offset := 0; offset <= 0x1000-len(ageVRAMFirstSCXExpected); offset++ {
		addr := uint16(0xc000 + offset)
		score := 0
		for i, expected := range ageVRAMFirstSCXExpected {
			if g.Bus.Get(addr+uint16(i)) == expected {
				score++
			}
		}
		if score > best {
			best, address = score, addr
		}
	}
	// Below 45/60 the scan is usually matching uninitialized WRAM.
	return address, best, best >= 45
}

func logAgeVRAMRawFirstSCX(t *testing.T, g *gameboy.GameBoy) {
	t.Helper()
	base, score, ok := ageVRAMRawTimingBase(g)
	t.Logf("AGE raw VRAM timing candidate: address=%#04x fingerprint_match=%d/%d accepted=%t", base, score, len(ageVRAMFirstSCXExpected), ok)
	if !ok {
		t.Log("AGE raw VRAM timing candidate is low-confidence; byte samples below are diagnostic only")
	}

	for probe := 0; probe < 5; probe++ {
		var actual [12]byte
		for i := range actual {
			actual[i] = g.Bus.Get(base + uint16(12*probe+i))
		}
		expected := ageVRAMFirstSCXExpected[12*probe : 12*(probe+1)]
		t.Logf("AGE raw VRAM SCX=0 speed=normal probe=%d expected=% x observed=% x", probe, expected, actual)
	}
}

// TestAgeVRAMReadDiagnostics is intentionally not part of the normal
// Test_Regressions gate. It runs only when explicitly selected in CI to
// print the first failing byte of each AGE VRAM timing probe group.
// It never adjusts the checked-in expected-failure baseline.
func TestAgeVRAMReadDiagnostics(t *testing.T) {
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("pinned AGE ROMs not present: %v", err)
	}
	for _, tc := range []struct {
		rom     string
		model   types.Model
		rows    int
	}{
		{"vram-read-cgbBCE.gb", types.CGBBC, 16},
		{"vram-read-cgbBCE.gb", types.CGBDE, 16},
		{"vram-read-ncmBCE.gb", types.CGBBC, 8},
		{"vram-read-ncmBCE.gb", types.CGBDE, 8},
		{"vram-read-dmgC.gb", types.DMGABC, 8},
	} {
		t.Run(tc.rom+"@"+tc.model.String(), func(t *testing.T) {
			g, err := runGameboy(filepath.Join(ageROMPath, "vram", tc.rom),
				5, DebugBreakpoint, gameboy.Debug(), gameboy.AsModel(tc.model))
			if err != nil {
				t.Fatal(err)
			}
			if !g.CPU.DebugBreakpoint {
				t.Fatal("AGE test did not reach its final breakpoint")
			}
			// Dump an actual byte-for-byte timing trace for one CGB revision.
			// Remaining revisions keep the existing compact diagnostics.
			if tc.rom == "vram-read-cgbBCE.gb" && tc.model == types.CGBBC {
				logAgeVRAMRawFirstSCX(t, g)
			}

			matches, failures := 0, 0
			for row := 0; row < tc.rows; row++ {
				vals, inverse, ok := ageCompactResultsFromBG(g, row)
				if !ok {
					t.Fatalf("AGE final tile map could not be decoded at row %d", row)
				}
				groups := 5
				if tc.rows == 16 && row >= 8 {
					groups = 7
				}
				for col := 0; col < groups; col++ {
					if vals[col] == 0xff && !inverse[col] {
						matches++
						continue
					}
					failures++
					t.Logf("first divergence: model=%s ROM=%s scx=%d speed=%s probe=%d first_bad_byte=%d inverse=%t",
						tc.model, tc.rom, row%8,
						map[bool]string{false: "normal", true: "double"}[row>=8],
						col, vals[col], inverse[col])
				}
			}
			t.Logf("AGE VRAM compact probes: model=%s ROM=%s groups_matched=%d groups_mismatched=%d",
				tc.model, tc.rom, matches, failures)
		})
	}
}
