//go:build !android

package glfw

import "testing"

func TestOSDScaleTracksDesktopResolution(t *testing.T) {
	tests := []struct {
		name          string
		width, height int32
		want          int32
	}{
		{name: "windowed", width: 640, height: 576, want: 1},
		{name: "720p", width: 1280, height: 720, want: 2},
		{name: "1080p", width: 1920, height: 1080, want: 2},
		{name: "1440p", width: 2560, height: 1440, want: 3},
		{name: "4k", width: 3840, height: 2160, want: 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := osdScale(tc.width, tc.height); got != tc.want {
				t.Fatalf("osdScale(%d, %d) = %d, want %d", tc.width, tc.height, got, tc.want)
			}
		})
	}
}
