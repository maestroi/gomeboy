package cartridge

import (
	"testing"
	"time"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func rtcWriteReg(b *bus.Bus, addr uint32, value byte) {
	b.Write16(addr, uint16(value), bus.Access{})
}

func rtcBeginCommand(b *bus.Bus, command byte) {
	rtcWriteReg(b, bus.GPIOControlAddress, 1)
	rtcWriteReg(b, bus.GPIODirectionAddress, 0x7)
	rtcWriteReg(b, bus.GPIODataAddress, 0x1)
	rtcWriteReg(b, bus.GPIODataAddress, 0x5)
	for bit := 7; bit >= 0; bit-- {
		value := byte(0x4 | ((command>>uint(bit))&1)<<1)
		rtcWriteReg(b, bus.GPIODataAddress, value)
		rtcWriteReg(b, bus.GPIODataAddress, value|1)
	}
}

func rtcReadBytes(b *bus.Bus, count int) []byte {
	rtcWriteReg(b, bus.GPIODirectionAddress, 0x5) // SIO becomes RTC input.
	out := make([]byte, count)
	for bit := 0; bit < count*8; bit++ {
		rtcWriteReg(b, bus.GPIODataAddress, 0x4)
		value, _ := b.Read16(bus.GPIODataAddress, bus.Access{})
		out[bit>>3] |= byte((value>>1)&1) << uint(bit&7)
		rtcWriteReg(b, bus.GPIODataAddress, 0x5)
	}
	return out
}

func rtcWriteBytes(b *bus.Bus, data []byte) {
	rtcWriteReg(b, bus.GPIODirectionAddress, 0x7)
	for bit := 0; bit < len(data)*8; bit++ {
		value := byte(0x4 | ((data[bit>>3]>>uint(bit&7))&1)<<1)
		rtcWriteReg(b, bus.GPIODataAddress, value)
		rtcWriteReg(b, bus.GPIODataAddress, value|1)
	}
}

func fixedRTCBus(t *testing.T, now time.Time) (*bus.Bus, *RTC) {
	t.Helper()
	rom := make([]byte, 0x200)
	b := bus.New(nil, rom)
	rtc := newRTC(func() time.Time { return now })
	gpio := newGPIOWithRTC(PeripheralRTC, rtc)
	b.AttachGamePakDevice(gpio)
	return b, rtc
}

func TestDetectPeripheralsRTCMarker(t *testing.T) {
	if got := DetectPeripherals([]byte("prefix SIIRTC_V001 suffix")); got != PeripheralRTC {
		t.Fatalf("DetectPeripherals = %02x, want RTC", got)
	}
	if got := DetectPeripherals([]byte("ordinary ROM")); got != 0 {
		t.Fatalf("DetectPeripherals(no marker) = %02x, want none", got)
	}
}

func TestGPIOReadEnableExposesRegistersOverROM(t *testing.T) {
	rom := make([]byte, 0x200)
	rom[0xc4] = 0xa5
	rom[0xc5] = 0x5a
	b := bus.New(nil, rom)
	b.AttachGamePakDevice(NewGPIO(PeripheralRTC))

	if got, _ := b.Read16(bus.GPIODataAddress, bus.Access{}); got != 0x5aa5 {
		t.Fatalf("disabled GPIO read = %04x, want underlying ROM 5aa5", got)
	}

	rtcWriteReg(b, bus.GPIOControlAddress, 1)
	rtcWriteReg(b, bus.GPIODirectionAddress, 0x7)
	rtcWriteReg(b, bus.GPIODataAddress, 0x5)
	if got, _ := b.Read16(bus.GPIODirectionAddress, bus.Access{}); got != 0x0007 {
		t.Fatalf("GPIO direction read = %04x, want 0007", got)
	}
	if got, _ := b.Read16(bus.GPIOControlAddress, bus.Access{}); got != 0x0001 {
		t.Fatalf("GPIO control read = %04x, want 0001", got)
	}
}

func TestRTCStatusReadWrite(t *testing.T) {
	b, rtc := fixedRTCBus(t, time.Date(2026, time.September, 27, 11, 22, 33, 0, time.UTC))

	rtcBeginCommand(b, rtcCommandReadStatus)
	if got := rtcReadBytes(b, 1); len(got) != 1 || got[0] != 0x40 {
		t.Fatalf("initial status = %v, want [40]", got)
	}

	rtcBeginCommand(b, rtcCommandWriteStatus)
	rtcWriteBytes(b, []byte{0x08})
	if got := rtc.Control(); got != 0x08 {
		t.Fatalf("written control = %02x, want 08", got)
	}

	rtcBeginCommand(b, rtcCommandReadStatus)
	if got := rtcReadBytes(b, 1)[0]; got != 0x08 {
		t.Fatalf("read-back status = %02x, want 08", got)
	}
}

func TestRTCDateTimeReadBCD(t *testing.T) {
	now := time.Date(2026, time.September, 27, 23, 58, 41, 0, time.UTC) // Sunday
	b, _ := fixedRTCBus(t, now)

	rtcBeginCommand(b, rtcCommandReadDateTime)
	got := rtcReadBytes(b, 7)
	want := []byte{0x26, 0x09, 0x27, 0x06, 0x23, 0x58, 0x41}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("datetime[%d] = %02x, want %02x (all=% x)", i, got[i], want[i], got)
		}
	}
}

func TestRTCTimeWriteAdjustsExposedClock(t *testing.T) {
	now := time.Date(2026, time.September, 27, 9, 10, 11, 0, time.UTC)
	b, rtc := fixedRTCBus(t, now)

	rtcBeginCommand(b, rtcCommandWriteTime)
	rtcWriteBytes(b, []byte{0x17, 0x34, 0x56})

	got := rtc.CurrentTime()
	if got.Hour() != 17 || got.Minute() != 34 || got.Second() != 56 {
		t.Fatalf("adjusted RTC = %s, want 17:34:56", got.Format(time.RFC3339))
	}

	rtcBeginCommand(b, rtcCommandReadTime)
	if data := rtcReadBytes(b, 3); data[0] != 0x17 || data[1] != 0x34 || data[2] != 0x56 {
		t.Fatalf("time read-back = % x, want 17 34 56", data)
	}
}

func TestConfigureAutoAttachesRTCFromStandardMarker(t *testing.T) {
	rom := []byte("FLASH1M_V103 ... SIIRTC_V001")
	b := bus.New(nil, rom)
	setup := Configure(b, rom, Config{})

	if setup.Peripherals&PeripheralRTC == 0 || setup.GPIO == nil || setup.GPIO.RTC() == nil {
		t.Fatalf("RTC setup = %+v, want attached GPIO RTC", setup)
	}
	if setup.SaveType != SaveFlash128K {
		t.Fatalf("save type = %s, want flash-128k", setup.SaveType)
	}
}
