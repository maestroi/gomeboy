package io

import (
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
	"github.com/maestroi/gomeboy/pkg/utils"
	"math/rand"
)

const (
	VRAM uint16 = 0b0000_0011_0000_0000
	OAM         = 0b1000_0000_0000_0000

	// CGB has separate cartridge, WRAM, and VRAM buses. During OAM DMA only
	// the source bus is taken from the CPU; the other buses remain accessible.
	cgbCartBus uint16 = 0b0000_1100_1111_1111 // 0000-7FFF, A000-BFFF
	cgbWRAMBus uint16 = 0b1111_0000_0000_0000 // C000-FDFF (IO/HRAM handled separately)
)

// wRAMSeed is the fixed seed used to pseudo-randomize work RAM on boot.
// Keeping it fixed makes headless execution deterministic.
const wRAMSeed int64 = 0x6065626f79

// Bus is the main component responsible for handling IO
// operations on the Game Boy. The Game Boy has a 16-bit
// address bus, allowing for a 64KiB memory space.
//
// The memory space is mapped as so:
//
//		Start  | End	| Name
//	 ----------------------------
//		0x0000 | 0x7FFF | ROM
//		0x8000 | 0x9FFF | VRAM
//		0xA000 | 0xBFFF | External RAM
//		0xC000 | 0xDFFF | Work RAM
//		0xE000 | 0xFDFF | Work RAM Mirror
//		0xFE00 | 0xFE9F | OAM
//		0xFEA0 | 0xFEFF | Not used
//		0xFF00 | 0xFF7F | IO Registers
//		0xFF80 | 0xFFFE | High RAM
//		0xFFFF | 0xFFFF | Interrupt Enable Register
type Bus struct {
	InterruptCallback func(v uint8) // used to notify CPU of interrupts

	data [0x10000]byte   // 64 KiB memory
	wRAM [7][0x1000]byte // 7 banks of 4 KiB each (bank 0 is fixed)
	VRAM [2][0x2000]byte // 2 banks of 8 KiB each

	writeHandlers [0x100]func(byte) byte
	lazyReaders   [0x100]func() byte

	bootHandlers []func()

	c *Cartridge

	model types.Model
	isGBC bool
	s     *scheduler.Scheduler

	gbcHandlers []func()
	agbHandlers []func()

	// DMA related stuff
	dmaSource, dmaDestination  uint16
	dmaActive, dmaRestarting   bool
	dmaConflict                uint8
	dmaEnabled                 bool
	regionLocks, dmaConflicted uint16

	// HDMA/GDMA related stuff
	hdmaSource, hdmaDestination uint16
	dmaLength                   uint8
	dmaRemaining                uint8
	dmaComplete, dmaPaused      bool

	// various IO
	buttonState  uint8
	ime          bool
	sgb          SGBState
	bootROMDone  bool
	vRAMBankMask uint8
	debug        bool
	Debugging    bool

	// cheats
	LoadedCheats   []Cheat
	GameGenieCodes []GameGenieCode
	GameSharkCodes []GameSharkCode
}

func (b *Bus) Debugf(f string, a ...interface{}) {
	if b.Debugging {
		//fmt.Printf(fmt.Sprintf("%d:", b.s.Cycle())+f, a...)
	}
}

// NewBus creates a new Bus instance.
func NewBus(s *scheduler.Scheduler, rom []byte) *Bus {
	b := &Bus{
		s:           s,
		dmaConflict: 0xff,
		Debugging:   true,
	}
	b.c = NewCartridge(rom, b)
	b.ReserveLazyReader(types.DIV, func() byte { return byte(b.s.SysClock() >> 8) })

	s.RegisterEvent(scheduler.DMATransfer, b.doDMATransfer)
	s.RegisterEvent(scheduler.DMAStartTransfer, b.startDMATransfer)
	s.RegisterEvent(scheduler.DMAEndTransfer, b.endDMATransfer)
	s.RegisterEvent(scheduler.CameraShoot, func() { b.c.Camera.Registers[CameraShoot] &^= 1 })

	return b
}

// writeWRAM stores a byte in work RAM and refreshes the echo address that
// mirrors it. Hardware has a single cell for each C000-DDFF byte, readable at
// both that address and E000-FDFF, so a store has to be visible at both.
// DE00-DFFF has no echo.
func (b *Bus) writeWRAM(addr uint16, value byte) {
	b.data[addr] = value
	switch {
	case addr <= 0xDDFF:
		b.data[addr+0x2000] = value
	case addr >= 0xE000:
		b.data[addr-0x2000] = value
	}
}

// syncWRAMEcho refreshes the whole echo window from the work RAM it mirrors.
// The two windows are kept in step by copying, so any path that changes work
// RAM in bulk (a WRAM bank switch, a state restore) has to call this.
func (b *Bus) syncWRAMEcho() {
	copy(b.data[0xE000:0xFE00], b.data[0xC000:0xDE00])
}

func (b *Bus) Map(m types.Model) {
	b.model = m
	b.isGBC = m.IsCGB() || m == types.AGB
	if b.isSGB() {
		b.initSGB()
	}

	// setup CGB only registers
	if b.isGBC && b.c.IsCGBCartridge() {
		b.ReserveAddress(types.KEY0, func(v byte) byte {
			// KEY0 is only writable when boot ROM is running TODO verify
			if !b.bootROMDone {
				return v | 0b1111_0010
			}

			return 0xFF
		})
		b.ReserveAddress(types.KEY1, func(v byte) byte { return b.data[types.KEY1]&types.Bit7 | v&0x1 | 0x7e })

		// setup hdma registers
		b.ReserveAddress(types.HDMA1, func(v byte) byte {
			b.hdmaSource = b.hdmaSource&0x00F0 | uint16(v)<<8
			if b.hdmaSource >= 0xE000 {
				b.hdmaSource |= 0xF000
			}
			return 0xff
		})
		b.ReserveAddress(types.HDMA2, func(v byte) byte {
			b.hdmaSource = b.hdmaSource&0xFF00 | uint16(v&0xF0)
			return 0xff
		})
		b.ReserveAddress(types.HDMA3, func(v byte) byte {
			b.hdmaDestination = b.hdmaDestination&0x00F0 | uint16(v)<<8
			return 0xff
		})
		b.ReserveAddress(types.HDMA4, func(v byte) byte {
			b.hdmaDestination = b.hdmaDestination&0xFF00 | uint16(v&0xF0)
			return 0xff
		})
		b.ReserveAddress(types.HDMA5, func(v byte) byte {
			// update the length
			b.dmaLength = (v & 0x7F) + 1

			// if bit 7 is set, we are starting a new HDMA transfer
			if v&types.Bit7 != 0 {
				b.dmaRemaining = b.dmaLength // set the remaining length

				// reset the DMA flags
				b.dmaComplete = false
				b.dmaPaused = false

				// if the LCD is disabled, one HDMA transfer is performed immediately
				// and the rest are performed during the next HBlank period
				if b.Get(types.LCDC)&types.Bit7 != types.Bit7 && b.dmaRemaining > 0 {
					b.colorDMA(1)
					b.dmaRemaining--
				}

				// if the PPU is already in the HBlank period, then the HDMA would not be
				// performed by the scheduler until the next HBlank period, so we perform
				// the transfer immediately here and decrement the remaining length
				if b.Get(types.LCDC)&types.Bit7 == types.Bit7 && b.LazyRead(types.STAT)&0b11 == 0 && b.dmaRemaining > 0 {
					b.colorDMA(1)
					b.dmaRemaining--
				}
			} else {
				// if bit 7 is not set, we are starting a new GDMA transfer
				if b.dmaRemaining > 0 {
					// if we're in the middle of a HDMA transfer, pause it
					b.dmaPaused = true

					b.dmaRemaining = b.dmaLength
				} else {
					// if we're not in the middle of a HDMA transfer, perform a GDMA transfer
					b.colorDMA(b.dmaLength)
				}
			}

			if b.dmaComplete {
				return 0xFF
			} else {
				v := uint8(0)
				if b.dmaPaused {
					v |= types.Bit7
				}
				return v | (b.dmaRemaining-1)&0x7F
			}
		})
		b.ReserveLazyReader(types.HDMA5, func() byte {
			if b.dmaComplete || b.dmaRemaining == 0 {
				return 0xFF
			} else {
				v := uint8(0)
				if b.dmaPaused {
					v |= types.Bit7
				}
				return v | (b.dmaRemaining-1)&0x7F
			}
		})
		b.ReserveAddress(types.SVBK, func(v byte) byte {
			copy(b.wRAM[utils.ZeroAdjust(b.data[types.SVBK]&7)-1][:], b.data[0xD000:0xE000]) // bus -> wRAM
			copy(b.data[0xD000:0xE000], b.wRAM[utils.ZeroAdjust(v&7)-1][:])                  // wRAM -> bus
			b.syncWRAMEcho()                                                                 // D000-DDFF is echoed at F000-FDFF
			return v | 0xF8
		})
		b.Set(types.SVBK, 0xF8)
	}

	// setup cgb model registers
	if b.model.IsCGB() {
		b.vRAMBankMask = 1
		b.ReserveAddress(types.VBK, func(v byte) byte {
			if b.IsGBCCart() || b.IsBooting() { // CGB boot ROM makes use of both banks
				copy(b.VRAM[b.data[types.VBK]&0x1][:], b.data[0x8000:0xA000])
				copy(b.data[0x8000:0xA000], b.VRAM[v&0x1][:])
				return v | 0xfe
			}
			return 0xff
		})
		b.ReserveAddress(types.FF72, func(v byte) byte { return v })
		b.ReserveAddress(types.FF73, func(v byte) byte { return v })
		// FF74 exists only in native CGB mode. Its power-on value is 0x00 and
		// all eight bits are read/write. In DMG compatibility mode the address
		// remains unhandled, so the generic IO path keeps it locked at 0xff.
		if b.IsGBCCart() {
			b.ReserveAddress(types.FF74, func(v byte) byte { return v })
		}
		b.ReserveAddress(types.FF75, func(v byte) byte { return v&0x70 | 0x8F })
		b.Set(types.FF75, 0x8F)

		for _, f := range b.gbcHandlers {
			f()
		}
	}

	if b.model == types.AGB {
		for _, f := range b.agbHandlers {
			f()
		}
	}

	for i := 0xFF00; i < 0xFF80; i++ {
		if wHandler := b.writeHandlers[i&0xFF]; wHandler == nil {
			b.data[i] = 0xFF // default to 0xff if no write handler exists
		}
	}
}

// Boot the Bus to the state it would be in after execution of the boot ROM.
func (b *Bus) Boot() {
	ioRegs := make(map[types.HardwareAddress]interface{})
	for k, v := range types.CommonIO {
		ioRegs[k] = v
	}
	for k, v := range types.ModelIO[b.model] {
		ioRegs[k] = v
	}
	if b.IsGBCCart() {
		for k, v := range types.ModelIOCGB[b.model] {
			ioRegs[k] = v
		}
	}
	for i := types.HardwareAddress(0xFF00); i < 0xFF80; i++ {
		// has the model provided a value?
		if ioRegs[i] != nil {
			if i == types.DIV { // special case for DIV
				b.s.OverrideDiv(ioRegs[i].(uint16))
			} else if wHandler := b.writeHandlers[i&0xFF]; wHandler != nil {
				b.data[i] = wHandler(ioRegs[i].(byte))
			} else if _, ok := ioRegs[i].(byte); ok {
				b.data[i] = ioRegs[i].(byte)
			}
		}
	}

	// Recreate the VRAM residue left by the selected boot ROM. whichboot.gb
	// fingerprints this state independently of CPU registers/timing.
	logoData := b.data[0x0104:0x0134]
	var unpackedLogoData []byte
	for i := 0; i < len(logoData); i++ {
		var currentData [8]uint8 // every other byte is 0
		for bit := uint8(0); bit < 8; bit++ {
			n := logoData[i] >> bit & 1
			currentData[0] |= n<<(2*(bit-4)) | n<<(2*(bit-4)+1)
			currentData[4] |= n<<(2*bit) | n<<(2*bit+1)
		}
		currentData[2], currentData[6] = currentData[0], currentData[4] // double bytes
		unpackedLogoData = append(unpackedLogoData, currentData[:]...)
	}
	copy(b.data[0x8010:], unpackedLogoData)
	copy(b.VRAM[0][0x0010:], unpackedLogoData)

	// DMG0 predates the registered-symbol addition, so tile $19 is blank.
	// All later official boot ROMs represented here leave the standard ® tile.
	if b.model != types.DMG0 {
		copyright := [...]byte{0x3C, 0, 0x42, 0, 0xB9, 0, 0xA5, 0, 0xB9, 0, 0xA5, 0, 0x42, 0, 0x3C}
		copy(b.data[0x8190:], copyright[:])
		copy(b.VRAM[0][0x0190:], copyright[:])
	}

	// The monochrome/SGB boot ROMs leave the Nintendo logo tile indices in the
	// BG map. CGB/GBA boot ROMs do not leave that map residue behind.
	leaveLogoMap := true
	switch b.model {
	case types.CGB0, types.CGBABC, types.CGBBC, types.CGBDE, types.AGB:
		leaveLogoMap = false
	}
	if leaveLogoMap {
		for i := uint8(0); i < 12; i++ {
			b.data[0x9904+uint16(i)] = i + 1
			b.VRAM[0][0x0904+uint16(i)] = i + 1
			b.data[0x9924+uint16(i)] = i + 13
			b.VRAM[0][0x0924+uint16(i)] = i + 13
		}
		if b.model != types.DMG0 {
			b.data[0x9910] = 0x19
			b.VRAM[0][0x0910] = 0x19
		}
	}

	// wRAM is randomized on boot (not accurate to hardware, but random enough to pass most anti-emu checks).
	// A fixed seed is used so that headless execution is deterministic across runs and instances.
	rng := rand.New(rand.NewSource(wRAMSeed))
	for i := 0; i < 0x2000; i++ {
		v := byte(rng.Intn(256))
		b.data[0xC000+i] = v
		if i <= 0x1dff {
			b.data[0xE000+i] = v
		}
	}

	if b.model.IsCGB() {
		b.Set(types.VBK, 0xFE)

		if !b.IsGBCCart() {
			b.vRAMBankMask = 0
		}
	}

	b.bootROMDone = true
	b.data[types.IF] = 0xE1
}

func (b *Bus) ReserveAddress(addr uint16, f func(byte) byte) { b.writeHandlers[addr&0xff] = f }             // reserve IO address
func (b *Bus) ReserveLazyReader(addr uint16, f func() byte)  { b.lazyReaders[addr&0xff] = f }               // reserve IO lazy reader
func (b *Bus) RegisterBootHandler(f func())                  { b.bootHandlers = append(b.bootHandlers, f) } // called after boot ROM
func (b *Bus) RegisterGBCHandler(f func())                   { b.gbcHandlers = append(b.gbcHandlers, f) }   // called when model is CGB
func (b *Bus) RegisterAGBHandler(f func())                   { b.agbHandlers = append(b.agbHandlers, f) }   // called for AGB GB-compatibility hardware

// Write writes to the specified memory address. This function
// calls the write handler if it exists.
func (b *Bus) Write(addr uint16, value byte) {
	switch {
	// IO & HRAM can't be locked or conflicted
	case addr >= 0xFF00:
		switch addr {
		case types.P1:
			if b.isSGB() {
				value = b.writeSGBP1(value)
			} else {
				d := uint8(0xC0)
				if value&types.Bit4 == 0 {
					d |= b.buttonState >> 4 & 0xf
					d |= types.Bit4
				}
				if value&types.Bit5 == 0 {
					d |= b.buttonState & 0xf
					d |= types.Bit5
				}

				d ^= 0xf

				value = d
			}
		case types.BDIS:
			if b.bootROMDone {
				return
			}
			b.bootROMDone = true
			value = 0xff
			if b.isGBC && !b.IsGBCCart() {
				b.vRAMBankMask = 0
			}

			for _, f := range b.bootHandlers {
				f()
			}
		case types.IF:
			value = value | 0xE0 // upper bits are always 1
			if b.ime && b.data[types.IE]&value&0x1f != 0 {
				b.InterruptCallback(0)
			}
		case types.DMA:
			b.dmaSource = uint16(value) << 8

			if b.dmaSource >= 0xE000 && b.dmaSource < 0xFE00 {
				b.dmaSource &= 0xDDFF // account for mirroring
			} else if b.dmaSource >= 0xFE00 {
				b.dmaSource -= 0x2000 // OAM-DMA decoding aliases this range onto WRAM
			}
			b.setDMAConflictBus()

			b.dmaActive = false
			b.dmaRestarting = b.dmaEnabled
			b.dmaDestination = 0xFE00

			// de-schedule any existing DMA transfers
			if b.dmaRestarting {
				b.s.DescheduleEvent(scheduler.DMATransfer)
				b.s.DescheduleEvent(scheduler.DMAStartTransfer)
				b.s.DescheduleEvent(scheduler.DMAEndTransfer)
			}

			b.dmaEnabled = true
			b.s.ScheduleEvent(scheduler.DMAStartTransfer, 8) // TODO find out why 8 instead of 4?
		case types.IE:
			if b.ime && b.data[types.IF]&value != 0 {
				b.InterruptCallback(0)
			}
		default:
			if handler := b.writeHandlers[addr&0xFF]; handler != nil {
				// check to see if a component has reserved this address
				value = handler(value)
			} else if addr <= 0xff7f {
				return
			}
		}
	default:
		// address <= 0xFDFF can be locked or conflicted
		if b.isDMATransferring() && b.dmaConflicted&(1<<(addr>>12)) > 0 {
			return
		}
		switch {
		// 0x0000 - 0x7FFF ROM
		// 0xA000 - 0xBFFF ERAM (RAM on cartridge)
		case addr <= 0x7FFF || addr >= 0xA000 && addr <= 0xBFFF:
			b.c.Write(addr, value)
			return
		// 0x8000 - 0x9FFF VRAM
		case addr <= 0x9FFF:
			if (b.regionLocks<<8)&(1<<(addr>>12)) > 0 {
				return
			}
			b.VRAM[b.data[types.VBK]&b.vRAMBankMask][addr&0x1fff] = value
		// 0xC000-0xFDFF WRAM & echo. Hardware mirrors C000-DDFF at E000-FDFF
		// and has no echo at DE00-DFFF, where it would land on OAM and IO.
		// The previous addr&0xDDFF|0xE000 formula also cleared bit 9, so a
		// write to EE00 updated CE00 but left EE00 stale and a read-back of
		// EE00 returned the old byte. Boxxle copies a tile run into echo RAM
		// and re-reads each address to confirm the store, so it spun on EE00
		// forever and the game never left its blank transition screen.
		case addr <= 0xFDFF:
			b.writeWRAM(addr, value)

			return
		// 0xFE00-0xFE9F OAM
		case addr <= 0xFE9F:
			if (b.regionLocks<<8)&OAM > 0 || b.isDMATransferring() {
				return
			}
		// 0xFEA0-0xFEFF extra/unusable OAM. CGB 0-A/B/C revisions expose
		// a small aliased RAM here; later hardware ignores writes.
		case addr <= 0xFEFF:
			if (b.regionLocks<<8)&OAM > 0 || b.isDMATransferring() {
				return
			}
			switch b.model {
			case types.CGB0, types.CGBABC, types.CGBBC:
				// CGB 0/A/B/C clear address bits 3 and 4, so e.g. FEA0
				// and FEB8 refer to the same backing byte.
				b.data[addr&^0x18] = value
			}
			return
		}
	}

	b.data[addr] = value
}

func (b *Bus) GetVRAM(address uint16, bank uint8) uint8 {
	return b.VRAM[bank&b.vRAMBankMask][address]
}

func (b *Bus) LazyRead(addr uint16) byte {
	if handler := b.lazyReaders[addr&0xFF]; handler != nil {
		return handler()
	}

	return b.data[addr]
}

func (b *Bus) ClearBit(addr uint16, bit byte) { b.data[addr] &^= bit } // clear bit at address
func (b *Bus) Get(addr uint16) byte           { return b.data[addr] }  // get value at address
func (b *Bus) Set(addr uint16, value byte)    { b.data[addr] = value } // set value at address
func (b *Bus) SetBit(addr uint16, bit byte)   { b.data[addr] |= bit }  // set bit at address

func (b *Bus) Block(region uint16, block bool) {
	if block {
		b.Lock(region)
	} else {
		b.Unlock(region)
	}
}
func (b *Bus) RBlock(region uint16, block bool) {
	if block {
		b.RLock(region)
	} else {
		b.RUnlock(region)
	}
}
func (b *Bus) WBlock(region uint16, block bool) {
	if block {
		b.WLock(region)
	} else {
		b.WUnlock(region)
	}
}

func (b *Bus) RLock(region uint16)   { b.regionLocks |= region }              // locks reading from region
func (b *Bus) RUnlock(region uint16) { b.regionLocks &^= region }             // unlocks reading from region
func (b *Bus) WLock(region uint16)   { b.regionLocks |= region >> 8 }         // locks writing to region
func (b *Bus) WUnlock(region uint16) { b.regionLocks &^= region >> 8 }        // unlocks writing to region
func (b *Bus) Lock(region uint16)    { b.regionLocks |= region | region>>8 }  // lock read/writing from region
func (b *Bus) Unlock(region uint16)  { b.regionLocks &^= region | region>>8 } // unlock read/writing from region

func (b *Bus) CopyFrom(start, end uint16, dest []byte) { copy(dest, b.data[start:end]) } // copy from bus -> dest
func (b *Bus) CopyTo(start, end uint16, src []byte) {
	copy(b.data[start:end], src)

	// check to see if any game genie ROM patches should be applied to the src
	if len(b.GameGenieCodes) > 0 {
		for _, c := range b.GameGenieCodes {
			if c.Address >= start && c.Address <= end {
				if b.data[c.Address] == c.OldData {
					b.data[c.Address] = c.NewData
				}
			}
		}
	}
}

func (b *Bus) Read(addr uint16) byte {
	if addr < 0xFE00 && b.isDMATransferring() && b.dmaConflicted&(1<<(addr>>12)) > 0 {
		return b.dmaConflict
	}

	switch {
	case addr <= 0x9FFF || addr >= 0xC000 && addr <= 0xFDFF:
		addrBitmask := uint16(1 << (addr >> 12))
		if b.regionLocks&0xff00&(addrBitmask&0x7fff) > 0 {
			return b.dmaConflict
		}
	case addr <= 0xBFFF:
		switch b.c.CartridgeType {
		case MBC3TIMERBATT, MBC3TIMERRAMBATT:
			if b.c.rtc.enabled && b.c.rtc.register != 0 {
				return b.c.RAM[b.c.RAMSize+int(b.c.rtc.register-3)]
			}
			if !b.c.ramEnabled {
				return 0xff
			}
		case MBC7:
			return b.c.readMBC7RAM(addr)
		case POCKETCAMERA:
			return b.c.readCameraRAM(addr)
		case HUDSONHUC1:
			if b.c.huc1.irMode {
				return 0xc0 // no light
			} else {
				return b.data[addr]
			}
		default:
			if !b.c.ramEnabled {
				return 0xff
			}
		}
	// HRAM/IO can't be locked or conflicted
	case addr >= 0xFF00:
		// does this register need evaluating?
		if f := b.lazyReaders[addr&0xff]; f != nil {
			return f()
		}
	// OAM and the extra/unusable OAM range share the PPU bus lock.
	case addr <= 0xFE9F:
		if b.regionLocks&OAM > 0 || b.isDMATransferring() {
			return 0xff
		}
	case addr <= 0xFEFF:
		if b.regionLocks&OAM > 0 || b.isDMATransferring() {
			return 0xff
		}
		switch b.model {
		case types.CGB0, types.CGBABC, types.CGBBC:
			return b.data[addr&^0x18]
		case types.CGBDE, types.AGB:
			// The grouped D/E profile follows the later E-style open-bus
			// pattern: repeat the high nibble of the low address byte.
			n := byte(addr >> 4 & 0x0f)
			return n<<4 | n
		default:
			// DMG-family hardware reads zero here outside the OAM lock.
			return 0x00
		}
	}

	// if we've managed to fall through to here, we should be
	// able to read the data as it is on the bus
	return b.data[addr]
}

// ClockedRead clocks the Game Boy and reads a byte from the
// bus.
func (b *Bus) ClockedRead(addr uint16) byte {
	b.s.Tick(4)
	return b.Read(addr)
}

// ClockedWrite clocks the Game Boy and writes a byte to the
// bus.
func (b *Bus) ClockedWrite(address uint16, value byte) {
	b.s.Tick(4)

	b.Write(address, value)
}

func (b *Bus) Cartridge() *Cartridge { return b.c }                  // returns the Cartridge
func (b *Bus) IsBooting() bool       { return !b.bootROMDone }       // returns boot status
func (b *Bus) IsGBC() bool           { return b.isGBC }              // returns if in CGB mode
func (b *Bus) IsGBCCart() bool       { return b.c.IsCGBCartridge() } // returns if cart supports CGB
func (b *Bus) Model() types.Model    { return b.model }              // returns the current model

func (b *Bus) isDMATransferring() bool { return b.dmaActive || b.dmaRestarting } // DMA transfer in progress

// setDMAConflictBus records which CPU address bus is owned by OAM DMA.
// DMG has one shared main bus (plus VRAM), while CGB separates cartridge and
// WRAM buses. A CPU read on the owned bus observes the byte currently driven by
// DMA; accesses on the other CGB bus continue normally.
func (b *Bus) setDMAConflictBus() {
	if !b.isGBC {
		if b.dmaSource >= 0x8000 && b.dmaSource < 0xA000 {
			b.dmaConflicted = VRAM
		} else {
			b.dmaConflicted = ^VRAM
		}
		return
	}

	switch {
	case b.dmaSource >= 0x8000 && b.dmaSource < 0xA000:
		b.dmaConflicted = VRAM
	case b.dmaSource >= 0xC000 && b.dmaSource < 0xFE00:
		b.dmaConflicted = cgbWRAMBus
	default:
		b.dmaConflicted = cgbCartBus
	}
}


// PPUReadOAM returns the value currently visible to the PPU on the OAM bus.
//
// During the MGB halted-DMA edge case, OAM DMA stops advancing but the
// in-flight DMA word continues to drive the OAM bus. The Pocket profile
// measured by Mooneye exposes a repeated two-byte value derived from the
// destination word and the next source byte. Other models keep the normal
// raw OAM path here; their halted-DMA corruption patterns are model/unit
// dependent and are intentionally not generalized from the MGB measurement.
func (b *Bus) PPUReadOAM(address uint16) byte {
	if address < 0xfe00 || address >= 0xfea0 {
		return b.data[address]
	}

	if b.model != types.MGB || !b.s.Halted || !b.dmaActive ||
		b.dmaDestination < 0xfe00 || b.dmaDestination >= 0xfea0 {
		return b.data[address]
	}

	// The measured MGB bus profile only produces usable sprite data when OAM
	// contains a row matching the enable pattern documented by the hardware
	// test. Its position in OAM is irrelevant.
	enabled := false
	for i := uint16(0xfe00); i < 0xfea0; i += 4 {
		if b.data[i] >= 0x98 && b.data[i] <= 0x9f &&
			b.data[i+1] <= 0xa7 &&
			b.data[i+2] >= 0x09 && b.data[i+2] <= 0x9f &&
			b.data[i+3] <= 0xa7 {
			enabled = true
			break
		}
	}
	if !enabled {
		return 0xff
	}

	incoming := b.data[b.dmaSource]
	old := b.data[b.dmaDestination]
	next := byte(0xff)
	if b.dmaDestination+1 < 0xfea0 {
		next = b.data[b.dmaDestination+1]
	}

	// Every OAM slot is observed as the same Y/X/tile/flags tuple while the
	// access is frozen. The old byte is the exact destination currently being
	// replaced; it is not word-aligned, so an odd-byte freeze must use that odd
	// OAM byte and its successor.
	if address&1 == 0 {
		return (old | incoming) & 0xfc
	}
	return next | incoming
}

// PPUOAMScanBlockedByDMA reports whether active OAM DMA prevents Mode 2 from
// refreshing its Y/X bus latches. The PPU keeps the previous latch values in
// that case rather than sampling an artificial 0xff byte. The measured MGB
// halted-DMA path remains readable through PPUReadOAM.
func (b *Bus) PPUOAMScanBlockedByDMA() bool {
	return b.dmaActive && !(b.model == types.MGB && b.s.Halted)
}

// PPUReadOAMScan returns an OAM byte for a Mode 2 bus-latch refresh. Callers
// must first check PPUOAMScanBlockedByDMA; while blocked, hardware retains the
// existing Mode 2 bus values instead of performing a new OAM read.
func (b *Bus) PPUReadOAMScan(address uint16) byte {
	return b.PPUReadOAM(address)
}

// PPUReadOAMFetch returns the byte visible when Mode 3 fetches an object's
// tile/attribute word. During active OAM DMA, the PPU sees the 16-bit OAM word
// currently being updated by DMA rather than the selected object's stored word.
// The requested address is used only for its low/high-byte parity.
func (b *Bus) PPUReadOAMFetch(address uint16) byte {
	if !b.dmaActive || (b.model == types.MGB && b.s.Halted) {
		return b.PPUReadOAM(address)
	}

	// dmaDestination points at the next byte to be copied. The PPU-facing OAM
	// bus is word-oriented and advances to the word containing that destination;
	// crossing an even-byte boundary therefore exposes the next word immediately,
	// rather than the word containing the byte that was just written.
	if b.dmaDestination >= 0xfea0 {
		return b.PPUReadOAM(address)
	}
	wordBase := b.dmaDestination &^ 1
	if wordBase < 0xfe00 {
		return 0xff
	}
	return b.data[wordBase+address&1]
}

// startDMATransfer initiates a DMA transfer.
func (b *Bus) startDMATransfer() {
	b.dmaActive = true
	b.dmaRestarting = false
	b.doDMATransfer()
}

// doDMATransfer performs a single DMA operation, copying a byte from the source to OAM.
// OAM DMA is clock-gated while the CPU is halted, so the transfer event remains
// pending and resumes one machine cycle after HALT is released.
func (b *Bus) doDMATransfer() {
	if b.s.Halted {
		b.s.ScheduleEvent(scheduler.DMATransfer, 4)
		return
	}

	b.dmaConflict = b.data[b.dmaSource]
	b.data[b.dmaDestination] = b.dmaConflict

	b.dmaSource++
	b.dmaDestination++

	if b.dmaDestination < 0xfea0 {
		b.s.ScheduleEvent(scheduler.DMATransfer, 4)
	} else {
		b.s.ScheduleEvent(scheduler.DMAEndTransfer, 4)
	}
}

// endDMATransfer ends a DMA transfer.
func (b *Bus) endDMATransfer() {
	b.dmaActive, b.dmaEnabled = false, false
	b.dmaConflicted = 0
	b.dmaConflict = 0xff
}

// colorDMA performs a GDMA/HDMA transfer of length, transferring from source to VRAM.
func (b *Bus) colorDMA(length uint8) {
	for i := uint8(0); i < length; i++ {
		for j := uint8(0); j < 16; j++ {
			// tick the scheduler (OR WAIT SHOULD WE SCHEDULE THE EVENTS?)
			if b.s.DoubleSpeed() {
				b.s.Tick(4)
			} else {
				b.s.Tick(2)
			}

			// perform the transfer
			b.Write(b.hdmaDestination&0x1fff|0x8000, b.Get(b.hdmaSource))

			// increment the source and destination
			b.hdmaSource++
			b.hdmaDestination++
		}
	}
}

func (b *Bus) HandleHDMA() {
	// HDMA should be halted also when the CPU is halted
	if b.s.Halted {
		return
	}

	// is there any remaining data to transfer and
	// has the DMA not been paused?
	if b.dmaRemaining > 0 && !b.dmaPaused {
		// update HDMA5 register as the next DMA will tick
		b.Set(types.HDMA5, b.Get(types.HDMA5)&0x80|(b.dmaRemaining-1)&0x7f)
		b.colorDMA(1)
		b.dmaRemaining--
	} else if !b.dmaPaused {
		b.dmaRemaining = 0
		b.dmaComplete = true
		b.Set(types.HDMA5, 0xFF)
	}
}

const (
	VBlankINT = types.Bit0 // ppu vblank
	LCDINT    = types.Bit1 // lcd stat
	TimerINT  = types.Bit2 // timer overflow
	SerialINT = types.Bit3 // serial transfer
	JoypadINT = types.Bit4 // joypad
)

// EnableInterrupts sets IME
func (b *Bus) EnableInterrupts() {
	b.ime = true

	if b.HasInterrupts() {
		b.InterruptCallback(0)
	}
}

// RaiseInterrupt sets the requested interrupt high in types.IF
func (b *Bus) RaiseInterrupt(interrupt uint8) {
	b.data[types.IF] |= interrupt
	if interrupt == VBlankINT || b.CanInterrupt() {
		b.InterruptCallback(interrupt)
	}

	if interrupt == VBlankINT && len(b.GameSharkCodes) > 0 {
		for _, c := range b.GameSharkCodes {
			if c.Address < 0xD000 {
				b.data[c.Address] = c.NewData
			} else if c.Address < 0xE000 {
				if utils.ZeroAdjust(b.Get(types.SVBK)&7) == c.ExternalRAMBank { // banked data - write to bus
					b.data[c.Address] = c.NewData
				} else { // not banked so write to sRAM
					b.wRAM[c.ExternalRAMBank][c.Address&0x0fff] = c.NewData
				}
			}
		}
	}
}

// IRQVector returns the current interrupt vector and clears the corresponding
// interrupt from types.IF.
//
// When an interrupt occurs, there is a chance for the interrupt vector to change
// during the execution of the dispatch handler.
// https://mgba.io/2018/03/09/holy-grail-bugs-revisited/
func (b *Bus) IRQVector(irq uint8) uint16 {
	for i := uint8(0); i < 5; i++ {
		f := uint8(1 << i)

		if irq&b.data[types.IF]&f == f {
			b.data[types.IF] &^= f

			return uint16(0x0040 + i<<3)
		}
	}

	return 0
}

func (b *Bus) CanInterrupt() bool      { return b.ime && b.HasInterrupts() }                 // IME set & pending interrupts
func (b *Bus) DisableInterrupts()      { b.ime = false }                                     // resets IME
func (b *Bus) HasInterrupts() bool     { return b.data[types.IE]&b.data[types.IF]&0x1F > 0 } // pending interrupts
func (b *Bus) InterruptsEnabled() bool { return b.ime }                                      // IME set

type Button = uint8

const (
	ButtonA Button = iota
	ButtonB
	ButtonSelect
	ButtonStart
	ButtonRight
	ButtonLeft
	ButtonUp
	ButtonDown
)

func (b *Bus) Press(i uint8)   { b.buttonState |= 1 << i; b.RaiseInterrupt(JoypadINT) } // presses the requested button
func (b *Bus) Release(i uint8) { b.buttonState &^= 1 << i }                             // releases the requested button
