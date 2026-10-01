package audio

// FIFOState captures one Direct Sound FIFO.
type FIFOState struct {
	Data [fifoCapacity]byte
	Head int
	Size int
}

type DirectSoundState struct {
	FIFO    FIFOState
	Current int8
}

type EnvelopeState struct {
	Initial  uint8
	Volume   uint8
	Period   uint8
	Timer    uint8
	Increase bool
	DAC      bool
}

type SquareState struct {
	Enabled      bool
	Duty         uint8
	Position     uint8
	Frequency    uint16
	PhaseCycles  uint64
	Length       uint16
	LengthEnable bool
	Envelope     EnvelopeState
	SweepPeriod  uint8
	SweepShift   uint8
	SweepTimer   uint8
	SweepNegate  bool
	SweepShadow  uint16
	SweepEnable  bool
}

type WaveState struct {
	Enabled      bool
	DAC          bool
	TwoBanks     bool
	Bank         uint8
	VolumeCode   uint8
	Frequency    uint16
	PhaseCycles  uint64
	Position     uint8
	Length       uint16
	LengthEnable bool
	RAM          [32]byte
}

type NoiseState struct {
	Enabled      bool
	Length       uint16
	LengthEnable bool
	Envelope     EnvelopeState
	ClockShift   uint8
	DivisorCode  uint8
	Width7       bool
	PhaseCycles  uint64
	LFSR         uint16
}

type PSGState struct {
	Enabled     bool
	Frame       uint8
	FrameCycles uint64
	ControlLow  uint16
	Raw         [10]uint16
	Reg         [10]uint16
	Square      [2]SquareState
	Wave        WaveState
	Noise       NoiseState
}

// State captures hardware-visible audio state and timing. Host output policy,
// mute state, and buffered samples intentionally remain local to the receiver.
type State struct {
	ControlHigh uint16
	SoundBias   uint16
	Channel     [2]DirectSoundState
	PSG         PSGState
	SamplePhase uint64
}

func envelopeSnapshot(e psgEnvelope) EnvelopeState {
	return EnvelopeState{Initial: e.initial, Volume: e.volume, Period: e.period, Timer: e.timer, Increase: e.increase, DAC: e.dac}
}

func restoreEnvelope(s EnvelopeState) psgEnvelope {
	return psgEnvelope{initial: s.Initial, volume: s.Volume, period: s.Period, timer: s.Timer, increase: s.Increase, dac: s.DAC}
}

func squareSnapshot(v squarePSG) SquareState {
	return SquareState{
		Enabled: v.enabled, Duty: v.duty, Position: v.position, Frequency: v.frequency,
		PhaseCycles: v.phaseCycles, Length: v.length, LengthEnable: v.lengthEnable,
		Envelope: envelopeSnapshot(v.envelope), SweepPeriod: v.sweepPeriod,
		SweepShift: v.sweepShift, SweepTimer: v.sweepTimer, SweepNegate: v.sweepNegate,
		SweepShadow: v.sweepShadow, SweepEnable: v.sweepEnable,
	}
}

func restoreSquare(s SquareState) squarePSG {
	return squarePSG{
		enabled: s.Enabled, duty: s.Duty, position: s.Position, frequency: s.Frequency,
		phaseCycles: s.PhaseCycles, length: s.Length, lengthEnable: s.LengthEnable,
		envelope: restoreEnvelope(s.Envelope), sweepPeriod: s.SweepPeriod,
		sweepShift: s.SweepShift, sweepTimer: s.SweepTimer, sweepNegate: s.SweepNegate,
		sweepShadow: s.SweepShadow, sweepEnable: s.SweepEnable,
	}
}

func waveSnapshot(v wavePSG) WaveState {
	return WaveState{
		Enabled: v.enabled, DAC: v.dac, TwoBanks: v.twoBanks, Bank: v.bank,
		VolumeCode: v.volumeCode, Frequency: v.frequency, PhaseCycles: v.phaseCycles,
		Position: v.position, Length: v.length, LengthEnable: v.lengthEnable, RAM: v.ram,
	}
}

func restoreWave(s WaveState) wavePSG {
	return wavePSG{
		enabled: s.Enabled, dac: s.DAC, twoBanks: s.TwoBanks, bank: s.Bank,
		volumeCode: s.VolumeCode, frequency: s.Frequency, phaseCycles: s.PhaseCycles,
		position: s.Position, length: s.Length, lengthEnable: s.LengthEnable, ram: s.RAM,
	}
}

func noiseSnapshot(v noisePSG) NoiseState {
	return NoiseState{
		Enabled: v.enabled, Length: v.length, LengthEnable: v.lengthEnable,
		Envelope: envelopeSnapshot(v.envelope), ClockShift: v.clockShift,
		DivisorCode: v.divisorCode, Width7: v.width7, PhaseCycles: v.phaseCycles, LFSR: v.lfsr,
	}
}

func restoreNoise(s NoiseState) noisePSG {
	return noisePSG{
		enabled: s.Enabled, length: s.Length, lengthEnable: s.LengthEnable,
		envelope: restoreEnvelope(s.Envelope), clockShift: s.ClockShift,
		divisorCode: s.DivisorCode, width7: s.Width7, phaseCycles: s.PhaseCycles, lfsr: s.LFSR,
	}
}

func psgSnapshot(v psgState) PSGState {
	return PSGState{
		Enabled: v.enabled, Frame: v.frame, FrameCycles: v.frameCycles,
		ControlLow: v.controlLow, Raw: v.raw, Reg: v.reg,
		Square: [2]SquareState{squareSnapshot(v.square[0]), squareSnapshot(v.square[1])},
		Wave: waveSnapshot(v.wave), Noise: noiseSnapshot(v.noise),
	}
}

func restorePSG(s PSGState) psgState {
	return psgState{
		enabled: s.Enabled, frame: s.Frame, frameCycles: s.FrameCycles,
		controlLow: s.ControlLow, raw: s.Raw, reg: s.Reg,
		square: [2]squarePSG{restoreSquare(s.Square[0]), restoreSquare(s.Square[1])},
		wave: restoreWave(s.Wave), noise: restoreNoise(s.Noise),
	}
}

func (a *Audio) Snapshot() State {
	var s State
	s.ControlHigh = a.controlHigh
	s.SoundBias = a.soundBias
	for i := range a.channel {
		s.Channel[i] = DirectSoundState{
			FIFO: FIFOState{Data: a.channel[i].fifo.data, Head: a.channel[i].fifo.head, Size: a.channel[i].fifo.size},
			Current: a.channel[i].current,
		}
	}
	s.PSG = psgSnapshot(a.psg)
	s.SamplePhase = a.samplePhase
	return s
}

func (a *Audio) Restore(s State) {
	headless := a.headless
	muted := a.mute.Load()
	a.controlHigh = s.ControlHigh & controlStoredMask
	a.soundBias = s.SoundBias & soundBiasMask
	for i := range a.channel {
		a.channel[i].fifo.data = s.Channel[i].FIFO.Data
		a.channel[i].fifo.head = s.Channel[i].FIFO.Head
		a.channel[i].fifo.size = s.Channel[i].FIFO.Size
		a.channel[i].current = s.Channel[i].Current
	}
	a.psg = restorePSG(s.PSG)
	a.samplePhase = s.SamplePhase % masterClockHz
	a.bufferPos = 0
	a.headless = headless
	a.mute.Store(muted)
}
