package gomeboy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	"github.com/maestroi/gomeboy/internal/gba/cartridge"
)

func gbaROMWithMarker(marker string) []byte {
	rom := append([]byte(nil), gbaLoopROM()...)
	return append(rom, []byte(marker)...)
}

func gbaMachine(t *testing.T, e *Emulator) *gbaCore {
	t.Helper()
	core, ok := e.core.(*gbaCore)
	if !ok || core.machine == nil {
		t.Fatalf("core = %T, want loaded *gbaCore", e.core)
	}
	return core
}

func TestGBASavePersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	rom := gbaROMWithMarker("SRAM_V113")
	const saveOffset = uint32(0x1234)
	const want = byte(0x5a)

	first, err := New(Headless(), WithSaveDir(dir))
	if err == nil {
		err = first.LoadROMBytes(rom, "persist.gba")
	}
	if err != nil {
		t.Fatalf("first emulator: %v", err)
	}
	core := gbaMachine(t, first)
	core.machine.Bus.Write8(bus.SaveStart+saveOffset, want, bus.Access{})
	if err := first.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	path := filepath.Join(dir, "persist.sav")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if len(data) != cartridge.SRAMSize {
		t.Fatalf("save size = %d, want %d", len(data), cartridge.SRAMSize)
	}
	if data[saveOffset] != want {
		t.Fatalf("saved byte = %02x, want %02x", data[saveOffset], want)
	}

	second, err := New(Headless(), WithSaveDir(dir))
	if err == nil {
		err = second.LoadROMBytes(rom, "persist.gba")
	}
	if err != nil {
		t.Fatalf("second emulator: %v", err)
	}
	defer second.Close()

	got, _ := gbaMachine(t, second).machine.Bus.Read8(bus.SaveStart+saveOffset, bus.Access{})
	if got != want {
		t.Fatalf("reloaded SRAM byte = %02x, want %02x", got, want)
	}
}

func TestGBASavesRemainOptIn(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)

	e, err := New(Headless())
	if err == nil {
		err = e.LoadROMBytes(gbaROMWithMarker("SRAM_V113"), "no-disk.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	core := gbaMachine(t, e)
	core.machine.Bus.Write8(bus.SaveStart, 0x42, bus.Access{})
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := os.Stat(filepath.Join(cwd, "no-disk.sav")); !os.IsNotExist(err) {
		t.Fatalf("save file exists without WithSaveDir: %v", err)
	}
}

func TestGBAResetPreservesLiveCartridgeSave(t *testing.T) {
	e, err := New(Headless())
	if err == nil {
		err = e.LoadROMBytes(gbaROMWithMarker("FLASH1M_V103"), "reset.gba")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	core := gbaMachine(t, e)
	const addr = bus.SaveStart + 0x5555
	// Program a byte through the actual flash command protocol.
	core.machine.Bus.Write8(bus.SaveStart+0x5555, 0xaa, bus.Access{})
	core.machine.Bus.Write8(bus.SaveStart+0x2aaa, 0x55, bus.Access{})
	core.machine.Bus.Write8(bus.SaveStart+0x5555, 0xa0, bus.Access{})
	core.machine.Bus.Write8(addr, 0x37, bus.Access{})
	before, _ := core.machine.Bus.Read8(addr, bus.Access{})
	if before != 0x37 {
		t.Fatalf("pre-reset flash byte = %02x, want 37", before)
	}

	if err := e.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	after, _ := gbaMachine(t, e).machine.Bus.Read8(addr, bus.Access{})
	if after != before {
		t.Fatalf("post-reset flash byte = %02x, want %02x", after, before)
	}
}

func TestGBALoadRejectsSaveForWrongHardwareSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wrong.sav")
	if err := os.WriteFile(path, make([]byte, cartridge.Flash64KSize), 0o644); err != nil {
		t.Fatal(err)
	}

	e, err := New(Headless(), WithSaveDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	err = e.LoadROMBytes(gbaROMWithMarker("FLASH1M_V103"), "wrong.gba")
	if err == nil {
		t.Fatal("LoadROMBytes accepted a 64 KiB save for 128 KiB flash")
	}
}
