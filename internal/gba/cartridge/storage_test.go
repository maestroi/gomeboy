package cartridge

import (
	"bytes"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestPersistentDevicesRoundTripRawSaveData(t *testing.T) {
	tests := []struct {
		name string
		new  func() PersistentDevice
		size int
	}{
		{"sram", func() PersistentDevice { return NewSRAM() }, SRAMSize},
		{"flash64", func() PersistentDevice { return NewFlash64K() }, Flash64KSize},
		{"flash128", func() PersistentDevice { return NewFlash128K() }, Flash128KSize},
		{"eeprom512", func() PersistentDevice { return NewEEPROM512B() }, EEPROM512BSize},
		{"eeprom8k", func() PersistentDevice { return NewEEPROM8K() }, EEPROM8KSize},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.new()
			if got := src.SaveSize(); got != tt.size {
				t.Fatalf("SaveSize = %d, want %d", got, tt.size)
			}

			payload := make([]byte, tt.size)
			for i := range payload {
				payload[i] = byte(i*37 + 11)
			}
			if err := src.LoadSaveData(payload); err != nil {
				t.Fatalf("LoadSaveData: %v", err)
			}
			snapshot := src.SaveData()
			if !bytes.Equal(snapshot, payload) {
				t.Fatal("snapshot differs from loaded payload")
			}

			// SaveData must be an owned snapshot, not a mutable view into the
			// emulated cartridge.
			snapshot[0] ^= 0xff
			if bytes.Equal(snapshot, src.SaveData()) {
				t.Fatal("SaveData returned aliased storage")
			}

			dst := tt.new()
			if err := dst.LoadSaveData(src.SaveData()); err != nil {
				t.Fatalf("round-trip LoadSaveData: %v", err)
			}
			if !bytes.Equal(dst.SaveData(), src.SaveData()) {
				t.Fatal("round-trip save differs")
			}
		})
	}
}

func TestPersistentDeviceRejectsWrongSaveSize(t *testing.T) {
	device := NewFlash128K()
	if err := device.LoadSaveData(make([]byte, Flash64KSize)); err == nil {
		t.Fatal("loading 64 KiB into 128 KiB flash succeeded")
	}
}

func TestSetupPersistentDeviceTracksSelectedStorage(t *testing.T) {
	tests := []struct {
		saveType SaveType
		size     int
	}{
		{SaveSRAM, SRAMSize},
		{SaveFlash64K, Flash64KSize},
		{SaveFlash128K, Flash128KSize},
		{SaveEEPROM512B, EEPROM512BSize},
		{SaveEEPROM8K, EEPROM8KSize},
	}

	for _, tt := range tests {
		b := bus.New(nil, nil)
		setup := Configure(b, nil, Config{SaveType: tt.saveType})
		device := setup.PersistentDevice()
		if device == nil {
			t.Fatalf("%s PersistentDevice = nil", tt.saveType)
		}
		if got := device.SaveSize(); got != tt.size {
			t.Fatalf("%s size = %d, want %d", tt.saveType, got, tt.size)
		}
	}

	b := bus.New(nil, nil)
	if device := Configure(b, nil, Config{SaveType: SaveNone}).PersistentDevice(); device != nil {
		t.Fatalf("SaveNone PersistentDevice = %T, want nil", device)
	}
}
