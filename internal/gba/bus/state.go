package bus

// PrefetchState captures the Game Pak opcode-prefetch pipeline.
type PrefetchState struct {
	StartAddress       uint32
	NextFill           uint32
	Count              uint8
	Credit             uint32
	LastConsumeHit     bool
	LastConsumePartial bool
}

// State captures mutable GBA bus and memory state. BIOS/ROM bytes and attached
// cartridge devices are configuration and remain owned by the receiving bus.
type State struct {
	EWRAM   []byte
	IWRAM   []byte
	Palette []byte
	VRAM    []byte
	OAM     []byte
	IOData  [IOSize]byte

	WAITCNT         uint16
	Prefetch        PrefetchState
	OpenBus         uint32
	CPUOpenBus      uint32
	CPUOpenBusValid bool
	BIOSPrefetch    uint32
	CPUInBIOS       bool
	OBJVRAMStart    uint32
}

func copyStateBytes(dst []byte, src []byte) []byte {
	if cap(dst) < len(src) {
		dst = make([]byte, len(src))
	} else {
		dst = dst[:len(src)]
	}
	copy(dst, src)
	return dst
}

// Snapshot captures the bus state.
func (b *Bus) Snapshot() State {
	var s State
	b.SnapshotInto(&s)
	return s
}

// SnapshotInto captures the bus while reusing destination buffers.
func (b *Bus) SnapshotInto(s *State) {
	if s == nil {
		return
	}
	s.EWRAM = copyStateBytes(s.EWRAM, b.ewram)
	s.IWRAM = copyStateBytes(s.IWRAM, b.iwram)
	s.Palette = copyStateBytes(s.Palette, b.palette)
	s.VRAM = copyStateBytes(s.VRAM, b.vram)
	s.OAM = copyStateBytes(s.OAM, b.oam)
	s.IOData = b.io.data
	s.WAITCNT = b.wait.value
	s.Prefetch = PrefetchState{
		StartAddress: b.prefetch.startAddress, NextFill: b.prefetch.nextFill,
		Count: b.prefetch.count, Credit: b.prefetch.credit,
		LastConsumeHit: b.prefetch.lastConsumeHit,
		LastConsumePartial: b.prefetch.lastConsumePartial,
	}
	s.OpenBus = b.openBus
	s.CPUOpenBus = b.cpuOpenBus
	s.CPUOpenBusValid = b.cpuOpenBusValid
	s.BIOSPrefetch = b.biosPrefetch
	s.CPUInBIOS = b.cpuInBIOS
	s.OBJVRAMStart = b.objVRAMStart
}

// Restore rebuilds mutable bus state without replacing ROM/BIOS/device wiring.
func (b *Bus) Restore(s State) {
	copy(b.ewram, s.EWRAM)
	copy(b.iwram, s.IWRAM)
	copy(b.palette, s.Palette)
	copy(b.vram, s.VRAM)
	copy(b.oam, s.OAM)
	b.io.data = s.IOData
	b.wait.value = s.WAITCNT & 0x5fff
	b.prefetch = prefetchState{
		startAddress: s.Prefetch.StartAddress,
		nextFill: s.Prefetch.NextFill,
		count: s.Prefetch.Count,
		credit: s.Prefetch.Credit,
		lastConsumeHit: s.Prefetch.LastConsumeHit,
		lastConsumePartial: s.Prefetch.LastConsumePartial,
	}
	b.openBus = s.OpenBus
	b.cpuOpenBus = s.CPUOpenBus
	b.cpuOpenBusValid = s.CPUOpenBusValid
	b.biosPrefetch = s.BIOSPrefetch
	b.cpuInBIOS = s.CPUInBIOS
	b.objVRAMStart = s.OBJVRAMStart
}
