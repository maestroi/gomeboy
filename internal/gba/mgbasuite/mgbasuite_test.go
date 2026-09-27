package mgbasuite

import (
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/system"
)

func TestParseResults(t *testing.T) {
	log := "Game Boy Advance Test Suite\n===\nA: PASS\nB: Got 1 vs 2: FAIL\nC: PASS\n"
	passed, failed, failures := parseResults(log)
	if passed != 2 || failed != 1 {
		t.Fatalf("counts = %d/%d, want 2 pass 1 fail", passed, failed)
	}
	if len(failures) != 1 || !strings.Contains(failures[0], "B:") {
		t.Fatalf("failures = %v", failures)
	}
}

func TestClassifyBaselineTransitions(t *testing.T) {
	category := Category{
		ID: "io", Name: "I/O", Enabled: true, ExpectedTotal: 10,
		Baseline: &Baseline{Passed: 7, Failed: 3, Total: 10},
	}

	expected := CategoryResult{Passed: 7, Failed: 3, Total: 10}
	classify(&expected, category)
	if expected.Status != StatusExpected {
		t.Fatalf("equal baseline status = %s", expected.Status)
	}

	regression := CategoryResult{Passed: 6, Failed: 4, Total: 10}
	classify(&regression, category)
	if regression.Status != StatusRegression {
		t.Fatalf("regression status = %s", regression.Status)
	}

	xpass := CategoryResult{Passed: 8, Failed: 2, Total: 10}
	classify(&xpass, category)
	if xpass.Status != StatusXPASS {
		t.Fatalf("xpass status = %s", xpass.Status)
	}

	unbaselined := CategoryResult{Passed: 7, Failed: 3, Total: 10}
	noBaseline := category
	noBaseline.Baseline = nil
	classify(&unbaselined, noBaseline)
	if unbaselined.Status != StatusUnbaselined {
		t.Fatalf("unbaselined status = %s", unbaselined.Status)
	}
}

func TestHLECPUSetCopiesAndFillsThroughGBABus(t *testing.T) {
	m := system.NewWithCartridgeConfig(nil, nil, cartridge.Config{SaveType: cartridge.SaveSRAM})
	src := bus.EWRAMStart + 0x100
	dst := bus.IWRAMStart + 0x100
	m.Bus.Write16(src, 0x1122, bus.Access{})
	m.Bus.Write16(src+2, 0x3344, bus.Access{})

	m.CPU.WriteRegister(0, src)
	m.CPU.WriteRegister(1, dst)
	m.CPU.WriteRegister(2, 2)
	hleCPUSet(m, false)
	if got, _ := m.Bus.Read16(dst, bus.Access{}); got != 0x1122 {
		t.Fatalf("CpuSet first halfword = %04x", got)
	}
	if got, _ := m.Bus.Read16(dst+2, bus.Access{}); got != 0x3344 {
		t.Fatalf("CpuSet second halfword = %04x", got)
	}
	if got := m.CPU.ReadRegister(3); got != 0x170 {
		t.Fatalf("CpuSet r3 = %08x, want 00000170", got)
	}

	m.Bus.Write32(src, 0xaabbccdd, bus.Access{})
	m.CPU.WriteRegister(0, src)
	m.CPU.WriteRegister(1, dst)
	m.CPU.WriteRegister(2, (1<<24)|(1<<26)|2)
	hleCPUSet(m, false)
	for offset := uint32(0); offset < 8; offset += 4 {
		if got, _ := m.Bus.Read32(dst+offset, bus.Access{}); got != 0xaabbccdd {
			t.Fatalf("CpuSet fill +%d = %08x", offset, got)
		}
	}
}

func TestMenuDriverPulsesDownThenStartsCategory(t *testing.T) {
	m := system.New(nil, nil)
	d := newMenuDriver(m, 2)
	if m.Keypad.PressedMask() == 0 {
		t.Fatal("initial Down pulse missing")
	}
	d.vblankWait()
	if m.Keypad.PressedMask() != 0 {
		t.Fatalf("first release gap mask = %04x", m.Keypad.PressedMask())
	}
	d.vblankWait()
	if m.Keypad.PressedMask() == 0 {
		t.Fatal("second Down pulse missing")
	}
	d.vblankWait()
	wantA := uint16(1) << 0
	if got := m.Keypad.PressedMask(); got != wantA {
		t.Fatalf("start mask = %04x, want A", got)
	}
}


func TestHandleSuiteIRQAcknowledgesVBlankAndReturns(t *testing.T) {
	m := system.New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	m.CPU.SetPC(bus.ROM0Start + 0x100)
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.VBlank), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.IRQ.Request(gbairq.VBlank)

	if err := m.CPU.EnterException(cpu.ExceptionIRQ, bus.ROM0Start+0x104); err != nil {
		t.Fatal(err)
	}
	if err := handleSuiteIRQ(m); err != nil {
		t.Fatal(err)
	}
	if got := m.CPU.CPSR().Mode(); got != cpu.ModeSystem {
		t.Fatalf("mode after IRQ return = %v, want system", got)
	}
	if got := m.CPU.PC(); got != bus.ROM0Start+0x100 {
		t.Fatalf("PC after IRQ return = %#08x, want %#08x", got, bus.ROM0Start+0x100)
	}
	if got := m.IRQ.IF(); got&uint16(gbairq.VBlank) != 0 {
		t.Fatalf("VBlank IF remained set: %#04x", got)
	}
}

func TestHandleSuiteIRQRejectsNonVBlank(t *testing.T) {
	m := system.New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	m.IRQ.Request(gbairq.Timer0)
	if err := m.CPU.EnterException(cpu.ExceptionIRQ, bus.ROM0Start+4); err != nil {
		t.Fatal(err)
	}
	if err := handleSuiteIRQ(m); err == nil || !strings.Contains(err.Error(), "unsupported IRQ") {
		t.Fatalf("non-VBlank IRQ error = %v", err)
	}
}
