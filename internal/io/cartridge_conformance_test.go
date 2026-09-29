package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
)

func newMBC3RTCTestBus(t *testing.T) (*Bus, *uint64) {
	t.Helper()

	rom := make([]byte, 0x8000)
	rom[0x0147] = byte(MBC3TIMERRAMBATT)
	rom[0x0149] = 0x03 // 32 KiB RAM
	b := NewBus(scheduler.NewScheduler(), rom)

	now := uint64(0)
	b.c.rtcNow = func() uint64 { return now }
	b.Write(0x0000, 0x0a) // enable RAM + RTC
	return b, &now
}

func writeRTCRegister(b *Bus, register, value byte) {
	b.Write(0x4000, register)
	b.Write(0xa000, value)
}

func latchRTC(b *Bus) {
	b.Write(0x6000, 0)
	b.Write(0x6000, 1)
}

func readRTCRegister(b *Bus, register byte) byte {
	b.Write(0x4000, register)
	return b.Read(0xa000)
}

func TestMBC3RTCLatchKeepsSubsecondRemainder(t *testing.T) {
	b, now := newMBC3RTCTestBus(t)
	writeRTCRegister(b, 0x08, 0)

	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 0 {
		t.Fatalf("initial latched seconds = %d, want 0", got)
	}

	*now += rtcCyclesPerSecond + rtcCyclesPerSecond/2
	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 1 {
		t.Fatalf("latched seconds after 1.5 seconds = %d, want 1", got)
	}

	*now += rtcCyclesPerSecond / 2

	// A lone 1 is not a latch command; hardware requires a 0 -> 1 sequence.
	b.Write(0x6000, 1)
	if got := readRTCRegister(b, 0x08); got != 1 {
		t.Fatalf("latch changed without 0 -> 1 transition: got %d, want 1", got)
	}

	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 2 {
		t.Fatalf("latched seconds after two half-second remainders = %d, want 2", got)
	}
}

func TestMBC3RTCHaltResumePreservesPhase(t *testing.T) {
	b, now := newMBC3RTCTestBus(t)
	writeRTCRegister(b, 0x08, 0)

	*now += rtcCyclesPerSecond + rtcCyclesPerSecond/2
	writeRTCRegister(b, 0x0c, 0x40) // halt

	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 1 {
		t.Fatalf("seconds when halted after 1.5 seconds = %d, want 1", got)
	}

	*now += 10 * rtcCyclesPerSecond
	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 1 {
		t.Fatalf("halted RTC advanced: seconds = %d, want 1", got)
	}

	writeRTCRegister(b, 0x0c, 0x00) // resume
	*now += rtcCyclesPerSecond / 2
	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 2 {
		t.Fatalf("seconds after resume + preserved half-second = %d, want 2", got)
	}
}

func TestMBC3RTCDayCarryAt512Days(t *testing.T) {
	b, now := newMBC3RTCTestBus(t)

	writeRTCRegister(b, 0x0c, 0x41) // halt, day bit 8 set
	writeRTCRegister(b, 0x08, 59)
	writeRTCRegister(b, 0x09, 59)
	writeRTCRegister(b, 0x0a, 23)
	writeRTCRegister(b, 0x0b, 0xff)
	writeRTCRegister(b, 0x0c, 0x01) // resume at day 511

	*now += rtcCyclesPerSecond
	latchRTC(b)

	for _, tc := range []struct {
		register byte
		want     byte
	}{
		{0x08, 0},
		{0x09, 0},
		{0x0a, 0},
		{0x0b, 0},
		{0x0c, 0x80}, // day bit 8 cleared, carry set
	} {
		if got := readRTCRegister(b, tc.register); got != tc.want {
			t.Errorf("RTC register %#02x after day 511 overflow = %#02x, want %#02x", tc.register, got, tc.want)
		}
	}
}

func TestMBC3RTCRegisterMasks(t *testing.T) {
	b, _ := newMBC3RTCTestBus(t)

	writeRTCRegister(b, 0x0c, 0x40) // halt while programming registers
	writeRTCRegister(b, 0x08, 0xff)
	writeRTCRegister(b, 0x09, 0xff)
	writeRTCRegister(b, 0x0a, 0xff)
	writeRTCRegister(b, 0x0b, 0xff)
	writeRTCRegister(b, 0x0c, 0xff)
	latchRTC(b)

	for _, tc := range []struct {
		register byte
		want     byte
	}{
		{0x08, 0x3f},
		{0x09, 0x3f},
		{0x0a, 0x1f},
		{0x0b, 0xff},
		{0x0c, 0xc1},
	} {
		if got := readRTCRegister(b, tc.register); got != tc.want {
			t.Errorf("RTC register %#02x masked value = %#02x, want %#02x", tc.register, got, tc.want)
		}
	}
}

func TestMBC3RTCReadRequiresEnable(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x0147] = byte(MBC3TIMERRAMBATT)
	rom[0x0149] = 0x03
	b := NewBus(scheduler.NewScheduler(), rom)

	b.Write(0x4000, 0x08)
	if got := b.Read(0xa000); got != 0xff {
		t.Fatalf("disabled RTC read = %#02x, want 0xff", got)
	}

	b.Write(0x0000, 0x0a)
	writeRTCRegister(b, 0x08, 7)
	latchRTC(b)
	if got := readRTCRegister(b, 0x08); got != 7 {
		t.Fatalf("enabled RTC read = %d, want 7", got)
	}

	b.Write(0x0000, 0x00)
	if got := b.Read(0xa000); got != 0xff {
		t.Fatalf("RTC read after disable = %#02x, want 0xff", got)
	}
}

func TestMBC3ROMBankRegisterIsSevenBits(t *testing.T) {
	const banks = 128
	rom := make([]byte, banks*0x4000)
	rom[0x0147] = byte(MBC3)
	rom[0x0148] = 0x06 // 2 MiB
	for bank := 1; bank < banks; bank++ {
		rom[bank*0x4000] = byte(bank)
	}

	b := NewBus(scheduler.NewScheduler(), rom)
	b.Write(0x2000, 0x80) // high bit discarded -> 0 -> bank 1

	if got := b.Read(0x4000); got != 1 {
		t.Fatalf("MBC3 bank after writing 0x80 = %d, want bank 1", got)
	}
}

func TestMBC30CanSelectRAMBankSeven(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x0147] = byte(MBC3TIMERRAMBATT)
	rom[0x0149] = 0x05 // 64 KiB RAM / MBC30-style eight banks
	b := NewBus(scheduler.NewScheduler(), rom)

	b.Write(0x0000, 0x0a)
	b.Write(0x4000, 7)
	b.Write(0xa000, 0x77)

	b.Write(0x4000, 3)
	b.Write(0xa000, 0x33)

	b.Write(0x4000, 7)
	if got := b.Read(0xa000); got != 0x77 {
		t.Fatalf("RAM bank 7 value = %#02x, want 0x77", got)
	}
	b.Write(0x4000, 3)
	if got := b.Read(0xa000); got != 0x33 {
		t.Fatalf("RAM bank 3 value = %#02x, want 0x33", got)
	}
}

func TestMBC7AccelerometerLatchAndRAMGates(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x0147] = byte(MBC7)
	b := NewBus(scheduler.NewScheduler(), rom)

	b.Write(0x0000, 0x0a)
	if got := b.Read(0xa020); got != 0xff {
		t.Fatalf("MBC7 read with second RAM gate disabled = %#02x, want 0xff", got)
	}

	b.Write(0x4000, 0x40)
	b.c.AccelerometerX = 1
	b.c.AccelerometerY = 2
	b.Write(0xa000, 0x55)
	b.Write(0xa010, 0xaa)

	if got := uint16(b.Read(0xa020)) | uint16(b.Read(0xa030))<<8; got != 0x8240 {
		t.Errorf("latched X = %#04x, want 0x8240", got)
	}
	if got := uint16(b.Read(0xa040)) | uint16(b.Read(0xa050))<<8; got != 0x82b0 {
		t.Errorf("latched Y = %#04x, want 0x82b0", got)
	}

	// A second latch command without the required 0x55 reset must leave the
	// previous sample untouched.
	b.c.AccelerometerX = 3
	b.c.AccelerometerY = 4
	b.Write(0xa010, 0xaa)
	if got := uint16(b.Read(0xa020)) | uint16(b.Read(0xa030))<<8; got != 0x8240 {
		t.Errorf("X changed without latch reset: got %#04x, want 0x8240", got)
	}

	b.Write(0xa000, 0x55)
	if got := uint16(b.Read(0xa020)) | uint16(b.Read(0xa030))<<8; got != 0x8000 {
		t.Errorf("X after latch reset = %#04x, want 0x8000", got)
	}
	b.Write(0xa010, 0xaa)
	if got := uint16(b.Read(0xa020)) | uint16(b.Read(0xa030))<<8; got != 0x8320 {
		t.Errorf("X after reset + relatch = %#04x, want 0x8320", got)
	}
	if got := uint16(b.Read(0xa040)) | uint16(b.Read(0xa050))<<8; got != 0x8390 {
		t.Errorf("Y after reset + relatch = %#04x, want 0x8390", got)
	}
}

func newMBC7EEPROMTestBus(t *testing.T) *Bus {
	t.Helper()
	rom := make([]byte, 0x8000)
	rom[0x0147] = byte(MBC7)
	b := NewBus(scheduler.NewScheduler(), rom)
	b.Write(0x0000, 0x0a)
	b.Write(0x4000, 0x40)
	return b
}

func mbc7EEPROMClockBit(b *Bus, bit byte) byte {
	value := byte(0x80) // CS high, CLK low
	if bit != 0 {
		value |= 0x02 // DI
	}
	b.Write(0xa080, value)
	b.Write(0xa080, value|0x40)
	out := b.Read(0xa080) & 1
	b.Write(0xa080, value)
	return out
}

func mbc7EEPROMCommand(b *Bus, command uint16) {
	b.Write(0xa080, 0x00) // lower CS
	b.Write(0xa080, 0x80) // raise CS
	mbc7EEPROMClockBit(b, 1)
	for bit := 9; bit >= 0; bit-- {
		mbc7EEPROMClockBit(b, byte(command>>bit&1))
	}
}

func mbc7EEPROMData(b *Bus, value uint16) {
	for bit := 15; bit >= 0; bit-- {
		mbc7EEPROMClockBit(b, byte(value>>bit&1))
	}
	b.Write(0xa080, 0x00)
}

func mbc7EEPROMReadWord(b *Bus, address uint16) uint16 {
	mbc7EEPROMCommand(b, 0x200|address&0x7f) // READ 10xAAAAAAA
	var value uint16
	for range 16 {
		value = value<<1 | uint16(mbc7EEPROMClockBit(b, 0))
	}
	b.Write(0xa080, 0x00)
	return value
}

func mbc7EEPROMWriteWord(b *Bus, address, value uint16) {
	mbc7EEPROMCommand(b, 0x100|address&0x7f) // WRITE 01xAAAAAAA
	mbc7EEPROMData(b, value)
}

func TestMBC7EEPROMCommandsAndWriteEnable(t *testing.T) {
	b := newMBC7EEPROMTestBus(t)
	const address = uint16(3)

	// Programming commands are ignored until EWEN.
	mbc7EEPROMWriteWord(b, address, 0x1234)
	if got := mbc7EEPROMReadWord(b, address); got != 0x0000 {
		t.Fatalf("WRITE before EWEN changed EEPROM: got %#04x, want 0x0000", got)
	}

	mbc7EEPROMCommand(b, 0x0c0) // EWEN 0011xxxxxx
	b.Write(0xa080, 0x00)
	mbc7EEPROMWriteWord(b, address, 0x1234)
	if got := mbc7EEPROMReadWord(b, address); got != 0x1234 {
		t.Fatalf("READ after WRITE = %#04x, want 0x1234", got)
	}

	mbc7EEPROMCommand(b, 0x300|address) // ERASE 11xAAAAAAA
	b.Write(0xa080, 0x00)
	if got := mbc7EEPROMReadWord(b, address); got != 0xffff {
		t.Fatalf("READ after ERASE = %#04x, want 0xffff", got)
	}

	mbc7EEPROMCommand(b, 0x040) // WRAL 0001xxxxxx
	mbc7EEPROMData(b, 0x55aa)
	for _, address := range []uint16{0, 63, 127} {
		if got := mbc7EEPROMReadWord(b, address); got != 0x55aa {
			t.Errorf("WRAL address %d = %#04x, want 0x55aa", address, got)
		}
	}

	mbc7EEPROMCommand(b, 0x000) // EWDS 0000xxxxxx
	b.Write(0xa080, 0x00)
	mbc7EEPROMWriteWord(b, address, 0xbeef)
	if got := mbc7EEPROMReadWord(b, address); got != 0x55aa {
		t.Fatalf("WRITE after EWDS changed EEPROM: got %#04x, want 0x55aa", got)
	}
}

func TestHuC1IRModePreservesRAM(t *testing.T) {
	rom := make([]byte, 0x8000)
	rom[0x0147] = byte(HUDSONHUC1)
	rom[0x0149] = 0x03
	b := NewBus(scheduler.NewScheduler(), rom)

	b.Write(0x0000, 0x00) // any value other than 0x0e selects RAM mode
	b.Write(0xa000, 0x5a)
	if got := b.Read(0xa000); got != 0x5a {
		t.Fatalf("HuC1 RAM read = %#02x, want 0x5a", got)
	}

	b.Write(0x0000, 0x0e) // IR mode
	if got := b.Read(0xa000); got != 0xc0 {
		t.Fatalf("HuC1 IR read = %#02x, want 0xc0 (no light)", got)
	}
	b.Write(0xa000, 0xa5) // ignored while in IR mode

	b.Write(0x0000, 0x00)
	if got := b.Read(0xa000); got != 0x5a {
		t.Fatalf("HuC1 RAM changed while in IR mode: got %#02x, want 0x5a", got)
	}
}

func TestM161AllowsOneBankSwitchPerSession(t *testing.T) {
	rom := make([]byte, 8*0x8000)
	for bank := 0; bank < 8; bank++ {
		rom[bank*0x8000] = byte(bank)
		rom[bank*0x8000+0x4000] = byte(bank)
	}
	b := NewBus(scheduler.NewScheduler(), rom)
	b.c.CartridgeType = M161

	b.Write(0x1234, 5)
	if got := b.Read(0x0000); got != 5 {
		t.Fatalf("M161 first bank switch mapped %#02x, want 5", got)
	}
	if got := b.Read(0x4000); got != 5 {
		t.Fatalf("M161 upper half after first bank switch = %#02x, want 5", got)
	}

	b.Write(0x1234, 2)
	if got := b.Read(0x0000); got != 5 {
		t.Fatalf("M161 accepted a second bank switch: got bank marker %d, want 5", got)
	}
}

func TestPocketCameraRAMAndRegisterMapping(t *testing.T) {
	rom := make([]byte, 0x100000)
	rom[0x0147] = byte(POCKETCAMERA)
	rom[0x0148] = 0x05 // 1 MiB ROM
	rom[0x0149] = 0x04 // 128 KiB RAM
	b := NewBus(scheduler.NewScheduler(), rom)

	b.Write(0x0000, 0x0a)
	b.Write(0x4000, 2)
	b.Write(0xa000, 0x42)
	if got := b.Read(0xa000); got != 0x42 {
		t.Fatalf("camera RAM bank 2 value = %#02x, want 0x42", got)
	}

	b.Write(0x4000, 0x10) // map camera registers
	b.Write(0xa001, 0x7b)
	if got := b.c.Camera.Registers[1]; got != 0x7b {
		t.Fatalf("camera register 1 = %#02x, want 0x7b", got)
	}
	if got := b.Read(0xa001); got != 0x00 {
		t.Fatalf("write-only camera register read = %#02x, want 0x00", got)
	}

	b.Write(0x4000, 2)
	if got := b.Read(0xa000); got != 0x42 {
		t.Fatalf("camera RAM bank 2 changed after register mapping: got %#02x, want 0x42", got)
	}
}
