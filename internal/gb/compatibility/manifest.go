package compatibility

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// Manifest is the checked-in description of one compatibility corpus.
type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	Suite         string         `json:"suite"`
	SuiteRevision string         `json:"suite_revision"`
	Tests         []ManifestTest `json:"tests"`
}

// ManifestTest describes one staged compatibility case.
type ManifestTest struct {
	Name           string               `json:"name"`
	ROM            string               `json:"rom"`
	ROMName        string               `json:"rom_name"`
	ROMSHA256      string               `json:"rom_sha256"`
	Source         string               `json:"source,omitempty"`
	SourceRevision string               `json:"source_revision,omitempty"`
	Model          gomeboy.Model        `json:"model"`
	MaxFrames      uint64               `json:"max_frames"`
	TargetStage    Stage                `json:"target_stage"`
	Checkpoint     *ManifestCheckpoint  `json:"checkpoint,omitempty"`
	Inputs         []ManifestInput      `json:"inputs,omitempty"`
	Persistence    *ManifestPersistence `json:"persistence,omitempty"`
}

// ManifestCheckpoint is a small memory signature.
type ManifestCheckpoint struct {
	Address string `json:"address"`
	Value   string `json:"value"`
	Hex     bool   `json:"hex,omitempty"`
}

// ManifestInput schedules one deterministic button transition.
type ManifestInput struct {
	Frame   uint64 `json:"frame"`
	Button  string `json:"button"`
	Pressed bool   `json:"pressed"`
}

// ManifestPersistence defines the before/after signatures around a save flush.
type ManifestPersistence struct {
	Before ManifestCheckpoint `json:"before"`
	After  ManifestCheckpoint `json:"after"`
}

// LoadManifest reads a compatibility manifest and its ROMs.
func LoadManifest(path string) (Manifest, []Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode compatibility manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return Manifest{}, nil, fmt.Errorf("unsupported compatibility manifest schema %d", manifest.SchemaVersion)
	}
	if manifest.Suite == "" || manifest.SuiteRevision == "" {
		return Manifest{}, nil, fmt.Errorf("compatibility manifest suite and suite_revision are required")
	}
	if len(manifest.Tests) == 0 {
		return Manifest{}, nil, fmt.Errorf("compatibility manifest must contain at least one test")
	}

	base := filepath.Dir(path)
	cases := make([]Case, 0, len(manifest.Tests))
	for _, test := range manifest.Tests {
		rom, err := os.ReadFile(resolvePath(base, test.ROM))
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read ROM for %s: %w", test.Name, err)
		}
		checkpoint, err := convertCheckpoint(test.Checkpoint)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("%s checkpoint: %w", test.Name, err)
		}
		persistence, err := convertPersistence(test.Persistence)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("%s persistence: %w", test.Name, err)
		}
		inputs, err := convertInputs(test.Inputs)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("%s inputs: %w", test.Name, err)
		}
		cases = append(cases, Case{
			Name:           test.Name,
			ROM:            rom,
			ROMName:        test.ROMName,
			ROMSHA256:      strings.ToLower(test.ROMSHA256),
			Source:         test.Source,
			SourceRevision: test.SourceRevision,
			Model:          test.Model,
			MaxFrames:      test.MaxFrames,
			TargetStage:    test.TargetStage,
			Checkpoint:     checkpoint,
			Inputs:         inputs,
			Persistence:    persistence,
		})
	}
	return manifest, cases, nil
}

func resolvePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, filepath.FromSlash(path))
}

func convertCheckpoint(in *ManifestCheckpoint) (*Checkpoint, error) {
	if in == nil {
		return nil, nil
	}
	out, err := checkpointValue(*in)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func convertPersistence(in *ManifestPersistence) (*Persistence, error) {
	if in == nil {
		return nil, nil
	}
	before, err := checkpointValue(in.Before)
	if err != nil {
		return nil, fmt.Errorf("before: %w", err)
	}
	after, err := checkpointValue(in.After)
	if err != nil {
		return nil, fmt.Errorf("after: %w", err)
	}
	return &Persistence{Before: before, After: after}, nil
}

func checkpointValue(in ManifestCheckpoint) (Checkpoint, error) {
	raw := strings.TrimSpace(in.Address)
	value, err := strconv.ParseUint(raw, 0, 16)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("invalid address %q: %w", in.Address, err)
	}
	var bytes []byte
	if in.Hex {
		bytes, err = hex.DecodeString(in.Value)
		if err != nil {
			return Checkpoint{}, fmt.Errorf("invalid hex value %q: %w", in.Value, err)
		}
	} else {
		bytes = []byte(in.Value)
	}
	if len(bytes) == 0 {
		return Checkpoint{}, fmt.Errorf("checkpoint value is empty")
	}
	if int(value)+len(bytes) > 0x10000 {
		return Checkpoint{}, fmt.Errorf("checkpoint at %#x with %d bytes wraps address space", value, len(bytes))
	}
	return Checkpoint{Address: uint16(value), Value: bytes}, nil
}

func convertInputs(in []ManifestInput) ([]InputEvent, error) {
	out := make([]InputEvent, 0, len(in))
	for _, event := range in {
		button, ok := parseButton(event.Button)
		if !ok {
			return nil, fmt.Errorf("unknown button %q", event.Button)
		}
		out = append(out, InputEvent{Frame: event.Frame, Button: button, Pressed: event.Pressed})
	}
	return out, nil
}

func parseButton(value string) (gomeboy.Button, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "a":
		return gomeboy.ButtonA, true
	case "b":
		return gomeboy.ButtonB, true
	case "start":
		return gomeboy.ButtonStart, true
	case "select":
		return gomeboy.ButtonSelect, true
	case "up":
		return gomeboy.ButtonUp, true
	case "down":
		return gomeboy.ButtonDown, true
	case "left":
		return gomeboy.ButtonLeft, true
	case "right":
		return gomeboy.ButtonRight, true
	default:
		return 0, false
	}
}
