from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    p = Path(path)
    text = p.read_text()
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"{path}: expected exactly one replacement target, found {count}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "internal/scheduler/scheduler.go",
    '''func (s *Scheduler) DescheduleEvent(eventType EventType) {
\tif s.root == nil {
\t\treturn
\t}

\tvar prev *Event
\tevent := s.root

\tfor event != nil {
\t\tif event.eventType == eventType {
\t\t\tif prev == nil {
\t\t\t\ts.root = event.next
\t\t\t\tbreak
\t\t\t} else {
\t\t\t\tprev.next = event.next
\t\t\t\tbreak
\t\t\t}
\t\t}
\t\tprev = event
\t\tevent = event.next
\t}
}
''',
    '''func (s *Scheduler) DescheduleEvent(eventType EventType) {
\tif s.root == nil {
\t\treturn
\t}

\tvar prev *Event
\tevent := s.root

\t// Never remove the MaxUint64 sentinel. Its zero-value event type may
\t// otherwise alias a real EventType and leave the scheduler without a tail.
\tfor event != nil && event.cycle != math.MaxUint64 {
\t\tif event.eventType == eventType {
\t\t\tif prev == nil {
\t\t\t\ts.root = event.next
\t\t\t\tif s.root != nil {
\t\t\t\t\ts.nextEventAt = s.root.cycle
\t\t\t\t} else {
\t\t\t\t\ts.nextEventAt = math.MaxUint64
\t\t\t\t}
\t\t\t} else {
\t\t\t\tprev.next = event.next
\t\t\t}
\t\t\tevent.next = nil
\t\t\tbreak
\t\t}
\t\tprev = event
\t\tevent = event.next
\t}
}
''',
)

replace_once(
    "internal/apu/apu.go",
    '''\tbuffer                  []float32
\tbufferPos               uint32
\tb                       *io.Bus
''',
    '''\tbuffer                  []float32
\tbufferPos               uint32
\tcapacitors              [2]float32
\tb                       *io.Bus
''',
)

replace_once(
    "internal/apu/apu.go",
    '''// SetHeadless controls whether generated audio samples are accumulated.
// When headless is true, the APU keeps evolving its internal state but
// does not grow its sample buffer, so long-running headless emulation
// does not leak memory.
func (a *APU) SetHeadless(headless bool) { a.headless = headless }
''',
    '''// SetHeadless controls whether audio output is generated. Hardware-visible
// APU state (channels, frame sequencer, PCM reads, and noise state) continues to
// evolve, but the 96 kHz output-sampling event is removed while headless. Noise
// state is already advanced lazily by catchupLFSR when it becomes observable.
func (a *APU) SetHeadless(headless bool) {
\tif a.headless == headless {
\t\treturn
\t}
\ta.headless = headless
\tif headless {
\t\ta.s.DescheduleEvent(scheduler.APUSample)
\t\ta.buffer = nil
\t\ta.bufferPos = 0
\t\treturn
\t}
\tif a.buffer == nil {
\t\ta.buffer = make([]float32, bufferSize)
\t}
\ta.s.ScheduleEvent(scheduler.APUSample, samplePeriod)
}
''',
)

replace_once(
    "internal/apu/apu.go",
    '''var capacitors [2]float32

func highPass(ch int, in float32, dacEnabled bool) float32 {
\tout := float32(0.0)
\tif dacEnabled {
\t\tout = in - capacitors[ch]
\t\tcapacitors[ch] = in - out*0.998166636
\t}
\treturn out
}
''',
    '''func (a *APU) highPass(ch int, in float32, dacEnabled bool) float32 {
\tout := float32(0.0)
\tif dacEnabled {
\t\tout = in - a.capacitors[ch]
\t\ta.capacitors[ch] = in - out*0.998166636
\t}
\treturn out
}
''',
)

replace_once(
    "internal/apu/apu.go",
    '''func (a *APU) sample() {
\tchannels := a.channels
''',
    '''func (a *APU) sample() {
\t// Headless mode has no audio consumer. SetHeadless normally removes this
\t// event entirely; this guard also makes old/inconsistent save states safe.
\tif a.headless {
\t\treturn
\t}

\tchannels := a.channels
''',
)

replace_once(
    "internal/apu/apu.go",
    '''\tfLeft := highPass(0, left, enabled)
\tfRight := highPass(1, right, enabled)
''',
    '''\tfLeft := a.highPass(0, left, enabled)
\tfRight := a.highPass(1, right, enabled)
''',
)

replace_once(
    "internal/apu/apu.go",
    '''\tif !a.headless {
\t\tif a.bufferPos < bufferSize {
\t\t\ta.buffer[a.bufferPos] = fLeft
\t\t\ta.buffer[a.bufferPos+1] = fRight
\t\t} else {
\t\t\ta.buffer = append(a.buffer, fLeft, fRight)
\t\t}
\t\ta.bufferPos += 2
\t}
''',
    '''\tif a.bufferPos < bufferSize {
\t\ta.buffer[a.bufferPos] = fLeft
\t\ta.buffer[a.bufferPos+1] = fRight
\t} else {
\t\ta.buffer = append(a.buffer, fLeft, fRight)
\t}
\ta.bufferPos += 2
''',
)

replace_once(
    "internal/apu/state.go",
    '''\tMute                    bool
\tHeadless                bool

\tDebug struct {
''',
    '''\tMute                    bool
\tHeadless                bool
\tCapacitors              [2]float32

\tDebug struct {
''',
)

replace_once(
    "internal/apu/state.go",
    '''\t\tMute:        a.mute,
\t\tHeadless:    a.headless,
\t}
''',
    '''\t\tMute:        a.mute,
\t\tHeadless:    a.headless,
\t\tCapacitors:  a.capacitors,
\t}
''',
)

replace_once(
    "internal/apu/state.go",
    '''\ta.mute = s.Mute
\ta.headless = s.Headless

\ta.channel1.duty = s.Channel1.Square.Duty
''',
    '''\ta.mute = s.Mute
\ta.headless = s.Headless
\ta.capacitors = s.Capacitors
\tif a.headless {
\t\ta.buffer = nil
\t\ta.bufferPos = 0
\t}

\ta.channel1.duty = s.Channel1.Square.Duty
''',
)

replace_once(
    "internal/gameboy/gameboy.go",
    '''func (g *GameBoy) Step() {
\tg.mu.Lock()
\tdefer g.mu.Unlock()
\tg.running = true
\tg.CPU.Frame()
\tg.running = false
\tg.frames++
}
''',
    '''func (g *GameBoy) Step() {
\tg.mu.Lock()
\tdefer g.mu.Unlock()
\tg.running = true
\tg.CPU.Frame()
\tg.running = false
\tg.frames++
}

// StepFrames advances n frames while taking the frame/save-state mutex once.
// This preserves the same serialization guarantee as Step without paying one
// mutex lock/unlock pair per frame in batch/agent workloads.
func (g *GameBoy) StepFrames(n int) {
\tif n <= 0 {
\t\treturn
\t}
\tg.mu.Lock()
\tdefer g.mu.Unlock()
\tg.running = true
\tfor i := 0; i < n; i++ {
\t\tg.CPU.Frame()
\t\tg.frames++
\t}
\tg.running = false
}
''',
)

replace_once(
    "pkg/gomeboy/gomeboy.go",
    '''func (e *Emulator) StepFrames(n int) {
\tfor i := 0; i < n; i++ {
\t\te.gb.Step()
\t}
}
''',
    '''func (e *Emulator) StepFrames(n int) {
\te.gb.StepFrames(n)
}
''',
)

replace_once(
    "pkg/gomeboy/inspect.go",
    '''func (e *Emulator) PeekInto(addr uint16, dst []byte) {
\tfor i := range dst {
\t\tdst[i] = e.gb.Bus.Get(addr + uint16(i))
\t}
}
''',
    '''func (e *Emulator) PeekInto(addr uint16, dst []byte) {
\tif len(dst) == 0 {
\t\treturn
\t}

\t// Most agent observations are contiguous WRAM/HRAM ranges. Use the
\t// runtime-optimized bulk copy when the range does not wrap the 16-bit bus.
\tif end := int(addr) + len(dst); end <= 0xffff {
\t\te.gb.Bus.CopyFrom(addr, uint16(end), dst)
\t\treturn
\t}

\t// Preserve the historical uint16 wraparound semantics for boundary-crossing
\t// or unusually large reads.
\tfor i := range dst {
\t\tdst[i] = e.gb.Bus.Get(addr + uint16(i))
\t}
}
''',
)

Path("internal/scheduler/scheduler_test.go").write_text(r'''package scheduler

import "testing"

func TestDescheduleRootUpdatesNextEvent(t *testing.T) {
\ts := NewScheduler()
\tearlyRan := false
\tfutureRan := false
\ts.RegisterEvent(APUSample, func() { earlyRan = true })
\ts.RegisterEvent(APUChannel1, func() { futureRan = true })
\ts.ScheduleEvent(APUSample, 4)
\ts.ScheduleEvent(APUChannel1, 20)

\ts.DescheduleEvent(APUSample)
\ts.Tick(4)
\tif earlyRan {
\t\tt.Fatal("descheduled event ran")
\t}
\tif futureRan {
\t\tt.Fatal("future event ran early after root deschedule")
\t}
\tif got := s.Until(APUChannel1); got != 16 {
\t\tt.Fatalf("Until(APUChannel1) = %d, want 16", got)
\t}

\ts.Tick(16)
\tif !futureRan {
\t\tt.Fatal("future event did not run at its scheduled cycle")
\t}
}

func TestDescheduleDoesNotRemoveSentinel(t *testing.T) {
\ts := NewScheduler()
\ts.DescheduleEvent(EventType(0))
\ts.Tick(1)
\tif got := s.Cycle(); got != 1 {
\t\tt.Fatalf("Cycle() = %d, want 1", got)
\t}
}
''')

Path("internal/apu/headless_test.go").write_text(r'''package apu

import (
\t"testing"

\t"github.com/thelolagemann/gomeboy/internal/io"
\t"github.com/thelolagemann/gomeboy/internal/scheduler"
)

func TestHighPassStateIsPerAPU(t *testing.T) {
\ta := &APU{}
\tb := &APU{}

\tif got := a.highPass(0, 1, true); got != 1 {
\t\tt.Fatalf("first APU highPass = %v, want 1", got)
\t}
\tif got := b.highPass(0, 1, true); got != 1 {
\t\tt.Fatalf("second APU inherited filter state: got %v, want 1", got)
\t}
}

func TestHeadlessRemovesOutputSamplingEvent(t *testing.T) {
\ts := scheduler.NewScheduler()
\tb := io.NewBus(s, make([]byte, 32*1024))
\ta := New(b, s)

\tif got := s.Until(scheduler.APUSample); got == 0 {
\t\tt.Fatal("APUSample was not scheduled by default")
\t}
\ta.SetHeadless(true)
\tif got := s.Until(scheduler.APUSample); got != 0 {
\t\tt.Fatalf("APUSample still scheduled in headless mode: %d cycles", got)
\t}
\tif a.buffer != nil || a.bufferPos != 0 {
\t\tt.Fatal("headless mode retained transient audio buffer")
\t}

\ta.SetHeadless(false)
\tif got := s.Until(scheduler.APUSample); got == 0 {
\t\tt.Fatal("APUSample was not restored after leaving headless mode")
\t}
\tif len(a.buffer) != bufferSize {
\t\tt.Fatalf("audio buffer length = %d, want %d", len(a.buffer), bufferSize)
\t}
}
''')

Path("pkg/gomeboy/perf_optimizations_test.go").write_text(r'''package gomeboy

import (
\t"bytes"
\t"testing"
)

func newPerfTestEmulator(t *testing.T) *Emulator {
\tt.Helper()
\te, err := New(WithROMBytes(perfROM()), Headless())
\tif err != nil {
\t\tt.Fatalf("New: %v", err)
\t}
\tt.Cleanup(func() { _ = e.Close() })
\treturn e
}

func TestPerfStepFramesMatchesRepeatedStep(t *testing.T) {
\ta := newPerfTestEmulator(t)
\tb := newPerfTestEmulator(t)

\ta.StepFrames(5)
\tfor i := 0; i < 5; i++ {
\t\tb.StepFrame()
\t}

\tif a.FrameCount() != b.FrameCount() || a.Cycle() != b.Cycle() {
\t\tt.Fatalf("batch stepping diverged: frames %d/%d cycles %d/%d", a.FrameCount(), b.FrameCount(), a.Cycle(), b.Cycle())
\t}
\tif !bytes.Equal(a.Frame().RGB, b.Frame().RGB) {
\t\tt.Fatal("batch stepping produced a different frame")
\t}
}

func TestPerfPeekIntoMatchesPeek8(t *testing.T) {
\te := newPerfTestEmulator(t)
\tdst := make([]byte, 4096)
\te.PeekInto(0xC000, dst)
\tfor i, got := range dst {
\t\twant := e.Peek8(0xC000 + uint16(i))
\t\tif got != want {
\t\t\tt.Fatalf("PeekInto byte %d = %02x, want %02x", i, got, want)
\t\t}
\t}

\twrap := make([]byte, 4)
\te.PeekInto(0xFFFE, wrap)
\tfor i, got := range wrap {
\t\twant := e.Peek8(0xFFFE + uint16(i))
\t\tif got != want {
\t\t\tt.Fatalf("wrapped PeekInto byte %d = %02x, want %02x", i, got, want)
\t\t}
\t}
}
''')
