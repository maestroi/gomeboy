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

	bestAt, bestMatched, runnerUp, candidates := -1, -1, -1, 0
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
		matched := 0
		for i := 0; i < matrixBytes; i++ {
			if g.ROM[romAt+i] == g.Bus.Get(base+uint16(i)) {
				matched++
			}
		}
		if matched > bestMatched {
			runnerUp = bestMatched
			bestAt, bestMatched = romAt, matched
		} else if matched > runnerUp {
			runnerUp = matched
		}
	}
	t.Logf("AGE NCM VRAM expected matrix candidate: rom_offset=%#x matching_samples=%d/%d runner_up=%d candidates=%d",
		bestAt, bestMatched, matrixBytes, runnerUp, candidates)
	if bestAt < 0 || bestMatched < matrixBytes/2 {
		t.Log("AGE NCM raw expected matrix unresolved; refusing to guess")
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
