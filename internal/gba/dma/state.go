package dma

// ChannelState captures one DMA channel, including an in-flight transfer.
type ChannelState struct {
	SourceInitial uint32
	DestInitial   uint32
	CountInitial  uint16
	Control       uint16

	SourceCurrent uint32
	DestCurrent   uint32
	CountCurrent  uint32
	DestReload    uint32
	CountReload   uint32

	LastCycles uint32
	LastUnits  uint32
	DataLatch  uint32

	DisableAfterRun bool

	TransferActive      bool
	CompletionPending   bool
	TransferRemaining   uint32
	TransferUnits       uint32
	TransferWidth       uint32
	TransferSourceMode  uint16
	TransferDestMode    uint16
	TransferStartSource uint32
	TransferStartDest   uint32
	TransferCycles      uint32
}

// State captures the DMA controller and arbitration state.
type State struct {
	Channels  [4]ChannelState
	Pending   uint8
	Active    uint8
	Servicing bool
}

func channelSnapshot(c channel) ChannelState {
	return ChannelState{
		SourceInitial: c.sourceInitial, DestInitial: c.destInitial,
		CountInitial: c.countInitial, Control: c.control,
		SourceCurrent: c.sourceCurrent, DestCurrent: c.destCurrent,
		CountCurrent: c.countCurrent, DestReload: c.destReload,
		CountReload: c.countReload, LastCycles: c.lastCycles,
		LastUnits: c.lastUnits, DataLatch: c.dataLatch,
		DisableAfterRun: c.disableAfterRun,
		TransferActive: c.transferActive, CompletionPending: c.completionPending,
		TransferRemaining: c.transferRemaining, TransferUnits: c.transferUnits,
		TransferWidth: c.transferWidth, TransferSourceMode: c.transferSourceMode,
		TransferDestMode: c.transferDestMode, TransferStartSource: c.transferStartSource,
		TransferStartDest: c.transferStartDest, TransferCycles: c.transferCycles,
	}
}

func restoreChannel(s ChannelState) channel {
	return channel{
		sourceInitial: s.SourceInitial, destInitial: s.DestInitial,
		countInitial: s.CountInitial, control: s.Control,
		sourceCurrent: s.SourceCurrent, destCurrent: s.DestCurrent,
		countCurrent: s.CountCurrent, destReload: s.DestReload,
		countReload: s.CountReload, lastCycles: s.LastCycles,
		lastUnits: s.LastUnits, dataLatch: s.DataLatch,
		disableAfterRun: s.DisableAfterRun,
		transferActive: s.TransferActive, completionPending: s.CompletionPending,
		transferRemaining: s.TransferRemaining, transferUnits: s.TransferUnits,
		transferWidth: s.TransferWidth, transferSourceMode: s.TransferSourceMode,
		transferDestMode: s.TransferDestMode, transferStartSource: s.TransferStartSource,
		transferStartDest: s.TransferStartDest, transferCycles: s.TransferCycles,
	}
}

func (d *DMA) Snapshot() State {
	var s State
	for i := range d.ch {
		s.Channels[i] = channelSnapshot(d.ch[i])
	}
	s.Pending = d.pending
	s.Active = d.active
	s.Servicing = d.servicing
	return s
}

func (d *DMA) Restore(s State) {
	for i := range d.ch {
		d.ch[i] = restoreChannel(s.Channels[i])
	}
	d.pending = s.Pending & 0x0f
	d.active = s.Active & 0x0f
	d.servicing = s.Servicing
}
