package io

// SnapshotInto captures cartridge state into dst while reusing its RAM storage
// across repeated checkpoints.
func (c *Cartridge) SnapshotInto(dst *CartridgeState) {
	ram := dst.RAM
	camera := dst.Camera
	*dst = CartridgeState{}
	if cap(ram) < len(c.RAM) {
		ram = make([]byte, len(c.RAM))
	} else {
		ram = ram[:len(c.RAM)]
	}
	copy(ram, c.RAM)
	dst.RAM = ram
	dst.RAMEnabled = c.ramEnabled
	dst.ROMOffset = c.romOffset
	dst.RAMOffset = c.ramOffset
	dst.MBC1.BankShift = c.mbc1.bankShift
	dst.MBC1.Bank1 = c.mbc1.bank1
	dst.MBC1.Bank2 = c.mbc1.bank2
	dst.MBC1.Mode = c.mbc1.mode
	dst.M161Latched = c.m161Latched
	dst.MBC7.LatchReady = c.mbc7.latchReady
	dst.MBC7.RAMEnabled = c.mbc7.ramEnabled
	dst.MBC7.XLatch = c.mbc7.xLatch
	dst.MBC7.YLatch = c.mbc7.yLatch
	dst.MBC7.Eeprom.DO = c.mbc7.eeprom.do
	dst.MBC7.Eeprom.DI = c.mbc7.eeprom.di
	dst.MBC7.Eeprom.CLK = c.mbc7.eeprom.clk
	dst.MBC7.Eeprom.CS = c.mbc7.eeprom.cs
	dst.MBC7.Eeprom.WriteEnabled = c.mbc7.eeprom.writeEnabled
	dst.MBC7.Eeprom.Command = c.mbc7.eeprom.command
	dst.MBC7.Eeprom.BitsIn = c.mbc7.eeprom.bitsIn
	dst.MBC7.Eeprom.BitsOut = c.mbc7.eeprom.bitsOut
	dst.MBC7.Eeprom.BitsLeft = c.mbc7.eeprom.bitsLeft
	dst.HUC1.IRMode = c.huc1.irMode
	dst.RTC.Enabled = c.rtc.enabled
	dst.RTC.Latched = c.rtc.latched
	dst.RTC.Latching = c.rtc.latching
	dst.RTC.Register = c.rtc.register
	dst.RTC.LastUpdate = c.rtc.lastUpdate
	dst.RTC.HeldTicks = c.rtc.heldTicks
	dst.AccelerometerX = c.AccelerometerX
	dst.AccelerometerY = c.AccelerometerY
	if c.Camera != nil {
		if camera == nil {
			camera = &CameraState{}
		}
		camera.Registers = c.Camera.Registers
		camera.RegistersMapped = c.Camera.registersMapped
		dst.Camera = camera
	}
}

// SnapshotInto captures bus state into dst while preserving reusable cartridge
// checkpoint storage.
func (b *Bus) SnapshotInto(dst *State) {
	cartridge := dst.Cartridge
	*dst = State{
		Data:            b.data,
		WRAM:            b.wRAM,
		VRAM:            b.VRAM,
		ButtonState:     b.buttonState,
		IME:             b.ime,
		BootROMDone:     b.bootROMDone,
		VRAMBankMask:    b.vRAMBankMask,
		Model:           b.model,
		IsGBC:           b.isGBC,
		DMASource:       b.dmaSource,
		DMADestination:  b.dmaDestination,
		DMAActive:       b.dmaActive,
		DMARestarting:   b.dmaRestarting,
		DMAConflict:     b.dmaConflict,
		DMAEnabled:      b.dmaEnabled,
		RegionLocks:     b.regionLocks,
		DMAConflicted:   b.dmaConflicted,
		HDMASource:      b.hdmaSource,
		HDMADestination: b.hdmaDestination,
		DMALength:       b.dmaLength,
		DMARemaining:    b.dmaRemaining,
		DMAComplete:     b.dmaComplete,
		DMAPaused:       b.dmaPaused,
	}
	dst.Cartridge = cartridge
	b.c.SnapshotInto(&dst.Cartridge)
}
