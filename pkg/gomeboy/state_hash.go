package gomeboy

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
)

// StateHash returns a deterministic SHA-256 fingerprint of emulator execution
// state. The RGB framebuffer is excluded because it is output-only and may be
// disabled with WithoutVideo; CPU, scheduler, bus/cartridge, PPU execution,
// APU, timer, serial, model, and frame count remain included.
func (e *Emulator) StateHash() ([32]byte, error) {
	state := e.gb.Snapshot()
	state.PPU.PreparedFrame = [144][160][3]uint8{}
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(state); err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(buf.Bytes()), nil
}

// StateHashHex is StateHash rendered as lowercase hexadecimal.
func (e *Emulator) StateHashHex() (string, error) {
	h, err := e.StateHash()
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h[:]), nil
}
