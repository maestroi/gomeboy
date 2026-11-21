package cpu

import (
	"testing"

	gbio "github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

// The five interrupt-entry M-cycles must stay individually observable to
// other scheduled devices. These snapshots are deliberately taken *between*
// M-cycle boundaries, and not after serviceInterrupt has finished.
func TestInterruptEntryFiveMachineCycleBusPhases(t *testing.T) {
	for _, tc := range []struct {
		name string
		model types.Model
		double bool
	}{
		{"DMG", types.DMGABC, false},
		{"CGB-normal", types.CGBBC, false},
		{"CGB-double", types.CGBDE, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := scheduler.NewScheduler()
			b := gbio.NewBus(s, make([]byte, 0x8000))
			b.Map(tc.model)
			c := NewCPU(b, s)
			if tc.double {
				s.ChangeSpeed(true)
			}
			c.PC, c.SP = 0x3456, 0xc100
			b.Set(types.IE, gbio.LCDINT)
			b.Set(types.IF, gbio.LCDINT)
			b.EnableInterrupts()

			type sample struct {
				at uint64
				sp, pc uint16
				high, low, flags byte
			}
			var samples []sample
			// Use independent non-PPU event slots: double-speed changes
			// PPU-dot scheduling but not CPU T-cycle/M-cycle lengths.
			for _, v := range []struct {
				event scheduler.EventType
				cycle uint64
			}{
				{scheduler.DMAStartTransfer, 7},  // second acknowledge M-cycle
				{scheduler.DMAEndTransfer, 11},  // first stack access in flight
				{scheduler.DMATransfer, 13},     // second stack access in flight
				{scheduler.TimerTIMAReload, 15}, // low stack byte in flight
				{scheduler.TimerTIMAIncrement, 17}, // vector selected
			} {
				s.RegisterEvent(v.event, func() {
					samples = append(samples, sample{s.Cycle(), c.SP, c.PC,
						b.Get(0xc0ff), b.Get(0xc0fe), b.Get(types.IF)&0x1f})
				})
				s.ScheduleEvent(v.event, v.cycle)
			}
			c.serviceInterrupt()
			if got := s.Cycle(); got != 20 {
				t.Fatalf("IRQ service length = %d T-cycles; want five M-cycles (20)", got)
			}
			want := []sample{
				{7, 0xc100, 0x3456, 0, 0, gbio.LCDINT},
				{11, 0xc0ff, 0x3456, 0, 0, gbio.LCDINT},
				{13, 0xc0fe, 0x3456, 0x34, 0, gbio.LCDINT},
				{15, 0xc0fe, 0x3456, 0x34, 0, gbio.LCDINT},
				{17, 0xc0fe, 0x0048, 0x34, 0x56, 0},
			}
			if len(samples) != len(want) {
				t.Fatalf("IRQ bus snapshots = %v; want %v", samples, want)
			}
			for i, w := range want {
				if samples[i] != w {
					t.Errorf("IRQ phase %d snapshot = %+v; want %+v", i, samples[i], w)
				}
			}
			if c.SP != 0xc0fe || c.PC != 0x0048 ||
				b.Get(0xc0ff) != 0x34 || b.Get(0xc0fe) != 0x56 ||
				b.Get(types.IF)&gbio.LCDINT != 0 || b.InterruptsEnabled() {
				t.Fatalf("IRQ final state: PC=%04x SP=%04x stack=%02x %02x IF=%02x IME=%v",
					c.PC, c.SP, b.Get(0xc0ff), b.Get(0xc0fe),
					b.Get(types.IF), b.InterruptsEnabled())
			}
		})
	}
}

func TestInterruptEntryResamplesPriorityAfterHighStackPush(t *testing.T) {
	s := scheduler.NewScheduler()
	b := gbio.NewBus(s, make([]byte, 0x8000))
	b.Map(types.DMGABC)
	c := NewCPU(b, s)
	c.PC, c.SP = 0x3456, 0xc100
	b.Set(types.IE, gbio.LCDINT)
	b.Set(types.IF, gbio.VBlankINT|gbio.LCDINT)
	b.EnableInterrupts()

	// IE changes during M3, before the CPU samples its IRQ priority.
	s.RegisterEvent(scheduler.TimerTIMAReload, func() {
		b.Set(types.IE, gbio.VBlankINT)
	})
	s.ScheduleEvent(scheduler.TimerTIMAReload, 11)
	c.serviceInterrupt()

	if got := c.PC; got != 0x0040 {
		t.Errorf("interrupt vector after IE change = %04x; want VBlank $0040", got)
	}
	if got := b.Get(types.IF)&0x1f; got != gbio.LCDINT {
		t.Errorf("pending IF after switching interrupt priority = %02x; want STAT only", got)
	}
	if c.SP != 0xc0fe || b.Get(0xc0ff) != 0x34 || b.Get(0xc0fe) != 0x56 {
		t.Errorf("interrupt push changed by IE race: PC=%04x SP=%04x", c.PC, c.SP)
	}
}
