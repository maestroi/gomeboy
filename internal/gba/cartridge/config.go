package cartridge

import (
	"bytes"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

// SaveType identifies GBA cartridge save hardware.
type SaveType uint8

const (
	// SaveAuto selects save hardware from ROM library markers.
	SaveAuto SaveType = iota
	// SaveNone disables cartridge save hardware.
	SaveNone
	// SaveSRAM selects the standard 32 KiB SRAM device.
	SaveSRAM
	// SaveFlash64K selects a 512-kbit / 64 KiB flash device.
	SaveFlash64K
	// SaveFlash128K selects a 1-Mbit / 128 KiB flash device.
	SaveFlash128K
	// SaveEEPROM identifies EEPROM when ROM markers do not encode capacity.
	// Configuration resolves this generic type to the documented 8 KiB
	// fallback unless an exact EEPROM size is explicitly selected.
	SaveEEPROM
	// SaveEEPROM512B selects the small 4-kbit / 512-byte EEPROM device.
	SaveEEPROM512B
	// SaveEEPROM8K selects the large 64-kbit / 8-KiB EEPROM device.
	SaveEEPROM8K
)

func (s SaveType) String() string {
	switch s {
	case SaveAuto:
		return "auto"
	case SaveNone:
		return "none"
	case SaveSRAM:
		return "sram"
	case SaveFlash64K:
		return "flash-64k"
	case SaveFlash128K:
		return "flash-128k"
	case SaveEEPROM:
		return "eeprom"
	case SaveEEPROM512B:
		return "eeprom-512b"
	case SaveEEPROM8K:
		return "eeprom-8k"
	default:
		return "unknown"
	}
}

// Config selects cartridge save hardware. SaveAuto scans the ROM for standard
// Nintendo/SDK save-library markers. Exact values always override detection.
type Config struct {
	SaveType SaveType
}

// Detection reports the marker-derived save type. Ambiguous is set when a ROM
// contains markers for multiple incompatible save devices; callers should use
// an explicit Config in that case instead of guessing.
type Detection struct {
	Type      SaveType
	Markers   []string
	Ambiguous bool
}

// Setup is the resolved cartridge save configuration attached to a bus.
//
// Save and EEPROM intentionally retain their concrete bus-facing interfaces so
// the persistence layer can reuse the selected device in the next slice.
type Setup struct {
	SaveType  SaveType
	Detected  Detection
	Explicit  bool
	Fallback  bool
	Save      bus.SaveDevice
	EEPROM    bus.EEPROMDevice
}

type markerDefinition struct {
	marker string
	save   SaveType
}

var saveMarkers = []markerDefinition{
	{marker: "EEPROM_V", save: SaveEEPROM},
	{marker: "SRAM_F_V", save: SaveSRAM},
	{marker: "SRAM_V", save: SaveSRAM},
	{marker: "FLASH1M_V", save: SaveFlash128K},
	{marker: "FLASH512_V", save: SaveFlash64K},
	{marker: "FLASH_V", save: SaveFlash64K},
}

// DetectSaveType scans a ROM for the standard save-library identification
// strings embedded by common GBA development libraries.
func DetectSaveType(rom []byte) Detection {
	detection := Detection{Type: SaveNone}
	var foundType SaveType
	for _, definition := range saveMarkers {
		if !bytes.Contains(rom, []byte(definition.marker)) {
			continue
		}
		detection.Markers = append(detection.Markers, definition.marker)
		if foundType == SaveAuto {
			foundType = definition.save
			continue
		}
		if foundType != definition.save {
			detection.Type = SaveAuto
			detection.Ambiguous = true
		}
	}
	if detection.Ambiguous {
		return detection
	}
	if foundType != SaveAuto {
		detection.Type = foundType
	}
	return detection
}

// Configure resolves save detection/override policy, creates the selected save
// device, and attaches it to the correct cartridge bus boundary.
//
// EEPROM_V identifies EEPROM technology but not its capacity. Auto/generic
// EEPROM therefore uses the 8 KiB device as a documented compatibility
// fallback; callers that know the cartridge uses the small device should pass
// SaveEEPROM512B explicitly.
func Configure(b *bus.Bus, rom []byte, config Config) Setup {
	if b == nil {
		panic("gba cartridge: nil bus")
	}

	detection := DetectSaveType(rom)
	selected := config.SaveType
	explicit := selected != SaveAuto
	if !explicit {
		if detection.Ambiguous {
			selected = SaveNone
		} else {
			selected = detection.Type
		}
	}

	setup := Setup{
		SaveType: selected,
		Detected: detection,
		Explicit: explicit,
	}

	if selected == SaveEEPROM {
		selected = SaveEEPROM8K
		setup.SaveType = selected
		setup.Fallback = true
	}

	switch selected {
	case SaveNone:
		// No save hardware.
	case SaveSRAM:
		setup.Save = NewSRAM()
	case SaveFlash64K:
		setup.Save = NewFlash64K()
	case SaveFlash128K:
		setup.Save = NewFlash128K()
	case SaveEEPROM512B:
		setup.EEPROM = NewEEPROM512B()
	case SaveEEPROM8K:
		setup.EEPROM = NewEEPROM8K()
	default:
		// Unknown explicit values are treated as no save hardware. This keeps
		// configuration deterministic and avoids attaching the wrong protocol.
		setup.SaveType = SaveNone
	}

	b.AttachSaveDevice(setup.Save)
	b.AttachEEPROMDevice(setup.EEPROM)
	return setup
}
