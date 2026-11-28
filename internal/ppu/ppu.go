//go:generate go run golang.org/x/tools/cmd/stringer -type=GlitchedLineState,LineState,OffscreenLineState,FetcherState,ObjectFetcherState -output=ppu_string.go
package ppu

import (
	"fmt"
	"math/bits"
	"sort"

	"github.com/maestroi/gomeboy/internal/io"
	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
	"github.com/maestroi/gomeboy/pkg/utils"
)

const (
	// ScreenWidth is the width of the screen in pixels.
	ScreenWidth = 160
	// ScreenHeight is the height of the screen in pixels.
	ScreenHeight = 144
)

const (
	// ModeHBlank (Mode 0) - Horizontal Blanking Period
	//
	// 	Duration 87 - 204 dots (variable per line)
	//	- Allows CPU access to VRAM/OAM
	// 	- STAT interrupt available if enabled via STAT.3
	ModeHBlank = iota

	// ModeVBlank (Mode 1) - Vertical Blanking Period
	//
	//	Duration 4560 dots (10 lines)
	//	- Allows full CPU access to VRAM/OAM
	//	- VBlank interrupt available if enabled via IF.0
	//	- STAT interrupt available if enabled via STAT.4 or STAT.5 (bug?)
	//	- Active during LY 144-153
	ModeVBlank

	// ModeOAM (Mode 2) - OAM Scan
	//
	//	Duration: 80 dots (fixed)
	//	- Locks OAM bus
	//	- STAT interrupt available if enabled via STAT.5 (only during mode transition)
	//	- PPU searches OAM for visible sprites
	//	- Occurs at start of each line
	ModeOAM

	// ModeVRAM (Mode 3) - Pixel Transfer
	//
	//	Duration: 172-289 dots (variable depending on objects and window)
	//	- Locks both OAM and VRAM buses
	//	- No STAT interrupts available
	//	- Active during visible pixel rendering
	//	- Pixel FIFOs are continually clocked until LX=168
	ModeVRAM
)

// PPU implements the Game Boy's (P)ixel (P)rocessing (U)nit.
//
// References:
//   - [Pan Docs](https://gbdev.io/pandocs/Graphics.html)
//   - [Hacktix GBEDG](https://hacktix.github.io/GBEDG/ppu/)
//   - [Mooneye test suite](https://github.com/Gekkio/mooneye-test-suite)
type PPU struct {
	// LCDC register
	enabled     bool  // LCDC.7 - LCD Enable
	bgEnabled   bool  // LCDC.0 - BG Display
	winEnabled  bool  // LCDC.5 - Window Enable
	objEnabled  bool  // LCDC.1 - OBJ Enable
	bgTileMap   uint8 // LCDC.3 - BG Tile Map Select (0=9800-9BFF, 1=9C00-9FFF)
	winTileMap  uint8 // LCDC.6 - Window Tile Map Select
	objSize     uint8 // LCDC.2 - OBJ Size (0=8x8, 1=8x16)
	addressMode uint8 // LCDC.4 - BG/Win Tile Data Select (0=8800-97FF, 1=8000-8FFF)

	// Rendering state
	mode      uint8 // Mode reported to STAT register
	modeToInt uint8 // Mode used to raise STAT interrupt
	ly        uint8 // Current line (0-153)
	lx        uint8 // Current dot within line (0-167)
	status    uint8 // Local copy of STAT register

	// Window rendering state
	winFetcherX  uint8 // Window-specific LX position counter
	wly          uint8 // Window line counter
	winTriggerWy bool  // Window Y-position trigger
	winTriggerWx bool  // Window X-position trigger
	// DMG: a same-line WX=6 write suppresses the ordinary open at that
	// column. If LX reaches the open edge while WX is still 6, the line
	// is poisoned so a later WX cannot open. When WX then stays 6 deep
	// into visible Mode 3 before departing (Mealybug LY=6: WX=LY keeps
	// 6 until the trailing WX=80), the LCD/PPU desync fills that
	// scanline with BGP colour 3. Preset WX=6 without a same-line write
	// still opens (AGE window STAT).
	wx6SuppressLine bool
	wx6PoisonedLine bool
	wx6DesyncLine   bool

	// Scroll discard state
	scxDiscarded uint8 // Number of pixels discarded for SCX alignment
	scxToDiscard uint8 // Target number of pixels to discard based on SCX % 8
	// Extra Mode-3 dots while WY is latched and WIN_EN is set. AGE
	// window STAT shows this configuration lengthens Mode 3 even
	// before WX opens the window (SameBoy also refuses to batch).
	windowMode3Stall uint8

	// Scroll registers
	scy, scx uint8 // Background viewport position
	wy, wx   uint8 // Window Position

	// LY comparison state
	lyCompare       uint8  // LYC register value
	lyForComparison uint16 // Effective LY value used for LYC checks

	// Interrupt lines
	lycInt  bool // Current LYC interrupt line
	statInt bool // Current STAT interrupt line

	// Palette configuration
	cRAM                     [128]uint8    // CGB palette RAM (64 BG + 64 OBJ entries)
	ColourPalette            ColourPalette // Active BG Palette data
	ColourOBJPalette         ColourPalette // Active OBJ Palette data
	BGColourisationPalette   Palette       // BG Palette used for DMG->CGB colourisation
	OBJ0ColourisationPalette Palette       // OBJ0 Palette used for DMG->CGB colourisation
	OBJ1ColourisationPalette Palette       // OBJ1 Palette used for DMG->CGB colourisation

	// DMG BGP mid-Mode-3 write pipeline (SameBoy GB_CONFLICT_READ_OR).
	bgp             uint8
	bgpPending      uint8
	bgpPendingFirst uint8
	bgpPendingArmed bool
	bgpSettleExtra  uint8 // extra Mode-3 dots before BGP OR/settle
	// Extra Mode-3 dots after LX=8 before visible shifts. Candidate count
	// is loaded for Mode-2-STAT lines; bgpStallActive makes it engage on
	// the write line. bgpStallSession keeps later lines of the same frame
	// on the early-stall phase after the first Mode-3 BGP poke (Mealybug
	// m3_window_timing) without lengthening win_en (no Mode-3 BGP).
	postOffscreenStall uint8
	bgpStallActive     bool
	bgpStallSession    bool

	// External components
	b *io.Bus
	s *scheduler.Scheduler

	// Frame buffers
	PreparedFrame [ScreenHeight][ScreenWidth][3]uint8
	videoOutput   bool

	// Pixel slice fetcher
	bgFIFO               *utils.FIFO[FIFOEntry] // Background/Window pixel FIFO
	objFIFO              *utils.FIFO[FIFOEntry] // OBJ pixel fifo
	fetcherState         FetcherState           // Current pixel slice fetcher phase
	fetcherTileNo        uint8                  // Current tile index from tile map
	fetcherTileAttr      uint8                  // Current attributes
	fetcherData          [2]uint8               // Tile pattern data (low + high bytes)
	fetcherTileNoAddress uint16                 // VRAM address of current tile map entry
	tileSelectGlitch     bool                   // Pending CGB LCDC.4 1->0 high-bitplane bus conflict

	// Object fetcher
	objectFetcherState ObjectFetcherState // Current object fetcher phase
	objFetcherTileNo   uint8              // Current OBJ tile index from tile map
	objFetcherTileAttr uint8              // Current attributes for OBJ
	objFetcherData     [2]uint8           // OBJ tile pattern data (low + high bytes)
	fetcherObj         bool               // True when fetching an OBJ instead of BG/Window
	fetchingObj        Object             // Active OBJ being fetched
	// Extra Mode-3 bubble after a ready-path sprite fetch when fine
	// SCX is nonzero (AGE sprite STAT). Mooneye's sprite Mode-0 table
	// keeps SCX=0, so it is unaffected.
	objAlignStall uint8

	// Internal rendering state
	lineState          LineState          // Visible line progression
	offscreenLineState OffscreenLineState // VBlank handling
	glitchedLineState  GlitchedLineState  // First-line startup behaviour
	objBuffer          []Object           // Scanline object buffer
	oamScanIndex       uint8              // Next OAM entry inspected during Mode 2
	oamScanYBus        uint8              // Last Y byte observed by the Mode 2 OAM bus
	oamScanXBus        uint8              // Last X byte observed by the Mode 2 OAM bus

	// Timing counters
	lineDot  uint64 // Cycle at which the current line began
	frameDot uint64 // Cycle at which the current frame began

	// First LCD-on scanline: CPU OAM reads become accessible
	// separately from VRAM reads and OAM writes (B/C NCM: one dot,
	// D/E: one dot in native colour and two in compatibility mode). The absolute
	// scheduler cycle is unaffected by subsequent SCX writes.
	startupOAMReadOpenAt uint64

	// Physical CGB B/C and D/E in DMG compatibility mode keep the CPU
	// STAT mode-3 bits for one clock at the SCX 2/6 initial HBlank
	// handover, independent of coarse VRAM/OAM and Mode-0 IRQ phases.
	startupSTATMode3Until uint64

	// CGB DMG-compatibility mode has a separate CPU OAM write handover
	// on the abbreviated first LCD-on line. SCX phases 2/6 leave the
	// write port PPU-owned one dot beyond the coarse HBlank unlock.
	startupOAMWriteOpenAt uint64

	// CGB-specific features
	cgbMode       bool  // CGB mode
	bcpsIndex     uint8 // BG palette index
	ocpsIndex     uint8 // OBJ palette index
	bcpsIncrement bool  // BG palette index auto-increment
	ocpsIncrement bool  // OBJ palette index auto-increment

	// Debug controls
	Debug struct {
		OBJDisabled        bool // Force disable OBJ rendering
		BackgroundDisabled bool // Force disable BG layer
		WindowDisabled     bool // Force disable window layer
	}
}

// A FIFOEntry represents a single pixel entry in a FIFO.
type FIFOEntry struct {
	Color      uint8
	Attributes uint8
	OBJIndex   uint8
	Palette    uint8
}

// Object is used to define the attributes of an object in OAM.
type Object struct {
	x, y  uint8
	attr  uint8
	id    uint8
	index uint8
}

// GobEncode implements encoding/gob's Encoder interface so that Object can be
// serialized as part of a save state.
func (o Object) GobEncode() ([]byte, error) {
	return []byte{o.x, o.y, o.attr, o.id, o.index}, nil
}

// GobDecode implements encoding/gob's Decoder interface.
func (o *Object) GobDecode(data []byte) error {
	if len(data) != 5 {
		return fmt.Errorf("ppu: invalid Object encoding: got %d bytes, want 5", len(data))
	}
	o.x, o.y, o.attr, o.id, o.index = data[0], data[1], data[2], data[3], data[4]
	return nil
}

// New creates and initializes a PPU instance ready to be used.
func New(b *io.Bus, s *scheduler.Scheduler) *PPU {
	p := &PPU{
		b:           b,
		s:           s,
		videoOutput: true,

		bgFIFO:  utils.NewFIFO[FIFOEntry](8),
		objFIFO: utils.NewFIFO[FIFOEntry](8),
	}
	// CPU VRAM reads have a short end-of-transfer access edge distinct
	// from the PPU bus locks and from CPU VRAM writes/OAM arbitration.
	b.RegisterPPUAccessHandlers(p.cpuVRAMReadOpenAtLastPixel, p.cpuOAMReadOpen, p.cpuOAMWriteOpen)

	for pal := 0; pal < 8; pal++ {
		for c := 0; c < 4; c++ {
			p.ColourPalette[pal][c] = [3]uint8{0xff, 0xff, 0xff}
			p.ColourOBJPalette[pal][c] = [3]uint8{0xff, 0xff, 0xff}
		}
	}

	b.ReserveAddress(types.LCDC, func(v byte) byte {
		oldLCDC := b.Get(types.LCDC)
		oldTileSel := oldLCDC&types.Bit4 != 0
		newTileSel := v&types.Bit4 != 0
		if p.cgbMode && p.mode == ModeVRAM && oldTileSel && !newTileSel {
			// CPU writes become visible after the scheduler has advanced the
			// current machine cycle. Carry the CGB TILE_SEL reset conflict into
			// the next high-bitplane fetch, where the tile ID can appear on the
			// data bus instead of VRAM data.
			p.tileSelectGlitch = true
		}
		p.winTileMap = v >> 6 & 1
		// CGB (including DMG-compat) clears the WY latch when WIN_EN
		// falls during HBlank/VBlank. Mode 2 and Mode 3 keep the latch
		// so mid-frame toggles (Mealybug m2_win_en_toggle / mid-Mode-3
		// re-enable) can resume the window. DMG keeps the latch until
		// frame end regardless of mode.
		if p.enabled && p.b.Model().IsCGB() &&
			oldLCDC&types.Bit5 != 0 && v&types.Bit5 == 0 &&
			(p.mode == ModeHBlank || p.mode == ModeVBlank) {
			p.winTriggerWy = false
		}
		p.winEnabled = v&types.Bit5 > 0
		// Re-sample WY when the window is turned back on (CGB may have
		// cleared the latch on the previous disable).
		if p.winEnabled {
			p.checkWindowTriggerWY()
		}
		p.addressMode = 1 &^ (v >> 4 & 1)
		p.bgTileMap = v >> 3 & 1
		p.objSize = 8 + (v & types.Bit2 << 1)
		p.objEnabled = v&types.Bit1 > 0
		p.bgEnabled = v&types.Bit0 > 0

		// is the screen turning off?
		if p.enabled && v&types.Bit7 == 0 {
			p.enabled = false

			// the screen should only be turned off in VBlank
			if p.mode != ModeVBlank {
				// warn user of incorrect behaviour TODO
			}

			// deschedule all PPU events
			for i := scheduler.PPUHandleVisualLine; i <= scheduler.PPUHandleOffscreenLine; i++ {
				p.s.DescheduleEvent(i)
			}

			// when the LCD is off, LY reads 0, and STAT mode reads 0 (HBlank)
			p.b.Set(types.LY, 0)
			p.ly, p.mode = 0, 0
			p.lyForComparison = 0
			p.wly = 255
			p.startupOAMReadOpenAt = 0
			p.startupOAMWriteOpenAt = 0
			p.startupSTATMode3Until = 0

			// OAM & VRAM bus is released
			p.b.Unlock(io.OAM | io.VRAM)
			p.resetFetcher()
		} else if !p.enabled && v&types.Bit7 != 0 {
			p.enabled = true
			p.cgbMode = b.IsGBCCart() && b.IsGBC()

			p.frameDot = p.s.Cycle()

			// handle DMG0 specific timing differences from boot ROM (not accurate)
			// (TODO make generic interface available for all models)
			if p.b.IsBooting() {
				if p.b.Model() == types.DMG0 {
					p.mode, p.modeToInt = ModeVBlank, ModeVBlank
					p.ly = 144
					p.offscreenLineState = VBlankHandleInt
					p.s.ScheduleEvent(scheduler.PPUHandleOffscreenLine, 372)
					return v
				}
			}

			p.glitchedLineState = StartGlitchedLine
			p.statUpdate()

			if p.cgbMode {
				p.handleGlitchedLine0()
			} else {
				p.s.ScheduleEvent(scheduler.PPUHandleGlitchedLine0, 1)
			}
		}

		return v
	})
	b.ReserveAddress(types.STAT, func(v byte) byte {
		//p.b.Debugf("writing stat %08b\n", v)
		if !p.b.IsGBC() {
			oldStat := p.status
			p.status = 0xff
			p.statUpdate()
			p.status = oldStat
		}

		// clear INT bits from stat
		p.status = p.status&0b1000_0111 | v&0b0111_1000
		p.statUpdate()

		return p.cpuVisibleSTAT()
	})
	b.ReserveLazyReader(types.STAT, p.cpuVisibleSTAT)
	b.ReserveAddress(types.SCY, func(v byte) byte {
		p.scy = v
		return v
	})
	b.ReserveAddress(types.SCX, func(v byte) byte {
		p.scx = v
		// Fine SCX stays mutable through the Mode-3 junk/offscreen
		// phase (lx < 8). Mealybug m3_scx_low_3_bits clears SCX in
		// Mode 2 then writes SCX=2 around line-dot 91 (early rows,
		// still in lx < 8) or 95 (late rows, after visible output
		// began). Updating the discard target only while lx < 8
		// accepts the early write and freezes the late one.
		if p.mode == ModeVRAM && p.lx < 8 {
			p.scxToDiscard = v & 7
		}
		return v
	})
	b.ReserveAddress(types.LY, func(v byte) byte {
		if b.IsBooting() {
			p.ly = v
			visibleLY := v
			// DMG0's HLE bootstrap keeps its hidden line pipeline one line
			// ahead of the LY value visible at cartridge entry. Keeping p.ly
			// at the established phase preserves the later boot-HWIO cadence,
			// while whichboot observes the measured LY=145 handoff.
			if b.Model() == types.DMG0 && v == 0x92 {
				visibleLY = 0x91
			}

			// CGB/AGB boot ROMs hand CGB-compatible cartridges off while the
			// LCD is already in VBlank (LY=144). Bus.Boot applies LCDC before
			// LY, so the ordinary LCD-enable path has already scheduled a
			// visible-line event. Re-home the PPU state machine in VBlank when
			// the HLE boot profile writes that hardware-visible LY value.
			if b.IsGBCCart() && v >= 144 && v <= 153 &&
				(b.Model().IsCGB() || b.Model() == types.AGB) {
				p.s.DescheduleEvent(scheduler.PPUHandleVisualLine)
				p.s.DescheduleEvent(scheduler.PPUHandleGlitchedLine0)
				p.s.DescheduleEvent(scheduler.PPUHandleOffscreenLine)
				p.mode, p.modeToInt = ModeVBlank, ModeVBlank
				p.offscreenLineState = StartVBlank
				p.handleOffscreenLine()
			}
			return visibleLY
		}
		return p.b.Get(types.LY)
	})
	b.ReserveAddress(types.LYC, func(v byte) byte {
		p.lyCompare = v
		p.statUpdate()
		return v
	})
	b.ReserveAddress(types.BGP, func(v byte) byte {
		if p.b.IsGBCCart() && p.b.IsGBC() {
			return v
		}
		old := p.bgp
		p.bgp = v
		// Outside Mode 3 the palette is combinational. Mid-Mode-3 uses
		// SameBoy GB_CONFLICT_READ_OR (write-dot keeps old; next dot OR;
		// then settle) for both window-off and windowed lines so Mealybug
		// m3_bgp_change and m3_window_timing share one phase model.
		if p.mode != ModeVRAM {
			p.applyBGP(v)
			p.bgpPendingArmed = false
			return v
		}
		p.bgpPending = v
		p.bgpPendingFirst = v | old
		if p.b.Model().IsCGB() && !p.winEnabled {
			// CGB NCM Mealybug m3_bgp_change: skip OR overlap on first settle.
			p.bgpPendingFirst = v
		}
		p.bgpPendingArmed = true
		p.bgpStallActive = true
		p.bgpStallSession = true
		// CGB-C settles mid-Mode-3 BGP one dot later than D/E
		// (Mealybug m3_window_timing@CGBBC and m3_bgp_change@CGBBC).
		if p.b.Model() == types.CGBBC {
			p.bgpSettleExtra++
		}
		return v
	})
	b.ReserveAddress(types.OBP0, func(v byte) byte {
		if p.b.IsGBCCart() && p.b.IsGBC() {
			return v
		}
		p.ColourOBJPalette[0] = p.OBJ0ColourisationPalette.remap(v)

		return v
	})
	b.ReserveAddress(types.OBP1, func(v byte) byte {
		if p.b.IsGBCCart() && p.b.IsGBC() {
			return v
		}
		p.ColourOBJPalette[1] = p.OBJ1ColourisationPalette.remap(v)

		return v
	})
	b.ReserveAddress(types.WY, func(v byte) byte {
		p.wy = v
		return v
	})
	b.ReserveAddress(types.WX, func(v byte) byte {
		if !p.b.Model().IsCGB() && v == 6 {
			p.wx6SuppressLine = true
		}
		// Poisoned line that kept WX=6 deep into visible Mode 3, then
		// departs (Mealybug LY=6: WX=LY is also 6, so the first departure
		// is the trailing WX=80): LCD/PPU desync fills the scanline with
		// BGP colour 3. Early departures at LX≈8 (WX=LY on LY≠6) must not.
		if !p.b.Model().IsCGB() && p.wx6PoisonedLine && p.wx == 6 && v != 6 && p.lx >= 16 {
			p.wx6DesyncLine = true
			fill := p.ColourPalette[0][3]
			for x := uint8(0); x < ScreenWidth; x++ {
				p.PreparedFrame[p.ly][x] = fill
			}
		}
		p.wx = v
		// Mid-Mode-3 WX writes can arrive after the exact trigger LX; re-check
		// so a late write still opens the window for the remainder of the line.
		if p.mode == ModeVRAM {
			p.checkWindowTriggerWX()
		}
		return v
	})

	b.RegisterBootHandler(func() {
		if b.IsGBC() && !b.IsGBCCart() {
			p.BGColourisationPalette = p.ColourPalette[0]
			p.OBJ0ColourisationPalette = p.ColourOBJPalette[0]
			p.OBJ1ColourisationPalette = p.ColourOBJPalette[1]
		}
	})

	// setup CGB only registers
	b.RegisterGBCHandler(func() {
		b.ReserveAddress(types.BCPS, func(v byte) byte {
			p.bcpsIndex = v & 0x3F
			p.bcpsIncrement = v&types.Bit7 > 0
			return v | types.Bit6
		})
		b.ReserveAddress(types.BCPD, func(v byte) byte {
			if p.b.IsGBCCart() || p.b.IsBooting() {
				if p.mode != ModeVRAM {
					p.cRAM[p.bcpsIndex] = v
					palIndex, colorIndex := p.bcpsIndex>>3&7, p.bcpsIndex&7>>1

					colour := uint16(p.ColourPalette[palIndex][colorIndex][0]>>3) |
						uint16(p.ColourPalette[palIndex][colorIndex][1]>>3)<<5 |
						uint16(p.ColourPalette[palIndex][colorIndex][2]>>3)<<10

					if p.bcpsIndex&1 == 1 {
						colour = (colour & 0x00FF) | uint16(v)<<8
					} else {
						colour = (colour & 0xFF00) | uint16(v)
					}

					p.ColourPalette[palIndex][colorIndex][0] = uint8(colour&0x1f)<<3 | uint8(colour&0x1f)>>2
					p.ColourPalette[palIndex][colorIndex][1] = uint8(colour>>5&0x1f)<<3 | uint8(colour>>5&0x1f)>>2
					p.ColourPalette[palIndex][colorIndex][2] = uint8(colour>>10&0x1f)<<3 | uint8(colour>>10&0x1f)>>2
				}
				if p.bcpsIncrement {
					p.bcpsIndex = (p.bcpsIndex + 1) & 0x3f
					p.b.Set(types.BCPS, p.b.Get(types.BCPS)&0xC0|p.bcpsIndex)
				}

				return v
			}

			return 0xff
		})
		b.ReserveLazyReader(types.BCPD, func() byte {
			if p.mode != ModeVRAM && p.b.IsGBCCart() {
				return p.cRAM[p.bcpsIndex]
			}
			return 0xff
		})
		b.ReserveAddress(types.OCPS, func(v byte) byte {
			p.ocpsIndex = v & 0x3F
			p.ocpsIncrement = v&types.Bit7 > 0
			return v | types.Bit6
		})
		b.ReserveAddress(types.OCPD, func(v byte) byte {
			if p.b.IsGBCCart() || p.b.IsBooting() {
				if p.mode != ModeVRAM {
					p.cRAM[64+p.ocpsIndex] = v
					palIndex, colorIndex := p.ocpsIndex>>3&7, p.ocpsIndex&7>>1

					colour := uint16(p.ColourOBJPalette[palIndex][colorIndex][0]>>3) |
						uint16(p.ColourOBJPalette[palIndex][colorIndex][1]>>3)<<5 |
						uint16(p.ColourOBJPalette[palIndex][colorIndex][2]>>3)<<10

					if p.ocpsIndex&1 == 1 {
						colour = (colour & 0x00FF) | uint16(v)<<8
					} else {
						colour = (colour & 0xFF00) | uint16(v)
					}

					p.ColourOBJPalette[palIndex][colorIndex][0] = uint8(colour&0x1f)<<3 | uint8(colour&0x1f)>>2
					p.ColourOBJPalette[palIndex][colorIndex][1] = uint8(colour>>5&0x1f)<<3 | uint8(colour>>5&0x1f)>>2
					p.ColourOBJPalette[palIndex][colorIndex][2] = uint8(colour>>10&0x1f)<<3 | uint8(colour>>10&0x1f)>>2
				}

				if p.ocpsIncrement {
					p.ocpsIndex = (p.ocpsIndex + 1) & 0x3f
					p.b.Set(types.OCPS, p.b.Get(types.OCPS)&0xC0|p.ocpsIndex)
				}

				return v
			}

			return 0xff
		})
		b.ReserveLazyReader(types.OCPD, func() byte {
			if p.mode != ModeVRAM && p.b.IsGBCCart() {
				return p.cRAM[64+p.ocpsIndex]
			}
			return 0xff
		})
	})

	s.RegisterEvent(scheduler.PPUHandleVisualLine, p.handleVisualLine)
	s.RegisterEvent(scheduler.PPUHandleGlitchedLine0, p.handleGlitchedLine0)
	s.RegisterEvent(scheduler.PPUHandleOffscreenLine, p.handleOffscreenLine)
	return p
}

// GlitchedLineState represents the progression states for handling the peculiar first line
// that occurs immediately after enabling the LCD. This glitched line has different timings and
// behaviour compared to normal lines, requiring special state management to accurately emulate
// the hardware behaviour.
//
// The states model the sequence of events that occur during this glitched line period,
// including OAM/VRAM locking, mode transitions and the shortened line length.
type GlitchedLineState int

const (
	// StartGlitchedLine is the initial state when beginning the glitched first line.
	// Occurs after a 1-dot delay from the PPU being enabled on DMG, and instantly on CGB.
	StartGlitchedLine GlitchedLineState = iota

	// GlitchedLineOAMWBlock represents the PPU acquiring a write lock on the OAM bus shortly
	// before the end of the ModeOAM scanning phase.
	GlitchedLineOAMWBlock

	// GlitchedLineEndOAM marks the end of the glitched ModeOAM scanning phase and transition
	// to ModeVRAM.
	GlitchedLineEndOAM

	// GlitchedLineStartPixelTransfer initiates the beginning of the ModeVRAM pixel transfer phase.
	// The actual pixel transfer processing is skipped, since this first frame will never be displayed.
	GlitchedLineStartPixelTransfer

	// Extra read-bus handover on CGB double-speed startup. Keep this
	// enum at the end so existing snapshot values remain stable.
	GlitchedLineVRAMReadLock
)

// glitchedLineCycles defines the duration (in dots) for each state in the glitched line
// sequence.
//
//   - StartGlitchedLine:				76 dots (completes at 77)
//   - GlitchedLineOAMWBlock:	 		 2 dots (completes at 79)
//   - GlitchedLineEndOAM:		 		 2 dots (CPU VRAM read handover)
//   - GlitchedLineVRAMReadLock:			 3 dots (completes at pixel transfer)
//   - GlitchedLineStartPixelTransfer:	 0 dots (-> enterPixelTransfer)
var glitchedLineCycles = []uint64{
	StartGlitchedLine:     76,
	GlitchedLineOAMWBlock: 2,
	GlitchedLineEndOAM:    2,
}

// handleGlitchedLine0 handles the very first line after turning the LCD on. Behaviour is
// a bit peculiar, with the line itself being cut short by 4 dots, the initial mode presenting
// as ModeHBlank instead of ModeOAM, and the PPU itself failing to acquire the OAM bus.
//
// As a consequence of the PPU failing to acquire a lock on the OAM bus the subsequent OAM
// scanning will not find any objects. This detail is irrelevant to our implementation as
// the LCD will never receive the first frame anyway, but it's still an interesting detail.
//
//   - [mooneye/acceptance/ppu/lcdon_timing]
//   - [mooneye/acceptance/ppu/lcdon_write_timing]
func (p *PPU) handleGlitchedLine0() {
	switch p.glitchedLineState {
	case StartGlitchedLine:
		// DMG models take a dot to turn the PPU on, but CGB models
		// appear to turn on instantly, therefore we update lineDot
		// here rather than in writes to LCDC. TODO verify how&why
		p.lineDot = p.s.Cycle()
	case GlitchedLineOAMWBlock:
		p.b.WLock(io.OAM)
	case GlitchedLineEndOAM:
		p.mode, p.modeToInt = ModeVRAM, ModeVRAM
		p.b.Lock(io.OAM)
		// At the initial Mode-3 edge CGB double speed takes the VRAM write
		// bus first. The CPU read port closes two PPU dots later, while
		// DMG-compatible startup retains its earlier read/write lock.
		p.b.WBlock(io.VRAM, p.s.DoubleSpeed() || !p.cgbMode)
		// Physical CGB VRAM read ownership also applies to a DMG-only
		// cartridge at normal speed: NCM keeps its early write lock but
		// the CPU read port closes at the actual startup pixel transfer.
		// The synthetic double-speed NCM path retains its previous lock:
		// hardware NCM cannot switch to double speed via KEY1.
		ncmDoubleSpeed := !p.cgbMode && p.s.DoubleSpeed()
		p.b.RBlock(io.VRAM, !p.b.Model().IsCGB() || ncmDoubleSpeed)
		p.glitchedLineState = GlitchedLineVRAMReadLock
		p.s.ScheduleEvent(scheduler.PPUHandleGlitchedLine0, 2)
		return
	case GlitchedLineVRAMReadLock:
		// On physical CGB the CPU VRAM read port closes two dots after
		// the early startup Mode-3 transition. Colour double-speed and
		// NCM normal-speed take this path; the synthetic NCM double-speed
		// case already closed the read port at the earlier transition.
		cgbStartupReadEdge := p.b.Model().IsCGB() &&
			(p.cgbMode && p.s.DoubleSpeed() || !p.cgbMode && !p.s.DoubleSpeed())
		if cgbStartupReadEdge {
			p.b.RLock(io.VRAM)
		}
		p.glitchedLineState = GlitchedLineStartPixelTransfer
		p.s.ScheduleEvent(scheduler.PPUHandleGlitchedLine0, 3)
		return
	case GlitchedLineStartPixelTransfer:
		p.b.Lock(io.VRAM)

		// The first frame is not displayed, so skip pixel generation, but
		// keep the SCX discard dots on CGB colour-mode hardware. These
		// still extend the first Mode-3 bus ownership even though its
		// pixels are not rendered. DMG/NCM retain their existing startup
		// timing until their separate AGE traces are corrected.
		startupTransferDots := uint64(168)
		if p.cgbMode {
			scxDiscardDots := uint64(p.scx & 7)
			// On the short LCD-enable scanline, the CPU samples the
			// first HBlank read on different pixel-dot phases at
			// normal and double speed. Keep the FIFO discard phase
			// aligned to that CPU read boundary, without changing
			// ordinary visible scanlines.
			if p.s.DoubleSpeed() {
				if p.scx&1 != 0 {
					scxDiscardDots++
				}
			} else if p.scx&3 == 1 {
				scxDiscardDots--
			}
			startupTransferDots += scxDiscardDots
		} else if p.b.Model().IsCGB() && !p.s.DoubleSpeed() {
			// The NCM PPU retains a DMG pixel pipeline, but its VRAM
			// ownership is on physical CGB hardware. Its abbreviated
			// first line starts the dummy transfer two dots earlier
			// than CGB colour mode and still discards SCX fine pixels.
			startupTransferDots = 166 + uint64(p.scx&7)
		} else if !p.b.Model().IsCGB() {
			// DMG hardware also discards fine-scroll pixels during the
			// abbreviated first scanline. As on normal-speed CGB, the
			// CPU's first HBlank read samples one dot earlier when the
			// fine SCX offset is congruent to 1 modulo four.
			scxDiscardDots := uint64(p.scx & 7)
			if p.scx&3 == 1 {
				scxDiscardDots--
			}
			startupTransferDots += scxDiscardDots
		}
		p.lineState = EnterHBlank
		p.lineDot -= 4
		p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, startupTransferDots)
		return
	}

	p.s.ScheduleEvent(scheduler.PPUHandleGlitchedLine0, glitchedLineCycles[p.glitchedLineState])
	p.glitchedLineState++
}

// LineState represents the progression states of a standard visible line (LY=0-143) on
// the PPU. It attempts to model the ModeOAM, ModeHBlank and ModeVRAM phases with relatively
// accurate timing.
//
// The state machine coordinates memory bus locking, pixel pipeline operation, and synchronization
// with other PPU components. States transition through:
//
//	ModeOAM -> ModeVRAM -> ModeHBlank -> LY++ ; Repeat
//
// Timing values are derived from hardware measurements where possible and validated against
// test ROMs.
type LineState int

const (
	// StartOAMScan initiates ModeOAM search phase. Locks OAM bus access, initializes
	// LY comparison for LYC checks, checks for the WY==LY condition and handles ModeOAM
	// STAT interrupts.
	StartOAMScan LineState = iota

	// ReleaseOAMBus handles the OAM bus being briefly released on dot 76 of ModeOAM search.
	ReleaseOAMBus

	// StartPixelTransfer enter ModeVRAM pixel transfer phase. Initializes FIFOs and
	// manages the initial pipeline priming for visible pixel generation.
	StartPixelTransfer

	// PixelTransferDummy handles initial pipeline setup. Prepares pixel transfer phase
	// for either direct pixel output or SCX alignment.
	PixelTransferDummy

	// PixelTransferLX handles actual visible pixel generation (LX = 8-167). Blends
	// BG and OBJ pixels as necessary.
	PixelTransferLX

	// EnterHBlank marks transition to ModeHBlank. Unlocks memory buses, handles HDMA
	// transfers (CGB), and schedules end of ModeHBlank
	EnterHBlank

	// HBlankUpdateLY increments internal LY register and checks for ModeVBlank transition.
	// Manages STAT mode transitions and ModeOAM preparation for the next line.
	HBlankUpdateLY

	// HBlankUpdateOAM configures OAM bus access for next line.
	HBlankUpdateOAM

	// HBlankUpdateVisibleLY finally updates the memory-mapped LY register (visible to the CPU)
	HBlankUpdateVisibleLY

	// HBlankEnd completes the ModeHBlank period and prepares to handle the next line's
	// ModeOAM search phase.
	HBlankEnd
)

// stateCycles defines the duration (in dots) for each visible line state phase.
//   - StartOAMScan:          		76 dots (completes at 76)
//   - ReleaseOAMBus:          		 4 dots (completes at 80)
//   - StartPixelTransfer:     		 5 dots (completes at 85)
//   - PixelTransferDummy:     		 0 dots (completes at 85)
//   - PixelTransferLX:      172 - 289 dots (variable)
//   - EnterHBlank:  452 - (80 + mode3Dots) (variable)
//   - HBlankUpdateLY:				 1 dots (completes at 452)
//   - HBlankUpdateOAM:				 2 dots (completes at 454)
//   - HBlankUpdateVisibleLY:		 1 dots (completes at 455)
//   - HBlankEnd:					 0 dots (-> enterOAMScan)
var stateCycles = []uint64{
	StartOAMScan:       76,
	ReleaseOAMBus:      4,
	StartPixelTransfer: 5,
	PixelTransferDummy: 1,
	PixelTransferLX:    1,
	// EnterHBlank is variable and is manually scheduled
	HBlankUpdateLY:        1,
	HBlankUpdateOAM:       2,
	HBlankUpdateVisibleLY: 1,
	HBlankEnd:             0,
}

// cpuVisibleSTAT samples the CPU-facing STAT mode bits. Physical CGB
// B/C and D/E have a one-clock mode-3-to-0 read handover at the first
// LCD-enabled HBlank edge for fine-scroll residues 2/6, distinct from
// the VRAM/OAM bus unlock and the STAT interrupt source transition.
func (p *PPU) cpuVisibleSTAT() byte {
	mode := p.mode
	if p.enabled && p.cgbMode && p.b.Model().IsCGB() &&
		!p.s.DoubleSpeed() && mode == ModeOAM &&
		p.lineState == StartOAMScan && p.oamScanIndex == 1 &&
		p.s.Cycle() == p.lineDot {
		// Native normal-speed CGB begins its Mode-2 OAM scan before
		// the CPU-facing STAT port changes. Most lines retain the
		// final Mode-0 clock, but D/E hardware entering LY=0 keeps
		// VBlank's Mode 1 instead: CGB E has no intervening Mode-0
		// machine cycle at the end of Mode 1. OAM bus ownership and
		// the STAT IRQ source have already handed over unchanged.
		mode = ModeHBlank
		if p.b.Model() == types.CGBDE && p.ly == 0 {
			mode = ModeVBlank
		}
	}
	if p.enabled && p.cgbMode && p.b.Model().IsCGB() &&
		!p.s.DoubleSpeed() && mode == ModeVRAM &&
		p.lineState == PixelTransferDummy &&
		p.s.Cycle()-p.lineDot == 80 {
		// The native-colour CGB CPU samples the last Mode-2 read
		// on the clock that begins the PPU's dot-80 pixel transfer.
		// This is an independent read-port phase: the PPU's Mode-3
		// state, VRAM/OAM locks and interrupt source already changed.
		mode = ModeOAM
	}
	if p.enabled && p.cgbMode && p.b.Model().IsCGB() &&
		!p.s.DoubleSpeed() && mode == ModeHBlank &&
		p.lineState == EnterHBlank && p.scx&3 != 3 {
		// Normal-speed physical CGB colour hardware usually retains
		// the CPU-visible Mode 3 after the last buffered pixel until
		// the scheduled HBlank entry. At fine-scroll phases 3 and 7
		// that CPU observation handover occurs at the earlier final-
		// pixel edge instead. The PPU's bus and IRQ timing is shared.
		mode = ModeVRAM
	}
	if p.enabled && p.cgbMode && p.b.Model().IsCGB() &&
		p.s.DoubleSpeed() && mode == ModeVRAM &&
		p.lineState == EnterHBlank && p.lx == 168 && p.scx&1 == 0 &&
		!(p.winTriggerWx && p.wx == 0 && p.scx&7 != 0) {
		// For regular lines, even SCX phases expose Mode 0 as
		// soon as the final pixel is buffered. The shortened first
		// LCD-on line also uses EnterHBlank during its *entire* startup
		// transfer, with lx=0; it must keep returning Mode 3 until
		// HBlank actually begins. Internal mode, bus ownership and
		// STAT IRQ continue to hand over at the scheduled PPU event.
		//
		// WX=0 routes the SCX fine-scroll discard through window
		// pixels. Under double speed that discard shifts the
		// CPU-visible Mode-3→0 edge: even SCX with nonzero fine
		// scroll keeps Mode 3 through enterHBlankDelay (AGE DS
		// window STAT), while fine SCX 0 still exposes Mode 0 at
		// the buffered-pixel edge like an ordinary even phase.
		mode = ModeHBlank
	}
	if p.enabled && p.cgbMode && p.b.Model().IsCGB() &&
		p.s.DoubleSpeed() && mode == ModeHBlank && p.ly == 144 &&
		p.offscreenLineState == VBlankHandleInt &&
		p.s.Cycle()-p.lineDot >= 8 {
		// On double-speed CGB, the CPU STAT read port exposes the
		// start of VBlank at dot 4, one dot before the PPU updates its
		// internal Mode 1 / VBlank IRQ state. Keep the interrupt and
		// internal transition at their independently scheduled edges.
		mode = ModeVBlank
	}
	if p.enabled && p.cgbMode && p.b.Model().IsCGB() &&
		p.s.DoubleSpeed() && mode == ModeVRAM &&
		p.glitchedLineState == GlitchedLineVRAMReadLock &&
		p.s.Until(scheduler.PPUHandleGlitchedLine0) > 2 {
		// The shortened LCD-on scanline enters internal Mode 3 at the
		// glitched OAM handover, but CPU STAT continues to read Mode 0
		// for the first PPU dot (two double-speed CPU clocks). The
		// separate VRAM read/write locks and Mode-3 IRQ timing do not
		// move; this only corrects the CPU-facing STAT register phase.
		mode = ModeHBlank
	}
	if p.enabled && p.b.Model().IsCGB() && !p.cgbMode &&
		!p.s.DoubleSpeed() && p.startupSTATMode3Until != 0 &&
		p.s.Cycle() < p.startupSTATMode3Until {
		mode = ModeVRAM
	}
	return types.Bit7 | p.status | mode
}

// handleVisualLine manages the complete lifecycle of the visible lines (LY = 0-143)
// progressing through ModeOAM, ModeVRAM and ModeHBlank phases with the best effort attempt
// at hardware accurate timings.
//
//   - [mooneye/acceptance/ppu/intr_2_0_timing]
//   - [mooneye/acceptance/ppu/intr_2_mode_0_timing]
//   - [mooneye/acceptance/ppu/intr_2_mode3_timing]
//   - [mooneye/acceptance/ppu/intr_2_oam_ok_timing]
//
// scanOAMEntry performs one of the 40 Mode 2 object checks. Hardware
// advances to a new four-byte OAM entry every two dots; keeping that cadence
// visible to the scheduler is required for OAM DMA overlap tests such as
// Strikethrough.
func (p *PPU) scanOAMEntry() {
	if p.oamScanIndex >= 40 || len(p.objBuffer) >= 10 || p.Debug.OBJDisabled {
		return
	}

	base := uint16(0xfe00) + uint16(p.oamScanIndex)*4
	// Mode 2 has dedicated Y/X bus latches. During ordinary active OAM DMA the
	// PPU cannot refresh them, so each entry is tested using whatever values the
	// previous readable scan left on the bus. This persistence across scanlines is
	// observable in strikethrough.gb.
	if !p.b.PPUOAMScanBlockedByDMA() {
		p.oamScanYBus = p.b.PPUReadOAMScan(base)
		p.oamScanXBus = p.b.PPUReadOAMScan(base + 1)
	}
	y, x := p.oamScanYBus, p.oamScanXBus

	if p.ly+16 < y || p.ly+16 >= y+p.objSize {
		return
	}

	// Mode 2 only selects objects from their Y/X coordinates. Tile number and
	// attributes are sampled later by the Mode 3 object fetcher.
	p.objBuffer = append(p.objBuffer, Object{
		y:     y,
		x:     x,
		index: p.oamScanIndex,
	})
}

// enterHBlankDelay returns the number of PPU dots between exposing Mode 0
// and the HBlank entry edge that raises the Mode-0 STAT interrupt and releases
// the PPU buses. Hardware-backed AGE timing shows one additional dot on
// normal-speed CGB compared with the DMG path. The remaining HBlank duration is
// derived from lineDot, so this does not change the 456-dot scanline length.
func (p *PPU) enterHBlankDelay() uint64 {
	if p.cgbMode && !p.s.DoubleSpeed() {
		return 2
	}
	return 1
}

func (p *PPU) handleVisualLine() {
	switch p.lineState {
	case StartOAMScan:
		// Mode 2 inspects one OAM entry every two dots. The previous
		// implementation took a single snapshot at dot 80, which made DMA
		// overlapping the scan impossible to model.
		if p.oamScanIndex == 0 {
			p.b.Lock(io.OAM)
			p.checkWindowTriggerWY()
			p.objBuffer = p.objBuffer[:0]

			p.lineDot = p.s.Cycle()
			p.lyForComparison = uint16(p.ly)
			p.mode, p.modeToInt = ModeOAM, ModeOAM
			p.statUpdate()
			p.modeToInt = 255
			p.statUpdate()
		}

		p.scanOAMEntry()
		p.oamScanIndex++
		if p.oamScanIndex < 38 {
			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 2)
			return
		}

		// OAM entries 38 and 39 are still searched on dots 76 and 78,
		// while the CPU-facing bus starts its documented late-Mode-2
		// accessibility transition.
		p.lineState = ReleaseOAMBus
		p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 2)
		return
	case ReleaseOAMBus: // dots 76 and 78
		if p.oamScanIndex == 38 {
			// The CPU read port is owned by the physical CGB VRAM bus,
			// even when the cartridge keeps the PPU in DMG compatibility
			// mode. In that case VRAM must stay readable through late
			// Mode 2 until actual pixel transfer starts at dot 80.
			p.b.RBlock(io.VRAM, !p.b.Model().IsCGB())
			p.b.WBlock(io.OAM, p.cgbMode)
			p.b.WUnlock(io.VRAM)
		}

		p.scanOAMEntry()
		p.oamScanIndex++
		if p.oamScanIndex < 40 {
			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 2)
			return
		}

		// Fetch order is left-to-right; stable ordering preserves OAM order
		// for equal X coordinates, which is needed by the priority mixer.
		sort.SliceStable(p.objBuffer, func(i, j int) bool {
			return p.objBuffer[i].x < p.objBuffer[j].x
		})
		p.lineState = StartPixelTransfer
		p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 2)
		return
	case StartPixelTransfer: // on dot 80
		p.resetFetcher()
		p.mode, p.modeToInt = ModeVRAM, ModeVRAM
		p.statUpdate() // clear stat line
		p.b.Lock(io.OAM | io.VRAM)
	case PixelTransferDummy: // on dot 85
		p.bgFIFO.Size = 8

		// the first tile fetch is used to handle SCX % 8 > 0, however this data
		// will just be discarded, so we "fill" the BG FIFO with junk data
		p.scxToDiscard = p.scx & 7
		p.scxDiscarded = 0
		p.windowMode3Stall = 0
		if p.winTriggerWy && p.winEnabled {
			// WY-latched + WIN_EN lengthens Mode 3 even before WX opens
			// (AGE window STAT / SameBoy mode3 batching). Native CGB
			// colour already retains Mode 3 through enterHBlankDelay,
			// so it needs a smaller stall than DMG/NCM.
			if p.cgbMode {
				// Native CGB colour needs one more Mode-3 stall dot than the
				// prior Mealybug-tuned 2 (AGE window STAT 482/512, DS 757/768).
				p.windowMode3Stall = 3
			} else {
				p.windowMode3Stall = 4
			}
		}
		// Candidate BGP alignment stall for Mode-2-IRQ lines (window on or
		// off). Inactive until a Mode-3 BGP write sets bgpStallActive. Do
		// not clear bgpStallActive here: a write between Mode-3 start
		// (dot 80) and Dummy (dot 85) must still engage the stall.
		p.postOffscreenStall = 0
		if p.status&types.Bit5 != 0 {
			p.postOffscreenStall = 11
			if p.ly == 0 {
				p.postOffscreenStall = 7 // Mealybug LY0 handler is 4 cycles shorter
			}
			// WY+WIN_EN already queues windowMode3Stall ahead of this
			// BGP stall; keep the combined pause equal to the Mealybug
			// calibration that predates #267.
			if p.postOffscreenStall > p.windowMode3Stall {
				p.postOffscreenStall -= p.windowMode3Stall
			} else {
				p.postOffscreenStall = 0
			}
		}
		p.lineState = PixelTransferLX
		fallthrough
	case PixelTransferLX:
		p.checkWindowTriggerWX()

		// is the PPU currently fetching an object?
		if p.fetcherObj {
			p.stepObjectFetcher()

			// if still fetching object just wait another dot
			if p.fetcherObj {
				p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)

				// object fetching halts pixel output to the LCD
				return
			}
		}

		// BG-ready-path sprite alignment: burn remaining dots before
		// the next pixel / SCX discard (dmg-timing-spec penalty model).
		if p.objAlignStall > 0 {
			p.objAlignStall--
			p.stepPixelFetcher()
			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)
			return
		}

		// are there any pending objects on this X coordinate?
		if len(p.objBuffer) > 0 && (p.objEnabled || p.cgbMode) && p.objBuffer[0].x == p.lx {
			// do we need to finish the current BG/Window fetch?
			if p.fetcherState < BGWinGetTileDataHighT2 || p.bgFIFO.Size == 0 {
				p.stepPixelFetcher()
			} else {
				// Ready-path Mode-3 bubble when fine SCX is nonzero.
				// Mooneye intr_2_mode0_timing_sprites keeps SCX=0, so
				// this does not disturb its M-cycle table; AGE sprite
				// STAT sets SCX=LY and needs the extra dot on fine
				// phases that currently fall to Mode 0 too early.
				if p.scx&7 != 0 {
					p.objAlignStall = 1
				}
				p.stepPixelFetcher()
				p.fetcherObj = true
				p.objectFetcherState = OBJGetTileNoT1
				p.fetchingObj = p.objBuffer[0]
				p.objBuffer = p.objBuffer[1:]
			}

			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)

			return // object fetching immediately halts pushing pixels to the lcd
		}

		if p.windowMode3Stall > 0 && p.lx >= 8 {
			p.windowMode3Stall--
			p.commitBGPAfterDot()
			// When the window has already opened, WY+WIN_EN Mode-3 stall
			// must not advance the restarted window fetcher. Otherwise
			// WX=7 (open at lx==8) overlaps stall with fetch and Mode 3
			// ends ~windowMode3Stall dots early versus WX>=8 / AGE.
			if !p.winTriggerWx {
				p.stepPixelFetcher()
			}
			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)
			return
		}

		if p.lx >= 8 && p.postOffscreenStall > 0 && (p.bgpStallActive || p.bgpStallSession) {
			p.postOffscreenStall--
			p.commitBGPAfterDot()
			p.stepPixelFetcher()
			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)
			return
		}

		p.pushPixel()
		p.commitBGPAfterDot()
		p.stepPixelFetcher()

		// have we reached the end of the line yet?
		if p.lx != 168 {
			p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)
			return // remain in the PixelTransferLX state until we reach LX==168
		}

		// Native CGB colour window lines keep internal Mode 3 until
		// EnterHBlank so SCX phases 2/6 do not sample a one-dot Mode-0
		// hole after ordinary WX opens (AGE window STAT).
		if !p.s.DoubleSpeed() && !(p.cgbMode && p.winTriggerWx) {
			p.mode, p.modeToInt = ModeHBlank, ModeHBlank
		}
		// DMG: CPU OAM writes reopen when the final pixel has been
		// pushed, one enterHBlankDelay dot before the coarse OAM/VRAM
		// Unlock in EnterHBlank. AGE oam-write-dmgC probe 2/3 Mode-0
		// edge samples depend on that earlier write port.
		if !p.b.Model().IsCGB() {
			p.b.WUnlock(io.OAM)
		}

		p.lineState = EnterHBlank
		p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, p.enterHBlankDelay())
		return
	case EnterHBlank: // variable
		p.flushBGP()
		p.wx6SuppressLine = false
		p.wx6PoisonedLine = false
		p.wx6DesyncLine = false
		p.mode, p.modeToInt = ModeHBlank, ModeHBlank
		p.statUpdate()
		p.b.Unlock(io.OAM | io.VRAM)

		// Physical CGB B/C in DMG compatibility mode exposes one
		// additional CPU STAT mode-3 clock at fine SCX residues 2/6
		// on the abbreviated LCD-on line. Its coarse bus and STAT
		// interrupt boundaries keep their independent timings.
		if p.b.Model().IsCGB() && !p.cgbMode && !p.s.DoubleSpeed() &&
			p.ly == 0 && p.lx == 0 &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer &&
			p.scx&3 == 2 {
			p.startupSTATMode3Until = p.s.Cycle() + 1
		}

		// The physical CGB's DMG-compatible startup write port has
		// its own first-HBlank handover at fine SCX phases 2/6.
		// The CPU can read VRAM and the PPU can unlock its coarse OAM
		// bus while CPU OAM writes remain blocked for one more dot.
		if p.ly == 0 && p.lx == 0 &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer &&
			p.b.Model().IsCGB() && !p.cgbMode && !p.s.DoubleSpeed() &&
			p.scx&3 == 2 {
			p.startupOAMWriteOpenAt = p.s.Cycle() + 1
		}

		// The shortened first LCD-on scanline releases the coarse
		// OAM/VRAM locks before the CPU OAM read port on physical CGB.
		// At normal speed B/C NCM retains that read lock for one
		// additional dot, while D/E retains it for two dots in both
		// native colour and DMG-compatibility mode. Neither delay
		// changes the CPU OAM write or VRAM release phase.
		if p.ly == 0 && p.lx == 0 &&
			p.glitchedLineState == GlitchedLineStartPixelTransfer &&
			!p.s.DoubleSpeed() {
			switch p.b.Model() {
			case types.CGBBC:
				if !p.cgbMode {
					p.startupOAMReadOpenAt = p.s.Cycle() + 1
				}
			case types.CGBDE:
				// Native colour uses a one-dot handover except at
				// fine-scroll phases 1/5, where the CPU read sample
				// is aligned one dot earlier. Compatibility mode
				// retains its independently measured two-dot delay.
				readDelay := uint64(2)
				if p.cgbMode && p.scx&3 != 1 {
					readDelay = 1
				}
				p.startupOAMReadOpenAt = p.s.Cycle() + readDelay
			}
		}

		dotsPassed := p.s.Cycle() - p.lineDot

		if p.s.DoubleSpeed() {
			dotsPassed >>= 1
		}

		// TODO verify timing
		if p.cgbMode {
			p.b.HandleHDMA()
		}

		p.lineState = HBlankUpdateLY
		p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 452-dotsPassed)
		return
	case HBlankUpdateLY: // dot 452
		if p.ly != 143 {
			p.modeToInt = ModeOAM
		}
		p.ly++

		// are we about to enter vblank?
		if p.ly == 144 {
			// the LCD never receives the first frame after turning the PPU on,
			// or rather the LCD doesn't starting clocking any data in until the
			// first vblank has been reached
			// [little-things-gb/first-white]
			if p.s.Cycle()-p.frameDot < 65664 { // shorter frame is probably the glitched line so we're turning on
				p.renderBlank()
			}

			// move to vblank
			p.offscreenLineState = StartVBlank
			p.handleOffscreenLine()
			return
		}
		// we are just continuing OAM -> VRAM -> HBlank loop
		p.lineState = HBlankUpdateOAM
		fallthrough
	case HBlankUpdateOAM: // dot 452 these 4 dots overlap with the beginning of OAM scan (to handle STAT delay)
		p.b.WBlock(io.OAM, p.cgbMode && !p.s.DoubleSpeed())
	case HBlankUpdateVisibleLY: // dot 454
		p.b.Set(types.LY, p.ly)
	case HBlankEnd: // dot 455
		p.b.RBlock(io.OAM, !p.s.DoubleSpeed())
		if p.ly != 0 {
			p.lyForComparison = 0xffff
		} else {
			p.lyForComparison = 0
		}

		// The Mode-2 STAT interrupt source rises before its visible
		// mode bits on ordinary scanlines. At frame rollover (LY=0),
		// physical CGB D/E retains visible Mode 1 through the final
		// HBlank-end handover even with a DMG-only cartridge; there
		// is no extra Mode-0 CPU read cycle before the Mode-2 scan.
		if p.ly != 0 {
			p.modeToInt = ModeOAM
			p.mode = ModeHBlank
		} else if !p.cgbMode &&
			(p.b.Model() != types.CGBDE || p.s.DoubleSpeed()) {
			p.mode = ModeHBlank
		}
		p.statUpdate()

		p.oamScanIndex = 0
		p.lineState = StartOAMScan
		p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, 1)
		return
	}

	p.s.ScheduleEvent(scheduler.PPUHandleVisualLine, stateCycles[p.lineState])
	p.lineState++
}

// OffscreenLineState represents the progression states during vertical blanking
// period (ModeVBlank, LY = 144-153) for the PPU.
//
// The state machine models both standard ModeVBlank lines (144-152) and the peculiar
// line 153 behaviour.
type OffscreenLineState int

const (
	// StartVBlank initiates the vertical blanking period. Disables LY comparison
	// and updates the STAT mode to ModeVBlank.
	StartVBlank OffscreenLineState = iota

	// VBlankUpdateLY writes current LY value to memory-mapped register.
	// Handles CGB-specific OAM stat interrupt timing differences.
	VBlankUpdateLY

	// VBlankUpdateLYC updates LY comparison status and triggers
	// LY=LYC interrupt if enabled.
	VBlankUpdateLYC

	// VBlankHandleInt manages VBlank interrupt at LY=144 as well as handling
	// LY increments and the beginning of line 153.
	VBlankHandleInt

	// StartVBlankLastLine prepares for line 153 handling with special LY
	// comparison reset.
	StartVBlankLastLine

	// Line153LYUpdate updates the LY register to 153
	Line153LYUpdate

	// Line153LY0 resets the LY register to 0 but maintains the LY to compare
	// against being 153 still
	Line153LY0

	// Line153LYC disables the LY comparison and processes STAT updates.
	Line153LYC

	// Line153LYC0 finalizes the LY 153 -> 0 transition during line 153.
	Line153LYC0

	// EndFrame completes frame processing and resets PPU state for
	// new frame.
	EndFrame

	// Line153CGBDENormalLYRelease is a CPU-visible LY register update
	// at dot 9 on CGB D/E normal speed, one dot after the internal
	// line-153 LYC comparison update. Keep the new state at the end
	// so existing snapshot state IDs remain unchanged.
	Line153CGBDENormalLYRelease
)

// offscreenLineCycles defines timing for VBlank period states.
//   - StartVBlank:         2 dots (completes at 2)
//   - VBlankUpdateLY:      2 dots (completes at 4)
//   - VBlankUpdateLYC:     1 dot  (completes at 5)
//   - VBlankHandleInt:   451 dots (completes at 456)
//   - StartVBlankLastLine: 2 dots (completes at 2)
//   - Line153LYUpdate:		4 dots (completes at 6)
//   - Line153LY0: 			2 dots (completes at 8)
//   - Line153LYC:			4 dots (completes at 12)
//   - Line153LYC0:		  444 dots (completes at 456)
//   - EndFrame:			0 dot  (-> enterNewFrame)
//
// These timings maintain the 456 dots/line cadence during VBlank.
var offscreenLineCycles = []uint64{
	StartVBlank:         2,
	VBlankUpdateLY:      2,
	VBlankUpdateLYC:     1,
	VBlankHandleInt:     451,
	StartVBlankLastLine: 2,
	Line153LYUpdate:     4,
	Line153LY0:          2,
	Line153LYC:          4,
	Line153LYC0:         444,
}

// lateLine153LYZero selects the later line-153 LY/LYC phase on CGB.
// The externally visible register changes at dot 6 on B/C normal-speed,
// dot 8 on double-speed CGB, and dot 9 on D/E normal-speed. In the last
// case LYC comparison still updates at dot 8.
//
// The generic CGB profile follows the established production behavior and
// therefore takes the late path only in double speed.
func (p *PPU) lateLine153LYZero() bool {
	if p.b.Model() == types.CGBDE {
		return true
	}
	return p.b.Model().IsCGB() && p.s.DoubleSpeed()
}

func (p *PPU) offscreenStateCycles(state OffscreenLineState) uint64 {
	if !p.lateLine153LYZero() {
		return offscreenLineCycles[state]
	}
	switch state {
	case Line153LYUpdate:
		return 2
	case Line153LY0:
		return 4
	default:
		return offscreenLineCycles[state]
	}
}

// handleOffscreenLine manages the ModeVBlank period (LY = 144 - 153) maintaining
// the 456 dots/line cadence. This also includes handling the erratic behaviour that
// occurs on line 153 in regard to the LY/LYC register and STAT comparison checks.
//
//   - [mooneye/acceptance/ppu/intr_1_2_timing-GS]
//   - [mooneye/acceptance/ppu/vblank_stat_intr-GS]
//   - [mooneye/misc/ppu/vblank_stat_intr-C]
func (p *PPU) handleOffscreenLine() {
	switch p.offscreenLineState {
	case StartVBlank: // dot 0
		p.lineDot = p.s.Cycle()
		p.lyForComparison = 0xffff
		p.bgpStallSession = false
		p.statUpdate()
	case VBlankUpdateLY: // dot 2
		p.b.Set(types.LY, p.ly)
		// CGB models appear to trigger the OAM stat int during
		// vblank 2 dots earlier than DMG models?
		// [mooneye/misc/ppu/vblank_stat_intr-C]
		if p.b.Model() >= types.CGB0 && p.ly == 144 && !p.statInt && p.status&0x20 > 0 {
			p.b.RaiseInterrupt(io.LCDINT)
		}
	case VBlankUpdateLYC: // dot 4
		p.lyForComparison = uint16(p.ly)
		p.statUpdate()
	case VBlankHandleInt: // dot 5
		switch p.ly {
		case 144: // entering vblank
			p.mode, p.modeToInt = ModeVBlank, ModeVBlank
			p.b.RaiseInterrupt(io.VBlankINT)

			// entering vblank also triggers the OAM interrupt
			// [mooneye/acceptance/ppu/vblank_stat_intr-GS.gb]
			if p.b.Model() < types.CGB0 && !p.statInt && p.status&0x20 > 0 {
				p.b.RaiseInterrupt(io.LCDINT)
			}
			p.statUpdate()
		case 152: // leaving vblank
			p.ly++
			p.offscreenLineState = StartVBlankLastLine
			p.s.ScheduleEvent(scheduler.PPUHandleOffscreenLine, 451)
			return
		}

		p.ly++
		p.offscreenLineState = StartVBlank
		p.s.ScheduleEvent(scheduler.PPUHandleOffscreenLine, 451)
		return
	case StartVBlankLastLine: // dot 0
		p.lineDot = p.s.Cycle()
		p.lyForComparison = 0xffff
		p.statUpdate()
	case Line153LYUpdate:
		p.b.Set(types.LY, 153)
	case Line153LY0:
		// B/C normal-speed hardware exposes LY=0 from dot 6. D/E and
		// double-speed CGB retain LY=153 for two more dots.
		if !p.lateLine153LYZero() {
			p.b.Set(types.LY, 0)
		}
		p.lyForComparison = 153
		p.statUpdate()
	case Line153LYC:
		if p.lateLine153LYZero() {
			// The D/E normal-speed CPU observes LY=153 on the dot-8
			// handover. Update its visible register one dot later,
			// while maintaining the established dot-8 LY compare
			// and dot-12 comparison reset schedule.
			if p.b.Model() != types.CGBDE || p.s.DoubleSpeed() {
				p.b.Set(types.LY, 0)
			}
			p.lyForComparison = 153
		} else {
			p.lyForComparison = 0xffff
		}
		p.statUpdate()
		if p.b.Model() == types.CGBDE && !p.s.DoubleSpeed() {
			p.offscreenLineState = Line153CGBDENormalLYRelease
			p.s.ScheduleEvent(scheduler.PPUHandleOffscreenLine, 1)
			return
		}
	case Line153CGBDENormalLYRelease: // dot 9, normal-speed D/E only
		p.b.Set(types.LY, 0)
		p.offscreenLineState = Line153LYC0
		p.s.ScheduleEvent(scheduler.PPUHandleOffscreenLine, 3)
		return
	case Line153LYC0:
		p.lyForComparison = 0
		p.statUpdate()
	case EndFrame:
		p.ly = 0
		p.winTriggerWy = false
		p.wly = 255
		p.frameDot = p.s.Cycle()
		p.lineState = HBlankUpdateOAM
		p.handleVisualLine()
		return
	}

	p.s.ScheduleEvent(scheduler.PPUHandleOffscreenLine, p.offscreenStateCycles(p.offscreenLineState))
	p.offscreenLineState++
}

// applyBGP remaps the DMG background palette into ColourPalette[0].
func (p *PPU) applyBGP(v uint8) {
	p.ColourPalette[0] = p.BGColourisationPalette.remap(v)
}

func (p *PPU) commitBGPAfterDot() {
	if !p.bgpPendingArmed {
		return
	}
	if p.bgpSettleExtra > 0 {
		p.bgpSettleExtra--
		return
	}
	p.applyBGP(p.bgpPendingFirst)
	if p.bgpPendingFirst != p.bgpPending {
		p.bgpPendingFirst = p.bgpPending
		return
	}
	p.bgpPendingArmed = false
}

func (p *PPU) flushBGP() {
	if !p.bgpPendingArmed {
		return
	}
	p.applyBGP(p.bgpPending)
	p.bgpPendingArmed = false
}

// pushPixel attempts to push a pixel out from one of the two FIFOs and into
// the LCD. A few things can prevent the LCD from receiving any data in. The
// BG FIFO must have at least one pixel in it, the PPU cannot be fetching an
// object, and there can't be any pending objects at the current LX position.
// If any of these conditions aren't met then the LCD will not receive any data
// and the LX position will remain the same.
//
// Once all three of these conditions are met, then the BG FIFO will be popped,
// alongside the Object FIFO if it has any pending pixels, and merged according
// to the current mode of operation (DMG vs CGB). If LX < 8 then these pixels
// are simply discarded (but still popped) and the LCD will not receive them.
// Once LX >= 8 then the LCD starts clocking in the data from the FIFOs and LX
// increases as normal.
func (p *PPU) pushPixel() {
	if !p.canPopBG() {
		return
	}

	var color [3]uint8

	var bgPX, objPX *FIFOEntry
	var bgPriority, drawObject bool
	var bgEnabled = true

	bgPX = p.bgFIFO.Pop()
	bgPriority = bgPX.Attributes&types.Bit7 > 0

	// are there any pending object pixels? Keep consuming the FIFO even
	// when video output is disabled: FIFO timing is emulation state.
	if p.objFIFO.Size > 0 {
		objPX = p.objFIFO.Pop()
	}
	// are we adjusting for horizontal scroll?
	if p.scxToDiscard > p.scxDiscarded {
		p.scxDiscarded++
		return
	}

	// are we currently offscreen? (pixels are simply discarded)
	if p.lx < 8 {
		p.lx++
		return
	}

	// Rendering the RGB value is output-only. Agent/search workloads that
	// observe memory can disable it without changing PPU timing, FIFO state,
	// interrupts, or bus locking.
	if !p.videoOutput {
		p.lx++
		return
	}

	if objPX != nil && objPX.Color > 0 && p.objEnabled {
		color = p.ColourOBJPalette[objPX.Palette][objPX.Color]

		drawObject = true
		if objPX.Attributes&types.Bit7 > 0 {
			bgPriority = true
		}
	}

	// BG_EN bit is different on CGB
	if !p.bgEnabled {
		if p.cgbMode {
			bgPriority = false
		} else {
			bgEnabled = false
		}
	}

	if !drawObject || bgPriority && bgPX.Color > 0 {
		// are we drawing a BG pixel?
		if bgEnabled {
			if (p.winTriggerWx && !p.Debug.WindowDisabled) || (!p.winTriggerWx && !p.Debug.BackgroundDisabled) {
				color = p.ColourPalette[bgPX.Attributes&7][bgPX.Color]
			} else {
				// user has disabled rendering through debugging functions
				color = [3]uint8{0xff, 0xff, 0xff} // white TODO customizable disabled color
			}
		} else {
			color = p.ColourPalette[bgPX.Attributes&7][0]
		}
	}

	// draw pixel to frame
	if p.wx6DesyncLine {
		// SameBoy LCD/PPU desync fill after the WX=6 glitch column:
		// remainder of the line is BGP colour 3 (black under $E4).
		color = p.ColourPalette[0][3]
	}
	p.PreparedFrame[p.ly][p.lx-8] = color
	p.lx++
}

// FetcherState represents the internal state machines of the PPU's pixel slice fetcher
// responsible for retrieving tile data from VRAM and converting it into pixel streams.
// It handles rendering the background and window layers, and can be interrupted by
// the object fetching process.
//
// The fetcher progresses through distinct phases for each layer:
//  1. Tile number lookup
//  2. Attribute retrieval (CGB only)
//  3. Tile data fetching (low/high bytes)
//  4. Pushing pixels to FIFO
type FetcherState int

// Background/Window activation states
const (
	BGWinActivating        FetcherState = iota // Initial state selecting BG/Window layer
	BGGetTileNoT1                              // BG: Start tile number lookup (dot 1)
	BGGetTileNoT2                              // BG: Complete tile number lookup (dot 2)
	BGGetTileDataLowT1                         // BG: Start tile data low fetch (dot 1)
	BGGetTileDataLowT2                         // BG: Complete tile data low fetch (dot 2)
	BGGetTileDataHighT1                        // BG: Start tile data high fetch (dot 1)
	BGWinGetTileDataHighT2                     // BG/Window: Complete tile data high fetch and finalize tile data (dot 2)
	BGWinPushPixels                            // Push decoded pixels to BG FIFO
)

// Window-specific states
const (
	WinActivating        = BGWinPushPixels + 1 + iota // Window layer activating
	WinGetTileNoT1                                    // Window: Start tile number lookup (dot 1)
	WinGetTileNoT2                                    // Window: Complete tile number lookup (dot 2)
	WinGetTileDataLowT1                               // Window: Start tile data low fetch (dot 1)
	WinGetTileDataLowT2                               // Window: Complete tile data low fetch (dot 2)
	WinGetTileDataHighT1                              // Window: Start tile data high fetch (dot 1)
)

// stepPixelFetcher advances the pixel slice fetcher forward by 1 step depending
// on the state it's currently in.
func (p *PPU) stepPixelFetcher() {
	// WIN_EN clear mid-window-fetch must not remap window fetcher states
	// onto BG states with -=8: that can pair window low-plane data with a
	// BG high plane and push a corrupt tile (Mealybug win_en 60..64 strip).
	// Restart BG fetch cleanly; FIFO pixels already queued still drain.
	if p.fetcherState >= WinGetTileNoT1 && p.fetcherState <= WinGetTileDataHighT1 && !p.winEnabled {
		p.winTriggerWx = false
		p.fetcherState = BGWinActivating
		p.fetcherTileAttr = 0
		p.fetcherData = [2]uint8{}
	}

	switch p.fetcherState {
	case BGWinActivating:
		// are we fetching for the window or BG?
		if p.winTriggerWx {
			if p.winEnabled {

				// move to WinGetTileNoT1
				p.fetcherState = WinGetTileNoT1
				p.stepPixelFetcher()
				return
			}

			// window fetch was aborted, proceed with bg fetch
			p.winTriggerWx = false
		}

		// we're fetching background
		p.fetcherState = BGGetTileNoT1
		p.stepPixelFetcher()
		return
	// Background
	case BGGetTileNoT1:
		address := uint16(0x1800)
		address |= uint16(p.bgTileMap) << 10

		// we should have already shifted out the necessary pixels for SCX
		// adjustment during the PixelTransferLXSCXDiscard stage, therefore
		// we just need to apply the current SCX to the current LX position
		// [age/m3-bg-scx/m3-bg-scx]:CGB
		// [age/m3-bg-scx/m3-bg-scx-ds]:CGB
		// [age/m3-bg-scx/m3-bg-scx-nocgb]:CGB
		// TODO determine timings that alter the CGB expected result
		x := ((int(p.scx) + int(p.lx)) >> 3) & 0x1f

		// determine x,y position on tilemap
		address |= (uint16((p.ly+p.scy)>>3) & 0x1f) << 5 // Y pos
		address |= uint16(x)                             // X pos
		p.fetcherTileNoAddress = address

		// TODO verify when CGB accesses tile attrs
		if p.cgbMode {
			p.fetcherTileAttr = p.b.GetVRAM(p.fetcherTileNoAddress, 1)
		}
	case BGGetTileDataLowT2:
		p.fetcherData[0] = p.b.GetVRAM(p.getBGTileAddress(), p.fetcherTileAttr&types.Bit3>>3)
	case BGWinGetTileDataHighT2:
		tileNo := p.b.GetVRAM(p.fetcherTileNoAddress, 0)
		if p.cgbMode && p.tileSelectGlitch && tileNo&types.Bit7 == 0 {
			// On CGB hardware, an LCDC.4 1->0 transition racing the high
			// bitplane fetch can put the unsigned tile ID itself on the data bus.
			p.fetcherData[1] = tileNo
		} else {
			p.fetcherData[1] = p.b.GetVRAM(p.getBGTileAddress()|1, p.fetcherTileAttr&types.Bit3>>3)
		}
		p.tileSelectGlitch = false
		if p.fetcherTileAttr&types.Bit5 > 0 {
			p.fetcherData[0] = bits.Reverse8(p.fetcherData[0])
			p.fetcherData[1] = bits.Reverse8(p.fetcherData[1])
		}
		fallthrough
	// TODO handle immediate window push?
	case BGWinPushPixels:
		p.fetcherState = BGWinPushPixels
		// pixels can only be pushed to the BG FIFO when it is empty, otherwise
		// we need to remain in this state until it is
		isEmpty := p.bgFIFO.Size == 0

		if isEmpty {
			// TODO handle LX == WX glitch here?

			attr := p.fetcherTileAttr
			low, high := p.fetcherData[0], p.fetcherData[1]
			p.bgFIFO.Data = [8]FIFOEntry{
				{Attributes: attr, Color: (low>>7)&1 | (high>>7)&1<<1},
				{Attributes: attr, Color: (low>>6)&1 | (high>>6)&1<<1},
				{Attributes: attr, Color: (low>>5)&1 | (high>>5)&1<<1},
				{Attributes: attr, Color: (low>>4)&1 | (high>>4)&1<<1},
				{Attributes: attr, Color: (low>>3)&1 | (high>>3)&1<<1},
				{Attributes: attr, Color: (low>>2)&1 | (high>>2)&1<<1},
				{Attributes: attr, Color: (low>>1)&1 | (high>>1)&1<<1},
				{Attributes: attr, Color: (low)&1 | (high)&1<<1},
			}
			p.bgFIFO.Size = 8

			p.fetcherState = BGWinActivating
		}

		return
	// Window
	case WinActivating:
		p.fetcherState = BGWinActivating
		return
	case WinGetTileNoT1:
		address := uint16(0x1800)

		address |= uint16(p.winTileMap) << 10   // Tilemap
		address |= uint16((p.wly>>3)&0x1f) << 5 // Y pos
		address |= uint16(p.winFetcherX)        // X pos

		// window has its own internal lx counter
		p.winFetcherX++

		p.fetcherTileNoAddress = address

		// TODO verify when CGB accesses tile attrs
		if p.cgbMode {
			p.fetcherTileAttr = p.b.GetVRAM(p.fetcherTileNoAddress, 1)
		}
	case WinGetTileDataLowT1:
		p.fetcherData[0] = p.b.GetVRAM(p.getWinTileAddress(), p.fetcherTileAttr&types.Bit3>>3)
	case WinGetTileDataHighT1:
		p.fetcherData[1] = p.b.GetVRAM(p.getWinTileAddress()|1, p.fetcherTileAttr&types.Bit3>>3)
		if p.fetcherTileAttr&types.Bit5 > 0 {
			p.fetcherData[0] = bits.Reverse8(p.fetcherData[0])
			p.fetcherData[1] = bits.Reverse8(p.fetcherData[1])
		}

		// the push stage is the same as the BG so we move to that state
		p.fetcherState = BGWinPushPixels
		return
	}

	p.fetcherState++
}

// ObjectFetcherState represents the internal state machine of the PPU's object
// fetching pipeline. It handles rendering the object layer and has a few key
// distinctions from the background/window pixel slice fetcher.
//
// One of the main differences is that the object fetcher will simply read the tile
// ID from the object buffer, rather than reading it from the tile map. Alongside the
// ID, the object fetcher also grabs the object attributes from the object buffer,
// whereas only the CGB has tile attributes available to the BG/Window fetcher.
//
// Whilst the BG/Window pixel slice fetcher will wait until the BG FIFO is empty
// in order to start pushing new pixels, the Object fetcher will simply merge
// existing pixels with incoming. The rules for which pixels are kept during this
// merging stage are explained further at the relevant steps. As there are only 6
// steps to the object fetching process, the object fetch will always take 6 dots
// to complete.
//
// An object fetch may interrupt a BG/Window fetch as it's halfway through fetching.
// When this occurs, the object fetcher waits until the BG/Window fetching process is
// complete and then starts the object fetch. So even though object fetching will always
// take 6 dots, the duration of it may appear to have taken longer as it's waiting upon
// a BG/Window fetch to finish.
type ObjectFetcherState int

// Object (sprite) rendering states
const (
	OBJGetTileNoT1       ObjectFetcherState = iota // OBJ: Start obj tile number lookup (dot 1)
	OBJGetTileNoT2                                 // OBJ: Complete obj tile number lookup (or get attr?) (dot 2)
	OBJGetTileDataLowT1                            // OBJ: Start tile data low fetch (dot 1)
	OBJGetTileDataLowT2                            // OBJ: Complete tile data low fetch (dot 2)
	OBJGetTileDataHighT1                           // OBJ: Start tile data high fetch (dot 1)
	OBJGetTileDataHighT2                           // OBJ: Complete tile data high fetch (dot 2)
)

// stepObjectFetcher advances the object fetcher forward by 1 step depending on the
// state it's currently in.
func (p *PPU) stepObjectFetcher() {
	switch p.objectFetcherState {
	case OBJGetTileNoT1:
		// The first OBJ fetch dot advances the interrupted BG/window fetcher;
		// OAM's tile/attribute word is sampled on the following fetch phase.
		p.stepPixelFetcher()
	case OBJGetTileNoT2:
		base := uint16(0xfe00) + uint16(p.fetchingObj.index)*4
		p.objFetcherTileNo = p.b.PPUReadOAMFetch(base + 2)
		p.objFetcherTileAttr = p.b.PPUReadOAMFetch(base + 3)
		p.fetchingObj.id = p.objFetcherTileNo
		p.fetchingObj.attr = p.objFetcherTileAttr
		p.stepPixelFetcher()
	case OBJGetTileDataLowT2:
		p.objFetcherData[0] = p.b.GetVRAM(p.getObjectTileAddress(), p.objFetcherTileAttr&types.Bit3>>3)
	case OBJGetTileDataHighT2:
		p.objFetcherData[1] = p.b.GetVRAM(p.getObjectTileAddress()|1, p.objFetcherTileAttr&types.Bit3>>3)
		if p.objFetcherTileAttr&types.Bit5 > 0 {
			p.objFetcherData[0] = bits.Reverse8(p.objFetcherData[0])
			p.objFetcherData[1] = bits.Reverse8(p.objFetcherData[1])
		}
		j := uint8(0)
		for i := uint8(0x80); i > 0; i >>= 1 {
			objFIFO := FIFOEntry{
				Attributes: p.fetchingObj.attr,
				Palette:    p.fetchingObj.attr & types.Bit4 >> 4,
				OBJIndex:   p.fetchingObj.index,
			}
			if p.cgbMode {
				objFIFO.Palette = p.fetchingObj.attr & 7
			}
			objFIFO.Color = ((p.objFetcherData[0] & i) >> (7 - j)) | ((p.objFetcherData[1]&i)>>(7-j))<<1

			// on DMG objects with the lowest x wins priority, as we already stable sort by x at the end
			// of the oam phase then we only need to check to see if the existing pixel has a colour ID of
			// 0 (=transparent)
			// on CGB (in CGB mode) objects inserted first into the OAM wins priority, so we have to compare
			// the existing pixels insertion index with the new one
			// in all cases an object pixel with a colour id of 0 will be overwritten
			if int(j) < p.objFIFO.Size {
				if p.objFIFO.GetIndex(int(j)).Color == 0 ||
					p.cgbMode &&
						objFIFO.Color != 0 && p.objFIFO.GetIndex(int(j)).OBJIndex > p.fetchingObj.index {
					p.objFIFO.ReplaceIndex(int(j), objFIFO)
				}
			} else {
				p.objFIFO.Push(objFIFO)
			}

			j++

		}

		// are there any more pending OBJ at this LX?
		if p.canFetchOBJ() {
			p.fetchingObj = p.objBuffer[0]
			p.objBuffer = p.objBuffer[1:]
			p.objectFetcherState = OBJGetTileNoT1
		} else {
			p.fetcherObj = false
		}
		return
	}

	p.objectFetcherState++
}

// getBGTileAddress determines the address to read from VRAM for the tile
// that is currently loaded in the pixel fetcher.
func (p *PPU) getBGTileAddress() uint16 {
	// read tile number from VRAM
	tileNo := p.b.GetVRAM(p.fetcherTileNoAddress, 0)

	yPos := (p.ly + p.scy) & 7
	if p.fetcherTileAttr&types.Bit6 > 0 {
		yPos = ^yPos & 7
	}

	// determine where the tile is in VRAM
	address := uint16(0x0000)
	address |= uint16(tileNo) << 4                      // Tile ID offset
	address |= uint16(p.addressMode&^(tileNo>>7)) << 12 // Negation of ID.7 when LCDC.4 is set
	address |= uint16(yPos) << 1                        // Y pos

	return address
}

// getObjectTileAddress determines the address to read from VRAM for the OBJ tile
// that is currently loaded in the pixel fetcher.
func (p *PPU) getObjectTileAddress() uint16 {
	// handle obj size
	tileY := p.fetchingObj.y - 16
	tileY = (p.ly - tileY) & (p.objSize - 1)

	// handle y pos
	if p.objFetcherTileAttr&types.Bit6 > 0 {
		tileY = ^tileY & (p.objSize - 1)
	}

	// determine where the tile is in VRAM. In 8x16 mode the hardware
	// ignores tile-number bit 0; tileY (0..15) naturally crosses into the
	// second 16-byte tile.
	tileNo := p.objFetcherTileNo
	if p.objSize == 16 {
		tileNo &^= 1
	}
	address := uint16(0x0000)
	address |= uint16(tileNo) << 4 // Tile ID offset
	address |= uint16(tileY) << 1  // Y pos

	return address
}

// getWinTileAddress determines the address to read from VRAM for the tile
// that is currently loaded in the pixel fetcher.
func (p *PPU) getWinTileAddress() uint16 {
	// read tile number from vram
	tileNo := p.b.GetVRAM(p.fetcherTileNoAddress, 0)

	yPos := p.wly & 7
	if p.fetcherTileAttr&types.Bit6 > 0 {
		yPos = ^yPos & 7
	}

	// determine where the tile is in VRAM
	address := uint16(0x0000)
	address |= uint16(tileNo) << 4                      // Tile ID offset
	address |= uint16(p.addressMode&^(tileNo>>7)) << 12 // Negation of ID.7 when LCDC.4 is set
	address |= uint16(yPos) << 1                        // Y pos

	return address
}

// statUpdate handles updating the STAT interrupt. As the conditions for
// raising a STAT interrupt are checked every dot, we need to call this
// whenever one of the dependent conditions changes as it would be
// too expensive to use the scheduler.
//
// The OAM STAT interrupt is an exception to this rule, it is only raised
// as a consequence of the HBlank -> OAM transition, and the bugged VBlank
// trigger. Thus writing to STAT's OAM interrupt flag whilst already in
// ModeOAM (or ModeVBlank) does not raise a request.
func (p *PPU) statUpdate() {
	if !p.enabled {
		// STAT & LYC call this but the PPU may be disabled when doing so
		// in which case the STAT line isn't processed
		return
	}

	// update LYC_EQ_LY flag
	if p.lyForComparison != 0xffff || (p.b.Model() <= types.DMGABC || p.b.Model().IsCGB()) && !p.s.DoubleSpeed() {
		// 0xffff marks a temporarily disabled LY comparison during
		// scanline / VBlank handovers. Never truncate that sentinel
		// to 0xff: if LYC is also 0xff, it would falsely assert STAT's
		// visible coincidence bit even though no LY match occurred.
		if p.lyForComparison != 0xffff && uint8(p.lyForComparison) == p.lyCompare {
			p.lycInt = true
			p.status |= types.Bit2
		} else {
			if p.lyForComparison != 0xffff {
				p.lycInt = false
			}
			p.status &^= types.Bit2
		}
	}

	// handle the STAT mode interrupt
	statINT := (p.modeToInt == ModeHBlank && p.status&types.Bit3 != 0) ||
		(p.modeToInt == ModeVBlank && p.status&types.Bit4 != 0) ||
		(p.modeToInt == ModeOAM && p.status&types.Bit5 != 0) ||
		(p.lycInt && p.status&types.Bit6 != 0)

	// did STAT go low -> high
	if !p.statInt && statINT {
		p.b.RaiseInterrupt(io.LCDINT)
	}

	p.statInt = statINT
}

// canFetchOBJ determines whether an OBJ should be fetched.
// There are only two conditions that must be met in order to
// initiate an OBJ fetch.
//   - Pending OBJ at the current LX
//   - OBJ enabled in LCDC
func (p *PPU) canFetchOBJ() bool {
	return len(p.objBuffer) > 0 && p.objBuffer[0].x == p.lx && (p.objEnabled || p.cgbMode)
}

// canPopBG determines whether a pixel can be popped from the
// BG FIFO. There are three conditions that must be met in order
// to be able to pop a pixel.
//   - BG FIFO isn't empty
//   - Fetcher isn't fetching an OBJ
//   - There are no more pending OBJs for this LX or OBJ are disabled
func (p *PPU) canPopBG() bool {
	return p.bgFIFO.Size > 0 &&
		!p.fetcherObj &&
		!((p.objEnabled || p.cgbMode) && len(p.objBuffer) > 0 && p.objBuffer[0].x == p.lx)
}

// checkWindowTriggerWY samples the window's Y condition at the start
// of each scanline, not during pixel output. Once WY equals LY, the latch
// remains set even if WY later changes, until frame reset or a physical
// CGB window-disable transition clears it.
//
// The Y comparator operates independently of LCDC's window-enable bit:
// that bit gates the later X/pixel-fetch handover, so enabling the window
// after the matching scanline can still display it.
func (p *PPU) checkWindowTriggerWY() {
	if p.wy == p.ly {
		p.winTriggerWy = true
	}
}

// checkWindowTriggerWX checks to see if the window should be triggered
// for the current LX position.
func (p *PPU) checkWindowTriggerWX() {
	if !p.winTriggerWy || p.winTriggerWx || !p.winEnabled {
		return
	}

	// WX=0 switches to the window before the initial SCX fine-scroll
	// discard. Keep scxToDiscard intact: those discarded window pixels
	// are what shift the visible window left at this special position.
	// Ordinary WX values compare against LX each Mode-3 dot. Using >=
	// rather than == lets a late mid-Mode-3 WX write still open the
	// window once the scan position has reached WX (Mealybug path).
	// On physical CGB hardware WX=166 still starts a window fetch at the
	// final on-screen pixel (LX=167). The tile-fetch pause therefore
	// contributes to Mode 3 even though only one window pixel is shown.
	// Monochrome hardware has a distinct WX=166 whole-line glitch and
	// must not be treated as this ordinary CGB handover.
	if p.wx == 0 {
		// Mealybug Mode-2 IRQ may program WX=0 after LX has left 0 but
		// still before the first visible column; own the offscreen phase.
		if p.lx >= 8 {
			return
		}
	} else {
		if p.wx6PoisonedLine {
			return
		}
		windowX := p.lx > 0 && p.lx >= p.wx+1
		if p.wx6SuppressLine && p.wx == 6 && windowX {
			// Glitch column reached while WX is still 6 after a same-line
			// write: do not open, and ignore later WX opens this line.
			p.wx6PoisonedLine = true
			return
		}
		if !(windowX && (p.wx < 166 || p.wx == 166 && p.b.Model().IsCGB())) {
			return
		}
	}

	p.winTriggerWx = true

	// Increase the internal window line counter once at the handover.
	p.wly++

	// Restart tile fetching from the window map.
	p.winFetcherX = 0
	p.bgFIFO.Reset()
	// WX=0 keeps the one-dot WinActivating pause (AGE / kevtris: WX=0
	// Mode 3 is one dot longer than ordinary window opens). Ordinary WX
	// values restart at BGWinActivating so Mode 3 matches AGE. Mealybug
	// m3_window_timing writes BGP mid-Mode-3 before the window opens and
	// needs the activating idle to keep the stair-step BGP edge; detect
	// that via the BGP stall session armed by those writes.
	if p.wx == 0 || p.bgpStallActive || p.bgpStallSession {
		p.fetcherState = WinActivating
	} else {
		p.fetcherState = BGWinActivating
	}

	// Window tile-fetch already pauses Mode 3. Retune the BGP
	// post-offscreen stall so the first visible window pixel lines up
	// with mid-Mode-3 BGP writes (Mealybug m3_window_timing):
	//
	//   - Early window (LX<8): trim one BGP-stall dot.
	//   - LY0 WX=0: absolute 6-dot BGP pause (minus pending WY+WIN_EN stall).
	//   - WX=7 (LX==8): BGP stall + window fetch = 17, minus WY+WIN_EN stall.
	//   - Exact WX>7 boundary (LX==WX+1): one extra BGP settle dot.
	//     Mid-scanline WIN_EN re-opens (LX already past WX) skip this.
	switch {
	case p.lx < 8:
		if p.wx == 0 && p.ly == 0 {
			if p.windowMode3Stall < 6 {
				p.postOffscreenStall = 6 - p.windowMode3Stall
			} else {
				p.postOffscreenStall = 0
			}
		} else if p.postOffscreenStall > 0 {
			p.postOffscreenStall--
		}
	case p.lx == 8:
		p.postOffscreenStall = 17 - p.windowMode3Stall
	case p.lx == p.wx+1:
		p.bgpSettleExtra = 1
	}
}

// renderBlank blanks the current screen.
func (p *PPU) renderBlank() {
	for y := uint8(0); y < ScreenHeight; y++ {
		for x := uint8(0); x < ScreenWidth; x++ {
			p.PreparedFrame[y][x] = p.ColourPalette[0][0]
		}
	}
}

// resetFetcher resets the pixel slice fetcher and the two pixel
// FIFOs so that they can be used for the next line.
func (p *PPU) resetFetcher() {
	p.bgFIFO.Reset()
	p.objFIFO.Reset()

	p.winFetcherX = 0
	p.winTriggerWx = false
	p.windowMode3Stall = 0
	p.objAlignStall = 0
	// Keep wx6SuppressLine across Mode-3 start: a Mode-2 WX=6 write on
	// this same scanline must still suppress. Clear at HBlank instead.

	p.fetcherState = BGWinActivating
	p.tileSelectGlitch = false
	p.lx = 0
	p.postOffscreenStall = 0
	p.bgpStallActive = false
	p.bgpSettleExtra = 0
}
