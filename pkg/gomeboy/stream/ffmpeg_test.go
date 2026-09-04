package stream

import (
	"reflect"
	"testing"
)

func TestFFmpegCommandDefaults(t *testing.T) {
	n, args, err := FFmpegCommand(FFmpegOptions{Output: "capture.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if n.Binary != "ffmpeg" || n.Width != 160 || n.Height != 144 || n.Scale != 4 || n.VideoCodec != "libx264" {
		t.Fatalf("unexpected normalized options: %+v", n)
	}

	want := []string{
		"-hide_banner", "-loglevel", "warning",
		"-f", "rawvideo",
		"-pixel_format", "rgb24",
		"-video_size", "160x144",
		"-framerate", "59.727501",
		"-i", "pipe:0",
		"-an",
		"-vf", "scale=640:576:flags=neighbor",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-tune", "zerolatency",
		"-crf", "18",
		"-pix_fmt", "yuv420p",
		"capture.mp4",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("FFmpeg args:\n got %#v\nwant %#v", args, want)
	}
}

func TestFFmpegCommandRTMPIsRealtimeWhenRequested(t *testing.T) {
	_, args, err := FFmpegCommand(FFmpegOptions{
		Output:       "rtmps://example.invalid/live/key",
		Realtime:     true,
		Scale:        1,
		VideoBitrate: "2500k",
	})
	if err != nil {
		t.Fatal(err)
	}

	mustContainPair(t, args, "-f", "flv")
	mustContainPair(t, args, "-b:v", "2500k")
	if !contains(args, "-re") {
		t.Fatalf("args do not contain -re: %#v", args)
	}
	if contains(args, "-vf") {
		t.Fatalf("native scale unexpectedly added filter: %#v", args)
	}
	if contains(args, "-crf") {
		t.Fatalf("bitrate mode unexpectedly added CRF: %#v", args)
	}
}

func TestFFmpegCommandRejectsInvalidOptions(t *testing.T) {
	for _, tc := range []FFmpegOptions{
		{},
		{Output: "x.mp4", Width: -1},
		{Output: "x.mp4", FPS: -1},
		{Output: "x.mp4", Scale: -1},
	} {
		if _, _, err := FFmpegCommand(tc); err == nil {
			t.Fatalf("FFmpegCommand accepted invalid options: %+v", tc)
		}
	}
}

func mustContainPair(t *testing.T, args []string, key, value string) {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return
		}
	}
	t.Fatalf("args do not contain %q %q: %#v", key, value, args)
}

func contains(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
