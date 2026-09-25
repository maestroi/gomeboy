package cpu

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbamemory "github.com/maestroi/gomeboy/internal/gba/memory"
)

func armSwap(byteSwap bool, rn, rd, rm uint32) uint32 {
	instruction := uint32(0xe1000090) | rn<<16 | rd<<12 | rm
	if byteSwap {
		instruction |= 1 << 22
	}
	return instruction
}

func TestARMSwapWord(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}

	address := uint32(bus.IWRAMStart + 0x100)
	b.Write32(address, 0x11223344, bus.Access{})
	c.WriteRegister(0, address)
	c.WriteRegister(1, 0xaabbccdd)

	result, err := c.ExecuteARMWithMemory(armSwap(false, 0, 2, 1), b)
	if err != nil {
		t.Fatal(err)
	}

	if got := c.ReadRegister(2); got != 0x11223344 {
		t.Fatalf("SWP destination = %08x, want 11223344", got)
	}
	if got, _ := b.Read32(address, bus.Access{}); got != 0xaabbccdd {
		t.Fatalf("SWP memory = %08x, want aabbccdd", got)
	}
	if result.MemoryCycles != 2 || result.InternalCycles != 1 {
		t.Fatalf("SWP timing = %+v, want memory=2 internal=1", result)
	}
	if got := c.PC(); got != 4 {
		t.Fatalf("SWP PC = %08x, want 00000004", got)
	}
}

func TestARMSwapByte(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}

	address := uint32(bus.IWRAMStart + 0x123)
	b.Write8(address, 0x5a, bus.Access{})
	c.WriteRegister(0, address)
	c.WriteRegister(1, 0x123456e7)

	result, err := c.ExecuteARMWithMemory(armSwap(true, 0, 2, 1), b)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(2); got != 0x5a {
		t.Fatalf("SWPB destination = %08x, want 0000005a", got)
	}
	if got, _ := b.Read8(address, bus.Access{}); got != 0xe7 {
		t.Fatalf("SWPB memory = %02x, want e7", got)
	}
	if result.MemoryCycles != 2 || result.InternalCycles != 1 {
		t.Fatalf("SWPB timing = %+v, want memory=2 internal=1", result)
	}
}

func TestARMSwapAllowsSameSourceAndDestination(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}

	address := uint32(bus.IWRAMStart + 0x180)
	b.Write32(address, 0x01020304, bus.Access{})
	c.WriteRegister(0, address)
	c.WriteRegister(1, 0xa0b0c0d0)

	if _, err := c.ExecuteARMWithMemory(armSwap(false, 0, 1, 1), b); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(1); got != 0x01020304 {
		t.Fatalf("SWP same Rd/Rm register = %08x, want old memory", got)
	}
	if got, _ := b.Read32(address, bus.Access{}); got != 0xa0b0c0d0 {
		t.Fatalf("SWP same Rd/Rm memory = %08x, want original register", got)
	}
}

func TestARMSwapMisalignedWordUsesLDRSTRAlignmentRules(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}

	base := uint32(bus.IWRAMStart + 0x200)
	b.Write32(base, 0x44332211, bus.Access{})
	c.WriteRegister(0, base+1)
	c.WriteRegister(1, 0xaabbccdd)

	if _, err := c.ExecuteARMWithMemory(armSwap(false, 0, 2, 1), b); err != nil {
		t.Fatal(err)
	}

	if got := c.ReadRegister(2); got != 0x11443322 {
		t.Fatalf("misaligned SWP loaded = %08x, want rotated 11443322", got)
	}
	if got, _ := b.Read32(base, bus.Access{}); got != 0xaabbccdd {
		t.Fatalf("misaligned SWP store = %08x, want aligned aabbccdd", got)
	}
}

type swapSpyMemory struct {
	word       uint32
	readAccess  []gbamemory.Access
	writeAccess []gbamemory.Access
}

func (m *swapSpyMemory) Read8(_ uint32, access gbamemory.Access) (byte, uint32) {
	m.readAccess = append(m.readAccess, access)
	return byte(m.word), 3
}
func (m *swapSpyMemory) Read16(_ uint32, access gbamemory.Access) (uint16, uint32) {
	m.readAccess = append(m.readAccess, access)
	return uint16(m.word), 3
}
func (m *swapSpyMemory) Read32(_ uint32, access gbamemory.Access) (uint32, uint32) {
	m.readAccess = append(m.readAccess, access)
	return m.word, 3
}
func (m *swapSpyMemory) Write8(_ uint32, value byte, access gbamemory.Access) uint32 {
	m.writeAccess = append(m.writeAccess, access)
	m.word = uint32(value)
	return 4
}
func (m *swapSpyMemory) Write16(_ uint32, value uint16, access gbamemory.Access) uint32 {
	m.writeAccess = append(m.writeAccess, access)
	m.word = uint32(value)
	return 4
}
func (m *swapSpyMemory) Write32(_ uint32, value uint32, access gbamemory.Access) uint32 {
	m.writeAccess = append(m.writeAccess, access)
	m.word = value
	return 4
}
func (m *swapSpyMemory) Idle(uint32) {}

func TestARMSwapMarksReadAndWriteLockedAndNonSequential(t *testing.T) {
	mem := &swapSpyMemory{word: 0x11112222}
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(0, 0x02000000)
	c.WriteRegister(1, 0x33334444)

	result, err := c.ExecuteARMWithMemory(armSwap(false, 0, 2, 1), mem)
	if err != nil {
		t.Fatal(err)
	}

	if len(mem.readAccess) != 1 || len(mem.writeAccess) != 1 {
		t.Fatalf("access count read=%d write=%d, want 1/1", len(mem.readAccess), len(mem.writeAccess))
	}
	for _, access := range []gbamemory.Access{mem.readAccess[0], mem.writeAccess[0]} {
		if !access.Locked {
			t.Fatalf("SWP access not marked locked: %+v", access)
		}
		if access.Sequential || access.Instruction {
			t.Fatalf("SWP data access flags = %+v, want locked non-sequential data", access)
		}
	}
	if result.MemoryCycles != 7 {
		t.Fatalf("SWP spy memory cycles = %d, want read3+write4=7", result.MemoryCycles)
	}
}

func TestARMSwapRejectsUnpredictableRegisterCombinations(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		rn, rd, rm uint32
	}{
		{"PC base", 15, 0, 1},
		{"PC destination", 0, 15, 1},
		{"PC source", 0, 1, 15},
		{"base equals destination", 0, 0, 1},
		{"base equals source", 0, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.ExecuteARMWithMemory(armSwap(false, tc.rn, tc.rd, tc.rm), b); err == nil {
				t.Fatal("unpredictable SWP combination was accepted")
			}
		})
	}
}

func TestARMSwapConditionFailureDoesNotNeedMemory(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(0x100)

	instruction := armSwap(false, 0, 1, 2)
	instruction &^= 0xf << 28 // EQ, with Z clear.
	if _, err := c.ExecuteARM(instruction); err != nil {
		t.Fatalf("condition-failed SWP required memory: %v", err)
	}
	if got := c.PC(); got != 0x104 {
		t.Fatalf("condition-failed SWP PC=%08x, want 00000104", got)
	}
}

func TestARMSwapRequiresMemoryWhenExecuted(t *testing.T) {
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ExecuteARM(armSwap(false, 0, 1, 2)); err != ErrMemoryRequired {
		t.Fatalf("SWP without memory error=%v, want ErrMemoryRequired", err)
	}
}

func TestARMSwapStepAccountsFetchDataAndInternalCycles(t *testing.T) {
	rom := make([]byte, 8)
	putARM(rom, 0, armSwap(false, 0, 2, 1))
	b := newExecutionBus(rom)

	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.SetPC(bus.ROM0Start)
	c.WriteRegister(0, bus.IWRAMStart)
	c.WriteRegister(1, 0xaabbccdd)
	b.Write32(bus.IWRAMStart, 0x11223344, bus.Access{})

	result, err := c.Step(b)
	if err != nil {
		t.Fatal(err)
	}
	if result.FetchCycles != 8 || result.MemoryCycles != 2 || result.InternalCycles != 1 || result.TotalCycles != 11 {
		t.Fatalf("SWP Step timing=%+v, want fetch=8 memory=2 internal=1 total=11", result)
	}
	if c.ReadRegister(2) != 0x11223344 {
		t.Fatalf("SWP Step r2=%08x, want 11223344", c.ReadRegister(2))
	}
	if got, _ := b.Read32(bus.IWRAMStart, bus.Access{}); got != 0xaabbccdd {
		t.Fatalf("SWP Step memory=%08x, want aabbccdd", got)
	}
}
