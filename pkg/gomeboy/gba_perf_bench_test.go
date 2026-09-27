package gomeboy

import (
	"encoding/binary"
	"testing"
)

// gbaPerfROM returns a tiny self-contained ARM ROM used only for performance
// benchmarks. It enables OBJ rendering and then spins forever. Leaving OAM and
// OBJ VRAM at reset values intentionally exercises the renderer's sprite
// traversal on every visible frame without depending on external ROM assets.
func gbaPerfROM() []byte {
	rom := make([]byte, 512)

	// 08000000: ldr r0, [pc, #8]  ; r0 = DISPCNT
	// 08000004: ldr r1, [pc, #8]  ; r1 = OBJ enable
	// 08000008: str r1, [r0]
	// 0800000c: b .
	// 08000010: .word 0x04000000
	// 08000014: .word 0x00001000
	words := [...]uint32{
		0xe59f0008,
		0xe59f1008,
		0xe5801000,
		0xeafffffe,
		0x04000000,
		0x00001000,
	}
	for i, word := range words {
		binary.LittleEndian.PutUint32(rom[i*4:], word)
	}
	return rom
}

func newGBAPerfEmulator(b *testing.B, headless bool) *Emulator {
	b.Helper()
	opts := []Option{
		WithROMBytes(gbaPerfROM()),
		WithModel(ModelAGB),
	}
	if headless {
		opts = append(opts, Headless())
	}
	e, err := New(opts...)
	if err != nil {
		b.Fatalf("New GBA perf emulator: %v", err)
	}
	for i := 0; i < 10; i++ {
		e.StepFrame()
	}
	return e
}

// BenchmarkPerfGBAStepFrameHeadless measures one full emulated GBA frame while
// suppressing host audio sample buffering.
func BenchmarkPerfGBAStepFrameHeadless(b *testing.B) {
	e := newGBAPerfEmulator(b, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

// BenchmarkPerfGBAStepFrameAudio measures one full GBA frame including the
// normal 96 kHz host-audio generation path used by the desktop frontend.
func BenchmarkPerfGBAStepFrameAudio(b *testing.B) {
	e := newGBAPerfEmulator(b, false)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrame()
	}
}

// BenchmarkPerfGBAStepFrames60Headless makes the real-time budget obvious:
// one iteration advances roughly one second of native GBA video time.
func BenchmarkPerfGBAStepFrames60Headless(b *testing.B) {
	e := newGBAPerfEmulator(b, true)
	defer e.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.StepFrames(60)
	}
}
