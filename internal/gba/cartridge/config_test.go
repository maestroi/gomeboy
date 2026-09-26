package cartridge

import (
	"reflect"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestDetectSaveTypeMarkers(t *testing.T) {
	tests := []struct {
		name   string
		marker string
		want   SaveType
	}{
		{"SRAM", "SRAM_V113", SaveSRAM},
		{"SRAM_F", "SRAM_F_V110", SaveSRAM},
		{"Flash", "FLASH_V126", SaveFlash64K},
		{"Flash512", "FLASH512_V130", SaveFlash64K},
		{"Flash1M", "FLASH1M_V103", SaveFlash128K},
		{"EEPROM", "EEPROM_V124", SaveEEPROM},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectSaveType([]byte("prefix-" + tt.marker + "-suffix"))
			if got.Type != tt.want || got.Ambiguous {
				t.Fatalf("DetectSaveType(%q) = type=%s ambiguous=%v, want %s/false",
					tt.marker, got.Type, got.Ambiguous, tt.want)
			}
			if len(got.Markers) != 1 {
				t.Fatalf("markers = %v, want one marker", got.Markers)
			}
		})
	}
}

func TestDetectSaveTypeNoMarker(t *testing.T) {
	got := DetectSaveType([]byte("ordinary ROM data"))
	if got.Type != SaveNone || got.Ambiguous || len(got.Markers) != 0 {
		t.Fatalf("no-marker detection = %+v, want SaveNone", got)
	}
}

func TestDetectSaveTypeAllowsAliasesForSameDevice(t *testing.T) {
	got := DetectSaveType([]byte("SRAM_V110...SRAM_F_V102"))
	if got.Type != SaveSRAM || got.Ambiguous {
		t.Fatalf("same-device aliases = %+v, want SRAM/non-ambiguous", got)
	}
	wantMarkers := []string{"SRAM_F_V", "SRAM_V"}
	if !reflect.DeepEqual(got.Markers, wantMarkers) {
		t.Fatalf("markers = %v, want %v", got.Markers, wantMarkers)
	}
}

func TestDetectSaveTypeRejectsConflictingMarkers(t *testing.T) {
	got := DetectSaveType([]byte("FLASH1M_V103...EEPROM_V124"))
	if !got.Ambiguous || got.Type != SaveAuto {
		t.Fatalf("conflicting detection = %+v, want ambiguous SaveAuto", got)
	}
	if len(got.Markers) != 2 {
		t.Fatalf("conflicting markers = %v, want two", got.Markers)
	}
}

func TestConfigureAutoAttachesDetectedSRAM(t *testing.T) {
	b := bus.New(nil, []byte("header SRAM_V113 trailer"))
	setup := Configure(b, []byte("header SRAM_V113 trailer"), Config{})

	if setup.SaveType != SaveSRAM || setup.Explicit || setup.Save == nil || setup.EEPROM != nil {
		t.Fatalf("SRAM setup = %+v", setup)
	}

	b.Write8(bus.SaveStart+0x1234, 0x5a, bus.Access{})
	if got, _ := b.Read8(bus.SaveStart+0x1234, bus.Access{}); got != 0x5a {
		t.Fatalf("configured SRAM read = %02x, want 5a", got)
	}
}

func TestConfigureExplicitOverrideWinsOverDetectionConflict(t *testing.T) {
	rom := []byte("FLASH1M_V103...EEPROM_V124")
	b := bus.New(nil, rom)
	setup := Configure(b, rom, Config{SaveType: SaveFlash64K})

	if !setup.Detected.Ambiguous {
		t.Fatal("test ROM should remain reported as ambiguous")
	}
	if !setup.Explicit || setup.SaveType != SaveFlash64K {
		t.Fatalf("override setup = %+v, want explicit flash64", setup)
	}
	flash, ok := setup.Save.(*Flash)
	if !ok || len(flash.data) != Flash64KSize {
		t.Fatalf("override device = %T, want 64 KiB Flash", setup.Save)
	}
}

func TestConfigureGenericEEPROMUsesDocumented8KFallback(t *testing.T) {
	rom := []byte("EEPROM_V124")
	b := bus.New(nil, rom)
	setup := Configure(b, rom, Config{})

	if setup.SaveType != SaveEEPROM8K || !setup.Fallback || setup.Explicit {
		t.Fatalf("generic EEPROM setup = %+v", setup)
	}
	eeprom, ok := setup.EEPROM.(*EEPROM)
	if !ok || len(eeprom.data) != EEPROM8KSize {
		t.Fatalf("fallback device = %T, want 8 KiB EEPROM", setup.EEPROM)
	}
	if got, _ := b.Read16(bus.EEPROMStart, bus.Access{}); got != 1 {
		t.Fatalf("configured EEPROM idle bit = %04x, want 0001", got)
	}
}

func TestConfigureExplicitEEPROM512BOverridesGenericMarker(t *testing.T) {
	rom := []byte("EEPROM_V124")
	b := bus.New(nil, rom)
	setup := Configure(b, rom, Config{SaveType: SaveEEPROM512B})

	if setup.SaveType != SaveEEPROM512B || setup.Fallback || !setup.Explicit {
		t.Fatalf("explicit small EEPROM setup = %+v", setup)
	}
	eeprom, ok := setup.EEPROM.(*EEPROM)
	if !ok || len(eeprom.data) != EEPROM512BSize {
		t.Fatalf("explicit device = %T, want 512-byte EEPROM", setup.EEPROM)
	}
}

func TestConfigureAmbiguousAutoAttachesNoSaveDevice(t *testing.T) {
	rom := []byte("SRAM_V113...FLASH1M_V103")
	b := bus.New(nil, rom)
	setup := Configure(b, rom, Config{})

	if setup.SaveType != SaveNone || !setup.Detected.Ambiguous || setup.Save != nil || setup.EEPROM != nil {
		t.Fatalf("ambiguous auto setup = %+v, want no attached save", setup)
	}
}

func TestSaveTypeString(t *testing.T) {
	tests := map[SaveType]string{
		SaveAuto:       "auto",
		SaveNone:       "none",
		SaveSRAM:       "sram",
		SaveFlash64K:   "flash-64k",
		SaveFlash128K:  "flash-128k",
		SaveEEPROM:     "eeprom",
		SaveEEPROM512B: "eeprom-512b",
		SaveEEPROM8K:   "eeprom-8k",
	}
	for saveType, want := range tests {
		if got := saveType.String(); got != want {
			t.Fatalf("%d.String() = %q, want %q", saveType, got, want)
		}
	}
}
