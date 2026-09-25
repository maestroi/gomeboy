// Package bus implements the Game Boy Advance 32-bit memory bus.
package bus

import (
	"encoding/binary"
	"math/bits"

	"github.com/maestroi/gomeboy/internal/gba/memory"
)

const (
	BIOSStart    uint32 = 0x00000000
	BIOSSize            = 0x00004000
	EWRAMStart   uint32 = 0x02000000
	EWRAMSize           = 0x00040000
	IWRAMStart   uint32 = 0x03000000
	IWRAMSize           = 0x00008000
	IOStart      uint32 = 0x04000000
	IOSize              = 0x00000400
	PaletteStart uint32 = 0x05000000
	PaletteSize         = 0x00000400
	VRAMStart    uint32 = 0x06000000
	VRAMSize            = 0x00018000
	OAMStart     uint32 = 0x07000000
	OAMSize             = 0x00000400
	ROM0Start    uint32 = 0x08000000
	ROM1Start    uint32 = 0x0a000000
	ROM2Start    uint32 = 0x0c000000
	ROMWindowSize       = 0x02000000
	SaveStart    uint32 = 0x0e000000
	SaveWindowSize      = 0x02000000
)

// Access is retained as a compatibility alias for the shared GBA access
// descriptor used by CPU, DMA, and the memory bus.
type Access = memory.Access

// SaveDevice is the byte-wide Game Pak save-memory boundary. SRAM/Flash/EEPROM
// protocols are implemented by cartridge devices in a later layer.
type SaveDevice interface {
	Read8(addr uint32) byte
	Write8(addr uint32, value byte)
}

// Bus is the GBA address-space implementation.
type Bus struct {
	bios    []byte
	rom     []byte
	ewram   []byte
	iwram   []byte
	palette []byte
	vram    []byte
	oam     []byte
	io      *IO
	save    SaveDevice

	wait     WaitControl
	prefetch prefetchState
	openBus  uint32

	// Byte writes to OBJ VRAM are ignored. Tile modes start OBJ VRAM at
	// 0x10000; bitmap modes move it to 0x14000.
	objVRAMStart uint32
}

// New creates a GBA bus. BIOS and ROM are copied so the bus owns its memory.
func New(bios, rom []byte) *Bus {
	b := &Bus{
		bios:         make([]byte, BIOSSize),
		rom:          append([]byte(nil), rom...),
		ewram:        make([]byte, EWRAMSize),
		iwram:        make([]byte, IWRAMSize),
		palette:      make([]byte, PaletteSize),
		vram:         make([]byte, VRAMSize),
		oam:          make([]byte, OAMSize),
		io:           NewIO(),
		objVRAMStart: 0x10000,
	}
	copy(b.bios, bios)
	b.installSystemRegisters()
	return b
}

// IO returns the I/O-register map so hardware blocks can register callbacks.
func (b *Bus) IO() *IO { return b.io }

// AttachSaveDevice attaches cartridge-side save storage.
func (b *Bus) AttachSaveDevice(device SaveDevice) { b.save = device }

// SetOBJVRAMStart selects the first VRAM byte treated as OBJ data for STRB.
// Use 0x10000 for tile modes and 0x14000 for bitmap modes.
func (b *Bus) SetOBJVRAMStart(offset uint32) {
	switch offset {
	case 0x10000, 0x14000:
		b.objVRAMStart = offset
	default:
		panic("gba bus: invalid OBJ VRAM start")
	}
}

// Direct memory views are intended for other GBA hardware blocks.
func (b *Bus) EWRAM() []byte     { return b.ewram }
func (b *Bus) IWRAM() []byte     { return b.iwram }
func (b *Bus) PaletteRAM() []byte { return b.palette }
func (b *Bus) VRAM() []byte      { return b.vram }
func (b *Bus) OAM() []byte       { return b.oam }

// OpenBus returns the current 32-bit bus latch.
func (b *Bus) OpenBus() uint32 { return b.openBus }

// SetOpenBus allows the CPU/fetch pipeline to provide a more accurate latch
// value when implementing instruction-derived open-bus behavior.
func (b *Bus) SetOpenBus(value uint32) { b.openBus = value }

// Read8 performs an 8-bit bus read and returns the access duration in cycles.
func (b *Bus) Read8(addr uint32, access Access) (byte, uint32) {
	cycles := b.accessCycles(addr, 1, access)
	value, mapped := b.readByte(addr)
	if !mapped {
		value = byte(b.openBus >> ((addr & 3) * 8))
	}
	b.openBus = uint32(value) * 0x01010101
	b.afterAccess(addr, 1, access, mapped)
	return value, cycles
}

// Read16 performs an ARM7-style halfword read. Odd addresses read the aligned
// halfword and rotate it by 8 bits.
func (b *Bus) Read16(addr uint32, access Access) (uint16, uint32) {
	cycles := b.accessCycles(addr, 2, access)
	if isSave(addr) && b.save != nil {
		value := b.save.Read8(addr - SaveStart)
		out := uint16(value) * 0x0101
		b.openBus = uint32(out) | uint32(out)<<16
		b.afterAccess(addr, 2, access, true)
		return out, cycles
	}
	aligned := addr &^ 1
	var value uint16
	var mapped bool
	if isIO(aligned) {
		value, mapped = b.io.Read16(aligned - IOStart)
	} else {
		lo, ok0 := b.readByte(aligned)
		hi, ok1 := b.readByte(aligned + 1)
		mapped = ok0 && ok1
		value = uint16(lo) | uint16(hi)<<8
	}
	if !mapped {
		value = uint16(bits.RotateLeft32(b.openBus, -int((addr&3)*8)))
	}
	if addr&1 != 0 {
		value = value>>8 | value<<8
	}
	b.openBus = uint32(value) | uint32(value)<<16
	b.afterAccess(addr, 2, access, mapped)
	return value, cycles
}

// Read32 performs an ARM7-style word read. Misaligned reads rotate the aligned
// word right by 8/16/24 bits.
func (b *Bus) Read32(addr uint32, access Access) (uint32, uint32) {
	cycles := b.accessCycles(addr, 4, access)
	if isSave(addr) && b.save != nil {
		value := b.save.Read8(addr - SaveStart)
		out := uint32(value) * 0x01010101
		b.openBus = out
		b.afterAccess(addr, 4, access, true)
		return out, cycles
	}
	aligned := addr &^ 3
	var value uint32
	var mapped bool
	if isIO(aligned) {
		value, mapped = b.io.Read32(aligned - IOStart)
	} else {
		var raw [4]byte
		mapped = true
		for i := range raw {
			var ok bool
			raw[i], ok = b.readByte(aligned + uint32(i))
			mapped = mapped && ok
		}
		value = binary.LittleEndian.Uint32(raw[:])
	}
	if !mapped {
		value = b.openBus
	}
	value = bits.RotateLeft32(value, -int((addr&3)*8))
	b.openBus = value
	b.afterAccess(addr, 4, access, mapped)
	return value, cycles
}

// Write8 performs an 8-bit write and returns the access duration in cycles.
func (b *Bus) Write8(addr uint32, value byte, access Access) uint32 {
	cycles := b.accessCycles(addr, 1, access)
	b.writeByte(addr, value)
	b.openBus = uint32(value) * 0x01010101
	b.afterAccess(addr, 1, access, true)
	return cycles
}

// Write16 aligns the address down to a halfword boundary.
func (b *Bus) Write16(addr uint32, value uint16, access Access) uint32 {
	cycles := b.accessCycles(addr, 2, access)
	if isSave(addr) && b.save != nil {
		b.save.Write8(addr-SaveStart, byte(value>>((addr&1)*8)))
		b.openBus = uint32(value) | uint32(value)<<16
		b.afterAccess(addr, 2, access, true)
		return cycles
	}
	aligned := addr &^ 1
	if isIO(aligned) {
		b.io.Write16(aligned-IOStart, value)
	} else {
		b.writeByteWide(aligned, byte(value))
		b.writeByteWide(aligned+1, byte(value>>8))
	}
	b.openBus = uint32(value) | uint32(value)<<16
	b.afterAccess(addr, 2, access, true)
	return cycles
}

// Write32 aligns the address down to a word boundary.
func (b *Bus) Write32(addr uint32, value uint32, access Access) uint32 {
	cycles := b.accessCycles(addr, 4, access)
	if isSave(addr) && b.save != nil {
		b.save.Write8(addr-SaveStart, byte(value>>((addr&3)*8)))
		b.openBus = value
		b.afterAccess(addr, 4, access, true)
		return cycles
	}
	aligned := addr &^ 3
	if isIO(aligned) {
		b.io.Write32(aligned-IOStart, value)
	} else {
		for i := uint32(0); i < 4; i++ {
			b.writeByteWide(aligned+i, byte(value>>(i*8)))
		}
	}
	b.openBus = value
	b.afterAccess(addr, 4, access, true)
	return cycles
}

// Peek8 reads mapped memory without timing, open-bus latch updates, or I/O
// write side effects. It is intended for debugger/inspection tooling.
func (b *Bus) Peek8(addr uint32) byte {
	value, mapped := b.readByte(addr)
	if !mapped {
		return byte(b.openBus >> ((addr & 3) * 8))
	}
	return value
}

func (b *Bus) readByte(addr uint32) (byte, bool) {
	switch addr >> 24 {
	case 0x00:
		if addr < BIOSSize {
			return b.bios[addr], true
		}
	case 0x02:
		return b.ewram[(addr-EWRAMStart)&(EWRAMSize-1)], true
	case 0x03:
		return b.iwram[(addr-IWRAMStart)&(IWRAMSize-1)], true
	case 0x04:
		if isIO(addr) {
			return b.io.Read8(addr - IOStart)
		}
	case 0x05:
		return b.palette[(addr-PaletteStart)&(PaletteSize-1)], true
	case 0x06:
		return b.vram[vramOffset(addr)], true
	case 0x07:
		return b.oam[(addr-OAMStart)&(OAMSize-1)], true
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d:
		offset := addr & (ROMWindowSize - 1)
		if offset < uint32(len(b.rom)) {
			return b.rom[offset], true
		}
	case 0x0e, 0x0f:
		if b.save != nil {
			return b.save.Read8(addr - SaveStart), true
		}
	}
	return 0, false
}

func (b *Bus) writeByte(addr uint32, value byte) {
	switch addr >> 24 {
	case 0x02:
		b.ewram[(addr-EWRAMStart)&(EWRAMSize-1)] = value
	case 0x03:
		b.iwram[(addr-IWRAMStart)&(IWRAMSize-1)] = value
	case 0x04:
		if isIO(addr) {
			b.io.Write8(addr-IOStart, value)
		}
	case 0x05:
		// Palette STRB duplicates the byte across the addressed halfword.
		off := (addr - PaletteStart) & (PaletteSize - 1)
		off &^= 1
		b.palette[off] = value
		b.palette[off+1] = value
	case 0x06:
		off := vramOffset(addr)
		// OBJ VRAM ignores byte writes. BG VRAM duplicates the byte.
		if off >= b.objVRAMStart {
			return
		}
		off &^= 1
		b.vram[off] = value
		b.vram[off+1] = value
	case 0x07:
		// OAM ignores byte writes.
	case 0x0e, 0x0f:
		if b.save != nil {
			b.save.Write8(addr-SaveStart, value)
		}
	}
}

func (b *Bus) writeByteWide(addr uint32, value byte) {
	switch addr >> 24 {
	case 0x02:
		b.ewram[(addr-EWRAMStart)&(EWRAMSize-1)] = value
	case 0x03:
		b.iwram[(addr-IWRAMStart)&(IWRAMSize-1)] = value
	case 0x04:
		if isIO(addr) {
			b.io.Write8(addr-IOStart, value)
		}
	case 0x05:
		b.palette[(addr-PaletteStart)&(PaletteSize-1)] = value
	case 0x06:
		b.vram[vramOffset(addr)] = value
	case 0x07:
		b.oam[(addr-OAMStart)&(OAMSize-1)] = value
	case 0x0e, 0x0f:
		if b.save != nil {
			// Wider save writes are byte-bus accesses; each lane is presented
			// individually here. Cartridge devices may further constrain this.
			b.save.Write8(addr-SaveStart, value)
		}
	}
}

func isIO(addr uint32) bool {
	return addr >= IOStart && addr < IOStart+IOSize
}

func vramOffset(addr uint32) uint32 {
	off := (addr - VRAMStart) & 0x1ffff
	if off >= 0x18000 {
		off -= 0x8000
	}
	return off
}

func isROM(addr uint32) bool {
	return addr >= ROM0Start && addr < SaveStart
}

func isSave(addr uint32) bool {
	return addr >= SaveStart
}
