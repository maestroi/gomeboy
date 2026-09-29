package gameboy

import (
	"os"
	"path/filepath"
	"testing"
)

const mbc3RTCCyclesPerSecond uint64 = 4_194_304

func mbc3PersistenceROM() []byte {
	rom := make([]byte, 0x8000)
	rom[0x0147] = 0x10 // MBC3 + TIMER + RAM + BATTERY
	rom[0x0149] = 0x03 // 32 KiB RAM
	return rom
}

func newMBC3PersistenceGameBoy(t *testing.T, name string, opts ...Opt) *GameBoy {
	t.Helper()
	g := NewGameBoy(opts...)
	if err := g.LoadROMBytes(mbc3PersistenceROM(), name); err != nil {
		t.Fatalf("LoadROMBytes: %v", err)
	}
	g.Bus.Write(0x0000, 0x0a)
	return g
}

func gbWriteRTC(g *GameBoy, register, value byte) {
	g.Bus.Write(0x4000, register)
	g.Bus.Write(0xa000, value)
}

func gbLatchRTC(g *GameBoy) {
	g.Bus.Write(0x6000, 0)
	g.Bus.Write(0x6000, 1)
}

func gbReadRTC(g *GameBoy, register byte) byte {
	g.Bus.Write(0x4000, register)
	return g.Bus.Read(0xa000)
}

func TestMBC3RTCBatterySaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	g := newMBC3PersistenceGameBoy(t, "rtc-persist", WithSaveDir(dir))

	gbWriteRTC(g, 0x0c, 0x40) // halt for deterministic programming
	gbWriteRTC(g, 0x08, 37)
	gbWriteRTC(g, 0x09, 12)
	gbWriteRTC(g, 0x0a, 6)
	gbWriteRTC(g, 0x0b, 0xab)
	gbWriteRTC(g, 0x0c, 0xc1)
	gbLatchRTC(g)

	wantSize := int64(len(g.Bus.Cartridge().RAM))
	if err := g.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path := filepath.Join(dir, "rtc-persist.sav")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
	if info.Size() != wantSize {
		t.Fatalf("RTC save size = %d, want %d (RAM plus live/latched RTC registers)", info.Size(), wantSize)
	}

	restored := newMBC3PersistenceGameBoy(t, "rtc-persist", WithSaveDir(dir))
	for _, tc := range []struct {
		register byte
		want     byte
	}{
		{0x08, 37},
		{0x09, 12},
		{0x0a, 6},
		{0x0b, 0xab},
		{0x0c, 0xc1},
	} {
		if got := gbReadRTC(restored, tc.register); got != tc.want {
			t.Errorf("restored RTC register %#02x = %#02x, want %#02x", tc.register, got, tc.want)
		}
	}
}

func TestMBC3RTCSerializedStateRoundTrip(t *testing.T) {
	g := newMBC3PersistenceGameBoy(t, "rtc-state", WithoutSaves())

	gbWriteRTC(g, 0x0c, 0x40)
	gbWriteRTC(g, 0x08, 12)
	gbWriteRTC(g, 0x09, 34)
	gbLatchRTC(g)

	state, err := g.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	gbWriteRTC(g, 0x08, 55)
	gbWriteRTC(g, 0x09, 56)
	gbLatchRTC(g)
	if got := gbReadRTC(g, 0x08); got != 55 {
		t.Fatalf("mutated RTC seconds = %d, want 55", got)
	}

	if err := g.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := gbReadRTC(g, 0x08); got != 12 {
		t.Errorf("seconds after LoadState = %d, want 12", got)
	}
	if got := gbReadRTC(g, 0x09); got != 34 {
		t.Errorf("minutes after LoadState = %d, want 34", got)
	}
}

func TestMBC3RTCCheckpointRoundTripPreservesSubsecondPhase(t *testing.T) {
	g := newMBC3PersistenceGameBoy(t, "rtc-checkpoint", WithoutSaves())

	gbWriteRTC(g, 0x08, 0)
	g.Scheduler.Tick(mbc3RTCCyclesPerSecond + mbc3RTCCyclesPerSecond/2)
	gbWriteRTC(g, 0x0c, 0x40) // halt at 1.5 seconds
	gbLatchRTC(g)
	if got := gbReadRTC(g, 0x08); got != 1 {
		t.Fatalf("seconds at checkpoint = %d, want 1", got)
	}

	var checkpoint State
	g.CheckpointInto(&checkpoint)

	gbWriteRTC(g, 0x0c, 0x00)
	g.Scheduler.Tick(5 * mbc3RTCCyclesPerSecond)
	gbLatchRTC(g)
	if got := gbReadRTC(g, 0x08); got == 1 {
		t.Fatal("RTC did not change after checkpoint mutation")
	}

	g.Restore(checkpoint)
	if got := gbReadRTC(g, 0x08); got != 1 {
		t.Fatalf("seconds after checkpoint restore = %d, want 1", got)
	}

	gbWriteRTC(g, 0x0c, 0x00)
	g.Scheduler.Tick(mbc3RTCCyclesPerSecond / 2)
	gbLatchRTC(g)
	if got := gbReadRTC(g, 0x08); got != 2 {
		t.Fatalf("seconds after restored half-second phase completes = %d, want 2", got)
	}
}
