package bus

import "testing"

func TestIORegisterCallbacks(t *testing.T) {
	io := NewIO()
	var value uint16 = 0x1234
	var writes int
	io.Register16(0x40,
		func() uint16 { return value },
		func(v uint16) {
			value = v
			writes++
		},
	)

	if got, ok := io.Read16(0x40); !ok || got != 0x1234 {
		t.Fatalf("Read16 = %04x ok=%v", got, ok)
	}

	io.Write8(0x41, 0xab)
	if value != 0xab34 || writes != 1 {
		t.Fatalf("high-byte write: value=%04x writes=%d", value, writes)
	}
	io.Write8(0x40, 0xcd)
	if value != 0xabcd || writes != 2 {
		t.Fatalf("low-byte write: value=%04x writes=%d", value, writes)
	}

	io.Write16(0x40, 0xbeef)
	if value != 0xbeef || writes != 3 {
		t.Fatalf("halfword write: value=%04x writes=%d", value, writes)
	}
}

func TestIO32ComposesAdjacentHalfwordHandlers(t *testing.T) {
	io := NewIO()
	var lo, hi uint16
	io.Register16(0x80, func() uint16 { return lo }, func(v uint16) { lo = v })
	io.Register16(0x82, func() uint16 { return hi }, func(v uint16) { hi = v })

	io.Write32(0x80, 0x44332211)
	if lo != 0x2211 || hi != 0x4433 {
		t.Fatalf("Write32 lo=%04x hi=%04x", lo, hi)
	}
	got, ok := io.Read32(0x80)
	if !ok || got != 0x44332211 {
		t.Fatalf("Read32 = %08x ok=%v", got, ok)
	}
}
