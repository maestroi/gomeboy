package main

import (
	"fmt"

	"github.com/maestroi/gomeboy/internal/gameboy"
	"github.com/maestroi/gomeboy/internal/io"
)

const rom = "/home/maestro/Documents/projects/PokePilot/roms/pokemon_red.gb"

func main() {
	gb := gameboy.NewGameBoy(gameboy.WithoutSaves())
	if err := gb.LoadROM(rom); err != nil {
		panic(err)
	}

	stepN := func(n int) {
		for i := 0; i < n; i++ {
			gb.Step()
		}
	}
	tap := func(b io.Button) {
		gb.Bus.Press(b)
		stepN(3)
		gb.Bus.Release(b)
		stepN(7)
	}

	// Replicate skill.BootToOverworld exactly.
	stepN(300)
	tap(io.ButtonStart)
	stepN(30)
	tap(io.ButtonA)
	stepN(30)
	tap(io.ButtonDown)
	stepN(10)
	tap(io.ButtonA)
	stepN(30)
	tap(io.ButtonDown)
	stepN(10)
	tap(io.ButtonA)
	stepN(30)

	const budget = 3000
	start := gb.FrameCount()
	booted := false
	for gb.FrameCount() < start+uint64(budget) {
		stepN(10)
		tap(io.ButtonA)
		curMap := gb.Bus.Get(0xD35E)
		fontLoaded := gb.Bus.Get(0xCFC4)
		joyIgnore := gb.Bus.Get(0xCD6B)
		walkCounter := gb.Bus.Get(0xCFC5)
		if curMap == 0x26 && fontLoaded == 0 && joyIgnore == 0 && walkCounter == 0 {
			booted = true
			break
		}
	}
	fmt.Printf("booted=%v at frame %d\n", booted, gb.FrameCount())

	// Dump the live vector table (0x0038-0x0060) and compare to the ROM file.
	fmt.Print("live  0x0038-0x005f: ")
	for i := 0x38; i < 0x60; i++ {
		fmt.Printf("%02x ", gb.Bus.Get(uint16(i)))
	}
	fmt.Println()
	fmt.Print("rom   0x0038-0x005f: ")
	for i := 0x38; i < 0x60; i++ {
		fmt.Printf("%02x ", gb.ROM[i])
	}
	fmt.Println()
	cs := gb.CPU.Snapshot()
	fmt.Printf("halted=%v skipHalt=%v hasInt=%v hasFrame=%v haltBug=%v\n",
		cs.Halted, cs.SkippingHalt, cs.HasInt, cs.HasFrame, cs.HaltBug)
	fmt.Println("stack top (SP-8..SP):")
	sp := gb.CPU.SP
	for i := 8; i >= 0; i -= 2 {
		a := sp - uint16(i)
		fmt.Printf("  [%04x]=%02x ", a, gb.Bus.Get(a))
	}
	fmt.Println()

	dump(gb, 0)

	// Observe what happens over 5000 frames; report when OAM first becomes
	// non-zero and when the player first moves.
	oamSeen := -1
	movedSeen := -1
	sx, sy := gb.Bus.Get(0xD362), gb.Bus.Get(0xD361)
	for i := 1; i <= 500; i++ {
		stepN(10)
		var oamNonZero bool
		for j := 0; j < 0xA0; j++ {
			if gb.Bus.Get(0xFE00+uint16(j)) != 0 {
				oamNonZero = true
				break
			}
		}
		if oamNonZero && oamSeen < 0 {
			oamSeen = i * 10
		}
		if x := gb.Bus.Get(0xD362); x != sx {
			if movedSeen < 0 {
				movedSeen = i * 10
			}
		}
		if y := gb.Bus.Get(0xD361); y != sy {
			if movedSeen < 0 {
				movedSeen = i * 10
			}
		}
		if i%50 == 0 || i <= 5 {
			dump(gb, i)
		}
	}
	fmt.Printf("oam first non-zero at frame %d; player first moved at frame %d\n", oamSeen, movedSeen)
}

func dump(gb *gameboy.GameBoy, i int) {
	x := gb.Bus.Get(0xD362)
	y := gb.Bus.Get(0xD361)
	wc := gb.Bus.Get(0xCFC5)
	dir := gb.Bus.Get(0xD52A)
	breg := gb.Bus.Get(0xFF42)
	creg := gb.Bus.Get(0xFF43)
	ldc := gb.Bus.Get(0xFF40)
	ly := gb.Bus.Get(0xFF44)
	ifr := gb.Bus.Get(0xFF0F)
	ier := gb.Bus.Get(0xFF0E)
	var oam [16]byte
	for i := range oam { oam[i] = gb.Bus.Get(0xFE00 + uint16(i)) }
	fmt.Printf("t=%3d pc=%04x sp=%04x x=%d y=%d wc=%d dir=%d B=%02x C=%02x LCDC=%02x LY=%02x IF=%02x IE=%02x OAM=%x\n",
		i*10, gb.CPU.PC, gb.CPU.SP, x, y, wc, dir, breg, creg, ldc, ly, ifr, ier, oam)
}
