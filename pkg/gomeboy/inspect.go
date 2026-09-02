package gomeboy

import "fmt"

// AddressSpaceSize is the size, in bytes, of the Game Boy's 16-bit address
// space. It is exported for callers that keep reusable inspection buffers.
const AddressSpaceSize = 1 << 16

// Peek8 reads a byte without any hardware side effects. Unlike Read8 it
// ignores PPU region locks, DMA conflicts and IO lazy readers, so it always
// returns the byte actually held in the mapped address space. Use Peek for
// observation; use Read for CPU-accurate reads.
func (e *Emulator) Peek8(addr uint16) byte {
	return e.gb.Bus.Get(addr)
}

// Peek16 reads a little-endian 16-bit value without side effects.
func (e *Emulator) Peek16(addr uint16) uint16 {
	return uint16(e.Peek8(addr)) | uint16(e.Peek8(addr+1))<<8
}

// PeekInto fills dst with len(dst) bytes starting at addr, without side
// effects and without allocating.
func (e *Emulator) PeekInto(addr uint16, dst []byte) {
	if len(dst) == 0 {
		return
	}

	end := int(addr) + len(dst)
	if end <= 0xffff {
		e.gb.Bus.CopyFrom(addr, uint16(end), dst)
		return
	}

	// uint16 cannot represent the exclusive end 0x10000. Keep the full
	// address-space snapshot on the bulk-copy path by copying through 0xFFFE
	// and reading the final byte directly.
	if end == AddressSpaceSize {
		last := len(dst) - 1
		if last > 0 {
			e.gb.Bus.CopyFrom(addr, 0xffff, dst[:last])
		}
		dst[last] = e.gb.Bus.Get(0xffff)
		return
	}

	// Preserve the historical uint16 wraparound semantics for ranges that
	// cross the end of the address space.
	for i := range dst {
		dst[i] = e.gb.Bus.Get(addr + uint16(i))
	}
}

// SnapshotMemory copies the emulator's complete 16-bit address space into
// dst and returns the frame number that snapshot belongs to. The operation is
// side-effect free and allocation-free when the caller reuses dst.
//
// Emulator is deliberately single-goroutine: callers must not step it while
// SnapshotMemory is running. Under that contract the returned frame and bytes
// describe one stable emulator state. Requiring the exact address-space size
// prevents accidental uint16 wraparound from looking like a partial snapshot.
func (e *Emulator) SnapshotMemory(dst []byte) (uint64, error) {
	if len(dst) != AddressSpaceSize {
		return 0, fmt.Errorf("gomeboy: SnapshotMemory: buffer is %d bytes, want %d", len(dst), AddressSpaceSize)
	}
	e.PeekInto(0, dst)
	return e.FrameCount(), nil
}
