//go:build test

package tests

import (
	"bytes"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
)

// The DMG-C AGE VRAM ROM executes 40 normal-speed groups of 12 reads.
// Unlike the CGB ROM, DMG_C compiles the CFF conditional expected bytes as
// 0xff. Find this expected matrix using the DMG-specific 60-byte fingerprint,
// then cross-check all 40 group outcomes against AGE's BG-tile verdicts.
func logAgeDMGVRAMRaw(t *testing.T, g *gameboy.GameBoy) (int, bool) {
	t.Helper()
	const groups, bytesPerGroup, matrixBytes = 40, 12, 480
	fingerprint := ageVRAMFirstSCXExpected
	for _, i := range []int{1*12+2, 1*12+4, 1*12+8, 1*12+10, 2*12+0} {
		fingerprint[i] = 0xff
	}

	base, bestWRAMScore := uint16(0), -1
	for off := 0; off <= 0x1000-len(fingerprint); off++ {
		addr, score := uint16(0xc000+off), 0
		for i, expected := range fingerprint {
			if g.Bus.Get(addr+uint16(i)) == expected { score++ }
		}
		if score > bestWRAMScore {
			bestWRAMScore, base = score, addr
		}
	}
	t.Logf("AGE DMG VRAM WRAM candidate: address=%#04x fingerprint_match=%d/%d", base, bestWRAMScore, len(fingerprint))
	if bestWRAMScore < 45 { return 0, false }

	var compact [8][5]byte
	for scx := 0; scx < 8; scx++ {
		values, _, ok := ageCompactResultsFromBG(g, scx)
		if !ok { return 0, false }
		copy(compact[scx][:], values[:5])
	}

	bestROM, bestMatch, bestAgreement, count := -1, -1, -1, 0
	for from := 0; from < len(g.ROM); {
		index := bytes.Index(g.ROM[from:], fingerprint[:])
		if index < 0 { break }
		at := from + index
		from = at + 1
		if at+matrixBytes > len(g.ROM) { continue }
		count++
		matching, agreement := 0, 0
		for group := 0; group < groups; group++ {
			firstBad := byte(0xff)
			for i := 0; i < bytesPerGroup; i++ {
				k := group*bytesPerGroup + i
				if g.ROM[at+k] == g.Bus.Get(base+uint16(k)) {
					matching++
				} else if firstBad == 0xff {
					firstBad = byte(i)
				}
			}
			if firstBad == compact[group/5][group%5] { agreement++ }
		}
		t.Logf("AGE DMG VRAM candidate: rom_offset=%#x matching=%d/480 compact_agreement=%d/40",at,matching,agreement)
		if agreement > bestAgreement || agreement == bestAgreement && matching > bestMatch {
			bestROM, bestMatch, bestAgreement = at, matching, agreement
		}
	}
	t.Logf("AGE DMG VRAM selected matrix: rom_offset=%#x matched=%d/480 compact_agreement=%d/40 candidates=%d",bestROM,bestMatch,bestAgreement,count)
	if bestROM < 0 || bestAgreement != 40 { return 0, false }

	badGroups, badBytes := 0, 0
	for scx := 0; scx < 8; scx++ {
		for probe := 0; probe < 5; probe++ {
			off := (scx*5+probe)*bytesPerGroup
			var expected, actual [12]byte
			first, mismatch := -1, 0
			for i := range expected {
				expected[i] = g.ROM[bestROM+off+i]
				actual[i] = g.Bus.Get(base+uint16(off+i))
				if expected[i] != actual[i] {
					if first < 0 { first = i }
					mismatch++
				}
			}
			if mismatch > 0 {
				badGroups++; badBytes += mismatch
				t.Logf("AGE DMG VRAM divergence: scx=%d probe=%d first_bad_byte=%d bad_samples=%d expected=% x observed=% x",scx,probe,first,mismatch,expected,actual)
			}
		}
	}
	t.Logf("AGE DMG VRAM raw matrix: groups_mismatched=%d samples_mismatched=%d total_groups=40 total_samples=480 samples_matched=%d",badGroups,badBytes,480-badBytes)
	return 480-badBytes, true
}
