package cartridge

import (
	"reflect"
	"testing"
	"time"
)

func TestStateRoundTripFlashProtocol(t *testing.T) {
	flash := NewFlash128K()
	setup := Setup{SaveType: SaveFlash128K, Save: flash}
	flash.data[0x10002] = 0x5a
	flash.bank = 1
	flash.phase = flashPhaseUnlock2
	flash.command = flashCmdProgram

	want := setup.Snapshot()
	flash.data[0x10002] = 0xff
	flash.bank = 0
	flash.phase = flashPhaseIdle
	flash.command = 0

	if err := setup.Restore(want); err != nil {
		t.Fatal(err)
	}
	if got := setup.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("flash state after restore = %#v, want %#v", got, want)
	}
}

func TestStateRoundTripEEPROMAndRTCProtocol(t *testing.T) {
	fixed := time.Date(2026, 10, 1, 12, 30, 0, 0, time.UTC)
	rtc := newRTC(func() time.Time { return fixed })
	gpio := newGPIOWithRTC(PeripheralRTC, rtc)
	eeprom := NewEEPROM512B()
	setup := Setup{
		SaveType: SaveEEPROM512B, EEPROM: eeprom,
		Peripherals: PeripheralRTC, GPIO: gpio,
	}

	eeprom.data[8] = 0x7c
	eeprom.mode = eepromWriteData
	eeprom.read = false
	eeprom.address = 3
	eeprom.bitCount = 17
	eeprom.writeData = 0x123456
	eeprom.busyCycles = 321

	gpio.data = 0x05
	gpio.direction = 0x07
	gpio.readEnabled = true
	rtc.control = 0x40
	rtc.state = rtcInput
	rtc.command = rtcCommandWriteTime
	rtc.commandBit = -1
	rtc.bitIndex = 7
	rtc.input = []byte{0x12, 0x34, 0}
	rtc.writeCmd = rtcCommandWriteTime

	want := setup.Snapshot()
	eeprom.resetProtocol()
	eeprom.data[8] = 0xff
	eeprom.busyCycles = 0
	gpio.data = 0
	gpio.direction = 0
	gpio.readEnabled = false
	rtc.resetSerial()
	rtc.control = 0

	if err := setup.Restore(want); err != nil {
		t.Fatal(err)
	}
	if got := setup.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("EEPROM/RTC state after restore = %#v, want %#v", got, want)
	}
}
