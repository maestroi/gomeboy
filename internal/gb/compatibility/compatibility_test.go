package compatibility

import (
	"strings"
	"testing"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

func TestStageOrder(t *testing.T) {
	stages := []Stage{StageLoaded, StageRendered, StageCheckpoint, StageInput, StagePersistence, StageStable}
	for i := 1; i < len(stages); i++ {
		if !reached(stages[i], stages[i-1]) {
			t.Fatalf("%s should include %s", stages[i], stages[i-1])
		}
	}
}

func TestValidateCaseRequiresPinnedBoundedROM(t *testing.T) {
	err := validateCase(Case{Name: "bad", ROM: []byte{1}, ROMName: "bad.gb", Model: gomeboy.ModelDMG, MaxFrames: 1, TargetStage: StageLoaded})
	if err == nil || !strings.Contains(err.Error(), "rom_sha256") {
		t.Fatalf("validateCase error = %v, want rom_sha256 failure", err)
	}
}

func TestParseButton(t *testing.T) {
	for _, name := range []string{"a", "b", "start", "select", "up", "down", "left", "right"} {
		if _, ok := parseButton(name); !ok {
			t.Fatalf("parseButton(%q) failed", name)
		}
	}
	if _, ok := parseButton("turbo"); ok {
		t.Fatal("unknown button parsed successfully")
	}
}

func TestMarkdownKeepsCompatibilitySeparate(t *testing.T) {
	report := Report{Suite: "smoke", SuiteRevision: "1", GomeBoyCommit: "test"}
	text := report.Markdown()
	if !strings.Contains(text, "not hardware-accuracy percentages") {
		t.Fatalf("report missing compatibility disclaimer: %s", text)
	}
}
