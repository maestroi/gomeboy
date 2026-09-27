package cartridge

import "fmt"

// PersistentDevice is cartridge save storage that can be loaded from and
// snapshotted to a conventional raw .sav payload.
type PersistentDevice interface {
	SaveSize() int
	SaveData() []byte
	LoadSaveData([]byte) error
}

// PersistentDevice returns the selected SRAM, Flash, or EEPROM storage device.
// SaveNone cartridges return nil.
func (s Setup) PersistentDevice() PersistentDevice {
	if device, ok := s.Save.(PersistentDevice); ok {
		return device
	}
	if device, ok := s.EEPROM.(PersistentDevice); ok {
		return device
	}
	return nil
}

// Advance advances cartridge-side storage timers on the GBA master clock.
func (s Setup) Advance(cycles uint32) {
	if clocked, ok := s.EEPROM.(interface{ Advance(uint32) }); ok {
		clocked.Advance(cycles)
	}
}

func saveData(data []byte) []byte {
	return append([]byte(nil), data...)
}

func loadSaveData(dst, src []byte) error {
	if len(src) != len(dst) {
		return fmt.Errorf("gba cartridge: save size %d bytes does not match selected hardware size %d", len(src), len(dst))
	}
	copy(dst, src)
	return nil
}

func (s *SRAM) SaveSize() int { return len(s.data) }

func (s *SRAM) SaveData() []byte { return saveData(s.data[:]) }

func (s *SRAM) LoadSaveData(data []byte) error {
	return loadSaveData(s.data[:], data)
}

func (f *Flash) SaveSize() int { return len(f.data) }

func (f *Flash) SaveData() []byte { return saveData(f.data) }

func (f *Flash) LoadSaveData(data []byte) error {
	return loadSaveData(f.data, data)
}

func (e *EEPROM) SaveSize() int { return len(e.data) }

func (e *EEPROM) SaveData() []byte { return saveData(e.data) }

func (e *EEPROM) LoadSaveData(data []byte) error {
	if err := loadSaveData(e.data, data); err != nil {
		return err
	}
	e.busyCycles = 0
	e.resetProtocol()
	return nil
}
