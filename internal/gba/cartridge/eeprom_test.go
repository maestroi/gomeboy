package cartridge

import "testing"

var _ interface {
	ReadBit() byte
	WriteBit(byte)
} = (*EEPROM)(nil)

func eepromWriteAddress(e *EEPROM, address uint32) {
	for bit := int(e.addressBits) - 1; bit >= 0; bit-- {
		e.WriteBit(byte(address >> uint(bit)))
	}
}

func eepromWriteBlock(e *EEPROM, address uint32, data uint64, stop byte) {
	e.WriteBit(1)
	e.WriteBit(0)
	eepromWriteAddress(e, address)
	for bit := 63; bit >= 0; bit-- {
		e.WriteBit(byte(data >> uint(bit)))
	}
	e.WriteBit(stop)
}

func eepromReadBlock(t *testing.T, e *EEPROM, address uint32) uint64 {
	t.Helper()
	e.WriteBit(1)
	e.WriteBit(1)
	eepromWriteAddress(e, address)
	e.WriteBit(0)

	for i := 0; i < 4; i++ {
		if got := e.ReadBit(); got != 0 {
			t.Fatalf("dummy read bit %d = %d, want 0", i, got)
		}
	}

	var data uint64
	for i := 0; i < 64; i++ {
		data = data<<1 | uint64(e.ReadBit())
	}
	return data
}

func TestEEPROMIdleAndBlankRead(t *testing.T) {
	for _, e := range []*EEPROM{NewEEPROM512B(), NewEEPROM8K()} {
		if got := e.ReadBit(); got != 1 {
			t.Fatalf("idle bit = %d, want 1", got)
		}
		if got := eepromReadBlock(t, e, 0); got != ^uint64(0) {
			t.Fatalf("blank block = %016x, want ffffffffffffffff", got)
		}
		if got := e.ReadBit(); got != 1 {
			t.Fatalf("post-read ready bit = %d, want 1", got)
		}
	}
}

func TestEEPROM512BWriteRead(t *testing.T) {
	e := NewEEPROM512B()
	const address = 37
	const want uint64 = 0x0123456789abcdef

	eepromWriteBlock(e, address, want, 0)
	if got := eepromReadBlock(t, e, address); got != want {
		t.Fatalf("read block = %016x, want %016x", got, want)
	}
	if got := eepromReadBlock(t, e, address+1); got != ^uint64(0) {
		t.Fatalf("neighbor block changed to %016x", got)
	}
}

func TestEEPROM8KFourteenBitAddressProtocol(t *testing.T) {
	e := NewEEPROM8K()
	const address = 0x3ff // last physical 8-byte block; transmitted as 14 bits
	const want uint64 = 0xfedcba9876543210

	eepromWriteBlock(e, address, want, 0)
	if got := eepromReadBlock(t, e, address); got != want {
		t.Fatalf("read block = %016x, want %016x", got, want)
	}
}

func TestEEPROMMalformedStopDoesNotCommitWrite(t *testing.T) {
	e := NewEEPROM512B()
	eepromWriteBlock(e, 5, 0x1122334455667788, 1)
	if got := eepromReadBlock(t, e, 5); got != ^uint64(0) {
		t.Fatalf("malformed write committed %016x", got)
	}
}

func TestEEPROMMalformedReadStopReturnsReady(t *testing.T) {
	e := NewEEPROM512B()
	e.WriteBit(1)
	e.WriteBit(1)
	eepromWriteAddress(e, 0)
	e.WriteBit(1)
	if got := e.ReadBit(); got != 1 {
		t.Fatalf("invalid read stop left line low: %d", got)
	}
}

func TestEEPROMOutOfRangeAddressDoesNotAlias(t *testing.T) {
	e := NewEEPROM8K()
	const invalid = 0x400
	eepromWriteBlock(e, invalid, 0x0011223344556677, 0)
	if got := eepromReadBlock(t, e, invalid); got != ^uint64(0) {
		t.Fatalf("out-of-range block = %016x, want erased", got)
	}
	if got := eepromReadBlock(t, e, 0); got != ^uint64(0) {
		t.Fatalf("out-of-range write aliased block 0: %016x", got)
	}
}
