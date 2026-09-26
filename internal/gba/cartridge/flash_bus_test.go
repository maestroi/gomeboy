package cartridge_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
)

func flashBusCommand(b *bus.Bus, command byte) {
	b.Write8(bus.SaveStart+0x5555, 0xaa, bus.Access{})
	b.Write8(bus.SaveStart+0x2aaa, 0x55, bus.Access{})
	b.Write8(bus.SaveStart+0x5555, command, bus.Access{})
}

func TestFlashOnGamePakSaveBus(t *testing.T) {
	b := bus.New(nil, nil)
	f := cartridge.NewFlash128K()
	b.AttachSaveDevice(f)

	flashBusCommand(b, 0xa0)
	b.Write8(bus.SaveStart+0x1234, 0x5a, bus.Access{})

	if got, cycles := b.Read8(bus.SaveStart+0x1234, bus.Access{}); got != 0x5a || cycles != 5 {
		t.Fatalf("flash read = %02x cycles=%d, want 5a/5", got, cycles)
	}
	if got, _ := b.Read8(bus.SaveStart+0x01000000+0x1234, bus.Access{}); got != 0x5a {
		t.Fatalf("0x0f flash mirror = %02x, want 5a", got)
	}

	flashBusCommand(b, 0x90)
	if got, _ := b.Read8(bus.SaveStart, bus.Access{}); got != 0x62 {
		t.Fatalf("manufacturer ID through bus = %02x, want 62", got)
	}
	if got, _ := b.Read8(bus.SaveStart+1, bus.Access{}); got != 0x13 {
		t.Fatalf("device ID through bus = %02x, want 13", got)
	}

	flashBusCommand(b, 0xf0)
	flashBusCommand(b, 0xb0)
	b.Write8(bus.SaveStart, 1, bus.Access{})
	if got, _ := b.Read8(bus.SaveStart+0x1234, bus.Access{}); got != 0xff {
		t.Fatalf("bank 1 initial read = %02x, want ff", got)
	}
}
