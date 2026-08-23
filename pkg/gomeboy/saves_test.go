package gomeboy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pokemonRedROM returns the path of the ROM named by the POKEMON_RED_ROM
// environment variable, skipping the test if it is unset. A relative path is
// resolved against the repository root (the test CWD is pkg/gomeboy).
func pokemonRedROM(t *testing.T) string {
	t.Helper()
	p := os.Getenv("POKEMON_RED_ROM")
	if p == "" {
		t.Skip("POKEMON_RED_ROM not set")
	}
	if !filepath.IsAbs(p) {
		if _, err := os.Stat(p); err != nil {
			p = filepath.Join("..", "..", p) // relative to the repository root
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			t.Fatalf("filepath.Abs(%q): %v", p, err)
		}
		p = abs
	}
	return p
}

func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func requireSaveFile(t *testing.T, dir string) {
	t.Helper()
	for _, name := range entryNames(t, dir) {
		if strings.HasSuffix(name, ".sav") {
			return
		}
	}
	t.Fatalf("no .sav file found in %s", dir)
}

func TestNoDiskIOByDefault(t *testing.T) {
	path := pokemonRedROM(t)
	t.Chdir(t.TempDir())

	e, err := New(WithROM(path))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.StepFrames(10)
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if names := entryNames(t, "."); len(names) != 0 {
		t.Errorf("expected no files in the working directory, found %v", names)
	}
}

func TestWithSaveDirWritesThere(t *testing.T) {
	path := pokemonRedROM(t)
	dir := t.TempDir()
	t.Chdir(t.TempDir())

	e, err := New(WithROM(path), WithSaveDir(dir))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	e.StepFrames(10)
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if names := entryNames(t, "."); len(names) != 0 {
		t.Errorf("expected no files in the working directory, found %v", names)
	}
	requireSaveFile(t, dir)
}

func TestTwoInstancesDoNotShareSaveFiles(t *testing.T) {
	path := pokemonRedROM(t)
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	e1, err := New(WithROM(path), WithSaveDir(dir1))
	if err != nil {
		t.Fatalf("New (1): %v", err)
	}
	e2, err := New(WithROM(path), WithSaveDir(dir2))
	if err != nil {
		t.Fatalf("New (2): %v", err)
	}

	e1.StepFrames(10)
	e2.StepFrames(10)
	if err := e1.Close(); err != nil {
		t.Fatalf("Close (1): %v", err)
	}
	if err := e2.Close(); err != nil {
		t.Fatalf("Close (2): %v", err)
	}

	requireSaveFile(t, dir1)
	requireSaveFile(t, dir2)
}
