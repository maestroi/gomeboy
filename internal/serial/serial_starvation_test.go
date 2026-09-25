package serial

import (
	"testing"
	"time"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

type neverReadyExternalClock struct {
	polls int
}

func (d *neverReadyExternalClock) Receive(bool) {}
func (d *neverReadyExternalClock) Send() bool   { return true }
func (d *neverReadyExternalClock) PollClock() (ClockPulse, bool) {
	d.polls++
	return ClockPulse{}, false
}
func (d *neverReadyExternalClock) ReplyClock(ClockPulse, bool) error { return nil }

func TestExternalClockPollingDoesNotStarveOtherEvents(t *testing.T) {
	s := scheduler.NewScheduler()
	b := io.NewBus(s, make([]byte, 0x8000))
	c := NewController(b, s)
	device := &neverReadyExternalClock{}
	c.Attach(device)

	// Request an externally-clocked transfer. The peer deliberately never
	// becomes ready, matching a slow/mid-negotiation network peer.
	b.Write(types.SC, types.Bit7)

	const frameAt = uint64(17556)
	frameReady := false
	s.RegisterEvent(scheduler.PPUHandleVisualLine, func() { frameReady = true })
	s.ScheduleEvent(scheduler.PPUHandleVisualLine, frameAt)

	// Model the CPU HALT fast-forward loop. With fixed 32-tick polling this
	// needs ~549 serial polls before the frame event can run. Backoff keeps
	// the poll event from monopolizing the HALT scheduler.
	const maxSkips = 64
	for i := 0; i < maxSkips && !frameReady; i++ {
		s.Skip()
	}

	if !frameReady {
		t.Fatalf("frame event starved after %d scheduler skips; serial polls=%d cycle=%d", maxSkips, device.polls, s.Cycle())
	}
	if device.polls >= maxSkips {
		t.Fatalf("external clock did not back off: %d polls in at most %d skips", device.polls, maxSkips)
	}
	if !c.TransferRequest {
		t.Fatal("pending external transfer was incorrectly cancelled")
	}
}

// Pokemon Red's Pokemon Center script re-arms an external-clock transfer
// (SC=$80) every frame while the previous poll is still pending, and its Cable
// Club re-arms internal clock the same way. The scheduler pools one node per
// event type, so scheduling again without descheduling linked that node into
// the list twice: the next list walk never terminated and StepFrame hung.
func TestRearmingSCWhilePendingKeepsSchedulerFinite(t *testing.T) {
	for _, sc := range []byte{types.Bit7, types.Bit7 | types.Bit0} {
		s := scheduler.NewScheduler()
		b := io.NewBus(s, make([]byte, 0x8000))
		c := NewController(b, s)
		c.Attach(&neverReadyExternalClock{})

		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := 0; i < 4; i++ {
				b.Write(types.SC, sc)
				s.Tick(40) // the first poll/bit reschedules itself
				b.Write(types.SC, sc)
				s.Tick(16384)
			}
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("SC=%#02x: scheduler looped forever after SC was re-armed mid-transfer", sc)
		}
	}
}
