package cpu

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func putARM(rom []byte, offset int, instruction uint32) {
	binary.LittleEndian.PutUint32(rom[offset:offset+4], instruction)
}

func putThumb(rom []byte, offset int, instruction uint16) {
	binary.LittleEndian.PutUint16(rom[offset:offset+2], instruction)
}

func newExecutionBus(rom []byte) *bus.Bus {
	return bus.New(make([]byte, bus.BIOSSize), rom)
}

func TestARMStepFetchesExecutesAndAccountsBusCycles(t *testing.T) {
	rom := make([]byte, 0x40)
	putARM(rom, 0x00, 0xe5801000) // STR r1,[r0]
	putARM(rom, 0x04, 0xe5902000) // LDR r2,[r0]
	putARM(rom, 0x08, 0xe2823001) // ADD r3,r2,#1

	b := newExecutionBus(rom)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)
	c.WriteRegister(0, bus.IWRAMStart)
	c.WriteRegister(1, 0x11223344)

	first, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Read32(bus.IWRAMStart, bus.Access{}); got != 0x11223344 {
		t.Fatalf("STR result = %08x, want 11223344", got)
	}
	if first.FetchCycles != 8 || first.MemoryCycles != 1 || first.InternalCycles != 1 || first.TotalCycles != 10 {
		t.Fatalf("first step timing = %+v, want fetch=8 memory=1 internal=1 total=10", first)
	}

	second, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0x11223344 {
		t.Fatalf("LDR r2 = %08x, want 11223344", got)
	}
	if second.FetchCycles != 6 || second.MemoryCycles != 1 || second.TotalCycles != 8 {
		t.Fatalf("second step timing = %+v, want sequential fetch=6 memory=1 total=8", second)
	}

	third, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(3); got != 0x11223345 {
		t.Fatalf("ADD r3 = %08x, want 11223345", got)
	}
	if third.FetchCycles != 6 || third.MemoryCycles != 0 || third.TotalCycles != 7 {
		t.Fatalf("third step timing = %+v, want fetch=6 internal=1 total=7", third)
	}
	if got := c.PC(); got != bus.ROM0Start+12 {
		t.Fatalf("PC = %08x, want %08x", got, bus.ROM0Start+12)
	}
}

func TestStepPipelineFlushMakesNextFetchNonSequential(t *testing.T) {
	rom := make([]byte, 0x20)
	putARM(rom, 0x00, 0xea000000) // B +0 -> current PC +8
	putARM(rom, 0x08, 0xe3a00007) // MOV r0,#7

	b := newExecutionBus(rom)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)

	branch, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if !branch.PipelineFlush || c.PC() != bus.ROM0Start+8 {
		t.Fatalf("branch result=%+v PC=%08x", branch, c.PC())
	}

	next, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if next.FetchCycles != 8 {
		t.Fatalf("post-branch fetch = %d cycles, want non-sequential 8", next.FetchCycles)
	}
	if got := c.ReadRegister(0); got != 7 {
		t.Fatalf("post-branch MOV r0=%d, want 7", got)
	}
}

func TestARMWordTransferAlignmentAndWriteback(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}

	b.Write32(bus.IWRAMStart, 0x44332211, bus.Access{})
	c.WriteRegister(0, bus.IWRAMStart+1)

	// LDR r2,[r0],#4: the bus performs ARM7 misaligned word rotation and r0
	// receives the post-indexed address.
	result, err := c.ExecuteARMWithMemory(0xe4902004, b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0x11443322 {
		t.Fatalf("misaligned LDR = %08x, want 11443322", got)
	}
	if got := c.ReadRegister(0); got != bus.IWRAMStart+5 {
		t.Fatalf("post-index r0 = %08x, want %08x", got, bus.IWRAMStart+5)
	}
	if result.MemoryCycles != 1 {
		t.Fatalf("IWRAM LDR memory cycles = %d, want 1", result.MemoryCycles)
	}

	c.WriteRegister(1, 0xaabbccdd)
	// STR r1,[r0,#3]!: effective address +3 is aligned down by the bus;
	// writeback keeps the architectural unaligned address.
	if _, err := c.ExecuteARMWithMemory(0xe5a01003, b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != bus.IWRAMStart+8 {
		t.Fatalf("pre-index writeback r0 = %08x, want %08x", got, bus.IWRAMStart+8)
	}
	if got, _ := b.Read32(bus.IWRAMStart+8, bus.Access{}); got != 0xaabbccdd {
		t.Fatalf("STR memory = %08x, want aabbccdd", got)
	}
}

func armHalfTransfer(load, immediate, pre, up, writeback bool, rn, rd uint32, kind uint8, offset uint32) uint32 {
	instruction := uint32(0xe0000090) |
		rn<<16 | rd<<12 | uint32(kind&3)<<5
	if load {
		instruction |= 1 << 20
	}
	if immediate {
		instruction |= 1 << 22
		instruction |= ((offset >> 4) & 0xf) << 8
		instruction |= offset & 0xf
	} else {
		instruction |= offset & 0xf
	}
	if pre {
		instruction |= 1 << 24
	}
	if up {
		instruction |= 1 << 23
	}
	if writeback {
		instruction |= 1 << 21
	}
	return instruction
}

func TestARMHalfwordAndSignedTransfers(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(0, bus.IWRAMStart+0x100)

	b.Write16(bus.IWRAMStart+0x102, 0x80ff, bus.Access{})
	ldrh := armHalfTransfer(true, true, true, true, false, 0, 1, 1, 2)
	if _, err := c.ExecuteARMWithMemory(ldrh, b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(1); got != 0x000080ff {
		t.Fatalf("LDRH = %08x, want 000080ff", got)
	}

	ldrsh := armHalfTransfer(true, true, true, true, false, 0, 2, 3, 2)
	if _, err := c.ExecuteARMWithMemory(ldrsh, b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0xffff80ff {
		t.Fatalf("LDRSH = %08x, want ffff80ff", got)
	}

	b.Write8(bus.IWRAMStart+0x103, 0x80, bus.Access{})
	oddLDRSH := armHalfTransfer(true, true, true, true, false, 0, 3, 3, 3)
	if _, err := c.ExecuteARMWithMemory(oddLDRSH, b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(3); got != 0xffffff80 {
		t.Fatalf("odd LDRSH = %08x, want ffffff80 signed byte", got)
	}

	ldrsb := armHalfTransfer(true, true, true, true, false, 0, 4, 2, 3)
	if _, err := c.ExecuteARMWithMemory(ldrsb, b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(4); got != 0xffffff80 {
		t.Fatalf("LDRSB = %08x, want ffffff80", got)
	}

	c.WriteRegister(5, 0xabcd1234)
	strh := armHalfTransfer(false, true, true, true, false, 0, 5, 1, 4)
	if _, err := c.ExecuteARMWithMemory(strh, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Read16(bus.IWRAMStart+0x104, bus.Access{}); got != 0x1234 {
		t.Fatalf("STRH = %04x, want 1234", got)
	}
}

func TestARMTransferNeedsMemoryOnlyWhenConditionPasses(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x100)

	if _, err := c.ExecuteARM(0xe5900000); !errors.Is(err, ErrMemoryRequired) {
		t.Fatalf("unconditional LDR error = %v, want ErrMemoryRequired", err)
	}

	// LDREQ with Z clear is skipped without touching memory.
	if _, err := c.ExecuteARM(0x05900000); err != nil {
		t.Fatalf("failed-condition LDR unexpectedly needed memory: %v", err)
	}
	if c.PC() != 0x104 {
		t.Fatalf("condition-failed LDR PC=%08x, want 00000104", c.PC())
	}
}

func TestThumbStepRunsLoadStoreProgram(t *testing.T) {
	rom := make([]byte, 0x20)
	putThumb(rom, 0x00, 0x6001) // STR r1,[r0,#0]
	putThumb(rom, 0x02, 0x6802) // LDR r2,[r0,#0]
	putThumb(rom, 0x04, 0x3201) // ADD r2,#1

	b := newExecutionBus(rom)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetThumb(true)
	c.SetPC(bus.ROM0Start)
	c.WriteRegister(0, bus.IWRAMStart)
	c.WriteRegister(1, 0x55667788)

	first, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if first.FetchCycles != 5 || first.MemoryCycles != 1 || first.TotalCycles != 7 {
		t.Fatalf("Thumb STR step = %+v, want fetch=5 memory=1 internal=1 total=7", first)
	}

	second, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0x55667788 {
		t.Fatalf("Thumb LDR r2 = %08x, want 55667788", got)
	}
	if second.FetchCycles != 3 || second.MemoryCycles != 1 || second.TotalCycles != 5 {
		t.Fatalf("Thumb LDR step = %+v, want fetch=3 memory=1 total=5", second)
	}

	if _, err := c.Step(b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0x55667789 {
		t.Fatalf("Thumb ADD r2 = %08x, want 55667789", got)
	}
}

func thumbRegisterTransfer(op uint16, rm, rn, rd uint16) uint16 {
	return 0x5000 | (op&7)<<9 | (rm&7)<<6 | (rn&7)<<3 | (rd & 7)
}

func TestThumbSignedAndHalfwordRegisterTransfers(t *testing.T) {
	b := newExecutionBus(nil)
	c := newThumbCPU(t)
	c.WriteRegister(0, bus.IWRAMStart+0x200)
	c.WriteRegister(1, 1)

	b.Write8(bus.IWRAMStart+0x201, 0x80, bus.Access{})
	if _, err := c.ExecuteThumbWithMemory(thumbRegisterTransfer(3, 1, 0, 2), b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0xffffff80 {
		t.Fatalf("Thumb LDRSB = %08x, want ffffff80", got)
	}

	if _, err := c.ExecuteThumbWithMemory(thumbRegisterTransfer(7, 1, 0, 3), b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(3); got != 0xffffff80 {
		t.Fatalf("Thumb odd LDRSH = %08x, want ffffff80", got)
	}

	c.WriteRegister(4, 0x1234)
	c.WriteRegister(1, 2)
	if _, err := c.ExecuteThumbWithMemory(thumbRegisterTransfer(1, 1, 0, 4), b); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Read16(bus.IWRAMStart+0x202, bus.Access{}); got != 0x1234 {
		t.Fatalf("Thumb STRH = %04x, want 1234", got)
	}
}

func TestThumbLiteralAndSPRelativeTransfers(t *testing.T) {
	rom := make([]byte, 0x20)
	putThumb(rom, 0x00, 0x4800) // LDR r0,[PC,#0] -> ROM+4
	binary.LittleEndian.PutUint32(rom[4:8], 0x12345678)

	b := newExecutionBus(rom)
	c := newThumbCPU(t)
	c.SetPC(bus.ROM0Start)

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != 0x12345678 {
		t.Fatalf("literal LDR r0 = %08x, want 12345678", got)
	}
	if result.MemoryCycles != 8 {
		t.Fatalf("literal ROM data cycles = %d, want non-sequential word 8", result.MemoryCycles)
	}

	c.WriteRegister(13, bus.IWRAMStart+0x300)
	c.WriteRegister(1, 0xaabbccdd)
	if _, err := c.ExecuteThumbWithMemory(0x9101, b); err != nil { // STR r1,[SP,#4]
		t.Fatal(err)
	}
	if got, _ := b.Read32(bus.IWRAMStart+0x304, bus.Access{}); got != 0xaabbccdd {
		t.Fatalf("SP-relative STR = %08x, want aabbccdd", got)
	}
	if _, err := c.ExecuteThumbWithMemory(0x9a01, b); err != nil { // LDR r2,[SP,#4]
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0xaabbccdd {
		t.Fatalf("SP-relative LDR r2 = %08x, want aabbccdd", got)
	}
}

func TestThumbAddressHelpers(t *testing.T) {
	c := newThumbCPU(t)
	c.SetPC(0x102)
	c.WriteRegister(13, 0x03007f00)

	if _, err := c.ExecuteThumb(0xa001); err != nil { // ADD r0,PC,#4
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != 0x108 {
		t.Fatalf("Thumb PC address = %08x, want 00000108", got)
	}

	if _, err := c.ExecuteThumb(0xa901); err != nil { // ADD r1,SP,#4
		t.Fatal(err)
	}
	if got := c.ReadRegister(1); got != 0x03007f04 {
		t.Fatalf("Thumb SP address = %08x, want 03007f04", got)
	}

	if _, err := c.ExecuteThumb(0xb081); err != nil { // SUB SP,#4
		t.Fatal(err)
	}
	if got := c.ReadRegister(13); got != 0x03007efc {
		t.Fatalf("Thumb adjusted SP = %08x, want 03007efc", got)
	}
}
