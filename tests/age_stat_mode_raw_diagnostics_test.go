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

// Read the pinned AGE stat-mode ROM's uncompressed CPU STAT timing samples.
// Match the expected ROM bytes against WRAM independently of the hex-tile
// compact verdict. Do not change emulator timing or the regression baseline.
func TestAgeSTATModeRawDiagnostics(t *testing.T) {
	if _,err:=os.Stat(ageROMPath);err!=nil {t.Skipf("AGE corpus missing: %v",err)}
	for _,tc:=range []struct{rom string;model types.Model}{
		{"stat-mode-dmgC-cgbBC.gb",types.DMGABC},
		{"stat-mode-dmgC-cgbBC.gb",types.CGBBC},
		{"stat-mode-cgbE.gb",types.CGBDE},
		{"stat-mode-ds-cgbBCE.gb",types.CGBBC},
		{"stat-mode-ds-cgbBCE.gb",types.CGBDE},
		{"stat-mode-ncmBC.gb",types.CGBBC},
		{"stat-mode-ncmE.gb",types.CGBDE},
	}{
		t.Run(tc.rom+"@"+tc.model.String(),func(t *testing.T){
			g,err:=runGameboy(filepath.Join(ageROMPath,"stat-mode",tc.rom),
				5,DebugBreakpoint,gameboy.Debug(),gameboy.AsModel(tc.model))
			if err!=nil {t.Fatal(err)}
			if !g.CPU.DebugBreakpoint {t.Fatal("AGE ROM did not reach breakpoint")}
			var verdict [40]byte
			for row:=0;row<14;row++{
				v,_,ok:=ageCompactResultsFromBG(g,row)
				if !ok {t.Fatalf("cannot decode compact row %d",row)}
				n:=3;if row>=12 {n=2}
				for col:=0;col<n;col++{
					i:=row*3+col
					if row==13 {i--} // row 12 has only two probes
					verdict[i]=v[col]
				}
			}
			// Pinned first twelve expected STAT reads on the first
			// visible scanlines (SCX=0, Mode-0 transition probe).
			prefix:=[]byte{0x83,0x83,0x83,0x83,0x83,0x83,0x81,0x81,0x81,0x81,0x83,0x83}
			const n=40*12
			bestAgreement,bestMatches,bestROM,bestRAM,candidates:=-1,-1,-1,uint16(0),0
			for at:=0;at<len(g.ROM);{
				k:=bytes.Index(g.ROM[at:],prefix)
				if k<0 {break}
				at+=k
				if at+n>len(g.ROM) {at++;continue}
				for raw:=0xc000;raw+n<=0xd000;raw++{
					prefixMatches:=0
					for i,want:=range prefix {
						if g.Bus.Get(uint16(raw+i))==want {prefixMatches++}
					}
					if prefixMatches<9 {continue}
					candidates++
					matches,agree:=0,0
					for group:=0;group<40;group++{
						first:=byte(0xff)
						for j:=0;j<12;j++{
							off:=group*12+j
							if g.Bus.Get(uint16(raw+off))==g.ROM[at+off] {
								matches++
							}else if first==0xff {first=byte(j)}
						}
						if first==verdict[group] {agree++}
					}
					if agree>bestAgreement || agree==bestAgreement&&matches>bestMatches {
						bestAgreement,bestMatches,bestROM,bestRAM=agree,matches,at,uint16(raw)
					}
				}
				at++
			}
			if bestROM<0 || bestAgreement!=40 {
				t.Fatalf("cannot identify raw STAT timing matrix candidates=%d agreement=%d/40 samples=%d/480",candidates,bestAgreement,bestMatches)
			}
			t.Logf("AGE raw STAT matrix: model=%s rom=%s rom_offset=%#x wram=%#x samples_matched=%d/480 compact_agreement=%d/40",
				tc.model,tc.rom,bestROM,bestRAM,bestMatches,bestAgreement)
			for row:=0;row<14;row++{
				n:=3;if row>=12 {n=2}
				for col:=0;col<n;col++{
					group:=row*3+col
					if row==13 {group--}
					if verdict[group]==0xff {continue}
					var expected,observed [12]byte
					for j:=0;j<12;j++{
						expected[j]=g.ROM[bestROM+group*12+j]
						observed[j]=g.Bus.Get(bestRAM+uint16(group*12+j))
					}
					t.Logf("AGE raw STAT divergence: model=%s rom=%s row=%d probe=%d first_bad=%d expected=% x observed=% x",
						tc.model,tc.rom,row,col,verdict[group],expected,observed)
				}
			}
		})
	}
}
