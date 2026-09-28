package bus

const waitCNTOffset uint32 = 0x204

// WaitControl contains the implemented WAITCNT bits.
type WaitControl struct {
	value uint16
}

var firstAccessWait = [4]uint32{4, 3, 2, 8}
var ws0SecondWait = [2]uint32{2, 1}
var ws1SecondWait = [2]uint32{4, 1}
var ws2SecondWait = [2]uint32{8, 1}

// WAITCNT returns the current wait-state control register.
func (b *Bus) WAITCNT() uint16 { return b.wait.value }

// SetWAITCNT updates supported WAITCNT bits. Bit 13 is unused and bit 15 is
// the read-only Game Pak type flag (0 in GBA mode).
func (b *Bus) SetWAITCNT(value uint16) {
	oldPrefetch := b.wait.value&(1<<14) != 0
	b.wait.value = value & 0x5fff
	if oldPrefetch != b.PrefetchEnabled() {
		b.prefetch.reset(0)
	}
}

func (b *Bus) PrefetchEnabled() bool {
	return b.wait.value&(1<<14) != 0
}

func (b *Bus) installSystemRegisters() {
	b.io.Register16(waitCNTOffset,
		func() uint16 { return b.WAITCNT() },
		func(value uint16) { b.SetWAITCNT(value) },
	)

	// These addresses are physically unimplemented in the native GBA I/O map.
	// Writes disappear and reads expose the CPU pipeline/open-bus halfword.
	openBusHoles := []uint32{
		0x04e,
		0x056, 0x058, 0x05a, 0x05c, 0x05e,
		0x08c, 0x08e,
		0x0a8, 0x0aa, 0x0ac, 0x0ae,
	}
	for offset := uint32(0x0e0); offset <= 0x0fe; offset += 2 {
		openBusHoles = append(openBusHoles, offset)
	}
	for _, offset := range openBusHoles {
		offset := offset
		b.io.Register16(offset,
			func() uint16 {
				value := b.CPUOpenBus()
				if offset&2 != 0 {
					return uint16(value >> 16)
				}
				return uint16(value)
			},
			nil,
		)
	}
}

// AccessCycles returns the duration of a bus transaction without performing
// the transaction or mutating bus/open-bus state. DMA uses this for source
// regions that are physically inaccessible but still consume bus time.
func (b *Bus) AccessCycles(addr uint32, width uint32, access Access) uint32 {
	return b.accessCycles(addr, width, access)
}

func (b *Bus) accessCycles(addr uint32, width uint32, access Access) uint32 {
	switch addr >> 24 {
	case 0x00, 0x03:
		// BIOS/IWRAM are 32-bit and complete in one cycle.
		return 1
	case 0x02:
		// Default external WRAM timing: 2 waits + access, on a 16-bit bus.
		if width == 4 {
			return 6
		}
		return 3
	case 0x04, 0x05, 0x06:
		if width == 4 {
			return 2
		}
		return 1
	case 0x07:
		// OAM is a 32-bit bus. Supported 16/32-bit accesses complete in
		// one cycle.
		return 1
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d:
		if access.Instruction && b.PrefetchEnabled() {
			partialTail := uint32(0)
			if width == 4 {
				partialTail = b.gamePakROMCycles((addr&^3)+2, 2, true)
			}
			if cycles, ok := b.prefetch.consume(addr, width, partialTail); ok {
				return cycles
			}
		}
		return b.gamePakROMCycles(addr, width, access.Sequential)
	case 0x0e, 0x0f:
		// The save bus is physically 8-bit. Wider CPU loads/stores still issue
		// one save-bus access; the bus repeats/selects that byte in Bus methods.
		return 1 + firstAccessWait[b.wait.value&0x3]
	default:
		return 1
	}
}

func (b *Bus) gamePakROMCycles(addr uint32, width uint32, sequential bool) uint32 {
	n, s := b.romWait(addr)
	first := n
	if sequential && addr&0x1ffff != 0 {
		first = s
	}

	if width <= 2 {
		return 1 + first
	}

	// Game Pak ROM is 16-bit: a word is two halfword accesses, with the second
	// half always sequential.
	return (1 + first) + (1 + s)
}

func (b *Bus) romWait(addr uint32) (nonSequential, sequential uint32) {
	switch {
	case addr < ROM1Start:
		return firstAccessWait[(b.wait.value>>2)&0x3], ws0SecondWait[(b.wait.value>>4)&1]
	case addr < ROM2Start:
		return firstAccessWait[(b.wait.value>>5)&0x3], ws1SecondWait[(b.wait.value>>7)&1]
	default:
		return firstAccessWait[(b.wait.value>>8)&0x3], ws2SecondWait[(b.wait.value>>10)&1]
	}
}

func (b *Bus) afterAccess(addr, width uint32, access Access, mapped bool, cycles uint32) {
	if !b.PrefetchEnabled() {
		return
	}

	region := addr >> 24
	gamePakBus := region >= 0x08 && region <= 0x0f
	if isROM(addr) {
		if access.Instruction && mapped {
			if b.prefetch.lastConsumeHit && !b.prefetch.lastConsumePartial {
				// The CPU is reading entirely from the internal prefetch queue,
				// so the external cartridge bus is free to keep filling.
				if !access.DMA {
					b.prefetch.advance(cycles, b.romWait)
				}
			} else {
				// A miss, or the cartridge tail of a partially buffered ARM
				// word, occupies the external bus and restarts filling after
				// the complete CPU fetch.
				next := (addr &^ 1) + 2
				if width == 4 {
					next += 2
				}
				b.prefetch.reset(next)
			}
		} else {
			// Game Pak data accesses occupy the same external bus and break the
			// opcode-prefetch stream.
			b.prefetch.reset(0)
		}
	} else if !gamePakBus && !access.DMA {
		// Internal-memory/I/O/video accesses leave the Game Pak bus idle.
		// Prefetch therefore progresses in parallel with those CPU bus cycles.
		b.prefetch.advance(cycles, b.romWait)
	}
	b.prefetch.lastConsumeHit = false
}

// Idle gives the Game Pak prefetcher CPU-internal idle cycles.
func (b *Bus) Idle(cycles uint32) {
	if !b.PrefetchEnabled() {
		return
	}
	b.prefetch.advance(cycles, b.romWait)
}
