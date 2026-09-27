package cartridge

import (
	"bytes"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

// Peripheral identifies optional hardware connected to the Game Pak GPIO port.
type Peripheral uint8

const (
	// PeripheralRTC is an S-3511A-compatible real-time clock.
	PeripheralRTC Peripheral = 1 << iota
)

// DetectPeripherals finds standard cartridge-library markers without relying
// on game-specific IDs.
func DetectPeripherals(rom []byte) Peripheral {
	var peripherals Peripheral
	if bytes.Contains(rom, []byte("SIIRTC_V")) {
		peripherals |= PeripheralRTC
	}
	return peripherals
}

// GPIO implements the cartridge 4-bit general-purpose I/O registers at
// 0x080000C4/0x080000C6/0x080000C8. Three pins are used by the RTC; the fourth
// remains available for future rumble/sensor devices.
type GPIO struct {
	peripherals Peripheral
	data        byte
	direction   byte
	readEnabled bool
	rtc         *RTC
}

// NewGPIO creates the GPIO device set requested by peripherals.
func NewGPIO(peripherals Peripheral) *GPIO {
	g := &GPIO{peripherals: peripherals}
	if peripherals&PeripheralRTC != 0 {
		g.rtc = NewRTC()
	}
	return g
}

func newGPIOWithRTC(peripherals Peripheral, rtc *RTC) *GPIO {
	g := &GPIO{peripherals: peripherals, rtc: rtc}
	if peripherals&PeripheralRTC != 0 && g.rtc == nil {
		g.rtc = NewRTC()
	}
	return g
}

// Peripherals returns the optional cartridge hardware connected to this GPIO.
func (g *GPIO) Peripherals() Peripheral { return g.peripherals }

// RTC returns the attached real-time clock, or nil when the cartridge has none.
func (g *GPIO) RTC() *RTC { return g.rtc }

// Read8 implements bus.GamePakDevice. When GPIO reads are disabled, the data
// and direction registers fall through to the ROM bytes beneath them.
func (g *GPIO) Read8(addr uint32) (byte, bool) {
	switch addr {
	case bus.GPIODataAddress:
		if !g.readEnabled {
			return 0, false
		}
		value := g.data & 0x0f
		if g.rtc != nil && g.direction&0x02 == 0 {
			value = value&^0x02 | g.rtc.DataBit()<<1
		}
		return value, true
	case bus.GPIODataAddress + 1:
		return g.highByteVisible()
	case bus.GPIODirectionAddress:
		if !g.readEnabled {
			return 0, false
		}
		return g.direction & 0x0f, true
	case bus.GPIODirectionAddress + 1:
		return g.highByteVisible()
	case bus.GPIOControlAddress:
		if g.readEnabled {
			return 1, true
		}
		return 0, true
	case bus.GPIOControlAddress + 1:
		return 0, true
	default:
		return 0, false
	}
}

func (g *GPIO) highByteVisible() (byte, bool) {
	if !g.readEnabled {
		return 0, false
	}
	return 0, true
}

// Write8 implements bus.GamePakDevice.
func (g *GPIO) Write8(addr uint32, value byte) bool {
	switch addr {
	case bus.GPIODataAddress:
		previous := g.data & 0x0f
		g.data = value & 0x0f
		if g.rtc != nil && g.readEnabled {
			g.rtc.WritePins(previous, g.data, g.direction)
		}
		return true
	case bus.GPIODataAddress + 1:
		return true
	case bus.GPIODirectionAddress:
		g.direction = value & 0x0f
		return true
	case bus.GPIODirectionAddress + 1:
		return true
	case bus.GPIOControlAddress:
		g.readEnabled = value&1 != 0
		return true
	case bus.GPIOControlAddress + 1:
		return true
	default:
		return false
	}
}

var _ bus.GamePakDevice = (*GPIO)(nil)
