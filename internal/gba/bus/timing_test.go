package bus

import "testing"

func TestDefaultMemoryTiming(t *testing.T) {
	b := testBus()
	cases := []struct {
		name   string
		addr   uint32
		width  uint32
		access Access
		want   uint32
	}{
		{"BIOS word", BIOSStart, 4, Access{}, 1},
		{"IWRAM word", IWRAMStart, 4, Access{}, 1},
		{"EWRAM half", EWRAMStart, 2, Access{}, 3},
		{"EWRAM word", EWRAMStart, 4, Access{}, 6},
		{"IO word", IOStart, 4, Access{}, 2},
		{"VRAM word", VRAMStart, 4, Access{}, 2},
		{"WS0 nonseq half", ROM0Start, 2, Access{}, 5},
		{"WS0 seq half", ROM0Start + 2, 2, Access{Sequential: true}, 3},
		{"WS0 nonseq word", ROM0Start, 4, Access{}, 8},
		{"WS1 nonseq half", ROM1Start, 2, Access{}, 5},
		{"WS1 seq half", ROM1Start + 2, 2, Access{Sequential: true}, 5},
		{"WS2 nonseq half", ROM2Start, 2, Access{}, 5},
		{"WS2 seq half", ROM2Start + 2, 2, Access{Sequential: true}, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := b.accessCycles(tc.addr, tc.width, tc.access); got != tc.want {
				t.Fatalf("cycles = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestWAITCNTConfiguresGamePakTiming(t *testing.T) {
	b := testBus()
	// SRAM=3 waits, WS0 first=2 waits, WS0 second=1 wait,
	// WS1 first=3 waits, WS1 second=1 wait,
	// WS2 first=8 waits, WS2 second=1 wait, prefetch enabled.
	const waitcnt uint16 =
		2 | // SRAM: 2 waits + access = 3
			(2 << 2) | (1 << 4) |
			(1 << 5) | (1 << 7) |
			(3 << 8) | (1 << 10) |
			(1 << 14)
	b.SetWAITCNT(waitcnt)

	if got := b.WAITCNT(); got != waitcnt {
		t.Fatalf("WAITCNT = %04x, want %04x", got, waitcnt)
	}
	if !b.PrefetchEnabled() {
		t.Fatal("prefetch bit not enabled")
	}

	cases := []struct {
		addr       uint32
		sequential bool
		want       uint32
	}{
		{ROM0Start, false, 3},
		{ROM0Start + 2, true, 2},
		{ROM1Start, false, 4},
		{ROM1Start + 2, true, 2},
		{ROM2Start, false, 9},
		{ROM2Start + 2, true, 2},
	}
	for _, tc := range cases {
		if got := b.gamePakROMCycles(tc.addr, 2, tc.sequential); got != tc.want {
			t.Errorf("ROM %08x seq=%v cycles=%d, want %d", tc.addr, tc.sequential, got, tc.want)
		}
	}

	if got := b.accessCycles(SaveStart, 1, Access{}); got != 3 {
		t.Fatalf("SRAM cycles = %d, want 3", got)
	}
}

func TestWAITCNTMappedThroughIO(t *testing.T) {
	b := testBus()
	const value = uint16(0x4317)
	b.Write16(IOStart+waitCNTOffset, value, Access{})
	if got := b.WAITCNT(); got != value {
		t.Fatalf("WAITCNT after I/O write = %04x, want %04x", got, value)
	}
	if got, _ := b.Read16(IOStart+waitCNTOffset, Access{}); got != value {
		t.Fatalf("WAITCNT I/O read = %04x, want %04x", got, value)
	}
}

func TestSequentialTimingResetsAt128KBoundary(t *testing.T) {
	b := testBus()
	// Default WS0 is N=5 cycles, S=3 cycles including access.
	if got := b.gamePakROMCycles(ROM0Start+0x1fffe, 2, true); got != 3 {
		t.Fatalf("before boundary = %d, want sequential 3", got)
	}
	if got := b.gamePakROMCycles(ROM0Start+0x20000, 2, true); got != 5 {
		t.Fatalf("at boundary = %d, want non-sequential 5", got)
	}
}

func TestPrefetchConsumesIdleCycles(t *testing.T) {
	b := testBus()
	b.SetWAITCNT(1 << 14)

	_, first := b.Read16(ROM0Start, Access{Instruction: true})
	if first != 5 {
		t.Fatalf("initial opcode fetch = %d cycles, want 5", first)
	}

	// Default WS0 sequential fetch costs 3 cycles per halfword.
	b.Idle(3)
	_, prefetched := b.Read16(ROM0Start+2, Access{Sequential: true, Instruction: true})
	if prefetched != 1 {
		t.Fatalf("prefetched halfword = %d cycles, want 1", prefetched)
	}

	// Fill two more halfwords and consume them as one word.
	b.Idle(6)
	_, wordCycles := b.Read32(ROM0Start+4, Access{Sequential: true, Instruction: true})
	if wordCycles != 2 {
		t.Fatalf("prefetched word = %d cycles, want 2", wordCycles)
	}
}

func TestGamePakDataAccessBreaksPrefetchStream(t *testing.T) {
	b := testBus()
	b.SetWAITCNT(1 << 14)
	b.Read16(ROM0Start, Access{Instruction: true})
	b.Idle(3)

	// A cartridge data read occupies the same bus and drops the queued stream.
	b.Read16(ROM0Start+0x100, Access{})
	_, cycles := b.Read16(ROM0Start+2, Access{Sequential: true, Instruction: true})
	if cycles != 3 {
		t.Fatalf("fetch after data access = %d cycles, want ordinary sequential 3", cycles)
	}
}
