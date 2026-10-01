package cartridge

import (
	"fmt"
	"time"
)

// FlashState captures both persistent bytes and the in-flight command parser.
type FlashState struct {
	Data    []byte
	Bank    byte
	Phase   uint8
	Command byte
}

// EEPROMState captures persistent bytes and the serial protocol/busy timer.
type EEPROMState struct {
	Data        []byte
	AddressBits uint8
	Mode        uint8
	Read        bool
	Address     uint32
	BitCount    uint8
	WriteData   uint64
	ReadBit     uint8
	BusyCycles  uint32
}

// RTCState captures the emulated clock value and in-flight serial transfer.
type RTCState struct {
	UnixNano   int64
	Control    byte
	Serial     uint8
	Command    byte
	CommandBit int
	BitIndex   int
	Input      []byte
	Output     []byte
	WriteCmd   byte
}

// GPIOState captures cartridge GPIO/RTC protocol state.
type GPIOState struct {
	Peripherals Peripheral
	Data        byte
	Direction   byte
	ReadEnabled bool
	RTC         *RTCState
}

// State is the mutable cartridge-side state for a configured machine.
type State struct {
	SaveType SaveType
	SRAM     []byte
	Flash    *FlashState
	EEPROM   *EEPROMState
	GPIO     *GPIOState
}

func copyBytes(dst, src []byte) []byte {
	if cap(dst) < len(src) {
		dst = make([]byte, len(src))
	} else {
		dst = dst[:len(src)]
	}
	copy(dst, src)
	return dst
}

func (r *RTC) snapshotInto(dst *RTCState) {
	if dst == nil {
		return
	}
	dst.UnixNano = r.CurrentTime().UnixNano()
	dst.Control = r.control
	dst.Serial = uint8(r.state)
	dst.Command = r.command
	dst.CommandBit = r.commandBit
	dst.BitIndex = r.bitIndex
	dst.Input = copyBytes(dst.Input, r.input)
	dst.Output = copyBytes(dst.Output, r.output)
	dst.WriteCmd = r.writeCmd
}

func (r *RTC) restoreState(s RTCState) {
	now := r.now()
	target := time.Unix(0, s.UnixNano).In(now.Location())
	r.offset = target.Sub(now)
	r.control = s.Control
	r.state = rtcSerialState(s.Serial)
	r.command = s.Command
	r.commandBit = s.CommandBit
	r.bitIndex = s.BitIndex
	r.input = copyBytes(r.input, s.Input)
	r.output = copyBytes(r.output, s.Output)
	r.writeCmd = s.WriteCmd
}

// Snapshot captures cartridge protocol state.
func (s Setup) Snapshot() State {
	var out State
	s.SnapshotInto(&out)
	return out
}

// SnapshotInto captures cartridge state while reusing variable buffers.
func (s Setup) SnapshotInto(out *State) {
	if out == nil {
		return
	}
	out.SaveType = s.SaveType
	out.SRAM = out.SRAM[:0]
	out.Flash = nil
	out.EEPROM = nil
	out.GPIO = nil

	switch dev := s.Save.(type) {
	case *SRAM:
		out.SRAM = copyBytes(out.SRAM, dev.data[:])
	case *Flash:
		out.Flash = &FlashState{
			Data: copyBytes(nil, dev.data), Bank: dev.bank,
			Phase: uint8(dev.phase), Command: dev.command,
		}
	}
	if dev, ok := s.EEPROM.(*EEPROM); ok {
		out.EEPROM = &EEPROMState{
			Data: copyBytes(nil, dev.data), AddressBits: dev.addressBits,
			Mode: uint8(dev.mode), Read: dev.read, Address: dev.address,
			BitCount: dev.bitCount, WriteData: dev.writeData, ReadBit: dev.readBit,
			BusyCycles: dev.busyCycles,
		}
	}
	if s.GPIO != nil {
		gpio := &GPIOState{
			Peripherals: s.GPIO.peripherals, Data: s.GPIO.data,
			Direction: s.GPIO.direction, ReadEnabled: s.GPIO.readEnabled,
		}
		if s.GPIO.rtc != nil {
			gpio.RTC = &RTCState{}
			s.GPIO.rtc.snapshotInto(gpio.RTC)
		}
		out.GPIO = gpio
	}
}

// Restore restores cartridge state into the already-configured devices.
func (s Setup) Restore(st State) error {
	if st.SaveType != s.SaveType {
		return fmt.Errorf("gba cartridge: state save type %s does not match cartridge %s", st.SaveType, s.SaveType)
	}

	switch dev := s.Save.(type) {
	case nil:
		if len(st.SRAM) != 0 || st.Flash != nil {
			return fmt.Errorf("gba cartridge: state contains byte-wide save hardware for save type %s", s.SaveType)
		}
	case *SRAM:
		if len(st.SRAM) != len(dev.data) {
			return fmt.Errorf("gba cartridge: SRAM state size %d, want %d", len(st.SRAM), len(dev.data))
		}
		copy(dev.data[:], st.SRAM)
	case *Flash:
		if st.Flash == nil || len(st.Flash.Data) != len(dev.data) {
			return fmt.Errorf("gba cartridge: incompatible flash state")
		}
		copy(dev.data, st.Flash.Data)
		dev.bank = st.Flash.Bank
		dev.phase = flashPhase(st.Flash.Phase)
		dev.command = st.Flash.Command
	default:
		return fmt.Errorf("gba cartridge: unsupported save device %T", s.Save)
	}

	if dev, ok := s.EEPROM.(*EEPROM); ok {
		if st.EEPROM == nil || len(st.EEPROM.Data) != len(dev.data) || st.EEPROM.AddressBits != dev.addressBits {
			return fmt.Errorf("gba cartridge: incompatible EEPROM state")
		}
		copy(dev.data, st.EEPROM.Data)
		dev.mode = eepromMode(st.EEPROM.Mode)
		dev.read = st.EEPROM.Read
		dev.address = st.EEPROM.Address
		dev.bitCount = st.EEPROM.BitCount
		dev.writeData = st.EEPROM.WriteData
		dev.readBit = st.EEPROM.ReadBit
		dev.busyCycles = st.EEPROM.BusyCycles
	} else if st.EEPROM != nil {
		return fmt.Errorf("gba cartridge: state contains EEPROM for cartridge without EEPROM")
	}

	if s.GPIO == nil {
		if st.GPIO != nil {
			return fmt.Errorf("gba cartridge: state contains GPIO for cartridge without GPIO")
		}
		return nil
	}
	if st.GPIO == nil || st.GPIO.Peripherals != s.GPIO.peripherals {
		return fmt.Errorf("gba cartridge: incompatible GPIO state")
	}
	s.GPIO.data = st.GPIO.Data & 0x0f
	s.GPIO.direction = st.GPIO.Direction & 0x0f
	s.GPIO.readEnabled = st.GPIO.ReadEnabled
	if s.GPIO.rtc != nil {
		if st.GPIO.RTC == nil {
			return fmt.Errorf("gba cartridge: state is missing RTC state")
		}
		s.GPIO.rtc.restoreState(*st.GPIO.RTC)
	} else if st.GPIO.RTC != nil {
		return fmt.Errorf("gba cartridge: state contains RTC for cartridge without RTC")
	}
	return nil
}
