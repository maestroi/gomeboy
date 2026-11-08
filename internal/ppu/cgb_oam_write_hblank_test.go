package ppu

import (
	"fmt"
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func TestCGBOAMWriteHBlankReleaseDelay(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		for _, doubleSpeed := range []bool{false, true} {
			name := fmt.Sprintf("%s/double=%t", model, doubleSpeed)
			t.Run(name, func(t *testing.T) {
				s := scheduler.NewScheduler()
				rom := make([]byte, 0x8000)
				rom[0x143] = 0x80
				b := gbio.NewBus(s, rom)
				b.Map(model)
				p := New(b, s)
				if doubleSpeed {
					s.ChangeSpeed(true)
				}
				p.enabled = true
				p.cgbMode = model.IsCGB()
				p.mode = ModeHBlank
				p.lineState = HBlankUpdateLY
				b.Set(0xfe00, 0x5a)
				p.startOAMWriteHBlankRelease()

				delayExpected := uint64(0)
				if model.IsCGB() && doubleSpeed {
					delayExpected = 2
				}
				if got := p.oamCPUWriteReadyAt - s.Cycle(); got != delayExpected {
					t.Fatalf("CPU OAM write release delayed %d cycles, wanted %d", got, delayExpected)
				}
				b.Write(0xfe00, 0xa5)
				want := byte(0xa5)
				if delayExpected != 0 {
					want = 0x5a
				}
				if got := b.Get(0xfe00); got != want {
					t.Fatalf("OAM write at HBlank unlock: got %02x, expected %02x", got, want)
				}
				if delayExpected != 0 {
					s.Tick(1)
					b.Write(0xfe00, 0x77)
					if got := b.Get(0xfe00); got != 0x5a {
						t.Fatalf("OAM writes reopened before one PPU dot: %02x", got)
					}
					s.Tick(1)
					b.Write(0xfe00, 0xcc)
					if got := b.Get(0xfe00); got != 0xcc {
						t.Fatalf("OAM write remained blocked after release edge: %02x", got)
					}
				}

				b.WLock(gbio.OAM)
				b.Write(0xfe00, 0x00)
				if got := b.Get(0xfe00); got != want && delayExpected == 0 {
					t.Fatalf("coarse OAM lock was bypassed: %02x", got)
				}
			})
		}
	}
}

func TestCGBOAMWriteHBlankReleaseRestoresAcrossStateAndCheckpoint(t *testing.T) {
	s := scheduler.NewScheduler()
	rom := make([]byte, 0x8000)
	rom[0x143] = 0x80
	b := gbio.NewBus(s, rom)
	b.Map(types.CGBDE)
	p := New(b, s)
	s.ChangeSpeed(true)
	p.enabled = true
	p.cgbMode = true
	p.mode = ModeHBlank
	p.lineState = HBlankUpdateLY
	p.startOAMWriteHBlankRelease()

	snap := p.Snapshot()
	if snap.OAMCPUWriteReadyAt != p.oamCPUWriteReadyAt {
		t.Fatalf("Snapshot lost OAM write release phase")
	}
	var checkpoint State
	p.SnapshotInto(&checkpoint)
	if checkpoint.OAMCPUWriteReadyAt != p.oamCPUWriteReadyAt {
		t.Fatalf("SnapshotInto lost OAM write release phase")
	}
	p.oamCPUWriteReadyAt = 0
	p.Restore(checkpoint)
	if p.oamCPUWriteReadyAt != checkpoint.OAMCPUWriteReadyAt {
		t.Fatalf("Restore lost OAM write release phase")
	}
}
