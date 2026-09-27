// Package mgbasuite adapts the upstream mgba-emu/suite ROM to GomeBoy's
// deterministic headless GBA machine.
package mgbasuite

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
	"github.com/maestroi/gomeboy/internal/gba/cpu"
	"github.com/maestroi/gomeboy/internal/gba/keypad"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
	"github.com/maestroi/gomeboy/internal/gba/system"
)

const (
	sramLogSize = 32 * 1024
	defaultStepLimit uint64 = 25_000_000
)

type CategoryStatus string

const (
	StatusExpected   CategoryStatus = "expected"
	StatusRegression CategoryStatus = "regression"
	StatusXPASS      CategoryStatus = "xpass"
	StatusError      CategoryStatus = "error"
	StatusNotRun     CategoryStatus = "not-run"
	StatusUnbaselined CategoryStatus = "unbaselined"
)

type Baseline struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	Total  int `json:"total"`
}

type Category struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	MenuIndex     int       `json:"menu_index"`
	ExpectedTotal int       `json:"expected_total"`
	Enabled       bool      `json:"enabled"`
	StepLimit     uint64    `json:"step_limit,omitempty"`
	Baseline      *Baseline `json:"baseline,omitempty"`
}

type Config struct {
	SchemaVersion int        `json:"schema_version"`
	Suite         string     `json:"suite"`
	SuiteRevision string     `json:"suite_revision"`
	SourceCommit  string     `json:"source_commit"`
	ROM_SHA256    string     `json:"rom_sha256,omitempty"`
	Categories    []Category `json:"categories"`
}

type CategoryResult struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Status   CategoryStatus `json:"status"`
	Passed   int            `json:"passed"`
	Failed   int            `json:"failed"`
	Total    int            `json:"total"`
	PassRate float64        `json:"pass_rate"`
	Steps    uint64         `json:"steps"`
	Cycles   uint64         `json:"cycles"`
	Detail   string         `json:"detail,omitempty"`
	Failures []string       `json:"failures,omitempty"`
}

type Report struct {
	SchemaVersion int              `json:"schema_version"`
	Suite         string           `json:"suite"`
	SuiteRevision string           `json:"suite_revision"`
	SourceCommit  string           `json:"source_commit"`
	GomeBoyCommit string           `json:"gomeboy_commit"`
	ROMSHA256     string           `json:"rom_sha256"`
	Categories    []CategoryResult `json:"categories"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode mGBA-suite config: %w", err)
	}
	if cfg.SchemaVersion != 1 {
		return Config{}, fmt.Errorf("unsupported mGBA-suite config schema %d", cfg.SchemaVersion)
	}
	if cfg.Suite == "" || cfg.SuiteRevision == "" || cfg.SourceCommit == "" {
		return Config{}, fmt.Errorf("suite, suite_revision, and source_commit are required")
	}
	if len(cfg.Categories) == 0 {
		return Config{}, fmt.Errorf("at least one mGBA-suite category is required")
	}
	for i, category := range cfg.Categories {
		if category.ID == "" || category.Name == "" {
			return Config{}, fmt.Errorf("category %d must have id and name", i)
		}
		if category.MenuIndex < 0 {
			return Config{}, fmt.Errorf("category %s has negative menu index", category.ID)
		}
		if category.Enabled && category.ExpectedTotal <= 0 {
			return Config{}, fmt.Errorf("enabled category %s needs expected_total", category.ID)
		}
	}
	return cfg, nil
}

func Run(cfg Config, rom []byte, commit string) Report {
	hash := sha256.Sum256(rom)
	report := Report{
		SchemaVersion: 1,
		Suite: cfg.Suite,
		SuiteRevision: cfg.SuiteRevision,
		SourceCommit: cfg.SourceCommit,
		GomeBoyCommit: commit,
		ROMSHA256: hex.EncodeToString(hash[:]),
		Categories: make([]CategoryResult, 0, len(cfg.Categories)),
	}
	if report.GomeBoyCommit == "" {
		report.GomeBoyCommit = "unknown"
	}

	hashOK := cfg.ROM_SHA256 == "" || strings.EqualFold(cfg.ROM_SHA256, report.ROMSHA256)
	for _, category := range cfg.Categories {
		if !category.Enabled {
			report.Categories = append(report.Categories, CategoryResult{
				ID: category.ID, Name: category.Name, Status: StatusNotRun,
				Detail: "category is registered but not enabled in the checked-in baseline",
			})
			continue
		}
		if !hashOK {
			report.Categories = append(report.Categories, CategoryResult{
				ID: category.ID, Name: category.Name, Status: StatusError,
				Detail: fmt.Sprintf("ROM SHA-256 mismatch: got %s, want %s", report.ROMSHA256, cfg.ROM_SHA256),
			})
			continue
		}
		result := runCategory(rom, category)
		classify(&result, category)
		report.Categories = append(report.Categories, result)
	}
	return report
}

func (r Report) Passed() bool {
	ran := false
	for _, category := range r.Categories {
		switch category.Status {
		case StatusNotRun:
			continue
		case StatusExpected:
			ran = true
		default:
			return false
		}
	}
	return ran
}

func classify(result *CategoryResult, category Category) {
	if result.Status == StatusError {
		return
	}
	if result.Total != category.ExpectedTotal {
		result.Status = StatusError
		result.Detail = fmt.Sprintf("category produced %d results, want %d", result.Total, category.ExpectedTotal)
		return
	}
	if category.Baseline == nil {
		result.Status = StatusUnbaselined
		result.Detail = "no checked-in baseline yet; record this result intentionally"
		return
	}
	if category.Baseline.Total != category.ExpectedTotal {
		result.Status = StatusError
		result.Detail = fmt.Sprintf("baseline total %d does not match expected_total %d", category.Baseline.Total, category.ExpectedTotal)
		return
	}
	switch {
	case result.Passed < category.Baseline.Passed || result.Failed > category.Baseline.Failed:
		result.Status = StatusRegression
		result.Detail = fmt.Sprintf("baseline %d/%d passed; current %d/%d", category.Baseline.Passed, category.Baseline.Total, result.Passed, result.Total)
	case result.Passed > category.Baseline.Passed || result.Failed < category.Baseline.Failed:
		result.Status = StatusXPASS
		result.Detail = fmt.Sprintf("accuracy improved from baseline %d/%d to %d/%d; baseline update required", category.Baseline.Passed, category.Baseline.Total, result.Passed, result.Total)
	default:
		result.Status = StatusExpected
	}
}

func runCategory(rom []byte, category Category) CategoryResult {
	result := CategoryResult{ID: category.ID, Name: category.Name}
	stepLimit := category.StepLimit
	if stepLimit == 0 {
		stepLimit = defaultStepLimit
	}

	m := system.NewWithCartridgeConfig(nil, rom, cartridge.Config{SaveType: cartridge.SaveSRAM})
	m.Audio.SetHeadless(true)
	if err := initPostBIOS(m); err != nil {
		result.Status = StatusError
		result.Detail = err.Error()
		return result
	}
	if m.Cartridge.Save == nil {
		result.Status = StatusError
		result.Detail = "suite SRAM device was not attached"
		return result
	}

	driver := newMenuDriver(m, category.MenuIndex)
	collector := resultCollector{}
	startCycle := m.Cycle()
	for result.Steps < stepLimit {
		stepResult, err := m.Step()
		result.Steps++
		if err != nil {
			result.Status = StatusError
			result.Detail = fmt.Sprintf("emulation error at pc=%#08x cpsr=%#08x: %v", m.CPU.PC(), uint32(m.CPU.CPSR()), err)
			result.Cycles = m.Cycle() - startCycle
			return result
		}
		if stepResult.CPU.ExceptionTaken && stepResult.CPU.Exception == cpu.ExceptionIRQ {
			if err := handleSuiteIRQ(m); err != nil {
				result.Status = StatusError
				result.Detail = err.Error()
				result.Cycles = m.Cycle() - startCycle
				return result
			}
		}
		if m.CPU.PC() == 0x08 && m.CPU.CPSR().Mode() == cpu.ModeSupervisor {
			number, handled, err := handleSuiteSWI(m)
			if err != nil {
				result.Status = StatusError
				result.Detail = err.Error()
				result.Cycles = m.Cycle() - startCycle
				return result
			}
			if handled && number == 0x05 {
				driver.vblankWait()
			}
			if !handled {
				result.Status = StatusError
				result.Detail = fmt.Sprintf("mGBA-suite invoked unsupported BIOS SWI 0x%02x", number)
				result.Cycles = m.Cycle() - startCycle
				return result
			}
		}

		collector.poll(m.Cartridge.Save)
		result.Failed = collector.failed
		result.Failures = collector.failures
		if passed, total, ok := readSuiteCount(m.Bus); ok {
			result.Passed = passed
			result.Total = total
			result.Failed = total - passed
			result.Cycles = m.Cycle() - startCycle
			if result.Total > 0 {
				result.PassRate = math.Round((float64(result.Passed)/float64(result.Total))*10000) / 100
			}
			if collector.failed != result.Failed {
				result.Status = StatusError
				result.Detail = fmt.Sprintf("suite screen reports %d failures, SRAM log contains %d failure lines", result.Failed, collector.failed)
			}
			return result
		}
	}
	result.Status = StatusError
	result.Cycles = m.Cycle() - startCycle
	result.Detail = fmt.Sprintf("step budget %d exhausted before suite result screen; observed %d SRAM failure lines", stepLimit, collector.failed)
	if collector.pending != "" {
		result.Detail += "; last partial SRAM line: " + collector.pending
	}
	return result
}

func initPostBIOS(m *system.Machine) error {
	if err := m.CPU.SetMode(cpu.ModeIRQ); err != nil {
		return err
	}
	m.CPU.WriteRegister(13, 0x03007fa0)
	if err := m.CPU.SetMode(cpu.ModeSupervisor); err != nil {
		return err
	}
	m.CPU.WriteRegister(13, 0x03007fe0)
	if err := m.CPU.SetCPSR(cpu.PSR(cpu.ModeSystem)); err != nil {
		return err
	}
	m.CPU.WriteRegister(13, 0x03007f00)
	m.CPU.SetPC(bus.ROM0Start)
	return nil
}

type menuDriver struct {
	m          *system.Machine
	target     int
	moved      int
	releaseGap bool
	started    bool
}

func newMenuDriver(m *system.Machine, target int) *menuDriver {
	d := &menuDriver{m: m, target: target}
	if target == 0 {
		m.Keypad.Press(keypad.ButtonA)
		d.started = true
	} else {
		m.Keypad.Press(keypad.ButtonDown)
	}
	return d
}

func (d *menuDriver) vblankWait() {
	if d.started {
		d.m.Keypad.Release(keypad.ButtonA)
		return
	}
	if d.releaseGap {
		d.releaseGap = false
		d.m.Keypad.Press(keypad.ButtonDown)
		return
	}

	d.moved++
	d.m.Keypad.Release(keypad.ButtonDown)
	if d.moved >= d.target {
		d.m.Keypad.Press(keypad.ButtonA)
		d.started = true
		return
	}
	d.releaseGap = true
}

func handleSuiteIRQ(m *system.Machine) error {
	pending := m.IRQ.EnabledPendingMask()
	if pending&uint16(gbairq.VBlank) == 0 {
		return fmt.Errorf("mGBA-suite entered unsupported IRQ with IE&IF=%#04x", pending)
	}

	// The stock suite enables VBlank IRQ only for its menu/UI pacing. The
	// no-BIOS harness already handles VBlankIntrWait directly, so acknowledge
	// that UI interrupt and perform the architectural IRQ return here. Other
	// interrupt sources remain visible and will be rejected on a later IRQ
	// entry until their categories get a proper BIOS/IRQ adapter.
	m.Bus.Write16(bus.IOStart+0x202, uint16(gbairq.VBlank), bus.Access{})
	lr := m.CPU.ReadRegister(14)
	if lr < 4 {
		return fmt.Errorf("invalid IRQ return address %#x", lr)
	}
	if err := m.CPU.RestoreCPSRFromSPSR(); err != nil {
		return fmt.Errorf("restore IRQ SPSR: %w", err)
	}
	m.CPU.SetPC(lr - 4)
	return nil
}

func handleSuiteSWI(m *system.Machine) (byte, bool, error) {
	saved, ok := m.CPU.SPSR()
	if !ok {
		return 0, false, fmt.Errorf("SWI handler entered without SPSR")
	}
	lr := m.CPU.ReadRegister(14)
	var number byte
	if saved.Thumb() {
		if lr < 2 {
			return 0, false, fmt.Errorf("invalid Thumb SWI return address %#x", lr)
		}
		instruction := uint16(m.Bus.Peek8(lr-2)) | uint16(m.Bus.Peek8(lr-1))<<8
		if instruction&0xff00 != 0xdf00 {
			return 0, false, fmt.Errorf("exception at BIOS SWI vector did not follow Thumb SWI: %#04x", instruction)
		}
		number = byte(instruction)
	} else {
		if lr < 4 {
			return 0, false, fmt.Errorf("invalid ARM SWI return address %#x", lr)
		}
		addr := lr - 4
		instruction := uint32(m.Bus.Peek8(addr)) |
			uint32(m.Bus.Peek8(addr+1))<<8 |
			uint32(m.Bus.Peek8(addr+2))<<16 |
			uint32(m.Bus.Peek8(addr+3))<<24
		if instruction&0x0f000000 != 0x0f000000 {
			return 0, false, fmt.Errorf("exception at BIOS SWI vector did not follow ARM SWI: %#08x", instruction)
		}
		number = byte(instruction >> 16)
	}

	switch number {
	case 0x05: // VBlankIntrWait: only used for suite UI pacing here.
	case 0x0b:
		hleCPUSet(m, false)
	case 0x0c:
		hleCPUSet(m, true)
	default:
		return number, false, nil
	}
	if err := m.CPU.RestoreCPSRFromSPSR(); err != nil {
		return number, true, err
	}
	m.CPU.SetPC(lr)
	return number, true, nil
}

func hleCPUSet(m *system.Machine, fast bool) {
	src := m.CPU.ReadRegister(0)
	dst := m.CPU.ReadRegister(1)
	control := m.CPU.ReadRegister(2)
	count := control & 0x000fffff
	fill := control&(1<<24) != 0
	word := fast || control&(1<<26) != 0
	access := bus.Access{}

	if word {
		src &^= 3
		dst &^= 3
		var fillValue uint32
		if fill {
			fillValue, _ = m.Bus.Read32(src, access)
		}
		for i := uint32(0); i < count; i++ {
			value := fillValue
			if !fill {
				value, _ = m.Bus.Read32(src, access)
				src += 4
			}
			m.Bus.Write32(dst, value, access)
			dst += 4
		}
	} else {
		src &^= 1
		dst &^= 1
		var fillValue uint16
		if fill {
			fillValue, _ = m.Bus.Read16(src, access)
		}
		for i := uint32(0); i < count; i++ {
			value := fillValue
			if !fill {
				value, _ = m.Bus.Read16(src, access)
				src += 2
			}
			m.Bus.Write16(dst, value, access)
			dst += 2
		}
	}
	if !fast {
		m.CPU.WriteRegister(3, 0x170)
	}
}

type resultCollector struct {
	offset   uint32
	pending  string
	failed   int
	failures []string
}

func (c *resultCollector) poll(save bus.SaveDevice) {
	if save == nil || c.offset >= sramLogSize {
		return
	}
	start := c.offset
	data := make([]byte, 0, 256)
	for c.offset < sramLogSize {
		b := save.Read8(c.offset)
		if b == 0 || b == 0xff {
			break
		}
		data = append(data, b)
		c.offset++
	}
	if c.offset == start {
		return
	}
	text := c.pending + string(data)
	lines := strings.Split(text, "\n")
	c.pending = lines[len(lines)-1]
	for _, line := range lines[:len(lines)-1] {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, "FAIL") {
			c.failed++
			c.failures = append(c.failures, line)
		}
	}
}

func readSuiteCount(b *bus.Bus) (passed, total int, ok bool) {
	const (
		gridStride = 32
		row = 1
		column = 21
		width = 9
	)
	var text [width]byte
	for i := 0; i < width; i++ {
		tile := b.Peek8(bus.VRAMStart + uint32((row*gridStride+column+i)*2))
		if tile == 0 {
			text[i] = ' '
		} else {
			text[i] = tile + ' '
		}
	}
	parts := strings.Split(string(text[:]), "/")
	if len(parts) != 2 {
		return 0, 0, false
	}
	p, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	t, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || t == 0 || p < 0 || p > t {
		return 0, 0, false
	}
	return p, t, true
}

func readSRAMLog(save bus.SaveDevice) string {
	data := make([]byte, 0, sramLogSize)
	for i := uint32(0); i < sramLogSize; i++ {
		b := save.Read8(i)
		if b == 0 || b == 0xff {
			break
		}
		data = append(data, b)
	}
	return string(data)
}

func parseResults(log string) (passed, failed int, failures []string) {
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasSuffix(line, "PASS"):
			passed++
		case strings.HasSuffix(line, "FAIL"):
			failed++
			failures = append(failures, line)
		}
	}
	return passed, failed, failures
}
