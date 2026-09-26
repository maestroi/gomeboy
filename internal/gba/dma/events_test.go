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
	if got0 != 1 || got1 != 2 {
		t.Fatalf("re-enabled repeat produced %04x/%04x, want 0001/0002 from re-latched SAD", got0, got1)
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

func TestBlankingEventsDoNotTriggerSpecialDMA(t *testing.T) {
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
		t.Fatal("special DMA did not remain armed")
	}
}

func TestDirectSoundFIFOForcesFourWordBurst(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.IWRAMStart + 0xc00)
	words := []uint32{0x11111111, 0x22222222, 0x33333333, 0x44444444}
	for i, value := range words {
		b.Write32(source+uint32(i*4), value, bus.Access{})
	}

	// Deliberately program a one-unit, 16-bit, incrementing-destination DMA.
	// Direct Sound refill semantics override those fields internally.
	control := uint16(controlEnable | controlRepeat | timingSpecial)
	programDMA(b, 1, source, fifoAAddress, 1, control)

	stall := d.TriggerFIFO(FIFOA)
	if stall != 14 {
		t.Fatalf("FIFO DMA stall = %d, want 14", stall)
	}
	if units, cycles := d.LastTransfer(1); units != 4 || cycles != 14 {
		t.Fatalf("FIFO LastTransfer = %d/%d, want 4/14", units, cycles)
	}
	if got, _ := b.Read32(fifoAAddress, bus.Access{}); got != words[3] {
		t.Fatalf("FIFO A final word = %08x, want %08x", got, words[3])
	}
	if got, _ := b.Read32(fifoBAddress, bus.Access{}); got != 0 {
		t.Fatalf("FIFO B was touched by fixed FIFO A DMA: %08x", got)
	}
	if got := d.ch[1].sourceCurrent; got != source+16 {
		t.Fatalf("FIFO source progression = %08x, want %08x", got, source+16)
	}
	if got := d.ch[1].destCurrent; got != fifoAAddress {
		t.Fatalf("FIFO destination progression = %08x, want fixed %08x", got, fifoAAddress)
	}
	if got := d.Control(1); got != control {
		t.Fatalf("FIFO DMA rewrote programmer-visible control = %04x, want %04x", got, control)
	}
}

func TestDirectSoundFIFOSelectsMatchingFIFOAndChannel(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	sourceA := uint32(bus.IWRAMStart + 0xd00)
	sourceB := uint32(bus.IWRAMStart + 0xe00)
	for i := 0; i < 4; i++ {
		b.Write32(sourceA+uint32(i*4), 0xa0000000+uint32(i), bus.Access{})
		b.Write32(sourceB+uint32(i*4), 0xb0000000+uint32(i), bus.Access{})
	}
	control := uint16(controlEnable | controlRepeat | timingSpecial)
	programDMA(b, 1, sourceA, fifoAAddress, 7, control)
	programDMA(b, 2, sourceB, fifoBAddress, 9, control)

	if stall := d.TriggerFIFO(FIFOA); stall == 0 {
		t.Fatal("FIFO A request did not run DMA1")
	}
	if got, _ := b.Read32(fifoAAddress, bus.Access{}); got != 0xa0000003 {
		t.Fatalf("FIFO A result = %08x, want a0000003", got)
	}
	if got, _ := b.Read32(fifoBAddress, bus.Access{}); got != 0 {
		t.Fatalf("FIFO A request unexpectedly ran DMA2: %08x", got)
	}
	if units, _ := d.LastTransfer(2); units != 0 {
		t.Fatalf("DMA2 ran on FIFO A request: units=%d", units)
	}

	if stall := d.TriggerFIFO(FIFOB); stall == 0 {
		t.Fatal("FIFO B request did not run DMA2")
	}
	if got, _ := b.Read32(fifoBAddress, bus.Access{}); got != 0xb0000003 {
		t.Fatalf("FIFO B result = %08x, want b0000003", got)
	}
	if units, _ := d.LastTransfer(2); units != 4 {
		t.Fatalf("DMA2 FIFO units = %d, want 4", units)
	}
}

func TestDirectSoundFIFOWithoutRepeatDisablesAfterBurst(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.IWRAMStart + 0xf00)
	for i := 0; i < 8; i++ {
		b.Write32(source+uint32(i*4), 0x100+uint32(i), bus.Access{})
	}
	programDMA(b, 1, source, fifoAAddress, 0x20, controlEnable|timingSpecial)

	if stall := d.TriggerFIFO(FIFOA); stall == 0 {
		t.Fatal("one-shot FIFO request did not run")
	}
	if d.Control(1)&controlEnable != 0 {
		t.Fatal("FIFO DMA without Repeat did not clear Enable")
	}
	first, _ := b.Read32(fifoAAddress, bus.Access{})
	if first != 0x103 {
		t.Fatalf("first FIFO burst result = %08x, want 00000103", first)
	}

	if stall := d.TriggerFIFO(FIFOA); stall != 0 {
		t.Fatalf("disabled FIFO DMA ran again with stall %d", stall)
	}
	if got, _ := b.Read32(fifoAAddress, bus.Access{}); got != first {
		t.Fatalf("disabled FIFO DMA changed FIFO = %08x, want %08x", got, first)
	}
}

func TestDirectSoundFIFORejectsOtherSpecialDMAChannelsAndDestinations(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.IWRAMStart + 0x1000)
	b.Write32(source, 0xdeadbeef, bus.Access{})
	programDMA(b, 0, source, fifoAAddress, 1, controlEnable|controlRepeat|timingSpecial)
	programDMA(b, 1, source, bus.IWRAMStart+0x1200, 1, controlEnable|controlRepeat|timingSpecial)
	programDMA(b, 3, source, fifoAAddress, 1, controlEnable|controlRepeat|timingSpecial)

	if stall := d.TriggerFIFO(FIFOA); stall != 0 {
		t.Fatalf("ineligible special DMA produced stall %d", stall)
	}
	if stall := d.TriggerFIFO(SoundFIFO(0)); stall != 0 {
		t.Fatalf("invalid FIFO identifier produced stall %d", stall)
	}
	if got, _ := b.Read32(fifoAAddress, bus.Access{}); got != 0 {
		t.Fatalf("ineligible special DMA wrote FIFO A: %08x", got)
	}
	for _, index := range []int{0, 1, 3} {
		if d.Control(index)&controlEnable == 0 {
			t.Fatalf("DMA%d was disarmed by an ineligible FIFO request", index)
		}
	}
}

func TestVideoCaptureRunsDMA3OnDisplayStartWindow(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.IWRAMStart + 0x1400)
	dest := uint32(bus.VRAMStart + 0x1400)
	for i, value := range []uint16{0x1111, 0x2222, 0x3333} {
		b.Write16(source+uint32(i*2), value, bus.Access{})
	}

	control := uint16(controlEnable | controlRepeat | timingSpecial)
	programDMA(b, 3, source, dest, 1, control)

	for _, line := range []uint16{0, 1, 162, 227} {
		if stall := d.TriggerVideoCapture(line); stall != 0 {
			t.Fatalf("VCOUNT %d capture stall = %d, want 0", line, stall)
		}
	}
	if got, _ := b.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("capture ran outside active window: %04x", got)
	}

	if stall := d.TriggerVideoCapture(2); stall == 0 {
		t.Fatal("VCOUNT 2 did not start video capture")
	}
	if got, _ := b.Read16(dest, bus.Access{}); got != 0x1111 {
		t.Fatalf("VCOUNT 2 capture = %04x, want 1111", got)
	}
	if d.Control(3)&controlEnable == 0 {
		t.Fatal("repeated video capture disabled after first scanline")
	}

	d.TriggerVideoCapture(3)
	if got, _ := b.Read16(dest+2, bus.Access{}); got != 0x2222 {
		t.Fatalf("VCOUNT 3 capture = %04x, want 2222", got)
	}

	// VCOUNT 161 is the final capture line. It still transfers, then clears
	// Enable even though Repeat is programmed.
	d.TriggerVideoCapture(161)
	if got, _ := b.Read16(dest+4, bus.Access{}); got != 0x3333 {
		t.Fatalf("VCOUNT 161 capture = %04x, want 3333", got)
	}
	if d.Control(3)&controlEnable != 0 {
		t.Fatal("video capture remained enabled after final scanline")
	}
	if got := d.ch[3].sourceCurrent; got != source+6 {
		t.Fatalf("video capture source progression = %08x, want %08x", got, source+6)
	}
}

func TestVideoCaptureOnlyUsesDMA3SpecialWithoutDRQ(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.IWRAMStart + 0x1500)
	b.Write16(source, 0xabcd, bus.Access{})

	// DMA0 special timing is prohibited for video capture.
	programDMA(b, 0, source, bus.IWRAMStart+0x1600, 1, controlEnable|controlRepeat|timingSpecial)
	if stall := d.TriggerVideoCapture(2); stall != 0 {
		t.Fatalf("DMA0 special produced capture stall %d", stall)
	}
	if units, _ := d.LastTransfer(0); units != 0 {
		t.Fatalf("DMA0 ran as video capture: units=%d", units)
	}

	// DMA3 DRQ mode overrides the display-synchronized special timing.
	programDMA(b, 3, source, bus.IWRAMStart+0x1700, 1,
		controlEnable|controlGamePakDRQ|controlRepeat|timingSpecial)
	if stall := d.TriggerVideoCapture(2); stall != 0 {
		t.Fatalf("DRQ DMA3 ran as video capture with stall %d", stall)
	}
	if units, _ := d.LastTransfer(3); units != 0 {
		t.Fatalf("DRQ DMA3 ran on video capture trigger: units=%d", units)
	}
}

func TestGamePakDRQOverridesProgrammedStartTiming(t *testing.T) {
	rom := []byte{0x11, 0x11, 0x22, 0x22}
	b := bus.New(nil, rom)
	d := New(b, nil, Hooks{})

	dest := uint32(bus.IWRAMStart + 0x1800)
	control := uint16(controlEnable | controlGamePakDRQ | controlRepeat | timingHBlank)
	programDMA(b, 3, bus.ROM0Start, dest, 2, control)

	// Bit 11 replaces the ordinary timing source, so HBlank must not start it.
	if stall := d.Trigger(StartHBlank); stall != 0 {
		t.Fatalf("HBlank started DRQ DMA with stall %d", stall)
	}
	if got, _ := b.Read16(dest, bus.Access{}); got != 0 {
		t.Fatalf("DRQ DMA ran before cartridge request: %04x", got)
	}

	if stall := d.TriggerGamePakDRQ(); stall == 0 {
		t.Fatal("Game Pak request did not start DMA3")
	}
	got0, _ := b.Read16(dest, bus.Access{})
	got1, _ := b.Read16(dest+2, bus.Access{})
	if got0 != 0x1111 || got1 != 0x2222 {
		t.Fatalf("Game Pak DRQ result = %04x/%04x, want 1111/2222", got0, got1)
	}
	if units, _ := d.LastTransfer(3); units != 2 {
		t.Fatalf("Game Pak DRQ units = %d, want 2", units)
	}

	// Hardware requires Repeat=0 in DRQ mode. If software nevertheless leaves
	// the bit set, the request still completes as a one-shot transfer.
	if d.Control(3)&controlEnable != 0 {
		t.Fatal("Game Pak DRQ remained enabled because Repeat was set")
	}
	if d.Control(3)&controlRepeat == 0 {
		t.Fatal("DRQ completion rewrote programmer-visible Repeat bit")
	}
	if stall := d.TriggerGamePakDRQ(); stall != 0 {
		t.Fatalf("completed DRQ DMA accepted another request with stall %d", stall)
	}
}

func TestGamePakDRQWithImmediateTimingWaitsForRequest(t *testing.T) {
	b := bus.New(nil, nil)
	d := New(b, nil, Hooks{})

	source := uint32(bus.IWRAMStart + 0x1900)
	dest := uint32(bus.IWRAMStart + 0x1a00)
	b.Write32(source, 0x44332211, bus.Access{})

	programDMA(b, 3, source, dest, 1,
		controlEnable|controlGamePakDRQ|controlWord|timingImmediate)

	if got, _ := b.Read32(dest, bus.Access{}); got != 0 {
		t.Fatalf("immediate-timing DRQ DMA ran on Enable: %08x", got)
	}
	if d.Control(3)&controlEnable == 0 {
		t.Fatal("DRQ DMA did not remain armed for cartridge request")
	}

	d.TriggerGamePakDRQ()
	if got, _ := b.Read32(dest, bus.Access{}); got != 0x44332211 {
		t.Fatalf("DRQ-triggered word = %08x, want 44332211", got)
	}
}
