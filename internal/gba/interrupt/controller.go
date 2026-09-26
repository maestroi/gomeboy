// Package interrupt implements the Game Boy Advance interrupt controller.
package interrupt

import "github.com/maestroi/gomeboy/internal/gba/bus"

const (
	ieOffset  uint32 = 0x200
	ifOffset  uint32 = 0x202
	imeOffset uint32 = 0x208

	validMask uint16 = 0x3fff
)

// Source identifies one GBA interrupt source bit shared by IE and IF.
type Source uint16

const (
	VBlank Source = 1 << iota
	HBlank
	VCount
	Timer0
	Timer1
	Timer2
	Timer3
	Serial
	DMA0
	DMA1
	DMA2
	DMA3
	Keypad
	GamePak
)

// LineSink receives the controller's resolved level-sensitive IRQ output.
type LineSink interface {
	SetIRQLine(bool)
}

// Controller owns IE, IF, and IME and resolves them to the CPU IRQ line.
type Controller struct {
	sink LineSink

	ie    uint16
	flags uint16
	ime   bool
}

// New maps IE/IF/IME onto b and optionally drives sink.
func New(b *bus.Bus, sink LineSink) *Controller {
	if b == nil {
		panic("gba interrupt: nil bus")
	}
	c := &Controller{sink: sink}
	c.install(b.IO())
	c.updateLine()
	return c
}

func (c *Controller) install(io *bus.IO) {
	io.Register16(ieOffset,
		func() uint16 { return c.ie },
		func(value uint16) {
			c.ie = value & validMask
			c.updateLine()
		},
	)

	io.Register16WithByteWrite(ifOffset,
		func() uint16 { return c.flags },
		func(value uint16) { c.acknowledge(value) },
		func(byteOffset uint32, value byte) {
			mask := uint16(value)
			if byteOffset != 0 {
				mask <<= 8
			}
			c.acknowledge(mask)
		},
	)

	io.Register16(imeOffset,
		func() uint16 {
			if c.ime {
				return 1
			}
			return 0
		},
		func(value uint16) {
			c.ime = value&1 != 0
			c.updateLine()
		},
	)
}

func (c *Controller) acknowledge(mask uint16) {
	c.flags &^= mask & validMask
	c.updateLine()
}

func (c *Controller) updateLine() {
	asserted := c.ime && c.ie&c.flags != 0
	if c.sink != nil {
		c.sink.SetIRQLine(asserted)
	}
}

// Request latches one or more interrupt-source bits into IF. Requests are
// recorded regardless of IE/IME state.
func (c *Controller) Request(source Source) {
	c.flags |= uint16(source) & validMask
	c.updateLine()
}

// Reset clears IE, IF, and IME and deasserts the CPU IRQ line.
func (c *Controller) Reset() {
	c.ie = 0
	c.flags = 0
	c.ime = false
	c.updateLine()
}

// IE returns the enabled-source mask.
func (c *Controller) IE() uint16 { return c.ie }

// IF returns the latched request flags.
func (c *Controller) IF() uint16 { return c.flags }

// IME reports the master interrupt-enable bit.
func (c *Controller) IME() bool { return c.ime }

// EnabledPending reports the HALT wake condition. GBA HALT is released when
// any requested interrupt is enabled in IE, independently of IME and CPSR.I.
func (c *Controller) EnabledPending() bool {
	return c.ie&c.flags != 0
}

// IRQAsserted reports the controller's resolved output level.
func (c *Controller) IRQAsserted() bool {
	return c.ime && c.EnabledPending()
}
