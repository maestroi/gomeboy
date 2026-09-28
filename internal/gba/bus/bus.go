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
	ROM2Start      uint32 = 0x0c000000
	ROMWindowSize         = 0x02000000
	EEPROMStart    uint32 = 0x0d000000
	EEPROMHighStart uint32 = 0x0dffff00
	GPIODataAddress      uint32 = ROM0Start + 0x000000c4
	GPIODirectionAddress uint32 = ROM0Start + 0x000000c6
	GPIOControlAddress   uint32 = ROM0Start + 0x000000c8
	SaveStart            uint32 = 0x0e000000
	SaveWindowSize              = 0x02000000
)

// Access is retained as a compatibility alias for the shared GBA access
// descriptor used by CPU, DMA, and the memory bus.
type Access = memory.Access

// SaveDevice is the byte-wide Game Pak save-memory boundary used by SRAM and
// Flash cartridge devices.
type SaveDevice interface {
	Read8(addr uint32) byte
	Write8(addr uint32, value byte)
}

// EEPROMDevice is the serial 1-bit save-memory boundary exposed through the
// ROM2/0x0D Game Pak window. Each 16-bit access transfers bit 0 only.
type EEPROMDevice interface {
	ReadBit() byte
	WriteBit(value byte)
}

// GamePakDevice intercepts cartridge ROM-space bytes implemented by hardware
// rather than ROM. Returning handled=false exposes the original ROM byte.
type GamePakDevice interface {
	Read8(addr uint32) (value byte, handled bool)
	Write8(addr uint32, value byte) (handled bool)
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
	eeprom  EEPROMDevice
	gamePak GamePakDevice

	wait         WaitControl
	prefetch     prefetchState
	openBus      uint32
	cpuOpenBus      uint32
	cpuOpenBusValid bool
	biosPrefetch    uint32
	cpuInBIOS    bool

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

// AttachSaveDevice attaches byte-wide SRAM/Flash cartridge storage.
func (b *Bus) AttachSaveDevice(device SaveDevice) { b.save = device }

// AttachEEPROMDevice attaches serial EEPROM storage in the ROM2 window.
func (b *Bus) AttachEEPROMDevice(device EEPROMDevice) { b.eeprom = device }

// AttachGamePakDevice attaches ROM-space cartridge hardware such as the GPIO
// port used by the S-3511A real-time clock.
func (b *Bus) AttachGamePakDevice(device GamePakDevice) { b.gamePak = device }

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

// OpenBus returns the current physical 32-bit bus latch.
func (b *Bus) OpenBus() uint32 { return b.openBus }

// CPUOpenBus returns the CPU-visible instruction-pipeline/open-bus value used
// when a CPU data read targets an unmapped or write-only location.
func (b *Bus) CPUOpenBus() uint32 {
	if b.cpuOpenBusValid {
		return b.cpuOpenBus
	}
	return b.openBus
}

// SetOpenBus seeds both physical and CPU-visible latches. It is primarily used
// by focused bus tests and debugger-style integrations.
func (b *Bus) SetOpenBus(value uint32) {
	b.openBus = value
	b.cpuOpenBus = value
	b.cpuOpenBusValid = true
}

// SetCPUOpenBus updates only the CPU-visible pipeline latch. Instruction fetch
// adapters use this without pretending that speculative prefetch replaced the
// physical bus transaction that actually occurred.
func (b *Bus) SetCPUOpenBus(value uint32) {
	b.cpuOpenBus = value
	b.cpuOpenBusValid = true
}

// SetBIOSPrefetch updates the protected BIOS read latch. BIOS HLE adapters use
// this when they emulate a BIOS call without executing the real instruction
// stream; real BIOS instruction fetches maintain the latch automatically.
func (b *Bus) SetBIOSPrefetch(value uint32) { b.biosPrefetch = value }

// BIOSPrefetch returns the current protected BIOS read latch.
func (b *Bus) BIOSPrefetch() uint32 { return b.biosPrefetch }

// Read8 performs an 8-bit bus read and returns the access duration in cycles.
func (b *Bus) Read8(addr uint32, access Access) (byte, uint32) {
	cycles := b.accessCycles(addr, 1, access)
	b.noteInstructionFetch(addr, 1, access)
	if value, ok := b.protectedBIOSRead(addr, 1, access); ok {
		out := byte(value)
		b.openBus = uint32(out) * 0x01010101
		b.afterAccess(addr, 1, access, true, cycles)
		return out, cycles
	}
	value, mapped := b.readByte(addr)
	if !mapped {
		if b.isOutOfBoundsROM(addr) {
			value = outOfBoundsROMByte(addr)
		} else {
			latch := b.CPUOpenBus()
			if access.DMA {
				latch = b.openBus
			}
			value = byte(latch >> ((addr & 3) * 8))
		}
	}
	b.openBus = uint32(value) * 0x01010101
	b.afterAccess(addr, 1, access, mapped, cycles)
	return value, cycles
}

// Read16 performs an ARM7-style halfword read. Odd addresses read the aligned
// halfword and rotate it by 8 bits.
func (b *Bus) Read16(addr uint32, access Access) (uint16, uint32) {
	cycles := b.accessCycles(addr, 2, access)
	b.noteInstructionFetch(addr, 2, access)
	if raw, ok := b.protectedBIOSRead(addr, 2, access); ok {
		value := uint16(raw)
		if addr&1 != 0 {
			value = value>>8 | value<<8
		}
		b.openBus = uint32(value) | uint32(value)<<16
		b.afterAccess(addr, 2, access, true, cycles)
		return value, cycles
	}
	if b.isEEPROMAddress(addr) {
		value := uint16(b.eeprom.ReadBit() & 1)
		b.openBus = uint32(value) | uint32(value)<<16
		b.afterAccess(addr, 2, access, true, cycles)
		return value, cycles
	}
	if isSave(addr) && b.save != nil {
		physical := addr + uint32(access.Misalignment&1)
		value := b.save.Read8(physical - SaveStart)
		out := uint16(value) * 0x0101
		b.openBus = uint32(out) | uint32(out)<<16
		b.afterAccess(addr, 2, access, true, cycles)
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
		if b.isOutOfBoundsROM(aligned) {
			value = uint16((aligned >> 1) & 0xffff)
		} else {
			latch := b.CPUOpenBus()
			if access.DMA {
				latch = b.openBus
			}
			value = uint16(bits.RotateLeft32(latch, -int((addr&3)*8)))
		}
	}
	if addr&1 != 0 {
		value = value>>8 | value<<8
	}
	b.openBus = uint32(value) | uint32(value)<<16
	b.afterAccess(addr, 2, access, mapped, cycles)
	return value, cycles
}

// Read32 performs an ARM7-style word read. Misaligned reads rotate the aligned
// word right by 8/16/24 bits.
func (b *Bus) Read32(addr uint32, access Access) (uint32, uint32) {
	cycles := b.accessCycles(addr, 4, access)
	b.noteInstructionFetch(addr, 4, access)
	if raw, ok := b.protectedBIOSRead(addr, 4, access); ok {
		value := bits.RotateLeft32(raw, -int((addr&3)*8))
		b.openBus = value
		b.afterAccess(addr, 4, access, true, cycles)
		return value, cycles
	}
	if isSave(addr) && b.save != nil {
		value := b.save.Read8(addr - SaveStart)
		out := uint32(value) * 0x01010101
		b.openBus = out
		b.afterAccess(addr, 4, access, true, cycles)
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
		if b.isOutOfBoundsROM(aligned) {
			low := (aligned >> 1) & 0xffff
			high := ((aligned + 2) >> 1) & 0xffff
			value = low | high<<16
		} else {
			value = b.CPUOpenBus()
			if access.DMA {
				value = b.openBus
			}
		}
	}
	value = bits.RotateLeft32(value, -int((addr&3)*8))
	b.openBus = value
	b.afterAccess(addr, 4, access, mapped, cycles)
	return value, cycles
}

// Write8 performs an 8-bit write and returns the access duration in cycles.
func (b *Bus) Write8(addr uint32, value byte, access Access) uint32 {
	cycles := b.accessCycles(addr, 1, access)
	b.writeByte(addr, value)
	b.openBus = uint32(value) * 0x01010101
	b.afterAccess(addr, 1, access, true, cycles)
	return cycles
}

// Write16 aligns the address down to a halfword boundary.
func (b *Bus) Write16(addr uint32, value uint16, access Access) uint32 {
	cycles := b.accessCycles(addr, 2, access)
	if b.isEEPROMAddress(addr) {
		b.eeprom.WriteBit(byte(value))
		b.openBus = uint32(value) | uint32(value)<<16
		b.afterAccess(addr, 2, access, true, cycles)
		return cycles
	}
	if isSave(addr) && b.save != nil {
		b.save.Write8(addr-SaveStart, byte(value>>((addr&1)*8)))
		b.openBus = uint32(value) | uint32(value)<<16
		b.afterAccess(addr, 2, access, true, cycles)
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
	b.afterAccess(addr, 2, access, true, cycles)
	return cycles
}

// Write32 aligns the address down to a word boundary.
func (b *Bus) Write32(addr uint32, value uint32, access Access) uint32 {
	cycles := b.accessCycles(addr, 4, access)
	if isSave(addr) && b.save != nil {
		b.save.Write8(addr-SaveStart, byte(value>>((addr&3)*8)))
		b.openBus = value
		b.afterAccess(addr, 4, access, true, cycles)
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
	b.afterAccess(addr, 4, access, true, cycles)
	return cycles
}

// Peek16 reads an aligned mapped halfword without timing or open-bus latch
// updates. It is used by the CPU-facing adapter for Thumb pipeline state.
func (b *Bus) Peek16(addr uint32) uint16 {
	aligned := addr &^ 1
	lo, ok0 := b.readByte(aligned)
	hi, ok1 := b.readByte(aligned + 1)
	if !ok0 || !ok1 {
		return uint16(bits.RotateLeft32(b.openBus, -int((aligned&3)*8)))
	}
	return uint16(lo) | uint16(hi)<<8
}

// Peek32 reads an aligned mapped word without timing or open-bus latch updates.
// It is used by the CPU-facing adapter to model the ARM instruction prefetch
// value that appears on open bus.
func (b *Bus) Peek32(addr uint32) uint32 {
	aligned := addr &^ 3
	var raw [4]byte
	for index := range raw {
		value, mapped := b.readByte(aligned + uint32(index))
		if !mapped {
			return b.openBus
		}
		raw[index] = value
	}
	return binary.LittleEndian.Uint32(raw[:])
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



func (b *Bus) isOutOfBoundsROM(addr uint32) bool {
	if !isROM(addr) || b.isEEPROMAddress(addr) {
		return false
	}
	if b.gamePak != nil {
		if _, handled := b.gamePak.Read8(addr); handled {
			return false
		}
	}
	offset := addr & (ROMWindowSize - 1)
	return offset >= uint32(len(b.rom))
}

func outOfBoundsROMByte(addr uint32) byte {
	halfword := (addr >> 1) & 0xffff
	if addr&1 != 0 {
		return byte(halfword >> 8)
	}
	return byte(halfword)
}

func (b *Bus) protectedBIOSRead(addr, width uint32, access Access) (uint32, bool) {
	if addr >= BIOSSize || access.Instruction || access.DMA || b.cpuInBIOS {
		return 0, false
	}
	switch width {
	case 1:
		return (b.biosPrefetch >> ((addr & 3) * 8)) & 0xff, true
	case 2:
		return (b.biosPrefetch >> ((addr & 2) * 8)) & 0xffff, true
	case 4:
		return b.biosPrefetch, true
	default:
		return 0, false
	}
}

func (b *Bus) noteInstructionFetch(addr, width uint32, access Access) {
	if !access.Instruction {
		return
	}
	if addr >= BIOSSize {
		b.cpuInBIOS = false
		return
	}
	b.cpuInBIOS = true

	// The ARM7 BIOS protection latch reflects the instruction pipeline rather
	// than the addressed BIOS byte. Our CPU does not materialize a two-entry
	// fetch queue, so derive the second prefetched word from the current fetch.
	var ahead uint32
	if width == 2 {
		ahead = ((addr &^ 1) + 4) &^ 3
	} else {
		ahead = (addr &^ 3) + 8
	}
	if ahead+4 <= uint32(len(b.bios)) {
		b.biosPrefetch = binary.LittleEndian.Uint32(b.bios[ahead : ahead+4])
	}
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
		if b.gamePak != nil {
			if value, handled := b.gamePak.Read8(addr); handled {
				return value, true
			}
		}
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
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d:
		if b.gamePak != nil {
			b.gamePak.Write8(addr, value)
		}
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
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d:
		if b.gamePak != nil {
			b.gamePak.Write8(addr, value)
		}
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

func (b *Bus) isEEPROMAddress(addr uint32) bool {
	if b.eeprom == nil || addr < EEPROMStart || addr >= SaveStart {
		return false
	}
	// ROMs up to 16 MiB leave the whole 0x0D mirror available to EEPROM.
	// Larger carts overlap that mirror, so hardware only decodes the top 256
	// bytes at 0x0DFFFF00-0x0DFFFFFF for EEPROM serial traffic.
	if len(b.rom) > 16*1024*1024 {
		return addr >= EEPROMHighStart
	}
	return true
}
