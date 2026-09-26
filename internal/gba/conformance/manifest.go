package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Uint32 accepts either a JSON number or a quoted Go-style integer literal
// such as "0x08000000". Quoted hex keeps manifests readable without giving up
// strict uint32 bounds.
type Uint32 uint32

func (u *Uint32) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return fmt.Errorf("empty uint32 value")
	}
	var text string
	if data[0] == '"' {
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
	} else {
		text = string(data)
	}
	value, err := strconv.ParseUint(text, 0, 32)
	if err != nil {
		return fmt.Errorf("invalid uint32 %q: %w", text, err)
	}
	*u = Uint32(value)
	return nil
}

// Manifest is the on-disk description of one pinned conformance suite.
type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	Suite         string         `json:"suite"`
	SuiteRevision string         `json:"suite_revision"`
	Tests         []ManifestTest `json:"tests"`
}

// ManifestTest describes one ROM and its deterministic terminal conditions.
type ManifestTest struct {
	Name      string              `json:"name"`
	ROM       string              `json:"rom"`
	ROMSHA256 string              `json:"rom_sha256"`
	BIOS      string              `json:"bios,omitempty"`
	Boot      ManifestBoot        `json:"boot"`
	Limits    Limits              `json:"limits"`
	PassAll   []ManifestCondition `json:"pass_all"`
	FailAny   []ManifestCondition `json:"fail_any,omitempty"`
}

// ManifestBoot keeps BIOS-dependent and BIOS-independent startup explicit.
type ManifestBoot struct {
	Mode       BootMode `json:"mode"`
	EntryPoint Uint32   `json:"entry_point,omitempty"`
}

// ManifestCondition is the JSON-friendly form of Condition.
type ManifestCondition struct {
	Type     string `json:"type"`
	Address  Uint32 `json:"address,omitempty"`
	Width    uint8  `json:"width,omitempty"`
	Register int    `json:"register,omitempty"`
	Value    Uint32 `json:"value"`
	Mask     Uint32 `json:"mask,omitempty"`
}

// LoadManifest reads a manifest and its referenced ROM/BIOS files. Relative
// paths are resolved from the manifest's directory.
func LoadManifest(path string) (Manifest, []Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != 1 {
		return Manifest{}, nil, fmt.Errorf("unsupported conformance manifest schema %d", manifest.SchemaVersion)
	}
	if manifest.Suite == "" || manifest.SuiteRevision == "" {
		return Manifest{}, nil, fmt.Errorf("manifest suite and suite_revision are required")
	}
	if len(manifest.Tests) == 0 {
		return Manifest{}, nil, fmt.Errorf("manifest must contain at least one test")
	}

	base := filepath.Dir(path)
	cases := make([]Case, 0, len(manifest.Tests))
	for _, test := range manifest.Tests {
		rom, err := os.ReadFile(resolveManifestPath(base, test.ROM))
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read ROM for %s: %w", test.Name, err)
		}
		var bios []byte
		if test.BIOS != "" {
			bios, err = os.ReadFile(resolveManifestPath(base, test.BIOS))
			if err != nil {
				return Manifest{}, nil, fmt.Errorf("read BIOS for %s: %w", test.Name, err)
			}
		}

		cases = append(cases, Case{
			Name:       test.Name,
			ROM:        rom,
			ROMSHA256:  test.ROMSHA256,
			BIOS:       bios,
			Boot:       test.Boot.Mode,
			EntryPoint: uint32(test.Boot.EntryPoint),
			Limits:     test.Limits,
			PassAll:    convertConditions(test.PassAll),
			FailAny:    convertConditions(test.FailAny),
		})
	}
	return manifest, cases, nil
}

func resolveManifestPath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, filepath.FromSlash(path))
}

func convertConditions(in []ManifestCondition) []Condition {
	out := make([]Condition, 0, len(in))
	for _, condition := range in {
		out = append(out, Condition{
			Type:     condition.Type,
			Address:  uint32(condition.Address),
			Width:    condition.Width,
			Register: condition.Register,
			Value:    uint32(condition.Value),
			Mask:     uint32(condition.Mask),
		})
	}
	return out
}
