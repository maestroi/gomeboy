package gameboy

// WithVideoOutput controls whether the PPU materialises RGB pixels. PPU
// simulation remains cycle/timing accurate when output is disabled.
func WithVideoOutput(enabled bool) Opt {
	return func(g *GameBoy) {
		g.PPU.SetVideoOutput(enabled)
	}
}
