package emulator

// Controller defines the interface contract for an Emulator to
// implement in order for a display.Driver to be able to control
// it.
type Controller interface {
	LoadROM(string) error
	Pause()
	Resume()
	Paused() bool
	Initialised() bool
	QuickSave() error
	QuickLoad() error
	SetSpeed(int)
	Speed() int
}


// Button is a frontend-neutral gamepad input. GB/GBC cores use the first eight
// buttons; GBA additionally consumes L and R.
type Button uint8

const (
	ButtonA Button = iota
	ButtonB
	ButtonStart
	ButtonSelect
	ButtonUp
	ButtonDown
	ButtonLeft
	ButtonRight
	ButtonL
	ButtonR
)

// InputController is an optional Controller capability for frontends that can
// deliver inputs directly instead of through the legacy GB joypad channels.
type InputController interface {
	PressButton(Button)
	ReleaseButton(Button)
}

// FrameSizer is an optional Controller capability for displays that support
// cores with dimensions other than the original 160x144 Game Boy framebuffer.
type FrameSizer interface {
	FrameSize() (width, height int)
}
