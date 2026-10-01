package keypad

// State captures keypad register and IRQ-edge state.
type State struct {
	Pressed   uint16
	Control   uint16
	Condition bool
}

func (k *Keypad) Snapshot() State {
	return State{Pressed: k.pressed, Control: k.control, Condition: k.condition}
}

func (k *Keypad) Restore(s State) {
	k.pressed = s.Pressed & keyMask
	k.control = s.Control & controlMask
	k.condition = s.Condition
}
