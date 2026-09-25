package interrupt

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

type testLineSink struct {
	line    bool
	changes int
}

func (s *testLineSink) SetIRQLine(asserted bool) {
	if s.line != asserted {
		s.changes++
	}
	s.line = asserted
}

func newTestController(t *testing.T) (*Controller, *bus.Bus, *testLineSink) {
	t.Helper()
	b := bus.New(nil, nil)
	sink := &testLineSink{}
	c := New(b, sink)
	return c, b, sink
}

func TestInterruptRegistersMaskUnusedBits(t *testing.T) {
	c, b, _ := newTestController(t)

	b.Write16(bus.IOStart+ieOffset, 0xffff, bus.Access{})
	if got := c.IE(); got != validMask {
		t.Fatalf("IE = %04x, want %04x", got, validMask)
	}
	if got, _ := b.Read16(bus.IOStart+ieOffset, bus.Access{}); got != validMask {
		t.Fatalf("mapped IE = %04x, want %04x", got, validMask)
	}

	b.Write16(bus.IOStart+imeOffset, 0xffff, bus.Access{})
	if !c.IME() {
		t.Fatal("IME bit0 was not enabled")
	}
	if got, _ := b.Read16(bus.IOStart+imeOffset, bus.Access{}); got != 1 {
		t.Fatalf("mapped IME = %04x, want 0001", got)
	}
}

func TestRequestLatchesRegardlessOfIEAndIME(t *testing.T) {
	c, _, sink := newTestController(t)

	c.Request(VBlank)
	if got := c.IF(); got != uint16(VBlank) {
		t.Fatalf("IF = %04x, want VBlank", got)
	}
	if sink.line || c.IRQAsserted() {
		t.Fatal("disabled interrupt request asserted IRQ line")
	}
}

func TestIRQLineRequiresIMEAndEnabledPendingSource(t *testing.T) {
	c, b, sink := newTestController(t)

	c.Request(VBlank)
	b.Write16(bus.IOStart+ieOffset, uint16(VBlank), bus.Access{})
	if sink.line {
		t.Fatal("IE alone asserted IRQ while IME=0")
	}

	b.Write16(bus.IOStart+imeOffset, 1, bus.Access{})
	if !sink.line || !c.IRQAsserted() {
		t.Fatal("IME + enabled pending request did not assert IRQ")
	}

	b.Write16(bus.IOStart+ieOffset, 0, bus.Access{})
	if sink.line {
		t.Fatal("clearing IE did not deassert IRQ")
	}

	b.Write16(bus.IOStart+ieOffset, uint16(VBlank), bus.Access{})
	if !sink.line {
		t.Fatal("re-enabling pending source did not reassert IRQ")
	}

	b.Write16(bus.IOStart+imeOffset, 0, bus.Access{})
	if sink.line {
		t.Fatal("clearing IME did not deassert IRQ")
	}
}

func TestIFWriteOneToClearPreservesOtherRequests(t *testing.T) {
	c, b, sink := newTestController(t)
	b.Write16(bus.IOStart+ieOffset, uint16(VBlank|HBlank|DMA3), bus.Access{})
	b.Write16(bus.IOStart+imeOffset, 1, bus.Access{})
	c.Request(VBlank | HBlank | DMA3)

	b.Write16(bus.IOStart+ifOffset, uint16(HBlank), bus.Access{})
	want := uint16(VBlank | DMA3)
	if got := c.IF(); got != want {
		t.Fatalf("IF after HBlank ack = %04x, want %04x", got, want)
	}
	if !sink.line {
		t.Fatal("acknowledging one of several enabled requests deasserted IRQ")
	}

	b.Write16(bus.IOStart+ifOffset, want, bus.Access{})
	if got := c.IF(); got != 0 {
		t.Fatalf("IF after final ack = %04x, want 0000", got)
	}
	if sink.line {
		t.Fatal("acknowledging all pending requests left IRQ asserted")
	}
}

func TestIFByteWritesAcknowledgeOnlyWrittenByte(t *testing.T) {
	c, b, _ := newTestController(t)
	c.Request(VBlank | HBlank | DMA0 | DMA3)

	// Low-byte write clears only HBlank, leaving high-byte DMA flags intact.
	b.Write8(bus.IOStart+ifOffset, byte(HBlank), bus.Access{})
	want := uint16(VBlank | DMA0 | DMA3)
	if got := c.IF(); got != want {
		t.Fatalf("IF after low-byte ack = %04x, want %04x", got, want)
	}

	// High-byte write bit3 corresponds to IF bit11 (DMA3) and must not clear
	// the still-pending low-byte VBlank bit or DMA0.
	b.Write8(bus.IOStart+ifOffset+1, 1<<3, bus.Access{})
	want = uint16(VBlank | DMA0)
	if got := c.IF(); got != want {
		t.Fatalf("IF after high-byte ack = %04x, want %04x", got, want)
	}
}

func TestIFWriteZeroDoesNotClearRequests(t *testing.T) {
	c, b, _ := newTestController(t)
	c.Request(Timer0 | Keypad)

	b.Write16(bus.IOStart+ifOffset, 0, bus.Access{})
	if got := c.IF(); got != uint16(Timer0|Keypad) {
		t.Fatalf("IF after zero write = %04x", got)
	}
}

func TestIEAndIMEByteWritesUseNormalReadModifyWrite(t *testing.T) {
	c, b, _ := newTestController(t)

	b.Write8(bus.IOStart+ieOffset, byte(VBlank|HBlank), bus.Access{})
	b.Write8(bus.IOStart+ieOffset+1, byte(uint16(DMA0)>>8), bus.Access{})
	if got := c.IE(); got != uint16(VBlank|HBlank|DMA0) {
		t.Fatalf("byte-written IE = %04x", got)
	}

	b.Write8(bus.IOStart+imeOffset, 1, bus.Access{})
	b.Write8(bus.IOStart+imeOffset+1, 0xff, bus.Access{})
	if !c.IME() {
		t.Fatal("high-byte IME write disturbed low master-enable bit")
	}
}

func TestResetClearsControllerAndIRQLine(t *testing.T) {
	c, b, sink := newTestController(t)
	b.Write16(bus.IOStart+ieOffset, uint16(VBlank), bus.Access{})
	b.Write16(bus.IOStart+imeOffset, 1, bus.Access{})
	c.Request(VBlank)
	if !sink.line {
		t.Fatal("test setup did not assert line")
	}

	c.Reset()
	if c.IE() != 0 || c.IF() != 0 || c.IME() || c.IRQAsserted() || sink.line {
		t.Fatalf("reset state IE=%04x IF=%04x IME=%v IRQ=%v sink=%v",
			c.IE(), c.IF(), c.IME(), c.IRQAsserted(), sink.line)
	}
}
