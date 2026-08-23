package ppu

import (
	"github.com/thelolagemann/gomeboy/pkg/utils"
)

// State is a snapshot of the PPU's execution state, including all
// register-mirrored fields, the palette configuration, the prepared
// frame buffer, the pixel fetcher state, and the scanline state.
type State struct {
	// LCDC register
	Enabled, BGEnabled, WinEnabled, ObjEnabled  bool
	BGTileMap, WinTileMap, ObjSize, AddressMode uint8

	// Rendering state
	Mode, ModeToInt, LY, LX, Status uint8

	// Window rendering state
	WinFetcherX, WLY           uint8
	WinTriggerWY, WinTriggerWX bool

	// Scroll discard state
	SCXDiscarded, SCXToDiscard uint8

	// Scroll registers
	SCY, SCX, WY, WX uint8

	// LY comparison state
	LYCompare       uint8
	LYForComparison uint16

	// Interrupt lines
	LYCInt, STATInt bool

	// Palette configuration
	CRAM                     [128]uint8
	ColourPalette            ColourPalette
	ColourOBJPalette         ColourPalette
	BGColourisationPalette   Palette
	OBJ0ColourisationPalette Palette
	OBJ1ColourisationPalette Palette

	// Frame buffers
	PreparedFrame [ScreenHeight][ScreenWidth][3]uint8

	// Pixel slice fetcher
	BGFIFO               utils.FIFOState[FIFOEntry]
	ObjFIFO              utils.FIFOState[FIFOEntry]
	FetcherState         FetcherState
	FetcherTileNo        uint8
	FetcherTileAttr      uint8
	FetcherData          [2]uint8
	FetcherTileNoAddress uint16

	// Object fetcher
	ObjectFetcherState ObjectFetcherState
	ObjFetcherTileNo   uint8
	ObjFetcherTileAttr uint8
	ObjFetcherData     [2]uint8
	FetcherObj         bool
	FetchingObj        Object

	// Internal rendering state
	LineState          LineState
	OffscreenLineState OffscreenLineState
	GlitchedLineState  GlitchedLineState
	ObjBuffer          []Object

	// Timing counters
	LineDot  uint64
	FrameDot uint64

	// CGB-specific features
	CGBMode       bool
	BCPSIndex     uint8
	OCPSIndex     uint8
	BCPSIncrement bool
	OCPSIncrement bool

	// Debug controls
	Debug struct {
		OBJDisabled        bool
		BackgroundDisabled bool
		WindowDisabled     bool
	}
}

// Snapshot captures the PPU's execution state.
func (p *PPU) Snapshot() State {
	st := State{
		Enabled:                  p.enabled,
		BGEnabled:                p.bgEnabled,
		WinEnabled:               p.winEnabled,
		ObjEnabled:               p.objEnabled,
		BGTileMap:                p.bgTileMap,
		WinTileMap:               p.winTileMap,
		ObjSize:                  p.objSize,
		AddressMode:              p.addressMode,
		Mode:                     p.mode,
		ModeToInt:                p.modeToInt,
		LY:                       p.ly,
		LX:                       p.lx,
		Status:                   p.status,
		WinFetcherX:              p.winFetcherX,
		WLY:                      p.wly,
		WinTriggerWY:             p.winTriggerWy,
		WinTriggerWX:             p.winTriggerWx,
		SCXDiscarded:             p.scxDiscarded,
		SCXToDiscard:             p.scxToDiscard,
		SCY:                      p.scy,
		SCX:                      p.scx,
		WY:                       p.wy,
		WX:                       p.wx,
		LYCompare:                p.lyCompare,
		LYForComparison:          p.lyForComparison,
		LYCInt:                   p.lycInt,
		STATInt:                  p.statInt,
		CRAM:                     p.cRAM,
		ColourPalette:            p.ColourPalette,
		ColourOBJPalette:         p.ColourOBJPalette,
		BGColourisationPalette:   p.BGColourisationPalette,
		OBJ0ColourisationPalette: p.OBJ0ColourisationPalette,
		OBJ1ColourisationPalette: p.OBJ1ColourisationPalette,
		PreparedFrame:            p.PreparedFrame,
		BGFIFO:                   p.bgFIFO.Snapshot(),
		ObjFIFO:                  p.objFIFO.Snapshot(),
		FetcherState:             p.fetcherState,
		FetcherTileNo:            p.fetcherTileNo,
		FetcherTileAttr:          p.fetcherTileAttr,
		FetcherData:              p.fetcherData,
		FetcherTileNoAddress:     p.fetcherTileNoAddress,
		ObjectFetcherState:       p.objectFetcherState,
		ObjFetcherTileNo:         p.objFetcherTileNo,
		ObjFetcherTileAttr:       p.objFetcherTileAttr,
		ObjFetcherData:           p.objFetcherData,
		FetcherObj:               p.fetcherObj,
		FetchingObj:              p.fetchingObj,
		LineState:                p.lineState,
		OffscreenLineState:       p.offscreenLineState,
		GlitchedLineState:        p.glitchedLineState,
		ObjBuffer:                append([]Object(nil), p.objBuffer...),
		LineDot:                  p.lineDot,
		FrameDot:                 p.frameDot,
		CGBMode:                  p.cgbMode,
		BCPSIndex:                p.bcpsIndex,
		OCPSIndex:                p.ocpsIndex,
		BCPSIncrement:            p.bcpsIncrement,
		OCPSIncrement:            p.ocpsIncrement,
	}
	st.Debug.OBJDisabled = p.Debug.OBJDisabled
	st.Debug.BackgroundDisabled = p.Debug.BackgroundDisabled
	st.Debug.WindowDisabled = p.Debug.WindowDisabled
	return st
}

// Restore rebuilds the PPU's execution state from a snapshot.
func (p *PPU) Restore(s State) {
	p.enabled = s.Enabled
	p.bgEnabled = s.BGEnabled
	p.winEnabled = s.WinEnabled
	p.objEnabled = s.ObjEnabled
	p.bgTileMap = s.BGTileMap
	p.winTileMap = s.WinTileMap
	p.objSize = s.ObjSize
	p.addressMode = s.AddressMode
	p.mode = s.Mode
	p.modeToInt = s.ModeToInt
	p.ly = s.LY
	p.lx = s.LX
	p.status = s.Status
	p.winFetcherX = s.WinFetcherX
	p.wly = s.WLY
	p.winTriggerWy = s.WinTriggerWY
	p.winTriggerWx = s.WinTriggerWX
	p.scxDiscarded = s.SCXDiscarded
	p.scxToDiscard = s.SCXToDiscard
	p.scy = s.SCY
	p.scx = s.SCX
	p.wy = s.WY
	p.wx = s.WX
	p.lyCompare = s.LYCompare
	p.lyForComparison = s.LYForComparison
	p.lycInt = s.LYCInt
	p.statInt = s.STATInt
	p.cRAM = s.CRAM
	p.ColourPalette = s.ColourPalette
	p.ColourOBJPalette = s.ColourOBJPalette
	p.BGColourisationPalette = s.BGColourisationPalette
	p.OBJ0ColourisationPalette = s.OBJ0ColourisationPalette
	p.OBJ1ColourisationPalette = s.OBJ1ColourisationPalette
	p.PreparedFrame = s.PreparedFrame
	p.bgFIFO.Restore(s.BGFIFO)
	p.objFIFO.Restore(s.ObjFIFO)
	p.fetcherState = s.FetcherState
	p.fetcherTileNo = s.FetcherTileNo
	p.fetcherTileAttr = s.FetcherTileAttr
	p.fetcherData = s.FetcherData
	p.fetcherTileNoAddress = s.FetcherTileNoAddress
	p.objectFetcherState = s.ObjectFetcherState
	p.objFetcherTileNo = s.ObjFetcherTileNo
	p.objFetcherTileAttr = s.ObjFetcherTileAttr
	p.objFetcherData = s.ObjFetcherData
	p.fetcherObj = s.FetcherObj
	p.fetchingObj = s.FetchingObj
	p.lineState = s.LineState
	p.offscreenLineState = s.OffscreenLineState
	p.glitchedLineState = s.GlitchedLineState
	p.objBuffer = append([]Object(nil), s.ObjBuffer...)
	p.lineDot = s.LineDot
	p.frameDot = s.FrameDot
	p.cgbMode = s.CGBMode
	p.bcpsIndex = s.BCPSIndex
	p.ocpsIndex = s.OCPSIndex
	p.bcpsIncrement = s.BCPSIncrement
	p.ocpsIncrement = s.OCPSIncrement
	p.Debug.OBJDisabled = s.Debug.OBJDisabled
	p.Debug.BackgroundDisabled = s.Debug.BackgroundDisabled
	p.Debug.WindowDisabled = s.Debug.WindowDisabled
}
