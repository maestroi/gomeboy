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

// Compiled AGE OAM write ROMs use 0x81/0x00 bytes to mark accepted/blocked
// writes. The DMG-C ROM has a distinct conditional third probe.
func ageOAMWriteFirstSCXExpected(dmg bool) [60]byte {
	var out [60]byte
	for group:=0;group<5;group++ {
		for i:=0;i<12;i++ {
			val:=byte(0x00)
			if group<2 && i%2==0 || group>=3 && i%2==1 || group==2 && i%2==1 {
				val=0x81
			}
			out[group*12+i]=val
		}
	}
	if dmg {
		copy(out[24:36],[]byte{0x81,0,0x81,0,0,0,0,0,0x81,0x81,0x81,0x81})
	}
	return out
}

func logAGEOAMWriteMatrix(t *testing.T, g *gameboy.GameBoy, rows int, fingerprint [60]byte) (int, bool) {
	t.Helper()
	const bytesPerGroup = 12
	totalGroups := 8*5
	if rows == 16 { totalGroups = 8*5+8*7 }
	matrixBytes := totalGroups*bytesPerGroup

	// Identify the actual WRAM matrix from its mixed open and blocked reads.
	base, fingerprintScore := uint16(0), -1
	for offset := 0; offset <= 0x1000-len(fingerprint); offset++ {
		addr, score := uint16(0xc000+offset), 0
		for i, expected := range fingerprint {
			if g.Bus.Get(addr+uint16(i)) == expected { score++ }
		}
		if score > fingerprintScore { fingerprintScore, base = score, addr }
	}
	t.Logf("AGE OAM write WRAM candidate: address=%#04x fingerprint=%d/60", base, fingerprintScore)
	if fingerprintScore < 32 { return 0, false }

	var verdicts [16][7]byte
	for row := 0; row < rows; row++ {
		values, _, ok := ageCompactResultsFromBG(g, row)
		if !ok { return 0, false }
		copy(verdicts[row][:], values[:7])
	}

	bestAt, bestSamples, bestVerdicts, candidates := -1, -1, -1, 0
	for from := 0; from < len(g.ROM); {
		k := bytes.Index(g.ROM[from:], fingerprint[:])
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
		t.Logf("AGE OAM write ROM expected candidate: offset=%#x samples_matched=%d/%d compact_agreement=%d/%d",
			at, samples, matrixBytes, matchedVerdicts, totalGroups)
		if matchedVerdicts > bestVerdicts || matchedVerdicts == bestVerdicts && samples > bestSamples {
			bestAt, bestSamples, bestVerdicts = at, samples, matchedVerdicts
		}
	}
	t.Logf("AGE OAM write selected expected matrix: offset=%#x matching=%d/%d compact_agreement=%d/%d candidates=%d",
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
				t.Logf("AGE OAM write divergence: speed=%s scx=%d probe=%d first_bad_byte=%d bad_samples=%d expected=% x observed=% x",
					speed,row%8,probe,first,bad,expected,observed)
			}
		}
	}
	t.Logf("AGE OAM write raw matrix: groups_mismatched=%d samples_mismatched=%d total_groups=%d total_samples=%d samples_matched=%d",
		badGroups,badSamples,totalGroups,matrixBytes,matrixBytes-badSamples)
	return matrixBytes-badSamples, true
}


func TestAgeOAMWriteDiagnostics(t *testing.T) {
	runAgeOAMWriteDiagnostics(t)
}

func runAgeOAMWriteDiagnostics(t *testing.T) {
	t.Helper()
	if _,err:=os.Stat(ageROMPath);err!=nil {t.Skipf("AGE corpus not installed: %v",err)}
	for _,tc:=range []struct{rom string;model types.Model;rows int;dmg bool}{
		{"oam-write-cgbBCE.gb",types.CGBBC,16,false},
		{"oam-write-cgbBCE.gb",types.CGBDE,16,false},
		{"oam-write-dmgC.gb",types.DMGABC,8,true},
		{"oam-write-ncmBCE.gb",types.CGBBC,8,false},
		{"oam-write-ncmBCE.gb",types.CGBDE,8,false},
	}{
		t.Run(tc.rom+"@"+tc.model.String(),func(t *testing.T){
			g,err:=runGameboy(filepath.Join(ageROMPath,"oam",tc.rom),
				5,DebugBreakpoint,gameboy.Debug(),gameboy.AsModel(tc.model))
			if err!=nil {t.Fatal(err)}
			if !g.CPU.DebugBreakpoint {t.Fatal("AGE OAM write ROM did not finish")}
			if _,ok:=logAGEOAMWriteMatrix(t,g,tc.rows,ageOAMWriteFirstSCXExpected(tc.dmg));!ok {
				t.Log("AGE OAM write expected matrix could not be fully verified")
			}
			matched,total:=0,0
			for row:=0;row<tc.rows;row++{
				vals,inv,ok:=ageCompactResultsFromBG(g,row)
				if !ok {t.Fatalf("invalid AGE OAM write compact row %d",row)}
				n:=5
				if row>=8 {n=7}
				for col:=0;col<n;col++{
					total++
					if vals[col]==0xff && !inv[col] {matched++}
				}
			}
			t.Logf("AGE OAM write compact: model=%s ROM=%s groups_matched=%d/%d",tc.model,tc.rom,matched,total)
		})
	}
}
