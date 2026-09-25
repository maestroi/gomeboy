// Package ppu implements the Game Boy Advance LCD controller.
package ppu

import (
	"github.com/maestroi/gomeboy/internal/gba/bus"
)

const (
	ScreenWidth  = 240
	ScreenHeight = 160
	FrameBytes   = ScreenWidth * ScreenHeight * 3

	VisibleCycles     uint32 = 960
	HBlankFlagCycle  uint32 = 1006
	CyclesPerLine    uint32 = 1232
	VisibleLines      uint16 = 160
	ScanlinesPerFrame uint16 = 228

	VBlankStartLine uint16 = 160
	VBlankEndLine   uint16 = 227 // flag is clear on line 227
)

const (
	dispCNTOffset  uint32 = 0x000
	dispSTATOffset uint32 = 0x004
	vcountOffset   uint32 = 0x006
)

// IRQSource identifies one LCD interrupt request.
type IRQSource uint8

const (
	IRQVBlank IRQSource = iota
	IRQHBlank
	IRQVCount
)

// Hooks connect timing edges to the future DMA/interrupt controller.
//
// HBlank is a scanline blanking trigger and is emitted for all 228 scanlines.
// HBlank IRQ requests themselves are suppressed during VBlank, matching GBA
// LCD behavior. VBlank is emitted on entry to scanline 160.
type Hooks struct {
	HBlank func()
	VBlank func()
	IRQ    func(IRQSource)
}

// PPU owns LCD timing, display registers, and the 240x160 RGB framebuffer.
type PPU struct {
	bus   *bus.Bus
	hooks Hooks

	dispcnt  uint16
	dispstat uint16 // writable control bits only; status bits are synthesized
	vcount   uint16

	lineCycle uint32
	hblank    bool
	vblank    bool
	vcounter  bool
	frames    uint64

	frame [FrameBytes]byte
}

// New creates a GBA PPU and maps its LCD status/control registers onto b.
func New(b *bus.Bus, hooks Hooks) *PPU {
	if b == nil {
		panic("gba ppu: nil bus")
	}
	p := &PPU{bus: b, hooks: hooks}
	p.installRegisters()
	p.updateVCountMatch(false)
	return p
}

// Reset resets LCD timing and display-control state while preserving hooks and
// the attached bus.
func (p *PPU) Reset() {
	p.dispcnt = 0
	p.dispstat = 0
	p.vcount = 0
	p.lineCycle = 0
	p.hblank = false
	p.vblank = false
	p.vcounter = true // default compare value is 0
	p.frames = 0
	clear(p.frame[:])
	p.bus.SetOBJVRAMStart(0x10000)
}

// Advance advances LCD timing by master-clock cycles. It may cross any number
// of scanline/frame boundaries.
func (p *PPU) Advance(cycles uint32) {
	for cycles > 0 {
		next := CyclesPerLine
		switch {
		case p.lineCycle < VisibleCycles:
			next = VisibleCycles
		case p.lineCycle < HBlankFlagCycle:
			next = HBlankFlagCycle
		}

		step := next - p.lineCycle
		if cycles < step {
			p.lineCycle += cycles
			return
		}
		p.lineCycle += step
		cycles -= step

		switch p.lineCycle {
		case VisibleCycles:
			p.beginHBlank()
		case HBlankFlagCycle:
			p.setHBlankFlag()
		case CyclesPerLine:
			p.endScanline()
		}
	}
}

func (p *PPU) beginHBlank() {
	if p.vcount < VisibleLines {
		p.renderLine(int(p.vcount))
	}
	if p.hooks.HBlank != nil {
		p.hooks.HBlank()
	}
}

func (p *PPU) setHBlankFlag() {
	p.hblank = true
	if !p.vblank && p.dispstat&(1<<4) != 0 && p.hooks.IRQ != nil {
		p.hooks.IRQ(IRQHBlank)
	}
}

func (p *PPU) endScanline() {
	p.lineCycle = 0
	p.hblank = false

	p.vcount++
	if p.vcount == ScanlinesPerFrame {
		p.vcount = 0
		p.frames++
	}

	wasVBlank := p.vblank
	p.vblank = p.vcount >= VBlankStartLine && p.vcount < VBlankEndLine

	if !wasVBlank && p.vblank {
		if p.hooks.VBlank != nil {
			p.hooks.VBlank()
		}
		if p.dispstat&(1<<3) != 0 && p.hooks.IRQ != nil {
			p.hooks.IRQ(IRQVBlank)
		}
	}

	p.updateVCountMatch(true)
}

// FrameBuffer returns a zero-copy RGB24 view. It is overwritten as visible
// scanlines are rendered.
func (p *PPU) FrameBuffer() []byte { return p.frame[:] }

func (p *PPU) FrameCount() uint64 { return p.frames }
func (p *PPU) VCount() uint16     { return p.vcount }
func (p *PPU) LineCycle() uint32  { return p.lineCycle }
func (p *PPU) InHBlank() bool     { return p.hblank }
func (p *PPU) InVBlank() bool     { return p.vblank }
func (p *PPU) DISPControl() uint16 { return p.dispcnt }
func (p *PPU) DISPStatus() uint16  { return p.readDISPSTAT() }

func (p *PPU) updateVCountMatch(requestIRQ bool) {
	compare := uint16(p.dispstat >> 8)
	match := p.vcount == compare
	rising := match && !p.vcounter
	p.vcounter = match
	if requestIRQ && rising && p.dispstat&(1<<5) != 0 && p.hooks.IRQ != nil {
		p.hooks.IRQ(IRQVCount)
	}
}
