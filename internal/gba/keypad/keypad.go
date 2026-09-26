// Package keypad implements the Game Boy Advance keypad registers and IRQ logic.
package keypad

import (
	"github.com/maestroi/gomeboy/internal/gba/bus"
	gbairq "github.com/maestroi/gomeboy/internal/gba/interrupt"
)

const (
	keyInputOffset   uint32 = 0x130
	keyControlOffset uint32 = 0x132

	keyMask          uint16 = 0x03ff
	controlIRQEnable uint16 = 1 << 14
	controlAND       uint16 = 1 << 15
	controlMask      uint16 = keyMask | controlIRQEnable | controlAND
)

// Button identifies one of the ten GBA keypad inputs. Values match KEYINPUT
// and KEYCNT bit positions.
type Button uint8

const (
	ButtonA Button = iota
	ButtonB
	ButtonSelect
	ButtonStart
	ButtonRight
	ButtonLeft
	ButtonUp
	ButtonDown
	ButtonR
	ButtonL
)

// IRQSink receives asynchronous keypad interrupt/wake requests.
type IRQSink interface {
	RequestExternal(gbairq.Source)
}

// Keypad owns KEYINPUT/KEYCNT state. pressed uses active-high bits internally;
// KEYINPUT exposes the hardware's active-low representation.
type Keypad struct {
	irq IRQSink

	pressed   uint16
	control   uint16
	condition bool
}

// New maps KEYINPUT and KEYCNT onto b.
func New(b *bus.Bus, irq IRQSink) *Keypad {
	if b == nil {
		panic("gba keypad: nil bus")
	}
	k := &Keypad{irq: irq}
	k.install(b.IO())
	return k
}

func (k *Keypad) install(io *bus.IO) {
	io.Register16(keyInputOffset,
		func() uint16 { return k.Input() },
		nil,
	)
	io.Register16(keyControlOffset,
		func() uint16 { return k.control },
		func(value uint16) {
			k.control = value & controlMask
			k.evaluateIRQ()
		},
	)
}

// Press marks button as held and evaluates the keypad IRQ edge.
func (k *Keypad) Press(button Button) {
	k.Set(button, true)
}

// Release marks button as released and evaluates the keypad IRQ edge.
func (k *Keypad) Release(button Button) {
	k.Set(button, false)
}

// Set updates one button. Invalid button values are ignored.
func (k *Keypad) Set(button Button, pressed bool) {
	if button > ButtonL {
		return
	}
	bit := uint16(1) << button
	before := k.pressed
	if pressed {
		k.pressed |= bit
	} else {
		k.pressed &^= bit
	}
	if k.pressed != before {
		k.evaluateIRQ()
	}
}

// Input returns KEYINPUT. Bits 0-9 are active-low; unused upper bits read 0.
func (k *Keypad) Input() uint16 {
	return (^k.pressed) & keyMask
}

// Control returns the masked KEYCNT value.
func (k *Keypad) Control() uint16 { return k.control }

// PressedMask returns the active-high set of currently held buttons.
func (k *Keypad) PressedMask() uint16 { return k.pressed }

func (k *Keypad) evaluateIRQ() {
	active := false
	if k.control&controlIRQEnable != 0 {
		selected := k.control & keyMask
		if k.control&controlAND != 0 {
			active = k.pressed&selected == selected
		} else {
			active = k.pressed&selected != 0
		}
	}

	// The keypad IRQ input is edge-triggered. Acknowledge of IF while the
	// condition remains true must not continuously re-request the interrupt.
	if active && !k.condition && k.irq != nil {
		k.irq.RequestExternal(gbairq.Keypad)
	}
	k.condition = active
}
