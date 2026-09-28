package mgbasuite

import (
	"reflect"
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


func TestResultCollectorRetainsMemoryTestContext(t *testing.T) {
	save := cartridge.NewSRAM()
	log := "Memory test: BIOS load\nU8: Got 0x00 vs 0x04: FAIL\nMemory test: OAM load\n32: Got 1 vs 2: FAIL\n"
	for index := range []byte(log) {
		save.Write8(uint32(index), []byte(log)[index])
	}
	collector := resultCollector{}
	collector.poll(save)
	want := []string{
		"BIOS load: U8: Got 0x00 vs 0x04: FAIL",
		"OAM load: 32: Got 1 vs 2: FAIL",
	}
	if len(collector.failures) != len(want) {
		t.Fatalf("failures = %v, want %v", collector.failures, want)
	}
	for index := range want {
		if collector.failures[index] != want[index] {
			t.Fatalf("failure %d = %q, want %q", index, collector.failures[index], want[index])
		}
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


func TestHLECPUSetPreservesSRAMSourceLane(t *testing.T) {
	m := system.NewWithCartridgeConfig(nil, nil, cartridge.Config{SaveType: cartridge.SaveSRAM})
	dst := uint32(bus.IWRAMStart + 0x1c0)
	for index, value := range []byte{0x47, 0x61, 0x6d, 0x65} {
		m.Bus.Write8(bus.SaveStart+uint32(index), value, bus.Access{})
	}

	m.CPU.WriteRegister(0, bus.SaveStart+1)
	m.CPU.WriteRegister(1, dst)
	m.CPU.WriteRegister(2, (1<<26)|1)
	hleCPUSet(m, false)
	if got, _ := m.Bus.Read32(dst, bus.Access{}); got != 0x61616161 {
		t.Fatalf("unaligned SRAM CpuSet word = %08x, want 61616161", got)
	}

	m.CPU.WriteRegister(0, bus.SaveStart+1)
	m.CPU.WriteRegister(1, dst+4)
	m.CPU.WriteRegister(2, 1)
	hleCPUSet(m, false)
	if got, _ := m.Bus.Read16(dst+4, bus.Access{}); got != 0x0061 {
		t.Fatalf("unaligned SRAM CpuSet halfword = %04x, want 0061", got)
	}
}

func TestHLECPUSetRejectsProtectedBIOSSource(t *testing.T) {
	bios := make([]byte, bus.BIOSSize)
	bios[0], bios[1], bios[2], bios[3] = 0x11, 0x22, 0x33, 0x44
	m := system.New(bios, nil)
	dst := uint32(bus.IWRAMStart + 0x180)
	m.Bus.Write32(dst, 0xa5a5a5a5, bus.Access{})

	m.CPU.WriteRegister(0, bus.BIOSStart)
	m.CPU.WriteRegister(1, dst)
	m.CPU.WriteRegister(2, (1<<26)|1)
	hleCPUSet(m, false)

	if got, _ := m.Bus.Read32(dst, bus.Access{}); got != 0xa5a5a5a5 {
		t.Fatalf("CpuSet BIOS source changed destination to %08x", got)
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


func TestSuiteBIOSContainsRealIRQForwarder(t *testing.T) {
	bios := suiteBIOS()
	want := map[uint32]uint32{
		0x018: 0xea000042,
		0x128: 0xe92d500f,
		0x12c: 0xe3a00301,
		0x130: 0xe28fe000,
		0x134: 0xe510f004,
		0x138: 0xe8bd500f,
		0x13c: 0xe25ef004,
	}
	for addr, instruction := range want {
		got := uint32(bios[addr]) |
			uint32(bios[addr+1])<<8 |
			uint32(bios[addr+2])<<16 |
			uint32(bios[addr+3])<<24
		if got != instruction {
			t.Fatalf("BIOS IRQ instruction at %#03x = %#08x, want %#08x", addr, got, instruction)
		}
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

func TestHLEIntrWaitRunsRegisteredTimerIRQHandler(t *testing.T) {
	m := system.New(suiteBIOS(), nil)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSupervisor) | cpu.FlagIRQDisable); err != nil {
		t.Fatal(err)
	}
	m.CPU.SetPC(0x08)

	// Minimal ARM user IRQ handler. The BIOS wrapper enters with r0=IOStart.
	// Disable TM0, acknowledge Timer0 IF, then return to the BIOS wrapper.
	handler := uint32(bus.IWRAMStart + 0x100)
	instructions := []uint32{
		0xe1a02000, // MOV  r2,r0
		0xe2800c01, // ADD  r0,r0,#0x100
		0xe3a01000, // MOV  r1,#0
		0xe5801000, // STR  r1,[r0]      ; TM0CNT_L/H = 0
		0xe2822c02, // ADD  r2,r2,#0x200
		0xe3a01008, // MOV  r1,#8
		0xe5c21002, // STRB r1,[r2,#2]   ; acknowledge IF.Timer0
		0xe12fff1e, // BX   lr
	}
	for i, instruction := range instructions {
		m.Bus.Write32(handler+uint32(i*4), instruction, bus.Access{})
	}
	m.Bus.Write32(0x03007ffc, handler, bus.Access{})

	m.Bus.Write16(bus.IOStart+0x200, uint16(gbairq.Timer0), bus.Access{})
	m.Bus.Write16(bus.IOStart+0x208, 1, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x100, 0xfffc, bus.Access{})
	m.Bus.Write16(bus.IOStart+0x102, 0x00c0, bus.Access{}) // enable + IRQ, /1
	m.CPU.WriteRegister(0, 1)
	m.CPU.WriteRegister(1, uint32(gbairq.Timer0))

	if err := hleIntrWait(m); err != nil {
		t.Fatal(err)
	}
	if got := m.Timers.Control(0); got != 0 {
		t.Fatalf("IntrWait returned before Timer0 handler disabled timer: %#04x", got)
	}
	if got := m.IRQ.IF() & uint16(gbairq.Timer0); got != 0 {
		t.Fatalf("IntrWait returned before Timer0 IF acknowledge: %#04x", got)
	}
	if got := m.CPU.CPSR().Mode(); got != cpu.ModeSupervisor || m.CPU.PC() != 0x08 {
		t.Fatalf("IntrWait IRQ return state mode=%v pc=%#08x", got, m.CPU.PC())
	}
}


func TestResultCollectorKeepsTimingTestContext(t *testing.T) {
	save := cartridge.NewSRAM()
	log := "Timing test: nop / ldrh r2, [sp]\nARM/ROM P..: Got 6 vs 4: FAIL\nTimer IRQ test: FFFF\nGot 0004 != 0051: FAIL\n"
	for index, value := range []byte(log) {
		save.Write8(uint32(index), value)
	}

	collector := resultCollector{}
	collector.poll(save)
	want := []string{
		"nop / ldrh r2, [sp]: ARM/ROM P..: Got 6 vs 4: FAIL",
		"FFFF: Got 0004 != 0051: FAIL",
	}
	if !reflect.DeepEqual(collector.failures, want) {
		t.Fatalf("failures = %#v, want %#v", collector.failures, want)
	}
}
