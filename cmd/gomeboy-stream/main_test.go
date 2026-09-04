package main

import (
	"io"
	"testing"

	gbstream "github.com/maestroi/gomeboy/pkg/gomeboy/stream"
)

func TestParseConfigStdinDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{"-output", "capture.mp4"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.output != "capture.mp4" || cfg.width != 160 || cfg.height != 144 || cfg.scale != 4 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.fps != gbstream.GameBoyFPS {
		t.Fatalf("fps = %v, want %v", cfg.fps, gbstream.GameBoyFPS)
	}
	if cfg.recording != "" || cfg.rom != "" {
		t.Fatalf("stdin mode unexpectedly configured replay: %+v", cfg)
	}
}

func TestParseConfigReplay(t *testing.T) {
	cfg, err := parseConfig([]string{
		"-output", "rtmp://example.invalid/live/key",
		"-recording", "run.gbrun",
		"-rom", "game.gb",
		"-realtime",
		"-bitrate", "2500k",
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.recording != "run.gbrun" || cfg.rom != "game.gb" || !cfg.realtime || cfg.bitrate != "2500k" {
		t.Fatalf("unexpected replay config: %+v", cfg)
	}
}

func TestParseConfigRejectsInvalidModes(t *testing.T) {
	cases := [][]string{
		{},
		{"-output", "x.mp4", "-recording", "run.gbrun"},
		{"-output", "x.mp4", "-rom", "game.gb"},
		{"-output", "x.mp4", "-scale", "0"},
		{"-output", "x.mp4", "-recording", "run.gbrun", "-rom", "game.gb", "-width", "320"},
	}
	for _, args := range cases {
		if _, err := parseConfig(args, io.Discard); err == nil {
			t.Fatalf("parseConfig accepted invalid args: %v", args)
		}
	}
}
