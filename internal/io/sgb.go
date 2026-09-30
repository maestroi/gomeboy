package io

import "github.com/maestroi/gomeboy/internal/types"

const sgbCommandBytes = 16 * 7

// SGBState is the mutable Super Game Boy command/input state that must survive
// save states and checkpoints. SGB commands are clocked through P1 one bit at
// a time, so preserving partially received packets matters just as much as the
// active multiplayer controller.
type SGBState struct {
	Command           [sgbCommandBytes]byte
	CommandWriteIndex uint16
	ReadyForPulse     bool
	ReadyForWrite     bool
	ReadyForStop      bool

	// ControllerMask is the raw two-bit MLT_REQ value. Hardware uses it as a
	// mask, not a conventional player count: 0, 1 and 3 represent one, two and
	// four controllers, while 2 exposes the documented glitched mode.
	ControllerMask    uint8
	CurrentPlayer     uint8
	IncrementPending  bool
}

func (b *Bus) isSGB() bool {
	return b.model == types.SGB || b.model == types.SGB2
}

func (b *Bus) initSGB() {
	b.sgb = SGBState{
		// P1 is idle with both select lines high. Treat that high state as the
		// pulse that arms the first $00 packet-reset write.
		ReadyForPulse: true,
	}
}

func (b *Bus) writeSGBP1(value byte) byte {
	previous := b.data[types.P1]
	selectBits := value & 0x30

	b.handleSGBControllerTransition(selectBits, previous)
	b.handleSGBPacketWrite(selectBits)

	result := uint8(0xC0) | selectBits
	if selectBits == 0x30 {
		// With neither key group selected the SGB exposes the active controller
		// number on P10-P13. Controller 1 is $F, 2 is $E, 3 is $D, 4 is $C.
		if b.sgb.ControllerMask != 0 {
			result |= 0x0F - b.sgb.CurrentPlayer
		} else {
			result |= 0x0F
		}
		return result
	}

	pressed := uint8(0)
	if selectBits&types.Bit4 == 0 {
		pressed |= b.buttonState >> 4 & 0x0F
	}
	if selectBits&types.Bit5 == 0 {
		pressed |= b.buttonState & 0x0F
	}
	result |= ^pressed & 0x0F
	return result
}

// handleSGBControllerTransition models the SGB's P15 transition latch.
//
// A high->low transition toggles a pending increment, and a later transition
// back high consumes it. This is subtly different from incrementing on every
// low->high edge: intermediate P14/P15 states can cancel the latch, which is
// observable in SameSuite. The selected controller wraps by ANDing with the
// raw MLT_REQ mask.
func (b *Bus) handleSGBControllerTransition(value, previous byte) {
	s := &b.sgb
	bits := value >> 4 & 3
	previousBits := previous >> 4 & 3
	if bits == previousBits {
		return
	}

	if bits&2 != 0 {
		if s.IncrementPending {
			s.IncrementPending = false
			s.CurrentPlayer = (s.CurrentPlayer + 1) & s.ControllerMask
		}
		return
	}

	if previousBits&2 != 0 {
		s.IncrementPending = !s.IncrementPending
	}
}

func (b *Bus) handleSGBPacketWrite(value byte) {
	s := &b.sgb

	packetCount := int(s.Command[0] & 7)
	if packetCount == 0 {
		packetCount = 1
	}
	commandBits := uint16(packetCount * 16 * 8)

	switch (value >> 4) & 3 {
	case 3:
		s.ReadyForPulse = true

	case 2: // command bit 0
		if !s.ReadyForPulse || !s.ReadyForWrite {
			return
		}
		if s.ReadyForStop {
			if s.CommandWriteIndex == commandBits {
				b.runSGBCommand()
				s.CommandWriteIndex = 0
				clear(s.Command[:])
			}
			s.ReadyForPulse = false
			s.ReadyForWrite = false
			s.ReadyForStop = false
			return
		}
		if s.CommandWriteIndex < uint16(len(s.Command)*8) {
			s.CommandWriteIndex++
			s.ReadyForPulse = false
			if s.CommandWriteIndex%(16*8) == 0 {
				s.ReadyForStop = true
			}
		}

	case 1: // command bit 1
		if !s.ReadyForPulse || !s.ReadyForWrite {
			return
		}
		if s.ReadyForStop {
			// A one where the stop bit belongs corrupts the packet.
			s.ReadyForPulse = false
			s.ReadyForWrite = false
			s.CommandWriteIndex = 0
			s.ReadyForStop = false
			clear(s.Command[:])
			return
		}
		if s.CommandWriteIndex < uint16(len(s.Command)*8) {
			s.Command[s.CommandWriteIndex/8] |= 1 << (s.CommandWriteIndex & 7)
			s.CommandWriteIndex++
			s.ReadyForPulse = false
			if s.CommandWriteIndex%(16*8) == 0 {
				s.ReadyForStop = true
			}
		}

	case 0: // packet reset/start
		if !s.ReadyForPulse {
			return
		}
		s.ReadyForWrite = true
		s.ReadyForPulse = false
		if s.CommandWriteIndex%(16*8) != 0 || s.CommandWriteIndex == 0 || s.ReadyForStop {
			s.CommandWriteIndex = 0
			s.ReadyForStop = false
			clear(s.Command[:])
		}
	}
}

func (b *Bus) runSGBCommand() {
	s := &b.sgb
	if s.Command[0]&7 == 0 {
		return
	}

	switch s.Command[0] >> 3 {
	case 0x11: // MLT_REQ
		mask := s.Command[1] & 3
		if mask == 2 {
			// Real SGB hardware increments once before applying the mode-2
			// mask. This oddity is visible as the "glitched player 3" state.
			s.CurrentPlayer++
		}
		s.ControllerMask = mask
		s.CurrentPlayer &= s.ControllerMask
	}
}
