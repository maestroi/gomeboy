from pathlib import Path
import re


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    s = p.read_text()
    count = s.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one occurrence, found {count}: {old[:80]!r}")
    p.write_text(s.replace(old, new, 1))


ppu_path = Path("internal/ppu/ppu.go")
ppu = ppu_path.read_text()
old = "\t// Frame buffers\n\tPreparedFrame [ScreenHeight][ScreenWidth][3]uint8\n"
new = "\t// Frame buffers\n\tvideoOutput   bool\n\tPreparedFrame [ScreenHeight][ScreenWidth][3]uint8\n"
if ppu.count(old) != 1:
    raise SystemExit("ppu.go: frame buffer field anchor not found exactly once")
ppu = ppu.replace(old, new, 1)

old = "\t\tbgFIFO:  utils.NewFIFO[FIFOEntry](8),\n\t\tobjFIFO: utils.NewFIFO[FIFOEntry](8),\n"
new = "\t\tbgFIFO:      utils.NewFIFO[FIFOEntry](8),\n\t\tobjFIFO:     utils.NewFIFO[FIFOEntry](8),\n\t\tvideoOutput: true,\n"
if ppu.count(old) != 1:
    raise SystemExit("ppu.go: New() FIFO anchor not found exactly once")
ppu = ppu.replace(old, new, 1)

new_push = '''func (p *PPU) pushPixel() {
\tif !p.canPopBG() {
\t\treturn
\t}

\t// FIFO consumption and LX progression are hardware-visible simulation state,
\t// so they always happen even when RGB output is disabled.
\tbgPX := p.bgFIFO.Pop()
\tvar objPX *FIFOEntry
\tif p.objFIFO.Size > 0 {
\t\tobjPX = p.objFIFO.Pop()
\t}

\t// are we adjusting for horizontal scroll?
\tif p.scxToDiscard > p.scxDiscarded {
\t\tp.scxDiscarded++
\t\treturn
\t}

\t// are we currently offscreen? (pixels are simply discarded)
\tif p.lx < 8 {
\t\tp.lx++
\t\treturn
\t}

\t// Agent/test workloads often need exact PPU timing and memory side effects
\t// without materialising an RGB framebuffer. Skip only colour composition and
\t// the framebuffer write; all fetch/FIFO/timing state above remains exact.
\tif !p.videoOutput {
\t\tp.lx++
\t\treturn
\t}

\tvar color [3]uint8
\tbgPriority := bgPX.Attributes&types.Bit7 > 0
\tdrawObject := false
\tbgEnabled := true

\tif objPX != nil && objPX.Color > 0 && p.objEnabled {
\t\tcolor = p.ColourOBJPalette[objPX.Palette][objPX.Color]
\t\tdrawObject = true
\t\tif objPX.Attributes&types.Bit7 > 0 {
\t\t\tbgPriority = true
\t\t}
\t}

\t// BG_EN bit is different on CGB
\tif !p.bgEnabled {
\t\tif p.cgbMode {
\t\t\tbgPriority = false
\t\t} else {
\t\t\tbgEnabled = false
\t\t}
\t}

\tif !drawObject || bgPriority && bgPX.Color > 0 {
\t\tif bgEnabled {
\t\t\tif (p.winTriggerWx && !p.Debug.WindowDisabled) || (!p.winTriggerWx && !p.Debug.BackgroundDisabled) {
\t\t\t\tcolor = p.ColourPalette[bgPX.Attributes&7][bgPX.Color]
\t\t\t} else {
\t\t\t\tcolor = [3]uint8{0xff, 0xff, 0xff}
\t\t\t}
\t\t} else {
\t\t\tcolor = p.ColourPalette[bgPX.Attributes&7][0]
\t\t}
\t}

\tp.PreparedFrame[p.ly][p.lx-8] = color
\tp.lx++
}
'''
pattern = r'func \(p \*PPU\) pushPixel\(\) \{.*?\n\}\n\n// FetcherState'
ppu, n = re.subn(pattern, new_push + "\n// FetcherState", ppu, count=1, flags=re.S)
if n != 1:
    raise SystemExit(f"ppu.go: pushPixel replacement count={n}")

marker = "func (p *PPU) renderBlank() {\n"
if ppu.count(marker) != 1:
    raise SystemExit(f"ppu.go: renderBlank marker count={ppu.count(marker)}")
ppu = ppu.replace(marker, marker + "\tif !p.videoOutput {\n\t\treturn\n\t}\n\n", 1)
ppu_path.write_text(ppu)

replace_once(
    "internal/ppu/state.go",
    "\tp.objBuffer = append([]Object(nil), s.ObjBuffer...)\n",
    "\tp.objBuffer = append(p.objBuffer[:0], s.ObjBuffer...)\n",
)

apu_path = Path("internal/apu/state.go")
apu = apu_path.read_text()
old = '''\tbufLen := bufferSize
\tif l := len(s.Buffer); l > bufLen {
\t\tbufLen = l
\t}
\ta.buffer = make([]float32, bufLen)
\tcopy(a.buffer, s.Buffer)
\ta.bufferPos = s.BufferPos
\ta.lastCatchup = s.LastCatchup
\ta.mute = s.Mute
\ta.headless = s.Headless
\ta.capacitors = s.Capacitors
\tif a.headless {
\t\ta.buffer = nil
\t\ta.bufferPos = 0
\t}
'''
new = '''\ta.lastCatchup = s.LastCatchup
\ta.mute = s.Mute
\ta.headless = s.Headless
\ta.capacitors = s.Capacitors
\tif a.headless {
\t\ta.buffer = nil
\t\ta.bufferPos = 0
\t} else {
\t\tbufLen := bufferSize
\t\tif l := len(s.Buffer); l > bufLen {
\t\t\tbufLen = l
\t\t}
\t\tif cap(a.buffer) < bufLen {
\t\t\ta.buffer = make([]float32, bufLen)
\t\t} else {
\t\t\ta.buffer = a.buffer[:bufLen]
\t\t\tclear(a.buffer)
\t\t}
\t\tcopy(a.buffer, s.Buffer)
\t\ta.bufferPos = s.BufferPos
\t}
'''
if apu.count(old) != 1:
    raise SystemExit("apu/state.go: restore buffer block not found exactly once")
apu_path.write_text(apu.replace(old, new, 1))

replace_once(
    "pkg/gomeboy/gomeboy.go",
    "\theadless bool\n\tsaveDir  string\n",
    "\theadless bool\n\tnoVideo  bool\n\tsaveDir  string\n",
)
replace_once(
    "pkg/gomeboy/gomeboy.go",
    '''func Headless() Option {
\treturn func(c *config) { c.headless = true }
}
''',
    '''func Headless() Option {
\treturn func(c *config) { c.headless = true }
}

// WithoutVideo disables RGB framebuffer composition while preserving exact
// PPU timing, FIFO progression, interrupts, DMA behaviour, and memory state.
// Frame() is not updated while video output is disabled.
func WithoutVideo() Option {
\treturn func(c *config) { c.noVideo = true }
}
''',
)
replace_once(
    "pkg/gomeboy/gomeboy.go",
    '''\tif cfg.saves {
\t\tgbOpts = append(gbOpts, gameboy.WithSaveDir(cfg.saveDir))
\t} else {
\t\tgbOpts = append(gbOpts, gameboy.WithoutSaves())
\t}

\te := &Emulator{gb: gameboy.NewGameBoy(gbOpts...)}
''',
    '''\tif cfg.saves {
\t\tgbOpts = append(gbOpts, gameboy.WithSaveDir(cfg.saveDir))
\t} else {
\t\tgbOpts = append(gbOpts, gameboy.WithoutSaves())
\t}
\tif cfg.noVideo {
\t\tgbOpts = append(gbOpts, gameboy.WithVideoOutput(false))
\t}

\te := &Emulator{gb: gameboy.NewGameBoy(gbOpts...)}
''',
)
