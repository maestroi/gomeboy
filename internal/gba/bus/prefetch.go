package bus

type prefetchState struct {
	startAddress   uint32
	nextFill       uint32
	count          uint8
	credit         uint32
	lastConsumeHit bool
}

func (p *prefetchState) reset(next uint32) {
	p.startAddress = next
	p.nextFill = next
	p.count = 0
	p.credit = 0
	p.lastConsumeHit = false
}

func (p *prefetchState) consume(addr, width uint32) (uint32, bool) {
	halfwords := uint8(1)
	if width == 4 {
		halfwords = 2
	}
	aligned := addr &^ 1
	if aligned != p.startAddress || p.count < halfwords {
		p.lastConsumeHit = false
		return 0, false
	}

	p.startAddress += uint32(halfwords) * 2
	p.count -= halfwords
	p.lastConsumeHit = true
	// Prefetched halfwords have zero waitstates but still consume the access
	// cycle itself.
	return uint32(halfwords), true
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
