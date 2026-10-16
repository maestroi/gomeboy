package io

import "errors"

var (
	// ErrRTCUnavailable means this cartridge has no MBC3 clock.
	ErrRTCUnavailable = errors.New("gomeboy: cartridge RTC unavailable")
	// ErrRTCHalted means the cartridge clock is stopped; a clock leg cannot run.
	ErrRTCHalted = errors.New("gomeboy: cartridge RTC halted")
)

// RTCState is a copy of the live MBC3 registers (not the optionally stale
// latched read bank). Reading it never uses the host wall clock.
type RTCState struct {
	Seconds uint8
	Minutes uint8
	Hours   uint8
	Days    uint16
	Halted  bool
	Carry   bool
}

func (c *Cartridge) hasRTC() bool {
	return c != nil && c.Features.RTC && c.RAMSize+10 <= len(c.RAM)
}

// RTCState reports the clock at the emulator's current cycle. It does not
// change the mapper's selected bank or latch. The ordinary cartridge 0->1
// latch operation remains the only way to update the CPU-visible read bank.
func (c *Cartridge) RTCState() (RTCState, error) {
	if !c.hasRTC() {
		return RTCState{}, ErrRTCUnavailable
	}
	c.updateRTC()
	reg := c.RAM[c.RAMSize:]
	return RTCState{
		Seconds: reg[rtcS],
		Minutes: reg[rtcM],
		Hours: reg[rtcH],
		Days: uint16(reg[rtcDL]) | uint16(reg[rtcDH]&1)<<8,
		Halted: reg[rtcDH]&0x40 != 0,
		Carry: reg[rtcDH]&0x80 != 0,
	}, nil
}

// AdvanceRTCSeconds deterministically fast-forwards only the live cartridge
// RTC. CPU cycles, frames, the selected mapper register and the existing
// latched snapshot are untouched. A subsequent CPU latch observes the new
// time. It is intentionally not a wall-clock synchronization method.
func (c *Cartridge) AdvanceRTCSeconds(seconds uint32) error {
	state, err := c.RTCState()
	if err != nil {
		return err
	}
	if state.Halted {
		return ErrRTCHalted
	}
	if seconds == 0 {
		return nil
	}
	reg := c.RAM[c.RAMSize:]
	total := uint64(state.Seconds) + uint64(state.Minutes)*60 +
		uint64(state.Hours)*3600 + uint64(seconds)
	days := uint64(state.Days) + total/86400
	daySeconds := total % 86400
	reg[rtcS] = byte(daySeconds % 60)
	reg[rtcM] = byte((daySeconds / 60) % 60)
	reg[rtcH] = byte(daySeconds / 3600)
	reg[rtcDL] = byte(days % 512)
	high := byte((days / 256) & 1)
	if state.Carry || days >= 512 {
		high |= 0x80
	}
	reg[rtcDH] = high
	return nil
}
