// Package cpu implements the ARM7TDMI CPU used by the Game Boy Advance.
//
// It is intentionally independent of the GBA bus and PPU. Instruction fetch
// and memory timing are layered on top so register/ALU/exception behavior can
// be unit-tested without constructing the rest of the machine.
package cpu

import "fmt"

// Mode is an ARM7TDMI processor mode encoded in CPSR bits 0-4.
type Mode uint8

const (
	ModeUser       Mode = 0x10
	ModeFIQ        Mode = 0x11
	ModeIRQ        Mode = 0x12
	ModeSupervisor Mode = 0x13
	ModeAbort      Mode = 0x17
	ModeUndefined  Mode = 0x1b
	ModeSystem     Mode = 0x1f
)

func (m Mode) valid() bool {
	switch m {
	case ModeUser, ModeFIQ, ModeIRQ, ModeSupervisor, ModeAbort, ModeUndefined, ModeSystem:
		return true
	default:
		return false
	}
}

func (m Mode) String() string {
	switch m {
	case ModeUser:
		return "user"
	case ModeFIQ:
		return "fiq"
	case ModeIRQ:
		return "irq"
	case ModeSupervisor:
		return "supervisor"
	case ModeAbort:
		return "abort"
	case ModeUndefined:
		return "undefined"
	case ModeSystem:
		return "system"
	default:
		return fmt.Sprintf("mode(0x%02x)", uint8(m))
	}
}

// PSR is the ARM program status register layout used by CPSR and SPSRs.
type PSR uint32

const (
	FlagNegative   PSR = 1 << 31
	FlagZero       PSR = 1 << 30
	FlagCarry      PSR = 1 << 29
	FlagOverflow   PSR = 1 << 28
	FlagIRQDisable PSR = 1 << 7
	FlagFIQDisable PSR = 1 << 6
	FlagThumb      PSR = 1 << 5

	modeMask PSR = 0x1f
)

func (p PSR) Mode() Mode      { return Mode(p & modeMask) }
func (p PSR) Negative() bool  { return p&FlagNegative != 0 }
func (p PSR) Zero() bool      { return p&FlagZero != 0 }
func (p PSR) Carry() bool     { return p&FlagCarry != 0 }
func (p PSR) Overflow() bool  { return p&FlagOverflow != 0 }
func (p PSR) IRQDisabled() bool { return p&FlagIRQDisable != 0 }
func (p PSR) FIQDisabled() bool { return p&FlagFIQDisable != 0 }
func (p PSR) Thumb() bool     { return p&FlagThumb != 0 }

func (p PSR) withMode(mode Mode) (PSR, error) {
	if !mode.valid() {
		return p, fmt.Errorf("arm7tdmi: invalid processor mode 0x%02x", uint8(mode))
	}
	return (p &^ modeMask) | PSR(mode), nil
}

func (p PSR) withFlag(flag PSR, set bool) PSR {
	if set {
		return p | flag
	}
	return p &^ flag
}
