//go:build test

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)


// ageVRAMSampleBaseline pins a minimum count of correct raw CPU read samples.
// The whole-ROM AGE regression gate separately checks PASS/XPASS/XFAIL; this
// floor also detects partial accuracy regressions hidden inside existing XFAILs.
type ageVRAMSampleBaseline struct {
	SchemaVersion   int    `json:"schema_version"`
	SourceRevision string `json:"source_revision"`
	ROM            string `json:"rom"`
	Models         map[string]struct {
		SampleCount            int `json:"sample_count"`
		MinimumMatchingSamples int `json:"minimum_matching_samples"`
	} `json:"models"`
}

func readAgeVRAMSampleBaseline() (ageVRAMSampleBaseline, error) {
	data, err := os.ReadFile("age-vram-sample-baseline.json")
	if err != nil {
		return ageVRAMSampleBaseline{}, err
	}
	var baseline ageVRAMSampleBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return ageVRAMSampleBaseline{}, err
	}
	if baseline.SchemaVersion != 1 || baseline.SourceRevision != "cd3f654d13bf7137fa4f7fb7e5dd041d10f7098e" ||
		baseline.ROM != "vram-read-cgbBCE.gb" {
		return ageVRAMSampleBaseline{}, fmt.Errorf("unsupported AGE VRAM sample baseline identity")
	}
	return baseline, nil
}

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

func logAgeVRAMRawFirstSCX(t *testing.T, g *gameboy.GameBoy) (int, bool) {
	t.Helper()
	base, score, ok := ageVRAMRawTimingBase(g)
	t.Logf("AGE raw VRAM timing candidate: address=%#04x fingerprint_match=%d/%d accepted=%t", base, score, len(ageVRAMFirstSCXExpected), ok)
	if !ok {
		t.Log("AGE raw VRAM timing candidate is low-confidence; cannot enforce sample baseline")
		return 0, false
	}

	for probe := 0; probe < 5; probe++ {
		var actual [12]byte
		for i := range actual {
			actual[i] = g.Bus.Get(base + uint16(12*probe+i))
		}
		expected := ageVRAMFirstSCXExpected[12*probe : 12*(probe+1)]
		t.Logf("AGE raw VRAM SCX=0 speed=normal probe=%d expected=% x observed=% x", probe, expected, actual)
	}

	// AGE embeds the entire EXPECTED_TIMING_RESULTS matrix in its ROM.
	// Locate it using the same verified source fingerprint, so we can
	// compare *every* raw CPU sample without copying a 1,152-byte timing
	// matrix into the emulator test suite.
	const matrixBytes = (8*5 + 8*7) * 12
	expectedAt, bestScore, secondScore := -1, -1, -1
	candidates := 0
	for from := 0; from < len(g.ROM); {
		rel := bytes.Index(g.ROM[from:], ageVRAMFirstSCXExpected[:])
		if rel < 0 {
			break
		}
		candidate := from + rel
		from = candidate + 1
		if candidate+matrixBytes > len(g.ROM) {
			continue
		}
		candidates++
		score := 0
		for i := 0; i < matrixBytes; i++ {
			if g.ROM[candidate+i] == g.Bus.Get(base+uint16(i)) {
				score++
			}
		}
		if score > bestScore {
			secondScore = bestScore
			bestScore, expectedAt = score, candidate
		} else if score > secondScore {
			secondScore = score
		}
	}

	// A matching prefix can occur more than once in the ROM. Select the
	// matrix by its agreement across all 96 groups, and refuse low-confidence
	// candidates rather than silently interpreting unrelated ROM constants.
	t.Logf("AGE raw VRAM matrix candidates: count=%d best_offset=%#x matching_samples=%d/%d runner_up=%d",
		candidates, expectedAt, bestScore, matrixBytes, secondScore)
	if expectedAt < 0 || bestScore < matrixBytes*3/4 {
		t.Log("AGE raw VRAM matrix candidates are low-confidence; cannot enforce sample baseline")
		return 0, false
	}
	t.Logf("AGE raw VRAM expected matrix: ROM offset=%#x, WRAM address=%#04x", expectedAt, base)
	totalBadSamples := 0
	totalBadGroups := 0
	for row := 0; row < 16; row++ {
		groups := 5
		rowOffset := row * 5 * 12
		if row >= 8 {
			groups = 7
			rowOffset = 8*5*12 + (row-8)*7*12
		}
		for probe := 0; probe < groups; probe++ {
			off := rowOffset + probe*12
			firstBad := -1
			groupBad := 0
			var wanted, observed byte
			var expectedBytes, observedBytes [12]byte
			for i := 0; i < len(expectedBytes); i++ {
				want := g.ROM[expectedAt+off+i]
				got := g.Bus.Get(base + uint16(off+i))
				expectedBytes[i], observedBytes[i] = want, got
				if want != got {
					if firstBad < 0 {
						firstBad, wanted, observed = i, want, got
					}
					groupBad++
				}
			}
			if groupBad > 0 {
				totalBadGroups++
				totalBadSamples += groupBad
				speed := "normal"
				if row >= 8 {
					speed = "double"
				}
				t.Logf("AGE VRAM raw divergence: speed=%s scx=%d probe=%d first_bad_byte=%d expected=%02x observed=%02x bad_samples=%d",
					speed, row%8, probe, firstBad, wanted, observed, groupBad)
				// The six byte pairs sample Mode-3/Mode-0 edges on the first,
				// next, last visible, VBlank, and next-frame scanlines.
				// Report the full byte pattern to distinguish early closure
				// from late reopening without inventing a timing cutoff.
				t.Logf("AGE VRAM raw edge bytes: speed=%s scx=%d probe=%d expected=% x observed=% x",
					speed, row%8, probe, expectedBytes, observedBytes)
			}
		}
	}
	t.Logf("AGE VRAM raw matrix: groups_mismatched=%d samples_mismatched=%d total_groups=96 total_samples=%d samples_matched=%d",
		totalBadGroups, totalBadSamples, matrixBytes, matrixBytes-totalBadSamples)
	return matrixBytes - totalBadSamples, true
}

// TestAgeVRAMReadDiagnostics is intentionally not part of the normal
// Test_Regressions gate. It runs only when explicitly selected in CI to
// print the first failing byte of each AGE VRAM timing probe group.
// It never adjusts the checked-in expected-failure baseline.
func TestAgeVRAMReadDiagnostics(t *testing.T) {

	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("pinned AGE ROMs not present: %v", err)
	}
	baseline, err := readAgeVRAMSampleBaseline()
	if err != nil {
		t.Fatal(err)
	}
	ncmBaseline, err := readAgeNCMVRAMSampleBaseline()
	if err != nil {
		t.Fatal(err)
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
			// Pin a raw per-sample accuracy floor on the production CGB B/C
			// profile; log CGB D/E for a future measured baseline update.
			if tc.rom == baseline.ROM && (tc.model == types.CGBBC || tc.model == types.CGBDE) {
				matched, ok := logAgeVRAMRawFirstSCX(t, g)
				if !ok {
					t.Fatal("unable to decode AGE raw VRAM timing samples")
				}
				if floor, exists := baseline.Models[tc.model.String()]; exists {
					if floor.SampleCount != 1152 || floor.MinimumMatchingSamples < 0 || floor.MinimumMatchingSamples > floor.SampleCount {
						t.Fatalf("invalid AGE raw sample baseline for %s: %+v", tc.model, floor)
					}
					if matched < floor.MinimumMatchingSamples {
						t.Errorf("AGE VRAM raw-sample regression: model=%s got %d/%d matching, baseline minimum %d/%d",
							tc.model, matched, floor.SampleCount, floor.MinimumMatchingSamples, floor.SampleCount)
					}
				}
			}

			if tc.rom == ncmBaseline.ROM {
				matched, ok := logAgeNCMVRAMRaw(t, g)
				if !ok {
					t.Fatal("unable to decode AGE NCM raw VRAM timing samples")
				}
				floor, exists := ncmBaseline.Models[tc.model.String()]
				if !exists || floor.SampleCount != 480 ||
					floor.MinimumMatchingSamples < 0 || floor.MinimumMatchingSamples > floor.SampleCount {
					t.Fatalf("invalid or missing AGE NCM raw sample baseline for %s: %+v", tc.model, floor)
				}
				if matched < floor.MinimumMatchingSamples {
					t.Errorf("AGE NCM VRAM raw-sample regression: model=%s got %d/%d matching, baseline minimum %d/%d",
						tc.model, matched, floor.SampleCount, floor.MinimumMatchingSamples, floor.SampleCount)
				}
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
