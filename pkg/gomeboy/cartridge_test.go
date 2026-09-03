package gomeboy

import (
	"bytes"
	"os"
	"testing"
)

func TestROMBytes(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	file, err := os.ReadFile(testROM)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	rom := e.ROM()
	if len(rom) != len(file) {
		t.Fatalf("ROM length = %d, want %d", len(rom), len(file))
	}
	if !bytes.Equal(rom, file) {
		t.Error("ROM bytes do not match the file on disk")
	}
}

func TestROMReturnsDefensiveCopy(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	before := e.ROMSHA256()
	rom := e.ROM()
	if len(rom) == 0 {
		t.Fatal("ROM() returned an empty ROM")
	}
	rom[0] ^= 0xff

	if got := e.ROMSHA256(); got != before {
		t.Fatalf("mutating ROM() result changed emulator ROM hash: got %x, want %x", got, before)
	}
	if fresh := e.ROM(); fresh[0] == rom[0] {
		t.Fatal("ROM() aliases emulator storage; want caller-owned copy")
	}
}

func TestROMSHA256Stable(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	want := e.ROMSHA256()

	if err := e.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if got := e.ROMSHA256(); got != want {
		t.Errorf("ROMSHA256 after Reset = %x, want %x", got, want)
	}

	state, err := e.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := e.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got := e.ROMSHA256(); got != want {
		t.Errorf("ROMSHA256 after LoadState = %x, want %x", got, want)
	}
}

func TestCartridgeBeforeROMLoadReturnsZeroValue(t *testing.T) {
	e, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	if got := e.Cartridge(); got != (CartInfo{}) {
		t.Fatalf("Cartridge() before ROM load = %+v, want zero value", got)
	}
}

func TestCartridgeInfo(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	c := e.Cartridge()
	if c.ROMSize <= 0 {
		t.Errorf("Cartridge().ROMSize = %d, want > 0", c.ROMSize)
	}
	if c.CartridgeType == "" {
		t.Error("Cartridge().CartridgeType is empty, want non-empty")
	}
}

func TestCartridgeInfoPokemonRed(t *testing.T) {
	path := pokemonRedROM(t)

	e, err := New(WithROM(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer e.Close()

	c := e.Cartridge()
	if c.Title != "POKEMON RED" {
		t.Errorf("Title = %q, want %q", c.Title, "POKEMON RED")
	}
	if c.MapperCode != 0x13 {
		t.Errorf("MapperCode = %#x, want 0x13", c.MapperCode)
	}
	if !c.Battery {
		t.Error("Battery = false, want true")
	}
	if !c.RAM {
		t.Error("RAM = false, want true")
	}
	if !c.SGBFlag {
		t.Error("SGBFlag = false, want true")
	}
}
