// Package memprobe composes GomeBoy's generic introspection primitives into
// deterministic causal experiments for reverse-engineering tools.
//
// It deliberately contains no game-specific knowledge. Callers choose the
// memory regions and input actions to test, then interpret the resulting byte
// changes themselves (or hand them to an external agent).
package memprobe

import (
	"fmt"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

// Region is a non-wrapping range of the Game Boy address space to observe.
// Length may extend through 0xFFFF, but Start+Length may not exceed the 64 KiB
// address space.
type Region struct {
	Name   string `json:"name"`
	Start  uint16 `json:"start"`
	Length int    `json:"length"`
}

// InputTransition changes one joypad button state at the start of a Phase.
type InputTransition struct {
	Button  gomeboy.Button `json:"button"`
	Pressed bool           `json:"pressed"`
}

// Phase applies all Transitions in order, then advances exactly Frames display
// frames. An empty Transitions slice is a deterministic wait.
type Phase struct {
	Transitions []InputTransition `json:"transitions,omitempty"`
	Frames      int               `json:"frames"`
}

// Action is one named experiment applied from the shared baseline checkpoint.
type Action struct {
	Name   string  `json:"name"`
	Phases []Phase `json:"phases"`
}

// Change is one observed byte whose final value differs from the baseline.
type Change struct {
	Region  string `json:"region"`
	Address uint16 `json:"address"`
	Before  byte   `json:"before"`
	After   byte   `json:"after"`
	Delta   int16  `json:"delta"`
}

// Result describes one action run from the common baseline checkpoint.
type Result struct {
	Action     string   `json:"action"`
	StartFrame uint64   `json:"start_frame"`
	EndFrame   uint64   `json:"end_frame"`
	Frames     uint64   `json:"frames"`
	StartCycle uint64   `json:"start_cycle"`
	EndCycle   uint64   `json:"end_cycle"`
	Cycles     uint64   `json:"cycles"`
	Changes    []Change `json:"changes"`
}

// Tap returns a conventional button-tap action: press the button, run
// holdFrames, release it, then run settleFrames. Both frame counts must be
// non-negative.
func Tap(name string, button gomeboy.Button, holdFrames, settleFrames int) Action {
	return Action{
		Name: name,
		Phases: []Phase{
			{
				Transitions: []InputTransition{{Button: button, Pressed: true}},
				Frames:      holdFrames,
			},
			{
				Transitions: []InputTransition{{Button: button, Pressed: false}},
				Frames:      settleFrames,
			},
		},
	}
}

// Wait returns a no-input control action that advances exactly frames display
// frames. It is useful for separating ordinary time-driven changes from
// changes caused by an input experiment.
func Wait(name string, frames int) Action {
	return Action{Name: name, Phases: []Phase{{Frames: frames}}}
}

// Run executes every action from one identical emulator checkpoint and returns
// byte-level diffs against the memory observed at that checkpoint.
//
// The emulator is restored before each action and once more before Run
// returns, including when an error occurs. Successful Run therefore leaves the
// caller's emulator at exactly the state from which the experiment started.
//
// Changes are returned in region order, then ascending address order. If
// regions overlap, an address can appear once for each containing region; this
// preserves the caller's region labels rather than silently deduplicating
// them.
func Run(e *gomeboy.Emulator, regions []Region, actions []Action) (results []Result, err error) {
	if e == nil {
		return nil, fmt.Errorf("memprobe: emulator is nil")
	}
	if err := validateRegions(regions); err != nil {
		return nil, err
	}
	if err := validateActions(actions); err != nil {
		return nil, err
	}

	var checkpoint gomeboy.Checkpoint
	e.CheckpointInto(&checkpoint)
	defer func() {
		if restoreErr := e.RestoreCheckpoint(&checkpoint); restoreErr != nil && err == nil {
			err = fmt.Errorf("memprobe: restore baseline: %w", restoreErr)
			results = nil
		}
	}()

	startFrame := e.FrameCount()
	startCycle := e.Cycle()
	baseline := captureRegions(e, regions)
	current := make([][]byte, len(regions))
	for i, region := range regions {
		current[i] = make([]byte, region.Length)
	}

	results = make([]Result, 0, len(actions))
	for _, action := range actions {
		if err := e.RestoreCheckpoint(&checkpoint); err != nil {
			return nil, fmt.Errorf("memprobe: restore baseline before %q: %w", action.Name, err)
		}

		for _, phase := range action.Phases {
			for _, transition := range phase.Transitions {
				if transition.Pressed {
					e.Press(transition.Button)
				} else {
					e.Release(transition.Button)
				}
			}
			if phase.Frames > 0 {
				e.StepFrames(phase.Frames)
			}
		}

		changes := make([]Change, 0)
		for i, region := range regions {
			e.PeekInto(region.Start, current[i])
			for offset, after := range current[i] {
				before := baseline[i][offset]
				if after == before {
					continue
				}
				changes = append(changes, Change{
					Region:  region.Name,
					Address: region.Start + uint16(offset),
					Before:  before,
					After:   after,
					Delta:   int16(after) - int16(before),
				})
			}
		}

		endFrame := e.FrameCount()
		endCycle := e.Cycle()
		results = append(results, Result{
			Action:     action.Name,
			StartFrame: startFrame,
			EndFrame:   endFrame,
			Frames:     endFrame - startFrame,
			StartCycle: startCycle,
			EndCycle:   endCycle,
			Cycles:     endCycle - startCycle,
			Changes:    changes,
		})
	}

	return results, nil
}

func captureRegions(e *gomeboy.Emulator, regions []Region) [][]byte {
	out := make([][]byte, len(regions))
	for i, region := range regions {
		out[i] = make([]byte, region.Length)
		e.PeekInto(region.Start, out[i])
	}
	return out
}

func validateRegions(regions []Region) error {
	if len(regions) == 0 {
		return fmt.Errorf("memprobe: at least one memory region is required")
	}
	for i, region := range regions {
		if region.Name == "" {
			return fmt.Errorf("memprobe: region %d has an empty name", i)
		}
		if region.Length <= 0 {
			return fmt.Errorf("memprobe: region %q length must be > 0", region.Name)
		}
		if int(region.Start)+region.Length > gomeboy.AddressSpaceSize {
			return fmt.Errorf("memprobe: region %q wraps past 0xFFFF", region.Name)
		}
	}
	return nil
}

func validateActions(actions []Action) error {
	if len(actions) == 0 {
		return fmt.Errorf("memprobe: at least one action is required")
	}
	for i, action := range actions {
		if action.Name == "" {
			return fmt.Errorf("memprobe: action %d has an empty name", i)
		}
		if len(action.Phases) == 0 {
			return fmt.Errorf("memprobe: action %q has no phases", action.Name)
		}
		for phaseIndex, phase := range action.Phases {
			if phase.Frames < 0 {
				return fmt.Errorf("memprobe: action %q phase %d has negative frame count", action.Name, phaseIndex)
			}
			for transitionIndex, transition := range phase.Transitions {
				if transition.Button > gomeboy.ButtonRight {
					return fmt.Errorf("memprobe: action %q phase %d transition %d has invalid button %d", action.Name, phaseIndex, transitionIndex, transition.Button)
				}
			}
		}
	}
	return nil
}
