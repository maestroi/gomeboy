package gomeboy

// ReadInto performs CPU-accurate reads into dst without allocating a result
// slice. Reads wrap through the 16-bit address space exactly as Read does.
func (e *Emulator) ReadInto(addr uint16, dst []byte) {
	for i := range dst {
		dst[i] = e.gb.Bus.Read(addr + uint16(i))
	}
}
