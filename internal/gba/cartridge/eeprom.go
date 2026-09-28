package cartridge

const (
	// EEPROM512BSize is the capacity of the small GBA serial EEPROM part.
	EEPROM512BSize = 512
	// EEPROM8KSize is the capacity of the large GBA serial EEPROM part.
	EEPROM8KSize = 8 * 1024

	// EEPROMWriteSettleCycles is the write-cycle busy interval exposed by the
	// serial ready/busy bit after a successful 64-bit block write.
	//
	// Real parts vary. This value matches the established mGBA estimate and,
	// importantly, models the hardware-visible low-ready interval instead of
	// making EEPROM writes appear instantaneous.
	EEPROMWriteSettleCycles uint32 = 115000
)

type eepromMode uint8

const (
	eepromWait eepromMode = iota
	eepromCommand
	eepromAddress
	eepromWriteData
	eepromWriteStop
	eepromReadStop
	eepromReadData
)

// EEPROM models the 1-bit serial save protocol used by GBA EEPROM cartridges.
// The chip is organized as 8-byte blocks. Small parts use six transmitted
// address bits; large parts use fourteen transmitted address bits.
type EEPROM struct {
	data        []byte
	addressBits uint8

	mode       eepromMode
	read       bool
	address    uint32
	bitCount   uint8
	writeData  uint64
	readBit    uint8
	busyCycles uint32
}

// NewEEPROM512B creates a blank 512-byte EEPROM using six address bits.
func NewEEPROM512B() *EEPROM { return newEEPROM(EEPROM512BSize, 6) }

// NewEEPROM8K creates a blank 8-KiB EEPROM using fourteen address bits.
func NewEEPROM8K() *EEPROM { return newEEPROM(EEPROM8KSize, 14) }

func newEEPROM(size int, addressBits uint8) *EEPROM {
	e := &EEPROM{
		data:        make([]byte, size),
		addressBits: addressBits,
		mode:        eepromWait,
	}
	for i := range e.data {
		e.data[i] = 0xff
	}
	return e
}

// Advance advances the EEPROM's internal write-cycle timer in GBA master-clock
// cycles. While a write is settling the serial output line reports busy (0)
// and new commands are ignored.
func (e *EEPROM) Advance(cycles uint32) {
	if cycles >= e.busyCycles {
		e.busyCycles = 0
		return
	}
	e.busyCycles -= cycles
}

// WriteBit clocks one serial input bit into the EEPROM. Only bit 0 is
// significant; callers may pass a full bus byte/halfword reduced to byte.
func (e *EEPROM) WriteBit(value byte) {
	if e.busyCycles != 0 {
		return
	}

	bit := value & 1

	switch e.mode {
	case eepromWait:
		// Every command begins with a start bit of 1.
		if bit == 1 {
			e.mode = eepromCommand
		}

	case eepromCommand:
		// The second command bit selects write (10) or read (11).
		e.read = bit == 1
		e.address = 0
		e.bitCount = 0
		e.writeData = 0
		e.mode = eepromAddress

	case eepromAddress:
		e.address = e.address<<1 | uint32(bit)
		e.bitCount++
		if e.bitCount == e.addressBits {
			e.bitCount = 0
			if e.read {
				e.mode = eepromReadStop
			} else {
				e.mode = eepromWriteData
			}
		}

	case eepromWriteData:
		e.writeData = e.writeData<<1 | uint64(bit)
		e.bitCount++
		if e.bitCount == 64 {
			e.bitCount = 0
			e.mode = eepromWriteStop
		}

	case eepromWriteStop:
		// A valid transfer terminates with a zero stop bit. Buffering the full
		// payload until here prevents malformed commands from partially writing.
		if bit == 0 && e.commitWrite() {
			e.busyCycles = EEPROMWriteSettleCycles
		}
		e.resetProtocol()

	case eepromReadStop:
		// Read commands also terminate their input phase with a zero stop bit.
		if bit == 0 {
			e.readBit = 0
			e.mode = eepromReadData
		} else {
			e.resetProtocol()
		}

	case eepromReadData:
		// Writes during the output phase are ignored until all 68 output bits
		// (four dummy bits plus 64 data bits) have been shifted out.
	}
}

// ReadBit clocks one serial output bit from the EEPROM. During the EEPROM's
// internal write cycle the line is low (busy). Otherwise an idle line is high
// (ready), and an active read shifts four dummy bits plus 64 data bits.
func (e *EEPROM) ReadBit() byte {
	if e.busyCycles != 0 {
		return 0
	}
	if e.mode != eepromReadData {
		return 1
	}

	index := e.readBit
	e.readBit++

	var bit byte
	if index >= 4 {
		dataBit := index - 4
		if block := e.blockOffset(); block >= 0 {
			value := e.data[block+int(dataBit>>3)]
			bit = (value >> (7 - (dataBit & 7))) & 1
		} else {
			// Out-of-range addresses behave like erased storage.
			bit = 1
		}
	}

	if e.readBit == 68 {
		e.resetProtocol()
	}
	return bit
}

func (e *EEPROM) commitWrite() bool {
	block := e.blockOffset()
	if block < 0 {
		return false
	}
	for i := 0; i < 8; i++ {
		shift := uint(56 - i*8)
		e.data[block+i] = byte(e.writeData >> shift)
	}
	return true
}

func (e *EEPROM) blockOffset() int {
	blocks := uint32(len(e.data) / 8)
	if e.address >= blocks {
		return -1
	}
	return int(e.address * 8)
}

func (e *EEPROM) resetProtocol() {
	e.mode = eepromWait
	e.read = false
	e.address = 0
	e.bitCount = 0
	e.writeData = 0
	e.readBit = 0
}
