package cpu

import (
	"encoding/binary"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func armBlockTransfer(pre, up, user, writeBack, load bool, rn uint32, list uint16) uint32 {
	instruction := uint32(0xe8000000) | rn<<16 | uint32(list)
	if pre {
		instruction |= 1 << 24
	}
	if up {
		instruction |= 1 << 23
	}
	if user {
		instruction |= 1 << 22
	}
	if writeBack {
		instruction |= 1 << 21
	}
	if load {
		instruction |= 1 << 20
	}
	return instruction
}

func TestARMBlockTransferAddressingModes(t *testing.T) {
	cases := []struct {
		name       string
		pre, up    bool
		startDelta int32
		wbDelta    int32
	}{
		{"IA", false, true, 0, 12},
		{"IB", true, true, 4, 12},
		{"DA", false, false, -8, -12},
		{"DB", true, false, -12, -12},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newExecutionBus(nil)
			c := New()
			if err := c.SetMode(ModeSystem); err != nil {
				t.Fatal(err)
			}
			base := uint32(bus.IWRAMStart + 0x200)
			c.WriteRegister(0, base)
			c.WriteRegister(1, 0x11111111)
			c.WriteRegister(3, 0x33333333)
			c.WriteRegister(7, 0x77777777)

			result, err := c.ExecuteARMWithMemory(
				armBlockTransfer(tc.pre, tc.up, false, true, false, 0, (1<<1)|(1<<3)|(1<<7)),
				b,
			)
			if err != nil {
				t.Fatal(err)
			}

			start := uint32(int64(base) + int64(tc.startDelta))
			for i, want := range []uint32{0x11111111, 0x33333333, 0x77777777} {
				got, _ := b.Read32(start+uint32(i*4), bus.Access{})
				if got != want {
					t.Fatalf("word %d at %08x = %08x, want %08x", i, start+uint32(i*4), got, want)
				}
			}
			wantBase := uint32(int64(base) + int64(tc.wbDelta))
			if got := c.ReadRegister(0); got != wantBase {
				t.Fatalf("writeback = %08x, want %08x", got, wantBase)
			}
			if result.MemoryCycles != 3 || result.InternalCycles != 0 {
				t.Fatalf("STM timing = %+v, want 3 memory cycles and 0 internal", result)
			}
		})
	}
}

func TestARMLDMUsesNonSequentialThenSequentialDataAccesses(t *testing.T) {
	rom := make([]byte, 16)
	binary.LittleEndian.PutUint32(rom[0:4], 0x11223344)
	binary.LittleEndian.PutUint32(rom[4:8], 0x55667788)

	b := newExecutionBus(rom)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(0, bus.ROM0Start)

	result, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, false, true, true, 0, (1<<1)|(1<<2)),
		b,
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.ReadRegister(1) != 0x11223344 || c.ReadRegister(2) != 0x55667788 {
		t.Fatalf("LDM values r1=%08x r2=%08x", c.ReadRegister(1), c.ReadRegister(2))
	}
	// Default WS0 word timing is 8 cycles non-sequential and 6 sequential.
	if result.MemoryCycles != 14 || result.InternalCycles != 1 {
		t.Fatalf("LDM timing = %+v, want memory=14 internal=1", result)
	}
	if got := c.ReadRegister(0); got != bus.ROM0Start+8 {
		t.Fatalf("LDM writeback = %08x, want %08x", got, bus.ROM0Start+8)
	}
}

func TestARMBlockTransferIgnoresLowAddressBits(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	b.Write32(bus.IWRAMStart+0x100, 0x44332211, bus.Access{})
	c.WriteRegister(0, bus.IWRAMStart+0x101)

	if _, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, false, false, true, 0, 1<<1),
		b,
	); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(1); got != 0x44332211 {
		t.Fatalf("unaligned-base LDM = %08x, want unrotated aligned word", got)
	}
}

func TestARMSTMBaseInListUsesOldOrUpdatedBaseByPosition(t *testing.T) {
	t.Run("base first", func(t *testing.T) {
		b := newExecutionBus(nil)
		c := New()
		if err := c.SetMode(ModeSystem); err != nil {
			t.Fatal(err)
		}
		base := uint32(bus.IWRAMStart + 0x300)
		c.WriteRegister(0, base)
		c.WriteRegister(1, 0x11111111)

		if _, err := c.ExecuteARMWithMemory(
			armBlockTransfer(false, true, false, true, false, 0, (1<<0)|(1<<1)),
			b,
		); err != nil {
			t.Fatal(err)
		}
		if got, _ := b.Read32(base, bus.Access{}); got != base {
			t.Fatalf("first base store = %08x, want old base %08x", got, base)
		}
	})

	t.Run("base later", func(t *testing.T) {
		b := newExecutionBus(nil)
		c := New()
		if err := c.SetMode(ModeSystem); err != nil {
			t.Fatal(err)
		}
		base := uint32(bus.IWRAMStart + 0x340)
		c.WriteRegister(0, 0xaaaaaaaa)
		c.WriteRegister(1, base)

		if _, err := c.ExecuteARMWithMemory(
			armBlockTransfer(false, true, false, true, false, 1, (1<<0)|(1<<1)),
			b,
		); err != nil {
			t.Fatal(err)
		}
		if got, _ := b.Read32(base+4, bus.Access{}); got != base+8 {
			t.Fatalf("later base store = %08x, want updated base %08x", got, base+8)
		}
	})
}

func TestARMLDMBaseInListLoadedValueWinsWriteback(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	base := uint32(bus.IWRAMStart + 0x380)
	b.Write32(base, 0x12345678, bus.Access{})
	b.Write32(base+4, 0xaabbccdd, bus.Access{})
	c.WriteRegister(0, base)

	if _, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, false, true, true, 0, (1<<0)|(1<<1)),
		b,
	); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(0); got != 0x12345678 {
		t.Fatalf("base-in-list LDM r0 = %08x, want loaded value", got)
	}
	if got := c.ReadRegister(1); got != 0xaabbccdd {
		t.Fatalf("LDM r1 = %08x, want aabbccdd", got)
	}
}

func TestARMBlockTransferUserBankInFIQMode(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(8, 0x11111111) // user/system r8
	if err := c.SetMode(ModeFIQ); err != nil {
		t.Fatal(err)
	}
	c.WriteRegister(8, 0x22222222) // banked FIQ r8
	c.WriteRegister(0, bus.IWRAMStart+0x400)

	// STMIA r0,{r8}^ stores the user-bank r8.
	if _, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, true, false, false, 0, 1<<8),
		b,
	); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.Read32(bus.IWRAMStart+0x400, bus.Access{}); got != 0x11111111 {
		t.Fatalf("STM^ user r8 = %08x, want 11111111", got)
	}

	b.Write32(bus.IWRAMStart+0x400, 0x33333333, bus.Access{})
	if _, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, true, false, true, 0, 1<<8),
		b,
	); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(8); got != 0x22222222 {
		t.Fatalf("LDM^ changed current FIQ r8 = %08x", got)
	}
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	if got := c.ReadRegister(8); got != 0x33333333 {
		t.Fatalf("LDM^ user r8 = %08x, want 33333333", got)
	}
}

func TestARMLDMPCWithSRestoresCPSR(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSupervisor); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSPSR(PSR(ModeSystem) | FlagThumb | FlagCarry); err != nil {
		t.Fatal(err)
	}
	base := uint32(bus.IWRAMStart + 0x440)
	b.Write32(base, 0xabcdef01, bus.Access{})
	b.Write32(base+4, bus.ROM0Start+0x101, bus.Access{})
	c.WriteRegister(0, base)

	result, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, true, true, true, 0, (1<<1)|(1<<15)),
		b,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush {
		t.Fatal("LDM^ PC did not flush pipeline")
	}
	if c.CPSR().Mode() != ModeSystem || !c.CPSR().Thumb() || !c.CPSR().Carry() {
		t.Fatalf("restored CPSR = %08x", c.CPSR())
	}
	if got := c.PC(); got != bus.ROM0Start+0x100 {
		t.Fatalf("restored PC = %08x, want %08x", got, bus.ROM0Start+0x100)
	}
	if got := c.ReadRegister(1); got != 0xabcdef01 {
		t.Fatalf("LDM^ r1 = %08x, want abcdef01", got)
	}
	if got := c.ReadRegister(0); got != base+8 {
		t.Fatalf("LDM^ writeback = %08x, want %08x", got, base+8)
	}
}

func TestARMEmptyRegisterListTransfersPCAndWritesBack64Bytes(t *testing.T) {
	b := newExecutionBus(nil)
	c := New()
	if err := c.SetMode(ModeSystem); err != nil {
		t.Fatal(err)
	}
	base := uint32(bus.IWRAMStart + 0x480)
	b.Write32(base, bus.ROM0Start+0x200, bus.Access{})
	c.WriteRegister(0, base)

	result, err := c.ExecuteARMWithMemory(
		armBlockTransfer(false, true, false, true, true, 0, 0),
		b,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush || c.PC() != bus.ROM0Start+0x200 {
		t.Fatalf("empty LDM result=%+v PC=%08x", result, c.PC())
	}
	if got := c.ReadRegister(0); got != base+0x40 {
		t.Fatalf("empty LDM writeback = %08x, want %08x", got, base+0x40)
	}
}

func TestThumbPushPopRoundTripWithPC(t *testing.T) {
	b := newExecutionBus(nil)
	c := newThumbCPU(t)
	sp := uint32(bus.IWRAMStart + 0x600)
	c.WriteRegister(13, sp)
	c.WriteRegister(0, 0x11111111)
	c.WriteRegister(2, 0x22222222)
	c.WriteRegister(14, bus.ROM0Start+0x101)

	push, err := c.ExecuteThumbWithMemory(0xb505, b) // PUSH {r0,r2,lr}
	if err != nil {
		t.Fatal(err)
	}
	if push.MemoryCycles != 3 || c.ReadRegister(13) != sp-12 {
		t.Fatalf("PUSH result=%+v SP=%08x", push, c.ReadRegister(13))
	}
	for i, want := range []uint32{0x11111111, 0x22222222, bus.ROM0Start + 0x101} {
		got, _ := b.Read32(sp-12+uint32(i*4), bus.Access{})
		if got != want {
			t.Fatalf("PUSH word%d=%08x want=%08x", i, got, want)
		}
	}

	c.WriteRegister(0, 0)
	c.WriteRegister(2, 0)
	pop, err := c.ExecuteThumbWithMemory(0xbd05, b) // POP {r0,r2,pc}
	if err != nil {
		t.Fatal(err)
	}
	if !pop.PipelineFlush {
		t.Fatal("POP PC did not flush pipeline")
	}
	if c.ReadRegister(0) != 0x11111111 || c.ReadRegister(2) != 0x22222222 {
		t.Fatalf("POP regs r0=%08x r2=%08x", c.ReadRegister(0), c.ReadRegister(2))
	}
	if c.PC() != bus.ROM0Start+0x100 || c.ReadRegister(13) != sp {
		t.Fatalf("POP PC=%08x SP=%08x", c.PC(), c.ReadRegister(13))
	}
}

func TestThumbSTMIAAndLDMIABaseInListRules(t *testing.T) {
	t.Run("STM base later stores updated base", func(t *testing.T) {
		b := newExecutionBus(nil)
		c := newThumbCPU(t)
		base := uint32(bus.IWRAMStart + 0x680)
		c.WriteRegister(0, 0xaaaaaaaa)
		c.WriteRegister(3, base)

		if _, err := c.ExecuteThumbWithMemory(0xc309, b); err != nil { // STMIA r3!,{r0,r3}
			t.Fatal(err)
		}
		if got, _ := b.Read32(base+4, bus.Access{}); got != base+8 {
			t.Fatalf("stored base = %08x, want updated %08x", got, base+8)
		}
		if got := c.ReadRegister(3); got != base+8 {
			t.Fatalf("STMIA writeback = %08x, want %08x", got, base+8)
		}
	})

	t.Run("LDM base in list suppresses writeback", func(t *testing.T) {
		b := newExecutionBus(nil)
		c := newThumbCPU(t)
		base := uint32(bus.IWRAMStart + 0x6c0)
		b.Write32(base, 0x11111111, bus.Access{})
		b.Write32(base+4, 0x22222222, bus.Access{})
		c.WriteRegister(0, base)

		if _, err := c.ExecuteThumbWithMemory(0xc803, b); err != nil { // LDMIA r0!,{r0,r1}
			t.Fatal(err)
		}
		if got := c.ReadRegister(0); got != 0x11111111 {
			t.Fatalf("LDMIA base r0 = %08x, want loaded value", got)
		}
		if got := c.ReadRegister(1); got != 0x22222222 {
			t.Fatalf("LDMIA r1 = %08x, want 22222222", got)
		}
	})
}

func TestThumbEmptyMultipleListLoadsPCAndAdvancesBase64Bytes(t *testing.T) {
	b := newExecutionBus(nil)
	c := newThumbCPU(t)
	base := uint32(bus.IWRAMStart + 0x700)
	b.Write32(base, bus.ROM0Start+0x300, bus.Access{})
	c.WriteRegister(0, base)

	result, err := c.ExecuteThumbWithMemory(0xc800, b) // LDMIA r0!,{}
	if err != nil {
		t.Fatal(err)
	}
	if !result.PipelineFlush || c.PC() != bus.ROM0Start+0x300 {
		t.Fatalf("empty Thumb LDM result=%+v PC=%08x", result, c.PC())
	}
	if got := c.ReadRegister(0); got != base+0x40 {
		t.Fatalf("empty Thumb LDM writeback = %08x, want %08x", got, base+0x40)
	}
}
