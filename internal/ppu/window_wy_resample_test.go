package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/types"
)

// After CGB clears the WY latch on WIN_EN fall, turning the window back
// on must re-check LY==WY so later lines can show the window again.
func TestCGBWindowEnableResamplesWY(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(model)
			p.b.Set(types.LCDC, types.Bit7) // LCD on, WIN_EN off
			p.winEnabled = false
			p.winTriggerWy = false
			p.wy = 40
			p.ly = 40
			p.b.Write(types.LCDC, types.Bit7|types.Bit5)
			if !p.winEnabled || !p.winTriggerWy {
				t.Fatalf("WIN_EN set at LY==WY did not re-latch: en=%t wy=%t",
					p.winEnabled, p.winTriggerWy)
			}
		})
	}
}

func TestDMGWindowEnableResamplesWY(t *testing.T) {
	p := newWindowWXTriggerTestPPU(types.DMGABC)
	p.b.Set(types.LCDC, types.Bit7)
	p.winEnabled = false
	p.winTriggerWy = false
	p.wy = 10
	p.ly = 10
	p.b.Write(types.LCDC, types.Bit7|types.Bit5)
	if !p.winTriggerWy {
		t.Fatal("DMG WIN_EN set at LY==WY should latch WY")
	}
}

// Mid-Mode-3 WIN_EN clear on CGB must keep the WY latch so the window
// can resume after the current tile finishes (Mealybug WIN_EN docs).
func TestCGBKeepsWYLatchAcrossMidMode3WinEnClear(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(model)
			p.b.Set(types.LCDC, types.Bit7|types.Bit5)
			p.winEnabled = true
			p.winTriggerWy = true
			p.mode = ModeVRAM
			p.b.Write(types.LCDC, types.Bit7)
			if p.winTriggerWy {
				return
			}
			t.Fatal("mid-Mode-3 WIN_EN clear dropped CGB WY latch")
		})
	}
}

// Mode-2 WIN_EN clear on CGB must also keep the WY latch (Mealybug
// m2_win_en_toggle toggles the window every Mode-2 IRQ).
func TestCGBKeepsWYLatchAcrossMode2WinEnClear(t *testing.T) {
	for _, model := range []types.Model{types.CGBBC, types.CGBDE} {
		t.Run(model.String(), func(t *testing.T) {
			p := newWindowWXTriggerTestPPU(model)
			p.b.Set(types.LCDC, types.Bit7|types.Bit5)
			p.winEnabled = true
			p.winTriggerWy = true
			p.mode = ModeOAM
			p.b.Write(types.LCDC, types.Bit7)
			if !p.winTriggerWy {
				t.Fatal("Mode-2 WIN_EN clear dropped CGB WY latch")
			}
		})
	}
}
