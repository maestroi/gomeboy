package compatibility

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/maestroi/gomeboy/internal/gba/conformance"
)

// Manifest is the on-disk description of one pinned compatibility corpus.
type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	Suite         string         `json:"suite"`
	SuiteRevision string         `json:"suite_revision"`
	Tests         []ManifestTest `json:"tests"`
}

// ManifestTest describes one ROM and the compatibility milestone it must reach.
type ManifestTest struct {
	Name        string              `json:"name"`
	ROM         string              `json:"rom"`
	ROMSHA256   string              `json:"rom_sha256"`
	BIOS        string              `json:"bios,omitempty"`
	Boot        ManifestBoot        `json:"boot"`
	Limits      conformance.Limits  `json:"limits"`
	TargetStage Stage               `json:"target_stage"`
	Checkpoint  *ManifestCheckpoint `json:"checkpoint,omitempty"`
}

// ManifestBoot keeps BIOS-backed reset and BIOS-less direct boot explicit.
type ManifestBoot struct {
	Mode       conformance.BootMode `json:"mode"`
	EntryPoint conformance.Uint32   `json:"entry_point,omitempty"`
	CPSR       *conformance.Uint32  `json:"cpsr,omitempty"`
	Registers  []ManifestRegister   `json:"registers,omitempty"`
}

// ManifestRegister initializes one direct-boot register.
type ManifestRegister struct {
	Register int                `json:"register"`
	Value    conformance.Uint32 `json:"value"`
}

// ManifestCheckpoint is the JSON representation of a deterministic checkpoint.
type ManifestCheckpoint struct {
	Type     string             `json:"type"`
	Address  conformance.Uint32 `json:"address,omitempty"`
	Width    uint8              `json:"width,omitempty"`
	Register int                `json:"register,omitempty"`
	Value    conformance.Uint32 `json:"value"`
	Mask     conformance.Uint32 `json:"mask,omitempty"`
}

// LoadManifest reads a compatibility manifest and its referenced ROM/BIOS
// files. Relative paths are resolved from the manifest's directory, while
// absolute paths make local-only commercial-ROM manifests possible without
// adding those ROMs to the repository.
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
		var bios []byte
		if test.BIOS != "" {
			bios, err = os.ReadFile(resolvePath(base, test.BIOS))
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("read BIOS for %s: %w", test.Name, err)
			}
		}

		var initialCPSR *uint32
		if test.Boot.CPSR != nil {
			value := uint32(*test.Boot.CPSR)
			initialCPSR = &value
		}

		cases = append(cases, Case{
			Name:             test.Name,
			ROM:              rom,
			ROMSHA256:        test.ROMSHA256,
			BIOS:             bios,
			Boot:             test.Boot.Mode,
			EntryPoint:       uint32(test.Boot.EntryPoint),
			InitialCPSR:      initialCPSR,
			InitialRegisters: convertRegisters(test.Boot.Registers),
			Limits:           test.Limits,
			TargetStage:      test.TargetStage,
			Checkpoint:       convertCheckpoint(test.Checkpoint),
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

func convertRegisters(in []ManifestRegister) []RegisterValue {
	out := make([]RegisterValue, 0, len(in))
	for _, register := range in {
		out = append(out, RegisterValue{
			Register: register.Register,
			Value:    uint32(register.Value),
		})
	}
	return out
}

func convertCheckpoint(in *ManifestCheckpoint) *Checkpoint {
	if in == nil {
		return nil
	}
	return &Checkpoint{
		Type:     in.Type,
		Address:  uint32(in.Address),
		Width:    in.Width,
		Register: in.Register,
		Value:    uint32(in.Value),
		Mask:     uint32(in.Mask),
	}
}
