package keypad

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

func newTestKeypad(t *testing.T) (*Keypad, *bus.Bus, *gbairq.Controller) {
	t.Helper()
	b := bus.New(nil, nil)
	irq := gbairq.New(b, nil)
	k := New(b, irq)
	return k, b, irq
}

func TestKeyInputIsActiveLowAndReadOnly(t *testing.T) {
	k, b, _ := newTestKeypad(t)

	if got, _ := b.Read16(bus.IOStart+keyInputOffset, bus.Access{}); got != keyMask {
		t.Fatalf("initial KEYINPUT = %04x, want %04x", got, keyMask)
	}

	k.Press(ButtonA)
	k.Press(ButtonL)
	want := keyMask &^ (1<<ButtonA | 1<<ButtonL)
	if got := k.Input(); got != want {
		t.Fatalf("KEYINPUT after A+L = %04x, want %04x", got, want)
	}
	if got, _ := b.Read16(bus.IOStart+keyInputOffset, bus.Access{}); got != want {
		t.Fatalf("mapped KEYINPUT = %04x, want %04x", got, want)
	}

	b.Write16(bus.IOStart+keyInputOffset, 0, bus.Access{})
	if got := k.Input(); got != want {
		t.Fatalf("KEYINPUT write changed state: %04x, want %04x", got, want)
	}

	k.Release(ButtonA)
	k.Release(ButtonL)
	if got := k.Input(); got != keyMask {
		t.Fatalf("released KEYINPUT = %04x, want %04x", got, keyMask)
	}
}

func TestKeyControlMasksUnusedBitsAndSupportsByteWrites(t *testing.T) {
	k, b, _ := newTestKeypad(t)

	b.Write16(bus.IOStart+keyControlOffset, 0xffff, bus.Access{})
	if got := k.Control(); got != controlMask {
		t.Fatalf("KEYCNT = %04x, want %04x", got, controlMask)
	}

	b.Write16(bus.IOStart+keyControlOffset, 0, bus.Access{})
	b.Write8(bus.IOStart+keyControlOffset, byte(1<<ButtonB), bus.Access{})
	if got := k.Control(); got != 1<<ButtonB {
		t.Fatalf("low-byte KEYCNT write = %04x, want %04x", got, uint16(1<<ButtonB))
	}

	b.Write8(bus.IOStart+keyControlOffset+1, byte((controlIRQEnable|controlAND)>>8), bus.Access{})
	want := uint16(1<<ButtonB) | controlIRQEnable | controlAND
	if got := k.Control(); got != want {
		t.Fatalf("high-byte KEYCNT write = %04x, want %04x", got, want)
	}
}

func TestKeypadORInterruptIsEdgeTriggered(t *testing.T) {
	k, b, irq := newTestKeypad(t)
	selected := uint16(1<<ButtonA | 1<<ButtonB)
	b.Write16(bus.IOStart+keyControlOffset, selected|controlIRQEnable, bus.Access{})

	k.Press(ButtonA)
	if got := irq.IF(); got != uint16(gbairq.Keypad) {
		t.Fatalf("IF after A press = %04x, want Keypad", got)
	}
	b.Write16(bus.IOStart+0x202, uint16(gbairq.Keypad), bus.Access{})

	// The OR condition stays true throughout these changes, so acknowledging IF
	// must not cause another request until the condition first falls false.
	k.Press(ButtonB)
	k.Release(ButtonA)
	if got := irq.IF(); got != 0 {
		t.Fatalf("held OR condition retriggered IRQ: IF=%04x", got)
	}

	k.Release(ButtonB)
	k.Press(ButtonB)
	if got := irq.IF(); got != uint16(gbairq.Keypad) {
		t.Fatalf("new OR edge IF = %04x, want Keypad", got)
	}
}

func TestKeypadANDInterruptRequiresAllSelectedButtons(t *testing.T) {
	k, b, irq := newTestKeypad(t)
	selected := uint16(1<<ButtonA | 1<<ButtonStart)
	b.Write16(bus.IOStart+keyControlOffset, selected|controlIRQEnable|controlAND, bus.Access{})

	k.Press(ButtonA)
	if got := irq.IF(); got != 0 {
		t.Fatalf("partial AND condition requested IRQ: %04x", got)
	}
	k.Press(ButtonStart)
	if got := irq.IF(); got != uint16(gbairq.Keypad) {
		t.Fatalf("complete AND condition IF = %04x, want Keypad", got)
	}

	b.Write16(bus.IOStart+0x202, uint16(gbairq.Keypad), bus.Access{})
	k.Release(ButtonA)
	k.Press(ButtonA)
	if got := irq.IF(); got != uint16(gbairq.Keypad) {
		t.Fatalf("second AND edge IF = %04x, want Keypad", got)
	}
}

func TestEnablingKeyIRQWhileConditionHeldCreatesEdge(t *testing.T) {
	k, b, irq := newTestKeypad(t)
	k.Press(ButtonR)
	if got := irq.IF(); got != 0 {
		t.Fatalf("disabled keypad requested IRQ: %04x", got)
	}

	b.Write16(bus.IOStart+keyControlOffset, uint16(1<<ButtonR)|controlIRQEnable, bus.Access{})
	if got := irq.IF(); got != uint16(gbairq.Keypad) {
		t.Fatalf("KEYCNT enable edge IF = %04x, want Keypad", got)
	}
}
