// Command multihw-fixture builds the project-owned GBA compatibility ROM used
// to exercise keypad input, Direct Sound, timers, and SRAM in one bounded run.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	passSignature uint32 = 0x504d4f43 // "COMP" in little-endian memory
	failSignature uint32 = 0x4c494146 // "FAIL" in little-endian memory
)

func main() {
	output := flag.String("output", "", "output .gba path")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-output is required")
		os.Exit(2)
	}

	// Direct-boot ARM program:
	//   1. wait for A through KEYINPUT;
	//   2. configure Direct Sound A, push one FIFO word, and start Timer0;
	//   3. write/read SRAM byte 0x5a;
	//   4. publish "COMP" to EWRAM only after every check succeeds;
	//   5. remain alive so the compatibility runner can require bounded stability.
	//
	// Literal loads keep the binary independent of an external assembler.
	words := []uint32{
		0xe59f007c, // ldr  r0, =0x04000130 (KEYINPUT)
		0xe1d010b0, // ldrh r1, [r0]
		0xe3110001, // tst  r1, #1 (A is active-low)
		0x1afffffc, // bne  wait_for_a

		0xe59f0070, // ldr  r0, =0x04000082 (SOUNDCNT_H)
		0xe59f1070, // ldr  r1, =0x00000304 (FIFO A full volume, L+R)
		0xe1c010b0, // strh r1, [r0]
		0xe1d020b0, // ldrh r2, [r0]
		0xe1520001, // cmp  r2, r1
		0x1a000012, // bne  fail

		0xe59f0060, // ldr  r0, =0x040000a0 (FIFO_A)
		0xe59f1060, // ldr  r1, =0x7f008140
		0xe5801000, // str  r1, [r0]

		0xe59f005c, // ldr  r0, =0x04000100 (TM0CNT_L)
		0xe59f105c, // ldr  r1, =0x0000ff00
		0xe1c010b0, // strh r1, [r0]
		0xe59f0058, // ldr  r0, =0x04000102 (TM0CNT_H)
		0xe59f1058, // ldr  r1, =0x00000080 (enable, /1)
		0xe1c010b0, // strh r1, [r0]

		0xe59f0054, // ldr  r0, =0x0e000000 (SRAM)
		0xe3a0105a, // mov  r1, #0x5a
		0xe5c01000, // strb r1, [r0]
		0xe5d02000, // ldrb r2, [r0]
		0xe1520001, // cmp  r2, r1
		0x1a000003, // bne  fail

		0xe59f0040, // ldr  r0, =0x02000010 (result)
		0xe59f1040, // ldr  r1, =passSignature
		0xe5801000, // str  r1, [r0]
		0xeafffffe, // pass: b pass

		0xe59f0030, // fail: ldr r0, =0x02000010
		0xe59f1034, // ldr  r1, =failSignature
		0xe5801000, // str  r1, [r0]
		0xeafffffb, // b fail

		0x04000130,
		0x04000082,
		0x00000304,
		0x040000a0,
		0x7f008140,
		0x04000100,
		0x0000ff00,
		0x04000102,
		0x00000080,
		0x0e000000,
		0x02000010,
		passSignature,
		failSignature,
	}

	rom := make([]byte, len(words)*4, len(words)*4+len("SRAM_V"))
	for index, word := range words {
		binary.LittleEndian.PutUint32(rom[index*4:], word)
	}
	// Standard save-library marker makes the normal cartridge detector attach
	// SRAM; this is data only and is never executed.
	rom = append(rom, "SRAM_V"...)

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*output, rom, 0o644); err != nil {
		panic(err)
	}
}
