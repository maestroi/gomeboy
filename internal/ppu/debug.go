package ppu

// DebugState is a compact, read-only view of hardware-visible PPU state for
// debuggers and automation. It intentionally excludes FIFOs, object buffers,
// palettes, and the framebuffer so querying it is cheap.
type DebugState struct {
	Mode       uint8
	LY, LX     uint8
	STAT       uint8
	LCDEnabled bool
	BGEnabled  bool
	WinEnabled bool
	ObjEnabled bool
	SCY, SCX   uint8
	WY, WX     uint8
	LYC        uint8
	CGBMode    bool
	Video      bool
}

// DebugState returns a compact copy of the PPU's current state.
func (p *PPU) DebugState() DebugState {
	return DebugState{
		Mode:       p.mode,
		LY:         p.ly,
		LX:         p.lx,
		STAT:       p.status,
		LCDEnabled: p.enabled,
		BGEnabled:  p.bgEnabled,
		WinEnabled: p.winEnabled,
		ObjEnabled: p.objEnabled,
		SCY:        p.scy,
		SCX:        p.scx,
		WY:         p.wy,
		WX:         p.wx,
		LYC:        p.lyCompare,
		CGBMode:    p.cgbMode,
		Video:      p.videoOutput,
	}
}
