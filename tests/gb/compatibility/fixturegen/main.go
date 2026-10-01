// Command fixturegen builds the project-owned GB/GBC compatibility ROMs.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

var nintendoLogo = []byte{
	0xCE, 0xED, 0x66, 0x66, 0xCC, 0x0D, 0x00, 0x0B,
	0x03, 0x73, 0x00, 0x83, 0x00, 0x0C, 0x00, 0x0D,
	0x00, 0x08, 0x11, 0x1F, 0x88, 0x89, 0x00, 0x0E,
	0xDC, 0xCC, 0x6E, 0xE6, 0xDD, 0xDD, 0xD9, 0x99,
	0xBB, 0xBB, 0x67, 0x63, 0x6E, 0x0E, 0xEC, 0xCC,
	0xDD, 0xDC, 0x99, 0x9F, 0xBB, 0xB9, 0x33, 0x3E,
}

type fixup struct {
	offset int
	label  string
}

type assembler struct {
	code   []byte
	labels map[string]int
	fixups []fixup
}

func newAssembler() *assembler { return &assembler{labels: map[string]int{}} }

func (a *assembler) emit(values ...byte) { a.code = append(a.code, values...) }

func (a *assembler) label(name string) { a.labels[name] = len(a.code) }

func (a *assembler) jr(op byte, label string) {
	a.emit(op, 0)
	a.fixups = append(a.fixups, fixup{offset: len(a.code) - 1, label: label})
}

func (a *assembler) resolve() []byte {
	for _, f := range a.fixups {
		target, ok := a.labels[f.label]
		if !ok {
			panic("missing label: " + f.label)
		}
		delta := target - (f.offset + 1)
		if delta < -128 || delta > 127 {
			panic(fmt.Sprintf("jump to %s out of range: %d", f.label, delta))
		}
		a.code[f.offset] = byte(int8(delta))
	}
	return a.code
}

func main() {
	out := flag.String("out", "tests/gb/compatibility/roms", "output directory")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		panic(err)
	}

	fixtures := map[string][]byte{
		"dmg-input-render.gb":  visualROM(false),
		"cgb-input-render.gbc": visualROM(true),
		"mbc1-banking.gb":     mapperROM("mbc1", false),
		"mbc3-persistence.gb": mapperROM("mbc3", true),
		"mbc5-banking.gb":     mapperROM("mbc5", false),
	}
	for name, data := range fixtures {
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o644); err != nil {
			panic(err)
		}
	}
}

func visualROM(cgb bool) []byte {
	a := newAssembler()
	a.emit(0xF3, 0x31, 0xFE, 0xFF) // DI; LD SP,$FFFE
	a.emit(0xAF, 0xE0, 0x40)       // XOR A; LDH (LCDC),A

	// Three tiles exercise BG/window/OBJ paths.
	a.emit(0x21, 0x00, 0x80) // LD HL,$8000
	patterns := make([]byte, 0, 48)
	for i := 0; i < 8; i++ {
		patterns = append(patterns, 0xAA, 0x00)
	}
	for i := 0; i < 8; i++ {
		patterns = append(patterns, 0x55, 0xFF)
	}
	for i := 0; i < 8; i++ {
		patterns = append(patterns, 0xFF, 0xFF)
	}
	for _, value := range patterns {
		a.emit(0x3E, value, 0x22) // LD A,n; LD (HL+),A
	}

	a.emit(0x3E, 0x01)
	writeA16(a, 0x9C00) // first window tile = tile 1

	for _, entry := range []struct {
		addr uint16
		val  byte
	}{
		{0xFE00, 72}, // Y=56
		{0xFE01, 98}, // X=90
		{0xFE02, 2},
		{0xFE03, 0},
	} {
		a.emit(0x3E, entry.val)
		writeA16(a, entry.addr)
	}

	a.emit(0x3E, 0xE4, 0xE0, 0x47) // BGP
	a.emit(0x3E, 0xE4, 0xE0, 0x48) // OBP0

	if cgb {
		// CGB BG palette 0: white, red, green, black.
		a.emit(0x3E, 0x80, 0xE0, 0x68)
		for _, value := range []byte{0xFF, 0x7F, 0x1F, 0x00, 0xE0, 0x03, 0x00, 0x00} {
			a.emit(0x3E, value, 0xE0, 0x69)
		}
		// CGB OBJ palette 0.
		a.emit(0x3E, 0x80, 0xE0, 0x6A)
		for _, value := range []byte{0xFF, 0x7F, 0x1F, 0x00, 0xE0, 0x03, 0x00, 0x00} {
			a.emit(0x3E, value, 0xE0, 0x6B)
		}
	}

	for _, reg := range []struct {
		addr byte
		val  byte
	}{
		{0x43, 3},  // SCX: exercise scrolling
		{0x4A, 48}, // WY
		{0x4B, 87}, // WX
	} {
		a.emit(0x3E, reg.val, 0xE0, reg.addr)
	}

	// Exercise free-running timer and internal-clock serial state while the
	// smoke fixture waits for input.
	a.emit(0x3E, 0x05, 0xE0, 0x07) // TAC
	a.emit(0x3E, 0x55, 0xE0, 0x01) // SB
	a.emit(0x3E, 0x81, 0xE0, 0x02) // SC

	a.emit(0x3E, 0xF3, 0xE0, 0x40) // LCDC: LCD/window/OBJ/BG on

	a.label("wait")
	a.emit(0x3E, 0x10, 0xE0, 0x00) // select action buttons
	a.emit(0xF0, 0x00, 0xE6, 0x01) // read P1, test A
	a.jr(0x20, "wait")              // JR NZ

	writeSignature(a, "INPT")
	a.label("loop")
	a.jr(0x18, "loop")

	title := "GBSMOKEDMG"
	flag := byte(0)
	if cgb {
		title = "GBSMOKECGB"
		flag = 0xC0
	}
	rom := baseROM(title, 0x00, 0x00, 0x00, flag)
	return finishROM(rom, a.resolve())
}

func mapperROM(kind string, persistence bool) []byte {
	cartridgeType := map[string]byte{
		"mbc1": 0x03,
		"mbc3": 0x13,
		"mbc5": 0x1B,
	}[kind]

	a := newAssembler()
	a.emit(0xF3, 0x31, 0xFE, 0xFF)
	a.emit(0x3E, 0x0A)
	writeA16(a, 0x0000) // enable cartridge RAM

	if persistence {
		readA16(a, 0xA000)
		a.emit(0xFE, 0x42)
		a.jr(0x28, "persisted")
	}

	a.emit(0x3E, 0x02)
	writeA16(a, 0x2000)
	if kind == "mbc5" {
		a.emit(0xAF)
		writeA16(a, 0x3000)
	}
	readA16(a, 0x4000)
	a.emit(0xFE, 0x02)
	a.jr(0x20, "fail")

	if persistence {
		a.emit(0x3E, 0x42)
		writeA16(a, 0xA000)
		writeSignature(a, "INIT")
		a.jr(0x18, "loop")

		a.label("persisted")
		a.emit(0x3E, 0x02)
		writeA16(a, 0x2000)
		readA16(a, 0x4000)
		a.emit(0xFE, 0x02)
		a.jr(0x20, "fail")
		writeSignature(a, "PERS")
		a.jr(0x18, "loop")
	} else {
		a.emit(0x3E, 0x5A)
		writeA16(a, 0xA000)
		readA16(a, 0xA000)
		a.emit(0xFE, 0x5A)
		a.jr(0x20, "fail")
		writeSignature(a, "COMP")
		a.jr(0x18, "loop")
	}

	a.label("fail")
	writeSignature(a, "FAIL")
	a.label("loop")
	a.jr(0x18, "loop")

	rom := baseROM(stringsUpper(kind)+"SMOKE", cartridgeType, 0x01, 0x03, 0)
	for bank := 1; bank < 4; bank++ {
		rom[bank*0x4000] = byte(bank)
	}
	return finishROM(rom, a.resolve())
}

func writeA16(a *assembler, addr uint16) {
	a.emit(0xEA, byte(addr), byte(addr>>8))
}

func readA16(a *assembler, addr uint16) {
	a.emit(0xFA, byte(addr), byte(addr>>8))
}

func writeSignature(a *assembler, value string) {
	for i := 0; i < len(value); i++ {
		a.emit(0x3E, value[i])
		writeA16(a, 0xC000+uint16(i))
	}
}

func baseROM(title string, cartridgeType, romSize, ramSize, cgbFlag byte) []byte {
	banks := 2
	if romSize == 0x01 {
		banks = 4
	}
	rom := make([]byte, banks*0x4000)
	for i := range rom {
		rom[i] = 0xFF
	}
	copy(rom[0x100:0x104], []byte{0x00, 0xC3, 0x50, 0x01})
	copy(rom[0x104:0x134], nintendoLogo)
	if len(title) > 15 {
		title = title[:15]
	}
	copy(rom[0x134:0x143], []byte(title))
	rom[0x143] = cgbFlag
	rom[0x146] = 0
	rom[0x147] = cartridgeType
	rom[0x148] = romSize
	rom[0x149] = ramSize
	rom[0x14A] = 1
	rom[0x14B] = 0x33
	rom[0x14C] = 0
	return rom
}

func finishROM(rom, code []byte) []byte {
	copy(rom[0x150:], code)

	var header byte
	for i := 0x134; i <= 0x14C; i++ {
		header = header - rom[i] - 1
	}
	rom[0x14D] = header
	rom[0x14E], rom[0x14F] = 0, 0

	var global uint32
	for _, value := range rom {
		global += uint32(value)
	}
	rom[0x14E] = byte(global >> 8)
	rom[0x14F] = byte(global)
	return rom
}

func stringsUpper(value string) string {
	out := []byte(value)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - ('a' - 'A')
		}
	}
	return string(out)
}
