package cartridge

const (
	// Flash64KSize is the capacity of a 512-kbit GBA flash save chip.
	Flash64KSize = 64 * 1024
	// Flash128KSize is the capacity of a 1-Mbit GBA flash save chip.
	Flash128KSize = 128 * 1024

	flashUnlock1Addr uint16 = 0x5555
	flashUnlock2Addr uint16 = 0x2aaa
	flashSectorSize         = 4 * 1024
)

const (
	flashUnlock1Value byte = 0xaa
	flashUnlock2Value byte = 0x55

	flashCmdChipErase   byte = 0x10
	flashCmdSectorErase byte = 0x30
	flashCmdErase       byte = 0x80
	flashCmdID          byte = 0x90
	flashCmdProgram     byte = 0xa0
	flashCmdBank        byte = 0xb0
	flashCmdReset       byte = 0xf0
)

type flashPhase uint8

const (
	flashPhaseIdle flashPhase = iota
	flashPhaseUnlock1
	flashPhaseUnlock2
)

// Flash models the byte-wide command protocol used by standard GBA 64 KiB
// and 128 KiB flash save chips.
//
// Both parts expose a 64 KiB address window. The 128 KiB variant selects one
// of two banks with the B0 command.
type Flash struct {
	data           []byte
	bank           byte
	manufacturerID byte
	deviceID       byte
	phase          flashPhase
	command        byte
}

// NewFlash64K creates a 64 KiB flash device.
//
// The ID bytes identify a Panasonic MN63F805MNP-compatible 512-kbit part,
// matching a commonly emulated GBA flash device.
func NewFlash64K() *Flash {
	return newFlash(Flash64KSize, 0x32, 0x1b)
}

// NewFlash128K creates a 128 KiB flash device.
//
// The ID bytes identify a Sanyo LE26FV10N1TS-compatible 1-Mbit part.
func NewFlash128K() *Flash {
	return newFlash(Flash128KSize, 0x62, 0x13)
}

func newFlash(size int, manufacturerID, deviceID byte) *Flash {
	f := &Flash{
		data:           make([]byte, size),
		manufacturerID: manufacturerID,
		deviceID:       deviceID,
	}
	for i := range f.data {
		f.data[i] = 0xff
	}
	return f
}

// Read8 reads one byte from the selected flash bank. Addresses mirror through
// the Game Pak save window because the chip only decodes the low 16 bits.
func (f *Flash) Read8(addr uint32) byte {
	a := uint16(addr)
	if f.command == flashCmdID {
		switch a {
		case 0:
			return f.manufacturerID
		case 1:
			return f.deviceID
		}
	}
	return f.data[f.bankOffset()+int(a)]
}

// Write8 feeds one byte into the flash command state machine.
func (f *Flash) Write8(addr uint32, value byte) {
	a := uint16(addr)

	// Program and bank-select commands consume the very next write.
	switch f.command {
	case flashCmdProgram:
		f.data[f.bankOffset()+int(a)] = value
		f.command = 0
		f.phase = flashPhaseIdle
		return
	case flashCmdBank:
		if len(f.data) == Flash128KSize && a == 0 && value < 2 {
			f.bank = value
		}
		f.command = 0
		f.phase = flashPhaseIdle
		return
	}

	// Flash reset/read-array is accepted from the idle phase without requiring
	// another unlock sequence. Games also commonly send it as AA/55/F0.
	if value == flashCmdReset && f.phase == flashPhaseIdle {
		f.command = 0
		return
	}

	switch f.phase {
	case flashPhaseIdle:
		if a == flashUnlock1Addr && value == flashUnlock1Value {
			f.phase = flashPhaseUnlock1
		}

	case flashPhaseUnlock1:
		if a == flashUnlock2Addr && value == flashUnlock2Value {
			f.phase = flashPhaseUnlock2
			return
		}

		// A malformed sequence is discarded. A repeated first unlock byte can
		// immediately begin a fresh sequence.
		f.phase = flashPhaseIdle
		if a == flashUnlock1Addr && value == flashUnlock1Value {
			f.phase = flashPhaseUnlock1
		}

	case flashPhaseUnlock2:
		f.phase = flashPhaseIdle

		// Erase is a two-unlock command: AA/55/80, AA/55, then 10 at 5555
		// for chip erase or 30 at an address in the target 4 KiB sector.
		if f.command == flashCmdErase {
			switch {
			case a == flashUnlock1Addr && value == flashCmdChipErase:
				f.eraseChip()
				f.command = 0
			case value == flashCmdSectorErase:
				f.eraseSector(a)
				f.command = 0
			}
			return
		}

		if a != flashUnlock1Addr {
			return
		}

		switch value {
		case flashCmdErase, flashCmdID, flashCmdProgram:
			f.command = value
		case flashCmdBank:
			if len(f.data) == Flash128KSize {
				f.command = value
			}
		case flashCmdReset:
			f.command = 0
		}
	}
}

func (f *Flash) bankOffset() int {
	if len(f.data) == Flash128KSize {
		return int(f.bank) * Flash64KSize
	}
	return 0
}

func (f *Flash) eraseChip() {
	for i := range f.data {
		f.data[i] = 0xff
	}
}

func (f *Flash) eraseSector(addr uint16) {
	start := f.bankOffset() + int(addr&^(flashSectorSize-1))
	for i := 0; i < flashSectorSize; i++ {
		f.data[start+i] = 0xff
	}
}
