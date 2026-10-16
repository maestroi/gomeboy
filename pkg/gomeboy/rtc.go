package gomeboy

import "github.com/maestroi/gomeboy/internal/io"

// RTCState is the live cartridge RTC, sampled from emulated cycles only.
type RTCState = io.RTCState

var (
	ErrRTCUnavailable = io.ErrRTCUnavailable
	ErrRTCHalted      = io.ErrRTCHalted
)

// RTC returns a copy of the MBC3 clock, or ErrRTCUnavailable for cartridges
// such as Pokemon Red that have no clock.
func (e *Emulator) RTC() (RTCState, error) {
	if e == nil || e.gb == nil || e.gb.Bus == nil {
		return RTCState{}, ErrRTCUnavailable
	}
	return e.gb.Bus.Cartridge().RTCState()
}

// AdvanceRTCSeconds skips emulated cartridge time without executing hours of
// CPU frames. Call only at a controlled gameplay boundary. The new live RTC
// is checkpointed by ordinary SaveState/LoadState and is visible to the game
// the next time it latches the MBC3 registers.
func (e *Emulator) AdvanceRTCSeconds(seconds uint32) error {
	if e == nil || e.gb == nil || e.gb.Bus == nil {
		return ErrRTCUnavailable
	}
	return e.gb.Bus.Cartridge().AdvanceRTCSeconds(seconds)
}
