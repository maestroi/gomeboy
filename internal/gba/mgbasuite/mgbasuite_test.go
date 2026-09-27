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

	// Halfword copy keeps an odd source address. ARM7TDMI LDRH rotates the
	// zero-extended aligned halfword by 8 before STRH stores the low half.
	m.Bus.Write32(src, 0xdeadbeef, bus.Access{})
	m.CPU.WriteRegister(0, src+1)
	m.CPU.WriteRegister(1, dst)
	m.CPU.WriteRegister(2, 2)
	hleCPUSet(m, false)
	if got, _ := m.Bus.Read32(dst, bus.Access{}); got != 0x00de00be {
		t.Fatalf("unaligned CpuSet halfword copy = %08x, want 00de00be", got)
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


func TestSuiteIRQWrapperRunsUserVectorAndRestoresState(t *testing.T) {
	m := system.New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	returnPC := uint32(bus.ROM0Start + 0x100)
	handler := uint32(bus.ROM0Start + 0x200)
	m.CPU.SetPC(returnPC)
	m.CPU.WriteRegister(0, 0x10)
	m.CPU.WriteRegister(1, 0x11)
	m.CPU.WriteRegister(2, 0x12)
	m.CPU.WriteRegister(3, 0x13)
	m.CPU.WriteRegister(12, 0x1c)
	m.Bus.Write32(0x03007ffc, handler, bus.Access{})

	if err := m.CPU.EnterException(cpu.ExceptionIRQ, returnPC+4); err != nil {
		t.Fatal(err)
	}
	state := suiteIRQState{}
	if err := beginSuiteIRQ(m, &state); err != nil {
		t.Fatal(err)
	}
	if !state.active {
		t.Fatal("IRQ wrapper state not active")
	}
	if got := m.CPU.PC(); got != handler {
		t.Fatalf("user IRQ vector PC = %#08x, want %#08x", got, handler)
	}
	if got := m.CPU.ReadRegister(0); got != bus.IOStart {
		t.Fatalf("BIOS IRQ r0 = %#08x, want %#08x", got, uint32(bus.IOStart))
	}
	if got := m.CPU.ReadRegister(14); got != biosIRQReturnPC {
		t.Fatalf("BIOS IRQ LR = %#08x, want %#08x", got, biosIRQReturnPC)
	}

	m.CPU.WriteRegister(0, 0)
	m.CPU.WriteRegister(1, 0)
	m.CPU.WriteRegister(2, 0)
	m.CPU.WriteRegister(3, 0)
	m.CPU.WriteRegister(12, 0)
	m.CPU.SetPC(biosIRQReturnPC)
	if err := finishSuiteIRQ(m, &state); err != nil {
		t.Fatal(err)
	}
	if state.active {
		t.Fatal("IRQ wrapper state remained active")
	}
	if got := m.CPU.CPSR().Mode(); got != cpu.ModeSystem {
		t.Fatalf("mode after IRQ return = %v, want system", got)
	}
	if got := m.CPU.PC(); got != returnPC {
		t.Fatalf("PC after IRQ return = %#08x, want %#08x", got, returnPC)
	}
	for reg, want := range map[int]uint32{0: 0x10, 1: 0x11, 2: 0x12, 3: 0x13, 12: 0x1c} {
		if got := m.CPU.ReadRegister(reg); got != want {
			t.Fatalf("r%d after IRQ return = %#08x, want %#08x", reg, got, want)
		}
	}
}

func TestSuiteIRQWrapperRequiresUserVector(t *testing.T) {
	m := system.New(nil, nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		t.Fatal(err)
	}
	if err := m.CPU.EnterException(cpu.ExceptionIRQ, bus.ROM0Start+4); err != nil {
		t.Fatal(err)
	}
	state := suiteIRQState{}
	if err := beginSuiteIRQ(m, &state); err == nil || !strings.Contains(err.Error(), "no user vector") {
		t.Fatalf("missing vector error = %v", err)
	}
}


func TestReadSuiteCountFromTextVRAM(t *testing.T) {
	b := bus.New(nil, nil)
	const displayed = "1290/1552"
	const offset = (1*32 + 21) * 2
	for i := 0; i < len(displayed); i++ {
		b.VRAM()[offset+i*2] = displayed[i] - ' '
	}
	passed, total, ok := readSuiteCount(b)
	if !ok || passed != 1290 || total != 1552 {
		t.Fatalf("suite count = %d/%d ok=%v, want 1290/1552 true", passed, total, ok)
	}
}


func TestAdvanceToNextVBlank(t *testing.T) {
	m := system.New(nil, nil)
	if m.PPU.InVBlank() {
		t.Fatal("new PPU unexpectedly starts in VBlank")
	}
	advanceToNextVBlank(m)
	if !m.PPU.InVBlank() || m.PPU.VCount() != 160 || m.PPU.LineCycle() != 0 {
		t.Fatalf("first wait ended at vcount=%d lineCycle=%d vblank=%v", m.PPU.VCount(), m.PPU.LineCycle(), m.PPU.InVBlank())
	}
	first := m.Cycle()
	advanceToNextVBlank(m)
	if !m.PPU.InVBlank() || m.PPU.VCount() != 160 || m.PPU.LineCycle() != 0 {
		t.Fatalf("second wait ended at vcount=%d lineCycle=%d vblank=%v", m.PPU.VCount(), m.PPU.LineCycle(), m.PPU.InVBlank())
	}
	wantDelta := uint64(228 * 1232)
	if got := m.Cycle() - first; got != wantDelta {
		t.Fatalf("second VBlank wait advanced %d cycles, want %d", got, wantDelta)
	}
}

func TestHLEDivAndDivArm(t *testing.T) {
	m := system.New(nil, nil)

	m.CPU.WriteRegister(0, 0xfffffff3)
	m.CPU.WriteRegister(1, 5)
	if err := hleDiv(m, false); err != nil {
		t.Fatal(err)
	}
	if got := int32(m.CPU.ReadRegister(0)); got != -2 {
		t.Fatalf("Div quotient = %d, want -2", got)
	}
	if got := int32(m.CPU.ReadRegister(1)); got != -3 {
		t.Fatalf("Div remainder = %d, want -3", got)
	}
	if got := m.CPU.ReadRegister(3); got != 2 {
		t.Fatalf("Div abs quotient = %d, want 2", got)
	}

	m.CPU.WriteRegister(0, 5)
	m.CPU.WriteRegister(1, 13)
	if err := hleDiv(m, true); err != nil {
		t.Fatal(err)
	}
	if got := m.CPU.ReadRegister(0); got != 2 {
		t.Fatalf("DivArm quotient = %d, want 2", got)
	}
	if got := m.CPU.ReadRegister(1); got != 3 {
		t.Fatalf("DivArm remainder = %d, want 3", got)
	}
}

func TestHLESqrt(t *testing.T) {
	m := system.New(nil, nil)
	m.CPU.WriteRegister(0, 81)
	hleSqrt(m)
	if got := m.CPU.ReadRegister(0); got != 9 {
		t.Fatalf("Sqrt(81) = %d, want 9", got)
	}
	m.CPU.WriteRegister(0, 80)
	hleSqrt(m)
	if got := m.CPU.ReadRegister(0); got != 8 {
		t.Fatalf("Sqrt(80) = %d, want floor 8", got)
	}
}

func TestHLEArcTan(t *testing.T) {
	m := system.New(nil, nil)
	m.CPU.WriteRegister(0, 0)
	hleArcTan(m)
	if got := m.CPU.ReadRegister(0); got != 0 {
		t.Fatalf("ArcTan(0) = %#x, want 0", got)
	}
	m.CPU.WriteRegister(0, 0x4000)
	hleArcTan(m)
	if got := m.CPU.ReadRegister(0); got != 0x2000 {
		t.Fatalf("ArcTan(1.0) = %#x, want 0x2000", got)
	}
}

func TestHLEIntrWaitAdvancesToTimerIRQAndAcknowledgesIt(t *testing.T) {
	m := system.New(nil, nil)
	m.Bus.Write16(bus.IOStart+0x100, 0xfffc, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, 0x00c0, bus.Access{}) // enable + IRQ, /1
	m.CPU.WriteRegister(0, 1)
	m.CPU.WriteRegister(1, uint32(gbairq.Timer0))

	start := m.Cycle()
	if err := hleIntrWait(m); err != nil {
		t.Fatal(err)
	}
	if got := m.Cycle() - start; got != 4 {
		t.Fatalf("IntrWait advanced %d cycles, want 4", got)
	}
	if got := m.IRQ.IF() & uint16(gbairq.Timer0); got != 0 {
		t.Fatalf("IntrWait left Timer0 IF set: %#04x", got)
	}
}
