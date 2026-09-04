package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	probe "github.com/maestroi/gomeboy/pkg/memprobe"
)

type regionOutput struct {
	Name     string `json:"name"`
	Start    uint16 `json:"start"`
	StartHex string `json:"start_hex"`
	Length   int    `json:"length"`
}

type changeOutput struct {
	Region     string `json:"region"`
	Address    uint16 `json:"address"`
	AddressHex string `json:"address_hex"`
	Before     byte   `json:"before"`
	After      byte   `json:"after"`
	Delta      int16  `json:"delta"`
}

type resultOutput struct {
	Action     string         `json:"action"`
	StartFrame uint64         `json:"start_frame"`
	EndFrame   uint64         `json:"end_frame"`
	Frames     uint64         `json:"frames"`
	StartCycle uint64         `json:"start_cycle"`
	EndCycle   uint64         `json:"end_cycle"`
	Cycles     uint64         `json:"cycles"`
	Changes    []changeOutput `json:"changes"`
}

type output struct {
	ROMPath       string         `json:"rom_path"`
	ROMSHA256     string         `json:"rom_sha256"`
	Title         string         `json:"title"`
	Model         gomeboy.Model  `json:"model"`
	BaselineFrame uint64         `json:"baseline_frame"`
	BaselineCycle uint64         `json:"baseline_cycle"`
	Regions       []regionOutput `json:"regions"`
	Results       []resultOutput `json:"results"`
}

func main() {
	romPath := flag.String("rom", "", "path to the GB/GBC ROM to probe (required)")
	statePath := flag.String("state", "", "optional raw SaveState file to load before probing")
	regionSpec := flag.String("regions", "wram:C000-DFFF,hram:FF80-FFFE", "comma-separated name:START-END regions (hex)")
	actionSpec := flag.String("actions", "control,up,down,left,right,a,b,start,select", "comma-separated built-in actions")
	warmup := flag.Int("warmup", 0, "frames to advance before capturing the baseline")
	hold := flag.Int("hold", 1, "frames to hold a tapped button")
	settle := flag.Int("settle", 7, "frames to advance after releasing a tapped button")
	compact := flag.Bool("compact", false, "emit compact JSON instead of indented JSON")
	flag.Parse()

	if err := run(*romPath, *statePath, *regionSpec, *actionSpec, *warmup, *hold, *settle, *compact); err != nil {
		fmt.Fprintf(os.Stderr, "memprobe: %v\n", err)
		os.Exit(2)
	}
}

func run(romPath, statePath, regionSpec, actionSpec string, warmup, hold, settle int, compact bool) error {
	if romPath == "" {
		return errors.New("-rom is required")
	}
	if warmup < 0 {
		return errors.New("-warmup must be >= 0")
	}
	if hold < 0 {
		return errors.New("-hold must be >= 0")
	}
	if settle < 0 {
		return errors.New("-settle must be >= 0")
	}

	regions, err := parseRegions(regionSpec)
	if err != nil {
		return err
	}
	actions, err := parseActions(actionSpec, hold, settle)
	if err != nil {
		return err
	}

	e, err := gomeboy.New(
		gomeboy.WithROM(romPath),
		gomeboy.Headless(),
		gomeboy.WithoutVideo(),
	)
	if err != nil {
		return err
	}
	defer e.Close()

	if statePath != "" {
		state, err := os.ReadFile(statePath)
		if err != nil {
			return fmt.Errorf("read state: %w", err)
		}
		if err := e.LoadState(state); err != nil {
			return fmt.Errorf("load state: %w", err)
		}
	}
	if warmup > 0 {
		e.StepFrames(warmup)
	}

	baselineFrame := e.FrameCount()
	baselineCycle := e.Cycle()
	results, err := probe.Run(e, regions, actions)
	if err != nil {
		return err
	}

	hash := e.ROMSHA256()
	cart := e.Cartridge()
	out := output{
		ROMPath:       romPath,
		ROMSHA256:     hex.EncodeToString(hash[:]),
		Title:         cart.Title,
		Model:         e.Model(),
		BaselineFrame: baselineFrame,
		BaselineCycle: baselineCycle,
		Regions:       make([]regionOutput, 0, len(regions)),
		Results:       make([]resultOutput, 0, len(results)),
	}
	for _, region := range regions {
		out.Regions = append(out.Regions, regionOutput{
			Name:     region.Name,
			Start:    region.Start,
			StartHex: fmt.Sprintf("0x%04X", region.Start),
			Length:   region.Length,
		})
	}
	for _, result := range results {
		jr := resultOutput{
			Action:     result.Action,
			StartFrame: result.StartFrame,
			EndFrame:   result.EndFrame,
			Frames:     result.Frames,
			StartCycle: result.StartCycle,
			EndCycle:   result.EndCycle,
			Cycles:     result.Cycles,
			Changes:    make([]changeOutput, 0, len(result.Changes)),
		}
		for _, change := range result.Changes {
			jr.Changes = append(jr.Changes, changeOutput{
				Region:     change.Region,
				Address:    change.Address,
				AddressHex: fmt.Sprintf("0x%04X", change.Address),
				Before:     change.Before,
				After:      change.After,
				Delta:      change.Delta,
			})
		}
		out.Results = append(out.Results, jr)
	}

	encoder := json.NewEncoder(os.Stdout)
	if !compact {
		encoder.SetIndent("", "  ")
	}
	if err := encoder.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

func parseRegions(spec string) ([]probe.Region, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("-regions must not be empty")
	}
	parts := strings.Split(spec, ",")
	regions := make([]probe.Region, 0, len(parts))
	for i, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			return nil, fmt.Errorf("region %d is empty", i)
		}

		name := fmt.Sprintf("region%d", i)
		rangeSpec := part
		if colon := strings.IndexByte(part, ':'); colon >= 0 {
			name = strings.TrimSpace(part[:colon])
			rangeSpec = strings.TrimSpace(part[colon+1:])
			if name == "" {
				return nil, fmt.Errorf("region %d has an empty name", i)
			}
		}

		bounds := strings.SplitN(rangeSpec, "-", 2)
		start, err := parseHexAddress(bounds[0])
		if err != nil {
			return nil, fmt.Errorf("region %q start: %w", name, err)
		}
		end := start
		if len(bounds) == 2 {
			end, err = parseHexAddress(bounds[1])
			if err != nil {
				return nil, fmt.Errorf("region %q end: %w", name, err)
			}
		}
		if end < start {
			return nil, fmt.Errorf("region %q end 0x%04X is before start 0x%04X", name, end, start)
		}
		regions = append(regions, probe.Region{
			Name:   name,
			Start:  start,
			Length: int(end-start) + 1,
		})
	}
	return regions, nil
}

func parseHexAddress(raw string) (uint16, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X")
	s = strings.ReplaceAll(s, "_", "")
	if s == "" {
		return 0, errors.New("empty address")
	}
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid hex address %q", raw)
	}
	return uint16(v), nil
}

func parseActions(spec string, hold, settle int) ([]probe.Action, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("-actions must not be empty")
	}
	buttons := map[string]gomeboy.Button{
		"a":      gomeboy.ButtonA,
		"b":      gomeboy.ButtonB,
		"start":  gomeboy.ButtonStart,
		"select": gomeboy.ButtonSelect,
		"up":     gomeboy.ButtonUp,
		"down":   gomeboy.ButtonDown,
		"left":   gomeboy.ButtonLeft,
		"right":  gomeboy.ButtonRight,
	}

	parts := strings.Split(spec, ",")
	actions := make([]probe.Action, 0, len(parts))
	for i, raw := range parts {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			return nil, fmt.Errorf("action %d is empty", i)
		}
		if name == "control" || name == "wait" || name == "none" {
			actions = append(actions, probe.Wait("control", hold+settle))
			continue
		}
		button, ok := buttons[name]
		if !ok {
			return nil, fmt.Errorf("unknown action %q (use control, up, down, left, right, a, b, start, select)", name)
		}
		actions = append(actions, probe.Tap(name, button, hold, settle))
	}
	return actions, nil
}
