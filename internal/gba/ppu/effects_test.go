package ppu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func setupTwoTextBGs(t *testing.T, b *bus.Bus) {
	t.Helper()
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f) // red
	setBGPaletteColor(b, 2, 0x03e0) // green

	set4bppPixel(vram, 0, 1, 0, 0, 1)
	set4bppPixel(vram, 0, 1, 1, 0, 1)
	setScreenEntry(vram, 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 0|(8<<8), bus.Access{})

	set4bppPixel(vram, 1, 1, 0, 0, 2)
	set4bppPixel(vram, 1, 1, 1, 0, 2)
	setScreenEntry(vram, 10, 0, 1)
	b.Write16(bus.IOStart+0x00a, 1|(1<<2)|(10<<8), bus.Access{})

	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|(1<<9), bus.Access{})
}

func TestColorEffectRegisters(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})

	b.Write16(bus.IOStart+0x050, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+0x052, 0xffff, bus.Access{})
	b.Write16(bus.IOStart+0x054, 0xffff, bus.Access{})

	if got, _ := b.Read16(bus.IOStart+0x050, bus.Access{}); got != 0x3fff {
		t.Fatalf("BLDCNT = %04x, want 3fff", got)
	}
	if got, _ := b.Read16(bus.IOStart+0x052, bus.Access{}); got != 0x1f1f {
		t.Fatalf("BLDALPHA = %04x, want 1f1f", got)
	}
	if p.bldy != 0x001f {
		t.Fatalf("BLDY stored = %04x, want 001f", p.bldy)
	}

	b.SetOpenBus(0x44332211)
	if got, _ := b.Read16(bus.IOStart+0x054, bus.Access{}); got != 0x2211 {
		t.Fatalf("BLDY read = %04x, want open-bus low lane 2211", got)
	}

	if p.alphaA() != 16 || p.alphaB() != 16 || p.brightnessY() != 16 {
		t.Fatalf("coefficients not clamped to 16: A=%d B=%d Y=%d", p.alphaA(), p.alphaB(), p.brightnessY())
	}
}

func TestAlphaBlendTopAndImmediateSecondTarget(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setupTwoTextBGs(t, b)

	// BG0 is first target, BG1 second target, 8/16 + 8/16.
	b.Write16(bus.IOStart+0x050,
		(1<<0)|(effectAlpha<<6)|(1<<9),
		bus.Access{})
	b.Write16(bus.IOStart+0x052, 8|(8<<8), bus.Access{})

	p.Advance(VisibleCycles)

	want := [3]byte{123, 123, 0} // 15/31 red + 15/31 green
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != want {
		t.Fatalf("alpha blend = %v, want %v", got, want)
	}
}

func TestAlphaBlendDoesNotSkipBlockingLayer(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	vram := b.VRAM()

	setBGPaletteColor(b, 1, 0x001f) // BG0 red
	setBGPaletteColor(b, 2, 0x03e0) // BG1 green blocker
	setBGPaletteColor(b, 3, 0x7c00) // BG2 blue second-target candidate

	for bg := 0; bg < 3; bg++ {
		charBlock := bg
		screenBlock := 8 + bg*2
		set4bppPixel(vram, charBlock, 1, 0, 0, byte(bg+1))
		setScreenEntry(vram, screenBlock, 0, 1)
		b.Write16(bus.IOStart+uint32(0x008+bg*2),
			uint16(bg)|(uint16(charBlock)<<2)|(uint16(screenBlock)<<8),
			bus.Access{})
	}

	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|(1<<9)|(1<<10), bus.Access{})
	// BG0 first target, BG2 second target. BG1 is visible between them but is
	// deliberately not a second target, so hardware does not skip past it.
	b.Write16(bus.IOStart+0x050,
		(1<<0)|(effectAlpha<<6)|(1<<10),
		bus.Access{})
	b.Write16(bus.IOStart+0x052, 8|(8<<8), bus.Access{})

	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("blend skipped blocking BG1: got %v, want top BG0 red", got)
	}
}

func TestSemiTransparentOBJForcesAlphaBlend(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	// BG0 blue beneath a red semi-transparent OBJ.
	setBGPaletteColor(b, 1, 0x7c00)
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 2, 0, 0, 1)
	setOBJAttrs(b, 0, 1<<10, 0, 2)

	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|dispOBJEnable, bus.Access{})

	// Effect mode is "none" and OBJ is not selected as a first target. A
	// semi-transparent OBJ still forces alpha if BG0 is a second target.
	b.Write16(bus.IOStart+0x050, 1<<8, bus.Access{})
	b.Write16(bus.IOStart+0x052, 8|(8<<8), bus.Access{})

	p.Advance(VisibleCycles)

	want := [3]byte{123, 0, 123}
	if got := rgbAt(p.FrameBuffer(), 0, 0); got != want {
		t.Fatalf("semi-transparent OBJ = %v, want %v", got, want)
	}
}

func TestSemiTransparentOBJFallsBackToBrightnessWithoutSecondTarget(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setOBJPaletteColor(b, 1, 0x0010) // red intensity 16
	setOBJ4bppPixel(b, 0, 0, 0, 1)
	setOBJAttrs(b, 0, 1<<10, 0, 0)

	b.Write16(bus.IOStart+dispCNTOffset, dispOBJEnable, bus.Access{})
	// Brighten OBJ as a normal first target; backdrop is not selected as a
	// second target, so forced semi-transparency has nothing to blend with.
	b.Write16(bus.IOStart+0x050,
		(1<<4)|(effectBrighten<<6),
		bus.Access{})
	b.Write16(bus.IOStart+0x054, 8, bus.Access{})

	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{189, 0, 0} {
		t.Fatalf("semi-transparent brightness fallback = %v, want [189 0 0]", got)
	}
}

func TestBrightnessIncreaseAndDecrease(t *testing.T) {
	base := bgr555(0x4210) // each channel intensity 16

	if got := brighten(base, 8); got != [3]byte{189, 189, 189} {
		t.Fatalf("brighten 16 by 8/16 = %v, want [189 189 189]", got)
	}
	if got := darken(base, 8); got != [3]byte{66, 66, 66} {
		t.Fatalf("darken 16 by 8/16 = %v, want [66 66 66]", got)
	}
	if got := brighten(base, 31); got != [3]byte{255, 255, 255} {
		t.Fatalf("brighten clamped coefficient = %v, want white", got)
	}
	if got := darken(base, 31); got != [3]byte{0, 0, 0} {
		t.Fatalf("darken clamped coefficient = %v, want black", got)
	}
}

func TestBrightnessOnlyAffectsSelectedFirstTarget(t *testing.T) {
	render := func(firstTarget bool) [3]byte {
		p, b := newTestPPU(t, Hooks{})
		setBGPaletteColor(b, 1, 0x0010)
		set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
		setScreenEntry(b.VRAM(), 8, 0, 1)
		b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})
		b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})

		cnt := uint16(effectBrighten << 6)
		if firstTarget {
			cnt |= 1 << 0
		}
		b.Write16(bus.IOStart+0x050, cnt, bus.Access{})
		b.Write16(bus.IOStart+0x054, 8, bus.Access{})
		p.Advance(VisibleCycles)
		return rgbAt(p.FrameBuffer(), 0, 0)
	}

	if got := render(false); got != [3]byte{132, 0, 0} {
		t.Fatalf("unselected brightness target changed: %v", got)
	}
	if got := render(true); got != [3]byte{189, 0, 0} {
		t.Fatalf("selected brightness target = %v, want [189 0 0]", got)
	}
}

func TestWindowEffectBitGatesColorEffects(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setupTwoTextBGs(t, b)

	setWindowRect(b, 0, 0, 1, 0, 1)
	// Inside WIN0 both BGs are visible but effects are disabled. Outside both
	// BGs are visible and effects are enabled.
	b.Write16(bus.IOStart+0x048, windowBG0|windowBG1, bus.Access{})
	b.Write16(bus.IOStart+0x04a, windowBG0|windowBG1|windowEffect, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset,
		(1<<8)|(1<<9)|dispWIN0Enable,
		bus.Access{})

	b.Write16(bus.IOStart+0x050,
		(1<<0)|(effectAlpha<<6)|(1<<9),
		bus.Access{})
	b.Write16(bus.IOStart+0x052, 8|(8<<8), bus.Access{})

	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{255, 0, 0} {
		t.Fatalf("window-disabled effect = %v, want red", got)
	}
	if got := rgbAt(p.FrameBuffer(), 1, 0); got != [3]byte{123, 123, 0} {
		t.Fatalf("outside-window effect = %v, want blended yellow", got)
	}
}

func TestBackdropCanBeSecondTarget(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	setBGPaletteColor(b, 0, 0x7c00) // blue backdrop
	setBGPaletteColor(b, 1, 0x001f) // red BG0
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 8<<8, bus.Access{})
	b.Write16(bus.IOStart+dispCNTOffset, 1<<8, bus.Access{})

	b.Write16(bus.IOStart+0x050,
		(1<<0)|(effectAlpha<<6)|(1<<13),
		bus.Access{})
	b.Write16(bus.IOStart+0x052, 8|(8<<8), bus.Access{})
	p.Advance(VisibleCycles)

	if got := rgbAt(p.FrameBuffer(), 0, 0); got != [3]byte{123, 0, 123} {
		t.Fatalf("BG/backdrop alpha = %v, want purple blend", got)
	}
}

func TestEqualPriorityOBJRemainsAboveBGInEffectStack(t *testing.T) {
	p, b := newTestPPU(t, Hooks{})
	disableAllOBJ(b)

	setBGPaletteColor(b, 1, 0x7c00)
	set4bppPixel(b.VRAM(), 0, 1, 0, 0, 1)
	setScreenEntry(b.VRAM(), 8, 0, 1)
	b.Write16(bus.IOStart+0x008, 1|(8<<8), bus.Access{})

	setOBJPaletteColor(b, 1, 0x001f)
	setOBJ4bppPixel(b, 2, 0, 0, 1)
	setOBJAttrs(b, 0, 0, 0, (1<<10)|2)

	b.Write16(bus.IOStart+dispCNTOffset, (1<<8)|dispOBJEnable, bus.Access{})

	stack, count := p.visibleLayerStack(0, 0, 0, windowAll)
	if count < 2 || stack[0].layer != layerOBJ || stack[1].layer != layerBG0 {
		t.Fatalf("equal-priority stack = %#v count=%d, want OBJ then BG0", stack[:count], count)
	}
}
