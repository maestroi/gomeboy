package memory

// Access describes the timing context of one GBA bus transaction.
//
// Sequential selects sequential timing for the first Game Pak halfword.
// Instruction marks an opcode fetch so WAITCNT prefetch may satisfy it.
// Locked marks the read/write pair of an atomic ARM SWP transaction. The
// current bus timing model does not change access duration for Locked traffic,
// but exposing it here lets future DMA/arbitration code keep the pair indivisible.
type Access struct {
	Sequential  bool
	Instruction bool
	Locked      bool
}
