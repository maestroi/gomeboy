// Package stream contains optional video-streaming adapters for GomeBoy.
// The emulator core has no multimedia dependency; this package launches an
// external FFmpeg process and feeds it raw RGB24 frames.
package stream

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
)

const (
	// GameBoyWidth and GameBoyHeight are the native LCD dimensions.
	GameBoyWidth  = 160
	GameBoyHeight = 144

	// GameBoyFPS is the native frame rate derived from the 4,194,304 Hz master
	// clock and 70,224 clocks per frame.
	GameBoyFPS = 4194304.0 / 70224.0
)

// FFmpegOptions configures an FFmpeg-backed RGB frame sink.
type FFmpegOptions struct {
	// Binary is the FFmpeg executable. It defaults to "ffmpeg" and is resolved
	// through PATH by os/exec.
	Binary string

	// Output is an FFmpeg output path or URL and is required. RTMP/RTMPS URLs
	// automatically use the FLV muxer unless Format is set explicitly.
	Output string

	// Width and Height describe the raw RGB input. They default to 160x144.
	Width  int
	Height int

	// FPS is the input frame rate. It defaults to the native Game Boy rate.
	FPS float64

	// Scale is an integer nearest-neighbor output multiplier. It defaults to 4.
	// Set it to 1 to keep native resolution.
	Scale int

	// Realtime asks FFmpeg to pace raw input at FPS instead of consuming frames
	// as quickly as the producer can write them.
	Realtime bool

	// VideoCodec and Preset default to libx264 and veryfast. The default codec
	// uses H.264 with zerolatency tuning and yuv420p output.
	VideoCodec string
	Preset     string

	// VideoBitrate optionally sets -b:v (for example "2500k"). When empty,
	// FFmpeg uses CRF 18 for the default libx264 codec.
	VideoBitrate string

	// Format optionally forces the output muxer (for example "flv" or "mp4").
	Format string

	// LogLevel defaults to "warning".
	LogLevel string

	// ExtraArgs are inserted immediately before the output path/URL. They are
	// passed directly to exec.Command without shell interpretation.
	ExtraArgs []string

	// Stderr receives FFmpeg diagnostics. Nil discards them.
	Stderr io.Writer
}

// FFmpegSink implements gomeboy.FrameSink by writing RGB24 frames to an FFmpeg
// stdin pipe. It is intentionally synchronous: a slow encoder or realtime
// output applies backpressure to the emulator caller.
type FFmpegSink struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	width  int
	height int
	closed bool
}

// NewFFmpegSink starts FFmpeg and returns a sink ready to receive frames.
func NewFFmpegSink(opts FFmpegOptions) (*FFmpegSink, error) {
	normalized, args, err := FFmpegCommand(opts)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(normalized.Binary, args...)
	if normalized.Stderr != nil {
		cmd.Stderr = normalized.Stderr
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("gomeboy stream: ffmpeg stdin: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("gomeboy stream: start %s: %w", normalized.Binary, err)
	}

	return &FFmpegSink{
		cmd:    cmd,
		stdin:  stdin,
		width:  normalized.Width,
		height: normalized.Height,
	}, nil
}

// FFmpegCommand validates options and returns the normalized options plus the
// exact argument list passed to FFmpeg. It does not start a process.
func FFmpegCommand(opts FFmpegOptions) (FFmpegOptions, []string, error) {
	n, err := normalizeFFmpegOptions(opts)
	if err != nil {
		return FFmpegOptions{}, nil, err
	}

	args := []string{"-hide_banner", "-loglevel", n.LogLevel}
	if n.Realtime {
		args = append(args, "-re")
	}
	args = append(args,
		"-f", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", fmt.Sprintf("%dx%d", n.Width, n.Height),
		"-framerate", strconv.FormatFloat(n.FPS, 'f', 6, 64),
		"-i", "pipe:0",
		"-an",
	)

	if n.Scale != 1 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:flags=neighbor", n.Width*n.Scale, n.Height*n.Scale))
	}
	args = append(args, "-c:v", n.VideoCodec)
	if n.Preset != "" {
		args = append(args, "-preset", n.Preset)
	}
	if n.VideoCodec == "libx264" {
		args = append(args, "-tune", "zerolatency")
		if n.VideoBitrate == "" {
			args = append(args, "-crf", "18")
		}
	}
	if n.VideoBitrate != "" {
		args = append(args, "-b:v", n.VideoBitrate)
	}
	args = append(args, "-pix_fmt", "yuv420p")

	format := n.Format
	if format == "" && isRTMP(n.Output) {
		format = "flv"
	}
	if format != "" {
		args = append(args, "-f", format)
	}
	args = append(args, n.ExtraArgs...)
	args = append(args, n.Output)
	return n, args, nil
}

func normalizeFFmpegOptions(opts FFmpegOptions) (FFmpegOptions, error) {
	if strings.TrimSpace(opts.Output) == "" {
		return FFmpegOptions{}, fmt.Errorf("gomeboy stream: ffmpeg output is required")
	}
	if opts.Binary == "" {
		opts.Binary = "ffmpeg"
	}
	if opts.Width == 0 {
		opts.Width = GameBoyWidth
	}
	if opts.Height == 0 {
		opts.Height = GameBoyHeight
	}
	if opts.Width < 1 || opts.Height < 1 {
		return FFmpegOptions{}, fmt.Errorf("gomeboy stream: dimensions must be positive, got %dx%d", opts.Width, opts.Height)
	}
	if opts.FPS == 0 {
		opts.FPS = GameBoyFPS
	}
	if opts.FPS <= 0 {
		return FFmpegOptions{}, fmt.Errorf("gomeboy stream: fps must be > 0")
	}
	if opts.Scale == 0 {
		opts.Scale = 4
	}
	if opts.Scale < 1 {
		return FFmpegOptions{}, fmt.Errorf("gomeboy stream: scale must be >= 1")
	}
	if opts.VideoCodec == "" {
		opts.VideoCodec = "libx264"
	}
	if opts.Preset == "" && opts.VideoCodec == "libx264" {
		opts.Preset = "veryfast"
	}
	if opts.LogLevel == "" {
		opts.LogLevel = "warning"
	}
	return opts, nil
}

func isRTMP(output string) bool {
	lower := strings.ToLower(output)
	return strings.HasPrefix(lower, "rtmp://") || strings.HasPrefix(lower, "rtmps://")
}

// WriteFrame implements gomeboy.FrameSink. RGB is consumed before the method
// returns, so the emulator may safely reuse its zero-copy framebuffer afterward.
func (s *FFmpegSink) WriteFrame(frame uint64, cycle uint64, image gomeboy.Frame) error {
	if image.Width != s.width || image.Height != s.height {
		return fmt.Errorf("gomeboy stream: frame %d has dimensions %dx%d, want %dx%d", frame, image.Width, image.Height, s.width, s.height)
	}
	return s.WriteRGB(image.RGB)
}

// WriteRGB writes one raw RGB24 frame. This is useful for external controllers
// that already have raw framebuffer bytes and do not need a gomeboy.Frame.
func (s *FFmpegSink) WriteRGB(rgb []byte) error {
	if s == nil || s.stdin == nil || s.closed {
		return fmt.Errorf("gomeboy stream: ffmpeg sink is closed")
	}
	want := s.width * s.height * 3
	if len(rgb) != want {
		return fmt.Errorf("gomeboy stream: RGB frame is %d bytes, want %d", len(rgb), want)
	}
	for len(rgb) > 0 {
		n, err := s.stdin.Write(rgb)
		if err != nil {
			return fmt.Errorf("gomeboy stream: write ffmpeg frame: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("gomeboy stream: write ffmpeg frame: short write")
		}
		rgb = rgb[n:]
	}
	return nil
}

// Close finishes FFmpeg input and waits for the encoder process to exit.
func (s *FFmpegSink) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true
	closeErr := s.stdin.Close()
	waitErr := s.cmd.Wait()
	if closeErr != nil {
		return fmt.Errorf("gomeboy stream: close ffmpeg stdin: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("gomeboy stream: ffmpeg exited: %w", waitErr)
	}
	return nil
}

var _ gomeboy.FrameSink = (*FFmpegSink)(nil)
