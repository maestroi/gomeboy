package gomeboy

import (
	"bytes"
	"os"
	"testing"
)

func TestWithROMBytesMatchesWithROM(t *testing.T) {
	rom, err := os.ReadFile(testROM)
	if err != nil {
		t.Fatalf("os.ReadFile(%s): %v", testROM, err)
	}

	a, err := New(WithROM(testROM))
	if err != nil {
		t.Fatalf("New(WithROM): %v", err)
	}
	defer a.Close()

	b, err := New(WithROMBytes(rom))
	if err != nil {
		t.Fatalf("New(WithROMBytes): %v", err)
	}
	defer b.Close()

	for i := 0; i < 30; i++ {
		a.StepFrame()
		b.StepFrame()
	}

	if !bytes.Equal(a.snapshot(), b.snapshot()) {
		t.Fatal("framebuffers differ between WithROM and WithROMBytes")
	}
}

func TestWithROMBytesEmpty(t *testing.T) {
	if _, err := New(WithROMBytes(nil)); err == nil {
		t.Fatal("New(WithROMBytes(nil)): expected error, got nil")
	}
}

func TestWithROMBytesNoFileAccess(t *testing.T) {
	rom, err := os.ReadFile(testROM)
	if err != nil {
		t.Fatalf("os.ReadFile(%s): %v", testROM, err)
	}

	t.Chdir(t.TempDir())

	e, err := New(WithROMBytes(rom))
	if err != nil {
		t.Fatalf("New(WithROMBytes): %v", err)
	}
	e.StepFrames(10)
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("os.ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no files in temp dir, found %d entries", len(entries))
	}
}
