package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The special 0xffff comparison value is not a real LY value. Although
// uint8(0xffff) == 0xff, STAT.2 must not claim LY=LYC when LYC=0xff.
func TestSTATCoincidenceInvalidLYComparisonSentinel(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC,types.MGB,types.CGBBC,types.CGBDE} {
		for _, doubleSpeed := range []bool{false,true} {
			t.Run(model.String()+"/double="+map[bool]string{false:"false",true:"true"}[doubleSpeed],func(t *testing.T){
				s:=scheduler.NewScheduler()
				b:=gbio.NewBus(s,make([]byte,0x8000))
				b.Map(model)
				p:=New(b,s)
				s.ChangeSpeed(doubleSpeed)
				p.enabled=true
				p.lyCompare=0xff
				p.lyForComparison=0xffff
				p.mode=ModeVBlank
				p.modeToInt=ModeVBlank
				p.status=0
				p.statUpdate()
				if got:=b.Read(types.STAT)&types.Bit2;got!=0 {
					t.Fatalf("invalid LY comparator falsely asserted coincidence: STAT=%02x",b.Read(types.STAT))
				}

				// A genuine 8-bit LY=255 is still a match, proving the
				// sentinel check does not suppress actual comparisons.
				p.lyForComparison=255
				p.statUpdate()
				if got:=b.Read(types.STAT)&types.Bit2;got==0 {
					t.Fatalf("valid LY=255, LYC=255 did not assert coincidence")
				}

				// On normal-speed DMG and physical CGB, the
				// sentinel phase updates the coincidence flag.
				// MGB and synthetic double-speed fixtures preserve
				// the previous bit in this phase; seed it low and
				// assert they cannot manufacture a false match.
				if !(model==types.DMGABC && !doubleSpeed ||
					model.IsCGB() && !doubleSpeed) {
					p.status &^= types.Bit2
				}
				p.lyForComparison=0xffff
				p.statUpdate()
				if got:=b.Read(types.STAT)&types.Bit2;got!=0 {
					t.Fatalf("invalid LY comparison produced coincidence: STAT=%02x",b.Read(types.STAT))
				}

				// Re-enabling the comparison with a different LY
				// must not manufacture a coincidence interrupt.
				p.lyForComparison=153
				p.statUpdate()
				if got:=b.Read(types.STAT)&types.Bit2;got!=0 {
					t.Fatalf("mismatched LY=153, LYC=255 asserted coincidence")
				}
			})
		}
	}
}
