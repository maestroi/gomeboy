package system

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/keypad"
)

func TestKeypadIRQUsesCentralPropagationDelay(t *testing.T) {
	m := New(nil, nil)
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Keypad), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x132, uint16(1<<keypad.ButtonA)|(1<<14), bus.Access{})

	m.Keypad.Press(keypad.ButtonA)
	if got := m.IRQ.IF(); got != uint16(gbairq.Keypad) {
		t.Fatalf("keypad IF = %04x, want Keypad", got)
	}
	if m.CPU.IRQLine() {
		t.Fatal("keypad IRQ reached CPU without propagation delay")
	}

	m.Advance(uint32(IRQPropagationLatency - 1))
	if m.CPU.IRQLine() {
		t.Fatal("keypad IRQ reached CPU one cycle before propagation deadline")
	}
	m.Advance(1)
	if !m.CPU.IRQLine() {
		t.Fatal("keypad IRQ did not reach CPU at propagation deadline")
	}
}

func TestKeypadConditionWakesSTOPWithoutLatchingIF(t *testing.T) {
	m := New(nil, nil)
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Keypad), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x132, uint16(1<<keypad.ButtonStart)|(1<<14), bus.Access{})
	enterSTOP(t, m)

	m.Keypad.Press(keypad.ButtonStart)
	if got := m.IRQ.IF(); got != 0 {
		t.Fatalf("keypad STOP signal latched IF = %04x, want 0000", got)
	}

	wake, err := m.Step()
	if err != nil {
		t.Fatal(err)
	}
	if !wake.Woke || wake.Stopped || wake.Halted || wake.ElapsedCycles != 0 {
		t.Fatalf("keypad STOP wake = stopped:%v halted:%v woke:%v elapsed:%d",
			wake.Stopped, wake.Halted, wake.Woke, wake.ElapsedCycles)
	}
	if m.Cycle() != 0 {
		t.Fatalf("keypad STOP wake advanced clock to %d, want 0", m.Cycle())
	}
}
