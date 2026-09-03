package apu

// Muted reports whether generated audio samples are currently silenced.
// Muting affects output only; APU hardware state continues to advance.
func (a *APU) Muted() bool {
	return a.mute
}
