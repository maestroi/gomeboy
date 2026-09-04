package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/maestroi/gomeboy/pkg/gomeboy"
	gbstream "github.com/maestroi/gomeboy/pkg/gomeboy/stream"
)

type config struct {
	output       string
	ffmpeg       string
	width        int
	height       int
	fps          float64
	scale        int
	realtime     bool
	codec        string
	preset       string
	bitrate      string
	format       string
	logLevel     string
	rom          string
	recording    string
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gomeboy-stream:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	sink, err := gbstream.NewFFmpegSink(gbstream.FFmpegOptions{
		Binary:       cfg.ffmpeg,
		Output:       cfg.output,
		Width:        cfg.width,
		Height:       cfg.height,
		FPS:          cfg.fps,
		Scale:        cfg.scale,
		Realtime:     cfg.realtime,
		VideoCodec:   cfg.codec,
		Preset:       cfg.preset,
		VideoBitrate: cfg.bitrate,
		Format:       cfg.format,
		LogLevel:     cfg.logLevel,
		Stderr:       stderr,
	})
	if err != nil {
		return err
	}

	var streamErr error
	if cfg.recording != "" {
		streamErr = replayRecording(cfg, sink)
	} else {
		streamErr = streamRGB(stdin, sink, cfg.width*cfg.height*3)
	}
	closeErr := sink.Close()
	if streamErr != nil {
		if closeErr != nil {
			return errors.Join(streamErr, closeErr)
		}
		return streamErr
	}
	return closeErr
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("gomeboy-stream", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var cfg config
	fs.StringVar(&cfg.output, "output", "", "FFmpeg output file or URL (required)")
	fs.StringVar(&cfg.ffmpeg, "ffmpeg", "ffmpeg", "FFmpeg executable")
	fs.IntVar(&cfg.width, "width", gbstream.GameBoyWidth, "raw RGB input width")
	fs.IntVar(&cfg.height, "height", gbstream.GameBoyHeight, "raw RGB input height")
	fs.Float64Var(&cfg.fps, "fps", gbstream.GameBoyFPS, "input/output frame rate")
	fs.IntVar(&cfg.scale, "scale", 4, "integer nearest-neighbor upscale factor")
	fs.BoolVar(&cfg.realtime, "realtime", false, "pace FFmpeg input at -fps instead of encoding as fast as possible")
	fs.StringVar(&cfg.codec, "codec", "libx264", "FFmpeg video codec")
	fs.StringVar(&cfg.preset, "preset", "veryfast", "FFmpeg encoder preset")
	fs.StringVar(&cfg.bitrate, "bitrate", "", "optional video bitrate such as 2500k")
	fs.StringVar(&cfg.format, "format", "", "optional FFmpeg output muxer such as flv or mp4")
	fs.StringVar(&cfg.logLevel, "log-level", "warning", "FFmpeg log level")
	fs.StringVar(&cfg.rom, "rom", "", "ROM path when replaying a .gbrun recording")
	fs.StringVar(&cfg.recording, "recording", "", "optional .gbrun to replay instead of reading RGB24 frames from stdin")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: gomeboy-stream -output <file-or-url> [options]\n\n")
		fmt.Fprintf(fs.Output(), "Live/stdin mode reads contiguous RGB24 frames from stdin.\n")
		fmt.Fprintf(fs.Output(), "Replay mode uses -recording run.gbrun -rom game.gb.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if cfg.output == "" {
		return config{}, fmt.Errorf("-output is required")
	}
	if cfg.recording != "" && cfg.rom == "" {
		return config{}, fmt.Errorf("-rom is required with -recording")
	}
	if cfg.recording == "" && cfg.rom != "" {
		return config{}, fmt.Errorf("-rom is only used with -recording")
	}
	if cfg.width < 1 || cfg.height < 1 {
		return config{}, fmt.Errorf("-width and -height must be positive")
	}
	if cfg.fps <= 0 {
		return config{}, fmt.Errorf("-fps must be > 0")
	}
	if cfg.scale < 1 {
		return config{}, fmt.Errorf("-scale must be >= 1")
	}
	if cfg.recording != "" && (cfg.width != gbstream.GameBoyWidth || cfg.height != gbstream.GameBoyHeight) {
		return config{}, fmt.Errorf("replay mode requires native %dx%d input dimensions", gbstream.GameBoyWidth, gbstream.GameBoyHeight)
	}
	return cfg, nil
}

func streamRGB(r io.Reader, sink *gbstream.FFmpegSink, frameSize int) error {
	buf := make([]byte, frameSize)
	for {
		n, err := io.ReadFull(r, buf)
		switch {
		case err == nil:
			if err := sink.WriteRGB(buf); err != nil {
				return err
			}
		case errors.Is(err, io.EOF) && n == 0:
			return nil
		case errors.Is(err, io.ErrUnexpectedEOF):
			return fmt.Errorf("stdin ended with partial RGB frame: got %d of %d bytes", n, frameSize)
		default:
			return fmt.Errorf("read RGB frame: %w", err)
		}
	}
}

func replayRecording(cfg config, sink *gbstream.FFmpegSink) error {
	recording, err := gomeboy.LoadRecording(cfg.recording)
	if err != nil {
		return err
	}
	emu, err := gomeboy.New(
		gomeboy.WithROM(cfg.rom),
		gomeboy.Headless(),
	)
	if err != nil {
		return err
	}
	defer emu.Close()

	return emu.ReplayRecordingFrames(recording, func(frame uint64, image gomeboy.Frame) error {
		return sink.WriteFrame(frame, emu.Cycle(), image)
	})
}
