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

type ageSTATProbe struct {
	row, col int
	firstBad byte
}

// Normal-speed AGE STAT mode runs 12 rows with 3 probes and 2 rows with 2.
// The double-speed ROM runs 10 rows with 6 probes and 4 rows with 3.
// In particular, the double-speed raw matrix has 72*12 bytes, not 40*12.
func ageSTATProbeColumns(row int, doubleSpeed bool) int {
	if doubleSpeed {
		if row < 10 {
			return 6
		}
		return 3
	}
	if row < 12 {
		return 3
	}
	return 2
}

func TestAgeSTATProbeLayout(t *testing.T) {
	for _, tc := range []struct {
		doubleSpeed bool
		want        int
	}{
		{false, 40},
		{true, 72},
	} {
		total := 0
		for row := 0; row < 14; row++ {
			total += ageSTATProbeColumns(row, tc.doubleSpeed)
		}
		if total != tc.want {
			t.Fatalf("doubleSpeed=%t has %d probes, want %d", tc.doubleSpeed, total, tc.want)
		}
	}
}

// Read the pinned AGE stat-mode ROM's uncompressed CPU STAT timing samples.
// Locate an embedded ROM expectation and WRAM data pair using the same
// independently rendered compact first-mismatch verdict for EVERY probe.
// A plausible prefix or raw byte count by itself is never enough to claim
// a raw accuracy score; neither this test nor its diagnostics alters hardware.
func TestAgeSTATModeRawDiagnostics(t *testing.T) {
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("AGE corpus missing: %v", err)
	}
	for _, tc := range []struct {
		rom   string
		model types.Model
	}{
		{"stat-mode-dmgC-cgbBC.gb", types.DMGABC},
		{"stat-mode-dmgC-cgbBC.gb", types.CGBBC},
		{"stat-mode-cgbE.gb", types.CGBDE},
		{"stat-mode-ds-cgbBCE.gb", types.CGBBC},
		{"stat-mode-ds-cgbBCE.gb", types.CGBDE},
		{"stat-mode-ncmBC.gb", types.CGBBC},
		{"stat-mode-ncmE.gb", types.CGBDE},
	} {
		t.Run(tc.rom+"@"+tc.model.String(), func(t *testing.T) {
			g, err := runGameboy(filepath.Join(ageROMPath, "stat-mode", tc.rom),
				5, DebugBreakpoint, gameboy.Debug(), gameboy.AsModel(tc.model))
			if err != nil {
				t.Fatal(err)
			}
			if !g.CPU.DebugBreakpoint {
				t.Fatal("AGE ROM did not reach breakpoint")
			}

			doubleSpeed := strings.Contains(tc.rom, "-ds-")
			var probes []ageSTATProbe
			for row := 0; row < 14; row++ {
				values, _, ok := ageCompactResultsFromBG(g, row)
				if !ok {
					t.Fatalf("cannot decode compact row %d", row)
				}
				for col := 0; col < ageSTATProbeColumns(row, doubleSpeed); col++ {
					probes = append(probes, ageSTATProbe{row, col, values[col]})
				}
			}
			n := len(probes) * 12

			// Pinned AGE initial SCX=0 12-byte expected read group.
			// Native-colour Mode-0 failures may disagree on most of
			// this group, so requiring 9/12 matching WRAM bytes would
			// silently reject the correct buffer before checking the
			// independent compact verdict for all 40 or 72 groups.
			prefix := []byte{0x83, 0x83, 0x83, 0x83, 0x83, 0x83, 0x81, 0x81, 0x81, 0x81, 0x83, 0x83}
			bestAgreement, bestMatches, bestROM, bestRAM, candidates := -1, -1, -1, uint16(0), 0
			for at := 0; at < len(g.ROM); {
				k := bytes.Index(g.ROM[at:], prefix)
				if k < 0 {
					break
				}
				at += k
				if at+n > len(g.ROM) {
					at++
					continue
				}
				for raw := 0xc000; raw+n <= 0xd000; raw++ {
					prefixMatches := 0
					for i, want := range prefix {
						if g.Bus.Get(uint16(raw+i)) == want {
							prefixMatches++
						}
					}
					if prefixMatches < 4 {
						continue
					}
					candidates++
					matches, agreement := 0, 0
					for group, probe := range probes {
						firstBad := byte(0xff)
						for j := 0; j < 12; j++ {
							off := group*12 + j
							if g.Bus.Get(uint16(raw+off)) == g.ROM[at+off] {
								matches++
							} else if firstBad == 0xff {
								firstBad = byte(j)
							}
						}
						if firstBad == probe.firstBad {
							agreement++
						}
					}
					if agreement > bestAgreement || agreement == bestAgreement && matches > bestMatches {
						bestAgreement, bestMatches, bestROM, bestRAM = agreement, matches, at, uint16(raw)
					}
				}
				at++
			}

			if bestROM < 0 || bestAgreement != len(probes) {
				// No unverified pairing may be reported as a raw score.
				if bestROM >= 0 {
					t.Logf("AGE tentative STAT matrix: model=%s rom=%s rom_offset=%#x wram=%#x compact_agreement=%d/%d (unverified)",
						tc.model, tc.rom, bestROM, bestRAM, bestAgreement, len(probes))
					for group, probe := range probes {
						firstBad := byte(0xff)
						var expected, observed [12]byte
						for j := 0; j < 12; j++ {
							off := group*12 + j
							expected[j] = g.ROM[bestROM+off]
							observed[j] = g.Bus.Get(bestRAM+uint16(off))
							if firstBad == 0xff && expected[j] != observed[j] {
								firstBad = byte(j)
							}
						}
						if firstBad != probe.firstBad {
							t.Logf("AGE tentative STAT mismatch: model=%s rom=%s row=%d probe=%d compact_first_bad=%d candidate_first_bad=%d expected=% x observed=% x",
								tc.model, tc.rom, probe.row, probe.col, probe.firstBad, firstBad, expected, observed)
						}
					}
				}
				// Known DMG and NCM layouts have already been independently
				// validated. Their matrix must remain unambiguous.
				if tc.model == types.DMGABC || tc.rom == "stat-mode-ncmBC.gb" || tc.rom == "stat-mode-ncmE.gb" {
					t.Fatalf("required raw STAT matrix not validated: candidates=%d agreement=%d/%d matched=%d/%d",
						candidates, bestAgreement, len(probes), bestMatches, n)
				}
				t.Logf("AGE raw STAT matrix uncertain: model=%s rom=%s candidates=%d agreement=%d/%d matched=%d/%d (no raw score claimed)",
					tc.model, tc.rom, candidates, bestAgreement, len(probes), bestMatches, n)
				return
			}

			t.Logf("AGE raw STAT matrix: model=%s rom=%s rom_offset=%#x wram=%#x samples_matched=%d/%d compact_agreement=%d/%d",
				tc.model, tc.rom, bestROM, bestRAM, bestMatches, n, bestAgreement, len(probes))
			for group, probe := range probes {
				if probe.firstBad == 0xff {
					continue
				}
				var expected, observed [12]byte
				for j := 0; j < 12; j++ {
					off := group*12 + j
					expected[j] = g.ROM[bestROM+off]
					observed[j] = g.Bus.Get(bestRAM+uint16(off))
				}
				t.Logf("AGE raw STAT divergence: model=%s rom=%s row=%d probe=%d first_bad=%d expected=% x observed=% x",
					tc.model, tc.rom, probe.row, probe.col, probe.firstBad, expected, observed)
			}
		})
	}
}
