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
	case 0x04, 0x05, 0x06, 0x07:
		if width == 4 {
			return 2
		}
		return 1
	case 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d:
		if access.Instruction && b.PrefetchEnabled() {
			if cycles, ok := b.prefetch.consume(addr, width); ok {
				return cycles
			}
		}
		return b.gamePakROMCycles(addr, width, access.Sequential)
	case 0x0e, 0x0f:
		perByte := 1 + firstAccessWait[b.wait.value&0x3]
		switch width {
		case 1:
			return perByte
		case 2:
			return perByte * 2
		default:
			return perByte * 4
		}
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

func (b *Bus) afterAccess(addr, width uint32, access Access, mapped bool) {
	if !b.PrefetchEnabled() {
		return
	}
	if isROM(addr) {
		if access.Instruction && mapped {
			// A real fetch which missed the buffer restarts prefetch after the
			// fetched halfword(s).
			if !b.prefetch.lastConsumeHit {
				next := (addr &^ 1) + 2
				if width == 4 {
					next += 2
				}
				b.prefetch.reset(next)
			}
		} else {
			// Game Pak data accesses occupy the cartridge bus and invalidate
			// the simple sequential opcode stream.
			b.prefetch.reset(0)
		}
	}
	b.prefetch.lastConsumeHit = false
}

// Idle gives the Game Pak prefetcher CPU-internal idle cycles. A future CPU
// step loop should call this for internal cycles while executing from ROM.
func (b *Bus) Idle(cycles uint32) {
	if !b.PrefetchEnabled() || b.prefetch.nextFill == 0 || cycles == 0 {
		return
	}

	b.prefetch.credit += cycles
	for b.prefetch.count < 8 {
		addr := b.prefetch.nextFill
		_, seqWait := b.romWait(addr)
		cost := uint32(1 + seqWait)
		// Crossing a 128KB Game Pak boundary forces non-sequential timing.
		if addr&0x1ffff == 0 {
			nonSeqWait, _ := b.romWait(addr)
			cost = 1 + nonSeqWait
		}
		if b.prefetch.credit < cost {
			break
		}
		b.prefetch.credit -= cost
		b.prefetch.count++
		b.prefetch.nextFill += 2
	}
}
