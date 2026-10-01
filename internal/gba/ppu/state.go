package ppu

// State captures hardware-visible PPU registers, timing, affine progression,
// and the partially rendered framebuffer. Transient OBJ scanline caches are
// intentionally rebuilt after restore.
type State struct {
	DISPCNT  uint16
	DISPSTAT uint16
	VCount   uint16

	BGCNT  [4]uint16
	BGHOFS [4]uint16
	BGVOFS [4]uint16

	AffineParam   [2][4]int16
	AffineRefRaw  [2][2]uint32
	AffineCurrent [2][2]int32

	WinH   [2]uint16
	WinV   [2]uint16
	WinIn  uint16
	WinOut uint16

	BLDCNT   uint16
	BLDALPHA uint16
	BLDY     uint16
	Mosaic   uint16

	LineCycle uint32
	HBlank    bool
	VBlank    bool
	VCounter  bool
	Frames    uint64
	Frame     [FrameBytes]byte
}

func (p *PPU) Snapshot() State {
	return State{
		DISPCNT: p.dispcnt, DISPSTAT: p.dispstat, VCount: p.vcount,
		BGCNT: p.bgcnt, BGHOFS: p.bghofs, BGVOFS: p.bgvofs,
		AffineParam: p.affineParam, AffineRefRaw: p.affineRefRaw,
		AffineCurrent: p.affineCurrent,
		WinH: p.winH, WinV: p.winV, WinIn: p.winIn, WinOut: p.winOut,
		BLDCNT: p.bldcnt, BLDALPHA: p.bldalpha, BLDY: p.bldy, Mosaic: p.mosaic,
		LineCycle: p.lineCycle, HBlank: p.hblank, VBlank: p.vblank,
		VCounter: p.vcounter, Frames: p.frames, Frame: p.frame,
	}
}

func (p *PPU) Restore(s State) {
	p.dispcnt = s.DISPCNT
	p.dispstat = s.DISPSTAT
	p.vcount = s.VCount
	p.bgcnt = s.BGCNT
	p.bghofs = s.BGHOFS
	p.bgvofs = s.BGVOFS
	p.affineParam = s.AffineParam
	p.affineRefRaw = s.AffineRefRaw
	p.affineCurrent = s.AffineCurrent
	p.winH = s.WinH
	p.winV = s.WinV
	p.winIn = s.WinIn
	p.winOut = s.WinOut
	p.bldcnt = s.BLDCNT
	p.bldalpha = s.BLDALPHA
	p.bldy = s.BLDY
	p.mosaic = s.Mosaic
	p.lineCycle = s.LineCycle
	p.hblank = s.HBlank
	p.vblank = s.VBlank
	p.vcounter = s.VCounter
	p.frames = s.Frames
	p.frame = s.Frame

	p.objLineY = 0
	p.objLineMode = 0
	p.objLineValid = false
	p.objWindowLine = [ScreenWidth]bool{}

	switch p.dispcnt & 7 {
	case 3, 4, 5:
		p.bus.SetOBJVRAMStart(0x14000)
	default:
		p.bus.SetOBJVRAMStart(0x10000)
	}
}
