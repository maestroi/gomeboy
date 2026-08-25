package apu

// ChannelState is a snapshot of a single APU channel's state.
type ChannelState struct {
	EnableTime              uint64
	EnableTimeIncurred      uint64
	LengthCounter           uint16
	Frequency               uint16
	Period                  uint8
	VolumeEnvelopeTimer     uint8
	StartingVolume          uint8
	CurrentVolume           uint8
	Clock, ShouldLock, Lock bool
	EnvelopeDirection       bool
	LengthCounterEnabled    bool
	Enabled, DACEnabled     bool
}

// SquareChannelState is a snapshot of a square channel's duty state.
type SquareChannelState struct {
	Duty             uint8
	LockedDuty       uint8
	WaveDutyPosition uint8
	HasLockedDuty    bool
}

// State is a snapshot of the APU's execution state, including all four
// channels, the frame sequencer, the wave RAM, the mixer, and the
// (bounded) sample buffer.
type State struct {
	Channels [4]ChannelState

	Channel1 struct {
		Square          SquareChannelState
		FrequencyShadow uint16
		SweepPeriod     uint8
		SweepTimer      uint8
		Shift           uint8
		Negate          bool
		DidNegate       bool
		SweepEnabled    bool
	}
	Channel2 SquareChannelState
	Channel3 struct {
		WaveRAMLastRead     uint64
		VolumeCode          uint8
		WaveRAMPosition     uint8
		WaveRAMSampleBuffer uint8
		WaveRAMLastPosition uint8
	}
	Channel4 struct {
		ClockShift     uint8
		DivisorCode    uint8
		WidthMask      uint16
		LFSR           uint16
		DelayedCycles  uint64
		IsTriggered    bool
		CyclesIncurred uint64
		FrequencyTimer uint64
	}

	Enabled                 bool
	FrameSequencerStep      uint8
	WaveRAM                 [16]uint8
	VinLeft, VinRight       bool
	LeftEnable              [4]bool
	RightEnable             [4]bool
	VolumeLeft, VolumeRight uint8
	TurningOn, TurnedOn     bool
	EnableTimer             uint64
	Buffer                  []float32
	BufferPos               uint32
	LastCatchup             uint64
	Mute                    bool
	Headless                bool

	Debug struct {
		Square1, Square2, Wave, Noise bool
	}
}

// Snapshot captures the APU's execution state.
func (a *APU) Snapshot() State {
	a.RLock()
	defer a.RUnlock()

	st := State{
		Enabled:            a.enabled,
		FrameSequencerStep: a.frameSequencerStep,
		WaveRAM:            a.waveRAM,
		VinLeft:            a.vinLeft,
		VinRight:           a.vinRight,
		LeftEnable:         a.leftEnable,
		RightEnable:        a.rightEnable,
		VolumeLeft:         a.volumeLeft,
		VolumeRight:        a.volumeRight,
		TurningOn:          a.turningOn,
		TurnedOn:           a.turnedOn,
		EnableTimer:        a.enableTimer,
		// The audio output buffer is transient (already-generated samples
		// waiting for the audio device), not simulation state. The APU
		// regenerates it from its channel/sequencer state, so it is
		// deliberately excluded to keep save states small and fast.
		Buffer:      nil,
		BufferPos:   0,
		LastCatchup: a.lastCatchup,
		Mute:        a.mute,
		Headless:    a.headless,
	}
	st.Channel1.Square = SquareChannelState{
		Duty:             a.channel1.duty,
		LockedDuty:       a.channel1.lockedDuty,
		WaveDutyPosition: a.channel1.waveDutyPosition,
		HasLockedDuty:    a.channel1.hasLockedDuty,
	}
	st.Channel1.FrequencyShadow = a.channel1.frequencyShadow
	st.Channel1.SweepPeriod = a.channel1.sweepPeriod
	st.Channel1.SweepTimer = a.channel1.sweepTimer
	st.Channel1.Shift = a.channel1.shift
	st.Channel1.Negate = a.channel1.negate
	st.Channel1.DidNegate = a.channel1.didNegate
	st.Channel1.SweepEnabled = a.channel1.sweepEnabled
	st.Channel2 = SquareChannelState{
		Duty:             a.channel2.duty,
		LockedDuty:       a.channel2.lockedDuty,
		WaveDutyPosition: a.channel2.waveDutyPosition,
		HasLockedDuty:    a.channel2.hasLockedDuty,
	}
	st.Channel3.WaveRAMLastRead = a.channel3.waveRAMLastRead
	st.Channel3.VolumeCode = a.channel3.volumeCode
	st.Channel3.WaveRAMPosition = a.channel3.waveRAMPosition
	st.Channel3.WaveRAMSampleBuffer = a.channel3.waveRAMSampleBuffer
	st.Channel3.WaveRAMLastPosition = a.channel3.waveRAMLastPosition
	st.Channel4.ClockShift = a.channel4.clockShift
	st.Channel4.DivisorCode = a.channel4.divisorCode
	st.Channel4.WidthMask = a.channel4.widthMask
	st.Channel4.LFSR = a.channel4.lfsr
	st.Channel4.DelayedCycles = a.channel4.delayedCycles
	st.Channel4.IsTriggered = a.channel4.isTriggered
	st.Channel4.CyclesIncurred = a.channel4.cyclesIncurred
	st.Channel4.FrequencyTimer = a.channel4.frequencyTimer

	for i := 0; i < 4; i++ {
		st.Channels[i] = ChannelState{
			EnableTime:           a.channels[i].enableTime,
			EnableTimeIncurred:   a.channels[i].enableTimeIncurred,
			LengthCounter:        a.channels[i].lengthCounter,
			Frequency:            a.channels[i].frequency,
			Period:               a.channels[i].period,
			VolumeEnvelopeTimer:  a.channels[i].volumeEnvelopeTimer,
			StartingVolume:       a.channels[i].startingVolume,
			CurrentVolume:        a.channels[i].currentVolume,
			Clock:                a.channels[i].clock,
			ShouldLock:           a.channels[i].shouldLock,
			Lock:                 a.channels[i].lock,
			EnvelopeDirection:    a.channels[i].envelopeDirection,
			LengthCounterEnabled: a.channels[i].lengthCounterEnabled,
			Enabled:              a.channels[i].enabled,
			DACEnabled:           a.channels[i].dacEnabled,
		}
	}
	st.Debug.Square1 = a.Debug.Square1
	st.Debug.Square2 = a.Debug.Square2
	st.Debug.Wave = a.Debug.Wave
	st.Debug.Noise = a.Debug.Noise
	return st
}

// Restore rebuilds the APU's execution state from a snapshot.
func (a *APU) Restore(s State) {
	a.Lock()
	defer a.Unlock()

	a.enabled = s.Enabled
	a.frameSequencerStep = s.FrameSequencerStep
	a.waveRAM = s.WaveRAM
	a.vinLeft = s.VinLeft
	a.vinRight = s.VinRight
	a.leftEnable = s.LeftEnable
	a.rightEnable = s.RightEnable
	a.volumeLeft = s.VolumeLeft
	a.volumeRight = s.VolumeRight
	a.turningOn = s.TurningOn
	a.turnedOn = s.TurnedOn
	a.enableTimer = s.EnableTimer
	bufLen := bufferSize
	if l := len(s.Buffer); l > bufLen {
		bufLen = l
	}
	a.buffer = make([]float32, bufLen)
	copy(a.buffer, s.Buffer)
	a.bufferPos = s.BufferPos
	a.lastCatchup = s.LastCatchup
	a.mute = s.Mute
	a.headless = s.Headless

	a.channel1.duty = s.Channel1.Square.Duty
	a.channel1.lockedDuty = s.Channel1.Square.LockedDuty
	a.channel1.waveDutyPosition = s.Channel1.Square.WaveDutyPosition
	a.channel1.hasLockedDuty = s.Channel1.Square.HasLockedDuty
	a.channel1.frequencyShadow = s.Channel1.FrequencyShadow
	a.channel1.sweepPeriod = s.Channel1.SweepPeriod
	a.channel1.sweepTimer = s.Channel1.SweepTimer
	a.channel1.shift = s.Channel1.Shift
	a.channel1.negate = s.Channel1.Negate
	a.channel1.didNegate = s.Channel1.DidNegate
	a.channel1.sweepEnabled = s.Channel1.SweepEnabled
	a.channel2.duty = s.Channel2.Duty
	a.channel2.lockedDuty = s.Channel2.LockedDuty
	a.channel2.waveDutyPosition = s.Channel2.WaveDutyPosition
	a.channel2.hasLockedDuty = s.Channel2.HasLockedDuty
	a.channel3.waveRAMLastRead = s.Channel3.WaveRAMLastRead
	a.channel3.volumeCode = s.Channel3.VolumeCode
	a.channel3.waveRAMPosition = s.Channel3.WaveRAMPosition
	a.channel3.waveRAMSampleBuffer = s.Channel3.WaveRAMSampleBuffer
	a.channel3.waveRAMLastPosition = s.Channel3.WaveRAMLastPosition
	a.channel4.clockShift = s.Channel4.ClockShift
	a.channel4.divisorCode = s.Channel4.DivisorCode
	a.channel4.widthMask = s.Channel4.WidthMask
	a.channel4.lfsr = s.Channel4.LFSR
	a.channel4.delayedCycles = s.Channel4.DelayedCycles
	a.channel4.isTriggered = s.Channel4.IsTriggered
	a.channel4.cyclesIncurred = s.Channel4.CyclesIncurred
	a.channel4.frequencyTimer = s.Channel4.FrequencyTimer

	for i := 0; i < 4; i++ {
		a.channels[i].enableTime = s.Channels[i].EnableTime
		a.channels[i].enableTimeIncurred = s.Channels[i].EnableTimeIncurred
		a.channels[i].lengthCounter = s.Channels[i].LengthCounter
		a.channels[i].frequency = s.Channels[i].Frequency
		a.channels[i].period = s.Channels[i].Period
		a.channels[i].volumeEnvelopeTimer = s.Channels[i].VolumeEnvelopeTimer
		a.channels[i].startingVolume = s.Channels[i].StartingVolume
		a.channels[i].currentVolume = s.Channels[i].CurrentVolume
		a.channels[i].clock = s.Channels[i].Clock
		a.channels[i].shouldLock = s.Channels[i].ShouldLock
		a.channels[i].lock = s.Channels[i].Lock
		a.channels[i].envelopeDirection = s.Channels[i].EnvelopeDirection
		a.channels[i].lengthCounterEnabled = s.Channels[i].LengthCounterEnabled
		a.channels[i].enabled = s.Channels[i].Enabled
		a.channels[i].dacEnabled = s.Channels[i].DACEnabled
	}
	a.Debug.Square1 = s.Debug.Square1
	a.Debug.Square2 = s.Debug.Square2
	a.Debug.Wave = s.Debug.Wave
	a.Debug.Noise = s.Debug.Noise
}
