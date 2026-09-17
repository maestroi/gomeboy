package gameboy

import "github.com/maestroi/gomeboy/internal/serial"

// WithSerialDevice attaches a serial device after the controller is created.
// Because GameBoy re-applies options during Reset, the same device is
// reattached automatically after a reset.
func WithSerialDevice(device serial.Device) Opt {
	return func(gb *GameBoy) {
		if gb.Serial != nil {
			gb.Serial.Attach(device)
		}
	}
}
