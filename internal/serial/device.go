package serial

// Device is a device that can be attached to the Controller.
type Device interface {
	Receive(bool)
	Send() bool
}

// BitExchanger is an optional extension for devices that can exchange one
// clocked bit atomically. Network-backed devices implement this so the
// controller can send the outgoing bit before waiting for the incoming bit.
type BitExchanger interface {
	ExchangeBit(out bool) (in bool, err error)
}

// ClockPulse represents a clock edge received from an external clock master.
// Sequence is transport metadata used to pair the response with the request.
type ClockPulse struct {
	Sequence uint64
	Incoming bool
}

// ExternalClockDevice is an optional extension for devices that can deliver
// externally-clocked link pulses without touching emulator state from a
// network goroutine. The serial controller polls these pulses on its own
// scheduler thread, preserving deterministic bus access.
type ExternalClockDevice interface {
	PollClock() (ClockPulse, bool)
	ReplyClock(ClockPulse, bool) error
}

// nullDevice is an implementation of Device that
// simply returns true on Send and does nothing on
// Receive. This is most commonly used for when no
// device is attached to the Controller.
type nullDevice struct{}

// Receive does nothing.
func (n nullDevice) Receive(bool) {}

// Send always returns true.
func (n nullDevice) Send() bool { return true }
