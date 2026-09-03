package io

import (
	"github.com/maestroi/gomeboy/internal/types"
)

// CartridgeState is a snapshot of a Cartridge's mutable runtime state.
// The ROM header fields (title, sizes, checksums, ...) are derived from the
// ROM image and are therefore not part of the state.
type CartridgeState struct {
	RAM        []byte
	RAMEnabled bool
	ROMOffset  uint32
	RAMOffset  uint32

	MBC1 struct {
		BankShift uint8
		Bank1     uint8
		Bank2     uint8
		Mode      bool
	}
	M161Latched bool
	MBC7        struct {
		LatchReady bool
		RAMEnabled bool
		XLatch     uint16
		YLatch     uint16
		Eeprom     struct {
			DO, DI, CLK, CS bool
			WriteEnabled    bool
			Command         uint16
			BitsIn, BitsOut uint16
			BitsLeft        uint8
		}
	}
	HUC1 struct {
		IRMode bool
	}
	RTC struct {
		Enabled, Latched, Latching bool
		Register                   uint8
		LastUpdate, HeldTicks      uint64
	}
	AccelerometerX, AccelerometerY float32
	Camera                         *CameraState
}

// CameraState is a snapshot of a Pocket Camera cartridge's register state.
// The camera's image source is external input and is not part of the state.
type CameraState struct {
	Registers       [0x36]uint8
	RegistersMapped bool
}

// Snapshot captures the Cartridge's mutable runtime state.
func (c *Cartridge) Snapshot() CartridgeState {
	st := CartridgeState{
		RAM:        append([]byte(nil), c.RAM...),
		RAMEnabled: c.ramEnabled,
		ROMOffset:  c.romOffset,
		RAMOffset:  c.ramOffset,
	}
	st.MBC1.BankShift = c.mbc1.bankShift
	st.MBC1.Bank1 = c.mbc1.bank1
	st.MBC1.Bank2 = c.mbc1.bank2
	st.MBC1.Mode = c.mbc1.mode
	st.M161Latched = c.m161Latched
	st.MBC7.LatchReady = c.mbc7.latchReady
	st.MBC7.RAMEnabled = c.mbc7.ramEnabled
	st.MBC7.XLatch = c.mbc7.xLatch
	st.MBC7.YLatch = c.mbc7.yLatch
	st.MBC7.Eeprom.DO = c.mbc7.eeprom.do
	st.MBC7.Eeprom.DI = c.mbc7.eeprom.di
	st.MBC7.Eeprom.CLK = c.mbc7.eeprom.clk
	st.MBC7.Eeprom.CS = c.mbc7.eeprom.cs
	st.MBC7.Eeprom.WriteEnabled = c.mbc7.eeprom.writeEnabled
	st.MBC7.Eeprom.Command = c.mbc7.eeprom.command
	st.MBC7.Eeprom.BitsIn = c.mbc7.eeprom.bitsIn
	st.MBC7.Eeprom.BitsOut = c.mbc7.eeprom.bitsOut
	st.MBC7.Eeprom.BitsLeft = c.mbc7.eeprom.bitsLeft
	st.HUC1.IRMode = c.huc1.irMode
	st.RTC.Enabled = c.rtc.enabled
	st.RTC.Latched = c.rtc.latched
	st.RTC.Latching = c.rtc.latching
	st.RTC.Register = c.rtc.register
	st.RTC.LastUpdate = c.rtc.lastUpdate
	st.RTC.HeldTicks = c.rtc.heldTicks
	st.AccelerometerX = c.AccelerometerX
	st.AccelerometerY = c.AccelerometerY
	if c.Camera != nil {
		st.Camera = &CameraState{
			Registers:       c.Camera.Registers,
			RegistersMapped: c.Camera.registersMapped,
		}
	}
	return st
}

// Restore rebuilds the Cartridge's mutable runtime state from a snapshot.
func (c *Cartridge) Restore(s CartridgeState) {
	if len(s.RAM) == len(c.RAM) {
		copy(c.RAM, s.RAM)
	}
	c.ramEnabled = s.RAMEnabled
	c.romOffset = s.ROMOffset
	c.ramOffset = s.RAMOffset
	c.mbc1.bankShift = s.MBC1.BankShift
	c.mbc1.bank1 = s.MBC1.Bank1
	c.mbc1.bank2 = s.MBC1.Bank2
	c.mbc1.mode = s.MBC1.Mode
	c.m161Latched = s.M161Latched
	c.mbc7.latchReady = s.MBC7.LatchReady
	c.mbc7.ramEnabled = s.MBC7.RAMEnabled
	c.mbc7.xLatch = s.MBC7.XLatch
	c.mbc7.yLatch = s.MBC7.YLatch
	c.mbc7.eeprom.do = s.MBC7.Eeprom.DO
	c.mbc7.eeprom.di = s.MBC7.Eeprom.DI
	c.mbc7.eeprom.clk = s.MBC7.Eeprom.CLK
	c.mbc7.eeprom.cs = s.MBC7.Eeprom.CS
	c.mbc7.eeprom.writeEnabled = s.MBC7.Eeprom.WriteEnabled
	c.mbc7.eeprom.command = s.MBC7.Eeprom.Command
	c.mbc7.eeprom.bitsIn = s.MBC7.Eeprom.BitsIn
	c.mbc7.eeprom.bitsOut = s.MBC7.Eeprom.BitsOut
	c.mbc7.eeprom.bitsLeft = s.MBC7.Eeprom.BitsLeft
	c.huc1.irMode = s.HUC1.IRMode
	c.rtc.enabled = s.RTC.Enabled
	c.rtc.latched = s.RTC.Latched
	c.rtc.latching = s.RTC.Latching
	c.rtc.register = s.RTC.Register
	c.rtc.lastUpdate = s.RTC.LastUpdate
	c.rtc.heldTicks = s.RTC.HeldTicks
	c.AccelerometerX = s.AccelerometerX
	c.AccelerometerY = s.AccelerometerY
	if s.Camera != nil && c.Camera != nil {
		c.Camera.Registers = s.Camera.Registers
		c.Camera.registersMapped = s.Camera.RegistersMapped
	}
}

// State is a snapshot of the Bus's execution state, including the
// entire 64 KiB address space, the work/VRAM banks, the cartridge,
// the joypad, and the DMA/HDMA transfer state.
type State struct {
	Data         [0x10000]byte
	WRAM         [7][0x1000]byte
	VRAM         [2][0x2000]byte
	Cartridge    CartridgeState
	ButtonState  uint8
	IME          bool
	BootROMDone  bool
	VRAMBankMask uint8
	Model        types.Model
	IsGBC        bool

	// DMA
	DMASource, DMADestination  uint16
	DMAActive, DMARestarting   bool
	DMAConflict                uint8
	DMAEnabled                 bool
	RegionLocks, DMAConflicted uint16

	// HDMA/GDMA
	HDMASource, HDMADestination uint16
	DMALength, DMARemaining     uint8
	DMAComplete, DMAPaused      bool
}

// Snapshot captures the Bus's execution state.
func (b *Bus) Snapshot() State {
	st := State{
		Data:            b.data,
		WRAM:            b.wRAM,
		VRAM:            b.VRAM,
		Cartridge:       b.c.Snapshot(),
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
	return st
}

// Restore rebuilds the Bus's execution state from a snapshot.
func (b *Bus) Restore(s State) {
	b.data = s.Data
	b.wRAM = s.WRAM
	b.VRAM = s.VRAM
	b.c.Restore(s.Cartridge)
	b.buttonState = s.ButtonState
	b.ime = s.IME
	b.bootROMDone = s.BootROMDone
	b.vRAMBankMask = s.VRAMBankMask
	b.model = s.Model
	b.isGBC = s.IsGBC
	b.dmaSource = s.DMASource
	b.dmaDestination = s.DMADestination
	b.dmaActive = s.DMAActive
	b.dmaRestarting = s.DMARestarting
	b.dmaConflict = s.DMAConflict
	b.dmaEnabled = s.DMAEnabled
	b.regionLocks = s.RegionLocks
	b.dmaConflicted = s.DMAConflicted
	b.hdmaSource = s.HDMASource
	b.hdmaDestination = s.HDMADestination
	b.dmaLength = s.DMALength
	b.dmaRemaining = s.DMARemaining
	b.dmaComplete = s.DMAComplete
	b.dmaPaused = s.DMAPaused
}
