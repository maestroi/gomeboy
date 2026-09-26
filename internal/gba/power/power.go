// Package power implements the Game Boy Advance BIOS system-control register
// pair at POSTFLG/HALTCNT.
package power

import "github.com/maestroi/gomeboy/internal/gba/bus"

const systemControlOffset uint32 = 0x300

// Hooks connect HALTCNT writes to the system timing layer.
//
// BIOSAccess reports whether the current CPU access originates while executing
// inside the BIOS. Retail GBA hardware only accepts POSTFLG/HALTCNT writes from
// BIOS execution. Halt and Stop are invoked for high-byte HALTCNT writes after
// POSTFLG has been set by the BIOS boot sequence.
type Hooks struct {
	BIOSAccess func() bool
	Halt       func()
	Stop       func()
}

// Controller owns POSTFLG and decodes the write-only HALTCNT byte.
type Controller struct {
	hooks    Hooks
	postFlag byte
}

// New maps POSTFLG (0x04000300) and HALTCNT (0x04000301) onto b.
func New(b *bus.Bus, hooks Hooks) *Controller {
	if b == nil {
		panic("gba power: nil bus")
	}
	p := &Controller{hooks: hooks}
	p.install(b.IO())
	return p
}

func (p *Controller) install(io *bus.IO) {
	io.Register16WithByteWrite(
		systemControlOffset,
		func() uint16 {
			// HALTCNT itself is write-only. Only POSTFLG is observable.
			return uint16(p.postFlag)
		},
		p.write16,
		p.write8,
	)
}

func (p *Controller) write16(value uint16) {
	if !p.canWrite() {
		return
	}

	// The current POSTFLG value gates HALTCNT. The BIOS first sets POSTFLG
	// during boot, then later writes the upper byte to enter low-power mode.
	armed := p.postFlag != 0
	p.postFlag = byte(value) & 1
	if armed {
		p.writeHALTCNT(byte(value >> 8))
	}
}

func (p *Controller) write8(byteOffset uint32, value byte) {
	if !p.canWrite() {
		return
	}
	if byteOffset == 0 {
		p.postFlag = value & 1
		return
	}
	if p.postFlag == 0 {
		return
	}
	p.writeHALTCNT(value)
}

func (p *Controller) writeHALTCNT(value byte) {
	if value&0x80 != 0 {
		if p.hooks.Stop != nil {
			p.hooks.Stop()
		}
		return
	}
	if p.hooks.Halt != nil {
		p.hooks.Halt()
	}
}

func (p *Controller) canWrite() bool {
	return p.hooks.BIOSAccess == nil || p.hooks.BIOSAccess()
}

// POSTFlag reports the boot flag visible at 0x04000300.
func (p *Controller) POSTFlag() byte { return p.postFlag }

// Reset clears POSTFLG. HALTCNT has no readable/latching state.
func (p *Controller) Reset() { p.postFlag = 0 }
