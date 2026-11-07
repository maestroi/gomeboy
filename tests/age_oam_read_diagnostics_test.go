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

// AGE's OAM CPU-read test writes twelve timing samples per probe into WRAM0.
// Five normal-speed probes run at each SCX=0..7; full CGB-colour ROMs also
// execute seven double-speed probes. The first-SCX expected bytes are common
// to the ROM variants (the EFF hardware distinction is in later SCX groups).
var ageOAMReadFirstSCXExpected = func() [60]byte {
	var result [60]byte
	mode3 := [...]byte{0, 255, 0, 255, 0, 255, 0, 0, 0, 255, 0, 255}
	mode2 := [...]byte{255, 0, 255, 0, 255, 0, 0, 0, 255, 0, 255, 0}
	for probe := 0; probe < 5; probe++ {
		a := mode2[:]
		if probe < 2 { a = mode3[:] }
		copy(result[probe*12:probe*12+12], a)
	}
	return result
}()

func logAGEOAMReadMatrix(t *testing.T, g *gameboy.GameBoy, rows int) (int, bool) {
	t.Helper()
	const bytesPerGroup = 12
	totalGroups := 8*5
	if rows == 16 { totalGroups = 8*5+8*7 }
	matrixBytes := totalGroups*bytesPerGroup

	// Identify the actual WRAM matrix from its mixed open and blocked reads.
	base, fingerprintScore := uint16(0), -1
	for offset := 0; offset <= 0x1000-len(ageOAMReadFirstSCXExpected); offset++ {
		addr, score := uint16(0xc000+offset), 0
		for i, expected := range ageOAMReadFirstSCXExpected {
			if g.Bus.Get(addr+uint16(i)) == expected { score++ }
		}
		if score > fingerprintScore { fingerprintScore, base = score, addr }
	}
	t.Logf("AGE OAM read WRAM candidate: address=%#04x fingerprint=%d/60", base, fingerprintScore)
	if fingerprintScore < 32 { return 0, false }

	var verdicts [16][7]byte
	for row := 0; row < rows; row++ {
		values, _, ok := ageCompactResultsFromBG(g, row)
		if !ok { return 0, false }
		copy(verdicts[row][:], values[:7])
	}

	bestAt, bestSamples, bestVerdicts, candidates := -1, -1, -1, 0
	for from := 0; from < len(g.ROM); {
		k := bytes.Index(g.ROM[from:], ageOAMReadFirstSCXExpected[:])
		if k < 0 { break }
		at := from+k
		from = at+1
		if at+matrixBytes > len(g.ROM) { continue }
		candidates++
		samples, matchedVerdicts := 0, 0
		for row := 0; row < rows; row++ {
			n := 5
			start := row*5*12
			if row >= 8 { n=7; start=8*5*12+(row-8)*7*12 }
			for probe := 0; probe < n; probe++ {
				firstBad := byte(255)
				for i := 0; i < bytesPerGroup; i++ {
					offset := start+probe*12+i
					if g.ROM[at+offset] == g.Bus.Get(base+uint16(offset)) {
						samples++
					} else if firstBad == 255 { firstBad = byte(i) }
				}
				if firstBad == verdicts[row][probe] { matchedVerdicts++ }
			}
		}
		t.Logf("AGE OAM read ROM expected candidate: offset=%#x samples_matched=%d/%d compact_agreement=%d/%d",
			at, samples, matrixBytes, matchedVerdicts, totalGroups)
		if matchedVerdicts > bestVerdicts || matchedVerdicts == bestVerdicts && samples > bestSamples {
			bestAt, bestSamples, bestVerdicts = at, samples, matchedVerdicts
		}
	}
	t.Logf("AGE OAM read selected expected matrix: offset=%#x matching=%d/%d compact_agreement=%d/%d candidates=%d",
		bestAt, bestSamples, matrixBytes, bestVerdicts, totalGroups, candidates)
	if bestAt < 0 || bestVerdicts != totalGroups { return 0, false }

	badSamples, badGroups := 0, 0
	for row := 0; row < rows; row++ {
		n := 5
		start := row*5*12
		if row >= 8 { n=7; start=8*5*12+(row-8)*7*12 }
		for probe := 0; probe < n; probe++ {
			offset := start+probe*12
			var expected, observed [12]byte
			first, bad := -1, 0
			for i := range expected {
				expected[i] = g.ROM[bestAt+offset+i]
				observed[i] = g.Bus.Get(base+uint16(offset+i))
				if expected[i] != observed[i] {
					if first < 0 { first = i }
					bad++
				}
			}
			if bad > 0 {
				badGroups++; badSamples += bad
				speed := "normal"
				if row >= 8 { speed="double" }
				t.Logf("AGE OAM read divergence: speed=%s scx=%d probe=%d first_bad_byte=%d bad_samples=%d expected=% x observed=% x",
					speed,row%8,probe,first,bad,expected,observed)
			}
		}
	}
	t.Logf("AGE OAM read raw matrix: groups_mismatched=%d samples_mismatched=%d total_groups=%d total_samples=%d samples_matched=%d",
		badGroups,badSamples,totalGroups,matrixBytes,matrixBytes-badSamples)
	return matrixBytes-badSamples, true
}

func TestAgeOAMReadDiagnostics(t *testing.T) {
	runAgeOAMReadDiagnostics(t)
}

// runAgeOAMReadDiagnostics is also invoked from the pinned VRAM diagnostics
// job, so CI captures all OAM-read timing bytes without a workflow change.
func runAgeOAMReadDiagnostics(t *testing.T) {
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("pinned AGE ROMs not present: %v",err)
	}
	for _, tc := range []struct{rom string; model types.Model; rows int}{
		{"oam-read-cgbE.gb",types.CGBDE,16},
		{"oam-read-dmgC-cgbBC.gb",types.DMGABC,8},
		{"oam-read-dmgC-cgbBC.gb",types.CGBBC,16},
		{"oam-read-ncmBC.gb",types.CGBBC,8},
		{"oam-read-ncmE.gb",types.CGBDE,8},
	} {
		t.Run(tc.rom+"@"+tc.model.String(),func(t *testing.T){
			g,err:=runGameboy(filepath.Join(ageROMPath,"oam",tc.rom),5,DebugBreakpoint,gameboy.Debug(),gameboy.AsModel(tc.model))
			if err!=nil { t.Fatal(err) }
			if !g.CPU.DebugBreakpoint {t.Fatal("AGE OAM read ROM did not reach breakpoint")}
			if _,ok:=logAGEOAMReadMatrix(t,g,tc.rows);!ok {
				t.Log("AGE OAM read matrix could not be identified confidently; compact results remain authoritative")
			}
			groupsMatched:=0
			for row:=0;row<tc.rows;row++ {
				values,_,ok:=ageCompactResultsFromBG(g,row)
				if !ok {t.Fatalf("cannot decode AGE OAM compact row %d",row)}
				count:=5
				if row>=8{count=7}
				for i:=0;i<count;i++ {if values[i]==0xff{groupsMatched++}}
			}
			totalGroups := 40
			if tc.rows == 16 {
				totalGroups = 96
			}
			t.Logf("AGE OAM read compact: model=%s ROM=%s groups_matched=%d/%d",
				tc.model,tc.rom,groupsMatched,totalGroups)
		})
	}
}
