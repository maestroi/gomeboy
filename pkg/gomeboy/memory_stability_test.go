package gomeboy

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	defaultMemoryStabilityFrames = 3000
	heapGrowthTolerance          = 4 << 20 // 4 MiB of retained heap after GC
	heapObjectGrowthTolerance    = 10000
)

func TestHeadlessMemoryStability(t *testing.T) {
	e := newTestEmulator(t, Headless())
	defer e.Close()

	frames := memoryStabilityFrames(t)
	segment := frames / 2
	if segment == 0 {
		t.Fatal("memory stability test requires at least two frames")
	}

	// Warm up one-time emulator/runtime allocations before comparing retained
	// heap. Measuring two equal steady-state windows avoids treating normal
	// startup caches as leaks.
	for i := 0; i < 500; i++ {
		e.StepFrame()
	}
	for i := 0; i < segment; i++ {
		e.StepFrame()
	}
	before := retainedHeap()

	for i := 0; i < segment; i++ {
		e.StepFrame()
	}
	after := retainedHeap()

	t.Logf("retained heap after %d + %d measured frames: %d -> %d bytes; objects: %d -> %d",
		segment, segment, before.HeapAlloc, after.HeapAlloc, before.HeapObjects, after.HeapObjects)

	if after.HeapAlloc > before.HeapAlloc+heapGrowthTolerance {
		t.Fatalf("retained heap grew by %d bytes (limit %d) across an equal steady-state frame window",
			after.HeapAlloc-before.HeapAlloc, heapGrowthTolerance)
	}
	if after.HeapObjects > before.HeapObjects+heapObjectGrowthTolerance {
		t.Fatalf("retained heap objects grew by %d (limit %d) across an equal steady-state frame window",
			after.HeapObjects-before.HeapObjects, heapObjectGrowthTolerance)
	}
}

func TestEmulatorLifecycleDoesNotLeakGoroutines(t *testing.T) {
	baseline := runtime.NumGoroutine()

	for i := 0; i < 25; i++ {
		e := newTestEmulator(t, Headless())
		for frame := 0; frame < 5; frame++ {
			e.StepFrame()
		}
		if err := e.Close(); err != nil {
			t.Fatalf("Close iteration %d: %v", i, err)
		}
	}

	// Give any legitimate shutdown goroutines a brief chance to exit. A
	// persistent per-instance leak will remain well above this small allowance.
	deadline := time.Now().Add(time.Second)
	for {
		runtime.GC()
		runtime.Gosched()
		got := runtime.NumGoroutine()
		if got <= baseline+2 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine count grew from %d to %d after repeated emulator create/close cycles", baseline, got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func memoryStabilityFrames(t *testing.T) int {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("GOMEBOY_MEMORY_STABILITY_FRAMES"))
	if raw == "" {
		return defaultMemoryStabilityFrames
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 2 {
		t.Fatalf("GOMEBOY_MEMORY_STABILITY_FRAMES=%q must be an integer >= 2", raw)
	}
	return n
}

func retainedHeap() runtime.MemStats {
	// Two collections make this deliberately about live/retained memory rather
	// than allocation rate or pages the Go runtime has merely kept for reuse.
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats
}
