package tests

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/types"
)

// Mealybug Tearoom compares the frame at its LD B,B software breakpoint, not
// at an arbitrary frame/time. Its reference images are hardware-revision
// specific, so generic CGB must never silently stand in for CGB C or D.
//
// The first CI slice intentionally covers six independent Mode 2/3 behaviors.
// Expand this table with new pinned test/model pairs as measured baselines
// become available; the upstream archive has 31 ROMs, not all with DMG images.
var mealybugPPUTests = []string{
	"m2_win_en_toggle",
	"m3_bgp_change",
	"m3_lcdc_win_en_change_multiple",
	"m3_scx_low_3_bits",
	"m3_window_timing",
	"m3_wx_6_change",
}

type mealybugReference struct {
	model types.Model
	dir   string
}

var mealybugReferences = []mealybugReference{
	{types.DMGABC, "DMG-blob"},
	{types.CGBBC, "CPU CGB C"},
	{types.CGBDE, "CPU CGB D"},
}

const mealybugRoot = "roms/mealybug"

type mealybugTest struct {
	*basicTest
	expectedImage string
}

func newMealybugCollection(suite *TestSuite) *TestCollection {
	col := suite.NewTestCollection("ppu")
	for _, name := range mealybugPPUTests {
		rom := filepath.Join(mealybugRoot, name+".gb")
		for _, ref := range mealybugReferences {
			expected := filepath.Join(mealybugRoot, "expected", ref.dir, name+".png")
			for _, path := range []string{rom, expected} {
				if _, err := os.Stat(path); err != nil {
					// Missing fixtures are infrastructure failures, never an
					// XFAIL attributed to the emulated hardware.
					panic(fmt.Sprintf("Mealybug fixture %s unavailable: %v; run tests/fetch-mealybug.sh", path, err))
				}
			}
			base := newBasicTest(rom, ref.model)
			base.name = name + "@" + ref.model.String()
			col.AddTests(&mealybugTest{basicTest: base, expectedImage: expected})
		}
	}
	return col
}

func testMealybug(table *TestTable) {
	newMealybugCollection(table.NewTestSuite("mealybug"))
}

func TestMealybug(t *testing.T) {
	if _, err := os.Stat(mealybugRoot); os.IsNotExist(err) {
		t.Skip("Mealybug fixtures not installed; run tests/fetch-mealybug.sh")
	}
	table := &TestTable{}
	testMealybug(table)
	table.testSuites[0].collections[0].Run(t)
}

func (m *mealybugTest) Run(t *testing.T) {
	m.passed = false
	t.Run(m.name, func(t *testing.T) {
		t.Parallel()
		// Step instruction-by-instruction to the LD B,B boundary.
		// Frame()-chunked execution diverges across VBlank stop/resume
		// once fine SCX discard lengthens Mode 3 (m3_scx_low_3_bits),
		// and can redraw scanlines after the ROM stops mid-Mode-3 writes.
		g := gameboy.NewGameBoy(
			gameboy.Debug(), gameboy.AsModel(m.model),
			gameboy.WithoutSaves(), gameboy.WithHeadlessAudio())
		if err := g.LoadROM(m.romPath); err != nil {
			t.Errorf("Mealybug ROM failed to load/execute: %v", err)
			return
		}
		const maxSteps = 20 * 70240 * 60
		for steps := 0; steps < maxSteps && !g.CPU.DebugBreakpoint; steps++ {
			g.StepInstruction()
		}
		if !g.CPU.DebugBreakpoint {
			t.Errorf("Mealybug ROM never reached LD B,B within bounded execution: %s", m.romPath)
			return
		}
		file, err := os.Open(m.expectedImage)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		expected, err := png.Decode(file)
		if err != nil {
			t.Fatalf("decode Mealybug reference %s: %v", m.expectedImage, err)
		}
		diff, err := mealybugPixelDifferences(expected, g.FrameBuffer())
		if err != nil {
			t.Fatal(err)
		}
		if diff != 0 {
			if err := writeMealybugArtifacts(m.name, expected, g.FrameBuffer()); err != nil {
				t.Errorf("write mismatch artifacts: %v", err)
			}
			skipKnownFailure(t, m.name)
			t.Errorf("Mealybug %s: %d RGB pixels differ at LD B,B (reference %s)", m.name, diff, m.expectedImage)
			return
		}
		m.passed = true
	})
}

// No fuzzy threshold or palette quantization: upstream references explicitly
// use DMG shades 00/55/AA/FF and CGB 5-bit expanded as (v<<3)|(v>>2).
func mealybugPixelDifferences(expected image.Image, actual *[144][160][3]uint8) (int, error) {
	if expected.Bounds().Dx() != 160 || expected.Bounds().Dy() != 144 {
		return 0, fmt.Errorf("Mealybug reference size %v, want 160x144", expected.Bounds())
	}
	diff := 0
	b := expected.Bounds()
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			r, g, blue, _ := expected.At(b.Min.X+x, b.Min.Y+y).RGBA()
			pixel := actual[y][x]
			if pixel[0] != uint8(r>>8) || pixel[1] != uint8(g>>8) || pixel[2] != uint8(blue>>8) {
				diff++
			}
		}
	}
	return diff, nil
}

// Artifacts are diagnostic only; the exact pixel result determines PASS/FAIL.
func writeMealybugArtifacts(name string, expected image.Image, actual *[144][160][3]uint8) error {
	dir := filepath.Join("results", "mealybug")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw := image.NewNRGBA(image.Rect(0, 0, 160, 144))
	overlay := image.NewNRGBA(raw.Bounds())
	b := expected.Bounds()
	for y := 0; y < 144; y++ {
		for x := 0; x < 160; x++ {
			p := actual[y][x]
			raw.SetNRGBA(x, y, color.NRGBA{R: p[0], G: p[1], B: p[2], A: 255})
			r, g, blue, _ := expected.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if p[0] != uint8(r>>8) || p[1] != uint8(g>>8) || p[2] != uint8(blue>>8) {
				overlay.SetNRGBA(x, y, color.NRGBA{R: 255, G: 0, B: 255, A: 255})
			} else {
				overlay.SetNRGBA(x, y, color.NRGBA{R: p[0], G: p[1], B: p[2], A: 255})
			}
		}
	}
	for suffix, img := range map[string]image.Image{"actual": raw, "diff": overlay} {
		path := filepath.Join(dir, name+"_"+suffix+".png")
		out, err := os.Create(path)
		if err != nil {
			return err
		}
		encodeErr := png.Encode(out, img)
		closeErr := out.Close()
		if encodeErr != nil {
			return encodeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func TestMealybugPixelDifferences(t *testing.T) {
	frame := new([144][160][3]uint8)
	ref := image.NewNRGBA(image.Rect(0, 0, 160, 144))
	if n, err := mealybugPixelDifferences(ref, frame); err != nil || n != 0 {
		t.Fatalf("identical pixels: n=%d err=%v", n, err)
	}
	ref.SetNRGBA(12, 34, color.NRGBA{R: 85, G: 170, B: 255, A: 255})
	if n, err := mealybugPixelDifferences(ref, frame); err != nil || n != 1 {
		t.Fatalf("one different pixel: n=%d err=%v", n, err)
	}
	if _, err := mealybugPixelDifferences(image.NewNRGBA(image.Rect(0, 0, 8, 8)), frame); err == nil {
		t.Fatal("missing dimension validation")
	}
}
