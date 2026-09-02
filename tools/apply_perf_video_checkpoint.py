from pathlib import Path


def replace(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    if new in text:
        return
    if old not in text:
        raise SystemExit(f"expected snippet not found in {path}: {old[:80]!r}")
    p.write_text(text.replace(old, new, 1))


# PPU: retain exact timing/fetch behavior while skipping output-only work.
replace(
    "internal/ppu/ppu.go",
    "\t// Frame buffers\n\tPreparedFrame [ScreenHeight][ScreenWidth][3]uint8\n",
    "\t// Frame buffers\n\tPreparedFrame [ScreenHeight][ScreenWidth][3]uint8\n\tvideoOutput   bool\n",
)
replace(
    "internal/ppu/ppu.go",
    "\tp := &PPU{\n\t\tb: b,\n\t\ts: s,\n\n\t\tbgFIFO:  utils.NewFIFO[FIFOEntry](8),\n\t\tobjFIFO: utils.NewFIFO[FIFOEntry](8),\n\t}\n",
    "\tp := &PPU{\n\t\tb:           b,\n\t\ts:           s,\n\t\tvideoOutput: true,\n\n\t\tbgFIFO:  utils.NewFIFO[FIFOEntry](8),\n\t\tobjFIFO: utils.NewFIFO[FIFOEntry](8),\n\t}\n",
)
replace(
    "internal/ppu/ppu.go",
    "\t// are there any pending object pixels?\n\tif p.objFIFO.Size > 0 {\n\t\tobjPX = p.objFIFO.Pop()\n\n\t\tif objPX.Color > 0 && p.objEnabled {\n\t\t\tcolor = p.ColourOBJPalette[objPX.Palette][objPX.Color]\n\n\t\t\tdrawObject = true\n\t\t\tif objPX.Attributes&types.Bit7 > 0 {\n\t\t\t\tbgPriority = true\n\t\t\t}\n\t\t}\n\t}\n",
    "\t// are there any pending object pixels? Keep consuming the FIFO even\n\t// when video output is disabled: FIFO timing is emulation state.\n\tif p.objFIFO.Size > 0 {\n\t\tobjPX = p.objFIFO.Pop()\n\t}\n",
)
replace(
    "internal/ppu/ppu.go",
    "\t// are we currently offscreen? (pixels are simply discarded)\n\tif p.lx < 8 {\n\t\tp.lx++\n\t\treturn\n\t}\n\n\t// BG_EN bit is different on CGB\n",
    "\t// are we currently offscreen? (pixels are simply discarded)\n\tif p.lx < 8 {\n\t\tp.lx++\n\t\treturn\n\t}\n\n\t// Rendering the RGB value is output-only. Agent/search workloads that\n\t// observe memory can disable it without changing PPU timing, FIFO state,\n\t// interrupts, or bus locking.\n\tif !p.videoOutput {\n\t\tp.lx++\n\t\treturn\n\t}\n\n\tif objPX != nil && objPX.Color > 0 && p.objEnabled {\n\t\tcolor = p.ColourOBJPalette[objPX.Palette][objPX.Color]\n\n\t\tdrawObject = true\n\t\tif objPX.Attributes&types.Bit7 > 0 {\n\t\t\tbgPriority = true\n\t\t}\n\t}\n\n\t// BG_EN bit is different on CGB\n",
)
Path("internal/ppu/output.go").write_text('''package ppu

// SetVideoOutput controls RGB framebuffer generation. Disabling output keeps
// the complete PPU timing/fetch pipeline running, but skips palette composition
// and writes to PreparedFrame.
func (p *PPU) SetVideoOutput(enabled bool) {
\tp.videoOutput = enabled
}

// VideoOutputEnabled reports whether RGB framebuffer generation is enabled.
func (p *PPU) VideoOutputEnabled() bool {
\treturn p.videoOutput
}
''')

# Internal option so the setting survives LoadROM/Reset, which rebuild the PPU.
options = Path("internal/gameboy/options.go")
text = options.read_text()
addition = '''\n// WithoutVideoOutput disables RGB framebuffer generation while preserving PPU\n// timing and hardware-visible behavior.\nfunc WithoutVideoOutput() Opt {\n\treturn func(gb *GameBoy) { gb.PPU.SetVideoOutput(false) }\n}\n'''
if "func WithoutVideoOutput() Opt" not in text:
    options.write_text(text.rstrip() + "\n" + addition)

# Public option + allocation-free ReadInto.
replace(
    "pkg/gomeboy/gomeboy.go",
    "\theadless bool\n\tsaveDir  string\n",
    "\theadless bool\n\tnoVideo  bool\n\tsaveDir  string\n",
)
replace(
    "pkg/gomeboy/gomeboy.go",
    "// WithModel selects the hardware model to emulate, overriding the model\n",
    "// WithoutVideo disables RGB framebuffer generation. The PPU still runs its\n// full timing, fetcher, interrupt, and bus-lock behavior, making this useful for\n// memory/state-driven agents that do not inspect rendered frames. Frame() returns\n// the last framebuffer contents while video output is disabled.\nfunc WithoutVideo() Option {\n\treturn func(c *config) { c.noVideo = true }\n}\n\n// WithModel selects the hardware model to emulate, overriding the model\n",
)
replace(
    "pkg/gomeboy/gomeboy.go",
    "\tif cfg.printer {\n\t\tgbOpts = append(gbOpts, gameboy.WithPrinter())\n\t}\n",
    "\tif cfg.printer {\n\t\tgbOpts = append(gbOpts, gameboy.WithPrinter())\n\t}\n\tif cfg.noVideo {\n\t\tgbOpts = append(gbOpts, gameboy.WithoutVideoOutput())\n\t}\n",
)
replace(
    "pkg/gomeboy/gomeboy.go",
    "func (e *Emulator) Read(addr uint16, length int) []byte {\n\tout := make([]byte, length)\n\tfor i := range length {\n\t\tout[i] = e.gb.Bus.Read(addr + uint16(i))\n\t}\n\treturn out\n}\n",
    "func (e *Emulator) Read(addr uint16, length int) []byte {\n\tout := make([]byte, length)\n\te.ReadInto(addr, out)\n\treturn out\n}\n\n// ReadInto performs CPU-accurate reads into dst without allocating. Reads can\n// be affected by DMA conflicts and PPU region locks, just like Read8 and Read.\nfunc (e *Emulator) ReadInto(addr uint16, dst []byte) {\n\tfor i := range dst {\n\t\tdst[i] = e.gb.Bus.Read(addr + uint16(i))\n\t}\n}\n",
)

# Opaque in-process checkpoints: avoid gob encode/decode while keeping portable
# SaveState/LoadState unchanged.
Path("pkg/gomeboy/checkpoint.go").write_text('''package gomeboy

import "github.com/thelolagemann/gomeboy/internal/gameboy"

// Checkpoint is an opaque in-memory emulator snapshot intended for fast
// branching/search workloads. It is process-local: use SaveState for portable
// serialized state.
type Checkpoint struct {
\tstate gameboy.State
}

// CheckpointInto captures the current execution state into cp. Reusing the same
// Checkpoint avoids serialized save-state encoding and its byte-buffer churn.
func (e *Emulator) CheckpointInto(cp *Checkpoint) {
\tif cp == nil {
\t\treturn
\t}
\tcp.state = e.gb.Snapshot()
}

// RestoreCheckpoint restores a checkpoint previously captured from an emulator
// running the same ROM.
func (e *Emulator) RestoreCheckpoint(cp *Checkpoint) {
\tif cp == nil {
\t\treturn
\t}
\te.gb.Restore(cp.state)
}
''')

# Behavioral tests.
Path("pkg/gomeboy/perf_features_test.go").write_text('''package gomeboy

import (
\t"bytes"
\t"reflect"
\t"testing"
)

func TestWithoutVideoMatchesExecutionState(t *testing.T) {
\tvideo, err := New(WithROMBytes(perfROM()), Headless())
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tdefer video.Close()

\tnoVideo, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tdefer noVideo.Close()

\tvideo.StepFrames(120)
\tnoVideo.StepFrames(120)

\ta := video.gb.Snapshot()
\tb := noVideo.gb.Snapshot()
\t// PreparedFrame is deliberately output-only and therefore expected to differ.
\ta.PPU.PreparedFrame = [144][160][3]uint8{}
\tb.PPU.PreparedFrame = [144][160][3]uint8{}
\tif !reflect.DeepEqual(a, b) {
\t\tt.Fatal("disabling video changed emulation state")
\t}
}

func TestCheckpointRestore(t *testing.T) {
\te, err := New(WithROMBytes(perfROM()), Headless(), WithoutVideo())
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tdefer e.Close()

\te.StepFrames(5)
\twantCycle := e.Cycle()
\twantFrames := e.FrameCount()
\twantMemory := e.SnapshotMemory()

\tvar cp Checkpoint
\te.CheckpointInto(&cp)
\te.Press(ButtonA)
\te.StepFrames(3)
\te.RestoreCheckpoint(&cp)

\tif e.Cycle() != wantCycle || e.FrameCount() != wantFrames {
\t\tt.Fatalf("checkpoint restored cycle/frame = %d/%d, want %d/%d", e.Cycle(), e.FrameCount(), wantCycle, wantFrames)
\t}
\tif got := e.SnapshotMemory(); !bytes.Equal(got, wantMemory) {
\t\tt.Fatal("checkpoint did not restore memory")
\t}

\t// Reuse the same checkpoint object at a new point in time.
\te.StepFrames(2)
\twantCycle = e.Cycle()
\te.CheckpointInto(&cp)
\te.StepFrames(1)
\te.RestoreCheckpoint(&cp)
\tif e.Cycle() != wantCycle {
\t\tt.Fatalf("reused checkpoint restored cycle = %d, want %d", e.Cycle(), wantCycle)
\t}
}

func TestReadIntoMatchesRead(t *testing.T) {
\te, err := New(WithROMBytes(perfROM()), Headless())
\tif err != nil {
\t\tt.Fatal(err)
\t}
\tdefer e.Close()

\twant := e.Read(0xC000, 256)
\tgot := make([]byte, 256)
\te.ReadInto(0xC000, got)
\tif !bytes.Equal(got, want) {
\t\tt.Fatal("ReadInto differs from Read")
\t}
}
''')

# Benchmarks: retain previous names and add explicit video/checkpoint/read cases.
p = Path("pkg/gomeboy/perf_bench_test.go")
text = p.read_text()
replace_old = '''func newPerfEmulator(b *testing.B, headless bool) *Emulator {
\tb.Helper()
\topts := []Option{WithROMBytes(perfROM())}
\tif headless {
\t\topts = append(opts, Headless())
\t}
\te, err := New(opts...)
'''
replace_new = '''func newPerfEmulatorWithVideo(b *testing.B, headless, video bool) *Emulator {
\tb.Helper()
\topts := []Option{WithROMBytes(perfROM())}
\tif headless {
\t\topts = append(opts, Headless())
\t}
\tif !video {
\t\topts = append(opts, WithoutVideo())
\t}
\te, err := New(opts...)
'''
if "func newPerfEmulatorWithVideo" not in text:
    if replace_old not in text:
        raise SystemExit("benchmark helper snippet not found")
    text = text.replace(replace_old, replace_new, 1)
    text = text.replace("newPerfEmulator(b, true)", "newPerfEmulatorWithVideo(b, true, true)")
    text = text.replace("newPerfEmulator(b, false)", "newPerfEmulatorWithVideo(b, false, true)")
    text += '''

func BenchmarkPerfStepFrameHeadlessNoVideo(b *testing.B) {
\te := newPerfEmulatorWithVideo(b, true, false)
\tdefer e.Close()
\tb.ReportAllocs()
\tb.ResetTimer()
\tfor i := 0; i < b.N; i++ {
\t\te.StepFrame()
\t}
}

func BenchmarkPerfStepFrames60HeadlessNoVideo(b *testing.B) {
\te := newPerfEmulatorWithVideo(b, true, false)
\tdefer e.Close()
\tb.ReportAllocs()
\tb.ResetTimer()
\tfor i := 0; i < b.N; i++ {
\t\te.StepFrames(60)
\t}
}

func BenchmarkPerfCheckpointRoundTrip(b *testing.B) {
\te := newPerfEmulatorWithVideo(b, true, false)
\tdefer e.Close()
\tvar cp Checkpoint
\tb.ReportAllocs()
\tb.ResetTimer()
\tfor i := 0; i < b.N; i++ {
\t\te.CheckpointInto(&cp)
\t\te.RestoreCheckpoint(&cp)
\t}
}

func BenchmarkPerfSaveStateRoundTrip(b *testing.B) {
\te := newPerfEmulatorWithVideo(b, true, false)
\tdefer e.Close()
\tb.ReportAllocs()
\tb.ResetTimer()
\tfor i := 0; i < b.N; i++ {
\t\tstate, err := e.SaveState()
\t\tif err != nil {
\t\t\tb.Fatal(err)
\t\t}
\t\tif err := e.LoadState(state); err != nil {
\t\t\tb.Fatal(err)
\t\t}
\t}
}

func BenchmarkPerfReadInto4K(b *testing.B) {
\te := newPerfEmulatorWithVideo(b, true, false)
\tdefer e.Close()
\tdst := make([]byte, 4096)
\tb.ReportAllocs()
\tb.ResetTimer()
\tfor i := 0; i < b.N; i++ {
\t\te.ReadInto(0xC000, dst)
\t}
}
'''
    p.write_text(text)

# README documentation for the new APIs and corrected Headless wording.
p = Path("README.md")
text = p.read_text()
text = text.replace(
    "| `Headless() Option` | Disable APU sample accumulation (prevents memory growth when running long). |",
    "| `Headless() Option` | Disable APU output sampling while preserving hardware-visible APU timing. |\n| `WithoutVideo() Option` | Skip RGB framebuffer composition/writes while preserving PPU timing and hardware-visible behavior. |",
)
text = text.replace(
    "| `Read8(addr uint16) byte` / `Read(addr uint16, n int) []byte` | CPU-accurate reads (DMA / PPU locks apply). |",
    "| `Read8(addr uint16) byte` / `Read(addr uint16, n int) []byte` / `ReadInto(addr, dst)` | CPU-accurate reads (DMA / PPU locks apply); `ReadInto` reuses caller storage. |",
)
text = text.replace(
    "| `SaveState() ([]byte, error)` / `LoadState([]byte) error` | Serialize / restore the full emulator state. |",
    "| `CheckpointInto(*Checkpoint)` / `RestoreCheckpoint(*Checkpoint)` | Fast opaque in-process checkpoint/restore for branching and agent search. |\n| `SaveState() ([]byte, error)` / `LoadState([]byte) error` | Serialize / restore the full emulator state. |",
)
p.write_text(text)
