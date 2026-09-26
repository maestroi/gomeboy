package cartridge_test

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
	"github.com/maestroi/gomeboy/internal/gba/dma"
)

func appendEEPROMBits(dst []uint16, value uint64, bits int) []uint16 {
	for bit := bits - 1; bit >= 0; bit-- {
		dst = append(dst, uint16(value>>uint(bit))&1)
	}
	return dst
}

func eepromWritePacket(addressBits int, address uint32, data uint64) []uint16 {
	packet := []uint16{1, 0}
	packet = appendEEPROMBits(packet, uint64(address), addressBits)
	packet = appendEEPROMBits(packet, data, 64)
	return append(packet, 0)
}

func eepromReadRequest(addressBits int, address uint32) []uint16 {
	packet := []uint16{1, 1}
	packet = appendEEPROMBits(packet, uint64(address), addressBits)
	return append(packet, 0)
}

func writeEEPROMPacketSource(b *bus.Bus, base uint32, packet []uint16) {
	for i, bit := range packet {
		b.Write16(base+uint32(i*2), bit, bus.Access{})
	}
}

func runDMA3(b *bus.Bus, source, dest uint32, count uint16, control uint16) {
	b.Write32(bus.IOStart+0x0d4, source, bus.Access{})
	b.Write32(bus.IOStart+0x0d8, dest, bus.Access{})
	b.Write16(bus.IOStart+0x0dc, count, bus.Access{})
	b.Write16(bus.IOStart+0x0de, control|(1<<15), bus.Access{})
}

func TestEEPROMROM2WindowRouting(t *testing.T) {
	small := bus.New(nil, nil)
	small.AttachEEPROMDevice(cartridge.NewEEPROM512B())

	if got, cycles := small.Read16(bus.EEPROMStart, bus.Access{}); got != 1 || cycles != 5 {
		t.Fatalf("small-ROM EEPROM idle read = %04x cycles=%d, want 0001/5", got, cycles)
	}

	rom := make([]byte, 16*1024*1024+2)
	rom[16*1024*1024] = 0x34
	rom[16*1024*1024+1] = 0x12
	large := bus.New(nil, rom)
	large.AttachEEPROMDevice(cartridge.NewEEPROM512B())

	if got, _ := large.Read16(bus.EEPROMStart, bus.Access{}); got != 0x1234 {
		t.Fatalf("large-ROM lower 0x0D window = %04x, want ROM 1234", got)
	}
	if got, _ := large.Read16(bus.EEPROMHighStart, bus.Access{}); got != 1 {
		t.Fatalf("large-ROM top EEPROM window idle read = %04x, want 0001", got)
	}
}

func TestEEPROMDMA3RoundTrip(t *testing.T) {
	b := bus.New(nil, nil)
	e := cartridge.NewEEPROM8K()
	b.AttachEEPROMDevice(e)
	d := dma.New(b, nil, dma.Hooks{})

	const (
		address uint32 = 0x155
		want    uint64 = 0x0123456789abcdef
		eepAddr uint32 = bus.EEPROMHighStart
	)

	writePacket := eepromWritePacket(14, address, want)
	writeSource := uint32(bus.EWRAMStart + 0x1000)
	writeEEPROMPacketSource(b, writeSource, writePacket)

	// DMA3 halfword writes, source increment / destination fixed.
	runDMA3(b, writeSource, eepAddr, uint16(len(writePacket)), 2<<5)
	if units, _ := d.LastTransfer(3); units != uint32(len(writePacket)) {
		t.Fatalf("EEPROM write DMA units = %d, want %d", units, len(writePacket))
	}

	readRequest := eepromReadRequest(14, address)
	requestSource := uint32(bus.EWRAMStart + 0x2000)
	writeEEPROMPacketSource(b, requestSource, readRequest)
	runDMA3(b, requestSource, eepAddr, uint16(len(readRequest)), 2<<5)

	readDest := uint32(bus.EWRAMStart + 0x3000)
	// DMA3 halfword reads, source fixed / destination increment.
	runDMA3(b, eepAddr, readDest, 68, 2<<7)

	for i := 0; i < 4; i++ {
		if got, _ := b.Read16(readDest+uint32(i*2), bus.Access{}); got&1 != 0 {
			t.Fatalf("EEPROM DMA dummy bit %d = %d, want 0", i, got&1)
		}
	}

	var got uint64
	for i := 4; i < 68; i++ {
		value, _ := b.Read16(readDest+uint32(i*2), bus.Access{})
		got = got<<1 | uint64(value&1)
	}
	if got != want {
		t.Fatalf("EEPROM DMA round trip = %016x, want %016x", got, want)
	}
	if units, _ := d.LastTransfer(3); units != 68 {
		t.Fatalf("EEPROM read DMA units = %d, want 68", units)
	}
}
