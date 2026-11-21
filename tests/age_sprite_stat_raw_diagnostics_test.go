//go:build test

package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// The pinned AGE sprite STAT ROM reads Mode 3/HBlank once for each
// of 16 object-X positions, at 5 normal-speed or 8 double-speed
// sampling offsets. Its uncompressed result is an 8-byte group per
// position; the compact screen reports the first mismatching byte
// for every group. Do not claim raw accuracy unless all independent
// compact verdicts agree with the selected ROM/WRAM matrix pair.
func TestAgeSpriteSTATRawDiagnostics(t *testing.T) {
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("pinned AGE corpus unavailable: %v", err)
	}
	for _, tc := range []struct {
		rom   string
		model types.Model
	}{
		{"stat-mode-sprites-dmgC-cgbBCE.gb", types.DMGABC},
		{"stat-mode-sprites-dmgC-cgbBCE.gb", types.CGBBC},
		{"stat-mode-sprites-dmgC-cgbBCE.gb", types.CGBDE},
		{"stat-mode-sprites-ds-cgbBCE.gb", types.CGBBC},
		{"stat-mode-sprites-ds-cgbBCE.gb", types.CGBDE},
	} {
		t.Run(tc.rom+"@"+tc.model.String(), func(t *testing.T) {
			g, err := runGameboy(filepath.Join(ageROMPath, "stat-mode-sprites", tc.rom),
				5, DebugBreakpoint, gameboy.Debug(), gameboy.AsModel(tc.model))
			if err != nil {
				t.Fatal(err)
			}
			if !g.CPU.DebugBreakpoint {
				t.Fatal("AGE sprite STAT ROM did not reach result breakpoint")
			}

			double := strings.Contains(tc.rom, "-ds-")
			rows := 10
			if double {
				rows = 16
			}
			const groupsPerRow = 8
			const bytesPerGroup = 8
			n := rows * groupsPerRow * bytesPerGroup

			var verdicts [16][8]byte
			for row := 0; row < rows; row++ {
				values, _, ok := ageCompactResultsFromBG(g, row)
				if !ok {
					t.Fatalf("undecodable AGE sprite compact row %d", row)
				}
				copy(verdicts[row][:], values[:8])
			}

			// First 16 groups of the pinned expected ROM matrix contain
			// Mode-3 samples (A3), independent of the hardware revision.
			// The unusually long prefix rules out unrelated ROM data;
			// the 8-byte first-mismatch checksum for all groups checks
			// the matrix position independently.
			prefix := bytes.Repeat([]byte{0xa3}, 16*bytesPerGroup)
			bestROM, bestRAM, bestMatches, bestAgreement, candidates := -1, uint16(0), -1, -1, 0
			for start := 0; start < len(g.ROM); {
				index := bytes.Index(g.ROM[start:], prefix)
				if index < 0 {
					break
				}
				at := start + index
				start = at + 1
				if at+n > len(g.ROM) {
					continue
				}
				// The suite allocates its STAT_VALUES section in WRAM0.
				// Look for the unique matrix within that memory rather
				// than assuming a ROM or test-specific address.
				for raw := 0xc000; raw+n <= 0xd000; raw++ {
					firstGroupMatch := 0
					for i := 0; i < bytesPerGroup; i++ {
						if g.Bus.Get(uint16(raw+i)) == g.ROM[at+i] {
							firstGroupMatch++
						}
					}
					if firstGroupMatch < 4 {
						continue
					}
					candidates++
					matches, agreement := 0, 0
					for row := 0; row < rows; row++ {
						for col := 0; col < groupsPerRow; col++ {
							firstBad := byte(0xff)
							for j := 0; j < bytesPerGroup; j++ {
								offset := (row*groupsPerRow+col)*bytesPerGroup+j
								if g.Bus.Get(uint16(raw+offset)) == g.ROM[at+offset] {
									matches++
								} else if firstBad == 0xff {
									firstBad = byte(j)
								}
							}
							if firstBad == verdicts[row][col] {
								agreement++
							}
						}
					}
					if agreement > bestAgreement || agreement == bestAgreement && matches > bestMatches {
						bestROM, bestRAM, bestMatches, bestAgreement = at, uint16(raw), matches, agreement
					}
				}
			}
			if bestROM < 0 || bestAgreement != rows*groupsPerRow {
				t.Logf("AGE sprite STAT matrix unverified: rom=%s model=%s candidates=%d agreement=%d/%d; no raw accuracy claimed",
					tc.rom, tc.model, candidates, bestAgreement, rows*groupsPerRow)
				return
			}
			t.Logf("AGE sprite STAT raw matrix: rom=%s model=%s rom_offset=%#x wram=%#x samples_matched=%d/%d compact_agreement=%d/%d",
				tc.rom, tc.model, bestROM, bestRAM, bestMatches, n, bestAgreement, rows*groupsPerRow)

			for row := 0; row < rows; row++ {
				badGroups, badSamples := 0, 0
				var firstBadBySPX [8]int
				for spx := 0; spx < groupsPerRow; spx++ {
					firstBadBySPX[spx] = -1
					for j := 0; j < bytesPerGroup; j++ {
						offset := (row*groupsPerRow+spx)*bytesPerGroup+j
						if g.Bus.Get(bestRAM+uint16(offset)) != g.ROM[bestROM+offset] {
							badSamples++
							if firstBadBySPX[spx] < 0 {
								firstBadBySPX[spx] = j
								badGroups++
							}
						}
					}
				}
				if badGroups > 0 {
					t.Logf("AGE sprite STAT divergence: rom=%s model=%s NOP=%d SPX=%d-%d groups_bad=%d/8 samples_bad=%d first_bad=%v",
						tc.rom, tc.model, row/2, (row%2)*8, (row%2)*8+7, badGroups, badSamples, firstBadBySPX)
				}
			}
		})
	}
}
