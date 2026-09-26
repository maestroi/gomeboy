package cartridge

import "testing"

var _ interface {
	Read8(uint32) byte
	Write8(uint32, byte)
} = (*Flash)(nil)

func flashUnlockCommand(f *Flash, command byte) {
	f.Write8(0x5555, 0xaa)
	f.Write8(0x2aaa, 0x55)
	f.Write8(0x5555, command)
}

func flashProgram(f *Flash, addr uint32, value byte) {
	flashUnlockCommand(f, 0xa0)
	f.Write8(addr, value)
}

func flashEraseSector(f *Flash, addr uint32) {
	flashUnlockCommand(f, 0x80)
	f.Write8(0x5555, 0xaa)
	f.Write8(0x2aaa, 0x55)
	f.Write8(addr, 0x30)
}

func flashEraseChip(f *Flash) {
	flashUnlockCommand(f, 0x80)
	f.Write8(0x5555, 0xaa)
	f.Write8(0x2aaa, 0x55)
	f.Write8(0x5555, 0x10)
}

func flashSelectBank(f *Flash, bank byte) {
	flashUnlockCommand(f, 0xb0)
	f.Write8(0, bank)
}

func TestFlashStartsBlankAndMirrors64KWindow(t *testing.T) {
	f := NewFlash64K()

	for _, addr := range []uint32{0, 1, 0xffff, 0x10000, 0x01001234} {
		if got := f.Read8(addr); got != 0xff {
			t.Fatalf("Read8(%08x) = %02x, want ff", addr, got)
		}
	}

	flashProgram(f, 0x1234, 0x5a)
	if got := f.Read8(0x10000 + 0x1234); got != 0x5a {
		t.Fatalf("64 KiB mirror = %02x, want 5a", got)
	}
	if got := f.Read8(0x01000000 + 0x1234); got != 0x5a {
		t.Fatalf("0x0f save-window mirror = %02x, want 5a", got)
	}
}

func TestFlashProgramCommand(t *testing.T) {
	f := NewFlash64K()

	flashProgram(f, 0x1234, 0x5a)
	if got := f.Read8(0x1234); got != 0x5a {
		t.Fatalf("programmed byte = %02x, want 5a", got)
	}
	if got := f.Read8(0x1235); got != 0xff {
		t.Fatalf("neighbor changed to %02x, want ff", got)
	}

	// A0 applies only to the next byte write.
	f.Write8(0x1235, 0x6b)
	if got := f.Read8(0x1235); got != 0xff {
		t.Fatalf("raw write after program command changed flash to %02x", got)
	}
}

func TestFlashIDMode(t *testing.T) {
	tests := []struct {
		name         string
		f            *Flash
		manufacturer byte
		device       byte
	}{
		{"64K", NewFlash64K(), 0x32, 0x1b},
		{"128K", NewFlash128K(), 0x62, 0x13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flashProgram(tt.f, 0x1234, 0x5a)
			flashUnlockCommand(tt.f, 0x90)

			if got := tt.f.Read8(0); got != tt.manufacturer {
				t.Fatalf("manufacturer ID = %02x, want %02x", got, tt.manufacturer)
			}
			if got := tt.f.Read8(1); got != tt.device {
				t.Fatalf("device ID = %02x, want %02x", got, tt.device)
			}

			flashUnlockCommand(tt.f, 0xf0)
			if got := tt.f.Read8(0x1234); got != 0x5a {
				t.Fatalf("read-array after ID exit = %02x, want 5a", got)
			}
		})
	}
}

func TestFlash128KBanksAreIndependent(t *testing.T) {
	f := NewFlash128K()

	flashProgram(f, 0x2222, 0x11)
	flashSelectBank(f, 1)
	if got := f.Read8(0x2222); got != 0xff {
		t.Fatalf("bank 1 initially = %02x, want ff", got)
	}

	flashProgram(f, 0x2222, 0x22)
	flashSelectBank(f, 0)
	if got := f.Read8(0x2222); got != 0x11 {
		t.Fatalf("bank 0 = %02x, want 11", got)
	}

	flashSelectBank(f, 1)
	if got := f.Read8(0x2222); got != 0x22 {
		t.Fatalf("bank 1 = %02x, want 22", got)
	}
}

func TestFlash64KIgnoresBankSelect(t *testing.T) {
	f := NewFlash64K()
	flashProgram(f, 0x100, 0x44)

	flashUnlockCommand(f, 0xb0)
	f.Write8(0, 1)

	if got := f.Read8(0x100); got != 0x44 {
		t.Fatalf("64 KiB bank command changed visible data to %02x", got)
	}
}

func TestFlashSectorEraseAffectsOnlySelected4KiBSector(t *testing.T) {
	f := NewFlash128K()

	flashProgram(f, 0x1234, 0x11)
	flashProgram(f, 0x2345, 0x22)
	flashEraseSector(f, 0x1234)

	if got := f.Read8(0x1234); got != 0xff {
		t.Fatalf("erased sector byte = %02x, want ff", got)
	}
	if got := f.Read8(0x2345); got != 0x22 {
		t.Fatalf("neighbor sector changed to %02x, want 22", got)
	}

	flashSelectBank(f, 1)
	flashProgram(f, 0x1234, 0x33)
	flashEraseSector(f, 0x1234)
	if got := f.Read8(0x1234); got != 0xff {
		t.Fatalf("bank 1 erased sector byte = %02x, want ff", got)
	}

	flashSelectBank(f, 0)
	if got := f.Read8(0x2345); got != 0x22 {
		t.Fatalf("bank 0 neighbor changed after bank 1 erase to %02x", got)
	}
}

func TestFlashChipEraseClearsAllBanks(t *testing.T) {
	f := NewFlash128K()

	flashProgram(f, 0x1111, 0x11)
	flashSelectBank(f, 1)
	flashProgram(f, 0x2222, 0x22)

	flashEraseChip(f)

	flashSelectBank(f, 0)
	if got := f.Read8(0x1111); got != 0xff {
		t.Fatalf("bank 0 after chip erase = %02x, want ff", got)
	}
	flashSelectBank(f, 1)
	if got := f.Read8(0x2222); got != 0xff {
		t.Fatalf("bank 1 after chip erase = %02x, want ff", got)
	}
}

func TestFlashMalformedUnlockDoesNotMutateData(t *testing.T) {
	f := NewFlash64K()

	f.Write8(0x5555, 0xaa)
	f.Write8(0x2aaa, 0x54) // wrong second unlock value
	f.Write8(0x5555, 0xa0)
	f.Write8(0x1234, 0x5a)

	if got := f.Read8(0x1234); got != 0xff {
		t.Fatalf("malformed unlock programmed %02x, want ff", got)
	}

	f.Write8(0x5554, 0xaa) // wrong first unlock address
	f.Write8(0x2aaa, 0x55)
	f.Write8(0x5555, 0xa0)
	f.Write8(0x1234, 0x6b)

	if got := f.Read8(0x1234); got != 0xff {
		t.Fatalf("wrong unlock address programmed %02x, want ff", got)
	}
}
