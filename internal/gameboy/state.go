package gameboy

import (
	"bytes"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"

	"github.com/thelolagemann/gomeboy/internal/apu"
	"github.com/thelolagemann/gomeboy/internal/cpu"
	"github.com/thelolagemann/gomeboy/internal/io"
	"github.com/thelolagemann/gomeboy/internal/ppu"
	"github.com/thelolagemann/gomeboy/internal/scheduler"
	"github.com/thelolagemann/gomeboy/internal/serial"
	"github.com/thelolagemann/gomeboy/internal/timer"
	"github.com/thelolagemann/gomeboy/internal/types"
)

// State is a complete snapshot of the emulator's execution state. It
// composes the state of every component: the CPU, the scheduler, the bus
// (including the cartridge), the PPU, the APU, the timer, and the serial
// controller. Restoring a State returns the emulator to the exact
// deterministic point it was captured at.
type State struct {
	CPU       cpu.State
	Scheduler scheduler.State
	Bus       io.State
	PPU       ppu.State
	APU       apu.State
	Timer     timer.State
	Serial    serial.State
	Model     types.Model
	Frames    uint64
}

// Snapshot captures the emulator's complete execution state.
func (g *GameBoy) Snapshot() State {
	return State{
		CPU:       g.CPU.Snapshot(),
		Scheduler: g.Scheduler.Snapshot(),
		Bus:       g.Bus.Snapshot(),
		PPU:       g.PPU.Snapshot(),
		APU:       g.APU.Snapshot(),
		Timer:     g.Timer.Snapshot(),
		Serial:    g.Serial.Snapshot(),
		Model:     g.model,
		Frames:    g.frames,
	}
}

// Restore rebuilds the emulator's complete execution state from a snapshot.
// The ROM must already be loaded and match the one that was present when the
// snapshot was taken.
func (g *GameBoy) Restore(s State) {
	g.CPU.Restore(s.CPU)
	g.Scheduler.Restore(s.Scheduler)
	g.Bus.Restore(s.Bus)
	g.PPU.Restore(s.PPU)
	g.APU.Restore(s.APU)
	g.Timer.Restore(s.Timer)
	g.Serial.Restore(s.Serial)
	g.model = s.Model
	g.frames = s.Frames
}

// SaveState serializes the emulator's complete execution state into a byte
// slice. The returned bytes can be passed to LoadState (on this or another
// GameBoy running the same ROM) to restore the exact state.
func (g *GameBoy) SaveState() ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(g.Snapshot()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LoadState deserializes a state produced by SaveState and restores the
// emulator to that exact state.
func (g *GameBoy) LoadState(data []byte) error {
	var s State
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&s); err != nil {
		return err
	}
	g.Restore(s)
	return nil
}

// QuickSave writes the complete emulator state to <savedir>/<romname>.state,
// where romname is the loaded ROM's base name without its extension and
// savedir is the directory set by WithSaveDir (the working directory if none
// was set).
func (g *GameBoy) QuickSave() error {
	if g.filename == "" {
		return errors.New("gomeboy: no ROM loaded")
	}
	state, err := g.SaveState()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.saveDir, g.filename+".state"), state, 0644)
}

// QuickLoad restores the emulator state from <savedir>/<romname>.state, where
// romname is the loaded ROM's base name without its extension and savedir is
// the directory set by WithSaveDir (the working directory if none was set).
func (g *GameBoy) QuickLoad() error {
	if g.filename == "" {
		return errors.New("gomeboy: no ROM loaded")
	}
	data, err := os.ReadFile(filepath.Join(g.saveDir, g.filename+".state"))
	if err != nil {
		return err
	}
	return g.LoadState(data)
}
