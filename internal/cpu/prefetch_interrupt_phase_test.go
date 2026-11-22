package cpu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// Prefetch is a full four-T-cycle M-cycle, but the interrupt acceptance
// decision is sampled after T2. Verify both the acceptance boundary and
// whether the opcode is fetched (PC changes) on either side of that edge.
func TestPrefetchInterruptAcceptancePhase(t *testing.T) {
	for _, hw := range []struct {
		name   string
		model  types.Model
		double bool
	}{
		{"DMG", types.DMGABC, false},
		{"CGB-BC-normal", types.CGBBC, false},
		{"CGB-DE-normal", types.CGBDE, false},
		{"CGB-BC-double", types.CGBBC, true},
		{"CGB-DE-double", types.CGBDE, true},
	} {
		for _, tc := range []struct {
			name       string
			raiseAt    uint64
			ime        bool
			accepted   bool
			wantOpcode byte
		}{
			{"pending-before-fetch", 0, true, true, 0},
			{"assert-at-T1", 1, true, true, 0},
			{"assert-at-T2", 2, true, true, 0},
			{"assert-at-T3", 3, true, false, 0x00},
			{"assert-at-T4", 4, true, false, 0x00},
			{"IME-disabled", 2, false, false, 0x00},
		} {
			t.Run(hw.name+"/"+tc.name, func(t *testing.T) {
				s := scheduler.NewScheduler()
				b := gbio.NewBus(s, make([]byte, 0x8000))
				b.Map(hw.model)
				c := NewCPU(b, s)
				if hw.double {
					s.ChangeSpeed(true)
				}
				c.PC, c.SP = 0xff80, 0xc100
				b.Set(0xff80, 0x00) // NOP in HRAM (uncontended bus)
				b.Set(types.IE, gbio.LCDINT)
				b.Set(types.IF, 0) // clear boot/initial IF state before scheduling the edge
				if tc.ime {
					b.EnableInterrupts()
				}
				if tc.raiseAt == 0 {
					b.RaiseInterrupt(gbio.LCDINT)
				} else {
					s.RegisterEvent(scheduler.TimerTIMAReload, func() {
						b.RaiseInterrupt(gbio.LCDINT)
					})
					s.ScheduleEvent(scheduler.TimerTIMAReload, tc.raiseAt)
				}

				op, accepted := c.prefetchOpcode()
				if accepted != tc.accepted {
					t.Errorf("interrupt accepted=%t; expected %t on prefetch T2", accepted, tc.accepted)
				}
				if op != tc.wantOpcode {
					t.Errorf("opcode read=%#02x, expected %#02x", op, tc.wantOpcode)
				}
				pc := uint16(0xff81)
				if tc.accepted {
					pc = 0xff80
				}
				if c.PC != pc {
					t.Errorf("PC after fetch=%04x, expected %04x", c.PC, pc)
				}
				if got := s.Cycle(); got != 4 {
					t.Errorf("prefetch length=%d, expected 4 T-cycles", got)
				}
			})
		}
	}
}

// A request accepted at T2 is latched. When IF is withdrawn at T3,
// hardware still performs the five-M-cycle interrupt entry. The existing
// IRQVector resolver returns $0000 when nothing remains pending; the old
// Frame handler incorrectly rechecked CanInterrupt at T4 and skipped the
// acknowledged interrupt entirely.
func TestPrefetchLatchedInterruptSurvivesIFWithdrawalBeforeT4(t *testing.T) {
	for _, hw := range []struct {
		name   string
		model  types.Model
		double bool
	}{
		{"DMG", types.DMGABC, false},
		{"CGB-normal", types.CGBBC, false},
		{"CGB-double", types.CGBDE, true},
	} {
		t.Run(hw.name, func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(hw.model)
			c := NewCPU(b, s)
			if hw.double {
				s.ChangeSpeed(true)
			}
			c.PC, c.SP = 0xff80, 0xc100
			b.Set(0xff80, 0x00) // would have been a NOP
			b.Set(types.IE, gbio.LCDINT)
			b.Set(types.IF, 0)
			b.EnableInterrupts()

			// End Frame after the accepted interrupt, so the unit
			// test does not advance into the $0000 vector ROM.
			c.Debug = true
			c.DebugBreakpoint = true
			// T1: request STAT after the opcode prefetch begins.
			// T3: withdraw it after the CPU's T2 acceptance sample.
			s.RegisterEvent(scheduler.TimerTIMAReload, func() {
				b.RaiseInterrupt(gbio.LCDINT)
			})
			s.ScheduleEvent(scheduler.TimerTIMAReload, 1)
			s.RegisterEvent(scheduler.TimerTIMAIncrement, func() {
				b.Set(types.IF, 0)
			})
			s.ScheduleEvent(scheduler.TimerTIMAIncrement, 3)

			c.Frame()

			if got := s.Cycle(); got != 24 {
				t.Errorf("accepted IRQ service length=%d; want prefetch 4 + service 20", got)
			}
			if c.PC != 0 || c.SP != 0xc0fe {
				t.Errorf("CPU failed to enter withdrawn-vector path: PC=%04x SP=%04x", c.PC, c.SP)
			}
			if hi, lo := b.Get(0xc0ff), b.Get(0xc0fe); hi != 0xff || lo != 0x80 {
				t.Errorf("IRQ stack return address=%02x%02x, want FF80", hi, lo)
			}
			if b.InterruptsEnabled() {
				t.Error("accepted IRQ failed to clear IME")
			}
			if got := b.Get(types.IF) & 0x1f; got != 0 {
				t.Errorf("withdrawn IF should remain clear, got %#02x", got)
			}
		})
	}
}

func TestPrefetchHaltBugDefersUntilOpcodeActuallyFetched(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	c := NewCPU(b, s)
	c.PC = 0xff81
	c.haltBug = true
	b.Set(0xff80, 0x3e)
	b.Set(types.IE, gbio.LCDINT)
	b.Set(types.IF, gbio.LCDINT)
	b.EnableInterrupts()

	if _, accepted := c.prefetchOpcode(); !accepted {
		t.Fatal("pending IRQ was not accepted")
	}
	if c.PC != 0xff81 || !c.haltBug {
		t.Fatalf("cancelled fetch consumed HALT bug: PC=%04x haltBug=%t", c.PC, c.haltBug)
	}
	b.Set(types.IF, 0)
	b.DisableInterrupts()
	op, accepted := c.prefetchOpcode()
	if accepted || op != 0x3e || c.PC != 0xff81 || c.haltBug {
		t.Fatalf("deferred HALT bug: accepted=%t op=%02x PC=%04x haltBug=%t",
			accepted, op, c.PC, c.haltBug)
	}
}
