package system

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
)

func TestSystemNewAutoConfiguresDetectedSaveHardware(t *testing.T) {
	rom := []byte("test ROM with SRAM_V113 marker")
	m := New(nil, rom)

	if m.Cartridge.SaveType != cartridge.SaveSRAM {
		t.Fatalf("save type = %s, want sram", m.Cartridge.SaveType)
	}
	m.Bus.Write8(bus.SaveStart+0x1234, 0x5a, bus.Access{})
	if got, _ := m.Bus.Read8(bus.SaveStart+0x1234, bus.Access{}); got != 0x5a {
		t.Fatalf("auto-configured SRAM read = %02x, want 5a", got)
	}
}

func TestSystemExplicitCartridgeConfigOverridesROMDetection(t *testing.T) {
	rom := []byte("test ROM with FLASH1M_V103 marker")
	m := NewWithCartridgeConfig(nil, rom, cartridge.Config{
		SaveType: cartridge.SaveEEPROM512B,
	})

	if m.Cartridge.SaveType != cartridge.SaveEEPROM512B || !m.Cartridge.Explicit {
		t.Fatalf("cartridge setup = %+v, want explicit EEPROM512B", m.Cartridge)
	}
	if m.Cartridge.Detected.Type != cartridge.SaveFlash128K {
		t.Fatalf("detected type = %s, want flash-128k", m.Cartridge.Detected.Type)
	}
	if got, _ := m.Bus.Read16(bus.EEPROMStart, bus.Access{}); got != 1 {
		t.Fatalf("explicit EEPROM idle read = %04x, want 0001", got)
	}
}

func TestSystemAmbiguousMarkerDetectionDoesNotGuess(t *testing.T) {
	rom := []byte("SRAM_V113...FLASH1M_V103")
	m := New(nil, rom)

	if !m.Cartridge.Detected.Ambiguous {
		t.Fatal("conflicting markers should be reported as ambiguous")
	}
	if m.Cartridge.SaveType != cartridge.SaveNone {
		t.Fatalf("ambiguous auto type = %s, want none", m.Cartridge.SaveType)
	}
}
