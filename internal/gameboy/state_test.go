package gameboy

import "testing"

// TestLoadStateRestoresAPUBuffer reproduces the F6 quick-load panic: the
// audio callback drains the APU sample buffer every frame, so at save time
// the pending-sample slice is small (often empty). Restore must rebuild the
// buffer with at least the base capacity, or the next Frame() indexes an
// empty slice.
func TestLoadStateRestoresAPUBuffer(t *testing.T) {
	gb := NewGameBoy()
	if err := gb.LoadROM("../../tests/roms/little-things-gb/firstwhite.gb"); err != nil {
		t.Fatalf("LoadROM: %v", err)
	}
	for i := 0; i < 10; i++ {
		gb.Frame()
	}
	gb.APU.Samples()
	state, err := gb.SaveState()
	if err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := gb.LoadState(state); err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	gb.Frame()
}
