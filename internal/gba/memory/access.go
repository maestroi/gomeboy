package memory

// Access describes the timing context of one GBA bus transaction.
//
// Sequential selects sequential timing for the first Game Pak halfword.
// Instruction marks an opcode fetch so WAITCNT prefetch may satisfy it.
type Access struct {
	Sequential bool
	Instruction bool
}
