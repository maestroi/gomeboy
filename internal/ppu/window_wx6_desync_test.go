package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

func TestDMGWX6WriteSuppressesSameLineOpen(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.mode = ModeOAM
	p.b.Write(types.WX, 6)
	if !p.wx6SuppressLine {
		t.Fatal("WX=6 write should arm same-line suppress")
	}
	p.mode = ModeVRAM
	p.lx = 7
	p.checkWindowTriggerWX()
	if p.winTriggerWx {
		t.Fatal("same-line DMG WX=6 must not open at LX=7")
	}
	if p.wx6PoisonedLine {
		t.Fatal("LX=7 must not poison yet; Mode-3 may rewrite WX at LX=8")
	}
	p.lx = 8
	p.checkWindowTriggerWX()
	if p.winTriggerWx {
		t.Fatal("visible LX=8 with WX still 6 must not open")
	}
	if !p.wx6PoisonedLine {
		t.Fatal("reaching visible WX=6 column under suppress should poison")
	}
	p.wx = 80
	p.lx = 81
	p.checkWindowTriggerWX()
	if p.winTriggerWx {
		t.Fatal("poisoned line must not open at later WX")
	}
}

func TestDMGWX6LateDepartureDesyncs(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.mode = ModeOAM
	p.b.Write(types.WX, 6)
	p.mode = ModeVRAM
	p.lx = 8
	p.checkWindowTriggerWX()
	p.lx = 100
	p.b.Write(types.WX, 80)
	if !p.wx6DesyncLine {
		t.Fatal("late departure from WX=6 while poisoned must desync")
	}
	if p.PreparedFrame[p.ly][0] != p.ColourPalette[0][3] {
		t.Fatal("desync should fill the scanline with BGP colour 3")
	}
}

func TestDMGWX6EarlyDepartureNoDesync(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.mode = ModeOAM
	p.b.Write(types.WX, 6)
	p.mode = ModeVRAM
	p.lx = 7
	p.checkWindowTriggerWX()
	p.lx = 8
	p.b.Write(types.WX, 4)
	if p.wx6DesyncLine {
		t.Fatal("early WX=LY departure must not desync")
	}
	if p.wx6PoisonedLine || p.wx6SuppressLine {
		t.Fatal("early rewrite must clear poison/suppress for staircase open")
	}
	p.checkWindowTriggerWX()
	if !p.winTriggerWx {
		t.Fatal("early rewrite to WX=4 must open at LX=8")
	}
}

func TestDMGPresetWX6OpensWithoutSameLineWrite(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.wx = 6
	p.wx6SuppressLine = false
	p.lx = 7
	p.checkWindowTriggerWX()
	if !p.winTriggerWx {
		t.Fatal("preset WX=6 must still open at LX=7")
	}
}

func TestCGBWX6WriteDoesNotSuppress(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.CGBDE)
	p.mode = ModeOAM
	p.b.Write(types.WX, 6)
	if p.wx6SuppressLine {
		t.Fatal("CGB must not arm DMG WX=6 suppress")
	}
	p.mode = ModeVRAM
	p.lx = 7
	p.checkWindowTriggerWX()
	if !p.winTriggerWx {
		t.Fatal("CGB WX=6 must open at LX=7")
	}
}
