package gomeboy

import "fmt"

// FrameSink consumes one rendered emulator frame. The Frame is a zero-copy
// view into emulator-owned memory and is only valid until the next emulation
// step. A sink that retains RGB data must copy it before WriteFrame returns.
//
// WriteFrame is called synchronously by PublishFrame, StepFrameTo, and
// StepFramesTo. Slow sinks therefore apply backpressure to the caller, which is
// useful for realtime encoders but should be avoided on high-throughput search
// workers unless explicitly desired.
type FrameSink interface {
	WriteFrame(frame uint64, cycle uint64, image Frame) error
}

// FrameSinkFunc adapts a function to FrameSink.
type FrameSinkFunc func(frame uint64, cycle uint64, image Frame) error

// WriteFrame implements FrameSink.
func (f FrameSinkFunc) WriteFrame(frame uint64, cycle uint64, image Frame) error {
	return f(frame, cycle, image)
}

// PublishFrame sends the emulator's current rendered framebuffer to sink
// without advancing emulation. Video output must be enabled; emulators created
// with WithoutVideo do not regenerate RGB frames.
func (e *Emulator) PublishFrame(sink FrameSink) error {
	if e == nil || e.gb == nil {
		return fmt.Errorf("gomeboy: PublishFrame: nil emulator")
	}
	if sink == nil {
		return fmt.Errorf("gomeboy: PublishFrame: nil sink")
	}
	if !e.PPUState().Video {
		return fmt.Errorf("gomeboy: PublishFrame: RGB video output is disabled")
	}
	return sink.WriteFrame(e.FrameCount(), e.Cycle(), e.Frame())
}

// StepFrameTo advances exactly one frame and immediately publishes the newly
// rendered frame to sink. The sink observes the same emulator instance the
// caller is controlling; no second emulator or framebuffer copy is created by
// this API.
func (e *Emulator) StepFrameTo(sink FrameSink) error {
	if sink == nil {
		return fmt.Errorf("gomeboy: StepFrameTo: nil sink")
	}
	e.StepFrame()
	if err := e.PublishFrame(sink); err != nil {
		return fmt.Errorf("gomeboy: StepFrameTo: %w", err)
	}
	return nil
}

// StepFramesTo advances n frames and publishes every rendered frame to sink.
// Unlike StepFrames, this intentionally takes the per-frame path so a live
// viewer or encoder does not miss intermediate frames.
func (e *Emulator) StepFramesTo(n int, sink FrameSink) error {
	if n < 0 {
		return fmt.Errorf("gomeboy: StepFramesTo: frame count must be >= 0")
	}
	if sink == nil {
		return fmt.Errorf("gomeboy: StepFramesTo: nil sink")
	}
	for i := 0; i < n; i++ {
		if err := e.StepFrameTo(sink); err != nil {
			return err
		}
	}
	return nil
}
