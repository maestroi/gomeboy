//go:build test

package tests

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// The pinned AGE window-mode tests record eight Mode 0/3 CPU STAT samples
// for each of 16 WX phases and 4 (normal) or 6 (double) NOP offsets.
// Recover the compiled ROM expectation using a three-group fingerprint,
// then validate its uncompressed WRAM values against the separately
// rendered compact first-divergence code for every WX/NOP group.
func ageWindowSTATPrefix(doubleSpeed bool) []byte {
	prefix := []byte{
		0xa3,0xa3,0xa3,0xa3,0xa3,0xa3,0xa3,0xa3,
		0xa0,0xa3,0xa3,0xa3,0xa3,0xa3,0xa3,0xa3,
	}
	if doubleSpeed {
		return append(prefix, []byte{0xa0,0xa0,0xa3,0xa3,0xa3,0xa3,0xa3,0xa3}...)
	}
	return append(prefix, []byte{0xa0,0xa0,0xa0,0xa0,0xa0,0xa3,0xa3,0xa3}...)
}

func TestAgeWindowSTATRawDiagnostics(t *testing.T) {
	if _,err:=os.Stat(ageROMPath);err!=nil {t.Skipf("AGE corpus missing: %v",err)}
	for _,tc:=range []struct{
		rom string
		model types.Model
	}{
		{"stat-mode-window-cgbBCE.gb", types.CGBBC},
		{"stat-mode-window-cgbBCE.gb", types.CGBDE},
		{"stat-mode-window-dmgC.gb", types.DMGABC},
		{"stat-mode-window-ds-cgbBCE.gb", types.CGBBC},
		{"stat-mode-window-ds-cgbBCE.gb", types.CGBDE},
		{"stat-mode-window-ncmBCE.gb", types.CGBBC},
		{"stat-mode-window-ncmBCE.gb", types.CGBDE},
	}{
		t.Run(tc.rom+"@"+tc.model.String(),func(t *testing.T){
			g,err:=runGameboy(filepath.Join(ageROMPath,"stat-mode-window",tc.rom),
				5,DebugBreakpoint,gameboy.Debug(),gameboy.AsModel(tc.model))
			if err!=nil {t.Fatal(err)}
			if !g.CPU.DebugBreakpoint {t.Fatal("AGE window ROM did not reach breakpoint")}
			ds:=strings.Contains(tc.rom,"-ds-")
			cols:=4
			if ds {cols=6}
			const rows=16
			const groupSize=8
			total:=rows*cols*groupSize
			prefix:=ageWindowSTATPrefix(ds)
			var verdicts [rows][6]byte
			for row:=0;row<rows;row++ {
				v,_,ok:=ageCompactResultsFromBG(g,row)
				if !ok {t.Fatalf("compact AGE row %d undecodable",row)}
				copy(verdicts[row][:],v[:cols])
			}

			bestAt, bestAgree, bestSamples, candidates:=-1,-1,-1,0
			for start:=0;start<len(g.ROM); {
				found:=bytes.Index(g.ROM[start:],prefix)
				if found<0 {break}
				at:=start+found
				start=at+1
				if at+total>len(g.ROM) {continue}
				candidates++
				agree,matched:=0,0
				for row:=0;row<rows;row++ {
					for col:=0;col<cols;col++ {
						firstBad:=byte(0xff)
						for i:=0;i<groupSize;i++ {
							offset:=(row*cols+col)*groupSize+i
							if g.ROM[at+offset]==g.Bus.Get(uint16(0xc000+offset)) {
								matched++
							} else if firstBad==0xff {firstBad=byte(i)}
						}
						if firstBad==verdicts[row][col] {agree++}
					}
				}
				if agree>bestAgree || agree==bestAgree && matched>bestSamples {
					bestAt,bestAgree,bestSamples=at,agree,matched
				}
			}
			if bestAt<0 || bestAgree!=rows*cols {
				t.Logf("AGE window STAT matrix unverified: model=%s rom=%s candidates=%d compact_agreement=%d/%d; no raw score",
					tc.model,tc.rom,candidates,bestAgree,rows*cols)
				return
			}
			t.Logf("AGE window STAT raw matrix: model=%s rom=%s rom_offset=%#x wram=0xc000 samples_matched=%d/%d compact_agreement=%d/%d",
				tc.model,tc.rom,bestAt,bestSamples,total,bestAgree,rows*cols)
			for row:=0;row<rows;row++ {
				groupBad:=0
				badSamples:=0
				firstGroups:=make([]string,0,cols)
				for col:=0;col<cols;col++ {
					firstBad:=-1
					bad:=0
					for i:=0;i<groupSize;i++ {
						offset:=(row*cols+col)*groupSize+i
						if g.ROM[bestAt+offset]!=g.Bus.Get(uint16(0xc000+offset)){
							bad++
							if firstBad<0 {firstBad=i}
						}
					}
					if bad>0 {
						groupBad++
						badSamples+=bad
						firstGroups=append(firstGroups,fmtWindowFirstBad(col,firstBad,bad))
					}
				}
				if groupBad>0 {
					wx:=row
					if row>=10 {wx=row-10+162}
					t.Logf("AGE window STAT divergence: model=%s rom=%s WX=%d groups_bad=%d/%d samples_bad=%d first_bad_by_probe=%v",
						tc.model,tc.rom,wx,groupBad,cols,badSamples,firstGroups)
				}
			}
		})
	}
}

func fmtWindowFirstBad(probe,first,bad int) string {
	return fmt.Sprintf("nop+%d:first=%d,bad=%d",probe,first,bad)
}
