package bus

import "testing"

func testBus() *Bus {
	bios := make([]byte, BIOSSize)
	bios[0] = 0x12
	bios[BIOSSize-1] = 0x34
	rom := make([]byte, 0x400)
	for i := range rom {
		rom[i] = byte(i)
	}
	return New(bios, rom)
}



func TestOutOfBoundsGamePakReadsUseAddressPattern(t *testing.T) {
	b := New(nil, make([]byte, 4))
	const addr uint32 = 0x092468ac

	if got, _ := b.Read8(addr, Access{}); got != 0x56 {
		t.Fatalf("OOB ROM byte = %02x, want 56", got)
	}
	if got, _ := b.Read8(addr+1, Access{}); got != 0x34 {
		t.Fatalf("OOB ROM odd byte = %02x, want 34", got)
	}
	if got, _ := b.Read16(addr, Access{}); got != 0x3456 {
		t.Fatalf("OOB ROM halfword = %04x, want 3456", got)
	}
	if got, _ := b.Read16(addr+1, Access{}); got != 0x560034 {
		t.Fatalf("OOB ROM odd halfword = %08x, want 56000034", uint32(got))
	}
	if got, _ := b.Read32(addr, Access{}); got != 0x34573456 {
		t.Fatalf("OOB ROM word = %08x, want 34573456", got)
	}
	for offset, want := range []uint32{0x34573456, 0x56345734, 0x34563457, 0x57345634} {
		got, _ := b.Read32(addr+uint32(offset), Access{})
		if got != want {
			t.Fatalf("OOB ROM word +%d = %08x, want %08x", offset, got, want)
		}
	}
}

func TestProtectedBIOSReadsUsePrefetchLatchOutsideBIOS(t *testing.T) {
	bios := make([]byte, BIOSSize)
	bios[0], bios[1], bios[2], bios[3] = 0x11, 0x22, 0x33, 0x44
	b := New(bios, make([]byte, 4))
	b.SetBIOSPrefetch(0xe3a02004)

	if got, _ := b.Read8(BIOSStart, Access{}); got != 0x04 {
		t.Fatalf("protected BIOS byte = %02x, want 04", got)
	}
	if got, _ := b.Read16(BIOSStart, Access{}); got != 0x2004 {
		t.Fatalf("protected BIOS halfword = %04x, want 2004", got)
	}
	if got, _ := b.Read32(BIOSStart, Access{}); got != 0xe3a02004 {
		t.Fatalf("protected BIOS word = %08x, want e3a02004", got)
	}
}

func TestBIOSReadsRemainDirectWhileExecutingBIOS(t *testing.T) {
	bios := make([]byte, BIOSSize)
	bios[0], bios[1], bios[2], bios[3] = 0x11, 0x22, 0x33, 0x44
	bios[8], bios[9], bios[10], bios[11] = 0x78, 0x56, 0x34, 0x12
	rom := []byte{0, 0, 0, 0}
	b := New(bios, rom)

	if got, _ := b.Read32(BIOSStart, Access{Instruction: true}); got != 0x44332211 {
		t.Fatalf("BIOS instruction fetch = %08x, want 44332211", got)
	}
	if got, _ := b.Read32(BIOSStart, Access{}); got != 0x44332211 {
		t.Fatalf("BIOS data read while in BIOS = %08x, want 44332211", got)
	}
	if got := b.BIOSPrefetch(); got != 0x12345678 {
		t.Fatalf("BIOS prefetch latch = %08x, want 12345678", got)
	}

	b.Read32(ROM0Start, Access{Instruction: true})
	if got, _ := b.Read32(BIOSStart, Access{}); got != 0x12345678 {
		t.Fatalf("protected BIOS after leaving BIOS = %08x, want 12345678", got)
	}
}

func TestMemoryMapAndMirrors(t *testing.T) {
	b := testBus()

	if got := b.Peek8(BIOSStart); got != 0x12 {
		t.Fatalf("BIOS[0] = %02x, want 12", got)
	}
	if got := b.Peek8(BIOSStart+BIOSSize-1); got != 0x34 {
		t.Fatalf("BIOS[last] = %02x, want 34", got)
	}

	b.Write8(EWRAMStart+0x1234, 0xa1, Access{})
	if got := b.Peek8(EWRAMStart+EWRAMSize+0x1234); got != 0xa1 {
		t.Fatalf("EWRAM mirror = %02x, want a1", got)
	}

	b.Write8(IWRAMStart+0x321, 0xb2, Access{})
	if got := b.Peek8(IWRAMStart+IWRAMSize*7+0x321); got != 0xb2 {
		t.Fatalf("IWRAM mirror = %02x, want b2", got)
	}

	b.Write16(PaletteStart+0x22, 0x4433, Access{})
	if got, _ := b.Read16(PaletteStart+0x100000+0x22, Access{}); got != 0x4433 {
		t.Fatalf("palette mirror = %04x, want 4433", got)
	}

	b.Write16(VRAMStart+0x10010, 0x6655, Access{})
	// 0x18000-0x1ffff mirrors 0x10000-0x17fff.
	if got, _ := b.Read16(VRAMStart+0x18010, Access{}); got != 0x6655 {
		t.Fatalf("VRAM upper mirror = %04x, want 6655", got)
	}
	// The 128KB pattern repeats through the whole 0x06 region.
	if got, _ := b.Read16(VRAMStart+0x20000+0x10010, Access{}); got != 0x6655 {
		t.Fatalf("VRAM 128K repeat = %04x, want 6655", got)
	}

	b.Write16(OAMStart+0x30, 0x8877, Access{})
	if got, _ := b.Read16(OAMStart+0x400+0x30, Access{}); got != 0x8877 {
		t.Fatalf("OAM mirror = %04x, want 8877", got)
	}
}

func TestROMWaitStateWindowsMapSameCartridge(t *testing.T) {
	b := testBus()
	for _, base := range []uint32{ROM0Start, ROM1Start, ROM2Start} {
		got, _ := b.Read32(base+0x20, Access{})
		if want := uint32(0x23222120); got != want {
			t.Fatalf("ROM window %08x = %08x, want %08x", base, got, want)
		}
	}
}

func TestBIOSAndIOAreNotGenerallyMirrored(t *testing.T) {
	b := testBus()
	b.SetOpenBus(0xaabbccdd)

	got, _ := b.Read8(BIOSStart+BIOSSize, Access{})
	if got != 0xdd {
		t.Fatalf("read after BIOS = %02x, want open bus dd", got)
	}

	b.IO().Write16(0x20, 0x1234)
	if got, _ := b.Read16(IOStart+0x20, Access{}); got != 0x1234 {
		t.Fatalf("I/O register = %04x, want 1234", got)
	}
	b.SetOpenBus(0x55667788)
	if got, _ := b.Read16(IOStart+IOSize+0x20, Access{}); got != 0x7788 {
		t.Fatalf("I/O mirror read = %04x, want open bus 7788", got)
	}
}

func TestWidthAndAlignmentBehavior(t *testing.T) {
	b := testBus()

	b.Write32(IWRAMStart+1, 0x44332211, Access{})
	if got, _ := b.Read32(IWRAMStart, Access{}); got != 0x44332211 {
		t.Fatalf("misaligned STR word = %08x, want 44332211 at aligned address", got)
	}

	b.Write32(IWRAMStart, 0x44332211, Access{})
	if got, _ := b.Read32(IWRAMStart+1, Access{}); got != 0x11443322 {
		t.Fatalf("misaligned LDR rotate = %08x, want 11443322", got)
	}

	b.Write16(IWRAMStart+4, 0xaa55, Access{})
	if got, _ := b.Read16(IWRAMStart+5, Access{}); got != 0x55aa {
		t.Fatalf("odd LDRH rotate = %04x, want 55aa", got)
	}
}

func TestVideoByteWriteRules(t *testing.T) {
	b := testBus()

	b.Write8(PaletteStart+1, 0x5a, Access{})
	if got, _ := b.Read16(PaletteStart, Access{}); got != 0x5a5a {
		t.Fatalf("palette STRB = %04x, want 5a5a", got)
	}

	b.Write8(VRAMStart+0x20, 0x6b, Access{})
	if got, _ := b.Read16(VRAMStart+0x20, Access{}); got != 0x6b6b {
		t.Fatalf("BG VRAM STRB = %04x, want 6b6b", got)
	}

	b.Write16(VRAMStart+0x10020, 0x1234, Access{})
	b.Write8(VRAMStart+0x10020, 0x7c, Access{})
	if got, _ := b.Read16(VRAMStart+0x10020, Access{}); got != 0x1234 {
		t.Fatalf("OBJ VRAM STRB changed value to %04x", got)
	}

	b.SetOBJVRAMStart(0x14000)
	b.Write8(VRAMStart+0x12000, 0x8d, Access{})
	if got, _ := b.Read16(VRAMStart+0x12000, Access{}); got != 0x8d8d {
		t.Fatalf("bitmap BG VRAM STRB = %04x, want 8d8d", got)
	}

	b.Write16(OAMStart, 0xbeef, Access{})
	b.Write8(OAMStart, 0x99, Access{})
	if got, _ := b.Read16(OAMStart, Access{}); got != 0xbeef {
		t.Fatalf("OAM STRB changed value to %04x", got)
	}
}

func TestOpenBusLatch(t *testing.T) {
	b := testBus()
	b.Write32(IWRAMStart, 0x11223344, Access{})
	got, _ := b.Read32(0x01000000, Access{})
	if got != 0x11223344 {
		t.Fatalf("open bus word = %08x, want 11223344", got)
	}

	b.SetOpenBus(0x11223344)
	got8, _ := b.Read8(0x01000002, Access{})
	if got8 != 0x22 {
		t.Fatalf("open bus byte lane = %02x, want 22", got8)
	}
}

type fakeSave struct {
	data [16]byte
}

func (s *fakeSave) Read8(addr uint32) byte {
	return s.data[addr%uint32(len(s.data))]
}
func (s *fakeSave) Write8(addr uint32, value byte) {
	s.data[addr%uint32(len(s.data))] = value
}

func TestSaveDeviceBoundary(t *testing.T) {
	b := testBus()
	save := &fakeSave{}
	b.AttachSaveDevice(save)

	b.Write8(SaveStart+3, 0x9a, Access{})
	got, cycles := b.Read8(SaveStart+3, Access{})
	if got != 0x9a {
		t.Fatalf("save read = %02x, want 9a", got)
	}
	if cycles != 5 {
		t.Fatalf("default SRAM cycles = %d, want 5", cycles)
	}

	got16, cycles16 := b.Read16(SaveStart+3, Access{})
	if got16 != 0x9a9a || cycles16 != 5 {
		t.Fatalf("save halfword = %04x cycles=%d, want 9a9a/5", got16, cycles16)
	}
	got32, cycles32 := b.Read32(SaveStart+3, Access{})
	if got32 != 0x9a9a9a9a || cycles32 != 5 {
		t.Fatalf("save word = %08x cycles=%d, want 9a9a9a9a/5", got32, cycles32)
	}

	b.Write32(SaveStart+2, 0x44332211, Access{})
	if got := save.Read8(2); got != 0x33 {
		t.Fatalf("save word write selected byte = %02x, want 33", got)
	}
}


func TestDMAAccessCannotUseGamePakSaveBus(t *testing.T) {
	b := testBus()
	save := &fakeSave{}
	b.AttachSaveDevice(save)
	save.data[2] = 0x5a

	b.SetOpenBus(0x44332211)
	got, cycles := b.Read16(SaveStart+2, Access{DMA: true})
	if got != 0x4433 {
		t.Fatalf("DMA save read = %04x, want open-bus 4433", got)
	}
	if cycles != 5 {
		t.Fatalf("DMA save read cycles = %d, want configured save-bus timing 5", cycles)
	}

	b.Write16(SaveStart+2, 0xbeef, Access{DMA: true})
	if got := save.Read8(2); got != 0x5a {
		t.Fatalf("DMA save write reached save device: %02x, want 5a", got)
	}

	// CPU accesses remain unchanged.
	if got, _ := b.Read8(SaveStart+2, Access{}); got != 0x5a {
		t.Fatalf("CPU save read changed by DMA restriction: %02x", got)
	}
}
