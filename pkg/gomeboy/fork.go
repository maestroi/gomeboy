package gomeboy

import "fmt"

// Fork creates an independent emulator at the exact current execution state.
// It reuses the immutable ROM bytes but owns all mutable CPU/PPU/APU/bus and
// cartridge state. The clone preserves headless audio and video-output modes.
// Input logs and flight recorders are intentionally not copied.
func (e *Emulator) Fork() (*Emulator, error) {
	if e == nil || e.gb == nil || len(e.gb.ROM) == 0 {
		return nil, fmt.Errorf("gomeboy: Fork: no ROM loaded")
	}

	state := e.gb.Snapshot()
	opts := []Option{
		WithROMBytes(e.gb.ROM),
		WithModel(e.Model()),
	}
	if state.APU.Headless {
		opts = append(opts, Headless())
	}
	if !e.gb.PPU.VideoOutputEnabled() {
		opts = append(opts, WithoutVideo())
	}

	clone, err := New(opts...)
	if err != nil {
		return nil, err
	}
	clone.gb.Restore(state)
	// videoOutput is a runtime output policy rather than serialized execution
	// state, so Restore intentionally does not carry it; New above does.
	return clone, nil
}
