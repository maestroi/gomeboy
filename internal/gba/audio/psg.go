package audio

import "github.com/maestroi/gomeboy/internal/gba/bus"

const (
	sound1ControlLowOffset  uint32 = 0x060
	sound1ControlHighOffset uint32 = 0x062
	sound1ControlXOffset    uint32 = 0x064
	sound2ControlLowOffset  uint32 = 0x068
	sound2ControlHighOffset uint32 = 0x06c
	sound3ControlLowOffset  uint32 = 0x070
	sound3ControlHighOffset uint32 = 0x072
	sound3ControlXOffset    uint32 = 0x074
	sound4ControlLowOffset  uint32 = 0x078
	sound4ControlHighOffset uint32 = 0x07c
	soundControlLowOffset   uint32 = 0x080
	soundControlXOffset     uint32 = 0x084
	waveRAMOffset           uint32 = 0x090

	psgFramePeriod uint64 = masterClockHz / 512
)

var squareDuty = [4][8]uint8{
	{0, 0, 0, 0, 0, 0, 0, 1},
	{1, 0, 0, 0, 0, 0, 0, 1},
	{1, 0, 0, 0, 0, 1, 1, 1},
	{0, 1, 1, 1, 1, 1, 1, 0},
}

type psgEnvelope struct {
	initial  uint8
	volume   uint8
	period   uint8
	timer    uint8
	increase bool
	dac      bool
}

func (e *psgEnvelope) write(value byte) {
	e.initial = value >> 4
	e.increase = value&0x08 != 0
	e.period = value & 7
	e.dac = value&0xf8 != 0
}

func (e *psgEnvelope) trigger() {
	e.volume = e.initial
	e.timer = e.period
	if e.timer == 0 {
		e.timer = 8
	}
}

func (e *psgEnvelope) clock() {
	if e.period == 0 {
		return
	}
	if e.timer > 0 {
		e.timer--
	}
	if e.timer != 0 {
		return
	}
	e.timer = e.period
	if e.increase {
		if e.volume < 15 {
			e.volume++
		}
	} else if e.volume > 0 {
		e.volume--
	}
}

type squarePSG struct {
	enabled      bool
	duty         uint8
	position     uint8
	frequency    uint16
	phaseCycles  uint64
	length       uint16
	lengthEnable bool
	envelope     psgEnvelope

	sweepPeriod uint8
	sweepShift  uint8
	sweepTimer  uint8
	sweepNegate bool
	sweepShadow uint16
	sweepEnable bool
}

func (s *squarePSG) periodCycles() uint64 {
	return uint64(2048-(s.frequency&0x7ff)) * 16
}

func (s *squarePSG) advance(cycles uint64) {
	if !s.enabled {
		return
	}
	period := s.periodCycles()
	if period == 0 {
		return
	}
	s.phaseCycles += cycles
	for s.phaseCycles >= period {
		s.phaseCycles -= period
		s.position = (s.position + 1) & 7
	}
}

func (s *squarePSG) sample() uint8 {
	if !s.enabled || !s.envelope.dac || squareDuty[s.duty][s.position] == 0 {
		return 0
	}
	return s.envelope.volume
}

func (s *squarePSG) trigger(withSweep bool) {
	s.enabled = s.envelope.dac
	if s.length == 0 {
		s.length = 64
	}
	s.phaseCycles = 0
	s.position = 0
	s.envelope.trigger()
	if withSweep {
		s.sweepShadow = s.frequency
		s.sweepTimer = s.sweepPeriod
		if s.sweepTimer == 0 {
			s.sweepTimer = 8
		}
		s.sweepEnable = s.sweepPeriod != 0 || s.sweepShift != 0
		if s.sweepShift != 0 {
			s.sweep(false)
		}
	}
}

func (s *squarePSG) sweep(apply bool) bool {
	if s.sweepShift == 0 {
		return true
	}
	delta := s.sweepShadow >> s.sweepShift
	next := int(s.sweepShadow)
	if s.sweepNegate {
		next -= int(delta)
	} else {
		next += int(delta)
	}
	if next < 0 || next > 0x7ff {
		s.enabled = false
		return false
	}
	if apply {
		s.sweepShadow = uint16(next)
		s.frequency = uint16(next)
	}
	return true
}

func (s *squarePSG) clockSweep() {
	if !s.sweepEnable {
		return
	}
	if s.sweepTimer > 0 {
		s.sweepTimer--
	}
	if s.sweepTimer != 0 {
		return
	}
	s.sweepTimer = s.sweepPeriod
	if s.sweepTimer == 0 {
		s.sweepTimer = 8
	}
	if s.sweepPeriod != 0 && s.sweep(false) {
		s.sweep(true)
		s.sweep(false)
	}
}

type wavePSG struct {
	enabled      bool
	dac          bool
	twoBanks     bool
	bank         uint8
	volumeCode   uint8
	frequency    uint16
	phaseCycles  uint64
	position     uint8
	length       uint16
	lengthEnable bool
	ram          [32]byte
}

func (w *wavePSG) periodCycles() uint64 {
	return uint64(2048-(w.frequency&0x7ff)) * 8
}

func (w *wavePSG) advance(cycles uint64) {
	if !w.enabled || !w.dac {
		return
	}
	period := w.periodCycles()
	if period == 0 {
		return
	}
	w.phaseCycles += cycles
	for w.phaseCycles >= period {
		w.phaseCycles -= period
		if w.twoBanks {
			w.position = (w.position + 1) & 63
		} else {
			w.position = (w.position + 1) & 31
		}
	}
}

func (w *wavePSG) playBank() uint8 {
	if w.twoBanks {
		if w.position >= 32 {
			return w.bank ^ 1
		}
		return w.bank
	}
	return w.bank
}

func (w *wavePSG) sample() uint8 {
	if !w.enabled || !w.dac {
		return 0
	}
	pos := w.position & 31
	bank := w.playBank()
	value := w.ram[int(bank)*16+int(pos>>1)]
	if pos&1 == 0 {
		value >>= 4
	} else {
		value &= 0x0f
	}
	if w.volumeCode&4 != 0 {
		return (value * 3) >> 2
	}
	switch w.volumeCode & 3 {
	case 0:
		return 0
	case 1:
		return value
	case 2:
		return value >> 1
	default:
		return value >> 2
	}
}

func (w *wavePSG) trigger() {
	w.enabled = w.dac
	if w.length == 0 {
		w.length = 256
	}
	w.position = 0
	w.phaseCycles = 0
}

func (w *wavePSG) accessBank(masterEnabled bool) uint8 {
	if !masterEnabled {
		return 1
	}
	return w.bank ^ 1
}

type noisePSG struct {
	enabled      bool
	length       uint16
	lengthEnable bool
	envelope     psgEnvelope
	clockShift   uint8
	divisorCode  uint8
	width7       bool
	phaseCycles  uint64
	lfsr         uint16
}

func (n *noisePSG) periodCycles() uint64 {
	divisor := uint64(8)
	if n.divisorCode != 0 {
		divisor = uint64(n.divisorCode) * 16
	}
	return (divisor << n.clockShift) * 4
}

func (n *noisePSG) advance(cycles uint64) {
	if !n.enabled {
		return
	}
	period := n.periodCycles()
	if period == 0 {
		return
	}
	n.phaseCycles += cycles
	for n.phaseCycles >= period {
		n.phaseCycles -= period
		xor := (n.lfsr & 1) ^ ((n.lfsr >> 1) & 1)
		n.lfsr = (n.lfsr >> 1) | (xor << 14)
		if n.width7 {
			n.lfsr = (n.lfsr &^ (1 << 6)) | (xor << 6)
		}
	}
}

func (n *noisePSG) sample() uint8 {
	if !n.enabled || !n.envelope.dac || n.lfsr&1 != 0 {
		return 0
	}
	return n.envelope.volume
}

func (n *noisePSG) trigger() {
	n.enabled = n.envelope.dac
	if n.length == 0 {
		n.length = 64
	}
	n.envelope.trigger()
	n.lfsr = 0x7fff
	n.phaseCycles = 0
}

type psgState struct {
	enabled bool
	frame   uint8
	frameCycles uint64

	controlLow uint16
	raw        [10]uint16
	reg        [10]uint16
	square     [2]squarePSG
	wave       wavePSG
	noise      noisePSG
}

func (a *Audio) installPSG(io *bus.IO) {
	offsets := [...]uint32{
		sound1ControlLowOffset, sound1ControlHighOffset, sound1ControlXOffset,
		sound2ControlLowOffset, sound2ControlHighOffset,
		sound3ControlLowOffset, sound3ControlHighOffset, sound3ControlXOffset,
		sound4ControlLowOffset, sound4ControlHighOffset,
	}
	for index, offset := range offsets {
		index, offset := index, offset
		io.Register16WithByteWrite(offset,
			func() uint16 { return a.psg.reg[index] },
			func(value uint16) { a.writePSGRegister(index, value) },
			func(byteOffset uint32, value byte) {
				current := a.psg.raw[index]
				if byteOffset == 0 {
					current = current&0xff00 | uint16(value)
				} else {
					current = current&0x00ff | uint16(value)<<8
				}
				a.writePSGRegister(index, current)
			},
		)
	}
	io.Register16(soundControlLowOffset,
		func() uint16 { return a.psg.controlLow },
		func(value uint16) { a.psg.controlLow = value & 0xff77 },
	)
	io.Register16(soundControlXOffset,
		func() uint16 { return a.soundControlX() },
		a.writeSoundControlX,
	)
	for half := 0; half < 8; half++ {
		half := half
		offset := waveRAMOffset + uint32(half*2)
		io.Register16WithByteWrite(offset,
			func() uint16 {
				bank := a.psg.wave.accessBank(a.psg.enabled)
				base := int(bank)*16 + half*2
				return uint16(a.psg.wave.ram[base]) | uint16(a.psg.wave.ram[base+1])<<8
			},
			func(value uint16) {
				bank := a.psg.wave.accessBank(a.psg.enabled)
				base := int(bank)*16 + half*2
				a.psg.wave.ram[base] = byte(value)
				a.psg.wave.ram[base+1] = byte(value >> 8)
			},
			func(byteOffset uint32, value byte) {
				bank := a.psg.wave.accessBank(a.psg.enabled)
				base := int(bank)*16 + half*2 + int(byteOffset)
				a.psg.wave.ram[base] = value
			},
		)
	}
}

func (a *Audio) writePSGRegister(index int, value uint16) {
	a.psg.raw[index] = value
	if index == 2 || index == 4 || index == 7 || index == 9 {
		a.psg.raw[index] &^= 0x8000
	}
	switch index {
	case 0:
		a.psg.reg[index] = value & 0x007f
		s := &a.psg.square[0]
		s.sweepShift = uint8(value & 7)
		s.sweepNegate = value&0x08 != 0
		s.sweepPeriod = uint8((value >> 4) & 7)
	case 1:
		a.psg.reg[index] = value & 0xffc0
		s := &a.psg.square[0]
		s.duty = uint8((value >> 6) & 3)
		s.length = 64 - uint16(value&0x3f)
		s.envelope.write(byte(value >> 8))
		if !s.envelope.dac {
			s.enabled = false
		}
	case 2:
		a.psg.reg[index] = value & 0x4000
		s := &a.psg.square[0]
		s.frequency = (s.frequency & 0x700) | (value & 0xff)
		s.frequency = (s.frequency & 0xff) | (((value >> 8) & 7) << 8)
		s.lengthEnable = value&0x4000 != 0
		if value&0x8000 != 0 {
			s.trigger(true)
		}
	case 3:
		a.psg.reg[index] = value & 0xffc0
		s := &a.psg.square[1]
		s.duty = uint8((value >> 6) & 3)
		s.length = 64 - uint16(value&0x3f)
		s.envelope.write(byte(value >> 8))
		if !s.envelope.dac {
			s.enabled = false
		}
	case 4:
		a.psg.reg[index] = value & 0x4000
		s := &a.psg.square[1]
		s.frequency = (s.frequency & 0x700) | (value & 0xff)
		s.frequency = (s.frequency & 0xff) | (((value >> 8) & 7) << 8)
		s.lengthEnable = value&0x4000 != 0
		if value&0x8000 != 0 {
			s.trigger(false)
		}
	case 5:
		a.psg.reg[index] = value & 0x00e0
		w := &a.psg.wave
		w.twoBanks = value&0x20 != 0
		w.bank = uint8((value >> 6) & 1)
		w.dac = value&0x80 != 0
		if !w.dac {
			w.enabled = false
		}
	case 6:
		a.psg.reg[index] = value & 0xe000
		w := &a.psg.wave
		w.length = 256 - uint16(value&0xff)
		w.volumeCode = uint8((value >> 13) & 7)
	case 7:
		a.psg.reg[index] = value & 0x4000
		w := &a.psg.wave
		w.frequency = (w.frequency & 0x700) | (value & 0xff)
		w.frequency = (w.frequency & 0xff) | ((value >> 8) & 7 << 8)
		w.lengthEnable = value&0x4000 != 0
		if value&0x8000 != 0 {
			w.trigger()
		}
	case 8:
		a.psg.reg[index] = value & 0xff00
		n := &a.psg.noise
		n.length = 64 - uint16(value&0x3f)
		n.envelope.write(byte(value >> 8))
		if !n.envelope.dac {
			n.enabled = false
		}
	case 9:
		a.psg.reg[index] = value & 0x40ff
		n := &a.psg.noise
		feedback := byte(value)
		n.divisorCode = feedback & 7
		n.width7 = feedback&0x08 != 0
		n.clockShift = feedback >> 4
		n.lengthEnable = value&0x4000 != 0
		if value&0x8000 != 0 {
			n.trigger()
		}
	}
}

func (a *Audio) soundControlX() uint16 {
	var status uint16
	if a.psg.square[0].enabled { status |= 1 << 0 }
	if a.psg.square[1].enabled { status |= 1 << 1 }
	if a.psg.wave.enabled { status |= 1 << 2 }
	if a.psg.noise.enabled { status |= 1 << 3 }
	if a.psg.enabled { status |= 1 << 7 }
	return status
}

func (a *Audio) writeSoundControlX(value uint16) {
	enable := value&0x80 != 0
	if !enable {
		waveRAM := a.psg.wave.ram
		a.psg = psgState{}
		a.psg.wave.ram = waveRAM
		a.controlHigh &^= controlPSGVolumeMask
		return
	}
	if !a.psg.enabled {
		a.psg.enabled = true
		a.psg.frame = 0
		a.psg.frameCycles = 0
	}
}

func (a *Audio) advancePSG(cycles uint64) {
	if !a.psg.enabled || cycles == 0 {
		return
	}
	a.psg.square[0].advance(cycles)
	a.psg.square[1].advance(cycles)
	a.psg.wave.advance(cycles)
	a.psg.noise.advance(cycles)
	a.psg.frameCycles += cycles
	for a.psg.frameCycles >= psgFramePeriod {
		a.psg.frameCycles -= psgFramePeriod
		a.clockPSGFrame()
	}
}

func (a *Audio) clockPSGFrame() {
	a.psg.frame = (a.psg.frame + 1) & 7
	switch a.psg.frame {
	case 0, 2, 4, 6:
		a.clockPSGLengths()
		if a.psg.frame == 2 || a.psg.frame == 6 {
			a.psg.square[0].clockSweep()
		}
	case 7:
		a.psg.square[0].envelope.clock()
		a.psg.square[1].envelope.clock()
		a.psg.noise.envelope.clock()
	}
}

func (a *Audio) clockPSGLengths() {
	for i := range a.psg.square {
		s := &a.psg.square[i]
		if s.lengthEnable && s.length > 0 {
			s.length--
			if s.length == 0 { s.enabled = false }
		}
	}
	if w := &a.psg.wave; w.lengthEnable && w.length > 0 {
		w.length--
		if w.length == 0 { w.enabled = false }
	}
	if n := &a.psg.noise; n.lengthEnable && n.length > 0 {
		n.length--
		if n.length == 0 { n.enabled = false }
	}
}

func (a *Audio) mixPSG() (left, right int32) {
	if !a.psg.enabled {
		return 0, 0
	}
	samples := [4]uint8{
		a.psg.square[0].sample(),
		a.psg.square[1].sample(),
		a.psg.wave.sample(),
		a.psg.noise.sample(),
	}
	routing := byte(a.psg.controlLow >> 8)
	var leftRaw, rightRaw int32
	for i, sample := range samples {
		if routing&(1<<i) != 0 { rightRaw += int32(sample) }
		if routing&(1<<(i+4)) != 0 { leftRaw += int32(sample) }
	}
	leftRaw *= int32(1 + ((a.psg.controlLow >> 4) & 7))
	rightRaw *= int32(1 + (a.psg.controlLow & 7))
	leftRaw <<= 3
	rightRaw <<= 3
	ratio := a.controlHigh & controlPSGVolumeMask
	shift := uint(4 - ratio)
	leftRaw >>= shift
	rightRaw >>= shift
	return leftRaw, rightRaw
}
