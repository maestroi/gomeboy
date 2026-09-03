package gomeboy

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"runtime/debug"
)

const (
	checkedStateMagic         = "GMBSTATE"
	checkedStateFormatVersion = uint16(1)
	gomeboyModulePath         = "github.com/maestroi/gomeboy"
)

type checkedStateEnvelope struct {
	FormatVersion uint16
	CoreVersion   string
	ROMSHA256     [32]byte
	Model         Model
	Frame         uint64
	Cycle         uint64
	PayloadSHA256 [32]byte
	Payload       []byte
}

// StateMetadata describes a self-identifying checked save state without
// exposing its serialized payload.
type StateMetadata struct {
	FormatVersion uint16
	CoreVersion   string
	ROMSHA256     string
	Model         Model
	Frame         uint64
	Cycle         uint64
	PayloadSHA256 string
}

// SaveStateChecked serializes the current state with a versioned envelope,
// ROM identity, hardware model, frame/cycle coordinates, and payload checksum.
// It is intended for durable checkpoints and bug reports. For very frequent
// in-process branching, use Checkpoint instead.
func (e *Emulator) SaveStateChecked() ([]byte, error) {
	if e == nil || e.gb == nil || len(e.gb.ROM) == 0 {
		return nil, fmt.Errorf("gomeboy: SaveStateChecked: no ROM loaded")
	}
	payload, err := e.gb.SaveState()
	if err != nil {
		return nil, err
	}
	env := checkedStateEnvelope{
		FormatVersion: checkedStateFormatVersion,
		CoreVersion:   gomeboyBuildVersion(),
		ROMSHA256:     sha256.Sum256(e.gb.ROM),
		Model:         e.Model(),
		Frame:         e.FrameCount(),
		Cycle:         e.Cycle(),
		PayloadSHA256: sha256.Sum256(payload),
		Payload:       payload,
	}
	var buf bytes.Buffer
	buf.WriteString(checkedStateMagic)
	if err := gob.NewEncoder(&buf).Encode(env); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// InspectCheckedState validates the envelope shape and checksum and returns
// its metadata. It does not require an Emulator or load the state.
func InspectCheckedState(data []byte) (StateMetadata, error) {
	env, err := decodeCheckedState(data)
	if err != nil {
		return StateMetadata{}, err
	}
	return metadataForEnvelope(env), nil
}

// LoadStateChecked verifies the checked state's format, payload checksum, ROM
// SHA-256, and hardware model before restoring it. This prevents a state from
// silently being applied to the wrong game or machine model.
func (e *Emulator) LoadStateChecked(data []byte) error {
	if e == nil || e.gb == nil || len(e.gb.ROM) == 0 {
		return fmt.Errorf("gomeboy: LoadStateChecked: no ROM loaded")
	}
	env, err := decodeCheckedState(data)
	if err != nil {
		return err
	}
	romHash := sha256.Sum256(e.gb.ROM)
	if env.ROMSHA256 != romHash {
		return fmt.Errorf("gomeboy: LoadStateChecked: ROM SHA-256 mismatch: state %s, emulator %s",
			hex.EncodeToString(env.ROMSHA256[:]), hex.EncodeToString(romHash[:]))
	}
	if model := e.Model(); env.Model != model {
		return fmt.Errorf("gomeboy: LoadStateChecked: hardware model mismatch: state %s, emulator %s", env.Model, model)
	}
	return e.gb.LoadState(env.Payload)
}

func decodeCheckedState(data []byte) (checkedStateEnvelope, error) {
	if len(data) < len(checkedStateMagic) || string(data[:len(checkedStateMagic)]) != checkedStateMagic {
		return checkedStateEnvelope{}, fmt.Errorf("gomeboy: checked state: bad or missing magic")
	}
	var env checkedStateEnvelope
	if err := gob.NewDecoder(bytes.NewReader(data[len(checkedStateMagic):])).Decode(&env); err != nil {
		return checkedStateEnvelope{}, fmt.Errorf("gomeboy: checked state: decode: %w", err)
	}
	if env.FormatVersion != checkedStateFormatVersion {
		return checkedStateEnvelope{}, fmt.Errorf("gomeboy: checked state: unsupported format version %d (want %d)", env.FormatVersion, checkedStateFormatVersion)
	}
	payloadHash := sha256.Sum256(env.Payload)
	if payloadHash != env.PayloadSHA256 {
		return checkedStateEnvelope{}, fmt.Errorf("gomeboy: checked state: payload checksum mismatch")
	}
	return env, nil
}

func metadataForEnvelope(env checkedStateEnvelope) StateMetadata {
	return StateMetadata{
		FormatVersion: env.FormatVersion,
		CoreVersion:   env.CoreVersion,
		ROMSHA256:     hex.EncodeToString(env.ROMSHA256[:]),
		Model:         env.Model,
		Frame:         env.Frame,
		Cycle:         env.Cycle,
		PayloadSHA256: hex.EncodeToString(env.PayloadSHA256[:]),
	}
}

func gomeboyBuildVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if bi.Main.Path == gomeboyModulePath {
		if bi.Main.Version != "" {
			return bi.Main.Version
		}
		return "devel"
	}
	for _, dep := range bi.Deps {
		if dep.Path != gomeboyModulePath {
			continue
		}
		if dep.Replace != nil {
			if dep.Replace.Version != "" {
				return dep.Replace.Version
			}
			if dep.Replace.Path != "" {
				return dep.Replace.Path
			}
		}
		if dep.Version != "" {
			return dep.Version
		}
	}
	return "devel"
}
