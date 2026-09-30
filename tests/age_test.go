package tests

import (
	"github.com/maestroi/gomeboy/internal/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	ageROMPath = "roms/age"
)

// ageModelsForName returns the hardware families encoded in an AGE ROM
// filename. AGE's B/C and D/E suffix families map to explicit emulator
// revision profiles so results are attributed to the intended hardware rather
// than collapsed into the generic CGB profile.
func ageModelsForName(name string) []types.Model {
	name = strings.TrimSuffix(name, filepath.Ext(name))

	switch {
	case strings.HasSuffix(name, "dmgC-cgbBCE"):
		return []types.Model{types.DMGABC, types.CGBBC, types.CGBDE}
	case strings.HasSuffix(name, "dmgC-cgbBC"):
		return []types.Model{types.DMGABC, types.CGBBC}
	case strings.HasSuffix(name, "cgbBCE"),
		strings.HasSuffix(name, "ncmBCE"):
		return []types.Model{types.CGBBC, types.CGBDE}
	case strings.HasSuffix(name, "cgbBC"),
		strings.HasSuffix(name, "ncmBC"):
		return []types.Model{types.CGBBC}
	case strings.HasSuffix(name, "cgbE"),
		strings.HasSuffix(name, "ncmE"):
		return []types.Model{types.CGBDE}
	case strings.HasSuffix(name, "dmgC"):
		return []types.Model{types.DMGABC}
	default:
		return []types.Model{types.DMGABC}
	}
}
func TestAgeModelsForName(t *testing.T) {
	cases := []struct {
		name string
		want []types.Model
	}{
		{"ei-halt-dmgC-cgbBCE.gb", []types.Model{types.DMGABC, types.CGBBC, types.CGBDE}},
		{"ly-dmgC-cgbBC.gb", []types.Model{types.DMGABC, types.CGBBC}},
		{"oam-write-cgbBCE.gb", []types.Model{types.CGBBC, types.CGBDE}},
		{"stat-mode-sprites-ds-cgbBCE.gb", []types.Model{types.CGBBC, types.CGBDE}},
		{"oam-write-ncmBCE.gb", []types.Model{types.CGBBC, types.CGBDE}},
		{"ly-cgbE.gb", []types.Model{types.CGBDE}},
		{"oam-write-dmgC.gb", []types.Model{types.DMGABC}},
	}
	for _, tc := range cases {
		if got := ageModelsForName(tc.name); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
func newAgeTestCollectionFromDir(suite *TestSuite, dir string) *TestCollection {
	romDir := filepath.Join(ageROMPath, dir)
	tc := suite.NewTestCollection(dir)

	// read the directory
	files, err := os.ReadDir(romDir)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".gb" {
			continue
		}

		// get models that should pass
		models := ageModelsForName(file.Name())

		// Create one stable regression identity per ROM/model pair. AGE filenames
		// can target more than one hardware family, so the model must be part of
		// the test name rather than relying on Go's duplicate-subtest #01 suffix.
		for _, model := range models {
			basic := newBasicTest(filepath.Join(romDir, file.Name()), model)
			basic.name += "@" + model.String()
			tc.AddTests(&mooneyeTest{basicTest: basic})
		}
	}

	return tc
}

func TestAge(t *testing.T) {
	// the age ROMs are not shipped in roms.zip; skip rather than panic when
	// they are absent
	if _, err := os.Stat(ageROMPath); err != nil {
		t.Skipf("skipping age tests: %v", err)
	}

	// create top level test
	//tS := table.NewTestSuite("age")
	table := &TestTable{}
	tS := table.NewTestSuite("age")
	// halt
	newAgeTestCollectionFromDir(tS, "halt").Run(t)
	// lcd-align-ly
	newAgeTestCollectionFromDir(tS, "lcd-align-ly").Run(t)
	// ly
	newAgeTestCollectionFromDir(tS, "ly").Run(t)
	// oam
	newAgeTestCollectionFromDir(tS, "oam").Run(t)
	// stat-interrupt
	newAgeTestCollectionFromDir(tS, "stat-interrupt").Run(t)
	// stat-mode
	newAgeTestCollectionFromDir(tS, "stat-mode").Run(t)
	// stat-mode-sprites
	newAgeTestCollectionFromDir(tS, "stat-mode-sprites").Run(t)
	// stat-mode-window
	newAgeTestCollectionFromDir(tS, "stat-mode-window").Run(t)
	// vram
	newAgeTestCollectionFromDir(tS, "vram").Run(t)
}

func testAge(t *TestTable) {
	// create top level test
	tS := t.NewTestSuite("age")

	// halt
	newAgeTestCollectionFromDir(tS, "halt")
	// lcd-align-ly
	newAgeTestCollectionFromDir(tS, "lcd-align-ly")
	// ly
	newAgeTestCollectionFromDir(tS, "ly")
	// oam
	newAgeTestCollectionFromDir(tS, "oam")
	// stat-interrupt
	newAgeTestCollectionFromDir(tS, "stat-interrupt")
	// stat-mode
	newAgeTestCollectionFromDir(tS, "stat-mode")
	// stat-mode-sprites
	newAgeTestCollectionFromDir(tS, "stat-mode-sprites")
	// stat-mode-window
	newAgeTestCollectionFromDir(tS, "stat-mode-window")
	// vram
	newAgeTestCollectionFromDir(tS, "vram")
}
