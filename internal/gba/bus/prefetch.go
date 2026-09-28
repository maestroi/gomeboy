package bus

type prefetchState struct {
	startAddress   uint32
	nextFill       uint32
	count          uint8
	credit         uint32
	lastConsumeHit     bool
	lastConsumePartial bool
}

func (p *prefetchState) reset(next uint32) {
	p.startAddress = next
	p.nextFill = next
	p.count = 0
	p.credit = 0
	p.lastConsumeHit = false
	p.lastConsumePartial = false
}

func (p *prefetchState) consume(addr, width uint32, partialWordTailCycles uint32) (uint32, bool) {
	halfwords := uint8(1)
	if width == 4 {
		halfwords = 2
	}
	aligned := addr &^ 1
	if aligned != p.startAddress || p.count == 0 {
		p.lastConsumeHit = false
		p.lastConsumePartial = false
		return 0, false
	}

	if p.count < halfwords {
		// ARM instruction fetches are 32-bit on a 16-bit Game Pak bus. If one
		// halfword is already queued, hardware uses it and only fetches the
		// missing sequential tail from the cartridge.
		p.startAddress += 2
		p.count--
		p.lastConsumeHit = true
		p.lastConsumePartial = true
		return 1 + partialWordTailCycles, true
	}

	p.startAddress += uint32(halfwords) * 2
	p.count -= halfwords
	p.lastConsumeHit = true
	p.lastConsumePartial = false
	// Prefetched halfwords have zero waitstates but still consume the CPU-side
	// access cycle itself.
	return uint32(halfwords), true
}


func (p *prefetchState) finishPending(addr, width uint32, wait func(uint32) (uint32, uint32)) (uint32, bool) {
	aligned := addr &^ 1
	if p.nextFill == 0 || p.count != 0 || aligned != p.startAddress || p.nextFill != aligned {
		return 0, false
	}

	_, seqWait := wait(aligned)
	halfCost := uint32(1 + seqWait)
	remaining := halfCost
	if p.credit < halfCost {
		remaining -= p.credit
	} else {
		remaining = 0
	}

	// The CPU takes ownership of the in-flight sequential cartridge access.
	// Any accumulated partial progress is consumed rather than discarded.
	cycles := remaining
	if width == 4 {
		cycles += halfCost
	}
	if cycles == 0 {
		cycles = 1
	}

	p.startAddress = aligned + width
	p.nextFill = p.startAddress
	p.count = 0
	p.credit = 0
	p.lastConsumeHit = false
	p.lastConsumePartial = true
	return cycles, true
}

func (p *prefetchState) advance(cycles uint32, wait func(uint32) (uint32, uint32)) {
	if p.nextFill == 0 || cycles == 0 {
		return
	}
	p.credit += cycles
	for p.count < 8 {
		addr := p.nextFill
		nonSeqWait, seqWait := wait(addr)
		cost := uint32(1 + seqWait)
		// Crossing a 128 KiB Game Pak boundary forces a non-sequential fill.
		if addr&0x1ffff == 0 {
			cost = 1 + nonSeqWait
		}
		if p.credit < cost {
			break
		}
		p.credit -= cost
		p.count++
		p.nextFill += 2
	}
}
