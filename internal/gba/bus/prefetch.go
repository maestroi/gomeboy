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
