package gomeboy

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

const (
	// RecordingFormatVersion is the current durable session-recording format.
	RecordingFormatVersion uint16 = 1

	// RecordingFileExtension is the conventional extension for durable GomeBoy recordings.
	RecordingFileExtension = ".gbrun"

	recordingManifestName = "manifest.json"
	recordingStartState   = "start.state"
	recordingMaxEntrySize = 128 << 20 // 128 MiB safety bound per archive entry.
)

// RecordingOptions configures a durable emulator session recording. Metadata
// is copied when recording starts and is otherwise opaque to GomeBoy, making
// it suitable for downstream run IDs, model names, experiment labels, or
// other game-agnostic annotations.
type RecordingOptions struct {
	Metadata map[string]string
}

// SessionRecorder owns one in-progress recording. It captures an exact checked
// state at StartSessionRecording and records subsequent joypad transitions
// until Stop is called.
type SessionRecorder struct {
	emulator   *Emulator
	startState []byte
	startMeta  StateMetadata
	metadata   map[string]string
	stopped    bool
}

// Recording is a durable deterministic emulator session. The starting checked
// state is kept private so callers cannot accidentally mutate it; use
// StartStateChecked when a copy is needed.
type Recording struct {
	FormatVersion  uint16
	CoreVersion    string
	ROMSHA256      string
	Model          Model
	StartFrame     uint64
	StartCycle     uint64
	EndFrame       uint64
	EndCycle       uint64
	FinalStateHash string
	Inputs         []InputEvent
	Metadata       map[string]string

	startState []byte
}

type recordingManifest struct {
	FormatVersion  uint16            `json:"format_version"`
	CoreVersion    string            `json:"core_version"`
	ROMSHA256      string            `json:"rom_sha256"`
	Model          Model             `json:"model"`
	StartFrame     uint64            `json:"start_frame"`
	StartCycle     uint64            `json:"start_cycle"`
	EndFrame       uint64            `json:"end_frame"`
	EndCycle       uint64            `json:"end_cycle"`
	FinalStateHash string            `json:"final_state_hash"`
	Inputs         []recordingInput  `json:"inputs"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type recordingInput struct {
	Frame   uint64 `json:"frame"`
	Cycle   uint64 `json:"cycle"`
	Button  string `json:"button"`
	Pressed bool   `json:"pressed"`
}

var recordingButtonNames = [...]string{
	"a",
	"b",
	"start",
	"select",
	"up",
	"down",
	"left",
	"right",
}

// StartSessionRecording starts a durable deterministic session recording. It
// snapshots the emulator's exact checked state first, then records subsequent
// Press and Release calls. Existing low-level input recording must be stopped
// before starting a session recording.
func (e *Emulator) StartSessionRecording(opts RecordingOptions) (*SessionRecorder, error) {
	if e == nil || e.gb == nil || len(e.gb.ROM) == 0 {
		return nil, fmt.Errorf("gomeboy: StartSessionRecording: no ROM loaded")
	}
	if e.inputRecording {
		return nil, fmt.Errorf("gomeboy: StartSessionRecording: input recording already active")
	}

	state, err := e.SaveStateChecked()
	if err != nil {
		return nil, err
	}
	meta, err := InspectCheckedState(state)
	if err != nil {
		return nil, err
	}

	e.StartInputRecording()
	return &SessionRecorder{
		emulator:   e,
		startState: append([]byte(nil), state...),
		startMeta:  meta,
		metadata:   cloneStringMap(opts.Metadata),
	}, nil
}

// Stop finishes a session recording and returns an in-memory Recording. The
// final state hash covers deterministic emulator execution state, including
// current joypad state, so input transitions made on the final frame are
// preserved even when no additional frame is stepped afterward.
func (r *SessionRecorder) Stop() (*Recording, error) {
	if r == nil || r.emulator == nil {
		return nil, fmt.Errorf("gomeboy: SessionRecorder.Stop: nil recorder")
	}
	if r.stopped {
		return nil, fmt.Errorf("gomeboy: SessionRecorder.Stop: recording already stopped")
	}
	r.stopped = true

	inputs := r.emulator.StopInputRecording()
	romHash := r.emulator.ROMSHA256()
	if got := fmt.Sprintf("%x", romHash); got != r.startMeta.ROMSHA256 {
		return nil, fmt.Errorf("gomeboy: SessionRecorder.Stop: ROM changed during recording: started %s, now %s", r.startMeta.ROMSHA256, got)
	}
	if model := r.emulator.Model(); model != r.startMeta.Model {
		return nil, fmt.Errorf("gomeboy: SessionRecorder.Stop: hardware model changed during recording: started %s, now %s", r.startMeta.Model, model)
	}
	finalHash, err := r.emulator.StateHashHex()
	if err != nil {
		return nil, err
	}

	return &Recording{
		FormatVersion:  RecordingFormatVersion,
		CoreVersion:    r.startMeta.CoreVersion,
		ROMSHA256:      r.startMeta.ROMSHA256,
		Model:          r.startMeta.Model,
		StartFrame:     r.startMeta.Frame,
		StartCycle:     r.startMeta.Cycle,
		EndFrame:       r.emulator.FrameCount(),
		EndCycle:       r.emulator.Cycle(),
		FinalStateHash: finalHash,
		Inputs:         append([]InputEvent(nil), inputs...),
		Metadata:       cloneStringMap(r.metadata),
		startState:     append([]byte(nil), r.startState...),
	}, nil
}

// StartStateChecked returns a caller-owned copy of the checked state captured
// at the beginning of the recording.
func (r *Recording) StartStateChecked() []byte {
	if r == nil {
		return nil
	}
	return append([]byte(nil), r.startState...)
}

// DurationFrames returns the number of emulated frames stepped while the
// recording was active.
func (r *Recording) DurationFrames() uint64 {
	if r == nil || r.EndFrame < r.StartFrame {
		return 0
	}
	return r.EndFrame - r.StartFrame
}

// MarshalBinary encodes a Recording as a .gbrun ZIP archive containing a
// human-readable manifest and the checked starting state.
func (r *Recording) MarshalBinary() ([]byte, error) {
	if err := validateRecording(r); err != nil {
		return nil, err
	}

	manifest, err := manifestFromRecording(r)
	if err != nil {
		return nil, err
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("gomeboy: recording: encode manifest: %w", err)
	}
	manifestBytes = append(manifestBytes, '\n')

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	if err := writeRecordingEntry(zw, recordingManifestName, manifestBytes); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := writeRecordingEntry(zw, recordingStartState, r.startState); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("gomeboy: recording: close archive: %w", err)
	}
	return out.Bytes(), nil
}

// ParseRecording validates and decodes a .gbrun archive.
func ParseRecording(data []byte) (*Recording, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("gomeboy: recording: open archive: %w", err)
	}

	var manifestBytes, stateBytes []byte
	for _, file := range zr.File {
		switch file.Name {
		case recordingManifestName:
			manifestBytes, err = readRecordingEntry(file)
		case recordingStartState:
			stateBytes, err = readRecordingEntry(file)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	if len(manifestBytes) == 0 {
		return nil, fmt.Errorf("gomeboy: recording: missing %s", recordingManifestName)
	}
	if len(stateBytes) == 0 {
		return nil, fmt.Errorf("gomeboy: recording: missing %s", recordingStartState)
	}

	var manifest recordingManifest
	dec := json.NewDecoder(bytes.NewReader(manifestBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("gomeboy: recording: decode manifest: %w", err)
	}

	recording, err := recordingFromManifest(manifest, stateBytes)
	if err != nil {
		return nil, err
	}
	if err := validateRecording(recording); err != nil {
		return nil, err
	}
	return recording, nil
}

// SaveRecording writes a recording archive to path. Disk I/O occurs only when
// this explicit helper is called.
func SaveRecording(path string, recording *Recording) error {
	data, err := recording.MarshalBinary()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("gomeboy: recording: write %s: %w", path, err)
	}
	return nil
}

// LoadRecording reads and validates a recording archive from path.
func LoadRecording(path string) (*Recording, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gomeboy: recording: read %s: %w", path, err)
	}
	return ParseRecording(data)
}

// ReplayRecording restores the recording's exact starting state, replays all
// input transitions, and verifies the final deterministic state hash.
func (e *Emulator) ReplayRecording(recording *Recording) error {
	return e.replayRecording(recording, nil)
}

// RecordingFrameFunc receives the initial restored frame and every subsequent
// stepped frame during ReplayRecordingFrames. Callers that want RGB output
// should replay on an emulator created without WithoutVideo(). The Frame is a
// zero-copy view and must be copied if retained.
type RecordingFrameFunc func(frame uint64, image Frame) error

// ReplayRecordingFrames is ReplayRecording with a per-frame callback suitable
// for spectators, offline encoders, dataset generation, and future video
// exporters without storing every framebuffer in the recording archive.
func (e *Emulator) ReplayRecordingFrames(recording *Recording, onFrame RecordingFrameFunc) error {
	return e.replayRecording(recording, onFrame)
}

func (e *Emulator) replayRecording(recording *Recording, onFrame RecordingFrameFunc) error {
	if e == nil || e.gb == nil || len(e.gb.ROM) == 0 {
		return fmt.Errorf("gomeboy: ReplayRecording: no ROM loaded")
	}
	if e.inputRecording {
		return fmt.Errorf("gomeboy: ReplayRecording: stop input recording before replay")
	}
	if err := validateRecording(recording); err != nil {
		return err
	}
	if err := e.LoadStateChecked(recording.startState); err != nil {
		return fmt.Errorf("gomeboy: ReplayRecording: restore start state: %w", err)
	}
	if e.FrameCount() != recording.StartFrame || e.Cycle() != recording.StartCycle {
		return fmt.Errorf("gomeboy: ReplayRecording: restored coordinates frame=%d cycle=%d, want frame=%d cycle=%d",
			e.FrameCount(), e.Cycle(), recording.StartFrame, recording.StartCycle)
	}
	if onFrame != nil {
		if err := onFrame(e.FrameCount(), e.Frame()); err != nil {
			return fmt.Errorf("gomeboy: ReplayRecording: frame callback: %w", err)
		}
	}

	inputIndex := 0
	for {
		frame := e.FrameCount()
		for inputIndex < len(recording.Inputs) && recording.Inputs[inputIndex].Frame == frame {
			event := recording.Inputs[inputIndex]
			if event.Pressed {
				e.Press(event.Button)
			} else {
				e.Release(event.Button)
			}
			inputIndex++
		}
		if frame == recording.EndFrame {
			break
		}
		e.StepFrame()
		if onFrame != nil {
			if err := onFrame(e.FrameCount(), e.Frame()); err != nil {
				return fmt.Errorf("gomeboy: ReplayRecording: frame callback: %w", err)
			}
		}
	}

	if inputIndex != len(recording.Inputs) {
		return fmt.Errorf("gomeboy: ReplayRecording: %d input events were not consumed", len(recording.Inputs)-inputIndex)
	}
	if e.Cycle() != recording.EndCycle {
		return fmt.Errorf("gomeboy: ReplayRecording: final cycle %d, want %d", e.Cycle(), recording.EndCycle)
	}
	gotHash, err := e.StateHashHex()
	if err != nil {
		return err
	}
	if gotHash != recording.FinalStateHash {
		return fmt.Errorf("gomeboy: ReplayRecording: final state hash mismatch: got %s, want %s", gotHash, recording.FinalStateHash)
	}
	return nil
}

func validateRecording(recording *Recording) error {
	if recording == nil {
		return fmt.Errorf("gomeboy: recording: nil recording")
	}
	if recording.FormatVersion != RecordingFormatVersion {
		return fmt.Errorf("gomeboy: recording: unsupported format version %d (want %d)", recording.FormatVersion, RecordingFormatVersion)
	}
	if recording.EndFrame < recording.StartFrame {
		return fmt.Errorf("gomeboy: recording: end frame %d is before start frame %d", recording.EndFrame, recording.StartFrame)
	}
	if len(recording.startState) == 0 {
		return fmt.Errorf("gomeboy: recording: missing checked start state")
	}
	meta, err := InspectCheckedState(recording.startState)
	if err != nil {
		return fmt.Errorf("gomeboy: recording: invalid checked start state: %w", err)
	}
	if meta.ROMSHA256 != recording.ROMSHA256 || meta.Model != recording.Model || meta.Frame != recording.StartFrame || meta.Cycle != recording.StartCycle {
		return fmt.Errorf("gomeboy: recording: start-state metadata does not match manifest")
	}
	if recording.FinalStateHash == "" {
		return fmt.Errorf("gomeboy: recording: missing final state hash")
	}
	for i, event := range recording.Inputs {
		if event.Button > ButtonRight {
			return fmt.Errorf("gomeboy: recording: input %d has invalid button %d", i, event.Button)
		}
		if event.Frame < recording.StartFrame || event.Frame > recording.EndFrame {
			return fmt.Errorf("gomeboy: recording: input %d at frame %d is outside [%d,%d]", i, event.Frame, recording.StartFrame, recording.EndFrame)
		}
		if i > 0 && event.Frame < recording.Inputs[i-1].Frame {
			return fmt.Errorf("gomeboy: recording: inputs are not sorted at index %d", i)
		}
	}
	return nil
}

func manifestFromRecording(recording *Recording) (recordingManifest, error) {
	inputs := make([]recordingInput, len(recording.Inputs))
	for i, event := range recording.Inputs {
		name, err := recordingButtonName(event.Button)
		if err != nil {
			return recordingManifest{}, err
		}
		inputs[i] = recordingInput{Frame: event.Frame, Cycle: event.Cycle, Button: name, Pressed: event.Pressed}
	}
	return recordingManifest{
		FormatVersion:  recording.FormatVersion,
		CoreVersion:    recording.CoreVersion,
		ROMSHA256:      recording.ROMSHA256,
		Model:          recording.Model,
		StartFrame:     recording.StartFrame,
		StartCycle:     recording.StartCycle,
		EndFrame:       recording.EndFrame,
		EndCycle:       recording.EndCycle,
		FinalStateHash: recording.FinalStateHash,
		Inputs:         inputs,
		Metadata:       cloneStringMap(recording.Metadata),
	}, nil
}

func recordingFromManifest(manifest recordingManifest, state []byte) (*Recording, error) {
	inputs := make([]InputEvent, len(manifest.Inputs))
	for i, event := range manifest.Inputs {
		button, err := parseRecordingButton(event.Button)
		if err != nil {
			return nil, fmt.Errorf("gomeboy: recording: input %d: %w", i, err)
		}
		inputs[i] = InputEvent{Frame: event.Frame, Cycle: event.Cycle, Button: button, Pressed: event.Pressed}
	}
	return &Recording{
		FormatVersion:  manifest.FormatVersion,
		CoreVersion:    manifest.CoreVersion,
		ROMSHA256:      manifest.ROMSHA256,
		Model:          manifest.Model,
		StartFrame:     manifest.StartFrame,
		StartCycle:     manifest.StartCycle,
		EndFrame:       manifest.EndFrame,
		EndCycle:       manifest.EndCycle,
		FinalStateHash: manifest.FinalStateHash,
		Inputs:         inputs,
		Metadata:       cloneStringMap(manifest.Metadata),
		startState:     append([]byte(nil), state...),
	}, nil
}

func recordingButtonName(button Button) (string, error) {
	if button > ButtonRight {
		return "", fmt.Errorf("gomeboy: recording: invalid button %d", button)
	}
	return recordingButtonNames[button], nil
}

func parseRecordingButton(name string) (Button, error) {
	for button, candidate := range recordingButtonNames {
		if name == candidate {
			return Button(button), nil
		}
	}
	return 0, fmt.Errorf("unknown button %q", name)
}

func writeRecordingEntry(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("gomeboy: recording: create %s: %w", name, err)
	}
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("gomeboy: recording: write %s: %w", name, err)
	}
	return nil
}

func readRecordingEntry(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > recordingMaxEntrySize {
		return nil, fmt.Errorf("gomeboy: recording: %s is too large (%d bytes)", file.Name, file.UncompressedSize64)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("gomeboy: recording: open %s: %w", file.Name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, recordingMaxEntrySize+1))
	if err != nil {
		return nil, fmt.Errorf("gomeboy: recording: read %s: %w", file.Name, err)
	}
	if len(data) > recordingMaxEntrySize {
		return nil, fmt.Errorf("gomeboy: recording: %s exceeds size limit", file.Name)
	}
	return data, nil
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
