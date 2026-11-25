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
