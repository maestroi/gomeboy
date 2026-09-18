package gomeboy

import "testing"

func TestCoreNeutralMemoryAPIUsesActiveAddressSpace(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	if got, want := e.AddressSpaceSize(), uint64(AddressSpaceSize); got != want {
		t.Fatalf("AddressSpaceSize() = %d, want %d", got, want)
	}

	const addr = uint32(0xC000)
	got, err := e.Peek8At(addr)
	if err != nil {
		t.Fatalf("Peek8At: %v", err)
	}
	if want := e.Peek8(uint16(addr)); got != want {
		t.Fatalf("Peek8At(0x%X) = 0x%02X, want 0x%02X", addr, got, want)
	}

	if _, err := e.Peek8At(uint32(AddressSpaceSize)); err == nil {
		t.Fatal("Peek8At accepted address immediately beyond GB address space")
	}
	if _, err := e.Read8At(uint32(AddressSpaceSize)); err == nil {
		t.Fatal("Read8At accepted address immediately beyond GB address space")
	}
	if err := e.PeekIntoAt(0xffff, make([]byte, 2)); err == nil {
		t.Fatal("PeekIntoAt accepted range crossing GB address-space boundary")
	}
}

func TestCheckpointStorageIsOwnedByCore(t *testing.T) {
	e := newTestEmulator(t)
	defer e.Close()

	var cp Checkpoint
	e.CheckpointInto(&cp)
	if !cp.valid {
		t.Fatal("checkpoint not marked valid")
	}
	if cp.coreID != "gb" {
		t.Fatalf("checkpoint coreID = %q, want gb", cp.coreID)
	}
	first := cp.state

	e.StepFrame()
	e.CheckpointInto(&cp)
	if cp.state != first {
		t.Fatal("CheckpointInto replaced reusable storage for same core")
	}
	if err := e.RestoreCheckpoint(&cp); err != nil {
		t.Fatalf("RestoreCheckpoint: %v", err)
	}
}
