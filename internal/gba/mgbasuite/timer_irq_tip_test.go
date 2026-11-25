package mgbasuite

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/system"
)

const (
	spIRQ    = 0x03007fa0
	spSystem = 0x03007f00
)

// TestTimerIRQFFFFTipWithSuiteBIOS reproduces the mGBA-suite timer-irq FFFF
// tip with the suite BIOS forwarder and a minimal user handler (disable TM0
// + ack IF).
//
// Hardware (libgba dispatcher) freezes at 0x51 from the 4-NOP sample onward.
// With this minimal handler the freeze value is lower (~0x1c) because the
// dispatcher is absent; the tip index is what matters here.
//
// GomeBoy preempts a halfword data access when the IRQ line rises during that
// access (or during the same instruction's fetch, before the data phase
// samples). That moves the tip to hardware's nops=4 without lowering
// IRQPropagationLatency (which would skew the libgba freeze away from 0x51).
func TestTimerIRQFFFFTipWithSuiteBIOS(t *testing.T) {
	results := make([]uint16, 10)

	for nops := 0; nops < 10; nops++ {
		m := system.New(suiteBIOS(), nil)
		if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
			t.Fatal(err)
		}
		m.CPU.WriteRegister(13, spSystem)
		if err := m.CPU.SetMode(cpu.ModeIRQ); err != nil {
			t.Fatal(err)
		}
		m.CPU.WriteRegister(13, spIRQ)
		if err := m.CPU.SetMode(cpu.ModeSystem); err != nil {
			t.Fatal(err)
		}

		handler := uint32(bus.IWRAMStart + 0x100)
		for i, instruction := range []uint32{
			0xe1a02000, // MOV  r2, r0
			0xe2800c01, // ADD  r0, r0, #0x100
			0xe3a01000, // MOV  r1, #0
			0xe5801000, // STR  r1, [r0]
			0xe2822c02, // ADD  r2, r2, #0x200
			0xe3a01008, // MOV  r1, #8
			0xe5c21002, // STRB r1, [r2, #2]
			0xe12fff1e, // BX   lr
		} {
			m.Bus.Write32(handler+uint32(i*4), instruction, bus.Access{})
		}
		m.Bus.Write32(0x03007ffc, handler, bus.Access{})

		m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
		m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})

		code := uint32(bus.IWRAMStart + 0x200)
		m.Bus.Write32(code+0, 0xe5813000, bus.Access{}) // STR  r3,[r1]
		m.Bus.Write32(code+4, 0xe1c120b0, bus.Access{}) // STRH r2,[r1]
		off := uint32(8)
		for i := 0; i < nops; i++ {
			m.Bus.Write32(code+off, 0xe1a00000, bus.Access{})
			off += 4
		}
		ldrh := code + off
		m.Bus.Write32(ldrh, 0xe1d100b0, bus.Access{}) // LDRH r0,[r1]
		done := ldrh + 4
		m.Bus.Write32(done, 0xe320f000, bus.Access{})   // NOP
		m.Bus.Write32(done+4, 0xeafffffe, bus.Access{}) // B .

		m.CPU.SetPC(code)
		m.CPU.WriteRegister(1, bus.IOStart+0x100)
		m.CPU.WriteRegister(2, 0x00c00000)
		m.CPU.WriteRegister(3, 0x00c0ffff)

		got := uint16(0xffff)
		for step := 0; step < 8192; step++ {
			if _, err := m.Step(); err != nil {
				t.Fatalf("nops=%d step %d: %v (pc=%#08x mode=%v)",
					nops, step, err, m.CPU.PC(), m.CPU.CPSR().Mode())
			}
			if m.CPU.CPSR().Mode() == cpu.ModeSystem && m.CPU.PC() == done+4 {
				got = uint16(m.CPU.ReadRegister(0))
				break
			}
		}
		results[nops] = got
	}

	t.Logf("FFFF tip results: %v", results)

	for nops := 0; nops < 4; nops++ {
		if results[nops] != uint16(nops) {
			t.Errorf("nops=%d got %#04x, want counting %#04x", nops, results[nops], nops)
		}
	}

	tip := -1
	var freeze uint16
	for nops := 0; nops < 10; nops++ {
		if results[nops] != uint16(nops) {
			tip = nops
			freeze = results[nops]
			break
		}
	}
	if tip < 0 {
		t.Fatal("timer IRQ never froze the counter")
	}
	for nops := tip; nops < 10; nops++ {
		if results[nops] != freeze {
			t.Errorf("nops=%d got %#04x, want stable freeze %#04x", nops, results[nops], freeze)
		}
	}

	if tip != 4 {
		t.Errorf("timer IRQ tip = nops=%d (freeze %#04x); want hardware tip at 4", tip, freeze)
	}
}
