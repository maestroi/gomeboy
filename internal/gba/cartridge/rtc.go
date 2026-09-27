package cartridge

import (
	"encoding/binary"
	"fmt"
	"time"
)

const (
	// RTCFooterSize is the conventional GBA RTC trailer size used alongside
	// raw cartridge save data: seven BCD clock bytes, control, and an int64
	// little-endian host timestamp.
	RTCFooterSize = 16

	rtcCommandReset         byte = 0x60
	rtcCommandWriteStatus   byte = 0x62
	rtcCommandReadStatus    byte = 0x63
	rtcCommandWriteDateTime byte = 0x64
	rtcCommandReadDateTime  byte = 0x65
	rtcCommandWriteTime     byte = 0x66
	rtcCommandReadTime      byte = 0x67
)

type rtcSerialState uint8

const (
	rtcIdle rtcSerialState = iota
	rtcCommand
	rtcInput
	rtcOutput
)

// RTC models the Seiko S-3511A-compatible clock found on GBA cartridges.
// Communication is bit-banged through the cartridge GPIO port.
type RTC struct {
	now     func() time.Time
	offset  time.Duration
	control byte

	state      rtcSerialState
	command    byte
	commandBit int
	bitIndex   int
	input      []byte
	output     []byte
	writeCmd   byte
}

// NewRTC creates an RTC backed by the host wall clock.
func NewRTC() *RTC {
	return newRTC(time.Now)
}

func newRTC(now func() time.Time) *RTC {
	if now == nil {
		now = time.Now
	}
	return &RTC{
		now:        now,
		control:    0x40, // 24-hour mode, as expected by common GBA RTC software.
		commandBit: -1,
	}
}

// Control returns the S-3511A control/status byte.
func (r *RTC) Control() byte { return r.control }

// CurrentTime returns the wall-clock time currently exposed by the emulated RTC.
func (r *RTC) CurrentTime() time.Time { return r.now().Add(r.offset) }

func (r *RTC) resetSerial() {
	r.state = rtcIdle
	r.command = 0
	r.commandBit = -1
	r.bitIndex = 0
	r.input = nil
	r.output = nil
	r.writeCmd = 0
}

// DataBit returns the RTC-driven SIO value. Output data is shifted least
// significant bit first, matching the S-3511A serial protocol.
func (r *RTC) DataBit() byte {
	if r.state != rtcOutput || r.bitIndex < 0 || r.bitIndex >= len(r.output)*8 {
		return 1
	}
	value := r.output[r.bitIndex>>3]
	return (value >> uint(r.bitIndex&7)) & 1
}

// WritePins observes the three RTC GPIO pins: bit 0 SCK, bit 1 SIO, bit 2 CS.
// Direction bit 1 is 1 while the CPU is driving SIO and 0 while the RTC is.
func (r *RTC) WritePins(previous, current, direction byte) {
	previous &= 0x07
	current &= 0x07

	// Command framing starts with the documented 1 -> 5 transition.
	if r.state == rtcIdle && previous == 0x01 && current == 0x05 {
		r.state = rtcCommand
		r.command = 0
		r.commandBit = 7
		r.bitIndex = 0
		return
	}

	// Dropping chip-select aborts an in-flight serial transfer.
	if current&0x04 == 0 {
		if r.state != rtcIdle {
			r.resetSerial()
		}
		return
	}

	risingClock := previous&0x01 == 0 && current&0x01 != 0
	if !risingClock {
		return
	}

	switch r.state {
	case rtcCommand:
		if r.commandBit < 0 {
			return
		}
		r.command |= ((current >> 1) & 1) << uint(r.commandBit)
		r.commandBit--
		if r.commandBit < 0 {
			r.executeCommand()
		}

	case rtcInput:
		if direction&0x02 == 0 || r.bitIndex >= len(r.input)*8 {
			return
		}
		r.input[r.bitIndex>>3] |= ((current >> 1) & 1) << uint(r.bitIndex&7)
		r.bitIndex++
		if r.bitIndex == len(r.input)*8 {
			r.finishInput()
			r.resetSerial()
		}

	case rtcOutput:
		if direction&0x02 != 0 {
			return
		}
		r.bitIndex++
		if r.bitIndex >= len(r.output)*8 {
			r.resetSerial()
		}
	}
}

func (r *RTC) executeCommand() {
	switch r.command {
	case rtcCommandReset:
		// Force reset resets the serial/control circuitry, not the oscillator.
		r.control = 0x40
		r.resetSerial()
	case rtcCommandWriteStatus:
		r.beginInput(rtcCommandWriteStatus, 1)
	case rtcCommandReadStatus:
		r.beginOutput([]byte{r.control})
	case rtcCommandWriteDateTime:
		r.beginInput(rtcCommandWriteDateTime, 7)
	case rtcCommandReadDateTime:
		r.beginOutput(r.dateTimeBytes())
	case rtcCommandWriteTime:
		r.beginInput(rtcCommandWriteTime, 3)
	case rtcCommandReadTime:
		data := r.dateTimeBytes()
		r.beginOutput(data[4:7])
	default:
		r.resetSerial()
	}
}

func (r *RTC) beginInput(command byte, size int) {
	r.state = rtcInput
	r.writeCmd = command
	r.input = make([]byte, size)
	r.output = nil
	r.bitIndex = 0
}

func (r *RTC) beginOutput(data []byte) {
	r.state = rtcOutput
	r.output = append(r.output[:0], data...)
	r.input = nil
	r.bitIndex = 0
}

func (r *RTC) finishInput() {
	switch r.writeCmd {
	case rtcCommandWriteStatus:
		r.control = r.input[0]
	case rtcCommandWriteDateTime:
		_ = r.setDateTime(r.input)
	case rtcCommandWriteTime:
		r.setTime(r.input)
	}
}

// SaveFooter serializes the RTC using the widely used 16-byte GBA RTC save
// trailer. The timestamp anchors the BCD wall-clock reading so configured RTC
// offsets continue to advance while the emulator is not running.
func (r *RTC) SaveFooter() []byte {
	footer := make([]byte, RTCFooterSize)
	copy(footer[:7], r.dateTimeBytes())
	footer[7] = r.control
	binary.LittleEndian.PutUint64(footer[8:], uint64(r.now().Unix()))
	return footer
}

// LoadFooter restores a 16-byte GBA RTC save trailer.
func (r *RTC) LoadFooter(footer []byte) error {
	if len(footer) != RTCFooterSize {
		return fmt.Errorf("gba cartridge: RTC footer size %d, want %d", len(footer), RTCFooterSize)
	}

	now := r.now()
	oldControl := r.control
	r.control = footer[7]
	if !r.setDateTime(footer[:7]) {
		r.control = oldControl
		return fmt.Errorf("gba cartridge: invalid RTC BCD footer")
	}

	savedUnix := int64(binary.LittleEndian.Uint64(footer[8:]))
	savedAt := time.Unix(savedUnix, 0)
	r.offset += now.Sub(savedAt)
	r.resetSerial()
	return nil
}

func (r *RTC) dateTimeBytes() []byte {
	now := r.CurrentTime()
	return []byte{
		encodeBCD(now.Year() % 100),
		encodeBCD(int(now.Month())),
		encodeBCD(now.Day()),
		encodeBCD((int(now.Weekday()) + 6) % 7), // Monday=0, Sunday=6.
		r.encodeHour(now.Hour()),
		encodeBCD(now.Minute()),
		encodeBCD(now.Second()),
	}
}

func (r *RTC) encodeHour(hour int) byte {
	if r.control&0x40 != 0 {
		return encodeBCD(hour)
	}
	pm := hour >= 12
	hour %= 12
	if hour == 0 {
		hour = 12
	}
	value := encodeBCD(hour)
	if pm {
		value |= 0x80
	}
	return value
}

func (r *RTC) decodeHour(value byte) (int, bool) {
	if r.control&0x40 != 0 {
		hour, ok := decodeBCD(value & 0x3f)
		return hour, ok && hour < 24
	}
	pm := value&0x80 != 0
	hour, ok := decodeBCD(value & 0x7f)
	if !ok || hour < 1 || hour > 12 {
		return 0, false
	}
	if hour == 12 {
		hour = 0
	}
	if pm {
		hour += 12
	}
	return hour, true
}

func (r *RTC) setDateTime(data []byte) bool {
	if len(data) != 7 {
		return false
	}
	year, yOK := decodeBCD(data[0])
	month, mOK := decodeBCD(data[1])
	day, dOK := decodeBCD(data[2])
	hour, hOK := r.decodeHour(data[4])
	minute, minOK := decodeBCD(data[5])
	second, sOK := decodeBCD(data[6])
	if !yOK || !mOK || !dOK || !hOK || !minOK || !sOK ||
		month < 1 || month > 12 || day < 1 || day > 31 ||
		minute > 59 || second > 59 {
		return false
	}

	now := r.now()
	target := time.Date(2000+year, time.Month(month), day, hour, minute, second, 0, now.Location())
	if target.Month() != time.Month(month) || target.Day() != day {
		return false
	}
	r.offset = target.Sub(now)
	return true
}

func (r *RTC) setTime(data []byte) {
	if len(data) != 3 {
		return
	}
	hour, hOK := r.decodeHour(data[0])
	minute, minOK := decodeBCD(data[1])
	second, sOK := decodeBCD(data[2])
	if !hOK || !minOK || !sOK || minute > 59 || second > 59 {
		return
	}
	now := r.CurrentTime()
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, second, 0, now.Location())
	r.offset += target.Sub(now)
}

func encodeBCD(value int) byte {
	return byte((value/10)<<4 | value%10)
}

func decodeBCD(value byte) (int, bool) {
	high, low := value>>4, value&0x0f
	if high > 9 || low > 9 {
		return 0, false
	}
	return int(high)*10 + int(low), true
}
