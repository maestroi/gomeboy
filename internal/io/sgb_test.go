package io

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

func newSGBTestBus(t *testing.T, model types.Model) *Bus {
	t.Helper()
	b := NewBus(scheduler.NewScheduler(), make([]byte, 0x8000))
	b.Map(model)
	return b
}

func sendSGBPacket(b *Bus, command [16]byte) {
	b.Write(types.P1, 0x00)
	b.Write(types.P1, 0x30)
	for _, v := range command {
		for bit := uint8(0); bit < 8; bit++ {
			if v&(1<<bit) != 0 {
				b.Write(types.P1, 0x10)
			} else {
				b.Write(types.P1, 0x20)
			}
			b.Write(types.P1, 0x30)
		}
	}
	b.Write(types.P1, 0x20)
	b.Write(types.P1, 0x30)
}

func sendMLTREQ(b *Bus, mode byte) {
	var packet [16]byte
	packet[0] = 0x89 // MLT_REQ (0x11), one 16-byte packet
	packet[1] = mode
	sendSGBPacket(b, packet)
}

func incrementSGBController(b *Bus) {
	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x30)
}

func TestSGBMLTREQModes(t *testing.T) {
	for _, model := range []types.Model{types.SGB, types.SGB2} {
		t.Run(model.String(), func(t *testing.T) {
			b := newSGBTestBus(t, model)

			for _, tc := range []struct {
				mode byte
				want uint8
			}{
				{0, 1},
				{1, 2},
				{2, 4},
				{3, 4},
			} {
				sendMLTREQ(b, 0)
				sendMLTREQ(b, tc.mode)
				if got := b.sgb.PlayerCount; got != tc.want {
					t.Fatalf("MLT_REQ %d player count = %d, want %d", tc.mode, got, tc.want)
				}
			}
		})
	}
}

func TestSGBMLTREQControllerIDsAndSinglePlayerReset(t *testing.T) {
	b := newSGBTestBus(t, types.SGB)
	sendMLTREQ(b, 1)

	if got := b.Read(types.P1); got != 0xFF {
		t.Fatalf("initial two-player P1 = %#02x, want 0xff", got)
	}

	incrementSGBController(b)
	if got := b.Read(types.P1); got != 0xFE {
		t.Fatalf("second controller P1 = %#02x, want 0xfe", got)
	}

	sendMLTREQ(b, 0)
	if got := b.sgb.CurrentPlayer; got != 0 {
		t.Fatalf("single-player MLT_REQ left current player at %d, want 0", got)
	}
	if got := b.Read(types.P1); got != 0xFF {
		t.Fatalf("single-player P1 = %#02x, want 0xff", got)
	}
}

func TestSGBMLTREQOnePlayerIncrementEdges(t *testing.T) {
	b := newSGBTestBus(t, types.SGB)
	sendMLTREQ(b, 1)

	var got []byte
	record := func() { got = append(got, b.Read(types.P1)) }

	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x20)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x20)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x20)
	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x00)
	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x10)
	b.Write(types.P1, 0x00)
	b.Write(types.P1, 0x30)
	record()

	b.Write(types.P1, 0x00)
	b.Write(types.P1, 0x30)
	record()

	want := []byte{0xFE, 0xFE, 0xFF, 0xFF, 0xFE, 0xFF, 0xFE, 0xFF}
	if len(got) != len(want) {
		t.Fatalf("got %d samples, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %#02x, want %#02x", i, got[i], want[i])
		}
	}
}

func TestSGBStateSurvivesSnapshot(t *testing.T) {
	b := newSGBTestBus(t, types.SGB)
	sendMLTREQ(b, 3)
	incrementSGBController(b)
	incrementSGBController(b)

	state := b.Snapshot()
	b.sgb = SGBState{PlayerCount: 1}
	b.Restore(state)

	if b.sgb.PlayerCount != 4 || b.sgb.CurrentPlayer != 2 {
		t.Fatalf("restored SGB state = players %d current %d, want 4/2", b.sgb.PlayerCount, b.sgb.CurrentPlayer)
	}
}

func TestNonSGBJoypadWriteUnchanged(t *testing.T) {
	b := NewBus(scheduler.NewScheduler(), make([]byte, 0x8000))
	b.Map(types.DMGABC)

	b.Write(types.P1, 0x30)
	if got := b.Read(types.P1); got != 0xCF {
		t.Fatalf("DMG P1 no-selection value = %#02x, want legacy 0xcf", got)
	}
	b.Write(types.P1, 0x20)
	if got := b.Read(types.P1); got != 0xDF {
		t.Fatalf("DMG P1 direction-selection value = %#02x, want legacy 0xdf", got)
	}
}
