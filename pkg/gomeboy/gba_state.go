package gomeboy

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maestroi/gomeboy/internal/gba/system"
)

const (
	gbaStateMagic   = "GBAS"
	gbaStateVersion = uint16(1)
	gbaStateHeader  = 4 + 2 + sha256.Size
)

type gbaCheckpoint struct {
	ROMHash [sha256.Size]byte
	Machine system.State
	Valid   bool
}

func (c *gbaCore) romHash() [sha256.Size]byte { return sha256.Sum256(c.rom) }

func (c *gbaCore) SaveState() ([]byte, error) {
	if c.machine == nil || len(c.rom) == 0 {
		return nil, errors.New("gomeboy: cannot save GBA state: no ROM loaded")
	}

	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(c.machine.Snapshot()); err != nil {
		return nil, fmt.Errorf("gomeboy: encode GBA state: %w", err)
	}

	out := make([]byte, gbaStateHeader, gbaStateHeader+payload.Len())
	copy(out[:4], gbaStateMagic)
	binary.LittleEndian.PutUint16(out[4:6], gbaStateVersion)
	hash := c.romHash()
	copy(out[6:gbaStateHeader], hash[:])
	out = append(out, payload.Bytes()...)
	return out, nil
}

func (c *gbaCore) LoadState(data []byte) error {
	if c.machine == nil || len(c.rom) == 0 {
		return errors.New("gomeboy: cannot load GBA state: no ROM loaded")
	}
	if len(data) < gbaStateHeader {
		return errors.New("gomeboy: invalid GBA state: truncated header")
	}
	if string(data[:4]) != gbaStateMagic {
		return errors.New("gomeboy: invalid GBA state: wrong core or format")
	}
	version := binary.LittleEndian.Uint16(data[4:6])
	if version != gbaStateVersion {
		return fmt.Errorf("gomeboy: unsupported GBA state version %d (want %d)", version, gbaStateVersion)
	}
	wantHash := c.romHash()
	if !bytes.Equal(data[6:gbaStateHeader], wantHash[:]) {
		return errors.New("gomeboy: GBA state ROM does not match loaded ROM")
	}

	var state system.State
	if err := gob.NewDecoder(bytes.NewReader(data[gbaStateHeader:])).Decode(&state); err != nil {
		return fmt.Errorf("gomeboy: decode GBA state: %w", err)
	}
	if err := c.machine.Restore(state); err != nil {
		return fmt.Errorf("gomeboy: restore GBA state: %w", err)
	}
	c.lastError = nil
	return nil
}

func (c *gbaCore) quickStatePath() (string, error) {
	if c.name == "" || len(c.rom) == 0 {
		return "", errors.New("gomeboy: no GBA ROM loaded")
	}
	return filepath.Join(c.saveDir, c.name+".state"), nil
}

func (c *gbaCore) QuickSave() error {
	path, err := c.quickStatePath()
	if err != nil {
		return err
	}
	state, err := c.SaveState()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("gomeboy: create GBA state directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, state, 0o644); err != nil {
		return fmt.Errorf("gomeboy: write GBA state %s: %w", path, err)
	}
	return nil
}

func (c *gbaCore) QuickLoad() error {
	path, err := c.quickStatePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("gomeboy: read GBA state %s: %w", path, err)
	}
	return c.LoadState(data)
}

func (c *gbaCore) NewCheckpoint() any { return &gbaCheckpoint{} }

func (c *gbaCore) CheckpointInto(dst any) {
	state, ok := dst.(*gbaCheckpoint)
	if !ok {
		panic(fmt.Sprintf("gomeboy: invalid checkpoint storage %T for GBA core", dst))
	}
	if c.machine == nil || len(c.rom) == 0 {
		panic("gomeboy: cannot checkpoint GBA core without a ROM")
	}
	state.ROMHash = c.romHash()
	c.machine.SnapshotInto(&state.Machine)
	state.Valid = true
}

func (c *gbaCore) RestoreCheckpoint(src any) error {
	state, ok := src.(*gbaCheckpoint)
	if !ok {
		return fmt.Errorf("gomeboy: invalid checkpoint storage %T for GBA core", src)
	}
	if !state.Valid {
		return errors.New("gomeboy: GBA checkpoint is not initialized")
	}
	if c.machine == nil || len(c.rom) == 0 {
		return errors.New("gomeboy: cannot restore GBA checkpoint: no ROM loaded")
	}
	if state.ROMHash != c.romHash() {
		return errors.New("gomeboy: GBA checkpoint ROM does not match loaded ROM")
	}
	if err := c.machine.Restore(state.Machine); err != nil {
		return fmt.Errorf("gomeboy: restore GBA checkpoint: %w", err)
	}
	c.lastError = nil
	return nil
}
