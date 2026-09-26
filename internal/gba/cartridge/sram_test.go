package cartridge

import "testing"

var _ interface {
	Read8(uint32) byte
	Write8(uint32, byte)
} = (*SRAM)(nil)

func TestSRAMStartsBlank(t *testing.T) {
	s := NewSRAM()
	for _, addr := range []uint32{0, 1, SRAMSize - 1, SRAMSize, 0x01000000} {
		if got := s.Read8(addr); got != 0xff {
			t.Fatalf("Read8(%08x) = %02x, want ff", addr, got)
		}
	}
}

func TestSRAMReadWrite(t *testing.T) {
	s := NewSRAM()
	s.Write8(0x1234, 0x5a)
	if got := s.Read8(0x1234); got != 0x5a {
		t.Fatalf("Read8(1234) = %02x, want 5a", got)
	}
	if got := s.Read8(0x1235); got != 0xff {
		t.Fatalf("neighbor changed to %02x, want ff", got)
	}
}

func TestSRAMMirrorsEvery32KiB(t *testing.T) {
	s := NewSRAM()
	const offset uint32 = 0x2345

	s.Write8(offset, 0x6b)
	for _, addr := range []uint32{
		offset + SRAMSize,
		offset + 2*SRAMSize,
		offset + 0x01000000,
	} {
		if got := s.Read8(addr); got != 0x6b {
			t.Fatalf("mirrored Read8(%08x) = %02x, want 6b", addr, got)
		}
	}

	s.Write8(offset+3*SRAMSize, 0x7c)
	if got := s.Read8(offset); got != 0x7c {
		t.Fatalf("mirrored write = %02x, want 7c", got)
	}
}
