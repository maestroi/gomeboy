package serial

import (
	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

const (
	ticksPerBit          = 512
	externalPollInterval = 32
)

// Controller is the serial controller. It is responsible for sending and
// receiving data to and from devices.
// Before a transfer, data holds the next byte to be sent. AKA types.SB
// During a transfer, it has a mix of the incoming data and the outgoing data.
// each cycle, the leftmost bit of data is sent to the attached device, and
// shifted out of data, and the incoming bit is shifted into data.
//
// example:
//
//	Before : data = o7 o6 o5 o4 o3 o2 o1 o0
//	Cycle 1: data = o6 o5 o4 o3 o2 o1 o0 i0
//	Cycle 2: data = o5 o4 o3 o2 o1 o0 i0 i1
//	Cycle 3: data = o4 o3 o2 o1 o0 i0 i1 i2
//	Cycle 4: data = o3 o2 o1 o0 i0 i1 i2 i3
//	Cycle 5: data = o2 o1 o0 i0 i1 i2 i3 i4
//	Cycle 6: data = o1 o0 i0 i1 i2 i3 i4 i5
//	Cycle 7: data = o0 i0 i1 i2 i3 i4 i5 i6
//	Cycle 8: data = i0 i1 i2 i3 i4 i5 i6 i7
//
// Where o0-o7 are the outgoing bits, and i0-i7 are the incoming bits.
type Controller struct {
	count           uint8 // the number of bits that have been transferred.
	InternalClock   bool  // if true, this controller is the master.
	TransferRequest bool  // if true, a transfer has been requested.

	b              *io.Bus
	AttachedDevice Device // the device that is attached to this controller.

	s *scheduler.Scheduler // the scheduler.
}

// Attach attaches a Device to the Controller.
func (c *Controller) Attach(d Device) {
	if d == nil {
		d = nullDevice{}
	}
	c.AttachedDevice = d
}

// NewController creates a new Controller. A Controller is responsible for
// sending and receiving data to and from devices. It is also responsible for
// triggering serial interrupts.
//
// By default, the Controller is attached to a nullDevice, which acts as if
// there is no device attached. This is the same as if the device is not
// plugged in. If you want to attach a device, use the Controller.Attach method.
func NewController(b *io.Bus, s *scheduler.Scheduler) *Controller {
	c := &Controller{
		b:              b,
		AttachedDevice: nullDevice{},
		s:              s,
	}
	b.ReserveAddress(types.SB, func(v byte) byte {
		return v
	})
	b.ReserveAddress(types.SC, func(v byte) byte {
		c.InternalClock = (v & types.Bit0) == types.Bit0
		c.TransferRequest = (v & types.Bit7) == types.Bit7

		// was the transfer request bit set?
		if c.TransferRequest {
			// we need to determine when to schedule the first bit transfer,
			// when bit 8 of DIV produces a falling edge.
			// e.g.
			// DIV = 0b0000_0001_1111_1111 (511)
			// DIV = 0b0000_0010_0000_0000 (512) <- falling edge
			// DIV = 0b0000_0010_0000_0001 (513)
			// ...
			// DIV = 0b0000_0011_1111_1111 (1023)
			// DIV = 0b0000_0100_0000_0000 (1024) <- falling edge

			if c.InternalClock {
				// a bit is sent every 128 M-cycles (8.192 kHz)
				ticksToGo := (s.SysClock() + 4) & (ticksPerBit - 1)
				s.ScheduleEvent(scheduler.SerialBitTransfer, uint64(ticksPerBit-ticksToGo))
			} else if _, ok := c.AttachedDevice.(ExternalClockDevice); ok {
				// Network input is received on a goroutine, but consumed here on
				// the emulator scheduler thread so it never mutates the bus directly.
				s.ScheduleEvent(scheduler.SerialExternalClock, externalPollInterval)
			}
		}

		return v | 0x7E // bits 1-6 are always set
	})
	b.Set(types.SC, 0x7E) // bits 1-6 are unused

	s.RegisterEvent(scheduler.SerialBitTransfer, func() {
		if !c.InternalClock || !c.TransferRequest {
			return
		}

		out := c.b.Get(types.SB)&types.Bit7 == types.Bit7
		var in bool
		if exchanger, ok := c.AttachedDevice.(BitExchanger); ok {
			var err error
			in, err = exchanger.ExchangeBit(out)
			if err != nil {
				// An unplugged Game Boy link reads high. Treat network failure the
				// same way and let the transport expose the detailed error separately.
				in = true
			}
		} else {
			// Preserve the original Device ordering: sample the peer before
			// shifting our outgoing bit into it.
			in = c.AttachedDevice.Send()
			c.AttachedDevice.Receive(out)
		}

		c.shiftIncoming(in)
		c.count++
		if c.count == 8 {
			c.count = 0
			c.TransferRequest = false
			c.b.RaiseInterrupt(io.SerialINT)
			c.b.ClearBit(types.SC, types.Bit7)
		} else {
			ticksToGo := (s.SysClock() + 4) & (ticksPerBit - 1)
			s.ScheduleEvent(scheduler.SerialBitTransfer, uint64(ticksPerBit-ticksToGo))
		}
	})

	s.RegisterEvent(scheduler.SerialExternalClock, func() {
		if c.InternalClock || !c.TransferRequest {
			return
		}
		device, ok := c.AttachedDevice.(ExternalClockDevice)
		if !ok {
			return
		}

		pulse, ready := device.PollClock()
		if !ready {
			s.ScheduleEvent(scheduler.SerialExternalClock, externalPollInterval)
			return
		}

		out := c.b.Get(types.SB)&types.Bit7 == types.Bit7
		_ = device.ReplyClock(pulse, out)
		c.shiftIncoming(pulse.Incoming)
		c.checkTransfer()
		if c.TransferRequest {
			s.ScheduleEvent(scheduler.SerialExternalClock, externalPollInterval)
		}
	})

	s.RegisterEvent(scheduler.SerialBitInterrupt, func() {
		c.b.RaiseInterrupt(io.SerialINT)
	})
	return c
}

func (c *Controller) shiftIncoming(bit bool) {
	c.b.Set(types.SB, c.b.Get(types.SB)<<1)
	if bit {
		c.b.Set(types.SB, c.b.Get(types.SB)|1)
	}
}

// checkTransfer checks if a transfer has been completed, and if so,
// triggers a serial interrupt, and clears the transfer request.
func (c *Controller) checkTransfer() {
	if c.count++; c.count == 8 {
		c.count = 0
		c.b.RaiseInterrupt(io.SerialINT)

		// clear transfer request
		c.b.ClearBit(types.SC, types.Bit7)
		c.TransferRequest = false
	}
}

// Send returns the leftmost bit of the data register, unless
// the caller is the master, in which case it always returns true.
// This is because the master is driving the clock, and thus should
// not be trying to read from its own data register.
func (c *Controller) Send() bool {
	// if c is the master, return true.
	if c.InternalClock {
		return true
	}
	return (c.b.Get(types.SB) & types.Bit7) == types.Bit7
}

// Receive receives a bit from the attached device, and shifts it into
// the data register. If the caller is the master, it does nothing.
func (c *Controller) Receive(bit bool) {
	if !c.InternalClock {
		c.shiftIncoming(bit)
		c.checkTransfer()
	}
}

// State is a snapshot of the serial controller's execution state.
// The attached device is external and is not part of the state.
type State struct {
	Count           uint8
	InternalClock   bool
	TransferRequest bool
}

// Snapshot captures the serial controller's execution state.
func (c *Controller) Snapshot() State {
	return State{
		Count:           c.count,
		InternalClock:   c.InternalClock,
		TransferRequest: c.TransferRequest,
	}
}

// Restore rebuilds the serial controller's execution state from a snapshot.
func (c *Controller) Restore(s State) {
	c.count = s.Count
	c.InternalClock = s.InternalClock
	c.TransferRequest = s.TransferRequest
}
