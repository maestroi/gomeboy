package serial

import (
	"testing"

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
