package ppu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// AGE NCM probes use a DMG-only cartridge on physical CGB hardware. The PPU
// may use a DMG pixel pipeline while the CPU still sees CGB VRAM bus timing.
func TestNCMVRAMReadRemainsOpenAtLateMode2Edge(t *testing.T) {
	for _, model := range []types.Model{types.DMGABC, types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000)) // DMG-only cartridge
			b.Map(model)
			p := New(b, s)
			p.enabled = true
			p.cgbMode = false
			p.ly = 1
			p.mode = ModeOAM
			p.lineState = ReleaseOAMBus
			p.oamScanIndex = 38
			b.Write(0x8000, 0x5a)
			b.RLock(gbio.VRAM)

			// Dot 76: PPU performs the 39th OAM scan and changes its
			// CPU-facing VRAM port ownership.
			p.handleVisualLine()
			expected := byte(0x5a)
			if !model.IsCGB() {
				expected = 0xff
			}
			if got := b.Read(0x8000); got != expected {
				t.Fatalf("late Mode-2 VRAM read: got %02x, expected %02x", got, expected)
			}

			// Dot 78 still scans OAM; neither physical CGB family should
			// lose VRAM read access until pixel transfer starts at dot 80.
			s.Tick(2)
			if got := b.Read(0x8000); got != expected {
				t.Fatalf("VRAM read changed at dot 78: got %02x, expected %02x", got, expected)
			}

			s.Tick(2) // dot 80 begins Mode 3, closing both VRAM ports
			if got := b.Read(0x8000); got != 0xff {
				t.Fatalf("Mode-3 VRAM read unexpectedly open: %02x", got)
			}
		})
	}
}
