package main

import (
	"testing"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

func TestParseRegions(t *testing.T) {
	regions, err := parseRegions("wram:C000-DFFF, flag:0xFF0F")
	if err != nil {
		t.Fatalf("parseRegions: %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("len(regions) = %d, want 2", len(regions))
	}
	if regions[0].Name != "wram" || regions[0].Start != 0xC000 || regions[0].Length != 0x2000 {
		t.Fatalf("wram region = %+v", regions[0])
	}
	if regions[1].Name != "flag" || regions[1].Start != 0xFF0F || regions[1].Length != 1 {
		t.Fatalf("flag region = %+v", regions[1])
	}
}

func TestParseRegionsRejectsDescendingRange(t *testing.T) {
	if _, err := parseRegions("bad:D000-C000"); err == nil {
		t.Fatal("parseRegions accepted descending range")
	}
}

func TestParseActions(t *testing.T) {
	actions, err := parseActions("control,right,a", 2, 5)
	if err != nil {
		t.Fatalf("parseActions: %v", err)
	}
	if len(actions) != 3 {
		t.Fatalf("len(actions) = %d, want 3", len(actions))
	}
	if actions[0].Name != "control" || len(actions[0].Phases) != 1 || actions[0].Phases[0].Frames != 7 {
		t.Fatalf("control = %+v", actions[0])
	}
	if actions[1].Name != "right" || actions[1].Phases[0].Transitions[0].Button != gomeboy.ButtonRight {
		t.Fatalf("right = %+v", actions[1])
	}
	if actions[2].Name != "a" || actions[2].Phases[0].Transitions[0].Button != gomeboy.ButtonA {
		t.Fatalf("a = %+v", actions[2])
	}
}

func TestParseActionsRejectsUnknownAction(t *testing.T) {
	if _, err := parseActions("jump", 1, 1); err == nil {
		t.Fatal("parseActions accepted unknown action")
	}
}
