package power

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
)

func TestPOSTFLGAndHALTCNTByteLanes(t *testing.T) {
	b := bus.New(nil, nil)
	var halts, stops int
	p := New(b, Hooks{
		BIOSAccess: func() bool { return true },
		Halt:       func() { halts++ },
		Stop:       func() { stops++ },
	})

	// HALTCNT is not armed until BIOS POSTFLG has been set.
	b.Write8(bus.IOStart+systemControlOffset+1, 0, bus.Access{})
	if halts != 0 {
		t.Fatal("HALTCNT halted before POSTFLG was set")
	}

	b.Write8(bus.IOStart+systemControlOffset, 1, bus.Access{})
	if p.POSTFlag() != 1 {
		t.Fatalf("POSTFLG = %d, want 1", p.POSTFlag())
	}
	if got, _ := b.Read16(bus.IOStart+systemControlOffset, bus.Access{}); got != 1 {
		t.Fatalf("system-control readback = %04x, want 0001", got)
	}

	b.Write8(bus.IOStart+systemControlOffset+1, 0x00, bus.Access{})
	if halts != 1 || stops != 0 {
		t.Fatalf("HALTCNT halt callbacks = halt:%d stop:%d, want 1/0", halts, stops)
	}

	b.Write8(bus.IOStart+systemControlOffset+1, 0x80, bus.Access{})
	if halts != 1 || stops != 1 {
		t.Fatalf("HALTCNT stop callbacks = halt:%d stop:%d, want 1/1", halts, stops)
	}
	if got, _ := b.Read8(bus.IOStart+systemControlOffset+1, bus.Access{}); got != 0 {
		t.Fatalf("write-only HALTCNT readback = %02x, want 00", got)
	}
}

func TestSystemControlWritesRequireBIOSExecution(t *testing.T) {
	b := bus.New(nil, nil)
	allowed := false
	halts := 0
	p := New(b, Hooks{
		BIOSAccess: func() bool { return allowed },
		Halt:       func() { halts++ },
	})

	b.Write8(bus.IOStart+systemControlOffset, 1, bus.Access{})
	b.Write8(bus.IOStart+systemControlOffset+1, 0, bus.Access{})
	if p.POSTFlag() != 0 || halts != 0 {
		t.Fatalf("non-BIOS system-control write changed state: POSTFLG=%d halts=%d", p.POSTFlag(), halts)
	}

	allowed = true
	b.Write8(bus.IOStart+systemControlOffset, 1, bus.Access{})
	b.Write8(bus.IOStart+systemControlOffset+1, 0, bus.Access{})
	if p.POSTFlag() != 1 || halts != 1 {
		t.Fatalf("BIOS system-control write state: POSTFLG=%d halts=%d", p.POSTFlag(), halts)
	}
}

func TestHalfwordWriteUsesExistingPOSTFLGToArmPowerControl(t *testing.T) {
	b := bus.New(nil, nil)
	var halts, stops int
	p := New(b, Hooks{
		BIOSAccess: func() bool { return true },
		Halt:       func() { halts++ },
		Stop:       func() { stops++ },
	})

	// The first write sets POSTFLG but does not simultaneously enter HALT.
	b.Write16(bus.IOStart+systemControlOffset, 0x0001, bus.Access{})
	if p.POSTFlag() != 1 || halts != 0 {
		t.Fatalf("first halfword write POSTFLG=%d halts=%d, want 1/0", p.POSTFlag(), halts)
	}

	b.Write16(bus.IOStart+systemControlOffset, 0x0001, bus.Access{})
	if halts != 1 {
		t.Fatalf("armed halfword HALT count = %d, want 1", halts)
	}

	b.Write16(bus.IOStart+systemControlOffset, 0x8001, bus.Access{})
	if stops != 1 {
		t.Fatalf("armed halfword STOP count = %d, want 1", stops)
	}
}

func TestResetClearsPOSTFLG(t *testing.T) {
	b := bus.New(nil, nil)
	p := New(b, Hooks{BIOSAccess: func() bool { return true }})
	b.Write8(bus.IOStart+systemControlOffset, 1, bus.Access{})
	p.Reset()
	if p.POSTFlag() != 0 {
		t.Fatalf("POSTFLG after reset = %d, want 0", p.POSTFlag())
	}
}
