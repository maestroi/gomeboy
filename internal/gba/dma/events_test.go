package dma

import (
	"testing"

	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

func TestVBlankDMAOneShotClearsEnable(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x100)
	dest := uint32(bus.IWRAMStart + 0x100)
	b.Write16(source, 0x1234, bus.Access{})

	programDMA(b, 0, source, dest, 1, controlEnable|timingVBlank)
	if got, _ := b.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("VBlank DMA ran before trigger: %04x", got)
	}

	stall := d.Trigger(StartVBlank)
	if got, _ := b.Read16(dest, bus.Access{}); got != 0x1234 {
		t.Fatalf("VBlank DMA result = %04x, want 1234", got)
	}
	if stall != 6 {
		t.Fatalf("VBlank DMA stall = %d, want 6", stall)
	}
	if d.Control(0)&controlEnable != 0 {
		t.Fatal("one-shot VBlank DMA did not clear Enable")
	}
}

func TestHBlankRepeatReloadsCountAndDestinationButNotSource(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x200)
	dest := uint32(bus.IWRAMStart + 0x200)
	b.Write16(source, 0x1111, bus.Access{})
	b.Write16(source+2, 0x2222, bus.Access{})

	control := uint16(controlEnable | controlRepeat | timingHBlank | (3 << 5))
	programDMA(b, 0, source, dest, 1, control)

	d.Trigger(StartHBlank)
	if got, _ := b.Read16(dest, bus.Access{}); got != 0x1111 {
		t.Fatalf("first HBlank value = %04x, want 1111", got)
	}
	if d.Control(0)&controlEnable == 0 {
		t.Fatal("repeat HBlank DMA cleared Enable")
	}
	if got := d.ch[0].countCurrent; got != 1 {
		t.Fatalf("repeat count reload = %d, want 1", got)
	}
	if got := d.ch[0].destCurrent; got != dest {
		t.Fatalf("repeat destination reload = %08x, want %08x", got, dest)
	}
	if got := d.ch[0].sourceCurrent; got != source+2 {
		t.Fatalf("repeat source unexpectedly reloaded = %08x, want %08x", got, source+2)
	}

	d.Trigger(StartHBlank)
	if got, _ := b.Read16(dest, bus.Access{}); got != 0x2222 {
		t.Fatalf("second HBlank value = %04x, want 2222 at reloaded destination", got)
	}
	if got := d.ch[0].sourceCurrent; got != source+4 {
		t.Fatalf("second repeat source = %08x, want %08x", got, source+4)
	}
}

func TestRepeatWithoutDestinationReloadContinuesDestination(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x300)
	dest := uint32(bus.IWRAMStart + 0x300)
	b.Write16(source, 0xaaaa, bus.Access{})
	b.Write16(source+2, 0xbbbb, bus.Access{})

	programDMA(b, 1, source, dest, 1, controlEnable|controlRepeat|timingVBlank)
	d.Trigger(StartVBlank)
	d.Trigger(StartVBlank)

	first, _ := b.Read16(dest, bus.Access{})
	second, _ := b.Read16(dest+2, bus.Access{})
	if first != 0xaaaa || second != 0xbbbb {
		t.Fatalf("repeat increment destination = %04x/%04x, want aaaa/bbbb", first, second)
	}
}

func TestRepeatReloadUsesValuesLatchedOnEnable(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x380)
	destA := uint32(bus.IWRAMStart + 0x380)
	destB := uint32(bus.IWRAMStart + 0x3c0)
	for i, value := range []uint16{1, 2, 3} {
		b.Write16(source+uint32(i*2), value, bus.Access{})
	}

	control := uint16(controlEnable | controlRepeat | timingHBlank | (3 << 5))
	programDMA(b, 0, source, destA, 1, control)
	d.Trigger(StartHBlank)

	// Writes while enabled update the programmer-visible latches but do not
	// replace the internal repeat reload values captured on the enable edge.
	base := dmaBase(0)
	b.Write32(base+4, destB, bus.Access{})
	b.Write16(base+8, 2, bus.Access{})

	d.Trigger(StartHBlank)
	if got, _ := b.Read16(destA, bus.Access{}); got != 2 {
		t.Fatalf("repeat destination changed before re-enable: %04x, want 0002 at original DAD", got)
	}
	if got, _ := b.Read16(destB, bus.Access{}); got != 0 {
		t.Fatalf("updated DAD affected enabled repeat early: %04x", got)
	}
	if got := d.ch[0].countCurrent; got != 1 {
		t.Fatalf("repeat count reload = %d, want originally latched 1", got)
	}

	// A new enable edge re-latches the newly programmed DAD/CNT_L.
	b.Write16(base+10, controlRepeat|timingHBlank|(3<<5), bus.Access{})
	b.Write16(base+10, control, bus.Access{})
	d.Trigger(StartHBlank)

	got0, _ := b.Read16(destB, bus.Access{})
	got1, _ := b.Read16(destB+2, bus.Access{})
	if got0 != 3 || got1 != 0 {
		t.Fatalf("re-enabled repeat produced %04x/%04x, want 0003/0000 from remaining source", got0, got1)
	}
}

func TestTriggerServicesChannelsInHardwarePriorityOrder(t *testing.T) {
	b := bus.New(nil, nil)
	order := []int{}
	var stallHook uint32
	d := New(b, nil, Hooks{
		Complete: func(channel int, units, cycles uint32) {
			order = append(order, channel)
		},
		Stall: func(cycles uint32) {
			stallHook += cycles
		},
	})

	for _, index := range []int{3, 1, 0, 2} {
		source := bus.EWRAMStart + 0x500 + uint32(index*0x20)
		dest := bus.IWRAMStart + 0x500 + uint32(index*0x20)
		b.Write16(source, uint16(index+1), bus.Access{})
		programDMA(b, index, source, dest, 1, controlEnable|timingVBlank)
	}

	stall := d.Trigger(StartVBlank)
	wantOrder := []int{0, 1, 2, 3}
	if len(order) != len(wantOrder) {
		t.Fatalf("completion order = %v, want %v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("completion order = %v, want %v", order, wantOrder)
		}
	}
	if stall != 24 || stallHook != 24 {
		t.Fatalf("priority batch stall return/hook = %d/%d, want 24/24", stall, stallHook)
	}
}

func TestStallHookAlsoAccountsImmediateDMA(t *testing.T) {
	b := bus.New(nil, nil)
	var stall uint32
	d := New(b, nil, Hooks{Stall: func(cycles uint32) { stall += cycles }})

	source := uint32(bus.EWRAMStart + 0x700)
	dest := uint32(bus.IWRAMStart + 0x700)
	b.Write16(source, 0xbeef, bus.Access{})
	programDMA(b, 0, source, dest, 1, controlEnable)

	if stall != 6 {
		t.Fatalf("immediate DMA stall hook = %d, want 6", stall)
	}
	if units, cycles := d.LastTransfer(0); units != 1 || cycles != 6 {
		t.Fatalf("immediate LastTransfer = %d/%d, want 1/6", units, cycles)
	}
}

func TestWrongEventDoesNotStartArmedChannel(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x800)
	dest := uint32(bus.IWRAMStart + 0x800)
	b.Write16(source, 0x4567, bus.Access{})
	programDMA(b, 0, source, dest, 1, controlEnable|timingVBlank)

	if stall := d.Trigger(StartHBlank); stall != 0 {
		t.Fatalf("wrong event stall = %d, want 0", stall)
	}
	if got, _ := b.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("wrong event started DMA: %04x", got)
	}
	if d.Control(0)&controlEnable == 0 {
		t.Fatal("wrong event disarmed DMA")
	}
}

func TestSoftwareDisablePreventsFutureEvent(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0x880)
	dest := uint32(bus.IWRAMStart + 0x880)
	b.Write16(source, 0xcafe, bus.Access{})
	programDMA(b, 0, source, dest, 1, controlEnable|controlRepeat|timingHBlank)

	base := dmaBase(0)
	b.Write16(base+10, controlRepeat|timingHBlank, bus.Access{})
	if d.Control(0)&controlEnable != 0 {
		t.Fatal("software disable did not clear Enable")
	}
	d.Trigger(StartHBlank)
	if got, _ := b.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("disabled channel transferred: %04x", got)
	}
}

func TestReenableRelatchesProgrammedSourceDestinationAndCount(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	sourceA := uint32(bus.EWRAMStart + 0x900)
	sourceB := uint32(bus.EWRAMStart + 0x940)
	destA := uint32(bus.IWRAMStart + 0x900)
	destB := uint32(bus.IWRAMStart + 0x940)
	b.Write16(sourceA, 0x1111, bus.Access{})
	b.Write16(sourceB, 0x2222, bus.Access{})

	programDMA(b, 0, sourceA, destA, 1, controlEnable|timingVBlank)
	d.Trigger(StartVBlank)

	base := dmaBase(0)
	b.Write32(base, sourceB, bus.Access{})
	b.Write32(base+4, destB, bus.Access{})
	b.Write16(base+8, 1, bus.Access{})
	b.Write16(base+10, controlEnable|timingVBlank, bus.Access{})
	d.Trigger(StartVBlank)

	if got, _ := b.Read16(destB, bus.Access{}); got != 0x2222 {
		t.Fatalf("re-enabled DMA result = %04x, want 2222", got)
	}
}

func TestRepeatedDMARequestsIRQOnEveryCompletion(t *testing.T) {
	b := bus.New(nil, nil)
	irq := gbairq.New(b, nil)
	d := New(b, irq, Hooks{})

	source := uint32(bus.EWRAMStart + 0xa00)
	dest := uint32(bus.IWRAMStart + 0xa00)
	b.Write16(source, 1, bus.Access{})
	b.Write16(source+2, 2, bus.Access{})
	programDMA(b, 2, source, dest, 1,
		controlEnable|controlRepeat|controlIRQ|timingHBlank)

	d.Trigger(StartHBlank)
	if got := irq.IF(); got != uint16(gbairq.DMA2) {
		t.Fatalf("first repeated DMA IF = %04x, want DMA2", got)
	}
	b.Write16(bus.IOStart+0x202, uint16(gbairq.DMA2), bus.Access{})
	if got := irq.IF(); got != 0 {
		t.Fatalf("DMA2 IF acknowledge failed: %04x", got)
	}

	d.Trigger(StartHBlank)
	if got := irq.IF(); got != uint16(gbairq.DMA2) {
		t.Fatalf("second repeated DMA IF = %04x, want DMA2", got)
	}
}

func TestSpecialTimingRemainsArmedAndUntriggeredInThisSlice(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.EWRAMStart + 0xb00)
	dest := uint32(bus.IWRAMStart + 0xb00)
	b.Write16(source, 0x9999, bus.Access{})
	programDMA(b, 1, source, dest, 1, controlEnable|timingSpecial)

	d.Trigger(StartVBlank)
	d.Trigger(StartHBlank)
	if got, _ := b.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("special DMA was triggered by blanking event: %04x", got)
	}
	if d.Control(1)&controlEnable == 0 {
		t.Fatal("deferred special DMA did not remain armed")
	}
}
