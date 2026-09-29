package tests

import (
	"github.com/maestroi/gomeboy/internal/types"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	ageROMPath = "roms/age"
)

// ageModelsForName returns the hardware families encoded in an AGE ROM
// filename. AGE distinguishes several CGB revisions and native/compatibility
// modes more finely than GomeBoy currently does, so all CGB variants collapse
// to the CGB model until the core exposes those revisions separately.
func ageModelsForName(name string) []types.Model {
	name = strings.TrimSuffix(name, filepath.Ext(name))

	switch {
	case strings.HasSuffix(name, "dmgC-cgbBCE"), strings.HasSuffix(name, "dmgC-cgbBC"):
		return []types.Model{types.DMGABC, types.CGBABC}
	case strings.HasSuffix(name, "cgbBCE"),
		strings.HasSuffix(name, "cgbBC"),
		strings.HasSuffix(name, "cgbE"),
		strings.HasSuffix(name, "ncmBCE"),
		strings.HasSuffix(name, "ncmBC"),
		strings.HasSuffix(name, "ncmE"):
		return []types.Model{types.CGBABC}
	case strings.HasSuffix(name, "dmgC"):
		return []types.Model{types.DMGABC}
	default:
		return []types.Model{types.DMGABC}
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
