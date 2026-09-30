package io

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/maestroi/gomeboy/internal/scheduler"
	"github.com/maestroi/gomeboy/internal/types"
)

var whichbootNintendoLogo = [...]byte{
	0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B,
	0x03, 0x73, 0x00, 0x83, 0x00, 0x0C, 0x00, 0x0D,
	0x00, 0x08, 0x11, 0x1F, 0x88, 0x89, 0x00, 0x0E,
	0xDC, 0xCC, 0x6E, 0xE6, 0xDD, 0xDD, 0xD9, 0x99,
	0xBB, 0xBB, 0x67, 0x63, 0x6E, 0x0E, 0xEC, 0xCC,
	0xDD, 0xDC, 0x99, 0x9F, 0xBB, 0xB9, 0x33, 0x3E,
}

var whichbootCopyrightTile = [...]byte{
	0x3C, 0x00, 0x42, 0x00, 0xB9, 0x00, 0xA5, 0x00,
	0xB9, 0x00, 0xA5, 0x00, 0x42, 0x00, 0x3C, 0x00,
}

func newWhichbootBootBus(t *testing.T, model types.Model) *Bus {
	t.Helper()
	rom := make([]byte, 0x8000)
	copy(rom[0x0104:0x0134], whichbootNintendoLogo[:])
	rom[0x0147] = 0x00 // ROM only
	b := NewBus(scheduler.NewScheduler(), rom)
	b.Map(model)
	b.Boot()
	return b
}

func TestBootVRAMMatchesWhichbootHardwareFingerprint(t *testing.T) {
	// Reference: nitro2k01/whichboot.gb v1.1, source revision
	// 545436fb485f9006d47e0f26f0ceec76cd8e3a07.
	// whichboot.gb classifies the official boot ROMs using four VRAM
	// properties: Nintendo logo tiles, BG-map residue, the registered-symbol
	// tile, and whether all unrelated bytes are zero.
	const expectedLogoSHA256 = "41c946a2a92c09642f2b659de8b73c7af83c7842f093fe3e66559141b45b8497"

	tests := []struct {
		model       types.Model
		mapPresent  bool
		copyright   bool
	}{
		{types.DMG0, true, false},
		{types.DMGABC, true, true},
		{types.MGB, true, true},
		{types.SGB, true, true},
		{types.SGB2, true, true},
		{types.CGB0, false, true},
		{types.CGBABC, false, true},
		{types.CGBBC, false, true},
		{types.CGBDE, false, true},
		{types.AGB, false, true},
	}

	for _, tc := range tests {
		t.Run(tc.model.String(), func(t *testing.T) {
			b := newWhichbootBootBus(t, tc.model)
			vram := b.VRAM[0]

			logoSum := fmt.Sprintf("%x", sha256.Sum256(vram[0x0010:0x0190]))
			if logoSum != expectedLogoSHA256 {
				t.Fatalf("Nintendo logo SHA-256 = %s, want %s", logoSum, expectedLogoSHA256)
			}

			if tc.copyright {
				for i, want := range whichbootCopyrightTile {
					if got := vram[0x0190+i]; got != want {
						t.Fatalf("copyright tile byte %d = %02x, want %02x", i, got, want)
					}
				}
			} else {
				assertWhichbootZero(t, vram[0x0190:0x01a0], "copyright tile")
			}

			if tc.mapPresent {
				for i := 0; i < 12; i++ {
					if got, want := vram[0x0904+i], byte(i+1); got != want {
						t.Fatalf("top logo map byte %d = %02x, want %02x", i, got, want)
					}
					if got, want := vram[0x0924+i], byte(i+13); got != want {
						t.Fatalf("bottom logo map byte %d = %02x, want %02x", i, got, want)
					}
				}
				wantR := byte(0x19)
				if !tc.copyright {
					wantR = 0
				}
				if got := vram[0x0910]; got != wantR {
					t.Fatalf("logo-map registered-symbol entry = %02x, want %02x", got, wantR)
				}
			} else {
				assertWhichbootZero(t, vram[0x0904:0x0911], "top logo map")
				assertWhichbootZero(t, vram[0x0924:0x0930], "bottom logo map")
			}

			assertWhichbootZero(t, vram[0x0000:0x0010], "tile zero")
			assertWhichbootZero(t, vram[0x01a0:0x0904], "unused tiles/map prefix")
			assertWhichbootZero(t, vram[0x0911:0x0924], "map gap")
			assertWhichbootZero(t, vram[0x0930:0x2000], "map suffix")
		})
	}
}

func assertWhichbootZero(t *testing.T, data []byte, name string) {
	t.Helper()
	for i, got := range data {
		if got != 0 {
			t.Fatalf("%s byte %d = %02x, want 00", name, i, got)
		}
	}
}
