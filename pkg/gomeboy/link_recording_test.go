package gomeboy

import (
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/maestroi/gomeboy/pkg/link"
)

// serialLoopROM idles ~3 frames, then alternates one internally-clocked and
// one externally-clocked byte transfer forever, storing every received byte
// at $C000+.
func serialLoopROM() []byte {
	rom := perfROM()
	copy(rom[0x0100:], []byte{
		0x01, 0x00, 0x20, // LD BC,$2000
		0x0B,       // delay: DEC BC
		0x78,       // LD A,B
		0xB1,       // OR C
		0x20, 0xFB, // JR NZ,delay
		0x21, 0x00, 0xC0, // LD HL,$C000
		0x3E, 0x5A, // loop: LD A,$5A
		0xE0, 0x01, // LDH (SB),A
		0x3E, 0x81, // LD A,$81 (start, internal clock)
		0xE0, 0x02, // LDH (SC),A
		0xF0, 0x02, // wait1: LDH A,(SC)
		0x87,       // ADD A,A (bit 7 -> carry)
		0x38, 0xFB, // JR C,wait1
		0xF0, 0x01, // LDH A,(SB)
		0x22,       // LD (HL+),A
		0x3E, 0x80, // LD A,$80 (start, external clock)
		0xE0, 0x02, // LDH (SC),A
		0xF0, 0x02, // wait2: LDH A,(SC)
		0x87,       // ADD A,A
		0x38, 0xFB, // JR C,wait2
		0xF0, 0x01, // LDH A,(SB)
		0x22,       // LD (HL+),A
		0x18, 0xE2, // JR loop
	})
	return rom
}

// randomLinkPeer answers the emulator's clocks with random bits and drives
// external clock pulses after random wall-clock delays, so readiness depends
// on scheduling and cannot be reproduced from joypad input alone.
func randomLinkPeer(t *testing.T, conn net.Conn) {
	enc, dec := link.NewEncoder(conn), link.NewDecoder(conn)
	var writeMu sync.Mutex
	write := func(msg link.Message) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return enc.Encode(msg)
	}
	replied := make(chan struct{}, 1)
	go func() {
		for seq := uint64(1); ; seq++ {
			time.Sleep(time.Duration(rand.Intn(300)) * time.Microsecond)
			if write(link.Message{Type: link.MessageClock, Sequence: seq, Bit: rand.Intn(2) == 1}) != nil {
				return
			}
			if _, ok := <-replied; !ok {
				return
			}
		}
	}()
	go func() {
		defer close(replied)
		for {
			msg, err := dec.Decode()
			if err != nil {
				return
			}
			switch msg.Type {
			case link.MessageClock:
				if write(link.Message{Type: link.MessageClockReply, Sequence: msg.Sequence, Bit: rand.Intn(2) == 1}) != nil {
					return
				}
			case link.MessageClockReply:
				replied <- struct{}{}
			}
		}
	}()
}

func TestSessionRecordingReplaysNetworkLinkWithoutPeer(t *testing.T) {
	source, err := New(WithROMBytes(serialLoopROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	source.StepFrame()

	recorder, err := source.StartSessionRecording(RecordingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	source.StepFrame() // attach mid-session, before the ROM starts transferring
	local, remote := net.Pipe()
	randomLinkPeer(t, remote)
	networkLink := NewDirectLink(local, 2*time.Second)
	if err := source.AttachLink(networkLink); err != nil {
		t.Fatal(err)
	}
	source.StepFrames(40)
	// A closed link stays attached, as a host does after its session ends.
	_ = networkLink.Close()
	_ = remote.Close()
	source.StepFrames(5)

	recording, err := recorder.Stop()
	if err != nil {
		t.Fatal(err)
	}
	var exchanges, clocks int
	for _, event := range recording.Links {
		switch event.Kind {
		case LinkExchange:
			exchanges++
		case LinkClock:
			clocks++
		}
	}
	if exchanges == 0 || clocks == 0 {
		t.Fatalf("recorded %d exchanges and %d clocks, want both exercised", exchanges, clocks)
	}

	archive, err := recording.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseRecording(archive)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := New(WithROMBytes(serialLoopROM()), Headless(), WithoutVideo())
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if err := replay.ReplayRecording(parsed); err != nil {
		t.Fatal(err)
	}

	// Dropping the link log must fail loudly rather than replay a different game.
	parsed.Links = nil
	if err := replay.ReplayRecording(parsed); err == nil {
		t.Fatal("replay without link events matched the linked session")
	}
}
