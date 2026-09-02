package gomeboy

import "fmt"

// MemoryRange describes a contiguous, non-wrapping address-space range.
type MemoryRange struct {
	Start  uint16
	Length int
}

// MemoryChange is one byte that changed between two tracker observations.
type MemoryChange struct {
	Address uint16
	Before  byte
	After   byte
	Frame   uint64
	Cycle   uint64
}

// MemoryTracker keeps a caller-selected memory range and reports byte changes
// since the previous ChangesSince call. It uses side-effect-free PeekInto and
// reuses its buffers across calls.
type MemoryTracker struct {
	range_  MemoryRange
	before  []byte
	current []byte
	frame   uint64
}

func validateMemoryRange(r MemoryRange) error {
	if r.Length <= 0 {
		return fmt.Errorf("gomeboy: memory range length must be > 0")
	}
	if int(r.Start)+r.Length > AddressSpaceSize {
		return fmt.Errorf("gomeboy: memory range %04X+%d crosses the address space", r.Start, r.Length)
	}
	return nil
}

// TrackMemory starts tracking range r from the emulator's current state.
func (e *Emulator) TrackMemory(r MemoryRange) (*MemoryTracker, error) {
	if err := validateMemoryRange(r); err != nil {
		return nil, err
	}
	t := &MemoryTracker{
		range_:  r,
		before:  make([]byte, r.Length),
		current: make([]byte, r.Length),
		frame:   e.FrameCount(),
	}
	e.PeekInto(r.Start, t.before)
	return t, nil
}

// Range returns the tracked memory range.
func (t *MemoryTracker) Range() MemoryRange {
	if t == nil {
		return MemoryRange{}
	}
	return t.range_
}

// ChangesSince reports bytes that changed since the previous call (or since
// TrackMemory for the first call) and advances the tracker's baseline to the
// current emulator state. The returned slice is newly allocated; the tracker
// itself reuses the memory snapshots.
func (e *Emulator) ChangesSince(t *MemoryTracker) []MemoryChange {
	if t == nil || len(t.before) == 0 {
		return nil
	}
	e.PeekInto(t.range_.Start, t.current)
	frame := e.FrameCount()
	cycle := e.Cycle()
	var out []MemoryChange
	for i, after := range t.current {
		before := t.before[i]
		if before == after {
			continue
		}
		out = append(out, MemoryChange{
			Address: t.range_.Start + uint16(i),
			Before:  before,
			After:   after,
			Frame:   frame,
			Cycle:   cycle,
		})
	}
	t.before, t.current = t.current, t.before
	t.frame = frame
	return out
}
