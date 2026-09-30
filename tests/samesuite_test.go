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
	samesuiteROMPath = "roms/same-suite"
)

func samesuiteModelsForName(dir, name string) []types.Model {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	switch {
	case dir == "sgb":
		return []types.Model{types.SGB}
	case strings.HasSuffix(base, "-A"):
		return []types.Model{types.AGB}
	case strings.HasSuffix(base, "-cgb0BC"), strings.HasSuffix(base, "-cgb0B"):
		return []types.Model{types.CGB0, types.CGBBC}
	case strings.HasSuffix(base, "-cgbDE"):
		return []types.Model{types.CGBDE}
	case strings.HasSuffix(base, "-cgbB"), strings.HasSuffix(base, "-cgbBC"):
		return []types.Model{types.CGBBC}
	case strings.HasSuffix(base, "-cgb0"):
		return []types.Model{types.CGB0}
	case strings.Contains(name, "volume") || strings.Contains(dir, "apu/channel") ||
		name == "blocking_bgpi_increase.gb" || strings.Contains(name, "dma"):
		return []types.Model{types.CGBABC}
	default:
		return []types.Model{types.Unset}
	}
}

func TestSamesuiteModelsForName(t *testing.T) {
	cases := []struct {
		dir  string
		name string
		want []types.Model
	}{
		{"apu/channel_1", "channel_1_extra_length_clocking-cgb0B.gb", []types.Model{types.CGB0, types.CGBBC}},
		{"apu/channel_1", "channel_1_freq_change_timing-cgbDE.gb", []types.Model{types.CGBDE}},
		{"apu/channel_3", "channel_3_extra_length_clocking-cgbB.gb", []types.Model{types.CGBBC}},
		{"apu/channel_1", "channel_1_freq_change_timing-A.gb", []types.Model{types.AGB}},
		{"sgb", "command_mlt_req.gb", []types.Model{types.SGB}},
		{"apu/channel_2", "plain.gb", []types.Model{types.CGBABC}},
	}
	for _, tc := range cases {
		if got := samesuiteModelsForName(tc.dir, tc.name); !slices.Equal(got, tc.want) {
			t.Errorf("%s/%s: got %v, want %v", tc.dir, tc.name, got, tc.want)
		}
	}
}

func newSamesuiteTestCollectionFromDir(suite *TestSuite, dir string) *TestCollection {
	// More or less the same as Mooneye as it uses the same pass/fail mechanism.
	romDir := filepath.Join(samesuiteROMPath, dir)
	tc := suite.NewTestCollection(dir)

	files, err := os.ReadDir(romDir)
	if err != nil {
		panic(err)
	}

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".gb" {
			continue
		}

		models := samesuiteModelsForName(dir, file.Name())
		for _, model := range models {
			name := strings.Split(file.Name(), ".")[0]
			if len(models) > 1 {
				name += "@" + model.String()
			}
			tc.AddTests(&mooneyeTest{
				basicTest: &basicTest{
					romPath: filepath.Join(romDir, file.Name()),
					name:    name,
					model:   model,
				},
				emulatedSeconds: 5,
			})
		}
	}

	return tc
}
func testSamesuite(roms *TestTable) {
	// create top level test suite
	tS := roms.NewTestSuite("samesuite")

	// apu
	newSamesuiteTestCollectionFromDir(tS, "apu")
	newSamesuiteTestCollectionFromDir(tS, "apu/channel_1")
	newSamesuiteTestCollectionFromDir(tS, "apu/channel_2")
	newSamesuiteTestCollectionFromDir(tS, "apu/channel_3")
	newSamesuiteTestCollectionFromDir(tS, "apu/channel_4")

	// dma
	newSamesuiteTestCollectionFromDir(tS, "dma")

	// interrupt
	newSamesuiteTestCollectionFromDir(tS, "interrupt")

	// ppu
	newSamesuiteTestCollectionFromDir(tS, "ppu")

	// sgb
	newSamesuiteTestCollectionFromDir(tS, "sgb")
}
