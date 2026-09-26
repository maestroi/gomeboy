// Package cartridge implements Game Boy Advance cartridge-side hardware.
package cartridge

const (
	// SRAMSize is the capacity of the standard GBA SRAM save device.
	SRAMSize = 32 * 1024
)

// SRAM is the standard 32 KiB byte-wide GBA save RAM device.
//
// The cartridge save bus exposes a much larger address window than the SRAM
// chip itself, so addresses mirror every 32 KiB.
type SRAM struct {
	data [SRAMSize]byte
}

// NewSRAM creates an empty SRAM device. Fresh save storage is filled with
// 0xff, matching the conventional blank GBA save-file state.
func NewSRAM() *SRAM {
	s := &SRAM{}
	for i := range s.data {
		s.data[i] = 0xff
	}
	return s
}

// Read8 reads one byte from SRAM.
func (s *SRAM) Read8(addr uint32) byte {
	return s.data[addr&(SRAMSize-1)]
}

// Write8 writes one byte to SRAM.
func (s *SRAM) Write8(addr uint32, value byte) {
	s.data[addr&(SRAMSize-1)] = value
}
