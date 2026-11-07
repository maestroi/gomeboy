//go:build test

package tests

import (
	"bytes"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
)

// NCM is CGB hardware booted with a DMG-only cartridge. AGE's pinned NCM ROM
// runs eight SCX values x five *normal-speed* probes (480 individual reads).
// The CGB-colour ROM runs another seven double-speed probes per SCX and is
// already guarded by the 1,152-sample floor in age_vram_diagnostics_test.go.
//
// The ROM itself judges pass/fail. This diagnostic does not change any xfails.
func logAgeNCMVRAMRaw(t *testing.T, g *gameboy.GameBoy) {
	t.Helper()
	const matrixBytes = 8 * 5 * 12
	base, fingerprint, ok := ageVRAMRawTimingBase(g)
	t.Logf("AGE NCM raw VRAM WRAM candidate: address=%#04x first_scx_matches=%d/%d accepted=%t",
		base, fingerprint, len(ageVRAMFirstSCXExpected), ok)
	if !ok {
		t.Log("AGE NCM raw result buffer not confidently identified")
		return
	}

	// The ROM contains two similar expected-data arrays. Selecting one by
	// highest matching raw byte count alone can switch candidates whenever
	// the emulator improves. AGE's final BG tile-map stores its *own* first
	// mismatching byte for each group: use those independent ROM results to
	// identify the expected-data array, not the byte score we're measuring.
	var compact [8][5]byte
	for scx := 0; scx < 8; scx++ {
		values, _, valid := ageCompactResultsFromBG(g, scx)
		if !valid {
			t.Logf("AGE NCM compact result row %d could not be decoded", scx)
			return
		}
		copy(compact[scx][:], values[:5])
	}

	bestAt, bestMatched, bestAgreement, candidates := -1, -1, -1, 0
	for from := 0; from < len(g.ROM); {
		rel := bytes.Index(g.ROM[from:], ageVRAMFirstSCXExpected[:])
		if rel < 0 {
			break
		}
		romAt := from + rel
		from = romAt + 1
		if romAt+matrixBytes > len(g.ROM) {
			continue
		}
		candidates++
		matched, agreement := 0, 0
		for scx := 0; scx < 8; scx++ {
			for probe := 0; probe < 5; probe++ {
				off := (scx*5 + probe)*12
				firstBad := byte(0xff)
				for i := 0; i < 12; i++ {
					if g.ROM[romAt+off+i] == g.Bus.Get(base+uint16(off+i)) {
						matched++
					} else if firstBad == 0xff {
						firstBad = byte(i)
					}
				}
				if firstBad == compact[scx][probe] {
					agreement++
				}
			}
		}
		t.Logf("AGE NCM VRAM expected candidate: rom_offset=%#x matching_samples=%d/%d compact_agreement=%d/40",
			romAt, matched, matrixBytes, agreement)
		if agreement > bestAgreement || (agreement == bestAgreement && matched > bestMatched) {
			bestAt, bestMatched, bestAgreement = romAt, matched, agreement
		}
	}
	t.Logf("AGE NCM VRAM selected expected matrix: rom_offset=%#x matching_samples=%d/%d compact_agreement=%d/40 candidates=%d",
		bestAt, bestMatched, matrixBytes, bestAgreement, candidates)
	if bestAt < 0 || bestMatched < matrixBytes/2 || bestAgreement < 39 {
		t.Log("AGE NCM raw expected matrix does not agree with ROM's compact verdicts; refusing to guess")
		return
	}

	badGroups, badSamples := 0, 0
	for scx := 0; scx < 8; scx++ {
		for probe := 0; probe < 5; probe++ {
			off := (scx*5 + probe)*12
			var expected, observed [12]byte
			firstBad, bad := -1, 0
			for i := range expected {
				expected[i] = g.ROM[bestAt+off+i]
				observed[i] = g.Bus.Get(base + uint16(off+i))
				if expected[i] != observed[i] {
					if firstBad < 0 {
						firstBad = i
					}
					bad++
				}
			}
			if bad != 0 {
				badGroups++
				badSamples += bad
				t.Logf("AGE NCM VRAM raw divergence: scx=%d probe=%d first_bad_byte=%d bad_samples=%d expected=% x observed=% x",
					scx, probe, firstBad, bad, expected, observed)
			}
		}
	}
	t.Logf("AGE NCM VRAM raw matrix: groups_mismatched=%d samples_mismatched=%d total_groups=40 total_samples=%d samples_matched=%d",
		badGroups, badSamples, matrixBytes, matrixBytes-badSamples)
}
