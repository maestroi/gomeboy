package memory

// Access describes the timing context of one GBA bus transaction.
//
// Sequential selects sequential timing for the first Game Pak halfword.
// Instruction marks an opcode fetch so WAITCNT prefetch may satisfy it.
// Locked marks the read/write pair of an atomic ARM SWP transaction.
// DMA marks a transfer driven by a DMA channel rather than the CPU.
// Misalignment preserves the original low address bits when the CPU aligns a
// transfer before presenting it to Memory. Byte-wide buses such as Game Pak
// SRAM still observe those bits even though ordinary halfword memory does not.
// The current bus timing model does not arbitrate these classes yet, but the
// distinction lets memory restrictions and future scheduling stay centralized.
type Access struct {
	Sequential  bool
	Instruction bool
	Locked      bool
	DMA          bool
	Misalignment uint8
}
