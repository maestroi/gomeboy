package gameboy

// ToggleMute toggles audio output and reports the resulting mute state.
// It is intentionally not part of emulator.Controller; frontends may detect
// this optional capability when they want to expose audio controls.
func (g *GameBoy) ToggleMute() bool {
	g.APU.ToggleMute()
	return g.APU.Muted()
}
