package gomeboy

import "fmt"

// AddressSpaceSize returns the size in bytes of the active emulator core's
// address space. Unlike the legacy package-level AddressSpaceSize constant,
// this method is core-dependent and can represent address spaces wider than
// the Game Boy's 16 bits.
func (e *Emulator) AddressSpaceSize() uint64 {
	if e == nil || e.core == nil {
		return 0
	}
	return e.core.AddressSpaceSize()
}

// Read8At performs a CPU-accurate byte read using a 32-bit address. It is the
// core-neutral counterpart to Read8, whose uint16 parameter is retained for
// compatibility with existing GB/GBC callers.
func (e *Emulator) Read8At(addr uint32) (byte, error) {
	if e == nil || e.core == nil {
		return 0, fmt.Errorf("gomeboy: emulator core is not initialized")
	}
	return e.core.Read8At(addr)
}

// ReadAt performs CPU-accurate reads starting at a 32-bit address.
func (e *Emulator) ReadAt(addr uint32, length int) ([]byte, error) {
	if length < 0 {
		return nil, fmt.Errorf("gomeboy: negative memory length %d", length)
	}
	out := make([]byte, length)
	if err := e.ReadIntoAt(addr, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ReadIntoAt performs CPU-accurate reads into dst using a 32-bit address.
func (e *Emulator) ReadIntoAt(addr uint32, dst []byte) error {
	if e == nil || e.core == nil {
		return fmt.Errorf("gomeboy: emulator core is not initialized")
	}
	return e.core.ReadIntoAt(addr, dst)
}

// Peek8At reads a byte without hardware side effects using a 32-bit address.
func (e *Emulator) Peek8At(addr uint32) (byte, error) {
	if e == nil || e.core == nil {
		return 0, fmt.Errorf("gomeboy: emulator core is not initialized")
	}
	return e.core.Peek8At(addr)
}

// PeekIntoAt fills dst without hardware side effects using a 32-bit address.
func (e *Emulator) PeekIntoAt(addr uint32, dst []byte) error {
	if e == nil || e.core == nil {
		return fmt.Errorf("gomeboy: emulator core is not initialized")
	}
	return e.core.PeekIntoAt(addr, dst)
}
