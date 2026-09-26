package cartridge_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
)

func TestSRAMOnGamePakSaveBus(t *testing.T) {
	b := bus.New(nil, nil)
	s := cartridge.NewSRAM()
	b.AttachSaveDevice(s)

	const offset uint32 = 0x1234
	b.Write8(bus.SaveStart+offset, 0x5a, bus.Access{})

	if got, _ := b.Read8(bus.SaveStart+offset, bus.Access{}); got != 0x5a {
		t.Fatalf("save read = %02x, want 5a", got)
	}
	if got, _ := b.Read8(bus.SaveStart+cartridge.SRAMSize+offset, bus.Access{}); got != 0x5a {
		t.Fatalf("32 KiB SRAM mirror = %02x, want 5a", got)
	}
	if got, _ := b.Read8(bus.SaveStart+0x01000000+offset, bus.Access{}); got != 0x5a {
		t.Fatalf("0x0f save-window mirror = %02x, want 5a", got)
	}
	if got, _ := b.Read32(bus.SaveStart+offset, bus.Access{}); got != 0x5a5a5a5a {
		t.Fatalf("word read = %08x, want 5a5a5a5a", got)
	}
}
