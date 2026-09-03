package gomeboy

import "crypto/sha256"

// CartInfo describes the loaded cartridge. It is a value copy; mutating it
// has no effect on the emulator.
type CartInfo struct {
	Title            string
	ManufacturerCode string
	CartridgeType    string // human-readable, e.g. "MBC3RAMBATT"
	MapperCode       uint16
	ROMSize          int
	RAMSize          int
	CGBFlag          uint8
	SGBFlag          bool
	HeaderChecksum   uint8
	GlobalChecksum   uint16
	Battery          bool
	RAM              bool
	RTC              bool
	Rumble           bool
	Accelerometer    bool
}

// ROM returns a caller-owned copy of the loaded ROM bytes. Mutating the
// returned slice cannot alter the running emulator or its cartridge state.
func (e *Emulator) ROM() []byte {
	return append([]byte(nil), e.gb.ROM...)
}

// ROMSHA256 returns the SHA-256 of the loaded ROM.
func (e *Emulator) ROMSHA256() [32]byte {
	return sha256.Sum256(e.gb.ROM)
}

// Cartridge returns the loaded cartridge's header metadata. Before a ROM is
// loaded it returns the zero value.
func (e *Emulator) Cartridge() CartInfo {
	if e.gb.Bus == nil {
		return CartInfo{}
	}
	c := e.gb.Bus.Cartridge()
	if c == nil {
		return CartInfo{}
	}
	return CartInfo{
		Title:            c.Title,
		ManufacturerCode: c.ManufacturerCode,
		CartridgeType:    c.CartridgeType.String(),
		MapperCode:       uint16(c.CartridgeType),
		ROMSize:          c.ROMSize,
		RAMSize:          c.RAMSize,
		CGBFlag:          uint8(c.CGBFlag),
		SGBFlag:          c.SGBFlag,
		HeaderChecksum:   c.HeaderChecksum,
		GlobalChecksum:   c.GlobalChecksum,
		Battery:          c.Features.Battery,
		RAM:              c.Features.RAM,
		RTC:              c.Features.RTC,
		Rumble:           c.Features.Rumble,
		Accelerometer:    c.Features.Accelerometer,
	}
}
